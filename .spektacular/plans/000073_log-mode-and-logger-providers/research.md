---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Research: 000073_log-mode-and-logger-providers

## Alternatives considered and rejected

### Option A: Keep `debug.enabled` and add a separate always-on logger beside it

Keep `debug.enabled` and add a separate always-on logger beside it.

**Rejected**: The spec's Constraints require debug mode to be replaced, not kept beside it (user: "refactor this debug mode to be a log mode"). Two recorders would also double the per-command cost on the hot path in `spektacular:cmd/root.go:110-176`.

### Option B: Use the OpenTelemetry Go SDK (`go.opentelemetry.io/otel/sdk/trace`) with a custom exporter

Use the OpenTelemetry Go SDK (`go.opentelemetry.io/otel/sdk/trace`) with a custom exporter.

**Rejected**: The SDK keeps spans in memory and exports them in batches. Each command is a short-lived process, and 000074's spans open in one command and close in another, so an in-memory span could never carry them. The SDK also adds a large dependency tree to a CLI whose default-level budget is under 10 ms per command (spec Success Metrics). The spec asks for the OpenTelemetry *data model* written by Spektacular's own logger, so plain Go structs that carry the model's fields meet it.

### Option C: Keep the run identifier in a separate run registry or marker file

Keep the run identifier in a separate run registry or marker file.

**Rejected**: It contradicts the spec's Constraints ("The run identifier must come from the current workflow state, not be held anywhere else"). The workflow state also already decides which run a command belongs to: `spektacular:cmd/root.go:247-345` (`snapshotStatePath`) picks the lane or shared state file.

### Option D: Derive the trace identifier by hashing kind + name, as today's `session_id` does (`spektacular:internal/sessionlog/record.go:38-46`)

Derive the trace identifier by hashing kind + name, as today's `session_id` does (`spektacular:internal/sessionlog/record.go:38-46`).

**Rejected**: A hash cannot tell a `--force` restart from a resume (spec AC "A restarted run gets a new trace"), and the spec's Technical Approach says to mint the identifier when a workflow starts.

### Option E: Rely only on `init`/`migrate` ignore rules to keep logs out of git

Rely only on `init`/`migrate` ignore rules to keep logs out of git.

**Rejected**: Insufficient on its own, though both are still done because the spec requires them. Auto-commit stages with `git add -A` (`spektacular:internal/autocommit/git.go:92-93`) and the uncommitted-changes gate checks whether the tree is dirty (`git.go:72`). So an un-ignored log written by one command would dirty the repo for the next command. That would happen in any project whose `.spektacular/.gitignore` predates this change but which has not been migrated yet, under a custom log directory, and in the Go tests' temp projects, which are never `init`ed. The logger therefore also writes a `.gitignore` containing `*` into its directory when it creates a log file, the way Python virtualenvs ignore themselves.

### Option F: Rename the per-command variable to `SPEKTACULAR_LOG`

Rename the per-command variable to `SPEKTACULAR_LOG`.

**Rejected**: The maintainer's `spek-status` agent-host mod (`~/.claude/dev-mods/*/spek-status/hooks/register.tsx:37`) sets `SPEKTACULAR_SESSION_LOG: 'off'` on every poll. Renaming would silently flood every run's log with status polls. The spec allows keeping the variable and remapping it onto levels. **Superseded at review (2026-10-25):** the variable is now removed altogether, not renamed or remapped (Option J).

### Option G: Name each run's file by its trace id and find it with a directory scan

Name each run's file by its trace id and find it with a directory scan.

**Rejected**: The run's start time, kind and name are already in its state (`created_at`, `kind`, `data.name`; `spektacular:internal/workflow/state.go:14-21`), so the file name can be computed with no scan. This also keeps Harbor's file-name regex (`spektacular:tests/harbor/shared/sessionlog.py:25-29`, `<started>_<agent>_<kind>_<name>.jsonl`) matching unchanged.

### Option H: Keep counting lines for a `seq` field (`spektacular:internal/sessionlog/record.go:166-174`)

Keep counting lines for a `seq` field (`spektacular:internal/sessionlog/record.go:166-174`).

**Rejected**: It reads the whole file on every write, so its cost grows with the run, which goes against the under-10 ms budget. Start times and ids order the records, and readers pair records by id (spec Technical Approach).

### Option I: Rewrite Harbor's readers for the new format only

Rewrite Harbor's readers for the new format only.

**Rejected**: Harbor's checked-in reference runs (`spektacular:tests/harbor/shared/tests/data/*/debug`, produced by `trim_reference.py:181-205`) are in the old format and back offline acceptance tests. Spec 000076 replaces the reader anyway, so the smallest change is to normalise new-format records into the existing `Event` shape and keep reading old ones.

