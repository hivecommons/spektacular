---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Plan: 000078_token-use-in-run-stats

<!-- Metadata -->
<!-- Created: 2026-10-25T10:36:41Z -->
<!-- Commit: 2445aa0 -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Spektacular's run report (`spektacular stats`, spec 000075) gains token use and cost beside its timings. For a run driven by Claude Code, the report gives:
- the input, output, cache-read and cache-write tokens of the run, each step, each plan task and each helper agent;
- the models used;
- the cost, from an optional price table in the project's configuration;
- working time per thousand output tokens.

It finds them in the agent's own session files without the agent doing anything extra. Comparisons and summaries carry the same figures. Maintainers can then judge each part of a run by how long it took and how much work it did, so a step that is slow because it did a great deal is told apart from one that is slow while doing little, and runs are compared fairly.

## Conventions

- **Error messages must describe the problem and suggest remediation** — the new `stats.prices` validation errors are `output.NewError("config_invalid", …).WithNextAction(…)` naming the key and the accepted shape; token data that cannot be read is never an error but an unknown with a stated reason.
- **A plan never changes the active skills and configuration** — the optional `stats.prices` key needs no settings-format bump, no template, skill or managed section changes, and no task touches this repo's `.spektacular/config.yaml`; manual checks use the built binary on real logs and sessions, read-only.
- **Tests must not depend on execution order** — `cmd` tests go through `runRootCmd`/`resetRootCmd` (the new `--sessions-dir` flag is reset with the tree), fixture sessions live in each test's temp directory with fixed timestamps and explicitly set file modification times, and `TestMain` pins the default sessions directory to an empty temp directory so no test reads the developer's home or the wall clock.
- **Passing tests are required before calling work done** — the report changes touch 000075's golden files and every `stats` subcommand; the full shuffled Go suite must pass.
- **Spektacular's own files are written through Spektacular** — agent session files and logs are not stores, so the reader opens them directly and read-only; no task writes a store file by hand.
- **Plans must sketch content structure (docs repo)** — the new Run statistics sections and the `stats` configuration key carry content outlines with the verified field names, keys and reasons.
- **No em dashes (docs repo)** — all new docs, README and changelog prose avoids them.
- **MDX authoring conventions (docs repo)** — the additions go inside the existing `Prose` block of `run-stats.mdx` and a slot-pattern `ConfigKey` on `configuration.mdx`, with fenced code blocks and no layout HTML.
- **Site layout conventions (docs repo)** — no new component, width or heading size; existing sections only.

## Architecture & Design Decisions

**Shape.** Token use joins the run report 000075 builds, as two new halves around it. All of the code is in the `spektacular` repo; only the documentation is in the `docs` repo.
- **A reader for agent sessions (new leaf package `internal/agentusage`).** It finds and reads Claude Code session transcripts read-only and turns them into plain data. Each transcript (the main session, or one helper) becomes its model requests (time, model, input, output, cache-read and cache-write tokens), its shell calls (command text, start and end), its helper launches (tool-call id, brief, time), and its link to the launch that started it. The session's own usage totals come from Claude Code's `cost-state` record when present. It knows nothing of runs or logs. Candidate sessions are every session under the Claude projects directory whose files were modified at or after a run's start. The directory is `$CLAUDE_CONFIG_DIR/projects`, else `~/.claude/projects`, or the new `stats --sessions-dir`. So nothing depends on how Claude Code names its project folders.
- **Attribution and figures (new files in `internal/runstats`).** These are pure functions from a run and the parsed sessions to token figures. `BuildReport` keeps its signature, which 000077 relies on. Its `Options` gain one optional field carrying the sessions, the project's price table and the reason when nothing could be read. `Compare` and `Summarise` pass it through. `cmd/stats.go` loads the logs as before, asks the reader for the sessions that could cover the selected runs (parsing each file once per command), reads `stats.prices` from the project's configuration, and hands both to the package.

**How tokens are placed.** These rules are what make the figures trustworthy.
- **Finding the driving session.** A transcript drove a run when the run's own `<kind> new` and `<kind> goto` command records fall inside that transcript's shell calls. A record matches when its start time lies within the call's interval, with one second of tolerance, and the call's command text contains the command's first two words. Every matching transcript is a driver, so a run resumed in a second session is read from both. An orchestrated child is matched to its own helper transcript inside the orchestrator's session. This needs no agent action and no hook: the spec's preferred approach, so the hook fallback is not built.
- **Placing the main agent's requests.** Each request is timed at the first record of its response. It goes to the step entry and task span open at that moment. A request outside every step entry is counted as outside any step. When one transcript drove several runs, a request between runs goes to the next run it drove, and one after the last run goes to that last run. So a session that drove only one run gives that run all of its requests.
- **Placing helpers.** A helper transcript belongs to the driver whose `Agent` call launched it, found through the helper's recorded launch id. Helpers that helper started are rolled into it. It is matched to a `sub-agent` span of the run in one of three ways:
  - first, by the span id 000074's instructions put in every helper's brief;
  - otherwise, in launch order to the latest unmatched span that started at or before the launch;
  - otherwise it is listed as an unmatched helper.

  All its tokens go to that launch, and to the step and task the launch happened in, even when it finishes after the step ends. A helper transcript that drove a run of its own counts there, never as a helper.
- **Session overhead.** Claude Code counts requests its transcript never records, such as permission checks and titles. On real sessions these were 97–99% of uncached input and up to 64% of output. When a session drove only this run and holds Claude Code's own totals, the difference is reported as session overhead. It is counted in the run total and in the tokens outside any step, so the step figures plus the outside figure always equal the total. Otherwise overhead is unknown with a reason.

**The figures.**
- **Tokens.** Every token figure gives input, output, cache-read and cache-write tokens.
- **Cost.** Cost is each type's tokens times that model's price per million tokens from `stats.prices`, in whatever currency the table uses. A model with no price has unknown cost, never zero. A total that includes unpriced tokens gives the priced part's cost, is marked incomplete and names the unpriced models.
- **Time per token.** Working time per thousand output tokens:
  - the run uses working time;
  - a step or task uses its duration less its user wait;
  - a helper uses its own duration.

  It is `null` (not applicable) with no output tokens.
- **Unknown, never zero.** Missing data is reported as unknown, with a reason code and message, and never as an error or a failed report. That covers an unsupported agent, a missing sessions directory, no matching session, an unreadable file or an unrecognised format. Timings are untouched by it.

Only runs whose agent is `claude`, or unset (`unknown`), are looked up. Every other agent is unknown with the reason.

**The output only grows.** 000075's JSON is the contract for 000076 and 000077, so every existing field, figure, order and unit stays as it is (constraint: existing figures must not change).
- Each `steps`, `tasks` and `helpers` row gains a `tokens` object.
- The report gains a top-level `tokens` section: status, reason, sessions, total, outside any step, session overhead, the models used with their tokens, and unmatched helpers.
- `compare` and `summary` append six figures after their existing ones, for totals and for each step: the four token types, `cost` and `working_ms_per_1k_output`.
- The element type of the figure row's values widens from integer to float, so a fractional cost fits. Integer figures still encode as JSON integers.

A contract test proves the change is additive. It strips the new keys from the regenerated golden files and compares them with 000075's golden files, kept as a pre-token copy.

**Read-only and never logged.** The reader only opens files read-only. `stats` already carries 000073's logging-exempt annotation. Tests hash every log and session file before and after a report. Prices are a new optional top-level `stats` key in `config.yaml`. Like 000072's optional `knowledge.prune` key, it needs no settings-format change and `init` does not write it. An incomplete or negative price is refused as `config_invalid` with a next action (error-messages-must-suggest-remediation).

**Why this direction.** The rejected options were a hook installed by `init`, Claude's headless stream-json output, `cost-state` alone, transcript requests alone, a separate `stats tokens` command, built-in prices, and matching by run name or by time alone. They either need agent-side setup, miss interactive sessions, lose the per-step split, miss most of what Claude Code counts, separate tokens from timings, or break concurrent launches. See `research.md#alternatives-considered-and-rejected`. Keeping the reader a separate leaf leaves `runstats` pure, so the attribution rules are table-tested with hand-built sessions. It also means a reader for another agent can be added later without touching them. The plan changes no template, skill, settings format or this repository's own `.spektacular/` (plans-never-change-the-active-install). Tests use fixture sessions with fixed timestamps and set file modification times, never the wall clock (tests-must-not-depend-on-order).

