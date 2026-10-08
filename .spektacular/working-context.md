# Working context — plan 000066_epic-mid-run-revisions

## Origin
- Spec 000066 finished and committed 2026-10-08 (commit 5517bd9, user approved auto-commit incl. unrelated tree changes).
- Spec drafted from defaults; user approved finishing it, so the spec's "open question (drafter's default)" bullets are taken as the accepted defaults: only the user-facing agent applies amendments; record = `## Amendments` section + plan changelog entry; recorded amendment exempts plan from staleness; amendable sections = requirements, acceptance criteria, constraints, success metrics.

## User direction
- Never commit unless asked (global rule); auto_commit is `full` so workflow `goto finished` commits — ask before that step.

## Code facts carried from spec session
- templates/steps/implement/11-reconcile_spec.md: implement only flips spec checkboxes.
- Plan staleness: internal/status/classify.go (~136) spec mtime vs plan mtime; strict mode refusal in cmd/implement.go refuseStalePlan (plan_stale).
- Orchestrated children hand back `QUESTION: <spec>` (spek-implement SKILL.md).
- `design author` keeps capture date + existing references on rewrite.

## Discovery learnings (plan)
- Repos: spektacular (CLI/templates/skills/tests) + docs (spektacular-website). No design refs on the spec.
- Chosen mechanism: `spec amend` CLI verb + modelled `amendments` frontmatter (body hash, checkbox-normalised); PlanIsStale exempts when current body hash == last amendment hash, else today's mtime check.
- Pre-existing: reconcile_spec likely stales unamended plans under strict mode — out of scope, flag to user in walkthrough.
- Test traps: epic skill `ask` allow-list (templates/implement_epic_skill_test.go:200-224); `{{command}}` regex; orchestrated wording only inside {{#orchestrated}}; QUESTION: only in spek-implement's last section; metadata exact-bytes tests.
- Never re-run init/migrate on this repo (convention: plans never change active install).
- Architecture locked: partial `templates/partials/implement-spec-conflict.md` in steps 02-05; `spec amend --data {name,reason,run,design?} [--from]`; frontmatter `amendments` {at,sections,design,hash}; PlanIsStale hash exemption.
- Tasks drafted: 10 tasks (M1: metadata, staleness, spec amend, strict e2e tests; M2: implement partial, spek-implement skill, epic skill; M3: README, docs implement/epics pages, docs reference pages). Docs tasks depend on the CLI tasks they describe.
- Plan written and approved by the user 2026-10-08 (walkthrough complete, user approved the finish auto-commit). Out-of-scope follow-up: reconcile_spec strict-mode staleness on unamended specs.
