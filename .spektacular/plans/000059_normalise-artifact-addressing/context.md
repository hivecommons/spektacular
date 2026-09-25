---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# Context: 000059_normalise-artifact-addressing

## Current State Analysis

- One builder, `newStoreFileCmd` (`cmd/storefile.go:181`), creates the write/read/delete/list/set-document-status verbs for spec (`cmd/file.go:8`), plan (`cmd/plan_file.go:14`) and changelog (`cmd/changelog_file.go:20`). Every verb joins `args[0]` verbatim onto the store dir, so callers must supply `.md` and, for plans, a joined `feature/doc.md`.
- Live behaviour on 2026-09-25 (dev build):
  - `spec file read <name>` gives `not_found`.
  - `plan file read <name>` gives `internal_error "read /abs/path: is a directory"`, which leaks a host path.
  - `plan file read <name> plan` gives `internal_error "accepts 1 arg(s)"`.
  - List names carry `.md`.
- Locations:
  - Central `spec`/`plan`/`changelog file list` paths are project-root-relative (`.spektacular/specs/x.md`), because store dirs are rewritten project-root-relative by `resolveStoreDirs` (`internal/config/config.go:440-460`) and the list's `TrimPrefix(…, st.Root())` (`cmd/storefile.go:320`) is a no-op.
  - `changelog file list --repo X` is rooted at the repo.yaml folder, so it is already config-relative (`changelog/<project>/x.md`).
  - `knowledge list` paths are relative to the source location.
- Workflow `new`/`goto` results report absolute `spec_path`/`plan_path`, and step templates render absolute `{{spec_path}}`/`{{plan_path}}`/`{{context_path}}`/`{{research_path}}`/`{{changelog_path}}`/`{{plan_dir}}` (about 30 uses in 7 templates). That tells agents to open store files directly, which fails for a store that is not on disk.
- Status:
  - `plan status` (no arg) and `implement status` report an absolute `plan_path` (`cmd/plan.go:264`, `cmd/implement.go:363-365`) and have no `plan_document`.
  - The named `plan status <name>` already takes the bare name and reports no path.
- Layout helpers encode `.md` in several places: `spec.SpecFilePath`, `plan.PlanFilePath/ContextFilePath/ResearchFilePath`, `implement` duplicates, `ChangelogFilePath`, and `implement/steps.go:226`'s test-plan join.
- No central error-code registry exists. Codes are literals, with domain const groups in `internal/knowledge/address.go` and `internal/design/errors.go`.
- Shipped instructions spell the old addressing in about 45 template files and one Go string (`internal/workflow/workflow.go:239`). The harbor suites hold hand-maintained copies. The installed `.claude`/`.bob` skill copies lag the templates.
- Docs: the site has no command reference, no error-code list and no migration page. The configuration reference's store keys describe config input only.

## Per-Task Technical Notes

### Task: Add the artifact address package

**Requirement attribution**: spektacular repo. Carries "Names never carry a file extension", "An extension is refused with the correct form" (grammar half) and "Reading a plan without a document is an actionable refusal" (grammar half).

**File changes**:
- `internal/artifact/address.go` (new):
  - `Kind` consts `KindSpec`, `KindPlan`, `KindChangelog`.
  - `Address{Kind, Feature, Document}`.
  - `Parse(kind, args []string) (Address, error)`.
    - Spec/changelog: exactly 1 arg.
    - Plan document verbs: 1 or 2 args.
    - When the one plan arg contains `/`, split it into feature/doc for the correction.
    - A segment containing `.` or any path separator gives `*ExtensionError{Input, Corrected}`. The corrected form strips from the first `.` in each segment (`x.md` → `x`, `x/plan.md` → feature `x` doc `plan`, `specs/x.md` → `x` taking the last element).
    - A plan with 1 arg and no `/` gives `*DocumentRequiredError{Feature}`.
    - An empty segment is refused as `ExtensionError`, or via a separate `bad_input` if clearer. Keep it simple.
  - `(Address) StorePath(dir) string`: `dir/feature.md` for spec/changelog, `dir/feature/document.md` for plan.
  - `FeatureDir(dir, feature)`.
  - `NameFromEntry(name string, isDir bool, want EntryKind)`, where `EntryKind` is `EntryFile` or `EntryFeatureDir`. Only `.md` files, or only directories, are addressable.
  - Constants `ErrCodeUnexpectedExtension = "unexpected_extension"` and `ErrCodeDocumentRequired = "document_required"`, following the const-group pattern at `internal/knowledge/address.go:58-74`.
  - Keep `.md` as one unexported const `docExt`.
