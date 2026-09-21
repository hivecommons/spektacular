---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Plan: 000055_design-authoring-skill

<!-- Metadata -->
<!-- Created: 2026-09-20T15:38:45Z -->
<!-- Commit: caaa76134ea3c176e867f25e1967111077576e04 -->
<!-- Branch: f-migrate -->
<!-- Repository: git@github.com:jumppad-labs/spektacular.git -->

## Overview

Spektacular can already store and reference a design document, but it assumes the design already
exists and only needs writing down. This plan adds the ability to help a user work one out: a
`spek-design` skill running a guided interview, a standing instruction that catches design
conversation in all the forms it actually takes, and a lifecycle record on the designs
Spektacular authors so they report where they stand, where they came from, and which specs
depend on them. Teams who already keep their own design documents are unaffected, which is the
point: their files are still stored exactly as supplied and gain nothing.

## Conventions

From the `spektacular` repo's always-applied conventions:

- **Error messages must describe the problem and suggest remediation** (`output.NewError(code,
  message).WithNextAction(<runnable command>)`, never a bare `fmt.Errorf`) — this feature adds a
  new write verb and several new refusal paths, and the two that matter most are precisely the
  ones an agent will hit while improvising: `design write` refusing to overwrite an authored
  document, and a reference operation whose back-link write failed. The spec makes the second one
  a constraint in its own right ("the failure must say so and name what to repair"), which is
  this convention applied to the hardest case. The rollback-failed message must name both
  documents, not just report that something went wrong.
- **Tests must not depend on execution order** (`make test` runs `go test -shuffle=on ./...`;
  anything executing `rootCmd` goes through `resetRootCmd`) — the new `design author` command and
  the new flags on it are package-global cobra commands, so their parsed flag values would
  otherwise leak into whichever test runs next. Every new command test drives through
  `resetRootCmd` + `runRootCmd`, exactly as the existing design command tests already document at
  the top of `cmd/design_test.go`. The convention's second clause also applies directly: the
  metadata tests must inject `opts.Today` rather than relying on two writes landing in the same
  second.
- **Passing tests are required before calling work done** (`go test ./...` green, pre-existing
  failures raised rather than dismissed) — this change touches `internal/metadata`, which every
  other artifact class depends on, so a regression here surfaces far from the code that caused
  it. The byte-exact guard on an empty metadata block is the specific test most likely to break
  from a careless field addition, and it must stay green rather than be updated to match new
  output.

From the `docs` repo's always-applied conventions, all binding on the documentation phase:

- **MDX authoring: no layout HTML in page bodies, slots over string props, blank lines around
  slot content, fenced markdown code blocks** — the design documents page is edited and extended,
  so new sections are `Section` + `Prose nested` with fenced `bash` blocks, never `<div>` or
  `class=`. The convention's own verification table is the phase's exit check: the `grep -nE
  "<div|<section|class=" src/pages/*.mdx` guard returns nothing, `npm run build` succeeds, and
  `npx astro check` reports zero errors and zero warnings, which is what the spec's acceptance
  criterion "the site builds with no errors and no warnings" resolves to.
- **No em dashes in authored text** — the documentation phase writes a substantial amount of new
  prose explaining the two classes of design document, and this convention covers page content,
  READMEs and commit descriptions alike. A spaced hyphen is explicitly not an acceptable
  substitute.
- **Alternate section background shading** (`surface` alternates against the section immediately
  before it) — new sections are being inserted into an existing page whose bands already
  alternate, so every subsequent section's `surface` value flips. This is the convention most
  likely to be missed, because the change looks local and its effect is not.
- **Site layout: one frame, one flow width, one heading scale, one component per job** — the new
  content composes from the existing `Section` / `Prose` / `CtaBanner` components rather than
  introducing a new band type. Note the practical consequence recorded when the page shipped: the
  body stylesheet carries no table styling, so a comparison of the two classes of design document
  is authored as bullets, not a markdown table.
- **Plans must sketch content structure, not just summarize it** — this plan requires a
  substantial content change to a documentation page, so the phase covering it carries a concrete
  `**Content outline**` block with headings in order and an illustrative example per section, not
  a prose summary of what the page should cover.

Deliberately not carried:

- **Label before filename in file-scoped reference headings** (`docs`) — it governs reference
  pages that document more than one underlying file, such as the configuration page's
  `config.yaml` / `repo.yaml` split. The design documents page is a concept page with no
  file-scoped sections, so the rule has nothing to act on here.
- The `conventions/README.md` and `glossary/README.md` entries in both repos are category
  descriptors explaining what belongs in each knowledge category, not rules a change can honour
  or violate.

## Architecture & Design Decisions

This feature adds a guided way to *author* a design document, and it does so without touching
the state machine. The work divides into three pieces that meet at one place: a new
`spek-design` skill carrying a Flipped Interaction interview, a rewritten standing instruction in
`AGENTS.md` that puts the agent on alert during design talk and hands off to that skill, and a
new CLI write verb that stamps Spektacular's existing lifecycle metadata onto the documents
Spektacular itself authors. Everything an agent does still goes through the CLI, as it already
does for specs, plans, changelog records and knowledge.

