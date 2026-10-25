---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Plan: 000077_html-log-viewer

<!-- Metadata -->
<!-- Created: 2026-10-25T08:53:17Z -->
<!-- Commit: 8e69383 -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Maintainers can open any recorded workflow run as a web page that draws it as a waterfall. The page lays out every step, task, helper agent, wait on the user, other marked work and command on one timeline, nested under what contains it, with failures and long pauses easy to spot. A new form of the run statistics command, `spektacular stats view`, writes that page as a single HTML file. It opens in any browser without a server or network, so it can be kept with a test run or passed to a colleague. Its totals are the same figures `spektacular stats run` reports (spec 000075), so the picture and the numbers always agree.

## Conventions

- **Error messages must describe the problem and suggest remediation**: every `stats view` refusal must be an `output.NewError(...).WithNextAction(...)` with a runnable fix. That covers an `--out` inside the logs directory, an unwritable `--out`, and every selection and directory error reused from 000075.
- **Tests must not depend on order**: the new `cmd` tests drive `runRootCmd`/`resetRootCmd` (the new `--out` flag is reset with the tree) and use fixture logs with fixed timestamps in their own temp directories, never the wall clock.
- **Passing tests are required before calling work done**: the full shuffled Go suite must pass, including 000075's `stats` tests that share the group's flags.
- **A plan never changes the active skills and configuration**: the feature adds a subcommand and a package and touches no template, skill, managed section or settings format. No task installs the binary or touches this repo's `.spektacular/`. Manual checks use the built binary against copied or fixture logs.
- **Store files are written through the CLI**: the logs directory is not a store and is only read. The page is written outside it, never into a store directory.
- **Plans must sketch content structure, not just summarize it (docs repo)**: the new Run viewer page carries a content outline with the verified command, flags and page controls.
- **No em dashes (docs repo)**: all new docs, README and changelog prose avoids em dashes, and so does the page's own visible text.
- **MDX authoring conventions (docs repo)**: the new page is built from `Hero`, `Prose` and `CtaBanner`, with fenced code blocks and no layout HTML in its body.
- **Site layout conventions (docs repo)**: the page composes existing section components and adds no new frame, width or heading size.

## Architecture & Design Decisions

**Shape.** A fifth `stats` form, `spektacular stats view`, writes one recorded run as a self-contained HTML waterfall. It selects the run exactly as `stats run` does. It inherits the `stats` group's persistent flags from 000075 (`--dir`, `--format`, `--name`, `--kind`, `--agent`, the repeatable `--run`, `--gap-threshold`), its directory resolution, selection and error mapping, and its logging-exempt annotation. So rendering a run never writes a log record, and it works on a logs directory copied from anywhere. The page is written to `--out`: a file path, or an existing directory (the default file name goes inside it). The default is `spektacular-run-<first 8 characters of the run id>.html` in the current directory. The command then reports what it wrote through the shared output writer.

Code changes are in the `spektacular` repo:
- a new package `internal/runview` builds the page;
- `cmd/stats.go` gains the subcommand;
- the README lists it.

The `docs` repo gains a "Run viewer" page.

**Go decides the layout; the page only draws.** `internal/runview` turns a `runstats.Run` and its `runstats.Report` into a compact view model. 000075 loads and assembles the run, and `BuildReport` gives the totals and every gap. The model holds:
- the run's identity;
- the report's wall, working, user-wait and helper-wait totals, copied from `Report.Totals` and so equal to `stats run` by construction;
- one row per span and per command record, in tree order with its depth. Rows carry parent, start offset and duration in milliseconds from the run's start (using 000075's effective end for unfinished spans), type, problem flag and details;
- every gap of at least one second (or of the threshold, if that is shorter), with its step and the events either side;
- the default gap threshold from `--gap-threshold`.

The tree follows 000074's parent rules: tasks and task-less steps at the top, steps under their task, commands and agent spans under their step, and agent spans under the span they name. A span whose parent is not in the run goes to the top level. Siblings are ordered by start time.

Strings repeated across rows (names, steps, statuses) go into one string table, and rows are columnar arrays. That keeps a 2,000-span page well under 1 MB. Verbose-level attributes (`spektacular.response`, state snapshots) are never copied in. All of this is pure and is tested in Go. CI runs only Go, with no browser or JavaScript engine, so every layout decision lives where it can be tested.

**One static file, drawn with plain browser features.** The package embeds a page shell, a stylesheet and a script with `go:embed`, following `templates/templates.go`. It splices the model in as JSON produced by `encoding/json`. That escapes `<`, `>`, `&` and the two JavaScript line separators, so a hostile span name cannot close the script element. The script draws the waterfall on a `<canvas>`. A fixed label column with disclosure triangles sits beside a time axis. The script draws only the rows in view, so a 5,000-span run opens fast, and a zoom or selection redraws at most a screenful of rows.

On the page:
- **Types.** Each span type has its own colour. User waits are also hatched, and each one shades a band across the whole timeline height, so they stand apart at any nesting.
- **Problems.** Failed spans (an error status from a helper stopped with an error, or a command with a non-zero exit), interrupted, abandoned and never-closed spans each get a distinct border and glyph. A collapsed row shows the strongest marker among its hidden descendants.
- **Gaps.** Gaps longer than the threshold are drawn as hatched bands with their length. The threshold can be changed on the page, and the gap count and total follow it, matching `stats run` at the same threshold.
- **Zooming.** Drag across the axis to zoom to a range, Ctrl or Cmd with the mouse wheel to zoom at the cursor, or double-click a bar to fit exactly that span. Reset or Escape returns to the whole run.
- **Collapsing.** A triangle collapses or expands a row. There are "expand all" and "collapse to steps" actions, and the page opens collapsed to steps.
- **Details.** Selecting a bar opens a details panel with its name, type, start, duration, status, end reason, agent, workflow, step, task, and for commands the arguments and exit code.
- **Summary.** A side summary lists the longest steps and the longest gaps. Each entry zooms to its span or gap, so the slowest step and the longest stall can be found at a glance (success metric).

Untrusted text is drawn only with `fillText` and `textContent`, never as HTML. The output is deterministic: it has no generation time and no absolute paths. So a page rendered from a copied logs directory is byte-identical to one rendered in the project, and the test for collected logs is exact.

**Why this direction.**
- **Embedding raw records and pairing them in the browser** was rejected. It would repeat 000075's pairing, capping, wait and gap rules in JavaScript, so the totals could drift from the report. It would also carry verbose payloads into the file.
- **An SVG node per span** was rejected because 5,000 spans would make zooming slow.
- **A charting library** was rejected because of the spec's Technical Approach and the 1 MB target.
- **A separate top-level command** was rejected because it would duplicate the `stats` group's flags, helpers and logging exemption.
- **Writing the page into the logs directory** was rejected because 000075 guarantees that reporting never changes that directory. An `--out` that resolves inside it is refused.
- **A JavaScript engine module to unit-test the script** was rejected. The script's interactive behaviour is checked by hand in a browser instead.

