---
created_date: "2026-10-01"
document_status: final
closed_date: "2026-10-01"
---

# Research: 000060_epics-and-seeded-specs

## Alternatives considered and rejected

- **Extend the shared `metadata.Metadata` to carry an epic's frontmatter.** Rejected: an epic's
  `specs` is a list of `{name, depends_on}` while `Metadata.Specs` is the design back-link list of
  bare names (`internal/metadata/metadata.go:85`); `decodeSpecNames` (`metadata.go:215-230`)
  silently decodes a mapping list to nothing, so any epic rewritten through `metadata.Merge`
  would lose its graph. The design itself says epics get "their own frontmatter type"
  (design `epics-and-seeded-specs.md`, *Epic document*). Chosen: a separate epic type in a new
  `internal/epic` package, reusing the lifecycle helpers.
- **Reuse `newStoreFileCmd` for the epic verbs (`epic file read/write/...`).** Rejected: it gives
  the `file` sub-level the design does not use (`epic read / write / list / delete`) and routes
  writes through `metadata.Merge`, which drops epic `specs` (`cmd/storefile.go:233-290`). The
  design family (`cmd/design.go:15-25`) is the precedent for a hand-written verb set.
- **Optional `epic` config key with no schema bump** (the precedent in 000054/000057/000058, which
  added defaulted keys with no migrate step). Rejected for this feature: the spec's AC
  "Store configured for new and existing projects" requires an existing project to have the
  `epic` block *after migration*, and idempotency on a second migrate. Chosen: `project3to4` plus
  `CurrentProjectSchema` 3 -> 4 (`internal/config/schema.go:15-21`,
  `internal/migrate/steps_project.go:90-134`).
- **`spec new --from <issue-url>` / staged source file / tracker client in the CLI.** Rejected by
  the spec's constraints (CLI never fetches; seeding's only CLI surface is accepting `sources`)
  and recorded as rejected in the spec session's working context.
- **Make `split` reachable from every section step (wide `Src` lists, or a generic engine
  feature).** Rejected in the walkthrough: a split always acts on a complete spec, so a
  mid-workflow request is recorded and acted on at the `split` step. The engine has no wildcard
  source anyway (`internal/workflow/workflow.go:188`).
- **Splits write stub specs that each go through their own workflow run, with chaining between
  stubs** (the original design). Rejected in the walkthrough by the user: when a complete spec is
  split, every requirement and constraint is already written and agreed, so the resulting specs
  can be written complete in one operation. Work that starts as a set of items uses an epic-first
  route instead, where each spec is built from new in the epic.
- **Drop `epic split` and have the agent compose `spec file write` plus `epic write`.** Rejected in
  the walkthrough: the CLI must own ID allocation, two-way links and an all-or-nothing write across
  the epic, the narrowed spec and the new specs. A multi-command agent sequence can stop halfway.
- **Put the split check inline in `09-finished.md`.** Rejected: `finished` is terminal (no
  footer, `InProgress()` false once reached), so a split accepted there could not narrow the spec
  inside the workflow, and the check text would be duplicated for the explicit-request path.
  Chosen: a `split` step on the linear path between `verification` and `finished`, its text in
  partials shared with the `spek-new` skill.
- **Warn-and-continue at implement as an interactive prompt from the CLI.** Rejected: the CLI is
  non-interactive JSON. Chosen: `implement new` refuses with a structured `dependencies_unmet`
  error whose next_action is the same command plus `"override_dependencies": true`; the agent
  asks the user; strict mode refuses the override too. Precedent: `refuseStalePlan`
  (`cmd/implement.go:253-272`) and `refuseUnstartableTask` (`cmd/implement.go:278-328`).
- **CLI-written changelog entry for a dependency override.** Rejected: no CLI-side changelog
  writer exists; all changelog content is agent-authored from templates
  (`templates/steps/implement/07-update_changelog.md`, `10-update_feature_changelog.md`). Chosen:
  persist the override in workflow data and render it into the changelog steps via `Extra`.
- **A `stub: true` frontmatter marker, or stub detection from draft status.** No longer needed:
  splits write complete `final` specs (`metadata/close.go:16-30`), so there are no stubs.
- **Keep `spec status` etc. as deprecated aliases.** Rejected by the spec's constraints (removed
  outright). Retired spellings join the instruction-surface deny-list, as 000059 did.
- **Reusing `plantask.findCycle` directly.** It is typed to `plantask.Task`
  (`internal/plantask/validate.go:120-164`). Chosen: extract a generic cycle finder into a small
  `internal/depgraph` package used by both plantask and epic validation.

## Chosen approach — evidence

