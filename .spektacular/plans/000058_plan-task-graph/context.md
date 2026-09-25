---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# Context: 000058_plan-task-graph

## Current State Analysis

- Plans describe work as `#### - [ ] Phase N.M:` headings under `### Milestone N:` in `## Milestones & Phases` (`templates/scaffold/plan.md:109-133`, `templates/steps/plan/10-phases.md:10-29`). Phases carry a `**Repo:**` line (which may list several repos), a summary, a technical-detail link and acceptance criteria. There are no ids, dependencies or executor.
- Only two pieces of Go read plan.md: `cmd/implement.go:18` (the `unchecked_phases` count in `implement status`) and `internal/autocommit/milestones.go:12-74` (milestone auto-commits). Everything else is template prose that the agent follows.
- `plan file write` (`cmd/storefile.go:190-235`) merges metadata and writes with no content validation. It is the single path by which plan.md reaches the store.
- `plan status <name>` (`cmd/artifact_status.go:60-114`) reports lifecycle fields only. It has no progress fields.
- `implement new` (`cmd/implement.go:74-174`) accepts only `name`, and the FSM (`internal/steps/implement/steps.go:22-37`) always works through every phase and always ends with test_plan → update_feature_changelog → reconcile_spec → finished. `finished()` requires the feature changelog (:167-177).
- Repos declare code location as a `source` provider block (`internal/config/repo.go:167-188`). Both registered repos here use `file` with `..`, so no git location is declared.
- No UUID library is in `go.mod`, and there is no non-JSON success output anywhere in the CLI.

## Per-Phase Technical Notes

### Phase 1.1: Add the plan task reader and validator

**Requirements covered**: Plans are made of tasks; Every task has a permanent identifier (parsing side); Every task states its dependencies / executor; Dependencies are readable in the plan; Invalid task structure is refused (rules).

**File changes**:
- New package `internal/plantask/` (`plantask.go`, `validate.go`, `plantask_test.go`, `validate_test.go`).
  - Line scanner modelled on `internal/autocommit/milestones.go:12-74`. The section starts at `## Milestones & Tasks` or the legacy `## Milestones & Phases` and ends at the next `## ` heading.
  - Milestone: `^###\s+Milestone\s+(\d+)\s*:\s*(.*)$`.
  - Task heading: `^####\s+-\s+\[([ xX])\]\s+Task:\s*(.+)$`.
  - Legacy heading: `^####\s+-\s+\[([ xX])\]\s+Phase\b`. It sets `FormatLegacy` and counts toward milestone Items/Open only.
  - Field lines inside a task block (until the next `####`/`###`/`##`):
    - `^\*\*Id:\*\*\s*(\S+)`
    - `^\*\*Repo:\*\*\s*(.+)$`: split on `,` and whitespace to detect more than one repo.
    - `^\*\*Depends on:\*\*\s*(.*)$`: `none` (case-insensitive) or empty followed by `^- (\S+)\s+—\s+.*$` list items. Also accept `-` / `--` separators and a bare id; keep only the first token.
    - `^\*\*Execution:\*\*\s*(agent|human)(?:\s+—\s+(.+))?$`: store the raw type so an unknown type can be reported.
  - Acceptance criteria: after `**Acceptance criteria**:`, lines `^- \[([ xX])\]` count toward Total/Met. Any other `- [ ]` inside the task block before that marker is ignored.
  - Track a `seen` flag per required line so "missing" can be told apart from "empty".
  - Mixed legacy and task headings: `Format = FormatTasks`. The phase headings are ignored for tasks and still counted for milestones.
- `Validate(p Plan, repos []string) error` returns `*output.ErrorResponse` with code `plan_task_invalid`, `resource` = task id (or title when the id is missing), and a message of the form `task "<title>" (<id>): <rule>`.
  - Each rule gets an exact `next_action`. Examples: "run `<command> plan task-id` and add `**Id:** <id>` under the task heading"; "set `**Repo:**` to one of: a, b"; "declare `**Depends on:** none` or list dependency ids".
  - Cycle detection uses DFS with three colours. The message lists the cycle path by title.
  - Duplicate ids are reported on the second occurrence.
- `RequireTasks()` returns `plan_structure_invalid` with the message "the plan contains no task ids" (FormatLegacy/FormatNone) and a next_action pointing at the task format docs or re-planning. It must not mention age or version.
- Needs `internal/output` for errors, and is the command name in next_action passed as a parameter (`Validate(p, repos, command)`) or set via an options struct.

