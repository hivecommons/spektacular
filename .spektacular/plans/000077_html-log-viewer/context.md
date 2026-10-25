---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Context: 000077_html-log-viewer

## Current State Analysis

- **There is no picture of a run today.** Timings come from transcripts and Harbor's scripts (`tests/harbor/shared/timeline.py` prints a text timeline of epic lanes), or, once 000075 lands, from `spektacular stats` as numbers.
- **The logs and the report this viewer reads do not exist yet.** 000073 (planned) writes `span` records per command to `.spektacular/logs/`. 000074 (planned) adds `span_start`/`span_end` records for steps, tasks, `sub-agent`, `user-wait` and `work` spans, with end reasons and parents. 000075 (planned) adds:
  - `internal/runstats`: `Load`, `Assemble`, `Candidates`, `ForReport`, `Run`, `Span`, `BuildReport`, `Report`;
  - `cmd/stats.go`: the `stats` group with `list`, `run`, `compare` and `summary`, persistent flags, the `statsLogs`/`statsFilter`/`statsError` helpers, and the `{logAnnotation: logExempt}` annotation that `runRoot` honours.
- **CLI conventions to follow.**
  - Group subcommands share persistent flags. `--format pretty|json` defaults to pretty (`cmd/status.go:22-26`).
  - JSON goes through `output.New(w, globalFields).WriteResult` (`internal/output/writer.go`).
  - Refusals are `output.NewError(...).WithNextAction(...)`.
  - `--schema` uses `commandSchema` (`cmd/spec.go:34-55`).
- **Embedding files in the binary.** `templates/templates.go:1-7` embeds the template tree with `//go:embed all:*`. No HTML, CSS or JavaScript is embedded today, and `html/template` is not used anywhere.
- **Build and CI.** `make test` runs `go test -shuffle=on ./...` (`Makefile:52-53`). CI builds with dagger and runs only Go, with no browser and no Node.
- **Test helpers.** `runRootCmd` and `resetRootCmd` (`cmd/root_test.go:45-76`) and `writeCurrentConfig` (`cmd/root_test.go:93`). 000075 adds `seedLogs(t, dir)` in `cmd/stats_test.go`. Go 1.25 (`go.mod`) provides `t.Chdir`.
- **Docs.** `docs:src/components/Nav.astro:16-25` lists the Resources pages. After 000073 and 000075 it carries "Logging and debugging" and "Run statistics". Prose pages use `Hero`, `Prose`, `CtaBanner` and `Button` (`docs:src/pages/debugging.mdx:1-14`).

**Requirement to repo and files**

