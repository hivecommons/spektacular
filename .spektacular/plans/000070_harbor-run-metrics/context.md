---
created_date: "2026-10-23"
document_status: final
closed_date: "2026-10-23"
---

# Context: 000070_harbor-run-metrics

## Current State Analysis

- A harbor run today reports only pass or fail. Every suite's `tests/test.sh` runs pytest and writes `1` or `0` to `/logs/verifier/reward.txt` (e.g. `tests/harbor/plan-workflow/tests/test.sh:4-10`). Harbor's `result.json` `agent_result` has null tokens and cost for every Claude and Bob job on disk.
- The raw data already exists in every run:
  - Claude's stream-json `/logs/agent/claude-code.txt` (`result` events with totals and `subagent_stats`; timestamped messages; `system/task_notification` with per-sub-agent `duration_ms`);
  - the ATIF `trajectory.json` harbor writes for Bob (no timestamps, tokens or cost) and for Codex;
  - Spektacular's session log under `/app/.spektacular/debug/` (one record per command with `state_before`/`state_after`; `internal/sessionlog/record.go:19-55`).
- Shared helpers `tests/harbor/shared/transcripts.py`, `sessionlog.py` and `timeline.py` parse these. Only the epic, standing-rules, implement-epic and goal-check render targets copy them into a suite (`Makefile:203,242,288,330`) and have `python3` in their images. Spec, plan, repo and implement do neither, and `harbor-test-implement` (`Makefile:173-178`) runs harbor from the source directory.
- Earlier analysis of these logs was done ad hoc with throwaway scripts. The reference Claude plan run `tests/harbor/jobs/epic-claude-plan/` took 15.8 min, 85 turns and $3.51, with the `assemble` step 5.9 min of that.
- Collected job directories contain root-owned files under `agent/sessions/`, unreadable on the host. `verifier/` and `result.json` are readable.

## Per-Task Technical Notes

### Requirement → repo and files

Every requirement lands in the `spektacular` repo (root `/home/nicj/code/github.com/hivecommons/spektacular`). The `docs` repo is untouched: the website is a spec non-goal.

| Requirement | Files |
|---|---|
| Standard record per run; totals; step breakdown; sub-agents; check score; unknowns | `tests/harbor/shared/transcripts.py`, `tests/harbor/shared/sessionlog.py`, `tests/harbor/shared/metrics.py` (new) |
| Record collected with every suite | `Makefile`, `tests/harbor/*/tests/test.sh` (8), `tests/harbor/{spec,plan,repo,implement}-workflow/environment/Dockerfile` |
| Local report: list, single run, compare, flags | `tests/harbor/shared/report.py` (new), `Makefile` (`harbor-report`) |
| Documentation | `README.md` (End-to-end (Harbor) section) |
| Self-checks | `tests/harbor/shared/tests/` (new) |

### Task: Read run summaries from every agent log

**File changes**

- `tests/harbor/shared/transcripts.py:1-13` — extend the module docstring. It now also reads run summaries: Claude `result` events, the ATIF trajectory harbor writes for ACP agents and Codex, and the Codex rollout fallback. Keep `AGENT_LOG_DIR`, `CLAUDE_TRANSCRIPT`, `ACP_TRAJECTORY` (`:20-23`) and add `CODEX_OUTPUT = AGENT_LOG_DIR / "codex.txt"` and `CODEX_SESSIONS = AGENT_LOG_DIR / "sessions"`.
- `tests/harbor/shared/transcripts.py:26-35` — leave `SubagentLaunch` and the existing functions (`read_jsonl`, `claude_main_models`, `claude_subagent_launches`, `acp_steps`, `acp_subagent_launches`) unchanged in signature and behaviour. They are imported by `tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py:23-24` and the goal-check verifier.
- `tests/harbor/shared/transcripts.py:112` (append) — new dataclasses:
  - `Unknown(field: str, reason: str)`;
  - `Totals`, with `wall_time_s`, `model_time_s`, `turns`, `cost_usd`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `model` (each `None` when unknown), plus `unknowns: list[Unknown]`;
  - `Message(at: datetime | None, chars: int, parent: str | None, order: int, goto_step: str | None)`;
  - `Subagents(started, completed, failed, stopped, durations_s, unknowns)`.
