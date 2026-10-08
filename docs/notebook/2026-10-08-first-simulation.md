# 2026-10-08: First simulation

**Simulation, not a measurement** ([ADR-0011](../adr/0011-simulate-before-the-testbed.md)).
Reproduce with `go run ./cmd/wattproof-sim`.

## Question

Before any hardware: on a small testbed, how much energy could Wattproof (T) save against stock
Kubernetes (B0) and a tuned open-source setup (B1)? Is the margin over B1 above the 3% the
experiment is designed to detect?

## Inputs

- Power curves: SPECpower results for a Dell PowerEdge R7725 (2024, 768 threads, 138 W idle,
  861 W full load) and a Dell PowerEdge R7425 (2018, 128 threads, 85 W idle, 287 W full load). The
  sources are in `testdata/`.
- Demand shape: one week of hourly German Wikipedia pageviews (September 2025, CC0), interpolated to
  minutes. Night traffic is about 1/13 of the peak.
- Assumed, not measured: peak requests at 70% of worker capacity; usage at 50% of requests; 10 W
  when off; a boot draws the power of 30% load; shutdown takes 1 minute at idle; at least one worker
  stays on; one control-plane server at idle in every arm. The week runs twice and only the second
  is scored.
- Arms:
  - B0: all nodes on;
  - B1: tight packing and Cluster Autoscaler's default rules, approximated;
  - T: the greedy planner forecasting from yesterday;
  - T*: the planner with a perfect forecast;
  - Tp: T plus MostAllocated scoring.

## Results

| Workers | Fixed | Boot | B0 kWh | B1/B0 | T/B0 | T*/B0 | Tp/B0 | T/B1 | Tp/B1 | Pending min B1 | T | Tp |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 6 new | 0% | 2 min | 348 | 0.675 | 0.685 | 0.685 | 0.681 | 1.015 | 1.010 | 44 | 0 | 0 |
| 6 new | 0% | 5 min | 348 | 0.676 | 0.686 | 0.686 | 0.683 | 1.016 | 1.010 | 110 | 0 | 0 |
| 6 new | 0% | 10 min | 348 | 0.677 | 0.689 | 0.689 | 0.685 | 1.018 | 1.012 | 220 | 0 | 0 |
| 6 new | 50% | 2 min | 431 | 0.814 | 0.825 | 0.825 | 0.822 | 1.013 | 1.010 | 18 | 0 | 0 |
| 6 new | 50% | 5 min | 431 | 0.815 | 0.826 | 0.826 | 0.823 | 1.014 | 1.010 | 45 | 0 | 0 |
| 6 new | 50% | 10 min | 431 | 0.815 | 0.827 | 0.827 | 0.824 | 1.014 | 1.011 | 90 | 0 | 0 |
| 3 new + 3 old | 0% | 2 min | 257 | 0.660 | 0.632 | 0.632 | 0.629 | 0.957 | 0.953 | 34 | 0 | 0 |
| 3 new + 3 old | 0% | 5 min | 257 | 0.661 | 0.633 | 0.633 | 0.630 | 0.958 | 0.954 | 85 | 0 | 0 |
| 3 new + 3 old | 0% | 10 min | 257 | 0.662 | 0.635 | 0.635 | 0.632 | 0.960 | 0.955 | 170 | 0 | 0 |
| 3 new + 3 old | 50% | 2 min | 308 | 0.768 | 0.754 | 0.754 | 0.739 | 0.982 | 0.963 | 18 | 0 | 0 |
| 3 new + 3 old | 50% | 5 min | 308 | 0.768 | 0.755 | 0.755 | 0.740 | 0.983 | 0.963 | 45 | 0 | 0 |
| 3 new + 3 old | 50% | 10 min | 308 | 0.768 | 0.756 | 0.756 | 0.741 | 0.984 | 0.964 | 90 | 0 | 0 |

"Fixed" is the share of peak requests that never scales down (fixed replicas). "Pending min" counts
minutes in the scored week with requests above the capacity of Ready nodes.

## Reading

- **Powering nodes off is the big lever.** Against B0, B1 and T both save 18–37%. That is the
  lever's size on this demand curve, not Wattproof's contribution.
- **On identical servers, T does not beat B1 on energy.** It uses 1.3–1.8% more. It pays for
  headroom and for powering on before demand returns. In exchange, no pods are ever pending, while
  B1 has 18–220 pending minutes a week, growing with boot time. On identical hardware, Wattproof's
  case is service level at nearly the same energy, not lower energy.
- **On mixed hardware, T beats B1 by 1.6–4.3%.** It switches the old servers off first. The margin
  is near or below the 3% the experiment aims to detect.
- **The forecast barely matters here.** T and T* are equal to three decimals. With six large nodes,
  one node is a sixth of capacity, so small forecast errors rarely change a decision. AutoScale
  (2012) reported the same about prediction.
- **Packing adds 0.3–2 points.** That is a scheduler setting the operator can choose. v0.1 does not
  change it (ADR-0005).

## Decision

Phase 1S's rule is triggered: the margin over B1 on identical hardware is below 3%. **Two hardware
generations are a hard requirement for the testbed.** The protocol should also pre-register pending
time as a co-primary endpoint against B1: the simulation says that is where T differs.

## Limits

- The model is fluid: no pod sizes, no fragmentation, no memory.
- B1 is an approximation of Cluster Autoscaler's rules, not the real tool.
- The week is replayed, which flatters forecasting.
- Every assumption above moves the numbers. Only the testbed measures them.
