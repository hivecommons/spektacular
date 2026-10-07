---
created_date: "2026-10-07"
document_status: final
closed_date: "2026-10-07"
designs:
    - source: design
      path: epics-and-seeded-specs.md
sources:
    - uri: https://github.com/hivecommons/spektacular/issues/78
      retrieved_date: "2026-10-07"
---

# Feature: 000065_implement-in-worktrees

## Overview

Every implement run — part of an epic or started on its own — builds its spec in the spec's own git worktrees and merges them back when it finishes, so two people or two terminals can implement different specs at the same time without seeing each other's half-finished changes. Alongside that it closes three gaps found while running an epic: agents an implement run starts are not reliably told where the spec's code lives, a fresh worktree is missing the dependencies a repo needs to build and test, and the epic's uncommitted-work warning is raised by repos no spec touches.

## Requirements

- [x] **Every implement run builds in its own worktrees**
  An implement run, whether started on its own or by an epic, builds its spec in a separate worktree of every repo its plan touches, so concurrent runs on different specs never share a checkout.
- [x] **Worktrees are on by default and can be turned off**
  Runs started on their own use worktrees unless the project opts out, including in projects that existed before this change. A project that opts out builds those runs in its main checkouts, as it does today. Epic runs always use worktrees, because their specs are built side by side.
- [x] **A spec's own branch always records its work**
  The work a run produces is committed to the spec's own branch whatever the project's automatic-commit setting, so there is always something to merge back.
- [x] **A run started on its own merges back when it finishes**
  When an implement run that was not started by an epic completes, its work is merged into the main line of every touched repo, or of none. On a conflict the merge is refused, the conflicting paths are named, nothing is merged anywhere, and the user resolves it. The agent never resolves it.
- [x] **A finished run cleans up after a successful merge**
  Once a run's work is merged, its worktrees and branches are removed. If the merge is refused, they are kept for the user to resolve.
- [x] **A dependency must be merged, not only implemented**
  The dependency check at the start of an implement run treats a dependency that is implemented but whose work is not yet merged into the main line as unmet, because the run's worktrees would not contain its code. It then warns or refuses exactly as for any other unmet dependency.
- [x] **Sub-agents are told where the code lives**
  The implement workflow tells the agent to pass the spec's code locations explicitly to every sub-agent it launches, including task implementers, test authors and verifiers, and to keep every code edit, build and check in those locations, never in a main checkout.
- [x] **A repo can declare how to prepare a new worktree**
  A repo can declare a setup command, such as installing its dependencies, that is run in each new worktree of that repo before work starts. If the command fails, the run is refused and the failure is reported. A repo that declares nothing needs no setup.
- [x] **The agent knows a worktree starts without ignored files**
  The implement workflow tells the agent that a new worktree holds only tracked files. Dependencies are prepared inside the worktree, never installed into or shared from a main checkout.
- [x] **The uncommitted-work check covers only the repos being built**
  An epic's uncommitted-work warning considers only the repos its specs' plans touch, and names each repo that is dirty, so the specs affected can be identified.
- [x] **Documentation covers the new behaviour**
  The documentation site describes the worktree opt-out, the per-repo setup command, implement runs started on their own building in worktrees and merging back, and the narrowed uncommitted-work warning.

## Constraints

- Each spec is isolated in a git worktree, on a branch of its own, in every repo its plan touches. This is the mechanism epics already use, and runs started on their own adopt it rather than a different form of isolation.
- Spektacular commands always run from the project, never from inside a worktree, and nothing under a worktree's `.spektacular` directory is read or written. Specs, plans, changelogs and progress stay in the project; worktrees hold only code.
- An agent never resolves a merge or git conflict itself, and never merges, rebases or switches branches on its own initiative.
- A project with worktrees turned off must behave exactly as implementing does before this change.
- The epic's status output must stay backwards compatible: the existing uncommitted-work flag keeps its name and type, and the names of the dirty repos are added alongside it.
- The dependency check at the start of an implement run follows the `design` source's `epics-and-seeded-specs.md`, which settles what a dependency means, how each one is classified, and when an unmet dependency warns or refuses (`epic.strict_dependencies`). An implemented but unmerged dependency is an unmet dependency under those same rules.

## Acceptance Criteria

- [x] **Two runs started on their own proceed side by side**
  With two specs planned against the same repo, starting an implement run for each from two terminals gives each its own worktree. Files changed by one run do not appear in the other run's worktree or in the main checkout until that run merges.
