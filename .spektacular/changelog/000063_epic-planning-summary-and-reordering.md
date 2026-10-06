---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# 000063_epic-planning-summary-and-reordering

## What was built

Planning an epic with one request ("plan this epic") now leaves one planning summary document per epic and orders colliding specs itself. Planning, whether for an epic or a single spec, now stops to ask whenever a plan would contradict something the user already decided.

- **Planning summary (`epic summary read/write`).** Each epic can hold `<epics>/<epic>/summary.md` in the epic store. It is rendered in a fixed order: Decisions to settle ("None." when empty), Order added for shared files, then one section per planned spec in epic list order. A write replaces exactly one section (`decisions` or a member spec), so repeated planning and review edits never disturb the others. `ordering`, unknown sections, non-members and bodies with `#` / `##` headings are refused, and reading a missing summary is refused with a pointer to "plan this epic". `epic delete` removes the summary with the epic, inside its transaction.
- **Automatic ordering (`epic order`).** It reads the files each planned spec's tasks name in the plan's context document (`plantask.TaskFiles`). When two specs share a file and nothing orders them, directly or transitively (`depgraph.Reaches`), it makes the later-listed spec depend on the earlier one without asking. Only the epic is written, so nothing is re-planned. Each addition is logged in the summary with the shared files. `--data '{"unorder":…}'` removes an addition and records the pair in a new optional `parallel_with` on the epic entry, so it is never re-added. `epic split` preserves `parallel_with`, and validation refuses one naming a non-member or the spec itself.
- **Questions while planning.** The "proceed unless genuinely blocked" paragraph, previously copied into eleven plan steps, is now one shared partial. It adds the rule that a choice contradicting a decision the user recorded for the spec (its sections, a referenced design, or its interview notes where they still exist) or a knowledge entry is always a stop to ask, never an assumption, a `human` task, an open question or a review note. Discovery names the recorded decisions; architecture and verification check against them; tasks and open questions forbid parking a contradiction. Under epic orchestration the stop becomes a `QUESTION:` while the other specs keep planning.
- **"Plan this epic" builds and walks the summary.** Orchestrated plan runs hand back a fifth summary point, the project-wide rules each plan relies on or decides. The `spek-plan-epic` skill writes each spec's section on `DONE:` and runs `epic order` when the loop ends. It lists every cross-plan rule disagreement under Decisions to settle with one proposed answer. The review then walks the summary decisions first, applying each change to both the plan and its section and settling disagreements across every plan involved.
- **Plans name their files reliably.** The tasks step requires every context-document file change to start with its backticked path.
- **Design and docs.** The epics design describes the new field, the summary, both verbs and how epic planning now ends. The public Epics page and site changelog document the summary, the automatic ordering and the new questions.

## Why it matters

Specs planned side by side could quietly collide. Two plans changed the same files with nothing ordering them, plans disagreed on a project-wide rule such as how the changelog is kept, or a plan departed from a choice the user had already made. These surfaced only as a dense list at the end of planning, or not at all. Now colliding specs are ordered deterministically, contradictions are raised while planning runs, and disagreements come with a proposed answer. Users get plans that can be implemented in parallel without merge conflicts, and one document they can review and return to.

## Deviations from the plan

- `epic split` rebuilt the split spec's own entry and dropped its `parallel_with`; the tests found this and `splitGraph` now carries the field over.
- The summary's folder is removed after `epic delete`'s transaction, on a best-effort basis, because the transaction can only remember files. A rollback that restores the summary recreates the folder anyway.
- The overlap reader's path rule was made concrete. A token must contain `/` or end in a lower-case extension; directories (a trailing `/`) and anything in fenced code are skipped.
- The epics design's Process step 3 was updated as well as "Epics → Planning", since both said "Unchanged".
- The docs cross-reference targets `#plan-this-epic`, because `Section` headings carry no id.
- The plan's open question was resolved without a stop: on this repo's pre-change plans the reader finds files for nearly every task.
