---
created_date: "2026-09-16"
status: completed
closed_date: "2026-09-16"
---

# Plan: 000052_document-status-vocabulary

<!-- Metadata -->
<!-- Created: 2026-09-16T11:08:14Z -->
<!-- Commit: 0f133c0 -->
<!-- Branch: b-metadata -->
<!-- Repository: git@github.com:jumppad-labs/spektacular.git -->

## Overview

This plan renames the lifecycle metadata on every spec, plan and changelog entry to `document_status`, with the document values `draft`, `final`, `superseded` and `archived`. It replaces the work-flavoured `in-progress`/`completed`, which lead planning agents to read a finished spec as a finished feature. Agents and people reading or filtering artifacts benefit. Legacy artifacts stay readable with a blank status, and no migration or aliases are provided.

## Conventions

- **Error messages must describe the problem and suggest remediation** — the renamed `invalid_document_status` and `missing_document_status` errors must keep an `output.NewError(...).WithNextAction(...)` listing the four new values and the new flag name.
- **Passing tests are required before calling work done** — the rename touches many Go test files across `cmd`, `internal/metadata` and `internal/steps`; the full Go test suite must pass.
- **No em dashes (docs repo)** — applies to the reworded lines in the website pages.
- **MDX authoring conventions (docs repo)** — the website edits stay inside existing fenced code blocks and prose; no layout HTML is introduced, and the site's layout guard, build and type-check must still pass.
- **Plans must sketch content structure (docs repo)** — the website changes are small copy edits, so the relevant phase carries a `**Content example**` for each edited line.

## Architecture & Design Decisions

This is a rename of the existing artifact lifecycle, not a new mechanism, and it lands in two repos. In **spektacular**, the `internal/metadata` package stays the single owner of the schema. Its on-disk key becomes `document_status`, its Go type becomes `DocumentStatus`, and its four values become `draft`, `final`, `superseded` and `archived`. The existing merge rules in `internal/metadata/merge.go` already implement every lifecycle rule the spec asks for: default on first write, stamp `closed_date` once, clear it on a return to draft, and preserve the status when no update is given. A blank status is simply a fifth, unnamed state that `isClosed` treats as open, so those rules carry over unchanged apart from the constant names. The CLI layer (`cmd/storefile.go`, `cmd/artifacts.go`, `cmd/artifactfilter.go`) renames `--status` to `--document-status`, `set-status` to `set-document-status`, and the `status` output keys to `document_status`. The workflow close sites in `internal/steps/{spec,plan,implement}/steps.go` pass `final`. In **docs** (the website repo), three pages get wording updates: the frontmatter example in `src/pages/projects.mdx`, the lifecycle example in `src/pages/how-it-works.mdx`, and the "a status" prose in `src/pages/configuration.mdx`.

The load-bearing decision is to be **lenient on read and strict on input**. `UnmarshalYAML` stops rejecting unknown values. It decodes the status leniently, so that even a non-string YAML value cannot fail the parse, and it normalises anything outside the four values (including the retired `in-progress`/`completed`) to blank. The legacy `status:` key needs no handling at all: the YAML decoder already ignores unknown keys, and `Render` drops them on the next write (`internal/metadata/frontmatter.go`). Command-line input goes the other way. `--document-status` and `set-document-status` reject any value outside the four with an actionable `invalid_document_status` error, so a typo can never silently blank an artifact. Both paths use one exported check in `internal/metadata`, which replaces the three hand-synced enum switches that exist today (`internal/metadata/metadata.go:112`, `cmd/artifactfilter.go:72`, `cmd/storefile.go:53`). The rename then touches one list of values instead of three.

