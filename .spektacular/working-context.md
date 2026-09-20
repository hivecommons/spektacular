# Working context: 000055_design-authoring-skill (implement)

The spec and plan are closed. This file now holds the **implement** run's cross-cutting notes.
The plan documents are the source of truth for what to build; read them with
`go run . plan file read 000055_design-authoring-skill/{plan,context,research}.md`.

## Repos

`go run . repo list` reports two, both present on disk and both in scope:

- `spektacular` — root `/home/nicj/code/github.com/jumppad-labs/spektacular`. Phases 1.1 to 3.3
  and half of 4.2.
- `docs` — root `/home/nicj/code/github.com/jumppad-labs/spektacular-website`. Phases 4.1 and
  half of 4.2.

## Validation gate (read_plan) — passed clean

- **Structure**: all ten required `##` sections present, plus an extra `## Conventions`. Ten
  `#### - [ ] Phase` headings, every one carrying a `*Technical detail:*` link that resolves to
  a matching `### Phase N.M:` heading in `context.md`.
- **Drift**: no mismatches. Every file, package, Go symbol, test name, CLI command and template
  path named in `plan.md` and `context.md` was checked and still exists. Notably confirmed:
  `workflowSkills`/`workflowDescriptions` hold exactly five skills; the three per-agent test
  tables still carry the literal comment "Exactly five SKILL.md files"
  (`internal/agent/{bob,claude,codex}_test.go`) plus the wrapper table at `bob_test.go:43`;
  `design` has subcommands `sources|list|read|write|ref` and **no** `author` yet.
- **Spec coverage**: all 10 requirements and all 12 acceptance criteria map to a phase. Nothing
  descoped, so `plan.md` carries no `**Descoped requirements**:` list and should not gain one
  unless something is descoped later in this run.
- **Changelog mode**: `plan.md` has no `## Changelog` section, so this is a **first-phase**
  invocation. Start at phase 1.1.

## Open Question 2 resolved at the gate

The plan flagged that the `docs` working tree might have moved on. It has not. Checked on
branch `main`: `src/pages/design-documents.mdx` is still **untracked** at 319 lines with its
`<Section>` headings at lines 17, 40, 76, 120, 161, 241 and 282, exactly as planned against;
`src/components/Nav.astro` and `src/pages/configuration.mdx` are still modified-unstaged. So
every line reference in phases 4.1 and 4.2 is still good, and the "if the page is gone, STOP
and ask" branch does not apply.

## Open Question 1 RESOLVED — the answer is "both routes, for different cases"

Exercised for real in phase 2.2. The session runs as uid 1000, not root, and the filesystem does
enforce the permission, so the preferred no-seam route works **for the back-link-write-failure
case**: chmod the design document to 0444 and `set.Write` fails with EACCES. Confirmed live for
both `ref add` and `ref remove`, each returning `design_ref_backlink_failed` with the spec
byte-identical afterwards.

It does **not** work for the rollback-failure case, and this is a fact about `store.FileStore`
rather than about the platform. `FileStore.Write` is `os.MkdirAll` then `os.WriteFile`
(`internal/store/store.go:165-173`). Writing an existing file needs write permission on the file,
not on its directory, so any permission state that makes the rollback write fail makes the
identical first spec write fail too, and the run never reaches the compensation. The spec has to
become unwritable *between* the two writes, and the only thing running between them is the
back-link call itself.

So one narrow seam was added: `var writeBackLinkFn = writeBackLink` in `cmd/design_ref.go`, which
`applyRef` calls. A test substitutes it to both return an error and chmod the spec, and restores
it in `t.Cleanup`.

**Deviation from the plan**: the plan named `designSetFactory = newDesignSet` as the fallback
seam. `writeBackLinkFn` is the same shape and the same precedent (`var sourceFS fs.FS =
templates.FS`, `internal/agent/skills.go:36`) but a strictly smaller surface, and it is the only
one of the two that can also run the hook the plan itself said the rollback case requires: a
substituted design set can fail its write but cannot make the spec unwritable partway through.

## Standing traps to carry through every phase

- `internal/metadata`'s `yamlShape` is a **closed schema**: a field added to some but not all of
  `Metadata`, `yamlShape`, `yamlInShape` and the marshal pair does not fail, it silently loses
  the value on the second write. `Designs` is the worked precedent; copy it including the
  byte-exact guard `TestRender_OmitsDesignsKeyEntirelyWithNoReferences`.