- Closed frontmatter schema and the tri-state list pattern: `internal/metadata/metadata.go:62-230`,
  `internal/metadata/merge.go:6-148` (`Designs *[]DesignRef` carry-forward at 112-120). `epic` and
  `sources` on specs follow `Designs` exactly; `Epic` needs a pointer so an unlink can clear it.
- Two-way link with compensating rollback: `cmd/design_ref.go:173-319` (`writeRefs`,
  `writeBackLink`, `applyRef`, codes `design_ref_backlink_failed` /
  `design_ref_backlink_rollback_failed`, seam `writeBackLinkFn` :269).
- Delete-while-referenced refusal: `cmd/design.go:440-537` (`refuseIfReferenced`).
- Graph validation order (per-item, duplicates, unknown deps, cycles): `internal/plantask/validate.go:26-164`.
- Config: `internal/config/config.go` (`SpecTriggerThreshold*` 22-26, `PlanConfig.StrictSpecChanges`
  101, `Config` 284-308, `NewDefault` 321-357, `ParseYAMLFile` 375-407 prefill 390-392,
  `storeDirs` 430-436, `Validate` 583-627, default-dir map 688).
- Migration: `internal/migrate/registry.go:28-68`, `steps_project.go:90-134`, `node.go:39-80`,
  `engine.go:242-316` (early return on `from == want` gives idempotency).
- File store creates parent dirs on write: `internal/store/store.go:180-189`.
- Artifact kinds: `internal/artifact/address.go:23-27`.
- ID allocation: `internal/identifier/identifier.go:66-297`; spec wrapper
  `internal/steps/spec/identifier.go:33-52`; `spec new` allocates at `cmd/spec.go:230-246`.
  Counter IDs scan only the spec dir, so `epic split` must allocate and write new specs one at a time.
- `spec new` today: `cmd/spec.go:161-274` (unknown `--data` fields silently ignored; an existing full
  name mints a fresh prefix; the `new` callback overwrites with the scaffold at
  `internal/steps/spec/steps.go:78-89`).
- Template vars come only from strategy PathVars plus callback `Extra`
  (`internal/stepkit/stepkit.go:82-147`); partials via `{{> partials/<name>}}` (:190); skills render
  partials too (`internal/agent/skills.go:44-69`).
- `goto` copies `--data` keys into workflow data (`cmd/autocommit.go:84-88`).
- Status building blocks: `cmd/artifact_status.go:26-230`, `cmd/plan.go:286-316`
  (`strictPlanStatusHook`, `strictPlanIsStale`), `cmd/plan_export.go:52-131`,
  `internal/plantask/export.go:12-116`, `internal/plantask/plantask.go:122-346`.
- Workflow state: `internal/workflow/state.go:14-27`, `cmd/resume.go:104-131`.
- Implement insertion point: after `refuseStalePlan` (`cmd/implement.go:150`) and before
  `startGate` (:162); refusal writes no state.
- Autocommit commit-point table: `internal/autocommit/points.go:30` (spec `verification -> finished`).
- Spec-trigger threshold read live by the agent: `templates/agents/spec-trigger.md:11-15`.

## Files examined

