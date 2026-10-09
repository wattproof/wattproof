# Trace analysis plan: what real clusters reserve and use

Status: stated 2026-10-08, before any full query has run. Changes after the first results are
listed at the end with their reason, as in the experiment protocol.

## Purpose

The simulator's results depend on assumed numbers (notebook entries of 2026-10-08). The largest
is how much of its CPU request a service uses at peak: across 20–90%, Wattproof's rightsizing goes
from roughly even with the Vertical Pod Autoscaler to 12% better on mixed hardware. This plan
measures those numbers in public traces, so the simulator's prediction for Experiment 2 rests on
measured inputs (evidence level E2) instead of assumed ones (E1).

## What is measured, and which assumption it replaces

| Simulator assumption (now) | Measure |
|---|---|
| Usage ÷ requests at peak (swept 20–90%) | Per service: CPU usage over CPU request in the service's peak hour. Distribution weighted by requested CPU. |
| Share of requests from fixed-replica services (50% or 80%) | Per service: whether its requests follow the day. Aggregate estimator: F = (R_min/R_max − s_min)/(1 − s_min), with R the hourly requests and s the usage shape normalised to its peak. |
| Daily demand shape (one Wikipedia week) | Hour-of-week profile of service usage, normalised to its peak. |
| Request-shaping margin (+20% over hourly peak) | Highest 5-minute usage in each hour over the hourly mean, per service. |
| Memory is ignored | Memory usage over memory request per service, and whether memory or CPU sets the number of servers needed each hour. |
| Power follows SPECpower curves | Per power domain: power against CPU usage, and the day-to-night swing in power. |

## Definitions, fixed before the data is seen

- **Service:** a collection in the mid or production tier (priority 116 and above) that runs for at
  least 168 hours, so that one full week can train hour-of-week shaping.
- **Alloc sets:** tasks inside an alloc set are excluded; the alloc instance carries the
  reservation and its usage. This follows Google's own analysis notebooks.
- **Request:** the limit in force when each usage window starts. Limits change during a job's life,
  for example under Autopilot.
- **Peak:** the hour with the service's highest mean usage. The cluster-level ratio uses the hour
  with the highest total service usage.
- **Weighting:** distributions are weighted by requested CPU, because requests decide how many
  servers stay on. Unweighted counts are reported next to them.
- **Manual and automated:** services are split by `vertical_scaling`: not autoscaled (the stand-in
  for clusters without automation) and Autopilot (constrained or full).
- **Daily-cycle service:** robust to interrupted days and trends, which defeat plain
  autocorrelation (Kumbhare et al., ATC 2021). For each day, correlate its 24 hourly means with the
  service's median day. The service follows a daily cycle if the correlation is at least 0.8 on at
  least 70% of its days, and its median day-to-night swing ((max − min)/max) is at least 20%. Alibaba
  reports a median swing of about 27% for its seasonal services (Zhang et al., SoCC 2022).
- **Time:** buckets are aligned to Pacific time, as both Google traces are.

## Data

| Source | Role | Cost |
|---|---|---|
| Google 2019 cluster trace, cell a (May 2019, BigQuery) | Primary. Cell a has the largest share of production work of the eight cells. | One scan, 507 GB of the 1 TiB monthly free tier |
| Google 2019 cluster trace, cell g (Singapore) | Replication in another time zone and mix | November quota |
| Google 2019 power traces with the machine-to-power-domain map | Power against CPU, day-to-night swing | Under 1 GB |
| Alibaba 2018 trace, online-service containers (`container_meta` with requests and limits; `container_usage`) | Second operator. One operator's trace is not enough: research tuned to Google's trace often fails on other clusters (Amvrosiadis et al., ATC 2018). The unit of `cpu_util_percent` is checked against the trace's documentation and published analyses before use. | 28 GB download |
| Published figures (below) | Bounds the trace results must sit within, or explain | None |

Published figures:

