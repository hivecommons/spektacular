---
created_date: "2026-10-01"
document_status: final
closed_date: "2026-10-01"
designs:
    - source: design
      path: epics-and-seeded-specs.md
---

# Feature: 000060_epics-and-seeded-specs

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Large requests currently become a single specification whose acceptance criteria are too thin to
build from, and work already written up as a tracker issue or design document has to be
re-explained from scratch, with no record of where it came from. This feature lets a specification
be started from that existing material so that only its gaps are asked about, offers to break an
oversized request into a group of smaller specifications that each carry their own testable
criteria and declared order, and gives one place to see how far the whole group has progressed.
Teams who already plan in their trackers get specifications faster, and the agents building from
them get complete criteria instead of filling gaps with guesses.

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

### Epics

- [ ] **Group specs into an epic**
  Users can have several specifications grouped under one epic that holds the overall overview and the list of its specifications, and nothing else.
- [ ] **Two-way membership**
  The system must keep an epic's list of specifications and each specification's record of its epic in agreement whenever either changes.
- [ ] **One epic per spec, no nesting**
  The system must refuse to place a specification in more than one epic, or an epic inside another epic.
- [ ] **Epics are optional, their store is always configured**
  Projects that never split a specification see no change in behaviour. Every project, new or existing, has an epic store configured: new projects get it when they are set up, and existing projects get it when they are brought up to date. The store's folder is created the first time an epic is written, if it does not already exist.

### Splitting

- [ ] **Split offered when a spec is complete**
  When a specification is complete, the agent checks whether it describes more than one independently useful piece of work and, if so, offers a split.
- [ ] **Split on request**
  Users can ask for a split at any point, on any specification, including one already written.
- [ ] **Concrete offers only**
  The agent offers a split only when it can name at least two specifications, each with an acceptance criterion that can be verified without the others.
- [ ] **Supporting work never triggers a split**
  Docs, tests, migrations, config and other work that supports the same change must never count towards a split.
- [ ] **Never automatic**
  The system must never create an epic or split a specification without the user's explicit agreement. After a decline, the offer returns only if a new independent requirement group appears.
- [ ] **Every resulting spec has its own criteria**
  A split must produce specifications that each carry their own overview and testable acceptance criteria.
- [ ] **Split preserves work done**
  Splitting keeps content already written. Content belonging to the narrowed specification stays with it, the rest moves to the new specifications, and shared constraints and non-goals are copied to each specification they apply to.
- [ ] **Splitting a spec already in an epic**
  Splitting a specification that already belongs to an epic adds the new specifications to that same epic.
- [ ] **Chaining**
  When a specification from a split finishes, the agent offers to continue with the next one that hasn't been specified yet, preferring one whose dependencies are met.
- [ ] **Adjustable sensitivity**
  Users can configure how readily split offers are made, independently of how readily new specifications are offered.

### Dependencies

- [ ] **Declare order**
  Users can record which specifications in an epic depend on which others.
- [ ] **Invalid graphs refused**
  The system must refuse an epic whose dependencies name unknown specifications, list a specification twice, or form a cycle.
- [ ] **Checked only when implementing**
  Writing and planning a specification are never blocked by its dependencies. Before implementation starts, the system reports each dependency that isn't yet implemented, along with its state.
- [ ] **Warn or refuse**
  By default, users can continue past an unmet dependency after a warning, and the override is recorded. Projects can configure this to refuse instead.

### Status

- [ ] **One view of progress**
  Users can ask for the status of an epic, specification or plan by name and get the whole epic, every specification's state and what blocks it, plus every plan's tasks, in one result.
- [ ] **Same shape every time**
  The result has the same shape whether the name belongs to an epic, a specification in an epic, or a standalone specification.
- [ ] **Workflow in progress**
  Without a name, users get the status of whichever workflow is currently in progress.
- [ ] **Epic completion is derived**
  An epic counts as done once every one of its specifications has a plan and every task in every plan is complete. This is never set by hand.
- [ ] **Human and machine output**
  Status is available both as readable text and as structured data.

### Starting from existing material

- [ ] **Start from a source**
  Users can start a specification from an existing issue, epic, design document, file, web page or pasted text, whether they refer to it by number, by link, or as "spec from …".
- [ ] **Any tracker**
  The system collects the source's title, body, discussion, child items and a stable link from any tracker. If the source can't be reached, the user is told and asked for the content.