- "Authored" means `metadata.Split` returns **non-nil metadata AND a nil error**. A team's own
  frontmatter yields `(nil, error)` and must be treated as not-authored at every call site, never
  propagated. One shared helper in `cmd/design.go`, landed in phase 1.3 and reused in 2.1.
- Skill templates write the CLI as `{{command}}`, never the rendered `go run .`, and must never
  instruct an agent to fetch a nested skill with `{{command}} skill spek-...`.
  `internal/agent/design_trigger_test.go:253` is the guard and stays unchanged.
- Hand-maintained oracles that fail silently until run: the three per-agent skill tables and
  their "Exactly five" comments, `bob_test.go:49-50`, `templates/skill_list_command_test.go:49-60`,
  and the five literals at `internal/steps/spec/steps_test.go:541,570,572,588,589`.
- `make test` runs `go test -shuffle=on ./...`; anything driving `rootCmd` goes through
  `resetRootCmd` + `runRootCmd`, and metadata tests inject `opts.Today`.

## Phase progress

- **1.1 Record which specs reference a design** — analysis done. All six change sites confirmed:
  `Metadata` (metadata.go:59), `yamlShape` (:88), `yamlInShape` (:103), `MarshalYAML` (:119),
  `UnmarshalYAML`'s `m.Designs = decodeDesignRefs(...)` line, and `decodeDesignRefs` itself.
  `UpdateOptions.Designs` and both `Merge` branches (fresh at merge.go:73, existing at :99)
  confirmed. Test patterns to mirror: `TestRender_SplitRoundTripDesignRefs`,
  `TestRender_OmitsDesignsKeyEntirelyWithNoReferences`,
  `TestSplit_MalformedDesignsReadAsNoReferences` in metadata_test.go, and the five
  `*[]DesignRef` tri-state tests in merge_test.go using `fixedToday()` / `documentStatusPtr`.
  Code landed: `Specs []string` added to `Metadata`, `yamlShape` (`specs,omitempty`),
  `yamlInShape` (raw node), `MarshalYAML`, `UnmarshalYAML` via a new lenient `decodeSpecNames`,
  plus `UpdateOptions.Specs *[]string` and both `Merge` branches. Builds clean, gofmt clean, and
  the existing `internal/metadata` suite including the byte-exact guard is still green
  unmodified. Tests added append-only by a sub-agent and independently re-verified: five merge
  tri-state tests plus round-trip, a sibling byte-exact guard naming both fields, and a lenient
  decode table that adds a list-of-non-scalar case the designs original does not have. New
  fixtures `twoSpecNames()` / `twoSpecNamesYAML` / `draftWithSpecs` / `specNamesPtr()` mirror the
  design-ref ones. Verified: `make test` (whole suite, shuffled, 21 packages) pass, `make lint`
  pass, `gofmt -l .` silent. Nothing downstream regressed despite the field reaching every
  artifact class. Plan ticked: `#### - [x] Phase 1.1` plus all five acceptance criteria, written
  back through `plan file write`. Changelog entry appended; `## Changelog` section created after
  `## Out of Scope` (first-phase invocation). **Phase 1.1 DONE.**