| Requirement | Repo | Files |
|---|---|---|
| Users can render a run as a page (selection, default latest) | spektacular | `cmd/stats.go`, `cmd/stats_view.go` |
| The page draws a waterfall (bars, nesting) | spektacular | `internal/runview/{model,build}.go`, `internal/runview/assets/viewer.js` |
| Span types are distinguishable; user waits set apart | spektacular | `internal/runview/build.go` (types), `internal/runview/assets/{viewer.css,viewer.js}` |
| Problem spans stand out | spektacular | `internal/runview/build.go` (problem flags), `internal/runview/assets/viewer.js` (markers) |
| Details are available on demand | spektacular | `internal/runview/build.go` (details columns), `internal/runview/assets/viewer.js` (panel) |
| The page shows the run's totals | spektacular | `internal/runview/build.go` (copied from `runstats.BuildReport`), `internal/runview/assets/viewer.js` (header) |
| Long gaps are visible; threshold adjustable | spektacular | `internal/runview/build.go` (gaps), `cmd/stats_view.go` (`--gap-threshold`), `internal/runview/assets/viewer.js` (bands, threshold control) |
| Users can zoom and collapse | spektacular | `internal/runview/assets/viewer.js`, `internal/runview/assets/page.html` |
| The page works on its own | spektacular | `internal/runview/{assets.go,write.go}`, `internal/runview/assets/*` |
| Large runs stay usable | spektacular | `internal/runview/assets/viewer.js` (visible-row drawing), `internal/runview/write_test.go` (benchmark, size) |
| Collected logs can be rendered | spektacular | `cmd/stats_view.go` (`--dir` via 000075's `statsLogs`), `internal/runview/build.go` (no paths in output) |
| The documentation describes the viewer | docs, spektacular | `docs:src/pages/run-viewer.mdx`, `docs:src/components/Nav.astro`, `docs:src/pages/run-stats.mdx`, `README.md` |

## Per-Task Technical Notes

### Task: Build the page data from a run

**File changes**
- `internal/runview/doc.go` (new): the package comment. It turns one `runstats.Run` into a self-contained HTML waterfall. The builder is pure, the writer writes to an `io.Writer`, and the package never touches the logs directory.
- `internal/runview/model.go` (new): `Model`, `RunInfo`, `Totals`, `Rows` and `Gaps` with the JSON tags in plan.md's Data Structures.
  - `Rows` is columnar: `Parent []int`, `Depth []int`, `StartMS []int64`, `DurationMS []int64`, `Type []int`, `Name []int`, `Problem []int`, `EndReason []int`, `Status []int`, `Message []int`, `Step []int`, `TaskID []int`, `SpanID []string`, `Args []int`, `ExitCode []*int`.
  - `Gaps` has `StartMS`, `DurationMS`, `Step`, `After` and `Before`.
  - Fixed lists: `Types = []string{tracelog.TypeCommand, TypeStep, TypeTask, TypeSubAgent, TypeUserWait, TypeWork}` (000073 and 000074 constants; if `TypeCommand` is not a constant, use `"command"`), and `Problems = []string{"", "failed", "interrupted", "abandoned", "open"}`.
  - `const Version = 1`.
- `internal/runview/build.go` (new): `Options{GapThreshold time.Duration; Skipped int}`, `Build(r *runstats.Run, o Options) Model`, `GapFloor` and `DefaultFileName`.
  - **Report.** `rep := runstats.BuildReport(r, runstats.Options{Gaps: math.MaxInt32, GapThreshold: o.GapThreshold})` (000075 plan, Go interfaces). The cap is larger than any event count, so `rep.Gaps` is every gap.
  - **Nodes.** One node per `r.Spans` element (ID, Parent, Type, Name, Start, End, EndReason, Status, Message, Step, TaskID). One more per `r.Commands` record:
    - `Type` command, `Name` the record's name, `Parent = ParentSpanID`;
    - Start and End from the record, with End = Start if EndTime is zero;
    - `Status` from `Status.Code`, `Message` from `Status.Message`;
    - `Step` from `spektacular.step`;
    - `Args` from `process.command_args` (`[]any`), joined with spaces and cut to 2,000 bytes with a trailing `…`;
    - `ExitCode` from `process.exit_code` (a float64 after JSON), as an int.
  - **Tree.**
    - Index nodes by span id. A node is a root when `Parent == ""` or the parent id is not in the index, or would form a cycle (guard with a visited set).
    - Children are sorted by Start, then by type rank (task, step, sub-agent, user-wait, work, command), then by span id, for determinism.
    - Depth-first emission gives the rows, with `Parent` as a row index (-1 for roots) and `Depth`.
  - **Offsets.** `StartMS = Start.Sub(r.Start).Milliseconds()`. `DurationMS = max(0, End.Sub(Start).Milliseconds())`. Spans use `runstats.Span.End`, the effective end.
  - **Problems** (first match wins):
    - a span with EndReason `interrupted` is interrupted, `abandoned` is abandoned, and `open` is open;
    - a span with `Status == "error"` is failed;
    - a command with status `error` or a non-zero exit code is failed;
    - otherwise 0.
  - **Status.** `ok` is 0, `error` is 1, and empty (open) is -1.
  - **Strings.** An interning table, `map[string]int` plus a slice, in first-use order, so output is deterministic. The empty string maps to -1.
  - **Gaps.** Keep `rep.Gaps` with `duration_ms >= GapFloor(o.GapThreshold).Milliseconds()`, sorted by `started_at`.
    - `StartMS` is the offset from `r.Start`.
    - `Step` is the interned step name, or -1.
    - `After` and `Before` are labels: `"<event> of <type> <name>"`, for example `end of command plan goto` or `start of user-wait which approach?`.
  - **Longest steps.** Row indexes of `Type == step` rows, sorted by DurationMS descending, then StartMS. At most 5.
  - **Totals and run.**
    - `Totals{WallMS, WorkingMS, UserWaitMS, HelperWaitMS}` are copied from `rep.Totals`.
    - `RunInfo` comes from 000075's `(*Run).Summary()` (`run_id`, `kind`, `name`, `agent`, `status`, `started_at`, `ended_at`) plus `SkippedLines: o.Skipped`. It deliberately omits `log_file` and every path, so the output does not depend on where the logs live.
  - `GapThresholdMS` and `GapFloorMS` come from the options.
- `internal/runview/build_test.go` (new): table tests over runs built by hand. A helper builds `runstats.Run` values directly, with fixed `time.Date` times. If 000075 makes them hard to build by hand, go through `runstats.Assemble` on hand-built `tracelog.Record`s instead. Cases:
  - `oneRowPerSpan`: a task containing two steps, each with commands, a sub-agent containing a nested `work` span, a user-wait, and the instant terminal step. Checks row count, parents, depths, order and offsets.
  - `orphanParent`: a span whose parent is not in the run becomes a root.
  - `effectiveEnd`: an open helper and an abandoned span end at `Span.End`, not at the recorded end.
  - `problems`: a failed helper (`stopped` with error), an interrupted step, an abandoned span, an open span, a command with exit code 2, and finished work.
  - `helperDetails`: name, type, start, duration, status, end reason, step, and run agent and workflow.
  - `noVerbose`: a command with `spektacular.response` and state attributes set; the model's JSON contains neither key nor value.
  - `totalsMatchReport`: Model totals equal `runstats.BuildReport` totals for the 000075 `knownWaits`-style fixture.
  - `gaps`: 20-minute and 2-minute pauses between commands. Both are present with their steps. Over 5 m is 1 and over 1 m is 2, using a test helper applying "duration > threshold". A 500 ms gap is dropped at a 5 m threshold.
  - `longestSteps`: six steps, the top five longest first.
  - `deterministic`: building twice gives equal models.
- `internal/runview/testdata/model.golden.json` (new): the golden JSON of the `oneRowPerSpan` model, pinning field names and shape. It is regenerated only with an `-update` flag.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) model types, tree building and rows with their tests; (b) gaps, longest steps, totals and strings with their tests, against a stub row builder. Integrate sequentially, then write the golden file.

