# Feature research: GPU inference serving

Researched 2026-10-09. Decision: **change** (section 3). Go for inference serving, with GPU clock
locking instead of power caps, a wall measurement of a GPU node before anything is built, and
training ruled out.

## 1. The feature

- **Lever.** Two levers on GPU nodes that serve models (LLM inference behind vLLM, SGLang,
  KServe or Dynamo on Kubernetes):
  - *Power state.* Inference load follows people, so at night and at weekends fewer model replicas
    are needed. Pack the remaining replicas onto fewer GPU nodes, power the rest off, and power
    them on early enough that the model is loaded before the morning ramp.
  - *GPU clocks.* Lock the SM clock per serving phase. Decode is memory-bound and loses little
    throughput at a lower clock; prefill is compute-bound and needs the clock.
- **Claim, if it works.** On GPU nodes the operator owns, Wattproof cuts wall energy per token
  served and fits more serving into the same power budget, without breaking time-to-first-token
  (TTFT) and inter-token latency (ITL) targets. A holdout verifies both.
- **Strongest alternative.** NVIDIA's own stack: the Dynamo Planner scales prefill and decode
  workers against latency targets, and the Workload Power Profile Solution (WPPS) applies
  energy-optimised GPU profiles. Commercially, ScaleOps' AI Infra product (November 2025) scales
  GPU replicas and speeds up model loading; Emerald AI caps and shifts AI load for the grid.

**Training is out of scope.** Training clusters run flat at full load. Microsoft measured about 3%
power headroom in training clusters against 21% in inference clusters (Patel et al., ASPLOS 2024).
Nothing is idle to power off. The training levers, slowing GPUs that are not on the critical path
(Perseus, SOSP 2024) and NVIDIA's training power profiles, belong to the model builders and to
NVIDIA.

## 2. Findings

### Why it matters

- **AI servers are the growth in data-centre power.** The IEA's Energy and AI report (April 2025)
  puts data centres at 415 TWh in 2024, rising to about 945 TWh in 2030 in its base case.
  Accelerated servers drew about 24% of server electricity and 15% of all data-centre electricity
  in 2024, growing about 30% a year, and account for almost half of the net increase to 2030.
  Checked through secondary reports, not the IEA's own tables.
- **Power, not chips, limits new AI capacity.** Inference is where the headroom is: POLCA
  (Patel et al., ASPLOS 2024, Microsoft Azure) fits 30% more inference servers into an existing
  power budget in simulation, with minimal performance loss. NVIDIA reports up to 13% more
  throughput in a power-constrained facility from its power profiles (below).

### Research

- **The Azure inference traces have a strong daily cycle.** In the week-long Azure 2024 trace, the
  coding service's peak is 2.8× its average and 34.6× its valley; the conversation service's peak
  is 1.7× its average and 3.3× its valley (DynamoLLM, Stojkovic et al., HPCA 2025, best paper).
  Alibaba's Model Studio shows the same afternoon peak and early-morning drop over 3.5 billion
  requests (ServeGen, 2025), as does a year-long public trace from a serverless provider (Harvard,
  2026). Reasoning models are flatter than chat models (ServeGen).
- **DynamoLLM reconfigures an inference cluster through the day** (DGX H100, 8 GPUs per server).
  Its knobs are the number of instances (every 30 minutes), model parallelism (every 5 minutes)
  and GPU frequency (every 5 seconds). Against one pool at full frequency it saves 53% of energy at
  service level while meeting latency targets. Alone, frequency saves 19%, re-sharding 7% and
  instance scaling 4.1%. Over the simulated week, the average number of servers falls from 40 to
  24.6, a 38% cut. Starting a new instance takes 6–8 minutes; a frequency change 50–80 ms. One-hour
  runs on real servers, the week in a simulator. The paper does not say how energy was measured,
  or whether released servers count as off.
- **Power caps do nothing in decode; clock locking does.** On an H200, decode draws only 137–300 W
  of the 700 W limit, so a power cap never engages: the driver keeps about 1830 MHz whatever the
  cap. Locking the SM clock recovers up to 32% of decode energy at under 1% throughput loss;
  1980 → 780 MHz saves 47–90 W (Ma, Afzal, Eitzinger, Wellein, FAU Erlangen, arXiv 2605.11999, May
  2026). One GPU, four ~4B-parameter models, NVML at 50 ms checked against the hardware energy
  counter to within 2%.
