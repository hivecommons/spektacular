---
created_date: "2026-09-19"
document_status: final
closed_date: "2026-09-19"
---

# Research: 000053_config-schema-versioning-and-migrations

## Alternatives considered and rejected

- **Key upgrades on the binary version (`written_by`) instead of an integer schema.** Rejected: `version` defaults to `0.1.0` (`spektacular:cmd/root.go:24`) and is only overridden by ldflags (`Makefile:10`, `dagger/main.go:152`), so every `go run .` build reports 0.1.0 while formats change between tags; string/semver ranges are fragile. Spec constraint also forbids it.
- **Keep `.spektacular/version` as the skills-freshness record.** Rejected by spec (single place = project settings). The file only exists in the project folder and is written by `cmd/init.go:55-58` only; its sole reader is `runVersionCheck` (`cmd/version.go:36-102`).
- **Do upgrades inside config loading (`ParseYAMLFile` / `RepoConfigFromYAMLFile`) silently.** Rejected by spec constraint ("loading may only read the current format and detect that a file is out of date"). Precedent for detect-and-refuse: `rejectLegacyRepoAddress` (`internal/config/config.go:331`), `rejectLegacyKnowledgeScope` (:309), `rejectLegacyRepoKnowledgeBlock` (`internal/config/repo.go:91`), `knowledge.unreachableStore` (`internal/knowledge/set.go:645-668`).
- **Gate only inside `loadConfig()` (`cmd/root.go:275`, 20 production call sites).** Rejected as the *sole* gate: commands that never load config (e.g. `skill`, `cmd/skill.go:27`) would slip through, and stale skills are not a config-load concern. Chosen: a root `PersistentPreRunE` gate with an exemption list, backed by the loader's own outdated/newer refusal as defence in depth.
- **Re-root the project spec/plan/changelog stores at `ProjectConfigDir(root)` (mirroring `repoRoutedStore`, `cmd/storefile.go:100-132`).** Rejected: touches ~20 call sites (`cmd/spec.go:229,243,313,366`, `cmd/plan.go:131,200,261`, `cmd/implement.go:138,143,212,273`, `cmd/repo.go:248,313`, `cmd/artifacts.go:188,217,266`, `cmd/file.go:10`, `cmd/plan_file.go:11`, `cmd/changelog_file.go:22`, `internal/stepkit/stepkit.go:83-87`, step strategies), and the spec's technical approach says resolve once at load so call sites need not change.
- **Unmarshal settings into `map[string]any` for migration steps.** Rejected in favour of `yaml.Node` (`gopkg.in/yaml.v3`, already the dependency in `internal/config/config.go:11`): a node tree preserves key order and comments in user-edited files, which a map round-trip destroys.
- **Revive the dead `executeMigration` as-is** (`cmd/version.go:242`). Rejected: it has no production caller, no rollback, always backs up to a fixed `config.yaml.old`. Its `scanProjectMetadata` (`cmd/version.go:182`) is reused as the legacy-split step's metadata source.
- **Treat unversioned repo.yaml as already current (repo schema 1 = current).** Rejected: the file would never carry a format version, contradicting "every settings file records its format version" and the AC that an upgrade leaves repo files at the same version as a fresh setup. A stamp-only repo step (1→2) is used instead.

## Chosen approach — evidence

