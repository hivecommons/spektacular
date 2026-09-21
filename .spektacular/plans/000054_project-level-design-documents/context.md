---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Context: 000054_project-level-design-documents

## Current State Analysis

**There is no design-document concept anywhere in the codebase today.** No `design` command, no
`internal/design` package, no `templates/scaffold/design*.md`, no config key, no knowledge
category. Verified by grep across both registered repos.

**What a spec can do with a settled design today, and why it is not enough.** A spec is a single
file with seven fixed sections (`templates/scaffold/spec.md`). The section where mechanism is
welcome is Technical Approach, and its step template explicitly pushes worked design *out*:
`templates/steps/spec/05-technical_approach.md:15` tells the agent "Stay at direction altitude,
do not write the design itself", with a test for having dropped too low ("a numbered pipeline or
algorithm, step-by-step processing, data shapes, field or function names") and an instruction to
"compress each to a one-line steer". Technical Approach is also declared non-binding: the plan
"may adopt, adapt, or replace" it. So a settled design placed there is both compressed and
discardable. No plan step template mentions an external document or following a link out of the
spec, so a design referenced informally has no guaranteed reader.

**The four existing artifact classes and how they are reached.** Specs, plans and changelog
records are each one store, addressed by path, and their `file` command groups are all a single
factory call apart: `newStoreFileCmd(short string, dir storeDirFunc, requireID, repoRouted bool)`
at `cmd/storefile.go:172`, wired at `cmd/file.go:7-14` (spec), `cmd/plan_file.go:8-14` and
`cmd/changelog_file.go:20-25`. It provides `write`, `read`, `delete`, `list` and
`set-document-status` (`cmd/storefile.go:414`). Its `write` merges Spektacular's frontmatter
(`:210-236`) and its `list` re-reads every entry to inject `created_date`/`document_status`/
`closed_date` (`:304-336`) — the two behaviours that make it unusable for design documents.

**Knowledge is the one existing multi-named-source subsystem, and it is the model.**
`config.KnowledgeConfig{Sources []SourceConfig}` (`internal/config/config.go:127-176`) is already
"a list of named sources, each naming a provider and a location". `SourceConfig` carries
`Name`, `Provider`, `Config.Location` and a `yaml:"-"` `Tier` stamped only during aggregation.
Project-tier sources are declared as that list; a repo declares one unnamed store instead
(`RepoKnowledgeConfig`, `:138-153`), and a sources list in a repo's settings is actively refused
(`internal/config/repo.go:96-123`). Aggregation, tier stamping and location resolution happen in
`cmd/knowledge.go:286-342`; duplicate names within a tier are refused at `:349-365`. Addressing
is `--data '{"tier":…,"name":…,"path":…}'` via `knowledgeAddressInput`
(`cmd/knowledge.go:549-553`). This project currently declares **no** project-tier knowledge
source, so `design.sources` will be the first project-tier sources list its config carries.

**Why the store shape cannot be used.** `Config.storeDirs()`
(`internal/config/config.go:357-363`) lists the spec, plan and changelog directories;
`resolveStoreDirs`/`resolveStoreDir` (`:365-393`) re-express them project-root-relative and
`escapesRoot` (`:396-399`) causes `Validate` to refuse anything outside the project root.
Knowledge locations bypass all of that: `internal/knowledge/set.go:96-106` joins a relative
location onto `config.ProjectConfigDir(projectRoot)` (`internal/config/config.go:219-221`),
`os.Stat`s it, and refuses only when it is not a directory. The spec's "no files moved"
success metric therefore forces the knowledge path.

**The store layer.** `store.Store` (`internal/store/store.go:64-97`) is a single seven-method
interface: `Root`, `Read`, `Write`, `Delete`, `List`, `Exists`, `Search`. `ErrNotFound`
(`:12-13`) is the only exported sentinel. `FileStore` (`:101-196`) bakes in a filesystem root,
path-escape rejection, `MkdirAll` on write, non-recursive `List`, and idempotent `Delete`.
`NewSourceStore(root, label)` (`internal/store/ignore.go:73-79`) is what every call site
actually uses; it wraps `FileStore` with `.spektacular_ignore` filtering that applies to `List`
and `Search` but never blocks a directly-named path. There is **no** interface segregation and
**no** capability detection anywhere: the only existing "can this provider do X" check is a
provider-string comparison, in three places
(`internal/repo/footprint.go:78-101`, `internal/project/init.go:94-98`,
`cmd/knowledge.go:327`). `ProviderGit` exists but never produces a `Store`; it resolves a repo's
code to a local directory that a `FileStore` is then pointed at.

**Frontmatter is a closed schema.** `metadata.Metadata`
(`internal/metadata/metadata.go:59-70`) carries `CreatedDate`, `DocumentStatus`, `ClosedDate`
plus four provenance fields used only by derived changelog entries. `yamlShape` (`:72-81`) and
`yamlInShape` (`:83-93`) are the on-disk twins, and anything not modelled in them is **dropped**
when the block is re-rendered. Reads are deliberately lenient (`:112-118`) and writes strict.
`Merge` (`internal/metadata/merge.go:38`) is the single site that preserves existing fields
across a body-only rewrite — which matters because the spec workflow commits by writing a freshly
assembled body over the stored file (`templates/steps/spec/08-verification.md:120`).

**Settings format versioning.** `CurrentProjectSchema = 3` (`internal/config/schema.go:16-21`),
and `checkSchema` (`:87-98`) refuses any file whose `schema` is not exactly that. Loading never
upgrades; `internal/migrate` owns that, with `registry` and `current` pinned together by
`internal/migrate/registry_test.go:13-30`. No `KnownFields(true)` appears anywhere in
`internal/`, so an unknown key is silently ignored rather than rejected.

