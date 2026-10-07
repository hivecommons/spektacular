# Working context — spec 000065_implement-in-worktrees

Source: GitHub issue #78 (hivecommons/spektacular), "Epic runs: sub-agents default to the wrong checkout, worktrees lack ignored deps, dirty check covers untouched repos". Raised after running a 4-spec, 3-repo epic (xcl `examples-and-output`) with `spek-implement-epic`.

## Problem
1. Sub-agents launched by an implement run (task implementers, verifiers) inherit the working directory, not knowledge of where code lives. A verifier ran checks in the main checkout instead of the spec's worktree. After 000064 (store stays in project, worktrees are code-only, `spektacular` always runs from the project), any sub-agent not pointed at the worktree *always* lands in the main checkout.
2. `git worktree add` checks out tracked files only: no `node_modules` or other ignored deps. Each child worked out `npm ci` / symlinking on its own.
3. `epic.run.dirty` is true when any registered repo is dirty, even ones no plan touches.

## User's direction (this conversation)
- User: "I think 1 and 2 are the main things."
- User wants worktrees for *every* implementation, epic or not: "I might want to have two terminals open which are both working on different specs."
- Decided (user chose recommended options):
  - Worktrees default ON for every implement run, with a config opt-out.
  - Standalone runs merge back at the end, all-or-nothing like `epic merge`; on conflict refuse naming paths, user resolves (agent never resolves).
  - Ordering stays an epic concern; standalone run only refuses/warns when a spec it depends on is not implemented and merged.
  - Include item 3 in this spec, taking PR #79's dirty_repos approach.
- Item 2 becomes a real mechanism (not only wording) because every run would hit it: a repo can declare a worktree setup command (e.g. `npm ci`) run after its worktree is created; failure is a refusal; repos with nothing to set up declare nothing.
- Item 1: task/test/verify step output tells the agent to pass the code roots to every sub-agent; plus a check that main checkouts' *code* is unchanged after a run (excluding `.spektacular/` store writes, which 000064 puts in the project on purpose).
- "Other repos": worktrees already cover every registered repo the plan touches (via `epic worktree`); no change needed beyond reusing that for standalone runs.

## Relevant current state (from code, branch b-patch-epics after 000064)
- `cmd/implement.go` `codeRootsFor` already reads the spec's worktree record for any implement run and feeds `worktree_roots` into steps 01 and 10. Only creation (`epic worktree`) and merge (`epic merge`) are epic-only.
- `templates/partials/implement-current-task.md` is used by steps 03/04/05.
- auto_commit is `full` in this project; 000064 merge guard refuses a branch touching `.spektacular`.

## PR #79 (mendezr / Hive agent, open, based on origin/main without 000064)
- Its dirty_repos change (RunSource.Dirty(repos) []string, epic.run.dirty_repos) is sound — reuse.
- Its template/skill wording conflicts with 000064 (tells child to run commands from project worktree) and its main-checkout baseline check would false-positive on project store writes. Do not take those parts as-is.

## Interview answers (user)
- auto_commit off: spec branch always gets commits; merge is part of worktree behaviour (turn worktrees off to avoid). Confirm at review.
- Worktrees on for everyone on upgrade.
- Standalone run refuses when a dependency is not implemented and merged.
- Docs repo in scope.
- User (at acceptance criteria): "Just keep going until the end, I will look at the final" — draft remaining sections without per-section confirmation; user reviews the assembled spec.
- Deviation to flag at final review: user chose "refuse" for unmet dependency on a run started on its own, but design `design:epics-and-seeded-specs.md` (binding, now referenced as a constraint) already defines warn-by-default / refuse under epic.strict_dependencies. Kept the design; spec only adds "implemented but unmerged counts as unmet".
- Terminology: design uses "standalone spec" = spec with no epic. Spec says "a run started on its own" (not by an epic orchestrator) to avoid the clash.
- Split offered (epic fixes vs worktrees for every run); user declined — keep one spec.
