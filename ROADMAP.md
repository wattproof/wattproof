# Roadmap

Status: v1.3, 2026-10-09. It replaces v1.2 of the same day, v1.1 and v1 of 2026-10-08 and draft v0 of 2026-09-26. The business track lives separately
and refers to these phases by number.

Wattproof has three pillars: **rightsize, power state, verify**
([ADR-0012](docs/adr/0012-rightsizing-is-a-product-pillar.md)). Rightsizing frees reserved
capacity; power state turns freed servers into saved energy; verification proves the saving.

The goal of v0.1 is one thing: **a published, reproducible result showing whether Wattproof saves
energy on real hardware against stock and tuned-stock Kubernetes.** The only work alongside it is
the rightsizing report (Phase 1R), because it needs no testbed and it is what an operator can run
first.

Every phase ends in tests that can fail. A green suite that cannot go red proves nothing.

## What changed in v1.3

- **The GPU segment is inference serving, not training** ([feature note](docs/research/features/gpu-inference.md)).
  Inference load follows the day; training runs flat with almost no power headroom.
- **The GPU clock lever is SM clock locking, not power caps.** Power caps do not engage in
  memory-bound decode. The evidence for clock locking (15–34% of GPU energy) is larger and better
  measured than for CPU power settings, so the GPU clock lever moves to v0.3 and CPU power
  settings to v0.4 ([ADR-0014](docs/adr/0014-gpu-inference-before-cpu-power-settings.md)).
- **A GPU node is measured at the wall before v0.1 ships** (Phase 1G). Nobody has published what an
  idle or powered-off GPU node draws, so the size of the GPU power-state lever is unknown.

## What changed in v1.2

- **Measured inputs changed the rightsizing picture** ([trace simulation](docs/notebook/2026-10-08-trace-simulation.md),
  Google 2019 cell a, one operator). Once CPU requests are rightsized, memory decides how many
  servers stay on. CPU-only rightsizing loses to B1+VPA on energy in every scenario; with memory
  rightsized too, Wattproof wins only on mixed hardware (1–10%). The v1.1 statement below that
  rightsizing is "the larger lever" no longer holds.