- `internal/artifact/address_test.go` (new): table tests with hand-written literal expectations, per the memory rule on independent oracles.
  - Parse accepts, per kind.
  - Extension/joined refusals, with the exact `Corrected` value: `x.md`, `x.markdown`, `x/plan.md`, `x/plan`, `specs/x.md`.
  - Document-required.
  - StorePath layout literals, e.g. `".spektacular/plans/f/plan.md"`.
  - NameFromEntry round-trip, plus rejection of a directory in file mode and of `notes.txt`.
- Name-grammar evidence: feature names are `^[a-z0-9_-]+$` (`cmd/spec.go:22`); plan docs are plan|context|research|test-plan (`cmd/artifacts.go:43-46`).

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Address spec, plan and changelog document commands by bare name

**Requirement attribution**: spektacular repo. Carries the requirements:

- "One name addresses a feature's spec"
- "One name addresses a feature's changelog record"
- "A plan is addressed by feature and document"
- "Listed names are exactly the accepted names"
- "An extension is refused with the correct form"
- "Reading a plan without a document is an actionable refusal"

**File changes**:
- `cmd/storefile.go:181` `newStoreFileCmd`: replace the positional params with a descriptor:

  ```go
  type storeFileKind struct {
      kind      artifact.Kind
      short     string
      dir       storeDirFunc
      requireID bool
      repoRouted bool
      validate  writeValidator
  }
  ```

  - Plan verbs write, read, delete and set-document-status use `cobra.RangeArgs(1,2)`; spec and changelog keep `ExactArgs(1)`. Update the `Use` strings to `read <feature>` and `read <feature> <document>`.
  - Each verb calls `artifact.Parse(kind, args)` first and then renders the typed errors with a new helper, `addressRefusal(cmd, cfg, err)`, in a new file `cmd/storefile_address.go`. The helper builds the output envelope:
    - Code: `artifact.ErrCode*`.
    - Message: names the input and the rule, e.g. `"000057_x.md" carries a file extension; spec documents are addressed by the bare feature name`.
    - Resource: the input as typed.
    - Next action: `` `<cfg.Command> <cmd.CommandPath() minus root> <corrected args> <flags the caller set via cmd.Flags().Visit, rendered --name value>` ``.
  - For `DocumentRequiredError`, the next action is ``run `<cmd> plan file list <feature>` to see its documents, then e.g. `<cmd> plan file read <feature> plan` ``.
  - Replace `filepath.Join(storeDir, args[0])` with `addr.StorePath(storeDir)` in write `:220`, read `:263`, delete `:285` and set-document-status `:401`.
  - `validateIDPrefix` (`:70-84`): pass `addr.Feature`, drop the path-splitting, and fix the doc comment examples at `:66`. The NextAction at `:83` should name `spec file list`, which already prints bare IDs.
  - `provenanceOpts` (`:148`): take the feature, not a path.
  - `not_found` in read `:266` and set-document-status `:405`: add a next action ``run `<cmd> <kind> file list [<feature>]` to see available names``. For a plan, list that feature's documents. The message quotes the address, never a path.
  - list (`:297-356`):
    - `plan file list` with no arg lists `cfg.Plan dir` directories: `NameFromEntry(..., EntryFeatureDir)`.
    - `plan file list <feature>` parses `<feature>` as a feature (a `.`/`/` gives `unexpected_extension`) and lists `artifact.FeatureDir`, `.md` files only, stripped.
    - Spec/changelog list does not accept a positional argument any more: `MaximumNArgs(0)` for spec/changelog. Today `[path]` allows subpaths, which are meaningless with bare names.
    - A missing plan feature dir gives `not_found` with a next action of `plan file list`, instead of the raw `store.ErrNotFound` passthrough at `:314-317`.
    - Emit `name` = bare name. `path` stays as today in this task; the next milestone changes it.
    - Keep `modified_at` and the front-matter fields and filters.
  - set-document-status output (`:420-428`): `{"name": feature, "document": doc (plan only), "path": <store path as today>, ...}`.
