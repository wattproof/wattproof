# ADR-0009: The test cluster is public and live

Status: accepted, 2026-09-27

## Context
A savings claim convinces more people when they can watch the test cluster instead of reading a
summary afterwards. Showing results live during a registered experiment invites the objection that
we stopped when the numbers looked good.

## Decision
The project website shows the testbed live, with a delay of one to five minutes:
- wall power per node and in total, with the meter class;
- node states (on, draining, off);
- the running arm and the time left in the block;
- offered load and p99 latency;
- cumulative energy per arm.

It also shows the hardware inventory, calibration results, workload definitions and the exact B1
configuration (linked to git).

The site has two sections that are never mixed:
- **Lab notebook:** everything since day zero, including exploratory runs and versions. It shows
  progress.
- **Registered results:** only the confirmatory experiments. These are the numbers we claim.

For registered experiments, the number of blocks and the analysis are fixed in advance and the
experiment does not stop early. The page says so.

Data flows one way only. The recorder pushes aggregated data to object storage, and a static site
reads it. Nothing on the internet can reach the testbed. Serial numbers and internal addresses are
removed before publishing.

## Consequences
Failures are visible too. That is the point. The page costs about a week of work in Phase 2 and
becomes the project's main marketing asset.
