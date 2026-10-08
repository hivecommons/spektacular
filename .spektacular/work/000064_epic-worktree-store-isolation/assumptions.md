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
