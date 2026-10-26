## Alternatives considered and rejected

- **Extend Harbor's Python tooling (`tests/harbor/shared/report.py`, `metrics.py`) instead of a CLI command.** Rejected: the spec's first constraint is that the report is a Spektacular command, and 000076 requires Harbor to read the logs *through* this report. Harbor's per-step split (`metrics.py:277-347`) is kept only as the model for the table's columns.
- **Reuse 000074's `tracelog.Reader` and `tracelog.Index` as the loading layer.** Rejected for loading: `Reader.Records(run Run)` reads one run's file, needs a `Run` (trace id, kind, name, start, agent) the report does not have up front, and pre-filters out every `span` (command) record that has no `parent_span_id` (000074 context, "internal/tracelog/reader.go"), so command counts and gaps would be wrong. `Index` answers open-span questions (open, descendants, last command under a parent) and keeps its maps private; it does not pair spans with durations. The report reuses `tracelog.Record`, `tracelog.Status` and the record/type/end-reason/attribute constants instead, so the contract stays in one place.
- **Fold the report into `spektacular status`.** Rejected: `status` reports workflow state from `state.json` and the stores (`cmd/status.go:28-45`); the report reads only the logs, must work on a copied logs directory with no project, and the spec's Technical Approach names `spektacular stats`.
- **One `stats` command with mode flags (`--compare`, `--summary`).** Rejected for subcommands `list`, `run`, `compare`, `summary`: every other multi-mode surface is a cobra group whose bare form returns the `unknown_subcommand` envelope (`cmd/root.go:42-68`), and per-mode flags (`--gaps`) then only appear where they apply.
- **Summing helper-wait and user-wait periods.** Rejected for the union of intervals: concurrent helpers (epic lanes, parallel research agents) would sum to more than the wall time. The union equals the sum whenever periods do not overlap, which is what the spec's "Totals add up" criterion describes.
- **Deciding run status from workflow state (`state.json`, lane files).** Rejected: a copied logs directory has no state, and the spec requires the same figures there as in the project ("Collected logs can be reported on"). Status is derived from the log alone.
- **An index or summary file written beside the logs.** Rejected: the report must never change the logs, and 000073/000074 keep the logger's hot path to one append.
- **Letting `stats` be logged like every other command.** Rejected: inside a project, each `stats` call would append to `standalone_<day>.jsonl` and could trigger the retention sweep (000073 plan, "File-logger mechanics"), breaking "Logs are unchanged by reporting". The `stats` group is marked exempt from logging, the same annotation pattern the gate uses (`cmd/gate.go:127`, `cmd/init.go:21`).
- **Seconds as floats (Harbor's `duration_s`).** Rejected for integer milliseconds (`*_ms`): exact, no rounding drift between JSON and table, and the old debug log already used `duration_ms`.

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
- spektacular:cmd/root.go:95-176 — current `runRoot` recorder (to be rewritten by 000073); executed command is known from `ExecuteC`.
- spektacular:cmd/root.go:503-528 — command registration in `init()`.
- spektacular:cmd/gate.go:30-130 — gate behaviour and the annotation-based exemption pattern.
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

- Plan `000073_log-mode-and-logger-providers` (plan, context) — record format, file naming, `LogDir`, `runRoot` pipeline, logging levels; the place the logging exemption hooks in.
- Plan `000074_trace-spans-for-agent-work` (plan, context) — span kinds, types, end reasons, parents, interrupted/abandoned semantics, `Reader`/`Index` (rejected as the loader).
- Spec `000076_harbor-on-built-in-logs` — consumer of the JSON: steps and timings, helper count with spec and step, total user wait, total gap time, per-step working/helper/user split, cross-run differences, epic timelines.
- Spec `000077_html-log-viewer` — consumer of the loaded runs and the totals ("Totals match the report"); reuses the package's loader and pairing.

## Open assumptions

- 000073 and 000074 land with the record fields, constants and `config.LogDir` named in their plans. If names differ, adapt; if a field or end-reason meaning differs, STOP and ask.
- An abandoned `span_end` written by a restart carries the restart time as `end_time` (000074 `AbandonRun`), so the report excludes abandoned ends from the event timeline and caps abandoned spans at the run's last other event.
- The terminal step is named `finished` in every workflow and 000074 records it as an instant step span closed with `finished`.
- Logs written before 000074 (commands only) have no step spans; for them the report gives totals and gaps but an empty step breakdown.

## Rehydration cues

- `spektacular spec file read 000075_run-stats-command`
- `spektacular plan file read 000073_log-mode-and-logger-providers plan` and `... context`
- `spektacular plan file read 000074_trace-spans-for-agent-work plan` and `... context`
- Read `cmd/status.go`, `cmd/gate.go`, `internal/output/writer.go`, `docs/workflow-performance-investigation.md`, `tests/harbor/shared/metrics.py:277-557`.
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
