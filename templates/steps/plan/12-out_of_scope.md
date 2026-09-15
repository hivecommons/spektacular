## Step {{step}}: {{title}}

Draft the **Out of Scope** section of `plan.md`. This section lists the explicit exclusions agreed during planning — what this plan is NOT doing, and where those things are tracked instead.

### Rules

- One bullet per exclusion.
- State what is not being done, in plain language.
- Where a follow-up exists (another spec, another plan, a ticket), name it so a reader can find it.
- Pull out-of-scope items from three places:
  1. The spec's § Non-Goals section
  2. Anything the user said "not now" to during the architecture step
  3. Anything the chosen design deliberately leaves to a later plan
- If the plan truly has no exclusions worth naming, say so explicitly — an empty section is usually a sign something was missed.

### What to produce

A draft Out of Scope section ready to drop into plan.md at verification time.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Out of Scope** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/out_of_scope.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted exclusions are saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