- **Frequency control per phase, against the default governor.** GreenLLM (EPFL, arXiv 2508.16449,
  2025) saves up to 34% of energy with under 3.5% more latency-target violations and no
  throughput loss, on Alibaba and Azure trace replays. EcoInfer (vLLM, 2026) saves 21.5% on average,
  up to 25.4%. Both measure the GPU, not the server.
- **NVIDIA ships the clock lever.** Its Blackwell power profiles (Narayanaswamy et al., arXiv
  2510.03872, 2025) save up to 15% of energy at above 97% performance on B200, and raise
  throughput by up to 13% in a power-constrained facility. WPPS offers four profiles (Max-P and
  Max-Q, each for training and inference), applied through Mission Control on Slurm nodes of DGX
  SuperPODs.
- **What an idle GPU still draws.** A GPU holding a CUDA context draws 26–66 W above bare idle; the
  context, not the loaded model, is over 98% of the cost, and unloading pays after 1–5 minutes
  (arXiv 2605.23918, April 2026, one author, 14 H100s in production). Across 756 GPUs of six
  generations, GPUs with a program loaded but doing nothing drew about 110 W against about 35 W in
  deep idle; academic serving spent 48% of its GPU energy there (Lei et al., CMU, arXiv
  2604.04745, April 2026, NVML board power). Both are GPU board power, not the server.
- **The GPU is about half of a GPU server.** On an 8×A100 server about 50% of the provisioned power
  goes to the GPUs; CPUs, fans and the rest take the other half (Patel et al., ASPLOS 2024). An
  8×H100 HGX node peaked at about 8.4 kW in training, 18% below its 10.2 kW rating (single-node
  measurement, 2025). So GPU-board savings shrink by roughly half at the wall, and rated power
  overstates what a node draws.
- **No wall measurement of an idle or powered-off GPU node was found.** The 2–3 kW often quoted for
  an idle 8×H100 server is a vendor-blog estimate. This is the missing input for the power-state
  lever.
- **Power caps can backfire on users.** At supercomputing scale, GPU power caps lowered power and
  temperature with little job slowdown, but users who notice a slowdown may request more GPUs and
  cancel the saving (Zhao et al., MIT Lincoln Laboratory, arXiv 2402.18593, 2024).

### A production inference service close to home

GWDG's Chat AI (Doosthosseini et al., arXiv 2407.00110, 2024) serves open models to German
universities on KISSKI's inference platform: 21 nodes with 4 H100 PCIe GPUs each, in Göttingen and
Hannover, run with Slurm and Kubernetes. What the paper reports:

- 400–500 active users on a typical working day, with clear drops at weekends and holidays;
- loading a larger model into GPU memory "can take as long as ten minutes", so scaling up lags
  demand;
- no scale to zero;
- as future work, the authors propose scaling up ahead of the daily pattern and, at night,
  releasing the GPUs to ordinary Slurm jobs.

**This is our thesis in an operator's own words.** It is also the main caveat: on a shared HPC
site, GPUs freed at night are given to batch jobs, not powered off. The gain there is capacity,
not energy.

### Newest changes

- **Emerald AI** raised a $150M Series A at a $1.05B valuation (August 2026). Its Conductor
  slows, pauses, caps or moves AI jobs, and uses on-site batteries, when the grid needs less load. A
  256-GPU test in Phoenix cut power by 25% for three hours within service targets. In September
  2026 NVIDIA, Google and Emerald AI founded an AI Energy Management Alliance around a 100 MW
  "power-flexible AI factory" in Virginia.
- **ScaleOps AI Infra** (November 2025) allocates and scales GPU resources for self-hosted models,
  speeds up model loading and claims 50–70% lower GPU cost. Energy, power and on-premises use are
  not mentioned.
- **CAST AI's 2026 report** measured 5% average GPU utilisation across 23,000 unoptimised
  Kubernetes clusters (vendor data, cloud).
- **Kubernetes serving stacks scale replicas, not nodes' power.** KServe scales to zero only in its
  Knative mode. The Dynamo Planner does not fully support scale to zero; its documentation notes
  that starting a GPU worker takes minutes.

