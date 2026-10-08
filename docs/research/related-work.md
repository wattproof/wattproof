# Related work

Researched 2026-09-27, research section added 2026-10-08. What exists, what it does, and where
Wattproof differs.

## The lever is old; adoption is the problem

Consolidating load and powering off idle hosts has existed for virtual machines for more than
fifteen years:

- **VMware vSphere DPM** (Distributed Power Management) powers ESXi hosts on and off based on
  cluster utilisation. It is still supported in VMware Cloud Foundation 9.1.1. A 2026 retrospective
  calls it "the feature most enterprises never used". The reasons it lists:
  - flat-rate power contracts, so there was no saving on the bill;
  - distrust of automation power-cycling hardware;
  - inconsistent behaviour across mixed hardware;
  - boot latency against sudden demand;
  - early BMC bugs that kept hosts from waking.

  Its summary: "it basically boils down to trust."
- **OpenStack Watcher**, "Saving Energy" strategy. Together with VM consolidation, it powers off
  unused compute nodes through Ironic, keeping a configured number of free hosts.
- **Hyperscalers** (Google, Meta, Microsoft) run workload-aware power management internally. They
  do not sell it.

Wattproof's bet: the missing piece is not the mechanism but **evidence and safety an operator can
check**. That means a registered experiment, a continuous holdout in production, tested
invariants and a fail-neutral design.

Consolidation, powering off and demand forecasting are all old. Wattproof does not claim them.
What it adds:

1. **Verified savings as a product feature.** Wall meters, pre-registered randomised experiments,
   and a holdout that keeps measuring in production. No Kubernetes tool does this.
2. **CPU requests that follow the day, combined with node power state.** In-place pod resize (GA
   in Kubernetes 1.35) makes it practical. It targets what keeps nodes powered: reservations, not
   load.
3. **A neutral verification toolkit** that can check any energy measure, not only Wattproof's.

## What research says

The lever and its control policies are well studied. These results shape Wattproof:

- **Predicting demand is not where the saving comes from.** AutoScale (Gandhi et al., ACM TOCS
  2012) powered 28 application servers on and off through a PDU. A policy that predicts nothing,
  but waits before switching a server off, met its response-time SLA on six real traces and beat
  predictive policies. B1's Cluster Autoscaler, which waits 10 minutes before removing an
  unneeded node, is such a policy. So a forecast alone is not expected to save more than B1. A
  planner can still do better by:
  - choosing the most efficient nodes, which needs mixed hardware;
  - powering on before a predictable daily ramp, which shows in latency and pending time more
    than in energy;
  - break-even hysteresis.

  Experiment 1 must be able to see these, so it needs two hardware generations.
- **Setup time decides.** With short enough setup times, even a naive policy saves a lot; for some
  traces, nothing helps (Gandhi et al., HotPower 2011). Server boot takes minutes, which is why
  suspend-to-RAM stays on the list.
- **Concentrating load saves power even without switching off.** Facebook's Autoscale (2014)
  routed web traffic to fewer servers: 10–15% less power over a day, 27% at night.
- **Idle power is still large.** Across 16 years of SPECpower results (arXiv 2411.07062, 2024),
  active idle drew 70.1% of full-load power in 2006, a minimum of 15.7% in 2017, and 25.7% in
  2024. Benchmark systems are tuned; servers at default BIOS settings are unlikely to do better.
