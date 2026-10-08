# Working context — spec 000066_epic-mid-run-revisions

## Origin
- GitHub #78, second comment, item 6: "A design decision made during implement has no clean path." During spec A's verify step the design's rule disagreed with the spec's success metric; the user changed the design mid-run. Revising from the project root didn't reach the child's worktree; writing into the worktree was refused at merge; the spec's success metric couldn't be updated from the implement workflow and is now out of date, with only the plan changelog saying so.
- Reviewed against shipped 000064/000065: children now run from the project root and read specs/plans/designs from the project store, so a design revised in the project *is* visible to a child. What remains: (a) no defined path for the orchestrator (or an interactive implement run) to revise a design or spec mid-run, (b) the implement workflow cannot amend the spec's success metrics / requirements — reconcile_spec only ticks checkboxes, (c) no record of who changed the spec mid-run and why.

## User direction
- User: "generate the spec and I will review it tomorrow" — no questions asked during drafting; open decisions defaulted and listed as open questions in the spec.
- Scope: item 6 only. #80 and #78 items 1/4/5 are being fixed separately.
- Never commit (user rule). auto_commit is full; stop before `finished`.

## Code facts found
- templates/steps/implement/11-reconcile_spec.md: implement only flips spec checkboxes [ ]→[x]; never edits text.
- Plan staleness (internal/status/classify.go:136) is spec mtime vs plan mtime; under plan.strict_spec_changes `implement new` (incl. resume of an orchestrated lane) refuses with plan_stale (cmd/implement.go refuseStalePlan). A mid-run spec amendment would therefore block resuming the run under strict mode.
- Orchestrated children never ask the user; they hand back `QUESTION: <spec>` (spek-implement SKILL.md:152); a genuine question includes "the plan no longer matching the code" and verification failures the child cannot fix (spek-implement-epic SKILL.md).
- `design author` keeps capture date and existing spec references when rewriting.

## Defaults chosen (to flag as open questions)
- Only the orchestrator (epic) or the user-facing implement agent (interactive) applies an amendment, always after the user's explicit approval; a child never edits spec text itself.
- An amendment is recorded in a new `## Amendments` section of the spec (date, which section changed, why, which run) plus a plan-changelog entry.
- An amendment recorded this way does not make the plan stale.

## Status
- All sections drafted from defaults (no user Q&A), self-reviewed (fork could not spawn a reviewer subagent), and written to the store via `spec file write`. Working dir removed.
- Split check: one weak signal (9 requirements), counter-signal applies (amendment stop, apply path, record and staleness exemption are tightly coupled) → no split offered.
- Workflow resumed at `split` (2026-10-08): split check re-confirmed, no offer. User explicitly approved advancing to `finished` with the auto-commit, including unrelated uncommitted changes in the tree. Next: plan this spec via spek-plan.
