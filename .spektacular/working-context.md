# Working context: 000071_harbor-child-agents

## Plan workflow (started 2026-10-24)

- Planning spec 000071_harbor-child-agents (final). User chose it via /spek-plan after asking
  which spec/plan was latest. Spec-era notes below are carried forward as background.
- Spec's success metrics to carry into Testing Approach: 100% children recorded with context
  across nine Claude suites; full Claude batch passes with no skipped required delegation and
  no idle flag; epic-planning idle < 25% of wall; zero walkthrough/changelog-check misreads;
  report alone answers "which children, what were they told".
- Target repo: spektacular only (docs website is a non-goal). No design refs.
- User answers at discovery (2026-10-24):
  - Both models skip implement test/verify children (only goal reviewers spawned in
    implement-epic on 4.6 and 5.5): STRENGTHEN the step instructions (test, verify, and spec
    verification) AND enforce.
  - Epic implementing child prompt: ADD to spek-implement-epic that the prompt names the spec's
    worktree paths (from `epic worktree`), and check for them.
- Learnings: Agent tool_use input is the prompt source (task_started.prompt sometimes absent;
  background Bash also emits task_started). task_updated.end_time is the only child end time;
  task_notification.usage.duration_ms is the agent's own duration. Child msgs stream inline with
  parent_tool_use_id. Session log answers walkthrough + changelog checks. Implement suite is
  seeded past test/verify/check_goal; goal-check unmet retry and implement-epic run them.
  Plan suite's discovery spawn check must become record-only (constraint).
- User answer at tasks (2026-10-24): epic verifiers get Claude-only transcript checks; update
  knowledge entry architecture/testing-architecture.md via spek-knowledge in the implement run.
- Progress: all three plan documents committed to the store (2026-10-24); working dir removed. Walkthrough done; user approved the plan 2026-10-24. Next: implement workflow.
- Architecture chosen (Option A): shared Child reader -> recorder + new shared delegation.py
  rules; session-log based walkthrough/changelog checks; schema 2; probe-gated headless wording.

## Problem and motivation

Builds on 000070_harbor-run-metrics (metrics.json per harbor run, `make harbor-report`).
The 2026-10-23 batches compared claude-sonnet-4-6 (jobs `tests/harbor/jobs/m70-claude-*`)
with claude-sonnet-5-5 (`s55-claude-*`; Makefile default is now sonnet-5-5).

- **Epic suite idle time.** On 5.5 the plan-epic orchestrator started its first two children
  in the background, ended its turn, and called `ScheduleWakeup` (1200 s, "fallback while
  children plan"). The children's task-notifications woke it and it finished at 13:41, but the
  pending wakeup kept `claude -p` alive until 13:57 (wall 20.8 min for ~4.6 min of work). 4.6
  kept children in the foreground and never used the tool.
- **Why the suites don't test the real path.** `tests/harbor/epic-workflow/instruction.md:53`
  and `implement-epic-workflow/instruction.md:54` say "This session is headless", which triggers
  the epic skills' headless rule (`templates/skills/workflows/spek-plan-epic/SKILL.md:46`,
  `spek-implement-epic/SKILL.md:58`): start children as foreground sub-agents and handle every
  hand-back inside one turn, because "headless Claude Code stops [a background sub-agent] after
  ten minutes". Interactive users get background children plus completion notices — the path
  where the stray wakeup appears. The 5.5 run shows background children + task-notification
  wake-ups do work under `claude -p` (`origin.kind: task-notification`). The ten-minute claim
  is unverified.
- **Skipped delegation.** On 5.5 the spec run's `verification` step spawned no sub-agent
  (4.6 delegated it, 136 s). It still passed 48/48: nothing checks that delegated steps use a
  child agent.
- **Metrics only count children.** `subagents` in metrics.json holds started/completed/failed/
  stopped and durations. Launch prompts are already parsed (`SubagentLaunch` in
  `tests/harbor/shared/transcripts.py`) but not recorded.

## User's words

- "we actually want the tests to use child agents just like the real process. Otherwise we
  can't test this. The metrics should show the child agents created and the context passed to
  them so we can keep track of this."
- Chose all three test changes: background children in epic suites; require child agents where
  workflows use them; check the context passed to children.
- "We need to inform that the session is headless so that it does not block on waiting for a
  question. You might need to validate the test there to check" — keep telling the agent it is
  headless (so it never waits on a question), but headless must not force foreground children;
  validate the headless wording/test.

## Proposed metrics shape (shown to the user, not yet confirmed as final)

`subagents.children[]`: id, parent, type, description, background, lane, launched_at, ended_at,
duration_s, status (completed/failed/stopped/no hand-back), hand_back (first line), models,
output_chars, prompt (full), prompt_chars. Report: `--run` prints a children table; `--prompts`
prints each child's full launch prompt. Bob/Codex record unknowns.

## Related findings (context, maybe out of scope)

- Verifier false negatives on 5.5 from command style: plan `walkthrough` check (gotos piped
  through `python3 -c` / chained with `&&`), implement per-repo changelog check (writes in a
  shell loop with `$a`).
- Real 5.5 differences: goal-check agent fixed the README before the reviewer; repo
  one-at-a-time recorded values without proposing them.
- 401s: HARBOR_AUTH passes the short-lived OAuth token, rotated mid-batch; `claude setup-token`
  would avoid it.
- Ideas already floated: `idle_s` metric; a check that no ScheduleWakeup/CronCreate is used;
  denying those tools in the container only as a backstop.

## Decisions

- Spec name harbor-child-agents (000071), standalone (not in epic 000001).
- Pre-existing changes (Makefile/README model bump, lane_for fix + tests) committed first.

## Interview answers (2026-10-23)

- Enforcement: required delegations fail (spec verification, implement test/verify/check_goal,
  epic children); suggested ones (plan discovery, analyze, update_changelog) recorded only.
- Headless: verify the 10-minute background-child claim first, then fix the epic skills.
- Extra scope: no-wakeup check + idle_s metric; fix the two verifier false negatives.
  Not chosen: long-lived auth token, website docs.
- Agents: Claude enforced; Bob/Codex recorded with unknowns.

- User (overview step): "just finish the spec, I will review the whole thing" — draft every section without per-section confirmation; user reviews the assembled spec at verification.
- Spec committed to the store after the user approved the full draft (reviewer findings applied; finding 14 compatibility constraint skipped as untraceable; reference runs kept as checked-in test data in Technical Approach).
- Split offered (child metrics / delegation checks / verifier fixes); user declined: keep one spec.
