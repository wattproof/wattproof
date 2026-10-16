# Slurm trace analysis plan: how much idle time a Slurm cluster has, and what powering it off costs

Status: stated 2026-10-16, after reading the schema and five rows of the first trace, before any
analysis has run. Changes after the first results are listed at the end with their reason.

## Purpose

Slurm already powers idle nodes off after a fixed idle time
([feature note](features/slurm.md), [ADR-0015](../adr/0015-slurm-observe-and-verify-first.md)).
Before any site's data, public traces answer three questions:

1. **How much node time is idle**, by hour of week and node class (CPU, GPU)?
2. **How much of it comes in intervals long enough to power off**, for a given idle timer
   (`SuspendTime`) and boot time?
3. **What does reactive power-off cost in job wait**, and how much of that would a perfect forecast
   of the next job remove?

Question 3 bounds what a Wattproof planner could add on top of Slurm's own power saving. If the
bound is small, Slurm work stays verification only.

## Data

| Source | Role | Licence |
|---|---|---|
| SURF Lisa (Chu et al., ICPADS 2024; Zenodo 11028934): 343 nodes (about 15% with GPUs), Slurm job table December 2021 to November 2022; node metrics with CPU and GPU power at 30 s, June to November 2022 | Primary. Job table first; node power second, for idle draw by node class | CC-BY-4.0 |
| CINECA Marconi100 (M100 ExaData, Borghesi et al., Scientific Data 2023; PM100): 980 GPU nodes, Slurm job table March 2020 to September 2022, IPMI sensor data | Replication on a second, GPU-only site | Check before use |

One site is not enough: the same rule as in the [trace analysis plan](trace-analysis-plan.md).

## Definitions, fixed before the results are seen

- **Busy:** a node is busy at time t if any job row on it has start ≤ t < end, whatever the number
  of cores. Slurm suspends only nodes with no job, so a shared node counts as busy.
- **Rows used:** every job state, including failed, cancelled and timed-out jobs, because they held
  the node. Rows with end ≤ start, or with a start or end missing, are dropped and counted.
- **Window:** from the trace's first job start plus 7 days (jobs running before the trace began are
  missing) to its last job end minus 1 day. Lisa's longest time limit is checked in the data; if it
  exceeds 7 days, the warm-up becomes that limit.
- **Nodes:** those in `node_hardware_info` that run at least one job in the window. GPU node:
  `gpu_count` > 0.
- **Idle interval:** a maximal span in the window in which the node is not busy.
- **Unavailable:** without node-state data, a node that is down cannot be told from one that is
  idle. Idle intervals of 72 hours or more are classified as unavailable, and reported separately.
  Every result is also given with them counted as idle.
- **Time zone:** the timestamps' zone is not documented. If the hourly profile of job submissions
  has its minimum between 01:00 and 06:00 UTC, the timestamps are taken as UTC and shown in
  Amsterdam time; otherwise they are taken as local time. The rule and its outcome are reported.

## Measures

1. **Idle share:** idle node-hours over available node-hours, per node class, overall and by hour of
   week.
