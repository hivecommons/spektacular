# Working Context

## Active work: implement 000057_git-commit

Started implement workflow for plan `000057_git-commit` (auto_commit setting +
git commit engine + workflow wiring + docs).

### Repo roots

- `spektacular` — `/home/nicj/code/github.com/jumppad-labs/spektacular`
- `docs` — `/home/nicj/code/github.com/jumppad-labs/spektacular-website`

### read_plan gate results

- Structural validation: passed. All 10 `##` sections present; phases 1.1, 1.2,
  1.3, 2.1, 2.2, 3.1, 3.2 each carry a `*Technical detail:*` link resolving to a
  matching `### Phase N.M:` heading in `context.md`.
- Drift check: **no mismatches**. Everything the plan names still exists.
  Confirmed in particular:
  - `internal/config/config.go` already imports `internal/output`, so the new
    `auto_commit` validation can use `output.NewError(...)`. Note the existing
    `spec_trigger_threshold` switch (`config.go:553-557`) still returns a plain
    `fmt.Errorf`; the plan only asks for the *new* key's error to be remediating.
  - Commit-point step transitions all verified against the real step tables:
    spec `verification`→`finished` (`internal/steps/spec/steps.go:36-37`),
    plan `walkthrough`→`finished` (`internal/steps/plan/steps.go:51-52`),
    implement `reconcile_spec`→`finished` and `update_changelog`→`analyze`|`test_plan`
    (`internal/steps/implement/steps.go:26,31-35`).
  - `templates/scaffold/plan.md` milestone/phase structure is at :118-124, not
    the :109-131 the plan cites. Content is as described; line numbers only.
  - New paths are correctly absent: `internal/gitexec`, `internal/autocommit`,
    `cmd/autocommit.go`, `templates/partials/git-commit-message.md`.
- Spec coverage: all 19 requirements and 19 acceptance criteria in
  `000057_git-commit.md` map to a phase. No descoped items recorded.
- Changelog mode: **first-phase** invocation (no `## Changelog` section in
  `plan.md`). Starting at Phase 1.1.

### Decisions / answers from the user

- Plan to implement: `000057_git-commit` (chosen from `plan file list`).
- **"Continue, don't ask again"** — drive the remaining phases straight through
  without the per-phase continue-or-pause prompt. Still stop for genuine design
  decisions, plan/reality drift, or a failing verification.
- Agreed to save the Phase 1.1 YAML-boolean discovery to the knowledge base;
  written to `repo/spektacular` at
  `gotchas/yaml-boolean-tokens-as-config-string-values.md`.

### Learnings to carry forward

- `config.Validate()`'s existing `spec_trigger_threshold` switch returns a bare
  `fmt.Errorf`. The new `auto_commit` switch uses `output.NewError(...)`
  `.WithNextAction(...)` instead, per the plan and the repo's error-remediation
  convention. The two now differ in shape on purpose; the older one is out of
  scope for this plan.
- `withProjectSchema()` (the `schema: 3` prefix for config.yaml fixtures) lives
  in `internal/config/repo_test.go:34`, not `config_test.go`.
- `workflow.Config` construction sites needing `AutoCommit`:
  `cmd/spec.go:243,313`, `cmd/plan.go:131,200`, `cmd/implement.go:143,212`.
  The schema/status/steps sites that pass `workflow.Config{}` are inert and stay
  unset, as does `cmd/repo.go:248,313` (the repo workflow has no commit points).
- Error-envelope test style: `errors.As(err, &envelope)` onto
  `*output.ErrorResponse`, then assert `Code`, `Message` and `NextAction`
  substrings (see `internal/config/config_test.go:485-505`).

## Phase log

### Phase 1.1: Add the auto_commit setting — code written

Changes made (spektacular repo):

- `internal/config/config.go`: `AutoCommitOff/Workflow/Full` constants beside
  the `SpecTriggerThreshold*` block; `AutoCommit string \`yaml:"auto_commit"\``
  on `Config` directly after `SpecTriggerThreshold`; `AutoCommit: AutoCommitOff`
  in `NewDefault()`; an `auto_commit` switch in `Validate()` accepting `""` plus
  the three values and otherwise returning a `config_invalid`
  `output.NewError(...).WithResource("auto_commit").WithNextAction(...)`; and a
  new `(c Config) AutoCommitMode()` helper that maps `""` to `off`.
