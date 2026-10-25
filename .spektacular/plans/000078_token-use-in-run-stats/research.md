---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Research: 000078_token-use-in-run-stats

## Alternatives considered and rejected

- **A hook that `init` installs to write the session's transcript path into the run log.** The spec names this as the fallback. Rejected as unnecessary: every Claude Code session already records each `Bash` tool call with its command text and timestamps, and each tool result with its own timestamp (verified on real transcripts under `~/.claude/projects/-home-nicj-code-github-com-hivecommons-spektacular/`, e.g. `bc997d3b-…jsonl`: `tool_use` `{"name":"Bash","input":{"command":"spektacular implement new --data …"}}` at `2026-10-24T08:24:33.124Z`, result at `08:24:33.866Z`). Matching on these needs no agent action and works for sessions started before upgrading. A hook would change `init`'s installed agent settings (an agent-specific surface the plans-never-change-the-active-install rule makes awkward to verify) and would not help orchestrated children, whose transcripts are helper transcripts of the orchestrator's session.
- **Reading Claude Code's headless `stream-json` output (Harbor's `claude-code.txt`).** It exists only for `claude -p` runs, not interactive sessions, and Harbor already reads it (`tests/harbor/shared/transcripts.py:286-334`). Changing Harbor's token reading is a spec Non-Goal. Rejected.
- **Using only Claude Code's `cost-state` record (its session totals).** It has no timestamps per request, so no step, task or helper split. Rejected as the main source; kept only for the session-overhead figure (below).
- **Using only the transcript's per-request `usage`.** Measured on 12 real sessions on this machine, the transcript's de-duplicated requests (main plus every `subagents/agent-*.jsonl`) fall short of the session's own `cost-state.modelUsage` by 97–99% on uncached input, 4–35% on cache reads, 0.4–6.5% on cache writes and 0.2–64% on output, and miss small side models (haiku) entirely. Requests Claude Code makes outside the conversation (permission classifier, titles, summaries) are counted in `cost-state` but never written as `assistant` records. The success metric (run total within 1% of Claude Code's own totals) cannot be met from requests alone, so the run total adds the difference as session overhead when it can be known.
- **A separate `stats tokens` subcommand.** Rejected: the spec's constraint is that tokens sit beside timings, and comparisons and summaries must carry them in the same output.
- **Matching a run to a transcript by the run's name in command text.** Only `new` and orchestrated `goto` commands carry the name; a plain `goto --data '{"step":"…"}'` does not. Rejected in favour of time windows plus the command's own words (`<kind> new`, `<kind> goto`).
- **Matching helper transcripts to sub-agent spans by time alone.** Concurrent launches in one message make start times ambiguous. The primary key is the sub-agent span id 000074's instructions put in every helper's brief ("Your parent span is <id>"), found in the `Agent` tool call's `prompt`; launch order is only the fallback, as 000076 does for orchestrator children.
- **Built-in model prices.** Forbidden by the spec's constraints (prices change). Prices come only from an optional `stats.prices` table in `config.yaml`.
- **Prices under `log:`.** `log` configures recording (000073); prices only affect reporting. A new optional top-level `stats` key, like 000072's optional `knowledge.prune.max_age_days`, needs no settings-format bump and is not written by `init`.
- **Changing `runstats.Figure` to a new shape for cost.** Rejected: 000075's generic figure row must keep its shape. Only the Go element type of its value slices widens from integer to float so a fractional cost fits; integer figures still encode as JSON integers (Go's `encoding/json` prints float64 values below 1e21 without exponent or decimals), proven by a pre-token golden comparison.
- **Putting token reading in `internal/runstats` itself.** Rejected: `runstats` is pure analysis over the logs with I/O only at its loader. Agent session files live outside the logs directory and need their own read-only loader, so a new leaf package `internal/agentusage` reads them and `runstats` only attributes.

## Chosen approach — evidence

