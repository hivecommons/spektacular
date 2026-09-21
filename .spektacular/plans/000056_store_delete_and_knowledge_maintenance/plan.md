---
created_date: "2026-09-21"
document_status: final
closed_date: "2026-09-21"
---

# Plan: 000056_store_delete_and_knowledge_maintenance

<!-- Metadata -->
<!-- Created: 2026-09-21T09:17:27Z -->
<!-- Commit: 8520f82 -->
<!-- Branch: f-migrate -->
<!-- Repository: git@github.com:jumppad-labs/spektacular.git -->

## Overview

Knowledge entries and design documents become removable through Spektacular's own CLI, closing the
last gap that forced an agent finding an obsolete entry to reach past the tool and delete the file
itself, a workaround that stops working the moment a store is backed by anything other than a local
directory. Removing a design that specs still reference is refused rather than allowed to leave a
dangling pointer, and on that removal the knowledge skill gains the capability the gap was blocking:
reviewing a knowledge base for whether its entries are still *true*, not merely whether they are
well labelled. Anyone maintaining a Spektacular project benefits, because the knowledge base can now
be corrected and shrunk rather than only grown.


## Conventions

- **Error messages must describe the problem and suggest remediation (`spektacular`)** — this feature is mostly refusals: an undeclared store or source, a design a spec still references, a generated category description, and a report of unreachable tags. Every one is built with `output.NewError(code, message).WithNextAction(<runnable step>)`, never a bare `fmt.Errorf`, and each next action names something the agent can actually run: the declared store or source names, a `design ref remove` per referencing spec, and the `init` that regenerates a category description. The whole point of the feature is that an agent hitting a refusal corrects itself rather than reaching past the CLI for `rm`.
- **Validation belongs in the layer that holds the facts (`spektacular`, gotcha)** — the corollary of the rule above, and it decides where each new check lives. The undeclared-store and undeclared-source refusals stay in `internal/knowledge` and `internal/design`, which are the only things that know the declared names; the referenced-design and category-description refusals stay in the `cmd` layer, which is the only place the lifecycle block and the category registry are both in hand. No check moves up to the command surface for tidiness.
- **Spektacular's own files are written through Spektacular, never with file tools (`spektacular`)** — the convention this feature exists to make satisfiable. It currently names five stores and no delete verb, so the managed `templates/agents/store-access.md` section and this entry are both part of the surface that must end up saying removal is a CLI verb too. Nothing in this plan may instruct an agent to `rm` a managed file.
- **Tests must not depend on execution order (`spektacular`)** — the change registers two new cobra subcommands with their own flags on package-global command trees, and `make test` runs `go test -shuffle=on ./...`. Every new `cmd` test goes through `resetRootCmd`/`runRootCmd` rather than a per-command reset. Sharpened here by `internal/knowledge/set_test.go:500-549`, which mutates the `Categories` registry in place: any new registry-driven test must restore it in a `defer` for the same reason.
- **Passing tests are required before calling work done (`spektacular`)** — `go test ./...` must be green at every phase boundary. The baseline was confirmed green at `8520f82` before this plan started, so any failure during implementation is attributable to this work and is not to be dismissed as pre-existing.
- **MDX authoring conventions (`docs`)** — the two documentation pages are edited in place with named-block components and native slot content only: no `<div>`, `<section>` or `class=` in a page body, a blank line either side of every slot body, and fenced markdown code blocks rather than string props. The new command examples are ```bash fences and the refusal example a ```json fence, exactly as the surrounding prose already does.
- **Site layout conventions (`docs`)** — the additions are new prose and fences inside existing `Section` + `Prose nested` blocks, reusing the five components both pages already import. No new component, no new heading size, no per-page tuning.
- **Label before filename in file-scoped reference headings (`docs`)** — applies only if a new heading is introduced; the planned additions sit inside existing sections, so this constrains rather than directs, and is listed so an implementer tempted to add a heading knows the form.
- **No em dashes (`docs`)** — binding on every word written into the docs repo: the two page edits and that repo's commit text. Note this convention is repo-scoped, so it does not bind the `spektacular` repo's own prose.
- **Plans must sketch content structure, not just summarize it (`docs`)** — the documentation phases carry a `**Content example**` block giving the prose and fences to be added, with the command shapes, JSON payloads and error codes fixed by this plan's research rather than left for the implementer to invent.
- Deliberately dropped: **Alternate section background shading (`docs`)** — no new `Section` band is added, so there is no alternation to set or disturb. The **glossary** entries in both stores are the category-README scaffolds with no project terms defined, so there is no vocabulary to honour beyond the terms this plan defines itself (and those READMEs are, fittingly, among the things this feature removes from retrieval).


## Architecture & Design Decisions

Removal becomes a verb on the two document families that lack it, and it is built where those
families already keep their addressing rather than where the other three keep theirs. Knowledge
gains `Set.Delete(addr, path)` in `internal/knowledge`, composing the existing `Set.resolve` gate
so an undeclared store is refused with the names available in that tier before anything is
touched; design gains `Set.Delete(doc)` in `internal/design`, composing `Set.lookup` and the same
empty-path and read-only-provider checks `Set.Write` already runs. Both call the `Delete` the
storage layer has always declared (`store.Writer`, documented as returning nil when the file is
absent), so the spec's idempotent-removal requirement falls out of the existing contract instead
of being built, and removal keeps working the day a source is backed by something other than a
local directory. Neither family is folded into `newStoreFileCmd`, the factory that gives `spec`,
`plan` and `changelog` their delete: that factory stamps and re-reads Spektacular's lifecycle
frontmatter on every write and listing, which is precisely why the design commands were
hand-written in the first place, and adopting it to gain one verb would drag the whole write path
with it. The two new commands are therefore siblings of `design write` and `knowledge write`,
addressed exactly as those are (`{source, path}` and `{tier, name, path}`), introducing no new
way to name a document.

Three refusals carry the design's weight, and each is placed in the layer that holds the facts its
remediation is built from, per `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`. An
undeclared store or source is refused by the domain set, which is the only thing that knows the
declared names; that is the one addressing failure that errors, while a valid address holding no
document succeeds and changes nothing. A design a spec still references is refused in the command
layer, because that is where the lifecycle block is understood: an authored design carries its
referencing specs in its own `specs:` list, maintained transactionally with the spec side by the
reference verbs, so the check is a single read of the document rather than a scan of the spec
store, and it names every spec in the refusal with a runnable `design ref remove` for each. A
design the project did not author carries no such block, so the check finds nothing and the delete
proceeds — which is the correct answer, not an oversight. Nothing about the refusal touches a
spec: clearing references stays an explicit, separate act, and the document is still there
afterwards byte for byte. A generated category description is refused by `knowledge delete` for a
different reason: it is a descriptor rendered from the category registry, not knowledge, and
nothing restores one. Because that refusal must name a working way to regenerate it, and because
`EnsureFootprint` currently writes a category README only when it is *absent*, the footprint write
is changed to rewrite one whose bytes differ from `Category.README()` — making `init`, which
cascades over every registered repo, the single remedy both this refusal and the drift report can
honestly point at.

Keeping category descriptions out of retrieval is one rule with one home and two applications. The
category registry in `internal/knowledge/category.go` already owns what a category *is* and which
categories are always-applied; it gains the statement that a category's `README.md` is its
description rather than an entry. That predicate is then applied at the two places the existing
always-applied exclusion is already applied: the post-merge filter in `Set.Search`, and the file
loop in `readCategories` that builds the always-applied payload. It is deliberately *not* applied
inside `listFiles` or inside `FileStore.search`. `listFiles` also backs `List` and `Tags`, and the
new drift report needs `knowledge list` to keep enumerating the READMEs so it can read and compare
them; and the store layer is documented as category-agnostic, so a second exclusion rule living
there would be a rule in two layers waiting to drift. The related tag defect is reported rather
than fixed by weakening anything: writing an entry into an always-applied category whose tags a
search can never reach still writes the entry, and returns the unreachable tags plus a runnable
next step on the success envelope — the shape `design ref list` already established for reporting
without failing. The existing exclusion of always-applied categories from search and from the tag
vocabulary is untouched: the spec puts reporting its consequence in scope and changing it out of
scope, and this keeps to that line.

Maintenance is prose, not code, and lands as a fifth intent in the existing `spek-knowledge`
skill alongside lookup, contribute, update and audit. It composes commands that already exist plus
the new `knowledge delete`, and inherits the audit intent's shape wholesale: read and propose
only, one entry at a time, explicit confirmation per entry, no bulk operation and no second write
path. What it adds is the judgement the audit deliberately excludes — whether an entry is still
*true* rather than whether it is well labelled — and the single classification rule that decides
whether this feature helps or harms: an entry stating a standard the code has not yet met is
`current`, not `stale`, because in this project an entry states the target and the code is what
has yet to meet it. Only an entry whose subject no longer exists is stale, every staleness or
incorrectness verdict must name the specific file, command or behaviour that changed, and "looks
old" is not a finding. Because the skill's behaviour is prose, its regression tests are phrase
assertions on the rendered file, which is also why the branch-count sentences, the hand-maintained
set of registered `knowledge` subcommands, and the decline-handling carve-out all move in this same
change. The documentation half of the feature is carried in the `docs` repo, extending the
knowledge base page's entry-lifecycle section and the design page's command section, with the
referenced-design refusal documented in the JSON shape that page already uses for `design_not_found`
and an explicit sentence separating removing a design from removing a reference to one.