- `internal/workflow/workflow.go`: `AutoCommit string` on `workflow.Config`,
  documented as not persisted.
- `cmd/spec.go:243,313`, `cmd/plan.go:131,200`, `cmd/implement.go:143,212`: each
  `wfCfg` gains `AutoCommit: cfg.AutoCommitMode()`. `cmd/repo.go` left unset.

Notes:

- The field has no `omitempty`, so `ToYAMLFile` now always emits `auto_commit`.
  Existing config/migrate/cmd tests and golden fixtures stayed green, so no
  fixture needed updating.
- No schema bump: `CurrentProjectSchema` stays 3 and no migrate step is added.

Tests added in `internal/config/config_test.go` (5 new + 1 line on
`TestNewDefault_HasExpectedDefaults`), covering criteria 1-4. Full `go test ./...`
green.

**Carried to Phase 1.3: acceptance criterion 5 ("spec, plan and implement
workflows receive the configured mode at run time") has no test yet.**
`workflow.Config.AutoCommit` is written at the six cmd sites but read nowhere in
production code, so it is not observable from any cmd- or step-level test
without a production seam this phase deliberately does not add. Phase 1.3 wires
the mode into `stepkit` rendering; assert criterion 5 there rather than adding a
source-grepping test now.

Criterion 2's second half ("migrate reports nothing to do") is already covered by
`cmd/migrate_test.go` `TestMigrate_DifferentWrittenByIsUpToDate` and
`TestMigrate_SecondRunIsUpToDateAndChangesNothing`; not duplicated.

Verification: `make build`, `make test`, `make lint` (go vet) and
`go test -shuffle=on ./...` all green, no regressions.

Plan updated: Phase 1.1 heading ticked, acceptance criteria 1-4 ticked,
**criterion 5 deliberately left unticked**. The mode IS passed at all six
`wfCfg` sites, but nothing reads it yet, so verification could not assert it.
Phase 1.3 must tick it once `stepkit` renders on the mode.

Changelog entry appended to `plan.md`'s new `## Changelog` section. Phase 1.1
**done**. Next unchecked phase: 1.2 (Build the git commit engine).

### Phase 1.2: Build the git commit engine — code written

- New `internal/gitexec/gitexec.go`: `Run(dir string, stdin io.Reader, args ...string)`,
  lifted from `internal/repo/git.go`. Prepends `-C dir` when dir is non-empty,
  sets `cmd.Stdin` when stdin is non-nil, keeps `GIT_TERMINAL_PROMPT=0`, the
  conditional `GIT_SSH_COMMAND`, and stderr capture from `*exec.ExitError`.
  LookPath message made generic.
- `internal/repo/git.go`: `execGitRunner.run` is now a one-line delegate to
  `gitexec.Run("", nil, args...)`; imports trimmed to `fmt`, `strings`,
  `gitexec`; `GitRunner` doc comment updated to say the shared exec path lives
  in gitexec and the commit side has its own narrow interface.
  **`internal/repo` tests pass unchanged** — the regression guard for the
  extraction.
- New `internal/autocommit`: `git.go` (`Git` interface + `execGit`),
  `targets.go` (`Target`, `Targets`), `commit.go` (`CommitError`,
  `DirtyTargets`, `CommitDirty`), `points.go` (`Point`, `PointFor`,
  `LeadsToCommit`), `message.go` (`ValidateMessage`, `PreWorkflowMessage`).

Decisions worth carrying:

- **Confirmed `repo.Set.LocalSource` never invokes git**, so `Targets` passes a
  nil `GitRunner` to `repo.New`. `Resolve`/`ResolveAll` are never called, which
  is what keeps `Targets` from cloning.
- `milestonePoints` in `points.go` is **deliberately left empty** — the plan
  assigns the milestone rows to Phase 3.1, so 3.1 must populate it and add the
  full-mode branch's coverage. `PointMilestone` and the `LeadsToCommit` full-mode
  branch already exist, so 3.1 is a data change plus `milestones.go`.
- `CommitDirty` returns the targets committed *before* a failure alongside the
  `*CommitError`. Those commits stand; a retry finds them clean. The caller
  reports and halts.
- `ValidateMessage` returns an `*output.ErrorResponse` with the **code only**.
  The cmd layer adds `next_action`, because only it knows the exact goto to
  re-run. Phase 1.3 must do that.
- `execGit.TopLevel` detects "not a git repository" by substring on git's
  stderr to distinguish a non-git directory (skip, nil error) from a real
  failure.
- **Git redirects a hook's stdout to its own stderr.** A pre-commit hook that
  `echo`s without `>&2` still lands in `ExitError.Stderr`, so `gitexec.Run`
  needs no stdout fallback and adding one is dead code. Verified directly
  against git, and pinned by
  `TestIntegration_PreCommitHookOnStdoutIsReported`. A test-step sub-agent
  flagged the opposite as a risk; it was wrong, and a fallback was written and
  then reverted.

Tests added: `internal/testutil/gittest` (new non-test package exporting
`RequireGit`/`RunGit`, extracted from `internal/repo/git_integration_test.go`
per the DRY preference, with that file rewired to it), plus
`internal/autocommit/{targets,commit,points,message,git_integration}_test.go`.
`points_test.go` pins every step name in `completionPoints` against the real
`spec.Steps()`/`plan.Steps()`/`implement.Steps()` lists, so a step rename cannot
orphan a commit point. No import cycle.

Verification all green (gofmt, build, test, vet, `-shuffle=on`, `-short`).
Phase 1.2 **done**, changelog entry appended. Next: Phase 1.3.

**Pending knowledge offer** (not yet accepted): the git hook stdout→stderr
gotcha. Worth a `gotchas` entry in `repo/spektacular` so nobody re-adds a
stdout fallback to `gitexec.Run`.

### Phase 1.3: Commit when a spec, plan or implementation completes — starting

High complexity (~50k). Code written; both carried-forward obligations are now
discharged (`stepkit` reads the mode, and the cmd layer adds `next_action`).

- New `templates/partials/git-commit-message.md`, with a `{{#commit.milestone}}`
  variant. Smoke-rendered both variants before handing to tests.
- `internal/stepkit/stepkit.go`: appends the partial when
  `autocommit.LeadsToCommit(cfg.AutoCommit, cfg.Kind, req.StepName) != PointNone`,
  **before** the working-context footer, so the footer stays last and existing
  footer assertions hold. Vars: `commit{point, milestone, kind, spec_name, tmp_path}`.
- New `cmd/autocommit.go`: `autoCommitGit` swap var, `gotoWithAutoCommit` plus
  `flushBuffer`, `workflowName`, `commitMessagePath`, `readCommitMessage`,
  `commitMessageAction`, `commitRetryAction`, `withCommitNextAction`.
- `cmd/spec.go:314`, `cmd/plan.go:201`, `cmd/implement.go:213`: each goto tail is
  now a single `gotoWithAutoCommit(...)` call.

Decisions worth carrying:

- **Import cycle, and how it was broken.** `stepkit` now imports `autocommit`,
  so `autocommit`'s own test could no longer import the steps packages
  (autocommit-test → steps → stepkit → autocommit). Production code has no
  cycle. Fixed by exporting `autocommit.ReferencedSteps()` and moving the pin
  test to `internal/autocommit/points_pin_test.go` in **external** package
  `autocommit_test`, which may import both sides. Do not move it back.
- `gotoWithAutoCommit` buffers output on **every** path, flushing immediately
  off a commit point. That is byte-identical to writing straight to stdout,
  and it is what lets the commit path discard output on failure.
- The helper takes `missingNameErr string`: plan and implement pass their
  "no active workflow" message, spec passes `""` to skip the check. That
  preserves each handler's prior behaviour.
- `commit_message_from` is deleted from `input` before the SetData loop, so it
  never persists into workflow data. The staged file is removed **before** the
  transition so it is not swept into the commit it describes.
- Rollback restores `state.json` from a snapshot taken after validation and
  before `Goto`. It does **not** undo commits already made to other repos —
  those stand, and the retry finds them clean.

Verified: 402 `cmd` tests, 1154 repo-wide, 0 FAIL, 0 SKIP; gofmt/build/lint/
shuffle/short all green. Independently mutation-checked: neutering `PointFor`
fails all five end-to-end commit tests, so they are load-bearing.

Phase 1.3 **done**. **Milestone 1 is complete** (1.1, 1.2, 1.3 all ticked).
Phase 1.1's deferred criterion 5 is now ticked too.

Remaining: 2.1, 2.2, 3.1, 3.2.

**Still-pending knowledge offer** (never accepted or declined): the git
hook stdout→stderr gotcha from Phase 1.2. Raise once more at a natural break.

### Phase 2.1: Check for uncommitted changes before a workflow starts — code written

- `cmd/resume.go`: `resumeOrClear` split into `probeResume` (read-only) +
  `clearState`, with `resumeOrClear` kept as a thin wrapper.
  **Drift from the plan:** the plan said spec/plan/implement were the only
  callers; `cmd/repo.go:229` is a fourth. The wrapper is kept for it, which is
  the branch the plan anticipated ("keep a thin wrapper only if other callers
  need it").
- `cmd/autocommit.go`: added `startGate`, `commitExistingAnswer`,
  `newCommandWith`, `describeTargets`, `targetNames`.
- `cmd/spec.go`, `cmd/plan.go`, `cmd/implement.go`: each `new` now calls
  `probeResume` → `startGate` → `clearState` → `workflow.New`. The gate runs
  after the name is resolved/validated (and after implement's plan-exists
  check) so its pre-workflow message names the real spec.

Decisions worth carrying:

- **`commit_existing` never reaches workflow data.** None of the three `new`
  handlers copy arbitrary `--data` keys — each unmarshals into a typed struct
  reading only `name` (and `id` for spec). So the plan's "strip it before the
  SetData loop" concern does not apply to `new`; `startGate` reads it straight
  from the raw `--data` string.
- `newCommandWith` rebuilds the re-run command by re-marshalling the caller's
  own `--data` with the answer added. `json.Marshal` sorts map keys, so the
  rendered command is stable and testable.
- **Existing test needed updating, not the code.**
  `TestAutoCommit_ImplementCompletionCommitsEachChangedRepoOnce` seeded plan and
  changelog fixtures after `gitProject`, leaving the tree dirty, so the new gate
  correctly refused to start. Added `commitFixtures`, which **amends** the
  baseline commit rather than adding one, so every hand-written commit-count
  oracle ("1" before, "2" after) stays true.
- Smoke-tested manually: a dirty tree yields `uncommitted_changes` naming the
  repo and both re-run commands; `commit_existing:true` then starts the
  workflow.

Verified: 899 `cmd` test cases, 0 FAIL, 0 SKIP; repo workflow (which still uses
the `resumeOrClear` wrapper) unaffected. Mutation-checked: neutering `startGate`
fails all six criterion-1 subtests plus criteria 2 and 6.
Phase 2.1 **done**. Remaining: 2.2, 3.1, 3.2.

### Phase 2.2: Teach the workflow skills to ask the user — code written

Added an "If the project has uncommitted changes" section to all three
`templates/skills/workflows/spek-{new,plan,implement}/SKILL.md`. Each names the
`uncommitted_changes` code, says to relay the repo names and ask the user, gives
the `commit_existing: true|false` re-run command, explains what each answer does,
forbids deciding for the user, and covers `auto_commit_failed`.

Decisions worth carrying:

- **Placement matters and differs from the plan's sketch.** The gate only fires
  on the *named* `new` invocation: a bare `{{command}} spec new` with no `--data`
  returns `name_required` (or a resume report) before the gate is ever reached.
  So the new section sits under "Starting a new spec" / after the resume block,
  **not** as a third outcome of spek-new's bare in-progress probe. Adding it to
  that probe list would have been wrong.
- All three use `{{command}}`, never a rendered `go run .`, per the standing
  skill-file convention. Verified by grep.
- No banned stdin/heredoc substrings introduced.
- This repo's installed copies under `.claude/skills/` are regenerated by
  `init`/`migrate` and were deliberately not hand-edited. Confirmed nothing in
  the repo compares those checked-in copies against `templates/`, so leaving
  them stale breaks no test.
- **Criterion 1 was taken literally.** It says each skill "gives both restart
  commands". The first draft gave one parameterised command plus bullets
  explaining `true`/`false`. A test sub-agent flagged the gap rather than
  weakening the assertion; the templates now carry two explicit code blocks
  ("To commit the existing changes first" / "To start without committing
  them") and the test asserts both. Mutation-checked.
- Cross-agent identity of skill text was **not** already covered by
  `agent_test.go` / `bob_test.go` / `codex_test.go` / `instruction_surface_test.go`
  (they only assert per-skill substrings per agent).
  `TestUncommittedChangesTextIsIdenticalAcrossAgents` is the only guard against
  per-agent drift.
- Criterion 4 needed nothing new: `instruction_surface_test.go`'s banned-substring
  walk installs all workflow skills into a temp dir and sweeps every `*.md`, so
  it already reaches the new section.

Phase 2.2 **done**. **Milestone 2 complete.** Remaining: 3.1, 3.2.

### Phase 3.1: Commit after each implementation milestone in full mode — code written

- New `internal/autocommit/milestones.go`: `CompletedMilestones(planMarkdown)`
  and `DueMilestones(completed, committed)`.
- `internal/autocommit/points.go`: `milestonePoints` populated with implement
  `update_changelog`→`analyze` and `update_changelog`→`test_plan`.
- `cmd/autocommit.go`: `committedMilestonesKey`, `dueMilestones`,
  `committedMilestones`, `mergeMilestones`; `gotoWithAutoCommit` resolves due
  milestones at a `PointMilestone` and treats an empty set as `PointNone`.

Decisions worth carrying:

- **A milestone point is a candidate, not a commit.** When no milestone just
  completed, the transition falls through to the plain `Goto` path: no message
  is asked for, and a message the agent staged anyway is left on disk
  untouched. That keeps mid-milestone phase wrap-ups silent.
- `committed_milestones` is set in workflow data **before** `Goto`, so the state
  save carries it and the rollback forgets it with the step — a retry after a
  failed commit asks for the same milestones again.
- Workflow data round-trips through JSON, so recorded numbers come back as
  `[]any` of `float64`; `committedMilestones` normalises `float64` and `int`.
- Parser scope: only between `## Milestones & Phases` and the next `## `
  heading. Smoke-verified that a `#### - [ ] Phase 9.9` decoy under
  `## Out of Scope` is ignored, a milestone with no phases is never complete,
  and `- [X]` counts as ticked.
- `dueMilestones` reads `plan.md` through `store.NewSourceStore(root,"project")`
  with `implement.PlanFilePath`, the same helper the implement steps use.

Phase 3.1 **done** (mutation-checked: neutering `CompletedMilestones` fails 4
end-to-end tests). Correction to a claim I made to a sub-agent: `points_test.go`
had **no** `update_changelog` rows before 3.1, so nothing was stale; the agent
checked rather than obeying, and only added rows.

### Phase 3.2: Document the auto_commit setting — done

- `docs: src/pages/configuration.mdx` — frontmatter description, example config
  line `auto_commit: "off"  # off | workflow | full`, key list "Thirteen" →
  "Fourteen" plus the key, and a new `<ConfigKey name="auto_commit">` entry
  placed between `spec_trigger_threshold` and `debug`.
- `spektacular: README.md` — `auto_commit: "off"` in the config example.
- `spektacular: cmd/docs_test.go` — `TestREADMEDocumentsAutoCommitSetting`,
  mutation-checked.

Verified: spektacular gofmt/build/test/lint/shuffle/short all green; docs
`make build` passes and `make check` reports 0 errors, 0 warnings (1 pre-existing
hint in `Shell.astro`, unrelated); MDX layout-markup guard 0 matches; no em
dashes in the new text.

**ALL 7 PHASES COMPLETE. All acceptance criteria ticked. Next: test_plan, then
update_feature_changelog, reconcile_spec, finished.**

### Wrap-up artifacts written

- `plan file write 000057_git-commit/test-plan.md` — three manual procedures:
  a real-agent full cycle in `workflow` and `full`, the agent actually relaying
  the uncommitted-changes question, and the open question about interactive
  commit-signing pinentry. The other two success metrics are fully automated.
- `changelog file write 000057_git-commit.md` — project-level record.
- `changelog file write 000057_git-commit.md --repo spektacular` and
  `--repo docs` — one repo-level record each. Both repos are affected.
- `spec file write 000057_git-commit.md` — reconciled: all 19 requirements and
  all 19 acceptance criteria ticked, 0 remaining. Nothing was descoped.