- `tests/harbor/shared/transcripts.py` (append) — `claude_summary(objs) -> Totals`:
  - Collect every `type == "result"` object in order.
  - With exactly one: `wall_time_s = duration_ms/1000` and `turns = num_turns`.
  - With several (background sub-agents split the run): `turns = sum(num_turns)`, and `wall_time_s` = last minus first `timestamp` over every object that has one. Parse with `datetime.fromisoformat(s.replace("Z", "+00:00"))`.
  - From the last result: `model_time_s = duration_api_ms/1000` and `cost_usd = total_cost_usd`. Tokens are sums over `modelUsage.*`: `inputTokens`, `outputTokens`, `cacheReadInputTokens`, `cacheCreationInputTokens`. `model` = the `modelUsage` key(s) joined with `+`, else the `system/init` `model`.
  - With no result event, every field is `None` and each gets an `Unknown` with reason "no result event in claude-code.txt".
  - The single-versus-several rule was confirmed by the user. Evidence: `jobs/epic-claude-epic` has 4 results summing to 88 s against a 1094 s span.
- `tests/harbor/shared/transcripts.py` (append) — `claude_messages(objs) -> list[Message]`:
  - One `Message` per `type == "assistant"` object, `at` from `timestamp`, `parent` from `parent_tool_use_id`.
  - `chars` sums each content block: `len(text)` for text, `len(thinking)` for thinking, `len(json.dumps(input))` for tool_use.
  - `goto_step` is set when a tool_use's `input.command` contains `spektacular` and ` goto `; the step comes from its `--data` JSON `step`, parsed the same way as `sessionlog.Event.data`.
- `tests/harbor/shared/transcripts.py` (append) — `claude_subagents(objs, summary_result) -> Subagents`:
  - Counts from the last result's `subagent_stats`: `started = spawned`, `completed`, `failed`, `stopped = sum(killed.values())`.
  - `durations_s` from each `system/task_notification` `usage.duration_ms/1000`, in event order.
  - A launch with no notification gets no duration, and a `Unknown("subagents.durations_s", "n sub-agents never reported a duration")` is added.
  - Without `subagent_stats`, fall back to counting `claude_subagent_launches` (`:66-90`) and notifications by `status`.
- `tests/harbor/shared/transcripts.py` (append) — `claude_launch_root(objs) -> dict[str, str]`: maps every sub-agent message's `parent_tool_use_id` to its top-level launch id. A nested launch is a tool_use inside a message that itself has a parent, so walk up the chain. Used for lane attribution.
- `tests/harbor/shared/transcripts.py` (append) — ATIF readers over `json.loads(trajectory.json)`:
  - `atif_summary(traj) -> Totals`: `turns` = number of `source == "agent"` steps.
  - Tokens from `final_metrics.total_prompt_tokens` / `total_completion_tokens` / `total_cached_tokens`, and `cost_usd` from `final_metrics.total_cost_usd`. Each is `None` with an `Unknown` naming the agent (`traj["agent"]["name"]`) when absent; Bob has only `final_metrics.total_steps`, as in `jobs/2026-10-16__10-19-11/.../trajectory.json`.
  - `wall_time_s` = span of step `timestamp`s when they exist. When steps have none (Bob), it is left `None` here and filled by the recorder from the Bob shell log or session-log span.
  - `model` = `agent.model_name` when present.
