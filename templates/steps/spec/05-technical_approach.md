## Step {{step}}: {{title}}

Draft any technical direction already decided, from the interview findings in `.spektacular/work/{{spec_name}}/interview.md` (and this section's own working file, if one already exists from a prior pass). Present the draft to the user and ask them to confirm it or tell you what's wrong.

Examples of the kind of direction to look for:
• Key architectural decisions already made
• Preferred patterns or technologies
• Integration points with existing systems
• Known risks or areas of uncertainty

Technical Approach is **non-binding direction** — preferences and suggestions the planning agent may adopt, adapt, or replace. It holds no hard rules. If the user states something as a hard "must" (e.g. "must use SQLite", "must not break the API"), that is a **constraint**, not technical direction — it belongs in the Constraints section, not here.

**Capture only the direction the user has already decided — do not design it for them, and do not investigate the codebase to "ground" it.** Reading source files, listing the repo, and reporting existing types, functions, or routes is plan-discovery work; it does not belong in a spec. If the user has decided nothing here, that is fine — note that no technical direction has been decided and let the plan workflow propose it. Keep whatever you capture at high-level-direction altitude (e.g. "use an embedded datastore", "integrate with the existing user store"), not implementation detail discovered from the code.

**Stay at direction altitude — do not write the design itself.** This is the section where mechanism is *welcome*, which makes it the easiest place to drift too low. Technical Approach is a *steer*, not a worked solution: name each decision or preference in a sentence or two, with its *why* if it helps, and stop. Designing the *how* is the downstream **plan workflow's** job — its steps own discovery, architecture, components, data structures, implementation detail, dependencies, testing approach, milestones, and phases. Apply this test before writing anything down: *would this content be re-derived by one of those plan steps?* If yes, it belongs to the plan, not here. Concrete tells that you have dropped too low: a numbered pipeline or algorithm, step-by-step processing, data shapes, field or function names, the ordering of operations, or anything that reads like "first do X, then Y, then Z". Compress each to a one-line steer (e.g. not the five steps of a de-duplication pipeline, but "prefer consolidating results behind a dedicated lookup step, because duplication can only be judged after the full content is read") and leave the worked design to the plan.

**When the design is settled, offer to capture it rather than compress it away.** The paragraph
above is where a worked design gets reduced to a one-line steer, which is exactly where it can
be lost. If this conversation has settled an API's shape, a user-facing flow, a data format, or
a worked example of any of these, that is a **design document**: the worked design the feature
is built to, kept in one of the project's declared design sources and referenced by this spec
rather than copied into it.

The bar is high, and all three parts must hold: the detail is **settled** (the user decided it,
not merely floated it), it is **worked** (a concrete shape, format or flow rather than a
direction), and it would **make the spec unreadable if written inline**. A hard boundary belongs
in Constraints; a preference the planner may adapt belongs here as a one-line steer; only a
worked design that would swamp the spec belongs in a document of its own. If a one-line steer
captures it, it is not a design document.

Offer, never write unprompted. Say what you would capture, which declared source you would
write it to (`{{config.command}} design sources` lists them), and why this spec is better off
pointing at it than containing it. Then wait for the user's decision:

- **Accept** — stage the document, write it with `{{config.command}} design write --data
  '{"source":"<name>","path":"<path>"}' --from <staged file>`, then record the reference with
  `{{config.command}} design ref add --data '{"spec":"{{spec_name}}","source":"<name>","path":"<path>"}'`.
  Both steps, every time: a design nothing references is invisible to the plan workflow, and a
  reference to a design that was never written is a broken reference. The design's content does
  **not** also go into this section — a one-line pointer to it is what belongs here.
- **Defer** ("not now", "once we've settled it") — write nothing, carry on, and you may raise
  the offer again later in this conversation if the detail keeps developing.
- **Decline** ("no", "keep it in the spec") — write nothing, and do not raise the offer again
  for this detail for the rest of the conversation. A decline is final for that detail, not a
  "not now". Declining does not mean the detail moves into the spec body instead: it stays out,
  or it stays as the one-line steer it already was.

Silence or deflection is not acceptance. A direct instruction from the user to write a design
document *is* the required agreement and needs no further confirmation.

**Do not restate content already captured in another section.** Anything that belongs in Constraints (e.g. "must use an embedded datastore", "must replace the existing file storage", "the database file location must be configurable") lives there, not here — do not copy it back into Technical Approach. Capture only *additional* technical direction that is not already a requirement or constraint. If there is none beyond what is already captured, say exactly that in one line — e.g. "No technical direction has been decided beyond the captured constraints; the detailed design is left for the plan workflow to propose." — without re-listing those constraints.

If the interview surfaced no technical direction beyond what's already captured elsewhere, draft the section as saying so plainly rather than leaving it blank with no explanation.

**Format each direction or steer as its own bullet point** (`- ...`), one per line, rather than a paragraph running multiple points together. Write the working file in this shape from the start — it is assembled into the final spec largely as-is.

Before advancing, save this section to its working file. Using your own `Write` tool, write the agreed **Technical Approach** content (the body only — no `## ` heading line) to `.spektacular/work/{{spec_name}}/technical_approach.md`. This working file is git-tracked and is read back on resume and when the spec is assembled, so it must hold the final agreed content for this section. It is **not** a spec store document — write it directly with your file tools and do **not** route it through `{{config.command}} spec file write` (that command is only for the final assembled spec).

Once you are satisfied, move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