**The skill is a static playbook, not a fifth workflow.** A design FSM would raise the cross-kind
lock (`cross_kind_workflow_in_progress`) and so could not run during a spec workflow, which is
exactly when design conversation happens. `spek-design` therefore follows `spek-knowledge`: it
recognises the user's intent, branches, and calls the CLI directly, holding no workflow state and
running anywhere. It has four branches, matching the four ways a design enters a project:
**author** (nothing written yet, run the interview), **bring in** (the user already has the
document, store it untouched), **revise** (a conversation changed an existing design, rewrite it
in place), and **reference only** (the document is already in a declared source, just record the
reference). The interview is modelled directly on the spec workflow's own interview step, which
implements the Flipped Interaction pattern (White et al., arXiv:2302.11382) with a stated goal,
adaptive questions rather than a script, and an explicit stopping condition. That stopping
condition is not decoration: it is the mechanism the spec requires to stop an interview producing
a transcript rather than a design, and the skill states it as a rule with a worked test ("would
another answer change the document?"). The skill is installed by adding one row to
`workflowSkills` and one to `workflowDescriptions`; every supported agent then installs it, and
because `migrate` re-runs the agent install, existing projects pick it up on upgrade without a
settings-schema migration step.

**The standing instruction separates noticing from qualifying, and hands off by name.** The
current section conflates the two, so the agent only looks once the detail already qualifies, and
it covers only a design *emerging* in conversation. The rewrite opens with a short alert that
applies during any design talk, then states the qualifying bar as the gate on offering, then adds
the entry cases the current section is missing: a design the user already has, a design being
revised, and design talk with no spec in sight. Because a design can be authored before any spec
exists, the accept branch no longer presumes a spec name. Critically, the instruction names the
`spek-design` skill in prose and must not tell the agent to fetch it with `<command> skill
spek-design`: skills nested under the workflow skills directory are not resolvable that way, so
such an instruction would fail at the moment it mattered. The existing regression guard asserting
the section does not contain `skill spek-` stays in place and is the thing that keeps this
honest.

**Metadata splits at the write verb, not in the store.** This is the load-bearing decision. A
design source holds two kinds of document that must behave differently, and the cleanest seam
between them is which command wrote it. A new `design author` verb routes through
`metadata.Merge`, so an authored design gains the project's existing block: `created_date`,
`document_status` from the one four-value vocabulary (`draft`, `final`, `superseded`,
`archived`), an optional `spec` naming the spec whose conversation produced it, and a new `specs`
list holding the back-links. The existing `design write` verb is left exactly as it is, verbatim
and stamping nothing, and gains one guard: it refuses to overwrite a document that already
carries a metadata block, pointing the caller at `design author` instead, because a verbatim
overwrite would silently destroy that document's back-links. Two consequences follow and are
accepted deliberately. First, "Spektacular never adds frontmatter to a design document" narrows
to "never adds frontmatter to a design document it did not author", a real conceptual cost, and
the reason the documentation work is a requirement rather than a nicety. Second, the
discriminator for every later decision is the document itself: a design carrying a block is one
Spektacular authored, one without is not. `metadata.Split` already returns cleanly for a document
with no block, so this needs no registry, no manifest and no naming convention, and the fact
cannot drift from the document it describes. The back-link field is added to `Metadata`,
`yamlShape`, `yamlInShape` and the marshal pair in lockstep, because that schema is closed and
drops any key it does not model on the next render; `Designs []DesignRef` is the worked
precedent for doing this correctly.

**Back-link maintenance belongs to the reference verbs, and fails as a unit.** `design ref add`
and `design ref remove` are the only places a reference is created or destroyed, so they are the
only places a back-link can be kept accurate. Each becomes a two-document write with no
transaction available, which the spec settles by fixing the failure behaviour: the operation
fails as a whole and the spec is left exactly as it was. The order that satisfies that is
spec-first, back-link-second, restoring the spec's original bytes if the back-link write fails;
`refsOf` already returns those original bytes, so the rollback payload costs nothing to obtain.
The reverse order would make the spec trivially untouched on failure but would break the
requirement that a design never lists a spec that does not reference it, so it is rejected rather
than preferred. If the rollback itself fails, the error says so explicitly and names both
documents to repair, which is what the project's error convention demands of every refusal. A
design carrying no metadata block records no back-link at all: `ref add` and `ref remove` still
succeed, and the document is not touched, which is what keeps a pre-existing design referenceable
without gaining anything. The same is true of a reference recorded before its document exists,
which the reference verbs already allow on purpose.

Reporting follows the same split. `design read` already returns raw bytes, so an authored
design's block is visible on read with no change. `design list` gains the metadata fields for
documents that carry a block, mirroring what `spec file list` already reports, which is how an
authored design comes to report its status, its provenance and the specs referencing it while a
pre-existing design continues to list as nothing but a source and a path. Designs are
deliberately **not** added to the cross-kind `artifacts list` surface or its shared filter: no
requirement asks for it, the spec's non-goals rule out indexing designs, and that scan is built
for in-project stores while a design source may point anywhere on disk.

The documentation work lands in the `docs` repo and is a requirement in its own right, because
two classes of design document is exactly the kind of distinction users get wrong if it is not
written down. It is not additive: the design documents page currently states flatly that
Spektacular never adds frontmatter to a design document and that a document written through the
CLI comes back byte for byte identical, and that paragraph has to be reworked rather than
appended to.

Rejected alternatives, with citations, are recorded in
`research.md#alternatives-considered-and-rejected`: the design FSM, fetching the skill through
the CLI, a parallel metadata schema, an unmodelled YAML key for back-links, derived-on-demand
back-links, reusing the store-file command factory, a registry-based authored/pre-existing
discriminator, and the reverse write order.

## Component Breakdown

**Artifact metadata (changed).** The project's single definition of the frontmatter block every
Spektacular-written document carries, and of the four-value document status vocabulary. It gains
one field: the list of specs that reference this document, the reverse of the design-reference
list a spec already carries. It also gains the matching update option, with the same three-state
semantics the existing reference list uses, so that a body-only rewrite preserves back-links
while an explicit replacement sets them. It is the only component that knows the on-disk shape of
a metadata block, and every other component in this feature reaches that shape through it rather
than parsing or emitting YAML itself. Nothing else about its behaviour changes: the created date
is still stamped once and preserved, the closed date still stamped once on the first transition
to a closed status, and a document with no block is still read as a bare document rather than an
error.

**The design source set (unchanged).** Owns resolving the project's declared design sources to
directories, and reading, writing, listing and locating documents within them. It continues to
store bytes exactly as given and to add nothing of its own. This feature deliberately does not
teach it about metadata: it stays the byte-level layer, and everything that knows a design might
carry a lifecycle block sits above it. That keeps the guarantee for a team's own folder intact at
the layer that actually touches their files.

**The authored-design write command (new).** A sibling to the existing verbatim write verb. It
owns exactly one thing: taking a staged document and storing it into a declared source with
Spektacular's lifecycle block merged in. It composes the artifact metadata component with the
design source set, so the block is produced by the same code that produces a spec's or a plan's,
and the bytes land through the same store path as any other design write. It accepts a lifecycle
status and an originating spec name, both optional, and it preserves the created date and the
existing back-link list when the document already exists, which is what makes revising an
authored design an update in place rather than a replacement. It is the only component that
stamps a block onto a design document.

**The verbatim design write command (changed).** Keeps its whole existing contract: the staged
bytes are stored exactly as supplied, with nothing added, removed, reordered or reformatted. It
gains a single guard. If the document it is about to overwrite already carries a lifecycle block,
it refuses and names the authored-write command as the corrective step. Without that guard a
verbatim overwrite would silently strip an authored design's block and with it every back-link,
leaving specs referencing a design that no longer lists them, and no error would be raised.

**The design reference verbs (changed).** They already own the only two moments at which a
reference between a spec and a design comes into existence or ceases to, which is why back-link
maintenance belongs to them and nowhere else. Each becomes a two-document operation: it records
or removes the reference on the spec, then brings the design's own back-link list into agreement.
They are responsible for the consistency rule in both directions, and for the failure behaviour
when the second write cannot be completed: the operation fails as a whole, the spec is returned
to its previous state, and if that restoration itself fails the refusal says so and names both
documents to repair. They are also responsible for the two cases where there is nothing to write:
a design that carries no block, and a reference recorded before its document exists. Both
succeed, and neither touches the design.

**The design listing command (changed).** Reports what each declared source holds. It gains the
lifecycle fields for any document that carries a block, which is how an authored design comes to
report its status, when it was captured, the spec its conversation belonged to, and the specs
that reference it. A document with no block continues to list as nothing but a source and a path,
so the listing is where the two classes become visible side by side. The read command is
unchanged: it already returns a document's raw bytes, so an authored design's block is visible on
read without any work.

**The `spek-design` skill (new).** A static playbook, not a workflow: it holds no state machine
and can therefore run inside a spec conversation, which is when design talk actually happens. It
owns recognising which of four situations the user is in and driving the right one: authoring a
design that does not exist yet, bringing in one the user already has, revising one that exists,
or recording a reference to one already in a declared source. The authoring branch owns the
Flipped Interaction interview, which means it owns a stated goal, adaptive questions rather than
a script, and the stopping condition that keeps an authored design proportionate to the design
rather than a transcript of the conversation. Every branch ends in a direct CLI call, and every
branch that writes is gated on explicit user agreement stated in prose. It composes the commands
above and adds no storage or addressing of its own.

**The workflow skill registry (changed).** The single table naming the skills every supported
agent installs, paired with the table of descriptions rendered into the slash-command wrappers
for agents without a native skill mechanism. Adding the new skill to both is what makes it
install for every agent, and what makes an existing project receive it when it upgrades, since
the upgrade path re-runs the agent install rather than needing its own migration step.

**The standing design instruction (changed).** The managed section in the project's agent
instruction file that tells an agent to watch for design conversation. Today it conflates
noticing with qualifying, covers only a design emerging in conversation, says nothing about
revising one, and presumes a spec exists. It is restructured into an alert that applies during
any design talk, then the qualifying bar as the gate on offering, then the entry cases for a
design the user already has and one being revised, with the accept branch no longer requiring a
spec. It hands off by naming the skill, never by instructing the agent to fetch it through the
CLI, because skills nested under the workflow skills directory cannot be fetched that way.

**The spec-side design guidance in the existing skills and step templates (changed).** The spec
skill and the spec workflow's technical-approach step both already describe capturing a design.
They are updated so their offer points at the new skill for the authoring case rather than
implying the only path is writing down a design that already exists in the conversation, and so
the command list they publish includes the authored-write verb. The plan skill's consumption
side is unchanged: it reads references and documents exactly as it does now.

**The documentation site's design documents page (changed).** Owns the public explanation of what
a design document is and how Spektacular relates to one. It gains an explanation of guided
authoring and of what each metadata field on an authored design means, and its existing statement
that Spektacular never adds frontmatter to a design document is reworked into the narrower and
now-correct claim about documents it did not author. The configuration page and the project
README carry the same narrowing wherever they repeat that guarantee.

## Data Structures & Interfaces

### The metadata block gains one field

`Metadata` is the in-memory mirror of the YAML frontmatter block every Spektacular-written
document carries. It gains `Specs`, the list of spec names that reference this document. It is
the exact reverse of the `Designs` list a spec already carries, and only a design document
carries it today.

```go
type Metadata struct {
    CreatedDate    time.Time
    DocumentStatus DocumentStatus   // draft | final | superseded | archived
    ClosedDate     time.Time
    Project        string
    ProjectSource  string
    Spec           string           // provenance: the spec this document's conversation belonged to
    Plan           string
    Designs        []DesignRef      // spec -> design: the designs this artifact references
    Specs          []string         // design -> spec: the specs that reference this design
}
```

`Spec` and `Specs` are deliberately different facts rather than one field doing double duty: a
design originates in at most one spec's conversation and may be referenced by many. The
serialization boundary is a closed schema, so the field is added to the encode shape, the decode
shape and the marshal pair together; a key the schema does not model is dropped on the next
render, which is the failure mode this lockstep exists to prevent.

On disk, an authored design's block and the spec that references it are mirror images:

```yaml
# the design document, authored by Spektacular
---
created_date: "2026-09-21"
document_status: draft
spec: 000055_design-authoring-skill     # provenance, omitted when authored outside a spec
specs:                                  # back-links, omitted when nothing references it
    - 000055_design-authoring-skill
---
```

```yaml
# the spec that references it, unchanged by this feature
---
created_date: "2026-09-20"
document_status: final
designs:
    - source: design
      path: authoring-flow.md
---
```

A design document the project already had carries none of this. It has no block at all, which is
what the whole feature keys off.

### The update contract gains a matching option

`UpdateOptions` carries caller-supplied field updates into the merge that computes a document's
new bytes. `Specs` joins it with the same three-state pointer semantics the existing `Designs`
option uses, because the same three states are genuinely distinct and a plain slice cannot
express them.

```go
type UpdateOptions struct {
    DocumentStatus *DocumentStatus
    Today          time.Time
    Project        string
    ProjectSource  string
    Spec           string
    Plan           string
    Designs        *[]DesignRef
    Specs          *[]string      // nil: leave alone | non-nil: replace | non-nil empty: clear
}
```

The `nil` case is the important one and it is what an ordinary authored-design rewrite passes: it
means back-links survive a revision that only changes the document's body. The reference verbs
are the only callers that pass a non-nil value.

### The authored write is a new command, not a new interface

No new Go interface is introduced. The authored-design write composes two existing contracts that
already have the right shapes: the metadata merge that produces a document's bytes from an
existing blob, a new body and a set of updates, and the design source set's byte-level write. Its
own contract is the command-line one, which is the contract agents actually consume.

```
design author --data '{"source":"<name>","path":"<path within the source>"}' \
              --from <staged file> \
              [--document-status draft|final|superseded|archived] \
              [--spec <spec name>]

-> { "source": ..., "path": ..., "location": ..., "document_status": ..., "created_date": ... }
```

The address shape is identical to the existing read and write verbs, so nothing new has to be
learned to address a document. `--from` is required for the same reason it is on the verbatim
write: a document's content is read from a file, never from prose on the command line.

### The listing envelope grows optional fields

`design list` currently returns a document as a source and a path. It gains the lifecycle fields,
present only when the document carries a block, which is how the two classes become
distinguishable from the listing alone.

```
-> { "documents": [
       { "source": "design", "path": "authoring-flow.md",
         "created_date": "2026-09-21", "document_status": "draft",
         "spec": "000055_design-authoring-skill",
         "specs": ["000055_design-authoring-skill"] },
       { "source": "design", "path": "legacy/api-sketch.md" }
     ] }
```

The second entry is a design the project already had: same envelope, no lifecycle keys. This
mirrors how the spec, plan and changelog listings already report metadata for artifacts that
carry it and omit it for those that do not.

### The reference verbs' envelopes are unchanged

`design ref add`, `design ref remove` and `design ref list` keep the shapes they publish today.
Back-link maintenance is a side effect on the design document, not a change to what the reference
verbs return, so an existing caller sees no difference. What changes is the set of failures they
can produce: a back-link write that cannot be completed now fails the whole operation, and that
refusal carries a distinct error code plus a next action naming the documents involved. The
rollback-failed case is a second, separate code, because the corrective action differs: one says
retry, the other says repair these two files by hand.

## Implementation Detail

**No new module boundary, and no new abstraction over the two write paths.** The codebase already
has two ways to write a document: one that merges Spektacular's lifecycle block in, used by
specs, plans and changelog records, and one that stores bytes untouched, used by design
documents. Both already exist and both already work. This feature does not unify them behind an
interface, does not teach the design source set about metadata, and does not add a fifth command
family. It adds one command that composes the two existing pieces, and that composition is the
whole of the new mechanism. A developer reading the changed code should find nothing structurally
novel: the authored write reads like a design write with a merge step in front of it, because
that is exactly what it is.

**The one genuinely new pattern is a two-document write with a compensating rollback.** Nothing
else in this codebase writes two files that must agree. The reference verbs become the first
place that does, and there is no transaction available to make it atomic, so the shape has to be
explicit rather than incidental: capture the spec's original bytes before touching it, write the
spec, attempt the back-link, and on failure write the captured bytes back. This is a compensating
action rather than a real rollback, and the distinction matters because the compensation can
itself fail. That third outcome is a named, reported state rather than an unhandled one, and it
is the part of this feature most likely to be implemented carelessly. The sequence belongs in one
place shared by both reference verbs rather than written twice, since add and remove differ only
in how they compute the new list.

**Deciding whether a design is one Spektacular authored is a read, not a lookup.** Every branch
that needs to know asks the document: split its bytes, and treat the presence of a metadata block
as the answer. This keeps the fact in the document rather than in a registry that could disagree
with it, and it reuses the existing parser's established behaviour that a document with no block
is a bare document rather than an error. The consequence a reader should expect is that several
commands now do a read they did not do before, and that a design source's contents are parsed
where previously they were only listed or copied. Nothing about that parse is new code; it is the
same split the spec, plan and changelog listings already perform.

**Adding a field to the metadata block is a lockstep change with a known trap.** The on-disk
schema is closed: a key it does not model is silently dropped the next time the block is
rendered, which means a half-finished field addition does not fail, it quietly loses data on the
second write. The existing design-reference field is the worked precedent for doing this
correctly, including a byte-exact regression test proving that a document with no references
renders exactly the bytes it did before the field existed. The new field copies that precedent,
including the test, and the decode side copies the existing leniency: a malformed or unexpected
value reads as no back-links rather than failing the parse and making the document unreadable.
Leniency on read and strictness on write is the established split here and is not being changed.

**The skill is prose, and its contract is enforced by prose plus template tests.** This is the
established shape for everything agent-facing in this project: judgement lives in markdown
templates rather than in Go, and the regression tests assert that specific anchor phrases are
present, or absent, in the rendered template. The new skill and the rewritten standing
instruction are both tested that way, with hand-maintained literal phrases rather than anything
derived from the templates themselves, since deriving them would make the assertions
tautological. Two negative assertions carry real weight and should be read as load-bearing rather
than defensive: the standing instruction must not tell an agent to fetch the skill through the
CLI, because skills nested under the workflow skills directory cannot be fetched that way and the
instruction would fail at precisely the moment it mattered; and the propose-then-confirm gate
before any write is stated in prose because there is no CLI guard behind it, exactly as the
knowledge skill already documents about itself.

**Installation needs no migration step, which is worth stating because it looks like it should.**
The project has a settings-schema migration registry, and adding a capability feels like
something that belongs in it. It does not. The upgrade path re-runs the full agent install, so
adding a row to the skill registry is sufficient for both a fresh install and an existing project
upgrading. The migration registry is for config format changes only, and this feature changes no
config format.

**The documentation change is a correction, not an addition.** The public page currently makes an
unqualified claim that this feature falsifies. A developer approaching the docs work as "add a
section about authoring" will produce a page that contradicts itself two screens apart. The
existing claim is narrowed first, then the new material is added around it. The site's own
conventions also make one non-obvious demand: section background shading alternates against the
preceding section, so inserting a section flips every subsequent one, and the effect of the
change is not local to the lines edited.

## Dependencies

**Design documents this plan was built on: none.** The spec carries no design references;
`design ref list` for it returns an empty list with nothing unresolved. Stated explicitly so the
absence reads as a fact rather than a step that was skipped.

### Repos

- **`spektacular`** (role: tool) — carries all of the work except the public documentation: the
  metadata field, the authored-write command, the guard on the verbatim write, back-link
  maintenance in the reference verbs, the listing change, the new skill template, the rewritten
  standing instruction, and the registry rows that install it. No changes needed to the repo
  itself before this plan starts.
- **`docs`** (role: documentation) — carries the documentation requirement and its acceptance
  criterion. One caveat that affects sequencing rather than scope: the design documents page and
  its navigation and configuration edits are currently uncommitted in that repo's working tree,
  so the implementation must check that tree's state before editing and re-read the page if it
  has since been committed, rebased or discarded.

### Internal packages

- **Artifact metadata** — provides the frontmatter block, the four-value document status
  vocabulary, the split and render pair, and the merge that computes a document's new bytes.
  **Changes required**: one new field carried through the in-memory type, both serialization
  shapes and the marshal pair, plus the matching update option and its preserve-versus-replace
  branch in the merge.
- **The design source set** — provides resolution of declared sources, and byte-level read,
  write, list, resolve and exists over the documents in them. **No changes required**: it stays
  the byte-level layer deliberately, and everything metadata-aware composes it from above.
- **The output package** — provides the error type carrying a code, a resource and a next action,
  and the success envelope every command writes. **No changes required**; the new refusals use it
  as every other refusal in the design commands already does.
- **The store layer** — provides the path-safe reader and writer the spec store and each design
  source are built on. **No changes required.**
- **The agent install package** — provides the skill registry, the command-wrapper descriptions
  and the managed-section installer that writes the standing instruction into the project's agent
  instruction file. **Changes required**: one row in each of the two registries, and the
  rewritten section template it renders.
- **The spec and plan step packages** — provide the workflow step templates, one of which carries
  a duplicate of the capture offer. **Changes required**: the spec workflow's technical-approach
  step is updated so its offer covers authoring rather than capture alone.

### External libraries

- **`gopkg.in/yaml.v3`** — already the encoder and decoder behind the metadata block, including
  the raw-node decoding that makes reads lenient. Used as-is, no version change.
- **`github.com/spf13/cobra`** — already the command framework; the new command is registered the
  same way as its siblings. No version change.
- **`github.com/cbroglie/mustache`** — already renders skill and instruction templates, including
  the partial the new skill opens with. No version change.
- **`github.com/stretchr/testify`** — already the assertion library throughout. No version change.
- **Astro 5, MDX and Tailwind v4** in the docs repo — already the site's toolchain; the
  documentation work composes existing components and adds no dependency. Verification uses the
  site's existing build and type-check commands.

### Prior specs and plans

- **`000054_project-level-design-documents`** — the spec and plan this one extends. It delivered
  design sources, addressing, the read, write, list and reference commands, the capture offer and
  the plan workflow's obligation to resolve references. It must be, and is, already landed; this
  plan changes its surfaces rather than duplicating them. Its research also records the two
  rejections this work revisits, one of which stands and one of whose premises no longer holds.
- **`000052_document-status-vocabulary`** — fixed the four-value status vocabulary an authored
  design is required by constraint to reuse. Already landed; no changes.
- **`000041_workflow-knowledge-capture-offers`** — established the accept, defer and decline
  outcome vocabulary the rewritten instruction keeps, and first recorded that skills nested under
  the workflow skills directory cannot be fetched through the CLI. Already landed; no changes.
- **`000022_spek-knowledge-skill`** — the static-playbook precedent the new skill is modelled on:
  intent recognition, one branch per intent, direct CLI calls, and a propose-then-confirm gate
  enforced by prose. Already landed; no changes.
- **`000043_flipped-interaction-spec-interview`** — the interview the design interview mirrors,
  and the source of the stated-goal, adaptive-questions, explicit-stopping-condition shape.
  Already landed; no changes.

### Nothing blocks the start of this plan

No dependency has to land or change first. Every internal package, library and prior plan this
work builds on is already in place; the only sequencing note is the uncommitted state of the docs
repo's working tree, which is a thing to check rather than a thing to wait for.

## Testing Approach

Testing follows the project's established three-layer model, and this feature lands in the first
two layers only. The first layer is Go unit and command tests covering deterministic mechanics:
the metadata field's round-trip and merge semantics, the authored write, the guard on the
verbatim write, back-link maintenance and its failure behaviour, and the listing change. The
second is template-contract tests, which are how prose-driven behaviour is regression-tested
here: the new skill and the rewritten standing instruction are markdown, so their guarantees are
asserted as the presence or absence of hand-maintained anchor phrases in the rendered template.
The third layer, the end-to-end harbor suites, gains nothing new, for a reason given below.

**The metadata field carries the heaviest unit coverage relative to its size**, because it is the
one change that reaches every other artifact class in the project. The load-bearing assertions
are that a document with no back-links renders byte-for-byte identically to how it rendered
before the field existed, that back-links survive a rewrite that only changes the document's
body, that an explicit empty list clears them, and that a malformed or unexpected value on disk
reads as no back-links rather than failing the parse. The first of those is the specific
regression guard the existing design-reference field established and is the test most likely to
catch a careless field addition; it is copied rather than reinvented.

**The reference verbs carry the heaviest behavioural coverage**, because the two-document write
is the only genuinely new mechanism and the spec fixes its failure behaviour by constraint. In
plain language the tests guarantee: recording a reference to an authored design adds the spec to
that design's list and the design to the spec's; removing it removes both; at no point does an
authored design list a spec that does not reference it, nor omit one that does; recording and
removing a reference to a design that carries no block both succeed and leave that document
byte-for-byte unchanged; the same is true of a reference recorded before its document exists; and
when the back-link write cannot be completed the whole operation fails and the spec is left
exactly as it was. The case where the compensating write itself fails is tested too, asserting
that the refusal says so and names what to repair rather than reporting a generic failure. That
last one needs an injected write failure rather than a real one, which is the only place this
feature needs a test seam that does not already exist.

**The two classes of design document are tested as a pair, not separately.** The assertions that
matter are comparative: an authored design carries a block and reports its status, its capture
date, its originating spec and its referencing specs when listed; a design the project already
had carries no block after being read, referenced, de-referenced or written through the verbatim
path, and lists as nothing but a source and a path. Testing each in isolation would let both
pass while the boundary between them drifted.

**Command tests follow the existing design-command conventions unchanged.** Every command is
driven through the shared reset-and-run helpers rather than by calling its handler directly,
because the commands are package-level globals whose parsed flags would otherwise leak into
whichever test the shuffled runner executes next. Fixtures seed design documents with direct file
writes rather than through the commands under test, keeping the oracle independent of the code
being exercised, and the existing two-source project fixture (one source relative, one absolute
and outside the project root) is reused so the new verb is exercised against both shapes.
Metadata tests inject the clock rather than relying on two writes landing in the same second.

**Template-contract tests cover the prose surfaces.** For the standing instruction: that noticing
and qualifying are stated as separate things, that the entry cases for a pre-existing design and
a revision are present, that the three response outcomes survive with a decline stated as final,
that a non-answer is not acceptance, and, as a negative assertion, that it never instructs the
agent to fetch the skill through the CLI. For the skill: that all four branches are present and
each names the command it ends in, that the interview states a goal and an explicit stopping
condition, and that the write gate is stated as requiring explicit agreement. The existing
hand-maintained table pinning which skill must name which commands gains a row for the new skill.
These assertions use literal phrases maintained by hand; deriving them from the templates would
make them tautological, which is a rule this project has already learned and written down.

**Deliberate gaps.** No new harbor end-to-end suite is added. The suites exist to prove that a
real agent drives a state machine correctly, and this feature deliberately adds no state machine;
a static playbook has no step order to verify. The existing suites are checked for drift rather
than extended, since a change to skill or template surfaces is exactly the kind of change their
hand-maintained oracles are coupled to. There is also no test that an agent conducts a good
interview: that is judgement, and the testable proxy, that the stopping condition is stated as a
rule, is covered by the template-contract layer.

### Success metrics

- **Design conversation produces a design rather than being lost.** *Manual — captured in the
  implementation test plan.* The metric is about conversations that happen after delivery, so no
  automated test can assert it. The testable proxy, that the instruction surface actually tells
  the agent to watch and offer, is covered by the template-contract assertions above; the metric
  itself is observed.
- **Users are helped to write designs they would not have written unaided.** *Manual — captured
  in the implementation test plan.* Whether a design was produced by interview or handed over
  complete is not recorded anywhere and is not worth recording; this is observed over real use.
- **Bringing in an existing design costs no rework.** *Behavioural test.* Guaranteed by the
  round-trip assertion that a document stored through the verbatim path reads back
  byte-identical, together with the assertions that referencing and de-referencing it leave it
  unchanged and that it never gains a block. This metric is fully covered automatically.
- **Back-links can be trusted.** *Behavioural test.* This is the bidirectional consistency
  guarantee described above, including both failure paths. Fully covered automatically.
- **The two classes of design document do not confuse users.** *Manual — captured in the
  implementation test plan.* User confusion is not assertable. The documentation requirement is
  the mitigation, and its own acceptance criterion (the site explains the distinction and builds
  clean) is checked by the documentation phase's build and type-check gates.
- **Authored designs stay proportionate.** *Manual — captured in the implementation test plan.*
  Document length relative to the design it describes is a judgement about output quality. The
  mechanism the spec requires, an explicit stopping condition in the interview, is asserted by a
  template-contract test; whether it produces proportionate designs in practice is observed.

## Milestones & Phases

### Milestone 1: Designs Spektacular writes can carry a lifecycle

**What changes**: Until now every design document looked the same to Spektacular, a file it
stored and handed back without comment. After this milestone there are two kinds. A design the
project already had is untouched exactly as before, and a design Spektacular writes on the user's
behalf carries the same lifecycle record every spec and plan already carries: when it was
captured, where it stands in its life, and which spec's conversation produced it. Listing a
source shows the difference plainly, with the team's own files reported as nothing more than a
source and a path. Writing a design the old way over one Spektacular authored is refused rather
than silently stripping that record, and the refusal says which command to use instead.

**Validation point**: a document written through the new authored path reads back with its
lifecycle block, and a document written through the existing path reads back byte-identical with
no block; listing a source distinguishes the two; the verbatim write refuses to clobber an
authored document and names the corrective command; and the whole Go suite passes shuffled,
including the byte-exact guard proving that a document with no back-links renders exactly as it
did before the new field existed.

#### - [x] Phase 1.1: Record which specs reference a design

**Repo:** spektacular

The lifecycle record Spektacular stamps on the documents it writes gains one more fact: the list
of specs that reference this document. It is the exact reverse of the list of designs a spec
already carries, and it is what lets an authored design report who is using it. The block's
on-disk schema is closed, so a field added in only some of the places it must appear does not
fail loudly, it quietly loses the value on the next write. This phase adds it everywhere it
belongs in one go, and copies the regression guard that proves a document with nothing to record
still renders exactly the bytes it did before.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-record-which-specs-reference-a-design)

