## Resume: an in-progress {{kind}} workflow was found

An unfinished **{{kind}}** workflow (`{{name}}`) is already in progress. It stopped at step **`{{current_step}}`**. Nothing has been changed on disk.

{{^orchestrated}}
Ask the user whether to **resume** the existing workflow or **start a new one**, then follow the matching path below.
{{/orchestrated}}
{{#orchestrated}}
This is an orchestrated run, kept in its own lane: do not ask the user. Resume it, following the path below, unless your orchestrator told you to start it afresh.
{{/orchestrated}}

### To resume from where it stopped

First re-read everything the previous session left behind so you pick up its accumulated work without re-asking the user (use your own file tools — these working files are git-tracked and agent-owned):

1. If this is a **spec** or **plan** workflow, read every per-section working file under `.spektacular/work/{{name}}/` — these hold the content of sections already completed, so you do not gather them again. (The **implement** workflow has no such directory, and neither does a **repo** add: every answer it has already agreed travels inside the workflow itself and comes back with it.)
2. Read `{{notes_path}}` for the cross-cutting learnings and the answers the user gave to your questions.
3. Run `{{config.command}} repo list` for the registered repos and the `root` each one's code lives at; that is your map for any code you research, cite, or change from here. Never assume the directory you started in is a repo's code.
4. Then re-present the interrupted step and continue:

   ```
   {{config.command}} {{kind}} goto --data '{"step":"{{current_step}}"{{#goto_name}},"name":"{{goto_name}}"{{/goto_name}}}'
   ```

   This re-emits the `{{current_step}}` instruction without losing any completed work.

### To discard it and start fresh

This overwrites the in-progress workflow's state (recoverable via git if needed):

```
{{config.command}} {{kind}} new --force{{#orchestrated}} --data '{"name":"{{name}}","orchestrated":true}'{{/orchestrated}}
```
