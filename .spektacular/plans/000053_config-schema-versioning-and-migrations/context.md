---
created_date: "2026-09-19"
document_status: final
closed_date: "2026-09-19"
---

# Context: 000053_config-schema-versioning-and-migrations

## Current State Analysis

- **Settings model:** `Config` (`internal/config/config.go:211-223`) and `RepoConfig` (`internal/config/repo.go:33-40`) carry no version fields. The loaders (`ParseYAMLFile` :274, `RepoConfigFromYAMLFile` repo.go:63) unmarshal over defaults. The writers (`ToYAMLFile` config.go:541, repo.go:316) are a plain marshal plus WriteFile.
- **Folder resolution is inconsistent:**
  - `repos[].location` (config.go:195) and project knowledge locations (`internal/knowledge/set.go:99-102`) resolve from the folder holding config.yaml.
  - `spec/plan/changelog.config.directory` default to `.spektacular/specs|plans|changelog` (config.go:42-49) and are used project-root-relative at ~20 call sites through `store.NewSourceStore(root,"project")` (`cmd/storefile.go:84-94`, `cmd/spec.go`, `cmd/plan.go`, `cmd/implement.go`, `cmd/artifacts.go`, `internal/stepkit/stepkit.go:83-87`, the step strategies).
  - The repo.yaml changelog already resolves from the repo.yaml folder (`cmd/storefile.go:126-131`).
- **Skills freshness:** `.spektacular/version` has one writer, `cmd/init.go:55-58`, and one reader, `version check` (`cmd/version.go:36-102`), which compares plain strings and reports `match|mismatch|missing|migration_needed`.
- **Legacy migration:** `detectMigrationNeeded` (cmd/version.go:151) is live, but `scanProjectMetadata` (:182) and `executeMigration` (:242) are dead code with test-only callers.
- **No command gate:** there are no `PersistentPreRun` hooks. `loadConfig()` (cmd/root.go:275) only returns `no_project`.
- **Footprint repair:** `repo.EnsureFootprint` (`internal/repo/footprint.go:40-45`) overwrites any repo.yaml that fails to load.
- **Uncommitted fix:** a change on `main` (`internal/project/init.go:93-100`) makes repo knowledge resolve from `.spektacular`.

## Per-Phase Technical Notes

### Requirement → repo attribution

