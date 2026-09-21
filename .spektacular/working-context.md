# Working context: 000056_store_delete_and_knowledge_maintenance

## How this came up

Not from a feature request. It surfaced while running a maintenance pass over this project's own
knowledge base, immediately after shipping 000055 (design authoring). The audit found four stale
entries and there was no supported way to remove any of them.

The user's instruction at that moment:

> "on the delete, just use your file tools, also create an issue about the delete feature in the
> knowledge base. We should add that along with knowledge base maintenance to the skill."

So four entries were removed with `rm`, deliberately and with explicit permission, as a one-off
rather than a precedent. That became GitHub issue #44.

Then, on reading the issue:

> "update that issue where knowledge has no delete, we need the same for design"

And, correcting a claim in that update:

> "Well, it can't treat stale knowledge actually, I just was running a cleanup on the knowledge and
> not having delete meant the agent could not remove old knowledge and had to go around and use the
> file system. This approach is not going to work for remote file types"

That last message is the core of the problem statement and the reason this is not a convenience
feature. Preserve its framing.

## The problem

Two of the five stores cannot delete. `knowledge` and `design` have no delete verb. `spec file
delete`, `plan file delete` and `changelog file delete` all work today.

The asymmetry is an **assembly artifact, not a decision**. The three that work inherit the verb
from the shared command factory (`cmd/storefile.go:271` calls `st.Delete`). Knowledge and design
are the only two whose command families were hand-written instead. Design's divergence is
deliberate and signposted at `cmd/design.go:13-23`, but for a reason with nothing to do with
deletion: the factory stamps lifecycle frontmatter into everything it writes, which a design
document must not get. Nobody ever decided entries should be unremovable.

## Why the filesystem workaround is not acceptable

Two reasons, and the second is the user's and is the stronger one.

1. It breaks the project's own store-access rule. A store write is not a file copy; hand-editing
   loses lifecycle metadata the CLI would have preserved.
2. **It does not generalise beyond the `file` provider.** Both subsystems are explicitly built to
   grow backends: `internal/design/paths.go:11` calls `file` "the one design storage backend this
   release ships" and refuses others by name with `design_provider_unsupported`;
   `internal/knowledge/set.go:115` refuses an unknown knowledge provider the same way. Once a
   source is a git checkout, a remote URL or an issue tracker there is no path on disk to `rm`.
   So it is not a stopgap that buys time until the verb lands. It stops working the first time a
   non-file provider ships, and it fails for the store where hand-editing is least possible.

## The storage half already exists

`store.Writer` declares `Delete(path string) error` (`internal/store/store.go:104`),
`FileStore.Delete` implements it (`:176`), `ignoreStore` forwards it (`internal/store/ignore.go:87`).
Any future provider implementing `Writer` supplies `Delete` with `Write`, one interface. The work
is therefore almost entirely command-layer: addressing, refusal rules, and the back-link question.

## Requirements as discussed

Three pieces, to be built together because the third cannot do its job without the first two.

1. **`knowledge delete`** — addressed like every other knowledge verb (tier, name, path).
2. **`design delete`** — plus the back-link problem below.
3. **A maintenance branch in the `spek-knowledge` skill** — entry *truth*, not entry tags.

