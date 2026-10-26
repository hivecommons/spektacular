- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports none).
- **Upstream spec `000073_log-mode-and-logger-providers` must land first.** It is planned but not implemented. This plan builds on:
  - its record format and `internal/tracelog` record type;
  - its run-file and day-file naming in the logs directory;
  - `config.LogDir(projectRoot)`;
  - its rewritten `runRoot`, which gains the logging exemption here.

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
- **`cmd` (changed).** It gains a new `stats` group, and `runRoot` gains the logging-exempt annotation check. The `commandSchema` and `runUnknownSubcommand` conventions are reused.
- **Go standard library only** (`bufio`, `encoding/json`, `os`, `path/filepath`, `sort`, `math`, `time`, `text/tabwriter`), plus the existing `cobra`. There is no new module.
- **Documentation site, `docs` repo (changed).** A new page `src/pages/run-stats.mdx`, a Resources nav entry in `src/components/Nav.astro`, and one sentence in `src/pages/debugging.mdx` as 000073 and 000074 leave it. It is checked by the site build and `astro check`.
- **`README.md` (changed).** The "How It Works" command overview.
- **Harbor test harness: not changed.** `tests/harbor/shared/metrics.py` and `report.py` were read as the model for the figures and the table. Changing Harbor is spec 000076.
- **Investigation report `docs/workflow-performance-investigation.md` (read, unchanged).** Its method and per-step table are what the report reproduces from logs (success metric).
