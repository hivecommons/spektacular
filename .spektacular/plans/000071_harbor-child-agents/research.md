---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Research: 000071_harbor-child-agents

## Alternatives considered and rejected

### A separate child-agent tool beside the metrics recorder

Rejected: the spec's Technical Approach says to extend the existing record and report, and `tests/harbor/shared/metrics.py:101-110` (`AgentLog`) and `report.py:269-316` (`breakdown`) already have the natural slots (a children list next to `launches`; a table between the "Sub-agents:" line and "Unknown:").

### Build children from `task_started` alone

Rejected: `task_started.prompt` is absent in some runs (s55 implement-epic: every `task_started` lacks `prompt`), and background Bash tasks also emit `task_started` with `task_type` other than `local_agent` (`bt5kegl7r`, a `sleep 240` poll). The `Agent`/`Task` `tool_use` input is the reliable prompt source; `task_started` (matched by `tool_use_id`) supplies `task_id`, `is_backgrounded`, `spawn_depth`, `parent_task_id`.

### Extend `SubagentLaunch` with the new fields

Rejected: `test_transcripts_summary.py:429-447` pins it by dataclass equality, and `goal-check-workflow/tests/test_verifier_selfcheck.py:148-153` builds it positionally. A new `Child` dataclass and reader beside it leaves those untouched.

### Take a child's end from the launch's `tool_result` timestamp

Rejected: a background launch's `tool_result` returns in milliseconds ("Async agent launched successfully", `tool_use_result.isAsync: true`), so `SubagentLaunch.returned` and that timestamp say nothing about the child finishing. `task_updated.patch.end_time` (epoch ms) is the only absolute end time; system events carry no `timestamp` (knowledge `gotchas/harbor-agent-log-fields.md`).

### Child duration from `ended_at - launched_at`

Rejected for the recorded `duration_s`: the acceptance criterion requires the agent's own report of run time, which is `task_notification.usage.duration_ms` (s55 epic storage child: 109.6 s). The timestamp span is kept only as the start/end pair.

### Idle time from the session log alone

Rejected: the session log records only `spektacular` calls, so long model thinking between calls would read as idle. Claude message timestamps (main and children, children are streamed inline with `parent_tool_use_id`) plus `task_updated.end_time` give the real activity clock; the session log is a secondary source for agents without per-message time (Bob).

### Idle as total of gaps above a threshold

Rejected in favour of the spec's definition, the single longest stretch: on the reference runs the longest gap gives s55 epic 942 s of 1249 s (75%, flagged, ≥ 15 min as required) and m70 epic 88 s of 992 s (9%, not flagged).

### Keep pattern-matching command text for the walkthrough and changelog checks, with a looser regex

Rejected: the spec asks the checks to judge what ran. `test_plan_workflow.py:1394-1438` needs the goto's tool-result text (`_step_payload`), which piping or chaining hides; `test_implement_workflow.py:98-104,332-370` misses backslash continuations, reordered flags and shell loops. The session log (`internal/sessionlog/record.go:18-29`: argv, `exit_code`, `state_after.current_step`) holds every advance and every write for both reference runs.

### Enforce required delegation in the implement suite

Rejected: its seeded `environment/state.json` starts at `test_plan` with `test`, `verify` and `check_goal` already completed, so those steps never run there. They run in the goal-check suite (seed at `check_goal`; the unmet scenario retries `implement → test → verify → check_goal`) and inside implement-epic's children.

### Keep the plan suite's discovery spawn check (`EXPECTED_SPAWN_STEPS = {"discovery"}`, `test_plan_workflow.py:108,936-953`)

Rejected: discovery only suggests a child (`templates/steps/plan/02-discovery.md:55`), and the spec's constraint forbids failing a run for not using a suggested child. It becomes record-only.

### A Claude-only rule switched on per agent in the epic skills

Rejected as unavailable: skills are rendered byte-identically for every agent (`internal/agent/skills.go:24-71`); agent-specific advice is written inline ("headless Claude Code", "Under Bob").

### Derive the required-delegation set from the templates at verify time