- [x] **Every touched repo gets a worktree**
  An implement run started on its own whose plan touches two registered repos has a worktree on the spec's branch in each of them, and none in a registered repo the plan does not touch.
- [x] **Opting out restores today's behaviour**
  With worktrees turned off, an implement run started on its own creates no worktrees or branches, and its changes appear directly in the main checkouts. An epic run in the same project still builds each spec in its own worktrees.
- [x] **Existing projects get worktrees without a config change**
  A project whose configuration predates this change, and does not mention the setting, uses worktrees for its next implement run.
- [x] **Work reaches the spec branch with automatic commits off**
  With automatic commits turned off, the changes of a completed implement run started on its own are present as commits on the spec's branch and are merged into the main line.
- [x] **A clean run merges and cleans up**
  When an implement run started on its own and touching two repos completes with no conflicts, both repos' main lines contain its changes, and its worktrees and branches no longer exist.
- [x] **A conflicting merge changes nothing**
  When one of two touched repos' main lines has a conflicting change, the end-of-run merge is refused with a message naming the repo and conflicting paths. Neither repo's main line moves, and the worktrees and branches remain for the user to resolve.
- [x] **An unmerged dependency is reported as unmet**
  Starting an implement run for a spec whose dependency has every task complete but whose work is not yet merged names that dependency as unmet. The run then warns, or refuses under strict dependencies, exactly as it does for an unimplemented dependency.
- [x] **Code-touching steps name the code locations for sub-agents**
  The instructions the implement workflow gives for implementing, testing and verifying a task list each touched repo's worktree location, and state that every sub-agent launched must be given those locations and must work only in them.
- [x] **Implement instructions explain worktree dependencies**
  The same instructions state that a new worktree has only tracked files and that dependencies are prepared inside the worktree, not taken from a main checkout.
- [x] **A declared setup command runs in each new worktree**
  For a repo that declares a setup command, the command's effect, such as an installed dependency directory, is present in the spec's new worktree of that repo before the run's first task starts. A repo without one gets no setup.
- [x] **A failing setup command refuses the run**
  When a repo's setup command exits with an error, the run is refused with a message naming the repo and the command's failure, and no task starts.
- [ ] **Main checkouts' code is unchanged by a run**
  After an implement or epic run that started from a clean tree, and before its merge, no main checkout has modified or untracked files outside the project's own Spektacular store.
- [x] **The uncommitted-work warning ignores untouched repos**
  For an epic whose plans touch only some registered repos, an untouched repo with uncommitted changes does not raise the warning. A dirty touched repo raises it, and the epic's status names that repo.
- [x] **Docs describe the new behaviour**
  The documentation site's configuration reference describes the worktree opt-out and the per-repo setup command. Its implement and epic workflow pages describe implement runs started on their own building in worktrees and merging back, and the narrowed uncommitted-work warning.

## Technical Approach

- Build on the worktree machinery epics already have rather than a second mechanism: a run started on its own gets its worktrees and its end-of-run merge the same way an epic's spec does, sharing one creation path and one all-or-nothing merge.
- Extend the implement workflow's existing discovery of a spec's worktree locations so the steps that implement, test and verify a task carry the sub-agent and dependency guidance next to the locations.
- Declare the worktree setup command with the repo's own registration, so it travels with the repo, and run it as part of creating that repo's worktree.
- Reuse the narrowed uncommitted-work check proposed in PR #79 (https://github.com/hivecommons/spektacular/pull/79). Write the step and skill guidance fresh rather than reusing that PR's wording, which predates the project-root and code-only-worktree rules above.

## Success Metrics

- Two specs can be implemented at the same time from two terminals with no interference between them, and both merge back without manual cleanup when they do not conflict.
- In the next multi-repo epic run, no child or sub-agent runs its checks or edits in a main checkout.
- In the next multi-repo epic run, every spec's worktrees are ready to build and test before its first task, with no ad hoc dependency installation by the agent.
- The epic's uncommitted-work warning is raised only when a repo the epic actually builds is dirty, and its status says which repo that is.

## Non-Goals

- Scheduling or ordering several runs started on their own. Running specs in dependency order remains the epic's job.
- Worktrees for the spec or plan workflows. Only implementing builds in worktrees.
- Changing how dependencies between specs are declared, or allowing a dependency on a spec outside an epic.

