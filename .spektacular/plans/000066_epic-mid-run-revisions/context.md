---
created_date: "2026-10-08"
document_status: final
closed_date: "2026-10-08"
---

# Context: 000066_epic-mid-run-revisions

## Current State Analysis

- **Staleness is a pure mtime comparison with no exemptions.** `status.PlanIsStale` (`internal/status/classify.go:136-153`) compares the spec file's mtime with `plans/<name>/plan.md`'s mtime for a `final` plan.
  - `status.DocumentStatus` (`:123-131`) applies `plan.strict_spec_changes` (`internal/config/config.go:111-117`).
  - Consumers: `refuseStalePlan` (`cmd/implement.go:370-390`, called at `:173` and `:301-305`), `status` (`internal/status/report.go:437-473`), `DependenciesOf` (`report.go:261`), `EpicComplete` (`report.go:214`) and the epic run view (`internal/status/run.go:270-293`).
- **No approval hash or spec version is recorded anywhere.** Plan approval is `metadata.Close` on each plan document (`internal/steps/plan/steps.go:328-338`).
- **The spec frontmatter schema is closed.** `internal/metadata/metadata.go:65-211` drops unknown keys on render (`frontmatter.go:69-84`). `metadata.Merge` (`merge.go:65-173`) carries known fields forward.
- **The implement workflow can only tick spec checkboxes** (`templates/steps/implement/11-reconcile_spec.md`). Its STOPs are all framed as plan-versus-code (`02-analyze.md:45-47`, `03-implement.md:29-37`, `04-test.md:37-39`, `05-verify.md:33-39`). Nothing covers the spec or a design being wrong.
- **Orchestrated runs append `templates/partials/orchestrated-stop.md`** (`internal/stepkit/stepkit.go:137-143`), turning any STOP into a `QUESTION: <plan_name>` hand-back.
- **The epic skill's genuine-question definition** (`templates/skills/workflows/spek-implement-epic/SKILL.md:79-81`) omits spec and design conflicts. Step 6 (`:95-102`) relays answers but has no apply-an-amendment path.
- **Designs:** `design author` preserves the capture date and back-links (`cmd/design.go:545-643`). `design write` over an authored design is refused (`:384-403`).
- **The epic merge guard** refuses any spec branch that changes `.spektacular/**` (`internal/worktree/worktree.go:763-803`).
- **Likely pre-existing behaviour (traced, not run):** `reconcile_spec`'s full spec rewrite makes an unamended plan stale under strict mode, so `goto finished` is refused. This is out of scope.

## Per-Task Technical Notes

### Task: Record amendments in spec metadata

**File changes:**
- `internal/metadata/metadata.go:65-96` — add `Amendments []Amendment` to `Metadata`, with a comment in the style of `Designs`: closed schema, only a spec carries it, only `spec amend` appends.
  - Add `type Amendment struct { At string; Sections []string; Design *DesignRef; Hash string }` next to `DesignRef` (`:110-113`), with yaml/json tags `at`, `sections,omitempty`, `design,omitempty`, `hash`.
- `internal/metadata/metadata.go:118-130` — add `Amendments []Amendment \`yaml:"amendments,omitempty"\`` to `yamlShape`, after `Sources` (render order: `... epic, sources, amendments`).
- `internal/metadata/metadata.go:134-152` — add the decode twin to `yamlInShape` as a raw `yaml.Node`.
  - Write a lenient decoder beside the existing ones (`:219-282`) that skips malformed entries rather than failing the whole block.
- `internal/metadata/metadata.go:155-211` — copy the field in `MarshalYAML`/`UnmarshalYAML`.
- `internal/metadata/merge.go:11-42` — add `Amendments *[]Amendment` to `UpdateOptions`: nil carries forward, non-nil replaces. Document it next to `Designs`.
- `internal/metadata/merge.go:86-113,115-144` — apply the option in the fresh-write branch, and carry forward or replace in the existing-record branch, mirroring `Designs` (`:129-132`).
- `internal/metadata/hash.go` (new) — `BodyHash(body []byte) string` returns `"sha256:" + hex(sha256(normalised))`.
  - Normalisation: regex `(?m)^(\s*[-*+] )\[[xX]\]` → `${1}[ ]`.
  - Also normalise CRLF to LF, so a Windows checkout hashes the same.
