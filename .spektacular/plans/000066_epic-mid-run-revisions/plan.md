---
created_date: "2026-10-08"
document_status: final
closed_date: "2026-10-08"
---

# Plan: 000066_epic-mid-run-revisions

<!-- Metadata -->
<!-- Created: 2026-10-08T06:39:11Z -->
<!-- Commit: 5517bd9 -->
<!-- Branch: b-patch-epics -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

This plan adds a defined path for correcting a spec, or a design it references, while the spec is being implemented. The implement steps gain a stop for "the spec or a design is wrong":
- an interactive run asks the user;
- an orchestrated child hands the conflict back to its epic orchestrator;
- the user-facing agent applies an approved amendment in the project with a new `spektacular spec amend` command.

The command records every amendment on the spec, in a `## Amendments` section and in its metadata. A recorded amendment does not make the plan stale, even under `plan.strict_spec_changes`, so the run carries on against the corrected text. This closes item 6 of #78 for users running epics and interactive implement runs.

## Conventions

- **Error messages must describe the problem and suggest remediation** — every `spec amend` refusal (missing reason or run, no change, non-amendable section changed, unreferenced or unresolved design, missing spec or plan) uses `output.NewError(...).WithNextAction(...)` with a runnable next step.
- **Store files are written through the CLI, never with file tools** — amendments go through `spec amend` and design revisions through `design author`/`design write`. Templates supply bodies with `--from` staged under `.spektacular/tmp/`, never stdin or heredoc (pinned by `internal/agent/instruction_surface_test.go`).
- **A plan never changes the active install** — skill and step templates change under `templates/` only; this repo's `.claude/skills` and `.spektacular/` are not regenerated or migrated, and changes are verified with the Go test suite and throwaway projects.
- **Tests must not depend on order** — the new staleness and amend tests pin spec and plan mtimes explicitly (`os.Chtimes` via `writeArtifactStatusFile`) and run commands through `runRootCmd`/`resetRootCmd`.
- **Passing tests are required before calling work done** — every milestone ends with the full Go test suite green. The docs repo ends with a clean site build and type check.
- **Docs: MDX authoring conventions** — the new docs content uses existing `Section`/`Prose` slots and fenced code blocks, with no layout HTML in page bodies.
- **Docs: no em dashes** — all new prose in the docs repo, the docs `CHANGELOG.md` entry and the `## Amendments` entry format avoid em dashes.
- **Docs: plans must sketch content structure** — each docs task carries a content outline or content example.
- **Docs: alternate section background shading** — considered: new content goes in `###` subsections inside existing sections, so no band is added and shading is unchanged. A new top-level `Section` would have to alternate `surface`.
- **Docs: site layout conventions** — compose from the existing section components; no new widths or heading sizes.

## Architecture & Design Decisions

The feature has three parts.

1. **A stop.** The implement step templates and both implement skills treat "the spec or a design it references is wrong" as a stop, alongside the existing plan-versus-code stops.
2. **A CLI verb, `spektacular spec amend`.** It is the only supported way to change a spec's text mid-run, and it records the change at the same moment.
3. **A staleness rule.** `status.PlanIsStale` honours a recorded amendment.

**The stop and the hand-back.** A shared partial, `templates/partials/implement-spec-conflict.md`, is included in the analyze, implement, test and verify steps (`templates/steps/implement/02`–`05`). It tells the agent to stop when a requirement, acceptance criterion, constraint or success metric of the spec, or a rule in a referenced design, is wrong or contradicts another. When it stops, it names the document, the section, the conflict and a proposed amendment.

- **Interactive run:** the agent asks the user. On explicit approval it applies the amendment itself, because it is the agent talking to the user.
- **Orchestrated run:** the partial's `{{#orchestrated}}` block says the child never writes spec or design text. The existing `orchestrated-stop.md` partial turns the STOP into a `QUESTION: <spec>` hand-back.
- **After either path:** the agent re-reads the amended document with `spec file read` / `design read` from the project root and re-runs the current step's verification of the task against the amended text before advancing.
- **Changelog:** the task's plan changelog entry gains an `**Amendments**` field (`07-update_changelog.md`).
- **Skills:** `spek-implement-epic` adds the conflict to its definition of a genuine question and to the child prompt. It also gains an "apply an approved amendment" step: the orchestrator applies the change from the project root after the user approves, then answers the child naming what changed. `spek-implement` gains a user-facing section describing the same path, and one line in its orchestrated section.

No new FSM step is added, so the step table and the harbor `EXPECTED_STEP_ORDER` oracle are unchanged.

**`spec amend` keeps the record and the exemption together.** It is invoked as `spektacular spec amend --data '{"name","reason","run","design"?}' [--from <staged spec>]`.