**Acceptance criteria**:

- [x] A document written with a list of referencing specs reads back with that list intact, in
      order.
- [x] A rewrite that changes only a document's body leaves its referencing-spec list untouched.
- [x] Explicitly clearing the list removes it from the document entirely rather than leaving an
      empty entry behind.
- [x] A document with no referencing specs renders byte-for-byte as it did before this field
      existed.
- [x] A referencing-spec list that has been hand-edited into something malformed reads as no
      references rather than making the document unreadable.

#### - [x] Phase 1.2: Write a design document Spektacular authored

**Repo:** spektacular

A new command stores a staged document into a declared source with the lifecycle record merged
in, alongside the existing command that stores one untouched. It takes the same address as every
other design command, reads its content from a file rather than the command line for the same
reason the existing write does, and optionally takes a lifecycle status and the name of the spec
whose conversation produced the document. Writing over a document that already exists preserves
when it was first captured and the specs already referencing it, which is what makes revising an
authored design an update in place rather than a replacement.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-write-a-design-document-spektacular-authored)

**Acceptance criteria**:

- [x] A document written through the new command reads back carrying its capture date and its
      lifecycle status.
- [x] Naming the spec a design came from records it on the document, and leaving it out records
      nothing in its place.
- [x] Rewriting an existing authored design keeps its original capture date and its existing
      referencing-spec list while replacing its content.
