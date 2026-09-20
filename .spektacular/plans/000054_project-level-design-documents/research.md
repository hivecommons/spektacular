---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Research: 000054_project-level-design-documents

## Alternatives considered and rejected

**A full `design` workflow FSM (a fifth workflow beside spec/plan/implement/repo).**
A design document could have been produced by its own interactive step machine, with
`internal/steps/design/`, `templates/steps/design/*.md`, a harbor suite, rows in
`cmd/instruction_contract_test.go` (`contractWorkflows`, `cmd/instruction_contract_test.go:24`)
and `cmd/cross_kind_test.go`. Rejected: the spec asks for design documents to be *stored,
addressed, read and written*, not authored by a state machine. Requirement "Users and agents
can write design documents" is satisfied by a `write` command plus the capture-offer prose;
adding an FSM would drag in a cross-kind lock (`cross_kind_workflow_in_progress`,
`cmd/cross_kind_test.go:46`) that would block a design capture *during* a spec workflow,
which is exactly when the spec says capture must happen. Cost avoided: ~13 of the test
surfaces enumerated in the testing inventory.

**Store-shaped configuration: one project design store, like `spec`/`plan`/`changelog`.**
Model design storage on `SpecConfig`/`PlanConfig`/`ChangelogConfig`
(`internal/config/config.go:63-124`) with a single `design: {provider, config: {directory}}`
block, reached through the shared `newStoreFileCmd` factory (`cmd/storefile.go:172`).
Rejected on two counts. First, the spec requires several named locations
("The project can declare one or more named design sources"), which a single directory
cannot express. Second, and decisively, store directories are resolved by
`resolveStoreDir` (`internal/config/config.go:376-393`) and refused by `escapesRoot`
(`internal/config/config.go:396-399`) when they land outside the project root; success
metric "Teams adopt design documents without relocating anything" requires pointing at a
folder the team already has, which is frequently outside the project. Knowledge sources
have no such refusal (`internal/knowledge/set.go:96-106` resolves and `os.Stat`s, no
escape check), which is why the sources shape is the right precedent.

**Repo-tier design sources (a `design:` block in `repo.yaml`, like repo knowledge).**
Rejected by an explicit spec constraint ("Design sources are declared by the project only").
There is also a direct precedent against re-using the list shape at repo tier:
`rejectLegacyRepoKnowledgeBlock` (`internal/config/repo.go:96-123`) actively refuses a
`knowledge.sources` list in `repo.yaml`, because a repo has exactly one store addressed by
the name the project registered it under (`internal/config/config.go:134-141`).

**Recording design references in the spec body (a `## Design References` markdown section).**
A new scaffold section in `templates/scaffold/spec.md` plus an assembly row in
`templates/steps/spec/08-verification.md:12-18`. Rejected as the *source of truth*: the
acceptance criterion "A reference to an undeclared source is refused ... and no reference
is recorded" needs validation in Go at the moment of recording, and body prose assembled by
an agent cannot be validated or refused. Frontmatter is machine-readable and already has a
merge-preserving write path. A human-readable body section remains possible as a rendering
of the frontmatter, but is not the record.

**Free-form string references (e.g. `designs: ["designs/api.md"]`).**
Rejected by the spec constraint that "A design reference must identify both the source it
belongs to and the document within that source". A bare path cannot name its source, so an
unresolvable reference could not report *which* declared source was searched.

**Adding `Capabilities()` to `store.Store` for the read-only-provider question.**
A method like `CanWrite() bool` on the interface. Rejected in favour of interface
segregation: there is no capability-detection precedent anywhere in the codebase (confirmed
absent across `internal/store`, `internal/knowledge`, `cmd`), whereas Go's idiomatic answer
- a narrower interface plus an optional-interface assertion - costs nothing today because
`*store.FileStore` already satisfies both halves. The only existing "capability" gate is a
provider-string comparison (`internal/repo/footprint.go:78-101`,
`internal/project/init.go:94-98`, `cmd/knowledge.go:327`), which the plan should not extend
further.

**Bumping the project settings schema to 4 with a migration step.**
`project1to2` and `project2to3` exist because the *meaning* of existing data changed
(`internal/migrate/steps_project.go:90-134` re-expressed relative store directories).
Adding a new optional key whose absence is valid changes nothing about existing data, and
`registry.go:59-68` + `registry_test.go:13-30` would require a step that does nothing but
stamp a number, forcing every existing project through `migrate` for no content change.
Rejected - but see Open assumptions, since `checkSchema`
(`internal/config/schema.go:87-98`) refuses any file whose `schema` is not exactly current,
so the consequence of *not* bumping is that an older binary silently ignores a `design:`
block rather than refusing the file.

**Indexing design documents in `knowledge search`.**
Explicitly a spec non-goal, and the knowledge ranking pipeline
(`internal/knowledge/set.go:163-266`) assumes tier/category/tags frontmatter
(`internal/store/frontmatter.go`) that a user's own design document will not carry.
Rejected.

**Adding designs to `spektacular artifacts list`.**
`cmd/artifacts.go:1` describes that command as querying *workflow-produced* artifacts, and
`allArtifactKinds` (`cmd/artifacts.go:31`) enumerates spec/plan/changelog, all of which a
workflow writes. A design document is authored by a person or captured on request, not
produced by a workflow, so it does not belong in that listing. Rejected for this feature;
`design list` covers the requirement.

## Chosen approach — evidence

**Config shape mirrors `KnowledgeConfig`, project tier only.**
`internal/config/config.go:127-176` already defines exactly the shape the spec describes:
`KnowledgeConfig{Sources []SourceConfig}` where
`SourceConfig{Name, Provider, Config FileKnowledgeConfig{Location}, Tier}`. A `DesignConfig`
reusing `SourceConfig` is a two-field addition, and `KnowledgeConfig.Validate`
(`internal/config/config.go:651-675`) is a ready-made template for per-source validation
including the duplicate-name refusal built with
`output.NewError("config_invalid", …).WithNextAction(…)`.

