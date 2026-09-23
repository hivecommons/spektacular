---
created_date: "2026-09-22"
document_status: final
closed_date: "2026-09-22"
---

# Feature: 000057_git-commit

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Spektacular will be able to record a project's progress in git automatically, so developers don't have to remember to commit after each step of the spec → plan → implement cycle. A project can choose to commit whenever a spec, plan or implementation is completed, to also commit after each milestone of an implementation, or to turn automatic commits off (the default). When automatic commits are on, before a workflow starts the user is told about any uncommitted changes and can commit them first, so their own work doesn't get mixed in with the agent's. After that, each commit carries a descriptive message, happens without asking the user, and is made separately in every repository the work touched. If a commit fails, the workflow stops, so the history stays in step with the work.

<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [ ] **Commit mode setting**
  A project can set an automatic-commit mode to one of three values: `off`, `workflow` or `full`.
- [ ] **Off by default**
  A project that has not set the mode behaves as `off`.
- [ ] **Off makes no commits**
  In `off` mode, no spec, plan or implement workflow creates any git commit.
- [ ] **Commit on workflow completion**
  In `workflow` mode, the system commits the agent's changes when a spec workflow completes, when a plan workflow completes, and when an implement workflow completes.
- [ ] **Commit on milestone completion**
  In `full` mode, the system makes every commit that `workflow` mode makes, and also commits the agent's changes each time an implement workflow finishes one of the plan's milestones. The completion commit at the end of implementation then picks up whatever changed after the last milestone.
- [ ] **No confirmation prompt**
  In `workflow` and `full` modes, automatic commits happen without asking the user to confirm them.
- [ ] **Uncommitted changes flagged at workflow start**
  In `workflow` and `full` modes, when a spec, plan or implement workflow starts and any registered repository has uncommitted changes, the system tells the user which repositories have them and asks whether to commit them before the workflow begins.
- [ ] **Pre-existing changes committed on request**
  If the user agrees, the system commits the uncommitted changes in each affected repository, with a message that names the spec about to be worked on and says they are the user's changes from before the workflow, before the workflow does any work.
- [ ] **Declining means whole-tree commits**
  If the user declines, the workflow goes ahead and every automatic commit it makes includes all uncommitted changes in the repository, both the user's and the agent's.
- [ ] **Clean tree, no question**
  When no registered repository has uncommitted changes at workflow start, the workflow begins without asking the user anything.
- [ ] **Commits include all changes**
  Each automatic commit includes every uncommitted change in its repository at that moment, including new files that have never been added to git.
- [ ] **New files count as uncommitted**
  New files that have never been added to git count as uncommitted changes for the check at workflow start.
- [ ] **Repositories outside git are skipped**
  A registered repository that is not a git repository is left alone: it is not checked at workflow start, gets no automatic commits, and does not stop the workflow.
- [ ] **Descriptive message**
  Each commit message names the spec it relates to, names the milestone when the commit marks one, and describes the work it contains (what was specified, planned or implemented), not just a generic label.
- [ ] **One commit per repository**
  When the agent's changes span several of the project's registered repositories, each repository that changed gets its own commit.
- [ ] **Nothing to commit**
  A repository with no uncommitted changes at a commit point gets no commit, and this is not treated as an error.
- [ ] **Failure stops the workflow**
  If a commit can't be made (for example, a commit hook rejects it or git returns an error), the workflow stops and tells the user what failed, rather than moving on.
- [ ] **Local only**
  Automatic commits are never pushed to a remote.
- [ ] **Documented setting**
  The documentation site's configuration reference describes the setting, its three values and its default.

<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- Commits must be made with the standard command-line `git` tool installed on the user's machine, not with a built-in git library or any other tool.
- Automatic commits must run the repository's git hooks, exactly as a manual commit would, and must never skip them.
- Automatic commits must use the user's configured git identity and signing settings, exactly as a manual commit would.
- Automatic commits must work the same way for every coding agent Spektacular supports (Claude, Bob, Codex), not just Claude.

<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [ ] **Mode accepts three values**
  A project whose settings name the mode as `off`, `workflow` or `full` runs its workflows without a settings error for each of the three values.
- [ ] **Unset mode behaves as off**
  In a project that has never set the mode, running a spec, plan and implement workflow to completion leaves the git log of every registered repository unchanged.