### Task: Write the run as a self-contained page

**File changes**
- `internal/runview/assets/page.html` (new): the page shell. It has a `<!doctype html>`, a charset meta, a static `<title>Spektacular run</title>` (the script sets `document.title` from the data), `<style>/*@CSS@*/</style>`, `<script type="application/json" id="run-data">@DATA@</script>` and `<script>/*@JS@*/</script>`. The layout:
  - a header with the run identity and totals;
  - a toolbar holding the legend;
  - the main area: a scrollable waterfall `<canvas>` with a fixed label column of about 280 px;
  - a details panel.

  It has no `src`, `href` or external URL.
- `internal/runview/assets/viewer.css` (new): plain CSS with system fonts only, in light and dark via `prefers-color-scheme`. Type colours are defined once as custom properties, read by the script with `getComputedStyle`:
  - task purple, step blue, sub-agent teal, user-wait amber, work olive, command slate (thin, half-height);
  - problem colours: failed red, interrupted orange, abandoned grey.
- `internal/runview/assets/viewer.js` (new): plain ES2020 in one IIFE, with no modules, no imports and no `fetch`.
  - **Data.** Parse `#run-data` with `JSON.parse(el.textContent)`. Fill the header with `textContent`: kind, name, agent, run id (8 chars), status, start in UTC, and the four totals formatted like 000075's `fmtMS` (`12.3s`, `4m05s`, `1h02m03s`), with exact ms in `title`. Show `Skipped lines: N` when N > 0.
  - **Rows.** Visible rows are every row in this task; collapse comes in Milestone 2. Rows are 20 px high. On scroll and resize, draw only the rows inside the viewport.
  - **Bars.** The x scale maps `[0, wall_ms]` to the timeline width. A bar has a minimum width of 2 px, so instant spans still show.
  - **User waits.** Each user-wait span draws an amber band, at 15% opacity, across the full canvas height, behind the bars. Its own bar is hatched with a canvas pattern.
  - **Problem markers.**
    - failed: a 2 px solid red border and a `✕` in the label column;
    - interrupted: a dashed orange border (`setLineDash`) and `‖`;
    - abandoned: a dotted grey border and `∅`;
    - open: a right-hand gradient fade and `…`.
  - **Gaps.** Gaps with `duration_ms > gap_threshold_ms` draw as a hatched grey band across the full height, labelled with the duration at the top.
  - **Axis.** A time axis at the top with ticks at 1-2-5 steps chosen to fit, labelled as offsets (`+12m`).
  - **Legend.** One swatch per type and per problem, as DOM elements with `textContent`.
  - **Selection.** A click is hit-tested by row index (`floor((y + scrollTop) / rowHeight)`) and the x range. It highlights the bar and fills the details panel through `textContent`:
    - name, type, start (UTC, and the offset), duration, status (and message), end reason;
    - agent, workflow `kind name`, step, task id, span id;
    - for commands, the command line and exit code.

    For a bar that did not complete, the panel notes that it is drawn to where the span was cut off.
  - All strings from the data go through `fillText` or `textContent` only, never `innerHTML`. The visible text has no em dashes.
