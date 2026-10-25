---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Context: 000076_harbor-on-built-in-logs

## Current State Analysis

- **Harbor measures timing itself, from two parsers.**
  - `tests/harbor/shared/sessionlog.py` (378 lines) reads the debug session log: lanes, segments, step windows from `state_before`/`state_after`, resumes from `response`, epic polls, reactions, merges, `plan_steps_entered` and `changelog_writes_by_repo`.
  - `tests/harbor/shared/metrics.py:277-607` joins those windows to transcript messages for per-step output, places children by launch time (Claude) or goto order (Bob), and takes `idle_s` as the longest gap.
  - `tests/harbor/shared/transcripts.py` (813 lines) reads every agent's own format. Codex has no sub-agent signal at all (`transcripts.py:8,713-720,745-748`).
  - `timeline.py` (178 lines) prints the epic timeline from `sessionlog`.
  - `delegation.py` places children through `metrics.place_children` and parses `epic worktree` responses (`delegation.py:106-120`).
  - The six shared modules total 2,999 lines.
- **Verifiers build `SessionLog` directly.** These are epic (`test_epic_workflow.py:24,72-73,110-164`), implement-epic (`:25,75-76,210-283,381-394`), goal-check (`test_goal_check.py:286-292`, self-check `:23,45-50,195-269`), plan (`test_plan_workflow.py:1369-1386`), implement (`test_implement_workflow.py:328-332`), standing-rules (`test_standing_rules.py:178-184`) and spec through delegation (`test_spec_workflow.py:574-585`). The checks needing verbose-only data are `Lane.children`/`stops` (response, `state_before`), the goal-record check (`state_before`), `worktree_paths` (response) and every step window (`state_after`).
- **After 000073 to 000075 (planned, not implemented):**
  - logs go to `.spektacular/logs/` as run files plus `standalone_<YYYYMMDD>.jsonl`;
  - the default `standard` level records argv, times, exit status and message, agent, workflow, step and `step.to`, but no response or state;
  - Harbor images set `SPEKTACULAR_SESSION_LOG=verbose`, and a normalising reader maps new records onto `sessionlog.Event`;
  - 000074 adds `shared/spans.py`, span checks in the verifiers, `TestUserWaits` and `totals.trace_calls`;
  - 000075 adds `spektacular stats list|run|compare|summary` over workflow runs only, leaving out standalone traces.
- **Wiring.**
  - Every suite image except background-child copies a working-tree binary to `/usr/local/bin/spektacular`.
  - The verifier runs in the agent's container.
  - Every suite except background-child collects `/app/.spektacular` → `artifacts/spektacular`.
  - The Makefile copies `tests/harbor/shared/*.py` into each suite (`test_suite_wiring.py:151-206` pins it).
  - `test.sh` ends with `python3 /tests/metrics.py || true` after the reward.
- **Self-tests.**
  - `tests/harbor/shared/tests/` holds 11 files, run by hand with `uvx --with pytest pytest --import-mode=importlib tests/harbor/shared/tests`. There is no CI and no conftest.
  - The reference runs (`data/{m70-epic,s55-epic,s55-plan,s55-implement}`) hold Claude transcripts plus old-format `debug/` logs. Their pinned idle and placement values come from the debug logs.

**Requirement to repo and files**

| Requirement | Repo | Files |
|---|---|---|
| Step timings from Spektacular's logs | spektacular | `tests/harbor/shared/runlog.py`, `tests/harbor/shared/metrics.py` |
| Helper agents from Spektacular's logs | spektacular | `internal/runstats/listings.go`, `cmd/stats.go`, `tests/harbor/shared/runlog.py`, `tests/harbor/shared/metrics.py` |
| Idle and user-wait time in full | spektacular | `tests/harbor/shared/metrics.py`, `tests/harbor/shared/report.py` |
| Epic run timelines from the logs | spektacular | `internal/runstats/listings.go` (outside-run records), `tests/harbor/shared/runlog.py`, `tests/harbor/shared/timeline.py`, suite `test.sh` |
| Delegation checks judge from spans | spektacular | `tests/harbor/shared/delegation.py`, epic, implement-epic, goal-check and spec verifiers |
| Tokens, models and cost from transcripts | spektacular | `tests/harbor/shared/transcripts.py`, `tests/harbor/shared/tests/test_reference_runs.py` |
| Old parsers removed | spektacular | delete `tests/harbor/shared/sessionlog.py` and `spans.py`, trim `transcripts.py` and `metrics.py`, verifiers |
| Trial artifacts carry the logs | spektacular | suite Dockerfiles, `task.toml` (unchanged artifact), `tests/harbor/shared/tests/test_suite_wiring.py` |
| Missing data stays explicit; measuring never fails | spektacular | `tests/harbor/shared/runlog.py` (`Unavailable`), `tests/harbor/shared/metrics.py` (`gather`) |
| Cross-run reports use the new measures | spektacular | `tests/harbor/shared/report.py`, `README.md` |
| Documentation | spektacular, docs | `README.md`, knowledge entries, `docs:src/pages/run-stats.mdx` |

## Per-Task Technical Notes

### Task: List commands and spans in the run statistics package

