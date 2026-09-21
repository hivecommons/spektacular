---
created_date: "2026-09-21"
document_status: final
closed_date: "2026-09-21"
---

# Context: 000056_store_delete_and_knowledge_maintenance

## Current State Analysis

Two of Spektacular's five kinds of stored document cannot be removed. `spec file delete`,
`plan file delete` and `changelog file delete` all work, inheriting the verb from the shared command
factory at `spektacular:cmd/storefile.go:262-273`; `knowledge` and `design` are the two families
whose commands were hand-written instead, and neither ever gained it. The asymmetry is an assembly
artifact rather than a decision: the design family's divergence is deliberate and signposted at
`spektacular:cmd/design.go:14-24`, but for a reason with nothing to do with removal (the factory
stamps lifecycle frontmatter into everything it writes, which a design document must not get).

**The storage half already exists.** `store.Writer` declares `Delete(path string) error` documented
as returning nil when the file is absent (`spektacular:internal/store/store.go:104-105`),
`FileStore.Delete` implements it by swallowing `fs.ErrNotExist` (`:176-186`), and the ignore-aware
wrapper forwards it (`spektacular:internal/store/ignore.go:87`). Any future provider implementing
`Writer` supplies `Delete` with `Write`, one interface, so the work is almost entirely command-layer.

**Nothing tests removal.** `grep -rn '"delete"' cmd/ internal/ templates/` returns nothing. The
three working removals share one implementation with zero coverage; only
`spektacular:internal/store/store_test.go:43,49,103,124` exercises `FileStore.Delete` directly.
The shared `del` arm also resolves through `storeFileStore(dir)` rather than the `resolveStore`
closure at `:177-182`, so `changelog file delete --repo <name>` ignores repo routing that write,
read and list honour. Recorded, not fixed.

**The back-link relationship is already recorded on the design.** An authored design carries the
specs referencing it in its own lifecycle block, written by `writeBackLink`
(`spektacular:cmd/design_ref.go:229-258`) and surfaced by `design list`
(`spektacular:cmd/design.go:284-286`). The two writes that keep a spec and a design in agreement
are made to fail as a unit by compensation in `applyRef` (`cmd/design_ref.go:288-319`), and the one
outcome that can leave them disagreeing already reports `design_ref_backlink_rollback_failed`. This
project currently holds **no design documents at all** (`design list` returns an empty set), so all
design fixtures in this plan are synthetic.

**The three audit findings reproduce live.**

1. *Category descriptions leak into both retrieval surfaces.* `knowledge search "purpose belongs
   elsewhere entry shape"` returns eight hits, every one a category `README.md`
   (`architecture`, `decisions`, `gotchas`, `learnings`, across both repo stores), because
   `Set.Search` excludes always-applied categories (`internal/knowledge/set.go:214-226`) but nothing
   excludes a description sitting in a looked-up one. Separately,
   `knowledge always-applied --tier repo --filter spektacular --filter docs` returns
   `conventions/README.md` and `glossary/README.md` from both stores, because `readCategories`
   (`set.go:493-519`) appends every file it walks. In the `spektacular` store the glossary holds
   nothing else, so its description is the only thing the glossary contributes to every task.
2. *Labels on always-applied entries are inert.* `Set.Search` skips always-applied categories
   (`set.go:221`) and `Set.Tags` omits them from the vocabulary (`set.go:412,423`). Both are
   deliberate and correct; the side effect is that
   `conventions/store-files-must-be-written-through-the-cli.md`
   (`tags: [storage, paths, cli, artifacts]`) and `conventions/tests-must-not-depend-on-order.md`
   (`tags: [testing, isolation, shuffle, cobra, flake]`) carry labels nothing can ever reach.
3. *Category descriptions drift between stores and nothing detects or repairs it.*
   `spektacular:.spektacular/knowledge/architecture/README.md` matches `knowledge.Categories` while
   `docs:.spektacular/knowledge/architecture/README.md` still carries the pre-000037 Purpose and
   Belongs-elsewhere text. `EnsureFootprint` (`spektacular:internal/repo/footprint.go:93-100`)
   writes a description only when it is absent, so no command repairs this today, while
   `internal/project/init.go:157-168` overwrites project-tier descriptions unconditionally.

**The knowledge skill has four intents.** `templates/skills/workflows/spek-knowledge/SKILL.md`
(129 lines) routes between lookup (`:25`), contribute (`:48`), update (`:84`) and audit (`:99`),
with decline handling at `:123`. The audit is explicitly tags-only. Its branch count is asserted as
a literal in `internal/agent/instruction_surface_test.go:443-461`, and the set of knowledge
subcommands a skill may invoke is hand-maintained at `:426-436`.

**Baseline.** `go test ./...` is green at `8520f82` on branch `f-migrate`.


## Per-Phase Technical Notes

### Phase 1.1: Pin the behaviour of the removals that already work

**Requirement-to-repo resolution:** none of the spec's requirements; this phase serves the
constraint "the behaviour of the three stores that can already delete must not change", carried out
entirely in `spektacular`.

**File changes**

- `cmd/storefile.go:262-273` — read only. The `del` arm of `newStoreFileCmd`, shared by
  `spec file delete`, `plan file delete` and `changelog file delete`. It calls `st.Delete` and
  emits no envelope. Note it resolves through `storeFileStore(dir)` rather than the `resolveStore`
  closure at `:177-182`, so `changelog file delete --repo <name>` ignores repo routing. **Record
  that asymmetry in the test's comment; do not fix it — it is outside this spec.**
- `cmd/file_test.go` (new tests, appended) — characterisation tests driving the real command tree.
  Model on `TestSpecFileRead_MissingFileNamesResourceInError` at `cmd/file_test.go:146-165` for the
  envelope-and-exit-code shape.
- Reuse the three-kind table `kindFixture` / `kindFixtures()` at
  `cmd/storefile_metadata_test.go:17-62`, which already carries `kind`, `configYAML`,
  `artifactName` and `storeRelPath` per row, so all three verbs are covered from one table.
- Helpers, all in `cmd/root_test.go`: `resetRootCmd` (`:35`), `runRootCmd` (`:70`),
  `writeSpecFileFixture` (`:112`), `writeCurrentConfig` (`:93`). `writeSpecCommandConfig`
  (`cmd/spec_test.go:26`) is the general project scaffolder.
- Assert, per kind: a stored document removed by name is gone from disk and from `file list`; a
  second identical removal exits 0 and changes nothing; the current stdout is recorded as-is so a
  later change to it is a visible diff rather than a silent one.
- `internal/store/store_test.go:43,49,103,124` — read only; already covers `FileStore.Delete`
  including the absent-file and path-escape cases. Do not duplicate it here.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution. One test file, one table, no production