Rejected: knowledge `architecture/testing-architecture.md` requires hand-maintained, independent oracles. Instead a self-check keeps the hand-kept list in step with the templates' delegation wording.

### Per-verifier transcript parsing (architecture option B)

Each suite's verifier parses the Claude transcript itself, as the plan suite does today, with the recorder kept separate.

**Rejected**: Duplicates the readers 000070 centralised in `tests/harbor/shared/transcripts.py`, so a verifier and the record could disagree on where a child was. Chosen option A shares one reader and one placement function.

### Recorder plus report only (architecture option C)

Compute children and delegation in the recorder and judge them in the report.

**Rejected**: The report runs on the host after the fact and cannot fail a run, but the spec requires failed runs. The recorder must never fail a run either (`metrics.py:477-496`).

## Chosen approach — evidence

- Claude stream-json child data (verified on `jobs/s55-claude-epic`, `s55-claude-implement-epic`, `m70-claude-implement-epic`, `m70-claude-spec`):
  - `Agent` tool_use input: `description`, `prompt`, `subagent_type` (often absent → default `general-purpose`), `run_in_background` (true/false/absent). Launch time = enclosing assistant object's `timestamp`; a nested launch's assistant object carries `parent_tool_use_id` of its parent child.
  - `system/task_started`: `task_id`, `tool_use_id`, `is_backgrounded`, `spawn_depth`, `parent_task_id` (depth ≥ 2), `task_type` (`local_agent` for children), sometimes `prompt` (identical to the tool_use prompt when present: 1723/1705/1759 chars in s55 epic).
  - `system/task_updated.patch`: `status`, `end_time` (epoch ms), `error` on failure.
  - `system/task_notification`: `status` (completed/failed/killed), `summary` (clean hand-back text, first line e.g. `DONE: 20260101000001-storage`), `usage{total_tokens, tool_uses, duration_ms}` (absent on failure).
  - Child messages appear inline with `parent_tool_use_id` = the launch id, timestamped; their `message.model` gives the child's models.
  - `result.subagent_stats.spawned` is the agent's own child count (s55 implement-epic: 14, of which 10 `spawned_by_subagents`; 15 Agent tool_uses, one refused launch with no `task_started`).
  - Turn evidence: each turn ends in a `result` event; a turn woken by a child carries `origin: {"kind":"task-notification", ...}` and is preceded by `system/init`. A `ScheduleWakeup` call shows as tool_use `ScheduleWakeup` (s55 epic: `delaySeconds: 1200`, "fallback while children plan") and a later `command_lifecycle` with `wakeup_source: loop_wakeup`.
