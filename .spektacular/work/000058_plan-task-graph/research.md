## Alternatives considered and rejected

- **Stored `plan.json` / `tasks.json` alongside plan.md** — rejected: two sources of truth that drift; spec constraint "no separately stored export". The export parses plan.md at call time (design `design:plan-task-graph.md` §plan export).
- **Plan-local `T1` refs resolved/rewritten by the CLI on write** — rejected in the spec interview: the CLI would rewrite authored content and refs cannot express cross-spec dependencies. Ids are opaque, minted by `plan task-id`, never rewritten.
- **Implicit sequential dependency default** — rejected: `**Depends on:**` is required; explicit `none` separates "parallel" from "forgot".
- **Validating `plan.task_id.provider` at config load (`PlanConfig.Validate`, `internal/config/config.go:697`)** — rejected: a bad value would block every command via the `gate` (`cmd/gate.go:30`); the AC only requires an error naming the provider when an id is requested. Resolve at request time instead.
- **Config schema bump for `plan.task_id`** — rejected: `ParseYAMLFile` seeds `NewDefault()` then unmarshals (`internal/config/config.go:362-394`), so an absent optional key defaults cleanly; a bump (`internal/config/schema.go:15-21`) would force every project through `migrate`.
- **Adding `github.com/google/uuid`** — rejected: a v4 UUID is 16 `crypto/rand` bytes with version/variant bits set; `internal/identifier` already uses `crypto/rand` (`identifier.go:241-247`). No new dependency.
- **Reading `repo.location` from `git remote` or from a `file` source path** — rejected: spec forbids inference; design says "the repo's declared git source". A `file` provider source is a local path, not a canonical location.
- **Validating plan.md at the command surface in `cmd/`** — rejected in favour of a validator that holds the facts (task parser + repo registry) so errors name the task and list registered repos (knowledge `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`).
- **Deciding "last open task" purely in template prose** — rejected: current loop exit is prose-counted (`templates/steps/implement/07-update_changelog.md:55-75`); single-task scoping needs a Go callback that reads plan.md through the store and passes a flag via `Extra` (pattern: `internal/steps/plan/steps.go:305-313` `plan_incomplete`, `internal/steps/spec/steps.go:169-183`).
- **Separate ad-hoc regexes per consumer** — rejected: spec Technical Approach requires one reusable reader shared by export, status, implement and milestone commits.

## Chosen approach — evidence

- Single choke point for plan.md writes: `cmd/storefile.go:190-235` (`write` handler) — body stripped at :228, merged at :229, written at :233. Validation inserted between :228 and :233 leaves the prior file byte-identical (`FileStore.Write` = `os.WriteFile`, `internal/store/store.go:180-189`). `plan file` builder at `cmd/plan_file.go:8-15`; plan workflow never writes plan.md itself (`internal/steps/plan/steps.go:205-289`).
- Existing plan.md parsers to replace: `cmd/implement.go:18` (`uncheckedPhaseRegexp`, used :307-321 for `unchecked_phases`), `internal/autocommit/milestones.go:12,17,35` (`milestoneHeading`, `phaseCheckbox`, section `## Milestones & Phases`), consumed by `cmd/autocommit.go:301-311` `dueMilestones`.
- Id minting home: `internal/identifier/identifier.go:1-9` package doc ("single home for every ID-related rule"), switch-based `Resolve` :66-112, typed errors :185-202.
- `plan status <name>` → `runArtifactStatus` (`cmd/artifact_status.go:60-114`, result :25-38, schema :40-56), shared with spec; plan-only hook exists (`strictPlanStatusHook`, `cmd/plan.go:284-313`); raw bytes already read (:71) so parse once there.
- Repo lookup: `repo.New` / `Set.Entries()` / `Set.DescriptiveMetadata(name)` (`internal/repo/set.go:66,80,167`) give `RepoConfig.Source`; `RepoConfig.ParseSource` (`internal/config/repo.go:254-278`) distinguishes `SourceGit` (location as written) from `SourceFile`.
- Implement FSM: `internal/steps/implement/steps.go:22-37`; deferred goto from callbacks (`internal/workflow/workflow.go:169-202`); templates are mustache with `Extra` overriding vars (`internal/stepkit/stepkit.go:76-142,170-176`); only `name` is read from data automatically (:91).
- `implement new` order (`cmd/implement.go:74-174`): resume probe → parse `--data` (struct{Name}, unknown keys ignored, :127-135) → name check → plan exists (:141-145) → `refuseStalePlan` (:146) → `startGate` (:153) → `clearState` (:157) → workflow. Task pre-checks slot in after :146, before :153, so refusals touch nothing.
- `finished()` hard-fails `changelog_missing` when the feature changelog is absent (`internal/steps/implement/steps.go:167-177`) — must be bypassed for non-final single-task runs.
- Commit points: completion `reconcile_spec→finished`, milestone candidates `update_changelog→analyze|test_plan` (`internal/autocommit/points.go:122,133-136`, pinned by `points_pin_test.go:21-39`). A new `update_changelog→finished` edge needs a point.
- Error shape: `output.NewError(code,msg).WithResource().WithNextAction()` (`internal/output/writer.go:46-81`); convention `conventions/error-messages-must-suggest-remediation.md`.
- JSON output: `output.Write` (`internal/output/writer.go:98-161`); there is no non-JSON mode yet — `--format pretty` is the first text renderer; errors stay JSON.

