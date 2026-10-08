---
created_date: "2026-10-07"
document_status: final
closed_date: "2026-10-07"
---

# Test plan: 000065_implement-in-worktrees

Every behaviour below also has automated coverage (`cmd/implement_worktree_flow_test.go`, `cmd/implement_worktrees_test.go`, `cmd/implement_lane_test.go`, `cmd/implement_merge_test.go`, `cmd/status_dirty_test.go`, `internal/worktree/worktree_test.go`). These procedures cover what a unit test cannot: real agent sessions, real epic runs, the harbor suite and the rendered docs.

**Setup for every procedure:** build and install the new binary between workflows, from the spektacular repo: `make install-local`, then confirm `spektacular version check` reports `match` in the project you test in. Use a throwaway two-repo project (a project repo plus one sibling repo registered with `spektacular repo add`), both git repos with at least one commit, and two planned specs.

## 1. Two specs implemented side by side from two terminals (success metric)

- **What to measure**: two implement runs, on two specs planned against the same repo, proceed at once with no interference, and both merge back without manual cleanup when they do not conflict. Pass threshold: 0 cross-contaminated files and 0 manual git commands needed.
- **How**:
  1. Terminal A: in the project, start an agent and run `/spek-implement` for spec `A`. Terminal B: same for spec `B`. Neither run should be offered the other's resume.
  2. While both are in progress: `ls .spektacular/workflows/` shows `implement-A.json` and `implement-B.json`; `git worktree list` in the project repo shows `.spektacular/worktrees/A/...` and `.spektacular/worktrees/B/...` on `spek/A` and `spek/B`.
  3. Pick a file A's run created: it must not exist in B's worktree or in the main checkout (`git status --porcelain -- . ':!.spektacular'` in each main checkout is empty).
  4. Let both finish. Each finished step tells the agent to run `spektacular implement merge --data '{"name":"<spec>"}'`; let both merge.
- **Expected result**: both runs complete; both merges report `merged: true, removed: true`; afterwards `git branch --list 'spek/*'` is empty in every repo and `.spektacular/worktrees/` holds nothing for A or B. 0 manual git commands.
- **Who / when**: the maintainer, before releasing this change.

## 2. No child or sub-agent edits a main checkout in a multi-repo epic run (success metric)

- **What to measure**: in the next real multi-repo epic run, every code edit, build and check happens in the spec worktrees. Pass threshold: 0 modified or untracked files outside `.spektacular` in any main checkout during the run.
- **How**: run "implement this epic" on an epic touching at least two repos. While children run, and before each merge, run `git status --porcelain --untracked-files=all -- . ':!.spektacular'` in every registered repo's main checkout. Also read one child's analyze/implement/test/verify instructions: each has a "Where the code lives" block listing the worktree roots and the "Give every sub-agent you launch" rule.
- **Expected result**: the status output is empty in every main checkout at every check; sub-agent prompts in the transcript name the worktree roots.
- **Who / when**: the maintainer, during the next real multi-repo epic run.

## 3. Epic worktrees are ready to build before the first task (success metric)

- **What to measure**: every spec's worktrees can build and test before the first task, with no ad hoc dependency installation by the agent. Pass threshold: 0 dependency-install commands run by agents inside worktrees.
- **How**: in the docs repo (or another repo that needs `node_modules`), declare the command: `spektacular repo add --data '{"name":"docs","location":"<path>","worktree_setup":"npm ci"}'`, and check `spektacular repo list` shows `"worktree_setup": "npm ci"`. Run "implement this epic" on an epic whose plans touch that repo. After `epic worktree` creates a spec's worktrees, check `<worktree>/node_modules` exists before the child's first task. Optionally set `worktree_setup` to a failing command (`exit 3`) on a throwaway spec and confirm `epic worktree` refuses with `worktree_setup_failed` and leaves no worktree for that repo.
- **Expected result**: `node_modules` is present in each new worktree before any task; no agent runs `npm install`/`npm ci` itself; the failing command is refused with the repo, command and output named.
- **Who / when**: the maintainer, during the next real multi-repo epic run.

## 4. Real standalone two-repo run (manual review)

- **Where**: the throwaway two-repo project, with the installed binary.
- **What to look at**: run `/spek-implement` on a spec whose plan touches both repos, with `auto_commit: "off"`. Check that `implement new` creates `spek/<spec>` worktrees in both repos (`git worktree list`), that the first instruction names the worktree roots, that the run commits its code on `spek/<spec>` at the end (`git log spek/<spec>` in each repo) while the main checkouts get no commit, and that `implement merge` merges both and cleans up. Then set `implement: {worktrees: false}` in `.spektacular/config.yaml`, implement another spec, and confirm no `spek/` branch is made and changes land in the main checkout.
- **Passing**: every check above holds, and the agent never merged, rebased or switched branches except through `implement merge`.
- **Who / when**: the maintainer, before releasing.

## 5. Harbor implement suite (manual review)

- **Where**: `tests/harbor/implement-workflow/`, whose seeded `config.yaml` now sets `implement.worktrees: false`.
- **What to look at**: run the suite once with harbor as usual for step-template changes. The suite must still exercise the main-checkout path (`state.json` in the shared slot).
- **Note**: the seeded `config.yaml` is an older settings format that this build refuses to load directly (pre-existing, not caused by this change). If the run fails on the format check, migrate the seeded environment first or report it as a separate fix.
- **Passing**: the suite's oracle passes.
- **Who / when**: the maintainer, before releasing.

## 6. Documentation pages (manual review)

- **Where**: in the docs repo (`spektacular-website`), run `npm run dev` and open `/configuration/`, `/how-it-works/`, `/epics/` and `/projects/`. (`npm run build` and `npx astro check` already pass with 0 errors.)
- **What to look for**: the `implement` section and the `worktree_setup` key render as ConfigKey cards in the right groups ("Project configuration keys", "Repository configuration keys"), and both appear in the example YAML; the key count reads seventeen; the implement stage on how-it-works has the worktree paragraph; the epics page shows the uncommitted-work bullet, the `worktree_setup` sentence and `"dirty_repos": []` in the status example; the projects page lists `worktree_setup`. No em dashes in the new text, and the new sections render in both light and dark themes.
- **Passing**: everything renders, reads correctly, and matches the shipped behaviour.
- **Who / when**: the maintainer, before publishing the site.