- `cmd/file.go:8`, `cmd/plan_file.go:14`, `cmd/changelog_file.go:20`: construct the descriptors.
- `cmd/plan_file.go:27` `validatePlanDocument`: change the signature to take `artifact.Address` and gate on `addr.Document == "plan"`.
- `cmd/design_ref.go:112,122,148`, `cmd/spec.go:213`, `cmd/plan.go:123`, `cmd/implement.go:126`, `cmd/plan_export.go:85`: next actions only name list commands, which is fine. Verify that no string there tells the caller to append `.md`.
- Tests (rewrite to the new spelling, via `runRootCmd`/`resetRootCmd`, per `conventions/tests-must-not-depend-on-order.md`):
  - `cmd/file_test.go`, `cmd/plan_file_test.go`, `cmd/plan_file_validate_test.go`, `cmd/changelog_file_test.go` (including the `--repo` cases at `:179-477`).
  - `cmd/storefile_metadata_test.go:38` `kindFixtures()`: add feature/document fields instead of an `artifactName` with `.md`.
  - `cmd/storefile_list_filter_test.go` and `cmd/storefile_list_modified_test.go`: expected names lose `.md`.
  - The single invocations in `cmd/root_test.go`, `cmd/format_refusal_test.go`, `cmd/implement_task_run_test.go`, `cmd/plan_export_test.go`, `cmd/plan_task_id_test.go` and `cmd/plantask_fixture_test.go`.
- New test `cmd/storefile_address_test.go`:
  - Round-trip (list → read) for spec, plan (features and documents) and changelog, both central and `--repo`, using the `memberRepoProject` helpers from `cmd/changelog_file_test.go`.
  - Extension refusal for every verb × kind. Assert the exact `next_action` string, and assert the store is unchanged by snapshotting the files before and after.
  - `document_required` for all four plan document verbs. Assert the code, that there is no `internal_error`, that `strings.Contains(out, t.TempDir())` is false, and the next-action content.
  - Assert the `not_found` next action.

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis, sequential integration.
1. One agent reworks `cmd/storefile.go`, `cmd/storefile_address.go` and the descriptors, and writes `cmd/storefile_address_test.go`.
2. Then two parallel agents rewrite the existing test suites: one takes `file_test`, `plan_file*` and `storefile_*`; the other takes `changelog_file_test` and the single-invocation files.
3. Finish with an integrated `go test -shuffle=on ./cmd/...`.

### Task: Route workflow layout helpers and the walkthrough hint through the address

**Requirement attribution**: spektacular repo. Supports "Callers shipped with Spektacular use the new addressing" (Go-built instruction) and keeps the store layout single-sourced.

**File changes**:
- `internal/steps/spec/steps.go:15-18` `SpecFilePath`: return `artifact.Address{Kind: KindSpec, Feature: name}.StorePath(dir)`.
- `internal/steps/plan/steps.go:12-28`: `PlanFilePath`, `ContextFilePath` and `ResearchFilePath` delegate the same way, with Document `plan`/`context`/`research`.
- `internal/steps/implement/strategy.go:13-38`: delegate the duplicate plan helpers and `ChangelogFilePath`.
- `internal/steps/implement/steps.go:226`: replace `Join(PlanDir, name, "test-plan.md")` with an address that has Document `test-plan`.
- `internal/steps/*/strategy.go`: where `spec_path` is built as `instanceName+".md"`, delegate to `SpecFilePath`. The absolute `PathVars` are unchanged: they are template variables, and that is out of scope per the assumptions.
- `internal/workflow/workflow.go:239` `walkthroughRevisionHint`: `` `%s %s file write %v <doc> --from <scratch>` ``.
- Tests:
  - Update the assertion at `internal/workflow/workflow_test.go:172` to `"go run . plan file write 000045_config-file-migration <doc> --from <scratch>"`.
  - The existing step tests in `internal/steps/*/steps_test.go` that check scaffolded file paths must pass unchanged. That is the regression guard.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Move shipped skills and step instructions to the new addressing

**Requirement attribution**: spektacular repo. Carries "Callers shipped with Spektacular use the new addressing" and the acceptance criterion "Shipped instructions produce no refused command" (static half).

