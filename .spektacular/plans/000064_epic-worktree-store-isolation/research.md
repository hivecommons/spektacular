---
created_date: "2026-10-06"
document_status: final
closed_date: "2026-10-06"
---

# Research: 000064_epic-worktree-store-isolation

## Alternatives considered and rejected

- **Child keeps running from the project worktree; stores redirected to the main project.** Rejected: the worktree's `.spektacular/` would still be read (config, overlay), violating the spec constraint that it is never read or written. Today `projectRoot()` is the cwd (`cmd/root.go:397-403`) and every store, lane and overlay derives from it.
- **Spec-scoped `repo list --data '{"spec":...}'` for the child.** Rejected by the user and by spec constraint: it makes the child epic-aware.
- **Discover the spec's worktrees with git (`worktree.Manager.Find`) inside `implement new`/`goto`.** Rejected: `cmd/implement_test.go:296-356` (`TestImplementNewAndGoto_PersistNoRosterAndRunNoGit`) pins zero git calls and no persisted roster on `implement new` and `goto`; `Manager.List` (`internal/worktree/worktree.go:431-486`) shells out to `git worktree list` for every registered top.
- **Derive worktree code roots from the path convention alone (`.spektacular/worktrees/<spec>/<dir>`).** Rejected: the repo's path inside its worktree is relative to its checkout's git top (`worktree.go:283-297`), which needs git to compute; a record written once at `epic worktree` time avoids it.
- **Persist the roots in workflow data (`SetData`).** Rejected: same zero-roster test, and roots are a property of the worktree, not of the workflow.
- **Auto-commit the main checkout with `CommitAll` during an epic implement lane.** Rejected: with several specs running at once, `CommitAll` (`internal/autocommit/commit.go:47-63`) would sweep other specs' in-flight artifacts and the user's own work into one spec's commit, and would commit code into the main line if the agent ever touched it. Plan lanes already solved this with a path-scoped commit under a lock (`cmd/autocommit.go:190-198, 247-269`, `internal/autocommit/lock.go:15`).
- **Leave artifact commits to `epic merge`.** Rejected: that is the current model the spec removes.

## Chosen approach — evidence

- Lanes are already keyed per kind and spec in the main project: `internal/workflow/lane.go:15-66` (`<dataDir>/workflows/<kind>-<name>.json|.md`); orchestrated plan lanes already coexist there. Implement lanes need no new storage, only to be created from the main root.
- Lane notes path is chosen in `internal/stepkit/stepkit.go:106-113` and `cmd/resume.go:35-37`, both cwd-relative, so they follow the main root automatically.
- Overlay has a single writer (`worktree.Manager.Ensure`, `internal/worktree/worktree.go:264, 283-310`) and a single reader (`repo.New`, `internal/repo/set.go:75-92, 108-127`); `Overlay.Spec` is written but never read. Relocating the record and making overlay application explicit touches two functions.
- `Ensure` already excludes `/<rel>/.spektacular/worktrees/` via info/exclude (`worktree.go:346-393`), so a per-spec record stored under the main project's `.spektacular/worktrees/<spec>/` is never committed and sits outside every worktree.
- Steps get no repo data today (`internal/workflow/workflow.go:17-36`); precedent for cmd feeding config into steps: `EpicDir` (`cmd/spec.go:276,385`, `internal/steps/spec/steps.go:255`). List rendering precedent: `dependency_override` (`internal/steps/implement/steps.go:303-338`, `templates/steps/implement/10-update_feature_changelog.md:43-52`).
- `templates/repo_source_test.go:67-86` forbids `{{#repos}}` / `## Repos` rosters in templates, so the worktree block needs its own key and must be scoped to the epic case.
- `status` reads the worktree's lane today (`internal/status/run.go:341-355`) and `finishedIn` reads the worktree's store (`run.go:379-389`); both switch to main-project reads. `implementWorkflow` (`run.go:287-295`) already checks main lanes first, but reports `Root: opts.Run.ProjectRoot`.
- Merge prechecks run per repo before the dry run (`worktree.go:522-541`); `dirtyOverlap` already uses `diff --name-only HEAD...<branch>` (`worktree.go:611`), the same form a `.spektacular/` guard needs.
- Auto-commit targets come from `repo.New` + `LocalSource` (`internal/autocommit/targets.go:27-62`); with the overlay gone from `New`, a spec-scoped view is needed to point code commits at worktrees.

## Files examined

