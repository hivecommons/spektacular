---
created_date: "2026-10-25"
---

# Planning summary: 000002_workflow-tracing-and-run-stats

## Decisions
- **Read-only log commands are not logged.** 000073_log-mode-and-logger-providers logged every command in a project, while 000075_run-stats-command exempts commands that only read the logs (`spektacular stats` and its forms) and 000077_html-log-viewer relies on that exemption. Outcome (user, 2026-10-25: "A stats call should not add to the log this is for me to use"): the exemption stands; 000075 adds it to 000073's command recorder, and 000073's plan records it under Out of Scope.
- **`status` is never logged.** 000073_log-mode-and-logger-providers logged `status` and kept a per-command override for the maintainer's polling mod; 000076_harbor-on-built-in-logs measured epic hand-off gaps from logged `status <epic>` polls. Outcome (user, 2026-10-25: "just make it not log. I don't understand why we would want it to?"): 000073 adds the logging-exempt annotation and marks `status`; 000075 marks `stats` with it; 000076 measures hand-off gaps from the orchestrator's sub-agent spans (000074) and retires the epic suite's status-poll check. 000073's spec is amended.
- **No per-command logging override.** 000073 kept `SPEKTACULAR_SESSION_LOG` for the polling mod and Harbor; 000076 removed Harbor's use. Outcome (user, 2026-10-25, after confirming Harbor's projects are throwaway test projects): the variable is removed; Harbor sets its level in its seeded test projects' config (000073 verbose, 000076 back to default). 000073's spec is amended.

## Order added for shared files
- Added: `000073_log-mode-and-logger-providers` now depends on `000072_knowledge-access-index-and-prune`. Both change `spektacular:cmd/root.go`, `spektacular:state.json`, `spektacular:internal/config/config.go`, `spektacular:internal/config/config_test.go`, `spektacular:internal/config/schema.go`, `spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py`, `spektacular:architecture/testing-architecture.md`, `spektacular:README.md`, `docs:src/pages/configuration.mdx`.
- Added: `000077_html-log-viewer` now depends on `000076_harbor-on-built-in-logs`. Both change `spektacular:spektacular.step`, `spektacular:cmd/stats.go`, `spektacular:README.md`, `docs:src/pages/run-stats.mdx`.

## 000072_knowledge-access-index-and-prune
### Approach
- Each knowledge store gets a `.usage-index.json` at its root, recording each entry's creation date, when tracking began, and discovery and retrieval counts with recent events. Only the knowledge commands write it, through a new atomic `store.Writer.Update` over a shared file lock (also used by the commit lock) and a temp-file rename, so parallel runs lose nothing.
- Recording is explicit in the command handlers so internal reads never count: `search` records discoveries; `read`, `always-applied` and `conventions` record retrievals; `write` creates a record, `delete` removes it. A read-only `knowledge usage` command shows the index.
- Old entries get a creation date from git's first-add date (else mtime); their tracking clock starts at first use after the upgrade, so the first prune cannot remove them.
- `knowledge prune [--dry-run]` first removes entries past the age setting, tracked that long and not retrieved in the window; then it hands the agent one instruction per remaining entry (text included), answered with `knowledge prune verdict`, which checks the claim against the store. The report covers every entry.
- Every "offer, then wait" instruction becomes "write it, then tell the user which entry, where and why"; the spek-knowledge skill's maintenance review becomes a prune intent.
- Rejected: a prune state machine, tracking inside `Set.Read`, frontmatter creation dates, a JSON Lines log, flock.

### Milestones and tasks
- M1, the knowledge base records usage: shared file lock and atomic store update; usage index per store; record usage from the knowledge commands.
- M2, prune: prune age setting; `knowledge prune` command.
- M3, agents write knowledge themselves: rewrite installed instructions to write then report; knowledge skill writes directly and drives prune; drop the "decline the knowledge offer" line from three Harbor suites.
- M4, documentation: README and maintainer knowledge doc; website (docs repo).

### Tasks a person must do
None. All 10 tasks are agent tasks.

