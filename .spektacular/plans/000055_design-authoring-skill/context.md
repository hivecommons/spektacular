---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Context: 000055_design-authoring-skill

## Current State Analysis

**Two repos are in play.** `spektacular` (root `/home/nicj/code/github.com/jumppad-labs/spektacular`)
carries the CLI, the skill and instruction templates, and every test surface. `docs` (root
`/home/nicj/code/github.com/jumppad-labs/spektacular-website`) carries the public documentation.
Always confirm with `go run . repo list` before opening a file; the directory a session starts in
is not necessarily either of them.

**What 000054 shipped, and what it deliberately did not.** Design sources are declared per project
under `design.sources` in `config.yaml` (`internal/config/config.go:133-148,283`) and resolved by
`internal/design.NewSet`. They are deliberately absent from `Config.storeDirs()`, because that list
rewrites a path on read and write and refuses any directory resolving outside the project root; a
design source must be able to point at a folder the team already keeps, anywhere on disk. The
reasoning is written up in `.spektacular/knowledge/gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md`
and must not be undone.

`internal/design` is the byte-level layer: `Set.Read`, `Set.Write`, `Set.List`, `Set.Resolve` and
`Set.Exists`, with six refusal codes in `internal/design/errors.go`, each carrying a resource and a
runnable next action. Its package doc (`internal/design/design.go:1-21`) states the contract this
plan narrows rather than breaks: "It never adds frontmatter, reformats content, or imposes a
structure on a design document, which is why this package exists instead of the shared artifact-file
machinery." That stays true of `internal/design` itself. Stamping happens one layer up, in `cmd`.

The command layer is hand-written against that package rather than going through `newStoreFileCmd`,
and the divergence is signposted at `cmd/design.go:13-23`: the store-file factory merges lifecycle
frontmatter into everything it writes and re-reads the block on every listing
(`cmd/storefile.go:228,312-321`), which is exactly what a design document must not get. The
reference verbs live in `cmd/design_ref.go` rather than `internal/design`, because a spec lives in
the spec store and `internal/design` must not learn where specs are kept (`cmd/design_ref.go:17-22`).

**The metadata block today.** `internal/metadata` is the single definition of the frontmatter block
every Spektacular-written document carries. The status vocabulary is four values, `draft`, `final`,
`superseded`, `archived` (`internal/metadata/metadata.go:22-31`), with `DocumentStatuses()` as the
one list flag help and error text derive from. `Metadata` (`:59-76`) already carries
`CreatedDate`, `DocumentStatus`, `ClosedDate`, provenance `Project`/`ProjectSource`/`Spec`/`Plan`,
and `Designs []DesignRef` (`:77-85`), the spec-to-design reference list. `Merge`
(`internal/metadata/merge.go:47-131`) stamps `CreatedDate` once, preserves it thereafter, stamps
`ClosedDate` exactly once on the first transition to a closed status, and carries `Designs` forward
unless explicitly replaced.

**The trap that shapes phase 1.1.** `yamlShape` (`internal/metadata/metadata.go:88-99`) is a closed
schema. `Render`'s own comment (`internal/metadata/frontmatter.go:56-58`) states that unknown keys
in an original block are not preserved. A field added to some but not all of `Metadata`,
`yamlShape`, `yamlInShape` and the marshal pair does not fail: it silently loses its value on the
second write. `Designs` is the worked precedent for doing it correctly, and
`internal/metadata/metadata_test.go:770` is the byte-exact guard proving the addition perturbed
nothing.

**The authored/pre-existing discriminator, verified not assumed.** Probed against the real parser
during planning:

| Input | `Split` result |
|---|---|
| A Spektacular block (`created_date`, `document_status`) | `(non-nil, nil)` |
| No leading `---` | `(nil, nil)` |
| The team's own frontmatter (`title:`, `author:`) | `(nil, error)` |
| An unterminated `---` block | `(nil, error)` |

The error arises because `UnmarshalYAML` parses `created_date` strictly
(`internal/metadata/metadata.go:145-148`). So "authored" means **non-nil metadata and nil error**,
and the error case must be treated as not-authored at every call site. Propagating it would make
`design list` and `design ref add` fail outright on an ordinary design file carrying a YAML header,
which is precisely the file this feature promises not to disturb.

**The agent-facing surfaces.** Skills install from two tables that must stay in step:
`workflowSkills` (`internal/agent/skills.go:26-32`) and `workflowDescriptions`
(`internal/agent/commands.go:18-24`). All three agents consume them
(`internal/agent/claude.go:22`, `bob.go`, `codex.go`), and `cmd/migrate.go:76` re-runs the whole
install, so a new skill reaches existing projects on upgrade with no settings-schema step.
`internal/migrate/registry.go:60-68` is for config format changes only and is not touched.

The standing instruction is a managed section keyed on its heading
(`internal/agent/design_trigger.go:10-19`), currently 51 lines at
`templates/agents/design-trigger.md`. Two other surfaces carry near-duplicates of the same offer:
`templates/skills/workflows/spek-new/SKILL.md:42-56` and
`templates/steps/spec/05-technical_approach.md:17-49`.

**Verified constraint.** `go run . skill spek-knowledge` exits 1; `go run . skill spawn-planning-agents`
exits 0. Only the flat `templates/skills/skill_*.md` files resolve through the CLI, never the nested
workflow skills. This is why the instruction must name `spek-design` in prose, and why
`internal/agent/design_trigger_test.go:253` asserts the section never contains `skill spek-`.

**Docs repo state at planning time.** `src/pages/design-documents.mdx` was untracked and
`src/components/Nav.astro` and `src/pages/configuration.mdx` were modified-unstaged, all from
000054. `src/pages/design-documents.mdx:25-30` currently asserts, without qualification, that
Spektacular "never adds frontmatter to a design document" and that a CLI-written document "comes
back byte for byte identical". That paragraph is falsified by this feature and must be reworked
rather than appended to.

## Per-Phase Technical Notes

### Phase 1.1: Record which specs reference a design

**Requirement-to-repo resolution.** "Designs Spektacular authors carry metadata" and "Recorded
back-links stay accurate" are both carried out in `spektacular`, in `internal/metadata`.

**File changes**