**Tests**: table tests with hand-written fixtures as independent oracles (never derive expected values from the parser under test). Include a fixture where a `- [ ]` in `## Testing Approach` and `## Changelog` must not count.

**Complexity**: Medium
**Token estimate**: ~25k
**Agent strategy**: Single agent, sequential (reader then validator; tests alongside).

### Phase 1.2: Refuse invalid task structure when a plan is saved

**Requirements covered**: Invalid task structure is refused when a plan is saved; Identifiers survive edits (no rewriting on write).

**File changes**:
- `cmd/storefile.go:173` `newStoreFileCmd(short, dir, requireID, repoRouted)`: add a trailing `validate writeValidator` parameter (nil for spec/changelog). In the write handler (`cmd/storefile.go:190-235`), call it after `body := stripLeadingFrontmatterBlocks(content)` (:228) and before `metadata.Merge` / `st.Write` (:229-233). Return the error unwrapped so `toErrorResponse` (`cmd/root.go:252-261`) passes the structured error through.
- `cmd/plan_file.go:8-15`: pass `validatePlanDocument`, which:
  - acts only when `filepath.Base(docPath) == "plan.md"`;
  - parses the body;
  - skips validation when `Format != FormatTasks`;
  - otherwise builds repo names from `cfg.Repos[*].Name` and calls `plantask.Validate`.
- Update the other callers of `newStoreFileCmd` (`cmd/spec_file.go`, `cmd/changelog_file.go`) to pass `nil`.
- `set-document-status` (`cmd/storefile.go:360-418`) is not validated. It does not change the body.
- Tests in `cmd/plan_file_test.go`, following `TestPlanFileWrite_ResolvesConfiguredDirectory` (:15-35):
  - Save a valid plan, then for each invalid variant assert exit 1, the `plan_task_invalid` code, and a message containing the task title.
  - Assert `plan file read` returns the original bytes.
  - Assert the unregistered-repo next_action contains the registered repo names.
  - Assert a legacy plan and a `context.md` save.
  - Edit round trip: reorder, add a task, retitle, save, then parse and assert ids are unchanged.

**Complexity**: Low
**Token estimate**: ~12k
**Agent strategy**: Single agent, sequential.

### Phase 1.3: Issue task identifiers from a pluggable provider

**Requirements covered**: Identifiers are issued by Spektacular; Identifier source is selectable.

**File changes**:
- `internal/identifier/taskid.go` (new):
  - `type TaskIDProvider func() (string, error)` and `var taskIDProviders = map[string]TaskIDProvider{"uuid": newUUIDv4}`.
  - `TaskIDProviderFor(name)`: an empty name means `uuid`. Unknown names give `output.NewError("task_id_provider_unknown", fmt.Sprintf("task id provider %q does not exist", name)).WithNextAction("set plan.task_id.provider in .spektacular/config.yaml to one of: uuid")`, with the list sorted from the map keys.
  - `newUUIDv4` takes 16 bytes from `crypto/rand` (the pattern at `identifier.go:241-247`), sets `b[6] = b[6]&0x0f | 0x40` and `b[8] = b[8]&0x3f | 0x80`, and formats as 8-4-4-4-12.
  - Doc comment: providers with external side effects must tolerate ids issued for tasks later discarded (spec Technical Approach).
- `internal/config/config.go:95-99`: add `TaskID TaskIDConfig \`yaml:"task_id,omitempty"\``. In `NewDefault()` (:324-329), set `TaskID: TaskIDConfig{Provider: "uuid"}`. Do **not** validate the provider in `PlanConfig.Validate` (:697). Keep `CurrentProjectSchema` unchanged (`internal/config/schema.go:15-21`). Check that `ToYAMLFile` round-trips the key, and that migration golden files (`internal/migrate/testdata/golden`) are not affected. If a default write now emits `task_id`, update the goldens and justify it.
- `cmd/plan.go`: add a `planTaskIDCmd` (`task-id`, no args). It supports `--schema` in the style of `cmd/plan.go:73-85`, loads config, calls the provider and writes `{"id": ...}` via `output.Write`. Register it at `cmd/plan.go:346`.
- Tests:
  - `internal/identifier/taskid_test.go`: v4 regex `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$` and 200 unique values.
  - `cmd/plan_task_id_test.go`: the default config returns a valid UUID; `plan: {task_id: {provider: nope}}` gives exit 1 with code `task_id_provider_unknown` and a message containing `nope`; other commands (e.g. `plan file list`) still work with the bad provider set.

