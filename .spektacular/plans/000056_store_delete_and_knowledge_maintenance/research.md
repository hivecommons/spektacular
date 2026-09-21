---
created_date: "2026-09-21"
document_status: final
closed_date: "2026-09-21"
---

# Research: 000056_store_delete_and_knowledge_maintenance

## Alternatives considered and rejected

**Register `knowledge` and `design` as further instances of the shared store-file factory.**
`newStoreFileCmd` (`spektacular:cmd/storefile.go:172`) already builds `write/read/delete/list`
for `spec`, `plan` and `changelog`, and its delete arm (`:262-273`) is three lines. Rejected:
the factory merges Spektacular lifecycle frontmatter into every write and re-reads it on every
listing (`:212-232`, `:312-325`), which `cmd/design.go:14-24` records as the specific reason the
design commands were hand-written, and knowledge entries carry their own `tags:` block rather
than lifecycle metadata. Adopting the factory to obtain one verb would drag the whole write
path with it. The spec's Technical Approach reaches the same conclusion independently.

**Cascade a design delete into the referencing specs.** Rejected during the spec conversation
and recorded in `.spektacular/working-context.md`: it is the same multi-document write that
`applyRef` (`spektacular:cmd/design_ref.go:288-319`) needed a compensating rollback for, with
more documents in play and no transaction available. The user chose refusal
("I like delete while referenced, the agent can always remove the references, this forces a
cleanup").

**Delete the design and leave the spec's references dangling.** Cheapest, and contradicts
000054's acceptance criterion that an authored design and the specs referencing it never
disagree (`spektacular:cmd/design.go:350-376` exists solely to protect that invariant against
`design write`).

**Find the referencing specs by scanning the spec store.** Rejected: the relationship is already
recorded on the design itself as the `specs:` back-link list, written by
`writeBackLink` (`spektacular:cmd/design_ref.go:229-258`) and surfaced by
`design list` (`spektacular:cmd/design.go:284-286`). A scan would be a second, drift-prone
source of truth for a fact the document already carries.

**Exclude category `README.md` inside the shared `listFiles` walk**
(`spektacular:internal/knowledge/set.go:617-639`). Rejected: `listFiles` backs `List` and `Tags`
as well as `readCategories`, and the maintenance intent's drift report needs `knowledge list` to
keep returning the READMEs so it can read and compare them. Excluding there would remove the
very entries the new drift check must enumerate.

**Exclude category `README.md` inside `FileStore.search`.** Rejected: the store is deliberately
category-agnostic — `spektacular:internal/store/search.go:44-48` states that tier-based exclusion
of always-applied categories lives in the knowledge layer and never here, and
`internal/knowledge/set.go:214-226` is where the existing always-applied exclusion sits. A second
exclusion rule in a different layer would make the two drift.

**Add a Go command for maintenance.** Rejected: the spec's Technical Approach places maintenance
in the existing `spek-knowledge` skill as a further intent, and the skill's `audit` intent
(`spektacular:templates/skills/workflows/spek-knowledge/SKILL.md:99-121`) already establishes the
exact shape required — read-and-propose only, per-entry confirmation, composes existing
primitives, "adds no new command, no bulk operation, and no second write path". A Go command
would duplicate judgement the prose layer owns.

**Leave category-README regeneration alone and point the refusals at `repo add`.** Rejected
because it would make the refusal's next action untrue. `EnsureFootprint`
(`spektacular:internal/repo/footprint.go:93-100`) writes a category README **only when it is
absent** (`os.Stat` / `os.IsNotExist` guard), so a drifted README is never brought back into
line by re-running `repo add`, and the delete refusal is required to say how to regenerate one.
Verified live: `.spektacular/knowledge/architecture/README.md` matches the registry while
`spektacular-website/.spektacular/knowledge/architecture/README.md` still carries the superseded
pre-000037 purpose and boundary text, and no existing command repairs it.

## Chosen approach — evidence

**The storage half is already done.** `store.Writer` declares
`Delete(path string) error` with "Returns nil if the file does not exist"
(`spektacular:internal/store/store.go:104-105`); `FileStore.Delete`
(`:176-186`) swallows `fs.ErrNotExist` and returns nil; `ignoreStore` forwards it
(`spektacular:internal/store/ignore.go:87`). Any future provider implementing `Writer` supplies
`Delete` alongside `Write`, one interface. Idempotent-delete-succeeds therefore falls out of the
existing contract rather than needing to be built, and the docs site already states it publicly
(`docs:src/pages/extending.mdx:38-39,94-95`).

**Knowledge addressing and its refusals.** `Set.resolve`
(`spektacular:internal/knowledge/set.go:537-556`) is the single place an address becomes one
store, and every refusal it produces names the stores available in the tier concerned. A new
`Set.Delete` composes `resolve` + `src.store.Delete`, inheriting `ErrCodeStoreUnknown` and the
tier/name validation with no new refusal logic. `Set.Read` (`:331-350`) and `Set.Write`
(`:352-361`) are the shape to copy.

**Design addressing and its refusals.** `Set.lookup`
(`spektacular:internal/design/design.go:150-160`) is the equivalent single gate; `unknownSource`
and `incompleteAddress` (`spektacular:internal/design/errors.go:36-53`) already produce the
"undeclared source" refusal listing the declared names. `Set.Write` (`design.go:252-264`) shows
the exact `lookup` → empty-path check → `writer == nil` read-only check → store call sequence a
`Set.Delete` follows.

**The back-link check is a read, not a scan.** `authoredMetadata`
(`spektacular:cmd/design.go:204-210`) turns a design document's bytes into its lifecycle block or
nil, deliberately swallowing parse errors so a team's own YAML header reads as "not authored".
`fm.Specs` is the list of referencing specs. `design list` already projects it
(`cmd/design.go:284-286`). A document with no lifecycle block has no back-links, which is exactly
the spec's "a design the project did not author carries no record of referencing specs".

**Success envelopes may carry a `next_action`.** `runDesignRefList`
(`spektacular:cmd/design_ref.go:486-495`) sets `next_action` on a **success** result when
`unresolved > 0`. That is the precedent for `knowledge write` reporting unreachable tags without
failing the write, and for a delete reporting what it did.

**Where the always-applied exclusion already lives.** `Set.Search` applies it after the merge
(`spektacular:internal/knowledge/set.go:214-226`), reading from the single registry derivation
`AlwaysApplied()` (`spektacular:internal/knowledge/category.go:117-125`). `Set.Tags` applies the
same exclusion (`set.go:422-425`). `readCategories` (`set.go:493-519`) is the always-applied
payload's walk. Search and the always-applied reader do **not** share a walk, so one predicate is
applied at two call sites.

**Category READMEs are rendered from the registry.** `Category.README()`
(`spektacular:internal/knowledge/category.go:105-111`) is the single renderer; both write sites
call it — `internal/project/init.go:161-168` (project stores, unconditional overwrite) and
`internal/repo/footprint.go:93-100` (repo stores, only when absent). Drift is therefore
detectable by rendering the registry and comparing bytes, and repairable only by making the
footprint path rewrite a differing README.

**Empirical confirmation of the three audit findings.**
- `knowledge search "purpose belongs elsewhere entry shape"` returns eight hits, every one a
  category `README.md` (`architecture`, `decisions`, `gotchas`, `learnings` across both repo
  stores). Confirms READMEs in looked-up categories compete with real entries.
- `knowledge always-applied --tier repo --filter spektacular --filter docs` returns
  `conventions/README.md` and `glossary/README.md` from both stores in the payload injected on
  every task. In the `spektacular` store the glossary holds nothing else.
- `conventions/store-files-must-be-written-through-the-cli.md` carries
  `tags: [storage, paths, cli, artifacts]` and
  `conventions/tests-must-not-depend-on-order.md` carries
  `tags: [testing, isolation, shuffle, cobra, flake]`; both are unreachable, since `Set.Search`
  skips always-applied categories (`set.go:221`) and `Set.Tags` omits them from the vocabulary
  (`set.go:422-425`).
- `diff .spektacular/knowledge/architecture/README.md
  ../spektacular-website/.spektacular/knowledge/architecture/README.md` differs on the Purpose and
  Belongs-elsewhere lines; the `spektacular` copy matches `knowledge.Categories`, the `docs` copy
  does not.

**Baseline.** `go test ./...` is green at `8520f82` on branch `f-migrate`, so any failure during
implementation is attributable to this work.

**Docs site shape.** `docs:src/pages/knowledge-base.mdx` "The lifecycle of an entry"
(L94, body L106-153) and `docs:src/pages/design-documents.mdx` "Working with designs from the
command line" (L259, verbs L272-324) are the two insertion points; the refusal JSON pattern to
copy is `design-documents.mdx:370-382`. Both pages compose `Section` + `Prose nested` with fenced
`bash`/`json` blocks and need no new imports and no nav change. Note the two pages use different
JSON spacing conventions and different body-flags (`knowledge write --file`,
`design write --from`); match the page being edited.

## Files examined

- `spektacular:cmd/storefile.go:153-273` — the shared `write/read/delete/list` factory; its
  delete arm calls `st.Delete` and emits no JSON envelope at all.
- `spektacular:cmd/storefile.go:212-232` — the lifecycle-frontmatter merge on every write, the
  reason design and knowledge cannot adopt the factory.
- `spektacular:cmd/design.go:14-24` — the signposted comment forbidding consolidation with the
  factory.
- `spektacular:cmd/design.go:87-161` — the `--data` address schema and the output schemas each
  design verb publishes; a delete verb needs its own pair.
- `spektacular:cmd/design.go:171-184` — `designAddressData`, the shared `--data` parser, and its
  `design_data_required` refusal.
- `spektacular:cmd/design.go:204-210` — `authoredMetadata`; the discriminator between an authored
  design and the team's own file, and the source of the back-link list.
- `spektacular:cmd/design.go:350-376` — `design_authored_overwrite`, the closest existing
  precedent for a refusal that protects a lifecycle block, including its quoted copy-pasteable
  next action.
- `spektacular:cmd/design.go:499-512` — command registration; where a delete subcommand and its
  flags are wired.
- `spektacular:cmd/design_ref.go:194-258` — `bareSpecName`, `nextBackLinks`, `writeBackLink`: how
  a design's `specs:` list is maintained.
- `spektacular:cmd/design_ref.go:288-319` — `applyRef`'s compensating rollback, the concrete cost
  of a multi-document write and the argument against cascading.
- `spektacular:cmd/design_ref.go:387-428` — `design ref remove`, the command a delete refusal must
  point at, and its idempotent no-op shape.
- `spektacular:cmd/design_ref.go:439-498` — `design ref list`; sets `next_action` on a success
  envelope, the precedent for reporting without failing.
- `spektacular:cmd/knowledge.go:240-248` — `knowledgeAddressInputSchema`, reused by a delete verb
  unchanged.
- `spektacular:cmd/knowledge.go:450-471` — `runKnowledgeWrite`, the shape a delete handler copies
  and the place an unreachable-tag report is emitted.
- `spektacular:cmd/knowledge.go:566-582` — `knowledgeAddressData`; note its `--data is required`
  path is a bare `fmt.Errorf` with no next action, unlike its design counterpart.
- `spektacular:cmd/knowledge.go:602-615` — command registration and flag wiring.
- `spektacular:internal/store/store.go:98-115` — `Writer`/`Store`; `Delete` is already part of the
  contract every writable provider must satisfy.
- `spektacular:internal/store/store.go:176-186` — `FileStore.Delete`; idempotent by construction.
- `spektacular:internal/store/ignore.go:87` — the ignore-aware wrapper forwards `Delete`.
- `spektacular:internal/store/search.go:44-48` — the store is category-agnostic by design;
  exclusions belong to the knowledge layer.
- `spektacular:internal/store/frontmatter.go:51-76` — `ParseEntry`, how an entry's `tags:` block is
  read; the input to the unreachable-tag check.
- `spektacular:internal/knowledge/set.go:214-226` — the always-applied search exclusion, where the
  README exclusion joins it.
- `spektacular:internal/knowledge/set.go:311-318` — `categoryOf`, the first-path-segment rule a
  category-description predicate builds on.
- `spektacular:internal/knowledge/set.go:331-361` — `Set.Read` and `Set.Write`; the two-line shape
  `Set.Delete` mirrors.
- `spektacular:internal/knowledge/set.go:366-383` — `Set.List`; must keep returning READMEs so the
  drift check can enumerate them.
- `spektacular:internal/knowledge/set.go:408-449` — `Set.Tags`; already excludes always-applied
  categories, which is why tags there are inert.
- `spektacular:internal/knowledge/set.go:493-519` — `readCategories`, the always-applied payload
  walk and the second call site for the README exclusion.
- `spektacular:internal/knowledge/set.go:537-556` — `Set.resolve`; the single address gate a delete
  composes.
- `spektacular:internal/knowledge/category.go:57-100` — the `Categories` registry, the oracle a
  drift check compares against.
- `spektacular:internal/knowledge/category.go:105-125` — `Category.README()` and `AlwaysApplied()`.
- `spektacular:internal/project/init.go:157-168` — project-tier README write; unconditional
  overwrite.
- `spektacular:internal/project/init.go:170-195` — the cascade that calls `EnsureFootprint` for
  every registered repo, so `init` is the single remedy once the footprint path repairs drift.
- `spektacular:internal/repo/footprint.go:20-100` — `EnsureFootprint`; strictly additive today,
  which is why a drifted README is never repaired.
- `spektacular:internal/output/writer.go:36-84` — `ErrorResponse` and its builder chain; there is
  no warning channel, so a non-fatal report travels as fields on the success envelope.
- `spektacular:cmd/root.go:30-75` — `runUnknownSubcommand`; a new subcommand automatically joins
  the named-subcommand list an unknown-subcommand refusal reports.
- `spektacular:templates/skills/workflows/spek-knowledge/SKILL.md:99-129` — the `audit` intent and
  the decline-handling section; the exact shape the maintenance intent is modelled on, including
  the per-entry confirmation rule.
- `spektacular:templates/agents/store-access.md:7-43` — the managed AGENTS.md section that names
  every CLI surface; it says nothing about removal today.
- `spektacular:docs/knowledge-base.md:381-397` — the repo's own command-reference table.
- `spektacular:README.md:136-144` — the repo's own `knowledge` subcommand list.
- `docs:src/pages/knowledge-base.mdx:94-154` — "The lifecycle of an entry"; Creating / Searching
  and retrieving / Keeping it up to date. Removal slots after L153.
- `docs:src/pages/knowledge-base.mdx:106-118` — the command-documentation house style on that page
  (bolded lead-in, `--file`, spaced JSON).
- `docs:src/pages/knowledge-base.mdx:421-426` — a refusal documented in prose with no JSON block;
  the precedent for `knowledge delete`'s refusals.
- `docs:src/pages/design-documents.mdx:259-352` — the command section; `design delete` slots after
  the `design author` prose at L311 and before the reference verbs at L313.
- `docs:src/pages/design-documents.mdx:370-382` — the only JSON refusal example on the site; the
  template for the referenced-design refusal.
- `docs:src/pages/design-documents.mdx:385-391` — the existing two-document transactional prose,
  the anchor for sharpening the delete-vs-remove-a-reference distinction.
- `docs:src/pages/extending.mdx:38-39,94-95` — the site already states `Delete` is idempotent at
  the provider layer; new prose must not contradict it.
- `docs:src/components/sections/Section.astro`, `Prose.astro`, `Hero.astro`, `CtaBanner.astro`,
  `Button.astro` — the five components both pages import; no new component or import is needed.
- `docs:src/components/Nav.astro:6-22` — both pages are already registered; adding a section needs
  no nav change.
- `docs:package.json:7-12`, `docs:Makefile:9-16` — `npm run build` and `make check`
  (`npx astro check`) are the two gates; CI runs the build only.
- `docs:src/content.config.ts` — the content collection covers `src/content/tutorials` only, so
  page frontmatter is unvalidated.

## Test surfaces examined

The repository's test architecture has three layers (`spektacular` knowledge entry
`architecture/testing-architecture.md`): Go unit/step tests, template-contract tests that assert
anchor phrases in rendered prose, and harbor E2E suites that do not run in CI. All three are in
play here, and the hand-maintained oracles below must move in the same change as the surface they
mirror.

