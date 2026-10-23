---
tags: [harbor, logs, metrics, tokens, atif, codex, claude]
---

## Reading numbers out of harbor agent logs: fields that do not mean what they look like

**ATIF and Codex prompt tokens include the cached ones.** Harbor's ATIF
`trajectory.json` `final_metrics.total_prompt_tokens` is documented as
"including cached tokens", and Codex's rollout `total_token_usage.input_tokens`
includes `cached_input_tokens` (OpenAI's convention). Claude's
`modelUsage.inputTokens` excludes cache reads. To compare input across agents,
take uncached input as prompt minus cached.

**Claude's stream-json timestamps only `assistant` and `user` objects.**
`system/*` events — `init`, `task_started`, `task_progress`,
`task_notification` — carry no `timestamp`, so a run's time span comes from
the message objects alone, and a sub-agent's duration comes from its
`task_notification` `usage.duration_ms`, not from timestamps.

**A failed Claude sub-agent may report no duration.** Its
`task_notification` can arrive with `status: "failed"` and no
`usage.duration_ms` (seen in `jobs/epic-claude-epic`: 3 spawned, 2
durations). Treat the missing duration as unknown, never as zero.

**With background sub-agents, Claude writes several `result` events.**
Each one's `duration_ms` and `num_turns` cover only its foreground segment,
while `duration_api_ms`, `total_cost_usd`, `modelUsage` and `subagent_stats`
are cumulative. Summing `duration_ms` badly understates the run (88 s against
an 18-minute run); use the transcript's first-to-last timestamp instead.

**Not every Codex `token_count` event is a model call.** Codex also emits
`token_count` events with `info: null` for rate-limit updates. Count only
those carrying usage; harbor's own rollout-to-ATIF conversion skips them too.

The readers that apply these rules live in `tests/harbor/shared/transcripts.py`.
