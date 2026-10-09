# 2026-10-08: How much the conclusions depend on the usage assumption

> **Correction (2026-10-08, later):** this entry ignores memory. With memory as a second limit
> and inputs from the Google trace, CPU-only rightsizing uses more energy than B1+VPA in every
> scenario, and its gain over T was overstated by 8–18 points. See the
> [trace simulation](2026-10-08-trace-simulation.md).

**Simulation, not a measurement** ([ADR-0011](../adr/0011-simulate-before-the-testbed.md)).
Reproduce with `go run ./cmd/wattproof-sim` (the "SENSITIVITY" table).

## Question

The [request-shaping entry](2026-10-08-request-shaping.md) assumed that, at the daily peak,
services use 50% of the CPU they request. That number is a guess. Which conclusions survive if it
is 20%, 35%, 70% or 90%?

At the peak, a ratio of 50% means half of each reservation is idle. Over the whole week, the
demand curve averages 44% of its peak, so the weekly mean ratio is 0.44 × the peak ratio: 9% at
the low end of the sweep, 40% at the high end.

## Results

Boot 5 minutes. "Fixed" is the share of peak requests from services with a fixed number of
replicas; the rest autoscale.

T+R's saving against B1+VPA, in percent (negative: T+R uses more):

| Workers | Fixed | 20% | 35% | 50% | 70% | 90% |
|---|---|---|---|---|---|---|
| 6 new | 50% | −1.9 | −0.1 | 0.5 | −0.1 | 0.6 |
| 6 new | 80% | −0.4 | 2.4 | 1.3 | 3.4 | 2.6 |
| 3 new + 3 old | 50% | −2.2 | −2.8 | 2.6 | 5.5 | 6.6 |
| 3 new + 3 old | 80% | −0.4 | −4.6 | 0.3 | 12.0 | 9.0 |

At every ratio, in both hardware sets:

| | Range across the sweep |
|---|---|
| B1/B0: power state, tuned open source | 0.72–0.85 |
| B1+VPA/B0: plus constant rightsizing | 0.47–0.77 |
| T/B1, identical servers | 1.023–1.094 (T uses 2–9% more) |
| T/B1, mixed servers | 0.985–1.002 (within 1.5%) |
| Pending minutes a week, B1+VPA | 0–85 |
| Pending minutes a week, T and T+R | 0 in all 40 runs |
| CPU contention minutes a week, B1+VPA | 0 up to a 50% ratio; up to 246 at 70–90% |
| CPU contention minutes a week, T+R | 0 in all 20 runs |

CPU contention means some node's usage exceeded its capacity. It happens when VPA's constant
request sits below peak usage and B1 packs nodes full by requests.

## What holds whatever the ratio

1. **Powering nodes off is a real lever.** Tuned open source alone saves 15–28% against stock
   Kubernetes.
2. **Shrinking requests matters more than a clever power plan.** Adding VPA takes B1 from
   0.72–0.85 down to 0.47–0.77 of B0.
3. **v0.1 alone does not beat tuned open source on energy.** On identical servers it uses 2–9%
   more: it keeps headroom, powers on early and leaves pods spread. On mixed servers it is within
   1.5%.
4. **Wattproof never leaves pods pending** and never causes CPU contention, in any run. The
   baselines do.

## What depends on the ratio

5. **Whether v0.2 beats open source on energy.**
   - Where reservations are loose (peak usage at 35% of requests or less), VPA's single cut
     already captures everything. T+R is within ±5%, often slightly worse.
   - Where reservations are tight (70–90%), VPA cannot cut much and keeps its request through the
     night. T+R wins by up to 3.4% on identical servers and 5.5–12% on mixed servers, while VPA causes
     contention at the peak.
   - 50% is in between.

**Which end real clusters sit at is the open question.** CAST AI's 2026 report puts requests on
average 69% above usage: a mean ratio of about 0.59. That is beyond the tight end of this sweep
(0.40). But it comes from cloud clusters, and nobody has published it for bare metal.

## Strategic conclusions

1. **Do not claim that v0.1 saves more energy than open source.** It does not, at any ratio.
   v0.1's case:
   - it makes the lever work on bare metal, where Cluster Autoscaler has no power driver;
   - pods never wait;
   - every saving is proven.
2. **Make v0.2 the product's energy claim, for one kind of cluster.** Request shaping plus power
   state wins where reservations are already tight and hardware is mixed. Elsewhere, recommend VPA
   honestly.
3. **Qualify every prospect on three measurable questions.** Observe mode answers them in its
   first week:
   - Does a saved kWh reach their bill?
   - What is their peak usage as a share of requests?
   - Do they run mixed hardware generations?
4. **Measure the ratio before choosing the experiment's workload.** Public traces record both
   requests and usage per container: Alibaba 2018, Google 2019. Set Experiment 2's ratio from
   them and pre-register it. Choosing it to suit T would bias the result.
5. **Revisit spreading (ADR-0005).** On identical servers, part of v0.1's deficit is that the
   stock scheduler spreads pods. Recommend mode could advise MostAllocated, which the operator
   would set. Open question, not decided.

## Limits

Everything in the earlier entries' limits still applies: a fluid model, B1 and VPA approximated,
one replayed week. This sweep varies one assumption. Peak load at 70% of capacity, boot power and
standby power stay fixed.
