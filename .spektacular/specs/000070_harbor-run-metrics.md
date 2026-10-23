---
created_date: "2026-10-23"
document_status: final
closed_date: "2026-10-23"
sources:
    - uri: https://github.com/hivecommons/spektacular/issues/90
      retrieved_date: "2026-10-23"
---

# Feature: 000070_harbor-run-metrics

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Every automated test run of a coding agent through Spektacular records a standard set of measurements: how long it took, how many turns, tokens and dollars it used, how its time and writing divided across the workflow's steps, what its sub-agents did, and what share of the run's checks passed. A local report reads those records across past runs, flags the ones that look wrong, and compares a set of runs before a change with a set after it. Today a run reports only pass or fail, with its cost and token use left blank, so the people improving Spektacular cannot tell which workflow is slow, why, or whether a change made it faster, cheaper or better; this lets them find where the time goes, such as the part of planning that spends a third of its time retyping text it has already written, and measure whether a fix helped.

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

- [ ] **Every run records its metrics in a standard record**
  Each automated test run produces one metrics record that is collected with its pass-or-fail result, in the same shape for every agent and every test suite.
- [ ] **The record holds the run's totals**
  The record holds the run's wall time, time spent waiting on the model, turns, cost, and input, output and cached tokens, together with the agent, the model, the suite, and the Spektacular version and commit the run was made from.
- [ ] **The record breaks the run down by workflow step**
  For each Spektacular workflow step the run passed through, the record holds how long the agent spent in it and how much text the agent produced while there, in a unit the record states.
- [ ] **The record describes the run's sub-agents**
  The record holds how many sub-agents the run started, how many completed, failed or were stopped, and how long each one ran.
- [ ] **The record scores the run's checks**
  The record holds the fraction of the run's checks that passed, overall and for each group of related checks.
- [ ] **Unknown values are recorded as unknown**
  A measure an agent's logs do not supply, such as Bob's tokens and cost or Codex's sub-agents, or that cannot be read because a run's logs are missing or unreadable, is recorded as unknown, never as zero; the run still produces its record and its pass-or-fail result.
- [ ] **A local report lists runs across collected jobs**
  Users can run one command, without elevated permissions, that reads every collected run and prints one row per run with its totals, check score and result, grouped by suite.
- [ ] **The report shows one run's step breakdown**
  Users can ask the report for a single run and see each step's duration and output volume, so the slowest step stands out.
- [ ] **The report compares two sets of runs**
  Users can name a baseline set of runs and a candidate set, each as a list or pattern of job directories, and the report shows, per suite, how time, turns, tokens, cost and check score changed between them.
- [ ] **The report flags likely problems**
  The report highlights outliers, such as a step that takes a large share of its run, or a measure notably worse in the candidate set than in the baseline, so a problem stands out without reading every number.
- [ ] **The metrics are documented for the people running the suites**
  The people running the harbor suites can find where a run's metrics record is, what each of its fields means, and how to use the report.

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

- A run's pass-or-fail result must keep its current meaning, 1 only when every check passes; recording metrics must never change it.
- Must cover every agent the harbor suites run today: Claude, Codex and Bob.

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

- [ ] **Every suite leaves a metrics record**
  After one run of each harbor suite with each of Claude, Codex and Bob, every run's collected verifier output contains a metrics record, and every record has the same top-level fields.
- [ ] **Claude totals match the agent's own log**
  For a Claude run, the record's wall time, model time, turns, cost and input, output and cached token counts equal the values in that run's agent log summary.
- [ ] **Run identity is recorded**
  Every record names the run's agent, model, suite, Spektacular version and commit; a model the agent did not report is recorded as unknown.
- [ ] **Step breakdown covers every step the run reached**
  For a plan workflow run, the record lists every workflow step the run's own command log shows it entered, in order, each with a duration and an output volume, and the step durations add up to no more than the run's wall time.
- [ ] **Sub-agent counts match the agent's own log**
  For a Claude run that started sub-agents, the record's started, completed, failed and stopped counts equal the agent log summary's, and each sub-agent has a duration.
- [ ] **Check score reflects the verifier's checks**
  For a run whose verifier ran N checks of which P passed, the record's overall score is P/N, each group's score is its own passed fraction, and the run's pass-or-fail result is still 1 only when every check passed.
- [ ] **Bob runs record unknowns, not zeros**
  For a Bob run, the record's tokens and cost are marked unknown, and its wall time and turns are present.
- [ ] **Codex runs record unknowns, not zeros**
  For a Codex run, the record's sub-agent measures are marked unknown, and its wall time and turns are present.
- [ ] **A run with unreadable logs still scores normally**
  When a run's agent log is missing or unreadable, the run's pass-or-fail result is unchanged, the verifier does not fail because of it, and the record marks the affected measures unknown.
- [ ] **The report lists every collected run**
  Running the report over the collected jobs directory prints one row per run that has a metrics record, grouped by suite, each row showing totals, check score and result, without needing elevated permissions.
- [ ] **The report shows a run's step breakdown**
  Running the report for one named run prints each step it reached with its duration and output volume.
- [ ] **The report compares baseline with candidate**
  Running the report with two sets of job directories prints, for each suite present in both, the baseline value, candidate value and change for time, turns, tokens, cost and check score.
- [ ] **A dominant step is flagged**
  For a run in which one step takes more than a third of the run's wall time, such as a plan run where one step took 5.9 of 15.8 minutes, the report's step breakdown flags that step.
- [ ] **A regression is flagged**
  When a suite's candidate runs take at least twice the baseline's time, turns, tokens or cost, or their check score falls by a tenth or more, the comparison flags that measure for that suite.
- [ ] **Usage is documented with the harbor suites**
  The harbor suites' documentation in this repository explains where a run's metrics record is, what each field means, and how to run the report, its single-run view and its comparison.

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

- Build on the harbor helpers the suites already share for reading the agent's transcript and Spektacular's command log, rather than starting a separate toolchain, so every suite picks the record up the same way.
- Have each suite's verifier write the record as a JSON file beside its reward, so it is collected with the rest of the run's verifier output.
- Take totals from the agent's own end-of-run summary where it has one (Claude's), and find step boundaries from Spektacular's own command log, where each step change is a timestamped command; this keeps the step breakdown the same for every agent.
- Treat a group of checks as a test class where the suite uses them, falling back to the test module for suites written as plain test functions.
- Make the report work without elevated permissions, either by fixing the ownership of collected job directories in the run target, or by having the report read only files a run leaves readable; the planner may choose.
- Known risk: Bob and Codex expose far less than Claude, so their records will be mostly unknowns until those agents report more.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Finding where a workflow spends its time takes one report command, with no hand-written scripts: the plan workflow's slowest step is identifiable from the report alone.
- The next workflow speed-up, such as replacing the planning step that retypes already-written sections with one that assembles them automatically, is judged by a before-and-after comparison from the report rather than by impression.
- Every Claude run's record is complete, with no unknown totals, turns, tokens, cost or step breakdown.

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

- Sending runs to Braintrust or any other hosted evaluation store; results stay on the local machine (a later step of issue #90, the evaluation roadmap).
- Running a suite several times and reporting pass rates or pass^k (a later step of #90).
- Spektacular-specific measures such as questions asked of the user, sub-agents cut off and restarted, and goal-check verdict correctness (a later step of #90).
- LLM-judge scoring of spec and plan quality.
- Making any workflow faster; this spec measures, and the speed-ups it points to, such as a deterministic plan assemble step, are separate work.
- Documenting the metrics on the spektacular.dev website; documentation stays with the harbor suites in this repository.
