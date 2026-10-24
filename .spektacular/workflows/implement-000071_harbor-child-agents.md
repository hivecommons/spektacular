# Implement 000071_harbor-child-agents — working context

## Locations
- Code: worktree `/home/nicj/code/github.com/hivecommons/spektacular/.spektacular/worktrees/000071_harbor-child-agents/spektacular` (branch `spek/000071_harbor-child-agents`).
- Run `spektacular` from the project root `/home/nicj/code/github.com/hivecommons/spektacular`.
- Reference jobs are gitignored and exist only in the MAIN checkout: `/home/nicj/code/github.com/hivecommons/spektacular/tests/harbor/jobs/` (m70/s55-claude-*). Trial dirs named in context.md all exist. Never read `agent/sessions/` (root-owned).

## read_plan
- Structure, drift and spec coverage all passed; no mismatches. First-task mode (no Changelog section yet).
- Tooling present: docker, harbor, uv/uvx, go.

## Learnings
- Bob trajectory (`jobs/2026-10-16__09-21-21/epic-workflow-bob__zqFGXrn`): `Running subagent: <suffix>` where suffix is a ~140-char truncation of `arguments.description`; `arguments.description` is the full prompt; `arguments.name` is the kind (`general`). Steps carry no timestamps.
- Claude stream-json facts (research on 2026-10-23 runs; scripts in session scratchpad):
  - NO `timestamp` on any `system` or `result` object; only assistant/user objects have one. Times: launch = enclosing assistant `timestamp`; end = `task_updated.patch.end_time` (epoch ms); duration = `task_notification.usage.duration_ms`.
  - `task_updated` has only `task_id` + `patch{status,end_time}` — join by task_id. `task_notification` has `task_id`, `tool_use_id`, `status`, `summary` (bare, unframed hand-back), `usage`.
  - Grandchildren (spawn_depth 2) stream NO messages → their models/output are per-child unknowns, not []/0.
  - Absent `run_in_background` ≠ foreground (s55 top-level launches were backgrounded). Use `task_started.is_backgrounded`.
  - Only tool name `Agent` seen. `local_bash` task_started exists (filter `task_type == "local_agent"`). s55 implement-epic: 15 Agent calls, 14 local_agent starts, spawned 14; refused launch has an is_error tool_result.
  - `result.origin {"kind":"task-notification"}` only on s55-epic result #1; wake-up shows as `command_lifecycle` with `wakeup_source: "loop_wakeup"`. Mid-stream `system/init` opens a new turn.
  - No failed/killed child in any reference file.

  - `command_lifecycle` is a TOP-LEVEL `type` (not a system subtype): `{"type":"command_lifecycle","state":"started","wakeup_source":"loop_wakeup",...}`.

## Task 1 (readers) — implemented in transcripts.py
- New: `Child`, `Turn`, `Wakeup`, `claude_children`, `claude_turns`, `claude_wakeups`, `atif_children` (+ `ACP_CHILD_GAPS`, `CHILD_ENDINGS`).
- Fallbacks chosen: with NO task_started events in a log, every non-refused launch counts (background then agent-wide unknown); a child with no notification but a non-async launch tool_result takes its ending/hand-back from it (frame + 2-space indent stripped).
- On references: s55-epic 3 children (bg T,T,F; turns start/task-notification/wakeup; ScheduleWakeup delaySeconds=1200); m70-epic 3 foreground; s55 implement-epic 14 = spawned, grandchildren have models/output unknown; Bob 3 children kind `general`.

- Task 1 tests: 18 new cases in `tests/harbor/shared/tests/test_transcripts_summary.py` (builders `task_started`, `task_updated`, `agent_launch`, `launch_result`, `schedule_wakeup`, `turn_result`, …); shared suite 248 passing. Bob parent unknown reason: "bob logs no sub-agent parent".

- Task 1 DONE (ticked + changelog entry). Knowledge `gotchas/harbor-agent-log-fields.md` (repo spektacular) updated with the child-event facts (user accepted).