change.

### Phase 1.2: Name a category's description, and make a drifted one repairable

**Requirement-to-repo resolution:** supports requirement "Deleting a generated category description
is refused" (its recognition half and its remedy) and requirement "Category descriptions that have
drifted are reported" (its remedy half). All in `spektacular`.

**File changes**

- `internal/knowledge/category.go` — add beside `README()` (`:105-111`) and `AlwaysApplied()`
  (`:117-125`):
  - `const CategoryDescriptionFile = "README.md"`.
  - `func IsCategoryDescription(path string) bool` — true only when `path` splits into exactly two
    slash-separated segments, the first resolving through `CategoryByName` (`:129-136`) and the
    second equal to `CategoryDescriptionFile`. A deeper `README.md` returns false, by design.
  - Keep the doc comment in the register's idiom: state that three behaviours consult this, so
    renaming or re-tiering stays a single-field change.
- `internal/knowledge/category_test.go` (64 lines today, five tests, none covering `README()`) —
  add: `README()` renders every registry field for a representative category; `IsCategoryDescription`
  true for `conventions/README.md`, false for `conventions/sub/README.md`,
  `conventions/naming.md`, `README.md` at the store root, and `notacategory/README.md`.
- `internal/repo/footprint.go:93-100` — change the write guard. Today:
  `if _, err := os.Stat(readmePath); os.IsNotExist(err) { … WriteFile … }`. Replace with: read the
  file; write when it is absent **or** when its bytes differ from `c.README()`; leave it untouched
  when they match. Set `status = FootprintRepaired` only when a write actually happens, so a
  repeated run still reports `FootprintUnchanged`. Update the routine's doc comment at `:20-28`,
  which currently promises "existing knowledge files are never overwritten" — narrow it explicitly
  to exempt the generated category description and say why.
- `internal/repo/footprint_test.go:46-60` —
  `TestEnsureFootprint_MissingREADMERepairedWithoutRewritingOthers` pins the create-only-when-absent
  behaviour with a sentinel file. Extend rather than replace: keep its guarantee that a *non-README*
  file is never rewritten, and add that a README whose content differs is brought back into line
  while one that matches is left byte-identical (compare modification time or content, not both).
- `internal/project/init.go:157-168` — read only; already overwrites project-tier descriptions
  unconditionally, and `:170-195` already cascades `EnsureFootprint` over every registered repo, so
  `init` becomes the single remedy with no change here. Confirm with a test at the `cmd` level that
  running init over a project whose repo store holds a drifted description repairs it.
- **Known live instance to use as the worked case**:
  `spektacular-website/.spektacular/knowledge/architecture/README.md` carries pre-000037 Purpose and
  Belongs-elsewhere text while `spektacular/.spektacular/knowledge/architecture/README.md` matches
  the registry. Do not hand-edit either; the phase's own change is what repairs it.

**Complexity**: Medium
**Token estimate**: ~22k tokens
**Agent strategy**: 2 parallel agents — one on the registry statement and its unit tests, one on the
footprint guard and its tests — then sequential integration, since the footprint change consumes
nothing from the registry statement and the two touch disjoint files.

### Phase 1.3: Remove a knowledge entry

**Requirement-to-repo resolution:** requirements "A knowledge entry can be deleted", "Deleting a
document that is not there succeeds and changes nothing", "Deleting from a store or source the
project does not declare is refused" and "Deleting a generated category description is refused", all
in `spektacular`.

**File changes**

- `internal/knowledge/set.go` — add `func (s *Set) Delete(addr Address, path string) error`
  immediately after `Write` (`:355-361`). Body mirrors `Write` exactly: `s.resolve(addr)` (`:537`)
  then `src.store.Delete(path)`. Add no refusal: `resolve` already produces `ErrCodeStoreUnknown`
  and the tier/name validation, and `store.Writer.Delete` (`internal/store/store.go:104-105`) is
  contractually nil for an absent file.
- `internal/knowledge/set_test.go` — add beside the write tests at `:328-336` and `:890-930`:
  delete then `List`/`Search` no longer return the entry; delete of an absent path returns nil;
  delete with an incomplete address is refused with the same codes `Write` gives
  (`ErrCodeTierRequired`, `ErrCodeNameRequired`, `ErrCodeStoreUnknown`) and removes nothing. Add one
  test against a substitute `store.Store` implementation that is not `FileStore`, modelled on the
  `tagBlindStore` fake at `:1856-1884`, proving `Delete` travels through the interface.
- `cmd/knowledge.go`:
  - Add `knowledgeDeleteCmd` beside `knowledgeWriteCmd` (`:48-52`) and register it in the
    `AddCommand` call at `:615`.
  - `runKnowledgeDelete` modelled on `runKnowledgeWrite` (`:450-471`): schema branch first, then
    `knowledgeAddressData(cmd)`, then the category-description refusal, then `newKnowledgeSet()`,
    then `set.Delete`, then the envelope.
  - The refusal: when `knowledge.IsCategoryDescription(input.Path)`, return
    `output.NewError("knowledge_category_description_delete", …).WithResource(input.Path).WithNextAction(…)`.
    The message says the path is a category's generated description rather than a knowledge entry
    and that nothing restores it; the next action names `<command> init`, built from `cfg.Command`
    as `cmd/storefile.go:124` does, never a hard-coded binary name. Place this check in `cmd`, not
    in `internal/knowledge`: it is the layer holding both the registry and the name of the command
    that regenerates a descriptor, per `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`.
  - `deleted` is determined before the call — `set.Read` returning `ErrCodeEntryNotFound` means
    false — because `Delete` cannot distinguish. Keep that explicit in a comment; a read on the
    delete path costs nothing and is the only way to report the outcome honestly.
  - Add `knowledgeDeleteOutputSchema` beside `knowledgeWriteOutputSchema` (`:195-202`) with
    `tier`, `name`, `path`, `deleted`. Input reuses `knowledgeAddressInputSchema` (`:240-248`)
    unchanged.
  - Register `--data` on the new command in `init()` (`:605-607`), matching read and write.
  - **Corrective change:** `knowledgeAddressData` (`:566-582`) refuses a missing `--data` with a
    bare `fmt.Errorf` at `:569`. Replace with
    `output.NewError("knowledge_data_required", "--data is required").WithNextAction(…)` carrying an
    example payload and a pointer to `knowledge sources`, modelled on `designAddressData`
    (`cmd/design.go:171-184`). This changes the refusal shape of the existing `knowledge read` and
    `knowledge write` too, deliberately.
