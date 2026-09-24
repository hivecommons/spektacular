---
created_date: "2026-09-22"
document_status: final
closed_date: "2026-09-22"
---

# Plan: 000057_git-commit

<!-- Metadata -->
<!-- Created: 2026-09-22T14:10:29Z -->
<!-- Commit: d245644 -->
<!-- Branch: f-migrate -->
<!-- Repository: git@github.com:jumppad-labs/spektacular.git -->

## Overview

Developers using Spektacular currently have to remember to run `git commit` after each spec, plan and implementation, and their own unrelated edits easily get mixed into the agent's work. This plan adds an `auto_commit` project setting (`off` by default, `workflow` or `full`). With it on, Spektacular makes a local git commit in every registered repository that changed:
- when a spec, plan or implementation completes;
- in `full` mode, after each milestone of an implementation as well.

Spektacular runs the commits itself, through the user's own `git`, with their hooks, identity and signing. The coding agent writes each message, which names the spec and the milestone where there is one, and nobody is asked to confirm. Before a workflow starts, the user is told about any uncommitted changes and can commit them separately first. A failed commit stops the workflow where it was. Claude, Bob and Codex users get identical behaviour, and the docs site's configuration reference documents the setting.

## Conventions

- **Error messages must describe the problem and suggest remediation** (spektacular) — every new refusal (`uncommitted_changes`, `commit_message_required`, `commit_message_invalid`, `auto_commit_failed`, the `auto_commit` enum validation) needs an `output.NewError(...).WithNextAction(...)` naming the exact command to re-run. Tests assert the next_action content, not just that it is present.
- **Spektacular's own files are written through Spektacular, never with file tools** (spektacular) — the commit message goes by path (`commit_message_from`) under `.spektacular/tmp/`, never on stdin or in a heredoc. `internal/agent/instruction_surface_test.go` bans those substrings in the new partial and skill text.
- **Tests must not depend on execution order** (spektacular) — the new cmd tests run `rootCmd` through `runRootCmd`/`resetRootCmd`, and real-git fixtures live in `t.TempDir()` with env-pinned identity and null global config.
- **Passing tests are required before calling work done** (spektacular) — the full test suite must be green, including the `instruction_contract_test.go` template table and the config tests.
- **Plans must sketch content structure, not just summarize it** (docs) — the configuration reference change needs a Content example with the exact key, values and default.
- **MDX authoring conventions** (docs) — the new `ConfigKey` uses a slot body, with blank lines around slot content and no layout HTML.
- **Label before filename in file-scoped reference headings** (docs) — the new entry goes inside the existing "Project configuration keys" block, so no new heading is needed; recorded so the implementer doesn't add a filename-first section.
- **No em dashes** (docs) — applies to the new ConfigKey prose and the docs changelog entry.

Deliberately dropped: **alternate section background** and **site layout** (docs), because no new section or band is added; the entry goes inside an existing `ConfigurationKeys` block.

## Architecture & Design Decisions

**Spektacular runs the commits and the agent writes the messages.** A new top-level project setting, `auto_commit: off | workflow | full` (default `off`), is added to `internal/config/config.go` the same way `spec_trigger_threshold` was: constants, a field, a default in `NewDefault()` and a `switch` in `Validate()`. No schema bump or migrate step is needed, because an absent key loads as the default. The work is owned by a new `internal/autocommit` package in the spektacular repo, which has four parts:
- **Commit targets:** registered repos come from `repo.New(...).Entries()`/`LocalSource()`, which never clone. A repo that is not a git work tree is skipped silently, and repos that share a work tree are merged into one target via `git rev-parse --show-toplevel`.
- **Dirty check:** `git status --porcelain`, which also lists untracked files.
- **Whole-tree commit:** `git add -A` then `git commit -F -`. No identity or config overrides and no `--no-verify`, so hooks, identity and signing behave exactly as a manual commit (spec constraints).
- **Commit-point table:** records which transitions are commit points, per workflow kind and mode.

Git runs through the user's `git` binary. The exec helper in `internal/repo/git.go:46-71` is extracted into a small shared `internal/gitexec` package, so `repo.GitRunner` stays the narrow clone-and-head interface its doc comment requires, and the new commit side gets its own equally narrow interface. Because every commit decision and git call lives in Go and is reached through the CLI's own `goto`/`new` commands, behaviour is identical for Claude, Bob and Codex.

**Commits happen at transitions, after state is saved, and are rolled back on failure.** The commit points are:
- **Completion (`workflow` and `full`):** spec `verification`→`finished`, plan `walkthrough`→`finished`, implement `reconcile_spec`→`finished`.
- **Milestone (`full` only):** implement `update_changelog`→`analyze` or `test_plan`, when `plan.md` shows a milestone whose `#### - [x] Phase N.M` checkboxes are now all ticked and which has not been committed yet. Committed milestone numbers are recorded in workflow data.

At a commit point the agent stages its message under `.spektacular/tmp/` and passes `"commit_message_from": "<path>"` in the `goto --data`, following the knowledge rule that bodies go by path, never stdin or heredoc. Before transitioning, the `goto` handler checks three things and refuses with a remediating error if any fails:
- the message file exists;
- it is non-empty;
- it names the spec, plus `Milestone N` for a milestone commit.

It then removes the file, so the message never lands in the commit. The handler snapshots `state.json`, buffers the step's output and runs the transition. Only after that does it commit each dirty target. Committing after the state save matters because `.spektacular/state.json` and `working-context.md` are tracked in git: committing inside a step callback, which runs as `before_<event>` (`internal/workflow/workflow.go:115-133`), would leave `state.json` dirty after every commit. The next workflow would then always see uncommitted changes. If any commit fails, the handler restores the state snapshot, discards the buffered instruction and returns an `auto_commit_failed` error that names the repo and git's stderr. The workflow therefore stays on the step it was on, and re-running the same `goto` retries the commit. Targets that did commit are now clean, so a retry commits only what is left. A clean target at a commit point is skipped without error.

**The start-of-workflow check reuses the resume-report pattern.** `spec new`, `plan new` and `implement new` gain a check that runs after the in-progress (resume) check, which still wins, and before anything touches disk. That includes the stale-`state.json` deletion that `resumeOrClear` does today (`cmd/resume.go:151-157`), so `resumeOrClear` is split into a read-only probe and a clear step. When the mode is not `off` and any target is dirty, `new` returns an `uncommitted_changes` report, shaped like the resume report (`cmd/resume.go:67-91`), that names each repo and changes nothing. The agent asks the user, then re-runs `new` with `"commit_existing": true` or `false`:
- **`true`:** commits each dirty target first, with a Spektacular-written message that names the spec and says these are the user's changes from before the workflow.
- **`false`:** the workflow proceeds, and its later automatic commits sweep in everything.

