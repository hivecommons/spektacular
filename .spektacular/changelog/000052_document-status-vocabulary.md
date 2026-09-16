---
created_date: "2026-09-16"
document_status: final
closed_date: "2026-09-16"
---

# Changelog: 000052_document-status-vocabulary

## What was built

Every spec, plan, test plan and changelog entry now records its lifecycle as **`document_status`**. The field was previously `status`. It takes the values `draft`, `final`, `superseded` and `archived`, replacing `in-progress` and `completed`.

**spektacular (the CLI)**
- **Metadata package.** `internal/metadata` now owns a single list of allowed values (`DocumentStatuses`) and a single validator (`ParseDocumentStatus`).
- **Lenient reading.** A stored artifact whose status is missing, retired, unrecognised or not a string reads as a blank status and never as an error. A legacy `status:` key is ignored and dropped the next time the artifact is written. A blank status is written back as `document_status: ""`, is treated as open, and keeps any existing closed date.
- **Lifecycle rules.** New artifacts start as `draft`. Finished spec, plan and implement workflows close their artifacts as `final`. A closed date is stamped only when an artifact is first closed, and cleared on a return to `draft`.
- **Renamed CLI.**
  - `<kind> file write`, `<kind> file list` and `artifacts list` take `--document-status`.
  - `set-status` became `set-document-status`.
  - List and update JSON report `document_status`.
- **Strict input.** Command-line values outside the four (including `completed` and `in-progress`) are rejected with `invalid_document_status` and a remediation listing the valid values. The artifact is left untouched. A missing flag on `set-document-status` returns `missing_document_status`.
- **Agent wording.** The plan workflow's closing message now says the documents are "marked final".
- **End-to-end suite.** The manual harbor implement-workflow suite now expects `document_status: final` on the changelog. Its seeded fixtures carry `document_status: draft`.

**docs (the website)**
- The projects page frontmatter example uses `document_status: draft`.
- The how-it-works lifecycle list describes "Document status" with the four values.
- The configuration page's spec, plan and changelog prose refers to "a document status".

## Why it matters

"In-progress" and "completed" read as statements about the *work*, not the *document*. Planning agents given a spec marked "completed" concluded the feature had already been built and stopped to question the task. Document vocabulary removes that misreading: a `final` spec is a signed-off document, not finished work. Because reading is lenient, artifacts written before this change stay readable (with a blank status) without any migration.

## Deviations from the plan

- **Unknown-subcommand guard.** The `<kind> file` command group was missing the project's unknown-subcommand guard, so a bare `file set-status <path>` printed help and exited 0. The guard was added so the retired command is reported as unknown.
- **Earlier validator use.** The shared flag validator was introduced in Phase 1.1 rather than 1.2. It lives in `cmd/artifactfilter.go`.
- **Extra wording and fixtures.** The harbor suite's seeded `plan.md` and its Dockerfile comment were also updated. The website's how-it-works summary paragraph was also reworded.
- **Manual harbor run pending.** `make harbor-test-implement` was not run during implementation. It is listed in the test plan, and its acceptance criterion remains unchecked.
- **Legacy frontmatter on this plan.** This plan's own documents lost their legacy `status` key when rewritten through the CLI, and now read as blank. Existing artifacts in this repo are to be rewritten separately.
