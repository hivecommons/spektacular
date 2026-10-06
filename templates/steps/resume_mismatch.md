## A different workflow is already in progress

You tried to run a **{{requested_kind}}** command, but an unfinished **{{kind}}** workflow (`{{name}}`) is already in progress — it stopped at step **`{{current_step}}`**. A **{{requested_kind}}** workflow cannot resume or operate on a **{{kind}}** workflow. Nothing has been changed on disk.

Tell the user a **{{kind}}** workflow is in progress, and ask which they want.

### To continue the {{kind}} workflow

Switch to the **{{kind}}** skill and resume it where it stopped:

```
{{config.command}} {{kind}} goto --data '{"step":"{{current_step}}"{{#goto_name}},"name":"{{goto_name}}"{{/goto_name}}}'
```

### To discard it and start a new {{requested_kind}} workflow

This overwrites the in-progress **{{kind}}** workflow's state (recoverable via git if needed), then starts fresh:

```
{{config.command}} {{requested_kind}} new --force
```

Do not resume the {{kind}} workflow as if it were a {{requested_kind}} workflow, and do not overwrite it without the user's go-ahead.