A clean tree, or mode `off`, starts silently. This keeps "before any workflow work" literally true, because spec `new` writes its scaffold immediately. It also adds no FSM step, so no step table or harbor step-order oracle changes.

**Instructions.** Agents learn about both conversations through CLI-rendered text:
- **Commit message partial:** a `partials/git-commit-message.md` partial is appended by `stepkit.WriteStepResult` next to the working-context footer, but only when `auto_commit` is on and the step leads into a commit point. It tells the agent what the message must name and how to pass it. It is driven by a new `AutoCommit` field on `workflow.Config` and the shared commit-point table, not by per-step template edits.
- **Skills:** the three workflow skills (`spek-new`, `spek-plan`, `spek-implement` SKILL.md) learn to handle the `uncommitted_changes` report the way they already handle the resume report.
- **Docs:** the docs repo's configuration reference (`src/pages/configuration.mdx`) and the spektacular README's config block document the key.

This direction beats the alternatives because only Go-side execution can guarantee that a failed commit stops the workflow, and it can be tested with real git in the Go test suite. The rejected options are:
- the agent running git itself;
- committing inside step callbacks;
- a standalone commit command;
- widening `GitRunner`;
- a schema bump;
- agent-only milestone detection;
- a new FSM step for the start check.

The evidence for each is in research.md#alternatives-considered-and-rejected.

Conventions applied: every new refusal (`uncommitted_changes`, `commit_message_required`, `commit_message_invalid`, `auto_commit_failed`, the enum validation error) is built with `output.NewError(...).WithNextAction(...)` and gives the exact command to run next (**error messages must suggest remediation**). The message is supplied by `--from`-style path, never stdin or heredoc (**store files written through the CLI**). Real-git cmd tests go through `runRootCmd`/`resetRootCmd` and never rely on the wall clock (**tests must not depend on order**). Docs changes follow the MDX, label-before-filename, no-em-dash and content-outline conventions of the docs repo.

## Component Breakdown

- **Auto-commit setting (changed, config package, spektacular repo):** Owns the `auto_commit` key: the `off`, `workflow` and `full` constants, the `off` default in the project defaults, and validation that rejects any other value with a remediating error. Every other component reads the mode from here. It adds no schema version and no upgrade step.

- **Git exec helper (new, extracted from the repo package, spektacular repo):** The single place that locates the user's `git` binary and runs it. It uses non-interactive credential settings, can feed a message on stdin to `commit -F -`, and turns git's stderr into the error text. The repo package's clone/head runner is rewritten on top of it without changing its interface. The new commit-side runner uses it too, so there are still two narrow git interfaces and one exec path.

- **Auto-commit engine (new, `autocommit` package, spektacular repo):** Owns every decision about committing. It depends on the setting, the git exec helper and the repo registry, and knows nothing of cobra or output.
  - **Targets:** resolves registered repos to local git work trees without cloning. It skips non-git and not-yet-cloned repos, and merges repos that share a work tree.
  - **Dirty check:** reports which targets have uncommitted changes, untracked files included.
  - **Commit:** commits every dirty target in full with a given message, skips clean targets, and stops at the first failure with the repo and cause.
  - **Commit-point table:** maps (workflow kind, from step) and mode to "completion", "milestone" or none.
  - **Milestone detection:** reads a plan's `## Milestones & Phases` block and reports which milestones have every phase ticked.
  - **Message validation:** checks that a message is non-empty and names the spec, plus the milestone when required.
  - **Pre-workflow message:** writes the fixed message for the pre-workflow commit of the user's changes.

- **Workflow command handlers (changed, cmd `spec`/`plan`/`implement` new and goto, spektacular repo):** The integration point. The engine decides; the handlers enforce.
  - **`new`:** runs a start check between the read-only resume probe and any disk write, which splits today's resume-or-clear helper into probe and clear. When a registered repo is dirty and the mode is on, it returns an `uncommitted_changes` report, or honours `commit_existing`.
  - **`goto`:** asks the engine whether the requested transition is a commit point. If it is, the handler reads, validates and removes the staged message file before transitioning. It then buffers output, snapshots state, transitions, commits, and on failure restores state and reports `auto_commit_failed`.
  - **Shared helper:** one helper serves all three kinds, so the kinds cannot drift apart.

- **Workflow runtime config (changed, workflow package):** Carries the resolved `auto_commit` mode alongside the command and store folders, so step rendering can see it. It is not persisted, like the rest of that config.

