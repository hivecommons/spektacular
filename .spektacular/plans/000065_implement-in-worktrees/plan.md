---
created_date: "2026-10-07"
document_status: final
closed_date: "2026-10-07"
---

# Plan: 000065_implement-in-worktrees

<!-- Metadata -->
<!-- Created: 2026-10-07T11:35:16Z -->
<!-- Commit: a5d725c -->
<!-- Branch: b-patch-epics -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Every implement run, started on its own or by an epic, builds its spec in the spec's own git worktrees, one per repo its plan touches. A run started on its own merges back all or nothing when its plan is complete, so two people or terminals can implement different specs side by side without seeing each other's half-finished work. Alongside that, the plan closes three gaps found in epic runs: sub-agents are told exactly where the code lives, a repo can declare a setup command that prepares each new worktree, and the epic's uncommitted-work warning covers only the repos it builds and names them. Teams running Spektacular on multi-repo projects benefit, whether they use epics or implement specs one at a time.

## Conventions

- **Error messages must describe the problem and suggest remediation** — the new refusals (touched repo not a git repo or without commits, failed worktree setup command, `implement merge` conflicts and prechecks) all need a concrete `next_action`.
- **A plan never changes the active skills and configuration** — this plan changes implement step templates, skills and config defaults. It avoids a schema bump, so this repo needs no migration, and verification uses `go test` and throwaway projects only.
- **Spektacular's own files are written through the CLI** — worktrees stay code-only. The setup command and record are read from the main project, never from a worktree's `.spektacular`, and no task hand-builds a store path.
- **Tests must not depend on execution order** — new cmd tests executing `rootCmd` go through `resetRootCmd`/`runRootCmd`, and real-git fixtures pin identity.
- **Passing tests are required before calling work done** — the full `go test ./...` passes at the end of every task.
- **A config value that is a YAML 1.1 boolean token needs a round-trip test** (gotcha) — `implement.worktrees` is a real bool, but a round-trip test still pins that `false` is written and read back, and that an absent key loads as `true`.
- **Mustache HTML-escapes double-brace values** (gotcha) — worktree roots and the setup command reach the agent with triple braces.
- **docs: No em dashes** — all new docs prose and the docs CHANGELOG entry avoid em dashes.
- **docs: MDX authoring conventions** — new ConfigKeys and prose use slots and fenced code, with no layout HTML. Verified by `npm run build` and `npx astro check`.
- **docs: Plans must sketch content structure** — each docs task carries a content example.
- **docs: Label before filename in file-scoped reference headings** — new keys go under the existing "Project configuration keys" and "Repository configuration keys" groups rather than new filename-first sections.

## Architecture & Design Decisions

**Shape.** Every implement run builds a spec in the worktrees epics already use (`spek/<spec>` under `.spektacular/worktrees/<spec>/`, one per touched checkout). There is no second mechanism. In the `spektacular` repo, `implement new` (`cmd/implement.go`) gains one step between the dependency check and the workflow start. For a run that is not orchestrated, not a dry run, has worktrees enabled and has no worktree record yet, it computes the plan's touched repos and calls the same `worktree.Manager.Ensure` that `epic worktree` uses (`cmd/epic_worktree.go`, `internal/worktree/worktree.go`). If a record already exists, from an earlier task run, a resume or an `epic worktree`, it is reused and no git runs. Code roots then flow exactly as 000064 made them flow: `codeRootsFor` reads the record and the steps render the worktree locations. Epic children are orchestrated, so they never create worktrees. The orchestrator still does, whatever the opt-out says, which keeps "epic runs always use worktrees". The opt-out is a new `implement.worktrees` bool, default `true`, in `internal/config/config.go`. Because the config parses over `NewDefault()`, a project whose config predates the key gets worktrees with no schema bump and no migration (convention: a plan never changes the active install, and this repo is not migrated). So that two terminals can each run a spec, a run started on its own with worktrees on also keeps its workflow progress and notes in a per-spec lane, the same lane files epic children use (`internal/workflow/lane.go`), rather than the single shared `state.json`. Being in a lane is separated from being orchestrated (`cmd/implement.go`, `cmd/autocommit.go`, `internal/stepkit/stepkit.go`): the user chose this, and only orchestrated runs hand back to an orchestrator. With the opt-out set, `implement new` uses the shared slot, skips creation and every later behaviour falls through to today's code paths, so the run behaves exactly as before.

**Finishing a run started on its own.** Merging stays a single all-or-nothing operation. A new `implement merge` command (`cmd/implement_merge.go`) wraps `worktree.Manager.Merge`, the same prechecks, the `.spektacular` guard, the `merge-tree` dry run and the cleanup `epic merge` uses, with conflict output shared with `mergeConflict`. The finished step (`templates/steps/implement/12-finished.md`) renders a merge instruction only for a worktree run that is not orchestrated and whose plan is complete. A task run that leaves tasks open keeps and reuses its worktrees. On a refusal the agent reports the named repos and paths and stops. It never resolves, rebases or switches branches, and the worktrees stay for the user. That the workflow's own instruction runs the merge, rather than the CLI merging inside the `finished` transition, was the user's choice. It mirrors how the epic orchestrator already runs `epic merge`, and it keeps the merge outcome out of the commit-and-rollback path in `gotoWithAutoCommit` (`cmd/autocommit.go`). For there always to be something to merge, a run with a worktree record commits its **code** in the worktrees at completion points even when `auto_commit` is `off`. `internal/autocommit/points.go` and `internal/stepkit/stepkit.go` treat completion as a code-only commit point for such runs, and `commitImplementLane` skips the main-checkout artifact commit when auto_commit is off. With auto_commit on, behaviour is unchanged from 000064. Touched repos that are not git repos, or have no commits, are refused up front with a remediation naming the repo and the opt-out (convention: error messages suggest remediation). They never fall back to the main checkout silently.

**Preparing a worktree, and telling sub-agents where it is.** A repo declares `worktree_setup`, a shell command, in its own `repo.yaml` (`internal/config/repo.go`), so the command travels with the repo. It can be set through `repo add --data` (`internal/repo/register.go`, `cmd/repo.go`), and `repo list` shows it. `Ensure` reads it from the main registration, never from the worktree's copy (spec constraint: nothing under a worktree's `.spektacular` is read). It runs the command through a fakeable runner, modelled on `gitexec`, in each touched repo's code root of a newly created worktree, so epics benefit too. A non-zero exit removes that just-created worktree, so a retry starts clean, and refuses with the repo, the command and its stderr. The implement steps that analyse, implement, test and verify a task (`templates/steps/implement/02`–`05`) include a new partial, `templates/partials/implement-code-locations.md`. When worktree roots are present, it lists each repo's worktree root and requires every sub-agent to be given those roots and to edit, build and check only there. It also says a new worktree holds only tracked files and its dependencies are prepared inside it, never installed into or shared from a main checkout. When no worktree roots are present, it still requires sub-agents to be passed each repo's `root` from `repo list`. The `spek-implement` skill gains a short worktree section: merge at the end, conflicts are the user's to resolve, and no merge, rebase or branch switch on the agent's own initiative.