- `internal/runview/assets.go` (new): `//go:embed assets/page.html assets/viewer.css assets/viewer.js`, following `templates/templates.go:1-7`.
- `internal/runview/write.go` (new):
  - `Write(w io.Writer, m Model) error` uses `json.Marshal(m)`, whose default escaping turns `<`, `>`, `&`, U+2028 and U+2029 into `\u` escapes. It replaces each of the three markers exactly once. It returns an error if any marker is missing or repeated, or if the CSS or JS contains `</style` or `</script` (case-insensitive).
  - `Decode(page []byte) (Model, error)` finds the `run-data` element and unmarshals it.
- `internal/runview/write_test.go` (new):
  - `roundTrip`: `Decode(Write(m)) == m`.
  - `selfContained`: no `src=`, `href=`, `url(` or `@import`, and no `http://` or `https://`, anywhere in the page.
  - `hostileName`: a span named `</script><script>alert(1)</script>` and `<img src=x onerror=alert(1)>` never appears raw in the output.
  - `deterministic`: identical bytes on two writes. The page contains neither the temp directory path nor the current year's date, except the run's own times.
  - `under1MB`: a generated run of 2,000 spans gives a page under 1,000,000 bytes, and the test logs the size. `genRun(n, seed)` is a deterministic generator with tasks, steps, about 70% commands with realistic command lines, helpers, user waits, work spans and a few long gaps.
  - `markersOnce`: the shell has each marker exactly once.
  - `TestSamplePages`: skipped unless the `-sample-dir` test flag is set. It writes these pages into that directory for the manual checks:
    - `every-type.html`: every type, a failed helper, an interrupted step, an abandoned span and an open helper;
    - `gaps.html`: a 20-minute and a 2-minute pause;
    - `spans-2000.html` and `spans-5000.html`.

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis: (a) the Go writer, `Decode` and the write tests; (b) the shell, CSS and script against the golden model from the previous task. Sequential integration by one agent, which opens the sample pages in a browser to check them.

### Task: Render a run from the command line

**File changes**
- `cmd/stats.go` (as 000075 creates it):
  - Add `statsViewCmd` with `Use: "view"`, a `Short` and a `Long` describing the page, `--out`, the selection, `--dir` and `--gap-threshold`. Register it with `statsCmd.AddCommand(statsViewCmd)` beside the other four. It inherits the persistent flags and the group's `{logAnnotation: logExempt}`; no change to `runRoot`.
  - Flag: `--out` (`String`, default ""). `--schema` uses `commandSchema` (`cmd/spec.go:34-55`), with an output schema of `log_dir`, `skipped_lines`, `gap_threshold_ms`, `run`, `file`, `rows` and `bytes`.
