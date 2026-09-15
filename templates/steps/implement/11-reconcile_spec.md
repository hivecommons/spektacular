## Step {{step}}: {{title}}

Reconcile the specification's own Requirements and Acceptance Criteria against what was actually built. This step runs after the feature changelog record is written, so the full implementation history is available to judge each checkbox against.

### Step 1: Gather source material

Read the feature's spec and the plan's accumulated implementation history through their respective stores — never with the `Read` tool:

```
{{config.command}} spec file read {{plan_name}}.md
{{config.command}} plan file read {{plan_name}}/plan.md
```

From the spec, take every `- [ ]`/`- [x]` checkbox under `## Requirements` and `## Acceptance Criteria`. From the plan, take the `{{changelog_section_name}}` section — the phase-by-phase implementation audit log, including each phase's **What was done**, **Deviations**, **Files changed**, and **Discoveries** entries. This is the record of what was actually built; judge against it rather than re-deriving anything from the original plan text.

### Step 2: Judge each checkbox

For every spec checkbox still unchecked, decide whether the plan's `{{changelog_section_name}}` entries show it is genuinely satisfied:

- **Satisfied** — a phase entry's "What was done" (or a combination of entries) directly delivers what the checkbox describes. Mark it `[x]`.
- **Unsatisfied** — nothing in the changelog record addresses it, or a "Deviations" entry explicitly says it was dropped, deferred, or descoped. Leave it `[ ]`.

Do not mark a checkbox satisfied on a partial or speculative match — if in doubt, leave it unchecked. A checkbox already `[x]` in the spec stays `[x]`.

### Step 3: Write the updated spec

The spec is a spec-store artifact. **Never write it with the `Write`/`Edit` tools directly into the spec directory** — stage it, then commit it through the CLI:

1. Use the `Write` tool to stage the full updated spec content (with satisfied checkboxes flipped to `[x]`, everything else unchanged) at the scratch path `.spektacular/tmp/spec_reconcile.md`.
2. Commit it and remove the scratch file:

   ```
   {{config.command}} spec file write {{plan_name}}.md --from .spektacular/tmp/spec_reconcile.md
   rm .spektacular/tmp/spec_reconcile.md
   ```

After the write succeeds, confirm it with `{{config.command}} spec file read {{plan_name}}.md`.

### STOP-on-mismatch

If the feature's spec cannot be found under `{{plan_name}}.md`, or the plan's `{{changelog_section_name}}` section is missing or empty, STOP and report it to the user: ask whether to (a) locate the correct spec name and retry, (b) skip reconciliation and leave the spec untouched, or (c) proceed with only the partial record available. Do not guess at satisfaction from nothing.

### Advance

Once the spec is reconciled and confirmed:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
