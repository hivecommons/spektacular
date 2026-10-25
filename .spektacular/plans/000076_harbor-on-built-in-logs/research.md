---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Research: 000076_harbor-on-built-in-logs

## Alternatives considered and rejected

### Option A: Harbor reads the JSON Lines logs itself (a Python log reader replacing `sessionlog.py`)

**Rejected**: Rejected: the spec's Constraints require Harbor to read Spektacular's logs through the run statistics report of 000075, and a second reader would recreate the "one parser per format" problem the spec removes. It would also duplicate span pairing, capping and interval rules that `internal/runstats` (000075 plan, Architecture) defines once.

### Option B: Keep `SPEKTACULAR_SESSION_LOG=verbose` in the suite images so the verifiers keep reading responses and state snapshots

**Rejected**: Rejected: the spec's Constraints say every measure must come from logs recorded at the default level with no special setting, and its Technical Approach says to drop the session-log override. At the default level a record holds argv, times, exit status and message, agent, workflow kind and name, `spektacular.step` and `spektacular.step.to`, but no response or state (000073 plan, Data Structures).

### Option C: Get the epic orchestrator's hand-off gaps from its `sub-agent` spans only

**Rejected**: Rejected as the sole source: matching an orchestrator span to the child it launched has no recorded link (000074 plan, Out of Scope: "Linking an epic orchestrator's sub-agent span to the child's own workflow run"), and spans depend on agents marking them (000074's 90% target). The timeline keeps today's command-based definitions (`tests/harbor/shared/timeline.py:1-31`), read from the orchestrator's standalone command records, so figures stay comparable with earlier runs.

### Option D: Fold command listings into `stats run` (a `--commands` flag) and standalone spans into `stats list`

**Rejected**: Rejected: it would change two output shapes 000075 pins with golden tests and 000077 reuses. Two new read-only listing subcommands add surface without touching existing contracts.

### Option E: Keep per-step output characters in the metrics record by joining transcript messages to the new step entries

**Rejected**: Rejected: it is a transcript-based step measure (spec Requirement "The old parsers are removed"), is unknown for Codex and split-lane runs (agent-specific, against the success metric "no agent-specific fields"), and keeping it would retain `claude_messages`, `atif_messages`, `goto_step_of` and `claude_launch_root` (`tests/harbor/shared/transcripts.py:253-425,694-709`).

### Option F: Run the span-judged delegation checks for every agent

**Rejected**: Rejected: the spec's Non-Goals exclude changing any suite's pass and fail checks beyond where they read their data. The checks stay in the Claude-gated `TestChildren` classes (`tests/harbor/shared/tests/test_epic_children.py:393-407` pins that).

### Option G: Feed the Python tests from the real binary only

**Rejected**: Rejected as the only source: `pytest tests/harbor` must pass without Docker or an installed new binary. Unit tests use canned `stats` JSON, taken where possible from 000075's golden files; one integration module builds the binary from the working tree into a temp dir and skips when Go is unavailable.

## Chosen approach — evidence

- `tests/harbor/shared/sessionlog.py:87-92,151-228` — `step_after`, `Lane.resumes`, `Lane.stops` and `step_windows` need `state_after`, `state_before` and `response`; at the default level those are gone, so the reader must be replaced, not patched.
- `tests/harbor/shared/delegation.py:106-120` — `worktree_paths` parses `epic worktree` responses; responses are verbose-only. `internal/worktree/worktree.go:37,277-280,339,363-367` — every worktree path is `<root>/.spektacular/worktrees/<spec>/<repo or project>`, so the brief check can use the deterministic base folder.
- `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py:381-394` — `test_goal_record_precedes_update_plan` uses `state_before.current_step`; the default-level `spektacular.step` attribute (step at command start) carries the same fact.
- `tests/harbor/epic-workflow/tests/test_epic_workflow.py:110-164`, `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py:210-283`, `tests/harbor/goal-check-workflow/tests/test_goal_check.py:106-129,286-292`, `tests/harbor/plan-workflow/tests/test_plan_workflow.py:1369-1386`, `tests/harbor/implement-workflow/tests/test_implement_workflow.py:328-358`, `tests/harbor/standing-rules/tests/test_standing_rules.py:178-184` — every other verifier rule uses argv, exit code, times and the step only: all available from default-level command records.
- 000075 plan, Data Structures — `stats run` gives steps (`duration_ms`, `helper_wait_ms`, `user_wait_ms`, `entries`, `incomplete`), `step_entries`, `tasks`, `helpers` (with `step`, `task_id`, `end_reason`, `complete`), `user_waits`, `gaps`, totals; `stats list` gives run summaries; `--dir` works on a copied directory; every success response carries `log_dir` and `skipped_lines`. 000075 Out of Scope: standalone traces are not reported, "If 000076 needs orchestrator hand-off gaps from them, it extends the report."
- 000075 context, Task "Load logs into runs" — `Assemble` groups by trace and keeps only traces with `spektacular.workflow.kind`; command records are already kept on `Run.Commands`, so a command listing needs only an exported view plus a standalone bucket.
- 000075 plan — the `stats` group carries the logging-exempt annotation inherited by subcommands, so new subcommands never write to the logs they read.
- `Makefile:108,115` and every suite Dockerfile (`COPY spektacular /usr/local/bin/spektacular`) — the image's binary is built from the working tree, so `stats` (and this plan's additions) is available to the verifier, which runs in the agent's container (no `environment_mode` in any `task.toml`). `tests/harbor/background-child` has no binary: its metrics must report log measures as unknown.
- Every suite `task.toml` except background-child collects `/app/.spektacular` → `artifacts/spektacular`, so `.spektacular/logs/` already reaches the trial artifacts once 000073 writes there.
- `tests/harbor/shared/tests/test_suite_wiring.py:151-206` — the Makefile copies `tests/harbor/shared/*.py` non-recursively, so a new shared module ships automatically and a removed one stops shipping.
- `tests/harbor/shared/metrics.py:624-674` — the `gather` pattern (each section on its own, `Unknown` on failure, always exit 0) is kept; it already satisfies "Measuring never fails a run".
- `docs/workflow-performance-investigation.md:286,405-406` — `idle_s` as the longest gap hides small waits; the recommendation is total idle and a per-step split, which the spec adopts.

