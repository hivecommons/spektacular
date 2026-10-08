---
created_date: "2026-10-06"
document_status: final
closed_date: "2026-10-06"
---

# Test plan: 000064_epic-worktree-store-isolation

Automated tests cover every acceptance criterion that can be checked in code. These are in `internal/worktree`, `internal/repo`, `internal/status`, `internal/steps/implement`, `cmd/autocommit_test.go`, `cmd/implement_test.go`, `cmd/epic_worktree_test.go`, `cmd/epic_flow_test.go` and the template contract tests. The procedures below are the manual metrics and reviews the plan's Testing Approach lists.

## 1. Real two-spec epic run (success metric: no manual reconciliation; live plan ticks)

- **What to measure**:
  - Plan ticks appear in the project while the run is going.
  - No spec worktree's `.spektacular` is touched.
  - Both specs merge with no record needing manual reconciliation.
- **Setup**:
  1. From the spektacular repo, run `make install-local`, between workflows and never mid-run.
  2. In a throwaway project under git (`git init`, `spektacular init claude`, commit), set `auto_commit: workflow` and register a second repo with `spektacular repo add`.
  3. Create an epic with two independent specs, each with a plan whose tasks name both repos. Plan them with "plan this epic", then commit.
- **How**:
  1. Ask the agent to "implement this epic".
  2. While the children run, from the project root:
     - repeat `spektacular plan file read <spec> plan` and watch the task checkboxes flip;
     - repeat `spektacular status <epic> --format json` and confirm each spec shows its own `current_step` and its worktree as `root`.
  3. For each spec, before it is merged, run both of these in each of its worktrees (`.spektacular/worktrees/<spec>/<repo>`):
     - `git status --porcelain --ignored -- .spektacular`
     - `git diff $(git merge-base HEAD main) -- .spektacular`
  4. After the run, from each repo's main checkout, run `git log --oneline -6` and `git status`.
- **Expected result**:
  - Ticks are visible mid-run, before any merge.
  - Both worktree commands print nothing for every worktree.
  - Both specs report `done`.
  - Each repo's merge commits change no path under `.spektacular` (`git show --name-only <merge>`).
  - `git status` is clean apart from work that was already there.
  - Each spec's changelog record reads the same before and after its merge (`spektacular changelog file read <spec>`).
  - Nothing needs reconciling by hand.
- **Who / when**: the maintainer, once before release, on the installed build.

## 2. Harbor implement suite (the read-plan and feature-changelog step templates changed)

- **What to measure**: standalone implement behaves as before, end to end with a real agent.
- **How**: `make harbor-install` once, then `make harbor-test-implement` from the spektacular repo.
- **Expected result**: the suite passes, with the same oracles as before this change.
- **Who / when**: the maintainer, before release.

## 3. Epics docs page review (docs repo)

- **What to review**: the "Implement this epic" section of the epics page in `spektacular-website` (`src/pages/epics.mdx`).
- **How**: run `npm run build` and `npx astro check` (both passed during implementation with 0 errors), then `npm run dev` and open `/epics`.
- **What to look for**:
  - The worktree bullet says worktrees hold only code.
  - The next bullet says specs, plans, changelogs and progress stay in the project and can be followed during a run.
  - A bullet says a spek whose branch changes Spektacular's own files is not merged.
  - Nothing still says repos resolve to the spek's own copy.
  - There are no em dashes, and the list renders like the neighbouring bullets.
- **Passing**: all of the above hold, and the section reads naturally.
- **Who / when**: the maintainer, before the docs are deployed.
