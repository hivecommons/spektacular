---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Research: 000077_html-log-viewer

## Alternatives considered and rejected

### Option A: Draw every span as an SVG element

One `<rect>` and `<text>` per span is the simplest code, but a 5,000-span run means over 10,000 DOM nodes, and every zoom moves all of them.

**Rejected**: it puts the spec criterion "A large run stays responsive" (5,000 spans, under 3 s to render, under 200 ms per zoom or selection) at risk, and it makes the page heavier to open. A canvas that draws only the rows in view was chosen instead.

### Option B: Inline a charting or timeline library (d3, vis-timeline, Chart.js)

These would supply axes and zoom for free.

**Rejected**: d3 alone is about 280 KB minified, which cuts into the "page for about 2,000 spans is under 1 MB" success metric. It also goes against the spec's Technical Approach ("prefer drawing with plain browser features (SVG or canvas) over a charting library, to keep the file small and self-contained").

### Option C: Embed the raw log records and pair them in the browser

The page would read the records and compute spans, waits and gaps itself.

**Rejected**: it repeats 000075's pairing, end capping, wait unions and gap rules in JavaScript, so the totals could drift from `stats run`, which breaks "Totals match the report". It would also bring verbose-level attributes (`spektacular.response`, state snapshots, many MB) into the file. Go builds a compact view model from `runstats` instead (000075 plan, Data Structures: `Run.Spans`, `Run.Commands`, `BuildReport`).

### Option D: A separate top-level command (`spektacular view`)

A new command group just for the page.

**Rejected**: it would need its own `--dir`, selection flags, directory resolution, error mapping and logging exemption. A `stats` subcommand inherits all of them from 000075's `stats` group (persistent flags, `statsLogs`, `statsFilter`, `statsError`, the group's logging-exempt annotation). The spec's Technical Approach names "a `spektacular stats` form" first.

### Option E: `stats run --format html`

Make the page another output format of the run report.

**Rejected**: `--format` is a persistent flag shared by every `stats` form and writes to stdout, while the page is a file. Mixing them would make `--format html` meaningless for `list`, `compare` and `summary`.

### Option F: Write the page into the logs directory by default

Keep the page next to the logs it came from.

**Rejected**: 000075 guarantees that reporting never writes, moves or deletes anything in the logs directory (000075 plan, Architecture "The report never changes the logs"). The page goes to the current directory by default, and an `--out` inside the logs directory is refused.

### Option G: Serve the page or open a browser

Start a local server, or launch the system browser.

**Rejected**: a server is a spec Non-Goal ("a server that hosts pages"). Opening a browser was not asked for and is platform-specific.

### Option H: Add a JavaScript engine (for example goja) to the Go test suite

This would unit-test the inline script in CI.

**Rejected**: it adds a third-party module for tests only. CI runs only Go (`Makefile:52-53`, dagger build), with no browser and no Node. Layout decisions are computed in Go and unit-tested there. The script only draws and handles input, and that is checked by hand in a browser (manual test plan).

### Option I: Render the data through `html/template`

Use Go's HTML templating for the whole page, data included.

**Rejected**: `html/template` treats a `<script type="application/json">` body as a JavaScript context and would escape the value as a JS string literal, not raw JSON. The writer uses `encoding/json` instead, which escapes `<`, `>`, `&`, U+2028 and U+2029 by default, spliced into a fixed placeholder in the embedded shell. That is safe inside a script element even with a hostile span name such as `</script>`.

## Chosen approach — evidence

