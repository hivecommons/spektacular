## Step {{step}}: {{title}}

Write the code for the current task.

### Step 1: Load the current task

Do not rely on an earlier step's output still being in your context. Read the task from the plan itself:

{{> partials/implement-current-task}}

1. Run `{{config.command}} plan file read {{plan_name}}/plan.md` and find the current task's heading.
2. Run `{{config.command}} plan file read {{plan_name}}/context.md` and read the current task's section of the plan's `context.md` in full.

If the section is missing, unreadable, or empty, STOP and ask the user whether to fix the plan's `context.md` before proceeding. This is a plan/reality mismatch — do not guess.

### Step 2: Write the code

Use the task's section and the codebase research from `analyze` as your map, not as a narrative to follow verbatim.

### Rules

- **Follow existing patterns.** Match the shape of nearby code. If the task's integration points use a particular idiom, use the same idiom for consistency.
- **No tests in this step.** Test authoring is the next step and runs in a dedicated sub-agent context. Do not write `*_test.go` files yet.
- **No speculative abstractions.** Implement what the task says, not what you think the task should have said.
- **Small, readable diffs.** If a single task balloons beyond the scope described in its section of the plan's `context.md`, STOP and ask the user whether to split it.

### STOP-on-mismatch

If the code you need to modify has drifted materially from what the plan describes (file renamed, function signature changed, type removed, import path moved), STOP before making any change. Report the mismatch to the user with a concrete description of what the plan expected and what you found. Ask the user to pick one of three options:

1. **Fix the plan first.** Update the plan's `plan.md` and/or `context.md` through `{{config.command}} plan file write`, then restart this step.
2. **Proceed with an agreed-upon substitution.** The user tells you how to map stale names to current names.
3. **Skip this task.** {{#task}}End this run without implementing it.{{/task}}{{^task}}Return to the task-selection step and pick the next unchecked task.{{/task}}

Do not silently adapt — visible drift must be acknowledged.

### Advance

Once the code for the current task compiles:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