- `cmd/stats_view.go` (new), or the same file, `runStatsView(cmd, args)`:
  1. Validate the format as 000075 does (`stats_format_unsupported`) and the threshold (`stats_bad_option` when `<= 0`).
  2. Load the logs: `logs, err := statsLogs(cmd)` (000075).
  3. Select the run: `c, err := logs.Candidates(statsFilter(cmd))`, then `run, err := runstats.ForReport(c)`, with errors through `statsError(err)`.
  4. Build the model: `m := runview.Build(run, runview.Options{GapThreshold: thr, Skipped: logs.Skipped})`.
  5. Resolve the output path:
     - an empty `--out` gives `filepath.Join(cwd, runview.DefaultFileName(run.ID))`;
     - otherwise `filepath.Abs(out)`, and if that is an existing directory, join the default name.
     - Resolve symlinks on the existing parent directory and on `logs.Dir`. If `filepath.Rel(logDir, outDir)` does not start with `..`, refuse with `stats_view_out_in_logs`.
  6. Write the file: `runview.Write` into a `bytes.Buffer`, then `os.CreateTemp(outDir, ".spektacular-run-*.html")`, write, `Close`, `Chmod(0o644)` and `os.Rename` to the target. On any error, remove the temp file and return `stats_view_write_failed` with `WithResource(path)` and the next action "check that <dir> exists and is writable, or pass --out <writable path>".
  7. Report the result:
     - JSON: `viewResult{Source; GapThresholdMS int64 \`json:"gap_threshold_ms"\`; Run runstats.RunSummary \`json:"run"\`; File string \`json:"file"\`; Rows int \`json:"rows"\`; Bytes int \`json:"bytes"\`}` through `output.New(cmd.OutOrStdout(), globalFields).WriteResult`;
     - pretty: `Wrote <kind> <name> (run <id8>, <rows> spans) to <file>`, plus `Skipped lines: N` when N > 0.
- `cmd/stats_view_test.go` (new), through `runRootCmd` (`cmd/root_test.go:70`) with 000075's `seedLogs(t, dir)` fixture and `writeCurrentConfig` (`cmd/root_test.go:93`). Each test uses `t.Chdir` into its own temp directory, which is supported in Go 1.25. Tests:
  - the default writes `spektacular-run-<id8>.html` for the latest run, and `runview.Decode` gives its `run_id`;
  - `--name`, `--run` (full and prefix), `--kind` and `--agent` each give the same `run_id` as `stats run --format json` with the same flags;
  - `--out file.html`, and `--out existing-dir/`;
  - `--format json` parses with `"error": false` and `file`, `rows` and `bytes`, and `--fields '["file"]'` works; the pretty line names the file;
  - the decoded totals equal the `stats run --format json` totals;
  - `--gap-threshold 1m` gives `gap_threshold_ms` 60000 in the page;
  - copy the logs directory with `cp -r` (a Go copy helper) outside the project, `t.Chdir` to a non-project directory, and run `stats view --dir <copy> --out a.html`; the page is byte-identical to the one rendered in the project;
  - hash every log file, run `stats view` in a project with `log.level: verbose`, then hash again: no new, removed or changed file;
  - `--out <logs dir>/x.html` gives `stats_view_out_in_logs` with a next action, and no file is created;
  - `--out /nonexistent/dir/x.html` gives `stats_view_write_failed` with a next action;
  - `--schema` lists the fields.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution. The command and its tests share fixtures and are small.

### Task: Zoom, collapse and summarise on the page

