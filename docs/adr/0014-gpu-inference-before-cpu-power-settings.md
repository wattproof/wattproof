# ADR-0014: GPU inference serving is the GPU segment; its clock lever comes before CPU power settings

Status: accepted, 2026-10-09. Amends ADR-0004 (order of levers: CPU power settings in v0.3, GPU
clocks in v0.4).

## Context
The [GPU inference feature note](../research/features/gpu-inference.md) found:

- Inference load follows a daily cycle (Azure 2024 week: coding peak 34.6× its valley,
  conversation 3.3×). Training runs flat, with about 3% power headroom against 21% for inference
  (Patel et al., ASPLOS 2024).
- Power caps do not engage in memory-bound decode. Locking the SM clock does: up to 32% of decode
  energy at under 1% throughput loss (arXiv 2605.11999). Per-phase frequency control saves 15–34%
  of GPU energy within latency targets (GreenLLM, DynamoLLM, NVIDIA's power profiles). All of these
  measure the GPU board, which is about half of a GPU server's power.
- No published evidence of a similar size was found for CPU power settings on the workloads we
  target.
- Nobody has published the wall power of an idle or powered-off GPU node, the input the
  power-state lever needs.

## Decision
- The GPU segment is **inference serving**. Training is out of scope.
- The GPU clock lever is **SM clock locking per serving phase**, decode first, in band through
  NVML or DCGM. Power caps are not used as a decode lever.
- **The GPU clock lever moves to v0.3; CPU power settings move to v0.4.**
- **A GPU node is measured at the wall before v0.1 ships** (ROADMAP Phase 1G). Building the GPU
  levers waits for that measurement.
- Powering GPU nodes off remains the v0.1 lever applied to GPU nodes. Two additions: power-on lead
  time includes model load and warm-up, and a node is uncordoned only after a GPU health check.
  Where a batch queue can use the freed GPUs, Wattproof reports the capacity and does not power the
  nodes off.

## Consequences
- v0.3 needs a GPU node for development and testing, which a CPU-only testbed does not provide:
  an operator's inference nodes, or rented GPU nodes.
- The simulator gains an inference mode: replicas per model from token traces, and model load time
  as part of the transition.
- Experiment 3 (GPU inference) gets wall kWh per million tokens, TTFT and ITL attainment, and
  minutes of requests waiting for a model to load as endpoints.
- If, with Phase 1G's measured idle draw and break-even time, simulated GPU power state saves less
  than 5% of B0's energy on the Azure week, it is dropped and only the clock lever remains.