- [x] Setting a closed lifecycle status records when it closed, once, and returning it to draft
      clears that.
- [x] A status outside the project's four allowed values is refused, and the refusal lists the
      values that are allowed.
- [x] Writing to a source the project has not declared is refused, and the refusal names the
      sources it could have used.
- [x] Omitting the content file is refused with an explanation of how to supply one.
- [x] The command publishes its own input and output shapes on request, as its sibling commands
      do.

#### - [x] Phase 1.3: Keep the two kinds of design document apart

**Repo:** spektacular

With two kinds of design document in one folder, two things have to hold. Writing the old,
verbatim way over a document Spektacular authored is refused, because it would silently strip
that document's lifecycle record and with it every spec that references it, and the refusal points
at the command to use instead. And listing a source now shows the difference: an authored design
reports its status, its capture date, its originating spec and the specs referencing it, while a
document the project already had reports nothing but where it lives. A file carrying frontmatter
of the team's own that Spektacular did not write counts as one of theirs, not one of ours.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-keep-the-two-kinds-of-design-document-apart)

**Acceptance criteria**:

- [x] Writing verbatim over an authored design is refused and nothing on disk changes.
- [x] The refusal names the command that performs an authored write instead.
- [x] Writing verbatim over a document the project already had still succeeds and stores the bytes
      exactly as supplied.
