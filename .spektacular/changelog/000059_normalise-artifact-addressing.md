---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# 000059_normalise-artifact-addressing: one name addresses every document

## What was built

A feature's bare name, the one every workflow records in `data.name`, is now the address for its spec and its changelog record. A plan document is addressed by that name plus a document name (`plan`, `context`, `research`, `test-plan`) as two arguments. The `spec file`, `plan file` and `changelog file` commands all parse their arguments through a new address layer (`internal/artifact`), which is also the only place that knows documents are stored as `.md` files. Lists print bare names, and every printed name is accepted unchanged by read, write, delete and set-document-status. `plan file list` lists features; `plan file list <feature>` lists that plan's documents.

Old spellings are refused, not tolerated. A name carrying an extension or path separator, or a plan written as one joined path, fails with `unexpected_extension`, and the next action restates the caller's command correctly spelled, flags included. A plan document verb given only a feature fails with `document_required`, which points at `plan file list <feature>` and an example read. A missing document's `not_found` points at the matching list command. No refusal writes anything or carries a host path.

Reported locations are portable. A list's `path`, `set-document-status`'s `path`, the `plan_path` from `plan status` and `implement status`, and the `spec_path`/`plan_path` in workflow `new`/`goto`/status results are relative to the folder holding the config file that declares the store: `specs/<name>.md`, `plans/<name>/plan.md`, or `changelog/<project>/<name>.md` for a repo's changelog. The status and workflow results add `plan_document`. Step instructions no longer receive absolute path template variables; they name each document and the CLI command that reads it, because a store's documents may not be on disk.

Everything shipped moved with the change: the workflow skills, step templates, the implement plan-documents partial, the walkthrough revision hint, the harbor suites, and the regenerated `.claude`/`.bob` skill copies. Template and rendered-corpus guards fail the build if an old spelling or a removed path variable returns. The README and `CHANGELOG.md` document the rules and the migration, and the documentation site gains a Documents reference page, configuration notes and a status sample.

## Why it matters

Each kind of document used to want its own spelling (`<name>.md`, `<name>/plan.md`), list output followed three different conventions, and some commands leaked absolute host paths or raised internal errors. External orchestrators such as Hive (hivecommons/hive#8227) key a run on the single recorded name; they can now reach the spec, the plan and the implementation record from that name with no per-store string handling, and every location they are handed is portable.

## Deviations from the plan

- The manual end-to-end run replaced the harbor suites, as the user decided at the walkthrough; the suites' spellings were updated but not run. The user tested the new commands by hand.
- `artifact.Parse` gained `ParseFeature` and an `ErrEmptyName` sentinel (rendered as `bad_input`); `implement.PlanDocumentPath` was added so the test-plan path also goes through the address.
- `delete` stays idempotent on a missing document, and a missing central list directory keeps its previous error; only `plan file list <feature>` for a missing feature became `not_found`.
- The plan document `not_found` message reads `plan "<f>" has no document named "<doc>"`.
- The config-relative location base is attached in the command's store resolver rather than by changing `storeFileStore`, which other commands share. A store configured outside `.spektacular/` reports a `../`-prefixed path, the directory as written.
- A `description` field was added to the shared JSON schema property type so schemas can describe the relative paths.
- The template guard's `{{plan_name}}/` pattern skips `.spektacular/work/{{plan_name}}/` scratch paths, which are not store addresses.
- The repo workflow keeps its absolute `repo_path`, a code folder rather than a stored document.
- Regenerating skill copies with `init` also rewrote `config.yaml`; that was reverted. Regeneration pulled in unrelated earlier drift, including a missing `.bob` spek-design copy.
- On the docs site, the plan-tasks status sample documents `plan status <name>`, which has no `plan_path`, so an `implement status` sample was added to the single-task section instead.
- Found but out of scope: an unregistered `--repo` name on `changelog file` commands returns `internal_error` with no next action.