- `internal/metadata/metadata.go:75` — add `Specs []string` to `Metadata`, immediately after
  `Designs []DesignRef`. Doc comment states the direction: `Designs` is this artifact's outbound
  references, `Specs` is the inbound ones, and only a design document carries the latter today.
  Note the neighbouring `Spec string` at `:70` is provenance (which spec's conversation produced
  this document) and is a different fact; say so in the comment, because the two names are one
  character apart.
- `internal/metadata/metadata.go:98` — add `Specs []string \`yaml:"specs,omitempty"\`` to
  `yamlShape`, after `Designs`. `omitempty` is load-bearing: it is what keeps a document with no
  back-links rendering exactly as before.
- `internal/metadata/metadata.go:115` — add `Specs yaml.Node \`yaml:"specs"\`` to `yamlInShape`,
  as a raw node for the same reason `Designs` is one (`:111-115`): a malformed value must read as
  nothing rather than failing the whole parse and making the document unreadable.
- `internal/metadata/metadata.go:127` — add `Specs: m.Specs` to the `yamlShape` literal in
  `MarshalYAML`.
- `internal/metadata/metadata.go:167` — add `m.Specs = decodeSpecNames(in.Specs)` in
  `UnmarshalYAML`, beside the existing `m.Designs = decodeDesignRefs(in.Designs)`.
- `internal/metadata/metadata.go:177-195` — add `decodeSpecNames(node yaml.Node) []string`
  modelled on `decodeDesignRefs`: non-sequence yields nil, a non-scalar or empty entry is dropped.
  Keep it beside `decodeDesignRefs` so the two lenient decoders read as a pair.
- `internal/metadata/merge.go:27` — add `Specs *[]string` to `UpdateOptions`, after `Designs`.
  Reuse the existing tri-state doc comment at `:21-27` by extending it: nil means no change,
  non-nil replaces, non-nil empty clears.
- `internal/metadata/merge.go:73-75` — in the `fresh` branch, mirror the `opts.Designs` handling:
  `if opts.Specs != nil { result.Specs = *opts.Specs }`.
- `internal/metadata/merge.go:100-103` — in the existing branch, mirror exactly:
  `result.Specs = current.Specs` then `if opts.Specs != nil { result.Specs = *opts.Specs }`. This
  is the line that makes a back-link survive a body-only rewrite; the comment at `:96-99` already
  explains why for `Designs` and should be extended rather than duplicated.
- `internal/metadata/merge.go:30-46` — extend the `Merge` invariant list with the back-link
  equivalent of the design-reference invariant at `:43-44`.

**Tests**

- `internal/metadata/metadata_test.go:747` — add a `Specs` round-trip test modelled on
  `TestRender_SplitRoundTripDesignRefs`: two names survive render-then-split in order.
- `internal/metadata/metadata_test.go:770` — the existing
  `TestRender_OmitsDesignsKeyEntirelyWithNoReferences` asserts byte-exact output
  (`"---\ncreated_date: \"2026-07-01\"\ndocument_status: draft\n---\n\n# body\n"`). **It must stay
  green unmodified.** Add a sibling asserting the same bytes with `Specs` also empty, so the guard
  names both fields.
- `internal/metadata/metadata_test.go:786` — mirror
  `TestSplit_MalformedDesignsReadAsNoReferences` for `specs`: absent, empty list, scalar, mapping,
  and a list containing a non-scalar all read as no back-links, never an error.
- `internal/metadata/merge_test.go:29-118` — mirror all five `*[]DesignRef` tri-state tests for
  `*[]string`: body-only rewrite preserves, non-nil replaces, non-nil empty clears, fresh write
  records, and back-links survive a status transition.
- Inject `opts.Today` in every merge test, per
  `conventions/tests-must-not-depend-on-order.md`.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential. The change is mechanical but must touch all six
sites together; splitting it across agents is how one site gets missed, and a missed site fails
silently rather than loudly.

---

### Phase 1.2: Write a design document Spektacular authored

**Requirement-to-repo resolution.** "Users are guided to author a design they have not yet
written" (the storage half), "An existing design can be revised", "Design authoring does not
require a spec" and "Designs Spektacular authors carry metadata" are carried out in
`spektacular`, in `cmd/design.go`.

**File changes**

- `cmd/design.go:49-53` — add `designAuthorCmd` beside `designWriteCmd`, `Use: "author"`,
  `Short: "Write one design document into a declared source, stamping Spektacular's lifecycle
  metadata"`. `RunE: runDesignAuthor`.
- `cmd/design.go:115-122` — add `designAuthorOutputSchema`: the three keys
  `designWriteOutputSchema` publishes (`source`, `path`, `location`) plus `document_status` and
  `created_date`. Reuse `designAddressInputSchema` (`:74-81`) for input; the address shape is
  unchanged.
- `cmd/design.go` — add `runDesignAuthor`, modelled on `runDesignWrite` (`:234-277`) and on the
  stamping path in `cmd/storefile.go:207-232`:
  1. `--schema` short-circuit, as every sibling does.
  2. `designAddressData(cmd)` (`:132-145`) for the address.
  3. `--from` required, same refusal as `cmd/design.go:250-255` with code
     `design_from_required`.
  4. Parse `--document-status` through `parseDocumentStatusFlag` (`cmd/artifactfilter.go:27-35`)
     so the refusal is the project's existing `invalid_document_status` naming all four values.
     Empty flag means no status update, exactly as `metadataOptsForDocumentStatus`
     (`cmd/storefile.go:48-57`) already handles.
  5. `newDesignSet()` (`:150-160`), then `set.Read(doc)` to obtain existing bytes; a
     `design_not_found` refusal here is the *expected* first-write case and must be treated as
     "no existing content", not propagated. Distinguish it by checking `set.Exists(doc)` first,
     which avoids depending on error-string matching.
  6. `stripLeadingFrontmatterBlocks(content)` from `cmd/storefile.go:31-40` on the staged bytes,
     so re-authoring a document that was read back with its block does not stack a second one.
     That helper is currently unexported in package `cmd`; `cmd/design.go` is in the same package,
     so it is directly reusable with no move.
  7. `metadata.Merge(existing, body, opts)` with `opts.Spec` set from `--spec` and
     `opts.DocumentStatus` from step 4. **Do not** set `opts.Specs`: back-links are owned by the
     reference verbs and passing nil is what preserves them across a revision.
  8. `set.Write(doc, merged)`, then `set.Resolve(doc)` for the reported location.
  9. Re-split the merged bytes to report the resulting `document_status` and `created_date`, the
     same way `cmd/storefile.go:400-407` reports after a status change.
