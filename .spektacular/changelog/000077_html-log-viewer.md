---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# 000077_html-log-viewer: an HTML waterfall viewer for recorded runs

## What was built

`spektacular stats view` writes one recorded workflow run as a single, self-contained HTML file that draws the run as a waterfall. Every step, task, helper agent, wait on the user, other marked work and command is a bar on one timeline, placed at its start, sized by its duration, and nested under what contains it. The file opens in any current browser with no server and no network, so it can be kept with a test run or passed to a colleague.

- **Choosing the run.** The run is selected exactly as `stats run` selects it: the latest run by default, or the latest matching `--name`, `--kind`, `--agent` or `--run` (full id or an 8+ character prefix). `--dir` reads a logs directory copied from anywhere, from any folder, and the page is byte-identical to one rendered in the project.
- **Where it goes.** The page is written to `spektacular-run-<first 8 characters of the run id>.html` in the current directory, or to `--out` (a file, or an existing directory for the default name), through a temporary file renamed into place. An `--out` inside the logs directory is refused (`stats_view_out_in_logs`), and an unwritable one fails with `stats_view_write_failed`, each with a next action. Like every `stats` form, rendering is never logged and never changes a log.
- **The result.** One readable line (`Wrote plan 000075_run-stats-command (run 4bf92f35, 1234 spans) to ...`) or the standard JSON envelope with `file`, `rows`, `bytes`, `run`, `gap_threshold_ms`, `log_dir` and `skipped_lines`.
- **The page.** A header with the run's identity and its wall, working, user-wait and helper-wait times (copied from the run statistics report, so they always match `stats run`); a legend; a time axis; and a canvas that draws only the rows in view. Each type has its own colour; waits on the user are hatched and shade a band across the whole timeline; failed, interrupted, abandoned and never-closed spans each have their own border and glyph; gaps longer than the threshold are hatched bands labelled with their length. Selecting a bar shows its details. The page zooms (drag across the axis, Ctrl or Cmd with the wheel, double-click a bar to fit it, Reset zoom or Escape), collapses (opening collapsed to steps, with Expand all and Collapse to steps, and a collapsed row showing its hidden-row count and the strongest problem inside it), takes a Gap threshold in minutes with the gap count and total following it, and has a side summary of the longest steps and longest gaps, each of which zooms to its place.
- **How it is built.** A new package, `internal/runview`, turns a `runstats` run and its report into a compact, columnar page model (one row per span and command in tree order, interned strings, every gap of at least a second, the five longest step entries), pinned by a golden file. It embeds a page shell, stylesheet and plain script with `go:embed` and splices the model in as HTML-safe JSON, so a hostile span name stays text. The output carries no generation time and no path. A 2,000-span page is about 220 KB; a 5,000-span page about 500 KB, built and written in about 20 ms.
- **Documentation.** The documentation site has a new "Run viewer" page, linked from the Resources menu after Run statistics and from the Run statistics page, and the README lists `stats view`.

## Why it matters

Numbers in a report say how long a run took; a picture shows at a glance where it stalled, what ran at the same time and what waited on what. Maintainers investigating slow workflow runs can now open any run from the built-in logs, find the slowest step and the longest stall from the side summary in seconds, and share the page as one file.

## Deviations from the plan

- The run statistics report is asked for every gap with `Options{Gaps: 0}`, which 000075's implementation already defines as "every gap", rather than a very large cap.
- A span type the page does not know is drawn as other marked work, and a command with no recorded exit code counts 1 when its status is an error, as the `runstats` command listing does. Rows caught in a parent cycle are placed at the top level, so every row still appears once.
- Gap labels are drawn at the foot of the view rather than the top, where the first row's bar hid them. The details panel also names the problem, the arrow keys move the selection, and choosing a step in the side summary centres its row.
- `cmd/stats_test.go` was updated so the bare `stats` next action and the schema test list `view`.
- The page's step for an agent span is the span's own recorded step; `trace start` stamps the current step on every agent span, so only a span started with no step in progress shows none.
