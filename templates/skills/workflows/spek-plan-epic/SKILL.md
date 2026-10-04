---
name: spek-plan-epic
description: Plan every outstanding spec of an epic in one request, running the standard plan workflow for each spec through separate agents in dependency order. Use for "plan this epic", "plan the epic", "plan <epic name>", or "plan all the specs in this epic".
---

{{> partials/version-check}}

> **STOP. Read this before running any command below.**
> Planning an epic is a loop over many specs, and every spec is its own multi-step plan workflow. Starting the first child agent, or seeing the first spec finish, is **NOT** completion. Keep driving the loop below until `{{command}} status <epic> --format json` reports no spec left to plan, or the run has stopped, and only then give the final report.

# What this skill does

This skill plans a whole epic. It never plans the epic itself: an epic is never planned or implemented, and each of its specs gets an ordinary plan of its own, exactly as if that spec had been planned on its own. You are the **orchestrator**. For every spec that still needs a plan you start a separate agent, a **child**, which runs the standard `spek-plan` skill for that one spec. Specs that do not depend on each other are planned side by side; a spec whose dependencies are not yet planned waits for them, so its plan can build on theirs.

All planning happens in the project's own working copy. Each child's plan workflow is **orchestrated**: it keeps its own progress record and notes in a lane under `.spektacular/workflows/`, so several plans can be in progress at once beside any ordinary workflow.

The CLI, not you, works out what is left. `{{command}} status <epic> --format json` reports, for every spec, whether its planning is `done`, `in_progress` (with its step), `ready` or `blocked` (with what it waits on). Because that is read from what is on disk, repeating the request after an interruption picks up exactly where the epic stands: finished plans are skipped, and an interrupted plan resumes at the step it reached.

Your own notes for this run go in `.spektacular/working-context.md`: which epic, which specs you started, questions put to the user and their answers. When the request is repeated, read them back first, so a question the user already answered is not asked again. Each child keeps its own notes in its lane.

# Spektacular's files are reached through Spektacular

Never use your own file tools on a store directory, and never build a store path by hand. Read and write plans only with `{{command}} plan file read` and `{{command}} plan file write --from <path>`, specs with `{{command}} spec file`, and epics with `{{command}} epic`. Every child prompt must restate this rule, because a child inherits neither this skill nor your context.

# Step 1: Find the epic

- If the request names the epic, use that name.
- If it says "this epic", use the epic of the spec under discussion: `{{command}} status <spec> --format json` reports it under `epic.name`.
- Otherwise run `{{command}} epic list` and ask the user which epic they mean, but only when it is still ambiguous.

Run `{{command}} repo list` and note the `root` of the project's own repo. Every child runs there.

# Step 2: The loop

Repeat until nothing is left to start and no child is running:

1. Run `{{command}} status <epic> --format json`. Read `epic.run` and, for each spec, `run.plan`.
2. If `epic.run.problems` lists anything, mention it to the user once. Problems never block planning: a dependency cycle or a dependency outside the epic only means those specs are not ordered against each other.
3. For each spec whose `run.plan.state` is `in_progress` and has no child running, start a child to **resume** it, at the `current_step` reported.
4. For each spec whose `run.plan.state` is `ready`, start a child to **start** it.
5. Specs that are `done` are skipped; specs that are `blocked` wait for the specs in their `waiting_on`.
6. Report progress (Step 5), then wait for a child to hand back, and handle its hand-back (Step 4). Then go round again.

Use your agent's own way of running sub-agents in the background, so independent specs are planned in overlapping time. Start every ready spec; there is no limit on how many run at once.

A child that handed back a `QUESTION:` and is waiting for its answer still counts as running. Never start a second child for a spec that already has one, waiting or not.

# Step 3: The child prompt

Give each child exactly what it needs, because it inherits nothing from you:

- the spec name, and the project root to work in;
- to **start**: run `{{command}} plan new --data '{"name":"<spec>","orchestrated":true}'`;
- to **resume**: run the same `plan new` command. It returns a resume report for the spec's lane instead of starting over; read back what the report names (the plan's working files and the lane's notes), then run the `goto` it gives, `{{command}} plan goto --data '{"step":"<current_step>","name":"<spec>"}'`;
- "Follow the `spek-plan` skill. This run is orchestrated: every `goto` carries `\"name\":\"<spec>\"`, and you never ask the user anything yourself.";
- the store-access rule above, word for word;
- the hand-back contract and the definition of a genuine open question, below.

## The hand-back contract

A child ends every turn that stops its work with a final message whose **first line** is exactly one of:

- `DONE: <spec>`, followed by the plan summary its walkthrough step prepares (in an orchestrated run the walkthrough prepares a summary instead of asking for sign-off): the approach, the milestones and tasks (naming any task a person must do), what is out of scope, and the drafting assumptions. Then any durable discovery worth saving to the knowledge base.
- `QUESTION: <spec>`, followed by the question, the options, and the default it recommends. It then waits for your answer and carries on from the same step.
- `FAILED: <spec>`, followed by the step it reached and the reason it cannot go on.

## What counts as a genuine open question

Only a stop the plan workflow itself defines earns a question: a design reference that does not resolve, a disagreement with a referenced design, or a drafting decision with no reasonable default that only the user can settle. Everything else the child decides itself, records as a drafting assumption, and leaves for the review at the end of planning. A child never asks for approval of a section, and never asks the user to sign off its plan.

# Step 4: Handling a hand-back

- **`DONE:`** — keep its summary for the end-of-planning review. Re-read `status` and start any spec that has just become ready.
- **`QUESTION:`** — put the question to the user. Present it as ordinary text first, naming the spec, the options and the recommended default, then ask. While the user considers it, the other children keep going. Send the user's answer back to the same child, so it continues where it stopped. If your agent cannot continue a stopped child, start a fresh child on the same spec with the answer included in its prompt; it resumes the plan from its lane with the `goto` above.
- **The user declines to answer now** ("not now", "I'll think about it") — enter stopping mode.
- **`FAILED:`** — enter stopping mode.

**Stopping mode:** start no new child, let the children already running finish, then go to the final report. A child waiting on a question the user declined to answer is not resumed: its plan stays in progress in its lane, and repeating the request picks it up. Name the spec that stopped the run and why.

# Step 5: Progress

After every child you start and every hand-back, tell the user in one line which specs are being planned, with their steps from `status`, and how many remain. For example: "Planning 2 of 5 specs: 000071_a (tasks), 000073_c (discovery). 2 remaining."

# Step 6: The end-of-planning review

When no child is running and nothing is left to start, show the user one summary entry for **every plan produced in this run**, built from each child's `DONE:` summary, before anything is implemented. Invite changes. Apply each change to the plan document it belongs to, where `<doc>` is `plan`, `context` or `research`: read it with `{{command}} plan file read <spec> <doc>`, stage the edited document under `.spektacular/tmp/`, write it back with `{{command}} plan file write <spec> <doc> --from <path>`, and remove the scratch file. Offer to save any durable discovery the children reported with the `spek-knowledge` skill, and save it only if the user accepts. Close the review on a direct confirmation question.

Skip the review if no plan was produced in this run.

# Step 7: The final report

End every run, finished or stopped, with:

- **Completed in this run:** each spec planned now.
- **Skipped:** each spec that already had a plan.
- **Still outstanding:** each spec not planned, and why: blocked on which specs, stopped by which failure, or waiting on a question the user chose not to answer.

When everything is planned, say the epic is ready to implement with "implement this epic".
