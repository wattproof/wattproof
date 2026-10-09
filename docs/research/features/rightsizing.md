# Feature research: rightsizing on owned hardware

Researched 2026-10-08, local library added the same day. Decision: **go, with changes** (section 3).
Updated 2026-10-09 with the trace results: **change**: rightsizing must include memory, and its
simulated energy effect against VPA is small ([update](#update-2026-10-09-what-the-traces-changed)).

## 1. The feature

- **Lever.** Lower the CPU requests of running services to what they use, hour by hour, through
  in-place pod resize. Then pack the freed requests onto fewer servers and power the rest off.
  Report memory requests that could come down, but never act on memory
  ([ADR-0010](../../adr/0010-diurnal-overcommit-cpu-only.md)).
- **Claim, if it works.** On hardware the operator owns, rightsizing turns reserved-but-unused
  capacity into wall energy saved off-peak, and into capacity freed at peak. Both are measured, and
  a holdout keeps verifying them.
- **Strongest alternative.** The Vertical Pod Autoscaler (VPA) in `InPlaceOrRecreate` mode, with
  MostAllocated packing, the descheduler and Cluster Autoscaler (our B1+VPA arm). Commercially:
  ScaleOps, which already runs self-hosted on-premises.

**The key point.** On owned hardware, rightsizing alone saves no energy. It changes requests, not
usage. A server's power follows its usage and whether it is on, so the freed capacity sits idle.
In the cloud the saving appears because the autoscaler deletes virtual machines. On bare metal,
nothing powers the servers off unless a tool does. Our simulator shows this: request shaping with
every node left on (B0's power state) changes energy only through the curve's non-linearity.

## 2. Findings

### Research

- **Autopilot** (Rzadca et al., EuroSys 2020, Google). Vertical and horizontal autoscaling of Borg
  jobs from usage history. Autopiloted jobs had an average relative slack (limit minus usage) of
  23%, against 46% for manually managed jobs. Jobs severely hit by out-of-memory kills fell by a
  factor of 10. Memory recommendations take a high percentile or the maximum of usage, depending
  on the job's tolerance for OOM kills; CPU takes a percentile of load-adjusted usage. Measured in
  production, not energy. **This is the strongest evidence that automated rightsizing halves slack
  without hurting reliability.**
- **Borg: the next generation** (Tirmazi et al., EuroSys 2020), analysing the 2019 Google trace:
  "automatic vertical scaling is effective", and over-commitment grew between 2011 and 2019. The
  trace has both usage and limits per task, which is what our next measurement needs.
- **CPI²** (Zhang et al., EuroSys 2013, Google) and Caladan (OSDI 2020): packing more work on a
  machine raises interference and tail latency. Lower requests mean denser packing, so CPU
  contention is a real risk, not a theoretical one.
- **No paper found that measures the wall-energy effect of rightsizing on Kubernetes.** Recent
  energy-aware autoscaling work (NeuroScaler 2026, EPR-based autoscaling 2026, the CloudTech 2025
  survey) scales replicas or models energy. None combines rightsizing with node power state and
  measures at the wall. This is the gap Experiment 2 fills.

- **Alibaba's tidal scaling** (Zhang et al., SoCC 2022). Alibaba frees the resources of
  latency-critical services along the daily traffic cycle, in production, by switching whole
  instances dormant rather than shrinking requests. Two findings matter for us: **memory, not CPU,
  limits consolidation** (services use over 90% of the memory they request, with no daily
  variation), and tighter packing caused throttling at peak for 73% of shared services until a
  burstable CPU quota fixed it (0.12% after). About 62% of service instances follow a strong daily
  CPU cycle, with a median day-to-night swing of about 27%.
- **Over-reservation elsewhere.** A Twitter cluster of mostly user-facing services used under 20%
  of its CPU while reservations reached 80% (Quasar, ASPLOS 2014). On Azure, 60% of VMs average
  under 20% CPU, but many need nearly their whole allocation at the 95th percentile (Resource
  Central, SOSP 2017): peaks, not means, decide what can be reclaimed.
- **Kubernetes before in-place resize.** RUBAS (IEEE CLOUD 2019) improved on VPA by migrating
  containers, gaining about 10% utilisation, at a cost of 5–20% overhead per application.
- **One operator is not enough.** Research tuned to Google's trace often fails on other clusters
  (Amvrosiadis et al., ATC 2018).

### Industry measurements

- **CAST AI 2026 report.** Tens of thousands of clusters on AWS, Azure and GCP, 2025: CPU
  utilisation 8% (10% the year before), memory 20%; CPU requests 69% above usage (40% the year
  before), memory 79%. "Organizations that apply automated rightsizing reduce their provisioned CPU
  footprint by approximately 50%." Vendor data, cloud only, not independently verified.
- **No measured data for bare-metal clusters found.** Whether owned clusters are as
  over-provisioned is unknown. The public Google and Alibaba traces are the best proxy.

### Newest changes

- **In-place pod resize is GA in Kubernetes 1.35** (December 2025). VPA's `InPlaceOrRecreate` mode
  is beta; a pure `InPlace` mode is still in development. Rightsizing without restarts is now a
  standard feature, so the mechanism is no longer a differentiator.

### Tools

| Tool | Licence | What it does | Bare metal | Power state | Measured savings |
|---|---|---|---|---|---|
| VPA | Apache-2.0 | Recommends and sets requests from a decaying usage histogram (days, not hour of day) | Yes | No | No |
| Robusta KRR | MIT | CLI; recommends requests from Prometheus history | Yes | No | No |
| Crane (gocrane) | Apache-2.0 | Request and replica recommendations, prediction, cost views | Yes | No | No |
| Koordinator | Apache-2.0 | Reclaims requested-but-unused resources for batch work | Yes | No | No |
| ScaleOps | Commercial | Continuous pod rightsizing, replicas, nodes; self-hosted and air-gapped editions | Yes | Not found | Cost, not energy |
| CloudBolt (StormForge) | Commercial | ML rightsizing of requests and HPA utilisation targets | Not checked | No | Cost, not energy |
| CAST AI | Commercial | Rightsizing and instance choice | Cloud-first | No | Cost, not energy |
| Canonical MAAS | AGPL-3.0 | Power control API (IPMI, Redfish) for bare metal; power-off logic must be scripted | Yes | Interface only | No |
| MetalNap | MIT | Powers idle bare-metal nodes off; no rightsizing | Yes | Yes | No |

### Negative results and risks

- Aggressive request cuts caused outages where teams had padded requests for noisy neighbours,
  sidecars and bursts (practitioner reports, not studies). Requests carry knowledge the usage
  history does not show.
- A HorizontalPodAutoscaler with a `Utilization` target measures usage against the request.
  Lowering the request makes it add replicas, which undoes the saving. StormForge adjusts the HPA
  target together with the request for this reason.
- VPA already captures most of the energy effect in our simulation (below).

## 3. Judgement

1. **Is it new?** Rightsizing is prior art (Autopilot, VPA, ScaleOps). Hour-of-week shaping is
   an improvement on VPA's days-long histogram. **New, as far as found: rightsizing coupled to
   node power state, sequenced with it, and verified at the wall.**
2. **Predicted effect.** Slack halves with automation (Autopilot, measured; CAST AI, vendor). In
   our simulation (E1, assumed inputs, usage at 50% of requests), T+R uses 17–30% less energy than
   T and 35–41% less than B0. Against B1+VPA it ranges from −4.6% to +12% depending on the usage
   ratio, positive only when usage is 70–90% of requests at peak. No source measures it at the wall.
3. **Design changes.**
   - Report first: Observe mode gains a **rightsizing report in watts**. It reads requests and
     usage per workload, recommends CPU and memory requests, and runs the simulator to predict
     servers freed at peak, kWh saved off-peak, and the effect on peak power. It works on any
     cluster with Prometheus and needs no actuator.
   - Report capacity, not only energy: peak requested cores and peak powered servers become
     endpoints, because rightsizing lowers the peak while power-off does not.
   - Coordinate with HPA `Utilization` targets (scale the target with the request) instead of
     excluding those workloads. Most HPAs use `Utilization`, so exclusion leaves most of the lever
     unused. Planned after v0.2; v0.2 keeps the exclusion.
   - Memory stays recommend-only. CPU limits are never lowered, so throttling does not get worse
     than before. (Revised 2026-10-09: see the update below and ADR-0013.)
   - Memory can be the binding limit after CPU rightsizing (Alibaba). The report and the
     simulator count servers needed by CPU and by memory, and say which one binds.
4. **Experiment changes.** The inputs come from public traces as set out in the
   [trace analysis plan](../trace-analysis-plan.md). Experiment 2 adds peak requested cores and peak powered servers as
   secondary endpoints. Its effect-size prior is restated once the usage ratio is measured in the
   Google 2019 and Alibaba 2018 traces.
5. **Advantage.** Assessed privately.
6. **Decision: go, with the changes above.** Rightsizing becomes a headline feature, built in
   three steps: the report (Observe), the resize controller (v0.2, Act), HPA coordination (after
   v0.2).

## Update 2026-10-09: what the traces changed

### Measured inputs

Google 2019, cell a ([notebook](../../notebook/2026-10-08-google-trace-cell-a.md)): services sized
by hand with a daily cycle use a request-weighted median of 45% of their CPU request at peak.
Their load is flatter than assumed (mean 79% of peak). Memory requests at the cell's peak are 65%
of its memory against 80% of its CPU, so memory binds once CPU is rightsized.

### Simulated effect (E2, one operator)

From the [trace simulation](../../notebook/2026-10-08-trace-simulation.md):

- CPU-only rightsizing (T+R) uses 4–58% more energy than B1+VPA in every scenario. VPA lowers
  memory requests too.
- With memory set to the learned weekly peak plus 15% (T+RM): 1–10% less than B1+VPA on mixed
  hardware, 1–8% more on identical servers, 9–30% more for the services sized by hand with a daily
  cycle. The plan's decision rule (peak ratio below 0.5) predicted no energy claim there, and holds.
- Ignoring memory had overstated rightsizing by 8–18 points.
- Wattproof leaves no pods pending; B1+VPA leaves them pending 5–16 minutes over four weeks.

### Memory rightsizing among the alternatives

Every rightsizing tool found lowers memory requests, not only CPU:

| Tool | Memory | How it guards against OOM kills |
|---|---|---|
| VPA (`InPlaceOrRecreate`, VPA 1.2+) | Yes, by default | Raises the recommendation after an OOM kill; in-place memory needs `resizePolicy` `NotRequired` |
| Google Autopilot (internal) | Yes | High percentile or maximum of usage, by the job's OOM tolerance; severe OOM kills fell tenfold |
| CAST AI | Yes | Detects OOM kills per workload, raises the limit and pauses recommendations for it |
| ScaleOps | Yes, in place, including on-premises and air-gapped | Reactive: heals after the event |
| StormForge (CloudBolt) | Yes | Proactive OOM prevention, rolls back on health degradation |
| PerfectScale (DoiT) | Yes | Not checked |
| Robusta KRR, Goldilocks | Recommend only | Not applicable |

Kubernetes 1.35 allows memory limits to decrease in place; the kubelet applies a decrease only if
current usage is below the new limit, a best-effort check.

ScaleOps also consolidates nodes on-premises. Whether it powers bare-metal servers off was not
found; it is the competitor closest to our segment and is checked again before the next verdict.

### Revised judgement

1. **Memory rightsizing is needed.** Without it, Wattproof uses more energy than the free baseline
   in simulation. Proposed in [ADR-0013](../../adr/0013-memory-rightsizing.md).
2. **Its energy effect against VPA is small**, and only on mixed hardware, on the one operator
   measured so far (simulation). With memory rightsized, Wattproof left no pods pending where
   B1+VPA left them pending 5–16 minutes.
3. **Experiment 2** gets T+RM as its Wattproof arm if ADR-0013 is accepted, and OOM kills and
   memory-pressure evictions as non-inferiority endpoints. Its prior is restated after Alibaba.
4. **The report (Phase 1R step 4)** still comes first: it shows what any rightsizing would free in
   servers and kWh on owned hardware, which none of the tools above report.

Sources added: [Kubernetes 1.35 in-place resize GA](https://kubernetes.io/blog/2025/12/19/kubernetes-v1-35-in-place-pod-resize-ga/),
[VPA documentation](https://kubernetes.io/docs/concepts/workloads/autoscaling/vertical-pod-autoscale/),
[CAST AI: is automated rightsizing safe?](https://cast.ai/blog/is-automated-rightsizing-safe/),
[ScaleOps](https://scaleops.com/), [CloudBolt: ScaleOps vs StormForge](https://www.cloudbolt.io/blog/scaleops-alternative/).

## Sources

- Zhang et al., Workload consolidation in Alibaba clusters: the good, the bad, and the ugly, SoCC 2022
- Delimitrou and Kozyrakis, Quasar: resource-efficient and QoS-aware cluster management, ASPLOS 2014
- Cortez et al., Resource Central, SOSP 2017
- Rattihalli et al., Exploring potential for non-disruptive vertical auto scaling and resource estimation in Kubernetes (RUBAS), IEEE CLOUD 2019
- Amvrosiadis et al., On the diversity of cluster workloads and its impact on research results, USENIX ATC 2018

- [Autopilot: workload autoscaling at Google scale (EuroSys 2020)](https://research.google/pubs/autopilot-workload-autoscaling-at-google-scale/), checked 2026-10-08
- [Borg: the next generation (EuroSys 2020)](https://research.google/pubs/borg-the-next-generation/)
- [CPI²: CPU performance isolation for shared compute clusters (EuroSys 2013)](https://research.google/pubs/cpi2-cpu-performance-isolation-for-shared-compute-clusters/)
- [Caladan (OSDI 2020)](https://pdos.csail.mit.edu/papers/caladan:osdi20.pdf)
- [CAST AI 2026 Kubernetes optimization report](https://cast.ai/reports/kubernetes-optimization-report/), checked 2026-10-08
- [Kubernetes 1.35: in-place pod resize GA](https://kubernetes.io/blog/2025/12/19/kubernetes-v1-35-in-place-pod-resize-ga/)
- [VPA in-place resize (The New Stack)](https://thenewstack.io/kubernetes-vpa-inplace-resize/)
- [Robusta KRR](https://github.com/robusta-dev/krr), [Crane](https://github.com/gocrane/crane)
- [Koordinator overcommitment (Alibaba Cloud ACK docs)](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/dynamic-resource-overcommitment)
- [ScaleOps self-hosted](https://scaleops.com/product/self-hosted/), [air-gapped](https://scaleops.com/product/air-gapped/), checked 2026-10-08
- [CloudBolt acquires StormForge (The New Stack, 2025)](https://thenewstack.io/cloudbolt-acquires-stormforge-to-enhance-kubernetes-optimization/)
- [Canonical: cut data-centre energy costs with bare-metal automation (2025-06-26)](https://canonical.com/blog/cut-data-center-energy-costs-with-bare-metal-automation)
- [NeuroScaler (2026)](https://www.researchgate.net/publication/400605000_NeuroScaler_Towards_Energy-Optimal_Autoscaling_for_Container-Based_Services)
- [A survey of energy efficiency for sustainable Kubernetes operations (CloudTech 2025)](https://link.springer.com/chapter/10.1007/978-3-032-23844-3_12)
- Our simulation: [request shaping](../../notebook/2026-10-08-request-shaping.md), [sensitivity](../../notebook/2026-10-08-sensitivity.md)