**File changes**
- `internal/runstats/load.go` and `internal/runstats/assemble.go` (new in 000075): keep the records of traces with no `spektacular.workflow.kind` in a new `Logs.Outside []tracelog.Record` instead of dropping them. The run assembly itself is unchanged: `Runs`, their spans, events, statuses and order stay byte-identical, which 000075's `testdata/*.golden.json` proves. Keep the file name of each outside record for diagnostics only.
- `internal/runstats/listings.go` (new):
  - `CommandRow` with the JSON tags in plan.md's Data Structures. Fields: `run_id`, `kind`, `name`, `agent`, `span_id`, `parent_span_id`, `command` (record `Name`), `args` (`process.command_args`, `[]string`, empty array when absent), `step` (`spektacular.step`), `step_to` (`spektacular.step.to`), `started_at`, `ended_at`, `duration_ms`, `exit_code` (`process.exit_code`, else 0 for status `ok` and 1 otherwise), `status`, `message` (`Status.Message`, null when empty). Absent strings are `null` through pointer fields, as 000075 does.
  - `(*Logs).Commands(f Filter, outsideRuns bool) ([]CommandRow, error)`:
    - with `outsideRuns` and a non-empty filter, return `*SelectionError{Code:"bad_selection"}`;
    - with `outsideRuns`, use the command records in `Outside`;
    - otherwise, with a filter, use the commands of `l.Candidates(f)` (same errors as 000075);
    - otherwise use every run's `Commands` plus `Outside`.

    Sort by `StartTime`, then `span_id`.
  - `SpanRow`: 000075's helper row (`span_id`, `parent_span_id`, `name`, `step`, `task_id`, `started_at`, `ended_at`, `duration_ms`, `end_reason`, `status`, `message`, `complete`) plus `type`, `trace_id`, `run_id`, `kind` and `workflow` (the workflow's name, under its own key because `name` is the span's name).
  - `(*Logs).AgentSpans(f, outsideRuns)`: spans of type `sub-agent`, `user-wait` or `work`, from each selected run's paired `Spans` (effective ends as 000075 caps them) and, when not filtered by run, from `Outside`. Pair `Outside` records with the same pairing helper `Assemble` uses, keyed by `trace_id` plus `span_id`. Cap an outside span left open at the latest event of its own trace, else its start. The `step` comes from the attribute; `task_id` comes from the containing task span when in a run.
