---
created_date: "2026-10-23"
document_status: final
closed_date: "2026-10-23"
---

# Research: 000070_harbor-run-metrics

## Alternatives considered and rejected

- **pytest plugin (`conftest.py` with `pytest_sessionfinish`) writing the record.** Rejected: it runs inside the pytest process whose exit status sets `reward.txt` (`tests/harbor/*/tests/test.sh`), so a metrics bug could change the reward and break the spec's first constraint. It would also need a conftest per suite. A separate process run after the reward is written cannot affect the reward.
- **Compute the record on the host after `harbor run`, in a Makefile post-step or at report time.** Rejected: the spec's Technical Approach puts the record beside the reward, written by the verifier. A host step misses runs started outside the Makefile. Host-side reading also hits the root-owned files under `agent/sessions/` (knowledge `gotchas/harbor-agent-logs-and-artifacts.md`).
- **Use harbor's own `result.json` `agent_result` (tokens, cost).** Rejected: every Claude and Bob job on disk has `n_input_tokens`, `n_output_tokens` and `cost_usd` null (`tests/harbor/jobs/epic-claude-plan/plan-workflow-claude__f5mLhau/result.json`, `.../2026-10-16__10-19-11/plan-workflow-bob__ZPi4iq6/result.json`). Harbor fills it only when its own session conversion succeeds.
- **Write the report as a `spektacular` Go subcommand.** Rejected: the Technical Approach says to build on the shared harbor helpers rather than start a separate toolchain. The helpers are Python (`tests/harbor/shared/*.py`), and a test-harness report does not belong on the product CLI's surface.
- **Get version and commit by stamping the binary with `-ldflags -X cmd.version/cmd.sha`.** Rejected: a non-`dev` version takes part in the upgrade gate (`cmd/root.go:23-29`, `migrate.DevVersion`), so stamping harbor builds could change how `version check` behaves inside the suites. Writing a build-info file at render time touches nothing the agent runs.
- **Re-own collected job directories (`chown`) so the report can read everything.** Rejected: it needs elevated permissions or a docker hop. The record and `result.json` are already left readable (`verifier/` files are mode 0644; `result.json` is owned by the user), so the report reads only those.
- **Claude wall time = the summed `duration_ms` of every `result` event.** Rejected (user decision): with background sub-agents, Claude emits one `result` per foreground segment. In `epic-claude-epic` those sum to 88 s against a 1094 s transcript span and 1096 s of harbor agent execution.

## Chosen approach — evidence