**Relative locations resolve from the folder holding `config.yaml`.**
`config.ProjectConfigDir` (`internal/config/config.go:219-221`) and the knowledge resolution
at `internal/knowledge/set.go:96-106` are the exact pattern the spec constraint names
("Relative locations ... must resolve from the folder holding `config.yaml`, the same rule
every other relative path in that file follows"). Documented at `README.md:202`.

**Addressing mirrors `knowledge read`/`knowledge write`.**
`knowledgeAddressInput{Tier, Name, Path}` (`cmd/knowledge.go:549-553`) with
`--data '{"tier":…,"name":…,"path":…}'` (`cmd/knowledge.go:566-582`) is the established way
to address one document inside one of several named stores. Because design sources are
project tier only, the design equivalent drops `tier` and keeps `{source, path}`. The
`--schema` output on `knowledge read|write|list|sources` shows the machine-readable contract
these commands publish.

**Unknown-source refusal has a working precedent.**
`requireUniqueStoreNames` (`cmd/knowledge.go:349-365`) builds
`output.NewError("knowledge_store_name_duplicate", …).WithNextAction(…)`, and
`aggregateKnowledgeSources` (`cmd/knowledge.go:286-342`) is where an address is matched to a
declared store. The repo convention that every error names a concrete next step is
`.spektacular/knowledge/conventions/error-messages-must-suggest-remediation.md`.

**Store construction is a one-line reuse.**
`store.NewSourceStore(root, label)` (`internal/store/ignore.go:73-79`) wraps a `FileStore`
with `.spektacular_ignore` filtering, and is what every existing store call site uses
(`cmd/storefile.go:93`, `cmd/storefile.go:134`, `internal/knowledge/set.go:109`,
`cmd/artifacts.go:85`). A design source gets ignore-file support for free.

**Interface segregation for the read-only future.**
`store.Store` (`internal/store/store.go:66-97`) is one unified surface with seven methods;
`Write`/`Delete` bake in filesystem mutation and `Root()` returns an absolute directory path
that callers do `filepath` arithmetic against (`cmd/storefile.go:307`). Splitting a
`store.Reader` (Read/List/Exists/Search) out of `Store`, with `Store` retaining
`Reader + Write + Delete + Root`, leaves every existing call site compiling unchanged
because `*FileStore` and `ignoreStore` satisfy both.

**Design references belong in spec frontmatter, via `internal/metadata`.**
`metadata.Metadata` (`internal/metadata/metadata.go:59-70`) already carries non-lifecycle
fields (`Project`, `ProjectSource`, `Spec`, `Plan`) used only by derived changelog entries,
so a `Designs` field is in keeping. Critically, `yamlShape`
(`internal/metadata/metadata.go:72-81`) is a *closed* schema: anything not named in the
struct is dropped when the block is re-rendered, so a reference recorded outside the struct
would not survive the next `spec file write`. `metadata.Merge` (`internal/metadata/merge.go:38`)
is the single place that preserves existing frontmatter across a body-only rewrite, which is
how `templates/steps/spec/08-verification.md` commits an assembled spec (`spec file write
--from .spektacular/tmp/spec_template.md`) without losing `created_date`.

**Capture-offer prose has an exact template to copy.**
`templates/agents/knowledge-trigger.md` and `templates/agents/spec-trigger.md` are managed
AGENTS.md sections installed by `internal/agent/managed_section.go:31-88` and called per
agent from `internal/agent/claude.go:25-42` (and the bob/codex twins). Their tests
(`internal/agent/knowledge_trigger_test.go`, `internal/agent/spec_trigger_test.go`) give the
full battery a new managed section needs: creates-from-missing, appends-after-existing,
idempotent, preserves-surrounding-content, picks-up-template-change, cross-agent-idempotency.
`spec-trigger.md:11-15` additionally shows the config-knob pattern
(`spec_trigger_threshold`) if the design offer ever needs one.

**Plan-side obligation has a natural home.**
`templates/steps/plan/01-overview.md` is the step that reads the spec, and
`templates/steps/plan/02-discovery.md` Step 2 is the established "load referenced material
in full before designing" slot ("Knowledge outranks the code it describes"). The
Architecture step (`templates/steps/plan/03-architecture.md`) is where the design is chosen,
and the plan scaffold's `## Conventions` section (`templates/scaffold/plan.md:23-34`) is the
precedent for "carry external records forward and cite them inline".

**Docs precedent.**
`src/pages/knowledge-base.mdx` is the concept-page template (Hero, then `<Section>` bands
with strictly alternating `surface`, `<Fragment slot="sub">` ledes and `<Prose nested>`
bodies, closing `<CtaBanner>` + `<Button>`), and CLI usage lives on the concept page, not in
`configuration.mdx`. The `knowledge` `<ConfigKey>` at `src/pages/configuration.mdx:200-214`
is the exact form for a new `design` key. Nav is a flat array in
`src/components/Nav.astro:1-21`.

## Files examined

- `spektacular:.spektacular/config.yaml` — live project config; schema 3, no `design` key, `repos` lists `spektacular` and `docs`
- `spektacular:.spektacular/repo.yaml` — repo footprint; `knowledge` is a single unnamed block, not a list
- `spektacular:internal/config/config.go:28-37` — `ProviderFile`/`ProviderGit`; `file` is the only Store provider
- `spektacular:internal/config/config.go:63-124` — `SpecConfig`/`PlanConfig`/`ChangelogConfig` + their `File*Config` twins with the unexported `fileForm` round-trip field
- `spektacular:internal/config/config.go:127-176` — `KnowledgeConfig`, `RepoKnowledgeConfig`, `SourceConfig`, `FileKnowledgeConfig`: the sources shape to mirror
- `spektacular:internal/config/config.go:219-232` — `ProjectConfigDir`, `RepoEntry.ResolvedLocation`: the relative-path base rule
- `spektacular:internal/config/config.go:243-265` — top-level `Config`; the sibling key a `Design` field joins
- `spektacular:internal/config/config.go:269-299` — `NewDefault`; `Knowledge` deliberately left empty, the precedent for `Design`
- `spektacular:internal/config/config.go:350-399` — `storeDirs`/`resolveStoreDirs`/`escapesRoot`: why design sources must not be store-shaped
- `spektacular:internal/config/config.go:651-675` — `KnowledgeConfig.Validate`: per-source validation + duplicate-name refusal to copy
- `spektacular:internal/config/config.go:681-696` — `ToYAMLFile`: stamps schema and re-expresses store dirs on write
- `spektacular:internal/config/repo.go:96-123` — `rejectLegacyRepoKnowledgeBlock`: repo tier deliberately refuses a sources list
- `spektacular:internal/config/schema.go:11-21` — `CurrentProjectSchema = 3`; "rises by one only when that file's format changes"
- `spektacular:internal/config/schema.go:87-98` — `checkSchema` refuses any schema != current exactly
- `spektacular:internal/migrate/registry.go:31-68` — `Step`, `registry`, `current`: what a schema bump would cost
- `spektacular:internal/migrate/registry_test.go:13-30` — pins steps contiguous to the schema constant
- `spektacular:internal/migrate/steps_project.go:90-134` — `project2to3`, the only worked migration example
- `spektacular:internal/store/store.go:12-13` — `ErrNotFound` is the only exported sentinel
- `spektacular:internal/store/store.go:66-97` — the full seven-method `Store` interface
- `spektacular:internal/store/store.go:101-196` — `FileStore`: filesystem root, path-escape rejection, MkdirAll on write, non-recursive List, idempotent Delete
- `spektacular:internal/store/ignore.go:15,49-79` — `.spektacular_ignore`, `LoadIgnore`, `NewSourceStore`: the canonical store constructor
- `spektacular:internal/store/search.go:32-135,195-276` — in-process walk; the store finds candidates, the knowledge layer ranks
- `spektacular:internal/store/frontmatter.go` — `ParseEntry`: knowledge's lenient tags-only frontmatter, deliberately not shared with `internal/metadata`
- `spektacular:internal/knowledge/set.go:93-119` — `NewSet`: the provider switch that turns a source into a Store; single `case ProviderFile`, `default:` errors
- `spektacular:internal/knowledge/set.go:163-266` — ranking, tag re-enforcement, relative cutoff
- `spektacular:internal/metadata/metadata.go:20-50` — `DocumentStatus` vocabulary (draft/final/superseded/archived)
- `spektacular:internal/metadata/metadata.go:59-81` — `Metadata` + the closed `yamlShape`: unknown frontmatter keys are dropped on rewrite
- `spektacular:internal/metadata/frontmatter.go:16,59` — `Split`/`Render`; a document with no block is not an error
- `spektacular:internal/metadata/merge.go:11-38` — `Merge`/`UpdateOptions`: the single transition-logic site every write goes through
- `spektacular:internal/output/writer.go:45-98` — `ErrorResponse`, `NewError`, `WithResource`, `WithNextAction`, `Write`'s `"error": false` injection
- `spektacular:cmd/storefile.go:21,84-135,172,189-414` — `storeDirFunc`, `storeFileStore`, `repoRoutedStore`, `newStoreFileCmd(short, dir, requireID, repoRouted)` and its five subcommands
- `spektacular:cmd/file.go:7-14` — `spec file` wiring (the file is *only* this init)
- `spektacular:cmd/plan_file.go:8-14`, `cmd/changelog_file.go:20-25` — plan/changelog wiring; `requireID` and `repoRouted` are the only knobs
- `spektacular:cmd/artifacts.go:18-38,85,152-260` — artifact kind constants, `allArtifactKinds`, `scanArtifact`, per-kind append helpers
- `spektacular:cmd/artifactfilter.go:42-110` — the five shared list filters, reusable as-is
- `spektacular:cmd/knowledge.go:286-342` — `aggregateKnowledgeSources`: tier stamping and location resolution
- `spektacular:cmd/knowledge.go:344-365` — `requireUniqueStoreNames`: tier+name is the identity
- `spektacular:cmd/knowledge.go:549-606` — `knowledgeAddressInput` and the `--data '{"tier","name","path"}'` contract
- `spektacular:cmd/root_test.go:31-79` — `resetRootCmd`, `resetCommandFlags`, `runRootCmd`: the mandatory command-tree reset for order-independent tests
- `spektacular:cmd/cross_kind_test.go:20-58` — `cross_kind_workflow_in_progress`; the lock a design FSM would have collided with
- `spektacular:cmd/instruction_contract_test.go:22-52` — hand-maintained `contractWorkflows` + `stepTemplateTable`
- `spektacular:cmd/no_project_test.go:23-59` — every project-operating command must fail `no_project` naming `init`
- `spektacular:cmd/storefile_list_filter_test.go:46-62` — `listFilterFixtures()`: a table a fourth kind would join
- `spektacular:cmd/storefile_metadata_test.go:18-62` — `kindFixtures()`: the same, for write/status transitions
- `spektacular:cmd/docs_test.go:20-317` — README/CHANGELOG/`docs/knowledge-base.md` anchor-phrase assertions that new docs prose must not break
- `spektacular:templates/agents/knowledge-trigger.md` — the offer/accept/defer/decline text to mirror verbatim in shape
- `spektacular:templates/agents/spec-trigger.md:11-15` — the same pattern plus a config-threshold knob
- `spektacular:internal/agent/managed_section.go:31-113` — `installManagedSection`, `placeAtTop`/`placeAtEnd`, heading-match location
- `spektacular:internal/agent/claude.go:25-42` — the ordered list of `install*Section` calls a new section joins
- `spektacular:internal/agent/skills.go:26-68` — `workflowSkills` table and the `{{command}}`-only render of each `SKILL.md`
- `spektacular:internal/steps/spec/steps.go:25-39` — the spec step table; `technical_approach` is where design detail currently gets compressed away
- `spektacular:internal/steps/plan/steps.go:31-54` — the plan step table; `overview` reads the spec, `discovery` loads context, `architecture` decides
- `spektacular:internal/stepkit/stepkit.go:66-149` — `WriteStepResult`, var merge order (standard → strategy → Extra), `RenderTemplate`
- `spektacular:templates/scaffold/spec.md` — seven sections, each with an HTML comment block stating its format
- `spektacular:templates/scaffold/plan.md:23-34` — the `## Conventions` section: the precedent for carrying external records into a plan
- `spektacular:templates/steps/spec/05-technical_approach.md:15` — "Stay at direction altitude — do not write the design itself": the gap this feature fills
- `spektacular:templates/steps/spec/08-verification.md:5-18,120` — assembly from working files and the `spec file write --from` commit path
- `spektacular:templates/steps/plan/01-overview.md` — reads the spec; the earliest point a referenced design could be resolved
- `spektacular:templates/steps/plan/02-discovery.md` — Step 2 "Project Context", the load-referenced-material-in-full slot
- `spektacular:templates/steps/plan/03-architecture.md` — where the design is chosen and must build on, not redo, a referenced design
- `spektacular:templates/skill_list_command_test.go:14-31` — the hand-maintained skill/CLI-command table
- `spektacular:templates/vocabulary_containment_test.go:21-47` — the walk-a-directory, count-a-phrase-exactly-once test shape
- `spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py:61-178` — `EXPECTED_STEP_ORDER`, `EXPECTED_SKILLS_PER_STEP`, `EXPECTED_SPAWN_STEPS`, `CONVENTIONS_READ_COMMAND`, `SCAFFOLD_LEFTOVERS`, `EXPECTED_PLAN_SECTIONS`
- `spektacular:tests/harbor/implement-workflow/environment/config.yaml` — a seeded fixture carrying one block per artifact class
- `spektacular:Makefile` — `test` is `go test -shuffle=on ./...`; the four `harbor-test-*` targets are manual only
- `spektacular:README.md:150-235` — the configuration reference prose, including the `knowledge.sources` example and the one-base relative-path rule
- `docs:src/pages/knowledge-base.mdx` — the concept-page template: section order, surface alternation, CLI usage on the concept page
- `docs:src/pages/configuration.mdx:200-214` — the `knowledge` `<ConfigKey>`, the exact form a `design` key copies
- `docs:src/pages/configuration.mdx:18-74,249-293` — the two full-file YAML examples a `design:` block must be added to
- `docs:src/components/Nav.astro:1-21` — the flat nav array a new page is added to
- `docs:src/components/sections/{Section,ConfigurationKeys,ConfigKey,Prose}.astro` — props and slots for the page build

## External references

- `.spektacular/knowledge/architecture/testing-architecture.md` — the three-layer model and the hand-maintained-oracle rule; the reason the harbor surfaces are in scope for any step/template/CLI-name change
- `.spektacular/knowledge/architecture/working-with-files-from-steps.md` — steps reach files through `store.Store`, never `os`; paths are constants derived by typed helpers, never stored in `workflow.Data`
- `.spektacular/knowledge/architecture/workflow-steps.md` — the FSM/StepConfig model (note: predates the store parameter and the `internal/steps/*` layout; treat its package table as historical)
- `.spektacular/knowledge/architecture/cli-design-for-ai-agents.md` — raw-JSON input over bespoke flags, runtime schema introspection, exhaustive input validation; the `--schema` flag on `knowledge` subcommands is this applied
- `.spektacular/knowledge/conventions/error-messages-must-suggest-remediation.md` — every error names the problem *and* a runnable next step via `output.NewError(...).WithNextAction(...)`
- `.spektacular/knowledge/conventions/tests-must-not-depend-on-order.md` — `make test` runs `-shuffle=on`; any test executing `rootCmd` must go through `resetRootCmd`
- `.spektacular/knowledge/conventions/tests-must-pass-for-done.md` — `go test ./...` must be green before the work is called done
- `docs` repo conventions: `mdx-authoring.md` (no layout HTML in page bodies; slots over string props; blank lines around slot content; fenced code blocks), `site-layout.md` (one frame, one flow width, one heading scale, one component per job), `alternate-section-background.md` (alternate `surface` against the preceding section), `no-em-dashes.md` (no em dashes in any authored prose), `file-scoped-section-headings.md` (label before filename), `plan-content-pages.md` (a plan requiring a content page must carry a concrete content skeleton, not a prose summary)

## Prior plans / specs consulted

All read through `go run . plan file read` / `go run . spec file read`. Everything below is
*past intent*, not current behaviour.

- **`000047_repo-scoped-knowledge-addressing`** — the closest precedent. Established
  `Address{Tier,Name}` for a single store and `Selector{Tier,Filter}` for a fan-out, with
  name uniqueness enforced *within* a tier. Decisive finding: a read or write missing either
  field, or naming an unknown tier or name, is **refused outright and never resolved on the
  caller's behalf, even when only one candidate exists** - predictability was treated as a
  hard requirement. Every refusal names the stores available via `next_action`. Rejected: a
  packed string address (`"repo:docs"`, cannot express "all", needs escaping) and keeping a
  `scope` qualifier for non-breaking-ness. **Carry over** the name+path addressing,
  refuse-on-unknown, refuse-on-missing-field and next_action-bearing errors; **do not**
  import the `Tier`/`Selector` machinery, which exists for a two-level hierarchy design
  sources do not have (they are project tier only by spec constraint).
- **`000053_config-schema-versioning-and-migrations`** — a format bump is for a change that
  would make an old file *misread*, not for every new key; adding `schema`/`written_by`
  themselves did not bump the number. A purely additive optional key with a sensible zero
  value needs no bump and no step. When a bump *is* needed the checklist is heavy: a
  registered step, registry contiguity, node-level YAML editing, Action-only side effects,
  and nine required test classes. Rejected: silent in-loader upgrading, and semver-keyed
  versioning. **Carry over**: add `design:` as a plain additive key; do not over-build
  migration machinery.
- **`000041_workflow-knowledge-capture-offers`** — the capture-offer pattern, built as
  *prose only* in two templates that already converse with the user
  (`templates/steps/implement/07-update_changelog.md`,
  `templates/steps/plan/18-walkthrough.md`), with **no new FSM step and no new interruption
  point** (explicitly forbidden). Fixed four-part shape: framing → a worthiness test with a
  deliberately high bar → the offer (name it and why) → outcome rules. Semantics to reuse
  verbatim: "a decline is final for that item for the rest of the conversation; a deferral
  may be re-raised", and "silence or deflection is not acceptance". Tests render the real
  template, lowercase it, and `require.Contains` on anchor phrases plus a **negative** guard.
  **Gotcha recorded there**: `{{config.command}} skill <name>` does not resolve for skills
  nested under `templates/skills/workflows/`, so an offer must not tell the agent to fetch a
  workflow skill that way.
- **`000039_project-level-capabilities`** — defines "project level": `config.yaml` holds
  identity, the repo registry and project-*owned* shared resources; `repo.yaml` holds only a
  repo's own concerns and never points back at a project. Carries an explicit
  "clone the knowledge-sources pattern" checklist for adding a project-level capability:
  a provider-agnostic entry type with a unique slug `name` and a `provider` block; uniqueness
  and required fields validated in `Config.Validate`; defaults synthesised at load, never
  baked into written YAML; a sibling domain package that dispatches on provider via a literal
  switch failing fast on unknown providers; thin `--data`/`--schema` CLI commands with the
  logic in the domain package; agent-facing procedure in prose, never Go flags; and an
  explicit decision on whether the artifact is project-owned, repo-owned or aggregated.
  Two divergences this plan must state openly: (a) 000039 rejected growing the store so
  providers could proxy per-file access, ruling that "providers never proxy per-file access,
  resolution yields paths/metadata only" - design documents genuinely need content read and
  write, so this feature does proxy content, deliberately; (b) 000039's duplicate names
  resolved first-match-in-registry-order, which 000047 later replaced with hard refusal -
  follow 000047.
- **`000038_artifact_metadata` + `000052_document-status-vocabulary`** — frontmatter
  (`created_date`, `document_status`, `closed_date`) was applied to exactly four artifact
  classes and **deliberately not generalised**: the store interface was kept
  metadata-agnostic because "extending Hit with typed metadata fields would leak
  markdown-specific concerns into the substrate", and 000038's own open assumptions call the
  four classes exhaustive. Neither plan requires a new artifact class to inherit the
  lifecycle. Status vocabulary is `draft`/`final`/`superseded`/`archived`, renamed from
  work-flavoured words because agents misread a finished *document* as a finished *feature*;
  reads are lenient, writes strict. **This settles an open question**: design documents must
  carry no Spektacular-authored frontmatter, which is also what 000054's own non-goals
  require. The *referencing spec* keeps its frontmatter and is where the reference lives.
- **`000028_knowledge-base-categories-tiers-and-dedup`** — did **not** design the named-sources
  config shape (that predates it); its own words are "No changes to `config.Config`
  structures: tiers live in the category registry, not in config.yaml". Its
  category/retrieval-tier/dedup machinery is out of scope here by 000054's non-goal on
  knowledge-search indexing. Only transferable idea: one canonical declaration drives
  scaffolding, validation and CLI, never parallel hardcoded lists.
- **`000014_spektacular_store`** — the origin of the `Store` interface: deliberately minimal
  (`Read/Write/Delete/List/Exists`), with `Search` and `Update` **explicitly deferred** until
  a concrete backend needed them, to avoid speculative surface. It has no provider concept at
  all; artifact classes were distinguished by path prefix inside one store. The abstraction
  has visibly evolved past it (`NewSourceStore`, provider dispatch, `Search`), so it is an
  origin story, not a current contract. Its judgement call - defer the abstraction until a
  real backend needs it - is the direct precedent for 000054's own "leave room for a
  read-only remote provider without building it now".
- **`000044_projects-feature-documentation`** — the docs pattern for a new concept: one new
  page added to the existing **Resources** nav dropdown (no new top-level nav item, no new
  content collection), built as Hero → `Section` bands → closing `CtaBanner`, with sparse
  `ConfigKey` blocks that **link out** to the full per-key reference rather than duplicating
  it, plus a one-sentence cross-link from the getting-started tutorial and from `README.md`.
  Trigger rule: conceptual/narrative content gets its own page; per-key reference pages are
  never extended to carry concept narrative.

## Open assumptions

1. **No settings schema bump is needed for the new `design:` key.** Corroborated by 000053's
   own rule (a bump is for a change that would make an old file *misread*; adding
   `schema`/`written_by` themselves did not bump the number). The loader uses
   `yaml.Unmarshal` with no `KnownFields(true)` anywhere in `internal/` (verified by grep),
   so an older binary reading a config carrying `design:` ignores it silently rather than
   failing. The cost of not bumping is a silent degradation on an old binary rather than a
   refusal; the cost of bumping is forcing every existing project through `migrate` for a
   step that only stamps a number. If the explicit refusal is preferred instead, the
   implement workflow must STOP and add `project3to4` plus the `CurrentProjectSchema` bump
   together, because `internal/migrate/registry_test.go:13-30` fails if either moves without
   the other.
2. ~~**Design documents do not carry Spektacular-managed frontmatter.**~~ **Settled, not an
   assumption.** 000038's open assumptions call the four metadata-bearing artifact classes
   exhaustive and record that the store was deliberately kept metadata-agnostic; 000054's
   non-goals forbid imposing structure on a user's design document. So `design write` writes
   bytes through without `metadata.Merge`, and `design list` does not enrich entries with
   `created_date`/`document_status`. This is why the design commands are **not** another
   `newStoreFileCmd` registration (`cmd/storefile.go:172`), whose `write` and `list` both
   stamp and read that block (`cmd/storefile.go:210-236`, `304-336`).
3. **`design write` overwrites without a separate confirmation flag.** The spec's
   "never create or overwrite without the user's explicit agreement" is read as a rule
   binding the *agent* (enforced in AGENTS.md prose and the capture offer), not a CLI
   interlock, exactly as `knowledge write` works today (`cmd/knowledge.go:36-52`,
   `README.md:149`). If a CLI-level guard is wanted instead, the command surface grows.
4. **The docs site's in-flight edits land or are reverted before this work.** The `docs`
   repo has ~130 uncommitted lines in `configuration.mdx` from the 000053 schema-versioning
   work plus a half-finished paragraph in `knowledge-base.mdx`. The plan anchors insertion
   points on component boundaries rather than line numbers for this reason; if those edits
   are abandoned the anchors still hold.
5. **`spec file write` is the only writer of a stored spec.** Recording a design reference
   is assumed to go through `internal/metadata` on the stored file. If any other path writes
   a spec (a workflow callback calling `st.Write` directly), that path must be found and
   routed through `Merge` too, or references will be dropped on that write.
6. **No repo-tier design sources, now or later.** If a future feature adds them, the
   `requireUniqueStoreNames` tier+name identity model (`cmd/knowledge.go:349-365`) is the
   shape to extend, not a flat namespace.
7. **The capture offer attaches to prose in an existing spec step, not a new FSM step.**
   000041 forbade new interruption points and proved a standing AGENTS.md trigger alone is
   not enough (four discoveries, zero offers in the incident that motivated it), so this
   feature does both: a managed AGENTS.md section *and* in-step prose. If the harbor spec
   suite shows the in-step offer firing too often, the `spec_trigger_threshold` knob
   (`templates/agents/spec-trigger.md:11-15`) is the precedent for tuning it, but no knob is
   planned now.

## Drafting assumptions

### Scope research to two repos, not a broader sweep (discovery)
- **Decision**: research only `spektacular` (root `/home/nicj/code/github.com/jumppad-labs/spektacular`) and `docs` (root `/home/nicj/code/github.com/jumppad-labs/spektacular-website`), the two repos `repo list` reports.
- **Rationale**: the registry is the whole project; `spektacular` carries every Go/CLI/template surface and `docs` carries the documentation-site requirement. No third repo exists.
- **Rejected**: none available.

### No design workflow FSM (discovery)
- **Decision**: treat design documents as a stored, addressable artifact with `list`/`read`/`write` commands, not as a fifth interactive workflow with its own step machine.
- **Rationale**: the spec's requirements are storage, addressing, reference and capture-offer behaviours; none of them describes an interactive multi-step interview. A workflow FSM would also take the `cross_kind_workflow_in_progress` lock (`cmd/cross_kind_test.go:46`) and so would be unable to run *during* a spec workflow, which is precisely when the spec says capture must happen.
- **Rejected**: `internal/steps/design/` + `templates/steps/design/*.md` + a harbor design suite + rows in `contractWorkflows`/`stepTemplateTable`/`cross_kind_test`. Rejected as both contrary to the spec and roughly thirteen extra hand-maintained test surfaces.

### Design documents are excluded from `spektacular artifacts list` (discovery)
- **Decision**: do not add a design kind to `cmd/artifacts.go`'s `allArtifactKinds`.
- **Rationale**: that command is documented as querying *workflow-produced* artifacts (`cmd/artifacts.go:1`), and every current kind is written by a workflow. A design document is authored by a person or captured on request. `design list` satisfies the spec's listing requirement.
- **Rejected**: adding `artifactKindDesign` plus an `appendDesignArtifacts` helper and updating the shared list-filter fixtures. Deferred as out of scope rather than ruled out forever.

### Sources-shaped configuration, not store-shaped (discovery)
- **Decision**: model `design` on `KnowledgeConfig{Sources []SourceConfig}` rather than on `SpecConfig`/`PlanConfig`/`ChangelogConfig`.
- **Rationale**: the spec requires several named locations, which a single directory cannot express; and store directories are refused when they resolve outside the project root (`escapesRoot`, `internal/config/config.go:396-399`), which would break the success metric about adopting a folder the team already has. Knowledge-source resolution has no such refusal.
- **Rejected**: a single `design: {provider, config: {directory}}` block reached through the shared `newStoreFileCmd` factory.

### Design documents carry no Spektacular-authored frontmatter (discovery)
- **Decision**: `design write` writes bytes through unchanged; `design list` reports name and path only, with no `created_date`/`document_status` enrichment.
- **Rationale**: the spec's non-goals forbid rewriting or reformatting a user's design document or imposing structure on it, and 000038 deliberately did not generalise the frontmatter lifecycle beyond its four artifact classes, keeping the store metadata-agnostic on purpose.
- **Rejected**: registering the design commands through `newStoreFileCmd`, which stamps and reads that block on every write and list.

### No settings schema bump for the additive `design:` key (discovery)
- **Decision**: add `design` as an optional top-level key at schema 3; register no migration step.
- **Rationale**: 000053's rule is that the format version rises when a change would make an existing file *misread*, and adding `schema`/`written_by` themselves did not bump it. Absence of `design:` is valid and means "no design sources". Bumping would force every existing project through `migrate` for a step that only stamps a number, and `internal/migrate/registry_test.go:13-30` requires the step and the constant to move together.
- **Rejected**: `CurrentProjectSchema = 4` plus a no-op `project3to4`. Recorded as open assumption 1 because the trade-off is real: without a bump, an older binary silently ignores a `design:` block instead of refusing the file.

### Chosen direction: sources-shaped project config + a dedicated `design` command family (architecture)
- **Decision**: a new optional `design.sources` key in `config.yaml` reusing `config.SourceConfig`; a new `internal/design` domain package with a literal provider switch; a `spektacular design` command family (`sources`, `list`, `read`, `write`, plus `ref add`/`ref remove`/`ref list`) addressed by `--data '{"source":…,"path":…}'`; design references stored in the referencing spec's YAML frontmatter via `internal/metadata`; capture and consumption expressed as template prose plus a managed `AGENTS.md` section.
- **Rationale**: it is the only option that satisfies every spec requirement at once. The sources shape is the sole one that can express several named locations and point outside the project root; a dedicated command family is the only way to skip the frontmatter stamping that `newStoreFileCmd` applies and that this spec's non-goals forbid; frontmatter is the only reference store that survives the existing `spec file write` commit path; and prose is where every comparable workflow judgement already lives.
- **Rejected**: a fifth workflow FSM (takes the cross-kind lock, so it could not run during a spec workflow); store-shaped config reached through `newStoreFileCmd` (cannot express several sources, and `escapesRoot` refuses locations outside the project root); repo-tier design sources (forbidden by spec constraint, and `rejectLegacyRepoKnowledgeBlock` is a precedent against a sources list at repo tier); design references as body prose (cannot be validated or refused in Go); free-form string references (cannot name their source). All are recorded with citations in `research.md`.

### `design ref list` succeeds and reports resolution; `design read` fails hard (architecture)
- **Decision**: `design ref list --data '{"spec":…}'` always returns a success envelope listing every reference with a `resolved` flag, the absolute `location` searched, an `unresolved` count and a `next_action` when that count is non-zero. `design read` on a missing document fails with `design_not_found` naming the source, the path and the absolute location searched.
- **Rationale**: the plan workflow needs the complete picture of a spec's references in one call, which an error envelope cannot carry (`ErrorResponse` has fixed fields and no list). Making the *read* the hard failure means planning cannot proceed on a broken reference, which is what the acceptance criterion requires, while still letting an agent see which references are fine.
- **Rejected**: failing `design ref list` outright when any reference is unresolved (loses the resolvable ones and cannot enumerate them); returning content inline from `ref list` (wasteful of context and duplicates `design read`).

### Room for a read-only provider is a `store.Reader` split plus an optional writer (architecture)
- **Decision**: extract `store.Reader` (`Read`, `List`, `Exists`, `Search`) from `store.Store`, with `Store` embedding it and retaining `Write`, `Delete`, `Root`. A design source carries a reader always and a writer only when its provider can write; `design write` to a source with no writer is refused with a named error.
- **Rationale**: the spec asks the plan to leave room without building the provider. Interface segregation is Go's idiomatic answer, costs nothing at existing call sites because `*FileStore` and `ignoreStore` satisfy both halves, and yields one real, unit-testable behaviour today rather than a speculative abstraction. 000014's precedent is to defer backend surface until a backend needs it.
- **Rejected**: a `Capabilities()`/`CanWrite()` method on `Store` (no capability-detection precedent anywhere in the codebase); extending the existing provider-string comparisons (`internal/repo/footprint.go:78-101`, `internal/project/init.go:94-98`, `cmd/knowledge.go:327`), which is the pattern this avoids spreading; doing nothing at all, which leaves the spec's stated risk unanswered.

### The plan names its designs in Dependencies, not a new plan.md section (architecture)
- **Decision**: satisfy "planning records the design it read" through prose in `templates/steps/plan/02-discovery.md` (resolve and read) and `templates/steps/plan/07-dependencies.md` (one bullet per design naming its source and path), with the Architecture step required to build on the design and cite it inline. No new `## Design Documents` heading in `templates/scaffold/plan.md`.
- **Rationale**: the plan scaffold's Dependencies section already covers "upstream specs, or prior plans this work depends on", which is exactly what a referenced design is. A new scaffold heading would add itself to the harbor `EXPECTED_PLAN_SECTIONS` and `SCAFFOLD_LEFTOVERS` oracles for no reader benefit.
- **Rejected**: a new `## Design Documents` scaffold section (extra hand-maintained oracle surface); recording it only in `context.md` (the acceptance criterion asks for the plan to name them, and `plan.md` is what a reviewer reads).

### `init` does not create design source directories (architecture)
- **Decision**: `internal/project.Init` is not extended to scaffold design source locations; `internal/design.NewSet` fails fast when a declared location does not resolve to a directory, mirroring the knowledge `unreachableStore` refusal.
- **Rationale**: a design source points at a folder the team already has, so creating one would be presumptuous and would mask a typo'd location as an empty source. Knowledge scaffolds category directories only because the category registry defines them; design documents have no required structure.
- **Rejected**: creating the directory on first write (hides a misconfiguration); scaffolding at init (same, and contradicts "no files moved or reformatted").

### No new agent skill; the design surface rides existing skills and AGENTS.md (architecture)
- **Decision**: add a managed `templates/agents/design-trigger.md` section and update the `spek-new` and `spek-plan` skill prose, rather than adding a `spek-design` skill to `internal/agent/skills.go`'s `workflowSkills` table.
- **Rationale**: the spec requires a capture offer and a planning obligation, neither of which is a multi-step procedure warranting its own skill. It also sidesteps the gotcha recorded in 000041, that `{{config.command}} skill <name>` does not resolve for skills nested under `templates/skills/workflows/`.
- **Rejected**: a `spek-design` workflow skill (new install surface, new `templates/skill_list_command_test.go` row, and a skill-resolution gotcha for no behavioural gain).

### The reference recorder lives in `cmd`, not `internal/design` (components)
- **Decision**: `design ref add|remove|list` is implemented in the command layer over `internal/metadata` and `internal/design`, rather than giving the design package a dependency on spec storage.
- **Rationale**: `internal/design` owns design sources; a spec lives in the project's spec store, which is a different concern. Keeping the recorder in `cmd` means the design package never needs to know where specs live, and matches how `cmd/knowledge.go` composes `repo` + `config` + `knowledge` rather than pushing that wiring into the domain package.
- **Rejected**: a `design.Refs` API inside `internal/design` (couples the design set to spec storage); a new `internal/designref` package (a package for three thin functions).

### Design commands sit at the top level, not under `spec` (components)
- **Decision**: `spektacular design …` is its own command family; the reference verbs are `design ref …` rather than `spec design …`.
- **Rationale**: design documents are a project-level artifact class in their own right, which is the spec's first requirement. Grouping every design verb under one noun keeps the surface discoverable and means the reference verbs sit next to the document verbs an agent will use in the same breath.
- **Rejected**: `spec design add|list` (splits the design surface across two nouns and implies designs are a property of specs rather than a project artifact).

### A duplicate design reference is a no-op, not an error (data_structures)
- **Decision**: `design ref add` recording a `{source, path}` pair the spec already carries succeeds and changes nothing, rather than failing.
- **Rationale**: the operation is idempotent by nature and an agent re-running a capture after an interruption should not have to distinguish "already recorded" from "failed". The spec's only mandated refusal is an undeclared source.
- **Rejected**: failing with a duplicate-reference error (turns a harmless retry into an error path an agent must special-case).

### `design read` writes bytes to stdout, not a JSON envelope (data_structures)
- **Decision**: `design read` streams the document's raw bytes, matching `spec file read` and `plan file read` rather than wrapping content in JSON.
- **Rationale**: it is the established shape for reading an artifact's content in this CLI, avoids escaping a whole document into a JSON string, and keeps the output usable with `--from` on a subsequent write.
- **Rejected**: a `{"content": "..."}` envelope like `knowledge read` (knowledge entries are small and carry tier/name/path context worth returning; a design document is arbitrary length and its address was supplied by the caller).

### `design list` with no `--source` lists every source (data_structures)
- **Decision**: an unnarrowed `design list` returns documents across all declared sources, each tagged with the source it came from.
- **Rationale**: the acceptance criterion "Several sources are addressable at once" requires exactly this, and it matches `knowledge list`, which fans out by default and narrows with a flag.
- **Rejected**: requiring `--source` (fails the acceptance criterion); a separate `design list-all` verb (two verbs for one behaviour).

### `internal/design` parallels `internal/knowledge` rather than sharing a generic abstraction (implementation_detail)
- **Decision**: build a second, small sources-backed package that mirrors the knowledge package's shape, sharing only the store layer beneath it.
- **Rationale**: this repo's standing preference is to extract a shared helper rather than duplicate, but the duplication here is the four-beat shape (declare, validate, resolve, dispatch), not the substance. Knowledge carries tiers, a category registry, retrieval tiers, tag vocabularies, ranking and de-duplication that design documents are forbidden to have by this spec's non-goals; a shared abstraction would either drag that machinery into the design package or hollow out the knowledge package to fit. The genuinely shared layer is `store`, and both build on it.
- **Rejected**: extracting a generic `sourceset` package used by both (forces a refactor of a working, heavily-tested subsystem to serve a simpler new one, and the two diverge on almost every axis past resolution); giving `internal/knowledge` a second mode for designs (couples two artifact classes that must evolve independently, and would make design documents searchable, an explicit non-goal).

### Three of five success metrics are split into a behavioural half and a manual half (testing_approach)
- **Decision**: rather than classify each metric wholly behavioural or wholly manual, split the three that mix a testable mechanism with a field observation, testing the mechanism and flagging the observation for the implementation test plan.
- **Rationale**: metrics like "for every accepted offer a design exists" have a real, automatable core (acceptance produces both a document and a reference) wrapped in a claim about future human behaviour that no test can assert. Classifying the whole metric as manual would lose the testable core; classifying it as behavioural would overclaim.
- **Rejected**: all-or-nothing classification per metric (either drops real coverage or overstates what the tests prove).

### The end-to-end suites are updated and run as part of this work, not deferred (testing_approach)
- **Decision**: treat every hand-maintained end-to-end expectation this change touches as in scope, and include a run of the affected suites in the plan's verification.
- **Rationale**: the project's own testing-architecture knowledge entry is explicit that these expectations mirror product surfaces, do not run in continuous integration, and accumulate invisible drift paid for by whoever runs them next, citing a prior plan that spent four long runs clearing accumulated drift.
- **Rejected**: deferring the suite updates (nothing would fail at the time, which is precisely the trap the knowledge entry documents).

### Four milestones, split storage / references / workflows / docs (milestones)
- **Decision**: sequence the work as (1) declare and reach design documents, (2) record and resolve references on a spec, (3) the capture offer and the planning obligation, (4) documentation.
- **Rationale**: each is independently deliverable and each builds strictly on the one before. References cannot be validated without a source registry to validate against, and the workflow prose cannot direct an agent at commands that do not exist yet. Documentation last means it documents what actually shipped rather than what was planned.
- **Rejected**: merging storage and references into one milestone (a single large milestone with no usable intermediate state); documenting alongside each milestone (three partial passes over the same two pages, and the docs repo has uncommitted work that makes repeated edits there more likely to collide).

### `design ref add` allows a reference to a document that does not exist yet (phases)
- **Decision**: recording a reference validates the *source* but not the document's existence; a reference to a not-yet-written design succeeds, and `design ref list` is what reports it as unresolved.
- **Rationale**: the spec mandates exactly one refusal at record time, an undeclared source, and a capture conversation naturally records the intent before or alongside the write. Refusing would also make ordering matter between `design write` and `design ref add` for no benefit, and the unresolved report already covers the failure mode the spec cares about.
- **Rejected**: requiring the document to exist at record time (creates an ordering constraint the spec does not ask for, and would block referencing a design that lives in a source not yet populated).

### The design source location is not scaffolded when missing (phases)
- **Decision**: `design write` into a declared source whose location does not exist fails at set construction with the unreachable-source refusal, rather than creating the directory.
- **Rationale**: `NewSet` validates every declared source up front, so the failure is consistent regardless of which command was run, and a typo'd location surfaces as an error rather than as a new empty folder in an unexpected place.
- **Rejected**: creating the location on first write (masks a misconfiguration and is the behaviour the "no files moved" metric argues against).

### Extending the spec suite's verifier for design capture is left to the implementer's judgement (phases)
- **Decision**: Phase 3.3 names the optional end-to-end assertion that an accepted capture leaves both a design document and a reference, and requires an explicit decision either way rather than mandating it.
- **Rationale**: the contract tests already guarantee the instruction carries the offer, and the end-to-end suites cost 20-25 minutes per run and do not gate continuous integration. Whether the extra coverage is worth extending an already-long run is best judged with the implemented behaviour in hand.
- **Rejected**: mandating the assertion (may not be worth the run cost); omitting the question (leaves a silent gap in exactly the layer this project's knowledge base warns drifts invisibly).

## Rehydration cues

- `go run . repo list` — the two roots: `spektacular` at the project dir, `docs` at
  `../../spektacular-website`. Never assume the working directory holds the code.
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs` — the
  four Go conventions and the six docs conventions, in full.
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`
  — the three-layer test model and the hand-maintained oracle list.
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/working-with-files-from-steps.md"}'`
  — the store-not-`os` rule for step callbacks.
- `go run . knowledge search "store provider"` / `"workflow steps"` — the architecture and
  gotchas entries around the store and FSM.
- `go run . knowledge read --data '{"tier":"repo","name":"docs","path":"conventions/mdx-authoring.md"}'`
  and `.../conventions/plan-content-pages.md` — the docs authoring rules and the
  content-skeleton requirement.
- `go run . knowledge sources` — two sources, both repo tier; there is no project-tier
  knowledge store in this project today, which is why `design` is the first project-tier
  sources list the config will carry.
- `go run . knowledge read --data '{"tier":"repo","name":"docs","path":"conventions/no-em-dashes.md"}'`
  — binding on every word written into the `docs` repo.
- `go run . spec file read 000054_project-level-design-documents.md` — the spec.
- Re-read to rebuild the shape: `internal/config/config.go:127-176` (sources),
  `cmd/knowledge.go:286-365` (aggregation + name refusal),
  `cmd/knowledge.go:549-606` (address input), `internal/store/store.go:66-97` (the interface
  to segregate), `internal/metadata/metadata.go:59-81` (the closed frontmatter schema),
  `templates/agents/knowledge-trigger.md` (the offer text).
- `go test -shuffle=on ./...` — green baseline confirmed at the start of this plan
  (21 packages, all `ok`).