- [x] Creating a brand new document through the verbatim command still succeeds.
- [x] Listing a source reports the lifecycle fields for an authored design and omits them entirely
      for a document that has none.
- [x] A document carrying frontmatter of the team's own is treated as theirs: it lists without
      lifecycle fields and can still be overwritten verbatim.
- [x] Reading any design document still returns its bytes exactly as stored.

### Milestone 2: A design and the specs that reference it never disagree

**What changes**: Recording that a spec uses a design now updates both sides of that
relationship, so an authored design knows which specs reference it and never reports one that has
since dropped it. Removing a reference removes it from both. When the two writes cannot both be
completed the whole operation is refused and the spec is put back the way it was, so a spec and a
design are never left contradicting each other; in the rare case that even the restoration fails,
the failure says so and names the two documents to put right rather than reporting a vague error.
Designs the project already had gain nothing from any of this: referencing one still works, and
the file is not touched.

**Validation point**: a reference recorded on a spec appears on the authored design and
disappears from it when removed; referencing and de-referencing a design that carries no block
both succeed and leave that file byte-identical; a reference recorded before its document exists
still succeeds; a forced failure of the second write leaves the spec exactly as it was and
reports it; and a forced failure of the restoration reports the distinct outcome and names both
documents.

#### - [x] Phase 2.1: Keep a design and its referencing specs in agreement

**Repo:** spektacular

Recording that a spec uses a design becomes a change to both documents rather than one. The spec
gains the reference as it does today, and the authored design gains the spec in its own record;
removing the reference removes it from both. A design the project already had takes part in this
without gaining anything: the reference still goes on the spec, and the design file is not
touched. The same is true when a reference is recorded before its document exists, which the
reference commands already allow on purpose.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-keep-a-design-and-its-referencing-specs-in-agreement)

**Acceptance criteria**:

- [x] Recording a reference adds the spec to the authored design's list of referencing specs.
- [x] Removing a reference removes that spec from the design's list and leaves any others intact.
- [x] Recording the same reference twice still changes nothing the second time, and the design's
      list contains the spec once.
- [x] Two specs referencing one design both appear on it, and removing one leaves the other.
- [x] Recording or removing a reference to a design that has no lifecycle record succeeds and
      leaves that file byte-for-byte unchanged.
- [x] Recording a reference to a document that does not exist yet still succeeds.
- [x] An authored design never lists a spec that does not reference it, and never omits one that
      does.

#### - [x] Phase 2.2: Never leave a spec and a design disagreeing

**Repo:** spektacular

The two writes cannot be made atomic, so the behaviour when the second one fails is defined
rather than left to chance. The reference operation fails as a whole and the spec is put back
exactly as it was, so the pair is never left contradicting each other. If putting the spec back
also fails, that is reported as its own distinct outcome naming both documents to repair by hand,
because the corrective action is completely different from the ordinary failure and an agent that
cannot tell them apart will do the wrong thing.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-never-leave-a-spec-and-a-design-disagreeing)

**Acceptance criteria**:

- [x] When the design's record cannot be updated, the reference operation fails and the spec is
      left exactly as it was before the operation started.
- [x] That failure explains what went wrong and gives a runnable next step.
- [x] When restoring the spec also fails, the failure says so explicitly and names both the spec
      and the design that need repairing.
- [x] The two failures are distinguishable by an automated caller, not only by reading their
      wording.
- [x] Removing a reference has exactly the same guarantees as recording one.

### Milestone 3: The agent helps work a design out, not just write one down

**What changes**: This is the milestone the feature is named for. An agent now stays alert
whenever a conversation is working out how something will actually behave, rather than only
noticing once the detail has already settled, and it offers at the point the detail qualifies
instead of letting it pass. It handles the three situations the current behaviour misses
entirely: a design the user already has and only wants stored, a design that exists and is being
changed, and design talk happening before any spec exists. Where nothing is written down yet the
agent can now help the user work it out, asking adaptive questions toward a stated goal and
stopping once further questions would not change the result, then producing the document from
that conversation. Nothing is ever written without the user's explicit agreement, and a decline
leaves no file behind and does not quietly push the detail into the spec instead.

**Validation point**: the skill installs for every supported agent on a fresh initialisation and
on an upgrade of an existing project; the standing instruction reads as alert-then-qualify with
all three entry cases present and hands off by naming the skill rather than telling the agent to
fetch it; the spec workflow's own capture offer no longer contradicts it; and the template
assertions covering all of that pass, including the negative one that no instruction tells an
agent to fetch a nested skill through the command line.

#### - [x] Phase 3.1: Add the design skill

**Repo:** spektacular

The skill that does the authoring work. It recognises which of four situations the user is in and
drives the right one: authoring a design nothing has been written for yet, bringing in one the
user already has, revising one that exists, or just recording a reference to one already stored.
The authoring branch runs an interview with a stated goal and adaptive questions, and stops once
a further answer would not change the resulting document, which is what keeps an authored design
the size of the design rather than the size of the conversation. Nothing is written without the
user's explicit agreement. Adding the skill to the two registries is what installs it for every
supported agent and delivers it to existing projects when they upgrade.

