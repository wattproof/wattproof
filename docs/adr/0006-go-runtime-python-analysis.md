# ADR-0006: Go for everything in the cluster; Python only for offline analysis

Status: accepted, 2026-09-26

## Context
One language inside the cluster means one supply chain, one security review and one set of
operational habits. The Kubernetes controller ecosystem is Go. Reviewers of an energy study
expect analysis they can re-run with pandas and statsmodels.

## Decision
The controller, the agent and the drivers are written in Go (controller-runtime). The models
used at runtime are simple enough to fit in Go (gonum). Published analysis lives in `analysis/`
as Python, pinned and run in CI.

## Consequences
No Python service in any runtime path. If a model ever needs Python, it runs offline and ships
coefficients.
