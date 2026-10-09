# ADR-0012: Rightsizing is a product pillar, reported in watts

Status: accepted, 2026-10-08

## Context
ADR-0010 introduced CPU rightsizing as a second energy lever for v0.2. Since then:

- In our simulation, request shaping cuts 17–30% of energy beyond power state alone, more than
  the planner's own margin over B1 (notebook entries of 2026-10-08).
- On owned hardware, rightsizing alone saves no energy: it lowers requests, not usage, and the
  freed capacity stays powered. The saving needs a tool that packs and powers servers off. No
  rightsizing tool found does that
  ([feature research](../research/features/rightsizing.md)).
- Power state lowers energy off-peak but not the peak. Rightsizing lowers reservations at peak,
  which frees capacity: fewer servers to buy, more work inside a power budget.
- In-place pod resize is GA (Kubernetes 1.35), so the mechanism is now standard. VPA reaches most
  of the energy effect in simulation. The value is in the combination and the proof.

## Decision
Wattproof has three pillars: **rightsize, power state, verify**. Rightsizing is built in three
steps:

1. **Rightsizing report** (Observe mode, before v0.1). From Prometheus history it recommends CPU
   and memory requests per workload, and runs the simulator to predict servers freed at peak,
   energy saved off-peak, and the change in peak power. It needs no actuator and no meter, and it
   runs on any cluster. Its numbers are labelled as model estimates.
2. **Resize controller** (v0.2, Act mode), as in ADR-0010: CPU only, opt-in, sequenced with
   power state.
3. **HPA coordination** (after v0.2): workloads scaled on CPU `Utilization` get their target
   scaled with the request instead of being excluded.

Memory stays recommend-only. CPU limits are never lowered. (Under review: ADR-0013 proposes
rightsizing memory too, after the trace simulation of 2026-10-08.)

Every experiment that includes rightsizing reports capacity as well as energy: peak requested
cores and peak powered servers.

## Consequences
- The report is the first thing an operator can run, and the basis of the energy assessment.
- The same code that reads usage and requests also measures the usage ratio in public traces,
  which sets Experiment 2's prior.
- We now compete directly with VPA and with commercial rightsizers on owned hardware. The claim
  must always be made against B1+VPA, never against B0 alone.
- ADR-0010 still governs what the controller may change.
