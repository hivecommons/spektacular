# Working context — implement 000063_epic-planning-summary-and-reordering

## Origin (carried from the spec/plan sessions)
- Came from the user's xcl `references-and-secrets` epic: two specs both changed `configuration-text.mdx` / `encode.go` unordered; CHANGELOG handling disagreed; plans contradicted a user-chosen interface and a knowledge entry; end-of-run review was dense.
- User decisions: summary doc lives in epic store via `spektacular epic ...`; overlap deps written without asking; earlier-listed spec goes first; no re-plan on added dep; new contradiction questions apply to all planning.

## Implement session
- User chose 000063 to implement (2026-10-04).
- read_plan: structure OK, no drift found (all paths/symbols exist), every spec item covered. First-task invocation (no ## Changelog yet).
- Repo roots: spektacular = /home/nicj/code/github.com/hivecommons/spektacular, docs = /home/nicj/code/github.com/hivecommons/spektacular-website.
- Task 1 analysis: step templates already include partials (`{{> partials/...}}` via stepkit.FSPartials); new partial must avoid "orchestrat", "QUESTION:", "FAILED:", "DONE:", ".spektacular/workflows/" (cmd/orchestrated_test.go TestStandaloneStepsCarryNoOrchestratorHandBack).
- Task 1 implemented: `templates/partials/proceed-unless-blocked.md` (anchor phrase "contradict a decision the user recorded"), included in the 11 gathering steps.
- Env gotcha: `go test ./...` fails on a root-owned dir under gitignored `tests/harbor/jobs/` (permission denied). Run packages explicitly: `go test $(find . -name '*.go' -not -path './tests/harbor/jobs/*' -printf '%h\n' | sort -u)`.
- Task 1 tests: TestGatheringStepsStopOnContradictingRecordedDecision (steps_test.go), TestOrchestratedGatingStepHandsBackContradictionStop (orchestrated_test.go).
- Task 1 verified green. Local golangci-lint panics (built with go1.26, local go1.27); Makefile lint is just go vet — use go vet on explicit package trees.
- User said 'keep goin' after task 1: treating as continue without pausing between tasks. Knowledge offer (partial wording gotcha) not answered = deferred.
- Task 2 implemented: discovery 'Recorded decisions' paragraph + clarify bullet; architecture Step 2 check; tasks 'never a `human` task'; open-questions example; verification 'Recorded decisions and knowledge' quality bullet; spek-plan SKILL '# Recorded decisions and knowledge' section.
- Verify helper: scratchpad/verify.sh (build/vet/test on ./ ./cmd/... ./internal/... ./templates/...; dagger is a separate module). Tick helper: scratchpad/tick.py <file> <title>.
- Milestone 1 committed (0797f45). Milestone-closing loop goto needs commit_message_from.
- Task 3 implemented: depgraph.Reaches (iterative DFS); plantask/files.go TaskFiles + FileRef (skips fenced code, dir tokens ending '/', Go selectors via lower-case ext rule, unregistered prefix kept in path); 10-tasks.md File changes bullet requires backticked path first.
- Task 3 done (Reaches, TaskFiles). Next task 4 epic summary. Read-ahead: docTxn.remember on a dir errors; delete summary.md via t.delete then st.Delete(folder) last, outside txn.
- Task 4 implemented: internal/epic/summary.go (ParseSummary/Render/SetSection/SectionNames/ValidSectionBody; 'None.'/'None added.' normalised to empty on parse), cmd/epic_summary.go (summaryDir/summaryPath/readSummary/writeSummary/appendOrdering(t,cfg,epic,order,lines)), runEpicDelete deletes summary first in txn then best-effort st.Delete(folder). Pinned 'run one of: ...' verb lists in cmd tests updated (will need 'order' added in task 5).
- Task 4 done and verified.
- Task 5 implemented: EpicSpec.ParallelWith (+Render, Validate non-member/self, schema), cmd/epic_order.go (orderEpic/unorderEpic/writeOrderedEpic/planFiles/sharedFiles). Verb lists now include 'order'. Open question checked: TaskFiles finds files for nearly every task in pre-change plans 000060/000062 (legacy 000058 has no tasks); minor noise only (`.ext`, bare `interview.md`, dir `internal/status`) — not material, no STOP.
- Task 5 done; fixed splitGraph dropping split spec's parallel_with (+ regression test).
- Task 6 done. Milestone 2 commit on this advance.
- Task 7 implemented: walkthrough orchestrated point 5 'project-wide rules', finished DONE list, spek-plan SKILL orchestrated bullet.
- Task 7 done.
- Task 8 implemented: spek-plan-epic SKILL store rule (epic summary), DONE contract + rules, widened genuine question, Step 4 DONE writes section, new Step 6 order+decisions, Step 7 review walks summary, Step 8 report adds dependencies count. Test section names renumbered (Step 7 review, Step 8 report). Avoided new 'ask' words so the allowlist is unchanged.
- Task 8 done. Milestone 3 commit on this advance. Next: task 9 docs repo.
