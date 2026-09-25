# Working context — 000059_normalise-artifact-addressing

Source: GitHub issue hivecommons/spektacular#46 ("Normalise artifact addressing across spec,
plan and implement, and make output paths config-relative"). Motivated by Hive integration
(hivecommons/hive#8227): Hive wants `data.name` from `state.json` as the run key tying spec,
plan and implementation together, but that bare name cannot address any phase's artifact today.

## Verified current behaviour (live, 2026-09-25, dev build)
- `spec file read 000057_git-commit` → not_found; `...md` works. `spec file list` names carry `.md`.
- `plan file read 000057_git-commit` → internal_error "read /abs/path: is a directory" (leaks host path).
- `plan file read 000057_git-commit plan` → internal_error "accepts 1 arg(s), received 2".
- `plan file read 000057_git-commit/plan.md` works. `plan file list <feature>` names carry `.md`.
- `changelog file read 000057_git-commit` → not_found; names carry `.md`.
- Paths: `knowledge list` and `changelog file list --repo X` are config-relative;
  `spec/plan/changelog file list` (no --repo) are project-root relative (`.spektacular/...`);
  `implement status` `plan_path` is absolute. No `plan_document` field.
- `plan status <bare name>` already accepts the bare name (status verbs already follow the rule).
- File verbs all `cobra.ExactArgs(1)` in `cmd/storefile.go`.

## Issue's proposed rules
1. A name never carries a file extension (input or output); extension → `unexpected_extension` refusal naming correct form. No `.md` synonym.
2. `name` is the address, `path` is the storage location.
3. The name `list` prints is the name `read` accepts.
4. Single-doc stores (spec, changelog) read by feature; plan takes `<feature> <document>`;
   `plan file read <feature>` with no doc → `document_required` refusal pointing at `plan file list <feature>`.
- Paths always relative to the config file that declares the store; uniform across spec/plan/changelog, with and without --repo; `plan_path` config-relative in plan/implement status; add `plan_document`.
- Breaking change; all in-tree callers (skills, step templates, docs) move with it; migration note for external callers; document convention in configuration reference.

## Open questions from issue (answered below)
1. Should `implement` gain a `file` alias for the changelog record?
2. Does `design` need the same treatment?

## Decisions / user answers
- User asked to create the spec from issue #46 (2026-09-25).
- Q1: no `implement file` alias; use `changelog file`.
- Q2: `design` out of scope (non-goal).
- Rollout: hard break, old spellings refused with refusal naming new form; migration note.
- Docs site repo (`docs` = spektacular-website) in scope: command ref, config ref, migration note.
- Design doc offer for issue's command formats: declined. User: "I am not 100% wed to the solution, the main thing is consistency." → issue's command shapes and no-synonym rule are Technical Approach direction, not constraints. Hard break remains a constraint (explicit user choice).
- Spec approved by user ("This is great, commit") and written to the store 2026-09-25. Dev-version fix was committed separately by the user (1dfab1f).