**Complexity**: Low
**Token estimate**: ~10k
**Agent strategy**: Single agent, sequential.

### Phase 1.4: Move milestone commits and unchecked-work counts onto the task reader

**Requirements covered**: Existing plans keep milestone commits; implement progress reporting keeps working (constraint).

**File changes**:
- `internal/autocommit/milestones.go:12-74`: reimplement `CompletedMilestones(planMarkdown)` on `plantask.Parse`. A milestone is complete when `Items > 0 && Open == 0`. Remove the `milestoneHeading`/`phaseCheckbox` regexes. Keep `DueMilestones` (:78-90) unchanged.
- `internal/autocommit/message.go:35`: change the "whose phases are now all complete" wording to "whose tasks are now all complete". Check `cmd/autocommit.go:298` too.
- `cmd/implement.go:18,307-321`: replace `uncheckedPhaseRegexp` with `plantask.Parse`, counting open tasks (FormatTasks) or open phases (FormatLegacy). Keep the JSON field `unchecked_phases` (`internal/steps/implement/result.go:17,28`) because existing status fields must not change. Optionally add `unchecked_tasks` alongside it with the same value. Record this as an assumption only if added.
- Tests:
  - Keep every case in `internal/autocommit/milestones_test.go` and `cmd/milestones_test.go` (legacy fixtures `writeTwoMilestonePlan` :28-51) passing unchanged.
  - Add task-format twins: a two-milestone task plan fixture and a `tickTask` helper, with the same commit-count assertions (:171-197).
  - `cmd/implement_test.go:203-221`: add a task-format plan case for `unchecked_phases`.

**Complexity**: Low
**Token estimate**: ~12k
**Agent strategy**: Single agent, sequential.

### Phase 2.1: Add `plan export` with pretty and JSON output

**Requirements covered**: Export a plan's task graph; Exported tasks carry the full task picture; Repository location is never guessed; The export always reflects the current plan; Export works in any document status; Structural errors explain what is missing (export side).

**File changes**:
- `cmd/plan_export.go` (new): `planExportCmd` `export <name>` with `--format` (default `pretty`) and `--schema`. Flow:
  1. `loadConfig`.
  2. Build the store `store.NewSourceStore(root, "project")`.
  3. Read `plan.PlanFilePath(cfg.Plan.Config.Directory, name)` (`internal/steps/plan/steps.go:14`). `store.ErrNotFound` gives `artifact_not_found` with a next_action of `<cmd> plan file list`, matching the error shape `runArtifactStatus` uses (`cmd/artifact_status.go:60-114`).
  4. Validate the format first, before reading: anything other than `pretty`/`json` gives `export_format_unsupported`, "supported formats: pretty, json", written through the JSON failure path.
  5. `metadata.Split(raw)` gives the body and metadata. The document status is computed through the same helper `runArtifactStatus` uses with `strictPlanStatusHook(cfg, st, name)` (`cmd/plan.go:284-313`). Extract a small `planDocumentStatus(cfg, st, name, meta)` helper and use it in both places.
  6. `plantask.Parse(body).RequireTasks()`.
  7. For each task, repo location: `set := repo.New(cfg, root, repoGit)` (`internal/repo/set.go:66`), then `meta, err := set.DescriptiveMetadata(task.Repo)` (:167), then `kind, loc, _ := meta.ParseSource(configDir)` (`internal/config/repo.go:254-278`). Use `loc` if `kind == config.SourceGit`, else `""`. Must not clone (DescriptiveMetadata does not) and must not call git.
  8. Build `plantask.Export`, with `DependsOn` initialised to `[]string{}`.
- `internal/plantask/render.go` (new): `RenderPretty(w, Export, titlesByID)` in the design's layout:
  - header `<name>  (<status>)  N/M tasks complete`;
  - a blank line, then `Milestone N`;
  - one line per task: `  [x] <title padded>  <repo>  agent|human: <reason>`;
  - an indented id line, and an indented `depends on: <title>, <title>` line when non-empty.
  - Pure function, so it can be unit-tested with a golden string.