### Out of scope
- Ranking search by usage; undoing or archiving prunes (recovery via git); scheduled prune.
- Category, tag, layout or tag-audit changes.
- Attributing ad hoc reads that name no spec while several workflows run (the run-id work in 000073/000074 can cover it).
- Regenerating this repo's installed instructions; new Harbor verifier assertions.

### Drafting assumptions
- Creation dates live in the usage index, not in entry frontmatter.
- Retrievals are attributed to the spec the caller passes (`spec` on `knowledge read`, `--spec` on `always-applied`/`conventions`); with none, the workflow is named only when exactly one runs.
- A failure to record usage never fails the read or search (`usage_warning` instead).
- Setting `knowledge.prune.max_age_days`, optional, 90 when absent, no settings-format change, not written by `init`.
- `--dry-run` rather than `--preview`; the prune run file is `.spektacular/knowledge-prune.json`, deleted when the report is delivered.
- Orchestrated children write knowledge themselves and list what they wrote in their hand-back; the implement partial reports a `## Knowledge written` list.
- Website gains "How usage is recorded" and "Pruning the knowledge base" sections.

### Project-wide rules
- Decides: `store.Writer` gains `Update(path, fn)`; every store implementation and test double adds it.
- Decides: new `internal/filelock`; `autocommit.AcquireLock` wraps it unchanged.
- Decides: `knowledge.prune.max_age_days` in `KnowledgeConfig`, no settings-format bump (000073 edits a different struct in `config.go`; the second to merge rebases).
- Decides: website configuration page, `knowledge` entry only (000073 edits the logging part).
- Decides: new `knowledge usage`, `knowledge prune` and `prune verdict` commands; `spec` on `knowledge read`; `--spec` on `always-applied`/`conventions`.
- Decides: the "Knowledge-Worthy Discovery Recognition" heading stays; only its body changes.
- Decides: orchestrated children list knowledge they wrote; changes spek-plan-epic, spek-implement-epic and the orchestrated-stop partial (also edited by 000074).
- Relies on: the changelog is written by the implement workflow's changelog step.

### Manual checks
- Harbor plan, implement, epic, implement-epic and goal-check runs after the change contain zero "should I save this to the knowledge base?" questions.
- A prune dry run on this project's knowledge base, with the installed binary between workflows, lists every entry with a verdict.
- A reviewer can check the evidence for every corrected or deleted entry without opening the code.
- An agent session that finds a durable fact writes the entry without asking and names its tier, store, path and reason.
- An agent-driven prune in a seeded project corrects a wrong flag, deletes an entry about a removed command, and keeps an unmet-target rule as current, with evidence.
- The website's knowledge-base and configuration pages read correctly in a local build.

