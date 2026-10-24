---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
epic: 000002_workflow-tracing-and-run-stats
sources:
    - uri: docs/workflow-performance-investigation.md
      retrieved_date: "2026-10-24"
---

# Feature: 000076_harbor-on-built-in-logs

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Spektacular's end-to-end test harness, Harbor, measures each test run's step timings, helper agents and idle time by parsing every agent's own transcript format and Spektacular's old debug output: a large body of parsing code that differs for each agent and cannot see the helper agents of some agents at all. Harbor will take those measures from the timing records Spektacular itself now keeps, so every agent is measured the same way, and keep reading transcripts only for what Spektacular cannot see: tokens, models and cost. Maintainers get comparable numbers across agents and a much smaller harness to keep working.


<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [ ] **Step timings come from Spektacular's logs**
  Each Harbor run's metrics record must take its workflow steps and their timings from Spektacular's own logs, for every agent the harness runs.
- [ ] **Helper agents come from Spektacular's logs**
  Each run's metrics record must take the number of helper agents started, and the spec and step each was started in, from Spektacular's own logs, for every agent, including agents whose transcript shows no helper agents.
- [ ] **Idle and user-wait time are reported in full**
  Each run's metrics record must report total time waiting on the user, total gap time and each step's split into working, helper-wait and user-wait time, in place of the single longest gap reported today.
- [ ] **Epic run timelines come from Spektacular's logs**
  The harness's report on where an orchestrated epic run spent its time (when each spec's child started and finished, its slowest step, and the hand-off gaps between child and orchestrator) must be built from Spektacular's own logs.
- [ ] **Delegation checks judge from spans**
  The harness's checks that a step's required helper agent was started, and in which spec and step, must judge from Spektacular's own logs. Checks on what a helper's brief contained may keep reading the transcript.
- [ ] **Tokens, models and cost still come from transcripts**
  The metrics record must keep reporting token usage, models used and cost, unchanged.
- [ ] **The old parsers are removed**
  The harness's debug-log reader and its transcript-based step, helper-placement and idle measures must be removed once replaced, leaving one source for each measure.
- [ ] **Trial artifacts carry the logs**
  Each Harbor trial must collect the project's Spektacular logs into its artifacts in place of the old debug output, so a run can be reported on after the fact.
- [ ] **Missing data stays explicit**
  When a run's logs are missing or a measure cannot be read, the metrics record must report it as unknown with the reason, never as zero, as today.
- [ ] **Measuring never fails a run**
  Gathering metrics must never fail a test run or change its reward, as today.
- [ ] **Cross-run reports use the new measures**
  The harness's cross-run report must show the new per-step split and totals for each run, and the difference in each per-step figure and total between runs.


<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- Harbor must take every measure Spektacular's logs can supply (step timings, helper agents and where they started, idle, gap and user-wait time, epic run timelines and delegation placement) from them, replacing its own parsing (user: "We should look at how we can unify the work harbor is doing and replace as much of it as we can with the built in logger").
- Token usage, models and cost must keep coming from agent transcripts, since Spektacular cannot see them.
- The harness must get every measure it needs from logs recorded at Spektacular's default log level, with no special setting.
- The harness must read Spektacular's logs through the run statistics report defined by 000075_run-stats-command, and relies on the step, sub-agent and user-wait spans defined by 000074_trace-spans-for-agent-work; this spec depends on both.


<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [ ] **Every agent gets step timings**
  A Harbor implement run under each of Claude, Codex and Bob produces a metrics record whose steps and step timings match the run statistics report for the same trial's logs.
- [ ] **Codex helper agents are counted**
  A Harbor implement run under Codex that starts helper agents produces a metrics record with a non-zero helper count, each helper placed in its spec and step.
- [ ] **The per-step split is reported**
  A Harbor spec run's metrics record reports total user-wait time, total gap time and, for each step, working, helper-wait and user-wait time.
- [ ] **Epic timelines read the logs**
  The epic timeline report for a Harbor implement-epic trial lists each spec's child start and finish, its slowest step and both hand-off gaps, and is produced correctly for a trial that has no old debug directory.
- [ ] **Delegation checks pass from spans**
  A Harbor run in which a required helper agent was started passes the delegation checks, and a run in which it was not fails them naming the step, judged from sub-agent spans.
- [ ] **Token figures are unchanged**
  Running the old and new metrics gathering over the same collected trial artifacts gives identical token usage, models and cost.
- [ ] **Measures come from one source**
  With the old debug directory absent, a trial still produces full step, helper and idle measures; with Spektacular's logs removed, those measures are unknown and none is filled in from transcripts.
- [ ] **Artifacts hold the logs**
  A collected Harbor trial contains the project's Spektacular logs, and the run statistics report run against that copy gives the trial's step timings.
- [ ] **Missing logs are unknown**
  A trial whose Spektacular logs were removed produces a metrics record with step, helper and wait measures set to unknown with a reason, and still exits successfully.
- [ ] **Rewards are untouched**
  A trial's reward is identical whether metrics gathering succeeds or fails.
- [ ] **The cross-run report shows the split**
  The cross-run report over two trials shows each trial's per-step split and totals and their differences.
- [ ] **Harbor self-tests pass**
  The harness's local self-tests (`pytest tests/harbor`, which cover every requirement above that the harness can check without an agent) pass after the change.


<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- Replace `tests/harbor/shared/sessionlog.py` and `timeline.py`, and the transcript-based step, child-placement and idle parts of `metrics.py` and `transcripts.py`.
- `transcripts.py` keeps only what the transcript-sourced constraint and the delegation-brief checks need.
- Point `delegation.py`'s placement rules at sub-agent spans; prefer leaving its hand-kept oracle of which steps require a child unchanged.
- Collect `.spektacular/logs/` into trial artifacts where `.spektacular/debug/` is collected today, and drop the session-log environment override.
- `report.py` is where the per-step split and totals are surfaced.


<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- The harness's shared measurement scripts shrink by at least a third from about 3,000 lines.
- For the same scenario, Claude, Codex and Bob records report the same set of steps and a helper count for every helper-starting step, with no agent-specific fields.
- No Harbor suite's pass rate drops, comparing 5 runs per suite for Claude on the harness before and after the change.


<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- New Harbor suites, such as a single-spec implement suite with real tasks.
- Changing any suite's scenarios or its pass and fail checks, beyond where they read their data.
- A visual view of runs (covered by a separate spec, the HTML log viewer).

