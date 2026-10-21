---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Context: 000068_task-goal-validation

## Current State Analysis

- **Implement steps.** The implement FSM runs `analyze → implement → test → verify → update_plan → update_changelog`, looping back to `analyze` (`internal/steps/implement/steps.go:27-42`).
  - `verify` launches a shell-capable sub-agent that runs commands and returns pass/fail (`templates/steps/implement/05-verify.md`, `templates/skills/skill_verify-implementation.md`).
  - `update_plan` ticks a criterion "only if that criterion actually passed verification" (`templates/steps/implement/06-update_plan.md`).
  - No independent judgement of the criteria exists, and the implementing agent decides what passed.
- **Plan reader.** `plantask` parses tasks, ids, repos, dependencies, execution and acceptance-criteria counts (`internal/plantask/plantask.go:73-98`, `224-276`). It keeps no criterion text. Legacy `Phase N.M` headings are only counted (`:103`, `187-201`). Task files come from `context.md` `### Task:` sections (`internal/plantask/files.go:37-104`).
- **Orchestrated runs.** Every step of an orchestrated run gets the `QUESTION:`/`FAILED:` hand-back appended (`internal/stepkit/stepkit.go:106-143`). The epic orchestrator merges on `DONE:` or `awaiting_merge` (`templates/skills/workflows/spek-implement-epic/SKILL.md` Step 5), but `epic merge` itself checks no workflow state (`cmd/epic_worktree.go:143-209`).
- **Agent installs.** Installs write skills, wrappers and AGENTS.md sections only. No agent installs a sub-agent definition (`internal/agent/{claude,codex,copilot,omp,bob}.go`).
- **Configuration.** It has `implement.worktrees` only (`internal/config/config.go:155-163`). The settings format is 4 (`internal/config/schema.go:19`).
- **Harbor.**
  - `implement-workflow` is Claude-only with one fixed scenario starting at `test_plan`.
  - `implement-epic-workflow` is agent-independent, reading CLI state and the session log, with a hard-coded solution step list (`solution/solve.sh:16`).
  - The Bob harbor agent's files sit in `tests/harbor/implement-epic-workflow/agents/`, while the Makefile looks in `tests/harbor/agents/`.
- **Docs.** No per-step workflow reference page exists. `how-it-works.mdx` has the step tree and the Implement stage. `plan-tasks.mdx:88-93` states today's tick rule, and `configuration.mdx:301-313` documents `implement`.

**Requirement → repo and files**

| Requirement | Repo | Files |
|---|---|---|
| Each task has a goal | spektacular | `internal/plantask/plantask.go`, `internal/plantask/files.go`, `internal/goal/goal.go` |
| Independent check before done; only "met" ticked | spektacular | `internal/steps/implement/steps.go`, `templates/steps/implement/05b-check_goal.md`, `06-update_plan.md`, `internal/goal/decide.go` |
| Fresh context; reads the work; read-only | spektacular | `templates/steps/implement/05b-check_goal.md`, `templates/partials/goal-reviewer-brief.md`, `templates/subagents/*`, `cmd/implement_goal.go` (tree fingerprint) |
| A verdict and evidence for every criterion | spektacular | `internal/goal/decide.go`, `cmd/implement_goal.go` |
| Not yet met retried; stop when attempts run out; impossible stops | spektacular | `internal/goal/decide.go`, `templates/steps/implement/05b-check_goal.md`, `03-implement.md` |
| Orchestrated stop reaches the user; no merge while open | spektacular | `cmd/epic_worktree.go`, `templates/skills/workflows/spek-implement-epic/SKILL.md` |
| Fast, inexpensive model; works with every agent; per-agent override | spektacular | `internal/agent/subagents.go`, `internal/agent/{claude,codex,copilot,omp}.go`, `internal/config/config.go`, `README.md` |
| On by default, can be switched off | spektacular | `internal/config/config.go`, `internal/workflow/workflow.go`, `cmd/implement.go` |
| The outcome is recorded | spektacular | `internal/goal/store.go`, `cmd/implement_goal.go` |
| Design updated (constraint) | spektacular | design `design:plan-task-graph.md` via `design author` |
| Harbor surfaces (constraint) | spektacular | `tests/harbor/implement-workflow/*`, `tests/harbor/implement-epic-workflow/*`, `tests/harbor/goal-check-workflow/*`, `Makefile` |
| Documented | docs | `docs:src/pages/how-it-works.mdx`, `docs:src/pages/plan-tasks.mdx`, `docs:src/pages/configuration.mdx`, `docs:CHANGELOG.md` |

## Per-Task Technical Notes

### Task: Teach the plan reader criterion text and legacy phases

