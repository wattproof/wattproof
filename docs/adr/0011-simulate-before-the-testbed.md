# ADR-0011: Simulate before the testbed; a simulation never makes a claim

Status: accepted, 2026-10-08

## Context
Prior work suggests that a conservative reactive policy, such as B1's Cluster Autoscaler, captures
much of the saving from powering nodes off (AutoScale, 2012; see `docs/research/related-work.md`).
On identical hardware, Wattproof's margin over B1 may be smaller than the 3% the experiment is
designed to detect. Testbed time is scarce and slow to get. The planner also needs a scheduling
simulation of its own.

## Decision
A trace-driven cluster simulator is built before the testbed arrives. It uses published measured
power curves (SPECpower results) and public demand curves, never invented ones. It serves three
purposes:
- experiment design: an effect-size prior for T against B0 and B1, on identical and on mixed
  hardware, recorded before the protocol is registered;
- testing: the invariants and the planner's policies run against it in CI;
- the planner's scheduling simulation, which shares its code.

Simulated numbers are labelled "simulation" wherever they appear. They are never a measured
saving and never a headline.

## Consequences
The simulator is only as good as its curves and its model of the baselines. Its B1 is an
approximation of Cluster Autoscaler's rules, not the real tool. If the simulation shows a margin
over B1 below 3% on identical hardware, Experiment 1 needs two hardware generations, or its
hypothesis H2 says that it tests forecasting and hysteresis only.
