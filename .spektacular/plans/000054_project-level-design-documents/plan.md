---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Plan: 000054_project-level-design-documents

<!-- Metadata -->
<!-- Created: 2026-09-20T09:48:12Z -->
<!-- Commit: c88eecf -->
<!-- Branch: f-migrate -->
<!-- Repository: git@github.com:jumppad-labs/spektacular.git -->

## Overview

Spektacular gains design documents as a project-level artifact class alongside specs, plans
and changelog records. A project declares one or more named design sources pointing at
wherever it already keeps its worked designs, reaches the documents there through the CLI the
same way it reaches every other artifact, and records references to them on the specs they
bind, so a spec can point at a settled API shape or UX flow instead of absorbing it. Teams get
specs that stay readable while still binding implementation to the design that was agreed,
design detail raised in conversation is captured rather than lost, and a broken reference is
reported while planning rather than discovered halfway through implementation.

## Conventions

- **Error messages must describe the problem and suggest remediation (`spektacular`)** — this feature's headline failure modes are all errors an agent must recover from: an unknown design source, an unresolvable reference, a source whose location is not a directory, and a write to a read-only source. Each is built with `output.NewError(code, message).WithNextAction(...)` naming a runnable next step (the declared source names, the exact path searched, the command that lists sources), never a bare `fmt.Errorf`.
- **Tests must not depend on execution order (`spektacular`)** — the feature adds a new `design` cobra command tree with its own flags, and `make test` runs `go test -shuffle=on ./...`. Every new `cmd` test goes through `resetRootCmd`/`runRootCmd` rather than a per-command flag reset, so the new flags cannot leak into whichever test runs next.
- **Passing tests are required before calling work done (`spektacular`)** — `go test ./...` must be green at every phase boundary, and the baseline was confirmed green before this plan started, so any failure is attributable to this work.
- **MDX authoring conventions (`docs`)** — the new concept page and the `configuration.mdx` additions are authored with named-block components and native MDX slot content only: no `<div>`, `<section>` or `class=` in a page body, a blank line either side of every slot body, and fenced markdown code blocks rather than string props.
- **Site layout conventions (`docs`)** — the new page is composed from existing `sections/` components at the single frame width and heading scale rather than introducing new markup, so it looks like the rest of the site with no per-page tuning.
- **Alternate section background shading (`docs`)** — the new concept page's `Section` bands alternate `surface` against the value of the preceding section, and the `configuration.mdx` insertion must not break the existing alternation around it.
- **No em dashes (`docs`)** — binding on every word written into the docs repo: the concept page, the configuration reference additions, and the changelog and commit text for that repo.
- **Label before filename in file-scoped reference headings (`docs`)** — the `configuration.mdx` additions sit inside headings that already follow this form ("Project configuration: config.yaml"), and any new heading added there must keep it.
- **Plans must sketch content structure, not just summarize it (`docs`)** — the phase that creates the concept page carries a `**Content outline**` block giving its headings in order and an illustrative example per section, with the config keys and command names fixed by this plan's research rather than left for the implementer to invent.
- Deliberately dropped: the glossary entries in both repos are the category-`README.md` scaffolds with no project terms defined yet, so there is no vocabulary to honour beyond what this plan defines itself.

## Architecture & Design Decisions

Design documents become a fourth artifact class that Spektacular **addresses but does not
own**. The project declares any number of named design sources in `config.yaml` under a new
optional `design.sources` key, reusing the `SourceConfig` type that already backs
`knowledge.sources`: a `name` the source is addressed by, a `provider`, and a
provider-specific `config.location`. Relative locations resolve from the folder holding
`config.yaml`, the same base every other path in that file uses. A new `internal/design`
package turns that declaration into live stores with a literal provider switch that fails
fast on anything but `file`, and a thin `spektacular design` command family
(`sources`, `list`, `read`, `write`) exposes them with `--data '{"source":…,"path":…}'`
addressing and `--schema` introspection, exactly as `knowledge read`/`knowledge write` are
reached today. Because design sources are project tier only, the address is a source name
and a path, with no tier: the two-tier `Tier`/`Selector` machinery knowledge needs is
deliberately left out.

Two decisions shape the command surface and are the non-obvious ones. First, the design
commands are **not** another registration of the shared `newStoreFileCmd` factory that backs
`spec file`, `plan file` and `changelog file`, even though the verbs match. That factory
stamps and re-reads Spektacular's `created_date`/`document_status`/`closed_date` frontmatter
on every write and list, and this spec forbids imposing structure on a user's design
document or rewriting it. `design write` therefore writes bytes through unchanged and
`design list` reports name and path only. Second, the configuration is sources-shaped rather
than store-shaped for a mechanical reason as well as an expressive one: a store directory
that resolves outside the project root is refused outright, so the store shape could never
satisfy the requirement that a team point at a folder of designs it already has, wherever it
already lives. Knowledge-source resolution has no such refusal, which is why it is the right
precedent. The new key is additive and optional, so the settings format version does not
rise and no migration step is registered; absence simply means the project declares no
design sources.

A spec records its references in its own YAML frontmatter, as a `designs` list of
`{source, path}` pairs, extended into the existing `internal/metadata` schema and preserved
by its merge step. This is the only shape that survives the way a spec is actually committed:
the frontmatter schema is closed, so any key not modelled in Go is dropped the next time the
block is rendered, and the spec workflow commits by writing a freshly assembled body over the
stored file. References are recorded through `design ref add`, never by an agent editing
prose, so a reference naming a source the project has not declared is refused in Go with an
error naming the unknown source and listing the declared ones, and nothing is written. That
also keeps the spec body free of the design's content, which was the original complaint this
feature answers. Any spec may reference any design, and a design may be referenced by any
number of specs: the reference is a pointer in one direction only, and a design document
holds no back-links, so it outlives the feature that introduced it without any bookkeeping.

Capture and consumption are prose, not code, because that is where Spektacular's workflow
judgement lives. A new managed `AGENTS.md` section gives every agent the standing trigger for
recognising a settled API shape, user-facing flow, data format or worked example, with the
established accept / defer / decline outcomes, and matching in-step prose in the spec
workflow's Technical Approach step turns the offer into something that actually fires at the
moment design detail is being compressed away. Nothing is ever written without explicit
agreement. On the consuming side, the plan workflow is obliged to resolve and read every
referenced design before it designs anything, and to name each one with the source it came
from in the plan's Dependencies. An unresolvable reference is a loud failure rather than a
silent absence: `design ref list` reports each reference's resolution and what was searched,
and `design read` on a missing document fails with the source, the path and the absolute
location it looked in, so a broken reference is caught while planning instead of during
implementation. Room is left for a future read-only remote provider by splitting a narrow
`store.Reader` out of `store.Store` and letting a design source carry an optional writer, so
a provider that cannot write is a clear refusal rather than a violated contract; nothing
beyond that is built now. The rejected options, including a fifth workflow FSM, a
store-shaped configuration and body-prose references, are recorded with citations in
`research.md#alternatives-considered-and-rejected`.

## Component Breakdown

**Design source declaration (changed: project configuration).** The project configuration
type gains an optional `Design` section holding an ordered list of named sources, reusing the
same source type that already backs the project's shared knowledge stores: a name the source
is addressed by, a provider, and a provider-specific location. It owns validation of that
list, refusing an entry with no name, a duplicate name within the list, an unsupported
provider, or an empty location, and it owns the rule that a relative location resolves from
the folder holding the project settings file. It carries no knowledge of stores or documents;
it is a declaration that the design set consumes. Absence of the section is valid and means
the project declares no design sources, so the settings format version is unchanged.

**Design set (new: `internal/design`).** The domain component, and the only place that knows
how a declared source becomes something readable. It owns resolving each declared location to
an absolute path, refusing fast when a location is not a directory (naming the source, the
path it resolved to, and the base it resolved from), dispatching on the provider with a
literal switch that fails on anything it does not implement, and constructing the backing
store for each source. It exposes a narrow projection to callers: enumerate the sources,
list a source's documents, read one addressed document, and write one addressed document. It
is the single owner of the "unknown source name" refusal, which names the declared sources so
a caller can correct itself. It deliberately holds no ranking, no tiers, no categories and no
de-duplication: those belong to the knowledge set, which this component parallels but does not
share code with beyond the store layer.

**Storage reader interface (changed: `internal/store`).** The existing store interface is
split so that its read half stands alone: a reader that can read, list, test existence and
search, with the full store adding write, delete and root on top. It owns nothing new
behaviourally today, since the file store satisfies both halves unchanged and no existing
caller's type changes. Its purpose is structural: it is what lets a design source hold a
reader unconditionally and a writer only when its provider can supply one, so a future
read-only remote provider is a source that refuses writes by name rather than a store that
violates a documented contract.

**Design command family (new: `cmd`).** The agent- and user-facing surface. It owns argument
and `--data` parsing, schema introspection, and turning design-set results and refusals into
the project's standard JSON envelope. It is deliberately thin: every rule it enforces belongs
to the design set or the metadata component, and it adds only the presentation. It sits
beside, and explicitly does not reuse, the shared store-file command factory that backs the
spec, plan and changelog file commands, because that factory stamps and re-reads Spektacular's
artifact frontmatter on every write and list and design documents must be left untouched.

**Design reference recorder (new: `cmd`, over the metadata component).** The part of the
command family that reads and mutates a spec's recorded references. It owns the validation
that a reference names a declared source before anything is written, the refusal when it does
not, and the resolution report that says, for each of a spec's references, whether the
document was found and where it was looked for. It is the component the plan workflow calls
first and the spec workflow calls when a capture is accepted. It never edits document prose;
it only rewrites the spec's metadata block through the metadata component.

