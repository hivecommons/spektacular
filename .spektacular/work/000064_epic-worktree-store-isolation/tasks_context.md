### Task: Record spec worktree code roots in the main project

Requirement attribution: spec R-"Status and resume read the project's records" and R-"implement workflow tells the agent where to change code" (prerequisite) → repo `spektacular`, worktree package.

- `internal/worktree/worktree.go:44-62` — add `type Record struct{Spec string; Repos map[string]string}` beside `SpecWorktrees`.
- `internal/worktree/worktree.go:244-312` (`Ensure`) — after the overlay map is built (283-297), also write the record as JSON to `<m.worktreeRoot(spec)>/record.json`. That is the main project's `.spektacular/worktrees/<spec>/`, a sibling of the worktree dirs and already excluded by `excludeFromProject` (`worktree.go:346-393`, pattern `/<rel>/.spektacular/worktrees/`). Keep writing the overlay for now; Task "Make the worktree repo view explicit…" removes it.
- `internal/worktree/worktree.go` — add `func ReadRecord(projectRoot, spec string) (Record, bool, error)`, a plain `os.ReadFile` with no git, returning `(Record{}, false, nil)` when the file is missing. Add `func (m Manager) Record(spec string)` delegating to it. Only absolute roots are kept, mirroring `readOverlay` (`internal/repo/set.go:108-127`).
- `internal/worktree/worktree.go:579-590` (`Merge` removal) — remove the record before `os.Remove(m.worktreeRoot(spec))` at 589, so the now-empty directory can be removed.
- `internal/worktree/worktree_test.go` — new tests using `newFixture` (118-143) and `manager()` (152):
  - the record after `Ensure` maps `proj` and `website` to paths under the spec's worktrees;
  - `ReadRecord` on an unknown spec reports absent;
  - the record is gone after a clean `Merge`, extending `TestMerge_CleanMergeLandsEverywhereAndCleansUp` (377);
  - `git status --porcelain` in the main project top does not list the record.
- `cmd/epic_worktree_test.go:114` (`runEpicWorktreeCmd`) — one assertion that `ReadRecord` returns the paths reported in the command's `repos` result.

Complexity: Low. Token estimate: ~25k. Agent strategy: single agent, sequential.

### Task: Refuse to merge a spec branch that changes the Spektacular directory

Requirement attribution: spec R-"Merging refuses a branch that changes the Spektacular directory" → repo `spektacular`, worktree package and `epic merge` command.

- `internal/worktree/worktree.go:522-541` (`Merge` precheck loop) — add a fourth check after `dirtyOverlap` (535-540): `m.git(rw.Top, "diff", "--name-only", "HEAD..."+branch, "--", <pathspecs>)`.
  - For the project checkout, the pathspec is `filepath.ToSlash(filepath.Join(projRel, ".spektacular"))`, using `projectTop()` (154-169).
  - For every checkout, also `:(glob)**/.spektacular/**`, so sibling repos and embedded repos are covered.
  - Collect the paths per repo across all repos first, then refuse once.
- `internal/worktree/worktree.go:99-101` — add a constructor for `output.NewError("epic_merge_touches_spektacular", "<spec>'s branch changes Spektacular's files, which are only ever written in the project: <repo>: <paths>; …")`. Its `WithNextAction` says: "undo those commits on spek/<spec> in <worktree path> (for example `git -C <path> revert <commit>`), record the change through the CLI from the project root, then retry `<command> epic merge`". `WithResource(spec)`.
- `cmd/epic_worktree.go:136-167` (`runEpicMerge`) — passes the error through; no change beyond confirming it is not swallowed by `mergeConflict` (171-185).
- `internal/worktree/worktree_test.go` — two tests modelled on `TestMerge_OverlappingMainCopyChangesAreRefused` (460):
  - commit `proj/.spektacular/plans/alpha/plan.md` on the spec branch;
  - commit `website/.spektacular/<file>` on the spec branch.
  - Both use `requireRefusal(t, err, "epic_merge_touches_spektacular")` (166), and assert `head()` (182) is unchanged on both main checkouts and the paths appear in the message.
  - Keep `TestMerge_CleanMergeLandsEverywhereAndCleansUp` (377) green as the code-only case.
- `cmd/epic_worktree_test.go:240-262` (`TestEpicWorktree_Refusals` table) — add a merge row for the new code.

Complexity: Low. Token estimate: ~25k. Agent strategy: single agent, sequential.

### Task: Make the worktree repo view explicit and stop writing into worktrees

Requirement attribution: spec R-"Isolated copies are used only for code" and constraint "worktree .spektacular never read or written" → repo `spektacular`, repo and worktree packages.

