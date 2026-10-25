---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Context: 000078_token-use-in-run-stats

## Current State Analysis

- **The report this plan extends does not exist yet.** 000075 (planned) adds:
  - `internal/runstats`: `Load`, `Assemble`, `Run`, `Span`, `BuildReport(r, Options)`, `Report` with step, task and helper rows, `Compare`/`Summarise` with the generic `Figure`/`Stat` rows, and `render.go`;
  - its golden files `internal/runstats/testdata/{report,compare,summary}.golden.{json,txt}`;
  - `cmd/stats.go` with `list`, `run`, `compare` and `summary`, the shared flags, `statsLogs`, `statsFilter` and `statsError`, and 000073's logging-exempt annotation.

  000076 adds `stats commands` and `stats spans`. 000077 adds `stats view` and calls `BuildReport(r, runstats.Options{Gaps, GapThreshold})`.
- **The logs never hold tokens.** 000073's command records carry `process.command_args`, `start_time`, `end_time` and `spektacular.agent`. 000074 adds step, task and `sub-agent` spans, and its instructions put each helper's sub-agent span id (`<32hex>-<16hex>`) in the helper's brief.
- **Claude Code's own session files hold every request's tokens** (inspected read-only on this machine, Claude Code 2.1.309):
  - `~/.claude/projects/<cwd with non-alphanumerics as ->/<session>.jsonl`;
  - helpers in `<session>/subagents/agent-<id>.jsonl` with `agent-<id>.meta.json` (`toolUseId`, `parentAgentId`, `spawnDepth`, `description`);
  - `assistant` records with `message.{id, model, usage}`, repeated once per content block;
  - `Bash` and `Agent` `tool_use` blocks, and `tool_result` blocks in `user` records;
  - a `cost-state` record of Claude Code's own per-model totals near the end of the file.

  Transcript requests fall well short of those totals (97–99% of uncached input, up to 64% of output missing), because side requests such as permission checks are never written as `assistant` records.
- **Harbor reads a different Claude format.** `tests/harbor/shared/transcripts.py:286-334` reads headless `stream-json` `result.modelUsage`. It is unchanged here (spec Non-Goals).
- **Configuration.** `internal/config/config.go:376-404` holds the `Config` struct. `ParseYAMLFile` (`config.go:479-512`) decodes non-strictly, so an optional key needs no format bump, as 000072's optional `knowledge.prune` key also shows. Validation errors follow `config.go:191-212`.
- **Tests.** `cmd/main_test.go:12-15` is the package's `TestMain`. `cmd/root_test.go:45-76` holds `resetRootCmd`/`runRootCmd`.
- **Docs.** `docs:src/pages/configuration.mdx` holds the example `config.yaml` (lines 32-96), the top-level key count sentence (103-110) and `ConfigKey` entries. `docs:src/pages/run-stats.mdx` is created by 000075.

**Requirement to repo and files**

| Requirement | Repo | Files |
|---|---|---|
| A run's report gives its token use (total, steps, tasks, helpers) | spektacular | `internal/runstats/tokens*.go`, `internal/runstats/report.go`, `internal/runstats/render.go` |
| Helper agents' tokens are counted | spektacular | `internal/agentusage/meta.go`, `internal/runstats/tokens_helpers.go` |
| Tokens attributed by when they were used; outside any step | spektacular | `internal/runstats/tokens_place.go`, `internal/runstats/tokens_overhead.go` |
| Time set against work done | spektacular | `internal/runstats/tokens.go` |
| The report gives cost | spektacular | `internal/config/config.go`, `internal/runstats/prices.go`, `cmd/stats.go` |
| The models used are listed | spektacular | `internal/runstats/tokens.go`, `internal/runstats/render.go` |
| Comparisons include tokens | spektacular | `internal/runstats/compare.go`, `internal/runstats/render.go`, `cmd/stats.go` |
| Summaries include tokens | spektacular | `internal/runstats/summary.go`, `internal/runstats/render.go`, `cmd/stats.go` |
| Runs matched to agent sessions without agent action | spektacular | `internal/agentusage/claude.go`, `internal/runstats/tokens_match.go` |
| Claude Code sessions supported; others unknown | spektacular | `internal/agentusage/*`, `internal/runstats/tokens.go` |
| Missing token data stays explicit | spektacular | `internal/agentusage/claude.go`, `internal/runstats/tokens.go` |
| Reading token data changes nothing | spektacular | `internal/agentusage/transcript.go` (read-only), `cmd/stats_test.go` (byte-identical check) |
| The documentation describes token figures | docs, spektacular | `docs:src/pages/run-stats.mdx`, `docs:src/pages/configuration.mdx`, `README.md` |