**Artifact metadata schema (changed: `internal/metadata`).** The existing owner of the
frontmatter block gains one more field: the list of design references a spec carries. It owns
parsing that list leniently on read, rendering it on write, and preserving it across the
body-only rewrites the spec workflow performs when it commits an assembled spec. This is the
component that makes references durable, because the frontmatter schema is closed and a field
it does not model is dropped the next time the block is rendered. Its existing lifecycle
fields and their transition rules are untouched.

**Agent standing instruction (new: a managed `AGENTS.md` section, installed by
`internal/agent`).** A new managed section alongside the existing memory, knowledge-trigger,
spec-trigger, draft-presentation and historical-artifacts sections. It owns the standing
recognition rule: what counts as design-level detail worth capturing, that the agent offers
rather than writes, and the accept, defer and decline outcomes. It is installed idempotently
for every supported agent by the same mechanism the existing sections use, and it names the
design commands rather than a skill, so nothing depends on skill lookup.

**Spec workflow prose (changed: `templates/steps/spec`).** The Technical Approach step gains
the in-step half of the capture offer, placed where the step already tells the agent to
compress worked design down to a one-line steer. It owns turning the standing rule into an
offer that actually fires at the moment design detail would otherwise be lost, and, on
acceptance, directing the write and the reference recording. It adds no new step and no new
interruption point.

**Plan workflow prose (changed: `templates/steps/plan`).** Two steps change. The discovery
step gains the obligation to resolve and read every design the spec references before any
design work begins, and to stop and report rather than continue when one cannot be found. The
dependencies step gains the obligation to name each design read and the source it came from.
The architecture step's existing instruction to choose a direction is qualified so that a
referenced design is built on rather than re-derived. No new step, and no new heading in the
plan scaffold.

**Documentation site (changed: `docs` repo).** A new concept page owns the explanation of what
a design document is, when to use one instead of putting detail in a spec, how it relates to
specs and plans, and how the commands are used. The existing configuration reference gains the
new settings key, documented in the same shape as the project's shared knowledge stores and
linking out to the concept page rather than restating it, and the site navigation gains one
entry. The repository README's configuration section gains the matching key so the two stay in
step.

## Data Structures & Interfaces

**Project settings: the `design` section.** A new optional top-level key, declared with the
same source type the project's shared knowledge stores already use, so nothing new is
invented for it:

```yaml
design:
  sources:
    - name: api            # the name this source is addressed by, unique in the list
      provider: file       # only `file` ships today
      config:
        location: ../design/api   # relative to the folder holding config.yaml
    - name: ux
      provider: file
      config:
        location: ${HOME}/work/ux-designs
```

```go
// config
type DesignConfig struct {
    Sources []SourceConfig `yaml:"sources,omitempty"`
}
func (c DesignConfig) Validate() error

type Config struct {
    // ...existing fields...
    Design DesignConfig `yaml:"design,omitempty"`
}
```

`SourceConfig` and its file-provider config are reused unchanged. The `Tier` field on
`SourceConfig` stays unset for design sources: design is project tier only, so a source's
identity is its name alone and there is no tier to stamp.

**Storage: splitting the read half out of the store interface.** The existing interface keeps
every method it has; the change is purely that its read half is nameable:

```go
// store
type Reader interface {
    Read(path string) ([]byte, error)
    List(path string) ([]DirEntry, error)
    Exists(path string) bool
    Search(terms []string, opts SearchOptions) ([]Hit, error)
}

type Writer interface {
    Write(path string, content []byte) error
    Delete(path string) error
}

type Store interface {
    Reader
    Writer
    Root() string
}
```

No existing implementation or call site changes type: the file store and its ignore-aware
wrapper satisfy all three. What this buys is the ability for a source to hold a reader it
always has and a writer it may not.

**Design set: the domain contract.** The projection the command layer consumes. A set is
built once from the project settings and the project root, and everything else is addressed
through it:

```go
// design
type Source struct {
    Name     string
    Provider string
    Location string      // absolute, already resolved
}

type Document struct {
    Source string
    Path   string        // relative to the source's location
}

type Set struct{ /* one resolved source per declaration, in declaration order */ }

func NewSet(cfg config.Config, projectRoot string) (*Set, error)

func (s *Set) Sources() []Source
func (s *Set) List(sourceName string) ([]Document, error)   // "" lists every source
func (s *Set) Read(d Document) ([]byte, error)
func (s *Set) Write(d Document, content []byte) error
func (s *Set) Resolve(d Document) (absolutePath string, err error)
```

`NewSet` is where an undeclared provider and an unreachable location are refused. Every
method that takes a `Document` refuses an unknown source name before touching a store, and
`Read` distinguishes "no such source" from "no such document in that source" so the two
produce different errors. `Resolve` exists so a caller can report the absolute location that
was searched without reading the file.

**Design references: the serialization boundary on a spec.** A reference is recorded in the
referencing spec's frontmatter, in the artifact metadata block that already carries the
document lifecycle:

```yaml
---
created_date: "2026-09-20"
document_status: final
designs:
  - source: api
    path: payments/v2.md
  - source: ux
    path: checkout-flow.md
---
```

```go
// metadata
type DesignRef struct {
    Source string `yaml:"source"`
    Path   string `yaml:"path"`
}

type Metadata struct {
    // ...existing fields...
    Designs []DesignRef
}
```

The field is added to the metadata type and to both halves of its on-disk shape, and the merge
step preserves it across a body-only rewrite exactly as it preserves the created date. Reads
are lenient in keeping with the rest of the block: a malformed or absent list reads as no
references rather than failing the parse. Order is the order recorded, and a duplicate
`{source, path}` pair is a no-op rather than a second entry.

**Command input and output contracts.** Every design command that addresses something takes a
JSON payload and publishes its shape through the existing schema flag, matching how the
knowledge commands are reached:

| Command | Input | Output |
|---|---|---|
| `design sources` | none | `sources[]` of `{name, provider, location}` |
| `design list` | none, with `--source` to narrow | `documents[]` of `{source, path}` |
| `design read` | `{"source","path"}` | the document bytes on stdout |
| `design write` | `{"source","path"}` with `--from <file>` | `{source, path, location}` |
| `design ref add` | `{"spec","source","path"}` | `{spec, designs[]}` after the write |
| `design ref remove` | `{"spec","source","path"}` | `{spec, designs[]}` after the write |
| `design ref list` | `{"spec"}` | `{spec, refs[], unresolved, next_action?}` |

A `refs[]` entry is `{source, path, resolved, location}`, where `location` is the absolute
path that was searched whether or not the document was found. `design ref list` always
returns a success envelope, so a caller sees every reference at once; `unresolved` is the
count that could not be found and `next_action` is present only when that count is non-zero.
`design read` is the hard failure: a missing document returns the standard error envelope with
a code, a message naming the source and path, and a next action naming the absolute location
searched and the command that lists what the source does hold.

**Refusals.** Each new failure is a distinct code in the project's standard error envelope,
each carrying a concrete next action: an address naming a source the project has not declared;
an address missing its source or its path; a declared location that does not resolve to a
directory; a declared provider the build does not implement; a write to a source whose
provider cannot write; and a document that does not exist in the source that was named.

## Implementation Detail

**A second sources-backed domain package, deliberately parallel rather than shared.** The
knowledge subsystem is the only existing thing in the codebase shaped like this: a list of
named sources declared in settings, resolved to absolute locations, each backed by a store,
addressed by name plus path. The design set follows the same four beats in the same order
(declare, validate, resolve, dispatch on provider) so a developer who has read one can read
the other without relearning anything. What it does not do is share an implementation with
it. Knowledge carries tiers, a fixed category registry, always-applied versus looked-up
retrieval, tag vocabularies, ranking and de-duplication, none of which design documents have
or are allowed to have. Factoring a common "source set" abstraction out of the two would mean
carrying that machinery into a package that must not have it, or hollowing out the knowledge
package to fit. The shared layer is the store, which both build on, and that is where the
reuse correctly stops. A reviewer should expect two small, similar packages rather than one
generic one, and should read that as the intended shape.

**A deliberate divergence from the shared artifact-file command factory.** Spec, plan and
changelog file commands are all one factory call apart, differing only in which settings
directory they read and two booleans. A reader will reasonably expect design documents to be
a third boolean. They are not, and the reason is worth stating plainly in the code: that
factory stamps Spektacular's lifecycle frontmatter onto every document it writes and re-reads
it on every listing, which is exactly what this feature must not do to a document the team
already owns. The design commands are therefore hand-written against the design set. They
match the factory's verbs and output conventions so the surface still feels like one CLI, but
they share no code with it, and the divergence is signposted where a maintainer would
otherwise try to consolidate them.

**Interface segregation as the whole of the read-only-provider answer.** Naming the read half
of the store interface is a pure refactor: no implementation changes, no call site changes its
type, and the behaviour of every existing command is identical afterwards. The one new
behaviour it enables is that a design source can hold a writer that is absent, which turns
"this provider cannot write" from an undefined outcome into a named refusal with a next
action. That refusal is reachable and testable today with a stub source even though every
shipping provider can write. This is the entire mechanism the plan introduces for the future
read-only remote provider named in the spec's risk note; no provider registry, no capability
negotiation, no speculative remote code. The pattern to recognise is the existing one from the
store abstraction's own history: keep the interface narrow and add surface only when a
concrete backend needs it.

**Frontmatter grows a structured field for the first time.** Until now the artifact metadata
block has been flat: dates and strings. Design references make it carry a list of small
records. The shape of that change is unremarkable, but two properties of the existing code
govern it and a reader should know them. The block is a closed schema, so it is not possible
to record a reference "alongside" the metadata and have it survive; it has to be modelled.
And reads across that block are deliberately lenient while writes are strict, so a spec whose
reference list is absent, empty or malformed must read as carrying no references rather than
failing to parse, while an attempt to record a bad reference is refused. The merge step that
preserves lifecycle fields across body-only rewrites is the single place this preservation
belongs; adding it anywhere else would create a second write path that silently drops
references.

