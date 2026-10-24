---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Context: 000073_log-mode-and-logger-providers

## Current State Analysis

- `cmd/root.go:110-176` (`runRoot`) records a debug session log only when `debug.enabled` is true or `SPEKTACULAR_SESSION_LOG=on`: argv, duration, exit code, the full response (teed from stdout), and state snapshots before and after, written by `internal/sessionlog` to `.spektacular/debug/<ts>_<agent>_<kind>_<name>.jsonl`. The session id is `kind:name` of the after-state (`internal/sessionlog/record.go:38-46`). A new file is minted on each successful `<kind> new` (`cmd/root.go:178-205`). Each write counts the file's lines for `seq` (`record.go:166-174`). There is no retention.
- The lane-by-argument state selection (`cmd/root.go:247-345`) already files each orchestrated child's commands under its own lane state file (`internal/workflow/lane.go:20-22`).
- Workflow state (`internal/workflow/state.go:14-21`) has no run identifier. Fresh state is built only in `workflow.New` (`workflow.go:111-117`), and `new` clears the old state first (`cmd/resume.go:216-218`).
- Config has `debug.enabled` (`internal/config/config.go:84-87,395,423-425`) at settings format 4 (`internal/config/schema.go:17-20`). Migration steps edit YAML trees (`internal/migrate/steps_project.go:139-160`), and the engine performs only `create`/`remove` actions (`internal/migrate/engine.go:320-340`).
- `init` writes `templates/.spektacular/.gitignore`, which has no logs rule (`*.log` does not match `.jsonl`). `migrate` never rewrites it. This repo's root `.gitignore:44` ignores `.spektacular/debug/`. Auto-commit uses `git add -A` (`internal/autocommit/git.go:92-93`).
- Harbor sets `SPEKTACULAR_SESSION_LOG=on` in nine suite images and reads `.spektacular/debug` through `tests/harbor/shared/sessionlog.py`. That reader needs `state_before`/`state_after` and, for resume detection and epic worktree paths, `response`. The maintainer's `spek-status` mod sets `SPEKTACULAR_SESSION_LOG=off` on every poll.
- The docs site's `src/pages/debugging.mdx` documents debug mode, `src/pages/configuration.mdx:200-207` documents the `debug` key, and `src/components/Nav.astro:22` links "Debugging".

## Per-Task Technical Notes

### Requirement to repo and files

| Requirement (spec) | Repo | Files |
|---|---|---|
| Every command logged by default; recorded once; agent/run/step; one trace per run; pre-upgrade runs; default vs verbose; switch off; env per command | spektacular | `cmd/root.go`, `internal/tracelog/*`, `internal/workflow/{state,workflow}.go`, `internal/config/config.go` |
| Logger chosen in configuration; log location configurable | spektacular | `internal/config/config.go`, `internal/tracelog/logger.go` |
| Logs stay out of version control | spektacular | `templates/.spektacular/.gitignore`, `internal/migrate/{steps_project,engine,registry}.go`, `internal/tracelog/file.go`, `.gitignore` |
| Only the most recent runs are kept | spektacular | `internal/tracelog/file.go` |
| Logging never breaks a command | spektacular | `internal/tracelog/file.go`, `cmd/root.go` |
| Debug configuration migrated; old debug logs left alone | spektacular | `internal/migrate/steps_project.go`, `internal/config/schema.go` |
| Test harness keeps its session reporting | spektacular | `tests/harbor/shared/*.py`, `tests/harbor/*/tests/*`, `tests/harbor/*/environment/Dockerfile` |
| Documentation describes logging | docs, spektacular | `docs:src/pages/debugging.mdx`, `docs:src/pages/configuration.mdx`, `docs:src/components/Nav.astro`, `README.md`, knowledge entries |

### Task: Trace log package with the file logger

**File changes**
- `internal/tracelog/record.go` (new): `Record`, `Status` (`Code` json `code`, `Message` json `message,omitempty`), `Run`. Constants:
  - `RecordSpan = "span"`, `KindInternal = "internal"`, `TypeCommand = "command"`, `StatusOK = "ok"`, `StatusError = "error"`;
  - attribute keys: `AttrSpanType = "spektacular.span.type"`, `AttrAgent = "spektacular.agent"`, `AttrWorkflowKind = "spektacular.workflow.kind"`, `AttrWorkflowName = "spektacular.workflow.name"`, `AttrStep = "spektacular.step"`, `AttrStepTo = "spektacular.step.to"`, `AttrCommandArgs = "process.command_args"`, `AttrExitCode = "process.exit_code"`, `AttrResponse = "spektacular.response"`, `AttrStateBefore = "spektacular.state.before"`, `AttrStateAfter = "spektacular.state.after"`.

  `StartTime`/`EndTime` are `time.Time`, marshalled in UTC (call `.UTC()` before building). Resource keys are `service.name` = `spektacular` and `service.version` = the build version, passed in by the caller.