**The managed agent sections.** Six exist today (`templates/agents/`: repo-sources,
memory-context, knowledge-trigger, spec-trigger, draft-presentation, historical-artifacts),
each installed idempotently by `installManagedSection` (`internal/agent/managed_section.go:31-88`)
and called in a fixed order per agent (`internal/agent/claude.go:25-42`, and the bob/codex twins).
`internal/agent/spec_trigger_test.go:130-173` asserts the resulting heading order.
`templates/agents/knowledge-trigger.md` is the offer/accept/defer/decline text to mirror.

**Test baseline.** `go test -shuffle=on ./...` was green across all 21 packages before any change
in this plan. `make test` is that command; `make lint` is `go vet ./...`. The four end-to-end
suites (`tests/harbor/{spec,plan,implement,repo}-workflow`) run only by hand, take 20-25 minutes
each, and carry hand-written expectations that do not fail when a product surface drifts.

**The docs repo.** Ten pages under `src/pages/`, all on one `Shell.astro` layout.
`knowledge-base.mdx` is the concept-page precedent: Hero, then seven `<Section>` bands with
`surface` strictly alternating from false, each with a `<Fragment slot="sub">` lede and a
`<Prose nested>` body, closing with `<CtaBanner>` + `<Button>`. CLI usage lives on the concept
page; `configuration.mdx` carries only the settings keys and links back. Nav is a flat array in
`src/components/Nav.astro:1-21` with one "Resources" flyout. Build is `npm run build`; there is no
check script, so `npx astro check` is invoked directly. **In-flight work**: seven files are
uncommitted there, including roughly 130 lines in `configuration.mdx` from the settings-versioning
feature and a paragraph left unfinished mid-sentence in `knowledge-base.mdx` near the
"How a search is ranked" lede. Anchor edits on component boundaries, not line numbers, and leave
that paragraph alone.

## Per-Phase Technical Notes

### Phase 1.1: Declare design sources in project settings

- `internal/config/config.go:127-176` — add `DesignConfig{Sources []SourceConfig}` beside `KnowledgeConfig`. Reuse `SourceConfig` and `FileKnowledgeConfig` unchanged; do **not** rename them, since `SourceConfig`'s doc comment already describes a generic named store and a rename would churn `internal/knowledge` and `cmd/knowledge.go` for nothing. Leave `SourceConfig.Tier` unset for designs: design is project tier only.
- `internal/config/config.go:243-265` — add `Design DesignConfig \`yaml:"design,omitempty"\`` to `Config`, after `Knowledge` and before `Repos`, so the marshalled key order matches the documented order.
- `internal/config/config.go:269-299` — `NewDefault()`: leave `Design` at its zero value, with a comment mirroring the existing `Knowledge` comment at `:295-298` explaining that the project declares design sources by hand.
- `internal/config/config.go:651-675` — add `DesignConfig.Validate()` modelled line for line on `KnowledgeConfig.Validate()`: empty name, duplicate name within the list, unsupported provider, empty location. Use `output.NewError("config_invalid", …).WithNextAction(…)` for the name cases exactly as the knowledge version does, and keep the message text pointing at `design.sources` rather than `knowledge.sources`.
- Find `Config.Validate()` (the aggregate, which already calls `c.Knowledge.Validate()`) and add the `c.Design.Validate()` call beside it.
- **Do not** add an entry to `storeDirs()` at `internal/config/config.go:357-363`. That list drives `resolveStoreDirs`/`fileFormStoreDir` and `escapesRoot` refusal (`:376-399`), which would forbid a location outside the project root. Design locations are resolved in `internal/design`, the way knowledge locations are resolved in `internal/knowledge/set.go:96-106`.
- **Do not** touch `internal/config/schema.go:16-21` or `internal/migrate/registry.go:59-68`. `internal/migrate/registry_test.go:13-30` pins the constant to the registered steps, so moving either without the other fails.
- `internal/config/config_test.go` — add cases beside the existing knowledge-source cases (around `:661-772`): a config with two design sources round-trips; a config with none loads with `Design.Sources` nil and marshals without a `design:` key; each of the four validation refusals; a relative location is preserved as written by `ToYAMLFile`.
- `cmd/root_test.go:88-104` (`writeCurrentConfig`) and `cmd/spec_test.go:27-45` (`writeSpecCommandConfig`) — no change needed, since `design:` is optional, but confirm a fixture without it still loads.

**Complexity**: Low
**Token estimate**: ~12k
**Agent strategy**: Single agent, sequential. The change is one struct, one validator and its tests; splitting it would cost more in coordination than it saves.

### Phase 1.2: Name the read half of the storage interface

- `internal/store/store.go:64-97` — split the existing `Store` interface into `Reader` (`Read`, `List`, `Exists`, `Search`) and `Writer` (`Write`, `Delete`), with `Store` declared as `interface { Reader; Writer; Root() string }`. Move each method's existing doc comment with it verbatim, including the long `Search` contract at `:81-96`, so nothing about the documented behaviour changes.
- Nothing else in the package changes: `FileStore` (`internal/store/store.go:101-196`) and `ignoreStore` (`internal/store/ignore.go:61-124`) already implement every method and continue to satisfy `Store` by structural typing.
- Verify no call site changes: every construction goes through `store.NewSourceStore` (`internal/store/ignore.go:73-79`) and every consumer is typed `store.Store` — `cmd/storefile.go:93,134`, `cmd/spec.go:219,316`, `cmd/plan.go:134,203`, `cmd/implement.go:146,215`, `cmd/repo.go:247,315`, `cmd/artifacts.go:85,152`, `internal/knowledge/set.go:109`.
- `internal/store/store_test.go` — add a compile-time assertion that `*FileStore` satisfies `Reader`, `Writer` and `Store`, and that `NewIgnoreStore(...)` does too. This is the test that would catch a future method being added to `Store` but not to either half.

