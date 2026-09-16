---
created_date: "2026-09-16"
status: completed
closed_date: "2026-09-16"
---

# Context: 000052_document-status-vocabulary

## Current State Analysis

- **Where the lifecycle is defined.** `internal/metadata` owns the lifecycle schema.
  - The on-disk key is `status` (`internal/metadata/metadata.go:57`).
  - The values are `in-progress`, `completed`, `superseded` and `archived` (`metadata.go:22-30`).
  - Unknown values are rejected on read (`metadata.go:93-95`).
- **Merge rules.** `Merge` (`internal/metadata/merge.go:36-102`) defaults a first write to `in-progress`. It stamps `closed_date` once, on the first transition to a closed value, and clears it on a return to `in-progress`. When no update is given, it preserves the status.
- **Enum checks are duplicated.** The allowed values are hand-copied into three places: `metadata.go:112`, `cmd/artifactfilter.go:72` and `cmd/storefile.go:53`.
- **CLI surface.**
  - `--status` is accepted on `<kind> file write` (`cmd/storefile.go:236`), `<kind> file list` (`:338`) and `artifacts list` (`cmd/artifacts.go:104`).
  - `set-status` is defined at `cmd/storefile.go:351-413`.
  - The output key is `status` (`cmd/storefile.go:318,402`; `cmd/artifacts.go:168`).
- **Workflow close sites.** Workflows close their artifacts with `metadata.Close(..., StatusCompleted)` at `internal/steps/spec/steps.go:178`, `internal/steps/plan/steps.go:324` and `internal/steps/implement/steps.go:159,168`.
- **Agent-facing wording.** The only lifecycle wording is `templates/steps/plan/19-finished.md:26` ("marked completed"), which is asserted by `internal/steps/plan/steps_test.go:539`.
- **Harbor suite.** The implement-workflow suite hard-codes `STATUS_COMPLETED` and `fm.get("status")` (`tests/harbor/implement-workflow/tests/test_implement_workflow.py:113,294-300`). Its seeded fixtures carry `status: in-progress`.
- **Website** (docs repo). Three pages are affected: the frontmatter example in `src/pages/projects.mdx:322`, the lifecycle example in `src/pages/how-it-works.mdx:382-384` (which already uses the wrong spelling `in_progress`), and the prose in `src/pages/configuration.mdx:130-162`.

**Requirement to repo and files:**
- The field rename, vocabulary, lenient read, blank handling and closed-date rules are in **spektacular**: `internal/metadata/*` (Phase 1.1).
- Workflows closing as final is in **spektacular**: `internal/steps/{spec,plan,implement}/steps.go` (Phase 1.1).
- Strict CLI input, the renamed CLI surface, the renamed list output and the empty-filter behaviour are in **spektacular**: `cmd/storefile.go`, `cmd/artifactfilter.go`, `cmd/artifacts.go` (Phase 1.2).
- Agent-facing guidance is in **spektacular**: `templates/steps/plan/19-finished.md`, plus the harbor suite (Phase 2.1).
- Documentation is in **docs**: `src/pages/{projects,how-it-works,configuration}.mdx` (Phase 2.2). This repo's `docs/` directory has no lifecycle references, so it needs no change.

## Per-Phase Technical Notes

### Phase 1.1: Document status vocabulary and lenient reading in the metadata package

