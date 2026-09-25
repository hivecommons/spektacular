## Step {{step}}: {{title}}

Commit the plan's staged **context.md** to the plan store. Never write or edit the plan documents with the `Write` or `Edit` tools — `{{config.command}} plan file write` is the only supported way to write them.

{{#context_unwritten}}
The plan's `context.md` was assembled and staged at `.spektacular/tmp/context_template.md` in the assemble step. Commit it now, then remove the scratch file:

```
{{config.command}} plan file write {{plan_name}} context --from .spektacular/tmp/context_template.md
rm .spektacular/tmp/context_template.md
```

If the `.spektacular/tmp/context_template.md` scratch file is gone (that path is git-ignored and does not survive a crash), re-assemble it from the per-section working files under `.spektacular/work/{{plan_name}}/` before committing — they are the durable source.
{{/context_unwritten}}
{{^context_unwritten}}
The plan's `context.md` has already been committed to the plan store. If `.spektacular/tmp/context_template.md` is still present, remove it.
{{/context_unwritten}}

Then advance:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
```
