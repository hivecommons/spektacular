# Working context: 000074_trace-spans-for-agent-work (orchestrated implement run)

## Decisions and substitutions
- read_plan passed: structure complete, every spec item covered, no Changelog yet (first-task mode).
- 000073 landed with the names the plan assumed (tracelog.Record/Run/Logger/Status, NewSpanID/NewTraceID in tracelog/ids.go, runFilePath, workflow.State.RunID, NewRunID, EnsureRunID, runRoot with stateSnapshot/attributeRun/buildRecord/recordCommand, snapshotStatePath, logLevel, forcedLogLevel). Line numbers in context.md are stale; adapt on the fly.
- recordCommand builds the logger per command (tracelog.New(cfg.Log.Provider, FileOptions{cfg.LogDir(root), cfg.Log.Retention})).
- cmd TestMain has -log-level flag; tests asserting logging call useConfiguredLogLevel.
- Plan context.md says docs root is spektacular-website; use the docs worktree instead.

## Learnings
- Task 1 (tracelog spans) done. ParseSpanRef accepts all-zero ids (reviewer: criterion accepts every well-formed id). Added Spans.StartRecord and Spans.Index helpers.
- Goal-check scope list names only non-test files, but test files were accepted.
- Helpers in scratchpad s74/: tick.py (tick task), addlog.py (append changelog entry).
- Upcoming: cmd/log_test.go requireWellFormed requires Record==span; TestLog_OutputParityAcrossLevels compares outputs across levels (instructions will contain random step span ids once the trace block exists: normalise span ids there). recordNames helpers may need to filter span records.
- Task 2 (engine step spans) done: moveSpans/resumeSpans/prepareSpan in workflow.go; OpenSpan has extra step field. TODO in task 4: pass EndFinished to RunEnded on terminal and let sink always mark leftovers abandoned (reviewer caveat).
- Task 3 done (task roles; RunEnded now gets EndFinished on terminal). Task 4: sink must mark RunEnded leftovers abandoned whatever the reason.
- Task 4 done (cmd recorder). Milestone 1 committed on loop to analyze.
- Task 5 done (trace commands). Milestone 2 committed.
- Task 6 done (trace partials; lead-in '**Mark spans.**'). Harbor oracle must mirror partial wording: 'trace start', '--type sub-agent', '--type user-wait'.
- Task 7 done (Harbor). Milestone 3 committed.
