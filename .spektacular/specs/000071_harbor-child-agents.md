---
created_date: "2026-10-23"
document_status: final
closed_date: "2026-10-23"
---

# Feature: 000071_harbor-child-agents

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

The end-to-end tests that run Spektacular's workflows against real coding agents will exercise child agents the way a real session does: they run in the background while the main agent waits, every step that requires one actually uses one, and each child is given the context it needs. Every test run's record will list each child agent that was started, what it was asked to do and the full context it was given, so maintainers can see how delegation changes between models and releases. Today the tests make the main agent hold its children inside a single turn, which real sessions do not do, and never check that a required independent review actually happened, so problems such as a session sitting idle for a quarter of an hour after its work is done, or a model quietly skipping that review, pass unnoticed.


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

- [ ] **Every child agent is recorded**
  Each test run's metrics record must list every child agent started during the run, including children started by other children, with which agent started it.
- [ ] **What each child was asked to do**
  For each child, the record must hold its kind, its short description, and the full context (launch prompt) it was given, together with that context's size in characters.
- [ ] **How each child ran and ended**
  For each child, the record must show whether it ran in the background or the foreground, when it started and ended, how long it ran, the models it ran on, how much output it produced, how it ended (completed, failed, stopped, or no hand-back) and the first line of what it handed back.
- [ ] **Children placed in the workflow**
  For each child, the record must show the workflow step it was started in and, in multi-spec runs, the spec it worked on.
- [ ] **Children shown in the report**
  Maintainers can see a run's children in the local report, one row per child with its kind, spec, step, background or foreground, duration, how it ended and context size, and can ask the report to print each child's full context.
- [ ] **Unknown, not missing, for other agents**
  For agents whose logs cannot show a child detail, the record must mark that detail as unknown rather than omit it or record a zero.
- [ ] **Required delegation is enforced**
  A test run must fail when a workflow step whose instructions require a child agent completes without one. The steps that currently require one are the spec workflow's independent review of the assembled spec, the implement workflow's test, verify and goal-check steps, and one child per spec in the epic planning and epic implementing workflows; the check follows the instructions if that set changes. Reviewing inline instead of through a child does not satisfy the spec review in a test run.
- [ ] **Suggested delegation is recorded**
  Steps whose instructions only suggest a child agent (plan discovery, implement analysis, changelog formatting) must show in the record whether they used one.
- [ ] **Children's context is checked**
  A test run must fail when a required child's context lacks what its instructions say to pass: for an epic child, the spec it works on, the project root, that it is an orchestrated run, the code locations it must work in (for implementing) and, when the child continues a spec whose earlier run was interrupted, the instruction to resume that run rather than start over; for the spec review child, the staged spec to read and the instruction to review only what is written.
- [ ] **The headless background-child limit is settled**
  The claim that a background child agent is stopped after ten minutes in a headless session must be verified by a test run whose child runs past ten minutes, and the epic workflows' instructions must match the result: if children survive, headless means only that no one answers between turns; if they are stopped, the instructions keep the current workaround of starting children in the foreground together in one message and handling every hand-back before the turn ends.
- [ ] **Epic suites run children as a real session does**
  Unless the ten-minute verification shows background children are stopped, the epic planning and epic implementing test suites must run their children the way an interactive session does: started in the background, with the orchestrator ending its turn and resuming on their completion notices. If background children are stopped, the suites must instead check the foreground workaround.
- [ ] **Being told no one answers does not force foreground children**
  Telling the agent that no one will answer between its turns must not by itself make the orchestrator hold children in the foreground.
- [ ] **No timed wake-ups while waiting on children**
  The epic workflows' instructions must tell the orchestrator never to schedule a timed wake-up, timer or recurring job while waiting on children, and the epic suites must fail a run whose orchestrator does so.
- [ ] **Idle time is measured and flagged**
  Each run's record must hold its idle time, the longest stretch with no agent or child activity, and the report must flag a run whose idle time is more than a quarter of its wall time.
- [ ] **Walkthrough check reads every workflow advance**
  The plan suite's check that the walkthrough step was shown before the finished step must count every plan workflow advance the agent made, however the command that made it was written.
- [ ] **Per-repo changelog check reads every write**
  The implement suite's checks that each repo's changelog record was committed through the command-line tool must accept every such write that actually ran for that repo, however the command was written.


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

- Every test suite must keep telling the agent that the session is headless and that no one will answer between its turns, so it never blocks waiting on a question.
- The new delegation and context checks are enforced for Claude runs only.
- A step whose instructions only suggest a child agent must never fail a test run for not using one.
- The epic workflows' guidance on background versus foreground children must not change until the ten-minute headless limit has been verified in a recorded run.
- Writing the metrics record must never fail a test run, as the existing run metrics already require.
- Changes to the epic workflows' instructions are made in the source the project generates them from, since generated copies are overwritten on the next initialisation.


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

- [ ] **Every child appears in the record**
  After a Claude epic-implementing test run, the run's metrics record lists as many children as the agent's own transcript reports starting, and a child started by another child names that child as the one that started it.
- [ ] **A child's context can be read back in full**
  For any child in a Claude run's record, its recorded context is identical to the launch prompt in the agent's transcript, and its recorded context size equals that prompt's length in characters.
- [ ] **A child's details match the transcript**
  For any child in a Claude run's record, its kind, description, background or foreground setting, start and end times, models and output size match the agent's transcript, its duration matches the agent's own report of that child's run time, and its ending and first hand-back line match what the transcript shows it returned.
- [ ] **Children are placed by step and spec**
  In a Claude epic-implementing run, every child the orchestrator started for a spec is recorded against that spec, and a goal-check reviewer child in an implement run is recorded against the goal-check step.
