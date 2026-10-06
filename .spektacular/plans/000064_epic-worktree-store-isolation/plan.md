---
created_date: "2026-10-06"
document_status: final
closed_date: "2026-10-06"
---

# Plan: 000064_epic-worktree-store-isolation

<!-- Metadata -->
<!-- Created: 2026-10-06T14:55:55Z -->
<!-- Commit: 32fcdb0 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

During an epic run, each spec's child agent currently runs Spektacular from inside the spec's git worktree. So plan ticks, changelog records and progress notes land in the worktree's copy of the project's records, and reach the project only when git merges the branch. This plan makes the main project the only place Spektacular runs. Worktrees hold code only. The implement workflow itself tells the agent where each repo's code lives. Commits split between worktree code and project artifacts. `epic merge` refuses any branch that changes Spektacular's files. Teams running an epic can follow progress in the project as it happens, and each spec is built by the unchanged standard implement workflow.

## Conventions

- **Store files are written through the CLI, never with file tools** — the whole feature exists to make epic children honour this; artifact writes must reach the main project's stores through Spektacular, and no code may build a store path by hand.
- **Error messages must describe the problem and suggest remediation** — the new merge refusal for `.spektacular/` changes must carry a concrete `next_action`.
- **A plan never changes the active skills and configuration** — this plan edits skill and step templates; verification uses Go tests and throwaway projects, never a re-init or migrate of this repo.
- **Tests must not depend on execution order** — new cmd tests that execute `rootCmd` go through `resetRootCmd`/`runRootCmd`, and real-git fixtures pin identity.
- **Passing tests are required before calling work done** — the full Go test suite must pass at the end of every task.
- **docs: No em dashes** — the epics page update in the docs repo uses no em dashes.
- **docs: MDX authoring conventions** — the epics page edit stays prose and lists only, with no layout HTML; the site build and type check verify it.
- **docs: Plans must sketch content structure** — the docs task carries a content example for the changed bullets.

## Architecture & Design Decisions

**Shape.** During an epic run, the main project is the only place where Spektacular runs. Every `spektacular` command, the orchestrator's and each child's, runs from the main project root, so `projectRoot()` (the current directory, `cmd/root.go:397-403`) always resolves to the main project. Stores, lanes, lane notes and the commit-message scratch file all resolve there without any store-path changes. Spec worktrees become code-only. `epic worktree` stops writing the repo overlay into the project worktree's `.spektacular/`. Instead it records the spec's repo-to-code-root map once, in the main project, beside the spec's worktrees (`.spektacular/worktrees/<spec>/`). That location is already excluded from git by `Ensure`, and it sits outside every worktree. `repo.New` no longer applies an overlay implicitly. Code that needs a spec's view of the repos asks for it explicitly with a spec-scoped constructor that reads that record. That code is the implement workflow's code-root rendering and implement auto-commit.

**How the child learns where code lives.** `implement new` and `implement goto` read the spec's worktree record from the main project and pass the code roots into the implement workflow config. This is a plain file read with no git, so the zero-git, no-persisted-roster contract (`cmd/implement_test.go:296-356`) holds. The read-plan step's "Where the code lives." block and the feature-changelog step then render one of two forms. When the spec has worktrees, they list each touched repo's worktree root, under its own template key, since rosters keyed `repos` are banned (`templates/repo_source_test.go:67-86`). When it has none, they render today's `repo list` sentence unchanged. The child follows the standard `spek-implement` skill with no epic-specific instructions. The orchestrator's child prompt shrinks to "run Spektacular from `<project root>`", and the skill's orchestrated section stops mentioning worktrees.

**Commits, status and merge.** Implement auto-commit for a spec with worktrees splits by location. Code is committed in the spec's worktrees, on `spek/<spec>`, through the spec-scoped targets. Only that spec's artifacts are committed in the main checkouts: its plan documents, spec, changelog records and lane files. These are path-scoped commits taken under the existing commit lock, mirroring how orchestrated plan lanes already commit (`cmd/autocommit.go:190-198, 247-269`). Standalone implement keeps `CommitAll` unchanged. `status` stops reading the worktree's `.spektacular`. It takes the spec's implement lane and its "finished" evidence (ticked tasks, final changelog) from the main project, and still reports the spec's worktree as the run's `root`. `epic merge` gains a fourth per-repo precheck before the dry run. It lists the paths the spec's branch changes under `.spektacular/`, and if there are any it refuses with a new error code that names the paths and gives a remediation, and merges nothing.