- `internal/metadata/metadata_test.go:198,1018,1091` — extend the exact-bytes render tests with an `amendments` block, and confirm it is omitted when empty.
  - Add a round-trip case beside `:994`.
- `internal/metadata/merge_test.go` — add carry-forward and replace cases following the `Designs` pattern (`:29-400`).
- `internal/metadata/hash_test.go` (new) — table tests for `BodyHash`:
  - checkbox-only differences (`[x]`, `[X]`, nested items) give the same hash;
  - text changes, an added line or a changed heading give different hashes;
  - CRLF and LF give the same hash.
- `cmd/storefile_metadata_test.go:837` — add `TestStoreFileWrite_SpecAmendmentsSurviveOrdinaryWrite`, modelled on the design-references test.

**Complexity:** Medium
**Token estimate:** ~25k tokens
**Agent strategy:** Single agent, sequential. The metadata model, merge and tests are tightly coupled.

### Task: Honour recorded amendments in plan staleness

**File changes:**
- `internal/status/classify.go:136-153` — in `PlanIsStale`, after resolving `specName` and before the mtime comparison, read the spec via `st.Read(specPath(cfg, specName))` and `metadata.Split` it.
  - If the read and parse succeed, and `len(specFM.Amendments) > 0` and `specFM.Amendments[len-1].Hash == metadata.BodyHash(body)`, return false.
  - Any error falls through to the existing comparison.
  - Update the doc comment to describe the exemption.
- `internal/status/resolve.go:109-115` — reuse the `specPath`/`planPath` helpers unchanged.
- `internal/status/status_test.go:467` — beside `TestBuild_StalePlan`, add a table test `TestPlanIsStale_RecordedAmendments`. Use the existing `e.touch` helper to pin mtimes so the spec is newer than the plan. Cases:
  - (a) no amendments → stale;
  - (b) last amendment hash matches the body → not stale;
  - (c) body edited after the amendment → stale;
  - (d) checkbox-only edit after the amendment → not stale;
  - (e) frontmatter-only change (e.g. `epic:` added) after the amendment → not stale;
  - (f) malformed spec frontmatter → falls back to mtime (stale);
  - (g) a non-final plan → not stale, as today.
- `cmd/status_test.go:425-457` — add `TestStatus_StrictSpecChangesIgnoresRecordedAmendment` using `writeArtifactStatusFile` (`:24`) with pinned mtimes and a spec carrying a matching amendment hash. It asserts the state is not `stale`.

**Complexity:** Low
**Token estimate:** ~12k tokens
**Agent strategy:** Single agent, sequential.

### Task: Add the spec amend command

