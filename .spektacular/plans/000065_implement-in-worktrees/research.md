---
created_date: "2026-10-07"
document_status: final
closed_date: "2026-10-07"
---

# Research: 000065_implement-in-worktrees

## Alternatives considered and rejected

### Option A: A second isolation mechanism for standalone runs (e.g. a per-run copy or a stash)

A second isolation mechanism for standalone runs (e.g. a per-run copy or a stash)

**Rejected**: the spec's constraints require reusing epic worktrees, and `internal/worktree/worktree.go:296-404` (`Ensure`/`ensureOne`) and `:573-664` (`Merge`) already give per-spec creation and all-or-nothing merge with cleanup.

### Option B: The CLI merges automatically inside `gotoWithAutoCommit` when entering `finished`

The CLI merges automatically inside `gotoWithAutoCommit` when entering `finished`

**Rejected** (user decision, discovery): The commit runs after `Goto` (`cmd/autocommit.go:170-245`), so the merge would come after a state transition the agent has already been told about. A refused merge would need a state rollback, and the finished callback's changelog and test-plan close (`internal/steps/implement/steps.go:202-249`) would have to be idempotent. An explicit `implement merge` command that the finished step tells the agent to run mirrors how the epic orchestrator already runs `epic merge` (`templates/skills/workflows/spek-implement-epic/SKILL.md:82-92`).

### Option C: Fall back to the main checkout when a touched repo is not a git repo or has no commits

Fall back to the main checkout when a touched repo is not a git repo or has no commits

**Rejected** (user decision): it would lose isolation silently. Refuse with a remediation instead (`internal/worktree/worktree.go:115` `failed`).

### Option D: Merge after every single-task run

Merge after every single-task run

**Rejected** (user decision): merge only when the plan is complete. Partial task runs keep and reuse the worktrees, because the record is present (`cmd/implement.go:286-298` `codeRootsFor`).

### Option E: Detect "unmerged" only in `refuseUnmetDependencies`

Detect "unmerged" only in `refuseUnmetDependencies`

**Rejected**: the design (`design:epics-and-seeded-specs.md`, "Reported by status") requires the implement check and `status` to classify a dependency through the same function. Put it in `status.DependenciesOf`/`buildTarget` (`internal/status/report.go:208-273`) so both agree.

### Option F: Detect "unmerged" with git (branch `spek/<spec>` not merged into HEAD)

Detect "unmerged" with git (branch `spek/<spec>` not merged into HEAD)

**Rejected**: it breaks the no-git read path of the implement commands (`cmd/implement_test.go:362`). The worktree record's existence is enough: `Merge` deletes it on success (`worktree.go:647-663`), so a record means the spec is unmerged.

### Option G: Store the worktree setup command in the project's `repos[]` entry (`config.RepoEntry`, `internal/config/config.go:268`)

Store the worktree setup command in the project's `repos[]` entry (`config.RepoEntry`, `internal/config/config.go:268`)

**Rejected**: the spec says it travels with the repo, and `config.go:255-267` keeps a repo's descriptive data in its own `repo.yaml`.

### Option H: Bump the project schema (4→5) and add a migration for the worktree opt-out

Bump the project schema (4→5) and add a migration for the worktree opt-out

**Rejected**: `ParseYAMLFile` unmarshals over `NewDefault()` (`internal/config/config.go:415-449`), so a default-true bool loads as true when the key is absent, which meets "existing projects get worktrees without a config change". A bump would churn about a dozen `schema: 4` fixtures and force a `migrate` of this very repo (convention: plans never change the active install).

### Option I: Port PR #79 wholesale

Port PR #79 wholesale

**Rejected**: its templates and skill text predate 000064 (they tell the child to run from the worktree), and its main-checkout baseline would false-positive on project store writes. Only its `dirty_repos` code and tests are reused.

### Option J: Run the setup command for every registered repo in a new checkout