- **With `--from`:** it replaces the spec body. It first diffs the old and new bodies section by section, ignoring checkbox marks. It refuses when nothing changed, or when any section outside the amendable four (Requirements, Acceptance Criteria, Constraints, Success Metrics) changed. The changed sections are therefore derived by the CLI rather than claimed by the agent.
- **With `design` and no `--from`:** it records a revision already made to a design the spec references (via `design author`, or via `design write` for a team design the user supplied). It refuses a design the spec does not reference or that does not resolve.
- **What it writes:** it appends a dated entry to a `## Amendments` section at the end of the spec body, holding the sections or design changed, the reason and the run. It also adds a modelled `amendments` record to the spec's frontmatter: timestamp, sections or design, and a SHA-256 of the resulting body with checkbox marks normalised. All of this is one write through `metadata.Merge`.
- **Other writers:** because the metadata schema is closed (`internal/metadata/frontmatter.go:69-84`), `amendments` is modelled like `designs` and carried forward by every ordinary `spec file write`, including `reconcile_spec`.
- **Errors:** every refusal is an `output.NewError(...).WithNextAction(...)` naming the corrective command (convention: error messages must suggest remediation).

**Staleness.** `PlanIsStale` (`internal/status/classify.go:136`) gains one early rule: when the spec carries amendments and its current normalised body hash equals the last amendment's hash, the plan is not stale. Otherwise the existing mtime comparison runs unchanged.

- **Unrecorded edits:** a spec edited any other way changes the hash and falls back to today's behaviour, in strict and non-strict mode alike. An unrecorded edit made after an amendment does the same.
- **What doesn't break an amendment:** frontmatter-only writes (epic link, design ref add, set-document-status) and `reconcile_spec`'s checkbox ticks.
- **Coverage:** every consumer goes through this predicate: `refuseStalePlan` for `implement new`/`goto`, `status`, `DependenciesOf`, `EpicComplete` and the epic run view. One change therefore covers interactive runs, orchestrated lanes and status.

**Why this direction.** It beats re-writing `plan.md` after the spec to win the mtime race: that is racy against a child writing `plan.md`, invisible after a git checkout, and hides unrecorded edits. It also beats stamping a spec hash on the plan at approval, which would change staleness for unrecorded and frontmatter-only edits, against the spec's "keeps today's staleness behaviour" requirement. See `research.md#alternatives-considered-and-rejected`.

**Repos.**
- `spektacular` (`/home/nicj/code/github.com/hivecommons/spektacular`): the CLI verb, metadata, staleness, step templates, skills and their tests, plus `README.md`.
- `docs` (`/home/nicj/code/github.com/hivecommons/spektacular-website`): the implement, epics, documents, design-documents, configuration and plan-tasks pages, plus `CHANGELOG.md`.

**Self-hosting.** Skill templates change under `templates/skills/`, but the installed `.claude/skills` copies of this repo are not regenerated (convention: a plan never changes the active install). Verification is the Go test suite and throwaway projects only.

## Component Breakdown

- **Spec metadata: amendments record** (changed, `spektacular` repo). This is the spec frontmatter model.
  - It gains an `amendments` list. Each entry holds the moment the amendment was recorded, the sections or design it changed, and a hash of the resulting body.
  - It owns rendering, lenient decoding and carry-forward of the list across ordinary writes, exactly as it already does for design references.
  - It also owns the checkbox-normalised body hash, which both the amend command and the staleness check use. The two therefore can never compute it differently.
- **`spec amend` command** (new, `spektacular` repo). This is the single entry point for a mid-run amendment.
  - It validates the request: reason and run are required; the spec and its plan must exist; a design must be referenced and must resolve.
  - It diffs the staged body against the stored one by section to work out what changed, and refuses changes outside the amendable sections.
  - It appends the human-readable `## Amendments` entry and stores the body and the metadata record in one write through the shared metadata merge.
  - It sits beside the existing `spec file` verbs and reuses the store, config loading and error/next-action plumbing.
- **Plan staleness check** (changed, `spektacular` repo). This is the one predicate every staleness consumer calls.
  - It gains the amendment exemption: if the spec's current body hash matches its last recorded amendment, the plan is not stale. Otherwise the existing modification-time comparison decides.
  - Callers are unchanged: implement new/goto refusal, status, dependency checks, epic completion and the epic run view all inherit the behaviour.
- **Implement spec-conflict partial** (new, `spektacular` repo). This is shared instruction text for the analyze, implement, test and verify steps.
  - It defines the stop for a wrong spec or design, and what to name when stopping.
  - For interactive runs, it covers how the user-facing agent applies an approved amendment (`spec amend`, `design author`/`design write`), followed by the re-read and re-verification.
  - For orchestrated runs (inside the orchestrated-only block), the child never writes spec or design text and proposes the amendment through its hand-back. The existing orchestrated hand-back partial converts the stop into a `QUESTION:`.
- **Implement step templates** (changed, `spektacular` repo).
  - Steps 02–05 include the partial.
  - The plan-changelog step's entry format gains an `Amendments` field, naming the amended document, section and reason for the task in which the conflict was found.
- **`spek-implement` skill** (changed, `spektacular` repo).
  - A new user-facing section describes the amendment path for interactive runs.
  - Its final orchestrated section adds one line: a child never writes spec or design text.
