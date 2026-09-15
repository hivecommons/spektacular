## Step {{step}}: {{title}}

Draft the **Dependencies** section of `plan.md`. List the internal packages, external libraries, upstream specs, and prior plans this work depends on.

### Rules

- One bullet per dependency.
- For each: a one-line note on what it provides and whether it needs any changes.
- Cover both runtime dependencies (imported packages, external services) and planning dependencies (prior specs or plans that must land first).
- If a dependency must land or change before this plan starts, flag that explicitly.
- If there are no meaningful dependencies, say so explicitly — an empty section is not acceptable.

### What to produce

A draft Dependencies section ready to drop into plan.md at verification time.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Dependencies** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/dependencies.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted dependency list is saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
