---
created_date: "2026-09-22"
document_status: final
closed_date: "2026-09-22"
---

# Context: 000057_git-commit

## Current State Analysis

- Nothing in Spektacular makes git commits today. The only code that runs git is the clone/head runner in `internal/repo/git.go:22-83`, reached through the narrow `GitRunner` interface, which has fakes in `internal/repo/set_test.go:76-100` and `cmd/repo_test.go:75-117` and real-git tests in `internal/repo/git_integration_test.go:17-64`.
- Project settings are `internal/config/config.go` `Config` (:262-285). Enum keys follow the `spec_trigger_threshold` pattern (:22-26, :277, :292, :553-557). Loading unmarshals over `NewDefault()` (:341-373), so an absent key keeps its default, and there is no strict decoding.
- Workflows are linear looplab/fsm machines (`internal/workflow/workflow.go`). Step callbacks run as `before_<event>`, so an error vetoes the transition. State is saved in `enter_state` (:147-156) and in `commitTerminal` (:287-303). Output is written straight to stdout by the callback through `output.Writer` (`internal/output/writer.go:23-26`).
- `new` handlers: `cmd/spec.go:156-258`, `cmd/plan.go:68-145`, `cmd/implement.go:73-157`. `resumeOrClear` (`cmd/resume.go:144-163`) both returns the resume report and deletes a stale state file. `goto` handlers: `cmd/spec.go:260-332`, `cmd/plan.go:147-223`, `cmd/implement.go:159-235`. They copy every non-`step` `--data` key into persisted workflow data.
- Git-tracked Spektacular files: `.spektacular/state.json`, `.spektacular/working-context.md`, and `.spektacular/tmp/*` (not ignored). Ignored: `*.log`, `*.old`, `repos/`.
- Registered repos come from `repo.New(cfg, root, git)` (`internal/repo/set.go:66`), then `Entries()` (:80) and `LocalSource(name)` (:133). The project itself is always registered (`internal/config/config.go:588-592`).
- `stepkit.WriteStepResult` (`internal/stepkit/stepkit.go:66-115`) renders mustache templates and appends `partials/working-context-footer.md` to every non-terminal step. Only `command` reaches templates from config.
- Implement's loop runs per phase: `analyze → implement → test → verify → update_plan → update_changelog → (analyze | test_plan)`. The branch is agent-decided (`templates/steps/implement/07-update_changelog.md:91-111`), and no Go code knows about milestones.
- Docs: `spektacular-website/src/pages/configuration.mdx` lists 13 top-level keys (:85-92), with the example config at :33-79 and the `spec_trigger_threshold` ConfigKey at :135-145.

## Per-Phase Technical Notes

### Requirement → repo attribution

| Requirement | Repo | Where |
|---|---|---|
| Commit mode setting, off by default | spektacular | `internal/config/config.go` |
| Off makes no commits; commit on workflow completion; no prompt; commits include all; one commit per repo; nothing to commit; failure stops; local only | spektacular | `internal/autocommit/`, `internal/gitexec/`, `cmd/autocommit.go`, cmd goto handlers |
| Commit on milestone completion | spektacular | `internal/autocommit/milestones.go`, `cmd/autocommit.go` |
| Uncommitted changes flagged / committed on request / declining / clean tree / new files count / non-git skipped | spektacular | `cmd/autocommit.go`, cmd new handlers, `cmd/resume.go`, workflow SKILL.md templates |
| Descriptive message | spektacular | `internal/autocommit/message.go`, `templates/partials/git-commit-message.md` |
| Documented setting | docs | `docs:src/pages/configuration.mdx`; plus spektacular `README.md` |

### Phase 1.1: Add the auto_commit setting