## 000073_log-mode-and-logger-providers
### Approach
- The per-command recorder in `runRoot` becomes a provider-selected logger; a new leaf package `internal/tracelog` replaces `internal/sessionlog`.
- Each command writes one self-contained OpenTelemetry-model `span` record (trace and span ids, name, kind, start and end, status, attributes, resource).
- The `file` logger writes one JSON Lines file per run (`<created_at>_<agent>_<kind>_<name>.jsonl`) and one `standalone_<YYYYMMDD>.jsonl` per UTC day, each record a single append; its directory ignores itself; every failure is swallowed.
- The run id lives in workflow state as `run_id`, minted by `workflow.New`; `workflow.EnsureRunID` gives one to runs already in progress at upgrade.
- Some commands are never logged: this plan adds a logging-exempt cobra annotation (`logAnnotation: logExempt`, walked up parents by `logExempted`) checked in `runRoot`, and marks `status` with it; 000075 marks `stats` (epic decisions).
- Rejected: the OpenTelemetry SDK (in-memory spans, heavy), a run registry (contradicts the spec), a hashed session id (cannot tell restart from resume), renaming the env variable (breaks the maintainer's status-polling mod).

### Milestones and tasks
- M1, every command is logged by default, linked to its run: trace log package with the file logger; run identifier in workflow state; log settings replace debug, with migration; record every command through the configured logger.
- M2, old logs are trimmed automatically: keep only the most recent runs and days.
- M3, Harbor reports from the new logs: Harbor reader understands span records; Harbor suites read the new logs at the verbose level, set in their seeded test projects' config.
- M4, logging is documented: documentation site (docs repo); README and knowledge.

### Tasks a person must do
None. All 9 tasks are agent tasks.

### Out of scope
- Step, task, helper and user-wait spans (000074); room is reserved via the `record` discriminator, `spektacular.span.type` and `parent_span_id`.
- `stats` (000075), replacing Harbor's parsers (000076), the viewer (000077).
- Loggers other than `file`; tool, model and token capture; retention by age or size.
- Touching old `.spektacular/debug/` files; migrating this repo's `.spektacular/`; editing historical plans and changelog; converting Harbor's reference runs.

### Drafting assumptions
- Levels are `off | standard | verbose`, `standard` the default.
- One complete `span` record per command; attributes under `spektacular.*`, `process.command_args`, `process.exit_code`; verbose adds the response and state before and after.
- A command belongs to the after-state's run if in progress, else the before-state's, else it is standalone with a fresh trace id.
- A pre-upgrade run's id is written before the command runs, only when logging is on.
- File names come from the state's `created_at`; days are UTC; the retention sweep runs only when a new file is created; run files and day files are counted separately.
- `SPEKTACULAR_SESSION_LOG` is removed; there is no per-command logging override (epic decision). The Go suite's logging-off pass uses a test-only `-log-level` flag on `./cmd/`.
- The logs directory carries a `.gitignore` of `*` as well as the init/migrate rule.
- Harbor's reader accepts both formats so old fixtures stay valid.
- The docs page keeps `/debugging/`, retitled "Logging and debugging".

### Project-wide rules
- Decides: settings format 4 → 5 with `project4to5` (the only format bump in the epic).
- Decides: the migrate engine gains an idempotent `ignore` action; `init` writes `logs/`; the repo root `.gitignore` gains `.spektacular/logs/`.
- Decides: the record contract for 000074–000077: `record`, `trace_id`, `span_id`, `parent_span_id`, `name`, `kind`, `start_time`, `end_time`, `status{code: ok|error, message}`, `attributes`, `resource`; `spektacular.span.type` is `command` here.
- Decides: `run_id` lives only in workflow state, top-level, 32 hex.
- Decides: logs layout `.spektacular/logs/`, `log.config.directory`.
- Decides: Harbor's seeded test projects set `log.level: verbose` in config, seed with logging off, and verifiers switch logging off for their own CLI calls through a shared `loglevel.py` (000076 later returns the seeded configs to the default level).
- Relies on: changelog via the implement workflow; no task migrates this repo's install; knowledge updates through `spektacular knowledge write`.

### Manual checks
- A maintainer can read each step's start, end and duration from a real run's log alone.
- The default level adds under 10 ms to `spec file list`, over 100 runs with logging on and off.
- A complete implement run of a 10-task plan produces under 1 MB of log at the default level.
- No command's output or exit code differs between logging on and off across the Harbor workflow suites.
- Harbor plan and epic suites report the same steps and parallel runs as before, reading `.spektacular/logs/`.
- A read-through of the rewritten debugging page and configuration reference on the built site.

## 000074_trace-spans-for-agent-work
### Approach
- The workflow engine mints a span id on entering each step and keeps the open step span (and in implement the open task span) in state beside `run_id`; on each move, finish or resume it reports to an optional `SpanSink`, which `cmd` turns into `span_start`/`span_end` records. Each command record's parent is the step it ran in.
- Agents mark the rest with `spektacular trace start <name> --type sub-agent|user-wait|work [--parent <id>]` and `trace stop <id> [--status ok|error] [--message]`. The run's log is the registry of open spans; trace commands never write state.
- The id agents hold is `<trace_id>-<span_id>`, so a stop or parent reaches the right run, including a lane's.
- One shared instruction block, appended to every step instruction and included in every workflow skill, tells agents what to mark.
- Rejected: diffing state before and after (misses steps inside one command), open spans in state (races between helpers), a bare span id (cannot reach a lane).

### Milestones and tasks
- M1, every run's log shows its steps and tasks: span records and run log reading; workflow engine marks every step; implement tasks become spans; record step and task spans in every run's log.
- M2, agents can mark helpers and user waits: trace start and stop commands.
- M3, every workflow tells agents what to mark: trace instructions in every step and skill; Harbor reads and checks span records.
- M4, spans are documented: documentation site (docs repo); README and knowledge.

### Tasks a person must do
None. All 9 tasks are agent tasks.

### Out of scope
- Stats and excluding user waits (000075); Harbor's transcript timing (000076); the viewer (000077).
- Detecting helpers or user waits without the agent marking them; tool or model calls; changing how helpers launch.
- Linking an epic orchestrator's span to its child's run; closing standalone spans automatically.
- Trace wording in AGENTS.md standing rules; reworking the Harbor implement suite; reinstalling this repo's skills; editing historical plans.

### Drafting assumptions
- Abandoned and interrupted spans get status `{error, "abandoned"|"interrupted"}` plus `spektacular.span.end_reason` (`moved|finished|stopped|abandoned|interrupted`).
- A `span_end` repeats the whole span; a `span_start` omits end time and status.
- The command that moves the workflow is parented to the step it ran in.
- A task span stays open across a resume; `finished` is an instant span after which every open span is closed as abandoned.
- `--type` defaults to `work`; agents cannot start or stop step and task spans.
- Spans started outside any run go into a standalone trace and are never closed automatically.
- The spec's "≤2 extra tool calls per plan task" metric is read from the implement-epic and goal-check suites, because the Harbor implement suite runs no plan tasks (flagged for review).
- Epic child prompts pass each child its parent span id; a goto to the current step refreshes `step_span`.

### Project-wide rules
- Extends the 000073 record contract: `record: span_start|span_end`; `spektacular.span.type` `step|task|sub-agent|user-wait|work`; `spektacular.span.end_reason`; `spektacular.task.id`; `parent_span_id` on command records.
- Decides: state gains optional `step_span` and `task_span` beside `run_id`; no settings-format change.
- Decides: the file logger gains an optional `Reader` interface and a pure `tracelog.Index`.
- Decides: agent-visible span id `<32hex>-<16hex>`.
- Decides: shared partials `trace-marking.md` and `trace-helpers.md`, appended by stepkit before the orchestrated, commit and footer blocks and included after `version-check` in every workflow SKILL.md (000072 also edits the epic skills).
- Decides: Harbor's `sessionlog.py` skips span lines; new `tests/harbor/shared/spans.py` (replaced by 000076); trace oracle constants in `delegation.py` with a self-check.
- Relies on: span sections go into `debugging.mdx` as 000073 rewrites it; changelog via the implement workflow; no reinstall of this repo's skills.

### Manual checks
- In a Harbor implement run, step spans cover at least 95% of the time from first to last command.
- In Harbor implement, plan-epic and implement-epic runs, at least 90% of helper launches appear as sub-agent spans.
- In Harbor spec runs, at least 90% of questions to the scripted user appear as user-wait spans.
- Marking adds at most two tool calls per plan task on average, read from the implement-epic and goal-check suites.
- A read-through of the logging page's span sections on the built site.
- A read of one real epic run's log: sub-agent spans nest under the launching step, and a child's spans under the id it was given.

## 000075_run-stats-command
### Approach
- A `spektacular stats` group with `list`, `run`, `compare` and `summary`, over a new pure package `internal/runstats` that reads a logs directory without changing it, skips and counts damaged lines, groups records into runs by trace id, pairs span starts and ends, and builds the report, comparison or summary.
- `cmd/stats.go` is thin: `--dir`, `--format pretty|json`, selection flags `--name`, `--kind`, `--agent`, repeatable `--run`, `--gaps`, `--gap-threshold`, `--schema`.
- Rejected: 000074's `tracelog.Reader`/`Index` as the loader (one run at a time, drops parentless commands), a Harbor-side script, folding into `status`, summing waits (concurrent helpers would exceed wall time).

### Milestones and tasks
- M1, list runs and see where one run's time went: load logs into runs; report on one run; keep the stats commands out of the logs; list and report from the command line.
- M2, compare runs and summarise many: compare and summarise; from the command line.
- M3, the report is documented: documentation site (docs repo); README lists the stats commands.

### Tasks a person must do
None. All 8 tasks are agent tasks.

### Out of scope
- Tokens, model time and cost; the viewer (000077); Harbor changes (000076); cross-project and live reports.
- Standalone traces, including an epic orchestrator's own sub-agent spans (000076 extends the report for these).
- `work` span tables; step breakdowns for pre-000074 logs; telling a never-resumed session from a running one.
- Interpolated percentiles, `--last N`, colour, index or cache files; touching this repo's install.

### Drafting assumptions
- `--format pretty|json`, pretty by default; Harbor passes `--format json`.
- Filters combine with AND; `run` reports the latest match; `compare` uses the named runs in order, else the two latest of the latest run's kind; `summary` uses the named runs, else every run of that kind; `--run` takes a full id or a unique prefix of 8+ characters.
- Median is the true median; p90 is nearest-rank.
- Per-step `command_ms` stands in for the investigation's tool column.
- A run is a trace with workflow attributes; status is `finished`, `interrupted` (abandoned by restart, or superseded by a later run of the same name) or `in_progress`.
- Helper and user waits are unions of completed span intervals; unfinished ones are listed and flagged but left out of totals.
- Durations are integer milliseconds, times RFC 3339 UTC, percentages to one decimal.

### Project-wide rules
- Relies on: 000073's logging-exempt annotation (`logAnnotation: logExempt`), set on the `stats` group so no `stats` command is logged (epic decision); this plan no longer changes `runRoot`.
- Decides: the output contract 000076 and 000077 read, pinned by golden tests: integer `*_ms`, `log_dir` and `skipped_lines` on every success, `end_reason` (plus `open`) and `complete` on unfinished items, one generic row per figure in compare and summary.
- Relies on: 000073's record fields, file naming and `config.LogDir`; 000074's span meanings.
- Decides: docs add `src/pages/run-stats.mdx`, a nav entry after the logging page, and a pointer sentence in `debugging.mdx`.
- Relies on: changelog via the implement workflow; no template, skill, Harbor or settings-format change.

### Manual checks
- The investigation's per-step table (model, tool, helper-wait and user time) can be produced for a real run from the report alone, apart from model time.
- A report over a logs directory of 10 runs returns in under one second on a laptop.
- A field-by-field check that the JSON carries every step, helper and idle figure Harbor's `metrics.py` derives from transcripts today.
- `stats list`, `run`, `compare` and `summary` against real spec, plan and implement logs give plausible figures and readable tables.
- A read of the Run statistics page on the built site against shipped behaviour.

## 000076_harbor-on-built-in-logs
### Approach
- Harbor reads Spektacular's logs only by running `spektacular stats ... --dir <logs> --format json`, through a new shared `tests/harbor/shared/runlog.py` used by metrics, the epic timeline, the delegation rules and every verifier.
- `sessionlog.py` and 000074's `spans.py` are deleted; `transcripts.py` is cut back.
- The suites' seeded test-project configs drop 000073's verbose level, so runs record at the default level; logs reach trial artifacts through the existing `/app/.spektacular` artifact.
- `stats` gains two read-only listings Harbor needs: `stats commands` (each command's run, step, `step_to`, argv, exit code, message) and `stats spans` (helper, user-wait and work spans), both with `--outside-runs` for the epic orchestrator. 000075's `list` and `run` shapes are unchanged.
- Rejected: a Python log reader (spec requires the report), keeping verbose logging (spec requires the default level), changing `stats run`/`list` shapes (golden-pinned, reused by 000077).

### Milestones and tasks
- M1, `stats` lists commands and spans: run statistics package listings (Go); command line listings (Go); documentation site (docs repo); record baseline Harbor pass rates (human); pin today's token, model and cost figures.
- M2, Harbor measures and judges from the logs: Harbor run log reader; metrics record from Spektacular's logs (schema 3); delegation checks judge from spans; suite verifiers read the logs through stats; suite images record at the default level.
- M3, reports use the new measures and the old parsers are gone: epic timeline from the logs; cross-run report shows the per-step split (`report.py --compare A B`); remove the old parsers and debug fixtures; README and knowledge.

### Tasks a person must do
- Record baseline Harbor pass rates: needs Docker, the Harbor CLI and paid Claude logins, and must run before Milestone 2 merges.

### Out of scope
- New suites and any change to a suite's scenarios or pass and fail checks.
- The viewer (000077); tokens, cost or model time from the logs.
- Linking an orchestrator's span to the child's run; new-format reference runs; regrading trials in a separate container.
- Changing 000073–000075's figures, gap threshold, retention or levels; touching this repo's install.

### Drafting assumptions
- `--outside-runs` combined with a run selection is refused as `stats_bad_selection`.
- Epic hand-off gaps come from the orchestrator's sub-agent spans outside any run (epic decision: `status` is never logged): spawned after = the child's span start to its first command; picked up after = its last command to the span's end (or the orchestrator's next record if never stopped); children are matched to spans in launch order.
- The epic suite's `test_orchestrator_polled_epic_status` check is retired, since `status` polls are no longer logged (proposed at review; the only verifier assertion dropped).
- An implementing child's brief must name `<root>/.spektacular/worktrees/<spec>` (hand-kept constant with a self-check), replacing paths parsed from responses the default level no longer records.
- Dropped from metrics: per-step output characters and transcript sub-agent counts; child briefs stay.
- Required-child checks judge from spans but stay in the Claude-only `TestChildren` classes.
- Per-step `working = max(0, duration - helper wait - user wait)`; gap time uses the 5-minute default; multi-run trial totals are sums.
- Metrics schema 3 in seconds; `report.py` still reads schemas 1 and 2; `metrics.py --log-dir` with `--debug-dir` as an alias.
- A self-test holds the shared modules to 2,000 lines or fewer (from 2,999).
- `transcripts.py` keeps the turn, wake-up, child-background, launch and main-model readers that existing checks need; the plan reads "keeps only what … needs" as removing measures, not checks (flagged for review).

### Project-wide rules
- Decides: extends 000075's `cmd/stats.go` and `internal/runstats` (`Logs.Outside`, two subcommands, new golden files), existing shapes unchanged; 000077 also extends these files, so the second to land rebases and keeps both sets of golden files.
- Relies on: 000073's default-level attributes `process.command_args`, `process.exit_code`, `spektacular.step`, `spektacular.step.to` and the status message; default retention of 10 runs.
- Decides: returns 000073's seeded configs to the default level, removes 000073's Harbor normalising reader, and moves 000074's Harbor span checks onto `runlog` (000074's oracle constants stay).
- Relies on: knowledge entries `architecture/testing-architecture.md` and `gotchas/harbor-agent-logs-and-artifacts.md` are edited by 000073, 000074 and this plan in that order; `gotchas/harbor-agent-log-fields.md` by this plan.
- Relies on: changelog via the implement workflow; Python tests build a throwaway binary and never install it.

