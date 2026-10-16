# 2026-10-16: What powering idle Slurm nodes off saves, and what it costs (Lisa, Marconi100)

**Trace analysis with measured node power, two sites.** It continues
[idle node time on SURF Lisa](2026-10-16-slurm-idle-surf-lisa.md) with the plan's energy step and
the second site, CINECA's Marconi100 (M100 ExaData, Borghesi et al., Scientific Data 2023; Zenodo
7588815, CC-BY-4.0). Definitions, the energy step and the decision rules were stated in the
[Slurm trace analysis plan](../research/slurm-trace-plan.md) before the data was opened; the
changes made after first results are listed there with their reasons. Evidence level E2: public
traces, no intervention.

Reproduce (see [analysis/traces](../../analysis/traces/README.md)):

    uv run --no-project --with duckdb python -I analysis/traces/slurm_idle.py <lisa dir>
    uv run --no-project --with duckdb python -I analysis/traces/slurm_energy.py <lisa dir> '<power glob>' --tmp <dir>
    uv run --no-project --with duckdb python -I analysis/traces/slurm_m100.py <m100 dir> --tmp <dir>

## Data

- **Lisa:** job table as before; node power from the same Zenodo record, June to November 2022
  (144.8 days, 128 million samples at 30 s). `node_power_usage` is the BMC's node reading (class C);
  CPU package (RAPL) and GPU board (NVML) power are class D. The job-time alignment scan (−3 to
  +3 h) found no better shift than 0 h.
- **Marconi100:** 980 identical IBM Power9 nodes with four V100 GPUs each. May to October 2020:
  job table, both power supplies' input power per node (`ps0`, `ps1`, summed per minute, class C),
  and Slurm's own node counts. The job table has collection gaps; days on which its busy node-days
  are not within 25% of Slurm's allocated plus mixed nodes are excluded. 109 of 149 days pass.
  On those days, idle share by month from the job table against Slurm's own idle-or-down share:
  60/59%, 51/50%, 51/50%, 59/57%, 40/39%, 20/18% (May to October).
- **Was power saving already on?** Not on either site. Lisa's nodes rebooted a median of 3–4
  times in 145 days. On Marconi100, 0.5% of power supply readings are below 50 W, most from a few
  nodes, and idle nodes drew 517–560 W. The lowest readings, about 10 W per supply, are close to
  the 10–30 W assumed for a powered-off node.

## Idle and busy draw

| Nodes | Idle draw, median (10th–90th) | Busy draw | Idle / busy |
|---|---|---|---|
| Lisa CPU (286) | 36 W (36–44), CPU packages 13 W | 140 W (120–156) | 26% |
| Lisa GPU (51) | 432 W (360–480), GPU boards 84 W | 648 W (384–888) | 67% |
| Marconi100 (980) | 530 W (517–560) | 810 W (693–857) | 65% |

Lisa's CPU nodes are mostly single-socket servers in shared chassis (191 of 289 have one CPU). A
36 W reading is plausible for the node's own board, but it leaves out the chassis' shared fans
and power supplies. **Lisa's CPU-node savings are a lower bound.** The CPU packages' 13 W is below
it, so the reading is not contradicted.

## Results

`SuspendTime` 30 minutes, boot 5 minutes (15 in brackets), gaps of 72 hours or more counted as
unavailable. Saving: off-time × (idle draw − off draw) minus one boot at busy − idle draw per cycle,
for 10 W and 30 W off draw, as a share of the measured energy of the same nodes over the same
period.

| | Lisa CPU | Lisa GPU | Marconi100 2020 |
|---|---|---|---|
| Powered off, reactive | 18.6% | 24.0% | 39.5% |
| Saving, reactive | 1.2–4.3% (0.8–3.9%), lower bound | 18.0–18.9% (17.7–18.7%) | 26.7–27.8% (26.3–27.3%) |
| Saving, perfect forecast | same within 0.1 point | 0.2 points less | 0.4 points less (1.0) |
| Job starts delayed, trace placement | 3.77% | 0.88% | 7.48% |
| Delayed after re-placement (optimistic) | 3.08% | 0.76% | 4.16% |
| Boot wait left, node-hours | 4,451 (13,352) in 10 months | 759 (2,277) | 10,142 (30,426) in 109 days |

Measured energy in the power windows: Lisa CPU 109 MWh and GPU 73 MWh over 145 days; Marconi100
1,760 MWh over the 109 complete days. Other timers: at 10 minutes, Marconi100 saves 28.7–29.9% and
delays 10.9% of starts (6.85% after re-placement); at 60 minutes, 24.6–25.5% and 5.59% (2.85%).