- **File changes**:
  - `internal/config/config.go:22-26`: add `AutoCommitOff`, `AutoCommitWorkflow` and `AutoCommitFull` constants beside `SpecTriggerThreshold*`.
  - `internal/config/config.go:277`: add `AutoCommit string \`yaml:"auto_commit"\`` directly after `SpecTriggerThreshold`, so it marshals next to it.
  - `internal/config/config.go:292` (`NewDefault`): `AutoCommit: AutoCommitOff`.
  - `internal/config/config.go:553-557` (`Validate`): add a `switch` accepting `""` and the three values. On anything else, return `output.NewError("config_invalid", "auto_commit must be one of \"off\", \"workflow\", or \"full\"").WithNextAction("set auto_commit in .spektacular/config.yaml to off, workflow or full (or remove the key to use off)")`. Check that `config` may import `output`; `validateRepos` at :590-604 already does. Treat `""` as `off` everywhere through a helper `(c Config) AutoCommitMode() string`.
  - `internal/workflow/workflow.go:17-29`: add `AutoCommit string` to `Config`, with a doc comment saying it is not persisted.
  - Set `AutoCommit: cfg.AutoCommitMode()` at every construction site: `cmd/spec.go:243,313`, `cmd/plan.go:131,200`, `cmd/implement.go:143,212`. Leave `cmd/repo.go:248,313` unset (off); the repo workflow has no commit points.
  - Tests in `internal/config/config_test.go`, modelled on :25-34 and :92-106:
    - the default is `off`;
    - each of the three values loads;
    - an unknown value is refused with a `config_invalid` `ErrorResponse` whose next_action names the key;
    - `ToYAMLFile`/`FromYAMLFile` round-trips `off` as the string (yaml.v3 quotes it). Assert the raw file text contains `auto_commit: "off"` or `auto_commit: off`, and that it re-loads as `"off"`.
  - Check that `cmd/migrate_test.go:227-275` and `internal/migrate` golden fixtures stay green (no schema bump).
- **Complexity**: Low
- **Token estimate**: ~10k
- **Agent strategy**: Single agent, sequential execution.

### Phase 1.2: Build the git commit engine

- **File changes**:
  - New `internal/gitexec/gitexec.go`: `Run(dir string, stdin io.Reader, args ...string) (string, error)`, lifted from `internal/repo/git.go:46-71`.
    - Keep `exec.LookPath("git")`, `GIT_TERMINAL_PROMPT=0`, the conditional `GIT_SSH_COMMAND=ssh -oBatchMode=yes`, and stderr capture from `*exec.ExitError`.
    - Prepend `-C dir` when `dir != ""`, and set `cmd.Stdin` when non-nil.
    - Make the LookPath error message generic: "git is not installed or not on PATH".
  - `internal/repo/git.go`:
    - Rewrite `execGitRunner.run` to call `gitexec.Run("", nil, args...)`. `Clone`, `LocalHead` and `RemoteHead` stay byte-identical in behaviour.
    - Update the doc comment at :17-21 to say the shared exec path now lives in `gitexec`, and that `GitRunner` stays narrow (clone and head queries only).
    - `internal/repo/git_integration_test.go` must stay green unchanged.
  - New package `internal/autocommit`:
    - `git.go`: `Git` interface (`TopLevel`, `Dirty`, `CommitAll`) and `NewGit()` exec implementation.
      - `TopLevel` runs `rev-parse --show-toplevel`. An exit error whose stderr contains "not a git repository" means `ok=false, err=nil`; any other error is returned.
      - `Dirty` runs `status --porcelain` and returns true when the output is non-empty.
      - `CommitAll` runs `add -A`, then `commit -F -` with the message on stdin. No `--no-verify`, no `-c user.*`, no `--no-gpg-sign`.
    - `targets.go`: `Targets(cfg, projectRoot, git)`.
      - Use `repo.New(cfg, projectRoot, nil)`, then `Entries()`, then `LocalSource(name)` (`internal/repo/set.go:66,80,133`). Check that `LocalSource` does not need the GitRunner; pass the real one if it does. Never call `Resolve`/`ResolveAll`, which clone.
      - Skip entries with no local source, and directories that don't exist.
      - Resolve `TopLevel`, dedupe by top level (preserving registry order and collecting names), and skip `ok=false`.
    - `commit.go`: `DirtyTargets`, `CommitDirty` (stop at the first failure and return a `*CommitError{Target, Cause}`), and `CommitError.Error()` formatted as `git commit failed in <names> (<dir>): <cause>`.
    - `points.go`: `PointFor(mode, kind, from, to)`. The table is `{"spec","verification","finished"}`, `{"plan","walkthrough","finished"}` and `{"implement","reconcile_spec","finished"}` → `PointCompletion` when the mode is workflow or full. Milestone rows are added in Phase 3.1. Also add `LeadsToCommit(mode, kind, fromStep) Point` for the renderer.
    - `message.go`: `ValidateMessage(msg, specName, milestones []int)` returns `commit_message_required` when the trimmed message is empty, and `commit_message_invalid` when the spec name (case-sensitive substring) or `Milestone N` (case-insensitive) is missing. Use `output.NewError` codes; the handler adds the next_action with the exact goto.
  - Tests:
    - `internal/autocommit/*_test.go` unit tests with a fake `Git`.
    - `internal/autocommit/git_integration_test.go`, copying `requireGit`/`runGit` from `internal/repo/git_integration_test.go:17-64` (package-private there; duplicate the ~40 lines or extract `internal/testutil/gittest`; prefer extracting, per the DRY preference). Cases:
      - untracked, modified and deleted files are staged;
      - a clean repo is skipped;
      - a `.git/hooks/pre-commit` that runs `exit 1` with output yields a `CommitError` containing that output;
      - a commit made with `GIT_CONFIG_GLOBAL=/dev/null` and env identity uses that identity (proving no override);
      - `git for-each-ref refs/remotes` is unchanged after the commit;
      - a non-git tempdir gives `ok=false`.