**File changes**:
- `internal/metadata/metadata.go:1-6`: update the package doc so it names `document_status`.
- `internal/metadata/metadata.go:16-30`: rename `type Status` to `DocumentStatus`, and the constants to `StatusDraft="draft"`, `StatusFinal="final"`, `StatusSuperseded`, `StatusArchived`. Add `DocumentStatuses()`, which returns them in lifecycle order, and `ParseDocumentStatus(raw string) (DocumentStatus, bool)`.
- `internal/metadata/metadata.go:35-48`: rename `Metadata.Status` to `DocumentStatus`, and update the doc comment (draft / blank are open).
- `internal/metadata/metadata.go:55-63`: in `yamlShape`, change the field to `DocumentStatus` with the tag `yaml:"document_status"`. Decode it via a separate lenient shape, where the status is a `yaml.Node` or `any`, so that a non-scalar value cannot fail the decode. Keep `MarshalYAML` emitting a string (a blank value is written as `document_status: ""`).
- `internal/metadata/metadata.go:83-96`: replace the `validateStatus` error path with normalisation. Use `ParseDocumentStatus` and, when it reports false, set the status to `""`. Add a doc comment explaining that reads are lenient and input is strict.
- `internal/metadata/metadata.go:111-124`: make `validateStatus` call `ParseDocumentStatus`. Its error message lists `DocumentStatuses()`, and it rejects `""` along with any unknown value. `isClosed` checks final/superseded/archived, so blank counts as open. Update its comment.
- `internal/metadata/merge.go:8-33`: rename `UpdateOptions.Status` to `DocumentStatus *DocumentStatus`, and update the comments (in-progress becomes draft).
- `internal/metadata/merge.go:63`: change the fresh-write default to `StatusDraft`. Keep lines 64-69 and 75-100 as they are, apart from the renames. The existing `result.Status = current.Status` preservation already keeps a blank status and `closed_date` when no update is given.
- `internal/metadata/close.go:11-25`: change the `Close` parameter to `status DocumentStatus`, and update the comment.
- `internal/store/frontmatter.go:11-25`: update the doc comment wording to "document status".
- `internal/steps/spec/steps.go:178`, `internal/steps/plan/steps.go:324`, `internal/steps/implement/steps.go:159,168`: change `metadata.StatusCompleted` to `metadata.StatusFinal`. `internal/steps/spec/steps.go:82` needs no change; it inherits the draft default.
- `cmd/artifactfilter.go:17,39,72`, `cmd/storefile.go:53,318,402`, `cmd/artifacts.go:168`: make the minimal identifier renames needed to compile (`metadata.Status` becomes `DocumentStatus`, `m.Status` becomes `m.DocumentStatus`). The flag and output-key renames belong to Phase 1.2.
- Tests:
  - `internal/metadata/metadata_test.go`: rename `statusPtr`, and change every raw `status:` fixture (:40, 53, 68, 141, 157, 210, 216, 344, 434, 493, 520) to `document_status:` with the new values. Change the `status: bogus` case (:86) from expect-error to expect-blank.
  - Add table cases to `metadata_test.go`:
    - legacy `status: completed` with no `document_status` reads as blank, with no error;
    - `document_status: in-progress` / `completed` read as blank;
    - `document_status: [a, b]` reads as blank, with no error;
    - missing key reads as blank;
    - merge of a legacy file with nil opts produces no `status:` key, keeps a blank status, and keeps `created_date` / `closed_date`;
    - blank → final with an existing `closed_date` keeps the date; blank → final without one stamps today;
    - `ParseDocumentStatus` accepts exactly the four values and rejects `""`, `in-progress`, `completed` and `bogus`.
  - `internal/metadata/close_test.go:46-125`: `StatusCompleted` becomes `StatusFinal`.
  - `internal/steps/spec/steps_test.go:337-428`, `internal/steps/plan/steps_test.go:272-415`, `internal/steps/plan/planstill_test.go:35`, `internal/steps/implement/steps_test.go:75,638,656,708`: new constants and assertion messages.
  - `cmd/*_test.go`: rename only the constants needed to compile (`StatusInProgress` becomes `StatusDraft`, `StatusCompleted` becomes `StatusFinal`). Leave the flag names until 1.2, and update raw `status:` fixtures, such as `cmd/storefile_metadata_test.go:632`, to `document_status: draft`.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: A single agent works sequentially. The rename is compile-coupled across packages, so do the metadata package first, then fix callers until `go build ./... && go test ./...` passes.

### Phase 1.2: CLI speaks document status

**File changes**:
- `cmd/storefile.go:42-59`: rename `metadataOptsForStatus` to `metadataOptsForDocumentStatus`. It validates via `metadata.ParseDocumentStatus` and returns error code `invalid_document_status` with the message `--document-status %q is not one of the four allowed values`. Its `WithNextAction` lists `metadata.DocumentStatuses()`.
- `cmd/storefile.go:185,215,236`: rename the write flag to `--document-status`. Build its help text from `DocumentStatuses()`, e.g. "Optional document status to apply: one of draft, final, superseded, archived".
- `cmd/storefile.go:275,287,318,338`: rename the list flag to `--document-status`, and change the output key `item["status"]` to `item["document_status"]`.
- `cmd/storefile.go:351-413`:
  - Rename the subcommand to `Use: "set-document-status <path>"`, with Short "Update the document status of a stored artifact without rewriting its body".
  - Rename the variables `setStatusFlag` / `setStatus` to `setDocumentStatusFlag` / `setDocumentStatus`.
  - Rename error `missing_status` to `missing_document_status`, with message "--document-status is required for set-document-status" and a next_action listing the values.
  - Change the payload key at :402 to `"document_status"`.
  - Rename the flag at :410 to `document-status`, and change `MarkFlagRequired("document-status")` to match.
  - Update the `AddCommand` at :413.
