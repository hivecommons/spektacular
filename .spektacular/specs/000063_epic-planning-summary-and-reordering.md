---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Feature: 000063_epic-planning-summary-and-reordering

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

When an epic is planned with one request, the user gets a single summary document for the whole epic: a section for every plan it produced, with the decisions they need to make listed first. Specs planned side by side can quietly collide: two plans change the same files with nothing ordering them, plans disagree on a project-wide rule, or a plan departs from a choice the user already made. Today these surface only as a dense list at the end of planning, or not at all. With this change, planning orders colliding specs itself, raises contradictions of the user's own decisions as questions while it runs, and gathers cross-plan disagreements into one place with a proposed answer. Users then get plans that can be implemented in parallel without conflicts, and one document they can review and return to.

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

- [x] **A summary of the epic's plans is kept**
  When planning an epic finishes, the system leaves one summary document for that epic. It holds a section for each spec planned, covering the approach, the milestones and tasks, any tasks needing a person, what is out of scope, and the drafting assumptions.
- [x] **Decisions come first**
  The summary document opens with the decisions the user needs to make, before any per-plan section.
- [x] **The summary can be read and updated**
  Users and agents can read and update an epic's summary document.
- [x] **The review walks the summary**
  The end-of-planning review presents the summary document. Any change the user asks for is applied to the affected plan and to the summary, so the two never disagree.
- [x] **Repeated planning keeps the summary current**
  When planning an epic is repeated, the summary gains sections for newly planned specs and keeps the sections for specs planned before.
- [x] **Overlapping specs are ordered automatically**
  Two specs in the epic that change the same files, and have no ordering between them either directly or through other specs, are given a dependency without asking the user. The spec listed earlier in the epic goes first.
- [x] **Added dependencies are reported**
  Each dependency added because of overlap is listed in the summary with the files the two specs share, so the user can see why and undo it during the review.
- [x] **Adding a dependency never re-plans**
  Ordering two specs because they overlap changes only the order they are implemented in; neither plan is re-planned.
- [x] **Contradicting the user's own decisions is a question**
  Planning a spec stops to ask the user when its plan would contradict a decision the user recorded for that spec: in the spec, in a design it references, or, where they still exist, in the notes from writing it. During epic planning, the other specs carry on meanwhile.
- [x] **Contradicting the knowledge base is a question**
  Planning a spec stops to ask the user when its plan would contradict a knowledge entry. It is never left as a task for a person or a note for the review. During epic planning, the other specs carry on meanwhile.
- [x] **Cross-plan disagreements are gathered with a proposed answer**
  When plans disagree on a project-wide rule, meaning a convention that governs files or practices more than one plan touches (such as how the changelog is kept), the summary's decisions name the disagreement, the plans involved, and one proposed answer.
- [x] **Settling a disagreement updates every plan**
  When the user settles a cross-plan disagreement, every plan involved is updated to match, and the summary records the outcome.
- [x] **Single-spec planning is otherwise unchanged**
  Planning one spec on its own works as before, apart from the two questions above: it produces no summary document and changes no epic.
- [x] **Documentation**
  The public documentation's epic page describes the summary document, the automatic ordering of overlapping specs, and the questions raised while planning.

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

- The summary document is kept with its epic, not stored as a plan: plans stay one per spec.
- Each spec's plan is still produced by the standard plan workflow, run by a separate agent per spec.
- Every document Spektacular manages, including the summary, is read and written only through the CLI.
- Dependencies added because two specs overlap are written without asking the user first.
- If the epic's format changes, the epics design document must be updated to match.

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

- [ ] **A summary of the epic's plans is kept**
  Plan an epic of three unplanned specs. When planning finishes, the epic has a summary document with exactly three per-plan sections, one for each spec. Each section names the approach, the milestones and tasks, any tasks needing a person (or says there are none), what is out of scope, and the drafting assumptions.