- **Closest prior work on Kubernetes.** DREEM (master's thesis, Politecnico di Torino, 2025)
  forecasts load and powers Kubernetes nodes on and off, compared against Cluster Autoscaler. The
  abstract names no wall-power method and no numbers. MetalNap, below, is the closest open-source
  tool.
- **Requests, not usage, keep nodes powered.** CAST AI's 2026 report found average CPU utilisation
  of 8%, with requests 69% above usage. Its data comes from cloud clusters, not bare metal. If
  bare-metal clusters look similar, diurnal rightsizing (ADR-0010) may be a larger lever than
  power state alone.
- **GPU nodes.** A GPU holding a CUDA context draws 26–66 W above bare idle (arXiv 2605.23918,
  preprint, 2026). Industry estimates put an idle 8×H100 server at 2–3 kW. Powering whole GPU
  nodes off is the v0.1 lever applied to GPU nodes; the GPU clock lever is separate.
- **Experiment design.** A switchback experiment gives one unit randomised treatments over time.
  Bojinov, Simchi-Levi and Zhao (Management Science, 2023) derive optimal designs under carryover
  and give exact randomisation-based inference. Wattproof's blocks and its production holdout are
  switchback designs. Powering nodes off in one block changes the state at the start of the next.
  That carryover is what the warm-up exclusion handles, and the pilot measures it.
- **Measurement and verification.** The energy-services industry verifies savings with IPMVP.
  Option B isolates the retrofit and measures every parameter inside the boundary. Wattproof's
  boundary, every server at its inlets, fits Option B.

## Optimisers

- **Rebalancer** (Meta, Apache-2.0, September 2026) solves assignment problems: objects to bins
  under constraints and objectives, by MIP or by local search from the current assignment. The
  core is C++ with Python bindings, so it cannot run in the cluster (ADR-0006), and v0.1 places no
  pods (ADR-0005). It could serve as an offline optimal oracle that bounds how far the greedy
  planner is from optimal. At testbed scale, a plain MIP (HiGHS) does the same with fewer
  dependencies.

## Kubernetes, open source

| Project | What it does | Relation to Wattproof |
|---|---|---|
| MetalNap | Suspends idle bare-metal Kubernetes nodes over IPMI, Redfish, Wake-on-LAN or PDU. Drains gracefully. "Wake readily, sleep reluctantly." Nine safety rules; simulation harness with mutation testing; capacity ceilings for power or thermal limits. MIT, Python, v0.6.1 when checked on 2026-10-08, run in one environment (CI runners), no published energy measurements. | Closest in mechanism. Wattproof adds wall-power measurement, efficiency-aware node choice, service-level guards and verified savings. |
| Cluster Autoscaler | Adds and removes nodes in cloud node groups. On bare metal only through Cluster API or its external gRPC provider. | Part of our tuned baseline (B1). Reactive: it scales up after pods are pending. |
| Karpenter | Node provisioning and consolidation in clouds. | Cloud only. Not our segment. |
| Kepler | Prometheus exporter estimating energy per container, pod and node (RAPL; Redfish and NVIDIA experimental). | Measurement, not action. A class D source for our models. |
| kube-green | Scales down chosen namespaces on a schedule (e.g. dev at night). | A different lever: schedule-based and opt-in by namespace. Complementary. |
| Koordinator | Colocates batch work in reclaimed resources of latency-sensitive services, with usage prediction and interference detection. | Raises utilisation, not power state. A candidate to integrate for batch slack rather than rebuild. |
| Compute Gardener | Carbon- and price-aware delays, energy budgets, GPU power profiles. | Time shifting and carbon; not node power state. |
| Intel Kubernetes Power Manager | CPU frequency and C-state control per node or pod. | Actuator candidate for our v0.3 CPU lever. |
| Vertical Pod Autoscaler | Rightsizes requests. In-place pod resize is GA since Kubernetes 1.35 (December 2025), and VPA's InPlaceOrRecreate mode is beta. | Mechanism for diurnal rightsizing (ADR-0010). |

## Commercial, Kubernetes efficiency (cloud-first)

- **ScaleOps:** $130M Series C in March 2026, valuation above $800M; autonomous pod rightsizing.
- **CAST AI:** $108M Series C in April 2025.
- **Also:** Kubecost (IBM), Sedai, PerfectScale (DoiT), StormForge (CloudBolt), Platform9, Rafay.

They reduce cloud bills through rightsizing and instance choice. Energy is a side effect, not the
measured target, and bare-metal power state is not their focus.

## Commercial, facility side

- **Phaidra:** AI control of data-centre cooling; about $60M raised; partners include NVIDIA and
  CoreWeave.
- **Also:** Schneider Electric (EcoStruxure IT), Vertiv, Siemens, EkkoSense.

They optimise cooling and power infrastructure, not the workloads on the servers. Complementary:
IT-side savings reduce the heat they must remove.

## Server vendor tools

Dell OpenManage Power Manager, HPE iLO and Intel Data Center Manager offer power capping and
policies. They are not workload-aware and do not consolidate.

## Sources

- [MetalNap](https://github.com/mgd43b/metalnap)
- [VCF Insider on DPM (2026)](https://www.vcfinsider.com/automation/2026/03/12/distributed-power-management-the-vmware-feature-most-enterprises-never-used/)
- [vSphere DPM docs](https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/8-0/vsphere-resource-management/using-drs-clusters-to-manage-resources/managing-power-resources.html)
- [VCF 9.1.1 feature comparison](https://www.vmware.com/docs/vmware-cloud-foundation-9-1-feature-comparison-and-upgrade-paths)
- [OpenStack Watcher Saving Energy](https://docs.openstack.org/watcher/latest/strategies/saving_energy.html)
- [Koordinator](https://koordinator.sh/docs/architecture/overview)
- [Crane](https://github.com/gocrane/crane)
- [Compute Gardener](https://www.compute-gardener.com/solutions/open-source-scheduler)
- [Kepler](https://github.com/sustainable-computing-io/kepler)
- [In-place pod resize GA](https://kubernetes.io/blog/2025/12/19/kubernetes-v1-35-in-place-pod-resize-ga)
- [ScaleOps Series C (TechCrunch)](https://techcrunch.com/2026/03/30/scaleops-130m-series-c-kubernetes-efficiency-ai-demand-funding/)
- [Phaidra (DCD)](https://www.datacenterdynamics.com/en/news/ai-startup-phaidra-raises-12-million-for-data-center-cooling-control-system/)
- [AutoScale (Gandhi et al., ACM TOCS 2012)](https://www3.cs.stonybrook.edu/~anshul/tocs12.pdf)
- [Are sleep states effective in data centers? (HotPower 2011)](https://www.pdl.cmu.edu/ftp/PowerMgmt/hotpower_2011.pdf)
- [Facebook Autoscale (2014)](https://engineering.fb.com/2014/08/08/production-engineering/making-facebook-s-software-infrastructure-more-energy-efficient-with-autoscale)
- [16 Years of SPEC Power (arXiv 2411.07062)](https://arxiv.org/html/2411.07062v1)
- [DREEM (Politecnico di Torino, 2025)](https://webthesis.biblio.polito.it/37712/)
- [CAST AI 2026 report](https://cast.ai/blog/2026-state-of-kubernetes-resource-optimization-cpu-at-8-memory-at-20-and-getting-worse/)
- [The Model Parking Tax (arXiv 2605.23918)](https://arxiv.org/abs/2605.23918)
- [GPU server idle power, industry estimate (Spheron)](https://www.spheron.network/blog/ai-inference-power-electricity-cost-2026/)
- [Design and Analysis of Switchback Experiments](https://arxiv.org/abs/2009.00148v4)
- [IPMVP options overview](https://www.electrical-installation.org/enwiki/How_to_evaluate_energy_savings)
- [Rebalancer](https://engineering.fb.com/2026/09/21/open-source/rebalancer-generic-high-performance-library-assignment-problems/)