Rejected alternatives, with citations, are recorded in
`research.md#alternatives-considered-and-rejected`: adopting the shared store-file factory, a
single generic cross-store delete verb (refused by the constraint against inventing a new
addressing scheme), cascading a design delete into its referencing specs, leaving references
dangling, finding referencing specs by scanning the spec store, excluding READMEs in `listFiles`
or in the store's own walk, reusing `.spektacular_ignore` for the exclusion, a Go command for
maintenance, and leaving category-README regeneration unimplemented while pointing refusals at a
command that cannot perform it.


## Component Breakdown

**Knowledge set (changed: knowledge domain, `spektacular`).** The component that turns an address
into exactly one configured store gains removal alongside read and write. It owns resolving the
address through the single existing gate, so an undeclared store is refused with the names
available in that tier before any store is touched, and it owns nothing else about deletion: the
storage layer already guarantees that removing something absent succeeds. It also becomes the home
of the rule that a category's own description is not a retrievable entry, applying that rule at
the two points where it already applies the always-applied exclusion — the ranked search result and
the always-applied payload — and deliberately not at the plain listing or the tag vocabulary. It
gains one further read-only judgement: given an entry's destination and its declared labels, it
reports which of those labels no search will ever reach, because it is the only component that
knows both the retrieval tier of the destination category and the labels the entry carries.

**Category registry (changed: knowledge domain, `spektacular`).** The single declaration of the
category model already states each category's purpose, boundary, retrieval tier and entry shape,
and already renders the self-documenting description written into every category directory. It
gains one statement: that the rendered description is a descriptor rather than an entry. That
statement is what the knowledge set's retrieval exclusion consults, what the delete refusal
consults to recognise an attempt to remove a descriptor, and what a drift check compares a stored
description against. Keeping it here rather than restating the filename in three places is what
stops the three behaviours drifting apart, exactly as the existing retrieval-tier declaration does.

**Design set (changed: design domain, `spektacular`).** The component that resolves declared design
sources gains removal alongside read, write and existence. It owns the same sequence its write
already owns: refuse an address naming a source the project has not declared, listing the declared
names; refuse an incomplete address; refuse a source whose backend cannot be written to, by name
rather than by an undefined outcome. It knows nothing about lifecycle records or about specs, and
must not learn: the referenced-design question belongs a layer up.

**Knowledge command family (changed: `spektacular`).** The agent-facing surface gains a removal
verb addressed exactly as read and write are, and its existing write verb gains the reporting of
unreachable labels on its success result. The family owns argument and payload parsing, schema
introspection for the new verb, and turning the knowledge set's results and refusals into the
project's standard envelope. It owns one refusal of its own, because it is the only layer holding
both the category registry and the name of the command that regenerates a descriptor: an attempt
to remove a generated category description is refused with that command as the next step.

**Design command family (changed: `spektacular`).** Gains a removal verb beside write and author,
and owns the refusal that is the heart of the feature: a design a spec still references is not
removed. It performs that check as a single read of the document's own lifecycle record, which
carries the list of referencing specs, and it names every one of them in the refusal with a
runnable step to clear each reference. It owns the guarantee that the refusal path changes nothing
at all: the document is left byte for byte as it was and no spec is touched. A document with no
lifecycle record has no referencing specs to find, so it is removed without further conditions.
This family stays deliberately separate from the shared artifact-file command factory for the
reason already recorded on it, and removal does not change that.

**Repo footprint scaffolder (changed: `spektacular`).** The routine that creates or repairs a
repo's minimal Spektacular footprint currently writes a category's description only when none is
there. It gains the narrower obligation to bring a description that has drifted from the registry
back into line, so that regenerating a descriptor is something a caller can actually do. This is
what makes the delete refusal's and the maintenance drift report's remedies true rather than
aspirational. It is a change to when the write happens, not to what is written, and it touches no
knowledge entry.

**Knowledge skill (changed: agent instruction surface, `spektacular`).** The static playbook the
agent follows for ad-hoc knowledge work gains a fifth branch for maintenance, alongside lookup,
contribute, update and audit. It owns the judgement this feature is ultimately for: whether an
entry is still true, classified as current, stale, incorrect or unverifiable; the rule that an
entry stating a standard the code has not met is current rather than stale; the requirement that
every staleness or incorrectness verdict names the specific thing that changed; and the reporting
of a category description that no longer matches the project's definition of that category. It
composes only commands that already exist plus the new removal verb, adds no bulk operation, and
inherits the per-entry propose-then-confirm contract unchanged, including that declining one entry
neither writes anything nor ends the review. Its preamble, which tells the agent how many branches
it has, is part of the component: a branch the preamble does not mention is never reached.

**Managed agent-guidance section (changed: agent instruction surface, `spektacular`).** The
standing rule that every file Spektacular manages is reached through its CLI currently names read
and write and is silent on removal, which is the gap that made reaching for a raw file operation
look permissible. It gains removal explicitly, so no instruction anywhere tells an agent to delete
a managed file with its own tools.

**Knowledge base documentation page (changed: `docs`).** The page explaining what the knowledge
base is and how an entry is created, retrieved and revised gains removal as part of that
lifecycle, including that an address naming an undeclared store is refused while a path holding
nothing succeeds, and that a category's own description cannot be removed.

**Design documents documentation page (changed: `docs`).** The page explaining design documents and
how they are reached from the command line gains removal beside the other document verbs, the
refusal for a design a spec still references shown in the error shape the page already uses, and an
explicit statement separating removing a design from removing a reference to one — the distinction
that currently traps anyone who finds the reference verb first and believes it is the missing
delete.

**Repository reference documentation (changed: `spektacular`).** The repository's own README
command list and knowledge-base document command table enumerate the verbs each family offers.
They own nothing behavioural, but they are wrong the moment a verb exists that they do not list,
so they move with the commands.


## Data Structures & Interfaces

No new artifact format, configuration key or persisted schema is introduced. Nothing about a
knowledge entry, a design document, a spec's recorded references or a project's settings changes
shape, and the settings format version does not rise. What follows is therefore entirely
in-process contracts plus the JSON each new command publishes.

**Removal on the two domain sets.** Each family gains one method mirroring the read and write it
already has, addressed the way that family already addresses a document:

```go
// internal/knowledge
func (s *Set) Delete(addr Address, path string) error

// internal/design
func (s *Set) Delete(d Document) error
```

Both resolve the address through the family's existing single gate, so an undeclared store or
source is refused there with the declared names, and both then call the `Delete` already on
`store.Writer`. Neither introduces an error of its own: every refusal they can produce is one the
same family's read or write already produces today. The design method additionally refuses a
source whose provider cannot write, by name, exactly as its write does.

**What a category description is.** The category registry gains the one statement three behaviours
consult, so that renaming or re-tiering stays a single-field change:

```go
// internal/knowledge
const CategoryDescriptionFile = "README.md"

// IsCategoryDescription reports whether a store-relative path addresses a
// category's own generated description rather than a knowledge entry. True
// only for "<registry category>/README.md" exactly: a README deeper inside a
// category is a contributor's own file and is treated as an entry.
func IsCategoryDescription(path string) bool
```

**Label reachability.** A pure function over the registry and an entry's own frontmatter, returning
`nil` when every declared label is reachable, so the common case adds nothing to the write's
result:

```go
// internal/knowledge
type TagReachability struct {
    Category    string   `json:"category"`
    Unreachable []string `json:"unreachable_tags"`
    NextAction  string   `json:"next_action"`
}

func UnreachableTags(path string, content []byte) *TagReachability
```

It lives beside the registry rather than in the command layer because its next action has to name
the destination's retrieval tier and the categories a search does reach, which only the registry
knows. It is deliberately not a method on the set: it consults no store and must stay callable
without one.

**Two new refusal codes**, each following the family's existing naming and each carrying a runnable
next action:

| Code | Raised when | Next action names |
| --- | --- | --- |
| `design_referenced_delete` | a design document's lifecycle record lists one or more specs | one `design ref remove` invocation per referencing spec, then the same delete |
| `knowledge_category_description_delete` | the addressed path is a category's generated description | the command that regenerates it |

A third change is corrective rather than new: the shared knowledge `--data` parser currently
refuses a missing payload with a bare error carrying no code and no next action, which the new verb
would inherit. It gains `knowledge_data_required` with an example payload and a pointer to the
command that lists the store names, bringing the existing read and write verbs onto the project's
error convention with it.

**Command envelopes.** Both removal verbs publish input and output schemas, as every other verb in
their family does. Input is each family's existing address payload, unchanged:
`{"tier","name","path"}` for knowledge and `{"source","path"}` for design. Output distinguishes the
two successful outcomes without making either an error:

```jsonc
// knowledge delete
{ "error": false, "tier": "repo", "name": "docs",
  "path": "gotchas/db-timeouts.md", "deleted": true }

// design delete
{ "error": false, "source": "api", "path": "payments/v2.md",
  "location": "/work/design/api/payments/v2.md", "deleted": false }
```

`deleted` is `true` when a document was there and was removed and `false` when the address was
valid and held nothing. Both are successes, and an agent retrying a maintenance pass can tell them
apart without parsing prose.

**The write envelope gains two optional fields.** `knowledge write` keeps its existing
`{"tier","name","path"}` result and adds `unreachable_tags` and `next_action` only when the entry
declares labels no search can reach. Their absence is the normal case and means nothing is wrong,
so no existing consumer changes.

