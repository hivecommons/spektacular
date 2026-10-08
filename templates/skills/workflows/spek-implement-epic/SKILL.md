---
name: spek-implement-epic
description: Implement every planned spec of an epic in one request, running the standard implement workflow for each spec through separate agents, each in its own git worktrees, in dependency order. Use for "implement this epic", "build the epic", or "implement <epic name>".
---

{{> partials/version-check}}

> **STOP. Read this before running any command below.**
> Implementing an epic is a loop over many specs, and every spec is its own multi-step implement workflow. Starting the first child agent, or seeing the first spec finish, is **NOT** completion. Keep driving the loop below until `{{command}} status <epic> --format json` reports no spec left to implement, or the run has stopped, and only then give the final report.

# What this skill does

This skill implements a whole epic. It never implements the epic itself: each of its specs is implemented through its own plan, exactly as if it had been implemented on its own, and gets its own changelog. You are the **orchestrator**. For every spec ready to build you start a separate agent, a **child**, which runs the standard `spek-implement` skill for that one spec.

Each spec is built in its **own git worktrees**: one in every repo its plan touches, on branch `spek/<spec>`, under `.spektacular/worktrees/<spec>/`. Inside the spec's project worktree, every registered repo resolves to that spec's worktrees, so specs that do not depend on each other are implemented side by side without touching each other's files. When a spec finishes, you merge it back into every repo's main line before anything that depends on it starts.

The CLI, not you, works out what is left. `{{command}} status <epic> --format json` reports, for every spec, whether its implementing is `done`, `in_progress` (with its step and the worktree it runs in), `awaiting_merge`, `ready` or `blocked` (with what it waits on), and the repos its plan touches. Because that is read from what is on disk, repeating the request after an interruption picks up exactly where the epic stands: implemented specs are skipped, an interrupted spec resumes in its worktree, and a finished one is merged.

Your own notes for this run go in `.spektacular/working-context.md`: which epic, which specs you started, merges made, questions put to the user and their answers. When the request is repeated, read them back first, so a question the user already answered is not asked again. Each child keeps its own notes in its lane.

# Spektacular's files are reached through Spektacular

Never use your own file tools on a store directory, and never build a store path by hand. Read and write plans only with `{{command}} plan file read` and `{{command}} plan file write --from <path>`, specs with `{{command}} spec file`, changelogs with `{{command}} changelog file`, and epics with `{{command}} epic`. Every child prompt must restate this rule, because a child inherits neither this skill nor your context.

# Step 1: Find the epic

- If the request names the epic, use that name.
- If it says "this epic", use the epic of the spec under discussion: `{{command}} status <spec> --format json` reports it under `epic.name`.
- Otherwise run `{{command}} epic list` and ask the user which epic they mean, but only when it is still ambiguous.

# Step 2: Check up front

Run `{{command}} status <epic> --format json` and read `epic.run`.

- **Refuse when implementing is blocked.** If `epic.run.problems` holds any problem whose `blocks` includes `implement`, implement nothing. Relay each problem's `message` to the user: `epic_unplanned` names the specs with no plan yet, `epic_dependency_cycle` names the specs whose dependencies form a cycle, and `epic_dependency_outside` names a dependency outside the epic that is not implemented. Suggest "plan this epic" for unplanned specs, or fixing the epic's dependencies with `{{command}} epic write`. Then stop.
- **Uncommitted work.** If `epic.run.dirty` is true, a repo touched by this epic's plans has uncommitted changes. Name the repos in `epic.run.dirty_repos` and match them against each spec's `run.implement.repos` to explain which specs are affected; unrelated registered repos do not trigger this warning. Worktrees branch from each repo's last commit, so that work would not be in them. Ask the user once whether to commit it first; the answer is theirs. If they decline, go ahead, and say that the uncommitted work will not be in any spec's worktree; a spec whose own plan is uncommitted cannot start in its worktree, and its child hands back `FAILED:`. Apart from settling which epic is meant, this is the only start-of-run question.

Before launching children, record `git status --porcelain --untracked-files=all` from every registered repo's main code root (`{{command}} repo list`), including repos no spec touches. This is a read-only isolation baseline: preserve pre-existing user changes, never clean or stash them automatically. Identify the orchestrator's own project notes separately.

# Step 3: The loop

Repeat until nothing is left to start or merge and no child is running:

1. Run `{{command}} status <epic> --format json` and read each spec's `run.implement`.
2. For each spec that is `awaiting_merge`, merge it now (Step 5).
3. For each spec that is `in_progress` and has no child running, start a child to **resume** it in its `root`, the spec's project worktree.
4. For each spec that is `ready`, create its worktrees:

   ```
   {{command}} epic worktree --data '{"spec":"<spec>"}'
   ```

   Then start a child to **start** it in the returned `project` path.
5. Specs that are `done` are skipped; specs that are `blocked` wait for the specs in their `waiting_on`, which must be implemented **and merged** first.
6. Report progress (Step 7), then wait for a child to hand back and handle it (Step 6). Then go round again.

Use your agent's own way of running sub-agents in the background, so independent specs are implemented in overlapping time. Start every ready spec; there is no limit on how many run at once.