**File changes**
- `internal/runview/assets/viewer.js` (extend):
  - **State.** `view = {from, to}` (ms), a `collapsed` set of row indexes, and `selected`.
  - **Visible rows.** Derived in one pass over the tree-ordered rows: skip the descendants of a collapsed row by depth. Recomputed only when `collapsed` changes.
  - **Zoom.**
    - Drag on the axis strip draws a range, and mouseup sets `view`.
    - `wheel` with `ctrlKey || metaKey` scales around the pointer and calls `preventDefault`.
    - `dblclick` on a bar sets `view` to `[start, start + max(duration, 1)]` exactly.
    - The Reset zoom button and Escape restore `[0, wall_ms]`.
    - Minimum span: 10 ms.
    - The axis ticks recompute for the window, and bars outside it are skipped.
  - **Collapse.** A triangle in the label column toggles a row with children. "Expand all" clears the set. "Collapse to steps" collapses every row except task rows, so tasks show their steps and every other row hides its children. That is also the initial state.
    - A collapsed row draws the strongest problem among its descendants (failed > interrupted > abandoned > open) as a small marker, with its descendant count, for example `(42)`.
    - User-wait and gap bands are drawn from all rows and gaps, whatever is collapsed.
  - **Gap threshold.** A `Gap threshold` number input in minutes, with a `step` of 0.5 and `min = gap_floor_ms / 60000`, starting at `gap_threshold_ms`. On change it redraws the bands and recomputes "Gaps over threshold: N, total T" with the report's rule (`duration_ms > threshold`).
  - **Side summary.**
    - "Longest steps" comes from `longest_steps`, each entry showing name and duration.
    - "Longest gaps" holds the five longest gaps over the current threshold, each with its step and length.
    - Clicking an entry sets `view` to that interval with 5% padding, expands its ancestors and selects the row.
  - **Speed.** Redraws are batched with `requestAnimationFrame`. Drawing stays proportional to the visible rows and the gaps in the window.
