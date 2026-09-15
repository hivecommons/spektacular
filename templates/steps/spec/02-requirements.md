## Step {{step}}: {{title}}

Draft the specific, testable behaviours this feature must deliver from the interview findings in `.spektacular/work/{{spec_name}}/interview.md` (and this section's own working file, if one already exists from a prior pass). Present the draft to the user and ask them to confirm it or tell you what's wrong.

Use active voice:
• 'Users can...'
• 'The system must...'

Each item should be independently verifiable. One behaviour per line. If the interview findings don't give you enough to draft complete, testable requirements, ask the user directly rather than guessing.

**Format each requirement as a checklist item:** a short, bold title on the checkbox line, with the full testable detail indented on the line below —

```
- [ ] **Short title**
  Full requirement detail, in active voice, independently verifiable.
```

Write the working file in this shape from the start. It is assembled into the final spec largely as-is, so getting the shape right now avoids a reformatting pass later.

**Requirements describe WHAT, not HOW.** Apply this test to every requirement before you write it down:

> *If this were implemented with a completely different mechanism, would the requirement still be true?*

If the answer is no, the requirement is prescribing implementation — rephrase it at the behavior level and park the mechanism elsewhere: in **Constraints** if it is a hard rule the solution must honor (e.g. "must use X"), otherwise in **Technical Approach** as non-binding direction.

Example: the user says *"candidates are appended to `research.md` under a `## Knowledge Candidates` section."* That names a specific file and section — a different implementation (a sidecar file, an in-memory queue) would falsify the requirement. Capture it as *"the workflow surfaces candidate knowledge for review before anything becomes persistent"* and tell the user the `research.md` detail will land in Technical Approach.

Warning signs that a requirement is leaking HOW: specific file paths, section or heading names, step or state names, framework/library names, code identifiers, numeric step positions ("after step 13"). If you see any, rephrase.

Capture the requirements. Ask for clarification on any that are vague, ambiguous, not independently verifiable, or that leak implementation before moving on.

Before advancing, save this section to its working file. Using your own `Write` tool, write the agreed **Requirements** content (the body only — no `## ` heading line) to `.spektacular/work/{{spec_name}}/requirements.md`. This working file is git-tracked and is read back on resume and when the spec is assembled, so it must hold the final agreed content for this section. It is **not** a spec store document — write it directly with your file tools and do **not** route it through `{{config.command}} spec file write` (that command is only for the final assembled spec).

Once you are satisfied with the requirements, move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
