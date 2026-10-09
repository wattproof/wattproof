# 2026-10-08: The simulator on Google's services, with memory

**Simulation, not a measurement** ([ADR-0011](../adr/0011-simulate-before-the-testbed.md)), but its
inputs are now measured: Google cluster trace 2019, cell a ([previous entry](2026-10-08-google-trace-cell-a.md)).
Evidence level E2 in the trace analysis plan's terms. One operator only; the second (Alibaba) is
still to come.

Reproduce:

    go run ./cmd/wattproof-traceprep -in cell_a.csv -out testdata/google2019-cella-services.csv
    go run ./cmd/wattproof-sim -only trace

## What changed in the simulator

The [trace analysis plan](../research/trace-analysis-plan.md) required four changes before results
are used:

1. **Memory is a second limit.** Pods ask for memory in proportion to their CPU, so a node holds
   at most min(CPUs, memory ÷ that ratio). Placement, pending pods, the power-off invariants, B1's
   utilisation rule and the planner all use it. Without memory, every earlier result is unchanged
   (checked against the request-shaping entry).
2. **Rightsizing and VPA per service**, then summed: 13,469 services, each with its own learned
   requests.
3. **Out of sample.** Rightsizing learns each hour of the week from earlier weeks only; VPA from the
   trailing 8 days, updated daily. Week 1 trains; weeks 2–5 are scored. A test proves that a
   later value never changes an earlier request.
4. **Idle power varied** as a share of peak: published (16% and 30%), 30% and 50%. This replaces the
   plan's ±10% on the whole curve: scaling a curve changes every arm alike and barely moves the
   ratios, while the idle share is what transfers worst between servers (see the plan's change
   log).

New arm: **T+RM**, rightsizing CPU as T+R does and setting memory to the learned weekly peak plus
15%. ADR-0012 keeps memory recommend-only, so T+RM is what memory rightsizing would add, not a
planned feature. B1+VPA rightsizes both CPU and memory, as VPA does by default.

## Results

Boot 5 minutes. Energy relative to B0; T+R and T+RM relative to B1+VPA (below 1: Wattproof uses
less). "No memory" is T+R/T with memory ignored, as the simulator was before.

| Services | Workers | Idle | B1/B0 | B1+VPA/B0 | T/B0 | T+R/B0 | T+RM/B0 | T+R/(B1+VPA) | T+RM/(B1+VPA) | T+R/T | No memory |
|---|---|---|---|---|---|---|---|---|---|---|---|
| All | 6 new | published | 0.860 | 0.748 | 0.911 | 0.844 | 0.766 | 1.128 | 1.023 | 0.927 | 0.836 |
| All | 6 new | 50% | 0.778 | 0.615 | 0.858 | 0.748 | 0.622 | 1.216 | 1.012 | 0.872 | 0.719 |
| All | mixed | published | 0.828 | 0.759 | 0.805 | 0.791 | 0.699 | 1.042 | **0.920** | 0.982 | 0.868 |
| All | mixed | 50% | 0.773 | 0.682 | 0.797 | 0.773 | 0.614 | 1.133 | **0.900** | 0.969 | 0.770 |
| By hand | mixed | published | 0.844 | 0.695 | 0.797 | 0.778 | 0.685 | 1.120 | **0.985** | 0.976 | 0.848 |
| Daily cycle | mixed | published | 0.805 | 0.760 | 0.807 | 0.758 | 0.699 | 0.997 | **0.920** | 0.940 | 0.866 |
| By hand, daily cycle | 6 new | published | 0.864 | 0.629 | 0.906 | 0.813 | 0.687 | 1.293 | 1.092 | 0.897 | 0.713 |
| By hand, daily cycle | mixed | published | 0.866 | 0.589 | 0.785 | 0.659 | 0.659 | 1.119 | 1.119 | 0.839 | 0.761 |

All 24 rows: `go run ./cmd/wattproof-sim -only trace`.

Service level over the four scored weeks: B1+VPA left pods pending for 5–16 minutes in some
groups; every Wattproof arm for none. No arm overloaded CPU or node memory.

## Reading

1. **CPU-only rightsizing loses to VPA once memory counts.** T+R uses 4–58% more energy than B1+VPA
   in every row. VPA also shrinks memory requests; once CPU requests are small, memory decides how
   many servers stay on, and T+R leaves memory where it was.
2. **Ignoring memory overstated rightsizing by 8–18 points.** T+R/T is 0.84–0.98 with memory and
   0.70–0.87 without. Every earlier rightsizing number in this notebook is too optimistic.
3. **With memory rightsized too (T+RM), Wattproof beats VPA only on mixed hardware**, by 1–10%, for
   the broad groups. On identical servers it is 1–8% worse; for the decision group (sized by hand,
   daily cycle) 9–30% worse.
4. **The plan's decision rule holds.** For services sized by hand with a daily cycle (peak ratio
   0.45), there is no energy claim of rightsizing against B1+VPA.
5. **Power state still pays against stock Kubernetes.** B1/B0 0.73–0.87 and T/B0 0.79–0.91; on
   mixed hardware T beats B1 in most rows, on identical servers B1 wins. A higher idle share makes
   every arm save more, and favours B1+VPA a little more than T+R.
6. **Wattproof's consistent edge is service level:** no pending pods in any row, against up to 16
   minutes for B1+VPA.

## Limits

- One operator. Google's services follow the day weakly (load averages 79% of its peak), and
  Google rightsizes much of its fleet already.
- The model pools each node's pods. Per-pod memory above the request does not show here; the trace
  shows it is real: 0.7–1.6% of memory usage sits above VPA's memory request, and 1.6–3.2% above
  T+RM's weekly peak plus 15%, which learns growth too slowly.
- Worker memory equals CPUs, so both generations have the same balance; the cell's balance between
  CPU and memory requests is kept.
- T schedules with the stock spreading within its powered set (ADR-0005); B1 packs.

## What it means for the design

- **Rightsizing must include memory to compete on energy**, which ADR-0012 rules out today
  (ADR-0010 allows memory down to the observed peak plus a margin). A memory rule that learns from
  a trailing window and updates daily, like VPA's but at the peak, would be safer and tighter than
  T+RM's weekly peak. Proposed in [ADR-0013](../adr/0013-memory-rightsizing.md).
- The energy claim narrows to **power state on mixed hardware**; the claim on any hardware is
  **service level and proof**.