**There is no delete test anywhere in the repository.** `grep -rn '"delete"' cmd/ internal/
templates/` returns nothing. `spec file delete`, `plan file delete` and `changelog file delete`
share one implementation (`spektacular:cmd/storefile.go:262-273`) and have zero coverage. Only
`internal/store/store_test.go:43,49,103,124` exercises `FileStore.Delete` directly, including the
missing-file case and path-escape rejection. So there is no delete precedent at the command layer
to copy, and the constraint "the behaviour of the three stores that can already delete must not
change" is currently unenforced by any test — characterising it is cheap and turns the constraint
into something verifiable.

Also noted: that shared `del` command uses `storeFileStore(dir)` rather than `resolveStore`, so
`changelog file delete --repo <name>` silently ignores the repo routing that write/read/list
honour. Out of scope here; recorded so it is not mistaken for something this change introduced.

**Hand-maintained oracles that must be updated.**

- `spektacular:internal/agent/instruction_surface_test.go:426-436` — `knowledgeSubcommands`, the
  closed set of verbs registered by `cmd/knowledge.go`, hand-maintained to avoid an import cycle.
  `"delete": true` must be added or every rendered-skill scan rejects a `knowledge delete`
  invocation in the new intent.
- `spektacular:internal/agent/instruction_surface_test.go:443-461` — the branch-count contract:
  `"picks one of four branches (lookup / contribute / update / audit)"` (:448),
  `"One skill handles all four intents"` (:450), the audit's natural-language trigger (:452), and
  a stale-phrase ban list `{"one of three branches", "all three intents"}` (:457). A fifth branch
  changes all four, and `"one of four branches"` / `"all four intents"` join the ban list.