- `internal/runview/assets/page.html` and `viewer.css` (extend): toolbar controls (Reset zoom, Expand all, Collapse to steps, Gap threshold) and the side summary panel, with no external references.
- `internal/runview/write_test.go` (extend): `BenchmarkBuildAndWrite5000` builds and writes `genRun(5000, 1)`. `TestSamplePages` already writes `spans-5000.html` for the manual responsiveness check.
- `internal/runview/build_test.go` (extend, if needed): a test that the model's tree order lets a depth-based skip hide exactly a row's descendants (each row's descendants are contiguous and have greater depth).

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution. All the changes are in the page's own assets, followed by a hands-on browser check of the sample pages.

### Task: Documentation site describes the run viewer

**File changes**
- `docs:src/pages/run-viewer.mdx` (new):
  - frontmatter `layout: ../layouts/Shell.astro`, `title: "Run viewer - Spektacular"`, `description: "Open a recorded workflow run as a single-file HTML waterfall of its steps, tasks, helpers and waits."`;
  - imports `Hero`, `Prose`, `CtaBanner` and `Button`, as `docs:src/pages/debugging.mdx:6-9` does;
  - body: `<Hero>`, then one `<Prose>` holding sections 2 to 10 of the plan's content outline as markdown with fenced code blocks and a markdown table for the types, then `<CtaBanner>` with `<Button href="/run-stats/">`;
  - it follows the MDX conventions: a blank line inside slot bodies, no `<div>`, `<section>` or `class=`, and no em dashes.
- `docs:src/components/Nav.astro:16-25`: add `{ label: "Run viewer", href: "/run-viewer/" }` to the Resources children, straight after 000075's `{ label: "Run statistics", href: "/run-stats/" }`.
- `docs:src/pages/run-stats.mdx` (as 000075 creates it): one sentence at the end of its "Reporting on one run" section: "To see the same run as a picture, render it with [the run viewer](/run-viewer/)."
- Verify with `npm run build`, `npx astro check`, and `grep -nE "<div|<section|class=" src/pages/*.mdx`, which must return nothing.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: README lists stats view

**File changes**
- `README.md` ("How It Works", the `stats` code block 000075 adds after the `status` block): append the `stats view` line from the plan's content example. Then add one sentence after the block: "`stats view` writes a single HTML file that opens in any browser without a server; see the Run viewer page on the documentation site." No em dashes.

**Complexity**: Low
**Token estimate**: ~3k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Every Go test runs under the shuffled `go test` (`make test`). Fixtures use fixed `time.Date` timestamps and their own temp directories, and nothing reads the wall clock.

- **Build the page data from a run**: `internal/runview` table tests over hand-built runs:
  - one row per span at its recorded times, nested and ordered;
  - an orphan parent at the top level, and effective ends;
  - each type and problem flag;
  - helper details, with no verbose attributes;
  - totals equal to `BuildReport`;
  - the 20-minute and 2-minute gaps at 5 m and 1 m, and the floor;
  - the longest steps, determinism, and the `model.golden.json` contract.
- **Write the run as a self-contained page**: the round trip through `Decode`; no external reference; hostile names escaped; deterministic bytes; a 2,000-span page under 1 MB; each marker once. `TestSamplePages` writes the manual-check pages when `-sample-dir` is set.
- **Render a run from the command line**: `cmd` integration tests over a seeded fixture project:
  - the default file and latest run;
  - each selection matching `stats run`;
  - `--out` as a file and as a directory;
  - JSON, `--fields` and the pretty line;
  - totals equal to `stats run`, and `--gap-threshold` in the page;
  - a byte-identical page from a copied `--dir`;
  - byte-identical logs after rendering;
  - the in-logs and unwritable refusals with next actions, and `--schema`.
- **Zoom, collapse and summarise on the page**: `BenchmarkBuildAndWrite5000`, and the contiguous-descendants test on the tree order. The interaction is checked by hand on the sample pages.
- **Documentation site describes the run viewer**: `npm run build`, `npx astro check`, and the MDX layout-HTML grep.
- **README lists stats view**: reviewed in the diff. No test asserts README prose.

Success metrics, the acceptance criteria checked by hand, and the manual reviews are classified in plan.md's Testing Approach.

## Project References

- Spec `000077_html-log-viewer`, epic `000002_workflow-tracing-and-run-stats`.
- Upstream plans:
  - `000075_run-stats-command`: `internal/runstats` API, `stats` group, error codes, logging exemption;
  - `000074_trace-spans-for-agent-work`: span records, types, end reasons, parents;
  - `000073_log-mode-and-logger-providers`: record format, command attributes, log directory.
- Sibling spec `000076_harbor-on-built-in-logs`: independent of this plan.
- Design documents: none.
- Knowledge entries applied:
  - `conventions/error-messages-must-suggest-remediation.md`;
  - `conventions/tests-must-not-depend-on-order.md`;
  - `conventions/tests-must-pass-for-done.md`;
  - `conventions/plans-never-change-the-active-install.md`;
  - `conventions/store-files-must-be-written-through-the-cli.md`;
  - the docs repo's `conventions/plan-content-pages.md`, `conventions/no-em-dashes.md`, `conventions/mdx-authoring.md` and `conventions/site-layout.md`.
- Reference material: `docs/workflow-performance-investigation.md` (gaps and stalls).
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

This plan:
- Build the page data from a run: Medium, ~30k;
- Write the run as a self-contained page: High, ~45k;
- Render a run from the command line: Medium, ~25k;
- Zoom, collapse and summarise on the page: Medium, ~30k;
- Documentation site describes the run viewer: Low, ~12k;
- README lists stats view: Low, ~3k.

## Migration Notes

None. No settings format, state field, record field or existing JSON shape changes, and nothing is migrated. Installing the new binary is left to the maintainer between workflows (plans-never-change-the-active-install).

## Performance Considerations

- **Targets (spec):** a 5,000-span run renders in under 3 s and responds to a zoom or selection in under 200 ms. A page for about 2,000 spans is under 1 MB.
- **Size.** Columnar rows with interned strings cost roughly 60 to 100 bytes per row. Command lines are cut to 2,000 bytes, and verbose attributes are left out. A 2,000-span page should be a few hundred KB, which the `under1MB` test pins.
- **Go side.** Building is linear, apart from sorting children and gaps (`n log n`). `BenchmarkBuildAndWrite5000` tracks it. Loading is 000075's cost.
- **Browser side.** The script draws only the rows in the viewport at a fixed row height, so a redraw is proportional to the screen, not the run. Hit-testing is arithmetic. The visible-row list is recomputed only on collapse changes. Redraws are batched with `requestAnimationFrame`.
- **If the manual check misses a target**, internal script optimisations are allowed (an offscreen label canvas, lower detail when zoomed out). A library or a second file is not (see Open Questions).
