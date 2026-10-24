---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Plan: 000071_harbor-child-agents

<!-- Metadata -->
<!-- Created: 2026-10-24T08:18:38Z -->
<!-- Commit: 74e0810bc3f560cbcea3f1ad100b28ca26104a32 -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Every harbor end-to-end run will record each child agent it started: who started it, what it was asked and the full context it was given, how it ran and ended, and the spec and workflow step it belonged to. The run will also record its idle time, and the local report will show it all. The Claude suites will then fail a run that skips a required child agent, gives a child incomplete context, or schedules a timed wake-up while children run. The epic suites will run children in the background as a real session does, once a recorded probe run has settled whether headless Claude Code stops long background children. Two verifier checks that misread how a command was typed will judge what actually ran instead. Maintainers can then see how delegation changes between models and releases, and silent regressions stop passing unnoticed, such as a skipped independent review or a session idling for a quarter of an hour.

## Conventions

- **A plan never changes the active skills and configuration** — this plan edits step and skill templates (spec verification, implement test/verify, both epic skills); they are proven with the Go test suite and harbor's per-suite builds into `tests/harbor/.build/`, never by migrating, re-initialising or regenerating this repo's own `.spektacular/` or installed skills.
- **Passing tests are required before calling work done** — every task ends with the Go test suite passing (the template-contract tests pin the epic headless phrases and the implement sub-agent wording), and the shared harbor self-checks pass under pytest; root-owned files under `tests/harbor/jobs/` that break the Go test suite must be resolved or raised, not dismissed.
- **Tests must not depend on execution order** — the new recorder, rule and report self-checks use checked-in reference data and fixed synthetic timestamps, never the wall clock or state another test left, so they pass shuffled and alone.
- **Spektacular's own files are written through the CLI** — the template changes keep `--from <path>` as the only way to supply a body (the instruction-surface test bans stdin/heredoc forms), and the new verifier rules judge store writes from the CLI's own session log rather than from file paths built by hand.

## Architecture & Design Decisions

All of the work lands in the `spektacular` repo, in four areas:
- the shared harbor helpers in `tests/harbor/shared/`: the transcript readers, the recorder, the report and a new delegation rules module;
- the suite verifiers and task instructions under `tests/harbor/<suite>/`;
- the step and skill templates under `templates/`, with their Go contract tests;
- the README's run-metrics reference.

No Go command changes. The website docs stay untouched (a non-goal). There are three options for the shape:
- **(A)** one child-agent reader that both the recorder and the verifiers use;
- **(B)** each verifier parses the transcript itself, as the plan suite does today, with the recorder separate;
- **(C)** compute everything in the recorder and judge it in the report.

