## Step {{step}}: {{title}}

The spec `{{spec_name}}` is complete and committed. Before finishing, check once whether it describes more than one independently useful piece of work.

{{#epic_name}}
This spec already belongs to the epic `{{epic_name}}`. If it is split, the new specs join that same epic: run `{{command}} epic read {{epic_name}}` to see the specs it already holds before you propose anything.
{{/epic_name}}

**A split requested earlier.** Read `.spektacular/working-context.md`. If it records that the user asked for a split while this spec was still being written, act on that request now: run the check below and, when it passes, make the offer; when it does not, tell the user plainly why this spec cannot be split.

{{> partials/split-check}}

{{#unattended}}
**This run is unattended, so do not split.** Run the check, but never take the flow below without the user. When the check finds a split worth offering, add it to the stored spec's `### Open Questions` under `## Review Notes` instead: read the spec with `{{config.command}} spec file read {{spec_name}}`, stage it under `.spektacular/tmp/` with the proposed specs and why as one bullet appended there, write it back with `{{config.command}} spec file write {{spec_name}} --from <staged file>`, and remove the scratch file. Then advance.

{{/unattended}}
**If the user agrees to a split,** follow the flow below. If there is no offer to make, or the user declines, skip straight to advancing.

{{> partials/split-flow}}

Once the split is done, declined, or not offered, advance:

```
{{config.command}} spec goto --data '{"step":"{{next_step}}"}'
```
