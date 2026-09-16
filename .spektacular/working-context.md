# Working context — plan 000052_document-status-vocabulary

## Carried from the spec discussion
- User decisions: rename field `status` -> `document_status`; values draft/final/superseded/archived; no backward compat or migration; unknown/old values read as blank (never an error); CLI input strict; flags renamed `--document-status`, command `set-status` -> `set-document-status`, list JSON key `document_status`; blank = open but distinct from draft; writes without flag preserve blank; closed_date kept.
- Out of scope: rewriting this repo's ~93 existing artifacts (user does separately); website only needs the example in `src/pages/projects.mdx` (docs repo root /home/nicj/code/github.com/jumppad-labs/spektacular-website).
- User prefers driving workflows straight through, stopping only for real design decisions.
- Implement workflow closes test-plan and changelog as well as plan docs (internal/steps/implement/steps.go:159,168).

## Plan walkthrough
- User approved plan 000052_document-status-vocabulary without changes (all drafting assumptions accepted).
- Spec renamed from 36-document-status-vocabulary to 000052_document-status-vocabulary: spec.id_method is `counter`, and plan writes require the counter ID prefix. Passing an external `id` to `spec new` under counter mode produced a non-conforming name.

## Implementation (implement workflow)
- read_plan passed: structure valid, full spec coverage, no Changelog yet (first-phase run).
- Minor drift to adapt during implementation: `tests/harbor/implement-workflow/environment/plan.md:3` also has `status: in-progress` (update it too); website `how-it-works.mdx:389` prose "status and dates" should become "document status and dates".
- Phase 1.1: in cmd/, the two enum switches already call `metadata.ParseDocumentStatus` (the flag names and the `invalid_status` code are unchanged until 1.2). Lenient decode uses a separate `yamlInShape` with `DocumentStatus yaml.Node`. `validateDocumentStatus` rejects blank, so Merge/Close can never set blank explicitly.
- The test sweep was a perl word-boundary rename (in-progress->draft, completed->final). The `"marked completed"` assertion in internal/steps/plan/steps_test.go was left for Phase 2.1.
- Phase 1.1 tests: a sub-agent added a no-legacy-`status:`-line check to the TestMerge loop; every criterion is covered.
- Phase 1.1 verified: build, make test/lint, vet, gofmt, and a scratch-project smoke test all passed. Smoke procedure: build the binary into the scratchpad, then `spek init claude --name x`.
- Writing plan.md through the CLI drops its legacy `status: completed` key, so the plan now shows `document_status: ""`. This is expected under the no-migration rule.
- Phase 1.1 changelog written. Looping phases without asking, per the user's preference to drive straight through.
- Phase 1.2: shared helpers `parseDocumentStatusFlag` and `documentStatusValues` live in cmd/artifactfilter.go. Flag help and next_action text derive from `metadata.DocumentStatuses()`. CHANGELOG.md (a historical release note) is left untouched.
- Phase 1.2 deviation: the `<kind> file` group lacked the `runUnknownSubcommand` guard, so a bare `file set-status X` printed help and exited 0. Added `RunE: runUnknownSubcommand` to fileCmd in cmd/storefile.go, with a test.
- Pre-existing, not ours: `go test -shuffle=on ./cmd` fails intermittently in the order-dependent implement/goto tests.
- Phase 1.2 is done and its changelog written. Knowledge offer to raise: every cobra group needs `runUnknownSubcommand`.
- Phase 2.1 done. The harbor implement suite has NOT been run: it is billable and manual. The user decides; the criterion stays unchecked.
- Phase 2.2 done. The docs repo had unrelated uncommitted edits (unknown-criteria.mdx, knowledge-base.mdx) that were left alone.
- Test plan written: a manual agent check and the harbor implement run.
- Feature changelogs written: the project record plus the spektacular and docs repo records.
- Spec reconciled. The end-to-end AC 'Workflows close as final' stays unchecked until the harbor run.
