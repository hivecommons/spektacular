## Step {{step}}: {{title}}

This step is the **validation and drift gate** for the implement workflow. Nothing else runs until it passes. If any check below fails, STOP and report to the user with a three-option prompt — do not silently continue past a failed check.

**Where the code lives.** Run `{{config.command}} repo list` now if you have not already: it reports each registered repo and the `root` its code lives at. For the rest of this workflow, carry out every code-touching step — analysis, implementation, tests, verification — in the `root` reported for the repo the work belongs to, never in whatever directory you started in, and pass that `root` to any sub-agent you launch.


### Step 1: Full plan read

Read the three plan documents **in full** through the plan store. The plan documents are the implement workflow's own live artifact — this workflow owns them while it is actively running, which is why reading them here fits the AGENTS.md "specs and plans are historical" rule rather than breaking it. The plan documents are owned by spektacular — always read them with `{{config.command}} plan file read`, never with the `Read` tool, which bypasses the CLI.

{{> partials/implement-plan-documents}}

Here `<plan_name>` is `{{plan_name}}`. Read all three now, before any check below.
{{#task}}

This run implements **only task `{{task.title}}`** (id `{{task.id}}`). Still read the whole plan, its `context.md`, its `research.md` and every design document the plan references: a fact the task depends on may be stated only there. Only the implementing, testing, verifying and ticking that follow are limited to this one task.
{{/task}}

These are the source of truth for every downstream step.

### Step 2: Structural validation

Verify the plan has the complete plan-scaffold shape. Every one of these `## ` sections must be present:

1. `## Overview`
2. `## Architecture & Design Decisions`
3. `## Component Breakdown`
4. `## Data Structures & Interfaces`
5. `## Implementation Detail`
6. `## Dependencies`
7. `## Testing Approach`
8. `## Milestones & Tasks` (`## Milestones & Phases` in a plan written before tasks)
9. `## Open Questions`
10. `## Out of Scope`

Then verify the task structure:

- At least one unchecked work item exists under the milestones section: a `#### - [ ] Task: <title>` heading, or a `#### - [ ] Phase N.M:` heading in a plan written before tasks.
- In the plan's `plan.md`, every task (or phase) has a `*Technical detail:*` link into the plan's `context.md`, in the form `[context.md#task-<slug>](./context.md#...)` (`[context.md#phase-NM](./context.md#...)` for a phase).
- Every `*Technical detail:*` link target resolves to a matching `### Task: <title>` (or `### Phase N.M:`) heading inside the plan's `context` document.

If any structural check fails, STOP and report the failures to the user.

### Step 3: Drift check against each repo's source

For every **file path**, **package path**, **function name**, **type name**, **command path**, and **template path** named in the plan or the plan's `context` document (including inside code blocks and in `file:line` references), verify the target still exists in the codebase — checked in the `root` of the repo the reference belongs to (a `**Repo:**` line or a `<repo-name>: ` prefix says which; `{{config.command}} repo list` says where).

**Method**:

- For file/directory paths: use `ls` or attempt to read the file.
- For Go symbols and package import paths: use `grep -rn` or delegate to a codebase-locator sub-agent.
- For CLI commands like `{{config.command}} <something>`: check `cmd/` wiring.
- For template paths like `templates/steps/plan/01-overview.md`: check the file exists.

Collect **every** mismatch into a list — do not fix them silently as you find them.

If the list is non-empty, STOP and report all mismatches to the user in one block. Ask the user to pick one of three options:

1. **Fix the plan first.** Update the plan and/or the plan's `context` document to match the current codebase, then restart this step.
2. **Proceed with the mismatches noted in memory.** The agent will adapt during implementation, mapping stale pointers to their current equivalents on the fly.
3. **Abandon the workflow.** Stop the implement run entirely.

Do **not** continue to Step 3.5 until the user has picked an option.

### Step 3.5: Spec coverage check

Read the specification the plan was created from:

```
{{config.command}} spec file read {{plan_name}}
```

Enumerate every `- [ ]`/`- [x]` checkbox under the spec's `## Requirements` and `## Acceptance Criteria` headings. For each one, confirm it has corresponding coverage somewhere in the plan's milestones section — a task summary, an acceptance criterion, or a technical-detail note that addresses it (paraphrase or explicit mention both count; it does not need to be a verbatim match).

Before flagging any gap, check whether it is already recorded as accepted: look for a `**Descoped requirements**:` list under the milestones section in the plan (see the marker format below). A requirement or acceptance criterion listed there is already resolved — do not re-flag it.

If every remaining spec item has coverage (or is already marked descoped), proceed to Step 4 without interruption.

If one or more spec items have no coverage and are not already marked descoped, STOP and report the missing items to the user in one block, quoting each item's checkbox text. Ask the user to pick one of two options:

1. **Fix the plan first.** Update the plan to add the missing coverage, then restart this step.
2. **Accept the gap as descoped.** Record it using the marker format below, then continue to Step 4.

**Descoped marker format** — when the user accepts a gap, append (or add to an existing) `**Descoped requirements**:` list near the end of the plan's milestones section, one bullet per accepted gap:

```
**Descoped requirements**:
- <requirement/acceptance-criterion short title> — descoped: <one-line reason>
```

Apply the edit by reading the plan with `{{config.command}} plan file read {{plan_name}} plan`, adding or extending the list, staging the updated document with the `Write` tool at the scratch path `.spektacular/tmp/plan_update.md`, then committing it and removing the scratch file:

```
{{config.command}} plan file write {{plan_name}} plan --from .spektacular/tmp/plan_update.md
rm .spektacular/tmp/plan_update.md
```

Do **not** continue to Step 4 until every gap is either fixed in the plan or recorded as descoped.

### Step 4: Changelog mode detection

Check whether a `{{changelog_section_name}}` section already exists inside the plan.

- **Present** → this is a **subsequent-task** invocation. Later steps will append new task entries under the existing section. During `analyze`, pick up at {{#task}}the selected task{{/task}}{{^task}}the first unchecked `#### - [ ]` task in the plan{{/task}}.
- **Absent** → this is a **first-task** invocation. The `update_changelog` step will create the section on first use. During `analyze`, pick up at {{#task}}the selected task{{/task}}{{^task}}the first `#### - [ ]` task (which will be the first one, unless the user has partially checked off tasks manually){{/task}}.

### Advance

Once validation passes, drift is resolved, and changelog mode is known:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
