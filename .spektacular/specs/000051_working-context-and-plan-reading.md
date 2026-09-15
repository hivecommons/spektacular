---
created_date: "2026-09-15"
status: completed
closed_date: "2026-09-15"
---

# Feature: 000051_working-context-and-plan-reading

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

When a coding agent picks an interrupted implementation back up, it can mistake its session notes (the working context it keeps as it goes) for the implementation plan, because both are kept in files with the same name, and nothing on resume tells it to read the plan itself. It then reports that work the plan clearly describes was never planned, and offers to invent the missing detail. This feature gives the working context a name of its own, makes the start of an implementation state exactly which plan documents to read, and makes a resumed implementation read the plan before continuing, so an agent resuming work continues from the plan the team approved rather than from a partial record of the last session.

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

- [ ] **Session notes have a name of their own**
  The working context an agent keeps across a session must have a name that no plan document shares, so no instruction can refer to one in a way that reads as the other.

- [ ] **Every workflow keeps using the working context**
  The spec, plan, implement and repo-add workflows must all continue to read and refresh the working context under its new name.

- [ ] **Starting an implementation states which plan documents to read**
  The instructions for starting an implementation must name each plan document, say what each one holds, and say how to read it.

- [ ] **Starting and resuming describe the plan documents consistently**
  Starting and resuming an implementation must name the same plan documents and describe them the same way.

- [ ] **A resumed implementation reads the plan before continuing**
  When an interrupted implementation is resumed, the agent must be told, plainly and before anything else it does to continue, to read the plan documents, whichever step the run was interrupted at.

- [ ] **A resumed implementation finds its place from the plan**
  Resuming must direct the agent to identify the current phase from the plan itself.

- [ ] **Resuming adds only what differs from starting**
  The resume instructions for an implementation must say only that a run is already in progress, to read the plan, to read the working context, to find the next phase, and to continue from the interrupted step.

- [ ] **Steps that act on a phase fetch its detail themselves**
  The steps that write code, write tests and verify a phase must each read the current phase's technical detail from the plan, so none of them depends on an earlier step's output still being in the agent's context.

- [ ] **The plan's technical detail is always named unambiguously**
  Every agent instruction that refers to the plan's per-phase technical detail must make clear it belongs to the plan, so it cannot be mistaken for the working context.

- [ ] **The keep-context-current instruction is identical in every step**
  Every step instruction that continues to a further step must end with the same instruction to refresh the working context before advancing.

- [ ] **Every next command is shown in full**
  Every instruction that directs the agent to run a next command must show that command in full, including its command prefix.

- [ ] **Published documentation shows the current name**
  Documentation on the project's website that quotes agent instructions must show the working context's new name.

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

- The working-context file must be named `working-context` (`.spektacular/working-context.md`). The user chose this name.
- An existing `.spektacular/context.md` left by an earlier version must be neither migrated nor treated as an error. The user chose to ignore it over moving it on first touch or refusing to run, accepting that a run interrupted before upgrading resumes without its previous working context.
- Plans already written must remain readable without change. The plan documents keep their current names, including the plan's `context.md`; only the working-context file is renamed.

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

- [ ] **The working-context file no longer shares a plan document's name**
  After running any workflow step, the working-context file on disk has a name that differs from every document in a plan's directory, and no file named `context.md` is created or written outside a plan's directory.

- [ ] **Each workflow still reads and refreshes the working context**
  Running a step in each of the spec, plan, implement and repo-add workflows produces an instruction that tells the agent to read or refresh the working context under its new name.

- [ ] **The implement start lists the plan documents**
  The instructions an agent receives when starting an implementation name the plan's `plan.md`, its `context.md` and its `research.md`, state what each contains, and give the command to read each one.

- [ ] **Start and resume describe the same plan documents**
  The resume instructions for an implementation name the same plan documents as the start instructions, with the same description of each.

- [ ] **Resuming at any step says to read the plan first**
  For an implementation interrupted at each of its steps in turn, the resume instructions tell the agent to read the plan documents, and that instruction appears before the command to continue the interrupted step.

- [ ] **Resume says to find the current phase from the plan**
  The resume instructions for an implementation tell the agent to determine the current phase from the plan.

- [ ] **Resume instructions carry only the additive items**
  The resume instructions for an implementation contain the statement that a run is in progress, the read-the-plan instruction, the read-the-working-context instruction, the find-the-next-phase instruction and the continue command, and no other procedural steps.

- [ ] **Code, test and verify steps each read the phase detail**
  The instructions for the steps that write code, write tests and verify a phase each include the command to read the current phase's technical detail from the plan, and none of them states that an earlier step's summaries are already available.

- [ ] **No instruction refers to the plan's detail ambiguously**
  Across every instruction the workflows emit, every occurrence of `context.md` is qualified as belonging to the plan, either in words (such as "the plan's `context.md`") or by a path or command that includes the plan's name.

- [ ] **Every continuing step ends with the same refresh instruction**
  Every step instruction that continues to a further step ends with identical keep-context-current wording.

- [ ] **Every next command is shown in full**
  Every emitted instruction that names a next command shows it beginning with the configured command prefix; none shows a bare `spec goto`, `plan goto` or `implement goto`.

- [ ] **The website tutorial shows the new name**
  The tutorial page on the project website that quotes the keep-context-current instruction shows the working context's new name.

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

- Define the plan documents once, in the implement skill's start instructions, and have the resume path refer to that definition rather than restating it.
- Have the step renderer append the keep-context-current footer to every continuing step instruction, so it is defined in one place and changing it is a single edit, replacing the copy currently duplicated into each step template.
- Determine the current phase on resume as the plan's first unchecked phase, rather than recording the phase in workflow state.
- Carry the rename through everywhere instructions reach an agent: the shared resume instructions, the workflow skills, and the regenerated per-agent installed copies.
- Known risk: every emitted instruction changes wording at once, so guard tests that assert on the footer's text or the old file name will need their expected substrings updated as part of the change, not treated as unexpected breakage.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- The failure that prompted this reverses: an implementation interrupted after completing some phases and resumed in a fresh session continues into the next unfinished phase using that phase's detail from the plan, and does not report the plan as missing detail it actually contains.
- Over the first five implementation runs resumed in a fresh session after this ships, none reports a plan/reality mismatch that is traced to the agent reading the working context in place of the plan.

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

- The resume behaviour of the spec, plan and repo-add workflows is unchanged apart from the working-context rename. Naming the documents to read at start, and reading them again on resume, applies to the implement workflow only.
- Checking that every plan phase's technical-detail reference resolves to a matching section is out of scope: it does not address the resume failure this spec targets.