- **Complexity**: Medium
- **Token estimate**: ~25k
- **Agent strategy**: 2 parallel agents: (a) gitexec extraction plus the repo runner rewrite and its regression run; (b) the autocommit package and its tests against the gitexec signature above. Integrate by running `go test ./internal/...`.

### Phase 1.3: Commit when a spec, plan or implementation completes

- **File changes**:
  - New `cmd/autocommit.go`, the shared helpers.
    - `var autoCommitGit autocommit.Git = autocommit.NewGit()`, a swappable package var like `repoGit` at `cmd/repo.go:48-50`.
    - `gotoWithAutoCommit(cmd, cfg, root, statePath, kind string, steps []workflow.StepConfig, wfCfg workflow.Config, input map[string]any, stepVal string) error`:
      1. Pop `commit_message_from` from `input` before the SetData loop, so it is never persisted.
      2. Load the current step from the state file (`workflow.New(...).Current()`, or read state).
      3. `p := autocommit.PointFor(mode, kind, current, stepVal)`. If `p == PointNone`, or it is a dry run, or the mode is off: behave exactly as today (build the workflow with `output.New(cmd.OutOrStdout(), globalFields)` and `Goto`).
      4. Otherwise:
         - Read the message file through `os.ReadFile(filepath.Join(root, path))`, refusing paths that escape the root.
         - Missing or empty → `commit_message_required`. Its next_action: "write the git commit message to .spektacular/tmp/git-commit-message.md, then run: `<cmd> <kind> goto --data '{"step":"<step>","commit_message_from":".spektacular/tmp/git-commit-message.md"}'`".
         - `ValidateMessage` against the workflow `name` → `commit_message_invalid` with the same re-run command.
         - Remove the file.
         - Snapshot the state file bytes.
         - Build the workflow with `output.New(&buf, globalFields)`, then `Goto`. If Goto errors, write buf to stdout and return err, preserving today's behaviour.
         - Run `Targets` and `CommitDirty(message)`. On `*CommitError`: `os.WriteFile(statePath, snapshot)`, drop buf, and return `output.NewError("auto_commit_failed", err.Error()).WithResource(names).WithNextAction("fix the cause reported by git above (for example the failing hook), re-stage the message at .spektacular/tmp/git-commit-message.md, then re-run: <same goto command>")`.
         - On success, copy buf to stdout.
      - The spec name for validation is the workflow `name`. For plan and implement, the name is the spec/plan name, which share the spec's name. Check this at `cmd/implement.go` (the plan name equals the spec name).
  - `cmd/spec.go:260-332`, `cmd/plan.go:147-223`, `cmd/implement.go:159-235`: replace the tail (from `wfCfg :=` to `wf.Goto`) with a call to `gotoWithAutoCommit`. Keep `guardKind`, `readInputIntoWorkflow` and the `name` check inside the helper, so the three handlers stay thin.
  - `internal/stepkit/stepkit.go:51,89-111`:
    - Add `const gitCommitPartialPath = "partials/git-commit-message.md"`.
    - After the footer block, when `autocommit.LeadsToCommit(cfg.AutoCommit, cfg.Kind, req.StepName) != PointNone`, render the partial with vars plus `commit: {point, spec_name: instanceName, tmp_path}` and insert it **before** the working-context footer, so the footer stays last and `instruction_contract_test` footer assertions hold.
    - Check the import direction: stepkit → autocommit must not create a cycle (autocommit imports config, repo and output only, never workflow or stepkit).
  - New `templates/partials/git-commit-message.md`, with a mustache comment saying it is appended by stepkit and never included. The content:
    - "**Automatic git commit.** This project has `auto_commit` on, so advancing to `{{next_step}}` makes a git commit."
    - Write a message: a subject naming `{{commit.spec_name}}`, then a body describing what was specified, planned or implemented. For the milestone variant (3.1), it must also name the milestone.
    - Write it to `{{commit.tmp_path}}` with your own file tool, then add `"commit_message_from":"{{commit.tmp_path}}"` to the `goto --data`.
    - Don't ask the user to confirm.
    - If the CLI returns `auto_commit_failed`, tell the user which repo failed and why, and do not proceed.
  - `cmd/instruction_contract_test.go:44,185-199`: make sure the "every template is in stepTemplateTable" walk excludes partials, as the footer partial already is. Add tests:
    - rendering with `AutoCommit: off` is byte-identical to today;
    - with `workflow`, only `verification` (spec), `walkthrough` (plan) and `reconcile_spec` (implement) carry the partial;
    - the partial text contains "git commit".
  - `internal/agent/instruction_surface_test.go`: check that it walks partials too; if not, add the new partial to its scan.
  - A new test pins `PointFor`'s step names against `spec.Steps()`, `plan.Steps()` and `implement.Steps()`, via `workflow.New(...).StepNames()`.
  - End-to-end tests in new `cmd/autocommit_test.go`, using `resetRootCmd`/`runRootCmd` (`cmd/root_test.go:35,70`) and a `gitProject(t)` helper that:
    - writes the config (`writeSpecCommandConfig` at `cmd/spec_test.go:26`) plus `auto_commit`;
    - runs `git init` in the project, and in a second registered repo created under `t.TempDir()` with its own repo.yaml;
    - pins identity through `t.Setenv` for `GIT_AUTHOR_*`, `GIT_COMMITTER_*` and `GIT_CONFIG_GLOBAL=/dev/null`;
    - makes an initial commit.

    Drive `spec new` → gotos → `verification` → `finished` with a staged message. Assert that `git rev-list --count HEAD` grows by exactly one per changed repo, that `git status --porcelain` is empty, and that the message contains the spec name. Walk the plan and implement workflows the same way, seeding the minimal documents their `finished` callbacks check (`internal/steps/plan/steps.go:296-335`, `internal/steps/implement/steps.go:152-181`). The hook-failure case: `pre-commit` exits 1, then assert the error code `auto_commit_failed`, that the state file's current_step is unchanged, that stdout holds exactly one JSON document, and that the retry succeeds after the hook is removed. Also cover `off` and unset with no log change.
