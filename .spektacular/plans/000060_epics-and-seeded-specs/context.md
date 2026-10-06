---
created_date: "2026-10-01"
document_status: final
closed_date: "2026-10-01"
---

# Context: 000060_epics-and-seeded-specs

## Current State Analysis

- **No epics exist.** Nothing in the code mentions epics, splits, `sources` or spec-level dependencies. Artifact kinds are spec, plan and changelog (`internal/artifact/address.go:23-27`).
- **Spec frontmatter is a closed schema.** `internal/metadata` drops any key it does not model on the next write (`metadata.go:62-230`, `merge.go:6-148`). Design references (`designs`) and design back-links (`specs`) are the precedent for carried-forward lists.
- **Status is spread over four commands.** These are `spec status [name]`, `plan status [name]`, `implement status` and `plan export <name> [--format]`. Their logic lives in `cmd/artifact_status.go`, `cmd/plan.go:220-316`, `cmd/implement.go:330-399` and `cmd/plan_export.go`, built on `internal/plantask`. Callers include the `spek-implement` skill (`templates/skills/workflows/spek-implement/SKILL.md:65`), the implement task hint (`cmd/implement.go:292`), `README.md:29-38` and the plan-workflow harbor suite.
- **The spec workflow is linear.** It runs new, interview, overview, requirements, acceptance_criteria, constraints, technical_approach, success_metrics, non_goals, verification, finished (`internal/steps/spec/steps.go:26-40`). `spec new` ignores unknown `--data` fields and always mints a fresh ID (`cmd/spec.go:161-274`). The `new` step overwrites with the scaffold.
- **`implement new` takes a plan name.** Its help says "against an existing plan" (`cmd/implement.go:54`). It runs `refuseStalePlan` and `refuseUnstartableTask` before the start gate, so refusals write no state.
- **Config is on schema 3** (`internal/config/schema.go`). Migrations are N->N+1 steps in `internal/migrate`. `spec_trigger_threshold` is read live by the agent from `templates/agents/spec-trigger.md`.
- **Docs site.** Status, export and implement-status material is in `docs:src/pages/plan-tasks.mdx`, `documents.mdx` and `how-it-works.mdx`. Config keys are in `configuration.mdx`. The nav is hard-coded in `docs:src/components/Nav.astro`.

## Per-Task Technical Notes

### Task: Extract the dependency graph helper

- **File changes**:
  - New `internal/depgraph/depgraph.go`: `FindCycle(order []string, deps map[string][]string) []string`, three-colour DFS in declaration order, returning the cycle path with the first node repeated. Port it from `internal/plantask/validate.go:120-164` (`findCycle`).
  - `internal/plantask/validate.go:26-62`: `Validate` builds `order`/`deps` from `[]Task` and calls `depgraph.FindCycle`. Delete the local `findCycle`. Keep the error construction via `invalid()` (:104-114) unchanged, so codes and messages are identical.
  - New `internal/depgraph/depgraph_test.go`: acyclic, self-loop, 2-cycle, longer cycle, unknown nodes ignored (callers check unknowns first), declaration-order determinism.
  - `internal/plantask/validate_test.go`: must stay green unchanged.
- **Complexity**: Low
- **Token estimate**: ~15k tokens
- **Agent strategy**: Single agent, sequential execution.

### Task: Record epic and sources on specs

- **File changes**:
  - `internal/metadata/metadata.go`:
    - add `type SourceRef struct{ URI string \`yaml:"uri" json:"uri"\`; RetrievedDate string \`yaml:"retrieved_date" json:"retrieved_date"\` }` next to `DesignRef` (:92-95);
    - add `Epic string` and `Sources []SourceRef` to `Metadata` (:62-86);
    - `yamlShape` (:100-110): `Epic string \`yaml:"epic,omitempty"\``, `Sources []SourceRef \`yaml:"sources,omitempty"\``;
    - `yamlInShape` (:114-129): `Epic string`, `Sources yaml.Node`;
    - map both in `MarshalYAML` (:132-147) and `UnmarshalYAML` (:154-184);
    - new lenient `decodeSourceRefs` modelled on `decodeDesignRefs` (:192-208), which drops entries without `uri` and non-mapping entries.
  - `internal/metadata/merge.go`:
    - `UpdateOptions` (:6-33): add `Epic *string` and `Sources *[]SourceRef` (nil = keep, non-nil = replace, empty = clear);
    - `Merge` (:54-148): set both on a fresh write (:76-97) and carry both forward on an existing write, next to the `Designs` carry-forward (:112-120).
  - Tests:
    - `internal/metadata/merge_test.go`: preserve on body-only rewrite, replace, clear, fresh write, status transition carries (pattern :29-220).
    - `internal/metadata/metadata_test.go`: round-trip, omit-when-empty byte-exact guards beside `TestRender_OmitsDesignsKeyEntirelyWithNoReferences` (leave that guard unmodified), malformed reads empty (pattern :747-945).
- **Complexity**: Medium
- **Token estimate**: ~30k tokens
- **Agent strategy**: Single agent, sequential execution. Every closed-schema site must change in lockstep.

### Task: Add the epic store settings

