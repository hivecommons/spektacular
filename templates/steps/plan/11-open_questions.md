## Step {{step}}: {{title}}

Draft the **Open Questions** section of `plan.md`.

### Strict scope

This section is **strictly for questions that genuinely cannot be resolved until implementation begins**. Anything that can be answered by reading the code, running a quick experiment, or — when genuinely blocking — asking the user now must be resolved now, not parked here.

Examples of what belongs here:

- "Whether the downstream API returns X or Y under condition Z — only discoverable by exercising it"
- "Whether the refactor exposes a hidden assumption in an untested code path — will surface during implementation"

Examples of what does NOT belong here:

- "Which library should we use?" → decide now on the best-grounded default and record it as an assumption (ask the user only if genuinely blocked)
- "What is the current shape of the API?" → read the code now
- "How should we name the new field?" → decide now
- "Is there a test already covering this?" → check now

### What to produce

A draft Open Questions section. If, after a genuine pass, there are no impl-time-only uncertainties, state that explicitly: an empty Open Questions section is the expected healthy outcome.

For every item you keep: state the question, state what it depends on, and state what the implementer should do when they hit it (usually: STOP and ask the user).

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Open Questions** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/open_questions.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted list is saved and correctly scoped, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