### Option J: Keep `SPEKTACULAR_SESSION_LOG`, remapped onto levels (`off`, `standard`, `verbose`, with `on` meaning `verbose`)

Keep `SPEKTACULAR_SESSION_LOG`, remapped onto levels (`off`, `standard`, `verbose`, with `on` meaning `verbose`).

**Dropped at review (2026-10-25)**: This was the plan's original choice (see the superseded drafting assumption below). At the epic's end-of-planning review the user removed the variable entirely ("just make it not log. I don't understand why we would want it to?"). It had two users:
- the `spek-status` mod, which set `off` because it polls `status` every 3 s. `status` is now never logged, so the mod needs nothing;
- Harbor, which set `on` in its images. Its suites now set `log.level: verbose` in the configuration of the projects they seed.

The only use of logged `status` calls was an epic orchestrator's `status <epic>` polls, marking when it noticed a child finish. 000074's sub-agent spans replace that. A per-command switch would be a second, undocumented way to set the level with no remaining caller.

### Option K: Keep logging `status` like any other command

Keep logging `status` like any other command.

**Rejected at review (2026-10-25)**: A tool polling `status` every few seconds would fill the day's standalone log, or the run's log while a workflow is in progress, and trigger retention sweeps that can push out real runs. Its only analytical use is covered by 000074's spans. The spec now requires that `status` and run statistics commands are never recorded.

### Option L: Exempt commands by a list of names inside `runRoot`

Exempt commands by a list of names inside `runRoot`.

**Rejected**: A cobra annotation read through a parent walk is how the upgrade gate already exempts commands (`spektacular:cmd/gate.go:122-136`, `cmd/migrate.go:18-24`). With it, a command group such as 000075's `stats` marks itself once and every subcommand inherits it. A name list in `runRoot` would couple the recorder to command names and need editing for every new subcommand. The exemption is checked on the command `rootCmd.Find` resolves before the command runs, so an exempt command has no side effects, not even a run-id backfill. It is checked again on the executed command, which is authoritative and also holds in tests.

### Option M: Cover "logging off" in the Go suite with a config-driven run, or by switching logging off in TestMain

Cover "logging off" in the Go suite with a config-driven run, or by switching logging off in TestMain.

**Rejected**: With the variable gone, the old "export `SPEKTACULAR_SESSION_LOG=off` and rerun" pass needs a replacement.
- A config-driven run would mean every `cmd` fixture that writes a `config.yaml` (`writeSpecCommandConfig` and many others) writing `log.level: off` under some switch, which edits dozens of fixtures.
- Switching logging off in TestMain hides any breakage logging causes in existing tests (see the drafting assumption).

The chosen test-only `-log-level` flag on the `cmd` test binary sets an unexported `forcedLogLevel`. Nothing in the shipped binary sets it. The logging tests opt out with `useConfiguredLogLevel(t)`, so the same suite runs at the default and fully off.

### Option N: Give Harbor's suites the verbose level through anything but their seeded configuration

Give Harbor's suites the verbose level through anything but their seeded configuration.

**Rejected**: With no environment override, configuration is the only lever, and the user named it: each suite sets `log.level: verbose` in the throwaway project it seeds. The suites whose agent runs `init` get a minimal pre-seeded `config.yaml`, because `init` keeps an existing config and backfills name, repos and agent (`spektacular:internal/project/init.go:35-60,130-136`, `cmd/init.go:51-75`), which the implement suite's pre-seeded config already relies on. The pre-built projects switch logging off while seeding and to `verbose` last, which keeps the old "set after seeding" guarantee. The verifiers that call `spektacular` switch logging off before their first call, replacing their old per-call `off`.

## Chosen approach — evidence