- **Step result renderer (changed, stepkit):** When the mode is on and the step being rendered leads into a commit point (per the engine's table), it appends a commit-message instruction partial after the step body, next to the working-context footer it already appends. This needs no per-step template edits.

- **Commit-message instruction partial (new, templates):** Agent-neutral prose telling the agent how to stage the message and pass it in the next `goto`, and what the message must contain. For a milestone point it also explains the condition: include the message only when the phase just finished closed its milestone. Go refuses the transition if the message is missing when required.

- **Workflow skills (changed, spek-new / spek-plan / spek-implement SKILL.md templates):** Learn to handle the `uncommitted_changes` report as they already handle the resume report: name the repos to the user, ask whether to commit them first, and re-run `new` with the answer. This text is installed identically for every agent.

- **README config reference (changed, spektacular repo):** The commented config block gains the `auto_commit` key.

- **Configuration reference page (changed, docs repo):** The project config example, the key count and list, and a new `auto_commit` ConfigKey entry describing the three values and the `off` default.

## Data Structures & Interfaces

**Setting (config package).** There is one new top-level YAML key, typed as a plain string with three named constants:

```go
const (
    AutoCommitOff      = "off"      // default
    AutoCommitWorkflow = "workflow"
    AutoCommitFull     = "full"
)

type Config struct {
    // ...existing fields...
    SpecTriggerThreshold string `yaml:"spec_trigger_threshold"`
    AutoCommit           string `yaml:"auto_commit"`
    // ...
}
```

```yaml
# .spektacular/config.yaml
spec_trigger_threshold: moderate
auto_commit: "off"      # off | workflow | full
```

**Git exec helper (gitexec package).** This is the shared exec path. Both the existing clone/head runner and the new commit runner sit on top of it.

```go
// Run executes git with args in dir (dir "" = current directory), feeding stdin
// when non-nil, and returns trimmed stdout or an error carrying git's stderr.
func Run(dir string, stdin io.Reader, args ...string) (string, error)
```

**Commit-side git interface (autocommit package).** This is the narrow surface the engine needs. Tests fake it; production wraps `gitexec.Run`.

```go
type Git interface {
    TopLevel(dir string) (top string, ok bool, err error) // ok=false: not a git work tree
    Dirty(top string) (bool, error)                        // status --porcelain non-empty (untracked included)
    CommitAll(top, message string) error                   // add -A, then commit -F - (hooks, identity, signing untouched)
}
```

**Engine types (autocommit package).**

```go
// Target is one git work tree that one or more registered repos resolve to.
type Target struct {
    Repos []string // registered repo names sharing this work tree
    Dir   string   // absolute work-tree top level
}

// Point classifies a workflow transition for committing.
type Point string
const (
    PointNone       Point = ""
    PointCompletion Point = "completion"
    PointMilestone  Point = "milestone"
)

func Targets(cfg config.Config, projectRoot string, git Git) ([]Target, error)
func DirtyTargets(targets []Target, git Git) ([]Target, error)
func CommitDirty(targets []Target, message string, git Git) (committed []Target, err error) // err is *CommitError
func PointFor(mode, kind, fromStep, toStep string) Point
func CompletedMilestones(planMarkdown string) []int
func ValidateMessage(message, specName string, milestones []int) error // nil/empty = no milestone required
func PreWorkflowMessage(kind, specName string) string

// CommitError names the repo whose commit failed and git's own reason.
type CommitError struct {
    Target Target
    Cause  error
}
```

**CLI wire contract (serialization boundary with the agent).**

- `spec|plan|implement new --data`: gains an optional `"commit_existing": true|false`.
  - Absent while any target is dirty and the mode is on: the command returns the `uncommitted_changes` report and writes nothing.
- `spec|plan|implement goto --data`: gains an optional `"commit_message_from": "<project-relative path>"`.
  - Required at a due commit point; ignored otherwise.
  - It is never copied into workflow data.
- Workflow data gains `committed_milestones: [N, ...]`, recorded in the same transition that makes a milestone commit.
- New error-shaped responses, all in the existing `ErrorResponse` envelope:

| Code | When | `resource` | `next_action` gives |
|---|---|---|---|
| `uncommitted_changes` | `new` with a dirty target, mode on, and no `commit_existing` | the repo names | ask the user; re-run `new` with `commit_existing` true or false (exact commands) |
| `commit_message_required` | `goto` into a due commit point with no `commit_message_from`, or the file is missing or empty | the step | stage the message file and re-run the exact `goto` with `commit_message_from` |
| `commit_message_invalid` | the message omits the spec name or `Milestone N` | the step | what to add, then re-run |
| `auto_commit_failed` | git refused a commit (hook, error) | the repo name | fix the cause (git's stderr quoted), then re-run the same `goto` / `new` |

```json
{
  "error": true,
  "code": "uncommitted_changes",
  "message": "uncommitted changes in registered repos: spektacular (/home/u/proj), docs (/home/u/site)",
  "resource": "spektacular, docs",
  "next_action": "ask the user whether to commit these changes before the spec workflow starts; then run: spektacular spec new --data '{\"name\":\"login\",\"commit_existing\":true}' to commit them first, or with \"commit_existing\":false to continue without committing"
}
```

**Runtime config (workflow package).** `workflow.Config` gains `AutoCommit string`, set from the loaded setting at each construction site. It is not persisted.

**Template variables (stepkit).** Steps that lead into a commit point get a `commit` object, consumed only by the appended partial:

```go
"commit": map[string]any{
    "point":     "completion" | "milestone",
    "spec_name": "<name>",
    "tmp_path":  ".spektacular/tmp/git-commit-message.md",
}
```

## Implementation Detail

**A new "transition with side effect" shape in the command layer.** Until now a `goto` handler built the workflow, called `Goto`, and let the step callback print straight to stdout. For a due commit point the handler now wraps that call in a small sequence:
1. Validate the input.
2. Snapshot the state file.
3. Point the output writer at a buffer.
4. Transition.
5. Commit.
6. Either flush the buffer (success) or restore the snapshot and return the error (failure).

A developer reading the code sees one shared helper doing this for spec, plan and implement, not three hand-copied variants. When auto-commit is off, the transition is not a commit point, or it is a dry run, the helper takes today's path unchanged: no buffering and no snapshot. That keeps the default mode byte-for-byte identical to current behaviour. The workflow engine itself is untouched: no new FSM callbacks and no new step types. Step callbacks never learn that commits exist, which keeps the "callbacks must not access workflow internals" rule and keeps the `before_` veto semantics described in the knowledge base intact.

**The `new` prologue is split into probe → gate → clear.** Today one helper both detects an in-progress workflow and deletes a stale state file. It becomes two:
- a read-only probe, which still returns the resume report first;
- a clear, which runs only after the new start-of-workflow gate has passed.

The gate is a second shared helper used by all three `new` handlers. It runs after the spec name has been resolved, since spec IDs are assigned before anything is written, so the pre-workflow commit message can name the real spec. Dry runs skip it.

**Commit decisions live in one pure package; git access lives behind two narrow interfaces.**
- **The `autocommit` package** is deliberately free of cobra, output formatting and workflow types. It takes a mode, a kind, step names, plan text and a `Git`, and returns plain values. That makes its commit-point table, milestone parser and message validation unit-testable without git or a project on disk.
- **Real git** is exercised at two levels:
  - package integration tests that create throwaway repos, following the existing real-git test helpers;
  - cmd-level end-to-end tests that drive `new` → … → `finished` against temp git repos.
- **Extracting `gitexec`** follows the existing pattern: fakes through interfaces, and one exec path. The repo runner keeps its interface and its "never a façade" promise.

**Milestones are derived from the plan document, not tracked by new steps.** The parser follows the scaffold's fixed structure:
- `### Milestone N:` headings group the `#### - [ ]`/`- [x] Phase N.M` checkboxes beneath them.
- A milestone counts as complete when it has at least one phase and every phase is ticked.

The committed set is stored in workflow data in the same transition as the commit. So a rollback (state restore) forgets it too, and a retry asks again. When several milestones become complete at once, for example after a resumed session, the message must name each of them. They are recorded together.

**Instruction delivery follows the working-context-footer pattern.** The commit-message partial is appended programmatically by the step renderer, exactly like the footer:
- It is gated on the runtime `AutoCommit` value and the engine's commit-point table, keyed on the step being rendered and its onward step.
- No step template gains conditional sections, and the partial is never included from a template.
- The template contract test table learns the partial the same way it knows the footer.
- Skill text for the `uncommitted_changes` report follows the resume-report wording already in the three skills.

**Wording hygiene.** "Commit" already means "write a document to the store" throughout the step templates. All new prose says "git commit" or "automatic commit" explicitly, and the partial is named for git, so the two meanings never blur in an agent's instructions.

## Dependencies

- **The user's `git` command-line tool (external, runtime).** It carries out every status and commit. It must be on PATH whenever `auto_commit` is on and a registered repo is a git work tree. When git is missing, the command fails with a remediating error instead of silently skipping. No changes needed.
- **The config package (internal, changed).** Gains the `auto_commit` key, its default and its validation. There is no schema bump and no migrate step: the upgrade engine and its registry-contiguity test are untouched.
- **The repo package (internal, changed).** Its registry API (`New`, `Entries`, `LocalSource`) supplies commit targets without cloning, unchanged. Its git runner is rebuilt on the extracted exec helper, keeping the same interface.
- **The workflow engine (internal, lightly changed).** Its runtime config gains the mode. The FSM, callbacks and state persistence are unchanged; the command layer snapshots and restores the state file around a commit.
- **stepkit (internal, changed).** Appends the new partial next to the working-context footer.
- **The output package (internal, unchanged).** Its `ErrorResponse` builders carry the four new error codes. The existing `Writer` is pointed at a buffer for commit-point transitions.
- **`gopkg.in/yaml.v3` (external, unchanged).** It must round-trip the string `off` without turning it into a boolean. A config test pins this.
- **`github.com/looplab/fsm` (external, unchanged).** Its `before_` veto semantics still govern step-callback failures. Commit failures are handled after the transition, by state restore.
- **The spec/plan/implement step packages (internal, unchanged in Go).** Their step names are the keys of the commit-point table. Renaming a step in future would need that table updated, and a test pins the table against the step lists.
- **The docs repo (spektacular-website).** The configuration reference page is edited in the same change. No code dependency.
- **Prior work.** This builds on 000053 (config schema versioning and migrations) for the "defaulted optional key needs no bump" rule and the configuration page layout. It also relies on the resume-report pattern from the interruptible-workflow work. Neither needs changes before this plan starts.
- **Design documents this plan was built on:** none. The spec carries no design references.

## Testing Approach

Testing has four layers. Each one extends a convention the repo already uses.

- **Unit tests for the engine's pure logic.** These get the most coverage, because that logic is where the rules live:
  - **Commit-point table:** every (mode, kind, from, to) combination, including `off` never producing a point and `workflow` never producing a milestone point.
  - **Milestone parser:** partial, complete and empty milestones; a milestone with no phases; several milestones completing at once; checkboxes outside the Milestones block being ignored.
  - **Message validation:** missing spec name, missing `Milestone N`, empty message.
  - **Pre-workflow message wording.**
  - **Target resolution against a fake `Git`:** non-git repos and uncloned sources skipped; shared work trees merged.

  Config gets the existing enum tests: default is `off`, each of the three values loads, an unknown value is refused with a remediating error, and `off` round-trips through YAML as a string.
- **Integration tests with real git.** These use throwaway repos in `t.TempDir()`, with identity pinned through env and global/system config nulled, and skip when git is absent. They cover:
  - the dirty check sees modified and untracked files;
  - `CommitAll` stages new, modified and deleted files;
  - a clean repo gets no commit;
  - a failing `pre-commit` hook surfaces as a `CommitError` that names the repo and quotes the hook's output;
  - no remote-tracking ref changes after a commit.

  The refactored repo git runner keeps its existing integration tests green unchanged, which is the regression guard for the exec-helper extraction.
- **Command-level end-to-end tests.** These drive `rootCmd` through the shared helpers against a temp project whose registered repos are real git repos. They are the acceptance suite:
  - each spec acceptance criterion maps to one test;
  - one of the two registered repos is a git repo and the other is a second git repo, or a plain directory for the non-git case;
  - the tests walk the spec, plan and implement workflows with `goto`, using a two-milestone plan fixture;
  - they assert on `git log`, the commit contents and messages, and the JSON responses.

  The load-bearing guarantees are:
  - `off` and unset leave every log unchanged;
  - `workflow` adds exactly one commit per changed repo at each completion and none mid-workflow;
  - `full` adds one commit per changed repo per milestone plus a completion commit only for leftovers;
  - a dirty tree yields `uncommitted_changes` before anything is written; `commit_existing:true` puts the user's change in its own earlier commit, and `false` sweeps it into the first automatic commit;
  - a hook rejection leaves `state.json` at the previous step, prints only the error envelope, and a retry after fixing the hook succeeds;
  - the committed tree includes `state.json` at its new step, so the next `new` starts silently.
- **Template contract tests.**
  - The commit-message partial appears only on the steps that lead into a commit point, and only when the mode is on. The default-mode render stays byte-identical, pinned by the existing instruction-contract table.
  - The partial and the updated skills avoid the banned stdin/heredoc substrings, and say "git commit" rather than bare "commit".
  - A test pins the commit-point table's step names against the real spec/plan/implement step lists, so a future step rename cannot orphan a commit point.

**Deliberate gaps.** No new harbor scenario is added. Harbor runs with the default (`off`), and nothing about the step order or the default-mode instructions changes, so the existing suites need no oracle updates. The real-agent experience of the new conversations is captured in the implementation test plan instead (below). Agent parity (Claude, Bob, Codex) is structural rather than tested per agent: every behaviour is in Go or in CLI-rendered text that all agents receive byte for byte.

**Success metrics:**
- **"A developer finishes a whole spec → plan → implement cycle without running `git commit` by hand"**: behavioural test. The end-to-end cycle test in `workflow` and in `full` mode ends with a clean `git status` in every registered git repo and never shells out to git outside Spektacular. A real-agent run of the same cycle is manual, captured in the implementation test plan.
- **"Every milestone of a `full`-mode implementation ends with a commit in each repo it changed"**: behavioural test. The two-milestone end-to-end test asserts a commit per milestone per changed repo, and asserts that `goto` out of `update_changelog` is refused without a message once a milestone completes.
- **"A developer is never surprised by an automatic commit containing their own earlier work without first being asked"**: behavioural test. A dirty tree always yields `uncommitted_changes` before any write or commit, and the only path to sweeping those changes in is an explicit `commit_existing:false`. The real conversation, where the agent actually relays the question to the user, is manual, captured in the implementation test plan.
- **"Every automatic commit message can be traced to its spec, and milestone where there is one"**: behavioural test. Message validation refuses messages without the spec name or `Milestone N`, and the end-to-end tests assert each commit subject or body contains them.

## Milestones & Phases

### Milestone 1: Projects can have their work committed automatically when a workflow completes

**What changes**: A project can set `auto_commit` to `off`, `workflow` or `full` in its Spektacular settings. It defaults to `off`, and in that mode nothing changes. With `workflow` on, finishing a spec, a plan or an implementation makes one git commit in each registered repository that changed. The coding agent writes a message naming the spec and describing what was done, and nobody is asked for confirmation. Repositories that aren't git repositories, or that have nothing to commit, are left alone. If a commit is rejected, for example by a pre-commit hook, the workflow stays where it was and says which repository failed and why. Nothing is ever pushed. `full` is accepted from this milestone on, and behaves like `workflow` until Milestone 3 adds its milestone commits.

**Validation point**: With `auto_commit: workflow`, a spec → plan → implement run leaves one new commit per changed repository at each completion and none in between, each with a message naming the spec. Unset or `off` leaves every git log untouched. A failing pre-commit hook leaves the workflow on its previous step, and a retry after fixing the hook succeeds. The full test suite passes.

#### - [x] Phase 1.1: Add the auto_commit setting

**Repo:** spektacular

This phase adds the `auto_commit` project setting with its three values and its `off` default, and makes it available to the workflows at run time. Existing projects keep working without an upgrade, because a missing key simply means `off`. Nothing commits yet. This phase only establishes the switch every later phase reads.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-add-the-auto_commit-setting)

**Acceptance criteria**:
- [x] A project whose settings say `auto_commit: off`, `workflow` or `full` loads without error for each value.
- [x] A project that never mentions `auto_commit` loads as `off`, and `migrate` reports nothing to do for it.
- [x] Any other value is refused with a message that lists the three allowed values and says how to fix the file.
- [x] Saving the settings writes `off` back as the text `off`, not as a yes/no value.
- [x] Spec, plan and implement workflows receive the configured mode at run time.

#### - [x] Phase 1.2: Build the git commit engine

**Repo:** spektacular

This phase builds the self-contained part that knows how to commit, without wiring it into any workflow yet. It finds the git repositories behind the registered repos, skipping ones that aren't git or aren't on disk and treating two registered repos in one git repository as one. It tells which have uncommitted changes, including brand-new files, and commits everything in them using the user's own git. The existing clone logic is moved onto the same shared way of running git, so there is still one path to the git binary.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-build-the-git-commit-engine)

**Acceptance criteria**:
- [x] Modified, deleted and never-added files all count as uncommitted, and all end up in the commit.
- [x] A registered repo that is not a git repository, or has not been cloned, is skipped without an error.
- [x] Two registered repos inside the same git repository produce one commit there, not two.
- [x] A repo with nothing to commit gets no commit and no error.
- [x] A commit rejected by a pre-commit hook comes back as a failure naming the repo and quoting the hook's output.
- [x] Commits run the repository's hooks and use the user's own git identity and signing settings. Nothing is pushed.
- [x] Cloning a registered git repo works exactly as before.

#### - [x] Phase 1.3: Commit when a spec, plan or implementation completes

**Repo:** spektacular

This phase connects the engine to the three workflows. In `workflow` or `full` mode, the step before completion tells the agent to write a message naming the spec and what was done, and to hand it over with the final `goto`. Spektacular checks the message, finishes the workflow, and commits every changed repo. If any commit fails, the workflow is put back on the step it was on and the user is told which repo failed and why. In `off` mode the agent sees exactly the instructions it sees today.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-commit-when-a-spec-plan-or-implementation-completes)