**Re-placement** ([plan change](../research/slurm-trace-plan.md#changes-to-this-plan)): Slurm
allocates powered-up nodes first. A delayed start counts as avoidable if each of its powered-off
nodes can be swapped for an idle, still powered-on node of the same hardware type. It is
optimistic: the skipped node stays off and its next job is not followed. It removes 18% of the delayed
starts on Lisa's CPU nodes, 13% on its GPU nodes and 44% on Marconi100.

## Reading

1. **Powering idle nodes off is a large lever on both clusters.** At `SuspendTime` = 30 minutes,
   19–24% of Lisa's and 40% of Marconi100's available node-hours would be powered off, far above
   the plan's 5% bar. On GPU nodes, which draw two thirds of their busy power at idle, that is
   18–19% of Lisa's GPU energy and 27–28% of Marconi100's.
2. **That saving is Slurm's own.** Slurm's power saving delivers it with a timer. A perfect
   forecast adds nothing to it: it saves 0.1–1.0 points less, because it boots before the job
   instead of during the wait.
3. **What a forecast could remove is the wait.** Even after the optimistic re-placement, 3.1% of
   Lisa's CPU job starts and 4.2% of Marconi100's would wait for a boot. Both are above the 1% bar.
   Lisa's GPU nodes (0.76%) are below it.
4. **By the decision rules** (both sites, as stated): power state is not a small lever, so the
   Slurm work goes beyond verification; and since delays stay above 1% after re-placement, the
   Slurm planner is not dropped. Its target is the forecast bound: the waits above removed at the
   cost of 0.1–1.0 points of saving. It must approach that bound in simulation before any build.
5. **What this means for a Slurm site.** First the observation that both sites ran without power
   saving, leaving a quarter of GPU-node energy on the table; then verification of what enabling
   it saves (Verify study V2); then, only if simulation reaches the bound, a planner that removes
   the waits.

## Limits

- Two sites, both without power saving in the period. A site already running it has no saving
  left to find, only the waits.
- Marconi100 2020 was its first production year: 60% idle in May, 20% in October. The planned
  replication on January to June 2021 failed on data quality (below), so Marconi100 rests on one
  period.
- First order: delayed jobs delay nothing else, and re-placement does not follow the skipped node.
  Job wait in real queues compounds; the re-placement bound may remove more or fewer waits than
  Slurm would.
- Off draw and boot time are swept, not measured. Boot energy is charged at the busy draw.
- No node states on Lisa: down and idle cannot be told apart; gaps of 72 hours or more are a
  proxy, and every result is also computed with them counted as idle (in the scripts' output).

## Checking Marconi100's job table

A missing job reads as idle time, so the job table's completeness decides every Marconi100 figure.
Three day-level checks were compared on 2020, where Slurm's own node counts exist (the
[plan](../research/slurm-trace-plan.md#changes-to-this-plan) states each before its run):

| 2020, `SuspendTime` 30 min | By Slurm's node counts (used) | By power alone | By Slurm's job record alone |
|---|---|---|---|
| Days passed | 109 of 137 | 125 | 138 |
| Agreement with the counts check | | 78% of days | 80% of days |
| Idle share by month above Slurm's own | +1 to +2 points | up to +11 | +1 to +3 |
| Saving | 26.7–27.8% | 31.1–32.4% | 27.2–28.3% |
| Delayed after re-placement | 4.16% | 5.27% | 4.53% |

The power check (mean node power within 10% of the job table's prediction) is too loose: a tenth
of the busy nodes going missing barely moves a day's mean power.

**January to June 2021 is not used.** It has no node counts, and the job-record check passed every
day, yet the run gave idle nodes a higher draw than busy ones (997 W against 820 W). A direct check
([`slurm_m100_nodeday.py`](../../analysis/traces/slurm_m100_nodeday.py)) shows why: on days the
job table shows a node with no job, that node drew a median of 823–851 W in February, April and
June 2021, with 75–84% of those node-days above 700 W. In 2020 such node-days drew about 540 W,
except after the job table ends on 13 October. The 2021 job table misses much of the running work,
and Slurm's job record misses the same jobs. The 2020 figures do not depend on that record: they
use Slurm's node counts, against which the job table's monthly idle share agrees within 1–2
points.

## Next

1. A Slurm planner simulation against the forecast bound: the waits left after re-placement, at
   the cost of 0.1–1.0 points of saving.
2. A second clean Slurm period or site for replication. For Marconi100 2021, that would mean idle
   time defined from power, stated and validated on 2020 first.