*Technical detail:* [context.md#phase-31](./context.md#phase-31-add-the-design-skill)

**Acceptance criteria**:

- [x] Initialising a project installs the skill for the configured agent alongside the existing
      ones.
- [x] Upgrading an existing project installs it too, without any settings change.
- [x] Agents without a native skill mechanism get a command wrapper for it with a meaningful
      description.
- [x] The skill describes all four situations and names the command each one ends in.
- [x] The interview states its goal and an explicit condition for stopping.
- [x] The skill states that nothing is written without explicit agreement, and that a decline
      leaves no file behind and does not push the detail into the spec instead.
- [x] Nothing in the skill tells an agent to fetch another skill through the command line.

#### - [x] Phase 3.2: Rewrite the standing design instruction

**Repo:** spektacular

The standing rule that tells an agent to watch for design conversation is restructured. Today it
treats noticing and qualifying as the same moment, so the agent only looks once the detail has
already settled, and it covers only a design emerging in conversation. It becomes an alert that
applies during any design talk, then the qualifying bar as the gate on offering, then the cases it
currently misses: a design the user already has, a design being revised, and design talk before
any spec exists. It hands off by naming the skill, never by telling the agent to fetch it, because
skills of that kind cannot be fetched that way and the instruction would fail exactly when it
mattered.

*Technical detail:* [context.md#phase-32](./context.md#phase-32-rewrite-the-standing-design-instruction)

**Acceptance criteria**:

- [x] The instruction tells the agent to stay alert during design conversation, separately from
      the bar a detail must meet before an offer is made.
- [x] It covers a design the user already has, one being revised, and design talk with no spec in
      existence.
- [x] Its accept path no longer assumes a spec exists to reference.
- [x] It hands off to the skill by name and never instructs the agent to fetch it through the
      command line.
- [x] It keeps the three response outcomes, with a decline stated as final for that detail and
      silence stated as not acceptance.
- [x] Re-running installation leaves exactly one copy of the section, in its existing position
      relative to the other managed sections.

#### - [x] Phase 3.3: Align the spec workflow's own design offer

**Repo:** spektacular

Two other places describe capturing a design: the spec skill and the spec workflow's
technical-approach step. Both currently present writing down an already-settled design as the only
path, and one of them states that a design document never gains frontmatter, which stops being
true of authored ones. They are brought into line with the standing instruction so an agent gets
the same answer wherever it reads, which matters because the technical-approach step is exactly
where design conversation happens.

*Technical detail:* [context.md#phase-33](./context.md#phase-33-align-the-spec-workflows-own-design-offer)

**Acceptance criteria**:

- [x] The spec workflow's technical-approach step offers authoring as well as capture, and points
      at the skill for it.
- [x] Its accept path works whether or not the design already exists in written form.
- [x] The spec skill's list of design commands includes the authored write and describes the
      verbatim one accurately, as adding no frontmatter to documents Spektacular did not author.
- [x] No instruction surface claims Spektacular never adds frontmatter to a design document
      without qualifying which documents it means.
- [x] The plan skill's side is unchanged: it still reads references and documents exactly as
      before.

### Milestone 4: The documentation explains both kinds of design document

**What changes**: The public documentation currently states without qualification that
Spektacular never adds anything to a design document, which stops being true of the designs it
authors. After this milestone the site explains that Spektacular can help author a design, what
each field on an authored design means, and that a design the project already had carries none of
it, with the existing guarantee narrowed to the documents it did not write rather than left to
contradict the product. This is the mitigation for the one conceptual cost the whole approach
carries, and it is a requirement rather than a courtesy: two classes of document that are not
explained are two classes users get wrong.

**Validation point**: the documentation site explains authoring, the meaning of every metadata
field on an authored design, and the fact that a pre-existing design carries none; no page still
claims Spektacular never adds frontmatter to a design document without qualifying it; the site
builds and type-checks with zero errors and zero warnings; and the project's own design folder
no longer states the unqualified claim either.

#### - [x] Phase 4.1: Explain design authoring on the documentation site

**Repo:** docs

The design documents page gains the explanation of guided authoring and of what an authored
design's record means, and its existing unqualified claim that Spektacular never adds frontmatter
to a design document is narrowed to the documents it did not write. This is a correction before
it is an addition: left as it is, the page would contradict the product two screens apart. The
new material follows the site's existing patterns, with the interview shown the way the repository
registration conversation is already shown, and the record's fields listed as bullets rather than
a table because the body styles carry no table styling.

**Content outline**

- Rework of the existing "What a design document is" section: keep "Spektacular owns the
  reference, not the document", and replace the unqualified guarantee with the two-class
  statement. Illustrative: "A design your team already had is never changed by Spektacular. It
  gains no frontmatter, is never reformatted, and comes back byte for byte as you supplied it. A
  design Spektacular authors with you is different: it carries the same lifecycle record every
  spec and plan carries, so you can see where it stands and which specs depend on it."
- New section, "Working a design out with Spektacular" (shaded, placed after "When to use one
  instead of a spec section"): what the interview is for, that it is the same Flipped Interaction
  pattern used for specs, and that it stops once another answer would not change the document.
  Illustrative transcript in a bare fenced block, in the shape used for repository registration:

  ```
  Agent:  You have described the shape of the retry policy twice now, and it is
          settled. Shall I help you write it up as a design document?
  You:    yes
  Agent:  What triggers a retry: any failure, or only specific ones?
  ...
  Agent:  That is enough to write it. Nothing further would change the document.
  ```
- New section, "What an authored design records" (plain): the four facts, as bullets.
  Illustrative: "- **Capture date**: when the design was written. - **Document status**: where it
  stands, one of `draft`, `final`, `superseded` or `archived`. - **Originating spec**: the spec
  whose conversation produced it, when there was one. - **Referencing specs**: every spec that
  references it, kept in step automatically."
  Followed by a fenced `yaml` example of an authored design's block, and one sentence stating a
  pre-existing design has no such block at all.
- Extension of the existing command reference: a fenced `bash` block for the authored write
  alongside the existing verbs, and one sentence on when to use each.
- Extension of the existing "When a reference cannot be found" section: one paragraph on a
  reference operation failing as a whole so a spec and a design never disagree.

*Technical detail:* [context.md#phase-41](./context.md#phase-41-explain-design-authoring-on-the-documentation-site)

**Acceptance criteria**:

- [x] The page explains that Spektacular can help author a design through a guided conversation,
      and that the conversation stops rather than running on.
- [x] The page explains what every field on an authored design's record means.
- [x] The page states plainly that a design the project already had carries none of that record.
- [x] No claim on the page says Spektacular never adds frontmatter to a design document without
      saying which documents it means.
- [x] Section background shading still alternates correctly from the top of the page to the
      bottom.
- [x] The page uses no layout markup in its body, and no em dashes appear in the new prose.
- [x] The site builds and type-checks with zero errors and zero warnings.

#### - [x] Phase 4.2: Narrow the guarantee everywhere else it is stated

**Repo:** docs, spektacular

The same unqualified guarantee appears in two other places: the configuration page's description
of design sources on the site, and the README that sits in this project's own design folder. Both
are corrected to say the same thing as the main page, so a reader does not find the old claim in
whichever place they happen to look first. This is a small phase deliberately kept separate,
because it spans two repositories and is the one most easily forgotten once the main page reads
correctly.

*Technical detail:* [context.md#phase-42](./context.md#phase-42-narrow-the-guarantee-everywhere-else-it-is-stated)

**Acceptance criteria**:

- [x] The configuration page's design section agrees with the main page about what Spektacular
      does and does not add to a design document.
- [x] The project's own design folder README no longer states the unqualified claim, and mentions
      that authored designs carry a record.
- [x] Searching the documentation site for the old unqualified wording finds nothing.
- [x] The site still builds and type-checks with zero errors and zero warnings.

## Open Questions

Two items genuinely cannot be settled before implementation begins. Everything else surfaced
during planning has been decided and recorded rather than parked here.

- **Whether a write failure can be provoked through file permissions on this platform.**
  Phase 2.2 must prove that a reference operation whose back-link write fails leaves the spec
  exactly as it was, and that requires making a write fail on purpose. The preferred route needs
  no production change: make the design document, or its parent directory, unwritable inside the
  test's own temporary directory. Whether the process actually observes that permission depends
  on how the suite is run, and a run as root would not. **Depends on**: exercising it once in the
  real test environment, which cannot be done meaningfully before the code under test exists.
  **What the implementer does**: try the filesystem route first. If it does not reliably produce
  a failure, fall back to the package-level factory variable described in that phase's technical
  detail, which has a direct precedent in this codebase. Do not skip the test and do not stop to
  ask: both routes are pre-approved, and the choice between them is exactly the kind of thing
  that only exercising it can decide.

- **Whether the documentation repository's working tree is still in the state this plan was
  written against.** The design documents page, the navigation entry and the configuration page
  edits were all uncommitted when this plan was made. **Depends on** what happens to that tree
  between now and the documentation milestone, which is outside this plan's control. **What the
  implementer does**: run `git status` in that repository before editing anything, and re-read the
  page if the work has since been committed, rebased or discarded, because every line reference in
  the documentation phases moves with it. If the page is gone entirely rather than merely moved,
  **STOP and ask the user** rather than recreating it: that would mean 000054's documentation was
  abandoned, which changes what this milestone is for.

One thing that reads like an open question and is not: whether a design document carrying the
team's own frontmatter should count as one Spektacular authored. It was settled during planning
by probing the real parser rather than reasoning about it, and the answer is recorded both in
the phase detail and in the assumption log. The implementer should not re-litigate it, but should
**STOP and ask** if the probe's result turns out not to hold, since the whole authored versus
pre-existing distinction rests on it.

## Out of Scope

Carried from the spec's non-goals:

- **A fifth interactive workflow for designs.** No state machine, no step templates, no
  end-to-end suite. The design skill is a static playbook precisely so it can run inside a spec
  conversation, which a workflow could not.
- **Imposing a structure on a design document.** An authored design gains a lifecycle record and
  nothing else. There is no template, no required headings and no section set; the document's
  shape follows the design.
- **Adding a lifecycle record to designs the project already had.** No backfill command, and
  nothing stamps one on write. This plan actively defends the opposite, by refusing a verbatim
  overwrite of an authored document rather than by upgrading a pre-existing one.
- **Working out back-links by scanning specs on demand.** They are stored and maintained. No
  command scans the spec store to report which specs reference a given design.
- **Deleting design documents.** Authoring, revising and referencing only. Removal stays a manual
  file operation, as it is today.
- **Design references on plans or changelog records.** A spec references a design; no other
  artifact class does. The metadata field that carries references is left as it is for every
  artifact other than a design.
- **Storage backends other than local files.** Git checkouts, remote URLs and issue trackers
  remain later work. The design source set still refuses an unimplemented provider by name.
- **Design sources declared by a registered repository.** They stay a project-level declaration.
  Nothing in this plan touches repository settings.
- **Keeping a shipped design in step with the code.** A design can be moved to a superseded
  status by hand, but nothing detects drift, compares a design against an implementation, or
  requires the two to agree.
- **Indexing design content in knowledge search.** Designs still get no ranking, tags or
  categories, and the knowledge commands are untouched.

Deliberately left out by the chosen design, and worth naming because a reader will look for them:

- **Designs are not added to the cross-kind artifact listing or its shared filter.** Authored
  designs now carry the same lifecycle record that listing exists to query, so it is a reasonable
  place to look, and it would be a small change. It is excluded because no requirement asks for
  it, the spec's non-goals rule out indexing designs, and that scan is built for stores inside the
  project while a design source may point anywhere on disk. An authored design's status and
  provenance are reported by the design listing instead. This is separable if it is wanted later.
- **No command moves a design's lifecycle status on its own.** A status is set when the document
  is authored or re-authored. There is no equivalent of the status-setting subcommand the spec,
  plan and changelog stores have, because nothing in the spec asks for one and re-authoring
  already carries a status through. Adding one later is a small, self-contained follow-up.
- **The interview's quality is not measured.** Four of the spec's six success metrics are
  classified as manual for this reason. Nothing counts how many designs were produced by
  interview, how long they are, or whether users were confused by the two classes; those are
  observed after delivery, and the implement workflow captures them in its test plan.
- **No migration or backfill for existing projects beyond re-running the installer.** Upgrading
  installs the new skill and the rewritten instruction. It does not touch any design document
  already in a declared source, by design.

Nothing was deferred at the user's request during planning; every exclusion above is either
carried from the spec or a consequence of the chosen design.

## Changelog

### 2026-09-20 — Phase 1.1: Record which specs reference a design

**What was done**: Added `Specs []string` to the artifact metadata block, the back-link list
naming every spec that references a document. It is the exact reverse of the `Designs` list a
spec already carries. The field was added in lockstep across all six sites the closed schema
requires, together with a matching `UpdateOptions.Specs *[]string` carrying the same tri-state
pointer semantics as `Designs`, so back-links are preserved across a body-only rewrite, replaced
by a non-nil list and cleared by a non-nil empty one.

**Deviations**: None. Every site named in the phase's technical detail was changed exactly as
described, and the byte-exact guard `TestRender_OmitsDesignsKeyEntirelyWithNoReferences` was left
completely unmodified rather than updated.

**Files changed**:
- `spektacular: internal/metadata/metadata.go`
- `spektacular: internal/metadata/merge.go`
- `spektacular: internal/metadata/metadata_test.go`
- `spektacular: internal/metadata/merge_test.go`

**Discoveries**: The lenient decoder for `specs` needs a case the `designs` decoder does not: a
sequence whose entries are non-scalar. `decodeDesignRefs` drops a bad entry naturally, because
`item.Decode(&ref)` into a struct fails on a scalar and the entry is skipped; a `[]string` has no
such struct to fail against, so `decodeSpecNames` must check `item.Kind != yaml.ScalarNode`
explicitly or a mapping entry would decode into a garbage name. The test table for `specs`
therefore carries a `list of non-scalar` case with no counterpart in the designs original. Phase
2.1 is the first caller that will pass a non-nil `opts.Specs`; everything shipped here is dormant
until then, which is why the whole-suite verification passing unchanged is the meaningful signal
rather than a weak one.

### 2026-09-20 — Phase 1.2: Write a design document Spektacular authored

**What was done**: Added the `design author` command, a sibling to `design write` that stores a
staged document into a declared source with Spektacular's lifecycle block merged in. It takes the
same address as every other design verb, reads its content from `--from`, and optionally takes
`--document-status` and `--spec`. Rewriting an existing authored design preserves its capture date
and its back-link list, which is what makes a revision an update in place rather than a
replacement.

**Deviations**: One wording change beyond the plan. The plan asked for `metadata.Merge`'s failure
on a document carrying the team's own frontmatter to be wrapped in an actionable refusal, which it
is, as `design_frontmatter_not_authored`. The raw underlying cause is deliberately not repeated in
the message: it reads as `parsing created_date "": ...`, which invites an agent to add a
`created_date` to the team's block rather than leave the block alone, the opposite of the intended
remediation.

**Files changed**:
- `spektacular: cmd/design.go`
- `spektacular: cmd/design_test.go`

**Discoveries**: Two worth carrying into the phases that follow. First, `design author` is the
only write path in the feature with no clock seam, so its tests take today's date from the
in-package `today()` helper rather than a literal; phase 2.1's back-link writes go through the same
command surface and will need the same treatment. Second, and more consequential for phase 1.3:
`set.Exists` followed by `set.Read` is the right way to tell a first write from a rewrite, because
distinguishing them by matching on `design_not_found`'s error string would couple the command to
the wording of a refusal. Phase 1.3's `authoredMetadata` helper has the same choice to make and
should resolve it the same way.

### 2026-09-20 — Phase 1.3: Keep the two kinds of design document apart

**What was done**: Added the shared discriminator that tells a design Spektacular authored from
one the project already had, and used it in the two places that now have to distinguish them. The
verbatim write refuses to overwrite an authored document, pointing at the authored write instead,
and the listing reports an authored design's status, capture date, originating spec and
referencing specs while reporting nothing but a source and a path for a document with no record.

**Deviations**: One wording change beyond the plan. The refusal's next action embeds a runnable
`design author` command whose `--data` payload is left unquoted inside the quoted command, because
the obvious phrasing nests single quotes inside single quotes and is not copy-pasteable in a
shell.

**Files changed**:
- `spektacular: cmd/design.go`
- `spektacular: cmd/design_test.go`

**Discoveries**: The `authoredMetadata` helper is the single point the whole authored-versus
pre-existing distinction rests on, and its swallowed error case is the load-bearing half. A
document carrying the team's own frontmatter makes `metadata.Split` return `(nil, error)`, not
`(nil, nil)`, because `UnmarshalYAML` parses `created_date` strictly; treating that error as
anything but "not authored" would make listing and referencing fail outright on an ordinary design
file with a YAML header. The test pinning this is the verbatim write over a team-frontmatter
document succeeding byte-exactly, and it was mutation-checked: making the error case report
"authored" fails it. Phase 2.1 reuses this helper rather than writing the check again.

A second, smaller note for phase 2.1: `design list` now reads every document it lists, where
before it only enumerated paths. A document that cannot be read is still listed, with no lifecycle
fields, rather than failing the listing.

### 2026-09-20 — Phase 2.1: Keep a design and its referencing specs in agreement

**What was done**: Recording a reference is now a change to both documents rather than one. A
shared helper writes the spec first and the design's back-link second, and both reference verbs go
through it so the ordering cannot drift apart between them. A design the project already had takes
part without gaining anything: the reference still goes on the spec and the design file is not
touched, which is equally true of a reference recorded before its document exists.

**Deviations**: None.

**Files changed**:
- `spektacular: cmd/design_ref.go`
- `spektacular: cmd/design_ref_test.go`

**Discoveries**: Two that matter for phase 2.2. First, the spec name stored in a back-link is the
bare form, normalised after `specStore` has appended `.md`, because that is what `spec file list`
reports and what keeps a `specs:` list stable however a caller spelled the name; a dedicated
`bareSpecName` exists for this and the bare-versus-suffixed test bites on it. Second, and
structurally: the design-side write is isolated in `writeBackLink` and `applyRef` does nothing but
sequence the two writes. That is deliberate, so phase 2.2's compensation wraps `applyRef`'s second
call without touching the list arithmetic, and `refsOf` has already handed back the spec's original
bytes that the compensation needs as its payload.

`runDesignRefRemove` now constructs a design set it did not previously need. Its existing
write-only-if-changed condition was extended to cover both documents rather than just the spec, so
a no-op remove still rewrites neither.

### 2026-09-20 — Phase 2.2: Never leave a spec and a design disagreeing

**What was done**: The two writes in a reference operation now fail as a unit. When the design's
record cannot be updated, the spec's original bytes are written back and the whole operation is
refused, so the pair is never left contradicting each other. When that restoration also fails,
that is reported as its own distinct outcome naming both documents to repair by hand, because the
corrective action is completely different and an agent that cannot tell the two apart will do the
wrong thing.

**Deviations**: One, in the test seam, and it resolves the plan's first Open Question. The plan
asked for the filesystem route first and a package-level `designSetFactory = newDesignSet` only if
that proved unreliable. The answer turned out to be both, for different cases. The filesystem
route does work here, so the back-link-failure case needs no seam at all. The rollback-failure
case cannot be reached that way, and that is a property of the store rather than of the platform:
`store.FileStore.Write` is `os.MkdirAll` then `os.WriteFile`, and writing an existing file needs
permission on the file rather than on its directory, so any permission state that fails the
rollback fails the identical first spec write too and the run never reaches the compensation. The
seam added is therefore `var writeBackLinkFn = writeBackLink` rather than a design-set factory:
the same shape and the same precedent, a smaller surface, and the only one of the two that can
also make the spec unwritable between the two writes, which is what this case requires.

**Files changed**:
- `spektacular: cmd/design_ref.go`
- `spektacular: cmd/design_ref_test.go`

**Discoveries**: The reason the rollback-failure case needs a seam is worth stating plainly,
because it looks like a platform question and is not. It follows from `FileStore.Write`'s
implementation, so it will hold on every platform and would not have been settled by trying
harder with permissions. The back-link-failure case, by contrast, needs nothing: this suite runs
as a normal user and the permission is enforced, and if it were ever run as root those tests
would fail loudly on their exit-code assertion rather than passing vacuously.

### 2026-09-20 — Phase 3.1: Add the design skill

**What was done**: Added the `spek-design` skill, a static playbook rather than a fifth workflow
so it can run inside a spec conversation, which is when design talk actually happens. It covers
the four ways a design enters a project: authoring one that does not exist yet through a guided
interview, bringing in one the user already has, revising one that exists, and recording a
reference to one already stored. Adding it to the two install registries is what makes every
supported agent install it and what delivers it to existing projects on upgrade.

**Deviations**: One, in execution rather than scope. The plan's agent strategy for this phase was
two parallel agents, one drafting the skill prose and one wiring the registries and tests. It was
done in a single pass instead: the template context was already loaded, and splitting it would
have added an integration merge for two registry lines.

**Files changed**:
- `spektacular: templates/skills/workflows/spek-design/SKILL.md` (new)
- `spektacular: internal/agent/skills.go`
- `spektacular: internal/agent/commands.go`
- `spektacular: internal/agent/agent_test.go`
- `spektacular: internal/agent/claude_test.go`
- `spektacular: internal/agent/bob_test.go`
- `spektacular: internal/agent/codex_test.go`
- `spektacular: templates/skill_list_command_test.go`
- `spektacular: cmd/migrate_test.go`

**Discoveries**: Adding a skill breaks more hand-maintained oracles than the registries suggest,
and they fail in two different ways. Three fail loudly at once, because two `fstest.MapFS`
fixtures in `agent_test.go` enumerate the skill templates by hand and a missing entry is a read
error rather than a count mismatch. The rest fail only when exercised: the three per-agent tables,
the wrapper-filename table, and the skill-to-command table. One test name was already stale before
this change, `TestInstallWorkflowSkills_WritesFourSkillFiles` asserting five, so it was renamed to
stop encoding a number that rots on every addition.

The upgrade path turned out to be testable end to end rather than only readable in code, which
the plan had flagged as unverified: `TestMigrate_StaleSkillsOnCurrentProjectOnlyTouchesSkillsVersion`
already drives the real `migrate` command and already asserts no other setting changed, so
asserting the new skill file exists afterwards covers the whole criterion in one line.

The cross-table invariant the plan asked for is now in place and is worth knowing about for the
next skill: nothing previously checked that a `workflowSkills` entry had a `workflowDescriptions`
entry, so a skill added to one table and not the other rendered an empty description into every
non-Claude agent's slash-command menu with nothing failing. The new assertion covers the whole
table in both directions. Note the description path only bites on an agent with no native skill
mechanism, so it is exercised through bob rather than claude.

### 2026-09-20 — Phase 3.2: Rewrite the standing design instruction

**What was done**: The standing rule that tells an agent to watch for design conversation now
separates noticing from qualifying. It opens with an alert that applies during any conversation
working out how something will behave, states plainly that being alert is not the same as
offering, then keeps the existing three-part bar as the gate on offering. It adds the three entry
cases the old section walked past: a design the user already has, one that exists and is being
changed, and design talk with no spec in sight. The accept branch hands off to the `spek-design`
skill by name and records a reference only when a spec exists.

**Deviations**: None.

**Files changed**:
- `spektacular: templates/agents/design-trigger.md`
- `spektacular: internal/agent/design_trigger_test.go`

**Discoveries**: The rewrite passed every pre-existing anchor test unchanged, which is the useful
signal here rather than a lucky one: the pinned literals are all in the outcomes block, and the
restructure happened entirely above it. The `NotContains "skill spek-"` guard in particular stays
green while the section now names the skill, because naming it in prose does not contain that
substring and an instruction to fetch it would. That is exactly the distinction the guard was
built to hold, now exercised for real rather than hypothetically.

One wrapping trap for anyone editing this template later: the accept branch's conditional renders
as `**only` at the end of one line and `if a spec exists**` at the start of the next, so
`"only if a spec exists"` is not a contiguous string in the rendered section and an anchor written
that way would fail for the wrong reason. The test pins a contiguous phrase instead.

### 2026-09-20 — Phase 3.3: Align the spec workflow's own design offer

**What was done**: The two surfaces that carried near-duplicates of the design-capture offer now
say the same thing as the standing instruction. The spec workflow's technical-approach step
states that the offer covers a design settled in conversation but not yet written as well as one
the user already has, and its accept branch hands off to the `spek-design` skill and names the
authored write alongside the verbatim one. The spec skill's command list gains the authored write
and narrows its description of the verbatim one to documents Spektacular did not author.

**Deviations**: None to scope. One defect of my own was caught by an existing test and fixed: the
first draft wrapped `spektacular design write` across a newline, so the rendered command name was
no longer contiguous. The test was right and the template was wrong.

**Files changed**:
- `spektacular: templates/steps/spec/05-technical_approach.md`
- `spektacular: templates/skills/workflows/spek-new/SKILL.md`
- `spektacular: internal/steps/spec/steps_test.go`
- `spektacular: templates/skill_list_command_test.go`
- `spektacular: internal/agent/instruction_surface_test.go`

**Discoveries**: Three worth carrying.

Never let a command name straddle a line break in a template. This bit twice in this milestone,
once as a test failure here and once as an anchor that could not be written contiguously in 3.2.
The rendered command form is what an agent copies, so contiguity is a real property and the tests
that pin it are right to.

The repo-wide guard the plan asked for is narrower than its description suggests, and the test
says so rather than implying otherwise. A forbidden substring can only catch the exact old
sentence, `with no frontmatter added and nothing reformatted`, because the corrected wording
legitimately contains phrases like "adding no frontmatter to it" and "byte for byte"; any pattern
loose enough to catch the general class would fire on the correct text. A narrow guard that is
honest about its reach beats a broad one that is not.

**A loose end for the user, not fixable here.** The dogfooded installed copies under
`.claude/skills/spek-new/SKILL.md` and `.bob/skills/spek-new/SKILL.md` still carry the old
unqualified sentence at line 58, because they are generated artifacts that only refresh on
`init`. The new guard does not fail on them: it walks the embedded templates and renders through
the install path into a temp dir, never the committed copies. Running `go run . init` would bring
them into line, and that is deliberately left to the user, since installing files is an explicit
user-initiated action rather than something this workflow does.

### 2026-09-20 — Phase 4.1: Explain design authoring on the documentation site

**What was done**: The design documents page now explains both kinds of design document. The
paragraph that stated flatly that Spektacular never adds frontmatter to a design document was
reworked into the two-class statement rather than appended to, because left as it was the page
would have contradicted the product two screens apart. Two sections were added, one on working a
design out through the guided interview and one listing what an authored design's record holds,
and the command reference and the reference-failure section were extended.

**Deviations**: None to scope. The plan offered two ways to handle section shading and the
second was taken: rather than flipping every subsequent section's `surface` value, the two new
sections were inserted as a pair, plain then shaded, after a shaded section. That preserves the
whole downstream run untouched, so the change really is local to the lines edited, which the plan
had warned would not be true of the other approach.

**Files changed**:
- `docs: src/pages/design-documents.mdx`

**Discoveries**: Inserting sections in pairs is the general trick for this page, and it is worth
knowing before reaching for the flip-everything-below approach the convention's wording suggests.
An even number of new sections starting with the opposite value of the one above preserves
alternation for the entire rest of the page; an odd number forces a cascade.

The documentation phases have no unit tests and none were invented for them. The plan's testing
strategy makes the build and the type-check the gate, and both were run deliberately:
`npx astro check` is not an npm script and CI never runs it, so it is a local gate that has to be
invoked on purpose. It reports one pre-existing hint about `document.execCommand` in an unrelated
component, which is neither an error nor a warning and is not this work's to fix.

### 2026-09-20 — Phase 4.2: Narrow the guarantee everywhere else it is stated

**What was done**: The same unqualified guarantee appeared in two places outside the main
documentation page, and both now say what the main page says. The configuration page's design
section states that a source can hold both kinds of document side by side, and this project's own
design folder README no longer claims that nothing Spektacular writes adds frontmatter to a file
in there. The README's command list also gained the authored write.

**Deviations**: None.

**Files changed**:
- `docs: src/pages/configuration.mdx`
- `spektacular: .spektacular/design/README.md`

**Discoveries**: One judgement call worth recording, because the rule it turns on is easy to
apply too broadly. `.spektacular/design/` is a declared design source, and files under a store
directory must be written through the CLI rather than with file tools. That README is not a
stored artifact though: `design list` returns an empty list for the source, and the source's own
`.spektacular_ignore` names `README.md`. The project's rule makes the CLI's own listing the
authority on what counts as a stored artifact, precisely because a directory listing can show
entries Spektacular does not consider valid, so editing this file directly is what the rule
requires rather than an exception to it.

The re-run sweep turned up one hit that looks relevant and is not: `knowledge-base.mdx` describes
a malformed block being treated as no frontmatter, which is about knowledge entries rather than
design documents. Left alone deliberately.

### 2026-09-20 — Correction to Phase 2.2: the filesystem route is not CI-safe

**What happened**: CI failed on `TestDesignRef_BackLinkFailureLeavesNoDisagreementBehind`, all
three subtests, at commit 70adf22. The phase 2.2 entry above records that the filesystem route
works for the back-link-write failure and needs no seam. That is true locally and false in CI,
and the entry is corrected here rather than edited, since it was an accurate record of what was
believed at the time.

**Why it failed**: the subtests sabotaged a write with `chmod 0444`. CI runs the suite as root
inside a Dagger container, and root bypasses the permission bits entirely, so every sabotaged
write succeeded and the commands exited 0 where the tests required exit 1. The third subtest
failed the same way one level down: its seam fired, but the `chmod` it applied to the spec did
not stop the compensating write, so the run reported `design_ref_backlink_failed` instead of
`design_ref_backlink_rollback_failed`. Verified directly rather than inferred: under
`unshare -r`, `chmod 0444` followed by a write returns `permission denied` as uid 1000 and `nil`
as uid 0.

The risk was recorded at the time and judged acceptable on the grounds that running as root
would produce a loud failure rather than a silent pass. That reasoning held exactly, and the
failure was loud. The mistake was resolving the plan's first Open Question against the local
environment alone when CI is the environment that decides it.

**The fix**: sabotage by replacing the target file with an empty directory instead of by changing
its mode. `os.ReadFile` and `os.WriteFile` both fail with EISDIR on a directory, and that is a
kind-of-file error rather than a permission check, so no uid is exempt. The design document
becomes a directory for the two back-link-failure subtests, and the seam turns the spec into one
for the rollback-failure subtest. The `writeBackLinkFn` seam is still required for that third
case, for the reason already recorded: the spec has to become unwritable between the two writes.

Verified as uid 1000 and as uid 0 under `unshare -r`: the whole suite passes both ways.

**Deviation from the repo's existing convention, deliberately**: five other tests guard this with
`if os.Geteuid() == 0 { t.Skip("root ignores directory permissions") }`. A skip was rejected here
because it would make CI green by dropping coverage of the one failure mode the spec raises to a
constraint, in the environment where that coverage matters most. The directory technique keeps
the assertion live everywhere and makes the skip unnecessary.

**Files changed**:
- `spektacular: cmd/design_ref_test.go`

**Discoveries**: A chmod-based test sabotage is not a portable way to force an I/O failure in this
project, because CI runs as root. It fails open: the sabotage silently does nothing and the test
either passes vacuously or fails for a confusing reason far from its cause. Replacing the file
with a directory is the root-proof equivalent and costs nothing. Worth reaching for before a root
skip, which trades the coverage away.
