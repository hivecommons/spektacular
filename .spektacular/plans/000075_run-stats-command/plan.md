---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Plan: 000075_run-stats-command

<!-- Metadata -->
<!-- Created: 2026-10-25T08:37:59Z -->
<!-- Commit: 5bfbdbd -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Spektacular gains `spektacular stats`, a built-in report that reads the run logs it already keeps (specs 000073 and 000074). The report answers where a workflow run's time went:
- wall time and working time;
- the time spent waiting on helper agents and on the user;
- each step's and each plan task's share;
- every helper launch;
- the longest pauses.

It also compares runs side by side and summarises many runs with medians and 90th percentiles. Maintainers see slow runs and regressions in seconds instead of piecing them together from agent transcripts. The Harbor test harness (spec 000076) and the HTML run viewer (spec 000077) get one trusted, stable source for their timings.

## Conventions

- **Error messages must describe the problem and suggest remediation**: every `stats` refusal is an `output.NewError(...).WithNextAction(...)` with a runnable fix. These cover an unsupported format, no project and no `--dir`, a missing directory, no matching runs, an unknown or ambiguous run id, a bad kind, fewer than two runs to compare, and a bad gap count or threshold.
- **Tests must not depend on order**: the new `cmd` tests go through `runRootCmd`/`resetRootCmd` (the repeatable `--run` flag is a slice flag that the reset already handles). They use fixture logs with fixed timestamps in their own temp directories, never the wall clock.
- **Passing tests are required before calling work done**: the new `stats` group is registered on the root command, which every `cmd` test drives. The full shuffled Go suite must pass.
- **000073 owns the logging exemption**: spec 000073 adds the logging-exempt cobra annotation, checked in `runRoot`, and marks `spektacular status` with it. This plan only sets that annotation on the `stats` group, adds no exemption mechanism of its own and does not change `runRoot`. There is no per-command environment override to rely on: 000073 removes `SPEKTACULAR_SESSION_LOG`.
- **A plan never changes the active skills and configuration**: the feature adds a command and touches no template, skill, managed section or settings format. No task installs the binary or touches this repo's `.spektacular/`. Manual checks run the built binary against copied or throwaway logs.
- **Store files are written through the CLI**: the logs directory is not a store, so the report reads it directly (read-only). No task writes a store file by hand.
- **Plans must sketch content structure, not just summarize it (docs repo)**: the new Run statistics page carries a content outline with the verified command names, flags and field names.
- **No em dashes (docs repo)**: all new docs, README and changelog prose avoids em dashes.
- **MDX authoring conventions (docs repo)**: the new page is built from `Hero`, `Prose` and `CtaBanner`, with fenced code blocks and no layout HTML in its body.
- **Site layout conventions (docs repo)**: the page composes existing section components and adds no new frame, width or heading size.

## Architecture & Design Decisions

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

**The report never changes the logs.** The package only opens files read-only, and it never writes, renames or deletes anything. One more change makes the guarantee hold inside a project. 000073 makes `runRoot` log every command, which for `stats` would mean an append to the day's standalone log and possibly a retention sweep. 000073 adds a logging-exempt cobra annotation, which `runRoot` checks on the command that ran and its parents, writing no record for a command so marked. The `stats` group sets 000073's logging-exempt annotation, and its subcommands inherit it. This plan adds no exemption mechanism and does not change 000073's recorder. It is tested by comparing every log file byte for byte before and after each kind of report.

**Review decision (2026-10-25).** At the epic's end-of-planning review the user moved the logging-exemption mechanism (the cobra annotation and its check in `runRoot`) to spec 000073, which also marks `spektacular status` as never logged. 000073 also removes `SPEKTACULAR_SESSION_LOG`, so nothing here relies on a per-command override. This plan now only applies 000073's annotation to the `stats` group, using the annotation name 000073 ships.

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

## Component Breakdown

- **Run statistics package (new, `spektacular` repo).** This package owns everything the report computes. It is built in layers, each using the one before:
  - **Log loader.** Reads a logs directory read-only, line by line, into records. It skips and counts lines that are not valid records: unparsable JSON, a missing trace id, record kind or start time, or a partly written last line. It never fails on a single line.
  - **Run assembler.** Groups records into runs by trace id, across files. It keeps only traces that carry workflow attributes, takes kind, name, agent and log file from their records, and pairs span starts and ends by span id. It builds each run's event timeline, leaving out the ends of spans closed as abandoned, and derives the run's start, end, wall time and status.
  - **Selector.** Narrows runs by name, run id (or unique prefix), kind and agent, and applies each mode's defaults: the most recent run; the two most recent of its kind; every run of its kind.
  - **Run report.** Builds one run's totals, per-step figures, step entries, per-task figures, helper launches, user waits and gaps. It uses a small interval helper (union, clip, total) for waits and gaps.
  - **Comparison and summary.** Line runs up by step name. They produce each figure's per-run values with differences from the first run, or the count, median and 90th percentile across runs.
  - **Text renderer.** Draws the same results as plain text tables for people.

  The package depends only on the trace log package from 000073 and 000074, for the record type and the record, span-type, end-reason and attribute names. Its analysis is pure, so the command layer and spec 000077's viewer both reuse it.
- **Stats command group (new, `spektacular` repo).** `stats list`, `stats run`, `stats compare` and `stats summary`. It owns:
  - the shared flags: `--dir`, `--format`, `--name`, `--kind`, `--agent`, and a repeatable `--run`;
  - the run report's `--gaps` and `--gap-threshold`, which comparisons and summaries also use for their gap figures;
  - working out the logs directory: `--dir`, otherwise the project's configured log directory;
  - input validation and every refusal, each with a next action;
  - `--schema`.

  It writes results through the shared output writer (the JSON envelope and `--fields`) or through the package's text renderer. A bare `stats` returns the standard unknown-subcommand error. It carries 000073's logging-exempt annotation and is registered on the root command.