Repo: `spektacular` (`/home/nicj/code/github.com/hivecommons/spektacular`; in an epic run, the spec's worktree for this repo).

- `internal/plantask/plantask.go:73-77`: add `Items []Criterion` to `Criteria` with tag `json:"-"`, so `status` (`internal/status/report.go:132-135`) and `plan export` JSON are unchanged. Add `type Criterion struct{ Text string; Met bool }`.
- `internal/plantask/plantask.go:224-276` (`readTaskLine`): when `inCriteria` and the line matches `criterionBox`, append a `Criterion` with the text after the `- [ ] ` / `- [x] ` box, trimmed. Bold titles such as `**Title**` stay as written. Continuation lines indented under a criterion (spec-style detail) are appended to the previous criterion's text with a space.
- `internal/plantask/plantask.go:46-56,103,187-201`: add `Phases []Phase` to `Plan`. When `legacyHeading` matches, open a `Phase{Label: "Phase N.M", Title, Completed}`; extend the heading regexp to capture `N.M` and the title. Reuse the criteria-block reading, so a phase's `**Acceptance criteria**:` boxes fill `Phase.Criteria`. The existing `openPhases`/`phases` counters, `OpenItems` and `Items` (`:339-354`) keep their values.
- `internal/plantask/plantask.go:315-334`: add `func (p Plan) Phase(label string) (Phase, bool)` and `func (p Plan) OpenPhases() []Phase`.
- `internal/plantask/files.go:66-104` (`contextTaskSections`): also split `### Phase N.M: <title>` sections, keyed by label. Add `func PhaseFiles(plan, context []byte, repos []string) map[string][]FileRef`, keyed by phase label, sharing `fileRef` and the fence-skipping logic.
- `internal/plantask/plantask_test.go`, `files_test.go`: tests for criterion text, ticked state, multi-line criteria, legacy phases with criteria, and phase files. A golden check that `NewExport`/`RenderPretty` and `Criteria` JSON for an existing fixture are unchanged.

**Complexity**: Medium
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential: parser first, then tests.

### Task: Add the goal-check configuration

Repo: `spektacular`.

- `internal/config/config.go:155-163`: add `GoalCheck GoalCheckConfig \`yaml:"goal_check"\`` to `ImplementConfig`. Add `type GoalCheckConfig struct { Enabled bool \`yaml:"enabled"\`; Models map[string]string \`yaml:"models,omitempty"\` }` with a doc comment saying it is on unless switched off, including in configs written before the key existed (parsed over `NewDefault`).
- `internal/config/config.go:396`: `Implement: ImplementConfig{Worktrees: true, GoalCheck: GoalCheckConfig{Enabled: true}}`.
- `internal/config/config.go:639` (`Validate`): refuse a `models` key that is not one of the supported agent names. Keep the list in `internal/config` as `SupportedAgentNames = []string{"bob","claude","codex","copilot","omp"}`, because `config` cannot import `internal/agent`. Add a test in `internal/agent` pinning it equal to `agent.Supported()`. Refuse an empty model value. The message names the valid agents and suggests removing or renaming the key (convention: errors suggest remediation).
- `internal/config/schema.go:19`: no bump (`CurrentProjectSchema` stays 4). Add a comment beside `ImplementConfig` that `goal_check` needs no migration.
- `internal/workflow/workflow.go:30-41`: add `GoalCheck bool` to `workflow.Config` (runtime-only, like `CodeRoots`).
- `cmd/implement.go:207` and `:311`: set `GoalCheck: cfg.Implement.GoalCheck.Enabled` in both `workflow.Config` literals (new and goto). Grep `cmd/` for every other `workflow.Config{...Kind: "implement"` literal (resume, status helpers) and set it there too.
- `internal/config/config_test.go`:
  - default on;
  - an absent key loads as on (pattern: `TestFromYAMLFile_AbsentImplementWorktreesLoadsAsOn` at `:214`);
  - `enabled: false` round-trips, asserting the raw file text (pattern `:231`);
  - a `models` map round-trips;
  - an unknown agent is refused.
- `cmd/implement_test.go`: a run's workflow config carries the flag on new and goto.

**Complexity**: Low
**Token estimate**: ~10k
**Agent strategy**: Single agent, sequential.

### Task: Add the goal package

Repo: `spektacular`.

- `internal/goal/goal.go` (new): `Goal`, `Scope`, `Verdict`, `Check`, `Decision`, `ItemOutcome`, `Outcome`, `MaxRetries = 2` as in plan.md § Data Structures & Interfaces.
  - `Build(planBody, contextBody []byte, item string, repos []string) (Goal, error)` uses `plantask.Parse`, `Plan.Task`/`Plan.Phase`, and `TaskFiles`/`PhaseFiles`.
  - `Scope.Repo` is the task's `Repo`, or the only registered repo for a legacy phase.
  - `Scope.Rule` is the fixed sentence "Change only the files that belong to this item; change nothing that belongs to another task."
  - Errors use `output.NewError("goal_item_not_found", …).WithNextAction("spektacular plan file read <plan> plan")`.
- `internal/goal/current.go` (new): `Current(p plantask.Plan, outcome Outcome, selected string) (item string, ok bool)`.
  - Returns `selected` when non-empty.
  - Otherwise returns the first open task (task format) or open phase (legacy) whose `ItemOutcome.State != "skipped"`.
  - It mirrors `templates/partials/implement-current-task.md`.
- `internal/goal/decide.go` (new):
  - `ValidateReply(g Goal, raw []byte) ([]Verdict, error)`: exact N, 1-based, no duplicates, verdict in {met, not_met, impossible}, non-empty trimmed evidence; the error names each problem (`goal_reply_invalid`).
  - `Next(item ItemOutcome, verdicts []Verdict) (decision string, round, attempt int)` applies:
    - any impossible → `impossible`;
    - all met → `accept`;
    - not met with `attempt < 1+MaxRetries` → `retry`;
    - otherwise `exhausted`.
  - `ApplyDecision(item ItemOutcome, d string) (ItemOutcome, nextStep string, err error)`: valid only when the latest check's decision is `exhausted` or `impossible` (`goal_decision_not_due` otherwise).
    - `keep_fixing` → new round, next `implement`;
    - `recheck` → new round, next `check_goal`;
    - `accept` → `accepted_shortfall`, next `update_plan`;
    - `skip` → `skipped`, next `update_plan`.
  - `State` derivation:
    - `pending` when there is no check;
    - `retrying` after `retry`;
    - `awaiting_decision` after `exhausted`/`impossible` with no later decision;
    - `accepted` after `accept`;
    - `accepted_shortfall` after an `accept` decision;
    - `skipped` after a `skip` decision.
  - `Settled(item) bool` is true for the last three.
  - `MetCriteria(item) []int` comes from the latest check.
- `internal/goal/store.go` (new):
  - `FilePath(planDir, plan string) string` returns `planDir + "/" + plan + "/goals.json"`, beside `implement.PlanDocumentPath` (`internal/steps/implement/strategy.go:40-44`). It is a layout helper only and never rendered into agent output (knowledge: working with files from steps).
  - `Load(st store.Reader, planDir, plan) (Outcome, error)` treats `ErrNotFound` as empty.
  - `AppendCheck` and `AppendDecision` read-modify-write through `st.Write`, stamping `At` as RFC3339 UTC.
  - `Attempts` and `State` are derived on load, never trusted from disk.
- Tests (`internal/goal/*_test.go`, testify `require`, `t.TempDir()` with `store.NewFileStore`):
  - building for a task and a legacy phase includes no other item's criteria;
  - every `ValidateReply` failure;
  - the full decision table, including impossible on attempt 1 (no retry counted), two retries then exhausted, keep_fixing resetting the budget, recheck, accept and skip;
  - `Current` skips skipped items;
  - a store round trip.

**Complexity**: Medium
**Token estimate**: ~30k
**Agent strategy**: 2 parallel agents: one for build/current/store, one for decide/validate. Integrate and test sequentially.

### Task: Add the implement goal commands

Repo: `spektacular`.

- `cmd/implement_goal.go` (new): `implementGoalCmd` with `record`, `decide` and `show`, registered as `implementCmd.AddCommand(implementGoalCmd)` alongside `cmd/implement_merge.go:68`. Each supports `--schema` like `cmd/plan_task_id.go:24-33`.
  - `record`:
    1. Takes `--data '{"name","item"}'` and `--from <path>` (required; refusal `from_required` naming `.spektacular/tmp/<plan>/goal-check/reply.json`).
    2. Loads config; refuses `goal_check_disabled` when `!cfg.Implement.GoalCheck.Enabled`, with next action "set `implement.goal_check.enabled: true` in .spektacular/config.yaml".
    3. Reads plan and context through the project store (`store.NewSourceStore(root, "project")`, `implement.PlanFilePath`/`ContextFilePath`) and builds the goal.
    4. Checks the item is current for the active implement workflow or lane: `selected` comes from workflow data `task` (the lane is found the way `cmd/workflow_slot.go:55-89` resolves `goto` by `name`). Otherwise refuses `goal_item_not_current`, naming the current item.
    5. Validates, appends, and writes the result with `next_step` and `next_action` (`<command> implement goto --data '{"step":"<next>","name":"<plan>"}'`).
    6. For `exhausted`/`impossible` the `next_action` is the `decide` command shape with the four options.
    7. Before appending, compares each code root's live `git diff HEAD` and `git status --porcelain --untracked-files=all` (through `internal/gitexec`; the roots come from the run's worktree code roots, else the registered repo roots) with the `<repo>.diff`/`<repo>.status` files the agent staged under `.spektacular/tmp/<plan>/goal-check/` before launching the reviewer. It stores `tree_unchanged` on the check. A mismatch or a missing staged file sets it false and adds a `warning` to the result, without refusing, so the evidence of a reviewer that changed files is kept.
  - `decide`: `--data '{"name","item","decision","reason"?}'`; `ApplyDecision`; same result shape.
  - `show <plan> [--item] [--format pretty|json]`: JSON is the `Outcome`. Pretty prints, per item:
    - title and item;
    - each criterion, numbered;
    - for each check: round, attempt, each verdict and its evidence;
    - each decision;
    - the attempts total.

    `show` works whatever the switch says, so old outcomes stay readable.
