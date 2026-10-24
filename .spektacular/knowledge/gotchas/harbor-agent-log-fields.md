---
tags: [harbor, logs, metrics, tokens, atif, codex, claude, subagent]
---

## Reading numbers out of harbor agent logs: fields that do not mean what they look like

**ATIF and Codex prompt tokens include the cached ones.** Harbor's ATIF
`trajectory.json` `final_metrics.total_prompt_tokens` is documented as
"including cached tokens", and Codex's rollout `total_token_usage.input_tokens`
includes `cached_input_tokens` (OpenAI's convention). Claude's
`modelUsage.inputTokens` excludes cache reads. To compare input across agents,
take uncached input as prompt minus cached.

**Claude's stream-json timestamps only `assistant` and `user` objects.**
`system/*` events — `init`, `task_started`, `task_progress`, `task_updated`,
`task_notification` — and `result` events carry no `timestamp`, so a run's
time span comes from the message objects alone, and a sub-agent's duration
comes from its `task_notification` `usage.duration_ms`, not from timestamps.
A sub-agent's absolute end time is `task_updated`'s `patch.end_time`, in epoch
milliseconds.

**A failed Claude sub-agent may report no duration.** Its
`task_notification` can arrive with `status: "failed"` and no
`usage.duration_ms` (seen in `jobs/epic-claude-epic`: 3 spawned, 2
durations). Treat the missing duration as unknown, never as zero.

**Claude's sub-agent events join on different keys.** `task_started` and
`task_notification` carry the launching `Agent` call's `tool_use_id` and the
`task_id`; `task_updated` carries only `task_id` and `patch`
(`status`, `end_time`), so it must be joined through `task_started`'s
`tool_use_id` → `task_id` mapping. `task_notification.summary` is the bare
hand-back, without the `[Subagent hand-back] …` frame and two-space indent
that a foreground launch's `tool_result` wraps it in. Background shell tasks
also emit `task_started` (`task_type: "local_bash"`); only `local_agent` ones
are sub-agents.

**A launch without `run_in_background` may still run in the background.**
The 2026-10-23 Sonnet 5.5 epic run's top-level launches had no
`run_in_background` key and still started with `is_backgrounded: true`.
`task_started.is_backgrounded` is the only reliable background state.

**Sub-agents started by sub-agents stream no messages.** A depth-2 child
(`spawn_depth: 2`, with `parent_task_id`) appears only as its parent's launch
`tool_use` and `tool_result` plus its own `system/task_*` events. None of its
assistant messages are streamed, so its models and output are unknown, never
an empty list or zero. Depth-1 children stream theirs with
`parent_tool_use_id` set to their launch id.

**With background sub-agents, Claude writes several `result` events.**
Each one's `duration_ms` and `num_turns` cover only its foreground segment,
while `duration_api_ms`, `total_cost_usd`, `modelUsage` and `subagent_stats`
are cumulative. Summing `duration_ms` badly understates the run (88 s against
an 18-minute run); use the transcript's first-to-last timestamp instead. A
turn opened by a sub-agent's completion carries `origin.kind:
"task-notification"`; one opened by a scheduled wake-up follows a
`command_lifecycle` object with `wakeup_source`, which is a top-level `type`,
not a `system` subtype.

**Not every Codex `token_count` event is a model call.** Codex also emits
`token_count` events with `info: null` for rate-limit updates. Count only
those carrying usage; harbor's own rollout-to-ATIF conversion skips them too.

The readers that apply these rules live in `tests/harbor/shared/transcripts.py`.
