## Step {{step}}: {{title}}

{{#spec_unwritten}}
⚠️ The spec `{{spec_name}}` still holds the empty scaffold — the completed spec was never committed to the store.

Before telling the user the workflow is done, write the spec through Spektacular. If your assembled `.spektacular/tmp/spec_template.md` is gone (that scratch path is git-ignored and does not survive a crash), re-assemble it from the per-section working files under `.spektacular/work/{{spec_name}}/` — they are the durable source. Stage the completed spec to `.spektacular/tmp/spec_template.md` with the `Write` tool, point `spec file write` at it with `--from`, then remove the scratch file:

```
{{config.command}} spec file write {{spec_name}} --from .spektacular/tmp/spec_template.md
rm .spektacular/tmp/spec_template.md
```

Never edit the spec file with the `Write` or `Edit` tools — `{{config.command}} spec file write` is the only supported way to write it.

Then run this step again, so it records the spec's status:

```
{{config.command}} spec goto --data '{"step":"{{step}}"}'
```
{{/spec_unwritten}}
{{^spec_unwritten}}
The spec is complete.

{{^unattended}}
Inform the user that the spec workflow is finished and the spec file is ready to use.
{{/unattended}}
{{#unattended}}
This run was unattended, so the spec is stored as a draft marked `unreviewed`. The plan workflow refuses to start from it until the user signs it off.
{{/unattended}}
{{#epic_name}}

**Offer the next item in the epic.** This spec belongs to the epic `{{epic_name}}`. Look for the next piece of work in it that has no spec yet:

1. Run `{{command}} epic read {{epic_name}}` to see the specs the epic already lists.
2. Re-read the epic's source with your own tools{{#epic_has_sources}}: {{#epic_sources}}`{{{.}}}` {{/epic_sources}}{{/epic_has_sources}}{{^epic_has_sources}} (the epic records no source; if it was not started from one, there is nothing to chain to, so skip this offer){{/epic_has_sources}}. Spektacular prescribes no tool. Collect its child items (sub-issues, linked children, task-list entries), including any added since the epic was created.
3. Find the first child item, in the source's own order, that no spec in the epic was started from (each such spec records that item in its `sources`).

{{^unattended}}
If there is one, offer to start a spec for it in this epic, naming the item. Never start it without the user's agreement. On agreement, propose a name from the item's title and run:

```
{{command}} spec new --data '{"name":"<name>","sources":[{"uri":"<the child item's link>"}],"epic":"{{epic_name}}"}'
```

If every child item already has a spec, or the user declines, the workflow is simply finished. The offer stops at specifying: do not offer to plan or implement the next spec.
{{/unattended}}
{{#unattended}}
The user handed the epic off, so do not offer: if there is one, start its spec now, unattended as well, with a name taken from the item's title, and drive that workflow to its end the same way:

```
{{command}} spec new --data '{"name":"<name>","sources":[{"uri":"<the child item's link>"}],"epic":"{{epic_name}}","unattended":true}'
```

Keep going until every child item has a spec. Only then give the summary below, once, covering every spec this hand-off wrote. Chaining stops at specifying: do not plan or implement any of them.
{{/unattended}}
{{/epic_name}}
{{#unattended}}

**Give the user one summary, now, and nothing before it.** Find the specs this hand-off wrote with `{{command}} spec file list` (each carries `"unreviewed": true`){{#epic_name}} and `{{command}} epic read {{epic_name}}`{{/epic_name}}. For each one, read it back and report:

- its name and the command that shows it, `{{command}} spec file read <name>`;
- the assumptions and the open questions listed under its `## Review Notes`, so the user knows where to look.

Then tell the user how the review goes. Walk through a spec's review notes with them, apply what they change through `{{command}} spec file write`, remove the `## Review Notes` section, and sign the spec off with `{{command}} spec file set-document-status <name> --document-status final`. Signing it off clears the `unreviewed` mark, and only then can it be planned.
{{/unattended}}
{{/spec_unwritten}}