- **Complexity**: High
- **Token estimate**: ~50k
- **Agent strategy**: Parallel analysis, sequential integration. Agent A writes `cmd/autocommit.go` and the handler rewiring. Agent B writes the stepkit append, the partial and the contract tests. Then one agent writes the end-to-end tests against the integrated code and runs `go test -shuffle=on ./...`.

### Phase 2.1: Check for uncommitted changes before a workflow starts

- **File changes**:
  - `cmd/resume.go:144-163`: split `resumeOrClear` into `probeResume(statePath, command, kind, force) (handled bool, err error)`, which is read-only and returns the report, and `clearState(statePath)`, which does the `os.Remove`. Keep a thin `resumeOrClear` wrapper only if other callers need it (grep for it; spec, plan and implement are the only callers today).
  - `cmd/autocommit.go`: add `startGate(cmd, cfg, root, kind, specName string, input map[string]any) error`. It returns nil when the mode is off or it is a dry run. Otherwise it runs `Targets` and `DirtyTargets`; if none are dirty it returns nil. Then, depending on `input["commit_existing"]`:
    - absent → `output.NewError("uncommitted_changes", "uncommitted changes in registered repos: <name (dir)>, ...").WithResource("<names>").WithNextAction("ask the user whether to git commit these changes before the <kind> workflow starts; then run: <cmd> <kind> new --data '<original data + \"commit_existing\":true>' to commit them first, or the same with \"commit_existing\":false to continue without committing")`;
    - `true` → `CommitDirty(PreWorkflowMessage(kind, specName))`, returning `auto_commit_failed` on failure with a re-run next_action;
    - `false` → return nil.

    Strip `commit_existing` from the input before any `SetData` loop (`cmd/spec.go:248-251` copies other keys into data).
  - `internal/autocommit/message.go`: `PreWorkflowMessage(kind, spec)` returns the subject "Save uncommitted changes before <kind> workflow for <spec>" and the body "These are the user's changes from before the <kind> workflow for <spec> started. Spektacular committed them separately at the user's request so they are not mixed with the agent's work."
  - Wiring in `cmd/spec.go` (runSpecNew): replace `resumeOrClear` at ~:197 with `probeResume`. After `spec.ResolveIdentifier` (~:225-243), call `startGate(..., resolved.Name, input)`, then `clearState(statePath)`, then build the workflow.
  - Wiring in `cmd/plan.go:107` and `cmd/implement.go:112`: same pattern. Call `startGate` after the name and plan-existence checks (implement: after :137-141) and before `clearState` and `workflow.New`.
  - Tests in `cmd/autocommit_test.go`:
    - a dirty tree plus mode on → `uncommitted_changes`, no spec file, `state.json` unchanged, working-context unchanged;
    - `commit_existing:true` → HEAD~0 is the pre-workflow commit containing only the pre-existing file, before the completion commit, with a message containing the spec name and "before";
    - `false` → the pre-existing file is in the first automatic commit;
    - an untracked-only file triggers the check;
    - a clean tree, off mode, and a non-git second repo → no question;
    - an in-progress workflow → the resume report wins;
    - a failing hook during commit_existing → `auto_commit_failed` and no state written.
