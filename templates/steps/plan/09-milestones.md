## Step {{step}}: {{title}}

Define 2-4 milestones that deliver the spec you have in context.

Each milestone must have:

- **Title**: A user-facing description of what changes (not an engineering label like "Refactor X").
- **What changes**: A one-paragraph summary describing the user-visible difference when the milestone lands. Written in plain language, no file paths, no shell commands. This paragraph is what a reader of plan.md uses to decide whether the milestone is worth doing — make it outcome-focused, not implementation-focused.
- **Validation point**: How to confirm the milestone is done before moving on.

Rules:

- Each milestone should be independently deliverable.
- Milestones should build on each other in order.
- NO open questions — resolve any uncertainties now; ask the user only if genuinely blocked (see below).
- Purely internal cleanups (e.g. refactors with no user-visible effect) are allowed, but the "What changes" paragraph must say so explicitly and explain why the cleanup is worth its own milestone.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Milestones** content — the milestone titles and their "What changes" paragraphs that open the plan.md `## Milestones & Tasks` section (body only, no `## ` heading line) — to `.spektacular/work/{{plan_name}}/milestones.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted milestones are saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
