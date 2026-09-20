# Working context: 000054_project-level-design-documents

## How this came up

The user is working on a feature for **xclconfig** (hypothetical at this point;
not a registered repo of this project) and said:

> "I have a very specific UX design I would like for the api that will changed
> by this spec. How do we incorporate the design doc into the spec?"

Their objection to putting the design inline:

> "if we put all of that into the spec it becomes really untidy and potentially
> has a tonne of info. Maybe that is the right way, maybe that should be where
> the info stays."

Then, after research:

> "I also think we need Spektacular to formally own design docs. Probably we
> should make provision for this in the spec. Again people should be able to
> store design docs where they want, in the repo, files, another repo, github
> issues etc. We can use our storage capability to enable that."

> "I also think design should be a project level construct"

And on starting this spec:

> "Yes start the workflow but when you do, remember the docs, this feels like
> an important concept we should highlight"

So documenting the concept is an explicit user requirement, not an afterthought.

## The gap in Spektacular today

- A spec is a **single file**. The spec store tolerates a sibling
  (`000054_x/design.md` or `000054_x-design.md`) but no workflow step reads
  either, and both pollute `spec file list` (a bare directory entry with no
  metadata, or a second spec-looking entry with its own status).
- **Technical Approach is explicitly non-binding**: its step template tells the
  agent to keep it at one-line-steer altitude, and the plan "may adopt, adapt,
  or replace" it. So a settled design placed there can be discarded.
- **No plan step template mentions an external document, a design doc, or
  following a link out of the spec.** Verified by grep over
  `templates/steps/plan/*.md`. A design referenced from a spec therefore has no
  guaranteed reader: the plan agent may follow it or may re-derive the design.
- `.spektacular/work/<spec>/` holds transient per-section working files and is
  empty between runs, so it is not a home for a durable design document.

## What the research found (4 parallel web-research agents)

Full reports are in the session transcript. The findings that shaped the
direction:

- **Nobody puts a full API design in the requirements document.** Kubernetes
  KEP: the Proposal section "should not include things like API designs or
  implementation"; Design Details "may include API specs ... or even code
  snippets". Google's design-doc guidance: "one should withstand the temptation
  to copy-paste formal interface or data definitions into the doc as these are
  often verbose, contain unnecessary detail and quickly get out of date."
- **What predicts a split is whether something downstream is authoritative.**
  TC39 (explainer README vs normative spec.emu) and W3C TAG split because a
  standard owns the normative text. Rust, React, Ember and Kubernetes keep it
  inline because the proposal is the most spec-like artifact they own.
  Kubernetes deliberately consolidated, archiving design-proposals-archive in
  2021.
- **Rust's anti-drift rule**: the reference-level explanation "should return to
  the examples given in the previous section, and explain more fully how the
  detailed proposal makes those examples work". Same content, two fidelities.