2. **Powered-off share** under a reactive policy (Slurm's): a node powers off after `SuspendTime`
   of idle and stays off until the next job is allocated to it; the boot then happens while the job
   waits.
   `SuspendTime` of 10, 30 and 60 minutes; boot time of 5 and 15 minutes (the HPC range reported in
   the feature note starts at 10–15).
3. **Wake cost:** job rows that start on a node powered off at that moment, and the boot minutes
   they wait. First order: the job is delayed by the boot, and nothing else in the schedule
   changes. This understates real queue effects and is stated as a lower bound.
4. **Forecast bound:** the same policy with perfect foresight: each node is powered on exactly one
   boot time before its next job. No wake cost; off-time shrinks by the boots that now happen
   before the job instead of during its wait. The gap between 3 and 4 is the most a forecast could
   remove.
5. **Energy:** off node-hours times (idle draw − off draw), minus each cycle's transition energy
   above idle, per node class. Idle draw comes from the node power metrics (second step), labelled
   with their source; until then results are given in node-hours only.

## Energy step, stated 2026-10-16 before any power data was opened

- **Sources.** Marconi100: `ipmi_pub` `ps0_input_power` + `ps1_input_power`, the input power of the
  node's two power supplies read from the BMC: whole-node power, class C. Lisa: the node metrics'
  power fields; if they cover only CPU and GPU (RAPL, NVML), they are class D and give a **lower
  bound** on the node's idle draw, stated as such.
- **Idle draw per node:** the median power over samples in idle intervals, excluding the first and
  last 5 minutes of each interval (job tails and starts). Reported per node class as the median
  and the 10th–90th percentile across nodes.
- **Busy draw per node:** the median power over samples while busy, for the transition cost.
- **Off draw:** not in the data (a powered-off node reports nothing). Swept: 10 W and 30 W per
  node.
- **Transition cost:** each power cycle costs one boot time at the node's busy draw minus its idle
  draw (conservative: a boot rarely draws the busy median), with boot times of 5 and 15 minutes as
  above.
- **Saving:** over all idle intervals, off-time × (idle draw − off draw) minus transition costs,
  per policy. Reported in kWh and as a share of the measured energy of the same nodes over the same
  window (Marconi100), or of their estimated energy (Lisa, if only class D).
- **Marconi100 jobs:** monthly job tables are merged and deduplicated by job id; the window and
  warm-up follow the definitions above.

## Decision rule, stated in advance

- If the powered-off share at `SuspendTime` = 30 minutes is below **5% of available node-hours**
  on both sites, power state on Slurm clusters like these is a small lever: Slurm work stays
  verification (Verify study V2) and the Slurm report.
- If the reactive policy delays fewer than **1% of job starts**, a forecast has nothing to remove,
  and a Slurm planner is dropped. If it delays more, the forecast bound is reported as the target a
  Slurm planner would have to approach in simulation before any build.

## Changes to this plan

2026-10-16, after the first run on SURF Lisa:

- **Host lists expanded.** The job table has one row per job; multi-node jobs list their nodes as a
  Slurm host list (`r25n[18,20],r27n4`). The first run read each list as one unknown node and
  dropped it, which counted those nodes as idle (CPU idle 50% instead of 21%). The script now
  expands host lists. No definition changed.
- **Marconi100 months.** May to October 2020 (PM100's period) and January to June 2021. The system
  entered production in 2020: in May 2020, 58% of node time was idle, which is not a steady state.
  2021 was added before any 2021 data was opened. Each period is reported on its own.
- **Complete days only on Marconi100.** Checked against Slurm's own node counts (`slurm_pub`), the
  job table has collection gaps: on some days its jobs cover far fewer busy nodes than Slurm
  reports (1 August 2020: 226 against 486), and October 2020 ends on the 13th. A missing job reads
  as idle time, which overstated idle by 2–15 points a month. Only days on which the job table's
  busy node-days are within 25% of Slurm's allocated plus mixed nodes are used; idle intervals
  are cut at excluded days. Found after the first 2020 run, so stated as a change.
- **A power check where Slurm's counts are missing.** The 2021 months carry no node counts. A day
  is also checked by power: the measured mean node power must be within 10% of what the job table
  predicts from the median idle and busy draw (missing jobs make measured power higher). The 10%
  was set before the check was run; its agreement with the counts check on 2020 is reported, and
  2021 uses it only as the counts check's stand-in.
- **Job time alignment on Lisa.** The job table's time zone is undocumented and differs from the
  submit column. The energy step shifts job times by the whole hour (−3 to +3) at which idle
  minutes draw least relative to busy ones, ties to the smaller shift, and reports the scan.

2026-10-16, after both sites' first delay results, before the check below was run:

- **Re-placement check.** The wake cost above keeps every job on the node it ran on in the trace.
  Slurm does not: it "will allocate from powered up nodes first, powering up nodes second and
  lastly powered down nodes" ([power saving guide](https://slurm.schedmd.com/power_save.html),
  node `Weight`). So the delayed starts are recounted. A delayed start is **avoidable** if, at its
  start time, each of its powered-off nodes can be swapped for a node of the same hardware type
  that is idle and still powered on (idle for less than `SuspendTime`). Hardware type: CPU model
  and GPU model on Lisa (seven types); Marconi100's nodes are one type. Delayed starts are taken
  in time order; a node used for a swap is not available again in the same idle interval.
  Nodes in idle intervals of 72 hours or more count as unavailable, as above.
- **This is optimistic.** It does not follow the consequences: the skipped node stays off, and
  its next job may then wait. Partitions, node lists requested by jobs and core counts on shared
  nodes are ignored. So the avoidable share is an upper bound on what Slurm's own placement removes,
  and the trace placement (nothing avoidable) is the lower bound.
- **How it enters the decision rule.** One way only: if the delayed starts that remain after
  re-placement are below 1% on both sites, the Slurm planner is dropped even where the original
  count is above 1%. A result above 1% keeps the rule as stated, with both counts reported.
- **The power check's effect on the results.** On 2020 the power check agrees with the counts
  check on 107 of 137 days (78%), and passes 20 days the counts reject. Agreement on days does not
  say how much the results move, so 2020 is also run with the power check alone, and the
  difference in idle share, delayed starts and saving is reported. 2021 (power check only) is read
  with that difference as its uncertainty. Stated before the power-only run.
- **Result of that run, and a third check.** With the power check alone, 2020 reads 3.6 points
  more idle, a 4.4–4.6 points larger saving and 1.1 points more delayed starts (after
  re-placement) than with the counts check: it passes days with missing jobs, because a 10% band on
  mean power hides a tenth of the busy nodes going missing. So 2021 gets a direct check instead.
  Slurm's own record of finished jobs (`slurm_pub`, `metric=job_id`: one row per job, at its end,
  in 2020 and 2021) is compared with the job table: **a day is complete if at least 95% of the
  job ids Slurm records as ending that day are in the job table.** Fixed before the check was
  run. It is calibrated on 2020 as the power check was (agreement with the counts check, and 2020
  run with it alone); 2021 uses it in place of the counts, and the power check is dropped.
