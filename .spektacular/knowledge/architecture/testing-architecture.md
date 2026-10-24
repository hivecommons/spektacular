---
tags: [testing, harbor, e2e, template, ci]
---

# Testing Architecture: Three Layers and Their Hand-Maintained Couplings

Spektacular's behavior is prose-driven — workflow judgement lives in step templates, not Go — so its test architecture has three layers, each responsible for a different failure class:

1. **Go unit/step tests** (`internal/steps/*/[a-z_]*_test.go`) — deterministic mechanics: FSM step order and wiring, callback behavior, document metadata lifecycle, path helpers. Run with `go test ./...`; always in CI.

2. **Template-contract tests** (`templates/*_test.go` and template-content assertions in `internal/steps/*/steps_test.go`) — the enforcement layer for prose-driven behavior. They assert that rendered step instructions contain (or ban) specific anchor phrases: the context-refresh directive marker, working-file references, approval-gate phrasings, shared instruction blocks. When behavior is expressed as template prose, its regression test is a phrase assertion here. Also `go test ./...`; always in CI.

3. **Harbor E2E suites** (`tests/harbor/plan-workflow`, `tests/harbor/spec-workflow`, `tests/harbor/implement-workflow`, `tests/harbor/repo-workflow`, `tests/harbor/epic-workflow`) — behavioral proof: a real agent drives the full workflow in a Docker container and a pytest verifier asserts against its transcript and artifacts. Run via `make harbor-test-plan` / `make harbor-test-spec` / `make harbor-test-implement` / `make harbor-test-repo` / `make harbor-test-epic`; requires the `harbor` CLI, Docker, and the agent's credentials (taken from the machine's own Claude or Bob login), takes ~15–25 minutes per run, and does **not** run in CI.

   The epic suites' shared checks are agent-independent. They never read a transcript: they assert against CLI state and the CLI's own session log (`SPEKTACULAR_SESSION_LOG=on` is set in every suite's image), so one verifier serves Claude, Codex and Bob (`make harbor-test-epic-codex`, `make harbor-test-epic-bob`). A Claude-only `TestChildren` class in the epic and implement-epic verifiers also reads the transcript, and skips for other agents. It checks each spec's own child, the context it was given, required test/verify/goal-check children, and that no wake-up was set while children ran. It prints a timeline of the run from that log (`tests/harbor/epic-workflow/tests/timeline.py`) before the tests. The plan suite's transcript assertions also read the trajectory Harbor records for an ACP agent such as Bob (`make harbor-test-plan-bob`); the spec, implement and repo suites' understand only Claude and Codex transcripts.

The harbor layer is deliberately built on **hand-maintained, independent oracles** — deriving them from the templates at runtime would make the tests tautological. These oracles are couplings to product surfaces and must be updated in the same change as the surface they mirror:

- `EXPECTED_STEP_ORDER`, `EXPECTED_SKILLS_PER_STEP` — mirror `internal/steps/*/steps.go` and template skill references. The delegation oracle (`REQUIRED`, `SUGGESTED`, `PER_SPEC_CHILD` in `tests/harbor/shared/delegation.py`) mirrors which step templates require a child. A local self-check ties it to the templates' "does not complete this step" / "never as yourself" wording, so the two cannot drift apart silently
- Command-substring oracles (e.g. `CONVENTIONS_READ_COMMAND`) — mirror CLI commands named in templates
- `SCAFFOLD_LEFTOVERS` literals — mirror `templates/scaffold/*.md` placeholder slots (keep literals distinctive; generic ones false-positive on legitimate prose)
- Seeded environment fixtures (`environment/Dockerfile`, seeded spec/convention files) — must satisfy current store contracts (e.g. spec filenames need a valid `spec.id_method` ID prefix)
- `solution/solve.sh` goto sequence and `task.toml` timeouts — mirror the step table and realistic run length
- The epic suite's fixture is written through the CLI at image build (`environment/seed/seed.sh`), which fails the build if the epic does not start with two specs ready and one blocked; its session-log reader (`tests/sessionlog.py`) mirrors the log's file naming (`<started>_<agent>_<kind>_<name>.jsonl`) and the plan workflow's terminal step name `finished`; its summary checks mirror `epic summary`'s `## Decisions` and `## <spec>` sections

**Consequence for planning and implementing:** any change to workflow steps, step templates, scaffolds, CLI command names, or store validation contracts includes the matching surfaces of every harbor suite it touches — spec, plan, implement, repo and epic alike — in its scope, and its verification includes a harbor run of each. A change to the epic skills, `status`'s run view, the session log's format or how it files lanes includes the epic suite. Because the suites don't run in CI, skipping this doesn't fail anything at the time — drift accumulates invisibly and is paid for by whoever runs the suite next (plan 000040's E2E validation spent four ~25-minute runs clearing four such accumulated drifts).

