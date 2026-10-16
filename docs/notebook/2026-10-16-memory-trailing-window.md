# 2026-10-16: ADR-0013's memory rule on Google's services

**Simulation, not a measurement** ([ADR-0011](../adr/0011-simulate-before-the-testbed.md)), on the
same measured inputs as the [trace simulation](2026-10-08-trace-simulation.md): Google cluster
trace 2019, cell a. Evidence level E2, one operator.

The question, from [ADR-0013](../adr/0013-memory-rightsizing.md): the weekly-peak rule tested so
far (T+RM) learned growth too slowly, leaving 1.6–3.2% of memory usage above the request against
VPA's 0.7–1.6%. ADR-0013 proposes the maximum of usage over a trailing 8 days plus 15%, updated
daily, and is accepted only if that rule's shortfall is no worse than VPA's.

Reproduce:

    go run ./cmd/wattproof-traceprep -in cell_a.csv -out testdata/google2019-cella-services.csv
    go run ./cmd/wattproof-sim -only trace

`cell_a.csv` is the per-service result of `analysis/traces/google2019_cell.sql`. Regenerated from
the stored query result, every existing column of the trace file came out identical (0 of 2,976
rows differ); the file gains `mem_trail` and `mem_short_trail`.

## The rule

T+RW: CPU shaped by hour of week as in T+R; memory at the maximum of the trailing 8 days' usage
plus 15%, updated daily, per service (`sim.VPALearned` with the maximum in place of VPA's p90).
Before the first update the template's request holds. A test checks that a spike is learned at the
next daily update, not before, held for 8 days, and then released.

## Results

Memory usage above the request, as a share of memory usage, per service and summed over the four
scored weeks:

| Services | VPA (B1+VPA) | Weekly peak +15% (T+RM) | Trailing 8-day max +15% (T+RW) |
|---|---|---|---|
| All | 1.58% | 3.21% | **0.33%** |
| Sized by hand | 1.24% | 3.04% | **0.26%** |
| Daily cycle | 1.25% | 1.67% | **0.19%** |
| By hand, daily cycle | 0.72% | 1.59% | **0.18%** |

Energy relative to B1+VPA (below 1: Wattproof uses less), boot 5 minutes, idle share as published,
30% and 50%:

| Services | Identical servers | Mixed hardware |
|---|---|---|
| All | 1.018–1.027 (T+RM 1.012–1.023) | 0.900–0.920 (same as T+RM) |
| Sized by hand | 1.073–1.080 (same as T+RM) | 0.982–0.988 (same) |
| Daily cycle | 1.115–1.191 (T+RM 1.020–1.026) | 0.899–0.920 (same) |
| By hand, daily cycle | 1.124–1.235 (T+RM 1.092–1.175) | 1.119–1.300 (same) |

No arm left pods pending or overloaded node memory in any row; B1+VPA left pods pending 5–16
minutes in some groups, as before.

## Reading

1. **The rule passes ADR-0013's condition by a wide margin.** Its shortfall is a quarter or less
   of VPA's in every group, and about a tenth of the weekly-peak rule's. Learning daily from a trailing
   window follows growth; learning once a week from the same hour did not.
2. **On mixed hardware it costs nothing.** Energy is the same as with the weekly peak in every row:
   1–10% less than B1+VPA for the broad groups.
3. **On identical servers it costs energy for services with a daily cycle.** The trailing maximum
   holds each service's highest day for 8 days, where the weekly peak followed the week's shape.
   For the daily-cycle group, T+RW uses 9–17% more than T+RM, and 12–19% more than B1+VPA.
4. **Against VPA, the safer rule trades energy for memory safety.** VPA sets memory at p90 and
   leaves 0.7–1.6% of usage above the request; T+RW sets it at the maximum and leaves 0.2–0.3%.
   The energy claim against VPA stays where it was: mixed hardware only.

## Limits

- Hourly means per service. Memory above the request within an hour does not show; the shortfall
  is a lower bound for every rule alike.
- Pods are pooled per node; per-pod evictions are not modelled, only node memory overload.
- One operator.

## What it means for the design

- ADR-0013's default rule (trailing 8-day maximum plus 15%, daily) meets its acceptance condition.
  ADR-0013 was accepted on 2026-10-16.
- A rule between the two (a high quantile over the trailing window) could recover energy on
  identical servers at some cost in shortfall. Not tested here, to avoid tuning on one trace; it
  can be tested on Alibaba 2018 as a stated variant.
