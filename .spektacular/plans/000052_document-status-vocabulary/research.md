---
created_date: "2026-09-16"
status: completed
closed_date: "2026-09-16"
---

# Research: 000052_document-status-vocabulary

## Alternatives considered and rejected

- **Accept old values as aliases (`in-progress`→`draft`, `completed`→`final`) on read and on `--status`.** Proposed in issue #36 as the main compatibility option. Rejected by the user during the spec ("we should not have backward compatibility"); captured as a spec constraint.
- **One-shot migration command that rewrites existing artifacts.** Rejected by the user; the user will update this repo's ~93 artifacts separately, outside this plan.
- **Keep the field name `status` and change only the values.** The issue's open question. Rejected by the user in favour of `document_status`.
- **Reject unknown values on read (current behaviour, `internal/metadata/metadata.go:93-95`).** Rejected: the spec requires reads never to fail on document status; old values read as blank.
- **Keep three separate enum switches (`metadata.go:112`, `cmd/artifactfilter.go:72`, `cmd/storefile.go:53`).** Rejected in favour of one exported validator in `internal/metadata`: the rename would otherwise need three hand-synced edits, and the spec's technical approach prefers a single source of truth.
- **Omit `document_status` from rendered frontmatter when blank (`omitempty`).** Rejected: an explicit `document_status: ""` makes the blank state visible to a reader and matches today's always-present key; either choice reads back as blank.

## Chosen approach — evidence

- `internal/metadata/metadata.go:55-63` — `yamlShape` is the only place the on-disk key is named; renaming its tag renames the field everywhere.
- `internal/metadata/metadata.go:83-96` — `UnmarshalYAML` decodes via `node.Decode` without `KnownFields`, so a legacy `status:` key is silently ignored once the tag changes. Only the `validateStatus` call has to become lenient.
- `internal/metadata/metadata.go:57` — `Status` is decoded as a typed string; a non-scalar YAML value (e.g. a list) would fail decode, so lenient reading must decode the status as a raw node/`any` and normalise.
- `internal/metadata/merge.go:62-100` — defaulting, preservation, closed-date stamping and clearing already implement every lifecycle rule in the spec; with `current.Status == ""`, `isClosed("")` is false and a nil `opts.Status` preserves both the blank status and `closed_date`. Only constant names change.
- `internal/metadata/close.go:16` — single close helper; its four callers (`internal/steps/spec/steps.go:178`, `internal/steps/plan/steps.go:324`, `internal/steps/implement/steps.go:159`, `:168`) pass `StatusCompleted`.
- `cmd/artifactfilter.go:39` — filter compares `m.Status != f.status`, so a blank artifact never matches a `draft`/`final` filter; `active()` at :29 treats empty as no filter.
- `cmd/artifacts.go:73,104,168` and `cmd/storefile.go:236,287,318,338,351-413` — every flag, subcommand and output key to rename.
- `internal/metadata/frontmatter.go:60-63` — `Render` does not preserve unknown keys, so the legacy `status:` key is dropped on the next write automatically.

## Files examined

