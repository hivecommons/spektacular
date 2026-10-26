---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Research: 000075_run-stats-command

## Alternatives considered and rejected

### Option A: Extend Harbor's Python tooling instead of a CLI command

Build the report into `tests/harbor/shared/report.py` and `metrics.py`.

**Rejected**: the spec's first constraint requires a Spektacular command, and 000076 requires Harbor to read the logs through this report. Harbor's per-step split (`tests/harbor/shared/metrics.py:277-347`) is kept only as the model for the table's columns.

### Option B: Load through 000074's `tracelog.Reader` and `tracelog.Index`

Reuse the per-run reader and the open-span index that 000074 adds to the file logger.

**Rejected**: `Reader.Records(run Run)` reads one run's file and needs the run's identity up front. It also pre-filters out every command record with no `parent_span_id` (000074 context, "internal/tracelog/reader.go"), which would make command counts and gaps wrong. `Index` answers open-span questions, keeps its maps private, and does not pair spans with durations. The report reuses `tracelog.Record`, `tracelog.Status` and the constants instead.

### Option C: Put the analysis inside `internal/tracelog` or inside `cmd`

**Rejected**: `tracelog` is the write-side leaf package on every command's hot path (000073 plan, "Shape"). Analysis in `cmd` cannot be reused by 000077 and needs cobra to test. A separate pure package fixes both.

### Option D: Fold the report into `spektacular status`

**Rejected**: `status` reports workflow state from `state.json` and the stores (`cmd/status.go:28-45`). The report reads only logs, must work on a copied directory with no project, and the spec's Technical Approach names `spektacular stats`.

### Option E: One `stats` command with mode flags

Use a single command with `--compare` and `--summary` flags.

**Rejected**: multi-mode surfaces are cobra groups whose bare form returns `unknown_subcommand` (`cmd/root.go:42-68`). Subcommands keep mode-specific flags (`--gaps`) where they apply.

### Option F: Sum helper and user waits

**Rejected**: concurrent helpers (epic lanes, parallel research agents) would sum past the wall time. The union of intervals equals the sum whenever waits do not overlap, which is what the spec's "Totals add up" criterion describes.

### Option G: Decide run status from workflow state

**Rejected**: a copied logs directory has no state, and the spec requires the same figures there as in the project.

### Option H: Write an index or summary file beside the logs

**Rejected**: the report must never change the logs, and 000073 and 000074 keep the logger's hot path to one append.

### Option I: Let `stats` be logged like every other command

**Rejected**: inside a project, each call would append to `standalone_<day>.jsonl` and could trigger the retention sweep (000073 plan, "File-logger mechanics"), breaking "Logs are unchanged by reporting". The group is exempted by 000073's logging-exempt annotation, which follows the gate's pattern (`cmd/gate.go:120-135`, `cmd/init.go:21`).

### Option J: Float seconds for durations

Use float seconds, like Harbor's `duration_s`.

**Rejected**: integer milliseconds (`*_ms`) are exact, avoid rounding drift between the JSON and the table, and match the old debug log's `duration_ms`.

## Chosen approach — evidence