- [ ] **Off leaves history unchanged**
  With the mode set to `off`, running a spec, plan and implement workflow to completion leaves the git log of every registered repository unchanged, and the agent's changes stay uncommitted in the working tree.
- [ ] **Workflow mode commits at each completion**
  With the mode set to `workflow`, finishing a spec workflow, a plan workflow and an implement workflow each adds exactly one new commit to each repository the agent changed during that workflow, and, apart from the optional pre-existing-changes commit at workflow start, no commits are added while those workflows are still running.
- [ ] **Full mode commits per milestone**
  With the mode set to `full` and a plan with two milestones, the implement workflow adds one commit to each changed repository as each milestone finishes, then one completion commit to each repository that still has changes once the implementation completes (skipped for a repository with nothing left). The spec and plan workflows commit as in `workflow` mode.
- [ ] **No commit prompt**
  In `workflow` and `full` modes, the user is never asked whether to make an automatic commit. The only commit-related question is the uncommitted-changes question at workflow start.
- [ ] **Dirty tree is flagged**
  With the mode set to `workflow` or `full` and a modified file in a registered repository, starting a spec, plan or implement workflow shows the user a message naming that repository and asks whether to commit the changes first, before any workflow work begins.
- [ ] **Accepting commits pre-existing changes first**
  When the user accepts, the repository's git log gains a commit containing the pre-existing change, made before any of the workflow's own commits, and its message names the spec and says it holds the user's changes from before the workflow.
- [ ] **Declining sweeps changes into automatic commits**
  When the user declines, the pre-existing modified file is included in the workflow's first automatic commit in that repository, along with the agent's changes.
- [ ] **New file triggers the check and is committed**
  With the mode set to `workflow` or `full`, a new file that has never been added to git causes the uncommitted-changes question at workflow start, and a new file created during the workflow appears in the next automatic commit.
- [ ] **Non-git repository skipped**
  With a registered repository that is not a git repository, a workflow in `workflow` or `full` mode runs to completion, asks no question about that repository, and reports no error for it.
- [ ] **Clean tree starts silently**
  With every registered repository clean, starting a workflow shows no uncommitted-changes question.
- [ ] **Off mode never asks**
  With the mode set to `off`, starting a workflow with uncommitted changes shows no uncommitted-changes question.
- [ ] **Descriptive commit message**
  Each automatic commit's message names the spec it relates to and says what was completed (the spec, the plan, the implementation, or a named milestone), rather than a generic message that is the same for every commit.
- [ ] **Per-repository commits**
  When an implementation changes files in two registered repositories, each repository's git log gains its own commit, and each commit contains only that repository's files.
- [ ] **Unchanged repository skipped**
  When a commit point is reached and a registered repository has no uncommitted changes, that repository's git log is unchanged and the workflow carries on without reporting an error.
- [ ] **Commit failure halts the workflow**
  When a commit is rejected (for example by a pre-commit hook that exits with a failure), the workflow doesn't move to its next step, and the user sees a message saying which repository failed and why.
- [ ] **Nothing pushed**
  After any automatic commit, the remote-tracking branches of every repository are unchanged.
- [ ] **Setting documented**
  The documentation site's configuration reference has an entry for the setting that lists `off`, `workflow` and `full`, describes what each does, and states that the default is `off`.

<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- The mode lives with the project's other Spektacular settings, so it's set once per project rather than per run.
- The coding agent writes the commit message, drawing on the work it just finished, rather than Spektacular filling in a fixed template.
- Beyond this and the constraints already captured, no direction has been decided. The plan workflow will work out how commit points and the start-of-workflow check hook into the workflows.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- With `workflow` or `full` enabled, a developer finishes a whole spec → plan → implement cycle without running `git commit` by hand for any work the agent did.
- Across the milestones of an implementation run in `full` mode, every milestone ends with a commit in each repository it changed.
- A developer is never surprised by an automatic commit containing their own earlier work without having first been asked whether to commit it separately.
- Every automatic commit message can be traced to its spec, and to the milestone where there is one, just by reading it.

<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- Committing more often than once per plan milestone (for example after each individual step within a milestone) is out of scope.
- Creating branches, opening pull requests, tagging, or any other git operation beyond making a commit is out of scope for now.
- Running workflows in separate git worktrees is deferred to a future spec. Worktrees are a likely next step for git management but aren't addressed here.
- Undoing or squashing automatic commits after the fact is out of scope.