- **Complexity**: Medium
- **Token estimate**: ~25k
- **Agent strategy**: Single agent, sequential. The three handlers share one helper, so parallel edits would conflict.

### Phase 2.2: Teach the workflow skills to ask the user

- **File changes**:
  - `templates/skills/workflows/spek-new/SKILL.md:85-86`: add a third outcome bullet: "An uncommitted-changes report (`code: uncommitted_changes`)…". Add a short section after "Resuming an in-progress workflow" (~:112-125) that says to:
    - name the repos from `message`;
    - ask the user whether to git commit them before starting;
    - re-run `{{command}} spec new` with the same `--data` plus `"commit_existing": true` or `false`;
    - never choose for them.
  - `templates/skills/workflows/spek-plan/SKILL.md:78-91` and `templates/skills/workflows/spek-implement/SKILL.md:54-71`: the same section, adapted to `plan new` / `implement new`.
  - Use the `{{command}}` placeholder, never `go run .` (memory feedback: skill files use `{{command}}`).
  - This repo's own installed copies under `.claude/skills/` are regenerated by `init`/`migrate`, not hand-edited.
  - Tests: `internal/agent/instruction_surface_test.go` (banned stdin/heredoc substrings) must pass. Add a skills test asserting that each of the three rendered SKILL.md files mentions `uncommitted_changes` and `commit_existing`, following the existing skills tests in `internal/agent/*_test.go`.
- **Complexity**: Low
- **Token estimate**: ~8k
- **Agent strategy**: Single agent, sequential execution.

### Phase 3.1: Commit after each implementation milestone in full mode