## Per-Task Technical Notes

### Task: Read Claude Code sessions

**File changes**
- `internal/agentusage/doc.go` (new): package comment. It reads coding agents' own session files read-only into plain data: requests, shell calls, helper launches and session totals. It never writes, never imports `runstats`, `tracelog` or `config`, and never fails a caller (every problem is a `Problem`).
- `internal/agentusage/types.go` (new): `Usage`, `Request`, `ShellCall`, `Launch`, `Transcript`, `Session` and `Problem`, as in plan.md's Data Structures. Problem codes are constants: `ProblemDirMissing = "sessions_dir_missing"`, `ProblemUnreadable = "session_unreadable"`, `ProblemUnrecognised = "format_unrecognised"`. `(Usage).Add`.
- `internal/agentusage/claude.go` (new): `type Claude struct{ Dir string }`, `DefaultClaudeDir()` (`$CLAUDE_CONFIG_DIR/projects` when the variable is set and non-empty, else `filepath.Join(os.UserHomeDir(), ".claude", "projects")`), and `(Claude).Sessions(since time.Time) ([]*Session, []Problem)`.
  - A missing `Dir` gives one `ProblemDirMissing` naming the path.
  - Discovery: for each directory entry under `Dir` (a project), each `*.jsonl` file directly inside it is a session. Its latest modification time is the max of the main file and every `<session>/subagents/agent-*.jsonl`. It is a candidate when that time is `>= since`. Sort candidates by path for determinism.
  - Each candidate is parsed by `readSession(mainPath)`. A read error gives `ProblemUnreadable` (`session <id>: <err>`). A session with zero recognised records, meaning no `assistant` record with `message.usage` and no `cost-state`, gives `ProblemUnrecognised`. Problems never stop the other sessions.
- `internal/agentusage/transcript.go` (new): `readTranscript(path) (Transcript, skipped int, totals map[string]Usage, err error)`.
  - The file is opened with `os.Open` and read with `bufio.Reader.ReadBytes('\n')`, with no line cap. A last line without `\n` is still parsed. Blank lines are ignored.
  - Pre-filter: decode only lines containing `"type":"assistant"`, `"type":"user"` or `"type":"cost-state"`. Lines of `api-request-blob`, `attachment` and other types are skipped by substring before decoding. Decode into small structs holding only the fields in plan.md's Data Structures. `tool_result` content is not decoded beyond `tool_use_id`. A decode failure counts as skipped.
  - `assistant`: key by `message.id`. Keep the first record's `timestamp` as `Request.At` and the model. Take the field-wise max of `usage.{input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens}` across records with the same id. Real sessions repeat identical usage over a response's records, and a few later records carry larger values. For each `tool_use` block: `name == "Bash"` records a pending call `{id, input.command, At: record timestamp}`; `name` `Agent` or `Task` records `Launch{ToolUseID: id, Description: input.description, Prompt: input.prompt, At}`.
  - `user`: for each content block of type `tool_result` whose `tool_use_id` is a pending Bash call, set `End` to the record's timestamp. Calls with no result keep `End = Start`.
  - `cost-state`: the last one wins. `modelUsage.<model>.{inputTokens, outputTokens, cacheReadInputTokens, cacheCreationInputTokens}` become `Totals`.
  - `Transcript.Start`/`End` are the min and max record timestamps seen.
  - Requests are sorted by `At`, and calls by `Start`.
- `internal/agentusage/meta.go` (new): `readMeta(path)` decodes `agent-<id>.meta.json` `{toolUseId, parentAgentId, description}` into the transcript's `LaunchID`, `ParentID` and `Description`. A missing or bad meta leaves them empty, and the transcript is still read.
- `internal/agentusage/session.go` (new): `readSession(mainPath)`. Its `ID` is the file's base name without `.jsonl`. `Transcripts[0]` is the main one (`ID: "main"`), then every `<id>/subagents/agent-*.jsonl` sorted by name (`ID` = the agent id from the file name). `Totals` comes from the main file. `Skipped` is the sum over files.
- `internal/agentusage/testdata/claude/` (new): a hand-written projects tree shaped like real transcripts.
  - Project `-tmp-proj` holds session `s1`: a main file with two responses, one split over three records with a later record carrying larger usage; two `Bash` calls running `spektacular plan new …` and `spektacular plan goto …` with results; one `Agent` launch; and a final `cost-state`.
  - `s1/subagents/` holds `agent-a1.jsonl` and `agent-a1.meta.json` (`toolUseId` = the launch id), plus `agent-a2.*` with `parentAgentId: "a1"` and `spawnDepth: 2`.
  - Also: a malformed line, a session `s2` with no recognised records, and an unreadable session (created with mode 0 in the test, skipped on Windows and when running as root).
