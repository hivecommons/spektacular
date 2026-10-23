# Working context: plan for 000070_harbor-run-metrics

## Carried over from the spec workflow (still relevant)

- Motivation: the user finds planning "really slow" and wants a standard per-run metrics output to find issues and measure changes (issue #90 steps 2-3).
- Evidence that motivated it: plan run `epic-claude-plan` 15.8 min, 85 turns, $3.51; `assemble` step 5.9 of 15.8 min retyping working files.
- Per-step timing source: Spektacular debug log `.spektacular/debug/*.jsonl` (one entry per CLI command, timestamped). Claude totals: stream-json `agent/claude-code.txt` `result` event (duration_ms, duration_api_ms, num_turns, total_cost_usd, usage, subagent_stats).
- Existing helpers: `tests/harbor/shared/transcripts.py`, `sessionlog.py`, `timeline.py`; Makefile copies shared/ into each suite's tests/.
- Bob/ACP logs have no tokens or cost; Codex leaves no sub-agent signal.
- Gotcha: harbor leaves job dirs root-owned (agent/sessions unreadable; `go test ./...` from repo root fails with permission denied).

## Plan workflow decisions

- Spec chosen by user: 000070_harbor-run-metrics. Plan started fresh (no resume, no uncommitted-changes prompt).
- Target repo: spektacular only (docs website is a spec non-goal). No design refs on the spec. No interview notes.
- User decision (discovery): Claude wall time = single result's duration_ms; with several result events (background sub-agents) use transcript timestamp span. Turns = sum of num_turns; cost/tokens/api time/subagent_stats from last (cumulative) result.
- User decision (discovery): no new make targets; verify each suite with the agent targets it already has (repo + implement claude only; spec claude+codex).
- Learned: only epic/standing-rules/implement-epic/goal-check copy shared/*.py and have apt python3; spec/plan/repo/implement don't; harbor-test-implement runs straight from source dir (no .build render).
- Learned: harbor result.json agent_result tokens/cost are null for Claude and Bob; Bob trajectory has no timestamps/tokens/cost; Codex gets an ATIF trajectory.json from harbor post-run (unverified, no codex jobs on disk).
- Learned: Claude task_notification events carry usage.duration_ms per sub-agent; assistant messages carry timestamps.
- Architecture chosen: shared/metrics.py recorder (runs in test.sh after reward, `|| true`, reads --junitxml) + shared/report.py local report (reads only verifier/metrics.json + result.json); tests/build.json from Makefile for identity; chars as output unit; flags computed in report (median comparison).
- Tasks drafted (7, all agent): readers 92a98299, step windows b1822747, recorder ad91e65b, wiring c05ca510, report b19b3bd0, compare cd421be0, docs 899c287f. M1 = first four, M2 = report, M3 = compare + docs.
- Assembled with scratchpad script assemble.py (working files + *.extra in work dir) into .spektacular/tmp/000070_harbor-run-metrics/.
- Verification: removed shell commands from plan.md working files (conventions, architecture, data_structures report interface, dependencies, testing, milestones, out_of_scope).
- All three documents written to the store; work dir removed. Now in walkthrough (read docs with plan file read).
- Walkthrough complete: user approved the plan with no changes and explicitly allowed the auto-commit on finished.
