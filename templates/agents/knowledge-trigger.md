# Knowledge-Worthy Discovery Recognition

> Managed by `{{command}} init` and refreshed by `{{command}} migrate` — edit
> `templates/agents/knowledge-trigger.md` in the Spektacular source, not this file in
> place. Hand edits will not survive the next init. Agents read this file
> when its rule's short section in `AGENTS.md` sends them here, and never edit it.

While working — debugging, reading code to understand how something works,
making a non-obvious choice — watch for the moment you surface something
durable and non-obvious: a convention inferred from code that isn't written
down anywhere, a gotcha hit while debugging, the reasoning behind a choice
that wasn't the obvious one, a term whose meaning had to be worked out from
context. Recognizing this moment is your job, not the user's — don't wait to
be asked.

When you recognize the moment, offer — never write to the knowledge base
unprompted. Something like: "this looks like an undocumented convention —
want me to save it via `spek-knowledge`?" Say briefly what you'd capture and
why it's worth keeping, and name the tags you would propose for it, since
those decide whether the entry is ever found again. Wait for the user's
decision before doing anything else.

The user's response falls into one of three outcomes:

- **Accept** — invoke the `spek-knowledge` skill to write the entry. The
  skill's own propose-then-confirm flow handles tier and store selection and the
  actual write from there.
- **Defer** ("not now", "later", "remind me at the end") — do not invoke the
  skill. Continue the task normally, and treat this as temporary: if the
  work keeps surfacing related material, you may raise the offer again later
  in the same conversation.
- **Decline** ("no", "not worth saving") — do not invoke the skill, and do
  not raise the offer again for this discovery for the remainder of the
  conversation. A decline is final for that discovery, not a "not now."

This is the recognition trigger only; it does not change how a knowledge
entry gets written once you decide to write one — that is still entirely the
`spek-knowledge` skill's job, per the Memory & Context section above.