- `cmd/design.go:279-287` — register flags in `init`: `--data`, `--from`, `--document-status`
  (help text from `documentStatusValues()`, `cmd/artifactfilter.go:14-20`), `--spec`. Add
  `designAuthorCmd` to `designCmd.AddCommand(...)` at `:287`.
- Do **not** add a `--specs` flag. Back-links are not caller-supplied.

**Gotcha to honour**: `metadata.Merge` returns an error for malformed existing frontmatter
(`internal/metadata/merge.go:57-59`). A design whose file begins with the team's own frontmatter
will therefore fail an authored write. That is correct behaviour (Spektacular must not silently
replace their block), but the raw error is not actionable. Wrap it in an `output.NewError` naming
the file and saying the document already carries frontmatter Spektacular did not write, with a
next action of removing it or authoring to a different path.

**Tests** (`cmd/design_test.go`, using the existing helpers at `:69-122`)

- Authored write then read: bytes carry `created_date` and `document_status: draft`.
- `--spec` records provenance; omitted records nothing (no `spec:` key).
- Rewrite preserves `created_date` and any pre-existing `specs` list while replacing the body.
- `--document-status final` stamps `closed_date`; back to `draft` clears it. Mirrors
  `cmd/storefile_metadata_test.go:159,250`.
- Invalid status refused with `invalid_document_status`, file unchanged.
- Undeclared source refused with `design_source_unknown`, nothing written.
- Missing `--from` refused with `design_from_required`.
- Re-authoring content that already carries a block produces exactly one block.
- Authoring over a document with the team's own frontmatter is refused with an actionable message.
- `--schema` publishes the documented input and output shapes, mirroring
  `TestDesignSchema_PublishesDocumentedShapes` (`cmd/design_test.go:363`).
- Every test drives `resetRootCmd` + `runRootCmd`, per the file's own note at `:15-24`.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential. The command is one function composing pieces that
already exist; the risk is in the order of those pieces, not in volume.

---

### Phase 1.3: Keep the two kinds of design document apart

**Requirement-to-repo resolution.** "Designs Spektacular did not author gain nothing" and the
reporting half of "Authored designs report status, provenance and referencing specs" are carried
out in `spektacular`, in `cmd/design.go`.

**The discriminator, stated once.** A design document is one Spektacular authored **iff**
`metadata.Split` returns a non-nil `*Metadata` **and** a nil error. Verified by probe against the
real parser: a Spektacular block yields `(non-nil, nil)`; no block yields `(nil, nil)`
(`internal/metadata/frontmatter.go:18-20`); and a team's own frontmatter such as `title:`/`author:`
yields `(nil, error)` because `UnmarshalYAML` fails to parse the absent `created_date`
(`internal/metadata/metadata.go:145-148`), as does an unterminated block. **Every call site must
treat the error case as "not authored" and must not propagate it**, or a team's own frontmatter
would break listing and referencing. Put this in one unexported helper in `cmd/design.go`, e.g.
`authoredMetadata(raw []byte) *metadata.Metadata`, returning nil on either the no-block or the
error case, and use it from all three call sites (here and in phase 2.1).

**File changes**

- `cmd/design.go:264` — in `runDesignWrite`, before `set.Write`, read the existing document when
  `set.Exists(doc)` reports it is there, and refuse if `authoredMetadata` returns non-nil. New
  refusal in `internal/design/errors.go` style but raised in `cmd` (the design set has no notion
  of metadata and must not gain one): code `design_authored_overwrite`, message naming the path
  and saying it carries a lifecycle record Spektacular wrote, next action
  `"rewrite it with 'design author ...' , which preserves its capture date and the specs
  referencing it"`. Per
  `gotchas/remediation-needs-the-layer-that-holds-the-facts.md` the check belongs here, where the
  resolved path and the sibling command name are both in hand.
- `cmd/design.go:96-106` — extend `designDocumentItemSchema` and therefore
  `designListOutputSchema` with the optional `created_date`, `document_status`, `closed_date`,
  `spec` and `specs` keys.
- `cmd/design.go:197-206` — in `runDesignList`, for each document read it and, when
  `authoredMetadata` returns non-nil, add the fields to the item. Model the shape on
  `cmd/storefile.go:312-321`, which does exactly this for the spec, plan and changelog listings,
  including omitting `closed_date` when zero. Omit `spec` and `specs` when empty so a document
  with no provenance and no referrers does not report empty values.
- `runDesignRead` (`:214-232`) is unchanged: it already returns raw bytes.

**Tests** (`cmd/design_test.go`)

- Verbatim write over an authored document: refused, code `design_authored_overwrite`, file
  byte-identical afterwards, refusal's next action names `design author`.
- Verbatim write over a document with no block: still succeeds, bytes byte-identical to the
  staged file. This is the existing guarantee and must be asserted alongside the new refusal, not
  assumed.
- Verbatim write creating a new document: still succeeds.
- Verbatim write over a document carrying the team's own frontmatter: succeeds, and the stored
  bytes are exactly the staged bytes. This is the test that pins the error-means-not-authored
  rule; without it the rule is only in prose.
- Listing a source holding one authored and one pre-existing document: the first carries the
  lifecycle keys, the second carries only `source` and `path`.
- `--schema` for `list` publishes the extended item shape.

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential. The discriminator helper is shared with phase 2.1,
so it must land here and be reused there rather than written twice.

---

### Phase 2.1: Keep a design and its referencing specs in agreement

**Requirement-to-repo resolution.** "Recorded back-links stay accurate" is carried out in
`spektacular`, in `cmd/design_ref.go`.

**File changes**