See `research.md#alternatives-considered-and-rejected`. Conventions that drive specific choices:
- every refusal names its fix (error-messages-must-suggest-remediation);
- tests use fixed-timestamp fixtures in their own temp directories and go through `runRootCmd`/`resetRootCmd` (tests-must-not-depend-on-order);
- no task installs the binary, touches this repo's `.spektacular/` or changes a template or skill (plans-never-change-the-active-install).

## Component Breakdown

- **Run view package (new, `spektacular` repo).** This package owns turning one run into a page. It has two layers:
  - **View model builder.** It takes a loaded run, asks the run statistics package for its report (with every gap), and builds the compact model the page draws:
    - the run's identity;
    - the report's four totals (the page works out the gap count and total itself, from the gaps);
    - one row per span and per command, in tree order with depth and parent;
    - start offsets and durations in milliseconds;
    - the type and problem flags (failed, interrupted, abandoned, open) of each row;
    - the details shown on selection;
    - every gap down to the embedding floor;
    - the longest steps;
    - the default threshold.

    It deduplicates strings into one table and leaves out verbose attributes. It is pure (no I/O, no clock), so it is tested against hand-built runs.
  - **Page writer.** It joins the embedded page shell, stylesheet and script with the model, serialised as HTML-safe JSON, into one HTML document written to an `io.Writer`. Its output is deterministic: no generation time, no paths.

  The package depends on the run statistics package (000075) for the run, the paired spans and the report, and on the trace log package (000073 and 000074) only for record field and attribute names. Nothing depends on it except the stats command group.
- **Viewer page assets (new, `spektacular` repo, embedded in the run view package).** A page shell, a stylesheet and one plain script, with no external references. The script owns everything interactive:
  - drawing the visible rows on a canvas, with the label column and time axis;
  - the per-type appearance, user-wait bands, problem markers and gap bands;
  - zoom (drag a range, wheel at the cursor, double-click a bar, reset);
  - collapse and expand, including collapse-to-steps and expand-all;
  - selection and the details panel;
  - the gap-threshold control, with the gap count and total following it;
  - the side summary of longest steps and longest gaps.

  It reads the model from the page and never fetches anything. It renders untrusted text only as text.
- **Stats command group (changed, `spektacular` repo; created by 000075).** It gains the `view` subcommand. The subcommand:
  - reuses the group's persistent flags, logs-directory resolution, run selection (as `stats run` selects) and error mapping;
  - adds `--out`;
  - refuses an output inside the logs directory or one it cannot write, each with a next action;
  - writes the page through the run view package;
  - reports the written file, run, row count and byte size, through the shared output writer or as one readable line.

  It inherits the group's logging-exempt marker, so rendering a page never writes a log record. It does not change the other four forms.
- **Run statistics package (reused, unchanged; from 000075).** It loads a logs directory read-only, assembles and selects runs, and builds the report whose totals and gaps the page shows. The viewer passes a gap cap large enough to receive every gap. No exported type changes.
- **Trace log package (reused, unchanged; from 000073 and 000074).** It supplies the record type for command rows and the names of span types, end reasons and attributes.
- **Output writer (reused, unchanged).** It provides the JSON envelope, the `"error": false` discriminant, `--fields` and the error shape.
- **README (changed, `spektacular` repo).** The `stats` command block that 000075 adds gains a `stats view` line.
- **Documentation site (changed, `docs` repo).** A new "Run viewer" page explains rendering a run, choosing it, rendering collected logs, and reading the page: bars and nesting, types, problem markers, gaps and the threshold, zoom, collapse, details, and totals. The Resources navigation links to it after Run statistics, and the Run statistics page points to it.

## Data Structures & Interfaces

**Input (read, unchanged).** The viewer reads nothing new. It takes from 000075's `internal/runstats`:
- a loaded run (`Run`: identity, `Spans`, `Commands`, `Start`, `End`, `Status`, `LogFile`);
- the report `BuildReport(run, Options{Gaps, GapThreshold})` produces (`Totals`, `Gaps`, `StepEntries`), which the builder requests itself;
- the skipped-line count of the `Logs`.

Command rows read these fields of 000073's `tracelog.Record`: `span_id`, `parent_span_id`, `name`, `start_time`, `end_time`, `status`, and the attributes `process.command_args` and `process.exit_code`. No record field, report field or `stats` JSON field changes.

**Command.**

```text
spektacular stats view [selection] [--gap-threshold 5m] [--out <file or directory>] [--dir <path>] [--format pretty|json]

selection:  --name <workflow name>   --kind spec|plan|implement|repo   --agent <agent>
            --run <run id or unique prefix of 8+ hex characters>   (repeatable; the first is rendered)
```

- The selection, `--dir`, `--format` and `--gap-threshold` are the `stats` group's persistent flags from 000075, with the same defaults and validation.
- `--out` defaults to `spektacular-run-<first 8 characters of the run id>.html` in the current directory. If it names an existing directory, the default file name is used inside it.
- `--schema` is supported, and the JSON form honours the global `--fields`.

**Command result.** JSON uses the standard envelope; pretty prints one line.

```json
{"log_dir":"/abs/.spektacular/logs","skipped_lines":0,"gap_threshold_ms":300000,
 "run":{"run_id":"4bf92f3577b34da6a3ce929d0e0e4736","kind":"plan","name":"000075_run-stats-command",
        "agent":"claude","log_file":"...","started_at":"...","ended_at":"...","wall_ms":2481277,"status":"finished"},
 "file":"/abs/work/spektacular-run-4bf92f35.html","rows":1234,"bytes":412880,"error":false}
```

```text
Wrote plan 000075_run-stats-command (run 4bf92f35, 1234 spans) to /abs/work/spektacular-run-4bf92f35.html
```

`run` is 000075's run summary shape, unchanged. `rows` is the number of bars on the page, one per span and command record. `bytes` is the file size.

**New errors** (standard error envelope, each with a `next_action`). Every 000075 `stats` error code applies unchanged, for example `stats_no_runs`, `stats_run_not_found` and `stats_log_dir_not_found`.

| Code | When | Next action |
|---|---|---|
| `stats_view_out_in_logs` | `--out` resolves inside the logs directory being read | pass `--out` with a path outside the logs directory, or omit it to write to the current directory |
| `stats_view_write_failed` | the file cannot be created or written (missing parent, permission) | check the directory exists and is writable, or pass `--out <writable path>` |

**Page data (embedded JSON, the contract between the Go builder and the page script).** It is one object in a `<script type="application/json" id="run-data">` element. Rows and gaps are columnar: each key holds one array, and index `i` across the arrays is row `i`. Strings that repeat (names, steps, end reasons, messages, task ids, command lines, gap labels) are indexes into `strings`, and `-1` means none.