- `tests/harbor/shared/transcripts.py` (append) — `atif_messages(traj) -> list[Message]`: one per agent step. `chars` = `len(message)` + `len(reasoning_content or "")` + `len(json.dumps(tool_call.arguments))` per tool call. `at` comes from `timestamp` or `None`. `goto_step` is taken from a tool call whose `arguments.command` runs a `spektacular … goto`. `order` = the step index.
- `tests/harbor/shared/transcripts.py` (append) — `codex_rollout_summary(sessions_dir) -> Totals`, the fallback when no trajectory is readable. Read the newest `rollout-*.jsonl` under `sessions/`. `turns` = the count of `event_msg` payloads of type `token_count`. Tokens come from the last `token_count` `info.total_token_usage` (`input_tokens`, `output_tokens`, `cached_input_tokens`). The model comes from `turn_context.payload.model`. `wall_time_s` is the first-to-last line `timestamp`. Cost is unknown ("codex reports no cost").
- `tests/harbor/shared/tests/test_transcripts_summary.py` (new) — self-checks on synthetic stream-json lines and trajectories, with `sys.path.insert` of the parent directory as in `tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py:19-20`:
  - a single-result run whose totals equal the result's fields;
  - a three-result run, checking the summed turns, last-event cost and tokens, and span wall time;
  - sub-agent counts and durations, plus a missing notification;
  - a nested launch resolving to its root;
  - a Bob trajectory with null tokens and cost;
  - a Codex trajectory with tokens;
  - a rollout fallback;
  - the existing `claude_subagent_launches` output unchanged on the same input.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 2 parallel agents. One writes the Claude readers and their self-checks, the other the ATIF and rollout readers and theirs. A final sequential pass reconciles the shared dataclasses.

### Task: Turn the session log into step windows

**File changes**

- `tests/harbor/shared/sessionlog.py:31-75` — leave `Event` unchanged.
- `tests/harbor/shared/sessionlog.py:115-196` — add `StepWindow(step: str, start: datetime, end: datetime)`, a dataclass with `duration_s`, and add `Lane.step_windows() -> list[StepWindow]`:
  - Walk `self.segments` (`:127`) in order. Within a segment, walk events in time order.
  - An event whose `state_after.current_step` differs from the open window's step closes that window at `event.at` and opens a new one at `event.at`. When `state_after` is missing, use `step_after` (`:68-73`).
  - Each segment's last open window closes at that segment's last event's `end`, including the terminal `finished` step.
  - A new segment always opens a fresh window, so a resumed lane shows separate windows rather than one spanning the gap.
  - Failed commands (`exit_code != 0`) do not move the step.
- `tests/harbor/shared/sessionlog.py:198-230` — no change to `SessionLog`. The recorder reads `SessionLog(debug_dir).lanes`. A run with no lane file but shared events (a workflow run without a lane name) is out of scope. Every suite's workflow runs file lanes, as `jobs/epic-claude-plan/.../debug/*_plan_*.jsonl` shows.
- `tests/harbor/shared/tests/test_sessionlog_windows.py` (new) — self-checks with fixed timestamps:
  - a plan lane: overview → … → finished, every step in order, durations summing to the lane span;
  - a failed goto that does not move the step;
  - a resumed lane with two segments;
  - `Lane.stops`/`finish` unchanged.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution

### Task: Build and write the run metrics record

**File changes**