**Why this direction.** It satisfies every recorded decision with the smallest moving set. Lanes are already per spec in one directory (`internal/workflow/lane.go:15-66`). The overlay has exactly one writer and one reader (`internal/worktree/worktree.go:300-310`, `internal/repo/set.go:75-92`). The merge prechecks already have the per-repo shape the guard needs (`worktree.go:522-541`). Discovering worktrees with git at implement time would break the zero-git contract. Deriving roots from the path convention alone needs git top-level resolution. Committing the main checkout wholesale would sweep other specs' in-flight work into one commit. The evidence is in `research.md#alternatives-considered-and-rejected`. In line with the store-access convention, nothing here constructs a store path by hand: store documents are still reached only through the project store, and the worktree record is a worktree property, not a store document. In line with the active-install convention, all verification uses Go tests and throwaway projects, never this repository's own `.spektacular/`.

## Component Breakdown

- **Worktree manager (changed).** It still creates, lists and merges a spec's worktrees, and it now owns the spec's **worktree record**. It writes the record in the main project when worktrees are created, reads it back on request, and deletes it when the spec is merged. It no longer writes anything into a worktree's `.spektacular/`. Its merge gains a precheck that refuses any spec branch that changes `.spektacular/` content in any repo.
- **Worktree record (new, replaces the in-worktree overlay).** A small per-spec file in the main project, beside the spec's worktrees, mapping each repo the spec touches to its code root inside the worktree. The worktree manager writes it. The implement commands and the spec-scoped repo view read it. It is git-excluded and never inside a worktree. It is new because the implement commands need the roots without running git, and nothing else holds them.
- **Repo set (changed).** Building the repo set no longer applies an overlay implicitly, so every ordinary consumer (`repo list`, knowledge sources, repo-routed changelog stores, status) sees the registered locations of the main project. A spec-scoped constructor applies a worktree record explicitly, for the two consumers that need a spec's view: code-root rendering and implement auto-commit.
- **Implement commands (changed).** `implement new` and `implement goto` look up the spec's worktree record from the main project and hand any code roots to the workflow through its config. They never use git and never persist the roots into workflow data.
- **Implement step rendering (changed).** The read-plan step's "Where the code lives." block and the feature-changelog step render the spec's worktree code roots when the config carries them, and today's `repo list` direction otherwise. No other step changes.
- **Implement auto-commit (changed).** For a spec with a worktree record, it commits code in the spec's worktrees and commits only that spec's artifacts in the main checkouts, path-scoped and under the commit lock. Without a record it behaves as today.
- **Epic status (changed).** It reads a spec's implement lane and its finished evidence from the main project only, and still reports the spec's worktree as the run's root and awaiting-merge state.
- **Epic implement orchestrator skill (changed).** The child prompt tells the child to run Spektacular from the project root and follow `spek-implement`. It no longer sends the child into a worktree or to `repo list` there. Its resume and start wording follows the same change.
- **Implement skill, orchestrated section (changed).** It drops "from the worktree you were given" and the worktree framing, and otherwise stays the standard skill.
- **Epic docs page (changed, docs repo).** "Implement this epic" says worktrees hold only code and that plans and progress can be followed in the project during a run.

## Data Structures & Interfaces

**Worktree record** is the per-spec file in the main project. It replaces `repo.Overlay`, and its shape is the overlay's, moved:

```go
// worktree package
type Record struct {
    Spec  string            `json:"spec"`
    Repos map[string]string `json:"repos"` // registered repo name -> absolute code root in the spec's worktree
}

func (m Manager) Record(spec string) (Record, bool, error) // no git; (zero, false, nil) when absent
```

It is written by `Ensure`, removed by `Merge`, and read by `Record` (and by a package-level helper that takes the project root, so cmd can read it without building a manager).

**Spec-scoped repo view.** `repo.New` loses its implicit overlay. A sibling constructor applies a location map explicitly:

```go
func New(cfg config.Config, projectRoot string, git GitRunner) (*Set, error)            // registered locations only
func NewWithLocations(cfg config.Config, projectRoot string, git GitRunner, locs map[string]string) (*Set, error)
```

`OverlayFile`, `Overlay` and `readOverlay` are removed.

**Code roots in the workflow config.** `workflow.Config` gains a runtime-only field. It is not persisted, like the other config fields:

```go
type CodeRoot struct{ Repo, Root string }
type Config struct { /* … */ CodeRoots []CodeRoot } // empty unless the spec has a worktree record
```

Implement steps expose it to templates as `has_worktree_roots` (bool) and `worktree_roots` (a list of `{repo, root}`). These keys are distinct from the banned `repos` roster key.

**Auto-commit targets.** `autocommit.Targets` keeps its signature. A spec-scoped variant takes the record's location map, so the code targets resolve to the worktrees. The main-checkout artifact commit reuses the existing path-scoped commit-under-lock helper that plan lanes use. Its input is a list of `{dir, paths}` covering the spec's plan directory, spec file, changelog records and implement lane files.

**Merge refusal.** A new error code, `epic_merge_touches_spektacular`. Its message lists the offending paths per repo, and its `next_action` says to undo those commits on `spek/<spec>` in the named worktree and record the change through the CLI from the project root. `MergeResult` is unchanged.

**Status.** No type changes. `RunPart.Root` keeps naming the spec's project worktree when one exists. Only where the lane and finished evidence are read changes.

## Implementation Detail

- **Implicit becomes explicit.** Today, the directory an agent runs from silently decides which repos and stores it sees: the overlay is picked up whenever one exists under the cwd. After this change, the cwd is always the main project, and a spec's worktree view is something code asks for by name. It comes from a spec-scoped repo-set constructor fed by the worktree record. Someone reading the code sees every place a worktree is involved: the worktree package, the implement commands, implement auto-commit and status. No ambient file can redirect anything else.
- **Existing patterns followed.**
  - Per-spec lanes are reused unchanged for implement.
  - Orchestrated plan lanes already take a path-scoped commit under the commit lock, and epic implement lanes now use the same pattern for artifacts.
  - Runtime-only workflow config feeding a step template follows the `EpicDir` precedent.
  - List rendering in step templates follows the `dependency_override` precedent.
  - The merge guard is one more per-repo precheck in the existing precheck-then-dry-run loop, keeping all-or-nothing semantics.
- **Two-form template blocks.** The read-plan and feature-changelog step templates each gain a mustache section that renders the worktree code roots when present. An inverted section keeps today's `repo list` wording otherwise. Standalone implement renders byte-for-byte as before, so the existing template-contract tests and the harbor implement suite stay valid. Root paths reach the agent unescaped (triple-brace), per the mustache escaping gotcha.
- **Removal, not deprecation.** The in-worktree overlay file and its exclude pattern are deleted outright, along with the forced worktree removal that only existed because of it. There is no compatibility shim. In-flight runs are out of scope.
- **Skills stay thin.** The orchestrator's child prompt loses its location instructions, and the implement skill loses its worktree framing. The knowledge of where code lives moves from skill prose into CLI output.

## Dependencies

- **Worktree package**: creates, lists and merges spec worktrees. Changed: it gains the worktree record and the `.spektacular/` merge guard, and loses the overlay write.
- **Repo package**: the registry view. Changed: no implicit overlay, plus a spec-scoped constructor.
- **Workflow package**: lanes and runtime config. Changed: a runtime-only code-roots field. Lanes are reused unchanged.
- **Implement step package and step kit**: render step instructions. Changed: the read-plan and feature-changelog templates and their data.
- **Autocommit package**: commit targets, the lock, and path-scoped commits. Changed: spec-scoped targets for implement. The lock and path-scoped helper are reused.
- **Status package**: epic run state. Changed: lane and finished evidence come from the main project.
- **git (system binary)**: `merge-tree`, three-dot `diff`, `worktree`. No new git features beyond those already used.
- **Docs repo (`docs`)**: the epics page is updated in the same change. No build dependency.
- **Prior work that must already be in place**: the epics and seeded specs work and the epic plan/implement orchestration work, both already merged on `main`. Nothing else must land first.
- **Design documents this plan was built on**: none. The spec carries no design references.