**File changes** (mechanical rewrite: `{{plan_name}}/<doc>.md` → `{{plan_name}} <doc>`, `{{plan_name}}.md` or `{{spec_name}}.md` → the bare var, `<name>/<doc>.md` → `<name> <doc>`, `<name>.md` → `<name>`):
- Skills:
  - `templates/skills/workflows/spek-new/SKILL.md:34-36`
  - `templates/skills/workflows/spek-plan/SKILL.md:32-34,83`. Also reword the "Path arguments are plan-directory-relative document paths (e.g. `my-feature/plan.md`)" paragraph to describe feature plus document.
  - `templates/skills/workflows/spek-implement/SKILL.md:30-32,42`
- Partial: `templates/partials/implement-plan-documents.md:3-5`
- Implement steps:
  - `templates/steps/implement/01-read_plan.md:71,92,95`
  - `02-analyze.md:7,22`
  - `03-implement.md:11-12`
  - `04-test.md:11-12`
  - `05-verify.md:11-12`
  - `06-update_plan.md:19,24`
  - `07-update_changelog.md:9,40,43,79`
  - `09-test_plan.md:10,36,40`
  - `10-update_feature_changelog.md:15,16,46,50,64,70,74`. Reword the prose at `:74`, "found under `{{plan_name}}.md`", to "named `{{plan_name}}`".
  - `11-reconcile_spec.md:10,11,33,37,41`
  - `12-finished.md:33,34`
- Plan steps:
  - `templates/steps/plan/02-discovery.md:48`
  - `15-write_plan.md:9`
  - `16-write_context.md:9`
  - `17-write_research.md:9`
  - `18-walkthrough.md:8-10,18,20`
  - `19-finished.md:7-9,15,24`
- Spec steps: `templates/steps/spec/08-verification.md:104,108,110` (reword `:108`, "the argument is the spec file name only", to "the bare spec name") and `templates/steps/spec/09-finished.md:4,9`.
- `templates/agents/historical-artifacts.md:20,36-37` and `templates/agents/store-access.md:12-13,21-22`: command names only. Check for, and fix, any spelling of an argument.
- Guard:
  - `internal/agent/instruction_surface_test.go:32-40`: add a second list, `forbiddenAddressingPatterns`. Use regexes rather than literals:
    - `` (spec|changelog) file (read|write|delete|set-document-status) [^ `\n]*\.md ``
    - `` plan file (read|write|delete|set-document-status) [^ `\n]*/ ``
    - `` \{\{(plan|spec)_name\}\}(\.md|/) ``
  - Extend the walk roots to `partials` and `agents`.
  - Assert against both the embedded templates and `installWorkflowSkills` output.
- Whole-corpus test: `cmd/instruction_contract_test.go:344` `agentFacingCorpus`. Add `TestNoAgentFacingInstructionUsesOldAddressing`, modelled on `TestContextMdAlwaysQualified` `:425`, applying the same regexes to the rendered `demo-feature` corpus.
- Update the pinned spellings:
  - `cmd/instruction_contract_test.go:397-403,415,514-516`. The `unqualifiedContextMd` qualifiers must now recognise `demo-feature context` / `<plan_name> context`; re-express the check so it still catches a bare `context.md` mention in prose.
  - `cmd/resume_test.go:168`.
  - `internal/steps/implement/steps_test.go:247-249,295,308,346,348,471,484,491-494,513,607-610`.
- Regenerate the installed copies with `go run . init claude` and `go run . init bob`, which rewrite `.claude/skills/*`, `.bob/skills/*` and the managed sections of `AGENTS.md`/`CLAUDE.md`. Never hand-edit them. Accept the unrelated pre-existing drift they pick up and mention it in the commit message.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 2-3 parallel agents.
- Agent A: `templates/steps/implement/*` and the partial.
- Agent B: `templates/steps/plan/*`, `templates/steps/spec/*`, skills and agents.
- Agent C: the guard and contract tests.

Then run `go test ./...` and regenerate the installed copies sequentially.

### Task: Update harbor end-to-end suites to the new addressing

**Requirement attribution**: spektacular repo. Supports the acceptance criterion "Shipped instructions produce no refused command" (e2e half). The oracles are hand-maintained couplings per `architecture/testing-architecture.md`.

