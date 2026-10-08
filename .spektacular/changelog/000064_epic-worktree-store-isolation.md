---
created_date: "2026-10-06"
document_status: final
closed_date: "2026-10-06"
---

# 000064_epic-worktree-store-isolation: Epic worktrees hold only code

## What was built

When an epic is implemented, each spec is still built in its own git worktrees, one per repo it touches, on branch `spek/<spec>`. Those worktrees now hold **only code**. Spektacular itself always runs from the main project, so every spec, plan tick, changelog record, lane and working note is written straight into the project's own stores while the run is going.

- **Worktree record.** `epic worktree` writes a small record in the main project (`.spektacular/worktrees/<spec>/record.json`, git-excluded) that maps each repo to its code root inside the spec's worktrees. It writes nothing inside a worktree. `epic merge` deletes the record.
- **No implicit overlay.** The old repo overlay written into a worktree's `.spektacular` is gone. Building the repo registry never depends on the current directory any more. Code that needs a spec's view asks for it explicitly (`repo.NewWithCodeRoots`, `autocommit.TargetsWithCodeRoots`).
- **The implement workflow names the code roots.** `implement new` and `implement goto` read the record, with no git and no persisted roster. When it exists, the read-plan step's "Where the code lives." block and the feature-changelog step list each repo's worktree root, and tell the agent to run every command from the project and never touch a worktree's `.spektacular`. A spec without worktrees gets exactly the same output as before (checked byte for byte).
- **Split auto-commit.** For a spec with worktrees, code is committed on `spek/<spec>` in the worktrees. In the main checkouts only that spec's own files are committed: plan, spec, changelog records, scratch and lane files. These commits are path-scoped and taken under the commit lock, so other specs' and the user's work is never swept in.
- **Status reads the project.** `status` takes implement progress and "finished" evidence only from the main project, and still reports the spec's worktree as `root`.
- **Merge guard.** `epic merge` refuses, with `epic_merge_touches_spektacular`, any spec branch that changes a path under a `.spektacular` directory in any repo. The refusal names the paths per repo, gives a revert-and-retry next action, and merges nothing.
- **Skills and docs.** The epic orchestrator's child prompt now gives only the project root to run from. The implement skill's orchestrated section drops its worktree framing. `epic worktree` and `epic merge` help, and the public epics page, describe the new model.

## Why it matters

Previously each spec's worktree carried its own copy of the project's records, and updates reached the project only when git merged the spec's branch. Teams could not watch plans progress mid-run, and the project's records were left to version control to reconcile. Now plans and progress can be followed in the project as they happen, merges bring in only code, and each spec is built by the same implement workflow as a standalone spec.

## Deviations from the plan

- The worktree record maps each repo to its **code root**, not to its registered `.spektacular` location as the overlay did. The spec-scoped constructors are therefore `repo.NewWithCodeRoots` and `autocommit.TargetsWithCodeRoots`, overriding code sources only, rather than the planned `NewWithLocations` and `TargetsWithLocations`. The reason is that relocating locations would mean reading `repo.yaml` inside a worktree's `.spektacular`, which the spec forbids.
- The merge guard uses a single `:(glob)**/.spektacular/**` pathspec for every repo. A separate project-relative pathspec is not needed.
- When committing a repo's changelog record, the git top is resolved from the repo's root rather than the record's folder, so a repo the spec did not change doesn't break the commit. The test author found this bug during implementation.
- The epic skill's uncommitted-work bullet was also corrected. Plans are now read from the project, so an uncommitted plan no longer stops a spec starting.
- The init template's `.gitignore` keeps its harmless `worktree-repos.json` line, so installed files don't change for existing projects.
- The worktree code commits are taken before the lock, so if the main-checkout half fails, the code commits stand. A retry is still correct.