**File changes:**
- `cmd/spec_amend.go` (new) — `var specAmendCmd = &cobra.Command{Use: "amend", ...}`, with an `init()` registering `--data` and `--from` and calling `specCmd.AddCommand(specAmendCmd)`. Follow `cmd/spec.go:409-422` for registration, and the persistent `--schema`/`--dry-run` at `:410-411`.
  - `--schema` returns a `commandSchema` (`cmd/spec.go:33-58`) describing input `{name, reason, run, design?}` and the output.
  - `runSpecAmend` follows `runEpicWrite` (`cmd/epic.go:270-330`):
    1. Unmarshal `--data` into `specAmendInput{Name, Reason, Run string; Design *metadata.DesignRef}` → `bad_input` on error.
    2. Validate `Name` with `nameRegexp` (`cmd/spec.go:24`).
    3. Require a non-empty reason (`spec_amend_reason_required`) and run (`spec_amend_run_required`).
    4. Require `--from` or `design` (`spec_amend_nothing_to_record`).
    5. Load config and the store as `spec file write` does (`cmd/storefile.go:116`, `store.NewSourceStore`).
    6. Read the spec (`spec_not_found`, next action `spec file list`).
    7. Require `plans/<name>/plan.md` to exist via `st.Exists` and the plan path helper (`spec_amend_no_plan`, next action: edit with `spec file write`; amend is for planned specs).
    8. With `--from`: read the file, `stripLeadingFrontmatterBlocks` (`cmd/storefile.go:69`), split the old and new bodies by `## ` heading, and compare each section after checkbox normalisation (reuse the `metadata.BodyHash` normaliser via an exported `metadata.NormaliseCheckboxes`).
       - Changed set empty → `spec_amend_no_change`.
       - Any changed section outside {Requirements, Acceptance Criteria, Constraints, Success Metrics} → `spec_amend_section_not_amendable`, naming it.
       - The new `## Amendments` section must equal the old one → otherwise `spec_amend_section_not_amendable` ("amendment history is append-only").
       - Added or removed sections count as changed.
    9. With `design`: check it is in the spec's `Designs` → `spec_amend_design_not_referenced`, next action `design ref add`. Then `design.NewSet(cfg, root)` and `set.Resolve` (`internal/design/design.go:95,296`; pattern in `cmd/design_ref.go:354`) → `spec_amend_design_unresolved`.
    10. Build the entry `- **<YYYY-MM-DD>: <Section, Section | design source:path>** (<run>)\n  <reason>` and append it under `## Amendments` at the end of the body, creating the heading if absent. When both `--from` and `design` are given, one entry names both.
    11. Compute `hash := metadata.BodyHash(finalBody)` and `at := time.Now().UTC().Format(time.RFC3339)`.
    12. Build `next := append(existing.Amendments, Amendment{...})`, then `metadata.Merge(existingRaw, finalBody, metadata.UpdateOptions{Amendments: &next})` and `st.Write`. Skip the write under `--dry-run`.
  - Output: `{spec, amended_sections, design, recorded_at, hash, dry_run}`.
  - Every error is built with `output.NewError(code, msg).WithResource(name).WithNextAction(...)` (`internal/output/writer.go:60-81`).
- `cmd/spec_amend_section.go` (new, or inside `spec_amend.go`) — pure helpers `splitSections(body) []section` and `changedSections(old, new) []string`, unit-tested separately.
- `cmd/spec_amend_test.go` (new) — through `runRootCmd` (`cmd/root_test.go:70`), with config written by `writeSpecCommandConfig` (`cmd/spec_test.go:26`) and a plan fixture like `writeFixturePlan` (`cmd/implement_test.go:19`). Errors are decoded as `output.ErrorResponse` (`cmd/implement_test.go:127-133`). Cases:
  - success-metric amendment: new text, entry content, metadata record, derived section, hash equals `BodyHash` of the stored body;
  - design record only;
  - `--from` plus design;
  - each refusal code, with a non-empty `next_action`;
  - `--dry-run` writes nothing (file bytes unchanged);
  - `--schema` output;
  - existing `designs`, `sources`, `epic` and `created_date` are preserved;
  - nothing is written outside the spec store (the temp project tree diff contains only the spec file).
- `cmd/spec_amend_section_test.go` (new) — table tests for the section split and diff, including checkbox-only diffs, added and removed sections, and heading-level edge cases (`###` stays inside its `##`).

**Complexity:** High
**Token estimate:** ~45k tokens
**Agent strategy:** Parallel analysis, sequential integration. One agent writes the pure section helpers and their tests while another drafts the command skeleton and error paths. Integrate and run the command tests sequentially.

### Task: Prove a recorded amendment keeps a strict-mode run going

**File changes:**
- `cmd/implement_test.go:120` — beside `TestImplementNew_StrictModeRejectsStalePlan`, add:
  - `TestImplementNew_StrictModeAcceptsRecordedAmendment`: fixture plan final, `plan.strict_spec_changes: true`, run `spec amend` with `--from` changing Success Metrics, then `implement new` succeeds.
  - `TestImplementNew_StrictModeStillRejectsUnrecordedEdit`: `spec file write` edit → `plan_stale`.
  - Use `setupImplementCmd` (`:57-70`).
  - Pin mtimes with `os.Chtimes` so the spec is strictly newer than the plan regardless of filesystem resolution (convention: tests must not depend on order or timing).
