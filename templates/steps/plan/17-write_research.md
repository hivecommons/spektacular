## Step {{step}}: {{title}}

Commit the staged **research.md** to the plan store — the last of the three documents. Never write or edit the plan documents with the `Write` or `Edit` tools — `{{config.command}} plan file write` is the only supported way to write them.

{{#research_unwritten}}
research.md was assembled and staged at `.spektacular/tmp/research_template.md` in the assemble step. Commit it now, then remove the scratch file:

```
{{config.command}} plan file write {{plan_name}} research --from .spektacular/tmp/research_template.md
rm .spektacular/tmp/research_template.md
```

If the `.spektacular/tmp/research_template.md` scratch file is gone (that path is git-ignored and does not survive a crash), re-assemble it from the per-section working files under `.spektacular/work/{{plan_name}}/` before committing — they are the durable source.
{{/research_unwritten}}
{{^research_unwritten}}
research.md has already been committed to the plan store.
{{/research_unwritten}}

All three plan documents are now in the plan store. The per-section working files have served their purpose — remove the working directory with your own file tools:

```
rm -rf .spektacular/work/{{plan_name}}
```

Then advance to the finished step:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
```