**Dependencies and the dirty check.** The design (`design:epics-and-seeded-specs.md`) requires the implement check and `status` to classify a dependency through one function, so "implemented but not merged" goes into `status.DependenciesOf` and `buildTarget` (`internal/status/report.go`), not into `refuseUnmetDependencies` alone. A dependency whose plan is complete but whose worktree record still exists is unmet, described as "implemented but not yet merged". The record is deleted only by a successful merge, so its presence is a git-free unmerged signal. The warn-or-refuse handling under `epic.strict_dependencies` is unchanged, as the design requires. The epic's uncommitted-work check ports PR #79's code only. `RunSource.Dirty` takes the touched repos and returns the dirty ones (`internal/status/run.go`, `cmd/status.go`), and `epic.run.dirty_repos` is added beside the unchanged `dirty` bool. Its skill wording is written fresh for the post-000064 project-root rule. The `docs` repo (`src/pages/configuration.mdx`, `how-it-works.mdx`, `epics.mdx`, `projects.mdx`, `CHANGELOG.md`) documents all four behaviours. Rejected directions, among them automatic CLI merge, fallback to the main checkout, a schema bump, a git-based unmerged check and porting PR #79 wholesale, are recorded with evidence in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Project configuration (changed, spektacular).** Owns the new worktree opt-out, which is on unless the project turns it off and is read as on when the key is absent. It is consulted only by the implement start. Epic commands ignore it.
- **Repo configuration and registration (changed, spektacular).** A repo's own configuration gains an optional worktree setup command. Registration (`repo add`) can set it, and `repo list` reports it. It travels with the repo and is always read from the repo's registered location, never from inside a worktree.
- **Worktree manager (changed, spektacular).** Still the only component that creates, records, merges and removes a spec's worktrees, now for every run rather than only epics. Creation gains two things: a refusal when a touched repo cannot have a worktree (not a git repo, or no commits), and a setup step that runs each touched repo's declared command in a newly created worktree, removing that worktree and refusing on failure. A small command runner, separate from its git runner so tests can stub it, carries out the setup. Merge, its prechecks, the Spektacular-directory guard and cleanup are unchanged and shared by both merge entry points.
- **Implement start (changed, spektacular).** After the dependency check, a run that is not orchestrated, with worktrees on and no existing record, asks the worktree manager for the spec's worktrees before the workflow starts. Code roots then reach the steps exactly as they do for epic children today. With the opt-out, a dry run, an orchestrated child, or an existing record, it does nothing new.
- **Workflow slot selection (changed, spektacular).** Decides where a run keeps its progress and notes. A run started on its own with worktrees on now gets a per-spec lane, the same mechanism epic children use, so several runs can be in progress at once. Being in a lane is separated from being orchestrated: only orchestrated runs hand back to an orchestrator. With the opt-out, the shared slot is used as today.
- **Implement merge command (new, spektacular).** The merge entry point for a run started on its own. It hands the spec to the worktree manager's all-or-nothing merge and reports either the merged repos and cleanup, or a refusal naming each repo and its conflicting paths. It is new because `epic merge` is named and documented as an epic operation, and the finished step needs a command that reads naturally for any spec. It shares conflict reporting with `epic merge`.
- **Implement auto-commit (changed, spektacular).** For a run with worktrees, a completion point always commits the code in the spec's worktrees on the spec branch, even with automatic commits off. In that case the project-side artifact commit is skipped, as automatic commits off already means. With automatic commits on, nothing changes. The commit-message request in the step output follows the same rule, so the agent is asked for a message whenever a commit will happen.
- **Implement step instructions (changed, spektacular).** A new shared block, used by the analyse, implement, test and verify steps, names each repo's code location: the worktree roots when present, the registry's roots otherwise. It requires every sub-agent to be handed those locations and to work only there, and explains that a new worktree holds only tracked files and is prepared in place. The finished step gains a merge instruction for a complete run started on its own that has worktrees, with the stop-on-refusal rule.
- **Implement skill and epic implement skill (changed, spektacular).** The implement skill adds a short section on worktree runs: where code lives, merging at the end with `implement merge`, and never resolving conflicts or switching branches. The epic skill's uncommitted-work bullet reads the dirty repo names.
- **Dependency classification (changed, spektacular).** The single function behind both the implement dependency check and `status` now reports a fully implemented dependency whose worktrees are not yet merged as unmet, describing it as implemented but not yet merged. Warn and refuse behaviour around it is unchanged.
- **Epic run status (changed, spektacular).** The uncommitted-work check is restricted to the repos the epic's plans touch, and it reports their names alongside the unchanged flag.
- **Documentation site (changed, docs).** The configuration reference documents the opt-out and the setup command. The workflow and epics pages describe runs started on their own building in worktrees and merging back, and the narrowed warning. The site changelog records the change.

## Data Structures & Interfaces

**Worktree opt-out (project configuration).** A new `implement` section with one key. It is read as on when the section or key is absent.

```go
type ImplementConfig struct {
    Worktrees bool `yaml:"worktrees"` // default true via NewDefault()
}
type Config struct { /* … */ Implement ImplementConfig `yaml:"implement"` }
```

```yaml
implement:
  worktrees: true   # false builds runs started on their own in the main checkouts
```

**Worktree setup command (repo configuration).** One optional field in the repo's own `repo.yaml`. It is also accepted by `repo add --data` and reported by `repo list`.

```go
type RepoConfig struct { /* … */ WorktreeSetup string `yaml:"worktree_setup,omitempty"` }
```

```yaml
# <repo>/.spektacular/repo.yaml
worktree_setup: npm ci
```

`repo add` input and `repo list` output each gain `worktree_setup` (string, optional). Registration only overwrites it when the input supplies one, as it does for description, role and tags.

**Setup runner (worktree manager).** A fakeable runner beside the git runner. It runs a shell command in a directory and returns its combined output and exit status.

```go
type SetupRunner interface {
    Run(dir, command string) (output string, err error) // sh -c <command>, cmd.Dir = dir
}
type Manager struct { /* ProjectRoot, Config, Repos, Git */ Setup SetupRunner } // nil => real runner
```

