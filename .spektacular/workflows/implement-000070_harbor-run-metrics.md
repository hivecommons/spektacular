# Working context: implement 000070_harbor-run-metrics

## Locations
- Worktree (all code work): `/home/nicj/code/github.com/hivecommons/spektacular/.spektacular/worktrees/000070_harbor-run-metrics/spektacular`
- Run `spektacular` from the project root `/home/nicj/code/github.com/hivecommons/spektacular`.
- Reference harbor jobs (gitignored) live only in the main checkout: `/home/nicj/code/github.com/hivecommons/spektacular/tests/harbor/jobs/`.

## read_plan
- Structure, drift and spec coverage checks all passed; no mismatches, no descoped items.
- No `## Changelog` section yet: first-task mode.

## Learnings from real logs (task 1 analysis)
- Claude stream-json: only `assistant`/`user` objects carry `timestamp`; `system/*` events do not. `thinking` blocks are often redacted (empty text). Failed sub-agents' `task_notification` may lack `usage.duration_ms` (epic-claude-epic: 3 spawned, 2 durations).
- Harbor ATIF `final_metrics.total_prompt_tokens` INCLUDES cached tokens (harbor FinalMetrics docstring; OpenAI semantics). Decision: record `input_tokens = prompt - cached` so it means uncached input like Claude's. Same for Codex rollout `input_tokens`.
- Harbor computes Codex `total_cost_usd` via litellm when available; take it when present, else unknown.
- Bob ATIF steps: no timestamps, no reasoning_content; tool calls named `execute` with `arguments.command`.
- Decision: implement tasks directly rather than spawning sub-agents (small, tightly coupled Python helpers).

## Task 1 (Read run summaries) progress
- Implemented in `tests/harbor/shared/transcripts.py`; self-checks in `tests/harbor/shared/tests/test_transcripts_summary.py` (run: `uvx --with pytest pytest -p no:cacheprovider tests/harbor/shared/tests/...`; host python3 has no pytest).
- `claude_subagents(objs)` takes objs only (finds the last result itself) rather than the context's `(objs, summary_result)`.
- Claude with no result event still takes `model` from `system/init`.
- Codex rollout: only `token_count` events whose `info` is a dict count as turns (Codex emits info-null rate-limit updates; harbor skips them too).
- `atif_messages` `order` is the step index including user steps (used only for ordering).
- Task 1 DONE: ticked in plan, changelog entry written (plan now has `## Changelog`). Milestone 1 not finished, so no auto-commit yet; the commit message is due when task 4 (wiring) completes Milestone 1.

## User answers
- After task 1: "Continue, don't ask again" — loop through remaining tasks without pausing; stop only for real questions/failures.
- Accepted saving the harbor log gotcha (ATIF prompt tokens include cached; Claude timestamps only on assistant/user; failed sub-agent may lack duration) via spek-knowledge.

## Task 2 (step windows)
- Real lane logs record `duration_ms: 0` for every command; `state_after.current_step` is reliable. Reference plan run's `assemble` window = 368.6 s (16:17:47→16:23:56).
- A resume within one lane file (refused `<kind> new`, `workflow_in_progress`) does not start a new segment. Decision: a resume closes the open window at the previous event's end and opens a fresh window at the resume, so the hand-off gap is excluded.
- Running several suites' `test_verifier_selfcheck.py` in ONE pytest call fails collection ("import file mismatch", same basename, no `__init__.py`); add `--import-mode=importlib` or run them separately. Keep new shared self-check basenames unique.
- Task 2 DONE (ticked + changelog). Next: task 3 recorder (High); its completion is NOT a milestone end (task 4 ends Milestone 1).

## Task 3 (recorder) decisions
- Spec suite writes `*_spec_<name>.jsonl` lane files, but `sessionlog.LANE_FILE` matches only plan|implement. Added `SessionLog(debug_dir, kinds=LANE_KINDS)`; default unchanged (epic readers/timeline untouched); recorder passes ("spec","plan","implement").
- Repo suite runs no workflow: session log has events but no lane -> `workflows: []` with NO unknown (known-empty). Unknown("steps") only when the session log is missing/empty.
- Multi-lane Claude: sub-agent chars whose launch prompt names no lane count toward `orchestrator_output`.
- Bob order-based attribution: a goto message's chars go to the step it was issued from, then the step advances.
- Fixed during test step: with no build.json, `run.agent` filled from the log is no longer also listed unknown.
- Oracle/missing-log runs get wall time from the session-log span via `wall_time_fallback` (applies to any agent with unknown wall time, not only Bob).
- Task 3 DONE (ticked + changelog). Next: task 4 wiring — completes Milestone 1, so supply commit_message_from when advancing after its changelog.

## Task 4 (wiring)
- GOTCHA hit: `make -n harbor-test-<x>` prints the recursive `$(MAKE) _harbor-test-...` line, and executing that output runs the REAL suite (harbor run). It started a spec-workflow Claude trial (killed during setup; container, image and the root-owned job dir removed via docker). To dry-render, call the `_harbor-test-*` target directly and drop `make`/`harbor run`/`go build` lines.
- All 8 render targets verified: tests/ holds metrics.py, transcripts.py, sessionlog.py, timeline.py and valid build.json; implement-epic VARIANT -unlimited -> scenario "unlimited".
- Task 4 DONE; Milestone 1 complete; commit made on advance to analyze. Next: task 5 report (Milestone 2).