- **Command recorder in the root command (reused, unchanged; rewritten by 000073).** 000073's recorder already writes no record for a command carrying its logging-exempt annotation, or whose parent does. That keeps every report read-only inside a project. This plan changes nothing in 000073's pipeline.
- **Output writer (reused, unchanged).** It provides the JSON envelope, the `"error": false` discriminant, the `--fields` filter and the error shape.
- **Trace log package (reused, unchanged; from 000073 and 000074).** It is the source of the record type and the names the report keys on. Its per-run reader and span index are not used by the report.
- **README (changed, `spektacular` repo).** The "How It Works" command overview gains the four `stats` forms, next to `status`.
- **Documentation site (changed, `docs` repo).** A new "Run statistics" page explains:
  - listing, reporting, comparing and summarising runs;
  - selecting runs;
  - reporting on a copied logs directory;
  - what each figure means, and how unfinished work and damaged lines are shown.

  The Resources navigation links to it, and the logging page gains one sentence pointing to it.

## Data Structures & Interfaces

**Input: the log records (read, unchanged).** The report reads the record format that 000073 defines and 000074 extends, and changes none of it: `record` (`span`, `span_start`, `span_end`), `trace_id`, `span_id`, `parent_span_id`, `name`, `start_time`, `end_time`, `status{code,message}`, and these attributes:
- `spektacular.span.type`: `command`, `step`, `task`, `sub-agent`, `user-wait` or `work`;
- `spektacular.span.end_reason`;
- `spektacular.agent`;
- `spektacular.workflow.kind` and `spektacular.workflow.name`;
- `spektacular.step`;
- `spektacular.task.id`.

It reads every `*.jsonl` file in the directory: run files and `standalone_<day>.jsonl` files alike.

**Commands.**

```text
spektacular stats list    [selection] [--dir <path>] [--format pretty|json]
spektacular stats run     [selection] [--gaps 10] [--gap-threshold 5m] [--dir <path>] [--format pretty|json]
spektacular stats compare [selection] [--gap-threshold 5m] [--dir <path>] [--format pretty|json]
spektacular stats summary [selection] [--gap-threshold 5m] [--dir <path>] [--format pretty|json]

selection:  --name <workflow name>   --kind spec|plan|implement|repo   --agent <agent>
            --run <run id or unique prefix of 8+ hex characters>   (repeatable)
```

- `--format` defaults to `pretty`.
- `--dir` defaults to the project's configured log directory.
- `--gap-threshold` takes a Go duration.
- Every subcommand supports `--schema`, and the JSON form honours the global `--fields`.

**Structured output (JSON, field names are a contract for 000076 and 000077).** Every success response carries `"error": false`, `log_dir` (the absolute directory read) and `skipped_lines` (lines that could not be read). The units and conventions are:
- durations are integer milliseconds;
- times are RFC 3339 UTC;
- a figure that does not apply is `null`, never `0`.

*Run summary*: one per run, used by `list`, `run`, `compare` and `summary`.

```json
{"run_id":"4bf92f3577b34da6a3ce929d0e0e4736","kind":"plan","name":"000075_run-stats-command",
 "agent":"claude","log_file":"20261025T082050.123456789Z_claude_plan_000075_run-stats-command.jsonl",
 "started_at":"2026-10-25T08:20:50.123Z","ended_at":"2026-10-25T09:02:11.400Z",
 "wall_ms":2481277,"status":"finished"}
```

`status` is one of `finished`, `interrupted` or `in_progress`.

*`stats list`*: `{"log_dir", "skipped_lines", "runs": [<run summary>, ...]}`, newest first. An empty directory gives `"runs": []`.

*`stats run`*: a report on one run.

```json
{"log_dir":"/abs/.spektacular/logs","skipped_lines":0,"gap_threshold_ms":300000,
 "run":{"...":"run summary"},
 "totals":{"wall_ms":2481277,"working_ms":1803000,"user_wait_ms":678277,"helper_wait_ms":512000,
           "command_ms":4100,"commands":61,"helpers":3,"user_waits":2,
           "gap_ms":1520000,"gaps_over_threshold":3},
 "steps":[{"step":"discovery","entries":2,"duration_ms":812000,"commands":9,"command_ms":610,
           "helper_wait_ms":402000,"user_wait_ms":0,"incomplete":1}],
 "step_entries":[{"step":"discovery","span_id":"a3ce929d0e0e4736","task_id":null,
                  "started_at":"...","ended_at":"...","duration_ms":402000,
                  "end_reason":"interrupted","complete":false}],
 "tasks":[{"task_id":"df1bade8-66a7-4bb8-b261-3dda877ae82c","name":"Trace log package",
           "span_id":"...","started_at":"...","ended_at":"...","duration_ms":300000,
           "helper_wait_ms":120000,"user_wait_ms":0,"end_reason":"moved","complete":true}],
 "helpers":[{"span_id":"...","parent_span_id":"...","name":"research","step":"discovery",
             "task_id":null,"started_at":"...","ended_at":"...","duration_ms":150000,
             "end_reason":"stopped","status":"ok","message":null,"complete":true}],
 "user_waits":[{"...":"same shape as helpers"}],
 "gaps":[{"started_at":"...","ended_at":"...","duration_ms":1200000,"step":"walkthrough",
          "after":{"event":"end","type":"command","name":"plan goto"},
          "before":{"event":"start","type":"command","name":"plan goto"}}]}
```

- **Totals.** `gap_ms` is the total of gaps longer than the threshold, and `gaps_over_threshold` is how many there are. `commands`, `helpers` and `user_waits` count every item, finished or not.
- **Steps.** `steps` has one row per step name, in order of first entry. `incomplete` counts its entries that did not end with `moved` or `finished`.
- **End reasons.** `end_reason` is one of the 000074 reasons (`moved`, `finished`, `stopped`, `abandoned`, `interrupted`), or `open` for a span never closed. `complete` is true for `moved`, `finished` and `stopped`.
- **Unfinished spans.** For a span that is not complete, `ended_at` and `duration_ms` run to where it was cut off. These are the capped times, as Architecture defines them.
- **Gaps.** `gaps` is longest first, at most `--gaps` entries. `after` and `before` name the events either side. `event` is `start` or `end`, `type` is a span type, and `name` is the span or command name.

