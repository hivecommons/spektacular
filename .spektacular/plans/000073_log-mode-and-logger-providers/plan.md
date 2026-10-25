---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Plan: 000073_log-mode-and-logger-providers

<!-- Metadata -->
<!-- Created: 2026-10-24T20:25:41Z -->
<!-- Commit: 6730f8a -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Debug mode becomes an always-on log mode: every Spektacular command run in a project writes one OpenTelemetry-model span record (trace and span ids, timing, status, agent, workflow, step) to a git-ignored JSON Lines file under `.spektacular/logs/`, through a logger chosen by provider name, with one file per workflow run and only the most recent runs kept. Maintainers get reliable per-command and per-step timings for every run without reading agent transcripts, and the record format and run identifier become the foundation the rest of epic 000002 (trace spans, `spektacular stats`, Harbor on built-in logs, the HTML viewer) builds on.

## Conventions

- **Error messages must describe the problem and suggest remediation**: the new `log.provider`, `log.level` and `log.retention` validation errors, and any new migrate failure, must be `output.NewError("config_invalid", ...)` with a `WithNextAction` naming the accepted values and the exact key to edit.
- **A plan never changes the active skills and configuration**: the settings format moves from 4 to 5 and `.spektacular/.gitignore` gains a line. No task migrates or re-inits this repo's own `.spektacular/` or rewrites its `config.yaml`. The bump is verified with Go tests and throwaway projects, and the maintainer installs the new binary between workflows.
- **Store files are written through the CLI**: the plan's own knowledge-entry updates go through `spektacular knowledge write`. The new logs directory is not a store, so the logger writes it directly.
- **Tests must not depend on order**: logging is now on by default in every `cmd` test that drives `runRoot`. New tests reset the command tree through `resetRootCmd`, pin clocks or compare ids rather than timestamps, and use their own temp directories, so retention and day-file tests do not depend on the wall clock or on other tests.
- **Passing tests are required before calling work done**: making logging the default touches every `cmd` test. The full shuffled Go test suite (and Harbor's Python unit tests) must pass, with no failure dismissed as pre-existing.
- **Plans must sketch content structure, not just summarize it (docs repo)**: the debugging-page rewrite and the configuration-reference change carry a content outline with the verified keys, values and file names.
- **No em dashes (docs repo)**: all new docs, README and changelog prose avoids em dashes.
- **MDX authoring conventions (docs repo)**: the rewritten debugging page keeps prose in `<Prose>` and fenced code blocks, with no layout HTML in the page body. The `log` ConfigKey uses the slot pattern with `defaultValue` as an HTML string.

## Architecture & Design Decisions

**Review decision (2026-10-25).** At the epic's end-of-planning review the user decided that `spektacular status` is never logged, exactly like the read-only `stats` commands, and that `SPEKTACULAR_SESSION_LOG` is removed entirely, with no `off`, `standard`, `verbose` or `on` values and no per-command override ("just make it not log. I don't understand why we would want it to?"). The variable's only users were the maintainer's `spek-status` mod, which polls `status` every 3 s and no longer needs it, and Harbor, whose suites now set `log.level: verbose` in the configuration of the throwaway projects they seed. The only use of logged `status` calls, an epic orchestrator's polls marking when it noticed a child finish, is replaced by 000074's sub-agent spans. This plan now owns the logging-exemption mechanism, the `log: exempt` cobra annotation checked in `runRoot`, and marks `status` with it. Spec 000075 only marks its `stats` group with the same annotation.

**Shape.** Debug mode becomes a log mode. Every Spektacular command run inside a project writes one self-contained span record to a JSON Lines file, unless logging is switched off or the command is marked as never logged. The single choke point stays `runRoot` in `cmd/root.go`, which today records the debug session log. It now resolves the logging settings, checks whether the command is exempt, decides which run the command belongs to, builds one span record, and hands it to a logger chosen by name. A new leaf package, `internal/tracelog`, replaces `internal/sessionlog`. It holds three things:
- the record format: OpenTelemetry trace data model fields (`trace_id`, `span_id`, `parent_span_id`, `name`, `kind`, `start_time`, `end_time`, `status`, `attributes`, `resource`) plus a `record` discriminator;
- trace and span id minting;
- a small `Logger` interface with a provider registry, whose only entry is the `file` logger.

The file logger evolves today's recorder:
- one file per workflow run, named from the run's start time, agent, kind and name, so Harbor's file-name pattern keeps matching;
- commands outside any workflow go to one file per UTC day;
- a single `O_APPEND` write per record;
- a count-based retention sweep, run only when a new run or day file is created;
- a `.gitignore` of `*` kept in the log directory.

Every failure inside it is contained and dropped, panics included. That keeps the record cheap at the default level: no output capture, no line counting, and a directory listing only when a new file starts. So the under-10 ms budget is spent on two small state reads and one append.

**The run identifier lives in the workflow state.** `workflow.State` gains a top-level `run_id`, a 32-hex OpenTelemetry trace id. `workflow.New` mints it when it builds fresh state. `new` always clears the old state first, `--force` included, while the resume report never writes. So a run keeps its id across interruption and resume, gets a new one on restart, and each orchestrated lane has its own, because a lane is its own state file. A run that was already in progress before the upgrade has no `run_id`. On its first logged command, `runRoot` calls `workflow.EnsureRunID` on the state file the command will act on, before the command runs. That writes an id into the file once, keeping every other field byte for byte, and the command then loads and saves state with it in place. Which run a record belongs to is decided from the before and after snapshots:
- if the after-state is in progress, the command belongs to it (this covers `new`, `--force` and every in-run command);
- otherwise, if the before-state was in progress, it belongs there (the command that finishes a run, or that removes a finished lane);
- otherwise the command is standalone and gets a fresh trace id of its own.

The record carries the agent (from `agent` in config, `unknown` when empty), the workflow kind and name, the step the command ran in, and, when the step changed, the step it moved to. 000074 can then add step spans as `parent_span_id`, and its separate `span_start` and `span_end` records, without changing a field this spec defines.

**Configuration and levels.** `debug:` is replaced by a `log:` section that follows the spec, plan and epic provider pattern:
- `provider: file`;
- `level: off | standard | verbose`;
- `retention: 10`;
- `config.directory: logs`, relative to the folder holding `config.yaml`; an absolute path is allowed, and it is never added to the project-root-bound `storeDirs`.

`config.Validate` rejects an unknown provider with a `config_invalid` error naming the accepted loggers. It rejects an unknown level or a retention below 1 the same way. At the `verbose` level the record adds the full response, captured through the existing tee, plus the before and after state snapshots under attributes. The level comes from configuration alone. `SPEKTACULAR_SESSION_LOG` is no longer read, and there is no per-command environment override. As today, logging happens only where a project config exists. The settings format goes from 4 to 5. A `project4to5` migration step turns `debug.enabled: true` into `log.level: verbose` and false or absent into `standard`, writes the rest of the `log` defaults, and deletes `debug`. A new idempotent `ignore` action appends `logs/` to `.spektacular/.gitignore`, and the init template gains the same line. Old `.spektacular/debug/` files are never read, moved or deleted.

**Commands that are never logged.** A command is left out of the log when it, or any command above it, carries the cobra annotation `log: exempt` (constants `logAnnotation = "log"` and `logExempt = "exempt"` in `cmd/root.go`). This mirrors the upgrade gate's `gate: exempt` annotation and its parent walk. `runRoot` treats an exempt command exactly like logging off. It checks the command cobra is about to run, resolved from the same arguments the lane-by-argument state selection already reads, so an exempt command gets no run-id backfill, no snapshots and no tee. It checks again on the command that actually ran, so an exempt command never writes a record. That means no append, and no retention sweep either. `status` carries the annotation, so a tool polling it never adds to or trims the logs. 000075 sets the same annotation on its `stats` group, and every `stats` subcommand inherits it through the walk.

**Harbor and docs.** Harbor keeps its reporting with the smallest change:
- every suite that drives Spektacular sets `log.level: verbose` in the configuration of the throwaway project it seeds, instead of setting an environment variable in its image. The pre-built projects switch logging off while their seed scripts run and to `verbose` as their last step, so the log holds only the agent's run. The suites whose agent runs `init` itself get a minimal pre-seeded `config.yaml` that `init` keeps. Spec 000076 later sets them back to the default level;
- the verifiers that call `spektacular` themselves switch the project's logging off before their first call, which replaces their old `SPEKTACULAR_SESSION_LOG=off`;
- verifiers and tools look in `.spektacular/logs`;
- `tests/harbor/shared/sessionlog.py` turns new-format records into its existing `Event` shape and still reads the old-format reference data that backs its offline tests.

On the docs site, the debugging page (constraint: that page) is rewritten to describe logging, the configuration reference swaps `debug` for `log`, and the README follows. None of them describes an environment variable. They say instead that `status` and `stats` are never logged. This beats the OpenTelemetry SDK, a separate run registry, a hashed session id, and keeping or renaming a per-command environment override (see `research.md#alternatives-considered-and-rejected`). The first three either cannot carry a span across short-lived processes, contradict the spec's "run identifier comes from the workflow state" constraint, or cannot tell a restart from a resume. The override would keep a second switch whose only callers no longer need it. Conventions that drive specific choices:
- every new config error carries a next action (error-messages-must-suggest-remediation);
- the plan never migrates this repo's own `.spektacular/` and verifies the format bump in throwaway projects (plans-never-change-the-active-install);
- the new tests are order-independent and avoid wall-clock coupling (tests-must-not-depend-on-order).

## Component Breakdown

- **Trace log package (new, `spektacular` repo; replaces the session-log package).** A leaf package with no dependency on configuration or workflows. It owns:
  - the span record type and its JSON shape;
  - minting trace and span ids from a cryptographic random source;
  - the `Logger` interface (write one record for one run, never return an error);
  - the provider registry that maps a logger name to a constructor, with `file` as its only entry today and a list of accepted names for error messages.

  It is a new component rather than a rename-in-place because its contract changes: OTel-model records, run-keyed files, and retention. 000074 adds its start and end records here.
- **File logger (new, inside the trace log package; evolved from the session recorder).** Writes records to one JSON Lines file per run, or one per UTC day for standalone commands. Each record is a single append. The logger creates the directory and its self-ignoring `.gitignore` when it starts a file, and runs the retention sweep only then, keeping the configured number of newest runs and, separately, of newest days. It contains every error and panic, so it can never change a command's output or exit status.
- **Command recorder in the root command (changed, `spektacular` repo).** The one place every command's outcome converges. It now:
  - resolves the effective level from configuration alone, only where a project config exists;
  - treats a command carrying the logging-exempt annotation, on itself or a parent, as logging off, checked on the command about to run and again on the command that ran;
  - backfills a run id into a legacy in-progress state before the command runs;
  - snapshots state before and after;
  - decides the record's run, step and step-moved-to;
  - tees output only at the verbose level;
  - builds the record and hands it to the configured logger.

  It keeps its existing lane-by-argument state selection and its `terminalOut` handling (now set only when teeing). It owns the logging-exempt annotation (`logAnnotation`, `logExempt`) and its parent walk, which 000075 reuses for `stats`.
- **Status command (changed, `spektacular` repo).** `status` carries the logging-exempt annotation, so it is never logged at any level, and its behaviour is otherwise unchanged. Because it is never teed in a real process, its `terminalOut` fallback stays only as a defensive path.
- **Workflow state (changed, `spektacular` repo).** The persisted state gains a run id. The workflow engine mints it when it creates fresh state. A small helper ensures an existing in-progress state file has one, writing it atomically while keeping every other field. It is the only holder of the run id (spec constraint), and 000074 later keeps its open step-span id beside it.
- **Project configuration (changed, `spektacular` repo).** The `debug` section is replaced by a `log` section: provider, level, retention and the provider's directory. It has defaults on `NewDefault`, validation with remediation for an unknown provider, level or bad retention, and a resolver that turns the directory into an absolute path, relative to the folder holding `config.yaml`. The settings format version rises to 5.
- **Settings migration (changed, `spektacular` repo).** A new `4 → 5` project step converts `debug.enabled` into `log.level` (true becomes `verbose`, false or absent becomes `standard`), writes the other `log` defaults, deletes `debug`, and reports an action that adds `logs/` to the project's `.spektacular/.gitignore`. The engine gains that idempotent "ensure ignore line" action. Old debug files are never touched.
- **Project scaffolding (changed, `spektacular` repo).** The `.spektacular/.gitignore` template that `init` installs gains `logs/`, and this repository's root `.gitignore` gains `.spektacular/logs/`, so the maintainer's checkout stays clean once the new binary is installed.
- **Harbor session-log reader and verifiers (changed, `spektacular` repo).** The shared reader turns new-format span records into its existing event shape (time, argv, duration, exit code, state before and after, response) and still reads old-format lines, so the checked-in reference runs and offline tests keep working. Each suite's seeded project sets `log.level: verbose` in its configuration, and the verifiers that call `spektacular` switch the project's logging off first, through a small shared helper. Default log directories in the shared tools, verifiers and `test.sh` timeline calls move from `.spektacular/debug` to `.spektacular/logs`. Spec 000076 later replaces this reader.
- **Project documentation (changed, `spektacular` repo).** The README's configuration example and Harbor sections describe `log`, `.spektacular/logs/` and the seeded verbose level instead of `debug` and the environment variable. The Harbor knowledge entries that describe the old debug log are updated through the knowledge CLI.
- **Documentation site (changed, `docs` repo).** The debugging page is rewritten to describe logging: levels and what each records, location, retention, switching off, the commands that are never logged (`status` and `stats`), and the record format. It does not mention an environment variable. The configuration reference replaces the `debug` key and example lines with `log`, and the nav label is renamed to match.

## Data Structures & Interfaces

**Span record (log line format v1).** One JSON object per line, shared by the whole epic. 000075 and 000077 read these fields, and 000074 adds records with `record: "span_start"` and `"span_end"` and more `spektacular.span.type` values, without changing anything below. A default-level record for a `spec goto`:

```json
{"record":"span",
 "trace_id":"4bf92f3577b34da6a3ce929d0e0e4736",
 "span_id":"00f067aa0ba902b7",
 "name":"spec goto",
 "kind":"internal",
 "start_time":"2026-10-24T09:03:34.911373161Z",
 "end_time":"2026-10-24T09:03:34.934002113Z",
 "status":{"code":"ok"},
 "attributes":{
   "spektacular.span.type":"command",
   "spektacular.agent":"claude",
   "spektacular.workflow.kind":"spec",
   "spektacular.workflow.name":"000073_log-mode",
   "spektacular.step":"interview",
   "spektacular.step.to":"overview",
   "process.command_args":["spec","goto","--data","{\"step\":\"overview\"}"],
   "process.exit_code":0},
 "resource":{"service.name":"spektacular","service.version":"0.21.0"}}
```

- `parent_span_id` is omitted until 000074 sets it.
- On failure, `status` is `{"code":"error","message":"<the error response's message>"}` and `process.exit_code` is non-zero.
- A standalone command has no `spektacular.workflow.*` or `spektacular.step*` attributes.
- At the verbose level, attributes also carry:
  - `spektacular.response`: the full output, verbatim;
  - `spektacular.state.before` and `spektacular.state.after`: `{kind, name, run_id, current_step, completed_steps}`, or null.

```go
// internal/tracelog
type Record struct {
    Record       string         `json:"record"`            // "span" (000074 adds "span_start", "span_end")
    TraceID      string         `json:"trace_id"`          // 32 lowercase hex
    SpanID       string         `json:"span_id"`           // 16 lowercase hex
    ParentSpanID string         `json:"parent_span_id,omitempty"`
    Name         string         `json:"name"`
    Kind         string         `json:"kind"`              // "internal"
    StartTime    time.Time      `json:"start_time"`
    EndTime      time.Time      `json:"end_time"`
    Status       Status         `json:"status"`
    Attributes   map[string]any `json:"attributes"`
    Resource     map[string]any `json:"resource"`
}
type Status struct{ Code, Message string } // code "ok" | "error"; message omitempty

// Run says which log a record belongs to; the file logger derives the file from it.
type Run struct {
    TraceID   string
    Kind, Name string    // empty for a standalone command
    StartedAt time.Time  // the run's state.created_at; the command's start for standalone
    Agent     string     // "unknown" when unset
}

type Logger interface{ Write(run Run, rec Record) } // never errors, never panics outward

type FileOptions struct{ Directory string; Retention int }
func New(provider string, opts FileOptions) (Logger, error) // unknown provider -> error naming Providers()
func Providers() []string                                   // sorted: ["file"]
func NewTraceID() string; func NewSpanID() string
```

**Log files.** These live in the resolved directory, by default `.spektacular/logs/`:
- `<started 20060102T150405.000000000Z>_<agent>_<kind>_<name>.jsonl`: one per workflow run;
- `standalone_<YYYYMMDD>.jsonl`: one per UTC day for commands outside a workflow;
- `.gitignore` containing `*`.

Retention counts the first two groups separately.

**Workflow state.** `state.json` and each lane file gain one top-level field, `"run_id": "<32 hex>"` (`workflow.State.RunID`, `omitempty`). New helpers:

```go
func NewRunID() string                       // same format as tracelog.NewTraceID; workflow stays independent of tracelog
func EnsureRunID(statePath string) (string, error) // in-progress state without run_id: mint, write atomically, return it
```

**Project configuration (`config.yaml`, format 5).** `debug:` is removed. This replaces it:

```yaml
log:
  provider: file        # the logger; only "file" ships
  level: standard       # off | standard | verbose
  retention: 10         # most recent workflow runs kept (and as many days of standalone commands)
  config:
    directory: logs     # relative to the folder holding config.yaml, or absolute
```

```go
type LogConfig struct {
    Provider  string        `yaml:"provider"`
    Level     string        `yaml:"level"`
    Retention int           `yaml:"retention"`
    Config    FileLogConfig `yaml:"config"`
}
type FileLogConfig struct{ Directory string `yaml:"directory"` }
var LogProviders = []string{"file"} // pinned equal to tracelog.Providers() by test
func (c Config) LogDir(projectRoot string) string // absolute log directory
```

**Logging-exempt annotation.** The contract 000075 reuses for `stats`. `SPEKTACULAR_SESSION_LOG` is removed: nothing reads it, and the level comes from `log.level` alone. Nothing is ever recorded outside a project.

```go
// cmd/root.go
const (
    logAnnotation = "log"    // cobra annotation key runRoot reads
    logExempt     = "exempt" // the command, and every command under it, is never logged
)
func logExempted(c *cobra.Command) bool // true when c or any parent has Annotations[logAnnotation] == logExempt

// cmd/status.go
var statusCmd = &cobra.Command{ /* ... */ Annotations: map[string]string{logAnnotation: logExempt} }
// 000075, cmd/stats.go: statsCmd sets the same map, and its subcommands inherit it.
```

**Migration action.** `migrate.Action` gains `Op: "ignore"`, whose `Path` is the `.gitignore` file and whose `To` is the line. It is reported in previews. Apply appends the line when it is absent, creating the file if needed. It is idempotent.

## Implementation Detail

**A logger behind a provider registry, kept at the edge.** The trace log package is a leaf. It knows nothing of configuration, workflows or cobra, the same boundary the session-log package keeps today. The root command builds a `Run` and a `Record` from facts it already has (argv, timing, exit code, error message, state snapshots, agent) and hands them to whichever `Logger` the configured provider name resolves to. Selecting a logger by name and refusing an unknown one with the accepted names follows the task-id provider registry. The config package keeps its own list of accepted names, pinned to the registry by a test, the way the supported-agents list is pinned today. That keeps config free of an import on the logger. A second logger later (stdout, an OTel collector) is one more registry entry, with no change to the recorder.

**The recorder becomes a short pipeline in `runRoot`.** A reader of the root command sees five steps:
1. resolve the effective level (configuration only, never outside a project, and off for a logging-exempt command);
2. when logging, backfill a legacy run id, read the before-snapshot and, at verbose only, tee output;
3. run the command;
4. unless the command that ran is logging-exempt, read the after-snapshot, attribute the run, and build one record;
5. write it through the logger.

Each step is a small named helper with a unit test. The whole post-command block runs under a deferred recover, so a bug in logging can never alter the exit code. At the default level nothing touches the output stream. The verbose path is today's tee. Status's terminal detection still follows it, but `status` is exempt, so in a real process it is never teed. The lane-by-argument state selection is reused unchanged: it already gives orchestrated children their own state file, and so their own run id.

**Never-logged commands are marked, not listed.** The exemption is a cobra annotation, `log: exempt`, read through a parent walk exactly like the upgrade gate's `gate: exempt`. A command group marks itself once and its subcommands inherit it. That keeps `runRoot` free of command names, and 000075 needs only to set the map on `stats`. The check runs twice:
- before the command, on the target `rootCmd.Find` resolves from `os.Args[1:]`, the same argv the lane selection already reads, so an exempt command has no side effects at all, not even a run-id backfill;
- after it, on `ExecuteC`'s executed command, which is authoritative. It is the check that makes "no record" hold in tests, where cobra parses `SetArgs` rather than `os.Args`.

There is no environment override in either direction. Harbor's verbose capture moves into its seeded configuration, and polling `status` needs nothing. The only other source of a level is the `cmd` tests' unexported `forcedLogLevel`, set from a test-binary flag and unreachable from the shipped binary (see Testing Approach).

**Run identity moves into the workflow state.** The engine stamps a run id on every fresh state, in the one place fresh state is built, so every way a workflow starts gets one: spec, plan and implement `new`, orchestrated lanes, dry runs (into their throwaway state). Restart and resume semantics fall out of the existing `new` flow, which clears state before starting and never writes while reporting a resume. Older states are backfilled once by a helper that edits only the one key, through a generic JSON map so unknown fields survive. It writes through a temp file and rename, so a crash cannot truncate a state file.

**File-logger mechanics.** The run's file name is computed from the run (start time, agent, kind, name), so the common case is a stat and one `O_APPEND` write with no directory scan and no reading of earlier lines. The old per-line sequence number is dropped. Only when a write creates a file does the logger do three things: create the directory, ensure its `*` `.gitignore`, and sweep retention. The sweep lists `*.jsonl`, splits run files from day files by name, and deletes all but the newest N of each by their sortable start prefix. It ignores "already gone" errors from concurrent sweeps and never deletes the file it just wrote. This brings retention and self-ignoring to logging with no new background process.

**Migration follows the existing step pattern, with one new action kind.** `project4to5` is a pure YAML-tree edit, like `project3to4`. It reads `debug.enabled`, sets `log.*` defaults without overwriting any `log` key already present, deletes `debug`, and returns an `ignore` action for `.spektacular/.gitignore`. The engine performs that action idempotently, appending the line only when it is absent. This is the first action that edits a file rather than creating or removing one, so it gets its own op rather than overloading `create`. Old debug files are deliberately outside every code path.

**Harbor gets a normalising reader, not a rewrite.** `read_events` recognises a new-format line by its `trace_id` and maps it onto the existing `Event`:
- start time becomes `at`;
- end minus start becomes `duration_ms`;
- `process.command_args` becomes `command`;
- the exit code, response and state attributes map to their old fields.

Old-format lines still parse as before, so lanes, segments, step windows, timelines and metrics keep their exact behaviour and the pinned reference-run numbers do not move. The other Harbor edits are paths, comments, and where the verbose level comes from. Each suite's seeded configuration now sets it, and seeding itself runs with logging off. The verifiers that call `spektacular` switch logging off in the project's configuration before their first call, through one shared helper, because nothing else keeps their calls out of the log they assert against. This matters most for the standing-rules suite, whose own `spec file read` would otherwise satisfy the rule it checks.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports none).
- **Upstream specs: none must land first.** 000073 has no dependencies in epic `000002_workflow-tracing-and-run-stats`. Sibling `000072_knowledge-access-index-and-prune` is independent, but if it also raises the project settings format, whichever spec merges second must renumber its migration step (see Risks).
- **Downstream specs that build on this plan's contracts:** 000074 (trace spans for agent work: start/end records, `parent_span_id`, span types, step-span id beside `run_id`; its sub-agent spans replace the logged `status` polls this plan drops), 000075 (`spektacular stats` reads the record format and the logs directory, and marks its `stats` group with this plan's `logAnnotation: logExempt`), 000076 (Harbor replaces this plan's normalising reader), 000077 (HTML viewer reads the same records). None needs a change here, but the record fields defined in Data Structures are their contract and must not drift during implementation.
- **`internal/sessionlog` (replaced).** Its behaviour moves into the new trace log package and it is deleted, with no other importer outside `cmd`.
- **`internal/workflow` (changed).** It gains `RunID`, its minting on fresh state, and `EnsureRunID`.
- **`internal/config` (changed).** `LogConfig` replaces `DebugConfig`, and `CurrentProjectSchema` goes from 4 to 5.
- **`internal/migrate` (changed).** It gains the `project4to5` step and the `ignore` action.
- **`internal/project` and `templates/.spektacular/.gitignore` (changed).** `init` installs the new ignore line.
- **`cmd` (changed).** `runRoot`, the snapshot helpers and the status `terminalOut` comments change. `runRoot` gains the logging-exempt annotation and its walk, `status` carries it, and the `cmd` tests gain a test-only `-log-level` flag.
- **`internal/output` (used, unchanged).** `ErrorResponse.Message` is the recorded error message, and `NewError(...).WithNextAction` builds the config errors.
- **Go standard library only (`crypto/rand`, `encoding/hex`, `encoding/json`, `os`).** No new third-party module. The OpenTelemetry SDK is deliberately not added (research.md alternative 2).
- **Harbor test harness (changed, Python).** It covers `tests/harbor/shared/sessionlog.py`, `timeline.py`, `metrics.py`, a new shared `loglevel.py`, the suite verifiers, `test.sh` files, Dockerfiles, seed scripts and the implement suite's seeded `config.yaml`. Its unit tests run under pytest, and its suites need Docker, the Harbor CLI and agent logins.
- **Documentation site, `docs` repo (changed).** It covers `src/pages/debugging.mdx`, `src/pages/configuration.mdx` and `src/components/Nav.astro`, and is checked by the site build and Astro's type check.
- **Knowledge entries (changed through the CLI):** `gotchas/harbor-agent-logs-and-artifacts.md` and `architecture/testing-architecture.md` in the `spektacular` repo store.
- **Prior plan `000031_agent-session-debug-log`** explains why the recorder sits in `runRoot` and is the behaviour being replaced. Prior plans `000068`, `000069` and `000071` hold manual test steps that name `.spektacular/debug/`. Those are historical records and are not edited.

## Testing Approach

**Test types and where they slot in.** The work is covered in four layers. All of them run under the project's shuffled, order-independent Go test run. Every `cmd` test goes through `resetRootCmd`, uses its own temp project, and compares ids, file counts and record fields rather than wall-clock values.

- **Unit tests in the trace log package**, the most coverage, because the record format is the contract four later specs read. They cover:
  - record JSON field names and omission rules;
  - id formats (32 and 16 lowercase hex, never all zeros);
  - run and day file naming;
  - append without reading earlier lines;
  - the self-ignoring `.gitignore`;
  - retention: keep N newest runs, keep N newest days independently, never delete the file just written, tolerate a file already gone;
  - failure containment: an unwritable directory or an undeletable old log makes `Write` return normally with no panic;
  - the provider registry: an unknown name gives an error that lists `file`.
- **Unit tests in workflow, config and migrate.**
  - Workflow: fresh state gets a `run_id`, a loaded state keeps its own, and `EnsureRunID` adds one to a legacy in-progress state, preserves every other byte-level field and unknown keys, and leaves finished states and states that already have an id alone.
  - Config: defaults (`file`, `standard`, `10`, `logs`); validation errors with next actions for an unknown provider, an unknown level and a retention below 1; relative and absolute directory resolution; the `LogProviders` list pinned to the registry.
  - Migrate: `project4to5` gives `verbose` from debug on and `standard` from debug off or absent; `debug` is gone; existing `log` keys are kept; the `ignore` action appends `logs/` once, creates the file when missing, and is reported in dry runs. A full `migrate` of a format-4 fixture project reaches format 5 and leaves a pre-seeded `.spektacular/debug/` byte-identical.
- **Command-level integration tests in `cmd`**, which drive `runRoot` against throwaway projects and git repos. These carry the load-bearing acceptance criteria:
  - A fresh `init`ed project logs without setup.
  - Every record has the trace-model fields.
  - A `spec goto` record names agent `claude`, kind, name, step and step-to, and agent `unknown` with no agent configured.
  - Start, two gotos, a resume report and a goto share one trace. `--force` gives a new trace. Two orchestrated plan lanes keep distinct, internally constant traces.
  - A state file written without `run_id`, as the previous version wrote it, resumes without error and keeps one trace from then on.
  - Two standalone `knowledge search` commands get two traces in one day file.
  - The default level omits a large response and state snapshots, and a failing goto records `error` status, a non-zero exit code and the message.
  - Verbose records the exact output and both snapshots. `off` writes nothing.
  - `status`, pretty and JSON, with and without a name, at the standard and verbose levels, adds no record and creates no log file. Run on a legacy in-progress state, it leaves `state.json` byte-identical (no run-id backfill). The next `spec goto` still writes its record.
  - The annotation walk: a command marked `log: exempt`, and a subcommand of a marked group, are exempt; an unmarked command is not. Nothing is written outside a project.
  - An unknown provider fails commands with a config error naming `file`. A custom directory is honoured and the default one stays empty.
  - After `init` in a new git repo, and after `migrate` of a format-4 project, `git status --porcelain` shows nothing from the logs directory.
  - Retention: 12 runs leave 10; retention 3 leaves 3; standalone commands in between remove no run log.
  - An unwritable logs directory and an undeletable old log both leave stdout and the exit code identical to a logging-off run.
  - Old debug files stay untouched after `migrate` and further commands.

  The existing session-log tests are rewritten onto these, not deleted without replacement. The old environment-variable tests become the `status` exemption and annotation tests above.

  **Logging off across the whole `cmd` suite.** `cmd/main_test.go` gains a test-only flag, `-log-level`, which TestMain copies into an unexported `forcedLogLevel` in `cmd`. When it is set, `runRoot` uses that level in place of the configured one. No environment variable or shipped flag reaches it. The tests that assert logging behaviour call a `useConfiguredLogLevel(t)` helper, which clears the forced level for that test and restores it afterwards, so they still test what they claim under either run. The suite runs twice: as usual with logging on, and once as `go test -shuffle=on -count=1 ./cmd/ -args -log-level=off`. No other package drives `runRoot`.
- **Harbor Python unit tests.** New cases for the normalising reader: a new-format record maps to the same `Event` fields as its old-format twin, a mixed directory reads both, and a verbose resume refusal is still detected. The existing old-format fixtures and pinned reference-run values must pass unchanged.
- **Docs repo.** The site build and `astro check` pass, and the MDX guard (no layout HTML in page bodies) returns nothing.

**Load-bearing guarantees.**
- One record per command, in the documented format, and none for `status` or any other logging-exempt command.
- The run id follows the run, not the process.
- Logging never changes a command's output or exit code.
- Retention never removes workflow runs because of standalone commands.
- Logs never appear in `git status` for an `init`ed, migrated or never-initialised test project.

**Deliberate gaps.**
- No automated timing assertion: wall-clock thresholds are flaky in CI, so the latency metric is measured by hand below.
- No automated Harbor suite run: the suites need Docker and paid agent logins and are not in CI.

**Success metrics.**
- *A maintainer can read each step's start, end and duration from the log alone, for a real workflow run.* **Manual — captured in the implementation test plan.** Supporting behavioural test: records carry `spektacular.step` and `spektacular.step.to` with start and end times on every step-changing command.
- *The default level adds under 10 ms to a typical command, over 100 runs with logging on and off.* **Manual — captured in the implementation test plan.** `status` is never logged now, so timing it would measure nothing. The measurement uses `spec file list`, a typical read-only command that is logged, with `log.level` set to `standard` and then `off`. A Go benchmark of the record-and-write path is added as supporting evidence, but the metric is a wall-time measurement of the installed binary.
- *One complete implement run of a plan with 10 tasks produces a default-level log under 1 MB.* **Manual — captured in the implementation test plan.** A real run is needed for realistic command counts.
- *No command's output or exit code differs between logging on and off across the full Go test suite and the Harbor workflow suites.* **Behavioural test:** an integration test runs a representative command set (success, rejection, a large output, `status` pretty and JSON) at `off`, `standard` and `verbose` and asserts byte-identical stdout and exit codes. The `cmd` suite must also pass with logging forced off through its test-only flag (`go test -shuffle=on -count=1 ./cmd/ -args -log-level=off`), as well as with the default. The Harbor half is **Manual — captured in the implementation test plan.**

**Manual reviews.**
- **Manual — captured in the implementation test plan:** a Harbor workflow suite run (at least the plan suite and the epic suite) reports the same workflow steps and parallel runs as a run on the previous version, reading from `.spektacular/logs/` (spec AC "The test harness still reports sessions").
- **Manual — captured in the implementation test plan:** a read-through of the rewritten debugging page and the configuration reference on the built site, checking every level, the location, retention, switching off and the never-logged commands against the shipped behaviour, and that no environment variable is described.

## Milestones & Tasks

### Milestone 1: Every command is logged by default, linked to its run

**What changes**: Out of the box, every Spektacular command run in a project leaves one JSON line in `.spektacular/logs/`. The line records:
- when the command started and ended, and whether it succeeded or failed, with the error message on failure;
- which agent, workflow and step it belonged to, and the step it moved the workflow to;
- a run identifier shared by every command of the same workflow run.

That identifier survives an interruption and resume, changes on a `--force` restart, differs between epic planning's parallel lanes, and is added to a run that was already in progress when the project upgraded. The `debug` setting is gone. `migrate` turns it into the new `log` setting (`off`, `standard` or `verbose`, with a choice of logger and directory) and adds the logs folder to the project's ignore rules, as `init` now does for new projects. The verbose level keeps the full responses and state snapshots that debug mode kept. `spektacular status` is never logged, so a tool that polls it does not flood the log, and `SPEKTACULAR_SESSION_LOG` is gone: the level is set in the project's configuration only. Logging can never change a command's output or exit code, and the old debug files are left where they are.

**Validation point**: The full Go suite passes at the default level, and the `cmd` suite also passes with logging forced off through its test-only `-log-level=off` flag. Running `status` adds nothing to the logs. In a throwaway `init`ed git project, a spec workflow's commands appear in one run log with one trace id, standalone commands appear in that day's log with their own ids, and `git status` shows no log files. Migrating a format-4 throwaway project with `debug.enabled: true` yields `log.level: verbose` and an untouched `debug/` folder.

#### - [x] Task: Trace log package with the file logger
**Id:** df1bade8-66a7-4bb8-b261-3dda877ae82c
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds the package that defines the log record the whole epic shares: an OpenTelemetry-model span with trace and span ids, name, start and end times, status, Spektacular attributes and resource. The package also mints those ids and chooses a logger by provider name. Its only logger, `file`, appends each record as one JSON line to a per-run or per-day file, keeps a self-ignoring `.gitignore` in its directory, and swallows every failure so writing a log can never affect a command. Retention is added in Milestone 2.

*Technical detail:* [context.md#task-trace-log-package-with-the-file-logger](./context.md#task-trace-log-package-with-the-file-logger)

**Acceptance criteria**:
- [x] A record serialises with the field names in the plan's Data Structures section, omitting the parent span id and the error message when they are empty.
- [x] Trace ids are 32 and span ids 16 lowercase hex characters, never all zeros, and never repeat across many mintings.
- [x] A workflow run's records land in a file named from the run's start time, agent, kind and name; a standalone command's record lands in that UTC day's standalone file.
- [x] Writing appends exactly one line without reading the file's earlier contents, and the log directory contains a `.gitignore` that ignores everything in it.
- [x] Asking for the `file` logger succeeds; asking for any other name fails with an error that names the accepted loggers.
- [x] With an unwritable log directory, writing returns normally, with no panic and nothing printed.

#### - [x] Task: Run identifier in workflow state
**Id:** 9057f37e-b995-499e-a5dc-867e3b219337
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Gives every workflow run a run identifier held in its own state, the only place the spec allows it. The workflow engine mints one whenever it creates fresh state, so every `new` (and every orchestrated lane) starts a new run while resuming keeps the old one. A helper gives a run that was started before the upgrade an identifier the first time it is needed, changing nothing else in its state file.

*Technical detail:* [context.md#task-run-identifier-in-workflow-state](./context.md#task-run-identifier-in-workflow-state)

**Acceptance criteria**:
- [x] A newly started workflow's state carries a run identifier in the same format as a trace id.
- [x] Reloading and advancing an existing workflow keeps its run identifier unchanged.
- [x] An in-progress state written without a run identifier gains one through the helper, and every other field, including keys the current code does not know, is preserved.
- [x] The helper leaves a finished state, a missing state and a state that already has an identifier untouched.

#### - [x] Task: Log settings replace debug, with migration
**Id:** 154f3002-9b09-4cb4-aaec-f204081cee9a
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Replaces the project's `debug` setting with a `log` section: which logger to use, the level (`off`, `standard` or `verbose`), how many runs to keep, and the directory. Each has a default, and a bad value is refused with a fix-it message. The settings format moves to 5. `migrate` converts an existing debug setting, debug on to verbose and debug off to standard, and adds the logs folder to the project's ignore rules. `init` installs that ignore rule for new projects, and this repository ignores its own logs folder.

*Technical detail:* [context.md#task-log-settings-replace-debug-with-migration](./context.md#task-log-settings-replace-debug-with-migration)

**Acceptance criteria**:
- [x] A new project's configuration has logging on at the standard level, with the file logger, retention 10 and the `logs` directory, and no `debug` section.
- [x] An unknown logger name, an unknown level or a retention below 1 is refused as a configuration error naming the accepted values and the key to change.
- [x] A relative log directory resolves from the folder holding the configuration file, and an absolute one is used as written.
- [x] Migrating a project with debug turned on gives the verbose level, with debug off or absent the standard level, and in every case the debug setting is gone and other settings are unchanged.
- [x] After `init`, and after `migrate` of an older project, the project's ignore rules include the logs folder exactly once, and a migration preview lists that change.
- [x] Migration never reads, moves or deletes anything in the old debug folder.

#### - [x] Task: Record every command through the configured logger
**Id:** dd495862-2f2e-430b-b8ae-ef5b52cb2078
**Repo:** spektacular
**Depends on:**
- df1bade8-66a7-4bb8-b261-3dda877ae82c — Trace log package with the file logger
- 9057f37e-b995-499e-a5dc-867e3b219337 — Run identifier in workflow state
- 154f3002-9b09-4cb4-aaec-f204081cee9a — Log settings replace debug, with migration
**Execution:** agent

Rewires the place every command finishes so that it logs through the configured logger instead of the debug recorder. It does six things:
- works out the effective level from the project settings alone (`SPEKTACULAR_SESSION_LOG` is no longer read);
- leaves out any command marked as never logged, through a cobra annotation that `status` now carries and that spec 000075 will put on `stats`;
- gives an older run its identifier;
- decides which run, step and next step a command belongs to;
- captures the full response only at the verbose level;
- writes one record.

The old session-log package and its tests are replaced, and logging stays inside projects only. The `cmd` tests gain a test-only way to force logging off across the whole suite.

*Technical detail:* [context.md#task-record-every-command-through-the-configured-logger](./context.md#task-record-every-command-through-the-configured-logger)

**Acceptance criteria**:
- [x] In a freshly initialised project with no settings changed, any command leaves a record of itself in the project's logs directory.
- [x] A `spec goto` record carries the configured agent (or `unknown` when none is set), the workflow kind and name, the step it ran in and the step it moved to.
- [x] A spec run's start, gotos, resume report and later gotos share one trace id; a `--force` restart gets a new one; two parallel plan lanes each keep their own constant trace id.
- [x] A spec workflow whose state was written by the previous version resumes without error, and every record from then on carries the same trace id.
- [x] Two standalone `knowledge search` commands produce two records with different trace ids in the same day's log.
- [x] At the standard level a record holds the command, timing, exit status and any error message but no response or state; a failed command records an error status, a non-zero exit code and its message.
- [x] At the verbose level a record holds the exact output and the state before and after.
- [x] With logging off nothing is written, and nothing is ever written outside a project. Setting `SPEKTACULAR_SESSION_LOG` to any value changes nothing.
- [x] At the standard and verbose levels, `status` (readable or JSON, with or without a name) adds no record, creates no log file and does not give an older run its identifier, while the next workflow command is still recorded.
- [x] A command marked with the logging-exempt annotation, or one whose parent is marked, is never logged; the annotation's key and value are named constants that another command group can set.
- [x] A custom log directory receives the records and the default one stays empty.
- [x] After `init` in a new git repository, and after `migrate` of an older project, running commands leaves `git status` free of log files.
- [x] Every command's output and exit code are identical with logging off, standard and verbose, including when the logs directory cannot be written. The full test suite passes with logging on, and the `cmd` suite also passes with logging forced off through its test-only flag.
- [x] Files in an old `.spektacular/debug/` folder are byte-identical after `migrate` and further commands.

### Milestone 2: Old logs are trimmed automatically

**What changes**: The logs folder no longer grows without limit. Spektacular keeps the logs of the 10 most recent workflow runs (or the number set in `log.retention`) and deletes older ones when a new run starts. Commands run outside any workflow are kept as one log per day, with the same number of most recent days kept separately, so a burst of standalone commands never pushes out a workflow run's log. A log that cannot be deleted is skipped silently, never failing the command.

**Validation point**: Integration tests show that 12 runs leave 10 logs, retention 3 leaves 3, standalone commands in between remove no run log, and an undeletable old log leaves the command's output and exit code unchanged.

#### - [ ] Task: Keep only the most recent runs and days
**Id:** d1f4fa2f-933e-4a13-b19d-a2af3f910e2d
**Repo:** spektacular
**Depends on:**
- dd495862-2f2e-430b-b8ae-ef5b52cb2078 — Record every command through the configured logger
**Execution:** agent

Adds retention to the file logger. When a record starts a new run log, older run logs beyond the configured count are deleted. When a record starts a new day's standalone log, older day logs beyond the same count are deleted. The two groups are counted separately, so standalone commands never push out a workflow run. A log that cannot be deleted is skipped without affecting the command.

*Technical detail:* [context.md#task-keep-only-the-most-recent-runs-and-days](./context.md#task-keep-only-the-most-recent-runs-and-days)

**Acceptance criteria**:
- [ ] With the default retention, after 12 separate workflow runs the logs directory holds the logs of the 10 most recent runs only.
- [ ] With retention set to 3, it holds the 3 most recent runs' logs.
- [ ] Many standalone commands run on different days in between remove no workflow run's log, and only the configured number of most recent day logs is kept.
- [ ] The log being written to is never deleted by its own sweep.
- [ ] When an old log cannot be deleted, the command's output and exit code are identical to a run with logging off.

### Milestone 3: The Harbor test harness reports from the new logs

**What changes**: Harbor, the end-to-end agent test harness, keeps reporting the same workflow steps, timelines, parallel runs and metrics it reports today, now read from `.spektacular/logs/`. Every suite's seeded project records the agent's run at the verbose level, set in its configuration rather than through an environment variable. Seeding and the verifiers' own calls are kept out of the log. The shared reader understands the new span records and still reads the old-format reference runs its offline tests rely on. Users see no change in Harbor's results. The work exists so the harness does not break, until spec 000076 moves it onto `spektacular stats`.

**Validation point**: Harbor's Python unit tests pass, including new cases for new-format records and the unchanged pinned reference-run values. A grep of `tests/harbor` finds no `.spektacular/debug` path outside the old-format fixtures and their trimming script, and no `SPEKTACULAR_SESSION_LOG` anywhere. A real suite run is checked by hand (see Testing Approach).

#### - [ ] Task: Harbor reader understands span records
**Id:** 2a0cfbab-fa3b-43e3-89cc-15ac23bdca40
**Repo:** spektacular
**Depends on:**
- df1bade8-66a7-4bb8-b261-3dda877ae82c — Trace log package with the file logger
**Execution:** agent

Teaches Harbor's shared session-log reader to read the new span records, turning each into the same event it builds from an old debug record today: time, command, duration, exit code, state before and after, and response. Old-format lines still read exactly as before, so the checked-in reference runs and every existing offline test keep their results.

*Technical detail:* [context.md#task-harbor-reader-understands-span-records](./context.md#task-harbor-reader-understands-span-records)

**Acceptance criteria**:
- [ ] A new-format record and its old-format equivalent produce identical events.
- [ ] A directory mixing both formats is read in full, with lanes and shared events split by file name as before.
- [ ] A verbose resume refusal in the new format is still detected as a resume.
- [ ] All existing Harbor shared unit tests, including the pinned reference-run values, pass unchanged.

#### - [ ] Task: Harbor suites read the new logs at the verbose level
**Id:** 90909dbf-daa8-4bb5-a55f-571a60355636
**Repo:** spektacular
**Depends on:**
- 2a0cfbab-fa3b-43e3-89cc-15ac23bdca40 — Harbor reader understands span records
- dd495862-2f2e-430b-b8ae-ef5b52cb2078 — Record every command through the configured logger
**Execution:** agent

Points Harbor at `.spektacular/logs/` wherever it looked in `.spektacular/debug/`: every suite verifier, the timeline and metrics tools' defaults, and the `test.sh` timeline calls. Every suite that drives Spektacular sets `log.level: verbose` in the configuration of the project it seeds, so the responses and state snapshots Harbor reads are kept. The images no longer set `SPEKTACULAR_SESSION_LOG`. Seed scripts run their own commands with logging off, so the log holds only the agent's run. Verifiers that call `spektacular` themselves switch logging off first, so their calls stay out of the log they check. Spec 000076 later sets the seeded level back to the default. Comments, messages and the README's Harbor sections are updated to match.

*Technical detail:* [context.md#task-harbor-suites-read-the-new-logs-at-the-verbose-level](./context.md#task-harbor-suites-read-the-new-logs-at-the-verbose-level)

**Acceptance criteria**:
- [ ] Every suite that drives Spektacular starts the agent's run with `log.level: verbose` in its project configuration, whether the project is pre-built or the agent runs `init` itself. No image, script or verifier sets `SPEKTACULAR_SESSION_LOG`.
- [ ] The pre-built projects' logs directories hold no record of the seed scripts' own commands when the agent starts.
- [ ] Every verifier that runs `spektacular` switches the project's logging off before its first call, so none of its calls appear in the session log.
- [ ] No verifier, shared tool or `test.sh` reads `.spektacular/debug` any more, apart from the old-format test fixtures and their trimming script.
- [ ] The metrics recorder's pinned command line in `test.sh` is unchanged and still finds the session log by default.
- [ ] The README's Harbor sections name `.spektacular/logs/` and the seeded verbose level, and the Harbor shared unit tests and suite self-checks pass.

### Milestone 4: Logging is documented

**What changes**: The documentation site's debugging page, retitled "Logging and debugging", explains logging:
- what the standard and verbose levels record, with a sample line;
- where logs go and how to move them;
- how many runs are kept;
- how to switch logging off;
- which commands are never logged: `status`, so polling it leaves no trace, and the read-only `stats` reports.

The configuration reference documents the `log` settings and no longer mentions `debug`. The project README and the knowledge base describe the new logs instead of the debug log. Nothing describes an environment variable for logging.

**Validation point**: The site builds and `astro check` passes with no layout HTML in page bodies. Grepping the site, the README and the knowledge base finds no remaining `debug.enabled`, `.spektacular/debug` or `SPEKTACULAR_SESSION_LOG` description of current behaviour.

#### - [ ] Task: Documentation site describes logging
**Id:** 6f66ab15-ff74-47ff-bea3-f3d5adbfb1c9
**Repo:** docs
**Depends on:**
- dd495862-2f2e-430b-b8ae-ef5b52cb2078 — Record every command through the configured logger
- d1f4fa2f-933e-4a13-b19d-a2af3f910e2d — Keep only the most recent runs and days
**Execution:** agent

Rewrites the site's debugging page, retitled "Logging and debugging" with its address unchanged, to explain logging as shipped. The configuration reference swaps the `debug` key and example lines for the `log` section, and the navigation label follows the new title.

**Content outline** (debugging page, in order; the wording is illustrative, the keys and values are fixed):
1. Hero: heading "Logging and debugging", sub "Every Spektacular command leaves one JSON line, so you can see exactly what happened in a run."
2. `## Logging is on by default`: every command run in a project is recorded to `.spektacular/logs/`. The folder is already git-ignored by `init` and `migrate`. Example block:
   ```yaml
   log:
     provider: file        # the logger; only file ships today
     level: standard       # off | standard | verbose
     retention: 10         # most recent workflow runs kept
     config:
       directory: logs     # relative to the folder holding config.yaml
   ```
3. `## What each level records`. `standard`: the command, its run, agent, workflow and step, start and end time, exit status and error message. `verbose`: adds the full response and the workflow state before and after, as debug mode did. `off`: nothing.
4. `## Where logs go`: one file per workflow run, e.g. `20261024T090334.911373161Z_claude_plan_000008_debugging-docs.jsonl`, and one `standalone_20261024.jsonl` per day for commands outside a workflow. Explains `log.config.directory`.
5. `## How long logs are kept`: the 10 most recent runs (`log.retention`), and as many days of standalone commands, counted separately.
6. `## Commands that are never logged`: `spektacular status` is never logged at any level, so editors and tools that poll it do not fill the log. Commands that only report on the logs, `spektacular stats` and its forms, are never logged either, so a report never adds to or trims the logs it reads. The level is set in `config.yaml` only: there is no environment variable for it. (If `stats` has not shipped when this task runs, the section states the rule for reporting commands and names `status` only, and spec 000075's docs sentence names `stats`.)
7. `## Reading a record`: a sample standard-level line (from the plan's Data Structures section) and a short field list: `trace_id` (one per run), `span_id`, `name`, `start_time`/`end_time`, `status`, `attributes` (`spektacular.agent`, `spektacular.workflow.kind`/`.name`, `spektacular.step`, `spektacular.step.to`, `process.command_args`, `process.exit_code`).
8. `## Upgrading from debug mode`: `migrate` turns `debug.enabled: true` into `level: verbose` and false into `standard`, and leaves old `.spektacular/debug/` files for you to delete.
9. CtaBanner: "The `log` key is one of several top-level sections in `.spektacular/config.yaml`" and the existing button to the Configuration page.

**Content example** (configuration reference `log` key, replacing `debug` in the same position, with the frontmatter description and "Seventeen top-level keys" list updated):
```mdx
<ConfigKey name="log" type="section" defaultValue="<code>level: standard</code>, <code>retention: 10</code>, <code>logs</code>">

  How Spektacular logs the commands it runs.

  - `log.provider`: the logger; only `file` ships today.
  - `log.level`: `off`, `standard` (default) or `verbose`.
  - `log.retention`: how many recent workflow runs to keep, and how many days of standalone commands. Defaults to 10.
  - `log.config.directory`: where logs are written, relative to the folder holding `config.yaml`. Defaults to `logs`.

  See [Logging and debugging](/debugging/) for what each level records.

</ConfigKey>
```

*Technical detail:* [context.md#task-documentation-site-describes-logging](./context.md#task-documentation-site-describes-logging)

**Acceptance criteria**:
- [ ] The debugging page explains the levels and what each records, the logs location and how to change it, retention, switching logging off, and that `status` and the `stats` reports are never logged; it describes no environment variable.
- [ ] The configuration reference documents every `log` setting with its default, and no longer documents `debug` anywhere, including the example file and the key list.
- [ ] The navigation shows the page as "Logging and debugging" at its existing address.
- [ ] The site builds and type-checks without errors, and no page body contains layout HTML or em dashes in the new text.

#### - [ ] Task: README and knowledge describe logging
**Id:** 6ec1a2f7-0b73-4c90-b1a2-cf897ed8d097
**Repo:** spektacular
**Depends on:**
- dd495862-2f2e-430b-b8ae-ef5b52cb2078 — Record every command through the configured logger
- d1f4fa2f-933e-4a13-b19d-a2af3f910e2d — Keep only the most recent runs and days
- 90909dbf-daa8-4bb5-a55f-571a60355636 — Harbor suites read the new logs at the verbose level
**Execution:** agent

Brings the project's own descriptions up to date. The README's configuration example shows the `log` section instead of `debug`, and says `status` is never logged. The two knowledge entries that describe Harbor's use of the debug session log now describe `.spektacular/logs/` and the verbose level each suite sets in its seeded project's configuration. Historical plans and changelog entries are left as they are.

*Technical detail:* [context.md#task-readme-and-knowledge-describe-logging](./context.md#task-readme-and-knowledge-describe-logging)

**Acceptance criteria**:
- [ ] The README's configuration example shows the `log` section with its defaults and a one-line explanation of each key, mentions `debug` nowhere as a current setting, and describes no logging environment variable.
- [ ] The Harbor knowledge entries say the session log is collected from `.spektacular/logs/` at the verbose level set in each suite's seeded configuration, and no longer mention `.spektacular/debug/` or `SPEKTACULAR_SESSION_LOG` as current behaviour.

## Open Questions

- **Whether the default level meets the under-10 ms budget on the maintainer's machine.** This depends on the real cost of loading the configuration twice, reading the state twice and appending once, measured on the installed binary. If the manual measurement in the test plan exceeds 10 ms, STOP and ask the user before adding caching, for example passing the already-parsed configuration from the root probe into the command handlers. That would change how commands load configuration.
- **Whether any existing `cmd` test relies on `.spektacular/` holding no extra entries after commands.** This only surfaces once logging is on by default across the suite. Adjust such a test to ignore `logs/` when the assertion is about other files. If a test asserts behaviour the spec now changes in some other way, STOP and ask.
- **Whether a minimal pre-seeded `config.yaml` changes what the agent's `init` does in the spec, plan and repo suites.** Those suites have the agent run `init` itself, so their verbose level comes from a pre-seeded `schema: 5` file holding only `log.level: verbose`. `init` keeps an existing config and backfills the name, the colocated repo and the agent, as the implement suite's pre-seeded config already relies on. If any of those suites' verifiers or reported steps change because the config exists before `init`, STOP and ask.

No other implementation-time uncertainties remain. Every design choice is settled in the Architecture & Design Decisions section and the assumption log.

## Out of Scope

- **Step spans, task spans, agent-started spans and user-wait spans.** These include `goto` opening and closing step spans, implement's task spans, and `trace start`/`trace stop` for helpers and user waits. They are spec `000074_trace-spans-for-agent-work`. This plan only reserves room for them: the `record` discriminator, the `spektacular.span.type` attribute and the optional `parent_span_id`.
- **Reading, summarising or comparing logs** (`spektacular stats`): spec `000075_run-stats-command`. Commands that only read the logs are never logged themselves, so a report cannot add to or trim the logs it reads. This plan owns the mechanism: the `logAnnotation: logExempt` cobra annotation, its parent walk and its check in `runRoot`. It marks `status` with it. Spec 000075 only sets the same annotation on its `stats` group (epic review decision, user-confirmed 2026-10-25).
- **Any per-command logging override.** `SPEKTACULAR_SESSION_LOG` is removed, not remapped, and nothing replaces it (epic review decision, 2026-10-25). Dropping the variable from the maintainer's `spek-status` mod is outside this repository, and harmless if it is left in place, since nothing reads it.
- **Replacing Harbor's transcript and session-log parsers.** These belong to spec `000076_harbor-on-built-in-logs`. This plan only keeps Harbor's existing reporting working, through a normalising reader.
- **A visual or HTML view of a run**: spec `000077_html-log-viewer`.
- **Loggers other than `file`**: exporting to an OpenTelemetry collector, standard output or anything else is deferred (spec Non-Goals). The provider registry is the extension point.
- **Recording agent tool calls, model calls or token usage**, which the CLI cannot see (spec Non-Goals).
- **Retention by age or by total size.** Only a count is configurable (spec Non-Goals).
- **Removing or converting old `.spektacular/debug/` files.** They are never read, moved or deleted, and removing them is left to the user (spec requirement).
- **Migrating this repository's own `.spektacular/`** (config format 5, `.spektacular/.gitignore`). No task does this, per the plans-never-change-the-active-install convention. The maintainer installs the new binary between workflows, and the next skill version check prompts a migration.
- **Editing historical plans, test plans and changelog entries** that mention `.spektacular/debug/`. They record past intent and stay as written.
- **Converting Harbor's checked-in reference runs to the new format.** They stay old-format and are read through the dual-format reader until spec 000076.

## Changelog

### 2026-10-25 — Task: Trace log package with the file logger

**What was done**: Added `internal/tracelog`, a leaf package holding the OpenTelemetry-model span `Record` and `Status`, the `Run` a record belongs to, the attribute and resource key constants, trace and span id minting from `crypto/rand`, and a `Logger` interface behind a provider registry whose only entry is `file`. The file logger appends one JSON line per record to a per-run file (`<start>_<agent>_<kind>_<name>.jsonl`) or a per-UTC-day `standalone_<day>.jsonl`, writes a `*` `.gitignore` when it starts a file, and contains every failure and panic.

**Deviations**: Added two small helpers beyond the plan's sketch: `Run.Standalone()` and `Resource(version)`, which builds the `service.name`/`service.version` map. The unknown-provider error also carries `WithResource(provider)`. The retention `sweep` is a no-op hook until the retention task, as planned.

**Files changed**:
- `spektacular: internal/tracelog/record.go`
- `spektacular: internal/tracelog/ids.go`
- `spektacular: internal/tracelog/logger.go`
- `spektacular: internal/tracelog/file.go`
- `spektacular: internal/tracelog/record_test.go`
- `spektacular: internal/tracelog/ids_test.go`
- `spektacular: internal/tracelog/logger_test.go`
- `spektacular: internal/tracelog/file_test.go`

**Discoveries**: The accepted logger names appear in the error's `next_action`, not its message. The provider pin test against `config.LogProviders` belongs to the log settings task, since that list does not exist yet.

### 2026-10-25 — Task: Run identifier in workflow state

**What was done**: `workflow.State` gained a top-level `run_id` (`omitempty`), minted by `workflow.New` whenever it builds fresh state, so every `new` and every orchestrated lane starts a run with its own id and a resume keeps it. Added `NewRunID` (32 lowercase hex, never all zeros) and `EnsureRunID(path)`, which gives an in-progress state written without an id one, editing the file as a generic JSON map and writing it through a temp file and rename.

**Deviations**: `EnsureRunID` decodes with `UseNumber`, so numbers in the state survive exactly; key order is re-sorted by the map re-marshal, which keeps every field and value but not the original key order. The temp file keeps the original file's permissions.

**Files changed**:
- `spektacular: internal/workflow/state.go`
- `spektacular: internal/workflow/workflow.go`
- `spektacular: internal/workflow/workflow_test.go`
- `spektacular: internal/workflow/state_test.go`

**Discoveries**: None.

### 2026-10-25 — Task: Log settings replace debug, with migration

**What was done**: `config.DebugConfig` is replaced by `LogConfig` (`provider`, `level`, `retention`, `config.directory`) with defaults `file`, `standard`, `10`, `logs`, validation that refuses an unknown logger, an unknown level, a retention below 1 or an empty directory with `config_invalid` and a next action, `EffectiveLevel()`, and `Config.LogDir(projectRoot)` resolving the directory from the folder holding `config.yaml`. The project settings format is now 5. A new `project4to5` migrate step maps `debug.enabled: true` to `log.level: verbose` (false or absent to `standard`), writes the other `log` defaults without overwriting existing keys, deletes `debug`, and reports a new idempotent `ignore` action that appends `logs/` to `.spektacular/.gitignore`. `init`'s `.gitignore` template and the repo's root `.gitignore` ignore the logs folder.

**Deviations**: Removing `debug` is reported as a `set` action with an empty `To` (`Key: "debug.enabled", From: "<old>"`), documented on `Action`. To keep the tree compiling before the recorder task, `cmd/root.go` temporarily treats `log.level: verbose` as the old `debug.enabled: true` (one-line bridge, replaced by the recorder task). The schema bump required updating every test fixture that writes a current-format config (`schema: 4` to `schema: 5`) across `cmd`, `internal/config`, `internal/design`, `internal/project` and `internal/migrate`, plus the migrate goldens and the synthetic extensibility step (now 5 to 6). The session-log tests' `debug.enabled` toggles became `log.level` values. Added cmd-level `migrate` tests for a format-4 project with debug on and for the dry-run preview.

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/schema.go`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/config/schema_test.go`
- `spektacular: internal/config/repo_test.go`
- `spektacular: internal/tracelog/providers_pin_test.go`
- `spektacular: internal/migrate/steps_project.go`
- `spektacular: internal/migrate/registry.go`
- `spektacular: internal/migrate/engine.go`
- `spektacular: internal/migrate/engine_test.go`
- `spektacular: internal/migrate/steps_project_test.go`
- `spektacular: internal/migrate/testdata/current/config.yaml`
- `spektacular: internal/migrate/testdata/golden/legacy_single/config.yaml.golden`
- `spektacular: internal/migrate/testdata/golden/split_unversioned/config.yaml.golden`
- `spektacular: internal/project/init_test.go`
- `spektacular: internal/design/design_test.go`
- `spektacular: templates/.spektacular/.gitignore`
- `spektacular: .gitignore`
- `spektacular: cmd/root.go`
- `spektacular: cmd/migrate_test.go`
- `spektacular: cmd/gate_test.go`
- `spektacular: cmd/format_refusal_test.go`
- `spektacular: cmd/init_test.go`
- `spektacular: cmd/version_test.go`
- `spektacular: cmd/standing_rules_test.go`
- `spektacular: cmd/root_test.go`
- `spektacular: cmd/sessionlog_lane_test.go`

**Discoveries**: A settings format bump touches every test that hand-writes a current-format `config.yaml`: the literal `schema: N` appears in about a dozen test files, and `TestCurrentProjectSchema_UnchangedByImplementSection` pins the number.

### 2026-10-25 — Task: Record every command through the configured logger

**What was done**: `runRoot` now logs every command run in a project through the logger `log.provider` names, at the level `log.level` sets (standard by default), replacing the debug recorder. It resolves the level from configuration alone (off outside a project, when the config does not load, or for a logging-exempt command), backfills a legacy run id with `workflow.EnsureRunID`, snapshots the state before and after, attributes the record to the run in progress after, else before, else a standalone trace, tees output only at verbose, and builds one `tracelog.Record` under a recover. The `log: exempt` cobra annotation (`logAnnotation`, `logExempt`, `logExempted` parent walk) is checked before and after the command; `status` carries it. `SPEKTACULAR_SESSION_LOG`, `isWorkflowStart`, `stateAdvanced`, `sessionLogDir` and `internal/sessionlog` are gone. The `cmd` tests gained a test-only `-log-level` flag (`forcedLogLevel`) and `useConfiguredLogLevel(t)`, and the session-log tests were rewritten onto the new log.

**Deviations**: `logLevel` takes the configured level string rather than the whole config. Existing tests that snapshot a project tree across a logged command now skip `.spektacular/logs/` in their shared snapshot helpers (`snapshotDir`, `snapshotTree`, `listPaths`, `hashStore`, `addrSnapshot`), and `writeInProgressState` gives its fixture a fixed run id, since the pre-command backfill would otherwise change the fixture's bytes. Logging stays on in the default `cmd` run.

**Files changed**:
- `spektacular: cmd/root.go`
- `spektacular: cmd/status.go`
- `spektacular: cmd/main_test.go`
- `spektacular: cmd/log_test.go`
- `spektacular: cmd/log_lane_test.go`
- `spektacular: cmd/sessionlog_lane_test.go` (removed)
- `spektacular: cmd/root_test.go`
- `spektacular: cmd/init_test.go`
- `spektacular: cmd/epic_test.go`
- `spektacular: cmd/repo_test.go`
- `spektacular: cmd/knowledge_prune_test.go`
- `spektacular: cmd/storefile_address_test.go`
- `spektacular: cmd/implement_test.go`
- `spektacular: internal/sessionlog/record.go` (removed)
- `spektacular: internal/sessionlog/record_test.go` (removed)

**Discoveries**: `init` itself is never logged, since the level is decided before the project config exists. The no-data `spec new` resume report is a refusal (exit 1, `workflow_in_progress`), so it is recorded as an error span that still shares the run's trace. Any command run while a workflow is in progress (a `knowledge search`, `skill list`) is filed under that run, not the standalone day file. In tests the pre-command exemption check reads `os.Args`, so a test that needs it must set `os.Args` (`runDebugCmd`).
