---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# Feature: 000059_normalise-artifact-addressing

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Today a feature's single name — the one every Spektacular workflow already records and accepts — cannot be used to read that feature's spec, plan or implementation record (its changelog entry): each kind of document wants its own differently-shaped spelling, and the locations reported back follow three different conventions. This feature makes that one name the address for every document a feature produces, and reports every location consistently, so that agents and external orchestrators (such as Hive, an external orchestration tool that wants to use the name as the key tying a feature's spec, plan and implementation together) can go from a feature name to any of its documents without knowing how each one happens to be stored.

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

- [x] **One name addresses a feature's spec**
  Every spec document command (list, read, write, delete, set document status) addresses a spec by the feature's bare name — exactly the name the workflow records — and listing specs reports that same bare name.
- [x] **One name addresses a feature's changelog record**
  Every changelog document command addresses a record by the feature's bare name, both for the project-level changelog and for a named repo's changelog, and listing records reports that same bare name.
- [x] **A plan is addressed by feature and document**
  Listing plans reports each feature's bare name; listing one feature's plan reports the bare name of each document it holds (for example `plan`, `context`, `research`, `test-plan`); every plan document command (read, write, delete, set document status) addresses a document by the feature name together with a document name the listing reported, without the caller joining them into a path.
- [x] **Names never carry a file extension**
  No artifact name — feature or document — carries a file extension, in any command's input or output. This covers names only: a reported storage location may still show how the store persists the document.
- [x] **Listed names are exactly the accepted names**
  Every name a list command prints is accepted unchanged by the matching read and write commands; a caller never has to join segments or append anything to build an address.
- [x] **An extension is refused with the correct form**
  Passing a name that carries a file extension, or a plan address written as a single joined path, is refused with a distinct, documented error code whose message and next action name the correct spelling of the same command.
- [x] **Reading a plan without a document is an actionable refusal**
  Reading a plan with a feature name but no document name is refused with a distinct, documented error code whose next action tells the caller how to list that plan's documents and gives an example read; the refusal never reports an internal error and never reveals a host path.
- [x] **Locations are reported relative to the declaring configuration**
  Every storage location reported by the spec, plan and changelog list commands is relative to the configuration file that declares that store, matching how the knowledge commands already report locations.
- [x] **Changelog locations do not depend on how the record was selected**
  The changelog list command reports locations in the same convention whether or not a repo is named.
- [x] **Status reports the plan by address as well as location**
  Plan status and implement status report the plan's bare feature name and its document name as addresses, and its storage location relative to the declaring configuration — never as an absolute host path.
- [x] **Callers shipped with Spektacular use the new addressing**
  Every skill, workflow step instruction and in-repo document that tells an agent how to address a spec, plan or changelog record uses the new form, so no shipped instruction produces a refused command.
- [x] **The convention is documented**
  The configuration reference documents that reported locations are relative to the configuration file that declares the store, and the command reference documents the addressing rules and both new error codes.
- [x] **External callers get a migration note**
  A migration note tells external callers exactly how each old spelling maps to the new one and that the old spellings are now refused.
- [x] **The documentation site is updated**
  The project's documentation site reflects the new addressing, the location convention and the migration note.

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

- The change is a hard break: old spellings (a name with a file extension, or a plan addressed as a single joined path) are refused from the release that ships this, with no deprecation window and no period of accepting both. (User decision.)
- The shipped skills, workflow step instructions and docs must move to the new addressing in the same change, since a hard break would otherwise leave shipped instructions producing refused commands. (Issue #46 scope.)

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

- [x] **Bare feature name reads each phase**
  For a completed feature, reading its spec, its plan's `plan` document and its changelog record each succeeds using only the feature's bare name (plus `plan` as the document for the plan), with no extension or path joining.
- [x] **Spec list names round-trip**
  Every name printed by listing specs, passed unchanged to a spec read, returns that spec's content; no listed name ends in an extension.
- [x] **Plan list names round-trip**
  Listing a feature's plan prints document names with no extension, and each one, passed with the feature name to a plan read, returns that document's content.
- [x] **Changelog list names round-trip, with and without a repo**
  Every name printed by listing changelog records — project-level and for a named repo — passed unchanged to a changelog read with the same repo selection, returns that record; no listed name ends in an extension.
- [x] **Writes, deletes and status changes use the same address**
  Writing, deleting and changing the document status of a spec, a plan document and a changelog record each succeed when given the bare names a list printed, and act on the same document a read with those names returns.
- [x] **Extension is refused, naming the fix**
  Reading, writing, deleting or changing the document status of a spec, plan document or changelog record with a name ending in `.md`, or with a plan addressed as a joined `feature/plan.md`, fails with the documented extension error code, changes nothing on disk, and the next action shows the same command with the correct spelling.
- [x] **Plan read without document is actionable**
  Reading a plan with only a feature name fails with the documented document-required error code (not an internal error), the message and output contain no absolute host path, and the next action names the command that lists that plan's documents plus an example read.
- [x] **List locations are config-relative**
  The locations printed by listing specs, plans, a plan's documents and changelog records start with the store's configured directory and contain neither the project's hidden settings directory prefix nor an absolute path.
- [x] **Changelog location is the same shape with and without a repo**
  The locations printed by listing changelog records with and without a named repo follow the same relative-to-declaring-configuration convention.
- [x] **Status reports addresses and a relative location**
  Plan status and implement status for a feature report its bare plan name, a plan document name of `plan`, and a plan location relative to the declaring configuration; no absolute host path appears in either.
- [x] **Shipped instructions produce no refused command**
  Searching the shipped skills, workflow step instructions and in-repo docs finds no instruction that addresses a spec, plan or changelog record with an extension or a joined path, and a full spec → plan → implement run driven by those instructions completes without any addressing refusal.
- [x] **Docs describe the convention and the migration**
  The configuration reference, the command reference (both new error codes included), the migration note and the documentation site each describe the new addressing and location convention.

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

- **Consistency is the priority.** The command shapes below are the direction proposed in issue #46, not a fixed design; the planner may refine them where a better-fitting shape keeps the same consistency.
- **Suggested command shapes.** Spec and changelog documents take the feature name as their one argument (`spec file read <feature>`, `changelog file read <feature> [--repo <name>]`); plan documents take the feature and document as two arguments (`plan file list <feature>`, `plan file read <feature> <document>`), with the same shape for write, delete and set-document-status.
- **Error codes.** The issue proposes `unexpected_extension` and `document_required` for the two refusals.
- **Separate address from location.** Keep `name` as the address callers pass back and `path` as the reported storage location; the location may legitimately carry an extension and directory segments for the file-backed store, and callers never address by it.
- **Status output.** The issue proposes `implement status` report `plan_name`, a new `plan_document`, and a config-relative `plan_path`; `plan status` follows the same convention.
- **Store-agnostic.** Addresses should mean nothing about how a store happens to persist documents, since a store may be backed by something other than a local directory, where extensions and path separators do not exist.
- **Reference:** GitHub issue hivecommons/spektacular#46 carries worked examples of each command's output and both refusals.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- An external orchestrator (Hive) reaches a feature's spec, plan and implementation record from the single name recorded in the workflow state, with no per-store string handling on its side.
- After release, no bug reports or issues from agents or external callers about a document command refusing a name that a list command printed.
- After release, no reports of an absolute host path or an `internal_error` surfacing from the spec, plan or changelog document commands.

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

- No change to how design documents are addressed or how design sources report their location; design documents are addressed by source and path, and a user-supplied design may be in any format, so its extension is meaningful.
- No `implement` document command aliasing the changelog record; a feature's implementation record stays under the changelog commands.
- No change to the knowledge commands, which already address entries by name and report config-relative locations.
- No change to the workflow commands (`spec new`, `plan new`, `implement new`, the status commands' input), which already take the bare feature name.
