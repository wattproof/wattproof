# 2026-10-09: GPU node power state on the Azure inference week

**Simulation, not a measurement** ([ADR-0011](../adr/0011-simulate-before-the-testbed.md)). The
load is measured: the Azure LLM inference trace 2024, one week each of a coding and a conversation
service (DynamoLLM, HPCA 2025; CC-BY 4.0). The GPU node's power is not: nobody has published the
wall power of an idle or powered-off GPU node ([feature note](../research/features/gpu-inference.md)),
so its idle share and the model load time are swept. Evidence level E1 for the power, measured
inputs for the load.

The question, from [ADR-0014](../adr/0014-gpu-inference-before-cpu-power-settings.md): at which
idle power and model load time is powering GPU nodes off worth it? Phase 1G drops the lever if
it saves less than 5% of B0's energy.

Reproduce:

    python3 -I analysis/traces/azure2024_minutes.py AzureLLMInferenceTrace_code_1week.csv \
        AzureLLMInferenceTrace_conv_1week.csv > testdata/azure2024-llm-minutes.csv
    go run ./cmd/wattproof-sim -only gpu

## Inputs

- **Load per minute:** generated tokens plus a tenth of the context tokens. The hourly request
  counts reproduce the paper's shape: coding peak 3.07× its mean and 34.5× its valley (paper: 2.8×
  and 34.6×), conversation 1.67× and 3.6× (paper: 1.7× and 3.3×). The two files cover different
  weeks: coding 10–16 May, conversation 12–18 May 2024.
- **Cluster:** 20 identical GPU nodes, one model replica each. Replicas follow the load averaged
  over a window (5 or 30 minutes, as a replica autoscaler does); at the week's peak 90% of the
  nodes hold a replica, each at 70% of its throughput.
- **Node power:** linear from idle to peak (6 kW nominal; the ratios do not depend on it); idle at
  15%, 30% or 45% of peak; 50 W off.
- **Transition:** server boot 5 minutes plus model load of 2, 6 or 10 minutes, drawn at the power of
  30% load; shutdown 2 minutes at idle. At least 2 nodes stay on.
- **Arms:** B0, every node on. B1, Cluster Autoscaler's default rules, powering nodes on only once
  replicas are pending. T, Wattproof's planner forecasting from yesterday. T*, T with a perfect
  forecast, an upper bound. Th, T with a one-hour margin on the break-even cycle.

## Results

Energy relative to B0 (below 1: less). Pending: minutes in the scored week with replicas waiting
for a Ready node. Cycles: power-offs per node per day.

| Service | Window | Idle | Model load | T/B0 | B1/B0 | T/B1 | Pending B1 | Pending T | Cycles B1 | Cycles T |
|---|---|---|---|---|---|---|---|---|---|---|
| Coding | 5 min | 15% | 2–10 min | 0.71–0.73 | 0.68 | 1.05–1.07 | 876–1241 | 143–234 | 1.2–1.4 | 3.2–4.0 |
| Coding | 5 min | 45% | 2–10 min | 0.48–0.50 | 0.44 | 1.09–1.12 | 876–1241 | 163–267 | 1.2–1.4 | 3.5–4.4 |
| Coding | 30 min | 15% | 2–10 min | 0.71 | 0.69 | 1.03–1.04 | 657–1310 | 4–79 | 0.7 | 1.0 |
| Coding | 30 min | 45% | 2–10 min | 0.48 | 0.45 | 1.07–1.08 | 657–1310 | 4–79 | 0.7 | 1.1 |
| Conversation | 5 min | 15% | 2–10 min | 0.90–0.92 | 0.87–0.88 | 1.04–1.05 | 1065–1752 | 38–111 | 1.6–1.7 | 4.3–5.2 |
| Conversation | 5 min | 45% | 2–10 min | 0.76–0.78 | 0.71 | 1.07–1.10 | 1065–1752 | 50–116 | 1.6–1.7 | 4.8–5.8 |
| Conversation | 30 min | 15% | 2–10 min | 0.90–0.91 | 0.88 | 1.02–1.03 | 800–1683 | 0–3 | 0.9 | 1.2 |
| Conversation | 30 min | 45% | 2–10 min | 0.77–0.78 | 0.73 | 1.06 | 800–1683 | 0–3 | 0.9 | 1.3 |

The full table, with 30% idle and every load time, is the output of the command above.

## What it shows

1. **The 5% rule passes everywhere.** T saves 27–52% of B0's energy on the coding service and
   8–24% on the conversation service, at every idle share and load time tested. The idle share
   decides how much is saved, not whether: even at 15% of peak. On average T keeps only 7 (coding) and 13
   (conversation) of the 20 nodes on. **So Phase 1G no longer decides go or stop on idle power
   alone.** It sizes the saving.
2. **The reactive baseline uses less energy and fails on service.** T uses 2–12% more energy than
   B1, because B1 powers on only after replicas are pending. They then wait 11–29 hours a week in
   total, against at most 4.5 hours for T and under 80 minutes with a 30-minute window. This is the
   CPU result again ([first simulation](2026-10-08-first-simulation.md)): the planner pays a few
   percent of energy for being ready before the ramp.
3. **Model load time costs service, not energy.** From 2 to 10 minutes of load, T's energy moves
   by about 2 points; its pending minutes rise severalfold. What Phase 1G must time precisely is
   boot plus model load.
4. **Bursty load makes the planner cycle GPU nodes too often.** With a 5-minute window, T powers
   each node off 3–6 times a day; a one-hour hysteresis barely helps (3–4). With a 30-minute
   window, it falls to about once a day at almost the same energy. The churn comes from minute-scale
   noise in the forecast input, not from the daily cycle. A perfect forecast (T*) saves at most 3
   points more than T.

## What it changes

- **Phase 1G's questions, in order:** (1) how long boot plus model load takes, from local disk and
  from network storage; (2) whether nodes and GPUs come back healthy after one to a few power
  cycles a day; (3) the idle draw, which sizes the saving. Backfill (whether a batch queue would
  use the freed nodes) is asked of the operator, not measured.
- **The GPU planner smooths its input and limits cycles.** It plans on load averaged over at least
  30 minutes and carries a cycle budget per node per day, which this simulator does not yet have.
  Both are added before any GPU number is published as a prior.
- **The claim against B1 is service level**, as on CPU: no replicas waiting for a node during the
  ramp, at a few percent more energy than a reactive policy that lets them wait.

## Limits

- One week per service, one cloud provider, replayed twice. Fluid model: replicas are fractional,
  and every replica fits on any node.
- Power is linear from idle to peak; real GPU nodes are not, and decode draws far below the GPUs'
  limit ([feature note](../research/features/gpu-inference.md)).
- The cluster is sized for this week's peak. An operator with more spare nodes saves more against
  B0; one with fewer saves less.
- Model load is counted only when a node powers on. Loading a new replica on a node that is
  already on costs the same in every arm and is left out.