- **One choke point for every command.** `spektacular:cmd/root.go:110-176` (`runRoot`) already wraps every command. It reads state before and after (`readStateSnapshot`, `root.go:348-369`), contains every failure (`sessionlog.Record` never returns an error, `internal/sessionlog/record.go:136-164`), and picks the lane by argument (`snapshotStatePath`, `root.go:247-345`). The spec says to evolve this recorder rather than start afresh.
- **Fresh state is created in one place.** `spektacular:internal/workflow/workflow.go:103-117`: `workflow.New` builds a new `State` when no state file loads. `new` removes the old state file first (`clearState`, `spektacular:cmd/resume.go:216-218`), including under `--force`, and the resume report returns before any write (`probeResume`, `resume.go:193-210`). So a run id minted in `New`'s fresh branch changes on `--force`, stays the same on resume, and is distinct per lane.
- **Concurrent runs are separate lanes.** `spektacular:internal/workflow/lane.go:20-22`: each orchestrated plan or implement run has its own state file, `workflows/<kind>-<name>.json`, so each one mints and keeps its own id.
- **The provider pattern.** `spektacular:internal/config/config.go:840-873`: every section names `provider` and validates it against the shipped set with a next action. The next-action shape follows `EpicConfig.Validate` (`config.go:862-873`), and the registry-of-names shape follows `identifier.TaskIDProviderFor` (`spektacular:internal/identifier/taskid.go:36-52`). A config-package name list is pinned to a registry by test, following `SupportedAgentNames` (`config.go:197`).
- **Migration mechanics.** `spektacular:internal/migrate/steps_project.go:139-160` (`project3to4`) is the model for a `From: 4` step that edits the YAML tree with `getNode`/`setNode`/`deleteKey` (`internal/migrate/node.go:52-121`). `registry.go:61-68` registers it and `config/schema.go:17-20` bumps `CurrentProjectSchema`. The engine performs `create`/`remove` actions only (`engine.go:320-340`), so appending an ignore line needs a new action op.
- **Ignore rules a project gets.** `spektacular:templates/.spektacular/.gitignore` is written by `init` (`internal/project/init.go:148-155`). `migrate` never rewrites it, which `excludeFromProject` (`internal/worktree/worktree.go:503-540`) already works around for a line added later. The template's `*.log` line does not match `.jsonl`.
- **Status colour detection.** `spektacular:cmd/status.go:267-279`: `terminalOut` exists only because the debug tee hides the terminal. Only the verbose level needs to tee output, so at the default level it is never set. `status` is now logging-exempt, so in a real process it is never teed at all.
- **Polling caller.** `~/.claude/dev-mods/*/spek-status/hooks/register.tsx:37` sets `SPEKTACULAR_SESSION_LOG=off`. Since the review decision, `status` is never logged, so the mod needs no variable.
- **Exemption by annotation.** `spektacular:cmd/migrate.go:18-24` (`gateAnnotation = "gate"`, `gateExempt = "exempt"`) and `cmd/gate.go:122-136` (`gateExempted` walks the command and its parents) are the pattern `logAnnotation = "log"`, `logExempt = "exempt"` and `logExempted` follow. `rootCmd` has no `TraverseChildren`, so `rootCmd.Find(os.Args[1:])` resolves the command `ExecuteC` will run. The lane test sets `os.Args` to match its args (`cmd/sessionlog_lane_test.go:82-89`), which is how a pre-command decision is tested.
- **Harbor sets the variable to `on`.** Every suite's `environment/Dockerfile:17` (spec, plan, implement, repo, epic, implement-epic) sets `ENV SPEKTACULAR_SESSION_LOG=on`. The verifiers read `response` and `state_after.current_step` (`tests/harbor/shared/sessionlog.py:95-113`, `step_after` at `:86-91`), which only the verbose level keeps.
- **How Harbor projects are seeded.** The spec, plan and repo agents run `init` themselves (`instruction.md`). implement `COPY`s a full `config.yaml`. epic, implement-epic, goal-check and standing-rules build the project in `environment/seed/seed.sh` at image build time, running Spektacular commands (standing-rules runs `spec file read`). Those commands are unlogged today only because the variable is set after seeding. Four verifiers set `off` on their own calls (`test_epic_workflow.py:46`, `test_implement_epic_workflow.py:44`, `test_goal_check.py:266`, `test_standing_rules.py:174`). Standing-rules' own `spec file read` (`:223`) would otherwise satisfy `rule_spec_read_through_cli`.

## Files examined