- `cmd/knowledge_test.go`:
  - Add a result mirror struct for the delete envelope beside `knowledgeAddressResult` (`:55-63`).
  - **Add `"delete"` to `knowledgeConfigLoadingCmds` (`:1979-1988`)**, or the new verb is silently
    exempt from the config-loading and untagged-base sweeps, including
    `TestKnowledge_EveryCommandRunsAgainstAnUntaggedKnowledgeBase` (`:2134-2140`).
  - **Do not** add it to `knowledgeNarrowingCmds` (`:87-93`): delete is addressed, not fan-out, and
    takes no `--tier`/`--filter`.
  - Tests: round-trip write-then-delete-then-list against `twoScopeProject` (`:105-136`); delete
    twice; unknown store name refused naming the tier's stores, modelled on
    `TestKnowledgeAlwaysApplied_UnknownFilterNamesTheRegisteredStores` (`:1103-1118`); category
    description refused with the file still present and `init` named in the next action; schema
    published, modelled on `TestKnowledgeRead_SchemaDocumentsInputAndOutput` (`:435`).
  - Assert the **content** of every `NextAction`, never merely that it is non-empty.
- `cmd/no_project_test.go:42-78` — add a `t.Run` for `knowledge delete`, asserting the `no_project`
  refusal and that no `.spektacular` directory is created, beside the existing `knowledge sources`
  and `design read` blocks.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents — one on `internal/knowledge` (method plus unit tests), one on
`cmd` (command, refusals, schema, command tests) — with sequential integration, since the command
depends on the method's signature but nothing else.

### Phase 1.4: Remove a design document

**Requirement-to-repo resolution:** requirements "A design document can be deleted", "A design
nothing references deletes cleanly", "Deleting a document that is not there succeeds and changes
nothing" and "Deleting from a store or source the project does not declare is refused", all in
`spektacular`.

**File changes**

- `internal/design/design.go` — add `func (s *Set) Delete(d Document) error` after `Write`
  (`:252-264`), following its sequence exactly: `s.lookup(d.Source)` (`:150`), then the
  `d.Path == ""` check returning `incompleteAddress("path", s.names())`, then the
  `src.writer == nil` check returning `readOnlySource(src, d.Path)`
  (`internal/design/errors.go:92-98`), then `src.writer.Delete(d.Path)`. No new error constructor
  is needed in `errors.go`.
- `internal/design/design_test.go` — add beside `TestSet_WriteToSourceWithNilWriterRefusesAsReadOnly`
  (`:384-404`) and `TestSet_WriteThenReadRoundTripsBytesUnchanged` (`:406-431`): delete removes the
  document and it disappears from `List`; delete of an absent path returns nil; a nil-writer source
  refuses by name; an unknown source and an empty path are refused with the existing codes. Include
  one test against a non-`FileStore` writer to prove the call routes through `store.Writer`.
- `cmd/design.go`:
  - Add `designDeleteCmd` beside `designAuthorCmd` (`:62-66`) and register it in the `AddCommand`
    call at `:511`. Keep the file's opening comment (`:14-24`) accurate: removal is another reason
    these commands are hand-written, not a reason to reconsider the factory.
  - `runDesignDelete` modelled on `runDesignWrite` (`:321-391`) minus the `--from` handling: schema
    branch, `designAddressData(cmd)` (`:171`), `newDesignSet()` (`:215`), `set.Exists(doc)` (`:282`)
    to establish `deleted`, the referenced-design refusal (phase 1.5), `set.Delete`, then
    `set.Resolve(doc)` (`:270`) for `location` and the envelope.
  - Add `designDeleteOutputSchema` beside `designWriteOutputSchema` (`:138-145`) with `source`,
    `path`, `location`, `deleted`. Input reuses `designAddressInputSchema` (`:87-94`).
  - Register `--data` on the new command in `init()` (`:499-509`).
- `cmd/design_test.go`:
  - `TestDesignSchema_PublishesDocumentedShapes` (`:747-861`) — add a `t.Run("delete")` block with
    the hand-listed field names.
  - `TestDesignRefusals_CarryCodeAndNextAction` (`:862+`) — add rows for `design_data_required`,
    `design_source_unknown` and `design_address_incomplete` on the delete verb.
  - New tests against `twoSourceDesignProject` (`:141-159`), which already gives a relative source
    and an absolute one outside the project: delete then `design list` no longer reports it; delete
    twice; an unauthored document (use the `designUnauthoredDocs` table at `:481`) deletes with no
    special condition; the sibling documents seeded by the fixture are untouched, modelled on
    `TestDesignWrite_LeavesEveryOtherFileUntouched` (`:433`).
- `cmd/no_project_test.go:42-78` — add a `t.Run` for `design delete`.

**Complexity**: Medium
**Token estimate**: ~28k tokens
**Agent strategy**: 2 parallel agents — one on `internal/design`, one on `cmd` — sequential
integration. Mirrors phase 1.3 deliberately; the two phases can also run in parallel with each
other, since they share no file.

### Phase 1.5: Refuse removing a design a spec still references

**Requirement-to-repo resolution:** requirements "Deleting a design a spec still references is
refused" and the constraints "a design document and the specs referencing it must never be left
disagreeing" and "deleting a referenced design must not modify any spec", all in `spektacular`.

**File changes**

- `cmd/design.go`, inside `runDesignDelete` — before calling `set.Delete`, and only when
  `set.Exists(doc)` reported true: read the document with `set.Read(doc)` and pass the bytes to
  `authoredMetadata` (`:204-210`). When it returns non-nil and `fm.Specs` is non-empty, refuse. Use
  `authoredMetadata`, not `metadata.Split`, precisely because it swallows a parse error: a design
  carrying the team's own YAML header must read as unauthored rather than blowing up, which is the
  behaviour `:194-203` documents.
- The refusal: `output.NewError("design_referenced_delete", …)`. Message names the resolved location
  and every spec in `fm.Specs`. `WithResource` carries the resolved location, as
  `design_authored_overwrite` does at `:369-375`. `WithNextAction` gives one
  `design ref remove --data '{"spec":"<spec>","source":"<source>","path":"<path>"}'` per spec
  followed by the same delete to retry. Follow `:375`'s quoting: the payload is written with
  double quotes inside a single-quoted next action so it is copy-pasteable, and the whole thing is
  built with `%q` verbs rather than manual escaping.
- Ordering matters and must be asserted: the refusal returns **before** `set.Delete` is reached, so
  nothing is removed, and it performs no write of any kind, so no spec is touched. There is no
  compensating-rollback problem here precisely because there is only ever one write, which is the
  argument against cascading recorded in `research.md`.
