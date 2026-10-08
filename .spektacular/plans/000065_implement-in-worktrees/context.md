---
created_date: "2026-10-07"
document_status: final
closed_date: "2026-10-07"
---

# Context: 000065_implement-in-worktrees

## Current State Analysis

- **Worktrees are epic-only.** `epic worktree` (`cmd/epic_worktree.go:88-136`) creates a spec's worktrees through `worktree.Manager.Ensure` (`internal/worktree/worktree.go:296-347`) on `spek/<spec>` under `.spektacular/worktrees/<spec>/`, and writes the record `record.json` (`:67-78`, `:368-377`). `epic merge` (`cmd/epic_worktree.go:138-187`) runs the all-or-nothing `Manager.Merge` (`worktree.go:573-664`): prechecks, the `.spektacular` guard (`:675-707`), a `merge-tree` dry run, merge, then cleanup of the worktrees, branches and record.
- **`implement new` never creates worktrees** (`cmd/implement.go:56-201`). It reads a record if one exists (`codeRootsFor`, `:286-298`, no git) and renders the roots in the read-plan and feature-changelog steps only (`templates/steps/implement/01-read_plan.md:5-14`, `10-update_feature_changelog.md:23-32`). Steps 02–05 carry no code-location or sub-agent guidance.
- **Standalone implement uses the shared workflow slot** (`state.json`). A second `implement new` gets a resume report for the first (`cmd/resume.go:188-205`). Only orchestrated runs get per-spec lanes (`internal/workflow/lane.go`, `cmd/workflow_slot.go:55-153`).
- **Auto-commit off commits nothing** (`internal/autocommit/points.go:80-92`), so an epic child with auto_commit off leaves a dirty worktree that `Merge` refuses (`worktree.go:593-598`).
- **The dependency check ignores merging** (`cmd/implement.go:328-376` → `status.DependenciesOf`, `internal/status/report.go:242-273`). A complete but unmerged dependency counts as met. Unmet dependencies always refuse with an override offer, or refuse outright under `epic.strict_dependencies`.
- **The epic dirty flag covers every registered repo** (`internal/status/run.go:42-43,183-185`; `cmd/status.go:225-232`).
- **There is no worktree setup mechanism**, and repo metadata lives in `repo.yaml` (`internal/config/repo.go:33`).
- **Config parses over defaults** (`internal/config/config.go:415-449`), so a new default-true key needs no schema bump. The current project schema is 4 and the repo schema 2.

## Per-Task Technical Notes

### Task: Let a repo declare a worktree setup command

**File changes**:
- `internal/config/repo.go:33-46` — add `WorktreeSetup string \`yaml:"worktree_setup,omitempty"\`` to `RepoConfig`. No `Validate` change. No `CurrentRepoSchema` bump (`internal/config/schema.go:21`): yaml.v3 is non-strict, so older readers ignore the key.
- `internal/repo/register.go:16` — add `WorktreeSetup string` to `Registration`. At `:124-133`, overwrite `repoCfg.WorktreeSetup` only when the input is non-empty, as for Description/Role/Tags. Include it in `repoConfigDescriptiveFieldsEqual` (`:180`) or it is never written, and leave `DescriptiveFieldsEmpty` (`:191`) alone.
- `cmd/repo.go:117-123` — add `WorktreeSetup string \`json:"worktree_setup,omitempty"\`` to `repoAddInput` and pass it to `repo.Register` in `runRepoAdd` (`:142`). Add `worktree_setup` (string, optional) to `repoAddInputSchema` (`:52`).
- `cmd/repo.go:128,330-385` — add `WorktreeSetup` to `repoInfo` and copy it from `set.Footprint(name)` beside Description/Role/Tags. Add it to `repoListOutputSchema` (`:88`).
- `templates/skills/workflows/spek-manage-repos/SKILL.md` — in "Concepts", add a short paragraph: a repo may declare `worktree_setup` in its `repo.yaml`, a shell command run in each new worktree of the repo before an implement run starts (e.g. `npm ci`). Mention `worktree_setup` in the `repo add` `--data` fields.
- Tests: `internal/config/repo_test.go` (round-trip: absent loads as `""`; set value written and read back); `internal/repo/register_test.go` (register with setup writes it; re-register without it keeps it; changing only setup rewrites the file); `cmd/repo_test.go` (`repo list` shows `worktree_setup`; schema includes it). Any skill phrase test touching spek-manage-repos (`internal/agent/instruction_surface_test.go`) gains a `worktree_setup` assertion.