**New refusals.** Each is built with a message and a concrete next action:
- `worktree_unavailable`: a touched repo is not a git repository, or has no commits. The message names the repo, and the next action says to commit it, or to set `implement.worktrees: false`.
- `worktree_setup_failed`: a repo's setup command exited non-zero. The message names the repo, the command and its output. The next action says to fix the command or the repo, then rerun `implement new`.
- `implement merge` refusals reuse the existing merge codes (`epic_merge_conflict`, `epic_merge_touches_spektacular`, `worktree_failed`). A new `worktree_not_found` covers a spec with no worktrees.

**`implement merge` command.** Input `{"name": "<spec>"}`. On success it outputs the same shape as `epic merge`:

```json
{ "spec": "…", "merged": ["spektacular", "docs"], "removed": ["…"] }
```

On a conflict nothing is merged. The error names each repo with its conflicting paths, and the worktrees and branches are kept.

**Unmerged dependencies (status).** `status.Options` gains a hook that reports whether a spec's worktrees are still unmerged, meaning its worktree record exists. `Dependency` carries the result, and `Unmet` counts it.

```go
type Options struct { /* … */ Unmerged func(spec string) bool } // nil => nothing unmerged
type Dependency struct { /* Name, State, Progress, Description, Ready */ Unmerged bool }
// Unmet: State != StateImplemented || Unmerged
// Describe for an unmerged dependency: "is implemented but not yet merged"
```

The spec's own `State` is unchanged (it stays `implemented`), so the epic roll-up and completion are unaffected. Only readiness and dependency checks see the difference.

**Epic dirty repos (status run view).** Ported from PR #79:

```go
type RunSource struct { /* … */ Dirty func(repos []string) []string } // was func() bool
type EpicRun struct { /* … */ Dirty bool `json:"dirty"`; DirtyRepos []string `json:"dirty_repos"` } // never null
```

`dirty` keeps its name and type, and it is true exactly when `dirty_repos` is non-empty.

**Lane runs (workflow data).** Implement workflow data gains `lane: true` for a run started on its own that keeps its state in `.spektacular/workflows/implement-<spec>.json` and notes in `implement-<spec>.md`. `orchestrated` keeps meaning "hand back to an orchestrator" and implies a lane. The lane file format is unchanged.

**Step template data (implement steps).** No new keys for code locations: steps 02–05 reuse `has_worktree_roots`/`worktree_roots`. The finished step gains `merge_required` (bool), which is true when the run is not orchestrated, has worktree roots, and its plan has no open task.

## Implementation Detail

- **One worktree path for every run.** The worktree manager is the only code that creates or merges worktrees. `implement new` becomes its second caller, next to `epic worktree`, and the new `implement merge` becomes its second merge caller, next to `epic merge`. Both command pairs share helpers for building the manager, for listing touched repos and for rendering conflicts. A developer tracing worktree behaviour finds it all in the worktree package and two thin command files, not split between epic and implement code.
- **Creation is idempotent, and its presence is the signal.** The worktree record (from 000064) becomes the one source of truth for "this spec is being built in worktrees and is not merged yet". `implement new` creates worktrees only when there is no record. The step renderers, auto-commit, the finished step's merge instruction and the dependency check all read the same record, without git. This extends the 000064 pattern of "ask for a spec's worktree view by name" rather than introducing ambient state.
- **Setup is part of creation.** Preparing a worktree is a step inside the manager's creation of a new checkout. It is not a separate command or a step-template instruction, so epic and non-epic runs both get it, and a worktree is never handed out unprepared. It runs only for newly created checkouts. A failure removes that checkout's worktree so a retry starts clean. The setup runner sits beside the existing git runner as a second injectable dependency on the manager, following the same fake-in-tests pattern.
- **Commit points depend on worktrees as well as the setting.** Today the commit point is a pure function of the automatic-commit mode and the transition. It becomes a function of those plus whether the run has worktrees. The off-mode result for a worktree run is a code-only completion commit. The commit-message request in step output and the post-transition commit use that same decision, so they cannot drift apart.
- **Template guidance moves into one shared block.** Code-location and sub-agent guidance lives in one partial, included by every step that touches code, instead of being repeated per step or left to the skill. It renders two forms: worktree roots, or registry roots. The worktree form adds the tracked-files-only and dependency rule. This follows the existing two-form "Where the code lives" pattern in the read-plan step, and the triple-brace rule for paths.
- **The dependency classifier gains an input, not a second classifier.** Unmerged-ness is supplied to the status package through an options hook, alongside the existing lane and locate hooks. Every caller of the dependency classification (the implement check and the status views) wires the same hook, so the design's "implement and status always agree" guarantee holds by construction.
- **Dirty check narrows at the source.** Following PR #79, the run view hands the touched repos to the dirty probe and receives names back, instead of asking a yes/no question about every repo. The flag is derived from the names, so the two cannot disagree.
- **Skills stay thin.** The implement skill only explains the worktree lifecycle and the no-merge, no-branch-switch rule. Where code lives is carried by the step output, as 000064 established.

## Dependencies

- **Worktree package**: creates, records, merges and removes spec worktrees. Changed: a git-readiness refusal, setup on creation, and an injectable setup runner. Merge is unchanged.
- **Status package**: dependency classification and the epic run view. Changed: an unmerged hook on its options, and dirty repo names in the run view.
- **Autocommit package and step kit**: commit points and the commit-message request. Changed: worktree runs get code-only completion commits when automatic commits are off.
- **Implement step package and templates**: step rendering and instructions. Changed: a new code-locations partial in four steps and a merge instruction in the finished step.
- **Config package**: project and repo configuration. Changed: the `implement.worktrees` key, default on, and the repo `worktree_setup` field. No schema bump for either.
- **Repo package and `repo` commands**: registration and listing. Changed: they carry the setup command.
- **Output package**: error construction with `next_action`. Reused unchanged.
- **git (system binary)**: `worktree add/remove`, `merge-tree`, `merge`, `rev-parse`, `status`. No features beyond what epics already use.
- **POSIX shell (`sh`)**: runs a repo's setup command. Required only when a repo declares one.
- **Docs repo (`docs`)**: the configuration, how-it-works, epics and projects pages, plus its changelog. Updated in the same change, with no build dependency on the Go code.
- **Prior work that must already be in place**: 000064_epic-worktree-store-isolation (worktree record, code-only worktrees, `.spektacular` merge guard, split implement auto-commit). It is complete on the current branch. Nothing else must land first.
- **PR #79 (hivecommons/spektacular)**: the source of the `dirty_repos` code and tests, ported by hand because it does not apply cleanly to the current branch. The PR itself need not be merged first, and should be closed or rebased once this lands.
- **Design documents this plan was built on**: `epics-and-seeded-specs.md` from the `design` source. It fixes what a dependency is, when one counts as implemented, the warn-or-refuse handling under `epic.strict_dependencies`, and that the implement check and `status` classify dependencies through one function. This plan builds "implemented but not yet merged" into that function.

