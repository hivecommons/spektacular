## Step {{step}}: {{title}}

Draft the **Data Structures & Interfaces** section of `plan.md`. This section captures the types, interface signatures, and serialization boundaries introduced or changed by the plan.

### Rules

- Focus on contracts between components, not internal representation detail.
- Show type shapes in pseudocode or a short code block where it helps — but keep it concise; this is a plan, not source.
- Name each type or interface and describe its role in one or two sentences.
- If no new data structures or interfaces are introduced, say so explicitly — an empty section is not acceptable.
- Per-field implementation detail (defaults, validation, wire-format nuance) belongs in the plan's `context.md`, not here.

### What to produce

A draft Data Structures & Interfaces section ready to drop into plan.md at verification time.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Data Structures & Interfaces** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/data_structures.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted contracts are saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
