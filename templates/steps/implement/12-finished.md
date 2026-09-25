## Step {{step}}: {{title}}

{{#task_run}}
Task `{{task.title}}` (`{{task.id}}`) of `{{plan_name}}` is complete; {{open_tasks}} task(s) in the plan remain open.

### What to do next

Report to the user:

- The task that was completed, and the acceptance criteria ticked for it in the plan.
- Any deviations recorded in the task's entry in the inline `{{changelog_section_name}}` section of the plan.
- That the feature-level wrap-up (test plan, feature changelog, spec reconciliation) runs in the implement run that completes the plan's last open task.

This is the terminal state of this implement run. Do **not** emit a `goto` command — no further steps exist.
{{/task_run}}
{{^task_run}}
The implement workflow is complete for `{{plan_name}}`.

### Summary

- All tasks in the plan under its milestones section have been checked off.
- Per-task implementation entries have been appended to the inline `{{changelog_section_name}}` section of the plan.
- A project changelog record for this feature has been written under the name `{{plan_name}}` (read it with `{{config.command}} changelog file read {{plan_name}}`), and one derived record per affected repo has been written to that repo's configured changelog directory (each opening with a user-facing summary of what shipped in that repo).
- The specification's Requirements and Acceptance Criteria have been reconciled against the completed work; read it with `{{config.command}} spec file read {{plan_name}}` for the updated checkbox state.

### What to do next

Report to the user:

- The tasks that were completed (read the `#### - [x] Task:` headings, or `#### - [x] Phase` headings in a plan written before tasks, from the plan).
- Any deviations from the plan that were recorded in the inline changelog.
- The project changelog record, named `{{plan_name}}` (read it with `{{config.command}} changelog file read {{plan_name}}`).
- The location of each affected repo's changelog record: read each one with `{{config.command}} changelog file read {{plan_name}} --repo <repo-name>` and report the path it lives at, so the user can review each repo's release note before releasing.
- The specification's completion status: read the spec with `{{config.command}} spec file read {{plan_name}}`, and tell the user which Requirements/Acceptance-Criteria items are now checked, and for any still unchecked, the reason recorded during `reconcile_spec` (deferred, descoped, or not attempted).

This is the terminal state of the implement workflow. Do **not** emit a `goto` command — no further steps exist.
{{/task_run}}