### Tools

| Tool | Licence | What it does | Bare metal | Node power state | Measured savings |
|---|---|---|---|---|---|
| NVIDIA WPPS / power profiles | Proprietary, part of Mission Control | Energy-optimised clock and power profiles per workload | Yes (DGX, Slurm) | No | Up to 15% energy, B200, vendor |
| NVIDIA Dynamo Planner | Apache-2.0 | Scales prefill and decode workers against TTFT and ITL targets | Yes, Kubernetes | No | No energy figures |
| KServe, llm-d, KEDA | Apache-2.0 | Replica scaling; scale to zero in Knative mode | Yes | No | No |
| Zeus / Perseus (ML.ENERGY) | Apache-2.0 | GPU energy measurement; frequency and power-limit optimisation, mostly training | Yes | No | Research, GPU board |
| ScaleOps AI Infra | Commercial | GPU allocation, replica scaling, faster model loading | Not stated | Not found | 50–70% GPU cost, vendor |
| Emerald AI Conductor | Commercial | Grid flexibility: slow, pause, cap, shift AI jobs; batteries | AI data centres | Not found | 25% for 3 h, 256 GPUs, vendor demo |
| Compute Gardener | Open source | Carbon-aware scheduling, GPU power profiles | Yes | No | No |
| Kepler | Apache-2.0 | Energy estimates per pod, NVIDIA support experimental | Yes | No | Estimates only |

### Public traces for the simulator

| Trace | Content | Period | Licence |
|---|---|---|---|
| Azure LLM inference 2024 | Timestamp, input and output tokens; coding and conversation | One week, May 2024 | CC-BY-4.0 |
| Azure LLM inference 2023 | Same fields (Splitwise, ISCA 2024) | One day, November 2023 | CC-BY-4.0 |
| Azure multimodal 2025 | Images, input and output tokens | One week, October 2024 | CC-BY-4.0 |
| BurstGPT | 10.3M requests to Azure OpenAI services | 213 days | Public |
| Chutes workload (Harvard) | Serverless LLM provider | April 2025 to April 2026 | Public |
| ServeGen (Alibaba) | 3.5 billion requests, 12 models | Four months | Not public; generator open |

None of these traces has power data. The simulator needs a measured GPU-node power curve as well.

### Negative results and risks

- **Power caps are inert in decode** (above). A lever built on `nvidia-smi -pl` would show no effect
  on the phase that dominates serving.
- **Lower clocks cost tail latency when load comes back.** Lowering SM clocks on idle-but-loaded
  GPUs cut power by 22% on average but raised p95 latency by 29% (Lei et al., 2026).
- **Model load time is the new boot time.** 6–8 minutes per instance at Azure, up to 10 minutes at
  Chat AI, on top of the server's own boot. That is beyond the 2–10 minute boots our simulator has
  tested so far.
- **Freed GPUs are usually sold or backfilled, not switched off.** A GPU costs far more to own than
  to power, so operators fill idle GPUs with batch work or spot capacity first. Power-off pays only
  for capacity nobody will use.
- **A GPU node may not come back healthy.** A GPU that falls off the PCIe bus (XID 79) needs a
  reboot. No study was found on whether power cycles make GPU faults more likely, so every power
  cycle is followed by a GPU health check before the node takes work, and recorded.
- **Out-of-band GPU control is slow and unreliable.** Out-of-band frequency caps took up to 40 s
  and sometimes failed without an error (Patel et al., 2024). In-band control (NVML, DCGM) takes
  milliseconds.

## 3. Judgement

1. **Is it new?** GPU clock control for inference is prior art: DynamoLLM, GreenLLM, EcoInfer, and
   NVIDIA's own power profiles. Replica scaling is prior art (Dynamo Planner, KServe, ScaleOps).
   **New, as far as found:** powering whole GPU nodes off along the daily cycle, with model load
   time in the forecast, on Kubernetes the operator owns, and the saving proven at the wall.
   DynamoLLM's 38% fewer servers shows the size of the opportunity but stops at the cluster's
   boundary: it hands servers back, it does not power them off.