- Every suite's `test.sh` writes `reward.txt` from pytest's exit code (`tests/harbor/plan-workflow/tests/test.sh:4-10`, the same in all eight suites). Four suites already run a reporting helper with `|| true` before pytest (`tests/harbor/epic-workflow/tests/test.sh:5`, `goal-check-workflow/tests/test.sh:5`), which is the pattern for a step that never fails the run.
- Shared helpers are copied into a suite's tests by its Makefile render target (`Makefile` `_harbor-test-epic`: `cp tests/harbor/shared/*.py .../tests/`). Only epic, standing-rules, implement-epic and goal-check do this today. Spec, plan and repo copy named files only, and `harbor-test-implement` runs harbor straight from `tests/harbor/implement-workflow` with no render step.
- Claude's stream-json (`/logs/agent/claude-code.txt`) ends in `result` events carrying `duration_ms`, `duration_api_ms`, `num_turns`, `total_cost_usd`, `usage`, `modelUsage` (per model: `inputTokens`, `outputTokens`, `cacheReadInputTokens`, `cacheCreationInputTokens`, `costUSD`) and `subagent_stats` (`spawned`, `completed`, `failed`, `killed{parent,user,system}`). With several result events, `duration_ms` and `num_turns` are per segment, while `duration_api_ms`, `total_cost_usd`, `modelUsage` and `subagent_stats` are cumulative. This was checked across `2026-10-16__11-50-51` (8 events) and `epic-claude-epic` (4 events).
- Claude assistant and user messages carry an ISO `timestamp`. Sub-agent messages carry `parent_tool_use_id`. `system/task_notification` events carry `status` and `usage.duration_ms` per sub-agent (`epic-claude-plan` transcript).
- Spektacular's session log (`/app/.spektacular/debug/*.jsonl`, every suite sets `SPEKTACULAR_SESSION_LOG=on`) gives each command a `timestamp`, `duration_ms`, and `state_before`/`state_after` with `kind`, `name`, `current_step` (`internal/sessionlog/record.go:19-55`). The plan run's lane file shows a step change per `goto` (`epic-claude-plan/.../artifacts/spektacular/debug/*_plan_*.jsonl`). `tests/harbor/shared/sessionlog.py` already parses it into lanes.
- Bob runs leave an ATIF `trajectory.json` (`schema_version: ATIF-v1.6`, `final_metrics: {total_steps}`) with no per-step timestamps, tokens or cost, plus `acp-events.jsonl` with no timestamps (`2026-10-16__10-19-11/plan-workflow-bob__ZPi4iq6/agent/`). `tests/harbor/shared/transcripts.py:acp_steps`/`acp_subagent_launches` already read it.
- Codex: harbor tees `codex exec --json` to `/logs/agent/codex.txt`, copies `$CODEX_HOME/sessions` rollouts to `/logs/agent/sessions/`, and in `populate_context_post_run` converts them to an ATIF `trajectory.json` with `final_metrics` token totals and timestamped steps (harbor 0.23.0 `agents/installed/codex.py:1356-1460`, `:1160-1195`, `:512-540`). Post-run population happens before the verifier, and the plan suite's verifier already reads Bob's `trajectory.json` at verify time.
- pytest 8.4.1 is in every image (`uv tool install pytest==8.4.1`). `--junitxml` gives a `classname` of `module.Class` for class-based suites and `module` for function-style suites (goal-check and implement have 0 classes).
- A Go binary built with `go build` in the repo has `vcs.revision`/`vcs.modified` but no runtime command prints them (`spektacular --version` prints `dev`). The Makefile render targets are the place that knows the git state.

## Files examined

- `tests/harbor/shared/transcripts.py:1-112` — Claude stream-json reader (`read_jsonl`, `claude_main_models`, `claude_subagent_launches`) and ACP readers (`acp_steps`, `acp_subagent_launches`). Pure functions, self-checkable.
- `tests/harbor/shared/sessionlog.py:1-259` — `Event` (timestamp, command, `state_before`/`state_after`, `step_after`), `Lane`, `SessionLog(debug_dir)` grouping lane files by `<kind>_<name>`.
- `tests/harbor/shared/timeline.py:1-60` — report-only helper run from `test.sh` with `|| true`. The model for a never-fail step and for a CLI run locally on collected jobs.
- `tests/harbor/*/tests/test.sh` — eight suites, all `pytest <file> -v` then `echo 1|0 > /logs/verifier/reward.txt`.
- `tests/harbor/*/tests/test_*.py` — classes per verifier: plan 13, spec 15, repo 10, epic 3 (+1 function), implement-epic 5, standing-rules 4, goal-check 0 (8 functions), implement 0 (14 functions).
- `tests/harbor/*/environment/Dockerfile` — epic, standing-rules, implement-epic and goal-check apt-install `python3`. Plan, spec, repo and implement do not, and only have uv's Python 3.12 plus pytest as a uv tool.
- `Makefile:4-30, 66-330` — `HARBOR_ARGS_<agent>`, render targets per suite, `AGENT_ARTIFACTS`. Targets per suite: spec (claude, codex), plan (claude, codex, bob), implement (claude only, no render), repo (claude only, two scenarios), epic, implement-epic, standing-rules, goal-check (claude, codex, bob).
- `cmd/root.go:23-79` — `version`/`sha` are set by ldflags only. `versionString()` prints `dev` for harbor builds.
- `internal/sessionlog/record.go:19-55` — the session-log record and state snapshot fields.
- `README.md:376-505` — the harbor suites' documentation (prerequisites, make targets, test results layout, timeline usage, self-check usage). This is where metrics documentation belongs.
- `tests/harbor/jobs/epic-claude-plan/plan-workflow-claude__f5mLhau/` — reference Claude plan run: 85 turns, $3.51, `duration_ms` 948390, 4 sub-agents, 21 plan steps in the lane log.
- `tests/harbor/jobs/2026-10-16__10-19-11/plan-workflow-bob__ZPi4iq6/` — reference Bob run: trajectory, acp-events, acp-summary, Bob shell log under `artifacts/agent-logs/bob/`.
- `.gitignore:51-54` — `tests/harbor/jobs/`, `.build/`, `shared/__pycache__` are ignored.