### Manual checks
- For the same scenario, Claude, Codex and Bob records report the same steps, a helper count for every helper-starting step, and no agent-specific fields.
- No suite's pass rate drops, comparing 5 Claude runs per suite before (baseline task) and after.
- A Harbor implement run under each of Claude, Codex and Bob: the record's steps and timings match `stats run` on the trial's logs.
- A Codex implement run that starts helpers gives a non-zero helper count, each placed in its spec and step.
- A spec run's record shows total user wait, total gap time and each step's working, helper-wait and user-wait split.
- The epic timeline for an implement-epic trial lists each child's start, finish, slowest step and both hand-off gaps, with no debug directory present.
- A collected trial holds `artifacts/spektacular/logs/`, and `stats run --dir` on it gives the step timings.
- With a trial's logs removed, `metrics.py` reports the measures as unknown with a reason, exits 0, and the reward is unchanged.
- `report.py --compare` over two real trials shows each per-step split, the totals and the differences.

## 000077_html-log-viewer
### Approach
- `spektacular stats view` is a fifth `stats` form: it selects a run as `stats run` does, inherits the group's flags and its never-logged marker, and adds `--out`.
- A new pure package `internal/runview` builds a compact page model from a `runstats.Run` and `runstats.BuildReport`: one row per span and command nested under its parent, gaps of 1 s or more, the five longest steps, and the report's totals copied rather than recomputed.
- The model is embedded as JSON in a page shell; inline CSS and a plain script draw only the rows in view on a canvas.
- Output goes to `spektacular-run-<id8>.html` in the current directory or where `--out` says; `--out` inside the logs directory is refused; the page holds no generation time or paths, so a copied logs directory renders byte-identically.
- Rejected: pairing records in the browser, one SVG element per span, a charting library, a top-level command, a JS engine for tests.

