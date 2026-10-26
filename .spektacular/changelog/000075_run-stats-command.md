---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# 000075_run-stats-command: run statistics

## What was built

Spektacular has a built-in report on where a workflow run's time went, read from the logs it already keeps (specs 000073 and 000074). `spektacular stats` has four forms:

- `stats list` lists every recorded workflow run, newest first, with its kind, name, agent, start time, wall time and status (`finished`, `interrupted` or `in_progress`, worked out from the log alone).
- `stats run` reports on one run, by default the latest. It gives:
  - the totals: wall time, working time, user wait, helper wait, command time and the time in gaps over a threshold;
  - a row per step, with entries, duration, commands, command time, waits and incomplete entries;
  - every step entry and plan task;
  - every helper launch and wait on the user;
  - the longest gaps between recorded events, each with its step and the events either side.
- `stats compare` lines runs up step by step and in total, showing each run's figures and its difference from the first run in time and percentage. A step a run never entered is absent (`null`), never zero.
- `stats summary` gives the count, the median and the nearest-rank 90th percentile for each step and for the totals.

Every form takes the same selection: `--name`, `--kind`, `--agent`, and a repeatable `--run` taking a full run id or a unique prefix of 8 or more characters. Each prints a plain table by default, or the standard JSON envelope with `--format json`, which honours `--fields`. `--dir` reads a logs directory copied from anywhere, with no project needed. The JSON field names are a fixed contract (durations in integer milliseconds, times in RFC 3339 UTC), pinned by golden files.

The figures are built so they can be trusted:

- Helper and user waits are unions of the completed spans' intervals, so overlapping helpers count once.
- Unfinished work (`interrupted`, `abandoned`, `open`) is listed and flagged, but kept out of every wait total. An interrupted step counts up to its last command.
- A restart that closes a run's leftover spans days later does not stretch the run.
- A damaged log line is skipped and counted (`skipped_lines`), and changes nothing else.
- Reporting only ever opens files to read them. The `stats` group carries 000073's logging-exempt annotation, so a report is never logged, and the logs it reads stay byte-identical.

The work is in a new read-side package, `internal/runstats`, beside the write-side `internal/tracelog`:

- a loader;
- an assembler (runs, paired spans, a timeline per run);
- selection;
- the run report, comparison and summary as pure functions;
- plain text renderers.

The command layer is `cmd/stats.go`. The documentation site has a new "Run statistics" page, linked from the Resources menu and from the logging page, and the README lists the four forms.

## Why it matters

Maintainers can see slow runs and regressions in seconds instead of piecing timings together from agent transcripts and harness scripts. The Harbor test harness (spec 000076) and the HTML run viewer (spec 000077) get one trusted, stable source for their timings: 000076 reads the JSON output, and 000077 reuses the package's loaded runs, paired spans and report totals.

## Deviations from the plan

- `gap_threshold_ms` is a field of `Report`, `Comparison` and `Summary` themselves, so the golden files pin it. The command layer adds only `log_dir` and `skipped_lines`.
- `RunSummary` and `(*Run).Summary()` are in `internal/runstats/report.go`, not a separate `summary_types.go`.
- A line with no `span_id` is skipped and counted too (the goal check asked for it). Run ids given with `--run` are lower-cased, and a run named twice is kept once.
- Stretches shorter than one millisecond are not gaps. This was found running the built binary on real logs.
- `stats list` with a selection that matches nothing gives an empty list. `stats_no_runs` is for the reports.
- The JSON contract is reproduced in doc comments (`Report` in `internal/runstats/report.go`, and `statsCompareResult` in `cmd/stats.go`), so a reader of the code can check field names without the plan.
- The tables mark an unfinished task, helper or wait with `*`, and count a step's unfinished entries in `INCOMPLETE`.
- The supporting benchmark `BenchmarkLoadAndReport` was added at the end of the run. It covers ten generated runs, about 90 MB of verbose records, and measured about 0.10 s per load-and-report.
- The README block sits after the Spans paragraph rather than directly under the `status` code block.