## Files examined

- `spektacular:tests/harbor/shared/sessionlog.py:1-378` — debug-log reader; lanes, segments, step windows, resumes, stops, epic polls, reactions, merges, `plan_steps_entered`, `changelog_writes_by_repo`.
- `spektacular:tests/harbor/shared/timeline.py:1-178` — epic timeline: lanes, slowest step (longest gap), spawned/picked-up/merged gaps, resumed children, Bob turn-limit count.
- `spektacular:tests/harbor/shared/metrics.py:1-728` — record schema 2; `step_breakdown` 277-346, `place_children` 397-459, step child counts 462-523, idle 537-557, wall fallback 597-607, `build_record` 624-674.
- `spektacular:tests/harbor/shared/transcripts.py:1-813` — totals readers (286-334, 648-691, 773-813), sub-agent counts (373-403, 712-724), children (471-582, 741-770), messages (348-370, 694-709), turns and wake-ups (585-645), launches (85-132, 406-425).
- `spektacular:tests/harbor/shared/delegation.py:1-302` — oracle (`REQUIRED`, `SUGGESTED`, `PER_SPEC_CHILD`), `read_run`, `worktree_paths`, `missing_required`, `missing_spec_children`, brief checks, wake-up and foreground rules.
- `spektacular:tests/harbor/shared/report.py:1-600` — run table, breakdown, children table, prompts, baseline/candidate medians (`MEASURES` 458-465), thresholds 33-36.
- `spektacular:tests/harbor/shared/tests/*` — test_metrics (key pins 38-75), test_reference_runs (pinned idle and placement values 79-158, event-key limits 253-262), test_sessionlog_rules, test_sessionlog_windows, test_delegation, test_epic_children (AST guard 393-407), test_suite_wiring, test_readme_metrics, trim_reference.
- `spektacular:tests/harbor/shared/tests/data/{m70-epic,s55-epic,s55-plan,s55-implement}/debug/` — old-format debug logs; `probe-background-child/` has none.
- `spektacular:tests/harbor/*/tests/test.sh` — pytest, reward, `python3 /tests/metrics.py || true`; timeline at epic:5, goal-check:5, implement-epic:6, standing-rules:5.
- `spektacular:tests/harbor/*/environment/Dockerfile` — `ENV SPEKTACULAR_SESSION_LOG=on` in nine images (000073 makes it `verbose`).
- `spektacular:tests/harbor/background-child/tests/test_background_child.py:20-120` — uses `claude_children` ending, duration, launch and end times; no binary in its image.
- `spektacular:Makefile:24-39,89-90,100-123` — agent artifacts, build info, `harbor-report`, render targets.
- `spektacular:README.md:469-625` — Harbor test results, session log, run metrics field table, comparing runs, self-checks.
- `spektacular:internal/worktree/worktree.go:37,277-284,335-394` — worktree folder layout.
- `spektacular:docs/workflow-performance-investigation.md:270-300,395-410` — idle and per-step split recommendations.

