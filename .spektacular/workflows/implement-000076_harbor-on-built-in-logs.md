# Implement 000076_harbor-on-built-in-logs: working context

## read_plan (done)
- Structure valid; first-task invocation (no `## Changelog` yet).
- Drift: only line-number shifts and small name differences, which the plan pre-authorises ("if only names differ, adapt"):
  - 000073 added `tests/harbor/shared/loglevel.py` (46 lines) and the verifiers call `set_log_level(config, "off")` before running `spektacular` themselves. This answers the plan's Open Question on verifier CLI calls: they never reach the logs. Keep it.
  - Seeded verbose level lives in: seed.sh `set_log_level verbose` (epic, implement-epic, goal-check), `implement-workflow/environment/config.yaml` (`level: verbose`), plan-workflow and repo-workflow Dockerfiles (printf `level: verbose`).
  - 000074's launch-to-span constants are `LAUNCH_MARK_BEFORE_S = 120`, `LAUNCH_MARK_AFTER_S = 5` and `match_launches`/`unmarked_launches`/`marked_rate` in delegation.py (plan calls it `LAUNCH_SPAN_WINDOW_S`).
  - Shared modules total 3,299 lines now (plan said 2,999); the ceiling stays 2,000.
- Resume rule confirmed in Go: a refused `<kind> new` (workflow_in_progress) is filed in the same run's trace with `spektacular.step` (cmd/log_spans_test.go TestLogSpans_ResumeClosesTheInterruptedStep).
- `process.command_args` excludes the program name (`["plan","new",...]`).

## Decisions during tasks
- Tasks 1-3 (Go listings, CLI, docs) accepted by the goal check.
- "Record baseline Harbor pass rates" is a human task (Docker, Harbor CLI, paid Claude logins). The implement workflow defines no handling for a human task in a full run, so it is passed over (left unchecked) and the agent tasks continue in the worktree. Nothing merges until the orchestrator merges the spec, so the baseline can still be collected from the pre-merge harness first. Must be flagged in the DONE report and the test plan.
- Helper scripts for this run live in the session scratchpad s76/ (stage.sh, tick.py, tickall.sh, clog.sh, goal.py).
- Correction: the CLI's check_goal treats the human task (9ce541c6, Record baseline Harbor pass rates) as the current item, because it is the first unchecked task, even though I already wrote the next agent task's work ("Pin today's token, model and cost figures", 70054b85: PINNED_TOTALS + two tests in tests/harbor/shared/tests/test_reference_runs.py, a docstring note in test_transcripts_summary.py; shared self-tests 542 passed). Handed back a QUESTION at check_goal for the human task; no goal record made for it yet. On the answer: if skip, run `implement goal decide ... decision skip` for 9ce541c6 (if the CLI accepts it without a record), else follow the orchestrator.

## Baseline Harbor pass rates (task 9ce541c6), for the before/after comparison
Answer from the orchestrator (option 2): the baseline exists. Collected 2026-10-25 15:10 to 16:24 by the orchestrator: 50 jobs under the main checkout's `tests/harbor/jobs/`, prefix `base76-`, named `base76-<suite target>-r<1..5>`, one trial each, every trial with `verifier/reward.txt`. Built from a clean detached checkout at commit bfd5dc1b52bfa2392f40826d8c0bc42df4f7491a (000073 to 000075, none of this plan's changes); each trial's `verifier/metrics.json` records that commit. Read with `python3 tests/harbor/shared/report.py 'tests/harbor/jobs/base76-*'` from the main checkout. Compare after the change with `report.py --baseline 'tests/harbor/jobs/base76-*' --candidate <after jobs>`.

Pass counts out of 5 (Claude):
- harbor-test-background-child: 5/5
- harbor-test-epic-claude: 5/5
- harbor-test-implement: 5/5
- harbor-test-standing-rules-claude: 5/5
- harbor-test-goal-check-claude: 4/5
- harbor-test-plan-claude: 3/5
- harbor-test-implement-epic-claude: 2/5
- harbor-test-repo-delegated: 1/5
- harbor-test-repo-one-at-a-time: 0/5
- harbor-test-spec-claude: 0/5 (fails `TestUserWaits::test_constraints_question_is_a_user_wait` and `TestVerificationStep::test_review_was_a_briefed_child` every run)
These counts also go into this task's plan changelog entry and the test plan's before/after comparison.
- Tasks done and accepted so far: Go listings, CLI, docs, baseline (orchestrator-run), pin totals, run log reader, metrics schema 3. Milestone 1 committed by the CLI (bb1fd67 / docs 90e2050).
- Placement helpers (lane_for, window_of, lane_windows, PlacedChild, place_children) left in metrics.py and claude_messages/goto_step_of in transcripts.py only for delegation.py; the delegation task removes the dependency, the removal task deletes them.
- Delegation task plan: delegation rules take children/log/lanes/spans; also update the verifiers' delegation call sites (epic, implement-epic, goal-check, spec) in that task so the self-checks stay green; the rest of the SessionLog uses move in the verifier task.
- Milestone 2 done and committed by the CLI (8d5de3c). Remaining: timeline, report, remove old parsers, README+knowledge, then test_plan etc.
- test_sessionlog_rules.py already deleted (verifier task); parity test lives in tests/harbor/shared/tests/test_runlog_vs_sessionlog.py and goes with sessionlog.py in the removal task.
- Timeline (fc67375e) and report (b3638e0c) accepted.
- Removal task (a49f5e7b) in progress at step `implement`: all planned removals done in the worktree (sessionlog.py, spans.py, test_sessionlog_windows.py, test_spans.py, test_runlog_vs_sessionlog.py, data/*/debug deleted; READMEs note it; trim_reference copies logs/ unchanged; transcripts unused readers removed; reference-run tests updated; integration test for trimming with logs added). Shared suite 627 passed incl. 9 integration. The line-count self-test NOT written: shared/*.py total 2,990 lines (was 3,299 at bfd5dc1), 2,082 code-only lines, against the 2,000 ceiling. Plan Open Question says STOP and ask before loosening the ceiling; handed back a QUESTION.