- `spektacular:internal/agent/instruction_surface_test.go:97-104` — `expectedCRUDInvocations`;
  gains `knowledge delete`.
- `spektacular:internal/agent/instruction_surface_test.go:407-419` —
  `spekKnowledgeAuditSection` slices from `# Intent: audit` to the next `\n# `. A maintenance
  section inserted between the audit and `# Decline handling` is therefore safe, but the four
  audit tests that use it must be re-checked and a parallel
  `spekKnowledgeMaintenanceSection` helper added for the new section's own assertions.
- `spektacular:internal/agent/instruction_surface_test.go:561-586` —
  `TestRenderedSpekKnowledgeAuditConfirmsPerEntry` asserts the per-entry carve-out against the
  whole rendered skill, in `# Decline handling`. Maintenance needs its own per-entry carve-out
  there, and this test's scope assumption must be revisited.
- `spektacular:cmd/knowledge_test.go:1979-1988` — `knowledgeConfigLoadingCmds`, a map of
  subcommand to args driving the config-loading and untagged-base sweeps; `delete` must be added
  or the new verb is silently exempt. The same list feeds
  `TestKnowledge_EveryCommandRunsAgainstAnUntaggedKnowledgeBase` (`:2134-2140`).
- `spektacular:cmd/knowledge_test.go:87-93` — `knowledgeNarrowingCmds`; only if delete took
  `--tier`/`--filter`, which it must not: delete is an addressed verb, not a fan-out one.
