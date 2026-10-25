# Implement 000075_run-stats-command: working context

Orchestrated run (epic 000002). Code: spektacular worktree
`.spektacular/worktrees/000075_run-stats-command/spektacular`, docs worktree
`.spektacular/worktrees/000075_run-stats-command/docs` (needs `npm ci`).

## read_plan findings
- 000073 and 000074 have landed in the worktree branch: `internal/tracelog` (Record, constants),
  `cfg.LogDir(root)`, `logAnnotation = "log"`, `logExempt = "exempt"`, `logExempted` in cmd/root.go.
- Semantics match the plan: interrupted step and spans under it end at last command under the step;
  RunEnded closes leftovers as abandoned at the finish/restart time; command records parented to the
  state's step span; agent spans carry spektacular.step; terminal step is an instant span ended `finished`.
- Only line-number drift (rootCmd.AddCommand now near cmd/root.go:706; README status block ~line 29-37,
  Spans paragraph follows). Nav label for the logging page is "Logging and debugging".
- No `## Changelog` in plan yet: first-task invocation.

## Task 1 (Load logs into runs): done
- Goal check attempt 1 flagged a line with no span_id as not skipped; parseLine now rejects it.
- Test builders in internal/runstats/fixtures_test.go: `fx{trace,kind,name,agent}` with `.cmd`, `.open`,
  `.span`, `closeSpan`, `recs(...)`, `writeLog`, `snapshot`, `at(minutes)` around fixed `t0`.
- Plan-tick helper script: scratchpad s75/tick.py; changelog append: s75/addlog.py.
- golangci-lint crashes in this env; project lint is `go vet ./...` (Makefile).

## Task 2 (Report on one run): done
- Goal reviewers cannot read the plan: a criterion citing a plan section needs that text in the work
  (Report's doc comment now carries the JSON contract). Reviewer replies with trailing prose are refused
  as invalid JSON; re-ask a fresh reviewer.
- gap_threshold_ms lives in Report; cmd adds log_dir and skipped_lines only. RunSummary is in report.go.

## Task 3 (Keep stats out of the logs): done. cmd/stats.go holds statsCmd (exempt annotation), registered after statusCmd.

## Task 4 (stats list/run CLI): done; Milestone 1 committed via auto_commit.
- cmd tests: seedLogs/statsProject/stats/statsJSON helpers in cmd/stats_test.go; t.Chdir(t.TempDir()) for no-project commands (worktree guard).
- gaps under 1 ms are not gaps.

## Tasks 5-6 (compare/summary): done; Milestone 2 committed. Remaining: docs page (docs repo, needs npm ci) and README.

## Tasks 7-8 (docs page, README): done; Milestone 3 commit made on advancing to test_plan.