| Requirement (spec) | Repo | Files |
|---|---|---|
| Format version / writing version / skills version recorded | spektacular | internal/config/config.go, internal/config/repo.go, cmd/root.go, cmd/init.go |
| Skills version replaces standalone file | spektacular | internal/migrate/steps_project.go, cmd/version.go, cmd/init.go |
| Unversioned = oldest | spektacular | internal/migrate/engine.go, internal/config/schema.go |
| Blocked until upgraded | spektacular | cmd/gate.go, cmd/root.go, cmd/migrate.go, cmd/init.go, cmd/version.go (annotations) |
| Installation check reports | spektacular | cmd/version.go, templates/skills/workflows/*/SKILL.md |
| Upgrade on demand / preview / multi-format / idempotent / backups / failure report / newer refused / absent repos / legacy single-file | spektacular | internal/migrate/*, cmd/migrate.go, internal/config/{config,repo}.go, internal/repo/footprint.go |
| Setup upgrades automatically | spektacular | cmd/init.go |
| Upgrading refreshes stale skills | spektacular | internal/migrate/engine.go, cmd/migrate.go (installer from internal/agent) |
| Store folders resolve from settings file / new defaults / existing keep entries | spektacular | internal/config/config.go, internal/migrate/steps_project.go, internal/project/init.go |
| Public docs | docs (+ spektacular README) | docs:src/pages/configuration.mdx, docs:src/content/tutorials/getting-started.mdx, docs:src/pages/how-it-works.mdx, docs:src/pages/projects.mdx, spektacular:README.md |

### Phase 1.1: Settings files record their format and writer versions

**File changes**
- `internal/config/schema.go` (new):
  - Add `CurrentProjectSchema = 2`, which Phase 3.1 bumps to 3, and `CurrentRepoSchema = 2`.
  - Add `var WriterVersion = "unknown"`.
  - Add `func NormaliseSchema(n int) int`, which maps 0 to 1.
  - Add `func PeekSchema(path string) (int, error)`. It reads the file, unmarshals only `struct{ Schema int \`yaml:"schema"\` }` and returns the normalised value. The engine and Phase 2.1 use it.
- `internal/config/config.go:211-223` (`Config`): add `Schema int \`yaml:"schema"\`` as the first field, then `WrittenBy string \`yaml:"written_by,omitempty"\`` and `SkillsVersion string \`yaml:"skills_version,omitempty"\``. Field order controls marshal order, so these keys come first in the file.
- `internal/config/config.go:541` (`ToYAMLFile`): on a copy, set `Schema = CurrentProjectSchema` and `WrittenBy = WriterVersion` before marshalling. Do not touch `SkillsVersion`.
- `internal/config/repo.go:33-40` (`RepoConfig`): add `Schema` and `WrittenBy` in the same way. At `internal/config/repo.go:316` (`ToYAMLFile`), stamp `CurrentRepoSchema` and `WriterVersion`.
- `cmd/root.go:329` (`init()`): set `config.WriterVersion = version`. The ldflags override `version` before `init()` runs, so the value is correct.
- `cmd/root.go:317-320`: fix the stale comment. Knowledge is not project-root-relative.

**Tests**
- `internal/config/config_test.go` and `repo_test.go` need:
  - a round-trip test: write then read the raw bytes, which must contain `schema: 2` and `written_by: test-x`, with `WriterVersion` set in the test and restored via `t.Cleanup`
  - a test that `SkillsVersion` is preserved through a load/save cycle
  - a test that an unversioned file loads unchanged, as today
- Oracles are literal strings.

**Complexity**: Low. **Token estimate**: ~15k. **Agent strategy**: single agent, sequential.

### Phase 1.2: An upgrade engine with the first registered steps

**File changes** (all new, package `internal/migrate`)
- `node.go`: helpers over `*yaml.Node`:
  - `docRoot(doc)`, `getScalar(root, "a.b.c") (string, bool)`, `setScalar(root, path, value)` (creates intermediate mappings), `deleteKey`, `ensureSeq`, `appendMapping`
  - `setSchema(root, n)`, which inserts `schema` as the first key if absent
  - `encode(doc) []byte`, via `yaml.NewEncoder` with `SetIndent(4)` to match `yaml.v3` Marshal defaults used by `ToYAMLFile`
- `registry.go`:
  - `type Kind`, `type Step`, `type StepContext`, `type Action` (see plan Data Structures)
  - `var projectSteps, repoSteps []Step`, plus `Registered(kind) []Step`
  - `func stepsFrom(kind, from int) ([]Step, error)`, which returns an error if a gap is found
- `steps_project.go`:
  - `project1to2` ("split legacy single-file settings and record installed skills version"):
    - (a) If `<FileDir>/repo.yaml` is absent, build a RepoConfig from `scanMetadata(ProjectRoot)`. Start from `config.NewDefaultRepoConfig()` with `Source = config.DefaultRepoSource`, per the knowledge gotcha. Emit `Action{Op:"create", Path: repo.yaml, Content: <marshalled with schema 2>}`.
    - (b) If `repos` is absent or empty, derive the name from the `name` key, or else slugify the base of `ProjectRoot` (reuse the helper `internal/project/init.go:47-52` uses). Set `name` when missing, append `{name, location: "."}`, and emit `set` actions.
    - (c) If `skills_version` is absent and `<FileDir>/version` exists, set `skills_version` to its trimmed content, then emit a `set` plus `Action{Op:"remove", Path: <FileDir>/version}`.
  - Phase 3.1 appends `project2to3`.
- `steps_repo.go`: `repo1to2` ("record format version"), which returns no actions. The engine stamps the version.
- `scan.go`: move `scanProjectMetadata` from `cmd/version.go:182-240` to `scanMetadata(projectRoot) config.RepoConfig`, with its body unchanged.
- `engine.go`:
  - `Inspect(projectRoot, binaryVersion)` returns `run(Options{DryRun:true, Skills:false})`, but still computes `Skills.Status`.
  - `Apply(opts)`:
    1. `upgradeFile(projectConfigPath, KindProject)`.
    2. Decode the (possibly in-memory) upgraded project doc into a minimal `struct{Repos []config.RepoEntry; Agent string; SkillsVersion string}`.
    3. For each repo, `loc := entry.ResolvedLocation(projectRoot)`. If `<loc>/repo.yaml` is missing, append `SkippedRepo{Reason:"not on disk"}`; otherwise `upgradeFile(<loc>/repo.yaml, KindRepo)`.
    4. Skills: `installed := SkillsVersion`, falling back to the legacy file only if the project doc still lacks it (dry-run). The status is match, mismatch or missing. If `opts.Skills` and the status is not `match`: when `Agent == ""`, return a `StepError` with Cause "no agent recorded", which cmd maps to a next_action `<command> init <agent>`. Otherwise, when not DryRun, call `opts.Install(agent)`, and only on success set `skills_version` via a node edit plus an atomic write. On failure, return a `StepError` with no stamp.
  - `upgradeFile`:
    1. Read the bytes and parse them into a node, then `from := NormaliseSchema(schema)`.
    2. If `from > current`, return a `*config.FormatError` with `Newer`.
    3. If `from == current`, return nil, meaning no report.
    4. For each pending step: `acts, err := step.Run(sc, doc)`.
    5. On error, return a `StepError{Path, Step: desc, ReachedSchema: n, Cause}`.
    6. If not DryRun:
       - write the backup once, before the first write: `<path>.v<from>.old`, byte-identical and via `os.WriteFile` of the original bytes; refuse to overwrite an existing backup with different content by appending `.1`, `.2`, …
       - perform the create/remove actions, where create skips an existing file and remove ignores ENOENT
       - `setSchema(doc, n+1)`, `setScalar(written_by, BinaryVersion)`
       - write atomically, to a temp file in the same dir followed by `os.Rename`
    7. Collect `FileReport`.
  - Status is `upgraded` if anything was applied, `upgrade_needed` in a dry run with pending work, and otherwise `up_to_date`.
- `errors.go`: `StepError` with an `Error()` string that names the path, step, reached schema and cause, plus `Unwrap`.
- `config.FormatError`: add it in `internal/config/schema.go` in this phase, since the engine needs it. Phase 2.1 wires it into the loaders.

**Tests** (`internal/migrate/*_test.go`, with literal fixture YAML in `testdata/`)
- `testdata/legacy_single/config.yaml`: no repos, no schema, a `version` file of `0.0.9`.
- `testdata/split_unversioned/{config.yaml,repo.yaml}`.
- Expected post-upgrade YAML is written by hand as literal files (`*.golden`), never generated by running the engine.
- Tests:
  - registry contiguity per kind, ending at the config constant
  - Inspect on unversioned versus current
  - dry-run leaves the snapshot identical (copy `snapshotDir` from `cmd/init_test.go:21` into a local test helper)
  - the dry-run report equals the apply report, excluding `Backup`
  - backup bytes equal the original bytes
  - an injected failing step (a test-only registry swap via an unexported `withSteps` helper) leaves `schema` at the reached value, and `StepError` names the step
  - an absent repo is skipped
  - a failing installer leaves `skills_version` unchanged
  - the legacy version file is carried and removed
  - a newer schema returns a `FormatError` and the file is unchanged
  - a synthetic extra step is picked up with no other code change (success metric 2)

**Complexity**: High. **Token estimate**: ~60k. **Agent strategy**: parallel analysis, then sequential integration. One agent writes `node.go` plus its tests while another writes `scan.go` and the step bodies; the engine and its tests are integrated sequentially.

### Phase 1.3: The migrate command, an honest installation check, and self-upgrading setup

**File changes**
- `cmd/migrate.go` (new):
  - `migrateCmd` with `Use: "migrate"`, a `--dry-run` bool flag, and `Annotations: {"gate": "exempt"}`, which Phase 2.2 reads.
  - RunE:
    1. `root := projectRoot()`. If there is no `.spektacular/config.yaml`, return the same `no_project` error `loadConfig` uses (`cmd/root.go:275`).
    2. Call `migrate.Apply(Options{ProjectRoot, BinaryVersion: version, DryRun, Skills: true, Install: installerFor(root)})`.
    3. `installerFor`: `a, err := agent.Lookup(name)`, then load config with `config.FromYAMLFile` (by now the file is current), then `a.Install(root, cfg, io.Discard or cmd.ErrOrStderr())`. Stdout must stay JSON only.
    4. On success, write the `Report` via `output.New(cmd.OutOrStdout(), fields).WriteResult`.
  - Error mapping:
    - `StepError` → `output.NewError("migrate_failed", err.Error()).WithNextAction("fix the cause above, then re-run '<command> migrate'; the previous settings are in <backup>")`
    - `FormatError.Newer` → `config_newer_format`, with next_action "install a Spektacular release that supports schema N (this build supports M)"
    - missing agent → `migrate_failed` with next_action `<command> init <agent>` (list `agent.Supported()`)
  - `<command>`: read the `command` key via a lenient raw peek (`migrate.PeekCommand(path)`, defaulting to `spektacular`), because the config may not load.
  - Register it in `cmd/root.go:336-345`.
- `cmd/version.go`:
  - Delete `detectMigrationNeeded` (:151), `migrationPrompt` (:106), `scanProjectMetadata` (:182, moved), `executeMigration` (:242) and `writeVersionFile` (:283). Keep `versionFilePath` only as a constant path helper for the engine's legacy read, or move it into migrate.
  - Rewrite `runVersionCheck` (:56-102) to call `migrate.Inspect(root, version)` and map the result:
    - a `FormatError` that is newer → `unsupported_format`
    - any pending file → `upgrade_needed`
    - otherwise `Skills.Status` (match, mismatch or missing), with `installed_version = Skills.Installed`
  - Map a `StepError` from Inspect as today's `migration_check_failed`.
  - Actions: `staleAction` (:133) now reads "…run `<command> migrate` (preview with `--dry-run`)" for mismatch, missing and upgrade_needed; unsupported_format reads "…update Spektacular to a newer release".
  - Update the schema enum (:46).
  - With no config.yaml, keep today's behaviour (status `missing`).
  - Add `Annotations: {"gate":"exempt"}` to `versionCheckCmd` (:30).
- `cmd/init.go:36-67`, new order:
  1. `agent.Lookup`.
  2. If `.spektacular/config.yaml` exists, `migrate.Apply(Options{ProjectRoot: cwd, BinaryVersion: version, Skills: false})`. Return errors through the same mapping, via a shared helper `migrateError(err)` in `cmd/migrate.go`.
  3. `project.Init`.
  4. `loadConfigLenient`, set `Agent`, write.
  5. `a.Install(...)`.
  6. On success, set `cfg.SkillsVersion = version`, write config.yaml again, and remove any legacy `.spektacular/version`.
  7. Print a summary line `Skills:   <version>` in place of the version-file line.
  - Add `Annotations: {"gate":"exempt"}` to `initCmd`.
- `internal/project/init.go`: no change in this phase. Its config write goes through `ToYAMLFile`, so it is already stamped.
- `templates/skills/workflows/{spek-new,spek-plan,spek-implement,spek-knowledge,spek-manage-repos}/SKILL.md:6-9`: replace the preamble. Use one shared wording, and fix the spek-manage-repos punctuation drift while doing so:
  ```
  > **Version check first.** Before running any other command, run `{{command}} version check`.
  > - On `status: "match"`, continue with the skill and produce no version-related output.
  > - On `"mismatch"`, `"missing"` or `"upgrade_needed"`, the project's settings or installed Spektacular files are out of date: relay the response's `action` message to the user, ask them to run `{{command}} migrate` (they can preview it with `--dry-run`), and wait for their decision before continuing.
  > - On `"unsupported_format"`, relay the `action` message: the project was written by a newer Spektacular, which the user must install before continuing.
  > - Never run `migrate`, `init` or modify installed files yourself — upgrading is always an explicit, user-initiated action.
  ```
  Check whether a shared partial exists in `templates/partials/`. If one does, put the preamble there instead of duplicating it.

**Tests**
- `cmd/migrate_test.go`:
  - Fixture projects are built by writing literal legacy YAML plus spec, plan and changelog files.
  - Scenarios:
    - dry-run: `snapshotDir` is identical, and the report lists the steps, the created repo.yaml, the removed version file and `skills.reinstall=true`
    - apply, then `version check` returns `match`
    - a second apply returns `up_to_date` with an identical snapshot
    - the backup path is reported and byte-identical
    - a `written_by` differing from `version` on an otherwise current project returns `up_to_date` and `match`
    - a stale `skills_version` returns the skills reinstalled for the configured agent (assert that the `.claude/skills/...` files exist), `skills_version` is `0.1.0`, and no other key changes (compare parsed maps minus `skills_version` and `written_by`)
    - an absent repo is named in `skipped_repos`
    - a read-only directory makes `migrate_failed` with the reached schema reported. Skip when running as root.
- `cmd/version_test.go`:
  - Delete the tests at :222, :292, :363, :429, :524 (dead helpers).
  - Rewrite the status tests for the new source: `skills_version`, with the legacy-file fallback.
  - Assert on the action text containing `migrate`.
- `cmd/init_test.go`:
  - :86 `TestInit_RewritesStaleVersionFile` becomes "init stamps skills_version and removes the legacy file".
  - :225 preamble assertion: update it to the new wording.
  - Add: `init` on an unversioned fixture leaves both files at the current schema and `version check` at `match` (success metric 3).
- `internal/agent/{claude,bob,codex}_test.go`: update the preamble assertions (`claude_test.go:39`).

**Complexity**: High. **Token estimate**: ~55k. **Agent strategy**: Medium-High. Two parallel agents: (a) cmd/migrate.go and cmd/version.go with their tests; (b) the templates and agent test preamble updates. Then one agent integrates cmd/init.go.

### Phase 2.1: Loading refuses settings in the wrong format

**File changes**
- `internal/config/config.go:274` (`ParseYAMLFile`): after `os.ReadFile`, run a mini-unmarshal of `schema`, then `n := NormaliseSchema(...)`. If `n != CurrentProjectSchema`, return `&FormatError{Path, Kind:"project", Found:n, Want:CurrentProjectSchema}`. Because `FromYAMLFile` builds on this, it inherits the check.
- `internal/config/repo.go:63` (`RepoConfigFromYAMLFile`): apply the same check with Kind `"repo"`.
- `cmd/root.go:252` (`toErrorResponse`): add `errors.As(err, &fe)`.
  - Outdated → `output.NewError("config_outdated", "<path> is settings format N; this Spektacular needs M").WithNextAction("run '<command> migrate' (preview with '--dry-run')")`.
  - Newer → `config_newer_format` with next_action "install a newer Spektacular release (this file needs schema N; this build supports M)".
  - Use `migrate.PeekCommand` for `<command>`.
- `internal/repo/footprint.go:40-45`: before treating a load error as broken, run `if _, ok := config.IsFormatError(err); ok { return "", err }`.
- `internal/repo/register.go:118`: propagate the error unchanged. This is already the case, but confirm it and cover it with a test.
- `cmd/root.go:95` (debug probe): on any error from `loadConfigLenient`, carry on with debug disabled. Confirm that it does not already swallow errors; if it returns early with a failure, change it.
- `cmd/init.go` / `internal/project/init.go:35-45`: `ParseYAMLFile` now refuses outdated files, but Phase 1.3 runs `migrate.Apply` first, so init still works. Add a test that proves init succeeds on an unversioned project.

**Tests**
- `internal/config/config_test.go` / `repo_test.go`:
  - an outdated file returns a `FormatError` with `Found=1`
  - a newer file (`schema: 99`) returns one with `Newer()` true
- `internal/repo/footprint_test.go`: a `schema: 99` repo.yaml stays byte-identical after `EnsureFootprint`, and the call returns an error.
- `cmd/error_response_test.go`: the next_action text contains `migrate` for outdated files, and "newer Spektacular" for newer ones.
- Existing config tests that write unversioned YAML and expect a successful load must gain `schema: <current>`. Update them in `internal/config/config_test.go` (28 sites) and `repo_test.go` using a local `withSchema(body)` helper.

**Complexity**: Medium. **Token estimate**: ~30k. **Agent strategy**: 2 parallel agents, one for the config and repo package changes and tests, one for the cmd error mapping and debug probe.

### Phase 2.2: Commands are blocked until the project is upgraded

**File changes**
- `cmd/root.go:31` (`rootCmd`): add `PersistentPreRunE: gate`.
- `cmd/gate.go` (new): `func gate(cmd *cobra.Command, _ []string) error`.
  - Exemptions: skip when any command in the `cmd`→root chain has `Annotations["gate"]=="exempt"`, when `cmd.Name()` is `help`, `completion` or `__complete`, or when `cmd == rootCmd`.
  - Skip when `configFilePath()` does not exist, so `no_project` behaves as today.
  - Otherwise call `migrate.Inspect(projectRoot(), version)`:
    - a `FormatError` that is newer → `config_newer_format`
    - any pending file → `upgrade_required` with a message listing each file and its from→to schema, and next_action "run '<command> migrate' (preview with '--dry-run'); the files are not changed until you do"
    - `Skills.Status != "match"` → `upgrade_required` with a message "installed skills are <installed|not recorded>; this Spektacular is <version>" and the same next_action
    - a `StepError` → `migrate_failed`
- Check for any subcommand defining its own `PersistentPreRun(E)`; research found none. If one is added later it would shadow the root hook, so note that in the gate's doc comment.
- Test fixtures to bring to the current format:
  - `cmd/spec_test.go:26` (`writeSpecCommandConfig`): prepend `schema: <config.CurrentProjectSchema>\nskills_version: <version>\n` (use `fmt.Sprintf` with the package var `version`). The repo.yaml written via `ToYAMLFile` is already stamped.
  - Every other cmd test that writes config.yaml or repo.yaml by hand: `cmd/knowledge_test.go` (11), `cmd/repo_test.go` (9), `cmd/root_test.go` (4), `cmd/repo_workflow_test.go` (4), `cmd/no_project_test.go` (4), `cmd/storefile_metadata_test.go`, `cmd/changelog_file_test.go`, `cmd/docs_test.go` (reads README only, so check it).
    - Route them through `writeSpecCommandConfig`, or a new `writeCurrentConfig(t, dir, body)` / `writeCurrentRepoConfig` in `cmd/root_test.go`.
    - Find stragglers with `grep -rn 'config.yaml\|repo.yaml' cmd/*_test.go`.
  - `internal/knowledge/set_test.go`, `internal/repo/{register,set}_test.go`, `internal/steps/repo/registration_test.go`, `internal/project/init_test.go` (24): add `schema` to any hand-written YAML that is later loaded.
- Harbor: leave the fixtures as they are (unversioned). Every task's instruction or solve.sh runs `spektacular init claude` first (`tests/harbor/*/instruction.md`, `solution/solve.sh`), which upgrades them. Verify with one `make harbor-test-implement` run in the final verification.

**Tests** (`cmd/gate_test.go`)
- An unversioned fixture plus `spec new` returns `upgrade_required`, next_action contains `migrate`, and no new files are created (snapshot).
- After `migrate`, `spec new` succeeds.
- Stale `skills_version` returns `upgrade_required`; missing `skills_version` with no legacy file returns `upgrade_required`.
- `migrate --dry-run`, `init claude`, `version check`, `help` and `completion bash` all exit 0 on an out-of-date fixture.
- An empty directory plus `spec new` still returns `no_project`.
- A `written_by` differing from `version` does not block.
- `schema: 99` returns `config_newer_format` for `spec new` and `repo list`, and the file is unchanged.
- All tests use `runRootCmd`.

**Complexity**: High, mostly fixture churn. **Token estimate**: ~50k. **Agent strategy**: parallel analysis, then sequential integration. The gate and its tests are written first, then fixture updates are split across 2-3 agents by package (cmd; internal/config+project; internal/repo+knowledge+steps), and a final `go test -shuffle=on ./...` runs sequentially.

### Phase 3.1: Store folders resolve from the settings file, with an upgrade step for existing projects

**File changes**
- `internal/config/config.go:42-49`: `DefaultSpecDir = "specs"`, `DefaultPlanDir = "plans"`, `DefaultChangelogDir = "changelog"`. Rewrite the comment to say "relative to the folder holding config.yaml".
- `internal/config/config.go:67-100` (`FileSpecConfig`, `FilePlanConfig`, `FileChangelogConfig`): add an unexported `fileForm string` to each (or to a shared embedded `dirField`), plus a doc comment that says:
  - in memory `Directory` is project-root-relative
  - on disk it is relative to the settings folder
- `internal/config/config.go:226` (`NewDefault`): return in-memory form, `Directory = filepath.Join(ProjectConfigDirName, DefaultSpecDir)` = `.spektacular/specs`, and so on. Add `const ProjectConfigDirName = ".spektacular"` if it does not exist; `ProjectConfigDir` (:187) uses it.
- `internal/config/config.go:274` (`ParseYAMLFile`):
  - Seed with a file-form default: a new `newFileDefault()` that sets the directories to `specs`, `plans` and `changelog`.
  - After unmarshal and `foldLocationAlias`, call `resolveStoreDirs(&cfg, filepath.Dir(path))`. For each dir:
    1. `fileForm = Directory`.
    2. `abs := Directory` if absolute, else `filepath.Join(configDir, Directory)`.
    3. `projectRoot := filepath.Dir(configDir)`.
    4. `rel, err := filepath.Rel(projectRoot, abs)`.
    5. If `rel` starts with `..` or is absolute, keep the value and let `Validate` refuse it; otherwise `Directory = rel` (slash-normalised).
- `internal/config/config.go:471-508` (`Validate` for spec, plan and changelog): add an `escapesProjectRoot` check. Return `config_invalid` with next_action "set `<section>.config.directory` to a folder inside the project, relative to the folder holding config.yaml (e.g. `specs`)".
- `internal/config/config.go:541` (`ToYAMLFile`): on the copy, for each dir, compute `want := resolveFromFileForm(fileForm, configDir)`. If `fileForm != ""` and `want == Directory`, write `fileForm`. Otherwise write `filepath.Rel(configDir, filepath.Join(projectRoot, Directory))`, or `Directory` if it is absolute. This covers `NewDefault()` (fileForm empty) → `specs`.
- `internal/migrate/steps_project.go`: add `project2to3` ("resolve spec, plan and changelog folders from the settings file"). For each of `spec`, `plan` and `changelog`:
  - Read `<k>.config.directory`, using the old default `.spektacular/<x>` when absent.
  - If relative, `new := filepath.Rel(FileDir, filepath.Join(ProjectRoot, old))`. If absolute, leave it.
  - If `new != old` or the key was absent, `setScalar` and emit `Action{Op:"set", Key, From: old, To: new}`.
  - An explicit key is always written, so a later default change cannot move it.
- `internal/config/schema.go`: `CurrentProjectSchema = 3`.
- `internal/project/init.go:106-107`: unchanged. It uses the resolved `cfg.*.Config.Directory` joined onto `projectPath`, which is still correct. Update the comment at :104-105.
- `cmd/root.go:317-320`: the comment now says "store directories are resolved to project-root-relative paths at load".
- Harbor `tests/harbor/implement-workflow/environment/config.yaml`: leave it as is, because init upgrades it. Confirm in the harbor run.

**Tests**
- `internal/config/config_test.go`:
  - load `directory: x` → `Directory == ".spektacular/x"`
  - absent → `.spektacular/specs`
  - absolute inside the root → a relative project-rooted value
  - `../../outside` → Validate error with a next_action naming the key
  - round trip: load, then `ToYAMLFile`, then the raw bytes still contain `directory: x`
  - `NewDefault().ToYAMLFile` writes `directory: specs`
- `internal/migrate`:
  - golden `split_unversioned` goes to schema 3 with `specs`, `plans` and `changelog`
  - a custom `docs/specs` becomes `../docs/specs`
  - absolute values are unchanged
  - a two-formats-behind fixture reaches 3 in one run with the same resolved locations, repos and agent (compare the typed config after load)
- `cmd/migrate_test.go` preservation (success metric 1): the fixture has 2 specs, 1 plan and 1 changelog entry under `.spektacular/{specs,plans,changelog/testproj}`. After `migrate`:
  - `spec file list`, `plan file list`, `changelog file list` and `artifacts list` return exactly those names (a literal expected list)
  - `spec new` writes under `.spektacular/specs`
- `cmd/repo_test.go`: `repo add` on a current project leaves the raw `directory: specs` unchanged.
- `cmd/spec_test.go`: config `spec.config.directory: x` → a new spec path under `.spektacular/x/`, and likewise for plan and changelog.
- `internal/project/init_test.go`: a fresh init gives on-disk `.spektacular/specs` and `.spektacular/plans`, and raw config.yaml contains `directory: specs`. Update the existing assertions that expect `.spektacular/specs` in the file.
- `templates/skill_list_command_test.go:21-23`: unchanged. Prose paths are still accurate on disk.

**Complexity**: High. **Token estimate**: ~50k. **Agent strategy**: parallel analysis, then sequential integration. One agent does the config resolution and write-back plus the config tests; another writes the `project2to3` step plus goldens. The cmd-level preservation and round-trip tests are integrated sequentially.

### Phase 3.2: Upgrade Spektacular's own projects and update the README

**File changes**
- Before: record the `spec file list`, `plan file list`, `changelog file list`, `changelog file list --repo docs` and `artifacts list` output for this project. Use a build of the plan's base commit, e.g. `git worktree add /tmp/spek-base <base-sha>` and then `go run /tmp/spek-base` from the project root, because the gate refuses these commands on the current build until migrate runs.
- Run `go run . migrate --dry-run`, review it, then `go run . migrate`. The run upgrades:
  - `.spektacular/config.yaml` (agent `bob` → skills reinstalled for bob)
  - `.spektacular/repo.yaml`
  - `docs:/home/nicj/code/github.com/jumppad-labs/spektacular-website/.spektacular/repo.yaml`
- Commit the upgraded settings. Commit or remove the `.v1.old` backups according to `.spektacular/.gitignore`; add `*.old` to `templates/.spektacular/.gitignore` if it is not already ignored.
- Afterwards, compare the listings and `changelog file list --repo docs` against the "before" output.
- `README.md:151-224`:
  - Add `schema`, `written_by` and `skills_version` to the config.yaml example, and `schema` and `written_by` to the repo.yaml example.
  - Change the directory comments to `specs   # relative to the folder holding config.yaml`.
  - Extend the "Relative locations everywhere…" paragraph to cover the store directories.
  - Add a short "Upgrading (`migrate`)" subsection.
  - Keep the literal strings `repo.yaml`, `repos:`, `.spektacular_ignore` and "Breaking change" that `cmd/docs_test.go` asserts.