**Complexity**: Low
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Run a repo's setup command in each new worktree

**File changes**:
- `internal/worktree/setup.go` (new) — `type SetupRunner interface { Run(dir, command string) (string, error) }` and `NewSetupRunner()`, using `exec.Command("sh", "-c", command)` with `cmd.Dir = dir` and combined output. Model the error on `internal/gitexec/gitexec.go:53-62`: include the exit status and trimmed output.
- `internal/worktree/worktree.go:107-112` — add `Setup SetupRunner` to `Manager`, defaulting to `NewSetupRunner()` when nil.
- `internal/worktree/worktree.go:296-347` (`Ensure`) — after `ensureOne` returns `made == true` for a checkout, for each **touched** repo whose code root lies in that checkout (use the same mapping `codeRootIn`, `:351-365`, computes), read `WorktreeSetup` from the main registration via `m.Repos.Footprint(name)` (`internal/repo/set.go:245`), never from the worktree. Run it with `m.Setup.Run(codeRootInWorktree, cmd)`. On error: run `git worktree remove --force <path>`; when `ensureOne` created the branch in this call, also `git branch -D spek/<spec>`. Then return `output.NewError("worktree_setup_failed", fmt.Sprintf("the worktree setup command for %s (%q) failed in %s: %v", repo, cmd, path, err)).WithResource(repo).WithNextAction("fix the command in that repo's repo.yaml (worktree_setup) or the repo itself, then run the same command again")`. Do not write the record when a setup fails.
- `internal/worktree/worktree.go:380-404` (`ensureOne`) — also return whether the branch was newly created, so the cleanup above deletes only a branch it created.
- `cmd/epic_worktree.go:71-86` — `worktreeManager()` leaves `Setup` nil (real runner). Add a package var `worktreeSetup worktree.SetupRunner` that tests can swap, mirroring `worktreeGit` (`:20`).
- Tests: `internal/worktree/worktree_test.go` (fixture `newFixture` `:117`, `requireRefusal` `:165`): a setup that writes a file appears in the new worktree; no setup when undeclared; a failing setup refuses with `worktree_setup_failed`, and the worktree path and a newly created branch are gone; a second `Ensure` on an existing worktree does not run setup (fake runner counts calls); the command is read from the main footprint even if the worktree's `repo.yaml` differs. `cmd/epic_worktree_test.go`: `epic worktree` with a setup-declaring repo shows the effect.

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Single agent for the manager change, then one sub-agent for tests in parallel with the cmd wiring.

### Task: Tell code-touching steps where the code lives and how to brief sub-agents

**File changes**:
- `templates/partials/implement-code-locations.md` (new) — two-form block:
  - `{{#has_worktree_roots}}`: a "Where the code lives" heading; a list `- **{{repo}}**: \`{{{root}}}\`` over `worktree_roots`; "Every code edit, build and check happens in these locations, never in a main checkout. Give every sub-agent you launch (task implementers, test authors, verifiers) these exact locations, and tell it to work only in them. Run `{{config.command}}` itself from the project root, not from a worktree."; "A new worktree holds only tracked files: ignored dependency folders such as `node_modules` are not there. Prepare dependencies inside the worktree (a repo's declared setup command has already run). Never install into, symlink from or share dependencies with a main checkout."
  - `{{^has_worktree_roots}}`: "Run `{{config.command}} repo list`. Each repo's code lives at its `root`. Give every sub-agent you launch those exact locations for the repos its work touches, and tell it to work only there."
- `templates/steps/implement/02-analyze.md:9`, `03-implement.md:9`, `04-test.md:9`, `05-verify.md:9` — add `{{> partials/implement-code-locations}}` directly after `{{> partials/implement-current-task}}`.
- `internal/steps/implement/steps.go:57-69,345-360` — no change needed; `withCodeRoots` already feeds every `writeStep`. Confirm that 02–05 render through `writeStep`.
- `internal/steps/implement/steps_test.go:845-901` — add a test with `worktreeCodeRoots` asserting that each of analyze/implement/test/verify contains every root, "Give every sub-agent you launch", "work only in them", "only tracked files" and "Never install into". Add a test without roots asserting the `repo list` form. Update `TestStepsWithoutWorktreeRootsRenderUnchanged` to cover only steps other than 02–05, or to expect the new registry form there. Add a root containing `&` that renders unescaped.
- `tests/harbor/implement-workflow/tests/*` — if an oracle pins the step text of 02–05, update it (see the testing-architecture knowledge entry).

**Complexity**: Low
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Limit the epic's uncommitted-work warning to repos its plans touch

