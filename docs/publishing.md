# What we publish, and when

Status: in force since 2026-10-09.

Wattproof works in public: the design, the research, the protocols, the code and every result,
including the negative ones. Evidence an operator can check is the product's argument, so this
stays the default. Three rules set the limits.

## 1. Datasets are published with their write-up

A dataset from our own measurements (a calibration, a Phase 1G node, an experiment) is published
together with our analysis of it, as one release: the lab notebook entry or report, the raw data
with its DOI, and the code that reproduces every number.

- **Latest date:** eight weeks after the measurement ends, whether or not the write-up is finished.
  A dataset is never held back because of what it shows.
- **Registered experiments** follow their protocol: the result and the data are published whatever
  they show, on the date the protocol states.
- **The live lab page** ([ADR-0009](adr/0009-public-live-lab.md)) is not affected: it shows
  aggregated live data from the testbed as it runs.

## 2. Partners are named only with their consent

The name of an operator, research centre or company we measure with, test with or talk to appears
in public only after they have agreed in writing, and only in the form they agreed to. The same
holds for dates and sites that would identify them. Until then, public documents say "an
operator", "a research computing centre" or "a GPU operator".

Citing an organisation's own publications is not affected.

## 3. Facts are public; competitive judgements are not

Public documents state what each tool does, what our simulations and measurements show, and what
we decided and why, with sources. They do not carry our assessment of where we lead or trail a
competitor, or of how a competitor might respond. Those belong to business planning, which is
not part of this repository.

## What is never published

Prices, the sales pipeline, customer data, serial numbers and internal addresses
([ADR-0009](adr/0009-public-live-lab.md)), and anything a partner has asked us to keep private.