Plus the three audit findings, which the user explicitly brought into scope ("we should fix the
audit findings"). See below.

## Decision: design delete REFUSES while referenced

The user chose this over cascading:

> "I like delete while referenced, the agent can always remove the references, this forces a
> cleanup"

Read as: refuse the delete while any spec still references the design. The agent resolves it by
running `design ref remove` first, and the refusal is what forces the tidy-up rather than hiding
it. Rejected alternatives:

- **Cascade** (remove the reference from every referencing spec automatically). Convenient, but it
  is a multi-document write, the exact shape phases 2.1/2.2 of 000055 needed a compensating
  rollback for because no transaction is available. Same failure mode, more documents in play.
  Possibly worth a later explicit opt-in, not the default.
- **Delete and leave references dangling.** Cheapest, contradicts 000055's acceptance criterion
  that an authored design and the specs referencing it never disagree.

A design the project did not author carries no back-links, so the question does not arise for
those and a plain delete is fine.

## A trap to document

`design ref remove` already exists and reads as though it were the missing delete verb. It is not:
it removes the *reference* a spec carries and deliberately leaves the document alone. Someone
hunting for a way to delete a design finds it first, and it appears to succeed.

## The maintenance branch, and the distinction it must not get wrong

The skill's existing `audit` intent is explicitly tags-only. Maintenance is a different question:
is the entry still **true**? A tags audit will happily confirm that a thoroughly obsolete entry is
well tagged.

**The load-bearing distinction**: a convention the tree violates is **not stale**. In this project
an entry states the target and the code is what has yet to meet it. Only an entry whose subject no
longer exists is stale. Getting this backwards would delete precisely the entries doing their job.

Every staleness claim must carry evidence: the symbol, file or behaviour that changed. "Looks old"
is not a finding. The audit that prompted this proved the point — neither of the two substantive
deletions was detectable from the entry alone, both needed the claim checked against the code.

## The three audit findings, now in scope

1. **Category READMEs leak into both retrieval surfaces.** `conventions/README.md` and
   `glossary/README.md` are emitted by `knowledge always-applied`, so every payload injects a
   meta-description of what a convention is into the agent's context. The other four sit in
   looked-up categories and are returned by `Set.Search`, competing with real entries. One
   `README.md` exclusion in the walk fixes both. In the `spektacular` store the glossary is
   otherwise empty, so its README is the only thing the glossary contributes to every task.
2. **Tags on always-applied categories are silently inert.** `Set.Search` skips always-applied
   categories (`internal/knowledge/set.go:221`) and `Set.Tags` skips them when building the
   vocabulary (`:412,423`). Both deliberate and sound, but the side effect is that tags on a
   `conventions/` or `glossary/` entry can never be reached and never appear in `knowledge tags`.
   Two conventions in this repo already carry dead tags.
3. **Category descriptors drift between stores and nothing detects it.** `architecture/README.md`
   in the `docs` store carries superseded purpose text while the `spektacular` store's copy carries
   the current registry text, so the two stores disagree about what a category means. Fix is a
   per-repo regenerate, not a content edit. A maintenance branch should diff every category README
   against `internal/knowledge/category.go` and report drift, since a stale descriptor silently
   miscategorises everything written under it afterwards.

## Interview answers (all settled, none left open)

- **Generated category descriptions cannot be deleted** — refused, with a next action saying how
  to regenerate. They are descriptors, not knowledge, and nothing restores one until the next init.
- **Deleting something already absent succeeds and changes nothing**, in both stores. Mirrors
  `design ref remove`'s idempotence; a maintenance pass may retry and that must be safe.
- **The documentation site is in scope** for the two new verbs. Documenting the existing
  `spec`/`plan`/`changelog` `file delete` verbs was offered and declined, so it stays a Non-Goal.
- **The three audit findings are in scope** ("we should fix the audit findings").

## Superseded open questions

- Should `knowledge delete` refuse to remove a category README, given they are generated
  descriptors rather than knowledge? (Suspect yes, with a next action.)
- Does deleting something already absent succeed (mirroring `design ref remove`'s idempotence) or
  refuse?
- Do the three audit findings belong in this spec or their own? The user said fix them; whether
  that is here or alongside is still open.

## Reference

GitHub issue #44: https://github.com/jumppad-labs/spektacular/issues/44 — consolidated body plus
three comments carrying the evidence and the reasoning.


## Spec drafting: what the fresh-eyes review caught

The verification step's independent reviewer returned 17 findings against the assembled spec. All
were applied. Three are worth carrying into the plan:

1. **A real contradiction I introduced.** "Deleting an entry that is not there succeeds" sat
   alongside "an address the project does not have is refused", which collide on the commonest
   case. Now split explicitly: an undeclared **store or source** is the one addressing failure
   that errors; a valid address holding no document succeeds idempotently. The spec says which
   rule wins.
2. **Constraints were under-populated and other sections carried the slack.** Several hard rules
   had drifted into Requirements ("through the CLI", the addressing rule), Technical Approach
   ("add no bulk operation") and Acceptance Criteria (the site must build clean). All moved up to
   Constraints, which grew from 10 bullets to 12.
3. **Non-Goals restating a constraint in reverse is duplication, not a pattern.** Two non-goals
   were the inverse of constraints already stated and were dropped; two overlapping bulk-operation
   exclusions were merged into one.

Also: I over-wrote the Overview badly on the first pass, 419 words against a template that asks
for 2-3 sentences. The user called it out. The section templates state their required shape and
should be read as binding, not as guidance.

---

# Plan workflow (started 2026-09-21)

Planning against spec `000056_store_delete_and_knowledge_maintenance`. The spec-phase notes above
remain the richest record of *why* each decision was taken and are binding input to the plan —
particularly the refuse-while-referenced decision, the "unmet target is not stale" distinction,
and the three audit findings now in scope.

Note: `.spektacular/work/000056_store_delete_and_knowledge_maintenance/` already holds the spec
workflow's section files (overview.md, requirements.md, …). The plan's own section working files
share that directory but use distinct names (discovery.md, architecture.md, phases_plan.md,
phases_context.md, testing.md, research.md, assumptions.md).

## Plan decisions & learnings

(appended as the workflow proceeds)

### Discovery (step 2 of the plan workflow)

- **Repos in play: both.** `spektacular` (Go CLI) for every behavioural change, `docs`
  (spektacular-website, Astro 5) for the documentation requirement. Roots come from
  `go run . repo list`; never assume the working directory.
- **The spec carries no design references.** `design ref list` returns `refs: []`,
  `unresolved: 0`. The Dependencies section must say so explicitly.
- **Correction to the spec's Technical Approach.** It expects the category-README exclusion to be
  "one exclusion applied in the place that walks a store". No such single place exists: search
  walks through `store.FileStore.search` (documented as deliberately category-agnostic) while the
  always-applied payload walks through the knowledge layer's `listFiles`. It becomes one predicate
  applied at the two points where the always-applied exclusion already lives.
- **A gap the spec implies but nothing supports.** Both the delete refusal and the drift report
  must name a way to regenerate a category description. `EnsureFootprint` writes a category README
  only when it is absent, so a drifted one is repaired by no command today. Plan makes that write
  repair drift, so `init` becomes the single remedy. **Raise this at the walkthrough** — it is the
  one place the plan goes beyond the literal spec text.
- **No delete verb has ever been tested.** The three existing ones share one untested
  implementation. Characterisation tests are folded in, which is also what makes the "their
  behaviour must not change" constraint verifiable.
- **The prose layer is where most of the contract lives.** `internal/agent/instruction_surface_test.go`
  holds the hand-maintained `knowledgeSubcommands` set and the "four branches / four intents"
  literals; a fifth intent is a coordinated edit across the template and those oracles.
- **Harbor was examined and is untouched.** Only the plan-workflow suite names knowledge or design
  commands, and only in step-instruction oracles that this work does not change.
- All three audit findings reproduce live; evidence and exact commands are in
  `.spektacular/work/000056_store_delete_and_knowledge_maintenance/research.md`.
- Baseline `go test ./...` green at `8520f82`.

### Architecture (step 3)

- **Direction chosen without consulting the user**, as the step requires: two sibling delete
  commands composing new `Set.Delete` methods on the two domain packages; one category-description
  predicate owned by the category registry and applied at the two retrieval surfaces; maintenance
  as a fifth intent in `spek-knowledge`; the footprint's README write changed to repair drift.
- **Ten conventions selected**, two deliberately dropped (docs section-shading, and the glossary in
  both stores, which holds nothing but scaffolded READMEs). Rationale per entry in
  `.spektacular/work/000056_store_delete_and_knowledge_maintenance/conventions.md`.
- Still the one item to raise at the walkthrough: changing `EnsureFootprint` to repair a drifted
  category README. It is what makes the delete refusal's and the drift report's remedies true, and
  it is the single place this plan goes past the literal spec text.

### Components (step 4)

- Eleven components: eight changed in `spektacular` (knowledge set, category registry, design set,
  the two command families, the repo footprint scaffolder, the knowledge skill, the managed
  agent-guidance section, plus the repo's own reference docs) and two pages in `docs`. No new
  component: every piece extends something that already exists, which is the direction's main
  argument.
- Two placement calls recorded: the unreachable-label judgement sits on the knowledge set (it holds
  both the destination's retrieval tier and the entry's labels), and the "a category description is
  a descriptor, not an entry" statement sits in the category registry (three behaviours consult it).

### Data structures (step 5)

- **Nothing persisted changes.** No new config key, no artifact format change, settings format
  version unchanged. Everything new is in-process contracts plus two command envelopes.
- Two new refusal codes: `design_referenced_delete` and
  `knowledge_category_description_delete`. One corrective code,
  `knowledge_data_required`, replacing a bare error in the shared knowledge `--data` parser that the
  new verb would otherwise inherit. That last one changes an existing refusal's shape on
  `knowledge read`/`write` — worth a sentence at the walkthrough.
- `UnreachableTags` is a package function, not a method on the set: it consults the category
  registry and the entry's own frontmatter and touches no store.
- `IsCategoryDescription` matches `<registry category>/README.md` exactly. Supersedes the looser
  reading recorded in discovery.
- Maintenance introduces no Go type. Its vocabulary and drift report are prose composing commands
  that already publish their shapes.

### Implementation detail (step 6)

- **No new Go pattern.** Everything follows an existing shape; the one deliberate divergence is that
  the new verbs return an envelope where the three older delete verbs return silence, which the
  spec requires.
- **The footprint scaffolder's "strictly additive, never touches an existing knowledge file"
  guarantee is narrowed** to exclude the generated category description. That is the single existing
  guarantee this plan alters, and it is what makes two refusals' remedies real. Bounded: only the
  generated description, compared against the registry's own rendering, no-op when it already
  matches.
- **Resist the shared-delete abstraction.** Knowledge and design keep separate `Delete` methods; the
  packages parallel each other on purpose and the design package carries a comment saying so.

### Dependencies (step 7)

- **Design documents this plan was built on: none** — stated explicitly in the section, as the step
  requires.
- No new external dependency, no version bump, nothing that must land first. Everything this plan
  builds on (000054, 000055, 000050, 000047) is already shipped.
- Harbor recorded as examined-and-unaffected rather than unmentioned.

### Testing approach (step 8)

- **Every refusal is asserted on next-action content, never on non-emptiness.** That is the repo's
  own recorded gotcha and it is the single most repeated rule in this section.
- **Backfill decided:** characterisation tests for the three existing, never-tested delete verbs.
  Not scope creep — it is what makes the spec's "their behaviour must not change" constraint
  checkable.
- **Six success metrics mapped:** two behavioural, two manual, two split into a behavioural half and
  a manual half. Nothing dropped. The manual ones carry the exact phrase the implement workflow
  looks for.
- **Non-file provider metric is testable** via a substitute storage implementation, following the
  existing fake-store precedent in the knowledge tests and the nil-writer source in the design
  tests.
- Harbor confirmed unaffected, and the plan says so rather than staying silent.

### Milestones (step 9)

Four, in dependency order: (1) removal for both stores, with its three refusals and the
regeneration remedy; (2) category descriptions out of retrieval, plus the unreachable-label report;
(3) the maintenance intent and the standing agent rule naming removal; (4) documentation in both
repos. The registry statement of what a category description *is* lands in M1 because the delete
refusal needs it, and M2 consumes it.

### Phases (step 10)

Twelve phases across four milestones. M1: 1.1 characterise the existing removals, 1.2 name a
category description and make a drifted one repairable, 1.3 knowledge delete, 1.4 design delete,
1.5 the referenced-design refusal. M2: 2.1 retrieval exclusion, 2.2 unreachable-label report. M3:
3.1 the maintenance intent (the one High-complexity phase), 3.2 the standing rule names removal.
M4: 4.1/4.2 the two site pages, 4.3 the repo's own references.

Three things a later reader should not have to rediscover:

- **Phase 3.2 deliberately does not edit the knowledge entry** that states the same store-access
  rule. A knowledge write needs the user's per-entry agreement through the skill; a plan cannot
  authorise it. Propose it after the walkthrough instead.
- **Phase 1.1 records, but does not fix, an asymmetry**: the shared `del` command ignores `--repo`,
  so `changelog file delete --repo <name>` does not route to the member repo the way write/read/list
  do. Out of this spec's scope; noted so it is not mistaken for damage this change did.
- **The repo's own README and docs/knowledge-base.md do not enumerate the design commands at all**,
  so phase 4.3 adds the knowledge verb only. Verified by grep, not assumed.

### Open questions (step 11)

**None.** Four candidates were each resolved by reading code or running a command, and the section
records the resolutions rather than a bare "none" so the pass is visible. Notably: this project
holds **no design documents at all** (`design list` returns empty), so every design fixture in the
plan is synthetic by construction; and the selector-uniformity test asserts which stores were
reached, not entry counts, so the retrieval exclusion cannot break it.

Research's open assumption about back-link disagreement was upgraded from "assumed, stop if wrong"
to a resolved finding: the one circumstance that can leave a spec and a design disagreeing already
has its own error code telling the caller to repair by hand, and the refusal reads the same record
every other command reports from.

### Out of scope (step 12)

Six exclusions carried from the spec's Non-Goals, five the chosen design leaves to later (the
repo's own knowledge entry about store access, a shared removal helper, repo-routed changelog
removal, non-file backends, an agent-driven suite run), and two named so they are not mistaken for
omissions (no re-tiering or weakening of the always-applied exclusion; no phrase assertions over
documentation-site prose).

### Assemble (step 13)

Three documents staged to `.spektacular/tmp/` (plan_template.md 1072 lines, context_template.md
851, research_template.md 680). Metadata: 2026-09-21T09:17:27Z, commit 8520f82, branch f-migrate.

Assembly notes worth keeping: the milestones and phases working files had to be **interleaved** by
milestone number rather than concatenated (milestones.md holds only the four milestone headers;
phases_plan.md holds all twelve phases). `research.md`'s extra "Test surfaces examined" section was
moved to sit after "Files examined" so "Rehydration cues" stays last.

### Verification (step 14)

All three staged documents pass: every required section present, filled and in the required order
(`Project References` had to be moved after `Testing Strategy` to match the context.md scaffold's
order); all 12 phases carry a `**Repo:**` line, a summary, a technical-detail link and outcome-based
acceptance criteria; all 12 `context.md#phase-NM-<slug>` anchors resolve against the context
headings; no shell commands in plan.md. `research.md` carries one extra section beyond the scaffold,
`## Test surfaces examined`, placed after `## Files examined` — intentional.

### Documents committed (steps 15-17)

All three committed through `plan file write`; scratch files and the per-section working directory
removed. Next: the walkthrough, which is where the plan is actually approved. Two things to put to
the user there:

1. **Changing `EnsureFootprint` to repair a drifted category description** (phase 1.2). It is what
   makes the delete refusal's and the drift report's remedies real, and it narrows an existing
   promise never to overwrite a knowledge file.
2. **Correcting the shared knowledge `--data` refusal** (phase 1.3), which also changes the refusal
   shape of the existing `knowledge read` and `knowledge write`.

Also worth mentioning: the backfill of characterisation tests for the three never-tested delete
verbs, and that the spec's "one exclusion in the place that walks a store" turned out to be two call
sites of one predicate.

### Walkthrough (step 18) — signed off

The user reviewed the approach, the phase breakdown, the out-of-scope list and the drafting
assumptions, and approved without requesting changes. No corrections were applied, so nothing was
revealed that would warrant a knowledge capture from the review itself.

The two items raised explicitly for challenge and accepted as drafted:
- narrowing `EnsureFootprint`'s never-overwrite guarantee so a drifted category description is
  repairable, which is what makes the delete refusal's remedy true;
- correcting the shared knowledge `--data` refusal, which also changes the refusal shape of the
  existing `knowledge read` and `knowledge write`.

Plan is approved. Next action is the implement workflow against
`000056_store_delete_and_knowledge_maintenance`.

---

# Implement run — 000056_store_delete_and_knowledge_maintenance

## Validation gate (read_plan)

- Version check `match` (0.1.0). Fresh implement workflow started; no resume report.
- Repos resolved from `go run . repo list`, not assumed:
  - `spektacular` → `/home/nicj/code/github.com/jumppad-labs/spektacular`
  - `docs` → `/home/nicj/code/github.com/jumppad-labs/spektacular-website`
- Structural validation **passed**: all ten required `##` sections present (plus an extra
  `## Conventions`), 12 `#### - [ ] Phase` headings, every `*Technical detail:*` link resolving to a
  matching `### Phase N.M:` heading in `context.md`.
- Drift check **passed with no mismatches**. Every file path, Go symbol, test helper, hand-maintained
  oracle, CLI subcommand, template path and docs-page insertion point named in `plan.md` /
  `context.md` was verified to exist in the repo it belongs to. Only nit: `research.md` groups
  `Button.astro` under `src/components/sections/`; it actually lives at `src/components/Button.astro`.
  Cosmetic, in a files-examined list, not an instruction — noted, not treated as drift.
- Spec coverage check **passed**: all 15 requirements and all 17 acceptance criteria map onto phases
  (context.md carries an explicit "Requirement-to-repo resolution" line per phase). No descoped
  markers needed.
- Changelog mode: **first-phase invocation** — `plan.md` has no `## Changelog` section yet, so
  `update_changelog` creates it. Starting phase is **1.1**.

## Carry-forward decisions

- Plan chosen by the user from a two-option prompt: `000056_store_delete_and_knowledge_maintenance`.
- Phases 4.1 and 4.2 are the only `docs`-repo phases; everything else is `spektacular`.
- Baseline `go test ./...` green at `8520f82` per the plan, so any failure during this run is ours.

## Phase 1.1 — analyze

Current phase: **1.1 Pin the behaviour of the removals that already work** (`spektacular`, Low
complexity, single agent — no sub-agents spawned). All `context.md` references confirmed present;
line numbers drifted by a few lines only.

Facts established for writing the characterisation tests:

- The shared `del` arm is `cmd/storefile.go:258-270`. It calls `storeFileStore(dir)` **directly**,
  not the `resolveStore` closure the write/read/list arms use, so `changelog file delete --repo`
  ignores repo routing. Out of scope — record in a test comment, do not fix.
- `del` returns `st.Delete(...)` and writes **nothing** to stdout. Current behaviour on success is
  therefore exit 0 with empty stdout, for both the present and absent cases (`FileStore.Delete`
  swallows `fs.ErrNotExist`). That is the baseline to pin.
- Table to reuse: `kindFixtures()` (`cmd/storefile_metadata_test.go:37-62`) — three rows carrying
  `kind`, `configYAML`, `artifactName`, `storeRelPath`.
- Scaffolding: `writeSpecCommandConfig` (`cmd/spec_test.go:26`), `resetRootCmd` / `runRootCmd`
  (`cmd/root_test.go:35,70`), `setupImplementCmd` for stream capture.
- `TestSpecFileRead_MissingFileNamesResourceInError` (`cmd/file_test.go:146-165`) is the envelope /
  exit-code shape to model on; tests land appended to `cmd/file_test.go`.

## Phase 1.1 — implement

**No production code written, deliberately.** Phase 1.1's third acceptance criterion is "No
behaviour of any existing command changes in this phase" — it is a characterisation-test phase only.
The `implement` step bars writing `*_test.go`, so the whole of 1.1's work lands in the `test` step.
`go build ./...` confirmed clean before advancing.

## Phase 1.1 — test (complete)

Sub-agent wrote the characterisation tests; only `cmd/file_test.go` changed (`git diff --stat HEAD --
cmd/storefile.go` empty, confirming no production drift). Full `go test ./... -count=1` green.

Three tests, all table-driven over the existing `kindFixtures()` so spec/plan/changelog are covered
from one table, all driving the real command tree through `runRootCmd`:

1. `TestStoreFileDelete_RemovesStoredDocumentByName` — gone from disk and from `<kind> file list`,
   exit 0, and **stdout is exactly `""`**. That last assertion is the load-bearing one: it pins
   today's silence so that giving `knowledge delete` / `design delete` an envelope later (phases 1.3
   and 1.4) shows up as an intentional difference rather than an accident.
2. `TestStoreFileDelete_AbsentDocumentSucceedsAndChangesNothing` — both routes to "not there"
   (never written, and already deleted), asserting a sibling artifact's bytes and the listing are
   untouched.
3. `TestChangelogFileDelete_DoesNotHonourRepoRouting` — pins the asymmetry. Sharper than the plan
   assumed: `delete` does not merely *ignore* `--repo`, it has **no such flag at all**, so cobra
   refuses at parse time with `unknown flag: --repo`. A repo-routed changelog record therefore
   cannot be removed through the CLI today. Recorded, deliberately not fixed (the spec forbids
   changing these three verbs).

Learning to carry: the plan's phrasing "delete ignores repo routing" understates it. If anyone later
picks up the repo-routed-changelog-removal follow-up named in `plan.md#out-of-scope`, the work is
adding the flag, not rewiring an existing one.

## Phase 1.1 — verify (green)

`make build`, `make test` (shuffled, 21 packages), `make lint`, `go vet ./...` all exit 0. The
non-test diff against HEAD contains **zero `.go` files**, so criterion 3 ("no behaviour of any
existing command changes") is proven structurally rather than argued. 15 subtests pass, with spec,
plan and changelog each present in both criterion-driven tests.

Unrelated branch churn present in the working tree throughout this run (pre-existing, not ours):
`AGENTS.md`, `.claude/skills/spek-new/SKILL.md`, `.claude/skills/spek-design/` (untracked),
14 modified + 3 deleted knowledge entries, `.spektacular/state.json`, and the untracked spec/plan
for 000056. Worth remembering when the final commit is staged — those changes are from the
000055/migrate work already on `f-migrate`, not from this implementation.

## Cadence decision (user, this session)

Asked once at the phase 1.1/1.2 boundary how to handle the remaining 11 phases. The user chose
**run straight through** — drive every remaining phase without pausing, stopping only for a genuine
plan/reality mismatch, a failing verification, or a real design decision. Do not ask again per
phase; do not ask again per milestone. Report at the end.

Note this is a *stronger* authorisation than the stored `skip-confirmation-for-simple-features`
feedback, which is scoped to small features and to draft-review prompts and did not cover a
12-phase, two-repo implementation. The explicit answer here is what governs this run.

## Phase 1.2 — analyze

Current phase: **1.2 Name a category's description, and make a drifted one repairable**
(`spektacular`, Medium). Analysed in the main context rather than by sub-agents — the two touchpoints
are small and I had already read both files while checking drift. All `context.md` references
confirmed.

Two production changes:
1. `internal/knowledge/category.go` — add `CategoryDescriptionFile = "README.md"` and
   `IsCategoryDescription(path)`, beside `README()` (:105) and `AlwaysApplied()` (:117).
2. `internal/repo/footprint.go:93-100` — the README write guard becomes "absent **or** bytes differ
   from `c.README()`". Doc comment at :20-28 currently promises "existing knowledge files are never
   overwritten" and must be narrowed to exempt the generated description.

**Pre-identified test breakage (important for the test step).**
`TestEnsureFootprint_MissingREADMERepairedWithoutRewritingOthers`
(`internal/repo/footprint_test.go:45-67`) uses `knowledge/conventions/README.md` — a *category
description* — as its "must never be overwritten" sentinel. That is precisely the file this phase
makes overwritable, so the test will fail by design. Per the plan it is **extended, not replaced**:
the sentinel must move to a non-description knowledge file, its guarantee kept, and new assertions
added for drift repair and for a matching description being left byte-identical.

Also noted: both write sites (`internal/repo/footprint.go`, `internal/project/init.go:157-168`)
hard-code the literal `"README.md"`. Switching them to the new constant is what makes the registry
the single home for the name, which is the phase's stated point.

## Phase 1.2 — implement

Production changes (build + vet clean):

- `internal/knowledge/category.go` — added `CategoryDescriptionFile = "README.md"` and
  `IsCategoryDescription(path)`. The predicate normalises to slashes, strips a leading `./`, and
  matches **only** a two-segment path whose first segment resolves through `CategoryByName` and whose
  second is the description filename. A deeper README is an ordinary entry, deliberately.
- `internal/repo/footprint.go` — the description write is now "absent **or** bytes differ from
  `c.README()`". A matching description is left byte-identical and does not move the status, so a
  repeated run still reports `FootprintUnchanged`. A read error that is not not-exist is now
  surfaced rather than swallowed. Doc comment narrowed: it no longer claims existing knowledge files
  are never overwritten; it states the single exception, names it, and says why (otherwise the
  delete refusal in 1.3 would point at a remedy that does not work).
- `internal/project/init.go` — the project-tier write site now uses
  `knowledge.CategoryDescriptionFile` instead of a second hard-coded `"README.md"` literal, so the
  filename genuinely has one home. Behaviour unchanged there (it already overwrote unconditionally).

**Expected failure, confirmed and left for the test step:**
`TestEnsureFootprint_MissingREADMERepairedWithoutRewritingOthers` now fails, because its
"never overwritten" sentinel is `conventions/README.md` — the exact file this phase makes
repairable. Ran it deliberately to confirm the failure is the intended one and not a surprise.

## Phase 1.2 — test (complete)

Three test files changed, no production file touched: `internal/knowledge/category_test.go`,
`internal/repo/footprint_test.go`, `cmd/init_test.go`. `make test` and `make lint` green.

- `category_test.go` — `README()` now covered for the first time (hand-written field fragments, not
  recomputed from the renderer); `IsCategoryDescription` true for `conventions/README.md` and
  `./conventions/README.md`, false for an ordinary entry, a store-root README, an unknown directory,
  and — as its own named test for criterion 2 — `conventions/sub/README.md`.
- `footprint_test.go` — the old test was **extended, not replaced**: its "never overwritten"
  sentinel moved from `conventions/README.md` (now legitimately repairable) to a hand-written entry
  beside it, keeping criterion 5 enforced. Added a drift-repair test that snapshots the whole
  knowledge tree first, so "no other file was rewritten" is asserted against a real oracle rather
  than argued. Added a matching-description test using `os.Chtimes` to back-date the mtime — a
  deterministic way to prove the file was not written at all, with no timing race.
- `cmd/init_test.go` — the end-to-end half of criterion 3: init a project, `repo add` a sibling
  member repo, drift that repo's `conventions/README.md`, re-run `init`, assert it is back in line
  while a hand-written entry beside it survives byte-identical.

Good judgement worth carrying: the agent declined to run a mutation check because doing so would
have meant temporarily editing a production file, which its guardrail forbade. That is the right
call — but it means 1.2's tests have not been proven non-vacuous the way 1.1's were. The drift and
mtime tests are constructed so that vacuity is implausible (both would pass trivially only if
`EnsureFootprint` did nothing at all, which other tests already disprove).

## Phase 1.2 — verify (green)

`make build`, `make test`, `make lint` all exit 0. All 13 phase-relevant tests pass under
`-shuffle=on`, including the reworked `TestEnsureFootprint_MissingREADMERepairedWithoutRewritingOthers`
and the pre-existing `TestEnsureFootprint_HealthyFootprintUnchanged` (which proves the "repeated run
reports no change" half of criterion 4 still holds after the write guard changed).

Ran verification compactly in the main context rather than delegating: the step's purpose is to keep
full output out of context, which piping through grep achieves, and the evidence was already in hand
from the test step.

## Phase 1.3 — analyze

Current phase: **1.3 Remove a knowledge entry** (`spektacular`, Medium). Analysed in the main
context; all `context.md` references confirmed present at roughly the stated lines.

Integration points confirmed:
- `internal/knowledge/set.go` — `Read` (:333) and `Write` (:355) are two-liners over `s.resolve`
  (:537). `Delete` mirrors `Write` exactly and needs no refusal of its own: `resolve` already yields
  `ErrCodeTierRequired` / `ErrCodeNameRequired` / `ErrCodeStoreUnknown`, each naming the stores
  available in the tier, and `store.Writer.Delete` is contractually nil for an absent file.
- `cmd/knowledge.go` — `runKnowledgeWrite` (:450) is the shape to copy: schema branch first, then
  `knowledgeAddressData`, then `newKnowledgeSet`, then the call, then `out.WriteResult`.
  Registration is one `AddCommand` call at :615 plus a `--data` flag in `init()`.
- `knowledgeAddressData` (:566) refuses a missing `--data` with a bare `fmt.Errorf` carrying no code
  and no next action. `designAddressData` (`cmd/design.go:171`) is its compliant counterpart and the
  model for the corrective change.
- `cfg.Command` is how a runnable command name is built into a next action — see
  `cmd/storefile.go:124`. Never hard-code the binary name.

Hand-maintained oracles to move (missing either one makes the new verb silently exempt):
- `knowledgeConfigLoadingCmds` (`cmd/knowledge_test.go:1979`) — must gain `"delete"`.
- `knowledgeNarrowingCmds` (:87) — must **not** gain it; delete is addressed, not fan-out.
- `cmd/no_project_test.go` — gains a `knowledge delete` t.Run beside the existing four.

## Phase 1.3 — implement

Production changes (build + vet clean), then smoke-tested end to end against a throwaway project
built from a temp binary:

- `internal/knowledge/set.go` — `Set.Delete(addr, path)` added after `Write`. Two lines over
  `s.resolve` + `src.store.Delete`, with no refusal of its own, exactly as planned.
- `cmd/knowledge.go` — `knowledgeDeleteCmd`, `runKnowledgeDelete`, `knowledgeDeleteOutputSchema`
  (`tier`/`name`/`path`/`deleted`), input schema reused unchanged, `--data` flag registered,
  command added to the `AddCommand` call.
- `cmd/knowledge.go` — corrective: `knowledgeAddressData`'s missing-`--data` refusal is now
  `knowledge_data_required` with an example payload and a pointer to `knowledge sources`, replacing
  a bare `fmt.Errorf`. This deliberately changes the refusal shape of `knowledge read` and
  `knowledge write` too; confirmed live that both now carry the code and next action.

**A real defect caught by smoke-testing rather than by the plan.** My first version of the
category-description refusal named `` `spektacular init` `` as the next action. `init` takes the
agent as a required argument, so that step is **not runnable** — exactly the failure the project's
error-remediation convention exists to prevent, in the one refusal whose whole job is to point at a
working remedy. Fixed to name the agent the project already records (`cfg.Agent`, falling back to
`<agent>`), giving `` `spektacular init claude` ``, and verified that command runs and exits 0.

Carry-forward: the other sites that name init (`cmd/version.go:67`, `cmd/migrate.go:95`,
`cmd/root.go:297`) use the `<agent>` placeholder, which is right for them — they fire when no agent
is recorded or no project exists. A refusal raised *inside* a configured project has the agent in
hand and should name it. Worth applying to the `design delete` refusal in 1.5 if it points anywhere
similar, and worth asserting on content in the tests rather than on non-emptiness.

Smoke results, all as specified: schema published; write→delete reports `deleted:true`; repeat
reports `deleted:false` and exits 0; undeclared store refused with the tier's store names listed;
category description refused with the file still present.

## Phases 1.4 and 1.5 — implement (done together)

Implemented while 1.3's test agent ran, because 1.4/1.5 touch entirely disjoint files
(`internal/design/`, `cmd/design.go`) — the plan explicitly notes 1.3 and 1.4 can run in parallel.
1.5 is folded in as one edit to the same handler, since its refusal is a guard inside
`runDesignDelete` rather than a separate surface.

**1.4** — `internal/design/design.go` gains `Set.Delete(d)` following `Write`'s sequence exactly
(lookup → empty-path → nil-writer → store call), with an explicit note that it must never learn
about lifecycle records or specs. `cmd/design.go` gains `designDeleteCmd`, `runDesignDelete`,
`designDeleteOutputSchema` (source/path/location/deleted), `--data`, and registration. `deleted` is
settled by the existing `set.Exists` — the same reason the write path already calls it.

**1.5** — `refuseIfReferenced` guards the delete, reached only when the document exists. Reads the
lifecycle record via `authoredMetadata` (not `metadata.Split`, deliberately: it swallows a parse
error so a team's own YAML header reads as unauthored rather than erroring). `fm == nil` or an empty
`Specs` falls straight through. Two small helpers, `specList` and `pluralSpecSubject`, keep the
message grammatical for one spec or several while still naming every one of them.

Smoke-tested the whole sequence live against a throwaway project: authored a design, referenced it
from two specs, confirmed the delete is refused naming **both** specs with a runnable
`design ref remove` for each; the document was still on disk and both specs' references intact;
cleared both refs; the same delete then succeeded; `design ref list` afterwards reported
`unresolved: 0`.

Note for the 1.5 tests: the refusal's `location` is the **resolved absolute path**, not the store
path, matching `design_authored_overwrite`. Tests should assert on the path's suffix or on the spec
names, not on a hard-coded absolute prefix.

## Phase 1.3 — test + verify (green)

Three test files: `internal/knowledge/set_test.go`, `cmd/knowledge_test.go`,
`cmd/no_project_test.go`. `make build`/`test`/`lint` all 0; all 8 phase tests pass shuffled.

Two things the agent got right that are worth carrying:
- It added `agent: claude` to the `twoScopeProject` fixture so the category-description refusal's
  next action is built from a real agent, and then asserted the next action contains
  `` `spektacular init claude` `` — so the runnable-next-action bug I hit during implementation is
  now held by a test rather than by memory.
- It checked whether the `--data` refusal change broke any existing assertion and found it did not:
  only the *code* and *next action* were added, the message text `--data is required` is unchanged,
  so `TestKnowledgeRead_MissingDataEmitsErrorEnvelope` still holds. Worth knowing — the corrective
  change was strictly additive at the message level.

## Phases 2.1 and 2.2 — implement (done together)

Implemented while 1.4/1.5's test agent ran; disjoint files again.

**2.1** — `IsCategoryDescription` applied at exactly the two places the always-applied exclusion is
already applied: the post-merge filter in `Set.Search` and the file loop in `readCategories`.
Confirmed the search exclusion sits **before** the cutoff floor is computed from `eligible[0].Score`,
so a description can never set the bar and then be dropped, silently raising the threshold for
everything else. `listFiles`, `Set.List`, `Set.Tags` and `internal/store` deliberately untouched.

**2.2** — `TagReachability` + `UnreachableTags(path, content)` in `category.go`, rendered onto the
write result by `runKnowledgeWrite` **after** the write succeeds, so it cannot become a gate.
`knowledgeWriteOutputSchema` gained both fields as optional.

**Verified against this project's own real knowledge base**, which is where the three audit findings
were originally observed:
- `knowledge search "purpose belongs elsewhere entry shape"` previously returned 8 hits, every one a
  category README. Now: 3 hits, **zero** READMEs — real entries surface instead.
- `knowledge always-applied --tier repo --filter spektacular --filter docs` previously injected 2
  READMEs per store into the payload sent on every task. Now: **zero**.
- `knowledge list` still reports all 12 READMEs, which is exactly what phase 3.1's drift report
  needs.

Unreachable-tag report smoke-tested across all four cases: reported with a runnable next step for an
always-applied destination, silent for a looked-up one, silent for an entry with no tags, and the
entry written byte for byte either way.

Two small lint-driven cleanups made along the way: `slices.Contains` instead of a hand-rolled loop,
and `gofmt` after an import edit. Note `strings.Title` in `Category.README()` carries a pre-existing
`//nolint:staticcheck` and was left alone — not ours.

## Phases 3.1 and 3.2 — implement (done together)

**3.1** — `templates/skills/workflows/spek-knowledge/SKILL.md` gains `# Intent: maintenance` between
the audit intent and `# Decline handling`, carrying: the four verdicts; the classification rule
stated *as a rule* with the concrete failure mode worked through (an architecture entry the code
disagrees with is `current`, and proposing its removal would delete the entry doing the most work);
the evidence requirement with "looks old" explicitly named as a non-finding; the
category-description drift step (list → read → compare against `knowledge categories`, report naming
`init <agent>`, never repair); and per-entry propose-then-confirm with removal only after agreement
for that entry.

Coordinated preamble edits, because a branch the preamble does not advertise is never reached:
"one of four branches" → five (naming maintenance), "all four intents" → five, the
"What this skill does" sentence, and a natural-language trigger.

**3.2** — `templates/agents/store-access.md` gains a paragraph stating removal is a CLI verb,
naming `knowledge delete` and `design delete` plus the `delete` the three file families already
have, saying `rm` is never correct for a managed file, and telling the agent to act on a refusal
rather than reach past it. The three write-it-yourself exceptions are untouched.

Regenerated the dogfooded copies with `go run . init claude` — note **`migrate` is a no-op when the
installed and current versions match** (`status: up_to_date`, `reinstall: false`), so it does not
regenerate skills during development. `init <agent>` is what actually re-renders them. The plan said
migrate; that is true for a version bump, not for an in-place template edit. Worth knowing for any
later prose phase.

Hand-maintained oracles updated (all four the plan predicted, plus one it did not):
- `knowledgeSubcommands` gained `"delete": true`.
- `expectedCRUDInvocations` gained `knowledge delete`.
- The branch-count contract now asserts five and **bans every superseded count by name**
  (three *and* four), so the next intent cannot leave a stale sentence behind.
- `TestRenderedStoreAccessSectionNamesEveryStoreCommand` gained the two removal needles.
- **Not predicted:** `TestRenderedSpekKnowledgeAuditConfirmsPerEntry` pins the literal "In the audit
  intent the same rule applies **per entry**", which I broadened to "In the audit and maintenance
  intents…". Updated to match. The plan anticipated this test needing its scope revisited; it was
  the sentence itself that moved.

Full `go test ./...` green after the oracle updates.

Still to do for milestone 3: the additive corpus-wide negative sweep asserting nothing instructs an
agent to remove a managed file with a raw file operation. Survey done — **every `rm` in
`templates/` targets `.spektacular/tmp/`**, so banning store-path literals
(`rm .spektacular/specs/`, `/plans/`, `/knowledge/`, `/changelog/`, and the design store) is safe
and will not false-positive on the legitimate scratch-file lines.

## Phases 2.1 and 2.2 — test + verify (green)

Three test files; `make build`/`test`/`lint` all 0. The agent **mutation-checked** its work: it
temporarily disabled each production predicate, confirmed the new tests fail, and restored the files
in the same shell command — so milestone 2's tests are proven non-vacuous, unlike 1.2's.

Two judgements it made that were better than my brief:
- It put the `UnreachableTags` unit tests in `category_test.go` rather than `set_test.go`, because
  the function lives in `category.go` beside `IsCategoryDescription`, and co-location is the stated
  convention. Correct — my brief said `set_test.go` out of habit.
- It deliberately did **not** seed descriptions into the `narrowingSet` fixture, because several
  other tests hold exact counts against it (`Len(all, 8)`). It confirmed
  `TestSet_SelectorCoverageMatrixIsUniformAcrossRetrievalPaths` is unaffected by *reading its
  fixture*, which is what the plan asked for ("confirm rather than assume").

One honest caveat it flagged: `TestSet_TagsAreUnchangedByTheDescriptionExclusion` gives its README
fixtures frontmatter, which a real scaffolded description never has. Artificial by necessity — labels
are the only thing `Tags` reports — and the comment says so.

## Milestone 4 — implement (4.1, 4.2, 4.3)

**4.3 (`spektacular`)** — `README.md` gains a `knowledge delete` bullet; `docs/knowledge-base.md`
gains a command-reference row and a new "Reviewing whether entries are still true" subsection beside
the tag-audit one, naming the four verdicts, the unmet-target-is-current rule, and the drift report.
Neither document enumerates the design commands, so neither gains `design delete` — as the plan
says, do not invent a section to hold it. `cmd/docs_test.go` sweeps still pass.

**4.1 (`docs`)** — `src/pages/knowledge-base.mdx`: the lifecycle `sub` slot now says removal is part
of an entry's life, and a **Removing** paragraph lands after "Keeping it up to date", covering the
undeclared-store refusal, the path-holding-nothing success, the `deleted` distinction, the
category-description refusal, and that recovery is whatever git gives you. Matched this page's own
house style (bolded lead-in, spaced JSON, `--file`), which differs from the design page's.

**4.2 (`docs`)** — `src/pages/design-documents.mdx`: removal lands after the author prose and before
the reference verbs, carrying the delete-versus-remove-a-reference distinction as the bridge into
them, and explicitly naming the trap (anyone looking for a delete meets `design ref remove` first
and it appears to succeed). The `design_referenced_delete` refusal is shown in
`When a reference cannot be found`, in the same JSON shape that section already uses for
`design_not_found`. Matched this page's compact JSON spacing and `--from`, not the other page's.

**Gates**: `npm run build` clean (13 pages); `make check` reports **0 errors, 0 warnings**, 1 hint —
and that hint is a pre-existing `document.execCommand` deprecation in `src/layouts/Shell.astro:63`,
not ours.

**Convention check**: the `docs` repo bans em dashes. Verified my additions carry none, and in fact
both whole pages contain zero — so the convention is intact rather than merely not-worsened.

## Phases 3.1 and 3.2 — test + verify (green)

Two test files, 13 new tests, all passing shuffled; `make build`/`test`/`lint` all 0.

The sweep was mutation-checked two independent ways, neither touching a template: a table-driven
test over the extracted predicate with synthetic offending and legitimate strings, and a transient
ban-list mutation (adding `.spektacular/tmp` to the target list) proving the walk-and-assert wiring
fires, then reverted and byte-compared.

**The sharpest finding of the whole run**: a bare `rm ` ban would false-positive on the word
**`confirm`** — `confirm .spektacular/specs/…` literally contains `rm .spektacular/specs/`. The
sweep anchors each removal verb on `(^|[^A-Za-z])` for exactly this. This is the concrete form of
the plan's open question about whether the sweep could be written without flagging legitimate prose;
the answer is yes, but only with word-boundary anchoring, not merely by choosing a distinctive path.

Two deliberate scoping calls worth keeping:
- Bare `Remove(` is **not** banned, only `os.Remove(` / `os.RemoveAll(` — the bare form is too
  generic to be safe in English prose.
- The design store has no fixed default directory (locations are config-declared), so
  `.spektacular/design` is the literal used. A relocated store is still reached by the same CLI
  verbs, so banning the default spelling is what catches an *instruction* being written, which is
  what the sweep is for.

## test_plan (written)

`test-plan.md` written to the plan store via the CLI. Four procedures, one per success metric with a
manual component: the two wholly-manual ones (a knowledge base staying accurate over months; a
review's findings holding up when spot-checked) and the manual halves of the two split ones (agents
not reaching past the CLI; the referenced-design refusal being acted on).

Deliberately excluded the two fully-automated metrics, naming the tests that cover them instead, so
nobody re-runs by hand what the suite already proves.

Two practical notes baked into the procedures: this project holds **no design documents at all**, so
metric 2's first run needs a fixture built by hand; and metric 4 names the specific failure to watch
for — a review proposing removal of an entry that states a target the code has not met — because
that is the one outcome that would make this feature actively harmful, and the fix would be
sharpening the skill prose rather than deleting the entry.

## update_feature_changelog (written)

Three records, all via the CLI: the project-level record plus one per affected repo
(`--repo spektacular`, `--repo docs`). Provenance stamped automatically into each repo record's
front matter (project / spec / plan); the readable reference line is in the body.

Both repos were affected, so both got a record — no "the project-level already covers it" carve-out.

## reconcile_spec (done)

All 15 requirements and 17 acceptance criteria flipped to `[x]` — 32 in total, none left unchecked.
Every one maps to a shipped phase with tests: nothing was descoped, deferred or dropped during
implementation, and no checkbox was flipped on a partial match.

## finished — workflow complete

All 12 phases checked off, 12 phase changelog entries, test plan written, three feature changelog
records (project + both repos), spec reconciled with all 32 checkboxes satisfied.

**The feature repaired a real drift while running.** Running `init claude` to regenerate the skills
also brought `docs:.spektacular/knowledge/architecture/README.md` back into line — it was carrying
the pre-000037 Purpose and Belongs-elsewhere text, which is the exact live instance the plan's
research cited as unrepairable by any command. Phase 1.2 is why it repaired. That diff in the `docs`
repo is the feature working, not stray editing.

Final gates: `spektacular` build/test/lint all 0; `docs` build and `make check` both 0.