```json
{"version":1,
 "run":{"run_id":"4bf9...","kind":"plan","name":"000075_run-stats-command","agent":"claude",
        "status":"finished","started_at":"2026-10-25T08:20:50.123Z","ended_at":"...","skipped_lines":0},
 "totals":{"wall_ms":2481277,"working_ms":1803000,"user_wait_ms":678277,"helper_wait_ms":512000},
 "gap_threshold_ms":300000,"gap_floor_ms":1000,
 "types":["command","step","task","sub-agent","user-wait","work"],
 "problems":["","failed","interrupted","abandoned","open"],
 "strings":["discovery","plan goto","moved","..."],
 "rows":{"parent":[-1,0,0],"depth":[0,1,1],"start_ms":[0,1200,5400],"duration_ms":[812000,24,150000],
         "type":[1,0,3],"name":[0,1,7],"problem":[0,0,0],"end_reason":[2,-1,9],"status":[0,0,0],
         "message":[-1,-1,-1],"step":[0,0,0],"task_id":[-1,-1,-1],"span_id":["a3ce929d0e0e4736","...","..."],
         "args":[-1,12,-1],"exit_code":[null,0,null]},
 "gaps":{"start_ms":[1201000],"duration_ms":[1200000],"step":[4],"after":[15],"before":[16]},
 "longest_steps":[0,17,42]}
```

- **`rows` are in tree order** (each parent before its children, siblings by start time), so the script draws a collapse state by walking the arrays once. `depth` is the nesting level, and `parent` is a row index or `-1`.
- **Times.** `start_ms` is the offset from the run's start, and `duration_ms` runs to the effective end. `status` is `0` (ok), `1` (error) or `-1` (none, for an open span).
- **Problems.** `problem` indexes `problems`. `failed` means an error status on a stopped agent span, or a command with a non-zero exit. `interrupted` and `abandoned` come from the end reason. `open` means never closed.
- **Gaps.** `gaps` holds every gap of at least `gap_floor_ms`, which is min(1 s, `--gap-threshold`). `after` and `before` are labels such as `end of command plan goto`.
- **`longest_steps`** holds the row indexes of the five longest step entries.
- **`totals`** are the report's figures, copied, not recomputed. The page works out the gap count and total for the current threshold from `gaps`, by the report's rule (gaps longer than the threshold).

**Go interfaces (`internal/runview`, new).**

```go
type Options struct {
    GapThreshold time.Duration // the page's default threshold (from --gap-threshold)
    Skipped      int           // Logs.Skipped, shown on the page
}
// Build is pure: it builds the run's report with runstats.BuildReport, passing a
// gap cap large enough to receive every gap, and returns the page data.
func Build(r *runstats.Run, o Options) Model
// Decode reads the page data back out of a written page (used by tests and tools).
func Decode(page []byte) (Model, error)
// Write writes one self-contained HTML document: the embedded shell, stylesheet
// and script, with m serialised as HTML-safe JSON. Output is deterministic.
func Write(w io.Writer, m Model) error
func DefaultFileName(runID string) string // "spektacular-run-<id[:8]>.html"
func GapFloor(threshold time.Duration) time.Duration // min(time.Second, threshold)

// Model mirrors the page data above, with matching json tags.
type Model struct {
    Version        int      `json:"version"`
    Run            RunInfo  `json:"run"`
    Totals         Totals   `json:"totals"`
    GapThresholdMS int64    `json:"gap_threshold_ms"`
    GapFloorMS     int64    `json:"gap_floor_ms"`
    Types          []string `json:"types"`
    Problems       []string `json:"problems"`
    Strings        []string `json:"strings"`
    Rows           Rows     `json:"rows"`
    Gaps           Gaps     `json:"gaps"`
    LongestSteps   []int    `json:"longest_steps"`
}
```

`cmd` gains one flag, `--out`, on the new subcommand, and two error codes. No setting, record field, state field, existing type or existing JSON shape changes.

## Implementation Detail

**A presentation package on top of the read side.** 000075 adds `internal/runstats` as the read side of the logs. This plan adds one more layer above it, `internal/runview`, that turns a run into a page. The dependency runs one way: `tracelog` to `runstats` to `runview` to `cmd`. Neither lower package learns about HTML.

A developer opening `runview` finds:
- a builder, from run and report to model, which is pure and table-tested on hand-built runs;
- a writer, from model to HTML bytes;
- an `assets` folder holding the shell, stylesheet and script as ordinary files that can be opened and edited in an editor.

This mirrors 000075's split between pure analysis and a renderer writing to an `io.Writer`. It also mirrors the existing `go:embed` pattern used for templates.

**Embedding a front end in the binary is new to the codebase.** Until now the binary embedded only text templates rendered with mustache. Here the embedded files are a static page shell, CSS and plain JavaScript. They have no build step, no bundler, no npm dependency and no external URL, so the Go build stays the only build. The writer does not use a template engine. It replaces three fixed markers in the shell: the stylesheet, the script and the JSON data. That keeps escaping explicit and easy to review:
- CSS and JS are trusted files from the binary;
- the data is `encoding/json` output with its default HTML-safe escaping.

A test pins that the page has no `src=`, `href=` or `url(` pointing outside the document, so self-containment cannot regress silently.

**The page data is a small columnar contract.** Rows are arrays keyed by field, with shared strings in one table, instead of an array of objects. A 2,000-span page is then a few hundred kilobytes rather than megabytes. The script can also walk rows by index without allocating an object per span.

The contract is versioned (`version: 1`) and pinned by a golden test on a fixture run. A change to the builder that would break the script fails a Go test first.

Every rule about what a bar means lives in the builder, which tests cover:
- tree order;
- what counts as failed, interrupted, abandoned or open;
- effective ends;
- the gap floor;
- which steps are longest.

The script never re-derives a figure. It only draws what it is given and applies the gap threshold to the gaps it was handed.

**The script is organised for speed at thousands of rows.** It keeps three pieces of state:
- a zoom window (start and end in ms);
- a set of collapsed rows;
- the selected row.

From the collapsed set and the tree order it derives the list of visible rows. On each frame it draws only those inside the scroll viewport, at a fixed row height, so the cost of a redraw does not grow with the run's size. Hit-testing for selection is arithmetic on the row index and the x-scale, with no per-bar objects. Text appears only through `fillText` and `textContent`. All visible strings are fixed English labels, in the same plain style as the CLI's own messages.

