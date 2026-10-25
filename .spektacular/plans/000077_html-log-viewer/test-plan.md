---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Test plan: 000077_html-log-viewer

Manual checks for the run viewer (`spektacular stats view`). Everything that decides what the page shows (rows, nesting, types, problem flags, totals, gaps, the longest steps, the page's self-containment, size and determinism, and the command's selection, output and refusals) is covered by the Go suite. These procedures cover what needs a browser or a person.

## Setup (once)

From a checkout of the `spektacular` repo that includes this feature:

```bash
go build -o /tmp/spek-viewer/spektacular .
mkdir -p /tmp/spek-viewer/samples
go test ./internal/runview -count=1 -run TestSamplePages -sample-dir /tmp/spek-viewer/samples
```

This writes four sample pages into `/tmp/spek-viewer/samples/`:

- `every-type.html`: every span type, a failed helper (`broken helper`), an interrupted step (`verify`), an abandoned span (`abandoned work`), a never-closed helper (`helper still running`) and a command that exited with code 2;
- `gaps.html`: one step with a 20-minute and a 2-minute pause;
- `spans-2000.html` (about 220 KB) and `spans-5000.html` (about 500 KB): generated implement runs.

For a real run, render one from a project's logs (or a copy of them) without installing anything:

```bash
/tmp/spek-viewer/spektacular stats list --dir <project>/.spektacular/logs --kind implement
/tmp/spek-viewer/spektacular stats view --dir <project>/.spektacular/logs --run <run id> --out /tmp/spek-viewer/real.html
```

Use current desktop Chrome and current desktop Firefox. Open each page with `file://` (double-click, or File > Open).

## Success metric: the slowest step and the longest stall in under a minute

- **What to measure**: the time a maintainer takes to name the run's slowest step and its longest stall from the page alone, without reading the log. Threshold: under 60 seconds.
- **How**: render a real implement run with several tasks (`real.html` above, from a run of at least an hour). Ask a maintainer who has not seen this run's page to open it and, with a stopwatch running from the moment the page appears, name (a) the slowest step entry and its duration and (b) the longest gap and the step it fell in. They may use the side summary (Longest steps, Longest gaps), zoom and collapse.
- **Expected result**: both answers within 60 seconds, and both match `spektacular stats run --dir <logs> --run <run id>`: the step entry with the largest duration in `step_entries`, and the first entry of the "Longest gaps" table.
- **Who / when**: a maintainer, once after delivery, on a sample implement run.

## Types and problems are distinct

- **What**: on `every-type.html`, each span type has its own look, user waits stand apart, and the failed, interrupted and abandoned items are each marked.
- **How**: open the page, press Expand all. Compare each row with the legend.
- **Expected result**: task purple, step blue, helper agent teal, wait on the user amber and hatched with an amber band down the whole timeline, other work olive, command thin and slate. `broken helper` and the exit-2 `implement goto` have a red border and a red ✕; `verify` has an orange dashed border and ‖; `abandoned work` has a grey dotted border and ∅; `helper still running` fades out at its end and shows …. Collapsing `implement` (its triangle) leaves a red ✕ after `implement (5)`.
- **Who / when**: the implementer or a reviewer, before merge.

## Selecting shows details

- **How**: on `every-type.html`, click the `research` helper bar (or its name).
- **Expected result**: the Details panel shows Name `research`, Type `Helper agent`, Start (UTC and +offset), Duration, Status `ok`, End reason `stopped`, Agent `claude`, Workflow `implement 000077_html-log-viewer` and Step `analyze`. Clicking a command bar adds its Command line and Exit code. Clicking `helper still running` adds the note that it did not complete and is drawn to where it was cut off.
- **Who / when**: the implementer or a reviewer, before merge.

## A long gap is visible