- **Memory rightsizing is proposed** ([ADR-0013](docs/adr/0013-memory-rightsizing.md)). Every
  rightsizing tool found already does it ([feature note](docs/research/features/rightsizing.md#update-2026-10-09-what-the-traces-changed)),
  so Wattproof needs it to match them.
- **The claims that hold on any hardware are service level and proof.** Wattproof left no pods
  pending in any simulated scenario; B1+VPA left them pending for up to 16 minutes.
- **A dated plan of next steps** (below, "Next steps").

## What changed in v1.1

- **Rightsizing is a pillar, not a later lever** ([ADR-0012](docs/adr/0012-rightsizing-is-a-product-pillar.md),
  [feature research](docs/research/features/rightsizing.md)). On owned hardware, rightsizing alone
  saves nothing: the freed capacity stays powered. Coupled to power state it is the larger lever
  in our simulation.
- **New Phase 1R: the rightsizing report**, an Observe-mode report that works on any cluster and
  states its predictions in servers, kWh and peak power.
- **Experiments that include rightsizing report capacity**: peak requested cores and peak powered
  servers, not only energy.
- **Measured inputs before claims:** the [trace analysis plan](docs/research/trace-analysis-plan.md)
  fixes how public traces replace the simulator's assumed inputs, with decision rules stated
  before the data is seen. Memory becomes a second limit in the simulator.
- **Every feature starts with a literature note** in `docs/research/features/`: prior art, the
  newest findings, existing tools, negative results, and the effect they predict.

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

## Next steps (from 2026-10-09)

October, before the BigQuery quota resets:
1. **Decide ADR-0013** (memory rightsizing). Owner: Arman.
2. **Simulate the trailing-window memory rule** on cell a and compare its per-pod shortfall with
   VPA's (0.7–1.6% of memory usage). ADR-0013 is accepted only if it is no worse.
3. **Check 2**: compare cell a's hourly tier shape with Google's analysis notebook, from the data
   already downloaded.
4. **Phase 0 remaining** and the **testbed decision by 25 Oct**.
5. **Phase 1: the Redfish driver** against DMTF's mockup server; it is needed on every route.
6. **`wattproof-report`, first cut** (Phase 1R step 4), reading the trace file before Prometheus.

November:
7. **Alibaba 2018**, locally; **Google cell g** on BigQuery with check 3.
8. **Decision rules across both operators**; restate Experiment 2's prior (E2).
9. **Rerun the product verdict** with both operators' results.
10. **Prototype Fund (30 Nov)**: the lab notebook page carries the simulation results, the
    negative ones included.
11. ~~Decide ADR-0014~~: accepted 2026-10-09.
12. **Phase 1G access**: a GPU node with a wall meter at a GPU operator or research centre, otherwise one
    rented node with BMC readings (class C), stated as such.
13. ~~Simulator inference mode~~ on the Azure 2024 week: done 2026-10-09
    ([notebook](docs/notebook/2026-10-09-gpu-inference-simulation.md)). GPU power state clears the
    5% rule at every idle draw and load time tested; bursty load makes the planner cycle nodes 3–6
    times a day unless it plans on a 30-minute average.
14. **GPU planner: smoothed input and a cycle budget per node per day**, simulated before Phase 1G
    reports.

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
servers T uses 1.5–2.7% more energy than B1 but leaves no pods pending; on mixed hardware it saves
0.4–4.3% against B1. The rule is triggered: **two hardware generations are required.**
Request shaping ([notebook](docs/notebook/2026-10-08-request-shaping.md)) cuts 17–30% against T
alone, but B1 plus the Vertical Pod Autoscaler reaches all but 0.3–2.6% of it. Across usage
ratios from 20% to 90% of requests ([notebook](docs/notebook/2026-10-08-sensitivity.md)),
Wattproof beats that baseline on energy only where reservations are tight (up to 12% on mixed
hardware). Next: measure the real ratio in the Alibaba and Google traces (Phase 1R).

## Phase 1R: Rightsizing report (weeks 2–5)

1. **Inputs from public traces**, following the [trace analysis plan](docs/research/trace-analysis-plan.md),
   which fixes definitions, checks and decision rules before any full query runs. Google 2019
   (cell a, then cell g), its power traces, and Alibaba 2018 as a second operator. Measures: peak
   usage over requests per service, fixed-replica share, daily shape, within-hour peaks, memory,
   and power against CPU. Queries in `analysis/traces/`.
2. **Simulator changes before the results are used:** memory as a second limit (Alibaba reports
   memory, not CPU, limits consolidation); shaping and VPA per service instead of on the cluster
   total; four trace weeks scored out of sample; results rerun with the servers' idle share of
   power varied (this replaced ±10% on the whole curve). Done 2026-10-08.
3. Rerun the sweep at the measured inputs, apply the plan's decision rules, and restate
   Experiment 2's prior (E1 → E2 evidence).
4. **`wattproof-report`.** Reads requests and usage per workload from Prometheus (or a trace
   file). Recommends CPU requests per hour of week and a memory request at the learned peak
   ([ADR-0013](docs/adr/0013-memory-rightsizing.md)); never acts.
   Runs `internal/sim` on the cluster's node inventory and power curves (published SPECpower
   curves until measured ones exist) and reports, labelled as model estimates:
   - cores and memory freed at peak, and servers that would no longer be needed at peak;
   - energy saved off-peak, for power state alone and with rightsizing;
   - workloads excluded, with the reason (Guaranteed QoS, VPA, `Utilization` HPA, short history).
5. Run it on the testbed during the Phase 2 pilots, and offer it to the first operators.

Exit tests:
- On a synthetic cluster with known usage and requests, the recommended requests and the freed
  cores match hand-computed values.
- Excluded workloads are never given a recommendation, and each has its reason.
- The report's predicted energy equals a direct `wattproof-sim` run on the same inputs.
- Run against the Google 2019 trace, it completes and reports the ratio distribution.
- The trace checks in the analysis plan pass, or the deviation is explained in the plan's change
  log before results are used.
- With memory requests at 100% of capacity, CPU rightsizing frees no server in the simulator.

Status 2026-10-08: Google cell a is measured ([notebook](docs/notebook/2026-10-08-google-trace-cell-a.md)).
Hand-sized services with a daily cycle use a weighted median of 45% of their CPU request at peak;
the services' load is flatter than assumed (mean 79% of peak); memory would bind once CPU is
rightsized; idle power is a larger share than the SPECpower curves assume. The simulator changes
are done ([notebook](docs/notebook/2026-10-08-trace-simulation.md)): with memory counted,
CPU-only rightsizing uses more energy than B1+VPA in every scenario; with memory rightsized too, it
wins only on mixed hardware (1–10%); Wattproof leaves no pods pending where B1+VPA does.

November (BigQuery quota resets on 1 November):
- Alibaba 2018, locally: stream the 28 GB download once, aggregate online-service containers into
  the same hourly series, rerun the trace experiment.
- Google cell g on BigQuery (about 500 GB), recording the usage dropped by the request join
  (check 3).
- Then apply the plan's decision rules across both operators and restate Experiment 2's prior.
- A decision record on memory: rightsizing competes on energy only if it includes memory, which
  ADR-0012 rules out today. Drafted as [ADR-0013](docs/adr/0013-memory-rightsizing.md) (proposed).

If T0 arrives before Phase 1R is done, Phase 1 and Phase 2 take priority.

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

## Phase 1G: One GPU node at the wall (before v0.1 ships, as access allows)

Decided in [ADR-0014](docs/adr/0014-gpu-inference-before-cpu-power-settings.md). A measurement,
not an experiment: it gives the GPU levers their inputs before anything is built.

- One GPU inference node, metered at the wall (class B or better; class C only as a stated pilot),
  with NVML logged alongside as class D.
- States: off; bare idle; idle with a CUDA context; model loaded and idle; serving at three loads.
- The full transition: shutdown, boot, model load and warm-up, each timed and metered, for a small
  and a large model, from local NVMe and from network storage.
- Decode and prefill at three locked SM clocks, with TTFT and ITL at each.
- Simulator inference mode: replicas per model from the Azure 2024 week, the measured node curve,
  and model load as part of the transition time. A first run with the power swept instead of
  measured is in the [notebook](docs/notebook/2026-10-09-gpu-inference-simulation.md): it clears
  the 5% rule everywhere, so the measurement now sizes the saving and checks the transitions,
  in this order: boot plus model load, health after repeated power cycles, idle draw.
- In the same access window, if the operator agrees: **Verify study V1**, a vendor GPU power profile
  measured at the wall against default clocks and Wattproof's clock table
  ([protocol](docs/experiment-protocol.md#verify-study-v1-a-gpu-power-profile-at-the-wall-outline)).

The full measurement protocol is in [experiment-protocol.md](docs/experiment-protocol.md#phase-1g-one-gpu-inference-node-at-the-wall-measurement-protocol).

Exit tests:
- The wall energy of each state integrates from the recorded trace with no unflagged gap.
- NVML board power is reported next to wall power for every state, and never in place of it.
- A lab notebook entry states the break-even off-time of a GPU node, including model load.
- If, with the measured idle draw and break-even time, the simulated GPU power state saves less
  than 5% of B0's energy on the Azure week, GPU power state is dropped and only the clock lever
  goes on.

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
- If ADR-0013 is accepted: memory requests at the trailing-window peak plus a margin, updated daily;
  an OOM kill or memory-pressure eviction restores the template request and pauses the workload.
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
  same testbed: B1+VPA, T, T+R (T+RM if ADR-0013 is accepted). Its prior uses the inputs measured
  in Phase 1R, and it reports peak requested cores and peak powered servers next to energy, and OOM
  kills and memory-pressure evictions as non-inferiority endpoints.
- Publish v0.2.0 with its report and dataset at T0 + 14 weeks.

## After v0.2

| Version | Content |
|---|---|
| v0.3 | HPA coordination: workloads scaled on CPU `Utilization` get their target scaled with the request, instead of being excluded (ADR-0012). GPU inference lever (ADR-0014): GPU nodes powered off along the daily cycle with model load in the lead time, then SM clocks locked per serving phase, measured at the wall; NVML only as a feature. Field mode for BMC-only (class C) clusters with stated uncertainty. First design-partner install in Observe mode. The BMC compatibility record and the power-cycle record are published. With funding, a second engineer starts the GPU track in parallel right after v0.1. |
| v0.4 | CPU power settings lever (opt-in privileged agent). Experiment 3, GPU inference: wall kWh per million tokens, TTFT and ITL attainment, minutes waiting for a model to load. |
| v0.5 | Continuous self-tuning: the experiment engine tunes planner parameters. Production holdout reporting. |

Powering whole GPU nodes off is v0.1 on GPU nodes, with two additions: the lead time includes
model load and warm-up, and a node takes work only after a GPU health check. Where a batch queue
can use the freed GPUs, Wattproof reports the capacity and does not power them off. Training
clusters are out of scope ([feature note](docs/research/features/gpu-inference.md)).

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