*`stats compare`*: runs side by side.

```json
{"log_dir":"...","skipped_lines":0,"gap_threshold_ms":300000,
 "runs":[{"...":"run summary, base first"},{"...":"run summary"}],
 "totals":[{"figure":"wall_ms","values":[2481277,1990000],"delta":[0,-491277],"delta_pct":[0,-19.8]}],
 "steps":[{"step":"discovery","figures":[
   {"figure":"duration_ms","values":[812000,null],"delta":[0,null],"delta_pct":[0,null]}]}]}
```

- Arrays line up with `runs`. A step a run never entered is `null` there, with `null` deltas.
- `delta_pct` is `null` when the base value is 0 or `null`. Percentages are rounded to one decimal.
- Total figures, in order: `wall_ms`, `working_ms`, `user_wait_ms`, `helper_wait_ms`, `command_ms`, `commands`, `helpers`, `gap_ms`.
- Step figures, in order: `duration_ms`, `entries`, `commands`, `command_ms`, `helper_wait_ms`, `user_wait_ms`.

*`stats summary`*: many runs at once. It has the same top-level shape as `compare`, except that each figure is `{"figure":"duration_ms","count":5,"median":812000,"p90":1104000}`.
- `count` is the number of runs the figure applies to. For a step, that is the runs that entered it.
- `median` and `p90` are JSON numbers. A median of an even count can be fractional.

**Errors.** Every error is the standard error envelope with a `next_action`:

| Code | When | Next action |
|---|---|---|
| `stats_format_unsupported` | `--format` is not `pretty` or `json` | re-run with `--format pretty` or `--format json` |
| `no_project` | no project and no `--dir` | run from a project or pass `--dir <logs directory>` |
| `stats_log_dir_not_found` | the directory does not exist | check the path; the project's own is the `log.config.directory` setting |
| `stats_bad_selection` | unknown `--kind`, or a malformed `--run` | names the accepted kinds, or the id format |
| `stats_run_not_found` | a `--run` matches no run | run `spektacular stats list` |
| `stats_run_ambiguous` | a `--run` prefix matches several runs | give more characters, listing the matches |
| `stats_no_runs` | no run matches the selection | run `spektacular stats list`, or widen the selection |
| `stats_compare_needs_two` | fewer than two runs to compare | name two runs with `--run`, or widen the selection |
| `stats_bad_option` | `--gaps` below 1, or `--gap-threshold` not positive | gives the accepted range |

**Go interfaces (`internal/runstats`, new).**

```go
type Logs struct{ Dir string; Runs []*Run; Skipped int }
func Load(dir string) (*Logs, error)                 // read-only; error only when the directory cannot be read
func Assemble(recs []tracelog.Record) []*Run          // pure: group, pair, timeline, status

type Run struct {
    ID, Kind, Name, Agent, LogFile string
    Start, End time.Time
    Status   string            // finished | interrupted | in_progress
    Spans    []Span            // every non-command span, paired, in start order
    Commands []tracelog.Record // command records, in start order
    Events   []Event           // the timeline the wall time and gaps are measured on
}
type Span struct {
    ID, Parent, Name, Type, Step, TaskID string
    Start, End time.Time // End is the effective end (capped when not complete)
    EndReason, Status, Message string
    Complete bool
}
type Event struct{ At, Until time.Time; Edge, Type, Name string } // Until > At only for commands

type Filter struct{ Name, Kind, Agent string; Runs []string }
func (l *Logs) Candidates(f Filter) ([]*Run, error) // newest first; *SelectionError for unknown/ambiguous/malformed
func ForReport(c []*Run) (*Run, error)
func ForCompare(c []*Run, f Filter) ([]*Run, error)
func ForSummary(c []*Run, f Filter) ([]*Run, error)

type Options struct{ Gaps int; GapThreshold time.Duration }
func BuildReport(r *Run, o Options) Report
func Compare(runs []*Run, o Options) Comparison
func Summarise(runs []*Run, o Options) Summary
func RenderList(w io.Writer, l *Logs, runs []*Run) error // and RenderReport, RenderComparison, RenderSummary
```

`Report`, `Comparison` and `Summary` are the Go types behind the JSON above, with matching `json` tags. `cmd` adds `log_dir` and `skipped_lines`. No existing type, record field or setting changes. `cmd` adds no annotation key: the `stats` group uses 000073's logging-exempt annotation.

## Implementation Detail

**A read-side package beside the write-side one.** 000073 and 000074 make `internal/tracelog` the package that writes the log. This plan adds its counterpart for reading: `internal/runstats`.
- It imports `tracelog` for the record type and the names. `tracelog` never imports it, so the write path every command takes stays as small as 000073 planned.
- Inside the package, I/O sits at one edge: the loader, and the text renderer writing to an `io.Writer`. Everything between them is a pure function from records to results.
- A developer reading it finds one file per stage (load, assemble, select, report, gaps, intervals, compare, summary, render). Each stage has a table-driven test fed with hand-built records.
- This follows the layering of `internal/status`, which builds a `Report` as data and renders it separately as pretty text. `cmd` only wires flags to the package.

**One timeline per run, and everything measured against it.** Assembly turns a run's records into three things:
- paired spans, each with an effective end and a `complete` flag;
- its command records;
- a sorted list of timeline events.

Every figure then comes from those three:
- wall time is the timeline's extent;
- waits are interval unions of complete spans;
- per-step and per-task figures clip those unions to each entry's interval;
- gaps are the uncovered stretches between consecutive timeline events;
- status comes from the end reasons and from the other runs of the same workflow.

