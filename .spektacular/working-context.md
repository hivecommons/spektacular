# Working context — implement 000062_epic-plan-and-implement

## Origin
- Implement workflow started 2026-10-04 for `000062_epic-plan-and-implement` (user picked it).
- Repo roots: spektacular → /home/nicj/code/github.com/hivecommons/spektacular; docs → /home/nicj/code/github.com/hivecommons/spektacular-website.
- Binding design: `design:epics-and-seeded-specs.md`.

## Decisions / answers
- Task 1 (Restore a compiling build): duplicate installerFor removed from cmd/migrate.go; build compiles.
- Task 1 test step: no new tests (context.md says the existing suite is the test).
- Task 1 verify: go test -shuffle=on . ./cmd/... ./internal/... ./templates/... all green (27 pkgs). Use this package list; `go list ./...` fails on tests/harbor perms.
- Task 1 ticked in plan.md (heading + criterion).
- Task 1 changelog entry written. USER SAID: run without asking — loop between tasks automatically (still STOP on genuine mismatches/failed verification).
- read_plan: structure valid (10 sections, 13 tasks across 4 milestones, all context links resolve).
- Drift: none material. `internal/autocommit/git_test.go` is a new file to create; minor line shifts (e.g. workflowDescriptions at commands.go:18).
- Spec coverage: all requirements and acceptance criteria covered; no descoped items.
- Changelog mode: first-task (no `## Changelog` in plan yet).

## Learnings
- `go build ./...` trips on permission-denied tests/harbor/jobs; build named packages (`go build ./cmd/... ./internal/...`).
- Never re-init/migrate this repo or `go run .` to drive a workflow; verify with go test + throwaway projects.
- Task 2 (lanes) implemented: lane path helpers placed in internal/workflow/lane.go now (LaneStatePath/LaneNotesPath/LaneStateRel/LaneNotesRel/ReadLane/LaneNames) instead of moving them there in task 6; cmd/workflow_slot.go has workflowSlot, resolveGotoSlot, workflowNotFound, refuseLaneInProgress, orchestratedStart.
- goto routing: no name → shared; lane exists → lane; shared same kind+name → shared; shared other kind → shared (guardKind reports as before); else workflow_not_found.
- Standalone new refused (workflow_in_progress) while a lane for that spec is in progress, even with --force; next_action offers lane goto or `new --force` with orchestrated.
- Task 2 done: verified green, ticked, changelog entry written. Lane-unaware instructions are tasks 3/4 scope.
- Task 3 implemented: 31 template gotos carry "name":"{{plan_name}}"; plan/implement scratch paths → .spektacular/tmp/{{plan_name}}/...; autocommit.MessageTmpPath(name) shared by stepkit + cmd (per-name commit msg for ALL kinds incl. spec; name in goto only for plan/implement via commit.goto_name); Workflow.gotoData adds name to engine hints for plan/implement; resume templates get goto_name, notes_path (lane notes when orchestrated) and orchestrated --force data; resumeInstruction gained an `orchestrated bool` param.
- autocommit has git_integration_test.go (not git_test.go) — use it for CommitPaths tests in task 5.
- Task 3 done: verified green (27 pkgs), ticked, changelog written.
- Task 4 implemented: stepkit exposes `orchestrated` + `working_context_path` (lane notes when orchestrated) vars; footer uses {{working_context_path}}; new partials/orchestrated-stop.md auto-appended by stepkit to every step of an orchestrated workflow (DEVIATION: one appended section instead of including a partial at each STOP); walkthrough/finished (plan), 07 loop + 12-finished (implement), resume prompts have {{#orchestrated}} branches; standalone renders verified byte-identical.
- Task 4 done: verified green, ticked, changelog written (incl. fixes for hardcoded working-context in 13-assemble + implement-plan-documents partial w/ fallback, step 2b orchestrated, task-run DONE).
- Task 5 implemented: autocommit.Git.CommitPaths (filters paths to existing-or-tracked; add -A -- paths; skip if nothing staged; commit -F - --only -- paths); autocommit/lock.go AcquireLock(dataDir) at .spektacular/workflows/.commit.lock (30s timeout, 10m stale); cmd/autocommit.go: orchestrated lanes removed on reaching finished (both no-commit and commit paths, before commit), plan lane commits via commitPlanLane (plan store dir, work/<name>, tmp/<name>, lane json+md) under lock; restore also restores lane notes.
- Existing test TestOrchestratedPlanFinishesWithoutSignOff expects lane at finished — must change to expect lane removed.
- Task 5 done: fixed CommitPaths retry bug (HEAD-only paths) found by tests; verified green; ticked + changelog.
- Task 6 implemented: status.Options.Lane func(kind,name)*State (wired in cmd/status.go only; dependency-check call sites leave it nil); currentStep(opts,...) checks shared then lane; matchingWorkflow falls back to the first in-progress lane (WorkflowInfo.Orchestrated). Session log: snapshotStatePath(argv) picks lane for orchestrated `new` or `goto` naming a spec with a lane (skips values of --data/-d/--fields/--stdin/--file); finished lane (stateAfter nil) files under stateBefore's session id.
- Task 6 done: verified green, ticked + changelog. Milestone 1 complete → milestone commit due on next advance.