- `internal/tracelog/ids.go` (new):
  - `NewTraceID()`: 16 bytes from `crypto/rand`, hex-encoded, retried if all zero;
  - `NewSpanID()`: the same with 8 bytes;
  - on a rand failure, fall back to a time-derived id rather than error (logging must never fail).

  Model on `internal/identifier/taskid.go:56-64`.
- `internal/tracelog/logger.go` (new):
  - `type Logger interface{ Write(run Run, rec Record) }`;
  - `type FileOptions struct{ Directory string; Retention int }`;
  - a registry `providers = map[string]func(FileOptions) Logger{"file": newFileLogger}`;
  - `New(name string, opts FileOptions) (Logger, error)`: an unknown name returns `output.NewError("config_invalid", fmt.Sprintf("log.provider %q is not a known logger", name)).WithNextAction("set log.provider in .spektacular/config.yaml to one of: " + strings.Join(Providers(), ", "))`, following `internal/identifier/taskid.go:36-52`;
  - `Providers()` returns the sorted names.

  Importing `internal/output` is acceptable (it is a leaf too). Do not import `internal/config` or `internal/workflow`.
- `internal/tracelog/file.go` (new), evolved from `internal/sessionlog/record.go:57-174`:
  - `fileLogger{dir string; retention int}`.
  - `Write` has a `defer func(){ _ = recover() }()`.
  - `path := runFilePath(dir, run)`:
    - a workflow run (`run.Kind != ""`) uses `run.StartedAt.UTC().Format("20060102T150405.000000000Z") + "_" + agentOr(run.Agent) + "_" + sanitize(run.Kind) + "_" + sanitize(run.Name) + ".jsonl"`;
    - a standalone run uses `"standalone_" + run.StartedAt.UTC().Format("20060102") + ".jsonl"`;
    - `sanitize` replaces `:`, `/`, `\` with `_` (keep `sanitizeSessionID`, `record.go:107-109`).
  - `_, statErr := os.Stat(path); created := os.IsNotExist(statErr)`.
  - If created: `os.MkdirAll(dir, 0o755)`, then write `dir/.gitignore` with content `*\n` if absent.
  - `json.Marshal(rec)`, append `\n`, `os.OpenFile(path, O_APPEND|O_CREATE|O_WRONLY, 0o644)`, a single `f.Write`, close.
  - Leave a `sweep(created, path)` hook that is a no-op until the retention task.
  - Drop `Seq` and `countLines` entirely.
- `internal/tracelog/*_test.go` (new):
  - JSON golden of a sample record (field names, `parent_span_id` and `message` omitted when empty);
  - id format and uniqueness over 10k mints;
  - run and day file names;
  - two writes give two lines, and the second write does not read the file (assert by writing to a file pre-seeded with 10 MB and checking it stays fast, or simply that the content is the previous bytes plus one line);
  - `.gitignore` created with `*`;
  - unwritable dir (`chmod 0o500` parent; skip as root) gives `Write` returning with no panic;
  - `New("bogus", ...)` errors naming `file`.
- `internal/sessionlog/` is deleted in the recorder task, not here, because `cmd/root.go` still imports it until then.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential: record and ids first, then the file logger, then tests.

### Task: Run identifier in workflow state

**File changes**
- `internal/workflow/state.go:14-21`: add `RunID string \`json:"run_id,omitempty"\`` to `State`. Add `NewRunID()` (16 random bytes, hex, the same as the trace id format; duplicated rather than importing `tracelog`, keeping `workflow` independent). Add `EnsureRunID(path string) (string, error)`:
  - read the file into `map[string]any`, returning `"", nil` if it is missing;
  - if `current_step` is empty or `"finished"`, return `""`;
  - if `run_id` is a non-empty string, return it;
  - otherwise set `run_id`, `json.MarshalIndent(m, "", "  ")` (the same indent as `saveState`, `state.go:49-61`), and write via temp file plus `os.Rename` in the same directory.
- `internal/workflow/workflow.go:111-117`: in the fresh-state branch of `New`, set `RunID: NewRunID()`. The loaded-state branch (`:105-108`) is unchanged, so a legacy state without an id keeps none until `EnsureRunID` runs.
- `internal/workflow/workflow_test.go`, `internal/workflow/state_test.go` (new or extended):
  - a fresh `New` followed by `Goto` persists `run_id` (32 hex);
  - a reload keeps it;
  - `EnsureRunID` on a fixture JSON with an extra unknown key, written without `run_id`, keeps every key and value and adds `run_id`;
  - it returns the existing id unchanged;
  - a finished state and a missing file stay untouched (compare bytes).

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential.

### Task: Log settings replace debug, with migration

**File changes**
- `internal/config/config.go:84-87`: replace `DebugConfig` with `LogConfig{Provider, Level, Retention, Config FileLogConfig}` and `FileLogConfig{Directory}`. Add:
  - constants `LogLevelOff = "off"`, `LogLevelStandard = "standard"`, `LogLevelVerbose = "verbose"`, `DefaultLogDir = "logs"`, `DefaultLogRetention = 10`;
  - `var LogProviders = []string{ProviderFile}`.
- `internal/config/config.go:395`: replace `Debug DebugConfig \`yaml:"debug"\`` with `Log LogConfig \`yaml:"log"\``.
- `internal/config/config.go:423-425` (NewDefault): replace the `Debug` block with `Log: LogConfig{Provider: ProviderFile, Level: LogLevelStandard, Retention: DefaultLogRetention, Config: FileLogConfig{Directory: DefaultLogDir}}`. Do NOT add it to `storeDirs()` (`:525-532`), because logs may live outside the project root. ParseYAMLFile (`:479`) parses over NewDefault, so absent keys take defaults.
- `internal/config/config.go:689-745` (Validate): call `c.Log.Validate()` after `c.Implement.GoalCheck.Validate()` (`:728`). `LogConfig.Validate` returns `output.NewError("config_invalid", ...)` with `WithResource` and `WithNextAction` for each of these cases:
  - a provider not in `LogProviders` (the next action lists them);
  - a level not one of the three (an empty level is treated as `standard`, as `auto_commit` treats empty);
  - `Retention < 1` (an omitted key keeps the default, since it parses over NewDefault);
  - an empty `config.directory`.
- `internal/config/config.go` (new method): `func (c Config) LogDir(projectRoot string) string`. An absolute `Log.Config.Directory` is returned cleaned; otherwise it is joined with `ProjectConfigDir(projectRoot)`. It is written back verbatim by `ToYAMLFile` (`:944`) with no re-expression.
- `internal/config/schema.go:17-20`: `CurrentProjectSchema = 5`, and extend the comment ("format 5 replaces debug with log").
- `internal/config/config_test.go:29,277`: replace the `Debug` assertions with `Log` defaults and round-trip. Add validation tests for each error (assert `code` and that `next_action` names the accepted values).
- `internal/tracelog` is not imported by config. Add a test in `internal/tracelog` (`providers_pin_test.go`) asserting `config.LogProviders` equals `tracelog.Providers()`, following the `SupportedAgentNames` pin in `internal/agent`.
- `internal/migrate/steps_project.go` (append after `:160`): `project4to5 = Step{Kind: KindProject, From: 4, Description: "replace debug with the log section and ignore the logs folder", Run: runProject4to5}`. `runProject4to5`:
  - `enabled, _ := getScalar(root, "debug.enabled")`;
  - set `log.level` to `verbose` when `enabled == "true"`, else `standard`, but only if `log.level` is absent;
  - set `log.provider: file`, `log.retention: 10` (`!!int`) and `log.config.directory: logs` with the `setDefault` helper pattern from `:147-160`;
  - `deleteKey(root, "debug")` (`node.go:112`);
  - report "set" actions plus a `remove`-style report of `debug`, using `Op: "set", Key: "debug", From: "<old>", To: ""` or similar;
  - append `Action{Op: "ignore", Path: filepath.Join(sc.FileDir, ".gitignore"), To: "logs/"}`.
- `internal/migrate/registry.go:61`: `KindProject: {project1to2, project2to3, project3to4, project4to5}`. Document `"ignore"` in the `Action.Op` comment (`:48`).
- `internal/migrate/engine.go:320-340` (perform): add `case "ignore":`. Read the file (missing means empty), return if any trimmed line equals `a.To`, otherwise append (adding a leading newline when the file does not end with one) and write with `writeAtomic`. In dry-run nothing is performed (already the case, `engine.go:297`).
- `internal/migrate/testdata/current/config.yaml:1` and `internal/migrate/testdata/golden/*/config.yaml.golden:1`: bump to `schema: 5`. Replace any `debug:` block with the `log:` block the step produces. Regenerate the goldens the way existing tests do.
- `internal/migrate/steps_project_test.go` (add after `:227`):
  - format 4 with debug true gives verbose; with debug false gives standard; with no debug gives standard;
  - an existing `log.level: off` is kept;
  - `.gitignore` gains `logs/` once over two applies, and is created when missing;
  - a dry run reports the `ignore` action and changes nothing;
  - a seeded `.spektacular/debug/x.jsonl` is byte-identical afterwards.

  `registry_test.go:13` already pins contiguity.
- `templates/.spektacular/.gitignore`: add a comment and the line `logs/` ("Command logs (`log` settings) are local to each machine.").
- `.gitignore:43-44` (repo root): add `.spektacular/logs/` beside `.spektacular/debug/`. This is not an install change: it keeps this checkout clean once the new binary is installed, without migrating this repo.
- `internal/project/init_test.go:63`: assert the installed `.gitignore` contains `logs/`.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) the config package and its tests, (b) the migrate step, action and fixtures. Integrate sequentially, since the schema bump touches both.

### Task: Record every command through the configured logger

**File changes**
- `cmd/root.go:91-176`: rewrite `runRoot`.
  1. `cfg, cfgErr := loadConfigLenient()`; `level := logLevel(cfg, cfgErr == nil, projectConfigExists(), os.Getenv(sessionLogEnv))`, which replaces `sessionLogOn` (`:209-218`). Env `off` gives off; `standard` gives standard; `verbose` or `on` gives verbose, but only in a project; otherwise `cfg.Log.Level` (empty means standard). Outside a project the level is always off.
  2. If the level is not off:
     - `start := time.Now()`;
     - `snapshotPath := snapshotStatePath(argv)` (`:247`, unchanged);
     - `workflow.EnsureRunID(snapshotPath)`, ignoring errors;
     - `before := readStateSnapshot(snapshotPath)`.
  3. Only at verbose: the existing tee (`:126-129`) and setting `terminalOut`.
  4. `executedCmd, err := rootCmd.ExecuteC()`, then the existing failure write (`:134-140`).
  5. If the level is not off, `recordCommand(...)` inside `func(){ defer func(){ recover() }(); ... }()`:
     - `after := readStateSnapshot(snapshotPath)`;
     - `run, step, stepTo := attribute(before, after, start, cfg.Agent)`, where the run is after if `inProgress(after)`, else before if `inProgress(before)`, else standalone (`TraceID: tracelog.NewTraceID()`, `StartedAt: start`);
     - for a workflow run, `TraceID = snap.RunID`, falling back to a fresh trace id if it is somehow empty, and `StartedAt = snap.CreatedAt`;
     - `step = before.CurrentStep` when before belongs to the same run (same RunID); `stepTo = after.CurrentStep` when after belongs to the same run and differs from `step`;
     - build the `tracelog.Record`: `Name = commandName(executedCmd)` (`CommandPath()` without the leading `spektacular `, or `spektacular` for the root itself); `Status` error with `toErrorResponse(err).Message` when `err != nil`; `process.command_args = argv`; `process.exit_code`;
     - at verbose, add the response (`buf.String()`) and both snapshots;
     - `logger, lerr := tracelog.New(cfg.Log.Provider, tracelog.FileOptions{Directory: cfg.LogDir(root), Retention: cfg.Log.Retention})`, and if `lerr == nil`, `logger.Write(run, rec)`.
  6. Restore `rootCmd.SetOut(orig)` and `terminalOut = nil` only when they were teed.
- `cmd/root.go:178-205`: delete `isWorkflowStart`, since run attribution replaces it.
- `cmd/root.go:220-230`: extend `stateSnapshotFile` with `RunID string \`json:"run_id"\`` and `CreatedAt time.Time \`json:"created_at"\``.
- `cmd/root.go:348-387`: `readStateSnapshot` returns a cmd-local `stateSnapshot{Kind, Name, RunID, CurrentStep, CompletedSteps, CreatedAt}`, carrying the JSON tags `kind, name, run_id, current_step, completed_steps` for the verbose attributes (`CreatedAt` tagged `json:"-"`). Delete `stateAdvanced` (`:376-387`); the step and step-to attributes replace `advanced`.
- `cmd/root.go:389-398`: delete `sessionLogDir`, replaced by `cfg.LogDir(root)`; get the project root via `projectRoot()`.
- `cmd/root.go:97-108`: rewrite the `sessionLogEnv` doc comment for the new values. The constant name and value `SPEKTACULAR_SESSION_LOG` are kept.
- `cmd/root.go:18`: swap the import from `internal/sessionlog` to `internal/tracelog` and add `internal/workflow` (already imported).
- `cmd/status.go:209-211,267-279`: update the comments from "debug session log" to "verbose log". The behaviour is unchanged.
- `internal/sessionlog/record.go`, `internal/sessionlog/record_test.go`: delete.
- `cmd/root_test.go:617-1250`: rewrite the session-log tests against `.spektacular/logs`. Helpers `logFiles(t, dir)` and `readRecords(t, path) []tracelog.Record`. Tests to keep or replace one for one:
  - Disabled: `log: {level: off}` writes nothing.
  - EnvOff, EnvOn becomes EnvVerbose (including `on`), EnvOutsideProject, and the `logLevel` table, replacing `TestSessionLogOn` (`:723`).
  - EnabledRecordsSuccessAndRejection: default level, no response, error message present.
  - DoesNotChangeCallerVisibleOutput, extended to off/standard/verbose with a large output (`skill list` or `spec file read` of a large spec) and `status` in pretty and JSON form.
  - Advanced tests become step and step-to tests.
  - SameSessionID becomes the same trace across start, two gotos, the resume report (`spec new` with no data) and a goto.
  - DistinctSessionIDs, NoActiveWorkflow becomes standalone (two `knowledge search` give different trace ids in one `standalone_*.jsonl`).
  - Filename tests.
  - ForceRestart gives a new trace id and a new file.

  New tests:
  - agent `claude` and an unset agent give `unknown`;
  - a legacy state (write `state.json` by hand without `run_id`, as format 4 wrote it) resumes and every later record has the same trace id;
  - an unknown provider makes the command fail with `config_invalid` naming `file`;
  - a custom `log.config.directory` (relative, and absolute under `t.TempDir()`) is honoured and `.spektacular/logs` stays absent;
  - `git init` + `spektacular init` + commands give `git status --porcelain` with no `logs/` entries; same for a `migrate`d format-4 project (use `internal/testutil` git helpers);
  - an unwritable logs dir gives identical stdout and exit code;
  - a fresh `init` with no settings changed, then `version` or `status`, gives one record;
  - `.spektacular/debug/` seeded files are byte-identical after `migrate` and commands.
