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
- **Decision**: the `stats` command group carries a logging-exempt annotation; `runRoot` writes no record for a command so marked.
- **Rationale**: this spec requires every log file to be byte-identical before and after any report; a logged `stats` call appends to the day file and can trigger the retention sweep. 000073's "every command is logged" rule exists for workflow commands and already lets polling tools opt out.
- **Rejected**: logging stats like any other command (breaks the read-only acceptance criterion); asking users to set `SPEKTACULAR_SESSION_LOG=off` (makes the guarantee depend on the caller).

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
