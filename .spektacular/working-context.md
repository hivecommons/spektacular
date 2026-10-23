# Working context: 000070_harbor-run-metrics

## Problem and motivation

- The user feels some activities, planning in particular, are "really slow" and wants to understand how to improve them.
- First step, in the user's words: "I would like to standarize this output so that we can use it going forward to identify issues and measure changes."
- "This output" = the ad-hoc analysis done in conversation with throwaway scripts: a per-run table (wall time, API time, turns, cost, output tokens, cache-read tokens, sub-agents, reward) across every harbor job, and a per-step breakdown of one run (time per workflow step, characters written per step).
- Source: GitHub issue #90 (Quantitatively evaluate Spektacular's performance). This spec is the groundwork for its steps 2-3 (record cost/turns/time/tokens; Spektacular-specific measures). Braintrust and repeated runs/pass^k are later.

## Findings that motivated it (evidence, not requirements)

- Plan workflow (Sonnet 4.6, passing run `epic-claude-plan`): 15.8 min, 85 turns, $3.51, 55k output tokens, 5M cache-read tokens. 3-4x the output of any other single-spec workflow.
- Runs are output-bound: CLI calls return in ~0s, wall time tracks output tokens.
- The `assemble` step took 5.9 of 15.8 min retyping ~74k chars of working-file content into scaffolds. An Opus run wrote its own Python assembler and did it in 54s.
- Per-step timing came from Spektacular's debug log (`.spektacular/debug/*.jsonl`, one entry per CLI command with timestamp); per-message activity and written characters came from Claude's stream-json log (`agent/claude-code.txt`, whose `result` event has duration_ms, duration_api_ms, num_turns, total_cost_usd, usage, subagent_stats).

## Existing infrastructure to build on

- `tests/harbor/shared/transcripts.py` (reads Claude stream-json and ACP/Bob trajectory; sub-agent launches), `sessionlog.py` (Spektacular debug log), `timeline.py` (epic lane timeline, printed by four suites' test.sh). The Makefile copies shared/ into each suite's tests/.
- Jobs on disk: 35 Claude runs, 7 Bob (ACP) runs, 6 oracle runs. Codex leaves no sub-agent signal.

## Decisions and answers so far

- Spec name: harbor-run-metrics (000070). Standalone, not part of epic 000001. Source #90 recorded.
- Started without committing existing work (user chose commit_existing: false).
- Proposed scope the user accepted in principle ("yes, do it please"):
  - per-run `metrics.json`, same schema for every agent and suite, written by the verifier beside the reward;
  - totals: wall, API time, turns, cost, input/output/cache tokens, agent, model, suite, Spektacular version, commit;
  - per-step: duration and characters written in each workflow step;
  - sub-agents: spawned, completed/killed, duration;
  - fields an agent's logs can't supply are recorded as missing, never zero;
  - a local comparison command over `tests/harbor/jobs/`: one row per run grouped by suite, step breakdown for a chosen run, before/after comparison of two sets of runs.
  - Out of scope: Braintrust or any hosted store; repeated runs / pass rate / pass^k.
- Interview answers: include partial check scores (reward.txt stays 0/1; scores in the metrics record); exclude Spektacular-specific measures; comparison sets named as job dirs/globs; report flags outliers; docs in this repo only, not the website.
- Bob/ACP logs have no tokens or cost (harbor result.json agent_result fields null; trajectory final_metrics only total_steps).
- Spec 000070_harbor-run-metrics written to the store after a fresh-eyes review; user approved 13 of 17 findings (dropped: job-dir naming is the user's choice; no extra verifier-must-not-fail or local-only constraints).
- Split check (moderate): only one weak signal (11 requirements); record and report are tightly coupled, so no split offered.
- Pending: advancing to `finished` triggers auto_commit across ~200 uncommitted paths of the user's own WIP; user's global rule is never to commit unless explicitly asked, so confirm before advancing.
- Known gotcha: harbor leaves job dirs root-owned (session transcripts under agent/sessions unreadable; `go test ./...` from repo root fails with permission denied). Either fix ownership in the run target or rely only on files that stay readable.
