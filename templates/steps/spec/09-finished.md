## Step {{step}}: {{title}}

{{#spec_unwritten}}
⚠️ The spec `{{spec_name}}` still holds the empty scaffold — the completed spec was never committed to the store.

Before telling the user the workflow is done, write the spec through Spektacular. If your assembled `.spektacular/tmp/spec_template.md` is gone (that scratch path is git-ignored and does not survive a crash), re-assemble it from the per-section working files under `.spektacular/work/{{spec_name}}/` — they are the durable source. Stage the completed spec to `.spektacular/tmp/spec_template.md` with the `Write` tool, point `spec file write` at it with `--from`, then remove the scratch file:

```
{{config.command}} spec file write {{spec_name}} --from .spektacular/tmp/spec_template.md
rm .spektacular/tmp/spec_template.md
```

Never edit the spec file with the `Write` or `Edit` tools — `{{config.command}} spec file write` is the only supported way to write it.
{{/spec_unwritten}}
{{^spec_unwritten}}
The spec is complete.

Inform the user that the spec workflow is finished and the spec file is ready to use.
{{/spec_unwritten}}