- **File changes**:
  - `internal/config/config.go`:
    - constants `EpicSplitThreshold{Strict,Moderate,Lenient}` beside `SpecTriggerThreshold*` (:22-26);
    - `DefaultEpicDir = "epics"` beside :54-56;
    - types `EpicConfig{Provider string \`yaml:"provider"\`; StrictDependencies bool \`yaml:"strict_dependencies"\`; Config FileEpicConfig \`yaml:"config"\`}` and `FileEpicConfig{Directory string \`yaml:"directory"\`; fileForm string}` (model: `PlanConfig` :99-124, `ChangelogConfig` :128-131);
    - `Config` (:284-308): `EpicSplitThreshold string \`yaml:"epic_split_threshold"\`` beside :299, and `Epic EpicConfig \`yaml:"epic"\`` after `Changelog`;
    - `NewDefault()` (:321-357): `Epic{Provider: ProviderFile, Config: {Directory: filepath.Join(ProjectConfigDirName, DefaultEpicDir)}}`, `EpicSplitThreshold: moderate`;
    - `ParseYAMLFile` prefill (:390-392): `cfg.Epic.Config.Directory = DefaultEpicDir`, and empty provider/threshold read as defaults;
    - `storeDirs()` (:430-436): add the epic entry;
    - the default-name map at :688: add `"epic"`;
    - `Validate()` (:583-627): an `EpicConfig.Validate` (pattern `ChangelogConfig.Validate` :722-730) and the threshold switch (pattern :587-591), each with a next_action listing the allowed values.
  - `internal/artifact/address.go:23-27`: `KindEpic Kind = "epic"` (single-name kind, like spec).
  - `internal/project/init.go:102-128`: leave unchanged. Do not create the epics dir.
  - Tests:
    - `internal/config/config_test.go`: defaults and invalid values.
    - `internal/config/storedir_test.go`: round trip in file form; `TestToYAMLFile_NewDefaultWritesSettingsRelativeStoreDirs` (:133) gains the epic entry.
    - `cmd/init_test.go`: a fresh init writes the epic block and no `epics/` dir; `TestInit_SecondRunProducesNoChanges` (:340) stays green.
  - Gotcha: `strict_dependencies` is a real bool, so the YAML boolean-token gotcha does not apply. Compare fields, not whole `Config`, because of the unexported `fileForm`.
- **Complexity**: Low
- **Token estimate**: ~25k tokens
- **Agent strategy**: Single agent, sequential execution.

### Task: Add the epic document and its graph validation

- **File changes**:
  - New `internal/epic/epic.go`:
    - `EpicSpec{Name, DependsOn}` and `Epic{CreatedDate, DocumentStatus, ClosedDate, Spec, Specs, Sources []metadata.SourceRef, Body}`;
    - a closed YAML shape with `depends_on` always emitted (no omitempty, nil rendered as `[]`);
    - `Parse(raw)` reuses the fence handling from `internal/metadata/frontmatter.go:16-46`. Extract an unexported-to-exported `SplitRaw(raw) (yamlBytes, body, ok)` helper there rather than duplicating it;
    - `Render()`;
    - `Stamp(existing *Epic, now)`: created date stamped once, status validated via `metadata.ParseDocumentStatus`, closed date on close (reuse `dateFormat`, `isClosed`).
  - New `internal/epic/validate.go`: `Validate(specs []EpicSpec) error` in the order of `internal/plantask/validate.go:26-62`: per-entry (`name`, `depends_on` present; the decoder must distinguish a missing `depends_on` from `[]`, so keep a raw node or presence flag), duplicates, unknown dependencies, then `depgraph.FindCycle`. Errors are `output.NewError("epic_invalid", …).WithResource(name).WithNextAction("fix the specs list in --data and re-run epic write")`.
  - New `internal/epic/epic_test.go`, `validate_test.go`: round-trip with order preserved, `depends_on: []` emitted, every invalid shape including a self-cycle, and error messages naming the offending spec.
- **Complexity**: Medium
- **Token estimate**: ~35k tokens
- **Agent strategy**: Single agent, sequential execution.

### Task: Add the epic commands and membership links

- **File changes**:
  - New `cmd/epic.go`, hand-written like `cmd/design.go:15-646` (not `newStoreFileCmd`):
    - `epicCmd` with `read <name>`, `write <name> --from <path> [--data]`, `list`, `delete <name>`, persistent `--schema` (pattern :646), and `commandSchema` dumps (pattern :255-257, :349-357);
    - store via `store.NewSourceStore(root, "project")` and `artifact.StorePath(cfg.Epic.Config.Directory, name)`;
    - `read`: raw bytes; `epic_not_found` with next_action `epic list`;
    - `list`: mirror `cmd/storefile.go:347-428` output (`files[{name, path, modified_at, created_date, document_status, closed_date}]`);
    - `write`: a bare name that is not yet an epic is given an ID with `identifier.Resolve` (`internal/identifier/identifier.go:66-113`, method `cfg.Spec.IDMethod`, `Dir` the epic directory); an empty `specs` list is allowed; strip leading frontmatter (`stripLeadingFrontmatterBlocks` `cmd/storefile.go:65-74`), parse `--data {specs?, sources?, spec?}` (omitted fields keep the existing value), stamp lifecycle, `epic.Validate`, then the link writer;
    - `delete`: unlink every member, then delete; output `{name, deleted, unlinked}`.
  - New `cmd/epic_link.go`, the link writer modelled on `cmd/design_ref.go:173-319`:
    - read the epic's old members and diff them against the new members;
    - before any write: for each added spec, require it to exist (`epic_spec_not_found`, next_action `spec file list`) and its `epic` to be empty or this epic (`epic_membership_conflict`, naming the other epic and the `epic write` that removes it there); refuse any member name that is an existing epic (`epic_nested`);
    - write the epic first; then set or clear `epic` on each changed spec via `metadata.Merge(raw, body, UpdateOptions{Epic:&v})` (pattern `writeRefs` :173-187);
    - on failure, restore originals in reverse order: `epic_link_failed` (retry), and if the restore itself fails, `epic_link_rollback_failed` naming every document;
    - seam `var writeEpicLinkFn = writeEpicLink` (pattern :269).
  - `cmd/root.go:354-365`: register `epicCmd`.
  - `cmd/file.go:10-16` / `cmd/storefile.go:318-333`: add an optional pre-delete hook to `storeFileKind` (:48-55) used only by spec. It refuses with `spec_in_epic_delete` when the spec's `epic` is set; next_action is ``epic write <epic> --from <current body> --data '{"specs":[…without this spec…]}'`` (pattern `refuseIfReferenced` `cmd/design.go:440-467`).
  - `templates/agents/store-access.md:11-26`: add `{{command}} epic` to the CLI list and `{{command}} epic delete` to removal verbs.
  - `internal/agent/store_access_test.go:94,124`: pin the new commands.
  - `cmd/root_test.go`: subcommand pins (:1177-1179 and the unknown-subcommand list) gain `epic`.
  - New `cmd/epic_test.go`:
    - write/read round-trip; list; first write creates the dir;
    - both-way agreement after write, member removal and delete;
    - conflict and nesting refused with no bytes changed;
    - mid-write failure via the seam and via directory replacement (not chmod), with everything restored;
    - next_action content asserted;
    - all through `runRootCmd`/`resetRootCmd`.
  - `cmd/file_test.go`: spec delete refused while in an epic, allowed after removal.