- `tests/harbor/shared/metrics.py` (new) — the recorder. Module docstring as in `tests/harbor/shared/timeline.py:1-31`: what it writes and that it never fails the run. It is `sys.path`-relative like `timeline.py:39`, importing `transcripts` and `sessionlog`. The structure:
  - `SCHEMA_VERSION = 1`. Defaults: `BUILD_INFO = /tests/build.json`, `AGENT_DIR = /logs/agent`, `DEBUG_DIR = /app/.spektacular/debug`, `BOB_LOGS = /root/.bob/logs`, `JUNIT = /logs/verifier/junit.xml`, `REWARD = /logs/verifier/reward.txt`, `OUT = /logs/verifier/metrics.json`. `argparse` overrides each, for local runs and self-checks.
  - `read_build_info(path) -> (dict, list[Unknown])`. A missing file gives `suite`, `agent`, `spektacular_version` and `commit` as `None`, each with reason "no build.json (suite not run through its make target)". `scenario` defaults to `None` with no unknown.
  - `pick_reader(agent_dir, build_agent)`:
    - `claude-code.txt` present → claude;
    - else `trajectory.json` present → atif (agent name from the trajectory);
    - else a rollout under `sessions/` → codex rollout;
    - else none. An oracle run (no agent log) yields all totals unknown with reason "no agent log (oracle or missing)".
    - A reader that disagrees with `build_agent` adds an `Unknown("run.agent", …)` and the build agent is kept.
  - `score_checks(junit_text) -> (checks, groups)`:
    - Parse with `xml.etree.ElementTree` and walk every `testcase`.
    - Skipped cases are excluded. Failed or errored cases count toward `total`, and the rest are `passed`.
    - Group = the `classname`'s last dotted component when it starts with `Test`, otherwise the whole `classname` (the module, for function-style suites such as `tests/harbor/goal-check-workflow/tests/test_goal_check.py` and `tests/harbor/implement-workflow/tests/test_implement_workflow.py`).
    - `score = passed/total`, or `None` when `total == 0`.
  - `read_reward(path) -> int | None` reads `reward.txt` and never writes it.
  - `step_breakdown(lanes, messages, root_of, launches, agent) -> (dict, unknowns)`:
    - Per lane, `{kind, name, steps:[{step, started_at, duration_s, output}]}` from `Lane.step_windows()`.
    - **One lane, timestamped messages** (Claude, Codex): `output` = sum of `chars` of all messages, main and sub-agent, with `start <= at < end`.
    - **One lane, untimestamped messages** (Bob): walk messages in `order`. The current step starts at the lane's first window and advances when a message's `goto_step` names the next window's step. Its chars go to the current step.
    - **Several lanes, Claude**: a sub-agent message goes to the lane whose `name` appears in its root launch's prompt (`root_of` + `claude_subagent_launches` prompts), in the window containing its `at`. Main-agent messages go to `orchestrator_output`, a top-level number in `steps`.
    - **Several lanes, other agents**: `output = None` with `Unknown("steps.output", "<agent> output cannot be split between concurrent lanes")`.
    - No lanes: `workflows: []` with `Unknown("steps", "no workflow lane in the session log")`.
  - `wall_time_fallback(...)`: when the reader leaves `wall_time_s` unknown (Bob), use the first-to-last `ts` in the Bob shell logs under `BOB_LOGS` when present, else the session log's first-to-last event span. The `Unknown` entry is removed only when a fallback succeeds.
  - `build_record(...) -> dict` assembles the shape in plan.md § Data Structures. `unknown` is the concatenation of every gatherer's `Unknown`s as `{field, reason}`.
  - `main(argv) -> int` wraps each gatherer in its own `try/except Exception`; a failure becomes `Unknown(<section>, "recorder error: <exc>")`. It writes `OUT` atomically (temp file plus `os.replace`), `chmod 0o644`, prints a one-line summary to stdout, and **always returns 0**. It never opens `REWARD` for writing.
- `tests/harbor/shared/tests/test_metrics.py` (new) — self-checks building a temp `/logs`-like tree:
  - a Claude plan run: record keys equal the fixed top-level set, totals match the result event, steps in order with durations summing to at most the wall time, and nothing unknown under totals or steps;
  - a Bob run: tokens and cost null with reasons, wall time and turns present, chars attributed by goto order;
  - a Codex run: sub-agents null;
  - an oracle run;
  - a missing `claude-code.txt`, malformed JSON lines, a missing JUnit file, a missing debug dir and a missing build.json: each still writes a record with the affected fields unknown, and the exit code is 0;
  - a reward file that is byte-identical before and after;
  - JUnit scoring of class and module groups with a skip;
  - a two-lane Claude run attributing sub-agent chars by launch prompt.

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis, sequential integration. Write `score_checks` and `read_build_info` with their self-checks in parallel with `step_breakdown` and its self-checks, then integrate `build_record`/`main` and the end-to-end self-checks sequentially.