- Record contract: 000073 plan, Data Structures (`record`, `trace_id`, `span_id`, `parent_span_id`, `name`, `kind`, `start_time`, `end_time`, `status{code,message}`, `attributes`, `resource`; attributes `spektacular.span.type`, `spektacular.agent`, `spektacular.workflow.kind`, `spektacular.workflow.name`, `spektacular.step`, `spektacular.step.to`, `process.command_args`, `process.exit_code`).
- Span kinds and end reasons: 000074 plan, Data Structures (`span_start` without `end_time`/`status`; `span_end` repeats its start and adds `end_time`, `status`, `spektacular.span.end_reason` of `moved|finished|stopped|abandoned|interrupted`; types `command|step|task|sub-agent|user-wait|work`; task spans carry `spektacular.task.id`; agent spans carry `spektacular.step` of the step current at start; every command record has `parent_span_id` = its step span).
- Interrupted step spans already end at their last command (000074, "Key decisions", resume path), so an interrupted step counts "up to the point it was cut off" with no extra work; spans abandoned by a restart end at the restart time (`AbandonRun` in `clearState`), so the report caps them at the run's last other event.
- Files: `<started>_<agent>_<kind>_<name>.jsonl` per run and `standalone_<YYYYMMDD>.jsonl` per UTC day, plus a `.gitignore` (000073 plan, "Log files"); the run identifier is the trace id (`workflow.State.RunID`).
- Log directory: `config.LogDir(projectRoot)` (000073 plan, Project configuration).
- Output envelope: `output.New(w, globalFields).WriteResult(v)` injects `"error": false` and applies `--fields` (`internal/output/writer.go:22-140`); errors are `output.NewError(code,msg).WithNextAction(...)` (convention).
- Format flag: `--format pretty|json`, pretty default, used by `status` (`cmd/status.go:22-26,173-179,283`) and `implement goal show` (`cmd/implement_goal.go:393-399,605`), with `--schema` via `commandSchema` (`cmd/spec.go:34-55`).
- Gate: outside a project the gate does nothing (`cmd/gate.go:30-45`), so `stats --dir <copy>` works anywhere; inside an out-of-date project it refuses as for every command.
- Command-tree test hygiene: `resetRootCmd` resets slice flags with `Replace(nil)` (`cmd/root_test.go:45-62`), so a repeatable `--run` flag is safe.
- Gap method: `docs/workflow-performance-investigation.md` "Data and method" (gaps between consecutive events labelled by what precedes them; per-step model/tool/sub-agent/human table in the Appendix).
- Harbor figures to cover: step entries `{step, started_at, duration_s}` (`tests/harbor/shared/metrics.py:336-344`), child records `{id, parent, spec, step, launched_at, ended_at, duration_s, ending}` (`metrics.py:473-492`), `idle_s` longest gap (`metrics.py:529-557`), epic lane start/finish/slowest step (`tests/harbor/shared/timeline.py:75-125`).

## Files examined