- A design with `fm == nil` (the project's own file, no lifecycle block) or with an empty
  `fm.Specs` falls straight through to the delete. Cite `cmd/design_ref.go:229-258`
  (`writeBackLink`) in a comment as the place `Specs` is maintained, so a reader can see why this is
  a read rather than a scan.
- `cmd/design_ref_test.go` — add the end-to-end sequence beside the existing back-link tests
  (`TestDesignRef_BackLinksAgreeWithTheSpecsThatReferenceThem` at `:748`,
  `TestDesignRef_TwoSpecsShareOneDesignIndependently` at `:447`). Use `designRefProject` (`:172`),
  `writeSpecFixture` (`:189`), `seedAuthoredDesign` (`:101`), `requireAuthoredDesign` (`:110`),
  `specsListedBy` (`:124`) and `designsReferencedBy` (`:144`).
  Assert: two specs reference one design; delete is refused with `design_referenced_delete`; the
  document's bytes are unchanged (compare the full body, not just presence); both specs'
  `designs` lists are unchanged; the next action names **both** spec names and gives a runnable
  removal for each; after `design ref remove` for both, the same delete succeeds; and
  `design ref list` for each spec afterwards reports `unresolved: 0`.
- Also assert the negative: a design seeded with no lifecycle block, and one seeded with a lifecycle
  block whose `specs` list is empty, both delete without the refusal.

**Complexity**: Medium
**Token estimate**: ~24k tokens
**Agent strategy**: Single agent, sequential execution. The refusal and its tests are one tightly
coupled unit and splitting them risks a test written against an imagined message.

### Phase 2.1: Keep category descriptions out of retrieval

**Requirement-to-repo resolution:** requirement "Generated category descriptions stay out of
retrieval"; constraint "the existing exclusion of always-applied categories from search must not be
weakened". All in `spektacular`.

**File changes**

- `internal/knowledge/set.go:214-226` — inside `Set.Search`'s post-merge loop, beside the existing
  `if alwaysApplied[hit.Category] { continue }` at `:221`, add
  `if IsCategoryDescription(hit.Path) { continue }`. Keep the existing comment block and extend it:
  both exclusions are registry-driven and this is the single place the search surface applies them.
  It must sit **before** the cutoff floor is computed at `:168`-equivalent (the `eligible[0].Score`
  line) for the same reason the always-applied exclusion does: a description scoring highest would
  otherwise set the bar and then be dropped, silently raising the threshold for everything else.
- `internal/knowledge/set.go:493-519` — inside `readCategories`'s file loop (the `for _, f := range
  files` at `:511`), skip a path for which `IsCategoryDescription(f)` is true, before the
  `src.store.Read(f)`. This covers `AlwaysAppliedEntries` (`:462`) and `Conventions` (`:472`)
  together, since both route through here.
- **Do not** touch `listFiles` (`:617-639`), `Set.List` (`:366`) or `Set.Tags` (`:408`). `List` must
  keep returning descriptions for the phase 3.1 drift report, and `Tags` already skips
  always-applied categories while a looked-up description carries no tags to contribute.
- **Do not** touch `internal/store/search.go`. Its doc comment at `:44-48` states the store is
  category-agnostic and that exclusions live in the knowledge layer; that stays true.
- `internal/knowledge/set_test.go`:
  - `TestSet_AlwaysAppliedEntriesReturnsAllAlwaysAppliedCategories` (`:551-570`) — seed a
    `conventions/README.md` into the fixture and assert the `ElementsMatch` literal is **unchanged**,
    i.e. the description contributes no entry. This is the most direct regression guard for the
    always-applied half.
  - `TestSet_SearchExcludesAlwaysAppliedCategories` (`:476-494`) — add a sibling asserting a
    looked-up category's `README.md` is absent from results while a real entry in the same category
    is present.
  - Add: a `README.md` deeper inside a category (`gotchas/sub/README.md`) is still returned by
    search; `Set.List` still returns every description; `Set.Tags` output is unchanged.
  - `TestRetier_FlipsLoadAndSearchExclusionTogether` (`:500-549`) mutates the `Categories` registry
    in place and restores it in a `defer`. Any new registry-driven test must do the same, because
    `make test` runs `-shuffle=on`.
  - `TestSet_SelectorCoverageMatrixIsUniformAcrossRetrievalPaths` (`:734`) — confirm the new
    exclusion does not break selector uniformity across retrieval paths.
- `cmd/knowledge_test.go:617-640` — `alwaysAppliedProject` seeds no `README.md`, so it cannot catch
  a leak. Seed one there (or add a sibling fixture) and assert the command-level always-applied
  output excludes it.

**Complexity**: Medium
**Token estimate**: ~24k tokens
**Agent strategy**: Single agent, sequential execution. Two one-line production changes with a wide
blast radius across existing exclusion tests; splitting it would have two agents editing the same
test file.

### Phase 2.2: Report labels a search will never reach

**Requirement-to-repo resolution:** requirement "Labels that cannot be retrieved are not silently
accepted", in `spektacular`.

**File changes**

- `internal/knowledge/category.go` (or a small sibling file in the same package) — add:
  ```go
  type TagReachability struct {
      Category    string   `json:"category"`
      Unreachable []string `json:"unreachable_tags"`
      NextAction  string   `json:"next_action"`
  }
  func UnreachableTags(path string, content []byte) *TagReachability
  ```
  Implementation: `categoryOf(path)` (`internal/knowledge/set.go:314`) for the category;
  return nil unless that category is in `AlwaysApplied()` (`category.go:117`); parse the entry's
  labels with `store.ParseEntry(content)` (`internal/store/frontmatter.go:51-76`), which returns
  nil labels and the raw body for a malformed or absent block rather than erroring; return nil when
  there are none. `NextAction` names the category, states that always-applied categories are
  excluded from search and from the label vocabulary by design, and offers the two real options:
  move the entry to a looked-up category, or drop the labels. Build the looked-up category list from
  the registry rather than hard-coding names.
- It is a package function, not a method on `Set`: it consults no store, and keeping it callable
  without one is what lets the command layer ask before or after the write without ordering
  constraints.
- `cmd/knowledge.go:450-471` — in `runKnowledgeWrite`, after `set.Write` succeeds, call
  `knowledge.UnreachableTags(input.Path, content)` and, when non-nil, add `unreachable_tags` and
  `next_action` to the result map. Absent in the normal case, so no existing consumer changes.
  Extend `knowledgeWriteOutputSchema` (`:195-202`) with both as optional fields.
- Report after the write, not before: the requirement is to tell the caller, not to refuse, and
  reporting after makes it structurally impossible for this to become a gate.
- `internal/knowledge/` tests — `UnreachableTags` returns the labels for `conventions/x.md` and
  `glossary/x.md`; nil for `gotchas/x.md` with the same labels; nil for an entry with no frontmatter
  block; nil for a malformed block; nil for a path with no category segment. Assert the next
  action's **content** names the category and both options.
- `cmd/knowledge_test.go` — the write envelope carries the two fields for an always-applied
  destination and omits them otherwise; the entry is written either way, byte for byte, and is
  readable back. Reuse `seedKnowledgeFile` (`:716-722`) for independent verification of what landed
  on disk.
- **Real instances to use as fixtures' inspiration, not to modify:**
  `.spektacular/knowledge/conventions/store-files-must-be-written-through-the-cli.md` carries
  `tags: [storage, paths, cli, artifacts]` and
  `.spektacular/knowledge/conventions/tests-must-not-depend-on-order.md` carries
  `tags: [testing, isolation, shuffle, cobra, flake]`; both are unreachable today. Fixtures are
  synthetic; do not edit these entries in this phase.

**Complexity**: Low
**Token estimate**: ~18k tokens
**Agent strategy**: Single agent, sequential execution. One small pure function plus its rendering.

### Phase 3.1: Review a knowledge base for truth, not just labels

**Requirement-to-repo resolution:** requirements "An agent can judge whether a knowledge entry is
still true", "A maintenance review states the evidence for every claim it makes", "A maintenance
review distinguishes an unmet target from an obsolete entry", "Agreement to one entry's outcome
never applies to another" and "Category descriptions that have drifted are reported" (its reporting
half). All in `spektacular`.

**File changes**

- `templates/skills/workflows/spek-knowledge/SKILL.md` (129 lines) — add `# Intent: maintenance`
  between `# Intent: audit` (`:99-121`) and `# Decline handling` (`:123-129`). Model its structure
  on the audit intent, which already states read-and-propose-only, per-entry confirmation, and
  "adds no new command, no bulk operation, and no second write path". Maintenance differs on the
  last: it composes the existing primitives **plus** the new `{{command}} knowledge delete`, and
  must say so rather than repeat audit's claim verbatim.
  The section must carry, as literal prose an assertion can pin:
  - the four verdicts `current`, `stale`, `incorrect`, `unverifiable`;
  - the classification rule, stated as a rule: an entry stating a standard the code has not yet met
    is `current`, because an entry states the target and the code is what has yet to meet it; only
    an entry whose subject no longer exists is `stale`. Include the worked failure mode, mirroring
    how the audit intent carries its two failure modes at `:109-112`;
  - the evidence requirement: every `stale` or `incorrect` verdict names the specific file, command
    or behaviour that changed; "looks old" is not a finding;
  - the category-description drift step: enumerate with `{{command}} knowledge list`, read each
    description with `{{command}} knowledge read`, compare against `{{command}} knowledge
    categories`, and report a mismatch naming the store, the path, and `{{command}} init` as the
    remedy. Say explicitly that a drifted description is reported and **not** repaired by the
    review;
  - per-entry propose-then-confirm, with removal via `{{command}} knowledge delete` only after
    explicit agreement **for that entry**.
- Same file, `:10` — the preamble currently reads "picks one of four branches (lookup / contribute /
  update / audit)". It becomes five, naming maintenance. `:23` — "One skill handles all four
  intents" becomes five. `:12-22` — add a natural-language trigger for maintenance, e.g. asking
  whether the knowledge base is still accurate or still true.
