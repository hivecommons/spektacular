## Step {{step}}: {{title}}

Mark the current task and its acceptance criteria as complete in plan.md.

### What to edit

For the current task (the one you just implemented, tested, and verified):

{{> partials/implement-current-task}}

1. Change its heading from `#### - [ ] Task: <title>` to `#### - [x] Task: <title>` (in a plan written before tasks, from `#### - [ ] Phase N.M: <title>` to `#### - [x] Phase N.M: <title>`).
2. Change each `- [ ]` acceptance-criterion checkbox in that task's `**Acceptance criteria**:` block to `- [x]` **only if** that criterion actually passed verification in the previous step.
3. Leave criteria that did not pass as `- [ ]`. Do not mark them complete just because the task is "mostly done".{{#task}} Tick nothing belonging to any other task.{{/task}}

### How to apply the edit

The plan documents are owned by spektacular. **Never edit plan.md with the `Write` or `Edit` tools** — read and write it through the CLI:

1. Read the current plan.md: `{{config.command}} plan file read {{plan_name}} plan`.
2. Apply the checkbox changes above to the content you read.
3. Stage the updated plan.md with the `Write` tool at the scratch path `.spektacular/tmp/plan_update.md`, point `plan file write` at it with `--from`, then remove the scratch file:

   ```
   {{config.command}} plan file write {{plan_name}} plan --from .spektacular/tmp/plan_update.md
   rm .spektacular/tmp/plan_update.md
   ```

### STOP-on-mismatch

If any acceptance criterion passed verification but describes an outcome that no longer matches what the code actually does (e.g. the criterion says "function X returns Y" but the implementation returns Z and the user authorized the change), STOP. The plan must be updated to reflect the new reality before the checkbox can flip. Ask the user whether to (a) update the criterion text in plan.md first, (b) leave the checkbox unchecked and note the deviation for the changelog, or (c) flip it anyway because the user accepts the divergence.

### Advance

Once checkboxes are updated and committed:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