**Behaviour that is prose, tested as prose.** Both halves of the workflow change, the capture
offer and the planning obligation, are template text rather than Go. That is the established
shape here: workflow judgement lives in step instructions, and its regression tests are
assertions that rendered instructions contain, or do not contain, specific anchor phrases.
The new managed agent section follows the existing managed-section mechanism exactly, which
brings with it a known battery of expectations a reviewer should look for: it must create the
section when absent, replace it in place when present, survive installation for a second
agent without duplicating, leave surrounding content untouched, and pick up a template edit.
The capture offer's own wording is aligned word for word with the existing knowledge-capture
offer, because agents encountering two different vocabularies for the same accept, defer and
decline mechanic is a known failure mode here rather than a style preference.

**The failure paths are the product.** Four of this feature's acceptance criteria are about
what happens when something is wrong: an undeclared source, a missing document, a source
location that is not there, and a provider that cannot write. Each is a distinct code with a
message naming what was looked for and a next action naming a command that fixes it. The
codebase convention is one short code per failure site rather than a shared enum, and the
reason it exists is concrete: a driving agent that receives a vague refusal tends to abandon
the CLI and reach for raw file tools. A reviewer should treat a new refusal without a concrete
next action as a defect, not a nit.

**Cross-repo shape.** Everything above lands in the CLI repository. The documentation
repository receives no code: one new concept page built from the site's existing section
components, one navigation entry, and an addition to the configuration reference that
documents the new settings key in the same form as the existing shared-stores key and links
out to the concept page rather than restating it. The split between the two follows the site's
own established division, where a concept and its command usage live on the concept page and
the configuration reference carries only the settings keys.

## Dependencies

**Design documents this plan was built on**: none. The spec this plan implements carries no
design references, because the feature that makes them recordable is the one being planned
here. Every later plan for a spec that does carry references must list each design document
read and the source it came from in this section.

**Internal packages**

- **`internal/config`** — provides the project settings type, its validation, and the rule
  that relative locations resolve from the folder holding the settings file. **Changes**: gains
  an optional design section holding a list of named sources, reusing the existing source type,
  plus its validation and the default case where the section is absent.
- **`internal/store`** — provides the read/write/list/search surface every artifact store is
  built on, its file implementation, and the ignore-file wrapper that filters listings and
  searches. **Changes**: the read half of the interface is named separately so a source can
  hold a reader unconditionally and a writer conditionally. No implementation changes and no
  existing call site changes type.
- **`internal/metadata`** — provides the artifact frontmatter schema, its parse, render and
  merge behaviour, and the guarantee that lifecycle fields survive a body-only rewrite.
  **Changes**: gains the design-reference list as a modelled field on both halves of the
  on-disk shape and in the merge step. This is a hard dependency: without it, a reference does
  not survive the spec workflow's own commit.
- **`internal/output`** — provides the JSON envelope, the error type and the next-action
  builder every refusal is constructed with. **No changes**; used as-is by every new failure
  path.
- **`internal/agent`** — provides the managed-section installer that writes each standing rule
  into the agent instruction file idempotently, and the per-agent install sequences.
  **Changes**: one new managed section registered and installed alongside the existing five,
  for each supported agent.
- **`internal/knowledge`** — provides no code to this feature, but its source aggregation,
  unreachable-store refusal and duplicate-name refusal are the working reference the design set
  is modelled on. **No changes**; explicitly not refactored into a shared abstraction.
- **`internal/project`** — provides project initialisation and footprint repair. **No
  changes**: design source locations are folders the team already has, so nothing is scaffolded
  for them and a missing location is reported rather than created.
- **`cmd`** — provides the command tree, settings loading, the shared list filters and the
  test harness that resets the tree between runs. **Changes**: a new design command family and
  its reference verbs, registered into the existing tree.
- **`templates`** — provides the embedded step instructions, scaffolds and agent sections, and
  the contract tests that assert on their wording. **Changes**: one new agent section; prose
  added to one spec step and three plan steps. No scaffold headings change.

**External libraries**

- **`gopkg.in/yaml.v3`** — already the settings and frontmatter codec; the new settings section
  and the new frontmatter field are ordinary additions to types it already marshals. No version
  change.
- **`github.com/spf13/cobra`** — already the command tree; the design family is ordinary
  registration. No version change.
- **`github.com/stretchr/testify`** — already the assertion library for every Go test. No
  version change.
- **`github.com/sabhiram/go-gitignore`** — already backs the ignore-file wrapper, which design
  sources inherit by constructing their stores the standard way. No version change and no
  direct use.
- **Documentation site toolchain (Astro, MDX, Tailwind, expressive-code)** — already in place
  in the documentation repository; the new page and the configuration-reference addition use
  existing components only. No new packages.

**Planning dependencies**

- **Spec `000054_project-level-design-documents`** — the upstream spec this plan implements.
  Already final.
- **Plan `000053_config-schema-versioning-and-migrations`** — its settings format machinery is
  already merged and is what establishes that an additive optional key needs no format bump.
  Nothing must land first; this plan relies on its rule, not on further work.
- **Uncommitted work in the documentation repository** — roughly a hundred and thirty lines are
  currently uncommitted in the configuration reference from the settings-versioning feature,
  including a paragraph left unfinished mid-edit in the knowledge-base page. This plan's
  documentation phase anchors its insertions on component boundaries rather than positions, so
  it does not require that work to land first, but the implementer must not build on or tidy
  the unfinished paragraph.

**Manual verification dependencies**

- **The end-to-end suites** require the external harness runner, a container runtime and agent
  credentials, and do not run in continuous integration. They are a dependency of this plan's
  verification, not of its implementation, and the plan treats their hand-maintained
  expectations as surfaces this change must update rather than as tests that will catch the
  drift on their own.

## Testing Approach

Testing follows this project's three established layers, and the split between them is
decided by what kind of thing is being guaranteed. Deterministic mechanics, meaning settings
parsing and validation, source resolution, the addressing and refusal paths, and the
frontmatter round trip, are Go unit tests. Behaviour that is expressed as instruction prose,
meaning the capture offer and the planning obligation, is covered by contract tests that
assert the rendered instructions contain the phrases that carry the behaviour and do not
contain the shapes that were ruled out. Behaviour that only emerges when a real agent drives
a whole workflow is covered by the end-to-end suites, which are run by hand rather than in
continuous integration.

**Where coverage concentrates.** The heaviest coverage goes to the two components that own
refusals: the design set, which decides whether an address is valid and whether a source is
reachable, and the reference recorder, which decides whether a reference may be written at
all. Between them they carry six of the feature's distinct failure paths, and four of the
spec's acceptance criteria are statements about what happens when something is wrong rather
than when it is right. The second concentration is the frontmatter round trip, because a
reference that does not survive a subsequent spec write is a silent data-loss bug that no
other test would catch: the load-bearing assertion there is that a reference recorded on a
spec is still present after the spec workflow's own commit path rewrites the document body.
The settings layer gets ordinary table coverage alongside the existing sections, including
the case that matters most in practice, which is that a project with no design section
continues to load and behave exactly as before.

**What the tests guarantee, in plain language.** That a project can declare several design
sources in different places and reach documents in all of them by name. That a document
written through the commands comes back byte-identical and appears in its source's listing.
That nothing Spektacular writes adds, removes or reformats anything in a design document.
That naming a source the project has not declared is refused, with the refusal naming the
source and listing the ones that do exist, and that nothing is recorded when it is refused.
That a reference to a document that is not there is reported with the source, the path and
the absolute location that was searched, rather than passing silently. That two different
specs can reference the same design independently. And that no command anywhere requires a
design document to match the implementation or rewrites one to make it match.

**Deliberate gaps.** No test exercises a non-file provider, because none ships; the
read-only path is covered instead by a stub source with no writer, which proves the refusal
exists and is worded usefully without pretending a remote backend is implemented. No search
or ranking tests are added, because design content is deliberately not indexed. No
performance tests are added: every operation is a direct file read or write against a path
the caller supplied.

**How the end-to-end layer is treated.** The hand-maintained expectations in those suites
mirror product surfaces rather than being derived from them, which is deliberate so the tests
cannot become tautological, and it means they do not fail when they drift. Any suite
expectation this feature touches is therefore updated in the same change as the surface it
mirrors, and a run of the affected suites is part of this plan's verification rather than
something left for whoever runs them next.

**Verification of the spec's success metrics.**

- *Specs written for features with a settled design carry a reference instead of the design's
  content.* **Manual — captured in the implementation test plan.** This is an observation
  about how specs are written after delivery, not a property of the code. What is testable,
  and is tested, is the mechanism underneath it: a spec can record a reference, and recording
  one does not put the design's content into the spec body.
- *Design detail raised during spec conversations is retained: for every accepted capture
  offer, a design document exists in a declared source and the spec references it.*
  **Behavioural**, at the mechanism level, in two layers. Contract tests guarantee the spec
  workflow's instructions actually make the offer, with the accept, defer and decline
  outcomes stated, and that acceptance directs both the write and the reference recording. An
  end-to-end run guarantees that an agent accepting the offer leaves behind both a design
  document in a declared source and a matching reference on the spec. The field claim that
  this holds for *every* offer is **manual — captured in the implementation test plan.**
- *Teams adopt design documents without relocating anything: a project can declare a folder
  of design documents it already had and read them unchanged, with no files moved or
  reformatted.* **Behavioural.** A test declares a source pointing at a directory of
  pre-existing documents that lives outside the project, lists it, reads from it, and asserts
  the bytes come back identical and that the directory's contents are unchanged afterwards,
  including documents that carry frontmatter of their own or none at all.
- *Broken references are caught before implementation: unresolved design references are
  reported while planning, and none is first discovered during implementation.*
  **Behavioural**, for the reporting half: the reference listing reports which references
  resolve and which do not and names what was searched, and a read of a missing design fails
  rather than returning empty. Contract tests guarantee the planning instructions oblige the
  agent to resolve every reference before designing and to stop rather than continue when one
  is missing. The claim that none is *first* discovered during implementation is **manual —
  captured in the implementation test plan.**