- `internal/repo/set.go:65-92` (`New`) — drop the `readOverlay` call and overlay application (76-79, 86-88); update the doc comment at 65-74.
- `internal/repo/set.go` — add `NewWithLocations(cfg, projectRoot, git, locs map[string]string)`, which applies `locs` exactly as the overlay did (absolute locations only, unmapped repos untouched). `New` calls it with nil.
- `internal/repo/set.go:94-127` — delete `OverlayFile`, `Overlay` and `readOverlay`.
- `internal/repo/set_test.go:696-715` — replace `TestNew_OverlayRelocatesMappedReposOnly` with:
  - a `NewWithLocations` test that relocates mapped repos only;
  - a test that `New` ignores a stray `.spektacular/worktree-repos.json`.
- `internal/worktree/worktree.go:264, 283-310` (`Ensure`) — build the map into the record only, and stop writing `<result.Project>/.spektacular/worktree-repos.json`.
- `internal/worktree/worktree.go:362-365` (`excludeFromProject`) — drop the `/**/.spektacular/` + `OverlayFile` exclude line.
- `internal/worktree/worktree.go:580-581` — reword the comment on the forced removal (still forced, because build output may remain ignored in a worktree).
- `cmd/epic_worktree_test.go:152-185` — replace `TestEpicWorktree_ReposResolveIntoWorktreesFromInside` with two tests:
  - after `epic worktree`, `git -C <project worktree> status --porcelain --ignored -- .spektacular` is empty, and `git diff` against base is empty;
  - `repo list` run from the main project reports registered roots.
- `internal/worktree/worktree_test.go` — assert that no `worktree-repos.json` exists under any worktree after `Ensure`.

Complexity: Medium. Token estimate: ~35k. Agent strategy: 2 parallel agents, one on the repo package with its tests and one on the worktree package with the cmd tests, then integrate.

### Task: Tell the implement workflow where spec code lives

Requirement attribution: spec R-"The implement workflow tells the agent where to change code" and constraint "standard implement skill unchanged / roots from workflow output" → repo `spektacular`, implement cmd, workflow config, step templates.

- `internal/workflow/workflow.go:17-36` — add `type CodeRoot struct{ Repo, Root string }` and runtime-only `CodeRoots []CodeRoot` to `Config`. Not persisted.
- `cmd/implement.go:173` (`runImplementNew` wfCfg) and `cmd/implement.go:269` (`runImplementGoto` wfCfg) — call `worktree.ReadRecord(root, specName)`. When present, fill `CodeRoots`, sorted in record order with the project repo first: use the order of `cfg.Repos`, filtered to mapped names. No git, no `SetData`. Use a small helper `codeRootsFor(root, cfg, name)` in `cmd/implement.go`.
- `cmd/autocommit.go:52+` (`gotoWithAutoCommit`) — receives the same wfCfg from `runImplementGoto`, so no change here beyond plumbing.
- `internal/steps/implement/steps.go:57-69` (`writeStep`) — pass `has_worktree_roots` and `worktree_roots` (a list of `{repo, root}`) into the extra bundle when `len(cfg.CodeRoots) > 0`. Precedent: `withDependencyOverride` (303-338).
- `templates/steps/implement/01-read_plan.md:5` — wrap the existing "**Where the code lives.**" paragraph in `{{^has_worktree_roots}}…{{/has_worktree_roots}}`. Add a `{{#has_worktree_roots}}` variant: "**Where the code lives.** This spec is built in its own worktrees. For the rest of this workflow, carry out every code-touching step (analysis, implementation, tests, verification) in the root listed for the repo the work belongs to, never in whatever directory you started in, and pass that root to any sub-agent you launch. Run every `{{config.command}}` command from the directory you started in:" followed by `{{#worktree_roots}}- `{{repo}}`: `{{{root}}}`{{/worktree_roots}}`. Keep the phrases pinned by `templates/repo_source_test.go:109-114` in both branches.
- `templates/steps/implement/01-read_plan.md:47` — the drift-check parenthetical gets the same two-form split: "(`{{config.command}} repo list` says where)" vs "(the roots listed above say where)".
- `templates/steps/implement/10-update_feature_changelog.md:23-26` — the same two-form split. The worktree branch lists the roots, and keeps `{{config.command}} repo list` only as the way to see the registered repos for the prefix rule (30-31).
- Tests:
  - `internal/steps/implement/steps_test.go` — new tests using `renderStepWithData` (52-60) with a `workflow.Config{CodeRoots: …}`:
    - read_plan lists each root;
    - read_plan and update_feature_changelog without roots are unchanged; keep `TestRepoListStepsSendTheAgentToRepoList` (753-763) and `TestUpdateFeatureChangelogStepDerivesOneEntryPerAffectedRepo` (507-529) green;
    - a root containing `&` renders unescaped.
  - `TestWhereTheCodeLivesPreambleRenderedOnceByReadPlan` (765-778) stays green.
  - `templates/repo_source_test.go:67-86` — confirm the new key is not `repos` and the test passes unchanged.
  - `cmd/implement_test.go:296-356` — extend `TestImplementNewAndGoto_PersistNoRosterAndRunNoGit` with a variant where a record exists under `.spektacular/worktrees/<spec>/record.json`. It asserts zero git calls, no `repos` in state data, and that the instruction names the recorded roots.
  - `cmd/instruction_contract_test.go:165-200` — renders with an empty config; confirm there are no unrendered `{{` (332).