- [ ] **Ask only about gaps**
  The source's content pre-fills the specification. The interview lists what the source doesn't cover and asks only about that, and every later section is confirmed rather than asked from scratch.
- [ ] **Sources with child items**
  A source that already has child items leads to an offer to set up an epic with one specification per child. A child item that appears after the epic exists leads to an offer to add a specification for it.

### Provenance

- [ ] **Record where content came from**
  Specifications and epics record the sources that directly seeded them, with the date each was retrieved.
- [ ] **Full trail on read**
  Viewing a specification shows its own sources followed by its epic's, without copying the epic's sources onto it.

### Implementation

- [ ] **Implementation names the specification**
  Implementation is described to users as implementing a specification, not a plan, with no change in behaviour.

### Documentation

- [ ] **Docs site updated**
  The documentation site covers epics, splitting and split sensitivity; starting a specification from existing material; dependencies; and the single status view and the commands it replaces.

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

- Must be built to the design in design source `design` at `epics-and-seeded-specs.md`, which settles the epic document format, the split triggers and flow, the dependency model and check, the `status` command's output and resolution rules, seeding, provenance, and the decisions taken.
- The CLI must not fetch sources or contain any tracker client; the agent fetches with its own tools, and no specific tool or tracker may be required.
- Seeding must be carried by the skill and workflow instructions; its only CLI surface is accepting the sources to record on the specification.
- The existing `spec status`, `plan status`, `implement status` and `plan export` commands must be removed outright, with no aliases or deprecation period, and everything that calls them must move to the new status view in the same change.
- Epics, specifications and their records must be read and written only through Spektacular's own commands, under the same store-access rules as specs and plans.
- Only one workflow can be active at a time (a single workflow state); chaining and splitting must work within that. Parallel runs belong with #62.
- Everything ships in one release, including the documentation-site changes.

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

### Epics

- [ ] **Epic holds only overview and specs**
  After a split, the stored epic contains an overview and a list of its specifications, and no requirements, acceptance criteria, constraints, non-goals, technical approach or success metrics.
- [ ] **Membership stays in agreement**
  After a split, or after a specification is added to an epic, every specification listed by the epic names that epic, and every specification naming the epic is listed by it. After an epic is deleted, no specification still names it.
- [ ] **No second epic, no nesting**
  Attempting to add a specification that already belongs to one epic to another, or to list an epic as a member of an epic, is refused with an error, and nothing is written.
- [ ] **No change without epics**
  In a project that never splits, every existing spec, plan and implement workflow completes as before.
- [ ] **Store configured for new and existing projects**
  A newly initialised project, and an existing project after migration, both have an epic store configured with the default settings; migrating twice changes nothing further.
- [ ] **Folder created on first epic**
  Writing the first epic in a project whose epics folder does not exist creates the folder and succeeds.

### Splitting

- [ ] **Offer at completion**
  Completing a specification that has two independently shippable requirement groups produces a split offer naming at least two specifications. Completing one with a single coupled requirement set produces no offer.
- [ ] **Split on request**
  Asking for a split mid-workflow, and on an already-written specification, each produces a split offer or a stated reason why it cannot be split.
- [ ] **Offers name their specs**
  Every split offer names each proposed specification with its scope. No offer is made when fewer than two specifications with independently verifiable criteria can be named.
- [ ] **Code plus docs is one spec**
  Completing a specification whose only extra work is documentation, tests or config for the same change produces no split offer.
- [ ] **Nothing without agreement**
  Declining a split leaves no epic and no new specifications stored, and the same offer isn't repeated unless a new independent requirement group appears.
- [ ] **Resulting specs carry criteria**
  After a split, every new specification has a non-empty overview and at least one acceptance criterion.
- [ ] **Work preserved on split**
  After splitting a written specification, every requirement and acceptance criterion it held appears in exactly one of the resulting specifications. A constraint shared by several appears in each of them.
- [ ] **Split within an epic**
  Splitting a specification that already belongs to an epic leaves a single epic listing both the original and the new specifications.
- [ ] **Chaining offer**
  When a specification from a split finishes and another in the epic hasn't been specified yet, the agent offers to start it, naming a ready one first if one exists.
- [ ] **Sensitivity setting**
  A specification showing only weak split signals receives a split offer with the most lenient sensitivity and none with the strictest, and changing the setting leaves the offer behaviour for new specifications unchanged.

### Dependencies

- [ ] **Dependencies recorded**
  An epic written with dependencies between its specifications reads back with the same dependencies.
