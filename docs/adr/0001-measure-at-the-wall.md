# ADR-0001: Measure at the wall, and label every number with its meter class

Status: accepted, 2026-09-26

## Context
Energy software often reports estimates (RAPL, Kepler, model-based watts) as if they were
measurements. Buyers who have seen such claims fail to show up on their electricity bill distrust
all of them.

## Decision
The quantity we optimise and report is energy at the server power inlet. Meters are classed A
(reference analyser) to D (component estimates) as defined in `docs/measurement.md`. Headline
claims use class A or B only. Class D values are model features, never totals.

## Consequences
The lab testbed needs outlet-level metering and one reference analyser, so there is a hardware
cost. In the field, the product must work with class C (BMC) data and state its lower accuracy
openly.