- The docs site (`/home/nicj/code/github.com/jumppad-labs/spektacular-website/.spektacular`) has no `config.yaml`. It is only a repo footprint registered in this project, so its "project" check reduces to its repo.yaml upgrade and its repo-routed changelog entries.

**Complexity**: Low. **Token estimate**: ~15k. **Agent strategy**: single agent, sequential. This phase changes real user data, so it runs manually with the user's go-ahead.

### Phase 4.1: Configuration reference covers versions, folder rule and migrate

**File changes** (docs repo)
- `docs:src/pages/configuration.mdx:33-68` (example config.yaml):
  - Prepend `schema: 3`, `written_by: 0.16.0` and `skills_version: 0.16.0`, with comments.
  - Change the directories at :45, :50 and :55 to `specs`, `plans` and `changelog`.
- `docs:src/pages/configuration.mdx:76-78`: "Nine top-level sections…" becomes "Twelve top-level keys…", naming `schema`, `written_by` and `skills_version`.
- `docs:src/pages/configuration.mdx:~80` (before `command` at :82): add three `<ConfigKey>` entries for `schema` (type `integer`, defaultValue "written by Spektacular"), `written_by` (type `string`) and `skills_version` (type `string`), with slot bodies per the outline in plan.md.
- `docs:src/pages/configuration.mdx:119-164`: in the spec, plan and changelog ConfigKeys, change the `defaultValue` to `<code>specs</code>` etc., change the body "Defaults to `.spektacular/specs`" to "Defaults to `specs`, relative to the folder holding config.yaml (the same rule as `repos[].location`), so specs land in `.spektacular/specs`", and do the same for plans and changelog.
- `docs:src/pages/configuration.mdx:234-254` (example repo.yaml): add `schema: 2` and `written_by: 0.16.0`.
- `docs:src/pages/configuration.mdx:258-333`: add `schema` and `written_by` ConfigKeys to the repo keys.
- `docs:src/pages/configuration.mdx:~334` (before the CtaBanner at :335): add a new `<Section heading="Upgrading a project: migrate">`, with `surface` set opposite to the preceding `ConfigurationKeys` block and content per the plan outline. Commands go in fenced bash blocks, there are no em dashes, and no layout HTML is used.
- Values: use the literal `0.16.0` as an illustrative version. Check `Makefile:2` (`VERSION := 0.15.1`) at implementation time and use the next release number.