- Same file, `:123-129` — `# Decline handling` currently carves out per-entry behaviour "in the
  audit intent". Extend to name maintenance too, and state that a decline there leaves the entry
  untouched and never removes anything.
- `internal/agent/instruction_surface_test.go` — the primary gate. Required edits:
  - `knowledgeSubcommands` (`:426-436`) — **add `"delete": true`**, or every subcommand scan
    rejects the legitimate `knowledge delete` invocation in the new section. The comment at
    `:421-425` explains it is hand-maintained to avoid an import cycle; keep it in step.
  - `TestRenderedSpekKnowledgeAdvertisesAuditIntent` (`:443-461`) — update the branch-count literals
    at `:448` and `:450`, and add `"one of four branches"` and `"all four intents"` to the stale-phrase
    ban list at `:457`.
  - `expectedCRUDInvocations` (`:97-104`) — add `knowledge delete`.
  - Add a `spekKnowledgeMaintenanceSection` helper mirroring `spekKnowledgeAuditSection`
    (`:407-419`), keyed on `# Intent: maintenance`. Confirm the existing helper still returns the
    audit section intact now that another `\n# ` heading follows it — it slices to the next `\n# `,
    so it does, but assert it.
  - Add tests mirroring the audit family: the intent is present and advertised; it carries all four
    verdicts; it carries the classification rule and its worked failure mode; it carries the
    evidence requirement; it carries the drift step naming list, read, categories and init; it
    confirms per entry and its decline stops one entry only; it composes only registered
    subcommands.
- `.claude/skills/spek-knowledge/SKILL.md` — regenerate with `go run . migrate` and commit the
  result alongside the template edit. The rendered copy is what this repo dogfoods;
  `internal/agent/skills.go:44-69` is the renderer, substituting `{{command}}` from `cfg.Command`.
- Swept automatically, so the new prose must satisfy them: `templates/data_payload_wellformed_test.go:40`
  (every `--data '{…}'` example balances braces and closes its quote — the delete example must);
  `cmd/instruction_contract_test.go:307-326` (no surviving `{{` in the rendered corpus);
  `cmd/instruction_contract_test.go:417` (any `context.md` mention must be qualified — avoid
  mentioning it at all); `internal/agent/instruction_surface_test.go:45,67` (the forbidden
  stdin/heredoc substrings at `:32-40` — the new prose must use `--file`, which is what
  `knowledge write` actually takes, and must not describe piping a body).

**Complexity**: High
**Token estimate**: ~40k tokens
**Agent strategy**: Parallel analysis, sequential integration. One agent drafts the intent prose
against the audit section as its model; one agent inventories every assertion in
`internal/agent/instruction_surface_test.go` that the branch count or subcommand set touches. Then a
single agent integrates, because the prose and its phrase assertions must be written against each
other and cannot be reconciled after the fact.

### Phase 3.2: Make the standing rule name removal

**Requirement-to-repo resolution:** constraint "removal must be reachable only through the CLI; no
instruction, skill or workflow may tell an agent to delete a managed file with its own file tools",
in `spektacular`.

**File changes**