- **Complexity**: High
- **Token estimate**: ~70k tokens
- **Agent strategy**: Parallel analysis, sequential integration. One agent writes the link writer and its tests; another writes the verbs and schemas; integrate the delete guard and store-access template last.

### Task: Split into an epic from the command line

- **File changes**:
  - `cmd/epic.go`: `epic split --from <staged json>`.
    - **Validate the description** (see Data Structures). Refuse with `epic_split_invalid` when:
      - `spec` is missing or not an existing spec;
      - the result has fewer than 2 specs;
      - a new spec has no `title`;
      - any spec has an empty `body.overview` or no `body.acceptance_criteria`;
      - `overview` is missing when creating an epic.
    - **Resolve the target epic:** the `spec`'s current `epic`, else a new epic named after `spec` (design Decision 2).
    - **Allocate names** for each new spec, one at a time, with `spec.ResolveIdentifier` (`internal/steps/spec/identifier.go:33-52`, method `cfg.Spec.IDMethod`). Write each spec before allocating the next, so counter IDs do not collide (`internal/identifier/identifier.go:249-297`).
    - **Render every spec** (the narrowed one included) from `templates/scaffold/spec.md`:
      - fill every section from `body`: Overview prose; Requirements and Acceptance Criteria as `- [ ] **…**` items; the remaining sections as bullets;
      - the frontmatter of new specs comes from `metadata.Merge(nil, body, UpdateOptions{Sources:…})`, then is closed as `final` with `metadata.Close` (`internal/metadata/close.go:16-30`);
      - the narrowed spec is rewritten via `metadata.Merge(existing, body, …)`, preserving its dates, designs and status.
    - **Map titles to names** in `depends_on`.
    - **Build the epic:** merge with the existing epic's `specs` when extending, then run `epic.Validate`, then the completed-epic guard (added by "Guard additions to a completed epic"), then the link writer.
    - **On failure,** delete the new specs written so far, restore the narrowed spec's original bytes, and roll back through the link writer.
    - **Output:** `{epic, created, linked, path}`.
  - Add the epic body renderer: `## Overview` plus a `## Specs` table (`| # | Spec | Scope |`). Scope is an optional per-spec `scope` field, else the first sentence of that spec's overview.
  - `cmd/epic_test.go` covers:
    - a split of a standalone spec;
    - a split of a spec already in an epic (the epic is extended, not a second epic created);
    - the epic body contains only Overview and Specs;
    - each resulting spec is `final`, has every given section, and the narrowed spec keeps its `created_date` and `designs`;
    - each requirement and criterion lands in exactly one spec;
    - a shared constraint appears in each listed spec;
    - counter IDs are distinct and sequential (pin `spec.id_method: counter`);
    - invalid descriptions write nothing;
    - an injected failure restores everything.
- **Complexity**: High
- **Token estimate**: ~60k tokens
- **Agent strategy**: Parallel analysis, sequential integration.

### Task: Migrate existing projects to the epic store

- **File changes**:
  - `internal/config/schema.go:15-21`: `CurrentProjectSchema = 4`.
  - `internal/migrate/steps_project.go`: `project3to4` (pattern `project2to3` :90-134). It sets `epic.provider=file`, `epic.strict_dependencies=false` (a `!!bool` node via `setNode`, `node.go:80`; check how `setScalar` :74 tags values), `epic.config.directory=epics` and `epic_split_threshold=moderate`, only when absent, recording `Action{Op:"set"}` for each.
  - `internal/migrate/registry.go:61`: register it.
  - Tests and fixtures:
    - `internal/migrate/registry_test.go:13-30` and `steps_project_test.go:130-198`: `To == 4`, step list.
    - `internal/migrate/testdata/current/config.yaml`: schema 4 with the epic block; regenerate `golden/*config.yaml.golden`.
    - `engine_test.go:239`: `TestApply_SecondApplyIsUpToDate` stays green.
    - Add a test that existing `epic` values are kept.
    - Hard-coded `schema: 3`: `cmd/gate_test.go:65,99,125`, `cmd/migrate_test.go:230,355,369`, `cmd/version_test.go:25,86,110,128`.
    - Shared helpers (`cmd/root_test.go:78-108`, `writeSpecCommandConfig`) follow `CurrentProjectSchema`.
  - Self-hosting trap: after this lands, `go run .` in this repo is refused by the gate (`cmd/gate.go:30-80`) until the next task. Build a binary before the bump if the workflow must keep advancing.
- **Complexity**: Medium
- **Token estimate**: ~40k tokens
- **Agent strategy**: 2-3 parallel agents: one for the migration step and its tests, one for the fixture sweep.

### Task: Migrate this repository's own configuration

- **What the person does**: run `go run . migrate --dry-run` in the spektacular repo root, review the reported actions (schema 3 -> 4, `epic` block, `epic_split_threshold`), then run `go run . migrate`. If `go run . init` was also run to regenerate skills, restore any unrelated `config.yaml` rewrites (`agent`, `written_by`, `skills_version`), per the 000059 lesson.
- **How it is checked**: `go run . version check` reports `match`, and `.spektacular/config.yaml` shows `schema: 4` and the `epic` block.

