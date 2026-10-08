---
created_date: "2026-10-08"
document_status: final
closed_date: "2026-10-08"
---

--
created_date: "2026-10-07"
document_status: draft
sources:
    - uri: https://github.com/hivecommons/spektacular/issues/78
      retrieved_date: "2026-10-07"
---

# Feature: 000066_epic-mid-run-revisions

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

When implementing a spec shows that the spec itself, or a design it references, is wrong, there is no defined way to correct it during the run: the implement workflow can only tick checkboxes, the epic orchestrator has no step for revising documents, and the only record of the disagreement ends up buried in the plan changelog. This spec gives the implement workflow, interactive and orchestrated alike, a defined stop for "the spec or a design is wrong", a user-approved way to amend the spec or design in the project mid-run that the running child then picks up, and a record on the spec of what was amended and why. It closes the last open item from #78's second epic-run report (item 6).

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

- [x] **The implement workflow stops when the spec or a design is wrong**
  When implementing or verifying shows that a requirement, acceptance criterion, constraint or success metric of the spec, or a rule in a design the plan references, is wrong or contradicts another, the implement workflow treats it as a stop rather than working around it. An interactive run asks the user; an orchestrated run hands the question back to its orchestrator, naming the document, the section, the conflict, and a proposed amendment.
- [x] **Users can approve an amendment to the spec during an implement run**
  The agent that talks to the user (the orchestrator in an epic run, the implement agent in an interactive run) can, with the user's explicit approval, amend the spec's requirements, acceptance criteria, constraints or success metrics in the project while the spec's implement run is in progress.
- [x] **Users can approve a revision to a referenced design during an implement run**
  The same agent can, with the user's explicit approval, revise a design document the spec references, in the project, while the run is in progress, keeping the design's existing references and capture date.
- [x] **A child never amends a spec or design itself**
  An orchestrated child proposes amendments only through its hand-back; it never writes the spec's text or a design document.
- [x] **Every mid-run amendment is recorded on the spec**
  Each approved amendment is recorded in the spec with its date, the sections or design it changed, the reason, and the run it came from, so the spec explains its own history without reading the plan.
- [x] **Every mid-run amendment is recorded in the plan's changelog**
  The running implement workflow records the amendment as a plan changelog entry alongside the task in which the conflict was found.
- [x] **A recorded amendment does not block the run**
  An amendment recorded this way does not make the spec's plan stale, so the run continues, and can be resumed, even when the project treats spec changes after planning as making a plan stale. A spec edit made any other way keeps today's staleness behaviour.
- [x] **The running child picks up the amended document**
  After an amendment is applied, the child (or interactive run) re-reads the amended spec or design before it continues, and verifies the current task against the amended text.
- [x] **The skills and docs describe the amendment path**
  The guidance agents follow for implementing a spec and an epic, and the documentation site's implement and epics pages, describe when to raise an amendment, who approves it, and how it is applied and recorded.

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

- Specs, plans and designs are only ever written in the project, through the Spektacular CLI; nothing is written under a worktree's `.spektacular`, and the epic merge guard that refuses `.spektacular` changes on a spec branch stays as it is.
- An orchestrated child never asks the user anything itself; every amendment goes through the existing `QUESTION:` hand-back.
- No amendment is applied without the user's explicit approval, in epic and interactive runs alike.
- Design documents the user supplied (stored with `design write`) are never rewritten by Spektacular; revising one means the user supplies the new version.

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

- [ ] **An orchestrated child hands back a spec conflict as a question**
  In an orchestrated run whose verify step finds a spec success metric contradicting a referenced design, the child stops and hands back a question to the orchestrator that names the document, the section, the conflict and a proposed amendment; the spec and design files in the project are unchanged at that point.
- [ ] **An interactive run asks the user about a spec conflict**
  In an interactive implement run with the same conflict, the workflow stops and asks the user whether to amend the spec or design, rather than continuing to the next step.
- [x] **An approved spec amendment lands in the project and is recorded**
  After the user approves an amendment to a success metric, the spec in the project store holds the new metric text and an amendments record with date, section, reason and run; nothing under any worktree's `.spektacular` has changed.
- [x] **An approved design revision keeps its references**
  After the user approves a design revision mid-run, the design in the project holds the new text, its capture date is unchanged, and every spec that referenced it still does.
- [ ] **The plan changelog records the amendment**
  The plan's changelog has an entry for the task in which the conflict was found that names the amended document and the reason.
- [x] **A recorded amendment does not make the plan stale**
  With `plan.strict_spec_changes: true`, after a recorded mid-run amendment, `spektacular status <spec>` does not report the plan as stale, and re-running `implement new` for the orchestrated lane resumes it instead of refusing with `plan_stale`.
- [x] **An unrecorded spec edit is still stale**
  With `plan.strict_spec_changes: true`, editing the spec through `spec file write` without recording an amendment still makes the plan stale, as today.
- [ ] **The child continues against the amended text**
  After the orchestrator answers the question with the applied amendment, the child re-reads the spec or design and re-runs verification of the current task against it before advancing.
- [x] **A child cannot amend the spec**
  The child prompt the epic orchestrator gives, and the instructions the implement workflow returns, both state that a child never writes spec or design text and proposes amendments only through its hand-back.
- [x] **The docs describe the path**
  The documentation site's implement and epics pages describe raising, approving, applying and recording a mid-run amendment.

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

- Add the "spec or design is wrong" stop to the implement step instructions where conflicts surface (implement, test, verify), and to the definition of a genuine open question in the `spek-implement-epic` skill.
- Give the orchestrator skill an explicit "apply an approved amendment" step: amend the spec via `spektacular spec file read`/`write` (or revise the design via `design author` / user-supplied `design write`) from the project root, then answer the child naming what changed.
- Record amendments with a CLI-supported operation rather than free-form edits, so that the record and the staleness exemption cannot drift apart. One option is a `spec amend` command (or a flag on `spec file write`) that writes the spec, appends the `## Amendments` entry and records the amendment in the spec's metadata; plan staleness then ignores spec changes that are fully explained by recorded amendments. The planner may choose the mechanism.
- Plan staleness today compares when the spec and the plan were last written; the exemption needs something more than mtime, such as a recorded amendment timestamp or a content hash at plan approval.
- The child's re-read can reuse the existing read_plan guidance, which already reads every design the plan references.
- Open question for review (drafter's default): who applies an amendment. Default: only the agent talking to the user, after explicit approval; a child never does.
- Open question for review (drafter's default): where the record lives. Default: a `## Amendments` section on the spec, plus a plan changelog entry.
- Open question for review (drafter's default): whether a recorded amendment exempts the plan from staleness. Default: yes; unrecorded edits stay stale.
- Open question for review (drafter's default): scope of amendable sections. Default: requirements, acceptance criteria, constraints and success metrics, not only success metrics.
- Amendments need not add any new automatic git commits; they can ride on the run's existing commits under the project's `auto_commit` setting.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- In an epic run where verification reveals a spec/design conflict, the run completes without hand-editing files, without `override_dependencies`, and without writing anything inside a worktree's `.spektacular`.
- After such a run, the spec's text matches what was built, and its `## Amendments` section explains every mid-run change without needing to read the plan changelog.
- No epic merge is refused with `epic_merge_touches_spektacular` because of a mid-run spec or design change.

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

- Re-planning: an amendment that invalidates the plan's tasks is out of scope; the run stops and the user re-plans as today.
- Editing a spec outside an implement run; the spec workflow already covers that.
- #80 and #78 items 1, 4 and 5, which are being fixed separately.
