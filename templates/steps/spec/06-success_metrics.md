## Step {{step}}: {{title}}

Draft how success will be measured after delivery, from the interview findings in `.spektacular/work/{{spec_name}}/interview.md` (and this section's own working file, if one already exists from a prior pass). Present the draft to the user and ask them to confirm it or tell you what's wrong.

Be specific:
• Quantitative: 'p99 latency < 200ms', 'error rate < 0.1%'
• Behavioral: 'users complete the flow without support intervention'

If the interview didn't surface anything to measure, draft the section as saying no success metrics have been defined rather than leaving it blank with no explanation, and ask the user to confirm that's deliberate.

**Format each metric as its own bullet point** (`- ...`), one per line, rather than a paragraph running multiple metrics together. Write the working file in this shape from the start — it is assembled into the final spec largely as-is.

Before advancing, save this section to its working file. Using your own `Write` tool, write the agreed **Success Metrics** content (the body only — no `## ` heading line) to `.spektacular/work/{{spec_name}}/success_metrics.md`. This working file is git-tracked and is read back on resume and when the spec is assembled, so it must hold the final agreed content for this section. It is **not** a spec store document — write it directly with your file tools and do **not** route it through `{{config.command}} spec file write` (that command is only for the final assembled spec).

Once you are satisfied, move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
