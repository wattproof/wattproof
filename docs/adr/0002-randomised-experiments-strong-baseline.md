# ADR-0002: Prove savings with pre-registered randomised experiments against a strong baseline

Status: accepted, 2026-09-26

## Context
Before/after comparisons confound the change with load, weather and time. Comparing only
against stock Kubernetes invites the expert's objection that a tuned open-source setup would do
the same.

## Decision
Savings are measured in randomised block experiments (Latin square, crossover when two pools
exist) against two baselines: stock Kubernetes (B0) and a tuned open-source configuration (B1).
The protocol is registered with a timestamp before any treatment block runs. Raw data and
analysis code are published.

## Consequences
Experiments take weeks, not days. A result that does not beat B1 is still published, and its
value then lies in the measurement method and the product's safety, not in the headline.
