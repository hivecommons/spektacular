---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# Plan: 000059_normalise-artifact-addressing

<!-- Metadata -->
<!-- Created: 2026-09-25T12:27:46Z -->
<!-- Commit: 82ceb1b -->
<!-- Branch: f-normalize -->
<!-- Repository: jumppad-labs/spektacular (+ docs: jumppad-labs/spektacular-website) -->

## Overview

Today each kind of document a feature produces wants its own spelling: a spec takes `<name>.md`, a plan takes a joined `<name>/plan.md`, and a changelog record takes `<name>.md`. The locations the list commands report follow three different conventions, and some leak absolute host paths. This plan makes the feature's bare name, the one every workflow already records, the address for its spec and its changelog record. A plan document is addressed by that name plus a document name. Old spellings are refused with the correct command, and every reported location becomes relative to the configuration file that declares the store. Agents and external orchestrators such as Hive can then go from a single recorded name to any of a feature's documents with no per-store string handling. The shipped skills, step instructions, harbor suites and docs all move to the new addressing in the same change.

## Conventions

- **Error messages must describe the problem and suggest remediation** (spektacular) — both new refusals (`unexpected_extension`, `document_required`) and the upgraded `not_found` must carry a runnable `next_action` that restates the same command correctly spelled; tests assert the next_action content, not just that it is non-empty.
- **Spektacular's own files are written through Spektacular** (spektacular) — this feature changes the very CLI spellings that convention's instructions and every template use, so every template and the knowledge entry's examples must use the new addressing, and plan/spec/changelog reads during implementation go through the CLI.
- **Tests must not depend on order** (spektacular) — the many rewritten `cmd` tests execute the shared cobra tree; they must go through `runRootCmd`/`resetRootCmd`, especially now plan verbs change arity.
- **Passing tests are required before calling work done** (spektacular) — ~80 CLI test invocations and ~20 template assertions change spelling; the full shuffled test suite must be green before any milestone is reported done.
- **MDX authoring conventions** (docs) — the new document-command reference page and configuration.mdx edits must use named-block components and fenced code blocks, with no layout HTML in page bodies.
- **Site layout conventions** (docs) — the new page is composed from existing `src/components/sections/` components (`Hero`, `Section`, `Prose`, `ConfigurationKeys`) and added to the Resources nav.
- **Alternate section background shading** (docs) — sections on the new page alternate `surface` explicitly.
- **Label before filename in file-scoped reference headings** (docs) — the command reference headings lead with a plain-language label (for example "Specs: spec file"), not a bare command name.
- **Plans must sketch content structure** (docs) — the docs tasks carry a Content outline/example with the exact commands, fields and error codes verified here.
- **No em dashes** (docs) — all authored docs prose, README/CHANGELOG entries and commit messages avoid em dashes.

## Architecture & Design Decisions

**Shape.** All fifteen spec/plan/changelog document verbs are built by one function, `newStoreFileCmd` (`spektacular:cmd/storefile.go:181`), which today joins `args[0]` straight onto the store directory. The change introduces a small, store-agnostic **artifact address** layer, a new package `spektacular:internal/artifact`, and makes that builder address-driven. An `artifact.Address` is `{Kind, Feature, Document}`; `artifact.Parse(kind, args)` is the only validator of what a caller typed, and `Address.StorePath(dir)` is the only place the file provider's layout (`<dir>/<feature>.md`, `<dir>/<feature>/<document>.md`) is spelled out. Its inverse, `artifact.NameFromEntry`, turns a listed entry back into the bare name the list prints. The existing layout helpers (`spec.SpecFilePath`, `plan.PlanFilePath`/`ContextFilePath`/`ResearchFilePath`, `implement.ChangelogFilePath`) delegate to it, so `.md` is appended in exactly one place and every workflow step keeps the file names it already writes. Spec and changelog verbs take one argument, the feature. Plan document verbs take two, `<feature> <document>`, and `plan file list [<feature>]` lists features, or one feature's documents, by bare name. The `--repo` routing, ID-prefix check, provenance stamping and plan-body validation all keep working, but they are fed the parsed address rather than a raw path.

**Refusals live beside the facts.** Per `gotchas/remediation-needs-the-layer-that-holds-the-facts.md` and `conventions/error-messages-must-suggest-remediation.md`, the refusal is raised in the command builder. That builder is what knows the kind, the verb, the flags the caller set and the configured command name, so it can rebuild *the same command, correctly spelled* as the `next_action`. `artifact.Parse` returns typed errors, and the builder renders them into the envelope. Two new documented codes follow the domain const-group pattern (`spektacular:internal/knowledge/address.go:58-74`): `artifact.ErrCodeUnexpectedExtension = "unexpected_extension"` and `artifact.ErrCodeDocumentRequired = "document_required"`. Because feature names are `^[a-z0-9_-]+$` (`spektacular:cmd/spec.go:22`), any `.` in an address segment is an extension, and any `/` is a joined path. Both are refused with `unexpected_extension`. For a plan, `feature/doc.md` is split so the next action can show `plan file read <feature> <doc>`. A plan document verb given only a feature is refused with `document_required`. Its next action names `plan file list <feature>` and gives an example read. The existing `not_found` refusals gain a next action that points at the matching list command, and a missing plan directory on `plan file list <feature>` becomes a `not_found` instead of a raw store error. No refusal writes anything, and no message carries a host path.

**Name is the address, path is the location.** `list` reports `name` as the bare address and `path` as the storage location relative to the folder holding the configuration file that declares the store. For the central stores that is the `.spektacular/` folder (`config.ProjectConfigDir`, `spektacular:internal/config/config.go:260`), so the location becomes `specs/x.md`, `plans/x/plan.md` or `changelog/x.md`. For a `--repo` changelog it is the repo.yaml folder, whose store-relative path (`changelog/<project>/x.md`) is already in that shape. `resolveStore` therefore returns a location base alongside the store, and `list` computes `path` from it. This replaces today's no-op `TrimPrefix(…, st.Root())` (`spektacular:cmd/storefile.go:318-356`). Directory entries in spec/changelog listings, and non-document files, are not addresses and are dropped. `set-document-status` output follows the same split: `name` (plus `document` for a plan) and a config-relative `path`. The same rule reaches the workflow commands' own output. Documents Spektacular owns may not be on disk, so a host path is never something a caller can use. The `new`, `goto` and status results report the address plus a config-relative location. Step instructions never render a file path to a store document; they name it by address and give the CLI read (`conventions/store-files-must-be-written-through-the-cli.md`). The absolute path template variables (`spec_path`, `plan_path`, `context_path`, `research_path`, `changelog_path`, `plan_dir`) are removed from the step strategies, so no template can reintroduce one. The workflow-status forms of `plan status` and `implement status` report `plan_name`, a new `plan_document` (`"plan"`) and a config-relative `plan_path` in place of today's absolute one (`spektacular:cmd/plan.go:264`, `spektacular:cmd/implement.go:363-365`).