- `cmd/sessionlog_lane_test.go:119-160`: rename to `cmd/log_lane_test.go`. Two orchestrated `plan new` lanes for different specs, interleaved gotos: each lane's records share one trace id, the two differ, each sits in its own `<ts>_<agent>_plan_<name>.jsonl`. `TestSnapshotStatePath` (`:21`) is unchanged.
- `cmd/main_test.go`: no global override. Logging stays on in the suite (the success metric); the implementer also runs the suite once with `SPEKTACULAR_SESSION_LOG=off` exported. Any existing test that asserts an exact listing of `.spektacular/` contents, or a clean `git status`, after commands must be adjusted to ignore `logs/` (the self-ignoring `.gitignore` covers git).

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis (one agent surveys every `cmd` test that may break when logging is on by default: directory listings, git cleanliness, `debug:` config bodies), sequential integration of `runRoot` and the rewritten tests by one agent.

### Task: Keep only the most recent runs and days

**File changes**
- `internal/tracelog/file.go`: implement `sweep(created bool, current string)`. It runs only when `created`:
  - `os.ReadDir(dir)`; skip directories and the `.gitignore`;
  - split `*.jsonl` names into run files (not prefixed `standalone_`) and day files (`standalone_` prefix);
  - sort each group by name, descending; the fixed-width timestamp prefix sorts chronologically (`internal/sessionlog/record.go:57-63` rationale);
  - for the group `current` belongs to, delete entries beyond `retention`, never deleting `current`;
  - ignore `os.IsNotExist` and any other remove error.

  Retention below 1 never reaches here (config validation). Treat `<1` as 10 defensively.
