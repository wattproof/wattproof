# Experiment protocol

Status: DRAFT, public since 2026-10-08. It will be frozen and registered (OSF, timestamped) before
the first Wattproof block runs. After registration, every deviation is reported in the results.
Comments on the draft are welcome until then.

## Question

For the same offered load, does Wattproof reduce the cluster's wall energy compared with:

- **B0**, stock Kubernetes; and
- **B1**, a competent open-source configuration,

without making service levels worse?

Beating B0 is the headline. Beating B1 is the claim an expert will check, so both are
pre-registered.

## Arms

| Arm | Configuration |
|---|---|
| B0 | Stock kube-scheduler (`LeastAllocated`). All nodes powered all the time. |
| B1 | What a competent platform team can assemble from standard open-source parts today. kube-scheduler with `MostAllocated` scoring packs pods tightly. Descheduler `HighNodeUtilization` runs every 5 minutes and moves pods off lightly used nodes so they empty. Upstream Cluster Autoscaler with default scale-down settings powers empty nodes off and on, connected to the same power driver as Wattproof through its external gRPC provider. If that route proves infeasible in Phase 2, a published rule with Cluster Autoscaler's default thresholds replaces it, and this protocol says so before registration. |
| T | Stock kube-scheduler; Wattproof in Act mode with the parameters frozen at registration. |

## Testbed

Named here once chosen ([ROADMAP](../ROADMAP.md), Phase 0). The minimum is one control plane plus
four or more workers. Every worker is metered at class B or better, and the control plane is
metered too.

