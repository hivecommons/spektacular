---
created_date: "2026-10-23"
document_status: final
closed_date: "2026-10-23"
---

# Plan: 000070_harbor-run-metrics

<!-- Metadata -->
<!-- Created: 2026-10-23T10:24:37Z -->
<!-- Commit: 221979e -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Every automated harbor run of a coding agent through Spektacular will leave a standard metrics record beside its pass-or-fail reward. The record holds the run's time, turns, tokens and cost, a per-step breakdown of time and characters written, its sub-agents, and the share of checks that passed, in the same shape for Claude, Codex and Bob, with anything an agent does not report marked unknown. A local report reads those records across collected jobs to list runs, show where one run spent its time, and compare a baseline set of runs with a candidate set, flagging dominant steps and regressions. The people improving Spektacular can then find what makes a workflow slow, such as the plan step that retypes text it already wrote, and measure whether a fix helped, without hand-written scripts.

## Conventions

- **Passing tests are required before calling work done** — every task finishes with the Go test suite passing. The harbor jobs directory can hold root-owned files that make the Go test suite fail with permission denied, which must be resolved (or the user consulted), not dismissed. The new Python self-checks must pass too.
- **Tests must not depend on execution order** — the recorder's and report's self-checks use fixed synthetic timestamps and fixtures, never the wall clock or state left by another test, so they pass under any order and on their own.
- **A plan never changes the active skills and configuration** — the work stays in `tests/harbor/`, the `Makefile` and `README.md`. No task migrates or re-inits this repo's `.spektacular/` or regenerates installed skills. Suites build their own binary into `.build/`.

## Architecture & Design Decisions

Everything lands in the `spektacular` repo, under `tests/harbor/`, the `Makefile` and `README.md`. The product CLI is not touched. The work has two parts: a **recorder** that runs inside each trial's container and writes `/logs/verifier/metrics.json`, and a **report** that runs on the developer's machine over `tests/harbor/jobs/`. Both live in `tests/harbor/shared/` beside the existing `transcripts.py` and `sessionlog.py`, and reuse them. The Makefile already copies `shared/*.py` into a suite's tests, so every suite picks the recorder up the same way, as the spec's Technical Approach asks.

**Recorder.** Each suite's `test.sh` changes in one way. pytest also writes JUnit XML to `/logs/verifier/junit.xml`, its exit code still alone decides `reward.txt`, and only after the reward is written does the recorder run, with its exit status ignored. The recorder cannot change the reward, which meets the constraint that a run's pass-or-fail result keeps its meaning. A crash in the recorder leaves no record, never a failed verifier. Internally it is a set of pure functions over parsed inputs, so it can be self-checked locally on synthetic data, as the existing verifier self-checks are:
- **Identity**: `tests/build.json`, written by each Makefile render target with agent, suite, scenario, the git-described version, commit and dirty flag.
- **Totals and sub-agents**: one reader per agent log. Claude uses the stream-json `result` events. Codex and Bob use the ATIF `trajectory.json` harbor leaves, with Codex's rollout as fallback.
- **Step breakdown**: from Spektacular's session log through `sessionlog.SessionLog`.
- **Check score**: from the JUnit XML.

Anything a source does not supply is written as JSON `null` and named in an `unknown` list with the reason, never as zero.

**Step breakdown is agent-agnostic by construction.** A step's start and end come from the session log's `state_after.current_step` changes, one lane per workflow, so durations are the same whichever agent ran. Output volume is measured in **characters**: assistant text, thinking and tool-call input, attributed to the step whose window contains the message. Claude and Codex messages carry timestamps. Bob's trajectory has none, so Bob's messages are attributed by order, advancing the step at each `spektacular … goto` tool call in the trajectory. In an orchestrated (multi-lane) run, a Claude sub-agent's characters go to the lane whose spec its launch prompt names. Where an ATIF agent's output cannot be split between lanes, that lane's output is recorded as unknown.

**Report.** `tests/harbor/shared/report.py` (plain Python, stdlib only, with a Makefile wrapper) reads only `verifier/metrics.json` and the trial's `result.json`. Both are left readable on the host, so it never needs elevated permissions and never touches the root-owned `agent/sessions/` tree. It has three views:
- the default table, one row per run grouped by suite (and scenario);
- `--run <trial>`, the step breakdown for one run;
- `--baseline <dirs|globs> --candidate <dirs|globs>`, a per-suite comparison of medians.