- `spektacular:internal/metadata/metadata.go:62-230` — closed schema, decode helpers.
- `spektacular:internal/metadata/merge.go:6-148` — UpdateOptions tri-state, Merge carry-forward.
- `spektacular:internal/metadata/frontmatter.go:16-70` — Split/Render hard-wired to Metadata.
- `spektacular:internal/metadata/merge_test.go:29-220`, `metadata_test.go:747-945` — per-field test pattern.
- `spektacular:internal/config/config.go:16-821` — store config, thresholds, storeDirs, Validate, ToYAMLFile.
- `spektacular:internal/config/schema.go:15-21` — `CurrentProjectSchema = 3`.
- `spektacular:internal/migrate/{registry,steps_project,node,engine}.go` — migration steps and idempotency.
- `spektacular:internal/migrate/{registry_test,engine_test,steps_project_test,helpers_test}.go`, `testdata/` — pinned schema numbers, goldens.
- `spektacular:cmd/gate.go:30-94` — upgrade gate (self-hosting trap after the bump).
- `spektacular:cmd/{gate_test,migrate_test,version_test}.go` — hard-coded `schema: 3`.
- `spektacular:internal/store/store.go:84-253` — FileStore; MkdirAll on write.
- `spektacular:internal/artifact/address.go:23-200` — kinds and addressing.
- `spektacular:internal/identifier/identifier.go:49-309` — timestamp/counter/external allocation.
- `spektacular:cmd/storefile.go:48-514` — generic store verbs; `provenanceOpts` 166-178.
- `spektacular:cmd/storefile_address.go:71-122` — `listCommand` assumes `file list`.
- `spektacular:cmd/design.go:15-646` — hand-written verb family, `--schema`, delete refusal.
- `spektacular:cmd/design_ref.go:102-498` — two-way link maintenance and rollback.
- `spektacular:internal/plantask/validate.go:26-175` — graph validation; `RequireTasks`.
- `spektacular:internal/plantask/plantask.go:22-346`, `export.go:9-116` — plan parsing, export, pretty renderer.
- `spektacular:cmd/artifact_status.go:26-230` — named status, current_step resolution.
- `spektacular:cmd/plan.go:28-348` — plan status, strict hooks.
- `spektacular:cmd/plan_export.go:19-136` — export command, `--format`, `repoLocations`.
- `spektacular:cmd/spec.go:33-426` — `spec new`, `spec status`, schema types.
- `spektacular:cmd/implement.go:30-432` — implement new/status, help text, refusals, hint at :292.
- `spektacular:internal/steps/{spec,plan,implement}/result.go` — StatusResult types (removed with the commands).
- `spektacular:internal/workflow/{workflow,state,data}.go` — FSM, before_ callbacks, Goto, state.
- `spektacular:internal/steps/spec/steps.go:18-200` — spec steps, `new` scaffold write, `finished` close, `specStillScaffold`.
- `spektacular:internal/steps/implement/steps.go:31-249` — implement steps, read_plan, finished.
- `spektacular:internal/stepkit/stepkit.go:82-190` — rendering, Extra, partials.
- `spektacular:internal/autocommit/points.go:30` — commit points.
- `spektacular:templates/steps/spec/00-new.md … 09-finished.md` — spec step instructions.
- `spektacular:templates/steps/implement/01-read_plan.md, 07-update_changelog.md, 10-update_feature_changelog.md`.
- `spektacular:templates/partials/*` — existing partials.
- `spektacular:templates/skills/workflows/{spek-new,spek-implement}/SKILL.md` — skill sources (`.claude/skills`, `.bob/skills` are generated).
- `spektacular:templates/agents/{spec-trigger,store-access}.md` — managed AGENTS sections.
- `spektacular:internal/agent/{instruction_surface_test,store_access_test,skills}.go` — template constraints.
- `spektacular:templates/{section_drafting,rejection_repair_directive,work_files,skill_resume}_test.go` — template contracts.
- `spektacular:cmd/instruction_contract_test.go:36-696` — step table, footers, prefixes.
- `spektacular:cmd/{artifact_status,plan_status_progress,plan_export,status_address,cross_kind,implement,implement_task,root}_test.go` — tests of removed commands.
- `spektacular:cmd/root.go:34-366`, `cmd/root_test.go:35-108,1177` — registration, test helpers, subcommand pins.
- `spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py:1072-1081` — harbor `plan export` assertion.
- `spektacular:README.md:29-38` — status section.
- `docs:src/components/Nav.astro:6-23` — the only nav registry.
- `docs:src/pages/how-it-works.mdx:97-416` — workflow pages; status mentions at 142-144, 183-185.
- `docs:src/pages/plan-tasks.mdx:166-376` — `plan export`, `plan status`, `implement status` sections.
- `docs:src/pages/documents.mdx:223-242` — status commands' address output.
- `docs:src/pages/configuration.mdx:33-148,197-280` — config example and keys (fourteen-key list at 90-93).
- `docs:src/pages/design-documents.mdx` — model page structure for a new epics page.
- `docs:src/pages/index.mdx:41`, `src/content/tutorials/getting-started.mdx:49-51,853` — "implement the plan" wording.

## External references

