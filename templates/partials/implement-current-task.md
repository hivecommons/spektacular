{{#task}}
This run implements **only task `{{task.title}}`** (id `{{task.id}}`). That task is **the current task**: the `#### - [ ] Task: {{task.title}}` heading in `plan.md` whose `**Id:**` line is `{{task.id}}`, and the `### Task: {{task.title}}` section of the plan's `context.md` that its `*Technical detail:*` link points to. Work only on the current task: do not start, test, verify or tick any other task, even when other tasks are open.
{{/task}}
{{^task}}
**The current task** is the first unchecked work item in the plan's milestones section:

- In a plan whose work is written as tasks (a `## Milestones & Tasks` section), it is the first unchecked `#### - [ ] Task: <title>` heading. Its technical detail is the `### Task: <title>` section of the plan's `context.md` that its `*Technical detail:*` link points to.
- In a plan written before tasks (a `## Milestones & Phases` section), it is the first unchecked `#### - [ ] Phase N.M:` heading. Its technical detail is the matching `### Phase N.M:` section of the plan's `context.md`.
{{/task}}
