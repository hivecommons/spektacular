

# Agent Rules <!-- tessl-managed -->

@.tessl/RULES.md follow the [instructions](.tessl/RULES.md)

## Where the Code Lives — Read This First

> Managed by `go run . init` — edit `templates/agents/repo-sources.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

**STOP. The directory you are running in is not necessarily the code.**

This is a Spektacular project. The code it describes may live anywhere on
disk — a sibling directory, somewhere else entirely on the filesystem, or a
clone this project manages. A project directory holding only
`.spektacular/`, `knowledge/`, and `repos/` is normal, and is **not** an
empty or broken repository: the code is elsewhere, and the project knows
where.

**Before any code-touching work — searching, reading, grepping, editing,
running tests, verifying, or answering a question about how something
works — run:**

```
go run . repo list
```

Every registered repo comes back with a `root`: the absolute path where
that repo's code actually lives. Work in that `root`. Cite files by paths
under it. Run tests from it.

These rules are not negotiable:

- **Never assume the directory you started in holds a repo's code.** Check
  with `go run . repo list` first, in every session, before you open the
  first file.
- **Never guess a path** from the project's name, a repo's name, or a
  directory that looks plausible. The registry is the only authority on
  where code lives.
- **Pass the relevant repo's `root` explicitly to every sub-agent you
  launch.** A sub-agent inherits your working directory, not your knowledge
  of where the code is.
- **If a repo's `root` is not present on disk, stop and tell the user.**
  Report the path the registry gave and that it is missing. Do not
  substitute a directory that looks close, and do not silently carry on in
  the project directory instead.

This rule outranks your default assumptions about working directories, and
it binds everywhere in this project — inside spec, plan and implement
workflows, and equally in ad-hoc questions, unrelated skills, and general
exploration. It is not limited to workflow steps.

## Memory & Context

> Managed by `go run . init` — edit `templates/agents/memory-context.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

In this repository, do not persist anything to your per-user, per-machine
memory store. When you would normally write to it — a learning, convention,
gotcha, project fact, user preference, or anything else worth remembering
between sessions — route the write through the `spek-knowledge` skill
instead. The skill handles tier and store selection, search-before-write, and
propose-then-confirm.

Outside this repository, continue using your per-user memory store as normal.

## Spec-Worthy Discussion Recognition