(C) cannot fail a run, which the spec requires. (B) duplicates the parsing that 000070 already centralised, and the two copies would drift. **We take (A).** The comparison is in [research.md](research.md#alternatives-considered-and-rejected).

**One child list, built once per format, read by recorder and verifiers alike.** A new transcript reader turns Claude's stream-json into one `Child` per started child agent:
- **Identity and context:** the prompt, kind, description and requested background mode come from the `Agent`/`Task` call. The actual background state, depth, task id and parent come from `task_started`, joined on the tool-use id.
- **Ending:** the end time comes from `task_updated`. The status, the agent's own duration and the hand-back come from `task_notification`.
- **Models and output:** both come from the child's own streamed messages. Output is measured in characters, the same unit as the step breakdown.

Bob's trajectory gives a child list with kind, description and prompt, and everything else unknown. Codex has no child signal, so its whole list is unknown.

The recorder places each child in a spec and a step:
- **Spec:** the spec its own prompt names first, otherwise its top-level ancestor's spec.
- **Step:** the step window, from the session log, that holds its launch time.

The same placement gives each step entry in the breakdown a count of the children started in it. That is how a suggested delegation (plan discovery, implement analysis, changelog formatting) shows whether it happened. A run-level idle time is the longest gap in the activity clock: main and child message times, child end times, and session-log command times. The record's shape grows, so `schema_version` becomes 2. The report still reads version-1 records and prints their child detail as not recorded. Every new section goes through the recorder's per-section guard, so a parsing failure costs only that section and never the reward (the constraint 000070 already enforces).

**Delegation rules are pure functions over that child list, with a hand-kept oracle.** A new shared module holds:
- **The oracle:** the required delegations (spec `verification`; implement `test`, `verify` and `check_goal`; one child per spec in the two epic workflows) and the suggested ones.
- **The rules:** a required step completed with no child placed in it; a required child whose prompt lacks what its instructions say to pass; an orchestrator wake-up, cron job or background sleep while a child runs; and, for the epic suites, per-spec children started in the background with a turn ending while they run and a new turn woken by a completion notice.

Each Claude suite's verifier calls the rules and skips them for other agents. Per the constraint, enforcement is Claude-only. The checks go only where the steps actually run:
- the spec suite;
- the goal-check suite, whose unmet retry runs `test` and `verify`;
- implement-epic;
- epic.

The implement suite is seeded past those steps, so it gets none. The plan suite's discovery spawn check becomes record-only, because the constraint says a suggested child never fails a run.

Knowledge requires the harbor oracles to be hand-maintained and independent (*architecture/testing-architecture.md*), so the oracle is not derived from the templates at run time. A local self-check reads the step templates instead and fails when a step's delegation wording and the oracle disagree. That is how the check "follows the instructions if that set changes".

**The two false negatives judge what ran, from the session log.** The walkthrough check reads every plan advance in the plan lane: successful `goto` events with their `state_after.current_step`. The per-repo changelog checks read every successful `changelog file write` event and its `--repo` argument. However the command was written (piped, chained, looped or continued over lines), the CLI logged the argv it actually received. Both rules are pure functions over session-log events. They are proven against the checked-in, trimmed 2026-10-23 reference runs, which also back the idle and child-record acceptance criteria, so those criteria re-run without a live batch.

**Templates change in their source, and the epic guidance changes last, on evidence.**
- **Required delegation:** the user chose to strengthen the step instructions rather than only enforce them. The spec `verification` step and the implement `test` and `verify` steps state that an agent with a sub-agent tool must use it, and that doing the step inline does not complete it. `check_goal` is already explicit.
- **Epic child prompts:** `spek-implement-epic` gains a rule, also the user's choice, that each child's prompt names the spec's worktree paths, as `epic worktree` reports them. Both epic skills gain the rule never to schedule a wake-up, timer or recurring job while waiting on children.
- **Headless wording waits for the probe.** The constraint forbids changing it until the ten-minute limit has been verified in a recorded run. A small new harbor task therefore runs one deliberately long background child under headless Claude Code first. Its outcome decides the wording:
  - **If the child survives,** "headless" now means only that no one answers, and children run in the background with completion notices. The epic suites then check exactly that.
  - **If it is stopped,** the foreground workaround stays, the suites check the workaround instead, and the skills state the recorded outcome either way.

  Every suite keeps, or for plan and implement gains, the "headless, no one answers between your turns" wording, so the agent never waits on a question.

Template edits follow *a plan never changes the active skills and configuration*: they are proven by the Go test suite and harbor's own per-suite builds, never by re-initialising this repo.

## Component Breakdown

- **Child-agent readers (extended shared helper: the transcript module).**
  - **New:** a `Child` record and one reader per log format.
    - **Claude:** joins each `Agent`/`Task` call, at any depth, with its `task_started`, `task_updated` and `task_notification` events and its own streamed messages.
    - **Bob trajectory:** yields kind, description and prompt, with the rest unknown.
    - **Codex:** yields no list and one unknown.
  - **Also new:** a reader for orchestrator wake-up calls (`ScheduleWakeup`, `CronCreate`, background sleeping shell commands) and for turn boundaries (each turn's end, and what woke the next one).
  - **Unchanged:** the existing launch reader and its dataclass, which the goal-check verifier and its self-check depend on. These readers are the single source the recorder and the verifiers share.
- **Run recorder (changed shared helper: the metrics module).**
  - **New in the record:** the child list, with each child placed in its spec and step using the step windows it already builds; a child count on every step entry; and the run's idle time.
  - **Version:** the record moves to schema version 2.
  - **Unknowns:** per-agent unknowns, either per field across all children or per child for a single gap.
  - **Safety:** each new section has its own never-fail guard.
  - **For the verifiers:** it exposes the placement as a pure function, so they place children exactly as the record does.
- **Delegation rules (new shared helper).**
  - **The oracle:** the hand-kept list of required and suggested delegations, by workflow kind and step, plus the per-spec child rule for the epic workflows and each required child's mandatory context.
  - **The rules:** pure functions over placed children, step windows and turn boundaries. They return named failures:
    - a missing required child;
    - incomplete context;
    - a wake-up while children run;
    - a per-spec child not run in the background, or with no notice-driven turn.
  - **Who uses it:** the Claude suites' verifiers. It is copied into every suite with the other shared helpers. Justified as new because four suites need the same rules, and the recorder should not carry pass/fail judgement.
- **Run report (changed shared helper: the report module).**
  - **The run view:** gains a children table (kind, spec, step, background or foreground, duration, ending, context size), an idle line, and an idle mark when idle exceeds a quarter of wall time.
  - **A new option** prints every child's full context for one run.
  - **The run table:** marks idle runs.
  - **Older records:** it keeps reading version-1 records, showing child detail as not recorded.
- **Suite verifiers (changed: spec, plan, implement, goal-check, epic, implement-epic).**
  - **spec:** requires the review child and its context.
  - **plan:** the discovery spawn check becomes record-only, and the walkthrough-before-finished check reads the plan lane's advances from the session log.
  - **implement:** the per-repo changelog checks read successful writes from the session log.
  - **goal-check:** requires children for `check_goal`, and for `test` and `verify` on the retry.
  - **epic and implement-epic:** require one child per spec with full context, ban wake-ups, and check background children with notice-driven turns, or the foreground workaround if the probe found children stopped.
  - **All of them:** skip these rules for non-Claude agents.
- **Suite task instructions (changed: every suite's `instruction.md`).** Each keeps, and the plan and implement suites gain, the wording that the session is headless and no one answers between turns. The epic wording is unchanged; what changes is what the skills make of it.
- **Background-child probe (new small harbor task with a make target).** It runs one deliberately long background child, past ten minutes, under headless Claude Code. Its verifier records whether the child completed or was stopped, and whether a notice woke the orchestrator. It settles the ten-minute claim and can be re-run when Claude Code changes. Justified as new because no existing suite has a child that runs that long.
- **Step templates (changed: spec `verification`, implement `test` and `verify`).** They state that an agent with a sub-agent tool must delegate, and that doing the step inline does not complete it. What to hand the child is unchanged. Their Go template-contract tests pin the new wording.
- **Epic skills (changed: `spek-plan-epic`, `spek-implement-epic`).**
  - **Both:** gain the rule never to schedule a wake-up, timer or recurring job while waiting on children.
  - **The headless paragraph and the fresh-child-in-foreground rule:** rewritten to match the probe's outcome.
  - **`spek-implement-epic` only:** also requires each child's prompt to name the spec's worktree paths.
  - **Tests:** their Go skill tests, including the "ask" allowlists, follow.
- **Reference test data (new, checked in beside the shared self-checks).** The four 2026-10-23 reference runs (Sonnet 4.6 and 5.5 epic planning, Sonnet 5.5 plan and implement), trimmed. Plus a small trimming script, so the data can be regenerated and future runs added the same way. It backs the idle, child-record, walkthrough and changelog acceptance criteria offline.
- **Self-checks (extended local pytest).** Cover the new readers, placement, idle time, the delegation rules and the report views, using synthetic input and the reference data. One self-check ties the oracle to the step templates' delegation wording.
- **Harbor documentation (changed: the README's run-metrics section).** Documents the child fields, per-step counts, idle time and which agents leave which fields unknown, the new report option, and the probe target. Its README self-check follows.

## Data Structures & Interfaces

**Run metrics record, schema version 2** (`metrics.json`) is the contract between the recorder, the report and the README. Version 2 keeps every version-1 key and meaning. It adds three things:
- `subagents.children`, one entry per child agent started, nested children included;
- `children`, a count on each step entry;
- `totals.idle_s`.

A value the logs cannot show is `null`, with an `unknown` entry giving the reason. It is never `0`.

```jsonc
{
  "schema_version": 2,
  "totals": { "wall_time_s": 1249.4, "idle_s": 942.2, /* …version-1 totals… */ },
  "steps": { "unit": "characters", "workflows": [
    { "kind": "plan", "name": "20260101000001-storage",
      "steps": [ { "step": "discovery", "started_at": "…Z", "duration_s": 41.0, "output": 9120, "children": 0 } ] }
  ] },
  "subagents": {
    "started": 3, "completed": 3, "failed": 0, "stopped": 0, "durations_s": [109.6, 107.6, 137.0],
    "children": [
      { "id": "toolu_01Ls…",           // the launching call's id; stable across a run
        "parent": null,                // id of the child that started it; null = the main agent
        "kind": "general-purpose", "description": "Plan storage spec",
        "background": true,            // as actually run, not as requested
        "spec": "20260101000001-storage",
        "step": null,                  // null = started outside any workflow step (an orchestrator's own turn)
        "launched_at": "2026-10-23T13:36:31.983Z", "ended_at": "2026-10-23T13:38:21.592Z",
        "duration_s": 109.6,           // the agent's own report of the child's run time
        "ending": "completed",         // completed | failed | stopped | no_hand_back
        "hand_back": "DONE: 20260101000001-storage",
        "models": ["claude-sonnet-5-5"], "output": 61234,   // characters, same unit as steps
        "prompt": "Plan the spec `20260101000001-storage` …", "prompt_chars": 1723 }
    ]
  },
  "unknown": [
    { "field": "subagents.children.ended_at", "reason": "bob logs no sub-agent end time" },
    { "field": "subagents.children[toolu_09…].duration_s", "reason": "claude reported no duration for a failed child" }
  ]
}
```

- **How unknowns are written for children.** A gap that holds for every child of an agent is written once, as `subagents.children.<field>`. A gap in one child alone is written as `subagents.children[<id>].<field>`. For Codex, `children` itself is `null`, with the reason "codex reports no sub-agents".
- **What `step: null` means.** A `null` step that is not listed in `unknown` means the child was started outside any workflow step.

**Child (transcript module).** One child agent, as read from one log, before placement. Every reader returns `list[Child] | None` plus the unknowns it found.

```python
@dataclass
class Child:
    id: str; parent: str | None; kind: str | None; description: str | None
    background: bool | None; launched_at: datetime | None; ended_at: datetime | None
    duration_s: float | None; ending: str | None; hand_back: str | None
    models: list[str] | None; output: int | None; prompt: str | None

claude_children(objs) -> tuple[list[Child], list[Unknown]]
atif_children(trajectory) -> tuple[list[Child] | None, list[Unknown]]
claude_turns(objs) -> list[Turn]               # Turn(ended_at, woken_by: "start" | "task-notification" | "wakeup" | None)
claude_wakeups(objs) -> list[Wakeup]           # Wakeup(at, tool, detail) for ScheduleWakeup, CronCreate, background sleeps
```

**Placement and idle (metrics module).** These are pure functions, shared by the recorder and the verifiers.

```python
place_children(children, windows_by_lane) -> list[PlacedChild]   # Child + spec + step
step_child_counts(placed, windows_by_lane) -> dict[(lane, step_index), int]
idle_seconds(instants: list[datetime]) -> float | None             # longest gap; None with < 2 instants
```

**Delegation oracle and rules (new delegation module).** The oracle is literal data, maintained by hand:

```python
REQUIRED = {("spec", "verification"), ("implement", "test"), ("implement", "verify"), ("implement", "check_goal")}
SUGGESTED = {("plan", "discovery"), ("implement", "analyze"), ("implement", "update_changelog")}
PER_SPEC_CHILD = {"plan", "implement"}         # in an epic run, one top-level child per spec lane
REQUIRED_CONTEXT = {                           # phrase groups a required child's prompt must contain
    "epic-plan":      [spec_name, project_root, '"orchestrated":true'],
    "epic-implement": [spec_name, project_root, '"orchestrated":true', worktree_paths],
    "epic-resume":    [resume_instruction],    # only when the lane had an earlier child
    "spec-review":    [".spektacular/tmp/spec_template.md", "review only what is written"],
}

@dataclass
class Failure: rule: str; subject: str; detail: str    # e.g. ("missing_child", "implement:20260201000001-storage:test", "…")

missing_required(placed, windows_by_lane) -> list[Failure]
context_gaps(placed, lanes, root, worktrees) -> list[Failure]
wakeups_while_children_run(wakeups, placed) -> list[Failure]
epic_children_backgrounded(placed, turns, lanes) -> list[Failure]
epic_children_foreground_together(placed, turns, lanes) -> list[Failure]   # used only if the probe found children stopped
```

**Session-log rules (shared session-log module, called by the plan and implement verifiers).** Pure functions over `sessionlog.Event` lists, so the shared self-checks can prove them on the reference runs:

```python
plan_steps_entered(lane_events) -> list[str]                    # state_after.current_step of each successful goto/new
changelog_writes_by_repo(events) -> dict[str | None, int]       # None = the project record; key = --repo value
```

**Report interface.**
- `--run <trial>` gains the children table and the idle line.
- `--run <trial> --prompts` also prints each child's full context.
- `--prompts` without `--run` is a usage error, which exits with status 2.
- `IDLE_SHARE = 0.25` joins the threshold constants at the top of the module.

**Probe task result.** The probe verifier writes its outcome into its `metrics.json` like any run: one background child with `ending` and `duration_s`, plus the turn records. The outcome that decides the skills' wording is `completed` or `stopped` with a duration above 600 s.

## Implementation Detail

**Readers stay pure, and placement happens once.** The new child, turn and wake-up readers follow the shared helpers' existing shape: pure functions over the parsed log, each returning plain data together with the unknowns it found. The join rule is fixed. A child is identified by its launching call, the call's own input supplies the context, and the lifecycle events fill in the rest by matching that call. A launch the agent refused never gets a lifecycle event, so it is never counted as a child. The same goes for background shell tasks, which also emit lifecycle events. Placement in a spec and a step is a separate pure step over the children and the session log's step windows. The recorder and the verifiers both call it, so a verifier failure and the record always agree on where a child was. The recorder no longer discards the windows it builds; it hands them to placement.

**A new judgement layer sits beside the recorder.** Until now, the shared helpers only measured, and each verifier carried its own judgement. The delegation module introduces a shared judgement layer:
- a literal oracle at the top;
- small rules below it, each returning named failures;
- no I/O.

A suite's verifier does three things: reads the logs, runs the rules that apply to it, and asserts that the failures list is empty. That gives the failure message the step or spec and what was missing, as the acceptance criteria ask. This follows the goal-check verifier's pure-rule style (`rule_*` functions with a self-check), which becomes the model for the spec, plan, implement and epic verifiers too. Each verifier gets a Claude guard that skips these rules for any other agent, the oracle run included, as the goal-check suite already does.

**Reference runs become checked-in data, through a repeatable trim.** A small script turns a collected job into a compact fixture:
- **Kept from the transcript:** every object's type, subtype, timestamp, parent link and ids; every launch call's input in full; every lifecycle event; and every result event's turn fields. Message text is replaced by a placeholder of the same length, so character counts survive.
- **Kept from the session log:** argv, exit code, timestamps and state, with responses reduced to their step.
- **Re-checked:** the script recomputes the values the acceptance criteria name (child counts, idle time, steps entered, changelog writes) on the trimmed copy and refuses to write it if any differs from the original.

The self-checks then run the real rules against real runs, offline. Adding a future reference run is one command.

**Template changes are small, local and pinned.**
- **Required steps:** each required-delegation step gains one plainly worded rule: an agent with a sub-agent tool must use it, and doing the work inline does not complete the step. The existing fallback for agents with no sub-agent tool stays.
- **Epic skills:** they gain two rules, one paragraph each: the no-wake-up rule, and, for implementing, the worktree paths in the child's prompt. The headless paragraph is then rewritten once, to match the probe's recorded outcome, and states that outcome.
- **Tests:** every change is pinned the way these templates already are, by phrase assertions in the Go contract tests. New text containing a word starting with "ask" is added to the skills' "ask" allowlists.
- **Oracle tie:** the self-check that ties the delegation oracle to the templates reads the template sources for the agreed delegation phrases. The oracle is never generated from them.

**The probe is a minimal harbor task, not a workflow suite.** It has a tiny image and a one-paragraph instruction: start one background child that runs past ten minutes, end the turn, and finish when its notice arrives. Its verifier uses only the shared readers and the recorder, so the probe's record is a normal `metrics.json`. It follows the existing render-target pattern, so the wiring self-check covers it. Its trimmed run joins the reference data. The skills' wording task depends on it, so the epic guidance changes only after the outcome is on record.

**What a developer sees.**
- **The report:** one more table in the run view, and one option for the full contexts.
- **The verifiers:** short rule calls in place of hand-rolled transcript scans.
- **The README:** one place that documents every field, which agents leave which unknown, and how to re-run the probe.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **The run metrics recorder and report from 000070 (already landed).** This is the base the plan extends: the per-section never-fail guard, the unknown-with-reason convention, step windows from the session log, and the record-only report. Changed by this plan. Nothing has to land first.
- **The shared transcript and session-log readers.** They gain readers for children, turns and wake-ups. The existing functions and the `SubagentLaunch` dataclass keep their signatures, because the goal-check verifier and its self-check import them.
- **Claude Code's stream-json output.** The plan relies on the `Agent`/`Task` tool-use input, the `system/task_started`, `task_updated` and `task_notification` events, `parent_tool_use_id` on child messages, `result.subagent_stats`, `result.origin`, and the `command_lifecycle` wake-up events, with field names as recorded on 2026-10-23. Used as it is. A rename would show up as unknowns, not failed runs.
- **Spektacular's session log (`SPEKTACULAR_SESSION_LOG=on` in every suite image).** Every command's argv, exit code and resulting step. The new walkthrough and changelog rules, and child placement, read it. No Go change.
- **Harbor 0.23.0 and Docker, with the machine's Claude credentials.** These are needed to run the background-child probe and the end-to-end batch. The probe must run, and its outcome be recorded, before the epic skills' headless wording task starts.
- **The 2026-10-23 reference runs (`tests/harbor/jobs/m70-claude-epic`, `s55-claude-epic`, `s55-claude-plan`, `s55-claude-implement`).** They exist locally only, because the jobs directory is gitignored. The trim task must run while they are still on disk. If they are gone, those acceptance criteria cannot be proven offline, and the implement workflow must stop and ask.
- **pytest for the shared self-checks (run through uv, as today); the Go toolchain for the template-contract tests.** Both are already used. No new library is added, and the helpers stay stdlib-only.
- **The step and skill templates and their Go contract tests.** Spec `verification`, implement `test` and `verify`, `spek-plan-epic` and `spek-implement-epic` change in their source under `templates/`, per the constraint. Generated copies are never edited.
- **The epic worktree command's response.** It names each repo's worktree path for a spec. `spek-implement-epic` passes those paths to the child, and the context rule checks them. Used as it is.

## Testing Approach

**Test types.** The layers are the three the harbor work already uses, plus the template-contract layer for the prose changes.

1. **Local self-checks (unit, pytest, no container).** Almost all coverage goes here:
   - **What they cover:** the child, turn and wake-up readers; placement and idle time; the delegation rules; the session-log walkthrough and changelog rules; the report's new views; and the trim script.
   - **Synthetic input:** hand-built stream-json, trajectories and session-log events pin each rule in both directions, one input that passes and a minimal change that fails.
   - **Reference data:** the checked-in, trimmed 2026-10-23 runs prove the rules on real agent output. These are the acceptance criteria that name those runs.
   - **Conventions followed:** the existing self-checks' rules hold. Data is fixed, nothing depends on the wall clock or on test order, and the checks are runnable without Docker.
2. **Template-contract tests (Go).** These cover the new wording in the spec `verification` and implement `test`/`verify` steps and in both epic skills, plus the rewritten headless paragraph. Each is pinned by phrase assertions, the way these templates are already tested. Retired phrases are asserted absent, and the "ask" allowlists are updated.
3. **Regression of the existing harness.** The existing verifier self-checks (goal-check, repo, standing-rules), the 000070 recorder and report self-checks (updated for schema 2) and the Go test suite all still pass. The suite-wiring self-check covers the new probe target.
4. **End-to-end harbor runs (manual).** These are the only proof that the record, rules and wording hold with real agents in containers. They need Docker and credentials, and they are not in CI.

**Load-bearing assertions.**
- *Every child appears.* On the trimmed Claude epic-implementing reference run, the child count equals the transcript's own `subagent_stats.spawned`. Each nested child names the child that started it as its parent. Refused launches and background shell tasks are not counted.
- *Context reads back in full.* Each child's recorded prompt is byte-identical to its launch call's prompt, and the recorded prompt size equals its length.
- *Details match the transcript.* Kind, description, background mode, start and end times, models and output agree with the transcript. Duration equals the notification's own reported run time. Ending and first hand-back line match what the child returned. A failed child with no duration reads unknown, never zero.
- *Placement.* In an epic-implementing run, every top-level child, and every child nested under it, is recorded against the spec it worked on. A goal-check reviewer is recorded against `check_goal`.
- *Unknowns, never zeros.* A Codex record has `children` null with a reason. A Bob record has kind, description and prompt filled, and every other child field null and listed as unknown. No child field holds `0` in place of unknown.
- *Required delegation fails, suggested never does.* For each required step, a run without a child placed in it fails, naming the step or spec, and the same run with the child passes. A plan run whose discovery has no child passes every delegation rule, and its record shows `children: 0` on discovery.
- *Incomplete context fails.* Each of the following fails, naming the child and the missing item:
  - an epic child's prompt missing its spec, the project root or `"orchestrated":true`;
  - an implementing child's prompt missing its worktree paths;
  - a resumed child's prompt missing the resume instruction;
  - a spec reviewer not given the staged spec or the review-only-what-is-written instruction.
- *Wake-ups fail an epic run.* A `ScheduleWakeup`, a `CronCreate` or a background sleep issued by the orchestrator while a child runs fails, naming the call. The s55 epic-planning reference run fails this rule.
- *Background children and notice-driven turns.* An epic run passes only when its per-spec children ran in the background, a turn ended while they ran, and a turn was woken by a completion notice. (If the probe found children stopped, this inverts to the foreground workaround.)
- *Idle time.* The s55 epic-planning reference run records at least 15 minutes idle and is flagged. The m70 run is not flagged. The flag triggers above a quarter of wall time and not at or just below it.
- *Session-log rules.* The s55 plan reference run passes walkthrough-before-finished. A lane that jumps from `write_research` to `finished` fails. The s55 implement reference run passes both per-repo changelog rules, and removing one repo's successful write fails that repo's rule.
- *Oracle and templates agree.* Every step template carrying the required-delegation wording is in the oracle, and every oracle entry's template carries it.
- *Report.* The run view prints one row per child with kind, spec, step, background or foreground, duration, ending and context size, and an idle line. The prompts option prints each full context. Using the prompts option without a run is a usage error. A version-1 record shows child detail as not recorded.
- *Never fails a run.* A broken child or idle section yields a record with those fields unknown and the reward untouched.

**Success metrics.**
- *100% of child agents in a Claude run appear in its record with full context, across all nine Claude suites.*
  - **Behavioural test:** the count and prompt-identity assertions above, on the reference runs.
  - **Manual — captured in the implementation test plan:** confirming it on a full Claude batch of every suite.
- *A full Claude batch on the current default model passes, with no required delegation skipped and no run flagged for idle time.*
  - **Manual — captured in the implementation test plan.**
- *Epic-planning idle time on the current default model is under a quarter of wall time.*
  - **Behavioural test** for the measurement and the flag threshold.
  - **Manual — captured in the implementation test plan:** the real post-change epic-planning run.
- *Zero failures in a full Claude batch come from the walkthrough or per-repo changelog checks misreading how a command was written.*
  - **Behavioural test:** the reference runs whose commands were piped, chained and looped now pass.
  - **Manual — captured in the implementation test plan:** the full batch.
- *A maintainer can answer "which children did this run start, and what were they told?" from the local report alone.*
  - **Behavioural test:** the run view and the prompts option.
  - **Manual — captured in the implementation test plan:** answering it for a real collected run without opening the transcript.

**Manual reviews before acceptance.**
- **Manual — captured in the implementation test plan:** the recorded probe run exists with a background child that ran past ten minutes, and the epic skills state its outcome and keep children in the foreground only if the child was stopped.
- **Manual — captured in the implementation test plan:** a passing Claude epic-planning and epic-implementing run each record per-spec children in the background, with a notice-driven turn, and every spec is done.
- **Manual — captured in the implementation test plan:** no passing run's transcript ends with the agent waiting on a reply, and every suite's task tells the agent no one answers between turns.
- **Manual — captured in the implementation test plan:** a Codex run's and a Bob run's records mark the child fields they cannot show as unknown with reasons.
- **Manual — captured in the implementation test plan:** the README's run-metrics section documents the new fields, unknowns, report option and probe target, and its commands work.

**Deliberate gaps.**
- No automated container test. The harbor suites stay out of CI.
- No Go tests beyond the template-contract pins, because no Go code changes.
- Codex and Bob get no delegation enforcement (the spec's Claude-only constraint), only record shape checks.

## Milestones & Tasks

### Milestone 1: Every harbor run records the child agents it started and its idle time

**What changes**: Every harbor run's metrics record lists each child agent the run started, nested children included. Each entry shows who started it, its kind and description, the full context it was given and that context's size, whether it ran in the background, when it started and ended, how long it ran, its models and output, how it ended and the first line of its hand-back. It also shows the spec and workflow step it was started in. Every step in the breakdown shows how many children it started, so suggested delegations such as plan discovery are visible. Each run records its idle time, the longest stretch with no activity. The local report shows a run's children in a table and an idle line, prints every child's full context on request, and flags runs idle for more than a quarter of their wall time. Codex and Bob records mark the child details they cannot show as unknown. The 2026-10-23 reference runs are checked in, trimmed, so all of this is proven offline. Pass-or-fail results do not change in this milestone.

**Validation point**: The shared self-checks pass. On the trimmed reference runs:
- the child count equals the transcript's own;
- every recorded context is identical to its launch prompt;
- the s55 epic-planning run records at least 15 minutes idle and is flagged, and the m70 run is not.

The Go test suite and the existing verifier self-checks still pass. The README documents the new fields and report option.

#### - [x] Task: Read every child agent from each agent log
**Id:** 0daf6e29-e43b-4906-acbb-4b2b98293f28
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Teach the shared transcript helpers to read each child agent a run started, at any depth, together with its full launch context, its lifecycle and its hand-back. The same work adds readers for each turn's end and what woke the next one, and for any timed wake-up the main agent scheduled. Claude's log yields every detail. Bob's yields the kind, description and prompt, and Codex's yields nothing, recorded as unknown. The existing launch reader stays exactly as it is, because the goal-check verifier depends on it.

*Technical detail:* [context.md#task-read-every-child-agent-from-each-agent-log](./context.md#task-read-every-child-agent-from-each-agent-log)

**Acceptance criteria**:
- [x] A Claude log with top-level and nested children yields one entry per child actually started, each naming the child that started it, with refused launches and background shell tasks left out.
- [x] Each child's context is exactly the prompt it was launched with, and its duration is the agent's own reported run time.
- [x] A background child is not treated as finished when its launch call returns, and a failed child with no reported duration reads unknown rather than zero.
- [x] Bob children carry kind, description and prompt, with every other detail unknown and a reason. Codex reports no child list, with a reason.
- [x] Turn ends, notice-driven wake-ups and scheduled wake-ups or background sleeps are read with their times.
- [x] The existing readers and their self-checks are unchanged and still pass.

#### - [x] Task: Record children step counts and idle time
**Id:** 2491f7f6-e3ba-406e-a0c1-7c482e54d6c0
**Repo:** spektacular
**Depends on:**
- 0daf6e29-e43b-4906-acbb-4b2b98293f28 — Read every child agent from each agent log
**Execution:** agent

Extend the run metrics record, now schema version 2, with the child list. Each child is placed in the spec and workflow step it was started in. Every step entry gains a count of the children it started, and the run gains an idle time, the longest stretch with no activity. Placement is a shared function, so verifiers can use it too. Each new section is guarded, so a failure costs only that section and never the run's result.

*Technical detail:* [context.md#task-record-children-step-counts-and-idle-time](./context.md#task-record-children-step-counts-and-idle-time)

**Acceptance criteria**:
- [x] A Claude record lists every child with all the spec's details, each placed in its spec and step, and a child started outside any workflow step shows no step.
- [x] In a multi-spec run, nested children are recorded against the spec their parent worked on, and a goal-check reviewer is recorded against the goal-check step.
- [x] Every step entry shows how many children it started, including zero.
- [x] The record holds the run's idle time, and a run with too little activity to measure reads unknown.
- [x] Codex and Bob records mark every child detail they cannot show as unknown with a reason, and no unknown is written as zero.
- [x] A broken child or idle section still yields a record, with the reward untouched.

#### - [x] Task: Check in the trimmed 2026-10-23 reference runs
**Id:** 93011438-9b56-4123-8d47-510248854341
**Repo:** spektacular
**Depends on:**
- 2491f7f6-e3ba-406e-a0c1-7c482e54d6c0 — Record children step counts and idle time
**Execution:** agent

Add a small, repeatable trimming script and use it to check in the four reference runs the spec names: Sonnet 4.6 and 5.5 epic planning, and Sonnet 5.5 plan and implement. The script keeps the structure, timestamps, launch contexts, lifecycle events and session-log commands, and blanks message text to the same length. It refuses to write a fixture whose recomputed values differ from the original run's. Self-checks then prove the child record and idle acceptance criteria on these runs offline.

*Technical detail:* [context.md#task-check-in-the-trimmed-2026-10-23-reference-runs](./context.md#task-check-in-the-trimmed-2026-10-23-reference-runs)

**Acceptance criteria**:
- [x] The four reference runs are checked in, compact and free of conversation text, and the script that produced them can regenerate them.
- [x] On the 5.5 epic-planning run, the record's child count equals the transcript's own, every recorded context is identical to its launch prompt and its size equals that prompt's length.
- [x] The 5.5 epic-planning run records at least 15 minutes idle; the 4.6 epic-planning run records less than a quarter of its wall time.
- [x] Trimming a run never changes a value the checks compute from it.

#### - [x] Task: Show children and contexts in the report
**Id:** 7aacc18a-0c33-49c1-99f1-3a4a97b49780
**Repo:** spektacular
**Depends on:**
- 2491f7f6-e3ba-406e-a0c1-7c482e54d6c0 — Record children step counts and idle time
**Execution:** agent

Extend the local report so a maintainer can see, for any collected run, which children it started and what they were told, without opening the transcript. One run's view gains a children table, one row per child, and an idle line. A new option prints each child's full context. Runs idle for more than a quarter of their wall time are flagged in both the run table and the run view.

*Technical detail:* [context.md#task-show-children-and-contexts-in-the-report](./context.md#task-show-children-and-contexts-in-the-report)

**Acceptance criteria**:
- [x] One run's view prints one row per child with kind, spec, step, background or foreground, duration, ending and context size, and the run's idle time.
- [x] Asking for contexts prints every child's full context, and asking for them without naming a run is a clear usage error.
- [x] A run idle for more than a quarter of its wall time is flagged, and one at or below a quarter is not.
- [x] Unknown child details print as unknown, and older records without child detail say it was not recorded.

#### - [x] Task: Document the new record fields and report option
**Id:** 8abad8a2-3b2f-477c-ad4c-9e27ec7bc863
**Repo:** spektacular
**Depends on:**
- 2491f7f6-e3ba-406e-a0c1-7c482e54d6c0 — Record children step counts and idle time
- 7aacc18a-0c33-49c1-99f1-3a4a97b49780 — Show children and contexts in the report
**Execution:** agent

Update the README's run-metrics reference for schema version 2. That means the child fields, the per-step child count, idle time, which agents leave which child details unknown, the idle flag and the report's new option. The README self-check keeps the documented fields in step with the record.

*Technical detail:* [context.md#task-document-the-new-record-fields-and-report-option](./context.md#task-document-the-new-record-fields-and-report-option)

**Acceptance criteria**:
- [x] The README names every new field, says what it means and which agents leave it unknown, and shows how to print a run's children and their contexts.
- [x] The README self-check passes against the version-2 record.

### Milestone 2: Verifier checks judge what ran, not how a command was typed

**What changes**: The plan suite's walkthrough-before-finished check and the implement suite's per-repo changelog checks read the workflow's own session log, so piping, chaining, shell loops or line continuations no longer make a correct run fail. The plan suite stops failing a run whose discovery step used no child agent. Discovery only suggests one, and the record from Milestone 1 now shows whether it did. The plan and implement suites' tasks now say, as every other suite's already does, that the session is headless and no one answers between turns.

**Validation point**:
- The s55 plan reference run passes the walkthrough rule, and a lane that skips the walkthrough fails it.
- The s55 implement reference run passes both per-repo changelog rules, and removing one repo's write fails that repo's rule.
- A plan run without a discovery child passes.
- The Go test suite passes.

#### - [x] Task: Judge the plan walkthrough from the session log
**Id:** 0f375149-4bad-4631-aaba-8c98cf596ee9
**Repo:** spektacular
**Depends on:**
- 93011438-9b56-4123-8d47-510248854341 — Check in the trimmed 2026-10-23 reference runs
**Execution:** agent

Make the plan suite's walkthrough-before-finished check count every plan workflow advance the agent made, from the workflow's own session log, however the command was written. Also turn the discovery spawn check into a record-only note, because discovery only suggests a child agent and a suggested child must never fail a run.

*Technical detail:* [context.md#task-judge-the-plan-walkthrough-from-the-session-log](./context.md#task-judge-the-plan-walkthrough-from-the-session-log)

**Acceptance criteria**:
- [x] The 5.5 plan reference run, whose late advances were piped and chained, passes the walkthrough-before-finished check.
- [x] A run that goes straight from writing the documents to finished still fails it.
- [x] A plan run whose discovery used no child agent no longer fails, and its record shows discovery started none.

#### - [x] Task: Judge per-repo changelog writes from the session log
**Id:** 80315b59-a537-4807-ae13-97004ea26857
**Repo:** spektacular
**Depends on:**
- 93011438-9b56-4123-8d47-510248854341 — Check in the trimmed 2026-10-23 reference runs
**Execution:** agent

Make the implement suite's project and per-repo changelog checks accept every successful changelog write that actually ran for that repo, read from the workflow's session log. Writes in a loop, with flags reordered or continued over lines, then count the same as a plain command.

*Technical detail:* [context.md#task-judge-per-repo-changelog-writes-from-the-session-log](./context.md#task-judge-per-repo-changelog-writes-from-the-session-log)

**Acceptance criteria**:
- [x] The 5.5 implement reference run, which wrote both repos' records in a shell loop, passes both per-repo changelog checks.
- [x] A run that never successfully writes a repo's record still fails that repo's check, and a failed write does not count.
- [x] The project-level record check behaves the same way.

#### - [x] Task: Tell the plan and implement agents that no one answers
**Id:** 55e5ab4f-eef8-4267-bb25-7295b00df4c1
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add to the plan and implement suites' tasks the same wording the other suites already carry: the session is headless, no one answers between turns, and the agent must never end a turn waiting on a reply. Every suite then tells the agent this, so it never blocks on a question.

*Technical detail:* [context.md#task-tell-the-plan-and-implement-agents-that-no-one-answers](./context.md#task-tell-the-plan-and-implement-agents-that-no-one-answers)

**Acceptance criteria**:
- [x] Every harbor suite's task tells the agent the session is headless and no one answers between its turns.
- [x] The plan suite's stale status wording matches the document status the verifier checks.

### Milestone 3: Required delegation and the context children are given are enforced

**What changes**: A Claude test run now fails, naming the step or spec, when a step whose instructions require a child agent completes without one. Those steps are: the spec's independent review, the implement test, verify and goal-check steps, and one child per spec in the epic workflows. A run also fails when a required child's context lacks what its instructions say to pass. The spec, implement and epic instructions are made explicit so agents actually delegate:
- The spec review and the implement test and verify steps say that doing them inline does not complete them.
- The implement-epic skill has each child's prompt name the spec's worktree paths.
- Both epic skills forbid scheduling a timed wake-up, timer or recurring job while waiting on children, and the epic suites fail a run that does.

Codex and Bob runs are recorded but never failed by these rules.

**Validation point**:
- For each required step, a synthetic run without the child fails and the same run with it passes.
- Each missing context item fails, naming the child.
- The s55 epic-planning reference run fails the wake-up rule.
- The self-check tying the oracle to the templates passes.
- The Go test suite passes with the new template phrases pinned.

#### - [ ] Task: Make required delegation explicit in the step instructions
**Id:** 5460da84-b00f-46a3-95f7-84cf803efe6b
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Strengthen the spec workflow's independent review step and the implement workflow's test and verify steps so agents reliably delegate them. An agent that has a sub-agent tool must use it, and doing the step in its own context does not complete it. The existing fallback for agents without such a tool stays. The change is made in the template source and pinned by the template-contract tests.

*Technical detail:* [context.md#task-make-required-delegation-explicit-in-the-step-instructions](./context.md#task-make-required-delegation-explicit-in-the-step-instructions)

**Acceptance criteria**:
- [ ] The spec review, test and verify steps each say plainly that an agent with a sub-agent tool must delegate the step and that doing it inline does not complete it.
- [ ] What each step tells the agent to hand its child is unchanged.
- [ ] The template-contract tests pin the new wording and pass.

#### - [ ] Task: Add worktree paths and the no-wake-up rule to the epic skills
**Id:** 00c54dac-455c-43ba-b358-a3190d71b018
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Change both epic skills, in their template source, so the orchestrator never schedules a timed wake-up, timer or recurring job while waiting on children. It waits for their completion notices instead. Change the implement-epic skill so each child's prompt names the spec's worktree paths, as the worktree command reports them. The headless guidance is not touched here; it waits for the probe.

*Technical detail:* [context.md#task-add-worktree-paths-and-the-no-wake-up-rule-to-the-epic-skills](./context.md#task-add-worktree-paths-and-the-no-wake-up-rule-to-the-epic-skills)

**Acceptance criteria**:
- [ ] Both epic skills tell the orchestrator never to schedule a timed wake-up, timer or recurring job while waiting on children.
- [ ] The implement-epic skill requires each child's prompt to name the spec's worktree paths.
- [ ] The headless paragraph is unchanged, and the skill tests pin the new rules and pass.

#### - [ ] Task: Add the delegation rules and their oracle
**Id:** 128992a9-5756-4425-8353-1bfcf04b33fb
**Repo:** spektacular
**Depends on:**
- 2491f7f6-e3ba-406e-a0c1-7c482e54d6c0 — Record children step counts and idle time
- 5460da84-b00f-46a3-95f7-84cf803efe6b — Make required delegation explicit in the step instructions
- 00c54dac-455c-43ba-b358-a3190d71b018 — Add worktree paths and the no-wake-up rule to the epic skills
**Execution:** agent

Add a shared module of pure delegation rules with a hand-kept oracle of required and suggested delegations and each required child's mandatory context. Its rules name a required step completed without a child, a required child whose context is incomplete, and an orchestrator wake-up while children run. A self-check ties the oracle to the step templates' delegation wording, so the check follows the instructions when that set changes.

*Technical detail:* [context.md#task-add-the-delegation-rules-and-their-oracle](./context.md#task-add-the-delegation-rules-and-their-oracle)

**Acceptance criteria**:
- [ ] For each required step, a run without a child in it fails naming that step, and the same run with the child passes.
- [ ] A run whose suggested steps used no child passes every rule.
- [ ] Each missing context item fails naming the child and the item: spec, project root, orchestrated run, worktree paths, resume instruction, staged spec and review-only-what-is-written.
- [ ] The 5.5 epic-planning reference run fails the wake-up rule, naming the scheduled wake-up, and the 4.6 run passes it.
- [ ] Removing a step from the oracle, or its delegation wording from its template, fails the self-check that ties them.

#### - [ ] Task: Enforce delegation in the spec and goal-check suites
**Id:** 5128aaa7-c8ef-4b10-b3ce-1552d15d1d85
**Repo:** spektacular
**Depends on:**
- 128992a9-5756-4425-8353-1bfcf04b33fb — Add the delegation rules and their oracle
**Execution:** agent

Have the spec suite fail a Claude run whose independent review was done without a child, or whose reviewer was not given the staged spec and the instruction to review only what is written. Have the goal-check suite fail a Claude run whose goal check, or whose test and verify steps on the retry, completed without a child. Both suites skip these rules for other agents.

*Technical detail:* [context.md#task-enforce-delegation-in-the-spec-and-goal-check-suites](./context.md#task-enforce-delegation-in-the-spec-and-goal-check-suites)

**Acceptance criteria**:
- [ ] A Claude spec run that reviews inline fails, naming the review step, and one with a correctly briefed review child passes.
- [ ] A Claude goal-check run missing a child at the goal check, or at test or verify on the retry, fails, naming the step.
- [ ] Codex and Bob runs of both suites are not failed by these rules.

#### - [ ] Task: Enforce delegation and wake-up rules in the epic suites
**Id:** 8483e836-8b57-490b-b33f-91d1fa3e4700
**Repo:** spektacular
**Depends on:**
- 128992a9-5756-4425-8353-1bfcf04b33fb — Add the delegation rules and their oracle
- 00c54dac-455c-43ba-b358-a3190d71b018 — Add worktree paths and the no-wake-up rule to the epic skills
**Execution:** agent

Give the epic planning and implementing verifiers Claude-only rule checks. Every spec must have its own child with full context, including worktree paths for implementing and the resume instruction when a lane was continued. In implementing runs, every test, verify and goal-check step in a spec's lane must have its child. The orchestrator must not schedule any wake-up, timer or recurring job while children run. The suites stay agent-independent for everything else.

*Technical detail:* [context.md#task-enforce-delegation-and-wake-up-rules-in-the-epic-suites](./context.md#task-enforce-delegation-and-wake-up-rules-in-the-epic-suites)

**Acceptance criteria**:
- [ ] A Claude epic run with a spec planned or implemented without its own child fails, naming the spec.
- [ ] A Claude epic run whose child context omits any required item fails, naming the child and what was missing.
- [ ] A Claude implement-epic run whose test, verify or goal-check step in any spec's lane had no child fails, naming the spec and step.
- [ ] A Claude epic run whose orchestrator schedules a wake-up, timer or recurring job while children run fails, naming the call.
- [ ] Codex and Bob epic runs are checked exactly as before.

### Milestone 4: The ten-minute background limit is settled, and the epic suites run children as a real session does

**What changes**: A small new harbor task runs one deliberately long background child, past ten minutes, under headless Claude Code, and records whether it completed or was stopped. The epic skills state that outcome:
- **If the child survived,** "headless" means only that no one answers between turns. Children run in the background with completion notices, and the epic planning and implementing suites check exactly that: background per-spec children, the orchestrator ending its turn and a completion notice starting the next.
- **If it was stopped,** the skills keep the foreground workaround, and the suites check that instead.

Either way, telling the agent no one answers stops forcing foreground children on its own.

**Validation point**:
- The probe's trimmed record is checked in, with a child that ran past 600 s and its ending.
- The epic skills' headless paragraph matches that outcome, pinned by the Go test suite.
- The epic suites' background (or foreground) rule passes its self-check on synthetic runs.

#### - [ ] Task: Add the background-child probe task
**Id:** dd40f19a-5513-44de-86c1-a40a0fa1133f
**Repo:** spektacular
**Depends on:**
- 2491f7f6-e3ba-406e-a0c1-7c482e54d6c0 — Record children step counts and idle time
**Execution:** agent

Add a minimal harbor task, with a make target, that has headless Claude Code start one background child which deliberately runs past ten minutes. The orchestrator ends its turn and finishes only when the child's completion notice arrives. Its verifier records whether the child completed or was stopped and how long it ran, using the shared readers, and the run leaves a normal metrics record.

*Technical detail:* [context.md#task-add-the-background-child-probe-task](./context.md#task-add-the-background-child-probe-task)

**Acceptance criteria**:
- [ ] The probe can be run with one make target, like every other suite, and its wiring passes the suite-wiring self-check.
- [ ] Its verifier reports the child's ending and duration, and whether a completion notice started a new turn, without depending on which way the outcome goes.

#### - [ ] Task: Run the probe and record the ten-minute outcome
**Id:** 37a9fe91-7e9c-4ae0-b1f3-bb392fde38b1
**Repo:** spektacular
**Depends on:**
- dd40f19a-5513-44de-86c1-a40a0fa1133f — Add the background-child probe task
- 93011438-9b56-4123-8d47-510248854341 — Check in the trimmed 2026-10-23 reference runs
**Execution:** agent

Run the probe once against Claude, trim the run with the reference-data script and check it in, so the ten-minute claim is settled by a recorded run. Write down the outcome, completed or stopped and the child's duration, for the next task to act on. If Docker or the machine's Claude login is not available, stop and ask rather than guess the outcome.

*Technical detail:* [context.md#task-run-the-probe-and-record-the-ten-minute-outcome](./context.md#task-run-the-probe-and-record-the-ten-minute-outcome)

**Acceptance criteria**:
- [ ] A recorded probe run is checked in with a background child that ran for more than ten minutes in a headless session.
- [ ] Its outcome, completed or stopped, and the child's duration are written down where the skill-wording task reads them.

#### - [ ] Task: Rewrite the epic headless guidance to the probe outcome
**Id:** 5d07bd37-e6cf-45fd-b9f6-cf8bab8fd57b
**Repo:** spektacular
**Depends on:**
- 37a9fe91-7e9c-4ae0-b1f3-bb392fde38b1 — Run the probe and record the ten-minute outcome
- 00c54dac-455c-43ba-b358-a3190d71b018 — Add worktree paths and the no-wake-up rule to the epic skills
**Execution:** agent

Rewrite the headless paragraph and the fresh-child-in-foreground rule in both epic skills, in their template source, to match the recorded outcome.
- **If the child survived:** headless means only that no one answers between turns, and children run in the background with completion notices.
- **If it was stopped:** the foreground workaround stays.

Either way, the skills state the outcome, and being told no one answers no longer forces foreground children on its own.

*Technical detail:* [context.md#task-rewrite-the-epic-headless-guidance-to-the-probe-outcome](./context.md#task-rewrite-the-epic-headless-guidance-to-the-probe-outcome)

**Acceptance criteria**:
- [ ] Both epic skills state the probe's outcome and keep children in the foreground only if the probe's child was stopped.
- [ ] Neither skill treats being told no one answers as, by itself, a reason to hold children in the foreground.
- [ ] The skill tests pin the new wording, retire the old phrases and pass.

#### - [ ] Task: Check that the epic suites run children as a real session does
**Id:** cc98f1eb-1d8e-4bfe-91ec-b795d769dd54
**Repo:** spektacular
**Depends on:**
- 5d07bd37-e6cf-45fd-b9f6-cf8bab8fd57b — Rewrite the epic headless guidance to the probe outcome
- 8483e836-8b57-490b-b33f-91d1fa3e4700 — Enforce delegation and wake-up rules in the epic suites
**Execution:** agent

Add the background-children rule to both epic suites' Claude checks. Per-spec children must be launched in the background, a turn must end while they run, and a new turn must start on a child's completion notice. If the probe found children stopped, check the foreground workaround instead. The suites keep telling the agent the session is headless and no one answers.

*Technical detail:* [context.md#task-check-that-the-epic-suites-run-children-as-a-real-session-does](./context.md#task-check-that-the-epic-suites-run-children-as-a-real-session-does)

**Acceptance criteria**:
- [ ] A Claude epic run whose per-spec children ran in the foreground fails, and one with background children and a notice-driven turn passes, or the reverse if the probe found children stopped.
- [ ] Both epic suites' tasks still say the session is headless and no one answers between turns.
- [ ] Codex and Bob epic runs are not judged by this rule.

## Open Questions

- **Does a background child survive past ten minutes in a headless Claude Code session?** This can only be settled by running the probe. The plan already branches on it: the outcome decides the epic skills' wording and which rule the epic suites check. If the probe cannot be run, or its outcome is neither a completed child past 600 s nor a stopped one (for example, the run dies on an auth error or the orchestrator never starts a background child), STOP and ask the user before changing the epic guidance.
- **Do the strengthened instructions make the current default model delegate the test and verify steps?** Only a live Claude batch can show this. If a post-change batch still shows `test` or `verify` without a child, keep the checks (they are doing their job) and STOP to ask the user whether to strengthen the wording further. Never relax the rule to make the batch pass.

## Out of Scope

- **A long-lived sign-in token for harbor runs** is not added, so a batch can still fail when the short-lived token rotates mid-run (spec non-goal; `claude setup-token` is the known remedy).
- **The public website's documentation of the epic workflows** is not updated, even though their guidance on background children changes (spec non-goal; the `docs` repo is untouched).
- **Blocking the agent's wake-up and recurring-job tools inside the test containers** is not done. The epic suites detect and fail their use instead (spec non-goal).
- **Newer-model behaviour differences that do not concern child agents** are left alone. For example, the goal-check agent fixed the README before its reviewer saw it, and the guided repo setup recorded values without proposing them (spec non-goal).
- **Changes to harbor or to the coding agents' own software** are not made (spec non-goal).
- **Delegation enforcement for Codex and Bob** is not added. Their records carry child detail, or unknowns, but their runs are never failed by the delegation rules (spec constraint).
- **The implement suite gains no delegation checks.** Its seeded state starts past the test, verify and goal-check steps, so they never run there. Those steps are checked in the goal-check and implement-epic suites instead.
- **Making the plan, epic and implement-epic verifiers fully pure-rule with self-checks** is not attempted. Only the rules this plan adds or changes are pure functions with self-checks.
- **The other spawn-style step instructions** (plan discovery, implement analysis, changelog formatting, the spec split review) keep their current wording. They are suggested delegations, recorded but not enforced, except the split review, which no suite exercises.

## Changelog

### 2026-10-24 — Task: Read every child agent from each agent log

**What was done**: Added `Child`, `Turn` and `Wakeup` records and four pure readers to the shared transcript module. `claude_children` joins each `Agent`/`Task` call, at any depth, with its `task_started`, `task_updated` and `task_notification` events and its own streamed messages. `claude_turns` reads each turn's end and what woke it. `claude_wakeups` reads the main agent's scheduled wake-ups, cron jobs and sleeping shell commands. `atif_children` gives Bob children with kind, description and prompt, and Codex an unknown list. The existing launch readers are untouched.

**Deviations**:
- The wake-up reader counts any main-agent shell command that runs `sleep`, foreground or background, not only background ones. The user decided this on 2026-10-24, because the s55 implement-epic `sleep 240` poll the plan cites ran in the foreground.
- `command_lifecycle` is a top-level `type` in Claude's log, not a `system` subtype, so the turn reader matches it that way.
- Two fallbacks the plan did not spell out:
  - When a log has no `task_started` events at all, every launch the agent did not refuse counts as a child, with background state unknown.
  - A child with no notification but a foreground launch result takes its ending and first hand-back line from that result, with the hand-back frame and indent stripped.

**Files changed**:
- `tests/harbor/shared/transcripts.py`
- `tests/harbor/shared/tests/test_transcripts_summary.py`

**Discoveries**:
- In Claude's stream-json, no `system` or `result` object carries a `timestamp`. Only assistant and user objects do.
- `task_updated` holds only `task_id` and `patch`, so it joins by task id; `task_notification.summary` is the bare, unframed hand-back.
- Children started by children (spawn depth 2) stream no messages, so their models and output are per-child unknowns.
- A launch with no `run_in_background` key can still run in the background. `task_started.is_backgrounded` is the only reliable source.
- The 2026-10-23 reference runs contain no failed or killed child, so the failure paths are proven only on synthetic input.

### 2026-10-24 — Task: Record children step counts and idle time

**What was done**: The run metrics record is now schema version 2. It carries:
- `subagents.children`: every child agent, placed in its spec and workflow step;
- `children`: a count on every step entry;
- `totals.idle_s`: the longest gap in the run's activity.

Placement is a shared function, so the verifiers can place children exactly as the record does. Each new section has its own never-fail guard.

**Deviations**:
- `place_children` returns the unknowns it finds as well as the placed children, so its gaps reach the record.
- Turns and wake-ups are not stored on `AgentLog`. Nothing in the record uses them, so the verifiers will call the transcript readers directly.
- `Child` gained an `order` field (not in the record), so Bob's children can be placed by message order in a one-lane run.
- Bob runs read idle time from Bob's own shell logs. A run whose agent log carries no times at all records idle as unknown, because the session log alone would count model time as idle.

**Files changed**:
- `tests/harbor/shared/metrics.py`
- `tests/harbor/shared/transcripts.py`
- `tests/harbor/shared/tests/test_metrics.py`
- `tests/harbor/shared/tests/test_transcripts_summary.py`

**Discoveries**:
- On the reference runs, s55 epic planning records 942 s idle of 1249 s and m70 records 88 s of 993 s, both as the plan expected. Goal reviewers and the test and verify children land in `check_goal`, `test` and `verify`. Every epic orchestrator's children land on their spec with no step.
- A child with no launch time in a multi-lane run must read as step unknown, not "outside any step". The test task found this, and it is fixed and pinned.
- `test_readme_metrics.py` still pins version-1 keys and passes only because it reads README text. The README task must update it.

### 2026-10-24 — Task: Check in the trimmed 2026-10-23 reference runs

**What was done**:
- Added `trim_reference.py`, which turns a collected harbor trial into a compact reference run. It blanks message text to the same number of `x`, keeps only each child's first hand-back line, and cuts session-log responses to their error code and step.
- The script recomputes the metrics record on both copies and writes nothing if any value differs.
- Checked in the four reference runs: m70 and s55 epic planning, s55 plan, and s55 implement.
- Self-checks prove the child-record and idle acceptance criteria on them offline.

**Deviations**:
- Sizes:
  - The fixtures are about 2.3 MB on disk, because the blanks keep every character count the recorder reads.
  - Compressed, they total about 150 KB, against 11.7 MB for the raw trials.
- The recomputation compares the whole record (minus run identity and check score), not just the named values. The walkthrough and changelog session-log rules join it when Milestone 2 adds them.
- Each child's hand-back keeps only its first line, and `task_progress` notes are blanked. Wake-up inputs are kept in full, as planned.
- The plan's s55 child durations (109.6/107.6/137.0 s) were wrong. The notifications report 109.605/111.142/145.246 s, and those are pinned.

**Files changed**:
- `tests/harbor/shared/tests/trim_reference.py`
- `tests/harbor/shared/tests/test_reference_runs.py`
- `tests/harbor/shared/tests/data/m70-epic/` (README, build.json, claude-code.txt, debug/)
- `tests/harbor/shared/tests/data/s55-epic/`
- `tests/harbor/shared/tests/data/s55-plan/`
- `tests/harbor/shared/tests/data/s55-implement/`

**Discoveries**:
- The session log's resume detection reads the response's `code` (`workflow_in_progress`), so a trimmed response must keep it.
- When blanking a hand-back, newlines must stay newlines, or its first line runs on into the blank. The script's own recomputation caught this.
- Regenerating from the raw trials gives byte-identical fixtures, and so does re-trimming the fixtures.

### 2026-10-24 — Task: Show children and contexts in the report

**What was done**:
- The run table gains an `idle m` column, and a `◀ idle` mark on any run idle for more than `IDLE_SHARE` (a quarter) of its wall time.
- One run's view gains an `Idle:` line and a children table. The table has one row per child, nested ones indented with `└`, showing kind, spec, step, bg/fg, seconds, ending and context size.
- `--run <trial> --prompts` prints each child's full context. `--prompts` without `--run` is a usage error with exit status 2.

**Deviations**:
- None in scope.
- The children section has wording for each case:
  - an empty list prints `Children: none`;
  - an unknown list prints `Children: unknown (see below)`, and `--prompts` says the contexts are unknown rather than "none recorded";
  - a version-1 record prints `Children: not recorded (schema 1)`.

**Files changed**:
- `tests/harbor/shared/report.py`
- `tests/harbor/shared/tests/test_report.py`

**Discoveries**:
- On the reference records, s55 epic planning shows `Idle: 942.2 s of 1249.4 s (75%) ◀ idle` and m70 shows 9% with no mark, as the spec's acceptance criterion expects.

### 2026-10-24 — Task: Document the new record fields and report option

**What was done**: The README's run-metrics reference now covers schema version 2:
- every new field in `totals`, `steps` and `subagents.children`, with what it means and the `ending` values;
- how children are placed, and how idle time is measured and flagged (`IDLE_SHARE`);
- which child fields Bob, Codex and the oracle leave unknown;
- the report's idle column, children table and `--run … --prompts`;
- the checked-in reference runs and `trim_reference.py`.

`test_readme_metrics.py` pins all of it against hand-written version-2 keys.

**Deviations**: The probe target is not documented here. Its task, in Milestone 4, adds it.

**Files changed**:
- `README.md`
- `tests/harbor/shared/tests/test_readme_metrics.py`

**Discoveries**: None.

### 2026-10-24 — Task: Judge the plan walkthrough from the session log

**What was done**:
- Added `sessionlog.plan_steps_entered`: the step each successful `plan new` or `plan goto` left the workflow at, read from the CLI's own session log.
- The plan suite's walkthrough-before-finished check now judges that sequence, so piped and chained advances count.
- The discovery spawn check (`TestSubAgentSpawning`, `EXPECTED_SPAWN_STEPS`) is gone. Discovery only suggests a child, and the metrics record's per-step `children` count now shows whether it used one.
- Deleted the stray `temp.py`.

**Deviations**:
- The trim script's self-verification now includes `plan_steps_entered`, as the plan's trim task asked; that rule did not exist when the trim landed.
- The spawn check was deleted, with the docstring pointing to the record, rather than turned into a record assertion.

**Files changed**:
- `tests/harbor/shared/sessionlog.py`
- `tests/harbor/plan-workflow/tests/test_plan_workflow.py`
- `tests/harbor/plan-workflow/tests/temp.py` (deleted)
- `tests/harbor/shared/tests/trim_reference.py`
- `tests/harbor/shared/tests/test_sessionlog_rules.py`

**Discoveries**:
- `plan file` commands also log a `state_after` with the current step, so a step sequence must be built from `new` and `goto` events only.
- The s55 plan run's discovery did start one child (an Explore). Its old verifier failure came only from the walkthrough check misreading piped output.
- The plan verifier imports cleanly outside a container, but its session-scoped preflight fixture calls `pytest.exit` when no transcript exists. Running its tests on reference data needs that preflight stubbed.

### 2026-10-24 — Task: Judge per-repo changelog writes from the session log

**What was done**:
- Added `sessionlog.changelog_writes_by_repo`, which counts each successful `changelog file write <name>` per `--repo` value (`None` for the project record). It reads the argv the CLI received, so loops, shell variables, reordered flags and line continuations all count. Help requests and other records' writes are left out.
- The implement suite's project and per-repo changelog checks now judge that count instead of matching command text.
- The trim script's self-verification includes the rule too.

**Deviations**: The rule takes an optional record name, so a verifier counts only its own plan's writes.

**Files changed**:
- `tests/harbor/shared/sessionlog.py`
- `tests/harbor/implement-workflow/tests/test_implement_workflow.py`
- `tests/harbor/shared/tests/trim_reference.py`
- `tests/harbor/shared/tests/test_sessionlog_rules.py`

**Discoveries**:
- The s55 implement transcript wrote its records with shell variables in a loop (`changelog file write $P $a --from $f`). No command text contains the old substring, but the session log holds the resolved argv with exit 0.
- `changelog file write <name> --help` exits 0. The test task caught that it would count as a write, and it is now excluded.

### 2026-10-24 — Task: Tell the plan and implement agents that no one answers

**What was done**:
- The plan and implement suites' tasks gained the `## You are running non-interactively` section every other suite carries: no human is at the other end, nothing answers between turns, and the agent must never end its turn waiting for a reply.
- The plan task's success criteria now name `document_status: draft`/`final`, as its verifier checks, and say the discovery child is recorded, not required.
- `test_suite_wiring.py` pins the section's shared phrases in every suite with an `instruction.md`, so a new suite cannot leave it out.

**Deviations**: The wiring check pins the phrases every suite already shares: the heading, "no human at the other end" and "nothing will answer". It does not pin "between your turns", which only the epic, plan and implement suites use.

**Files changed**:
- `tests/harbor/plan-workflow/instruction.md`
- `tests/harbor/implement-workflow/instruction.md`
- `tests/harbor/shared/tests/test_suite_wiring.py`

**Discoveries**: The standing-rules suite's task has no "never end your turn waiting for a reply" line. It tells the agent to write the question into its answer and stop for that request, which fits a suite that checks the agent offers rather than decides.