- **When you split, the discipline is summarise-and-link, never restate**
  (arc42 Tip 4-4: "Avoid redundancy, don't repeat information from views or
  concepts"), plus W3C TAG's collapse rule: once the spec owns the detail,
  delete it from the explainer and "replace them with links to the relevant
  part of the spec".
- **AI spec tools** converge on requirements/design/tasks per feature folder
  (Kiro: requirements.md, design.md, tasks.md; Spec Kit: ~8 files) plus a
  project-level layer (Kiro steering, Spec Kit constitution). Thoughtworks'
  review of the elaborate end: the generated documents "were repetitive, both
  with each other, and with the code that already existed ... I'd rather review
  code than all these markdown files". Tessl is the counter-shape: one
  `.spec.md` per code file, no design document at all.
- **Nobody documents an obsolescence rule for shipped specs, and no AI tool has
  a durable design layer or an immutable decision log.** Spektacular already
  has the tier distinction (specs and plans historical, knowledge current and
  binding), which is why placing designs in the right tier deliberately is an
  advantage here.

## Direction agreed in conversation

- Spektacular **owns the reference, not the document**: it resolves and reads a
  design, it does not absorb or rewrite it. This keeps the spec tidy, which was
  the original complaint.
- **Storage is pluggable**, reusing the existing store/provider grain:
  `internal/store.Store` is an interface (Root, Read, Write, Delete, List,
  Exists, Search) with `FileStore` the only implementation; `ProviderFile` and
  `ProviderGit` constants already exist.
- **Design is a project-level construct**, a fourth artifact class beside spec,
  plan and changelog, not a field on a spec.
- Designs should be reachable **through the CLI** like every other artifact, so
  a reference has a guaranteed reader.
- The **plan workflow must be obliged** to read referenced designs and adopt
  rather than re-derive them, with `context.md` restating the design's examples
  at implementation fidelity (Rust's rule).
- **Docs are in scope**: the concept must be highlighted on the documentation
  site, not only in the configuration reference.

## Settled in the interview

1. **Sources-shaped, project tier only.** Named design sources declared by the
   project; repos do not declare their own. User: "a design document is really
   part of a spec. It should be project level."
2. **Local file provider only** in this feature; the shape must not foreclose
   git or read-only remote documents later.
3. **Lifecycle is exactly a spec's**: binding input while a plan is built,
   historical once shipped, never force-synced.
4. **The spec workflow can author designs**, offering capture when the
   conversation settles an API shape, a user-facing flow, a data format or a
   worked example. User's reason: "so that we can ensure that the spec retains
   facts."
5. **Many-to-many linkage**: any spec may reference any design.
6. **Docs get a dedicated concept page**, plus the configuration reference.

## Decisions taken during verification

- A fresh-eyes review returned 15 findings; 13 applied, 2 rejected as house
  convention (the `000054_` title form and the scaffold's comment blocks, which
  every stored spec carries).
- Two ambiguities were closed by new constraint text: a direct user instruction
  to write a design *is* the required agreement, while an agent-proposed
  capture must be accepted first; and a design reference names both its source
  and the document within it, with an undeclared source refused.

## Originally still to settle (now closed, kept for context)

1. **Store-shaped or sources-shaped?** Store-shaped mirrors spec/plan (one
   project design store, one provider). Sources-shaped mirrors knowledge (a
   list of named sources across project and repo tiers, each with its own
   provider). The user's "store them where they want ... repo, files, another
   repo, github issues" points at sources-shaped; I lean that way.
2. **Historical or current-and-binding?** Specs and plans are archaeology here;
   knowledge is binding and current. A settled API design behaves like
   knowledge: if the code disagrees, the code is wrong.
3. **Read-only remote providers.** The `Store` interface is filesystem-shaped
   (a root, path addressing, write access). A GitHub issue has none of those.
   Either design references need a narrower read-only interface, or remote
   providers materialise a local snapshot.
4. **Drift.** A design lives outside Spektacular and can change after a plan is
   built. Record a checksum and warn, or treat the design as always current?
5. **Scope of providers in this feature** versus later (file and repo-routed
   first, git and remote issues later?).
6. **Who may reference a design**: specs only, or plans and changelog too?

## Notes

- Feature 000053 (settings format versions, migrate, settings-relative store
  folders) was completed in this same session and is uncommitted: 53 changed
  files here plus 10 in the docs repo. Do not confuse its artifacts with this
  spec's.
- The docs repo carries unrelated uncommitted user edits in configuration.mdx,
  how-it-works.mdx, projects.mdx and knowledge-base.mdx.

---

## Plan phase (started 2026-09-20)

Planning `000054_project-level-design-documents` against the spec above.
Registered repos for this project (from `go run . repo list`):

- `spektacular` — root `/home/nicj/code/github.com/jumppad-labs/spektacular` (Go CLI, role `tool`)
- `docs` — root `/home/nicj/code/github.com/jumppad-labs/spektacular-website` (Astro 5 docs site, role `documentation`)

All code work for this feature lands in one of those two roots; requirements are
attributed per-repo in the plan's context document.

### Discovery findings that shape the plan

- **The sources shape the spec asks for already exists in code.**
  `config.KnowledgeConfig{Sources []SourceConfig}` with
  `SourceConfig{Name, Provider, Config.Location, Tier}` is exactly "a list of named sources,
  each naming a provider and a location". `design` reuses `SourceConfig`; project tier only,
  so the `Tier` field stays unused for designs.
- **Store-shaped config is ruled out by a hard mechanism, not taste.** `escapesRoot`
  refuses a store directory outside the project root, which would break the "adopt a folder
  you already have" success metric. Knowledge-source resolution has no such refusal.
- **Addressing precedent is `knowledge read/write`**: `--data '{"tier","name","path"}'`.
  Designs drop `tier` and use `{source, path}`. 000047's binding rule: refuse on unknown or
  missing, never resolve on the caller's behalf even when one candidate exists, and name the
  available sources in `next_action`.
- **No schema bump.** 000053: the format version rises only when a change would make an old
  file misread. `design:` is additive and optional. No migration step.
- **Design documents get no Spektacular frontmatter.** Settled by 000038 (the four
  metadata-bearing classes are exhaustive; the store is deliberately metadata-agnostic) plus
  000054's own non-goals. Consequence: the design commands are *not* another
  `newStoreFileCmd` registration, since that factory stamps and reads the block.
- **The reference lives in the spec's frontmatter.** `metadata.yamlShape` is a closed
  schema, so any key not in the struct is dropped when the block is re-rendered - a
  reference recorded outside it would not survive the next `spec file write`. A `Designs`
  field on `metadata.Metadata` plus `Merge` preservation is the only shape that survives the
  existing commit path.
- **No design workflow FSM.** A fifth workflow would take the cross-kind lock and so could
  not run during a spec workflow, which is exactly when capture must happen. Saves ~13
  hand-maintained test surfaces.
- **Capture offer = managed AGENTS.md section + in-step prose.** 000041 proved the standing
  trigger alone fails, and forbade new FSM interruption points. Gotcha from 000041:
  `{{config.command}} skill <name>` does not resolve for skills nested under
  `templates/skills/workflows/`.
- **Read-only-provider room**: split `store.Reader` (Read/List/Exists/Search) out of
  `Store`. Zero call-site churn; `*FileStore` satisfies both. 000014's precedent is to defer
  the abstraction until a real backend needs it, and 000039 explicitly rejected capability
  proxying, so nothing more is built now.
- **Docs**: one new concept page under the Resources nav dropdown, Hero → alternating
  `Section` bands → `CtaBanner`; CLI usage on the concept page, config keys in
  `configuration.mdx` with a sparse `ConfigKey` that links out. The docs repo has ~130
  uncommitted lines in `configuration.mdx` from 000053, so anchor on component boundaries,
  not line numbers.
- **Baseline**: `go test -shuffle=on ./...` green, 21 packages, before any change.

### Architecture locked (architecture step)

Chosen: `design.sources` in `config.yaml` reusing `config.SourceConfig` (project tier only,
no `tier` in the address), a new `internal/design` domain package with a literal provider
switch, and a `spektacular design` command family:

- `design sources` — the declared sources with their resolved locations
- `design list [--source <name>]` — documents, name and path only, no metadata enrichment
- `design read --data '{"source":…,"path":…}'` — fails `design_not_found` naming the source,
  the path and the absolute location searched
- `design write --data '{"source":…,"path":…}' --from <file>` — bytes through unchanged
- `design ref add|remove --data '{"spec":…,"source":…,"path":…}'` — refuses an undeclared
  source in Go and records nothing
- `design ref list --data '{"spec":…}'` — always succeeds; per-ref `resolved` + `location`,
  plus an `unresolved` count and a `next_action` when non-zero

References live in the **referencing spec's** frontmatter as a `designs:` list of
`{source, path}`, added to `metadata.Metadata`/`yamlShape`/`yamlInShape` and preserved by
`metadata.Merge`. The design document itself carries no Spektacular frontmatter.

Read-only-provider room: extract `store.Reader` (Read/List/Exists/Search) from `store.Store`;
a design source holds a reader always and a writer only when the provider can write, so a
write to a read-only source is a named refusal rather than a violated contract.

"Planning records the design it read" is satisfied by prose in plan steps 02-discovery
(resolve and read) and 07-dependencies (one bullet per design naming its source), not a new
`plan.md` scaffold section, so the harbor `EXPECTED_PLAN_SECTIONS`/`SCAFFOLD_LEFTOVERS`
oracles are untouched.

No new agent skill: a managed `templates/agents/design-trigger.md` section plus prose in the
spec Technical Approach step and the `spek-new`/`spek-plan` skills.

`init` does not scaffold design source directories; `design.NewSet` fails fast when a
declared location is not a directory, mirroring knowledge's `unreachableStore`.

### Phase plan shape (phases step)

12 phases across 4 milestones, all in `spektacular` except 4.1 and 4.2 which are `docs`:

1.1 config `design.sources` · 1.2 `store.Reader`/`Writer` split · 1.3 `internal/design`
· 1.4 `cmd/design.go` (sources/list/read/write)
2.1 `metadata.DesignRef` + `Merge` preservation · 2.2 `design ref add|remove|list`
3.1 `templates/agents/design-trigger.md` + `internal/agent/design_trigger.go`
· 3.2 spec 05-technical_approach offer + plan 02/03/07 obligations + skills
· 3.3 harbor oracle reconciliation and runs
4.1 `docs:src/pages/design-documents.mdx` + Nav · 4.2 `configuration.mdx` ConfigKey
· 4.3 README (spektacular repo)

Traps recorded in the phase detail so they are not rediscovered:
- Do **not** add design to `config.storeDirs()` — that list carries the `escapesRoot` refusal.
- Do **not** touch `CurrentProjectSchema` or the migrate registry; `registry_test.go` pins them
  together.
- Do **not** add headings to `templates/scaffold/{spec,plan}.md` — `specStillScaffold` compares
  against the rendered scaffold and the harbor `SCAFFOLD_LEFTOVERS`/`EXPECTED_PLAN_SECTIONS`
  oracles mirror those files.
- Do **not** route design writes through `newStoreFileCmd`; it stamps frontmatter.
- `UpdateOptions.Designs` must be a pointer so nil means "preserve"; an ordinary
  `spec file write` passes nil and must keep existing references.
- Anchor docs edits on `<ConfigKey>` boundaries, not line numbers (docs repo has ~130
  uncommitted lines in `configuration.mdx`).

### Assembly (assemble step)

Three documents staged to `.spektacular/tmp/{plan,context,research}_template.md`
(1131 / 445 / 534 lines). Metadata: created 2026-09-20T09:48:12Z, commit c88eecf,
branch f-migrate.

Assembly notes worth keeping if this is re-run:
- `milestones.md` and `phases_plan.md` must be **interleaved**, not concatenated: each
  `### Milestone N` followed by its own `#### - [ ] Phase N.M` blocks. Straight concatenation
  puts all four milestones before all twelve phases, which reads wrong. The interleaved result
  is cached at `.spektacular/work/<name>/_milestones_and_phases.md`.
- All 12 `*Technical detail:*` links were checked against the rendered `context.md` headings;
  all resolve.
- `## Overview` was derived (not from a working file, as the plan workflow has no overview
  drafting step); `## Drafting assumptions` in research.md is `assumptions.md` inserted before
  `## Rehydration cues`.

### Verification (verification step)

Fixed one assembly error: `context.md`'s required section order puts `## Project References`
**after** `## Testing Strategy`, not straight after `## Current State Analysis`. The scaffold
template does not list Project References at all, only the verification step does, so this is
easy to get wrong on a re-run.

Checks run and passing on the staged documents: section presence and order for all three; no
thin sections; no scaffold placeholders; 12/12 `*Technical detail:*` links resolve to real
`context.md` headings; 12/12 phases carry a `**Repo:**` line, a technical-detail link and
checkbox acceptance criteria; phase titles match between `plan.md` and `context.md`; no
"see context.md" cross-reference in `plan.md`; no em dashes in the docs-bound content blocks.

### Documents committed (write steps)

All three committed to the plan store and the working directory removed:
`.spektacular/plans/000054_project-level-design-documents/{plan,context,research}.md`.
Walkthrough with the user is the remaining step.

### Walkthrough (walkthrough step)

Plan walked through with the user across the four beats (approach, phases, out of scope, the
24 drafting assumptions, with six flagged for challenge). The user signed off with no changes
requested, so no correction was applied and no knowledge-capture offer was warranted.

---

## Implement phase (started 2026-09-20)

Implementing `000054_project-level-design-documents`. Fresh workflow (no resume report), so
this is a first-phase invocation: `plan.md` carries no `## Changelog` section yet and the
current phase is the first unchecked one, **Phase 1.1**.

### read_plan gate results

- All three plan documents read in full through `go run . plan file read`.
- **Structural validation passed.** All 10 required `##` sections present (plus an extra
  `## Conventions`, which the plan scaffold carries). 12 `#### - [ ] Phase N.M` headings, each
  with a `*Technical detail:*` link; all 12 anchors resolve to matching `### Phase N.M:`
  headings in `context.md`.
- **Drift check passed with one benign note.** Every file path, Go symbol, command path and
  template path named in `plan.md`/`context.md` was verified against the two repo roots
  (`spektacular` at the project dir, `docs` at `../../spektacular-website`). All resolve,
  including the exact line anchors for `config.go` (SourceConfig :163, KnowledgeConfig :129,
  KnowledgeConfig.Validate :652, storeDirs :358, escapesRoot :397, CurrentProjectSchema :18),
  `store.go:66` (the `Store` interface to split), `metadata.go` (:59/:75/:87), `merge.go`
  (:11/:38), `cmd/storefile.go:172` (`newStoreFileCmd`), `cmd/knowledge.go` (:286/:349/:549),
  `cmd/root.go:351-361` (registration point, `knowledgeCmd` at :355), and the docs repo's
  `configuration.mdx` (`knowledge` ConfigKey at :200, `repos` at :216) and `Nav.astro`
  Resources group.
  - **Note**: `internal/metadata/merge_test.go` does not exist; merge coverage currently lives
    in `internal/metadata/metadata_test.go`. Phase 2.1 names both files, so the merge cases go
    into a newly created `merge_test.go` (or into `metadata_test.go`). Not a stale pointer,
    just a file to be created.
  - Docs-repo line numbers in the plan are a little stale because of the ~130 uncommitted
    lines in `configuration.mdx`; the plan anticipated this and anchors on `<ConfigKey>`
    component boundaries instead. Confirmed those boundaries are where the plan says.
- **Spec coverage passed.** All 12 `## Requirements` and all 11 `## Acceptance Criteria`
  checkboxes in the spec have coverage in `plan.md`'s `## Milestones & Phases`. Nothing
  descoped, so no `**Descoped requirements**:` list was added.
- **Baseline confirmed green** before any change: `go test -shuffle=on ./...` exit 0 across
  all packages. Any later failure is attributable to this work.

### Standing decisions carried into implementation

- The two Open Questions in `plan.md` are live and must be answered, not skipped:
  (a) whether the settings format version must rise after all (default is **no bump**; if the
  silent-ignore behaviour of an older binary is judged unacceptable, **STOP and ask the user**
  before bumping, and move `CurrentProjectSchema` and a `project3to4` step together or
  `internal/migrate/registry_test.go` fails); and (b) whether the end-to-end spec suite should
  assert the capture offer's outcome, which must be decided explicitly in Phase 3.3's notes
  rather than left unstated.

### Phase 1.1 analysis (analyze step)

Current phase: **1.1 Declare design sources in project settings** (`spektacular`, Low
complexity, single agent sequential — no sub-agents spawned). Touchpoints confirmed at their
current lines:

- `internal/config/config.go:129` `KnowledgeConfig` — `DesignConfig` goes beside it.
- `internal/config/config.go:163` `SourceConfig`, `:175` `FileKnowledgeConfig` — reused
  unchanged, not renamed.
- `internal/config/config.go:263-264` — `Design` field goes between `Knowledge` and `Repos`.
- `internal/config/config.go:296-298` — the `Knowledge is empty by default` comment in
  `NewDefault()` is the comment to mirror.
- `internal/config/config.go:511` `Config.Validate`, with `c.Knowledge.Validate()` at `:535`
  — `c.Design.Validate()` goes immediately after it.
- `internal/config/config.go:652` `KnowledgeConfig.Validate` — the template; `DesignConfig`
  `Validate` goes after it, before `ToYAMLFile`.
- `internal/config/config_test.go` — model on `TestKnowledgeConfig_ValidateRejects*`
  (`:298`, `:313`, `:333`) and `TestToYAMLFile_ProjectOwnedKnowledgeSourcesRoundTrip`
  (`:657`). `projectConfigPath(t)` at `:17` is the temp-dir helper.

**Decision: all four `DesignConfig.Validate` refusals use
`output.NewError("config_invalid", …).WithNextAction(…)`, not just the two name cases.**
`KnowledgeConfig.Validate` uses a bare `fmt.Errorf` for its unsupported-provider and
empty-location cases, so copying it "line for line" would ship two refusals with no next
action. Phase 1.1's acceptance criterion requires the offending entry named **and a
correction given** for all four cases, and
`spektacular:conventions/error-messages-must-suggest-remediation.md` is explicit that the
rule "applies to every error path in the CLI, not just a specific command family". The
knowledge version is not touched (out of this phase's scope), so the two validators diverge
on purpose; if that bothers a reviewer, the fix is to bring knowledge up, not design down.

**Confirmed empirically**: `yaml:"...,omitempty"` on a struct field does omit an all-zero
struct in yaml.v3 — this project's own `.spektacular/config.yaml` carries no `knowledge:`
key. So `Design DesignConfig \`yaml:"design,omitempty"\`` satisfies the "marshals without a
design: key" criterion with no extra work.

### Phase 1.1 implementation (implement step)

Landed in `internal/config/config.go` only, +61 lines, no other file touched:

- `DesignConfig{Sources []SourceConfig}` beside `KnowledgeConfig`, with a doc comment stating
  it is project tier only and that `SourceConfig.Tier` stays unset for design entries.
- `Design DesignConfig \`yaml:"design,omitempty"\`` on `Config`, between `Knowledge` and
  `Repos`, so the marshalled key order matches the documented order.
- `NewDefault()` leaves `Design` zero, with the comment extended to say why nothing is
  scaffolded for a design source.
- `c.Design.Validate()` in `Config.Validate`, immediately after `c.Knowledge.Validate()`.
- `DesignConfig.Validate()` after `KnowledgeConfig.Validate()`, with all four refusals built
  as `output.NewError("config_invalid", …).WithNextAction(…)` per the decision recorded in
  the analyze notes. Its doc comment states that divergence from the knowledge validator
  explicitly, so a maintainer does not "fix" it back to a bare `fmt.Errorf`.

Not touched, as the phase requires: `storeDirs()` (it carries the `escapesRoot` refusal that
would forbid a location outside the project root), `internal/config/schema.go`,
`internal/migrate/registry.go`. `SourceConfig` and `FileKnowledgeConfig` reused unchanged.

`go build ./...` and `go vet ./internal/config/` clean. A scratch round-trip check confirmed
a `design:` block survives `ToYAMLFile`/`FromYAMLFile` with a relative location intact; the
scratch file was deleted, and the real tests are the next step's work.

### Phase 1.1 tests (test step)

Delegated to a sub-agent per the step's instruction, then verified in the main context. 8
tests added to `internal/config/config_test.go` (+186 lines, pure addition, no existing test
touched), sitting beside the knowledge-source cases:

- round-trip of two named design sources, asserting `schema: 3` is still stamped;
- absent `design:` section loads with `Design.Sources` nil and is not written back;
- a relative location is read and written back verbatim, never absolutised (the property that
  distinguishes a design source from a `storeDirs()` entry);
- an invalid source is refused through `FromYAMLFile`, proving the `Config.Validate` wiring;
- the four `DesignConfig.Validate` refusals, each asserting `config_invalid`, the exact
  message and a non-empty `NextAction`.

Verified myself rather than trusting the report: `go test -shuffle=on ./internal/config/` and
`go test -shuffle=on ./...` both green, oracles are hand-written literals, and
`withProjectSchema` is a pre-existing helper in `internal/config/repo_test.go` (same package),
not a new one.

Two deliberate omissions, both sound and worth keeping:
- **Criterion 4 is only half-assertable in this phase.** Nothing resolves a design location to
  an absolute path yet, so the test pins "carried and written as declared". Resolution from the
  settings folder is Phase 1.3's job and is where the other half gets asserted.
- **No new migration-registry assertion.** `internal/migrate/registry.go` still registers only
  `project1to2`/`project2to3`, `CurrentProjectSchema` is still 3, and
  `internal/migrate/registry_test.go` already covers the registry's contents; the
  schema-unchanged half is asserted in the round-trip test instead.

`cmd/root_test.go`'s `writeCurrentConfig` and `cmd/spec_test.go`'s `writeSpecCommandConfig`
needed no change, as expected: they write no `design:` key and the absent section is valid.

Pre-existing, unrelated to this work: a `go.mod` diagnostic suggesting
`github.com/spf13/pflag` should be a direct dependency. `go.mod` was not touched by this phase.

### Phase 1.1 verification (verify step)

All green, delegated per the step and consistent with the runs I did myself:

- `go build ./...`, `go test -shuffle=on ./...` (21 packages ok), `go vet ./...`,
  `gofmt -l internal/config/` — all pass.
- The 8 new design tests: 8 ran, 8 passed.
- Non-bump check: `internal/config/schema.go:18` still `CurrentProjectSchema = 3`, and
  `internal/migrate/registry.go` still registers only `{project1to2, project2to3}` for
  `KindProject`. `go test ./internal/migrate/` passes, so the registry test that pins the
  constant to the registered steps is satisfied.
- Diff scope: exactly `internal/config/config.go`, `internal/config/config_test.go`, plus the
  two Spektacular bookkeeping files. No stray edits.

### Phase 1.1 plan update (update_plan step)

Phase 1.1's heading and all five acceptance criteria flipped to `[x]` in the plan store via
`plan file read` → stage → `plan file write` (never the Edit tool). Phase 1.2 is now the first
unchecked phase.

**One criterion needed work before the box could honestly flip.** Criterion 4 ("a relative
location is understood as relative to the folder holding the settings file") was only half
satisfied: the tests prove the declared value is carried and written back verbatim, but
nothing in this phase *joins* it to the settings folder — that is `internal/design.NewSet`'s
job in Phase 1.3. Since the plan's Component Breakdown names the config layer as the owner of
the rule, I stated the rule on the `DesignConfig` doc comment (relative locations resolve from
the folder holding `config.yaml`; the declaration is never re-expressed on write, unlike a
`storeDirs()` entry; a location outside the project root is allowed). The behavioural half is
asserted in Phase 1.3, which is where the plan puts it. Build, gofmt and the package tests
stayed green after that edit.

### Phase 1.1 changelog (update_changelog step)

First `update_changelog` invocation for this plan, so a new `## Changelog` section was created
at the end of `plan.md` (after `## Out of Scope`) with the Phase 1.1 entry. Paths are prefixed
`spektacular:` because two repos are registered and the final feature-changelog step derives a
per-repo record from those prefixes.

The entry records the one deliberate deviation (all four `DesignConfig.Validate` refusals carry
a next action, unlike the knowledge validator's provider and location cases) and three
discoveries. 11 unchecked phases remain; Phase 1.2 is next.

Pending with the user at this point: whether to capture the `storeDirs()` coupling as a
knowledge entry, and whether to continue straight into Phase 1.2 or pause for review.

### Knowledge captured + autonomous mode (after Phase 1.1)

User answered both pending questions:

1. **Capture the `storeDirs()` coupling — accepted, "and also maybe add a comment to the
   method".** Both done. The entry is at `repo`/`spektacular`
   `gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md`, tagged
   `config, paths, storage, validation` (the `gotchas` tag I first floated was dropped as
   redundant with the category). Verified retrievable as the top hit. The matching doc comment
   is on `Config.storeDirs()` in `internal/config/config.go`, naming both effects
   (write-time re-expression, and the `escapesRoot` refusal) and both worked examples.
2. **"Continue, and don't ask again."** So from Phase 1.2 onward, drive the remaining phases
   straight through without the per-phase continue-or-pause prompt. Still stop for: a genuine
   STOP-on-mismatch, a failing verification, and the plan's two Open Questions (the settings
   format version bump, and the end-to-end capture assertion) — the first of those explicitly
   requires asking the user before bumping.

### Phase 1.2 implementation (implement step)

`internal/store/store.go` only. Split the seven-method `Store` interface into:

- `Reader` — `Read`, `List`, `Exists`, `Search`, each doc comment moved verbatim (including
  the long `Search` ranking contract, so nothing documented changed).
- `Writer` — `Write`, `Delete`, likewise verbatim.
- `Store` — `interface { Reader; Writer; Root() string }`, keeping its own doc comment.

`Reader`'s new doc comment states why it exists: so a source can hold a reader unconditionally
and a writer only when its provider can supply one, making a read-only backend a named refusal
rather than a violated `Write`/`Delete` contract.

Zero implementation and zero call-site changes, as the phase requires: `FileStore` and
`ignoreStore` already have every method and satisfy all three interfaces structurally.
`go build ./...` and the full `go test -shuffle=on ./...` suite pass untouched, which is the
real proof of behaviour-neutrality.

### Phase 1.2 tests (test step)

`internal/store/store_test.go` only: six compile-time assertions in a single `var` block,
`*FileStore` and `*ignoreStore` each against `Reader`, `Writer` and `Store`. No behavioural
tests, deliberately — the phase changed no behaviour and `store_test.go` / `ignore_test.go` /
`search_test.go` already cover both implementations, so anything more would be a redundant
assertion.

Two things worth keeping from this:

- **Assert the concrete decorator type, not the constructor's return value.**
  `NewIgnoreStore` and `NewSourceStore` are both declared as returning `Store`, so asserting
  their return value would only re-prove that `Store` embeds `Reader` and `Writer` and would
  say nothing about the implementation. The assertion names `*ignoreStore`, reachable because
  the tests are in the internal test package `store`.
- **The comment was tightened after review.** As first written it read as though the
  assertions guard "both directions of the split". They do not: a method added to `Store`
  directly rather than to `Reader` or `Writer` still compiles, because both implementations
  would carry it. The comment now states what the assertions do catch (an implementation
  losing a method, naming which interface) and records that keeping the split complete is a
  review obligation, with the rule for placing a new method.

Criterion 3 (no caller changes the type it works with) is not unit-assertable; it is proved by
`go build ./...` and the untouched suite passing. Full suite: exit 0, 21 packages ok.

### Phase 1.2 verification (verify step)

`go build` exit 0; `go test -shuffle=on ./...` exit 0 with 21 ok and 0 FAIL; `go vet ./...`
exit 0; `gofmt` clean. Criterion 3 proved directly rather than by assertion: **87**
`store.Store` references across `cmd/` and `internal/` and **0** references to `store.Reader`
or `store.Writer` anywhere outside the interface declaration and its test, so no existing
caller changed the type it works with. Only four files differ from HEAD, two of them Phase
1.1's.

**Process note**: I ran this verification in the main context with output digested to files
rather than delegating to a sub-agent as the step suggests. For a behaviour-neutral refactor
whose verification is four commands with one-line outputs, a sub-agent costs ~50k tokens and a
minute to re-run what I had already run, and the step's stated purpose (keep full command
output out of the main context) is met by the digest. Medium and High complexity phases still
get a delegated verification.

### Phase 1.3 implementation (implement step)

New package `internal/design`, three files:

- **`design.go`** — `Source`, `Document`, unexported `resolvedSource`, `Set`, `NewSet`, and the
  addressed methods `Sources`, `List`, `Read`, `Write`, `Resolve`, `Exists`. `NewSet` iterates
  `cfg.Design.Sources`, switches on provider with a single `case config.ProviderFile` and a
  `default:` that refuses by name, resolves a relative location onto
  `config.ProjectConfigDir(projectRoot)`, stats it, and builds the store with
  `store.NewSourceStore(location, "design:"+name)` so `.spektacular_ignore` filtering comes
  free. The package doc states the deliberate parallel with `internal/knowledge` and why no
  shared abstraction was extracted.
- **`errors.go`** — the six refusals, each `output.NewError(code, msg).WithResource(...)
  .WithNextAction(...)`: `design_source_unknown`, `design_address_incomplete`,
  `design_source_unreachable`, `design_provider_unsupported`, `design_source_read_only`,
  `design_not_found`.
- **`paths.go`** — `isAbs`/`joinPath`/`isNotFound` helpers plus the `providerFile` alias, so
  the error constructors stay pure message text and the provider switch cannot drift from the
  error wording.

Decisions worth carrying:

- **Every addressed method looks the source up before touching a store**, so
  `design_source_unknown` and `design_not_found` can never be confused. A source name is never
  resolved on the caller's behalf even when only one is declared (000047's predictability
  rule).
- **`Exists` was added** to the projection beyond the five methods the plan lists. `design ref
  list` in Phase 2.2 needs "is this document there?" per reference, and doing it via `Read` and
  discarding the bytes would read a whole file to answer a boolean.
- **`availableSources` distinguishes "wrong name" from "no sources declared"** and says so in
  the next action, rather than handing back an empty list.
- **The read-only seam is real**: `resolvedSource.writer` is nil-able and `Write` refuses with
  `design_source_read_only` when it is nil. Reachable in a test today with a stub source, which
  is what makes it more than a speculative abstraction.
- **`List` sorts paths within a source** and walks sources in declaration order, so listings
  are stable run to run.

Verified by a throwaway smoke test covering every behaviour below before deleting it, so all of
it is known-working: two sources resolving (one entirely outside the project root, the
success-metric case), relative resolution from the settings folder, fan-out listing with
recursion tagged by source, `--source` narrowing, byte-for-byte write/read round-trip including
CRLF and trailing whitespace and a document carrying its own frontmatter, and all six refusals
including a location that is a file rather than a directory. `go build`, `go vet` and `gofmt`
clean.

### Phase 1.3 tests and verification (test + verify steps)

`internal/design/design_test.go`, 15 focused tests, all 6 phase criteria covered and green
under `-shuffle=on` and `-count=3`. Verification: build, vet, gofmt clean; suite exit 0,
22 ok of 23 packages, 0 FAIL.

Worth keeping from the test design:

- **`snapshot` is a genuinely independent oracle.** It walks the filesystem with
  `filepath.WalkDir` + `os.ReadFile` directly, never through the design package, and is what
  proves "the pre-existing folder is unchanged" for the out-of-project-root case. That is the
  right shape for an oracle here: the subject under test must not be asked whether it modified
  anything.
- **The relative-location test uses a decoy.** It creates `<root>/design/decoy.md` alongside
  the real `<root>/.spektacular/design/real.md`, then asserts the listing contains only
  `real.md`. That distinguishes "resolved from the settings folder" from "resolved from the
  project root", which a single-directory fixture cannot.
- **The `${VAR}` case goes through a written `config.yaml` and `config.FromYAMLFile`**, since
  expansion is the config loader's behaviour and not `NewSet`'s. Incidental constraint learned
  building that fixture: `config.Validate` requires at least one `repos` entry, so any
  config-file fixture needs `schema`, `name`, `command` and a `repos` entry alongside the block
  under test.

**Stale-diagnostic note**: the editor reported `undefined: store.Reader` / `store.Writer` in
`internal/design/design.go` at this point. It was a stale language-server cache, not real:
`go build ./...`, `go vet` and the whole suite pass, and `Reader`/`Writer`/`Store` are declared
at `internal/store/store.go:72`, `:100` and `:110`. Trust the compiler over the editor here.

### Phase 1.4 implementation (implement step)

New `cmd/design.go` plus one line in `cmd/root.go` registering `designCmd` after
`knowledgeCmd`. Subcommands `sources`, `list`, `read`, `write`, each with `--schema`;
`-d/--data` on read and write, `--from <path>` on write, `--source <name>` on list.

- A file-level comment states plainly why this family is **not** a fourth `newStoreFileCmd`
  registration (that factory stamps lifecycle frontmatter on write and re-reads it on list),
  so a maintainer does not try to consolidate them.
- `design read` writes raw bytes to stdout, matching `spec file read` / `plan file read`.
- `design write` requires `--from`; a new `design_from_required` refusal says content comes
  from a file, never from prose on the command line.
- `design_data_required` replaces the knowledge family's bare `fmt.Errorf` for a missing
  `--data`, so the missing-payload case also carries a next action.
- Smoke-tested end to end against a temp project with two sources (one outside the project
  root): sources with resolved locations, fan-out list tagged by source, `--source` narrowing,
  raw-byte read, `write --from` then read-back, and all three `--schema` outputs.

**Defect found and fixed in my own work**: `designReadOutputSchema` originally advertised
`{"content": string}`, but `design read` writes the document's raw bytes. A schema that lies is
worse than no schema, because an agent that trusts it would try to parse a Markdown document as
JSON. It now publishes `{"type":"string"}` with a comment explaining why.

**Learned about the harness**: `--schema` is gated behind the project and version checks in the
root command's pre-run, so it does not work outside a valid project or against a config the
running binary considers out of date. That is pre-existing behaviour shared with the knowledge
family, not something this phase introduced. Also, a hand-written fixture `config.yaml` needs
`skills_version` as well as `schema`, or every command fails `upgrade_required`.

### Phase 1.4 tests and verification (test + verify steps)

`cmd/design_test.go` (9 test functions, 535 lines) plus two subtests appended to
`cmd/no_project_test.go` for `design sources` and `design read`. Verified independently: build,
vet, gofmt clean; `go test -shuffle=on -count=2 ./...` exit 0, 22 ok, 0 FAIL. The `-count=2`
run is the one that matters here, since it is what catches a cobra flag leaking between
shuffled tests. Confirmed the sub-agent left `cmd/design.go`, `cmd/root.go` and `internal/`
alone: `cmd/root.go`'s only diff is my single `rootCmd.AddCommand(designCmd)` line, and the
corrected `designReadOutputSchema` is intact.

Reuse worth remembering: **`cmd/init_test.go` already has a `snapshotDir` helper**
(`filepath.WalkDir` + `os.ReadFile` + sha256) and `cmd/spec_test.go` has
`writeSpecCommandConfig`, which supplies `schema`, `skills_version`, `name` and the `repos`
entry. The design tests reuse both, adding only a thin `writeDesignConfig` that renders the
`design:` block and delegates. A near-duplicate snapshot helper was written first and collided
at compile time, which is how the existing one was found — worth grepping `cmd/*_test.go` for a
helper before adding one.

**Another stale diagnostic**: the editor reported `snapshotDir redeclared` in
`cmd/init_test.go` after the collision was already resolved. Only one declaration exists and
the package compiles and vets clean. Same lesson as the `store.Reader` diagnostic earlier:
trust the compiler.

### Phase 2.1 implementation (implement step)

`internal/metadata/metadata.go` and `merge.go`:

- `DesignRef{Source, Path}` with both yaml and json tags, and `Designs []DesignRef` on
  `Metadata`, with a comment recording *why* it must be modelled: `yamlShape` is a closed
  schema, so an unmodelled key is dropped on the next render.
- `Designs []DesignRef \`yaml:"designs,omitempty"\`` on `yamlShape` (the load-bearing edit) and
  a raw `yaml.Node` on `yamlInShape`, decoded by a new `decodeDesignRefs` that is lenient in the
  same way `document_status` is: a non-sequence node yields no references, an entry that fails
  to decode is skipped, and an entry missing either half of the address is dropped.
- `UpdateOptions.Designs *[]DesignRef` — pointer-typed because three states are distinct and
  the sibling string fields cannot express them: nil = no change, non-nil = replace, non-nil
  empty = clear.
- `Merge` carries `current.Designs` forward in the preserve branch unless `opts.Designs` is
  non-nil, and the doc comment gains the matching invariant. **This is the single site that
  makes a reference durable**, because the spec workflow commits by writing a freshly assembled
  body over the stored file.

`cmd/storefile.go`'s `provenanceOpts` was deliberately left alone: it builds `UpdateOptions`
without `Designs`, so an ordinary `spec file write` passes nil and preserves references, which
is exactly the required behaviour.

Smoke-verified before handing off: an artifact with no references renders no `designs` key at
all (byte-identical to pre-change output); two references render nested under `designs:`; a
body-only rewrite with nil `Designs` preserves them; a non-nil empty slice clears them; and all
five lenient-read cases (absent, empty list, scalar, mapping, entries missing a half) parse
without error, with partial entries dropped and complete ones kept.

### Phase 2.1 tests and verification (test + verify steps)

`internal/metadata/metadata_test.go` (4 cases appended), new `internal/metadata/merge_test.go`
(5 cases), and one focused test in `cmd/storefile_metadata_test.go`. Build, vet, gofmt clean;
`go test -shuffle=on -count=2 ./...` exit 0, 22 ok, 0 FAIL. Non-test files carry only my own
edits.

Notes worth keeping:

- **The durability property is now pinned twice, at two levels**: `TestMerge_BodyOnlyRewrite
  PreservesDesigns` at the unit level, and `TestStoreFileWrite_SpecDesignReferencesSurvive
  OrdinaryWrite` at the command level, which drives a real `spec file write` and proves
  `provenanceOpts` passing a nil `Designs` preserves rather than clears. That is not a redundant
  pair: the unit test pins `Merge`'s contract, the cmd test pins that the write path actually
  uses it that way.
- **The cmd test seeds the reference into frontmatter on disk**, because no verb records one
  yet. `design ref add` is Phase 2.2, and a record-then-rewrite test through the CLI only
  becomes possible then. Worth revisiting in 2.2 to see whether it is then worth replacing the
  seeded fixture with the real verb, or whether that would just be the same assertion by a
  longer route.
- **`designCmd`'s `Short` already says "reference"** while only `sources`/`list`/`read`/`write`
  exist. Phase 2.2 lands the `ref` verbs immediately, so the string is one phase ahead rather
  than wrong, but if 2.2 were ever deferred that wording would need trimming.
- **`cmd/storefile_metadata_test.go` mixes two harness styles** — some existing tests use
  `rootCmd.SetArgs` + `rootCmd.Execute()` directly, others the `resetRootCmd`/`runRootCmd` pair.
  New tests use the pair only, per the order-independence convention. The older style in that
  file is pre-existing and was not touched.
- **Another round of stale editor diagnostics** claimed `DesignRef` undefined and
  `Metadata has no field Designs` while `go vet` and the full suite were clean. Third time this
  session; the language server lags well behind the compiler here.

### Phase 2.2 implementation (implement step)

New `cmd/design_ref.go`: `design ref add|remove|list`, registered onto `designCmd`. A
file-level comment records why the recorder lives in `cmd` rather than `internal/design` (that
package owns design sources; a spec lives in the spec store, and keeping the wiring here means
the design package never learns where specs are kept).

Shape of it:

- `designRefData` parses `--data`, with `requireDoc` false for `list`, which addresses only a
  spec. Three refusals: `design_data_required`, `design_ref_spec_required`,
  `design_address_incomplete`.
- `specStore` resolves the spec through the configured spec directory, appending `.md` to a
  bare name, and refuses a spec that is not stored with `design_ref_spec_not_found` pointing at
  `spec file list`.
- `writeRefs` rewrites **only** the frontmatter, carrying the body through untouched via
  `metadata.Split` + `metadata.Merge(raw, body, UpdateOptions{Designs: &next})`.
- `ref add` validates the source through `set.Resolve` **before** reading or writing the spec,
  so an undeclared source leaves the spec byte-identical. A duplicate pair is a silent no-op.
  A reference to a document that does not exist yet is allowed on purpose: the spec mandates
  exactly one refusal at record time, and requiring the document first would impose a
  pointless ordering between `design write` and `design ref add`.
- `ref remove` mirrors it; removing an absent reference succeeds and changes nothing.
- `ref list` always returns a **success** envelope with `refs[]` (`source`, `path`, `resolved`,
  `location`), an `unresolved` count, and a `next_action` only when that count is non-zero.
  `output.ErrorResponse` has fixed fields and cannot carry a list, and the plan workflow needs
  the whole picture in one call. A reference whose *source* is no longer declared is still
  listed, with an empty location and counted unresolved: a reference the project can no longer
  address is precisely what this command exists to surface.

Smoke-verified end to end: add; duplicate no-op; a reference to a not-yet-written document;
`ref list` reporting one resolved and one unresolved with absolute locations and the
`next_action`; the spec body untouched with references nested under `designs:` in frontmatter;
the same design recorded on two specs independently; an undeclared source refused with the
declared names listed and the spec left clean; remove; and removing an absent reference as a
no-op.

### Phase 2.2 tests and verification (test + verify steps)

`cmd/design_ref_test.go`, 12 focused tests, all driven through `resetRootCmd`/`runRootCmd`.
Build, vet, gofmt clean; `go test -shuffle=on -count=2 ./...` exit 0, 22 ok, 0 FAIL.

Test-design points worth keeping:

- **The `next_action` key is omitted entirely when everything resolves**, not set to an empty
  string. The test pins key *absence* via `map[string]json.RawMessage` + `require.NotContains`,
  and the author verified the assertion was not vacuous by temporarily inverting it and
  watching it fail. That is the right way to prove a negative assertion actually bites.
- **Read/resolve is exercised independently of write.** Tests 5 and 6 seed the `designs:` block
  as hand-written frontmatter rather than calling `ref add`, so the resolution path is pinned
  without depending on the write path, and the one place add-time laxity is asserted
  (recording a reference to a document that does not exist yet) stays unambiguous. The
  "source no longer declared" case *has* to be seeded by hand, since `ref add` would refuse it.
- **The stored file is pinned by two complementary assertions, not one redundant pair**: one
  test pins the frontmatter block, the other pins the body and the absence of the design's
  content. Neither asserts whole-file equality, which would have made the other pointless.
- **Frontmatter is re-rendered with yaml.v3's 4-space sequence indent** and a quoted date. A
  hand-written 2-space fixture reads fine but is rewritten to 4-space on the first `ref add` or
  `ref remove`, so expected literals must use the rendered form.

Fourth round of stale editor diagnostics (`metadata.DesignRef` undefined, `no field Designs`)
against a clean `go vet` and a green suite. Consistent pattern this session; ignore the editor.

### Phase 3.1 implementation (implement step)

New `templates/agents/design-trigger.md` (heading `## Design-Worthy Detail Recognition`) and
`internal/agent/design_trigger.go`, with `installDesignTriggerSection` added to all three
agents' sequences (`claude.go`, `bob.go`, `codex.go`) after `installSpecTriggerSection` and
before `installDraftPresentationSection`.

The template's wording:

- Names the spec's four triggers (an API's shape, a user-facing flow, a data format, or a
  worked example of any of these) and sets a **deliberately high bar** as a three-part test the
  detail must pass on all counts: settled (the user decided it, not floated it), worked (a
  concrete shape rather than a direction), and would make the spec unreadable inline. It
  restates the constraint / technical-direction / design-document split so an agent does not
  promote a one-line steer into a document.
- Reuses the accept / defer / decline vocabulary from `knowledge-trigger.md` **verbatim in
  shape**, including the decline-finality wording, because agents meeting two different
  vocabularies for the same mechanic is a known failure mode here. The decline branch adds one
  thing the knowledge version has no need for: declining must not smuggle the detail into the
  spec body instead.
- Accept names **both** steps, `design write` then `design ref add`, and says why both are
  required every time: a document nothing references is invisible to the plan workflow, and a
  reference to a document never written is a broken reference.
- Names `{{command}} design ...` directly and **never** `{{command}} skill <name>`, per the
  gotcha recorded in plan 000041 that skill lookup does not resolve for skills nested under
  `templates/skills/workflows/`.
- Closes with "silence or deflection is not acceptance", plus the spec's own carve-out that a
  direct instruction to write a design *is* the required agreement.

Smoke-verified: installing claude, bob and codex into one temp dir leaves exactly one copy of
the heading, in the order memory → knowledge → spec → **design** → draft-presentation →
historical, with `{{command}}` rendered to the configured command.

**Note on the existing heading-order test**: the plan expected
`internal/agent/spec_trigger_test.go`'s order assertion to need updating in this edit. It does
not *fail*, because it only pins the relative order of the four headings it names and the new
section slots between two of them. It should still be extended to name the design heading so
its position is pinned rather than incidental — that is the test step's work, not a fix.

### Phase 3.1 tests and verification (test + verify steps)

`internal/agent/design_trigger_test.go` (11 test functions) plus one edit extending
`TestInstallSpecTriggerSection_CrossAgentIdempotency` in `internal/agent/spec_trigger_test.go`
to count the design heading exactly once and pin its position in the order chain between
Spec-Worthy and Presenting Drafts. Build, vet, gofmt clean; `-shuffle=on -count=2` exit 0,
22 ok, 0 FAIL.

The tests split into a machinery battery against an in-memory fixture template (creates from
missing, appends after an existing block, idempotent, preserves surrounding content, picks up a
template change, cross-agent idempotency) and a content battery against the real embedded
template, rendered with `Command: "go run ."` so a placeholder leak cannot masquerade as a
pass. Anchor phrases are counted exactly once: offers-rather-than-writes, silence-is-not-
acceptance, the three outcome labels, decline-finality, both `design write` and `design ref add`
inside the accept branch only, and a negative guard that no `skill spek-` path appears.

`internal/agent/instruction_surface_test.go` was deliberately left alone: it does not enumerate
the managed AGENTS.md sections at all. Its tables are a different surface (forbidden stdin and
heredoc patterns, stale boolean-retrieval claims, knowledge subcommands, per-skill content) and
it never reads AGENTS.md or `templates/agents/*`.

**My own mistake, worth not repeating.** To check the extended order assertion was not vacuous
I temporarily moved the design install ahead of the spec install in `claude.go`, saw the test
fail as it should, then ran `git checkout -- internal/agent/claude.go` to undo it. That file's
Phase 3.1 edit was never committed, so the checkout reverted **my implementation change** along
with the experiment, silently dropping `installDesignTriggerSection` from the claude sequence
while bob and codex kept theirs. Caught immediately because the test still failed afterwards,
and restored. The right technique in an uncommitted working tree is to copy the file into the
scratch directory first and restore from that copy, never `git checkout --`.

Worth knowing before reformatting: **the wording guards match against single source lines of
`templates/agents/design-trigger.md`.** The template is hard-wrapped, so an anchor phrase
spanning a wrap would never match, and every asserted phrase was chosen to sit on one line. A
future reflow of that file could fail these tests without changing its meaning. That brittleness
is the point for a wording guard, but it is a trap if someone reflows the prose.

### Phase 3.2 implementation (implement step)

Six template edits, no Go changes, no new steps and no new interruption points:

- **`templates/steps/spec/05-technical_approach.md`** — the capture offer, placed immediately
  after the "Compress each to a one-line steer" sentence, which is the exact point a worked
  design would otherwise be lost. Same three-part bar and same accept/defer/decline vocabulary
  as the AGENTS.md section, plus the spec-specific rule that the design's content does not also
  go into the section and that declining does not move the detail into the spec body.
- **`templates/steps/plan/02-discovery.md`** — in Step 2, after the always-applied knowledge
  load: run `design ref list`, read every resolved reference in full with `design read`, treat a
  referenced design as binding input in the same way a knowledge entry is, and **STOP and report
  when `unresolved > 0`** rather than planning around the gap.
- **`templates/steps/plan/03-architecture.md`** — after "Ground each option in the research
  findings": options are weighed *within* a referenced design, never treating it as one
  candidate among the two or three, and believing a design wrong is a decision to raise with the
  user rather than make silently.
- **`templates/steps/plan/07-dependencies.md`** — one bullet per design document with its path
  and source, and an explicit "none" statement when the spec carries no references.
- **`templates/skills/workflows/spek-new/SKILL.md`** and **`spek-plan/SKILL.md`** — a section
  each introducing design documents and naming the commands that reach them.

Verified by rendering the real steps: discovery carries `design ref list`, `design read` and the
stop-on-unresolved instruction; architecture carries build-on-not-re-derive; dependencies
carries the naming rule and the explicit-none form; the spec step carries the offer with both
commands, decline-finality and silence-is-not-acceptance, with no unrendered placeholder and no
`skill spek-` path. `templates/scaffold/` is untouched, so `specStillScaffold` and the harbor
`SCAFFOLD_LEFTOVERS` / `EXPECTED_PLAN_SECTIONS` oracles are unaffected. Full suite exit 0.

**Two errors of my own, both caught before handing off:**

1. I nearly used `{{spec_name}}` without checking it exists in spec step templates. It does (32
   uses), but the plan-side templates use `{{plan_name}}` instead, and a wrong placeholder
   renders literally rather than failing loudly.
2. I wrote three `--data` examples in the skill files opened with `'{` and closed with `}"` —
   unbalanced quotes, so an agent copying them verbatim would issue a malformed command. Found
   by grepping for `}"` + backtick and fixed; every `--data '{` in both files now has a matching
   `}'`. Worth a guard: nothing in the test suite would have caught this, because the contract
   tests assert phrases are *present*, not that quoted shell examples are well formed.

### Phase 3.2 tests and verification (test + verify steps)

7 contract tests added across `internal/steps/spec/steps_test.go` (4) and
`internal/steps/plan/steps_test.go` (3), all via the existing `renderStep` helpers, counting
anchor phrases exactly once. Plus `TestWorkflowSkillsDocumentDesignCommands` in
`templates/skill_list_command_test.go` and a new `templates/data_payload_wellformed_test.go`.
Build, vet, gofmt clean; `-shuffle=on -count=2` exit 0.

**The `--data` guard is real and proven.** `TestDataPayloadExamplesAreWellFormed` walks all 80
`.md` files under `templates.FS` and checks each `--data '{` opener closes with `}'` by counting
braces. I proved it non-vacuously by reintroducing the exact malformed payload I had written
earlier and watching it fail, then restoring from a scratch copy. Two details that make it work:
brace *counting* rather than first-`}` matching, because mustache placeholders nest inside
payloads (`--data '{"step":"{{next_step}}"}'` opens three braces, and a naive scan would
false-positive on all 60 `goto` examples); and a `checked >= 70` floor so a walker that silently
stops finding examples fails rather than passing vacuously. The check is scoped to single-line
examples: the four genuine multi-line cases all break *after* `--data`, leaving the payload
intact on one line, and a future mid-payload wrap would be reported as unclosed, which is the
right prompt to unwrap it.

**Restoring from a scratch copy is the technique that works.** `cp <file> <scratch>` before an
experiment, `cp` back after. This is the correct version of the mistake I made in Phase 3.1.

**The skill table was extended rather than shoehorned, correctly.**
`TestWorkflowSkillsDirectAgentToCLIList`'s rows are `{skill, listCmd, storeDir}` and two of its
three assertions concern a store *directory* the agent must not poke with `ls`/`find`/`Read`.
Design documents have no such directory — they live wherever the project declares — so a row
there would have needed a meaningless `storeDir` and weakened an existing guardrail. A separate
table-style test asserts `spek-new` names `design sources`/`list`/`write`/`ref add` and
`spek-plan` names `design ref list`/`design read`.

**One claim in the hand-back was wrong, and checking mattered.** It closed by suggesting
`templates/agents/design-trigger.md`'s two command examples are "currently unasserted" and might
need a test. They are asserted: Phase 3.1's
`TestRenderedDesignTriggerAcceptBranchNamesBothCommands` (`internal/agent/design_trigger_test.go`)
pins both commands inside the accept branch specifically. Acting on that suggestion would have
added a redundant assertion. Sub-agent reports are worth verifying, particularly their
speculative closing notes.

### Phase 3.3 reconciliation (analyze/implement, runs pending)

Went through every hand-maintained expectation and **confirmed rather than assumed**:

- `EXPECTED_STEP_ORDER` in both the plan and spec suites matches the step tables in
  `internal/steps/{plan,spec}/steps.go` exactly. No step was added, as designed.
- `EXPECTED_SKILLS_PER_STEP`, `EXPECTED_SPAWN_STEPS`, `SCAFFOLD_LEFTOVERS` and
  `EXPECTED_PLAN_SECTIONS` are unaffected: this feature added CLI commands to step prose, not
  skill references, and touched no scaffold.
- Both `solution/solve.sh` goto sequences name only steps that exist.
- The spec suite's `technical_approach` assertions concern the produced section's length, not
  the instruction's, so a longer instruction does not disturb them.

**Two pieces of pre-existing drift found, from other features, not this one:**

1. **Repaired: the plan suite's status-lifecycle oracle.** It asserted `status: in-progress` →
   `status: completed`, but the code has used the key `document_status` with the vocabulary
   draft/final/superseded/archived since the document-status-vocabulary work. The whole
   `TestArtefactStatusLifecycle` class would have failed. Repaired to `document_status` /
   `draft` / `final`, verified against `internal/steps/plan/steps.go:324`
   (`metadata.Close(st, path, metadata.StatusFinal)`) and against a committed plan document on
   disk, with a comment recording that the rename happened elsewhere and this oracle was not
   moved with it. Fixed under Phase 3.3's third criterion ("any failure is fixed rather than
   recorded as pre-existing") rather than left for whoever ran the suite next.
2. **Not a defect after all: the implement suite's seeded `config.yaml` has no `schema:` key.**
   Read in isolation it fails `upgrade_required` (format 1, needs 3), which looked like drift
   from the schema-versioning work. It is not: that suite's `instruction.md` runs
   `spektacular init claude` first, and `init` stamps `schema: 3` onto an existing config while
   preserving its contents. Verified empirically by copying the seeded files into a temp project
   and running init — the config came out at schema 3 and `design sources` then answered
   cleanly. **No change made.** Worth recording because the static reading is misleading, and
   "fixing" it would have been a change with no cause.

**Decisions taken, both of which the plan asked to be explicit:**

- **Added** a `DESIGN_REF_LIST_COMMAND = "design ref list"` oracle beside
  `CONVENTIONS_READ_COMMAND`, asserting the obligation fires inside the discovery window. The
  prose obligation is unconditional, it mirrors an established pattern in the same file, and
  nothing else in the suite would notice the obligation disappearing. Deliberately **no content
  half**: the environment seeds no design source, so the command returns an empty list and there
  is nothing to digest. The comment records that this is the realistic no-design-documents shape
  and that the point is the obligation firing and degrading cleanly rather than being skipped
  because it has nothing to report.
- **Did not seed a `design:` block** into the implement suite's environment. Absence is valid
  and now verified to behave correctly (`design ref list` on a project with no design block
  returns `refs: []`, `unresolved: 0`, exit 0), so seeding one would add fixture surface and a
  matching directory in the Dockerfile for no coverage this feature needs.

### Phases 4.1-4.3 implementation (done ahead of the FSM, while the harbor run was in flight)

**4.1** New `docs:src/pages/design-documents.mdx`, seven `<Section>` bands alternating from
plain, plus a nav entry after Projects in the Resources group. Built from `Hero`, `Section`,
`Prose`, `CtaBanner` and `Button` only. Guards: **0** em dashes, **0** matches for
`<div|<section|class=`, `npm run build` succeeds, `npx astro check` reports 0 errors and 0
warnings (the 1 hint is pre-existing, `document.execCommand` in a copy-button component, not on
this page).

**Deviation from the plan's content outline**: it called for the when-to-use rule of thumb "as a
short table". I used bold-lead bullets instead, because `.spek-body` in `src/styles/global.css`
carries no table styling, so a markdown table would render as a bare unstyled HTML table.
Adding table CSS would be a site-wide styling change well outside this phase. The bullet form is
the `ConfigKey` body shape the site already styles.

**4.2** `docs:src/pages/configuration.mdx`: a `<ConfigKey name="design" type="section">` entry
inserted between the `knowledge` and `repos` entries (anchored on those component boundaries,
not line numbers, because that file carries uncommitted work), the `design:` block added to the
worked example after `knowledge:`, and the stated top-level key count corrected from Twelve to
Thirteen with `design` added to the list. The repository-keys block deliberately gets **no**
design entry, since a repo does not declare design sources. Build and check still clean.

**4.3** `README.md` at five sites: the feature list gains a design-documents bullet beside the
knowledge-base one; the two-file split sentence now says `config.yaml` holds the design sources;
the worked project example gains the `design:` block; and the one-base relative-path paragraph
covers `design.sources[].config.location` and states the deliberate difference from the store
directories, that a design location may resolve outside the project and is written back as
declared rather than re-expressed. `go test ./cmd -run 'TestREADME|TestKnowledgeDocs'` passes.

### Gotcha: a running harbor job breaks `go test ./...` at the walk stage

While the plan-workflow harbor suite was running, `go test -shuffle=on ./...` started returning
exit 1 with:

```
pattern ./...: open tests/harbor/jobs/<job>/agent/sessions/sessions: permission denied
FAIL	./... [setup failed]
```

**No test failed.** All 22 packages report `ok`; the error is Go's package *walk* hitting a
directory the in-flight container wrote that the host user cannot read. `tests/harbor/jobs/` is
gitignored, so it never shows up in `git status` either, which makes the cause easy to miss. It
presents exactly like a broken build and is not one. Check for `[setup failed]` and a
`permission denied` under `tests/harbor/jobs/` before believing a sudden whole-suite failure,
and re-read the per-package `ok` lines, which are still printed.

### Phase 3.3: plan-workflow harbor run PASSED

`make harbor-test-plan`: **92 passed, 0 failed.** Two results carry real weight beyond "green":

- **`test_design_references_resolved_during_discovery` passed.** My new oracle fired, which
  means a real agent driving the real workflow actually ran `design ref list` inside the
  discovery window. That is evidence the Phase 3.2 prose obligation *lands*, not merely that it
  renders. The contract tests can only prove the instruction contains the words.
- **Both `TestArtefactStatusLifecycle` tests passed**, validating the status-oracle repair
  against a real run: committed documents read `document_status: draft` during the walkthrough
  and `final` after the finished step, exactly as the repaired literals assert. Had I guessed
  the vocabulary wrong, this is where it would have shown.

Also confirmed unaffected, as predicted: step order, skill retrievals, spawn steps, all eleven
plan sections, the conventions oracle, and the assumption log.

**After the run** I renamed two test functions in that class
(`test_artefacts_completed_after_finished` → `test_artefacts_final_after_finished`,
`test_docs_in_progress_during_walkthrough` → `test_docs_draft_during_walkthrough`) so the names
match the vocabulary the rest of the class now uses. Half-renamed was worse than either
extreme. The bodies are byte-identical to the ones that passed and pytest discovers on the
`test_` prefix, so the rename cannot change the outcome; re-running 20-25 minutes to confirm a
function rename would not be proportionate.

### Phase 3.3: spec-workflow harbor run FAILED on a timeout (18 failed, 27 passed)

**Root cause: `AgentTimeoutError`. Elapsed 903.5 s against a 900 s budget. It missed by 3.5
seconds, 0.4% over.** The agent completed 9 of the 11 steps (new, interview, overview,
requirements, acceptance_criteria, constraints, technical_approach, success_metrics, non_goals)
and was inside `verification`, the step that commits the spec, when it was killed. All 18
failures are downstream of that single cause: the spec was never written to the store, so every
section-content assertion and `test_spec_file_write_command_used` fail as consequences, not as
independent defects.

**First: `make` exited 0 despite 18 failures.** The Makefile's last action is `@cat` of the
verifier output, so `make harbor-test-spec`'s exit code reflects the `cat`, not the tests. Never
trust that exit code; read the summary line. (I had also piped the plan run through `tail`
earlier, which made *that* exit code `tail`'s. Two separate ways to be misled about the same
thing.)

**The capture offer is not the defect, and the evidence is specific.** The agent ran
`spektacular design sources` **exactly once** out of 17 total Bash calls, found no declared
sources, and moved on. No runaway probing, no confusion, no retry loop. The prose landed and
behaved correctly; it just costs conversation.

**Baseline and attribution.** The suite passed 45/45 twice on 2026-09-04 and 44/45 once. My
change adds 34 lines to `templates/steps/spec/05-technical_approach.md`, growing that file from
29 to 63 lines, so it is a real contributor. But two other commits touched the spec step
templates since that green run, and a 3.5 s miss on a 900 s budget means the suite was already
sitting at its limit. Attributing this solely to this feature would be overclaiming; so would
calling it purely pre-existing.

**Budget context**: spec-workflow 900 s, implement-workflow 1200 s, plan-workflow 1800 s,
repo-workflow 600 s. The spec suite drives the longest interview on close to the smallest
budget.

**Not fixed unilaterally.** The plan's Open Question 2 says explicitly that if the agent does
not reach the offer reliably it is a prose defect to be raised with the user rather than worked
around; here the agent *did* reach it, and the question is instead whether the suite's budget is
stale. Raising a timeout changes a fixture everyone's runs depend on, and trimming the prose
trades away the three-part bar and outcome rules the spec asked for. Put to the user with the
evidence above.

### Phase 3.3: spec timeout raised to 1200s (user's decision)

User chose to raise the budget rather than trim the prose or re-run and hope.
`tests/harbor/spec-workflow/task.toml`: `timeout_sec` 900 -> 1200, matching implement-workflow.

**This is the second raise for the identical failure mode.** The existing comment already
documented the same story at 600s: "one run was killed mid-verification, the next completed
every step and wrote the spec but was killed before emitting its result event". So rather than
just change the number I extended the comment with the new evidence (903.5s against 900s, 0.4%
over, nine of eleven steps done, agent behaving correctly with 17 tool calls and no retry loop)
and named the pattern: every feature that adds prose to a spec step lengthens this conversation,
nothing fails at the time, and the next person to run the suite pays. The comment now says that
a **third** raise is the signal to question whether the interview has grown too long, rather
than to buy more time again.

That framing matters more than the number. This is the same class of problem as the stale status
oracle found earlier in this phase: a hand-maintained expectation that silently stops matching
reality. The difference is that a timeout degrades gradually instead of breaking outright, so it
gives no signal at all until it crosses the line.

### Phase 3.3: spec-workflow harbor run PASSED at 1200s -- and a correction

`make harbor-test-spec`: **45 passed, 0 failed**, no timeout. Matches the suite's historical
green state exactly.

**Correcting my own diagnosis.** The passing run took **446.5 s**, which would have fitted
inside the old 900 s budget with room to spare, running the identical prose. So the 903.5 s run
was an outlier rather than a new baseline, and this suite's runtime spans at least 446-903 s.
The 900 s budget sat *inside* that spread, which means it was always going to fail
intermittently and the run that happened to cross the line says little about what changed.

I had written into `task.toml` that "the step prose had simply grown again". That was my
hypothesis before I had the second data point, and it would have misled the next reader into
blaming whichever prose edit came last. Rewritten to state the variance evidence, to say
explicitly that a timeout here is not evidence the last prose edit was too long, and to tell
the next person to measure elapsed time in the transcript before concluding anything. The
third-raise guidance now says to check the distribution: several runs clustering near the
ceiling means the interview is too long, and buying time would only defer that.

The raise is still the right call, for a better reason than I first gave: 1200 s puts the budget
above the observed spread instead of inside it.

### Test plan written (test_plan step)

`000054_project-level-design-documents/test-plan.md` in the plan store. Four procedures, one
per success metric with a manual half; the "adopt without relocating" metric is fully
behavioural and correctly has none. Each procedure states what is **already automated** before
what needs a person, so nobody re-does by hand what the suite covers, and each is grounded in
the shipped commands rather than planning-time guesses.

**This is where the plan's second Open Question is answered, explicitly, as it demanded.** The
end-to-end spec suite does **not** assert that an accepted capture leaves both a design document
and a reference. Reasons, recorded in the artifact itself: the harbor spec environment declares
no design source, so asserting the accept path would mean seeding one *and* scripting the user
side of a conversation whose content drives whether the offer fires at all, which makes the
assertion a test of the script rather than of the behaviour; and that suite's runtime already
spans 446 to 903 seconds. It is a manual procedure instead, procedure 2, which also covers the
decline path, since "declining silently wrote something anyway" is the more damaging failure.

Worth noting what the harbor runs *did* establish, which is most of what that assertion would
have: a real agent reaches the design commands and uses them correctly (`design sources` once,
no probing loop), and the plan workflow's obligation fires unprompted. What remains unverified
by machine is only the accept branch producing both halves.

### Feature changelog written (update_feature_changelog step)

Three records. Affected repos were derived mechanically from the `<repo>:` prefixes in the
phase entries' Files-changed lists, which is exactly what those prefixes were for: 39 distinct
files in `spektacular`, 3 in `docs`.

- Project-level: `changelog/spektacular/000054_project-level-design-documents.md`.
- `spektacular` repo record, via `--repo spektacular`.
- `docs` repo record, via `--repo docs`, which landed in the docs repo's own store at
  `../spektacular-website/.spektacular/changelog/spektacular/000054_...md` beside the 000052 and
  000053 records. The `--repo` flag stamped project, spec and plan provenance into the
  frontmatter automatically; the body carries the readable reference line.

Each repo record opens with a summary written for someone who has never seen the plan, since it
is the only changelog written into that repo. The project record carries all five deviations
with their reasoning, and closes on verification including the one deliberate coverage decision.

### Spec reconciled (reconcile_spec step)

All 12 requirements and all 11 acceptance criteria flipped to `[x]`, judged against the plan's
`## Changelog` rather than against the original intent. Verified first that the spec carries
checkboxes only under those two sections, so nothing else was touched.

**Two criteria deserve their caveat stated rather than buried**, because both describe behaviour
in a live conversation and are verified by instruction contract tests plus manual procedure, not
by an automated end-to-end assertion:

- *"Design capture during a spec is offered, never automatic"* - the offer, the three outcomes
  and the decline rule are pinned by contract tests on the rendered instruction, and the harbor
  runs show a real agent reaching the design commands and using them correctly. The accept and
  decline branches producing the right artifacts in a live conversation is test-plan procedure 2.
- *"Planning records the design it read"* - the dependencies step's naming obligation is pinned
  by a contract test, and the harbor plan run confirms the agent runs `design ref list` during
  discovery unprompted. What was never exercised end to end is a plan for a spec that actually
  *carries* references, because no harbor fixture has one; seeding that was explicitly out of
  scope.

I checked both, on the grounds that the mechanism is delivered and asserted, and that the plan
itself treated the near-identically worded Phase 3.2 criterion as satisfied by its contract
test. The residual is a claim about agent behaviour over time, which is the same class the test
plan already isolates as manual for four of the five success metrics. Flagged to the user rather
than left implicit.

### Workflow finished

`implement` reached its terminal `finished` state for 000054_project-level-design-documents.
12/12 phases checked, 23/23 spec checkboxes reconciled, test plan and all three changelog
records written. Final gates: build, vet, gofmt clean; `go test -shuffle=on -count=2` green.

Deliberately **not** done: nothing was committed. The work sits in the working tree of both
repos for review.