- [ ] **Report shows and prints children**
  Running the local report on a single collected run prints one row per child with kind, spec, step, background or foreground, duration, ending and context size; asking it for contexts prints each child's full context.
- [ ] **Other agents record unknowns**
  A Codex run's record marks every child detail its logs cannot show as unknown with a reason, and contains no zero in place of an unknown value.
- [ ] **Missing required child fails the run**
  A Claude test run in which the spec review, an implement test, verify or goal-check step, or an epic spec is completed without a child agent fails, and its results name the step or spec that had no child; the same run with the child present passes that check.
- [ ] **Suggested delegation never fails a run**
  A Claude plan test run whose discovery step uses no child agent passes every delegation check, and its record shows that discovery used no child.
- [ ] **Incomplete context fails the run**
  A Claude test run fails, naming the child and what was missing, when an epic child's context omits the spec it works on, the project root or that it is an orchestrated run; when an epic implementing child's context omits the code locations it must work in; when a resumed epic child's context lacks the instruction to resume; or when the spec review child is not given the staged spec or the instruction to review only what is written.
- [ ] **Epic children run in the background**
  Unless the ten-minute verification found background children stopped, passing Claude epic-planning and epic-implementing test runs record the orchestrator's per-spec children as launched in the background, their transcripts show the orchestrator's turn ending while children run and a new turn starting on a child's completion notice, and every spec is planned or implemented.
- [ ] **Test sessions are told no one will answer**
  Every test suite's task tells the agent that no one answers between its turns, and no passing run's transcript ends with the agent waiting on a reply to a question.
- [ ] **Ten-minute limit verified**
  A recorded test run with a background child that runs for more than ten minutes in a headless session exists, its outcome (child completed or child stopped) is stated in the epic workflows' instructions, and those instructions tell the orchestrator to keep children in the foreground only if the child was stopped.
- [ ] **Timed wake-up fails an epic run**
  An epic test run whose orchestrator schedules a timed wake-up, timer or recurring job while children are running fails, naming the call; the epic workflows' instructions state that the orchestrator must not do this.
- [ ] **Idle time recorded and flagged**
  Recording the 2026-10-23 Sonnet 5.5 epic-planning reference run gives an idle time of at least 15 minutes, and the report flags it; the matching Sonnet 4.6 reference run is not flagged.
- [ ] **Walkthrough check counts piped and chained advances**
  Running the plan suite's checks against the 2026-10-23 Sonnet 5.5 plan reference run, whose late advances were piped through another program and chained after other commands, passes the walkthrough-before-finished check; a run that genuinely skips the walkthrough still fails it.
- [ ] **Per-repo changelog check counts looped writes**
  Running the implement suite's checks against the 2026-10-23 Sonnet 5.5 implement reference run, which wrote both repos' changelog records in a shell loop, passes both per-repo changelog checks; a run that never writes a repo's record still fails its check.


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

- Build on the existing harbor run metrics: extend its record and local report rather than adding a separate tool. Child launches, their prompts and their completion notices are already present in Claude's transcript and partly parsed by the shared transcript helpers.
- A proposed shape for each child's entry, which the planner may refine: an identifier, the child that started it, kind, description, background or foreground, spec, step, start and end times, duration, ending, first hand-back line, models, output size, full prompt and prompt size. A schema version bump is expected if the record's shape changes incompatibly.
- Place a child in its step by when it was launched relative to the workflow's own step timeline, the same way run metrics already place output in steps; place it in a spec by the spec its launch prompt names first.
- Measure idle time from gaps in the agent's and its children's timestamped activity; the 2026-10-23 Sonnet 5.5 epic-planning run is the reference case (about 16 of 20.8 minutes idle, caused by a 1200-second wake-up scheduled as a fallback while children ran).
- Keep the 2026-10-23 reference runs (Sonnet 4.6 and 5.5 epic planning, Sonnet 5.5 plan and implement) as checked-in test data, trimmed to what the checks and recorder need, so the acceptance criteria can be re-run without a live batch.
- The epic suites' "this session is headless" wording is what currently activates the epic skills' rule to hold children in the foreground.
- Use a deliberately long-running background child in a headless harbor run to verify the ten-minute claim.
- Make the walkthrough and per-repo changelog checks judge what ran (the workflow's own session log, or the workflow responses and store state) rather than pattern-matching command text.
- Known risks: background children in headless runs may still be stopped by the agent's own limits; requiring children makes the suites stricter, so a newer model that delegates less will now fail runs it used to pass.


<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- 100% of child agents started in a Claude test run appear in that run's record, with their full context, across all nine Claude suites.
- A full Claude batch on the current default model passes with no required delegation skipped and no run flagged for idle time.
- Epic-planning idle time on the current default model is under a quarter of wall time, against about 16 of 20.8 minutes on 2026-10-23.
- Zero failures in a full Claude batch come from the walkthrough or per-repo changelog checks misreading how a command was written.
- A maintainer can answer "which children did this run start, and what were they told?" for any collected run from the local report alone, without opening the transcript.


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

- Switching the test runs to a long-lived sign-in token, so a batch no longer fails when the short-lived one is rotated mid-run, is out of scope for this spec.
- Updating the public website's documentation of the epic workflows is out of scope, even if their guidance on background children changes.
- Blocking the agent's timed wake-up or recurring-job tools inside the test environment is out of scope.
- Differences in newer models' behaviour that do not concern child agents are out of scope, such as an agent fixing a problem itself before the independent review that was meant to find it, or recording values in a guided setup without first proposing them to the user.
- Changes to the third-party test runner or to the coding agents' own software are out of scope.