> Managed by `go run . init` — edit `templates/agents/spec-trigger.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

During an open-ended discussion (diagnostics, brainstorming, exploratory back-and-forth), watch for the moment it produces something substantial enough to be worth capturing as a specification — multiple requirements mentioned, a scoped decision reached, or a feature described in enough detail that it could be built from. Don't wait to be asked; recognizing this moment and offering is your job, not the user's.

This check is not limited to live back-and-forth discussion. It applies just as much the moment you finish reading a fully-formed request — a GitHub issue, a linked ticket, a pasted design doc — that already describes a feature in spec-worthy detail. Reading a well-specified issue and going straight to implementation is the same miss as skipping the offer mid-conversation: evaluate the request against the criteria above *before* starting any implementation work, not only when the detail accumulates turn-by-turn in front of you.

Before deciding whether a discussion has crossed that line, read `spec_trigger_threshold` from `.spektacular/config.yaml` — check it at the moment you are deciding, not once at the start of the session, since the user may change it mid-conversation and expects that to take effect immediately. Treat a missing or absent value as `"moderate"`. Use the configured value to calibrate how readily you offer:

- `"strict"` — only offer for substantial, multi-requirement features. Small fixes and minor tweaks should not trigger an offer.
- `"moderate"` — offer once a discussion reaches a clear, scoped decision or a feature description with more than one requirement. The default balance.
- `"lenient"` — offer readily, including for small bug fixes or narrowly scoped changes, whenever there's a decision worth recording.

When you recognize the moment, always offer — never start the spec workflow unilaterally. Propose capturing the discussion as a spec, briefly say why (e.g. "this sounds like it's grown into a few concrete requirements — want me to capture it as a spec?"), and wait for the user's decision before doing anything else.

The user's response falls into one of three outcomes:

- **Accept** — proceed to carry-forward below.
- **Defer** ("not yet", "still investigating", "let me think") — do not start the spec workflow. Continue the conversation normally, and treat this as temporary: if the discussion keeps developing, you may raise the offer again later in the same conversation.
- **Decline** ("no", "I don't want a spec for this") — do not start the spec workflow, and do not raise the offer again for this discussion topic for the remainder of the conversation. A decline is final for that topic, not a "not now."

If the user accepts, start the spec workflow:

```
go run . spec new --data '{"name":"..."}'
```

Then drive its existing steps — the "ask the user..." prompts in `templates/steps/spec/*.md` are unchanged and still apply — but answer each one from the conversation you already had instead of asking cold. For every step where the discussion already established an answer (e.g. the overview step's "describe this feature in 2-3 sentences"), propose a draft based on what was said and ask the user to confirm or refine it, rather than posing the question as if from scratch. Only ask a step's question directly when the conversation genuinely didn't cover it. The user's confirmation or correction is still the final word — never silently record your own draft as accepted without it.

This behavior is scoped to the current, single conversation: it does not persist across sessions, and it does not apply outside a Spektacular-initialized repository.

## Historical Artifacts: Specs and Plans as Archaeology

> Managed by `go run . init` — edit `templates/agents/historical-artifacts.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

In this repository, treat every file under `.spektacular/specs/` and
`.spektacular/plans/` as a historical, archaeological record. Each one
describes the intent behind a past change — *why* something was proposed,
what was in scope at that moment, and how the author framed the problem.
None of them are a description of what the codebase does today. Intent
recorded in a spec or plan may have been reshaped during implementation,
descoped, or abandoned entirely; only the shipped code, its tests, and its
configuration authoritatively describe current behavior.

Because of that, when you are exploring the codebase, summarizing a
feature, tracing how something works, or answering any question about
current-state behavior, do not read files under `.spektacular/specs/` or
`.spektacular/plans/` — through the `Read` tool, through `go run . spec
file read`, through `go run . plan file read`, or through any other
channel. Ground your answer in source files, tests, and configuration
instead, and cite paths under those directories rather than under the
spec or plan stores.

You may read a historical spec or plan only when the user is genuinely
investigating past intent — questions like "why was X built this way?",
"what was the original plan for Y?", or "which spec introduced Z?". In
that case, read the relevant document, and cite it explicitly as
historical context for a past decision rather than as a description of
current behavior. Archaeology is the only allowed reason to open these
files outside an active workflow.

There is one further exception: while a spec, plan, or implement
workflow is actively running, the workflow that owns its artifact may
read and update that artifact freely. That is what the workflow is for,
and it uses the dedicated CLI (`go run . spec file read/write`,
`go run . plan file read/write`) to do so. Once the workflow closes —
or for any agent that is not the workflow currently driving the
artifact — the artifact is historical again and subject to the same
rules as every other spec or plan on disk.

This rule applies everywhere you operate in the repository, not only
inside spec, plan, or implement workflow steps. It binds ad-hoc
questions, unrelated skills, and general exploration alike. Users
should not have to restate it in each session.

The knowledge base is the exception to all of the above, and the
distinction matters. Entries under a repo's or the project's knowledge
sources are **not** a record of past intent. They are current, curated
statements of how this project works and how work in it must be done,
and they are **binding**. Where a knowledge entry and the current code
disagree, the entry states the target and the code is what has yet to
meet it. Never conclude that an entry is stale, obsolete, or superseded
merely because the code does something else — that difference is work to
do, not evidence against the entry. If you genuinely believe an entry is
wrong, say so to the user and ask; do not silently overrule it.

More broadly, the remainder of `.spektacular/` — `working-context.md` and
changelog records — is generated output *about* the codebase, not the
codebase itself. A broad grep or file scan run to understand
current-state behavior should treat `.spektacular/specs/`,
`.spektacular/plans/`, `working-context.md` and the changelog as out of scope,
unless the task explicitly concerns them.

## Presenting Drafts and Confirmations

> Managed by `go run . init` — edit `templates/agents/draft-presentation.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

When you have drafted substantial content for the user to review — an architecture write-up, a set of options, a written section, a summary, a plan or spec excerpt — always present that draft as normal, readable chat text first, in full. Never embed the draft itself inside a structured yes/no or multiple-choice dialog element: that kind of UI truncates or compresses long text, making it hard for the user to actually read what you're proposing.

Once the draft has been shown in full as plain text, you may then ask a short, direct confirmation question — e.g. "does this look right, or should anything change?" — using a structured yes/no or multiple-choice element if one is available. That confirmation step comes strictly after the content has already been presented as text, and is never a substitute for showing it.

This rule applies across every workflow step that follows a draft-then-confirm pattern — spec, plan, and implement workflows alike — not just one. If a specific step's own instructions don't repeat this explicitly, this standing rule still applies.

## Knowledge-Worthy Discovery Recognition

> Managed by `go run . init` — edit `templates/agents/knowledge-trigger.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

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

## Spektacular's Files Are Reached Through Spektacular

> Managed by `go run . init` — edit `templates/agents/store-access.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

Every file Spektacular manages is reached through its CLI, never with your own
file tools. This covers specs, plans, their context and research documents,
test plans, changelog records, knowledge entries and design documents.

Never use the `Write` or `Edit` tool on a file under a store directory, and
never build a store path by hand. Use `go run . spec file`,
`go run . plan file`, `go run . changelog file`, `go run . knowledge`
and `go run . design` instead. A write supplies its body with
`--from <path>`, never on stdin and never as prose on the command line.

Removal is a CLI verb too. Deleting a managed file with `rm`, or with any
equivalent of your own, is never correct — not for a knowledge entry, not for
a design document, and not for anything else a store holds. Use
`go run . knowledge delete` and `go run . design delete`, alongside the
`delete` each of `go run . spec file`, `go run . plan file` and
`go run . changelog file` already offers. Going around the tool is how a
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
ticking a phase checkbox in a plan or appending a line to a changelog record.
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

This rule binds everywhere: inside the spec, plan and implement workflows, and
equally in ad-hoc questions, unrelated skills and general exploration. It also
binds every sub-agent you launch. A sub-agent inherits this file but not the
skill that spawned it, so when you delegate work that touches a store, say so
in its prompt.

## Design-Worthy Detail Recognition

> Managed by `go run . init` — edit `templates/agents/design-trigger.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

**Stay alert whenever a conversation is working out how something will
actually behave** — an API's shape, a user-facing flow, a data format, or a
worked example of any of these. Being alert is not the same as offering.
Noticing costs nothing and happens continuously; offering happens only once
the detail clears the bar below. Watching only for detail that has already
settled is how the moment gets missed, because by then the conversation has
usually moved on. Recognizing it is your job, not the user's — don't wait to
be asked.

What you are watching for is a design document: the worked design a feature is
built to, kept wherever the team already keeps its designs, and referenced by
the spec that needs it rather than copied into it.

The bar to offer is deliberately high. A design document is not a place to
park anything technical that came up. Ask whether the detail is **settled**
(the user has decided it, not merely floated it), whether it is **worked** (a
concrete shape, format or flow rather than a direction), and whether it would
**make the spec unreadable if written inline**. All three must hold. A hard
boundary the solution must honour is a constraint and belongs in the spec; a
preference the planner may adapt is technical direction and belongs in the
spec; only a worked design that would swamp the spec belongs in a document of
its own. If a one-line steer captures it, it is not a design document.

Note these are three homes for the *content*, not three degrees of how binding
it is. A design document is settled by definition, so a spec that references
one records that pointer among its **constraints**, never as technical
direction: the plan workflow builds on a referenced design, weighs its options
within the shape it fixes, and raises a disagreement with the user rather than
designing around it.

A design enters a project in more ways than one, and three of them are easy to
walk past:

- **The user already has the document.** They mention a design doc they wrote,
  or paste one in. Nothing needs working out; it needs storing, exactly as
  they supplied it, and referencing.
- **A design that already exists is being changed.** The conversation
  contradicts or extends a design the project already holds. The document is
  now wrong, and saying so is part of your job.
- **There is no spec in sight.** Design talk does not wait for a spec to
  exist. A design can be worked out and written down on its own, and a
  reference recorded later if a spec ever needs one.

When you recognize the moment, offer — never write a design document
unprompted. Say what you would capture, which declared source you would write
it to, and why the spec is better off pointing at it than containing it. Run
`go run . design sources` to see the declared sources if you do not
already know them. Wait for the user's decision before doing anything else.

The user's response falls into one of three outcomes:

- **Accept** — invoke the `spek-design` skill, which owns the conversation
  from here: it runs the interview when the design still has to be worked out,
  stores a document the user already has exactly as supplied, and revises one
  that already exists. It ends in `go run . design author --data
  '{"source":"<name>","path":"<path>"}' --from <staged file>` for a design
  Spektacular writes with the user, or `go run . design write --data
  '{"source":"<name>","path":"<path>"}' --from <staged file>` for one the user
  handed over, which is stored byte for byte with nothing added. Then, **only
  if a spec exists**, record the reference with `go run . design ref add
  --data '{"spec":"<spec name>","source":"<name>","path":"<path>"}'`, because a
  document nothing references is invisible to the plan workflow. When there is
  no spec yet, writing the document is the whole of the work.
- **Defer** ("not now", "later", "once we've settled it") — write nothing.
  Continue the conversation normally, and treat this as temporary: if the
  discussion keeps developing that detail, you may raise the offer again later
  in the same conversation.
- **Decline** ("no", "keep it in the spec") — write nothing, and do not raise
  the offer again for this detail for the remainder of the conversation. A
  decline is final for that detail, not a "not now." Declining also means the
  detail does not get smuggled into the spec body instead: it stays out, or it
  stays as the one-line steer it already was.

Silence or deflection is not acceptance. Never create or overwrite a design
document without the user's explicit agreement — though a direct instruction
to write one *is* that agreement, and needs no further confirmation.