- `templates/agents/store-access.md:11-15` — the paragraph naming `{{command}} spec file`,
  `{{command}} plan file`, `{{command}} changelog file`, `{{command}} knowledge` and
  `{{command}} design`, and stating that a write supplies its body with `--from <path>`. Add that
  removal is likewise a CLI verb, naming `{{command}} knowledge delete` and
  `{{command}} design delete`, and that removing a managed file with `rm` or an equivalent is never
  correct. Keep `:31-37`'s three exceptions exactly as they are.
- `internal/agent/store_access_test.go:94-108` —
  `TestRenderedStoreAccessSectionNamesEveryStoreCommand` carries a hand-maintained needle list
  including `go run . knowledge` and `go run . design`; add needles for the two removal verbs.
  `:114-125` covers the three exceptions; assert they are unchanged.
- Add a corpus-wide negative assertion in `internal/agent/instruction_surface_test.go`, alongside
  `forbiddenInstructionSubstrings` (`:32-40`) and the two sweeps at `:45` and `:67`: no template or
  rendered skill instructs removal of a managed file with a raw file operation. Keep the banned
  literals distinctive — a bare `rm ` would false-positive on the legitimate
  `rm .spektacular/tmp/<slug>.md` lines the contribute, update and audit intents already carry
  (`SKILL.md:82,97,119`). Pin the scratch-file exception explicitly rather than by luck.
- `AGENTS.md` at the repo root — regenerated by `go run . init`/`migrate` from the template; commit
  the regenerated section with the template change. `internal/agent/store_access.go` is the
  installer and needs no change; `internal/agent/store_access_test.go:...` already covers
  idempotency and surrounding-content preservation for this section family.
- The knowledge entry `conventions/store-files-must-be-written-through-the-cli.md` in this repo's own
  store states the same rule and is now incomplete. **Do not edit it in this phase.** Propose it to
  the user through the knowledge skill after the walkthrough; a knowledge write needs explicit
  agreement and is not a plan-authorised change.

**Complexity**: Low
**Token estimate**: ~16k tokens
**Agent strategy**: Single agent, sequential execution.

### Phase 4.1: Document removing a knowledge entry

**Requirement-to-repo resolution:** requirement "The documentation site explains how to remove an
entry", knowledge half, in `docs`.

**File changes**

- `docs:src/pages/knowledge-base.mdx` — the `The lifecycle of an entry` section opens at `:94`; its
  `sub` slot at `:96-102` currently frames the lifecycle as "create one, search and read it back,
  and rewrite it when things change" and must be extended to mention removal. The body runs
  **Creating** (`:106`), **Searching and retrieving** (`:127`), **Keeping it up to date**
  (`:147-153`). Insert a **Removing an entry** paragraph after `:153`, before the closing `</Prose>`
  at `:154`.
- Follow the page's own conventions, which differ from the design page's: a bolded lead-in phrase
  then a full sentence; a prose lead ending in a colon, a blank line, then a fenced block; JSON
  payloads written with `", "` spacing as at `:112`; British spelling; no em dashes; ~78 column
  wrap. Refusals on this page are documented in prose, not as JSON blocks — the precedent is
  `:421-426`. Follow it.
- No new component and no new import: the page already imports `Hero`, `Section`, `Prose`,
  `CtaBanner` and `Button`. No `docs:src/components/Nav.astro` change; the page is registered at
  `:8`. Adding a paragraph inside an existing `Section` needs no `surface` alternation change.