- `internal/agentusage/claude_test.go`, `transcript_test.go` (new): table tests for each acceptance criterion.
  - De-duplication and max usage; request time; call start and end; launches; meta links including depth 2; totals present and absent.
  - Discovery with `os.Chtimes`, so files are old except one helper file newer than `since`: the session is a candidate. Equally, a session older than `since` is not.
  - Each problem code; skipped-line counting.
  - A test that hashes every file under the testdata copy (copied into `t.TempDir()`) before and after `Sessions` and finds them equal.
- `internal/agentusage/bench_test.go` (new): `BenchmarkReadSession` over a generated ~10 MB main file with 2,000 responses and large `tool_result` lines.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) the transcript and meta parsers with their tests; (b) discovery, problems and session assembly with their tests against a stub parser. Integrate sequentially.

### Task: Price table in the project configuration

**File changes**
- `internal/config/config.go:376-404` (`Config` struct): add `Stats StatsConfig \`yaml:"stats,omitempty"\``. Place it after the `log` section 000073 adds (or after `AutoCommit` if `Debug` is still there).
  - Add `StatsConfig{Prices map[string]ModelPrice \`yaml:"prices,omitempty"\`}`.
  - Add `ModelPrice{Input, Output, CacheRead, CacheWrite *float64}` with yaml tags `input`, `output`, `cache_read` and `cache_write`. Pointers tell a missing price from 0.
  - Add `(StatsConfig) IsZero() bool` so `omitempty` drops an empty section.
- `internal/config/config.go:689-748` (`Validate`): call `c.Stats.Validate()` after `c.Implement.GoalCheck.Validate()`.
  - `StatsConfig.Validate` walks models in sorted order. A nil price for any of the four gives `output.NewError("config_invalid", fmt.Sprintf("stats.prices.%s has no %s price", model, key)).WithResource("stats.prices").WithNextAction(fmt.Sprintf("set stats.prices.%s.%s in .spektacular/config.yaml to its price per million tokens, or remove the model to report its cost as unknown", model, key))`.
  - A negative value gives the same code, with the message "must not be negative".
  - An empty model name is refused the same way.
- `internal/config/config.go:417-461` (`NewDefault`): no change. Prices have no default, and `init` and `migrate` never write them.
- `internal/config/schema.go:19`: no change (`CurrentProjectSchema` stays as 000073 leaves it).
- `internal/config/config_test.go` (extend):
  - two models with all prices load exactly;
  - a config without `stats` loads with an empty map and `ToYAMLFile` writes no `stats:` key (raw file text asserted);
  - a missing `cache_write` and a negative `output` each give `config_invalid` with a non-empty next action naming the key;
  - a round trip keeps prices.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Place tokens on steps, tasks and helpers