- **`spek-implement-epic` skill** (changed, `spektacular` repo).
  - The definition of a genuine question and the child prompt both include the spec or a design being wrong.
  - Step 6 gains how the orchestrator applies an approved amendment from the project root and answers the child.
  - The store-access rule names `design` and `spec amend`.
- **Template and skill contract tests** (changed, `spektacular` repo). These are the existing phrase-assertion suites for step templates, orchestrated wording and skills. They pin the new wording and keep orchestrator-only wording out of standalone steps.
- **CLI README** (changed, `spektacular` repo). The `plan.strict_spec_changes` paragraph gains the recorded-amendment exemption.
- **Documentation site pages** (changed, `docs` repo).
  - The implement pipeline stage and the epics page describe raising, approving, applying and recording an amendment.
  - The documents page lists `spec amend`.
  - The design-documents page caveats its "no command rewrites a design" statement.
  - The configuration page documents `plan.strict_spec_changes` and the exemption.
  - The plan-tasks page notes the exemption beside the stale state.
  - The docs `CHANGELOG.md` gains an entry.

## Data Structures & Interfaces

**`metadata.Amendment` (new) and `Metadata.Amendments` (new field).** This is one recorded mid-run amendment on a spec. It is modelled in the closed frontmatter schema, like `Designs` and `Sources`, so it survives every ordinary write. Only `spec amend` appends to it.

```go
type Amendment struct {
    At       string    `yaml:"at" json:"at"`                                 // RFC3339 UTC, stamped by the CLI
    Sections []string  `yaml:"sections,omitempty" json:"sections,omitempty"` // spec sections changed, e.g. "Success Metrics"
    Design   *DesignRef `yaml:"design,omitempty" json:"design,omitempty"`    // design revised, when the amendment records one
    Hash     string    `yaml:"hash" json:"hash"`                             // "sha256:<hex>" of the resulting body, checkbox-normalised
}

// on Metadata
Amendments []Amendment   // rendered as `amendments:` after `sources:`

// on UpdateOptions: nil = carry forward, non-nil = replace (same contract as Designs)
Amendments *[]Amendment
```

On disk:

```yaml
amendments:
    - at: "2026-10-09T14:02:11Z"
      sections: [Success Metrics]
      hash: sha256:3f1c…
    - at: "2026-10-09T15:40:00Z"
      design: {source: project, path: billing/rounding.md}
      hash: sha256:9ab2…
```

**`metadata.BodyHash(body []byte) string` (new).** This returns `"sha256:<hex>"` of the body with every task-list mark (`[x]`/`[X]` at the start of a list item) normalised to `[ ]`. It is the single definition shared by `spec amend` and `PlanIsStale`, so ticking checkboxes never invalidates a recorded amendment.

**`status.PlanIsStale` (signature unchanged, behaviour extended).** It still takes `(cfg, st store.Reader, planName, fm)`. It now also reads the spec through `st`, and returns false when the spec's last `Amendment.Hash == BodyHash(specBody)`. Otherwise it applies the existing modification-time rule.

**`spec amend` command contract (new CLI surface).**

```
spektacular spec amend --data '<json>' [--from <staged spec file>] [--dry-run] [--schema]
```

Input (`--data`):

```json
{
  "name":   "000070_billing",
  "reason": "Design rule rounds half-even; success metric said half-up. User chose the design.",
  "run":    "epic 000068_payments implement run, task 2.1",
  "design": {"source": "project", "path": "billing/rounding.md"}
}
```

- `name`, `reason` and `run` are required.
- At least one of `--from` and `design` is required.
- `--from` holds the full amended spec (frontmatter in it is ignored).
- `design` records a revision already applied to a design the spec references.

Output:

```json
{
  "error": false,
  "spec": "000070_billing",
  "amended_sections": ["Success Metrics"],
  "design": {"source": "project", "path": "billing/rounding.md"},
  "recorded_at": "2026-10-09T14:02:11Z",
  "hash": "sha256:3f1c…"
}
```

Error codes, each with a `next_action`:
- `bad_input`
- `spec_amend_reason_required`
- `spec_amend_run_required`
- `spec_amend_nothing_to_record`
- `spec_not_found`
- `spec_amend_no_plan`
- `spec_amend_no_change`
- `spec_amend_section_not_amendable`
- `spec_amend_design_not_referenced`
- `spec_amend_design_unresolved`

**`## Amendments` body section (new spec section, written only by the CLI).** It is appended at the end of the spec body and created on first use. The CLI appends each entry; the `--from` body's own `## Amendments` section must match the stored one, so an agent cannot rewrite past entries.

```markdown
## Amendments

- **2026-10-09: Success Metrics** (epic 000068_payments implement run, task 2.1)
  Design rule rounds half-even; success metric said half-up. User chose the design.
- **2026-10-09: design project:billing/rounding.md** (interactive implement run, task 3.2)
  Rounding rule clarified for zero-value invoices.
```

**Plan changelog entry field (template contract).** The `07-update_changelog` entry format gains an optional `**Amendments**:` line, after `**Deviations**`: "<document and section amended>: <reason>". It is omitted when no amendment was applied during the task.

