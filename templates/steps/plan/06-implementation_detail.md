## Step {{step}}: {{title}}

Draft the **Implementation Detail** section of `plan.md`.

### This section is high-level only

Sketch new patterns being introduced, major code-shape changes, and code-structure UX — enough for a reviewer to spot missing patterns or design gaps. This section is **high-level only**. Per-phase file:line work stays in the plan's `context.md`.

If you find yourself writing "in file X at line Y", stop and move that content to the plan's `context.md`. The test for "too low-level" is: could this be written before the phases are defined? If no, it belongs in context.md.

### What to include

- New patterns the plan introduces (e.g. a new state machine, a new agent orchestration shape, a new module boundary)
- Major code-shape changes (e.g. splitting one package into three, or introducing a new interface that replaces a concrete type across the codebase)
- Code-structure UX: what a developer reading the changed code will experience
- Where existing patterns in the codebase are being followed vs. where new ones are being introduced

### What NOT to include

- Specific file paths or line numbers
- Per-phase file changes
- Function signatures that belong in § Data Structures & Interfaces
- Shell commands

### What to produce

A draft Implementation Detail section ready to drop into plan.md at verification time.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Implementation Detail** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/implementation_detail.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted sketch is saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