- [x] **Decisions come first**
  In the summary document, the decisions section appears before the first per-plan section. It is present, and says there are none, when there are none.
- [x] **The summary can be read and updated**
  An epic's summary document can be read and rewritten through Spektacular. Asking for the summary of an epic that has none is refused with a message saying how to get one.
- [ ] **The review walks the summary**
  During the review, ask for a change to one plan. Afterwards, both that plan and its section in the summary show the change.
- [ ] **Repeated planning keeps the summary current**
  Plan an epic, stop after one spec is planned, then repeat the request. When it finishes, the summary has a section for every spec, and the first spec's section is unchanged.
- [x] **Overlapping specs are ordered automatically**
  Take an epic where independent specs A and B, with A listed first, both change the same file. After planning, the epic records that B depends on A, and the user was not asked. Specs that already have an ordering between them, directly or through another spec, gain no new dependency.
- [x] **Added dependencies are reported**
  In the same epic, the summary lists the dependency added between A and B and names the shared file.
- [x] **An added dependency can be undone**
  During the review, ask to remove the dependency added between A and B. Afterwards the epic no longer records it, and the summary shows it was removed.
- [x] **Adding a dependency never re-plans**
  In the same epic, A's and B's plans have the same content and created date before and after the dependency is added.
- [ ] **Contradicting the user's own decisions is a question**
  Take a spec whose referenced design records choice X while one of its requirements pushes toward not-X. Planning puts a question to the user about it before that spec's plan is final. During epic planning, an independent spec keeps planning meanwhile.
- [ ] **Contradicting the knowledge base is a question**
  Take a spec whose requirement contradicts a knowledge entry. Planning asks the user about it before that plan is final. No plan task assigns updating the entry to a person instead.
- [ ] **Cross-plan disagreements are gathered with a proposed answer**
  Take two specs in an epic whose plans keep the changelog differently, one editing it by hand and one treating it as generated. The summary's decisions section names the rule, both plans, and one proposed answer.
- [ ] **Settling a disagreement updates every plan**
  After the user accepts the proposed answer, both plans follow it, and the summary marks the decision settled. Likewise, when the user picks a different answer instead, both plans follow the user's choice.
- [x] **Single-spec planning is otherwise unchanged**
  Planning one spec on its own produces no summary document and changes no epic. Its steps and questions are the same as before this change, apart from the two new questions.
- [ ] **Documentation**
  The published epics page describes the summary document, the automatic ordering of overlapping specs, and the questions raised while planning.

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

- Build on 000062's epic planning: the orchestrator skill, the child hand-back contract, and the `run` view in `status`.
- Prefer the CLI working out overlapping specs deterministically, so it is covered by tests, rather than leaving the comparison to the agent's judgement. The files each plan names per task are the natural input.
- Build the summary from each child's `DONE:` summary, addressed by the epic's name.
- Widen the definition of a genuine open question where it already lives: the epic skills and the orchestrated hand-back text the plan steps show. Planners should check a plan's choices against the spec's recorded decisions and the knowledge base before treating it as final.
- The epics design (`epics-and-seeded-specs.md`) may change where that helps, for example if the summary or the added dependencies need the epic's format to change.
- Risk: the interview notes from writing a spec are removed once the spec is finished, so "decisions recorded in the notes" may not be available to the planner. The plan should decide how to handle that.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Across real epic implementation runs, no merge conflict comes from two specs that planning left unordered while both changed the same file.
- In real epic planning runs, no contradiction of a recorded user decision or a knowledge entry first appears at the end-of-planning review: each is raised as a question while planning runs.
- In real epic reviews, users settle every listed decision without opening an individual plan.

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

- A summary document for an epic that was not planned with one request.
- Changing how implementing an epic works, apart from it running in the new order.
- Refusing tasks for a person that only ask for a review, which is a separate follow-up.
- Settling what any particular project's own rules are, such as how a project keeps its changelog. Planning only surfaces the disagreement and proposes an answer.
