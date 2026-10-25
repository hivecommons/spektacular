**Shape.** A new command group, `spektacular stats`, reads a logs directory and reports on the workflow runs in it. It has four subcommands: `list`, `run`, `compare` and `summary`. All the work happens in the `spektacular` repo, apart from the documentation, which goes in the `docs` repo. A new package, `internal/runstats`, does the reading and the arithmetic. It depends only on `internal/tracelog` (from 000073 and 000074) for the record type and the record, span-type, end-reason and attribute constants, so the log contract is defined in one place. It works in four stages:
- **Load.** It reads every `*.jsonl` file in the directory, read-only, one line at a time. A line that is not a valid record is skipped and counted.
- **Assemble.** It groups records into runs by `trace_id`, across files. It pairs each `span_start` with its `span_end` by `span_id` (a `span_end` alone is a complete span), and builds one timeline of events per run.
- **Select.** It picks runs by name, run id, kind and agent.
- **Analyse.** It builds a run report (totals, steps, step entries, tasks, helper launches, user waits, gaps), a comparison or a summary.

The analysis code is pure: no I/O, no clock and no configuration. It is tested against hand-built records with fixed timestamps. `cmd/stats.go` is a thin layer over the package. It does five things:
- works out the directory: `--dir`, or else the project's `config.LogDir(root)` from 000073;
- parses the selection flags;
- calls the package;
- writes the result through the shared output writer (JSON envelope with `"error": false`, `--fields`) or as plain text tables;
- turns every refusal into an `output.NewError` with a next action (error-messages-must-suggest-remediation).

**How the figures are defined.** These rules are what keep the numbers trustworthy.
- **Wall time** runs from a run's first recorded event to its last. Events are every record's start and end times, except the end of a span closed as `abandoned`. A restart writes those ends at the moment of the restart, which can be days later.
- **Unfinished spans.** Spans that are still open, or were abandoned, are capped at the run's last other event. An `interrupted` step already ends at its last command, as 000074 records it. So "an interrupted step counts up to the point it was cut off" holds without special cases.
- **Helper wait and user wait** are the union of the intervals of completed `sub-agent` and `user-wait` spans, those closed with `stopped`. That is the time during which at least one helper was running, or the user was being waited on. So concurrent helpers never add up to more than the wall time, and for waits that do not overlap the union equals the sum. Unfinished helpers and waits are listed and flagged, but count toward no total. Per-step and per-task waits are the same unions clipped to each step or task interval.
- **Working time** is wall time less user wait.
- **Commands per step** are the command records whose `parent_span_id` is one of that step's spans.
- **Gaps** are the idle stretches between consecutive events on the run's timeline, where a command covers its own start-to-end interval. Each gap is labelled by the event before it and the event after it, and by the step it fell in. This follows the method in `docs/workflow-performance-investigation.md`.
- **Run status** comes from the log alone, so a copied directory gives the same answer:
  - `finished`: the terminal step was reached;
  - `interrupted`: the run never finished and was abandoned by a restart or overtaken by a later run of the same kind and name;
  - `in_progress`: anything else.

Comparisons and summaries key steps by name, as the spec's Technical Approach asks, so runs of different lengths line up. A step a run never entered is `null` for that run, never zero.

**The report never changes the logs.** The package only opens files read-only, and it never writes, renames or deletes anything. One more change makes the guarantee hold inside a project. 000073 makes `runRoot` log every command, which for `stats` would mean an append to the day's standalone log and possibly a retention sweep. So the `stats` group carries a logging-exempt cobra annotation, the same pattern as the gate's exemption (`cmd/gate.go:127`), and `runRoot` writes no record for an exempt command. This is the one change to 000073's recorder. It is tested by comparing every log file byte for byte before and after each kind of report.

**Output is a contract for 000076 and 000077.** The JSON field names are fixed in this plan's Data Structures section and pinned by golden tests:
- durations are integer milliseconds (`*_ms`);
- timestamps are RFC 3339 UTC;
- unfinished items carry `end_reason` (`moved`, `finished`, `stopped`, `abandoned`, `interrupted`, or `open` for a span never closed) and a `complete` flag;
- comparisons and summaries use one generic figure shape (`figure`, `values[]`, `delta[]`, `delta_pct[]`, or `count`, `median`, `p90`), so a new figure needs no new shape.

Step entries carry `started_at` and helper launches carry the step and task they started in, which covers every timing figure Harbor's `metrics.py` derives from transcripts today. 000077 reuses the package's loaded runs and paired spans, and its totals come from the same report.

**Why this direction.** Reading through 000074's `tracelog.Reader` was rejected: it reads one run at a time, needs the run's identity up front, and drops command records that have no parent. A Harbor-side script was rejected because the spec requires a Spektacular command. Folding the report into `status` was rejected because `status` reads workflow state, and a copied logs directory has none. Summed waits were rejected because concurrent helpers would add up past the wall time. See `research.md#alternatives-considered-and-rejected`. Conventions that shape the work:
- every refusal names its fix (error-messages-must-suggest-remediation);
- the tests use fixed-timestamp fixtures and their own temp directories, never the wall clock (tests-must-not-depend-on-order);
- the plan changes no template, skill, settings format or this repo's own `.spektacular/` (plans-never-change-the-active-install).