2. **Predicted effect.** Against B0 (every GPU node on, default clocks):
   - *Power state:* servers needed fall by 38% on average over the Azure week (DynamoLLM,
     simulated). The energy saved is those node-hours times a GPU node's wall idle power, which
     nobody has published. **No measured range yet.** Update 2026-10-09: our simulation of the
     Azure week, sweeping idle draw from 15% to 45% of peak and model load from 2 to 10 minutes,
     predicts 27–52% less energy than B0 for the coding service and 8–24% for conversation
     ([notebook](../../notebook/2026-10-09-gpu-inference-simulation.md); simulation, power
     assumed).
   - *Clocks:* 15–34% of GPU energy (NVIDIA, GreenLLM, Ma et al.; GPU board, one to a few GPUs).
     At the wall, with the GPUs about half of server power, that is roughly 7–17%. This is our
     estimate, not a measurement.

   Against B1 (a replica autoscaler plus reactive node power-off): the AutoScale result for CPU
   servers suggests similar energy, with Wattproof's gain showing in latency during the morning
   ramp, because loading models takes minutes. Not measured anywhere.
3. **Design changes.**
   - The clock lever **locks SM clocks per phase**, decode first, through NVML or DCGM in band. It
     never relies on power caps for decode. Prefill keeps full clocks unless a latency target allows
     otherwise.
   - On nodes where NVIDIA's WPPS is available, use its Max-Q inference profile as the actuator
     instead of our own clock table, and measure it.
   - **Power-on lead time = server boot + model load + warm-up.** The forecast must cover 10–20
     minutes on GPU nodes, against 2–10 on CPU nodes. Each model keeps a minimum of warm replicas.
   - Wattproof is **not another replica autoscaler.** It reads the replica count set by the
     operator's autoscaler (KServe, KEDA, Dynamo Planner), packs replicas onto fewer nodes and
     powers off empty ones. The same relation as with VPA.
   - **Backfill before power-off.** If a batch queue can use freed GPUs, Wattproof reports the
     freed capacity and does not power the nodes off. Power-off only for capacity with no taker.
   - After every power-on: a GPU health check (DCGM diagnostics) before the node is uncordoned. A
     power-cycle record per GPU node, as for CPU nodes.
   - **Measurement:** NVML stays class D. Every headline number comes from the node's wall power.
4. **Experiment changes.**
   - **First a measurement, not an experiment** (Phase 1G): one GPU node at the wall, in five
     states: off, bare idle, idle with a CUDA context, model loaded, serving at three loads, and
     the full cycle of boot, model load and warm-up, timed and metered. Plus decode and prefill at
     three locked SM clocks. These are the inputs the simulator lacks.
   - **Then a simulation** with the Azure 2024 week (coding and conversation) and the Chutes
     year, with model load as part of the transition time.
   - **Later, an Experiment 3 for GPU inference.** Endpoints: wall kWh per million tokens served,
     TTFT and ITL attainment at p95 and p99, minutes of requests waiting for a model to load.
     Arms: B0, B1, T, T+C (clock locking). Its prior comes from Phase 1G and the simulation.
5. **Advantage.** Assessed privately.
6. **Decision: change.**
   - **Go** for inference serving: GPU node power state, then SM clock locking.
   - **Drop** power caps as a decode lever, and drop training as a segment.
   - **Pull the GPU measurement forward** to before v0.1 ships, on an operator's node or a rented one.
   - **Move the clock lever ahead of the CPU power settings** (accepted 2026-10-09): its evidence is
     larger and better measured ([ADR-0014](../../adr/0014-gpu-inference-before-cpu-power-settings.md)).

## How Wattproof works with these tools

