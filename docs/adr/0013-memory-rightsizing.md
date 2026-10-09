# ADR-0013: Rightsize memory too, at the learned peak

Status: proposed, 2026-10-09. Amends ADR-0012 ("memory stays recommend-only"). Consistent with
ADR-0010, which already allows memory requests down to the observed peak plus a margin.

## Context
The trace-calibrated simulation ([notebook](../notebook/2026-10-08-trace-simulation.md), Google
2019 cell a) shows that once CPU requests are rightsized, memory decides how many servers stay on.
CPU-only rightsizing (T+R) used 4–58% more energy than B1+VPA in all 24 scenarios, because VPA
lowers memory requests as well. With memory set to the learned weekly peak plus 15% (T+RM),
Wattproof used 1–10% less than B1+VPA on mixed hardware, and 1–8% more on identical servers.

Every rightsizing tool found already lowers memory requests: VPA by default, Google's Autopilot,
CAST AI, ScaleOps (including on-premises), StormForge and PerfectScale
([feature note](../research/features/rightsizing.md#update-2026-10-09-what-the-traces-changed)).
Leaving memory out makes Wattproof use more energy than the free baseline in simulation.

Kubernetes 1.35 allows memory limits to decrease in place. The kubelet applies a decrease only if
current usage is below the new limit, a best-effort check that can race with a spike.

## Decision
From v0.2, for opted-in workloads only (ADR-0010's eligibility rules unchanged):

- The memory request is set to a high quantile or the maximum of usage over a trailing window
  (default: the maximum over 8 days), plus a margin (default 15%), updated daily. It never follows
  the hour of day: memory shows little daily variation (Alibaba, SoCC 2022) and a shortage kills.
- Memory limits are never lowered by Wattproof. Only requests change, so a pod above its request
  is not killed by its own limit; the risk is node memory pressure and eviction.
- An OOM kill or a memory-pressure eviction of a resized workload restores its template request
  and suspends memory rightsizing for that workload until an operator re-enables it.
- The report (Observe mode) shows memory recommendations first, with the usage above them in the
  history, as the trace measured it (0.7–3.2% of memory usage above the learned request).

## Consequences
- The simulator's T+RM arm becomes the planned v0.2 arm; Experiment 2 adds OOM kills and
  memory-pressure evictions as non-inferiority endpoints.
- The rule must beat the weekly peak tested so far, which learned growth too slowly (1.6–3.2% of
  usage above the request against VPA's 0.7–1.6%). The trailing-window rule is simulated before
  it is accepted.
- Wattproof's energy claim against VPA narrows to mixed hardware. The claims that hold on any
  hardware are service level (no pending pods) and proof at the wall.
