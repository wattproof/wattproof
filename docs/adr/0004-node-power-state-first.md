# ADR-0004: Node power state is the first lever

Status: accepted, 2026-09-26. The order of the later levers is amended by ADR-0014 (2026-10-09): GPU clocks in v0.3, CPU power settings in v0.4.

## Context
A powered, idle server draws a substantial share of its peak power. A powered-off server draws a
few watts. Pod placement among powered nodes changes energy by a few percent at best, and only on
mixed hardware.

## Decision
v0.1 consolidates load onto the most efficient nodes, powers off the rest and powers them on ahead
of demand. It respects break-even time, PodDisruptionBudgets and headroom. Diurnal CPU
rightsizing follows in v0.2, built alongside v0.1. CPU power settings follow in v0.3, GPU clocks
in v0.4.

## Consequences
Power control through BMCs (Redfish or IPMI) is required. Boot latency becomes a service-level
risk, which the forecast headroom and the guard handle.
