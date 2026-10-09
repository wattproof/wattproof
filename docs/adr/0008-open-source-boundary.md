# ADR-0008: Apache-2.0 for everything that makes one cluster better

Status: accepted, 2026-10-09. Proposed 2026-09-26; alternatives and trademark added on acceptance.

## Context
Adoption by operators and upstream projects needs a permissive license. A business needs
something to sell that does not cripple the open version.

What the licence has to do:

- **Be accepted where our first users are.** Research computing centres, public bodies and
  operators' legal teams accept OSI-approved licences routinely. Public open-source funding
  (the Prototype Fund) requires a FOSS licence. The Kubernetes ecosystem is Apache-2.0.
- **Protect what is hard to copy.** Our protection is the published evidence, the safety record
  and the reputation built in public, not the code: a funded company could rewrite the controller
  in a quarter. The name, and any "verified" mark on savings reports, are what others must not be
  able to use.
- **Keep the code assignable.** A company founded later, and any acquirer, must receive clean
  rights. With a permissive licence and a DCO on every contribution
  ([CONTRIBUTING.md](../../CONTRIBUTING.md)), no contributor licence agreement is needed: Apache-2.0
  already lets the copyright holder build closed work on top.

Alternatives considered:

| Licence | For | Against |
|---|---|---|
| Apache-2.0 | Standard for Kubernetes and the CNCF; patent grant; accepted by operators, public bodies and acquirers; FOSS | A competitor may ship our code inside a closed product |
| AGPL-3.0 | Competitors who ship modified versions must publish them | Many companies' legal teams forbid it, which hurts adoption by exactly the operators we target; funded competitors would rewrite rather than comply; keeping a dual-licence option would need a CLA |
| MPL-2.0 | File-level copyleft: changes to our files stay open | Little protection in practice; less familiar in the Kubernetes ecosystem |
| Source-available (BSL, FSL; open after 2–4 years) | Blocks competing commercial use for a period | Not open source: excludes the Prototype Fund, worries public operators, cannot join the CNCF; contradicts building trust in public |

## Decision
The repository is Apache-2.0: agent, controller, drivers, models, planner, guard, experiment
engine and analysis. A single cluster gets the full saving and its verification for free.
Commercial offerings, if any, sit outside this repository: a multi-cluster fleet view, signed
savings reports for contracts, support, and services.

Published data and the data format specification are CC-BY-4.0.

**The name is protected separately from the code.** "Wattproof" is registered as a trademark
before the live page goes public, and a trademark policy states that forks may use the code but
not the name, and that only reports signed by Wattproof may carry a Wattproof verification mark.

Every contribution is signed off under the Developer Certificate of Origin; CI checks it.

## Consequences
Anyone may run the product without paying. Revenue has to come from services, fleet features and
trust, not from locking the core. A competitor may reuse the code; it cannot reuse the evidence,
the name or the verification mark.