### Milestones and tasks
- M1, any recorded run opens as a waterfall page: build the page data from a run; write the run as a self-contained page; render a run from the command line.
- M2, large runs can be explored: zoom, collapse and summarise on the page.
- M3, the viewer is documented: documentation site (docs repo, new `run-viewer.mdx`); README lists `stats view`.

### Tasks a person must do
None. All 6 tasks are agent tasks.

### Out of scope
- A live view or server, several runs on one page, tokens or cost, editing (spec Non-Goals).
- Opening a browser automatically; a stdout form of the page; standalone traces; full command output and state on the page.
- Any change to `runstats`, its JSON or Harbor; automated browser tests; touching this repo's install.

### Drafting assumptions
- The viewer is `stats view`, not a top-level command or `--format html`.
- Go builds the page data and the script only draws; canvas, rows in view only, no SVG or library.
- `--out` takes a file or directory; an existing file is replaced through a temp file; with several `--run` values the first is rendered.
- Gaps are embedded down to min(1 s, threshold); the page's threshold control cannot go lower.
- The page opens collapsed to steps; a collapsed row shows the worst problem inside it; a side summary lists the longest steps and gaps.
- Unfinished spans are drawn to 000075's capped end and marked; agent and workflow in the details come from the run.
- Fixed look: task purple, step blue, sub-agent teal, user-wait amber hatched with a full-height band, work olive, command thin slate; failed red ✕, interrupted dashed orange ‖, abandoned dotted grey ∅, open fading ….
- Sample pages for manual checks come from a test flag (`-sample-dir`).