A small interval helper (merge, clip, total) is the only arithmetic primitive, so double counting is impossible by construction. The rules for unfinished work live in one place, the effective-end computation, instead of being repeated in each figure: abandoned ends are left out of the timeline, and open or abandoned spans are capped at the run's last other event.

**Comparison and summary as figure tables.** Both start from per-run reports. They join the reports by step name, keeping the step order of the first run that has each step, and emit one generic row per figure. That generic shape (`figure` with aligned `values`/`delta`/`delta_pct`, or `count`/`median`/`p90`) is new to the codebase. It is chosen so a later spec can add a figure without changing the output's shape. Harbor reads it in 000076, and the viewer in 000077.

**The command follows the existing group pattern.** `stats` is built like the other command groups:
- a cobra group whose bare form returns the `unknown_subcommand` envelope;
- subcommands with shared flags;
- `--schema` through `commandSchema`;
- `--format pretty|json` validated as `status` validates it;
- JSON through the shared output writer, so `--fields` works;
- every refusal an `output.NewError` with a next action.

Sentinel and typed errors from the package (an unknown, ambiguous or malformed run id, no runs, too few to compare) are mapped to output errors in one place in `cmd`.

**Exempt from logging, by annotation.** 000073 gives the root recorder a logging-exempt cobra annotation, matching the gate's: a command whose path carries it is left out of the log. It is checked on the command that actually ran, so it works in tests and in production. The `stats` group sets 000073's logging-exempt annotation once, on the group, and its subcommands inherit it through the parent walk. The implementer uses the annotation name 000073 ships. No code from 000073 is changed: a read-only log command is simply never logged.

**Reading large logs cheaply.** Verbose-level lines carry full responses and can be many megabytes long. The loader:
- reads lines with no length cap;
- decodes into the record type, whose attributes are a plain map;
- streams each file rather than holding it whole.

A benchmark over a generated ten-run directory backs the under-one-second success metric. No index, cache or sidecar file is introduced.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports none).
- **Upstream spec `000073_log-mode-and-logger-providers` must land first.** It is planned but not implemented. This plan builds on:
  - its record format and `internal/tracelog` record type;
  - its run-file and day-file naming in the logs directory;
  - `config.LogDir(projectRoot)`;
  - its rewritten `runRoot` and its logging-exempt annotation, which the `stats` group sets.

  No task starts until 000073's Milestone 1 is merged. If its names differ, adapt to them. If a record field's meaning differs from its plan, STOP and ask.
- **Upstream spec `000074_trace-spans-for-agent-work` must land first.** It is planned but not implemented. This plan builds on:
  - its `span_start`/`span_end` records and the span types;
  - the end reasons and what each means for timing: an interrupted step ends at its last command, and abandoned spans end when their parent or run ends;
  - parents (command records under their step span, step spans under task spans);
  - task spans' `spektacular.task.id`, and agent spans' `spektacular.step`;
  - the constants in `internal/tracelog`.

  Its `Reader` and `Index` are not used. The report's tasks need its Milestone 1 (step and task spans), and its helper and user-wait figures need its Milestone 2 (`trace start`/`trace stop`). The same adapt-or-STOP rule applies.
- **Downstream specs that read this plan's output:**
  - `000076_harbor-on-built-in-logs` reads the JSON output (step entries, helper launches with step and task, totals, per-step split, comparisons);
  - `000077_html-log-viewer` reuses the package's loaded runs, paired spans and report totals.

  The JSON field names and the package's exported types in Data Structures are their contract and must not drift during implementation.
- **`internal/output` (used, unchanged).** It provides the success envelope with `"error": false`, `--fields` filtering, and `NewError(...).WithNextAction` for every refusal.
- **`internal/config` (used, unchanged after 000073).** It resolves the project's log directory.
- **`cmd` (changed).** It gains a new `stats` group carrying 000073's logging-exempt annotation. `runRoot` is not changed. The `commandSchema` and `runUnknownSubcommand` conventions are reused.
- **Go standard library only** (`bufio`, `encoding/json`, `os`, `path/filepath`, `sort`, `math`, `time`, `text/tabwriter`), plus the existing `cobra`. There is no new module.
- **Documentation site, `docs` repo (changed).** A new page `src/pages/run-stats.mdx`, a Resources nav entry in `src/components/Nav.astro`, and one sentence in `src/pages/debugging.mdx` as 000073 and 000074 leave it. It is checked by the site build and `astro check`.
- **`README.md` (changed).** The "How It Works" command overview.
- **Harbor test harness: not changed.** `tests/harbor/shared/metrics.py` and `report.py` were read as the model for the figures and the table. Changing Harbor is spec 000076.
- **Investigation report `docs/workflow-performance-investigation.md` (read, unchanged).** Its method and per-step table are what the report reproduces from logs (success metric).

## Testing Approach

**Test types and where they slot in.** Every Go test runs under the project's shuffled, order-independent `go test`. No test reads the wall clock: fixtures are hand-built records, or JSON Lines files with fixed timestamps, in each test's own temp directory.

- **Unit tests in `internal/runstats`.** These carry the most coverage, because the figures are the product and two later specs consume them. They are table-driven over hand-built records:
  - **Loader:** run files and day files; a malformed line, a line missing its trace id or start time, and a partly written last line are each skipped and counted, without changing other runs; a missing directory errors; a very long verbose line is read.
  - **Assembly:**
    - records grouped by trace across files, and standalone traces excluded;
    - start and end pairing, including an end with no start;
    - kind, name, agent and log file;
    - wall time from the first to the last event, with an abandoned end left out;
    - status: finished, interrupted (abandoned by a restart, or overtaken by a later run of the same workflow) and in progress.
  - **Intervals:** merge, clip and total, with overlap, nesting and touching edges.
  - **Run report:**
    - totals for known waits: working time equals wall time less user wait, and the waits equal the sums of non-overlapping periods;
    - two overlapping helpers count once;
    - a step entered twice shows 2 entries, its total duration, its commands, its command time and its waits;
    - a two-task implement run gives both tasks with durations and waits;
    - three helper launches are listed with name, step and duration;
    - an interrupted step counts to its cut-off and is flagged;
    - an open or abandoned helper is listed and flagged, and left out of every total and per-step wait.
  - **Gaps:** a 20-minute pause is listed first with its step and its events either side; at most N gaps, longest first; totals above the default and a custom threshold; a command's own duration is never a gap.
  - **Comparison:** values and deltas against the first run, in time and percentage, including a step only one run entered (`null`, never 0) and a zero base (`null` percentage).
  - **Summary:** count, median (odd and even counts) and nearest-rank p90 over five runs, for steps and totals.
  - **Selection:** by name, kind, agent, kind with agent, full run id and unique prefix; an ambiguous prefix, an unknown id and a malformed id each give their typed error; the defaults for each mode.
  - **Rendering:** golden text for each mode, checked to show the same figures as the JSON.