## Implementation Detail

**A recorded edit, not a new workflow.** The plan adds no FSM step and no new workflow kind. A mid-run amendment is a stop inside the existing implement steps, followed by one CLI call made by whichever agent talks to the user. This follows the existing shape:
- the plan-versus-code STOPs in each implement step are prose;
- the orchestrated hand-back partial converts any STOP into a `QUESTION:`;
- the user-facing agent acts on the answer.

A developer reading the templates sees one new shared partial, included the same way the existing `implement-current-task` and `implement-code-locations` partials are. Orchestrated-only wording stays inside an `{{#orchestrated}}` block, so standalone runs never mention an orchestrator.

**`spec amend` follows the existing command pattern.** It is a typed `--data` struct plus an optional `--from` body and `--schema`, modelled on `epic write`.
- It is registered directly on `spec` rather than under `spec file`, because it is a domain operation with its own validation, not a generic store verb.
- Internally it is a short pipeline:
  1. load config and store;
  2. validate input;
  3. read the stored spec and require its plan;
  4. when `--from` is given, split both bodies into `## ` sections and compare them with checkbox marks normalised, which yields the changed set;
  5. when `design` is given, resolve it against the spec's design references;
  6. append the `## Amendments` entry;
  7. hash the final body;
  8. merge an `Amendments` replacement into the metadata and write once.
- `--dry-run` reports the would-be result without writing.
- Section splitting and comparison are small pure helpers, unit-tested apart from the command.

**The metadata change follows the `Designs` and `Sources` precedent exactly.**
- The field goes on the struct, both YAML shapes, the marshal and unmarshal pair, a lenient decoder, a pointer option on `UpdateOptions`, and carry-forward in both `Merge` branches.
- The exact-bytes render tests gain the new key in its fixed position after `sources`.
- The body hash lives beside the metadata, so the command and the status package share one definition.

**The staleness change is a guarded early return.** `PlanIsStale` still answers "is the spec newer than the plan". Before the mtime comparison, it reads the spec once and returns not-stale when the last amendment's hash equals the current body hash. Any read or parse failure falls through to the existing comparison, so a malformed spec never hides staleness. No caller changes.

**The skills gain sections, not new skills.**
- `spek-implement` gets a user-facing section placed before its final orchestrated section, which must stay last and self-contained.
- `spek-implement-epic` extends its hand-back definition, child prompt and Step 6. All new CLI mentions go through `{{command}}`, and any new "ask" wording is added to the test's allow-list.
- This repo's installed `.claude/skills` copies are deliberately not regenerated.

**Docs follow the site's existing components.** New prose sits inside existing `Section`/`Prose` bodies as `###` subsections, or as one new `Section` on the documents page. There is no layout markup in MDX, and no em dashes.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **`internal/metadata`**: the frontmatter model, `Merge`, `Split` and `Render`. Changed: it gains the `Amendments` field, its option and carry-forward, and the shared body hash.
- **`internal/status`**: the staleness predicate and its consumers. Changed: `PlanIsStale` gains the amendment exemption. Consumers are unchanged.
- **`internal/store`**: the store `Reader`/`Store` interface. Used as-is; no contract change, since staleness already reads through `st`.
- **`internal/design`** and the `design ref` lookup: resolve a design source and path, and list a spec's references. Used as-is by `spec amend`.
- **`internal/output`**: structured errors with `next_action`. Used as-is.
- **`cmd` (`spec` command tree, `runEpicWrite` pattern, test helpers `runRootCmd`/`resetRootCmd`/`writeArtifactStatusFile`)**: changed. A new `spec amend` subcommand is added.
- **`internal/stepkit` partial rendering and the `orchestrated-stop` partial**: used as-is. The new partial is rendered through the existing embedded-partial mechanism.
- **Standard library `crypto/sha256`**: the body hash. No new third-party dependency.
- **Prior work that must already be in place (landed):**
  - 000064_epic-worktree-store-isolation and 000065_implement-in-worktrees: children run from the project root, read specs and designs from the project store, and the merge guard refuses `.spektacular` changes on spec branches.
  - 000055_design-authoring-skill: `design author` preserves the capture date and back-links.

  This plan relies on all of these and changes none of them.
- **Docs repo (`spektacular-website`)**: Astro 5 + MDX with existing `Section`/`Prose` components. No new components or packages.
- **Harbor implement suite**: unaffected oracles (step order unchanged). A manual harbor run is a recommended check because step templates change.

## Testing Approach

**Test types and where they go.**

- **Unit tests (metadata).** They follow the existing exact-bytes, round-trip and carry-forward pattern used for `designs` and `sources`. Guarantees:
  - `amendments` renders in its fixed position and round-trips.
  - An ordinary `Merge` (any `spec file write`, including `reconcile_spec`) carries it forward untouched.
  - A non-nil option replaces it.
  - `BodyHash` ignores checkbox marks but changes on any other edit.
