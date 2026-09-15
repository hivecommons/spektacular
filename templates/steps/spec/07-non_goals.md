## Step {{step}}: {{title}}

Draft what is explicitly out of scope for this feature, from the interview findings in `.spektacular/work/{{spec_name}}/interview.md` plus what's already been confirmed in the spec so far — operations the requirements mention but do not require (e.g. delete when only create/update/retrieve are required), capabilities deferred to a follow-up, or alternatives the design explicitly excludes. Present the draft to the user and ask them to confirm it or tell you what's wrong.

Examples:
• 'Mobile support is out of scope (tracked in #456)'
• 'Internationalisation will be addressed in a follow-up spec'

Drafting is not deciding: a drafted non-goal still needs the user's **explicit confirmation** (and any edits) before it's saved. Never write a drafted non-goal as settled without the user confirming it — non-goals shape scope, and a wrong one silently narrows the feature.

**Do not duplicate the Constraints section.** A non-goal must not merely restate the inverse of a constraint already captured. If Constraints says *"must use SQLite"*, then *"other datastores are out of scope"* adds nothing — it is already implied by the constraint; leave it out. Likewise *"must replace the file storage"* already implies *"no dual-writing the legacy files."* Non-Goals is for scope exclusions the constraints do **not** already imply: capabilities deferred to a follow-up, operations the requirements mention but do not require (e.g. delete when only create/update/retrieve are required), or adjacent features a reader might reasonably expect that this feature will not address. Before saving, check each non-goal against the Constraints section and drop any that is just a constraint stated in reverse.

Capture the confirmed response. If the user confirms there are none, note that no non-goals have been defined.

**Format each non-goal as its own bullet point** (`- ...`), one exclusion per line, rather than a paragraph running multiple exclusions together. Write the working file in this shape from the start — it is assembled into the final spec largely as-is.

Before advancing, save this section to its working file. Using your own `Write` tool, write the agreed **Non-Goals** content (the body only — no `## ` heading line) to `.spektacular/work/{{spec_name}}/non_goals.md`. This working file is git-tracked and is read back on resume and when the spec is assembled, so it must hold the final agreed content for this section. It is **not** a spec store document — write it directly with your file tools and do **not** route it through `{{config.command}} spec file write` (that command is only for the final assembled spec).

Once you have captured the information from the user move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
