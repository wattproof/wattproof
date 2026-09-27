# Related work

Researched 2026-09-27. What exists, what it does, and where Wattproof differs.

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

## Kubernetes, open source

| Project | What it does | Relation to Wattproof |
|---|---|---|
| MetalNap | Suspends idle bare-metal Kubernetes nodes over IPMI, Redfish, Wake-on-LAN or PDU. Drains gracefully. "Wake readily, sleep reluctantly." Nine safety rules; simulation harness with mutation testing. MIT, Python, pre-1.0, run in one environment (CI runners), no published energy measurements. | Closest in mechanism. Wattproof adds wall-power measurement, efficiency-aware node choice, demand forecasting, service-level guards and verified savings. |
| Cluster Autoscaler | Adds and removes nodes in cloud node groups. On bare metal only through Cluster API or its external gRPC provider. | Part of our tuned baseline (B1). Reactive: it scales up after pods are pending. |
| Karpenter | Node provisioning and consolidation in clouds. | Cloud only. Not our segment. |
| Kepler | Prometheus exporter estimating energy per container, pod and node (RAPL; Redfish and NVIDIA experimental). | Measurement, not action. A class D source for our models. |
| kube-green | Scales down chosen namespaces on a schedule (e.g. dev at night). | A different lever: schedule-based and opt-in by namespace. Complementary. |
| Koordinator | Colocates batch work in reclaimed resources of latency-sensitive services, with usage prediction and interference detection. | Raises utilisation, not power state. A candidate to integrate for batch slack rather than rebuild. |
| Compute Gardener | Carbon- and price-aware delays, energy budgets, GPU power profiles. | Time shifting and carbon; not node power state. |
| Intel Kubernetes Power Manager | CPU frequency and C-state control per node or pod. | Actuator candidate for our v0.2 CPU lever. |
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
