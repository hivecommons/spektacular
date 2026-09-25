## Step {{step}}: {{title}}

Commit the staged **plan.md** to the plan store. The plan documents are owned by spektacular — **never write or edit them with the `Write` or `Edit` tools**. `{{config.command}} plan file write` is the only supported way to write them; it routes the write through the CLI into the configured plan directory.

{{#plan_unwritten}}
plan.md was assembled and staged at `.spektacular/tmp/plan_template.md` in the assemble step. Commit it now, then remove the scratch file:

```
{{config.command}} plan file write {{plan_name}} plan --from .spektacular/tmp/plan_template.md
rm .spektacular/tmp/plan_template.md
```

If the `.spektacular/tmp/plan_template.md` scratch file is gone (that path is git-ignored and does not survive a crash), re-assemble it from the per-section working files under `.spektacular/work/{{plan_name}}/` before committing — they are the durable source.
{{/plan_unwritten}}
{{^plan_unwritten}}
plan.md has already been committed to the plan store. If `.spektacular/tmp/plan_template.md` is still present, remove it.
{{/plan_unwritten}}

Then advance:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
```
