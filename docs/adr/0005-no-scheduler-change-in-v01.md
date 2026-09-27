# ADR-0005: v0.1 does not replace or reconfigure the scheduler

Status: accepted, 2026-09-26

## Context
Replacing kube-scheduler, or asking pods to opt into a second scheduler, is the most intrusive
change a customer can be asked for. It is also the smaller lever.

## Decision
Consolidation works by controlling which nodes are schedulable: NoSchedule taints on nodes that
are draining or off, plus eviction through the eviction API. The stock scheduler places pods
within the powered set. An energy-aware Score plugin is optional and later. When it comes, it
reads models from informer caches and makes no network calls while scheduling.

## Consequences
Within the powered set, pods spread according to the customer's existing scheduler settings. The
planner makes up for this by choosing which nodes to drain.