- JSON: `output.Write(cmd.OutOrStdout(), export, "")`. Note that `output.Write` injects `"error": false`. Check whether that is acceptable to Hive's decoder (unknown fields are ignored by `encoding/json`, so it is). Alternatively marshal directly. Choose `output.Write` for consistency with `--fields`.
- Register at `cmd/plan.go:346`.
- Tests in `cmd/plan_export_test.go`:
  - json field set and order;
  - `none` gives `[]`;
  - pretty with no flag equals `--format pretty` byte-for-byte;
  - a bad format gives exit 1, a JSON error naming both formats, and stdout containing no task title;
  - a git source repo (write `repo.yaml` with `config.GitSource("https://github.com/x/y")`) exports the URL, while a file source plus a real `git remote add origin` in the temp dir (use `internal/testutil/gittest`) exports `""`;
  - tick a task via `plan file write`, then export shows `completed: true`;
  - draft and final (`plan file set-document-status`) export statuses equal `plan status <name>`'s `document_status`;
  - a missing plan gives a structured error;
  - a legacy plan gives `plan_structure_invalid`, and the message contains "task ids" and not "version"/"old"/"legacy".

**Complexity**: Medium
**Token estimate**: ~25k
**Agent strategy**: 2 parallel agents. One writes the command plus JSON, the other the pretty renderer with its golden test. Integrate sequentially.

### Phase 2.2: Report per-task progress in plan status

**Requirements covered**: Plan status reports progress per task; Completion and criteria are reported separately.

**File changes**:
- `cmd/artifact_status.go:25-38` `artifactStatusResult`: add `Progress *taskProgress \`json:"progress,omitempty"\`` and `Tasks []taskStatus \`json:"tasks,omitempty"\``. Extend `artifactStatusOutputSchema` (:40-56) with both as optional.
- `runArtifactStatus` (:60): add an optional plan-only enrichment. Either add a second hook parameter `bodyHook func(body []byte, r *artifactStatusResult)`, passed only from `runPlanStatus` (`cmd/plan.go:243-246`), or add a `kind == "plan"` branch. Prefer the hook, to keep spec status untouched. It parses the body already read at :71 with `plantask.Parse`, and when `Format == FormatTasks` fills `progress{tasks_completed, tasks_total}` and `tasks[]{id,title,milestone,completed,acceptance_criteria{met,total}}`.
- Tests in `cmd/artifact_status_test.go`, following :117-152:
  - the 4-task fixture (2 complete, one at 2/3) asserts the numbers;
  - a completed task with 1/2 criteria reports `completed: true, met: 1, total: 2`;
  - a legacy plan's output has exactly the previous key set (assert on key presence);
  - the `--schema` test (:214) includes the new optional fields.

**Complexity**: Low
**Token estimate**: ~10k
**Agent strategy**: Single agent, sequential.

### Phase 3.1: Select a task when starting implement

**Requirements covered**: Implement one selected task; Task runs refuse tasks that cannot start; Implementation status names the task; Structural errors explain what is missing (implement side).

**File changes**:
- `cmd/implement.go:76-85`: `--schema` input gains an optional `task` string. At :127-135 the input struct becomes `struct{ Name string; Task string \`json:"task"\` }`.
- `cmd/implement.go`, after `refuseStalePlan` (:146) and before `startGate` (:153), when `input.Task != ""`:
  - read plan.md through `projectStore`;
  - `plantask.Parse`, then `RequireTasks()` (gives `plan_structure_invalid`);
  - `Task(id)` not found gives `task_not_found` with resource = id and next_action `<cmd> plan export <name>` to list ids;
  - `Completed` gives `task_completed`;
  - incomplete dependencies give `task_dependencies_incomplete`, listing the ids (and titles) in the message and in next_action ("implement these first: `<cmd> implement new --data '{"name":..,"task":"<dep>"}'`");
  - `Execution.Type == "human"` gives `task_requires_human`, with the reason in the message.
  - Each returns before `clearState` (:157), so no state is written.
- After `wf.SetData("name", ...)` (:164), add `wf.SetData("task", input.Task)` when set.
- `cmd/implement.go:262-323` status: read `wf.GetData("task")` into a new `StatusResult.Task string \`json:"task,omitempty"\`` (`internal/steps/implement/result.go:21-30`), and add it to the output schema (:30-42).
- `cmd/resume.go:27-91` / `templates/steps/resume_implement.md`: pass `task` into the render data, and in the template add `{{#task}}This run implements only task {{task}}; …{{/task}}`, so a resumed session continues the single-task run.
- Tests in `cmd/implement_test.go`, using `setupImplementCmd`/`runRootCmd`:
  - one test per refusal code, each asserting `require.NoFileExists(statePath)`;
  - a dependency refusal message contains the dependency id;
  - a human refusal contains the reason;
  - a legacy plan with `task` gives `plan_structure_invalid`;
  - a valid task gives `step == read_plan` and status `task == <id>`;
  - no task gives today's behaviour (existing tests unchanged).

