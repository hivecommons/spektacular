---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Context: 000062_epic-plan-and-implement

## Current State Analysis

- **One workflow per working copy.** Spec, plan and implement share one `.spektacular/state.json` (`cmd/spec.go:94-96`; project root = cwd, `cmd/root.go:266-340`). `plan goto` and `implement goto` take no name and act on whatever that file holds (`cmd/plan.go:138-194`, `cmd/implement.go:176-239`). A second `new` returns a resume report (`cmd/resume.go:68-97,164-181`). The guided repo add already keeps its own `repo-state.json` (`cmd/repo.go:184-189`).
- **Shared scratch.** `.spektacular/working-context.md` is one file (`internal/workingcontext/workingcontext.go:27-32`), and every step's footer names it (`templates/partials/working-context-footer.md`). Plan steps stage assembled documents under fixed names in `.spektacular/tmp/` (`templates/steps/plan/13-assemble.md:63-67`), and the commit message path is fixed (`cmd/autocommit.go:29`). `.spektacular/tmp/` is not git-ignored, and `state.json`, `working-context.md` and `repo-state.json` are tracked.
- **Auto-commit** runs `git add -A` plus a commit on every dirty registered work tree at completion points (`internal/autocommit/git.go:132-142`, `points.go:29-36`). This repo uses `auto_commit: full`.
- **Epics and status.** The epic graph `specs[{name, depends_on}]` is validated on write (`internal/epic/validate.go:23-54`) but not on read (`internal/epic/epic.go:82-122`). `status <name>` classifies each spec (`internal/status/classify.go:57-98`) and reports `ready`/`blocked_by` for implementation only (`internal/status/report.go:282-331`). It counts a draft plan as `planned`, and `implemented` flips at the last ticked task, before the implement wrap-up. `depgraph` has only `FindCycle` (`internal/depgraph/depgraph.go:11-58`).
- **Per-spec stop points.** The plan walkthrough is mandatory and needs explicit sign-off (`templates/steps/plan/18-walkthrough.md:3,24`; pinned by `internal/steps/plan/steps_test.go:426-450`). Implement asks between tasks unless told "run without asking" (`templates/steps/implement/07-update_changelog.md:93-94`). The implement dependency check refuses unless dependencies are implemented or overridden (`cmd/implement.go:262-317`).
- **Skills** are registered in `internal/agent/skills.go:26-33` and `commands.go:142-149`. Six skills are pinned by count in the agent tests.
- **No git worktree support** exists anywhere.
- **The build is broken at HEAD**: `cmd/migrate.go` declares `installerFor` twice (`:66` and `:80`).

## Per-Task Technical Notes

### Task: Restore a compiling build

- **File changes**:
  - `cmd/migrate.go:80-92`: delete the second, unindented `installerFor` declaration, which duplicates `:66-78`. It came in with commit 1188110.
  - Check with `go build ./cmd/...`, then run the full suite with `go test -shuffle=on ./...`. The plain `go build ./...` trips over a permission-denied `tests/harbor/jobs/...` directory, which is unrelated; build named packages instead.
- **Complexity**: Low
- **Token estimate**: ~3k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Run orchestrated plan and implement workflows in their own lanes