There is deliberately no compatibility layer: no aliases and no migration (a user-mandated constraint). Legacy artifacts read with a blank status and keep their `closed_date`. A blank artifact matches no `--document-status` filter and appears in unfiltered listings with an empty value. A blank status is rendered explicitly as `document_status: ""`, so the state is visible to a human reader. Agent-facing wording is changed only in `templates/` sources (the single lifecycle line is in `templates/steps/plan/19-finished.md`), never in installed copies. The harbor implement-workflow suite's hand-maintained oracle and seeded fixtures are updated in the same change, following the project's testing-architecture rule. The rejected options (aliases, migration, keeping `status` as the name, strict reads, three separate switches, omitting a blank key) are recorded with evidence in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Artifact metadata package (changed)**: still the single owner of the lifecycle schema. It now names the field `document_status`, defines the four document values, and exposes one validator that every other component uses to check a value. Its parser becomes lenient: any unrecognised or retired value, or a missing one, reads as a blank status and never as an error. Its merge and close helpers keep their existing rules, with `draft` as the first-write default and `final` as the value the close helper is called with.
- **Store-file CLI factory (changed)**: the shared `<kind> file` command tree used by spec, plan and changelog. Its write and list subcommands expose `--document-status` instead of `--status`. Its metadata-only subcommand becomes `set-document-status`. Its JSON output reports `document_status`. It validates flag input through the metadata package's validator and rejects bad values with a remediation-bearing error.
- **List filter (changed)**: the shared filter used by both `<kind> file list` and `artifacts list`. It parses `--document-status` through the metadata validator, and it continues to treat an empty value as "no filter", so a blank artifact matches no status filter.
- **Artifacts list command (changed)**: the cross-kind aggregator. It renames its flag and output key to match the store-file factory, and it reuses the same list filter.
- **Workflow close steps (changed)**: the spec, plan and implement workflows' terminal steps close the artifacts they own as `final` through the metadata close helper. Their behaviour is otherwise unchanged.
- **Plan finished-step template (changed)**: the agent-facing wording that tells the agent the documents are now closed uses the new vocabulary.
- **Harbor implement-workflow suite (changed)**: its hand-maintained oracle expects `document_status: final` on the changelog, and its seeded fixtures carry `document_status: draft`.
- **Website pages (changed, docs repo)**: the projects page frontmatter example, the how-it-works lifecycle description and the configuration page's lifecycle prose all use document-status vocabulary.

## Data Structures & Interfaces

**Artifact frontmatter (on-disk contract).** The `status` key is replaced by `document_status`. The other keys are unchanged.

```yaml
---
created_date: "2026-09-16"
document_status: final        # draft | final | superseded | archived | "" (blank)
closed_date: "2026-09-20"     # present once the artifact has first been closed
---
```

Reading is lenient. A missing key, a retired value (`in-progress`, `completed`), an unrecognised value or a non-string value all read as blank (`""`). A legacy `status` key is ignored and is not written back.

**`metadata.DocumentStatus` (Go).** This replaces `metadata.Status`, and it is the one list of allowed values.

```go
type DocumentStatus string

const (
    StatusDraft      DocumentStatus = "draft"
    StatusFinal      DocumentStatus = "final"
    StatusSuperseded DocumentStatus = "superseded"
    StatusArchived   DocumentStatus = "archived"
)

// ParseDocumentStatus reports whether raw is one of the four values.
// The CLI uses it for strict input; the parser uses it to decide
// between a named value and blank.
func ParseDocumentStatus(raw string) (DocumentStatus, bool)

// DocumentStatuses returns the four values in lifecycle order, for
// error messages and flag help text.
func DocumentStatuses() []DocumentStatus
```

**`metadata.Metadata` / `metadata.UpdateOptions`.** The `Status` field on each becomes `DocumentStatus` (`DocumentStatus` and `*DocumentStatus` respectively). A zero value means blank on `Metadata`, and "no change" on `UpdateOptions`. `metadata.Close(st, path, status DocumentStatus)` keeps its shape.

**CLI surface.**
- `<kind> file write --document-status <v>`
- `<kind> file list --document-status <v>`
- `artifacts list --document-status <v>`
- `<kind> file set-document-status <path> --document-status <v>` (flag required)

