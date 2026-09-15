## Step {{step}}: {{title}}

Decide the shape of the solution and lock in the chosen direction. This step produces the **Architecture & Design Decisions** section of `plan.md` — the load-bearing section of the whole plan. A reviewer should be able to spot missing patterns or design gaps from this section alone.

**The repos this project spans.** The architecture must place every part of the work in the repo it belongs to. `{{config.command}} repo list` reports each registered repo with its metadata and the `root` its code lives at.


For every requirement, the drafted architecture must name **which repo (and which files within it)** the requirement is carried out against, using the repos' metadata from that section and the discovery findings; look inside a repo at the source listed there (or run `{{config.command}} repo list` when a source is not on disk yet). Record the requirement-to-repo-and-files resolution in the plan's context document as part of the per-phase technical detail — no separate document.

### Step 1: Weigh Options

Enumerate 2-3 viable design options. For each:

- **Option name** and brief description
- **Pros**: Advantages, with file:line references from research.md where they apply
- **Cons**: Disadvantages, risks, complexity
- **Effort estimate**: Relative complexity (Low / Medium / High)

Ground each option in the research findings gathered during the discovery step.

### Step 2: Choose and Record

Pick the best-grounded direction yourself — do not put the choice to the user — and record it as the leading entry in the assumption log (see below), covering:

1. **Chosen direction** — which option to pursue
2. **Key design decisions** — the non-obvious trade-offs the chosen direction makes

Rejected options go into `research.md § Alternatives considered and rejected` with citations when the verification step writes the files.

### Step 3: Draft the Architecture & Design Decisions section

Draft 2-4 short paragraphs describing:

- The shape of the chosen solution
- The key design decisions and their trade-offs
- Why this direction beats the alternatives
- A cross-reference to `research.md#alternatives-considered-and-rejected` so plan.md readers can drill into the evidence

Keep this section self-contained. Do NOT write `see context.md for …` (pointing at the plan's `context.md`) — plan.md must stand on its own for readers outside the Milestones & Phases block.

Before advancing, save this section to its working file. Using your own `Write` tool, write the drafted **Architecture & Design Decisions** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/architecture.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

### Step 4: Select the conventions that apply

Now the design shape is locked and you know the surfaces this feature touches, select — from the conventions you loaded in full during discovery (the conventions-category entries returned by `{{config.command}} knowledge always-applied --tier repo --filter <name>` during discovery) — the subset that actually bears on this work. For each one you keep, write a one-line rationale for **why it applies to this feature**, and cite it inline in the Architecture & Design Decisions content above wherever it drives a specific choice. Include only the genuinely relevant conventions — not the whole knowledge base.

Relevance is **decided and recorded, not asked**: choose the conventions to apply (and any you deliberately drop) and record the choice as an assumption-log entry instead of asking for confirmation. If no conventions are relevant, or the project has none, say so plainly rather than padding the list — an empty or generic list is a visible signal the knowledge base was not consulted.

Save the result to its working file. Using your own `Write` tool, write the **Conventions** content (body only — no `## ` heading line) to `.spektacular/work/{{plan_name}}/conventions.md`. Use a bullet list, one convention per line in the form `- **<convention name / one-line summary>** — <why it applies to this feature>.`. When nothing applies, write a single explicit sentence instead, e.g. `No project conventions apply to this feature.`. This working file is always required — write it even in the "none apply" case — and like the others it is git-tracked, read back on resume and when the plan documents are assembled; do **not** route it through `{{config.command}} plan file write`.

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the chosen direction is recorded in the assumption log and the draft is saved, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
