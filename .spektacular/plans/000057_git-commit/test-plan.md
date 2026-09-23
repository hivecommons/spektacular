---
created_date: "2026-09-23"
document_status: final
closed_date: "2026-09-23"
---

# Implementation Test Plan: 000057_git-commit

Three of the spec's four success metrics are fully covered by automated
behavioural tests. Two of them have a residual manual half: the Go end-to-end
tests drive the CLI directly, so they prove the mechanics but cannot prove that
a real coding agent, reading only the rendered instructions, does the right
thing. Those halves are below, plus the plan's one open question, which can
only be answered against a real signing setup.

Run these before releasing the feature. Every procedure assumes a scratch
project with two registered repositories, both git work trees, and a coding
agent driving the workflows through its installed skills.

## 1. A whole spec to plan to implement cycle with no manual `git commit`

**Metric**: with `workflow` or `full` enabled, a developer finishes a whole
spec, plan and implement cycle without running `git commit` by hand for any
work the agent did.

**Automated half**: covered by `TestAutoCommit_SpecCompletionCommitsEachChangedRepoOnce`,
`TestAutoCommit_PlanCompletionCommitsEachChangedRepoOnce` and
`TestAutoCommit_ImplementCompletionCommitsEachChangedRepoOnce` in `cmd/autocommit_test.go`,
which assert one commit per changed repo per completion and a clean tree after.

**Manual procedure**:

1. Create a scratch project and run `spektacular init claude`. Register a
   second repository with `spektacular repo add`. Make both git repositories
   with a clean tree and at least one commit.
2. Set `auto_commit: workflow` in `.spektacular/config.yaml`.
3. Drive a small feature through `/spek-new`, then `/spek-plan`, then
   `/spek-implement`, letting the agent follow its own instructions. Do not
   prompt it about git and do not run `git` yourself at any point.
4. After each workflow finishes, run `git log --oneline` and
   `git status --porcelain` in both repositories.

**Expected result**: exactly one new commit in each repository that changed, at
each of the three completions, and nothing else. `git status --porcelain` is
empty after each completion. Every commit message names the spec. You never
typed a `git` command.

**Then repeat with `auto_commit: full`** and a plan with at least two
milestones. Expect, additionally, one commit per changed repository as each
milestone's last phase is ticked, each message naming the spec and
`Milestone N`, and a completion commit only where changes remain.

**Who / when**: the releasing maintainer, once per release, against a scratch
project rather than a real one.

## 2. The agent actually relays the uncommitted-changes question

**Metric**: a developer is never surprised by an automatic commit containing
their own earlier work without having first been asked whether to commit it
separately.

**Automated half**: covered by
`TestStartGate_UncommittedChangesStopEveryWorkflowBeforeAnythingIsWritten`,
`TestStartGate_CommitExistingTrueCommitsTheUsersWorkFirst` and
`TestStartGate_CommitExistingFalseFoldsTheChangeIntoTheWorkflowsCommit` in
`cmd/startgate_test.go`, which assert the CLI refuses, writes nothing, and
honours either answer. What they cannot assert is that the agent puts the
question to the user instead of answering it itself.

**Manual procedure**:

1. In the scratch project from procedure 1, with `auto_commit: workflow`, edit
   a tracked file and leave it uncommitted. Add a second, never-added file.
2. Ask the agent to start a spec workflow.
3. Observe what the agent does next, without steering it.

**Expected result**: the agent stops and asks you, in its own words, whether to
commit the existing changes before starting, naming both affected repositories.
It does **not** decide for you, and it does not start the workflow first and
ask afterwards.

4. Answer "commit them first". Expect the workflow to start, and
   `git log -1` to show a commit containing exactly your two files, whose
   message names the spec and says the changes are yours from before the
   workflow.
5. Repeat from a dirty tree, answering "continue without committing". Expect no
   commit at start, and your change to appear inside the workflow's first
   automatic commit.

**Who / when**: the releasing maintainer, once per release. Repeat for each
supported agent (Claude, Bob, Codex) if more than one is available, since the
skill text is identical but the agents' adherence to it is not.

## 3. Open question: commit signing that needs an interactive pinentry

**Question** (from the plan's Open Questions): does a GPG or SSH commit-signing
setup that needs an interactive pinentry work when Spektacular invokes git from
an agent's shell?

**Why manual**: it depends entirely on the user's signing agent configuration.
A cached agent works; one that needs a TTY prompt may not. Only a real signing
setup shows which.

**Procedure**:

1. Configure `commit.gpgsign true` with a signing key whose agent requires an
   interactive passphrase prompt, and ensure the passphrase is **not** cached.
2. With `auto_commit: workflow`, complete a spec workflow.

**Expected result**: either the commit succeeds (the agent found a way to
prompt, or the passphrase was cached), or it fails as an ordinary
`auto_commit_failed` carrying git's own stderr, leaving the workflow on its
previous step. Both are acceptable outcomes; the failure must be legible.

**If the failure message is unclear for this case**, add a hint to the
`auto_commit_failed` next_action, for example "unlock your signing key, then
re-run". **Do not** add `--no-gpg-sign` or any other bypass: commits must use
the user's signing settings, and a bypass would break a spec constraint. If a
bypass looks necessary, stop and raise it rather than implementing one.

**Who / when**: a maintainer who has a signing setup, before the first release
that ships `auto_commit`.

## Metrics needing no manual procedure

- **"Every milestone of a `full`-mode implementation ends with a commit in each
  repository it changed"** is fully covered by
  `TestMilestoneCommit_CommitsEachChangedRepoPerMilestone` and
  `TestMilestoneCommit_CompletionCommitsOnlyWhatChangedSince` in
  `cmd/milestones_test.go`.
- **"Every automatic commit message can be traced to its spec, and to the
  milestone where there is one"** is fully covered by message validation
  (`TestValidateMessage`) plus the end-to-end assertions that each commit
  subject or body contains the spec name and, at a milestone, `Milestone N`.