- **How**: open `gaps.html`. Then type `1` in the Gap threshold box and press Enter (or Tab out).
- **Expected result**: at the default 5 minutes, only the 20-minute pause is a hatched band labelled `gap 20m00s`, and the toolbar reads `Gaps over threshold: 1, total 20m00s`. At 1 minute both pauses are bands (`gap 20m00s` and `gap 2m00s`), the toolbar reads `Gaps over threshold: 2, total 22m00s`, and both are listed under Longest gaps.
- **Who / when**: the implementer or a reviewer, before merge.

## Zoom and collapse work

- **How**: on `every-type.html`, press Expand all, then:
  1. double-click the `implement` step bar;
  2. press Escape;
  3. drag across the time axis from about +5m to +15m;
  4. press Reset zoom, then hold Ctrl (Cmd on macOS) and scroll over the timeline;
  5. press Collapse to steps, then click the triangle beside `implement`, then click it again.
- **Expected result**: (1) the axis runs from +10m to +30m and the `implement` bar spans the full timeline width; (2) and (4, Reset) the whole run returns, 0 to +34m; (3) only that range fills the width; (4, wheel) the view zooms in or out around the pointer; (5) the page shows tasks and steps only, the first click shows `implement`'s five children, the second hides them again, and `implement (5)` with ✕ reappears.
- **Who / when**: the implementer or a reviewer, before merge.

## The page works offline

- **How**: disconnect from the network (or in Chrome DevTools, Network > Offline; in Firefox, File > Work Offline), close any server, and open `every-type.html` and `real.html` from disk.
- **Expected result**: both render fully: header totals, legend, bars, axis and side summary. The browser console shows no errors and the network panel shows no request other than the file itself.
- **Who / when**: the implementer or a reviewer, before merge.

## A large run stays responsive

- **What to measure**: `spans-5000.html` renders in under 3 seconds and responds to a zoom or a selection in under 200 ms, in current Chrome and Firefox.
- **How**: in each browser, open DevTools Performance (Chrome) or the Firefox Profiler, start recording, open `spans-5000.html`, and stop when the waterfall appears. Then record while you: press Expand all; drag-zoom across the axis; Ctrl or Cmd-scroll; click a bar; choose the first Longest steps entry; press Escape.
- **Expected result**: first paint of the waterfall under 3,000 ms after navigation; each interaction's frame within 200 ms of the input, with no long task over 200 ms. For reference, headless Chrome on the implementer's machine painted it in about 0.3 s and answered every one of these inputs within two frames (33 to 66 ms).
- **Who / when**: the implementer or a reviewer, before merge, on a typical developer laptop.

## Visual review on a real implement run

- **What**: legibility of the page in current Chrome and Firefox, in light and dark mode.
- **How**: open `real.html` in both browsers, once with the system in light mode and once in dark mode. Scroll the whole run, select a few bars, and use the side summary.
- **Expected result**: the legend, type colours, user-wait bands, problem markers, axis labels, gap labels, the details panel and the side summary are all readable and do not overlap in a way that hides information; long names are cut with an ellipsis, not overflowing the label column; no visible text contains an em dash.
- **Who / when**: a maintainer, before merge.

## Review of the documentation page

- **What**: the Run viewer page on the built documentation site matches the shipped behaviour.
- **How**: in a checkout of the docs repo with this feature, run `npm ci`, `npm run build`, then `npm run preview` and open `/run-viewer/`. Check the Resources menu shows Run viewer after Run statistics, and that the Run statistics page's "Reporting on one run" section links to it. Compare each section with the real command and page: run `spektacular stats view --help`, render a page, and try every control the page describes.
- **Expected result**: the command, the flags (`--out`, `--name`, `--kind`, `--agent`, `--run`, `--dir`, `--gap-threshold`, `--format json`), the default file name `spektacular-run-<first 8 characters>.html`, each type's look, the problem markers, the Gap threshold box, the zoom and collapse controls, the details fields, the totals and the side summary are all described as they behave. The JSON example's fields match `stats view --format json`. No em dashes in the page.
- **Who / when**: a maintainer, before the documentation is published.