## External references

- Harbor 0.23.0 source (`~/.local/share/uv/tools/harbor/lib/python3.14/site-packages/harbor/`): `agents/installed/codex.py` (codex.txt, sessions copy, ATIF conversion, token_count metrics), `agents/installed/claude_code.py:945-970` (cost read from the stream-json `result` line), `trial/single_step.py:40-60` (agent run, then artifacts, then verifier). Mattered for knowing what each agent leaves at verify time.
- pytest JUnit XML output (`--junitxml`): `testcase@classname`/`@name` with `failure`/`error`/`skipped` children. Mattered for check groups without touching verifier code.

## Prior plans / specs consulted

- Spec `000031_agent-session-debug-log` (historical) — why the session log exists: an agent-first, self-contained record of every command and response. That is what makes step boundaries agent-agnostic.
- Knowledge `architecture/testing-architecture.md` — the harbor layer's hand-maintained couplings. A change to how suites are rendered or what `test.sh` does must cover every suite.
- Knowledge `gotchas/harbor-agent-logs-and-artifacts.md` — the transcript is already collected under `agent/`, the session log comes back for every agent, and root-owned files under the job directory are unreadable on the host.

## Open assumptions

- Codex's harbor-written `trajectory.json` (ATIF, with `final_metrics.total_prompt_tokens`/`total_completion_tokens`/`total_cached_tokens` and step timestamps) exists at verify time, as Bob's does. Fallback: the rollout under `/logs/agent/sessions/` (`token_count` events) and `codex.txt`. No Codex job is on disk to confirm this. If neither is readable at verify time, the implement workflow must STOP and ask.
- Each Codex `token_count` event, or ATIF agent step with metrics, is one model call, and that count is the analogue of Claude's turns.
- Bob reports no model through ACP (`requested_model: null`), so its model is recorded as unknown.
- The `result` event fields named above keep their names in the Claude Code version the suites install (2.1.306 on disk).
- A Claude multi-result run's last `result` event holds the cumulative cost, tokens, model time and sub-agent stats (seen in both multi-result runs on disk).

## Drafting assumptions

### Chosen direction: in-container recorder plus local report, both in tests/harbor/shared (architecture)
- **Decision**: `shared/metrics.py` runs in each trial's verifier after the reward is written and writes `/logs/verifier/metrics.json`. `shared/report.py` reads only `verifier/metrics.json` and `result.json` locally. The key design decisions:
  - step windows come from the session log;
  - output volume is counted in characters;
  - unknowns are `null` plus an `unknown` list with reasons;
  - flags are computed in the report, not stored;
  - comparison uses medians per suite.