**Callers move in the same change.** This is a hard break with no accepted old spelling, so everything shipped moves with it:

- **Templates:** every template under `templates/` (skills, step templates and `partials/implement-plan-documents.md`).
- **Go-built instruction:** the `walkthroughRevisionHint` string (`spektacular:internal/workflow/workflow.go:239`).
- **Harbor suites:** instructions, `solve.sh` and verifier oracles, per `architecture/testing-architecture.md`.
- **Documentation:** the README and CHANGELOG in the CLI repo, and the configuration reference, a new document-command reference page and a migration section on the docs site (`docs:src/pages/`).

The installed skill copies are regenerated with `go run . init`, never hand-edited. The instruction-surface deny-list (`spektacular:internal/agent/instruction_surface_test.go`) is widened to cover `partials/` and `agents/` and to forbid the old spellings, so a regression fails in CI. This beats both alternatives. Tolerating old spellings would break the "listed name == accepted name" rule and the spec's hard-break constraint. Pushing addressing into `store.Store` would widen a backend contract for a naming concern. The evidence is in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Artifact address (new, spektacular).** Owns what an address is and how it maps onto the file provider's layout. It parses what a caller typed for a kind (spec, plan or changelog) into a feature and, for a plan, a document. It refuses a segment carrying an extension or a path separator, or a plan document verb given only a feature, with typed errors carrying the corrected address. It maps a valid address to its store path and a listed store entry back to its bare name. It defines the two new error codes. It is the single place that knows documents are persisted as `.md` files. The command builder and the workflow step layout helpers both depend on it. It depends on nothing but the standard library.
- **Store document command builder (changed, spektacular).** It already builds the write, read, delete, list and set-document-status verbs for all three stores, and it keeps doing so with a per-kind arity: one argument for spec and changelog, two for plan documents. It calls the artifact address to parse every argument. It turns parse errors into refusals whose next action restates the same command, with the flags the caller set, correctly spelled. It gives `not_found` a next action pointing at the matching list command. It feeds the parsed address to the existing ID-prefix check, provenance stamping and plan-body validation. It reports every `name` as a bare address and every `path` as a location relative to the folder of the configuration file that declares the store. It asks the store resolver for that location base.
- **Store resolver (changed, spektacular).** It picks the central project store or a named repo's changelog store, as today, and additionally returns the location base that reported paths are made relative to. For the central stores that is the project settings folder holding config.yaml. For a repo-routed changelog it is the folder holding repo.yaml.
- **Plan document validator (changed, spektacular).** It still refuses a malformed plan body before anything is written. It now recognises the plan document by its address (document `plan`) rather than by a `plan.md` file name.
- **Workflow step layout helpers (changed, spektacular).** The spec, plan and implement step packages keep their typed path helpers, but these now delegate to the artifact address, so steps write exactly the documents the CLI reads by name. Their outputs are unchanged.
- **Step path strategies (changed, spektacular).** The spec, plan and implement strategies stop producing absolute path template variables. They provide the document's address (names) for templates, and a config-relative location for the workflow result, which the shared step-result builder reports.
- **Status reporters (changed, spektacular).** The workflow-status forms of `plan status` and `implement status` report the plan by address (`plan_name`, a new `plan_document`) and by config-relative location (`plan_path`), in place of today's absolute host path. They read the plan through the same store path as before.
- **Walkthrough revision hint (changed, spektacular).** The workflow engine's plan-revision hint tells the agent to rewrite a plan document with the two-argument write form.
- **Shipped agent instructions (changed, spektacular).** The workflow skills, step templates, the implement plan-documents partial and the harbor e2e suites (instructions, scripted solutions, verifier oracles) use the new addressing. The installed skill copies and managed AGENTS.md sections are regenerated from the templates, never edited.
- **Instruction-surface guard (changed, spektacular).** The existing template deny-list test is widened to cover the partials and managed-agent templates. It also forbids the old spellings, so an instruction that would produce a refused command fails CI.
- **CLI repo documentation (changed, spektacular).** The README documents the addressing rules, the location convention and the new status fields. The CHANGELOG carries a breaking-change migration note mapping each old spelling to the new one.
- **Document command reference page (new, docs).** A new site page is the command reference for spec, plan and changelog document commands. It covers the addressing rules, list output, both error codes and a migration section for external callers. It is linked from the Resources navigation and built from the existing section components.
- **Configuration reference and plan-tasks page (changed, docs).** The spec, plan and changelog store keys document that reported locations are relative to the folder of the declaring configuration file. The plan-tasks page's status example shows `plan_name`, `plan_document` and the relative `plan_path`.

## Data Structures & Interfaces

**`artifact.Kind` and `artifact.Address`** (new). An address is what a caller passes and what a list prints. It never carries an extension or a path separator, and it says nothing about how a store persists the document.

```go
type Kind string // "spec" | "plan" | "changelog"

type Address struct {
    Kind     Kind
    Feature  string // bare feature name, e.g. "000059_normalise-artifact-addressing"
    Document string // plan only: "plan", "context", "research", "test-plan", ...; empty otherwise
}

// Parse validates positional arguments for a document verb of kind.
// Spec/changelog: exactly [feature]. Plan: [feature, document].
func Parse(kind Kind, args []string) (Address, error)

// StorePath is the file provider's location of the addressed document
// under a store directory: <dir>/<feature>.md or <dir>/<feature>/<document>.md.
func (a Address) StorePath(dir string) string

// FeatureDir is <dir>/<feature>, used by `plan file list <feature>`.
func FeatureDir(dir, feature string) string

// NameFromEntry turns a listed file entry into its bare address segment;
// ok is false for entries that are not addressable documents.
func NameFromEntry(entryName string, isDir bool, want EntryKind) (name string, ok bool)
```