**The command follows the `stats` group's pattern.** `stats view` is one more subcommand built like 000075's: shared persistent flags, the same selection and error mapping, and a JSON result through the shared writer or one readable line. The one new behaviour is writing a file. It goes to a temporary file in the target directory, which is then renamed into place, the same pattern 000073 uses for state files, so an interrupted write never leaves half a page. An output path inside the logs directory is refused before anything is written. The group's logging exemption covers the subcommand without new code, so the rule that a read-only log command is never logged holds for it too.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports none).
- **Upstream spec `000075_run-stats-command` must land first.** It is planned but not implemented. This plan builds on:
  - `internal/runstats`: `Load`, `Logs.Candidates`, `ForReport`, `Run`, `Span`, `BuildReport`, `Options`, `Report.Totals` and `Report.Gaps`;
  - the `stats` command group: its persistent flags, logs-directory resolution, filter building, error mapping, run summary shape and `--schema` pattern;
  - its logging-exempt annotation on the group.

  No change to any of them is needed. No task starts until 000075's Milestone 1 is merged. If its exported names differ, adapt to them. If a figure's meaning or the exemption's behaviour differs from its plan, STOP and ask.
- **Upstream spec `000074_trace-spans-for-agent-work` must land first.** It is planned but not implemented. It provides:
  - the span types, end reasons and their statuses;
  - the parent rules the tree follows;
  - task-span ids;
  - the instant terminal step span.

  The viewer reads them only through `runstats`, and through `tracelog`'s constants.
- **Upstream spec `000073_log-mode-and-logger-providers` must land first.** It is planned but not implemented. It provides the record format and `tracelog.Record`, the command attributes (`process.command_args`, `process.exit_code`), the log directory and `config.LogDir`. Verbose-only attributes are deliberately not read.
- **Sibling spec `000076_harbor-on-built-in-logs`: independent.** It neither reads nor changes the viewer, and the viewer changes nothing it reads.
- **`internal/output` (used, unchanged).** It provides the success envelope, `--fields`, and `NewError(...).WithNextAction` for the two new refusals.
- **`cmd` (changed).** `cmd/stats.go`, created by 000075, gains the `view` subcommand and its `--out` flag. Nothing else in `cmd` changes.
- **Go standard library only** (`embed`, `encoding/json`, `io`, `os`, `path/filepath`, `sort`, `time`), plus the existing `cobra`. There is no new module, and the page uses no third-party JavaScript or CSS.
- **Documentation site, `docs` repo (changed).** A new page `src/pages/run-viewer.mdx`, a Resources nav entry in `src/components/Nav.astro` after 000075's "Run statistics", and one sentence on 000075's `src/pages/run-stats.mdx`. These are checked by the site build and `astro check`.
- **`README.md` (changed).** One line in the `stats` block that 000075 adds.
- **Reference material (read, unchanged).** `docs/workflow-performance-investigation.md` defines the gaps and stalls that the success metric asks a maintainer to find on the page.

## Testing Approach

**Test types and where they slot in.** Every Go test runs under the project's shuffled, order-independent `go test`. No test reads the wall clock: fixtures are hand-built `runstats` runs, or JSON Lines files with fixed timestamps, in each test's own temp directory. CI runs only Go, with no browser or JavaScript engine. So everything that decides what the page shows is computed and tested in Go, and the script's drawing and interaction are checked by hand.

- **Unit tests in `internal/runview` (the builder).** These carry the most coverage, because every rule about what a bar means lives here. They are table-driven over hand-built runs:
  - **Rows:**
    - one row per span and per command record;
    - each row's offset and duration equal the recorded start and end, with the effective end for open and abandoned spans;
    - tree order and depth follow the parent rules (task, then step, then command or agent span, with nested agent spans);
    - a span whose parent is missing goes to the top level;
    - siblings are ordered by start time;
    - the instant terminal step has zero duration.
  - **Types and problems:**
    - each of the six span types maps to its type index;
    - a helper stopped with an error and a command with a non-zero exit are `failed`;
    - interrupted, abandoned and never-closed spans are flagged as such;
    - finished work is unflagged.
  - **Details:** a helper row carries its name, type, start, duration, status, end reason, step and task. The run carries agent and workflow. A command row carries its command line and exit code. Verbose attributes never appear in the model.
  - **Totals:** wall, working, user-wait and helper-wait equal the `BuildReport` totals for the same run.
  - **Gaps:** a run with a 20-minute and a 2-minute pause yields both gaps with their steps and labels. Applying the report's rule to the model's gaps marks one at the 5-minute default and both at 1 minute. Gaps under the floor are dropped. The default threshold is the one passed in.
  - **Longest steps:** the five longest step entries, longest first.
- **Page-writer tests:**
  - the page is one document whose embedded JSON parses back to the model;
  - it has no `src=`, `href=`, `url(` or `@import` pointing outside the document, so it works offline and without a server;
  - a span named `</script><script>alert(1)</script>` appears only escaped;
  - rendering the same model twice gives identical bytes.
- **Contract test.** A golden page-data JSON for one fixture run pins every field name and the columnar shape the script reads, so a builder change that would break the page fails a Go test first.
- **Command-level integration tests in `cmd`**, through `runRootCmd`/`resetRootCmd`, against a fixture project whose logs directory is seeded with fixture runs:
  - with no selection, the page is written for the most recent run, to the default file name in the working directory;
  - each selection (`--name`, `--run` full and prefix, `--kind`, `--agent`) renders the run that `stats run` reports for the same flags;
  - `--out` as a file path and as a directory;
  - the JSON result in the standard envelope, honouring `--fields`, and the pretty line;
  - the page's totals equal `stats run --format json` totals for the same run;
  - `--gap-threshold 1m` becomes the page's default threshold;
  - a page rendered from a `cp -r` copy with `--dir`, outside any project, is byte-identical to the page rendered in the project;
  - every log file is byte-identical before and after `stats view` in a project with logging on, which proves the inherited exemption;
  - an `--out` inside the logs directory is refused and writes nothing;
  - an unwritable `--out` is refused;
  - every refusal carries a next action.
- **Docs repo.** The site build and `astro check` pass, and the MDX guard returns nothing.

**Load-bearing guarantees.**
- Every bar is a recorded span at its recorded times, nested under its parent.
- The page's totals are the report's totals.
- Problems are flagged by one rule in one place.
- The page is a single file with no external reference, and untrusted names cannot break out of it.
- Rendering never touches the logs.
- The page is identical whether rendered in the project or from a copy.

**Deliberate gaps.**
- No automated test of the script's drawing, zoom, collapse or selection, because CI has no browser or JS engine and adding a JS engine module only for tests was rejected. Those are manual checks below.
- No automated wall-clock assertion, because timing thresholds are flaky in CI. A Go benchmark that builds and writes a generated 5,000-span page supports the speed criterion on the Go side.

