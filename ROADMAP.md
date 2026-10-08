# Roadmap

Status: v1, 2026-10-08. It replaces draft v0 of 2026-09-26. The business track lives separately
and refers to these phases by number.

The goal of v0.1 is one thing: **a published, reproducible result showing whether Wattproof saves
energy on real hardware against stock and tuned-stock Kubernetes.** Everything else waits.

Every phase ends in tests that can fail. A green suite that cannot go red proves nothing.

## What changed in v1

- **A simulator comes first** (Phase 1S, [ADR-0011](docs/adr/0011-simulate-before-the-testbed.md)).
  Prior work suggests that B1's reactive rules capture much of the saving
  ([related work](docs/research/related-work.md#what-research-says)). The margin over B1 is
  estimated before weeks of hardware time are spent.
- **Dates after Phase 1 count from T0**, the day a metered testbed is handed over. The testbed is
  the critical path, and its date is not ours to set.
- **Meter drivers follow the testbed.** Grid'5000 serves its wattmeters through its own API
  (Kwollect), not SNMP or SCPI. Only Redfish is needed on every route.
- **The experiment protocol is public** as a draft ([experiment-protocol.md](docs/experiment-protocol.md)).

## Assumptions

- 30 focused hours a week. Funding work (grant applications) is capped at about 15% of them.
- Target T0: week 6 (2 Nov 2026). Every week T0 slips moves every later phase by a week.
- With T0 in week 6, v0.1 publishes in week 16 (week of 11 Jan 2027) and v0.2 in week 20 (week of
  8 Feb 2027).
- Nothing is published between 20 December and 6 January: nobody reads it then.

Fixed external date: the Prototype Fund deadline on **30 Nov 2026**. By then the design, the
protocol draft and a lab notebook page with the simulation results are public.

## Phase 0: Foundations (weeks 1–3, from 28 Sep 2026)

Done:
- Name, GitHub organisation `wattproof`, public repository `wattproof/wattproof` with the design.
- Go module, CI (gofmt, vet, tests with the race detector), Apache-2.0 license file.
- `internal/energy` (energy from power or counters, gap rules) and `internal/meter` (driver
  interface, meter classes).

Remaining:
- CI: staticcheck now; a dependency license check once there are dependencies.
- `go 1.25`, which controller-runtime v0.23 and client-go v0.35 require.
- Testbed track: send the requests to Grid'5000, a research computing centre and hosting
  providers. The requirements are in the protocol's Testbed section and in
  [measurement.md](docs/measurement.md). **Decide by 25 Oct (week 4).**
- If no class B route is confirmed by then: start on rented bare metal with BMC readings
  (class C). That run is published as a pilot with its accuracy stated. It is not a headline claim
  ([ADR-0001](docs/adr/0001-measure-at-the-wall.md)), and the registered experiment waits for
  class B meters.

Exit tests:
- CI fails on a deliberately broken test, and passes once it is removed.
- The testbed decision is recorded by 25 Oct.

## Phase 1S: Simulator and expected effect (weeks 2–4)

- `internal/power`: a node's wall power as a curve over load, from measured load points; power
  when off; the transition model (boot time, boot and shutdown energy); break-even time.
- `internal/sim`: a one-minute, discrete-time cluster simulation over requested CPU and memory.
  Three arms:
  - B0: all nodes powered;
  - B1: Cluster Autoscaler's default rules (utilisation threshold 0.5, 10 minutes unneeded), tight
    packing, reactive power-on when pods are pending;
  - T: the greedy planner (most efficient nodes first, forecast headroom, break-even hysteresis).
- Published inputs only: SPECpower results from two hardware generations, and one public diurnal
  demand curve. `loadgen` will later follow the same curve.
- `wattproof-sim` prints a scenario table: identical against mixed hardware, boot times of 2, 5
  and 10 minutes.
- A lab notebook entry with the predicted T/B0 and T/B1, labelled "simulation".

Exit tests:
- The architecture's break-even example (5-minute boot at 200 W, 120 W idle, 10 W off) gives
  218 seconds off, 8.6 minutes for the whole cycle.
- B0's energy equals the integral of its power curves. Under constant full demand, every arm
  uses exactly B0's energy.
- No arm ever has fewer powered nodes than the minimum.
- T never powers a node off when the forecast says it would be needed again within its
  break-even time.
- If the simulated margin of T over B1 is below 3% on identical hardware, two hardware
  generations become a hard requirement for the testbed.

Status 2026-10-08: the simulator runs, and every exit test above passes and fails when the code is
broken. First result ([notebook](docs/notebook/2026-10-08-first-simulation.md)): on identical
servers T uses 1.3–1.8% more energy than B1 but leaves no pods pending; on mixed hardware it saves
1.6–4.3% against B1. The rule is triggered: **two hardware generations are required.**

## Phase 1: Measurement (weeks 4–6)

- Drivers for the `meter.Driver` interface, in this order:
  - **Redfish**, developed against DMTF's Redfish mockup server. It reads `EnvironmentMetrics`
    and `PowerSubsystem` first, and the deprecated `Power` resource only as a fallback. It also
    powers nodes on and off (`ComputerSystem.Reset`). It is needed on every testbed route, as a
    class C meter and as the power driver. It records vendor, model, firmware and every quirk it
    meets.
  - **The chosen testbed's meter**, and only that one: the Kwollect API for Grid'5000's
    wattmeters, SNMP for an outlet-metered PDU (tested against recorded SNMP data), or SCPI for a
    reference analyser (tested against a fake instrument).
- Recorder writing Prometheus metrics and immutable Parquet files. Gap detection.
- `wattproof-calibrate`: runs the five-point calibration and writes the fit and report.

Exit tests:
- A synthetic power trace with known energy integrates exactly (to float tolerance). Done.
- A 6-second gap is flagged and never interpolated. Done.
- Each driver passes its contract test against its mock.
- On hardware, once available: every PDU outlet calibrates against the analyser with residual
  below 0.5% at all five points, or is excluded.

## Phase 2: Testbed, workloads, baselines, noise floor (T0 to T0 + 3 weeks)

Before T0, on kind:
- Workload tools:
  - `loadgen`: an open-loop HTTP load generator that follows a request curve;
  - a batch replayer for the Alibaba 2018 trace, scaled to the testbed;
  - SLI collection (latency percentiles, pending time, job completion).
- Test workload service mix: some services with HorizontalPodAutoscalers targeting absolute CPU,
  some with fixed replicas. The mix is identical for Experiments 1 and 2.
- Baselines B0 and B1 as code. B1 uses the same power driver as Wattproof.
- Spike, 2–3 days: drive upstream Cluster Autoscaler through its external gRPC provider, so that
  B1 is the official tool with default settings rather than our own script. If it does not work,
  fall back to a published rule with its default thresholds.
- Public lab notebook page ([ADR-0009](docs/adr/0009-public-live-lab.md)) with the design, the
  protocol draft and the simulation results. Live data joins at T0.

From T0:
- Install Kubernetes on the testbed. Record the method, versions and BIOS settings as code.
- The recorder pushes aggregates to object storage; the page shows power, node states, the
  running arm, load and latency.
- A/A pilot: B0 against B0 over enough blocks to estimate block-to-block noise. The pilot also
  measures carryover: how long one arm still shows after the next one starts.

Exit tests:
- B1 completes 50 automated power cycles with no stuck node.
- The live page shows data less than 5 minutes old, and the testbed accepts no inbound connection
  from the internet (checked with an external port scan).
- The pilot yields the minimum detectable effect. If it is above 3%, the block design or testbed
  changes before Phase 4.
- The pilot yields the carryover length, and the warm-up covers it.

## Phase 3: Engine v0.1 (week 6 to T0 + 6 weeks)

- CRDs: `EnergyPolicy`, `PowerMeter`, `NodePowerModel`, `PowerPlan`, `ServiceLevel`,
  `Experiment`.
- Models:
  - node power (per node, with a hardware-class prior);
  - transitions (boot time, shutdown and boot energy);
  - demand forecast (quantiles, hour-of-week).
- Planner with break-even hysteresis. Its scheduling simulation is `internal/sim`.
- Actuators: taints, drain through the eviction API, power driver.
- Guard with built-in and `ServiceLevel` indicators. Kill switch.
- Observe, Recommend and Act modes. Helm chart. Least-privilege RBAC per component.
- Observe mode runs on the testbed during the Phase 2 pilots, so models train on real data early.

Exit tests:
- Invariants 1 to 8 in [architecture.md](docs/architecture.md) each have a unit test and an
  end-to-end test on kind with a fake power driver. Invariant 9 belongs to Phase 3R.
- The testbed completes 200 automated Wattproof power cycles with no stuck node.
- 72 hours in Act mode under replayed load with no guard trip and no stuck node.
- Model accuracy is reported against class B measurements. This is a number, not a pass/fail.

## Phase 4: Registered experiment (T0 + 6 to T0 + 9 weeks)

- Run the simulator on the testbed's measured curves and record its prediction in the protocol.
- Freeze [experiment-protocol.md](docs/experiment-protocol.md) and register it on OSF before the
  first T block runs.
- Run the blocks. Monitor meter gaps daily, holidays included: the blocks run unattended.
- The analysis in `analysis/` runs in CI from the raw data.
- If Umwelt-Campus Birkenfeld agrees, they replicate a subset independently.

Exit tests:
- CI reproduces every number in the results from the raw data.
- Results are final, whatever they show.

## Phase 5: Publish (T0 + 9 to T0 + 10 weeks)

- v0.1.0 release: signed images, SBOM, Helm chart.
- Report as web page and PDF: method, results with confidence intervals, every block, deviations
  from protocol, and the simulated prediction next to the measured result.
- Dataset on Zenodo with a DOI.
- Paper: an arXiv preprint (cs.DC). First-time submitters need an endorsement from an active
  arXiv author. Then submit to a peer-reviewed venue such as ACM e-Energy, ACM HotCarbon, ICPE or
  IEEE CLOUD (check deadlines). The paper is published whatever the result: a pre-registered null
  result is still a finding. It reports what was actually done on the testbed, and it follows the
  venue's policy on disclosing tool use in writing.
- Website live. Blog post. Talk submissions (check the dates: FOSDEM 2027, KubeCon EU 2027,
  Kubernetes Community Days in Germany).

Exit tests:
- Someone who has never seen the project installs v0.1.0 on kind from the README in under
  15 minutes.
- Someone else reproduces the report's numbers from the Zenodo data.

## Phase 3R: Resize controller (T0 + 4 to T0 + 9 weeks, in parallel)

Built on kind while Experiment 1 runs unattended on the testbed. The testbed is busy, but the
engineer is not. See the architecture's "Resize controller" section and
[ADR-0010](docs/adr/0010-diurnal-overcommit-cpu-only.md).

- Eligibility checks: opt-in annotation, Burstable QoS, no VPA, HPA targeting `AverageValue` only.
  Each rejection is reported with its reason.
- Hour-of-week usage quantiles per workload; target, margin and floor; warm-up for new pods.
- Evening and morning sequencing with the planner: resize down before draining, power on before
  resizing up, evict deferred pods.
- Guard signals: node CPU pressure (PSI), throttling, stuck `PodResizePending`.
- Observe mode first: report what would be resized, and the cores and nodes it would free.
- The simulator gains request shaping, so Experiment 2 also gets a stated prediction.

Exit tests:
- Unit tests prove that Guaranteed pods, VPA-managed workloads and Utilization-target HPAs are
  never resized.
- End-to-end test on kind:
  - A full compressed day completes with no resize left pending past the timeout.
  - The morning ramp powers on before resizing up.
  - The kill switch restores every template request (invariant 9).
- Load test: a forced usage spike at night is corrected by resizing up within 30 seconds.

## Phase 6: Experiment 2, diurnal rightsizing (T0 + 10 to T0 + 13 weeks)

- Register Experiment 2 ([experiment-protocol.md](docs/experiment-protocol.md)) and run it on the
  same testbed: B1+VPA, T, T+R.
- Publish v0.2.0 with its report and dataset at T0 + 14 weeks.

## After v0.2

| Version | Content |
|---|---|
| v0.3 | CPU power settings lever (opt-in privileged agent). Field mode for BMC-only (class C) clusters with stated uncertainty. First design-partner install in Observe mode. The BMC compatibility record and the power-cycle record are published. |
| v0.4 | GPU clock lever, measured at the wall on rented GPU nodes. NVML only as a feature. With funding, a second engineer starts this track in parallel right after v0.1. |
| v0.5 | Continuous self-tuning: the experiment engine tunes planner parameters. Production holdout reporting. |

Powering whole GPU nodes off needs no new lever. It is v0.1 on GPU nodes, and it can be tested as
soon as a GPU operator offers nodes.

## Funding-dependent

- **Wattproof Verify, June to November 2027**, if the Prototype Fund accepts it. The measurement
  and experiment engine is extracted into a standalone, neutral toolkit that can verify any
  energy-saving measure in a Kubernetes cluster, not only Wattproof's. It adds:
  - field meter drivers with explicit uncertainty;
  - an adapter interface for switching arbitrary interventions;
  - a report generator with reproducibility bundles;
  - an open data format;
  - a public registry of verified results.

## Backlog, not scheduled

- Jev evaluation, 2 days: render pod templates from about 200 popular Helm charts on Artifact
  Hub, label them by hand, and compare Jev with rules on accuracy and calibration. Keep it only
  if it clearly wins.
- Optimal oracle in `analysis/`: a MIP (HiGHS) that finds the minimum-energy set of powered nodes
  at each step, to bound how far the greedy planner is from optimal. Rebalancer was considered and
  is too heavy at this scale.
- Energy-aware Score plugin for mixed hardware.
- Opt-in exchange of hardware power profiles (hardware data only, ADR-0007).
- Suspend-to-RAM support, if the testbed hardware allows it.