## External references

- None. The work builds on the epic's planned contracts and the existing harness.

## Prior plans / specs consulted

- `000073_log-mode-and-logger-providers` (plan, context) — record format, default vs verbose attributes, run files and `standalone_<day>.jsonl`, retention 10, `SPEKTACULAR_SESSION_LOG` levels, its Harbor normalising reader and `verbose` images that this plan replaces.
- `000074_trace-spans-for-agent-work` (plan, context) — span records, end reasons, parents, agent spans carry `spektacular.step`; orchestrator spans are standalone; its Harbor task adds `shared/spans.py`, trace oracle constants, `unmarked_launches` (launch-to-span match within 120 s), verifier span checks, `TestUserWaits`, and `trace_calls` in metrics.
- `000075_run-stats-command` (plan, context) — `stats list|run|compare|summary`, JSON contract and golden files, `internal/runstats` API (`Load`, `Assemble`, `Run.Commands`, `Span`), logging exemption, standalone traces out of scope.
- Spec `000076_harbor-on-built-in-logs` — requirements, constraints, acceptance criteria, success metrics.

## Open assumptions

- 000073-000075 land as planned. If only names differ, adapt; if a field meaning, the default-level attribute set, or the `stats` JSON contract differs, STOP and ask.
- A lane resume (`<kind> new` on a lane already in progress) is recorded as a failed command in that lane's run at the default level, carrying `spektacular.step`. If it is not, resumes are counted from step entries ending `interrupted` instead; if neither shows them, STOP and ask.
- The epic orchestrator's commands (status polls, `epic` commands) are standalone records with `process.command_args` at the default level.
- Default retention (10 runs) holds every run of one Harbor trial; no suite starts more than 10 workflow runs.
- Agents mark helper launches and user waits as 000074 instructs; checks and measures judged from spans fail or report zero helpers when they do not.

## Drafting assumptions

### Chosen direction: Harbor reads the logs only through an extended `stats` (architecture)
- **Decision**: one shared module, `runlog.py`, runs `spektacular stats list|run|commands|spans --dir <logs> --format json` and is Harbor's only route to Spektacular's logs; metrics, timeline, delegation and every verifier rule use it. `sessionlog.py` and 000074's `spans.py` are deleted, and `transcripts.py` is cut to tokens, models, cost, turns, child briefs and launches. Images stop setting `SPEKTACULAR_SESSION_LOG`.
- **Rationale**: it meets the constraints (logs through the run statistics report, default level only) and keeps one Go definition of every figure, pinned by golden files.
- **Rejected**: a Python log reader; keeping verbose logging; changing the `stats run`/`stats list` shapes (see research.md alternatives).

### Extend `spektacular stats` with two listings (discovery)
- **Decision**: add `stats commands` (every command record, in runs and standalone, with argv, step, step_to, times, exit code and message) and `stats spans` (every agent-marked span, in runs and standalone) to the 000075 command group and `internal/runstats`.
- **Rationale**: the spec requires Harbor to read the logs through the run statistics report; the verifier rules need argv and steps per command, and the epic timeline needs the orchestrator's standalone commands, neither of which 000075 reports. 000075's plan says 000076 extends the report for standalone traces.
- **Rejected**: a Python log reader (contradicts the constraint); changing `stats run`/`stats list` shapes (pinned by 000075 golden tests and reused by 000077).