- *Design documents outlive the features that introduced them: at least some designs
  accumulate references from more than one spec.* **Behavioural**, for the capability: a test
  records the same design on two different specs and asserts each spec shows its reference
  independently and that the design document itself is unchanged by either. Whether this
  actually happens in a real project over time is **manual — captured in the implementation
  test plan.**

## Milestones & Phases

### Milestone 1: Designs are declared, listed, read and written

**What changes**: A project can point Spektacular at the places its design documents already
live, giving each one a name, and reach the documents there without moving or changing a
single file. Users and agents list what a source holds, read a document by naming its source
and path, and write a new one into a declared source, using the same command shapes they
already use for specs, plans and changelog records. Nothing Spektacular writes adds
frontmatter, reformats content, or imposes a structure on a design document. Getting an
address wrong is a clear refusal that names the declared sources or the exact location it
searched, never a silent empty result. A project that declares no design sources is
completely unaffected.

**Validation point**: A project declaring two sources in different locations, one of them a
folder outside the project that already contained documents, lists documents from both with
each tagged by its source; a document read back after being written is byte-identical; the
pre-existing folder is untouched; and naming an undeclared source, a missing document, or an
unreachable source location each produces a distinct refusal naming a concrete next step. The
full Go test suite passes.


#### - [x] Phase 1.1: Declare design sources in project settings

**Repo:** `spektacular`

A project gains an optional way to say where its design documents live, naming each place so
it can be addressed later. A project that says nothing carries on exactly as before. The
declaration is checked when settings are loaded, so a source with no name, a name used twice,
an unsupported storage backend or an empty location is rejected with a message that names the
problem and the correction, rather than surfacing much later as a confusing failure.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-declare-design-sources-in-project-settings)

**Acceptance criteria**:

- [x] A project can declare one or more named design sources in its settings, each naming a storage backend and a location.
- [x] A project that declares none loads and behaves exactly as it did before this change.
- [x] A source with no name, a duplicated name, an unsupported backend or an empty location is refused when settings load, with the offending entry named and a correction given.
- [x] A relative location is understood as relative to the folder holding the settings file, the same as every other relative path there.
- [x] Settings written back out preserve the declaration as it was written, and the settings format version is unchanged.


#### - [x] Phase 1.2: Name the read half of the storage interface

**Repo:** `spektacular`

A purely internal change with no user-visible effect, worth its own phase because it lands
before anything depends on it and must be provably behaviour-neutral. The storage interface
keeps every capability it has, but its read-only half becomes nameable on its own. This is
what lets a later phase give a design source a reader it always has and a writer it may not,
so a storage backend that cannot be written to becomes a clear refusal instead of an
undefined outcome. Nothing existing changes shape.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-name-the-read-half-of-the-storage-interface)

**Acceptance criteria**:

- [x] The read-only capabilities of the storage interface can be referred to independently of the write capabilities.
- [x] Every existing storage implementation satisfies the full interface exactly as before, with no implementation changes.
- [x] No existing caller changes the type it works with, and every existing command behaves identically.


#### - [x] Phase 1.3: Resolve declared sources into readable stores

**Repo:** `spektacular`

The piece that turns a declaration into something documents can actually be read from and
written to. Each declared location is resolved to a real place on disk, and a location that
is not there, or is not a folder, stops everything immediately with a message naming the
source, the place it resolved to and the base it resolved from, rather than quietly behaving
as an empty source. Addressing a document requires both the source it belongs to and the
document within it, and naming a source the project has not declared is refused with the
declared names listed.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-resolve-declared-sources-into-readable-stores)

**Acceptance criteria**:

- [x] Every declared source resolves to an absolute location, with relative locations resolved from the settings folder and absolute ones used as written.
- [x] A declared location that does not exist, or is not a folder, is refused with the source, the resolved location and the base it resolved from all named.
- [x] A declared backend that this build does not implement is refused by name rather than ignored.
- [x] Addressing a source the project has not declared is refused with the unknown name quoted and the declared names listed.
- [x] Addressing a document that does not exist within a source that does is a different failure from addressing an unknown source.
- [x] A source whose backend cannot be written to refuses a write by name instead of failing in an unspecified way.


#### - [x] Phase 1.4: Reach design documents from the command line

**Repo:** `spektacular`

The commands users and agents actually use: see what sources the project declares, list the
documents in them, read one by naming its source and path, and write one into a declared
source. Listing without narrowing covers every source at once, with each document reporting
which source it came from. Crucially, nothing here touches a document's content: no
frontmatter is added, nothing is reformatted, and a document read back after being written is
byte-for-byte what went in. Each command publishes its own input and output shape so an agent
can discover how to call it.

*Technical detail:* [context.md#phase-14](./context.md#phase-14-reach-design-documents-from-the-command-line)

**Acceptance criteria**:

- [x] The declared sources can be listed, each showing its name, backend and resolved location.
- [x] Design documents can be listed across every declared source at once, each tagged with its source, and narrowed to a single source on request.
- [x] A document can be read by naming its source and its path within that source.
- [x] A document can be written into a declared source and read back byte-for-byte identical, and it then appears in that source's listing.
- [x] Nothing Spektacular writes adds, removes or reformats any part of a design document's content.
- [x] Each command reports its own input and output shape on request, as the knowledge commands already do.
- [x] Running any of these commands outside a Spektacular project fails with the same explicit no-project error every other project command gives.


### Milestone 2: A spec can point at a design instead of absorbing it

**What changes**: A spec can carry references to one or more design documents, so the design
binds the work without its detail being copied into the spec. Recording a reference is an
explicit action that is checked before it takes effect: naming a source the project has not
declared is refused and nothing is written. A reference records which source it belongs to as
well as which document, so it can always be resolved back to a real place, and asking what a
spec references reports which of them are actually present and, for any that are not, exactly
what was looked for. References survive the spec being rewritten, which is what makes them
durable rather than something that quietly disappears the next time the spec is edited. The
same design can be referenced by any number of specs, and a design document knows nothing
about the specs that point at it, so it outlives the feature that introduced it.

**Validation point**: A reference recorded on a spec is visible when the spec is read, is
still there after the spec's body is rewritten through the normal commit path, and does not
put any of the design's content into the spec body. The same design recorded on two specs
shows independently on each. A reference naming an undeclared source is refused with the
unknown source named and nothing recorded. A reference to a document that has been removed is
reported as unresolved with the location that was searched, and reading it fails rather than
returning nothing. The full Go test suite passes.


#### - [x] Phase 2.1: Make design references part of a spec's record

**Repo:** `spektacular`

A spec gains the ability to carry a list of design references in its own record, each naming
both the source it belongs to and the document within it. The important property is
durability: the spec workflow commits a spec by writing a freshly assembled body over the
stored file, so a reference has to survive that rewrite or it silently disappears. Reading is
forgiving, so a spec with no references, or with a malformed list written by hand, reads as
carrying none rather than failing to load.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-make-design-references-part-of-a-specs-record)

**Acceptance criteria**:

- [x] A spec's record can carry an ordered list of design references, each naming a source and a path.
- [x] A reference survives the spec's body being rewritten through the normal commit path.
- [x] A spec with no references, an empty list, or a malformed list reads as carrying no references rather than failing.
- [x] The existing created, status and closed information on every artifact is unaffected, and artifacts that carry no references are written exactly as before.


#### - [x] Phase 2.2: Record, remove and resolve a spec's design references

**Repo:** `spektacular`

The commands that put a reference on a spec, take one off, and report what a spec references.
Recording is validated before anything is written: a reference naming a source the project
has not declared is refused with the unknown source named, and the spec is left untouched.
Asking what a spec references reports each one with whether the document was actually found
and the exact location searched, so a broken reference is visible rather than assumed absent.
Any spec may reference any design, and the same design may be referenced by any number of
specs.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-record-remove-and-resolve-a-specs-design-references)

**Acceptance criteria**:

- [x] A design reference can be recorded on a spec, and reading the spec afterwards shows it.
- [x] Recording a reference does not put any of the design's content into the spec body.
- [x] Recording a reference that names a source the project has not declared fails with the unknown source named, and no reference is recorded.
- [x] Recording a reference a spec already carries succeeds and changes nothing.
- [x] A reference can be removed from a spec, leaving the others in place.
- [x] Asking what a spec references reports every reference with whether it resolves and the exact location searched, and reports how many do not resolve.
- [x] Reading a referenced design that no longer exists fails, naming the source, the path and the location searched, and saying how to correct it.
- [x] Two different specs can reference the same design, each showing its reference independently of the other.


### Milestone 3: The workflows capture designs and build on them

**What changes**: Design detail stops being lost. When a spec conversation settles an API
shape, a user-facing flow, a data format or a worked example of any of these, the agent
offers to capture it as a design document and record the reference, rather than compressing
it into a one-line steer or letting it fall out of the conversation entirely. The offer is
always an offer: declining leaves nothing written and the detail out of the spec, accepting
writes the design and records the reference, and a decline is final while a deferral can be
raised again. On the other side, planning a spec that carries references now begins by
resolving and reading every one of them, builds on what they say instead of redesigning it,
and names each design and the source it came from in the finished plan. A reference that
cannot be found stops planning with a report rather than being passed over, so a broken
reference is caught before anyone starts implementing.

**Validation point**: The rendered spec and plan instructions carry the offer and the
obligation in the wording the contract tests pin, including the accept, defer and decline
outcomes and the stop-on-unresolved rule, and carry none of the shapes that were ruled out.
The standing agent instruction installs cleanly for every supported agent, is not duplicated
when a second agent is installed, and leaves surrounding content alone. An end-to-end run of
the affected workflow suites passes with their expectations updated in this same change. The
full Go test suite passes.


#### - [x] Phase 3.1: Give every agent the standing design-capture rule