- spektacular:cmd/root.go:42-68 — `runUnknownSubcommand`, the group-command convention.
- spektacular:cmd/root.go:95-176 — current `runRoot` recorder (to be rewritten by 000073, which adds the logging-exempt check); not changed by this plan.
- spektacular:cmd/root.go:503-528 — command registration in `init()`.
- spektacular:cmd/gate.go:30-130 — gate behaviour and the annotation-based exemption pattern 000073's logging exemption follows.
- spektacular:cmd/status.go:22-283 — `--format pretty|json`, `--schema`, error codes for a bad format.
- spektacular:cmd/implement_goal.go:380-420,605 — second instance of the pretty/json pattern.
- spektacular:cmd/spec.go:34-55 — `schemaProp`, `schemaObj`, `commandSchema`.
- spektacular:cmd/root_test.go:35-80 — `resetRootCmd`, `runRootCmd`, config fixture helpers.
- spektacular:internal/output/writer.go:1-150 — envelope, field filter, `ErrorResponse`.
- spektacular:internal/status/pretty.go:1-60 — readable output style (lipgloss, plain when not a terminal).
- spektacular:internal/steps/*/steps.go — every workflow's terminal step is `finished`; kinds are spec, plan, implement, repo.
- spektacular:docs/workflow-performance-investigation.md — gap and per-step method the report must reproduce.
- spektacular:tests/harbor/shared/metrics.py:277-557 — Harbor's step, child and idle figures.
- spektacular:tests/harbor/shared/report.py:1-80 — cross-run report, medians, unknown-not-zero rule.
- spektacular:tests/harbor/shared/timeline.py:75-160 — epic lane timeline, hand-off gaps from orchestrator events.
- spektacular:README.md:21-45 — "How It Works" command overview where `status` is listed.
- docs:src/pages/debugging.mdx — logging page (rewritten by 000073, extended by 000074).
- docs:src/components/Nav.astro — Resources menu (Debugging entry).
- docs:src/components/sections/ — Hero, Prose, CtaBanner used by reference pages.

## External references

- OpenTelemetry trace data model (via 000073) — field names the records follow; the report pairs spans by `span_id` and nests by `parent_span_id` as OTel tooling does.
- Nearest-rank percentile (standard definition) — p90 is always an observed value, simple to test by hand.

## Prior plans / specs consulted

- Plan `000073_log-mode-and-logger-providers` (plan, context) — record format, file naming, `LogDir`, `runRoot` pipeline, logging levels, and the logging-exempt annotation the `stats` group sets.
- Plan `000074_trace-spans-for-agent-work` (plan, context) — span kinds, types, end reasons, parents, interrupted/abandoned semantics, `Reader`/`Index` (rejected as the loader).
- Spec `000076_harbor-on-built-in-logs` — consumer of the JSON: steps and timings, helper count with spec and step, total user wait, total gap time, per-step working/helper/user split, cross-run differences, epic timelines.
- Spec `000077_html-log-viewer` — consumer of the loaded runs and the totals ("Totals match the report"); reuses the package's loader and pairing.

## Open assumptions

- 000073 and 000074 land with the record fields, constants and `config.LogDir` named in their plans. If names differ, adapt; if a field or end-reason meaning differs, STOP and ask.
- An abandoned `span_end` written by a restart carries the restart time as `end_time` (000074 `AbandonRun`), so the report excludes abandoned ends from the event timeline and caps abandoned spans at the run's last other event.
- The terminal step is named `finished` in every workflow and 000074 records it as an instant step span closed with `finished`.
- Logs written before 000074 (commands only) have no step spans; for them the report gives totals and gaps but an empty step breakdown.

## Drafting assumptions

### Chosen direction: a `stats` command group over a pure `internal/runstats` package (architecture)
- **Decision**: `spektacular stats list|run|compare|summary` in `cmd/stats.go`, over a new package `internal/runstats` that loads a logs directory read-only, groups records into runs by trace id, pairs spans by span id, selects runs and computes reports, comparisons and summaries as pure functions. It depends only on `internal/tracelog` for the record type and constants. Key trade-offs: a separate loader instead of 000074's per-run `Reader`; status and figures derived from the log alone; interval unions for waits; abandoned span ends excluded from the timeline; a generic figure shape for comparisons and summaries.
- **Rationale**: keeps the analysis testable without cobra or files, reusable by 000077, and identical for the project's logs and a copied directory.
- **Rejected**: analysis inside `internal/tracelog` (bloats the write-side leaf package on every command's hot path); analysis inside `cmd` (not reusable by 000077, needs cobra to test); folding into `status` (reads state, not logs).

### Pretty is the default format (architecture)
- **Decision**: `--format pretty|json`, default `pretty`, as `status` and `implement goal show` do; Harbor passes `--format json`.
- **Rationale**: the two existing people-first reports use exactly this flag and default; the spec asks for both a table and structured output consistent with other commands.
- **Rejected**: JSON by default (the report's first audience is a maintainer at a terminal); a `table` format name (inconsistent with `pretty`).

### Selection rules per mode (architecture)
- **Decision**: `--name`, `--kind`, `--agent` and repeatable `--run` narrow the candidates (all combined with AND). `list` shows every candidate, newest first. `run` reports the most recent candidate. `compare` uses the `--run` runs in the order given when two or more are named, otherwise the two most recent candidates of the most recent candidate's kind, shown oldest first. `summary` uses the named `--run` runs, otherwise every candidate of the most recent candidate's kind. `--run` accepts a full run id or a unique prefix of at least 8 characters.
- **Rationale**: matches the spec's defaults with no selection and extends them predictably; a name is shared by a spec's spec, plan and implement runs, so compare and summary stay within one kind unless the user names runs.
- **Rejected**: comparing every matching run (unbounded, and mixes kinds); a `--last N` flag (not asked for).

### Percentiles (architecture)
- **Decision**: median is the middle value (the mean of the two middle values for an even count); p90 is the nearest-rank value (`ceil(0.9 n)`-th smallest).
- **Rationale**: matches Python's `statistics.median` that Harbor's report uses, and nearest-rank is always an observed value, easy to check by hand.
- **Rejected**: interpolated percentiles (harder to verify, no consumer needs them).

### Command time stands in for tool time (architecture)
- **Decision**: each step also reports `command_ms`, the summed duration of the Spektacular commands in it, as the logs' stand-in for the investigation's "tool" column.
- **Rationale**: the success metric asks for the per-step table apart from model time; the logs see only Spektacular's own commands.
- **Rejected**: omitting the column (the metric could not be met); claiming it measures every tool (it does not).

### The stats commands are never logged (discovery)
- **Decision**: the `stats` command group carries 000073's logging-exempt annotation; 000073's `runRoot` writes no record for a command so marked. This plan adds no mechanism of its own (epic review decision, 2026-10-25: the annotation and its `runRoot` check moved to 000073, which also marks `status` as never logged).
- **Rationale**: this spec requires every log file to be byte-identical before and after any report; a logged `stats` call appends to the day file and can trigger the retention sweep. 000073's "every command is logged" rule exists for workflow commands, and its annotation is how read-only commands opt out.
- **Rejected**: logging stats like any other command (breaks the read-only acceptance criterion); asking callers to switch logging off per call (makes the guarantee depend on the caller, and 000073 offers no per-command override); adding a second exemption mechanism in this plan (superseded by the review decision).

### A run is a trace carrying workflow attributes (discovery)
- **Decision**: records are grouped by `trace_id` across every `*.jsonl` file; a trace is a run when any record carries `spektacular.workflow.kind`. Standalone traces (commands and spans outside a workflow) are not listed or reported.
- **Rationale**: the spec's runs have a kind, a name and an agent; 000073 gives each workflow run its own trace.
- **Rejected**: reporting standalone traces as runs (no kind or name; an epic orchestrator's standalone spans are left for 000076 to request).

### Run status from the log alone (discovery)
- **Decision**: `finished` when a step span ended with `finished` (or a command moved to step `finished`); `interrupted` when not finished and either its spans were closed `abandoned` with no later activity, or a later run of the same kind and name started after its last event; otherwise `in_progress`.
- **Rationale**: copied logs have no workflow state, and the spec requires identical figures for a copy.
- **Rejected**: reading `state.json` (differs between the project and a copy). Known limit: a session stopped and never resumed or restarted reads as in progress.

### Wait totals are unions of intervals (discovery)
- **Decision**: helper-wait and user-wait totals (run, step and task) are the union of the completed spans' intervals, clipped to the step or task.
- **Rationale**: concurrent helpers would otherwise sum past the wall time; for non-overlapping waits the union equals the sum, as the acceptance criterion states.
- **Rejected**: plain sums.

### Durations in integer milliseconds (discovery)
- **Decision**: every duration field is an integer `*_ms`; timestamps are RFC 3339 UTC strings; percentages are floats rounded to one decimal.
- **Rationale**: exact, stable for Harbor and the viewer; matches the old debug log's `duration_ms`.
- **Rejected**: float seconds like Harbor's `duration_s`.

### A restart's closing records do not stretch a run (verification)
- **Decision**: "first to last recorded event" counts every record's start and end times except the end time of a span closed as `abandoned`; abandoned and open spans are capped at the run's last other event.
- **Rationale**: 000074 writes abandoned ends when a later `--force` restart closes the old run, possibly days later; counting them would turn idle days into wall time, gap time and step time. An interrupted step already ends at its last command, so this keeps every unfinished item measured "up to the point it was cut off", as the spec requires.
- **Rejected**: counting abandoned ends (inflates the interrupted run by the time until the restart); dropping abandoned spans entirely (they must still be listed and flagged).

## Rehydration cues

- `spektacular spec file read 000075_run-stats-command`
- `spektacular plan file read 000073_log-mode-and-logger-providers plan` and `... context`
- `spektacular plan file read 000074_trace-spans-for-agent-work plan` and `... context`
- Read `cmd/status.go`, `cmd/gate.go`, `internal/output/writer.go`, `docs/workflow-performance-investigation.md`, `tests/harbor/shared/metrics.py:277-557`.
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