- `internal/metadata/metadata.go:16-124` — Status type, constants, yamlShape tag, Unmarshal validation, validateStatus, isClosed.
- `internal/metadata/merge.go:1-102` — UpdateOptions, Merge lifecycle rules; default `StatusInProgress` at :63.
- `internal/metadata/close.go:16-30` — Close helper.
- `internal/metadata/frontmatter.go:10-60` — Split/Render; unknown keys dropped.
- `internal/metadata/metadata_test.go` — table-driven, `statusPtr` helper :19, raw `status:` fixtures (:40,53,68,86,141,157,210,216,344,434,493,520), `status: bogus` expects error at :86.
- `internal/metadata/close_test.go:14-125` — fakeStore; StatusCompleted assertions.
- `cmd/artifactfilter.go:17-79` — filter struct, parseListFilter enum switch and `invalid_status` error.
- `cmd/artifactfilter_test.go` — asserts `invalid_status` at :83,:92; constants throughout.
- `cmd/storefile.go:42-59` — metadataOptsForStatus duplicate enum switch.
- `cmd/storefile.go:185,215,236` — write `--status` flag.
- `cmd/storefile.go:275,287,318,338` — list `--status` flag and `item["status"]`.
- `cmd/storefile.go:351-413` — set-status subcommand, `missing_status` error, payload `"status"` :402, `MarkFlagRequired("status")` :411.
- `cmd/artifacts.go:58,73,104,168` — artifacts list flag and `entry["status"]`.
- `cmd/storefile_metadata_test.go` — kindFixtures :38, reset helpers :76/:97 (flag names "status", "set-status"), raw fixture :632.
- `cmd/storefile_list_filter_test.go:20,190-203,242,377,424,468` — listFlagNames, `["status"]` assertions, `--status` args.
- `cmd/artifacts_test.go:25-27,245-452` — artifactsListFlagNames, `["status"]` assertions.
- `cmd/changelog_file_test.go:39,390`, `cmd/file_test.go:36,66`, `cmd/plan_file_test.go:33,63,187` — status assertions.
- `internal/steps/spec/steps.go:82,178`, `internal/steps/plan/steps.go:324`, `internal/steps/implement/steps.go:159,168` — default and close call sites.
- `internal/steps/spec/steps_test.go:337-428`, `internal/steps/plan/steps_test.go:272-415`, `internal/steps/plan/planstill_test.go:35`, `internal/steps/implement/steps_test.go:75,638,656,708` — lifecycle assertions.
- `internal/store/frontmatter.go:11-25` — doc comment only mentions "a status from a fixed enum".
- `templates/steps/plan/19-finished.md:26` — "the documents are now marked completed" (only lifecycle wording in templates).
- `tests/harbor/implement-workflow/tests/test_implement_workflow.py:113,287-300` — `STATUS_COMPLETED = "completed"` oracle, asserts `fm.get("status")`.
- `tests/harbor/implement-workflow/environment/spec.md:3`, `test-plan.md:3` — seeded `status: in-progress` fixtures.
- `docs:src/pages/projects.mdx:319-327` — frontmatter example with `status: in-progress`.
- `docs:src/pages/how-it-works.mdx:382-384` — "**Status**: … (for example, `in_progress` or `completed`)".
- `docs:src/pages/configuration.mdx:130-132,144-146,160-162` — prose "a created date, a status, and … a closed date".
- Not lifecycle (leave alone): `cmd/version.go` status, `internal/workflow/workflow.go:364-377` step status, `internal/steps/*/result.go:14`, `status` subcommands in `cmd/{spec,plan,implement}.go`, "in-progress workflow" resume wording, `docs/knowledge-base.md`, `README.md`.

## External references

- `gopkg.in/yaml.v3` `Node.Decode` — ignores unknown mapping keys unless `KnownFields(true)` is set on a Decoder; why the legacy key needs no special handling.

## Prior plans / specs consulted

- `000038_artifact_metadata/plan.md` — introduced the metadata package, `set-status`, the filter flags and `artifacts list`; confirms metadata sits above the byte-oriented store and that the terminal close rides `Close`/`Merge`. Historical context only.
- Knowledge `architecture/testing-architecture.md` — harbor suites hold hand-maintained oracles that must change in the same change as the surface they mirror; harbor does not run in CI.

## Open assumptions

- The only lifecycle wording in `templates/` is `templates/steps/plan/19-finished.md:26`; other "completed"/"in-progress" hits are ordinary English or workflow-resume wording.
- No other code path (store, workflow, search index) validates or reads the status value.
- Blank status is rendered as `document_status: ""`.
- No installed agent copies (`.claude/skills`, `.bob`) need hand edits; they regenerate from `templates/` on init.

## Drafting assumptions

### Chosen direction: rename in place, lenient read / strict input (architecture)
- **Decision**: Rename the field, Go type, constants, flags, subcommand, output keys and error codes in place; make `UnmarshalYAML` normalise any unrecognised status to blank; route all CLI validation through one exported `internal/metadata` check; Go names `DocumentStatus`, `StatusDraft`, `StatusFinal`, `StatusSuperseded`, `StatusArchived`, field `Metadata.DocumentStatus`; error codes `invalid_document_status` / `missing_document_status`.
- **Rationale**: Merge/Close already implement the lifecycle rules; the spec calls for a rename with a single source of truth, lenient reads and strict input.
- **Rejected**: (a) keep Go identifiers as `Status` and rename only the YAML tag/flags (cheaper, but leaves the old vocabulary in code and invites drift); (b) a separate normalisation layer in `cmd` over a still-strict parser (reads would still fail in Split callers such as Close and workflow steps).

