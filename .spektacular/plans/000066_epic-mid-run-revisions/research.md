---
created_date: "2026-10-08"
document_status: final
closed_date: "2026-10-08"
---

# Research: 000066_epic-mid-run-revisions

## Alternatives considered and rejected

- **A. Write `plan.md` after the spec so mtimes line up.** `spec amend` writes the spec, then appends to or re-closes `plan.md`, so `PlanIsStale`'s mtime comparison (`internal/status/classify.go:152`) returns false. Rejected because:
  - it depends on write order and mtime resolution;
  - it keeps the loophole where any later plan write hides an earlier unrecorded spec edit;
  - it means the CLI writes the plan while a child may be mid-step writing `plan.md` itself;
  - a git checkout resets mtimes, so it would be invisible after a clone.
- **C. Record the approved spec hash on the plan (`spec_hash`), with staleness defined as a hash mismatch.** Rejected because it changes behaviour for every unrecorded edit. For example, frontmatter-only writes such as epic link (`cmd/epic_link.go:236`) and design ref add (`cmd/design_ref.go:182`) would stop counting as staleness. That contradicts the spec's requirement that "a spec edit made any other way keeps today's staleness behaviour". It also needs a hash stamped at plan approval (`internal/steps/plan/steps.go:328-338`), and amend would have to write the plan.
- **A free-form `## Amendments` section written by the agent with `spec file write`.** Rejected: the spec's Technical Approach says record and exemption must not drift. Only a CLI operation can guarantee that the prose record, the metadata record and the exemption agree.
- **A flag on `spec file write` (e.g. `--amendment`) instead of a new verb.** Rejected because `spec file write` is the generic `newStoreFileCmd` shared by spec, plan and changelog (`cmd/storefile.go:196`, `cmd/file.go:10-18`), so a spec-only flag would leak into the shared builder. A dedicated `spec amend` also needs a "record only" form, used after a design revision, that does not take a full body.
- **Letting a child write the spec or design from its worktree.** Rejected by the spec's constraints and by the merge guard `epic_merge_touches_spektacular` (`internal/worktree/worktree.go:763-803`), which refuses any spec branch touching `.spektacular/**`.
- **A new FSM step ("amend") in the implement workflow.** Rejected: conflicts surface inside existing steps (analyze, implement, test, verify), and the existing STOP semantics plus the orchestrated hand-back partial (`templates/partials/orchestrated-stop.md`) already turn a STOP into `QUESTION:`. A new step would also change the harbor `EXPECTED_STEP_ORDER` oracle (`tests/harbor/implement-workflow/tests/test_implement_workflow.py:57`).

## Chosen approach — evidence

These cover option B: amendments are recorded in the spec's frontmatter with a body hash, and `PlanIsStale` honours them.

- `PlanIsStale` is the only staleness predicate. Every consumer goes through it:
  - `refuseStalePlan` (`cmd/implement.go:370-390`, called at `:173` for `implement new` and at `:301-305` for every `implement goto`);
  - `status.DocumentStatus` (`classify.go:123-131`) → `buildPlan` (`internal/status/report.go:437`), `EpicComplete` (`report.go:214`), `DependenciesOf` (`report.go:261`) and the epic run view (`internal/status/run.go:270-293`).
  So one change covers status, implement, dependencies and the epic run.
- The metadata schema is closed: unknown frontmatter keys are dropped on render (`internal/metadata/frontmatter.go:69-84`, `metadata.go:76-80`). A new `amendments` field therefore has to be modelled in `Metadata` (`metadata.go:65-96`), `yamlShape`/`yamlInShape` (`:118-152`), `MarshalYAML`/`UnmarshalYAML` (`:155-211`), and in the carry-forward in `Merge` (`merge.go:65-173`).
- `Merge` carries every current field forward on an ordinary write (`merge.go:115-144`), with pointer options meaning "replace" (`Designs`, `Specs`, `Epic`, `Sources`). The same pattern lets `spec file write` and `reconcile_spec` keep `amendments` intact.
- Hashing only the body, via `metadata.Split`, keeps frontmatter-only writes from changing the hash. A body hash with checkbox marks normalised (`[ ]`/`[x]`) means `reconcile_spec`'s ticking (`templates/steps/implement/11-reconcile_spec.md:25-35`) does not invalidate a recorded amendment.
- When the spec carries no amendment, or its body no longer matches the last amendment's hash, staleness falls back to the existing mtime comparison. Unrecorded edits therefore behave exactly as today.
- Command patterns:
  - `runEpicWrite` (`cmd/epic.go:270-330`) is the model for `--data` + `--from` + `--schema`;
  - errors use `output.NewError(...).WithNextAction` (`internal/output/writer.go:60-81`);
  - registration is `specCmd.AddCommand` (`cmd/spec.go:409-422`).