## Component Breakdown

- **Agent session reader (new, `spektacular` repo).** A leaf package with no dependency on runs, logs or configuration. It owns:
  - finding candidate Claude Code sessions under a projects directory by modification time;
  - reading each session's main transcript and helper transcripts read-only, line by line, skipping and counting lines it cannot read;
  - turning them into plain data: model requests de-duplicated by response, shell calls with their start and end, helper launches with their brief, each helper's launching call and parent helper, and the session's own usage totals when Claude Code has written them;
  - saying why a session could not be used (a missing directory, an unreadable file, no records it recognises), so the report can give a reason instead of failing.

  It knows only Claude Code's format today. Its types are neutral, so a reader for another agent can be added beside it later.
- **Token attribution and figures (new, inside the run statistics package, `spektacular` repo).** Pure functions over one run and the parsed sessions. They own:
  - matching a run to the transcripts that drove it, through the run's own `new` and `goto` commands;
  - placing every main-agent request in a step entry and a task, or outside any step;
  - matching helper transcripts to `sub-agent` spans and placing their tokens on the launch and its step and task;
  - the session-overhead figure;
  - pricing tokens from the price table, with unknown and incomplete costs;
  - the working-time-per-thousand-output-tokens figure;
  - the unknown status and reason when nothing usable was found.

  They use the run report's step entries, tasks, helpers and waits, and never reread the logs.