### Task: Wire the recorder into every harbor suite

**File changes**

- `Makefile:22-28` (after `AGENT_ARTIFACTS`) — add `define write_build_info`, called as `$(call write_build_info,<tests dir>,<suite>,<scenario or empty>)`. It writes `<tests dir>/build.json` with `printf` from:
  - `$(AGENT)` (empty for oracle-style direct runs is fine);
  - suite and scenario (`null` when empty);
  - `git describe --tags --always`;
  - `git rev-parse HEAD`;
  - `dirty` = `true` when `git status --porcelain --untracked-files=no` prints anything.
- `Makefile:83-99` `_harbor-test-spec` — after the `cp` of `test_spec_workflow.py` (`:94`), add `cp tests/harbor/shared/*.py tests/harbor/.build/spec-workflow-$(AGENT)/tests/` and `$(call write_build_info,tests/harbor/.build/spec-workflow-$(AGENT)/tests,spec-workflow,)`.
- `Makefile:116-136` `_harbor-test-repo` — after `:127`, copy the shared helpers into `.build/repo-workflow-$(SCENARIO)/tests/` and write build info with suite `repo-workflow` and scenario `$(SCENARIO)`.
- `Makefile:156-171` `_harbor-test-plan` — after the `cp -r` (`:160-161`), copy the shared helpers into `.build/plan-workflow-$(AGENT)/tests/` and write build info (`plan-workflow`).
- `Makefile:173-178` `harbor-test-implement` — replace it with a render target modelled on `_harbor-test-plan` (`:156-171`):
  - `rm -rf` and `mkdir tests/harbor/.build/implement-workflow-claude`;
  - `cp -r tests/harbor/implement-workflow/environment tests/harbor/implement-workflow/tests tests/harbor/implement-workflow/task.toml tests/harbor/implement-workflow/instruction.md` into it;
  - copy the shared helpers, write build info (`AGENT=claude`, `implement-workflow`), and build the binary into its `environment/`;
  - `$(HARBOR_AUTH) harbor run $(HARBOR_RUN_FLAGS) -p tests/harbor/.build/implement-workflow-claude -a claude-code -m $(HARBOR_MODEL) -o tests/harbor/jobs`;
  - `cat` the newest `implement-workflow-claude__*/verifier/test-stdout.txt`.

  Set `AGENT := claude` on the target (`harbor-test-implement: AGENT=claude`) so `write_build_info` sees it. Check that `implement-workflow/environment/Dockerfile` copies nothing the render misses (it is built from `environment/` only).
- `Makefile:196-216` `_harbor-test-epic`, `:233-251` `_harbor-test-standing-rules`, `:281-297` `_harbor-test-implement-epic`, `:321-341` `_harbor-test-goal-check` — they already copy the shared helpers (`:203`, `:242`, `:288`, `:330`). Add the `write_build_info` call to each: suite `epic-workflow`, `standing-rules`, `implement-epic-workflow` (scenario `$(VARIANT)` without its leading `-` when set), `goal-check-workflow` with scenario `$(SCENARIO)`.
- `Makefile:30` `.PHONY` — add `harbor-report`. The report target itself is added in Task "Report runs and step breakdowns".
- `tests/harbor/spec-workflow/environment/Dockerfile:3-5`, `tests/harbor/plan-workflow/environment/Dockerfile:3-5`, `tests/harbor/repo-workflow/environment/Dockerfile:3-5`, `tests/harbor/implement-workflow/environment/Dockerfile:3-5` — add `python3` to the `apt-get install` list, as in `tests/harbor/epic-workflow/environment/Dockerfile:4`.
- `tests/harbor/spec-workflow/tests/test.sh:4`, `plan-workflow/tests/test.sh:4`, `implement-workflow/tests/test.sh:4`, `repo-workflow/tests/test.sh:4`, `epic-workflow/tests/test.sh:7`, `standing-rules/tests/test.sh:7`, `goal-check-workflow/tests/test.sh:7`, `implement-epic-workflow/tests/test.sh:8`:
  - Append `--junitxml=/logs/verifier/junit.xml` to the existing `pytest` line. Leave the `if [ $? -eq 0 ]` reward block untouched, because it still reads pytest's status.
  - After the reward block, append a commented `python3 /tests/metrics.py || true`. The comment says the record goes beside the reward, runs after it is written, and never fails the run.
  - goal-check and implement-epic pass `--bob-logs /root/.bob/logs`, which is also the default.