- `spektacular:cmd/root.go:91-176` — runRoot, the env switch, the snapshot reads and the record write
- `spektacular:cmd/root.go:178-218` — `isWorkflowStart`, `sessionLogOn`
- `spektacular:cmd/root.go:220-398` — the snapshot file shape, lane selection by argument, `readStateSnapshot`, `stateAdvanced`, `sessionLogDir` (`.spektacular/debug`)
- `spektacular:cmd/root.go:466-480` — `loadConfigLenient`, used by the root probe outside a project
- `spektacular:internal/sessionlog/record.go:1-174` — Event, SessionID, file naming, Record and `countLines`
- `spektacular:internal/sessionlog/record_test.go` — unit tests to be rewritten for the new package
- `spektacular:cmd/root_test.go:617-1250` — session-log tests (helpers `sessionLogFiles` and similar; disabled, env, advanced, session id, filename and force-restart tests)
- `spektacular:cmd/sessionlog_lane_test.go:21-160` — lane filing tests (`TestSnapshotStatePath`, `TestSessionLog_LaneCommandsAreFiledUnderTheLane`)
- `spektacular:cmd/main_test.go:11-14` — TestMain pins the version only, with no logging override
- `spektacular:cmd/spec_test.go:26-47` — `writeSpecCommandConfig`, the temp project fixture (no `.gitignore`)
- `spektacular:cmd/status.go:207-279` — `terminalOut` and the watch frames
- `spektacular:cmd/resume.go:193-218` — `probeResume`, `clearState`
- `spektacular:cmd/spec.go:143-300` — spec new: resume probe, then clearState, then `workflow.New`
- `spektacular:cmd/migrate.go:38-78` — migrate command and installer
- `spektacular:internal/workflow/state.go:14-61` — the State struct, load and save
- `spektacular:internal/workflow/workflow.go:94-200` — New (fresh state), enter_state persistence
- `spektacular:internal/workflow/lane.go:13-80` — lane paths
- `spektacular:internal/config/config.go:84-87` — DebugConfig (to remove)
- `spektacular:internal/config/config.go:384-470` — the Config struct and NewDefault
- `spektacular:internal/config/config.go:470-560` — ParseYAMLFile and `storeDirs` (logs must NOT join `storeDirs`, which forbids paths outside the root)
- `spektacular:internal/config/config.go:680-750` — Validate and the `config_invalid` error shape
- `spektacular:internal/config/schema.go:17-20` — `CurrentProjectSchema = 4`
- `spektacular:internal/migrate/registry.go:28-95` — Step, Action, the registry
- `spektacular:internal/migrate/steps_project.go:139-160` — `project3to4` as the model
- `spektacular:internal/migrate/engine.go:242-340` — upgradeFile, perform
- `spektacular:internal/project/init.go:148-155` — writes the embedded `.spektacular/.gitignore`
- `spektacular:templates/.spektacular/.gitignore` — the ignore rules init installs
- `spektacular:internal/autocommit/git.go:72-150` — Dirty and `add -A`
- `spektacular:internal/identifier/taskid.go:36-64` — provider registry and the crypto/rand id pattern
- `spektacular:.gitignore:43-44` — the repo root ignores `.spektacular/debug/`
- `spektacular:README.md:290-291,479,499-504` — debug config line, Harbor artifacts, the `SPEKTACULAR_SESSION_LOG=on` paragraph
- `spektacular:tests/harbor/shared/sessionlog.py:1-380` — the reader: `LANE_FILE` regex, `read_events`, the `SessionLog(debug_dir)` lanes/shared split
- `spektacular:tests/harbor/shared/{timeline.py:43,171, metrics.py:11-24,53,349-352,590-617,709, delegation.py:93-98}` — default `.spektacular/debug` paths and the `--debug-dir` flag
- `spektacular:tests/harbor/{plan-workflow/tests/test_plan_workflow.py:59, implement-workflow/tests/test_implement_workflow.py:47, spec-workflow/tests/test_spec_workflow.py:583, epic-workflow/tests/test_epic_workflow.py:27, standing-rules/tests/test_standing_rules.py:33}` — verifiers' `DEBUG_DIR`
- `spektacular:tests/harbor/*/environment/Dockerfile:16-17` — `ENV SPEKTACULAR_SESSION_LOG=on`
- `spektacular:tests/harbor/shared/tests/*` — reference data in the old format under `data/*/debug`, tests building `tmp_path/"debug"`
- `spektacular:tests/harbor/shared/sessionlog.py:95-115` — `read_events` reads `timestamp`, `command`, `duration_ms`, `exit_code`, `state_before`, `state_after`, `response` and requires `timestamp`. It never reads `seq`, `session_id` or `advanced`
- `spektacular:tests/harbor/shared/sessionlog.py:118,157` — resume detection is `"workflow_in_progress" in e.response`, which needs the response (verbose level)
- `spektacular:tests/harbor/shared/delegation.py:106-120` — `worktree_paths` does `json.loads(e.response)` for `epic worktree` (implement-epic verifier, line 317)
- `spektacular:tests/harbor/shared/sessionlog.py:291-315` — lanes and segments come from file names only (one file is one segment), and every non-matching `*.jsonl` is shared
- `spektacular:tests/harbor/*/environment/Dockerfile` — `ENV SPEKTACULAR_SESSION_LOG=on` in nine images: background-child:18, epic-workflow:30, goal-check-workflow:30, implement-epic-workflow:32, implement-workflow:17, plan-workflow:17, repo-workflow:17, spec-workflow:17, standing-rules:34
- `spektacular:tests/harbor/*/task.toml` — the whole `/app/.spektacular` is collected as the `spektacular` artifact, so `logs/` comes back with no task change
- `spektacular:tests/harbor/{epic-workflow/tests/test.sh:5, goal-check-workflow/tests/test.sh:5, implement-epic-workflow/tests/test.sh:6, standing-rules/tests/test.sh:5}` — call `timeline.py /app/.spektacular/debug`
- `spektacular:tests/harbor/shared/tests/test_suite_wiring.py:33,241-246` — pins `python3 /tests/metrics.py || true` exactly, so the metrics default directory changes rather than gaining a flag
- `spektacular:tests/harbor/{epic-workflow/tests/test_epic_workflow.py:46, implement-epic-workflow/tests/test_implement_epic_workflow.py:44, goal-check-workflow/tests/test_goal_check.py:266, standing-rules/tests/test_standing_rules.py:174}` — the verifiers' own `cli()` calls set `SPEKTACULAR_SESSION_LOG=off`, which still works with the name kept
- `spektacular:tests/harbor/{implement-epic-workflow/tests/test_implement_epic_workflow.py:28, goal-check-workflow/tests/test_goal_check.py:30}` — further `DEBUG_DIR` constants
- `spektacular:tests/harbor/shared/tests/test_reference_runs.py:185-186,253-262,359-360` — pins each reference run's directory listing (`debug`) and old key set; exact metric values come from the old-format fixtures under `data/{m70-epic,s55-epic,s55-plan,s55-implement}/debug`
- `spektacular:tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py:194-235` — writes old-format lane records under `debug/`
- `spektacular:README.md:601,624` — `metrics.py --debug-dir` usage and the Harbor unit-test command `uvx --with pytest pytest --import-mode=importlib tests/harbor/shared/tests`
- `spektacular:cmd/gate.go:122-136`, `cmd/migrate.go:18-24`, `cmd/init.go:21` — the gate's exemption annotation and parent walk, the model for the logging-exempt annotation
- `spektacular:cmd/status.go:29-40` — `statusCmd`, which gains the logging-exempt annotation
- `spektacular:cmd/init.go:34-75`, `internal/project/init.go:24-60,130-136` — `init` migrates and keeps an existing `config.yaml`, backfilling name, repos and agent
- `spektacular:tests/harbor/*/environment/seed/seed.sh` — the four pre-built projects' seeding commands, run at image build time
- `spektacular:tests/harbor/implement-workflow/environment/config.yaml` — the pre-seeded full config (no `schema` line)
- `spektacular:tests/harbor/{spec,plan,repo}-workflow/instruction.md` — the agent runs `spektacular init {{agent}}` itself
- `docs:src/pages/debugging.mdx:1-103` — the debugging page: Hero, one Prose block (Enabling debug logging, What gets logged, Before you enable it), CtaBanner
- `docs:src/pages/configuration.mdx:4,44-45,105-109,200-207` — frontmatter list, example yaml, the "Seventeen top-level keys" sub, the `debug` ConfigKey
- `docs:src/components/Nav.astro:22` — the "Debugging" nav entry under Resources
- `docs:Makefile` — `make build` (npm run build), `make check` (npx astro check)