**JSON output.** List entries in both `{"files": [...]}` and `{"artifacts": [...]}` carry `"document_status": "<v or empty>"`. The set-document-status payload carries `"document_status"` instead of `"status"`.

**Error codes.** `invalid_status` is renamed `invalid_document_status`, and `missing_status` is renamed `missing_document_status`. Both keep a `next_action` that lists the four values.

## Implementation Detail

- **No new patterns.** The change follows the existing layering: the metadata package owns the schema, the CLI factories are thin wrappers over it, and workflows close artifacts through the shared close helper. A reader who knows the current code will find the same shape with new names.
- **One source of truth for values.** Today the list of allowed values is hand-copied into three switch statements. After this change there is one validator and one ordered list in the metadata package. CLI validation, flag help text and error `next_action` text all derive from them, so adding or renaming a value later is a one-place edit.
- **A split between parsing and validating.** Parsing a stored artifact becomes a normalising operation that never fails on status; it maps anything outside the four values to blank. Validating input stays strict and is only used where a person or agent supplies a value (flags, and the merge helper's update options). This asymmetry is the new idea a reader must understand, so it gets a short doc comment at each of the two entry points.
- **Blank is a real state, not an error.** The merge helper keeps treating "no update given" as "preserve", which now also preserves blank. The first-write default applies only when no frontmatter exists at all. The helper that decides whether an artifact is closed returns false for blank.
- **Rename sweep.** Go identifiers, flag names, subcommand names, output keys, error codes, help text, doc comments and test assertions all move to the new vocabulary in the same change. Unrelated uses of "status" and "completed" (version-check status, workflow step status, `completed_steps`, "in-progress workflow" resume wording) are left untouched.
- **Cross-repo.** The website change is a copy-only edit to existing MDX pages. It introduces no components or layout.

## Dependencies

- **`gopkg.in/yaml.v3`** (existing): frontmatter parsing and rendering. It ignores unknown keys on decode, which is what lets the legacy key disappear. No change needed.
- **`github.com/spf13/cobra`** (existing): flag and subcommand definitions. No change needed.
- **`internal/output`** (existing): the error envelope with `next_action`. No change needed.
- **`internal/store`** (existing): byte-oriented store. No change needed, apart from a doc comment that mentions the status enum.
- **Spec `000052_document-status-vocabulary`**: the source of requirements. It is final and nothing else must land first.
- **Prior plan `000038_artifact_metadata`**: introduced the metadata schema, the filter flags, `set-status` and `artifacts list` that this plan renames. It has already shipped.
- **Harbor E2E tooling** (`harbor` CLI, Docker, Claude credentials): needed only for the manual implement-workflow suite run. It is not in CI.
- **Website toolchain** (Node with the Astro build and type-check) in the docs repo: needed to verify the page edits.
- **No blocking dependencies**: nothing must land before this plan starts. The rewrite of this repo's existing artifacts is done separately by the user and does not block or depend on this plan.

## Testing Approach

- **Unit tests (metadata package)** carry the most weight, because the lenient/strict split lives there. They follow the existing table-driven, testify `require` style, and they guarantee the following:
  - The frontmatter round-trips with `document_status` and never writes a `status` key.
  - Each of the four values parses to itself.
  - A missing key, a retired value (`in-progress`, `completed`), an unrecognised value (`bogus`) and a non-string value all parse to blank, without error.
  - A legacy `status:` key is ignored, and it is absent after a merge.
  - The first write defaults to `draft`. A merge with no update preserves a blank status and its `closed_date`.
  - Moving from blank or draft to `final` stamps `closed_date` only when none exists. Moving back to `draft` clears it.
  - The close helper writes `final`.
  - The validator accepts exactly the four values and rejects the retired ones.
- **CLI contract tests (cmd)** extend the existing `kindFixtures`-driven tests across spec, plan and changelog, plus `artifacts list`. They guarantee the following:
  - `--document-status` and `set-document-status` work for all four values.
  - Retired and invalid values fail with `invalid_document_status`, and leave the file byte-for-byte unchanged.
  - `--status` and `set-status` fail as unknown.
  - List JSON carries `document_status` and no `status` key.
  - A legacy or bogus-status artifact lists with an empty `document_status`.
  - A blank artifact is excluded by `draft` and `final` filters and included when no filter is given.
  - Rewriting a legacy artifact drops `status`, keeps the status blank, and keeps its dates.
- **Workflow step tests (internal/steps)** keep their existing assertions with the new values. The spec, plan and implement finished steps must leave their artifacts at `final`, and new scaffolds must start at `draft`.
- **Template contract**: the existing template tests keep passing. The one reworded finished-step line needs no new phrase assertion, because the wording is not a behavioural anchor.
- **Harbor E2E (implement-workflow)**: the hand-maintained oracle and seeded fixtures are updated to `document_status`. Harbor is a manual run, outside CI, and it proves the changelog closes as `final` end to end.
- **Website (docs repo)**: the site's build, type-check and MDX layout guard must pass. There are no automated content tests.
- **Deliberate gap**: no test asserts that installed agent copies were regenerated. They are generated from templates on init, and the constraint is honoured by editing only `templates/`.

**Success metrics:**
- *Planning agents run against a finished spec no longer stop to question whether the feature has already been built*: **Manual — captured in the implementation test plan**.
- *No artifact read fails because of its document status value, including artifacts that still carry the retired field*: **Behavioural test**. The metadata unit tests and the CLI list tests read legacy, retired-value, unrecognised and non-string status artifacts, and assert that no error occurs and the status is blank.

## Milestones & Phases

### Milestone 1: Artifacts carry a document status

**What changes**: Every spec, plan and changelog entry the CLI writes records `document_status` with the values `draft`, `final`, `superseded` and `archived`. Finished workflows leave their artifacts at `final`. Users set, filter and change the status with `--document-status` and `set-document-status`, and listings report `document_status`. Artifacts written before this change still read and list without error; their status shows as blank until someone sets it. Retired or mistyped values on the command line are rejected with an error that lists the valid values.

**Validation point**: The full Go test suite passes. In a scratch project, writing an artifact, listing it with a `final` filter and changing its status all behave as described, including on a hand-seeded legacy artifact.

### Milestone 2: Agents, end-to-end tests and the website speak the new vocabulary

**What changes**: The agent-facing wording that tells an agent its documents have been closed says "final" instead of "completed". The manual end-to-end implement-workflow suite expects `document_status: final`. The website's frontmatter example, lifecycle description and configuration prose all use document-status vocabulary. After this milestone, nothing a user or agent reads describes a finished document as "completed" work.

**Validation point**: The full Go test suite still passes. The website builds and type-checks cleanly, with no layout markup in its pages. A manual run of the implement-workflow end-to-end suite passes. No agent template, repo doc or website page uses `in-progress`/`completed` as a lifecycle value, or shows a `status:` frontmatter key.

#### - [ ] Phase 1.1: Document status vocabulary and lenient reading in the metadata package

**Repo:** spektacular

The metadata package renames its lifecycle field to `document_status`, adopts the four document values, and gains one validator that everything else will share. Reading a stored artifact becomes lenient, so that missing, retired or unrecognised values read as blank instead of failing. The workflow close sites and the existing CLI callers are updated just enough to compile against the renamed identifiers. The flag and command renames follow in the next phase.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-document-status-vocabulary-and-lenient-reading-in-the-metadata-package)

