## Step {{step}}: {{title}}

Append a task entry to the inline `{{changelog_section_name}}` section of plan.md, then {{#task}}finish this task run{{/task}}{{^task}}decide whether to loop back for another task or advance to the repo-level changelog{{/task}}.

The plan documents are owned by spektacular. **Never read or edit plan.md with the `Read`, `Write`, or `Edit` tools** — go through `{{config.command}} plan file read` and `{{config.command}} plan file write`.

### Step 1: Ensure the `{{changelog_section_name}}` section exists

Start from plan.md as you wrote it back in `update_plan`; read it with `{{config.command}} plan file read {{plan_name}} plan` only if that content is not in your context (after a resume or a context compaction). If the `{{changelog_section_name}}` heading is absent, this is the first `update_changelog` invocation for this plan — append a new `{{changelog_section_name}}` section **after** the existing `## Out of Scope` section (or at the very end of the file if `## Out of Scope` is missing).

If the `{{changelog_section_name}}` heading is present, append new entries under the existing section, after any entries already there.

### Step 2: Write the task entry

For the task you just completed, append an entry with this shape (in a plan written before tasks, the heading names the phase instead: `### <YYYY-MM-DD> — Phase N.M: <phase title>`):

```
### <YYYY-MM-DD> — Task: <task title>

**What was done**: <1-3 sentences summarizing the code change in plain language>

**Deviations**: <anything that didn't match the plan, or "None" explicitly>

**Amendments**: <the document and section amended, and why; omit this line when no amendment was applied during this task>

**Files changed**:
- `path/to/file.go`
- `path/to/another/file.go`
- `<repo-name>: path/within/that/repo.go`

**Discoveries**: <anything the next task or a future maintainer should know — a tricky API, a hidden constraint, a renamed symbol, a missed edge case>
```

{{#goal_check}}
If the goal check stopped this task and the user accepted the shortfall or skipped the task (`{{config.command}} implement goal show {{plan_name}}` shows its state as `accepted shortfall` or `skipped`), name that under **Deviations**, with the criteria left unticked.

{{/goal_check}}
In the **Files changed** list, prefix every path with its repo's name (`<repo-name>: path`) whenever more than one repo is registered; an unprefixed path belongs to the only registered repo. The final feature-changelog step derives one entry per affected repo mechanically from these prefixes, so keep them accurate.

{{#has_dependency_override}}
### Record the dependency override

This run started past dependencies that were not implemented yet: the user chose to continue anyway. Record it under **Deviations** in this entry, stating that implementation started before these dependencies were implemented, naming each and the state it was in:

{{#dependency_override}}
- `{{name}}` — {{state}}
{{/dependency_override}}

{{/has_dependency_override}}
If you need the exact format and more examples, read the skill below, once per session rather than once per task. Do not launch a sub-agent for it:

```
{{config.command}} skill update-changelog
```

Apply the entry by adding it to that plan.md content, staging the updated document with the `Write` tool at the scratch path `.spektacular/tmp/{{plan_name}}/plan_update.md`, then committing it and removing the scratch file:

```
{{config.command}} plan file write {{plan_name}} plan --from .spektacular/tmp/{{plan_name}}/plan_update.md
rm .spektacular/tmp/{{plan_name}}/plan_update.md
```

### Step 2b: Assess discoveries for durable knowledge

{{^orchestrated}}
Re-read the `**Discoveries**` entry you just wrote and weigh each item for durability — does it hold beyond this one change? Durable means something a future plan or implementation would pay to re-discover: an undocumented coupling, a surprising contract, a convention inferred from behavior rather than written down anywhere, a wrong default that had to be corrected. Only items that are both durable and non-obvious beyond the current change qualify — most tasks produce none, and when nothing qualifies, say nothing about knowledge capture and continue straight to Step 3.

When a discovery does clear that bar, do not offer it now, and do not stop the run for it. Add it, phrased as the general lesson rather than the specific edit, to a `## Knowledge candidates` list in `{{working_context_path}}`. The run's `finished` step offers every candidate to the user at once. The `**Discoveries**` entry itself stays exactly as written; the candidate is additive on top of it.

{{/orchestrated}}
{{#orchestrated}}
Re-read the `**Discoveries**` entry you just wrote. This run is orchestrated, so do not offer to save anything yourself: note any item that is durable and non-obvious beyond this change, and list it in your closing summary for the orchestrator to raise with the user. Most tasks produce none.

{{/orchestrated}}
{{#task}}
### Step 3: Finish this task run

This run implements only task `{{task.title}}` (`{{task.id}}`). It does not loop on to another task.

{{#last_task}}
No open task remains in the plan: this run completed the plan's last open task, so it also carries out the feature-level wrap-up. Advance to `test_plan` to write the manual test plan; the feature-changelog and spec-reconciliation steps follow it:

```
{{config.command}} implement goto --data '{"step":"test_plan","name":"{{plan_name}}"}'
```
{{/last_task}}
{{^last_task}}
Other tasks in the plan are still open, so the feature-level wrap-up (test plan, feature changelog, spec reconciliation) is left to the run that completes the last of them. Advance straight to `finished`:

```
{{config.command}} implement goto --data '{"step":"finished","name":"{{plan_name}}"}'
```
{{/last_task}}

{{/task}}
{{^task}}
### Step 3: Check for remaining unchecked tasks

In the plan.md you just wrote, count the unchecked `#### - [ ] Task:` headings under `## Milestones & Tasks` (`#### - [ ] Phase` headings under `## Milestones & Phases` in a plan written before tasks).{{#goal_check}} Do not count any task or phase that `{{config.command}} implement goal show {{plan_name}}` reports as skipped.{{/goal_check}}

**If unchecked tasks remain**:

{{^orchestrated}}
- By default, do not ask whether to continue: loop on to the next task automatically. A one-line note of the task just completed and the next one is enough.
- Pause here only if the user asked you to: to stop after this task, after a given task or milestone, or between tasks for review. Then report the task completed and the next one, and wait for the user before looping.
{{/orchestrated}}
{{#orchestrated}}
- This run is orchestrated: do not ask whether to continue. Loop on to the next task automatically.
{{/orchestrated}}
- To loop, advance to `analyze` — this uses the multi-source FSM transition that lets `analyze` be reached from `update_changelog`:

  ```
  {{config.command}} implement goto --data '{"step":"analyze","name":"{{plan_name}}"}'
  ```

**If no unchecked tasks remain**:

- This was the last task. Advance to `test_plan` to write the manual test plan; the feature-changelog step that follows it writes the project record and one user-facing record per affected repo:

  ```
  {{config.command}} implement goto --data '{"step":"test_plan","name":"{{plan_name}}"}'
  ```

{{/task}}
### STOP-on-mismatch

If the plan file's state after Step 2 is inconsistent (e.g. you just wrote a changelog entry for a task but that task's checkbox is still unchecked after the previous `update_plan` step), STOP and report the inconsistency to the user.
