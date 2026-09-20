# Working context: implement 000053_config-schema-versioning-and-migrations

## Session decisions

- The user chose plan 000053 to implement.
- read_plan passed:
  - Structure is complete (10 sections; 9 phases, each linking to context.md).
  - No drift: every symbol and file the plan names exists.
  - Spec coverage is complete.
  - Changelog mode is first-phase (plan.md has no `## Changelog` yet).
- The plan's dependency, the knowledge-location fix in init, was committed on
  main before starting, at the user's request (e0ef95d).

## Repos

- spektacular: /home/nicj/code/github.com/jumppad-labs/spektacular (the code for Phases 1.1–3.2).
- docs: /home/nicj/code/github.com/jumppad-labs/spektacular-website (Phases 4.1–4.2). It has only a repo.yaml, no config.yaml.

## Progress

- The user said "continue, don't ask again": loop through the phases without
  pausing. Stop only for real design decisions, failures, and Phase 3.2
  (which migrates real settings).
- Phase 1.1 (version fields and stamping): DONE; verified, plan ticked, changelog written.
  - The schema constants, WriterVersion, NormaliseSchema and PeekSchema live in
    internal/config/schema.go.
  - The test helper `pinWriterVersion(t, v)` is in internal/config/repo_test.go.
  - Six existing repo_test oracles now expect `schema: 2` / `written_by`. They
    asserted the writer's own output, not user files, so open question 1 did
    not trigger a STOP.

- Phase 1.2 (upgrade engine): DONE; verified, ticked, logged. Code is in internal/migrate (node, registry, steps_project,
  steps_repo, scan, errors, engine). A smoke test passed.
  - Design choices beyond the plan:
    - Inspect = Apply with DryRun and Skills true. A missing agent errors only on
      a real apply, as ErrNoAgent inside a StepError.
    - The engine tracks the files steps create (u.created), so a dry-run does not
      report a freshly created repo.yaml as skipped. That keeps the preview equal
      to the apply.
    - SkillsReport.Installed/Status describe the state *before* a reinstall, so
      the preview and apply reports match.
    - The skills stamp rewrites config.yaml without taking a backup.
    - `registry` and `current` are package vars, so tests can swap in a
      synthetic step.
  - Added config.ProjectConfigFileName, plus config.FormatError/IsFormatError in schema.go.

- Phase 1.3: DONE (verified, ticked, logged).
- Phase 2.1: DONE (verified, ticked, logged).
  - cmd/migrate.go holds migrateCmd, installerFor (installer output is discarded
    because stderr must stay empty), migrateError, newerFormatError, and the
    gateAnnotation/gateExempt constants.
  - noProjectError was factored out of loadConfig.
  - version.go now sits on migrate.Inspect; the dead helpers are deleted.
  - init runs migrate.Apply (with Skills false) before project.Init and stamps
    SkillsVersion after the install.
  - The preamble now lives in templates/partials/version-check.md and is
    included from all 5 SKILL.md files.
  - Added migrate.PeekCommand.
  - A smoke test of init, legacy migrate and version check passed.

- Phase 3.2 baseline was captured before the gate landed, into
  .spektacular/tmp/baseline-000053/ (spec, plan, changelog, changelog-docs and
  artifacts JSON). The build used was post-1.3 and pre-gate, with the old
  folder resolution. Delete the folder after Phase 3.2.

- From Phase 2.1 until this repo is migrated in Phase 3.2, drive the workflow
  with the prebuilt binary
  /tmp/claude-1000/-home-nicj-code-github-com-jumppad-labs-spektacular/ce4d88d6-6803-4469-9bdc-02e0ec66474d/scratchpad/spek-driver
  (built post-1.3, before loaders refuse old formats), not `go run .`. The
  repo's own config.yaml is unversioned, so the new build refuses it. After
  3.2 migrates this repo, switch back to `go run .`: the old driver would
  misread the new settings-relative dirs. If the scratchpad is gone, build a
  driver from commit/stash state at the end of Phase 1.3.
- Phase 2.1 decision: EnsureFootprint *upgrades* an outdated repo.yaml via the
  engine (migrate.UpgradeRepoFile), because the spec says unregistered or absent
  repos upgrade when next set up. It refuses a newer-format file and never
  overwrites it.
- Phase 2.1 deviation: the loader refusal breaks every cmd/internal test fixture
  that writes an unversioned config.yaml or repo.yaml. So the fixture sweep
  planned for 2.2 starts in 2.1: shared helpers such as writeSpecCommandConfig
  now write current-format files.

- Phase 2.1 fix: repo.Register now refuses a newer-format target repo.yaml
  (PeekSchema) *before* writing config.yaml. Otherwise the refused repo was
  left in the registry, and the 2.2 gate would then block the whole project.
  TestRegister_NewerFormatRepoYAMLIsRefusedAndUntouched asserts no config.yaml
  is written.
- Known asymmetry: project.Init refuses an unversioned colocated repo.yaml
  (it loads it before EnsureFootprint). That is fine, because cmd init runs
  migrate.Apply first.

- Phase 2.2: DONE (verified, ticked, logged). Gate (cmd/gate.go; wired in root.go init()).
  - Decision: the gate lets a command through when Inspect fails for any
    reason other than a format refusal, so a broken config.yaml still reaches
    the command's own error.
  - Engine change: a registered repo.yaml that cannot be read is skipped as
    "unreadable settings" (unreadableError) rather than failing the whole run.
  - Pending knowledge offer: "cmd sites mapping *repo.FootprintError must
    check formatRefusal first". Offer it at the next natural stop (before 3.2).

- Phase 3.1: DONE (verified, ticked, logged).
  - Store dirs resolve from the settings folder (resolveStoreDirs), with
    fileForm write-back in ToYAMLFile. The escape check runs in project-level
    Config.Validate only, because ChangelogConfig is shared with repo.yaml.
  - Added project2to3 and set CurrentProjectSchema = 3.
  - The smoke test passed.
  - The change made 93 tests fail; the test step fixes them.

- Phase 3.2: DONE. This repo is migrated (schema 3), listings verified identical
  to the baseline, backups deleted, README updated. Use `go run .` from now on;
  the spek-driver binary is no longer needed.
- Phase 4 warning: the docs repo has UNCOMMITTED user edits in
  configuration.mdx, how-it-works.mdx, projects.mdx and knowledge-base.mdx,
  plus untracked changelog entries. Edit on top of them; never revert them.

- All 9 phases are done, verified, ticked and logged. Phase 4.1/4.2 (docs) are
  complete; the test plan is written. Remaining: the feature changelog, then
  the user commits.
- Harbor (`make harbor-test-implement`) was NOT run: it needs Docker and live
  agent credentials. It is recorded in the test plan for the maintainer.

- Feature changelog written (project + spektacular + docs records), spec
  reconciled (all 22 requirements and 25 acceptance criteria checked).

## Learnings

- `go run . plan file read` prints raw markdown, not JSON.
- Makefile VERSION is 0.15.1, so the docs examples use 0.16.0.
- Phase 3.2 changes real project data (migrates this repo's own settings), so ask
  the user for a go-ahead before running `migrate` there. Capture the listings
  from a base-commit build (e0ef95d) before the gate lands.
