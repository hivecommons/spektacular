---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
epic: 000002_workflow-tracing-and-run-stats
sources:
    - uri: docs/workflow-performance-investigation.md
      retrieved_date: "2026-10-24"
---

# Feature: 000074_trace-spans-for-agent-work

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

A run's time is spent on more than the commands an agent sends: in each workflow step, on each plan task, waiting for helper agents to finish, and waiting for the user to answer. Spektacular will record each of these as a timed span within the run, marking steps and tasks itself as the workflow moves, so agents only mark the helper agents they start and the questions they put to the user. Time spent waiting on the user is recorded separately, so maintainers can see how long the agent's own work took, apart from the time a person took to reply.


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

- [x] **Workflow steps are recorded without agent action**
  The system must record each workflow step of a run as a span that begins when the workflow enters the step and ends when it leaves it (moves to another step, finishes, or is restarted), without the agent doing anything beyond moving the workflow as it does today.
- [x] **Commands nest under their step**
  Each command recorded during a step must be linked to that step's span as its parent, so a run reads as steps containing the commands sent in them.
- [x] **Interrupted steps are closed on resume**
  When a run is resumed after its session stopped mid-step, the step left open must be closed, marked as interrupted, at the time of the last command recorded in it (or at its own start if none was), together with any span still open under it, and the resumed step recorded as a new span. A step left open when a run is restarted from scratch is closed, marked as abandoned.
- [x] **Plan tasks are recorded without agent action**
  When implementing a plan, the system must record each plan task as a span covering every step of that task's work, without the agent marking it. A task span belongs directly to the run, and the step spans for that task's work nest under it; outside a task, step spans belong directly to the run.
- [x] **Agents can start and stop a span**
  Agents can start a span with a name and a type and receive its identifier, then stop it by that identifier with a success or error status.
- [x] **Spans nest under a chosen parent**
  A span an agent starts must be linked to the current step's span by default, or to any other open span the agent names, so the spans of several helper agents running at the same time each nest under the span that launched them. Naming a parent that is unknown or already closed reports an error and records nothing.
- [x] **Span types are distinguishable**
  Every span must carry a type: step, task, sub-agent, user wait, or general work.
- [x] **Waits on the user are their own type**
  Time the agent spends waiting for the user to answer must be recordable as a user-wait span, distinct from every other type, so it can be counted separately from or left out of the run's working time.
- [x] **Instructions tell agents what to mark**
  The instructions Spektacular gives agents for every workflow (spec, plan, implement, epic planning and epic implementation) must tell the agent to mark each helper-agent launch as a sub-agent span and each question put to the user as a user-wait span, and must give helper agents the parent span to nest their own spans under.
- [x] **Unclosed spans are closed with their parent**
  A span still open when its parent span ends, or when its run finishes, must be closed then, marked as abandoned.
- [x] **Trace commands are safe**
  Starting or stopping a span must never change the workflow's state or step; stopping an unknown or already stopped span reports an error that names the problem and changes nothing.
- [x] **Trace commands respect log mode**
  Starting and stopping spans must follow the project's logging settings: with logging switched off they succeed and record nothing.
- [x] **The documentation describes spans**
  User documentation for logging must describe the span types, which spans Spektacular records itself, and the commands agents use to start and stop spans.


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

- Step spans must be opened and closed by the workflow's own step changes; agents must not be asked to mark steps (user: "it makes sense that goto opens and closes spans, also because it is one less thing for the agent to do").
- Existing workflow commands must keep their current interface and behaviour; recording spans must not require agents to change how they move a workflow or launch helper agents.
- Spans must use the record format, run identifier and logger defined by 000073_log-mode-and-logger-providers, on which this spec depends.


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

- [x] **Steps appear as spans**
  After a spec workflow moves through `new`, `interview` and `overview`, the run's log holds a step span for each of `new` and `interview`, each ending when the next began, and an open span for `overview`; the agent ran no command other than the usual workflow commands.
- [x] **A finished run closes its last step**
  After a workflow reaches `finished`, no step span of that run is left open.
- [x] **A restart closes the old run's step**
  Running `spec new --force` while a run is mid-step closes that run's open step span, marked abandoned.
- [x] **Commands are children of their step**
  Every command recorded while a run is on a step carries that step's span identifier as its parent.