A child that handed back a `QUESTION:` and is waiting for its answer still counts as running. Never start a second child for a spec that already has one, waiting or not.

# Step 4: The child prompt

Give each child exactly what it needs, because it inherits nothing from you:

- the spec name, and the root to work in: the spec's project worktree. Run workflow and store commands from that project root; `{{command}} repo list`, run from there, gives it each repo's code `root` inside the spec's worktrees. Run code edits, dependency setup and verification from those code roots, which may differ from the workflow project root;
- to **start** or **resume**: run `{{command}} implement new --data '{"name":"<spec>","orchestrated":true}'`. If it returns a resume report for the spec's lane, read back what the report names (the plan documents and the lane's notes), then run the `goto` it gives, which carries `\"name\":\"<spec>\"`;
- "Follow the `spek-implement` skill. This run is orchestrated: tasks run one after another without asking between them, every `goto` carries `\"name\":\"<spec>\"`, and you never ask the user anything yourself.";
- "Never resolve a merge or git conflict yourself, and never merge, rebase or switch branches: your worktree's branch is merged by the orchestrator.";
- "Pass the workflow project root and the relevant worktree code roots explicitly to every sub-agent you launch, including implementers, test authors and verifiers. Require code edits and checks to run in those worktree roots, never in a main checkout. Worktrees contain tracked files only; install ignored dependencies using each repo's documented setup inside its worktree, not in the main checkout, and do not symlink mutable dependencies from it.";
- the store-access rule above, word for word;
- the hand-back contract and the definition of a genuine open question, below.

## The hand-back contract

A child ends every turn that stops its work with a final message whose **first line** is exactly one of:

- `DONE: <spec>`, followed by the completion summary the implement workflow's finished step reports, and any durable discovery worth saving to the knowledge base.
- `QUESTION: <spec>`, followed by the question, the options, and the default it recommends. It then waits for your answer and carries on from the same step.
- `FAILED: <spec>`, followed by the step it reached and the reason it cannot go on.

## What counts as a genuine open question

Only a stop the implement workflow itself defines earns a question: the plan no longer matching the code, a task outgrowing its scope, or a verification failure the child cannot fix within the task. Everything else the child decides itself and records in the plan's changelog. A child never asks whether to continue to its next task.

# Step 5: Merging a finished spec

Merge a spec as soon as its child hands back `DONE:`, and whenever `status` reports it `awaiting_merge`:

```
{{command}} epic merge --data '{"spec":"<spec>"}'
```

The merge is all or nothing across the spec's repos. On success the spec's worktrees and branches are removed and its changes are in every repo's main line, so specs that depend on it can start. On `epic_merge_conflict`, nothing was merged in any repo: show the user the conflicting paths for each repo, leave the worktrees in place, and enter stopping mode. Never resolve a conflict yourself.

Any other refusal from `epic worktree` or `epic merge` is handled the same way: relay its `message` and `next_action` to the user, leave the worktrees as they are, and enter stopping mode.

# Step 6: Handling a hand-back

- **`DONE:`** — merge the spec (Step 5), keep its summary for the final report, then re-read `status` and start the specs that have just become ready.
- **`QUESTION:`** — put the question to the user. Present it as ordinary text first, naming the spec, the options and the recommended default, then ask. While the user considers it, the other children keep going. Send the user's answer back to the same child, so it continues where it stopped. If your agent cannot continue a stopped child, start a fresh child on the same spec in the same worktree with the answer included in its prompt; it resumes from its lane.
- **The user declines to answer now** — enter stopping mode.
- **`FAILED:`** — enter stopping mode.

**Stopping mode:** start no new child, let the children already running finish (and merge each one that finishes cleanly, since specs depending on it may already be waiting), then go to the final report. A child waiting on a question the user declined to answer is not resumed: its work stays in progress in its worktree, and repeating the request picks it up. Name the spec that stopped the run and why.

# Step 7: Progress

After every child you start, every hand-back and every merge, tell the user in one line which specs are being implemented, with their steps from `status`, and how many remain. For example: "Implementing 2 of 4 specs: 000071_a (implement), 000073_c (verify). 000071_a merged. 1 remaining."

# Step 8: The final report

Before reporting, repeat `git status --porcelain --untracked-files=all` in every main code root and compare with the isolation baseline. A run that started clean must leave no modified or untracked code files in any main checkout; merges may change HEAD, not leave uncommitted files. If the user chose to preserve existing dirt, it must remain unchanged. Account separately for your own project notes and any user-approved commits. Report unexpected changes as an isolation failure, naming the repo and paths; never discard them or claim a clean run.

End every run, finished or stopped, with:

- **Completed in this run:** each spec implemented and merged now, with its summary.
- **Skipped:** each spec that was already implemented.
- **Still outstanding:** each spec not implemented, and why: blocked on which specs, stopped by which failure or conflict, or waiting on a question the user chose not to answer.
- **Worktrees left behind:** each spec whose worktrees remain unmerged, with their paths, so the user can inspect or finish them.

Offer to save any durable discovery the children reported with the `spek-knowledge` skill, and save it only if the user accepts.