- `spektacular:cmd/design_test.go:747-861` — `TestDesignSchema_PublishesDocumentedShapes` has one
  `t.Run` per subcommand with hand-listed field names; a delete verb needs its own.
- `spektacular:cmd/design_test.go:862+` and `cmd/design_ref_test.go:527-565` — the refusal tables,
  which require a non-empty `NextAction` and assert its content, not merely its presence (the
  rule the `remediation-needs-the-layer-that-holds-the-facts` gotcha insists on).
- `spektacular:internal/knowledge/set_test.go:551-570` —
  `TestSet_AlwaysAppliedEntriesReturnsAllAlwaysAppliedCategories` asserts an exact
  `ElementsMatch` against three literal entries. It is the test the README exclusion most directly
  touches: adding a `conventions/README.md` to the fixture must leave this list unchanged.
- `spektacular:internal/knowledge/set_test.go:476-494, 500-549, 1995-2010` — the three existing
  always-applied exclusion tests (search, the re-tier coupling, tags). The README rule sits beside
  them and must not weaken them. Note `TestRetier_FlipsLoadAndSearchExclusionTogether` mutates the
  `Categories` registry in place and is not parallel-safe.
- `spektacular:cmd/knowledge_test.go:617-640` — `alwaysAppliedProject` seeds no `README.md`, so
  today's fixtures cannot catch a README-exclusion regression. A fixture that seeds one is part of
  the work, not an optional extra.
- `spektacular:internal/knowledge/category_test.go` (64 lines) — five tests, none of them covering
  `Category.README()`. The renderer that the drift check compares against is currently untested.
- `spektacular:internal/repo/footprint_test.go:22-60` —
  `TestEnsureFootprint_MissingREADMERepairedWithoutRewritingOthers` pins the current
  create-only-when-absent behaviour with a sentinel. Making the write repair drift changes what
  this test asserts, deliberately.
- `spektacular:cmd/docs_test.go` — sweeps `README.md` and `docs/knowledge-base.md`; the
  superseded-`scope:` bans (`:198-244`, `:308-328`) and the precedence-claim ban list (`:255-261`)
  apply to any new prose there. A tier/name/path `--data` example is fine.
- `spektacular:internal/agent/store_access_test.go:94-125` —
  `TestRenderedStoreAccessSectionNamesEveryStoreCommand` carries a hand-maintained needle list
  including `knowledge` and `design`, plus the three write-it-yourself exceptions. The managed
  section is where "removal goes through the CLI too" belongs.

**Contract tests a new subcommand passes for free but must not break.**
`cmd/instruction_contract_test.go` sweeps every rendered skill: `TestNextCommandsCarryPrefix`
(`:307-326`) rejects any surviving `{{` in the corpus, `TestContextMdAlwaysQualified` (`:417`)
requires every `context.md` mention to be qualified, and
`TestNoEmittedInstructionNamesOldWorkingContext` (`:354`) bans the old path.
`templates/data_payload_wellformed_test.go:40` walks every `.md` in `templates.FS` and requires
each `--data '{…}'` example to balance its braces and close its quote.
`cmd/no_project_test.go:42-78` has per-command `t.Run` blocks asserting the `no_project` refusal,
with `knowledge sources`, `design sources` and `design read` already present; the two delete verbs
are the natural additions.