## Testing Approach

**Kinds of tests.** Go unit and integration tests carry most of the weight. They use real git repositories in temp directories, as the worktree, epic and auto-commit tests already do, with pinned identity. Template-contract tests pin the new step and skill wording with hand-copied phrase oracles. Config round-trip tests pin the new keys. The harbor implement suite is updated so its seeded project can hold worktrees, and it is run manually, as every step-template change requires.

**Most coverage, and why.**
- **Implement start and merge, end to end at command level.** This is where the new behaviour is assembled. A two-repo fixture (project repo plus a sibling registered repo) drives `implement new` → tick tasks → finished → `implement merge`. The tests assert:
  - a worktree on `spek/<spec>` exists in each touched repo, and none in an untouched one;
  - main checkouts have no modified or untracked files outside the project's Spektacular directory before the merge;
  - a clean merge brings the changes into both main lines and removes worktrees, branches and the record;
  - a conflicting main-line change in one repo refuses with the repo and paths named, moves neither main line, and keeps worktrees and branches;
  - with `implement.worktrees: false`, no worktree or branch is created and changes land in the main checkout. An `epic worktree` in the same project still creates worktrees;
  - a config with no `implement` key creates worktrees;
  - two specs started from the same main checkout are both in progress at once, each in its own lane with separate worktrees, and one spec's file changes are absent from the other's worktree and from the main checkout;
  - a run started on its own keeps its progress in its own lane, resumes from it, never renders the orchestrated hand-back, and uses the shared slot under the opt-out;
  - a non-git or commit-less touched repo is refused with the opt-out remediation;
  - an orchestrated child, a dry run, or an existing record creates nothing and runs no git, so the 000064 no-git test still holds;
  - a single-task run that leaves tasks open keeps its worktrees, and the finished step asks for no merge.
- **Auto-commit.** With automatic commits off, a completing worktree run leaves its code as commits on the spec branch, the step output asks for a commit message, and no commit is made in the main checkout. With automatic commits on, behaviour is unchanged from 000064. A run without worktrees and with automatic commits off commits nothing, as today.
- **Worktree manager setup.** A declared setup command's effect (a file it creates) is present in the new worktree before `Ensure` returns. A repo without one runs nothing. A failing command refuses with the repo, the command and its output, and removes the just-created worktree. Setup does not rerun for an existing worktree. The command is read from the main registration, not the worktree copy.
- **Dependency classification.** A dependency with every task complete and a worktree record is unmet. `implement new` refuses with the usual override offer, or refuses outright under strict dependencies. `status` reports the dependent as not ready with the same description. Once the record is gone the dependency is met.
- **Epic dirty repos.** Ported from PR #79: an untouched dirty repo does not raise the warning; a dirty touched repo does, and is named in `dirty_repos`; `dirty_repos` is `[]`, never null, when clean; repos sharing a checkout are reported by touched name only.
- **Templates.** Steps 02–05 render each worktree root and the "give every sub-agent these locations, work only there" and "tracked files only, prepare dependencies inside the worktree" phrases when roots are present, and the registry-roots form otherwise. The finished step renders the merge instruction only when `merge_required`. The implement skill carries the worktree lifecycle and no-merge, no-branch-switch phrases. Paths with `&` pass through unescaped.

**Load-bearing assertions.**
- Concurrent specs never share a checkout or a workflow slot.
- Main checkouts' code is untouched until merge.
- Merge is all or nothing.
- The spec branch always records the work.
- Opt-out equals today's behaviour.
- An unmerged dependency is unmet in both implement and status.

**Conventions followed.** Tests that execute `rootCmd` go through `resetRootCmd`/`runRootCmd`, and nothing depends on order or wall clock. Real-git fixtures pin identity. Nothing touches this repository's own `.spektacular/`. The full suite passes at the end of every task.

**Spec success metrics.**
- *Two specs implemented at the same time from two terminals with no interference, both merging back without manual cleanup when they do not conflict.* **Behavioural test**: two specs in progress at once from the same main checkout have their own lanes and isolated worktrees, and both merge cleanly and are cleaned up in sequence. **Manual — captured in the implementation test plan**: two real terminals running two specs at once, because true concurrency of two agent sessions cannot be exercised in a unit test.
- *In the next multi-repo epic run, no child or sub-agent runs its checks or edits in a main checkout.* **Behavioural test**: the code-touching steps render the locations and sub-agent rule, and the main-checkout-clean assertion holds for the command-level flow. **Manual — captured in the implementation test plan**: observation of the next real multi-repo epic run.
- *In the next multi-repo epic run, every spec's worktrees are ready to build and test before its first task, with no ad hoc dependency installation.* **Behavioural test**: the setup command's effect is present after `epic worktree` and after `implement new`. **Manual — captured in the implementation test plan**: the same observation in a real epic run with a repo declaring `npm ci`.
- *The epic's uncommitted-work warning is raised only when a repo the epic builds is dirty, and status names it.* **Behavioural test**: the dirty-repos tests above.

**Manual reviews.**
- **Manual — captured in the implementation test plan**: review the rendered docs pages (configuration, how it works, epics, projects): site build, type check and a visual check of the new keys and sections.
- **Manual — captured in the implementation test plan**: run the harbor implement suite once, since the implement step templates and the default worktree behaviour change.
- **Manual — captured in the implementation test plan**: after `make install-local` between workflows, one real standalone run on a two-repo project, confirming worktrees, setup, merge and cleanup.

**Deliberate gaps.** No automated test of two simultaneous agent processes. Sequential starts from the same checkout prove the isolation property, and the real concurrency is a manual check. No harbor epic suite, as in 000064.

## Milestones & Tasks

### Milestone 1: Epic worktrees are ready to build, sub-agents know where the code is, and the dirty warning names the right repos

**What changes**: A repo can declare a setup command in its own configuration, such as installing its dependencies. Every new worktree of that repo is prepared with it before work starts, and a failing command refuses the run with the reason. The steps that analyse, implement, test and verify a task now name each repo's code location. They tell the agent to hand those locations to every sub-agent and to keep all edits, builds and checks there, and that a new worktree holds only tracked files and is prepared in place. An epic's uncommitted-work warning considers only the repos its specs' plans touch, and status names the dirty ones. Implementing a spec on its own does not use worktrees yet; this milestone fixes the three problems found in epic runs on their own.