- `cmd/design_ref.go` — add a shared helper implementing the two-document write, used by both
  verbs so the sequence and its failure behaviour exist once. Signature along the lines of
  `applyRef(st store.Store, specPath string, raw []byte, next []metadata.DesignRef, set *design.Set, doc design.Document, spec string, add bool) error`.
  Its body:
  1. `writeRefs(st, specPath, raw, next)` (`:173-187`) — the spec write, unchanged.
  2. If `set.Exists(doc)` is false, return nil. A reference may be recorded before its document
     exists; that is deliberate (`:198-205`).
  3. Read the document, `authoredMetadata` (phase 1.3). Nil means the project already had it:
     return nil, having written nothing to it.
  4. Compute the new back-link list: append `spec` if absent (add), or filter it out (remove).
     If unchanged, return nil without writing, mirroring `runDesignRefRemove`'s existing
     write-only-if-changed behaviour at `:280-284`.
  5. `metadata.Merge(existing, body, metadata.UpdateOptions{Specs: &nextSpecs})` and
     `set.Write(doc, merged)`.
  Failure handling is phase 2.2; this phase's helper returns the error and phase 2.2 wraps it.
- `cmd/design_ref.go:244-247` — `runDesignRefAdd` calls the helper instead of `writeRefs`
  directly, passing `add: true`. The existing duplicate short-circuit at `:237-242` stays: it
  returns before any write, and a duplicate `ref add` must remain a no-op on both documents.
- `cmd/design_ref.go:280-284` — `runDesignRefRemove` calls the helper with `add: false`. Note the
  current code writes the spec only when something was removed; the back-link removal must follow
  the same condition, or a no-op remove would rewrite the design.
- The spec name stored in the back-link is `input.Spec` **after** the `.md` normalisation
  `specStore` performs at `:140-142`, normalised back to the bare name. Pin it: the bare form is
  what `spec file list` reports and what `design ref` accepts either way
  (`TestDesignRef_BareAndSuffixedSpecNamesAddressTheSameSpec`, `cmd/design_ref_test.go:527`), so
  storing the bare form keeps `specs:` entries stable however the caller spelled them.

**Tests** (`cmd/design_ref_test.go`)

- Add records the spec on the authored design; the design's `specs` list contains it once.
- Remove drops it and leaves sibling entries intact.
- Duplicate add leaves the design's list with exactly one entry.
- Two specs referencing one design: both listed; removing one leaves the other. Extend the
  existing `TestDesignRef_TwoSpecsShareOneDesignIndependently` (`:363`) rather than adding a
  parallel fixture.
- Add and remove against a design with no block: both succeed, the file is byte-identical,
  asserted by hashing before and after (the file already imports `crypto/sha256` for this,
  `cmd/design_test.go:4`).
- Add and remove against a design carrying the team's own frontmatter: both succeed, file
  byte-identical.
- Add against a document that does not exist: succeeds, mirroring
  `TestDesignRefAdd_RecordsAReferenceToADocumentThatDoesNotExistYet` (`:399`).
- Bare and `.md`-suffixed spec names produce the same single `specs` entry.
- A bidirectional consistency test: after a sequence of adds and removes across two specs and two
  designs, every spec each design lists does reference it and none is missing. This is the
  assertion that maps directly to the success metric.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential. Both verbs change together through one helper.

---

### Phase 2.2: Never leave a spec and a design disagreeing

**Requirement-to-repo resolution.** The constraint "A reference operation that cannot complete
its back-link write must fail as a whole and leave the spec exactly as it was" is carried out in
`spektacular`, in `cmd/design_ref.go`.

**File changes**

- `cmd/design_ref.go` — wrap the phase 2.1 helper's steps 3 to 5 so that any error from them
  triggers compensation: write `raw` (the spec's original bytes, already returned by `refsOf` at
  `:154-167`) back with `st.Write(specPath, raw)`.
  - Compensation succeeds: return `output.NewError("design_ref_backlink_failed", ...)` naming the
    design and the underlying cause, with a next action of retrying the command, and stating that
    the spec was left unchanged.
  - Compensation fails: return `output.NewError("design_ref_backlink_rollback_failed", ...)`
    naming **both** the spec's absolute path and the design's absolute path
    (`set.Resolve(doc)`), stating that the spec now records a reference the design does not, and
    giving a next action naming the two files to reconcile by hand. Two distinct codes, per the
    judgement recorded in the assumption log: an agent branching on the code must not treat
    "retry" and "repair by hand" as the same outcome.
- Order is fixed and must not be changed to design-first: writing the back-link first would make
  the spec trivially untouched on failure but would leave a design listing a spec that does not
  reference it, which acceptance criterion "Back-links match references in both directions"
  forbids outright.

**Test seam.** This is the one place the feature needs a seam that does not exist. The design set
is constructed by `newDesignSet()` (`cmd/design.go:150-160`) from config; there is no injection
point. Two options, in order of preference:
1. Make the back-link write fail through the filesystem in a way the test owns: after the
   document is created, replace it with a read-only file, or make its parent directory
   non-writable, inside `t.TempDir()`. This needs no production change and stays within the
   "tests own their filesystem" rule. Confirm on this platform before relying on it; a test
   running as root would not observe the permission.
2. If (1) proves unreliable, introduce a package-level `var designSetFactory = newDesignSet` in
   `cmd/design.go` that tests substitute, following the precedent of `var sourceFS fs.FS =
   templates.FS` in `internal/agent/skills.go:36`, which exists for exactly this reason. Restore
   it in `t.Cleanup`, since `-shuffle=on` makes a leaked substitution a cross-test failure.

The rollback-failure case needs the spec store write to fail after the design write already has.
Option 2 cannot produce that on its own; provoke it by making the spec file itself read-only
between the two writes, which requires option 2's seam to run a hook. Prefer expressing this
single test through a substituted design set whose write fails, combined with a spec store whose
second write fails, and keep it in one focused test rather than spreading the seam.

**Tests** (`cmd/design_ref_test.go`)

- Back-link write fails: command exits non-zero with `design_ref_backlink_failed`; the spec file
  is byte-identical to before the command; the message names the design and the next action says
  retry.
- Same for `ref remove`.
- Rollback also fails: `design_ref_backlink_rollback_failed`; the message contains both absolute
  paths; the next action names manual reconciliation.
- The two codes are asserted as distinct string values, and their next actions asserted by
  content, not merely non-emptiness, per
  `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`.

**Complexity**: High
**Token estimate**: ~30k tokens
**Agent strategy**: Parallel analysis, sequential integration. Investigating which test seam works
on this platform is independent of writing the compensation logic; the integration of the two is
not.

---

### Phase 3.1: Add the design skill