**Skill rendering and propagation.** `internal/agent/skills.go:26-33` (`workflowSkills`) maps each
skill to its template path; `installWorkflowSkills` (`:44-69`) renders `{{command}}` through
mustache from `sourceFS` and writes to the agent's skills directory
(`.claude/skills` — `internal/agent/claude.go:22`). Every `TestRenderedSpekKnowledge*` test renders
through that production path rather than reading the committed copy, deliberately. The committed
`.claude/skills/spek-knowledge/SKILL.md` is regenerated by `go run . migrate`
(`cmd/migrate.go:28,54`), so the template edit and the regenerated copy land in the same commit.

**Harbor.** Only the plan-workflow suite carries knowledge or design oracles:
`CONVENTIONS_READ_COMMAND = "knowledge always-applied"`
(`spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py:114`) and
`DESIGN_REF_LIST_COMMAND = "design ref list"` (`:130`). Neither changes unless a step template
changes, and no step template changes here, so no harbor oracle needs editing. The suite's
environment (`tests/harbor/plan-workflow/environment/Dockerfile:24-25`) seeds
`conventions/auth-audit-logging.md` but no `conventions/README.md`, so it would not catch a
README-leak regression either way. The spec, implement and repo suites carry no knowledge or
design oracle at all. Recorded explicitly so a later reader knows harbor was examined and found
untouched rather than skipped.

**An implementation avenue considered and rejected.** `.spektacular_ignore`
(`spektacular:internal/store/ignore.go`, `cmd/knowledge_ignore_test.go`) already omits paths from
list and search while leaving them readable and writable. Reusing it for category READMEs was
rejected: it would hide them from `knowledge list`, which the drift report needs in order to
enumerate them, and it would make the rule a per-project opt-in file rather than a property of
the category model.

## External references

None. Every decision in this plan is grounded in the repository's own code, its knowledge base and
the two prior plans below; no library documentation, RFC or external article was consulted.

## Prior plans / specs consulted

- `000054_project-level-design-documents` (plan) — establishes why `internal/design` parallels
  `internal/knowledge` without sharing an abstraction, why the design commands are hand-written
  rather than a fourth registration of the store-file factory, and the `store.Reader`/`store.Writer`
  split that makes a future read-only provider a named refusal. Its phase and milestone shape is
  the model for this plan's.
- `000055_design-authoring-skill` (spec/plan) — introduced `design author`, the lifecycle block on
  a design document, and the `specs:` back-link list this feature's refusal reads. Its phases 2.1
  and 2.2 are where the compensating-rollback cost of a multi-document write was paid, which is the
  evidence against cascading.
- `000050_knowledge-entry-tags` (plan) — introduced entry tags, `knowledge tags`, and the
  deliberate exclusion of always-applied categories from both search and the vocabulary. That
  exclusion is the direct cause of audit finding 2 and is explicitly not to be weakened.
- `000056_store_delete_and_knowledge_maintenance` (spec) and
  `.spektacular/working-context.md` — the spec-phase record of the interview: the refuse-while-
  referenced decision in the user's own words, the "unmet target is not stale" distinction, and the
  three audit findings brought into scope.

## Open assumptions

1. **The `specs:` back-link list is the authoritative set of referencing specs.** Resolved rather
   than assumed: `applyRef` (`cmd/design_ref.go:288-319`) makes the spec write and the back-link
   write fail as a unit by compensation, and the single outcome that can leave them disagreeing —
   the compensation itself failing — is already reported under `design_ref_backlink_rollback_failed`
   with an instruction to repair the two named files by hand. The removal refusal reads the same
   record `design list` and `design ref list` report from, so in that state it is consistent with
   the rest of the tool rather than uniquely wrong. This project holds no design documents at all
   (`design list` returns an empty set), so there is no live instance and the fixtures are
   synthetic.
2. **A category description is identified by `README.md` as the final path segment under a
   registry category.** This matches both write sites, which only ever create `<category>/README.md`.
   A hand-placed `README.md` in a subdirectory of a category would also match. Assumed acceptable.
3. **Making `EnsureFootprint` rewrite a README whose bytes differ from the registry rendering is
   within the spec's intent.** The spec's Non-Goals exclude "automatic repair" and require drift
   repair to stay "a separate, deliberate act"; running `init` or `repo add` is read here as that
   deliberate act, and without it neither the delete refusal nor the drift report has a runnable
   remedy. Flagged for the walkthrough.
4. **`knowledge delete` and `design delete` return a JSON success envelope** rather than the silent
   nil that `spec file delete` returns today. The spec requires an idempotent removal to "report
   success", which the silent form does not do. The three existing delete verbs are unchanged, per
   the constraint.
5. **Maintenance is prose only.** No Go command is added for classification or for drift reporting;
   the skill composes `knowledge list`, `knowledge read`, `knowledge categories` and the new
   `knowledge delete`. Drift detection is the skill comparing a README's body against the
   `knowledge categories` output.
6. **The docs repo's `astro check` gate is `make check`, not an npm script.** Taken from the
   Makefile; if `make check` is unavailable, `npx astro check` is the direct equivalent.

## Drafting assumptions

These are the judgement calls made while drafting the plan, each recorded as it was taken, and
presented for challenge at the walkthrough rather than buried.