**File changes**:
- `tests/harbor/implement-workflow/instruction.md:60-61,72,81,83`: e.g. `spec file read 20260101000000-jwt-auth`, `plan file read 20260101000000-jwt-auth plan`, `changelog file write 20260101000000-jwt-auth --from ...`.
- `tests/harbor/implement-workflow/tests/test_implement_workflow.py:97-102`: the oracle literals become `f"changelog file write {PLAN_NAME} --from"` and the `--repo` variant. Update the messages at `:244,255,265`.
- `tests/harbor/plan-workflow/instruction.md:20,45`.
- `tests/harbor/plan-workflow/solution/solve.sh:176,244,311`: `plan file write "$PLAN_NAME" plan|context|research --from ...`.
- `tests/harbor/spec-workflow/solution/solve.sh:93`: `spec file write "$SPEC_NAME" --from <staged file>`. This also replaces the retired stdin pipe with a staged `.spektacular/tmp/` file.
- Grep `tests/harbor` (excluding the gitignored `.build/`) for any remaining `file (read|write|delete|list)` with `.md` or `/`.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Manually test a full run with the new addressing

**What the person must do**:
- In a scratch project (or this repo, on a throwaway feature), use a dev build of the branch.
- Run a full spec → plan → implement workflow through the shipped skills (`/spek-new`, `/spek-plan`, `/spek-implement`), letting the agent follow the rendered instructions.
- Note any command the CLI refuses.
- This replaces the harbor runs. The harbor suites are updated but not run, by the user's decision (2026-09-25).

**How the result is checked**:
- The run reaches `finished` in all three workflows.
- The transcript contains no `unexpected_extension` or `document_required` refusal. On one, report the command and the instruction that produced it so the template can be fixed.

### Task: Name documents by address in workflow output and step instructions

**Requirement attribution**: spektacular repo. The user brought this into scope at the walkthrough (2026-09-25): documents Spektacular owns may not be on disk, so reading one must always go through the CLI. It extends "Status reports the plan by address as well as location" to the workflow commands' output, and "Callers shipped with Spektacular use the new addressing" to path references.

**File changes**:
- Step strategies: stop returning `spec_path`/`plan_path`/`context_path`/`research_path`/`changelog_path`/`plan_dir` as template variables. Keep the names (`spec_name`, `plan_name`, `changelog_section_name`).
  - `internal/steps/spec/strategy.go:15-20`
  - `internal/steps/plan/strategy.go:16-31`
  - `internal/steps/implement/strategy.go:49-67`
- Primary location: replace `PrimaryPathField()` with `PrimaryLocation(instanceName string) string`. It returns the config-relative location (`specs/<f>.md`, `plans/<f>/plan.md`) using the shared `reportedLocation` helper from the status task.
  - `internal/stepkit/stepkit.go:25-32,98,144`: update the `stepkit.PathStrategy` interface and the builder that fills the result's primary path from it.
  - `internal/steps/repo/strategy.go:16-18` keeps its own `repo_path` behaviour. Check it isn't a store document; it is a repo root, so it is out of this rule.
- Results:
  - `internal/steps/spec/result.go:6,20` `spec_path` is now config-relative.
  - `internal/steps/plan` and `internal/steps/implement` `Result` types gain `plan_document`.
  - `cmd/spec.go:385` (spec status) uses the relative location.
  - Update the schemas at `cmd/spec.go:61,71` and the plan and implement new/goto schemas.
- Templates: rewrite every path-variable use to name the document and its CLI read. For example, "`{{plan_path}}`" becomes "the plan (read it with `{{config.command}} plan file read {{plan_name}} plan`)".
  - `templates/steps/implement/01-read_plan.md:24,41,47,60,76,82,85,92,103`
  - `templates/steps/implement/12-finished.md:10,11,21-24,30,32,34`
  - `templates/steps/plan/01-overview.md:3`: "Read the specification with `{{config.command}} spec file read {{plan_name}}`".
  - `templates/steps/plan/18-walkthrough.md:13,18`
  - `templates/steps/plan/19-finished.md:18-22`: name `plan`, `context` and `research` under feature `{{plan_name}}`.
  - `templates/steps/spec/00-new.md:6`: "The spec scaffold has been created as `{{spec_name}}`".
- Guards:
  - `internal/agent/instruction_surface_test.go`: forbid `{{spec_path}}`, `{{plan_path}}`, `{{context_path}}`, `{{research_path}}`, `{{changelog_path}}` and `{{plan_dir}}` in any template.
  - `cmd/instruction_contract_test.go` `agentFacingCorpus`: assert that no rendered instruction contains the temp project root.
