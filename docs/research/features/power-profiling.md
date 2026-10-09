# Feature research: power profile per server

Researched 2026-10-09. Decision: **go, with changes** (section 3).

## 1. The feature

- **Lever.** None directly. It is measurement: a complete wall-power profile for every server of
  the testbed, measured before the experiments. The profile is the input to everything that
  predicts energy: the simulator's prior for each registered experiment, the planner's ranking of
  nodes by efficiency, and the break-even time of every power cycle.
- **Claim, if it works.** Every prediction Wattproof publishes rests on curves measured on the
  same servers, at the wall, with their accuracy stated. No prediction rests on a vendor rating or
  a curve from another machine.
- **Strongest alternative.** SPEC's SERT 2 suite and SPECpower_ssj2008, which measure power at
  defined load levels with an accepted analyser; Kepler's model server, which trains power models
  from stress-ng runs on bare metal. Today the simulator uses published SPECpower results.

## 2. Findings

### Research

- **Load type changes power at the same load level** (von Kistowski, Block, Lange, Arnold, ICPE
  2015). Seven SERT CPU worklets on the same server, measured with an accepted analyser through
  PTDaemon, at 10% load steps. At 10% load the worklets drew 118.3–126 W. At 40% load the largest
  drew 243.5 W and the smallest 186.4 W, 57.1 W apart. At 100% load, 431.4 W against 343.8 W,
  87.6 W apart. Which worklet draws most changes with the load level. Curves are linear or slightly
  concave at low load, then rise steeply between 80% and 90%. Measured at 2% steps, the curves
  show features that 10% steps miss. **A curve measured with one load type does not describe
  another, even for CPU-only loads.**
- **Identical servers have different curves** (Davis, Rivoire, Goldszmidt, Ardestani, IEEE
  Computer Architecture Letters 2011). Four clusters of five identical nodes each. A power model
  trained on one node and multiplied by the node count had worst-case errors up to 150% of the
  dynamic range. Training on several nodes brought the worst case below 20% (embedded cluster)
  and 10% (the others); quadratic models had median errors of 5.7% or less. Their meters were
  ±1.5%. **Every server needs its own curve, not one per model.**
- **Predicting from standard ratings carries error.** Predicting a server's power for an
  application from its SPECpower results has about 9.5% mean error (von Kistowski et al., ICPE
  2019; already in [related work](../related-work.md)).
- **Manufacturing variation** among processors of the same model is reported at about 10% in
  power for identical work (see the sources in Davis et al. 2011); not re-measured here.

### Methods and tools

| Tool | What it does | Licence | Use for us |
|---|---|---|---|
| SPEC SERT 2 | CPU, memory and storage worklets at defined load levels (CPU worklets at 25–100%, SSJ at 12.5% steps), with an accepted analyser; one efficiency score | SPEC licence, not open source | The method to follow: load as a fraction of the server's own maximum throughput, several worklets, steady intervals. Not the tool: it cannot be redistributed with our open data and code. |
| SPECpower_ssj2008 | One Java workload at 11 levels (active idle, 10%–100%); still active, PTDaemon updated February 2026. Public use of results requires SPEC's review | SPEC licence | The format of our current simulator inputs (`testdata/specpower-*.csv`). Run by its rules as Profile S |
| SPEC PTDaemon | Network interface to accepted analysers, including the Yokogawa WT310/WT310E | Distributed only inside SPEC's licensed suites | Not usable. We need our own analyser driver. |
| stress-ng | Hundreds of stressors (CPU methods, memory, cache, I/O); CPU load can be set as a percentage | GPL-2.0 | The load generator for the synthetic load types |
| Kepler and its model server | Trains power models from stress-ng runs on bare metal; the 2026 re-architecture reads hardware meters and checks against BMC readings | Apache-2.0 | Prior art for training from stress runs. Not wall power at class A or B. |

### Negative results

None found for measuring per-server curves. The negative findings concern the alternative: one
curve per model, or a curve taken from a rating, does not predict a given server's power well
(both studies above).

## 3. Judgement

1. **Is it new?** No. It is standard practice in energy-efficiency benchmarking. What is ours:
   profiles of every testbed server, published with the raw data, in the simulator's input format,
   and produced by open software.
2. **Effect.** No energy effect. It removes the hardware part of the simulator's prediction error.
   The evidence above suggests that error is not small: about 9.5% from ratings alone, and node-to-
   node differences large enough to matter for choosing which servers to keep on.
3. **Design changes.**
   - Profile **every server**, not one per hardware generation.
   - **Two profiles per server** (updated 2026-10-09):
     - **Profile S** runs the licensed SPECpower_ssj2008 benchmark itself, by its run rules (three
       calibration intervals, 100% to 10% in descending 10% steps, then active idle; 240-second
       intervals with 30-second ramps). It replaces a published SPECpower curve like for like.
       SPEC requires review before public use of ssj results, so these curves stay internal
       unless a run is compliant and reviewed.
     - **Profile W** is ours and published: 21 levels in 5% steps, descending, with the same
       interval timing, for three load types (a CPU stressor, a memory stressor, and the
       experiment's own workload under `loadgen`). Finer steps because curves change shape
       between 10% steps.
   - Define load as a **fraction of the server's own maximum throughput per load type**, as SPEC
     does, and record CPU utilisation next to it.
   - Where a testbed server's model has a published SPECpower result, compare it with that
     server's Profile S: the difference is the error of using a published rating, which the
     rightsizing report does where nothing is measured.
   - **Off-state power and the transitions** (shutdown, boot to Ready) are part of the profile:
     they fill `internal/power.Node` (`OffWatts`, `Transition`) for the break-even time.
   - **The analyser has one channel.** Profiling every server in series with it would take days.
     So: calibrate each PDU outlet against the analyser first (five points), then profile all
     servers at once on their calibrated outlets. One server per generation is profiled a second
     time with the analyser in series, to check the calibrated curve against class A.
   - Profile at the start and the end of the campaign; the difference is drift, reported.
4. **Experiment changes.** The registered prior for each experiment is computed from the measured
   profiles, not from SPECpower. The A/A pilot starts after profiling. New exit tests (ROADMAP,
   Phase 2).
5. **Advantage:** assessed privately.
6. **Go, with the changes above.** It needs software (ROADMAP, Phase 1): a load runner that holds
   a target fraction of maximum throughput, an analyser driver, and `wattproof-profile`, which
   runs the schedule, power-cycles the nodes and writes the curves.

## Sources

- [von Kistowski et al., Analysis of the influences on server power consumption and energy
  efficiency for CPU-intensive workloads, ICPE 2015](https://research.spec.org/icpe_proceedings/2015/icpe/p223.pdf)
- [Davis et al., Including variability in large-scale cluster power models, IEEE CAL 2011](https://www.microsoft.com/en-us/research/wp-content/uploads/2011/11/CAL_2011.pdf)
- [SPEC SERT 2 metric](https://ssg.spec.org/sert2/SERT-metric.pdf)
- [SPECpower_ssj2008 run and reporting rules](https://www.spec.org/power/docs/SPECpower_ssj2008-Run_Reporting_Rules.html)
- [SPECpower_ssj2008 FAQ](https://spec.org/power/docs/specpower_ssj2008-faq)
- [SPEC PTDaemon device list](https://www.spec.org/power/docs/specpower-device_list/accepted_power_analyzers/)
- [Kepler re-architected (CNCF, 2026-06-30)](https://www.cncf.io/blog/2026/06/30/kepler-re-architected-improved-power-accuracy-and-a-community-call-to-action/)
- [Kepler model server](https://github.com/sustainable-computing-io/kepler-model-server)