Workers come from **two hardware generations**. Choosing the most efficient nodes is one of the
few places where Wattproof can beat B1, and identical hardware would hide it
([related work](research/related-work.md#what-research-says)). The first simulation
([notebook](notebook/2026-10-08-first-simulation.md)) predicts no energy margin over B1 on
identical servers, so two generations are required.

## Workload

Identical in every block, generated from a fixed seed.

- **Latency-sensitive:** a microservice benchmark (Online Boutique or DeathStarBench
  socialNetwork) under open-loop load that follows a real diurnal request curve. Open loop means
  requests arrive on schedule whether or not earlier ones finished, so slowdowns show up in
  latency instead of hiding as reduced load.
- **Batch:** jobs replayed from a public cluster trace (Alibaba 2018 batch tasks), scaled to the
  testbed.
- The service mix is the same in Experiments 1 and 2, declared at registration, and results are
  reported per class, because both kinds are common in production:
  - services with a HorizontalPodAutoscaler targeting absolute CPU (`AverageValue`), so replica
    counts follow the daily curve;
  - services with a fixed replica count, which no arm can consolidate at night without
    resizing.
- The daily curve is compressed only as far as node boot times allow. The compression factor is
  fixed after the pilot and stated.

## Public live data

Raw power, node states and the running arm are shown live on the project website during the
experiment. The number of blocks and the analysis are fixed at registration, and the experiment
does not stop early, so watching cannot change the result.

## Blocks and randomisation

- Block length: long enough to hold a full compressed load cycle plus transitions. Starting value:
  4 hours, fixed after the pilot.
- The first 15 minutes of every block are warm-up and excluded. This handles carryover: an arm
  that powered nodes off leaves them off when the next block starts. The pilot measures how long
  the previous arm still shows in power and node states, and the warm-up is set to cover it, as
  in the switchback design literature (Bojinov, Simchi-Levi and Zhao, 2023).
- Every block starts from the same state: all workers powered and Ready.
- Arms run in randomised order within each set of three blocks (a Latin square). The seed is
  published.
- With two identical node pools, arms also swap pools between sets (crossover).

## Endpoints

- **Primary:** wall energy per block at the measurement boundary (class A or B).
- **Secondary:**
  - energy per served request and per completed batch CPU-second
  - p99 request latency and minutes in SLO violation
  - p99 pod pending time
  - batch completion time
  - power cycles per node per day

## Hypotheses

- H1: energy(T) < energy(B0).
- H2: energy(T) < energy(B1).
- Non-inferiority, against both B0 and B1:
  - p99 latency(T) is at most the baseline's p99 plus a margin fixed at registration;
  - SLO violation minutes(T) are at most the baseline's plus a margin fixed at registration.

## Expected effect, stated in advance

Before registration, the cluster simulator ([ADR-0011](adr/0011-simulate-before-the-testbed.md))
runs the frozen arms on the testbed's hardware curves and the frozen workload. Its predicted
ratios T/B0 and T/B1 are recorded in the registration as a prior. They are a prediction to be
tested, not a result.

## Sample size

An A/A pilot (B0 against B0) measures the block-to-block noise. From it we compute the number of
blocks that detects a 3% difference with power 0.8 at α = 0.05. If the pilot shows the testbed
cannot detect 3% in a reasonable number of blocks, the testbed or the block design changes before
any T block runs.

## Analysis

- Paired differences within each Latin-square set; the ratio of mean energies with a bootstrap
  confidence interval.
- An exact randomisation test as well: the p-value comes from re-running the analysis under every
  arm order the randomisation could have produced. It needs no distributional assumption, which
  matters with a small number of blocks. If the two disagree, both are reported.
- Every block is reported. Exclusions happen only for pre-defined reasons (meter gaps above 1% of
  the block, a hardware fault), and each one is listed.
- One script produces every number in the report from the raw data. CI runs it.

## Experiment 2: diurnal CPU rightsizing (outline)

It runs on the same testbed, with the same workload, directly after Experiment 1, and it is
registered separately before its first treatment block.

| Arm | Configuration |
|---|---|
| B1+VPA | B1, plus the upstream VerticalPodAutoscaler in `InPlaceOrRecreate` mode with default settings. This is the open-source way to rightsize today: it follows a usage percentile over days, not the hour of day. |
| T | Wattproof as in Experiment 1: power state only. |
| T+R | Wattproof with the resize controller on for all eligible services. |

- **Primary:** wall energy per block. T+R against T isolates what resizing adds. T+R against
  B1+VPA is the expert comparison.
- **Secondary and non-inferiority:** everything from Experiment 1, plus CPU throttling ratio, node
  CPU pressure (PSI), resize latency, and the count of deferred or infeasible resizes.
- **Memory (if [ADR-0013](adr/0013-memory-rightsizing.md) is accepted):** the Wattproof arm also
  rightsizes memory (T+RM), because B1+VPA does. In the trace-calibrated simulation, CPU-only T+R
  used more energy than B1+VPA in every scenario. OOM kills and memory-pressure evictions become
  non-inferiority endpoints.
- **Capacity (secondary):** peak requested cores and peak powered servers per block. Power state
  lowers energy off-peak but not the peak; rightsizing can lower both
  ([ADR-0012](adr/0012-rightsizing-is-a-product-pillar.md)).

## Phase 1G: one GPU inference node at the wall (measurement protocol)

A measurement, not an experiment: it gives the GPU levers ([ADR-0014](adr/0014-gpu-inference-before-cpu-power-settings.md))
their inputs. Nobody has published the wall power of an idle or powered-off GPU node
([feature note](research/features/gpu-inference.md)).

- **Node:** one GPU inference node as the operator runs it. Hardware, firmware, driver, CUDA and
  serving-stack versions are recorded.
- **Meters:** wall power at class B or better at every inlet. Class C (BMC) is allowed only for a
  pilot, labelled as such. NVML board power is logged alongside, per GPU, as class D, so the gap
  between board and wall is part of the result.
- **States, 10 minutes each, three repeats:**
  1. off (BMC standby);
  2. bare idle (driver loaded, no process);
  3. idle with a CUDA context on every GPU;
  4. model loaded, no requests;
  5. serving at 25%, 50% and 90% of the node's measured maximum throughput, from a replay of the
     Azure 2024 conversation trace.
- **Transitions, five repeats each:** shutdown; boot to the node Ready; model load and warm-up to
  the first served token, for a small and a large model, from local NVMe and from network storage.
  Each is timed and metered. Boot plus model load is the input the simulated service level depends
  on most ([notebook](notebook/2026-10-09-gpu-inference-simulation.md)).
- **Repeated cycles:** at least 20 power cycles, each followed by the GPU health check, with every
  failure recorded. The simulated planner cycles a node about once a day.
- **Clocks:** decode and prefill at default clocks and at three locked SM clocks, with TTFT, ITL
  and wall energy per output token at each.
- **Derived and published:** the node's power curve over load; idle draw against off; the
  break-even off-time including model load; energy per output token at each clock.
- **Decision rule, fixed now:** if, with these inputs, the simulated GPU power state on the Azure
  2024 week saves less than 5% of B0's energy, GPU power state is dropped and only the clock lever
  continues.

## Verify study V1: a GPU power profile at the wall (outline)

The first study of Wattproof Verify: measuring a vendor's energy claim at the wall, independently.
NVIDIA reports up to 15% energy saved at above 97% performance for its power profiles on B200,
measured on the GPU (arXiv 2510.03872). Board savings are expected to shrink at the wall, because
the GPUs are about half of a GPU server's power.

| Arm | Configuration |
|---|---|
| D | Default clocks and power limits. |
| P | NVIDIA's Max-Q inference profile, where WPPS is installed. On nodes without WPPS, the documented knobs set by hand (SM clock lock, power limit), stated as such. |
| W | Wattproof's clock table: SM clocks locked per serving phase. |

- **Workload:** one model served by vLLM, replaying the Azure 2024 conversation trace at three
  fixed loads. The same requests in the same order in every block.
- **Blocks:** 30 minutes, the first 5 excluded, arms in randomised order within each set of three
  blocks; an A/A pilot (D against D) sets the number of sets.
- **Primary endpoint:** wall energy per output token.
- **Secondary:** TTFT and ITL at p50, p95 and p99; throughput; NVML board energy per token, to
  report the board-to-wall ratio.
- **Hypotheses:** P and W use less wall energy per token than D, with p99 ITL at most D's plus a
  margin fixed at registration. The wall saving is stated next to the board saving.
- **Neutrality:** the vendor's claim is quoted as published and tested as stated. The result is
  published whatever it shows, with the raw data.

Runs on the Phase 1G node in the same access window, if the operator agrees.

## Experiment 3: GPU inference (outline)

Registered after Phase 1G and the simulation, on a multi-node GPU testbed.

| Arm | Configuration |
|---|---|
| B0 | All GPU nodes powered, default clocks, the operator's replica autoscaler. |
| B1 | The same autoscaler, with empty GPU nodes powered off reactively (Cluster Autoscaler's default thresholds). |
| T | Wattproof power state: forecast power-on with model load in the lead time, backfill rule, health check. |
| T+C | T plus SM clock locking per serving phase. |

- **Primary:** wall kWh per million output tokens.
- **Secondary and non-inferiority:** TTFT and ITL attainment at p95 and p99; minutes of requests
  waiting for a model to load; power cycles and failed health checks per node.
