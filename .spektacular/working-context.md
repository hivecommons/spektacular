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

---
# Plan workflow — 000059_normalise-artifact-addressing (started 2026-09-25)
- Plan workflow started from spec 000059 (user chose it). Spec context above still applies.
- Discovery done: research.md + assumptions.md in .spektacular/work/000059_normalise-artifact-addressing/.
- Key learnings: one builder `newStoreFileCmd` (cmd/storefile.go:181) serves all 3 stores → put a per-kind address resolver there. Feature names are ^[a-z0-9_-]+$ so any '.' = extension, any '/' = joined path. Central list paths are project-root-relative today; --repo changelog already config(repo.yaml)-relative. No design refs on spec. Docs site has no command ref / error-code list / migration page (new content needed); docs repo on branch f-normalize-commands. Installed skills regenerate via `go run . init claude|bob` (picks up unrelated drift). Harbor suites hold old spellings (hand-maintained oracles).
- Architecture chosen: new `internal/artifact` address package + address-driven `newStoreFileCmd`; refusals built in cmd with next_action = same command correctly spelled; list path relative to config folder (.spektacular/ or repo.yaml folder); workflow-status forms get plan_document + relative plan_path; new docs page src/pages/documents.mdx. Details in work/ assumptions.md.
- Components drafted (components.md).
- Data structures drafted.
- Implementation detail drafted.
- Dependencies drafted.
- Testing approach drafted.
- Milestones: M1 addressing+callers+harbor, M2 locations+status, M3 docs.
- Tasks drafted (10 tasks, ids from plan task-id; harbor run is a human task).
- Open questions: only self-hosting window (dev build refuses old spellings before templates move) — follow next_action.
- Out of scope drafted.
- Assembled and staged plan/context/research templates in .spektacular/tmp/.
- Verification done: removed shell commands from plan.md prose; CLI command names kept as product surface; docs content examples kept per docs convention.
- plan.md written to store.
- context.md written.
- All three docs written; now in walkthrough.
- User: 000058 has been released (plan deps updated).
- User: skip harbor runs, they'll test manually → task 7558648e is now 'Manually test a full run' (human); harbor update task kept (knowledge testing-architecture requires oracles updated).
- User: files may not be on disk; all store reads via CLI → new M2 task 32733cb9 removes absolute path template vars and makes new/goto/status results config-relative + address.
- Knowledge updated (user-approved): working-with-files-from-steps.md + workflow-steps.md now forbid store paths in agent-facing output.
- User signed off plan walkthrough (2026-09-25); advancing to finished.
