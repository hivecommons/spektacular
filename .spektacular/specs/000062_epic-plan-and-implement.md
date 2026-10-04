---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
designs:
    - source: design
      path: epics-and-seeded-specs.md
---

# Feature: 000062_epic-plan-and-implement

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

When a piece of work has been split into an epic of several specs, users can move the whole epic forward with two simple requests: "plan this epic", which produces a plan for every spec that still needs one, and "implement this epic", which then builds them in dependency order. Specs that don't depend on each other are planned or built side by side by separate agents, each following the standard per-spec process. Today each spec has to be planned and implemented by hand, one at a time, with frequent stops for confirmation. This change cuts that repetitive overhead to a few deliberate checkpoints. Because each request only does the work that is still outstanding, an interrupted epic can be picked up again by making the same request.

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

- [ ] **Plan an epic on request**
  Users can ask the agent to plan an epic by name, or in plain wording such as "plan this epic". The agent then produces a plan for every spec in that epic that does not yet have one, without the user starting each spec's planning separately.
- [ ] **Implement an epic on request**
  Users can ask the agent to implement an epic by name, or in plain wording such as "implement this epic". The agent then implements every planned spec in that epic that is not yet implemented, without the user starting each spec's implementation separately.
- [ ] **Dependency order for implementation**
  When implementing an epic, the system must implement a spec only after every spec it depends on has been implemented.
- [ ] **Dependency order for planning**
  When planning an epic, a spec is planned only after every spec it depends on has been planned, so its plan can take theirs into account.
- [ ] **Independent specs run in parallel**
  When planning or implementing an epic, specs whose dependencies are all satisfied (planned for planning, implemented for implementation) are worked on at the same time by separate agents, not one after another.
- [ ] **Parallel work is combined before dependents start**
  Work from specs implemented in parallel is combined into the main line of development before any spec that depends on it starts. A conflict when combining is reported to the user, not resolved silently.
- [ ] **Open questions reach the user**
  When an agent working on one spec hits a genuine open question, the question is put to the user and the answer is returned to that agent. Agents working on other specs carry on meanwhile.
- [ ] **Only outstanding work is done**
  Specs that already have a plan are not re-planned when planning an epic. Specs already implemented are not re-implemented when implementing an epic.
- [ ] **Resumable by repeating the request**
  If a run is interrupted, the user can make the same request again. It continues from where the epic stands, with no work already completed being lost or repeated.
- [ ] **Fewer interruptions than per-spec runs**
  Planning or implementing an epic must ask the user for input less often than running each spec's plan or implementation one by one. The agent stops only for genuine open questions it cannot resolve, and for the end-of-planning review.
- [ ] **Review of the epic's plans**
  Once planning an epic has finished, the user is shown a summary of each plan it produced, so the plans can be reviewed before any implementation starts.
- [ ] **Stop on failure**
  If implementing a spec fails, no new specs are started; specs already running are allowed to finish. The user is told which spec failed, why, and what else completed.
- [ ] **Stop on planning failure**
  If planning a spec fails, or hits an open question the user chooses not to answer now, no new specs are started; specs already running are allowed to finish. The user is told which spec stopped the run, why, and what else completed.
- [ ] **Resume part-way through a spec**
  If a run is interrupted while a spec's plan or implementation is still in progress, repeating the request resumes that spec's in-progress work from where it stopped rather than restarting it.
- [ ] **Broken dependencies are refused**
  If an epic's dependencies contain a cycle, or name a spec outside the epic that is not implemented, implementing the epic refuses before starting any spec and reports the problem to the user.
- [ ] **Progress is visible**
  While planning or implementing an epic, the user can see which specs are being worked on and how many remain. At the end, the user is told what was completed, what was skipped because it was already done, and what is still outstanding.
- [ ] **Implementing needs plans**
  When asked to implement an epic in which some specs have no plan yet, the system refuses up front: it names the unplanned specs and implements nothing until they are planned.
- [ ] **Documentation**
  The project's public documentation explains how to plan and implement an epic, including the resume and skip behaviour.

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

- The feature is driven by an agent skill, so epics are planned and implemented by asking the agent. It is not a new standalone CLI command that runs on its own.
- The feature must work with epics as they are already defined, including the specs each epic lists and the dependencies between them. No new epic format or structure is needed before it can be used.
- Each spec's plan and implementation must come out as an ordinary plan and an ordinary implementation, the same as if that spec had been planned or implemented on its own. Anything that already reads plans, changelogs or epic status must keep working on them.
- Each spec's plan is produced with the standard plan skill, and each implementation with the standard implement skill, run by a separate agent per spec.
- Parallel planning happens in the project's own working copy.
- Parallel implementation gives each spec its own git worktree. Worktrees are merged back at every dependency junction, before any spec that depends on them starts.
- Must honour the existing epics design in the `design` source at `epics-and-seeded-specs.md`. It settles what an epic is, and that the tool never refuses to write or plan a spec because of its dependencies. Planning an epic in dependency order is this feature's own sequencing choice, not such a refusal.

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