### Epic hand-off gaps keep today's command-based definitions (discovery)
- **Decision**: spawned-after, picked-up-after and merged-after are computed from the orchestrator's standalone command records (status polls, `epic` commands, commands naming another spec, `epic merge`), as `timeline.py` defines them today.
- **Rationale**: keeps figures comparable with earlier runs and does not depend on agents marking spans.
- **Rejected**: orchestrator `sub-agent` spans alone (no recorded link to the child's run; marking is not guaranteed).

### Brief check uses the deterministic worktree folder (discovery)
- **Decision**: the implement-epic brief check requires an implementing child's prompt to name `<root>/.spektacular/worktrees/<spec>`, a hand-kept constant tied by a self-check to `internal/worktree`'s `Dir`, instead of the repo paths parsed from `epic worktree` responses.
- **Rationale**: responses are not recorded at the default level and the worktrees are removed by `epic merge`; the folder is a prefix of every repo worktree path, so any prompt that passed before still passes.
- **Rejected**: keeping verbose logging (contradicts the constraint); re-running `epic worktree` in the verifier (changes state).

### Drop per-step output characters and transcript sub-agent counts (discovery)
- **Decision**: the metrics record no longer carries `steps[].output`, `steps.orchestrator_output`, `steps.unit`, or transcript-derived `subagents.started/completed/failed/stopped/durations_s`; helper counts and durations come from spans. Child prompts and models stay, as transcript-sourced briefs.
- **Rationale**: spec "Measures come from one source" and "no agent-specific fields"; tokens, models and cost stay unchanged.
- **Rejected**: joining transcript messages to log step entries (keeps a transcript-based step measure).

### Span-judged delegation checks keep the Claude-only gating (discovery)
- **Decision**: required-child and per-spec-child checks judge from spans but stay inside the Claude-gated `TestChildren` classes.
- **Rationale**: spec Non-Goals exclude changing pass and fail checks beyond where they read their data.
- **Rejected**: running them for Codex and Bob too.

### A step's split adds up to its duration (architecture)
- **Decision**: per step, `working_s = max(0, duration - helper wait - user wait)`, beside `helper_wait_s` and `user_wait_s` taken from `stats run`. Run-level totals report user wait, helper wait and gap time, not a run-level "working" figure.
- **Rationale**: the spec asks for a split; 000075's run-level `working_ms` (wall less user wait) includes helper wait, so reusing that name for a run total would mean something different from the per-step figure.
- **Rejected**: per-step working as duration less user wait only (not a split).

### Gap time uses `stats`' default threshold (architecture)
- **Decision**: totals carry `gap_s` and `gaps` for gaps over 5 minutes, the `stats` default, and record `gap_threshold_s`.
- **Rationale**: a maintainer running `stats run` on the same artifacts sees the same figure; no Harbor-only knob.
- **Rejected**: a shorter Harbor threshold (diverges from the report it must match).

### Trial totals sum across runs (architecture)
- **Decision**: in a trial with several workflow runs (epic suites), the trial-level `user_wait_s`, `helper_wait_s` and `gap_s` are sums over the runs; each run's own figures sit beside its steps.
- **Rationale**: defined and comparable across suites; single-run trials equal the run's figures.
- **Rejected**: leaving trial totals unknown for multi-run trials.

### Conventions selected (architecture)
- **Decision**: apply error-messages-must-suggest-remediation, tests-must-not-depend-on-order, tests-must-pass-for-done, plans-never-change-the-active-install, store-files-must-be-written-through-the-cli, and the docs repo's plan-content-pages, no-em-dashes and mdx-authoring. Drop site-layout, alternate-section-background and file-scoped-section-headings.
- **Rationale**: the work adds Go subcommands, Python tests, knowledge updates and a small addition to an existing docs page; no new page layout or section is introduced.
- **Rejected**: listing every convention.

### Listings take an `--outside-runs` filter and the run selection (components)
- **Decision**: `stats commands` and `stats spans` list everything by default; the selection flags narrow to matching runs' records, and `--outside-runs` keeps only records written outside any workflow run (the orchestrator's).
- **Rationale**: Harbor needs both views of one trial; mirroring 000075's selection keeps one mental model.
- **Rejected**: a separate `stats standalone` subcommand (a third new surface for the same records).

### The run log reader caches per instance (components)
- **Decision**: each `stats` question is asked once per reader instance; verifiers build one reader per module (pytest fixture scope).
- **Rationale**: a verifier asks many questions of the same finished logs; subprocess calls cost about a few tens of milliseconds each.
- **Rejected**: an on-disk cache (would add files beside the logs).