**Complexity**: Medium
**Token estimate**: ~18k
**Agent strategy**: Single agent, sequential.

### Phase 3.2: Route feature wrap-up to the run that completes the last open task

**Requirements covered**: Feature-level wrap-up happens once; Whole-plan runs unchanged.

**File changes**:
- `internal/steps/implement/steps.go:35`: `finished` Src becomes `[]string{"reconcile_spec", "update_changelog"}`.
- `updateChangelog()` (:115-124):
  - When `data.Get("task")` is set, read plan.md via `st`, then `plantask.Parse(...).OpenTasks()`, and pass `Extra{"task": {...}, "last_task": len(open)==0}` to the template.
  - The template renders the single correct exit: `goto finished` when `!last_task` in a task run, and `goto test_plan` when `last_task`.
  - The whole-plan run keeps today's wording (`analyze` or `test_plan`).
  - Also refuse, in the goto path, a task run choosing `analyze`? Keep it to template guidance. The FSM still allows `analyze` from `update_changelog` for whole-plan runs.
- `finished()` (:152-181): when `data.Get("task") != ""` and plan.md still has open tasks, skip the `changelog_missing` check (:167-177) and do not close the test plan or changelog. Render `12-finished.md` with `Extra{"task_run": true}` so the summary says "task <title> complete; N tasks remain".
- `internal/autocommit/points.go:30-45`:
  - Add `{kind:"implement", from:"update_changelog", to:"finished"}` as a completion point, so the task run's work is committed.
  - If milestones are also due at that point, `gotoWithAutoCommit` (`cmd/autocommit.go:57-177`) must still record them. Make the completion path call `dueMilestones` too, or classify the edge as a milestone candidate that falls back to a completion commit. Decide by reading `cmd/autocommit.go:79-125`, and record the decision.
  - Update `points_pin_test.go:21-39`.
- Update the step-table tests: `internal/steps/implement/steps_test.go:85-201` (new Src), `cmd/instruction_contract_test.go:77-87` (update_changelog template exits), and `cmd/implement_test.go:177-192` if it walks the steps.
- Tests:
  - A two-task fixture. Drive `implement new` with task A, then goto through to `update_plan`, write a plan with A ticked, then `update_changelog`. Assert the instruction names `finished` and not `test_plan`. goto `finished` succeeds without a feature changelog, and no test-plan or changelog store file is created.
  - Then task B: `update_changelog` names `test_plan`, and `finished` requires the changelog as today.
  - Whole-plan run: unchanged exits.

**Complexity**: Medium
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential. The FSM, auto-commit and template are tightly coupled.

### Phase 3.3: Scope the implement instructions and skill to the selected task

**Requirements covered**: Task runs use the whole plan's context; Agents can be asked to implement one task; Existing plans still implement (template side).

**File changes**:
- `templates/partials/implement-plan-documents.md:3-4`: the shared description of "the current work item". Task plans use the first unchecked `#### - [ ] Task:` (or, in a task run, `{{task.title}}` (`{{task.id}}`)) and its `### Task: <title>` section in context.md. Legacy plans use the first unchecked `#### - [ ] Phase N.M:` and `### Phase N.M:`. Keep the phrase asserted by `cmd/instruction_contract_test.go:517`, or update that assertion.
- `templates/steps/implement/01-read_plan.md:20-39,97-102`:
  - Accept `## Milestones & Tasks` or `## Milestones & Phases`.
  - Require at least one open task or phase.
  - Each task/phase has a technical-detail link.
  - In a task run, state that the whole plan, context, research and referenced designs are read, but only `{{task.title}}` will be implemented.
- `02-analyze.md:3-40`, `03-implement.md:9-10`, `04-test.md:9-10`, `05-verify.md:9-10`:
  - Replace "first unchecked `#### - [ ] Phase N.M:`" with the partial's current-item wording.
  - Add a `{{#task}}` block: "Work only on task `{{task.title}}` (`{{task.id}}`). Do not start, test or tick any other task."
  - In the "skip this phase" options (03:31, 05:35), say task.