**Acceptance criteria**:
- [x] In `workflow` mode, finishing a spec, a plan and an implementation each adds exactly one commit to each repo that changed, and no commits appear while those workflows are still running.
- [x] The committed state includes the workflow's own bookkeeping, so the tree is clean straight after completion.
- [x] Finishing without a commit message, or with one that doesn't name the spec, is refused with instructions on how to supply a proper one, and nothing is changed.
- [x] A commit rejected by a hook leaves the workflow on its previous step and shows only an error naming the repo and the reason. Re-running the same step after fixing the hook completes it.
- [x] The user is never asked to confirm a commit.
- [x] With `off` or no setting, the three workflows run to completion with every git log unchanged, and the agent's instructions are identical to today's.
- [x] Commit instructions say "git commit" explicitly, so they can't be confused with saving a document to Spektacular.

### Milestone 2: Users are asked about their own uncommitted work before a workflow starts

**What changes**: When automatic commits are on and a spec, plan or implement workflow is started while any registered repository has uncommitted changes (new, never-added files included), the agent first tells the user which repositories have them and asks whether to commit them before starting. Answering yes records the user's changes in their own commit, whose message names the spec about to be worked on and says these are the user's earlier changes. Only then does the workflow begin. Answering no starts the workflow, and its automatic commits include everything. A clean tree, or `off` mode, starts with no question at all.

