# Working context: 000064_epic-worktree-store-isolation

## Problem
In epic mode each spec's child agent runs from its project worktree
(`.spektacular/worktrees/<spec>/`). The CLI's project root is the cwd
(`cmd/root.go` projectRoot), and the worktree is a full checkout including
`.spektacular/`, so `spektacular plan file write` etc. write into the
worktree's copy of the stores on branch `spek/<spec>`. Plan progress only
reaches the main project via `epic merge` (git). This turns the stores into
"files on a git branch", bypassing Spektacular's store abstraction, and would
break for non-local store providers.

User: "the agent should always be writing plans with the spektacular tool
never to files directly. Remember this is a core concept."
User: "in the worktree the .spektacular directory should never be touched."
User: "if we can just use the standard implement skill as a sub task in an
epic this keeps things super clean."

## Decisions
1. The CLI always runs from the main project root. The worktree's
   `.spektacular/` is never read or written by anything.
2. The implement workflow's own step output hands the child its code roots:
   for a spec with worktrees, the spec's worktree roots; otherwise registered
   roots. Child runs standard `spek-implement` unchanged, unaware of the epic.
   (Rejected: spec-scoped `repo list --data {"spec":...}` — leaks epic
   awareness into the child.)
3. Lanes / workflow state / working notes live in the main project, keyed by
   spec.
4. `epic merge` refuses a branch that changes anything under `.spektacular/`.
5. Repo overlay file (`worktree-repos.json`) removed; `spek-implement-epic`
   child prompt simplified (run spektacular from project root).

Rejected: keep store writes in worktree and merge via git (current design).

## User preference for this workflow
User asked to "just produce the final spec, I will review it as a single
doc" — draft all sections from the conversation without per-step
confirmations, then present the whole spec once.

## Status (2026-10-06)
- Spec reviewed by user as a single doc ("looks good") and written to the
  store. Working files removed.
- Split check: not offered (requirements tightly coupled, one capability).
- Workflow parked at `split`: advancing to `finished` triggers auto_commit,
  which conflicts with the user's global rule never to commit unless asked.
  Waiting on user to decide.