**Validation point**: Worktree tests show a declared setup command's effect in a new worktree, no setup for repos without one, and a refused, cleaned-up creation when the command fails. Step-rendering tests show the code-location and sub-agent guidance in all four code-touching steps, in both the worktree and registry forms. Status tests show `dirty_repos` naming only dirty touched repos and `dirty` unchanged in type. The full test suite passes.

#### - [x] Task: Let a repo declare a worktree setup command
**Id:** 98c6a685-5c90-4986-85bb-c29111055488
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add an optional setup command to a repo's own configuration, so it travels with the repo to every project that registers it. `repo add` can set it alongside description, role and tags, and `repo list` reports it. The repo-management skill explains what it is for, for example installing dependencies a fresh worktree lacks.

*Technical detail:* [context.md#task-let-a-repo-declare-a-worktree-setup-command](./context.md#task-let-a-repo-declare-a-worktree-setup-command)

**Acceptance criteria**:
- [x] A repo's configuration file can hold a setup command, and a file without one loads exactly as before.
- [x] Registering a repo with a setup command records it in the repo's own configuration, and re-registering without one leaves an existing command in place.
- [x] Listing repos shows each repo's setup command when it has one.
- [x] The repo-management skill describes the setup command and when to declare one.

#### - [x] Task: Run a repo's setup command in each new worktree
**Id:** acc28f89-d6e3-442f-915c-7f60900a9e83
**Repo:** spektacular
**Depends on:**
- 98c6a685-5c90-4986-85bb-c29111055488 — Let a repo declare a worktree setup command
**Execution:** agent

When a spec's worktree is newly created, run the declared setup command of every touched repo in that worktree before handing it out. The command is read from the repo's registration in the project, never from the worktree's copy. If it fails, the just-created worktree is removed so a retry starts clean, and creation is refused with the repo, the command and its output.

*Technical detail:* [context.md#task-run-a-repos-setup-command-in-each-new-worktree](./context.md#task-run-a-repos-setup-command-in-each-new-worktree)

**Acceptance criteria**:
- [x] After a spec's worktrees are created, the effect of each touched repo's setup command is present in that repo's worktree.
- [x] A repo with no setup command gets no setup, and an existing worktree is never set up again.
- [x] A failing setup command refuses creation with a message naming the repo and the command's failure, and with a next action. The failed worktree does not remain.
- [x] Epic worktree creation gets the same setup.

#### - [x] Task: Tell code-touching steps where the code lives and how to brief sub-agents
**Id:** 8fd1d9e9-60ce-4bc7-9d13-17df7552b1d2
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add one shared block to the steps that analyse, implement, test and verify a task. It lists each touched repo's code location: the spec's worktree roots when it has worktrees, otherwise the roots the repo registry reports. It requires every sub-agent the agent launches to be given those locations and to edit, build and check only there. For worktrees it also explains that a new worktree has only tracked files, and that dependencies are prepared inside it, never installed into or shared from a main checkout.

*Technical detail:* [context.md#task-tell-code-touching-steps-where-the-code-lives-and-how-to-brief-sub-agents](./context.md#task-tell-code-touching-steps-where-the-code-lives-and-how-to-brief-sub-agents)

**Acceptance criteria**:
- [x] With worktrees, each of the four steps names every touched repo's worktree location and states that every sub-agent must be given those locations and work only in them.
- [x] With worktrees, the same steps state that a new worktree holds only tracked files and that dependencies are prepared inside it, not taken from a main checkout.
- [x] Without worktrees, the steps still tell the agent to pass each repo's registered location to every sub-agent.
- [x] Locations containing special characters reach the agent unchanged.

#### - [x] Task: Limit the epic's uncommitted-work warning to repos its plans touch
**Id:** 42d0bb3e-b6a4-4a1b-8640-b5d58418bd53
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Port the dirty-repos change from PR #79: the epic run view checks only the repos its specs' plans touch and reports the dirty ones by name, next to the existing flag. Update the epic implement skill's uncommitted-work guidance to name the dirty repos, written fresh to the current project-root rule.

*Technical detail:* [context.md#task-limit-the-epics-uncommitted-work-warning-to-repos-its-plans-touch](./context.md#task-limit-the-epics-uncommitted-work-warning-to-repos-its-plans-touch)

**Acceptance criteria**:
- [x] An untouched registered repo with uncommitted changes does not raise the epic's warning.
- [x] A touched repo with uncommitted changes raises it, and the epic's status names that repo.
- [x] The existing flag keeps its name and type, and the list of dirty repos is empty rather than missing when nothing is dirty.
- [x] The epic implement skill tells the agent to name the dirty repos to the user.

### Milestone 2: A spec built in worktrees always has committed work to merge, can be merged on its own, and blocks its dependents until it is

**What changes**: Whenever a spec is built in worktrees, the completed run commits its code on the spec's branch, even with automatic commits turned off. A new `implement merge` command merges a spec's worktrees back into every touched repo's main line, all or nothing. It refuses and names the conflicting paths on a conflict, and removes the worktrees and branches after a clean merge. A dependency whose tasks are all complete but whose worktrees are not merged yet now counts as unmet. Implement warns or refuses on it exactly as for any other unmet dependency, and status shows its dependents as not ready. Epic runs benefit at once: a child run with automatic commits off no longer leaves an unmergeable, uncommitted worktree.

**Validation point**: Auto-commit tests show code commits on the spec branch with automatic commits off and no main-checkout commit. Command tests show `implement merge` merging two repos and cleaning up, and refusing a conflict without moving either main line. Dependency tests show an unmerged dependency reported as unmet by both `implement new` and `status`, met again after the merge. The full test suite passes.

#### - [ ] Task: Always commit a worktree run's code on the spec branch
**Id:** 182b302a-301a-4185-b07c-6d840eb995d6
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

When an implement run has worktrees and reaches a completion point, commit its code in the spec's worktrees, even with automatic commits off. With automatic commits off, nothing is committed in the main checkouts, as today. The step before the completion point asks the agent for a commit message whenever such a commit will happen. Runs without worktrees, and runs with automatic commits on, behave exactly as before.

*Technical detail:* [context.md#task-always-commit-a-worktree-runs-code-on-the-spec-branch](./context.md#task-always-commit-a-worktree-runs-code-on-the-spec-branch)

**Acceptance criteria**:
- [ ] With automatic commits off, a completed run with worktrees leaves its code changes as commits on the spec's branch, and its worktrees have no uncommitted changes.
- [ ] With automatic commits off, the main checkouts get no commit from that run.
- [ ] The agent is asked for a commit message before the transition that will commit.
- [ ] A run without worktrees and with automatic commits off commits nothing, as before.

#### - [ ] Task: Add an implement merge command
**Id:** 5f5fb7b6-4299-4ada-bc3a-1d38bc18327f
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add `implement merge`, which merges a spec's worktree branches back into every touched repo's main line through the same all-or-nothing merge `epic merge` uses. On a conflict it refuses, names each repo and its conflicting paths, and merges nothing. After a clean merge, the worktrees, branches and record are removed. A spec with no worktrees is refused with a clear next action.

*Technical detail:* [context.md#task-add-an-implement-merge-command](./context.md#task-add-an-implement-merge-command)

**Acceptance criteria**:
- [ ] Merging a spec that touches two repos, with no conflicts, brings its changes into both main lines and removes its worktrees and branches.
- [ ] When one repo's main line has a conflicting change, the merge is refused naming that repo and the conflicting paths, neither main line moves, and the worktrees and branches remain.
- [ ] The Spektacular-directory guard and the other merge prechecks apply exactly as for `epic merge`.
- [ ] Merging a spec with no worktrees is refused with a next action.

#### - [ ] Task: Count an implemented but unmerged dependency as unmet
**Id:** 7462c057-e300-4eb1-89a8-480dc62266e3
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Teach the single dependency classification shared by the implement check and status that a dependency with every task complete, but whose worktrees still exist, is not met yet, and describe it as implemented but not yet merged. Wire every caller to the same unmerged signal, so implement and status always agree. Warn and refuse behaviour is unchanged.

*Technical detail:* [context.md#task-count-an-implemented-but-unmerged-dependency-as-unmet](./context.md#task-count-an-implemented-but-unmerged-dependency-as-unmet)

**Acceptance criteria**:
- [ ] Starting an implement run for a spec whose dependency is complete but unmerged names that dependency as unmet and offers the usual override.
- [ ] Under strict dependencies the same run is refused outright.
- [ ] Status reports the dependent spec as not ready, with the same description of the dependency.
- [ ] Once the dependency is merged, the dependent can start without a warning.

### Milestone 3: Every implement run started on its own builds in its own worktrees and merges back when it finishes

**What changes**: Starting an implement run on its own now creates the spec's worktrees in every repo its plan touches, prepared by any setup command, and the whole run builds there. Each such run keeps its progress in a lane of its own, so two specs can therefore be implemented side by side from two terminals without seeing each other's changes. When the run completes the plan, the finished step has the agent run `implement merge`. A conflict is reported for the user to resolve, and the agent never resolves it or switches branches. A single-task run that leaves tasks open keeps its worktrees for the next run. This is the default, including in existing projects. Setting `implement.worktrees: false` restores today's behaviour exactly. A repo that cannot hold a worktree, because it is not a git repo or has no commits yet, refuses the run with a way out.

**Validation point**: End-to-end command tests on a two-repo fixture show two runs in progress at once in their own lanes, worktrees only in touched repos, main checkouts clean until the merge, a clean merge with cleanup, an all-or-nothing conflict refusal, isolation between two specs, the opt-out restoring main-checkout builds while epics still use worktrees, an absent key defaulting on, and the non-git refusal. Template and skill contract tests pass, the harbor implement fixture is updated, and the full test suite passes.

#### - [ ] Task: Add the implement worktrees opt-out setting
**Id:** 5daa0cc5-47e5-4cf6-a2e3-e81c397be977
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add an `implement` section with a `worktrees` setting that is on by default. A configuration that does not mention it reads as on, so existing projects need no change and no migration. New projects get it written out with the default.

*Technical detail:* [context.md#task-add-the-implement-worktrees-opt-out-setting](./context.md#task-add-the-implement-worktrees-opt-out-setting)

**Acceptance criteria**:
- [ ] A configuration without the setting reads worktrees as on.
- [ ] Setting it to false is written to and read back from the configuration file as false.
- [ ] The project's settings format version is unchanged.

#### - [ ] Task: Keep each run started on its own in its own lane
**Id:** e74f4569-7cae-4327-bde3-a2b9a2f6793c
**Repo:** spektacular
**Depends on:**
- 5daa0cc5-47e5-4cf6-a2e3-e81c397be977 — Add the implement worktrees opt-out setting
**Execution:** agent

With worktrees on, an implement run started on its own keeps its progress and working notes in a lane of its own, keyed by spec, as epic children already do, instead of the project's single shared workflow slot. A second run for a different spec can then start from another terminal instead of being offered the first run's resume. The run stays a normal interactive run: it asks the user directly and does not hand back to an orchestrator. With the opt-out, runs use the shared slot exactly as today.

*Technical detail:* [context.md#task-keep-each-run-started-on-its-own-in-its-own-lane](./context.md#task-keep-each-run-started-on-its-own-in-its-own-lane)

**Acceptance criteria**:
- [ ] With worktrees on, starting implement runs for two different specs succeeds for both, and each continues independently with its own progress and notes.
- [ ] Starting a run for a spec whose own run is already in progress offers to resume that run, and a run with no name given still offers to resume a run in the shared slot.
- [ ] A run in its own lane behaves interactively, never handing back to an orchestrator, and its lane is cleaned up when it finishes.
- [ ] With worktrees turned off, runs use the shared slot exactly as before.

#### - [ ] Task: Create a spec's worktrees when an implement run starts on its own
**Id:** 36a74c31-cac1-4164-9f16-238652c9efa3
**Repo:** spektacular
**Depends on:**
- acc28f89-d6e3-442f-915c-7f60900a9e83 — Run a repo's setup command in each new worktree
- 5daa0cc5-47e5-4cf6-a2e3-e81c397be977 — Add the implement worktrees opt-out setting
**Execution:** agent

When an implement run is started on its own with worktrees on, create the spec's worktrees in every repo its plan touches before the workflow starts, so every step builds there. An orchestrated child, a dry run, an existing set of worktrees and the opt-out all skip creation. If a touched repo is not a git repository or has no commits, refuse with a way out: commit it, or turn worktrees off.

*Technical detail:* [context.md#task-create-a-specs-worktrees-when-an-implement-run-starts-on-its-own](./context.md#task-create-a-specs-worktrees-when-an-implement-run-starts-on-its-own)

**Acceptance criteria**:
- [ ] A run started on its own whose plan touches two repos has a worktree on the spec's branch in each, and none in an untouched repo.
- [ ] The run's instructions name the worktree locations, not the main checkouts.
- [ ] With worktrees turned off, no worktree or branch is created.
- [ ] A later run for the same spec reuses the existing worktrees.
- [ ] A touched repo that is not a git repository, or has no commits, refuses the run with the repo named and the opt-out offered.

#### - [ ] Task: Merge back at the end of a run started on its own
**Id:** e23fd89a-1781-474f-be06-3b4be8216caa
**Repo:** spektacular
**Depends on:**
- 5f5fb7b6-4299-4ada-bc3a-1d38bc18327f — Add an implement merge command
- 36a74c31-cac1-4164-9f16-238652c9efa3 — Create a spec's worktrees when an implement run starts on its own
**Execution:** agent

When a run started on its own finishes with its plan complete and has worktrees, the finished step tells the agent to run `implement merge`. On a refusal it reports the repos and paths to the user and stops, never resolving the conflict, rebasing or switching branches. A task run that leaves tasks open is told its worktrees are kept for the next run. The implement skill gains a short section on this worktree lifecycle.

*Technical detail:* [context.md#task-merge-back-at-the-end-of-a-run-started-on-its-own](./context.md#task-merge-back-at-the-end-of-a-run-started-on-its-own)

**Acceptance criteria**:
- [ ] A complete run started on its own with worktrees is told to merge with `implement merge`, and to stop and report on a refusal.
- [ ] An orchestrated run, a run without worktrees, and a task run with open tasks are not told to merge.
- [ ] The implement skill states that the agent never resolves a conflict and never merges, rebases or switches branches on its own initiative.

#### - [ ] Task: Prove runs started on their own end to end
**Id:** 541740f7-1abc-48da-adaa-93aaf306d817
**Repo:** spektacular
**Depends on:**
- 182b302a-301a-4185-b07c-6d840eb995d6 — Always commit a worktree run's code on the spec branch
- 7462c057-e300-4eb1-89a8-480dc62266e3 — Count an implemented but unmerged dependency as unmet
- e23fd89a-1781-474f-be06-3b4be8216caa — Merge back at the end of a run started on its own
- e74f4569-7cae-4327-bde3-a2b9a2f6793c — Keep each run started on its own in its own lane
**Execution:** agent

Add command-level tests that drive whole runs started on their own on a real two-repo fixture: start, build, finish, merge. They cover isolation between two specs, a clean main checkout until merge, the conflict refusal, the opt-out, an absent setting and automatic commits off. Keep the harbor implement suite valid by giving its seeded project the opt-out, so it keeps exercising today's main-checkout behaviour.

*Technical detail:* [context.md#task-prove-runs-started-on-their-own-end-to-end](./context.md#task-prove-runs-started-on-their-own-end-to-end)

**Acceptance criteria**:
- [ ] Two specs planned against the same repo, both in progress at once, get separate worktrees and separate progress, and one spec's changes appear in neither the other's worktree nor the main checkout until it merges.
- [ ] Before the merge, no main checkout has modified or untracked files outside the project's Spektacular directory.
- [ ] A clean two-repo run ends with both main lines holding its changes and no worktrees or branches left, including with automatic commits off.
- [ ] A conflict in one repo moves neither main line and keeps the worktrees and branches.
- [ ] With worktrees turned off, changes land in the main checkout and an epic in the same project still uses worktrees.
- [ ] The harbor implement suite's project is configured so the suite still runs.

### Milestone 4: The documentation describes worktree runs, the setup command and the narrowed warning

**What changes**: The documentation site's configuration reference documents the `implement.worktrees` opt-out and the per-repo `worktree_setup` command. The workflow and epics pages explain that every implement run builds in worktrees and merges back at the end, how conflicts are handled, and that the uncommitted-work warning covers only the repos an epic builds, with the dirty repos named. The site changelog records the change.

**Validation point**: The site builds and type-checks cleanly, page bodies contain no layout HTML, there are no em dashes in the new text, and a visual check of the changed sections shows the new keys and behaviour.

#### - [ ] Task: Document the worktree setting and setup command in the configuration reference
**Id:** 3ce7e690-c7c0-4f3d-8dcf-535e1615545c
**Repo:** docs
**Depends on:**
- 98c6a685-5c90-4986-85bb-c29111055488 — Let a repo declare a worktree setup command
- 5daa0cc5-47e5-4cf6-a2e3-e81c397be977 — Add the implement worktrees opt-out setting
**Execution:** agent

Add the `implement.worktrees` opt-out to the project configuration example and keys, and the `worktree_setup` command to the repo configuration example and keys. Mention the setup command where the projects page explains how configuration is split.

*Technical detail:* [context.md#task-document-the-worktree-setting-and-setup-command-in-the-configuration-reference](./context.md#task-document-the-worktree-setting-and-setup-command-in-the-configuration-reference)

**Acceptance criteria**:
- [ ] The configuration reference describes the worktree opt-out, its default, and that epics always use worktrees.
- [ ] The configuration reference describes the per-repo setup command, when it runs, and what happens when it fails.
- [ ] The projects page points to the setup command as part of a repo's own configuration.
- [ ] The site builds and type-checks, and the new text has no em dashes or layout HTML.

#### - [ ] Task: Document worktree runs, merging back and the narrowed warning
**Id:** 1c1de5ef-50c0-481b-9685-7ac9b868f4c2
**Repo:** docs
**Depends on:**
- 42d0bb3e-b6a4-4a1b-8640-b5d58418bd53 — Limit the epic's uncommitted-work warning to repos its plans touch
- e23fd89a-1781-474f-be06-3b4be8216caa — Merge back at the end of a run started on its own
- 3ce7e690-c7c0-4f3d-8dcf-535e1615545c — Document the worktree setting and setup command in the configuration reference
**Execution:** agent

Describe on the how-it-works page that every implement run builds in its own worktrees and merges back when it finishes, all or nothing, with conflicts left to the user. On the epics page, describe the uncommitted-work warning covering only the repos an epic builds, add `dirty_repos` to the status example, and note the setup command. Add the site changelog entry.

*Technical detail:* [context.md#task-document-worktree-runs-merging-back-and-the-narrowed-warning](./context.md#task-document-worktree-runs-merging-back-and-the-narrowed-warning)

**Acceptance criteria**:
- [ ] The implement stage on the how-it-works page explains worktrees for runs started on their own, the merge at the end, conflict handling and the opt-out.
- [ ] The epics page explains the narrowed uncommitted-work warning and shows the dirty repos in the status example.
- [ ] The site changelog has an entry for this change naming the pages updated.
- [ ] The site builds and type-checks, and the new text has no em dashes or layout HTML.

## Open Questions

None. Every uncertainty found during planning was resolved with the user (non-git refusal, the merge trigger, single-task runs, per-spec lanes for runs started on their own) or recorded as a judgement call in the assumptions log. Assumptions that could prove wrong during implementation are listed in `research.md#open-assumptions`. If one does not hold (for example, something other than a successful merge removes a worktree record, or an existing test fixture cannot take the opt-out), STOP and ask the user before working around it.

## Out of Scope

- **Scheduling or ordering several runs started on their own** (spec non-goal). Each run checks its own dependencies, but running specs in dependency order remains the epic orchestrator's job.
- **Worktrees for the spec or plan workflows** (spec non-goal). Only implementing builds in worktrees, and plan lanes are unchanged.
- **Changing how dependencies between specs are declared, or a dependency on a spec outside an epic** (spec non-goal). The design's graph and validation are untouched. This plan only adds "implemented but not yet merged" to how a dependency is classified.
- **Automatic merging by the CLI.** The merge runs when the finished step instructs it, and is never folded into the `finished` transition (user decision).
- **Falling back to the main checkout for a repo that cannot hold a worktree.** Such a repo refuses the run instead (user decision). Turning worktrees off is the way to build there.
- **Committing project artifacts with automatic commits off.** A worktree run commits only its code on the spec branch. Plans, changelogs and progress in the project stay uncommitted, as the off setting means today.
- **PR #79's template and skill wording and its main-checkout baseline check.** Only its dirty-repos code and tests are reused. The PR should be closed or rebased after this lands.
- **A settings format bump or migration.** Neither the opt-out nor the setup command needs one. Bringing this repository's own configuration or skills up to date is done by installing the built binary between workflows, not by any task here.
- **Windows support for setup commands.** They run through a POSIX shell.
- **A harbor suite for worktree runs or epics.** Worktree runs are covered by Go flow tests, and the harbor implement suite keeps exercising the opt-out path.

## Changelog

### 2026-10-07 — Task: Let a repo declare a worktree setup command

**What was done**: A repo's own `repo.yaml` can now declare `worktree_setup`, a shell command for preparing a new worktree. `repo add --data` accepts it and writes it only when given, so re-registering without it keeps the existing command. `repo list` reports it. The repo-management skill explains what it is for and when to declare one.

**Deviations**: None. The guided add flow (`internal/steps/repo/registration.go`) was deliberately not extended; the plan scopes the field to `repo add --data` and `repo list`.

**Files changed**:
- `spektacular: internal/config/repo.go`
- `spektacular: internal/repo/register.go`
- `spektacular: cmd/repo.go`
- `spektacular: templates/skills/workflows/spek-manage-repos/SKILL.md`
- `spektacular: internal/config/repo_test.go`
- `spektacular: internal/repo/register_test.go`
- `spektacular: cmd/repo_test.go`
- `spektacular: templates/guided_add_skill_test.go`

**Discoveries**: `repoConfigDescriptiveFieldsEqual` gates whether `Register` rewrites `repo.yaml`, so any new field registration writes must be added there too or a change to only that field is silently dropped. The skill text for spek-manage-repos is asserted in `templates/guided_add_skill_test.go`.

### 2026-10-07 — Task: Run a repo's setup command in each new worktree

**What was done**: `worktree.Manager` gained an injectable `SetupRunner` (a `sh -c` runner by default). When `Ensure` creates a new worktree, each touched repo's `worktree_setup` command, read from its main registration, runs in that repo's code root inside the worktree. A failure removes the worktree (and its branch when this call created it), writes no record, and refuses with `worktree_setup_failed`. `epic worktree` gets this through a swappable `worktreeSetup` var.

**Deviations**: `worktreeManager()` sets `Setup: worktreeSetup` explicitly rather than leaving it nil, so tests can swap the runner.

**Files changed**:
- `spektacular: internal/worktree/setup.go`
- `spektacular: internal/worktree/worktree.go`
- `spektacular: cmd/epic_worktree.go`
- `spektacular: internal/worktree/worktree_test.go`
- `spektacular: cmd/epic_worktree_test.go`

**Discoveries**: When setup fails in a later checkout, worktrees already created for earlier checkouts in the same `Ensure` call (with their branches) stay in place; only the failing checkout's worktree is discarded. A retry reuses them without re-running their (already successful) setup.

### 2026-10-07 — Task: Tell code-touching steps where the code lives and how to brief sub-agents

**What was done**: A new partial, `templates/partials/implement-code-locations.md`, renders a "Where the code lives" block in the analyze, implement, test and verify steps. With worktree roots it lists each repo's root (triple-braced), requires every sub-agent to be given those locations and work only there, and explains that a new worktree holds only tracked files and dependencies are prepared inside it. Without roots it points at `repo list` and still requires passing each root to sub-agents.

**Deviations**: The partial is included at the end of each step's Step 1 (just before Step 2), not directly after `implement-current-task`, so it does not split Step 1's instructions. The 000064 test `TestWhereTheCodeLivesPreambleRenderedOnceByReadPlan`, which asserted steps 02–05 did *not* mention code locations, was inverted and renamed to `TestWhereTheCodeLivesPreambleRenderedByReadPlan`.

**Files changed**:
- `spektacular: templates/partials/implement-code-locations.md`
- `spektacular: templates/steps/implement/02-analyze.md`
- `spektacular: templates/steps/implement/03-implement.md`
- `spektacular: templates/steps/implement/04-test.md`
- `spektacular: templates/steps/implement/05-verify.md`
- `spektacular: internal/steps/implement/steps_test.go`

**Discoveries**: No harbor oracle pins the text of steps 02–05. Partials under `templates/partials/` resolve automatically by path from step templates.

### 2026-10-07 — Task: Limit the epic's uncommitted-work warning to repos its plans touch

**What was done**: Ported PR #79's code by hand. `status.RunSource.Dirty` now takes the epic's touched repos and returns the dirty ones; `EpicRun` gained `dirty_repos` (never null), and `dirty` is true exactly when it is non-empty. `statusRunSource` resolves only the touched repos' checkouts. The epic implement skill's uncommitted-work bullet names `dirty_repos`, written fresh for the post-000064 project-root rule.

**Deviations**: `statusRunSource`'s Dirty returns nil early for an empty touched list. The PR's isolation-baseline and worktree-cwd template changes were not ported (as planned).

**Files changed**:
- `spektacular: internal/status/run.go`
- `spektacular: cmd/status.go`
- `spektacular: templates/skills/workflows/spek-implement-epic/SKILL.md`
- `spektacular: cmd/status_dirty_test.go`
- `spektacular: cmd/status_test.go`
- `spektacular: internal/status/run_test.go`
- `spektacular: templates/implement_epic_skill_test.go`

**Discoveries**: `templates/implement_epic_skill_test.go` keeps an allow-list of the sentences where the epic skill may ask the user something; rewording a question in the skill requires updating that list too.
