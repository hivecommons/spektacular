# Working context: implement 000078_token-use-in-run-stats (orchestrated)

## Drift (read_plan) — adapted, names only (plan pre-authorises "if only names differ, adapt")
- No `HelperRow` type: helpers and user waits share `runstats.Wait`. Plan: add a `HelperRow{Wait; Tokens *TokenFigure}`
  (embedded Wait so JSON stays flat, `tokens` appended last) for `Report.Helpers`; user_waits stay `[]Wait` with no tokens.
- Line numbers in context.md are stale (Config struct ~493, Validate ~808, projectConfigExists cmd/root.go:617,
  configuration.mdx "Seventeen top-level keys" line 111, `log` ConfigKey line 206). 000073 has landed (schema 5, `log:` key).
- Docs repo worktree is `.spektacular/worktrees/000078_token-use-in-run-stats/docs` (context names spektacular-website).
- 000076 golden files (commands/spans) exist; Harbor Python tests (tests/harbor/shared/tests) read Go goldens, rerun them
  after regenerating: `uv run --with pytest pytest tests/harbor/shared/tests`.

## Decisions
- Changelog mode: first-task (no `## Changelog` in plan).
- Task 1 (session reader) done, goal accepted. Helper scripts in my scratchpad 078/ (stage.sh, tick.py, cl.py) — scratchpad is shared
  with other agents, so only use the 078/ subdirectory.
- Tasks 2-5 done; Milestone 1 committed. Notes: goal reviewer replies must be JSON only (say so in the brief);
  extract the GOAL block with scratchpad 078/goal.py (it also prints the parent span); re-entering check_goal opens
  a new step span, so start the reviewer span only after the last goto.
- Golden report is now built with goldenOptions() (contract_test.go); regenerate with
  `go test ./internal/runstats -run 'TestReportGoldenJSON|TestRenderReportGolden' -update`, then rerun Harbor shared tests.
- All 8 tasks done; Milestones 2 and 3 committed. Next: test_plan, feature changelog, spec reconciliation.
