# Knowledge-Worthy Discovery Recognition

> Managed by `spektacular init` and refreshed by `spektacular migrate` — edit
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

When you recognize the moment, write the entry yourself — do not offer it
and do not wait for a decision. Invoke the `spek-knowledge` skill, which
chooses the tier, store, category and tags with you and writes the entry
through `spektacular knowledge write`. Search first, and update an existing
entry rather than writing a near-duplicate of it. Phrase the entry as the
general lesson, not the specific edit that surfaced it.

Then, in your normal reply, tell the user what you wrote: which entry
(its path), where it lives (its tier and store), and why it is worth
keeping. That report is how the user sees the knowledge base change, so
never write an entry without saying so. A user who disagrees can ask for
the entry to be changed or deleted, and the project's version history
holds every earlier state.

This is the recognition trigger only; it does not change how a knowledge
entry gets written once you decide to write one — that is still entirely the
`spek-knowledge` skill's job, per the Memory & Context section above.
