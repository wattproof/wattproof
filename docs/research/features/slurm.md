# Feature research: Slurm clusters

Researched 2026-10-16. Decision: **go for observing and verifying; acting waits** (section 3).

## 1. The feature

- **Lever.** Node power state on clusters scheduled by Slurm instead of Kubernetes: idle nodes
  powered off, and on again before jobs need them. Most research computing centres and many GPU
  clusters run Slurm.
- **Claim, if it works.** On a Slurm cluster, Wattproof measures at the wall what powering idle
  nodes off saves and what it costs in job wait, power cycles and failed resumes, in a randomised
  design. Later, possibly: it saves more than Slurm's own power saving at the same job wait.
- **Strongest alternative.** Slurm's built-in power saving. `slurmctld` suspends nodes that stay
  idle for `SuspendTime` and resumes them when a job needs them, through site-written
  `SuspendProgram` and `ResumeProgram` scripts. It is free, part of every Slurm installation, and
  used in production.

## 2. Findings

### Slurm's own power saving

- **Mechanism** ([Slurm Power Saving Guide](https://slurm.schedmd.com/power_save.html)). Nodes
  idle for `SuspendTime` are suspended by `SuspendProgram`; nodes needed by allocated work are
  resumed by `ResumeProgram`. A node that does not register within `ResumeTimeout` is set DOWN
  and its jobs are requeued. `SuspendRate` and `ResumeRate` limit transitions per minute; the
  guide's example is a 1000-node job on an idle cluster, which would otherwise draw several
  megawatts at once. `SuspendExcNodes`, `SuspendExcParts` and, since 23.02, `SuspendExcStates`
  exclude nodes, partitions and node states. Slurm ships no power-on or power-off method: every
  site writes its own scripts.
- **The trigger is a fixed idle timer per partition.** Nothing in Slurm forecasts demand: a node
  is resumed when a job is already allocated to it, so the job waits for the boot. Sites trade
  this wait against the saving by choosing `SuspendTime`.

### Production reports

- **Jülich Supercomputing Centre** (Suarez et al., Energy-aware operation of HPC systems in
  Germany, arXiv 2411.16204, 2024; Frontiers in HPC, 2025). "At JSC, idle nodes are powered down
  by Slurm." The JUSUF system's consumption fell 25%, from roughly 660 MWh in 2022 to 495 MWh in
  2023, after "a rather large implementation effort for the power down feature". **A year-on-year
  comparison**: workload, utilisation and other changes between the two years are not separated
  from the power-down feature.
- **DTU, Niflheim cluster** (Nielsen, SLUG'23). IPMI scripts published as open source; a
  `SuspendTime` of 3600 s "to avoid too frequent power cycles". The saving is **estimated, not
  measured**: the share of node time in the Down state (8.59%, 58 nodes, over 59 hours) times an
  assumed 250 W idle draw gives 855 kWh. Operational findings:
  - before 23.02, power saving also suspended DOWN and DRAINED nodes;
  - powered-off nodes cannot take OS or firmware updates;
  - BMCs fail or reset to defaults after motherboard swaps, and then power commands fail;
  - most server models showed no change in failure rate, but **one model failed often within hours
    or days of a power cycle** (motherboard, DIMM, CPU);
  - some fabrics and storage services (Slingshot, InfiniBand and Omni-Path fabric managers, GPFS
    quorum nodes) have problems when nodes go down.
- **GPU nodes.** A site reported on the slurm-users list (October 2022) that it kept power saving
  off on GPU nodes because they sometimes failed to finish booting; another reported slower but
  reliable boots. GWDG's Chat AI (Doosthosseini et al., arXiv 2407.00110, 2024) serves models
  from Slurm-scheduled GPU nodes and proposes releasing GPUs to ordinary Slurm jobs at night
  ([GPU inference note](gpu-inference.md)).

### Research

- **Idle power is worth tuning, and the suspend route is harder.** TU Dresden cut idle power by
  about 90 W per node, 57 kW for a 630-node system, through idle-state configuration during
  installation (Ilsche, Schrader, Schöne, IEEE CLUSTER Workshops 2024). They note that suspend saves
  more but brings operational problems, resume reliability first.
- **Boot time is the cost.** Node shutdown and power-on on HPC systems "typically 10–15 minutes,
  and in extreme cases can reach 30–45 minutes", as cited by Amrizal et al. (SPARS, arXiv
  2512.13268, 2025) from Ohmura et al. (JSSPP 2022) and Budiarjo et al. (SCA 2025). Naive power-off
  makes jobs wait for boots.
- **Forecasting the wake-up is research, not a tool.** SPARS (a simulator with reinforcement
  learning) and SNF-ICON (Pulungan et al., arXiv 2608.29656, August 2026: smallest-need-first
  scheduling, predictive wake timing and adaptive warm spares) both evaluate in trace simulation
  only, and both find results depend strongly on transition-time and power-model assumptions. No
  open-source tool was found that forecasts Slurm demand and powers nodes on ahead of it.
- **No randomised or wall-metered evaluation** of Slurm power saving was found.

### Energy accounting in Slurm

- `acct_gather_energy` plugins read IPMI, RAPL or NVML (GPU); `scontrol show node` reports
  `CurrentWatts` and `AveWatts`. Per-job energy is reliable only for jobs that hold whole nodes
  exclusively (SchedMD, SLUG'13). Known problems: bogus `ConsumedEnergyRaw` values under BMC load
  (SchedMD ticket 20207), read errors recorded as 0 rather than as missing, and wrong DRAM energy
  in the RAPL plugin (ticket 9956). In Wattproof's terms these are class C (IPMI) and class D
  (RAPL, NVML) sources ([measurement.md](../../measurement.md#meter-classes)).

### Tools

| Tool | Licence | What it does | Power state | Measured savings |
|---|---|---|---|---|
| Slurm power saving | GPL-2.0 | Suspends idle nodes after a timer, resumes on allocation; scripts site-written | Yes | Year-on-year at JSC (25%); estimate at DTU |
| DTU Slurm_tools `power_save` | GPL-3.0 | IPMI suspend and resume scripts for Slurm | Actuator only | No |
| EAR (BSC, Lenovo) | Open core | Energy-aware runtime with a Slurm plugin: CPU and GPU frequency per job, accounting; in production on SuperMUC-NG since 2019 | No | Per job, from node sensors |
| ClusterCockpit | MIT | Job-specific performance and energy monitoring; experimental energy-control components for exclusive jobs, no GPU yet (2026) | No | No |
| HPE PowerSched | Proprietary | Dynamic power limits per job (HLRS) | No | Not found |
| NVIDIA WPPS / Mission Control | Proprietary | Clock and power profiles per workload on DGX Slurm nodes | No | Up to 15% energy, vendor |

SchedMD, Slurm's developer, has belonged to NVIDIA since December 2025.

## 3. Judgement

1. **Is it new?** Powering idle Slurm nodes off is prior art, built into Slurm and in production.
   Two things are not found anywhere: **a measured saving with a counterfactual**, and **power-on
   ahead of forecast demand** in a tool an operator can install.
2. **Expected effect.** Against no power saving (our B0 on Slurm): JSC's 25% is the only
   production figure, and it is year-on-year. Against Slurm's own power saving (B1 on Slurm): no
   source. The simulator found no energy margin over a reactive baseline on identical servers and
   a small one on mixed hardware ([trace simulation](../../notebook/2026-10-08-trace-simulation.md)).
   **On Slurm, assume no energy claim against stock power saving.** What a forecast could buy
   is shorter job waits at the same `SuspendTime`, or a shorter `SuspendTime` at the same wait.
   That is unmeasured, and on a busy queue idle nodes may be too rare to matter.
3. **What it changes in the design.**
   - **A scheduler adapter boundary.** Meters, power profiles, models, the simulator, the
     experiment engine and the report are independent of the scheduler. The scheduler adapter
     supplies cluster state (nodes, what runs where, pending demand) and applies power steps in
     the scheduler's own terms. Kubernetes is the first adapter; Slurm the second.
   - **On Slurm, Slurm stays in charge.** Wattproof never places or preempts jobs. Its power
     driver can serve as `SuspendProgram` and `ResumeProgram`; a later planner would only power
     nodes on early (`scontrol update NodeName=… State=POWER_UP`), choose which nodes go idle
     first through node `Weight`, and recommend `SuspendTime` and the exclusions.
   - **Fail neutral on Slurm.** If Wattproof stops, Slurm's own settings apply unchanged, and the
     resume program works without the Wattproof controller.
   - **Capacity with a taker.** Slurm suspends only IDLE nodes. A node held idle for a large
     pending job (state PLANNED) must not be powered off; `SuspendExcStates` covers it.
   - **Health check after resume.** As on GPU nodes in Kubernetes: a node takes jobs only after a
     health check passes (for example LBNL NHC). The power-cycle record includes failures by
     server model, given DTU's finding.
4. **What it changes in the experiment.** A Slurm study is a switchback on whole partitions,
   because nodes in one partition share one queue and cannot form separate arms. Arms: power
   saving off; the site's own `SuspendTime`; later, Wattproof. Primary endpoint: wall kWh per
   delivered node-hour (or GPU-hour). Non-inferiority: job wait time, time jobs spend waiting for
   a node to boot (the CONFIGURING state), failed resumes and requeued jobs. Outlined as Verify
   study V2 in the [protocol](../../experiment-protocol.md#verify-study-v2-slurm-power-saving-at-a-production-site-outline).
5. **Advantage.** Assessed privately.
6. **Go, change or drop.** **Go** for two pieces that need no actuator: a Slurm report from
   accounting data (idle node-hours, what powering them off would save, what job wait it would
   cost) and Verify study V2. **Acting waits** for V2's result at one site and for a site that
   asks for it. Neither moves T0 or v0.1.

## Update 2026-10-16: two public Slurm traces

[Notebook](../../notebook/2026-10-16-slurm-energy-two-sites.md), [plan](../slurm-trace-plan.md):
SURF Lisa (2022) and CINECA Marconi100 (2020), job tables with measured node power, neither
running power saving in the period.

- **Idle node time is large.** At `SuspendTime` = 30 minutes, 19–24% of Lisa's and 40% of
  Marconi100's node-hours would be powered off: 18–19% of Lisa's GPU-node energy and 27–28% of
  Marconi100's. Lisa's CPU nodes save 1–4%, a lower bound (BMC reading without shared chassis
  power).
- **A forecast adds no energy.** Perfect foresight saves 0.1–1.0 points less than the timer.
- **It could remove waits.** With Slurm allocating powered-up nodes first (counted optimistically),
  3.1% of Lisa's CPU job starts and 4.2% of Marconi100's would still wait for a boot; 0.8% on
  Lisa's GPU nodes.

This confirms judgement 2: no energy claim against stock power saving; the case for a Slurm
planner is job wait, now bounded on two sites. The report's first finding at a site without power
saving would be the saving it leaves on the table.

## Update 2026-10-16: energy at equal wait

Under the plan's [energy-at-equal-wait rule](../slurm-trace-plan.md#changes-to-this-plan), a perfect
forecast with a 10-minute timer gains −0.03 to +0.46 points of energy over a 30-minute timer at the
pessimistic end of the assumptions (Lisa CPU, Lisa GPU, Marconi100), and at most 1.5 points with a
5-minute boot ([notebook](../../notebook/2026-10-16-slurm-energy-two-sites.md#energy-at-equal-wait-added-2026-10-16-after-the-rule-was-written)).
Below the 1-point bar everywhere, so **a Slurm planner is dropped**. Acting on Slurm, if a site asks
for it, means Wattproof's power driver as `SuspendProgram` and `ResumeProgram` and recommended
settings, not a forecast.

## Sources

- [Slurm Power Saving Guide](https://slurm.schedmd.com/power_save.html)
- [Nielsen, Power Saving with Slurm, SLUG'23 (DTU)](https://slurm.schedmd.com/SLUG23/DTU-SLUG23.pdf)
- [DTU Slurm_tools, power_save](https://github.com/OleHolmNielsen/Slurm_tools/tree/master/power_save)
- [Suarez et al., Energy-aware operation of HPC systems in Germany (arXiv 2411.16204)](https://arxiv.org/abs/2411.16204)
- [Ilsche, Schrader, Schöne, Optimizing idle power of HPC systems (CLUSTER Workshops 2024)](https://tu-dresden.de/zih/forschung/ressourcen/dateien/projekte/energyefficiency/2024-Ilsche-optimizing-idle-power-of-hpc-systems-authorversion.pdf)
- [Amrizal et al., SPARS (arXiv 2512.13268)](https://arxiv.org/abs/2512.13268)
- [Pulungan et al., SNF-ICON (arXiv 2608.29656)](https://arxiv.org/abs/2608.29656)
- [slurm-users: GPU node not waking up after power save (2022)](https://lists.schedmd.com/pipermail/slurm-users/2022-October/009261.html)
- [SchedMD ticket 20207, bogus ConsumedEnergyRaw](https://support.schedmd.com/show_bug.cgi?id=20207)
- [SchedMD ticket 9956, RAPL plugin values](https://support.schedmd.com/show_bug.cgi?id=9956)
- [acct_gather.conf(5)](https://manpages.org/acct_gatherconf/5)
- [EAR at LRZ](https://doku.lrz.de/energy-aware-runtime-10746191.html)
- [ClusterCockpit](https://clustercockpit.org/)
- [Doosthosseini et al., Chat AI (arXiv 2407.00110)](https://arxiv.org/abs/2407.00110)
