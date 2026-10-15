# 2026-10-16: Idle node time on a Slurm cluster (SURF Lisa)

**Trace analysis, no power yet.** Job table of SURF's Lisa cluster, a Dutch national cluster for
universities: 343 nodes, 54 of them with GPUs, January to October 2022 (Chu et al., ICPADS 2024;
Zenodo 11028934, CC-BY-4.0). Definitions and decision rule were stated in the
[Slurm trace analysis plan](../research/slurm-trace-plan.md) before the analysis ran. One site;
the second (CINECA Marconi100) is still to come. Results are in node-hours; energy needs the
node power data (next step).

Reproduce:

    uv run --no-project --with duckdb python -I analysis/traces/slurm_idle.py <dir>

with `slurm_table_cleaned.parquet` and `node_hardware_info.parquet` from the Zenodo record in
`<dir>`.

## Data

- 1,596,963 job rows; 52,224 dropped (jobs that never ran: no node, or end not after start).
  Multi-node jobs list their nodes as Slurm host lists; 19,530 such rows were expanded.
- Window: 2 January to 31 October 2022, 301.6 days. Longest job 5.09 days, so the 7-day warm-up
  holds.
- 343 nodes. Excluded: `software1`, `software2` and `r38n5`, which are not in the hardware table.
- Time zone: submissions are fewest at 03:00 UTC, so by the plan's rule the timestamps are taken as
  UTC and shown in CET. But `start_date` lies a median 1.6 hours before the submit time, so the two
  columns are not in the same zone. The hour-of-day profile below may be shifted by one or two
  hours. Nothing else depends on it.

## Results

Gaps of 72 hours or more counted as unavailable (the plan's main reading). Off: share of available
node-hours a node would be powered off. Delayed: job starts that find a node off, as a share of all
1,538,594 starts in the window.

| Nodes | Idle | SuspendTime | Off, reactive | Off, perfect forecast | Delayed (reactive) | Boot wait, node-hours (5 / 15 min boot) |
|---|---|---|---|---|---|---|
| CPU (289) | 21.2% | 10 min | 19.9% | 19.5–18.9% | 5.50% | 7,974 / 23,922 |
| CPU | | 30 min | 18.6% | 18.4–17.9% | 3.77% | 5,476 / 16,428 |
| CPU | | 60 min | 17.2% | 17.0–16.6% | 2.85% | 4,211 / 12,632 |
| GPU (54) | 26.3% | 10 min | 25.2% | 24.9–24.3% | 1.22% | 1,267 / 3,802 |
| GPU | | 30 min | 24.0% | 23.7–23.2% | 0.88% | 896 / 2,690 |
| GPU | | 60 min | 22.6% | 22.4–22.0% | 0.71% | 713 / 2,140 |

The forecast range is for 5 and 15 minutes of boot. With gaps of 72 hours or more counted as idle,
every share rises by about 2 points on CPU nodes and 10 on GPU nodes (GPU idle 36.2%).

Idle share by hour of day (CET), CPU nodes: lowest 16% from 12:00 to 16:00, highest 28% at 03:00
and 04:00. By day: 15–16% Tuesday to Friday, 23% Monday, 26% Saturday, 31% Sunday. GPU nodes follow
the same pattern: 18–30% by hour, 18–32% by day.

## Reading

1. **A lot of idle node time, mostly in long intervals.** At `SuspendTime` = 30 minutes, 18.6% of
   CPU node-hours and 24.0% of GPU node-hours would be powered off: above the plan's 5% bar on this
   site. Lengthening the timer from 10 to 60 minutes loses less than 3 points.
2. **The cost is job wait, and it is mostly on CPU nodes.** Reactive power-off makes 3.8% of job
   starts wait for a boot at 30 minutes, 5.5% at 10 minutes. With a 15-minute boot that is 16,000
   node-hours of waiting over ten months on CPU nodes. GPU nodes have fewer, longer jobs: under 1%
   of starts wait.
3. **A perfect forecast saves almost no extra energy; it removes the wait.** Forecast power-on
   gives up 0.2–1.0 points of off time and removes every boot wait. So on Slurm, a planner's case
   is job wait at the same energy, or a shorter timer at the same wait. It is not more energy
   saved. This matches the feature note's expectation.
4. **The daily cycle is mild.** Idle time is spread over the week, with more at night and at
   weekends. A timer catches most of it; a forecast matters for the morning and Monday ramps.
5. **By the plan's rule**, the CPU result (3.8% delayed, above 1%) keeps the forecast bound as the
   target a Slurm planner would have to reach in simulation. The GPU result (0.88%) is below the
   1% bar. Both rules are applied across both sites, so nothing is decided until Marconi100 is in.

## Limits

- One site, one year. Lisa was a mixed university cluster with shared nodes; a busier site has
  less idle time.
- No node states: down and idle cannot be told apart, so 72-hour gaps are a proxy.
- The wait is first order: a delayed job delays nothing else. Real queues compound it.
- Whether Lisa already ran Slurm power saving is not known. It does not change idle time, which
  is read from the job table.
- No energy yet. Idle draw by node class comes from the node power metrics in the same Zenodo
  record (CPU and GPU power at 30 s, June to November 2022), labelled class C or D by source.

## Next

1. Idle draw per node class from Lisa's node metrics, to turn node-hours into kWh.
2. Marconi100 (M100 ExaData), the second site, before either decision rule is applied.