## Testing Approach

**Kinds of tests.** Go unit and integration tests carry most of the weight, using real git repositories in temp directories, as the worktree and epic command tests already do. Template-contract tests pin the new two-form step wording and the rewritten skill phrases. No new harbor suite is added. The implement harbor suite runs without worktrees and must keep passing unchanged, which shows that standalone implement is untouched.

**Most coverage, and why.**
- **Worktree package.** The record's lifecycle (written by `Ensure`, read with no git, removed by `Merge`), the absence of any file under a worktree's `.spektacular/` after `Ensure`, and the merge guard: a branch commit under `.spektacular/` in either the project repo or a sibling repo is refused, the refusal names the path, and no repo's main line moves.
- **Implement commands and auto-commit.** End-to-end cmd tests with a real worktree fixture, running `implement new`/`goto` from the main root:
  - the rendered instruction names each worktree root;
  - zero git calls and no persisted roster still hold;
  - a plan tick written through the CLI lands in the main project's plan store, and is readable before any merge;
  - the main project's copy is unchanged in the worktree;
  - auto-commit puts code commits on `spek/<spec>` in the worktree, and artifact commits in the main checkout limited to that spec's paths. Another spec's dirty artifacts in the main checkout stay uncommitted.
- **Status.** An in-progress implement lane in the main project with a worktree reports `in_progress`, its step, and the worktree root. Ticked tasks plus a final changelog in the main stores report `awaiting_merge`. Nothing is read from the worktree's `.spektacular/`.
- **Repo set.** `New` ignores any stray overlay-style file, and the spec-scoped constructor relocates only mapped repos.

**Load-bearing assertions.**
- Artifacts written during an epic implement land in the main project.
- A spec's worktree `.spektacular/` stays byte-identical to its base commit.
- Merge refuses any `.spektacular/` change.
- Standalone implement output is unchanged.

**Conventions followed.** Tests that execute `rootCmd` go through `resetRootCmd`/`runRootCmd`. Real-git fixtures pin identity. Template assertions are hand-copied substring oracles with a message per assertion. Nothing touches this repository's own `.spektacular/`.

**Spec success metrics.**
- *No epic run leaves project records needing manual reconciliation after merges.* **Behavioural test**: the cmd-level epic flow test merges a spec whose artifacts were written in the main project, and asserts that the main project's plan, changelog and spec are unchanged by the merge and the main checkout has no conflict or leftover. **Manual — captured in the implementation test plan**: one real two-spec epic run, to confirm the same in practice.
- *Acceptance criteria as the measure.* Each spec acceptance criterion maps to a behavioural test above. The exceptions are the docs criterion and the live mid-run observation of plan ticks by a person, both **Manual — captured in the implementation test plan**.

**Manual reviews.**
- **Manual — captured in the implementation test plan**: review the rendered epics docs page (site build, type check and visual check of the "Implement this epic" section).
- **Manual — captured in the implementation test plan**: run the harbor implement suite once, because the read-plan and feature-changelog step templates change (testing-architecture knowledge entry).
- **Manual — captured in the implementation test plan**: one real epic run with two independent specs, after installing the built binary locally, between workflows. Watch plan ticks appear in the main project mid-run, confirm each spec worktree's `.spektacular/` is untouched, and confirm both merge cleanly.

**Deliberate gaps.** No harbor epic suite. Orchestrator behaviour is prose, so its regression guard is the skill-phrase contract tests plus the manual epic run.

## Milestones & Tasks

### Milestone 1: Merging a spec refuses changes to Spektacular's files, and worktrees are recorded in the project

**What changes**: `epic merge` refuses a spec whose branch changes anything under the Spektacular directory in any repo. It names the offending paths, gives a way out, and merges nothing. Creating a spec's worktrees also records, in the main project, where each touched repo's code lives inside them. Epic runs otherwise behave as today. This milestone adds the guard and the record that Milestone 2 relies on, and nothing more.

**Validation point**: Worktree and epic command tests show the refusal for a project-repo and a sibling-repo `.spektacular/` change, with nothing merged. They also show the record written on `epic worktree`, readable without git, and removed on merge. the full test suite passes.