- `cmd/artifactfilter.go:14-79`: rename the filter field to `documentStatus`. Change the `parseListFilter` parameter, validate via `metadata.ParseDocumentStatus`, and use error code `invalid_document_status` with a `--document-status` message and a values list. Update the doc comments. An empty string stays "no filter" (`active()` at :29).
- `cmd/artifacts.go:58,73,104,168`: rename the flag to `--document-status`, update the help text, and change `entry["status"]` to `entry["document_status"]`.
- Tests:
  - `cmd/storefile_metadata_test.go`:
    - Rename the reset helpers to use `"document-status"` and `set-document-status` (:76-107).
    - Change all `--status`/`set-status` args (:224, 303, 339, 387, 434, 468, 514, 525, 563, 590) and comments.
    - Add a test that `--status` on write/list, and `set-status`, fail as unknown flag/command.
    - Add a test that `--document-status completed` / `in-progress` return `invalid_document_status` and leave the file bytes unchanged.
    - Add a test that `set-document-status` with no flag returns a required-flag error.
  - `cmd/storefile_list_filter_test.go`:
    - Change `listFlagNames` (:20) to `document-status`.
    - Change the `["status"]` assertions (:190, 197, 203, 250, 468) to `["document_status"]` with the new values, and change the args at :242, 377, 424.
    - Add a legacy (`status: completed`) fixture that lists with an empty `document_status`, is excluded under `draft` and `final` filters, and is included when no filter is given.
  - `cmd/artifacts_test.go`: change `artifactsListFlagNames` (:25-27), the `["status"]` keys (:251, 325, 357, 398, 408, 452) and the args (:245, 320, 364). Add the same legacy-artifact case for the cross-kind list.
  - `cmd/artifactfilter_test.go`: change the expected code at :83/:92 to `invalid_document_status`, and add retired-value rejection cases.
  - `cmd/changelog_file_test.go:39,390`, `cmd/file_test.go:36,66`, `cmd/plan_file_test.go:33,63,187`: new field and values.
- Leave alone: `cmd/version.go` status, the `status` subcommands in `cmd/{spec,plan,implement}.go`, and "in-progress workflow" resume wording.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Use 2 parallel agents after the production edits land: one for the `storefile_*` tests, one for the `artifacts` / `artifactfilter` / per-kind tests. Integrate them sequentially and finish with `go test ./...`.

### Phase 2.1: Agent wording and end-to-end suite use the new vocabulary

**File changes**:
- `templates/steps/plan/19-finished.md:26`: change "the documents are now marked completed" to "the documents are now marked final".
- `internal/steps/plan/steps_test.go:539`: change `require.Contains(t, out, "marked completed", …)` to `"marked final"`, and update its message.
- `tests/harbor/implement-workflow/tests/test_implement_workflow.py:113`: change `STATUS_COMPLETED = "completed"` to `STATUS_FINAL = "final"`.
- `tests/harbor/implement-workflow/tests/test_implement_workflow.py:287-300`: rename the test to `test_project_changelog_frontmatter_document_status_final`, assert `fm.get("document_status") == STATUS_FINAL`, and change the message text from `in-progress` to `draft`.
- `tests/harbor/implement-workflow/environment/spec.md:3` and `test-plan.md:3`: change `status: in-progress` to `document_status: draft`. Check `environment/plan.md` and `environment/Dockerfile:50-52` for any other frontmatter or comment, and update it if present.
- Do not edit the historical run output under `tests/harbor/jobs/`.
- Verify that `grep -rnE 'status: (in-progress|completed)|set-status|--status' templates` returns nothing. The remaining "completed" / "in-progress" hits in templates are ordinary English or workflow-resume wording, and are left alone.
- Do not hand-edit installed copies (`.claude/skills`, `.bob`); they are regenerated by init.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: A single agent works sequentially. Run `go test ./...`, then a manual `make harbor-test-implement` when Docker and credentials are available, and record the outcome in the test plan.