- Loader seeds defaults then unmarshals (`internal/config/config.go:274`, `internal/config/repo.go:63`) → a missing `schema` field reads as zero, which maps cleanly to "oldest format (1)".
- All settings writes funnel through two writers: `Config.ToYAMLFile` (`internal/config/config.go:541`) and `RepoConfig.ToYAMLFile` (`internal/config/repo.go:316`) → stamping `schema`/`written_by` there covers every writer (`internal/project/init.go:133-145`, `cmd/init.go:49-53`, `internal/repo/register.go:81-86,140-144`, `internal/repo/footprint.go:36-45`).
- Relative-to-settings-file precedent: `RepoEntry.ResolvedLocation` (`internal/config/config.go:195`), project knowledge (`internal/knowledge/set.go:99-102`), repo changelog (`cmd/storefile.go:126-131`) → store folders follow the same rule.
- Every store consumer treats `*.Config.Directory` as project-root-relative inside `store.NewSourceStore(root,"project")`, and `FileStore` rejects paths escaping the root (`internal/store/store.go:100,129-130`) → resolving at load into a project-root-relative path keeps call sites unchanged; a value escaping the project root is refused at load with a remediation.
- Writers re-save a *loaded* Config (`cmd/init.go:44-51`, `internal/repo/register.go:83`, `internal/steps/repo/registration.go:55,79`) → `ToYAMLFile` must convert resolved directories back to settings-relative form, or the next load would double-nest (`.spektacular/.spektacular/specs`).
- Skills install entry point: `agent.Lookup(name).Install(projectPath, cfg, out)` (`internal/agent/agent.go:20-23,39`; used at `cmd/init.go:26,67`) → migrate reuses it for `cfg.Agent`.
- Single error envelope: `runRoot()` (`cmd/root.go:92-113`) → a `PersistentPreRunE` error surfaces as a normal JSON failure with exit 1. The debug probe (`cmd/root.go:95`) uses `loadConfigLenient` and must tolerate out-of-date files.
- `EnsureFootprint` overwrites a repo.yaml that fails to load (`internal/repo/footprint.go:40-45`) → must distinguish "outdated/newer format" from "broken" so a newer-format file is never clobbered.
- Test harness: `resetRootCmd`/`runRootCmd` (`cmd/root_test.go:33,68`), `writeSpecCommandConfig` (`cmd/spec_test.go:26`), `snapshotDir` (`cmd/init_test.go:21`) → reusable for gate, dry-run byte-identity and idempotency tests. Tests rely on `version = "0.1.0"` as a hand-maintained oracle (`cmd/version_test.go:18-21`).

## Files examined