- Result tests: `cmd/implement_test.go:110,315` and `cmd/instruction_contract_test.go:134` get relative literals. Add spec new/goto/status and plan new/goto equivalents.

- Knowledge: `architecture/working-with-files-from-steps.md` and `architecture/workflow-steps.md` were updated at the walkthrough (2026-09-25) to state this rule, so they are the binding target this task brings the code up to. No further knowledge edit is needed.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents. One takes the strategies, results, schemas and result tests; the other takes the 7 templates and the guard tests. Integrate, then run `go test ./...`.

### Task: Report list locations relative to the declaring configuration

**Requirement attribution**: spektacular repo. Carries "Locations are reported relative to the declaring configuration" and "Changelog locations do not depend on how the record was selected".

**File changes**:
- `cmd/storefile.go:89` `storeFileStore`: also return base `config.ProjectConfigDirName` (".spektacular"). The store dirs are project-root-relative (`internal/config/config.go:440-460`).
- `cmd/storefile.go:105` `repoRoutedStore`: return base `""`. The store is rooted at the repo.yaml folder, so the store-relative path is already config-relative.
- `resolveStore` (`:186`): thread the base through.
- New helper `reportedLocation(base, storePath string) string`: `filepath.Rel(base, storePath)` when base is non-empty, then `filepath.ToSlash`. Use it for list `path` (replacing the no-op `TrimPrefix` at `:320`) and for the set-document-status `path`.
- Do not read `fileForm`, per `gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md`.
- Tests: in `cmd/storefile_address_test.go`, or a new `cmd/storefile_location_test.go`, assert these exact literal `path` values in a temp project with the default config:
  - `specs/<f>.md`
  - `plans/<f>`
  - `plans/<f>/plan.md`
  - `changelog/<f>.md`
  - `changelog/<project>/<f>.md` for `--repo`

  Also cover a non-default configured dir (e.g. `directory: docs/specs` gives `docs/specs/<f>.md`), and assert that no `path` has the `.spektacular/` prefix or is absolute. Add a set-document-status location assertion. Extend `cmd/storefile_list_filter_test.go:436` from key-exists to a value check.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Report the plan by address and relative location in status

**Requirement attribution**: spektacular repo. Carries "Status reports the plan by address as well as location" and the success metric "Hive reaches spec, plan and implementation record from the single recorded name".

**File changes**:
- `internal/steps/plan/result.go:18` and `internal/steps/implement/result.go:21`: add `PlanDocument string \`json:"plan_document"\`` after `PlanPath`, and document `PlanPath` as config-relative.
- `cmd/plan.go:264`: `planPath` = the `reportedLocation` equivalent of `plan.PlanFilePath(cfg.Plan.Config.Directory, planName)` relative to `.spektacular`. Set `PlanDocument: "plan"`.
- `cmd/implement.go:363-365`: keep reading through `planRel`, and report the relative location.
- Share the rel helper with the previous task. If that task has not landed, put `reportedLocation` in `cmd/storefile_location.go`, and have whichever task lands second reuse it.
- Update the schemas: `cmd/plan.go:28-39` `planStatusOutputSchema` and `cmd/implement.go:29-40`.
- The named `plan status <name>` path (`cmd/plan.go:245`, `runArtifactStatus`) is unchanged.
- Tests:
  - Update the absolute-path expectations at `cmd/implement_test.go:110,315` and `cmd/instruction_contract_test.go:134` to exact relative literals (`plans/fixture/plan.md`). Add `plan_document == "plan"` and a no-`t.TempDir()`-substring assertion.
  - Add a `plan status` no-arg equivalent in `cmd/plan_status_progress_test.go` or its sibling.
  - Keep `cmd/artifact_status_test.go:226` as-is: the named form still has no `plan_path`.
  - Success-metric test: after seeding a completed feature's spec, plan and changelog, read `data.name` from the state file, then run `spec file read <name>`, `plan file read <name> plan` and `changelog file read <name>`. Assert all three succeed.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Document addressing, locations and migration in the CLI repo

**Requirement attribution**: spektacular repo. Carries "The convention is documented" (CLI half) and "External callers get a migration note".

