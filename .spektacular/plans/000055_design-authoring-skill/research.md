---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Research: 000055_design-authoring-skill

## Alternatives considered and rejected

**A fifth FSM workflow for designs (`internal/steps/design/`, `templates/steps/design/*.md`,
a harbor suite).** The obvious shape, given spec/plan/implement/repo are all state machines.
Rejected, and rejected twice: 000054's research rejected it first, and this spec restates it as
a hard constraint. The decisive evidence is the cross-kind lock at `spektacular:cmd/resume.go`
(`cross_kind_workflow_in_progress`), keyed on `workflow.State.Kind` and raised whenever an FSM
workflow starts while another is in progress. Design conversation happens *during* a spec
workflow, which is precisely when a design FSM could not run. `spek-knowledge` is the
static-playbook precedent and says so in its own words
(`spektacular:templates/skills/workflows/spek-knowledge/SKILL.md:10`): "it does not drive an
interactive CLI state machine - it is a static playbook".

**Telling the AGENTS.md instruction to fetch the skill with `{{command}} skill spek-design`.**
This is how the flat helper skills are reached. Rejected because it does not work for skills
nested under `templates/skills/workflows/`. Verified by running it in this session:
`go run . skill spek-knowledge` exits 1, `go run . skill spawn-planning-agents` exits 0 - the
CLI resolves only the flat `templates/skills/skill_*.md` files
(`spektacular:templates/skills/`). `templates/agents/knowledge-trigger.md:24` is the working
precedent: "invoke the `spek-knowledge` skill", by name, with no CLI fetch. There is already a
regression guard for this on the design section,
`spektacular:internal/agent/design_trigger_test.go:253`
(`TestRenderedDesignTriggerSectionNamesCommandsDirectly`), asserting the rendered section does
**not** contain `skill spek-`. That guard stays correct under the rewrite and must not be
deleted: naming the skill in prose does not contain the substring `skill spek-`, whereas an
instruction to run `go run . skill spek-design` would.

**A parallel metadata schema for authored designs, separate from `internal/metadata`.**
Rejected. `internal/metadata` already carries `CreatedDate`, `DocumentStatus`, `ClosedDate` and
a `Spec` provenance field (`spektacular:internal/metadata/metadata.go:59-76`), which is exactly
the set the spec asks an authored design to record, and `Merge` already implements the
stamp-once/preserve-thereafter lifecycle
(`spektacular:internal/metadata/merge.go:30-46`). A second schema would duplicate the
`created_date` parsing, the status vocabulary and the closed-date rules, and the spec's
constraint "must use the project's existing document status vocabulary rather than introducing
a second one" points the same way.

**Leaving the back-link as an unmodelled YAML key on the design's frontmatter.** Rejected
outright, and the reason is a live trap rather than a preference:
`spektacular:internal/metadata/metadata.go:86-99` documents `yamlShape` as a *closed* schema -
"a key it does not name is dropped the next time the block is rendered". The comment exists
because `Designs []DesignRef` hit exactly this in 000054. A back-link field must therefore be
modelled on `Metadata`, `yamlShape`, `yamlInShape` and `UpdateOptions` or it will silently
vanish on the next write of the design.