- None beyond the repos. Tracker fetching is deliberately tool-agnostic (agent's own tools).

## Prior plans / specs consulted

- `000058_plan-task-graph` (plan) — single plan reader in `plantask`; implement pre-checks run
  before the start gate so a refusal writes no state; `plan export` pretty/json renderer; subcommand
  pins in `cmd/root_test.go`.
- `000053_config-schema-versioning-and-migrations` (plan) — one registered step plus a constant
  bump; self-hosting trap (this repo's config is refused by the gate after the bump until
  `migrate` runs); large fixture sweep via shared config helpers; `fileForm` breaks whole-Config
  equality.
- `000054_project-level-design-documents` (plan) — new artifact class precedent; closed schema
  lockstep; hand-written command family.
- `000055_design-authoring-skill` (plan) — back-link tri-state, compensating rollback with two
  codes, byte-exact "no key when empty" guards; directory replacement instead of chmod for I/O
  failure tests.
- `000056_store_delete_and_knowledge_maintenance` (plan) — delete refuses while referenced and
  names each runnable fix.
- `000057_git-commit` (plan) — threshold-style enum config; autocommit commit-point table and its
  pin test; start-gate ordering.
- `000059_normalise-artifact-addressing` (plan) — hard break plus deny-list for retired spellings;
  outputs carry addresses and config-relative paths, never host paths; `init` rewrites this repo's
  config when regenerating skill copies (restore it).
- `000060_epics-and-seeded-specs` (spec, the subject) and its design `design/epics-and-seeded-specs.md` (binding).

## Open assumptions

- "Implemented" for a legacy-format plan (no task structure) is computed from its open phase
  checkboxes (`plantask.OpenItems`); a plan with no checkboxes at all counts as `planned`.
- Sources are written to the spec's frontmatter by the `new` step;
  `spec file write` carries them forward, so they survive verification.
- After a split the epic takes the split spec's name; an epic created first (epic-first route) is
  named with the configured spec ID method run against the epic directory.
- The new `status` command reports only spec/plan/implement workflows; the separate repo guided-add
  state file is out of scope.
- The harbor suites are run manually (not in CI) and need updating for the step order, `spec new`
  schema and the `status` rename.
- Once `CurrentProjectSchema` is 4, this repo's own `.spektacular/config.yaml` must be migrated
  (with the user's go-ahead) before `go run .` works here again.

## Drafting assumptions

### Chosen direction: one owner package per rule, built on the design (architecture)
- **Decision**: implement the design with five areas: `internal/epic` (+ `internal/depgraph`) for the epic type, validation and links; `internal/status` for resolution, spec classification and the report shared by `status`, implement's dependency check and chaining; a `split` step plus `partials/split-check.md` and `partials/split-flow.md` shared with the `spek-new` skill; instruction-only seeding with `spec new` accepting `sources` and `epic`; docs pages in the docs repo.
- **Key design decisions**: separate epic frontmatter type (no `metadata.Merge` for epics); hand-written `epic` verbs (`read/write/list/delete/split`); `epic split` takes a staged JSON description via `--from`; `split` sits only on the linear path `verification -> split -> finished` (revised in the walkthrough; a mid-workflow request is acted on there); the dependency check refuses with `dependencies_unmet` and is overridden by re-running with `override_dependencies: true` (refused under strict mode); old status/export commands are removed and deny-listed.
- **Rationale**: every rule has exactly one home, so `status`, implement and chaining agree by construction, and the split check cannot drift between the step and the skill.
- **Rejected**: extending `metadata.Metadata` for epics; `epic file` through `newStoreFileCmd`; a split check inlined in `finished`; a split reachable only via the skill; `split` reachable from every section step (see research.md alternatives).

### `epic split` input format (architecture)
- **Decision**: a staged JSON description in `.spektacular/tmp/`, passed with `--from`, carrying every section of every resulting spec; the CLI renders each spec from the scaffold and writes it complete (revised in the walkthrough).
- **Rationale**: the spec leaves the format to the planner and asks for the one that matches other writes; `--from` is the only supported body channel, and a multi-spec description is too large and quote-heavy for `--data`.
- **Rejected**: `--data` inline JSON (fragile at this size); a parsed markdown epic draft (needs a second parser for every spec's sections).

### Conventions selected (architecture)
- **Decision**: apply the four spektacular conventions plus the glossary, and the five docs conventions (MDX rules, no em dashes, content outlines, layout/alternating surface, file-scoped headings) to the docs tasks.
- **Rationale**: the work touches errors, stores, cmd tests and the docs site; each applies directly.
- **Rejected**: none dropped.

### Schema bump for the epic store (discovery)
- **Decision**: add a `project3to4` migration and bump `CurrentProjectSchema` to 4, writing `epic` and `epic_split_threshold` defaults explicitly.
- **Rationale**: the spec's AC requires existing projects to have the epic store after migration, and a second migrate to change nothing.
- **Rejected**: the defaulted-key-without-migration precedent (000054/57/58): it would not write the key into existing configs.

### Splits write complete specs; work that starts as items goes epic-first (walkthrough, user decision)
- **Decision**: a split always acts on a complete spec (a mid-workflow request waits until completion), redistributes its content, reviews every resulting spec once, and `epic split` writes them complete and `final`. A source with child items creates the epic first, then each child is specified as its own spec in it. No stubs, and chaining offers the next source item without a spec.
- **Rationale**: the user's point that a complete spec already holds every requirement and constraint, so re-interviewing is waste; the spec and design were updated to match.
- **Rejected**: stub specs with their own later workflow runs; dropping `epic split` for agent-composed writes.

### Dependency override is a re-run with an explicit flag (discovery)
- **Decision**: `implement new` refuses with `dependencies_unmet`; the agent asks the user and re-runs with `"override_dependencies": true`; strict mode refuses even then.
- **Rationale**: the CLI is non-interactive; this keeps the user's decision explicit and recordable.
- **Rejected**: CLI-side prompting; silently continuing with only a printed warning (no recorded decision).

### Epic naming (discovery, revised in the walkthrough)
- **Decision**: after a split, the epic takes the split spec's name; an epic created first (epic-first route) is named by `epic write` with the configured spec ID method run against the epic directory.
- **Rationale**: the design's Decision 2 (no renaming) for splits; an epic created before any spec cannot take a spec's name.
- **Rejected**: having the agent construct epic IDs by hand; naming the epic after a spec created later (it would have to be renamed).

### Completed-epic additions need confirmation (walkthrough, user decision)
- **Decision**: adding a spec to a completed epic (via `spec new`, `epic write` or `epic split`) is refused with `epic_complete` unless re-run with `confirm_completed_epic: true`; completion stays derived, so the epic reads as in progress again automatically.
- **Rationale**: the user asked for a warning with the option to proceed; the CLI is non-interactive, so this mirrors the dependency override.
- **Rejected**: storing a completion flag that would need resetting.

### Spec delete guarded while in an epic (components)
- **Decision**: `spec file delete` refuses while the spec names an epic; `epic delete` clears every member's `epic`.
- **Rationale**: the "membership stays in agreement" AC; precedent `design delete` refusing while referenced.
- **Rejected**: letting the epic list a deleted spec (status would show `missing` forever).

### Auto-commit point moves to split -> finished (components)
- **Decision**: the spec completion commit point becomes `split -> finished`.
- **Rationale**: `split` is now the only predecessor of `finished`; the epic and specs written by a split are committed with the spec.
- **Rejected**: keeping a `verification -> finished` edge (it would bypass the split check).

### Status report adds `epic.done` and `epic.sources` (data_structures)
- **Decision**: the epic block carries `done` (derived) and its own `sources` alongside the design's fields; each spec carries effective `sources`.
- **Rationale**: the "Epic completion" and "Full trail shown" ACs need both visible in status; adding fields keeps the design's shape.
- **Rejected**: inferring done from progress counts only (callers would have to re-derive the rule).

### New specs are named by title in the split description (data_structures)
- **Decision**: new specs in the split JSON carry a `title`; the CLI allocates the ID and rewrites dependency references from titles to names.
- **Rationale**: the agent cannot know counter or timestamp IDs before allocation.
- **Rejected**: a two-call reserve-then-write flow.

### Schema bump sequenced after the epic store, then a human migrate (tasks)
- **Decision**: the migration/schema-bump task depends on the epic split task, the user migrates this repo in a `human` task, and the first tasks of milestones 2 and 3 depend on that human task.
- **Rationale**: after the bump the gate refuses `go run .` here, which would stall the implement workflow mid-run; migrating this repo's config needs the user's go-ahead.
- **Rejected**: bumping the schema in the settings task (stalls everything after it); leaving the ordering implicit.

### Harbor runs are a human task (tasks)
- **Decision**: oracle updates are agent work; running the harbor suites is a `human` task.
- **Rationale**: harbor needs agent API credentials and runs outside CI.
- **Rejected**: an agent running harbor (no credentials).

### Docs placement (tasks)
- **Decision**: a new `epics.mdx` page (in the Resources nav) for epics/splitting/dependencies; status replaces export/status sections in `plan-tasks.mdx` and `documents.mdx`; seeding goes in `how-it-works.mdx` stage 1.
- **Rationale**: there is no CLI reference page; plan-tasks already hosts the task-progress material that `status` replaces.
- **Rejected**: a standalone status page (splits closely related task material across pages).

## Rehydration cues

- `go run . spec file read 000060_epics-and-seeded-specs`
- `go run . design read --data '{"source":"design","path":"epics-and-seeded-specs.md"}'`
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs`
- `go run . knowledge search store`, `... migration`, `... workflow steps`, `... harbor`
- `go run . plan file read 000058_plan-task-graph plan`, `000055_design-authoring-skill`, `000053_config-schema-versioning-and-migrations`
- Re-read: `cmd/design_ref.go`, `internal/metadata/merge.go`, `cmd/artifact_status.go`,
  `cmd/plan_export.go`, `internal/steps/spec/steps.go`, `cmd/implement.go:120-330`,
  `internal/migrate/steps_project.go`.
