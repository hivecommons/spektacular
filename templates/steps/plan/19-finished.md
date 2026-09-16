## Step {{step}}: {{title}}

{{#plan_incomplete}}
⚠️ One or more plan documents are missing from the plan store, or still hold the empty scaffold. Before telling the user the workflow is done, commit the missing documents through the CLI and remove the scratch files:

```
{{config.command}} plan file write {{plan_name}}/plan.md     --from .spektacular/tmp/plan_template.md
{{config.command}} plan file write {{plan_name}}/context.md  --from .spektacular/tmp/context_template.md
{{config.command}} plan file write {{plan_name}}/research.md --from .spektacular/tmp/research_template.md
rm .spektacular/tmp/plan_template.md .spektacular/tmp/context_template.md .spektacular/tmp/research_template.md
```

If a scratch file under `.spektacular/tmp/` is gone (that path is git-ignored and does not survive a crash), re-assemble the affected document from the per-section working files under `.spektacular/work/{{plan_name}}/` before committing — they are the durable source.

Never write or edit the plan documents with the `Write` or `Edit` tools — `{{config.command}} plan file write` is the only supported way to write them. Verify each document with `{{config.command}} plan file read {{plan_name}}/<doc>.md`, then re-run this step.
{{/plan_incomplete}}
{{^plan_incomplete}}
The plan workflow is complete. Three documents are now in the plan store under `{{plan_dir}}`:

- `{{plan_path}}` — the user-scannable plan
- `{{context_path}}` — technical detail for implementation
- `{{research_path}}` — the decision log and rehydration cues

Read any of them back with `{{config.command}} plan file read {{plan_name}}/<doc>.md`.

The user signed off on the plan during the walkthrough, and the documents are now marked final. Inform the user that the plan workflow is finished and the plan is approved and ready for implementation.
{{/plan_incomplete}}