- `cmd/implement_lane_test.go:70` — add `TestImplementLane_OrchestratedResumeAfterRecordedAmendment`: start an orchestrated lane (`"orchestrated":true`), amend the spec, re-run `implement new` with the same data → it resumes rather than `plan_stale`. Use `laneProject` (`:27`).
- `cmd/status_test.go:425` — strict status after amend via the CLI (complementing the unit test in the staleness task): `status <spec>` is not `stale`, and after a further `spec file write` body edit it is `stale`.
- `cmd/design_test.go` (or the existing design author test file) — add `TestDesignAuthorThenSpecAmendKeepsCaptureDateAndRefs`:
  - `design author` a design, `design ref add` it to two specs, `design author` a revision, `spec amend --data {design}` on one spec;
  - assert the design's `created_date` is unchanged, both specs still list the reference, and the amended spec has the entry.

**Complexity:** Medium
**Token estimate:** ~20k tokens
**Agent strategy:** Single agent, sequential.

### Task: Add the spec-conflict stop to the implement steps

**File changes:**
- `templates/partials/implement-spec-conflict.md` (new) — heading `### If the spec or a design is wrong`. Body:
  - **When to stop:** implementing, testing or verifying shows that a requirement, acceptance criterion, constraint or success metric of the spec (`{{config.command}} spec file read {{plan_name}}`), or a rule in a design the spec references (`{{config.command}} design ref list --data '{"spec":"{{plan_name}}"}'` then `design read`), is wrong or contradicts another. STOP, do not work around it, and name the document, the section, the conflict and a proposed amendment.
  - `{{^orchestrated}}` block:
    - ask the user whether to amend;
    - only after explicit approval, stage the full amended spec under `.spektacular/tmp/{{plan_name}}/spec_amend.md` and run `{{config.command}} spec amend --data '{"name":"{{plan_name}}","reason":"…","run":"interactive implement run, task …"}' --from .spektacular/tmp/{{plan_name}}/spec_amend.md`, then `rm` the scratch file;
    - for a design: revise it with `design author` (or `design write` with the version the user supplies, for a design Spektacular did not author), then record it with `spec amend --data '{…,"design":{"source":"…","path":"…"}}'`;
    - an amendment that invalidates the plan's tasks is out of scope: stop and re-plan.
  - `{{#orchestrated}}` block:
    - never write the spec's text or a design document yourself;
    - put the proposed amendment in the hand-back;
    - when the answer says it was applied, continue as below.
  - Shared closing: re-read the amended spec or design through the CLI, re-run this step's check of the current task against the amended text before advancing, and record the amendment under the task's changelog entry.
  - Bodies are supplied with `--from` only (no heredoc or stdin wording; `internal/agent/instruction_surface_test.go:79,128`).
- `templates/steps/implement/02-analyze.md:45-47`, `03-implement.md:29-37`, `04-test.md:37-39`, `05-verify.md:33-39` — add `{{> partials/implement-spec-conflict}}` right after each existing STOP section.
- `templates/steps/implement/07-update_changelog.md:15-30` — add `**Amendments**: <document and section>: <reason> (omit when no amendment was applied during this task)` after `**Deviations**` in the entry format.
  - The orchestrated wording at `:59-70` stays as is.
- `internal/steps/implement/steps_test.go:437-456` — add `TestSpecConflictStopInAnalyzeImplementTestVerify`, which renders each of the four steps standalone and asserts the shared heading, `spec amend`, "explicit approval" and "re-run".
  - Add `TestUpdateChangelogEntryHasAmendmentsField`.
- `internal/steps/implement/orchestrated_test.go:19-53` — add `TestOrchestratedSpecConflictNeverWritesSpec`, which renders the four steps with `orchestrated: true` and asserts "never write the spec's text or a design document yourself" and "QUESTION:" (from the appended partial), and the absence of the `spec amend --from` apply instruction.
- `cmd/orchestrated_test.go:94-104` — the existing standalone guard must still pass (no "orchestrat" or "QUESTION:" in standalone renders). The new partial keeps orchestrated wording inside `{{#orchestrated}}`.
- `cmd/instruction_contract_test.go:829` — confirm the scratch path `.spektacular/tmp/{{plan_name}}/` satisfies the "scratch in the spec's own folder" contract.