- `06-update_plan.md:9-24`: tick `#### - [x] Task:` (or Phase) for the current item and its criteria only. In a task run, tick only `{{task.id}}`'s heading.
- `07-update_changelog.md:9-75`: the entry heading is `### <date> — Task: <title>` (legacy `Phase N.M:`), and the exits are driven by `last_task` (Phase 3.2).
- `09-test_plan.md`, `10-update_feature_changelog.md`, `11-reconcile_spec.md`, `12-finished.md:7,16`: change the wording to tasks.
- `templates/partials/git-commit-message.md:16`: "ticked task closed its milestone".
- `templates/skills/workflows/spek-implement/SKILL.md:15,34,51,62`:
  - Document `implement new --data '{"name":"<plan>","task":"<id>"}'`.
  - When the user names a task by title, resolve its id with `plan export <name> --format json`.
  - Resume uses the first unchecked Task (or Phase) or the recorded task.
  - Keep `{{> partials/implement-plan-documents}}` exactly once (`templates/skill_resume_test.go:67-77`).
- Helper skills: `templates/skills/skill_update-changelog.md:7,21-31,53,61`, `skill_verify-implementation.md:3-15`, `skill_follow-test-patterns.md`, `skill_spawn-implementation-agents.md`. Change phase to task, with a legacy note where they parse headings.
- Tests:
  - `internal/steps/implement/steps_test.go:270,276,362,384-390`: update the phrase assertions to the task wording plus the legacy fallback phrase.
  - Add render tests with `renderStepWithData` (:22-64) and `task` set, asserting `{{task.id}}` appears in the analyze, implement, test, verify and update_plan output, and does not appear when no task is set.
  - `internal/agent/instruction_surface_test.go` still passes (no stdin/heredoc).
  - The implement skill contains `"task"` in the `implement new` example.

**Complexity**: High
**Token estimate**: ~35k
**Agent strategy**: Parallel analysis, sequential integration. One agent does steps 01-07 plus the partials, one does 09-12 plus the skills. Integrate and run the template tests sequentially.

### Phase 4.1: Author plans as tasks in the plan workflow

**Requirements covered**: The planning process decides the executor deliberately; The plan review surfaces human tasks; Authored plans are made of tasks; Identifiers come from Spektacular (authoring side); Dependencies are human-readable.

**File changes**:
- `internal/steps/plan/steps.go:43-44,139-146`: rename the step `phases` to `tasks` (Name/Dst, `open_questions` Src, callback `tasks()` rendering `steps/plan/10-tasks.md`, `milestones()` next step `tasks`).
- `git mv templates/steps/plan/10-phases.md templates/steps/plan/10-tasks.md` and rewrite it to the design's format (`design:plan-task-graph.md` §Tasks in plan.md):
  - heading `#### - [ ] Task: <title>`;
  - `**Id:**` obtained by running `{{config.command}} plan task-id` once per new task, never invented and never changed for an existing task;
  - `**Repo:**`, exactly one;
  - `**Depends on:**` as `none` or `- <id> — <title>` lines;
  - `**Execution:**` as `agent` or `human — <reason>`;
  - the four human criteria verbatim from the design, and the split rule;
  - summary, `*Technical detail:* [context.md#task-<slug>](./context.md#task-<slug>)`, and acceptance criteria.
  - Context entry `### Task: <title>`. Working files `tasks_plan.md` and `tasks_context.md`.
- `templates/steps/plan/13-assemble.md:27,33,41`: `milestones.md` + `tasks_plan.md` go to `## Milestones & Tasks`, and `tasks_context.md` to `## Per-Task Technical Notes`.
- `templates/steps/plan/14-verification.md:15-25,52`: the required section is `## Milestones & Tasks`, and every task has Id, exactly one Repo, Depends on, and Execution (human with reason). Note that `plan file write` enforces the same rules, so fix any refusal before retrying.
- `templates/steps/plan/18-walkthrough.md:16`: beat 2 becomes "how the work breaks into tasks". Add: "Before asking for sign-off, name every task whose Execution is `human`, with its reason. If there are none, say so."
- Change the "phase" wording to tasks in `03-architecture.md:8`, `06-implementation_detail.md:7,21`, `08-testing_approach.md:7,31` and `09-milestones.md:18`.
- `templates/scaffold/plan.md:109-133`: use `## Milestones & Tasks` and a task example block with all four lines. This also fixes the missing `**Repo:**` drift.
- `templates/scaffold/context.md:8-20`: use `## Per-Task Technical Notes` with a `### Task: <title matching plan.md>` example. Add the missing `## Project References` section if verification still requires it (existing drift).
- `templates/skills/workflows/spek-plan/SKILL.md:15,62`: rename the step and mention `plan task-id`.
- `templates/agents/store-access.md:35`: "ticking a task checkbox in a plan". Then run `go run . init` against a temp project in tests only, never in this repo.
- Tests to update:
  - `internal/steps/plan/scaffold_test.go:27,51-52`;
  - `internal/steps/plan/steps_test.go:105,178,231,645-655`, renaming `TestPhasesStep…` to tasks and asserting `plan task-id`, the four criteria phrases and `human — `;
  - `cmd/instruction_contract_test.go:65-66`;
  - `cmd/autocommit_test.go:284`;
  - `cmd/cross_kind_test.go:123`;
  - `templates/work_files_test.go:48,136`;
  - add a walkthrough assertion for the human-task naming phrase.

