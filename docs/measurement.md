# Measurement

Status: draft v0, 2026-09-26.

A saving is only as credible as the meter behind it. This document defines what is measured, with
which devices, how they are calibrated, and what is published.

## Boundary

- **Measured:** every server of the cluster at its power inlet(s). A server with two power
  supplies is measured at both. Control-plane servers are included.
- **Measured and reported separately:** network switches.
- **Not measured:** facility overhead (cooling, UPS losses). If a facility-level figure is shown,
  it is labelled as an estimate with the PUE it assumes.

## Meter classes

Every published number states its class. Headline claims use class A or B only.

| Class | Devices | Accuracy | Sample rate | Use |
|---|---|---|---|---|
| A: reference | Power analysers on SPEC's PTDaemon accepted list, e.g. Yokogawa WT310E: ±(0.1% of reading + 0.05% of range), 100 kS/s internal sampling, 100 ms updates | about ±0.1–0.2% | 10 Hz | Calibration of class B; spot checks; small headline experiments |
| B: outlet-metered PDU | e.g. Raritan PX3: ±1% billing grade per outlet | ±1% | about 1 Hz | Continuous per-node wall power |
| C: BMC | Redfish, IPMI DCMI | Coarse. One study found 1-byte readings (about 14 W steps) and a 60 s averaging window | 0.1–1 Hz | Field installs with nothing better. Never headline claims |
| D: component estimates | RAPL, Kepler, NVML | Partial: CPU package and DRAM, or GPU board only. On A100/H100, nvidia-smi samples about 25% of runtime | ms | Model features and attribution. Never totals |

## Why ±1% meters are enough, given the right design

A saving is a ratio of two energies measured by the same meters. A constant gain error multiplies
both energies and cancels in the ratio.

What does not cancel:

- non-linearity across the load range;
- drift over time;
- poor resolution at low load;
- aliasing from slow sampling.

The procedure below targets exactly those four. It does not buy a class A analyser for every
outlet.

1. **Linearity.** Calibrate each class B outlet against the class A analyser at five load points
   (off, idle, 33%, 66%, 100%) and publish the curve.
2. **Drift.** Repeat the calibration before and after every campaign. If the two differ by more
   than 0.5% at any point, the campaign's results carry that as added uncertainty.
3. **Aliasing.** Sample at 1 Hz or faster. Prefer the meter's own energy counter to integrating
   power samples.
4. **Crossover.** Each arm of an experiment runs on each set of nodes and at each time of day
   equally, so remaining meter or ambient differences balance out.

## Calibration procedure

1. Put the class A analyser in series with one server's inlet. The PDU outlet stays in the path.
2. At each load point, run steady load for 10 minutes. Discard the first 2 minutes.
3. Compare the energy each device reports over the remaining 8 minutes. Fit gain and offset per
   outlet.
4. Accept an outlet if its residual after correction is below 0.5% at every load point. Otherwise,
   replace or exclude it.
5. Store the raw data, the fit and the analyser's own calibration certificate date with the
   campaign.

## Power profile

Every server is profiled after calibration, at the start and the end of each campaign
([feature note](research/features/power-profiling.md)). Each server gets two profiles: one
measured the way SPECpower_ssj2008 measures, so it replaces a published SPECpower curve like for
like, and a finer one for Wattproof's own models.

### Profile S: the SPECpower method

The SPECpower_ssj2008 benchmark itself, run on every server
([run rules](https://www.spec.org/power/docs/SPECpower_ssj2008-Run_Reporting_Rules.html)):

1. **Calibration:** the ssj workload runs unconstrained for three intervals; the maximum
   throughput is the mean of the last two.
2. **Levels:** 100% to 10% of that maximum in 10% steps, **in descending order**, then active idle
   (ready, no transactions): 11 levels.
3. **Timing:** 240-second measurement intervals, each with 30 seconds of ramp-up and 30 seconds of
   ramp-down, and 10 seconds of zero activity between levels.
4. **Result per level:** average active power and ssj_ops, in the same layout as the published
   results in `testdata/specpower-*.csv`.

Where we differ from a compliant run, and say so with every curve: the meter is the calibrated PDU
outlet (class B) on all servers at once. SPEC's PTDaemon supports only accepted analysers, one
instance per device, and a run measures one system. One server per hardware generation is also run
with the analyser through PTDaemon. SPEC requires its review before any public use of ssj results,
so Profile S curves stay internal unless a run is compliant and reviewed. Profile W is what we
publish.

### Profile W: Wattproof's profile

1. **Load types.** A CPU stressor and a memory stressor (stress-ng), and the experiment's own
   workload (`loadgen`). Each server's maximum throughput is measured per load type first, as in
   Profile S.
2. **Levels.** 100% down to 0% of that maximum in 5% steps, in descending order: 21 levels.
   CPU utilisation is recorded next to each level. The steps are finer than SPEC's because curves
   change shape between 10% steps (a steep rise between 80% and 90% load is typical), and the
   planner works in that range.
3. **Timing.** As in Profile S: 240-second measurement intervals with 30-second ramps.
4. **Off and transitions.** 10 minutes off (BMC standby), then three shutdown-and-boot cycles,
   each timed and metered from the end of the drain until off, and from power-on until the node
   is Ready.
5. **All servers at once,** on their calibrated outlets (class B, corrected). One server per
   hardware generation is profiled a second time with the analyser in series (class A), to check
   the corrected curve.

### Both profiles

- **Duration:** Profile S takes about 1.2 hours, Profile W about 7 hours for three load types plus
  the off state and the cycles. Together they take two nights.
- **Recorded with each profile:** BIOS power profile, firmware, inlet temperature, CPU
  frequency and C-state residency, and the achieved throughput at every level.
- **Published** with the raw samples (Profile W, and Profile S only after SPEC review): one curve
  per server and load type (`load,watts`), off power, and transition times and energies.
- **Comparison:** where a testbed server's model has a published SPECpower result, its Profile S
  curve is compared with it. The difference measures how far a published rating is from the same
  model measured in a rack, the error the rightsizing report carries when it uses published
  curves.

## Time

All meters, nodes and the recorder synchronise with chrony against the same source. The recorder
logs each source's clock offset. Samples from a source whose offset exceeds 100 ms are flagged.
Blocks are minutes long, so millisecond-level sync is not needed. Consistent sync is.

## From samples to energy

- Use energy counters where the device has them. Otherwise, integrate power with the trapezoid
  rule.
- Never interpolate across a gap longer than 5 seconds. Count gaps and report them per block.
- A block with gaps totalling more than 1% of its duration is excluded, and the exclusion is
  reported. This rule is fixed before the experiment starts.

## Covariates recorded

Recorded, not controlled:

- Server inlet temperature (BMC) and room temperature
- Fan speeds
- Supply voltage
- CPU frequency and C-state residency
- BIOS power profile
- Firmware versions

## Published with every report

- Raw samples (Parquet) and calibration data
- Hardware inventory: model, CPUs, memory, PSUs, BIOS settings, firmware
- Software versions and container image digests
- Block schedule with the randomisation seed
- Cluster events: power cycles, evictions, guard trips
- Analysis code that reproduces every number in the report from the raw data
- A DOI for the dataset (Zenodo)

The dataset is released together with its write-up, at the latest eight weeks after the
measurement ends ([publishing rules](publishing.md)).