- Placement: `metrics.window_of` (`metrics.py:246-251`) and `metrics.lane_for` (`metrics.py:254-259`) already place messages by time and lanes by the first spec named in a launch prompt; `read_steps` (`metrics.py:334-340`) builds the windows and currently discards them.
- Unknown convention: `transcripts.Unknown(field, reason)` (`transcripts.py:128-133`), `null` value plus a top-level `unknown` entry; each section is wrapped by `build_record.gather` (`metrics.py:406-413`) so a crash costs only that section; `main` always returns 0 (`metrics.py:477-496`) and runs after the reward (`test.sh`: `python3 /tests/metrics.py || true`).
- Session log as "what ran": s55 plan lane file has every goto with `state_after.current_step` ending `… write_research, walkthrough ×4, finished ×2` although its verifier failed `test_walkthrough_rendered_before_finished`; s55 implement lane file has `changelog file write 20260101000000-jwt-auth --repo auth` and `--repo docs` with `exit_code` 0 (seq 15, 16) although both per-repo checks failed. `sessionlog.Event.is_verb`, `.ok`, `.goto_step`, `.step_after` (`shared/sessionlog.py:43-92`) already expose this.
- Delegation reality on the reference runs: s55 spec run 0 children (verification inline), m70 spec 1 child ("Fresh-eyes spec review", foreground, 136 s); s55 implement 0 children; implement-epic on both models started only goal-check reviewers as grandchildren — no `test` or `verify` child in any lane. User decision: strengthen the step instructions and enforce.
- Epic child prompts: m70 implement-epic named the worktree (`/app/.spektacular/worktrees/20260201000001-storage/todo-app`); s55 did not. `spek-implement-epic/SKILL.md:64-73` asks only for spec name and project root. User decision: the skill will name the spec's worktree paths and the check will require them.
- Headless trigger: `tests/harbor/epic-workflow/instruction.md:53` and `implement-epic-workflow/instruction.md:54` ("This session is headless: no one replies between your turns") activate `spek-plan-epic/SKILL.md:46` and `spek-implement-epic/SKILL.md:58` ("In a headless session, keep every child inside your turn … headless Claude Code stops one after ten minutes"), plus the fresh-child-in-foreground rule at plan-epic :77 and implement-epic :102. Pinned by `templates/plan_epic_skill_test.go:113-114,151` and `templates/implement_epic_skill_test.go:134-135,208`. The 5.5 run shows background children and task-notification wake-ups do work under `claude -p` (children ran < 10 min).
- Delegation wording in templates: spec `verification` `templates/steps/spec/08-verification.md:25-36` (fresh-context subagent given `.spektacular/tmp/spec_template.md` and "review only what is written in that file"; fallback "If you have no way to spawn a subagent"); implement `test` `04-test.md:3,16,22-35`; `verify` `05-verify.md:3,20-30`; `check_goal` `05b-check_goal.md:54-70`. Suggested: plan `discovery` `02-discovery.md:55`, implement `analyze` `02-analyze.md:28-47`, `update_changelog` `07-update_changelog.md:50-54`. Nothing in `templates/` or `internal/` mentions wake-ups, timers or cron.
- "No one answers" wording per suite: present in spec (:35-45), goal-check (:32-45), repo (:30-41), standing-rules (:30-36), epic and implement-epic; absent from plan and implement instruction.md.

## Files examined