**Verification**: from the docs root, run `npm run build`, `npx astro check` and `grep -nE "<div|<section|class=" src/pages/*.mdx` (expect 0 matches), and grep the changed files for `—` (expect none in new text).

**Complexity**: Medium. **Token estimate**: ~20k. **Agent strategy**: single agent, sequential.

### Phase 4.2: Example configuration across the site uses the new defaults

**File changes** (docs repo)
- `docs:src/content/tutorials/getting-started.mdx:277-283`: `directory: .spektacular/specs` becomes `directory: specs`, with a comment. Keep the prose at :293-294 and :556, which names the on-disk folder and is still true.
- `docs:src/pages/how-it-works.mdx:309,341`: prose about the on-disk locations is still accurate, so there is no change. Verify only.
- `docs:src/pages/projects.mdx:125-134`: the tree is still accurate, so verify only. At :229-250 (repo.yaml example), add `schema: 2` if examples are meant to be complete; follow configuration.mdx's lead.
- `docs:src/pages/projects.mdx:177`: the sample transcript "Version matches…" is still a plausible `match` output, so there is no change.
- Search `grep -rn "directory: .spektacular" src/` and expect 0 hits after the change.

**Complexity**: Low. **Token estimate**: ~8k. **Agent strategy**: single agent, sequential.