**Success metrics.**
- *Timed on a sample implement run after delivery, the maintainer finds the slowest step and the longest stall from the page in under a minute, without reading the log.* **Manual — captured in the implementation test plan.** Supporting behavioural test: the model's `longest_steps` lists the slowest step entry first, and the model's gaps include the run's longest stall, so the side summary can name both.
- *The page for a run of about 2,000 spans is under 1 MB.* **Behavioural test:** a generated 2,000-span run, with realistic names, command lines, helpers and gaps, renders to a page under 1,000,000 bytes, and the test reports the size.

**Spec acceptance criteria checked by hand.** Each is **Manual — captured in the implementation test plan**:
- *Types and problems are distinct:* on a fixture run with every span type, a failed helper, an interrupted step and an abandoned span, each type looks different, user waits stand apart, and the three problems are each marked.
- *Selecting shows details:* selecting a helper bar shows its name, type, start, duration, status, agent, workflow and step.
- *A long gap is visible:* the 20-minute pause is marked at the default threshold and the 2-minute one is not; with the page's threshold set to 1 minute, both are marked.
- *Zoom and collapse work:* zooming into one step fills the width with that period, and collapsing a step hides its children until expanded.
- *The page works offline:* the file opens and renders fully with networking disabled and no server.
- *A large run stays responsive:* a generated 5,000-span page renders in under 3 seconds, and a zoom or selection responds in under 200 ms, in current Chrome and Firefox.

**Manual reviews.**
- **Manual — captured in the implementation test plan:** a visual review of the page in current Chrome and Firefox on a real implement run: legend, colours, user-wait bands, problem markers, axis labels, the details panel and the side summary are legible, and the visible text has no em dashes.
- **Manual — captured in the implementation test plan:** read the Run viewer page on the built site, checking the command, flags and every page control against the shipped behaviour.

## Milestones & Tasks

### Milestone 1: Maintainers can open any recorded run as a waterfall page

**What changes**: `spektacular stats view` writes one recorded run as a single HTML file that opens in any browser with no server and no network. By default it renders the latest run. It can also pick a run by workflow name, run id, kind or agent, exactly as `stats run` does, and it reads a logs directory collected from elsewhere with `--dir`. The page draws every step, task, helper agent, wait on the user, other marked work and command as a bar on one timeline, placed at its start, sized by its duration, and nested under what contains it:
- each span type has its own look, and waits on the user are hatched and shaded across the whole timeline;
- failed, interrupted, abandoned and never-closed spans are each marked;
- gaps longer than the threshold (5 minutes by default, or `--gap-threshold`) are drawn as marked bands;
- the header shows the run's wall, working, user-wait and helper-wait times, the same figures `stats run` gives;
- selecting a bar shows its details.

Rendering a page never changes the logs.

**Validation point**: The builder, page-writer and `cmd` tests pass in the full shuffled Go suite. They cover one bar per span at its recorded times, nesting, types, problem flags, details, totals equal to the report, gaps, the default and selected runs, a copied logs directory giving a byte-identical page, no external references, a 2,000-span page under 1 MB, and byte-identical logs after rendering. Opening a page rendered from a fixture run in a browser with networking off shows the waterfall, the totals and a details panel on selection.

#### - [x] Task: Build the page data from a run
**Id:** 9d0f8c1e-ed99-4f76-9d06-ca3b849a600c
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds the run view package's builder, which turns one recorded run into the compact data the page draws:
- one row per span and command, nested under its parent and placed at its recorded times;
- each row's type, its problem marker and its details;
- every gap worth showing;
- the longest steps;
- the run's totals, taken from the run statistics report.

It is pure and tested in Go, so every rule about what a bar means is pinned before any drawing exists. It starts only once spec 000075's run statistics package is merged.

