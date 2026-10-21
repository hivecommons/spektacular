# Memory & Context

> Managed by `{{command}} init` and refreshed by `{{command}} migrate` — edit
> `templates/agents/memory-context.md` in the Spektacular source, not this file in
> place. Hand edits will not survive the next init. Agents read this file
> when its rule's short section in `AGENTS.md` sends them here, and never edit it.

In this repository, do not persist anything to your per-user, per-machine
memory store. When you would normally write to it — a learning, convention,
gotcha, project fact, user preference, or anything else worth remembering
between sessions — route the write through the `spek-knowledge` skill
instead. The skill handles tier and store selection, search-before-write, and
propose-then-confirm.

Outside this repository, continue using your per-user memory store as normal.
