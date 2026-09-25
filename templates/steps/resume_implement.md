## Resume: an in-progress implement workflow was found

An unfinished **implement** workflow (`{{name}}`) is already in progress. It stopped at step **`{{current_step}}`**. Nothing has been changed on disk.
{{#task}}

This run implements **only task `{{task}}`** of the plan. A resumed session still reads the whole plan, but works on, tests, verifies and ticks that one task alone.
{{/task}}

Ask the user whether to **resume** the existing workflow or **start a new one**, then follow the matching path below.

### To resume from where it stopped

1. **Read the plan before anything else.** Do this first, whichever step the run stopped at. Here `<plan_name>` is `{{name}}`.

   {{> partials/implement-plan-documents}}

2. Read `.spektacular/working-context.md` for the previous session's learnings and the answers the user gave to your questions. It is a session log, not the plan.
3. Find the current task from the plan{{#task}}: task `{{task}}`, the `#### - [ ] Task:` heading in `plan.md` whose `**Id:**` line is `{{task}}`{{/task}}{{^task}}: the first unchecked `#### - [ ] Task:` heading under `## Milestones & Tasks` in `plan.md`, or the first unchecked `#### - [ ] Phase N.M:` heading under `## Milestones & Phases` in a plan written before tasks{{/task}}. Its technical detail is the matching `### Task: <title>` (or `### Phase N.M:`) section of the plan's `context.md`.
4. Continue the interrupted step:

   ```
   {{config.command}} implement goto --data '{"step":"{{current_step}}"}'
   ```

### To discard it and start fresh

This overwrites the in-progress workflow's state (recoverable via git if needed):

```
{{config.command}} implement new --force
```