**Deriving back-links on demand by scanning every spec's frontmatter.** No storage, no
two-document write, no consistency problem. Rejected by an explicit Non-Goal in the spec
("Deriving back-links on demand instead of storing them. Considered and rejected in favour of
maintained back-links"). Recorded here so it is not re-proposed.

**Reusing `newStoreFileCmd` (the `spec file` / `plan file` / `changelog file` factory) for
design documents.** Rejected in 000054 and the divergence is signposted in code at
`spektacular:cmd/design.go:13-23`: the factory merges lifecycle frontmatter into every document
it writes and re-reads the block on every listing (`spektacular:cmd/storefile.go:228,312-321`),
and a design document belongs to the team. This feature does **not** overturn that. It splits
the write boundary instead: the existing verbatim path stays exactly as it is for documents the
project already had, and authoring gets its own path that stamps.

**Discriminating authored from pre-existing designs by a registry, a manifest, or a naming
convention.** Rejected in favour of the document's own frontmatter being the discriminator: a
design carrying a metadata block is one Spektacular authored, one without is not. `Split`
already returns `(nil, raw, nil)` for a bare document with no block and treats that as "not an
error" (`spektacular:internal/metadata/frontmatter.go:16-20`), which gives the test for free and
keeps the fact in the one place that cannot drift from the document.

**Writing the back-link first and the spec reference second (so the spec is trivially
untouched on failure).** Rejected. It satisfies the constraint's letter ("leave the spec exactly
as it was") but breaks the acceptance criterion "At no point does an authored design list a spec
that does not reference it" - a failure between the two writes leaves the design claiming a
reference the spec never gained. Either order needs a rollback; the order that matches the
constraint's wording is spec-first, back-link-second, roll the spec back on failure.

## Chosen approach — evidence

**The skill is added to two tables and one template file.**
- `spektacular:internal/agent/skills.go:26-32` - `workflowSkills` is "the single source of truth
  for which skills every agent installs", consumed by all three agents.
- `spektacular:internal/agent/commands.go:18-24` - `workflowDescriptions`, keyed by the same
  skill names, renders the slash-command wrapper for bob/codex.
- `spektacular:internal/agent/claude.go:22`, `bob.go`, `codex.go` - each calls
  `installWorkflowSkills` with its own target directory (`.claude/skills`, `.bob/skills`,
  `.agents/skills`).
- `spektacular:cmd/migrate.go:76` - `migrate` re-runs `a.Install(root, cfg, io.Discard)`, so an
  existing project picks up a newly added skill and any changed managed section without a
  separate migration step. No `internal/migrate` registry entry is needed: that registry
  (`spektacular:internal/migrate/registry.go:60-68`) is for config *schema* changes only.

**The AGENTS.md section is a managed section, rewritten in place.**
- `spektacular:templates/agents/design-trigger.md` - the current 51-line section.
- `spektacular:internal/agent/design_trigger.go:10-19` - heading constant
  `## Design-Worthy Detail Recognition` plus `installManagedSection`; idempotent, rewrites in
  place.
- `spektacular:internal/agent/spec_trigger_test.go:157-169` - pins the heading *order* across
  the whole installed AGENTS.md (Memory & Context < Knowledge-Worthy < Spec-Worthy <
  Design-Worthy < Presenting Drafts). Changing the heading text breaks this test.

**The interview shape is already written down and cited.**
- `spektacular:templates/steps/spec/00b-interview.md:3-9` - the Flipped Interaction
  implementation: the citation (White et al., arXiv:2302.11382), a **stated goal**, "adaptive,
  open questions - not a fixed script", and the explicit stopping condition ("Stop once more
  questions wouldn't change the draft ... a small number of exchanges, not an exhaustive
  back-and-forth"). A design interview is the same three parts aimed at a different goal.
- `spektacular:templates/skills/workflows/spek-knowledge/SKILL.md:12-23,48-83` - the
  static-playbook structure to model: a "When to invoke" block of natural-language triggers,
  then one `# Intent:` heading per branch, each ending in a direct CLI call with an explicit
  propose-then-confirm gate.

**The metadata machinery, field by field.**
- `spektacular:internal/metadata/metadata.go:22-31` - the status vocabulary: `draft`, `final`,
  `superseded`, `archived`. `DocumentStatuses()` at :36 and `ParseDocumentStatus` at :44.
- `spektacular:internal/metadata/metadata.go:59-76` - `Metadata`: `CreatedDate`,
  `DocumentStatus`, `ClosedDate`, and provenance `Project` / `ProjectSource` / `Spec` / `Plan`.
  **`Spec` already exists**, so an authored design's provenance ("which spec the conversation
  belonged to") needs no new field.
- `spektacular:internal/metadata/metadata.go:77-83` - `Designs []DesignRef`, the forward
  reference, with the comment explaining why it had to be modelled.
- `spektacular:internal/metadata/metadata.go:88-99` - `yamlShape` (render side) and
  `yamlInShape` (decode side). Decode holds `Designs` as a raw `yaml.Node` deliberately, "so a
  malformed or non-list value must read as no references rather than failing the whole parse";
  a back-link field copies that leniency.
- `spektacular:internal/metadata/merge.go:11-28` - `UpdateOptions`. `Designs *[]DesignRef` is a
  pointer because "the three states are genuinely distinct": nil = no change, non-nil = replace,
  non-nil empty = clear. A back-link field wants the identical shape.
- `spektacular:internal/metadata/merge.go:47-131` - `Merge`. First write stamps `CreatedDate` and
  defaults status to `draft`; later writes preserve `CreatedDate` and carry `Designs` forward
  unless replaced (:95-103).
- `spektacular:internal/metadata/frontmatter.go:16-20` - `Split` returns `(nil, raw, nil)` for a
  document with no leading `---`, so "no block" is a clean, non-error signal.
- `spektacular:internal/metadata/frontmatter.go:56-58` - `Render`'s own warning: idempotency is
  guaranteed only for modelled fields, "unknown YAML keys in the original block are not
  preserved".
- `spektacular:internal/metadata/metadata.go:177-189` - `decodeDesignRefs`: a non-sequence reads
  as nil, and an entry missing either half of the address is dropped rather than failing. The
  decode helper a back-link field copies.
- `spektacular:internal/metadata/metadata.go:198-207` - `validateDocumentStatus` (blank is
  rejected as a caller-set value: "a state an artifact can read as, not one a caller can set")
  and `isClosed` (`final || superseded || archived`).
- `spektacular:internal/metadata/close.go:16-27` - `Close(store, path, status)`: read, split,
  `Merge` with only a status change, write. Body preserved byte-for-byte. This is how a design's
  lifecycle status would be moved to `superseded` without rewriting its content.

**Adding a metadata key is a four-place lockstep change.** The back-link field must be added to
all of: `Metadata` (`spektacular:internal/metadata/metadata.go:59`), `yamlShape` (`:90`),
`yamlInShape` (`:103`), and the `MarshalYAML`/`UnmarshalYAML` pair (`:119`, `:140`) - plus
`UpdateOptions` and the preserve-vs-replace branch in `Merge`
(`spektacular:internal/metadata/merge.go:88-115`) if it is to survive a body-only rewrite.
`Designs` is the worked example of doing this correctly, and
`spektacular:internal/metadata/metadata_test.go:770`
(`TestRender_OmitsDesignsKeyEntirelyWithNoReferences`) is the **byte-exact** regression guard
proving the new key perturbs nothing when empty. The new field copies that test.

**The metadata write path, precisely.** `spektacular:cmd/storefile.go:189-234` is the `write`
handler: read `--from` (:207), read any existing stored artifact (:212), build opts from
`--document-status` (:216), overlay provenance if `--repo` (:220-226),
`stripLeadingFrontmatterBlocks` on the incoming content (:227, defined :31-40 - strips *zero or
more* leading blocks so re-writing a copied artifact is idempotent), then
`metadata.Merge` (:228) and `st.Write` (:232). There is no opt-in flag: that path always stamps.
Conversely `spektacular:internal/design/design.go` `Set.Write` calls
`src.writer.Write(d.Path, content)` directly and stamps nothing. Those two are the write boundary
the two classes of design document diverge at.

**Cross-kind artifact surfaces that designs are currently absent from.**
`spektacular:cmd/artifacts.go:20-37` enumerates the artifact kinds (`spec`, `plan.plan`,
`plan.context`, `plan.research`, `plan.test-plan`, `changelog`) and `scanArtifact` (:149-181)
reads each one's block and applies the shared filter. `spektacular:cmd/artifactfilter.go:14-20`
(`documentStatusValues`) renders the vocabulary into flag help and error text, and
`parseDocumentStatusFlag` (:27-35) is the strict input validator emitting
`invalid_document_status`. Designs are not an artifact kind and this feature's Non-Goals do not
ask them to become one, so these surfaces are **out of scope** - but they are where a reader will
look, and the plan should say so rather than leave it ambiguous.

**Do not route designs through `internal/store/frontmatter.go`.**
`spektacular:internal/store/frontmatter.go:11-30` is a second, deliberately separate frontmatter
parser for knowledge entries, with the *opposite* schema contract: every field optional, no block
is the normal case, and unknown keys are preserved. Its comment states that routing an entry
through `internal/metadata` would reject a tags-only block as malformed and delete the tags on
write-back. An authored design wants the `internal/metadata` contract (closed schema, stamped
lifecycle), not this one.

**The reference verbs, which own back-link maintenance.**
- `spektacular:cmd/design_ref.go:17-22` - why the verbs live in `cmd/` and not
  `internal/design`: a spec lives in the spec store, which `internal/design` must not know about.
- `spektacular:cmd/design_ref.go:153-187` - `refsOf` (read current refs + the raw bytes) and
  `writeRefs` (rewrite only the frontmatter via `metadata.Merge`, body untouched). `refsOf`
  already returns the original `raw` bytes, which is exactly the rollback payload a failed
  back-link write needs.
- `spektacular:cmd/design_ref.go:206-250` - `runDesignRefAdd`: validates the source *before*
  touching the spec, then treats a duplicate as a silent idempotent no-op.
- `spektacular:cmd/design_ref.go:254-287` - `runDesignRefRemove`: removing an absent reference
  succeeds and writes nothing.
- `spektacular:cmd/design_ref.go:198-205` - the recorded decision that the design document need
  **not** exist when a reference is recorded. This matters for back-links: `ref add` against a
  document that is not there yet must still succeed, and there is nothing to write a back-link
  into.

**The design package and its write boundary.**
- `spektacular:internal/design/design.go:1-21` - the package doc: "It never adds frontmatter,
  reformats content, or imposes a structure on a design document, which is why this package
  exists instead of the shared artifact-file machinery".
- `spektacular:internal/design/design.go` - `Set.Read`, `Set.Write` (bytes unchanged),
  `Set.Resolve` (absolute path without reading), `Set.Exists`, `Set.List`.
- `spektacular:internal/design/errors.go` - every refusal is
  `output.NewError(code, msg).WithResource(...).WithNextAction(...)`; codes in use:
  `design_source_unknown`, `design_address_incomplete`, `design_source_unreachable`,
  `design_provider_unsupported`, `design_source_read_only`, `design_not_found`.
- `spektacular:cmd/design.go:234-277` - `runDesignWrite`: `--from <file>` only, never prose on
  the command line; returns `{source, path, location}`.
- `spektacular:cmd/design.go:183-207` - `runDesignList` returns only `{source, path}` today. The
  spec's "authored designs report status, provenance and referencing specs, when read" is the
  hook for extending this the way `spec file list` already reports metadata
  (`spektacular:cmd/storefile.go:312-321`).

**Config and sources.**
- `spektacular:internal/config/config.go:133-148,283` - `DesignConfig`, `Design` field.
- `spektacular:.spektacular/knowledge/gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md`
  - why `design.sources[].config.location` is deliberately **not** in `Config.storeDirs()`: that
  list rewrites paths on read/write and refuses anything resolving outside the project root.
  Nothing in this feature should add it.
- This project's own `.spektacular/config.yaml` declares one source, `design`, at
  `location: design`, resolving to `.spektacular/design`, currently holding only a
  `.spektacular_ignore`d `README.md`.

**Test conventions to follow.**
- `spektacular:cmd/design_test.go:15-24` - every command is driven through
  `resetRootCmd` + `runRootCmd`, never by calling a `RunE` directly, because the cobra commands
  are package-level globals whose flags leak under `-shuffle=on`.
- `spektacular:cmd/design_test.go:69-122` - fixture helpers: `writeDesignConfig`,
  `seedDesignDoc` (writes with `os.WriteFile`, independent of the code under test),
  `stageDesignDoc`, `twoSourceDesignProject` (one relative source, one absolute source outside
  the project root).
- `spektacular:internal/agent/design_trigger_test.go:198-268` - the template-contract shape:
  render the real template, then `require.Equal(t, 1, strings.Count(...))` on hand-maintained
  anchor literals, plus a negative guard.
- `spektacular:templates/skill_list_command_test.go:36-60` -
  `TestWorkflowSkillsDocumentDesignCommands`, the hand-maintained table of which skill must name
  which CLI commands. A new skill joins this table.
- `spektacular:internal/agent/instruction_surface_test.go:35-75` - walks every embedded skill and
  step template, and separately renders them through the real install path, asserting a closed
  list of forbidden substrings. A new skill template is picked up automatically by both walks.

**Docs repo (`docs`, root `/home/nicj/code/github.com/jumppad-labs/spektacular-website`).**
- `docs:src/pages/design-documents.mdx` - the whole 319-line concept page, currently
  **untracked** (the 000054 docs work is uncommitted; `src/components/Nav.astro` and
  `src/pages/configuration.mdx` are modified-unstaged).
- `docs:src/pages/design-documents.mdx:25-30` - the paragraph that has to be **reworked, not
  appended to**: it asserts Spektacular "never adds frontmatter to a design document" and that a
  CLI-written document "comes back byte for byte identical". Authored designs contradict the
  first half as written.
- `docs:src/pages/design-documents.mdx:161,241` - the surface-alternation pivot: 161 is plain,
  241 is shaded, so inserting a section between them flips every subsequent `surface` value.
- `docs:src/pages/design-documents.mdx:170-212` - the fenced-`bash`-block-per-verb command
  reference a new verb joins.
- `docs:src/pages/how-it-works.mdx:372-393` - the "Artifact metadata" section: the existing
  bullets for created date / document status (`draft`, `final`, `superseded`, `archived`) /
  closed date. The model for the authored-design metadata bullets.
- `docs:src/pages/how-it-works.mdx:233-286` - the Flipped Interaction interview explained for
  specs, with a worked `Agent:` / `You:` transcript at :242-252 and the stopping condition at
  :254-255. Closest prior art for describing a design interview.
- `docs:src/pages/projects.mdx:159-256` - `spek-manage-repos` documented as a guided
  conversation, with a long fenced agent-transcript block at :176-212. The best existing model
  on the site for documenting a guided-interview authoring skill.
- `docs:src/pages/knowledge-base.mdx:256-291` - documents that knowledge entries *optionally*
  carry a frontmatter block. The nearest existing template for "some documents carry a block and
  some do not", which is the authored-vs-pre-existing distinction.
- `docs:src/pages/knowledge-base.mdx:116-118` - how `spek-knowledge` is named on the site: inside
  the subsystem page, not on a page of its own. There is no page-per-skill and no combined skills
  page.
- `docs:src/components/Nav.astro:2-22` - the single flat nav array; no sidebar exists.
- `docs:package.json:7-12` + `docs:Makefile:15-16` - `npm run build`; `astro check` is **not** an
  npm script, it is `make check` / `npx astro check`. CI (`deploy.yml`) runs `npm run build`
  only, so type-checking is a local gate.

## Files examined

- `spektacular:internal/design/design.go` - `Set` API; `Write` stores bytes unchanged; package
  doc states why it is not the shared artifact machinery
- `spektacular:internal/design/errors.go` - the six refusal codes, each with resource +
  next_action
- `spektacular:internal/design/paths.go` - `providerFile`, `isAbs`, `joinPath`, `isNotFound`
  helpers
- `spektacular:cmd/design.go:13-23` - the signposted divergence from `newStoreFileCmd`
- `spektacular:cmd/design.go:234-277` - `design write` is `--from <file>` only
- `spektacular:cmd/design_ref.go:153-187` - `refsOf` returns raw bytes; `writeRefs` rewrites only
  frontmatter
- `spektacular:cmd/design_ref.go:198-250` - `ref add` validates source first, duplicate is a
  no-op, document need not exist
- `spektacular:cmd/design_ref.go:298-357` - `ref list` always returns success plus an
  `unresolved` count and `next_action`
- `spektacular:cmd/storefile.go:228,312-321` - the write path that stamps metadata, and the
  listing that reports `created_date` / `document_status` / `closed_date`
- `spektacular:internal/metadata/metadata.go:22-99` - status vocabulary, `Metadata`, `DesignRef`,
  the closed `yamlShape` / lenient `yamlInShape` pair
- `spektacular:internal/metadata/merge.go:11-131` - `UpdateOptions` pointer semantics and the
  full `Merge` lifecycle
- `spektacular:internal/metadata/frontmatter.go:16-70` - `Split` (no block is not an error) and
  `Render` (unknown keys are dropped)
- `spektacular:internal/agent/skills.go:26-68` - `workflowSkills`, mustache render with
  `{{command}}` only, partials from `sourceFS`
- `spektacular:internal/agent/commands.go:18-60` - `workflowDescriptions` and the wrapper render
  for non-skill agents
- `spektacular:internal/agent/claude.go:18-47` - the ordered `install*Section` calls
- `spektacular:internal/agent/design_trigger.go:10-19` - heading constant and managed-section
  install
- `spektacular:internal/agent/design_trigger_test.go:198-268` - anchor-literal assertions plus the
  `NotContains "skill spek-"` guard
- `spektacular:internal/agent/spec_trigger_test.go:157-169` - AGENTS.md heading-order pin
- `spektacular:internal/agent/instruction_surface_test.go:35-75` - embedded + rendered template
  walks
- `spektacular:internal/metadata/metadata_test.go:298-495` - `TestMerge`, 17 table cases covering
  every stated invariant; the pattern any new field's merge tests follow
- `spektacular:internal/metadata/metadata_test.go:747-836` - the four design-ref tests:
  round-trip, byte-exact omission when empty, malformed reads as none, half-address entry dropped
- `spektacular:internal/metadata/merge_test.go:29-118` - the five `*[]DesignRef` tri-state tests
  (preserve / replace / clear / fresh / survives status transition)
- `spektacular:internal/metadata/close_test.go:48-140` - `Close` on a bare artifact attaches a
  fresh block; idempotence; read errors propagate
- `spektacular:cmd/storefile_metadata_test.go:812` -
  `TestStoreFileWrite_SpecDesignReferencesSurviveOrdinaryWrite`, the durability guarantee a
  back-link field needs its own twin of
- `spektacular:cmd/storefile.go:31-40` - `stripLeadingFrontmatterBlocks`, why a repeated write
  never stacks duplicate blocks
- `spektacular:cmd/artifacts.go:20-37,149-181` - the cross-kind artifact kinds and `scanArtifact`;
  designs are deliberately not among them
- `spektacular:cmd/artifactfilter.go:14-35` - `documentStatusValues` and
  `parseDocumentStatusFlag`; the strict CLI-side status validator
- `spektacular:internal/store/frontmatter.go:11-30` - the separate knowledge-entry parser and the
  comment explaining why it must not be merged with `internal/metadata`
- `spektacular:internal/steps/spec/steps.go:82,178` - the scaffold write that stamps, and the
  `finished()` close-out to `final`
- `spektacular:internal/steps/plan/steps.go:213-217,324` - `planDocs` and the loop closing all
  three plan documents
- `spektacular:cmd/migrate.go:76` - `migrate` re-runs the agent install, so skills refresh
- `spektacular:internal/migrate/registry.go:60-68` - the schema-step registry; not needed for a
  skill addition
- `spektacular:templates/agents/design-trigger.md` - the section being rewritten; its four gaps
  are listed in `.spektacular/working-context.md`
- `spektacular:templates/agents/knowledge-trigger.md:24` - "invoke the `spek-knowledge` skill",
  the by-name precedent
- `spektacular:templates/skills/workflows/spek-knowledge/SKILL.md:10,12-23,48-83` - the
  static-playbook structure
- `spektacular:templates/skills/workflows/spek-new/SKILL.md:42-56` - the capture side of design
  commands as a skill documents them
- `spektacular:templates/skills/workflows/spek-plan/SKILL.md:38-56` - the consumption side
- `spektacular:templates/steps/spec/00b-interview.md:3-9` - Flipped Interaction: goal, adaptive
  questions, stopping condition
- `spektacular:templates/steps/spec/05-technical_approach.md:15-49` - the existing in-workflow
  capture offer and its three-part bar
- `spektacular:templates/partials/version-check.md` - the partial every skill opens with
- `spektacular:templates/scaffold/plan.md` - the eleven plan sections this plan must fill
- `spektacular:templates/skill_list_command_test.go:36-60` - the skill/command table
- `spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py:117-130` -
  `DESIGN_REF_LIST_COMMAND`, the design obligation oracle
- `spektacular:.spektacular/config.yaml` - one declared design source, `design` ->
  `.spektacular/design`; `agent: bob`
- `spektacular:.spektacular/design/README.md` - the scaffold README, `.spektacular_ignore`d
- `spektacular:README.md:17,156,199-204` - the design bullet, the config split sentence, the
  `design:` YAML example
- `docs:src/pages/design-documents.mdx` - all 319 lines; section inventory and surface
  alternation above
- `docs:src/pages/configuration.mdx:69-74,87-90,223-239` - the `design:` example, the key count
  sentence, the `<ConfigKey name="design">` entry
- `docs:src/pages/how-it-works.mdx:233-286,372-393` - the interview prose and the artifact
  metadata section
- `docs:src/pages/projects.mdx:159-256,319-328` - the guided-conversation model and the only
  rendered provenance frontmatter example
- `docs:src/pages/knowledge-base.mdx:94-155,256-291` - the lifecycle-of-an-entry structure and
  optional frontmatter prose
- `docs:src/components/Nav.astro:2-22` - the nav array
- `docs:package.json:7-12`, `docs:Makefile:15-16` - build and check commands

## External references

- White et al., "A Prompt Pattern Catalog to Enhance Prompt Engineering with ChatGPT",
  arXiv:2302.11382 - the Flipped Interaction pattern. Already cited verbatim in
  `templates/steps/spec/00b-interview.md:3`; the design interview must cite it the same way so
  the two interviews are recognisably one pattern rather than two inventions.
- `spektacular:.spektacular/knowledge/architecture/testing-architecture.md` - the three-layer
  model (Go unit/step tests, template-contract tests, harbor E2E) and the hand-maintained-oracle
  rule. Why any change to skills, templates or CLI command names pulls the harbor oracles into
  scope even though they do not run in CI.
- `spektacular:.spektacular/knowledge/architecture/working-with-files-from-steps.md` - steps
  reach files through `store.Store`, never `os`; paths are constants derived by typed helpers.
- `spektacular:.spektacular/knowledge/architecture/cli-design-for-ai-agents.md` - predictability
  and defence against hallucination over discoverability; raw JSON `--data`, `--schema`
  introspection.
- `spektacular:.spektacular/knowledge/conventions/error-messages-must-suggest-remediation.md` -
  every error via `output.NewError(code, msg).WithNextAction(<runnable command>)`.
- `spektacular:.spektacular/knowledge/gotchas/remediation-needs-the-layer-that-holds-the-facts.md`
  - a check whose `next_action` must name configured values belongs beside whatever owns them,
  not at the CLI boundary. Directly relevant to where the back-link failure is detected and
  worded.
- `spektacular:.spektacular/knowledge/gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md`
  - why design locations are not in `Config.storeDirs()`.
- `spektacular:.spektacular/knowledge/conventions/tests-must-not-depend-on-order.md` -
  `make test` is `go test -shuffle=on ./...`; anything executing `rootCmd` goes through
  `resetRootCmd`.
- `spektacular:.spektacular/knowledge/conventions/tests-must-pass-for-done.md` - `go test ./...`
  green before done, including pre-existing failures.
- `docs` repo conventions, all binding on the docs phase: `mdx-authoring.md` (no layout HTML in
  page bodies; slots over string props; blank line around slot content; fenced code blocks),
  `site-layout.md` (one frame, one flow width, one heading scale, one component per band),
  `alternate-section-background.md` (alternate `surface` explicitly against the preceding
  section), `no-em-dashes.md` (no em dashes in any authored prose, including commit messages),
  `file-scoped-section-headings.md`, `plan-content-pages.md` (a phase touching a content page
  carries a concrete `**Content outline**` skeleton, not a prose summary).

## Prior plans / specs consulted

- **`000054_project-level-design-documents`** (plan + research) - the feature this one extends.
  Its research records the two rejections this spec revisits: the design FSM (rejection stands,
  reinforced by the cross-kind lock) and a `spek-design` skill (rejected on the premise that
  "neither a capture offer nor a planning obligation is a multi-step procedure warranting its own
  skill" - an authoring interview is, so this is new scope rather than a reversal). Its plan also
  supplies the exact surface a design change touches: `git show --stat caaa761` lists 48 files
  including `internal/design/*`, `cmd/design*.go`, `internal/metadata/*`, `internal/agent/*`,
  `templates/agents/design-trigger.md`, four step templates, two skill templates, and the harbor
  plan-workflow oracles.
- **`000041_workflow-knowledge-capture-offers`** - the source of the three-outcome
  accept/defer/decline vocabulary and the "silence or deflection is not acceptance" line, and the
  place the nested-skill-fetch gotcha was first recorded.
- **`000022_spek-knowledge-skill`** - the static-playbook skill precedent: intent recognition,
  one branch per intent, direct CLI calls, propose-then-confirm enforced by prose rather than a
  CLI guard.
- **`000043_flipped-interaction-spec-interview`** - the interview this one mirrors, and the
  precedent for pairing a deep docs page section with a homepage feature card.
- **`000052_document-status-vocabulary`** - fixed the four-value vocabulary
  (`draft`/`final`/`superseded`/`archived`) an authored design must reuse.
- **`000039_project-level-capabilities`** - the "clone the knowledge-sources pattern" checklist
  for adding a project-level capability, and the rule that `config.yaml` holds project-owned
  shared resources while `repo.yaml` holds only a repo's own concerns.

## Open assumptions

1. **A design carrying a metadata block is exactly the set of designs Spektacular authored.**
   Nothing stamps a block today, so the discriminator is sound at the moment this ships. If a
   team hand-writes YAML frontmatter at the top of their own design file, Spektacular will read
   it as authored and maintain back-links in it. Judged acceptable (it is also arguably correct),
   but it is an assumption, not a proof. If the implement workflow finds a case where this is
   wrong, STOP and ask.
2. **`internal/metadata`'s lenient decode is enough to protect a pre-existing design.** A design
   with no leading `---` returns `(nil, raw, nil)` from `Split`, so "no block" is unambiguous.
   Assumed no pre-existing design in any project's declared source begins with a `---` line for
   some other reason (a horizontal rule as the literal first line of the file would be read as an
   unterminated or malformed block). If found, STOP and ask.
3. **`migrate` is sufficient to deliver a new skill to existing projects.** Verified in code
   (`cmd/migrate.go:76` re-runs `Install`), not verified by running an end-to-end upgrade against
   an older project. The implement workflow should run it once.
4. **The docs repo's uncommitted 000054 work will still be present, in the same shape, when the
   docs phase runs.** `src/pages/design-documents.mdx` is untracked and `Nav.astro` /
   `configuration.mdx` are modified-unstaged. If that tree has been committed, rebased or
   discarded by then, re-read the page before editing: every line number cited above moves.
5. **No harbor suite asserts anything about the design-trigger section's prose.** Greps found
   only `DESIGN_REF_LIST_COMMAND` in the plan-workflow suite, which is about the plan step's
   obligation and is untouched here. Assumed the spec-workflow suite has no design-prose oracle;
   its `task.toml` was changed by 000054 and was not read line by line.

## Drafting assumptions

### Both registered repos are in scope (discovery)
- **Decision**: plan changes against `spektacular` (CLI, skill and agent templates) and `docs`
  (the Astro site). Research was run in both `root`s reported by `repo list`.
- **Rationale**: the requirement "Public docs explain design authoring" and its acceptance
  criterion "the site builds with no errors and no warnings" can only land in the `docs` repo,
  whose registered role is `documentation`. Everything else is CLI and template work in
  `spektacular`.
- **Rejected**: planning only against `spektacular` and leaving docs as a follow-up. The spec
  makes the docs a requirement with its own acceptance criterion, so descoping it would be
  shipping a partial spec.

### A metadata block is the authored/pre-existing discriminator (discovery)
- **Decision**: treat "carries an `internal/metadata` frontmatter block" as the definition of a
  design Spektacular authored, rather than keeping a registry, a manifest, or a naming rule.
- **Rationale**: `metadata.Split` already returns `(nil, raw, nil)` for a document with no
  leading `---` and treats that as a non-error
  (`internal/metadata/frontmatter.go:16-20`), so the test is free and the fact lives in the one
  place that cannot drift from the document itself. The spec's own wording ("A design document
  the project already had must never gain a metadata block") points at the block as the
  distinguishing property.
- **Rejected**: a side-car index in the design source (pollutes a folder the team owns, and the
  spec forbids adding anything to it); a path or filename convention (a team's existing folder
  has its own naming, which Spektacular must not constrain).
- **Residual risk recorded as an open assumption**: a team that hand-writes YAML frontmatter on
  their own design file will be read as authored. Flagged in research.md rather than guarded
  against.

### The cross-kind `artifacts list` surface stays out of scope (discovery)
- **Decision**: do not add a `design` kind to `cmd/artifacts.go` or the shared
  `cmd/artifactfilter.go` filter, even though authored designs will now carry the same
  lifecycle block those surfaces exist to query.
- **Rationale**: no requirement or acceptance criterion asks for it, and the spec's Non-Goals are
  explicit that designs get no indexing, ranking, tags or categories. Adding a sixth artifact
  kind would also pull the design sources (which may sit anywhere on disk, outside the project
  root) into a scan built for in-project stores.
- **Rejected**: extending `artifacts list` opportunistically because the metadata is now there.
  That is scope the user did not ask for, and it is separable if wanted later.
- **Consequence to state in the plan**: `design list` is where an authored design's status and
  provenance get reported, not `artifacts list`. A reader who looks in the obvious place will
  not find it, so the plan says so rather than leaving it implicit.

### Spec-first, back-link-second, roll the spec back on failure (discovery)
- **Decision**: `design ref add` writes the spec's reference first, then the design's back-link,
  and restores the spec's original bytes if the back-link write fails.
- **Rationale**: this is what the spec's constraint literally requires ("must fail as a whole and
  leave the spec exactly as it was"), and `refsOf` already hands back the original raw bytes
  (`cmd/design_ref.go:154-167`), so the rollback payload costs nothing to obtain.
- **Rejected**: back-link first, spec second. It makes the spec trivially untouched on failure
  but violates the acceptance criterion "At no point does an authored design list a spec that
  does not reference it" - a failure between the writes leaves the design claiming a reference
  the spec never gained. Either order needs a rollback; only this one matches the constraint's
  wording.

### Chosen direction: split the two classes of design at the write verb (architecture)
- **Decision**: add a new `design author` CLI verb that routes through `metadata.Merge` and
  stamps the project's existing lifecycle block; leave `design write` verbatim and unchanged
  except for one guard refusing to overwrite a document that already carries a block. The
  presence of that block is then the discriminator every later decision reads.
- **Rationale**: it is the smallest seam that gives the two classes genuinely different
  behaviour, and it falls exactly where the codebase has already split them: `cmd/storefile.go`
  always stamps via `metadata.Merge`, `internal/design.Set.Write` writes raw bytes and stamps
  nothing. It reuses the existing status vocabulary and the existing `Spec` provenance field
  rather than inventing a second schema, which a constraint requires. The discriminator needs no
  registry or naming rule, and cannot drift from the document it describes.
- **Rejected**:
  - *Split in the skill only, with no new CLI verb*: the skill composes the frontmatter as text
    and uses the existing verbatim `design write`. Rejected because the block's correctness would
    rest entirely on prose: no status validation, no created-date preservation across a revision,
    and the closed-schema round-trip unenforced. The back-link write in `ref add` would still need
    Go-side merging, so it does not even avoid the Go change.
  - *Split in the store, by declaring a source as authored or external in `config.yaml`*:
    rejected because it forces the user to partition their design folders up front, and it gets
    the wrong answer for the common case of a team folder that will hold both a design they wrote
    and one Spektacular later authored beside it. It also contradicts the spec's framing, which
    distinguishes documents by who authored them, not by where they live.
  - *`design write --authored` as a flag rather than a sibling verb*: workable, but a flag that
    silently changes whether a document is stamped is exactly the kind of thing an agent adds or
    omits by accident. The repo's own CLI guidance prefers predictability over discoverability,
    and four distinct skill branches map more cleanly onto distinct verbs.

### `design write` refuses to overwrite an authored document (architecture)
- **Decision**: `design write` gains one new refusal: if the target document already carries a
  metadata block, it fails with a next action pointing at `design author`.
- **Rationale**: a verbatim overwrite of an authored design would silently strip its block and
  destroy its back-links, leaving specs pointing at a design that no longer lists them. That is
  the exact inconsistency the spec's back-link requirement exists to prevent, and it would happen
  without any error. Nothing in the spec asks for `design write` to be able to clobber an authored
  document.
- **Rejected**: letting the overwrite proceed and re-deriving back-links afterwards (deriving
  back-links is an explicit non-goal); or silently upgrading the write to an authored write
  (makes `design write` unpredictable, which is the thing the verbatim guarantee is for).

### Back-links are a `specs` list on the design's own block (architecture)
- **Decision**: add `Specs []string` to `Metadata`, serialised as `specs`, holding the names of
  the specs that reference the design. Provenance reuses the existing `Spec` field.
- **Rationale**: it mirrors `designs` on the spec exactly, so the two directions of the same
  relationship look alike on disk. Spec names are what `design ref add` already receives and what
  `spec file list` already returns, so no new addressing is invented. Reusing `Spec` for
  provenance avoids adding a field that already exists.
- **Rejected**: a list of richer objects (nothing needs more than the name, and `DesignRef` needed
  two halves only because a path alone is ambiguous across sources); a single `spec` field doing
  double duty as provenance and back-link (they are different facts and a design can be
  referenced by several specs while originating in at most one).

### Conventions selected, and one deliberately dropped (architecture)
- **Decision**: carry three `spektacular` conventions (remediation in errors, order-independent
  tests, passing tests before done) and five `docs` conventions (MDX authoring, no em dashes,
  alternating section backgrounds, site layout, plan-sketches-content-structure). Drop
  `file-scoped-section-headings`.
- **Rationale**: each carried convention bears on a specific choice this feature makes, recorded
  inline in the conventions working file. `file-scoped-section-headings` governs reference pages
  documenting more than one underlying file; the design documents page is a concept page with no
  file-scoped sections, so it has nothing to act on.
- **Rejected**: listing every loaded convention. An undifferentiated list is a visible signal the
  knowledge base was not actually consulted, and the step says so.

### The spec skill and the technical-approach step template are in scope (components)
- **Decision**: update `templates/skills/workflows/spek-new/SKILL.md` and
  `templates/steps/spec/05-technical_approach.md` alongside the `AGENTS.md` section, rather than
  changing only the standing instruction.
- **Rationale**: all three carry a near-duplicate of the same capture offer, and two of them make
  claims this feature falsifies. `spek-new/SKILL.md:54` states `design write` stores a document
  "with no frontmatter added", which is still true of that verb but reads as a statement about
  design documents in general once authored ones exist; and
  `05-technical_approach.md:17-49` presents capture as the only path, with no authoring branch and
  an accept step that hard-codes the current spec name. Leaving them stale would mean the agent
  gets one answer from the standing instruction and a contradicting one from inside the spec
  workflow, which is precisely where design conversation happens. Requirement "Agents act on
  conversation about a design" is not satisfied by fixing one of three copies.
- **Rejected**: touching only `templates/agents/design-trigger.md` and treating the others as a
  follow-up. It would ship a self-contradicting instruction surface, and
  `templates/skill_list_command_test.go` already pins which commands each skill must name, so the
  skill's command list has to be revisited anyway.

### The project's own design-folder README is corrected, the plan's public docs are not extended (components)
- **Decision**: correct `.spektacular/design/README.md:22` ("Nothing it writes adds frontmatter to
  a file in here") as part of this work, but do not treat the project README or any other
  in-repo prose as a documentation deliverable beyond the narrowing of that one claim.
- **Rationale**: that sentence becomes false for this project the moment `design author` exists,
  and it sits in the folder the feature writes into, so a reader hits it at exactly the wrong
  moment. It is a dogfooding artifact of this project, not a template, so correcting it is a
  one-line change with no product surface. The spec's documentation requirement names the
  documentation site specifically, so the site is where the explanatory work belongs.
- **Rejected**: leaving it, on the grounds that it is only this project's own file. A false
  statement in the design folder is the first thing a contributor reads about design documents
  here.

### Back-links are spec names, not full addresses (data structures)
- **Decision**: `Specs` is a `[]string` of spec names, not a list of structured references.
- **Rationale**: a spec is addressed by name alone everywhere else in the system; `design ref add`
  receives a name, `spec file list` returns names, and there is exactly one spec store per
  project. `DesignRef` needs two halves only because a path is ambiguous across several declared
  design sources, and that ambiguity has no counterpart on the spec side.
- **Rejected**: a struct mirroring `DesignRef` for symmetry. It would carry a constant field and
  invite the question of what a second spec store would mean, which the project does not have.

### `design author` publishes status and created date in its result (data structures)
- **Decision**: the authored-write result returns `document_status` and `created_date` alongside
  the address and location the verbatim write already returns.
- **Rationale**: the caller has just stamped a lifecycle block and the two facts it most needs to
  confirm are which status was applied and whether this was a first write or an update to an
  existing document. The status-setting subcommand on the store-file commands already reports the
  resulting status the same way, so this follows an established output shape rather than inventing
  one.
- **Rejected**: returning the verbatim write's envelope unchanged, which would force a follow-up
  `design list` or `design read` to learn what was just written.

### Two distinct error codes for the back-link failure path (data structures)
- **Decision**: a failed back-link write and a failed rollback of the spec get separate error
  codes rather than one code with different message text.
- **Rationale**: the project's error convention requires a runnable next action, and the two cases
  have genuinely different ones. A failed back-link with a successful rollback leaves a consistent
  system and the next action is to retry. A failed rollback leaves a spec and a design
  disagreeing, and the next action is manual repair of two named files, which the spec makes an
  explicit constraint. One code would force a single next action that is wrong for one of them.
- **Rejected**: a single code distinguished only by prose, which an agent matching on the code
  would handle identically and therefore wrongly in the worse case.

### The two-document write sequence is shared, not duplicated (implementation detail)
- **Decision**: `ref add` and `ref remove` share one helper implementing capture-write-compensate,
  parameterised by how the new reference list is computed.
- **Rationale**: the two verbs differ only in list arithmetic; the consistency rule, the failure
  behaviour and the two error codes are identical. Writing the sequence twice is how the remove
  path ends up with a subtly different rollback, and the spec holds both verbs to the same
  guarantee.
- **Rejected**: inlining the sequence in each verb, which reads more directly at each call site
  but doubles the surface where the hardest-to-test behaviour in the feature can drift.

### No new harbor end-to-end suite (testing approach)
- **Decision**: add no harbor suite for the design skill; check the existing suites for drift
  instead.
- **Rationale**: the harbor layer exists to prove a real agent drives a workflow state machine in
  the right order, and this feature deliberately adds no state machine. A static playbook has no
  step order, no goto sequence and no completed-steps oracle, so a suite would have nothing to
  assert beyond what the template-contract tests already cover, at roughly 25 minutes a run
  outside CI.
- **Rejected**: a `design-workflow` harbor suite mirroring the others. It would encode a step
  order that does not exist, and the project's own testing-architecture note warns that harbor
  oracles are hand-maintained couplings that rot silently when they mirror nothing real.

### Four success metrics are classified manual, two behavioural (testing approach)
- **Decision**: classify "conversation produces a design", "users are helped to write designs",
  "the two classes do not confuse users" and "authored designs stay proportionate" as manual, and
  "bringing in an existing design costs no rework" and "back-links can be trusted" as behavioural.
- **Rationale**: the four manual ones are each statements about real use after delivery, about
  output quality, or about user perception, none of which an assertion can express. Rather than
  drop them, each is paired here with the testable proxy that does exist (the instruction surface
  actually tells the agent to offer; the interview states a stopping condition; the docs explain
  the distinction and build clean) so the implement workflow carries the proxy and the metric.
- **Rejected**: inventing weak automated proxies and presenting them as covering the metrics, for
  example counting authored designs in a fixture. That would report the metric as verified when
  nothing about real behaviour had been tested.

### The failed-rollback case needs an injected write failure (testing approach)
- **Decision**: test the compensating-write-failed path with an injected failure rather than
  attempting to provoke a real one.
- **Rationale**: it is the one outcome the spec makes a constraint and the one least likely to
  occur naturally, so leaving it untested means the hardest behaviour in the feature ships
  unverified. Provoking a genuine write failure (permissions, a full disk) is not portable and
  would violate the project's rule that tests own their filesystem.
- **Rejected**: leaving it to manual inspection, which would make the failure mode the spec cares
  most about the only one with no regression guard.

### Four milestones, CLI before prose before docs (milestones)
- **Decision**: metadata and the authored write first, back-links second, the skill and standing
  instruction third, documentation fourth.
- **Rationale**: each earlier milestone is what makes the next one honest. The skill cannot tell
  an agent to run `design author` before that command exists, and the documentation cannot explain
  what an authored design's fields mean before they are decided. Back-links are separated from
  milestone 1 because the two-document write is the riskiest mechanism in the feature and deserves
  its own validation point rather than being absorbed into a milestone about stamping metadata.
- **Rejected**: a single CLI milestone combining stamping and back-links (it would hide the
  compensating-write behaviour inside a much larger change); and putting documentation first to
  settle the vocabulary (the page would have to be written twice, since the field names are
  decided by the implementation).

### "Authored" means Split returns metadata AND no error (phases)
- **Decision**: a design document counts as one Spektacular authored only when
  `metadata.Split` returns a non-nil block *and* a nil error. Every call site treats the error
  case as "not authored" and does not propagate it.
- **Rationale**: probed against the real parser rather than assumed. A Spektacular block yields
  `(non-nil, nil)`; no block yields `(nil, nil)`; and a team's own frontmatter such as
  `title:`/`author:` yields `(nil, error)`, because `UnmarshalYAML` fails on the absent
  `created_date`. An unterminated block does too. Propagating that error would make `design list`
  and `design ref add` fail outright on a perfectly ordinary design file that happens to carry the
  team's own YAML header, which is exactly the file this feature promises not to disturb.
- **Rejected**: treating any leading `---` as an authored block (wrong for the team's own
  frontmatter, and would then try to merge into it); and propagating the parse error (turns a
  supported file into a broken one). Both were ruled out by the probe rather than on judgement.

### Phase 2.2 gets a test seam only if the filesystem route fails (phases)
- **Decision**: try to force the back-link write failure through file permissions inside the
  test's own temp directory first, and introduce a package-level factory variable for the design
  set only if that proves unreliable.
- **Rationale**: the permission route needs no production change and keeps the test owning its
  own filesystem. The fallback has a direct precedent in this codebase (`var sourceFS fs.FS =
  templates.FS` in `internal/agent/skills.go:36` exists for exactly this purpose), so it is a
  known-acceptable shape rather than a novel one if it is needed.
- **Rejected**: adding the seam unconditionally (production indirection that may not be needed);
  and skipping the test (it covers the one failure mode the spec raises to a constraint).

### A missing cross-table invariant is added rather than just satisfied (phases)
- **Decision**: alongside adding the new skill to `workflowSkills` and `workflowDescriptions`,
  add a test asserting every entry in the first has an entry in the second.
- **Rationale**: nothing currently enforces the pairing, so a skill added to one table and not the
  other renders an empty description into every non-Claude agent's command menu, silently. This
  feature is the second skill addition since those tables were created and the trap is real.
  Asserting over the whole table costs nothing more than asserting the one new row and stops the
  next addition repeating it.
- **Rejected**: adding only the row. It would leave the plan's own change correct and the next
  one just as exposed.

### No status-setting subcommand for designs (out of scope)
- **Decision**: a design's lifecycle status is set when it is authored or re-authored; no
  equivalent of the store-file status-setting subcommand is added.
- **Rationale**: no requirement or acceptance criterion asks for one, re-authoring already carries
  a status through, and the only lifecycle transition the spec mentions for designs is marking one
  superseded, which a re-author does. Adding a command nobody asked for widens the CLI surface an
  agent has to learn.
- **Rejected**: mirroring the store-file status subcommand for symmetry. Symmetry with the artifact
  stores is exactly what the design commands deliberately do not have, and that divergence is
  already signposted in the code.

## Rehydration cues

- `go run . repo list` - always first. Two repos: `spektacular` (root
  `/home/nicj/code/github.com/jumppad-labs/spektacular`) and `docs` (root
  `/home/nicj/code/github.com/jumppad-labs/spektacular-website`). Never assume the cwd is the
  code.
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs` - the
  conventions and glossary for both repos in full.
- `go run . spec file read 000055_design-authoring-skill.md` - the spec. 10 requirements, 12
  acceptance criteria, 10 constraints, 6 success metrics, 10 non-goals.
- `go run . design sources` / `go run . design list` - this project declares one source,
  `design`, resolving to `.spektacular/design`, currently empty of documents.
- `go run . design ref list --data '{"spec":"000055_design-authoring-skill"}'` - returns 0 refs,
  0 unresolved. **This spec carries no design references.**
- `go run . skill spawn-planning-agents` - the parallel-research playbook (exits 0).
  `go run . skill spek-knowledge` exits 1: that asymmetry is the constraint's whole basis.
- Re-read in this order to rebuild the design: `internal/metadata/metadata.go:22-99`, then
  `internal/metadata/merge.go:11-131`, then `cmd/design_ref.go:153-250`, then
  `cmd/design.go:13-23`, then `templates/agents/design-trigger.md`, then
  `templates/skills/workflows/spek-knowledge/SKILL.md`, then
  `templates/steps/spec/00b-interview.md`.
- `git show --stat caaa761` - the full 48-file surface 000054 touched, the best available map of
  what a design change reaches.
- In the docs repo: `git status` first (the 000054 work is uncommitted), then
  `src/pages/design-documents.mdx:25-30` (the "never adds frontmatter" paragraph),
  `how-it-works.mdx:372-393` (artifact metadata), `projects.mdx:176-212` (the guided-conversation
  transcript model).