*Technical detail:* [context.md#task-build-the-page-data-from-a-run](./context.md#task-build-the-page-data-from-a-run)

**Acceptance criteria**:
- [x] A run gives exactly one row per recorded span and command. Each row starts and ends at the span's recorded times, with unfinished spans ending where they were cut off, and sits under its parent: tasks at the top, steps under their task, commands and helpers under their step.
- [x] Each span type gets its own type. A helper stopped with an error and a command that failed are marked failed. Interrupted, abandoned and never-closed spans are each marked as such, and finished work is unmarked.
- [x] A helper's row carries its name, type, start, duration, status, end reason and step, and the run carries its agent and workflow. Full command output and state snapshots are never included.
- [x] The wall, working, user-wait and helper-wait times equal the run statistics report's figures for the same run.
- [x] A run with a 20-minute and a 2-minute pause carries both gaps, each with its step. By the report's rule one is over the default 5-minute threshold, and both are over a 1-minute one.
- [x] The data's field names and shape match the plan's Data Structures section, and a fixture run's data is pinned so a change to it is noticed.

#### - [x] Task: Write the run as a self-contained page
**Id:** 4b33b999-493b-46d6-b081-f758f3907ce1
**Repo:** spektacular
**Depends on:**
- 9d0f8c1e-ed99-4f76-9d06-ca3b849a600c — Build the page data from a run
**Execution:** agent

Adds the page itself: an embedded page shell, stylesheet and script, joined with the run's data into one HTML file. In the browser the page:
- shows the run's identity and its four totals;
- draws every span as a bar on one timeline, nested under its parent, with each type in its own look;
- hatches waits on the user and shades them across the whole timeline;
- marks failed, interrupted, abandoned and never-closed spans;
- draws gaps longer than the threshold as marked bands;
- shows a span's details when its bar is selected.

The file needs no server and no network, and a hostile span name cannot break it.

*Technical detail:* [context.md#task-write-the-run-as-a-self-contained-page](./context.md#task-write-the-run-as-a-self-contained-page)

**Acceptance criteria**:
- [x] The page is one file with no reference to anything outside it, and the run data inside it reads back exactly as built.
- [x] A span whose name contains HTML or a closing script tag appears only as text and cannot change the page.
- [x] Writing the same run twice gives identical files, with no generation time and no file paths in them.
- [x] The page for a generated run of about 2,000 spans is under 1 MB.
- [x] In a browser with networking off, the page shows the totals and a bar for every span, each type looks different, user waits stand out across the timeline, each problem is marked, gaps over the threshold are marked, and selecting a bar shows its details.
- [x] Sample pages for the manual checks (every span type with each problem, a 20-minute and a 2-minute pause, and runs of 2,000 and 5,000 spans) can be generated on request from the test suite.

#### - [x] Task: Render a run from the command line
**Id:** d3b05758-9f66-45e6-a5b8-591be850bed5
**Repo:** spektacular
**Depends on:**
- 9d0f8c1e-ed99-4f76-9d06-ca3b849a600c — Build the page data from a run
- 4b33b999-493b-46d6-b081-f758f3907ce1 — Write the run as a self-contained page
**Execution:** agent

Adds `spektacular stats view`. It picks a run exactly as `stats run` does, the latest by default, and writes its page to the current directory, or to the file or directory given with `--out`. It reads the project's logs or a directory given with `--dir`, and takes the gap threshold as the page's default. It reports what it wrote as a readable line or as the standard JSON envelope. Like every `stats` form it is never logged, and it refuses to write into the logs directory it reads.

*Technical detail:* [context.md#task-render-a-run-from-the-command-line](./context.md#task-render-a-run-from-the-command-line)

**Acceptance criteria**:
- [x] With no selection, a page for the most recent run is written to a default file name in the current directory. Selecting by workflow name, run id, kind or agent renders the run `stats run` reports for the same selection.
- [x] `--out` writes to the named file, or uses the default file name inside a named directory.
- [x] The result names the file, the run, the number of bars and the file size, as a readable line or in the standard JSON envelope, honouring `--fields`.
- [x] The page's totals equal `stats run`'s for the same run, and `--gap-threshold` sets the page's default threshold.
- [x] A page rendered from a copy of the logs directory, from outside any project, is identical to the page rendered in the project.
- [x] Every log file is byte-identical before and after `stats view` in a project with logging on.
- [x] An output path inside the logs directory, or one that cannot be written, is refused with a message naming the fix, and nothing is written.

### Milestone 2: Maintainers can explore large runs on the page

**What changes**: The page becomes a tool for finding where a run stalled. Users can:
- zoom into any part of the timeline, by dragging across the axis, by scrolling with Ctrl or Cmd held, or by double-clicking a bar to fit exactly that span, and reset to the whole run;
- collapse or expand any span's children; the page opens collapsed to steps, a collapsed row still shows any problem hidden inside it, and one action expands everything;
- change the gap threshold on the page, with the marked gaps and the gap count and total following it;
- use a side summary listing the longest steps and the longest gaps, each of which zooms to its place on click.

A run of several thousand spans stays quick to open and to zoom or select, because the page draws only the rows in view.

**Validation point**: A Go benchmark builds and writes a generated 5,000-span page, and the full Go suite passes. In current Chrome and Firefox, that page opens in under 3 seconds, and a zoom, a collapse or a selection responds in under 200 ms. Zooming into a step fills the width with that step. Collapsing a step hides its children until expanded. With the threshold set to 1 minute, a 2-minute pause is marked as well as a 20-minute one.

#### - [x] Task: Zoom, collapse and summarise on the page
**Id:** 4940ba43-261c-47ea-b214-ba19f8235260
**Repo:** spektacular
**Depends on:**
- 4b33b999-493b-46d6-b081-f758f3907ce1 — Write the run as a self-contained page
**Execution:** agent

Makes the page a tool for finding stalls. Users can:
- zoom by dragging across the axis, by scrolling with Ctrl or Cmd held, or by double-clicking a bar to fit that span, and reset to the whole run;
- collapse or expand any row's children; the page opens collapsed to steps, with hidden problems still shown on the collapsed row, and can expand everything at once;
- change the gap threshold on the page, with the marked gaps, count and total following it;
- use a side summary listing the longest steps and longest gaps, each of which zooms to its place.

Drawing stays limited to the rows in view, so runs of thousands of spans stay quick.

*Technical detail:* [context.md#task-zoom-collapse-and-summarise-on-the-page](./context.md#task-zoom-collapse-and-summarise-on-the-page)

**Acceptance criteria**:
- [x] Zooming into one step shows only that period across the full width of the timeline, and reset returns to the whole run.
- [x] Collapsing a step hides its children until it is expanded again. A collapsed row shows any problem among its hidden children. The page opens collapsed to steps, and one action expands every row.
- [x] With the page's threshold set to 1 minute, a 2-minute pause is marked as well as a 20-minute one, and the gap count and total change to match.
- [x] The side summary lists the longest steps and the longest gaps, and choosing one zooms to it.
- [x] Building and writing a generated 5,000-span page is measured by a benchmark in the Go suite, and that page opens and responds to zooming and selection without noticeable delay in a current browser.

### Milestone 3: The run viewer is documented

**What changes**: The documentation site gains a "Run viewer" page, linked from the Resources menu after Run statistics and from the Run statistics page. It explains how to render a run, how to choose it, how to render collected logs, and how to read the page: bars and nesting, span types, problem markers, gaps and the threshold, zooming, collapsing, the details panel, the totals and the side summary. The README's `stats` commands gain `stats view`.

**Validation point**: The site builds and `astro check` passes, with no layout HTML in the page body and no em dashes in the new text. The README shows `stats view`.

#### - [x] Task: Documentation site describes the run viewer
**Id:** 139a70b2-621d-4bc9-872d-f40a2e22dc21
**Repo:** docs
**Depends on:**
- d3b05758-9f66-45e6-a5b8-591be850bed5 — Render a run from the command line
- 4940ba43-261c-47ea-b214-ba19f8235260 — Zoom, collapse and summarise on the page
**Execution:** agent

Adds a "Run viewer" page to the documentation site. It explains how to render a run, choose it and render collected logs, and how to read the page: bars and nesting, span types, problem markers, gaps and the threshold, zooming, collapsing, details, totals and the side summary. The Resources menu links to it after Run statistics, and the Run statistics page points to it.

**Content outline** (new page, in order; the wording is illustrative, but the command, flags, defaults, file name and page controls are fixed):
1. Hero: heading "Run viewer", sub "See a recorded run as a waterfall: every step, task, helper and wait on the user, on one timeline."
2. `## Rendering a run`: `spektacular stats view` writes the latest run to a single HTML file and prints where. Example block:
   ```text
   $ spektacular stats view
   Wrote plan 000075_run-stats-command (run 4bf92f35, 1234 spans) to /home/me/project/spektacular-run-4bf92f35.html
   ```
   Open the file in any browser. It needs no server and no network, so it can be attached to a bug or kept with a test run. `--out report.html` or `--out some/dir/` chooses where it goes. It is never written into the logs directory.
3. `## Choosing the run`: the same selection as `stats run`: `--name <workflow name>`, `--kind spec|plan|implement|repo`, `--agent <agent>`, `--run <id or 8+ character prefix>`. The latest matching run is drawn. `spektacular stats list` shows the runs and their ids.
4. `## Logs from elsewhere`: `spektacular stats view --dir path/to/collected/logs` works from any folder. The page is identical to one rendered in the project. Rendering never changes a log and is never logged.
5. `## Reading the waterfall`: time runs left to right from the run's first event. Each bar is one span, indented under the span that contains it: tasks, then steps, then the commands, helper agents and waits inside them. A table of types and their look:
   - Task: purple bar.
   - Step: blue bar.
   - Helper agent (`sub-agent`): teal bar.
   - Wait on the user (`user-wait`): amber hatched bar, with an amber band across the whole timeline.
   - Other marked work (`work`): olive bar.
   - Command: thin slate bar.
6. `## Problems`: failed (red border, ✕), interrupted (orange dashed border, ‖), abandoned (grey dotted border, ∅) and never closed (fading end, …). An interrupted or unfinished bar ends where the span was cut off. A collapsed row shows the strongest problem hidden inside it.
7. `## Gaps`: a stretch with no recorded event longer than the threshold is a hatched band labelled with its length. The threshold defaults to 5 minutes, or `--gap-threshold 10m`, and can be changed in the page's Gap threshold box. The gap count and total follow it and match `stats run` at the same threshold.
8. `## Zooming and collapsing`: drag across the time axis to zoom to a range; hold Ctrl (Cmd on macOS) and scroll to zoom at the pointer; double-click a bar to fit that span; Reset zoom or Escape shows the whole run. The triangle beside a row collapses or expands it. The page opens collapsed to steps; Expand all and Collapse to steps switch between the two.
9. `## Details, totals and the summary`: selecting a bar shows its name, type, start, duration, status, end reason, agent, workflow, step and task, and for a command its command line and exit code. The header shows wall time, working time, user wait and helper wait, the same figures as `stats run` (defined on the Run statistics page). The side summary lists the longest steps and the longest gaps; choosing one zooms to it.
10. `## For programs`: `--format json` reports `{"file":"/abs/spektacular-run-4bf92f35.html","rows":1234,"bytes":412880,"run":{"run_id":"...","kind":"plan"},"gap_threshold_ms":300000,"log_dir":"...","skipped_lines":0,"error":false}`.
11. CtaBanner: "Want the numbers instead?" with a button to the Run statistics page.

*Technical detail:* [context.md#task-documentation-site-describes-the-run-viewer](./context.md#task-documentation-site-describes-the-run-viewer)

**Acceptance criteria**:
- [x] The Run viewer page explains rendering a run, choosing it, rendering collected logs, and reading every part of the page: types, problems, gaps and the threshold, zoom, collapse, details, totals and the summary.
- [x] The Resources menu links to the page after Run statistics, and the Run statistics page points to it.
- [x] The site builds and type-checks without errors, and the page body has no layout HTML and no em dashes.

#### - [x] Task: README lists stats view
**Id:** 586b4c2b-2f35-41c6-b311-d6d25285fc47
**Repo:** spektacular
**Depends on:**
- d3b05758-9f66-45e6-a5b8-591be850bed5 — Render a run from the command line
**Execution:** agent

Adds `stats view` to the README's `stats` commands, with one line saying it writes a run as a self-contained HTML waterfall, and a pointer to the documentation site's Run viewer page.

**Content example** (README "How It Works", appended to the `stats` block 000075 adds):
```text
spektacular stats view [--name <spec>]     # the run as a single-file HTML waterfall you can open or share
```

*Technical detail:* [context.md#task-readme-lists-stats-view](./context.md#task-readme-lists-stats-view)

**Acceptance criteria**:
- [x] The README shows `stats view` beside the other `stats` commands, with a one-line description and a pointer to the Run viewer documentation.

## Open Questions

- **Whether 000075 lands with the API and behaviour this plan builds on.** That depends on 000075's implementation. The plan uses:
  - `runstats.Load`, `Logs.Candidates`, `ForReport`, `Run` with `Spans` and `Commands`, `Span`'s effective end and `Complete`, `BuildReport` with a gap cap, `Report.Totals`, `Report.Gaps` and `(*Run).Summary()`;
  - the `stats` group's persistent flags and its `statsLogs`, `statsFilter` and `statsError` helpers;
  - the logging-exempt annotation on the group, inherited by children.

  If only names differ, adapt to them. If a figure's meaning, the gap list's contents or the exemption's reach differs from 000075's plan, STOP and ask the user, because the page's totals and the read-only guarantee rest on them.
- **Whether canvas drawing of only the visible rows meets the 5,000-span targets (under 3 s to open, under 200 ms per zoom or selection) in real browsers.** That depends on browser behaviour, which only the manual check can measure. If it misses, internal script changes are fine: an offscreen canvas for the label column, coarser hit-testing, or drawing commands at lower detail when zoomed out. Adding a library, a server or a second file is not, and needs the user's say (spec Technical Approach and requirement "The page works on its own").

No other implementation-time uncertainties remain. Every design choice is settled in Architecture & Design Decisions, Data Structures and the assumption log.

## Out of Scope

- **A live view of a run as it happens, or a server that hosts pages** (spec Non-Goals). The page is a snapshot of the logs at the moment `stats view` runs. Re-run it to refresh.
- **Several runs on one page, or comparing runs visually** (spec Non-Goals). `stats compare` and `stats summary` (spec 000075) cover comparison as numbers.
- **Tokens, models or cost** (spec Non-Goals). The logs do not hold them.
- **Editing or annotating runs from the page** (spec Non-Goals). The page is read-only and saves nothing.
- **Opening the page in a browser automatically, or a stdout form of the page.** Neither was asked for. The command prints the file's path.
- **Standalone traces** (commands run outside any workflow, including an epic orchestrator's own spans). The viewer renders workflow runs as 000075 defines them, and those traces are not runs there.
- **Full command output and state snapshots on the page.** Verbose-level attributes are left out to keep the file small. The details panel shows the command line and exit code only.
- **Changing the run statistics package, its JSON output, or the Harbor harness.** The viewer reads 000075's package as it is. Harbor on the built-in logs is spec 000076.
- **Automated browser tests of the page's drawing and interaction.** CI has no browser or JavaScript engine. These are manual checks in the test plan.
- **Installing the new binary or touching this repository's own `.spektacular/`** (plans-never-change-the-active-install).

## Changelog

### 2026-10-25 — Task: Build the page data from a run

**What was done**: Added the `internal/runview` package's builder. `Build` turns one `runstats.Run` into the columnar page `Model`: one row per span and command in tree order with depth and parent, offsets and effective-end durations in ms, type and problem indexes, details (name, end reason, status, message, step, task id, span id, command line cut to 2,000 bytes, exit code), every report gap of at least `GapFloor` with its step and edge labels, the five longest step entries, and the four totals copied from `runstats.BuildReport`. A golden file pins the contract.

**Deviations**: `BuildReport` is called with `Options{Gaps: 0}`, which in 000075's implementation already means "every gap", instead of the plan's `math.MaxInt32`. A span type the page does not know is drawn as `work`. A command with no recorded exit code gets 1 when its status is an error and 0 otherwise, as `runstats`' command listing does. Nodes caught in a parent cycle are placed at the top level, so every row still appears once.

**Files changed**:
- `spektacular: internal/runview/doc.go`
- `spektacular: internal/runview/model.go`
- `spektacular: internal/runview/build.go`
- `spektacular: internal/runview/build_test.go`
- `spektacular: internal/runview/testdata/model.golden.json` (regenerate with `go test ./internal/runview -run TestModelGolden -update`)

**Discoveries**: A row's step is the span's own `spektacular.step` attribute. The run report falls back to the enclosing step entry for a helper or wait recorded without one (`waitRow`), so the page shows no step for such a span. `trace start` stamps the current step on every agent span (`cmd/trace.go`), so this only affects a span started with no step in progress; left as recorded. `runstats`' `attr`/`intAttr`/`stringsAttr` helpers are unexported, so `runview` keeps small copies.

### 2026-10-25 — Task: Write the run as a self-contained page

**What was done**: Added the embedded page (`assets/page.html`, `viewer.css`, `viewer.js`, embedded by `assets.go`) and `Write`/`Decode` in `write.go`. `Write` splices the stylesheet, the script and the `encoding/json` model into the shell's three markers in one `strings.Replacer` pass, refusing a shell whose markers are missing or repeated and assets that contain `</style` or `</script`. The script draws the visible rows on one sticky canvas inside a scrolling pane (header with identity and the four totals, legend, time axis, label column with problem glyphs, per-type bars, hatched user waits with full-height bands, problem borders, hatched gap bands labelled with their length, selection by click or arrow keys with a details panel filled through `textContent`). Tests cover the round trip, self-containment, hostile names, determinism, markers, the 2,000-span size (210,700 bytes) and the `-sample-dir` sample pages.

**Deviations**: Gap labels are drawn at the foot of the view, over the bars, rather than at the top, where the first row's bar hid them. The details panel also shows a Problem line, and arrow keys move the selection. The test author added `BenchmarkBuildAndWrite2000` beside the size test; Milestone 2 adds the 5,000-span benchmark.

**Files changed**:
- `spektacular: internal/runview/assets.go`
- `spektacular: internal/runview/write.go`
- `spektacular: internal/runview/assets/page.html`
- `spektacular: internal/runview/assets/viewer.css`
- `spektacular: internal/runview/assets/viewer.js`
- `spektacular: internal/runview/write_test.go`

**Discoveries**: The page can be checked without CI changes: `go test ./internal/runview -run TestSamplePages -sample-dir <dir>` writes the sample pages, and local headless Chrome (`google-chrome-stable --headless=new`, driven over the DevTools protocol with networking emulated offline) loads the 5,000-span page and paints it in about 0.3 s, with selection responding in about 30 ms. The self-containment test scans the script and stylesheet too, so they must never contain `src=`, `href=`, `url(`, `@import` or an `http` scheme, even in comments.

### 2026-10-25 — Task: Render a run from the command line

**What was done**: Added `spektacular stats view` (`cmd/stats_view.go`), registered with the other `stats` forms. It selects the run through `statsSelect` and `runstats.ForReport` exactly as `stats run` does, builds and writes the page with `internal/runview`, and writes it to `--out` (a file, or an existing directory given the default `spektacular-run-<id8>.html`; by default the current directory) through a temporary file renamed into place. A target inside the logs directory (symlinks resolved) is refused with `stats_view_out_in_logs`, and a failed write with `stats_view_write_failed`, each with a next action. The result is one readable line or the standard JSON envelope (`log_dir`, `skipped_lines`, `gap_threshold_ms`, `run`, `file`, `rows`, `bytes`), and `--schema` lists it. The group's logging exemption covers it with no new code.

**Deviations**: `cmd/stats_test.go` changed too: the bare `stats` next action and the schema test's list of forms now include `view`. The plan's separate `runStatsView` steps (format, threshold, logs, candidates) reuse 000075's `statsSelect`, which does all four.

**Files changed**:
- `spektacular: cmd/stats.go`
- `spektacular: cmd/stats_view.go`
- `spektacular: cmd/stats_view_test.go`
- `spektacular: cmd/stats_test.go`

**Discoveries**: Rendered against a copy of this project's own logs, a real 2h55m implement run (365 spans) makes a 60 KB page whose biggest feature is the long idle gaps between tasks, which is exactly what the viewer is for.

### 2026-10-25 — Task: Zoom, collapse and summarise on the page

**What was done**: The page script gained a zoom window (drag across the axis, Ctrl or Cmd with the wheel at the pointer, double-click a bar to fit exactly that span, Reset zoom or Escape for the whole run, 10 ms at narrowest), collapse and expand (a triangle per row with children, Expand all, Collapse to steps, opening collapsed to steps), a collapsed row's hidden-row count and the strongest problem among its hidden rows, a Gap threshold input in minutes (down to the page's gap floor) with "Gaps over threshold: N, total T" following it, and a side summary of the five longest steps and longest gaps over the threshold, each zooming to its place (a step is also revealed, selected and centred). The tree index (each row's subtree end and worst hidden problem) is built once; the visible-row list is rebuilt only when the collapse state changes. Tests add `BenchmarkBuildAndWrite5000` (about 20 ms, a 499 KB page), a test that each row's descendants are exactly the deeper rows that follow it, and a test that the page carries every control the script reads.

**Deviations**: Arrow keys still move the selection, and picking a step from the summary centres its row. A row with no children has no triangle.

**Files changed**:
- `spektacular: internal/runview/assets/viewer.js`
- `spektacular: internal/runview/assets/page.html`
- `spektacular: internal/runview/assets/viewer.css`
- `spektacular: internal/runview/write_test.go`
- `spektacular: internal/runview/build_test.go`

**Discoveries**: Measured in headless Chrome on the 5,000-span sample page, offline: first paint about 0.3 s, and every zoom, collapse, expand, summary pick and selection lands within two animation frames (33 to 66 ms). Headless Firefox renders the same page identically.

### 2026-10-25 — Task: Documentation site describes the run viewer

**What was done**: Added the "Run viewer" page (`src/pages/run-viewer.mdx`), built from `Hero`, `Prose` and `CtaBanner` and following the plan's content outline: rendering a run and where the file goes, choosing the run, logs from elsewhere, reading the waterfall (with a table of each type's look), problems, gaps and the threshold, zooming and collapsing, details, totals and the summary, and the JSON result for programs. The Resources menu links to it after Run statistics, and the Run statistics page's "Reporting on one run" section points to it.

**Deviations**: The page also mentions the arrow keys and the collapsed row's hidden-row count, which the page has, and that writing again replaces the file. The CtaBanner carries a short body, as the Run statistics page's does.

**Files changed**:
- `docs: src/pages/run-viewer.mdx`
- `docs: src/components/Nav.astro`
- `docs: src/pages/run-stats.mdx`

**Discoveries**: None.

### 2026-10-25 — Task: README lists stats view

**What was done**: Added `spektacular stats view [--name <spec>]` to the README's run statistics command block, with its one-line description, and a sentence after the block saying it writes a single HTML file that opens without a server, linking to the documentation site's Run viewer page.

**Deviations**: None.

**Files changed**:
- `spektacular: README.md`

**Discoveries**: None.
