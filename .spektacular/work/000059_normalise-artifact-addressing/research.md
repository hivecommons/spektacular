## Alternatives considered and rejected

- **Accept both spellings during a deprecation window** (strip a trailing `.md`, split a joined `feature/doc.md`). Rejected: the spec makes the hard break a constraint (user decision), and accepting both would leave the "listed name == accepted name" rule unenforceable. `cmd/design_ref.go:136-150,195` already silently normalises `.md` for spec names; that tolerant pattern is exactly what is being removed from the file verbs, not copied.
- **Validate addresses at the store layer (`internal/store`)**. Rejected: the store is extension-agnostic byte storage (`internal/store/store.go:84-130`, `DirEntry{Name,IsDir,ModTime}`); `.md` is a file-provider/CLI concern today (`cmd/storefile.go:220,263,285,312,401` join `args[0]` verbatim). Pushing addressing into `store.Store` would widen an interface every backend must implement for a concern that belongs to artifact naming. Knowledge `gotchas/remediation-needs-the-layer-that-holds-the-facts.md` says the check belongs where the remediation facts live, which is the per-kind command builder (it knows the kind, the verb and the correct spelling), not the store.
- **Single-arg plan address `feature:document` or `--document` flag**. Rejected: spec's suggested shape is two positional args (`plan file read <feature> <document>`) and it is consistent with `plan file list <feature>` already taking the feature positionally (`cmd/storefile.go:297`, `MaximumNArgs(1)`). A flag would make `read`'s required argument optional-looking.
- **Report list `path` relative to the store directory (knowledge-style, e.g. `x.md`)**. Rejected: `knowledge list` paths are relative to the source location (`architecture/README.md`, live run 2026-09-25), but the spec acceptance criterion requires locations that "start with the store's configured directory" and contain neither `.spektacular/` nor an absolute path. Config-relative (`specs/x.md`, `plans/f/plan.md`, `changelog/x.md`) satisfies it; `changelog file list --repo` already emits exactly this shape (`changelog/spektacular/000039_...md`, live run).
- **Add a central error-code registry package for all CLI codes**. Rejected as scope creep: codes are string literals at call sites (`internal/output/writer.go:45-88`), with only domain const groups (`internal/knowledge/address.go:58-74`, `internal/design/errors.go`). Two new consts beside the address helper follow the domain-const-group pattern without refactoring every code.
- **Change `plan new`/`goto`/`implement new` results and step-template `{{plan_path}}`/`{{spec_path}}` to config-relative**. Rejected: spec limits the location requirement to list commands and the two status commands; step PathVars (`internal/steps/plan/strategy.go:18-31`, `internal/steps/implement/strategy.go:51-67`) are absolute paths the agent opens and are rendered into prose. Changing them is a separate concern.

## Chosen approach — evidence