**Refusal errors** (new, typed so the command layer renders the envelope and next action):

```go
const (
    ErrCodeUnexpectedExtension = "unexpected_extension"
    ErrCodeDocumentRequired    = "document_required"
)

// ExtensionError: the input carried an extension or a joined path.
// Corrected holds the correctly spelled address the next action restates.
type ExtensionError struct { Input string; Corrected Address }

// DocumentRequiredError: a plan document verb got only a feature.
type DocumentRequiredError struct { Feature string }
```

**Store resolver contract** (changed). It returns the location base alongside the store and directory:

```go
// base is store-relative: the directory reported paths are made relative to
// (".spektacular" for central stores, "" for a repo-routed changelog store).
resolveStore(repoName string) (st store.Store, storeDir string, base string, err error)
```

**List output** (changed wire format, same keys). Every entry is `{ "name": <bare address segment>, "path": <location relative to the declaring config folder>, "modified_at"?, "created_date"?, "document_status"?, "closed_date"? }`.

- `spec file list` and `changelog file list [--repo]`: `name` is the feature; `path` is e.g. `specs/<feature>.md`, `changelog/<feature>.md`, or with `--repo`, `changelog/<project>/<feature>.md`.
- `plan file list`: `name` is the feature; `path` is `plans/<feature>`.
- `plan file list <feature>`: `name` is the document; `path` is `plans/<feature>/<document>.md`.

**`set-document-status` output** (changed): `{ "name": <feature>, "document"?: <document, plan only>, "path": <config-relative location>, "document_status", "closed_date"? }`. Previously `{ "path": <raw argument>, ... }`.

**Status results** (changed). `plan.StatusResult` and `implement.StatusResult` gain `PlanDocument string \`json:"plan_document"\`` (always `"plan"`). `PlanPath` changes meaning from an absolute host path to a location relative to the config folder (`plans/<feature>/plan.md`). The JSON output schemas for both status commands are updated to match.

**Workflow results** (changed). `spec` `new`/`goto`/`status` results keep `spec_name`, and `spec_path` becomes config-relative (`specs/<feature>.md`). `plan` and `implement` `new`/`goto` results keep `plan_name`, gain `plan_document` (`"plan"`), and `plan_path` becomes config-relative. The step strategy contract swaps the absolute path variables for a config-relative primary location plus the address names.

**Error envelope** (unchanged shape). Both refusals use the existing `output.ErrorResponse` with `code`, `message`, `resource` (the address as typed) and `next_action`. No store interface (`store.Store`, `Reader`, `Writer`) changes.

## Implementation Detail

**A new module boundary: addresses versus storage.** Today the document commands, the step packages and a handful of command handlers each spell out the file layout on their own, by appending `.md` or joining a plan folder. The plan introduces one small leaf package that owns the address grammar and the layout mapping. Everything that names a spec, plan document or changelog record, whether a CLI verb, a workflow step or a status report, goes through it. A reader of the command builder sees "parse the address, resolve the store, act on `address.StorePath(dir)`", not string surgery on `args[0]`. A reader of a step package sees its familiar typed helper, now a one-line delegation. This follows the existing pattern of a domain package owning its address grammar and error-code constants, as the knowledge address module and the design errors module already do.

**Address-driven command builder.** The single builder that serves all three stores stays single. It gains a per-kind description (its kind, its arity, whether it is repo-routed, whether it requires an ID prefix, and its body validator) in place of today's positional boolean parameters. The five verbs read that description instead of branching on booleans. Every verb follows the same three-stage shape:

1. Parse the address, or refuse.
2. Resolve the store and location base.
3. Act and report `name` and `path` separately.

Only the arity differs, and only for plans. Cobra's arity check is relaxed for plan verbs so that a one-argument call reaches the parser and receives the actionable `document_required` refusal rather than cobra's generic "accepts 2 arg(s)" message.

**Refusals that restate the command.** A new, reusable helper in the command layer renders the typed address errors into the standard error envelope. It rebuilds the invoked command from the configured command name, the command path, the corrected positional arguments and the flags the caller actually set, so the next action is the same command, correctly spelled, and can be run as-is. This is the concrete application of the remediation convention, and the knowledge gotcha about where validation lives: the parser holds the grammar, and the command layer holds the facts the next action needs. The same helper backs the upgraded `not_found`, which points at the matching list command.

**Location reporting is derived, never stored.** A reported location is computed at output time from the store-relative path and the location base the resolver hands back. The location base is the folder that holds config.yaml for the central stores, and the repo.yaml folder for a repo-routed changelog. The configured directory value is never read back verbatim, per the store-dirs gotcha. Listing uses one path-rendering routine for every kind, with and without `--repo`, which is what makes the convention uniform. Status reporting uses the same routine.

**Instructions move with the grammar, guarded by tests.** Templates are rewritten mechanically: `{{plan_name}}/plan.md` becomes `{{plan_name}} plan`, `{{plan_name}}.md` becomes `{{plan_name}}`, and prose that talks about a spec "found under `<name>.md`" is reworded to the bare name. The regression guard follows the existing deny-list pattern and extends its reach to the partials and managed-agent templates. A whole-corpus check over the rendered agent-facing text (the existing contract harness) asserts that no rendered instruction addresses a document with an extension or a joined path. The harbor oracles are hand-maintained and are updated as literals, never derived from templates, per the testing-architecture knowledge.

**Documentation follows existing site patterns.** The new reference page is composed from the existing `Hero`, `Section`, `Prose` and `ConfigurationKeys` components. Error codes use the `ConfigKey` pattern already used for plan-task output fields. The migration note reuses the CHANGELOG's established `**Breaking change**:` paragraph shape. No new site components are introduced.

## Dependencies