### Metrics schema bumps to 3, durations stay in seconds (data_structures)
- **Decision**: `metrics.json` moves to schema 3 and keeps seconds (`*_s`, three decimals), converting from `stats`' milliseconds; `report.py` still loads schema 1 and 2 records and shows the new figures as unknown.
- **Rationale**: the record's existing units and readers stay consistent; old jobs remain reportable.
- **Rejected**: switching the record to milliseconds (breaks every existing reader of totals).

### Orchestrator helpers get a spec by time, never a step (data_structures)
- **Decision**: a `sub-agent` span written outside any run takes as its `spec` the lane whose first run started while the span was open (else `null`), and `step: null` with `outside_run: true`.
- **Rationale**: the spec asks for each helper's spec and step; the orchestrator runs no workflow step, and the child's run start is the only recorded link.
- **Rejected**: leaving every orchestrator helper's spec null (loses the per-spec count the epic suites need).

### 000074's trace-call count is kept (data_structures)
- **Decision**: `totals.trace_calls` (and its per-task average from 000074) stays transcript-counted; its per-task divisor now comes from `stats run` task rows.
- **Rationale**: it measures agent tool calls, which the logs cannot see.
- **Rejected**: counting `trace` command records instead (misses calls that failed before reaching the CLI).

### The line-count metric is enforced by a self-test (testing_approach)
- **Decision**: a shared self-test requires the shared Python modules (not their tests) to total 2,000 lines or fewer.
- **Rationale**: the metric is mechanically checkable; enforcing it stops the harness regrowing.
- **Rejected**: a manual count only (drifts silently after the change).

### Baseline pass rates are collected before the change, by a person (testing_approach)
- **Decision**: a human task collects 5 Claude runs per suite on the harness as 000073-000075 leave it, before this plan's Harbor changes merge.
- **Rationale**: the success metric compares before and after; the before runs cannot be produced afterwards and need paid logins.
- **Rejected**: comparing against old jobs from before 000073 (a different harness and CLI).

### Task breakdown and sequencing (tasks)
- **Decision**: 14 tasks: Go listings, CLI and docs first; a person-run baseline and a token characterisation test before any Harbor change; then the reader, recorder, delegation, verifiers and images; then the timeline, report, removals and docs. Old modules are deleted only after every consumer has moved.
- **Rationale**: each task stays reviewable and the harness keeps working between tasks; the baseline must predate the Harbor change.
- **Rejected**: one big Harbor rewrite task (unreviewable, no green points in between).

### `metrics.py` takes `--log-dir`, keeping `--debug-dir` as an alias (tasks)
- **Decision**: the recorder's directory flag becomes `--log-dir`, with `--debug-dir` accepted as a hidden alias.
- **Rationale**: the name now matches the logs; old hand-run commands keep working.
- **Rejected**: keeping only `--debug-dir` (misleading once debug logs are gone).

### Transcript checks the logs cannot see stay on transcripts (verification)
- **Decision**: besides tokens, models, cost and the brief checks, `transcripts.py` keeps the turn, wake-up and child-background readers (the wake-up and foreground delegation rules, the background-child probe) and the launch and main-model readers (goal-check's cheaper-reviewer check).
- **Rationale**: Spektacular cannot see turns, wake-ups, a child's background state or its model, so these stay with "what Spektacular cannot see" (spec Overview). Dropping them would remove pass and fail checks, which the spec's Non-Goals exclude. The Technical Approach's "keeps only what the transcript-sourced constraint and the delegation-brief checks need" is read as removing every transcript *measure* the logs replace, not as deleting checks the logs cannot replace.
- **Rejected**: deleting those readers and their checks (changes suite pass and fail rules).

## Rehydration cues

- `spektacular spec file read 000076_harbor-on-built-in-logs`
- `spektacular plan file read 000075_run-stats-command plan` (Data Structures: the JSON contract) and `context` (Task "Load logs into runs").
- `spektacular plan file read 000074_trace-spans-for-agent-work context` (Task "Harbor reads and checks span records").
- `spektacular plan file read 000073_log-mode-and-logger-providers plan` (Data Structures: default-level attributes).
- Read `tests/harbor/shared/{sessionlog,metrics,delegation,timeline}.py` and grep `tests/harbor/*/tests/*.py` for `SessionLog`, `DEBUG_DIR`, `dl.`.
- `spektacular knowledge read` for `architecture/testing-architecture.md`, `gotchas/harbor-agent-logs-and-artifacts.md`, `gotchas/harbor-agent-log-fields.md` (repo `spektacular`).

