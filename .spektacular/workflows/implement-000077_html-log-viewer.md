# Working context: implement 000077_html-log-viewer (orchestrated)

## Drift check (read_plan)
- 000075 has landed: internal/runstats and cmd/stats.go exist with the names the plan uses.
- Notes (no stop needed): `runstats.Options{Gaps: 0}` already means "every gap" (plan suggested math.MaxInt32; use 0).
  `stats` group now has six forms (list, run, compare, summary, commands, spans from 000076); view is the seventh.
  cmd/stats.go has a `statsSelect` helper (format, opts, logs, candidates) and `statsOptions` validates --gap-threshold.
- Gap edges are `runstats.GapEdge{Event, Type, Name}`; label as "<event> of <type> <name>".
- cmd tests: `statsProject(t, level)`, `seedLogs`, `copyLogs`, `stats(t, args...)`, `statsJSON` exist in cmd/stats_test.go.
- Changelog mode: first task (no ## Changelog in plan).

## Task 1 (Build the page data) done
- internal/runview/{doc,model,build}.go + build_test.go + testdata/model.golden.json. `go test ./internal/runview -run TestModelGolden -update` regenerates.
- Test fixture helpers in build_test.go: `run1` fx, `treeRecords()`, `pausedRecords()`, `build(t, recs, thr)`, `rowOf`, `str`. Reuse them for write tests and genRun.
- Plan-store edits: scratchpad scripts tick.py (tick task + criteria) and cl.py (append changelog entry).
- Goal check: stage diffs with scratchpad stage.sh.

## Task 2 (Write the page) done
- assets/{page.html,viewer.css,viewer.js}, assets.go, write.go, write_test.go (genRun(n,seed), everyTypeRecords(), page(), -sample-dir flag, BenchmarkBuildAndWrite2000).
- Browser check tooling: scratchpad tools/cdp.mjs <page> <prefix> <steps.json> (headless Chrome over CDP, offline; steps click/dblclick/drag/wheel/key/eval/shot).
- Reviewer replies must be JSON only; if it wraps prose, SendMessage it to resend JSON only.
- viewer.js state: {from,to,selected,threshold}; `visible` list + `position` index; Milestone 2 extends these (collapse, zoom, threshold input, summary).

## Task 3 (stats view command) done; Milestone 1 committed
- cmd/stats_view.go + cmd/stats_view_test.go; cmd/stats_test.go lists updated for `view`.
- Built binary for manual checks: scratchpad bin/spek; copied project logs at scratchpad logs/ (read-only copy).

## Task 4 (zoom/collapse/summary) done; Milestone 2 committed
- Page controls ids: reset-zoom, expand-all, collapse-steps, gap-threshold, gap-summary, longest-steps, longest-gaps. Triangles are ▶/▼ glyphs in the label column.
- Docs task: real look is task purple, step blue, sub-agent teal, user-wait amber hatched + band, work olive, command thin slate; problems failed red ✕, interrupted orange dashed ‖, abandoned grey dotted ∅, never closed fading end …; gap labels "gap 20m00s" at the foot.