- Claude Code session layout (read-only inspection, `~/.claude/projects/<slug>/`): `<session>.jsonl`, `<session>/subagents/agent-<agentId>.jsonl` plus `agent-<agentId>.meta.json` with `toolUseId` (the launching `Agent` tool call's id), `description`, `agentType`, `spawnDepth` and, at depth 2, `parentAgentId` (676 helper transcripts surveyed, every one with a meta file; 417 at depth 1, 259 at depth 2).
- `assistant` records carry `timestamp`, `message.id`, `message.model` and `message.usage{input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens}`. One response is split over several records (one per content block) that repeat the same `message.id` and usage (679 records, 311 ids in `bc997d3b`); a few later records carry a larger usage (1–28 per session), so the reader keeps the maximum of each field per id.
- `user` records carry `tool_result` blocks with `tool_use_id` and their own `timestamp`, giving each `Bash` call an interval `[call record time, result record time]` that contains the command's execution.
- `cost-state` record: `{"type":"cost-state","sessionId":…,"modelUsage":{"<model>":{"inputTokens","outputTokens","cacheReadInputTokens","cacheCreationInputTokens","costUSD",…}}}`, written near the end of the session file (record 504 of 505 in `f138ad0d`; 2596 and 2598 of 2599 in `dced7868`). It includes helper usage (helper requests alone exceed the main transcript's).
- Every record has `cwd` and `sessionId`; the project directory name is the cwd with non-alphanumerics replaced by `-`, but the plan does not depend on it: candidate sessions are found by modification time across all project directories.
- 000073 plan, Data Structures: command records carry `process.command_args`, `start_time`, `end_time`, `spektacular.agent` at the default level.
- 000074 plan, Data Structures and trace partial: sub-agent spans with `spektacular.step`, composite id `<32hex>-<16hex>`, helper briefs carry the id.
- 000075 plan, Data Structures and Go interfaces: `Report`, `Options{Gaps, GapThreshold}`, `Figure{Figure, Values, Delta, DeltaPct}`, `Stat`, figure order lists, golden files `internal/runstats/testdata/{report,compare,summary}.golden.json`.
- `cmd/main_test.go:12-15`: a `TestMain` already exists in `cmd`, where the token reader's default directory can be pinned to an empty temp directory so no test reads the developer's home.
- `internal/config/config.go:479-512`: `ParseYAMLFile` decodes non-strictly, so an optional new key loads without a format bump; `config.go:191-212` shows the `config_invalid` + `WithNextAction` validation pattern.

## Files examined

- spektacular:cmd/main_test.go:12 — `TestMain` sets the build version; a place to pin the agent-session directory for every `cmd` test.
- spektacular:internal/config/config.go:376-404 — `Config` struct; new optional `Stats StatsConfig \`yaml:"stats,omitempty"\`` goes here.
- spektacular:internal/config/config.go:191-212 — `GoalCheckConfig.Validate`, the validation and next-action pattern to copy for prices.
- spektacular:internal/config/config.go:479-512 — non-strict YAML decode; unknown keys are ignored.
- spektacular:internal/config/schema.go:15-20 — `CurrentProjectSchema` (4 today, 5 after 000073); not changed here.
- spektacular:internal/output/writer.go:60 — `NewError(code, message)`.
- spektacular:tests/harbor/shared/transcripts.py:286-334 — Harbor's `claude_summary` (stream-json `result.modelUsage`), the reference for model and token field names; unchanged.
- spektacular:tests/harbor/shared/transcripts.py:83-107 — sub-agent launches keyed by the `Agent`/`Task` tool call id.
- docs:src/pages/configuration.mdx:32-96,103-110,342-356 — the example `config.yaml`, the "Seventeen top-level keys" sentence and the `ConfigKey` slot pattern; a `stats` key is added.
- docs:src/pages/ — no `run-stats.mdx` yet (000075 creates it).
- ~/.claude/projects/-home-nicj-code-github-com-hivecommons-spektacular/*.jsonl and */subagents/* (read-only) — the transcript format above.

## External references

- Claude Code on-disk session transcripts (`~/.claude/projects`, relocated by `CLAUDE_CONFIG_DIR`): undocumented JSON Lines format, grounded on version 2.1.309 records on this machine. Why it mattered: it is the only source of per-request token use.
- Go `encoding/json` float formatting (`strconv.FormatFloat` with `'f'` below 1e21): why widening figure values to float64 leaves integer figures' JSON unchanged.

## Prior plans / specs consulted

- `000075_run-stats-command` plan, context: the package layout, report JSON contract, figure order, golden files and `cmd/stats.go` shape this plan extends; its Out of Scope names tokens as a later spec.
- `000074_trace-spans-for-agent-work` plan: span types, `spektacular.step` on agent spans, the helper-brief span id, the composite id format.
- `000073_log-mode-and-logger-providers` plan: command record attributes (`process.command_args`, times, `spektacular.agent`), `config.LogDir`, the logging-exempt annotation the `stats` group already carries.
- `000076_harbor-on-built-in-logs` plan, context: also extends `cmd/stats.go`, `internal/runstats/render.go`, README and `run-stats.mdx`; keeps Harbor's transcript token reading; launch-order matching precedent for children.
- `000077_html-log-viewer` plan, context: reuses `BuildReport(r, Options{Gaps, GapThreshold})` and `Report.Totals`; so `Options` may only gain fields and `BuildReport` keeps its signature.
- Epic `000002` planning summary: settings format 4 → 5 is 000073's and the only bump; 000072's optional config key precedent; docs nav and `run-stats.mdx` ownership.

## Open assumptions

- 000075 lands with the Go names and JSON shapes in its plan (`Report`, `Options`, `Figure`, `Stat`, `StepRow`/`TaskRow`/`HelperRow` equivalents, figure order lists, golden files). If only names differ, adapt; if a figure's meaning differs, STOP and ask.
- Claude Code keeps the transcript format observed in version 2.1.309: `assistant` records with `message.id`, `message.model`, `message.usage`; `tool_use`/`tool_result` blocks; `subagents/agent-<id>.meta.json` with `toolUseId`/`parentAgentId`; `cost-state` with `modelUsage`. A changed format must give unknown token figures with a reason, never an error.
- The `cost-state` record holds the totals of the session file it is in, helpers included. If a resumed session's record turns out to cover only the latest process, the overhead may come out negative; the reader then reports overhead as unknown rather than subtracting.
- A `spektacular` command run by the agent appears in its transcript as a `Bash` tool call whose command text contains the command's first two words (`plan goto`), and the command record's start time falls inside that call's interval (with one second of tolerance).
- 000074's instructions keep putting the sub-agent span id in each helper's brief.

## Drafting assumptions

### Chosen direction: a leaf session reader plus pure attribution in runstats (architecture)
- **Decision**: New leaf package `internal/agentusage` reads Claude Code transcripts read-only; new pure files in `internal/runstats` match runs to transcripts, place requests and helpers, price them and build figures; `BuildReport` keeps its signature and `Options` gains one optional field; output is additive (a `tokens` object on step, task and helper rows, a top-level `tokens` section, six appended compare and summary figures).
- **Key design decisions**: drivers found by the run's own `new`/`goto` commands inside transcript shell calls (no hook); every matching transcript is a driver (resumed runs); between-run requests go to the next run, trailing ones to the last; helpers matched by the span id in their brief, then launch order, else listed unmatched; session overhead from Claude Code's `cost-state` folded into outside-steps when the session drove only this run; `Figure` value slices widen to float64 with JSON unchanged; tokens read by default for `claude` and unset agents.
- **Rationale**: Keeps `runstats` pure and table-testable, respects 000075's JSON contract and 000077's use of `BuildReport`, and needs no agent action.
- **Rejected**: separate `stats tokens` command; hook-based identification; reading tokens inside `runstats`' loader; see research.md alternatives.

### No init-installed hook (discovery)
- **Decision**: Runs are matched to Claude Code sessions only by finding the run's own `<kind> new` and `<kind> goto` commands among the transcripts' `Bash` tool calls; the hook fallback the spec mentions is not built.
- **Rationale**: The spec prefers the no-hook approach and names the hook only as a fallback; real transcripts carry every command with timestamps, so matching works without agent action, for orchestrated children too, and for sessions started before the upgrade.
- **Rejected**: Building the hook as well (changes `init`'s installed agent settings, agent-specific, unneeded). Flagged for review.

### Session overhead from Claude Code's own totals (discovery)
- **Decision**: When a session drove only this run and has a `cost-state` record, the difference between its totals and the transcript's requests is reported as session overhead, counted in the run total and in the tokens outside any step. Otherwise overhead is unknown with a reason and the totals cover transcript requests only.
- **Rationale**: Measured on 12 real sessions, transcript requests miss 97–99% of uncached input and up to 64% of output that Claude Code counts (permission classifier and other side requests). Without it the success metric cannot be met.
- **Rejected**: Ignoring the gap (metric unreachable, totals misleading); using `cost-state` alone (no step split).

### Candidate sessions by modification time (discovery)
- **Decision**: Candidate transcripts are every session under the Claude projects directory (`$CLAUDE_CONFIG_DIR/projects`, else `~/.claude/projects`, or `--sessions-dir`) whose main file or helper files were modified at or after the run's start.
- **Rationale**: Avoids depending on Claude Code's project-directory naming, and covers sessions started in another folder or a worktree.
- **Rejected**: Computing the project slug from the project root only (misses other start folders; undocumented naming).

### Tokens read for `claude` and unset agents only (architecture)
- **Decision**: A run whose `spektacular.agent` is `claude` or `unknown` is looked up in Claude Code sessions; any other agent gets unknown tokens with reason `agent_unsupported`.
- **Rationale**: Spec supports Claude Code only; a project with no `agent` set may still be driven by Claude.
- **Rejected**: Claude only when the agent is exactly `claude` (loses data for unset projects).

### Price table shape (architecture)
- **Decision**: Optional top-level `stats.prices` in `config.yaml`, keyed by the model name exactly as the report lists it, each with `input`, `output`, `cache_read` and `cache_write` prices per million tokens; all four required and non-negative; currency is whatever the table uses, so the JSON field is `cost` (no currency suffix). No settings-format bump; `init` does not write it.
- **Rationale**: Mirrors 000072's optional key precedent; per-million is how providers publish prices; requiring all four avoids a silently low cost.
- **Rejected**: Under `log:` (recording, not reporting); prefix or alias model matching (surprising); `cost_usd` (asserts a currency).

### Cache writes priced at one rate (architecture)
- **Decision**: Cache-write tokens are one type with one price, although Claude's usage splits them into 5-minute and 1-hour writes.
- **Rationale**: The spec names four types; Claude's own `cost-state` totals also report one cache-creation figure.
- **Rejected**: Two cache-write types (beyond the spec's four).

### Request time is the first record of its response (architecture)
- **Decision**: A main-agent request is placed at the timestamp of the first transcript record of its response (by `message.id`).
- **Rationale**: The response that issues a `goto` is written before the `goto` runs, so it lands in the step it was made in; the next request lands in the new step.
- **Rejected**: The preceding `api-request` record time (not present in every version, adds a join).

### Steps by name and rows only (architecture)
- **Decision**: Token objects go on `steps` (by name), `tasks` and `helpers` rows and the run; `step_entries` and `user_waits` get none.
- **Rationale**: The spec asks for each step, task and helper; comparisons key by step name.
- **Rejected**: Per-entry token figures (more output with no requirement behind it).

### Token JSON details (data_structures)
- **Decision**: Token counts as integers; `cost` rounded to 6 decimals, `null` when tokens exist but none are priced; `working_ms_per_1k_output` integer ms, `null` with no output tokens and on figures without time (outside steps, overhead); comparisons show an incomplete cost as its priced part, the run report marks it incomplete; no new error code (unreadable sessions are reasons, not refusals).
- **Rationale**: Matches 000075's integer-ms convention and "null, never 0"; the spec forbids errors for missing session data.
- **Rejected**: Cost in integer micro-units (unreadable); a `cost_complete` flag inside the generic figure row (would change its shape).

### `--sessions-dir` flag (data_structures)
- **Decision**: A persistent `--sessions-dir` flag on the `stats` group overrides the Claude Code projects directory; 000076's and 000077's subcommands inherit and ignore it.
- **Rationale**: Hermetic tests and reading copied session folders; no environment variable is introduced by Spektacular itself (Claude's own `CLAUDE_CONFIG_DIR` is honoured).
- **Rejected**: Only honouring the environment (tests would need process-wide env changes).

### Task split and order (tasks)
- **Decision**: Eight agent tasks: session reader and price config in parallel; attribution; report and renderer; command line; comparisons and summaries; docs (docs repo); README. No human tasks.
- **Rationale**: The reader and config are independent leaves; attribution is the high-risk core and gets its own task; the contract-preserving report change is isolated so its additivity test lands with it.
- **Rejected**: One task per package layer including compare (compare needs the report's token rows first); a separate task for `cmd` compare and summary wiring (one helper already serves all three).

### `cmd` tests isolated through `CLAUDE_CONFIG_DIR` in TestMain (tasks)
- **Decision**: `cmd`'s existing `TestMain` points `CLAUDE_CONFIG_DIR` at an empty temp directory for the whole package; token tests pass `--sessions-dir`.
- **Rationale**: Keeps 000075's existing `stats` tests deterministic without editing each one; process-wide but set once before any test, so order-independent.
- **Rejected**: A package-level override variable (one more test hook to reset); editing every existing test.

## Rehydration cues

- `spektacular spec file read 000078_token-use-in-run-stats`
- `spektacular plan file read 000075_run-stats-command plan` and `… context` (contract being extended); `000074_trace-spans-for-agent-work plan` (span meanings); `000073_log-mode-and-logger-providers plan` (record fields).
- `spektacular epic summary read 000002_workflow-tracing-and-run-stats` (cross-plan rules and file overlaps).
- Inspect one real transcript read-only with a short `python3 -I` script: count `type`s, show one `assistant` record's `message.usage`, list `<session>/subagents/*.meta.json`, and compare summed usage to the last `cost-state`.
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"gotchas/harbor-agent-log-fields.md"}'` (Claude field meanings in Harbor's format).