**Requirement-to-repo resolution.** "Users are guided to author a design they have not yet
written", "The guided conversation ends when it stops being useful", "A design the user already
has can be brought in" and "An existing design can be revised" are carried out in `spektacular`,
in `templates/skills/workflows/spek-design/SKILL.md` and the two install registries.

**File changes**

- `templates/skills/workflows/spek-design/SKILL.md` — new file. Structure modelled on
  `templates/skills/workflows/spek-knowledge/SKILL.md`:
  - Frontmatter `name: spek-design`, `description: Author, bring in, revise or reference a design
    document.` The description is also what `workflowDescriptions` must carry.
  - `{{> partials/version-check}}` as the first body line, as every skill does
    (`templates/partials/version-check.md`).
  - `# What this skill does` — state explicitly that it is a static playbook and does **not**
    drive an interactive CLI state machine, mirroring `spek-knowledge/SKILL.md:10`. This sentence
    is the one a future maintainer reads before wondering why it is not a workflow.
  - `# When to invoke` — natural-language triggers for all four situations.
  - `# Intent: author` — the interview. Cite White et al., arXiv:2302.11382 by name and number,
    the same citation as `templates/steps/spec/00b-interview.md:3`, so the two interviews are
    recognisably one pattern. State the goal ("understand the design well enough to write a
    document someone could build from"), that questions are adaptive rather than a script, and
    the stopping condition in testable words: stop when a further answer would not change the
    document. Add the anti-transcript rule the spec's metric requires: the document records the
    decisions, not the conversation that produced them. Ends in
    `{{command}} design author --data '{"source":"<name>","path":"<path>"}' --from <staged file>`,
    optionally `--spec`, then `{{command}} design ref add` **only if a spec exists**.
  - `# Intent: bring in` — the user already has it. Ends in `{{command}} design write ... --from`,
    and states the guarantee that the bytes are stored exactly as supplied.
  - `# Intent: revise` — ends in `design author` again, noting it preserves the capture date and
    the referencing specs, so existing references keep resolving.
  - `# Intent: reference only` — ends in `design ref add` alone.
  - `# Decline handling` — one gate section mirroring `spek-knowledge/SKILL.md:123-129`: nothing
    is written without explicit agreement, a decline writes nothing and is final for that detail,
    and the detail does not get smuggled into the spec body instead. State that the contract is
    enforced by prose, not a CLI guard.
  - Use `{{command}}`, never the rendered `go run .`.
  - Do **not** write `{{command}} skill spek-...` anywhere.
- `internal/agent/skills.go:26-32` — add
  `{Name: "spek-design", TemplatePath: "skills/workflows/spek-design/SKILL.md"}` to
  `workflowSkills`. Placement after `spek-knowledge` keeps the table reading workflow-first then
  playbooks.
- `internal/agent/commands.go:18-24` — add the matching `workflowDescriptions` entry. A skill in
  one table and not the other renders an empty description into every non-Claude agent's
  slash-command menu.
- No `internal/migrate` registry change. `cmd/migrate.go:76` re-runs `a.Install(root, cfg, ...)`,
  which calls `installWorkflowSkills` (`internal/agent/skills.go:43`), so both fresh install and
  upgrade pick the skill up. `internal/migrate/registry.go:60-68` is for config schema steps only.

**Tests**

- **Three hand-maintained skill tables must be updated together.** There is no
  `skills_test.go`; the oracle lives once per agent, each a map of skill name to an expected
  content substring, each preceded by a comment that literally says "Exactly five SKILL.md
  files": `internal/agent/claude_test.go:25-34`, `internal/agent/bob_test.go:24-33`,
  `internal/agent/codex_test.go:24-33`. Add a `spek-design` row and correct the count in the
  comment in all three. `bob_test.go:49-50` additionally pins the command-wrapper filenames and
  their content; add `spek-design.md` there. `validateSkillFrontmatter`
  (`claude_test.go:51`, `bob_test.go:71`, `codex_test.go:52`) then checks the new skill's
  frontmatter for free.
- **Add the missing cross-table invariant.** Nothing currently asserts that every entry in
  `workflowSkills` has a `workflowDescriptions` entry, which is why a skill can be added to one
  and not the other and render an empty description into every non-Claude agent's command menu.
  Add that assertion over the whole table rather than the one new row, so the next skill cannot
  repeat the mistake. `internal/agent/agent_test.go` is the natural home, being the only
  agent-package test file not tied to one agent.
- `templates/skill_list_command_test.go:49-60` — add a `spek-design` row to
  `TestWorkflowSkillsDocumentDesignCommands` naming `design author`, `design write`,
  `design ref add` and `design sources`.
- New template-contract test beside it, rendering the skill and asserting anchor literals: the
  static-playbook sentence, all four intent headings, `arXiv:2302.11382`, the stopping-condition
  phrase, and the explicit-agreement phrase. Hand-maintained literals, never derived from the
  template, per `architecture/testing-architecture.md`.
- `internal/agent/instruction_surface_test.go:35-75` picks the new template up automatically in
  both its walks; no change needed, but confirm it passes.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: one drafts the skill prose, one wires the registries and
tests. Integration is a single merge, since they touch disjoint files.

---

### Phase 3.2: Rewrite the standing design instruction

**Requirement-to-repo resolution.** "Agents act on conversation about a design" is carried out in
`spektacular`, in `templates/agents/design-trigger.md`.

**File changes**

- `templates/agents/design-trigger.md` — rewrite the body. Keep the heading
  `## Design-Worthy Detail Recognition` **exactly**: `internal/agent/design_trigger.go:11` matches
  on it, and `internal/agent/spec_trigger_test.go:157-169` pins its position between the
  Spec-Worthy and Presenting-Drafts sections. Keep the managed-section banner at `:3-5`.
  New structure:
  1. **Alert** (new, short): during any conversation working out how something will actually
     behave, stay alert. Separate sentence from the bar, because conflating them is gap 2.
  2. **Qualifying bar** (keep `:15-23` largely as-is): settled, worked, would swamp the spec. All
     three.
  3. **Entry cases** (new): the user already has a design; a conversation is changing one that
     exists; design talk is happening with no spec.
  4. **Offer** (keep `:25-29` shape): say what you would capture and which source; run
     `{{command}} design sources` if the sources are not known.
  5. **Outcomes** — keep all three bullets and their existing load-bearing phrases verbatim, since
     tests pin them: `- **Accept**`, `- **Defer**`, `- **Decline**`, `one of three outcomes`,
     `decline is final for that detail`, `offer — never write a design document`,
     `Silence or deflection is not acceptance.` The accept branch changes: it now says to invoke
     the `spek-design` skill by name, and records a reference **only when a spec exists**, instead
     of the current unconditional two-command sequence at `:33-38`.
- Keep `{{command}} design sources`, `{{command}} design write` and `{{command}} design ref add`
  named somewhere in the section: `internal/agent/design_trigger_test.go:235-243`
  (`TestRenderedDesignTriggerAcceptBranchNamesBothCommands`) asserts the first two appear **in the
  accept branch**, between `- **Accept**` and `- **Defer**`. That test needs updating in step
  with the rewrite rather than deleting: keep the commands in the accept branch and add
  `design author` to the asserted set.
- `internal/agent/design_trigger_test.go:253` —
  `TestRenderedDesignTriggerSectionNamesCommandsDirectly` asserts `NotContains "skill spek-"`.
  **Keep it unchanged.** Naming the skill in prose ("invoke the `spek-design` skill", as
  `templates/agents/knowledge-trigger.md:24` does) does not contain that substring; an instruction
  to run `{{command}} skill spek-design` would. This is the guard for the spec's hard constraint.

**Tests** (`internal/agent/design_trigger_test.go`)

- Extend the anchor-literal tests at `:198-232` with new literals for the alert sentence, the
  pre-existing-design case, the revision case and the no-spec case.
- Update `TestRenderedDesignTriggerAcceptBranchNamesBothCommands` (`:235`) to the new command set.
- Keep `:253` and the placeholder test at `:262` unchanged; both must still pass.
- `internal/agent/spec_trigger_test.go:157-169` must still pass unmodified: one heading, in the
  same position.

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential. The prose and its pinned literals must move
together; a second agent editing the tests independently is how the two drift.

---

### Phase 3.3: Align the spec workflow's own design offer

**Requirement-to-repo resolution.** "Agents act on conversation about a design" also reaches
`spektacular`'s spec skill and spec step templates, which carry duplicates of the same offer.

**File changes**

- `templates/steps/spec/05-technical_approach.md:17-49` — the in-workflow capture offer. Rewrite
  to match phase 3.2's structure: it currently presents capture of an already-settled design as
  the only path, and its accept branch hard-codes `{{spec_name}}`. Add the authoring case,
  pointing at the `spek-design` skill by name, and keep the three-part bar at `:24-29` and the
  "Silence or deflection is not acceptance" line at `:48`.
- `templates/skills/workflows/spek-new/SKILL.md:42-56` — the design section. Line `:54` currently
  reads "store a design document, byte for byte, with no frontmatter added and nothing
  reformatted", describing `design write`. Narrow it to say that is what the verbatim command
  does, add `{{command}} design author` as the command for a design Spektacular writes with the
  user, and point at the `spek-design` skill for the authoring conversation.
- `templates/skills/workflows/spek-plan/SKILL.md:38-56` — **no change**. The plan side reads
  references and documents; nothing about how a design came to exist changes what it does.
**Tests**

- **Five literals in `internal/steps/spec/steps_test.go:531-589` pin the technical-approach
  template and move with it**:
  `"**When the design is settled, offer to capture it rather than compress it away.**"` (`:541`),
  `"spektacular design write"` (`:570`), `"spektacular design ref add"` (`:572`), the
  decline-does-not-move-the-design-into-the-spec-body assertion (`:588`), and
  `"Silence or deflection is not acceptance."` (`:589`). Note `:570-572` assert the **rendered**
  command form (`spektacular design write`, not `{{config.command}} ...`), so a new
  `design author` literal takes the same form. Update `:541` to whatever the rewrite makes the
  heading, add an assertion for the authoring branch, and keep `:588-589` unchanged.
- `internal/steps/plan/steps_test.go` also gained design assertions in 000054
  (`git show --stat caaa761`). The plan templates are not being changed, so those should pass
  untouched; confirm rather than assume.
- `templates/skill_list_command_test.go:49-60` — update the `spek-new` row to include
  `design author`.
- A repo-wide template assertion that no skill or step template claims Spektacular adds no
  frontmatter to a design document without qualifying it. Express it as a forbidden-substring
  entry in the existing closed list at `internal/agent/instruction_surface_test.go:23-30`, which
  already walks `skills/workflows` and `steps` and the rendered skills.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential. Small, but every edit has a pinned literal beside
it.

---

### Phase 4.1: Explain design authoring on the documentation site

**Requirement-to-repo resolution.** "Public docs explain design authoring" is carried out in
`docs`, root `/home/nicj/code/github.com/jumppad-labs/spektacular-website`.

**Check the working tree first.** At planning time `docs:src/pages/design-documents.mdx` was
**untracked** and `docs:src/components/Nav.astro` and `docs:src/pages/configuration.mdx` were
modified-unstaged, all from the 000054 docs work. Run `git status` before editing. If that work
has since been committed or rebased, re-read the page: every line number below moves.

**File changes** (all paths relative to the `docs` root)

- `src/pages/design-documents.mdx:21-34` — rework the "Spektacular owns the reference, not the
  document" paragraph inside `<Section heading="What a design document is">` (opens `:17`). Keep
  the ownership sentence. Replace the unqualified "It never adds frontmatter to a design
  document ... comes back byte for byte identical" with the two-class statement from the phase's
  content outline in plan.md.
- New `<Section heading="Working a design out with Spektacular" surface>` after the existing
  `<Section heading="When to use one instead of a spec section" surface>` (`:40`, shaded) and
  before `<Section heading="How they relate to specs and plans">` (`:76`, plain). **Surface
  alternation**: inserting here breaks the run, so set the new section plain and flip `:76`,
  `:120`, `:161`, `:241` and `:282` accordingly, or place the new section so the existing values
  still alternate. Per `conventions/alternate-section-background.md`, set the value explicitly
  after checking what the preceding section resolved to; do not leave it unset.
- New `<Section heading="What an authored design records">` after it, carrying the four bullets
  and a fenced `yaml` block. **Bullets, not a table**: the body stylesheet carries no table
  styling, recorded in the 000054 changelog record at
  `.spektacular/changelog/spektacular/000054_project-level-design-documents.md:47-50`.
- `src/pages/design-documents.mdx:170-235` — extend the command reference inside
  `<Section heading="Working with designs from the command line">` (`:161`) with a fenced `bash`
  block for `design author`, placed next to the existing `design write` block, plus one sentence
  distinguishing them.
- `src/pages/design-documents.mdx:241-276` — extend
  `<Section heading="When a reference cannot be found" surface>` with a paragraph on a reference
  operation failing as a whole.
- Components: compose from `Section` and `Prose nested`, already imported at `:1-10`. No new
  component and no import change needed.

**Conventions binding here** (`docs/.spektacular/knowledge/conventions/`)

- `mdx-authoring.md` Rule 1: no `<div>`, `<section>` or `class=` in the page body. Guard:
  `grep -nE "<div|<section|class=" src/pages/*.mdx` returns nothing.
- Rule 3: blank line after each opening component tag and before each closing tag.
- Rule 4: code blocks are fenced markdown; the transcript goes in a bare fenced block, as
  `src/pages/projects.mdx:176-212` and `src/pages/how-it-works.mdx:242-252` already do.
- `no-em-dashes.md`: no em dashes anywhere in the new prose, and a spaced hyphen is not a
  substitute.
- `site-layout.md`: one component per band; do not introduce a new band type.

**Verification**

- `npm run build` succeeds, and `npx astro check` (or `make check`, `Makefile:15-16`) reports 0
  errors and 0 warnings. `astro check` is **not** an npm script and CI never runs it
  (`.github/workflows/deploy.yml:41` runs `npm run build` only), so it is a local gate that must
  be run deliberately.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential. The surface-alternation change is global to the
page, so parallel editors would conflict on the same file.

---

### Phase 4.2: Narrow the guarantee everywhere else it is stated

**Requirement-to-repo resolution.** Spans both repos: `docs` for the configuration page,
`spektacular` for the project's own design-folder README.

**File changes**

- `docs:src/pages/configuration.mdx:223-239` — the `<ConfigKey name="design" type="section">`
  entry. Bring its description into line with the reworked main page and keep the existing
  cross-link at `:238` (`See the [Design Documents](/design-documents/) page`).
- `spektacular:.spektacular/design/README.md:22` — "Spektacular owns the reference, not the
  document. Nothing it writes adds frontmatter to a file in here" is false once `design author`
  exists. Narrow it, and add the `design author` command to the list at `:11-15`.
- Sweep for stragglers: `grep -rn "adds frontmatter\|byte for byte\|no frontmatter"` across
  `docs:src/pages/` and `spektacular:README.md`, `spektacular:templates/`. At planning time the
  only hits outside the design page were
  `spektacular:templates/skills/workflows/spek-new/SKILL.md:54` (phase 3.3) and
  `spektacular:.spektacular/design/README.md:22` (here). Re-run the sweep rather than trusting
  that list.

**Verification**

- Site builds and type-checks clean, as phase 4.1.
- `go test ./...` still green in `spektacular` (the README change touches no code, but the forbidden-substring
  assertion added in phase 3.3 walks template files and must not be tripped).

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential. Two small edits in two repos plus a sweep.

## Testing Strategy

The plan sits in two of the project's three test layers. Per-phase detail is in the phase notes
above; this is how the layers divide across the work.

**Layer 1, Go unit and command tests.** Phases 1.1 through 2.2 are almost entirely this layer.
Phase 1.1 is pure `internal/metadata` unit work, mirroring the existing `Designs` tests one for
one: round-trip in `metadata_test.go`, tri-state merge semantics in `merge_test.go`, lenient decode
of malformed input, and the byte-exact no-field guard. Phases 1.2, 1.3 and 2.1 are `cmd` command
tests driven through `resetRootCmd` + `runRootCmd`, using the fixture helpers already in
`cmd/design_test.go:69-122`, including `twoSourceDesignProject` so the new verb is exercised
against both a relative source and an absolute one outside the project root. Phase 2.2 is the one
place needing a seam that does not exist; the two acceptable routes are in its phase notes and the
choice between them is an Open Question.

**Layer 2, template-contract tests.** Phases 3.1 through 3.3 are entirely this layer, because the
behaviour is prose. Assertions are hand-maintained literal phrases rendered from the real template,
never derived from it, per `architecture/testing-architecture.md`. Existing homes:
`internal/agent/design_trigger_test.go` for the standing instruction,
`internal/steps/spec/steps_test.go:531-589` for the technical-approach step,
`templates/skill_list_command_test.go` for the skill-to-command table. Two negative assertions are
load-bearing rather than defensive and must survive: `design_trigger_test.go:253`
(`NotContains "skill spek-"`) and the forbidden-substring list at
`internal/agent/instruction_surface_test.go:23-30`, which phase 3.3 extends.

**Layer 3, harbor end-to-end.** Nothing is added. The suites prove a real agent drives a state
machine in the right order and this feature adds no state machine. The existing suites are
**checked for drift** rather than extended, because skill and template changes are exactly what
their hand-maintained oracles couple to. The relevant oracle is
`tests/harbor/plan-workflow/tests/test_plan_workflow.py:117-130` (`DESIGN_REF_LIST_COMMAND`), which
concerns the plan step's obligation and should be unaffected; confirm rather than assume. The
suites do not run in CI and take roughly 25 minutes each, so a run is a deliberate act at the end
of the work, not a per-phase gate.

**Hand-maintained oracles this plan must update.** Each of these would pass silently until the
suite ran, so they are listed together rather than left to be discovered:

- `internal/agent/claude_test.go:25-34`, `bob_test.go:24-33`, `codex_test.go:24-33` — the three
  per-agent skill tables, each preceded by a comment that literally says "Exactly five SKILL.md
  files". All three need the new row and a corrected count.
- `internal/agent/bob_test.go:49-50` — the command-wrapper filename table.
- `templates/skill_list_command_test.go:49-60` — the skill-to-design-command table, gaining a
  `spek-design` row and an updated `spek-new` row.
- `internal/steps/spec/steps_test.go:541,570,572,588,589` — five literals pinning the
  technical-approach template. Note `:570-572` assert the **rendered** form
  (`spektacular design write`), not the `{{config.command}}` placeholder.

**A missing invariant this plan adds.** Nothing currently asserts that every `workflowSkills` entry
has a `workflowDescriptions` entry. A skill in one table and not the other renders an empty
description into every non-Claude agent's command menu, silently. Phase 3.1 adds the assertion over
the whole table.

**Success metric coverage.** Two of the spec's six metrics are covered automatically: "bringing in
an existing design costs no rework" by the byte-identical round-trip and untouched-on-reference
assertions, and "back-links can be trusted" by the bidirectional consistency test in phase 2.1 plus
the two failure paths in phase 2.2. The other four are classified *Manual, captured in the
implementation test plan*, each paired with the testable proxy that does exist. The classification
and its reasoning are in plan.md's Testing Approach; the implement workflow reads the plan, not the
spec, so nothing about them may be dropped here.

**Gate before done.** `go test ./...` green (the Makefile runs it with `-shuffle=on`), plus, for
the documentation phases, `npm run build` and `npx astro check` reporting zero errors and zero
warnings in the `docs` repo. `astro check` is not an npm script and CI never runs it
(`.github/workflows/deploy.yml` runs `npm run build` only), so it is a deliberate local gate.

## Project References

- **Knowledge, `spektacular`**: `conventions/error-messages-must-suggest-remediation.md`,
  `conventions/tests-must-not-depend-on-order.md`, `conventions/tests-must-pass-for-done.md`,
  `architecture/testing-architecture.md` (the three-layer model and the hand-maintained-oracle
  rule), `architecture/working-with-files-from-steps.md`,
  `architecture/cli-design-for-ai-agents.md`,
  `gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md`,
  `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`.
- **Knowledge, `docs`**: `conventions/mdx-authoring.md`, `conventions/no-em-dashes.md`,
  `conventions/site-layout.md`, `conventions/alternate-section-background.md`,
  `conventions/plan-content-pages.md`, `decisions/frame-width-flow.md`.
- **Prior plans**: `000054_project-level-design-documents` (the feature extended, and the source of
  the surface map via `git show --stat caaa761`), `000052_document-status-vocabulary`,
  `000041_workflow-knowledge-capture-offers`, `000022_spek-knowledge-skill`,
  `000043_flipped-interaction-spec-interview`, `000039_project-level-capabilities`.
- **External**: White et al., "A Prompt Pattern Catalog to Enhance Prompt Engineering with
  ChatGPT", arXiv:2302.11382, cited verbatim at `templates/steps/spec/00b-interview.md:3` and to be
  cited identically in the new skill.
- **Design documents this plan was built on**: none. `design ref list` for this spec returns an
  empty list with nothing unresolved.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per-phase estimates, summing to roughly 217k across the plan:

| Phase | Estimate | Complexity | Strategy |
|---|---|---|---|
| 1.1 Record which specs reference a design | ~12k | Low | Single agent, sequential |
| 1.2 Write a design document Spektacular authored | ~25k | Medium | Single agent, sequential |
| 1.3 Keep the two kinds of design document apart | ~20k | Medium | Single agent, sequential |
| 2.1 Keep a design and its referencing specs in agreement | ~25k | Medium | Single agent, sequential |
| 2.2 Never leave a spec and a design disagreeing | ~30k | High | Parallel analysis, sequential integration |
| 3.1 Add the design skill | ~30k | Medium | 2 parallel agents (prose / wiring) |
| 3.2 Rewrite the standing design instruction | ~20k | Medium | Single agent, sequential |
| 3.3 Align the spec workflow's own design offer | ~15k | Low | Single agent, sequential |
| 4.1 Explain design authoring on the documentation site | ~30k | Medium | Single agent, sequential |
| 4.2 Narrow the guarantee everywhere else it is stated | ~10k | Low | Single agent, sequential |

Several medium phases are marked single-agent against the tier table's default. That is
deliberate: 1.1 must touch six sites in one package together or lose data silently, 1.3 and 2.1
share a discriminator helper that must land once, and 4.1 changes section shading across a whole
page, so parallel editors would conflict on one file. Parallelism is used where the work is
genuinely disjoint (3.1) or where investigation and implementation are separable (2.2).

## Migration Notes

**No settings-schema migration.** This plan changes no config format, so nothing is registered in
`internal/migrate/registry.go`. `config.CurrentProjectSchema` and `config.CurrentRepoSchema` are
untouched.

**Delivery to existing projects is the existing upgrade path.** `cmd/migrate.go:76` re-runs
`a.Install(root, cfg, io.Discard)`, which reinstalls every workflow skill and re-renders every
managed instruction section. So an existing project receives the new skill and the rewritten
standing instruction by running `migrate`, with no new command and no manual step. Verify this once
end to end rather than trusting the code path, since it was read but not exercised during planning.

**No data migration for design documents.** Designs already in a declared source are not touched:
none gains a lifecycle record, and no backfill exists or is wanted. A pre-existing design becomes
an authored one only if a user deliberately re-authors it, which is their choice rather than an
upgrade effect.

**One forward-compatibility note.** A design document authored by this release and then read by an
older Spektacular would have its `specs` key dropped on the next write by that older binary,
because the schema is closed in both directions. This is the same exposure `designs` already
carries on specs and is not newly introduced here, but it is worth knowing before anyone tests
across versions.

## Performance Considerations

**One new read per document in the design listing.** `design list` currently lists paths without
opening files; it now reads each document to report lifecycle fields. The cost is proportional to
the number of documents in a declared source, and the precedent is exact: `spec file list`,
`plan file list` and `changelog file list` already read every artifact they list
(`cmd/storefile.go:314-316`) and have never needed optimising. A source's `.spektacular_ignore`
already keeps unwanted files out of the listing, which is the existing lever if a source ever grows
large enough to matter.

**Reference operations become two writes instead of one.** Both are small local file writes and
both are already bounded by the spec and design stores being on local disk under the `file`
provider, the only backend this release ships. No caching, batching or concurrency is introduced,
and none is warranted: these commands run once per design reference during a conversation, not in
a loop.

**Nothing on a hot path.** Every surface this plan touches is a one-shot CLI invocation driven by
an agent during a conversation with a human in it. There are no latency requirements in the spec
and none are introduced.