- One builder serves all three stores: `newStoreFileCmd(short, dir, requireID, repoRouted, validate)` `cmd/storefile.go:181`, wired from `cmd/file.go:8` (spec), `cmd/plan_file.go:14` (plan), `cmd/changelog_file.go:20` (changelog). A per-kind address resolver passed into that builder changes all 15 verb/kind combinations in one place.
- Every verb does `filepath.Join(storeDir, args[0])` and relies on the caller's `.md` (`cmd/storefile.go:220,263,285,312,401`); `cobra.ExactArgs(1)` on write/read/delete/set-document-status (`:198,254,276,374`), `MaximumNArgs(1)` on list (`:297`). Plan verbs become `RangeArgs`/`ExactArgs(2)`.
- Path helpers already encode the file layout: `spec.SpecFilePath` `internal/steps/spec/steps.go:15-18` (`dir/name.md`), `plan.PlanFilePath/ContextFilePath/ResearchFilePath` `internal/steps/plan/steps.go:12-28`, `implement.ChangelogFilePath` `internal/steps/implement/strategy.go:13-38`. The resolver maps address → these helpers (plus a generic plan-document helper), so `.md` lives only in the file-layout helpers.
- Feature names are `^[a-z0-9_-]+$` (`cmd/spec.go:22`) and plan document names are `plan`, `context`, `research`, `test-plan` (`cmd/artifacts.go:43-46`, `internal/steps/implement/steps.go:226`), so any `.` in an address is unambiguously an extension and any `/` is a joined path.
- ID-prefix checks derive the slug with `TrimSuffix(Base(path), Ext)` (`cmd/storefile.go:70-84,148`); `validatePlanDocument` gates on `filepath.Base(docPath) == "plan.md"` (`cmd/plan_file.go:27`). Both keep working if fed the resolved store path, or can be switched to the address.
- Store roots: central stores use `store.NewSourceStore(projectRoot,"project")` with project-root-relative dirs (`cmd/storefile.go:89`, `internal/config/config.go:440-460` `resolveStoreDirs`); config-relative location = `filepath.Rel(config.ProjectConfigDirName, storePath)` (`internal/config/config.go:260` `ProjectConfigDir`). Repo-routed changelog store is rooted at the repo.yaml folder with dir `rc.Changelog.Config.Directory/<project>` (`cmd/storefile.go:105`), so its store-relative path is already relative to its declaring config (repo.yaml).
- List builds `name = e.Name` and `path = TrimPrefix(join(path,e.Name), st.Root()+sep)` (`cmd/storefile.go:318-356`); the TrimPrefix is a no-op because the path is already store-relative. Fix: strip `.md` for file entries' `name`, compute `path` via a per-store "location base" (config dir for central, `""` for repo-routed).
- Status: `runPlanStatus` `cmd/plan.go:218-273` (no-arg form builds absolute `planPath` at `:264`; named form goes via `runArtifactStatus` at `:245`), `runImplementStatus` `cmd/implement.go:327-365` (absolute at `:363-365`); result types `internal/steps/plan/result.go:18`, `internal/steps/implement/result.go:21`; JSON schemas `cmd/plan.go:28-39`, `cmd/implement.go:29-40`.
- Error envelope: `output.NewError(code,msg).WithResource().WithNextAction()` `internal/output/writer.go:60-88`; convention `conventions/error-messages-must-suggest-remediation.md` requires a runnable next_action and asserting its content.
- Go-built instruction string with old spelling: `internal/workflow/workflow.go:239` `walkthroughRevisionHint` (`%s %s file write %v/<doc>.md --from <scratch>`), asserted at `internal/workflow/workflow_test.go:172`.
- Instruction-surface guard to extend: `internal/agent/instruction_surface_test.go:32-40` (`forbiddenInstructionSubstrings`, walks `skills/workflows` and `steps` only — not `partials/` or `agents/`); whole-corpus harness `cmd/instruction_contract_test.go:344` `agentFacingCorpus`, model test `TestContextMdAlwaysQualified` `:425`.
- Installed skill copies are generated by `internal/agent/skills.go` `installWorkflowSkills` via `go run . init <agent>` (`cmd/init.go:74`) / `migrate` (`cmd/migrate.go:76`); never hand-edited.

## Files examined

### spektacular (CLI) — implementation
- `spektacular:cmd/storefile.go:66-84` — `validateIDPrefix`, `missing_id_prefix`; doc comment cites `.md` examples.
- `spektacular:cmd/storefile.go:89,105` — `storeFileStore` (project store, project-root-relative dir) vs `repoRoutedStore` (repo.yaml-folder root, `changelog/<project>`).
- `spektacular:cmd/storefile.go:148` — `provenanceOpts` slug from `Base(path)` minus ext.
- `spektacular:cmd/storefile.go:181-435` — `newStoreFileCmd`; verbs write/read/delete/list/set-document-status; `Use` strings `<path>`; delete and set-document-status never take `--repo`.
- `spektacular:cmd/storefile.go:314-356` — list: raw `ErrNotFound` passthrough on missing dir; `name=e.Name`; no-op TrimPrefix for `path`; filters exclude dirs and front-matter-less entries.
- `spektacular:cmd/file.go:8`, `cmd/plan_file.go:14,27`, `cmd/changelog_file.go:20` — per-kind wiring; `validatePlanDocument` gated on `plan.md`.
- `spektacular:cmd/plan.go:28-39,218-273,304-308` — plan status schema/impl; named form via `runArtifactStatus`.
- `spektacular:cmd/implement.go:29-40,126,144,255,277,327-365` — implement status; NextAction naming `plan file list`.
- `spektacular:cmd/spec.go:22` — `nameRegexp ^[a-z0-9_-]+$`; `:213` NextAction `spec file list`.
- `spektacular:cmd/design_ref.go:112-150,195` — spec-name normalisation (`.md` tolerant); out of scope (design non-goal) but its NextActions name `spec file list`.
- `spektacular:cmd/artifacts.go:43-46,163,197,255-285` — `artifacts list`: doc-name→kind map; scans central changelog under `<dir>/<cfg.Name>` though central writes are flat (pre-existing mismatch, not in this spec).
- `spektacular:cmd/plan_export.go:80,85`, `cmd/autocommit.go:309` — internal `PlanFilePath` users; NextAction naming `plan file list`.
- `spektacular:internal/store/store.go:14-190`, `internal/store/ignore.go:77` — store interfaces; no extension handling.
- `spektacular:internal/config/config.go:92-143,260,333-346,390-392,440-476` — fileForm, `ProjectConfigDir`, defaults `specs/plans/changelog`, `resolveStoreDirs`.
- `spektacular:internal/config/repo.go:61` — repo changelog dir default `changelog`.
- `spektacular:internal/steps/spec/steps.go:15-18`, `internal/steps/plan/steps.go:12-28,209-213`, `internal/steps/implement/strategy.go:13-67`, `internal/steps/implement/steps.go:226-262` — layout helpers and PathVars.
- `spektacular:internal/steps/plan/result.go:18`, `internal/steps/implement/result.go:21` — status result types.
- `spektacular:internal/stepkit/stepkit.go:92-144` — template var bundle; `PrimaryPathField` → `Result.PlanPath`.
- `spektacular:internal/workflow/workflow.go:239` — `walkthroughRevisionHint`.
- `spektacular:internal/output/writer.go:45-88` — error envelope builders.
- `spektacular:internal/knowledge/address.go:58-74` — model for a domain `ErrCode*` const group.
- `spektacular:internal/agent/skills.go` — `installWorkflowSkills` renders skills from templates.