- `internal/tracelog/file_test.go`:
  - 12 run writes with increasing `StartedAt` leave 10 files, the newest;
  - retention 3 leaves 3;
  - 15 standalone writes on 15 distinct days with retention 10 leave 10 day files and all run files;
  - a run older than the newest N (legacy start time) never deletes itself on creation;
  - an undeletable old file (directory with mode `0o500` holding the files, set after creation; skip as root) makes `Write` return normally.
- `cmd/root_test.go` (new tests):
  - default retention: 12 `spec new --force` runs (pin `spec.id_method: counter` and distinct names, or `--force` on one name) leave 10 run logs;
  - `log.retention: 3` leaves 3;
  - standalone commands interleaved remove no run log;
  - an undeletable old log gives the same stdout and exit code as with logging off.

  Use the existing `resetRootCmd` and `t.TempDir()`. Day files cannot be faked through `runRoot`'s clock, so day rotation is covered in the `tracelog` unit tests.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential.

### Task: Harbor reader understands span records

**File changes**
- `tests/harbor/shared/sessionlog.py:95-115` (`read_events`): if `"trace_id" in raw`:
  - `attrs = raw.get("attributes") or {}`;
  - `at = parse_time(raw["start_time"])`, `end = parse_time(raw["end_time"])`;
  - `duration_ms = round((end - at).total_seconds() * 1000)`;
  - `command = attrs.get("process.command_args") or []`;
  - `exit_code = attrs.get("process.exit_code", 0 if raw.get("status", {}).get("code") == "ok" else 1)`;
  - `state_before = attrs.get("spektacular.state.before")`, `state_after = attrs.get("spektacular.state.after")`;
  - `response = attrs.get("spektacular.response") or ""`.

  Otherwise keep the old mapping. Lines missing both `timestamp` and `start_time` are skipped rather than raising. Note that `test_metrics.py:949` expects a record with no timestamp to surface as a recorder error: keep the `KeyError` for old-format lines lacking `timestamp`, and only skip new-format lines lacking `start_time`. Check that test before changing.