- The design revision path already exists:
  - `design author` keeps the capture date and back-links (`cmd/design.go:545-643`, `opts.Specs` nil at `:579-580`);
  - `design write` over an authored design is refused (`cmd/design.go:384-403`);
  - a user-supplied (team) design is rewritten with `design write` (spek-design `SKILL.md:120-128`).
- Stop and hand-back plumbing:
  - every implement step already has STOP sections (`03-implement.md:29-37`, `04-test.md:37-39`, `05-verify.md:33-39`, `02-analyze.md:45-47`);
  - `orchestrated-stop.md` is appended automatically to orchestrated steps (`internal/stepkit/stepkit.go:137-143`) and turns any STOP into `QUESTION: <plan_name>`;
  - orchestrated-only wording must sit inside `{{#orchestrated}}` (`cmd/orchestrated_test.go:94-104`).
- The epic skill's definition of a genuine question (`templates/skills/workflows/spek-implement-epic/SKILL.md:79-81`) omits the spec or a design being wrong. spek-plan-epic `SKILL.md:70` already lists a design disagreement, which is precedent wording.
- The plan changelog entry format is `07-update_changelog.md:15-30`. `{{#has_dependency_override}}` (`:34-43`) is precedent for a structured extra under an entry.

## Files examined

- internal/status/classify.go:123-153 — `DocumentStatus`; `PlanIsStale` compares spec mtime with plan mtime and has no exemption.
- internal/status/resolve.go:109-115 — the spec and plan store paths used by the staleness check.
- internal/status/report.go:214,261,419-473 — `EpicComplete`, `DependenciesOf` and `buildPlan` all consume `DocumentStatus`.
- internal/status/run.go:270-293 — the epic run view treats a stale plan as not done.
- internal/store/store.go:29-32,240-253 — `FileInfo` holds only `ModTime`/`CreatedAt`; there is no Chtimes.
- internal/config/config.go:111-117 — `PlanConfig.StrictSpecChanges`, yaml `strict_spec_changes`.
- cmd/implement.go:173,301-305,370-390 — `refuseStalePlan` and its callers; the `plan_stale` error.
- internal/metadata/metadata.go:27-29,56,65-282 — the `Metadata` struct, the closed schema, the YAML shapes and the lenient decoders.
- internal/metadata/merge.go:11-173 — `UpdateOptions` and the `Merge` carry-forward.
- internal/metadata/frontmatter.go:69-84 — unknown keys are dropped.
- internal/metadata/close.go:16 — `Close` is used at spec finish and plan approval.
- internal/metadata/metadata_test.go:198,301,586,994,1018,1091 — exact-bytes and round-trip tests that pin the field set.
- internal/metadata/merge_test.go:29-400 — the per-field carry-forward test pattern.
- cmd/storefile.go:69,86-95,196,236-287,463-554 — the shared store-file builder, the write path and set-document-status.
- cmd/storefile_metadata_test.go:837 — `TestStoreFileWrite_SpecDesignReferencesSurviveOrdinaryWrite`, the model for "amendments survive an ordinary write".
- cmd/file.go:10-18 — spec file registration.
- cmd/spec.go:24,33-58,332-388,409-422 — `nameRegexp`, the `commandSchema` types, the goto parsing and subcommand registration.
- cmd/epic.go:270-330,492-498 — the `--data`/`--from`/`--schema` command template.
- cmd/design.go:51-67,213-237,348-418,545-658 — `design write`/`design author`, authored detection and the overwrite refusal.
- cmd/design_ref.go:24-51,182-186 — the reference verbs and typed `--data`; ref add rewrites the spec frontmatter.
- cmd/epic_link.go:236-242, cmd/epic_split.go:279,302, cmd/epic_order.go:21-26 — other spec writers. `epic_order` deliberately avoids staling plans.
- cmd/root_test.go:35,70 — `resetRootCmd`, `runRootCmd`.
- cmd/implement_test.go:19,45,57-70,120-133 — `setupImplementCmd`, `writeFixturePlan`, and the strict-mode stale test plus its error decoding.
- cmd/status_test.go:24,33,425-457 — `writeArtifactStatusFile` with pinned mtimes; the strict stale status test.
- internal/status/status_test.go:467 — `TestBuild_StalePlan`.
- internal/worktree/worktree.go:763-803 — the `epic_merge_touches_spektacular` guard.
- internal/steps/implement/steps.go:27-69,192-200,290-370 — step table, `writeStep`, reconcile_spec and Extra variables.
- internal/stepkit/stepkit.go:79-82,108-177,217-240 — template variables, orchestrated partial appending and partial rendering.
- templates/steps/implement/01-read_plan.md:9,19-26 — never touch a worktree's `.spektacular`. Designs are read only in a task run.
- templates/steps/implement/02-analyze.md:26,45-47; 03-implement.md:14,29-37; 04-test.md:14,37-39; 05-verify.md:14,33-39 — the current plan-vs-code STOPs, which say nothing about the spec or a design being wrong.
- templates/steps/implement/06-update_plan.md:28-30 — criterion mismatch options.
- templates/steps/implement/07-update_changelog.md:7-55 — the changelog entry format and the dependency-override precedent.
- templates/steps/implement/11-reconcile_spec.md:7-41 — checkbox-only spec rewrite.
- templates/partials/orchestrated-stop.md:6-9 — the STOP → `QUESTION:` hand-back contract.
- templates/partials/proceed-unless-blocked.md:3 — wording for recorded decisions and designs.
- templates/skills/workflows/spek-implement/SKILL.md:87,146-153 — the orchestrated section, which must stay last and self-contained.
- templates/skills/workflows/spek-implement-epic/SKILL.md:19,23,60-81,95-102 — the notes file, store-access rule, child prompt, hand-back contract, genuine-question definition and Step 6 answering.
- templates/skills/workflows/spek-design/SKILL.md:112-128 — the revise branch: `design author` for authored designs, `design write` for team designs.
- templates/implement_epic_skill_test.go:53-82,172-298 — phrase pins, the `{{command}}` regex and the `ask` allow-list trap (`:200-224`).
- templates/orchestrated_skill_section_test.go:24-146 — orchestrated-section pins; `QUESTION:` only in the last section.
- internal/agent/instruction_surface_test.go:180-262 — rendered-skill `Contains` checks.
- internal/steps/implement/steps_test.go:44-90,437-456,613,835-844 and orchestrated_test.go:19-53 — step rendering helpers and STOP pins.
- cmd/orchestrated_test.go:76-104 — standalone steps must not contain orchestrator wording.
- tests/harbor/implement-workflow/tests/test_implement_workflow.py:57 — `EXPECTED_STEP_ORDER`, which is unchanged by this plan.
- README.md:41,234 — documents `plan.strict_spec_changes`.
- docs:src/pages/how-it-works.mdx:432-470 — the "Implement the Spek" pipeline stage body.
- docs:src/pages/epics.mdx:315-411 — "Planning and implementing an epic", with "What still stops for you" at :377-393.
- docs:src/pages/documents.mdx:51-85 — the "Specs: spec file" command list.
- docs:src/pages/design-documents.mdx:174-216 — "no command rewrites one" (:191-193) needs a caveat.
- docs:src/pages/configuration.mdx:53-58,223-244 — the `plan` key, where `strict_spec_changes` is undocumented.
- docs:src/pages/plan-tasks.mdx:392-415,576-579 — the stale state and reconciling the spec.
- docs:CHANGELOG.md — one `## <spec-name>` entry per docs change.

