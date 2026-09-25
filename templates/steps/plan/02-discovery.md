## Step {{step}}: {{title}}

Research the codebase to understand what's needed to implement the spec you read in the previous step. The output of this step is `research.md` — a **decision log**, not a transcript — that captures your investigation so a future cold session can rehydrate without re-doing the work.

### Step 1: Identify the target repos

**The repos this project spans.** Every requirement is ultimately carried out in one of the project's registered repos. Run `{{config.command}} repo list` now if you have not already: it reports each repo with its description, role, tags, and the `root` its code lives at. Never assume the directory you started in is a repo's code.


From the spec and that roster's description, role, and tags alone — before reading any code — identify which registered repo(s) this plan's changes will land in. This is a quick scoping judgement call, not the codebase research itself (that's Step 2). If a repo's involvement is genuinely unclear at this point, include it rather than guessing it out; Step 2 will confirm or correct the list as research proceeds.

### Step 2: Project Context

Load the project's **always-applied** knowledge in full, narrowed to the repo(s) identified in Step 1: `{{config.command}} knowledge always-applied --tier repo --filter <name>` (repeat `--filter` for each target repo). This returns every always-applied entry's body — the **conventions** and the **glossary** — each tagged with the tier and store name it came from and its category. The narrowing is honoured with no exceptions: a store you do not name contributes nothing, so this keeps the load proportional to the repos actually in play rather than pulling every registered repo's conventions into every plan. If this plan also needs the project's shared knowledge, ask for `--tier all` and name the shared stores you want alongside the repos; `{{config.command}} knowledge sources` lists what is available. You only need to load this once: it then stays in your context for the rest of the plan. Read all of it — the conventions are the standards every plan must reflect (the next step selects the relevant subset into the plan), and the glossary is the shared vocabulary you need to read the rest of the knowledge base and the code correctly. If Step 3's research turns up an additional repo this plan touches, re-run this command with the expanded `--filter` list.

**Resolve and read every design document this spec references, before you design anything.**
Run `{{config.command}} design ref list --data '{"spec":"<spec name>"}'`. For every reference it
reports as resolved, read the document in full with `{{config.command}} design read --data
'{"source":"<source>","path":"<path>"}'`. A referenced design is the **settled shape** the spec
is bound to: it is binding input to this plan in the same way a knowledge entry is, not
background reading. Architecture is built on it, not re-derived from scratch around it.

**If `unresolved` is greater than zero, STOP and report to the user.** The response names every
reference that could not be found and the exact location it searched. Do not carry on as though
the spec carried no design, and do not quietly plan around the gap: a broken reference caught
here is the whole reason the report exists, and one discovered during implementation instead is
a failure of this step. Present the unresolved references, say what was searched for each, and
let the user decide whether to write the missing design, correct the reference, or drop it.

If the spec carries no design references at all, say so explicitly when you reach the
Dependencies step rather than leaving it unstated.

Then pull in topic-specific knowledge for the **design surfaces** this feature introduces — the concrete areas the work touches (for example `database`, `auth`, `http-middleware`, `cli`). For each surface, run a targeted `{{config.command}} knowledge search <surface>` rather than one broad "anything about this area" sweep; this keeps research focused as the knowledge base grows. Results come back ranked — one per matching document, strongest match first, each tagged with the tier and store name it came from and its category, and carrying a title, score, excerpts, and the entry's own `tags`; the category label tells you what kind of knowledge a hit is (e.g. a `gotchas` warning, an `architecture` fact, a `learnings` finding). **A document does not have to contain every word of your query** — it is returned if it carries evidence for any of them, ranked on how much of the query it covers and how strongly, with an entry's tags weighing far more than the same word appearing in its prose. So a hit is not proof that every query word appeared in it, and an empty result is not proof that nothing on the subject exists: try a differently-worded query, or narrow with a repeatable `--tag <tag>`, before concluding the knowledge base is silent on a surface. A hit carries everything a read needs, so read a promising one in full with `{{config.command}} knowledge read --data '{"tier":"<tier>","name":"<name>","path":"<path>"}'`, taking all three values straight from the hit. Narrow a search the same way as the load above, with `--tier` and `--filter`. The always-applied categories (conventions and glossary) are deliberately excluded from search results (you already loaded them in full above), so search surfaces only reference looked-up knowledge — architecture notes, gotchas, prior learnings. If something relevant exists, read it before investigating; it may already answer your questions or flag dead ends. Nothing is required to exist — the knowledge sources can be empty.

**Knowledge outranks the code it describes.** Every entry you load or retrieve above — conventions, glossary, architecture, gotchas, learnings, decisions — states how this project works and how the work must be done. The code is the current state, and it may not have caught up yet. Where an entry and the code disagree, **plan to the entry** and treat the difference as work to do; a codebase that does something else is not evidence that an entry is stale, obsolete, or superseded. This applies with full force when an entry mandates a library, a structure, or a pattern the code does not use yet — that is the entry doing its job. If you genuinely believe an entry is wrong, raise it with the user and ask; never silently overrule it in the plan.

If the plan touches tests, read the relevant test files directly as part of Step 3 (Codebase Research) to understand conventions (framework, naming, fixtures, mocking) before planning changes. Don't cache findings — the test files are the source of truth.

### Step 3: Codebase Research

Research across every repo identified in Step 1, in the `root` `repo list` reports for it — and if this research reveals the plan also touches a repo not identified in Step 1, add it and re-run the always-applied load above before continuing.

A repo's `root` is where its code lives; for a repo whose code is not on disk yet, `{{config.command}} repo list` reports its registered location and staleness warnings, and `{{config.command}} repo add` clones its source. Use each repo's description, role, and tags to scope which repos each research question belongs to, and search within every relevant repo. Note that `.spektacular_ignore` exclusions keep noise (build artifacts, dependency directories) out of Spektacular's own listing and search results; your native file tools are not bound by them.

Research the codebase in parallel to find:

1. **Files related to the plan** — Organize by category (implementation, tests, config, docs)
2. **Prior research** — Prior plans and specs are historical archaeology explaining *why* past decisions were made, not descriptions of what the codebase does today; consult them here to inform this new plan, not to derive current-state behaviour. Search the knowledge sources with `{{config.command}} knowledge search <query>`, list prior plans with `{{config.command}} plan file list` and prior specs with `{{config.command}} spec file list`, then read any that look relevant with `{{config.command}} plan file read <name> plan` or `{{config.command}} spec file read <name>`. Always reach plans and specs through these CLI commands — never with the `Read` tool
3. **Similar implementations** — Code examples to model after, with file:line references
4. **Architecture and integration points** — How the relevant components fit together
5. **Alternatives to consider** — Identify 2-3 viable approaches so the next step can compare them with evidence

Use your agent orchestration capability to parallelize this research. For guidance: `{{config.command}} skill spawn-planning-agents`

### Step 4: Distill findings into research.md — the decision log

You are gathering the content for research.md's required sections — you will save it to a working file at the end of this step, and the verification step assembles it into the store document. The required sections are:

- **Alternatives considered and rejected** — options you considered; for each, what it is, why rejected, with citation (file:line or external reference). This prevents future agents from re-proposing the same dead ends.
- **Chosen approach — evidence** — the file:line or external references that support the option you'll recommend in the next step. Evidence, not the decision itself.
- **Files examined** — terse one-liner per file: `path:line — what was learned`. In a multi-repo project, prefix entries from registered repos with the repo name: `<repo>:path:line — what was learned`.
- **External references** — papers, RFCs, library docs, blog posts, with a one-line "why this mattered".
- **Prior plans / specs consulted** — links with what was learned from each.
- **Open assumptions** — things assumed but not verified. If any turn out wrong, the implement workflow must STOP and ask.
- **Rehydration cues** — skill invocations and file re-reads that can rebuild context from cold.

Keep this dense. Assume a future agent will read it cold and need to make decisions from it.

### Step 5: Read and Clarify

- Read all findings fully
- Ask only questions the code cannot answer
- Report a brief summary of key discoveries to the user in passing — informational, not a review gate; do not wait for acknowledgement

### Step 6: Capturing a learning (optional)

If your research surfaces a durable learning, gotcha, or convention worth keeping for future plans, you may persist it with `{{config.command}} knowledge write`. Before writing, run `{{config.command}} knowledge sources` to see the configured stores by tier and name, then **propose to the user a target tier and store name, the path, and the exact content you intend to write, and wait for explicit confirmation**. Do not invoke `{{config.command}} knowledge write` until the user has confirmed. Propose, then wait for confirmation — never write to a knowledge source unprompted.

Before advancing, save your research to its working file. Using your own `Write` tool, write the distilled research content — all seven sections above (alternatives, chosen-approach evidence, files examined, external references, prior plans/specs, open assumptions, rehydration cues), body content only — to `.spektacular/work/{{plan_name}}/research.md`. This working file is git-tracked and is read back on resume and when the plan documents are assembled, so it must hold the final content. It is **not** a plan store document — write it directly with your file tools and do **not** route it through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once research is complete, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
