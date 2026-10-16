# ADR-0015: Slurm clusters: observe and verify first, act through Slurm's own power saving

Status: proposed, 2026-10-16.

## Context
Most research computing centres and many GPU clusters run Slurm, not Kubernetes. The
[Slurm feature note](../research/features/slurm.md) found:

- Slurm already powers idle nodes off, after a fixed idle timer per partition, through scripts
  each site writes. Jülich reports 25% less annual energy on one system after enabling it, measured
  year on year; DTU estimates its saving from node-hours down times an assumed idle draw. No
  measured saving with a counterfactual was found.
- Nothing in Slurm, and no open-source tool found, powers nodes on ahead of forecast demand. Jobs
  wait for the boot, typically 10–15 minutes on HPC systems.
- Most of Wattproof does not depend on Kubernetes: meters, power profiles, the power and
  transition models, the simulator, the experiment engine and the report. What does: the planner's
  pod placement check, taints, the eviction API, the resize controller and the rightsizing report.

## Decision
- **The scheduler is an adapter.** It supplies cluster state (nodes, what runs where, pending
  demand) and applies power steps in the scheduler's own terms. Kubernetes is the first adapter;
  v0.1 stays Kubernetes-only.
- **On Slurm, two pieces come first, neither of which acts on the cluster:**
  1. **A Slurm report** from accounting data the site exports (`sacct`, `sreport`, node events):
     idle node-hours by partition and hour of week, the energy they draw (measured where meters
     exist, otherwise from power profiles, labelled), and, where power saving runs, its job wait
     and power cycles.
  2. **Verify study V2**: a randomised measurement of what Slurm's own power saving saves at the
     wall and costs in job wait, at a site that agrees.
- **Acting on Slurm waits** for V2's result at one site and a site that asks for it. If it comes,
  Slurm keeps scheduling: Wattproof provides the `SuspendProgram` and `ResumeProgram` through its
  power driver, powers nodes on ahead of forecast demand, ranks nodes through `Weight`, and
  recommends `SuspendTime` and exclusions. It never places, holds or preempts jobs.
- **Fail neutral on Slurm.** If Wattproof stops, the site's Slurm settings apply unchanged, and
  resume works without the Wattproof controller.

## Consequences
- The architecture states which components are independent of the scheduler; the README says
  what works without Kubernetes.
- The Slurm report is built only when a site offers data, and never before Phase 1R's Kubernetes
  report. It reuses the report's output format and the power profiles.
- The experiment protocol gains Verify study V2 (outline). The unit is a whole partition in
  switchback blocks, because nodes in one partition share one queue.
- No energy claim against Slurm's own power saving is expected. The simulator found none against a
  reactive baseline on identical servers. A Slurm planner would have to win on job wait at equal
  energy, or on energy at equal wait, in V2's design before it ships.
- Public traces support this (2026-10-16, [notebook](../notebook/2026-10-16-slurm-energy-two-sites.md),
  two sites, E2): a 30-minute timer alone would save 18–28% of GPU-node energy at sites that run
  without power saving; perfect foresight adds nothing to that, and would remove the boot waits of
  3–4% of job starts that remain after Slurm's own placement.
- Holding BMC credentials on a production HPC site needs the same threat model and security
  review as a Kubernetes design partner (ROADMAP v0.3).