### Category-README exclusion is one predicate at two call sites, not one walk (discovery)
- **Decision**: Introduce a single `isCategoryDescription(path)` predicate in
  `internal/knowledge` and apply it at both retrieval surfaces — the post-merge filter in
  `Set.Search` (beside the existing always-applied exclusion) and the file loop in
  `readCategories`. `Set.List` and `Set.Tags` are left alone.
- **Rationale**: The spec's Technical Approach expects "one exclusion applied in the place that
  walks a store, covering both the search surface and the always-applied payload". There is no
  such single place: search goes through `store.Search` (a store-layer walk that is deliberately
  category-agnostic, `internal/store/search.go:44-48`) while the always-applied payload goes
  through the knowledge layer's `listFiles`. One rule, stated once, applied where each surface
  already applies the always-applied exclusion, is the closest honest realisation and keeps the
  two from drifting.
- **Rejected**: Excluding inside `listFiles` — it backs `List` and `Tags` too, and the new drift
  report needs `knowledge list` to keep returning the READMEs. Excluding inside
  `FileStore.search` — the store layer is documented as category-agnostic and a rule split across
  two layers would drift.

### Regenerating a drifted category description requires changing the footprint write (discovery)
- **Decision**: Make the category-README write in `repo.EnsureFootprint` rewrite a README whose
  bytes differ from `Category.README()`, not only create one that is absent, so that `init` (which
  cascades over every registered repo) becomes the single runnable remedy both the delete refusal
  and the drift report point at.
- **Rationale**: The delete refusal is required to say how to regenerate a category description,
  and the drift report is required to say how to bring one back into line. Today neither statement
  can be true: `EnsureFootprint` guards its write with `os.IsNotExist`
  (`internal/repo/footprint.go:94-99`), so a drifted repo-tier README is never repaired by any
  command. Verified live against the `docs` store's `architecture/README.md`.
