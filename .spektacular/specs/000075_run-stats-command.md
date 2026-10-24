---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
epic: 000002_workflow-tracing-and-run-stats
sources:
    - uri: docs/workflow-performance-investigation.md
      retrieved_date: "2026-10-24"
---

# Feature: 000075_run-stats-command

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Maintainers will be able to ask Spektacular directly where a run's time went: how long it took overall and in each step and task, how much of it was spent waiting for helper agents or for the user, and where the longest pauses were. They will also be able to compare runs side by side. Today these numbers are pieced together by hand from agent transcripts and test-harness scripts. A built-in report, read from the logs Spektacular already keeps, makes slow runs and regressions visible in seconds and gives the test harness one trusted source for its timings.


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

- [ ] **Users can list recorded runs**
  Users can list the runs held in the logs, newest first, each with its workflow kind, name, agent, start time, wall time and whether it finished, was interrupted or is still in progress.
- [ ] **Users can choose which runs to report**
  Users can select runs by workflow name, by run identifier, by workflow kind or by agent, and combine these. With no selection, a single-run report covers the most recent run, a comparison covers the two most recent runs of that run's workflow kind, and a summary covers every held run of that kind.
- [ ] **A run's report gives its totals**
  A report on one run must give its wall time (from its first to its last recorded event, so a run in progress is measured to its latest event), its working time (wall time less time waiting on the user), its total time waiting on the user and its total time waiting on helper agents.
- [ ] **A run's report breaks time down by step**
  A report on one run must give, for each workflow step, its total duration, how many times the step was entered, the number of Spektacular commands the agent ran in it, and the time in it spent waiting on helper agents and on the user.
- [ ] **A run's report breaks time down by task**
  For an implement run, the report must give each plan task's duration, with its time waiting on helper agents and on the user.
- [ ] **Helper-agent launches are listed**
  A run's report must list each helper-agent launch with its name, the step it was launched in and how long it ran.
- [ ] **The longest gaps are reported**
  A run's report must list the longest gaps between consecutive recorded events, longest first, each with its length and the step it fell in (10 by default, a number the user can change), and the total time in gaps longer than a threshold (5 minutes by default, a value the user can change).
- [ ] **Interrupted and unfinished work is flagged**
  Steps, tasks, helper launches and waits on the user that were interrupted or never finished must be shown as such. An interrupted step counts toward its step's duration up to the point it was cut off; a helper launch or wait on the user that never finished is listed but left out of the helper-wait and user-wait totals and the per-step wait times.
- [ ] **Users can compare runs**
  Users can compare two or more selected runs side by side, step by step and in total, showing each run's figures and its difference from the first selected run in both time and percentage. A step missing from a run is shown as absent for that run, not as zero.
- [ ] **Users can summarise many runs**
  Users can summarise a set of selected runs, giving for each step and for the totals the number of runs, the median and the 90th percentile.
- [ ] **Reports can be read by programs and people**
  The report must be available as structured output consistent with Spektacular's other commands, and as a readable table.
- [ ] **Users can report on logs from elsewhere**
  Users can point the report at a logs directory other than the project's own, such as one collected from a test run, and get the same report.
- [ ] **Damaged logs do not stop the report**
  A log line that cannot be read must be skipped and counted in the report, not stop the report or change other runs' numbers.
- [ ] **The report never changes the logs**
  Producing a report must not modify, move or delete any log.
- [ ] **The documentation describes the report**
  User documentation must describe how to list, report on, compare and summarise runs, and what each figure means.


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

- The report must be a Spektacular command (user: "It would be good to have a spektacular command that could show run stats").
- The report must read the per-command records, run identifiers and step, task, helper and user-wait spans defined by 000073_log-mode-and-logger-providers and 000074_trace-spans-for-agent-work, on which this spec depends.
- Structured output must follow the JSON envelope and output-format conventions of Spektacular's other commands.
- The structured output's field names must stay stable, because the Harbor spec in this epic (Harbor is Spektacular's end-to-end agent test harness) reads them.


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

