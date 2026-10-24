# Spektacular's Files Are Reached Through Spektacular

> Managed by `spektacular init` and refreshed by `spektacular migrate` — edit
> `templates/agents/store-access.md` in the Spektacular source, not this file in
> place. Hand edits will not survive the next init. Agents read this file
> when its rule's short section in `AGENTS.md` sends them here, and never edit it.

Every file Spektacular manages is reached through its CLI, never with your own
file tools. This covers specs, plans, their context and research documents,
test plans, changelog records, knowledge entries, design documents and epics.

Never use the `Write` or `Edit` tool on a file under a store directory, and
never build a store path by hand. Use `spektacular spec file`,
`spektacular plan file`, `spektacular changelog file`, `spektacular knowledge`,
`spektacular design` and `spektacular epic` instead. A write supplies its body with
`--from <path>`, never on stdin and never as prose on the command line.

Removal is a CLI verb too. Deleting a managed file with `rm`, or with any
equivalent of your own, is never correct — not for a knowledge entry, not for
a design document, and not for anything else a store holds. Use
`spektacular knowledge delete`, `spektacular design delete` and
`spektacular epic delete`, alongside the
`delete` each of `spektacular spec file`, `spektacular plan file` and
`spektacular changelog file` already offers. Going around the tool is how a
spec is left pointing at a design that is not there, and it stops working
entirely the moment a store is backed by something other than a local
directory. If a removal is refused, the refusal names what to do instead:
act on it rather than reaching past it.

Do not use `ls`, `find`, or the `Read` tool to discover what a store holds,
against `.spektacular/specs/`, `.spektacular/plans/`, or any configured store
directory. The CLI's own `file list` is the source of truth for what counts as
a stored artifact: a directory listing can show entries Spektacular does not
consider valid, and omits the metadata the CLI reports alongside each one.

This includes an edit that looks too small to be worth a command, such as
ticking a task checkbox in a plan or appending a line to a changelog record.
Read the document with the CLI, apply the change, and write it back with the
CLI. A store write is not a file copy: it merges Spektacular's lifecycle
metadata, preserving a created date and carrying forward fields such as a
spec's recorded design references. An in-place edit destroys them silently, and
the loss surfaces much later.

Three paths are deliberately yours to write with your own tools, because they
are scratch and hand-off surfaces rather than stores:

- `.spektacular/tmp/` — content staged on the way to a CLI write, removed once
  it succeeds.
- `.spektacular/work/<name>/` — a workflow's per-section working files.
- `.spektacular/working-context.md` — the notes a resumed session reads back.

The guidance files under `.spektacular/guidance/`, this one included, are not a
store either. Read them with your own file tools whenever a rule's short section
in `AGENTS.md` points you there. Never edit, move or delete them: `spektacular init`
writes them and `spektacular migrate` rewrites them, so a change made by hand is
lost.

This rule binds everywhere: inside the spec, plan and implement workflows, and
equally in ad-hoc questions, unrelated skills and general exploration. It also
binds every sub-agent you launch. A sub-agent inherits this file but not the
skill that spawned it, so when you delegate work that touches a store, say so
in its prompt.