- **Unit tests (staleness).** These are table tests over `PlanIsStale` with pinned modification times. Guarantees:
  - A spec newer than its final plan is not stale when its body hash matches the last recorded amendment.
  - It is stale again after any later unrecorded body edit.
  - A frontmatter-only write or checkbox ticking after an amendment keeps it not-stale.
  - A spec with no amendments behaves exactly as today.
  - Unreadable or malformed specs fall back to the mtime rule.
- **Command tests (`spec amend`).** These run through the root command with the shared reset helpers. Guarantees:
  - A success-metric amendment lands in the store with new text, a `## Amendments` entry carrying date, section, reason and run, and a metadata record.
  - Changed sections are derived by the CLI.
  - Each refusal returns its code and a runnable `next_action`: missing reason or run, nothing to record, no plan, no change, non-amendable section changed, rewritten amendment history, unreferenced or unresolved design.
  - A design record keeps the spec body otherwise unchanged.
  - `--dry-run` writes nothing.
  - `--schema` describes the input.
  - Nothing is written outside the project's spec store.
- **Integration tests (strict mode end to end through the CLI).**
  - With `plan.strict_spec_changes: true`, after `spec amend`, `status <spec>` does not report stale and `implement new` (including an orchestrated lane resume) proceeds instead of refusing with `plan_stale`.
  - After a plain `spec file write` edit, both still report and refuse as stale.
  - A design revised with `design author` then recorded keeps its capture date and every spec's reference.
- **Template-contract tests (step templates).** Guarantees:
  - The analyze, implement, test and verify steps render the spec-conflict stop.
  - Standalone renders contain the ask-the-user and apply-after-approval wording, and no orchestrator wording.
  - Orchestrated renders state that a child never writes spec or design text, that it proposes the amendment through its hand-back, and that it re-reads and re-verifies after the answer.
  - The changelog step's entry format includes the `**Amendments**` field.
- **Skill-contract tests.**
  - `spek-implement`: the new user-facing section and the orchestrated-section line.
  - `spek-implement-epic`: the extended genuine-question definition, child prompt, apply-an-approved-amendment step and store-access rule. Every CLI mention goes through `{{command}}`, and every new "ask" wording is in the allow-list.
- **Docs repo.** The site build and type check must pass, along with the no-layout-HTML guard on MDX pages.

**Most coverage** goes to `spec amend` and `PlanIsStale`, because they are the only code that can silently corrupt a spec or hide staleness. The template and skill wording is covered by phrase assertions, as the project's testing architecture prescribes for prose-driven behaviour.

**Deliberate gaps.**
- No automated test drives a real agent through a conflict. Agent behaviour against the prose is covered by a manual harbor run and a manual epic-run check, below.
- The pre-existing strict-mode staleness after `reconcile_spec` on an unamended spec is not tested or fixed. It is out of scope.

**Success metrics.**
- *"In an epic run where verification reveals a spec/design conflict, the run completes without hand-editing files, without `override_dependencies`, and without writing anything inside a worktree's `.spektacular`."* **Manual — captured in the implementation test plan.** It needs a real orchestrated epic run with a seeded conflict. The integration tests cover the CLI half: amend writes only the project store, and the lane resumes under strict mode.
- *"After such a run, the spec's text matches what was built, and its `## Amendments` section explains every mid-run change without needing to read the plan changelog."* **Behavioural test** for the mechanics: amend writes the new text and a complete entry. **Manual — captured in the implementation test plan** for the end-to-end judgement on a real run.
- *"No epic merge is refused with `epic_merge_touches_spektacular` because of a mid-run spec or design change."* **Behavioural test** for the mechanism: `spec amend` and `design author` write only under the project store, never a worktree. **Manual — captured in the implementation test plan** for the observed epic merge in the real run.

**Manual reviews.**
- **Manual — captured in the implementation test plan:** a harbor implement-workflow run, because the step templates change (testing-architecture convention).
- **Manual — captured in the implementation test plan:** a read-through of the updated docs pages in the built site, to check the new content reads correctly and alternates section shading.

## Milestones & Tasks

### Milestone 1: A spec can be amended and recorded without making its plan stale

**What changes**: A user, or the agent acting for them, can run `spektacular spec amend` to change a spec's requirements, acceptance criteria, constraints or success metrics while it is being implemented.
- The spec then carries a dated `## Amendments` entry saying what changed, why, and in which run.
- The same command records a revision made to a design the spec references.
- Under `plan.strict_spec_changes: true`, a recorded amendment no longer makes the plan stale: `status` keeps reporting the plan as current, and `implement new`/`goto` carry on.
- A spec edited any other way still goes stale exactly as before.

On its own, this already gives a user a safe way to correct a spec mid-run by hand.

**Validation point**: With strict mode on, amend a planned spec's success metric with `spec amend`. The spec shows the new text and an Amendments entry; `status` does not report stale; `implement new` proceeds. A plain `spec file write` edit is still reported stale and refused. The full Go test suite passes.