Complexity: Medium. Token estimate: ~45k. Agent strategy: 2 parallel agents, one on the Go plumbing (config, cmd, steps.go and its tests) and one on the templates and template tests, then sequential integration.

### Task: Split implement auto-commits between worktree code and project artifacts

Requirement attribution: spec R-"Project artifacts are written to the project…" and R-"Isolated copies are used only for code" (commit side) → repo `spektacular`, autocommit.

- `internal/autocommit/targets.go:27-62` — add `TargetsWithLocations(cfg, projectRoot, git, locs)`, built on `repo.NewWithLocations`. `Targets` calls it with nil.
- `cmd/autocommit.go:186-216` — before the generic `Targets`/`CommitDirty` path, add a branch for `kind == "implement"` when `worktree.ReadRecord(root, specName)` is present. It does two things:
  1. **Code commits**: `TargetsWithLocations(cfg, root, git, record.Repos)` keeps only the targets whose `Dir` is under the spec's worktree root, then `CommitDirty`.
  2. **Artifact commits**: a new `commitImplementLane(cfg, root, statePath, specName, message)` modelled on `commitPlanLane` (247-269). It takes `autocommit.AcquireLock(<root>/.spektacular)` and commits path-scoped in the project top:
     - the plan store dir `filepath.Dir(implement.PlanFilePath(cfg.Plan.Config.Directory, name))`;
     - the spec file;
     - the project changelog record dir for the spec;
     - `.spektacular/work/<name>`;
     - `.spektacular/tmp/<name>`;
     - `statePath`;
     - `laneNotesFor(statePath)`.

     For each other touched repo, it also commits that repo's repo-routed changelog record path in its main checkout, resolved like `cmd/storefile.go:141` (`repoRoutedStore`) and using the registered, non-worktree root via `repo.New`. Paths that do not exist are skipped by `CommitPaths` itself (`internal/autocommit/git.go:87-119`).
- `cmd/autocommit.go:99-104, 231-239` — lane removal is unchanged. The finished lane's deletion is part of the path-scoped commit, as for plan lanes.
- `cmd/autocommit_test.go` — new tests using `pinGitIdentity`/`tempWorkTree` (52, 64) and the worktree fixture pattern from `cmd/epic_worktree_test.go:85-112`:
  - an implement goto that crosses a commit point leaves the code change committed on `spek/<spec>` in the worktree and the plan tick committed on the main branch;
  - a dirty file under another spec's plan dir in the main checkout stays uncommitted;
  - with no record, behaviour matches the existing `CommitAll` tests.

Complexity: High. Token estimate: ~55k. Agent strategy: parallel analysis of the autocommit and storefile changelog resolution, then sequential integration by one agent.

### Task: Read epic implement progress from the main project

Requirement attribution: spec R-"Status and resume read the project's records" → repo `spektacular`, status package.

- `internal/status/run.go:337-365` (`implementPart`):
  - When a worktree exists and `implementWorkflow(opts, s.Name)` (287-295) returns a main-project lane, return `RunInProgress` with `Root: sw.Project`, not `opts.Run.ProjectRoot`.
  - Delete the worktree-lane read (`filepath.Join(sw.Project, ".spektacular")` / `workflow.ReadLane`, 345-346 / 351-352).
  - Keep the interrupted fallback `RunInProgress{Root: sw.Project}`.
- `internal/status/run.go:375-389` (`finishedIn`) — read through `opts.Store` (the main project store) instead of `StoreAt(sw.Project)`. Drop the `sw.Project` parameter.
- `cmd/status.go:215-248` (`statusRunSource`) — remove `StoreAt` if it has no remaining users. `cmd/status.go:67` — reword the root description to "the project, or the spec's worktree for its code".
- `internal/status/*_test.go` — update the tests that seed a lane or stores inside a worktree to seed them in the main project, and add:
  - a test where a stray in-progress lane under the worktree's `.spektacular` is ignored;
  - a test for `awaiting_merge` from main-project ticks plus a final changelog.

Complexity: Medium. Token estimate: ~30k. Agent strategy: single agent, sequential.

### Task: Point epic children at the project root in the skills

Requirement attribution: spec constraint "every Spektacular command in an epic run from the project root" and "standard implement skill unchanged" → repo `spektacular`, skill templates and `epic worktree` help.