### spektacular — templates (all need spelling changes)
- `templates/skills/workflows/spek-new/SKILL.md:34-36`, `spek-plan/SKILL.md:32-34,83`, `spek-implement/SKILL.md:30-32,42`.
- `templates/partials/implement-plan-documents.md:3-5` (included by `steps/implement/01-read_plan.md:12`, `steps/resume_implement.md:15`).
- `templates/steps/implement/01-read_plan.md:71,92,95`, `02-analyze.md:7,22`, `03-implement.md:11-12`, `04-test.md:11-12`, `05-verify.md:11-12`, `06-update_plan.md:19,24`, `07-update_changelog.md:9,40,43,79`, `09-test_plan.md:10,36,40`, `10-update_feature_changelog.md:15,16,46,50,64,70,74`, `11-reconcile_spec.md:10,11,33,37,41`, `12-finished.md:33,34`.
- `templates/steps/plan/02-discovery.md:48`, `15-write_plan.md:9`, `16-write_context.md:9`, `17-write_research.md:9`, `18-walkthrough.md:8-10,18,20`, `19-finished.md:7-9,15,24`.
- `templates/steps/spec/08-verification.md:104,108,110`, `09-finished.md:4,9`.
- `templates/agents/historical-artifacts.md:20,36-37`, `store-access.md:12-13,21-22` — command names only (no change needed unless wording mentions spelling).

### spektacular — tests
- `cmd/root_test.go:35,70` — `resetRootCmd`, `runRootCmd`.
- `cmd/file_test.go`, `cmd/plan_file_test.go`, `cmd/plan_file_validate_test.go`, `cmd/changelog_file_test.go` (`--repo` at `:179-477`), `cmd/storefile_metadata_test.go:38` (`kindFixtures` artifactName with `.md`), `cmd/storefile_list_filter_test.go:436` (only asserts `path` key exists), `cmd/storefile_list_modified_test.go`, `cmd/plan_status_progress_test.go` — ~77 CLI invocations with `.md`/joined args.
- `cmd/implement_test.go:110,315`, `cmd/instruction_contract_test.go:134,397-425,514-516`, `cmd/artifact_status_test.go:226`, `cmd/resume_test.go:168` — plan_path and spelling assertions.
- `internal/steps/implement/steps_test.go:247-610` (15 spelling assertions), `internal/workflow/workflow_test.go:172`.
- `internal/agent/instruction_surface_test.go:32-40` — deny-list guard.
- `cmd/error_response_test.go` — envelope shape.

### spektacular — harbor suites (not in CI)
- `tests/harbor/implement-workflow/instruction.md:60-61,72,81,83`; `tests/implement-workflow/tests/test_implement_workflow.py:97-102,244,255,265`.
- `tests/harbor/plan-workflow/instruction.md:20,45`; `solution/solve.sh:176,244,311`.
- `tests/harbor/spec-workflow/solution/solve.sh:93` (`spec file write "$SPEC_NAME.md"`, also uses stdin — already stale).

### spektacular — docs
- `README.md:28-38` (status/list addressing), `:64`, `:166-253` (configuration, "Relative locations" paragraph ~225, `> **Breaking change**:` pattern ~173, Upgrading ~253).
- `CHANGELOG.md` — `## <id>_<slug>` + `**Breaking change**:` paragraph pattern (e.g. 000050, 000047).
- `docs/knowledge-base.md:408-425` — command-reference table pattern (knowledge only).

