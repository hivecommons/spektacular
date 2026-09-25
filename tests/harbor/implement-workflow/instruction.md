# Finish an Implementation with Spektacular

You are testing the `spektacular` CLI tool by driving the tail end of the
implement workflow — the changelog and reconciliation steps — for a
multi-repo project. The binary is already installed at
`/usr/local/bin/spektacular`, and the project has been pre-seeded at
`/app` with a fully-implemented plan whose per-phase implementation
audit log is already recorded in the plan's `## Changelog` section.

The project is called `auth-service` and has two member repositories
registered in `.spektacular/config.yaml`:

- `auth` — colocated at `.` (the project's own tree, `/app`)
- `docs` — kept separately: its Spektacular files (`repo.yaml`, knowledge,
  changelog) live under the project at `/app/repos/docs/.spektacular/`,
  and its code lives at `/opt/docs-repo`, declared by `source` in its
  `repo.yaml`. Nothing Spektacular writes may land in `/opt/docs-repo`.

The plan `20260101000000-jwt-auth` implemented a JWT authentication
surface: two phases touched the auth repo (paths with no repo prefix
in the plan's `## Changelog` **Files changed** lists), and one phase
touched the docs repo (paths prefixed with `docs: `). All phases are
already checked off, and the workflow state is positioned at
`test_plan` — the next step is `update_feature_changelog`.

## Setup

First, initialize the project so the driving skills are installed. init
preserves the pre-seeded config and does not clobber the plan, spec,
state, or repo footprints:

```bash
spektacular init claude
```

## Task

Drive the workflow from its current position (`test_plan`, completed)
through `update_feature_changelog`, `reconcile_spec`, and `finished` by
using the `/spek:implement` skill that init installed:

```
/spek:implement 20260101000000-jwt-auth
```

Because a workflow is already in progress on disk, the skill's first
CLI call (`implement new`) will return a **resume report** rather than
start a fresh run. **Do not ask the user which to do — resume the
existing workflow directly.** Advance immediately to
`update_feature_changelog` (the next step after the completed
`test_plan`):

```bash
spektacular implement goto --data '{"step":"update_feature_changelog"}'
```

From that point on, follow each rendered instruction exactly. In
particular, at the `update_feature_changelog` step you MUST:

1. Read the spec through `spektacular spec file read 20260101000000-jwt-auth`
   and the plan through `spektacular plan file read 20260101000000-jwt-auth plan` —
   never with the `Read` tool.
2. Identify the affected member repos from the plan's `## Changelog`
   section's Files-changed lists. Both `auth` (colocated, unprefixed
   paths) and `docs` (prefixed with `docs: `) are affected here.
3. Author one **project-level** record covering all changes across both
   repos, stage it at `.spektacular/tmp/changelog_project.md`, and
   commit it with the CLI. This record is a required artifact — the
   finished step will error if it is missing:

   ```bash
   spektacular changelog file write 20260101000000-jwt-auth \
       --from .spektacular/tmp/changelog_project.md
   ```

4. Author one **repo-level** record per affected member (both `auth` and
   `docs`), each covering only that repo's changes, and commit each
   through the CLI with `--repo`:

   ```bash
   spektacular changelog file write 20260101000000-jwt-auth \
       --repo auth --from .spektacular/tmp/changelog_auth.md
   spektacular changelog file write 20260101000000-jwt-auth \
       --repo docs --from .spektacular/tmp/changelog_docs.md
   ```

5. Never write any changelog record with the built-in `Write` or `Edit`
   tools directly — the CLI is the only supported way to commit them.
   Use `Write` only to stage the scratch content at
   `.spektacular/tmp/changelog_*.md` before each `changelog file write`
   invocation, then remove the scratch file.

Advance through `reconcile_spec` and `finished` normally. The
`reconcile_spec` step will report on the spec's Requirements and
Acceptance Criteria — write the reconciliation summary requested but
do not treat missing test evidence as blocking; there is no real
implementation to run against here.

## After completion

Copy the project's `.spektacular` directory, the docs repo's
`.spektacular` directory, and the docs repo's code checkout to
`/logs/artifacts/` so the verifier can inspect all three:

```bash
cp -r /app/.spektacular /logs/artifacts/spektacular
cp -r /app/repos/docs/.spektacular /logs/artifacts/docs-repo-spektacular
cp -r /opt/docs-repo /logs/artifacts/docs-repo-source
```

### Success criteria

- The workflow reaches the `finished` state (`state.json`'s
  `current_step` is `finished` and every step through the FSM appears
  in `completed_steps`).
- The **project-level** changelog record exists at
  `/app/.spektacular/changelog/20260101000000-jwt-auth.md` — flat, no
  `<project>/` subfolder. This is required; the finished step errors
  hard if it is missing.
- The **auth** (colocated) repo-level changelog record exists at
  `/app/.spektacular/changelog/auth-service/20260101000000-jwt-auth.md`
  — under the `auth-service/` project subfolder.
- The **docs** (separate) repo-level changelog record exists at
  `/app/repos/docs/.spektacular/changelog/auth-service/20260101000000-jwt-auth.md`
  — inside the docs repo's own changelog store under the project folder,
  also under the `auth-service/` project subfolder.
- The docs repo's code at `/opt/docs-repo` contains no `.spektacular/`
  directory and no `CHANGELOG.md`; no `CHANGELOG.md` exists under `/app`
  either. Only the three records above are written.
- No changelog record was written by the built-in `Write` or `Edit`
  tools — every commit went through `spektacular changelog file write`.
