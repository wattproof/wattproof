# ADR-0003: Learn models of the system; decide with an optimiser, not a learned policy

Status: accepted, 2026-09-26

## Context
A learned end-to-end placement policy (deep reinforcement learning) is hard to explain, hard to
verify and needs a simulator whose fidelity limits it. When the energy function is known, a
one-step decision is simply its minimum.

## Decision
Learning goes into models of power per node, transitions and demand. These are small, inspectable
and continuously refitted. Decisions come from an optimiser, greedy at first, that uses those
models and records its reasons. Learned policies may be added later only for decisions with
delayed consequences, and only after they beat the optimiser in a registered experiment.

## Consequences
The optimiser's quality is bounded by its models. So model error is monitored and published.
