# 2026-10-08: What Google's services reserve and use (2019 trace, cell a)

**A measurement of a public trace, not of our testbed.** It replaces assumed simulator inputs, as
set out in the [trace analysis plan](../research/trace-analysis-plan.md). No simulator result in
this entry yet: the plan's simulator changes come first.

Data: Google cluster trace 2019, cell a, May 2019 (CC-BY 4.0), and the matching power traces.
Queries and analysis in [`analysis/traces/`](../../analysis/traces/). One scan, 507 GB read,
140 minutes; result 7.3 million rows.

## Checks

| Check | Result | Expected | Outcome |
|---|---|---|---|
| 1. Production CPU usage ÷ limits, month | 0.388 | 0.20–0.40 | Pass |
| 1. Production memory usage ÷ limits, month | 0.546 | 0.50–0.80 | Pass |
| 2. Hourly tier shape against Google's notebook | not yet compared | | Open |
| 3. Usage dropped by the request join | not computable from this output | < 1% | Deviation, see the plan's change log |
| 4. Power-domain CPU total ÷ tier total | 0.9999 | 0.99–1.01 | Pass |

A 1% block sample had given 0.45 for check 1; sampling drops request events, so usage meets stale
requests. The full scan is the measurement.

## Services

13,469 mid- and production-tier collections ran for at least a week. Peak-hour CPU usage over
CPU request, weighted by requested CPU:

| Group | Services | Share of requested CPU | p10 | p25 | Median | p75 | p90 |
|---|---|---|---|---|---|---|---|
| All | 13,446 | 1.00 | 0.09 | 0.34 | 0.68 | 0.87 | 1.16 |
| All, daily cycle | 810 | 0.21 | 0.19 | 0.45 | 0.57 | 0.79 | 1.09 |
| Sized by hand | 3,292 | 0.39 | 0.10 | 0.31 | 0.48 | 0.85 | 1.05 |
| **Sized by hand, daily cycle** | **194** | **0.07** | 0.12 | 0.34 | **0.45** | 0.48 | 0.70 |
| Autopilot | 10,150 | 0.61 | 0.08 | 0.46 | 0.77 | 0.88 | 1.22 |
| Autopilot, daily cycle | 613 | 0.14 | 0.28 | 0.48 | 0.68 | 0.88 | 1.22 |

Ratios above 1 are possible: Borg lets CPU usage exceed the limit when the machine has room.

Other measures:

| Simulator assumption | Assumed | Measured (cell a) |
|---|---|---|
| Daily shape, mean ÷ peak | 0.44 (Wikipedia week) | 0.79 |
| Daily shape, minimum ÷ peak | about 0.2 | 0.58 |
| Share of requests with fixed replicas | 0.5 or 0.8 | 0.45 (aggregate estimator); 0.52 of requested CPU in services whose requests stay within 10% |
| Highest 5 minutes ÷ hourly mean | covered by +20% | median 1.07–1.14 by group |
| Memory | ignored | services' memory requests at peak: 65% of cell memory; CPU requests: 80% of cell CPU |

## Power

Power-domain power (share of the domain's capacity, including data-centre floor cooling) against
CPU usage (share of the machines' CPU capacity), 5-minute points, 8,928 per domain:

| Domain | CPU range | Power range | Power at zero CPU ÷ power at full CPU (fit) | r² |
|---|---|---|---|---|
| pdu6 | 0.35–0.74 | 0.61–0.77 | 0.53 | 0.87 |
| pdu7 | 0.36–0.73 | 0.62–0.76 | 0.61 | 0.69 |
| pdu8 | 0.35–0.73 | 0.61–0.75 | 0.57 | 0.88 |
| pdu9 | 0.38–0.73 | 0.63–0.78 | 0.53 | 0.88 |
| pdu10 | 0.38–0.74 | 0.64–0.79 | 0.51 | 0.90 |

Over the day, CPU usage swings 13–14% of its peak and power 4–6%: batch work fills the night.

## Reading

1. **The decision group is small, and its ratio is low.** Services sized by hand that follow a
   daily cycle hold 7% of requested CPU, with a weighted median of 0.45. That is below the plan's
   0.5 threshold, where the simulator expects no energy gain of Wattproof's rightsizing over VPA.
   The rule is applied formally only after the simulator changes and the second operator.
2. **Google's services follow the day weakly.** The services' total load averages 79% of its peak,
   against 44% in the Wikipedia week we assumed. Only 21% of requested CPU sits in services that
   pass the daily-cycle test. Flatter demand leaves fewer servers to power off at night. Google
   serves the world from each cell; a regional operator's day is likely deeper, which the Alibaba
   trace and our design partners have to show.
3. **Memory binds after CPU rightsizing.** Today CPU requests (80% of capacity at peak) bind before
   memory (65%). Rightsized to peak usage plus 20%, CPU requests would fall to about 46% of
   capacity, below memory. Memory then decides: the servers needed fall by about a fifth, not by
   about two fifths. Without memory in the simulator, rightsizing's saving is overstated.
4. **Idle power is a larger share than our curves assume.** Extrapolated to zero CPU, a power
   domain draws 51–61% of its full-load power, against 16–30% in the SPECpower curves we use.
   Floor cooling and the extrapolation inflate this, but it points one way: each server powered off
   saves more than the simulator credits.
5. **The +20% shaping margin covers typical within-hour peaks** (median 7–14% above the hourly
   mean). Bursty services in the upper tail still need the guard.

## Next

- The simulator changes in the plan: memory as a second limit, per-service shaping and VPA,
  out-of-sample weeks, power curves ±10% (replaced by an idle-share sweep, see the plan's change
  log). Then the trace series as simulator input. Done: [trace simulation](2026-10-08-trace-simulation.md).
- The second operator (Alibaba 2018) and cell g (November quota).
- Check 2 against Google's notebook.