**Maintenance introduces no type at all.** The classification vocabulary — `current`, `stale`,
`incorrect`, `unverifiable` — and the drift report are prose in the knowledge skill, produced by
composing commands that already publish their own shapes: the category listing supplies each
category's current purpose, boundary, retrieval tier and entry shape, and a read of a stored
description supplies what to compare against them. Nothing is serialised, so there is nothing for a
Go type to constrain, and adding one would put judgement in a layer that is deliberately prose.


## Implementation Detail

**No new pattern is introduced in Go.** Every part of the behavioural work follows a shape the
codebase already uses, and a reviewer should judge it on whether it follows those shapes faithfully
rather than on whether it invents a better one. A removal method on each domain set is the existing
read-and-write shape with a third verb: resolve the address through the family's single gate,
refuse there if it cannot be resolved, then call the storage layer. A removal command is the
existing addressed-command shape: publish a schema, parse the payload, build the set, call it,
render an envelope. The one deliberate divergence from an existing shape is that the two new verbs
return a result envelope where the three older delete verbs return silence, and that divergence is
the point rather than an inconsistency — the spec requires an idempotent removal to report success,
and a silent exit does not.

**The module boundaries do not move, and the temptation to move them is the thing to resist.** Two
packages will now carry near-identical delete methods over the same storage interface, which reads
like an invitation to hoist a shared abstraction. It is not. The two packages already parallel each
other without sharing code beyond the store layer, for reasons recorded on the design package
itself: knowledge carries tiers, a category registry, ranking, tag vocabularies and de-duplication,
none of which a design document has or may have. A shared delete would be the first strand of an
abstraction the project has twice decided not to build. The same applies to the artifact-file
command factory: it stays a three-family factory, and the two new commands stay hand-written
siblings of their families' existing verbs.

**Refusal placement is the design, and it is visible in the code shape.** A reader tracing any
refusal in this feature should find it in the layer that owns the facts its next action is built
from, never one layer up for tidiness. That puts undeclared-store and undeclared-source refusals in
the domain sets, which alone know the declared names; it puts the referenced-design refusal in the
design command family, which alone understands the lifecycle record; and it puts the
category-description refusal in the knowledge command family, which alone holds both the category
registry and the name of the command that regenerates a descriptor. The consequence a reviewer
should check for is that no refusal's next action is generic: the referenced-design refusal names
every referencing spec and a runnable step per spec, not "clear the references first".

**One rule, one home, two applications.** The category-description exclusion is stated once, in the
registry that already declares the category model, and consulted at the two retrieval surfaces that
already consult that registry for the always-applied exclusion. A developer reading either surface
will see two adjacent one-line exclusions with a shared origin, which is the intended experience;
what they must not see is a filename literal repeated at each site, nor a third exclusion appearing
in the store layer, which is documented as category-agnostic and must stay that way. The plain
listing and the tag vocabulary are deliberately left alone, and that asymmetry is load-bearing
rather than an oversight: the maintenance review needs the listing to keep enumerating
descriptions in order to check them.

**The footprint scaffolder's guarantee is narrowed from additive to convergent.** It presently
promises never to touch an existing knowledge file, and the category description is the one file
that must stop enjoying that promise, because it is generated output rather than content. The
change is small and must be visibly bounded: it applies to the generated description and to nothing
else in the store, it compares against the registry's own rendering rather than against a heuristic,
and it leaves a description that already matches untouched so that a repeated run still reports no
change. This is the one place the plan alters an existing guarantee, and it exists so that two
refusals elsewhere can name a remedy that actually works.

**The largest surface is prose, and its tests are phrase assertions.** The maintenance intent, the
managed agent-guidance sentence about removal, and the two documentation pages are all behaviour
expressed as words, which in this project means their regression tests assert that specific anchor
phrases are present or absent in the rendered output. Three consequences shape the work. The
skill's preamble counts its own branches, so a fifth intent is a coordinated edit to the preamble,
the trigger list and the branch-count assertions, not an insertion; a branch the preamble does not
advertise is never reached. The hand-maintained set of registered knowledge subcommands, which
exists to catch a skill inventing a command, must gain the new verb or it will reject a legitimate
invocation. And the decline-handling contract, which currently carves out per-entry behaviour for
the audit alone, must carve out the same for maintenance, because maintenance is the first intent
that can remove something rather than only rewrite it.

**The classification rule is the riskiest thing here and is written as a rule, not a hint.** The
distinction between an entry that has gone stale and an entry that states a target the code has not
met is the difference between a maintenance pass that improves a knowledge base and one that
deletes the entries doing the most work. It is expressed as an explicit, non-negotiable
classification instruction with a worked example of the failure mode, and paired with the
requirement that every staleness or incorrectness verdict names the specific file, command or
behaviour that changed. "Looks old" is not a finding, and an entry the code disagrees with is
evidence of work to do, not evidence against the entry.


## Dependencies

**Design documents this plan was built on: none.** The spec carries no design references —
`design ref list` for it reports an empty list and zero unresolved. Stated explicitly because an
unremarked absence reads as a step that was skipped rather than a fact that was checked.

**Runtime and package dependencies (all internal, all already present):**

- **Storage layer (`internal/store`, `spektacular`)** — supplies the `Delete` both new verbs call,
  already part of the writer contract every writable provider must satisfy and already documented
  as succeeding when the file is absent. **No change needed**, and that is the load-bearing fact:
  it is why removal keeps working for a provider backed by something other than a local directory,
  and why the idempotent-removal requirement costs nothing to satisfy. Its ignore-aware wrapper
  already forwards the call.
- **Knowledge domain (`internal/knowledge`, `spektacular`)** — supplies address resolution and the
  refusals that name the stores available in a tier. **Changed**: gains removal, the statement that
  a category's generated description is a descriptor, the retrieval exclusion that consults it, and
  the label-reachability report.
- **Design domain (`internal/design`, `spektacular`)** — supplies source resolution and the
  refusals that name the declared sources. **Changed**: gains removal. It gains no knowledge of
  lifecycle records or specs, which stay a layer up.
- **Artifact metadata (`internal/metadata`, `spektacular`)** — supplies the parser that turns a
  design document's bytes into the lifecycle record carrying its referencing specs. **No change
  needed**; the referenced-design check is a read of what this already produces.
- **Repo footprint scaffolder (`internal/repo`, `spektacular`)** — creates and repairs a repo's
  minimal footprint, including a description for every category. **Changed**, narrowly: it must
  bring a description that has drifted from the registry back into line, not only create one that
  is absent. This is what gives the two new refusals a remedy that works.
- **Project initialisation (`internal/project`, `spektacular`)** — already rewrites project-tier
  category descriptions unconditionally and already cascades the footprint routine over every
  registered repo. **No change needed**; it becomes the single remedy purely by virtue of the
  footprint change above.
- **Output envelope (`internal/output`, `spektacular`)** — supplies the refusal builder every new
  error uses and the success writer both new verbs render through. **No change needed**; a
  non-fatal report travels as fields on a success result, a shape the design reference listing
  already established.
- **Command surface (`cmd`, `spektacular`)** — supplies the two command families, their payload
  parsers and their schema introspection. **Changed**: two new verbs, one corrected refusal in the
  shared knowledge payload parser, and the label report rendered onto the write result.
- **Agent instruction surface (`internal/agent` and `templates`, `spektacular`)** — supplies the
  skill renderer that substitutes the configured command name into every skill, and the managed
  agent-guidance sections. **Changed**: the knowledge skill gains a fifth intent and the
  store-access guidance gains removal. No change to the renderer itself. Note the committed,
  dogfooded copy of the skill is regenerated by the migrate command, so the template edit and the
  regenerated file land together.
- **Cobra (`github.com/spf13/cobra`)** — the only external library involved, already a direct
  dependency, supplying the subcommand and flag registration both new verbs use. **No change, no
  version bump**; no new external dependency is introduced by any part of this work.
- **Astro 5, MDX and Tailwind v4 (`docs`)** — already power the documentation site. **No change**:
  the two page edits add prose and fenced blocks inside existing section components and need no new
  component, no new import and no navigation entry.

**Planning dependencies — all already landed; nothing must ship before this plan starts:**

- **Spec `000056_store_delete_and_knowledge_maintenance`** — the source of truth for scope,
  requirements, constraints and success metrics. Already final.
- **Plan/spec `000054_project-level-design-documents`** — introduced design sources, the design
  command family, and the split of the storage interface into a read half and a write half that
  makes a future read-only provider a named refusal. This plan builds directly on all three and
  changes none of them.
- **Plan/spec `000055_design-authoring-skill`** — introduced the authored design document, its
  lifecycle record, and the spec back-link list that the referenced-design refusal reads. Without
  it there would be nothing to check and the refusal could not exist in this form.
- **Plan/spec `000050_knowledge-entry-tags`** — introduced entry labels, the label vocabulary
  command, and the deliberate exclusion of always-applied categories from both search and the
  vocabulary. That exclusion is the direct cause of the unreachable-label defect this plan reports,
  and the spec forbids weakening it.
- **Plan/spec `000047_repo-scoped-knowledge-addressing`** — introduced the tier-and-name addressing
  both the new knowledge verb and its refusals inherit unchanged.

**Tooling the verification of this plan depends on:** the Go toolchain with the shuffled test run
for the `spektacular` repo, and the site's build plus its type check for the `docs` repo. The
end-to-end harbor suites were examined and are not affected: the only suite naming a knowledge or
design command pins step-instruction text that this work does not change. That is recorded as a
checked fact rather than an assumption.