- [ ] **Plan an epic on request**
  Take an epic of three specs, none of them planned. A single "plan this epic" request ends with a plan in the plan store for each of the three specs. The user never has to start an individual spec's planning.
- [ ] **Implement an epic on request**
  Take an epic whose specs are all planned and none implemented. A single "implement this epic" request ends with every spec reported as implemented by the epic's status report. The user never has to start an individual spec's implementation.
- [ ] **Dependency order for implementation**
  Take an epic where spec B depends on spec A. Implementing the epic finishes A's implementation (it has a changelog record and shows as implemented) before B's implementation begins. This holds even when B is listed before A in the epic.
- [ ] **Dependency order for planning**
  Take an epic where spec B depends on spec A, neither planned. Planning the epic produces A's plan before B's planning begins, and B's plan can refer to A's plan.
- [ ] **Independent specs run in parallel**
  Take an epic with independent specs A and C. Planning the epic plans A and C in overlapping time rather than one after the other, and implementing it later implements them in overlapping time too.
- [ ] **Parallel work is combined before dependents start**
  Take an epic where B depends on independent specs A and C. When implementing it, A's and C's changes are both present in the main line before B's implementation begins. If combining A's and C's changes conflicts, the run stops and the user is shown the conflict.
- [ ] **Open questions reach the user**
  While an epic is planned, an open question raised by the agent planning A is shown to the user, and the user's answer is reflected in A's plan. Meanwhile, planning of an independent spec C continues.
- [ ] **Only outstanding work is done**
  Take an epic where one spec already has a plan. Planning the epic leaves that plan unchanged (same content and created date) and plans only the others. Likewise, an already-implemented spec gets no new implementation run and no new changelog record when the epic is implemented.
- [ ] **Resumable by repeating the request**
  Stop a plan or implement run on an epic after its first spec completes, then make the same request again. The second run starts with the next outstanding spec, and when it ends every spec is planned (or implemented) exactly once.
- [ ] **Fewer interruptions than per-spec runs**
  Plan an epic whose specs raise no genuine open questions. The user is not asked to confirm individual plan sections, and the only stop is the end-of-planning review. Implementing such an epic asks for no input between specs.
- [ ] **Review of the epic's plans**
  When planning an epic finishes, the user is shown one summary entry for every plan produced in that run, before any implementation starts.
- [ ] **Stop on failure**
  Take an epic where spec A's implementation fails while independent spec C is being implemented. C's implementation completes, no further spec starts, and the final message names A as the failed spec with its reason and lists C as completed.
- [ ] **Stop on planning failure**
  Plan an epic where planning spec A raises an open question the user declines to answer now while independent spec C is being planned. C's plan completes, no further spec starts planning, and the final message names A and the reason it stopped and lists C as completed.
- [ ] **Resume part-way through a spec**
  Interrupt an epic run while a spec's plan is part-way through, then repeat the request. The run continues that spec's plan from the step it had reached, and the sections already completed are not gathered again.
- [ ] **Broken dependencies are refused**
  Ask to implement an epic whose dependencies form a cycle (A depends on B, B depends on A), and separately one in which a spec depends on an unimplemented spec outside the epic. In both cases no spec is implemented, and the user is told the cycle or the missing dependency.
- [ ] **Progress is visible**
  During a plan or implement run on an epic, the user is told which specs are being worked on and how many remain. The final message lists the specs completed, skipped as already done, and still outstanding.
- [ ] **Implementing needs plans**
  Ask to implement an epic in which one spec has no plan. The user is told the name of the unplanned spec, and no spec in the epic is implemented by that request.
- [ ] **Documentation**
  The published documentation has a page or section that describes planning and implementing an epic, including that a repeated request resumes and that completed work is skipped.

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

- Spektacular currently allows only one workflow in progress at a time. Parallel planning in a single working copy needs that lifted for plan workflows. The plan workflow should design how.
- Prefer working out what is outstanding (unplanned or unimplemented specs) from the project's existing record of where an epic stands, rather than keeping separate progress tracking for an epic run. That is what lets a repeated request resume.
- Still open: whether this comes from extending the existing plan and implement skills to accept an epic, or from new epic-level skills. The plan workflow should propose one.
- Risk: cutting interruptions during planning means the agent settles plan decisions the user would normally confirm section by section. The planner should decide what counts as a "genuine open question" that still stops the run.
- Risk: the open-questions relay and the worktree merging add coordination the planner should design deliberately.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Epics are taken from written to implemented without the user starting any individual spec's plan or implementation.
- Across real epic planning runs, the number of user prompts per run is at most one plus the number of genuine open questions raised.

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

- Writing the specs of an epic in one request ("spec this epic") is out of scope. This feature starts once the epic's specs exist.
- A single request that plans and then implements an epic end to end is out of scope. Planning and implementing stay as two separate requests, with a review in between.
- Planning or implementing only some of an epic's specs, by the user picking a subset, is out of scope. A request covers every outstanding spec in the epic.
- Detecting or refreshing stale plans when a spec has changed is out of scope; they are left for the user to handle as they do today.