## Task 2 (record) — implemented in metrics.py
- `AgentLog.read_children` is a zero-arg callable (lazy) so a child-reader crash costs only `subagents.children` (own `gather`). Turns/wakeups NOT stored on AgentLog — verifiers call `tx.claude_turns/claude_wakeups` directly.
- New: `read_session`, `session_lanes`, `lane_windows`, `PlacedChild(child, lane, window, spec, step)`, `place_children(children, windows_by_lane, messages=None) -> (placed, unknowns)` (returns unknowns too, unlike plan sig), `step_child_counts`, `child_record`, `read_children`, `count_step_children`, `idle_seconds`, `read_idle`, `bob_log_times`.
- `Child.order` (default None) added in transcripts.py, set by `atif_children`, used to place Bob children in a one-lane run by message order.
- Idle = longest gap over message times (+ child launch/end + session-log event start/end); unknown when agent log has no times (Bob uses shell-log `ts`).
- Reference results: s55-epic idle 942.167/1249.4; m70-epic 87.63/993.2; goal-check children at check_goal/test/verify; m70 spec child at verification; implement-epic grandchildren at check_goal.

- Task 2 tests: test_metrics.py updated to schema 2 (new consts RECORD_TOTALS_KEYS, RECORD_SUBAGENT_KEYS, STEP_KEYS, CHILD_KEYS, plan_steps(outputs, children)). Fixed: a time-less (Bob) child naming no spec in a multi-lane run is now step-unknown, not "outside any step". 260 shared checks pass.
- For the README task: `test_readme_metrics.py` still pins version-1 keys (passes only because it reads README text) — update it there.

## Task 3 (reference runs) — implemented
- `tests/harbor/shared/tests/trim_reference.py <trial|fixture> <name> [--out dir]`; fixtures in `tests/harbor/shared/tests/data/{m70-epic,s55-epic,s55-plan,s55-implement}/` = `claude-code.txt`, `debug/*.jsonl`, `build.json`, `README`. Self-verifies: whole `build_record` minus run/result must be equal on original and trimmed copy. Idempotent on fixtures.
- Debug responses cut to JSON `{error, code, step}` (resume detection needs `code: workflow_in_progress`); `epic worktree` responses kept in full (for the epic-implement worktree-path rule later).
- Size: ~2.3 MB plain (x-runs keep char counts), gzip ~56 KB for the largest → small in git.
- Plan's s55 durations (109.6/107.6/137.0) are WRONG; actual notification durations are storage 109.605, help 111.142, commands 145.246.

- Task 3 DONE. Trim keeps only each hand-back's first line (newlines preserved) and blanks task_progress descriptions; ScheduleWakeup input kept whole (by plan).

## Task 4 (report) — implemented in report.py
- `IDLE_SHARE`, `idle_share`, `is_idle`, `IDLE_MARK`; table gains `idle m`; breakdown gains `Idle:` line + `children_table` (schema-1: "Children: not recorded (schema 1)"); `--prompts` (needs `--run`, else exit 2) → `prompts(run)`.

- Tasks 4, 5 DONE. Milestone 1 committed (worktree d4346a4; main checkout df4368f).

## Task 6 (plan walkthrough) — implemented
- `sessionlog.plan_steps_entered(events)`; plan verifier's walkthrough test reads `SessionLog(DEBUG_DIR, kinds=("plan",))`; `TestSubAgentSpawning`/`EXPECTED_SPAWN_STEPS`/`_spawn_params` deleted; `temp.py` git-rm'd; `trim_reference.measured()` includes `plan_steps_entered`.
- s55-plan fixture: walkthrough before finished ✓; discovery children = 1.

- Task 6 DONE.

## Task 7 (changelog writes) — implemented
- `sessionlog.changelog_writes_by_repo(events, name=None)` (+ `_positional`, `_VALUE_FLAGS`); implement verifier's three changelog tests read `SessionLog(DEBUG_DIR, kinds=("implement",)).all_events()`; `*_WRITE_SUBSTR` removed; trim `measured()` includes it. s55-implement → {None:1, auth:1, docs:1}.

## User decisions
- 2026-10-24: PENDING SPEC, start only AFTER this implement run completes (user's instruction): spec name `knowledge-access-index-and-prune` (no epic). Run `spek-new`; `spec new` will raise uncommitted_changes → ask the user again. The user's request: agents write knowledge without asking; a JSON access index in the knowledge base, updated only by the `spektacular knowledge` tool, recording when entries are found in search and when they're retrieved for a plan; then all entries update automatically; a `knowledge prune` command, pass 1 using the access index + entry created date to prune invalid entries, pass 2 reads each entry, checks correctness, corrects or deletes.
- 2026-10-24: "Run without asking" — loop through tasks without pausing between them; still stop for genuine questions/mismatches.
- 2026-10-24: wake-up reader counts ANY main-agent Bash that runs `sleep` (foreground or background) as a timer — the s55 implement-epic `sleep 240` poll was foreground, contrary to the plan's assumption. ScheduleWakeup and CronCreate as planned.