- **File changes**:
  - New `cmd/workflow_slot.go` (alongside `stateFilePath`, `cmd/spec.go:94-96`):
    - `Slot` struct, `laneStatePath(dataDir, kind, name)` → `.spektacular/workflows/<kind>-<name>.json`, `laneNotesPath` → `.spektacular/workflows/<kind>-<name>.md`;
    - `ResolveSlot(dataDir, kind, name)`:
      - lane file exists → lane;
      - else the shared `state.json`, refusing `workflow_not_found` when that holds a different kind or name. The refusal message lists the in-progress lanes found by globbing `workflows/<kind>-*.json`, and its next_action gives the exact `goto` with the right name.
    - Errors go through `output.NewError(...).WithNextAction(...)` (convention).
  - `cmd/plan.go:49-136` (`plan new`):
    - read `orchestrated` from `--data`;
    - if set:
      - `statePath = laneStatePath(...)`;
      - `probeResume` against the lane only (`cmd/resume.go:164-181`);
      - skip `startGate` (`plan.go:115`);
      - `clearState` (`:119`) removes only the lane file;
      - `wf.SetData("orchestrated", true)`.
    - If not set: before the existing probe, refuse `workflow_in_progress` when `laneStatePath("plan", name)` exists and is in progress (needs the name, so check after name validation).
  - `cmd/implement.go:92-157` (`implement new`): same changes, keeping the existing check order (plan exists → stale → task → dependencies → gate → clear). The gate is skipped for lanes.
  - `cmd/plan.go:138-194` and `cmd/implement.go:176-239` (`goto`):
    - read `name` from the input and `delete(input, "name")` before calling `gotoWithAutoCommit`, mirroring `commit_message_from` (`cmd/autocommit.go:68-70`);
    - resolve the slot;
    - pass `slot.StatePath` to `guardKind` and `gotoWithAutoCommit`;
    - `implement goto`'s stale-plan read (`:228-234`) uses the slot's state.
  - `cmd/resume.go:68-97` `emitResumeReport`: include `name` in the template vars, so the resume `goto` can carry it (see the next task).
  - Dry run: append `.dryrun-tmp` to the lane path exactly as today (`plan.go:86`).
  - Tests, new `cmd/workflow_lane_test.go`, using `resetRootCmd` and helpers:
    - two lanes plus a standalone run, advanced interleaved;
    - repeated orchestrated `new` gives a resume report;
    - standalone `new` is refused while a lane is in progress;
    - `goto` routing, name not persisted, mismatch and unknown refusals;
    - a `goto` with no name is unchanged.
  - Existing tests that must stay green: `cmd/resume_test.go`, `cmd/cross_kind_test.go`, `cmd/startgate_test.go`.
- **Complexity**: High
- **Token estimate**: ~45k tokens
- **Agent strategy**: Parallel analysis of the plan and implement handlers, then a single agent integrates the slot resolver and both handlers sequentially, so the refusal wording stays consistent.

### Task: Name the spec in every plan and implement instruction and keep scratch files per spec

- **File changes**:
  - Every `goto --data` line in `templates/steps/plan/01-19*.md` and `templates/steps/implement/01-12*.md` (32 files across steps and partials contain `"step":"`):
    - add `,"name":"{{plan_name}}"` (plan) or the implement equivalent (`plan_name` is exposed through `PathVars`, `internal/steps/plan/strategy.go:19-21` and `internal/steps/implement/strategy.go:58`).
  - `templates/steps/resume.md:17`, `resume_implement.md:22`, `resume_mismatch.md:12`: add `"name":"{{name}}"` for the plan and implement kinds. Spec stays name-free; use a `{{#name}}` section or a per-kind var supplied by `emitResumeReport`.
  - `templates/partials/git-commit-message.md:24-28`: carry the name, and use the staged path `.spektacular/tmp/{{commit.name}}/git-commit-message.md`.
  - `internal/stepkit/stepkit.go:67` builds `commit.tmp_path` per name.
  - `cmd/autocommit.go:29` `commitMessageTmpPath` becomes a function of the name, used in refusal messages.
  - `internal/workflow/workflow.go:221-239`: `nextActionForSteps` and `walkthroughRevisionHint` render the name into suggested `goto` payloads when the workflow data has `name` and the kind is plan or implement.
  - Plan scratch documents move to `.spektacular/tmp/{{plan_name}}/plan_template.md`, `context_template.md` and `research_template.md`:
    - `templates/steps/plan/13-assemble.md:63-67`;
    - `14-verification.md:5-7`;
    - `15-write_plan.md:9-10`;
    - `16-write_context.md:9-10`;
    - `17-write_research.md:9-10`;
    - `19-finished.md:7-10`.
  - Implement tmp names (`06-update_plan.md:21-25`, `10-update_feature_changelog.md:56,74`) also move under `.spektacular/tmp/{{plan_name}}/`, for one consistent shape.
  - Tests:
    - extend `cmd/instruction_contract_test.go:313-333` so every rendered plan and implement `goto` contains `"name":"`;
    - update `internal/steps/plan/steps_test.go` and `internal/steps/implement/steps_test.go` expectations that quote literal `goto` lines or tmp paths;
    - `templates/data_payload_wellformed_test.go:40` must still pass (balanced braces);
    - `templates/skill_resume_test.go` stays green.
- **Complexity**: Medium
- **Token estimate**: ~35k tokens
- **Agent strategy**: Two or three parallel agents: plan templates, implement templates plus resume, and the Go hint rendering plus tests. Then one sequential pass of the contract tests.