**Complexity**: Low
**Token estimate**: ~8k
**Agent strategy**: Single agent, sequential. Behaviour-neutral refactor; `go build ./...` plus the existing suite is the proof.

### Phase 1.3: Resolve declared sources into readable stores

- New `internal/design/design.go` — `Source{Name, Provider, Location}`, `Document{Source, Path}`, `Set`, `NewSet(cfg config.Config, projectRoot string) (*Set, error)`. Model `NewSet` on `internal/knowledge/set.go:93-119`: iterate `cfg.Design.Sources`, `switch src.Provider` with a single `case config.ProviderFile` and a `default:` that refuses by name, resolve a relative `src.Config.Location` with `filepath.Join(config.ProjectConfigDir(projectRoot), location)` (`internal/config/config.go:219-221`), `os.Stat` it and refuse when absent or not a directory, then build the store with `store.NewSourceStore(location, "design:"+name)` (`internal/store/ignore.go:73-79`) so `.spektacular_ignore` filtering comes for free.
- New `internal/design/errors.go` — the refusals, each `output.NewError(code, msg).WithResource(...).WithNextAction(...)` per `.spektacular/knowledge/conventions/error-messages-must-suggest-remediation.md`. Codes: `design_source_unknown` (message quotes the name, next action lists the declared names and points at `design sources`), `design_source_unreachable` (modelled on `knowledge.unreachableStore`, naming source, resolved path and the base it resolved from), `design_provider_unsupported`, `design_source_read_only`, `design_not_found` (names source, path and absolute location searched, next action points at `design list --source <name>`), and `design_address_incomplete` (a missing `source` or `path`).
- New `internal/design/set.go` methods — `Sources()`, `List(sourceName string)` (empty string fans out over every source in declaration order, tagging each `Document` with its source; recurse with `store.List` one level at a time as `internal/knowledge/set.go:614-639` does), `Read(Document)`, `Write(Document, []byte)`, `Resolve(Document) (string, error)`. Each addressed method looks the source up first and returns `design_source_unknown` before touching a store, so an unknown source and a missing document are never confused.
- The read-only seam: hold `reader store.Reader` always and `writer store.Writer` (nil-able) per source. The file provider sets both; `Write` on a source with a nil writer returns `design_source_read_only`.
- **Do not** add ranking, tags, categories or tier handling. `internal/knowledge/set.go:163-309` stays where it is; design documents are explicitly not indexed.
- New `internal/design/design_test.go` — `t.TempDir()` fixtures per `.spektacular/knowledge/conventions/tests-must-not-depend-on-order.md`. Cover: two sources in different locations both resolve; a location outside the project root resolves and is readable (the success-metric case that rules out the store-dir shape); a relative location resolves from the settings folder, not the process working directory; an absolute location is used as written; a `${VAR}` location expands (`internal/config/config.go:699-704`); a missing location and a location that is a file both refuse; an unknown provider refuses; an unknown source name refuses and the message lists the declared names; a missing document refuses differently; a stub source with a nil writer refuses a write.

**Complexity**: Medium
**Token estimate**: ~28k
**Agent strategy**: 2-3 parallel agents — one on `NewSet` plus resolution, one on the `Set` methods, one on the error constructors and their tests — then sequential integration, since they share the `Set` type.

### Phase 1.4: Reach design documents from the command line