**Validation point**: A dirty tree with the mode on makes `new` return the uncommitted-changes report before any file is written. Re-running with "commit first" produces a separate earlier commit holding exactly the pre-existing change. Re-running with "continue" sweeps that change into the first automatic commit. A clean tree or `off` mode never asks. The full test suite passes.

#### - [x] Phase 2.1: Check for uncommitted changes before a workflow starts

**Repo:** spektacular

This phase adds the start-of-workflow check to `spec new`, `plan new` and `implement new`. With automatic commits on, if any registered repo has uncommitted changes, starting a workflow stops before anything is written and reports which repos are affected. Starting again with "commit first" saves the user's changes in their own commit, whose message names the spec and says these are the user's earlier changes. Starting again with "continue" goes ahead. An in-progress workflow is still offered for resume first, as today.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-check-for-uncommitted-changes-before-a-workflow-starts)

**Acceptance criteria**:
- [x] With the mode on and a modified or new file in a registered repo, starting any of the three workflows reports the affected repos and writes nothing: no spec scaffold, no state, no working-context reset.
- [x] Choosing "commit first" adds a commit holding exactly the pre-existing changes, before any of the workflow's own commits, with a message naming the spec and saying they are the user's changes from before the workflow.
- [x] Choosing "continue" starts the workflow, and the pre-existing change appears in its first automatic commit.
- [x] A clean tree, `off` mode, and non-git repos never trigger the question.
- [x] An interrupted workflow is still offered for resume before any uncommitted-changes question.
- [x] A failed "commit first" commit stops the workflow from starting and says which repo failed and why.

#### - [x] Phase 2.2: Teach the workflow skills to ask the user

**Repo:** spektacular

This phase updates the spec, plan and implement skills installed for every agent so they recognise the uncommitted-changes report. The agent tells the user which repositories have uncommitted changes, asks whether to commit them before starting, and restarts the workflow with the answer. It follows the same shape the skills already use for resuming an interrupted workflow.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-teach-the-workflow-skills-to-ask-the-user)

**Acceptance criteria**:
- [x] Each of the three skills explains the uncommitted-changes report and gives both restart commands.
- [x] The skills tell the agent to ask the user, never to decide on the user's behalf.
- [x] Claude, Bob and Codex receive identical skill text.
- [x] The new text passes the existing checks that keep piped-input and heredoc instructions out of installed skills.

### Milestone 3: Full mode commits after every implementation milestone, and the setting is documented

**What changes**: With `auto_commit: full`, an implementation also makes a commit in each changed repository as each of the plan's milestones finishes. The message names the spec and the milestone. The completion commit at the end then picks up only what changed after the last milestone, and is skipped where nothing is left. The documentation site's configuration reference and the project README describe `auto_commit`, its three values and its `off` default.

**Validation point**: With `full` and a two-milestone plan, the implement workflow adds one commit per changed repository at each milestone, then a completion commit only where changes remain. Leaving a finished milestone without a message is refused. The docs site builds with the new configuration entry listing `off`, `workflow` and `full` and the default. The full test suite passes.