## Testing Approach

The project's tests sit in three layers and this feature touches all three: Go unit tests for
deterministic mechanics, template-contract tests that assert anchor phrases in rendered agent
instructions because that is where prose-driven behaviour lives, and the end-to-end harbor suites.
Harbor is examined and unaffected — the only suite naming a knowledge or design command pins
step-instruction text this work does not change — so no harbor oracle moves and no harbor run is
required for this plan. That is a checked finding, not an omission.

**Where the coverage concentrates, and why.** The heaviest coverage goes on the refusals, for the
same reason the feature exists: an agent handed a vague refusal abandons the CLI and reaches for a
raw file operation. Every refusal is asserted on three things together — the code an agent branches
on, the message naming what went wrong, and the **content** of the next action, never merely that a
next action is non-empty. A test for non-emptiness passes on exactly the message the project's error
convention exists to prevent, and the repository has already been bitten by a check that sat one
layer too high to say anything useful. The referenced-design refusal gets the most attention of
all: it is asserted to name every referencing spec, to give a runnable step per spec, to leave the
document byte for byte unchanged, and to leave every referencing spec's recorded references
untouched.

**The second concentration is on the classification rule in the maintenance review**, which the
spec identifies as the riskiest part of the feature. Because that behaviour is prose, its regression
tests are phrase assertions on the rendered skill: that the intent exists and is reachable from a
preamble advertising the right number of branches, that it carries the vocabulary of four verdicts,
that it states the rule separating an unmet target from an obsolete entry, that it requires every
staleness verdict to name what changed, and that its per-entry confirmation and decline handling are
present and scoped to one entry. These anchors are hand-maintained literals asserted against output
rendered through the production install path, never derived from the template under test, since an
assertion built from the file it checks passes whatever that file says.

**Unit coverage** lands on the two domain sets and the category registry: that removal resolves
through the same gate as read and write and refuses an undeclared store or source with the declared
names listed; that removing from a valid address holding nothing succeeds and changes nothing; that
the category-description predicate matches a generated description and not a contributor's own
README deeper in a category; that the exclusion removes descriptions from ranked search results and
from the always-applied payload while leaving the plain listing and the label vocabulary exactly as
they are; and that the label-reachability report names the unreachable labels for an always-applied
destination and returns nothing for a looked-up one. The existing always-applied exclusion tests are
the sibling set these join, and none of them may weaken.

**Command-level coverage** exercises both new verbs end to end through the real command tree: the
success envelope for each outcome, the published input and output schemas, the refusal paths, and
the behaviour outside a project. Every such test goes through the shared command-tree reset, because
commands are package globals and the suite runs shuffled; a test that mutates the category registry
restores it for the same reason.

**A deliberate backfill.** The three existing delete verbs share one implementation and have never
been tested. The spec constrains their behaviour not to change, and nothing currently enforces that,
so characterisation tests are added for them. They are not a scope extension: they are what turns a
stated constraint into something the suite checks, and they give the two new verbs a documented
baseline to differ from where the spec requires a difference.

**A deliberate gap.** The documentation pages get no assertion on their prose content. The
repository's own reference documents are already swept by existing contract tests and will be kept
in step with them, but the site's pages are verified by their build and type check rather than by
phrase assertions, matching how every prior documentation change in this project has been verified.
Adding phrase oracles for marketing-site prose would be a maintenance cost without a corresponding
failure mode.

**Success metrics, each made verifiable:**

- *No agent reaches past the CLI to remove a managed file.* **Partly behavioural, partly manual.**
  Behavioural: a contract assertion sweeping the whole agent-facing instruction corpus — every
  rendered skill, every step instruction and every managed guidance section — for any instruction to
  remove a managed file with a raw file operation, alongside the positive assertion that the
  store-access guidance names removal as a CLI verb. This extends a sweep the project already runs
  over that corpus. The remaining half, whether agents in the wild then behave, is **manual —
  captured in the implementation test plan**.
- *A knowledge base stays accurate over time rather than only growing.* **Manual — captured in the
  implementation test plan.** It is a property of repeated use over months, not of a single run.
- *A maintenance review's staleness findings hold up when spot-checked.* **Manual — captured in the
  implementation test plan.** The load-bearing half of it is guarded automatically by the
  classification-rule phrase assertions above, but whether a real review's findings survive a
  reviewer checking the evidence can only be judged by running one and checking it.
- *Deleting a design never leaves a spec pointing at a document that is not there.* **Behavioural.**
  A test removes a referenced design, asserts the refusal, asserts the document and every
  referencing spec are unchanged, then clears the references and asserts the same delete now
  succeeds and that reference resolution afterwards reports nothing unresolved.
- *The refusal for a referenced design is acted on rather than worked around.* **Partly behavioural,
  partly manual.** Behavioural: the refusal is asserted to carry a step the agent can run verbatim,
  naming each referencing spec, and the clear-then-retry sequence is asserted to succeed — so the
  recovery path provably exists. Whether an agent takes it rather than falling back to a raw
  removal is **manual — captured in the implementation test plan**.
- *Removal works unchanged when a store is backed by something other than a local directory.*
  **Behavioural.** Both removal paths are exercised against a substitute storage implementation that
  is not the filesystem one, proving the call travels through the storage interface rather than
  around it, alongside the existing assertion that a source whose backend cannot write refuses by
  name rather than failing in an unspecified way.


## Milestones & Phases

### Milestone 1: A knowledge entry and a design document can be removed

**What changes**: The two kinds of stored document that could never be removed can now be removed,
through the tool rather than around it. An agent or a person names a knowledge entry by the store it
lives in and its path there, or a design document by its declared source and its path there, and it
is gone. Removing something that is already absent reports success and changes nothing, so a
maintenance pass that retries is safe; naming a store or source the project has not declared is
refused with the names it could have used. Two removals are refused on purpose. A design that one or
more specs still reference is refused, with every referencing spec named and a runnable step to
clear each one, and neither the document nor any spec is touched by the attempt, so a design and the
specs pointing at it can never be left disagreeing. A category's own generated description is
refused, because it is a descriptor rather than knowledge and nothing would restore it; the refusal
names the command that regenerates it, and that command is made to actually do so, since until now
a description that had drifted from the project's definition of its category could be repaired by
nothing at all. The three document kinds that could already be removed behave exactly as before.

**Validation point**: A knowledge entry removed by address no longer appears in that store's listing
or its search results. An unreferenced design document removed by address no longer appears in its
source's listing. A design referenced by two specs survives the attempt byte for byte, the refusal
names both specs and gives a step per spec, neither spec's recorded references change, and once both
references are cleared the same removal succeeds and reference resolution reports nothing
unresolved. Issuing either removal a second time reports success and changes nothing. Naming an
undeclared store or source is refused with the available names listed, and nothing is removed.
Attempting to remove a category description is refused, the file is still there, and running the
command the refusal names brings a description that had drifted back into line while leaving one
that already matched untouched. The full Go test suite passes, including new coverage proving the
three pre-existing removals are unchanged.

#### - [x] Phase 1.1: Pin the behaviour of the removals that already work

**Repo:** `spektacular`

Three kinds of stored document can already be removed, and none of that has ever been covered by a
test. This phase changes nothing: it writes down, as tests, what those removals do today, so that
the rule that their behaviour must not change becomes something the suite actually checks rather
than a sentence in a document. It goes first deliberately, so the two new removals that follow are
built against a recorded baseline rather than against an assumption about one.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-pin-the-behaviour-of-the-removals-that-already-work)

**Acceptance criteria**:

- [x] Removing a stored spec, plan and changelog record by name is covered by tests that describe
      what each one does today.
- [x] Issuing one of those removals for a document that is not there is recorded as succeeding and
      changing nothing.
- [x] No behaviour of any existing command changes in this phase.

#### - [x] Phase 1.2: Name a category's description, and make a drifted one repairable

**Repo:** `spektacular`

Every knowledge category directory carries a short blurb describing what belongs in it, generated
from the project's own definition of that category. Nothing currently says, in one place, that this
blurb is a descriptor rather than an entry, and nothing can repair one that has fallen out of step
with the definition it was generated from. Both gaps are closed here, because the removals in the
next phases need the first to recognise a descriptor and need the second so that refusing to remove
one can point at something that genuinely restores it. Setting up a project repairs a description
that has drifted and leaves an up-to-date one alone.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-name-a-categorys-description-and-make-a-drifted-one-repairable)

**Acceptance criteria**:

- [x] The project's definition of its knowledge categories states, in one place, that a category's
      generated description is a descriptor rather than a knowledge entry.
- [x] A contributor's own README placed deeper inside a category is not treated as a category
      description.
- [x] Setting up or repairing a project brings a category description that no longer matches the
      current definition back into line.
- [x] A category description that already matches is left untouched, and a repeated run reports no
      change.
- [x] No knowledge entry is altered by any of this, and no other file in a knowledge store is
      rewritten.

#### - [x] Phase 1.3: Remove a knowledge entry

**Repo:** `spektacular`

Knowledge entries become removable by naming the store they live in and their path within it, the
same way they are already read and written. Removing something that is not there reports success
and changes nothing, so a maintenance pass that retries is safe, while naming a store the project
has not declared is refused with the names it could have used. Removing a category's generated
description is refused, since it is a descriptor rather than knowledge and nothing would bring it
back on its own; the refusal names the command that regenerates it. The shared refusal for a
missing request payload, which until now carried no code and no next step, is brought onto the same
footing as every other refusal in the tool.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-remove-a-knowledge-entry)

