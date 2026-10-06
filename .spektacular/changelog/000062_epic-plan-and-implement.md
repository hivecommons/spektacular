---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Changelog: 000062_epic-plan-and-implement

## What was built

A whole epic can now be taken forward with two requests to the agent: "plan this epic" and then "implement this epic". Two new agent skills, `spek-plan-epic` and `spek-implement-epic`, act as orchestrators. They start one sub-agent per spec, and each sub-agent runs the unchanged `spek-plan` or `spek-implement` workflow for that spec, side by side wherever the epic's dependencies allow. Each spec still gets an ordinary plan, implementation and changelog.

Small, deterministic CLI support makes this resumable and testable:

- **Orchestrated workflows (lanes).** `plan new` and `implement new` accept `"orchestrated": true`. Such a run keeps its state and notes in `.spektacular/workflows/<kind>-<name>.json/.md`, so several plans, and an ordinary workflow, can be in progress at once in one project. Every plan and implement `goto` now carries the spec's `name`, which routes it to the right workflow. An unknown or mismatched name is refused with `workflow_not_found`. Scratch files and staged commit messages are kept per spec under `.spektacular/tmp/<name>/`.
- **Hand-back instead of asking.** In an orchestrated run, every stop is handed back to the orchestrator as `QUESTION:`, a failure as `FAILED:`, and completion as `DONE:` with a summary. The plan walkthrough prepares a summary instead of asking for sign-off, and the implement run loops between tasks without asking. Ordinary runs render exactly as before.
- **Scoped commits.** A finishing orchestrated plan commits only its own files (`git commit --only`) under a project-level commit lock, so parallel plans never sweep up each other's work. Lane files are removed when a run finishes.
- **Worktrees and merge.** `epic worktree` gives a spec a `spek/<spec>` git worktree in every repo its plan touches, kept out of git. A repo overlay makes every repo resolve into those worktrees from inside. `epic merge` merges a finished spec into every repo together after a dry run in each, or merges nothing and reports `epic_merge_conflict` with the paths per repo.
- **The run view in `status`.** Naming an epic or spec in `status` reports, for planning and for implementing, whether each spec is done, in progress (with step and where it runs), awaiting merge, ready or blocked (with what it waits on), plus the repos each plan touches. The epic gets the dependency order, counts, whether any repo is dirty, and the problems that block implementing: unplanned specs, dependency cycles and unimplemented dependencies outside the epic. The CLI also gained stable dependency ordering and cycle detection.
- **Documentation.** The website's Epics page explains planning and implementing an epic, and the README points to it.

## Why it matters

Before this, every spec of an epic had to be planned and implemented by hand, one at a time, with frequent stops for confirmation. Now an epic is planned and built with a couple of deliberate checkpoints: genuine open questions and one end-of-planning review. Independent specs proceed in parallel, and dependents start only once their dependencies are merged. Because everything is derived from what is on disk, repeating a request after an interruption resumes exactly where the epic stands.

## Deviations from the plan

- **Lane path helpers:** they live in `internal/workflow/lane.go` from the start, rather than moving there later.
- **Orchestrated stop section:** it is appended to every orchestrated step by `stepkit` as one section, rather than included at each STOP sentence.
- **Per-name commit-message path:** this applies to every workflow kind. Only plan and implement gotos carry the name.
- **`gitexec.RunCode`:** added, because `git merge-tree` reports conflicts through its exit code.
- **Bugs found by tests and fixed:**
  - **`CommitPaths` retry:** it looks at HEAD as well as the index, so a retry after a rejected commit still commits an already-staged deletion.
  - **`epic merge` overlap check:** it uses name-only queries, because output trimming broke porcelain parsing.
- **Status semantics:**
  - **Outside dependencies:** they count as met when implemented, the same rule as the single-spec dependency check.
  - **`awaiting_merge` count:** it is always present.
- **Skill wording:**
  - **Child resume:** a child resumes through the same orchestrated `new`, whose resume report names its lane notes.
  - **Waiting child:** a child waiting on a question counts as running.
  - **Declined commit:** the implement skill says what happens when the user declines to commit uncommitted work, and how to handle other worktree or merge refusals.
- **Docs link:** the docs page refers to the new section by name rather than by anchor link, because sections render no heading ids.
- **Build fix:** a duplicate `installerFor` that stopped the build was removed first.
- **Known minor gap:** two waiters finding the same stale commit lock (older than 10 minutes) can race.
- **Manual end-to-end checks:** those needing a live agent are in the plan's test plan.