**Content example** (illustrative wording; the command shape, flag and refusal behaviour are fixed
by this plan's research, the prose is not):

```mdx
  **Removing.** An entry is removed with `knowledge delete`, addressed exactly as a read or a
  write is: the tier, the store, and the path within that store.

  ```bash
  spektacular knowledge delete \
    --data '{"tier": "repo", "name": "docs", "path": "gotchas/db-timeouts.md"}'
  ```

  Naming a store the project has not declared is refused, and the refusal lists the stores
  available in that tier. Naming a path that simply holds nothing is not an error: the removal
  reports success and changes nothing, so a maintenance pass that retries is safe. The one entry
  that cannot be removed is a category's own `README.md`. It is generated from the project's
  definition of that category rather than written by anyone, so removing it would leave a gap
  nothing fills; the refusal points at `spektacular init`, which regenerates it.
```

**Complexity**: Low
**Token estimate**: ~14k tokens
**Agent strategy**: Single agent, sequential execution. Verify with `npm run build` and `make check`
in the `docs` root.

### Phase 4.2: Document removing a design document

**Requirement-to-repo resolution:** requirement "The documentation site explains how to remove an
entry", design half, including that a referenced design is refused and that removing a design
differs from removing a reference. In `docs`.

**File changes**

- `docs:src/pages/design-documents.mdx` — the `Working with designs from the command line` section
  opens at `:259`. Its verb order is `design sources` (`:272`), `list` (`:280`), `read` (`:288`),
  `write` (`:295`), `author` (`:303`), the write-versus-author prose (`:308-311`), then the
  reference verbs (`:313-319`) and `design ref list` with its JSON response (`:321-348`). Insert the
  removal after the author prose at `:311` and before the "Record a reference" paragraph at `:313`,
  so document-level verbs stay together, and carry the delete-versus-reference distinction as the
  bridge into the reference verbs.
- Insert the refusal example in `When a reference cannot be found` (`:354`), which already holds the
  page's only JSON error examples (`:375-383`); copy that block's shape exactly — `error`, `code`,
  `message`, `resource`, `next_action`.
- `:385-391` already carries the transactional prose about recording or removing a reference being a
  two-document change. That paragraph is the anchor for sharpening the distinction; extend rather
  than duplicate it.
- Page conventions differ from the knowledge page: compact JSON payloads with **no** space after the
  colon (`:288`, `:295`), `--from` rather than `--file`, fences indented two spaces inside
  `<Prose nested>`. Match this page, not the other.
- No new component, no new import, no `Nav.astro` change (the page is registered at `:16`).
- `docs:src/pages/extending.mdx:38-39,94-95` already states that the provider-level `Delete` is
  idempotent. The new prose must not contradict it.

**Content example** (illustrative wording; the command shape, the error code and the refusal's
content are fixed by this plan's research):

```mdx
  Remove one from a declared source. A document nothing references is removed outright, and
  removing one that is already gone reports success rather than an error:

  ```bash
  spektacular design delete --data '{"source":"api","path":"payments/v2.md"}'
  ```

  A design that a spec still references is not removed. The refusal names every spec that
  references it and gives you the step to clear each one, and neither the document nor any spec
  is changed by the attempt, so a spec can never be left pointing at a design that is not there.

  Removing a design and removing a *reference* to one are different things, and it is worth being
  clear which you want. `design ref remove` drops the pointer a spec carries and deliberately
  leaves the document alone. `design delete` removes the document and refuses while any pointer
  remains. Clearing the references first, then deleting, is the order the refusal walks you
  through.
```

…and in `When a reference cannot be found`:

```mdx
  ```json
  {
    "error": true,
    "code": "design_referenced_delete",
    "message": "…names the resolved location and every referencing spec…",
    "resource": "/work/design/api/payments/v2.md",
    "next_action": "…one 'design ref remove' per spec, then retry the delete…"
  }
  ```
```

**Complexity**: Low
**Token estimate**: ~16k tokens
**Agent strategy**: Single agent, sequential execution. Verify with `npm run build` and `make check`
in the `docs` root.

### Phase 4.3: Keep the repository's own command references in step

**Requirement-to-repo resolution:** no spec requirement; this keeps the repository's own reference
documents from going stale, matching plan 000054's phase 4.3. In `spektacular`.

**File changes**

- `README.md:138-144` — the bulleted `spektacular knowledge` subcommand list. Add a
  `knowledge delete` bullet beside the `knowledge read` / `knowledge write` line at `:142`, in the
  same one-line form with its `--data` shape.
- `docs/knowledge-base.md:387-397` — the command-reference table. Add a row for
  `spektacular knowledge delete --data '{"tier":"repo","name":"docs","path":"gotchas/x.md"}'`,
  matching the neighbouring rows' voice.
- `docs/knowledge-base.md:351-360` — the `Auditing the tags on existing entries` subsection
  describing what the knowledge skill does. Add a short paragraph naming the maintenance review as
  the sibling that judges whether an entry is still true rather than whether it is well labelled,
  and pointing at the category-description drift report.
- Neither document enumerates the design commands anywhere (verified by grep), so neither gains
  `design delete`. Do not invent a design section to hold it.
- Constraints from `cmd/docs_test.go`, which reads both files from the repo root: no `scope:` key
  and no `--data` example keyed on `"scope"` (`:198-244`, `:308-328`); none of the banned
  precedence claims at `:255-261`. A tier/name/path payload satisfies all of them.
- `CHANGELOG.md` — out of scope for this phase. Changelog records are produced by the implement
  workflow, not by a documentation phase.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.


## Testing Strategy

Testing is planned per phase rather than as a trailing pass, because two of the three test layers in
this project are coupled to the surface they cover and drift silently when they are not moved
together. The overall strategy is in `plan.md#testing-approach`; what follows is where each phase's
coverage lands and which hand-maintained oracle it must move.

- **Phase 1.1** — `cmd/file_test.go`, new characterisation tests over the three-kind table at
  `cmd/storefile_metadata_test.go:17-62`. Pure test change. Establishes the baseline the spec's
  "their behaviour must not change" constraint is measured against.
- **Phase 1.2** — `internal/knowledge/category_test.go` (currently five tests, none covering
  `README()`): the renderer and the new predicate, including the negative cases. 
  `internal/repo/footprint_test.go:46-60` is extended rather than replaced, keeping its guarantee
  that a non-description file is never rewritten.
- **Phase 1.3** — `internal/knowledge/set_test.go` beside the write tests at `:328-336` and
  `:890-930`, plus a non-`FileStore` substitute modelled on `tagBlindStore` (`:1856-1884`).
  `cmd/knowledge_test.go` for the command: **`"delete"` must join `knowledgeConfigLoadingCmds`
  (`:1979-1988`)** or the verb is exempt from the config-loading and untagged-base sweeps, and it
  must **not** join `knowledgeNarrowingCmds` (`:87-93`). A `t.Run` joins
  `cmd/no_project_test.go:42-78`.
- **Phase 1.4** — `internal/design/design_test.go` beside `:384-431`, and `cmd/design_test.go`:
  a new `t.Run("delete")` in `TestDesignSchema_PublishesDocumentedShapes` (`:747-861`) and new rows
  in `TestDesignRefusals_CarryCodeAndNextAction` (`:862+`). A `t.Run` joins `cmd/no_project_test.go`.
- **Phase 1.5** — `cmd/design_ref_test.go`, beside the back-link tests at `:447` and `:748`, using
  `designRefProject` (`:172`), `seedAuthoredDesign` (`:101`) and `designsReferencedBy` (`:144`).
  The load-bearing assertions are that the refusal names **every** referencing spec, that the
  document's full body is unchanged, that no spec's reference list changed, and that
  clear-then-retry succeeds with reference resolution afterwards reporting nothing unresolved.
- **Phase 2.1** — the tightest coupling in the plan.
  `TestSet_AlwaysAppliedEntriesReturnsAllAlwaysAppliedCategories`
  (`internal/knowledge/set_test.go:551-570`) asserts an exact `ElementsMatch` of three literal
  entries; seed a description into its fixture and assert that literal is **unchanged**.
  `TestSet_SearchExcludesAlwaysAppliedCategories` (`:476-494`) gains a looked-up-category sibling.
  `TestSet_SelectorCoverageMatrixIsUniformAcrossRetrievalPaths` (`:734`) asserts which stores each
  path reached rather than entry counts, and its fixture seeds no descriptions, so it is unaffected
  — confirm rather than assume. `TestRetier_FlipsLoadAndSearchExclusionTogether` (`:500-549`)
  mutates the registry in place and restores it in a `defer`; any new registry-driven test must do
  the same, because `make test` runs `-shuffle=on`.
  `cmd/knowledge_test.go:617-640`'s `alwaysAppliedProject` seeds no description and must gain one.
- **Phase 2.2** — unit tests in `internal/knowledge` for the reachability function across
  always-applied and looked-up destinations, no frontmatter, malformed frontmatter and a path with
  no category segment; `cmd/knowledge_test.go` for the two optional envelope fields and for the
  entry still landing byte for byte, verified with `seedKnowledgeFile` (`:716-722`) rather than
  through the code under test.
- **Phase 3.1** — `internal/agent/instruction_surface_test.go` is the gate. Required edits:
  `"delete": true` into `knowledgeSubcommands` (`:426-436`); the branch-count literals at `:448`
  and `:450`; `"one of four branches"` and `"all four intents"` onto the stale-phrase ban list at
  `:457`; `knowledge delete` into `expectedCRUDInvocations` (`:97-104`); a
  `spekKnowledgeMaintenanceSection` helper mirroring `:407-419`. New assertions mirror the audit
  family at `:443, :468, :519, :561, :596`. Swept automatically and therefore constraining the new
  prose: `templates/data_payload_wellformed_test.go:40`,
  `cmd/instruction_contract_test.go:307-326` and `:417`, and the forbidden-substring sweeps at
  `internal/agent/instruction_surface_test.go:45,67` with their literals at `:32-40`.
- **Phase 3.2** — `internal/agent/store_access_test.go:94-125` gains needles for the two removal
  verbs while its three write-it-yourself exceptions are asserted unchanged. A new corpus-wide
  negative assertion joins the sweeps, with literals distinctive enough not to flag the legitimate
  `rm .spektacular/tmp/<slug>.md` lines the contribute, update and audit intents already carry
  (`SKILL.md:82,97,119`).
- **Phases 4.1 and 4.2** — verified by `npm run build` and `make check` in the `docs` root. No
  phrase assertions, deliberately: the site's prose has always been verified this way, and an oracle
  over it is maintenance cost without a failure mode the build does not already catch.
- **Phase 4.3** — covered by the existing sweeps in `cmd/docs_test.go`, which read `README.md` and
  `docs/knowledge-base.md` from the repo root and ban a superseded `scope:` key (`:198-244`,
  `:308-328`) and a list of precedence claims (`:255-261`). A tier/name/path payload satisfies all
  of them.

**Every refusal is asserted on the content of its next action, never on its non-emptiness.** That is
the repository's own recorded gotcha: a non-emptiness assertion passes on exactly the message the
error convention exists to prevent.

**The agent-driven end-to-end suites are not run for this plan.** They were examined: only the
plan-workflow suite names a knowledge or design command, at
`tests/harbor/plan-workflow/tests/test_plan_workflow.py:114` (`knowledge always-applied`) and
`:130` (`design ref list`), and both pin step-instruction text this plan does not change. Recorded
as a checked finding rather than an omission.


## Project References

- **Spec** — `000056_store_delete_and_knowledge_maintenance`, read through
  `go run . spec file read`. The source of truth for scope, requirements, constraints and success
  metrics.
- **Design documents this plan was built on** — none. `go run . design ref list --data
  '{"spec":"000056_store_delete_and_knowledge_maintenance"}'` reports an empty list and zero
  unresolved references.
- **Registered repos** — `spektacular` at `/home/nicj/code/github.com/jumppad-labs/spektacular`
  (role `tool`, Go CLI) and `docs` at
  `/home/nicj/code/github.com/jumppad-labs/spektacular-website` (role `documentation`, Astro 5).
  Resolve both through `go run . repo list` rather than assuming a working directory.
- **Binding knowledge entries** — `conventions/error-messages-must-suggest-remediation.md`,
  `conventions/store-files-must-be-written-through-the-cli.md`,
  `conventions/tests-must-not-depend-on-order.md`,
  `conventions/tests-must-pass-for-done.md` (`spektacular`);
  `conventions/mdx-authoring.md`, `conventions/site-layout.md`, `conventions/no-em-dashes.md`,
  `conventions/plan-content-pages.md`, `conventions/file-scoped-section-headings.md` (`docs`).
  Also `architecture/testing-architecture.md`, `architecture/knowledge-search-ranking.md` and
  `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`, which constrain where checks live
  and which test surfaces move with a change.
- **Prior plans** — `000054_project-level-design-documents` (the design command family and the
  reader/writer split), `000055_design-authoring-skill` (the lifecycle record and the back-link
  list), `000050_knowledge-entry-tags` (labels and the always-applied exclusion),
  `000047_repo-scoped-knowledge-addressing` (tier-and-name addressing). Reach them with
  `go run . plan file read <name>/plan.md`.
- **GitHub issue #44** — where this work was first raised, carrying the evidence and the reasoning
  from the audit that prompted it.


## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per-phase estimates and strategies are stated in each phase's notes above. Three observations shape
them.

Phases 1.3 and 1.4 are deliberate mirrors of each other over disjoint files, so they may run in
parallel with each other as well as splitting internally between the domain package and the command
layer. Phase 3.1 is the only High phase and is the one where parallelism must stop before
integration: its prose and its phrase assertions are written against each other and cannot be
reconciled afterwards, so two agents may gather in parallel but one must integrate.

Phase 2.1 is nominally Medium but has an unusually wide blast radius across one test file; a single
agent avoids two agents editing `internal/knowledge/set_test.go` at once.

A sub-agent inherits the working directory, not the knowledge of which repo a phase targets. Every
phase's `**Repo:**` line names it, and each phase's root must be passed explicitly to any sub-agent
launched for it, resolved from `go run . repo list`.


## Migration Notes

No data migration. No configuration key is added or changed, no stored artifact changes shape, and
the settings format version does not rise, so no migration step is registered and no existing
project needs converting.

Two propagation steps are needed, neither of which is a migration in the schema sense:

- **The knowledge skill's rendered copy.** Phase 3.1 edits the template under `templates/`. The
  copy this repository dogfoods at `.claude/skills/spek-knowledge/SKILL.md` is produced by
  `internal/agent/skills.go:44-69` and regenerated by `go run . migrate`; regenerate it and commit
  it alongside the template edit. Note that command regenerates every workflow skill, so unrelated
  skills already out of step with their templates in the working tree will be brought into step at
  the same time.
- **The managed agent-guidance section.** Phase 3.2 edits `templates/agents/store-access.md`; the
  rendered section in the repository's own `AGENTS.md` is regenerated by the same command and must
  be committed with it. Other projects pick the change up when they next run it.

**One existing behaviour is deliberately narrowed** and is worth calling out for anyone upgrading:
`EnsureFootprint` currently promises never to overwrite an existing knowledge file, and after phase
1.2 a category's generated description is exempt from that promise. A description that has been
hand-edited away from the project's definition of its category will be brought back into line the
next time the project is set up or a repo is added. This is intended: a description is generated
output rather than content, and until now a drifted one could be repaired by nothing at all.


## Performance Considerations

Nothing here is on a hot path, and no measurement is planned. Three points are worth recording so a
later reader does not have to re-derive them.

**The removal verbs each perform one extra read.** Both establish whether a document was present
before removing it, because the storage contract's `Delete` returns nil either way and cannot report
the outcome. That is one additional read on a path a human or agent invokes interactively, one
document at a time, and it is what lets the result say honestly whether anything was removed.

**The retrieval exclusion is strictly cheaper than what it replaces.** In search it is a predicate
on a hit already in hand, evaluated after the merge alongside the existing always-applied check; in
the always-applied reader it skips the file's read entirely, so the payload assembled on every task
gets both smaller and cheaper. The direct effect is that two descriptions stop being read and
injected into every request in every session.

**The maintenance review is bounded by how much an agent reads, not by anything the tool does.** It
reads each entry in scope once, exactly as the existing audit does, and adds no bulk or recursive
operation. Its cost scales with the store's size and is paid deliberately when someone runs a
review, never on an ordinary task.