- Google, all eight 2019 cells: production CPU usage about 30% of limits, memory about 65%, monthly
  means (Tirmazi et al., EuroSys 2020).
- Twitter, a Mesos cluster of mostly user-facing services: CPU use below 20% of capacity while
  reservations reach 80% (Delimitrou and Kozyrakis, ASPLOS 2014).
- Azure: 60% of VMs average under 20% CPU, but many need nearly their whole allocation at the 95th
  percentile (Cortez et al., SOSP 2017). Peaks, not means, decide.
- Datadog 2025 (cloud): most Kubernetes workloads use under 25% of requested CPU.
- CAST AI 2026 (cloud): CPU requests 69% above usage.
- Alibaba: memory, not CPU, limits consolidation; services use over 90% of the memory they request,
  with no daily variation (Zhang et al., SoCC 2022).

## Checks that can fail

1. Cell a's monthly production-tier CPU usage over limits lies between 20% and 40%, and memory
   between 50% and 80% (the paper's eight-cell means are about 30% and 65%). Otherwise the query is
   wrong or cell a is unusual, and that is investigated before anything else.
2. Hourly usage by tier for cell a matches the shape in Google's analysis notebook for the same
   cell.
3. Every service row has a request; rows dropped by the request join are counted and stay below
   1% of usage.
4. The power-domain CPU totals summed over the cell equal the tier totals within 1%.

## Changes to the simulator before the results are used

1. **Memory as a second limit.** Each server has CPU and memory capacity; the servers needed in an
   hour are set by whichever runs out first. Rightsizing changes CPU requests only (ADR-0010), so if
   memory binds, CPU rightsizing frees fewer servers. Test: with memory requests at 100% of
   capacity, rightsizing CPU frees no server.
2. **Per-service shaping and VPA.** Shaped requests and VPA targets are computed per service and
   summed. Shaping the cluster total, as now, is optimistic: the sum of per-service peaks is at least
   the peak of the sum.
3. **Out-of-sample weeks.** The trace has 31 days. Week 1 trains, weeks 2–4 are scored. This
   replaces replaying one week twice, which flatters forecasting.
4. **Power uncertainty.** Predicting a server's power from its SPECpower results has about 9.5%
   mean error (von Kistowski et al., ICPE 2019). Results are reported with the servers' idle power
   varied as a share of peak (see the change log: this replaced ±10% on the whole curve).

## Decision rules, stated before the data

For daily-cycle services sized by hand, the request-weighted peak ratio decides the v0.2 claim:

| Peak usage ÷ requests | Energy claim of rightsizing against B1+VPA |
|---|---|
| Below 0.5 | None expected. The claim is service level (pending pods, contention), capacity freed at peak, and proof. |
| 0.5 to 0.7 | Small, mainly on mixed hardware. |
| Above 0.7 | 3–12% on mixed hardware is plausible. |

If memory binds in most hours at the measured ratios, the energy claim of rightsizing is withdrawn
whatever the CPU ratio, and memory rightsizing (recommend only) becomes the lever to study next.

These rules read simulator output, so they are applied after simulator changes 1–3.

## Limits of this evidence

- Google and Alibaba are hyperscalers: overcommitted, batch fills spare capacity, and Google
  rightsizes much of its fleet automatically. Their services are not our customers' services.
  Results are labelled "trace-calibrated simulation" (E2), never customer evidence.
- Borg's limit is the closest equivalent of a Kubernetes request: the scheduler reserves it, and CPU
  above it is allowed when the machine has room.
- The power traces express power as a share of each power domain's capacity, include data-centre
  floor cooling, and the production share is modelled, not measured.

## Cost and safeguards

- BigQuery sandbox: no billing account, so nothing can be charged. 1 TiB of queries a month, 10 GiB
  of storage over the project's lifetime, tables expire after 60 days.
- Every query first runs as a dry run and then with a hard byte cap (`--maximum_bytes_billed`).
- A 1% sample run (about 5 GB) checks the query before the full scan.
- Results are downloaded and kept locally. Derived tables small enough for the repository go into
  `testdata/` with attribution (the traces are CC-BY 4.0). The queries are in `analysis/traces/`.

## Sources

- [Google cluster data and power traces](https://github.com/google/cluster-data)
- Tirmazi et al., [Borg: the next generation](https://research.google/pubs/borg-the-next-generation/), EuroSys 2020
- Rzadca et al., [Autopilot](https://research.google/pubs/autopilot-workload-autoscaling-at-google-scale/), EuroSys 2020
- Sakalkar et al., Data center power oversubscription with a medium voltage power plane and priority-aware capping, ASPLOS 2020 (the power traces)
- Radovanovic et al., Carbon-aware computing for datacenters, IEEE Transactions on Power Systems 38(2), 2023 (per-power-domain CPU-to-power models)
- Amvrosiadis et al., On the diversity of cluster workloads and its impact on research results, USENIX ATC 2018
- Zhang et al., Workload consolidation in Alibaba clusters: the good, the bad, and the ugly, SoCC 2022
- Delimitrou and Kozyrakis, Quasar, ASPLOS 2014
- Cortez et al., Resource Central, SOSP 2017
- Kumbhare et al., Prediction-based power oversubscription in cloud platforms, USENIX ATC 2021
- von Kistowski et al., Predicting server power consumption from standard rating results, ICPE 2019
- [Datadog State of Containers and Serverless 2025](https://www.datadoghq.com/state-of-containers-and-serverless/)
- [CAST AI 2026 Kubernetes optimization report](https://cast.ai/reports/kubernetes-optimization-report/)
- [Alibaba cluster trace v2018](https://github.com/alibaba/clusterdata/tree/master/cluster-trace-v2018)
- [BigQuery sandbox](https://docs.cloud.google.com/bigquery/docs/sandbox)

## Changes after the first results

- **2026-10-08, check 3.** The full query does not record how much usage the request join drops,
  and measuring it needs another scan of about 240 GB. The check is deferred, not dropped: it runs
  with the cell g query in November, which will record the dropped usage. Until then, results from
  cell a carry this open check.
- **2026-10-08, the 1% sample.** It gave a production CPU ratio of 0.45, outside check 1's band;
  the full scan gave 0.388. Block sampling drops request events, so the sample is used only to test
  that a query runs, never for values.
- **2026-10-08, power uncertainty.** ±10% on the whole curve is replaced by a sweep of the idle
  share (published, 30%, 50% of peak). Scaling a curve changes every arm alike and barely moves the
  ratios; the idle share is what transfers worst between servers, and Google's power traces put it
  at 51–61% of full-load power, far above the published curves' 16–30%.
- **2026-10-08, memory overload.** The simulator now counts minutes in which a node's memory usage
  exceeds its memory, the counterpart of CPU throttling. The pooled model cannot see per-pod memory
  above requests, so the trace's per-pod shortfall is reported next to it.
- **2026-10-08, results of the simulator changes:** [notebook](../notebook/2026-10-08-trace-simulation.md).
- **2026-10-09, decision rules on cell a (provisional, one operator).** The peak ratio for
  services sized by hand with a daily cycle is 0.45: no energy claim of rightsizing against
  B1+VPA, as the simulation confirms. Memory binds once CPU is rightsized, so the energy claim of
  CPU rightsizing is withdrawn, and memory rightsizing is the lever studied next. It is proposed
  for action, not only recommendation ([ADR-0013](../adr/0013-memory-rightsizing.md)), because
  every alternative already acts on memory. The rules are applied again once Alibaba and cell g are
  in.
- **2026-10-08, November work.** Alibaba 2018 runs locally: the BigQuery sandbox's 10 GiB lifetime
  storage cannot hold it, so the 28 GB download is streamed once and aggregated without unpacking.
  Cell g runs on BigQuery with the November quota and records the usage dropped by the request join
  (check 3).