- `internal/runstats/render.go` (extend): `RenderCommands` (columns TIME, RUN (8-character id or `-`), STEP, COMMAND, EXIT, DURATION) and `RenderSpans` (TIME, RUN, TYPE, NAME, STEP, DURATION, END).
- `internal/runstats/listings_test.go` (new): table tests over hand-built records (000075's `rec(...)` helper). They cover:
  - two runs plus a `standalone_*` set;
  - ordering;
  - null run fields outside runs;
  - argv and exit-code fallbacks;
  - selection narrowing;
  - outside-runs only;
  - the selection-plus-outside refusal;
  - an open helper capped and `complete:false`;
  - an outside sub-agent span open at the end.
- `internal/runstats/testdata/commands.golden.json` and `spans.golden.json` (new): pin the field names (contract for Harbor's reader). They are regenerated only with `-update`.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: 2 parallel agents: (a) the command listing with its tests and golden file; (b) the span listing with its tests and golden file. The `Outside` bucket is done first by one agent.

### Task: List commands and spans from the command line

**File changes**
- `cmd/stats.go` (new in 000075): add `statsCommandsCmd` (`Use: "commands"`) and `statsSpansCmd` (`Use: "spans"`) under `statsCmd`. They inherit the persistent flags (`--dir`, `--format`, `--name`, `--kind`, `--agent`, `--run`) and the logging-exempt annotation. Each gains a local `--outside-runs` bool and `--schema`.
  - JSON goes through `output.New(cmd.OutOrStdout(), globalFields).WriteResult(v)` with `commandsResult{Source; Commands []runstats.CommandRow}` and `spansResult{Source; Spans []runstats.SpanRow}`, where `Source` carries `log_dir`/`skipped_lines`.
  - Pretty output uses `RenderCommands`/`RenderSpans`.
  - Map `bad_selection` to `stats_bad_selection`, with the next action "use --outside-runs on its own, or drop it and select runs with --name, --kind, --agent or --run". Other errors map through 000075's `statsError`.
- `cmd/stats_test.go` (extend 000075's tests, reusing `seedLogs`): add a standalone orchestrator command and an outside `sub-agent` span to the fixture.
  - Both commands in pretty and JSON form, agreeing.
  - `--fields '["commands"]'`.
  - `--outside-runs` returns only the standalone records.
  - The selection-plus-outside refusal has a `next_action`.
  - `--dir` on a copy, run from a non-project directory, gives identical JSON apart from `log_dir`.
  - Byte-identical logs before and after both commands.
- `README.md` ("How It Works" `stats` block that 000075 adds): two lines, `spektacular stats commands` (every recorded command, with its step and arguments) and `spektacular stats spans` (helpers and waits, including an orchestrator's).

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Documentation site describes the two listings

**File changes**
- `docs:src/pages/run-stats.mdx` (new in 000075): insert a `## Listing commands and spans` section inside the existing `<Prose>`, after `## Summarising runs` and before `## What each figure means`, following the plan's content example. Use fenced code blocks only, no layout HTML, no em dashes.
- `docs:src/pages/run-stats.mdx` `## Reports for programs` section: add one sentence naming the `commands` and `spans` arrays as stable fields.
- Verify with `npm run build`, `npx astro check`, `grep -nE "<div|<section|class=" src/pages/*.mdx` (expect nothing) and an em-dash grep over the changed file.

**Complexity**: Low
**Token estimate**: ~6k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Record baseline Harbor pass rates

**What the person does**
- After 000073, 000074 and 000075 are merged, and before any task of this plan's Milestone 2 is merged, build from that commit. Run each Claude suite target 5 times, using a job-name prefix such as `base76-` (Makefile `HARBOR_RUN_FLAGS` or by renaming the job directories under `tests/harbor/jobs/`). The targets are:
  - `make harbor-test-spec-claude`, `harbor-test-plan-claude`, `harbor-test-implement`;
  - `harbor-test-repo-one-at-a-time`, `harbor-test-repo-delegated`;
  - `harbor-test-epic-claude`, `harbor-test-implement-epic-claude`;
  - `harbor-test-standing-rules-claude`, `harbor-test-goal-check-claude`;
  - `harbor-test-background-child`.
- Record each suite's pass count out of 5 with `python3 tests/harbor/shared/report.py 'tests/harbor/jobs/base76-*'`. Keep the jobs: the test plan's after-comparison reads them with `report.py --baseline 'tests/harbor/jobs/base76-*' --candidate <after jobs>`.

**How it is checked**: five trials per suite under the prefix, each with a `verifier/reward.txt`, from a build whose `build.json` commit predates this plan's Milestone 2.

**Complexity**: Low (person time: about a day of suite runs)
**Token estimate**: n/a
**Agent strategy**: n/a (person).

### Task: Pin today's token, model and cost figures

**File changes**
- `tests/harbor/shared/tests/test_reference_runs.py` (extend; `RUNS` at :36, pinned values at :79-82): add `PINNED_TOTALS`, a dict per reference run (m70-epic, s55-epic, s55-plan, s55-implement) of `{model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, turns, model_time_s, wall_time_s}`. Use the values `metrics.read_agent_log(<run dir>, agent).totals` computes on the unchanged harness, plus the per-child models already pinned at :148-158.
  - Add `test_reference_totals_are_pinned`, which reads each run's `claude-code.txt` through `transcripts.claude_summary` (`tests/harbor/shared/transcripts.py:286-334`) and compares field by field.
  - Write the values by running the current code once, and put them in the test as literals with a comment: "pinned before 000076; must not change".
- `tests/harbor/shared/tests/test_transcripts_summary.py` (no change): it already pins the ATIF and Codex readers on hand-built input (:582-610 and the summary tests). Note that in the test docstring.

**Complexity**: Low
**Token estimate**: ~6k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Harbor run log reader

**File changes**
- `tests/harbor/shared/runlog.py` (new; budget about 250 lines): implement the contract in plan.md's Data Structures.
  - `_stats(args)` runs `[binary, "stats", *args, "--dir", str(log_dir), "--format", "json"]` with `runner(..., capture_output=True, text=True, timeout=60)`.
    - `binary` is the constructor argument, else `os.environ.get("SPEKTACULAR_BIN")`, else `shutil.which("spektacular")`. None raises `Unavailable("no spektacular binary on PATH; set SPEKTACULAR_BIN to one that has `stats`")`.
    - It checks `log_dir.is_dir()` first: `Unavailable(f"no Spektacular logs at {log_dir}")`.
    - It parses stdout JSON. When `"error": true`, raise `Unavailable(f"stats {sub}: {code}: {message}")`. A timeout, `OSError` or `ValueError` becomes `Unavailable`.
  - `Command.data()`, `is_verb`, `goto_step` and `_positional` port the argv helpers from `tests/harbor/shared/sessionlog.py:60-85,281-298` unchanged. `parse_time` ports `sessionlog.py:34-40`, the nanosecond trim.
  - `lanes(kinds)`:
    - group `commands()` by `(kind, name)` where `kind in kinds`, and attach the `runs()` summaries with the same kind and name;
    - `start` is the first `ok` `is_verb(kind, "new")`;
    - `finish` is the first `ok` `goto_step == "finished"`;
    - `resumes` are the commands `is_verb(kind, "new") and not ok` with `at > start.at`;
    - `stops` is `[c.step or "?" for c in resumes]`, plus `"restart"` for each later run of the lane that began with a successful `new` (the old segment rule, `sessionlog.py:165-180`).
  - `outside()` is the commands with `run_id is None`.
  - `epic_polls()` is outside commands `is_verb("status")` whose second arg is not a lane name (`sessionlog.py:367-378`).
  - `names(cmd)` follows `sessionlog.py:332-338`.
  - `orchestrator_reaction(lane)` follows `sessionlog.py:340-358`, over `outside()`.
  - `merge(spec)` follows `sessionlog.py:360-365`, over `outside()`.
  - `steps_entered(kind)` is the `step_to` of each `ok` `<kind> new|goto` in time order, falling back to `goto_step`.
  - `changelog_writes_by_repo(name)` follows `sessionlog.py:247-269` over all commands.
  - Cache `runs()`, `commands()`, `spans()` and each `report(run_id)` in instance dicts.
- `tests/harbor/shared/tests/test_runlog.py` (new):
  - A `FakeRunner` maps `(subcommand, frozenset(flags))` to canned stdout.
  - Canned JSON comes from 000075's `internal/runstats/testdata/{report,compare}.golden.json` and this plan's `commands.golden.json`/`spans.golden.json` (loaded by path from the repo root). Add a small hand-written epic set: two plan lanes, one resumed through a failed `plan new` carrying `step: "architecture"`, plus outside status polls, `epic order` and `epic merge` commands.
  - Cases: typed parsing; each `Unavailable` cause; lanes, start, finish, resumes, children, stops; epic polls; reaction skipping commands naming the finished spec; merge; `steps_entered`; changelog writes for project, `--repo X` and `--repo=X`, skipping `--help`.
  - Port the rule cases from `tests/harbor/shared/tests/test_sessionlog_rules.py:1-495`, re-expressed over commands, so the semantics are visibly the same.
- `tests/harbor/shared/tests/test_runlog_integration.py` (new):
  - `pytest.importorskip` style skip when `shutil.which("go")` is None.
  - A session fixture runs `go build -o <tmp>/spektacular .` from the repo root (never installs, never `go run`).
  - It writes fixture JSON Lines into `<tmp>/logs` in 000073/000074 format: a spec run file with step spans, commands with `parent_span_id`, a `sub-agent` span and a `user-wait` span; an epic set of two plan lane files, one resume; and a `standalone_20261025.jsonl` holding status polls, an `epic merge` and an outside `sub-agent` span.
  - It checks that the reader's answers equal the canned-runner answers for the same fixture, and that a `cp -r` copy gives identical answers.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) `runlog.py` with the canned-JSON unit tests; (b) the fixture logs and the integration module. Integrate sequentially.

### Task: Metrics record from Spektacular's logs

**File changes**
- `tests/harbor/shared/metrics.py` (rewrite of 277-607 and 624-674; budget about 400 lines):
  - `SCHEMA_VERSION = 3` (:48).
  - `DEBUG_DIR` becomes `LOG_DIR = Path("/app/.spektacular/logs")` (:53). Keep `--debug-dir` as a hidden alias of `--log-dir`, because `test_suite_wiring.py:242-247` pins only the default invocation.
  - Remove `step_breakdown`, `window_of`, `lane_for` (moved to delegation), `read_session`, `session_lanes`, `lane_windows`, `PlacedChild`, `place_children`, `step_child_counts`, `child_record`, `read_children`, `count_step_children`, `idle_seconds`, `read_idle` and `session_span_s` (:269-594).
  - Add `read_logs(log_dir) -> (sections, unknowns)`. It builds one `runlog.RunLog`. For each `runs()` summary (start order) it calls `report(run_id)` and builds the workflow entry:
    - `run_id`, `kind`, `name`, `status`;
    - `wall_s = wall_ms/1000`, `user_wait_s`, `helper_wait_s`, `gap_s`, `gaps` from `totals`;
    - each `steps[]` row becomes `{step, started_at (first matching step_entries started_at), entries, duration_s, working_s = max(0, duration - helper - user), helper_wait_s, user_wait_s, commands, helpers (count of report helpers whose step == row.step), incomplete}`.

    Trial totals: sums of the runs' `user_wait_s`, `helper_wait_s`, `gap_s` and `gaps`, plus `gap_threshold_s` from `gap_threshold_ms`. `subagents` comes from `spans()` filtered to `type == "sub-agent"`:
    - `started` = count; `completed` = complete with status `ok`; `failed` = status `error`; `unfinished` = not complete;
    - `durations_s` = durations of the complete ones;
    - `helpers` = rows as in Data Structures. An outside span (`run_id` null) gets `outside_run: true`, `step: null`, and `spec` = the lane (from `lanes(("spec","plan","implement"))`) whose first run's `started_at` lies in `[span.start, span.end]`, else null.

    On `runlog.Unavailable` as `u`, return null sections and `Unknown` entries for `steps`, `subagents.helpers`, `subagents.started`, `totals.user_wait_s`, `totals.helper_wait_s`, `totals.gap_s`, each with `u.reason`. An empty `runs()` gives `Unknown("steps", "the Spektacular logs hold no workflow run")`.
  - `read_briefs(log)`: from `AgentLog.read_children` (Claude `tx.claude_children`, ATIF `tx.atif_children`, Codex none). Map to `{id, parent, kind, description, models, prompt, prompt_chars}`, with Unknowns as today (:495-504 logic for prompt_chars).
  - `AgentLog` (:105-119): drop `subagents`, `messages`, `launch_root` and `launches`; keep `agent`, `totals` and `read_children`. `read_agent_log` (:151-198) drops the matching reader calls. `missing_log` (:122-127) drops the subagent unknowns.
  - `wall_time_fallback` (:597-607): replace `session_span_s` with the span of `RunLog.runs()` (min `started_at` to max `ended_at`), skipping silently on `Unavailable`.
  - `build_record` (:624-674):
    - gather `logs` once (section `steps`) and `briefs` (section `subagents.briefs`);
    - `totals` = `TOTAL_FIELDS` + 000074's `trace_calls` (its per-task divisor = the count of `tasks` rows across reports) + the log totals;
    - `steps = {"workflows": ...}`;
    - `subagents = {started, completed, failed, unfinished, durations_s, helpers, briefs}`.
  - `summary` (:677-686): add `steps` and `helpers` counts.
- `tests/harbor/shared/transcripts.py` (trim; budget about 560 lines):
  - remove `Message` (:172-180), `_STEP_IN_DATA`/`goto_step_of` (:253-283), `_block_chars`/`claude_messages` (:337-370), `claude_subagents` (:373-403), `claude_launch_root` (:406-425), `atif_messages` (:694-709), `atif_subagents` (:712-724) and `Subagents` (:183-190);
  - `claude_children` (:471-582) keeps every field except `output`. Compute `models` directly from assistant messages with `parent_tool_use_id` instead of through `claude_messages`, and drop `Child.output` (:193-211) and its `ACP_CHILD_GAPS` entry (:727-738);
  - update the docstring (:1-20).
- `tests/harbor/shared/tests/test_metrics.py` (rewrite the step, child, idle and Bob-children sections at :1100-1491; update the key pins at :38-75 to schema 3). Add:
  - a fake `RunLog` (monkeypatch `metrics.runlog.RunLog`);
  - split arithmetic, including working floored at 0;
  - helper placement, including an outside span;
  - multi-run sums;
  - every Unavailable reason producing nulls and Unknowns, with no transcript fallback;
  - a recorder error in `read_logs` costing only its fields;
  - the reward untouched (existing :1590-1610);
  - three agents (Claude transcript, ATIF trajectory, Codex rollout fixtures already in the file) with the same fake logs giving identical `steps` and `subagents.helpers`.
- `tests/harbor/shared/tests/test_transcripts_summary.py`: delete the message and goto cases (:456-472) and the sub-agent-count cases. Keep the children, turns and wake-ups cases without `output`.
- `tests/harbor/shared/tests/test_runlog_integration.py` (extend): run `metrics.build_record` with `Paths(log_dir=<fixture logs>)`, and assert that its `steps.workflows[*].steps[*].duration_s` × 1000 equals `stats run --format json` `steps[*].duration_ms` for the same run.
- `tests/harbor/shared/tests/test_reference_runs.py`: the reference runs have no new-format logs. Assert their records carry the log sections as unknown with the "no Spektacular logs" reason, and that `PINNED_TOTALS` still match through `build_record`. Remove the `S55_EPIC_IDLE_S`/`M70_EPIC_IDLE_S` pins and the children placement pins (:79-82, :134-145). Keep the children prompt, model and duration pins through `claude_children`.
- `README.md` "#### Run metrics" (:506-559) — the field table (:519-527), step durations (:529-534), idle (:536-539), placement (:541-545) and the agent table (:552-556): rewrite for schema 3 (the `totals` log fields, `steps.workflows[].steps[]` split fields, `subagents.helpers`/`briefs`, and the per-agent table where only briefs and totals differ).
- `tests/harbor/shared/tests/test_readme_metrics.py` (:25-60, :127-187): update the hand-copied keys to schema 3, `SCHEMA_VERSION = "3"`, drop `idle_s`, and require the split fields and `helpers`/`briefs`.

**Complexity**: High
**Token estimate**: ~50k tokens
**Agent strategy**: Parallel analysis, sequential integration: (a) the `metrics.py` log sections with their tests; (b) the `transcripts.py` trim with its tests and the reference-run updates; (c) the README table and README test. One agent integrates `build_record` and runs the full shared suite.

### Task: Delegation checks judge from spans

**File changes**
- `tests/harbor/shared/delegation.py`:
  - Imports (:28-35): drop `metrics`, `PlacedChild` and `sessionlog`; import `runlog`.
  - Add `WORKTREE_DIR = ".spektacular/worktrees"` and `LAUNCH_SPAN_WINDOW_S = 120` beside the oracle (:36-56).
  - `Run` (:81-91) becomes `children: list[tx.Child]`, `log: runlog.RunLog`, `spans: list[runlog.Span]`, `lanes: dict[(kind, name), runlog.Lane]`, `turns`, `wakeups`, `end`.
  - `read_run(transcript, log_dir)` (:94-103) reads the transcript as today, plus `RunLog(log_dir)` with `spans()` and `lanes(("spec","plan","implement"))`. It propagates `Unavailable` as a `Failure("logs_unavailable", ...)` for callers that assert empty lists.
  - Replace `worktree_paths` (:106-120) with `worktree_folder(spec, root=PROJECT_ROOT) = f"{root}/{WORKTREE_DIR}/{spec}"`.
  - `missing_required(log, lanes, spans)` replaces :136-161:
    - for each lane, for each run of the lane, read `report(run_id)["step_entries"]`;
    - group consecutive entries of the same step into attempts (port `step_runs`, :124-133);
    - judge every attempt but the lane's last one;
    - pass when any `sub-agent` span has `run_id == run` and `step == step`, with `start` inside one of the attempt's entries;
    - the failure subject is unchanged: `kind:name:step (attempt n)`.
  - `missing_spec_children(lanes, spans)`, replacing :170-176: a `PER_SPEC_CHILD` lane fails unless some `sub-agent` span with `run_id is None` has `start <= lane.runs[0].started_at <= end`.
  - `lane_for` moves here from `tests/harbor/shared/metrics.py:269-274`.
  - `spec_children(children, lane)` (:164-167) uses top-level transcript children whose prompt names the lane (`lane_for`).
  - `epic_context_gaps(children, lanes, root)` (:181-211) replaces the per-repo worktree paths with `worktree_folder(name, root)` for implement lanes.
  - `review_context_gaps(children, spans)` (:214-226) uses the transcript children matched to a span with `step == "verification"` and `kind == "spec"`. The match is `match_spans(children, spans)`: for each child with `launched_at`, take the latest unused `sub-agent` span with `start <= launched_at <= start + LAUNCH_SPAN_WINDOW_S`.
  - The wake-up, turn and foreground rules (:229-302) take `children` instead of `placed`, with `p.child.x` becoming `c.x`.
  - Fold 000074's `unmarked_launches`/`marked_rate` (added beside the oracle by 000074) onto `match_spans`: an unmarked launch is a child with no matched span.
- `tests/harbor/shared/tests/test_delegation.py` (rewrite the run builders, keeping every rule case; :151, :341, :488-530 and :743-792 build lanes from session-log files and `worktree_paths` from responses):
  - Build `runlog` records directly: spans, lanes, step entries.
  - Reference-run cases (:743-792) keep only the transcript rules (wake-ups, settled children) on s55-epic and m70-epic. Placement cases use hand-built spans.
  - Add `test_worktree_dir_matches_go`: read `internal/worktree/worktree.go`, find `const Dir = "worktrees"`, and assert `WORKTREE_DIR == ".spektacular/" + Dir`.
  - Keep `oracle_template_mismatches` (:794+) and 000074's trace oracle test.
- `tests/harbor/shared/tests/test_epic_children.py` (:92-117 build `epic worktree` responses; :229-270 run the verifiers' `TestChildren`): replace the response-built records with fake `RunLog`s. Keep the AST guard (:393-407) unchanged, so only `TestChildren`/`children_run` touch delegation.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 2 parallel agents: (a) the `delegation.py` rules with `test_delegation.py`; (b) `test_epic_children.py` fixtures. Integrate sequentially.

### Task: Suite verifiers read the logs through stats

**File changes**
- `tests/harbor/epic-workflow/tests/test_epic_workflow.py`:
  - `from sessionlog import SessionLog` (:24) becomes `from runlog import RunLog, Unavailable`, and `DEBUG_DIR` (:27, as `.../logs` after 000073) becomes `LOG_DIR`.
  - The `log` fixture (:72-73) returns a small adapter: `RunLog(LOG_DIR)` with `lanes()` cached, so `log.lane(kind, name)`, `log.empty` (no runs and no commands; `Unavailable` as empty with its reason in the assertion message) and `log.shared` (= `outside()`) keep their call sites (:110-164).
  - `test_session_log_recorded` (:111-115) message: "no Spektacular logs under LOG_DIR: <reason>".
  - `TestChildren` (:175-214) calls the new delegation signatures with `LOG_DIR`.
- `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py`: the same import, fixture and adapter changes (:25, :28, :75-76, :210-283). The goal-record check (:381-394) replaces `(e.state_before or {}).get("current_step") != "update_plan"` with `e.step != "update_plan"`. `TestChildren` (:294-341) drops `worktrees=dl.worktree_paths(...)`, because the folder now comes from the spec name.
- `tests/harbor/goal-check-workflow/tests/test_goal_check.py`: the `events` fixture (:286-292) uses `RunLog(LOG_DIR).commands()`. The rules at :106-129 use `Command` (same `ok`, `is_verb`, `goto_step`, `command`→`args`, `at`). `rule_required_children` (:237-243) and `read_run` (:350-354) use the new delegation signatures.
- `tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py`: replace `from sessionlog import Event` (:23) and the `Event(...)` builder (:45-50) with `runlog.Command` builders. Replace the synthetic lane file (:195-269) with a fake `RunLog` holding step entries and a reviewer span.
- `tests/harbor/plan-workflow/tests/test_plan_workflow.py` `test_walkthrough_rendered_before_finished` (:1369-1386): `RunLog(LOG_DIR).steps_entered("plan")` replaces `SessionLog` plus `plan_steps_entered`. Fix the import at :51.
- `tests/harbor/implement-workflow/tests/test_implement_workflow.py` (:31, :328-332): `RunLog(LOG_DIR).changelog_writes_by_repo(PLAN_NAME)`. With `kinds=("implement",)` gone, the reader's commands already include standalone writes; confirm against `test_sessionlog_rules.py`'s s55-implement expectations, ported into `test_runlog.py`.
- `tests/harbor/standing-rules/tests/test_standing_rules.py` (:178-184): `[c.args for c in RunLog(LOG_DIR).commands()]`. Update the comment in `standing-rules/tests/test_verifier_selfcheck.py:32`.
- `tests/harbor/spec-workflow/tests/test_spec_workflow.py` (:574-585): `dl.read_run(CLAUDE_TRANSCRIPT, SPEK_DIR / "logs")`, then `missing_required` and `review_context_gaps` with the new signatures. 000074's `TestUserWaits` (added near :449-488) uses `RunLog(...).spans()`: a `user-wait` span with `step == "constraints"`.
- The epic, implement-epic and goal-check verifiers: 000074's `test_every_launch_is_a_sub_agent_span` checks call `dl.unmarked(children, spans)` instead of `spans.py`.
- `tests/harbor/shared/tests/test_sessionlog_rules.py` (:209-215, :460-475 monkeypatch the plan and implement verifiers' `DEBUG_DIR`): move those two verifier-wiring cases into `test_runlog.py` with a fake `RunLog`. The file itself is deleted in "Remove the old parsers and debug fixtures".

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 3 parallel agents: (a) the epic and implement-epic verifiers; (b) goal-check with its self-check, plus standing-rules; (c) plan, implement and spec. Then one agent runs every suite self-check.

### Task: Suite images record at the default level

**File changes**
- Remove the `ENV SPEKTACULAR_SESSION_LOG=...` line and its comment (set to `verbose` by 000073) from:
  - `tests/harbor/background-child/environment/Dockerfile:16-18`
  - `tests/harbor/epic-workflow/environment/Dockerfile:27-30`
  - `tests/harbor/goal-check-workflow/environment/Dockerfile:27-30`
  - `tests/harbor/implement-epic-workflow/environment/Dockerfile:29-32`
  - `tests/harbor/implement-workflow/environment/Dockerfile:15-17`
  - `tests/harbor/plan-workflow/environment/Dockerfile:15-17`
  - `tests/harbor/repo-workflow/environment/Dockerfile:15-17`
  - `tests/harbor/spec-workflow/environment/Dockerfile:15-17`
  - `tests/harbor/standing-rules/environment/Dockerfile:31-34`
- Keep the verifiers' own `SPEKTACULAR_SESSION_LOG: "off"` in their `cli()` helpers (epic :46, implement-epic :44, goal-check :266, standing-rules :174): it keeps verifier commands out of the agent's log and is not an image setting.
- `tests/harbor/shared/tests/test_suite_wiring.py` (extend; `SUITES` :44-90):
  - `test_no_image_sets_the_session_log`: no suite Dockerfile contains `SPEKTACULAR_SESSION_LOG`.
  - `test_spektacular_suites_collect_their_logs`: every suite whose Dockerfile copies the binary has a `task.toml` `[[artifacts]]` with `source = "/app/.spektacular"`.
  - The metrics-last check (:242-247) is unchanged.
- `tests/harbor/epic-workflow/task.toml:15-16`, `tests/harbor/implement-epic-workflow/task.toml:16`: update the timeline comments to name `.spektacular/logs`.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Epic timeline from Spektacular's logs

**File changes**
- `tests/harbor/shared/timeline.py` (rewrite; budget about 130 lines):
  - `DEFAULT_DIR = Path("/app/.spektacular/logs")` (:43). Keep `DEFAULT_BOB_LOGS`, `bob_turn_limit_hits`, `span` and `offset` (:44-72).
  - `report(log: RunLog, bob_logs)`:
    - on `Unavailable`, print "No Spektacular logs under <dir>: <reason>" and return;
    - run origin and end: min `started_at` and max `ended_at` over `runs()` and `outside()`;
    - lanes from `log.lanes()`, sorted by start;
    - per lane: start (`lane.start.at`), finish (`lane.finish.end`), took, steps (count of ok gotos), children, and slowest step (the `steps` row with the largest `duration_ms` across the lane's run reports, printed as `step 4m05s`);
    - the hand-off table follows :128-148, using `log.epic_polls()`, `log.orchestrator_reaction(lane)` and `log.merge(lane.name)`;
    - the resumed-children block follows :150-161, with `lane.stops`;
    - the Bob line follows :163-165.
  - Docstring (:1-31): the gaps are unchanged, now read through `stats`. The usage is `python3 timeline.py <trial>/artifacts/spektacular/logs [<trial>/artifacts/agent-logs/bob]`.
- `tests/harbor/epic-workflow/tests/test.sh:5`, `tests/harbor/goal-check-workflow/tests/test.sh:5`, `tests/harbor/implement-epic-workflow/tests/test.sh:6`, `tests/harbor/standing-rules/tests/test.sh:5`: `python3 /tests/timeline.py /app/.spektacular/logs ... || true` (already moved by 000073; confirm, and keep `|| true`).
- `tests/harbor/shared/tests/test_timeline.py` (new): over a fake `RunLog`, an implement-epic fixture with two lanes, one resumed, polls, a reaction and merges. It asserts the printed lane rows (start, finish, slowest step), each gap, the resumed block and the missing-logs message.
- `tests/harbor/shared/tests/test_runlog_integration.py` (extend): `timeline.report` over the fixture epic logs names both lanes and three gap columns.
- `tests/harbor/shared/tests/test_suite_wiring.py` (extend): every `timeline.py` call in a `test.sh` reads `/app/.spektacular/logs` and ends in `|| true`.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Cross-run report shows the per-step split

**File changes**
- `tests/harbor/shared/report.py` (budget about 400 lines):
  - Thresholds (:33-36): `IDLE_SHARE` becomes `GAP_SHARE = 0.25` (gap time over a quarter of wall time), and `IDLE_MARK` becomes `GAP_MARK = "  ◀ gaps"`.
  - Run table (:188-223): replace "idle m" with "user wait m" and "gap m".
  - `breakdown` (:375-431):
    - per workflow, a step table with columns `step, entries, seconds, share, working, helper wait, user wait, helpers`;
    - a run totals line (wall, user wait, helper wait, gaps over threshold);
    - a helpers table (`name, spec, step, seconds, end`) from `subagents.helpers`, replacing the children table (:286-350);
    - briefs listed by `--prompts` from `subagents.briefs`.
  - Add `compare_trials(a, b)` and the `--compare A B` argument:
    - per workflow, matched by `(kind, name)`, a step table with `A` and `B` columns and `B - A` for duration, working, helper wait, user wait and helpers;
    - steps in A's order, then B's new ones;
    - a missing step shows `-`;
    - then a totals table (wall, user wait, helper wait, gap, cost, total tokens).
  - `MEASURES` (:458-465): add `user wait m` (`totals.user_wait_s`), `helper wait m` (`totals.helper_wait_s`) and `gap m` (`totals.gap_s`).
  - Schema 1 and 2 records: missing fields print `unknown`, and the old `children`/`idle_s` keys are ignored.
  - `main` (:548-600): `--compare` excludes the other modes, with the existing style of usage error.
- `tests/harbor/shared/tests/test_report.py` (rewrite the idle and children sections at :1081-1369; extend the comparison tests at :703-1079):
  - the table columns and the gap mark;
  - the breakdown split and helpers;
  - `--compare` values, deltas and `-` for a missing step;
  - medians of the new measures;
  - a schema-2 record loading with unknowns.
- `README.md` "#### Comparing runs" (:561-602): document `--compare`, the new columns and the `◀ gaps` mark.
- `tests/harbor/shared/tests/test_readme_metrics.py` (:183-187, :200-234): `GAP_SHARE`/"quarter" and the `◀ gaps` mark replace the idle ones, and the section shows `--compare`.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: 2 parallel agents: (a) the table and breakdown; (b) `--compare` and the medians. Integrate sequentially with the README updates.

### Task: Remove the old parsers and debug fixtures

**File changes**
- Delete:
  - `tests/harbor/shared/sessionlog.py`;
  - `tests/harbor/shared/spans.py` (added by 000074);
  - `tests/harbor/shared/tests/test_sessionlog_windows.py`;
  - `tests/harbor/shared/tests/test_sessionlog_rules.py` (its rule cases are ported into `test_runlog.py`).
- Delete `tests/harbor/shared/tests/data/{m70-epic,s55-epic,s55-plan,s55-implement}/debug/`. Update each run's `README` to say its Spektacular logs predate the new format.
- `tests/harbor/shared/tests/test_reference_runs.py`:
  - the directory listings (:185, :300) drop `debug`;
  - remove the event-key test (:253-262);
  - `measured()` comparisons (:277-280) use the new record.
- `tests/harbor/shared/tests/trim_reference.py`:
  - `layout()` (:177-205) reads `artifacts/spektacular/logs` and copies its `*.jsonl` unchanged. Default-level lines hold no responses, and the whole directory stays under `MAX_RUN_BYTES`.
  - Remove `KEEP_EVENT`/`KEEP_RESPONSE` (:51-54) and the response trimming (:156-173).
  - `measured()` (:211-224) drops `plan_steps_entered`/`changelog_writes_by_repo` and uses the schema-3 record. When the trimmed directory holds new logs, it runs `metrics.build_record` with `log_dir` pointed at the copy.
- `tests/harbor/shared/transcripts.py`: remove whatever no remaining caller uses after the earlier tasks (check with grep across `tests/harbor`, excluding `.build` and `jobs`), for example `CODEX_OUTPUT` (:34), which has no users.
- `tests/harbor/shared/tests/test_suite_wiring.py` (extend): `test_shared_scripts_stay_small` sums the lines of `tests/harbor/shared/*.py` (not `tests/`) and requires `<= 2000`, with a message naming the 000076 baseline of 2,999.
- Repo-wide check: a grep for `sessionlog|SessionLog|spektacular/debug|spans import` in `tests/harbor` (excluding `.build`, `jobs` and the data READMEs) returns nothing. Then the full `uvx --with pytest pytest --import-mode=importlib tests/harbor` and `go test ./...` pass.

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution (deletions and fixture edits interact).

### Task: README and knowledge describe Harbor on built-in logs

**File changes**
- `README.md` (Harbor sections; 000073 moves them to `.spektacular/logs` and `verbose`):
  - "Test results" tree (:469-487): `spektacular/ # the project's .spektacular, with logs/ (Spektacular's run logs)`.
  - Task table (:489-497): the epic suites are "checked against CLI state and Spektacular's logs".
  - Session-log paragraph (:499-504): suites record at the default level with no override. Harbor reads the logs only through `spektacular stats` (`list`, `run`, `commands`, `spans`). The timeline is run with `python3 tests/harbor/shared/timeline.py <trial>/artifacts/spektacular/logs`. Running it locally needs a `spektacular` with `stats` on `PATH`, or `SPEKTACULAR_BIN`.
  - The hand-run metrics command (:601): `--log-dir <trial>/artifacts/spektacular/logs`.
  - Self-tests (:604-625): name the run-log reader tests, the integration test (it needs Go and skips without it) and the line-count check.
  - No em dashes.
- Knowledge `architecture/testing-architecture.md` (repo store `spektacular`): read with `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`.
  - Rewrite the epic-suite paragraph and the session-log reader bullet: Harbor reads Spektacular's logs only through `stats` at the default level, via `tests/harbor/shared/runlog.py`; the timeline is `tests/harbor/shared/timeline.py`.
  - Add the couplings: the `stats` JSON field names (pinned by Go golden files that the Python tests read), `WORKTREE_DIR` (tied to `internal/worktree.Dir` by a self-check) and the line-count ceiling.
  - Stage under `.spektacular/tmp/`, write with `spektacular knowledge write ... --from <path>`, and remove the staged file.
- Knowledge `gotchas/harbor-agent-logs-and-artifacts.md`: replace the "Spektacular's own session log comes back for every agent" paragraph. No image sets `SPEKTACULAR_SESSION_LOG`; every run records at the default level to `.spektacular/logs/` (run files plus `standalone_<day>.jsonl`); the `/app/.spektacular` artifact carries them; Harbor reads them through `stats`. Written the same way.
- Knowledge `gotchas/harbor-agent-log-fields.md`: the last line names the remaining readers (`transcripts.py`: totals, children, turns, wake-ups, launches). Add one sentence: step, helper and wait timing no longer come from transcripts. Written the same way.
- Leave `CHANGELOG.md`, prior plans and the investigation report unchanged.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Each task's tests, at task granularity:
- **List commands and spans in the run statistics package**: `internal/runstats` table tests over hand-built records (two runs plus standalone records, ordering, null run fields, argv and exit-code fallbacks, selection, outside-only, the combined-selection refusal, capped open spans); new `commands.golden.json` and `spans.golden.json`; 000075's golden files unchanged.
- **List commands and spans from the command line**: `cmd` tests over the seeded fixture project (both formats agreeing, `--fields`, `--outside-runs`, refusal with next action, `--dir` copy, byte-identical logs).
- **Documentation site describes the two listings**: `npm run build`, `npx astro check`, the MDX layout guard and an em-dash grep.
- **Record baseline Harbor pass rates**: a person checks five trials per suite exist under the baseline prefix, each with a reward.
- **Pin today's token, model and cost figures**: `test_reference_runs.py` pins the transcript totals per reference run, and passes on the unchanged harness.
- **Harbor run log reader**: `test_runlog.py` over canned JSON (000075 golden files plus this plan's), covering every `Unavailable` cause and every ported session-log rule; `test_runlog_integration.py` with a throwaway binary over fixture logs, skipping without Go.
- **Metrics record from Spektacular's logs**: `test_metrics.py` with a fake `RunLog` (schema 3 keys, the split, helpers including an outside one, multi-run sums, unknowns with no transcript fallback, section isolation, the untouched reward, three agents giving identical log sections); reference-run totals still equal the pins; integration steps equal `stats run`; README table test.
- **Delegation checks judge from spans**: `test_delegation.py` over hand-built spans and lanes (attempts, resumed entries, the last step not judged, per-spec lanes, brief joins, the worktree folder); a self-check of `WORKTREE_DIR` against `internal/worktree`; the oracle and trace-oracle template ties; `test_epic_children.py` with fake readers and the unchanged AST guard.
- **Suite verifiers read the logs through stats**: every suite self-check against fake readers; the goal-check self-check rebuilt on `runlog.Command`.
- **Suite images record at the default level**: wiring tests (no image sets the variable; every binary suite collects `/app/.spektacular`; metrics last).
- **Epic timeline from Spektacular's logs**: `test_timeline.py` over a fake reader (lanes, slowest step, gaps, resumes, missing logs); integration over the fixture epic logs; wiring test for the `test.sh` calls.
- **Cross-run report shows the per-step split**: `test_report.py` (table, gap mark, breakdown, `--compare`, new medians, schema-2 records); README test.
- **Remove the old parsers and debug fixtures**: the full `pytest tests/harbor`; the line-count self-test; a grep showing no reference to the old reader; `go test ./...`.
- **README and knowledge describe Harbor on built-in logs**: README tests already updated; the knowledge entries read back through the CLI.

Success metrics and manual reviews are classified in plan.md's Testing Approach. The manual Harbor runs and the before-and-after pass-rate comparison go to the implementation test plan.

## Project References

- Spec `000076_harbor-on-built-in-logs`, epic `000002_workflow-tracing-and-run-stats`.
- Upstream plans:
  - `000073_log-mode-and-logger-providers`: record format, default-level attributes, logs directory, retention, `SPEKTACULAR_SESSION_LOG`, its Harbor reader;
  - `000074_trace-spans-for-agent-work`: span records, end reasons, parents, orchestrator spans outside runs, its Harbor span reader and checks;
  - `000075_run-stats-command`: the `stats` JSON contract, `internal/runstats`, logging exemption, golden files.
- Concurrent plan: `000077_html-log-viewer`, which also extends `internal/runstats` and `cmd/stats.go`.
- Design documents: none.
- Knowledge entries applied:
  - `conventions/error-messages-must-suggest-remediation.md`;
  - `conventions/tests-must-not-depend-on-order.md`;
  - `conventions/tests-must-pass-for-done.md`;
  - `conventions/plans-never-change-the-active-install.md`;
  - `conventions/store-files-must-be-written-through-the-cli.md`;
  - `architecture/testing-architecture.md`, `gotchas/harbor-agent-logs-and-artifacts.md` and `gotchas/harbor-agent-log-fields.md` (all three updated by this plan);
  - the docs repo's `conventions/plan-content-pages.md`, `conventions/no-em-dashes.md` and `conventions/mdx-authoring.md`.
- Reference material: `docs/workflow-performance-investigation.md` (idle and per-step split recommendations).
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

This plan, task by task:

| Task | Tier | Tokens |
|------|------|--------|
| List commands and spans in the run statistics package | Medium | ~25k |
| List commands and spans from the command line | Low | ~12k |
| Documentation site describes the two listings | Low | ~6k |
| Record baseline Harbor pass rates | person | n/a |
| Pin today's token, model and cost figures | Low | ~6k |
| Harbor run log reader | Medium | ~30k |
| Metrics record from Spektacular's logs | High | ~50k |
| Delegation checks judge from spans | Medium | ~35k |
| Suite verifiers read the logs through stats | Medium | ~35k |
| Suite images record at the default level | Low | ~8k |
| Epic timeline from Spektacular's logs | Low | ~15k |
| Cross-run report shows the per-step split | Medium | ~25k |
| Remove the old parsers and debug fixtures | Medium | ~20k |
| README and knowledge describe Harbor on built-in logs | Low | ~10k |

## Migration Notes

- `metrics.json` moves from schema 2 to schema 3. `report.py` still reads schema 1 and 2 records from older jobs and shows the new figures as unknown. No collected job is rewritten.
- The checked-in reference runs lose their old-format `debug/` logs, and their log-sourced pins are removed. Their transcript pins stay.
- No settings format, state field or log record changes. Nothing is migrated in any project. Installing the new binary is left to the maintainer between workflows (plans-never-change-the-active-install).

## Performance Considerations

- Each `stats` call is one subprocess over a small trial log directory, so it costs tens of milliseconds. The recorder makes one `list`, one `spans` and one `run` per workflow run; a verifier module makes a handful of calls, cached per reader. A 60-second timeout per call keeps a hung binary from stalling the verifier, and the timeout becomes an unknown, not a failure.
- Default-level logs are small, holding no responses, so reading and trimming them is cheap. The trimmer copies them whole within the existing `MAX_RUN_BYTES`.
- Removing transcript message parsing makes the recorder faster on long Claude transcripts.