- `tests/harbor/shared/sessionlog.py:1-11`: update the docstring to describe both formats, the `.spektacular/logs/` location and the `standalone_<day>.jsonl` shared file (any non-lane file is shared, unchanged at `:303-315`).
- `tests/harbor/shared/tests/test_sessionlog_windows.py` (extend): a `span_record(...)` helper that builds a new-format line. Tests:
  - a span record and its old twin give equal `Event`s (at, command, duration_ms, exit_code, state_before/after, response);
  - a mixed directory (one old lane file, one new lane file, one `standalone_20261024.jsonl`) gives the expected lanes and shared events;
  - a verbose new-format `workflow_in_progress` refusal counts in `Lane.resumes`.

  Existing fixtures under `tests/harbor/shared/tests/data/*/debug` and every existing test stay unchanged.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential. Run `uvx --with pytest pytest --import-mode=importlib tests/harbor/shared/tests`.

### Task: Harbor suites read the new logs at the verbose level

**File changes**
- `tests/harbor/{background-child/environment/Dockerfile:16-18, epic-workflow/environment/Dockerfile:27-30, goal-check-workflow/environment/Dockerfile:27-30, implement-epic-workflow/environment/Dockerfile:29-32, implement-workflow/environment/Dockerfile:15-17, plan-workflow/environment/Dockerfile:15-17, repo-workflow/environment/Dockerfile:15-17, spec-workflow/environment/Dockerfile:15-17, standing-rules/environment/Dockerfile:31-34}`: `ENV SPEKTACULAR_SESSION_LOG=verbose`, with the comment naming `.spektacular/logs/`.
- `tests/harbor/shared/timeline.py:30,43,93-94`: `DEFAULT_DIR = Path("/app/.spektacular/logs")`, plus the usage note and the empty-log message (`SPEKTACULAR_SESSION_LOG=verbose`).
- `tests/harbor/shared/metrics.py:11,24,53`: `DEBUG_DIR = Path("/app/.spektacular/logs")` and the docstring. Keep the `--debug-dir` flag name (README and tests use it) and the default-only invocation pinned by `tests/harbor/shared/tests/test_suite_wiring.py:33`.
- `tests/harbor/epic-workflow/tests/test_epic_workflow.py:27,114`, `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py:28,214`, `tests/harbor/implement-workflow/tests/test_implement_workflow.py:47`, `tests/harbor/plan-workflow/tests/test_plan_workflow.py:59`, `tests/harbor/goal-check-workflow/tests/test_goal_check.py:30`, `tests/harbor/standing-rules/tests/test_standing_rules.py:33`: `DEBUG_DIR` becomes `... / "logs"`, and the assertion messages name `SPEKTACULAR_SESSION_LOG=verbose`. The verifiers' own `cli()` calls that set `SPEKTACULAR_SESSION_LOG: "off"` (`test_epic_workflow.py:46`, `test_implement_epic_workflow.py:44`, `test_goal_check.py:266`, `test_standing_rules.py:174`) stay as they are; the name is kept.
- `tests/harbor/spec-workflow/tests/test_spec_workflow.py:583`: `SPEK_DIR / "logs"`.
- `tests/harbor/{epic-workflow/tests/test.sh:5, goal-check-workflow/tests/test.sh:5, implement-epic-workflow/tests/test.sh:6, standing-rules/tests/test.sh:5}`: `python3 /tests/timeline.py /app/.spektacular/logs ...`.
- `tests/harbor/shared/tests/test_sessionlog_rules.py:210,462`: these monkeypatch `DEBUG_DIR` and are unchanged in behaviour; confirm they still pass.
- `Makefile:232,268,343,372`: comments say "the session log in .spektacular/logs".
- `README.md:479,499-504,529,601`:
  - `spektacular/ # The project's .spektacular, with logs/ command logs`;
  - "Every suite sets `SPEKTACULAR_SESSION_LOG=verbose`, so each run's `artifacts/spektacular/logs/` holds ...";
  - the `timeline.py <job>/<trial>/artifacts/spektacular/logs` and `metrics.py --debug-dir <trial>/artifacts/spektacular/logs` commands.
