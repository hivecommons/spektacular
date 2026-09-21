---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Feature: 000055_design-authoring-skill

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Spektacular can help a user work out and write down a design document through a guided
interview, rather than assuming the design already exists in the conversation and only needs
capturing. Agents notice conversation about a design in each of its forms, whether that is a
design the user already has, one being revised, or design talk before any spec exists, and act
on it instead of letting the detail pass. Designs Spektacular authors record their lifecycle
status, where they came from, and the specs that reference them, while a folder of designs the
team already had is read and stored with its content untouched.

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

- [x] **Agents act on conversation about a design**
  When a conversation concerns the worked shape of something (an API, a user-facing flow, a data format, or a worked example of one), the agent must offer to capture or hand off that design once it qualifies, rather than letting the detail pass unremarked or waiting to be asked.
- [x] **A design the user already has can be brought in**
  When the user arrives with a design already written, or points at one, the system must be able to store and reference it without the user re-authoring it and without anything being added to or changed in its content.
- [x] **Users are guided to author a design they have not yet written**
  When a design is not yet written down, the system must help the user work it out through adaptive questions toward a stated goal, and produce a design document from that conversation, rather than requiring them to author it unaided.
- [x] **The guided conversation ends when it stops being useful**
  The interview must stop once further questions would not change the resulting design, so an authored design reflects the decisions made rather than transcribing the conversation that made them.
- [x] **An existing design can be revised**
  When a conversation changes a design that already exists, the system must be able to update that document in place, leaving any references to it intact.
- [x] **Design authoring does not require a spec**
  A design can be authored when no spec exists yet, and referenced once one does.
- [x] **Designs Spektacular authors carry metadata**
  A design document the system authors must record its lifecycle status, its provenance, and the specs that reference it. Provenance means when the design was captured and which spec, if any, the conversation that produced it belonged to.
- [x] **Designs Spektacular did not author gain nothing**
  A design document the project already had must never gain a metadata block, and content written through the system must be stored exactly as given, with nothing added, removed, reordered or reformatted.
- [x] **Recorded back-links stay accurate**
  When a reference to a design is recorded or removed, that design's own record of the specs referencing it must be updated to match, so it never reports a reference that no longer exists or omits one that does. A design carrying no metadata block records no back-link, and recording or removing a reference to it must still succeed.
- [x] **Public docs explain design authoring**
  The documentation site must explain that Spektacular can help author a design, what the metadata on an authored design means, and that a design the project already had carries none.

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

- The design skill must be a static playbook, not a fifth interactive workflow. A workflow state machine takes the cross-kind lock and so could not run during a spec workflow, which is precisely when design conversation happens.
- The standing agent instruction must invoke the skill by its name, as the knowledge-capture instruction does, and must not direct the agent to fetch it through the CLI's skill command. Skills nested under the workflow skills directory are not resolvable that way, so such an instruction would fail at the moment it mattered.
- Spektacular must never create or overwrite a design document without the user's explicit agreement. A direct instruction from the user to write one is itself that agreement.
- A design document the project already had must never gain a metadata block, and its content must be stored exactly as supplied. The existing guarantee that nothing Spektacular writes adds frontmatter to a design document is narrowed to documents Spektacular did not author, not removed.
- Because only authored designs carry metadata, recording or removing a reference to a pre-existing design must succeed and leave that document untouched, rather than failing or writing a back-link into it.
- A reference operation that cannot complete its back-link write must fail as a whole and leave the spec exactly as it was, so a spec and a design are never left disagreeing. Where that cannot be achieved, the failure must say so and name what to repair.
- The lifecycle status on an authored design must use the project's existing document status vocabulary rather than introducing a second one.
- Design sources remain a project-level declaration only. A registered repository still declares none of its own.
- Agents must reach design documents through the CLI rather than by reading files directly, as they already do for specs, plans, changelog records and knowledge.
- No change to how specs, plans, changelog records or knowledge entries are stored or addressed.

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

- [x] **A qualifying design in conversation produces an offer**
  Given a conversation in which the user works out an API shape, a flow or a data format, the agent offers to capture or hand off the design once it meets the qualifying bar (settled, worked, and too large to sit inline), and does not offer before that bar is met.
- [x] **A design handed over is stored unchanged**
  Given the user provides a design they have already written, it can be stored in a declared source and referenced from a spec, and reading it back returns content byte-identical to what the user provided.
- [x] **An unwritten design is produced by interview**
  Given the user has a design in mind but nothing written down, a guided conversation produces a design document in a declared source whose content reflects the decisions made in that conversation, without the user having authored the file.
- [x] **A revision updates in place**
  Given a design document referenced by at least one spec, revising it leaves the same path in the same source, with every existing reference unchanged and still resolving.
- [x] **Authoring with no spec succeeds**
  Given no spec exists for the work in hand, a design can still be authored into a declared source, and a reference recorded from a spec created later resolves to it.
