---
created_date: "2026-09-23"
document_status: final
closed_date: "2026-09-23"
---

# Per-artifact status for specs and plans

## What was built

`spektacular spec status` and `spektacular plan status` now accept an optional artifact name. With no name they keep reporting the existing single in-progress workflow shape. With a name, they read the artifact through the configured store and return a per-artifact JSON status for external orchestrators:

- the artifact `kind` and `name`,
- `document_status`, `created_at`, and `closed_at` from artifact metadata,
- `spec` and `plan` frontmatter cross-references when present,
- `current_step`, `completed_steps`, and `updated_at` from the matching in-progress `state.json` entry when this artifact is currently being worked,
- otherwise `current_step: "finished"` for closed document statuses and the artifact file modification time as `updated_at` when the file store can report one.

A missing named artifact exits non-zero with the shared JSON error envelope, code `artifact_not_found`, and a next action pointing at `<kind> file list`.

The store list entries already carried `name`, `path`, `created_date`, `document_status`, and `closed_date` when metadata is present, so the list output did not need to change.

## Why it matters

External workflow schedulers can now poll Spektacular's CLI for the state of a specific spec or plan without reading `state.json` or assuming artifacts live on local disk. The named status path reads artifact metadata through the same store interface as the existing file commands, while the legacy no-argument status output stays compatible for callers that inspect the active workflow.

## Worth knowing afterwards

Artifact metadata stores dates at day precision. The status commands emit those dates as RFC3339 midnight UTC timestamps so every timestamp field has one wire format.