- [x] **Interrupted steps are marked on resume**
  Stopping a session mid-step, with a helper span still open, and resuming the run produces a closed span for the interrupted step, marked interrupted and ending at the time of its last recorded command, the helper span closed and marked interrupted, and a new span for the resumed step.
- [x] **Tasks appear as spans in implement**
  After implementing a plan with two tasks, the run's log holds one task span per task, each naming its task, with the step spans of that task's work as its children, and no trace command run by the agent.
- [x] **An agent-started span is recorded**
  Starting a span of type sub-agent named `test-writer` returns an identifier; stopping it with that identifier records a span of type sub-agent named `test-writer` with a start, an end and a success status.
- [x] **A failed span records its error**
  Stopping a span with an error status records the span with an error status.
- [x] **Default parent is the current step**
  A span started without naming a parent carries the current step's span identifier as its parent.
- [x] **Concurrent children nest correctly**
  Two sub-agent spans started at the same time, each naming a different parent span, are recorded with their own parents, and spans started by each helper with its parent's identifier nest under the right one.
- [x] **User waits are typed**
  A span started with the user-wait type is recorded with the user-wait type, and no other span type is recorded as user wait.
- [x] **Unclosed spans are abandoned**
  A span started during a step and never stopped is closed, marked abandoned, when the workflow moves to the next step; a span started in the last step and never stopped is closed, marked abandoned, when the workflow reaches `finished`.
- [x] **A bad parent is refused**
  Starting a span that names an unknown parent, or one already closed, returns an error naming the problem and records no span.
- [x] **Trace commands leave the workflow alone**
  Starting and stopping spans leaves the workflow's current step and completed steps unchanged.
- [x] **Bad stops are reported**
  Stopping an identifier that was never started, or one already stopped, returns an error naming the problem, and the log holds no new span end for it.
- [x] **Logging off records no spans**
  With logging switched off, starting and stopping a span succeed and no record is written.
- [x] **Instructions name the spans**
  Every workflow step instruction or skill that launches a helper agent tells the agent to mark the launch as a sub-agent span and passes the parent span to the helper, and every one that asks the user a question tells the agent to mark the wait as a user-wait span.
- [ ] **Harbor runs show helper and user spans**
  In one run each of the Harbor agent test harness's implement, plan-epic and implement-epic suites, every helper-agent launch appears in the log as a sub-agent span; in one run of its spec suite, every question put to the scripted user appears as a user-wait span. (Success Metrics track the rate over repeated runs.)
- [x] **The docs describe spans**
  The documentation site's logging page describes the span types, the spans recorded automatically, and the start and stop commands; the site builds without errors.


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

- Add `spektacular trace start` (name, type, optional `--parent`) returning a span identifier, and `spektacular trace stop <id>` (optional status).
- Spans that open in one command and close in another are written as separate start and end records and paired by identifier when read, the shape 000073 leaves room for.
- Pass the parent span identifier to a helper agent in its brief, rather than keeping a span stack in workflow state; a stack cannot represent several helpers running at the same time.
- Keep the open step span's identifier where moving the workflow can find it (alongside the run identifier in the workflow state), so the next move can close it; the same applies to implement's current task.
- Put the span instructions in shared template partials, so every step and skill words them the same way, and keep them short: long instructions and extra agent turns are the main costs of a workflow (see the performance investigation in `docs/workflow-performance-investigation.md`).
- Keep explicit trace calls to helper launches and user waits, and let agents chain them with the command they already run where they can, so marking work adds as few agent turns as possible.
- Harbor's instruction-contract checks will need updating alongside the templates.


<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- In a Harbor implement run, the step spans together cover at least 95% of the time between the run's first and last command.
- In Harbor implement, plan-epic and implement-epic runs, at least 90% of helper-agent launches appear as sub-agent spans.
- In Harbor spec runs, at least 90% of questions put to the scripted user appear as user-wait spans.
- Marking spans adds on average no more than two agent tool calls per plan task in the Harbor implement suite.


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

- Summarising, comparing or excluding user waits from timings is done by the run statistics spec in this epic; this spec only records the spans.
- Replacing the test harness's transcript-based timing is covered by the Harbor spec in this epic.
- Drawing spans as a waterfall is covered by the log viewer spec in this epic.
- Detecting waits on the user or helper-agent launches without the agent marking them.
- Recording the agent's own tool calls or model calls as spans, which the CLI cannot see.
- Changing how or when helper agents are launched.