### Conventions selected (architecture)
- **Decision**: Apply error-remediation, tests-must-pass, no-em-dashes, MDX authoring, and plan-content-pages conventions; drop alternate-section-background, file-scoped-section-headings and site-layout.
- **Rationale**: The dropped ones govern new sections/layout, which this change does not add.
- **Rejected**: Listing all loaded conventions.

### Website how-it-works lifecycle line is in scope (discovery)
- **Decision**: Update `src/pages/how-it-works.mdx:382-384` (which names `in_progress`/`completed` as lifecycle examples) and the "a status" prose in `configuration.mdx`, alongside the `projects.mdx` example.
- **Rationale**: The spec's acceptance criterion requires the website's pages to carry no lifecycle reference to the retired values; the user's "only the example" answer was given before this line was found, and it is a one-line wording fix, not a new reference section.
- **Rejected**: Leaving it, which would fail the acceptance criterion.

### Harbor implement suite oracle and fixtures are in scope (discovery)
- **Decision**: Update `tests/harbor/implement-workflow` oracle (`STATUS_COMPLETED`, `fm.get("status")`) and seeded fixtures.
- **Rationale**: Project knowledge (testing-architecture) requires harbor oracles to change with the surface they mirror.
- **Rejected**: Leaving harbor drift for the next runner.

### Single exported validator (discovery)
- **Decision**: Replace the three enum switches with one exported check in `internal/metadata`.
- **Rationale**: Spec technical approach prefers a single source of truth; the rename otherwise needs three synced edits.
- **Rejected**: Renaming each switch in place.

### Blank status rendered explicitly (discovery)
- **Decision**: Render a blank status as `document_status: ""` (no omitempty).
- **Rationale**: Matches today's always-present key and makes the blank state visible.
- **Rejected**: Omitting the key when blank.

### Error codes renamed (data_structures)
- **Decision**: `invalid_status` -> `invalid_document_status`, `missing_status` -> `missing_document_status`.
- **Rationale**: The error codes name the flag/field that is now `document_status`; no backward compatibility is required.
- **Rejected**: Keeping the old codes (leaves the retired vocabulary on the CLI surface).

### Two milestones split on the Go compile boundary (milestones)
- **Decision**: Milestone 1 bundles metadata, CLI and workflow close sites; Milestone 2 covers templates, harbor and the website.
- **Rationale**: Renaming the Go type and constants breaks every caller at once, so the Go changes cannot land in smaller independently compiling pieces without a temporary alias layer the spec forbids.
- **Rejected**: A separate CLI milestone (would not compile on its own); one single milestone (hides the independently shippable docs/harbor work).

### Phase 1.1 includes compile-only caller renames (phases)
- **Decision**: Phase 1.1 renames Go identifiers in `cmd` and `internal/steps` just enough to compile; flag/key/command renames are Phase 1.2.
- **Rationale**: Each phase must leave the tree building and tests passing.
- **Rejected**: Leaving cmd broken between phases.

### Plan finished-step wording and its contract test (phases)
- **Decision**: Change "marked completed" to "marked final" and update the `internal/steps/plan/steps_test.go:539` phrase assertion.
- **Rationale**: It is the only agent-facing lifecycle wording and the spec requires agent guidance to use the new vocabulary.
- **Rejected**: Leaving the phrase (it tells the agent documents are "completed").

## Rehydration cues

- `go run . spec file read 000052_document-status-vocabulary.md`
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`
- `grep -rn -E 'StatusInProgress|StatusCompleted|"status"|--status|set-status' --include='*.go' cmd internal`
- Re-read `internal/metadata/metadata.go`, `internal/metadata/merge.go`, `cmd/artifactfilter.go`, `cmd/storefile.go:42-59,351-413`, `cmd/artifacts.go`.
- `grep -rn status tests/harbor/implement-workflow`
