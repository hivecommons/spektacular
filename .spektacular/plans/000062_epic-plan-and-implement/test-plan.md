---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Test plan: 000062_epic-plan-and-implement

Everything deterministic (ordering, the `run` view, lanes, scoped commits, worktrees, merge conflicts, skill wording) is covered by `go test`. The procedures below need a live agent driving the two epic skills, so they are run by hand.

## Setup (shared by every procedure)

Run by: the maintainer, before releasing the version that ships `spek-plan-epic` and `spek-implement-epic`, with Claude Code (repeat the end-to-end runs with Bob and Codex where available).

1. Build and install the binary from this branch: `make install-local`, then `spektacular version`.
2. Create a throwaway project with two repos, so cross-repo worktrees are exercised:
   ```bash
   mkdir -p /tmp/epic-e2e && cd /tmp/epic-e2e
   mkdir app site && (cd site && git init -q && echo "# site" > index.md && git add -A && git commit -qm init)
   cd app && git init -q && echo "package main" > main.go && git add -A && git commit -qm init
   spektacular init claude
   ```
   Register `site` with the guided repo add (`/spek-manage-repos`), then commit everything (`git add -A && git commit -m setup` in both repos). Set `auto_commit: full` in `.spektacular/config.yaml`.
3. Write a three-spec epic with `/spek-new` (or write the speks and run `spektacular epic write`). Spec A, spec C independent of A, spec B depends on A and C. Make B also change a file in `site`, so it gets two worktrees. List B **first** in the epic, to prove list order does not override dependencies. Confirm with `spektacular status <epic>`: `epic.run.order` is A, C, B (or C, A, B by list order).

## 1. Plan an epic on request, end to end (success metric 1, requirement "Plan an epic on request")

- **How:** in a fresh agent session in `app`, say "plan this epic" (no spec names).
- **Expected:**
  - The agent starts children for A and C without being asked, and planning overlaps in time: two `.spektacular/workflows/plan-*.json` lanes exist at once.
  - B starts only after A's and C's plans are final.
  - No per-section confirmations and no per-spec sign-off are asked for.
  - The run ends with one summary entry per plan and a completed/skipped/outstanding report.
  - `spektacular status <epic> --format json` shows `epic.run.plan.done == 3`.
  - Each plan's completion commit (`git log --stat`) contains only that plan's files.
- **Pass:** all three plans exist and are `final`, and the user started no individual spec's planning.

## 2. Question relay and declined question (requirements "Open questions reach the user", "Stop on planning failure")

- **How:** before planning, add to spec A a requirement with a genuine choice and no reasonable default, for example "the export either keeps the old field or drops it; the user decides". Run "plan this epic".
- **Expected:** the agent shows `QUESTION:` for A as plain text naming A, while C's plan keeps advancing (its lane's `current_step` changes during the wait). Answer it; A's plan reflects the answer.
- **Repeat, declining:** reset (delete the plans with `spektacular plan file delete`), and this time answer "not now".
- **Expected after declining:** no new spec starts, C finishes, and the final message names A and the reason it stopped and lists C as completed.

## 3. Resume part-way (requirements "Resumable by repeating the request", "Resume part-way through a spec")

- **How:** start "plan this epic" and interrupt the session (Ctrl-C) while a plan is mid-way, with a lane at a drafting step such as `architecture`. Start a new session and say "plan this epic" again.
- **Expected:** finished plans are reported as skipped. The interrupted plan resumes at the step in its lane, and its `.spektacular/work/<spec>/` sections are not gathered again. Every spec ends planned exactly once, and existing plans keep their `created_date`.

## 4. Implement an epic on request, end to end (success metric 1, requirement "Implement an epic on request")

- **How:** with all three plans final and committed, say "implement this epic".
- **Expected:**
  - A and C each get `.spektacular/worktrees/<spec>/...` and implement at overlapping times.
  - Each is merged (`git log --graph` shows a `--no-ff` merge of `spek/<spec>`) before B's worktree is created.
  - B gets worktrees in both `app` and `site`, and its `site` change lands in `site`'s main line.
  - No input is asked for between specs or tasks.
  - The run ends with every spec `done` in `status`, with no worktrees or `spek/*` branches left (`git worktree list`, `git branch`).
- **Pass:** `spektacular status <epic>` reports 3/3 specs implemented and the user started no individual implementation.

## 5. Refusals (requirements "Implementing needs plans", "Broken dependencies are refused")

- **How:** delete one plan, then say "implement this epic".
- **Expected:** the agent names the unplanned spec (`epic_unplanned`) and implements nothing: no worktree is created and no lane exists.
- **Repeat, cycle:** `epic write` refuses a cycle, so in the throwaway project only, edit the epic's file under `.spektacular/epics/` by hand so that A depends on B and B on A. Expect `epic_dependency_cycle` naming both, and nothing implemented.
- **Repeat, outside dependency:** have A depend on an unimplemented spec outside the epic. Expect `epic_dependency_outside`.

## 6. Merge conflict and failure (requirements "Parallel work is combined before dependents start", "Stop on failure")

- **How:** make A's and C's plans both change the same line of the same file, then implement the epic.
- **Expected:** the second merge returns `epic_merge_conflict`. The agent shows the conflicting paths per repo, merges nothing more, starts nothing new, and leaves that spec's worktrees in place. The final report lists the worktrees left behind.
- **Repeat, failure:** make one spec's verification fail (a deliberately failing test the plan cannot fix).
- **Expected after the failure:** the child hands back `FAILED:`, an independent spec already running completes and is merged, no further spec starts, and the final message names the failed spec and reason and lists the completed one.

## 7. Progress messages (requirement "Progress is visible")

- **Observe during runs 1 and 4:** after each start, hand-back and merge, one line naming the specs in progress with their steps and how many remain.

## 8. Prompts per planning run (success metric 2)

- **What to measure:** user prompts per epic planning run, at most 1 plus the number of genuine open questions raised.
- **How:** during runs 1 and 2, count every message where the agent waited for user input. Exclude the user's opening request.
- **Expected:** run 1 with no genuine questions has exactly one prompt (the end-of-planning review confirmation). Run 2 has at most 2 (the question plus the review). If the project had uncommitted changes at the start of an implement run, the single uncommitted-work question is the only extra prompt.
- **Who / when:** the maintainer, over the first real epic planning runs after release. Record the counts per run.
