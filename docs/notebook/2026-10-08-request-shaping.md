# 2026-10-08: Simulating request shaping (v0.2)

> **Correction (2026-10-08, later):** this entry ignores memory. With memory as a second limit
> and inputs from the Google trace, CPU-only rightsizing uses more energy than B1+VPA in every
> scenario, and its gain over T was overstated by 8–18 points. See the
> [trace simulation](2026-10-08-trace-simulation.md).

**Simulation, not a measurement** ([ADR-0011](../adr/0011-simulate-before-the-testbed.md)).
Reproduce with `go run ./cmd/wattproof-sim`.

## Question

v0.2 lowers the CPU requests of fixed-replica services at night (ADR-0010), so more nodes can be
off. How much does that add? And how does it compare with what open source already offers, the
Vertical Pod Autoscaler (Experiment 2's baseline, B1+VPA)?

## Change to the model

The first simulation treated fixed-replica services as using a constant amount of CPU. That was
wrong: their *requests* stay fixed, but their *usage* follows traffic. The simulator now carries
usage and requests as separate series. That changes the "fixed 50%" rows of
[the first entry](2026-10-08-first-simulation.md); the corrected Experiment 1 table is below.

## Arms

- B1+VPA: B1, plus VPA on fixed-replica services. VPA sets one constant request: the 90th
  percentile of usage over the past week, plus 15% (its defaults).
- T+R: T, plus the resize controller. Each hour, requests follow that hour's peak usage plus
  20%. They never go below 10% of the template's request, and never above it.
- Autoscaled services are left alone in every arm.

## Results, Experiment 2 (boot 5 minutes)

| Workers | Fixed | B1/B0 | B1+VPA/B0 | T/B0 | T+R/B0 | T+R/T | T+R/(B1+VPA) | Pending min B1+VPA · T+R |
|---|---|---|---|---|---|---|---|---|
| 6 new | 50% | 0.783 | 0.657 | 0.804 | 0.654 | 0.813 | 0.995 | 50 · 0 |
| 6 new | 80% | 0.821 | 0.629 | 0.887 | 0.621 | 0.700 | 0.987 | 35 · 0 |
| 3 new + 3 old | 50% | 0.739 | 0.629 | 0.735 | 0.613 | 0.834 | 0.974 | 85 · 0 |
| 3 new + 3 old | 80% | 0.788 | 0.588 | 0.782 | 0.587 | 0.750 | 0.997 | 70 · 0 |

No arm throttled pods (usage never exceeded a node's capacity).

## Results, Experiment 1, corrected (boot 5 minutes)

| Workers | Fixed | B1/B0 | T/B0 | T/B1 | Pending min B1 · T |
|---|---|---|---|---|---|
| 6 new | 0% | 0.676 | 0.686 | 1.016 | 110 · 0 |
| 6 new | 50% | 0.783 | 0.804 | 1.027 | 45 · 0 |
| 3 new + 3 old | 0% | 0.661 | 0.633 | 0.958 | 85 · 0 |
| 3 new + 3 old | 50% | 0.739 | 0.735 | 0.995 | 45 · 0 |

## Reading

- **Shrinking requests is a large lever.** Against T alone, T+R uses 17–30% less energy. With
  power state, it brings the cluster to 59–65% of B0.
- **VPA gets almost all of it.** B1+VPA is within 0.3–2.6% of T+R. In this model, what matters
  is that requests come down from the template at all. Following the hour of day adds little:
  at night VPA's constant request is already small enough to empty all but one or two nodes.
- **Wattproof's energy edge over the best open-source setup stays small**, 0.3–2.6% here and
  −2.7% to +4.3% in Experiment 1. Its consistent differences are elsewhere:
  - no pending pods in any scenario, against 35–110 minutes a week for the baselines;
  - the efficiency ranking on mixed hardware;
  - the verification.
- **The product case shifts.** On bare metal, nothing assembles B1+VPA out of the box: Cluster
  Autoscaler has no power driver for physical servers, and the descheduler and VPA must be tuned
  by hand. The simulation says the value lies in making the lever **safe, available on bare
  metal, and proven**, not in a cleverer optimiser.

## Limits

- Usage at 50% of requests at peak is an assumption. Lower ratios widen every saving.
- Real VPA uses decaying histograms per container, with bounds; this is its steady state only.
- One week replayed, a fluid model, B1 approximated: as in the first entry.

## Decision proposed

- Pitch v0.2 as "rightsizing that follows the day, safely, on bare metal", not as a saving over
  VPA.
- Keep Experiment 2's comparison against B1+VPA. It is the honest one, and the simulation
  predicts a small margin; publish it as such.
- Give more weight to Wattproof Verify, the neutral measurement toolkit. Its value does not
  depend on beating open source.