**Acceptance criteria**:
- [ ] Newly written artifacts carry `document_status: draft` and never a `status` key.
- [ ] Finished spec, plan and implement workflows leave their artifacts at `final`.
- [ ] An artifact holding `status: completed`, `document_status: bogus`, or no status at all reads without error and reports a blank status.
- [ ] Rewriting a legacy artifact drops the old `status` key, keeps the status blank, and keeps its created and closed dates.
- [ ] Moving an artifact to `final` stamps a closed date only when it has none, and moving it back to `draft` clears the date.

#### - [ ] Phase 1.2: CLI speaks document status

**Repo:** spektacular

The `file write`, `file list` and `artifacts list` commands take `--document-status`, and the status-only update command becomes `set-document-status`. List and update output report `document_status`. All command-line validation goes through the shared validator, so retired or mistyped values are rejected with an error that names the valid values. The old flag and command no longer exist.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-cli-speaks-document-status)

**Acceptance criteria**:
- [ ] Users can set, filter by and change a document status with each of the four values from the command line.
- [ ] Passing `completed`, `in-progress` or a typo to any document-status option fails with an error listing the four valid values, and leaves the artifact untouched.
- [ ] The old `--status` option and `set-status` command are reported as unknown.
- [ ] Listings from both the per-kind and cross-kind list commands show `document_status`, with an empty value for legacy artifacts.
- [ ] A blank-status artifact appears in an unfiltered listing, and in neither a `draft` nor a `final` filtered listing.