- **Design documents this plan was built on: none.** Spec 000059 carries no design references (`design ref list` reported zero refs, zero unresolved). The command shapes come from the spec's Technical Approach and GitHub issue hivecommons/spektacular#46.
- **`internal/store` (spektacular)**: the backend-agnostic `Store`/`Reader`/`Writer` interfaces every document verb reads and writes through. Used as-is, no changes.
- **`internal/config` (spektacular)**: supplies the configured store directories (already project-root-relative) and the settings-folder name that reported locations are made relative to. Used as-is.
- **`internal/repo` (spektacular)**: resolves a named repo for `--repo` changelog routing and its repo.yaml folder. Used as-is.
- **`internal/output` (spektacular)**: the error envelope (`NewError`, `WithResource`, `WithNextAction`) that both new refusals use. No changes.
- **`internal/metadata` (spektacular)**: front-matter merge and split on write and set-document-status. Unchanged, but now fed the parsed address.
- **`internal/steps/spec`, `internal/steps/plan`, `internal/steps/implement` (spektacular)**: their typed layout helpers change to delegate to the new artifact address package; their outputs must stay byte-identical.
- **`internal/agent` skill installer (spektacular)**: renders the installed skills and managed AGENTS.md sections from templates. It is used unchanged via `go run . init claude` and `go run . init bob` to regenerate the copies. Regeneration also pulls in pre-existing template drift in those copies.
- **`github.com/spf13/cobra`**: command arity for the plan verbs changes from `ExactArgs(1)` to a range. No version change.
- **Harbor e2e harness (`tests/harbor/*`, spektacular)**: requires the `harbor` CLI, Docker and Claude credentials to run the spec, plan and implement suites that verify a full run hits no addressing refusal. It does not run in CI.
- **docs repo (spektacular-website, Astro 5 + MDX)**: existing section components (`Hero`, `Section`, `Prose`, `ConfigurationKeys`, `ConfigKey`) and the Resources navigation. Docs changes land on its `f-normalize-commands` branch and must build and type-check cleanly.
- **Prior work**: builds on spec/plan 000058 (plan task graph, already released), whose status and task code is touched by the status changes, and on 000047/000050 (knowledge addressing and config-relative locations), which established the "name is the address, location is relative to the declaring config" model this plan extends. Nothing must land first.
- **External consumer**: Hive (hivecommons/hive#8227) will adopt the new addressing once released. It is not a build dependency.

## Testing Approach

Testing follows the project's three layers: Go unit tests, template-contract tests and harbor end-to-end suites. The load-bearing layer is the CLI behaviour tests in `cmd`, which exercise the real command tree through the shared `runRootCmd` helper and are shuffle-safe.

**Unit tests: the artifact address package.** This is table-driven coverage of the address grammar, because every other component trusts it:

- Bare names parse for each kind.
- A `.` or `/` in any segment is refused as an extension error, with the correct corrected address. That includes `feature.md`, `feature/plan.md` passed to a plan verb, `feature.markdown` and `specs/feature`.
- A plan verb with one argument is refused as document-required.
- `StorePath` produces the exact file-provider layout.
- `NameFromEntry` round-trips it.

The expected values are hand-written literals, never computed from the code under test.

**CLI behaviour tests: the round-trip and refusal guarantees.** For each of spec, plan and changelog, with and without `--repo` for changelog, the tests guarantee:

- **Round-trip.** Every `name` a list prints, passed unchanged to read, returns that document's content. No listed name ends in an extension.
- **Same address for every verb.** Write, delete and set-document-status given the same bare names act on the document read returns.
- **Extension refusal.** Every verb given `x.md`, and every plan verb given `feature/plan.md`, returns `unexpected_extension` and leaves the store byte-identical. The test asserts the exact `next_action` text: the same command, correctly spelled, with the caller's flags.
- **Document required.** A plan document verb given only a feature returns `document_required`, not an internal error. Its message and output contain no absolute path, and its `next_action` names `plan file list <feature>` and an example read.
- **Relative locations.** Every list `path` starts with the configured store directory, and contains neither the settings-folder prefix nor an absolute path. That holds for spec, plan, a plan's documents, and changelog with and without `--repo`.
- **Status.** `plan status` and `implement status` report `plan_name`, `plan_document: "plan"` and a relative `plan_path` with no absolute path.

The existing store-file suites are rewritten to the new spelling rather than duplicated. The shared kind-fixture tables carry the bare addresses.

**Template-contract tests: no shipped instruction produces a refused command.** The instruction-surface deny-list is extended with the old spellings and to cover the partials and managed-agent templates. A whole-corpus test over every rendered step instruction, skill and managed AGENTS.md section asserts that nothing addresses a spec, plan or changelog document with an extension or a joined path. Existing template assertions that pin old spellings are updated to the new ones, not removed, so they keep pinning the exact instruction.

**Instructions never carry a store path.** A template-contract test fails if any template references a removed path variable, and the whole-corpus check asserts that no rendered instruction contains the test project's root path. Workflow `new`/`goto`/status result tests assert config-relative literals and the address fields.

**Regression.** Existing step and workflow tests guard that the step layout helpers still produce the same store paths after delegation: plan scaffolding, changelog, test plan and implement plan reads. The walkthrough revision hint test pins the new two-argument write form.

**End-to-end (manual).** The spec, plan and implement harbor suites have their spellings updated (instructions, scripted solutions, verifier oracles) so they stay usable, but they are not run for this change: by the user's decision, a hand-driven spec → plan → implement run is the proof that shipped instructions produce no addressing refusal. The suites do not run in CI.

**Docs.** The docs site is verified by a clean site build and type check plus the MDX no-layout-HTML guard. Content accuracy (every command, field and code on the new page matches the CLI) is checked by running each documented command against a scratch project.

**Deliberate gaps.**

- No new tests for design or knowledge addressing, which are unchanged non-goals.
- No test that the `.claude`/`.bob` installed copies match the templates. They are regenerated by `init`, and the rendered-skill guard already renders from templates.

**Success metrics.**

- *Hive reaches spec, plan and implementation record from the single recorded name with no per-store string handling*: **Behavioural test**. A CLI test takes the `name` recorded in workflow state (`state.json` `data.name`) after a completed run and reads the spec, the plan's `plan` document and the changelog record using only that name (plus `plan`). All three succeed with no string manipulation.
- *No bug reports about a document command refusing a name a list printed*: **Behavioural test** for the guarantee (the list→read round-trip tests above, for every kind and both changelog routings). The post-release absence of reports is **Manual — captured in the implementation test plan**.
- *No reports of an absolute host path or `internal_error` from the spec, plan or changelog document commands*: **Behavioural test** for the guarantee (the refusal tests assert no absolute path and no internal error for extension, joined-path, missing-document and not-found cases). Post-release monitoring is **Manual — captured in the implementation test plan**.

## Milestones & Tasks

### Milestone 1: One bare name addresses every spec, plan document and changelog record

**What changes**: A feature's bare name, exactly as the workflow records it, reads, writes, deletes and changes the status of its spec and changelog record. Its plan documents are addressed as the feature plus a document name (`plan`, `context`, `research`, `test-plan`). The list commands print exactly those bare names, so anything a list prints can be passed straight back. Old spellings are refused from this point with a clear message and the same command correctly spelled: a name carrying `.md`, or a plan addressed as `feature/plan.md`. Reading a plan with no document name gives an actionable refusal instead of an internal error that leaked a host path. Every shipped skill, workflow step instruction and harbor suite moves to the new spelling in the same milestone, so a full spec → plan → implement run never hits a refusal. A test guard keeps old spellings from creeping back.

**Validation point**:

- The full Go test suite passes.
- For an existing feature, `spec file read <feature>`, `plan file read <feature> plan` and `changelog file read <feature>` all succeed, and each list output round-trips into read.
- The `.md` and joined forms are refused with `unexpected_extension` and a next action that shows the correct command.
- `plan file read <feature>` is refused with `document_required`.
- A manual spec → plan → implement run completes without an addressing refusal.

#### - [ ] Task: Add the artifact address package
**Id:** d20ef613-ccf0-47b6-84d7-5df0b7e34ab1
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Introduce a small package that owns what a document address is: a kind, a feature name and, for plans, a document name. It parses what a caller typed, refuses any segment carrying an extension or a path separator, and refuses a plan document address with no document. It maps a valid address onto the file provider's layout and turns a listed entry back into its bare name. It defines the two new error codes, `unexpected_extension` and `document_required`, and is the only place that knows documents are stored as `.md` files.

*Technical detail:* [context.md#task-add-the-artifact-address-package](./context.md#task-add-the-artifact-address-package)

**Acceptance criteria**:
- [ ] Bare spec, changelog and plan-document addresses parse, and each maps to the exact file location the workflows already write.
- [ ] Any address segment containing a `.` or `/` (such as `x.md`, `x/plan.md` or `specs/x`) is refused as an unexpected extension, and the refusal carries the correctly spelled address.
- [ ] A plan document address with only a feature is refused as document required.
- [ ] A listed file or folder entry turns back into exactly the bare name that parses to it, and entries that are not documents are reported as not addressable.

#### - [ ] Task: Address spec, plan and changelog document commands by bare name
**Id:** 5abaeadf-acd7-41a4-8038-50b6fd0d2e15
**Repo:** spektacular
**Depends on:**
- d20ef613-ccf0-47b6-84d7-5df0b7e34ab1 — Add the artifact address package
**Execution:** agent

Rework the one command builder behind every `spec file`, `plan file` and `changelog file` verb so it parses its arguments through the address package. Spec and changelog verbs take the bare feature name. Plan document verbs take the feature and the document as two arguments, and `plan file list` lists features, or one feature's documents, by bare name. Old spellings are refused, and the next action restates the same command, with the caller's flags, correctly spelled. A missing document gets a `not_found` pointing at the matching list command. `set-document-status` reports the address and the location separately.

*Technical detail:* [context.md#task-address-spec-plan-and-changelog-document-commands-by-bare-name](./context.md#task-address-spec-plan-and-changelog-document-commands-by-bare-name)

**Acceptance criteria**:
- [ ] Every name printed by listing specs, changelog records (with and without a repo), plans and one plan's documents, passed unchanged to read, returns that document's content, and no listed name ends in an extension.
- [ ] Write, delete and set-document-status succeed with the bare names a list printed and act on the same document read returns. Changelog write, read and list with `--repo` still route to the named repo.
- [ ] Every verb given a name ending in `.md`, or a plan addressed as `feature/plan.md`, fails with `unexpected_extension`, changes nothing in the store, and its next action is the same command correctly spelled.
- [ ] Every plan document verb given only a feature fails with `document_required`, never an internal error. Its message contains no absolute path, and its next action names `plan file list <feature>` and an example read.
- [ ] The ID-prefix check, repo provenance stamping and plan-body validation behave exactly as before for correctly addressed writes.

#### - [ ] Task: Route workflow layout helpers and the walkthrough hint through the address
**Id:** 9e200427-c471-4d7f-af3e-df8795c0659d
**Repo:** spektacular
**Depends on:**
- d20ef613-ccf0-47b6-84d7-5df0b7e34ab1 — Add the artifact address package
**Execution:** agent

Make the spec, plan and implement workflow packages' existing path helpers delegate to the address package, so workflow steps write exactly the documents the CLI reads by name, and `.md` is spelled out in one place. Update the plan walkthrough's revision hint, which the workflow engine builds in Go, so it tells the agent to rewrite a plan document with the two-argument write form.

*Technical detail:* [context.md#task-route-workflow-layout-helpers-and-the-walkthrough-hint-through-the-address](./context.md#task-route-workflow-layout-helpers-and-the-walkthrough-hint-through-the-address)

**Acceptance criteria**:
- [ ] Every workflow still creates, reads and writes the same spec, plan, context, research, test-plan and changelog files it did before.
- [ ] The walkthrough revision hint names `plan file write <feature> <doc> --from <scratch>` and no longer mentions a `.md` document path.

#### - [ ] Task: Move shipped skills and step instructions to the new addressing
**Id:** 5e105e3b-47be-4a35-9719-a41556c34edd
**Repo:** spektacular
**Depends on:**
- 5abaeadf-acd7-41a4-8038-50b6fd0d2e15 — Address spec, plan and changelog document commands by bare name
- 9e200427-c471-4d7f-af3e-df8795c0659d — Route workflow layout helpers and the walkthrough hint through the address
**Execution:** agent

Rewrite every workflow skill, step template and shared partial that tells an agent how to read, write or list a spec, plan document or changelog record so it uses the bare-name and feature-plus-document forms. Update the error next actions that name the list commands. Widen the instruction guard test so any old spelling in any shipped template or rendered instruction fails the build. Update the existing template assertions to the new spellings, then regenerate the installed skill copies and managed agent sections from the templates.

*Technical detail:* [context.md#task-move-shipped-skills-and-step-instructions-to-the-new-addressing](./context.md#task-move-shipped-skills-and-step-instructions-to-the-new-addressing)

**Acceptance criteria**:
- [ ] No shipped skill, step instruction, partial or managed agent section addresses a spec, plan document or changelog record with an extension or a joined path.
- [ ] A test fails if an old spelling is reintroduced in any template, including partials and managed agent sections, or in any rendered agent-facing instruction.
- [ ] The installed Claude and Bob skill copies and the managed AGENTS.md sections in this repo match what the templates now render.

#### - [ ] Task: Update harbor end-to-end suites to the new addressing
**Id:** 8ce57330-46a5-43fd-b48e-a594303a3118
**Repo:** spektacular
**Depends on:**
- 5e105e3b-47be-4a35-9719-a41556c34edd — Move shipped skills and step instructions to the new addressing
**Execution:** agent

The harbor suites carry hand-maintained copies of CLI spellings in their task instructions, scripted solutions and verifier oracles. Update every spec, plan and changelog document command in the spec, plan and implement suites to the new forms, as literals rather than derived from the templates. The same pass fixes the spec suite's scripted spec write, which already used a retired stdin form.

*Technical detail:* [context.md#task-update-harbor-end-to-end-suites-to-the-new-addressing](./context.md#task-update-harbor-end-to-end-suites-to-the-new-addressing)

**Acceptance criteria**:
- [ ] No harbor suite instruction, scripted solution or verifier oracle uses an extension or joined-path address.
- [ ] The implement suite's verifier looks for the bare-name changelog writes the updated step instructions now produce.

#### - [ ] Task: Manually test a full run with the new addressing
**Id:** 7558648e-d65e-409e-a125-b6d84c6e8bf4
**Repo:** spektacular
**Depends on:**
- 8ce57330-46a5-43fd-b48e-a594303a3118 — Update harbor end-to-end suites to the new addressing
**Execution:** human — the user has chosen to verify the end-to-end run by hand instead of running the harbor suites

Drive a real spec → plan → implement run by hand, using only the shipped skills and step instructions, to confirm that no step produces an addressing refusal. This replaces running the harbor suites for this change. The suites' spellings are still updated, so they stay usable, but they are not run as part of this plan. Report any refused command so the instruction that produced it can be fixed.

*Technical detail:* [context.md#task-manually-test-a-full-run-with-the-new-addressing](./context.md#task-manually-test-a-full-run-with-the-new-addressing)

**Acceptance criteria**:
- [ ] A full spec → plan → implement run driven by the shipped instructions completes.
- [ ] No step in that run hits an `unexpected_extension` or `document_required` refusal.

### Milestone 2: Reported locations are relative to the configuration that declares the store

**What changes**: Every storage location the spec, plan and changelog list commands print is relative to the folder holding the configuration file that declares the store, for example `specs/<feature>.md` instead of `.spektacular/specs/<feature>.md`. It looks the same whether or not a changelog `--repo` is named. `plan status` and `implement status` report the plan by address, with `plan_name` and a new `plan_document`, and give its location as a config-relative `plan_path` instead of an absolute host path, so no host path leaks from any of these commands. The workflow commands' `new` and `goto` results follow the same convention. Step instructions stop pointing agents at file paths: they name each document and the CLI command that reads it, since a store's documents may not be on disk. External callers can rely on `name` as the address and `path` as a portable location.

**Validation point**:

- The full Go test suite passes.
- On this repo, `spec file list`, `plan file list`, `plan file list <feature>` and `changelog file list` (with and without `--repo spektacular`) print paths starting with the configured directory, with no `.spektacular/` prefix and no absolute path.
- `plan status` and `implement status` during a run show `plan_document: "plan"` and a relative `plan_path`.

#### - [ ] Task: Report list locations relative to the declaring configuration
**Id:** b741574a-ce76-41f8-a559-82e188d1ec7f
**Repo:** spektacular
**Depends on:**
- 5abaeadf-acd7-41a4-8038-50b6fd0d2e15 — Address spec, plan and changelog document commands by bare name
**Execution:** agent

Make every `path` printed by the spec, plan and changelog list commands, and by set-document-status, relative to the folder that holds the configuration file declaring the store. For the central stores that is the folder holding config.yaml, and for a repo's changelog it is the folder holding repo.yaml. The store resolver hands back that base with the store, and one routine renders every reported location, so the convention is identical with and without `--repo`.

*Technical detail:* [context.md#task-report-list-locations-relative-to-the-declaring-configuration](./context.md#task-report-list-locations-relative-to-the-declaring-configuration)

**Acceptance criteria**:
- [ ] Locations printed by listing specs, plans, a plan's documents and changelog records start with the store's configured directory, and contain neither the project's hidden settings folder prefix nor an absolute path.
- [ ] Changelog locations follow the same convention whether or not a repo is named.
- [ ] Set-document-status reports its location in the same convention.

#### - [ ] Task: Report the plan by address and relative location in status
**Id:** 364d550d-1546-4e57-98f8-b6b7ccdd5865
**Repo:** spektacular
**Depends on:**
- d20ef613-ccf0-47b6-84d7-5df0b7e34ab1 — Add the artifact address package
**Execution:** agent

Make `plan status` and `implement status` report the plan by address, with the existing `plan_name` and a new `plan_document` that is always `plan`. Replace today's absolute `plan_path` with the plan's location relative to the declaring configuration. Update both commands' published output schemas to match. The named `plan status <feature>` form, which already reports the bare name and no location, is unchanged.

*Technical detail:* [context.md#task-report-the-plan-by-address-and-relative-location-in-status](./context.md#task-report-the-plan-by-address-and-relative-location-in-status)

**Acceptance criteria**:
- [ ] During an active plan or implement run, the status output shows the bare `plan_name`, `plan_document` of `plan`, and a `plan_path` such as `plans/<feature>/plan.md`.
- [ ] No absolute host path appears in either status output.
- [ ] Both commands' schema output lists `plan_document` and describes `plan_path` as config-relative.
- [ ] The feature name recorded in workflow state reads the feature's spec, `plan` document and changelog record with no string manipulation.

#### - [ ] Task: Name documents by address in workflow output and step instructions
**Id:** 32733cb9-3068-475f-8b86-e58c60fb6076
**Repo:** spektacular
**Depends on:**
- 5e105e3b-47be-4a35-9719-a41556c34edd — Move shipped skills and step instructions to the new addressing
- 364d550d-1546-4e57-98f8-b6b7ccdd5865 — Report the plan by address and relative location in status
**Execution:** agent

Stop handing agents host file paths for documents Spektacular owns, because those documents may not be on disk at all. Every step instruction that points at a spec, plan document or changelog record names it by address and gives the CLI command that reads it. The absolute path template variables are removed, so an instruction can no longer render one. The results of the spec, plan and implement `new` and `goto` commands, and `spec status`, report the document by address with a config-relative location, matching the status commands.

*Technical detail:* [context.md#task-name-documents-by-address-in-workflow-output-and-step-instructions](./context.md#task-name-documents-by-address-in-workflow-output-and-step-instructions)

**Acceptance criteria**:
- [ ] No rendered step instruction contains a host path to a spec, plan document or changelog record. Every reference names the document and the CLI command that reads it.
- [ ] A template that tries to use a removed path variable fails a test.
- [ ] The `new`, `goto` and status results for spec, plan and implement workflows report locations relative to the declaring configuration, with no absolute host path, alongside the document's address.

### Milestone 3: The new addressing and location convention are documented, with a migration note

**What changes**: Users and external callers can find the rules. The CLI README and the documentation site's configuration reference explain that reported locations are relative to the configuration file declaring the store. A new document-command reference page on the site covers:

- how specs, plan documents and changelog records are addressed
- what the list commands print
- both new error codes

A migration note, in the CLI CHANGELOG and on the site, maps each old spelling to its new form and states that the old spellings are now refused. This milestone changes no behaviour.

**Validation point**:

- The docs site builds and type-checks cleanly, and the new page is reachable from the Resources navigation.
- Every command, field and error code on it matches the CLI's actual output on a scratch project.
- The README and CHANGELOG contain the location rule and the migration mapping.

#### - [ ] Task: Document addressing, locations and migration in the CLI repo
**Id:** f76db69e-9c1c-42dc-9a47-ff7b956cf8e4
**Repo:** spektacular
**Depends on:**
- 5e105e3b-47be-4a35-9719-a41556c34edd — Move shipped skills and step instructions to the new addressing
- b741574a-ce76-41f8-a559-82e188d1ec7f — Report list locations relative to the declaring configuration
- 364d550d-1546-4e57-98f8-b6b7ccdd5865 — Report the plan by address and relative location in status
**Execution:** agent

Update the README so it documents how specs, plan documents and changelog records are addressed. It also covers what the list and status commands report, the two new error codes, and the rule that reported locations are relative to the configuration file declaring the store. Add a breaking-change migration note to the CHANGELOG that maps each old spelling to its new form and says the old spellings are now refused. Update the examples in the store-access knowledge entry, through the knowledge CLI, if they spell a document address.

*Technical detail:* [context.md#task-document-addressing-locations-and-migration-in-the-cli-repo](./context.md#task-document-addressing-locations-and-migration-in-the-cli-repo)

**Content example** (README, under the existing status and list paragraph):

```markdown
### Addressing specs, plans and changelog records

Every document a feature produces is addressed by the feature's bare name, the same
name the workflow records, with no file extension:

    spektacular spec file read 000059_normalise-artifact-addressing
    spektacular plan file list 000059_normalise-artifact-addressing      # plan, context, research, test-plan
    spektacular plan file read 000059_normalise-artifact-addressing plan
    spektacular changelog file read 000059_normalise-artifact-addressing [--repo <name>]

Every `name` a list prints is accepted unchanged by read, write, delete and
set-document-status. `path` is where the store keeps the document, relative to the
folder holding the configuration file that declares the store (`specs/<name>.md`,
`plans/<name>/plan.md`, or `changelog/<project>/<name>.md` in a repo's changelog).

A name with an extension, or a plan written as one path (`<name>/plan.md`), is refused
with `unexpected_extension`; `plan file read <name>` with no document is refused with
`document_required`. Both refusals' `next_action` shows the correct command.
```

**Content example** (CHANGELOG entry):

```markdown
**Breaking change**: spec, plan and changelog document commands no longer accept file
extensions or joined plan paths. `spec file read <name>.md` is now `spec file read <name>`;
`plan file read <name>/plan.md` is now `plan file read <name> plan` (likewise `write`,
`delete`, `set-document-status`); `changelog file read <name>.md` is now
`changelog file read <name>`. The old spellings are refused with `unexpected_extension`,
whose `next_action` shows the new command. List `path` values and the `plan_path`
reported by `plan status` and `implement status` are now relative to the folder holding
the declaring config file (e.g. `specs/<name>.md`), and both status commands add
`plan_document`.
```

**Acceptance criteria**:
- [ ] The README describes bare-name addressing for specs, plan documents and changelog records, the list and status output, both new error codes and the config-relative location rule, with no em dashes.
- [ ] The CHANGELOG carries a breaking-change note mapping every old spelling to its new form and stating that old spellings are refused.
- [ ] Every command shown in the README and CHANGELOG succeeds, or is refused, exactly as documented when run against this repo.

#### - [ ] Task: Publish the document command reference and migration note on the docs site
**Id:** 36248681-b4f7-45f2-8df2-7c405d0b5efb
**Repo:** docs
**Depends on:**
- b741574a-ce76-41f8-a559-82e188d1ec7f — Report list locations relative to the declaring configuration
- 364d550d-1546-4e57-98f8-b6b7ccdd5865 — Report the plan by address and relative location in status
- 5abaeadf-acd7-41a4-8038-50b6fd0d2e15 — Address spec, plan and changelog document commands by bare name
**Execution:** agent

Add a new documentation page, linked from the Resources navigation, that is the command reference for spec, plan and changelog documents. It covers the addressing rules, what the list commands print, both new error codes, and a migration section for external callers. Update the configuration reference so the spec, plan and changelog store keys say that reported locations are relative to the configuration file declaring the store. Update the plan-tasks page's status example to show `plan_name`, `plan_document` and the relative `plan_path`.

*Technical detail:* [context.md#task-publish-the-document-command-reference-and-migration-note-on-the-docs-site](./context.md#task-publish-the-document-command-reference-and-migration-note-on-the-docs-site)

**Content outline** (new page `Documents`, `src/pages/documents.mdx`):

1. **Hero**: "Addressing specs, plans and changelogs". Sub: "One name, the feature's, reaches every document a feature produces."
2. **Section `Addressing documents: the rules`** (`surface={false}`), prose plus a list:
   - A document is addressed by the feature's bare name, exactly as the workflow records it.
   - A plan document is the feature plus a document name: `plan`, `context`, `research`, `test-plan`.
   - A name never carries a file extension.
   - What a list prints as `name` is exactly what read, write, delete and set-document-status accept.
3. **Section `Specs: spec file`** (`surface={true}`), fenced example:
   ```bash
   spektacular spec file list
   spektacular spec file read 000059_normalise-artifact-addressing
   spektacular spec file write 000059_normalise-artifact-addressing --from .spektacular/tmp/spec.md
   ```
4. **Section `Plans: plan file`** (`surface={false}`), fenced example plus sample list output:
   ```bash
   spektacular plan file list 000059_normalise-artifact-addressing
   spektacular plan file read 000059_normalise-artifact-addressing plan
   ```
   ```json
   { "files": [ { "name": "plan", "path": "plans/000059_normalise-artifact-addressing/plan.md", "modified_at": "2026-09-25T12:00:00Z" } ] }
   ```
5. **Section `Changelog records: changelog file`** (`surface={true}`): the bare-name read with and without `--repo`, and the two `path` shapes (`changelog/<name>.md`, and `changelog/<project>/<name>.md` in a repo).
6. **Section `Where documents are stored: name and path`** (`surface={false}`): `name` is the address, `path` is the location relative to the folder holding the declaring config file. Also shows the `plan status` fields `plan_name`, `plan_document` and `plan_path`.
7. **ConfigurationKeys `Error codes`** (`surface={true}`), one `ConfigKey` per code:
   - `unexpected_extension`: the name carried an extension or a joined plan path. `next_action` shows the command correctly spelled. Includes a sample JSON envelope for `spec file read x.md`.
   - `document_required`: a plan document command got a feature and no document. `next_action` names `plan file list <feature>` and an example read.
8. **Section `Upgrading from earlier spellings`** (`surface={false}`): an old → new mapping table for read, write, delete, set-document-status and list across all three stores, a statement that the old forms are refused, and a note that list `path` and status `plan_path` are now config-relative.
9. **CtaBanner** linking to Configuration.

**Content example** (configuration.mdx, appended to each of the `spec`, `plan` and `changelog` ConfigKey bodies):

```mdx
  Locations the CLI reports for these documents (a list's `path`, status `plan_path`)
  are relative to the folder holding `config.yaml`, for example `specs/<name>.md`.
  Documents are addressed by name, never by this location: see [Documents](/documents/).
```

**Acceptance criteria**:
- [ ] A reader can find a Documents page from the Resources menu that explains how specs, plan documents and changelog records are addressed, what list prints, both new error codes, and how to migrate from the old spellings.
- [ ] The configuration reference states that reported locations are relative to the folder holding the configuration file that declares the store.
- [ ] The plan-tasks page's status example shows `plan_name`, `plan_document` and a relative `plan_path`.
- [ ] The site builds and type-checks cleanly, page bodies carry no layout HTML, and no authored text uses em dashes.
- [ ] Every command, output field and error code shown matches the CLI's real behaviour.

## Open Questions

- **Does the implement run for this very plan trip over its own change?** This repo drives its workflows with `go run .`, a dev build of the code being changed. Once the command builder is address-driven, and before the templates move, instructions rendered by the running implement workflow still say `plan file read <name>/plan.md`, and the CLI will refuse them. Whether this actually bites depends on which implement steps are rendered in that window, which is only known during implementation. *What to do:* treat an `unexpected_extension` refusal from the workflow's own instructions as expected in that window. Run the `next_action` it gives, which is the same command correctly spelled, and carry on. Land the addressing and instruction-migration tasks back to back. Do not change the CLI to accept old spellings temporarily. If a refusal blocks a workflow step in a way the next action cannot resolve, STOP and ask the user.

No other implementation-time uncertainties remain. Every other decision is recorded in the assumption log.

## Out of Scope

- **Design documents are unchanged.** They stay addressed by source and path, and design sources keep reporting their location as they do today. A user-supplied design may be in any format, so its extension is meaningful. (Spec non-goal.)
- **No `implement` document command.** A feature's implementation record stays under `changelog file`, with no alias. (Spec non-goal; issue #46 open question 1.)
- **Knowledge commands are unchanged.** They already address entries by name and report config-relative locations. (Spec non-goal.)
- **Workflow commands' input is unchanged.** `spec new`, `plan new`, `implement new` and the status commands already take the bare feature name. (Spec non-goal.)
- **No deprecation window.** Old spellings are refused from the release that ships this. The CLI never accepts both forms. (Spec constraint.)
- **The named `plan status <feature>` form is unchanged.** It already reports the bare name and no location. `spec status` is unchanged.
- **No `--repo` on changelog delete or set-document-status.** Those verbs keep working on the central changelog only, as today.
- **`artifacts list` is unchanged.** Its pre-existing mismatch is left alone: it scans central changelog records one folder below where they are written. It is a separate command outside this spec, and worth its own issue.
- **No site-wide command reference.** The new docs page covers the spec, plan and changelog document commands only, not every CLI command.
- **No template-drift cleanup beyond regeneration.** Regenerating the installed skill copies picks up unrelated earlier template changes as a side effect. No other reconciliation of those copies is planned.
