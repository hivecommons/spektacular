## Resume: an in-progress implement workflow was found

An unfinished **implement** workflow (`{{name}}`) is already in progress. It stopped at step **`{{current_step}}`**. Nothing has been changed on disk.

Ask the user whether to **resume** the existing workflow or **start a new one**, then follow the matching path below.

### To resume from where it stopped

1. **Read the plan before anything else.** Do this first, whichever step the run stopped at. Here `<plan_name>` is `{{name}}`.

   {{> partials/implement-plan-documents}}

2. Read `.spektacular/working-context.md` for the previous session's learnings and the answers the user gave to your questions. It is a session log, not the plan.
3. Find the current phase from the plan: the first unchecked `#### - [ ] Phase N.M:` heading under `## Milestones & Phases` in `plan.md`. Its technical detail is the matching `### Phase N.M:` section of the plan's `context.md`.
4. Continue the interrupted step:

   ```
   {{config.command}} implement goto --data '{"step":"{{current_step}}"}'
   ```

### To discard it and start fresh

This overwrites the in-progress workflow's state (recoverable via git if needed):

```
{{config.command}} implement new --force
```
