## Step {{step}}: {{title}}

**You are writing a spec, not a plan.** A spec captures *what* is being built and the direction the user has *already decided* — requirements, acceptance criteria, constraints, and any high-level technical direction. It does **not** design the *how*, and it does **not** investigate the codebase: do not read source files, run code searches, or ground decisions in specific files, types, or functions. That grounding is the plan workflow's `discovery` step. Stay at spec altitude in every step that follows.

**Where the spec ends and the plan begins.** Everything in a spec stays at the level of *what* and *why*. The *how* is designed later by the downstream **plan workflow**, whose steps own discovery, architecture, components, data structures, implementation detail, dependencies, testing approach, milestones, and phases. Whenever your content starts to resemble any of those — a worked design, a numbered pipeline or algorithm, step-by-step processing, data shapes, file/field/function names, the ordering of operations — it belongs to the plan, not the spec. The most a spec should do with a *how* is name it in a sentence as direction (that is what Technical Approach is for) and leave the design to the plan. This boundary holds for every step below.

Draft the Overview from the interview findings in `.spektacular/work/{{spec_name}}/interview.md` (and this section's own working file, if one already exists from a prior pass): 2-3 sentences covering what is being built, what problem it solves, and who benefits. Present the draft to the user and ask them to confirm it or tell you what's wrong.

**A pre-filled draft is confirmed, never asked from scratch.** If `.spektacular/work/{{spec_name}}/overview.md` already holds content (seeded from a source at the interview, or carried over from an earlier pass), that content is your draft. Present it to the user to confirm or refine, noting anything the interview added since, and do not ask for this section as if it were blank.

Be specific — avoid generic phrases like 'improve the experience'. If the interview findings don't give you enough to draft a specific, non-generic overview, ask the user directly rather than drafting something vague.

**Keep the overview stakeholder-readable.** No file paths, no section names, no step names, no framework or library names, no code identifiers. A non-engineer should be able to read it and understand the value. If you can't explain the feature without naming an implementation artifact, ask the user one more "so that…" question until you find the user-visible value.

If the user volunteers implementation detail ("we add a new FSM state", "append to research.md"), capture it mentally and tell them it will land in Technical Approach — then rephrase the overview at the behavior level.

Ask for clarification if the description is vague, incomplete, or leaks implementation before moving on.

Before advancing, save this section to its working file. Using your own `Write` tool, write the agreed **Overview** content (the body only — no `## ` heading line) to `.spektacular/work/{{spec_name}}/overview.md`. This working file is git-tracked and is read back on resume and when the spec is assembled, so it must hold the final agreed content for this section. It is **not** a spec store document — write it directly with your file tools and do **not** route it through `{{config.command}} spec file write` (that command is only for the final assembled spec).

Once you are satisfied with the overview, move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**A split asked for now.** If the user asks to split this spec into an epic before it is complete, do not split yet: a split always acts on a complete spec. Record the request in `.spektacular/working-context.md` (what they asked, and any specs they named) and carry on with this step. The request is acted on at the `split` step, once every section has been gathered.

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