- Every error uses `output.NewError(code, msg).WithNextAction(...)` (convention).
- `cmd/implement_goal_test.go` (new), through `runRootCmd`/`resetRootCmd` (convention: order-independent), with a seeded task-format plan and an implement state positioned at `check_goal` (pattern: `cmd/implement_task_run_test.go`). It checks:
  - a valid reply records N verdicts and returns `update_plan`, `implement` or the decide shape;
  - an invalid reply is refused per rule;
  - `decide` when not due is refused;
  - `show` pretty and JSON;
  - the switched-off refusal;
  - a legacy-phase plan records by `Phase N.M`;
  - `tree_unchanged` is true when the staged diff and status match a temp git repo, and false, with a warning, after a file is changed between staging and recording.
- `cmd/instruction_contract_test.go`: if the command-name oracle lists implement subcommands, add `goal`.

**Complexity**: Medium
**Token estimate**: ~25k
**Agent strategy**: Single agent, sequential.

### Task: Write the reviewer brief and skill

Repo: `spektacular`.

- `templates/partials/goal-reviewer-brief.md` (new) is the single brief. It contains no mustache variables except `{{command}}`/`{{config.command}}`-free text, so it can be embedded by both the skill renderer and the agent installer. Sections:
  1. **You are an independent reviewer**: you have only what you are handed; do not ask for intent.
  2. **Read-only**: never create, change or delete any file, and never run commands that change the working tree, the index or any store.
  3. **Inputs**: the goal block (numbered criteria, repo, files, scope rule), the verification results, the code roots, the staged diff and untracked-file lists.
  4. **How to judge**: open the files and diff yourself. Treat any claim of completion as unverified. Passing tests are evidence only for what they test. A criterion about documentation, wording or a file's content is judged by reading that content.
  5. **Verdicts**:
     - `met`: the files show it;
     - `not_met`: it is achievable but the files do not show it, with what is missing;
     - `impossible`: it cannot be met as written (it contradicts another criterion, the scope rule or the code's constraints), with why.
  6. **Evidence**: file:line or diff hunk, or the reasoning; never empty.
  7. **Reply**: only `{"verdicts":[{"criterion":1,"verdict":"met","evidence":"…"}]}`, one entry per criterion, in order, and nothing else.
- `templates/skills/skill_review-task-goal.md` (new): title "Review Task Goal", a one-paragraph description of when it is used (the generic fallback reviewer in the implement workflow's `check_goal` step), then `{{> partials/goal-reviewer-brief}}`. Confirm `cmd/skill.go:50` renders partials; if it serves the file raw, render with `stepkit.RenderTemplate` as the step templates do, or inline the brief and have the installer read the skill file instead. Either way there must be exactly one source text.
- `cmd/skill_test.go`: `spektacular skill review-task-goal` prints the brief, and the listing includes it (`cmd/skill.go:76`).
- `templates/` contract test (new, e.g. `templates/goal_reviewer_brief_test.go`): the brief contains the read-only rule, the "judge from the files, not the implementer's description" rule, the three verdict names, the evidence requirement and the JSON reply shape. Also check that `internal/agent/instruction_surface_test.go:73` walks `partials`, which it already does.

**Complexity**: Low
**Token estimate**: ~10k
**Agent strategy**: Single agent, sequential.

### Task: Add the check_goal step to the implement workflow

Repo: `spektacular`.

**Callbacks and FSM**

- `internal/steps/implement/steps.go:27-42`:
  - add `{Name: "check_goal", Src: []string{"verify"}, Dst: "check_goal", Callback: checkGoal()}` after `verify`;
  - `implement` gets `Src: []string{"analyze", "check_goal"}`;
  - `update_plan` gets `Src: []string{"verify", "check_goal"}`;
  - update the doc comment to describe the goal loop.
- `steps.go:119-127` (`verify`): next step is `"check_goal"` when `cfg.GoalCheck`, else `"update_plan"`.
- `steps.go` new `checkGoal()`:
  1. Loads plan, context and outcome through `st` (`internal/goal`).
  2. Resolves the current item with `goal.Current` (selected = `task` data).
  3. Builds the goal and renders `steps/implement/05b-check_goal.md` with extra:
     - `goal` (item, title, numbered criteria, repo, files, rule);
     - `attempt` and `round` (next attempt number), `retries_left`;
     - `awaiting_decision`, with the last decision `exhausted`/`impossible` and the verdict list, so a re-render after a stop shows the stop again;
     - `goal_tmp_dir` = `.spektacular/tmp/<plan>/goal-check`.
  4. `next_step` is `update_plan`. The template names both exits explicitly.
  5. With `st == nil` (dry run), renders with a placeholder goal.
- `steps.go:99-107` (`implementStep`): when `cfg.GoalCheck` and the current item's state is `retrying`, add extra `goal_retry` holding the not-met criteria with their evidence from the latest check, and `attempt`.
- `steps.go:129-137` (`updatePlan`): when `cfg.GoalCheck`:
  - refuses with `output.NewError("goal_check_not_settled", …)` unless `goal.Settled(item)`. The next action is `goto check_goal` (or the decide shape when awaiting a decision). With no record at all the message says the check has not run, and the next action also names switching `implement.goal_check.enabled` off for this task (a run upgraded mid-task, sitting at `update_plan`, cannot go back to `check_goal`).
  - Then adds extra `goal_check: true`, `goal_met` (the met criteria texts), `goal_skipped` (bool) and `goal_shortfall` (bool).
- `steps.go:146-171` (`updateChangelog`): in a task run, `last` counts open tasks excluding skipped ones. For the whole-plan branch, add extra `goal_check` so the template's count excludes skipped items.

**Templates**

- `templates/steps/implement/05b-check_goal.md` (new), structured like `05-verify.md`:
  - Step 1, load the current item: `{{> partials/implement-current-task}}`, plus the goal block exactly as rendered, which is the goal to judge.
  - `{{> partials/implement-code-locations}}`.
  - Step 2, stage the work: for each code root, write `git -C <root> diff HEAD` to `{{goal_tmp_dir}}/<repo>.diff` and `git -C <root> status --porcelain --untracked-files=all` to `{{goal_tmp_dir}}/<repo>.status`, using the agent's own file tools.
  - Step 3, launch the reviewer in a fresh context: use the `spek-goal-reviewer` helper if the agent has one; under Bob use the built-in `explore` helper; otherwise any fresh-context sub-agent given the brief from `{{config.command}} skill review-task-goal` verbatim. The input is exactly the goal block, the verify step's pass/fail summary, the code roots, the staged paths and the item's files, "and nothing from your own conversation: no summary of what you did, no claim that criteria are met". The reviewer must not be given write tools where the agent lets you choose.
  - Step 4, record: write the reviewer's reply unchanged to `{{goal_tmp_dir}}/reply.json`, run `{{config.command}} implement goal record --data '{"name":"{{plan_name}}","item":"{{goal.item}}"}' --from {{goal_tmp_dir}}/reply.json`, then remove the scratch files. Never edit or summarise the reply.
  - Step 5, act on the decision:
    - `accept` → goto `update_plan`;
    - `retry` → goto `implement`;
    - `exhausted` → STOP: report each not-met criterion with evidence and ask to keep fixing, accept the shortfall or skip the task;
    - `impossible` → STOP immediately: report the criterion and evidence and ask how to proceed (fix the criterion in the plan through `plan file write`, then `recheck`; accept; skip; or keep fixing). Record the answer with `{{config.command}} implement goal decide …` and run the `goto` it returns.
  - `{{> partials/implement-spec-conflict}}`.
  - Advance block listing both gotos.
  - No orchestrator wording: the appended partial handles hand-backs (`cmd/orchestrated_test.go:94`).
- `templates/steps/implement/03-implement.md`: add a `{{#goal_retry}}` block after Step 1: "The goal check judged these criteria not yet met (attempt N). Fix them:", followed by the criteria with evidence.
- `templates/steps/implement/06-update_plan.md`: wrap today's criteria rules in `{{^goal_check}}…{{/goal_check}}`, keeping them byte-identical. Add a `{{#goal_check}}` branch:
  - tick the heading unless `goal_skipped`;
  - tick exactly the listed `goal_met` criteria and no other;
  - leave the rest unticked.
  - A shortfall or skip is recorded under Deviations in the next step.
- `templates/partials/implement-current-task.md` (whole-plan branch) and `07-update_changelog.md` Step 3: add a `{{#goal_check}}` clause: "skip any task or phase that `{{config.command}} implement goal show {{plan_name}}` reports as skipped". Step 2 adds: an accepted shortfall or skip is named under **Deviations**.

**Tests**

- `internal/steps/implement/steps_test.go`:
  - `TestStepsOrderMatchesExpected` (`:92-112`) includes `check_goal`;
  - new wiring tests for the `implement`/`update_plan`/`check_goal` Src sets;
  - the FSM walk (`:147`) with `GoalCheck` on and off;
  - the retry block rendering;
  - the `update_plan` refusal and met-only list;
  - `check_goal` content assertions (reviewer helper, `explore`, the fallback skill, the "nothing from your own conversation" rule, the record command with `--from`, both STOPs);
  - add `check_goal` to the STOP-directive map (`:437`), `subAgentCodeSteps()` (`:748`) and `specConflictSteps()` (`:1108`);
  - with the check off, `06-update_plan` renders byte-identically to the current text.
- `cmd/instruction_contract_test.go:82`: change the verify row's next step to `check_goal`, and add `{"implement","check_goal","steps/implement/05b-check_goal.md","update_plan"}`.
- Hard-coded walks to update so they pass through `check_goal` (or set `GoalCheck` off where the test is about something else):
  - `cmd/implement_lane_test.go:198,222`
  - `cmd/epic_flow_test.go:102`
  - `cmd/autocommit_test.go:319,509,800`
  - `cmd/implement_task_run_test.go:37,113,154`
  - `cmd/orchestrated_commit_test.go:236`
  - `cmd/milestones_test.go:132`
  - `cmd/implement_worktree_flow_test.go:78`

  Walks that reach `update_plan` with the check on must seed or record an accepted outcome first.
- `internal/autocommit/points_pin_test.go:21` still passes; no commit point changes.
- `templates/work_files_test.go:73` and `repo_source_test.go`: include the new template where they enumerate implement templates.
- `templates/skills/workflows/spek-implement/SKILL.md:15`: add "check the task's goal" to the example step list.

**Complexity**: High
**Token estimate**: ~60k
**Agent strategy**: Parallel analysis, sequential integration: one agent for the callbacks, FSM and Go tests, one for the templates and contract tests, then a sequential pass updating the hard-coded `cmd` walks and running `go test ./...`.

### Task: Install a reviewer helper for every agent

Repo: `spektacular`.

- `templates/subagents/` (new folder, kept apart from `templates/agents/`, which holds AGENTS.md fragments):
  - `claude.md`: frontmatter with `name: spek-goal-reviewer`, `description`, `tools: Read, Grep, Glob`, `model: {{model}}`, and the body `{{> partials/goal-reviewer-brief}}`.
  - `codex.toml`: `name`, `description`, `model = "{{model}}"`, `developer_instructions` holding the brief plus "Never edit, create or delete files; you are read-only." Add `sandbox_mode = "read-only"` if current Codex custom agents support it (confirm against Codex docs).
  - `copilot.agent.md`: frontmatter with `name`, `description`, `tools: ['read', 'search']`, `model: {{model}}`, and the brief.
  - `omp.md`: frontmatter with `name`, `description`, read-only tools as omp names them, `model: "{{model}}"`, and the brief.
- `internal/agent/subagents.go` (new):
  - `reviewerModel(cfg config.Config, agent string) string` returns `cfg.Implement.GoalCheck.Models[agent]` if set, else the default.
  - The defaults are `claude: "haiku"`, `codex:` the mini tier of Codex's default model, `copilot:` a cheap Copilot model, and `omp: "@smol"`.
  - Confirm the Codex and Copilot names against current docs before writing them. Record the chosen names in a comment with the source.
  - `installReviewer(projectPath, relPath, tmpl string, cfg, agent, out)` renders with `sourceFS` and mustache plus partials (as `skills.go:46-71`), writes with `writeFileAtomic` (`managed_section.go:220`), and prints `  Agent:    <path>`.
- Call sites:
  - `internal/agent/claude.go:18-50` → `.claude/agents/spek-goal-reviewer.md`
  - `codex.go:13-39` → `.codex/agents/spek-goal-reviewer.toml`
  - `copilot.go:15-41` → `.github/agents/spek-goal-reviewer.agent.md`
  - `omp.go:20-41` → `.omp/agents/spek-goal-reviewer.md`
  - `bob.go:18-47`: no call.
- `internal/agent/instruction_surface_test.go:73`: add `subagents` to `instructionTemplateRoots`. `templates/data_payload_wellformed_test.go:43` walks `.md` only; extend it, or exempt the `.toml` explicitly.
- Tests to update:
  - `copilot_test.go:46`: tree count 9 → 10;
  - `codex_test.go:47-50`: still no `.claude`;
  - `omp_test.go:38-93`: snapshot covers `.omp/agents`;
  - `claude_test.go`;
  - `cmd/init_test.go:403-420`: second run is byte-identical;
  - `cmd/copilot_test.go`, `cmd/omp_test.go`, `cmd/migrate_test.go:93,232`: migrate restores the helper.
- New cross-agent test (pattern: `design_trigger_test.go:133-150`): for every agent except bob, install writes the helper with the default model and read-only tools. An override for one agent changes only that agent's file. Bob writes nothing under `.bob/agents`.
- `README.md:100-145` (agents section): add a short "Goal reviewer" paragraph covering the helper name and path per agent, default models, the `implement.goal_check.models` override, re-running `spektacular init <agent>` after changing it, Bob's `explore`, and that Copilot prefers `.github/agents` and ignores the model under `Auto`.
- Never run `init`/`migrate` on this repository itself (convention: a plan never changes the active install; note this repo's `.codex` is a file). Verify with `t.TempDir()` projects only.

**Complexity**: Medium
**Token estimate**: ~30k
**Agent strategy**: 2-3 parallel agents: templates and the installer helper, per-agent call sites with tests, and the README. Integrate sequentially.

### Task: Keep epic merges off an open goal-check question

Repo: `spektacular`.

- `cmd/epic_worktree.go:143-157` (`epic merge` run func): before `writeMergeResult` (`:178-209`), read the spec's implement lane (`workflow.ReadLane(dataDir, "implement", spec)`, `internal/workflow/lane.go`). If it is `InProgress()` and `CurrentStep == "check_goal"`, return `output.NewError("goal_check_open", "<spec>'s implement run is stopped at the goal check waiting for an answer").WithResource(spec).WithNextAction("answer the spec's open question and resume its run: spektacular implement new --data '{\"name\":\"<spec>\",\"orchestrated\":true}'")`. `implement merge` (`cmd/implement_merge.go`) is left unchanged.
- `cmd/epic_worktree_test.go`: a lane at `check_goal` is refused with `goal_check_open`; a lane at `verify`, and no lane at all, behave as before.
- `templates/skills/workflows/spek-implement-epic/SKILL.md:82`: add to the genuine-question list "the goal check running out of attempts or judging a criterion impossible". Add a line to Step 4's child prompt (`:60-70`): "If the goal check stops your task, hand it back as a `QUESTION:` with the not-met or impossible criteria and their evidence."
- `templates/implement_epic_skill_test.go`: phrase assertions for both additions.
- `internal/steps/implement/orchestrated_test.go`: an orchestrated `check_goal` render carries `## Running under an orchestrator` and `QUESTION: 000007_billing`; a standalone render carries neither (pattern `:19-26`).

**Complexity**: Low
**Token estimate**: ~12k
**Agent strategy**: Single agent, sequential.

### Task: Update the implement and implement-epic harbor suites

Repo: `spektacular`.

- `tests/harbor/implement-workflow/tests/test_implement_workflow.py:57-70` (`EXPECTED_STEP_ORDER`): insert `"check_goal"` between `"verify"` and `"update_plan"`.
- `tests/harbor/implement-workflow/environment/state.json:4-12` (`completed_steps`): insert `"check_goal"` after `"verify"`.
- `tests/harbor/implement-workflow/environment/plan.md`: its ticked criteria are pre-ticked and its run starts at `test_plan`, so no outcome is needed. Confirm nothing in its flow reaches `update_plan`.
- `tests/harbor/implement-epic-workflow/solution/solve.sh:16`: STEPS becomes `analyze implement test verify check_goal update_plan …`. At `check_goal`, `goto()` writes a reply judging every criterion `met`, with evidence "solution walk", built from `spektacular implement goal show <spec> --format json` or the plan, then runs `implement goal record` before the next goto.
- `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py`: add `test_every_task_has_recorded_verdicts`. For each seeded spec, `cli("implement","goal","show",spec,"--format","json")` must list every task id from `plan export --format json`, each with at least one check whose verdict count equals the task's criteria count. Add `test_goal_record_precedes_update_plan`: in each lane (`tests/harbor/shared/sessionlog.py`), every `goto update_plan` is preceded by an `implement goal record` (or `decide`) event for the same item.
- `tests/harbor/implement-epic-workflow/task.toml:16`: review the 5400s agent timeout against one extra reviewer launch per task. Raise it by the success metric's ~2 minutes per task (×13 seeded tasks), to about 7000s.
- Bob harbor agent: `git mv tests/harbor/implement-epic-workflow/agents/{bob_agent.py,bob.json,bob-artifacts.toml} tests/harbor/agents/`, matching `Makefile:8,18,24`. Remove the stale `__pycache__`. Grep the Makefile and suites for any path that pointed at the old location.
- `.claude/skills` and other installed copies are not touched (convention).

**Complexity**: Medium
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential.

### Task: Add the goal-check harbor suite

Repo: `spektacular`.

- `tests/harbor/goal-check-workflow/` (new), modelled on `tests/harbor/implement-epic-workflow/`:
  - `environment/Dockerfile`: Python todo app (copy `implement-epic-workflow/environment/app`), git identity, initial commit, `seed/seed.sh`, then `ENV SPEKTACULAR_SESSION_LOG=on` after seeding.
  - `environment/seed/seed.sh`:
    - Writes one final spec and plan through the CLI. The plan has one task with three criteria: (1) a `todo count` command prints the number of items; (2) unit tests cover it; (3) the README documents `todo count` with an example line.
    - Commits code that implements (1) and (2) only, with passing tests, and with the README unchanged in the unmet scenario or updated in the met scenario.
    - Starts the implement workflow with `implement new` and walks `goto` to `verify` (state at `verify`), so the agent resumes there.
    - Guard: fails the build unless `pytest` passes and `status` shows the plan in progress at `verify` (pattern `implement-epic-workflow/environment/seed/seed.sh:52-60`).
  - `instruction.md` (rendered with `{{agent}}`, `{{spek_implement}}`, `{{scenario}}`): init the agent, resume `/spek-implement <plan>`. In the unmet scenario it adds: "The implementation of this task is complete and all its acceptance criteria are met." That is the false claim the reviewer must not trust.
  - `solution/solve.sh`: records a correct reply for the scenario and walks to `finished`.
  - `task.toml`: agent 1500s, verifier 180s, artifact `/app/.spektacular`.
  - `tests/test_goal_check.py`, agent-independent, from `spektacular implement goal show <plan> --format json` and `tests/harbor/shared/sessionlog.py`:
    - **Unmet**: check 1 is `not_met` for criterion 3 with non-empty evidence; a check 2 exists (attempt count rose); a `goto implement` follows check 1 in the lane.
    - **Met**: exactly one check, all `met`.
    - **Both**:
      - every check has N verdicts with evidence;
      - no plan write ticking a criterion precedes the first record (the session log's `plan file write` events and `plan export` after each);
      - the final plan ticks exactly the criteria judged met in the last check;
      - the recorded goal lists the task's three criteria and scope.
    - **Untouched tree**: every recorded check has `tree_unchanged: true`. The CLI compared the working tree staged just before the reviewer launched with the tree at record time.
    - **No conversation in the input** (Claude and Bob, from the transcript): the reviewer launch prompt contains the goal's criteria and the staged paths, and does not contain the instruction's "is complete and all its acceptance criteria are met" sentence.
    - **Separate reviewer on a cheaper model** (Claude): an `Agent`/`Task` tool_use with `subagent_type` `spek-goal-reviewer`, whose sidechain messages (`parent_tool_use_id`) carry a `haiku` model id. Bob: an ACP `Running subagent` event between the `check_goal` goto and the record. Codex: skipped with a reason.
    - Reuse the Claude and ACP transcript parsing from `tests/harbor/plan-workflow/tests/test_plan_workflow.py:324-363,429-458`, copied into `tests/harbor/shared/transcripts.py` and copied in by the Makefile like `sessionlog.py`.
  - `tests/test.sh`: timeline, then pytest, then reward.
- `Makefile`: `harbor-test-goal-check[-claude|-codex|-bob]` with `SCENARIO=unmet|met` (default `unmet`), rendering to `tests/harbor/.build/goal-check-workflow-$(AGENT)-$(SCENARIO)` with `{{agent}}`/`{{spek_implement}}`/`{{scenario}}` substituted (pattern `Makefile:209-252` and the repo suite's scenario render `:124-130`), exporting `SPEK_SCENARIO`. Add the new targets to `.PHONY`.
- `tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py` (excluded from the image, pattern `tests/harbor/repo-workflow/tests/test_verifier_selfcheck.py`): each pure rule checked against a synthetic outcome that honours it and one that breaks it.
- `knowledge` (no write here): the orchestrator may offer a knowledge entry on the suite's scenario mechanics afterwards.

**Complexity**: High
**Token estimate**: ~50k
**Agent strategy**: Parallel analysis, sequential integration: one agent for the environment and seed, one for the verifier and self-check, then a sequential pass on the Makefile and render.

### Task: Update the tick rule in the plan-task-graph design

Repo: `spektacular` (the `design` source lives in this project).

- Read the design with `spektacular design read --data '{"source":"design","path":"plan-task-graph.md"}'`.
- Change only the table row `| Acceptance criteria | ... ticked by implement only when the criterion passed verification. |` (section "Tasks in plan.md") to: "`- [ ]` / `- [x]` checkboxes, ticked by implement only when the goal check judged the criterion met; with `implement.goal_check.enabled: false`, only when it passed verification."
- Stage the full document under `.spektacular/tmp/000068_task-goal-validation/plan-task-graph.md` and write it with `spektacular design author --data '{"source":"design","path":"plan-task-graph.md"}' --from .spektacular/tmp/000068_task-goal-validation/plan-task-graph.md`, which preserves `created_date` and the `specs` back-links (knowledge: store files are written through the CLI; `design write` over an authored document is refused). Remove the scratch file afterwards.
- **Orchestrated run**: the child never writes a design document (`spek-implement-epic` child prompt). It hands the staged revision back in a `QUESTION:` naming the document, the row and the new text, and the orchestrator applies it with `design author` from the project root and answers.
- Check: `design read` shows the new row, `created_date: "2026-09-25"`, and `specs` listing `000058_plan-task-graph` and `000068_task-goal-validation`. `design ref list --data '{"spec":"000068_task-goal-validation"}'` still resolves.

**Complexity**: Low
**Token estimate**: ~5k
**Agent strategy**: Single agent, sequential.

### Task: Document the goal check on the docs site

Repo: `docs` (`/home/nicj/code/github.com/hivecommons/spektacular-website`; in an epic run, the spec's worktree for this repo).

- `docs:src/pages/how-it-works.mdx:141`: the Core concepts tree becomes `└── Steps: read_plan → analyze → implement → test → verify → check_goal → update_plan → ...`.
- `docs:src/pages/how-it-works.mdx:444`: the Implement `PipelineNode` `detail` becomes `"independent reviewer<br>checks each criterion"`. Leave `docs:src/pages/index.mdx:41` unchanged unless it repeats the same wording.
- `docs:src/pages/how-it-works.mdx:448-485`: add a paragraph after the first one in the Implement stage body. **Content example**:

  ```mdx
  After each task's tests and checks pass, a separate reviewer checks the
  task's goal: its acceptance criteria and the files it may change. The
  reviewer starts fresh, sees only the goal, the check results and the work
  itself, and changes nothing. It judges each criterion **met**, **not yet
  met** or **impossible**, with the evidence it found. Only met criteria are
  ticked. Not yet met work goes back for up to two more attempts with the
  reviewer's reasons; after that, Spek asks you to keep fixing, accept the
  shortfall or skip the task. An impossible criterion stops the run at once
  so you can fix the plan. Every verdict is recorded; read it back with
  `spektacular implement goal show <spek>`. Switch the check off with
  `implement.goal_check.enabled: false` in [Configuration](/configuration/).
  ```

- `docs:src/pages/plan-tasks.mdx:88-93`: the `Acceptance criteria` `ConfigKey` body becomes "`- [ ]` outcome statements, ticked by the implement workflow only when its goal check judged the criterion met (or, with the check switched off, when it passed verification)."
- `docs:src/pages/configuration.mdx:72-73`: the example adds:

  ```yaml
  implement:
      worktrees: true
      goal_check:
          enabled: true
          models:
              claude: haiku
  ```

- `docs:src/pages/configuration.mdx:301-313`: the `implement` `ConfigKey` `defaultValue` becomes `<code>worktrees: true</code>, <code>goal_check.enabled: true</code>`. Add bullets. **Content example**:

  ```mdx
  - `implement.goal_check.enabled`: whether a separate reviewer checks each
    task's acceptance criteria before they are ticked. On by default,
    including in projects whose configuration predates the setting. Set it
    to `false` to tick criteria from the verification results as before.
  - `implement.goal_check.models.<agent>`: the model the reviewer uses for
    one coding agent (`claude`, `codex`, `copilot` or `omp`). Other agents
    keep their defaults. Re-run `spektacular init <agent>` after changing it.

  Reviewer defaults and limits by agent:

  - `claude`: `haiku`, read and search tools only.
  - `codex`: <the confirmed mini model>. Codex has no read-only mode for
    helpers, so the reviewer is told not to change files.
  - `copilot`: <the confirmed cheap model>, read and search tools only.
    When the session model is `Auto`, Copilot ignores the helper's model.
  - `omp`: `@smol`, which maps to your own cheap model.
  - `bob`: Bob has no per-helper model setting; the reviewer is Bob's
    built-in read-only `explore` helper on Bob's lighter model.

  Your own agent settings that pin a helper's model take precedence.
  ```

  Use the model names the installer task confirmed (`spektacular:internal/agent/subagents.go`).
- `docs:src/pages/configuration.mdx:4`: frontmatter description; add `goal_check` if it lists `implement` sub-keys.
- `docs:CHANGELOG.md`: a new entry, "Implement runs check each task's goal with an independent reviewer", with no em dashes.
- Conventions: no layout HTML in page bodies (`grep -nE "<div|<section|class=" src/pages/*.mdx` returns 0), fenced code, blank lines around slot content, no em dashes, no new sections, so shading is unchanged.
- Verify: `make build` (`npm run build`) and `make check` (`npx astro check`) are clean.

**Complexity**: Low
**Token estimate**: ~12k
**Agent strategy**: Single agent, sequential.

## Testing Strategy

Per task:

- **Teach the plan reader criterion text and legacy phases.** Table tests in `internal/plantask/plantask_test.go` and `files_test.go` cover criterion text, ticked state, multi-line criteria, legacy phases and their files. A golden check covers `status`/`export` output on an existing fixture.
- **Add the goal-check configuration.** `internal/config/config_test.go`:
  - default on;
  - an absent key loads as on;
  - `enabled: false` round-trips, checked against the raw file text (gotcha: YAML booleans);
  - a models map round-trips;
  - an unknown agent is refused.

  `cmd` tests confirm `workflow.Config.GoalCheck` on new and goto.
- **Add the goal package.** Pure-function tests for `Build`, `Current`, `ValidateReply`, `Next`, `ApplyDecision`, `State`/`Settled`, plus a store round trip on `t.TempDir()`.
- **Add the implement goal commands.** `cmd/implement_goal_test.go`, through `runRootCmd`, checks record/decide/show, every refusal's code and `next_action`, the switched-off refusal, a legacy phase, and `tree_unchanged` true and false against a temp git repo.
- **Write the reviewer brief and skill.** `cmd/skill_test.go` checks the skill output, and a template phrase test checks the brief.
- **Add the check_goal step to the implement workflow.** `internal/steps/implement/steps_test.go` covers order, wiring, walks with the check on and off, the retry block, the update_plan refusal and met-only list, check_goal phrases, the STOP map, sub-agent steps and spec-conflict steps. `cmd/instruction_contract_test.go` gets the table rows, and the hard-coded `cmd` walks are updated. `go test ./...` must be green.
- **Install a reviewer helper for every agent.** Per-agent install tests, a cross-agent default/override test, the updated tree counts, the init second-run identity check, and migrate restoring the helper.
- **Keep epic merges off an open goal-check question.** `cmd/epic_worktree_test.go` checks the merge guard, `templates/implement_epic_skill_test.go` the phrases, and `internal/steps/implement/orchestrated_test.go` the orchestrated `check_goal` render.
- **Update the implement and implement-epic harbor suites.** Oracle and state edits, a verdict assertion for every task, and the session-log ordering check. Run `make harbor-test-implement` and `make harbor-test-implement-epic-{claude,codex,bob}` by hand (manual, test plan).
- **Add the goal-check harbor suite.** The verifier self-check runs locally with pytest. The scenario runs under each agent are manual and repeated for the success metrics (test plan).
- **Update the tick rule in the plan-task-graph design.** `design read` shows the new row with metadata preserved; `design ref list` still resolves.
- **Document the goal check on the docs site.** `make build` and `make check` in `docs`, plus the MDX Rule 1 grep.

## Project References

- **Spec:** `000068_task-goal-validation` (`spektacular spec file read 000068_task-goal-validation`), in epic `000001_agent-workflow-improvements`; sibling `000069_compact-agents-md`.
- **Design:** `plan-task-graph.md` from the `design` source (`spektacular design read --data '{"source":"design","path":"plan-task-graph.md"}'`).
- **Knowledge (repo `spektacular`):**
  - `architecture/testing-architecture.md`
  - `architecture/workflow-steps.md`
  - `architecture/working-with-files-from-steps.md`
  - `gotchas/harbor-agent-logs-and-artifacts.md`
  - `gotchas/yaml-boolean-tokens-as-config-string-values.md`
  - conventions: error messages suggest remediation; plans never change the active install; store files through the CLI; tests order-independent; tests pass before done.
- **Knowledge (repo `docs`):** conventions MDX authoring, no em dashes, plan content pages, site layout, alternate section shading, file-scoped headings.
- **Repo roots:**
  - `spektacular`: `/home/nicj/code/github.com/hivecommons/spektacular`
  - `docs`: `/home/nicj/code/github.com/hivecommons/spektacular-website`

  In an epic implement run, use the spec's worktree roots named by the implement steps instead.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential: config, reviewer brief, epic merge guard, design rule, docs |
| Medium | ~25k | 2-3 parallel agents: plan reader, goal package, goal commands, agent installers, existing harbor suites |
| High | ~50k+ | Parallel analysis, sequential integration: check_goal step, goal-check harbor suite |

## Migration Notes

- **No settings-format bump.** `implement.goal_check` is parsed over the defaults, so older `config.yaml` files load with the check on and need no migrate step. `init` writes the section for new projects because `ToYAMLFile` serialises the struct.
- **Agent installs.** Existing projects get the reviewer helper the next time `spektacular init <agent>` runs, or when `migrate` reinstalls skills on a version change. Until then, the `check_goal` step falls back to a generic fresh-context sub-agent with the same brief.
- **In-flight runs.** `update_plan` keeps `verify` as a source, so the check can be switched off. With the check on, its guard refuses any task that has no settled outcome, which is what stops a run from skipping `check_goal`. A run caught mid-upgrade at `update_plan` for a task with no outcome gets the refusal. Its `next_action` says to finish that one task with `implement.goal_check.enabled: false` and switch the check back on afterwards. Record this upgrade edge in the feature changelog.
- **Never** run `init`/`migrate` on this repository itself during implementation (convention).

## Performance Considerations

- One extra sub-agent per task check on a cheap model. The spec's budget is under 2 minutes per task and under 10% of the implementer's spend (manual metrics in the test plan).
- `implement goal record` runs two git commands per code root to fingerprint the tree, which is negligible.
- `goals.json` grows by one check per attempt; it is read in full per call, which is fine at plan scale (tens of tasks).