**Acceptance criteria**:

- [x] A knowledge entry can be removed by naming its store and its path within that store, and
      afterwards appears in neither that store's listing nor its search results.
- [x] Removing an entry that is not there reports success, changes nothing, and can be repeated
      safely.
- [x] Removing from a store the project has not declared is refused by name, nothing is removed,
      and the refusal lists the store names available in that tier.
- [x] Removing a category's generated description is refused, the description is still there
      afterwards, and the refusal names the command that regenerates it.
- [x] The command reports its own input and output shape on request, as every other knowledge
      command does, and run outside a project it fails with the same explicit error they do.
- [x] A request missing its payload is refused with a code and a runnable next step rather than a
      bare message.
- [x] Removal reaches the store through the same storage abstraction reading and writing use, and
      is proven to do so against a store that is not a local directory.

#### - [x] Phase 1.4: Remove a design document

**Repo:** `spektacular`

Design documents become removable by naming their declared source and their path within it, matching
how they are already read and written. As with knowledge entries, removing something already absent
reports success, and naming a source the project has not declared is refused with the declared
names listed. A source whose storage backend cannot be written to refuses removal by name rather
than failing in an unspecified way, so the seam left for a future read-only backend keeps working.
The refusal that protects a referenced design arrives in the next phase; this phase removes a
document nothing points at.

*Technical detail:* [context.md#phase-14](./context.md#phase-14-remove-a-design-document)

**Acceptance criteria**:

- [x] A design document can be removed by naming its source and its path within that source, and
      afterwards no longer appears in that source's listing.
- [x] A document the project already had, carrying no record Spektacular wrote, is removed without
      any special condition.
- [x] Removing a document that is not there reports success and changes nothing, and can be repeated
      safely.
- [x] Removing from a source the project has not declared is refused by name, nothing is removed,
      and the refusal lists the declared source names.
- [x] An address missing either its source or its path is refused rather than guessed at, and a
      source whose backend cannot write refuses removal by name.
- [x] The command reports its own input and output shape on request, as every other design command
      does, and run outside a project it fails with the same explicit error they do.
- [x] Removal reaches the source through the same storage abstraction reading and writing use, and
      is proven to do so against a source that is not a local directory.

#### - [x] Phase 1.5: Refuse removing a design a spec still references

**Repo:** `spektacular`

The phase the whole feature turns on. A design document that one or more specs still reference is
not removed: the attempt is refused, and the refusal names every referencing spec together with a
runnable step to clear each one, so the tidy-up becomes explicit work rather than a silently broken
pointer. Nothing is modified by the refusal, in either direction: the document is left exactly as it
was, and no spec's recorded references are touched, because clearing a reference stays a separate
and deliberate act by the caller. Once the references are cleared the same removal succeeds. A
design the project handed over rather than one Spektacular authored carries no record of
referencing specs, so there is nothing to find and it is removed like any other.

*Technical detail:* [context.md#phase-15](./context.md#phase-15-refuse-removing-a-design-a-spec-still-references)

**Acceptance criteria**:

- [x] Removing a design that a spec references is refused, and the document is still there
      afterwards, byte for byte unchanged.
- [x] No spec's recorded references are altered by the refused attempt.
- [x] The refusal names every spec that references the document, not only the first, and gives a
      runnable step to clear each one followed by the removal to retry.
- [x] After the referencing specs drop their references, removing the same document succeeds and
      reference resolution afterwards reports nothing unresolved.
- [x] A document carrying no record of referencing specs is removed without this condition applying
      at all.

### Milestone 2: A category's description stops competing with the knowledge it describes

**What changes**: The self-documenting blurb sitting in every knowledge category directory stops
behaving like an entry. It no longer appears in search results, where it had been outranking real
entries on any query resembling the words every one of those blurbs contains, and it is no longer
injected into the material an agent receives on every single task, where two of them had been
occupying the context budget on every request in every session. Both stay fully visible to anyone
listing or reading a store, because they are still worth reading and because the maintenance review
in the next milestone has to be able to check them. Separately, writing an entry whose labels a
search will never be able to reach now says so, instead of storing them as dead weight: the entry is
still written, and the report names the labels and what to do about them. Nothing about which
categories are loaded on every task changes, and no existing exclusion is weakened.

**Validation point**: Searching any store returns no category description, for any query, while the
same descriptions still appear in that store's plain listing and can still be read by address. The
material returned on every task contains no category description and is otherwise unchanged,
entry for entry. A contributor's own README placed deeper inside a category is still treated as an
ordinary entry and is still found. Writing an entry carrying labels into an always-applied category
reports those labels as unreachable with a runnable next step and still writes the entry; writing
the same entry into a looked-up category reports nothing. The full Go test suite passes.

#### - [x] Phase 2.1: Keep category descriptions out of retrieval

**Repo:** `spektacular`

A category's generated description stops behaving like an entry on the two surfaces where it was
doing damage. It no longer appears in search results, where any query resembling the words every one
of those descriptions shares was returning them ahead of real entries, and it is no longer part of
the material an agent receives on every task, where two of them were consuming context on every
single request. It stays fully visible to anyone listing or reading a store, deliberately: the
descriptions are still worth reading, and the maintenance review added later has to be able to
enumerate and check them. The existing rule that always-applied categories are excluded from search
is untouched.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-keep-category-descriptions-out-of-retrieval)

**Acceptance criteria**:

- [x] Searching a store returns no category description, for any query, in any category.
- [x] The material returned to an agent on every task contains no category description, and is
      otherwise unchanged entry for entry.
- [x] Category descriptions still appear in a plain listing of a store and can still be read by
      address.
- [x] A contributor's own README placed deeper inside a category is still returned by search and
      still treated as an ordinary entry.
- [x] The existing exclusion of always-applied categories from search and from the label vocabulary
      is unchanged.

#### - [x] Phase 2.2: Report labels a search will never reach

**Repo:** `spektacular`

Labels on an entry in an always-applied category can never be reached, because those categories are
deliberately excluded from search and from the label vocabulary. That exclusion is correct and stays
exactly as it is; what changes is that the tool stops accepting such labels in silence. Writing an
entry whose labels no search will reach still writes the entry, and reports which labels those are
along with what to do about it, so a contributor finds out at the moment of writing rather than
never.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-report-labels-a-search-will-never-reach)

**Acceptance criteria**:

- [x] Writing an entry carrying labels into an always-applied category reports those labels as
      unreachable and gives a runnable next step.
- [x] The entry is still written, exactly as it would have been; the report is not a refusal.
- [x] Writing an entry carrying labels into a looked-up category reports nothing, and the result is
      unchanged from today.
- [x] Writing an entry with no labels reports nothing, wherever it goes.
- [x] Nothing about which categories are excluded from search or from the label vocabulary changes.

### Milestone 3: A knowledge base can be kept true, not just tidy

**What changes**: The knowledge skill gains a fifth thing it can do. Until now it could look
something up, record something new, revise something, and check whether entries were labelled well
— but nothing could ask the question that actually matters, which is whether an entry is still
true. An agent can now review a store entry by entry and classify each one as current, stale,
incorrect or unverifiable, and every stale or incorrect verdict must name the specific file, command
or behaviour that makes it so, so a finding can be checked rather than taken on trust. The
distinction the review is built around is the one that decides whether this helps or harms: an entry
stating a standard the code has not met yet is current, not stale, because in this project an entry
states the target and the code is what has yet to meet it. Only an entry whose subject no longer
exists is stale. The review also reports any category description that no longer matches the
project's current definition of that category, naming where it is and how to bring it back into
line. Nothing is written or removed without explicit agreement for that one entry: agreeing to one
entry's outcome never applies to another, and declining one leaves it untouched and does not end the
review. And the standing rule every agent reads, which until now named only reading and writing,
now names removal too, so nothing anywhere tells an agent to delete a managed file with its own
tools.

**Validation point**: The rendered skill offers the maintenance branch and is reachable from a
preamble that advertises it; it carries all four verdicts, the rule separating an unmet standard
from an obsolete entry, the requirement that every staleness verdict names what changed, the
category-description drift report, and per-entry confirmation with a decline that stops one entry
and nothing else. It invokes only commands the tool actually registers. The standing agent rule
names removal as a CLI verb, and no instruction anywhere in the agent-facing material tells an agent
to remove a managed file with a raw file operation. The full Go test suite passes.

#### - [x] Phase 3.1: Review a knowledge base for truth, not just labels

**Repo:** `spektacular`

The knowledge skill gains a fifth thing it can do: judge whether an entry is still *true*, rather
than only whether it is well labelled. Each entry in scope is classified as current, stale,
incorrect or unverifiable, and every stale or incorrect verdict must name the specific file, command
or behaviour that makes it so, so a finding can be checked instead of trusted. The rule the review
is built around is the one that decides whether it helps or harms: an entry stating a standard the
code has not met yet is current, not stale, because an entry states the target and the code is what
has yet to meet it. The review also reports any category description that no longer matches the
project's current definition of that category. Nothing is written or removed without explicit
agreement for that one entry, and declining one entry leaves it untouched without ending the review.