## Files examined

- `spektacular:cmd/plan.go:41-70,219-313,334-347` — plan command tree, status, flag registration; new `export` / `task-id` subcommands go here.
- `spektacular:cmd/plan_file.go:8-15`, `cmd/storefile.go:173-424` — shared store file builder; write handler :190-235; `set-document-status` :360-418 (no validation needed).
- `spektacular:cmd/artifact_status.go:25-114` — named status result/schema shared by spec and plan.
- `spektacular:cmd/implement.go:18,74-174,176-239,262-323` — phase regex, new/goto/status.
- `spektacular:cmd/autocommit.go:57-177,194-238,296-311` — goto with commits, start gate, `committed_milestones`, `dueMilestones`.
- `spektacular:cmd/resume.go:27-91` — resume report renders `steps/resume_implement.md` with name + step only.
- `spektacular:cmd/root.go:50-65,113-119,252-261,347` — unknown-subcommand error, error envelope, gate.
- `spektacular:internal/autocommit/milestones.go`, `points.go`, `message.go:23-39` — milestone parsing, commit points, message validation.
- `spektacular:internal/steps/implement/steps.go:22-37,51-63,67-71,109-119,152-181`, `result.go:17-30`, `strategy.go:50-67` — FSM, callbacks, status result, template path vars.
- `spektacular:internal/steps/plan/steps.go:14-26,31-54,139-146,165-215,305-313` — plan FSM (`phases` step :43), assemble, path helpers.
- `spektacular:internal/workflow/workflow.go`, `state.go:14-21` — FSM engine, `state.json` shape (`data` map).
- `spektacular:internal/stepkit/stepkit.go:76-176` — template vars, mustache, auto-commit partial, working-context footer.
- `spektacular:internal/config/config.go:74-105,309-394,570-705`, `schema.go:15-21`, `repo.go:25-340` — config structs, defaults, validation, schema, repo source.
- `spektacular:internal/identifier/identifier.go` — id minting, typed errors.
- `spektacular:internal/repo/set.go:66-215` — registry lookup.
- `spektacular:templates/steps/plan/*.md` — 19 steps; `10-phases.md` (format :10-29, working files :40-43), `13-assemble.md:27,33,41`, `14-verification.md:15-25,52`, `18-walkthrough.md:16`; phase wording also in `03`,`06`,`08`,`09`.
- `spektacular:templates/scaffold/plan.md:109-133`, `scaffold/context.md:8-20` — scaffolds (plan scaffold lacks `**Repo:**` — existing drift).
- `spektacular:templates/steps/implement/01..12*.md`, `resume_implement.md`, `partials/implement-plan-documents.md`, `partials/git-commit-message.md:11-21` — phase-format parsing in prose.
- `spektacular:templates/skills/workflows/spek-plan/SKILL.md`, `spek-implement/SKILL.md:15,34,51,62`, `skill_update-changelog.md`, `skill_verify-implementation.md`, `skill_follow-test-patterns.md`, `skill_spawn-implementation-agents.md` — skills with phase wording.
- `spektacular:templates/agents/store-access.md:35` — "ticking a phase checkbox".
- Tests: `cmd/implement_test.go:19-66,203-313`, `cmd/milestones_test.go:28-95,171-293`, `internal/autocommit/milestones_test.go`, `internal/steps/plan/{scaffold,steps}_test.go`, `internal/steps/implement/steps_test.go:85-390`, `cmd/instruction_contract_test.go:65-87,517,545`, `cmd/autocommit_test.go:284`, `cmd/cross_kind_test.go:123`, `templates/work_files_test.go:48,136`, `cmd/plan_file_test.go:15-95`, `cmd/artifact_status_test.go:117-214`, `cmd/spec_test.go:26-47` (`writeSpecCommandConfig`), `cmd/root_test.go:35-76` (`resetRootCmd`, `runRootCmd`).
- Harbor: `tests/harbor/plan-workflow/tests/test_plan_workflow.py:61-82,90,199`, `solution/solve.sh:29,108-185`, `instruction.md:40`; `tests/harbor/implement-workflow/environment/plan.md` (legacy Phase fixture — keep as the old-plan regression).
- `spektacular:.spektacular/knowledge/glossary/phase.md` — glossary term to replace with "Task".
- `docs:src/pages/how-it-works.mdx:111-154,372-425` — only page explaining phases / plan commands.
- `docs:src/pages/configuration.mdx:52-55,190-203` — `plan` ConfigKey (add `plan.task_id.provider`).
- `docs:src/pages/design-documents.mdx:259-373` — CLI-section pattern (`Section` + `Prose nested` + fenced bash).
- `docs:src/components/Nav.astro:6-21` — hard-coded nav; new page must be added to the Resources children.
- `docs:src/components/sections/SpecFormat.astro`, `SpecKey.astro` — candidate components for the task format.