### Task: Let an orchestrated run hand back instead of asking

- **File changes**:
  - `templates/steps/plan/18-walkthrough.md`: wrap the existing body in `{{^orchestrated}}…{{/orchestrated}}` so standalone text is byte-identical. Add a `{{#orchestrated}}` branch:
    - read the three documents back;
    - hand back to the orchestrator a summary (approach, milestones and tasks with any `human` tasks, out of scope, drafting assumptions);
    - advance to `finished` without a sign-off question;
    - the finished hand-back message starts `DONE: {{plan_name}}`.
  - Check that the plan FSM allows `walkthrough → finished` with no extra data. It does today (`internal/steps/plan/steps.go`).
  - `templates/steps/implement/07-update_changelog.md:93-94`: under `{{#orchestrated}}`, always loop to `analyze` without asking; the standalone bullet is unchanged.
  - Every STOP sentence in the plan drafting steps (02-12 "Proceed unless genuinely blocked" paragraph) and in implement 01-11:
    - add one orchestrated sentence: "If this run is orchestrated, hand the question back to your orchestrator as `QUESTION: <spec>` with the options and your recommended default, then wait for its answer";
    - render it once from a new partial `templates/partials/orchestrated-stop.md`, included where STOP appears.
  - `templates/partials/working-context-footer.md` and `internal/stepkit/stepkit.go:141-147`: render `{{working_context_path}}`, which is the lane notes path when `orchestrated` and `.spektacular/working-context.md` otherwise. `internal/workingcontext/workingcontext.go:27-32` gains `LanePath(kind, name)`.
  - The workflow data must expose `orchestrated` to the template vars. Check how `stepkit` builds vars from `Data` (`internal/stepkit/stepkit.go:97`).
  - Tests:
    - new orchestrated-render tests in `internal/steps/plan/steps_test.go` (walkthrough has no "explicit affirmative" and has `DONE:`) and `internal/steps/implement/steps_test.go` (no "ask the user" in the orchestrated update_changelog);
    - the existing pins at `plan/steps_test.go:426-450` and `implement/steps_test.go:384-391,430-449` keep passing for the standalone render;
    - `cmd/instruction_contract_test.go:212-239` footer checks for both renders.
- **Complexity**: Medium
- **Token estimate**: ~30k tokens
- **Agent strategy**: A single agent, sequential. The edits are template-wide and must stay consistent.

### Task: Scope orchestrated plan commits to their own files

- **File changes**:
  - `internal/autocommit/git.go:132-142`: add `CommitPaths(dir, paths, msg)`, which runs `git add -A -- <paths>` and then `git commit -F - --only -- <paths>`. Extend the `Git` interface and its fake.
  - New `internal/autocommit/lock.go`: a project-level lock at `.spektacular/workflows/.commit.lock`. It uses O_CREATE|O_EXCL with a retry and back-off for up to ~30s. A stale lock older than 10 minutes is broken and its holder is reported.
  - `cmd/autocommit.go:57-184` `gotoWithAutoCommit`:
    - when the state's data has `orchestrated` and the kind is `plan`, the completion point (`internal/autocommit/points.go:29-36`, `walkthrough→finished`) uses `CommitPaths`. The paths are:
      - the plan's store directory (from the plan store's resolved dir for the name);
      - `.spektacular/work/<name>/`;
      - `.spektacular/tmp/<name>/`;
      - the lane `.json` and `.md`;
    - the commit is taken under the lock;
    - the snapshot and restore (`:152-163`) already act on the lane path once the previous task passes `statePath`. Confirm this.
  - Lane cleanup:
    - on reaching `finished`, remove the lane `.json` and `.md`, before the commit so the deletion is part of it;
    - put this in `cmd/autocommit.go` after a successful `wf.Goto` to `finished` for an orchestrated workflow;
    - without auto-commit, remove them as well.
  - `startGate` is untouched (lanes skip it).
  - Tests:
    - `internal/autocommit/git_test.go` (real temporary repo): `CommitPaths` excludes other dirty files and includes deletions;
    - lock test: concurrent goroutines serialise;
    - `cmd/autocommit_test.go`: two lanes in one temporary project; finishing one commits only its own paths while the other's work files stay dirty;
    - a failed commit restores only the lane.
- **Complexity**: Medium
- **Token estimate**: ~25k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Show orchestrated runs in status and the session log

