---
created_date: "2026-09-16"
document_status: ""
closed_date: "2026-09-16"
---

# Feature: 000052_document-status-vocabulary

## Overview

Every spec, plan and changelog entry records where its document is in its lifecycle, but two of the four lifecycle words ("in-progress" and "completed") read as statements about the work rather than the document. As a result, agents asked to plan a feature whose spec is marked "completed" conclude the feature is already built and stop to question the task. This change renames the lifecycle to document status with document vocabulary (draft, final, superseded, archived), so agents and people reading an artifact can no longer mistake a finished document for finished work.

## Requirements

- [x] **Document status field**
  The system must record an artifact's lifecycle as its document status, stored under the field name set in Constraints, and must no longer write a `status` field.
- [x] **Document vocabulary**
  The system must accept only the four document status values set in Constraints.
- [x] **New artifacts start as draft**
  When an artifact is written for the first time without an explicit document status, the system must record it as `draft`.
- [x] **Workflows close as final**
  When a spec, plan or implement workflow finishes successfully, the system must mark every artifact that workflow closes as `final`. This includes the spec, the plan documents, the test plan and the changelog entry.
- [x] **Closed date is stamped on closing**
  An artifact is closed when its document status is `final`, `superseded` or `archived`. The closed date is the day an artifact was first closed. When an artifact becomes closed and has no closed date, the system must record today's date. When it becomes closed and already has a closed date, the system must keep that date. When it is set back to `draft`, the system must clear the closed date.
- [x] **Lenient reading**
  The system must read an artifact without error when its document status is missing, unrecognised, or one of the retired values (`in-progress`, `completed`), and must treat that document status as blank.
- [x] **Old field is ignored**
  The system must not interpret a legacy `status` field in any way. The next write of that artifact must not carry the field forward.
- [x] **Blank status is preserved**
  A write that does not specify a document status must leave a blank document status blank rather than replacing it with `draft`.
- [x] **Blank status counts as open**
  The system must treat a blank document status as not closed. Listing output must show a blank document status as an empty value, never as `draft`.
- [x] **Closed date survives a blank status**
  The system must keep an artifact's existing closed date when its document status reads as blank.
- [x] **Strict CLI input**
  If a user supplies a document status on the command line that is not one of the four values (including the retired values), the command must fail with an error and must not change any artifact.
- [x] **Renamed CLI surface**
  Users can set and filter by document status with a `--document-status` flag on the `file write`, `file list` and `artifacts list` commands. They can change the status of an existing artifact with a `set-document-status` command. The old `--status` flag and `set-status` command must no longer exist.
- [x] **Renamed list output**
  Listing commands must report each artifact's document status under the key `document_status`.
- [x] **Empty filter means no filter**
  If a user passes an empty document status filter, the listing must return unfiltered results.
- [x] **Agent-facing guidance uses the new vocabulary**
  Every agent-facing workflow instruction and skill that names the lifecycle field or its values must use `document_status` and the new values.
- [x] **Documentation uses the new vocabulary**
  This repository's documentation and the website's example artifact frontmatter must show `document_status` with the new values.

## Constraints

- **The lifecycle field must be named `document_status`, and its only values must be `draft`, `final`, `superseded` and `archived`.** The same field and values must apply to specs, plans and changelog entries alike.
- **Retired values must not be translated into new values, and existing artifacts must not be migrated.** Artifacts that still carry retired values stay readable (with a blank status) but are not converted.
- **Reading an artifact must never fail because of its document status.**
- **Agent-facing instructions and skills must be changed in their source templates.** The installed copies are regenerated from those templates whenever the project is re-initialised, so edits made only to the installed copies would be lost.

## Acceptance Criteria

- [x] **New artifact is written as draft**
  After writing a new artifact with no document status flag, its frontmatter contains `document_status: draft` and contains no `status` key.
- [x] **Explicit status is written**
  After writing an artifact with `--document-status final`, its frontmatter contains `document_status: final` and a closed date of the day of the write.
- [x] **Each of the four values is accepted**
  Writing or setting an artifact with each of `draft`, `final`, `superseded` and `archived` succeeds, and the frontmatter afterwards shows that value.
- [x] **Retired and invalid CLI values are rejected**
  Passing `--document-status completed`, `--document-status in-progress` or `--document-status bogus` to `file write`, `set-document-status`, `file list` or `artifacts list` exits with an invalid-status error, and any targeted artifact file is byte-for-byte unchanged.
- [x] **Old flag and command are gone**
  Passing `--status` to `file write`, `file list` or `artifacts list`, or invoking `set-status`, fails as an unknown flag or command.
- [x] **Legacy artifact reads without error**
  An artifact whose frontmatter has `status: completed` (and no `document_status`) appears in list output with an empty `document_status`, and reading it succeeds.
- [x] **Unrecognised value reads as blank**
  An artifact whose frontmatter has `document_status: bogus` appears in list output with an empty `document_status`, and reading it succeeds.
- [x] **Rewrite drops the legacy field and keeps blank**
  Rewriting a legacy `status: completed` artifact with no document status flag produces frontmatter with no `status` key, a blank (not `draft`) document status, and the original created and closed dates unchanged.
- [x] **Blank artifact is filtered as neither draft nor final**
  A blank-status artifact is excluded from list results filtered by `--document-status draft` and by `--document-status final`, and included when no filter is given.
- [x] **Blank to final keeps the original closed date**
  Setting a legacy artifact that already has a closed date to `final` leaves that closed date unchanged; setting a blank artifact with no closed date to `final` stamps today's date.
- [x] **List output key**
  JSON output of file list and artifacts list contains a `document_status` key per artifact and no `status` key.
- [ ] **Workflows close as final**
  After completing a spec, plan and implement workflow end to end, the spec, plan documents, test plan and changelog entry each have `document_status: final` in their frontmatter.
- [x] **Agent guidance and docs are updated**
  A search of the installed workflow instructions, skills, this repository's documentation (excluding existing specs, plans and changelog entries) and the website's pages finds no reference to the lifecycle as `status` or to the values `in-progress` or `completed` as lifecycle values, and the website's frontmatter example shows `document_status`.

## Technical Approach

- Treat this as a rename of the existing lifecycle, not a new mechanism.
- Prefer a single source of truth for the four allowed values, shared by artifact reading and command-line validation, so the vocabulary cannot drift between them.

## Success Metrics

- Planning agents run against a finished spec no longer stop to question whether the feature has already been built.
- No artifact read fails because of its document status value after the change ships, including artifacts that still carry the retired field.

## Non-Goals

- Updating this repository's existing specs, plans and changelog entries to the new field; the user will do that separately.
- Filtering artifacts by a blank document status.
- Adding a dedicated artifact-metadata reference section to the website; only the existing frontmatter example is updated.
- Changing the created-date or closed-date fields, or the lifecycle rules beyond the rename.