**Complexity**: High
**Token estimate**: ~35k
**Agent strategy**: Parallel analysis, sequential integration. One agent does the step template, scaffolds and assemble/verification, one does the walkthrough, skill, AGENTS and the wording sweep. Integrate with a single test run.

### Phase 4.2: Update the glossary and end-to-end suites to tasks

**Requirements covered**: Old plans still implement (E2E); Authored plans are made of tasks (E2E); Human tasks are named in review and Mixed work is split (agent-judgement ACs, checked by harbor or manually).

**File changes**:
- Glossary, through the CLI only:
  - `go run . knowledge read` for `glossary/phase.md` (tier repo, name spektacular).
  - Write `glossary/task.md` via `go run . knowledge write` (staged under `.spektacular/tmp/`) with the tags `[workflow, nomenclature, plan]`: "A single unit of work in a plan, listed under a milestone, carried out in exactly one registered repo by an agent or a person, identified by a permanent id and declaring the tasks it depends on. A task sits below a milestone and is never a synonym for a step or a stage."
  - Remove `glossary/phase.md` with `go run . knowledge delete`. Follow `spek-knowledge` propose-then-confirm with the user before writing or deleting.
- `tests/harbor/plan-workflow/tests/test_plan_workflow.py:72,90,199`: change `"phases"` to `"tasks"` in `EXPECTED_STEP_ORDER` and `EXPECTED_SKILLS_PER_STEP`, and `"milestones & phases"` to `"milestones & tasks"`. Add an assertion that the final plan.md has `**Id:**`, `**Depends on:**` and `**Execution:**` on every task, and passes `spektacular plan export <name> --format json`.
- `tests/harbor/plan-workflow/solution/solve.sh:29,108-185`: the goto becomes `tasks`, the plan body moves to task format with ids from `spektacular plan task-id`, and the context headings become `### Task:`.
- `tests/harbor/plan-workflow/instruction.md:40`: step name.
- `tests/harbor/implement-workflow/`: leave `environment/plan.md` in legacy Phase format on purpose. Its passing run is the old-plan regression.
- Run `make harbor-test-plan` and `make harbor-test-implement`. Record the results in the changelog. Also run the agent-judgement checks from the Testing Approach manually if harbor does not cover them.

**Complexity**: Medium
**Token estimate**: ~15k (plus ~50 min of harbor runs)
**Agent strategy**: Single agent, sequential.

### Phase 4.3: Document the plan task format, export and single-task implement

**Requirements covered**: Public documentation covers the feature.

**File changes** (repo root `/home/nicj/code/github.com/jumppad-labs/spektacular-website`):
- `docs:src/pages/plan-tasks.mdx` (new): `layout: ../layouts/Shell.astro`. Sections follow the Content outline in plan.md. It is composed from `Hero`, `Section` + `Prose nested` + fenced blocks (the pattern in `docs:src/pages/design-documents.mdx:259-300`), `ConfigurationKeys`/`ConfigKey` for the per-line format rules and export fields, and alternating `surface` (convention). No `<div>`/`<section>`/`class=` in the body. No em dashes in prose. The em dash appears only inside the quoted plan.md code sample, because it is the format's separator.
- `docs:src/components/Nav.astro:6-21`: add `{ label: "Plan tasks", href: "/plan-tasks/" }` to the Resources `children`.
- `docs:src/pages/how-it-works.mdx`:
  - :129 stage tree `phases` becomes `tasks`;
  - :146-152 replace the phase definition with the task definition (Content example);
  - :394 change "breaks into phases" to tasks, and fix the "walkthrough is optional" drift to mandatory;
  - :413,417 use task wording;
  - link to `/plan-tasks/`.
