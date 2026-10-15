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
