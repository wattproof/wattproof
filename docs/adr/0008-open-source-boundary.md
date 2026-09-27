# ADR-0008: Apache-2.0 for everything that makes one cluster better

Status: proposed, 2026-09-26

## Context
Adoption by operators and upstream projects needs a permissive license. A business needs
something to sell that does not cripple the open version.

## Decision
The repository is Apache-2.0: agent, controller, drivers, models, planner, guard, experiment
engine and analysis. A single cluster gets the full saving and its verification for free.
Commercial offerings, if any, sit outside this repository: a multi-cluster fleet view, signed
savings reports for contracts, support, and services.

## Consequences
Anyone may run the product without paying. Revenue has to come from services, fleet features and
trust, not from locking the core.