## External references

- GitHub issue #50 (hivecommons/spektacular) — Hive's request for `plan export <name> --format json`; defines the consumer's field names (`kind`, `name`, `tasks`, `id`, `title`, `depends_on`).
- Hive `src/pkg/spektacular/runner.go` / `src/docs/spektacular.md` (from spec interview, recorded in working-context) — decoder falls back `ref`→`id`, rejects empty `tasks`, requires `name` == requested name.
- RFC 4122 §4.4 — v4 UUID from random bytes (version nibble 4, variant 10xx).
- Issue #62 — parallel implement runs (out of scope).

## Prior plans / specs consulted

- Knowledge `architecture/testing-architecture.md` (records plan 000040's lesson) — any change to step names, templates, scaffolds, or store validation must update harbor oracles in the same change and verify with a harbor run.
- Knowledge `architecture/workflow-steps.md`, `architecture/working-with-files-from-steps.md` — step/callback pattern; everything through `store.Store`.
- Spec `000058_plan-task-graph` and its working-context interview notes — all format/shape decisions.

## Open assumptions

- **Legacy detection**: a plan.md with no `#### - [ ] Task:` headings (and no `## Milestones & Tasks` section) is a legacy plan: `plan file write` does not apply task validation to it (so implement's `update_plan` keeps working on old plans); export/single-task refuse it with `plan_structure_invalid`. If the user wants legacy writes refused, STOP.
- **`repo.location`** is set only for a `git` provider source; `file` sources (including the default `..`) export `""`. Both registered repos here declare `file` sources, so their location is empty today.
- **Changelog per single-task run** = the plan's inline `## Changelog` entry (07 template), not a changelog-store record; the store record is the feature-level summary produced only on the last task.
- **Unknown `plan.task_id.provider`** errors only on `plan task-id` (code `task_id_provider_unknown`), not at config load.
- **`plan status` without a name** (workflow status) is unchanged; per-task progress is added only to `plan status <name>`.
- Legacy Phase plans keep `unchecked_phases` in `implement status`; for task plans the same field counts unchecked tasks (field name kept per "existing fields unchanged").

## Rehydration cues

- `go run . design read --data '{"source":"design","path":"plan-task-graph.md"}'` — binding format/JSON/refusal design.
- `go run . spec file read 000058_plan-task-graph.md`.
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs`.
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'` (+ `workflow-steps.md`, `working-with-files-from-steps.md`, `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`).
- Re-read `cmd/storefile.go:190-235`, `internal/autocommit/milestones.go`, `internal/steps/implement/steps.go`, `templates/steps/plan/10-phases.md`, `templates/scaffold/plan.md`.