#### - [ ] Phase 2.1: Agent wording and end-to-end suite use the new vocabulary

**Repo:** spektacular

The plan workflow's closing message tells the agent that the documents are now marked final, not completed. Its template contract test is updated to match. The manual implement-workflow end-to-end suite is updated in the same change: its oracle expects `document_status: final` on the changelog, and its seeded spec and test plan carry `document_status: draft`.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-agent-wording-and-end-to-end-suite-use-the-new-vocabulary)

**Acceptance criteria**:
- [ ] The plan workflow's finished message describes the documents as final.
- [ ] No agent-facing template uses `completed` or `in-progress` as a document lifecycle value.
- [ ] The implement-workflow end-to-end suite passes against the renamed field (manual run).

#### - [ ] Phase 2.2: Website describes document status

**Repo:** docs

The website's projects page shows a changelog frontmatter example using `document_status`. The how-it-works page describes the lifecycle with the new values. The configuration page's lifecycle prose calls the field a document status. These are copy-only edits to existing pages.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-website-describes-document-status)

**Content example** (projects page, frontmatter block):

```yaml
---
created_date: "2026-09-02"
document_status: draft
project: spektacular
project_source: git@github.com:jumppad-labs/spektacular.git
spec: 000046_relocatable-repo-footprint
plan: 000046_relocatable-repo-footprint
---
```

**Content example** (how-it-works page, lifecycle list item):

```mdx
  - **Document status**: where the document stands in its lifecycle
    (`draft`, `final`, `superseded` or `archived`).
```

**Content example** (configuration page, lifecycle prose, same change in the spec, plan and changelog keys):

```mdx
    Every spec record also carries lifecycle metadata: a created date, a
    document status, and, once resolved, a closed date, tracked
    automatically without any extra configuration.
```

**Acceptance criteria**:
- [ ] The website's frontmatter example and lifecycle description use `document_status` and the new values.
- [ ] No website page mentions `in-progress`, `in_progress` or `completed` as a lifecycle value.
- [ ] The site builds and type-checks cleanly, with no layout markup introduced.

## Open Questions

No open questions. Every design choice was resolved during the spec discussion or recorded as an assumption during planning.

## Out of Scope

- Rewriting this repository's existing specs, plans and changelog entries to `document_status`. The user will do this separately, outside this plan.
- Aliases that map `in-progress`/`completed` to the new values, and any migration command. The spec rules both out.
- Filtering artifacts by a blank document status.
- A dedicated artifact-metadata reference section on the website. The user flagged it as a possible follow-up; this plan makes only the copy edits.
- Changes to the created-date or closed-date fields, or to the lifecycle rules beyond the rename.
- Hand-editing installed agent copies (`.claude/skills`, `.bob`). They pick up the template change on the next init.
- Unrelated uses of "status" and "completed": version-check status, workflow step status, `completed_steps`, and the "in-progress workflow" resume wording.
