---
created_date: "2026-09-23"
document_status: final
closed_date: "2026-09-23"
---

# Automatic git commits

Spektacular can now record a project's progress in git by itself, so developers
no longer have to remember to commit after each step of the spec, plan and
implement cycle.

## What was built

A new project setting, `auto_commit`, takes one of three values and defaults to
`off`:

- **`off`** (the default) changes nothing. No git process is started, and the
  instructions agents receive are byte-identical to before.
- **`workflow`** makes one git commit in every registered repository that
  changed, each time a spec, plan or implementation completes.
- **`full`** does everything `workflow` does, and additionally commits each
  time an implementation finishes one of its plan's milestones. The completion
  commit at the end then picks up only what changed after the last milestone.

Spektacular runs the commits itself, through the user's own `git` binary. Hooks
run, the user's identity and signing settings apply, and nothing is ever pushed.
A repository that is not a git work tree is skipped silently, two registered
repositories sharing one work tree produce a single commit between them, and a
repository with nothing to commit is passed over without an error.

The coding agent writes each commit message. At a commit point the step
instructions tell it what the message must name, and Spektacular refuses the
transition if the message is missing, empty, does not name the spec, or (at a
milestone) does not name the milestone it completes. Nobody is asked to confirm
a commit.

If git refuses a commit, for example because a pre-commit hook rejects it, the
workflow is put back on the step it was on and the failure names the repository
and quotes git's own reason. Re-running the same command after fixing the cause
retries the commit.

Before a workflow starts, Spektacular checks whether any registered repository
already holds uncommitted work. If it does, `new` stops before writing anything
and reports which repositories are affected, so the agent can ask the user
whether to commit that work first. Answering yes records it in its own commit,
whose message says the changes are the user's and predate the workflow.
Answering no starts the workflow and lets its automatic commits sweep the work
in. A clean tree, or `off` mode, starts with no question at all.

Behaviour is identical for Claude, Bob and Codex: every decision lives in Go or
in CLI-rendered instruction text that all three receive byte for byte.

## Why it matters

Developers using Spektacular previously had to remember to run `git commit`
after each spec, plan and implementation, and their own unrelated edits easily
got mixed into the agent's work. Automatic commits keep the git history in step
with the workflow without anyone having to think about it, and the
start-of-workflow question means a developer is never surprised by a commit
containing work they did themselves.

## Deviations from the plan

Three, all small and all recorded as they happened:

- **Milestone commit points were deferred within the plan's own sequence.**
  Phase 1.2 deliberately shipped an empty milestone table so `full` mode was
  accepted but behaved as `workflow`, exactly as the plan specified; Phase 3.1
  populated it. This is the plan working as intended rather than a divergence
  from it.
- **`resumeOrClear` kept a wrapper.** The plan expected spec, plan and
  implement to be its only callers, so the helper could be replaced outright by
  a read-only probe plus a separate clear. The repo-add workflow is a fourth
  caller, so the wrapper was kept for it. The plan anticipated this branch.
- **The skills' uncommitted-changes section sits elsewhere than sketched.** The
  plan proposed adding it as a third outcome of the spec skill's bare
  in-progress probe. That would have been wrong: a bare `spec new` with no
  `--data` returns a name-required error or a resume report before the gate is
  reached, so the report can only come back from the named invocation. The
  section follows the start and resume blocks instead.

No schema bump and no migrate step were needed: an absent `auto_commit` key
loads as `off`, so existing projects keep working untouched. Installed skills
changed, so existing projects will see a skills mismatch after upgrading and
should run `migrate`, as for any release.