Flags are computed in the report, not stored in the record, so thresholds can change without re-running suites:
- a step taking more than a third of its run's wall time;
- a candidate median at least twice the baseline's time, turns, tokens or cost;
- a check-score drop of 0.1 or more.

**Why this shape.** It keeps the record next to the reward, so it is collected with every trial, including ones started by hand. The report reads only what harbor leaves readable. Neither harbor's empty `agent_result` nor a Go-side change is involved. The rejected alternatives are a pytest plugin, host-side extraction, harbor's `result.json` totals, a Go subcommand, ldflags version stamping and re-owning job directories. They are covered with evidence in [research.md](research.md#alternatives-considered-and-rejected). Two harness gaps are closed along the way:
- The spec, plan, repo and implement suites gain the shared-helper copy and `python3` in their images.
- `harbor-test-implement` starts rendering into `tests/harbor/.build/` like every other suite, rather than running from its source directory.

**Conventions.** Done means the Go test suite passes (*passing tests are required before calling work done*). The new Python is self-checked by `pytest` locally, without wall-clock or ordering dependencies (*tests must not depend on order*). Nothing here touches the active Spektacular install or `.spektacular/` (*a plan never changes the active skills and configuration*). Harbor builds its own binary per suite.

## Component Breakdown

- **Run recorder (new, shared helper).** Owns building one run's metrics record and writing it beside the reward. It is a thin entry point over pure functions: identity, totals, step breakdown, sub-agents and check score. Each function takes parsed inputs and returns plain data, so the recorder self-check can exercise them on synthetic input. The recorder reads the build info, the agent log readers, the session-log reader and pytest's JUnit output. It never raises past its entry point: any failure becomes an `unknown` entry, or at worst no record, never a changed reward.
- **Agent log readers (extended shared helper, the existing transcript module).** Already read Claude's stream-json and the ACP trajectory for sub-agent launches. They gain the run summaries:
  - **Claude**: totals from the `result` events, under the single-versus-several rule. Per-message timestamps and characters. Sub-agent lifecycle (status and duration from task notifications).
  - **ATIF trajectory** (Bob, and Codex through harbor's conversion): step count, final token totals where present, and per-step characters with timestamps where present.
  - **Codex rollout**: a fallback reader for `token_count` totals and the model when no trajectory is readable.

  The existing functions keep their signatures, so the verifiers that already import them are unaffected.
- **Session-log reader (reused, the existing session-log module).** Already groups Spektacular's command log into lanes. It gains one function that turns a lane's events into ordered step windows (step name, start, end), taken from each change of the workflow's current step. That is the agent-independent source of step durations. The timeline report and the epic verifiers keep using it unchanged.
- **Check scorer (new, part of the recorder).** Reads pytest's JUnit XML and returns passed and total counts overall and per group. A group is the test class, falling back to the test module for function-style suites.
- **Build info (new, written by the Makefile).** A small JSON file each suite's render target drops into the suite's tests directory. It names the agent, suite, scenario, Spektacular version, commit and whether the tree was dirty. It is the recorder's source for run identity that the container cannot discover itself.
- **Suite verifier scripts (changed, all eight `test.sh`).** Each gains JUnit output on its existing pytest call and a final, never-failing recorder call made after the reward is written. Their pytest verifiers are not changed.
- **Suite render targets and images (changed, Makefile and four Dockerfiles).** Every suite's render target copies the shared helpers and writes the build info. The implement suite moves to a render target like the others. The spec, plan, repo and implement images gain `python3`.
- **Run report (new, shared helper, with a Makefile wrapper).** A local, stdlib-only command over collected jobs. It discovers runs by finding records, joins each with its trial's `result.json`, and renders three views: the per-suite run table, one run's step breakdown, and a baseline-versus-candidate comparison. It owns the flag rules (dominant step, regression) so thresholds live in one place. It reads only the record and `result.json`, never agent logs.
- **Self-checks (new, local pytest).** Synthetic-input tests for the recorder, the readers' new functions and the report. They live beside the shared helpers, outside what the Makefile copies into images, and run without a container, as the existing verifier self-checks do.
- **Harbor suite documentation (changed, README's end-to-end section).** Gains where the record is, what each field means and which values may be unknown for which agent, plus how to run the report's three views.

## Data Structures & Interfaces

**Run metrics record** (`metrics.json`, one per trial, beside `reward.txt`) is the contract between the recorder and the report, and the shape the documentation describes. Every agent and suite writes the same top-level keys. A value the sources cannot supply is `null`, and its dotted path is listed under `unknown` with a reason. A number is never zero to stand in for missing data.

```jsonc
{
  "schema_version": 1,
  "run": {                       // identity
    "suite": "plan-workflow", "scenario": null,
    "agent": "claude", "model": "claude-sonnet-4-6",     // null when unreported
    "spektacular_version": "v0.9.0-12-g221979e", "commit": "221979e…", "dirty": true
  },
  "result": {                    // checks; reward mirrors reward.txt, never computed here
    "reward": 1,
    "checks": { "passed": 34, "total": 34, "score": 1.0 },
    "groups": [ { "name": "TestStepOrder", "passed": 3, "total": 3, "score": 1.0 } ]
  },
  "totals": {
    "wall_time_s": 948.4, "model_time_s": 1137.3, "turns": 85, "cost_usd": 3.51,
    "input_tokens": 118, "output_tokens": 66293,
    "cache_read_tokens": 5582933, "cache_write_tokens": 156794
  },
  "steps": {
    "unit": "characters",
    "workflows": [
      { "kind": "plan", "name": "20260101000000-user-auth",
        "steps": [ { "step": "assemble", "started_at": "…Z", "duration_s": 368.6, "output": 74210 } ] }
    ]
  },
  "subagents": {
    "started": 4, "completed": 4, "failed": 0, "stopped": 0,
    "durations_s": [16.6, 34.5, 68.8, 70.0]
  },
  "unknown": [ { "field": "totals.cost_usd", "reason": "bob reports no cost" } ]
}
```

**Internal interfaces** (pure functions in the shared helpers; each returns plain data plus the unknowns it found):

```python
# transcripts.py — agent log readers (new functions beside the existing ones)
claude_summary(objs) -> Totals                 # result events: single vs several rule
claude_messages(objs) -> list[Message]         # timestamp, chars, parent_tool_use_id
claude_subagents(objs) -> Subagents            # task_notification status + duration
atif_summary(trajectory: dict) -> Totals       # Bob and Codex trajectories
atif_messages(trajectory: dict) -> list[Message]  # timestamp may be None (Bob)

# sessionlog.py
Lane.step_windows() -> list[StepWindow]        # (step, start, end) from current_step changes

# metrics.py — the recorder
build_record(build_info, agent_logs, session_log, junit_xml, reward) -> dict
step_breakdown(lanes, messages, launches) -> dict
score_checks(junit_xml_text) -> (Checks, list[Group])
main(argv) -> int                              # always 0; writes metrics.json, mode 0644

# report.py — the report
load_runs(paths_or_globs) -> list[Run]         # record + result.json + trial path
table(runs) / breakdown(run) / compare(baseline, candidate) -> str
flags: dominant_step(run) / regressions(baseline, candidate) -> list[Flag]
```

`Totals`, `Message`, `Subagents`, `StepWindow`, `Checks`, `Group`, `Run` and `Flag` are small dataclasses internal to the shared helpers. Only `metrics.json` and `build.json` cross a process boundary.

**Build info** (`build.json`, written by each Makefile render target into the suite's tests directory) is the identity contract between the Makefile and the recorder:

```json
{ "suite": "plan-workflow", "scenario": null, "agent": "claude",
  "spektacular_version": "v0.9.0-12-g221979e", "commit": "221979e…", "dirty": true }
```

**Report interface** (the user-facing contract, documented with its exact commands in the README):

- *positional job directories or globs* (default: the collected jobs directory) — the per-suite run table;
- *run* option naming a trial directory or trial-name prefix — that run's step breakdown;
- *baseline* and *candidate* options, each taking job directories or globs, given together — the per-suite comparison;
- a Makefile wrapper that passes its arguments through to the report.

## Implementation Detail

**A never-fail tail on every verifier.** Every suite's verifier script ends the same way. pytest runs as it does today, now also writing JUnit XML, and its exit code alone writes the reward. Then the recorder runs and its outcome is ignored. Four suites already run a report-only helper with its failure ignored (the epic timeline), so this extends an established pattern to all eight rather than inventing one. The difference is placement: the timeline runs before pytest so it shows at the top of the output, while the recorder must run after the reward so the reward is settled before it starts.

**Pure core, thin shell.** The recorder follows the shape the shared helpers already use: parsing and arithmetic are pure functions over parsed records, and only a small entry point touches `/logs`, `/app` and `/tests`. Every rule — the Claude single-versus-several summary rule, step windows, character attribution, unknown bookkeeping, check grouping — is therefore testable on hand-built input. The existing verifier self-checks use the same approach. Each gatherer returns its values together with the unknowns it found. The entry point concatenates them, so the `unknown` list is built where each gap is discovered, not reconstructed afterwards. The entry point also wraps each gatherer separately. One unreadable log costs that section, marked unknown, and the rest of the record is still written.

**One reader per log format, not per agent.** Agent differences live entirely in the readers, and there are three log formats: Claude's stream-json, the ATIF trajectory harbor writes for ACP agents and Codex, and Codex's native rollout as a fallback. The recorder chooses a reader from what is present in the agent log directory, confirmed by the build info's agent name, and everything downstream — step breakdown, record assembly, report — is format-agnostic. A future agent that harbor records as ATIF needs no new reader.

**Step breakdown is two independent passes joined by time.** The session log yields step windows per workflow lane, the same for every agent. The agent log yields a stream of messages with characters and, where the format has them, timestamps. Joining puts each message in the window that contains it. Messages without timestamps (Bob) are placed by order instead, using the `goto` tool calls in the same trajectory as boundaries. The join is where per-lane attribution happens for orchestrated runs. Durations never depend on the agent log, so a missing transcript leaves durations intact and only `output` unknown.

**Implement suite moves onto the render pattern.** It is the one suite harbor runs straight from its source directory, with the binary built into that directory. It gains a render target into `.build/` like the other seven. Copying the shared helpers and writing build info then never dirties the source tree. The existing make target name and behaviour stay the same for the person running it.

**The report is a reader of records only.** It never opens agent logs, so it is fast over many jobs and never trips on root-owned files. Its three views share one loader and one set of flag rules. The thresholds sit as named constants at the top of the module, the one place to tune them. Output is plain fixed-width text that reads in a terminal and pastes into an issue, in line with how the timeline prints today.

**Documentation follows the suites.** The README's end-to-end section is where the suites, make targets and job layout are described, so the record and report join it there. That means a `metrics.json` and `junit.xml` line in the job layout, a field reference that names which agents leave which fields unknown, and the report's three invocations.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Shared harbor helpers (transcript reader, session-log reader, timeline)** — the base the recorder builds on. The transcript and session-log modules gain new functions. Their existing functions keep their signatures, because the goal-check, epic and implement-epic verifiers and their self-checks import them. No change is needed before work starts.
- **Spektacular's session log (`SPEKTACULAR_SESSION_LOG=on`, already set in every suite image)** — the step boundaries. It is used as it is: the command records with `state_before`/`state_after` already carry the current step. The Go side needs no change.
- **Harbor 0.23.0 (installed by the Makefile's harbor install target)** — runs the trials. The plan relies on three behaviours observed in its source:
  - the agent log directory is mounted at `/logs/agent` during verification;
  - post-run trajectory conversion (Codex and ACP agents) completes before the verifier starts;
  - `/logs/verifier` comes back to the host readable.

  No change to harbor is needed. A future harbor that moves these would surface as unknowns in the record, not as failed runs.
- **Claude Code's stream-json output (the `result`, `assistant`, `user` and `system/task_*` events)** — Claude totals, per-message timestamps and sub-agent lifecycle. Used as-is; field names as in Claude Code 2.1.306.
- **ATIF trajectory format (harbor's `trajectory.json`, schema ATIF-v1.6)** — Bob's and Codex's step counts, token totals and messages. Used as-is.
- **pytest 8.4.1 (already in every suite image)** — its built-in JUnit XML output is the check-score source. No plugin is added.
- **`python3` in every suite image** — the spec, plan, repo and implement images do not have it yet and gain it from the distribution's package. The other four already install it.
- **Python standard library only** for the recorder and report (`json`, `xml.etree`, `statistics`, `argparse`, `glob`, `pathlib`, `dataclasses`). No third-party package is added on the host or in images.
- **`git` on the developer's machine** — the Makefile reads the version, commit and dirty state for the build info. It is already required to build.
- **Prior work this builds on (already landed):** spec 000031 (the agent session debug log) and the shared-helper extraction made for the goal-check suite (000068). Nothing must land first.

## Testing Approach

**Test types.** There are three layers, matching how the harbor layer is already tested:

1. **Local self-checks (unit, pytest, no container)** for the recorder, the new reader functions and the report. They feed hand-built input — stream-json lines, ATIF trajectories, session-log events, JUnit XML, records — through the pure functions and assert on the returned data or the rendered text. This is where nearly all coverage goes, because every rule in the feature is a pure function over parsed input. They follow the existing verifier self-checks: plain pytest, runnable without Docker, fixed synthetic timestamps, no reliance on the wall clock or on test order.
2. **Regression of the existing harness.** The existing verifier self-checks still pass, because the shared modules' existing functions are unchanged. The Go test suite still passes, because no Go code changes.
3. **End-to-end harbor runs.** These are the only proof that the record lands beside the reward in a real container for every suite and agent. They need Docker, credentials and ~15–25 minutes each, so they are manual, as every harbor run is today.

**Load-bearing assertions (self-checks).**
- *Reward is untouched.* The recorder's entry point returns success and writes no reward on every failure path we can construct: missing agent log, malformed JSON, missing JUnit, missing session log, missing build info. Each of those still yields a record with the affected fields `null` and listed in `unknown`, and the other fields filled.
- *Claude totals equal the summary.* For a single-result transcript, wall time, model time, turns, cost and the token counts equal the `result` event's values exactly. For a multi-result transcript:
  - turns are the sum of `num_turns`;
  - cost, tokens and model time come from the last event;
  - wall time is the transcript's timestamp span.
- *Sub-agents match.* Started, completed, failed and stopped equal `subagent_stats`, and each sub-agent's duration comes from its task notification.
- *Step breakdown.* For a plan lane, every step the session log entered appears in order with a duration. The durations sum to no more than the wall time. Characters land in the window that contains each message. Bob-style messages without timestamps are placed by their `goto` boundaries. Multi-lane Claude runs attribute sub-agent characters to the lane their launch prompt names.
- *Unknowns, never zeros.* A Bob trajectory gives `null` tokens and cost with wall time and turns present. A Codex trajectory gives `null` sub-agent fields with wall time and turns present. Every `null` has a matching `unknown` entry.
- *Check score.* For N tests with P passed, the overall score is P/N. Groups follow the class name, or the module for function-style suites. Skips are excluded.
- *Report.* The table has one row per record, grouped by suite (and scenario), and skips trials without a record. The breakdown lists every step with duration and output, and flags a step above a third of wall time; the fixture mirrors the reference plan run, where one step took 5.9 of 15.8 minutes. The comparison prints baseline, candidate and change per suite present in both sets for time, turns, tokens, cost and check score. It flags a measure whose candidate median is at least twice the baseline's, or a check-score drop of 0.1 or more, and does not flag just below either threshold. Unknown values print as unknown, not zero, and do not trigger flags.

**Success metrics.**
- *Finding where a workflow spends its time takes one report command; the plan workflow's slowest step is identifiable from the report alone.* **Behavioural test**: the report's single-run view, given a plan-run record, names every step with its duration and flags the dominant one. Confirming it on a real plan run is **Manual — captured in the implementation test plan**.
- *The next workflow speed-up is judged by a before-and-after comparison from the report.* **Behavioural test** for the comparison's content and flags on synthetic sets. Using it on two real sets of runs is **Manual — captured in the implementation test plan**.
- *Every Claude run's record is complete, with no unknown totals, turns, tokens, cost or step breakdown.* **Behavioural test**: a Claude fixture yields a record whose `unknown` list holds nothing under totals or steps. Confirming it across real Claude runs of every suite is **Manual — captured in the implementation test plan**.

**Manual reviews before acceptance.**
- **Manual — captured in the implementation test plan**: one run of every suite with each agent target it has today leaves a `metrics.json` with the same top-level fields, and its reward is unchanged from what the checks alone decide.
- **Manual — captured in the implementation test plan**: on a real Claude run, the record's totals and sub-agent counts equal the values in that run's `claude-code.txt` summary.
- **Manual — captured in the implementation test plan**: on real Bob and Codex runs, tokens and cost (Bob) and sub-agent measures (Codex) read unknown, while wall time and turns are present. This also confirms the open assumption about Codex's trajectory at verify time.
- **Manual — captured in the implementation test plan**: the report runs over `tests/harbor/jobs/` as the normal user, with no permission errors.
- **Manual — captured in the implementation test plan**: the README's harbor section explains where the record is, every field, and the three report invocations, and the commands it shows work.

**Deliberate gaps.** There is no automated container test. The harbor suites stay out of CI, as they are today, and the end-to-end checks above are the manual runs any harbor-touching change already needs. No Go tests are added, because no Go code changes.

## Milestones & Tasks

### Milestone 1: Every harbor run leaves a metrics record

**What changes**: Every run of every harbor suite writes `metrics.json` beside its reward, in the same shape whichever agent ran it. The record holds the run's wall time, model time, turns, cost and tokens. It also holds the agent, model, suite, version and commit, a per-step breakdown of time and characters written, the sub-agents' counts and durations, and the share of checks that passed, overall and per group. Measures an agent does not report, and measures a missing or broken log hides, read as unknown with a reason, never as zero. Pass-or-fail results do not change: the reward is written exactly as today, before the recorder runs, and a recorder failure never fails the verifier.

**Validation point**: The recorder's self-checks pass, including Claude totals matching the summary, unknowns for Bob and Codex, and a broken-log record. The existing verifier self-checks and the Go test suite still pass. Rendering any suite through its make target puts the shared helpers and build info in its tests directory.

#### - [ ] Task: Read run summaries from every agent log
**Id:** 92a98299-5d35-41a2-83b7-65a2837a337d
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Teach the shared transcript helper to read each agent's own summary of a run.
- **Claude**: totals from its end-of-run summaries, using the single-versus-several rule when background sub-agents split the run; every message with its time and the characters it wrote; each sub-agent's outcome and duration.
- **Harbor's trajectory format** (Bob, and Codex): step counts, token totals and messages.
- **Codex's native rollout**: a fallback.

Whatever a log does not supply comes back as unknown with a reason, so the recorder never has to guess.

*Technical detail:* [context.md#task-read-run-summaries-from-every-agent-log](./context.md#task-read-run-summaries-from-every-agent-log)

**Acceptance criteria**:

- [ ] For a Claude run with one summary, wall time, model time, turns, cost and input, output and cached tokens equal that summary's values.
- [ ] For a Claude run split by background sub-agents, turns are the sum across summaries, cost, tokens and model time come from the last summary, and wall time is the span from first to last message.
- [ ] Sub-agent counts for started, completed, failed and stopped equal the Claude summary's, and each sub-agent has a duration.
- [ ] A Bob trajectory yields turns and messages with tokens and cost unknown. A Codex trajectory yields turns and tokens with sub-agents unknown.
- [ ] The functions the existing verifiers already use behave exactly as before.

#### - [ ] Task: Turn the session log into step windows
**Id:** b1822747-0d20-4713-96a8-70a263ef994b
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Give the shared session-log reader a way to list, for each workflow lane, the steps the workflow entered in order, with when each began and ended. The windows come from Spektacular's own command log, so they are the same whichever agent ran. They become the backbone of the per-step breakdown.

*Technical detail:* [context.md#task-turn-the-session-log-into-step-windows](./context.md#task-turn-the-session-log-into-step-windows)

**Acceptance criteria**:

- [ ] A plan lane lists every step its command log shows it entered, in order, each with a start and end.
- [ ] A step re-entered later, or a lane resumed by a second child, appears as separate windows rather than one window spanning the gap.
- [ ] The epic timeline and the verifiers that read lanes behave exactly as before.

#### - [ ] Task: Build and write the run metrics record
**Id:** ad91e65b-cb09-4607-8c8c-882c84eaaade
**Repo:** spektacular
**Depends on:**
- 92a98299-5d35-41a2-83b7-65a2837a337d — Read run summaries from every agent log
- b1822747-0d20-4713-96a8-70a263ef994b — Turn the session log into step windows
**Execution:** agent

Add the recorder a suite's verifier runs once the reward is written. It reads the build info, the agent's logs, the session log and pytest's results, then writes the run's metrics record in the shared shape. That shape covers identity, totals, a per-step breakdown in characters, sub-agents, the check score overall and per group, and a list of unknowns with reasons. Every section is gathered separately and every failure is caught, so a missing or broken input costs only its own fields and the reward is never touched.

*Technical detail:* [context.md#task-build-and-write-the-run-metrics-record](./context.md#task-build-and-write-the-run-metrics-record)

**Acceptance criteria**:

- [ ] Records for Claude, Codex, Bob and oracle runs all have the same top-level fields.
- [ ] For N checks of which P passed, the overall score is P/N, each group's score is its own passed fraction, and groups follow test classes, or modules for suites written as plain functions.
- [ ] A plan run's step breakdown lists every step its command log entered, in order, each with a duration and a character count. The durations add up to no more than the wall time.
- [ ] A missing or unreadable agent log, session log, test result file or build info leaves a record with the affected fields marked unknown. The recorder still exits successfully and never writes the reward.
- [ ] A Claude run's record lists nothing unknown under totals or steps.

#### - [ ] Task: Wire the recorder into every harbor suite
**Id:** c05ca510-c9c4-48ed-965e-592e3b986446
**Repo:** spektacular
**Depends on:**
- ad91e65b-cb09-4607-8c8c-882c84eaaade — Build and write the run metrics record
**Execution:** agent

Make all eight suites write the record the same way. Every render target copies the shared helpers and writes the build info: agent, suite, scenario, version, commit and dirty state. The implement suite moves onto a render target like the rest. The four images without Python gain it. Every verifier script asks pytest for its results file and calls the recorder after the reward, ignoring any failure.

*Technical detail:* [context.md#task-wire-the-recorder-into-every-harbor-suite](./context.md#task-wire-the-recorder-into-every-harbor-suite)

**Acceptance criteria**:

- [ ] Rendering any suite through its make target leaves the shared helpers and a build info file in that suite's tests directory, and the source tree stays clean.
- [ ] Every suite's reward is still 1 exactly when every check passes and 0 otherwise. The recorder runs only after the reward is written and its failure is ignored.
- [ ] The implement suite's make target still runs the same suite with the same agent, from a rendered copy instead of the source directory.
- [ ] Every suite image can run the recorder.

### Milestone 2: One command shows where runs spend their time

**What changes**: A developer can run one local command, as their normal user, over the collected jobs and get a table of every recorded run grouped by suite: time, turns, tokens, cost, check score and result. Naming a single run shows its step breakdown, with the step that takes more than a third of the run flagged. Finding the slow part of a workflow no longer needs hand-written scripts.

**Validation point**: The report's self-checks pass for the table and the single-run view, including the dominant-step flag and unknowns printed as unknown. The command runs over a jobs directory holding root-owned agent files without a permission error.

#### - [ ] Task: Report runs and step breakdowns
**Id:** b19b3bd0-0f9d-4726-bdf2-46501bc3f734
**Repo:** spektacular
**Depends on:**
- ad91e65b-cb09-4607-8c8c-882c84eaaade — Build and write the run metrics record
**Execution:** agent

Add a local report command, with a make wrapper, that finds every recorded run under the jobs directory and prints one row per run grouped by suite, with totals, check score and result. Naming one run prints its step breakdown, with any step that takes more than a third of the run flagged. It reads only the record and harbor's result file, so it never needs elevated permissions.

*Technical detail:* [context.md#task-report-runs-and-step-breakdowns](./context.md#task-report-runs-and-step-breakdowns)

**Acceptance criteria**:

- [ ] Over a jobs directory, the report prints one row per run that has a record, grouped by suite, with time, turns, tokens, cost, check score and result. Runs without a record are skipped.
- [ ] For one named run, it prints every step the run reached with its duration and characters written.
- [ ] A step taking more than a third of its run's wall time is flagged, and one just under a third is not.
- [ ] Unknown values print as unknown, never as zero.
- [ ] It runs as a normal user over jobs that contain root-owned agent files.

### Milestone 3: Changes are judged by a before-and-after comparison, and the metrics are documented

**What changes**: A developer can name a baseline set of runs and a candidate set, as job directories or patterns. For each suite present in both, the report shows how time, turns, tokens, cost and check score moved, and flags any measure that at least doubled or any check score that fell by a tenth or more. The README's harbor section explains where a run's record is, what each field means and when it is unknown, and how to run the report's three views.

**Validation point**: The comparison's self-checks pass at and just below both thresholds. The README section's commands run as written against collected jobs.

#### - [ ] Task: Compare baseline and candidate runs
**Id:** cd421be0-6ee1-44a8-8a3f-4beafd94bb87
**Repo:** spektacular
**Depends on:**
- b19b3bd0-0f9d-4726-bdf2-46501bc3f734 — Report runs and step breakdowns
**Execution:** agent

Add the report's comparison view. Given a baseline set and a candidate set of job directories or patterns, it shows, for each suite present in both, the baseline value, candidate value and change for time, turns, tokens, cost and check score, using the median of each set. A measure whose candidate median at least doubles, or a check score that drops by a tenth or more, is flagged.

*Technical detail:* [context.md#task-compare-baseline-and-candidate-runs](./context.md#task-compare-baseline-and-candidate-runs)

**Acceptance criteria**:

- [ ] For each suite present in both sets, the comparison prints baseline, candidate and change for time, turns, tokens, cost and check score. Suites present in only one set are named as unmatched.
- [ ] A measure whose candidate value is at least twice the baseline's is flagged, and one just under twice is not.
- [ ] A check score that falls by a tenth or more is flagged, and a smaller drop is not.
- [ ] A measure unknown in either set is shown as unknown and is never flagged.

#### - [ ] Task: Document run metrics and the report with the harbor suites
**Id:** 899c287f-5f93-4a0b-83be-43ccd388c0ce
**Repo:** spektacular
**Depends on:**
- c05ca510-c9c4-48ed-965e-592e3b986446 — Wire the recorder into every harbor suite
- cd421be0-6ee1-44a8-8a3f-4beafd94bb87 — Compare baseline and candidate runs
**Execution:** agent

Extend the README's end-to-end section, where the suites are already described, with:
- where each run's metrics record and test results file land;
- what every field in the record means and which agents leave which fields unknown;
- how to run the report's table, single-run view and comparison, including the make wrapper and how to run the self-checks.

*Technical detail:* [context.md#task-document-run-metrics-and-the-report-with-the-harbor-suites](./context.md#task-document-run-metrics-and-the-report-with-the-harbor-suites)

**Acceptance criteria**:

- [ ] The README's harbor section shows where a run's metrics record is, explains every field in it, and says which fields are unknown for Bob and for Codex.
- [ ] It shows how to run the report, its single-run view and its comparison, and the commands work as written.
- [ ] The job layout shown in the README includes the new record and test results file.

## Open Questions

- **Is Codex's harbor-written trajectory readable when the verifier runs, and does it carry token totals?** Harbor's source writes it during post-run, before the verifier, and Bob's trajectory is already read at verify time by the plan suite. But no Codex job exists on disk to confirm the file is there, or that its `final_metrics` hold `total_prompt_tokens`/`total_completion_tokens`/`total_cached_tokens`. It depends on harbor's Codex conversion in the installed harbor version, and is only discoverable by running a Codex suite. **When the implementer hits it:** the recorder already falls back to the Codex rollout, and the self-checks cover both paths, so implementation is not blocked. The first real Codex run (a manual test-plan item) settles it. If that run's record has unknown wall time or turns, STOP and ask the user before changing the Codex reader, because the spec requires both to be present.

No other uncertainties remain that only implementation can resolve.

## Out of Scope

- **Sending runs to Braintrust or any hosted evaluation store.** Records stay in the local jobs directory. This is a later step of issue #90 (the evaluation roadmap).
- **Repeated runs, pass rates and pass^k.** The report compares sets of single runs and does not schedule or aggregate repeats. This is a later step of #90.
- **Spektacular-specific measures**, such as questions asked of the user, sub-agents cut off and restarted, and goal-check verdict correctness. This is a later step of #90.
- **LLM-judge scoring** of spec and plan quality.
- **Making any workflow faster**, including the deterministic plan assemble step the motivating analysis pointed to. That is separate work, which this plan's comparison view is meant to judge.
- **Documenting the metrics on spektacular.dev.** Documentation stays in this repository's README with the harbor suites. The `docs` repo is untouched.
- **New agent targets for suites that lack them.** The repo and implement suites stay Claude-only and the spec suite has no Bob target. Every suite records metrics for whichever agent runs it, and verification uses the targets that exist (user decision during discovery).
- **Recording metrics for runs started without a make target.** A raw harbor run of a suite's source directory has no shared helpers in its tests directory, so it leaves no record. Its reward is unaffected.
- **Changing harbor itself, or its empty `agent_result` totals.** The record is built from the agent logs, not from harbor's summary.
- **Exact Claude-style totals for Bob and Codex.** Their records carry unknowns until those agents report more, as the spec's known risk anticipates.
- **Re-owning collected job directories** or otherwise fixing root-owned agent files. The report avoids them instead.