### Task: Build the status report

- **File changes**:
  - New `internal/status/` package:
    - `resolve.go`: name resolution in the order epic store, spec store, then plan store (plan -> spec via plan frontmatter `Spec`, else the same name, as in `cmd/plan.go:302-305`); unknown -> `artifact_not_found` naming the three stores searched, next_action `epic list` / `spec file list` / `plan file list`.
    - `classify.go`:
      - `Classify` in the design's order: `missing` (read fails), `stale` (strict stale hook), `specified` (no plan; a spec still being written shows its live `current_step`), `planned` (0/N), `in_progress` (k/N), `implemented` (N/N);
      - legacy plans count phase checkboxes via `plantask.Parse(...).OpenItems()` (`internal/plantask/plantask.go:336`); a plan with no checkboxes is `planned`;
      - `Describe` gives "in progress (2/5 tasks complete)";
      - `ready` (every `depends_on` implemented) and `blocked_by`.
    - `report.go`: `Build(store, cfg, state, name) (Report, error)` assembles the report:
      - per plan: `plantask.Parse`, `plantask.NewExport` (`internal/plantask/export.go:39-60`) tasks plus `Criteria` per task (`internal/plantask/plantask.go:72-96`);
      - plan and spec `document_status` via the logic in `resolveDocumentStatus` and `strictPlanStatusHook`/`strictPlanIsStale` (`cmd/artifact_status.go:136-144`, `cmd/plan.go:286-316`), moved here and exported;
      - `current_step` from matching live state or derived `finished` / `stale` (as in `cmd/artifact_status.go:186-196`);
      - repo locations moved from `cmd/plan_export.go:115-131`;
      - epic roll-up and `done`;
      - effective `sources` (spec's own, then the epic's);
      - the `workflow` block from `.spektacular/state.json` (`internal/workflow/state.go:14-27`), set when it belongs to a reported spec or plan, else null.
    - `pretty.go`: the tree from the design (epic header, one line per spec, requested spec expanded to milestones and tasks, reusing the shape of `plantask.RenderPretty` `internal/plantask/export.go:66-116`).
    - Tests in `internal/status/*_test.go` with a temp store: every state, `missing` not hiding siblings, readiness, done/not done (including a spec with no plan), resolution for every name kind, standalone shape, effective sources, pretty golden lines.
- **Complexity**: High
- **Token estimate**: ~70k tokens
- **Agent strategy**: Parallel analysis, sequential integration. Classification and resolution are built first; report assembly and pretty rendering follow.

### Task: Replace the status and export commands with status

- **File changes**:
  - New `cmd/status.go`:
    - `statusCmd` (`Use: "status [name]"`, MaximumNArgs(1)), local `--schema` (as `cmd/version.go:108`) and `--format` (default `pretty`; `status_format_unsupported` with next_action naming `pretty|json`, pattern `cmd/plan_export.go:62-68`);
    - JSON via `output.New(...).WriteResult`, pretty to stdout, errors always JSON;
    - with no name: `{"workflow": null}` when nothing is in progress, else the report for the state's `data.name`;
    - registered in `cmd/root.go:342-366`.
  - Remove:
    - `specStatusCmd`/`runSpecStatus`/`statusOutputSchema` (`cmd/spec.go:68,99-104,334-393,426`);
    - `planStatusCmd`/`runPlanStatus`/`planStatusOutputSchema` (`cmd/plan.go:28,60-65,220-284,348`);
    - `implementStatusCmd`/`runImplementStatus`/`implementStatusOutputSchema` (`cmd/implement.go:30-44,64-68,330-399,432`);
    - `cmd/plan_export.go` entirely;
    - `cmd/artifact_status.go` (logic moved to `internal/status`);
    - `StatusResult`/`StepEntry` in `internal/steps/{spec,plan,implement}/result.go` if unused.
  - Callers:
    - `cmd/implement.go:292` hint becomes ``run `%s status %s --format json` to see the spec's tasks and their ids``;
    - `templates/skills/workflows/spek-implement/SKILL.md:65-67` becomes `{{command}} status <spec_name> --format json` with tasks under `specs[].plan.tasks`;
    - `internal/plantask/export.go:9` doc comment;
    - `README.md:29-38` rewritten for `status`;
    - `tests/harbor/plan-workflow/tests/test_plan_workflow.py:1072-1081` runs `spektacular status <name> --format json` and asserts `specs[0].plan.tasks` is non-empty;
    - regenerate `.claude/skills` and `.bob/skills` copies (restore unrelated `config.yaml` rewrites afterwards).
  - Deny-list: `internal/agent/instruction_surface_test.go:31-39` gains `spec status`, `plan status`, `implement status`, `plan export`.
  - Tests:
    - Move or rewrite `cmd/artifact_status_test.go`, `cmd/plan_status_progress_test.go`, `cmd/plan_export_test.go`, `cmd/status_address_test.go`, the status cases in `cmd/cross_kind_test.go:111-287`, `cmd/implement_test.go:204-330` and `cmd/implement_task_test.go:89-120` into a new `cmd/status_test.go`. The `task` in workflow data is now observed via the `workflow` block or state.
    - Update `cmd/root_test.go:1177-1179` pins.
    - `templates/skill_resume_test.go:82-94` asserts `status <spec_name> --format json`.
    - Add a test that the retired commands are unknown subcommands.
- **Complexity**: High
- **Token estimate**: ~75k tokens
- **Agent strategy**: Parallel analysis, sequential integration. One agent adds the command and its tests; a second moves callers and deletes the old commands; integrate and run the full suite.

### Task: Check spec dependencies when implementation starts