### Project-wide rules
- Relies on: 000075's never-logged rule and its no-writes-into-the-logs guarantee (extended by refusing `--out` there).
- Relies on: 000075's `internal/runstats` API and `stats` JSON and error codes as a fixed contract (000076 also extends `cmd/stats.go`; the second to land rebases).
- Decides: page assets ship inside the binary through `go:embed`, with no build step, npm or external URL.
- Decides: docs nav order "Logging and debugging" (000073), "Run statistics" (000075), "Run viewer"; the README `stats` block gains one line.
- Relies on: changelog via the implement workflow.

### Manual checks
- On a sample implement run, a maintainer finds the slowest step and longest stall from the page in under a minute.
- On the every-type sample page, each span type looks different, user waits stand apart, and the failed, interrupted and abandoned items are each marked.
- Selecting a helper bar shows its name, type, start, duration, status, agent, workflow and step.
- A 20-minute pause is marked at the default threshold and a 2-minute one is not; at a 1-minute threshold both are.
- Zooming into a step fills the width with that period; collapsing a step hides its children until expanded.
- The page renders fully with networking off and no server.
- A 5,000-span page renders in under 3 s and a zoom or selection responds in under 200 ms in current Chrome and Firefox.
- A visual review on a real implement run of the legend, colours, bands, markers, axis, details panel and summary, with no em dashes in visible text.
- A read of the Run viewer docs page on the built site against shipped behaviour.