- `docs:src/pages/configuration.mdx`: add `task_id: {provider: uuid}` to the sample YAML (:52-55), and in the `plan` ConfigKey (:190-203) a bullet: "`plan.task_id.provider`: where task identifiers come from; `uuid` (default) issues random UUIDs."
- Verify with `npm run build`, `npx astro check`, and `grep -nE "<div|<section|class=" src/pages/*.mdx` returning nothing.

**Complexity**: Medium
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential. It works in the docs repo root.

## Testing Strategy

- **Phase 1.1**: table-driven unit tests of Parse/Validate over hand-written plan fixtures, one per refusal rule plus legacy, mixed and out-of-section checkbox cases.
- **Phase 1.2**: `cmd` tests: each invalid write is refused and the stored bytes are unchanged; a legacy plan and non-plan documents save; ids survive the edit round trip.
- **Phase 1.3**: provider unit tests (v4 format, 200 unique); `cmd` tests for `plan task-id` default and unknown provider, and other commands unaffected.
- **Phase 1.4**: existing milestone tests unchanged, plus task-format twins; `unchecked_phases` counts tasks.
- **Phase 2.1**: `cmd` tests for JSON field set and order, pretty equality and layout, bad format, location from git source only (with a real remote on a file source), liveness after tick, draft/final status parity, missing and legacy plan errors; pretty renderer golden test.
- **Phase 2.2**: artifact-status tests for the 4-task fixture, criteria/completion separation, and legacy key-set stability; schema test.
- **Phase 3.1**: one `cmd` test per refusal with no state file written; a valid task persists `task` and appears in status; the whole-plan path is unchanged.
- **Phase 3.2**: FSM/step tests for the new edge and last-task routing on a two-task fixture; `finished` tolerance; auto-commit point pin test.
- **Phase 3.3**: template-render tests with and without `task`; updated phrase assertions; instruction-surface test still green.
- **Phase 4.1**: plan step/scaffold/contract/work-file tests updated to `tasks`; new assertions for `plan task-id`, human criteria, split rule, verification lines and walkthrough human-task naming.
- **Phase 4.2**: harbor plan-workflow and implement-workflow runs (not in CI), plus manual agent-judgement checks.
- **Phase 4.3**: `npm run build`, `npx astro check`, and the layout-HTML grep guard in the docs repo.
- Every phase ends with `go test ./...` green (shuffle on).

## Project References

- Design: `plan-task-graph.md` from the `design` design source, read with `go run . design read --data '{"source":"design","path":"plan-task-graph.md"}'`. It is binding for format, JSON shapes and refusal codes.
- Spec: `000058_plan-task-graph` (GitHub issue #50).
- Knowledge (repo `spektacular`): `conventions/error-messages-must-suggest-remediation.md`, `conventions/store-files-must-be-written-through-the-cli.md`, `conventions/tests-must-not-depend-on-order.md`, `conventions/tests-must-pass-for-done.md`, `glossary/phase.md`, `architecture/testing-architecture.md`, `architecture/workflow-steps.md`, `architecture/working-with-files-from-steps.md`, `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`.
- Knowledge (repo `docs`): `conventions/mdx-authoring.md`, `conventions/no-em-dashes.md`, `conventions/plan-content-pages.md`, `conventions/site-layout.md`, `conventions/alternate-section-background.md`.
- Repo roots: `spektacular` at `/home/nicj/code/github.com/jumppad-labs/spektacular`, and `docs` at `/home/nicj/code/github.com/jumppad-labs/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Phases 3.3 and 4.1 are the largest (a wide template and prose sweep). Split them across two agents by template group and integrate before running the template-contract tests.

## Migration Notes

- No config schema bump. `plan.task_id.provider` is optional and defaults to `uuid`.
- Existing plans are not migrated. Legacy phase plans keep whole-plan implement and milestone commits, and are refused only by export and single-task implement.
- Renaming the plan step `phases` to `tasks` means a plan workflow paused exactly at `phases` cannot resume across the upgrade (see Open Questions).
- Hive must adapt to `repo` and `execution` being objects rather than strings (tracked on issue #50).

## Performance Considerations

Parsing is a single linear pass over plan.md (typically well under 100 KB) per command, with no caching. Dependency cycle detection is O(tasks + edges). Export resolves repo metadata per distinct repo without cloning or running git. None of this is performance-sensitive.
