# Working context: implement 000073_log-mode-and-logger-providers (orchestrated)

Code roots (worktrees, branch spek/000073_log-mode-and-logger-providers):
- spektacular: /home/nicj/code/github.com/hivecommons/spektacular/.spektacular/worktrees/000073_log-mode-and-logger-providers/spektacular
- docs: /home/nicj/code/github.com/hivecommons/spektacular/.spektacular/worktrees/000073_log-mode-and-logger-providers/docs

## read_plan findings
- Structure valid, 9 tasks, all context links resolve. No `## Changelog` yet (first-task mode).
- Drift: none material. Line numbers in context.md for config.go are shifted (~+26 lines, e.g. Debug field at :421 not :395). 000072 merged but did not bump the project settings format (still 4), so project4to5 stays numbered as planned.
- `cmd/stats.go` does not exist yet: docs page names `status` only and states the rule for reporting commands.
- Spec coverage: every requirement and AC is covered.

## Progress and decisions
- Tasks 1-3 done (tracelog package, run id in workflow state, log config + project4to5 migration). Nothing committed yet: milestone 1 commits after task 4.
- Goal-check reviewer: spek-goal-reviewer; tell it the untracked/modified files of earlier tasks are not this item's (tree is uncommitted across tasks).
- Bridge left in cmd/root.go by task 3: `sessionLogOn(cfg.Log.EffectiveLevel() == config.LogLevelVerbose, ...)`; task 4 replaces it. cmd session-log tests now toggle `log: level: verbose` instead of debug.
- Removing `debug` in project4to5 is reported as `set` with empty To (Key debug.enabled, From old value).
- Scratch helpers in the session scratchpad: tick.py/addlog.py/tickonly.sh/logonly.sh/stage.sh/clean.sh (session-local; recreate if lost).
