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