- New `cmd/design.go` — `designCmd` (`Use: "design"`, `RunE: runUnknownSubcommand`, matching `cmd/knowledge.go:20-24`) with `designSourcesCmd`, `designListCmd`, `designReadCmd`, `designWriteCmd`. Register with `rootCmd.AddCommand(designCmd)` at `cmd/root.go:351-361`, after `knowledgeCmd`.
- Flags: `-d/--data` on `read` and `write` (mirroring `cmd/knowledge.go:605-606`), `--from <path>` on `write` (mirroring `cmd/storefile.go:189-238`'s required source-file flag), `--source <name>` on `list`, and `--schema` on each, following `knowledge`'s `--schema` support.
- Input type `designAddressInput{Source, Path string}` with JSON tags, modelled on `knowledgeAddressInput` (`cmd/knowledge.go:549-553`), and its `--data` helper producing the same style of guidance message as `cmd/knowledge.go:566-582` (`--data is required (e.g. --data '{"source":"api","path":"payments/v2.md"}')`).
- Output via `output.New(cmd.OutOrStdout(), globalFields)` and `output.Write` (`internal/output/writer.go:98-114`), so `"error": false` is injected consistently. `design read` writes raw bytes with `cmd.OutOrStdout().Write(content)` exactly as `cmd/storefile.go:240-260` does, not a JSON envelope.
- **Do not** call `newStoreFileCmd` (`cmd/storefile.go:172`): its `write` merges frontmatter at `:210-236` and its `list` re-reads and injects metadata at `:304-336`, both forbidden for design documents.
- **Do not** add a kind to `cmd/artifacts.go:20-31`; that command is scoped to workflow-produced artifacts.
- `cmd/no_project_test.go:23-59` — add `design sources` (and one addressed command) to the covered command list so the `no_project` gate is proven for the new family.
- New `cmd/design_test.go` — drive the commands through `resetRootCmd`/`runRootCmd` (`cmd/root_test.go:31-79`), never by calling `RunE` directly, so the new flags cannot leak between shuffled tests. Cover: `sources` lists both declared sources with resolved locations; `list` fans out and tags each document, and narrows with `--source`; `read` returns bytes identical to the file; `write --from` then `read` round-trips byte-for-byte including a document carrying its own frontmatter and one carrying none; the written document then appears in `list`; `--schema` returns the documented input/output shape for each subcommand; each refusal surfaces with its code and a non-empty `next_action`.

**Complexity**: Medium
**Token estimate**: ~26k
**Agent strategy**: 2 parallel agents — one on the command wiring and schemas, one on the test battery — then sequential integration.

### Phase 2.1: Make design references part of a spec's record

- `internal/metadata/metadata.go:59-70` — add `Designs []DesignRef` to `Metadata`, and declare `DesignRef{Source, Path string}` with `yaml:"source"` / `yaml:"path"` tags in the same file.
- `internal/metadata/metadata.go:72-81` — add `Designs []DesignRef \`yaml:"designs,omitempty"\`` to `yamlShape`. This is the load-bearing edit: the shape is closed, so a field absent here is dropped whenever the block is re-rendered.
- `internal/metadata/metadata.go:83-93` — add the matching field to `yamlInShape`. Decode leniently in line with the existing `DocumentStatus` treatment at `:113-118`: hold it as a `yaml.Node` or decode into a tolerant slice so a malformed or non-list value reads as no references rather than failing the parse. Drop entries missing a source or a path on read.
- `internal/metadata/metadata.go:96-110` (`MarshalYAML`) — carry `Designs` through, relying on `omitempty` so artifacts without references are byte-identical to today.
- `internal/metadata/merge.go:11-21` — add `Designs *[]DesignRef` to `UpdateOptions`, pointer-typed so that nil means "no change" and an empty slice means "clear", matching the `DocumentStatus *DocumentStatus` convention documented at `:7-10`.
- `internal/metadata/merge.go:38+` — in both the `fresh` and the preserve branch, carry `current.Designs` forward unless `opts.Designs` is non-nil. This is what makes a reference survive `spec file write` re-assembling the body (`templates/steps/spec/08-verification.md:120`).
- `cmd/storefile.go:143-151` (`provenanceOpts`) — leave alone; `design ref` supplies its own `UpdateOptions`, and an ordinary `spec file write` must pass nil so references are preserved, not cleared.
- `internal/metadata/metadata_test.go` / `merge_test.go` — cover: round-trip of a two-reference list; an artifact with no references marshals with no `designs` key and is byte-identical to the pre-change output; an absent, empty, malformed and non-list `designs` value all read as no references; a body-only merge preserves an existing list; `Designs` non-nil replaces it; an empty non-nil slice clears it; the created/status/closed invariants at `merge.go:22-37` are unchanged.
- `cmd/storefile_metadata_test.go:38-62` — extend the existing `kindFixtures()` table's spec row with a case proving a reference survives a subsequent `spec file write`.

**Complexity**: Medium
**Token estimate**: ~22k
**Agent strategy**: Single agent, sequential. The three files are tightly coupled and the invariants are subtle; parallelism risks inconsistent lenient-read behaviour.

### Phase 2.2: Record, remove and resolve a spec's design references

- `cmd/design.go` — add `designRefCmd` (`Use: "ref"`, `RunE: runUnknownSubcommand`) with `add`, `remove` and `list` subcommands, registered onto `designCmd`. Input `designRefInput{Spec, Source, Path string}` for add/remove and `{Spec string}` for list, each via `-d/--data` with `--schema` support.
- Spec resolution: read the spec through the configured spec directory the same way `storeFileStore` does (`cmd/storefile.go:84-94`, `cfg.Spec.Config.Directory`), appending `.md` when the caller gives a bare name, and refuse a spec that does not exist with a `next_action` naming `spec file list`.
- `ref add`: build a `design.Set`, refuse `design_source_unknown` **before** any write, then `metadata.Split` the stored spec, append the pair unless already present (a duplicate is a silent no-op), and write back with `metadata.Merge(existing, body, UpdateOptions{Designs: &next})`. The document need not exist yet: a design may be referenced before it is written, and `ref list` is what reports the gap.
- `ref remove`: the mirror; removing an absent reference succeeds and changes nothing.
- `ref list`: for each recorded reference call `set.Resolve` then `Exists`, emitting `{source, path, resolved, location}`; always return a success envelope carrying `refs`, `unresolved` and, when `unresolved > 0`, a `next_action` naming the design commands that fix it. Deliberately not an error envelope: `output.ErrorResponse` (`internal/output/writer.go:45-52`) has fixed fields and cannot carry the list.
- New `cmd/design_ref_test.go` — via `resetRootCmd`/`runRootCmd`. Cover: add then `spec file read` shows the reference and the body is unchanged; add with an undeclared source fails with the source named and the stored spec byte-identical afterwards; add is idempotent; remove leaves siblings; `ref list` reports resolved and unresolved entries with locations and a correct `unresolved` count; `design read` of a removed document fails with `design_not_found`; the same reference recorded on two specs shows independently on each and the design document is unchanged by either.

**Complexity**: Medium
**Token estimate**: ~26k
**Agent strategy**: 2 parallel agents — one on add/remove, one on list plus resolution reporting — then sequential integration on the shared spec-resolution helper.

### Phase 3.1: Give every agent the standing design-capture rule

- New `templates/agents/design-trigger.md` — heading `## Design-Worthy Detail Recognition`. Open with the managed-section notice in the existing form (`templates/agents/knowledge-trigger.md:3-5`), naming its own template path. Follow the four-part shape: framing, a worthiness test naming the spec's four triggers (an API shape, a user-facing flow, a data format, or a worked example of any of these) with an explicit high bar, the offer itself, and the accept / defer / decline outcomes. Reuse the decline-finality wording verbatim from `knowledge-trigger.md:29-33` so agents meet one vocabulary. Name `{{config.command}} design write` and `{{config.command}} design ref add` directly; do **not** reference a skill via `{{config.command}} skill …`, which does not resolve for skills nested under `templates/skills/workflows/`.
- New `internal/agent/design_trigger.go` — a direct copy of `internal/agent/knowledge_trigger.go` with `designTriggerTemplatePath = "agents/design-trigger.md"` and the matching heading constant.
- `internal/agent/claude.go:25-42`, and the same sequences in `internal/agent/bob.go` and `internal/agent/codex.go` — add `installDesignTriggerSection` after `installSpecTriggerSection` and before `installDraftPresentationSection`, so the design offer sits beside the other two recognition triggers. `internal/agent/spec_trigger_test.go:130-173` asserts the relative order of the managed headings; update that expectation in this same edit.
- New `internal/agent/design_trigger_test.go` — the full battery the sibling tests establish: creates from missing; appends after an existing block; idempotent; preserves surrounding content; picks up a template change (`internal/agent/knowledge_trigger_test.go:117-128`); cross-agent idempotency asserting exactly one heading after installing claude, bob and codex into one temp dir (`:130-153`).
- `internal/agent/instruction_surface_test.go` — check whether it enumerates the managed sections; if so, add the new one.

**Complexity**: Low
**Token estimate**: ~16k
**Agent strategy**: Single agent, sequential. The Go side is a near-verbatim clone; the care is all in the template's wording.

### Phase 3.2: Make the workflows offer capture and honour designs

- `templates/steps/spec/05-technical_approach.md:15` — immediately after the "Stay at direction altitude" paragraph, add the in-step offer. This is the exact point where the template tells the agent to compress a worked design to a one-line steer, so it is where the detail would otherwise be lost. Use the four-part shape from `templates/agents/design-trigger.md` and name `{{config.command}} design write` then `{{config.command}} design ref add --data '{"spec":"{{spec_name}}","source":"…","path":"…"}'` on acceptance. State explicitly that a decline leaves nothing written and the detail out of the spec body.
- `templates/steps/plan/02-discovery.md` — in Step 2 (Project Context), after the always-applied knowledge load and before the topic-specific search, add the design obligation: run `{{config.command}} design ref list --data '{"spec":"<spec name>"}'`, read every resolved reference in full with `{{config.command}} design read`, and **stop and report to the user** when `unresolved` is non-zero rather than continuing. Mirror the existing "Knowledge outranks the code it describes" paragraph's force.
- `templates/steps/plan/03-architecture.md` — in Step 1 (Weigh Options), add one paragraph: where the spec references a design, the design is the settled shape and is built on, not re-derived; options are weighed within it, and departing from it is a decision to raise with the user, not to make silently.
- `templates/steps/plan/07-dependencies.md` — add a rule requiring one bullet per design document read, naming the document and the source it came from, and requiring an explicit "none" statement when the spec carries no references. This is what satisfies the acceptance criterion that the plan names each design and its source.
- `templates/skills/workflows/spek-new/SKILL.md` and `templates/skills/workflows/spek-plan/SKILL.md` — one short paragraph each: that a spec may reference design documents, and that designs are reached through `{{config.command}} design …` rather than by reading files directly. Keep the phrasing compatible with `templates/skill_list_command_test.go:14-31`'s existing assertions; add a design row to that table if the skills name `design list`.
- Template-contract tests: extend `internal/steps/spec/steps_test.go` with a `renderStep`-based assertion (`:46-62`) that the technical-approach instruction carries the offer's anchor phrases and the decline-finality wording, plus a negative guard that it does not tell the agent to fetch a skill via `skill spek-…`. Extend `internal/steps/plan/steps_test.go` similarly for discovery (resolve-and-read, stop-on-unresolved), architecture (build on, do not re-derive) and dependencies (name each design and its source). Follow the exactly-once counting style of `templates/rejection_repair_directive_test.go:33-42`.
- **Do not** add a heading to `templates/scaffold/plan.md` or `templates/scaffold/spec.md`: `internal/steps/spec/steps.go:194-208` (`specStillScaffold`) and the plan equivalent compare against the freshly rendered scaffold, and the harbor `SCAFFOLD_LEFTOVERS` and `EXPECTED_PLAN_SECTIONS` oracles mirror those files.

**Complexity**: High
**Token estimate**: ~34k
**Agent strategy**: Parallel analysis, sequential integration — one agent drafting the spec-side offer and its tests, one drafting the three plan-side edits and theirs, then a single integration pass reconciling wording so both halves use one vocabulary.

### Phase 3.3: Bring the end-to-end suites back in step

- `tests/harbor/spec-workflow/tests/test_spec_workflow.py:20` (`EXPECTED_STEP_ORDER`) — unchanged, since no step is added; confirm rather than assume.
- `tests/harbor/plan-workflow/tests/test_plan_workflow.py:61-81,88-97,101,114-115,147-178` — review `EXPECTED_STEP_ORDER`, `EXPECTED_SKILLS_PER_STEP`, `EXPECTED_SPAWN_STEPS`, `CONVENTIONS_READ_COMMAND`, `SCAFFOLD_LEFTOVERS` and `EXPECTED_PLAN_SECTIONS` against the templates after Phase 3.2. The step order and plan sections should be untouched by design; if a command-substring oracle is added for the design read, it belongs beside `CONVENTIONS_READ_COMMAND`.
- `tests/harbor/implement-workflow/environment/config.yaml` — the seeded fixture carries one block per artifact class. Adding a `design:` block is optional (absence is valid) but is the realistic shape; if added, seed a matching design source directory in `tests/harbor/implement-workflow/environment/Dockerfile:24-27` so the source resolves.
- `tests/harbor/spec-workflow/solution/solve.sh` and `tests/harbor/plan-workflow/solution/solve.sh:18-33,299-303` — the goto sequences mirror the step tables and need no change; confirm.
- Optionally extend the spec suite's verifier with an assertion that a captured design lands in a declared source and the spec records the reference. Weigh the ~20-25 minute run cost; if not added, say so explicitly rather than leaving it implied.
- Run `make harbor-test-spec` and `make harbor-test-plan` (`Makefile`). These need the harbor CLI, Docker and Claude credentials and take roughly 20-25 minutes each. Per `.spektacular/knowledge/architecture/testing-architecture.md`, a failure here is fixed now, not deferred: prior plan 000040 spent four such runs clearing accumulated drift.

**Complexity**: Medium
**Token estimate**: ~18k plus run time
**Agent strategy**: Single agent, sequential. The work is comparison and reconciliation against surfaces the earlier phases changed, and the runs are serial anyway.

### Phase 4.1: Explain design documents on the documentation site

- New `docs:src/pages/design-documents.mdx` — frontmatter `title: "Design Documents - Spektacular"`, a one-sentence `description`, `layout: ../layouts/Shell.astro`, matching `docs:src/pages/knowledge-base.mdx:1-5`. Build it from the bands in the plan's Content outline, importing `Hero`, `Section`, `Prose`, `CtaBanner` and `Button` as that page does at `:7-15`.
- Shading: `<Section>` defaults to `surface={false}` (`docs:src/components/sections/Section.astro:10-15`), so alternate by adding a bare `surface` to every second band, exactly as `knowledge-base.mdx` does across its seven sections. Ledes go in `<Fragment slot="sub">`; bodies in `<Prose nested>` (`docs:src/components/sections/Prose.astro:1-6`).
- Code blocks are fenced markdown, not string props — `astro-expressive-code` is registered once in `astro.config.mjs` ahead of `mdx()` and processes every fence, and fenced blocks are what make `<placeholder>` text safe inside MDX.
- `docs:src/components/Nav.astro:12-20` — add `{ label: "Design Documents", href: "/design-documents/" }` to the `children` array of the `Resources` group, beside `Projects`. `isActive`/`matches` at `:25-30` need no change.
- Guards before calling it done: `grep -nE "<div|<section|class=" src/pages/*.mdx` must stay at zero matches; `npm run build` must succeed; `npx astro check` must report zero errors and zero warnings. No em dash anywhere in the added prose, per that repo's convention.
- Do not touch the half-finished paragraph currently uncommitted in `docs:src/pages/knowledge-base.mdx` around the "How a search is ranked" lede; it belongs to unrelated in-flight work.

**Complexity**: Medium
**Token estimate**: ~24k
**Agent strategy**: Single agent, sequential. One file plus a one-line nav edit; the cost is prose quality, not coordination.

### Phase 4.2: Document the new settings key in the configuration reference

- `docs:src/pages/configuration.mdx:200-214` — insert the new `<ConfigKey name="design" type="section" defaultValue="none">` block immediately after the existing `knowledge` key and before `repos`, in the exact form shown in the plan's Content example. Anchor on those two `<ConfigKey>` boundaries, not on line numbers: that file currently carries roughly 130 uncommitted lines from the settings-versioning work.
- `docs:src/pages/configuration.mdx:18-74` — add the `design:` block to the worked project settings example, after the `knowledge:` block, with the same inline-comment style.
- `docs:src/pages/configuration.mdx:76-82` — the `<Fragment slot="sub">` on the project keys block states how many top-level keys there are; increment it.
- `docs:src/pages/configuration.mdx:295-384` — the repository keys block gets **no** design entry. A repository does not declare design sources, and `internal/config/repo.go:96-123` already refuses a sources list in a repository's settings.
- Keep `surface` on the surrounding `<Section>`/`<ConfigurationKeys>` blocks as it is; `ConfigurationKeys` defaults to `surface={true}` (`docs:src/components/sections/ConfigurationKeys.astro:10`) and the insertion is inside one, so the alternation is unaffected.
- Same three guards as Phase 4.1, plus no em dash.

**Complexity**: Low
**Token estimate**: ~12k
**Agent strategy**: Single agent, sequential.

### Phase 4.3: Keep the repository's own configuration docs in step

- `README.md:161-200` — add the `design:` block to the project settings example, after `knowledge:`, with a comment noting that design sources are declared by the project only.
- `README.md:202` — the "Relative locations everywhere in `config.yaml` share one base" paragraph enumerates which keys it covers; add the design source location to that list.
- `README.md:14-20` — the feature list names the knowledge base among the project's capabilities; add one line for design documents so the concept is introduced where the others are.
- `README.md:150-157` — the two-file configuration split describes what `config.yaml` holds; add design sources to that sentence.
- `cmd/docs_test.go:29-40,145-172,198-240` — these assert specific README phrases are present and that superseded shapes are absent. None of the additions above should trip them, but run `go test ./cmd -run TestREADME` and `-run TestKnowledgeDocs` to confirm rather than assuming. If a new assertion is warranted for the design key, add it beside the existing README tests.
- `CHANGELOG.md` — the implement workflow no longer writes this file (removed by plan 000046), so leave it alone unless the release process asks for an entry.

**Complexity**: Low
**Token estimate**: ~10k
**Agent strategy**: Single agent, sequential.

## Testing Strategy

Per-phase, with the layer chosen by what is being guaranteed. Every Go test that executes the
command tree goes through `resetRootCmd`/`runRootCmd` (`cmd/root_test.go:31-79`) because
`make test` runs `-shuffle=on` and the commands are package globals whose flags persist.

- **Phase 1.1** — `internal/config/config_test.go`, beside the existing knowledge-source cases
  around `:661-772`: two design sources round-trip through `ParseYAMLFile`/`ToYAMLFile`; a config
  with no `design:` key marshals without one and is byte-identical to today's output; the four
  `DesignConfig.Validate` refusals each surface with code `config_invalid` and a non-empty
  `next_action`; a relative location survives the write-back unchanged.
- **Phase 1.2** — `internal/store/store_test.go`: compile-time assertions that `*FileStore` and
  the value returned by `NewIgnoreStore` satisfy `Reader`, `Writer` and `Store`. The real proof is
  that `go build ./...` and the whole existing suite pass untouched.
- **Phase 1.3** — `internal/design/design_test.go`, `t.TempDir()` throughout: two sources in
  different locations both resolve and are independently readable; **a source location outside the
  project root resolves and reads**, which is the success-metric case that rules out the store-dir
  shape; a relative location resolves from the settings folder rather than the process working
  directory; an absolute location is used as written; a `${VAR}` location expands; a missing
  location and a location that is a file each refuse with `design_source_unreachable` naming the
  resolved path and the base; an unrecognised provider refuses with `design_provider_unsupported`;
  an unknown source name refuses with `design_source_unknown` whose message lists the declared
  names; a missing document refuses with `design_not_found`, distinctly from an unknown source; a
  stub source constructed with a nil writer refuses a write with `design_source_read_only`.
- **Phase 1.4** — `cmd/design_test.go`: `sources` lists both declared sources with resolved
  locations; `list` fans out and tags each document with its source, and narrows with `--source`;
  `read` returns bytes identical to the file on disk; `write --from` then `read` round-trips
  byte-for-byte for a document with its own frontmatter, a document with none, and a document with
  CRLF line endings and trailing whitespace; the written document then appears in `list`; the
  source directory's other files are unchanged; `--schema` returns the documented shape for every
  subcommand; each refusal carries its code and a non-empty `next_action`. Plus `design sources`
  added to `cmd/no_project_test.go:23-59`'s covered list.
- **Phase 2.1** — `internal/metadata/metadata_test.go` and `merge_test.go`: a two-reference list
  round-trips; an artifact with no references marshals with no `designs` key and is byte-identical
  to the pre-change output; absent, empty, malformed and non-list `designs` values all read as no
  references without failing the parse; entries missing a source or a path are dropped on read; a
  body-only `Merge` preserves an existing list; a non-nil `Designs` replaces it; an empty non-nil
  slice clears it; every existing created/status/closed invariant at `merge.go:22-37` still holds.
  Plus a case on the spec row of `cmd/storefile_metadata_test.go:38-62`'s `kindFixtures()` proving
  a reference survives a subsequent `spec file write`.
- **Phase 2.2** — `cmd/design_ref_test.go`: `ref add` then `spec file read` shows the reference
  and the body is unchanged; `ref add` with an undeclared source fails with the source named and
  the stored spec byte-identical afterwards; `ref add` is idempotent; `ref remove` leaves siblings
  intact and removing an absent reference is a no-op; `ref list` reports `resolved` and `location`
  per reference with a correct `unresolved` count and a `next_action` only when that count is
  non-zero; `design read` of a removed document fails with `design_not_found`; the same reference
  recorded on two specs shows independently on each and the design document is unchanged by either;
  a reference to a not-yet-written design records successfully and reports as unresolved.
- **Phase 3.1** — `internal/agent/design_trigger_test.go`, the full battery the sibling tests
  establish (`internal/agent/knowledge_trigger_test.go:30-153`): creates from missing; appends
  after an existing block; idempotent; preserves surrounding content; picks up a template change;
  cross-agent idempotency asserting exactly one heading after installing claude, bob and codex into
  one temp dir. The heading-order expectation in
  `internal/agent/spec_trigger_test.go:130-173` is updated in the same edit.
- **Phase 3.2** — phrase assertions via `renderStep` (`internal/steps/spec/steps_test.go:46-62`
  and the plan equivalent), counted exactly once in the style of
  `templates/rejection_repair_directive_test.go:33-42`. Spec side: the technical-approach
  instruction carries the capture offer, the four triggers, and the decline-finality wording, and
  a **negative** guard that it does not tell the agent to fetch a skill via `skill spek-…`. Plan
  side: discovery carries resolve-and-read plus stop-on-unresolved; architecture carries build-on
  rather than re-derive; dependencies carries name-each-design-and-its-source. Plus a row in
  `templates/skill_list_command_test.go:14-31` if the skills name `design list`.
- **Phase 3.3** — no new Go tests. Reconcile the hand-written expectations at
  `tests/harbor/plan-workflow/tests/test_plan_workflow.py:61-178` and
  `tests/harbor/spec-workflow/tests/test_spec_workflow.py:20`, then run `make harbor-test-spec`
  and `make harbor-test-plan` and fix what fails rather than recording it as pre-existing.
- **Phases 4.1-4.2** — no unit tests exist for the docs site. The gates are
  `grep -nE "<div|<section|class=" src/pages/*.mdx` returning zero matches, `npm run build`
  succeeding, `npx astro check` reporting zero errors and zero warnings, and a manual read for em
  dashes.
- **Phase 4.3** — `go test ./cmd -run TestREADME` and `-run TestKnowledgeDocs` against the
  existing assertions in `cmd/docs_test.go:29-40,145-172,198-240`.

**Deliberate gaps.** No non-file provider is exercised, because none ships; the read-only path is
proven with a stub source instead. No search or ranking tests, because design content is not
indexed. No performance tests: every operation is a direct file read or write against a
caller-supplied path.

**Success-metric verification** carries over from plan.md unchanged: the three metrics that mix a
testable mechanism with a field observation are tested at the mechanism level and flagged
**manual — captured in the implementation test plan** for the observation; the "adopt without
relocating" and "one design, several specs" metrics are fully behavioural and are covered by the
Phase 1.3 out-of-project-root case and the Phase 2.2 two-spec case respectively.

## Project References

**Registered repos** (`go run . repo list`):

| Repo | Root | Role | What this plan changes there |
|---|---|---|---|
| `spektacular` | `/home/nicj/code/github.com/jumppad-labs/spektacular` | tool | Phases 1.1-1.4, 2.1-2.2, 3.1-3.3, 4.3 |
| `docs` | `/home/nicj/code/github.com/jumppad-labs/spektacular-website` | documentation | Phases 4.1-4.2 |

**Requirement to repo and files.** Every spec requirement lands in `spektacular` except the
documentation one:

| Spec requirement | Repo | Phase | Principal files |
|---|---|---|---|
| Design documents are a project artifact | `spektacular` | 1.1, 1.3 | `internal/config/config.go`, `internal/design/` |
| Stored where the project keeps them | `spektacular` | 1.1, 1.3 | `internal/config/config.go`, `internal/design/design.go` |
| Local file storage is supported | `spektacular` | 1.3 | `internal/design/design.go`, `internal/store/ignore.go` |
| Reachable through the CLI | `spektacular` | 1.4 | `cmd/design.go`, `cmd/root.go:351-361` |
| Users and agents can write them | `spektacular` | 1.4 | `cmd/design.go` |
| A spec can reference without restating | `spektacular` | 2.1, 2.2 | `internal/metadata/`, `cmd/design.go` |
| One design serves many specs | `spektacular` | 2.2 | `cmd/design.go` (reference verbs) |
| The spec workflow captures design detail | `spektacular` | 3.1, 3.2 | `templates/agents/design-trigger.md`, `templates/steps/spec/05-technical_approach.md` |
| Planning must honour a referenced design | `spektacular` | 3.2 | `templates/steps/plan/02-discovery.md`, `03-architecture.md`, `07-dependencies.md` |
| An unresolvable reference is reported | `spektacular` | 1.3, 2.2, 3.2 | `internal/design/errors.go`, `cmd/design.go` |
| Designs follow the spec's lifecycle | `spektacular` | 3.2 | plan step prose; no code, by design |
| Public docs explain the concept | `docs`, `spektacular` | 4.1, 4.2, 4.3 | `docs:src/pages/design-documents.mdx`, `docs:src/pages/configuration.mdx`, `README.md` |

**Knowledge entries that bind this work**, reachable with
`go run . knowledge read --data '{"tier":"repo","name":"<repo>","path":"<path>"}'`:

- `spektacular:conventions/error-messages-must-suggest-remediation.md`
- `spektacular:conventions/tests-must-not-depend-on-order.md`
- `spektacular:conventions/tests-must-pass-for-done.md`
- `spektacular:architecture/testing-architecture.md`
- `spektacular:architecture/working-with-files-from-steps.md`
- `spektacular:architecture/cli-design-for-ai-agents.md`
- `docs:conventions/mdx-authoring.md`, `site-layout.md`, `alternate-section-background.md`,
  `no-em-dashes.md`, `file-scoped-section-headings.md`, `plan-content-pages.md`

**Design documents read for this plan**: none. The spec carries no design references, because
this plan is what makes them recordable.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per-phase estimates and strategies are stated with each phase above. Rolled up: Low for 1.1, 1.2,
3.1, 4.2 and 4.3; Medium for 1.3, 1.4, 2.1, 2.2, 3.3 and 4.1; High for 3.2, which is the only
phase where two halves of one vocabulary must be reconciled and so needs parallel drafting
followed by a single integration pass.

## Migration Notes

**No settings migration is registered, deliberately.** `design` is an additive optional key whose
absence is valid, so `CurrentProjectSchema` stays at 3 and `internal/migrate/registry.go:59-68`
is untouched. `internal/migrate/registry_test.go:13-30` pins the version constant to the
registered steps, so bumping one without the other fails; if the decision is revisited (see
plan.md § Open Questions) both must move together, and the step would be a `project3to4` that only
stamps the number.

**What an older binary does with a new config.** No `KnownFields(true)` appears in `internal/`,
so a Spektacular built before this change reads a config carrying `design:` without error and
silently ignores it. Every design source is then invisible to that binary while everything else
works. This is the accepted trade-off for not bumping the format, and it is the one thing to
re-examine if users run mixed versions.

**No data migration of any kind.** No existing file is rewritten. A spec with no design references
marshals byte-identically to before, because the new frontmatter field is `omitempty`. No design
document is created, moved or reformatted by this change.

**Ordering constraint within the plan.** Phase 2.1 must land before Phase 2.2, because a reference
recorded before `Merge` preserves the field would be silently dropped by the next
`spec file write`. Phase 1.3 must land before 1.4 and 2.2, both of which construct a design set.

## Performance Considerations

Nothing here is on a hot path, and no measurement is planned.

- `NewSet` does one `os.Stat` per declared source at construction. A project declares a handful,
  so this is negligible, and it is the same eager-validation trade the knowledge set already makes
  (`internal/knowledge/set.go:103-106`): a misconfiguration is found immediately rather than on
  first use.
- `design list` without `--source` walks each source's tree one directory level at a time through
  `store.List`, as `internal/knowledge/set.go:614-639` does. A source pointing at a very large
  directory would be proportionally slow, but no index is added: design documents are not searched,
  so there is nothing to amortise an index against.
- `design ref list` does one `Exists` per recorded reference. A spec carries a few.
- `.spektacular_ignore` filtering comes free with `NewSourceStore` and is the escape hatch if a
  design source's folder contains material that should not be listed.
- The frontmatter change adds one optional list to a block that is already parsed and rendered on
  every artifact read and write. No new file reads are introduced anywhere.
