## Step {{step}}: {{title}}

**Run the interview before drafting anything.** This step follows the Flipped Interaction pattern (White et al., "A Prompt Pattern Catalog to Enhance Prompt Engineering with ChatGPT," arXiv:2302.11382): rather than asking the user to author each section from a blank prompt, you drive the conversation with a stated goal, adaptive questions toward it, and an explicit stopping condition.

**Stated goal:** understand this feature well enough to draft a credible first pass at every section of the spec — Overview, Requirements, Acceptance Criteria, Constraints, Technical Approach, Success Metrics, and Non-Goals.

**Ask adaptive, open questions — not a fixed script.** Start from what the user has already said (in this conversation, or in `.spektacular/working-context.md` if `new` captured something). Ask about what's being built, who it's for, what problem it solves, what constraints apply, and what's explicitly out of scope. Each question should follow from the answer before it, pursuing what's still unclear rather than working through a predetermined checklist. You are not trying to enumerate every possible requirement — you are building a picture sufficient to draft a first pass that the user can then confirm or correct section by section.

**Stop once more questions wouldn't change the draft.** End the interview when the user's answers have stopped introducing new information, or when you have enough to draft every section credibly — not when every conceivable detail has been individually asked about. This should take a small number of exchanges, not an exhaustive back-and-forth. If the user's initial description already answers most of these questions, a short interview (or none at all) is correct — do not manufacture questions for their own sake.

**Do not design the *how*.** Stay at spec altitude, same as every step that follows. If the user volunteers implementation detail, capture it mentally for Technical Approach and keep steering the conversation at the level of what and why.

**The repos this project spans.** Requirements described as focused on one repo can still need changes elsewhere in the project. Run `{{config.command}} repo list` now if you have not already: it reports every registered repository — its description, role, tags, and the `root` its code lives at — not just the one this conversation is currently focused on.


If the project has more than one registered repo and the feature as described reads as focused on one of them, ask at least one question about whether it also needs changes in another registered repo, before concluding the interview. Shape the question by what that other repo actually is, not generically — a repo whose role or tags suggest documentation invites asking whether docs need updating; one that looks like a CLI or API invites asking whether callers need corresponding changes. Do not ask this as a generic "does this affect other repos?" question when a repo's own description already makes the likely answer obvious — ask the specific question that description suggests.

Before advancing, write your synthesized understanding (not a transcript of the conversation) to `.spektacular/work/{{spec_name}}/interview.md` using your own `Write` tool. Capture what you now know about the feature, its users, its constraints, and anything explicitly ruled out, organized so the following section-drafting steps can draft from it directly. This working file is git-tracked and is read back on resume and by every section step. It is **not** a spec store document — do not route it through `{{config.command}} spec file write`.

Once you are satisfied the interview has reached its stopping condition, move to the next step by running the command:

{{config.command}} spec goto --data '{"step":"{{next_step}}"}'

**If the user rejects this draft.** If the user indicates this draft is wrong, ask a follow-up question to understand why before changing anything, the issue may reveal a broader need you didn't surface, or may be a genuine miss on your part, and the follow-up conversation determines which. Apply any resulting changes directly to the working file(s) they belong to, which may include a different section's working file than the one under review; a section amended this way does not need a fresh confirmation step now, the end-of-workflow verification step is where everything, including this change, gets reviewed together. The follow-up conversation may surface edits to more than one section, or conclude that nothing needs to change after all — do not assume the fix is exactly one edit to exactly the section under review.