Run the setup command for every registered repo in a new checkout

**Rejected**: run it only for touched repos. The project checkout always holds every colocated repo (`worktree.go:189-235`), and untouched ones need no setup.

## Chosen approach — evidence

- `cmd/implement.go:56-201` `runImplementNew`: insertion point for worktree creation, after `refuseUnmetDependencies` (`:155`) and `startGate` (`:165-169`), before `codeRootsFor` (`:174`). It is skipped when orchestrated, on a dry run, when the opt-out is set, or when a record already exists (resume, epic-created, or a later task run).
- `cmd/epic_worktree.go:71-86` `worktreeManager()`, `:88-136` `runEpicWorktree`, `:138-187` `runEpicMerge`/`mergeConflict`: reusable to build the manager, run `TouchedRepos` + `Ensure`, and render conflicts for `implement merge`.
- `internal/worktree/worktree.go:380-404` `ensureOne` returns `made`, the hook point for the setup command. `:93-104` `Runner` is the fakeable pattern a setup runner follows. `:115` `failed` builds a refusal with a next_action.
- `internal/worktree/worktree.go:573-664` `Merge`: all-or-nothing prechecks (MERGE_HEAD, dirty worktree, `dirtyOverlap`, the `.spektacular` guard `:675-707`), a `merge-tree` dry run, then merge and cleanup.
- `internal/status/report.go:229-237` `Unmet()` and `:242-273` `DependenciesOf`, used by both `refuseUnmetDependencies` (`cmd/implement.go:328-376`) and the status spec graph (`buildSpec`/`buildTarget` `:316-382`). This is one place to add "implemented but not merged".
- `internal/status/run.go:271-279` `isImplDone` and `:333-393` `RunAwaitingMerge`/`finishedInProject`: the run view already treats a spec with worktrees as not done.
- `internal/autocommit/points.go:29-49,83-105` `PointFor`/`LeadsToCommit` and `cmd/autocommit.go:77-116,207-228` (`commitImplementLane`, `:309-373`): with auto_commit off, `PointNone` means nothing is committed, so a worktree run needs completion points forced on for code only. `internal/stepkit/stepkit.go:142` asks for the commit message.
- `internal/steps/implement/steps.go:57-69,345-360` `writeStep`/`withCodeRoots`: every implement step already receives `has_worktree_roots`/`worktree_roots`, so steps 02–05 can render a shared partial.
- `internal/config/config.go:149-153,318-339,356-398,415-449`: config structs, defaults and parse-over-defaults.
- `internal/config/repo.go:33-145`, `internal/repo/register.go:42-191`, `cmd/repo.go:52-410`: `RepoConfig`, `Register` (with the descriptive-equality check that must include the new field) and `repo add`/`repo list` I/O and schemas.
- `internal/gitexec/gitexec.go:31-62`: the stderr-capturing exec pattern to model a `sh -c` setup runner on.
- PR #79 (`gh pr diff 79`): `RunSource.Dirty func(repos []string) []string`, `EpicRun.DirtyRepos` (`json:"dirty_repos"`, never null), and `statusRunSource` filtering cfg to touched repos (`cmd/status.go:215-246`). Tests: `cmd/status_dirty_test.go`, `TestRun_DirtyAndReposPassThrough`.

## Files examined

