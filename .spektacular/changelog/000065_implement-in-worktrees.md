---
created_date: "2026-10-07"
document_status: final
closed_date: "2026-10-07"
---

# 000065_implement-in-worktrees

## What was built

Every implement run now builds its spec in the spec's own git worktrees, the same mechanism epics introduced in 000064, one per repo the plan touches, on branch `spek/<spec>` under `.spektacular/worktrees/<spec>/`.

- **Runs started on their own use worktrees by default.** `implement new` creates the spec's worktrees before the workflow starts (reusing any that already exist) and keeps the run's progress and notes in a per-spec lane under `.spektacular/workflows/`, so two specs can be implemented side by side from two terminals. A new `implement.worktrees` setting (default `true`, no schema bump) turns this off and restores the old main-checkout behaviour exactly. Epic runs always use worktrees. A touched repo that is not in git, or has no commits, is refused with `worktree_unavailable` and a way out.
- **Merging back.** A new `implement merge --data '{"name":"<spec>"}'` command merges the spec branch into every touched repo's main line, all or nothing, sharing the merge, conflict reporting and `.spektacular` guard with `epic merge`. The finished step of a complete worktree run tells the agent to run it, and to report and stop on a refusal; the agent never resolves conflicts, merges, rebases or switches branches on its own. A single-task run that leaves tasks open keeps its worktrees.
- **The spec branch always records the work.** With `auto_commit: off`, a worktree run still commits its code on the spec branch at completion (code only; nothing in the main checkouts), and the commit-message request says so.
- **Unmerged dependencies are unmet.** A dependency whose tasks are complete but whose worktree record still exists counts as unmet in both the implement dependency check and `status`, described "implemented but not yet merged"; the refusal offers to merge it.
- **Worktree setup command.** A repo can declare `worktree_setup` (e.g. `npm ci`) in its `repo.yaml`, set by `repo add` and shown by `repo list`. It runs in each newly created worktree of a touched repo, read from the main registration; a failure removes the worktree and refuses with `worktree_setup_failed`.
- **Sub-agents are told where the code lives.** The analyze, implement, test and verify steps list each repo's code location (worktree roots, or registry roots), require every sub-agent to be given those locations and work only there, and explain that a worktree holds only tracked files.
- **Narrowed epic dirty check.** `status` for an epic checks only the repos its plans touch for uncommitted work and names them in `epic.run.dirty_repos` (ported from PR #79); the epic skill names them to the user.
- **Docs.** The configuration, how-it-works, epics and projects pages document all of the above, with a site changelog entry.

## Why it matters

Before this change only epic children were isolated. Two people or terminals implementing different specs shared the main checkouts and saw each other's half-finished work, a run with automatic commits off left nothing mergeable on a spec branch, sub-agents could edit main checkouts because they inherited the wrong working directory, fresh worktrees lacked ignored dependencies, and the epic's uncommitted-work warning fired for repos the epic never built. Teams on multi-repo projects can now run specs concurrently and merge them back safely.

## Deviations from the plan

- `implement merge` returns `epic merge`'s real result shape (`merged`/`removed` booleans) rather than the lists sketched in the plan.
- `status` has no per-dependency description field, so it reports an unmerged dependency through the dependent's `ready: false` and `blocked_by`; the "implemented but not yet merged" wording appears in the implement refusal. Accepted by the user.
- The code-locations partial sits at the end of each step's Step 1, and the skill's Worktrees section before the orchestrator section, so neither splits existing instructions.
- Lane runs also resume a same-spec run left in the shared `state.json`, and refuse to start over an orchestrated lane of the same spec.
- `worktree_unavailable` uses the bare repo name as its resource.
- The guided `repo new` flow does not ask for `worktree_setup`; only `repo add --data` sets it.

Open concern for follow-up: with `auto_commit` on, a second terminal's uncommitted-changes check at start can see the first lane run's uncommitted project files.