- `stats view` is a fourth `stats` form beside `list`, `run`, `compare` and `summary`. It reuses the group's persistent flags (`--dir`, `--format`, `--name`, `--kind`, `--agent`, `--run`, `--gap-threshold`), `statsLogs(cmd)`, `statsFilter(cmd)`, `statsError(err)` and the group annotation `{logAnnotation: logExempt}`. Source: 000075 context.md, "Task: List runs and report on one from the command line" (cmd/stats.go file changes). Run selection reuses `(*Logs).Candidates` and `ForReport` (000075 plan, Go interfaces), which is "every way the run statistics report selects a run" (spec requirement 1).
- The totals come from `runstats.BuildReport(run, Options{Gaps: <all>, GapThreshold})`, the same function `stats run` uses. So wall, working, user-wait and helper-wait equal the report's figures by construction. Source: 000075 plan, Data Structures (`BuildReport`, `Report.Totals`).
- Every gap, not only the top N, comes from the same report: `Options.Gaps` caps the list, so the viewer passes a cap at least as large as the event count. Gaps carry `started_at`, `ended_at`, `duration_ms`, `step`, `after` and `before`. Source: 000075 plan (gaps "longest first, at most --gaps entries"), and 000075 context.md (`gaps(r *Run) []Gap` emits every uncovered stretch).
- Bars use `runstats.Span.Start/End` (the effective end, capped for open or abandoned spans) and `Complete`, `EndReason`, `Status`, `Message`, `Parent`, `Type`, `Step`, `TaskID`. Command bars use the `tracelog.Record` start and end, `ParentSpanID`, status, `process.command_args` and `process.exit_code`. Sources: 000075 plan, Go interfaces (`Run.Spans`, `Run.Commands`); 000073 plan, Data Structures (span record fields and attributes).
- The parent rules follow 000074 plan, Data Structures "Parents": step under task or the run, task with no parent, agent span under the named span or the current step, command under its step span. Spans whose parent is not in the run go to the top level.
- The problem markers follow 000074's end-reason table: `abandoned` and `interrupted` carry an error status with that message; a failed `trace stop --status error` is `stopped` with an error status; a failed command has an error status and a non-zero exit code (000073). `open` (never closed) comes from 000075 (`EndReason = "open"`).
- Embedding assets follows `templates/templates.go:1-7` (`//go:embed all:*` into an `embed.FS`). That is the existing pattern for shipping text files inside the binary.
- The logging exemption follows 000075's rule (context.md, "Task: Keep the stats commands out of the logs": `logExempted` walks parents). A `stats` child inherits it with no new code.
- The `--format pretty|json` convention comes from `cmd/status.go:22-26`. JSON goes through `output.New(...).WriteResult` (`internal/output/writer.go`). Refusals use `output.NewError(...).WithNextAction` (convention error-messages-must-suggest-remediation).
- Docs pages are built from `Hero`, `Prose`, `CtaBanner` and `Button` (`docs:src/pages/debugging.mdx:1-14`). The Resources nav is in `docs:src/components/Nav.astro:16-25`.

## Files examined

