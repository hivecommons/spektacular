## Step {{step}}: {{title}}

Review the requirements captured in the previous step (`.spektacular/work/{{spec_name}}/requirements.md`). For each one, draft a pass/fail condition that proves it is done, drawing on the interview findings in `.spektacular/work/{{spec_name}}/interview.md` for anything the requirement itself doesn't make unambiguous. Present the draft to the user and ask them to confirm it or tell you what's wrong.

A good criterion:
• Describes an observable outcome
• Passes or fails — no subjective judgment
• Is traceable to this requirement

Example: "When X happens, Y is visible / saved / returned." If you can't draft an unambiguous pass/fail condition for a requirement from what you already know, ask the user directly rather than guessing.

**Format each criterion as a checklist item:** a short, bold title on the checkbox line, with the observable pass/fail detail indented on the line below, matching Requirements' shape —

```
- [ ] **Short title**
  Observable, pass/fail detail, traceable to a requirement above.
```

Write the working file in this shape from the start. It is assembled into the final spec largely as-is, so getting the shape right now avoids a reformatting pass later.

**Stay at the blackbox level.** A criterion should be something a tester who has never read the source could verify by observing inputs and outputs — files on disk, API responses, UI state, log lines. Avoid criteria that inspect internal plumbing:

• Good: *"After the user accepts a candidate, a file exists under the knowledge directory containing the candidate's title and content."*
• Bad: *"Running the FSM goto from source state X is accepted and from source state Y is rejected."* — that's a unit test of an internal state machine, not an acceptance criterion.

If the user gives you a criterion that names internal states, private functions, or specific code paths, rephrase it as an observable outcome and tell them the internal check belongs in tests, not the spec.

Capture all criteria. Ask for clarification on any that are subjective, not traceable to a requirement, or that test internal plumbing before moving on.

Before advancing, save this section to its working file. Using your own `Write` tool, write the agreed **Acceptance Criteria** content (the body only — no `## ` heading line) to `.spektacular/work/{{spec_name}}/acceptance_criteria.md`. This working file is git-tracked and is read back on resume and when the spec is assembled, so it must hold the final agreed content for this section. It is **not** a spec store document — write it directly with your file tools and do **not** route it through `{{config.command}} spec file write` (that command is only for the final assembled spec).

Once you are satisfied with the acceptance criteria, move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
