---
created_date: "2026-10-08"
document_status: final
closed_date: "2026-10-08"
---

# 000066_epic-mid-run-revisions: correcting a spec or design mid-run

## What was built

Implement runs now have a defined path for a spec, or a design it references, that turns out to be wrong while it is being built.

- **A stop.** The implement workflow's analyze, implement, test and verify steps share a stop. When a requirement, acceptance criterion, constraint or success metric is wrong or contradicts another, or contradicts a rule in a referenced design, the run stops. It names the document, section, conflict and proposed amendment, and does not work around it.
  - An interactive run asks the user and applies an amendment only after explicit approval.
  - An orchestrated child never writes spec or design text. It hands the proposal back with `QUESTION:`, and the epic orchestrator applies the approved amendment from the project root and answers the child.
  - Either way, the run re-reads the amended text and re-runs the step's check of the current task.
- **`spektacular spec amend`.** This is the one supported way to make the change.
  - It takes `--data '{"name","reason","run","design"?}'` and the full amended spec via `--from`.
  - It works out which sections changed and refuses anything outside Requirements, Acceptance Criteria, Constraints and Success Metrics: the preamble, other sections, moved sections, and edits to past amendments.
  - With `design`, it records a revision already made to a referenced design, with `design author` or, for a user-supplied design, `design write`.
  - It appends a dated `## Amendments` entry naming what changed, the run and the reason. It records an `amendments` entry in the spec's frontmatter with a checkbox-normalised SHA-256 of the body, in one write.
  - Every refusal names a corrective next step.
- **Staleness.** `status.PlanIsStale` treats a spec whose body still matches its last recorded amendment as unchanged. Under `plan.strict_spec_changes`, a recorded amendment no longer marks the plan stale or blocks `implement new`/`goto` or an orchestrated lane. Any unrecorded edit, before or after an amendment, still does. Frontmatter-only writes and checkbox ticks do not undo an amendment.
- **Records.** The plan changelog entry format gains an `**Amendments**` line.
- **Skills.**
  - `spek-implement` describes the interactive path, and its orchestrated section forbids a child from amending.
  - `spek-implement-epic` counts a wrong spec or design as a genuine question, restates the rule in the child prompt, names `design` and `spec amend` in its store-access rule, and describes applying an approved amendment.
- **Documentation.**
  - The README describes the strict-mode exemption.
  - The docs site explains the path on the how-it-works and epics pages, lists `spec amend` on the documents page, and notes mid-run design corrections. It documents `plan.strict_spec_changes` on the configuration page and the exemption on the plan-tasks page.

## Why it matters

Before this change, an implement run that found its spec or a design wrong had no defined way to correct it:
- the workflow could only tick checkboxes;
- the epic orchestrator had no step for revising documents;
- the only record of the disagreement was buried in the plan changelog;
- under strict spec changes, any edit stalled the run.

Now the user approves the correction, it lands in the project rather than a worktree, so epic merges are not refused, the spec explains its own history, and the run carries on. This closes item 6 of issue #78.

## Deviations from the plan

- `spec amend` treats a section that moved relative to the others as changed, so a reorder cannot ride along with a legal amendment unrecorded. A test author found the gap.
- Outside the plan, at the user's request, 13 existing `cmd` tests now start in their own temp directory. The CLI refuses to run inside a spec's worktree, so those tests failed whenever the checkout sat in one.
- The README task gained a pin test following the existing README tests.
- The how-it-works page links to the Epics page by name rather than by fragment, matching the site's link style.

## Test plan

Manual verification is recorded in the plan's test plan:
- a real epic run with a seeded spec/design conflict (no hand edits, no `override_dependencies`, no worktree `.spektacular` writes, a clean merge, and a self-explaining `## Amendments` section);
- a harbor implement-workflow run;
- a read-through of the docs pages.