- **File changes**:
  - New `internal/autocommit/milestones.go`: `CompletedMilestones(plan string) []int`.
    - Scan only between `## Milestones & Phases` and the next `## ` heading.
    - A `### Milestone N:` line starts group N.
    - A `#### - [ ] Phase` line counts as open and a `#### - [x] Phase` line (case-insensitive x) as done.
    - Return the Ns with at least one phase and zero open phases, ascending.
    - Structure source: `templates/scaffold/plan.md:109-131`.
  - `internal/autocommit/points.go`: in full mode, add implement `update_changelog` → `analyze` and `update_changelog` → `test_plan` as `PointMilestone` candidates. `LeadsToCommit(full, implement, update_changelog)` returns `PointMilestone`.
  - `cmd/autocommit.go` (`gotoWithAutoCommit`), for a `PointMilestone` candidate:
    - Read the plan through the store: `store.NewSourceStore(root,"project").Read(cfg.Plan.Config.Directory + "/" + name + "/plan.md")`, using the same path helper the implement steps use (grep `internal/steps/implement` for the plan path function).
    - Compute `due = CompletedMilestones(plan) − data["committed_milestones"]`.
    - If `due` is empty, treat the transition as `PointNone`: no message is needed, and any supplied message file is left untouched.
    - Otherwise require and validate the message with `milestones=due`, and set `committed_milestones = existing ∪ due` in workflow data **before** `Goto`, so `enter_state` persists it and a rollback restores it.
    - Workflow data values come back from JSON as `[]any` of float64; normalise them.
  - `templates/partials/git-commit-message.md`: add a `{{#commit.milestone}}` variant. It tells the agent to include `commit_message_from` naming "Milestone N" only if the phase just ticked was the last open phase of its milestone, and that the CLI refuses the goto if a finished milestone has no message.
  - Tests:
    - `internal/autocommit/milestones_test.go`: fixture plans covering partial, complete, empty-milestone, uppercase X and checkboxes outside the section.
    - Points tests for full versus workflow.
    - `cmd/autocommit_test.go` full-mode end-to-end, with a two-milestone plan fixture in the plan store (each milestone having one phase), and implement walked with gotos. After ticking Phase 1.1 (a fixture edit) and at `update_changelog` → `analyze`: a missing message is refused; a message with "Milestone 1" is accepted, commits, and records `[1]`. The Phase 2.1 wrap-up → `test_plan` gives the Milestone 2 commit. `reconcile_spec` → `finished` gives a completion commit only where changes remain. Also assert that workflow mode makes no milestone commits, and that a move inside a milestone asks for no message.
- **Complexity**: Medium
- **Token estimate**: ~25k
- **Agent strategy**: 2 parallel agents: (a) the parser, points and unit tests; (b) the cmd wiring, partial variant and end-to-end tests once (a)'s signatures are fixed.

### Phase 3.2: Document the auto_commit setting

- **File changes**:
  - `docs:src/pages/configuration.mdx`:
    - Frontmatter description (:4): add `auto_commit` to the list of areas.
    - Example yaml (:33-79): add `auto_commit: "off"` after `spec_trigger_threshold: moderate` (:40), with a trailing `# off | workflow | full` comment matching the neighbouring comment style.
    - Key list sub (:85-92): change "Thirteen" to "Fourteen" and insert `auto_commit` after `spec_trigger_threshold`.
    - New `<ConfigKey name="auto_commit" …>` after the `spec_trigger_threshold` entry (:135-145) and before `debug` (:147), per the Content example in plan.md, following MDX Rule 2/3 (slot body, blank lines) and no em dashes.
  - `docs:CHANGELOG.md`: this is written by the implement workflow's feature changelog step, not by hand in this phase.
  - `README.md:164-206` (spektacular): add `auto_commit: "off"   # off | workflow | full: automatic git commits (local only)` near the `agent:` line (:171), matching the block's comment style.
  - Verification in the docs repo:
    - `make build`;
    - `make check` (astro check: 0 errors, 0 warnings);
    - `grep -nE "<div|<section|class=" src/pages/*.mdx` returns 0 matches;
    - `grep -n "—"` on the new lines returns nothing.