- **1.2 Write a design document Spektacular authored** — analysis done in main context (the
  plan's own agent strategy for this phase is single-agent sequential; the risk is the order of
  the composed pieces, not volume). Confirmed touchpoints: `designWriteCmd` (cmd/design.go:49),
  `designAddressInputSchema` (:74), `designWriteOutputSchema` (:112), `designAddressData` (:127),
  `newDesignSet` (:142), `runDesignWrite` (:232), flag registration in `init` (:277-287);
  `stripLeadingFrontmatterBlocks` (cmd/storefile.go:31) and `metadataOptsForDocumentStatus` (:48)
  are same-package and directly reusable; `parseDocumentStatusFlag` / `documentStatusValues` in
  cmd/artifactfilter.go. `cmd/design.go` does NOT yet import `internal/metadata` — that import is
  part of this phase. Test fixtures available: `writeDesignConfig`, `seedDesignDoc`,
  `stageDesignDoc`, `twoSourceDesignProject` (api = relative source, ux = absolute outside root).
  Code landed: `designAuthorCmd` + `designAuthorOutputSchema` + `runDesignAuthor` in
  `cmd/design.go`, flags `--data/--from/--document-status/--spec`, registered on `designCmd`,
  and `internal/metadata` added to that file's imports. Smoke-tested end to end in a scratch
  project against all eight acceptance criteria and all behaved correctly. One judgement call
  recorded below about the foreign-frontmatter refusal's wording. Tests added to
  `cmd/design_test.go` and independently re-verified: five `TestDesignAuthor_*` tests plus an
  `author` subtest on `TestDesignSchema_PublishesDocumentedShapes` and four on
  `TestDesignRefusals_CarryCodeAndNextAction`. New helpers `designAuthorResult` and
  `designAuthoredDoc(keys, body)`, the latter assembling expected documents from hand-written
  strings rather than `metadata.Render`, keeping the oracle independent. The revision test pins
  the load-bearing nil-`opts.Specs` behaviour: a seeded two-entry `specs:` list survives a
  body-only re-author. Verified: `make test`, `make lint`, `gofmt -l .` all clean, plus
  `go test -shuffle=on -count=3 ./cmd/` green, so the new package-global cobra flags do not leak.
  Plan ticked (8 criteria) and changelog entry appended. **Phase 1.2 DONE.**
- **1.3 Keep the two kinds of design document apart** — code landed in `cmd/design.go`: the
  shared `authoredMetadata(raw) *metadata.Metadata` discriminator (nil on BOTH the no-block and
  the parse-error case, which is the whole point of it existing), the `design_authored_overwrite`
  guard on `runDesignWrite`, and lifecycle fields on `runDesignList` plus the extended
  `designDocumentItemSchema`. Smoke-tested: an authored doc lists its lifecycle fields while a
  plain one and a team-frontmatter one list only source and path; verbatim write over an authored
  doc is refused with the file byte-identical afterwards; verbatim write over a team-frontmatter
  doc and to a brand new path both still succeed byte-exactly. `authoredMetadata` is the helper
  phase 2.1 reuses; it must not be written twice.
  Verified: `make test`, `make lint`, `gofmt -l .`, `go test -shuffle=on -count=3 ./cmd/` all
  clean. The test agent ran a mutation experiment against `cmd/design.go` and reverted it; checked
  and the file is intact. Plan ticked (7 criteria) and changelog entry appended.
  **Phase 1.3 DONE — MILESTONE 1 COMPLETE.**
- **2.1 Keep a design and its referencing specs in agreement** — code landed in
  `cmd/design_ref.go`: `bareSpecName`, `nextBackLinks`, `writeBackLink` and the shared `applyRef`
  that both verbs now call instead of `writeRefs` directly. `runDesignRefRemove` gained a design
  set it did not previously need. Order is fixed spec-first, back-link-second, and the remove
  path keeps its existing write-only-if-changed condition so a no-op remove does not rewrite the
  design. Smoke-tested: add records the bare spec name even when the caller spells it with `.md`;
  a duplicate add leaves one entry; two specs both appear and removing one leaves the other; a
  design with no block and one with the team's own frontmatter are byte-identical after an
  add-then-remove; a reference to a document that does not exist still succeeds and creates
  nothing. `writeBackLink` returns its error raw — phase 2.2 wraps it with the compensation and
  the two error codes.
  Verified green; plan ticked (7 criteria) and changelog appended. **Phase 2.1 DONE.**
- **2.2 Never leave a spec and a design disagreeing** — code landed: `writeBackLinkFn` seam and
  the compensation in `applyRef`, with `design_ref_backlink_failed` (retry) and
  `design_ref_backlink_rollback_failed` (repair two named files by hand) as two distinct codes.
  Smoke-tested live for both verbs. Verified green including `-count=3` for seam leakage. Plan
  ticked (5 criteria) and changelog appended. **Phase 2.2 DONE — MILESTONE 2 COMPLETE.**