- **File changes**:
  - `cmd/implement.go`:
    - parse struct (:130-136) gains `OverrideDependencies bool \`json:"override_dependencies"\``; the schema input (:78-85) documents it and describes `name` as "the spec to implement (its plan shares the name)";
    - `Short` (:54) becomes "Create a new implement workflow for an existing spec";
    - `name_required` next_action (:127-128) and plan-not-found (:146-149) are reworded to "spec … run `plan new` for it first".
  - New `refuseUnmetDependencies(cfg, st, name, override)` placed after `refuseUnstartableTask` (:153-157) and before `startGate` (:162):
    - uses `internal/status` to load the spec's epic and classify each direct `depends_on`;
    - unmet and no override → `dependencies_unmet`, message listing "<spec> depends on <dep>, which is <Describe(state)>", next_action the same `implement new --data` plus `"override_dependencies": true` after the user agrees, or `implement new` for the first unmet dependency that is ready;
    - `cfg.Epic.StrictDependencies` → `dependency_override_refused` even with the override, next_action naming the first ready unmet dependency;
    - on an accepted override, `wf.SetData("dependency_override", […{name, state}])` after :173.
  - `internal/steps/implement/steps.go`: the `update_changelog` and `update_feature_changelog` callbacks pass `dependency_override` via `Extra`.
  - `templates/steps/implement/07-update_changelog.md` and `10-update_feature_changelog.md`: a `{{#dependency_override}}` block telling the agent to record under Deviations that implementation started past unmet dependencies, naming each and its state.
  - `templates/skills/workflows/spek-implement/SKILL.md:3,13,44-59,89-109`: "implement a spec", "ask which spec to implement", the `<spec_name>` placeholder, and handling `dependencies_unmet` (ask the user, re-run with the override, or implement the named dependency).
  - `templates/skill_resume_test.go:68-88`: update the expected strings. Regenerate skill copies.
  - Tests:
    - `cmd/implement_test.go` / new `cmd/implement_dependencies_test.go`: standalone and no-deps silent; unmet refused with the state wording; override starts and records data; strict refuses the override; no state written on refusal; next_action content asserted.
    - The `spec new` and `plan new` cases for a spec with unmet dependencies succeed with no warning.
    - Template contract test for the changelog override block.
- **Complexity**: Medium
- **Token estimate**: ~45k tokens
- **Agent strategy**: 2-3 parallel agents: CLI check and tests; templates and skill wording.

### Task: Start a spec with sources or in an epic

- **File changes**:
  - `cmd/spec.go` `runSpecNew` (:161-274):
    - the schema (:162-175) gains `sources` (array of `{uri}`) and `epic` (string); `confirm_completed_epic` is added by the guard task;
    - the parse struct (:216-222) gains `Sources []struct{URI string}` and `Epic string`;
    - each source needs a non-empty `uri`, else `sources_invalid` with a next_action showing the exact shape;
    - stamp `retrieved_date` with today's date (`YYYY-MM-DD`), then `SetData("sources", …)`;
    - if `Epic` is set, it must exist in the epic store, else `epic_not_found` with next_action `epic list`. Run this check before `startGate` (:252), so a refusal writes nothing. Then `SetData("epic", …)`.
  - `TestSpecNewSchemaDocumentsNameAndOptionalID` (`cmd/spec_test.go:103`) is updated.
  - `internal/steps/spec/steps.go`:
    - `new` callback (:70-100): write the scaffold with `UpdateOptions{Sources:…}`. When `epic` is set, call the epic link writer (from "Add the epic commands and membership links") to append the spec to the epic's `specs` with `depends_on: []` and set the spec's `epic`.
    - `interview` callback: pass `sources` (list of URIs) and `epic` via `Extra`.
  - Tests:
    - `cmd/spec_test.go`: sources recorded with today's date; none without; invalid refused; a spec started with `epic` is listed by the epic and names it; an unknown epic is refused with no spec written; without sources or epic, behaviour is unchanged.
    - `internal/steps/spec/steps_test.go`: interview Extra carries both.
- **Complexity**: Medium
- **Token estimate**: ~35k tokens
- **Agent strategy**: Single agent, sequential execution.

### Task: Guard additions to a completed epic

- **File changes**:
  - New `internal/status` helper `EpicComplete(store, cfg, epicName) (bool, error)`, built on the report's `done`.
  - New shared check in `cmd/epic_link.go`, `refuseCompletedEpic(cfg, st, epic, confirmed)`:
    - refuses with `epic_complete` (message: "epic <name> is complete: every spec is implemented") when the epic is complete and the call is not confirmed;
    - next_action is the same command with `"confirm_completed_epic": true`, to run after the user agrees.
  - Call it from:
    - `epic write` when `specs` adds a name;
    - `epic split` before any write;
    - `spec new` when `epic` is set (`cmd/spec.go`), before `startGate`.
  - Each of these gains `confirm_completed_epic` in its `--data` and its `--schema`.
  - Tests:
    - each route refused with no bytes changed;
    - each route succeeds with confirmation, after which `status` reports the epic not done;
    - an incomplete epic needs no confirmation;
    - next_action content asserted.
- **Complexity**: Low
- **Token estimate**: ~20k tokens
- **Agent strategy**: Single agent, sequential execution.

### Task: Add the split step and chaining to the spec workflow

