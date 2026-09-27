# ADR-0010: Overcommit CPU from forecasts; never overcommit memory

Status: accepted, 2026-09-27

## Context
Many long-running services follow a daily cycle but keep their reservations (requests) through the
night, so the scheduler cannot consolidate them. In-place pod resize (GA since Kubernetes 1.35)
allows requests to follow the cycle without restarting pods. CPU shortage degrades performance and
can be reversed within seconds. Memory shortage kills pods.

## Decision
From v0.2, Wattproof may lower **CPU** requests of opted-in workloads. The lower value is the observed
usage quantile for that hour of the week plus a margin. Requests are raised again before the
forecast ramp. **Memory** requests are never set below the observed peak plus a margin.

On node CPU pressure (PSI), pod throttling or a service-level breach, requests are resized up
first, and nodes are powered on second.

Only Burstable pods are eligible. So is a workload scaled by a HorizontalPodAutoscaler, but only
if the autoscaler targets absolute CPU (`AverageValue`). A `Utilization` target is measured
against the request, so lowering the request would add replicas. Workloads managed by a
VerticalPodAutoscaler are left alone.

Workloads without enough history are excluded, as are bursty and strict tail-latency workloads.

Overcommit is off during events on the operator's calendar.

## Consequences
This lever needs its own registered experiment before it is claimed. Requests belong to
application teams, so the default is to recommend. Wattproof acts only on workloads that opt in by
annotation.