### Phase 2.2: Website describes document status

**File changes**:
- `docs:src/pages/projects.mdx:322`: change `status: in-progress` to `document_status: draft`, inside the existing fenced YAML block (319-327).
- `docs:src/pages/how-it-works.mdx:382-384`: change the `**Status**` list item to `**Document status**`: "where the document stands in its lifecycle (`draft`, `final`, `superseded` or `archived`)".
- `docs:src/pages/configuration.mdx:130-132,144-146,160-162`: change "a status" to "a document status" in the spec, plan and changelog lifecycle prose, and re-wrap the lines.
- Leave alone: `configuration.mdx:152` ("completed-feature"), `tutorials/unknown-criteria.mdx`, `.github/workflows/deploy.yml` (`cancel-in-progress`), `components/sections/Plugin.astro`.
- Follow the no-em-dashes rule, and MDX Rule 3 (blank lines around slot content; the existing structure is untouched).
- Verify in the docs repo root (`/home/nicj/code/github.com/jumppad-labs/spektacular-website`):
  - `grep -nE "<div|<section|class=" src/pages/*.mdx` returns 0 matches;
  - `npm run build` succeeds;
  - `npx astro check` reports 0 errors and 0 warnings;
  - `grep -rnE 'in[-_]progress|status: ' src/pages` shows no lifecycle hits.

**Complexity**: Low
**Token estimate**: ~5k tokens
**Agent strategy**: A single agent works sequentially in the docs repo root.

## Testing Strategy

- **Phase 1.1**
  - `internal/metadata` table tests cover lenient parsing: legacy key, retired values, bogus value, non-scalar value, missing key.
  - They also cover validator strictness, the draft default, preservation of a blank status and its `closed_date`, and closed-date stamping and clearing.
  - Close tests assert `final`.
  - Step tests assert that each finished step leaves its artifacts at `final`.
- **Phase 1.2**
  - `cmd` tests driven by `kindFixtures` cover all four values through `--document-status` and `set-document-status`.
  - They cover retired and bogus rejection with `invalid_document_status`, with the file bytes unchanged.
  - They check that the old `--status` and `set-status` are reported as unknown.
  - They check the `document_status` output key, and that no `status` key remains, in both list commands.
  - A legacy artifact must list with an empty value and be excluded by the `draft` and `final` filters.
  - Flag-reset helpers must use the new flag names, or state leaks between tests.
- **Phase 2.1**
  - The template contract assertion changes from "marked completed" to "marked final".
  - The harbor implement-workflow suite is run manually, outside CI, against the updated oracle and fixtures.
- **Phase 2.2**
  - In the docs repo: the MDX layout guard grep, `npm run build` and `npx astro check`.
- **Success metrics**
  - *No artifact read fails because of its document status*: **Behavioural test** (Phase 1.1 and 1.2 legacy and bogus cases).
  - *Planning agents no longer question a finished spec*: **Manual — captured in the implementation test plan**.

## Project References

- Spec: `000052_document-status-vocabulary` (GitHub issue #36).
- Knowledge: `spektacular:architecture/testing-architecture.md` says harbor oracles must change with the surface they mirror.
- Knowledge: `spektacular:conventions/error-messages-must-suggest-remediation.md`.
- Knowledge: `spektacular:conventions/tests-must-pass-for-done.md`.
- Knowledge: `docs:conventions/no-em-dashes.md`, `docs:conventions/mdx-authoring.md`, `docs:conventions/plan-content-pages.md`.
- Prior plan: `000038_artifact_metadata` (introduced the schema being renamed).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Phases 1.1 and 1.2 are Medium; 2.1 and 2.2 are Low. Phase 1.1 must run single-agent, because the rename is compile-coupled across packages.

## Migration Notes

None, by design. There are no aliases and no migration. Existing artifacts read with a blank `document_status` until someone sets one, and their legacy `status:` key is dropped on their next write. The user will rewrite this repo's existing artifacts separately.

## Performance Considerations

None. Parsing gains one normalisation lookup against a four-value set.