**File changes** (all in `internal/runstats`, pure, no I/O. They use 000075's `Run`, `Span`, the `Report` rows and `tracelog` attribute names, and import `internal/agentusage` for its types only.)
- `internal/runstats/prices.go` (new): `Price`, `Prices`, and `price(p Prices, model string, u agentusage.Usage) (cost float64, priced bool)`, computed as `(u.Input*p.Input + u.Output*p.Output + u.CacheRead*p.CacheRead + u.CacheWrite*p.CacheWrite) / 1e6`.
- `internal/runstats/tokens_match.go` (new): `drivers(run *Run, sessions []*agentusage.Session) []driverRef`.
  - Keys are the run's command records whose `process.command_args[0] == run.Kind` and `[1]` is `new` or `goto`.
  - A key matches a `ShellCall` when `call.Start-1s <= rec.StartTime <= max(call.End, call.Start)+1s` and the call's command text, with whitespace collapsed, contains `args[0]+" "+args[1]`.
  - Every transcript with at least one match is a driver; this is returned as `{session, transcript index, matches}`.
  - For the between-runs rule, `runsDrivenBy(transcript, allRuns)` applies the same match to every run in `TokenInput.AllRuns` whose window overlaps the transcript's `[Start, End]`. This is memoised per transcript within one call.
- `internal/runstats/tokens_place.go` (new): `placeRequests(run, report, drivers, allRuns) placement`.
  - For each driver transcript, its requests are owned by this run when:
    - `At` lies in some driven run's `[Start, End]` and that run is this one (on overlap, the latest start wins);
    - or `At` precedes every driven run and this run is the first;
    - or `At` lies between two driven runs and this run is the next one;
    - or `At` follows every driven run and this run is the last.
  - An owned request goes to the step entry whose `[Start, End)` contains `At` (from `report.StepEntries`, effective ends), and to the task whose span contains `At`. Otherwise it is outside any step.
  - Per-model usage is accumulated per step name, per task id and outside.
- `internal/runstats/tokens_helpers.go` (new): `placeHelpers(run, report, drivers, sessions, ownedDrivers) (perSpan map[spanID]usageByModel, unmatched []unmatchedHelper)`.
  - Helper transcripts are those with `LaunchID` equal to a `Launch.ToolUseID` in a driver transcript (depth 1), plus descendants by `ParentID` (rolled into their depth-1 ancestor).
  - A helper transcript that is itself a driver of any run in `AllRuns` is excluded.
  - The launch is owned by this run under the same ownership rule as requests, using `Launch.At`.
  - Matching:
    1. find every `<run.ID>-<16 hex>` in `Launch.Prompt` (regexp `\b([0-9a-f]{32})-([0-9a-f]{16})\b` with group 1 equal to `run.ID`) whose span is a `sub-agent` span of the run and not yet matched;
    2. otherwise, over launches in `At` order, the unmatched `sub-agent` span with the latest `Start <= Launch.At+1s`;
    3. otherwise unmatched.
  - A matched helper's usage goes to its span, and through the span's step (the helper row's `step`) and task (`task_id`) to the step and task.
  - An unmatched helper goes to the step entry and task containing `Launch.At`, or outside any step. It is listed with `name` (meta description, else the launch description), `step`, `task_id`, `started_at` (transcript start), `ended_at` (transcript end) and `duration_ms`.
- `internal/runstats/tokens_overhead.go` (new): `overhead(session, ownedAll bool, transcriptsTotal usageByModel) (usageByModel, reason string)`.
  - It is known only when every transcript of the session belongs to this run (`ownedAll`: the session's drivers drove only this run, and every helper transcript is placed here) and `session.Totals != nil`.
  - The value is `Totals[m] - transcriptsTotal[m]` per model and type. Any negative gives reason `totals_below_transcript` and no overhead.
  - Reasons are `several_runs`, `no_totals` and `totals_below_transcript`.
  - A known overhead is added to outside-any-step and to the total.
- `internal/runstats/tokens.go` (new): `buildTokens(run *Run, rep *Report, in *TokenInput) (TokenSection, rows)`.
  - `in == nil` gives `status: unknown`, `not_read`.
  - An agent other than `claude` or `unknown` gives `agent_unsupported`, with the message "token use is read only for Claude Code sessions; this run's agent is <agent>".
  - No sessions and a `sessions_dir_missing` problem gives that problem.
  - No driver gives `session_not_found`, with the message "no Claude Code session under <dir> ran this run's commands (N sessions searched)", or with the first `session_unreadable`/`format_unrecognised` problem appended when there was one.
  - Otherwise it builds:
    - `TokenFigure`s per step name, per task id and per helper span;
    - outside-any-step, overhead and the total, which is the sum of the steps plus outside, asserted equal to the transcripts total plus overhead;
    - `models`, sorted by model name;
    - `unmatched_helpers`;
    - `sessions` (`session_id`, `transcript`, `file`), one per driver transcript.
  - `TokenFigure` from `usageByModel` and a duration:
    - the four sums;
    - `cost` is the sum over priced models, rounded to 6 decimals; nil when the tokens are non-zero and no model is priced; 0 when there are no tokens;
    - `cost_complete` and `unpriced_models` (sorted);
    - `working_ms_per_1k_output = round(workingMS * 1000 / output)`, nil when output is 0 or the figure has no duration.
  - Durations:
    - the run uses `Totals.WorkingMS`;
    - a step uses the sum of its entries' `duration_ms` less its `user_wait_ms`, floored at 0;
    - a task uses `duration_ms - user_wait_ms`;
    - a helper uses its row's `duration_ms`;
    - an unmatched helper uses its transcript's duration.
- `internal/runstats/tokens_test.go`, `tokens_match_test.go`, `tokens_place_test.go`, `tokens_helpers_test.go`, `tokens_overhead_test.go`, `prices_test.go` (new). Table tests over runs built with 000075's `rec(...)` helper and `Assemble`, and sessions built in Go.
  - `oneSession`: two steps, six requests, one outside before `new`. The sums are checked against hand totals, and steps plus outside equal the total.
  - `gotoResponse`: the response that issues a `goto` lands in the old step.
  - `lateHelper`: launched in `discovery`, transcript ends after `architecture` began; all its usage lands on the span and in `discovery`.
  - `twoTasks`: an implement run with two task spans, tokens and cost per task.
  - `byBriefID`: a prompt containing the sub-agent span id.
  - `byLaunchOrder`: two launches in one response, two spans just before.
  - `unmatched`: a launch with no span.
  - `depthTwo`: a nested helper rolled into its parent.
  - `childDriver`: a helper transcript that drove its own run, excluded from the parent's helpers.
  - `resumed`: a run driven by transcripts in two sessions.
  - `twoRuns`: one transcript driving spec then plan runs, with between-run requests going to plan and trailing ones to plan.
  - `overheadKnown`: run total equals `Totals`. Also `several_runs`, `no_totals` and `totals_below_transcript`.
  - `pricing`: two priced models giving the exact cost; one unpriced giving the incomplete cost, named; no table giving nil; zero tokens giving 0.
  - `pace`: values for run, step, task and helper; a concurrent helper does not change another helper's figure; 0 output gives nil.
  - `bobRun`: `agent_unsupported`. `noSession`: `session_not_found`.

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis, sequential integration: (a) matching and request placement with tests; (b) helper matching and overhead with tests; (c) pricing and figures with tests. One agent integrates `buildTokens` and the invariant test.

### Task: Report and print token figures

**File changes**
- `internal/runstats/testdata/pre-tokens/` (new): before changing any type, copy 000075's `report.golden.json`, `compare.golden.json` and `summary.golden.json` here unchanged. Also copy 000076's `commands.golden.json` and `spans.golden.json` when they exist, to show they are untouched.
- `internal/runstats/report.go` (000075):
  - `Options` gains `Tokens *TokenInput` (last field; keyed literals elsewhere, including 000077's `Options{Gaps, GapThreshold}`, compile unchanged).
  - `Report` gains `Tokens TokenSection \`json:"tokens"\``.
  - The step, task and helper row types gain `Tokens *TokenFigure \`json:"tokens"\``.
  - `BuildReport` calls `buildTokens` at its end and fills them. No existing field is computed differently.
- `internal/runstats/tokens_types.go` (new): `TokenFigure`, `TokenSection`, `ModelTokens`, `UnmatchedHelper` and `SessionRef`, with JSON tags exactly as plan.md's Data Structures. Every nullable value is a pointer, and lists are non-nil empty slices.
- `internal/runstats/render.go` (000075):
  - `RenderReport`: the Steps table gains `OUT TOKENS`, `COST` and `TIME/1K OUT`, and so do Tasks and Helpers.
    - Tokens print with thousands separators.
    - Cost prints with 2 decimals, `?` when unknown, and a trailing `*` when incomplete.
    - `TIME/1K OUT` uses `fmtMS`, and `n/a` when null.
  - A new `Tokens` block after Steps:
    - a status line naming the session(s), or `Tokens: unknown (<reason>)`;
    - a table STEP, INPUT, OUTPUT, CACHE READ, CACHE WRITE, COST, with rows per step, `(outside any step)` (with `of which session overhead` when known, or its reason) and `TOTAL`;
    - then `Models` (MODEL, INPUT, OUTPUT, CACHE READ, CACHE WRITE, COST);
    - then `Unmatched helpers` when there are any;
    - and a footnote `* incomplete: no price for <models>`.
  - With tokens unknown, only the status line prints and the new columns show `?`.
- `internal/runstats/testdata/report.golden.json` and `report.golden.txt` (regenerated with `-update`).
- `internal/runstats/contract_test.go` (new):
  - `TestTokenFieldsAreAdditive` decodes each regenerated golden JSON into `map[string]any`, deletes `tokens` at the top level and in every `steps`, `tasks` and `helpers` element, and drops figure entries whose `figure` is one of the six new names. It re-encodes canonically and compares with the same canonical encoding of `testdata/pre-tokens/<name>`. The compare and summary halves are enabled by the next task.
  - `TestReportWithoutTokenInput` checks that `BuildReport(r, Options{Gaps, GapThreshold})` gives `tokens.status == "unknown"`, `reason_code == "not_read"`, null row tokens, and `Totals` equal to a build with token input.
- `internal/runstats/render_test.go` (extend): the token columns, the Tokens block, the unknown line, incomplete cost marking.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: 2 parallel agents: (a) types, report wiring and the contract tests; (b) the renderer and its golden text. Integrate sequentially.

### Task: Token figures from the command line

**File changes**
- `cmd/stats.go` (000075):
  - Add the persistent flag `statsCmd.PersistentFlags().String("sessions-dir", "", "Claude Code projects directory to read token use from (default $CLAUDE_CONFIG_DIR/projects or ~/.claude/projects)")`.
  - Add the helper `statsTokenInput(cmd, logs *runstats.Logs, runs []*runstats.Run) *runstats.TokenInput`:
    - the directory is `--sessions-dir` made absolute, else `agentusage.DefaultClaudeDir()`;
    - `since` is the earliest `Start` among `runs`;
    - it calls `agentusage.Claude{Dir}.Sessions(since)`;
    - prices: when `projectConfigExists` (`cmd/root.go:426`), `loadConfig()` and convert `cfg.Stats.Prices` to `runstats.Prices`. An unreadable config is ignored here, because the gate already validated it.
    - `AllRuns` is `logs.Runs`.
  - In `statsRunCmd`'s `RunE`, set `Options.Tokens` from the helper before `BuildReport`. The JSON and pretty paths are unchanged otherwise. The `--schema` output lists the `tokens` field.
- `cmd/main_test.go:12-15` (`TestMain`): before `m.Run()`, create a temp dir with `os.MkdirTemp` and `os.Setenv("CLAUDE_CONFIG_DIR", dir)`, removing it after the run. A comment says why: no `cmd` test may read the developer's own Claude Code sessions.
- `cmd/root_test.go:45-62` (`resetRootCmd`): no change needed. It already resets every flag on the tree, including persistent string flags. Verify `--sessions-dir` is reset by a test.
- `cmd/stats_test.go` (extend 000075's):
  - The helper `seedSessions(t, dir, runs)` writes a Claude projects tree whose `Bash` calls wrap the fixture runs' `new`/`goto` command times (taken from `seedLogs`). It adds requests in two steps, a helper launch with a matching `sub-agent` span id in its brief, and a `cost-state`. File times are set with `os.Chtimes` after the runs' starts.
  - Tests:
    - `stats run --sessions-dir <fixture> --format json` has `tokens.status == "known"`, step and helper token figures, and models;
    - pretty output shows the same output-token and cost numbers;
    - prices from a `config.yaml` with `stats.prices` (written through `writeCurrentConfig` plus the section) give the expected cost;
    - without prices, `cost` is null and the table shows `?`;
    - the fixture's bob run gives `agent_unsupported`;
    - after deleting the fixture session, `session_not_found`, with every timing field equal to the earlier output;
    - with no `--sessions-dir`, existing behaviour reads the empty `CLAUDE_CONFIG_DIR` and gives `sessions_dir_missing` or `session_not_found`, never real data;
    - byte-identical logs and session files: hash both trees, run `stats run`, hash again.

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Comparisons and summaries carry tokens

**File changes**
- `internal/runstats/compare.go` (000075):
  - `Figure.Values` and `Figure.Delta` change from `[]*int64` to `[]*float64`. Existing integer figures are converted with `float64(v)`, and JSON output is unchanged because integral float64 values encode without a decimal point.
  - Append to the total figure list, after `gap_ms`, and to the step figure list, after `user_wait_ms`: `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `cost`, `working_ms_per_1k_output`. Values come from each run's `Report.Tokens.Total` (totals) or the step row's `Tokens`.
  - A run with unknown tokens, a step a run never entered, a null cost and a null pace each give null.
  - `delta` and `delta_pct` follow 000075's rules. `delta` of `cost` is rounded to 6 decimals.
- `internal/runstats/summary.go` (000075): the same figure lists, with `Stat` over non-null values (no type change).
- `internal/runstats/render.go`: `RenderComparison` and `RenderSummary` add the new figures to the Totals tables, and add `OUT TOKENS`, `COST` and `TIME/1K OUT` columns to the Steps tables, formatted as in the run report.
- `internal/runstats/testdata/compare.golden.json`, `summary.golden.json`, `compare.golden.txt` and `summary.golden.txt` (regenerated). Enable the compare and summary cases of `TestTokenFieldsAreAdditive`.
- `internal/runstats/compare_test.go` and `summary_test.go` (extend):
  - two runs with known tokens give values, deltas and percentages per step and in total;
  - one run unknown gives nulls;
  - five runs with output tokens of 10k, 20k, 30k, 40k and 50k give a median of 30k and a p90 of 50k, and the same for cost and pace.
- `cmd/stats.go`: `statsCompareCmd` and `statsSummaryCmd` set `Options.Tokens` through `statsTokenInput` with their selected runs.
- `cmd/stats_test.go` (extend):
  - `compare` and `summary` over the fixture sessions show the new figures in JSON and pretty form;
  - an unknown run gives null;
  - byte-identical logs and session files.

**Complexity**: Medium
**Token estimate**: ~18k tokens
**Agent strategy**: 2 parallel agents: (a) compare and summary with tests and goldens; (b) the renderer and `cmd` wiring with tests. Integrate sequentially.

### Task: Documentation site describes token figures

**File changes**
- `docs:src/pages/run-stats.mdx` (created by 000075; 000076 may add a listings section). Inside the existing `<Prose>` block, insert sections 1 to 5 of the plan's content outline after `## Unfinished work and damaged lines` and before `## Reports for programs`. Add item 6 to `## Reports for programs`. Use markdown headings and fenced code blocks only, with a blank line inside slot bodies, no `<div>`, `<section>` or `class=`, and no em dashes. Update the page's frontmatter `description` to mention tokens and cost.
- `docs:src/pages/configuration.mdx:4` (frontmatter `description`): add `stats` to the list of keys.
- `docs:src/pages/configuration.mdx:32-96` (example `config.yaml`): add after the `log:` block (000073), or after `debug:` if 000073 has not landed:
  ```yaml
  stats:
    prices:                       # optional; per million tokens, no built-in prices
      claude-opus-5-5:
        input: 5.00
        output: 25.00
        cache_read: 0.50
        cache_write: 6.25
  ```
- `docs:src/pages/configuration.mdx:103-110` ("Seventeen top-level keys"): change it to "Eighteen", and add `stats` after `log` (or `debug`) in the list.
- `docs:src/pages/configuration.mdx:200-207` (after the `log` `ConfigKey` 000073 writes in place of `debug`): add `<ConfigKey name="stats" type="section" defaultValue="none">` with the content example's body as a slot (blank lines inside, `stats.prices.<model>.input` and the others as a markdown list, and a link to `/run-stats/`).
- Verify with `npm run build`, `npx astro check` and `grep -nE "<div|<section|class=" src/pages/*.mdx` (no matches).

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: README notes token figures

**File changes**
- `README.md` ("How It Works", the `stats` block 000075 adds after the `status` block; 000076 and 000077 may add lines to it): add the plan's content-example paragraph straight after the `stats` code block. No em dashes. No other README change.

**Complexity**: Low
**Token estimate**: ~3k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Every Go test runs under the shuffled `go test` (`make test`). Fixtures use fixed timestamps, set file modification times with `os.Chtimes`, and live in each test's temp directory. `cmd`'s `TestMain` points `CLAUDE_CONFIG_DIR` at an empty temp directory.

- **Read Claude Code sessions**: `internal/agentusage` table tests over a testdata projects tree. They cover:
  - de-duplicated responses with the largest usage, and request timing;
  - shell call intervals; launches; meta links including depth 2;
  - `cost-state` totals present and absent;
  - discovery by modification time, including helper-only activity;
  - each problem code; skipped lines;
  - read-only hashing; a parsing benchmark.
- **Price table in the project configuration**: `internal/config` tests for loading, absence (no key written), a missing or negative price refused with a next action, and a round trip.
- **Place tokens on steps, tasks and helpers**: `internal/runstats` table tests named for each rule:
  - `oneSession`, `gotoResponse`, `lateHelper`, `twoTasks`;
  - `byBriefID`, `byLaunchOrder`, `unmatched`, `depthTwo`, `childDriver`;
  - `resumed`, `twoRuns`, `overheadKnown` and its three unknown reasons;
  - `pricing`, `pace`, `bobRun`, `noSession`;
  - plus the invariant that steps plus outside equal the total.
- **Report and print token figures**: regenerated `report.golden.json`/`.txt`; `TestTokenFieldsAreAdditive` against `testdata/pre-tokens/`; `TestReportWithoutTokenInput`; renderer tests for the columns, blocks, unknown line and incomplete cost.
- **Token figures from the command line**: `cmd` tests with `seedSessions` and `--sessions-dir`. They cover:
  - known tokens in JSON and pretty form;
  - prices from `config.yaml`, and no prices giving a null cost;
  - `agent_unsupported`;
  - a removed session giving unknown with identical timings;
  - the default directory never yielding real data;
  - byte-identical logs and session files.
- **Comparisons and summaries carry tokens**: compare and summary tests for the six appended figures (values, deltas, percentages, nulls, median and p90 over five runs); regenerated goldens with the additivity check; `cmd` tests in both formats; byte-identical files.
- **Documentation site describes token figures**: `npm run build`, `npx astro check`, and the MDX layout-HTML grep.
- **README notes token figures**: reviewed in the diff.

Success metrics and manual reviews are classified in plan.md's Testing Approach.

## Project References

- Spec `000078_token-use-in-run-stats`, epic `000002_workflow-tracing-and-run-stats`.
- Upstream plans:
  - `000075_run-stats-command`: the package, JSON contract, figure lists, golden files and `cmd/stats.go`;
  - `000074_trace-spans-for-agent-work`: span types, `spektacular.step`, the helper-brief span id;
  - `000073_log-mode-and-logger-providers`: command record attributes, `config.LogDir`, the logging-exempt annotation.
- Concurrent plans sharing files: `000076_harbor-on-built-in-logs` (`cmd/stats.go`, `internal/runstats/render.go`, `README.md`, `docs:src/pages/run-stats.mdx`) and `000077_html-log-viewer` (the same, plus its use of `BuildReport`). `000072_knowledge-access-index-and-prune` and `000073` share `docs:src/pages/configuration.mdx` and `internal/config/config.go`.
- Design documents: none.
- Knowledge entries applied:
  - `conventions/error-messages-must-suggest-remediation.md`;
  - `conventions/plans-never-change-the-active-install.md`;
  - `conventions/tests-must-not-depend-on-order.md`;
  - `conventions/tests-must-pass-for-done.md`;
  - `conventions/store-files-must-be-written-through-the-cli.md`;
  - `gotchas/harbor-agent-log-fields.md`, for Claude field meanings;
  - the docs repo's `conventions/mdx-authoring.md`, `conventions/plan-content-pages.md`, `conventions/no-em-dashes.md` and `conventions/site-layout.md`.
- Reference material: real Claude Code transcripts under `~/.claude/projects/` (read-only); `tests/harbor/shared/transcripts.py`.
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

This plan's tasks, with their tier and budget:
- Read Claude Code sessions: Medium, ~30k.
- Price table in the project configuration: Low, ~8k.
- Place tokens on steps, tasks and helpers: High, ~45k.
- Report and print token figures: Medium, ~25k.
- Token figures from the command line: Medium, ~20k.
- Comparisons and summaries carry tokens: Medium, ~18k.
- Documentation site describes token figures: Low, ~12k.
- README notes token figures: Low, ~3k.

## Migration Notes

None. No settings-format, state or record change. `stats.prices` is optional, has no default and is never written by `init` or `migrate`. Existing JSON fields are unchanged, and only new fields and appended figures appear. Installing the new binary is left to the maintainer between workflows (plans-never-change-the-active-install).

## Performance Considerations

- Candidate sessions are chosen by modification time, so a report reads only sessions active since the earliest selected run started. Each is parsed once per command and shared by every run in a comparison or summary.
- The parser pre-filters lines by record type before decoding and decodes only the needed fields. Attachment, request-body and large tool-result content is skipped. `BenchmarkReadSession` over a generated ~10 MB session is the supporting evidence. Real sessions on this machine are 1–9 MB, with up to 72 helper files.
- Attribution is linear in requests plus `n log n` sorting.
- 000075's under-one-second metric was measured without token reading. A report that reads several large sessions may take longer. If the manual check shows a report over 10 runs well above one second, a pre-decode substring filter for `usage` is an allowed internal change. A cache or index file is not, because it would write beside the agent's files.
