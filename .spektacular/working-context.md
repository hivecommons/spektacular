# Working context — plan 000062_epic-plan-and-implement

## Origin
- Plan workflow started 2026-10-04 for spec `000062_epic-plan-and-implement` (user picked it).
- Binding design: `design:epics-and-seeded-specs.md` (epic never planned/implemented itself; plans 1:1 with specs; deps constrain implementation only; status derives all state).

## Decisions / answers
- Discovery done; research in .spektacular/work/000062_epic-plan-and-implement/research.md.
- Direction: opt-in per-name state files for epic-run children (name on goto); standalone runs unchanged on state.json.
- First task fixes duplicate `installerFor` in cmd/migrate.go (HEAD does not compile).

## Learnings
- state.json, working-context.md, repo-state.json, and a tmp file are git-tracked; `.spektacular/tmp/` is NOT gitignored.
- auto-commit does `git add -A` per work tree; plan tmp files and commit-message path are fixed names -> clash under parallel plans.
- status `planned` counts draft plans; `implemented` flips before implement wrap-up; no read-time cycle check; no topo sort in depgraph.
- docs repo location is outside project tree -> worktree isolation for it is unresolved (open assumption).
- Architecture chosen (option A): spek-plan-epic + spek-implement-epic skills; CLI: orchestrated lanes (.spektacular/workflows/<kind>-<name>.json/.md), name in every plan/implement goto, `epic next`, `epic worktree`/`epic merge`, deferred walkthrough, path-scoped lane commits, exclusive specs for external repos.
- Sections done: research, architecture, conventions, components, data_structures, implementation_detail.
- Tasks drafted (14 tasks, 4 milestones); ids minted via plan task-id.
- Assembled and staged to .spektacular/tmp/{plan,context,research}_template.md.
- All three plan documents written to store; work dir removed. Now at walkthrough.
- Walkthrough: user asked every touched repo gets its own worktree -> replaced 'exclusive' rule with per-repo worktrees + repo overlay (repo.New) + all-or-nothing merge via git merge-tree. All three docs rewritten.
- Walkthrough: user asked to fold epic next into status -> run view in status (internal/status/run.go), no epic next cmd/epicrun pkg; skills read status --format json; problems list w/ blocks:[implement].
- spawn-planning-agents store-access bug filed as issue #70; Out of Scope links it.
- User signed off the plan at walkthrough (2026-10-04).