**File changes**:
- `README.md:28-38`: rewrite the status/list paragraph. Line 36 already states the bare-name rule for status; extend it to the file commands and add `plan_document`/`plan_path`. Add the "Addressing specs, plans and changelog records" subsection per the content example in plan.md.
- `README.md:~225`: extend the "Relative locations everywhere in `config.yaml` share one base" paragraph with: "Locations the CLI reports (list `path`, status `plan_path`) use the same base."
- `README.md:64`: `$EDITOR .spektacular/specs/<returned-spec-name>.md` is a filesystem path for a human editor, not an address. Leave it.
- `CHANGELOG.md`: this is the changelog record the implement workflow writes. Ensure the entry for this feature carries the `**Breaking change**:` paragraph per the plan.md content example, following the 000050/000047 shape. When it is written by the `update_changelog` step, verify it; otherwise add it.
- Knowledge entry `conventions/store-files-must-be-written-through-the-cli.md`: read it with `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"conventions/store-files-must-be-written-through-the-cli.md"}'`. It names commands only (`:27,44`). Change it only if an argument spelling appears, and only through `go run . knowledge write` with user confirmation.
- No em dashes in new prose.
- Verify by running every README/CHANGELOG command against this repo.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Publish the document command reference and migration note on the docs site

**Requirement attribution**: docs repo (`/home/nicj/code/github.com/jumppad-labs/spektacular-website`, branch `f-normalize-commands`). Carries "The convention is documented" (site half), "External callers get a migration note" (site half) and "The documentation site is updated".

**File changes**:
- `docs:src/pages/documents.mdx` (new): frontmatter `layout: ../layouts/Shell.astro`, `title`, `description`. Import `Hero`, `Section`, `Prose`, `ConfigurationKeys`, `ConfigKey` and `CtaBanner` as in `docs:src/pages/configuration.mdx:6-11`. Structure per the plan.md content outline, with `surface` alternating explicitly. Use fenced code blocks only, and `<Prose nested>` inside a `Section` as `docs:src/pages/plan-tasks.mdx` does. Error codes use the `ConfigKey` pattern from `plan-tasks.mdx:240-311`, and the JSON envelope example follows `docs:src/pages/design-documents.mdx:391-415`.
- `docs:src/components/Nav.astro:5-22`: add `{ label: "Documents", href: "/documents/" }` to the Resources `children`, after Design Documents.
- `docs:src/pages/configuration.mdx`:
  - Append the location sentence (plan.md content example) to the `spec` (`:173-190`), `plan` (`:192-207`) and `changelog` (`:209-224`) ConfigKey bodies.
  - The YAML comments at `:50,70,77` stay (they describe config input).
- `docs:src/pages/plan-tasks.mdx:313-341`: add `plan_name`, `plan_document` and `plan_path` (relative, `plans/<name>/plan.md`) to the status JSON sample and its field list. Also update `:245` if it describes the status output.
- `docs:src/content/tutorials/getting-started.mdx:282`: its comment "relative to .spektacular/, where config.yaml lives" is consistent; leave it.
- `docs:src/pages/extending.mdx:70,94`: "relative to the store root" is a different concept; leave it.
- Verify with `npm run build` and `npx astro check` (0 errors, 0 warnings), and with `grep -nE "<div|<section|class=" src/pages/*.mdx` (0 matches). Check for no `—` in changed files. Run each documented command against a scratch spektacular project to confirm output fields and codes.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential. Write the page, then nav, then configuration and plan-tasks edits, then build and verify. The work is all in one repo, and the files are small enough that parallel agents would add coordination cost.

## Testing Strategy

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

**Regression.** Existing step and workflow tests guard that the step layout helpers still produce the same store paths after delegation: plan scaffolding, changelog, test plan and implement plan reads. The walkthrough revision hint test pins the new two-argument write form.

**End-to-end (harbor, manual).** The spec, plan and implement harbor suites are updated (instructions, scripted solutions, verifier oracles) and each is run once. This is the proof that a full spec → plan → implement run driven by shipped instructions completes with no addressing refusal. The suites do not run in CI.

**Docs.** The docs site is verified by a clean site build and type check plus the MDX no-layout-HTML guard. Content accuracy (every command, field and code on the new page matches the CLI) is checked by running each documented command against a scratch project.

**Deliberate gaps.**

- No new tests for design or knowledge addressing, which are unchanged non-goals.
- No test that the `.claude`/`.bob` installed copies match the templates. They are regenerated by `init`, and the rendered-skill guard already renders from templates.

**Success metrics.**