- **2021 fails on data quality and is not used.** The job-record check passed every 2021 day
  (coverage 99–100%), but the run showed idle nodes drawing more than busy ones (997 W against
  820 W). Node-days on which the job table shows no job drew a median of 823–851 W in February,
  April and June 2021, 75–84% of them above 700 W; in 2020 such node-days drew about 540 W
  (except after the job table ends on 13 October). The job table misses much of the running work,
  and Slurm's job record misses the same jobs, so it is not an independent check. On 2020 it
  agreed with the counts check on 110 of 137 days and moved the results by at most half a point,
  because 2020's gaps are fewer. A 2021 analysis would need idle defined from power, stated and
  validated on 2020 first. Not done.

2026-10-16, after both sites' results and before any new site or period is opened:

- **The 1%-of-starts rule is replaced by energy at equal wait.** The share of delayed starts does
  not grow with a cluster, and it says nothing about energy: a site that accepts some waits gets
  more energy from a shorter timer, and a forecast can give that energy without the waits. So the
  planner is judged by the energy it could add while waiting no more than Slurm's timer does.
- **Measure, per site and node class:** ΔE = the forecast bound's saving at `SuspendTime`
  10 minutes (the shortest tested; a real forecast cannot power off the moment a node goes idle)
  minus the reactive saving at the reference `SuspendTime` of 30 minutes, at the same boot time and
  off draw, in points of the node class's measured energy. The forecast bound has no boot waits,
  so ΔE is energy gained with fewer waits than the reference, never more. It is an upper bound: the
  forecast is perfect.
- **Reported next to it, not as a bar:** the reference policy's delayed starts after re-placement,
  and its boot wait in node-hours per 1,000 nodes per year, so that sites of different size compare.
- **Decision rule.** ΔE is taken at the 10 W and 30 W off draws and at the 5 and 15-minute boots,
  and the lowest of the four decides. If it is below **1 point** on every site and node class, a
  Slurm planner is dropped: 1 point is about the spread that the unmeasured off draw alone puts on
  the saving (0.9 points on Lisa's GPU nodes, 1.1 on Marconi100), so a smaller gain cannot be told
  from the assumptions.
  Otherwise the lowest ΔE is the planner's target: in simulation with a forecast it could actually
  run, it must reach at least half of it with no more delayed starts than the reference, before
  any build. The 5% rule on the powered-off share is unchanged.
- **Where it applies.** To every site and period opened from now on. Lisa and Marconi100 2020 are
  recomputed under it and labelled as computed after the rule was written: a rough reading of
  Marconi100's output, about 1.6 points at a 5-minute boot, had been seen before writing it.
