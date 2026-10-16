# Wattproof

Wattproof lowers the electricity a Kubernetes cluster draws at the wall, on hardware whose power
bill you pay, without breaking the service levels of the workloads on it. It proves the saving
with a randomised, pre-registered experiment that anyone can re-run from the published data.

Status: research in progress. There are simulations, analyses of public cluster traces, a
Redfish meter driver and a first report tool. Nothing has been measured on our own hardware yet,
and nothing here is ready to install.

## How it works

1. **Measure.** Wall power per server from metered PDU outlets or a reference power analyser.
   Every number carries the class of meter it came from.
2. **Learn.** Models of each node's power draw, boot time and the cluster's demand. They are
   fitted continuously and promoted only when they beat the model they replace.
3. **Rightsize.** Lower CPU requests to what services use, hour by hour, through in-place pod
   resize. On hardware you own this alone saves nothing: the freed capacity stays powered. So
   Wattproof reports, and later acts on, what it frees in servers and watts.
4. **Decide and act.** Keep the most efficient nodes powered, drain and power off the rest, and
   power them on again before demand needs them. Stock Kubernetes mechanisms only: taints, the
   eviction API, PodDisruptionBudgets.
5. **Prove.** A controlled experiment keeps running, so the saving is measured, not estimated.

## Without Kubernetes

The controller in v0.1 is written for Kubernetes: it moves work off a node through the eviction
API before powering the node off. The rest does not depend on Kubernetes:

- measuring wall power per server, from a reference analyser, metered PDUs or the BMC (Redfish,
  IPMI), with the class of every number stated;
- power profiles per server: what each one draws off, idle and under load, and its break-even
  time for a power cycle;
- the simulator and the experiment protocol, which measure a saving against a randomised
  counterfactual whatever the scheduler.

For Slurm clusters, Slurm already powers idle nodes off with its own power saving. The plan is to
measure first: a report from Slurm's accounting data on idle node-hours and what they draw, and a
randomised measurement of what Slurm's power saving saves at the wall and costs in job wait
([ADR-0015](docs/adr/0015-slurm-observe-and-verify-first.md), proposed;
[feature note](docs/research/features/slurm.md)). Wattproof does not schedule Slurm jobs, and it
will not build a Slurm planner: on two public traces, a perfect demand forecast adds under one
point of energy over Slurm's own idle timer
([notebook](docs/notebook/2026-10-16-slurm-energy-two-sites.md)).

## What we have found so far

All from public data, replayed or simulated: none of it is a measurement of our own. Each entry
in the [lab notebook](docs/notebook/) says how to reproduce it.

- **Slurm, two public traces with measured node power** (SURF Lisa 2022, CINECA Marconi100
  2020). Neither site ran power saving. A 30-minute idle timer would have saved 18–28% of
  GPU-node energy. Even with jobs placed on nodes still powered on first, 0.8–4.2% of job starts
  would wait for a boot. A perfect forecast adds no energy; it could only remove the waits.
- **Kubernetes, Google's 2019 cluster trace.** Against the Cluster Autoscaler with the Vertical
  Pod Autoscaler, Wattproof's planner is predicted to use 1–10% less energy on mixed hardware and
  more on identical servers; it left no pods waiting where the baseline did. Once CPU is
  rightsized, memory decides how many servers stay on.
- **Negative results are kept.** A planned Slurm planner was dropped by a rule written before
  the data; one trace check failed and is recorded as failed; a year of one trace was rejected on
  data quality.

## How this project is made

Wattproof is a one-person research project. I write its code and documents with an AI assistant, which drafts most of the text and code. The questions, the choice of data
and methods, the decision rules and what is kept, changed or dropped are mine. Every decision rule
is written down before the data it applies to is opened, and every later change to it is logged
with its reason. Commits are made under my name after I have reviewed them.

None of this replaces an outside check. Nobody outside the project has reviewed the work yet. The
code and data are public and every number reproduces from them, so please check: corrections go
into the document they change, with a note of what changed.

## Documents

- [Architecture](docs/architecture.md)
- [Measurement](docs/measurement.md)
- [Experiment protocol](docs/experiment-protocol.md) (draft, registered before the first run)
- [Lab notebook](docs/notebook/)
- [Decisions](docs/adr/)
- [Related work](docs/research/related-work.md) and [feature research](docs/research/features/)
- [Roadmap](ROADMAP.md)
- [What we publish, and when](docs/publishing.md)

## License

Apache-2.0 ([ADR-0008](docs/adr/0008-open-source-boundary.md)). Contributions are signed off under the
Developer Certificate of Origin ([CONTRIBUTING.md](CONTRIBUTING.md)).