#### - [ ] Task: Record spec worktree code roots in the main project
**Id:** 839eaf80-d6a0-43ef-9249-2b8ca986b3b8
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

When a spec's worktrees are created, write a small record in the main project, beside the spec's worktrees, that maps each touched repo to where its code lives inside them. The record can be read back without running git, and it is deleted when the spec is merged. For now the overlay is still written as well, so epic runs keep working until Milestone 2 switches children over.

*Technical detail:* [context.md#task-record-spec-worktree-code-roots-in-the-main-project](./context.md#task-record-spec-worktree-code-roots-in-the-main-project)

**Acceptance criteria**:
- [ ] After `epic worktree`, the main project holds a record for the spec naming every touched repo's code root inside the spec's worktrees.
- [ ] The record is readable without any git call, and reading it for a spec with no worktrees reports that there is none.
- [ ] After `epic merge`, the record is gone along with the worktrees.
- [ ] The record is never committed to git in the main project.

#### - [ ] Task: Refuse to merge a spec branch that changes the Spektacular directory
**Id:** e3691070-ff05-46e8-b5b0-0869e793c220
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add a check to `epic merge` that runs for every repo before anything is merged. If the spec's branch changes any path under the Spektacular directory, the merge is refused with a new error code. The refusal lists the offending paths per repo, says how to fix it, and nothing is merged anywhere.

*Technical detail:* [context.md#task-refuse-to-merge-a-spec-branch-that-changes-the-spektacular-directory](./context.md#task-refuse-to-merge-a-spec-branch-that-changes-the-spektacular-directory)

**Acceptance criteria**:
- [ ] A spec branch that commits a change under the project's Spektacular directory is refused, with the path named, and no repo's main line moves.
- [ ] A spec branch that commits a change under a sibling repo's Spektacular directory is refused the same way.
- [ ] A branch that changes only code still merges exactly as before.
- [ ] The refusal carries a concrete next action.

### Milestone 2: Epic children build each spec from the project, with the standard implement workflow

**What changes**: A child agent in an epic runs every Spektacular command from the project root. The implement workflow's own instructions tell it where each repo's code lives in the spec's worktrees. Plan ticks, changelog records and progress notes land in the project as they happen. Code is committed on the spec's branch, and only that spec's artifacts are committed in the project. Epic status reads progress from the project, and still names each spec's worktree. Nothing in an epic run touches a worktree's Spektacular directory any longer. The orchestrator's child prompt and the implement skill drop their worktree instructions. Implementing a spec on its own is unchanged.

**Validation point**: cmd-level tests with a real worktree fixture show four things. The instruction names the worktree roots. A plan tick written from the main root is readable in the project before merge. The worktree's `.spektacular/` is byte-identical to its base. Commits split between the worktree (code) and the main checkout (that spec's artifacts only). Status tests show `in_progress` and `awaiting_merge` from main-project records. Template-contract tests pass with the new wording, standalone implement renders exactly as before, and the full test suite passes.

#### - [ ] Task: Make the worktree repo view explicit and stop writing into worktrees
**Id:** 8345c4e2-cf84-4f80-b757-54813e6cc116
**Repo:** spektacular
**Depends on:**
- 839eaf80-d6a0-43ef-9249-2b8ca986b3b8 — Record spec worktree code roots in the main project
**Execution:** agent

Building the repo set no longer picks up an overlay from the current directory. A new spec-scoped constructor applies a spec's worktree record when a caller asks for it. Creating worktrees stops writing the overlay into the project worktree, and the overlay's file name, type, reader and git-exclude line are removed. After this, nothing Spektacular does writes inside a worktree's Spektacular directory.

*Technical detail:* [context.md#task-make-the-worktree-repo-view-explicit-and-stop-writing-into-worktrees](./context.md#task-make-the-worktree-repo-view-explicit-and-stop-writing-into-worktrees)

**Acceptance criteria**:
- [ ] After `epic worktree`, the Spektacular directory inside every one of the spec's worktrees is identical to its base commit, with no untracked files.
- [ ] `repo list` run from the main project reports registered locations, even while a spec has worktrees.
- [ ] The spec-scoped repo view relocates only the repos the spec's record maps.

#### - [ ] Task: Tell the implement workflow where spec code lives
**Id:** dd8d60be-3216-4c99-9fdf-9545084bfd6c
**Repo:** spektacular
**Depends on:**
- 839eaf80-d6a0-43ef-9249-2b8ca986b3b8 — Record spec worktree code roots in the main project
**Execution:** agent

`implement new` and `implement goto` read the spec's worktree record from the main project and pass the code roots to the workflow, without git and without saving them in workflow state. The read-plan step's "Where the code lives." block and the feature-changelog step list those roots when they exist. Otherwise they keep today's `repo list` wording word for word, so implementing a spec on its own is unchanged.

*Technical detail:* [context.md#task-tell-the-implement-workflow-where-spec-code-lives](./context.md#task-tell-the-implement-workflow-where-spec-code-lives)

**Acceptance criteria**:
- [ ] For a spec with worktrees, the first implement instruction names each touched repo's worktree code root as where its code lives.
- [ ] For a spec without worktrees, every implement instruction is exactly what it was before this change.
- [ ] `implement new` and `implement goto` still make no git calls and persist no repo roster.

#### - [ ] Task: Split implement auto-commits between worktree code and project artifacts
**Id:** 8616db31-f8c5-4488-b51e-0f07b275c83f
**Repo:** spektacular
**Depends on:**
- 8345c4e2-cf84-4f80-b757-54813e6cc116 — Make the worktree repo view explicit and stop writing into worktrees
**Execution:** agent

When an implement run's spec has worktrees, auto-commit commits code in the spec's worktrees, on its branch. In the main checkouts it commits only that spec's own artifacts: its plan documents, spec, changelog records, scratch and lane files. These are path-scoped commits taken under the existing commit lock, the same way orchestrated plan lanes commit. Implementing a spec without worktrees keeps committing exactly as today.

*Technical detail:* [context.md#task-split-implement-auto-commits-between-worktree-code-and-project-artifacts](./context.md#task-split-implement-auto-commits-between-worktree-code-and-project-artifacts)

**Acceptance criteria**:
- [ ] Code changed during an epic implement run is committed on the spec's branch in its worktree, never on the main checkout's branch.
- [ ] The spec's plan ticks and changelog records are committed in the main checkout, and nothing belonging to another spec or to the user is swept into that commit.
- [ ] Two specs committing at the same time queue on the lock rather than fail.
- [ ] Implementing a spec without worktrees commits exactly as before.

#### - [ ] Task: Read epic implement progress from the main project
**Id:** f2f15f68-42a6-4759-bf09-d6d900160fa1
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

`status` stops reading a spec's implement lane and its finished evidence from inside the spec's worktree, and reads both from the main project. It keeps reporting the spec's worktree as the run's root, and it still reports a finished, unmerged spec as awaiting merge.

*Technical detail:* [context.md#task-read-epic-implement-progress-from-the-main-project](./context.md#task-read-epic-implement-progress-from-the-main-project)

**Acceptance criteria**:
- [ ] A spec with worktrees and an in-progress implement lane in the main project is reported `in_progress`, with its current step and its worktree as root.
- [ ] A spec with worktrees whose plan tasks are all ticked and whose changelog is final in the main project is reported `awaiting_merge`.
- [ ] Nothing under a worktree's Spektacular directory affects what status reports.

#### - [ ] Task: Point epic children at the project root in the skills
**Id:** 432621d7-62ab-4a6c-997f-b2e4d89cfb2b
**Repo:** spektacular
**Depends on:**
- dd8d60be-3216-4c99-9fdf-9545084bfd6c — Tell the implement workflow where spec code lives
**Execution:** agent

Rewrite the epic implement skill's child prompt so the child runs every Spektacular command from the project root and follows `spek-implement`. It no longer works from a worktree or uses `repo list` there, because the workflow's own instructions name the code roots. Update the skill's start, resume and overview wording, the implement skill's orchestrated section, and the `epic worktree` help text to match. Update the template-contract tests that pin these phrases.

*Technical detail:* [context.md#task-point-epic-children-at-the-project-root-in-the-skills](./context.md#task-point-epic-children-at-the-project-root-in-the-skills)

**Acceptance criteria**:
- [ ] The epic implement skill tells each child to run Spektacular from the project root, and no longer tells it to work from, or resolve repos inside, a worktree.
- [ ] The implement skill's orchestrated section no longer mentions worktrees.
- [ ] The `epic worktree` help text no longer says repos resolve inside the project worktree.

#### - [ ] Task: Prove epic spec artifacts stay in the project end to end
**Id:** 6eb73340-282e-43b2-93e7-6b26e9ba24c7
**Repo:** spektacular
**Depends on:**
- e3691070-ff05-46e8-b5b0-0869e793c220 — Refuse to merge a spec branch that changes the Spektacular directory
- 8616db31-f8c5-4488-b51e-0f07b275c83f — Split implement auto-commits between worktree code and project artifacts
- f2f15f68-42a6-4759-bf09-d6d900160fa1 — Read epic implement progress from the main project
- 432621d7-62ab-4a6c-997f-b2e4d89cfb2b — Point epic children at the project root in the skills
**Execution:** agent

Add a cmd-level test that drives one epic spec through worktree creation, an implement run from the main root, and merge, using a real git fixture with a sibling repo. It checks the spec's acceptance criteria together:
- plan ticks and changelog records appear in the main project before the merge;
- the worktrees' Spektacular directories stay untouched;
- the merge brings in code only, and leaves the main project's records unchanged;
- status reports correctly throughout.

*Technical detail:* [context.md#task-prove-epic-spec-artifacts-stay-in-the-project-end-to-end](./context.md#task-prove-epic-spec-artifacts-stay-in-the-project-end-to-end)

**Acceptance criteria**:
- [ ] One test shows a plan tick readable in the main project before merge, and identical after it.
- [ ] The same test shows every worktree's Spektacular directory byte-identical to its base commit at the end of the run.
- [ ] The same test shows the merge changes nothing under any Spektacular directory, and needs no manual reconciliation.

### Milestone 3: The epic documentation describes the new model

**What changes**: The public epics page says that each spec's worktrees hold only code, and that specs, plans and progress stay in the project, where they can be followed during a run. The CLI's own help text for `epic worktree` says the same.

**Validation point**: The docs site builds and type-checks cleanly, and the "Implement this epic" section reads correctly. This is checked by hand and captured in the implementation test plan.

#### - [ ] Task: Update the epics page for code-only worktrees
**Id:** b28acf2a-f4ea-4c1a-b1b6-43f98cf81a3c
**Repo:** docs
**Depends on:**
- 432621d7-62ab-4a6c-997f-b2e4d89cfb2b — Point epic children at the project root in the skills
**Execution:** agent

Rewrite the worktree bullet in the "Implement this epic" section of the public epics page. It should say that each spek's worktrees hold only code, and that specs, plans and progress stay in the project, where they can be followed while the run is going. Add a bullet saying that a spek whose branch changes Spektacular's files is not merged.

*Technical detail:* [context.md#task-update-the-epics-page-for-code-only-worktrees](./context.md#task-update-the-epics-page-for-code-only-worktrees)

**Acceptance criteria**:
- [ ] The epics page says worktrees hold only code, and that plans and progress can be followed in the project during a run.
- [ ] It no longer says that, inside a spek's worktrees, repos resolve to the spek's own copy.
- [ ] It says a spek whose branch changes Spektacular's files is refused at merge.

## Open Questions

None. Every design question was resolved during planning and recorded in the assumption log. That covers where the record lives, how roots reach the steps, how commits split, the guard's error code, and what status reports as root.

## Out of Scope

- Removing the Spektacular directory from worktrees, or excluding it from their checkout. It stays as tracked files that are never read or written (spec non-goal).
- Changing how epic planning runs. Planning uses no worktrees (spec non-goal).
- Adding new store providers (spec non-goal).
- Migrating epic runs in progress under the old model. They are finished or restarted by hand (spec non-goal).
- A harbor end-to-end suite for epic runs. The orchestrator is covered by skill-phrase contract tests, the cmd-level flow test, and one manual epic run.
- Changing how worktrees are created, how code is isolated between specs, or how the all-or-nothing merge works, beyond the new `.spektacular/` refusal.