- [ ] **Invalid graphs refused**
  Writing an epic with a dependency on an unknown specification, a duplicated specification, or a cycle (including a specification depending on itself) fails with an error naming the problem, and nothing is written.
- [ ] **Only implementation is checked**
  Specifying and planning a specification whose dependencies aren't implemented succeed with no warning. Starting its implementation names each unmet dependency and its state, for example "in progress (2/5 tasks complete)".
- [ ] **Warn or refuse**
  With default settings, starting implementation past an unmet dependency warns, allows continuing, and records the override in the changelog. With refusal configured, it is refused.

### Status

- [ ] **Whole epic from any name**
  Asking for the status of an epic, of any of its specifications, or of any of their plans returns the same epic with every specification's state, blockers and plan tasks. The name asked for is identified.
- [ ] **Consistent shape**
  The status result for a standalone specification has the same top-level fields as for an epic, with an empty epic and a single specification.
- [ ] **No name**
  With a workflow in progress, status with no name reports that workflow. With none in progress, it reports that nothing is in progress.
- [ ] **Epic completion**
  An epic is reported as done exactly when every task in every one of its specifications' plans is complete, and as not done while any task is incomplete or any specification has no plan.
- [ ] **Text and structured output**
  Status can be produced as readable text and as structured data containing the same information.

### Starting from existing material

- [ ] **Start from a source**
  Asking to start a specification from an issue, using different phrasings (a number, a link, "spec from …"), starts a specification seeded from that issue in each case.
- [ ] **Unreachable source**
  When the source can't be fetched, the agent reports that and asks for the content, rather than starting a blank interview.
- [ ] **Only gaps asked**
  For a source covering some sections, the interview presents a list of uncovered sections and asks questions only about those. The covered sections are presented as drafts to confirm.
- [ ] **Child items**
  Starting from a source with child items produces an offer to create an epic with one specification per child, not a single specification.
- [ ] **Late child item**
  When a child item is raised for a source whose epic already exists, the agent offers to add a specification for it to that epic.
- [ ] **Source content collected**
  A specification seeded from an issue has drafts reflecting the issue's title, body and discussion, and records the issue's link.

### Provenance

- [ ] **Sources recorded**
  A specification or epic started from a source records that source's link and retrieval date, and a specification started without one records none.
- [ ] **Full trail shown**
  The status of a specification in an epic shows its own sources followed by its epic's, while the specification's stored record contains only its own.

### Implementation

- [ ] **Implement wording**
  The implement command's help and the implement skill describe implementing a specification; implementing an existing specification behaves as before.

### Documentation

- [ ] **Docs published**
  The documentation site has pages, or sections, covering epics and splitting (including split sensitivity), starting a specification from existing material, dependencies between specifications, and the status view. No page still documents the removed status commands as current.

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

- Split seeding guidance between the `spek-new` skill (recognising, fetching and starting) and the interview step's instructions (mapping the source onto sections and listing gaps), so a resumed session still seeds correctly.
- Write the split-check instructions once and reuse them both at the end of the spec workflow and when the user asks for a split, so the two cannot drift.
- The implement workflow's help text, input description and skill should say a spec is being implemented, since the plan shares the spec's name; behaviour does not change.
- The planner chooses the split input format (structured data or a parsed markdown draft); prefer whatever matches how other Spektacular writes take their input.
- Storing a snapshot or content hash of the source text is not expected.
- Prefer keeping source seeding separate from research seeding from knowledge, ADRs and Context7 (PR #65); the planner may share the pre-filled work-file mechanism where it fits naturally.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- A specification started from a tracker issue that covers its overview, requirements and acceptance criteria reaches the end of its interview having asked only about the sections the issue left out; no question re-asks what the issue already answered.
- Across this release's test scenarios, a change made of code plus its docs, tests or config never receives a split offer.
- A user can see where an epic stands — every specification's state, what blocks it, and task progress — with one command instead of the separate status and export commands.

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

- The agent's recognition of spec-worthy discussion is unchanged; split offers are made only when a specification is complete or on explicit request, never from open discussion.
- Plans stay one-to-one with specifications; no plan spans several specifications.
- No syncing with sources: later changes to an issue are not pulled into a specification seeded from it, and nothing is written back.
- No dependencies on specifications outside the same epic, or on standalone specifications.
- Chaining does not extend beyond specifying: no offers to plan or implement the next specification.
- A split never renames the epic or its first specification.
