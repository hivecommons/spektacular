## Step {{step}}: {{title}}

Draft the hard constraints and boundaries the solution must operate within, from the interview findings in `.spektacular/work/{{spec_name}}/interview.md` (and this section's own working file, if one already exists from a prior pass). Present the draft to the user and ask them to confirm it or tell you what's wrong.

Examples:
• Must integrate with the existing authentication system
• Cannot introduce breaking changes to the public API
• Must support the current minimum supported runtime versions

**Constraints are the hard rules and boundaries the solution MUST honor — and that the team does not get to negotiate.** Two things have to be true. A candidate that fails either one is not a constraint, and belongs in another section rather than being dropped.

Apply both tests, in order:

> **1. Is it binding?** *Is this a hard rule the solution is required to honor, or just a preferred direction the planner could adapt or replace?*

> **2. Whose decision was it?** *Did this come from outside the team's free choice — a user or stakeholder decision, a policy or regulation, an existing system, a platform reality — or did we simply pick it while drafting?*

Phrasing is the strongest signal for the first test: "must" / "must not" / "cannot" → binding; "prefer" / "consider" / "use … if" → Technical Approach. But **phrasing tells you nothing about the second test**, and that is where this section goes wrong. *"Must use SQLite"* reads identically whether the user mandated it or you concluded it was a sensible fit while writing the draft. Only one of those is a constraint, and you are the one choosing the wording — so the phrasing cannot be your evidence.

The practical form of the second test: **could the planner reasonably propose an alternative and be right?** If yes, it is direction, not a constraint. A constraint takes a decision away from the planner, so there must be a *reason* it was taken away. If you cannot state that reason, you are not looking at a constraint.

**Traceability rule — this is the one that keeps the section honest.** Every constraint you draft must trace back to something the user actually said, in the interview findings or in this conversation. If you are about to write a "must" that you inferred, assumed, or judged sensible rather than one the user imposed, do not write it as a constraint. Either ask the user whether it is binding and record their answer, or put it in Technical Approach as direction the planner may revise. An unasked question is not a mandate.

When a candidate fails, move it to the section that owns it:

• A **design decision you chose freely** — the planner could pick differently and be right → Technical Approach.
• A **statement of what you are *not* doing** — reads as "no changes to X", "we won't touch Y" → Non-Goals.
• Something the feature must ***do*** — describes behaviour rather than a bound on how it is built → Requirements.
• A **definition invented so an acceptance criterion becomes checkable** → leave Constraints alone and rephrase the criterion.

That last one is a trap worth naming. If an acceptance criterion cannot be verified because something is undefined, the fix is to rephrase the criterion so it refers to the thing generically. Pinning down a route, a filename, or a label purely so a criterion has something concrete to point at hands the planner a decision that was never taken away from them.

Examples:

• Constraint: *"must use SQLite"* — **when the user mandated it** (e.g. the whole point of a migration). It stays a constraint; do **not** move it to Technical Approach.
• Not a constraint: *"must use SQLite"* — **when you chose it** because it fit the requirements. The planner could reasonably propose otherwise; that is Technical Approach.
• Constraint: *"must integrate with the existing user store"* — a required integration boundary; the existing system is a fact, not a choice.
• Constraint: *"must not break the shape of the public JSON response"* — removing it lets the feature break downstream consumers.
• Constraint: *"must run embedded, with no separate database server"* — a deployment boundary.
• Constraint: *"must implement the payload shape settled in the `api` design source at `payments/v2.md`"* — a **referenced design document**. The design is the settled shape the plan builds on, so the pointer to it is binding; see the note below.
• Not a constraint: *"prefer a table-per-entity schema"* — a non-binding design preference; that's Technical Approach.
• Not a constraint: *"must be served at /reports/"* — a route is a design decision unless the user fixed it; that's Technical Approach.
• Not a constraint: *"no changes to the existing admin pages"* — a scope exclusion; that's Non-Goals.

(Hard rules about what the feature must *do* — its behaviour — are Requirements, captured in the previous step. Constraints are the hard rules about *how / within what bounds* it is built.)

**A design document this spec references is a constraint, and its pointer lives here.** A design is only captured once the user has settled it, so it is never the team's free choice, and the plan workflow treats a referenced design as the settled shape it builds *on* — weighing options *within* that shape and raising a disagreement with the user rather than designing around it. That is exactly a decision taken away from the planner, which is what this section is for. Record it as a bullet naming the design's source and path and what it settles, and nothing more: the document holds the design, and duplicating its content here is what the document exists to avoid. The offer to capture a design is made in the next step, Technical Approach, so a design settled later in the conversation arrives as an amendment to this section's working file rather than through this step — that is expected, not a mistake.

Before drafting an empty section, check the most common sources of real boundaries against the interview findings so an empty section is a deliberate choice and not an oversight — if the interview doesn't already rule these out, ask the user about each that the work might touch:

• An **existing system** the work must integrate with or replace (e.g. an existing datastore, file format, or API whose data or shape must be preserved).
• A **deployment or runtime** boundary (e.g. must run as a single self-contained binary, no separate server process, a fixed runtime or platform).
• A **compatibility** guarantee (e.g. must not break a public response shape or an existing deployment contract).
• A **design document the project already holds** that this feature must be built to. Run `{{config.command}} design list` to see what the declared sources hold; if one of them settles the shape of this work, reference it here and record it with `{{config.command}} design ref add --data '{"spec":"{{spec_name}}","source":"<name>","path":"<path>"}'` so the plan workflow can find it.

If, after checking these, there are genuinely none, draft the section as empty and say so plainly when presenting it — a blank section is a valid outcome, but the user should still confirm it was a deliberate check, not a skip.

**Format each constraint as its own bullet point** (`- ...`), one hard rule per line, rather than a paragraph running multiple rules together. Write the working file in this shape from the start — it is assembled into the final spec largely as-is.

**Before you save, re-read every bullet and confirm all four:**

- [ ] **Binding** — a rule the solution must honor, not a preference.
- [ ] **Not our free choice** — it came from a user or stakeholder decision, a policy, an existing system, or a platform reality.
- [ ] **Traceable** — you can point to where the user imposed it. If you cannot, ask them before saving it as a constraint.
- [ ] **Owned by this section** — not a scope exclusion, a behaviour, or a patch for an unverifiable acceptance criterion.

Move any bullet that fails a box to the section that owns it — do not simply delete it.

Before advancing, save this section to its working file. Using your own `Write` tool, write the agreed **Constraints** content (the body only — no `## ` heading line) to `.spektacular/work/{{spec_name}}/constraints.md`. This working file is git-tracked and is read back on resume and when the spec is assembled, so it must hold the final agreed content for this section. It is **not** a spec store document — write it directly with your file tools and do **not** route it through `{{config.command}} spec file write` (that command is only for the final assembled spec).

Once you are satisfied, move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