- **3.1 Add the design skill** — code landed: new
  `templates/skills/workflows/spek-design/SKILL.md` (four intent branches, the Flipped Interaction
  interview citing arXiv:2302.11382 with its stopping condition stated as a testable rule, an
  anti-transcript rule, and a Decline handling gate), plus the two registry rows in
  `internal/agent/skills.go` and `internal/agent/commands.go`. Verified by grep that the template
  contains no `skill spek-` fetch and no rendered `go run .`.
  **Deviation from the plan's agent strategy**: the plan suggested 2 parallel agents (prose /
  wiring). Done in one pass in the main context instead, because the full template context was
  already loaded and a two-agent split would have added an integration merge for two registry
  lines. No scope change.
  Adding the skill deliberately breaks four hand-maintained oracles, which is the test step's
  work, not a regression: `agent_test.go:100` and `:145` (two fixture `fstest.MapFS` maps that
  must gain a `spek-design` entry), `agent_test.go:133` and `:222` plus their "exactly five"
  messages, the three per-agent tables and "Exactly five" comments in
  `internal/agent/{bob,claude,codex}_test.go`, `bob_test.go:49-50` (wrapper filenames, needs
  `design.md`), and `templates/skill_list_command_test.go` (needs a `spek-design` row). The plan
  also asks for a NEW cross-table invariant asserting every `workflowSkills` entry has a
  `workflowDescriptions` entry.
  Verified green (`make test`, `make lint`, `gofmt`, `-count=2` on the three affected packages).
  All oracles repaired by the test agent; a stale test NAME was also fixed
  (`WritesFourSkillFiles` asserted five). The migrate upgrade path turned out testable end to end
  and is now asserted. Plan ticked (7 criteria) and changelog appended. **Phase 3.1 DONE.**
- **3.2 Rewrite the standing design instruction** — `templates/agents/design-trigger.md`
  rewritten. Structure is now alert (noticing is continuous and costs nothing) → qualifying bar
  (unchanged three-part test, now explicitly the gate on OFFERING) → the three entry cases the
  old section missed (user already has it, an existing design is being changed, no spec exists)
  → offer → the three outcomes. The accept branch now hands off to the `spek-design` skill BY
  NAME and records a reference only when a spec exists. Heading kept byte-identical, managed
  banner kept, and every pinned literal kept verbatim. All existing
  `internal/agent` tests pass unchanged, including the `NotContains "skill spek-"` guard and the
  accept-branch assertion (both `design write` and `design ref add` are still named there, with
  `design author` added alongside).
  Verified green; plan ticked (6 criteria) and changelog appended. **Phase 3.2 DONE.**
- **3.3 Align the spec workflow's own design offer** — code landed in
  `templates/steps/spec/05-technical_approach.md` (the offer now covers both authoring and
  bringing in, and its accept branch names `spek-design` plus `design author` alongside
  `design write`) and `templates/skills/workflows/spek-new/SKILL.md` (command list gained
  `design author`; the `design write` bullet narrowed to documents Spektacular did not author;
  a sentence added saying a design need not already exist to be captured).
  `spek-plan/SKILL.md` deliberately untouched — its pre-existing modified state in git predates
  this session.
  **Trap hit for real**: my first wrap split `spektacular design write` across a newline, failing
  `TestTechnicalApproachStepNamesBothDesignCommands`. The test was right and the template was
  wrong — a command name broken over a line is worse for the agent reading it than for the test.
  Rewrapped so every command name stays contiguous. This is the second time line wrapping has
  bitten (see the 3.2 changelog note); **never let a command name straddle a line break in a
  template.**
  Sweep done: the only surviving "no frontmatter"/"byte for byte" claims in `templates/` are all
  now scoped to the verbatim `design write` verb.
  Verified green; plan ticked (5 criteria) and changelog appended.
  **Phase 3.3 DONE — MILESTONE 3 COMPLETE.**
- **LOOSE END TO TELL THE USER AT THE END**: the dogfooded installed copies
  `.claude/skills/spek-new/SKILL.md:58` and `.bob/skills/spek-new/SKILL.md:58` still carry the
  old unqualified frontmatter sentence, and the new `spek-design` skill is not installed into
  them either. They refresh only on `go run . init`, which is an explicit user-initiated action
  this workflow must not take. No test fails because of it: the guard walks the embedded
  templates and renders through the install path into a temp dir, never the committed copies.