- `.gitignore:52` — keep `tests/harbor/*/environment/spektacular`. No source-tree build output is produced any more for implement, but older checkouts may still hold one.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: 2 parallel agents. One does the Makefile changes, the other the eight `test.sh` and four Dockerfile edits. Verify sequentially by running each render target up to just before `harbor run` (or `make -n`) and checking the `.build/<suite>/tests/` contents.

### Task: Report runs and step breakdowns

**File changes**

- `tests/harbor/shared/report.py` (new) — stdlib only, `#!/usr/bin/env python3`. Module docstring with usage in the style of `tests/harbor/shared/timeline.py:1-31`:
  - Thresholds as named constants at the top: `DOMINANT_STEP_SHARE = 1/3`, `REGRESSION_RATIO = 2.0`, `SCORE_DROP = 0.1`.
  - `Run` dataclass: trial dir, trial name, job name, started_at, reward (from `result.json`'s `verifier_result.rewards.reward`, falling back to the record's `result.reward`), and the record.
  - `load_runs(paths) -> list[Run]`: each argument is a directory or a glob (expand with `glob.glob`). Find `*/verifier/metrics.json` and `*/*/verifier/metrics.json` beneath it, so a job dir and a trial dir both work. Default `tests/harbor/jobs`.
    - Only `verifier/metrics.json` and the trial's `result.json` are opened, never `agent/`. An unreadable file is skipped with a one-line warning on stderr.
    - Sort by `started_at` (`result.json` `started_at`).
  - `table(runs)`: group by `(suite, scenario)` with a header per group. Columns: trial, agent, model, wall (m), turns, out tok, cache-read tok, cost $, checks P/N (score), result. A `None` value renders as `unknown`.
  - `find_run(runs, key)`: matches a trial dir path, or a trial-name prefix such as `plan-workflow-claude__f5mLhau`. Ambiguity is an error naming the matches and asking for the full trial dir.
  - `breakdown(run)`: per workflow lane, rows of step, duration (s), share of the run's wall time (%), output (characters), and a `◀ dominant` marker when share > `DOMINANT_STEP_SHARE`. The marker is computed by `dominant_steps(run) -> list[(lane, step, share)]`, using `totals.wall_time_s`, else the lane's summed durations when the wall time is unknown. Then the sub-agent counts and durations, and the `unknown` list with reasons.
  - `main(argv)` takes positional paths (table), `--run KEY` (breakdown), and `--baseline …`/`--candidate …` (added in Task "Compare baseline and candidate runs"; argparse is shaped for it now). Exit 0. A usage or lookup error exits 2 with a message saying what to pass instead.
- `Makefile` (after `harbor-install`, `:68-70`) — `harbor-report:` runs `python3 tests/harbor/shared/report.py $(ARGS)`, commented with the three invocations.
- `tests/harbor/shared/tests/test_report.py` (new) — self-checks over temp job trees holding hand-written records and `result.json`:
  - two suites and a trial without a record: rows grouped, the record-less trial skipped;
  - unknown printed as `unknown`;
  - a plan run with `assemble` at 354 of 948 s flagged and a step at 0.33 of wall time not flagged;
  - `--run` by prefix and an ambiguous prefix;
  - an `agent/` subdirectory made unreadable (`chmod 000`) that does not stop the report. Note `gotchas/chmod-test-sabotage-is-ignored-by-root`: skip that case when running as root.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution

### Task: Compare baseline and candidate runs

**File changes**

- `tests/harbor/shared/report.py` — add `compare(baseline_runs, candidate_runs) -> str` and `regressions(...) -> list[Flag]`:
  - Group each set by `(suite, scenario)`. For every group present in both, compute `statistics.median` over known values of `wall_time_s`, `turns`, `output_tokens`, total tokens (input + output + cache read + cache write, known only when all four are), `cost_usd` and `checks.score`.
  - Print baseline, candidate, change (absolute and ×ratio; points for score) and a `◀ regression` marker:
    - `candidate >= REGRESSION_RATIO * baseline` for the five cost-like measures;
    - `baseline_score - candidate_score >= SCORE_DROP` for the score.
  - A measure with no known value in either set prints `unknown` and is never flagged. A zero baseline never flags a ratio.
  - Groups in only one set are listed under "unmatched".
  - Wire `--baseline`/`--candidate` (each `nargs="+"`, dirs or globs, both required together) in `main`.
- `tests/harbor/shared/tests/test_report.py` — add comparison self-checks:
  - medians over three runs per set;
  - ratio exactly 2.0 flagged and 1.99 not;
  - score drop 0.10 flagged and 0.09 not;
  - an unknown on one side not flagged;
  - unmatched suites listed;
  - globs expanding to several job dirs.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution

### Task: Document run metrics and the report with the harbor suites

**File changes**

- `README.md:462-476` (Test results tree) — add `junit.xml # pytest results, one testcase per check` and `metrics.json # the run's metrics record (see Run metrics below)` under `verifier/`.
- `README.md:488-493` (after the session-log paragraph) — add a `#### Run metrics` subsection:
  - The record is written by every suite's verifier after the reward, never changes it, and is reached through the make targets, which supply its build info.
  - A field table for `run`, `result`, `totals`, `steps` (unit characters, per lane, `orchestrator_output`), `subagents` and `unknown`.
  - Which fields are unknown per agent: Bob has no tokens, cost or model. Codex has no sub-agents or cost. Oracle has no agent totals.
  - Claude's wall-time rule with background sub-agents.
- `README.md` (same subsection) — `#### Comparing runs`:
  - `make harbor-report`, `python3 tests/harbor/shared/report.py [jobs…]`, `--run <trial>`, and `--baseline <dirs|globs> --candidate <dirs|globs>`;
  - what each flag means (a step over a third of the run; a measure at least 2× baseline; a check score down 0.1 or more);
  - that the report reads only `verifier/` and `result.json`, so it needs no `sudo` despite root-owned agent files.
- `README.md:495-504` (self-check paragraph) — add the shared helpers' self-checks: `python3 -m pytest tests/harbor/shared/tests`.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution

## Testing Strategy

Per task:

- **Read run summaries from every agent log** — `tests/harbor/shared/tests/test_transcripts_summary.py`:
  - synthetic stream-json covering a single result, several results, sub-agent notifications (including a missing one) and nested launches;
  - ATIF trajectories for Bob (no tokens) and Codex (tokens), and a Codex rollout fallback;
  - a regression check that `claude_subagent_launches` and `acp_subagent_launches` return what they did before.
- **Turn the session log into step windows** — `tests/harbor/shared/tests/test_sessionlog_windows.py`:
  - a full plan lane in order, with durations summing to the lane span;
  - a failed goto that does not move the step;
  - a resumed lane giving separate windows;
  - `Lane.stops`/`finish` unchanged.
- **Build and write the run metrics record** — `tests/harbor/shared/tests/test_metrics.py`, over temp `/logs`-like trees:
  - Claude, Bob, Codex and oracle records with identical top-level keys;
  - Claude totals equal the summary, and nothing unknown under totals or steps;
  - Bob and Codex unknowns as the spec requires;
  - every missing or broken input still writing a record with exit 0 and an unchanged reward file;
  - JUnit class and module grouping with skips excluded;
  - two-lane attribution.
