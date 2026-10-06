## Step {{step}}: {{title}}

The spec `{{spec_name}}` is complete and committed. Before finishing, check once whether it describes more than one independently useful piece of work.

{{#epic_name}}
This spec already belongs to the epic `{{epic_name}}`. If it is split, the new specs join that same epic: run `{{command}} epic read {{epic_name}}` to see the specs it already holds before you propose anything.
{{/epic_name}}

**A split requested earlier.** Read `.spektacular/working-context.md`. If it records that the user asked for a split while this spec was still being written, act on that request now: run the check below and, when it passes, make the offer; when it does not, tell the user plainly why this spec cannot be split.

{{> partials/split-check}}

**If the user agrees to a split,** follow the flow below. If there is no offer to make, or the user declines, skip straight to advancing.

{{> partials/split-flow}}

Once the split is done, declined, or not offered, advance:

```
{{config.command}} spec goto --data '{"step":"{{next_step}}"}'
```