- **Rationale**: This puts the record beside the reward (the spec's Technical Approach), cannot alter the reward (constraint), reuses `transcripts.py`/`sessionlog.py`, and reads only files left readable, so no elevated permissions are needed.
- **Rejected**: host-side extraction at report time (no record beside the reward, unreadable files); pytest plugin (shares the reward's exit status); a Go subcommand (separate toolchain on the product surface).

### Claude wall time with several result events (discovery)
- **Decision**: With one `result` event, wall time is its `duration_ms`. With several (background sub-agents), it is the transcript's first-to-last timestamp span. Turns are the sum of `num_turns`. Cost, tokens, model time and sub-agent stats come from the last, cumulative event. Confirmed by the user.
- **Rationale**: The summary holds the run's wall time only when there is one segment. Summed segments read 88 s for an 18-minute run.
- **Rejected**: always the span (would not equal the summary even on simple runs); always the summed `duration_ms` (badly wrong for epic runs).

### Agents per suite for verification (discovery)
- **Decision**: No new make targets. Every suite writes a record for whichever agent runs it, and verification covers each suite's existing agent targets. Confirmed by the user.
- **Rationale**: The repo and implement verifiers read Claude transcripts, so new Codex/Bob targets there would fail their checks. The metrics code is agent-agnostic either way.
- **Rejected**: adding Codex/Bob targets to the repo and implement suites and a Bob target to spec.

### Record written by a separate script after the reward (discovery)
- **Decision**: `tests/harbor/shared/metrics.py` runs from each `test.sh` after `reward.txt` is written, with `|| true`, reading pytest's `--junitxml` output.
- **Rationale**: It cannot change the reward, which is the spec's first constraint.
- **Rejected**: pytest plugin/conftest (shares pytest's exit status); host-side post-step (misses non-Makefile runs, hits unreadable files).

### Version and commit from a build-info file (discovery)
- **Decision**: Each Makefile render target writes `tests/build.json` (`agent`, `suite`, `scenario`, `spektacular_version` from `git describe --tags --always`, `commit` from `git rev-parse HEAD`, `dirty` from `git status --porcelain`).
- **Rationale**: Leaves the binary and its version gate untouched. The render target already knows the agent and scenario.
- **Rejected**: ldflags stamping (changes `version check` behaviour inside suites).

### Output volume unit is characters (architecture)
- **Decision**: Per-step output volume is characters of assistant text, thinking and tool-call input, and the record states `"unit": "characters"`.
- **Rationale**: Every agent's log exposes the text. Only Claude exposes per-message tokens, and its stream repeats a message's usage on each content block. Characters were the measure that found the assemble step's retyping.
- **Rejected**: output tokens per step (Claude only, needs de-duplication); tool-call counts (do not show volume).

### Lane attribution in multi-lane runs (architecture)
- **Decision**: In orchestrated runs, a Claude sub-agent's characters go to the lane whose spec name its launch prompt names. An ATIF agent's per-lane output is recorded as unknown when it cannot be split.
- **Rationale**: Overlapping lane windows would double-count if attributed by time alone. Claude's `parent_tool_use_id` makes exact attribution possible.
- **Rejected**: time-window attribution across concurrent lanes (double counting).

### Flags computed by the report, comparison by median (architecture)
- **Decision**: Flags are not stored in `metrics.json`. The report computes them: a step over 1/3 of wall time; a candidate median at least 2× the baseline for time, turns, tokens or cost; a check-score median drop of 0.1 or more.
- **Rationale**: Thresholds can change without re-running suites. The median resists one slow outlier run.
- **Rejected**: mean (sensitive to one outlier); storing flags in the record (stale once thresholds change).

### Conventions selected (architecture)
- **Decision**: Applied: tests-must-pass-for-done, tests-must-not-depend-on-order, plans-never-change-the-active-install. Dropped:
  - error-messages-must-suggest-remediation, which governs the Go CLI's error envelope, not a test-harness script;
  - store-files-must-be-written-through-the-cli, since no store file is touched.
- **Rationale**: Only these three bear on test-harness Python, the Makefile and docs.
- **Rejected**: listing every convention.

### Extend existing shared modules instead of new reader modules (components)
- **Decision**: Agent-log summary readers go into `transcripts.py`, and step windows into `sessionlog.py`. Only the recorder (`metrics.py`) and the report (`report.py`) are new modules.
- **Rationale**: The spec asks to build on the existing helpers, and the Makefile already copies `shared/*.py`.
- **Rejected**: new per-agent modules (duplicate parsing, more files to copy).

### Self-checks live in a subdirectory of shared (components)
- **Decision**: The recorder and report self-checks go under `tests/harbor/shared/tests/`, so `cp tests/harbor/shared/*.py` does not copy them into images.
- **Rationale**: This matches the existing practice of keeping self-checks out of the image.
- **Rejected**: `shared/test_*.py` (would be copied by the glob).

### Claude token totals from modelUsage, summed across models (data_structures)
- **Decision**: `input_tokens`, `output_tokens`, `cache_read_tokens` and `cache_write_tokens` are sums over the last `result` event's `modelUsage` entries.
- **Rationale**: `modelUsage` includes sub-agents and matches `total_cost_usd`, whereas the top-level `usage` covers the main agent only (55,024 vs 66,293 output tokens in the reference plan run). Both are in the agent's own summary, so the "totals match the agent log" criterion holds.
- **Rejected**: top-level `usage` (understates runs with sub-agents and disagrees with cost).

### Unknowns as null plus a reasoned list; schema_version field (data_structures)
- **Decision**: Missing measures are JSON `null`, each listed in `unknown` with a reason. The record carries `schema_version: 1`.
- **Rationale**: `null` cannot be mistaken for zero, the reason tells a reader whether the gap is the agent or a broken log, and the version lets the report handle future shape changes.
- **Rejected**: omitting keys (breaks "same top-level fields"); sentinel strings in numeric fields.

### Check score counts only tests that ran (data_structures)
- **Decision**: `total` = passed + failed + errored. Skipped tests count in neither.
- **Rationale**: A skip is not a check the run was held to. This matches pytest's pass/fail exit semantics, which drive `reward.txt`.
- **Rejected**: counting skips as failures or passes.

### Recorder runs after pytest; the timeline stays where it is (implementation_detail)
- **Decision**: The recorder call goes after the reward is written. The existing timeline call before pytest is untouched.
- **Rationale**: The reward must be settled before the recorder starts, and the timeline's placement is deliberate (top of the output).
- **Rejected**: folding the timeline into the recorder (out of scope; it reports for people reading the output).

### Reader chosen from log files present, confirmed by build info (implementation_detail)
- **Decision**: `claude-code.txt` selects the Claude reader. `trajectory.json` selects ATIF. A Codex rollout under `sessions/` is the fallback. The build info's agent name confirms the choice, and a mismatch is recorded as unknown with a reason.
- **Rationale**: This works for runs started without the Makefile (no build info) and catches mis-labelled builds.
- **Rejected**: trusting build info alone (missing for hand-started runs).

### Seven tasks, all agent-executed; end-to-end harbor runs are manual test-plan items (tasks)
- **Decision**: Seven `agent` tasks: readers, step windows, recorder, suite wiring, report, comparison, docs. The harbor end-to-end runs per suite × agent are manual items in the Testing Approach, not tasks.
- **Rationale**: A run of the suites checks behaviour, and a check left as a task is never ticked. No task needs secrets beyond the developer's own agent logins, which the manual runs use.
- **Rejected**: a `human` task for "run every suite" (a review/check, not work that makes the change exist).

### Bob wall time falls back to its shell log, then the session-log span (tasks)
- **Decision**: When the trajectory has no timestamps (Bob), wall time comes from the first-to-last `ts` in Bob's shell logs under `/root/.bob/logs`, else from the session log's first-to-last event.
- **Rationale**: The spec's acceptance criteria require Bob's wall time to be present. The shell log brackets Bob's whole run, and the session log is the agent-agnostic last resort.
- **Rejected**: harbor's `agent_execution` timing (only in host-side `result.json`, not visible to the verifier).

### Multi-lane orchestrator output reported separately (tasks)
- **Decision**: In orchestrated Claude runs, main-agent characters go to a top-level `orchestrator_output`, and lane steps get only their child's characters.
- **Rationale**: The orchestrator is not inside any one lane's step. Folding it into lanes would double count.
- **Rejected**: assigning orchestrator output to whichever lane step is open (several are open at once).

### Plan name kept as the spec name (assemble)
- **Decision**: The plan is addressed as `000070_harbor-run-metrics`, the name the workflow was started with. No separate `NNNN-slug` is proposed.
- **Rationale**: `plan new` fixed the name, and the plan store already holds every plan under its spec's name (`spektacular plan file list`).
- **Rejected**: a fresh `NNNN-description` slug (would not match the store or the spec).

## Rehydration cues


- `spektacular spec file read 000070_harbor-run-metrics`
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'` and `gotchas/harbor-agent-logs-and-artifacts.md`
- Read `tests/harbor/shared/{transcripts,sessionlog,timeline}.py`, any suite's `tests/test.sh`, and `Makefile` lines 66-330.
- Inspect a reference run: `tail -1 tests/harbor/jobs/epic-claude-plan/*/agent/claude-code.txt | python3 -m json.tool` and the lane log under `.../artifacts/spektacular/debug/`.
- Bob reference run: `tests/harbor/jobs/2026-10-16__10-19-11/plan-workflow-bob__ZPi4iq6/agent/trajectory.json`.
