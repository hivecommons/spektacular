---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# 000073_log-mode-and-logger-providers: log mode and logger providers

## What was built

Debug mode is replaced by an always-on log mode. Every Spektacular command run in a project now writes one OpenTelemetry-model span record to a git-ignored JSON Lines file under `.spektacular/logs/`, through a logger chosen by provider name. Each record carries the trace and span ids, the command's name, start and end times and status, the agent, the workflow and step, and the step the command moved the workflow to. Logs are kept per run and trimmed to the most recent runs.

- **Record format and file logger (spektacular).** A new leaf package, `internal/tracelog`, defines the record shared by the rest of epic 000002: `record`, `trace_id`, `span_id`, an optional `parent_span_id`, `name`, `kind`, `start_time`, `end_time`, `status`, `attributes` and `resource`. It also mints the ids and holds a provider registry whose only logger is `file`.
  - The file logger appends one line per command to `<started>_<agent>_<kind>_<name>.jsonl` for a workflow run, or to `standalone_<YYYYMMDD>.jsonl` for commands outside one.
  - It keeps a self-ignoring `.gitignore` in the directory.
  - It contains every failure, so logging never changes a command's output or exit code.
  - It replaces `internal/sessionlog`.
- **Run identifier (spektacular).** Workflow state gains a `run_id`, minted whenever fresh state is built. A run keeps it across an interruption and resume, gets a new one on a `--force` restart, and each orchestrated lane has its own. A run already in progress before the upgrade is given one on its first logged command (`workflow.EnsureRunID`), with every other field of its state kept.
- **Settings and migration (spektacular).** The `log` section replaces `debug`, and the settings format moves from 4 to 5.
  - The keys are `provider: file`, `level: off | standard | verbose` (default `standard`), `retention` (default 10) and `config.directory` (default `logs`, relative to `.spektacular/`, or absolute).
  - Bad values are refused as `config_invalid` errors that name the accepted values.
  - `migrate` turns `debug.enabled: true` into `verbose` and false or absent into `standard`. Through a new idempotent `ignore` action, it adds `logs/` to `.spektacular/.gitignore`, as `init`'s template now does.
  - Old `.spektacular/debug/` files are never touched.
- **Recorder (spektacular).** `runRoot` decides the level from configuration alone. It logs only where a project config exists, snapshots the workflow state before and after, and attributes the record to the run in progress after the command, else before it, else a standalone trace. At `verbose`, the record also holds the full response and both state snapshots.
  - `status` carries a new `log: exempt` cobra annotation and is never logged. The run statistics group of spec 000075 sets the same annotation.
  - `SPEKTACULAR_SESSION_LOG` is no longer read.
  - The `cmd` tests gain a test-only `-log-level` flag, so the suite also runs with logging forced off.
- **Retention (spektacular).** When a write starts a new file, the file logger keeps the newest `log.retention` run files and, counted separately, the same number of day files. The file just written is never deleted, and a log that cannot be deleted is skipped.
- **Harbor (spektacular).** The shared session-log reader maps the new span records onto its existing events and still reads the old-format reference runs.
  - Every suite seeds `log.level: verbose` in its project's configuration. Seeding runs with logging off, and the image build fails if a seed command was logged.
  - Verifiers switch logging off before their own calls, through a new shared `loglevel.py`.
  - Tools, verifiers and `test.sh` read `.spektacular/logs/`.
- **Documentation (spektacular and docs).**
  - The docs site's debugging page is now "Logging and debugging". It covers what each level records, where logs go, retention, switching off, the commands that are never logged, a sample record and upgrading from debug mode.
  - The configuration reference documents `log` instead of `debug`.
  - The README and two Harbor knowledge entries describe the new logs.

## Why it matters

Maintainers get reliable per-command and per-step timings for every run without reading agent transcripts. The default level keeps the log small: the measured overhead is about 0.07 ms per command on a development build. It never gets in the way of a command, and the log stays out of version control and trimmed to recent runs. The record format and the run identifier are the foundation the rest of epic 000002 builds on: trace spans for agent work, `spektacular stats`, Harbor on built-in logs, and the HTML log viewer.

## Deviations from the plan

- `tracelog` adds two small helpers, `Run.Standalone()` and `Resource(version)`, and `logLevel` takes the configured level string rather than the whole config.
- Removing `debug` during migration is reported as a `set` action with an empty `To`.
- The settings format bump meant updating every test fixture that writes a current-format config, the migrate goldens, and the synthetic extensibility step (now 5 to 6).
- `workflow.EnsureRunID` decodes numbers exactly, but re-sorts the state's keys.
- The logging tests found two kinds of existing `cmd` test that broke:
  - Shared snapshot helpers now skip `.spektacular/logs/`, the expected side effect of every logged command.
  - A hand-written in-progress state fixture now carries a run id, so the pre-command backfill does not change its bytes.
- Every entry named like a log is a retention candidate, whatever its type. An undeletable old log is simulated with a non-empty directory of that name. The cmd-level retention tests live in `cmd/log_retention_test.go`.
- Harbor:
  - The epic suite's orchestrator-poll check was removed, because `status` is never logged. The timeline can no longer show when each child was spawned.
  - The spec, plan and repo verifiers' "init ran" checks now also require `skills_version`, because those images pre-seed a config.
  - The plan verifier's single `status` call also switches logging off first.
- The docs page names `spektacular stats` among the never-logged reports ahead of spec 000075, which the goal check required. The docs' example `schema:` line reads 5.
- The two Harbor knowledge entries were written through the knowledge CLI into the project's store in the main checkout, not onto the spec's worktree branch.
