# Presenting Drafts and Confirmations

> Managed by `spektacular init` and refreshed by `spektacular migrate` — edit
> `templates/agents/draft-presentation.md` in the Spektacular source, not this file in
> place. Hand edits will not survive the next init. Agents read this file
> when its rule's short section in `AGENTS.md` sends them here, and never edit it.

When you have drafted substantial content for the user to review — an architecture write-up, a set of options, a written section, a summary, a plan or spec excerpt — always present that draft as normal, readable chat text first, in full. Never embed the draft itself inside a structured yes/no or multiple-choice dialog element: that kind of UI truncates or compresses long text, making it hard for the user to actually read what you're proposing.

Once the draft has been shown in full as plain text, you may then ask a short, direct confirmation question — e.g. "does this look right, or should anything change?" — using a structured yes/no or multiple-choice element if one is available. That confirmation step comes strictly after the content has already been presented as text, and is never a substitute for showing it.

This rule applies across every workflow step that follows a draft-then-confirm pattern — spec, plan, and implement workflows alike — not just one. If a specific step's own instructions don't repeat this explicitly, this standing rule still applies.
