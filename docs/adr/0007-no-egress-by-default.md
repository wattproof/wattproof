# ADR-0007: Nothing leaves the cluster by default

Status: accepted, 2026-09-26

## Context
A component that can evict pods and power off servers will not pass a security review if it
phones home. Many European operators block outbound traffic from clusters.

## Decision
No telemetry and no external calls by default. The only outbound connections go to configured
meters and BMCs. Optional external features (for example an external intent classifier) are off
by default, and they document exactly what they send. A future opt-in exchange of hardware power
profiles would carry hardware data only, never workload data.

## Consequences
We learn about field installs only from users who tell us. Every product decision must rest on
our own experiments and on design partners.
