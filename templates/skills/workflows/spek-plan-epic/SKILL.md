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

Never use your own file tools on a store directory, and never build a store path by hand. Read and write plans only with `{{command}} plan file read` and `{{command}} plan file write --from <path>`, specs with `{{command}} spec file`, and epics with `{{command}} epic`. The epic's planning summary is read with `{{command}} epic summary read <epic>` and written one section at a time with `{{command}} epic summary write <epic> --data '{"section":"<section>"}' --from <path>`; only the orchestrator writes it, never a child. Every child prompt must restate this rule, because a child inherits neither this skill nor your context.

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
- that only you write the epic's planning summary, so the child never runs `{{command}} epic summary write`;
- the hand-back contract and the definition of a genuine open question, below.

## The hand-back contract

A child ends every turn that stops its work with a final message whose **first line** is exactly one of:

- `DONE: <spec>`, followed by the plan summary its walkthrough step prepares (in an orchestrated run the walkthrough prepares a summary instead of asking for sign-off): the approach, the milestones and tasks (naming any task a person must do), what is out of scope, the drafting assumptions, the project-wide rules it relies on or decides, one per line, and the manual checks its Testing Approach sends to the test plan (success metrics and reviews a person must make), one per line. Then any durable discovery worth saving to the knowledge base.
- `QUESTION: <spec>`, followed by the question, the options, and the default it recommends. It then waits for your answer and carries on from the same step.
- `FAILED: <spec>`, followed by the step it reached and the reason it cannot go on.

## What counts as a genuine open question

Only a stop the plan workflow itself defines earns a question: a design reference that does not resolve, a disagreement with a referenced design, a plan choice that would contradict a decision the user recorded for that spec (in the spec, a design it references, or its interview notes where they still exist) or a knowledge entry, or a drafting decision with no reasonable default that only the user can settle. A contradiction of a recorded decision or a knowledge entry is never parked as a drafting assumption, a task for a person or a note for the review. While one child waits on its question, every other child keeps planning. Everything else the child decides itself, records as a drafting assumption, and leaves for the review at the end of planning. A child never asks for approval of a section, and never asks the user to sign off its plan.

# Step 4: Handling a hand-back

- **`DONE:`** — keep its summary for Step 6; nothing is written to the epic's planning summary yet, because the summary is produced only once every decision between the plans is settled. Re-read `status` and start any spec that has just become ready.
- **`QUESTION:`** — put the question to the user. Present it as ordinary text first, naming the spec, the options and the recommended default, then ask. While the user considers it, the other children keep going. Send the user's answer back to the same child, so it continues where it stopped. If your agent cannot continue a stopped child, start a fresh child on the same spec with the answer included in its prompt; it resumes the plan from its lane with the `goto` above.
- **The user declines to answer now** ("not now", "I'll think about it") — enter stopping mode.
- **`FAILED:`** — enter stopping mode.

**Stopping mode:** start no new child, let the children already running finish, then go to the final report. A child waiting on a question the user declined to answer is not resumed: its plan stays in progress in its lane, and repeating the request picks it up. Name the spec that stopped the run and why. Before the final report, still run Step 6 when its conditions hold.

# Step 5: Progress

After every child you start and every hand-back, tell the user in one line which specs are being planned, with their steps from `status`, and how many remain. For example: "Planning 2 of 5 specs: 000071_a (tasks), 000073_c (discovery). 2 remaining."

# Step 6: Settle decisions, then write the summary

Once the loop ends, finished or stopped, run this step when a plan was produced in this run, or when a spec of the epic has a plan but no section in the epic's summary yet (a run stopped before its decisions were settled). For such a spec, read its plan documents to rebuild the summary its `DONE:` would have given. The planning summary is written only after this step's decisions are settled, so it never carries an open question.

1. **Gather cross-plan disagreements.** Compare the project-wide rules in this run's `DONE:` summaries with each other and with the sections of any summary the epic already has (`{{command}} epic summary read <epic>`). A disagreement is a rule two plans handle differently, such as one plan editing the changelog by hand while another treats it as generated.
2. **Settle each one with the user, before any plan is final.** Put each disagreement to the user as ordinary text first: the rule, every plan involved and what each does, and one proposed answer with a one-line reason. The user accepts the proposal or gives another answer. Apply the outcome to every plan involved: read each document with `{{command}} plan file read <spec> <doc>`, stage the edit under `.spektacular/tmp/`, write it back with `{{command}} plan file write <spec> <doc> --from <path>`, and remove the scratch file. Update the kept `DONE:` summaries to match. If the user declines to settle one now, enter stopping mode and write no summary: repeating the request comes back to this step.
3. **Write the summary.** For each spec planned in this run, and each one rebuilt above, stage its section and run `{{command}} epic summary write <epic> --data '{"section":"<spec>"}' --from <path>`, then remove the scratch file. A section covers the approach, the milestones and tasks, the tasks a person must do (or says there are none), what is out of scope, the drafting assumptions, the project-wide rules, and a `### Manual checks` list of what a person will verify through the test plan (or says there are none), under `###` sub-headings only, since `#` and `##` headings are refused. Sections for specs planned in earlier runs stay as they are. Then record the settled decisions with `{{command}} epic summary write <epic> --data '{"section":"decisions"}' --from <path>`: one entry per decision naming the rule, the plans involved and the outcome, after any entries already there. With no disagreement, leave the section as it is: it says "None." when empty.
4. **Order overlapping specs.** Run `{{command}} epic order <epic>`. It makes a spec depend on an earlier-listed one when their plans change the same files and nothing orders them yet, and logs each addition in the summary's "Order added for shared files" section. These dependencies are written without putting them to the user first; mention each one added in one line.

# Step 7: The end-of-planning review

When no child is running and nothing is left to start, read the summary with `{{command}} epic summary read <epic>` and walk the user through it before anything is implemented. It holds one summary entry for **every plan produced in this run**, built from each child's `DONE:` summary, beside the sections kept from earlier runs. Present it as plain text, section by section, decisions first: the decisions settled while planning, then the order added for shared files, then each plan with its manual checks.

- **An added dependency the user wants removed:** run `{{command}} epic order <epic> --data '{"unorder":{"spec":"<later>","depends_on":"<earlier>"}}'`. The summary records the removal, and planning never adds that dependency again.
- **Any other change:** Invite changes. Apply each one both to the plan document it belongs to and to that spec's section of the summary, so the two never disagree.

A change to a plan document, where `<doc>` is `plan`, `context` or `research`: read it with `{{command}} plan file read <spec> <doc>`, stage the edited document under `.spektacular/tmp/`, write it back with `{{command}} plan file write <spec> <doc> --from <path>`, and remove the scratch file. A change to the summary goes through `{{command}} epic summary write` the same way.

Offer to save any durable discovery the children reported with the `spek-knowledge` skill, and save it only if the user accepts. Close the review on a direct confirmation question.

Skip the review if no plan was produced in this run.

# Step 8: The final report

End every run, finished or stopped, with:

- **Completed in this run:** each spec planned now.
- **Skipped:** each spec that already had a plan.
- **Dependencies added for shared files:** how many `epic order` added, each named in the summary.
- **Still outstanding:** each spec not planned, and why: blocked on which specs, stopped by which failure, or waiting on a question the user chose not to answer.

When everything is planned, say the epic is ready to implement with "implement this epic".