- cmd/root.go:384-403 — `dataDir()`/`projectRoot()` are the cwd; no parent search.
- cmd/implement.go:55-196 — `implement new`: lane state path (94-102), resume probe (106), orchestrated skips startGate (164-168), no repo set built.
- cmd/implement.go:198-272 — `implement goto`: `resolveGotoSlot`, `gotoWithAutoCommit` at 270.
- cmd/workflow_slot.go:30-155 — shared vs lane slots, `orchestratedStart`.
- cmd/autocommit.go:52-269 — `gotoWithAutoCommit`; plan lanes path-scoped commit (190-198, 247-269); implement uses `Targets` + `CommitDirty` (201-216); lane removal (99-104, 231-239).
- cmd/resume.go:30-51 — implement resume uses `templates/steps/resume_implement.md`.
- cmd/epic_worktree.go:20-191 — `worktreeGit` runner var, `worktreeManager`, `runEpicWorktree` result `{spec, project, branch, repos, created}`, `runEpicMerge`, `mergeConflict`; Long text at 33-36 describes the overlay.
- cmd/status.go:67, 175-248 — status options, `statusRunSource`; root description "the project, or the spec's project worktree".
- cmd/repo.go:330-410 — `repo list` builds `repo.New(cfg, projectRoot())`.
- cmd/storefile.go:141, cmd/knowledge.go:288-315 — repo-routed changelog store and knowledge sources via `repo.New`.
- internal/repo/set.go:65-127 — `New` applies overlay; `OverlayFile`, `Overlay`, `readOverlay`.
- internal/repo/set_test.go:696-715 — `writeOverlay`, `TestNew_OverlayRelocatesMappedReposOnly`.
- internal/worktree/worktree.go:44-62 — `RepoWorktree`, `SpecWorktrees`.
- internal/worktree/worktree.go:118-143 — `TouchedRepos` reads plan `**Repo:**` lines.
- internal/worktree/worktree.go:154-219 — `projectTop`, `checkouts`.
- internal/worktree/worktree.go:244-339 — `Ensure`, `ensureOne`.
- internal/worktree/worktree.go:346-393 — `excludeFromProject`.
- internal/worktree/worktree.go:431-500 — `List`/`Find`.
- internal/worktree/worktree.go:509-628 — `Merge`, `dirtyOverlap`; removal uses `--force` because of the overlay (580-583) and `os.Remove(worktreeRoot)` (589).
- internal/worktree/worktree_test.go:38-182, 377-480 — real-git fixtures (`newFixture`, `manager`, `requireRefusal`, `commitAll`) and Merge tests.
- cmd/epic_worktree_test.go:70-273 — `worktreeProject`, `runEpicWorktreeCmd`, `TestEpicWorktree_ReposResolveIntoWorktreesFromInside` (152-185), merge tests, refusal table.
- internal/status/run.go:275-389 — `isImplDone`, `implementWorkflow`, `implementPart`, `finishedIn`.
- internal/status/report.go:146-154, 449-480 — `opts.lane`, orchestrated rendering.
- internal/workflow/lane.go:15-66 — lane paths.
- internal/workflow/workflow.go:17-36 — `workflow.Config` fields.
- internal/stepkit/stepkit.go:83-172 — `WriteStepResult` variables, lane notes path, appended partials.
- internal/steps/implement/steps.go:57-338 — `writeStep`, callbacks, `taskExtra`, `withDependencyOverride`.
- internal/autocommit/targets.go:27-62, commit.go:47-63, lock.go:15 — targets, CommitAll, commit lock.
- templates/steps/implement/01-read_plan.md:5,47 — "Where the code lives." block and drift-check `repo list`.
- templates/steps/implement/10-update_feature_changelog.md:23-26 — repo list for changelog derivation.
- templates/skills/workflows/spek-implement/SKILL.md:42, 136-143 — cross-repo note and orchestrated section ("from the worktree you were given").
- templates/skills/workflows/spek-implement-epic/SKILL.md:15,17,36,44,51,63,97 — worktree wording and child prompt.
- templates/repo_source_test.go:17-129 — roster ban, repo list consumers, "Where the code lives" pins.
- internal/steps/implement/steps_test.go:44-66, 507-529, 731-797 — render helpers and repo-list pins.
- internal/steps/implement/orchestrated_test.go:15-26 — orchestrated render helper.
- cmd/implement_test.go:19-356 — fixtures and zero-git test.
- cmd/resume_test.go:232-262 — implement resume has no `repo list`.
- cmd/instruction_contract_test.go:78-88, 165-200, 332, 384-389 — contract corpus renders with nothing set; no host project root in static instructions.
- templates/implement_epic_skill_test.go:79-313 — pinned epic skill phrases (240, 313 name the project worktree).
- templates/orchestrated_skill_section_test.go:43-44 — pinned spek-implement orchestrated sentences.
- tests/harbor/implement-workflow/ — no worktrees, no `repo list` oracle; drives update_feature_changelog→finished.
- docs:src/pages/epics.mdx:350-363 — "Implement this epic"; 355-358 describe repos resolving to the spek's own copy.
- docs:src/pages/epics.mdx:418-428 — status JSON example.
- README.md:43 — one-line epic summary (still accurate).

## External references

- git-merge-tree(1), git-diff(1) three-dot form — used for the dry run and the new `.spektacular/` guard.

## Prior plans / specs consulted

- None read. The spec 000064 itself is the source; no prior plan was needed beyond the shipped code.

## Open assumptions