- **File changes**:
  - `internal/steps/spec/steps.go:26-40`:
    - insert `split` between `verification` and `finished` (`split.Src = {verification}`, `finished.Src = {split}`);
    - the `split` callback renders `templates/steps/spec/08b-split.md` with `Extra{epic_name}` (the spec's current epic, if any);
    - the `finished` callback (:164-186) passes `epic_name` and that epic's `sources` (URIs) via `Extra`, read through `internal/epic`.
  - New `templates/partials/split-check.md`:
    - the gate, the strong signals and the weak signals (two or more);
    - supporting work never counts, and the counter-signals;
    - a live `epic_split_threshold` read from `.spektacular/config.yaml` (missing means moderate; strict, moderate and lenient move only the gate and signal counts);
    - the offer wording.
  - New `templates/partials/split-flow.md`:
    - agree specs, dependencies and where each requirement, criterion, constraint and non-goal goes;
    - fill any thin acceptance criteria with the user;
    - one fresh-eyes subagent review over every resulting spec, reusing the brief from `08-verification.md:25-73`;
    - stage the JSON description in `.spektacular/tmp/` and run `epic split --from …`;
    - sources move to the epic unless they are only about the narrowed spec;
    - decline handling and the re-offer rule;
    - never act without agreement;
    - for a spec already written with a plan, warn that the plan goes stale.
  - New `templates/steps/spec/08b-split.md`:
    - includes `{{> partials/split-check}}`;
    - honours a split request recorded in `.spektacular/working-context.md`;
    - on agreement, includes `{{> partials/split-flow}}`;
    - ends with `goto finished`.
  - Section step templates and `00b-interview.md`: one sentence saying that if the user asks for a split now, record the request in the working context and continue; it is acted on at `split`.
  - `templates/steps/spec/09-finished.md`: a `{{#epic_name}}` chaining offer. If the epic's source has a child item with no spec yet (re-check the source with the agent's own tools), offer `spec new` for it with `sources` and `epic`.
  - `templates/steps/spec/08-verification.md:113+`: goto `split` instead of `finished`.
  - `internal/autocommit/points.go:30`: `{spec, split, finished}` replaces `{spec, verification, finished}`; update `points_pin_test.go`.
  - `cmd/instruction_contract_test.go:36`: `stepTemplateTable` row for `08b-split.md`.
  - `internal/steps/spec/steps_test.go:64,85`: step order and FSM walk.
  - New template contract tests (`templates/split_test.go`):
    - both partials are included by the step;
    - the check contains the gate, signals, supporting-work rule, counter-signals and threshold levels;
    - the flow requires agreement, a review over all specs, and states the re-offer rule;
    - section steps record a mid-workflow split request;
    - `finished` carries the chaining offer.
  - Re-check `templates/{section_drafting,rejection_repair_directive,work_files}_test.go` expectations (`split` is not a drafting step).
- **Complexity**: High
- **Token estimate**: ~55k tokens
- **Agent strategy**: Parallel analysis, sequential integration. Do the FSM, callbacks and autocommit with their tests first, then the partials and templates with contract tests.

### Task: Seed specs from existing material

