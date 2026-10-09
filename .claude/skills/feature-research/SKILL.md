---
name: feature-research
description: Check the literature, the newest findings and existing tools before a Wattproof feature is designed or built, and record what it means for the design and the expected effect. Use whenever a feature, lever, policy or roadmap item is proposed, designed, started or significantly changed.
---

# Feature research

No feature is designed before we know what research and other tools already found. The result is
a public note, because the evidence base is part of the product.

## 1. Frame the feature

Write down in three lines:

- the lever: which energy or capacity it changes, and through what mechanism;
- the claim we would like to make if it works;
- the strongest alternative an operator already has (for example VPA, Cluster Autoscaler,
  descheduler, a commercial tool).

## 2. Search

Search the web in parallel. Use mode "extended" for findings from the last 24 months, and
"standard" for well-known references.

- **Research**: peer-reviewed papers and preprints (ACM, IEEE, USENIX, arXiv), theses, and
  measured industry reports. Prefer measured results on real hardware over simulations.
- **Newest findings**: anything from the last 24 months that changes the picture, including
  Kubernetes enhancement proposals (KEPs) and release notes.
- **Tools**: open-source projects and commercial products that do it. For each: licence,
  activity, whether it reports measured savings, and whether it runs on bare metal.
- **Negative results**: papers or retrospectives where the lever did not pay off, and why.

Read the primary source, not a summary of it. Record each source with its date and what it
measured (hardware, workload, baseline, effect, uncertainty).

## 3. Judge

Answer, each in a few lines:

1. **Is it new?** Novel, an improvement on prior art, or prior art we are packaging.
2. **What effect does the evidence predict**, against our B0 and B1 arms? Give a range with its
   source. If no source measures it, say so.
3. **What does it change in the design**: parameters, invariants, safety limits, measurement.
4. **What does it change in the experiment**: endpoints, effect-size prior, sample size.
5. **Advantage**: does it widen our lead over the strongest alternative, or only match it?
   **This answer is private.** Write it in `private/research/advantage.md` under the feature's name
   and date, never in the public note. The public note keeps the facts it rests on (what each tool
   does, with sources) and says only "Advantage: assessed privately".
6. **Go, change or drop.**

## 4. Record

- Write `docs/research/features/<feature>.md` with the frame, the findings with sources, and the
  judgement. Date it. This file is public, so it follows the publishing rules
  ([docs/publishing.md](../../../docs/publishing.md)): no prices, prospects or business plans; no
  advantage judgement; no partner names or dates before the partner agrees.
- Add the important sources and any new tool to `docs/research/related-work.md`.
- If the judgement changes a decision, propose an ADR in `docs/adr/` or an edit to `ROADMAP.md`.
- If it changes the business picture (a competitor already ships it, or the effect is larger or
  smaller than assumed), say so in chat and suggest running the `product-verdict` skill.

## 5. Report

In chat: the go, change or drop decision, the predicted effect with its source, and the design
changes. Link the note.