**File changes**:
- `internal/status/run.go:35-44` — change `Dirty func() bool` to `Dirty func(repos []string) []string` (doc: "lists the given repos that have uncommitted changes").
- `internal/status/run.go:87` — add `DirtyRepos []string \`json:"dirty_repos"\`` beside `Dirty bool`.
- `internal/status/run.go:183-185` (`buildRun`) — set `er.DirtyRepos = []string{}`. Build the touched set as the de-duplicated, first-seen union of `s.Run.Implement.Repos` over `r.Specs`. Call `src.Dirty(touched)`; when it returns non-empty, set `DirtyRepos` and `Dirty = true`.
- `cmd/status.go:215-246` (`statusRunSource`) — the `Dirty` closure copies cfg with `Repos` filtered to the touched names, runs `autocommit.Targets(filtered, root, autoCommitGit)` and `autocommit.DirtyTargets`, and flattens each `target.Repos` into the result. It returns nil on error. This is ported from PR #79 by hand because the PR does not apply cleanly.
- `cmd/status.go:86` — reword `dirty` as "a repo touched by this epic's plans has uncommitted changes", and add `dirty_repos` (array of string) to the schema.
- `templates/skills/workflows/spek-implement-epic/SKILL.md:36` — keep the current bullet's post-000064 wording ("uncommitted code will not be in any spec's worktree. Specs, plans and progress are read from the project itself…"), and add that `epic.run.dirty_repos` names the repos with uncommitted changes, which the agent names to the user. Do **not** take PR #79's isolation-baseline or worktree-cwd wording.
- Tests: new `cmd/status_dirty_test.go` (`TestStatusRunSource_DirtyOnlyChecksTouchedRepos`, from PR #79: unrelated dirt ignored; tracked and untracked; nil touched gives empty; duplicates de-duplicated; an alias sharing a checkout; nothing committed). `internal/status/run_test.go:435` (`TestRun_DirtyAndReposPassThrough` updated as in the PR). `cmd/status_test.go:624-671` (`dirty_repos == []` on clean runs; schema key). `templates/implement_epic_skill_test.go` (a `dirty_repos` phrase).

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent; use `gh pr diff 79 --repo hivecommons/spektacular` as the reference.

### Task: Always commit a worktree run's code on the spec branch

**File changes**:
- `internal/autocommit/points.go:80-92` (`PointFor`) and `:102-120` (`LeadsToCommit`) — add a `worktrees bool` parameter, or sibling functions `PointForRun`/`LeadsToCommitForRun`. When the mode is off and `worktrees` is true and the transition is a completion point (`completionPoints`, `:29-35`, kind `implement`), return `PointCompletion`. Milestone points stay mode-gated. Keep the existing signatures' behaviour for every other caller.
- `internal/stepkit/stepkit.go:142` — pass `len(cfg.CodeRoots) > 0` (runtime `workflow.Config.CodeRoots`, `internal/workflow/workflow.go:36-47`) into the `LeadsToCommit` call, so the commit-message partial renders for an off-mode worktree run at its completion exit.
- `cmd/autocommit.go:77-116` (`gotoWithAutoCommit`) — compute the point with the worktree flag, where the flag is a record existing for the run's spec (`worktree.ReadRecord(root, name)`; `codeRootsFor` already has it). At `:207-228`, pass the mode into `commitImplementLane` (`:309-373`). When the mode is off, commit only the worktree code targets (`autocommit.TargetsWithCodeRoots` filtered to `.spektacular/worktrees/<spec>/`) and skip the main-checkout `CommitPaths` artifact commit and its lock.
- Tests: `internal/autocommit/points_test.go` (off and worktrees gives completion; off and no worktrees gives none; full and worktrees unchanged). `cmd/autocommit_test.go:599-673` (add `TestAutoCommit_OffModeWorktreeRunCommitsCodeOnSpecBranchOnly`: after walking to `finished` with auto_commit off, `git log spek/<spec>` has a new commit containing the code change, the worktree is clean, and the main checkout's HEAD is unchanged). `internal/steps/implement/steps_test.go` (off-mode render with code roots at `update_changelog`→finished includes the commit-message partial; without roots it does not).

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential: points, then stepkit, then gotoWithAutoCommit, then tests.

### Task: Add an implement merge command

**File changes**:
- `cmd/implement_merge.go` (new) — `implementMergeCmd` registered on `implementCmd`, `--data '{"name":"<spec>"}'` with a JSON schema like the others (`cmd/implement.go:57-72` pattern), and help text. Handler: validate the name (reuse `worktreeSpec`-style validation, `cmd/epic_worktree.go:53-68`); if `worktree.ReadRecord(root, name)` reports none, return `output.NewError("worktree_not_found", "<spec> has no worktrees to merge").WithNextAction("run implement new for it, or check the name with spec file list")`. Otherwise build the manager with `worktreeManager()` (`:71-86`), call `m.Merge(spec)`, and on conflicts return `mergeConflict(spec, conflicts)` (`:173-187`). On success, write the same output shape as `runEpicMerge` (`:138-169`).
- `cmd/epic_worktree.go:138-187` — extract the success-output builder from `runEpicMerge` so both commands share it. Make `mergeConflict`'s next_action generic ("resolve the conflicts on spek/<spec> in the named repos yourself, then run the merge again"), unchanged for epics in substance.
- Tests: `cmd/implement_merge_test.go` (new), reusing `worktreeProjectWith` (`cmd/epic_worktree_test.go:94`), `wtWriteFile`, `wtCommitAll` and `runEpicWorktreeCmd` (`:123`) to create worktrees. Cases: a clean two-repo merge (both main lines contain the change; `git worktree list` lacks the paths; `spek/<spec>` branches gone; record gone); a conflict in the sibling repo (refused with code `epic_merge_conflict`, message names the repo and the path, both HEADs unchanged, worktrees and branches present); a `.spektacular` change refused with `epic_merge_touches_spektacular`; no worktrees gives `worktree_not_found`; schema and help.

**Complexity**: Low
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Count an implemented but unmerged dependency as unmet

**File changes**:
- `internal/status/report.go:129-145` — add `Unmerged func(spec string) bool` to `Options` (nil means nothing is unmerged).
- `internal/status/report.go:208-217` — add `Unmerged bool` to `Dependency`.
- `internal/status/report.go:229-237` — `Unmet()` includes `dep.Unmerged`.
- `internal/status/report.go:242-273,316-382` (`DependenciesOf`, `buildTarget`, `buildSpec`) — when classifying a dependency whose state is `StateImplemented` and `opts.Unmerged != nil && opts.Unmerged(name)`, set `Unmerged = true`, set the description to "is implemented but not yet merged" (add a helper beside `Describe`, `internal/status/classify.go:82-99`), and count it in `BlockedBy`, so `Ready` is false for its dependents. Leave the spec's own `State` as implemented.
- `cmd/implement.go:328-376` (`refuseUnmetDependencies`) — pass `Unmerged: func(s string) bool { _, ok, _ := worktree.ReadRecord(root, s); return ok }` into `status.Options`. Add a small helper (e.g. `unmergedFn(root)` in `cmd/status.go` or `cmd/implement.go`) used by every caller.
- `cmd/status.go` — every place that builds `status.Options` for the spec graph sets `Unmerged` from the same helper.
- Tests: `internal/status/report_test.go` (a complete dependency with `Unmerged` true is unmet, not ready, and described as "not yet merged"; with false it is met). `cmd/implement_dependencies_test.go:41-255` (reuse `depProject`; write `.spektacular/worktrees/<dep>/record.json` by hand for the dependency, as in `cmd/implement_test.go:362`, with all its tasks ticked: `implement new` gives `dependencies_unmet` naming it; under `strict_dependencies` gives refusal without override; delete the record and it starts). `cmd/status_test.go` (status shows the dependent not ready with the description).

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Add the implement worktrees opt-out setting

**File changes**:
- `internal/config/config.go:149-153` (next to `EpicConfig`) — add `type ImplementConfig struct { Worktrees bool \`yaml:"worktrees"\` }` with a doc comment.
- `internal/config/config.go:318-339` — add `Implement ImplementConfig \`yaml:"implement"\`` to `Config`.
- `internal/config/config.go:356-398` (`NewDefault`) — set `Implement: ImplementConfig{Worktrees: true}`. `ParseYAMLFile` (`:415-449`) unmarshals over the default, so an absent key stays true. No schema bump (`internal/config/schema.go:17`) and no migration step.
- Tests: `internal/config/config_test.go` — `TestNewDefault_HasExpectedDefaults` (`:25`) expects `Implement.Worktrees == true`; new `TestFromYAMLFile_AbsentImplementWorktreesLoadsAsOn` (a schema-4 config without `implement`); new `TestToYAMLFile_ImplementWorktreesFalseRoundTrips`, asserting the raw file text contains `worktrees: false` and reads back false (per the YAML boolean gotcha). Check that golden files `internal/migrate/testdata/golden/*/config.yaml.golden` and `internal/project/init_test.go` don't break because `ToYAMLFile` now writes an `implement:` block; update goldens if they compare a written config.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Keep each run started on its own in its own lane

**File changes**:
- `cmd/implement.go:94-114` — state path selection: a run is a **lane run** when it is orchestrated, or when it is not a dry run, `cfg.Implement.Worktrees` is true, and `--data` carries a valid `name`. A lane run probes and uses `workflow.LaneStatePath(dataDir, "implement", name)`. With no name, probe the shared slot as today. If nothing is resumable, the `name_required` next_action also lists in-progress implement lanes (`workflow.LaneNames`) with their `goto` resume commands.
- `cmd/implement.go:132-136` — `refuseLaneInProgress` applies only to non-lane (opt-out) starts. A lane start finds its own in-progress lane through `probeResume` and gets a resume report.
- `cmd/implement.go:181-199` — persist workflow data `lane: true` for lane runs. `orchestrated` stays false for a run started on its own.
- `cmd/workflow_slot.go:138-153` — generalise `orchestratedStart` into a lane decision that takes cfg, or add `implementLaneStart(cfg, dataStr, dryRun)` beside it, keeping plan's behaviour unchanged.
- `cmd/root.go:235-255` — the state-path resolver for `new` returns the lane for a lane implement start, not only for an orchestrated one.
- `cmd/resume.go:30-50` — `resumeInstruction` uses the lane notes path for any lane (orchestrated or `lane` data), and a fresh start offered for a lane run keeps the same `--data` (no `orchestrated:true` unless the run was orchestrated).
- `internal/stepkit/stepkit.go:106-112` — `working_context_path` is the lane notes path when data has `lane` or `orchestrated`. The orchestrated stop partial (`:133-139`) still keys only on `orchestrated`.
- `cmd/autocommit.go:99-112,166-186,251-256` — split `isOrchestrated` into `isLane` (`lane` or `orchestrated` data) and `isOrchestrated`. `finishLane`, the notes snapshot and restore use `isLane`. Plan-lane commit logic is unchanged. Implement lanes with a worktree record already commit through `commitImplementLane` (path-scoped artifacts including lane files, `:388-410`). Confirm the lane state and notes are among those paths for a non-orchestrated lane.
- `templates/skills/workflows/spek-implement/SKILL.md` — the resume guidance: with worktrees on, resume by running `implement new --data '{"name":"<spec>"}'` or the `goto` the resume report gives. Every `goto` already carries `name`.
- `templates/steps/resume_implement.md` — no change expected. Verify the rendered goto includes `name` for a lane.
- Tests: `cmd/implement_lane_test.go` (new): with worktrees on (fixture with a pre-written record or git-backed), start spec A, then spec B, and both succeed with separate lane files under `.spektacular/workflows/`. A goto with A's name advances only A. `implement new` for A again gives a resume report (`resumable: true`, `current_step`). With no name and an empty shared slot, `name_required` lists both lanes. The instruction never contains the orchestrated stop partial (`QUESTION:`). Finishing removes the lane files. With `implement.worktrees: false`, the start uses `state.json` exactly as before. Existing `cmd/resume_test.go` and `cmd/workflow_slot_test.go` keep passing.

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Parallel analysis (map every `orchestrated` read to "lane" or "hand back" meaning), sequential integration.

### Task: Create a spec's worktrees when an implement run starts on its own

**File changes**:
- `cmd/implement.go:155-178` — after `refuseUnmetDependencies` (`:155`) and `startGate` (`:165-169`), before `codeRootsFor` (`:174`), add `ensureImplementWorktrees(root, cfg, projectStore, input.Name, orchestrated, dryRun)`. It returns nil without doing anything when orchestrated, dry run, `!cfg.Implement.Worktrees`, or `worktree.ReadRecord(root, name)` finds a record; this check runs before any git call, so the no-git test (`cmd/implement_test.go:362`) still holds. Otherwise it runs `worktree.TouchedRepos(cfg, st, name)` (`internal/worktree/worktree.go:134-159`) and `worktreeManager().Ensure(name, touched)` (`cmd/epic_worktree.go:71-86`), returning its error unchanged.
- `internal/worktree/worktree.go:189-235` (`checkouts`) or `:380-404` (`ensureOne`) — before `git worktree add`, check that each checkout is a git work tree (`git rev-parse --is-inside-work-tree`) and has a commit (`git rev-parse --verify --quiet HEAD`). If not, return `output.NewError("worktree_unavailable", "<repo> at <path> is not a git repository" | "has no commits yet", so the spec cannot be built in its own worktree").WithResource(repo).WithNextAction("commit the repo's current state (git init and an initial commit if needed), or set implement.worktrees: false in .spektacular/config.yaml to build in the main checkouts")`. This also improves `epic worktree`'s error for the same case.
- Tests: `cmd/implement_worktrees_test.go` (new), using `worktreeProjectWith` (`cmd/epic_worktree_test.go:94`) with a plan touching `testproj` and `docs`. `implement new` creates `spek/<spec>` worktrees in both and writes the record. The first instruction names the worktree roots. A third registered repo not in the plan gets none. `implement: {worktrees: false}` creates none. `orchestrated:true` and dry run create none. A pre-existing record gives no git calls. A non-git sibling repo (no `git init`) gives `worktree_unavailable` with the opt-out in next_action. An empty repo with no commits gives the same. Go through `resetRootCmd`/`runRootCmd`, and pin git identity (`cmd/autocommit_test.go:55`).
- Existing tests that run `implement new` against a git fixture without expecting worktrees (`cmd/autocommit_test.go`, `cmd/startgate_test.go`, `cmd/implement_task_*_test.go`, `cmd/implement_dependencies_test.go`) — set `implement: {worktrees: false}` in their fixture config where the test is about main-checkout behaviour, or accept worktrees where the assertion is unaffected. Non-git fixtures (`writeFixturePlan` projects) need the opt-out or will now refuse; prefer adding the opt-out to the shared helpers (`depProject` `:41`, `setupImplementCmd` fixtures).

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis (one agent maps every test fixture that calls `implement new` and whether it is git-backed), then sequential integration of the command change and fixture updates.

### Task: Merge back at the end of a run started on its own

**File changes**:
- `internal/steps/implement/steps.go:202-249` (`finished`) — compute `merge_required`: `!orchestrated && len(cfg.CodeRoots) > 0 &&` the plan has no unchecked task (reuse the open-task check the `task_run` branch already does). Pass it in `Extra` along with `worktree_roots`. For a task run with open tasks and code roots, pass `worktrees_kept: true`.
- `templates/steps/implement/12-finished.md` — add a `{{#merge_required}}` section: "This spec was built in its own worktrees. Merge it back now: run `{{config.command}} implement merge --data '{\"name\":\"{{plan_name}}\"}'` from the project root. If it is refused, report the repos and conflicting paths it names to the user and stop. Never resolve a conflict yourself, and never merge, rebase or switch branches on your own initiative. The worktrees and branches are kept for the user. On success, report that the worktrees and branches were removed." Add a `{{#worktrees_kept}}` line: "The spec's worktrees are kept for the next task run." The `{{#orchestrated}}` branch is unchanged.
- `templates/skills/workflows/spek-implement/SKILL.md` — add a "Worktrees" section after the cross-repo note (`:42`): a run started on its own builds in the spec's worktrees unless `implement.worktrees` is false; where the code lives comes from the step output, and every sub-agent gets those locations; the finished step's `implement merge` brings the work back all or nothing; on a refusal, report and stop, never resolve, rebase or switch branches; a `worktree_unavailable` or `worktree_setup_failed` refusal is reported to the user with its next_action.
- Tests: `internal/steps/implement/steps_test.go` (finished with code roots and a complete plan, not orchestrated, contains `implement merge` and "Never resolve a conflict yourself"; orchestrated, no roots, or open tasks do not contain `implement merge`; open tasks with roots contain "kept for the next task run"). `internal/steps/implement/orchestrated_test.go:57` still passes. `internal/agent/instruction_surface_test.go:213-232` (the spek-implement subtest asserts "implement merge", "never resolve", "switch branches").

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Prove runs started on their own end to end

**File changes**:
- `cmd/implement_worktree_flow_test.go` (new) — model on `cmd/epic_flow_test.go:28-212` (`epicFlowCLI`, `epicFlowImplement`, `epicFlowTickPlan`, `epicFlowWalkToFinished`) and the two-repo `worktreeProjectWith`. Cases:
  1. A clean two-repo run with `auto_commit: off`: `implement new`, write code in each worktree root, tick tasks, walk to `finished`, assert the finished instruction contains `implement merge`. Before merge, `git status --porcelain --untracked-files=all -- ':!.spektacular'` is empty in both main checkouts. Run `implement merge`: both main lines contain the files, no `spek/` branches, no worktree dirs, no record.
  2. The same with `auto_commit: workflow` (artifacts committed in main, code on the branch).
  3. Isolation: specs A and B planned against `testproj`, both started with `implement new` and both left in progress in their own lanes. A file written in A's worktree is absent from B's worktree and from the main checkout. Both then finish and merge cleanly in turn.
  4. Conflict: commit a conflicting change on `docs`' main line after start; `implement merge` refuses naming `docs` and the path; both HEADs unchanged; worktrees and branches present.
  5. Opt-out: `implement: {worktrees: false}`: no `spek/` branch, code written to the registered roots shows in the main checkout; `epic worktree` for another spec in the same project still creates worktrees.
  6. An absent `implement` key creates worktrees.
- `tests/harbor/implement-workflow/environment/config.yaml` — add `implement:\n    worktrees: false`, since the seeded `/app` is not a git repo and the suite exercises main-checkout behaviour. Update any harbor oracle that pins 02–05 or finished text (`tests/harbor/implement-workflow/tests/`).

**Complexity**: High
**Token estimate**: ~50k tokens
**Agent strategy**: Parallel analysis (flow helpers vs. harbor oracles), sequential integration.

### Task: Document the worktree setting and setup command in the configuration reference

**File changes**:
- `docs:src/pages/configuration.mdx:33-89` — add to the example `config.yaml`, after the `epic:` block (`:65-69`):
  ```yaml
  implement:
    worktrees: true               # false builds runs started on their own in the main checkouts
  ```
- `docs:src/pages/configuration.mdx:97-100` — "Sixteen top-level keys" becomes "Seventeen", and `implement` is added to the list.
- `docs:src/pages/configuration.mdx` (after the `epic` ConfigKey, `:264-284`) — a new ConfigKey.
  **Content example**:
  ```mdx
  <ConfigKey name="implement" type="section" defaultValue="<code>worktrees: true</code>">

    How implement runs build a spek.

    - `implement.worktrees`: whether an implement run you start yourself builds
      the spek in its own git worktrees, one per repo its plan changes, on a
      branch named `spek/<spek>`, and merges them back when the plan is
      complete. On by default, including in projects whose configuration
      predates the setting. Set it to `false` to build in your main checkouts
      as before. Epic runs always use worktrees, whatever this says.

  </ConfigKey>
  ```
- `docs:src/pages/configuration.mdx:378-401` — add `worktree_setup: npm ci` to the example `repo.yaml`.
- `docs:src/pages/configuration.mdx` (repo keys, after `tags` `:445`) — a new ConfigKey.
  **Content example**:
  ```mdx
  <ConfigKey name="worktree_setup" type="string" defaultValue="none">

    A shell command run in each new worktree of this repo before an implement
    run starts work, such as `npm ci`. A worktree holds only tracked files, so
    this is where dependencies a build needs are prepared. It runs in the
    repo's folder inside the worktree. If it fails, the run is refused with the
    command's output and the worktree is removed. Set it with `repo add` or by
    editing `repo.yaml`.

  </ConfigKey>
  ```
- `docs:src/pages/projects.mdx:257-300` ("How configuration is split") — add `worktree_setup` to the `description, role, tags` ConfigKey's list of fields that live in `repo.yaml`, with one sentence pointing to the configuration reference.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent; verify with `npm run build`, `npx astro check`, the Rule 1 grep and an em-dash grep.

### Task: Document worktree runs, merging back and the narrowed warning

**File changes**:
- `docs:src/pages/how-it-works.mdx:432-458` (`PipelineStage` "Implement the Spek") — add a paragraph after the dependency-check text (`:442-454`).
  **Content example**:
  > Each implement run builds the spek in its own git worktrees, one for each repo its plan changes, on a branch named `spek/<spek>`. Two runs on different speks never see each other's half-finished changes, so you can implement two speks side by side from two terminals. When the plan is complete, the run merges its work back into every repo together, or into none if any repo would conflict. A conflict is shown to you with the files involved and left for you to resolve; Spek never resolves one itself. After a clean merge the worktrees and branches are removed. Set `implement.worktrees: false` to build in your main checkouts instead.
- `docs:src/pages/epics.mdx:351-369` ("Implement this epic") — add one sentence: "Each new worktree is prepared with the repo's `worktree_setup` command, if it declares one." Add a short paragraph on the warning: "Before it starts, the run checks for uncommitted changes, but only in the repos the epic's plans change. A dirty repo the epic does not build is ignored, and status names each dirty repo it does build in `dirty_repos`."
- `docs:src/pages/epics.mdx:413-445` — add `"dirty_repos": [],` after `"dirty": false,` (`:424`).
- `docs:CHANGELOG.md` — a new top entry:
  ```markdown
  ## 000065_implement-in-worktrees

  How it works: the implement stage explains that every run builds in its own worktrees and merges back when it finishes. Epics: the uncommitted-work warning covers only the repos an epic builds, status shows `dirty_repos`, and worktrees run the repo's setup command. Configuration: new `implement.worktrees` and `worktree_setup` keys. Projects: `worktree_setup` listed with a repo's own settings.
  ```

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent; verify with `npm run build`, `npx astro check`, the Rule 1 grep and an em-dash grep.

## Testing Strategy

Per task, tests sit beside the code they cover, using the project's existing harnesses:
- **Repo setup field**: config round-trip, registration upsert and `repo list` output tests.
- **Setup in worktrees**: `internal/worktree` tests with a fake `SetupRunner` (call counting, failure) and a real-git fixture (effect present, failed worktree removed); `epic worktree` cmd test.
- **Code-locations partial**: step-render phrase tests for 02–05, in both forms, and unescaped paths.
- **Dirty repos**: PR #79's `TestStatusRunSource_DirtyOnlyChecksTouchedRepos`, run view and status schema tests.
- **Forced code commits**: `points` unit tests, step-render commit-message tests, and a real-git auto-commit test with auto_commit off.
- **implement merge**: cmd tests on the two-repo `worktreeProjectWith` fixture (clean, conflict, `.spektacular` guard, not found, schema).
- **Unmerged dependencies**: status unit tests with the `Unmerged` hook; implement dependency cmd tests with a hand-written record; status cmd test.
- **Opt-out setting**: default, absent-key and raw-text round-trip tests; golden/init fixture updates.
- **Lanes for standalone runs**: cmd tests for two concurrent lanes, resume by name, `name_required` listing lanes, no orchestrated hand-back, and the opt-out using `state.json`.
- **Worktree creation at start**: cmd tests for touched-only creation, the skip conditions with no git, and the `worktree_unavailable` refusal; existing fixtures get the opt-out.
- **Finished-step merge**: step-render tests for `merge_required` and `worktrees_kept`; spek-implement skill phrase assertions.
- **End-to-end**: flow tests covering isolation, a clean main checkout before merge, clean and conflicting merges, the opt-out, an absent key and auto_commit off; harbor config opt-out.
- **Docs**: `npm run build`, `npx astro check`, the Rule 1 grep and an em-dash grep.

Every Go task ends with the full `go test ./...` passing. Tests executing `rootCmd` go through `resetRootCmd`/`runRootCmd` and pin git identity. Manual items (two real terminals, a real multi-repo epic run, the harbor implement run, the docs visual check, a real standalone two-repo run after `make install-local`) are captured in the implementation test plan.

## Project References

- Spec: `000065_implement-in-worktrees` (`spektacular spec file read 000065_implement-in-worktrees`).
- Design: `epics-and-seeded-specs.md` from the `design` source (Dependencies between specs; Status).
- Prior plan: `000064_epic-worktree-store-isolation` (worktree record, code-only worktrees, merge guard).
- PR: hivecommons/spektacular#79 (dirty_repos); issue hivecommons/spektacular#78.
- Knowledge: conventions (error remediation, plans never change the active install, store files through the CLI, order-independent tests, tests must pass); gotchas (YAML boolean tokens, mustache escaping, EnsureFootprint discards config, RepoConfig from default); architecture (workflow steps, testing architecture); docs conventions (no em dashes, MDX authoring, content structure in plans, label-before-filename headings).
- Repo roots: `spektacular` = `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` = `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

The two High tasks (worktree creation at start; end-to-end proof) should map the affected test fixtures in parallel first, then integrate sequentially. All other tasks fit a single agent.

## Migration Notes

None required. `implement.worktrees` defaults to on through `NewDefault()`, and `worktree_setup` is optional, so neither the project schema (4) nor the repo schema (2) changes, and no `migrate` step is added. Projects that want today's behaviour set `implement.worktrees: false`. Per the active-install convention, this repository's own configuration is never edited by a task. Run `make install-local` between workflows to use the new behaviour here.

## Performance Considerations

A run started on its own now runs `git worktree add` (one per touched checkout) and any setup command once at start, and a `merge-tree` dry run plus merges at the end. Setup commands such as `npm ci` can take minutes and run once per new worktree, never on reuse. The read paths (`codeRootsFor`, the unmerged check) remain file reads with no git.