*Technical detail:* [context.md#phase-31](./context.md#phase-31-review-a-knowledge-base-for-truth-not-just-labels)

**Acceptance criteria**:

- [x] The knowledge skill offers a maintenance branch, and the preamble that tells an agent which
      branches exist advertises it, so it is actually reachable.
- [x] A maintenance review classifies every entry in scope as current, stale, incorrect or
      unverifiable.
- [x] Every entry reported as stale or incorrect must name the specific file, command or behaviour
      that makes it so.
- [x] An entry stating a standard the code has not yet met is classified current, and is proposed
      for neither removal nor correction on that basis alone.
- [x] Each entry is its own decision: agreeing to one entry's outcome never applies to another, and
      declining one leaves it untouched and does not end the review.
- [x] A category description that no longer matches the project's current definition of that
      category is reported, naming where it is and how to bring it back into line; one that matches
      is not reported.
- [x] The review adds no bulk, recursive or cross-store operation, and invokes only commands the
      tool actually provides.

#### - [x] Phase 3.2: Make the standing rule name removal

**Repo:** `spektacular`

The rule every agent reads about reaching Spektacular's files through Spektacular currently names
reading and writing and says nothing about removal, which is exactly the silence that made reaching
for a raw file operation look permissible. It gains removal explicitly, for both of the stores that
previously had no way to do it. A sweep over everything an agent is ever shown confirms that nothing
anywhere tells it to remove a managed file with its own tools.

*Technical detail:* [context.md#phase-32](./context.md#phase-32-make-the-standing-rule-name-removal)

**Acceptance criteria**:

- [x] The standing rule an agent reads names removal as something done through the tool, for both
      knowledge entries and design documents.
- [x] No instruction anywhere in the material an agent is shown tells it to remove a managed file
      with a raw file operation.
- [x] The three paths that are deliberately the agent's own to write remain exactly as they are.
- [x] A project that already has this guidance picks up the change when it is next brought up to
      date, without losing anything around it.

### Milestone 4: Removal is documented where people look for it

**What changes**: The documentation catches up with the tool. The knowledge base page describes
removing an entry as part of an entry's life, alongside creating, finding and revising one, and says
what happens when the address names a store that does not exist and when it names a path that holds
nothing. The design documents page describes removing a document beside the other document
commands, shows the refusal for a design a spec still references in the same shape it already uses
for its other refusals, and states plainly that removing a design is not the same as removing a
reference to one — the trap anyone hits today, because the reference command reads like the missing
delete and appears to succeed. The repository's own command references list the two new verbs
alongside the ones they sit with. Nothing about behaviour changes in this milestone.

**Validation point**: Both documentation pages describe removal for their own store, the design page
states the refusal for a referenced design and distinguishes it from removing a reference, and the
repository's own command references list both new verbs. The documentation site builds and type
checks with no errors and no warnings, and the full Go test suite passes, including the existing
sweeps over the repository's own reference documents.

#### - [x] Phase 4.1: Document removing a knowledge entry

**Repo:** `docs`

The knowledge base page describes an entry's life as creating one, finding it, and revising it. It
gains the fourth thing that can happen to one. The new prose says how to remove an entry, that
naming a store the project has not declared is refused while naming a path that holds nothing simply
succeeds, and that a category's own description cannot be removed because it is generated rather
than written.

*Technical detail:* [context.md#phase-41](./context.md#phase-41-document-removing-a-knowledge-entry)

**Acceptance criteria**:

- [x] The knowledge base page describes removing an entry as part of an entry's life, alongside
      creating, finding and revising one.
- [x] It states that an undeclared store is refused while a path holding nothing succeeds, and that
      a category's own description cannot be removed.
- [x] The addition follows the page's existing shape and voice, introduces no new page component,
      and needs no navigation change.
- [x] The documentation site builds and type checks with no errors and no warnings.

#### - [x] Phase 4.2: Document removing a design document

**Repo:** `docs`

The design documents page gains removal beside the other document commands, and the refusal for a
design a spec still references shown in the same response shape the page already uses for its other
refusals. It also states plainly something the page currently leaves implicit and that traps people
today: removing a design is not the same as removing a reference to one. Anyone looking for a way to
delete a design finds the reference command first, and it appears to succeed while leaving the
document exactly where it was.

*Technical detail:* [context.md#phase-42](./context.md#phase-42-document-removing-a-design-document)

**Acceptance criteria**:

- [x] The design documents page describes removing a document beside the other document commands.
- [x] It shows the refusal for a design a spec still references, in the response shape the page
      already uses for refusals.
- [x] It states explicitly that removing a design is a different thing from removing a reference to
      one, and says what each leaves behind.
- [x] The addition follows the page's existing shape and voice, introduces no new page component,
      and needs no navigation change.
- [x] The documentation site builds and type checks with no errors and no warnings.

#### - [x] Phase 4.3: Keep the repository's own command references in step

**Repo:** `spektacular`

The repository's own README and its knowledge base document each enumerate the knowledge commands,
and both are wrong the moment a command exists that they do not list. Both gain the new removal in
the list it belongs to, and the knowledge base document's description of what the knowledge skill
can do gains the maintenance review beside the label audit it already describes. Neither document
enumerates the design commands at all, so neither gains the design removal. Nothing else changes.

*Technical detail:* [context.md#phase-43](./context.md#phase-43-keep-the-repositorys-own-command-references-in-step)

**Acceptance criteria**:

- [x] The repository's README lists the new knowledge removal alongside the other knowledge
      commands.
- [x] The repository's knowledge base document lists it in its command reference, in the same form
      as its neighbours.
- [x] That document's account of what the knowledge skill can do names the maintenance review
      beside the label audit it already describes.
- [x] The existing checks over the repository's own documentation still pass, and nothing else in
      either document changes.


## Open Questions

**None.** Every uncertainty raised while planning was resolved by reading the code or running the
command, and none of them depends on implementation having started. The four that looked like
candidates are recorded here with their resolutions, so a later reader can see the pass was done
rather than skipped.

- *Can a design's record of the specs referencing it disagree with the spec store, making the
  refusal under- or over-report?* **Resolved: yes, in exactly one named circumstance, and the tool
  already handles it.** The two writes that keep a spec and a design in agreement are made to fail
  as a unit by compensation, and the one outcome that can leave them disagreeing — the compensation
  itself failing — is reported under its own distinct code telling the caller to repair the two
  named files by hand. The removal refusal reads the same record every other command reports from,
  so in that state it is consistent with the rest of the tool rather than uniquely wrong. No
  additional handling is warranted. This project currently holds no design documents at all, so
  there is no live instance; the fixtures exercising it are synthetic by construction.
- *Does excluding category descriptions from retrieval break the test asserting that every
  retrieval path honours a selector identically?* **Resolved: no.** That test asserts which stores
  each path reached, not how many entries came back, and its fixture seeds no category
  descriptions.
- *Can a sweep asserting that nothing instructs an agent to remove a managed file with a raw file
  operation be written without flagging the legitimate removal of a staged scratch file?*
  **Resolved: yes**, by pinning distinctive literals and stating the scratch-directory exception
  explicitly rather than relying on the pattern happening not to match. The repository already uses
  exactly this technique for its other banned-substring sweeps, and its own guidance warns that a
  generic literal false-positives on legitimate prose.
- *Should making a drifted category description repairable be treated as overwriting a file someone
  may have edited deliberately?* **Resolved: no.** A category description is generated from the
  project's definition of that category and has two write sites that both render it from that
  definition; one of them already overwrites unconditionally. The plan states the narrowed guarantee
  in the routine's own documentation rather than leaving it implicit.

Two things are *flagged* rather than open, because they are decided but worth an implementer
pausing over:

- Changing the shared knowledge request-payload refusal also changes the refusal shape of the
  existing read and write verbs. That is intended, and the change moves them onto the project's
  error convention. If it turns out any caller depends on the old bare message, **STOP and ask** —
  but no such caller was found.
- The standing rule about reaching Spektacular's files through Spektacular exists both as managed
  agent guidance and as an entry in this repository's own knowledge base. The plan updates the
  first and deliberately leaves the second alone, because writing to a knowledge store requires the
  user's explicit per-entry agreement and a plan cannot grant it. An implementer who feels the entry
  should change should say so rather than change it.


## Out of Scope

**From the spec's Non-Goals:**

- **Cascading removal.** Nothing in this plan clears a design's references on your behalf. The
  refusal names the references and you clear them. An explicit opt-in that does it for you may be
  worth revisiting, and the evidence for why it was not the default this time — the compensating
  rollback a two-document write already needs, with more documents in play — is recorded in
  `research.md#alternatives-considered-and-rejected`.
- **Bulk, recursive or cross-store operations.** Each removal names one document in one store, and
  each maintenance review acts on one entry at a time. Removing a whole category, store or source in
  a single call is not built, and the maintenance review is explicitly barred from introducing one.
- **Undo, trash or soft-delete.** Removal is immediate. Recovery is whatever version control already
  provides, which for every store in this project is git.
- **Documenting the removals the spec, plan and changelog stores already have.** They are absent
  from the documentation site, which is a real gap, and this plan does not close it. It does cover
  them with tests for the first time, but that is to pin the constraint that their behaviour must
  not change, not to document them.
- **Automatic repair of a drifted category description.** The maintenance review reports drift and
  names the remedy; it never applies it. Bringing a store back into line stays something a person
  runs deliberately.
- **Maintenance of anything other than knowledge entries.** Specs, plans, changelog records and
  design documents are not reviewed for staleness, and nothing checks that a design still matches
  the code.

**Deliberately left to a later change by the design chosen here:**

- **The repository's own knowledge entry stating that Spektacular's files are reached through
  Spektacular.** The managed agent-guidance section that states the same rule is updated to name
  removal; the knowledge entry is not. Writing to a knowledge store requires the user's explicit
  agreement for that entry, through the knowledge skill, and a plan cannot grant it. It should be
  proposed once this work lands.
- **A shared removal helper across the knowledge and design packages.** The two will carry
  near-identical methods over the same storage interface, and that duplication is deliberate: the
  packages parallel each other without sharing code beyond the storage layer, for reasons recorded
  on the design package itself. Consolidating them is not a follow-up to schedule, it is a direction
  the project has twice declined.
- **Repo-routed removal for changelog records.** The shared removal that backs the three existing
  stores ignores the option that routes a changelog write, read or listing to a member repo, so
  removal there only ever addresses the central store. This plan records that asymmetry in a test
  comment and does not fix it: the spec constrains those three verbs not to change, and changing one
  would breach it.
- **Reaching a design source that is not a local directory.** Removal is built and proven against
  the storage abstraction rather than the filesystem, and a source whose backend cannot write
  refuses by name, so nothing here needs revisiting when such a backend arrives. Building one is a
  separate piece of work.
- **An end-to-end run of the agent-driven suites.** Those suites were examined and are unaffected:
  the only one naming a knowledge or design command pins step-instruction text this plan does not
  change. No run is scheduled, and the reasoning is recorded so a later reader knows it was checked
  rather than skipped.

**Also not in scope, and worth naming so it is not mistaken for an omission:**

- **Changing which categories are loaded on every task, or which are excluded from search.** This
  plan reports a consequence of the existing exclusion and removes generated descriptions from
  retrieval. It does not re-tier a category, weaken the exclusion, or make labels on an
  always-applied entry reachable.
- **Phrase assertions over the documentation site's prose.** Those pages are verified by the site's
  build and type check, as every prior documentation change in this project has been. Agent-facing
  prose is treated differently and does get assertions, because an agent acts on it.



## Changelog

### 2026-09-21 — Phase 1.1: Pin the behaviour of the removals that already work

**What was done**: Added characterisation tests for the three removals that already worked
(`spec file delete`, `plan file delete`, `changelog file delete`), which had never been covered by
any test. No production code changed, by design — the phase exists to turn the spec's constraint
that these three verbs must not change into something the suite actually checks, and to give the
two new removals in phases 1.3 and 1.4 a recorded baseline to differ from.

**Deviations**: None. The phase was implemented as planned, including the instruction to record the
changelog repo-routing asymmetry rather than fix it.

**Files changed**:
- `spektacular: cmd/file_test.go`

**Discoveries**:

- The `delete` arm's repo-routing gap is sharper than `context.md` describes. The plan says
  `changelog file delete --repo <name>` "ignores repo routing"; in fact `delete` has **no `--repo`
  flag at all**, so cobra refuses at parse time with `unknown flag: --repo`. A repo-routed changelog
  record therefore cannot be removed through the CLI by any invocation. This is pinned as the
  baseline by `TestChangelogFileDelete_DoesNotHonourRepoRouting`. It matters to the follow-up named
  in `## Out of Scope` ("Repo-routed removal for changelog records"): that work is *adding* a flag
  and routing through `resolveStore`, not rewiring an existing flag.
- Today's `delete` writes **nothing at all** to stdout — not an empty envelope, no output. The tests
  assert `stdout == ""` explicitly rather than merely checking the exit code, so when phases 1.3 and
  1.4 give the two new verbs a success envelope, the difference registers as an intentional
  divergence rather than an inconsistency someone later "tidies up".
- `delete` performs no ID-prefix validation, unlike `write`. An arbitrary name reaches the store
  layer even for `plan` and `changelog`, whose `write` would reject it for lacking an ID prefix.
  Relevant when writing the absent-document tests for the new verbs.

### 2026-09-21 — Phase 1.2: Name a category's description, and make a drifted one repairable

**What was done**: The category registry now states in one place that a category's generated
`README.md` is a descriptor rather than a knowledge entry, via `CategoryDescriptionFile` and
`IsCategoryDescription`. The repo footprint scaffolder's guarantee was narrowed from "never
overwrite an existing knowledge file" to "never overwrite a knowledge *entry*": a category
description whose bytes have drifted from the registry rendering is now brought back into line,
while one that already matches is left byte-identical and still reports `unchanged`. That makes
`init` a remedy the refusals in phases 1.3 and 3.1 can honestly point at.

**Deviations**: One small addition beyond the letter of the plan. `internal/project/init.go` also
had a hard-coded `"README.md"` literal at its write site; it now uses
`knowledge.CategoryDescriptionFile` too. Without that, the filename would still have had two homes
and the phase's stated point — that renaming stays a single-field change — would not have held. No
behaviour changed there; that site already overwrote unconditionally.

**Files changed**:
- `spektacular: internal/knowledge/category.go`
- `spektacular: internal/knowledge/category_test.go`
- `spektacular: internal/repo/footprint.go`
- `spektacular: internal/repo/footprint_test.go`
- `spektacular: internal/project/init.go`
- `spektacular: cmd/init_test.go`

**Discoveries**:

- `TestEnsureFootprint_MissingREADMERepairedWithoutRewritingOthers` used a *category description*
  (`conventions/README.md`) as its "must never be overwritten" sentinel, so this phase's production
  change made it fail by construction. That failure was the phase working, not a regression. The
  test was extended rather than replaced: the sentinel moved to a hand-written entry beside it, so
  the "an entry is never rewritten" guarantee it existed to protect is still enforced. Any future
  change to what the footprint may rewrite should expect to meet this test the same way.
- `Category.README()` had no test at all before this phase, despite being the renderer that both
  write sites and now the drift comparison depend on. It is covered now.
- Proving "the file was not written" is done with `os.Chtimes` to back-date the mtime and then
  asserting the mtime survives. That is deterministic, unlike comparing timestamps taken around the
  call, and avoids asserting content and mtime together.

### 2026-09-21 — Phase 1.3: Remove a knowledge entry

**What was done**: `knowledge delete` was added, addressed exactly as read and write are. It
composes a new two-line `Set.Delete` over the existing address gate, so an undeclared store is
refused with the tier's store names before anything is touched, and removing a path that holds
nothing is a success. Its envelope reports `deleted`, settled by a read before the removal, because
the storage contract's `Delete` returns nil either way. Removing a category's generated description
is refused with the command that regenerates it.

**Deviations**: One correction made during implementation and worth recording, because it is the
exact failure this feature exists to prevent. The refusal's next action first named `spektacular
init`, which is **not runnable** — `init` takes the agent as a required argument. It now names the
agent the project records (`spektacular init claude`), and a test holds that. The other sites that
name init keep the `<agent>` placeholder, correctly: they fire when no agent is recorded or no
project exists, whereas a refusal raised inside a configured project has the agent in hand.

**Files changed**:
- `spektacular: internal/knowledge/set.go`
- `spektacular: internal/knowledge/set_test.go`
- `spektacular: cmd/knowledge.go`
- `spektacular: cmd/knowledge_test.go`
- `spektacular: cmd/no_project_test.go`

**Discoveries**:

- The corrective change to `knowledgeAddressData` is strictly additive at the message level: it adds
  a code and a next action but leaves the text `--data is required` intact, so the one existing
  assertion on that message still holds. The plan flagged this change as possibly breaking a caller;
  it does not.
- `knowledgeConfigLoadingCmds` and `knowledgeNarrowingCmds` are a deliberate pair, and a new verb
  belongs to exactly one of them. `delete` joins the first (it loads config, so it must face the
  superseded-config and untagged-base sweeps) and must stay out of the second (it is addressed, not
  fan-out, and takes no `--tier`/`--filter`). A verb added to neither is silently exempt from the
  sweeps rather than failing loudly.
- A refusal's next action must be checked by *running it*, not by reading it. Asserting the content
  of a next action is the project's stated rule; the init-argument bug shows content assertions can
  still pass on a step that does not execute, so the smoke test is what caught it.

### 2026-09-21 — Phase 1.4: Remove a design document

**What was done**: `design delete` was added as a hand-written sibling of the other design verbs,
composing a new `Set.Delete` that follows `Write`'s sequence exactly — unknown source, incomplete
address, then a source whose backend cannot write, each refused by name before anything is touched.
Its envelope reports `deleted`, settled by the `Exists` call the write path already makes for the
same reason.

**Deviations**: None.

**Files changed**:
- `spektacular: internal/design/design.go`
- `spektacular: internal/design/design_test.go`
- `spektacular: cmd/design.go`
- `spektacular: cmd/design_test.go`
- `spektacular: cmd/no_project_test.go`

**Discoveries**:

- A design source's relative location resolves from the folder holding `config.yaml`, not from the
  project root. Declaring `location: ./design` and creating `<root>/design` yields
  `design_source_unreachable` pointing at `<root>/.spektacular/design`. The refusal says so clearly,
  which is how it was diagnosed in seconds; worth knowing before writing any design fixture by hand.
- `internal/design` now carries a `Delete` near-identical to the knowledge package's. That
  duplication is deliberate and recorded in `## Out of Scope`; it is not a consolidation candidate.

### 2026-09-21 — Phase 1.5: Refuse removing a design a spec still references

**What was done**: `design delete` now refuses a document one or more specs still reference. The
referencing specs are read from the document's own lifecycle record rather than found by scanning
the spec store, so the check is one read. The refusal names every referencing spec and gives a
runnable `design ref remove` for each, followed by the delete to retry, and writes nothing in either
direction — the document is left byte for byte and no spec is touched. A design carrying no
lifecycle block, or one whose specs list is empty, falls straight through to the removal.

**Deviations**: None. Implemented as one guard inside `runDesignDelete` rather than a separate
surface, which is what the plan describes.

**Files changed**:
- `spektacular: cmd/design.go`
- `spektacular: cmd/design_ref_test.go`

**Discoveries**:

- `authoredMetadata` rather than `metadata.Split` is load-bearing here, not stylistic. It swallows a
  parse error, so a design carrying the team's own YAML header reads as unauthored instead of making
  the delete fail outright on a perfectly ordinary file. That is the behaviour this feature promises
  not to disturb.
- `fm == nil` and `fm != nil && len(fm.Specs) == 0` are genuinely distinct branches: probing
  `metadata.Split` on `specs: []` returns a non-nil `*Metadata` with an empty list. Both are covered
  separately rather than assumed equivalent.
- The refusal's `resource` and the message's location are the **resolved absolute path**, matching
  `design_authored_overwrite`. Tests assert on suffixes and spec names rather than an absolute
  prefix.

### 2026-09-21 — Phase 2.1: Keep category descriptions out of retrieval

**What was done**: `IsCategoryDescription` is now consulted at the two surfaces that already consult
the registry for the always-applied exclusion — the post-merge filter in `Set.Search` and the file
loop in `readCategories`. A category's own description no longer appears in search results or in the
payload an agent receives on every task. `listFiles`, `Set.List`, `Set.Tags` and `internal/store`
are deliberately untouched, so descriptions remain listable and readable, which phase 3.1's drift
report depends on.

**Deviations**: None.

**Files changed**:
- `spektacular: internal/knowledge/set.go`
- `spektacular: internal/knowledge/set_test.go`
- `spektacular: internal/knowledge/category_test.go`
- `spektacular: cmd/knowledge_test.go`

**Discoveries**:

- Measured against this project's own knowledge base, which is where the audit findings were first
  observed: `knowledge search "purpose belongs elsewhere entry shape"` returned 8 hits, every one a
  category README; it now returns 3, none of them a README. The always-applied payload carried 2
  READMEs per store on every task; it now carries none. `knowledge list` still reports all 12.
- The exclusion's **position** in `Set.Search` is load-bearing, not incidental. It must run before
  the relevance floor is computed from `eligible[0].Score`, or a description scoring highest would
  set the bar and then be dropped, quietly raising the threshold for every real entry. A test pins
  this specifically, by seeding descriptions that outscore the one real entry.
- `alwaysAppliedProject` and the `internal/knowledge` always-applied fixture seeded no `README.md`,
  so before this phase no fixture anywhere could have caught a description leak. Seeding them is
  what turns the existing exact-match assertions into leak guards.

### 2026-09-21 — Phase 2.2: Report labels a search will never reach

**What was done**: `knowledge write` now reports labels no search can ever reach. `UnreachableTags`
returns them for an entry bound for an always-applied category, whose entries are deliberately
excluded from search and from the label vocabulary, and nil otherwise. The report travels as
`unreachable_tags` and `next_action` on the **success** envelope, after the write, so it can never
become a gate: the entry is written exactly as it would have been either way.

**Deviations**: None to the design. One test-placement improvement over the plan: the
`UnreachableTags` unit tests live in `category_test.go` beside the function rather than in
`set_test.go`, matching the project's co-location convention.

**Files changed**:
- `spektacular: internal/knowledge/category.go`
- `spektacular: internal/knowledge/category_test.go`
- `spektacular: cmd/knowledge.go`
- `spektacular: cmd/knowledge_test.go`

**Discoveries**:

- Asserting that an optional field is **absent** needs the envelope decoded as a raw object; a typed
  struct cannot distinguish "omitted" from "zero value", so an over-firing report would pass
  unnoticed. The command tests use a raw-map helper for exactly this.
- Adding two optional fields to `knowledgeWriteOutputSchema` broke
  `TestKnowledgeWrite_SchemaDocumentsTheAddressItRequires`, whose hand-listed oracle was
  `{tier,name,path}`. That is the oracle doing its job. It now lists all five and asserts the two
  new ones' types, with a comment saying they are optional and absent in the normal case.

### 2026-09-21 — Phase 3.1: Review a knowledge base for truth, not just labels

**What was done**: The `spek-knowledge` skill gained a fifth intent, maintenance, between the audit
intent and decline handling. It classifies each entry as current, stale, incorrect or unverifiable,
requires every stale or incorrect verdict to name the specific file, command or behaviour that
changed, reports drifted category descriptions without repairing them, and proposes and confirms one
entry at a time with removal only after agreement for that entry. The preamble, trigger list and
description sentence moved with it, because a branch the preamble does not advertise is never
reached.

**Deviations**: None to the content.

**Files changed**:
- `spektacular: templates/skills/workflows/spek-knowledge/SKILL.md`
- `spektacular: .claude/skills/spek-knowledge/SKILL.md` (regenerated)
- `spektacular: internal/agent/instruction_surface_test.go`

**Discoveries**:

- **`migrate` does not regenerate skills during development.** It reports `up_to_date` with
  `reinstall: false` whenever the installed and current versions match, so an in-place template edit
  is not picked up. `init <agent>` is what re-renders them. The plan's migration notes name migrate,
  which is right for a version bump and wrong for editing a template.
- The branch-count oracle previously banned only the phrasing it replaced ("three"), so adding a
  fifth intent could have left a stale "four branches" sentence passing. It now bans every
  superseded count by name, which closes the class rather than this instance.
- One oracle the plan did not predict also moved: the decline-handling test pins the literal "In the
  audit intent the same rule applies **per entry**", and covering maintenance meant broadening that
  very sentence.
- Slicing a section to the next `\n# ` still isolates the audit intent now that maintenance follows
  it, but that is now asserted rather than assumed, in both directions.

### 2026-09-21 — Phase 3.2: Make the standing rule name removal

**What was done**: The managed store-access guidance now names removal as a CLI verb, naming
`knowledge delete` and `design delete` alongside the `delete` the three file families already had,
stating that `rm` on a managed file is never correct, and telling an agent to act on a refusal
rather than reach past it. A corpus-wide sweep asserts no template and no rendered skill instructs a
raw removal of a managed file, while pinning the scratch-directory exception.

**Deviations**: None.

**Files changed**:
- `spektacular: templates/agents/store-access.md`
- `spektacular: AGENTS.md` (regenerated)
- `spektacular: internal/agent/store_access_test.go`
- `spektacular: internal/agent/instruction_surface_test.go`

**Discoveries**:

- **The sweep cannot ban a bare `rm ` — the word `confirm` ends in `rm`.** `confirm
  .spektacular/specs/…` literally contains `rm .spektacular/specs/`. Each removal verb is therefore
  anchored on `(^|[^A-Za-z])`. The plan asked whether such a sweep could be written without flagging
  legitimate prose; it can, but distinctive path literals alone are not enough — word-boundary
  anchoring is what makes it safe.
- Bare `Remove(` is deliberately not banned, only `os.Remove(` and `os.RemoveAll(`: the bare form is
  too generic to be safe in English prose.
- The design store has no fixed default directory, since locations are config-declared. The sweep
  bans the default spelling because its job is to catch an *instruction* being written, and a
  relocated store is reached by the same CLI verbs either way.

### 2026-09-21 — Phase 4.1: Document removing a knowledge entry

**What was done**: The knowledge base page now describes removal as part of an entry's life,
alongside creating, searching and rewriting one, covering the undeclared-store refusal, the
path-holding-nothing success, the reported distinction between the two, the category-description
refusal, and that recovery is whatever version control already provides.

**Deviations**: None.

**Files changed**:
- `docs: src/pages/knowledge-base.mdx`

**Discoveries**:

- The two documentation pages have genuinely different house styles and the plan was right to insist
  on matching the page being edited: this one uses a bolded lead-in, spaced JSON (`", "`), `--file`,
  and documents refusals in prose rather than as JSON blocks.

### 2026-09-21 — Phase 4.2: Document removing a design document

**What was done**: The design documents page gained removal beside the other document verbs, the
`design_referenced_delete` refusal shown in the same JSON shape the page already uses for
`design_not_found`, and an explicit statement that removing a design is not the same as removing a
reference to one, placed as the bridge into the reference verbs.

**Deviations**: None.

**Files changed**:
- `docs: src/pages/design-documents.mdx`

**Discoveries**:

- The trap this prose exists to close is real and worth naming plainly: anyone looking for a way to
  delete a design meets `design ref remove` first, and it appears to succeed while the document is
  still sitting where it was.
- This page's conventions are the opposite of the knowledge page's on two counts: compact JSON with
  no space after the colon, and `--from` rather than `--file`.

### 2026-09-21 — Phase 4.3: Keep the repository's own command references in step

**What was done**: The repository's README gained a `knowledge delete` bullet and
`docs/knowledge-base.md` gained a command-reference row plus a new subsection describing the
maintenance review beside the tag audit it already described.

**Deviations**: None. Neither document enumerates the design commands, so neither gained
`design delete`, exactly as the plan directed.

**Files changed**:
- `spektacular: README.md`
- `spektacular: docs/knowledge-base.md`

**Discoveries**:

- The `docs` repo bans em dashes as a repo-scoped convention. Both edited site pages contain zero,
  so the convention is intact rather than merely not-worsened. It does **not** bind the
  `spektacular` repo's own prose, which is why the entries above use them freely.