**Complexity:** Medium
**Token estimate:** ~25k tokens
**Agent strategy:** Single agent, sequential. The partial is shared, so the wording must be settled once.

### Task: Describe the amendment path in the implement skill

**File changes:**
- `templates/skills/workflows/spek-implement/SKILL.md:146` — before `# When an orchestrator starts this skill`, add `# When the spec or a design is wrong`. It covers: stop, name the document, section, conflict and proposal; ask the user; apply only after approval with `{{command}} spec amend` (or revise the design with `design author`/`design write` and record it with `spec amend`); re-read and re-verify; record in the task's changelog entry.
  - It must not contain `QUESTION:`, `DONE:`, `FAILED:` or `"orchestrated":true` (`templates/orchestrated_skill_section_test.go:94-111`).
- `templates/skills/workflows/spek-implement/SKILL.md:152` — add to the orchestrated section: "You never write the spec's text or a design document; propose any amendment in your `QUESTION:` hand-back and continue once your orchestrator says it is applied."
  - Do not use the word "worktree" (`:82-89` test).
- `templates/orchestrated_skill_section_test.go:24-53` — add the new orchestrated phrase to the `spek-implement` entry in `orchestratedSkillCases`.
  - Add `TestSpekImplementDescribesSpecAmendment`, modelled on `:122-146`, pinning the new section's phrases.
- `internal/agent/instruction_surface_test.go:213-261` — add `require.Contains` for `spec amend` in the rendered `spek-implement` subtest.
- Do **not** regenerate `.claude/skills/` in this repo (convention: plans never change the active install).

**Complexity:** Low
**Token estimate:** ~10k tokens
**Agent strategy:** Single agent, sequential.

### Task: Teach the epic orchestrator to apply approved amendments