- [ ] **Runs are listed newest first**
  In a project whose logs hold three finished runs and one in progress, listing runs shows all four, newest first, each with its kind, name, agent, start time, wall time and status.
- [ ] **The most recent run is the default**
  Asking for a report with no selection reports the run that started most recently.
- [ ] **Selections narrow the runs**
  Selecting by a spec's name reports only that spec's runs; selecting by one workflow kind reports only runs of that kind; selecting by agent reports only that agent's runs; selecting by kind and agent together reports only runs matching both; selecting by run identifier reports exactly that run.
- [ ] **Totals add up**
  For a run that waited on the user and on helper agents for known periods, the reported working time equals the wall time less the total of the user waits, and the reported user-wait and helper-wait totals equal the sums of those periods.
- [ ] **Steps are broken down**
  For a spec run that entered the same step twice, the report shows that step with an entry count of 2, its total duration, its command count and its helper and user wait times.
- [ ] **Tasks are broken down**
  For an implement run of a two-task plan, the report shows both tasks with their durations and wait times.
- [ ] **Helper launches are listed**
  For a run with three helper-agent launches, the report lists all three with name, step and duration.
- [ ] **Gaps are reported**
  For a run containing a 20-minute pause between two commands, the report lists that gap first among the longest gaps, with its step, lists at most 10 gaps longest first, and includes the pause in the total above the default 5-minute threshold; setting the threshold to 30 minutes leaves it out of that total, and asking for 3 gaps lists exactly 3.
- [ ] **Interrupted work is flagged**
  For a run with an interrupted step and a helper launch that never finished, the report marks both, counts the interrupted step's time up to where it was cut off, and leaves the unfinished helper out of the helper-wait totals.
- [ ] **Runs compare side by side**
  Comparing two spec runs shows each step's duration for both runs and the second run's difference from the first in time and percentage, and the same for the totals; a step only one run entered is shown as absent for the other.
- [ ] **Many runs summarise**
  Summarising five implement runs shows, for each step and for the totals, a count of 5, the median and the 90th percentile.
- [ ] **Output works for programs and people**
  The structured output parses as JSON in the same envelope as other Spektacular commands, and the table output shows the same figures.
- [ ] **Collected logs can be reported on**
  Copying a project's logs directory elsewhere and pointing the report at the copy gives the same figures as reporting in the project.
- [ ] **Damaged lines are skipped**
  Adding a malformed line to a log leaves every run's figures unchanged and the report shows one skipped line.
- [ ] **Logs are unchanged by reporting**
  Every log file's contents are byte-identical before and after running each kind of report.
- [ ] **The docs describe the report**
  The documentation site describes listing, reporting, comparing and summarising runs and defines each figure; the site builds without errors.


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

- Suggested command: `spektacular stats`, with a subcommand or form per mode (list, run, compare, summary).
- Pair start and end records by identifier, then roll commands up into steps, tasks and runs.
- Base the gap and wait analysis on the method in `docs/workflow-performance-investigation.md` (gaps between consecutive events, labelled by what surrounds them), so its per-step tables can be reproduced from logs alone.
- Harbor's `tests/harbor/shared/report.py` per-step split is a useful model for the table output.
- Comparisons and summaries key steps by name, so runs of different lengths line up.


<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- The per-step time table in `docs/workflow-performance-investigation.md` (each step's model, tool, helper-wait and user time) can be produced for a new run from the report alone, apart from the model-time column the logs cannot supply.
- A report over a logs directory holding 10 runs (the default retention) returns in under one second on a developer laptop.
- The report's structured output contains every figure Harbor currently derives from transcripts for step, helper and idle timing.


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

- Token counts, model time and cost, which the logs do not hold.
- A visual or HTML view of a run, covered by the log viewer spec in this epic.
- Changing the test harness, covered by the Harbor spec in this epic.
- Reports across several projects at once, or live reports of a run as it happens.

