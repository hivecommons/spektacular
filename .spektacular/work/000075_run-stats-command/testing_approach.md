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
  - every log file is byte-identical before and after each of the four subcommands, run inside an `init`ed project where logging is on, so the logging exemption is proven end to end.
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