- `templates/skills/workflows/spek-implement-epic/SKILL.md`:
  - :15 — "Each spec is built in its own git worktrees… They hold only code: every child runs Spektacular from the project root, and the implement workflow tells it where each repo's code lives in those worktrees." Remove "Inside the spec's project worktree, every registered repo resolves…".
  - :17 — keep "(with its step and the worktree it runs in)".
  - :44 — "start a child to **resume** it (its `root` is the spec's worktree, where its code lives)".
  - :51 — "Then start a child to **start** it."
  - :63 — the child prompt: "the spec name, and the project root to run every `{{command}}` command from. The implement workflow's own instructions name where each repo's code lives; it never runs `{{command}}` from inside a worktree and never touches a worktree's `.spektacular` directory."
  - :97 — "start a fresh child on the same spec with the answer included in its prompt; it resumes from its lane".
- `templates/skills/workflows/spek-implement/SKILL.md:138-140` — "…each running this skill for one spec." and "Start with `{{command}} implement new --data '{…\"orchestrated\":true}'`, from the project root you were given."
- `cmd/epic_worktree.go:33-36` (`epic worktree` Long) — "…The worktrees hold only code: Spektacular itself always runs from the project, and the implement workflow names each repo's worktree root."
- Tests:
  - `templates/implement_epic_skill_test.go:79` (`spekx repo list` must appear) — remove the assertion, or move it to a phrase still present.
  - `templates/implement_epic_skill_test.go:105-119, 234, 240, 313` — update to the new phrases.
  - Add NotContains for "repo list`, run from there" and "the root to work in: the spec's project worktree".
  - `templates/orchestrated_skill_section_test.go:43-44` — update both pinned sentences.
  - `cmd/instruction_contract_test.go:384-389` stays green (no host paths).

Complexity: Low. Token estimate: ~25k. Agent strategy: single agent, sequential.

### Task: Prove epic spec artifacts stay in the project end to end

Requirement attribution: spec acceptance criteria "Plan ticks appear in the project mid-run", "Changelog records land in the project", "Isolated copies' Spektacular directory is untouched", "Merged branches carry no Spektacular directory changes", "Epic children change code only in their copies", and success metric "no manual reconciliation" → repo `spektacular`, cmd tests.

- `cmd/epic_flow_test.go` (new) — build on `worktreeProject(t, withPlan)` (`cmd/epic_worktree_test.go:85-112`) with a sibling repo. The flow:
  1. Run `epic worktree`.
  2. Run `implement new {"orchestrated":true}` from the main root (`t.Chdir`), and assert the instruction lists the worktree roots.
  3. Write a code file in the project worktree and in the sibling worktree.
  4. Tick the plan's task through `plan file write` from the main root, write the changelog through `changelog file write`, and drive `implement goto` steps to `finished` with auto_commit on.
  5. Check that `plan file read` from the main root shows the tick before merge, and that `status <epic>` reports `awaiting_merge` with the worktree as root.
  6. Check that every worktree's `.spektacular` has `git diff <base> -- .spektacular` and `status --porcelain --ignored -- .spektacular` empty.
  7. Run `epic merge`, and check that the main project's plan, changelog and spec are byte-identical before and after the merge, the code files are present on main, and `git status` is clean.

  Use `resetRootCmd`/`runRootCmd`, `pinGitIdentity`, and `writeSpecCommandConfig` (`cmd/spec_test.go:26`).
- A second, short test: a sibling-worktree commit under `.spektacular/` is refused by `epic merge` (ties the guard into the flow).

Complexity: Medium. Token estimate: ~40k. Agent strategy: single agent, sequential.

### Task: Update the epics page for code-only worktrees

Requirement attribution: spec R-"Documentation describes the new model" → repo `docs`.

- `docs:src/pages/epics.mdx:355-358` — replace the worktree bullet.
- `docs:src/pages/epics.mdx:359-363` — add one bullet after the merge bullets for the new refusal.
- `docs:src/pages/epics.mdx:418-428` — the status example is unchanged (it has no `root`).

**Content example** (illustrative wording; structure and facts fixed):

```mdx
  - Each ready spek is built in its own git worktrees, one for each repo it
    changes, on a branch named `spek/<spek>` in each. The worktrees hold only
    code, so speks running side by side never touch each other's files.
  - Specs, plans, changelogs and progress stay in your project the whole time.
    Every agent runs Spektacular from the project, so you can follow a spek's
    plan being ticked off while the run is going.
  - A spek is merged back before anything that depends on it starts. ...
  - A spek whose branch changes Spektacular's own files is not merged. Those
    files are only ever written in the project, so the change is shown to you
    to undo.
```

Follow the docs conventions: no em dashes, no layout HTML in the MDX body, prose and lists only. Verify with `npm run build` and `npx astro check`.

Complexity: Low. Token estimate: ~10k. Agent strategy: single agent, sequential.