**File changes:**
- `templates/skills/workflows/spek-implement-epic/SKILL.md:79-81` — extend the genuine-question sentence: "...the plan no longer matching the code, the spec or a design it references being wrong, a task outgrowing its scope, ...".
- `templates/skills/workflows/spek-implement-epic/SKILL.md:66-69` — add a child-prompt line: the child never writes the spec's text or a design document, and proposes any amendment (document, section, conflict, proposed change) in its hand-back.
- `templates/skills/workflows/spek-implement-epic/SKILL.md:23` — the store-access rule adds `design` and `spec amend`.
- `templates/skills/workflows/spek-implement-epic/SKILL.md:95-102` (Step 6) — add a paragraph "**An amendment the user approves.**". It covers:
  - only on the user's explicit approval, from the project root, apply it with `{{command}} spec amend --data '{"name":"<spec>","reason":"…","run":"epic <epic> implement run, task …"}' --from <staged spec>`;
  - or revise the design with `{{command}} design author` (or `{{command}} design write` with the user's own version) and record it with `{{command}} spec amend --data '{…,"design":{…}}'`;
  - never in a worktree;
  - note it in `.spektacular/working-context.md`;
  - answer the child naming what changed so it re-reads and re-verifies;
  - on a decline, answer the child with the decision.
- `templates/implement_epic_skill_test.go:263-298` — update the verbatim genuine-question pin (`:274-282`) and child-prompt pins (`:284-287`). Add `requirePhrases` for the new Step 6 paragraph.
  - If the new text contains any `ask…` word, add the exact phrase to `allowed` at `:205-213`.
  - `TestImplementEpicSkillRendersLikeAnInstalledSkill` (`:53-82`) requires every command via `{{command}}`.
- Do **not** regenerate `.claude/skills/` in this repo.

**Complexity:** Medium
**Token estimate:** ~18k tokens
**Agent strategy:** Single agent, sequential.

### Task: Document the amendment exemption in the README

**File changes:**
- `README.md:41` — append: "A spec change recorded with `spektacular spec amend` during an implement run does not make the plan stale; any other edit to the spec's body still does."
- `README.md:234` — update the inline comment: `# true marks final plans stale after later spec edits (recorded amendments excepted)`.

**Complexity:** Low
**Token estimate:** ~3k tokens
**Agent strategy:** Single agent, sequential.

### Task: Document the amendment path on the implement and epics pages

**File changes:**
- `docs:src/pages/how-it-works.mdx:463-468` — after the gap-check and changelog paragraph in the "Implement the Spek" `PipelineStage` body slot, add one paragraph.

  **Content example:**

  > If implementing shows the spek itself, or a design it references, is wrong, the run stops rather than working around it. You approve an amendment, it is applied in the project with `spektacular spec amend` and recorded on the spek, and the run carries on against the corrected text. See [when the spek or a design is wrong](/epics#when-the-spek-or-a-design-is-wrong).
- `docs:src/pages/epics.mdx:393-395` — inside the "Planning and implementing an epic" `Prose`, add a new `###` subsection between "What still stops for you" and "When something fails". Add one sentence to "What still stops for you" (`:379-393`) pointing at it.

  **Content outline:**
  - `### When the spek or a design is wrong`
  - Paragraph on the stop: "A child that finds a requirement, acceptance criterion, constraint or success metric is wrong, or that the spek and a design disagree, stops and hands the question back. It names the document, the section, the conflict and the amendment it proposes. It never edits the spek or the design itself."
  - Paragraph on approval and applying: "The orchestrator puts the question to you. If you approve, it applies the amendment in the project, never in a worktree:"

    ```bash
    spektacular spec amend --data '{"name":"<spek>","reason":"<why>","run":"epic <epic> implement run, task <task>"}' --from .spektacular/tmp/<spek>/spec_amend.md
    ```

    "A design is revised with `spektacular design author` (or `design write` with your own version), then recorded with `spec amend` and a `design` field."
  - Paragraph on the record, with a short fenced markdown example of a `## Amendments` entry:

    ```markdown
    ## Amendments

    - **2026-10-09: Success Metrics** (epic 000068_payments implement run, task 2.1)
      Design rule rounds half-even; the metric said half-up.
    ```

    "The child's plan changelog entry for the task records it too."
  - Paragraph on staleness: "A recorded amendment does not make the plan stale, even with `plan.strict_spec_changes`, so the child re-reads the corrected text, re-checks its task and carries on. An amendment that invalidates the plan's tasks still means re-planning."
  - The same flow applies in a single interactive `/spek-implement` run, where you are asked directly.
- No new components, no layout HTML, no em dashes.
- Verify with `npm run build` and `npx astro check`, plus `grep -nE "<div|<section|class=" src/pages/*.mdx` returning nothing.

**Complexity:** Low
**Token estimate:** ~12k tokens
**Agent strategy:** Single agent, sequential.

### Task: Document spec amend and strict spec changes in the reference pages

**File changes:**
- `docs:src/pages/documents.mdx:51-85` — in the "Specs: spec file" `Section`'s `Prose`, after the `spec file` command list (`:62-81`), add a `### Amending a spek during implementation` subsection. This needs no new top-level `Section`, so no shading change.

  **Content example:**

  > `spektacular spec amend` changes a planned spek's Requirements, Acceptance Criteria, Constraints or Success Metrics during an implement run, and records why.
  >
  > ```bash
  > spektacular spec amend --data '{"name":"000070_billing","reason":"Metric contradicted the rounding design","run":"interactive implement run, task 2.1"}' --from .spektacular/tmp/000070_billing/spec_amend.md
  > ```
  >
  > It works out which sections changed, refuses changes to any other section, appends a dated entry to the spek's `## Amendments` section, and records the amendment in its metadata. Pass `"design":{"source":"…","path":"…"}` (with or without `--from`) to record a revision made to a design the spek references.
- `docs:src/pages/design-documents.mdx:191-193` — after "no command rewrites one to make it match", add:

  **Content example:**

  > The one exception is an implement run that finds a design is wrong: after you approve, the design is revised with `design author` (keeping its capture date and references) and the change is recorded on the spek with `spec amend`.
- `docs:src/pages/configuration.mdx:53-58` — add `strict_spec_changes: false` to the example `plan:` block.
- `docs:src/pages/configuration.mdx:223-244` — add a bullet to the `plan` `ConfigKey` slot:

  **Content example:**

  > - `plan.strict_spec_changes`: when `true`, a final plan is reported `stale`, and implement refuses it, once its spek is edited after approval. Defaults to `false`. A change recorded with `spec amend` during an implement run does not make the plan stale.
- `docs:src/pages/plan-tasks.mdx:392-393` — extend the `stale` line: "...under `plan.strict_spec_changes`, unless the change was recorded with `spec amend`."
- `docs:CHANGELOG.md` — add `## 000066_epic-mid-run-revisions` at the top, following the latest entry's format, summarising the pages changed. No em dashes.
- Verify with `npm run build` and `npx astro check`, plus the MDX layout guard.

**Complexity:** Low
**Token estimate:** ~12k tokens
**Agent strategy:** Single agent, sequential.

## Testing Strategy

Per task:
- **Record amendments in spec metadata:** exact-bytes and round-trip render tests with `amendments`; `Merge` carry-forward and replace tests; `BodyHash` table tests (checkbox, CRLF, text change); and the ordinary-write survival test in `cmd/storefile_metadata_test.go`.
- **Honour recorded amendments in plan staleness:** a `PlanIsStale` table test with pinned mtimes covering no amendment, matching hash, a later edit, checkbox-only, frontmatter-only, malformed and non-final cases; and a strict status test with a matching hash.
- **Add the spec amend command:** command tests for success, design record, combined, every refusal code and `next_action`, dry run, schema and metadata preservation; and pure section-diff table tests.
- **Prove a recorded amendment keeps a strict-mode run going:** strict-mode `implement new` accepts after amend and refuses after an unrecorded edit; an orchestrated lane resumes after amend; status stale or not stale through the CLI; and design author plus amend keeps the capture date and references.
- **Add the spec-conflict stop to the implement steps:** standalone render assertions for steps 02-05 and the changelog field; orchestrated render assertions; and the existing standalone no-orchestrator guard still passes.
- **Describe the amendment path in the implement skill:** section phrase pins, the orchestrated-section case, and the rendered-skill `Contains`.
- **Teach the epic orchestrator to apply approved amendments:** updated verbatim pins, new `requirePhrases`, the `ask` allow-list and the `{{command}}` rendering test.
- **Docs tasks and README:** `npm run build`, `npx astro check` and the MDX layout guard.
- **Every task:** `go test ./...` (shuffle on via `make test`) must pass.

Success metrics and manual items:
- The epic-run metric and the merge metric are partly behavioural (amend and design author write only the project store) and partly **Manual — captured in the implementation test plan**: a real orchestrated epic run with a seeded spec/design conflict, observing no hand edits, no `override_dependencies`, no worktree `.spektacular` writes, and a clean `epic merge`.
- The Amendments-explains-everything metric is behavioural for the entry content, and manual for the end-to-end judgement.
- Also manual: a harbor implement-workflow run, and a read-through of the built docs pages.

## Project References

- Spec: `spektacular spec file read 000066_epic-mid-run-revisions`.
- Design documents: none referenced by the spec.
- Knowledge (spektacular):
  - conventions: error-messages-must-suggest-remediation, store-files-must-be-written-through-the-cli, plans-never-change-the-active-install, tests-must-not-depend-on-order, tests-must-pass-for-done;
  - architecture: workflow-steps, working-with-files-from-steps, testing-architecture;
  - gotchas: mustache-html-escaping.
- Knowledge (docs): conventions mdx-authoring, no-em-dashes, plan-content-pages, site-layout, alternate-section-background.
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

- No migration is needed. `amendments` is optional and omitted when empty, so existing specs render byte-for-byte as before. There is no settings-format change.
- Do not migrate or re-init this repository's own `.spektacular/` or regenerate its installed skills (convention: a plan never changes the active install).

## Performance Considerations

- `PlanIsStale` now reads and hashes the spec once per call. Specs are small (kilobytes) and status already reads every spec and plan, so the cost is negligible.
