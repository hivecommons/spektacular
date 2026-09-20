---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Feature: 000054_project-level-design-documents

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Spektacular gains design documents as a first-class project artifact, alongside specs, plans and changelog records. A design document holds the worked design a feature must be built to, such as a settled API or UX shape and its examples, stored wherever the team already keeps it and addressed through Spektacular, so a spec can point at a design instead of absorbing it. Teams get a spec that stays readable while still binding implementation to the design that was agreed, and design detail that surfaces while writing a spec is captured rather than lost.

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

- [x] **Design documents are a project artifact**
  The system must provide design documents as a project-level artifact class alongside specs, plans and changelog records.
- [x] **Design documents are stored where the project keeps them**
  The project can declare one or more named design sources, each naming where its documents live, so designs may live in different places rather than one fixed folder.
- [x] **Local file storage is supported**
  The system must support design documents held as files in a location the project declares.
- [x] **Designs are reachable through the CLI**
  Users and agents can list and read design documents through commands, the same way they reach specs, plans and changelog records, so a reference always has a way to be read.
- [x] **Users and agents can write design documents**
  The system can create and update a design document in a declared source, so a design captured during a conversation is stored rather than pasted into another artifact.
- [x] **A spec can reference design documents without restating them**
  A spec must be able to record references to one or more design documents, so the design binds the work without its detail being copied into the spec.
- [x] **One design document can serve many specs**
  Any spec may reference any design in the project, so a design outlives the feature that introduced it.
- [x] **The spec workflow captures design detail as it arises**
  When a spec conversation settles any of a named set of design-level outcomes — an API shape, a user-facing flow, a data format, or a worked example of either — the system must offer to capture it as a design document and record the reference on the spec.
- [x] **Planning must honour a referenced design**
  The plan workflow must read every design document a spec references and build on it rather than redesigning it.
- [x] **An unresolvable design reference is reported**
  When a referenced design document cannot be found, the system must say so and name what it looked for, rather than continuing as though the spec had no design.
- [x] **Design documents follow the spec's lifecycle**
  A referenced design is binding input while a plan is being built, and a historical record once the work has shipped.
- [x] **Public docs explain the concept**
  The documentation site must explain design documents as a project-level concept in their own right, and document the configuration and commands that support them.

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

- Design documents must be reached through Spektacular's existing storage capability, extending the provider model that specs, plans, changelog records and knowledge already use, rather than introducing a parallel mechanism.
- Design sources are declared by the project only. Registered repos must not declare design sources of their own.
- Spektacular must never create or overwrite a design document without the user's explicit agreement. A user instructing Spektacular to write a design document is itself that agreement; a capture the agent proposes must be accepted by the user before anything is written.
- A design reference must identify both the source it belongs to and the document within that source, and a reference naming a source the project has not declared must be refused rather than silently skipped.
- Spektacular must never require a shipped design document to match the implementation, and must never rewrite a design document to make it match.
- Relative locations in a design source's configuration must resolve from the folder holding `config.yaml`, the same rule every other relative path in that file follows.
- Agents must reach design documents through the CLI rather than by reading files directly, as they already do for specs, plans and changelog records.
- The design must not foreclose non-file storage later, including read-only remote documents such as an issue tracker, even though only local files ship in this feature.
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

- [x] **A project can declare where designs live**
  Given a project with a design source declared in its configuration, listing design documents returns the documents in that source's location.
- [x] **Several sources are addressable at once**
  Given a project declaring two design sources in different locations, listing returns documents from both, each identified by the source it came from, and a document can be read by naming its source and path.
- [x] **Designs can be read and written through the CLI**
  A design document written through the CLI into a declared source can afterwards be read back through the CLI with identical content, and appears in the listing for that source.
- [x] **A spec records design references**
  Given a spec that references a design document, reading that spec shows the reference, and the spec body does not contain the design's content.
- [x] **One design serves several specs**
  Two different specs can reference the same design document, and each shows the reference independently of the other.
- [x] **Design capture during a spec is offered, never automatic**
  When a spec conversation settles an API shape, a user-facing flow, a data format, or a worked example of any of these, the user is offered the chance to capture it as a design document. If the user declines, no design document is written and the detail is not added to the spec body. If the user accepts, the design is written to a declared source and the spec records a reference to it.
- [x] **Planning records the design it read**
  Given a spec referencing a design document, planning that spec produces a plan that names each referenced design document and the source it was read from.
- [x] **A missing design is reported, not ignored**
  Given a spec referencing a design document that no longer exists in any declared source, reading the reference reports the failure, names what was looked for, and tells the user how to correct it. Planning does not proceed as though the spec carried no design.
- [x] **A reference to an undeclared source is refused**
  Attempting to record a reference naming a source the project has not declared fails with an error naming the unknown source, and no reference is recorded.
- [x] **A shipped design is never force-synced**
  After a feature ships, no command requires its design document to match the implementation, and no workflow rewrites the design document.
- [x] **The documentation site explains the concept**
  The documentation site carries a page explaining what design documents are, when to use one instead of putting detail in a spec, and how they relate to specs and plans. The configuration reference documents the design configuration and its commands. The site builds without errors or warnings.

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

- Model design sources on the existing knowledge-sources shape, a list of named sources each naming a provider and a location, because that model already supports several locations in one project.
- Mirror the existing artifact command family (`list`, `read`, `write`) so designs are reached exactly as specs, plans and changelog records already are.
- Model the capture offer on the existing knowledge-capture behaviour: recognise the moment, offer, and write only on explicit agreement.
- Known risk for the plan to resolve: the current storage interface assumes a filesystem root and write access, which a future read-only remote provider would not have. The plan should decide how to leave room for that without building it now.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Specs written for features that have a settled design carry a reference instead of the design's content: reviewing specs written after delivery, none restates a design its reference already points at.
- Design detail raised during spec conversations is retained rather than lost: for every capture offer the user accepts, a design document exists in a declared source and the spec references it.
- Teams adopt design documents without relocating anything: a project can declare a folder of design documents it already had and read them unchanged, with no files moved or reformatted.
- Broken references are caught before implementation: unresolved design references are reported while planning, and none is first discovered during implementation.
- Design documents outlive the features that introduced them: at least some designs accumulate references from more than one spec.

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

- Storage providers other than local files: git checkouts, remote URLs and issue trackers are deliberately later work.
- Design sources declared by registered repos.
- Keeping a shipped design document in step with the code: no drift detection, no checksums, no re-sync.
- Rewriting or reformatting a user's design document, or imposing a required structure on its content.
- Indexing design content in knowledge search.
- Immutable decision records with supersession, in the ADR sense. A design document is not a decision log.
- Any change to how specs, plans, changelog records or knowledge entries are stored.