### docs (spektacular-website, branch `f-normalize-commands`)
- `docs:src/pages/configuration.mdx:50,70,77,173-224` — spec/plan/changelog `ConfigKey` bodies (home for config-relative rule); `:436-478` migrate section.
- `docs:src/pages/plan-tasks.mdx:240-341,366,375-383` — status JSON/ConfigKey field pattern; inline error-code bullet list.
- `docs:src/pages/design-documents.mdx:391-415` — full JSON error envelope examples.
- `docs:src/pages/how-it-works.mdx:143,392,425,455-475`; `docs:src/content/tutorials/getting-started.mdx:282,294,556`; `docs:src/pages/extending.mdx:70,94` (store-root "relative" — different sense).
- `docs:src/components/Nav.astro:5-22` — Resources dropdown `children`; pages file-routed `src/pages/<slug>.mdx`.
- No command reference, error-code list or migration page exists on the site.

## External references

- GitHub issue hivecommons/spektacular#46 — source of the rules, suggested command shapes, `unexpected_extension` / `document_required` codes and worked output examples.
- hivecommons/hive#8227 — Hive's need to key a run on `state.json` `data.name`; the consumer the success metrics are about.

## Prior plans / specs consulted

- Spec 000059 (this spec) — requirements, hard-break constraint, non-goals (design, knowledge, workflow input unchanged).
- Knowledge `architecture/working-with-files-from-steps.md` — steps/commands reach artifacts through `store.Store`; `Root()` only for rendering; path helpers are the file layout.
- Knowledge `gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md` — spec/plan/changelog dirs are rewritten to project-root-relative on load and must stay in the project; config-relative location must be derived, not read from `fileForm`.
- Knowledge `gotchas/remediation-needs-the-layer-that-holds-the-facts.md` — put the refusal where the correct spelling can be built; assert next_action content.
- Knowledge `architecture/testing-architecture.md` — harbor oracles are hand-maintained couplings to CLI command spellings; must change in the same change and be run.
- Knowledge `conventions/error-messages-must-suggest-remediation.md`, `conventions/tests-must-not-depend-on-order.md`, `conventions/tests-must-pass-for-done.md`, `conventions/store-files-must-be-written-through-the-cli.md`.
- Docs knowledge `conventions/mdx-authoring.md`, `site-layout.md`, `plan-content-pages.md`, `no-em-dashes.md`, `file-scoped-section-headings.md`, `alternate-section-background.md`.

## Open assumptions

- Plan documents are always persisted as `<plan-dir>/<feature>/<document>.md`; any `.md` file in a plan directory is a document whose name is the filename minus `.md`. Non-`.md` files in a plan directory (none known) are omitted from `plan file list <feature>`.
- `plan file list <feature>` on a feature with no plan returns the existing not-found behaviour (made a documented `not_found` with next_action) rather than an empty list.
- Central changelog records are flat (`<changelog-dir>/<feature>.md`), matching `implement.ChangelogFilePath` and live `changelog file list` output; `artifacts.go:266`'s `<dir>/<project>` scan is a pre-existing mismatch left alone.
- Configured store directories always resolve inside the project root (`storeDirs()` enforces it), so `Rel(.spektacular, storePath)` never needs to express a path outside the project; it may start with `../` when a store is configured as a sibling of `.spektacular/`, which still "starts with the configured directory" as written in config.yaml.
- Regenerating `.claude`/`.bob` skill copies with `go run . init` will also pull unrelated template drift (both copies lag HEAD); that drift is accepted as part of the regeneration.
- The docs repo is checked out on `f-normalize-commands` and is where site changes land.

## Rehydration cues

- `go run . repo list` — roots: spektacular `/home/nicj/code/github.com/jumppad-labs/spektacular`, docs `/home/nicj/code/github.com/jumppad-labs/spektacular-website`.
- `go run . spec file read 000059_normalise-artifact-addressing.md` (old spelling until this ships; new: `spec file read 000059_normalise-artifact-addressing`).
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs`.
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/working-with-files-from-steps.md"}'` and `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`, `architecture/testing-architecture.md`.
- Re-read `cmd/storefile.go:181-435`, `cmd/plan.go:218-273`, `cmd/implement.go:327-365`, `internal/workflow/workflow.go:239`, `internal/agent/instruction_surface_test.go`, `cmd/instruction_contract_test.go:344-520`.
- `grep -rnE "(spec|changelog) file \w+ [^ ]*\.md|plan file \w+ [^ ]*/" templates/ tests/harbor README.md` — residual old spellings.
- Live checks: `go run . spec file list`, `go run . plan file list <feature>`, `go run . changelog file list [--repo spektacular]`, `go run . implement status`.
