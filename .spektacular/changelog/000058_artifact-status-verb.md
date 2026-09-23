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
- `spec` and `plan` frontmatter cross-references when present (they are rarely populated today; the join key across phases is the bare artifact name itself),
- `current_step`, `completed_steps`, and `updated_at` from the matching in-progress `state.json` entry when this artifact is currently being worked; `updated_at` is omitted otherwise, so its absence is the explicit "nothing live" signal,
- `modified_at`, the store's modification time for the artifact, reported through the store's new `Stat` and omitted when a backend cannot supply one,
- `current_step: "finished"` for closed document statuses with no live workflow.

Both verbs take the bare artifact name with no extension, the convention #46 proposes for every verb. `plan status <name>` reports the plan's `plan.md` only.

A missing named artifact exits non-zero with the shared JSON error envelope, code `artifact_not_found`, and a next action pointing at `<kind> file list`.

The store contract gained `Stat(path) (FileInfo, error)` on `Reader` and `ModTime` on `DirEntry`, so no caller needs `os.Stat` on a `Root()`-joined path; the two pre-existing `cmd/implement.go` callers moved onto the store with this change. `spec file list`, `plan file list` and `changelog file list` now carry `modified_at` per entry, so polling many artifacts is one list call rather than one status call each.

## Why it matters

External workflow schedulers can now poll Spektacular's CLI for the state of a specific spec or plan without reading `state.json` or assuming artifacts live on local disk. The named status path reads artifact bytes and timestamps through the same store interface as the existing file commands, so a non-filesystem backend answers the same questions from its own metadata, while the legacy no-argument status output stays compatible for callers that inspect the active workflow.

## Worth knowing afterwards

Artifact metadata stores dates at day precision. The status commands emit those dates as RFC3339 midnight UTC timestamps so every timestamp field has one wire format.

`updated_at` and `modified_at` answer different questions and must not be treated as interchangeable. A `git checkout`, a reformat or a stray `touch` moves `modified_at` without any workflow activity; only `updated_at` means the workflow advanced, and it is absent whenever no workflow holds the artifact.
