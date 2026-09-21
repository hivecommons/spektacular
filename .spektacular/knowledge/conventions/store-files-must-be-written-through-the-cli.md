---
tags: [storage, paths, cli, artifacts]
---

# Spektacular's own files are written through Spektacular, never with file tools

Every file Spektacular manages is read and written through its CLI. Specs, plans, context,
research, test plans, changelog records, knowledge entries and design documents are all reached
with `spec file`, `plan file`, `changelog file`, `knowledge` and `design`. Never use `Write`,
`Edit`, `cat >`, `ls` or `find` against a store directory, and never construct a store path by
hand.

There are exactly three exceptions, and they are scratch or hand-off surfaces rather than stores:

- `.spektacular/tmp/` — content staged on the way to a CLI write, removed after it succeeds.
- `.spektacular/work/<name>/` — the per-section working files a workflow reads back on resume.
- `.spektacular/working-context.md` — the cross-cutting notes a resumed session picks up.

All three are written with the agent's own file tools, deliberately. Everything else is not.

Four reasons, each of which has already caused a real failure or is designed to prevent one:

- **The location is configuration, not layout.** Store directories come from `config.yaml`, and
  knowledge and design sources resolve from the folder holding that file and may sit outside the
  project root entirely. A hand-built path encodes a guess about a layout the project is free to
  change.
- **Writes are not copies.** `spec file write`, `plan file write` and `changelog file write` merge
  lifecycle frontmatter through `internal/metadata`, stamping the created date once, preserving it
  afterwards, and carrying forward fields such as a spec's `designs` references. A raw file write
  silently destroys them, and the loss shows up much later.
- **The provider is pluggable.** `file` is the only backend shipping today. Direct filesystem
  access hard-codes it.
- **The CLI decides what counts.** Listings honour `.spektacular_ignore` and the store's own
  validity rules, so `ls` can show entries Spektacular does not consider valid, and miss context
  it would have supplied.

Design documents have two write verbs, and which one you use is part of the rule. `design write`
stores the staged bytes exactly as given and adds no frontmatter — use it for a document the team
owns. `design author` stamps Spektacular's lifecycle metadata: capture date, document status,
originating spec, and the specs referencing it. Rewrite an authored document with `design author`,
which preserves its capture date and back-links; `design write` over an authored document is
refused rather than allowed to strip them.

This applies to reads as well as writes: use `spec file read`, `plan file read`,
`knowledge read` and `design read` rather than opening the file. Writing content on stdin or in a
heredoc is also out. `--from <path>` is the only supported way to supply a body, and
`internal/agent/instruction_surface_test.go` keeps a closed list of the retired stdin and heredoc
substrings out of every template, so an instruction using one is a test failure rather than merely
a stale style.