## Testing Strategy

Per-phase test focus. The detail is in each phase's **Tests** block above.

- **1.1:** config package round-trip tests for the `schema` and `written_by` stamps and for `skills_version` preservation, with literal-string oracles.
- **1.2:** engine unit tests over checked-in literal fixtures and hand-written goldens. They cover:
  - registry contiguity
  - the same report from dry-run and apply
  - a byte-identical snapshot after dry-run
  - byte-identical backups
  - failure leaving the reached schema
  - skipped absent repos
  - an installer failure leaving `skills_version` alone
  - carry-over of the legacy version file
  - newer-format refusal
  - a synthetic extra step (success metric 2)
- **1.3:** cmd tests through `runRootCmd`:
  - migrate dry-run, apply, idempotence, backup, a differing `written_by`, skills reinstall and failure paths
  - rewritten `version check` statuses
  - init auto-upgrade (success metric 3) and the single-run settings plus skills upgrade (success metric 4)
  - preamble template assertions
- **2.1:** loader refusal tests for outdated and newer files. The `EnsureFootprint` test must not overwrite, and the error mapping must assert the `next_action` content.
- **2.2:** gate tests covering what is blocked, what is exempt, the no-project case, the newer-format case, and a differing `written_by` not blocking. Plus a fixture sweep across packages so the whole suite runs under the gate with `-shuffle=on`.
- **3.1:**
  - config resolution and write-back tests
  - the `project2to3` goldens
  - the preservation test (success metric 1, automated part)
  - custom-folder placement and the `repo add` round-trip
  - fresh-init layout