- The project's own repo is always registered (init seeds `{Name: cfg.Name, Location: "."}`, `internal/project/init.go:57-58`), so the project checkout is always among a spec's worktrees.
- `epic worktree` is always run from the main project root (the orchestrator runs it), so the per-spec record lands in the main project.
- A spec with an epic worktree record is implemented only through the epic orchestrator; a standalone `implement new` on such a spec also gets the worktree roots, which is the desired behaviour.
- No in-flight epic runs need migration (spec non-goal).

## Drafting assumptions

### Per-spec worktree record lives in the main project (discovery)
- **Decision**: `epic worktree` writes the spec's repo→code-root map to a record under the main project's `.spektacular/worktrees/<spec>/`, replacing the overlay written into the worktree.
- **Rationale**: keeps code-root resolution in the main project (spec Technical Approach), needs no git at implement time (zero-git test), and the location is already git-excluded.
- **Rejected**: git discovery at implement time (zero-git test); path convention only (needs git top-level).

### Epic implement lanes commit artifacts path-scoped in main, code in worktrees (discovery)
- **Decision**: for an implement run on a spec with worktrees, auto-commit commits code in the spec's worktrees and commits only that spec's artifacts in the main checkouts, under the existing commit lock.
- **Rationale**: mirrors plan lanes' path-scoped commit; avoids sweeping other specs' or the user's work into one commit.
- **Rejected**: CommitAll in main (cross-spec sweep); deferring artifact commits to merge (the model the spec removes).

### Chosen direction: main-project-only CLI with a per-spec worktree record (architecture)
- **Decision**: CLI always runs from the main project; `epic worktree` records the spec's code roots in the main project; implement steps render worktree roots from config; auto-commit splits code (worktrees) from artifacts (main, path-scoped, under lock); status reads main; merge refuses `.spektacular/` diffs.
- **Rationale**: meets every spec constraint with minimal new surface; reuses lanes, plan-lane commit pattern and merge precheck shape.
- **Rejected**: see research.md alternatives.

### Status keeps reporting the worktree as the implement root (architecture)
- **Decision**: `run.implement.root` stays the spec's project worktree when one exists, even though the lane is now in the main project.
- **Rationale**: the orchestrator and users use it to locate the spec's code; spec requires status to report isolated-copy locations.
- **Rejected**: reporting the main root (loses the worktree location).

### Conventions selected (architecture)
- **Decision**: kept store-access, error-remediation, active-install, test-order, tests-pass, and the three docs conventions; dropped glossary-only entries and docs layout/section-background conventions (no layout change).
- **Rationale**: only these bear on the touched surfaces.
- **Rejected**: listing all conventions.

### Worktree manager owns the record (components)
- **Decision**: the record is read and written by the worktree package, not by cmd or repo directly.
- **Rationale**: the manager already owns the worktree layout convention and the merge cleanup that must delete it.
- **Rejected**: a separate package (needless split); cmd-level JSON handling (duplicates layout knowledge).

### Code roots passed via workflow.Config, not workflow data (data_structures)
- **Decision**: runtime-only `Config.CodeRoots`, rendered as `worktree_roots`.
- **Rationale**: Config is not persisted (zero-roster test) and has precedent (`EpicDir`).
- **Rejected**: `SetData` (persisted roster, banned by test); `Extra` per step from cmd (steps have no cmd access).

### New error code for the merge guard (data_structures)
- **Decision**: `epic_merge_touches_spektacular` rather than reusing `worktree_failed`.
- **Rationale**: lets the orchestrator and tests distinguish the guard from operational failures.
- **Rejected**: `worktree_failed` (too generic).

### Milestone ordering keeps each one shippable (milestones)
- **Decision**: M1 is additive (guard + record, overlay still written); M2 switches children to the main root and removes the overlay in the same milestone; M3 is docs.
- **Rationale**: removing the overlay before implement reads the record would break epic runs between milestones.
- **Rejected**: removing the overlay in M1.

### Record file name and location (tasks)
- **Decision**: `<project>/.spektacular/worktrees/<spec>/record.json`.
- **Rationale**: already git-excluded, outside every worktree, removed with the spec's worktree root.
- **Rejected**: a top-level `.spektacular/worktrees.json` (shared file, concurrent writers).

### Status task depends on nothing (tasks)
- **Decision**: status change has no task dependency.
- **Rationale**: it only changes where status reads; it is valid even before children move.
- **Rejected**: chaining it behind the overlay removal.

## Rehydration cues

- `spektacular spec file read 000064_epic-worktree-store-isolation`
- `spektacular plan file read 000064_epic-worktree-store-isolation plan` (and `context`, `research`)
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/working-with-files-from-steps.md"}'`
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`
- Re-read: internal/worktree/worktree.go, internal/repo/set.go:65-127, cmd/autocommit.go:52-269, internal/status/run.go:275-389, templates/steps/implement/01-read_plan.md, templates/skills/workflows/spek-implement-epic/SKILL.md.
