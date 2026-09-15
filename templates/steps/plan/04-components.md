## Step {{step}}: {{title}}

Draft the **Component Breakdown** section of `plan.md`. This section lists the components (new or changed) that make up the chosen solution, their responsibilities, and how they interact.

### Rules

- One bullet or short paragraph per component.
- For each component: name it, state what it owns, and describe its relationship to the other components.
- Do NOT list file paths or line numbers in plan.md — those belong in the plan's `context.md`. Component responsibilities, not implementation sites.
- Reuse existing components wherever possible; a new component needs justification.
- Cover both new components and meaningfully-changed existing ones.

### What to produce

A draft Component Breakdown section ready to drop into plan.md at verification time.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Component Breakdown** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/components.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted component list is saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