- **Run report, comparison and summary (changed, run statistics package from 000075).** The report gains a token object on every step, task and helper row and a top-level token section. The comparison and the summary append the token, cost and time-per-token figures to their existing figure lists, for totals and for each step. The generic figure row keeps its shape. Only its values widen to hold a fractional cost. Report options gain one optional field carrying the sessions, prices and any reason. With it absent, the report gives tokens as unknown, and every existing figure is identical.
- **Text renderer (changed, run statistics package).** It adds output tokens, cost and time per thousand output tokens to the Steps, Tasks and Helpers tables. It adds a Tokens block (each step's four token types and cost, outside any step, session overhead, total), a Models table and unmatched helpers. For an unknown result it prints one line with the reason. It shows the same added figures in the comparison and summary tables.
- **Stats command group (changed, `spektacular` repo).** It gains a persistent `--sessions-dir` flag. Every report form does the following:
  - resolves the sessions directory (`--sessions-dir`, else `$CLAUDE_CONFIG_DIR/projects`, else `~/.claude/projects`);
  - asks the session reader for the sessions that could cover the selected runs, reading each file once per command;
  - reads the price table from the project's configuration when there is one;
  - passes both to the package.

  It adds no refusal: unreadable session data is a reason in the output, never an error. It keeps 000073's logging-exempt marker, so `stats` is still never logged.
- **Project configuration (changed, `spektacular` repo).** It gains an optional `stats` section holding the price table. The table maps each model to its input, output, cache-read and cache-write prices per million tokens. It is validated with remediation: every model needs all four prices, none negative. It has no default, is not written by `init`, and needs no settings-format change.
- **Command test harness (changed, `spektacular` repo).** The `cmd` package's test entry point pins the default sessions directory to an empty temporary directory. So no test reads the developer's own Claude Code sessions. Tests that need sessions pass fixture directories explicitly.
- **README (changed, `spektacular` repo).** The `stats` block says that reports include tokens and cost for Claude Code runs, and that prices come from `stats.prices`.
- **Documentation site (changed, `docs` repo).** The Run statistics page gains sections on tokens and cost:
  - which agents are supported;
  - how a run is matched to its session;
  - what each figure means;
  - the price table;
  - how unknown and incomplete values appear.

  The configuration reference gains the `stats` key and an example `stats.prices` block.

## Data Structures & Interfaces

**Input: Claude Code session files (read, never written).** The session reader reads these, under the projects directory (`$CLAUDE_CONFIG_DIR/projects`, else `~/.claude/projects`, or `--sessions-dir`):
- `<project>/<session>.jsonl`, the main transcript;
- `<project>/<session>/subagents/agent-<id>.jsonl`, one helper transcript each;
- `<project>/<session>/subagents/agent-<id>.meta.json`, that helper's `toolUseId`, `parentAgentId` and `description`.

It uses only these records:
- `assistant`: `timestamp`, `message.id`, `message.model`, `message.usage.{input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens}`, and its `tool_use` blocks (`id`, `name`, `input.command` for `Bash`, `input.prompt` and `input.description` for `Agent`/`Task`);
- `user`: its `tool_result` blocks (`tool_use_id`), with the record's `timestamp`;
- `cost-state`: `modelUsage.<model>.{inputTokens, outputTokens, cacheReadInputTokens, cacheCreationInputTokens}`, Claude Code's own totals for the session.

Every other record and field is ignored. A line that does not parse is skipped and counted.

**Input: the run logs (read, unchanged).** These are 000073's command records (`process.command_args`, `start_time`, `end_time`, `spektacular.agent`) and 000074's step, task and `sub-agent` spans, as 000075 assembles them.

**Configuration (`config.yaml`, optional, no format change).**

```yaml
stats:
  prices:                    # per million tokens, in any one currency
    claude-opus-5-5:         # the model name exactly as the report lists it
      input: 5.00
      output: 25.00
      cache_read: 0.50
      cache_write: 6.25
```

```go
// internal/config
type StatsConfig struct{ Prices map[string]ModelPrice `yaml:"prices,omitempty"` }
type ModelPrice  struct{ Input, Output, CacheRead, CacheWrite *float64 } // yaml: input, output, cache_read, cache_write
// Config gains: Stats StatsConfig `yaml:"stats,omitempty"`
```

**Command line.** The `stats` group gains one persistent flag, `--sessions-dir <path>`, the Claude Code projects directory to search. Every existing flag, default and error code is unchanged. No new error code is added: unreadable session data becomes a reason in the output.

**Structured output (additions only; every 000075 field, order and unit is unchanged).**

*Token figure.* This is one shape, used for the run total, outside any step, session overhead, and every step, task and helper row:

```json
{"input":1442,"output":641466,"cache_read":158466888,"cache_write":2554795,
 "cost":43.18,"cost_complete":false,"unpriced_models":["claude-haiku-5-5"],
 "working_ms_per_1k_output":2811}
```

- Token counts are integers.
- `cost` is the priced part, rounded to 6 decimals. It is `null` when the figure holds tokens and none of them are priced. It is `0` only when there are no tokens.
- `cost_complete` is false when any token in the figure is unpriced, and `unpriced_models` names those models (`[]` otherwise).
- `working_ms_per_1k_output` is integer milliseconds, or `null` with no output tokens. It uses the run's working time, a step's or task's duration less its user wait, or a helper's own duration. It is `null` on outside-any-step and session-overhead figures, which have no time.

*`stats run`*: each `steps[]`, `tasks[]` and `helpers[]` row gains `"tokens": <token figure>` (`null` when token use is unknown, or for a helper launch with no transcript). The report gains a top-level section:

```json
"tokens":{"status":"known","reason_code":null,"reason":null,"agent":"claude",
  "sessions":[{"session_id":"bc997d3b-1caa-4579-9318-a6795dd8b757","transcript":"main",
               "file":"/home/u/.claude/projects/-home-u-proj/bc997d3b-….jsonl"}],
  "skipped_lines":0,
  "total":{"...":"token figure"},
  "outside_steps":{"...":"token figure, session overhead included"},
  "session_overhead":{"...":"token figure"},"session_overhead_reason":null,
  "models":[{"model":"claude-opus-5-5","input":1442,"output":641466,"cache_read":158466888,
             "cache_write":2554795,"cost":43.18}],
  "unmatched_helpers":[{"name":"Inspect transcripts","step":"discovery","task_id":null,
                        "started_at":"...","ended_at":"...","duration_ms":150000,
                        "tokens":{"...":"token figure"}}]}
```

- `status` is `known` or `unknown`. When it is unknown, `reason_code` and `reason` say why, `total`, `outside_steps` and `session_overhead` are `null`, and the lists are empty. Reason codes:
  - `agent_unsupported`: the run's agent is not Claude Code;
  - `sessions_dir_missing`;
  - `session_not_found`;
  - `session_unreadable`;
  - `format_unrecognised`;
  - `not_read`: the report was built without session data, for example by another package.
- `session_overhead` is `null` when it cannot be known. `session_overhead_reason` then says why:
  - `several_runs`: the session drove more than this run;
  - `no_totals`: the session has no Claude Code totals yet;
  - `totals_below_transcript`.
- `models[].cost` is `null` for an unpriced model.
- Invariant: the sum of `steps[].tokens` plus `outside_steps` equals `total`, for each token type.

*`stats compare` and `stats summary`*: the figure lists gain six entries, appended after the existing ones, for `totals` and for each step: `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `cost`, `working_ms_per_1k_output`. They use the existing generic rows (`values`/`delta`/`delta_pct`, or `count`/`median`/`p90`). A run whose token use is unknown, or a cost that is `null`, gives `null` there. An incomplete cost is shown as its priced part, and the run's own report says it is incomplete.

**Go interfaces.**

```go
// internal/agentusage (new leaf package)
type Usage struct{ Input, Output, CacheRead, CacheWrite int64 }
type Request struct{ At time.Time; Model string; Usage Usage }   // one per response, at its first record
type ShellCall struct{ Command string; Start, End time.Time }   // Bash tool call to its result
type Launch struct{ ToolUseID, Description, Prompt string; At time.Time }
type Transcript struct {
    ID, File               string // "main", or the helper's agent id
    LaunchID, ParentID     string // the Agent call that started it; its parent helper ("" for depth 1)
    Description            string
    Start, End             time.Time
    Requests               []Request
    Calls                  []ShellCall
    Launches               []Launch
}
type Session struct {
    ID          string
    Transcripts []Transcript      // main first
    Totals      map[string]Usage  // Claude Code's own totals by model; nil when absent
    Skipped     int
}
type Problem struct{ Code, Message string } // a reason code from the list above
type Claude struct{ Dir string }
func DefaultClaudeDir() string                                       // $CLAUDE_CONFIG_DIR/projects or ~/.claude/projects
func (c Claude) Sessions(since time.Time) ([]*Session, []Problem)   // read-only; never an error
```

```go
// internal/runstats (additions)
type Price  struct{ Input, Output, CacheRead, CacheWrite float64 } // per million tokens
type Prices map[string]Price
type TokenInput struct {
    Sessions []*agentusage.Session
    Problems []agentusage.Problem
    Prices   Prices
    AllRuns  []*Run // every run in the logs, to split a session that drove several
}
// Options gains:  Tokens *TokenInput   (nil: token use reported as unknown, reason not_read)
type TokenFigure struct { /* fields as the JSON token figure */ }
type TokenSection struct { /* fields as the JSON tokens section */ }
// Report gains Tokens TokenSection; StepRow, TaskRow and HelperRow gain Tokens *TokenFigure.
// Figure.Values and Figure.Delta widen from []*int64 to []*float64 (integer values encode unchanged).
```

`BuildReport`, `Compare` and `Summarise` keep their signatures. `cmd` converts `config.StatsConfig` into `runstats.Prices`, so `config` imports neither package.

## Implementation Detail

**A second read-only source beside the logs.** 000075 gave `runstats` one I/O edge, its log loader. This plan adds a second source, kept in its own leaf package because its files live outside the logs directory and belong to the agent, not to Spektacular.
- The session reader mirrors the log loader's habits:
  - it streams line by line with no length cap;
  - it opens files read-only;
  - it skips and counts lines it cannot read;
  - it never fails the report: every problem comes back as a reason code with a message.
- It pre-filters lines by their record type before decoding. Large tool results (`user` lines) are decoded only for their `tool_result` ids, and attachment and request-body lines are skipped outright, so a 10 MB session parses in well under a second.
- A developer reading it finds one file per concern: discovery of candidate sessions, the transcript parser, helper metadata, and Claude Code's own totals. Each has a table test fed with small hand-written JSON Lines fixtures copied in shape from real transcripts.

**Attribution as a pure pass after the report.** Inside `runstats`, token work runs after the timing report is built, and reads only that report's step entries, task spans and helper rows plus the parsed sessions. It never changes a timing figure. Its stages, one file each, each with table tests over hand-built runs and sessions:
- **match:** find the run's driver transcripts by its `new` and `goto` commands;
- **place:** put each request in a step entry and task, or outside any step, with the between-runs rule;
- **helpers:** match helper transcripts to `sub-agent` spans, by span id, then launch order, else unmatched;
- **overhead:** subtract the transcript's requests from Claude Code's totals, when allowed;
- **figures:** sum by row, price, and compute time per output token.

The rules for "which bucket does this token go into" live in placement alone. Every figure is a sum over placed items, so the invariant "steps plus outside equals total" holds by construction and is tested once.

**Additive output, proven additive.** The report types gain fields; nothing is renamed or reordered.
- The compare and summary figure lists are appended to.
- The generic figure row's values widen to floating point, so a fractional cost fits. Go's JSON encoder writes integral floats exactly as it wrote integers, so existing figures look the same on the wire.
- Before regenerating 000075's golden files, the implementer copies them into a "pre-tokens" set. A contract test deletes the new keys and appended figures from today's output and requires the result to equal that set byte for byte. Any accidental change to an existing figure fails there before it reaches 000076 or 000077.
- The text renderer's tables grow columns. Text output was never a contract, so its golden files are simply regenerated.

**The command layer stays thin.** `cmd/stats.go` gains one helper, used by `run`, `compare` and `summary`. It does three things:
- resolves the sessions directory;
- calls the reader once with the earliest start among the selected runs;
- reads `stats.prices` through the existing config loader when a project is present. Outside a project, or with no table, every cost is unknown and says so.

It builds the report options as before, plus the new token input. The flag follows the existing persistent-flag pattern and is reset by `resetRootCmd` like every other. The `cmd` test entry point, which already pins the build version, also points `CLAUDE_CONFIG_DIR` at an empty temporary directory, so existing `stats` tests see "no sessions" rather than the developer's home.

**Configuration follows the optional-key precedent.** `stats.prices` is an optional section that defaults to empty, validated in the config package's existing validator style with a `config_invalid` error and a next action. It is never written by `init` or `migrate`, exactly like 000072's optional `knowledge.prune` setting, and so needs no settings-format change. The config package keeps its own plain types, and `cmd` converts them to the `runstats` price table, so `config` imports neither `runstats` nor `agentusage`.

**Followed versus new.** Followed patterns:
- the read-only loader that skips and counts bad lines (000075);
- pure analysis with a separate renderer (`internal/status`, 000075);
- "unknown, never zero" with a reason (Harbor's `transcripts.py`);
- launch-order matching of helpers to spans (000076);
- the config validator's remediation style.

New patterns:
- reading another tool's files from the user's home directory;
- a figure row whose values may be fractional;
- the "pre-tokens golden" additivity test.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports none).
- **Upstream spec `000075_run-stats-command` must land first.** It is planned but not implemented. This plan extends:
  - its `internal/runstats` package: `Run`, `Span`, `BuildReport`, `Options`, `Report` and its step, task and helper rows, `Compare`, `Summarise`, the generic `Figure` and `Stat` rows, the figure order lists, and the text renderer;
  - its golden files;
  - its `cmd/stats.go` group, with the shared flags, `statsLogs` and the output writer.

  No task starts until 000075's Milestone 2 is merged. If only names differ, adapt to them. If a figure's meaning or the JSON contract differs from its plan, STOP and ask.
- **Upstream spec `000074_trace-spans-for-agent-work` (through 000075).** This plan relies on:
  - step and task spans, which place main-agent requests;
  - `sub-agent` spans with `spektacular.step` and their composite id `<trace>-<span>`, put in each helper's brief by its instructions;
  - an interrupted step ending at its last command.

  No change to it.
- **Upstream spec `000073_log-mode-and-logger-providers` (through 000075).** This plan relies on:
  - command records with `process.command_args`, `start_time`, `end_time` and `spektacular.agent` at the default level;
  - `config.LogDir`;
  - the logging-exempt annotation the `stats` group already carries.

  No change to it. Its settings-format bump to 5 is the only one in the epic; this plan adds an optional key without a bump.
- **Concurrent specs `000076_harbor-on-built-in-logs` and `000077_html-log-viewer`.** Not dependencies, but they share files.
  - 000076 adds `stats commands` and `stats spans`, golden files and a README and `run-stats.mdx` section.
  - 000077 adds `stats view`, a README line and nav. It calls `BuildReport(r, Options{Gaps, GapThreshold})` and reads `Report.Totals`, both kept source-compatible here.

  Whichever lands later rebases onto the others' additions to `cmd/stats.go`, `internal/runstats/render.go`, `README.md` and `docs:src/pages/run-stats.mdx`, and keeps every set of golden files. Harbor parses `stats run --format json` and ignores the new fields; it needs no change (spec Non-Goals).
- **Concurrent specs `000072` and `000073` on `docs:src/pages/configuration.mdx`.** 000073 swaps `debug` for `log` and 000072 edits the `knowledge` entry. This plan adds a `stats` entry, an example block and raises the top-level key count by one. The later change rebases.
- **Claude Code's on-disk session format (external, not versioned by Spektacular).** These are the transcripts under `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR/projects`), grounded on Claude Code 2.1.309 records. A change in it degrades to unknown token figures with `format_unrecognised`, never an error.
- **`internal/config` (changed).** It gains the optional `stats.prices` section and its validation. No settings-format change.
- **`internal/output` (used, unchanged).** It provides the success envelope and `config_invalid` errors with `WithNextAction`.
- **Go standard library only** (`bufio`, `encoding/json`, `os`, `path/filepath`, `sort`, `strings`, `time`, `math`), plus the existing `cobra`, `yaml.v3` and `testify`. There is no new module.
- **Documentation site, `docs` repo (changed).** It gains sections in `src/pages/run-stats.mdx` (created by 000075) and a `stats` entry in `src/pages/configuration.mdx`. It is checked by the site build, `astro check` and the MDX layout grep.
- **`README.md` (changed).** The `stats` block that 000075 adds.
- **Harbor test harness (read only).** `tests/harbor/shared/transcripts.py` was read as the reference for Claude field names. It is not changed.

## Testing Approach

**Test types and where they slot in.** Every Go test runs under the project's shuffled, order-independent `go test`. No test reads the wall clock or the developer's home directory.
- Fixture sessions are small JSON Lines files shaped like real Claude Code transcripts (main, helper, helper meta, `cost-state`), with fixed timestamps, in each test's own temp directory.
- File modification times are set explicitly wherever discovery depends on them.
- The `cmd` test entry point points the default sessions directory at an empty temp directory, so 000075's existing `stats` tests see "no sessions" and stay deterministic.

- **Unit tests for the session reader.** Table-driven over fixture files. They cover:
  - responses split over several records counted once, keeping the largest usage;
  - a request timed at its response's first record;
  - shell calls paired with their results;
  - helper launches and helper metadata, with depth-2 helpers linked to their parent;
  - Claude Code's totals read from `cost-state`;
  - discovery by modification time, including a session whose only recent writes are in its helper files;
  - each problem code: missing directory, unreadable file, unrecognised format;
  - malformed lines skipped and counted;
  - a read-only check that hashes every fixture file before and after reading.
- **Unit tests for attribution** (`runstats`, pure, hand-built runs and sessions). These carry the most coverage, because the placement rules are the product. Cases:
  - **Matching:** a run matched through its `new` and `goto` commands; a run resumed in a second session read from both; an orchestrated child matched to its helper transcript; no match giving `session_not_found`; an unsupported agent giving `agent_unsupported`.
  - **Placement:** requests in two steps and outside any step, with steps plus outside equal to the total for every type; a request issuing a `goto` placed in the step it was made in; two tasks each with their own tokens; a session that drove two runs, split by the between-runs rule.
  - **Helpers:** a helper launched in one step and finishing after the next began, with all its tokens on its launch and the launching step; matching by span id in the brief, by launch order for two concurrent launches without ids, and an unmatched helper listed; a depth-2 helper rolled into its parent; a helper transcript that drove its own run not counted as a helper.
  - **Overhead:** Claude Code's totals minus the transcript's requests when the session drove only this run, so the run total equals Claude Code's totals; unknown with `several_runs`, `no_totals` and `totals_below_transcript`.
  - **Cost:** two priced models, where cost equals tokens times price summed over types and models; one of two models unpriced, giving the priced part, `cost_complete: false` and the model named; no price table, giving a null cost; zero tokens, giving a cost of 0.
  - **Time per token:** working time per thousand output tokens for the run, a step (duration less user wait), a task and a helper (its own duration, unaffected by a concurrent helper); `null` with no output tokens; `null` outside steps.
  - **Models:** a run using two models lists both with their tokens.
- **Comparison and summary tests.** The six appended figures in order, for totals and steps:
  - deltas and percentages against the first run;
  - `null` for a run with unknown tokens and for a null cost;
  - median and p90 of tokens, cost and time per token over five runs.
- **Contract tests.**
  - Regenerated golden JSON for each mode pins every new field name and unit.
  - The additivity test removes the new keys and appended figures and requires 000075's pre-token golden files byte for byte.
  - A report built without token input gives `status: unknown` with `not_read` and identical timing figures.
- **Configuration tests.** Valid `stats.prices` loads. A model missing one price, or with a negative price, is refused as `config_invalid` with a next action. A config without `stats` round-trips without gaining the key.
- **Command-level integration tests in `cmd`**, through `runRootCmd`/`resetRootCmd`, over 000075's seeded fixture project plus a fixture sessions directory passed with `--sessions-dir`. They cover:
  - `stats run` showing tokens, cost and time per token in JSON and pretty form;
  - `compare` and `summary` carrying the new figures;
  - a run by another agent shown as unknown with its reason;
  - a removed session shown as unknown with identical timings;
  - prices taken from the project's `config.yaml`;
  - every log file and session file byte-identical before and after each subcommand.
- **Docs repo.** The site build and `astro check` pass, and the MDX guard returns nothing.

**Load-bearing guarantees.**
- Every token is placed exactly once: steps plus outside equal the total.
- A helper's tokens follow its launch, never the clock.
- Cost is never zero for unpriced tokens, and an incomplete total says so.
- Missing or unrecognised session data is a reason, never an error, and never changes a timing.
- No existing JSON figure changes.
- Neither logs nor agent files are ever written.

**Deliberate gaps.**
- No test runs a real Claude Code session: the format is pinned by fixtures copied in shape from real transcripts, and a real run is a manual check.
- No automated performance assertion. A Go benchmark parses a generated 10 MB session as supporting evidence.

**Success metrics.**
- *For a real implement run in a Claude Code session that drove only that run, the report's run total, including tokens outside any step, matches Claude Code's own usage totals for the session within 1%.* **Manual — captured in the implementation test plan.** Supporting behavioural test: in a fixture session that drove one run and holds Claude Code's totals, the run total equals those totals for every token type, the session overhead is the difference, and steps plus outside equal the total.
- *The maintainer can rank a run's steps by time per thousand output tokens and identify steps that are slow for the work they did.* **Manual — captured in the implementation test plan.** Supporting behavioural test: a fixture run with a long step that did much work and a long step that did little gives each step's figure, the second clearly higher, in both JSON and the Steps table.

**Manual reviews.**
- **Manual — captured in the implementation test plan:** run `stats run`, `compare` and `summary` with the built binary against this machine's real logs and Claude Code sessions, a plan run with helpers and an implement run with tasks. Confirm the token, cost and time-per-token figures are plausible and the tables readable.
- **Manual — captured in the implementation test plan:** read the Run statistics token sections and the `stats` configuration entry on the built site against the shipped behaviour.

## Milestones & Tasks

### Milestone 1: One run's report shows the tokens, cost and pace of each part beside its timings

**What changes**: For a run driven by Claude Code, `spektacular stats run` gives the run's tokens next to its timings, found from the agent's own session files without the agent doing anything extra. It gives:
- the input, output, cache-read and cache-write tokens for the run in total, for each step, each plan task and each helper agent, and for the tokens used outside any step;
- the models used, with the tokens each used;
- what it cost, priced from an optional price table in the project's configuration;
- working time per thousand output tokens, so a long step that did a lot of work can be told from a long step that did little.

A helper's tokens stay with the step that launched it, even when it finishes later. An unpriced model's cost is unknown, never zero, and a total that includes one is marked incomplete and names it. Runs driven by other agents, or whose session cannot be found or read, show token figures as unknown with the reason, and their timings are unchanged. Reading the session files changes nothing, and every existing figure of the report stays exactly as it was.

**Validation point**: The session reader, attribution, configuration, contract and `cmd` tests pass in the full shuffled Go suite. They show tokens adding up across steps, helpers staying on their launch, tasks carrying tokens, cost following the price table, unknown reasons, the additivity check against 000075's golden files, and byte-identical log and session files. Running the built binary against a fixture logs and sessions directory shows the token columns and blocks.

#### - [x] Task: Read Claude Code sessions
**Id:** b7d6cb42-9b9d-4ffb-8941-1c93a17c2558
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds a small, self-contained reader for Claude Code's own session files. It finds the sessions that were active at or after a given time and reads each one, and its helper agents' files, without changing anything. It turns them into plain data: each model request with its time, model and tokens; each shell command with when it started and finished; each helper launch with its brief; and Claude Code's own usage totals for the session when it has written them. When a folder is missing, a file cannot be read or nothing in it is recognised, it gives a reason instead of failing.

*Technical detail:* [context.md#task-read-claude-code-sessions](./context.md#task-read-claude-code-sessions)

**Acceptance criteria**:
- [x] A response written over several records counts once, with its tokens, its model and the time of its first record.
- [x] Each shell command the agent ran is read with its command text, the time it was issued and the time its result came back.
- [x] Each helper agent's transcript is linked to the launch that started it, and a helper started by another helper is linked to that helper.
- [x] Claude Code's own per-model totals for a session are read when present, and reported as absent otherwise.
- [x] Only sessions with activity at or after the given time are read, including a session whose only recent activity is in its helpers' files.
- [x] A missing sessions folder, an unreadable file and a file with no recognisable records each give their own reason, and damaged lines are skipped and counted.
- [x] Every session file is byte-identical before and after reading.

#### - [x] Task: Price table in the project configuration
**Id:** a1cd3849-cb71-4659-bf32-9286976b50c7
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Lets a project give model prices in its configuration, as an optional `stats.prices` table of input, output, cache-read and cache-write prices per million tokens for each model. No prices are built in, `init` does not write the table, and projects without it load exactly as before. A table with a missing or negative price is refused with a message naming the key to fix.

*Technical detail:* [context.md#task-price-table-in-the-project-configuration](./context.md#task-price-table-in-the-project-configuration)

**Acceptance criteria**:
- [x] A configuration with prices for two models loads with every price as written.
- [x] A configuration without a price table loads as before, and saving it does not add one.
- [x] A model missing one of its four prices, or with a negative price, is refused with a message naming the model and the key to set.
- [x] The settings format version is unchanged.

#### - [x] Task: Place tokens on steps, tasks and helpers
**Id:** f0791ec4-1820-4ab8-a498-3d7725b5da27
**Repo:** spektacular
**Depends on:**
- b7d6cb42-9b9d-4ffb-8941-1c93a17c2558 — Read Claude Code sessions
**Execution:** agent

Adds the rules that turn a run and its agent sessions into token figures, as pure calculations inside the run statistics package. They do the following:
- find the transcripts that drove the run from the run's own workflow commands;
- place each main-agent request in the step and task running when it was made, or outside any step;
- give each helper's tokens to its launch and to the step and task it was launched in;
- add the requests Claude Code counted but never wrote down, as session overhead, when the session drove only this run;
- price the tokens and work out working time per thousand output tokens.

When nothing usable is found, the result says why.

*Technical detail:* [context.md#task-place-tokens-on-steps-tasks-and-helpers](./context.md#task-place-tokens-on-steps-tasks-and-helpers)

**Acceptance criteria**:
- [x] For a run driven by one session, every token is placed exactly once, and the step figures plus the tokens outside any step equal the run total for each token type.
- [x] A helper launched in one step that finishes after the next step began has all its tokens on its launch and in the launching step, not the next.
- [x] Each task of a two-task implement run has its own tokens and cost.
- [x] Helpers are matched to the helper launches the run recorded, by the id in their brief, or in launch order when the brief has none. A helper that matches no recorded launch is still counted, and listed as unmatched.
- [x] A run resumed in a second session counts both sessions' requests, and a session that drove two runs splits its requests between them.
- [x] With prices for both models, cost equals each type's tokens times its price, summed. With one of two models unpriced, cost covers the priced model only, is marked incomplete and names the other.
- [x] Working time per thousand output tokens is given for the run, each step, each task and each helper. A helper's figure uses only its own time and tokens, and a part with no output tokens shows not applicable.
- [x] When the session drove only this run and holds Claude Code's own totals, the run total equals those totals. Otherwise the overhead is unknown with a reason.
- [x] A run driven by another agent, or with no matching session, gives unknown token figures with the reason.

#### - [x] Task: Report and print token figures
**Id:** e5b929cc-c2e0-434c-8518-ac874da67f3e
**Repo:** spektacular
**Depends on:**
- f0791ec4-1820-4ab8-a498-3d7725b5da27 — Place tokens on steps, tasks and helpers
**Execution:** agent

Puts the token figures into the run report. Each step, task and helper row gains its tokens, cost and time per thousand output tokens. A new token section gives the session, the run total, the tokens outside any step, the session overhead, the models used and any unmatched helpers. The readable table form shows the same figures beside the timings. Every existing figure, field name and unit stays exactly as it was, and a check against the report's earlier contract proves it.

*Technical detail:* [context.md#task-report-and-print-token-figures](./context.md#task-report-and-print-token-figures)

**Acceptance criteria**:
- [x] The structured report gives input, output, cache-read and cache-write tokens, cost and time per thousand output tokens for the run, each step, each task and each helper, with the field names in the plan's Data Structures section.
- [x] The report names every model used with the tokens each used.
- [x] With the new token fields removed, the structured report is identical to the report before this change.
- [x] The table form shows output tokens, cost and time per thousand output tokens beside each step's, task's and helper's timings, plus a token breakdown, the models and any unmatched helpers.
- [x] When token use is unknown, both forms say so with the reason, and every timing is unchanged.

#### - [x] Task: Token figures from the command line
**Id:** bc066b45-20f2-41ed-897c-798fb1a645f8
**Repo:** spektacular
**Depends on:**
- b7d6cb42-9b9d-4ffb-8941-1c93a17c2558 — Read Claude Code sessions
- a1cd3849-cb71-4659-bf32-9286976b50c7 — Price table in the project configuration
- e5b929cc-c2e0-434c-8518-ac874da67f3e — Report and print token figures
**Execution:** agent

Makes `spektacular stats run` read token use for the runs it reports. It finds the Claude Code sessions folder, or the one given with a new `--sessions-dir` option, and reads each session once. It takes prices from the project's price table and passes both to the report. Nothing about the agent's work has to change for this. Command tests never look at the developer's own sessions.

*Technical detail:* [context.md#task-token-figures-from-the-command-line](./context.md#task-token-figures-from-the-command-line)

**Acceptance criteria**:
- [x] For a run driven by Claude Code with no change to how the agent works, `stats run` gives token, cost and time-per-token figures in both table and JSON form.
- [x] Costs use the prices in the project's configuration. Outside a project, or with no price table, every cost is unknown with the reason.
- [x] A run driven by another agent shows unknown token figures with the reason.
- [x] With the agent's session removed, token figures are unknown with a reason, and the timings match the report without token data.
- [x] Every log file and agent session file is byte-identical before and after `stats run`.
- [x] The existing `stats` tests still pass, without reading any real Claude Code sessions.

### Milestone 2: Comparisons and summaries weigh runs by tokens and cost as well as time

**What changes**: `spektacular stats compare` shows each run's tokens, cost and working time per thousand output tokens, in total and for each step, beside the timings it already compares, with each run's difference from the first. `spektacular stats summary` gives the same figures' median and 90th percentile across runs, beside the timings. A run whose token use is unknown shows as absent for those figures, never as zero. So a run that got slower only because it did more work stands apart from one that got slower doing the same work.

**Validation point**: Comparison and summary tests show the six new figures for totals and steps, with deltas, percentages, medians and 90th percentiles, and `null` for unknown runs. The additivity check still passes for compare and summary, and `cmd` tests show both commands in both formats with the new figures. The full Go suite passes.

#### - [x] Task: Comparisons and summaries carry tokens
**Id:** 9a1effc0-c05f-43f4-903f-c801e4887ef2
**Repo:** spektacular
**Depends on:**
- e5b929cc-c2e0-434c-8518-ac874da67f3e — Report and print token figures
- bc066b45-20f2-41ed-897c-798fb1a645f8 — Token figures from the command line
**Execution:** agent

Adds tokens, cost and working time per thousand output tokens to `stats compare` and `stats summary`, for totals and for each step. They come after the figures those commands already give and in the same shape. A comparison shows each run's value and its difference from the first run. A summary gives the median and 90th percentile. A run whose token use is unknown is absent for these figures, never zero.

*Technical detail:* [context.md#task-comparisons-and-summaries-carry-tokens](./context.md#task-comparisons-and-summaries-carry-tokens)

**Acceptance criteria**:
- [x] Comparing two runs shows each step's tokens, cost and time per thousand output tokens for both runs, with the second run's difference in amount and percentage, and the same for the totals.
- [x] Summarising five runs gives the median and 90th percentile of tokens, cost and time per thousand output tokens, for each step and the totals, beside the timings.
- [x] A run with unknown token use shows as absent for the new figures, and the timings are unaffected.
- [x] With the new figures removed, comparison and summary output is identical to before this change.
- [x] Every log file and agent session file is byte-identical before and after `stats compare` and `stats summary`.

### Milestone 3: Token figures are documented

**What changes**: The documentation site's Run statistics page explains the token, cost and time-per-token figures, which agents are supported, how a run is matched to its agent session, and how unknown and incomplete values appear. The configuration reference describes the `stats.prices` table with an example. The README notes that reports include tokens and cost for Claude Code runs.

**Validation point**: The site builds and `astro check` passes, with no layout HTML in page bodies and no em dashes in the new text. The README shows the note.

#### - [x] Task: Documentation site describes token figures
**Id:** a1a9901f-b9c4-468a-9cea-0a5ba5d50a6e
**Repo:** docs
**Depends on:**
- bc066b45-20f2-41ed-897c-798fb1a645f8 — Token figures from the command line
- 9a1effc0-c05f-43f4-903f-c801e4887ef2 — Comparisons and summaries carry tokens
**Execution:** agent

Adds token and cost sections to the Run statistics page. They explain which agents are supported, how a run is matched to its agent session, what each token, cost and time-per-token figure means, how to set prices, and how unknown and incomplete values appear. The configuration reference gains the `stats` key with an example price table.

**Content outline** (new sections in `run-stats.mdx`, inserted after "Unfinished work and damaged lines" and before "Reports for programs"; the wording is illustrative, the keys, field names, flags and reason codes are fixed):
1. `## Tokens and cost`: for runs driven by Claude Code, every report also shows how many tokens each part used, what that cost, and how much time it took per thousand output tokens, beside its timings. Example block (Steps table excerpt):
   ```text
   STEP          ENTRIES  DURATION  ...  OUT TOKENS  COST    TIME/1K OUT
   discovery     1        12m04s    ...  48,210      3.12    15.0s
   architecture  1        9m40s     ...  2,105       0.41    4m35s
   ```
   One sentence: "architecture took almost as long as discovery but did a twentieth of the work, so it is the step to look at."
2. `## How a run finds its session`: Spektacular never sees the agent's model requests. It reads Claude Code's own session files in `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR/projects`), finds the session whose shell commands ran this run's `new` and `goto` commands, and reads its helper agents too. The agent does nothing extra. `--sessions-dir <path>` points at another folder. Reading never changes those files.
3. `## What the token figures mean`: a definition list.
   - **Input, output, cache read, cache write**: the four token types the model reports.
   - **Steps and tasks**: each request counts in the step and task running when it was made.
   - **Helpers**: a helper's tokens count on its launch and in the step and task it was launched in, even if it finishes later. A helper with no recorded launch is listed as unmatched.
   - **Outside any step**: requests made before the run's first step, between steps of an interrupted run, or after it finished, in the same session.
   - **Session overhead**: tokens Claude Code counted for the session that its transcript never records (for example permission checks). Known only when the session drove just this run and Claude Code has written its totals. It is part of "outside any step".
   - **Models**: each model used, with its tokens.
   - **Time per 1k output tokens**: working time (wall time less user wait; a helper's own duration) per thousand output tokens. Shown as `n/a` with no output tokens.
4. `## Setting prices`: no prices are built in. Example block:
   ```yaml
   stats:
     prices:                  # per million tokens
       claude-opus-5-5:
         input: 5.00
         output: 25.00
         cache_read: 0.50
         cache_write: 6.25
   ```
   Keys are model names exactly as the report lists them. A model with no price has unknown cost, never zero. A total that includes one shows the priced part, marked incomplete with the unpriced models named (`cost_complete: false`, `unpriced_models`).
5. `## When tokens are unknown`: token figures show as unknown with a reason, and timings are unchanged. Reasons: `agent_unsupported` (only Claude Code is read today; Bob, Codex and others show unknown), `sessions_dir_missing`, `session_not_found`, `session_unreadable`, `format_unrecognised`. Comparisons show such a run as `-` for the token figures.
6. In `## Reports for programs`, an added example excerpt: `"tokens":{"status":"known","total":{"input":1442,"output":641466,"cache_read":158466888,"cache_write":2554795,"cost":43.18,"cost_complete":true,"unpriced_models":[],"working_ms_per_1k_output":2811}}`, and one sentence: comparisons and summaries add `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `cost` and `working_ms_per_1k_output` after the existing figures.

**Content example** (configuration page): a `stats` `ConfigKey` after `log`, with default "none", whose body reads: "Optional prices for the token costs `spektacular stats` reports. `stats.prices.<model>.input`, `.output`, `.cache_read`, `.cache_write`: the price per million tokens of each type, all four required. No prices are built in. See [Run statistics](/run-stats/)." The example `config.yaml` gains the same `stats.prices` block after `log`, and the top-level key count and list gain `stats`.

*Technical detail:* [context.md#task-documentation-site-describes-token-figures](./context.md#task-documentation-site-describes-token-figures)

**Acceptance criteria**:
- [x] The Run statistics page describes the token, cost and time-per-token figures, how a run is matched to its session, the price table, the supported agents and the unknown reasons.
- [x] The configuration reference describes `stats.prices` with an example, and the top-level key list includes `stats`.
- [x] The site builds and type-checks without errors, and the page bodies have no layout HTML and no em dashes.

#### - [x] Task: README notes token figures
**Id:** 67c0ae0d-c369-439c-937d-6e4373ceaa03
**Repo:** spektacular
**Depends on:**
- bc066b45-20f2-41ed-897c-798fb1a645f8 — Token figures from the command line
- 9a1effc0-c05f-43f4-903f-c801e4887ef2 — Comparisons and summaries carry tokens
**Execution:** agent

Adds one short paragraph to the README's `stats` block. It says that, for runs driven by Claude Code, reports also give tokens, cost and time per thousand output tokens, found from the agent's own session files with no extra setup, and that costs come from the optional `stats.prices` table.

**Content example** (after the `stats` code block):
```text
For runs driven by Claude Code, `stats run`, `compare` and `summary` also show tokens, cost and
time per thousand output tokens for each step, task and helper, read from Claude Code's own
session files. Set prices per million tokens under `stats.prices` in `.spektacular/config.yaml`;
none are built in. Other agents show these figures as unknown.
```

*Technical detail:* [context.md#task-readme-notes-token-figures](./context.md#task-readme-notes-token-figures)

**Acceptance criteria**:
- [x] The README says that reports include tokens, cost and time per thousand output tokens for Claude Code runs, that prices come from `stats.prices`, and that other agents show unknown.

## Open Questions

- **Whether 000075 (and through it 000073 and 000074) lands with the names, rows and figure lists this plan extends.** This depends on their implementations. The plan relies on:
  - `Options`, `Report` and its step, task and helper rows;
  - `Figure` and `Stat`, and the figure order lists;
  - the golden files;
  - `statsLogs`;
  - command records' `process.command_args` and times;
  - `sub-agent` spans' `spektacular.step` and their composite id in helper briefs.

  If only names differ, adapt to them. If a figure's meaning, the JSON contract or the span rules differ from their plans, STOP and ask the user, because the token placement rules and the additivity guarantee rest on them.
- **Whether Claude Code's `cost-state` record covers the whole session file it sits in.** This depends on Claude Code's behaviour when a session is resumed or continued, which can only be seen on real sessions once the reader exists. The plan reports overhead as unknown whenever totals fall below the transcript. If the manual success-metric check finds that a single-run session's run total still misses Claude Code's totals by more than 1%, STOP and ask the user before changing how overhead is computed.

No other implementation-time uncertainties remain. Every design choice is settled in Architecture & Design Decisions, Data Structures and the assumption log.

## Out of Scope

- **Tokens or cost in the HTML log viewer** (spec Non-Goals). Spec `000077_html-log-viewer` keeps its pages free of token figures. `BuildReport` stays source-compatible with it, and nothing on its page changes.
- **Changing Harbor's own token reading** (spec Non-Goals). Harbor keeps reading tokens, models and cost from its agents' transcripts (`000076_harbor-on-built-in-logs` pins those figures). It ignores the new JSON fields.
- **Budgets, alerts or limits on token use** (spec Non-Goals).
- **Token use for agents other than Claude Code.** Bob, Codex, Copilot and OMP runs show token figures as unknown with `agent_unsupported`. The reader's neutral types leave room for another agent's reader in a later spec.
- **Claude Code's headless `stream-json` output** (Harbor's `claude-code.txt`). Only the on-disk session transcripts are read.
- **A hook installed by `init` to record the session.** The spec's fallback is not built, because matching on the run's own commands works without agent action (flagged for review).
- **Built-in or fetched model prices.** Prices come only from `stats.prices` (spec Constraints). There is no currency conversion: costs are in whatever currency the table uses.
- **Separate 5-minute and 1-hour cache-write prices.** Cache writes are one type with one price.
- **Model time per step, and the cost of a request's thinking tokens apart from its output.** Only the four token types are reported.
- **Token figures per step entry and per user wait.** Steps are reported by name, as comparisons key them, and user waits make no requests.
- **Token figures for standalone traces**, such as an epic orchestrator's own session. Only workflow runs are reported, as in 000075.
- **A `--no-tokens` switch or a cache of parsed sessions.** Each report reads the candidate sessions once, read-only. Nothing is written beside the logs or the sessions.
- **Installing the new binary or touching this repository's own `.spektacular/`** (plans-never-change-the-active-install).

## Changelog

### 2026-10-25 — Task: Read Claude Code sessions

**What was done**: Added the leaf package `internal/agentusage`, which finds Claude Code sessions active at or after a given time (by the latest modification time of the main file and every helper file) and reads each one read-only into plain data: de-duplicated model requests (first-record time, model, field-wise largest usage), Bash calls with their result times, Agent/Task launches with their brief, helper links from `agent-<id>.meta.json`, and Claude Code's own `cost-state` totals. Problems come back as `sessions_dir_missing`, `session_unreadable` or `format_unrecognised`, never as errors.

**Deviations**: `readTranscript` and `readSession` return a small internal struct and a recognised-record count instead of the four-value signature sketched in context.md, so the "no recognised records" problem can be told apart from an empty transcript. Responses from Claude Code's `<synthetic>` model with no tokens are dropped, so they never appear as a model. An unreadable helper transcript is left out with its own `session_unreadable` problem while the rest of the session is still read. `Usage` also gained `Sub` and `IsZero` for the overhead step.

**Files changed**:
- `spektacular: internal/agentusage/doc.go`
- `spektacular: internal/agentusage/types.go`
- `spektacular: internal/agentusage/claude.go`
- `spektacular: internal/agentusage/transcript.go`
- `spektacular: internal/agentusage/meta.go`
- `spektacular: internal/agentusage/session.go`
- `spektacular: internal/agentusage/claude_test.go`
- `spektacular: internal/agentusage/transcript_test.go`
- `spektacular: internal/agentusage/bench_test.go`
- `spektacular: internal/agentusage/testdata/claude/-tmp-proj/` (fixture sessions s1 with two helpers, s2 unrecognised, s3 with no trailing newline)

**Discoveries**: A user record's `message.content` is sometimes a plain string rather than a block list, so content is decoded as raw JSON and only lists are walked. Filtering lines by the unescaped substring `"type":"assistant"` before decoding is safe, because the same text inside a string value is escaped (`\"type\":\"assistant\"`). On this machine's real sessions the reader parses a 26-transcript session in well under a second; the benchmark parses ~10 MB in about 40 ms.

### 2026-10-25 — Task: Price table in the project configuration

**What was done**: `config.Config` gained an optional `stats` section (`StatsConfig{Prices map[string]ModelPrice}`, each price a `*float64` for input, output, cache_read and cache_write per million tokens), dropped from the saved file when empty through `IsZero`. `StatsConfig.Validate`, called from `Config.Validate`, refuses an unnamed model, a missing price or a negative price as `config_invalid` with a next action naming `stats.prices.<model>.<key>`.

**Deviations**: None. (Placed after the `log` section 000073 added; schema stays 5.)

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/config_test.go`

**Discoveries**: None.

### 2026-10-25 — Task: Place tokens on steps, tasks and helpers

**What was done**: Added pure token attribution to `internal/runstats`: drivers are the transcripts whose Bash calls ran the run's own `<kind> new`/`<kind> goto` commands (±1 s); a transcript that drove several runs splits its moments by the between-runs rule; main-agent requests go to the step entry and task running at their time or outside any step; helper transcripts (with descendants rolled in) are matched to `sub-agent` spans by the span id in their brief, then by launch order, else listed as unmatched; session overhead is Claude Code's `cost-state` totals less every transcript request when the session drove only this run; `figure` prices tokens and computes working time per thousand output tokens. `buildTokens(run, rep, in)` returns the section and per-row figures (wired into `BuildReport` by the next task).

**Deviations**:
- The token JSON types (`TokenFigure`, `TokenSection`, `ModelTokens`, `UnmatchedHelper`, `SessionRef`) were written here in `tokens_types.go`, because `buildTokens` returns them; context.md placed that file in the next task.
- `TokenInput` gained `Dir` (the sessions directory, named in the `session_not_found` message) and an unexported per-transcript memo of the runs each drove.
- Matching adds one guard: a shell call whose `--data` names a workflow (`"name":"X"`) other than the run's is not that run's, so concurrent orchestrated children running `implement goto` at the same moment are not confused.
- Launch-order fallback pairs a response's concurrent launches with the latest unmatched spans in start order (earliest launch to earliest span), rather than each launch taking the single latest span.
- With no sessions read at all and a reader problem, the reason is that problem's own code (`sessions_dir_missing`, `session_unreadable` or `format_unrecognised`); `session_not_found` is used when sessions were read but none drove the run.
- The test author found that a transcript driving two runs was counted as owned for overhead; fixed so any transcript that drove runs must have driven only this one (`TestTokensOverheadOneTranscriptTwoRuns`).
- Codebase research for this High task was done in the main context (the report, assembly and listing code were already read in `read_plan`); the tests were written by a sub-agent.

**Files changed**:
- `spektacular: internal/runstats/prices.go`
- `spektacular: internal/runstats/tokens_types.go`
- `spektacular: internal/runstats/tokens.go`
- `spektacular: internal/runstats/tokens_match.go`
- `spektacular: internal/runstats/tokens_place.go`
- `spektacular: internal/runstats/tokens_helpers.go`
- `spektacular: internal/runstats/tokens_overhead.go`
- `spektacular: internal/runstats/prices_test.go`
- `spektacular: internal/runstats/tokens_test.go`
- `spektacular: internal/runstats/tokens_match_test.go`
- `spektacular: internal/runstats/tokens_place_test.go`
- `spektacular: internal/runstats/tokens_helpers_test.go`
- `spektacular: internal/runstats/tokens_overhead_test.go`

**Discoveries**: On this machine's real logs and sessions, every recent orchestrated implement child is matched to its own helper transcript inside the orchestrator's session, and its helpers to their spans. Overhead is `several_runs` for orchestrated children, because the orchestrator's session holds other runs. A helper span started without `--parent` after a step was re-entered lands in another trace, and that helper is then listed as unmatched; this is correct behaviour, not a matching fault.

### 2026-10-25 — Task: Report and print token figures

**What was done**: `BuildReport` now calls `buildTokens` at its end and fills `Report.Tokens` (the top-level section) and a `tokens` figure on every step, task and helper row; `Options` gained `Tokens *TokenInput` (nil gives `not_read`). The text report adds OUT TOKENS, COST and TIME/1K OUT columns to the Steps, Tasks and Helpers tables, a Tokens block (per-step four types and cost, outside any step with the session overhead in it, total), a Models table, Unmatched helpers when there are any, and a footnote naming unpriced models; unknown token use prints `unknown (<code>): <reason>` and `?` cells. 000075's and 000076's golden JSON files were copied to `testdata/pre-tokens/` before any type changed, and `TestTokenFieldsAreAdditive` proves the regenerated files differ only by the new keys.

**Deviations**:
- 000075 has no `HelperRow` type: helper launches and user waits shared `Wait`. A new `HelperRow{Wait; Tokens}` (embedded, so the JSON stays flat with `tokens` appended) is the type of `Report.Helpers`; `user_waits` keep `[]Wait` with no tokens.
- The golden run's `implement new`/`goto` commands gained `process.command_args`, and the golden report and text are now built with a fixture Claude session (`goldenSession`, opus priced, haiku not, with `cost-state` totals), so the golden files pin known figures, including session overhead and an incomplete cost.
- Test files outside the goal's list needed mechanical updates for the new row type and columns: `internal/runstats/report_test.go` (key sets, `HelperRow`), `internal/runstats/listings_test.go` (`rep.Helpers[0].Wait`) and `cmd/stats_test.go` (three row regexes now end with the three token columns).

**Files changed**:
- `spektacular: internal/runstats/report.go`
- `spektacular: internal/runstats/render.go`
- `spektacular: internal/runstats/tokens_helpers.go`
- `spektacular: internal/runstats/contract_test.go`
- `spektacular: internal/runstats/report_test.go`
- `spektacular: internal/runstats/render_test.go`
- `spektacular: internal/runstats/listings_test.go`
- `spektacular: cmd/stats_test.go`
- `spektacular: internal/runstats/testdata/report.golden.json`
- `spektacular: internal/runstats/testdata/report.golden.txt`
- `spektacular: internal/runstats/testdata/pre-tokens/` (report, compare, summary, commands, spans golden JSON as before token use)

**Discoveries**: Harbor's Python self-checks read `internal/runstats/testdata/report.golden.json` directly (`tests/harbor/shared/tests/test_metrics.py`, `test_runlog.py`), so any golden change must be followed by `uv run --with pytest pytest tests/harbor/shared/tests`; they pass with the added `tokens` keys.

### 2026-10-25 — Task: Token figures from the command line

**What was done**: The `stats` group gained a persistent `--sessions-dir` flag. `stats run` now builds its report with `statsTokenInput`, which resolves the sessions directory (`--sessions-dir` made absolute, else `$CLAUDE_CONFIG_DIR/projects`, else `~/.claude/projects`), reads the Claude Code sessions active since the earliest selected run started (only when a selected run's agent is `claude` or unset), takes `stats.prices` from the project's config when there is a project, and passes every run in the logs for the between-runs rule. `--schema` lists `tokens` and `sessions-dir`, and the group's help names the session folders and the price table. `cmd`'s `TestMain` points `CLAUDE_CONFIG_DIR` at an empty temp directory, so no test reads the developer's sessions.

**Deviations**:
- "Every cost is unknown with the reason": a cost with no price is `null` with `cost_complete: false` and `unpriced_models` naming the models (the JSON reason). The table's footnote now also says where prices go: "incomplete, no price for <models> (prices per million tokens go under stats.prices in .spektacular/config.yaml)" (a one-line change to `render.go` and its golden text).
- The cmd fixtures' command records carried no `process.command_args`; the token tests use their own logs (`seedTokenLogs`, the spec and implement runs with arguments) so the existing listing tests' output is unchanged.
- The first reviewer reply carried a note after its JSON and was refused as `goal_reply_invalid`; the reviewer was asked again with the same input.

**Files changed**:
- `spektacular: cmd/stats.go`
- `spektacular: cmd/main_test.go`
- `spektacular: cmd/stats_test.go`
- `spektacular: internal/runstats/render.go`
- `spektacular: internal/runstats/contract_test.go`
- `spektacular: internal/runstats/testdata/report.golden.txt`

**Discoveries**: `resetRootCmd` already resets persistent string flags, so `--sessions-dir` needed no reset code; `TestStatsRunNeverReadsTheDevelopersSessions` proves it.

### 2026-10-25 — Task: Comparisons and summaries carry tokens

**What was done**: The comparison and summary figure lists gained `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `cost` and `working_ms_per_1k_output`, appended after 000075's figures for the totals and every step, read from each run's report (null for a run with unknown token use, a null cost or a null pace). `Figure.Values` and `Figure.Delta` widened from `[]*int64` to `[]*float64` (whole numbers encode as before; a cost delta is rounded to 6 decimals). `stats compare` and `stats summary` build their token input like `stats run`, and their `--schema` lists the new figures and `--sessions-dir`. The tables print tokens with thousands separators, costs with 2 decimals and pace as a time.

**Deviations**:
- Rather than extra columns in the comparison's and summary's step table (one column per run already), each gets three more step tables, "Steps (output tokens)", "Steps (cost)" and "Steps (time per 1k output tokens)", shown only when some run's token use is known; the totals table lists all six new figures.
- `isDuration` now also treats `working_ms_per_1k_output` as milliseconds.
- The comparison and summary golden files are now built with fixture sessions (`withTokens`, `sessionFor` in `compare_test.go`); `seqRun`'s commands gained their `process.command_args`.
- Two existing `cmd` tests were updated for the appended figures (the figure order list, and the summary's per-figure count, which is 0 for token figures when no session drove the runs).

**Files changed**:
- `spektacular: internal/runstats/compare.go`
- `spektacular: internal/runstats/summary.go`
- `spektacular: internal/runstats/render.go`
- `spektacular: internal/runstats/compare_test.go`
- `spektacular: internal/runstats/summary_test.go`
- `spektacular: internal/runstats/testdata/compare.golden.json`
- `spektacular: internal/runstats/testdata/compare.golden.txt`
- `spektacular: internal/runstats/testdata/summary.golden.json`
- `spektacular: internal/runstats/testdata/summary.golden.txt`
- `spektacular: cmd/stats.go`
- `spektacular: cmd/stats_test.go`

**Discoveries**: None.

### 2026-10-25 — Task: Documentation site describes token figures

**What was done**: The Run statistics page gained "Tokens and cost", "How a run finds its session", "What the token figures mean", "Setting prices" and "When tokens are unknown" sections between "Unfinished work and damaged lines" and "Reports for programs", and "Reports for programs" gained a `tokens` JSON excerpt and the six appended comparison and summary figures; its description mentions tokens and cost. The configuration reference gained a `stats` `ConfigKey` after `log`, a `stats.prices` block in the example `config.yaml`, `stats` in the front-matter description, and "Eighteen top-level keys" with `stats` in the list.

**Deviations**: The pages also describe behaviour settled during implementation: a resumed run read from both sessions, an orchestrated run found in its own helper transcript, the `*` incomplete-cost mark and its note, and summaries leaving an unknown run out of their counts.

**Files changed**:
- `docs: src/pages/run-stats.mdx`
- `docs: src/pages/configuration.mdx`

**Discoveries**: None.

### 2026-10-25 — Task: README notes token figures

**What was done**: The README's Run statistics block gained the paragraph, straight after the `stats` code block, saying that for Claude Code runs `stats run`, `compare` and `summary` also show tokens, cost and time per thousand output tokens for each step, task and helper, read from Claude Code's own session files with no extra setup, that prices go under `stats.prices`, and that other agents show unknown.

**Deviations**: The paragraph also names `--sessions-dir`. The block's lead-in said reports work "without reading any agent transcript", which the token figures now do; it was changed to "the timings need no agent transcript", so the README does not contradict itself.

**Files changed**:
- `spektacular: README.md`

**Discoveries**: None.