Added 2026-10-09, after ADR-0014 was accepted. The design builds on them instead of replacing
them ([architecture](../../architecture.md#gpu-inference-nodes-v03-adr-0014)):

- **Replica autoscalers** (KServe, KEDA, Dynamo Planner, ScaleOps): Wattproof reads the replica
  counts they set, packs the replicas onto fewer GPU nodes and powers off the empty ones. It never
  sets replicas itself.
- **NVIDIA's power profiles:** where WPPS is installed, its Max-Q inference profile is the clock
  actuator. Wattproof measures its effect at the wall.
- **Grid-flexibility platforms** (Emerald AI): they act on grid events for hours at a time; Wattproof
  acts on the daily cycle. A wall-metered holdout is the kind of baseline a flexibility payment is
  measured against.
- **Verification:** the first study of Wattproof Verify measures a vendor's GPU power profile at
  the wall ([Verify study V1](../../experiment-protocol.md#verify-study-v1-a-gpu-power-profile-at-the-wall-outline)).

## Sources

- Patel et al., Characterizing power management opportunities for LLMs in the cloud (POLCA), ASPLOS 2024:
  [PDF](https://www.microsoft.com/en-us/research/wp-content/uploads/2024/03/GPU_Power_ASPLOS_24.pdf), checked 2026-10-09
- Stojkovic et al., DynamoLLM, HPCA 2025: [arXiv 2408.00741](https://arxiv.org/abs/2408.00741)
- Ma, Afzal, Eitzinger, Wellein, The illusion of power capping in LLM decode, 2026: [arXiv 2605.11999](https://arxiv.org/abs/2605.11999)
- Liu et al., GreenLLM, 2025: [arXiv 2508.16449](https://arxiv.org/abs/2508.16449)
- EcoInfer, Electronics 2026: [doi 10.3390/electronics15102139](https://doi.org/10.3390/electronics15102139)
- Narayanaswamy et al., Datacenter energy optimized power profiles, 2025: [arXiv 2510.03872](https://arxiv.org/abs/2510.03872)
- [NVIDIA Workload Power Profile Solution](https://docs.nvidia.com/mission-control/docs/systems-administration-guide/2.0.0/wpps/introduction.html)
- Vadari, The model parking tax, 2026: [arXiv 2605.23918](https://arxiv.org/abs/2605.23918)
- Lei et al., The energy cost of execution-idle in GPU clusters, 2026: [arXiv 2604.04745](https://arxiv.org/abs/2604.04745)
- Zhao et al., Sustainable supercomputing for AI: GPU power capping at HPC scale, 2024: [arXiv 2402.18593](https://arxiv.org/abs/2402.18593)
- Chung et al., Perseus: reducing energy bloat in large model training, SOSP 2024: [arXiv 2312.06902](https://arxiv.org/abs/2312.06902); [Zeus](https://ml.energy/zeus)
- [Single-node power demand during AI training on an 8-GPU H100 system (2025)](https://www.researchgate.net/publication/390221924_Single-Node_Power_Demand_During_AI_Training_Measurements_on_an_8-GPU_NVIDIA_H100_System)
- Doosthosseini et al., Chat AI: a seamless Slurm-native solution for HPC-based services, 2024: [arXiv 2407.00110](https://arxiv.org/abs/2407.00110)
- [KISSKI inference platform](https://kisski.gwdg.de/en/leistungen/2-01-02_inferenz), checked 2026-10-09
- Xiang et al., ServeGen, 2025: [arXiv 2505.09999](https://arxiv.org/abs/2505.09999)
- A year in LLM serving, 2026: [arXiv 2608.13573](https://arxiv.org/abs/2608.13573); [trace](https://github.com/HarvardMadSys/chutes_workload)
- [Azure public dataset (LLM inference traces)](https://github.com/Azure/AzurePublicDataset); [BurstGPT](https://github.com/hpmll/burstgpt)
- [NVIDIA Dynamo Planner](https://docs.nvidia.com/dynamo/latest/components/planner)
- [ScaleOps AI Infra launch (2025-11-20)](https://www.prnewswire.com/il/news-releases/scaleops-launches-ai-infrastructure-resource-management-product-to-power-self-hosted-ai-at-scale-302621807.html)
- [Emerald AI $150M Series A (SiliconANGLE, 2026-08-25)](https://siliconangle.com/2026/08/25/data-center-power-startup-emerald-ai-raises-150m-at-1-05b-valuation/);
  [AI Energy Management Alliance (SiliconANGLE, 2026-09-17)](https://siliconangle.com/2026/09/17/nvidia-google-and-emerald-ai-launch-flexible-data-center-consortium/)
- [CAST AI 2026 report, press release](https://cast.ai/press-release/2026-state-of-kubernetes-optimization-report/)
- IEA, Energy and AI (April 2025), via [S&P Global](https://www.spglobal.com/energy/en/news-research/latest-news/electric-power/041025-global-data-center-power-demand-to-double-by-2030-on-ai-surge-iea)