- spektacular:internal/config/config.go:42-56 — Default{Spec,Plan,Changelog}Dir are `.spektacular/...`, "relative to the project root"; repo defaults `knowledge`/`changelog` are repo.yaml-relative.
- spektacular:internal/config/config.go:187-199 — `ProjectConfigDir(root)` = `<root>/.spektacular`; `ResolvedLocation` joins relative repo locations onto it.
- spektacular:internal/config/config.go:211-259 — `Config` struct (no version fields); `NewDefault`; `FromYAMLFile` = Parse+Validate.
- spektacular:internal/config/config.go:274-331,461 — `ParseYAMLFile` pipeline: env expansion, defaults, legacy rejectors, `foldLocationAlias`.
- spektacular:internal/config/config.go:391-541 — `Validate` (directory non-empty); `ToYAMLFile` plain marshal + WriteFile 0644.
- spektacular:internal/config/repo.go:17-140,316 — `RepoConfig`, `NewDefaultRepoConfig`, `RepoConfigFromYAMLFile` always validates, `ToYAMLFile`.
- spektacular:internal/knowledge/set.go:99-102,645-668 — project knowledge resolved from config dir; `unreachableStore` precedent.
- spektacular:cmd/root.go:23-34,70 — `version`/`sha` vars, `versionString()`.
- spektacular:cmd/root.go:92-113 — `runRoot` error envelope; debug probe via `loadConfigLenient`.
- spektacular:cmd/root.go:260-345 — `configFilePath`, `loadConfig` (no_project gate), `loadConfigLenient`, `dataDir`, `projectRoot`, stale comment at :317-320, command registration :336-345; no PersistentPreRun hooks.
- spektacular:cmd/version.go:17-54 — `VersionCheckResult`, schema enum (`match|mismatch|missing|migration_needed`).
- spektacular:cmd/version.go:56-151 — check flow, `classifyVersion`, `staleAction` (points at `init <agent>`), `detectMigrationNeeded` (config.yaml without repo.yaml).
- spektacular:cmd/version.go:182-283 — dead `scanProjectMetadata`/`executeMigration`; `versionFilePath`, `writeVersionFile`.
- spektacular:cmd/init.go:14-67 — init order: Lookup → project.Init → reload → set Agent + write → write version file → print → `a.Install`.
- spektacular:internal/project/init.go:24-192 — scaffold; mkdir spec/plan via `filepath.Join(projectPath, dir)` (:106-107); no changelog mkdir; writes config only when new/renamed/seeded; repo cascade with `EnsureFootprint`. Uncommitted fix at :93-100 (repo knowledge resolves from `.spektacular`).
- spektacular:internal/agent/{agent.go,claude.go,bob.go,codex.go,skills.go,managed_section.go} — `Agent.Install`; skills always overwritten; managed sections idempotent.
- spektacular:internal/repo/footprint.go:28-61 — create/repair/unchanged; repair overwrites unparseable repo.yaml.
- spektacular:internal/repo/register.go:42-144 — `Register` rewrites config.yaml and repo.yaml when changed.
- spektacular:internal/steps/repo/registration.go:35-79 — guided flow loads via `FromYAMLFile` and calls `Register`.
- spektacular:cmd/storefile.go:84-132,169 — `storeFileStore` project store; `repoRoutedStore` repo.yaml-relative changelog.
- spektacular:cmd/artifacts.go:85,188,217,266 — listings rooted at project store.
- spektacular:internal/store/store.go:100,129-130 — escape-the-root rejection.
- spektacular:templates/skills/workflows/*/SKILL.md:6-9 — version-check preamble handles only match/mismatch/missing, points at `init <agent>`.
- spektacular:cmd/init_test.go:21,86,225; cmd/version_test.go:18-35,222,292,363,429,524; cmd/root_test.go:33,68,98; cmd/spec_test.go:26; cmd/no_project_test.go:24-40; internal/config/config_test.go:525,690; internal/config/repo_test.go:484; internal/agent/claude_test.go:39 — test helpers and preamble assertions.
- spektacular:tests/harbor/implement-workflow/environment/{Dockerfile,config.yaml,docs-repo.yaml,repo.yaml} — seeded unversioned configs with `.spektacular/specs` dirs; Dockerfile runs init after seeding.
- spektacular:.spektacular/config.yaml — this project's own config uses `.spektacular/specs|plans|changelog`, agent `bob`; `.spektacular/version` present.
- docs:src/pages/configuration.mdx:18-333 — config/repo reference; spec/plan/changelog keys :119-164 default `.spektacular/...`; sub-text "Nine top-level sections" :76-78; example :33-68; repo example :234-254.
- docs:src/content/tutorials/getting-started.mdx:277-294,556; src/pages/how-it-works.mdx:309,341; src/pages/projects.mdx:125-134,229-250 — other config examples / path mentions.
- docs:src/pages/install.mdx:19-104 — no upgrade section; candidate home for upgrading.
- docs:Makefile, package.json — `npm run build`, `npx astro check`.

## External references

- gopkg.in/yaml.v3 `yaml.Node` API — lets migration steps edit settings while preserving comments and key order.
- https://justin.poehnelt.com/posts/rewrite-your-cli-for-ai-agents/ (via knowledge `architecture/cli-design-for-ai-agents.md`) — `--dry-run` for mutations; JSON output for agents.

## Prior plans / specs consulted

- Plan 000045_config-file-migration (historical) — intended `version check` to prompt and migrate legacy single-file configs; `executeMigration` shipped without a caller. Confirms the split must become a real registered step.
- Knowledge `gotchas/repoconfig-must-start-from-default.md` — migrations constructing RepoConfig must start from `NewDefaultRepoConfig()`.
- Knowledge `gotchas/ensure-footprint-discards-your-config.md` — EnsureFootprint ignores the passed config when a healthy repo.yaml exists; relevant to init cascade after migration.
- Knowledge `gotchas/remediation-needs-the-layer-that-holds-the-facts.md` — outdated/newer refusals must carry a concrete next_action and tests must assert its content.
- Knowledge `architecture/testing-architecture.md` — harbor seeded fixtures are hand-maintained couplings; must stay valid under store contracts.

## Open assumptions

- Directory values that resolve outside the project root (absolute or `../..`) were never supported by the project store (`store.go:129-130`); assumed acceptable to refuse them at load with a remediation rather than re-rooting stores.
- The dev-build version `0.1.0` is stable across tests; tests use it as the oracle for `written_by` / `skills_version`.
- Blocking `skill` (and every non-exempt command) on an out-of-date project is acceptable; installed skills always run `version check` first, which is exempt.
- `completion` is treated like `help` (exempt).
- A project whose settings have no `agent` recorded cannot have skills reinstalled by migrate; migrate reports an error naming `init <agent>`.
- Harbor suites run `init` in their Dockerfile after seeding configs, so init's automatic upgrade keeps seeded unversioned fixtures working.

## Drafting assumptions

### Chosen direction: shared upgrade engine + root gate + load-time path resolution (architecture)
- **Decision**: Option A — new `internal/migrate` engine (ordered N→N+1 step registry over yaml.Node, Inspect/Apply with dry-run) shared by `migrate`, `init`, `version check` and a root `PersistentPreRunE` gate; typed loaders refuse outdated/newer files; `ToYAMLFile` stamps `schema`/`written_by`; store dirs resolved at load to project-rooted paths and written back in file form.
- **Key design decisions**: project schema 1→2 (legacy split + skills_version carry-over)→3 (settings-relative dirs); repo schema 1→2 stamp-only; per-step writes with `<file>.v<N>.old` backup; skills installer injected into engine; skills_version stamped only after install succeeds; gate exemptions migrate/init/version check/help/completion.
- **Rationale**: satisfies every spec constraint (engine owns rewrites; loader only detects; integer keys; one engine for four callers) with no change to the ~20 store call sites.
- **Rejected**: Option B loader-embedded upgrades (violates constraint); Option C gate-in-loadConfig + re-rooted stores (misses non-loading commands, High churn). See research.md alternatives.

### Field names `schema`, `written_by`, `skills_version` (architecture)
- **Decision**: keep the spec's working names.
- **Rationale**: short, already used through the spec interview and docs plan.
- **Rejected**: `format_version`/`spektacular_version` — longer, no added clarity.

### Gate also blocks commands that never load config (architecture)
- **Decision**: gate runs for every non-exempt command when `.spektacular/config.yaml` exists (including `skill`, `knowledge`, `artifacts`); `completion` exempt like `help`.
- **Rationale**: spec says every command other than upgrade/setup/check/help is refused.
- **Rejected**: gating in `loadConfig()` only — misses `skill`.

### Missing skills version blocks like a mismatch (architecture)
- **Decision**: no `skills_version` and no legacy file ⇒ `missing` ⇒ gate refuses; migrate reinstalls.
- **Rationale**: a project with no record of installed skills cannot be known fresh; keeps the `missing` status meaningful.
- **Rejected**: letting `missing` pass — would leave projects whose skills were never stamped unchecked.

### Store-dir write-back preserves the loaded file form (architecture)
- **Decision**: Config keeps the file-form value per store dir; `ToYAMLFile` writes it back when the resolved value is unchanged, otherwise re-expresses relative to the settings folder.
- **Rationale**: prevents double-nesting when init/repo add re-save a loaded config; preserves absolute values.
- **Rejected**: changing all call sites to accessor methods (spec prefers unchanged call sites).

### Version-check statuses (architecture)
- **Decision**: add `upgrade_needed` (replaces `migration_needed`) and `unsupported_format`; mismatch/missing/upgrade_needed actions name `<command> migrate`.
- **Rationale**: spec allows folding the legacy outcome into a general upgrade-needed outcome; newer-format needs its own remediation (update Spektacular).
- **Rejected**: reporting newer-format as an error envelope — check must stay a status report skills can branch on.

### Conventions selected (architecture)
- **Decision**: 3 spektacular conventions (remediation errors, order-independent tests, tests-pass-for-done) + 5 docs conventions (content outlines, label-before-filename, MDX authoring, no em dashes, alternate shading). Dropped docs `site-layout` (no new components/pages).
- **Rationale**: each touches a surface this feature changes.
- **Rejected**: listing all conventions.

### Command name `migrate` (discovery)
- **Decision**: The upgrade command is `migrate` with a `--dry-run` flag.
- **Rationale**: Name agreed during the spec interview (working context); spec calls it "the upgrade command".
- **Rejected**: `upgrade` — would collide conceptually with upgrading the binary itself.

### Repo schema gets a stamp-only step (discovery)
- **Decision**: repo.yaml current schema = 2; step 1→2 only stamps version fields.
- **Rationale**: an unversioned repo.yaml must end up carrying a format version after upgrade, matching a fresh setup.
- **Rejected**: repo schema 1 = current — unversioned files would never be stamped.

### Refuse store directories escaping the project root (discovery)
- **Decision**: at load, a spec/plan/changelog directory that resolves outside the project root is refused with a remediation.
- **Rationale**: the project store never supported escaping the root (`internal/store/store.go:129-130`); keeps call sites unchanged.
- **Rejected**: re-rooting stores per directory — large call-site churn, out of scope.

### Relocate scanProjectMetadata into the engine (components)
- **Decision**: move the dead `scanProjectMetadata` (and delete `executeMigration`/`detectMigrationNeeded`/`migrationPrompt`) from cmd/version.go into the migrate package.
- **Rationale**: the legacy-split step needs it and the engine must not import cmd.
- **Rejected**: leaving it in cmd and passing a callback — needless indirection.

### Inspect and Apply share one Report shape (data_structures)
- **Decision**: `migrate --dry-run`, `migrate`, and Inspect all emit the same `Report` JSON.
- **Rationale**: makes "preview and apply agree" directly testable by comparing reports.
- **Rejected**: separate preview/result types — would need a mapping to compare.

### Version check keeps its JSON shape (data_structures)
- **Decision**: only the status enum changes; no extra fields added to VersionCheckResult.
- **Rationale**: preserves the contract installed skills parse; detail lives in `migrate --dry-run`.
- **Rejected**: embedding the full Report in version check output.

### Gate exemptions via cobra annotation (implementation_detail)
- **Decision**: exempt commands carry a cobra annotation; help/completion matched by name.
- **Rationale**: exemption visible where the command is defined; no drifting central list.
- **Rejected**: a hard-coded list of command paths in root.go.

### Atomic writes for upgraded files (implementation_detail)
- **Decision**: engine writes via temp file + rename.
- **Rationale**: a crash mid-write must not corrupt a settings file; matches managed-section installer practice.
- **Rejected**: plain os.WriteFile as ToYAMLFile does today.

### No new harbor scenario (testing_approach)
- **Decision**: don't add a harbor E2E for migrate; keep seeded fixtures valid and run one harbor suite at verification.
- **Rationale**: migrate is deterministic CLI behaviour fully coverable by Go command tests; harbor is for prose-driven agent behaviour.
- **Rejected**: a new harbor suite — ~25 min per run, little extra signal.

### Milestone order: engine first, blocking second, path change third (milestones)
- **Decision**: M1 engine/migrate/check (project schema 2 current), M2 gate + loader refusal, M3 store-folder step (bumps project schema to 3), M4 docs. Skills-version carry-over moved into project step 1→2 so M1 is self-contained.
- **Rationale**: each milestone ships independently; M3 proves "next format change = one new step"; the gate lands before the risky path change so no un-upgraded project can mis-resolve folders.
- **Rejected**: path change first — would re-point existing projects before any upgrade path exists.

### Harbor fixtures left unversioned (phases)
- **Decision**: don't re-seed harbor configs; every harbor task runs `spektacular init claude` first, which upgrades them.
- **Rationale**: also exercises the "setup upgrades automatically" path end-to-end; one harbor run verifies.
- **Rejected**: hand-stamping fixtures with schema/skills_version — skills_version would couple to the harbor build's ldflags version.

### Docs-site "project" check reduces to its repo.yaml (phases)
- **Decision**: success-metric check for the documentation site covers its registered repo.yaml and repo-routed changelog only.
- **Rationale**: spektacular-website/.spektacular has no config.yaml; it is a repo footprint of this project.
- **Rejected**: creating a project config there — out of scope.

### Illustrative version in docs (phases)
- **Decision**: docs examples use the next release number (checked against Makefile VERSION at implementation time), e.g. 0.16.0.
- **Rationale**: examples should look like a real release, not the dev default 0.1.0.
- **Rejected**: placeholders like <VERSION> in YAML examples — less readable.

## Rehydration cues

- `go run . repo list` → spektacular root and docs root (`/home/nicj/code/github.com/jumppad-labs/spektacular-website`).
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs`.
- Re-read: `internal/config/config.go`, `internal/config/repo.go`, `cmd/root.go`, `cmd/version.go`, `cmd/init.go`, `internal/project/init.go`, `internal/repo/footprint.go`, `internal/agent/agent.go`.
- `grep -rn "Config.Directory" cmd internal` for store-directory consumers.
- Docs: `src/pages/configuration.mdx` in the docs repo.