#### - [ ] Task: Record amendments in spec metadata
**Id:** f237fe1d-6081-43f2-afa6-094f0ee9d792
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Teach the spec frontmatter model a new `amendments` list, so each recorded mid-run amendment is stored with its moment, the sections or design it changed, and a hash of the resulting spec body. It is modelled the same way design references are, so every ordinary spec write carries it forward instead of dropping it. A single shared body-hash helper ignores checkbox marks, so ticking boxes never looks like an edit.

*Technical detail:* [context.md#task-record-amendments-in-spec-metadata](./context.md#task-record-amendments-in-spec-metadata)

**Acceptance criteria**:
- [ ] A spec carrying amendments keeps them, unchanged, through any ordinary spec write, including the implement workflow's checkbox reconciliation.
- [ ] Amendments render in a fixed position in the frontmatter and read back identically.
- [ ] The body hash is identical for two bodies that differ only in checkbox marks, and different for any other change.

#### - [ ] Task: Honour recorded amendments in plan staleness
**Id:** 7330e44b-3c2c-4965-8918-a519a5e7c3ac
**Repo:** spektacular
**Depends on:**
- f237fe1d-6081-43f2-afa6-094f0ee9d792 — Record amendments in spec metadata
**Execution:** agent

Extend the single staleness check every command uses, so that a spec whose current body matches its last recorded amendment does not make its plan stale. Any other spec edit, including one made after an amendment, keeps today's behaviour. Because status, implement, dependency checks and the epic run view all share this check, they all pick up the exemption together.

*Technical detail:* [context.md#task-honour-recorded-amendments-in-plan-staleness](./context.md#task-honour-recorded-amendments-in-plan-staleness)

**Acceptance criteria**:
- [ ] A final plan whose spec was changed only by recorded amendments is not reported stale.
- [ ] A spec edited without recording an amendment, before or after an amendment, still makes its plan stale exactly as today.
- [ ] Frontmatter-only changes and checkbox ticks after an amendment do not make the plan stale.
- [ ] A spec with no amendments, or one that cannot be read, behaves exactly as today.

#### - [ ] Task: Add the spec amend command
**Id:** 00ffba92-573f-41d0-b9ed-9fab1c86c381
**Repo:** spektacular
**Depends on:**
- f237fe1d-6081-43f2-afa6-094f0ee9d792 — Record amendments in spec metadata
**Execution:** agent

Add `spektacular spec amend`, the one supported way to change a spec's requirements, acceptance criteria, constraints or success metrics mid-run, or to record a revision made to a design the spec references. The command works out which sections changed itself, and refuses anything outside the amendable sections. It appends a dated `## Amendments` entry with the reason and run, and stores the text and the metadata record in one write. Every refusal names the problem and the command to run instead.

*Technical detail:* [context.md#task-add-the-spec-amend-command](./context.md#task-add-the-spec-amend-command)

**Acceptance criteria**:
- [ ] Amending a success metric leaves the spec holding the new text, plus an Amendments entry with date, section, reason and run.
- [ ] Recording a design revision adds an Amendments entry naming the design and leaves the rest of the spec unchanged.
- [ ] The command refuses, with a corrective next step, when:
  - the reason or run is missing;
  - there is nothing to record;
  - the spec has no plan;
  - nothing changed;
  - a non-amendable section or a past amendment entry was changed;
  - the design is not referenced by the spec or does not resolve.
- [ ] A dry run reports the result without writing anything, and nothing is ever written outside the project's spec store.

#### - [ ] Task: Prove a recorded amendment keeps a strict-mode run going
**Id:** 4b92bea1-c4c0-4992-8fd4-f6bf35e8e5ba
**Repo:** spektacular
**Depends on:**
- 7330e44b-3c2c-4965-8918-a519a5e7c3ac — Honour recorded amendments in plan staleness
- 00ffba92-573f-41d0-b9ed-9fab1c86c381 — Add the spec amend command
**Execution:** agent

Add end-to-end command tests with `plan.strict_spec_changes` on, showing that an amendment made with `spec amend` leaves the plan current and lets implement start or resume, including an orchestrated lane. A plain spec edit is still refused as before. A design revised and recorded mid-run keeps its capture date and every spec's reference to it.

*Technical detail:* [context.md#task-prove-a-recorded-amendment-keeps-a-strict-mode-run-going](./context.md#task-prove-a-recorded-amendment-keeps-a-strict-mode-run-going)

**Acceptance criteria**:
- [ ] In strict mode, after a recorded amendment, status does not report the plan stale, and starting or resuming implement (interactive or orchestrated lane) proceeds.
- [ ] In strict mode, after an unrecorded spec edit, status reports stale and implement is refused, as today.
- [ ] A design revised and then recorded keeps its capture date, and every spec that referenced it still does.

### Milestone 2: Implement runs stop on a wrong spec or design and pick up the amendment

**What changes**: When implementing, testing or verifying shows that the spec or a referenced design is wrong, the implement workflow stops instead of working around it.
- **Interactive run:** it asks the user. After approval, it applies the amendment with `spec amend` (or revises the design and records it), re-reads the amended text and re-verifies the task.
- **Orchestrated child:** it never edits the spec or design. It hands back a question naming the document, section, conflict and proposed amendment, and continues against the amended text once the orchestrator answers.
- **Epic orchestrator:** the epic skill tells it how to put the question to the user, apply an approved amendment from the project root and answer the child.
- **Record:** each task's plan changelog entry records any amendment made during it.

**Validation point**: Rendered implement steps and both skills contain the new stop, apply and re-verify wording, with orchestrator wording only in orchestrated renders. The template and skill contract tests pass. The full Go test suite passes.

#### - [ ] Task: Add the spec-conflict stop to the implement steps
**Id:** d1d2f969-a3dc-4c28-bd54-e07aae42c679
**Repo:** spektacular
**Depends on:**
- 00ffba92-573f-41d0-b9ed-9fab1c86c381 — Add the spec amend command
**Execution:** agent

Give the analyze, implement, test and verify steps a shared instruction for when the spec or a referenced design turns out to be wrong. The run stops and names the document, section, conflict and proposed amendment.
- **Interactive run:** asks the user, applies an approved amendment with `spec amend` (or revises and records the design), then re-reads and re-verifies the task.
- **Orchestrated child:** never edits the spec or design. It hands the proposal back and continues against the amended text once answered.

The plan changelog entry for the task also gains a line recording any amendment.

*Technical detail:* [context.md#task-add-the-spec-conflict-stop-to-the-implement-steps](./context.md#task-add-the-spec-conflict-stop-to-the-implement-steps)

**Acceptance criteria**:
- [ ] The analyze, implement, test and verify instructions all describe the stop for a wrong spec or design, and what to name when stopping.
- [ ] Interactive instructions say to ask the user, apply only after approval, then re-read and re-verify, and never mention an orchestrator.
- [ ] Orchestrated instructions say the child never writes spec or design text, proposes the amendment through its hand-back, and re-reads and re-verifies after the answer.
- [ ] The plan changelog entry format includes a field naming the amended document, section and reason.

#### - [ ] Task: Describe the amendment path in the implement skill
**Id:** 3339cefc-a5d4-4cb3-80ad-c7d34fcf89a1
**Repo:** spektacular
**Depends on:**
- d1d2f969-a3dc-4c28-bd54-e07aae42c679 — Add the spec-conflict stop to the implement steps
**Execution:** agent

Add a section to the `spek-implement` skill explaining, for interactive runs, when the spec or a design is wrong, who approves an amendment, and how it is applied with `spec amend` and recorded. Its orchestrated section gains one line: a child never writes spec or design text and proposes amendments only through its hand-back.

*Technical detail:* [context.md#task-describe-the-amendment-path-in-the-implement-skill](./context.md#task-describe-the-amendment-path-in-the-implement-skill)

**Acceptance criteria**:
- [ ] The implement skill describes raising, approving, applying and recording an amendment in an interactive run.
- [ ] The skill's orchestrated section states a child never writes spec or design text, and that section remains the last one.

#### - [ ] Task: Teach the epic orchestrator to apply approved amendments
**Id:** 12379e12-728d-43a3-bbbf-b01ccf133a4a
**Repo:** spektacular
**Depends on:**
- 00ffba92-573f-41d0-b9ed-9fab1c86c381 — Add the spec amend command
**Execution:** agent

Extend the `spek-implement-epic` skill in four places:
- **Genuine question:** a wrong spec or design counts as one.
- **Child prompt:** says the child never edits spec or design text.
- **Store-access rule:** names `design` and `spec amend`.
- **Answering a child:** once the user approves an amendment, the orchestrator applies it from the project root (with `spec amend`, or by revising the design and recording it), notes it, and tells the child what changed so it re-reads and re-verifies.

*Technical detail:* [context.md#task-teach-the-epic-orchestrator-to-apply-approved-amendments](./context.md#task-teach-the-epic-orchestrator-to-apply-approved-amendments)

**Acceptance criteria**:
- [ ] The epic skill's definition of a genuine question includes the spec or a referenced design being wrong.
- [ ] The child prompt states the child never writes spec or design text and proposes amendments through its hand-back.
- [ ] The skill describes applying an approved amendment from the project root only after the user's explicit approval, then answering the child with what changed.
- [ ] Every command the skill names is rendered through the installed command name.

### Milestone 3: The docs describe the amendment path

**What changes**: The documentation site explains, on the how-it-works and epics pages, when an implement run stops for a wrong spec or design, who approves an amendment, and how it is applied and recorded.
- The documents page lists `spec amend`.
- The design-documents page notes that a design can be revised mid-run after approval.
- The configuration page documents `plan.strict_spec_changes`, including the recorded-amendment exemption.
- The CLI README carries the same exemption.

**Validation point**: The site builds and type-checks cleanly. The MDX layout-HTML guard returns no matches. Each page names the stop, the approval, `spec amend` and the `## Amendments` record.

#### - [ ] Task: Document the amendment exemption in the README
**Id:** 0081434f-3ca0-4c23-b12c-b2fdf2a196b4
**Repo:** spektacular
**Depends on:**
- 7330e44b-3c2c-4965-8918-a519a5e7c3ac — Honour recorded amendments in plan staleness
**Execution:** agent

Update the CLI README's description of `plan.strict_spec_changes` to say that a spec change recorded with `spec amend` does not make the plan stale, while any other spec edit still does.

*Technical detail:* [context.md#task-document-the-amendment-exemption-in-the-readme](./context.md#task-document-the-amendment-exemption-in-the-readme)

**Acceptance criteria**:
- [ ] The README's strict-mode description names `spec amend` as the exemption, and says other edits still make the plan stale.

#### - [ ] Task: Document the amendment path on the implement and epics pages
**Id:** 1925e88d-1418-4a15-b9c6-1384e1432160
**Repo:** docs
**Depends on:**
- d1d2f969-a3dc-4c28-bd54-e07aae42c679 — Add the spec-conflict stop to the implement steps
- 12379e12-728d-43a3-bbbf-b01ccf133a4a — Teach the epic orchestrator to apply approved amendments
**Execution:** agent

Add to the how-it-works page's implement stage and to the epics page an explanation of what happens when an implement run finds the spec or a design is wrong:
- the run stops;
- in an epic the child hands the question to the orchestrator;
- the user approves an amendment;
- it is applied in the project with `spec amend` and recorded on the spec and in the plan changelog;
- the run carries on.

*Technical detail:* [context.md#task-document-the-amendment-path-on-the-implement-and-epics-pages](./context.md#task-document-the-amendment-path-on-the-implement-and-epics-pages)

**Acceptance criteria**:
- [ ] The how-it-works implement stage mentions the stop and the approved amendment, and links to the fuller epics explanation.
- [ ] The epics page has a subsection describing raising, approving, applying and recording a mid-run amendment.
- [ ] The site builds and type-checks cleanly, with no layout markup in page bodies and no em dashes.

#### - [ ] Task: Document spec amend and strict spec changes in the reference pages
**Id:** 4f958095-4040-4607-9305-59774ecccd7a
**Repo:** docs
**Depends on:**
- 00ffba92-573f-41d0-b9ed-9fab1c86c381 — Add the spec amend command
- 7330e44b-3c2c-4965-8918-a519a5e7c3ac — Honour recorded amendments in plan staleness
**Execution:** agent

Update the reference pages:
- **Documents page:** lists `spec amend` with its input and what it records.
- **Design-documents page:** notes that a design can be revised mid-run after the user approves.
- **Configuration page:** documents `plan.strict_spec_changes`, including the recorded-amendment exemption.
- **Plan-tasks page:** notes the exemption beside the stale state.
- **Docs changelog:** gains an entry for this spec.

*Technical detail:* [context.md#task-document-spec-amend-and-strict-spec-changes-in-the-reference-pages](./context.md#task-document-spec-amend-and-strict-spec-changes-in-the-reference-pages)

**Acceptance criteria**:
- [ ] The documents page lists `spec amend` with an example and states which sections can be amended.
- [ ] The design-documents page explains that a design can be revised during a run after approval, keeping its capture date and references.
- [ ] The configuration page documents `plan.strict_spec_changes`, its default, and that recorded amendments do not make a plan stale.
- [ ] The plan-tasks stale state mentions the exemption, and the docs changelog has an entry for this spec.
- [ ] The site builds and type-checks cleanly, with no layout markup in page bodies and no em dashes.

## Open Questions

- **Does any existing spec in the store use a `## ` heading layout that the section splitter mis-parses?** Examples would be a heading inside a fenced code block, or setext headings. This depends on real spec bodies, and only shows up when `spec amend` runs against them. The splitter must ignore headings inside fenced code blocks. If an implementer finds another layout that breaks the diff, they should STOP and ask the user before widening the splitter's rules.

No other implementation-time uncertainties remain. Every other decision is resolved in the architecture and the assumption log.

## Out of Scope

- **Re-planning.** An amendment that invalidates the plan's tasks is not handled: the run stops and the user re-plans as today (spec Non-Goals).
- **Editing a spec outside an implement run.** The spec workflow and `spec file write` already cover this. `spec amend` requires a planned spec and is documented for mid-run use only (spec Non-Goals).
- **#80 and #78 items 1, 4 and 5.** These are being fixed separately (spec Non-Goals).
- **Pre-existing staleness after `reconcile_spec` on an unamended spec.** Under `plan.strict_spec_changes: true`, the reconcile step's checkbox rewrite likely makes an unamended plan stale and blocks `finished`. That predates this spec and is not changed here; it is worth its own spec.
- **Amending the Overview, Technical Approach or Non-Goals.** Only Requirements, Acceptance Criteria, Constraints and Success Metrics are amendable (accepted spec default).
- **Rewriting a team-owned design on Spektacular's behalf.** A design stored with `design write` is only replaced with a version the user supplies (spec Constraints).
- **Regenerating this repository's installed skills or migrating its `.spektacular/`.** The changed skill templates reach this repo only through a later `make install-local` and version check (convention: a plan never changes the active install).