- **File changes**:
  - `templates/steps/spec/00b-interview.md`:
    - a `{{#epic}}` branch first: read the epic with `epic read` and each member with `spec file read`; do not re-ask scope they cover; offer to record dependencies (via `epic write`).
    - a `{{#sources}}` branch before the questions: seed the section work files under `.spektacular/work/{{spec_name}}/` from the source:
      - title and body -> overview and requirements;
      - checklists -> acceptance criteria;
      - must / must-not -> constraints;
      - out of scope -> non-goals.
    - After seeding: record in `interview.md` what came from the source, list the gaps (missing or too-thin sections), and ask only about those.
  - `templates/steps/spec/01-overview.md` … `07-non_goals.md`: strengthen the existing "own working file, if one already exists" clause so a pre-filled draft is presented to confirm, never asked from scratch. Keep the marker phrase that `section_drafting_test.go` requires.
  - `templates/skills/workflows/spek-new/SKILL.md`, "Starting a new spec" (:88-108):
    - if `epic list` shows any epics, ask whether the spec belongs to one (unless the request says), and pass `epic`;
    - handle `epic_complete` by asking the user and re-running with confirmation.
  - New `spek-new` section, "Starting from existing material":
    - recognise a source however phrased (#45, a link, LIN-123, "spec from …", a design doc name, a file path, pasted text), and ask if ambiguous;
    - fetch with your own tools and collect title, body, discussion, child items and a stable link;
    - if unreachable, say so and ask for the content;
    - child items: offer an epic, create it with `epic write` (overview, `sources` the parent, no specs), then start the first child with `spec new --data '{"name":"…","sources":[{"uri":"<child>"}],"epic":"<epic>"}'`;
    - propose a name;
    - a late child item for an existing epic leads to an offer of `spec new` with that epic.
  - New `spek-new` section, "Splitting a spec that is already written": `{{> partials/split-check}}` and `{{> partials/split-flow}}`.
  - `spek-new` skill description trigger wording for sources.
  - Template contract tests:
    - the interview's epic branch and seeded branch with the gap list;
    - the skill's phrasing variants;
    - no named fetching tool or tracker required;
    - unreachable-source handling;
    - the child-item route;
    - the epic question only when epics exist;
    - partials included.
  - Regenerate the skill copies.
- **Complexity**: Medium
- **Token estimate**: ~45k tokens
- **Agent strategy**: Single agent, sequential execution.

### Task: Update the spec-workflow harbor suite

- **File changes**:
  - `tests/harbor/spec-workflow/tests/test_spec_workflow.py:20-31`: `EXPECTED_STEP_ORDER` gains `"split"` before `"finished"`. Add an assertion that no `epics/` document exists after the run (the scenario is a single coupled feature).
  - `tests/harbor/spec-workflow/solution/solve.sh:14-100`: the step loop includes `split` before `finished`.
  - Check `tests/harbor/plan-workflow` and `implement-workflow` oracles for `spec` step lists or retired commands (the plan-workflow export check is handled by the status-command task).
- **Complexity**: Low
- **Token estimate**: ~15k tokens
- **Agent strategy**: Single agent, sequential execution.

### Task: Run the harbor suites

- **What the person does**: run the spec-workflow, plan-workflow and implement-workflow harbor suites (`tests/harbor/*`) with agent credentials, and record pass/fail in the implementation test plan.
- **How it is checked**: all three suites pass. Any failure is fixed in the relevant task before the work is called done.

### Task: Document epics, splitting and dependencies

- **File changes**:
  - New `docs:src/pages/epics.mdx` (layout `../layouts/Shell.astro`; imports Hero, Section, Prose, CtaBanner, Button like `docs:src/pages/design-documents.mdx:6-10`). Alternate `surface` explicitly.
  - `docs:src/components/Nav.astro:13-22`: add "Epics" to the Resources dropdown after Design Documents.
  - `docs:CHANGELOG.md`: a `## 000060_epics-and-seeded-specs` entry (no em dashes).
- **Content outline**:
  1. Hero: "Epics" / "Split a request that is too big for one spek into specs that each carry their own criteria."
  2. Section "What an epic is" (plain): an epic holds an overview and its specs with dependencies, nothing else; each spec is planned and implemented on its own. Example frontmatter:
     ```yaml
     spec: 000060_epics-and-seeded-specs
     specs:
         - name: 000060_epics-and-seeded-specs
           depends_on: []
         - name: 000061_epic-split
           depends_on: [000060_epics-and-seeded-specs]
     ```
  3. Section "When Spek offers a split" (surface): runs once when a spek is complete, or when you ask; the gate (two specs, each independently verifiable); strong and weak signals; "Code plus its docs is one spek"; never automatic.
  4. Section "Split sensitivity" (plain): `epic_split_threshold: strict | moderate | lenient`, separate from `spec_trigger_threshold`, link to /configuration/.
  5. Section "What a split produces" (surface): a split always acts on a complete spek (a mid-workflow request waits until completion); content is redistributed, one review covers all, and the epic plus complete specs are written in one step with no further interviews; agent conversation example in a plain ``` block.
  6. Section "Starting with an epic" (plain): a tracker epic with sub-issues gets an epic first, then each child is specified as its own spek in it; the next child is offered when one finishes; joining an epic when starting a spek (asked only when epics exist; the agent reads the epic and its speks first); adding to a completed epic needs confirmation and reopens it.
  7. Section "Dependencies between specs" (surface): checked only at implement; the states (`unplanned`, `planned, not started`, `in progress (k/N)`, `implemented`); warn and override recorded in the changelog by default; `epic.strict_dependencies: true` refuses. Sample `dependencies_unmet` JSON envelope.
  8. Section "Working with epics from the command line" (plain): `spektacular epic read|write|list|delete|split` examples in ```bash; `spektacular status <name>` link to the status docs.
  9. CtaBanner linking to /how-it-works/.
- **Complexity**: Medium
- **Token estimate**: ~35k tokens
- **Agent strategy**: Single agent, sequential execution. Verify with the Rule 1 grep, `npm run build` and `npx astro check`.

### Task: Document the status view

- **File changes**:
  - `docs:src/pages/plan-tasks.mdx`:
    - replace "Exporting a plan" (:166-223) and "Tracking progress" (:313-338) with one Section "Seeing where work stands: status";
    - change the "Export fields" ConfigurationKeys (:229-309) to "Status fields" (`requested`, `epic`, `specs[].state`, `specs[].blocked_by`, `specs[].plan.tasks[]…`, `tasks[].acceptance_criteria`);
    - :245 wording; :366-376 `implement status` example becomes `status` showing the `workflow` block with `task`;
    - frontmatter description (:4); example task title (:31, :186, :193).
  - `docs:src/pages/documents.mdx:223-242,324-328`: status wording moves to `spektacular status`. Add rows to "Upgrading from earlier spellings" (:294+) mapping `spec status`, `plan status`, `implement status` -> `status`, `spec status <name>`/`plan status <name>` -> `status <name>`, and `plan export <name> [--format]` -> `status <name> [--format]`.
  - `docs:src/pages/how-it-works.mdx:142-144,183-185`: "`spektacular status`".
- **Content example** (plan-tasks):
  ```console
  $ spektacular status 000061_epic-split
  epic 000060_epics-and-seeded-specs  (draft)  1/3 specs implemented, 7/11 tasks

    000060_epics-and-seeded-specs   implemented   5/5 tasks
    000061_epic-split               in progress   2/6 tasks   ← requested
  ```
  followed by the `--format json` example (the `requested` / `epic` / `specs` shape from the design) and one sentence: "Give it an epic, a spek or a plan; you always get the whole epic. With no name, it reports the workflow in progress."
- **Complexity**: Medium
- **Token estimate**: ~30k tokens
- **Agent strategy**: Single agent, sequential execution. Grep the site for `plan export`, `plan status`, `spec status`, `implement status` afterwards. Only CHANGELOG history may remain.

### Task: Document seeding, implementing a spec and the new settings

- **File changes**:
  - `docs:src/pages/how-it-works.mdx`:
    - :362-367 extend with "Starting from existing material";
    - :97 Step heading "Implement the spek"; :406 PipelineStage "Implement the Spek"; :410 node sub and :416 wording;
    - :194 SpecFormat: add a `sources` note or link to the epics page.
  - `docs:src/pages/index.mdx:41`: node sub "implements the spek via<br>coding agent".
  - `docs:src/content/tutorials/getting-started.mdx:51,853`: wording.
  - `docs:src/pages/configuration.mdx`:
    - example block (:33-82): `epic_split_threshold: moderate` after :40 and an `epic:` block;
    - :90-93 says "Sixteen top-level keys" and the list gains `epic_split_threshold` and `epic`;
    - new ConfigKey `epic_split_threshold` after :138-148 (same shape);
    - new ConfigKey `epic` (type section; `epic.provider`, `epic.strict_dependencies`, `epic.config.directory`) after `changelog` (:220), linking to /epics/;
    - the migrate section (:452) notes that migrate adds them.
- **Content example** (how-it-works, stage 1):
  > Speks don't only start from a conversation. Point Spek at an issue, a tracker epic, a design document, a file, a web page or pasted text ("spec from #45"), and it fetches the material with your agent's own tools, drafts every section it can, lists what the material leaves out, and asks only about those gaps. The spek records where its content came from in `sources`:
  > ```yaml
  > sources:
  >     - uri: https://github.com/hivecommons/spektacular/issues/70
  >       retrieved_date: "2026-09-28"
  > ```
- **Complexity**: Medium
- **Token estimate**: ~30k tokens
- **Agent strategy**: Single agent, sequential execution. Verify with the Rule 1 grep, `npm run build` and `npx astro check`, and check for no em dashes.

## Testing Strategy

Per task:
- **Extract the dependency graph helper:** new unit tests for the helper. The existing plan-task validation tests must pass unchanged.
- **Record epic and sources on specs:** metadata round-trip, carry-forward, clear, and byte-exact no-key guards.
- **Add the epic store settings:** config defaults and validation; store-dir round trip; init writes the block and no folder.
- **Add the epic document and its graph validation:** parse/render round-trip and every invalid graph shape.
- **Add the epic commands and membership links:** command tests through `runRootCmd`. Cover two-way agreement, refusals with no bytes changed, rollback on injected failure (seam and directory replacement, not chmod), first-write folder creation, the spec-delete guard, store-access template pins, and next_action content asserted.
- **Split into an epic from the command line:** both split cases (new epic, existing epic), complete `final` specs with every section, each item in exactly one spec, shared constraints, counter IDs with `spec.id_method: counter`, invalid descriptions writing nothing, rollback.
- **Migrate existing projects to the epic store:** the migration step, idempotent second apply, existing values kept, goldens and hard-coded schema fixtures.
- **Build the status report:** classification of every state, resolution of every name kind, readiness, done, effective sources, and pretty golden lines.
- **Replace the status and export commands with status:** command tests moved from the retired commands, no-name behaviour, format refusal, retired commands unknown, deny-list, skill contract, and the harbor plan-workflow check.
- **Check spec dependencies when implementation starts:** silent start, refusal wording with states, override records data, strict refuses the override, no state on refusal, no warning when specifying or planning, and a changelog template contract.
- **Start a spec with sources or in an epic:** sources recorded and dated, invalid refused, joined to the epic from the start, unknown epic refused, unchanged without either.
- **Guard additions to a completed epic:** each route refused without confirmation and allowed with it, epic no longer done afterwards.
- **Add the split step and chaining to the spec workflow:** FSM order and walk, the autocommit pin, and template contracts for the partials, threshold, agreement, review, re-offer, mid-workflow request and chaining.
- **Seed specs from existing material:** template contracts for the seeded interview branch, gap list, skill phrasing variants, tool-agnostic fetching, unreachable sources, child items and shared partials.
- **Harbor:** the spec-workflow oracles gain `split` and a no-epic assertion. All three suites are run by a person.
- **Docs tasks:** the Rule 1 grep, `npm run build`, `npx astro check`, a grep for retired commands and em dashes.

Success metrics:
- A seeded spec asks only about gaps: contract tests, plus **Manual — captured in the implementation test plan**.
- Code plus docs, tests or config never gets a split offer: contract tests, plus **Manual — captured in the implementation test plan**.
- One command shows where an epic stands: **Behavioural test** on `status`.

## Project References

- Spec: `000060_epics-and-seeded-specs` (`go run . spec file read 000060_epics-and-seeded-specs`).
- Design: `epics-and-seeded-specs.md` from the `design` design source (`go run . design read --data '{"source":"design","path":"epics-and-seeded-specs.md"}'`).
- Repos:
  - `spektacular` (root `/home/nicj/code/github.com/hivecommons/spektacular`), the Go CLI;
  - `docs` (root `/home/nicj/code/github.com/hivecommons/spektacular-website`), the Astro docs site.
- Knowledge (repo/spektacular):
  - conventions: error messages must suggest remediation, store files written through the CLI, tests must not depend on order, tests must pass for done;
  - glossary: stage, step, task, workflow;
  - architecture: working-with-files-from-steps, workflow-steps, testing-architecture, cli-design-for-ai-agents;
  - gotchas: remediation needs the layer that holds the facts, storedirs rewrites paths, fsm cancel only before transition commits, harbor working files deleted before inspection, chmod test sabotage ignored by root.
- Knowledge (repo/docs): mdx-authoring, no-em-dashes, plan-content-pages, site-layout, alternate-section-background, file-scoped-section-headings.
- Prior plans: 000053, 000054, 000055, 000056, 000057, 000058, 000059.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

The High tasks (epic commands, epic split, status report, status command, split step) should each start with a fresh context reading only their own technical notes and the files listed there.

## Migration Notes

- `CurrentProjectSchema` goes from 3 to 4. A `project3to4` step writes `epic` (provider `file`, `strict_dependencies: false`, directory `epics`) and `epic_split_threshold: moderate` when absent. Every existing project is told to run `migrate`.
- After the bump, this repository's own config must be migrated (a human task, with the user's go-ahead) before `go run .` works here. Until then, drive the workflow with a binary built before the bump.
- `spec status`, `plan status`, `implement status` and `plan export` are removed with no aliases. External orchestrators move to `status`, and the release notes name the replacement fields: per-task data is under `specs[].plan.tasks`, and the plan lifecycle is under `specs[].plan.document_status`.
- Regenerating skill copies with `go run . init` rewrites `agent`, `written_by` and `skills_version` in this repo's config. Restore those afterwards.

## Performance Considerations

`status` on an epic reads the epic, every member spec and plan, and the state file. Epics are small (a handful of specs), so this is negligible. The implement dependency check reads only the direct dependencies. No caching is needed.