- [x] **Authored designs report status, provenance and referencing specs**
  A design document the system authored reports, when read, its lifecycle status, when it was captured, the spec its conversation belonged to if any, and the specs that reference it.
- [x] **Pre-existing designs are stored verbatim and gain no metadata**
  A design document the project already had carries no metadata block after being read, referenced or written through the system, and content written through the system is stored byte-for-byte as supplied.
- [x] **Referencing a pre-existing design succeeds without a back-link**
  Recording a reference to a design the system did not author succeeds and the reference appears on the spec; removing it succeeds too; and the design document is unchanged by either.
- [x] **Back-links match references in both directions**
  After a reference is recorded, the authored design lists that spec; after it is removed, the design no longer lists it. At no point does an authored design list a spec that does not reference it, or omit one that does.
- [x] **A failed back-link write leaves nothing half-done**
  Given the back-link cannot be written, the reference operation fails as a whole and the spec is left exactly as it was, so a spec and a design are never left disagreeing. If the spec cannot be returned to its previous state, the failure says so explicitly and names what to repair.
- [x] **A decline writes nothing**
  When the user declines an offer to author or capture a design, no file is created or modified in any declared source, and the detail is not added to the spec body instead.
- [x] **The docs explain authoring and the metadata**
  The documentation site explains that Spektacular can help author a design, what each metadata field on an authored design means, and that a pre-existing design carries none; and the site builds with no errors and no warnings.

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

- Name the skill following the existing convention for the installed workflow skills, which would make it `spek-design`. The name itself is a choice a planner may adapt; that the instruction invokes it by name rather than fetching it through the CLI is the hard rule, and is a constraint.
- Model the interview on the spec workflow's own interview step, which implements the Flipped Interaction pattern (White et al., arXiv:2302.11382) with a stated goal, adaptive questions rather than a script, and an explicit stopping condition. A design interview wants the same shape aimed at a different goal.
- Model the skill's structure on the existing knowledge skill, the static-playbook precedent: recognise the user's intent, branch, and call the CLI directly. The plausible branches here are author-from-scratch, bring-in-an-existing-design, update-an-existing-design, and reference-only.
- Prefer reusing the existing artifact metadata machinery for authored designs over a parallel schema, since the block already carries created date, lifecycle status and provenance fields. Back-links would be a new field on it. Note the consequence: authored designs would route through the same write path the design commands deliberately avoided, while pre-existing designs keep bypassing it, so the two classes diverge at the write boundary rather than in the store.
- Back-link maintenance belongs with the reference verbs, since they are the only place a reference is created or destroyed.
- The instruction rewrite likely wants noticing separated from qualifying: a short opening that puts the agent on alert during design talk, then the qualifying bar as the gate on offering, then the entry cases for a design that already exists or is being revised.

**Known risks**

- Two classes of design document, one carrying metadata and one not, is a real conceptual cost. If the documentation does not make the distinction obvious, users will reasonably expect their own files to gain frontmatter and be confused when they do not, or vice versa.
- Back-link consistency is a two-document write with no transaction available. The required behaviour on failure is fixed by a constraint, but achieving it reliably, including when the rollback itself fails, is the part most likely to be got wrong.
- The interview risks producing bloated design documents. A stopping condition is required rather than suggested, and it is the mechanism that has to earn that requirement.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Design conversation produces a design rather than being lost: for design conversations that happen after delivery, the agent offers to capture, and no settled design detail ends up existing only in conversation history.
- Users are helped to write designs they would not have written unaided: at least some design documents in declared sources were produced by interview rather than handed over already complete.
- Bringing in an existing design costs no rework: a user who already has a design gets it stored and referenced without editing or reformatting it themselves.
- Back-links can be trusted: reviewing authored designs, every spec a design lists does reference it, and no spec referencing it is missing from the list.
- The two classes of design document do not confuse users: nobody reports being surprised that their own design files did not gain metadata, or that an authored design carries it.
- Authored designs stay proportionate: designs produced by interview are as long as the design warrants, and none reads as an exhaustive transcript of the conversation that produced it.

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

- A fifth interactive workflow for designs. See Constraints for why a state machine is excluded.
- Imposing a body structure on a design document. Authored designs gain a metadata block, but the document's shape follows the design itself; no fixed template or required headings.
- Adding metadata to design documents the project already had. No backfill, and no stamping on write.
- Deriving back-links on demand instead of storing them. Considered and rejected in favour of maintained back-links; no command scans specs to report which reference a given design.
- Deleting design documents. The requirements cover authoring, updating and referencing only.
- Design references on plans or changelog records. A spec references a design; other artifact classes still do not.
- Storage backends other than local files. Git checkouts, remote URLs and issue trackers remain later work.
- Design sources declared by a registered repository. See Constraints.
- Keeping a shipped design in step with the code. A design can be marked superseded through its lifecycle status, but nothing detects drift, checksums a design against the implementation, or requires the two to match.
- Indexing design content in knowledge search. Designs still get no ranking, tags or categories.