- **File changes**:
  - `internal/status/report.go:413-424` `currentStep(s, kind, name, …)`: before falling back to the shared state, load `laneStatePath(kind, name)` when it exists and is in progress, and return its `CurrentStep`.
  - `status.Options` gains `DataDir` (or a `LaneReader func(kind, name) *workflow.State`) so the lookup stays testable. Wire it in at `cmd/status.go:135`, `cmd/implement.go:270` and `cmd/epic_link.go:319`.
  - Move the lane-path helper to a small package, `internal/workflow/lane.go`, so that `cmd` and `status` share it. The previous task's `cmd/workflow_slot.go` then calls it.
  - `cmd/root.go:196-237` `readStateSnapshot`: when argv is a `plan` or `implement` `goto`/`new` whose `--data` carries `name` (and, for `new`, `orchestrated`), snapshot the lane state instead of `state.json`. The session ID (`internal/sessionlog/record.go:40-44`) then becomes `kind:name` for the lane.
  - Tests:
    - `internal/status/status_test.go`: a lane in progress shows its step for that spec, while the shared state is unchanged;
    - `cmd/status_test.go`: the JSON for an epic with one orchestrated plan in progress;
    - a session-log snapshot test in `cmd/root_test.go`.
- **Complexity**: Low
- **Token estimate**: ~15k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Order epic specs and find cycles

- **File changes**: `internal/depgraph/depgraph.go` (beside `FindCycle`, `:11-58`):
  - `CycleMembers(order, deps)`: Tarjan SCC; members of any SCC larger than one node, plus self-loops, returned in `order` order.
  - `TopoOrder(order, deps)`: Kahn's algorithm that picks the earliest-in-`order` node among those ready; dependencies outside `order` are ignored; cycle members are appended in `order`.
  - Tests in `internal/depgraph/depgraph_test.go`: table cases (chain, diamond, disjoint, self-loop, two-cycle, outside names, ties).
- **Complexity**: Low
- **Token estimate**: ~8k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Give each spec its own worktree and merge it back

- **File changes**:
  - New `internal/worktree/worktree.go` over `internal/gitexec` (`gitexec.go:31-67`):
    - `Manager{ProjectRoot string, Repos *repo.Set, Git Runner}`.
    - **Touched repos.** The project repo, plus every `**Repo:**` named by the spec's plan tasks (`plantask.Parse`, `internal/plantask/plantask.go:124-221`). Each one is resolved to its git top level with `git rev-parse --show-toplevel`, from `repo.Set.LocalSource` (`internal/repo/set.go:130-160`). Repos with the same top level are de-duplicated, as `autocommit.Targets` does (`internal/autocommit/targets.go:169-203`). A touched repo whose source is not on disk is refused with `worktree_failed` and a next step of `repo add`.
    - **`Ensure(spec, repos)`.** For each top level:
      - if `git worktree list --porcelain` already shows `<project>/.spektacular/worktrees/<spec>/<repo>`, reuse it;
      - otherwise run `git -C <toplevel> worktree add -b spek/<spec> <path> HEAD`, or attach an existing `spek/<spec>` branch without `-b`.
      - The project repo's worktree is `.../<spec>/<project repo name>`.
      - Before adding, add `.spektacular/worktrees/` to the **project** repo's `$(git rev-parse --git-common-dir)/info/exclude` (idempotent). This keeps the nested worktrees out of `git add -A` in the main copy, where they would otherwise become embedded-repo gitlinks, and it works for existing projects.
      - Then write the **repo overlay** `<project worktree>/.spektacular/worktree-repos.json` (`RepoOverlay`). It maps every registered repo to its location inside its worktree: the repo's location path relative to its own top level, re-rooted under that repo's worktree. For example, `docs`, located at `<docs toplevel>/.spektacular`, maps to `<docs worktree>/.spektacular`.
      - Repos the plan does not touch are left unmapped and resolve as usual.
      - Add `.spektacular/worktree-repos.json` to the project's `info/exclude` so it is never committed or merged.
    - **`List()`.** Group the porcelain output of every registered top level by `spek/<spec>`.
    - **`Merge(spec)`.** All or nothing:
      1. Refuse if any touched repo has a merge in progress, or a dirty main working copy that overlaps the incoming changes.
      2. For every repo, run `git merge-tree --write-tree --name-only --no-messages HEAD spek/<spec>` (git 2.38 or later; 2.55 is installed). A non-zero exit collects that repo's conflicted paths.
      3. If any repo conflicts, return `Conflicts` for every repo and touch nothing.
      4. Otherwise, in each repo, run `git merge --no-ff --no-edit spek/<spec>`. Then run `git worktree remove` for each path and `git branch -d spek/<spec>`.
      5. If a real merge still fails after a clean dry run (a race), run `git merge --abort` in that repo, report `epic_merge_conflict` naming the repos already merged, and stop.
  - **Repo overlay applied in `internal/repo/set.go:66` `repo.New`.** When `<projectRoot>/.spektacular/worktree-repos.json` exists, override `Location` for each mapped entry with the absolute path. `LocalRoot`, `LocalSource`, the per-repo knowledge and changelog stores, and `autocommit.Targets` all follow, because they build on the `Set`. Unmapped entries are unchanged, and a project without the file is unaffected. Add a test in `internal/repo/set_test.go`.
  - `templates/.spektacular/.gitignore`: add `worktrees/` and `worktree-repos.json` for new projects. This repo's own `.spektacular/.gitignore` is not touched (convention: plans never change the active install); the info/exclude step covers it.
  - New `cmd/epic_worktree.go`:
    - `epic worktree` and `epic merge` with `--data` `{"spec": "<name>"}` and `--schema`, registered at `cmd/epic.go:488`;
    - `epic worktree` needs the spec's plan, to read the touched repos. It refuses with `plan_not_found` when the plan is missing.
    - Refusals:
      - `spec_not_found`;
      - `epic_merge_conflict`, whose message lists the paths per repo and whose next_action says to report them to the user and stop, with no automatic resolution;
      - `worktree_failed`, carrying git's message plus remediation.
  - Tests:
    - `internal/worktree/worktree_test.go`, using two real temporary repos (`git init`, `t.TempDir()`, isolated `GIT_CONFIG_GLOBAL`) registered as project and sibling repo with a relative location:
      - `Ensure` is idempotent and creates one worktree each, with the exclude written once;
      - the overlay maps both;
      - a clean merge lands in both and removes everything;
      - a conflict in the sibling repo leaves both HEADs and indexes unchanged.
    - `cmd/epic_worktree_test.go`: the JSON shapes. From inside the project worktree, `repo list` reports both roots inside the worktrees.
