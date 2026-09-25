## Step {{step}}: {{title}}

Identify the current task, then research the codebase touchpoints before writing any code.

### Step 1: Pick the current task

Re-read plan.md through the plan store — `{{config.command}} plan file read {{plan_name}}/plan.md` — and locate the current task.

{{> partials/implement-current-task}}

Record its title (and its id, or its phase number in an older plan) and its `*Technical detail:*` link to a section in the plan's `context.md`.

{{#task}}
If the current task is already checked, STOP and ask the user what to do.
{{/task}}
{{^task}}
If every task is already checked, STOP — this should only happen if the user manually advanced the workflow past `update_changelog` without looping. Report the situation and ask the user what to do.
{{/task}}

### Step 2: Read the task's technical detail

Read the plan's `context.md` through the plan store — `{{config.command}} plan file read {{plan_name}}/context.md` — and find the section the `*Technical detail:*` link points to. Read the entire section. It should contain file:line references, complexity, token estimate, and an agent strategy.

Always read the plan documents with `{{config.command}} plan file read`, never with the `Read` tool. If the section is missing, unreadable, or empty, STOP and ask the user whether to fix the plan's `context.md` before proceeding. This is a plan/reality mismatch — do not guess.

### Step 3: Delegate codebase research to sub-agents

For non-trivial tasks (Medium or High complexity), delegate the codebase research to sub-agents running in parallel. For Low-complexity tasks, you can do it yourself in the main context. Use the skill below for orchestration guidance:

```
{{config.command}} skill spawn-implementation-agents
```

The research should cover:

1. Every file:line reference listed in the task's section of the plan's `context.md` — confirm each still exists and the line numbers are approximately correct (drift may have moved them slightly).
2. The integration points where new code will sit — imports, callers, interfaces that need to be satisfied.
3. Existing patterns to follow — similar implementations elsewhere in the codebase that the new code should match in shape.
4. Tests that will need to be updated or added.

Each sub-agent should return a concise summary (not full file dumps) that the main agent can use as a reference when writing code.

### Step 4: STOP-on-mismatch

If any sub-agent reports that a file referenced by the task no longer exists, or that a named function/type/package has been renamed or removed, STOP immediately. Report the mismatches to the user and ask whether to (a) fix the plan first, (b) proceed with an agreed-upon substitution, or (c) skip this task.

### Advance

Once analysis is complete and you have a clear picture of the files, patterns, and integration points for the current task:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