- **3.2:** manual comparison of real-project listings before and after the upgrade (success metric 1, manual part). **Manual — captured in the implementation test plan.**
- **4.x:** docs build and type-check, the MDX layout-HTML guard, and an em-dash check.
- **Final:** `go test ./...` fully green. Run one harbor suite (`make harbor-test-implement`) to confirm that seeded unversioned fixtures upgrade through init.

## Project References

- Registered repos (`go run . repo list`):
  - `spektacular` at `/home/nicj/code/github.com/jumppad-labs/spektacular` (tool)
  - `docs` at `/home/nicj/code/github.com/jumppad-labs/spektacular-website` (documentation). The docs site has a repo.yaml only, no config.yaml.
- Knowledge:
  - spektacular: `conventions/error-messages-must-suggest-remediation.md`, `conventions/tests-must-not-depend-on-order.md`, `conventions/tests-must-pass-for-done.md`, `gotchas/repoconfig-must-start-from-default.md`, `gotchas/ensure-footprint-discards-your-config.md`, `gotchas/remediation-needs-the-layer-that-holds-the-facts.md`, `architecture/testing-architecture.md`
  - docs: `conventions/plan-content-pages.md`, `conventions/file-scoped-section-headings.md`, `conventions/mdx-authoring.md`, `conventions/no-em-dashes.md`, `conventions/alternate-section-background.md`
- Spec: `000053_config-schema-versioning-and-migrations`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Phase budgets: 1.1 ~15k, 1.2 ~60k, 1.3 ~55k, 2.1 ~30k, 2.2 ~50k, 3.1 ~50k, 3.2 ~15k, 4.1 ~20k, 4.2 ~8k.

## Migration Notes

This feature *is* the migration mechanism, and the user-facing upgrade path is `migrate` or re-running `init`. For contributors:
- Commit or carry the uncommitted `internal/project/init.go` knowledge fix before starting.
- Capture this project's listings from a base-commit build before Milestone 2 lands (Phase 3.2).
- Add `*.old` to the scaffolded `.spektacular/.gitignore` if it is not already covered, so upgrade backups are not committed by accident.

## Performance Considerations

The gate runs `migrate.Inspect` on every non-exempt command. That reads config.yaml plus one repo.yaml per registered repo that is present, and parses each into a YAML node, which costs a few small file reads per command and is negligible next to the existing config load. It performs no network or git operations: repo locations are resolved from the registry and only stat'ed. If a project with many registered repos ever makes this noticeable, the gate could cache on file mtimes, but that is not planned.