- Leave unchanged: `tests/harbor/shared/tests/trim_reference.py:177-205` (reads old collected trials; add `logs` as a second accepted source dir in `layout()`, so new trials can be trimmed too) and `tests/harbor/shared/tests/data/*/debug` (old-format fixtures pinned by `test_reference_runs.py:185-186`).

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential. Finish with `grep -rn "spektacular/debug\|SESSION_LOG=on" tests/harbor README.md Makefile --exclude-dir=jobs --exclude-dir=.build` returning only fixtures and the trimming script, then the Harbor unit tests and suite self-checks.

### Task: Documentation site describes logging

**File changes**
- `docs:src/pages/debugging.mdx:1-103`: rewrite to the plan's content outline.
  - Frontmatter `title: "Logging and debugging - Spektacular"`, description "Spektacular logs every command it runs; see what each level records and where logs go."
  - Hero `heading="Logging and debugging"`.
  - One `<Prose>` holding the `##` sections in outline order, with fenced code blocks only (MDX Rule 1 and Rule 4).
  - CtaBanner body names the `log` key.
- `docs:src/pages/configuration.mdx:4`: the frontmatter description lists `log` in place of `debug`.
- `docs:src/pages/configuration.mdx:44-45`: the example `config.yaml` replaces `debug:` with the `log:` block, comments aligned at about column 34 like their neighbours.
- `docs:src/pages/configuration.mdx:105-109`: the key list swaps `debug` for `log`, so the count stays "Seventeen".
- `docs:src/pages/configuration.mdx:200-207`: replace the `debug` ConfigKey with the `log` ConfigKey from the plan's content example, in the same position (between `auto_commit` and `spec`), with blank lines inside the slot (MDX Rule 3).
- `docs:src/components/Nav.astro:22`: `{ label: "Logging and debugging", href: "/debugging/" }`.
- Do not edit `docs:CHANGELOG.md:52-61` (history).

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential. Verify with `npm run build`, `npx astro check`, `grep -nE "<div|<section|class=" src/pages/*.mdx` (expect none), and a grep for `—` in the changed files.

