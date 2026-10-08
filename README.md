# Wattproof

Wattproof lowers the electricity a Kubernetes cluster draws at the wall, on hardware whose power
bill you pay, without breaking the service levels of the workloads on it. It proves the saving
with a randomised, pre-registered experiment that anyone can re-run from the published data.

Status: design. Nothing here is ready to install.

## How it works

1. **Measure.** Wall power per server from metered PDU outlets or a reference power analyser.
   Every number carries the class of meter it came from.
2. **Learn.** Models of each node's power draw, boot time and the cluster's demand. They are
   fitted continuously and promoted only when they beat the model they replace.
3. **Decide and act.** Keep the most efficient nodes powered, drain and power off the rest, and
   power them on again before demand needs them. Stock Kubernetes mechanisms only: taints, the
   eviction API, PodDisruptionBudgets.
4. **Prove.** A controlled experiment keeps running, so the saving is measured, not estimated.

## Documents

- [Architecture](docs/architecture.md)
- [Measurement](docs/measurement.md)
- [Experiment protocol](docs/experiment-protocol.md) (draft, registered before the first run)
- [Decisions](docs/adr/)
- [Related work](docs/research/related-work.md)
- [Roadmap](ROADMAP.md)

## License

Apache-2.0 (planned, see [ADR-0008](docs/adr/0008-open-source-boundary.md)).