- **Rejected**: Adding a dedicated regeneration command (more surface for one job `init` already
  almost does). Leaving the behaviour alone and writing a refusal that names no working remedy
  (violates the project's error-remediation convention). Note the spec's Non-Goal excludes
  *automatic* repair; running `init`/`repo add` is read here as the "separate, deliberate act" it
  asks for. Flagged for the walkthrough.

### Delete verbs return a JSON success envelope (discovery)
- **Decision**: `knowledge delete` and `design delete` write a success envelope naming what was
  addressed and whether anything was there, rather than the silent `nil` that `spec file delete`
  returns today (`cmd/storefile.go:262-273`).
- **Rationale**: The spec requires that deleting something absent "reports success rather than an
  error", which a silent exit does not do, and the project's CLI-for-agents architecture entry
  treats a predictable JSON envelope as the contract. The constraint that "the behaviour of the
  three stores that can already delete must not change" keeps the existing verbs as they are.
- **Rejected**: Matching the existing silent form for consistency — it would leave an agent unable
  to distinguish success from a swallowed failure, which is the failure mode the whole feature
  exists to stop.

### Unreachable-tag reporting is a non-fatal field on the write's success envelope (discovery)
- **Decision**: `knowledge write` still writes the entry, and reports unreachable tags as fields on
  its success result plus a `next_action`, rather than refusing the write.
- **Rationale**: The requirement is "tells the caller so, rather than storing them as dead weight",
  and the acceptance criterion is "reports that fact to the caller" — neither says refuse. There is
  no warning channel in `internal/output`, and `design ref list` already sets `next_action` on a
  success envelope (`cmd/design_ref.go:486-495`), so that is the established shape.
- **Rejected**: Refusing the write (would make an otherwise valid convention unwritable, and the
  constraint forbids weakening the always-applied exclusion, not the ability to write there).
  Emitting to stderr (not part of the JSON contract an agent reads).

### The repo's own README.md and docs/knowledge-base.md are kept in step (discovery)
- **Decision**: Include a small phase adding the two new verbs to
  `spektacular:README.md`'s knowledge subcommand list and
  `spektacular:docs/knowledge-base.md`'s command-reference table.
- **Rationale**: Both files carry per-command reference lists that would be silently wrong after
  this change, and plan 000054 set the precedent with its "Keep the repository's own configuration
  docs in step" phase. The spec's Non-Goal excludes documenting the *existing* spec/plan/changelog
  delete verbs, which this respects.
- **Rejected**: Restricting documentation strictly to the two site pages the requirement names,
  leaving the repo's own references stale.

### Category descriptions stay visible to `knowledge list` and `knowledge read` (discovery)
- **Decision**: The exclusion applies to `knowledge search` and the always-applied payload only.
  `knowledge list` keeps returning category READMEs, and `knowledge read` keeps returning their
  content.
- **Rationale**: The requirement names exactly two surfaces ("never returned by a search and never
  included in the material an agent receives on every task"). The new drift report needs
  `knowledge list` to enumerate the READMEs and `knowledge read` to fetch each one for comparison,
  so hiding them from either would break the feature it is meant to enable.
- **Rejected**: Following the `.spektacular_ignore` precedent (`internal/store/ignore.go`), which
  omits a path from both list and search while leaving it readable and writable. It hides one
  surface too many, and makes the rule a per-project opt-in file rather than a property of the
  category model.

### The three existing delete verbs get characterisation tests (discovery)
- **Decision**: Add coverage for `spec file delete`, `plan file delete` and `changelog file delete`
  as part of this work, rather than leaving them untested.
- **Rationale**: `grep -rn '"delete"' cmd/ internal/ templates/` returns nothing: the shared `del`
  command (`cmd/storefile.go:262-273`) has never been tested. The spec carries a constraint that
  their behaviour "must not change", and nothing currently enforces it. A small characterisation
  test turns that constraint into something the suite checks, and gives the two new verbs a
  precedent to match.
- **Rejected**: Leaving them uncovered (the constraint stays unverifiable). Rewriting them to match
  the new verbs' envelope (the constraint forbids it).

### Chosen direction: two sibling commands over the existing domain sets (architecture)
- **Decision**: `knowledge delete` and `design delete` are hand-written commands composing a new
  `Set.Delete` on `internal/knowledge` and `internal/design` respectively, addressed exactly as
  those families' existing verbs are. The category-README exclusion is one predicate owned by the
  category registry, applied at `Set.Search` and `readCategories`. Maintenance is a fifth intent in
  the existing `spek-knowledge` skill. The footprint's category-README write is changed to repair
  drift so `init` is a real remedy.
- **Rationale**: Every piece composes machinery that already exists and already carries the
  refusals this feature needs, so the new surface is the verbs and the rules, not new plumbing.
  The storage layer's `Delete` contract makes idempotent removal and future non-file providers
  free. Placing each refusal in the layer holding its remediation facts is the repo's own recorded
  gotcha.
- **Rejected**: A fourth registration of `newStoreFileCmd` (drags the lifecycle-frontmatter write
  path in). A single generic `delete --store <kind>` verb (the spec's constraint forbids inventing
  a new addressing scheme). A Go command for maintenance (the judgement belongs in prose). Full
  citations are in `research.md#alternatives-considered-and-rejected`.

### Conventions selected, and two deliberately dropped (architecture)
- **Decision**: Ten conventions apply across the two repos; `Alternate section background shading`
  is dropped because no new `Section` band is added, and the glossary is dropped because both
  stores' glossaries hold only the scaffolded README with no project terms.
- **Rationale**: The instruction requires relevance to be decided and recorded rather than asked,
  and an empty or padded list is a visible sign the knowledge base was not consulted. Each kept
  entry carries a rationale tying it to a specific choice in this plan.
- **Rejected**: Listing every loaded convention (dilutes the ones that actually bind). Listing none
  (untrue: the error-remediation and store-access conventions drive the core of the design).

### The unreachable-label judgement lives on the knowledge set, not the command layer (components)
- **Decision**: The knowledge set reports which of an entry's declared labels no search will reach;
  the command family only renders that report onto the write's success envelope.
- **Rationale**: The judgement needs both the retrieval tier of the destination category and the
  labels the entry declares. The set is the component holding both, and the repo's recorded gotcha
  says a check whose remediation names configured values belongs beside whatever owns them.
- **Rejected**: Computing it in `cmd` from the category registry and a re-parse of the staged body
  (duplicates knowledge the set already has, and puts the rule one layer above the facts).

### The category registry, not the knowledge set, states what a category description is (components)
- **Decision**: The statement that a category's rendered description is a descriptor rather than an
  entry lives in the category registry, beside the retrieval-tier declaration.
- **Rationale**: Three behaviours consult it — the retrieval exclusion, the delete refusal, and the
  drift comparison. The registry is already the single declaration of the category model and
  already renders the description, so adding the statement there keeps re-tiering or renaming a
  concern of one file, exactly as the existing tier declaration does.
- **Rejected**: Restating the filename at each of the three consuming sites (three copies of one
  rule, the drift mode the registry exists to prevent).

### A category description is exactly `<category>/README.md`, not any README (data_structures)
- **Decision**: `IsCategoryDescription` matches only a two-segment path whose first segment is a
  registry category and whose second is the description filename. A `README.md` deeper inside a
  category directory is treated as an ordinary entry.
- **Rationale**: Both write sites only ever produce `<category>/README.md`, so the strict form
  covers every generated descriptor. A contributor who puts a README inside a subdirectory wrote it
  themselves, and hiding it from search would be the exclusion overreaching into a caller's own
  content. This supersedes the looser reading recorded during discovery.
- **Rejected**: Matching any path whose final segment is `README.md` (hides files the feature has no
  claim over, and makes the rule a filename convention rather than a property of the category
  model).

### `knowledge delete` reports whether anything was there (data_structures)
- **Decision**: Both removal verbs return a `deleted` boolean on an otherwise successful envelope:
  true when a document was removed, false when the address was valid and held nothing.
- **Rationale**: The spec requires both outcomes to be successes, and an agent running a
  maintenance pass still needs to tell them apart to report honestly on what it changed. A boolean
  on a success envelope does that without turning the absent case into an error.
- **Rejected**: Returning nothing but `error: false` (an agent cannot report what it did).
  Returning `not_found` for the absent case (contradicts the requirement outright).

### The shared knowledge `--data` parser is brought onto the error convention (data_structures)
- **Decision**: Replace the bare `fmt.Errorf` for a missing `--data` in `knowledgeAddressData`
  (`cmd/knowledge.go:566-570`) with `knowledge_data_required`, carrying an example payload and a
  pointer to `knowledge sources`. This also affects the existing `knowledge read` and
  `knowledge write`.
- **Rationale**: The new delete verb goes through that same parser and would otherwise ship a
  refusal with no code and no next action, in direct breach of the project's error-remediation
  convention. Fixing the parser is strictly smaller than special-casing the new verb around it, and
  moves the two existing verbs toward the convention rather than away.
- **Rejected**: Leaving it and accepting one convention-violating refusal on a brand new command.
  Duplicating a compliant check in the delete handler only (two parsers, one of them still wrong).

### The two domain packages keep duplicate delete methods rather than sharing one (implementation_detail)
- **Decision**: `internal/knowledge` and `internal/design` each get their own `Delete`, with no
  shared helper extracted between them.
- **Rationale**: The two packages already parallel each other deliberately without sharing code
  beyond the store layer, and the design package carries a signposted comment explaining why. Their
  addressing differs (tier/name/path against source/path), their refusals differ, and a shared
  delete would be the first strand of an abstraction the project has twice chosen not to build.
- **Rejected**: A generic addressed-store helper covering both (couples two families that were
  separated on purpose, for the sake of about six lines).

### Three of six success metrics are split rather than classified whole (testing_approach)
- **Decision**: The "no agent reaches past the CLI" and "the refusal is acted on" metrics are each
  split into a behavioural half (the instruction corpus contains no raw-removal instruction; the
  refusal carries a runnable step and the clear-then-retry path provably works) and a manual half
  (what agents actually do), flagged with the required phrase.
- **Rationale**: Classifying either as wholly manual would discard a guarantee that is genuinely
  automatable and that this project already has machinery for. Classifying either as wholly
  behavioural would claim a test proves something about agent behaviour in the wild, which it
  cannot.
- **Rejected**: All-manual (throws away enforceable guarantees). All-behavioural (overclaims).

### The documentation pages get no prose assertions (testing_approach)
- **Decision**: The two site pages are verified by the site build and type check only; no phrase
  oracle is added for them. The repository's own reference documents remain covered by the existing
  contract sweeps.
- **Rationale**: Matches how every prior documentation change in this project has been verified, and
  a phrase oracle over marketing-site prose is maintenance cost without a failure mode the build
  does not already catch. Agent-facing prose is different and does get assertions, because an agent
  acts on it.
- **Rejected**: Adding phrase assertions for the site pages (cost without a corresponding failure
  mode). Adding none anywhere (would leave the skill's new intent, which agents act on, unguarded).

### The category-description statement lands in milestone 1, its retrieval use in milestone 2 (milestones)
- **Decision**: Four milestones. The registry statement of what a category description is arrives in
  milestone 1, because the delete refusal needs it; milestone 2 then consumes the same statement to
  keep descriptions out of retrieval.
- **Rationale**: Keeps every milestone independently deliverable and in dependency order. Milestone 3
  needs the removal verb from milestone 1; milestone 2 needs nothing from milestone 1 but the one
  statement, which milestone 1 has to introduce anyway.
- **Rejected**: Putting the whole category-description concern in one milestone before removal (the
  retrieval fix would block the headline feature on an unrelated defect). Three milestones with
  documentation folded into the others (documentation spans both repos and reads better as its own
  deliverable, as plan 000054 also found).

### `deleted` is established by a read before the removal (phases)
- **Decision**: Both verbs determine whether anything was there by asking the set before calling
  delete — a knowledge read that refuses with the entry-not-found code, and the design set's
  existing `Exists`.
- **Rationale**: `store.Writer.Delete` is contractually nil whether or not the file was there, so it
  cannot report the outcome. The design command already calls `Exists` on its write path for the
  same reason, and an extra read on a removal path costs nothing.
- **Rejected**: Changing `store.Writer.Delete` to report whether it removed anything (a change to an
  interface every provider implements, for a fact one caller wants). Reporting nothing (the spec
  wants both outcomes to be successes, but an agent still has to say what it did).

### The repo's own convention entry about store access is not edited by this plan (phases)
- **Decision**: Phase 3.2 updates the managed `templates/agents/store-access.md` section but leaves
  `.spektacular/knowledge/conventions/store-files-must-be-written-through-the-cli.md` alone, noting
  it should be proposed to the user afterwards.
- **Rationale**: A knowledge write requires the user's explicit agreement through the knowledge
  skill's propose-then-confirm flow. A plan cannot authorise one, and having an implementation phase
  silently write to the knowledge base would break the rule the feature is built to enforce.
- **Rejected**: Adding it as a phase (would have the implement workflow write to a knowledge store
  without the user's per-entry agreement).

### The maintenance review reports drift but never repairs it (phases)
- **Decision**: The drift step enumerates, compares and reports, naming `init` as the remedy, and
  explicitly says the review does not repair a drifted description itself.
- **Rationale**: The spec's Non-Goals exclude automatic repair and require bringing a store back
  into line to stay a separate, deliberate act. Saying so inside the intent, rather than leaving it
  unstated, is what stops an agent helpfully "fixing" it mid-review.
- **Rejected**: Letting the review offer to repair a drifted description per entry (contradicts the
  Non-Goal, and the repair is a whole-store regeneration rather than a per-entry change).

## Rehydration cues

- `go run . repo list` — the two repos and the absolute `root` each one's code lives at. Never
  assume the working directory.
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs` — the binding
  conventions for both repos. Note that this command's own output currently demonstrates audit
  finding 1: two category READMEs appear in it.
- `go run . knowledge search "knowledge store"` / `"cli command structure"` — reaches
  `architecture/knowledge-search-ranking.md`, `architecture/testing-architecture.md`,
  `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`, the three entries that most
  constrain this work.
- `go run . design ref list --data '{"spec":"000056_store_delete_and_knowledge_maintenance"}'` —
  returns `refs: []`, `unresolved: 0`. This spec references no design document.
- `go run . spec file read 000056_store_delete_and_knowledge_maintenance.md` — the spec.
- `go run . plan file read 000054_project-level-design-documents/plan.md` — the model for this
  plan's structure and for how the design command family was built.
- `go run . knowledge search "purpose belongs elsewhere entry shape"` — reproduces audit finding 1
  in one call; every hit is a category README.
- `diff .spektacular/knowledge/architecture/README.md
  ../spektacular-website/.spektacular/knowledge/architecture/README.md` — reproduces the category
  descriptor drift between the two stores.
- `go test ./...` in the `spektacular` root — the baseline gate; green before this plan started.
- `make check` and `npm run build` in the `docs` root — the documentation gates.