## External references

- OpenTelemetry Trace data model (opentelemetry.io/docs/specs/otel/trace/api and the OTLP span message): fields trace_id (16 bytes), span_id (8 bytes), parent_span_id, name, kind, start/end time, attributes, status {code, message}, resource. It is the model the spec's Constraints name.
- OpenTelemetry semantic conventions, `process.command_args`, `process.exit_code`, `service.name`, `service.version`: attribute names reused so the records read like OTel data.
- JSON Lines (jsonlines.org): one object per line. The spec's Constraints require it.
- POSIX `O_APPEND`: a single `write` of one line appends atomically on local filesystems, which concurrent lanes and standalone commands rely on.

## Prior plans / specs consulted

- `000031_agent-session-debug-log` (plan): why the recorder sits in `runRoot`, why it records argv verbatim, why `advanced` diffs state rather than reading the error, and that it left retention out of scope. That last point is where the line-count `seq` cost came from.
- `000068_task-goal-validation`, `000069_compact-agents-md`, `000071_harbor-child-agents` (plans and test plans): Harbor and manual checks read `spektacular/debug/*.jsonl`, and `000071` relies on `state_after.current_step` and argv in session-log events.
- Specs `000074`–`000077` (same epic, context only): 000074 needs `parent_span_id`, a span type, separate start and end records, and its step-span id kept beside the run id in workflow state. 000075 reads per-command records, run ids and the logs directory, and needs stable fields. 000076 replaces `tests/harbor/shared/sessionlog.py`. 000077 draws a waterfall from the same records.

## Open assumptions