- spektacular:cmd/implement.go:56-201 — `implement new` flow; no worktree creation today.
- spektacular:cmd/implement.go:203-281 — `implement goto` recomputes code roots from the persisted name.
- spektacular:cmd/implement.go:286-298 — `codeRootsFor` reads `record.json`, no git.
- spektacular:cmd/implement.go:328-392 — dependency refusal (`dependencies_unmet`, `dependency_override_refused`, `withOverride`).
- spektacular:cmd/autocommit.go:55-249 — `gotoWithAutoCommit`; commits after Goto; `finishLane` for orchestrated runs.
- spektacular:cmd/autocommit.go:309-410 — `commitImplementLane` splits code (worktree) from artifacts (main, under the lock).
- spektacular:cmd/autocommit.go:426 — `startGate`, a no-op when auto_commit is off.
- spektacular:cmd/epic_worktree.go:20-193 — `epic worktree`/`epic merge` commands, `worktreeGit` swappable.
- spektacular:cmd/status.go:86,185-187,215-246 — dirty schema, run source attachment, the `Dirty` closure.
- spektacular:internal/worktree/worktree.go:37-112 — conventions, types, Record, Runner, Manager.
- spektacular:internal/worktree/worktree.go:134-235 — `TouchedRepos` (from plan tasks' `Repo`) and `checkouts` grouping.
- spektacular:internal/worktree/worktree.go:257-377 — record path, `ReadRecord`, `Ensure`, `codeRootIn`, `writeRecord`.
- spektacular:internal/worktree/worktree.go:380-456 — `ensureOne`, `excludeFromProject`.
- spektacular:internal/worktree/worktree.go:573-743 — `Merge`, `.spektacular` guard, `dirtyOverlap`.
- spektacular:internal/status/classify.go:57-99 — `Classify`, `Describe`.
- spektacular:internal/status/report.go:208-382 — `Dependency`, `Unmet`, `DependenciesOf`, `buildTarget`, `buildSpec`.
- spektacular:internal/status/run.go:35-87,183-185,271-393 — RunSource, EpicRun.Dirty, the awaiting-merge logic.
- spektacular:internal/autocommit/points.go:29-105 — commit points and LeadsToCommit.
- spektacular:internal/autocommit/targets.go:13,35 — `Target`, `TargetsWithCodeRoots`.
- spektacular:internal/stepkit/stepkit.go:142 — appends the commit-message partial at commit points.
- spektacular:internal/steps/implement/steps.go:27-69,202-249,303-360 — FSM, finished callback, override and code-root rendering.
- spektacular:internal/workflow/workflow.go:36-47 — runtime `Config.CodeRoots`.
- spektacular:internal/config/config.go:40-44,149-153,268-303,318-453,645-653,879-895 — auto_commit, EpicConfig, RepoEntry, Config, defaults, parse, validate, write.
- spektacular:internal/config/repo.go:17-145,329 — RepoConfig, load/validate/write.
- spektacular:internal/config/schema.go:17-22 — project schema 4, repo schema 2.
- spektacular:internal/repo/register.go:16-191 — Registration, Register, descriptive-field equality.
- spektacular:internal/repo/set.go:197,245 — `DescriptiveMetadata`, `Footprint`.
- spektacular:cmd/repo.go:52-410 — repo add/list schemas and handlers.
- spektacular:internal/gitexec/gitexec.go:31-74 — exec with stderr capture (git only).
- spektacular:internal/output/writer.go:46-81 — `NewError`/`WithResource`/`WithNextAction`.
- spektacular:templates/steps/implement/01-read_plan.md:5-14,56 — "Where the code lives".
- spektacular:templates/steps/implement/02..05 — analyze/implement/test/verify; 04 and 05 spawn sub-agents.
- spektacular:templates/steps/implement/10-update_feature_changelog.md:23-32 — worktree roots.
- spektacular:templates/steps/implement/12-finished.md — no merge today; `{{#orchestrated}}` hands back DONE.
- spektacular:templates/partials/implement-current-task.md — the current-task block used by 03/04/05.
- spektacular:templates/skills/workflows/spek-implement/SKILL.md:42,54-71,113-143 — cross-repo, dependency, uncommitted and orchestrated sections.
- spektacular:templates/skills/workflows/spek-implement-epic/SKILL.md:15,36,45-92,114 — worktree model, dirty, merge.
- spektacular:templates/skills/workflows/spek-manage-repos/SKILL.md — repo.yaml concepts, repo add fields.
- spektacular:templates/.spektacular/.gitignore — `worktrees/` already ignored.
- spektacular:cmd/implement_test.go:19-53,362 — helpers; no-git record test.
- spektacular:cmd/implement_dependencies_test.go:41-255 — `depProject`, the dependency tests.
- spektacular:cmd/epic_worktree_test.go:29-354 — real two-repo fixture (`worktreeProjectWith`).
- spektacular:cmd/epic_flow_test.go:28-212 — end-to-end epic flow helpers.
- spektacular:cmd/autocommit_test.go:45-79,599-673 — git fixtures; worktree commit tests.
- spektacular:internal/worktree/worktree_test.go:37-640 — fixture, Ensure/Merge tests.
- spektacular:internal/steps/implement/steps_test.go:44,845-901 — render helpers, worktree-root template tests.
- spektacular:internal/agent/instruction_surface_test.go:180-232 — rendered skill phrase assertions.
- spektacular:templates/implement_epic_skill_test.go:25-341 — epic skill phrase tests.
- spektacular:internal/status/run_test.go:435; cmd/status_test.go:624-671 — dirty tests.
- spektacular:internal/config/config_test.go:25,136-195,488 — config default/round-trip tests.
- docs:src/pages/configuration.mdx:33-89,97-100,172-184,264-284,326-355,359-494,521-530 — config example, key count, auto_commit, epic, repos, repo.yaml keys, migrate.
- docs:src/pages/how-it-works.mdx:432-458 — "Implement the Spek" pipeline stage (no worktree text).
- docs:src/pages/epics.mdx:351-369,405-445 — "Implement this epic" worktree/merge text; status JSON with `dirty`.
- docs:src/pages/projects.mdx:257-300 — "How configuration is split" ConfigKeys.
- docs:CHANGELOG.md — one `## <spek>` entry per spek, newest first.

## External references

- `git worktree` docs (add/remove, branch per worktree): the mechanism and its limits. Ignored files are not checked out, which is why the setup command exists.
- `git merge-tree --write-tree`: the conflict dry run behind the all-or-nothing merge.
- GitHub issue hivecommons/spektacular#78: the problems this spec fixes.
- GitHub PR hivecommons/spektacular#79: the source of the `dirty_repos` change.

## Prior plans / specs consulted

- Plan 000064_epic-worktree-store-isolation: introduced the worktree record, code-only worktrees, `.spektacular` merge guard, two-form "Where the code lives" templates, split auto-commit. This plan extends it to standalone runs and keeps its zero-git read path.
- Design `design:epics-and-seeded-specs.md` (binding): defines a dependency, "implemented", the warn/refuse tree under `epic.strict_dependencies`, and that implement and `status` classify through the same function.

## Open assumptions

- A worktree record's presence means the spec is not merged. It holds as long as `Merge` is the only remover of the record and nothing else deletes it.
- A setup command runs with `sh -c` in the repo's code root inside the new worktree. Windows is not supported for setup commands.
- `git worktree add` from HEAD means uncommitted main-checkout changes are absent from a standalone run's worktree. The docs say so. The existing start gate still offers to commit when auto_commit is on.
- yaml.v3's non-strict unmarshal lets older Spektacular builds ignore a new `repo.yaml` key, so no repo schema bump is needed.
- The harbor implement suite's seeded project must be a git repo with a commit (or set the opt-out). Verify when updating the suite.

## Drafting assumptions

### Unmerged is read from the worktree record (discovery)
- **Decision**: A dependency counts as implemented-but-unmerged when every task is complete and its worktree record still exists. The check lives in `status.DependenciesOf`, so the implement check and `status` agree.
- **Rationale**: `Merge` deletes the record on success. Reading it needs no git, which keeps the implement commands' no-git read path. The design requires one shared classification function.
- **Rejected**: a git branch-merged check (adds git to the read path); an implement-only check (status would disagree, against the design).

### Worktree opt-out without a schema bump (discovery)
- **Decision**: A new `implement.worktrees` bool, default true through `NewDefault()`. No project schema bump or migration.
- **Rationale**: Config parses over defaults, so absent keys load as true, which meets the acceptance criterion. A bump would force a migrate of this repo and churn many fixtures.
- **Rejected**: schema 4→5 with a `setDefault` migration.

### Setup command lives in repo.yaml (discovery)
- **Decision**: `worktree_setup` (string) in the repo's own `repo.yaml`, settable via `repo add --data` and shown by `repo list`. Read from the main registration, never from a worktree. Run with `sh -c` in each touched repo's code root of a newly created worktree.
- **Rationale**: The spec says it travels with the repo. Reading from the main registration honours "nothing under a worktree's .spektacular is read".
- **Rejected**: the project `repos[]` entry; reading the copy inside the worktree.

### A failed setup removes the just-created worktree (discovery)
- **Decision**: When a setup command fails, the checkout's newly created worktree is removed, along with the branch if it was created in that call, and the run is refused with the repo, the command and its stderr.
- **Rationale**: Otherwise a retry finds the worktree, skips setup (`made=false`) and builds in an unprepared tree.
- **Rejected**: a setup-done marker file (more state); leaving the broken worktree.

### Chosen direction: reuse epic worktree machinery from implement new (architecture)
- **Decision**: `implement new` (non-orchestrated, worktrees on, no record yet) calls `TouchedRepos` + `Manager.Ensure`. A new `implement merge` command wraps `Manager.Merge`, and the finished step instructs it only for complete, non-orchestrated worktree runs. Worktree runs force code-only commits at completion points when auto_commit is off. Unmerged dependencies are classified in `status.DependenciesOf`. The setup command lives in repo.yaml and runs in `Ensure`. A shared partial in steps 02–05 carries the sub-agent and dependency guidance. PR #79's dirty_repos code is ported.
- **Rationale**: one creation and merge path (spec technical approach); keeps the 000064 no-git read path; satisfies the design's shared-classifier rule; user choices on merge trigger, non-git refusal and task runs.
- **Rejected**: see research.md alternatives.

### Code-only commits when auto_commit is off (architecture)
- **Decision**: For a run with a worktree record and auto_commit off, completion points (`update_changelog→finished`, `reconcile_spec→finished`) commit code in the worktrees only. Main-checkout artifacts are left uncommitted, as auto_commit off does today. Milestone points are not forced.
- **Rationale**: The spec requires the spec branch to record the work whatever the setting. Leaving main-checkout artifacts alone keeps the off setting's meaning there.
- **Rejected**: forcing artifact commits in main (overrides the user's off setting); committing per milestone (not required).

### Conventions selected (architecture)
- **Decision**: Kept error remediation, the active-install rule, store access, order-independent tests, tests-pass, the YAML bool and mustache gotchas, and the docs no-em-dash, MDX, content-skeleton and heading conventions. Dropped docs alternate-section-background and site-layout (no new sections or components).
- **Rationale**: Each kept entry drives a concrete choice in this plan.
- **Rejected**: n/a.

### Sub-agent guidance also renders without worktrees (architecture)
- **Decision**: The new partial in steps 02–05 has an inverted section that tells the agent to pass each repo's `repo list` root to sub-agents when there are no worktree roots.
- **Rationale**: The requirement applies to every run. The opt-out case still has sub-agents that inherit the wrong cwd in multi-repo projects. The 000064 "renders unchanged" test is updated accordingly.
- **Rejected**: rendering guidance only for worktree runs.

### New `implement merge` command rather than reusing `epic merge` (components)
- **Decision**: Add `implement merge --data '{"name":"<spec>"}'`, sharing `Manager.Merge` and conflict reporting with `epic merge`.
- **Rationale**: The finished step runs it for specs with no epic, and `epic merge` is documented as an epic operation. The thin wrapper keeps a single merge implementation.
- **Rejected**: telling standalone runs to call `epic merge` (confusing for specs outside an epic).

### Unmerged as a hook on status.Options, spec state unchanged (data_structures)
- **Decision**: `Options.Unmerged func(spec) bool` (callers back it with `worktree.ReadRecord`) and `Dependency.Unmerged`. `Unmet` includes unmerged dependencies. The spec's own `SpecState` stays `implemented`.
- **Rationale**: The status package reads through a store and has no project root. A hook keeps it testable. Leaving `SpecState` alone avoids changing the design's state table and epic completion.
- **Rejected**: a new `SpecState` value (changes the design's state table and status output for every view).

### New error codes (data_structures)
- **Decision**: `worktree_unavailable`, `worktree_setup_failed` and `worktree_not_found`. Merge refusals reuse the existing epic merge codes.
- **Rationale**: These are distinct remediations. Reusing the merge codes keeps agents' existing handling.
- **Rejected**: reusing the generic `worktree_failed` for everything (loses the remediation distinction).

### Milestone order: epic fixes first, standalone worktrees third (milestones)
- **Decision**: M1 covers the setup command, sub-agent guidance and dirty_repos. M2 covers forced commits, `implement merge` and unmerged dependencies. M3 turns standalone worktrees on by default. M4 is docs.
- **Rationale**: Each milestone is independently useful, and M3 depends on M1's setup and M2's merge and commit. Turning the default on last means no intermediate state leaves standalone runs with unmergeable worktrees.
- **Rejected**: one large standalone-worktrees milestone first (the epic fixes would wait behind the riskiest change).

### Harbor implement suite uses the opt-out (tasks)
- **Decision**: Add `implement: {worktrees: false}` to the harbor implement suite's seeded config.
- **Rationale**: Its `/app` project is not a git repo, so with the default on the run would be refused. The suite keeps exercising main-checkout behaviour, which the opt-out must preserve exactly.
- **Rejected**: making the harbor fixture a git repo that exercises worktrees (a bigger suite change; worktree behaviour is covered by the Go flow tests).

### Existing implement test fixtures get the opt-out (tasks)
- **Decision**: Shared fixtures that call `implement new` against non-git or main-checkout scenarios set `implement.worktrees: false`.
- **Rationale**: Those tests assert today's behaviour; worktree behaviour gets its own tests.
- **Rejected**: initialising git in every fixture (slower, and it changes what those tests prove).

### Non-git readiness check lives in the worktree manager (tasks)
- **Decision**: The `worktree_unavailable` refusal is raised inside the manager, so `epic worktree` benefits too.
- **Rationale**: One creation path; the epic path had the same failure with a less helpful message.
- **Rejected**: checking only in `implement new`.

### Lane-ness keyed on worktrees on (tasks)
- **Decision**: (User chose per-spec lanes.) A non-orchestrated, non-dry-run implement start with a name and `implement.worktrees: true` is a lane run (data `lane: true`). Lane-ness is separated from `orchestrated` (hand-back). With no name, the shared slot is probed, and `name_required` lists in-progress lanes.
- **Rationale**: This is needed for the two-terminal acceptance criterion and reuses the existing lane files and goto-by-name resolution.
- **Rejected**: keeping the shared slot (fails the criterion).

## Rehydration cues


- `spektacular spec file read 000065_implement-in-worktrees`
- `spektacular design read --data '{"source":"design","path":"epics-and-seeded-specs.md"}'` (Dependencies between specs, Status)
- `spektacular plan file read 000064_epic-worktree-store-isolation plan`
- Re-read: `cmd/implement.go`, `cmd/autocommit.go`, `cmd/epic_worktree.go`, `internal/worktree/worktree.go`, `internal/status/report.go`, `internal/status/run.go`, `internal/steps/implement/steps.go`, `templates/steps/implement/*.md`.
- `gh pr diff 79 --repo hivecommons/spektacular`
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