## External references

- None needed. The feature is internal to Spektacular's CLI, templates and docs.

## Prior plans / specs consulted

- 000064_epic-worktree-store-isolation and 000065_implement-in-worktrees (via the spec session's working context): children run from the project root and read specs, plans and designs from the project store, and the merge guard refuses `.spektacular` changes. This is why amendments must be applied in the project by the user-facing agent.
- 000066 spec Technical Approach: the drafter's defaults are accepted (the user finished the spec without changing them).

## Open assumptions

- `reconcile_spec` under `plan.strict_spec_changes: true` makes the plan stale today, and the following `implement goto finished` is refused with `plan_stale`. This was traced in code (`11-reconcile_spec.md` full rewrite → `cmd/implement.go:303`), not run. The plan does not fix it for specs without amendments; that is pre-existing and out of this spec's scope. If it proves false, nothing in this plan changes.
- Children re-read the spec and designs through `spec file read` / `design read` from the project root, which works because 000065 children run from the project root.
- A design revision changes no spec body, so it does not affect staleness. Its record (`spec amend` without `--from`) appends only the `## Amendments` entry and the metadata record, and the new hash covers that.
- The orchestrator can send an answer back to a waiting child (spek-implement-epic `SKILL.md:98`). If it cannot, it starts a fresh child with the answer in the prompt.

## Drafting assumptions

### Staleness exemption via amendment hash in spec frontmatter (discovery)
- **Decision**: Record amendments as a modelled `amendments` frontmatter list on the spec. Each record carries a body hash with checkbox marks normalised. `PlanIsStale` returns false when the spec's current body hash equals the last amendment's hash. Otherwise it keeps the existing mtime comparison.
- **Rationale**: Unrecorded edits keep exactly today's behaviour, as the spec requires. Frontmatter-only writes and reconcile_spec ticking don't break a recorded amendment, and one predicate covers every consumer.
- **Rejected**: plan-mtime bump (A), which is fragile and racy; plan-side `spec_hash` (C), which changes behaviour for unrecorded edits.

### Pre-existing reconcile_spec strict-mode staleness left out of scope (discovery)
- **Decision**: Don't fix reconcile_spec making an unamended plan stale under strict mode.
- **Rationale**: It predates this spec and the spec's non-goals don't include it. Normalising checkboxes in the hash keeps amended specs working through reconcile.
- **Rejected**: Widening the exemption to all checkbox-only edits, which is scope creep and changes unrecorded-edit behaviour.

### Chosen direction: stop partial + `spec amend` verb + hash-based staleness exemption (architecture)
- **Decision**: Add a shared implement partial for spec and design conflicts, included in steps 02-05. Add `spektacular spec amend`, which derives changed sections by diff, appends `## Amendments` and records a frontmatter `amendments` entry with a checkbox-normalised body hash. `PlanIsStale` exempts when the current hash equals the last amendment's hash.
- **Rationale**: This satisfies the spec's direction that record and exemption must not drift. Unrecorded edits keep today's behaviour, no FSM or harbor step-order change is needed, and one predicate covers every consumer.
- **Rejected**: plan-mtime bump; plan-side spec hash; a new FSM amend step; a flag on the shared `spec file write`.

### CLI derives changed sections instead of trusting the caller (architecture)
- **Decision**: `spec amend --from` diffs old and new bodies by `## ` section (ignoring checkbox marks). It refuses a no-op or a change outside Requirements, Acceptance Criteria, Constraints and Success Metrics.
- **Rationale**: The record can't misstate what changed, and the spec's amendable-section scope is enforced.
- **Rejected**: a caller-supplied `sections` list, which can drift from the actual edit.

### Amend does not require a running implement workflow (architecture)
- **Decision**: `spec amend` requires the spec to exist and to have a plan, but does not check that an implement run is in progress.
- **Rationale**: Orchestrated lanes and interactive state live in different places, and the skills scope when it is used. Requiring a plan stops it being used as a general spec editor before planning.
- **Rejected**: enforcing an active run, which is complex and brittle across lanes.

### Amendment record format (architecture)
- **Decision**: The body entry under `## Amendments` is `- **<YYYY-MM-DD>: <sections or design source:path>** (<run>)` followed by an indented reason line. Frontmatter `amendments` holds `at` (RFC3339), `sections`, `design`, `hash`.
- **Rationale**: Human-readable on the spec. The machine record carries only what staleness and status need. No em dashes.
- **Rejected**: duplicating the reason and run into frontmatter.

### Past amendment entries are append-only (data_structures)
- **Decision**: `spec amend --from` refuses when the staged body's `## Amendments` section differs from the stored one. The CLI alone appends entries.
- **Rationale**: Keeps the human-readable record and the frontmatter record in step. `## Amendments` is not an amendable section.
- **Rejected**: letting the agent author or edit the entry text.

### Hash prefixed with algorithm (data_structures)
- **Decision**: Store the hash as `sha256:<hex>`.
- **Rationale**: Leaves room to change the algorithm without misreading old records.
- **Rejected**: a bare hex string.

## Rehydration cues

- `spektacular spec file read 000066_epic-mid-run-revisions`
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'` (harbor coupling)
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/working-with-files-from-steps.md"}'` (store rules)
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
- Re-read internal/status/classify.go:123-153, internal/metadata/metadata.go:65-211, internal/metadata/merge.go:11-173, cmd/epic.go:270-330, templates/partials/orchestrated-stop.md and templates/skills/workflows/spek-implement-epic/SKILL.md:60-102.