- `spektacular:cmd/root.go:35-60` — `rootCmd`, `runUnknownSubcommand` for bare groups.
- `spektacular:cmd/root.go:426,455` — `projectConfigExists`, `noProjectError` (used by 000075's `statsLogs`).
- `spektacular:cmd/root.go:503-528` — command registration in `init()`. `stats` is added there by 000075, so nothing more is needed for a subcommand.
- `spektacular:cmd/gate.go:23-45,83-135` — the gate is a no-op outside a project, and the annotation-walk pattern (`gateExempted`) that 000075 copies for logging.
- `spektacular:cmd/status.go:22-26` — `pretty`/`json` format constants and the people-first default.
- `spektacular:templates/templates.go:1-7` — the `go:embed` pattern.
- `spektacular:go.mod` — Go 1.25. The page needs no new module (`embed`, `encoding/json`, `os`, `path/filepath` are standard library).
- `spektacular:Makefile:52-56` — `make test` runs the shuffled Go suite; `lint` is `go vet`. CI has no browser and no Node.
- `spektacular:docs/workflow-performance-investigation.md:57-65,286-287` — how gaps were measured. The longest stall is what the success metric asks a maintainer to find.
- `docs:src/components/Nav.astro:16-25` — the Resources children list. 000073 renames "Debugging" to "Logging and debugging", and 000075 adds "Run statistics" after it.
- `docs:src/pages/debugging.mdx:1-14` — page frontmatter and imports pattern (`Hero`, `Prose`, `CtaBanner`, `Button`).
- `docs:src/components/sections/` — the existing section components. No new component is needed for a prose page.

## External references

- HTML Living Standard, `<canvas>` 2D context — draws thousands of rectangles per frame without DOM nodes, which is why it was chosen over SVG for 5,000 spans.
- Go `encoding/json` documentation (`Marshal` escapes `<`, `>`, `&` and U+2028/U+2029 unless `SetEscapeHTML(false)`) — what makes the JSON safe to embed in a `<script>` element.
- Go `html/template` documentation ("JS contexts" include `<script type="application/json">`) — why the data is not passed through `html/template`.
- Chrome DevTools "Network conditions: Offline" — how the manual offline check is run.

## Prior plans / specs consulted

- `000075_run-stats-command` plan and context — the source of `internal/runstats` (`Load`, `Candidates`, `ForReport`, `Run`, `Span`, `BuildReport`, `Report.Totals`, `Report.Gaps`), the `stats` group's flags and helpers, the error codes, the JSON contract (integer `*_ms`, `log_dir`, `skipped_lines`, `end_reason` and `complete`), and the logging-exempt annotation. The viewer must not change any of them.
- `000074_trace-spans-for-agent-work` plan — span record kinds, span types, end reasons with their statuses, parent rules, task-span attributes, and the instant terminal step span (zero duration).
- `000073_log-mode-and-logger-providers` plan — the record format (`tracelog.Record`), command attributes (`process.command_args`, `process.exit_code`), verbose-only attributes (`spektacular.response`, `spektacular.state.*`), log file naming, `config.LogDir`.
- Spec `000077_html-log-viewer` — requirements, acceptance criteria and success metrics carried into the plan.

## Open assumptions

- 000075's `internal/runstats` lands with the exported API its plan names (`Load`, `Logs.Candidates`, `ForReport`, `Run{Spans, Commands, Agent, Kind, Name, ...}`, `Span{...}`, `BuildReport`, `Options{Gaps, GapThreshold}`, `Report.Totals`, `Report.Gaps`), and `BuildReport` with a large `Gaps` cap returns every gap. If only names differ, adapt. If a figure's meaning differs, STOP and ask.
- 000075's `cmd/stats.go` lands with the persistent flags and the `statsLogs`/`statsFilter`/`statsError` helpers on the `stats` group, and the logging exemption is set on the group so children inherit it.
- An agent span's agent and workflow are the run's (`Run.Agent`, `Run.Kind`, `Run.Name`). 000074 stamps agent spans with their run's workflow attributes, so `runstats.Span` not carrying attributes loses nothing.
- Canvas drawing of only the rows in view keeps a 5,000-span run within 3 s to render and 200 ms per interaction in a current desktop browser. This is verified manually. If it fails, internal drawing changes are allowed; adding a library is not (spec Technical Approach).

## Drafting assumptions

### Chosen direction: `stats view` with a Go view model drawn on a canvas (architecture)
- **Decision**: `spektacular stats view` (a 000075 `stats` form) loads and selects the run through `internal/runstats`. A new pure package `internal/runview` builds a compact, columnar view model: tree-ordered rows, offsets, flags, details, every gap of at least 1 s, and the report's totals. It splices the model as `encoding/json` into an embedded page shell whose inline script draws the visible rows on a canvas and handles zoom, collapse, selection and the gap threshold. The page goes to `--out` (default `spektacular-run-<id8>.html` in the current directory). The output is deterministic, with no generation time and no paths.
- **Key trade-offs**: the layout lives in Go so CI can test it, at the cost of a thin script that is checked only by hand in a browser. Canvas trades DOM inspectability for speed at 5,000 spans. Totals are copied from `BuildReport` rather than recomputed, so they cannot drift. Verbose attributes are left out to meet the 1 MB target.
- **Rejected**: raw records with pairing in JS; an SVG node per span; a charting library; a top-level `view` command; writing into the logs directory; a JS engine test dependency (see research.md).

### The viewer is `spektacular stats view` (discovery)
- **Decision**: add a fifth `stats` subcommand, `view`, that writes the page. It inherits the group's selection flags, `--dir`, `--format`, `--gap-threshold` and the logging-exempt annotation.
- **Rationale**: the spec's Technical Approach names "a `spektacular stats` form" first. A subcommand reuses 000075's directory resolution, selection, error mapping and logging exemption without duplicating them.
- **Rejected**: a top-level `spektacular view` (it would duplicate every flag and helper); `stats run --format html` (it would mix a file-writing output into a report format whose output goes to stdout, and `--format` is a persistent flag shared by all forms).

### Go builds the view model, the script only draws (discovery)
- **Decision**: Go computes the span tree order, depths, bar offsets, flags, details, all gaps and the totals from `internal/runstats`. The page embeds that compact model as JSON. The inline script only draws, zooms, collapses and selects.
- **Rationale**: the totals then equal `stats run`'s by construction; the file stays small (no verbose attributes); and CI, which runs only Go with no browser or JS engine, can test every layout decision.
- **Rejected**: embedding the raw records and computing in JS (duplicates 000075's rules, and its totals could drift); adding a JS engine module to test the script (new dependency for tests only).

### Canvas with only the visible rows drawn (discovery)
- **Decision**: draw bars on a `<canvas>`, drawing only the rows in view, with the labels drawn in a fixed left column on the same canvas.
- **Rationale**: the 5,000-span responsiveness criterion; plain browser features only, as the Technical Approach asks.
- **Rejected**: one SVG element per span (too many DOM nodes to move on each zoom); a charting library (size, and the Technical Approach).

### Default output path and overwrite (architecture)
- **Decision**: `--out` defaults to `spektacular-run-<first 8 of run id>.html` in the current directory. An existing directory gets the default name inside it. An existing file is replaced, through a temporary file renamed into place. There is no stdout form. An `--out` that resolves inside the logs directory is refused (`stats_view_out_in_logs`).
- **Rationale**: the acceptance criterion needs a page with no options. The current directory is the least surprising place, and the name is distinctive enough that overwriting a re-render is what the user wants. The logs-directory refusal keeps 000075's "reporting never changes the logs" guarantee.
- **Rejected**: requiring `--out` (fails "asking for a page with no selection produces an HTML file"); writing to the logs directory or `.spektacular/tmp/` (the first breaks the read-only guarantee, the second is agent scratch space); refusing to overwrite (re-rendering the same run is the common case); a `-` stdout form (nobody asked for it, and it would have to keep the JSON envelope off stdout).

### Several `--run` values pick the first, as `stats run` does (architecture)
- **Decision**: `stats view` uses `Candidates` and then `ForReport`, exactly like `stats run`, so with several `--run` values it renders the first.
- **Rationale**: the spec asks for every way the run statistics report selects a run, and identical behaviour is the least surprising.
- **Rejected**: refusing several runs (inconsistent with `stats run`); drawing several runs (a spec Non-Goal).

### Gaps embedded down to 1 second (architecture)
- **Decision**: the page embeds every gap of at least min(1 s, `--gap-threshold`), and the page's threshold control cannot go below that floor.
- **Rationale**: it keeps the file small while letting the threshold be lowered on the page to 1 minute and below, as the acceptance criterion asks.
- **Rejected**: embedding only gaps over the CLI threshold (the threshold could not then be lowered on the page); embedding every sub-second gap (size, for no reading value).

### Page opens collapsed to steps, with problem markers bubbling up (architecture)
- **Decision**: on open, tasks and steps are visible and each step's children are collapsed. A collapsed row shows the strongest problem marker of its hidden descendants and a child count. User-wait bands and gap bands are drawn across the full height whatever is collapsed. A side summary lists the longest steps and longest gaps, each clickable to zoom.
- **Rationale**: the success metric asks a maintainer to find the slowest step and longest stall in under a minute, and a fully expanded 2,000-row view buries both. Bubbling markers means collapsing never hides a failure.
- **Rejected**: fully expanded by default (too long to scan); fully collapsed to the top level (hides the steps, the unit people compare).

### Bars use the effective end for unfinished spans (architecture)
- **Decision**: open and abandoned spans are drawn to 000075's effective (capped) end and marked. Their details show the end reason and say the bar runs to where the span was cut off.
- **Rationale**: it matches how `stats run` measures them, and an abandoned span's recorded end can be days later (a restart), which would stretch the timeline.
- **Rejected**: drawing the recorded end (it distorts the axis and disagrees with the report).

### Agent and workflow details come from the run (architecture)
- **Decision**: a span's agent, workflow kind and workflow name in the details panel are the run's (`Run.Agent`, `Run.Kind`, `Run.Name`), because `runstats.Span` carries no attributes map.
- **Rationale**: 000074 stamps every span in a run with that run's workflow attributes and agent, so the values are the same. No change to 000075's types is needed.
- **Rejected**: extending `runstats.Span` with attributes (it changes 000075's published contract for no gain).

### Conventions selected (architecture)
- **Decision**: apply error-messages-must-suggest-remediation, tests-must-not-depend-on-order, tests-must-pass-for-done, plans-never-change-the-active-install, store-files-must-be-written-through-the-cli, and the docs repo's plan-content-pages, no-em-dashes, mdx-authoring and site-layout conventions. Drop alternate-section-background and file-scoped-section-headings.
- **Rationale**: the dropped two govern `Section`-band pages and file-scoped reference headings. The new page is a `Prose` page like `debugging.mdx` and documents no file.
- **Rejected**: listing every convention (padding).

### Interactive acceptance criteria are manual checks (testing_approach)
- **Decision**: the criteria about how the page looks and responds (distinct types and problems, details on selection, gaps marked, zoom and collapse, offline, 5,000-span responsiveness) are manual browser checks in the test plan. Each is backed by Go tests of the model it draws from, plus a static test that the page has no external reference.
- **Rationale**: CI runs only Go, with no browser or JS engine, and a JS engine module only for tests was rejected.
- **Rejected**: headless-browser tests (CI has no browser); goja or a similar module (new dependency for tests only).

### The page's visual vocabulary is fixed in the plan (tasks)
- **Decision**: the plan fixes each type's colour (task purple, step blue, sub-agent teal, user-wait amber and hatched with a full-height band, work olive, command thin slate) and each problem's marker (failed red border and ✕, interrupted dashed orange and ‖, abandoned dotted grey and ∅, open fading end and …). The docs page uses the same words.
- **Rationale**: the docs content outline must state the page's facts, and fixing them in one place stops the script and the docs drifting apart.
- **Rejected**: leaving the look to the implementer (the docs could not then be outlined with fixed facts).

### Sample pages come from a test flag, not a command (tasks)
- **Decision**: the pages used for the manual checks (every type with each problem, the two pauses, 2,000 and 5,000 spans) are written by a skipped-by-default Go test when `-sample-dir` is given.
- **Rationale**: the manual checks need repeatable inputs, and a test flag adds no user-facing surface.
- **Rejected**: a hidden CLI command (user-facing surface for a test need); checked-in HTML fixtures (large and they go stale).

## Rehydration cues

- `spektacular spec file read 000077_html-log-viewer`
- `spektacular plan file read 000075_run-stats-command plan` and `... context` (the `runstats` API, the `stats` group, error codes)
- `spektacular plan file read 000074_trace-spans-for-agent-work plan` (Data Structures: span records, end reasons, parents)
- `spektacular plan file read 000073_log-mode-and-logger-providers plan` (Data Structures: record format)
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
- Re-read `templates/templates.go`, `cmd/status.go:22-26`, `docs:src/components/Nav.astro`, `docs:src/pages/debugging.mdx`.