- **4.1 Explain design authoring on the documentation site** (`docs` repo, root
  `/home/nicj/code/github.com/jumppad-labs/spektacular-website`) — docs tree re-checked and still
  exactly as planned against, so Open Question 2 stays resolved. Changes to
  `src/pages/design-documents.mdx`: the unqualified "never adds frontmatter" paragraph reworked
  into the two-class statement; two new sections added ("Working a design out with Spektacular",
  plain, with a transcript in a bare fenced block; "What an authored design records", surface,
  four bullets plus a fenced yaml example); the command reference extended with `design author`
  and a sentence distinguishing the two write verbs; and the reference-failure section extended
  with the fail-as-a-unit paragraph.
  **Surface alternation solved by inserting a PAIR rather than one section.** Adding two sections
  (plain then surface) after a surface section preserves the whole downstream run, so no existing
  `surface` value had to be flipped. Verified: 17 plain, 46 surface, 82 plain, 129 surface, 174
  plain, 218 surface, 259 plain, 339 surface, 380 plain.
  Gates: `grep -nE "<div|<section|class=" src/pages/*.mdx` empty, zero em dashes in the page,
  `npm run build` succeeds, `npx astro check` reports 0 errors / 0 warnings (1 pre-existing hint
  about `document.execCommand` in an unrelated component).
  **No Go tests exist for this phase** and none were invented: the plan's testing strategy makes
  the build and type-check the gate for the documentation phases.
  Plan ticked (7 criteria) and changelog appended. **Phase 4.1 DONE.**
- **4.2 Narrow the guarantee everywhere else it is stated** — both repos edited.
  `docs:src/pages/configuration.mdx` design section gained the two-class sentence;
  `spektacular:.spektacular/design/README.md` had its unqualified claim narrowed and gained
  `design author` in its command list. Sweep re-run: no unqualified claim survives anywhere. The
  one remaining `no frontmatter` hit in `knowledge-base.mdx:289` is about knowledge entries, not
  design documents, and is correctly out of scope.
  **Judgement call on the store-access rule**: `.spektacular/design/` is a declared design
  source, so its files would normally have to be written through `go run . design write`. This
  README is not a stored artifact though: `design list` returns an empty list and
  `.spektacular/design/.spektacular_ignore` names `README.md` explicitly. AGENTS.md makes the
  CLI's own listing the source of truth for what counts as a stored artifact, so editing it
  directly is correct rather than a violation.
  Gates: site builds, `astro check` 0 errors / 0 warnings, Go suite green, vet and gofmt clean.
  **ALL TEN PHASES COMPLETE.**
- **Test plan written** to `000055_design-authoring-skill/test-plan.md`. Four manual metrics with
  grounded procedures; the two behavioural ones named with the tests that cover them. Its Setup
  section carries the `go run . init` loose end as a precondition, since the procedures exercise
  agent-facing surfaces that are stale until reinstalled.
- **Changelog records written**: project-level `000055_design-authoring-skill.md`, plus repo-level
  records for both `spektacular` and `docs` via `--repo`. All four deviations recorded in the
  project record, and the `go run . init` follow-up named in both the project and the
  `spektacular` repo records.
- **Spec reconciled**: all 10 requirements and all 12 acceptance criteria flipped to `[x]`,
  each judged against the plan's changelog entries rather than blanket-ticked. Nothing left
  unchecked and nothing descoped.

## Decisions and substitutions made during this run

- **Not looping back to the user between phases.** The recorded preference is to drive multi-step
  workflows straight through and stop only for real design decisions, so `update_changelog`'s
  default continue-or-pause prompt is skipped and the run loops to `analyze` automatically. Stop
  anyway for anything the plan marks STOP-and-ask, for a verification failure, or for a genuine
  design choice.
- **No knowledge capture offered for phase 1.1.** The one discovery (a `[]string` lenient decoder
  needs an explicit `yaml.ScalarNode` check, where a `[]struct` one gets that protection free from
  `item.Decode`) is already recorded by `decodeSpecNames`' own comment and by the named
  `list of non-scalar` test case, so it is re-derivable from the code and does not clear the
  durable-and-non-obvious bar.
- **The `design_frontmatter_not_authored` refusal does not repeat `metadata.Merge`'s underlying
  error.** The raw cause is `parsing created_date "": ...`, which reads as a complaint that the
  team's block is missing a field and invites an agent to add one to it. That is the opposite of
  the intended remediation, which is to leave their block alone. The message states the problem
  in its own words instead, and the next action offers removing the block or authoring to a
  different path. The plan asked for the wrap; dropping the raw cause is the only addition.

## Answers the user gave

- Plan to implement: `000055_design-authoring-skill`.