### Task: README and knowledge describe logging

**File changes**
- `README.md:290-291`: replace the `debug:` lines in the configuration example with:
  ```yaml
  log:
    provider: file                    # the logger; only file ships
    level: standard                   # off | standard | verbose (full responses and state); SPEKTACULAR_SESSION_LOG=off|standard|verbose overrides one command
    retention: 10                     # most recent workflow runs kept, and as many days of standalone commands
    config:
      directory: logs                 # relative to the folder holding config.yaml
  ```
  Bump the example's `schema:` comment to 5 if it shows a number (`README.md:284`).
- Knowledge `gotchas/harbor-agent-logs-and-artifacts.md` (repo store `spektacular`): read with `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"gotchas/harbor-agent-logs-and-artifacts.md"}'`. Rewrite the paragraph "Spektacular's own session log comes back for every agent" to say:
  - each suite sets `ENV SPEKTACULAR_SESSION_LOG=verbose`, recording every call to `.spektacular/logs/` at the verbose level;
  - each run, including each orchestrated lane, gets its own `<started>_<agent>_<kind>_<name>.jsonl`;
  - commands outside a workflow go to `standalone_<day>.jsonl`.

  Stage under `.spektacular/tmp/` and write with `spektacular knowledge write ... --from <path>`.
- Knowledge `architecture/testing-architecture.md`: update the sentences naming `SPEKTACULAR_SESSION_LOG=on` and the session-log file naming, so they name `verbose` and `.spektacular/logs/`, and add that the reader accepts both record formats until 000076. Write it the same way.
- Leave unchanged: `CHANGELOG.md` (history; the implement workflow's changelog step adds this feature's entry) and prior plans and test plans under `.spektacular/plans/`.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential.

## Testing Strategy