**Repo:** `spektacular`

Agents gain a standing instruction, installed into the project's agent instruction file
alongside the existing ones, telling them to recognise when a conversation has settled an API
shape, a user-facing flow, a data format or a worked example, and to offer to capture it as a
design document. The rule is explicit that this is always an offer: accepting writes the
design and records the reference, declining writes nothing and is final for that item, and a
deferral can be raised again later. It installs the same way the existing standing rules do,
so it survives reinstallation and installing for a second agent.

*Technical detail:* [context.md#phase-31](./context.md#phase-31-give-every-agent-the-standing-design-capture-rule)

**Acceptance criteria**:

- [x] The project's agent instruction file gains a design-capture section for every supported agent.
- [x] The section states what counts as design-level detail, that the agent offers rather than writes, and the accept, defer and decline outcomes.
- [x] Installing again, or installing for a second agent, leaves exactly one copy of the section and does not disturb the surrounding content or the order of the existing sections.
- [x] Editing the source of the section and reinstalling replaces it in place.
- [x] The section directs agents at the design commands rather than at a skill, so nothing depends on skill lookup.


#### - [x] Phase 3.2: Make the workflows offer capture and honour designs

**Repo:** `spektacular`

The two workflow changes that make the feature actually happen rather than merely be
possible. In the spec workflow, the step that today tells an agent to compress worked design
down to a one-line steer now also offers to capture it instead, at exactly the moment the
detail would otherwise be lost. In the plan workflow, planning a spec that carries references
must resolve and read every one before designing anything, build on what they say rather than
re-deriving it, name each design and its source in the finished plan, and stop and report
rather than continue if one cannot be found. No new steps and no new interruption points are
added.

*Technical detail:* [context.md#phase-32](./context.md#phase-32-make-the-workflows-offer-capture-and-honour-designs)

**Acceptance criteria**:

- [x] The spec workflow's technical-direction step offers to capture settled design detail as a design document, with the accept, defer and decline outcomes stated, and directs both the write and the reference recording on acceptance.
- [x] Declining leaves no design document written and the detail out of the spec body.
- [x] The plan workflow's discovery step obliges the agent to resolve and read every design the spec references before any design work begins.
- [x] The plan workflow stops and reports when a referenced design cannot be found, rather than proceeding as though the spec carried none.
- [x] The finished plan names each design document read and the source it was read from.
- [x] The plan workflow's architecture step is explicit that a referenced design is built on rather than redesigned.
- [x] The skills that introduce the spec and plan workflows mention design references and point at the commands that reach them.


#### - [x] Phase 3.3: Bring the end-to-end suites back in step

**Repo:** `spektacular`

The end-to-end suites check the workflows by having a real agent drive them, and their
expectations are written by hand on purpose so the tests cannot simply agree with whatever
the templates happen to say. That makes them the one layer that does not notice when a
product surface moves, so anything this feature changed that those expectations mirror is
brought back in step here, and the affected suites are actually run rather than left for
whoever runs them next.

*Technical detail:* [context.md#phase-33](./context.md#phase-33-bring-the-end-to-end-suites-back-in-step)

**Acceptance criteria**:

- [x] Every hand-written expectation in the end-to-end suites that mirrors a surface this feature changed is updated in this same change.
- [x] The seeded project settings used by the suites remain valid against the current settings rules.
- [x] The affected suites are run and pass, and any failure is fixed rather than recorded as pre-existing.


### Milestone 4: The concept is documented where people will find it

**What changes**: Design documents are explained as a project-level idea in their own right,
not left as a config key someone has to infer a concept from. The documentation site gains a
page covering what a design document is, when to reach for one instead of putting the detail
in a spec, how designs relate to specs and plans, and how the commands are used, reachable
from the site navigation. The configuration reference documents the new settings key in the
same shape as the project's existing shared-store key and links out to the concept rather
than restating it, and the repository's own configuration documentation gains the matching
key so the two do not drift apart.

**Validation point**: The documentation site builds and type-checks with no errors or
warnings, the new page contains no raw layout markup, its section shading alternates
correctly against its neighbours, and no em dash appears in any prose added to that
repository. The configuration reference's key list and its worked example both include the
new key. The repository documentation tests still pass.


#### - [x] Phase 4.1: Explain design documents on the documentation site

**Repo:** `docs`

A new page explains design documents as an idea in their own right: what one is, when to
reach for one instead of putting the detail into a spec, how a design relates to the specs and
plans that point at it, what happens to it once the work ships, and how the commands are used.
It is reachable from the site navigation. The page follows the site's existing shape for a
concept page, so it reads as part of the site rather than a bolted-on appendix.

*Technical detail:* [context.md#phase-41](./context.md#phase-41-explain-design-documents-on-the-documentation-site)

**Content outline**

Seven bands after the hero, shading alternating from plain, following the knowledge-base
page's shape. Wording below is illustrative; the key names, command names and payload shapes
in it are fixed by this plan and must be reproduced exactly.

1. *Hero* - heading "Design Documents", sub: "The worked design a feature is built to, kept
   where your team already keeps it, and bound to the spec that needs it."
2. *What a design document is* (plain). Two or three paragraphs. A design document holds the
   settled shape of something: an API, a user-facing flow, a data format, or a worked example
   of one. Spektacular owns the reference, not the document: it resolves and reads a design,
   it never rewrites one or imposes a structure on it.
3. *When to use one instead of a spec section* (shaded). The rule of thumb, as a short table:
   a hard boundary the solution must honour is a constraint and stays in the spec; a
   preference the planner may adapt is technical direction and stays in the spec; a worked
   design that would make the spec unreadable if inlined is a design document the spec points
   at. Close on the discipline: summarise and link, never restate.
4. *How they relate to specs and plans* (plain). A spec records references; a plan resolves
   and reads every one before designing and names each with its source; once the work ships
   the design is a historical record, never force-synced against the code. One short
   paragraph each, then a fenced example of a spec's recorded references:

   ```yaml
   ---
   created_date: "2026-09-20"
   document_status: final
   designs:
     - source: api
       path: payments/v2.md
   ---
   ```

5. *Declaring where designs live* (shaded). The settings block with both a relative and an
   absolute location, noting that a relative location resolves from the folder holding
   `config.yaml` and that only the `file` backend ships today, then a link to the
   configuration reference:

   ```yaml
   design:
     sources:
       - name: api
         provider: file
         config:
           location: ../design/api
       - name: ux
         provider: file
         config:
           location: ${HOME}/work/ux-designs
   ```

6. *Working with designs from the command line* (plain). One fenced block per verb with a
   one-line gloss above each: `design sources`, `design list` and `design list --source api`,
   `design read --data '{"source":"api","path":"payments/v2.md"}'`, `design write --data
   '{"source":"api","path":"payments/v2.md"}' --from ./draft.md`, then the reference verbs
   `design ref add --data '{"spec":"000054_example","source":"api","path":"payments/v2.md"}'`,
   `design ref remove`, and `design ref list --data '{"spec":"000054_example"}'` with a short
   sample of its output showing a resolved and an unresolved entry.
7. *When a reference cannot be found* (shaded). What the failure looks like and why it is a
   failure rather than an empty result: the report names the source, the path and the absolute
   location searched, so a broken reference is caught while planning instead of during
   implementation.
8. *Why it works this way* (plain). Three short rationale paragraphs: the spec stays readable
   because the design is referenced rather than absorbed; the document is never rewritten to
   match the code, because a shipped design is a record of what was agreed; storage is
   declared rather than imposed, so a team adopts design documents without moving a file.
9. *Closing call to action* - heading "See it in the workflow", body one sentence, button to
   the how-it-works page.

**Acceptance criteria**:

- [x] The documentation site carries a page explaining what design documents are, when to use one instead of putting detail in a spec, and how they relate to specs and plans.
- [x] The page documents the commands that list, read, write and reference design documents, with worked examples.
- [x] The page is reachable from the site navigation.
- [x] The page is built from the site's existing section components, with no raw layout markup, and its section shading alternates correctly.
- [x] No em dash appears anywhere in the added prose.
- [x] The site builds and type-checks with no errors and no warnings.


#### - [x] Phase 4.2: Document the new settings key in the configuration reference

**Repo:** `docs`

The configuration reference gains the new settings key, documented in exactly the same shape
as the project's existing shared-store key: a short description of what the section is for and
a bullet per sub-key, linking out to the concept page rather than restating the concept. The
worked settings example on that page gains the block too, so someone copying it gets something
that works.

*Technical detail:* [context.md#phase-42](./context.md#phase-42-document-the-new-settings-key-in-the-configuration-reference)

**Content example**

A new key entry in the project settings key list, placed after the existing shared-store key
and before the repository-registry key, in exactly the existing entry's form. Wording is
illustrative; the key names are fixed.

```mdx
<ConfigKey name="design" type="section" defaultValue="none">

  Ordered list of the design sources the project declares. A design document
  holds the worked design a feature is built to, and a spec references one
  rather than restating it. Design sources are declared by the project only;
  a repository does not declare its own.

  - `design.sources[].name`: the name this source is addressed by. Must be
    unique among the project's design sources.
  - `design.sources[].provider`: storage backend; only `file` ships today.
  - `design.sources[].config.location`: where the file provider reads and
    writes design documents. Relative paths resolve from the folder holding
    `config.yaml`, the same base a `repos` entry uses.

  See the [Design Documents](/design-documents/) page for the full reference.

</ConfigKey>
```

The worked settings example higher on the page gains the matching block, with the same
inline comments about the relative-location base:

```yaml
design:
  sources:                        # where the project's design documents live;
    - name: api                   # declared by the project, never by a repository
      provider: file
      config:
        location: ../design/api   # relative to this file
```

**Acceptance criteria**:

- [x] The configuration reference documents the new settings section and each of its sub-keys, in the same shape as the existing shared-store key.
- [x] The worked settings example on that page includes the new block, with its relative-location rule noted.
- [x] The key entry links out to the concept page instead of restating the concept.
- [x] The count of top-level keys stated on that page is corrected.
- [x] The surrounding section shading and heading style are unchanged, and no em dash appears in the added prose.


#### - [x] Phase 4.3: Keep the repository's own configuration docs in step

**Repo:** `spektacular`

The repository's own README documents the settings file key by key and is checked by tests
that assert specific wording is present. It gains the new key and a sentence placing design
documents alongside specs, plans and knowledge, so the two descriptions of the same settings
file do not drift apart.

*Technical detail:* [context.md#phase-43](./context.md#phase-43-keep-the-repositorys-own-configuration-docs-in-step)

**Acceptance criteria**:

- [x] The repository's configuration documentation shows the new settings block in its worked example and explains what it is for.
- [x] The relative-location rule stated there covers the new key alongside the existing ones.
- [x] Design documents are named alongside specs, plans and knowledge where the project's artifacts are introduced.
- [x] The existing documentation tests still pass.

## Open Questions

Two items genuinely cannot be settled before the code exists. Everything else that was open
during planning has been decided and recorded as an assumption rather than parked here.

- **Whether the settings format version must rise after all.** The plan adds the new settings
  section as a purely additive optional key and registers no upgrade step, because the project's
  own rule is that the format version rises only when a change would make an existing file
  misread, and an absent section validly means "no design sources". The residual risk is not in
  the loader, which tolerates the absence, but in what an older Spektacular does when it meets a
  settings file that carries the new section: it ignores it silently rather than refusing the
  file, so a stale binary appears to work while every design source is invisible. **Depends on**:
  observing that behaviour against a real older build, which is only possible once the new key
  exists. **What the implementer should do**: if the silent-ignore behaviour is judged
  unacceptable, STOP and ask the user before bumping, because the bump forces every existing
  project through an upgrade for a change that alters no existing value, and the version constant
  and the upgrade step must move together or the registry test fails.

- **Whether the end-to-end spec suite should assert the capture offer's outcome.** The plan
  requires the contract tests to prove the instruction carries the offer, and names the extra
  end-to-end assertion — that an accepted capture leaves both a design document in a declared
  source and a matching reference on the spec — as a decision to make explicitly rather than a
  requirement. **Depends on**: how long the spec suite actually runs once the new prose lengthens
  the conversation, and whether a real agent reliably reaches the offer at all, neither of which
  is knowable before the prose exists. **What the implementer should do**: run the suite, then
  decide and record the decision either way in the phase's notes. Do not leave it unstated. If
  the agent does not reach the offer reliably, that is a defect in the prose rather than a reason
  to drop the assertion, and it should be raised with the user rather than worked around.

## Out of Scope

From the spec's non-goals:

- **Storage backends other than local files.** Git checkouts, remote URLs and issue trackers are
  deliberately later work. This plan ships the file backend only, and refuses any other declared
  backend by name rather than ignoring it.
- **Design sources declared by a registered repository.** Design sources are a project-level
  declaration only. A repository's own settings gain nothing, and the existing refusal of a
  sources list in a repository's settings stands.
- **Keeping a shipped design document in step with the code.** No drift detection, no checksums,
  no re-sync, and no command that requires a design to match the implementation. A shipped design
  is a record of what was agreed.
- **Rewriting or reformatting a user's design document.** Nothing Spektacular writes adds
  frontmatter, changes whitespace, or imposes a structure on a design document's content. This is
  why the design commands do not reuse the shared artifact-file machinery, which stamps lifecycle
  frontmatter on every write.
- **Indexing design content in knowledge search.** Design documents are not searchable through
  the knowledge commands and get no ranking, tags, categories or de-duplication.
- **Immutable decision records with supersession, in the sense of an architecture decision log.**
  A design document is the current worked design, not a dated decision with a supersession chain.

Deliberately left to a later plan by the chosen design:

- **Any actual read-only remote backend.** The plan leaves room for one by naming the read half of
  the storage interface separately and letting a source carry an absent writer, so a backend that
  cannot be written to is a named refusal. It builds no remote backend, no backend registry and no
  capability negotiation.
- **Design documents in the cross-kind artifact listing.** The command that lists artifacts across
  kinds is scoped to artifacts a workflow produces, and a design document is authored by a person
  or captured on request. Listing design documents is served by their own listing command. This is
  a deferral, not a rejection, and would be a small addition if the scope of that command is ever
  widened.
- **A dedicated design skill.** The capture offer and the planning obligation are delivered as a
  standing agent instruction and step prose that name the commands directly. No new agent skill is
  installed, which also avoids the known problem that a nested workflow skill cannot be fetched by
  name at runtime.
- **A threshold setting for how readily the capture offer fires.** The existing spec-capture
  trigger has one, and it is the precedent to follow if the offer proves too eager or too shy in
  practice, but none is added now.
- **A human-readable design-references section in the spec body.** References live in the spec's
  record, which is what makes them validated and durable. Rendering them into the spec's visible
  text is possible later and changes nothing about where the truth lives.
- **Scaffolding design source locations.** A declared location that is not there is reported, not
  created, so project initialisation and footprint repair are untouched.

Explicitly not being changed:

- **How specs, plans, changelog records or knowledge entries are stored or addressed.** The only
  edit to an existing artifact's record is one additional optional field on a spec's frontmatter,
  which is absent and byte-for-byte invisible on every artifact that carries no design references.
- **The settings format version.** No version bump and no upgrade step, because the new section is
  additive and its absence is valid. The residual risk this leaves is recorded in Open Questions.

## Changelog

### 2026-09-20 - Phase 1.1: Declare design sources in project settings

**What was done**: The project settings type gained an optional `design` section holding an
ordered list of named design sources, reusing the same `SourceConfig` type that already backs
the project's shared knowledge stores. The section validates on load, refusing an entry with
no name, a name used twice in the list, an unsupported provider or an empty location. A project
that declares no design sources loads and marshals exactly as it did before, and the settings
format version is unchanged.

**Deviations**: One, deliberate. The phase notes said to model `DesignConfig.Validate` "line
for line" on `KnowledgeConfig.Validate`, whose unsupported-provider and empty-location cases
return a bare `fmt.Errorf` with no next action. Copying that would have shipped two refusals
that fail this phase's own acceptance criterion ("the offending entry named **and a correction
given**") and the repo convention that every error path carries a runnable next step. All four
design refusals are therefore `output.NewError("config_invalid", ...).WithNextAction(...)`, and
the validator's doc comment states the divergence so a maintainer does not "simplify" it back.
`KnowledgeConfig.Validate` was left untouched, being outside this phase's scope.

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/config_test.go`

**Discoveries**:
- `omitempty` on a struct-typed field does omit an all-zero struct in `gopkg.in/yaml.v3`, so
  `Design DesignConfig` needs nothing extra to stay invisible in a config that declares no
  design sources. This project's own `.spektacular/config.yaml` carries no `knowledge:` key for
  the same reason, which is the cheapest way to confirm the behaviour.
- A design source location is deliberately **not** part of `Config.storeDirs()`. That list
  drives both the write-time re-expression of a store directory and the `escapesRoot` refusal,
  so adding design to it would rewrite the author's declared path and forbid the
  outside-the-project-root locations this feature exists to support. The declared value is
  carried and written back verbatim instead, and resolution belongs to `internal/design`.
- Acceptance criterion 4's behavioural half (a relative location being *joined* to the settings
  folder) cannot land in this phase, because nothing resolves a design location yet. The
  declaration layer states the rule in the `DesignConfig` doc comment; Phase 1.3's `NewSet` is
  where the joining is implemented and asserted.

### 2026-09-20 - Phase 1.2: Name the read half of the storage interface

**What was done**: The storage interface's read half was given a name of its own. `Reader`
holds `Read`, `List`, `Exists` and `Search`; `Writer` holds `Write` and `Delete`; `Store` is
now `interface { Reader; Writer; Root() string }`. Every method's documentation moved with it
verbatim, including the long `Search` ranking contract, so nothing documented changed. This is
what lets a later phase give a design source a reader it always has and a writer it may not,
turning "this provider cannot write" into a named refusal.

**Deviations**: None to the change itself. Two process notes: the compile-time assertions name
the concrete `*ignoreStore` rather than the `Store`-typed return value of `NewIgnoreStore`,
because asserting the latter would only re-prove that `Store` embeds `Reader` and `Writer`
without saying anything about the implementation. And this phase's verification was run in the
main context rather than a sub-agent, the phase being a four-command behaviour-neutral check.

**Files changed**:
- `spektacular: internal/store/store.go`
- `spektacular: internal/store/store_test.go`

**Discoveries**:
- The refactor was genuinely free at every call site: **87** `store.Store` references across
  `cmd/` and `internal/` compile unchanged, and nothing outside the interface declaration and
  its test refers to `Reader` or `Writer` at all. That count is the cheapest proof of the
  "no caller changes the type it works with" criterion, and a better one than any unit test.
- The assertions cannot catch a method added to `Store` directly instead of to `Reader` or
  `Writer` — both implementations would carry it and everything would still compile. Keeping
  the split complete is therefore a review obligation, and the test comment says so along with
  the placement rule: a new method belongs in `Reader` if it only observes the store and in
  `Writer` if it mutates it.

### 2026-09-20 - Phase 1.3: Resolve declared sources into readable stores

**What was done**: A new `internal/design` package turns a declaration into something documents
can be read from and written to. `NewSet` resolves each declared location (relative ones from
the folder holding `config.yaml`, absolute ones as written, neither required to sit inside the
project root), refuses a provider this build does not implement and a location that is not a
directory, and builds each source's store with the standard constructor so `.spektacular_ignore`
filtering comes free. Documents are addressed by source and path together, with six named
refusals covering every way an address or a declaration can be wrong.

**Deviations**: One addition beyond the plan's five-method projection. `Set.Exists` was added
alongside `Resolve`, because Phase 2.2's reference-resolution report needs a per-reference
"is this document present?" and answering that through `Read` would pull a whole file off disk
to produce a boolean. `Resolve` and `Exists` are the pair that report on a reference without
reading it.

**Files changed**:
- `spektacular: internal/design/design.go`
- `spektacular: internal/design/errors.go`
- `spektacular: internal/design/paths.go`
- `spektacular: internal/design/design_test.go`

**Discoveries**:
- **Look the source up before touching a store, always.** Every addressed method resolves the
  source name first, which is what keeps `design_source_unknown` and `design_not_found`
  distinct. Reading through to the store first would collapse "that source does not exist" and
  "that source does not hold this" into one indistinguishable miss, and one of this phase's
  criteria is precisely that they differ.
- **An empty declared-sources list needs its own wording.** "Declared sources are: (none)" is
  useless to an agent. `availableSources` says instead that the project declares no design
  sources and names the config key to add one, which is the actionable form of the same fact.
- **The `${VAR}` expansion is the config loader's, not the design set's.** A test that wants to
  exercise it has to go through a written `config.yaml` and `config.FromYAMLFile`; calling
  `NewSet` with a hand-built `config.Config` never expands anything. Any config-file fixture
  also needs `schema`, `name`, `command` and at least one `repos` entry, because
  `config.Validate` requires a registered repo.
- **A directory snapshot taken with `filepath.WalkDir` is the only honest oracle for "nothing
  was modified".** Asking the design package whether it changed the folder it owns would be
  circular, so the test walks the filesystem itself before and after.

### 2026-09-20 - Phase 1.4: Reach design documents from the command line

**What was done**: A `spektacular design` command family, hand-written against
`internal/design`: `sources` reports each declared source with its resolved location, `list`
fans out across every source tagged by source and narrows with `--source`, `read` writes a
document's raw bytes to stdout, and `write --from <file>` stores bytes unchanged. Every
subcommand publishes its input and output shape with `--schema`, and every refusal carries a
runnable next action. Nothing here adds, removes or reformats any part of a design document.

**Deviations**: None to the plan. One correction to my own first draft: `design read --schema`
initially advertised an object with a `content` field while the command actually writes raw
bytes, so it now publishes `{"type":"string"}`. Two small refusals were added beyond the
plan's list, `design_data_required` and `design_from_required`, so a missing payload or a
missing source file also names its next step instead of falling back to a bare error string as
the knowledge family's equivalent does.

**Files changed**:
- `spektacular: cmd/design.go`
- `spektacular: cmd/root.go`
- `spektacular: cmd/design_test.go`
- `spektacular: cmd/no_project_test.go`

**Discoveries**:
- **A published schema that lies is worse than no schema.** An agent that trusts
  `output: {content: string}` would try to parse a Markdown document as JSON and conclude the
  command is broken. Where a command emits raw bytes, the schema has to say so.
- **`--schema` is not reachable outside a valid, current project.** The root command's pre-run
  runs the project and version checks before any `RunE`, so `design read --schema` in a bare
  directory returns `no_project`, and against an out-of-date config it returns
  `upgrade_required`. This is pre-existing behaviour shared with the knowledge family, not
  something this phase introduced, but it is worth knowing before writing a fixture.
- **A hand-written fixture `config.yaml` needs `skills_version` as well as `schema`**, or every
  command fails `upgrade_required` before reaching the code under test. `writeSpecCommandConfig`
  in `cmd/spec_test.go` already supplies both, plus `name` and a `repos` entry, and is the
  helper to build on.
- **`cmd/init_test.go` already has `snapshotDir`** (walk + read + sha256), the independent
  oracle for "this directory was not otherwise modified". Grep `cmd/*_test.go` for a helper
  before writing one; a near-duplicate was drafted here and only caught by the compiler.

### 2026-09-20 - Phase 2.1: Make design references part of a spec's record

**What was done**: The artifact frontmatter schema gained a modelled `designs` list of
`{source, path}` pairs, on the in-memory type, both halves of its on-disk shape, and the merge
step. Reads are lenient in keeping with the rest of the block: an absent, empty, malformed or
non-list value reads as no references rather than failing the parse, and an entry missing
either half of its address is dropped. A body-only rewrite preserves the list, which is what
makes a reference durable rather than something that disappears the next time a spec is
committed. An artifact carrying no references is written byte-identically to before.

**Deviations**: None. One judgement inside the plan's direction: `UpdateOptions.Designs` is a
pointer to a slice rather than a plain slice, because three states must be distinguishable and
the sibling provenance fields' `""`-means-no-change convention cannot express them. Nil means
"leave references alone", which is what every ordinary artifact write passes; a non-nil slice
replaces; a non-nil empty slice clears.

**Files changed**:
- `spektacular: internal/metadata/metadata.go`
- `spektacular: internal/metadata/merge.go`
- `spektacular: internal/metadata/metadata_test.go`
- `spektacular: internal/metadata/merge_test.go`
- `spektacular: cmd/storefile_metadata_test.go`

**Discoveries**:
- **The durability guarantee needs pinning at two levels, and that is not redundancy.** The
  unit test pins `Merge`'s contract that a nil `Designs` preserves; the command-level test
  drives a real `spec file write` and proves the write path actually passes nil via
  `provenanceOpts` rather than rebuilding the block from scratch. Either one alone would let
  the other regress silently.
- **`provenanceOpts` needed no change, and that is the point.** It builds `UpdateOptions`
  without touching `Designs`, so the ordinary write path preserves references for free. Any
  future write path that assembles `UpdateOptions` itself has to make the same choice
  deliberately, because passing a non-nil empty slice there would silently clear every
  reference on the artifact.
- **`cmd/storefile_metadata_test.go` mixes two test-harness styles.** Some existing tests drive
  `rootCmd.SetArgs` + `rootCmd.Execute()` directly rather than going through
  `resetRootCmd`/`runRootCmd`. The older style is pre-existing and was left alone, but it is a
  latent order-dependence risk in a suite that runs with `-shuffle=on`.

### 2026-09-20 - Phase 2.2: Record, remove and resolve a spec's design references

**What was done**: `design ref add`, `remove` and `list`. Recording validates the source
against the project's declarations before touching the spec, so an undeclared source is refused
with the declared names listed and the spec is left byte-identical. A reference lands in the
spec's frontmatter and the body comes through untouched, so none of the design's content enters
the spec. `ref list` reports every reference with whether it resolves and the absolute location
searched, plus an unresolved count and a next action when anything is broken. The same design
can be referenced by any number of specs, each independently.

**Deviations**: None. Three judgements inside the plan's direction, all of which it anticipated:
a duplicate `add` is a silent no-op rather than an error, since the operation is idempotent by
nature and an agent resuming an interrupted capture should not have to tell "already recorded"
from "failed"; `add` validates only the source and not the document's existence, so a reference
may be recorded before the design is written; and `ref list` returns a success envelope even
when nothing resolves, because the plan workflow needs every reference in one call and
`output.ErrorResponse` has fixed fields that cannot carry a list.

**Files changed**:
- `spektacular: cmd/design_ref.go`
- `spektacular: cmd/design_ref_test.go`

**Discoveries**:
- **A reference whose source is no longer declared still has to be listed.** `Resolve` refuses
  it, so the obvious implementation would drop it from the report entirely and show a spec as
  having fewer references than it carries. It is listed instead with an empty location and
  counted unresolved, which is exactly the case the command exists to surface. This one cannot
  be produced through `ref add`, so a test for it must seed the frontmatter by hand.
- **Prove a negative assertion by inverting it once.** The all-resolved case asserts that the
  `next_action` key is *absent* rather than empty; the way to know that assertion bites is to
  flip it to `Contains` and watch it fail. An unverified negative assertion can pass for the
  wrong reason indefinitely.
- **Frontmatter is normalised on the first reference write.** yaml.v3 re-renders the block with
  4-space sequence indentation and a quoted date, so a spec whose block was hand-written with
  2-space indent is read fine but comes back reformatted after `ref add`. This touches only the
  frontmatter block Spektacular owns, never the body, and never a design document.

### 2026-09-20 - Phase 3.1: Give every agent the standing design-capture rule

**What was done**: A new managed `AGENTS.md` section, `## Design-Worthy Detail Recognition`,
installed for claude, bob and codex between the spec-trigger and draft-presentation sections.
It names the four kinds of settled detail worth capturing, sets a deliberately high three-part
bar (settled, worked, and unreadable if inlined), states that the agent offers rather than
writes, and gives the accept, defer and decline outcomes in the same vocabulary the
knowledge-capture offer already uses. Accept names both `design write` and `design ref add`,
and the section names the commands directly rather than pointing at a skill.

**Deviations**: None. The decline branch says one thing its knowledge-trigger sibling has no
need to: declining must not result in the detail being smuggled into the spec body instead.

**Files changed**:
- `spektacular: templates/agents/design-trigger.md`
- `spektacular: internal/agent/design_trigger.go`
- `spektacular: internal/agent/claude.go`
- `spektacular: internal/agent/bob.go`
- `spektacular: internal/agent/codex.go`
- `spektacular: internal/agent/design_trigger_test.go`
- `spektacular: internal/agent/spec_trigger_test.go`

**Discoveries**:
- **The existing heading-order assertion did not fail when it arguably should have.** The plan
  expected `spec_trigger_test.go`'s order chain to need updating; it passed untouched, because
  it pins only the relative order of the four headings it names and the new section slots
  between two of them. So the new section's position was correct but *incidental* rather than
  pinned. It is now named in the chain, and inverting the install order was confirmed to make
  the test fail. This is a small live example of the hand-maintained-oracle problem the plan's
  testing section describes: an expectation that mirrors a surface does not complain when the
  surface grows.
- **`instruction_surface_test.go` is not an inventory of the managed sections**, despite the
  name suggesting it might be. It enumerates forbidden instruction substrings, stale retrieval
  claims, knowledge subcommands and per-skill content, and never reads `templates/agents/*`. A
  new managed section needs no entry there.
- **The wording guards are line-sensitive.** `templates/agents/design-trigger.md` is hard
  wrapped, so an anchor phrase spanning a line wrap can never match; every asserted phrase sits
  on a single source line. Reflowing that template would break the guards without changing its
  meaning, which is worth knowing before anyone tidies the prose.

### 2026-09-20 - Phase 3.2: Make the workflows offer capture and honour designs

**What was done**: Six template edits and no Go changes. The spec workflow's technical-approach
step now offers to capture a settled design at the exact point the step tells the agent to
compress it to a one-line steer, with the three-part bar, the accept/defer/decline outcomes,
both commands on acceptance, and the rule that declining leaves the detail out of the spec body
rather than moving it in. The plan workflow's discovery step must resolve and read every
referenced design before designing and stop and report when any is unresolved; architecture must
build on a referenced design rather than re-derive it; dependencies must name each design and
its source, or state explicitly that there are none. Both workflow skills introduce design
documents and name the commands. No new steps, no new interruption points, and no scaffold
headings.

**Deviations**: None to the plan. One addition beyond its brief: a template-wide guard that
every `--data '{...}'` example is well formed, prompted by a defect I introduced and caught by
hand (see Discoveries).

**Files changed**:
- `spektacular: templates/steps/spec/05-technical_approach.md`
- `spektacular: templates/steps/plan/02-discovery.md`
- `spektacular: templates/steps/plan/03-architecture.md`
- `spektacular: templates/steps/plan/07-dependencies.md`
- `spektacular: templates/skills/workflows/spek-new/SKILL.md`
- `spektacular: templates/skills/workflows/spek-plan/SKILL.md`
- `spektacular: internal/steps/spec/steps_test.go`
- `spektacular: internal/steps/plan/steps_test.go`
- `spektacular: templates/skill_list_command_test.go`
- `spektacular: templates/data_payload_wellformed_test.go`

**Discoveries**:
- **The contract tests could not catch a malformed command example, and now one can.** While
  writing the skill prose I opened three `--data` payloads with `'{` and closed them with `}"`.
  Every existing contract test asserts that a phrase is *present*, so all of them passed on
  prose that would make a copying agent issue a broken command. The new walker checks all 80
  template files and was proven by reintroducing the exact defect. It has to count braces rather
  than find the first `}`, because mustache placeholders nest inside payloads and a naive scan
  false-positives on all 60 `goto` examples.
- **A guard needs a floor, or it can pass by finding nothing.** The walker asserts it checked at
  least 70 examples, so a future change that breaks the walk itself fails rather than silently
  passing.
- **`TestWorkflowSkillsDirectAgentToCLIList` is the wrong table for design commands.** Its rows
  assert a store *directory* the agent must not poke directly, and design documents have no such
  directory, so a row there would have carried a meaningless field and diluted an existing
  guardrail. A sibling test in the same style covers the design commands instead.
- **A wrong placeholder renders literally rather than failing.** `{{spec_name}}` is valid in spec
  step templates (32 uses) but plan templates use `{{plan_name}}`; using the wrong one produces
  a literal `{{spec_name}}` in the rendered instruction, visible only to a reader. The negative
  guard asserting no `{{` survives rendering is what catches that class.

### 2026-09-20 - Phase 3.3: Bring the end-to-end suites back in step

**What was done**: Every hand-maintained expectation was checked against the code rather than
assumed. Step orders, skill retrievals, spawn steps, scaffold leftovers, plan sections and both
`solve.sh` goto sequences were confirmed unaffected, as designed. A `design ref list` oracle was
added to the plan suite's discovery window. Both affected suites were run: the plan suite passed
92/92 and the spec suite 45/45.

**Deviations**: Two, both repairs of drift this feature did not cause, made because this phase's
third criterion requires the suites to pass rather than record a failure as pre-existing. The
plan suite's status-lifecycle oracle asserted `status: in-progress` / `completed`, a vocabulary
replaced by `document_status: draft` / `final` some features ago; repaired and then validated by
the passing run. And the spec suite's agent timeout was raised 900s to 1200s, with the user's
explicit agreement, after a run missed by 0.4%.

**Files changed**:
- `spektacular: tests/harbor/plan-workflow/tests/test_plan_workflow.py`
- `spektacular: tests/harbor/spec-workflow/task.toml`

**Discoveries**:
- **The new prose obligation demonstrably lands with a real agent.** The plan run shows the
  agent ran `design ref list` inside the discovery window unprompted, and the spec run shows it
  ran `design sources` exactly once out of 17 tool calls and moved on. Contract tests can only
  prove an instruction contains the words; this proves the instruction is followed and does not
  cause probing loops.
- **`make harbor-test-*` exits 0 even when the suite fails**, because the Makefile's last action
  is `@cat` of the verifier output. Read the summary line, never the exit code. Piping the
  command through `tail` hides it a second way, by substituting `tail`'s exit code.
- **This suite's runtime varies about twofold between identical runs**, 446.5s and 903.5s
  observed. A budget inside that spread fails intermittently, and the run that crosses the line
  says nothing about what changed. The first diagnosis blamed the most recent prose edit and was
  wrong; the second data point corrected it. Measure elapsed time in the transcript before
  attributing a timeout to a change.
- **A timeout is the worst kind of hand-maintained expectation**, because it degrades gradually.
  A stale literal fails outright the moment it is wrong, but a budget that has become marginal
  gives no signal at all until a run happens to cross it, and then implicates whatever changed
  most recently.
- **A statically invalid fixture can be valid in use.** The implement suite's seeded
  `config.yaml` carries no `schema:` key and fails `upgrade_required` when read directly, which
  reads as drift. It is not: that suite runs `init` first, which stamps the current schema while
  preserving the file. Verified before concluding, and left unchanged.

### 2026-09-20 - Phase 4.1: Explain design documents on the documentation site

**What was done**: A new concept page at `/design-documents/`, built from the site's existing
Hero, Section, Prose, CtaBanner and Button components across seven bands whose shading
alternates from plain: what a design document is, when to reach for one instead of a spec
section, how designs relate to specs and plans, declaring where they live, the command surface
with worked examples and a sample `ref list` response, what an unresolvable reference looks
like, and why it works this way. Added to the Resources group in the site navigation after
Projects.

**Deviations**: One. The plan's content outline called for the when-to-use rule of thumb "as a
short table". `.spek-body` in `src/styles/global.css` carries no table styling, so a markdown
table would have rendered as a bare unstyled HTML table, and adding table CSS is a site-wide
styling change well outside this phase. The rule is presented as bold-lead bullets instead, the
shape the `ConfigKey` bodies already use and the stylesheet already handles.

**Files changed**:
- `docs: src/pages/design-documents.mdx`
- `docs: src/components/Nav.astro`

**Discoveries**:
- **Check the stylesheet before promising a layout in a plan.** The content outline specified a
  table for a page on a site that styles no tables. The outline was written from the content's
  shape rather than the site's capabilities, and the gap only appears when someone tries to
  build it.
- **`astro check` reports 1 pre-existing hint**, `document.execCommand` in a copy-button
  component. The acceptance criterion asks for no errors and no warnings, which is met; the hint
  predates this work and is not on this page.

### 2026-09-20 - Phase 4.2: Document the new settings key in the configuration reference

**What was done**: A `<ConfigKey name="design" type="section">` entry in the project settings
key list, placed between the existing `knowledge` and `repos` entries and following that
entry's exact shape: a short statement of what the section is for, a bullet per sub-key, and a
link out to the concept page rather than a restatement of the concept. The worked settings
example gained the matching `design:` block with the relative-location rule in an inline
comment, and the stated top-level key count was corrected from twelve to thirteen with `design`
added to the list. The repository keys block deliberately gets no design entry, because a
repository does not declare design sources.

**Deviations**: None.

**Files changed**:
- `docs: src/pages/configuration.mdx`

**Discoveries**:
- **The sub-key bullet needed one fact the knowledge entry's equivalent does not carry.** A
  design source's location may resolve outside the project and is written back exactly as
  declared, never re-expressed. That is the difference between a design source and a store
  directory, and the configuration reference is where someone comparing the two keys will look
  for it.
- **Anchoring on component boundaries rather than line numbers was necessary, not cautious.**
  That file carries roughly 130 uncommitted lines from other in-flight work, so every line
  number in the plan was already stale; the `<ConfigKey>` boundaries the plan named were exactly
  where it said they would be.

### 2026-09-20 - Phase 4.3: Keep the repository's own configuration docs in step

**What was done**: `README.md` at four sites. The feature list gains a design-documents bullet
beside the knowledge-base one, so the concept is introduced where the project's other
capabilities are. The two-file configuration split now states that `config.yaml` holds the
design sources the project declares. The worked project settings example gains the `design:`
block. And the one-base relative-path paragraph covers a `design.sources` entry's
`config.location` alongside the existing keys, with the deliberate difference spelled out: a
design source's location may resolve outside the project and is written back as declared rather
than re-expressed, because it points at a folder the team already keeps.

**Deviations**: None. `CHANGELOG.md` was left alone, as the phase notes require: the implement
workflow stopped writing it some plans ago.

**Files changed**:
- `spektacular: README.md`

**Discoveries**:
- **The two descriptions of one settings file drift in opposite directions.** The README
  explains the format in prose while the docs site explains it as a key reference, and the same
  fact has to be stated differently in each. The difference worth stating in both was the same
  one: that a design source location is exempt from the project-root confinement every store
  directory has, which is the question a reader of either document will actually have.
- **`cmd/docs_test.go` asserts README phrases are present and that superseded shapes are
  absent.** Additive prose passes it untouched, but the absence half means a future edit that
  reintroduces an old config shape into the README fails there rather than in review.