- **Complexity**: Low
- **Token estimate**: ~8k
- **Agent strategy**: Single agent, sequential execution, working in `/home/nicj/code/github.com/jumppad-labs/spektacular-website` for the docs changes and the spektacular root for the README.

## Testing Strategy

Per phase:

- **1.1:** config unit tests (default, three values, unknown value with a next_action, `off` round-trip); the migrate and golden tests unchanged.
- **1.2:** autocommit unit tests with a fake `Git` (targets, points, messages); real-git integration tests (untracked, modified and deleted staged; clean skipped; hook failure; identity untouched; no remote-ref change; non-git directory); `internal/repo` git integration tests unchanged as a regression guard for the exec extraction.
- **1.3:** instruction-contract tests (the off render is byte-identical; the partial appears only on commit-leading steps; "git commit" wording; no stdin or heredoc); the commit-point table pinned against the real step lists; cmd end-to-end tests in `workflow` mode across the spec, plan and implement completions, including hook-failure rollback and retry, a single JSON document on failure, a clean tree after completion, and off/unset leaving the logs unchanged.
- **2.1:** cmd end-to-end tests for the start gate (dirty gives the report with no writes; commit_existing true/false; untracked-only; clean, off and non-git silent; resume report precedence; hook failure during the pre-commit).
- **2.2:** skills tests (each of the three SKILL.md files mentions `uncommitted_changes` and `commit_existing`); the instruction-surface test passes.
- **3.1:** milestone parser unit tests; points full versus workflow; a full-mode two-milestone implement end-to-end test (refused without a message; a commit per milestone; `committed_milestones` recorded; a leftovers-only completion commit; no message needed mid-milestone).
- **3.2:** docs `make build` and `make check`, plus the MDX guard grep and a grep for no em dashes.

The success-metric mapping is in plan.md § Testing Approach. The manual items for the implementation test plan are a real-agent spec → plan → implement cycle with `workflow` and with `full`, and the agent relaying the uncommitted-changes question to the user. Every phase ends with a full `go test -shuffle=on ./...`.

## Project References

- Knowledge (spektacular): `architecture/workflow-steps.md`, `architecture/working-with-files-from-steps.md`, `gotchas/fsm-cancel-only-works-before-transition-commits.md`, `gotchas/goto-to-current-step-is-a-silent-noop.md` (a re-goto to the current step re-renders it and never commits), `architecture/testing-architecture.md`, and the conventions listed in plan.md § Conventions.
- Knowledge (docs): `conventions/mdx-authoring.md`, `conventions/no-em-dashes.md`, `conventions/plan-content-pages.md`, `conventions/file-scoped-section-headings.md`.
- Prior plan `000053_config-schema-versioning-and-migrations`: the schema-bump rule and the configuration page layout.
- Repo roots (`go run . repo list`): spektacular `/home/nicj/code/github.com/jumppad-labs/spektacular`, docs `/home/nicj/code/github.com/jumppad-labs/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Phases 1.1, 2.2 and 3.2 are Low. Phases 1.2, 2.1 and 3.1 are Medium. Phase 1.3 is High, because it integrates the engine, the handlers, stepkit and the end-to-end suite.

## Migration Notes

None. `auto_commit` is optional and defaults to `off`, so existing projects load unchanged, need no `migrate`, and behave exactly as before. `CurrentProjectSchema` stays 3 and no upgrade step is registered. Projects re-saved by `init` or `repo add` will gain an explicit `auto_commit: "off"` line, which is harmless. Installed skills change in Phase 2.2, so existing projects see `version check` report a skills mismatch after upgrading Spektacular and run `migrate` to reinstall them, as for any release.

## Performance Considerations

Each commit point adds, per registered repo, one `git rev-parse --show-toplevel` and one `git status --porcelain`, plus `git add -A` and `git commit` for dirty repos. That is milliseconds on typical repos and is paid only at the few commit points and at `new`, never on ordinary steps. `status --porcelain` on very large work trees can take longer; this is accepted, because it only runs when `auto_commit` is on. In `off` mode no git process is started at all.