- **JSON contract tests.** Golden JSON for one fixture run in each mode pins every field name and unit in Data Structures, so a rename breaks a test before it breaks 000076 or 000077.
- **Command-level integration tests in `cmd`**, through `runRootCmd`/`resetRootCmd`. They run against a fixture project whose logs directory is seeded with fixture files:
  - list shows four runs (three finished, one in progress) newest first, with each column;
  - the default report is the latest run;
  - each selection narrows as specified;
  - compare and summary;
  - `--format json` parses in the standard envelope and honours `--fields`, and pretty shows the same figures;
  - `--dir` on a copy outside any project gives identical JSON;
  - every refusal returns its code with a next action;
  - a bare `stats` returns `unknown_subcommand`;
  - every log file is byte-identical before and after each of the four subcommands, run inside an `init`ed project where logging is on, so 000073's logging exemption is proven end to end for `stats`.
- **Docs repo.** The site build and `astro check` pass, and the MDX guard returns nothing.

**Load-bearing guarantees.**
- Every figure is computed from the logs alone, identically for the project's logs and a copy.
- Waits are never double counted, and unfinished work never inflates a total.
- A damaged line changes nothing but the skipped count.
- Reporting never writes, moves or deletes a log.
- The JSON field names are pinned.

**Deliberate gaps.**
- No automated timing assertion: wall-clock thresholds are flaky in CI. A Go benchmark over a generated ten-run directory supports the speed metric.
- No end-to-end run against a real agent session, because real runs need 000073 and 000074 installed. That is a manual check.

**Success metrics.**
- *The investigation's per-step table (model, tool, helper-wait and user time per step) can be produced for a new run from the report alone, apart from model time.* **Manual — captured in the implementation test plan.** Supporting behavioural test: the run report's per-step rows carry duration, `command_ms` (the tool stand-in), `helper_wait_ms` and `user_wait_ms` for a fixture run built to mirror one investigation table row.
- *A report over a logs directory holding 10 runs returns in under one second on a developer laptop.* **Manual — captured in the implementation test plan.** Supporting evidence: a Go benchmark of load-and-report over a generated ten-run, verbose-sized directory.
- *The report's structured output contains every figure Harbor currently derives from transcripts for step, helper and idle timing.* **Manual — captured in the implementation test plan:** a field-by-field check of `metrics.py`'s step, child and idle figures against the JSON. Supporting behavioural test: the golden JSON carries step entries with `started_at` and `duration_ms`, helpers with step, task, start, end, duration and end reason, the longest gap, and the total gap time.

**Manual reviews.**
- **Manual — captured in the implementation test plan:** run `stats list`, `run`, `compare` and `summary` with the built binary against a real logs directory from a spec, plan and implement run, and confirm the figures are plausible and the tables readable.
- **Manual — captured in the implementation test plan:** read the Run statistics page on the built site, checking every command, flag and figure definition against the shipped behaviour.

## Milestones & Tasks

### Milestone 1: Maintainers can list runs and see where one run's time went

**What changes**: `spektacular stats list` shows every workflow run in the logs, newest first. Each run shows its kind, name, agent, start time, wall time, and whether it finished, was interrupted or is still in progress. `spektacular stats run` reports on one run, the latest by default, or one chosen by name, run id, kind or agent. The report gives:
- the run's wall time, working time, time waiting on the user and time waiting on helper agents;
- each step's total time, entry count, commands, command time and waits;
- each plan task's time and waits, for an implement run;
- every helper agent launched, with its step and duration;
- every wait on the user;
- the longest pauses, with the step each fell in, and the total time in pauses over a threshold.

Interrupted and unfinished steps, tasks, helpers and waits are flagged, and they never inflate a total. Both commands print a readable table, or the standard JSON envelope with `--format json`. They work on a logs directory copied from anywhere with `--dir`. A damaged log line is skipped and counted. Reporting never changes a log, because the `stats` commands are never logged themselves.

**Validation point**: The package's unit tests and the `cmd` integration tests pass in the full shuffled Go suite. They cover listing, every selection, totals, steps, tasks, helpers, gaps, unfinished work, damaged lines, a copied directory and byte-identical logs after reporting. Running the built binary against a fixture logs directory shows a readable list and run report.

#### - [ ] Task: Load logs into runs
**Id:** dad1813d-55e1-4a5e-9ee0-667f48f26a41
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds the run statistics package's foundation, which turns a logs directory into runs. It reads every log file read-only, skips and counts lines it cannot read, groups records into runs by run identifier, and pairs each span's start with its end. From those it works out each run's kind, name, agent, start and end, wall time and status. It also picks runs by name, run id, kind or agent, with each report mode's defaults. This task starts only once 000073's trace log package and 000074's span records are merged.