Per task:
- **Trace log package with the file logger**: unit tests in the new package for the record JSON shape, id formats, file naming, append-only writes, the self-ignoring `.gitignore`, the provider registry error, and failure containment (unwritable directory).
- **Run identifier in workflow state**: workflow unit tests for minting on fresh state, persistence across reloads, and `EnsureRunID` preserving every other field and leaving finished, missing and already-identified states alone.
- **Log settings replace debug, with migration**: config tests for defaults, the round trip, each validation error with its next action, directory resolution, and the provider pin. Migrate tests for debug on, off and absent, kept `log` keys, the idempotent `ignore` action, dry-run reporting, and untouched debug files. Init test for the template line. Updated golden fixtures.
- **Record every command through the configured logger**: `cmd` integration tests covering every acceptance criterion of Milestone 1. These include output parity across off, standard and verbose (with a large output and `status`), trace continuity across resume and legacy state, `--force` and parallel lanes, standalone day files, the environment variable values, a custom directory, `git status` cleanliness after init and migrate, and unwritable-directory parity. Also run the full suite once with `SPEKTACULAR_SESSION_LOG=off` exported.
- **Keep only the most recent runs and days**: tracelog unit tests with synthetic start times for run and day retention, self-preservation and undeletable files; `cmd` tests for 12 runs leaving 10, retention 3, and standalone commands not evicting runs.
- **Harbor reader understands span records**: Python unit tests for new-and-old equivalence, a mixed directory and resume detection; the existing suite must pass unchanged.
- **Harbor suites read the new logs at the verbose level**: Harbor shared unit tests, the suite self-checks, and a grep for leftover `.spektacular/debug` paths.
- **Documentation site describes logging**: `npm run build`, `npx astro check`, the MDX layout-HTML guard and an em-dash grep.
- **README and knowledge describe logging**: a read-back of the two knowledge entries through the CLI.

Success metrics and manual reviews (Manual — captured in the implementation test plan):
- step timings readable from a real run's log;
- under 10 ms added per `status` over 100 runs;
- an implement run of a 10-task plan logging under 1 MB;
- the Harbor half of output parity;
- a Harbor suite run reporting the same steps and parallel runs;
- a read-through of the docs pages.

## Project References

- Spec: `000073_log-mode-and-logger-providers` (epic `000002_workflow-tracing-and-run-stats`). Downstream specs that read this plan's contracts: `000074_trace-spans-for-agent-work`, `000075_run-stats-command`, `000076_harbor-on-built-in-logs`, `000077_html-log-viewer`.
- Design documents: none (the spec references none).
- Knowledge entries: conventions `error-messages-must-suggest-remediation`, `plans-never-change-the-active-install`, `store-files-must-be-written-through-the-cli`, `tests-must-not-depend-on-order`, `tests-must-pass-for-done` (repo `spektacular`); `plan-content-pages`, `mdx-authoring`, `no-em-dashes`, `site-layout` (repo `docs`); `gotchas/harbor-agent-logs-and-artifacts.md` and `architecture/testing-architecture.md` (both updated by this plan).
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.
- Prior plan: `000031_agent-session-debug-log` (the recorder being replaced).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Task tiers: trace log package (Medium), run identifier (Low), log settings and migration (Medium), recorder (High), retention (Low), Harbor reader (Low), Harbor wiring (Low), docs site (Low), README and knowledge (Low).

## Migration Notes

- Settings format 4 becomes 5. `project4to5` maps `debug.enabled: true` to `log.level: verbose`, and false or absent to `standard`. It writes `log.provider: file`, `log.retention: 10` and `log.config.directory: logs` where absent, deletes `debug`, and appends `logs/` to `.spektacular/.gitignore` (the new idempotent `ignore` action). The settings backup (`config.yaml.v4.old`) is taken by the engine as usual.
- Workflow state gains an optional `run_id`. Older in-progress states are backfilled on their first logged command after the upgrade. No state is rewritten by `migrate`.
- Old `.spektacular/debug/` files are never read, moved or deleted.
- This repository's own `.spektacular/` is NOT migrated by any task. Install the new binary with `make install-local` between workflows, and the next skill version check prompts `migrate`. The root `.gitignore` gains `.spektacular/logs/` so the checkout stays clean meanwhile.
- `SPEKTACULAR_SESSION_LOG=on` keeps working (it now means `verbose`). Existing `off` callers, such as the status mod, need no change.

## Performance Considerations

- The default level runs on every command in every project. It costs:
  - one extra YAML config parse (already done today by the root probe);
  - two small state-file reads;
  - one `os.Stat`;
  - one JSON marshal;
  - one `O_APPEND` write of a few hundred bytes.
- There is no output tee at the standard level and no line counting (the old `seq` read the whole file per write).
- A directory listing happens only when a record creates a new run or day file (the retention sweep).
- `EnsureRunID` writes at most once per legacy run.
- Budget: under 10 ms per `status` over 100 runs (manual check), and an implement run of a 10-task plan logging under 1 MB at the standard level (records are about 400-600 bytes; a run of a few hundred commands is well under 1 MB).
- Verbose records carry full responses and can be large. That is the documented trade-off of the verbose level, as with debug mode.