- Writing one JSON line with a single `write` on an `O_APPEND` file does not interleave between concurrent processes on the filesystems Spektacular runs on (local ext4/APFS). If a test shows interleaving, the implement workflow must STOP and ask.
- Retention running in one process may delete the file of a run that is still active in another, once that run is older than the N most recent. The next command recreates the file, so that run's earlier records are lost. This is accepted for a count-based retention (spec Non-Goals rule out other policies).
- `state.created_at` is written with nanosecond precision (Go `time.Time` RFC3339Nano JSON), so two runs of the same kind and name never share a file name.
- Harbor's `SessionLog` lanes and shared events stay equivalent when commands run after a workflow has finished go to the standalone day log rather than to the finished run's file. If a Harbor suite's reported steps or parallel runs change because of this, STOP and ask.
- Any caller that polls `status` is covered, because `status` is never logged. Nothing else outside this repo sets `SPEKTACULAR_SESSION_LOG`, and leaving it set is harmless because nothing reads it.
- A minimal pre-seeded `config.yaml` (`schema: 5` and `log.level: verbose`) is kept by the agent's `init` in the spec, plan and repo suites, with the name, colocated repo and agent backfilled as for a fresh project. If a suite's verifier or reported steps change because the config exists before `init`, STOP and ask.
- `rootCmd.Find(os.Args[1:])` resolves the same command `ExecuteC` runs in the shipped binary. If an exempt command is ever seen backfilling a run id or teeing in a real process, STOP and ask.

## Drafting assumptions

### Chosen direction: evolve runRoot's recorder into a provider-selected file logger, with the run id in workflow state (architecture)
- **Decision**: Replace `internal/sessionlog` with `internal/tracelog`, which holds the OTel-model span record, id minting, a `Logger` interface and the `file` provider. `runRoot` stays the one choke point and writes one complete `span` record per command. `workflow.State.run_id` is minted by `workflow.New` on fresh state and backfilled for older runs by `workflow.EnsureRunID`. Config `log: {provider, level, retention, config.directory}` replaces `debug`. The settings format goes 4 to 5 with a `project4to5` step that also appends `logs/` to `.spektacular/.gitignore`.
- **Rationale**: This follows the spec's Technical Approach (evolve the recorder; mint the id at workflow start; one file per run), honours the Constraints (OTel model, own logger, provider pattern, run id from state, JSONL in a logs dir), and keeps the default level cheap.
- **Rejected**: The OTel SDK with an exporter (in-memory spans, heavy dependencies). Emitting spans from inside the workflow engine (misses every non-workflow command). A run registry file (contradicts the constraint). See research.md alternatives.

### Level names `off | standard | verbose` (architecture)
- **Decision**: `log.level` takes `off`, `standard` (the default) or `verbose`.
- **Rationale**: The spec speaks of "the default level" and "a verbose level" and says logging can be switched off. One key with three values covers all three without a separate `enabled` flag. `standard` reads better in config than a level literally named `default`.
- **Rejected**: `enabled: bool` plus `verbose: bool`, which allows the meaningless pair `enabled: false, verbose: true`. A level named `default`, which is ambiguous in docs. `info`/`debug`, which suggests severity filtering the logger does not do.

### Record format: one complete `span` record per command, OTel field names, Spektacular attributes namespaced (architecture)
- **Decision**: Each line has `record: "span"`, `trace_id` (32 hex), `span_id` (16 hex), optional `parent_span_id`, `name` (the command path without the binary, e.g. `spec goto`), `kind: "internal"`, `start_time`/`end_time` (RFC 3339 UTC with nanoseconds), `status: {code: "ok"|"error", message}`, `attributes` and `resource: {service.name, service.version}`. Attributes: `spektacular.span.type: "command"`, `spektacular.agent`, `spektacular.workflow.kind`/`.name`, `spektacular.step`, `spektacular.step.to`, `process.command_args`, `process.exit_code`, and at verbose `spektacular.response`, `spektacular.state.before`, `spektacular.state.after`.
- **Rationale**: These are the OTel data model's own field names, with semantic-convention attribute names where they exist. The `record` discriminator and `spektacular.span.type` leave room for 000074's `span_start`/`span_end` records and its step, task, sub-agent, user-wait and work types without changing any field defined here.
- **Rejected**: Today's flat `Event` shape, which is not the OTel model. OTLP's nested `resourceSpans/scopeSpans` JSON, which is too heavy per line and hard to read with `jq`. A top-level `duration_ms`, which is derivable and not in the model.