*Technical detail:* [context.md#task-load-logs-into-runs](./context.md#task-load-logs-into-runs)

**Acceptance criteria**:
- [ ] Records from run files and day files are grouped into one run per workflow run, and commands run outside any workflow are not reported as runs.
- [ ] Each run has the kind, name, agent and log file its records name, and its wall time runs from its first recorded event to its last, ignoring the moment a later restart closed its leftover spans.
- [ ] A run that reached its last step is finished. One that was restarted or overtaken by a later run of the same workflow is interrupted. Any other is in progress.
- [ ] A malformed line, a line missing its identifiers and a partly written last line are each skipped and counted, and every run's figures are unchanged by them.
- [ ] Runs can be picked by workflow name, by kind, by agent, by these combined, and by a full run id or a unique prefix of one. An unknown, ambiguous or malformed id is refused with a reason.
- [ ] With no selection, a report picks the latest run, a comparison the two latest runs of that run's kind, and a summary every run of that kind.
- [ ] Reading never creates, changes, moves or deletes a file in the logs directory.

#### - [ ] Task: Report on one run
**Id:** d2c23d18-d5b3-4e67-9c65-c8c3354474b9
**Repo:** spektacular
**Depends on:**
- dad1813d-55e1-4a5e-9ee0-667f48f26a41 — Load logs into runs
**Execution:** agent

Builds the report on one run:
- its totals: wall time, working time, user wait and helper wait;
- a row per step: entries, duration, commands, command time and waits;
- each step entry and each plan task;
- every helper launch and wait on the user;
- the longest gaps between recorded events, each with its step.

Waits are counted once even when helpers overlap. Interrupted and unfinished work is flagged and never inflates a total. The report is plain data, ready to be printed as a table or as JSON.

*Technical detail:* [context.md#task-report-on-one-run](./context.md#task-report-on-one-run)

**Acceptance criteria**:
- [ ] For a run with known waits, working time equals wall time less the user waits, and the user-wait and helper-wait totals equal the sums of those periods. Two helpers running at the same time are counted once.
- [ ] A step entered twice shows two entries, its total duration, its command count, its command time, and its helper and user wait.
- [ ] An implement run of a two-task plan shows both tasks, each with its duration and waits.
- [ ] A run with three helper launches lists all three, each with its name, step and duration.
- [ ] A 20-minute pause between two commands is listed first among the longest gaps, with its step and the events either side. At most the requested number of gaps is listed, longest first. The pause counts toward the total over the default 5-minute threshold, but not over a 30-minute one.
- [ ] An interrupted step is marked and counted up to where it was cut off. A helper launch or user wait that never finished is listed and marked, and left out of every wait total and per-step wait.
- [ ] The structured form of the report uses exactly the field names and units given in the plan's Data Structures section.

#### - [ ] Task: Keep the stats commands out of the logs
**Id:** a49a4d72-79d8-4ce2-b320-6a6b63eca363
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds the `spektacular stats` command group, with no subcommands yet, and marks it with the logging-exempt marker that spec 000073 provides, so every `stats` command inherits it. Without this, running a report inside a project would add a line to the day's log, and possibly trigger the clean-up of old logs, so the report would change the logs it reads. The marker and the rule that skips marked commands belong to 000073; this task only applies the marker and proves `stats` writes no record. This task starts only once 000073's rewritten command recorder and its logging-exempt marker are merged.

*Technical detail:* [context.md#task-keep-the-stats-commands-out-of-the-logs](./context.md#task-keep-the-stats-commands-out-of-the-logs)

**Acceptance criteria**:
- [ ] Running `stats` inside a project leaves every log file as it was, even with logging at the verbose level.
- [ ] A normal command run straight after it is still logged exactly as before, and its output and exit code are unchanged.

#### - [ ] Task: List runs and report on one from the command line
**Id:** ad0176a5-97fb-47c0-98b1-7267765659a2
**Repo:** spektacular
**Depends on:**
- dad1813d-55e1-4a5e-9ee0-667f48f26a41 — Load logs into runs
- d2c23d18-d5b3-4e67-9c65-c8c3354474b9 — Report on one run
- a49a4d72-79d8-4ce2-b320-6a6b63eca363 — Keep the stats commands out of the logs
**Execution:** agent

Adds `list` and `run` to the `spektacular stats` command group. Both read the project's logs, or the directory given with `--dir`, and both take the run selection. They print readable tables by default, or the standard JSON envelope with `--format json`. `run` also takes the number of gaps to list and the gap threshold. Every mistake is refused with a message saying what to do. Both inherit the group's logging-exempt marker.

*Technical detail:* [context.md#task-list-runs-and-report-on-one-from-the-command-line](./context.md#task-list-runs-and-report-on-one-from-the-command-line)

**Acceptance criteria**:
- [ ] In a project whose logs hold three finished runs and one in progress, `stats list` shows all four, newest first, each with kind, name, agent, start time, wall time and status.
- [ ] `stats run` with no selection reports the run that started most recently, and each selection reports only the runs it names.
- [ ] The JSON output parses in the same envelope as other commands and honours `--fields`, and the table output shows the same figures.
- [ ] Pointing `--dir` at a copy of the logs directory, from outside any project, gives the same figures as reporting in the project.
- [ ] A malformed log line leaves every figure unchanged, and the report shows one skipped line.
- [ ] Every log file is byte-identical before and after `stats list` and `stats run`, run inside a project with logging on.
- [ ] An unsupported format, no project without `--dir`, a missing directory, no matching run, an unknown or ambiguous run id, a bad kind, and a bad gap count or threshold are each refused with a message naming the fix. A bare `stats` names its subcommands.

### Milestone 2: Maintainers can compare runs and summarise many

**What changes**: `spektacular stats compare` puts two or more runs side by side, step by step and in total. It shows each run's figures and its difference from the first run, in time and as a percentage. A step one run never entered shows as absent for that run, not as zero. By default it compares the two most recent runs of the latest run's kind. `spektacular stats summary` summarises a set of runs, by default every run of the latest run's kind. For each step and for the totals, it gives the number of runs, the median and the 90th percentile. Both take the same run selection and output formats as the run report. This is what makes a slow run or a regression visible across runs, and what the Harbor harness's cross-run report will read in spec 000076.

**Validation point**: Unit tests over five fixture implement runs give the expected counts, medians and percentiles. A two-run spec comparison shows each step's values, its differences, and an absent step as `null`. `cmd` tests show both commands in both formats, with their refusals. The full Go suite passes.

#### - [ ] Task: Compare and summarise runs
**Id:** 8c6b6ead-cdbf-48a4-b7f3-ce6a3a5ac516
**Repo:** spektacular
**Depends on:**
- d2c23d18-d5b3-4e67-9c65-c8c3354474b9 — Report on one run
**Execution:** agent

Adds side-by-side comparison and multi-run summaries to the run statistics package. A comparison lines runs up step by step and in total. It shows each run's figures and its difference from the first run, in time and as a percentage, and shows a step a run never entered as absent. A summary gives, for each step and for the totals, the number of runs, the median and the 90th percentile. Both come with readable table forms.

*Technical detail:* [context.md#task-compare-and-summarise-runs](./context.md#task-compare-and-summarise-runs)

**Acceptance criteria**:
- [ ] Comparing two spec runs shows each step's duration for both runs, and the second run's difference from the first in time and percentage. The totals are shown the same way.
- [ ] A step only one run entered is shown as absent for the other, never as zero, and a difference against a zero or absent base has no percentage.
- [ ] Summarising five implement runs shows, for each step and for the totals, a count of 5, the median and the 90th percentile.
- [ ] A step entered by only some of the summarised runs shows how many runs entered it.

#### - [ ] Task: Compare and summarise from the command line
**Id:** d2baacfc-4bdf-4150-8660-4e6a52befcbc
**Repo:** spektacular
**Depends on:**
- ad0176a5-97fb-47c0-98b1-7267765659a2 — List runs and report on one from the command line
- 8c6b6ead-cdbf-48a4-b7f3-ce6a3a5ac516 — Compare and summarise runs
**Execution:** agent

Adds `spektacular stats compare` and `spektacular stats summary`, with the same run selection, logs directory and output formats as `stats run`. With no selection, `compare` takes the two most recent runs of the latest run's kind, and `summary` takes every run of that kind. Naming runs with `--run` compares or summarises exactly those, in the order given.

*Technical detail:* [context.md#task-compare-and-summarise-from-the-command-line](./context.md#task-compare-and-summarise-from-the-command-line)

**Acceptance criteria**:
- [ ] `stats compare` with no selection compares the two most recent runs of the latest run's kind, oldest first. With two or more `--run` values, it compares exactly those, in that order.
- [ ] `stats summary` with no selection summarises every run of the latest run's kind, and accepts the same selection as the other commands.
- [ ] Both commands give the same figures in table and JSON form, and the JSON uses the field names in the plan's Data Structures section.
- [ ] Asking to compare fewer than two runs is refused with a message saying how to name two.
- [ ] Every log file is byte-identical before and after `stats compare` and `stats summary`.

### Milestone 3: The run report is documented

**What changes**: The documentation site gains a "Run statistics" page, linked from the Resources menu and from the logging page. It explains how to list, report on, compare and summarise runs, how to choose runs, and how to report on a copied logs directory. It also defines every figure: wall time, working time, user wait, helper wait, command time, step entries, gaps and the gap threshold, unfinished work, and skipped lines. The project README lists the four `stats` commands beside `status`.

**Validation point**: The site builds and `astro check` passes, with no layout HTML in the page body and no em dashes in the new text. The README shows the `stats` commands.

#### - [ ] Task: Documentation site describes run statistics
**Id:** a86aebb6-e10d-48b7-9fbb-a7e273e0a257
**Repo:** docs
**Depends on:**
- ad0176a5-97fb-47c0-98b1-7267765659a2 — List runs and report on one from the command line
- d2baacfc-4bdf-4150-8660-4e6a52befcbc — Compare and summarise from the command line
**Execution:** agent

Adds a "Run statistics" page to the documentation site. It explains how to list, report on, compare and summarise runs, how to choose runs, and how to report on a copied logs directory, and it defines every figure the report shows. The Resources menu links to the page, and the logging page points to it.

**Content outline** (new page, in order; the wording is illustrative, the commands, flags, defaults and field names are fixed):
1. Hero: heading "Run statistics", sub "Ask Spektacular where a run's time went: in each step and task, waiting on helper agents, or waiting on you."
2. `## Listing runs`: `spektacular stats list` reads `.spektacular/logs/` and shows each workflow run, newest first. Example block:
   ```text
   RUN       KIND       NAME                       AGENT   STARTED              WALL     STATUS
   4bf92f35  implement  000075_run-stats-command   claude  2026-10-25 09:10:02  41m12s   in_progress
   9d0e0e47  plan       000075_run-stats-command   claude  2026-10-25 08:20:50  41m21s   finished
   ```
3. `## Reporting on one run`: `spektacular stats run` reports the latest run. Shows the totals line, the per-step table (STEP, ENTRIES, DURATION, COMMANDS, CMD TIME, HELPER WAIT, USER WAIT), tasks for implement runs, helper launches, waits on the user and the longest gaps. `--gaps 3` and `--gap-threshold 30m` change the gap list and threshold.
4. `## Choosing runs`: `--name <workflow name>`, `--kind spec|plan|implement|repo`, `--agent <agent>`, `--run <id or 8+ character prefix>` (repeatable), combined with AND. Defaults: `run` takes the latest run; `compare` takes the two latest runs of that run's kind; `summary` takes every run of that kind.
5. `## Comparing runs`: `spektacular stats compare --run 9d0e0e47 --run 4bf92f35`. Each step's duration per run, the difference from the first run in time and percent, and the totals. A step a run never entered shows as `-`, not `0s`.
6. `## Summarising runs`: `spektacular stats summary --kind implement`. Per step and in total: the number of runs, the median and the 90th percentile.
7. `## What each figure means`: a definition list.
   - **Wall time**: from the run's first recorded event to its last.
   - **Working time**: wall time less user wait.
   - **User wait**: time a `user-wait` span was open and finished, counted once when several overlap.
   - **Helper wait**: time at least one `sub-agent` span was running, counted once when helpers overlap.
   - **Command time**: the time Spektacular's own commands took.
   - **Entries**: how many times a step was entered.
   - **Gap**: a stretch with no recorded event, labelled with the step it fell in and the events either side.
   - **Gap total**: the time in gaps longer than the threshold (5 minutes by default).
8. `## Unfinished work and damaged lines`: steps, tasks, helpers and waits that were `interrupted`, `abandoned` or never closed (`open`) are marked. An interrupted step counts up to where it was cut off. Unfinished helpers and waits are listed but left out of every wait total. A line that cannot be read is skipped and counted as `skipped_lines`.
9. `## Reports for programs`: `--format json` gives the standard envelope. Example excerpt: `{"run":{"run_id":"...","kind":"plan","wall_ms":2481277,"status":"finished"},"totals":{"wall_ms":2481277,"working_ms":1803000,"user_wait_ms":678277,"helper_wait_ms":512000,"gap_ms":1520000},"steps":[{"step":"discovery","entries":2,"duration_ms":812000}],"error":false}`. Durations are milliseconds, times are UTC, and these field names are stable.
10. `## Logs from elsewhere`: `spektacular stats run --dir path/to/collected/logs` works on a logs directory copied from a test run, from any folder. Reporting never changes a log, and `stats` commands are never logged themselves.
11. CtaBanner: "Where do these records come from?" with a button to the Logging and debugging page.

*Technical detail:* [context.md#task-documentation-site-describes-run-statistics](./context.md#task-documentation-site-describes-run-statistics)

**Acceptance criteria**:
- [ ] The Run statistics page explains listing, reporting on, comparing and summarising runs, choosing runs, and reporting on a copied logs directory.
- [ ] The page defines every figure the report shows, and how unfinished work and skipped lines appear.
- [ ] The Resources menu links to the page, and the logging page points to it.
- [ ] The site builds and type-checks without errors, and the page body has no layout HTML and no em dashes.

#### - [ ] Task: README lists the stats commands
**Id:** b152f2a6-8bbc-4b92-b981-558c68cb032e
**Repo:** spektacular
**Depends on:**
- ad0176a5-97fb-47c0-98b1-7267765659a2 — List runs and report on one from the command line
- d2baacfc-4bdf-4150-8660-4e6a52befcbc — Compare and summarise from the command line
**Execution:** agent

Adds the four `stats` forms to the README's command overview, next to `status`, with one line each on what they show. It also gives a pointer to the documentation site for the figure definitions.

**Content example** (README "How It Works", after the `status` block):
```text
spektacular stats list                     # every recorded run, newest first
spektacular stats run [--name <spec>]      # where one run's time went: steps, tasks, helpers, waits, gaps
spektacular stats compare                  # the two latest runs of a kind, side by side
spektacular stats summary --kind implement # median and 90th percentile per step
```

*Technical detail:* [context.md#task-readme-lists-the-stats-commands](./context.md#task-readme-lists-the-stats-commands)

**Acceptance criteria**:
- [ ] The README shows `stats list`, `stats run`, `stats compare` and `stats summary` with a one-line description each, and mentions `--dir` and `--format json`.

## Open Questions

- **Whether 000073 and 000074 land with the record fields, constants and meanings this plan reads.** This depends on their implementations. The plan reads:
  - the `tracelog.Record` fields and `config.LogDir`;
  - the span types and end reasons;
  - an interrupted step ending at its last command;
  - abandoned ends written at the restart time;
  - command records parented to their step span;
  - `spektacular.task.id` on task spans and `spektacular.step` on agent spans.

  If only names differ, adapt to them. If a field's meaning, an end reason's timing or the parenting rule differs from their plans, STOP and ask the user, because the figures' definitions and the output contract for 000076 and 000077 rest on them.
- **Whether loading a real verbose-level logs directory of 10 runs stays under one second.** This depends on real log sizes, which only exist once 000073 and 000074 are running. If the manual measurement in the test plan exceeds one second, STOP and ask the user before adding anything that changes the design. For example, skipping verbose attributes with a pre-decode filter is an internal change and can be made; an index or cache file beside the logs cannot, because it would change the logs directory.

No other implementation-time uncertainties remain. Every design choice is settled in Architecture & Design Decisions, Data Structures and the assumption log.

## Out of Scope

- **Token counts, model time and cost.** The logs do not hold them (spec Non-Goals). The per-step table leaves out the investigation's model-time column, and `command_ms` covers only Spektacular's own commands, not every tool.
- **A visual or HTML view of a run.** That is spec `000077_html-log-viewer`, which reuses this plan's package and totals.
- **Changing the Harbor test harness.** That is spec `000076_harbor-on-built-in-logs`, which reads this plan's JSON output. No Harbor file changes here.
- **Reports across several projects at once, and live reports of a run as it happens** (spec Non-Goals). A report reads one logs directory as it is when the command runs.
- **Standalone traces as runs.** Commands and spans recorded outside any workflow run are not listed or reported. That includes an epic orchestrator's own `sub-agent` spans, which 000074 records in a standalone trace. If 000076 needs orchestrator hand-off gaps from them, it extends the report.
- **`work` spans.** Agent-marked `work` spans appear on the timeline and in gap labels, but get no table or totals of their own. The spec asks only for helper and user waits.
- **Step breakdowns for logs written before 000074.** Command-only logs get totals and gaps, but an empty step and task breakdown.
- **Telling a stopped-and-never-resumed session apart from one still running.** Without workflow state (absent from a copied directory), such a run reads as `in_progress` until it is restarted or overtaken by a later run of the same workflow.
- **Interpolated percentiles, `--last N`, colour in the tables, and an index or cache beside the logs.** These were not asked for, and the last would change the logs directory.
- **Installing the new binary or touching this repository's own `.spektacular/`** (plans-never-change-the-active-install).