- **Complexity**: High
- **Token estimate**: ~40k tokens
- **Agent strategy**: Parallel analysis (repo resolution and stores, git merge-tree behaviour), then a single agent integrates sequentially. Git behaviour is best verified incrementally against real repos.

### Task: Report what an epic still needs in status

- **File changes**:
  - New `internal/status/run.go`, beside `classify.go` and `report.go`:
    - `buildRun(opts, target) (EpicRun, map[string]SpecRun)`, called from `buildTarget` (`internal/status/report.go:282-331`) after the existing per-spec fields are built. The existing fields, `ready`/`blocked_by` (`:305-316`), `Classify`/`Describe` (`classify.go:57-98`) and `DependenciesOf` (`report.go:212-243`, used by the implement check) are not changed.
    - New types `EpicRun`, `SpecRun`, `RunPart` and `Problem`, added to `Report`/`EpicStatus`/`SpecStatus` (`report.go:16-99`) as `run` with `json:"run,omitempty"`. Update the `--schema` in `cmd/status.go:57-96`.
    - `status.Options` gains a lane reader (shared with the previous task) and a worktree lister (the worktree manager's `List`), plus the repo `Set` for touched repos and `dirty`. Wire them in at `cmd/status.go:135`. The implement check's call sites (`cmd/implement.go:270`, `cmd/epic_link.go:319`) leave them nil, so the run view is skipped there and stays cheap.
  - Predicates:
    - **planDone**: the plan exists, its `document_status` is final, and no plan lane is in progress.
    - **implDone**: `StateImplemented`, a final project changelog record for the spec (via the changelog store), no implement lane in the project or in its worktree, and no `spek/<spec>` worktree.
    - **awaitingMerge**: a worktree exists, its lane has no in-progress state, and the spec classifies as implemented with a final changelog when read from inside its project worktree. The overlay makes the per-repo changelog records there resolve inside the worktrees.
    - **inProgress**: a plan lane in progress in the project, or an implement lane in progress in the spec's project worktree (`.spektacular/worktrees/<spec>/<project repo>/.spektacular/workflows/implement-<spec>.json`).
    - **repos**: the touched repos, computed by the same function the worktree manager uses (`internal/worktree`).
  - Problems, in this order. Each is reported with `blocks: ["implement"]` and a message naming the specs:
    1. `epic_dependency_cycle` (`depgraph.CycleMembers`);
    2. `epic_dependency_outside`: a `depends_on` name that is not a member, reported only when that spec is not implemented or does not exist;
    3. `epic_unplanned`: every member without a final plan.
  - Planning is never blocked. Cycle members simply lose their ordering among themselves.
  - Order the specs with `depgraph.TopoOrder`, using list order for ties. `ready` and `blocked` per part follow from the predicates. `dirty` comes from `git status --porcelain` on every registered repo's top level, because the worktrees branch from each repo's last commit.
  - The pretty format (`internal/status/pretty.go`) gains one line under the epic header: "planning: N done, M in progress, K ready" and the same for implementing, plus any problems.
  - Tests:
    - `internal/status/run_test.go`: fixture projects built through the store APIs in `t.TempDir()`, one case per acceptance criterion;
    - `cmd/status_test.go`: the JSON carries `run`, and the existing golden shapes are unchanged apart from the added `run`.
- **Complexity**: High
- **Token estimate**: ~40k tokens
- **Agent strategy**: Parallel analysis (status internals, the worktree lister, the changelog store), then a single agent integrates and tests sequentially.

### Task: Write the plan-this-epic skill

- **File changes**: New `templates/skills/workflows/spek-plan-epic/SKILL.md`.
  - Frontmatter `name: spek-plan-epic`. The description quotes "plan this epic", "plan the epic", "plan <epic name>" and "plan all the specs in this epic".
  - Body, in order:
    1. `{{> partials/version-check}}`.
    2. The "this is a loop, do not stop" banner, mirroring spek-plan.
    3. **Find the epic.** A name given in the request, or the epic of the spec under discussion via `{{command}} status <spec> --format json`, or `{{command}} epic list`. Ask only if it is still ambiguous.
    4. **Loop.**
       - `{{command}} status <epic> --format json`, reading `epic.run` and each spec's `run.plan`.
       - Mention any problems in `epic.run.problems` to the user. They never block planning.
       - For each `in_progress` spec without a live child, start a child to resume it: `plan goto` with `step` and `name`.
       - For each `ready` spec, start a child: `plan new` with `name` and `"orchestrated":true`.
       - Use the agent's orchestration capability. Each child runs in the project root reported by `{{command}} repo list` and follows the `spek-plan` skill.
    5. **Child prompt template.** It must state:
       - the spec name and the project root;
       - start or resume as above;
       - "follow the `spek-plan` skill; this run is orchestrated";
       - the store-access rule (only `plan file`, `spec file` and so on, never file tools on store directories);
       - the hand-back contract (`DONE:`, `QUESTION:`, `FAILED:`);
       - the definition of a genuine open question.
    6. **Relay.**
       - On `QUESTION:`, put the question to the user. Present it as plain text first, per the draft-presentation rule, and name the spec.
       - Send the answer back to that child; the others continue.
       - If the user declines to answer now, enter stopping mode.
    7. **Stopping mode** (on `FAILED:` or a declined question): start no new child, let running children finish, then go to the final report.
    8. **Progress.** After every start or finish, one line: the specs being worked on, with their steps from `status`, and how many remain.
    9. **End-of-planning review.** For every plan produced in this run, one summary entry built from the child's `DONE:` summary. Invite changes, apply them with `{{command}} plan file read/write` through `.spektacular/tmp/`, and close on an explicit confirmation question.
    10. **Final report**: completed this run, skipped as already planned, and still outstanding, with the reason.
  - It must name `.spektacular/working-context.md`, for the orchestrator's own notes, so it passes `TestEachWorkflowNamesWorkingContext` if mapped.
  - It must not use forbidden substrings (`internal/agent/instruction_surface_test.go:17-63`), and every `--data` example must be well-formed (`templates/data_payload_wellformed_test.go`).
- **Complexity**: Medium
- **Token estimate**: ~20k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Write the implement-this-epic skill

- **File changes**: New `templates/skills/workflows/spek-implement-epic/SKILL.md`.
  - Frontmatter `name: spek-implement-epic`. The description quotes "implement this epic", "build the epic" and "implement <epic name>".
  - Body:
    1. Version check, loop banner, find the epic (as in spek-plan-epic).
    2. **Up front.**
       - `{{command}} status <epic> --format json`, reading `epic.run` and each spec's `run.implement`.
       - If `epic.run.problems` holds any problem whose `blocks` includes `implement` (`epic_unplanned`, `epic_dependency_cycle`, `epic_dependency_outside`): relay each message, implement nothing, and suggest "plan this epic" or fixing the epic.
       - If `dirty`: ask once whether to commit first. Worktrees branch from each repo's last commit, so uncommitted work is not in them. The answer is the user's.
    3. **Loop.**
       - For `awaiting_merge`: `{{command}} epic merge --data '{"spec":"<spec>"}'`.
       - For `in_progress` without a live child: resume in its `root` (the spec's project worktree).
       - For `ready`: run `{{command}} epic worktree --data '{"spec":"<spec>"}'`, then start a child in the returned `project` path. Every repo resolves to the spec's worktrees there, so the child's `repo list` gives it the right roots.
    4. **Child prompt.** Spec, root, and `implement new` with `"orchestrated":true` (or the resume `goto` with `name`). It must state:
       - "follow the `spek-implement` skill; orchestrated, so tasks run without asking between them";
       - the store-access rule;
       - the hand-back contract;
       - the definition of a genuine question;
       - "never resolve a merge or git conflict yourself".
    5. **On `DONE:`.**
       - Merge the spec with `epic merge`. On `epic_merge_conflict`, show the conflicting paths for each repo to the user and enter stopping mode.
       - Re-read `status` and start the newly ready specs.
    6. Relay, stopping mode, progress and final report as in spek-plan-epic. The final report adds any unmerged worktrees left behind, with their paths.
  - It must name `.spektacular/working-context.md`.
- **Complexity**: Medium
- **Token estimate**: ~22k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Install the epic skills and teach the per-spec skills about orchestration

- **File changes**:
  - `internal/agent/skills.go:26-33`: add `spek-plan-epic` and `spek-implement-epic` to `workflowSkills`.
  - `internal/agent/commands.go:142-149`: add their `workflowDescriptions`.
  - Tests:
    - `internal/agent/agent_test.go:86-136` (fixtures and `Len 6` → 8);
    - `:181-227` (wrapper map and count);
    - `:240-253` (agreement);
    - `internal/agent/claude_test.go:25-55`, `bob_test.go:24-52`, `codex_test.go:24-31`;
    - `cmd/init_test.go:74-76`: optionally assert the new skills too;
    - `cmd/instruction_contract_test.go:488-530`: decide whether the epic skills map to a workflow kind. If not, they still pass the rendered-surface checks (`:313-333`, `:372-486`).
  - `templates/skills/workflows/spek-plan/SKILL.md` and `spek-implement/SKILL.md`: add a short "When an orchestrator starts this skill" paragraph:
    - start with `"orchestrated":true` (plan `new` and implement `new`);
    - every `goto` carries `name`;
    - questions are handed back as `QUESTION:`;
    - the walkthrough hands back a summary (plan), and tasks loop without asking (implement);
    - the run ends with `DONE:` or `FAILED:`.

    The existing pinned text stays as it is (`internal/agent/instruction_surface_test.go:180-250`, `templates/skill_list_command_test.go`, `templates/skill_resume_test.go`).
- **Complexity**: Low
- **Token estimate**: ~15k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Document planning and implementing an epic on the website

- **File changes**:
  - `docs:src/pages/epics.mdx`:
    - insert `<Section heading="Planning and implementing an epic" surface={false}>` after "Dependencies between specs" (`:249-308`, `surface`) and before "Working with epics from the command line" (`:310`), and flip that section to `surface`;
    - check that the CtaBanner at `:370-381` still alternates;
    - body as in the plan's content outline, with fenced code for the `status` example, prose and lists only (Rule 1: no `<div>`, `<section>` or `class=`);
    - add one sentence after `:29-31` linking to the new section ("To plan or implement every spec in an epic with one request, see Planning and implementing an epic").
  - Optionally cross-link from `docs:src/pages/how-it-works.mdx:432-458` (implement stage).
  - `docs:CHANGELOG.md`: add an entry for 000062.
  - Verify with `npm run build`, `npx astro check`, and `grep -nE "<div|<section|class=" src/pages/*.mdx` returning nothing. No em dashes in the new text.
- **Complexity**: Low
- **Token estimate**: ~12k tokens
- **Agent strategy**: Single agent, sequential, run from the docs repo root.

### Task: Point the README at epic planning and implementation

- **File changes**: `README.md`: add the content-example paragraph in How It Works, after the status block (`:29-41`).
- **Complexity**: Low
- **Token estimate**: ~3k tokens
- **Agent strategy**: Single agent.

## Testing Strategy

Per task:
- **Restore a compiling build**: the existing suite is the test.
- **Lanes**: `cmd` integration tests drive interleaved lanes and a standalone run, and cover the routing and refusal cases; `resume_test`, `cross_kind_test` and `startgate_test` stay green.
- **Name in instructions and per-spec scratch**: a contract test requires `"name":"` in every rendered plan and implement `goto`; the payload well-formedness test and the step tests are updated for new paths.
- **Orchestrated hand-back**: render tests for the orchestrated walkthrough (no sign-off, `DONE:`) and implement loop (no between-task question), and the footer naming the lane notes; the existing standalone pins are unchanged.
- **Scoped commits**: real-repo `CommitPaths` tests (excludes others, includes deletions), a concurrent lock test, and a two-lane `cmd` test proving commit isolation and lane-only restore on failure.
- **Status and session log**: a lane's step appears in `status`; the session-log snapshot uses the lane.
- **Order and cycles**: depgraph table tests.
- **Worktrees**: two real temporary repos (project and sibling): idempotent per-repo ensure, exclude, overlay resolution (repo list, stores, commit targets), an all-repo clean merge with removal, and a conflict in one repo leaving both unmerged; `cmd` JSON shape tests.
- **`status` run view**: table tests over fixture projects, one per acceptance criterion (plan and implement readiness, done, in progress in project and worktree, awaiting merge, touched repos, dirty, three problems, planning never blocked, existing fields unchanged, list-order ties); `cmd` JSON tests.
- **Skills**: install, frontmatter, version check, rendered-surface, forbidden-substring and payload checks, plus phrase checks for the trigger wording, the hand-back contract, the genuine-question definition, stop-on-failure, the review and the final report.
- **Docs**: site build, `astro check` and the MDX guard.
- **Manual (implementation test plan)**: end-to-end plan and implement of a throwaway three-spec epic with a live agent (overlap, question relay, stop on failure or declined question, interrupted resume, progress, review), plus both success metrics.

The whole suite runs with `go test -shuffle=on ./...` and must pass in full.

## Project References

- Spec: `000062_epic-plan-and-implement`.
- Design: `epics-and-seeded-specs.md` (source `design`), binding.
- Prior plans: `000060_epics-and-seeded-specs`, `000057_git-commit`, `000058_plan-task-graph`.
- Knowledge (conventions): error-messages-must-suggest-remediation, store-files-must-be-written-through-the-cli, plans-never-change-the-active-install, tests-must-not-depend-on-order, tests-must-pass-for-done (spektacular); mdx-authoring, site-layout, alternate-section-background, no-em-dashes, plan-content-pages (docs).
- Repo roots: `spektacular` → `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` → `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

The High tasks (lanes, worktrees and the `status` run view) analyse in parallel but integrate sequentially, because both touch shared refusal wording and the status classifier.

## Migration Notes

- No settings-format change and no state-format change: a lane file has the same format as `state.json`, so no `migrate` step is needed.
- Existing projects get the two new skills through the normal `migrate` skills reinstall.
- New projects get `worktrees/` in `.spektacular/.gitignore`; existing projects are covered by the `info/exclude` entry `epic worktree` writes.
- Instructions rendered before this change (no `name` in `goto`) keep working, because a `goto` without a name still drives the shared workflow.
- Per the "plans never change the active install" convention, this repo's own skills and config are updated only by `make install-local` followed by a normal version check, between workflows.

## Performance Considerations

- The `run` view adds, per `status` call: a read of each member's changelog, one `git worktree list` per registered repo, and one `git status --porcelain` per registered repo. That is linear in the epic's size. The implement dependency check and the completed-epic guard build status without the run view, so they stay as cheap as today.
- The commit lock serialises only the completion commits of parallel plan lanes. Each is a small path-scoped commit, so the wait is short; the lock times out after about 30 seconds with a remediable error.
- Worktree creation copies a checkout per parallel spec. This is disk-bound, and acceptable for the handful of specs an epic holds.
