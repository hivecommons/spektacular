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