- **Wire the recorder into every harbor suite** — no new automated test (the harness is not in CI). Verify by rendering each suite (`make -n` or running the render target up to `harbor run`) and inspecting `.build/<suite>/tests/` for `metrics.py`, `transcripts.py`, `sessionlog.py` and `build.json`, then by the manual end-to-end runs in the test plan.
- **Report runs and step breakdowns** — `tests/harbor/shared/tests/test_report.py`:
  - grouped table, and record-less trials skipped;
  - unknown rendered as `unknown`;
  - dominant-step flag above and just below a third;
  - `--run` lookup by prefix, and an ambiguous prefix;
  - unreadable `agent/` (skipped as root).
- **Compare baseline and candidate runs** — `tests/harbor/shared/tests/test_report.py`:
  - medians;
  - ratio flag at 2.0 and not at 1.99;
  - score flag at a 0.10 drop and not at 0.09;
  - unknowns never flagged;
  - unmatched suites;
  - glob expansion.
- **Document run metrics and the report with the harbor suites** — manual: the README commands run as written (in the test plan).

Across all tasks:
- `go test ./...` must still pass. If root-owned files under `tests/harbor/jobs/` break it, resolve that or raise it with the user (convention *tests-must-pass-for-done*).
- The existing verifier self-checks (`tests/harbor/{goal-check,repo,standing-rules}-workflow/tests/test_verifier_selfcheck.py`) must still pass.
- The success metrics and manual reviews are classified in plan.md § Testing Approach. Every **Manual — captured in the implementation test plan** item there goes into the implement workflow's test plan.

## Project References

- Spec: `000070_harbor-run-metrics` (read with `spektacular spec file read 000070_harbor-run-metrics`). Source: GitHub issue #90.
- Design documents: none referenced by the spec.
- Knowledge (repo `spektacular`):
  - `architecture/testing-architecture.md` — harbor suites and their hand-maintained couplings;
  - `gotchas/harbor-agent-logs-and-artifacts.md` — what each agent leaves under `agent/`, the session log in every suite, and root-owned job files;
  - `gotchas/chmod-test-sabotage-is-ignored-by-root.md` — skip permission-sabotage self-checks as root;
  - conventions `tests-must-pass-for-done`, `tests-must-not-depend-on-order`, `plans-never-change-the-active-install`.
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular` (all work). `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website` (untouched).
- Reference runs on disk: Claude plan `tests/harbor/jobs/epic-claude-plan/plan-workflow-claude__f5mLhau/`; Claude multi-result epic `tests/harbor/jobs/epic-claude-epic/`; Bob plan `tests/harbor/jobs/2026-10-16__10-19-11/plan-workflow-bob__ZPi4iq6/`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

This plan: step windows, comparison and docs are Low. Readers, wiring and report are Medium. The recorder is High. Total is ~165k tokens. Load `tests/harbor/shared/*.py` and one reference job's logs once per agent, rather than whole job directories: Claude transcripts are 1 MB or more.

## Migration Notes

- No data or configuration migration. Existing job directories without a record are simply skipped by the report.
- `harbor-test-implement` now renders into `tests/harbor/.build/implement-workflow-claude`, so its trials are named `implement-workflow-claude__*` instead of `implement-workflow__*`. Anyone comparing against older implement runs should group them by suite through the record, not by trial-name prefix.
- The spec, plan, repo and implement images gain `python3`, so the first run after this change rebuilds those images.

## Performance Considerations

- The recorder runs once per trial after the checks, streaming a transcript of a few MB and a session log of a few hundred KB, so it adds seconds at most to a 15–25 minute run. Its time is not part of the agent's measured wall time.
- The report opens only small `metrics.json` and `result.json` files, so listing dozens of jobs is near-instant. It never reads multi-MB transcripts.