### Which run a command belongs to (architecture)
- **Decision**: The command belongs to the after-state's run if that is in progress, else to the before-state's run if that was in progress, else it is standalone with a fresh trace id. A finished workflow's leftover state counts as no workflow.
- **Rationale**: This puts `new` and `--force` in the new run, puts the command that finishes a run (or removes its lane) in that run, and meets "a command run outside any workflow gets an identifier of its own" even when a finished state file remains.
- **Rejected**: Always using the after-state (today's rule), which files every command after a finished run under that dead run.

### Legacy runs get their id from runRoot before the command, only when logging is on (architecture)
- **Decision**: Before the command runs, `runRoot` calls `workflow.EnsureRunID(statePath)` when the command will be logged and the state is in progress with no `run_id`. The call adds the field atomically and changes nothing else. With logging off, the state file is left untouched.
- **Rationale**: This meets "given a run identifier on its first command after the upgrade, and keeps it". Writing before the command means a command that saves state carries the id forward. Skipping it when logging is off keeps the logging-off path free of side effects.
- **Rejected**: Backfilling inside `workflow.New`, which misses commands that read state without building a workflow, such as `status` and `knowledge`. Writing after the command, which races with the command's own save.

### Log file naming and retention ordering (architecture)
- **Decision**: A run's file is `<run start: state.created_at, UTC, 20060102T150405.000000000Z>_<agent>_<kind>_<name>.jsonl`. The standalone file is `standalone_<YYYYMMDD UTC>.jsonl`. Retention keeps the `log.retention` newest run files (sorted by the start-time prefix) and, separately, the same number of newest standalone day files. Sweeps run only when a record creates a new file, and they never delete the file just written.
- **Rationale**: The name is computed from state, so no directory scan is needed per command. It matches Harbor's existing lane regex unchanged, sorts by start, and keeps standalone days from pushing out runs (spec requirement).
- **Rejected**: Naming files by trace id, which is unreadable in a listing and does not match Harbor's regex. Sweeping on every command, which costs a directory listing per command for nothing. UTC is chosen over local days for deterministic tests and file names that do not depend on the machine.

### Day-file standalone commands are not linked to each other (architecture)
- **Decision**: Every standalone command gets its own fresh trace id, so two standalone commands never share a trace, even in one day file.
- **Rationale**: This is spec AC "two consecutive `knowledge search` commands produce two records with different trace identifiers, both in the same log for that day".
- **Rejected**: One trace per day, which would make unrelated commands look like one run.

### Keep `SPEKTACULAR_SESSION_LOG` as the per-command variable, remapped onto levels (discovery), superseded at review 2026-10-25
- **Superseded**: The user removed the variable entirely at the epic's end-of-planning review (Option J). The text below records the original choice.
- **Decision**: Keep the variable's name. `off` leaves the command out, `standard` records it at the default level, and `verbose` records it verbosely. `on` is still accepted as the older spelling of `verbose`, so Harbor's existing images and a user's habit keep working. Any other value leaves the level to the configuration.
- **Rationale**: The maintainer's `spek-status` mod sets `SPEKTACULAR_SESSION_LOG=off` on every poll. A rename would silently flood every run's log with polls. The spec allows keeping the variable and remapping it.
- **Rejected**: Renaming to `SPEKTACULAR_LOG`, which breaks the mod silently. Keeping both names, which leaves two variables to document.

### `status` and `stats` are never logged, by a cobra annotation this plan owns (review, 2026-10-25)
- **Decision**: `cmd/root.go` defines `logAnnotation = "log"` and `logExempt = "exempt"`, plus `logExempted`, which walks a command and its parents. `runRoot` treats an exempt command as logging off, checked on the target `rootCmd.Find` resolves before the command runs and again on the executed command. `statusCmd` carries the annotation. 000075 sets the same map on its `stats` group.
- **Rationale**: This is the user's review decision. Polling `status` must not flood or trim the logs, and its one analytical use moves to 000074's spans. It follows the gate's annotation pattern. Placing the mechanism here means 000075 adds no recorder change.
- **Rejected**: Logging `status` (Option K). A name list in `runRoot` (Option L). Checking only after the command, which would let an exempt `status` backfill a legacy run id.

### Harbor's verbose level comes from its seeded configuration (review, 2026-10-25)
- **Decision**: The images stop setting the variable. The spec, plan and repo suites pre-seed a minimal `config.yaml` with `log.level: verbose`, implement adds a `log` block to its pre-seeded config, and the pre-built projects' seed scripts switch logging off while seeding and to `verbose` last. The verifiers that call `spektacular` switch logging off first, through a shared `loglevel.py`.
- **Rationale**: This is the user's review decision. Configuration is the only lever left, and it keeps the log holding only the agent's run.
- **Rejected**: Leaving the seed commands logged, which would put standing-rules' own `spec file read` in the log it checks. Deleting the logs at the end of seeding, which is cruder and would leave the seeds logged at standard in between.

### Logs directory ignores itself, in addition to the init/migrate ignore rule (discovery)
- **Decision**: When the file logger creates a log file, it also makes sure a `.gitignore` containing `*` exists in the log directory.
- **Rationale**: Auto-commit stages with `git add -A` and the start gate checks for a dirty tree. Without this, a log written by one command would dirty the repo for the next in any project not yet migrated, under a custom directory, and in the Go tests' temp projects. The spec still gets its `init`/`migrate` rule.
- **Rejected**: Relying on `init`/`migrate` alone, which breaks the auto-commit and gate behaviour in the cases above.

### Harbor reads both record formats (discovery)
- **Decision**: Harbor's `read_events` turns a new-format record into the existing `Event` shape and still reads old-format lines.
- **Rationale**: Harbor's checked-in reference runs are old-format and back offline acceptance tests, and spec 000076 replaces the reader. This is the smallest change.
- **Rejected**: Regenerating the reference data, which needs live paid runs. A new-format-only reader, which breaks the offline tests.

### Update the Harbor knowledge entries that describe the debug log (components)
- **Decision**: A task rewrites `gotchas/harbor-agent-logs-and-artifacts.md` and the session-log sentences of `architecture/testing-architecture.md` through `spektacular knowledge write`, so they describe `.spektacular/logs/` and `SPEKTACULAR_SESSION_LOG=verbose`.
- **Rationale**: Both entries describe the current behaviour this spec deliberately replaces. The spec is the user's recorded decision, so this is not a contradiction of a knowledge entry. Leaving the entries stale would mislead every later plan, and knowledge outranks code.
- **Rejected**: Leaving them for spec 000076, which would keep them wrong for three specs.

### Keep the docs page slug `/debugging/`, retitle it "Logging and debugging" (components)
- **Decision**: The page stays at `src/pages/debugging.mdx` (the spec constraint names the debugging page). Its title, hero heading and nav label become "Logging and debugging".
- **Rationale**: This honours the constraint, keeps existing links working (the site has no redirects), and tells readers the page now covers logging.
- **Rejected**: A new `/logging/` page, which conflicts with the constraint and breaks links. An unchanged title, which hides the new content.

### No global logging override in the Go test suite; a test-only flag forces logging off (tasks; revised at review, 2026-10-25)
- **Decision**: `cmd`'s TestMain does not switch logging off by default. The suite runs with logging on, and the implementer also runs `go test -shuffle=on -count=1 ./cmd/ -args -log-level=off`. That test-only flag sets an unexported `forcedLogLevel` that `runRoot` prefers to the configured level inside a project. The logging tests call `useConfiguredLogLevel(t)` to clear it for themselves. Before the review, the second run exported `SPEKTACULAR_SESSION_LOG=off`, which no longer exists.
- **Rationale**: The spec's success metric asks that no command's output or exit code differ between logging on and off across the full Go suite. Running it both ways is that check. The self-ignoring logs directory keeps temp git projects clean.
- **Rejected**: Switching logging off in TestMain, which hides any breakage logging causes in existing tests. A config-driven off run, which edits dozens of fixtures (Option M).

### Day rotation is covered in tracelog unit tests, not through runRoot (tasks)
- **Decision**: The file logger takes the run's start time from its `Run` argument, so day-file rotation and day retention are tested with synthetic times in the `tracelog` package. The `cmd` tests cover run retention end to end.
- **Rationale**: Tests must not depend on the wall clock (convention), and `runRoot` uses the real clock.
- **Rejected**: Injecting a clock into `runRoot` just for tests, which adds surface for little gain.

### Docs and README tasks wait for retention (tasks)
- **Decision**: The documentation tasks depend on the retention task as well as the recorder, so they describe shipped behaviour.
- **Rationale**: Retention is part of what the docs must describe.
- **Rejected**: Documenting before retention lands, which risks describing behaviour that changes during implementation.

## Rehydration cues


- `spektacular spec file read 000073_log-mode-and-logger-providers`, plus 000074 and 000075 for the record-format consumers.
- Re-read `cmd/root.go:91-400`, `internal/sessionlog/record.go`, `internal/workflow/{state,workflow,lane}.go:1-200`, `internal/config/config.go:84-900`, `internal/migrate/{registry,steps_project,engine}.go`.
- Re-read `tests/harbor/shared/sessionlog.py` and grep `tests/harbor` for `"debug"` and `SPEKTACULAR_SESSION_LOG`. Re-read the seed scripts under `tests/harbor/*/environment/seed/` and the verifiers' `cli()` helpers.
- Re-read `cmd/gate.go:122-136` and `cmd/migrate.go:18-24` for the annotation pattern, and `cmd/status.go:29` for `statusCmd`.
- `spektacular knowledge read` for `gotchas/harbor-agent-logs-and-artifacts.md` and `architecture/testing-architecture.md` (both describe the old debug log and are updated by this plan).
- Docs repo: `src/pages/debugging.mdx`, `src/pages/configuration.mdx`, `src/components/Nav.astro`.
