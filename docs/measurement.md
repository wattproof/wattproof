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