- spektacular:tests/harbor/shared/metrics.py:46-58 — `SCHEMA_VERSION = 1`, `SUBAGENT_FIELDS`; :101-110 `AgentLog`; :126-182 reader choice and per-agent paths (claude, atif, codex-rollout, missing); :246-340 placement and step windows; :401-446 record assembly; :449-496 atomic write, never-fail `main`.
- spektacular:tests/harbor/shared/transcripts.py:35-46 `SUBAGENT_TOOLS`, `SubagentLaunch`; :75-100 top-level launches only; :110-122 Bob ACP launches (`arguments.name` kind, `arguments.description` prompt); :128-180 `Unknown`, `Message`, `Subagents`; :299 `claude_messages`; :324-354 `claude_subagents`; :357-376 `claude_launch_root`; :443-455 `atif_subagents`.
- spektacular:tests/harbor/shared/report.py:30-34 threshold constants; :148-153 unknown rendering; :170-217 `render`, run table; :246-316 wall time, dominant steps, `breakdown`; :433-474 argument handling and usage errors.
- spektacular:tests/harbor/shared/sessionlog.py:43-92 `Event`; :121-228 `StepWindow`, `Lane.step_windows`, `children`; :231-318 `SessionLog`, `all_events`.
- spektacular:tests/harbor/shared/tests/test_metrics.py:36-49,510,550,560-661,792,884-959 — exact shape, schema-version and unknown-list assertions to update; builders :96-491.
- spektacular:tests/harbor/shared/tests/test_transcripts_summary.py:75-206,429-447 — builders; `SubagentLaunch` equality pins.
- spektacular:tests/harbor/shared/tests/test_report.py:74-156,215-232,578,686-694,1021-1045 — record builder, goldens, constant pins, usage errors.
- spektacular:tests/harbor/shared/tests/test_suite_wiring.py:146-284 — copy glob, build info, test.sh tail, python3 in images.
- spektacular:tests/harbor/shared/tests/test_readme_metrics.py:25-37 — README `#### Run metrics` field tables must match record keys.
- spektacular:README.md:497-596 — run metrics field reference and report usage.
- spektacular:Makefile:46-47 `make test` is Go only; :83-90 `harbor-report`; render targets copy `shared/*.py` (spec :115, repo :151, plan :187, implement :210, epic :241, implement-epic :328, goal-check :371).
- spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py:108,936-953 discovery spawn check; :211,521-586 goto parsing; :1322-1342 `_step_payload`; :1394-1438 walkthrough check; :324-363 Claude/ACP detection.
- spektacular:tests/harbor/plan-workflow/tests/temp.py — stray debug script pointing at an old checkout; not run by test.sh.
- spektacular:tests/harbor/implement-workflow/tests/test_implement_workflow.py:98-104,332-370 changelog checks; :44,155-157 Claude-only; environment/state.json seeded at `test_plan`.
- spektacular:tests/harbor/implement-workflow/instruction.md:71-85 — shows the writes with backslash continuations; no "no one answers" wording.
- spektacular:tests/harbor/spec-workflow/tests/test_spec_workflow.py:559-570 — verification only checked as completed.
- spektacular:tests/harbor/goal-check-workflow/tests/test_goal_check.py:199-234,292-347 — reviewer launch rules, Claude-only skip pattern; environment/seed/seed.sh:73-82 seeds at `check_goal`.
- spektacular:tests/harbor/epic-workflow/tests/test_epic_workflow.py and implement-epic-workflow/tests/test_implement_epic_workflow.py — CLI and session-log only; `lane.children == 1`; no transcript reading.
- spektacular:tests/harbor/*/instruction.md — "no one answers" wording per suite (see evidence).
- spektacular:templates/skills/workflows/spek-plan-epic/SKILL.md:31,42-80,118 and spek-implement-epic/SKILL.md:45-106,121 — background, headless, child prompt, resume, cut-off rules.
- spektacular:templates/steps/spec/08-verification.md:25-36; templates/steps/implement/04-test.md, 05-verify.md, 05b-check_goal.md, 02-analyze.md, 07-update_changelog.md; templates/steps/plan/02-discovery.md:55.
- spektacular:templates/plan_epic_skill_test.go:113-194,446-455; templates/implement_epic_skill_test.go:132-245,417-440 — pinned headless phrases and the "ask" allowlist.
- spektacular:internal/agent/instruction_surface_test.go:32-147 — forbidden-substring list over templates and rendered skills.
- spektacular:internal/steps/implement/steps_test.go:319-340,740-1013; goalcheck_test.go:134-150 — pinned sub-agent wording in implement steps.
- spektacular:internal/sessionlog/record.go:18-56 — session-log event shape.
- spektacular:internal/agent/skills.go:24-71 — one rendering for every agent.
- tests/harbor/jobs/{s55,m70}-claude-{epic,implement-epic,spec,implement,plan} (gitignored) — reference transcripts and session logs (`agent/claude-code.txt`, `artifacts/spektacular/debug/*.jsonl`); `agent/sessions/` is root-owned and unreadable.

## External references

- Claude Code stream-json output (as recorded by Claude Code in the 2026-10-23 runs): `system/task_started|task_updated|task_notification|task_progress|background_tasks_changed`, `result.subagent_stats`, `result.origin`, `command_lifecycle.wakeup_source` — the field names this plan relies on; no published schema, so the reference runs are the source of truth.
- Harbor 0.23.0 — mounts `/logs/agent` at verify time and returns `/logs/verifier` readable (relied on by 000070, unchanged).

## Prior plans / specs consulted

- 000070_harbor-run-metrics (plan) — the recorder/report architecture this extends: pure functions over parsed logs, `gather` per section, unknown-with-reason, report reads only `metrics.json` and `result.json`, flags computed in the report with thresholds as module constants, README field reference kept in step by `test_readme_metrics.py`.
- 000071_harbor-child-agents (spec) — source of requirements; the spec-workflow working context records the 2026-10-23 reference runs and the user's choices (Claude enforced, others record unknowns; verify the ten-minute claim first).

## Open assumptions

- Claude Code keeps emitting `task_started`/`task_updated`/`task_notification` with the field names seen on 2026-10-23. If a newer Claude Code renames them, the child list degrades to unknowns, and the implement workflow must STOP and ask before changing the readers' contract.
- A depth-2 child's spec is the spec its own prompt names first, falling back to its top-level ancestor's spec; goal reviewers' prompts may not name the spec.
- A "timer" in the no-wake-up rule includes a background shell command that sleeps (s55 implement-epic's `sleep 240; spektacular status …` with `run_in_background`), besides `ScheduleWakeup` and `CronCreate`.
- Harbor runs (Docker, Claude credentials) can be started from the implementing machine for the ten-minute probe; if not, the implement workflow must STOP and ask, because the epic skill wording depends on the result.
- Trimming the reference runs (dropping message text, keeping structure, timestamps, launch inputs, task events and session-log argv/state) preserves every value the checks and recorder read; a trim that changes a computed value is a bug.
- `subagent_stats.spawned` counts every child actually started, including nested ones, and excludes refused launches.

## Drafting assumptions

### Chosen direction: one child reader shared by recorder and verifiers (architecture)
- **Decision**: Option A. A `Child` reader per log format feeds both the metrics recorder (placed by spec and step, plus per-step child counts and run idle time, schema 2) and a new pure-rule delegation module that Claude suite verifiers call. Walkthrough and changelog checks judge session-log events. Template changes: strengthen spec verification and implement test/verify, add worktree paths and the no-wake-up rule to the epic skills, change the headless wording only after a long-background-child probe run.
- **Key design decisions**: Claude-only enforcement with a skip for other agents; checks only where the steps run (not the implement suite); the plan discovery spawn check becomes record-only; the oracle is hand-kept and a self-check ties it to the templates' wording; reference runs checked in trimmed.
- **Rationale**: one parser cannot drift from itself; verifiers can fail runs, the report cannot; the session log records what actually ran.
- **Rejected**: (B) per-verifier transcript parsing (duplicates 000070's readers); (C) recorder-plus-report only (cannot fail a run).

### Schema version bumps to 2 (architecture)
- **Decision**: `schema_version: 2` for records with `subagents.children`, per-step child counts and `totals.idle_s`; the report keeps reading version 1 and shows child detail as not recorded.
- **Rationale**: the spec expects a bump when the shape changes; a version lets the report tell "no children" from "not recorded".
- **Rejected**: staying at 1 with additive keys (ambiguous for old records).

### Conventions selected (architecture)
- **Decision**: keep plans-never-change-the-active-install, tests-must-pass-for-done, tests-must-not-depend-on-order, store-files-through-the-CLI; drop error-messages-must-suggest-remediation.
- **Rationale**: no CLI error path changes; the other four bear directly on template edits, tests and store-access wording.
- **Rejected**: listing every convention.

### Prompt source for each child (discovery)
- **Decision**: take a child's prompt, description, kind and background request from its `Agent`/`Task` tool_use input, and join `task_started` by `tool_use_id` for task id, depth, parent and actual background state.
- **Rationale**: `task_started.prompt` is missing in some runs (s55 implement-epic), and background Bash tasks also emit `task_started`.
- **Rejected**: `task_started` alone; extending `SubagentLaunch` (pinned by equality tests).

### Idle time is the longest activity gap (discovery)
- **Decision**: idle = longest gap between consecutive activity instants (main and child message timestamps, child end times, session-log event start/end), in seconds.
- **Rationale**: matches the spec's definition and reproduces the reference figures (s55 epic 942 s of 1249 s, flagged; m70 epic 9%, not flagged).
- **Rejected**: session log alone (misses model time); sum of gaps over a threshold (not the spec's definition).

### Background sleeps count as timers (discovery)
- **Decision**: the no-wake-up check fails an orchestrator call to `ScheduleWakeup` or `CronCreate`, or a background shell command that sleeps, made while any child is running.
- **Rationale**: the spec bans "a timed wake-up, timer or recurring job"; the s55 implement-epic orchestrator polled with a background `sleep 240`.
- **Rejected**: only the two named tools (lets the polling sleep through).

### Where required-delegation checks run (discovery)
- **Decision**: spec suite (verification), goal-check suite (check_goal, and test/verify on the unmet retry), implement-epic (test, verify, check_goal per lane, one child per spec), epic (one child per spec). Not the implement suite.
- **Rationale**: the implement suite is seeded past test/verify/check_goal, so they never run there.
- **Rejected**: adding the check to the implement suite (it would test nothing).

### Unknown paths for children (data_structures)
- **Decision**: agent-wide child gaps are one `unknown` entry per field (`subagents.children.<field>`); a single child's gap is `subagents.children[<id>].<field>`; Codex's whole list is `null` with one entry.
- **Rationale**: keeps the unknown list short for Bob while still naming one-off gaps; follows the dotted-path convention of 000070.
- **Rejected**: a per-child `unknown` sub-list (two places to look); one entry per child per field (dozens of identical entries for Bob).

### `step: null` for orchestrator launches (data_structures)
- **Decision**: a child launched outside any step window has `step: null` with no unknown entry, documented as "started outside any workflow step".
- **Rationale**: an epic orchestrator's children are started from its own turn, not from a workflow step; that is a fact, not a gap.
- **Rejected**: a sentinel step name such as "orchestrator" (would be mistaken for a workflow step).

### Child output measured in characters (data_structures)
- **Decision**: a child's `output` is the characters of its own streamed messages, the step breakdown's unit.
- **Rationale**: one unit across the record; tokens are reported only in aggregate per child and not for Bob/Codex.
- **Rejected**: `usage.total_tokens` (mixes input and output); hand-back length (ignores the work).

### Trimmed reference data verified on write (implementation_detail)
- **Decision**: the trim script replaces message text with same-length placeholders and refuses to write a fixture whose recomputed acceptance values differ from the original's.
- **Rationale**: keeps fixtures small and free of conversation content while guaranteeing the offline acceptance checks mean the same as on the full run.
- **Rejected**: checking in whole job directories (megabytes, root-owned files, conversation text); hand-written fixtures (not the reference runs the spec names).

### Milestone order puts the probe last (milestones)
- **Decision**: record first (M1), session-log fixes (M2), enforcement (M3), then the probe and headless wording (M4).
- **Rationale**: the probe's verifier and its record reuse M1's readers; the constraint only requires the probe before the headless wording changes, which M4 keeps together; M1-M3 deliver value without waiting on a live run.
- **Rejected**: probe first (would need its own ad-hoc parsing before the readers exist).

### Probe and its run are agent tasks (tasks)
- **Decision**: adding and running the background-child probe are `agent` tasks; the run task stops and asks if Docker or the Claude login is unavailable.
- **Rationale**: harbor runs use the machine's own Claude login, as every Claude suite target does, so no secret beyond what the implementing agent already uses is needed.
- **Rejected**: a `human` task for the run (the run needs no access an agent on this machine lacks).

### Probe outcome handed over in a checked-in file (tasks)
- **Decision**: the probe run task writes a one-line `OUTCOME` beside the trimmed probe data; the skill-wording and epic-suite tasks read it.
- **Rationale**: the skills must state the recorded outcome, and the suites must pick the matching rule; a checked-in file survives sessions.
- **Rejected**: keeping the outcome only in session notes (lost between implement sessions).

### Spec split review left unenforced (out_of_scope)
- **Decision**: the spec `split` step's fresh-context review (required wording in `templates/partials/split-flow.md:19`) is not added to the oracle or enforced.
- **Rationale**: the spec's list of steps that currently require a child does not include it, and no harbor suite reaches the split step.
- **Rejected**: adding it to the oracle (would need a suite that splits a spec, outside this spec).

## Rehydration cues

- `spektacular spec file read 000071_harbor-child-agents`
- `spektacular plan file read 000070_harbor-run-metrics plan`
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`, and the gotchas `harbor-agent-log-fields.md`, `harbor-agent-logs-and-artifacts.md`.
- Re-read `tests/harbor/shared/{metrics,transcripts,report,sessionlog}.py` and their `shared/tests/` self-checks.
- Inspect a reference transcript: `python3` over `tests/harbor/jobs/s55-claude-epic/*/agent/claude-code.txt`, filtering `system/task_*`, `Agent` tool_use and `ScheduleWakeup`.
- `templates/skills/workflows/spek-{plan,implement}-epic/SKILL.md`, `templates/steps/spec/08-verification.md`, `templates/steps/implement/0{4,5,5b}-*.md`.