- *Hive reaches spec, plan and implementation record from the single recorded name with no per-store string handling*: **Behavioural test**. A CLI test takes the `name` recorded in workflow state (`state.json` `data.name`) after a completed run and reads the spec, the plan's `plan` document and the changelog record using only that name (plus `plan`). All three succeed with no string manipulation.
- *No bug reports about a document command refusing a name a list printed*: **Behavioural test** for the guarantee (the list→read round-trip tests above, for every kind and both changelog routings). The post-release absence of reports is **Manual — captured in the implementation test plan**.
- *No reports of an absolute host path or `internal_error` from the spec, plan or changelog document commands*: **Behavioural test** for the guarantee (the refusal tests assert no absolute path and no internal error for extension, joined-path, missing-document and not-found cases). Post-release monitoring is **Manual — captured in the implementation test plan**.

Per task:

- **Address package**: table unit tests with literal oracles.
- **Command builder**: `cmd/storefile_address_test.go` (round-trip, refusals with exact next_action, no store change, no host path) plus the rewritten existing store-file suites.
- **Layout helpers**: the existing step tests must pass unchanged. Update the walkthrough hint assertion.
- **Instructions**: the widened `instruction_surface_test.go` deny patterns, plus a whole-corpus test in `cmd/instruction_contract_test.go`, plus the updated pinned spellings.
- **Harbor**: updated oracles; updated but not run; a manual end-to-end run instead.
- **Locations**: literal `path` assertions for every kind, with and without `--repo`, and for a non-default dir.
- **Status**: relative `plan_path` literal, `plan_document`, no temp-dir substring, and the recorded-name → three-reads success-metric test.
- **Docs**: `npm run build`, `npx astro check`, the Rule 1 grep, and command-output spot checks.

## Project References

- Spec: `000059_normalise-artifact-addressing` (read with `go run . spec file read 000059_normalise-artifact-addressing.md` until this ships; afterwards without `.md`).
- Designs: none referenced by the spec.
- Source issue: hivecommons/spektacular#46. Consumer: hivecommons/hive#8227.
- Knowledge (repo `spektacular`):
  - `architecture/working-with-files-from-steps.md`
  - `architecture/testing-architecture.md`
  - `gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md`
  - `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`
  - `conventions/error-messages-must-suggest-remediation.md`
  - `conventions/store-files-must-be-written-through-the-cli.md`
  - `conventions/tests-must-not-depend-on-order.md`
  - `conventions/tests-must-pass-for-done.md`
- Knowledge (repo `docs`):
  - `conventions/mdx-authoring.md`
  - `conventions/site-layout.md`
  - `conventions/plan-content-pages.md`
  - `conventions/no-em-dashes.md`
  - `conventions/file-scoped-section-headings.md`
  - `conventions/alternate-section-background.md`
- Repo roots:
  - spektacular: `/home/nicj/code/github.com/jumppad-labs/spektacular` (branch `f-normalize`)
  - docs: `/home/nicj/code/github.com/jumppad-labs/spektacular-website` (branch `f-normalize-commands`)

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Only the command-builder task is High, and it splits into one integration agent plus two test-rewrite agents. The template migration is Medium and is split by template directory. Pass each sub-agent the relevant repo root explicitly, and tell it that store documents are reached only through the CLI.

## Migration Notes

This is a hard break for external callers, with no deprecation window:

| Old | New |
|-----|-----|
| `spec file read\|write\|delete\|set-document-status <name>.md` | `spec file <verb> <name>` |
| `plan file read\|write\|delete\|set-document-status <name>/<doc>.md` | `plan file <verb> <name> <doc>` |
| `plan file list <name>` names `plan.md`, `context.md`, … | names `plan`, `context`, … |
| `changelog file read\|write\|delete\|set-document-status <name>.md [--repo r]` | `changelog file <verb> <name> [--repo r]` |
| list `path` `.spektacular/specs/x.md` | `specs/x.md` |
| status `plan_path` `/abs/.../.spektacular/plans/x/plan.md` | `plans/x/plan.md`, plus `plan_document: "plan"` |

Old spellings are refused with `unexpected_extension`, and the `next_action` gives the new command. No project settings migration is needed: store layout on disk is unchanged, and `migrate` does not change. Installed skills update through `spektacular migrate`/`init` as usual.

## Performance Considerations

Negligible. Parsing adds constant-time string checks per command. List output does the same number of store reads as today; directory and non-document entries are now skipped earlier.