#### - [x] Phase 3.1: Commit after each implementation milestone in full mode

**Repo:** spektacular

This phase adds `full` mode's extra commits. After each phase of an implementation, Spektacular reads the plan's checkboxes. When a milestone has just had its last phase ticked, moving on requires a commit message naming the spec and that milestone, and every changed repo is committed. Milestones already committed are remembered so they are never committed twice. The completion commit at the end then picks up only what changed after the last milestone.

*Technical detail:* [context.md#phase-31](./context.md#phase-31-commit-after-each-implementation-milestone-in-full-mode)

**Acceptance criteria**:
- [x] With `full` and a two-milestone plan, finishing each milestone adds one commit per changed repo, naming the spec and the milestone.
- [x] The completion commit then adds a commit only to repos that still have changes, and skips the rest without error.
- [x] Moving on after a finished milestone without a message, or with one that doesn't name the milestone, is refused with instructions.
- [x] Moving between phases inside a milestone asks for no message and makes no commit.
- [x] `workflow` mode never makes milestone commits.
- [x] A rejected milestone commit leaves the implementation on the phase-wrap-up step, and a retry commits it.

#### - [x] Phase 3.2: Document the auto_commit setting

**Repo:** docs, spektacular

This phase documents the setting. The docs site's configuration reference gains an `auto_commit` entry, and the key is added to the example config and the key list. The spektacular README's config example gains the key too. Both describe the three values, the `off` default, and that commits are local only.

*Technical detail:* [context.md#phase-32](./context.md#phase-32-document-the-auto_commit-setting)

**Content example** (docs, `src/pages/configuration.mdx`, inside the "Project configuration keys" block, directly after `spec_trigger_threshold`):

```mdx
  <ConfigKey name="auto_commit" type="string" defaultValue="<code>off</code>">

    Whether Spektacular records the agent's work in git for you. One of
    `off` (default: no automatic commits), `workflow` (one commit in each
    changed repo when a spec, plan or implementation completes), or `full`
    (as `workflow`, plus a commit after each milestone of an
    implementation). Commits use your own `git`, run your hooks, use your
    identity and signing settings, and are never pushed. When it is on and a
    registered repo has uncommitted changes as a workflow starts, the agent
    asks whether to commit them first. A repo that isn't a git repository is
    skipped.

  </ConfigKey>
```

Example config line (after `spec_trigger_threshold: moderate`): `auto_commit: "off"`. The key list sub becomes "Fourteen top-level keys … `spec_trigger_threshold`, `auto_commit`, `debug`, …".

**Acceptance criteria**:
- [x] The configuration reference has an `auto_commit` entry listing `off`, `workflow` and `full`, describing each, and stating that the default is `off`.
- [x] The example config and the top-level key list include `auto_commit`, and the key count matches.
- [x] The README's config example shows `auto_commit` with its values.
- [x] The docs site builds and type-checks cleanly, and the new text contains no em dashes or layout markup.

## Open Questions

One question can only be answered by running real commits during implementation:

- **Does a GPG or SSH commit-signing setup that needs an interactive pinentry work when Spektacular invokes git from an agent's shell?**
  - **Depends on:** the user's signing agent configuration. A cached agent works; one that needs a TTY prompt may not. Only a real signing setup shows which.
  - **What to do:** a pinentry failure already surfaces as `auto_commit_failed` with git's stderr, and the workflow halts. That meets the spec. If manual testing shows the failure message is unclear for this case, add a hint to the `auto_commit_failed` next_action (for example "unlock your signing key, then re-run"). Do not add `--no-gpg-sign` or any bypass. If a bypass looks necessary, STOP and ask the user, because it would break the constraint that commits use the user's signing settings.

Nothing else is open. Every other decision is recorded in the assumption log.

## Out of Scope

- **Commits more often than once per milestone.** There are no commits per phase or per step inside a milestone. (Spec non-goal.)
- **Any git operation other than a local commit.** No push, branches, pull requests or tags. Automatic commits stay local. (Spec non-goal.)
- **Running workflows in separate git worktrees.** Deferred to a future spec. The user named worktrees as the likely next step for git management. (Spec non-goal.)
- **Undoing, amending or squashing automatic commits.** The user uses ordinary git for that. (Spec non-goal.)
- **Automatic commits for the repo-add workflow.** `repo new`/`repo goto` has no commit points. The spec covers only the spec, plan and implement workflows.
- **A per-repository or per-run override of the mode.** The mode is set once per project in its settings. There is no command-line flag and no per-repo setting.
- **A new harbor end-to-end scenario for automatic commits.** The mechanics are covered by the Go end-to-end tests with real git, and the real-agent conversation is a manual item in the implementation test plan. The existing harbor suites run with the default `off` and need no changes.
- **Changes to the upgrade (`migrate`) engine.** The new key needs no schema bump or upgrade step.

## Changelog

### 2026-09-23 — Phase 1.1: Add the auto_commit setting

**What was done**: Added the `auto_commit` project setting with its three values (`off`, `workflow`, `full`), defaulting to `off`, including validation that refuses any other value with a remediating `config_invalid` error. Added an `AutoCommitMode()` helper that resolves an absent key to `off`, carried the resolved mode onto `workflow.Config` as a non-persisted field, and set it at all six spec/plan/implement workflow construction sites. Nothing commits yet; this phase only establishes the switch later phases read.

**Deviations**: Acceptance criterion 5 ("spec, plan and implement workflows receive the configured mode at run time") is left unticked. The mode is passed at every construction site, but no production code reads `workflow.Config.AutoCommit` yet, so it is not observable from any cmd- or step-level test without a production seam this phase deliberately does not add. Phase 1.3 wires the mode into `stepkit` rendering and should tick this criterion then.

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/workflow/workflow.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/implement.go`

**Discoveries**:
- `off` is a YAML 1.1 boolean token. yaml.v3 decodes a bare `off` into a `string` field as `"off"` and quotes it on marshal, so the round-trip is safe, but it is pinned by a test (`TestToYAMLFile_AutoCommitOffIsWrittenAsText`) because the failure mode would be silent.
- `Config.AutoCommit` was added without `omitempty`, so `ToYAMLFile` now always emits the key. No golden fixture or migrate test needed updating, confirming the plan's "no schema bump" call: `ParseYAMLFile` unmarshals over `NewDefault()`, so an absent key keeps the default.
- The existing `spec_trigger_threshold` validation returns a bare `fmt.Errorf`, not an `output.NewError` envelope. The new `auto_commit` validation uses the envelope per the repo's error-remediation convention, so the two enum checks now differ in shape. Bringing `spec_trigger_threshold` into line is out of scope for this plan.
- `withProjectSchema()`, the `schema: 3` prefix helper for config.yaml fixtures, lives in `internal/config/repo_test.go`, not `config_test.go`.

### 2026-09-23 — Phase 1.2: Build the git commit engine

**What was done**: Extracted the git exec path out of the repo package into a new `internal/gitexec` package, so there is still exactly one place that locates and runs the user's `git`, and rewrote `execGitRunner.run` as a one-line delegate to it. Added the `internal/autocommit` package, which owns every commit decision: resolving registered repos to git work trees without cloning, reporting which are dirty, committing them whole with the user's own git, classifying workflow transitions as commit points, and validating an agent-written commit message. Nothing is wired into a workflow yet.

**Deviations**: `milestonePoints` in `points.go` is deliberately left empty. The plan assigns the milestone transition rows to Phase 3.1, so `full` mode is accepted but behaves exactly like `workflow` until then. The `PointMilestone` constant and the `LeadsToCommit` full-mode branch are already in place, so 3.1 is a data change plus the milestone parser.

Extracted the real-git test helpers into a new `internal/testutil/gittest` package rather than duplicating ~40 lines into `internal/autocommit`, and rewired `internal/repo/git_integration_test.go` onto it. The plan's context offered either option and preferred extraction.

**Files changed**:
- `spektacular: internal/gitexec/gitexec.go`
- `spektacular: internal/repo/git.go`
- `spektacular: internal/repo/git_integration_test.go`
- `spektacular: internal/testutil/gittest/gittest.go`
- `spektacular: internal/autocommit/git.go`
- `spektacular: internal/autocommit/targets.go`
- `spektacular: internal/autocommit/commit.go`
- `spektacular: internal/autocommit/points.go`
- `spektacular: internal/autocommit/message.go`
- `spektacular: internal/autocommit/targets_test.go`
- `spektacular: internal/autocommit/commit_test.go`
- `spektacular: internal/autocommit/points_test.go`
- `spektacular: internal/autocommit/message_test.go`
- `spektacular: internal/autocommit/git_integration_test.go`

**Discoveries**:
- **Git redirects a hook's stdout to its own stderr.** A pre-commit hook that `echo`s without `>&2` still arrives in `ExitError.Stderr`, so the exec path needs no stdout fallback to quote a rejecting hook. A fallback was written on a sub-agent's recommendation, found to be dead code when the new test passed without it, verified directly against git, and reverted. `TestIntegration_PreCommitHookOnStdoutIsReported` pins the behaviour so nobody re-adds it.
- `repo.Set.LocalSource` never invokes git — it resolves from the registry, `repo.yaml` and the filesystem. `Targets` therefore passes a nil `GitRunner` to `repo.New`, and never calls `Resolve`/`ResolveAll`, which is what keeps commit-target resolution from triggering a clone.
- `CommitDirty` returns the targets it committed *before* a failure alongside the `*CommitError`. Those commits stand, so a retry finds them clean and commits only what is left. Phase 1.3's rollback restores `state.json`, not the commits.
- `ValidateMessage` returns an `*output.ErrorResponse` carrying the code and message only. Phase 1.3 must add the `next_action`, since only the command layer knows the exact `goto` to re-run.
- `points_test.go` pins every step name in `completionPoints` against the real `spec.Steps()`/`plan.Steps()`/`implement.Steps()` lists, so a future step rename cannot silently orphan a commit point. No import cycle results.

### 2026-09-23 — Phase 1.3: Commit when a spec, plan or implementation completes

**What was done**: Connected the commit engine to the three workflows. A new shared `gotoWithAutoCommit` helper in `cmd/autocommit.go` backs all three `goto` handlers: off a commit point it behaves exactly as before, and at one it validates the agent's staged message, snapshots `state.json`, transitions, then commits every dirty repo, restoring the snapshot and reporting `auto_commit_failed` if git refuses. The step renderer now appends a new git-commit instruction partial on the steps that lead into a commit, driven by the commit-point table rather than per-step template edits.

This phase also closes Phase 1.1's deferred acceptance criterion 5. `stepkit` now reads `workflow.Config.AutoCommit`, so the configured mode is observable in a rendered step and is asserted for all three workflow kinds.

**Deviations**: None to the plan's intent. One structural consequence had to be worked around: making `stepkit` import `autocommit` meant `autocommit`'s own in-package test could no longer import the step packages, because that closes a cycle through the renderer. Production code has no cycle. Resolved by exporting `autocommit.ReferencedSteps()` and moving the commit-point pin test into the external `autocommit_test` package, which may import both sides. The pin is unchanged in strength.

**Files changed**:
- `spektacular: cmd/autocommit.go`
- `spektacular: cmd/autocommit_test.go`
- `spektacular: cmd/instruction_contract_test.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/implement.go`
- `spektacular: internal/stepkit/stepkit.go`
- `spektacular: internal/autocommit/points.go`
- `spektacular: internal/autocommit/points_test.go`
- `spektacular: internal/autocommit/points_pin_test.go`
- `spektacular: templates/partials/git-commit-message.md`

**Discoveries**:
- **`stepkit` importing `autocommit` creates a test-only import cycle.** Anything in `internal/autocommit` that wants the real step lists must live in the external `autocommit_test` package, because `steps/* -> stepkit -> autocommit` closes the loop for an in-package test. `go build ./...` stays green while `go test` fails, so this surfaces only when the tests are compiled. Do not move `points_pin_test.go` back into `package autocommit`.
- The helper buffers step output on **every** path, not just the commit path, flushing immediately when there is no commit. That keeps the default mode byte-identical to writing straight to stdout, and it is what makes discarding output on a failed commit possible at all.
- Rollback restores `state.json` only. Commits already made to earlier repos at the same commit point stand, and the retry finds those repos clean, so it commits only what is left. This is deliberate: the alternative is rewriting other repositories' history.
- The partial soft-wraps `**git commit**` across a line break, so the literal phrase `git commit` appears on one line only in the `## Automatic git commit` heading. Tests asserting on the wording flatten whitespace first rather than matching raw template text.
- `requireStepTableComplete` in `cmd/instruction_contract_test.go` walks only `steps/<workflow>/`, so a new file under `templates/partials/` is outside its scope by construction and needs no table entry.

### 2026-09-23 — Phase 2.1: Check for uncommitted changes before a workflow starts

**What was done**: Added the start-of-workflow gate to `spec new`, `plan new` and `implement new`. With automatic commits on, a registered repo holding uncommitted changes now stops the command before anything is written and reports which repos are affected, so the agent can put the question to the user. Re-running with `commit_existing: true` saves the user's work in its own commit, naming the spec and saying the changes predate the workflow; `false` proceeds and lets the workflow's own commits sweep them in. To make "writes nothing" literally true, `resumeOrClear` was split into a read-only `probeResume` and a separate `clearState`, so the stale state file is only deleted once the gate has passed.

**Deviations**: The plan expected spec, plan and implement to be `resumeOrClear`'s only callers, so the wrapper could be dropped. `cmd/repo.go` is a fourth caller. The wrapper was kept for it, which is the branch the plan anticipated; the repo workflow has no commit points and so needs no gate.

**Files changed**:
- `spektacular: cmd/resume.go`
- `spektacular: cmd/autocommit.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/implement.go`
- `spektacular: cmd/startgate_test.go`
- `spektacular: cmd/autocommit_test.go`

**Discoveries**:
- **`commit_existing` cannot leak into workflow data.** None of the three `new` handlers copy arbitrary `--data` keys the way the `goto` handlers do — each unmarshals into a typed struct reading only `name` (and `id` for spec). The plan's "strip it before the SetData loop" precaution applies to `goto`, not `new`, so `startGate` reads the answer straight from the raw `--data` string.
- A Phase 1.3 test had to change, and it was the test that was wrong. It seeded plan and changelog fixtures *after* building the git fixture, leaving the tree dirty, so the new gate correctly refused to start the workflow. The fix folds those fixtures into the baseline commit with `commit --amend`, which keeps every hand-written commit-count oracle ("1" before, "2" after) true rather than loosening it.
- `json.Marshal` sorts map keys, so rebuilding the caller's `--data` with the answer added produces a stable, testable command string. The rendered re-run command therefore reads `{"commit_existing":true,"name":"billing"}`, with the answer first.
- The gate deliberately runs *after* implement's plan-exists check, so a missing plan is still reported as a missing plan rather than being masked by an uncommitted-changes question.

### 2026-09-23 — Phase 2.2: Teach the workflow skills to ask the user

**What was done**: Added an "If the project has uncommitted changes" section to the spec, plan and implement workflow skills. Each names the `uncommitted_changes` report, says to relay the affected repositories to the user and ask whether to commit that work first, gives both restart commands, explains what each answer does, forbids deciding on the user's behalf, and covers `auto_commit_failed`.

**Deviations**: The plan sketched adding the report as a third outcome of spek-new's bare in-progress probe. That would have been wrong: the gate only fires on the *named* `new` invocation, because a bare `spec new` with no `--data` returns `name_required` or a resume report before the gate is reached. The section sits after the start and resume blocks in each skill instead.

**Files changed**:
- `spektacular: templates/skills/workflows/spek-new/SKILL.md`
- `spektacular: templates/skills/workflows/spek-plan/SKILL.md`
- `spektacular: templates/skills/workflows/spek-implement/SKILL.md`
- `spektacular: internal/agent/uncommitted_changes_test.go`

**Discoveries**:
- **Nothing in the repo compares the checked-in `.claude/skills/` copies against `templates/`.** Every rendered-skill test installs into `t.TempDir()` first, so the installed copies going stale between a template edit and the next `init`/`migrate` breaks no test. That is deliberate, but it does mean template drift is invisible until someone reinstalls.
- Cross-agent identity of skill text was not previously asserted anywhere. The per-agent tests (`claude_test.go`, `bob_test.go`, `codex_test.go`) only check per-skill substrings within one agent, so nothing would have caught Claude and Codex receiving different text. `TestUncommittedChangesTextIsIdenticalAcrossAgents` is now the only guard against that drift.
- `instruction_surface_test.go`'s banned-substring walk installs all workflow skills into a temp dir and sweeps every `*.md`, so new skill prose is covered by it automatically and needs no per-phase addition.
- Acceptance criteria are worth reading literally. "Gives both restart commands" was first satisfied with one parameterised command plus prose bullets; the tests flagged the gap rather than accommodating it, and the templates now carry two explicit code blocks.

### 2026-09-23 — Phase 3.1: Commit after each implementation milestone in full mode

**What was done**: Added `full` mode's extra commits. A new milestone parser reads the plan's `## Milestones & Phases` section and reports which milestones have every phase ticked. The two moves out of `update_changelog` became milestone commit points, so when a milestone has just closed, advancing requires a message naming the spec and that milestone, and every changed repo is committed. Milestone numbers already committed are recorded in workflow data so none is committed twice, and the completion commit at the end picks up only what changed after the last one.

**Deviations**: None.

**Files changed**:
- `spektacular: internal/autocommit/milestones.go`
- `spektacular: internal/autocommit/milestones_test.go`
- `spektacular: internal/autocommit/points.go`
- `spektacular: internal/autocommit/points_test.go`
- `spektacular: cmd/autocommit.go`
- `spektacular: cmd/milestones_test.go`

**Discoveries**:
- **A milestone commit point is a candidate, not a commit.** Most phase wrap-ups land mid-milestone, so when nothing is due the transition falls through to the ordinary `Goto` path: no message is asked for, and a message the agent staged anyway is left on disk untouched rather than consumed. Without that fall-through, every phase in a multi-phase milestone would demand a commit message.
- `committed_milestones` is written to workflow data **before** `Goto`, so the state save carries it and the rollback forgets it along with the step. A retry after a rejected commit therefore asks for the same milestones again rather than silently skipping them — verified by asserting the key is absent from `state.json` after a hook rejection.
- Workflow data round-trips through JSON, so recorded milestone numbers come back as `[]any` of `float64`, never `[]int`. Any future numeric workflow-data key needs the same normalisation.
- The parser is deliberately scoped between `## Milestones & Phases` and the next `## ` heading. A plan's own `## Changelog` section — which this very workflow appends phase entries to — and `## Out of Scope` both contain checkbox-shaped lines, so an unscoped scan would miscount.
- `ReferencedSteps()` now returns `implement: [reconcile_spec finished update_changelog test_plan analyze]`, so the external pin test covers the milestone step names too and a rename of any of them fails the pin.

### 2026-09-23 — Phase 3.2: Document the auto_commit setting

**What was done**: Documented the setting in both repos. The docs site's configuration reference gained an `auto_commit` entry describing the three values, the `off` default, and that commits are local only and use the user's own git; the key was added to the example config and the top-level key list, and the page frontmatter now names it. The spektacular README's project config example gained the key with its accepted values.

**Deviations**: None.

**Files changed**:
- `docs: src/pages/configuration.mdx`
- `spektacular: README.md`
- `spektacular: cmd/docs_test.go`

**Discoveries**:
- The docs site's top-level key list carries a hand-written count ("Thirteen top-level keys", now "Fourteen"). Adding a config key means updating the numeral as well as the list, and nothing checks the two agree, so the count can silently drift out of step with the keys beside it.
- `cmd/docs_test.go` asserts against the real `README.md` at the repository root rather than a rendered copy, deliberately, because the README itself is the artifact under test. That is the one place in this repo where a test reads a file outside its own scratch directory, and it is why a README change can fail the Go suite.
- The docs repo's `make check` reports one pre-existing hint from `src/layouts/Shell.astro` (a deprecated `document.execCommand`). It is unrelated to this change; the bar for the page is 0 errors and 0 warnings, which it meets.
