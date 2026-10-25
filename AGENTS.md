

# Agent Rules <!-- tessl-managed -->

@.tessl/RULES.md follow the [instructions](.tessl/RULES.md)

## Where the Code Lives — Read This First

**STOP. The directory you are running in is not necessarily the code.** A project holding only
`.spektacular/`, `knowledge/` and `repos/` is normal: its code lives wherever the registry says.

- **Before any code-touching work** (searching, reading, editing, running tests, verifying, or
  answering how something works), run `spektacular repo list` and work in each repo's `root`.
- **Never assume the starting directory holds a repo's code, and never guess a path.** The
  registry is the only authority on where code lives.
- **Pass the relevant repo's `root` explicitly to every sub-agent you launch.**
- **If a repo's `root` is not on disk, stop and tell the user** the path the registry gave.

This binds everywhere: every workflow, ad-hoc question, skill and sub-agent.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/repo-sources.md](.spektacular/guidance/repo-sources.md).

## Memory & Context

In this repository, never write to your per-user memory store; route anything worth remembering
through the `spek-knowledge` skill instead. Outside this repository, use your memory as normal.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/memory-context.md](.spektacular/guidance/memory-context.md).

## Spec-Worthy Discussion Recognition

When a discussion or a request (an issue, a ticket, a pasted doc) reaches spec-worthy detail,
offer to capture it as a spec before implementing; never start the spec workflow unilaterally.
Calibrate with `spec_trigger_threshold` in `.spektacular/config.yaml`, read at decision time.
Before offering, read the full guidance.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/spec-trigger.md](.spektacular/guidance/spec-trigger.md).

## Historical Artifacts: Specs and Plans as Archaeology

Specs and plans under `.spektacular/specs/` and `.spektacular/plans/` record past intent, not
current behaviour: never read them to explain how the code works today, only for "why was X
built this way?" questions or within the workflow that owns them. Knowledge entries are binding.
Before reading a spec or plan, read the full guidance.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/historical-artifacts.md](.spektacular/guidance/historical-artifacts.md).

## Presenting Drafts and Confirmations

Always show a draft in full as plain chat text first; never put the draft itself inside a yes/no
or multiple-choice dialog. A short confirmation question may follow it.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/draft-presentation.md](.spektacular/guidance/draft-presentation.md).

## Knowledge-Worthy Discovery Recognition

When you surface something durable and non-obvious (an unwritten convention, a gotcha, the reason
behind a choice), write it via `spek-knowledge` yourself, then tell the user which entry, where
(tier, store, path) and why. Do not ask first. Before writing, read the full guidance.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/knowledge-trigger.md](.spektacular/guidance/knowledge-trigger.md).

## Spektacular's Files Are Reached Through Spektacular

Never read, list, write, edit or delete a store file (specs, plans, changelog, knowledge, designs,
epics) with your own tools, and never build a store path by hand: use the `spektacular` CLI, writing
with `--from <path>`. Yours to write: `.spektacular/tmp/`, `.spektacular/work/<name>/`,
`.spektacular/working-context.md`; yours to read, never edit: `.spektacular/guidance/`. This binds
every sub-agent: say so when delegating. Before touching `.spektacular/`, read the full guidance.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/store-access.md](.spektacular/guidance/store-access.md).

## Design-Worthy Detail Recognition

When a discussion settles a worked design (an API shape, a flow, a data format) too large to
write into a spec, or the user brings or changes a design document, offer to capture it with the
`spek-design` skill; never write a design document unprompted. Before offering, read the full
guidance.
Managed by `spektacular init`; full guidance: [.spektacular/guidance/design-trigger.md](.spektacular/guidance/design-trigger.md).
