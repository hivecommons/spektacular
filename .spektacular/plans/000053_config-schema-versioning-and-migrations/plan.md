---
created_date: "2026-09-19"
document_status: final
closed_date: "2026-09-19"
---

# Plan: 000053_config-schema-versioning-and-migrations

<!-- Metadata -->
<!-- Created: 2026-09-19T16:23:24Z -->
<!-- Commit: 5d7af28 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:jumppad-labs/spektacular.git -->

## Overview

Spektacular's settings files record no format version. That means the tool can't safely change how a setting is read. The immediate case is making the spec, plan and changelog folders resolve relative to the settings file, the way repo and knowledge locations already do; with no version to go on, that change would silently point existing projects at empty folders. This plan does three things:
- It stamps every settings file with a format version, the Spektacular version that wrote it, and (in project settings) the version that installed the agent skills.
- It adds a single `migrate` command, with a `--dry-run` preview, that brings a project's settings and skills current through an ordered registry of one-step upgrades. Commands are blocked until the project is current.
- It ships the folder-resolution change as the first real upgrade.

Maintainers get a safe place to ship future format changes. Users with existing projects upgrade in one step, and none of their existing specs, plans or changelog entries go missing.

## Conventions

- **Error messages must describe the problem and suggest remediation** (spektacular): every new error needs a concrete `next_action` built with `output.NewError(...).WithNextAction(...)`, and tests must assert what that `next_action` says, not just that it is non-empty. This covers the gate refusal, the outdated and newer-format load refusals, migrate step failures and a missing agent.
- **Tests must not depend on execution order** (spektacular): the new gate and migrate command tests run `rootCmd`, so they must go through `resetRootCmd`/`runRootCmd`. The new `--dry-run` flag must be reset like every other flag, and tests must not rely on the wall clock.
- **Passing tests are required before calling work done** (spektacular): the gate will reject every existing cmd test fixture that writes an unversioned config, so the whole suite has to be brought green, fixtures included, before the work counts as done.
- **Plans must sketch content structure, not just summarize it** (docs): the configuration reference changes and the new migrate section need a content outline with the exact keys, defaults and command shapes.
- **Label before filename in file-scoped reference headings** (docs): the new "Upgrading a project: migrate" section and any key additions follow the "Label: file" heading pattern.
- **MDX authoring conventions** (docs): new ConfigKey entries use slot bodies, commands go in fenced code blocks, and page bodies contain no layout HTML.
- **No em dashes** (docs): applies to all new docs prose.
- **Alternate section background shading** (docs): a new section on configuration.mdx must alternate `surface` against the section before it.

## Architecture & Design Decisions

The work is built around one **upgrade engine**: a new `migrate` package in the spektacular repo that owns every decision about whether a project is out of date and how to bring it current. It holds an ordered registry of single-format steps per settings kind: project `config.yaml` and repo `repo.yaml`. Each step is registered as N → N+1. The engine exposes two operations. **Inspect** reports what is pending: each settings file's current and target format version, whether the installed skills are stale, and which registered repos are absent. **Apply** runs the pending steps; in dry-run mode it only reports. Four callers share it, as the spec's technical approach requires:
- `version check` and a new root-command gate call Inspect.
- The new `migrate` command (with `--dry-run`) and `init` call Apply.

The first registered project step is the old single-file → config.yaml/repo.yaml split, which replaces the ad-hoc `detectMigrationNeeded` heuristic. That step revives the currently dead `scanProjectMetadata` for the new repo.yaml, seeds the repo registry the way init does, and carries the standalone `.spektacular/version` into the new `skills_version` field. The next project step moves the store folders to settings-relative values. Repo files get a stamp-only step so that every file ends up carrying a version. The format numbers are:
- config.yaml: 1 (unversioned) → 2 (split and skills version) → 3 (settings-relative store folders, current).
- repo.yaml: 1 → 2 (current).

A missing `schema` field reads as 1. The next format change registers one more step and bumps a constant. Loading and the version check need no other change (success metric 2).

Steps edit the settings as a `yaml.v3` node tree, not through the typed structs. This keeps the typed loader strictly current-format, per the constraint that loading only reads the current format and detects out-of-date files. It also preserves the comments and key order in files users edit by hand. A step returns a list of side-effect **actions**:
- rewrite a key
- create a file
- remove a file

Preview and apply walk exactly the same code path. Dry-run stops before any write, which is what makes "preview and apply agree" hold by construction.

Apply behaves as follows:
- **Backup:** before the first rewrite, it keeps a byte-identical sibling backup of each settings file, named after the starting format, e.g. `config.yaml.v1.old`.
- **Per-step writes:** it writes the file after every step, so a failure leaves the file at the last format it reached. The error names the step and the cause.
- **Order:** project settings first, then each registered repo present on disk. Absent repos are skipped and named. The skills reinstall comes last.
- **Skills reinstall:** it goes through the existing `agent.Lookup(cfg.Agent).Install(...)` path, injected as an installer function so the engine does not import the agent layer. `skills_version` is stamped only after the install succeeds. A failed reinstall therefore leaves skills reported as stale.

`init` runs Apply (without the skills phase) before `project.Init` touches the settings. After its own install succeeds, it stamps `skills_version` in place of writing `.spektacular/version`.

**Detection and blocking** happen in three layers:
- **Typed loaders:** `config.ParseYAMLFile` / `FromYAMLFile` and `config.RepoConfigFromYAMLFile` read the `schema` field. They refuse an older file with a `config_outdated` error whose next_action is `<command> migrate`. They refuse a newer file with `config_newer_format`, whose next_action says to update Spektacular. Both follow the error-remediation convention.
- **Root gate:** a `PersistentPreRunE` on the root command calls Inspect whenever a project config exists. It refuses with `upgrade_required` when any file is behind or the skills are stale, and with `config_newer_format` when any file is ahead. It exempts `migrate`, `init`, `version check`, `help` and `completion`.
- **Repo footprint:** `repo.EnsureFootprint` stops treating a format refusal as a "broken" repo.yaml, so a newer-format file is never silently overwritten.

`version check` keeps `match`/`mismatch`/`missing` for skills freshness. It adds `upgrade_needed` (which replaces `migration_needed`) and `unsupported_format`. Every non-match action now names `migrate`. The five SKILL.md preambles are updated to match, and they still never apply an upgrade themselves.

**Version stamping** lives in the two writers, `Config.ToYAMLFile` and `RepoConfig.ToYAMLFile`. Both always write the current `schema` and a `written_by` value taken from a package-level writer version that `cmd` sets from its build `version`, so every existing writer is covered without changes. `skills_version` is a plain field on the project `Config`. Only init's install path and the migrate skills phase set it, so a rewrite by `repo add` leaves it alone.

**Settings-relative store folders** are resolved once, at load. The spec/plan/changelog `directory` values in the file are relative to the folder holding config.yaml, and the defaults become `specs`, `plans` and `changelog`. The loader turns each value into the project-root-relative path the ~20 existing call sites already expect: `specs` becomes `.spektacular/specs`. None of those call sites change. The known trap is that three writers re-save a config they loaded (init, `repo add`, the guided repo flow). The in-memory `Config` therefore remembers the file-form value it loaded. `ToYAMLFile` writes that value back unless the resolved path changed, in which case it re-expresses the new path relative to the settings folder. That prevents double-nesting such as `.spektacular/.spektacular/specs`. A directory that resolves outside the project root is refused at load with a corrective next_action. The project store never supported that case.

The rejected alternatives were:
- upgrading silently inside the loader
- keying upgrades on the binary version
- gating only in `loadConfig`
- re-rooting every store at the settings folder
- round-tripping settings through maps

The evidence for each is in research.md#alternatives-considered-and-rejected.

**Docs** land in the docs repo (spektacular-website). The configuration reference gains:
- the three version fields
- the settings-relative path rule and the new default folders
- an "Upgrading a project: migrate" section covering `--dry-run` and the blocking behaviour

Pages that show the old `.spektacular/specs` example values are updated to match.

## Component Breakdown

- **Upgrade engine (new, `migrate` package):** The single authority on whether a project is out of date and how to bring it current.
  - **Owns:** the step registries for project and repo settings; format-version detection from raw YAML; the Inspect report (pending steps per file, skills freshness, absent repos); and Apply. Apply supports dry-run, per-step writes, `<file>.v<N>.old` backups and failure reporting that names the step.
  - **Depends on:** the settings layer, for the current-format constants and `ProjectConfigDir`/`ResolvedLocation`, and on an injected skills installer. It never imports the agent or cmd layers.
  - **Used by:** the gate, `version check`, `migrate` and `init`.

- **Upgrade steps (new, registered in the engine):** Each step moves one settings kind up exactly one format version and owns its own detection and edits. The initial set:
  - **Project 1→2, legacy split and skills version:** creates a colocated repo.yaml from the project metadata scan when none exists, seeds the repo registry, and carries the standalone skills-version file into `skills_version`. It removes that file.
  - **Project 2→3, settings-relative store folders:** rewrites the spec, plan and changelog directories into settings-relative form.
  - **Repo 1→2, version stamp:** records the format version only.
  - **Contract:** a step edits a YAML node tree and returns side-effect actions. It never writes on its own.

- **Project metadata scanner (existing, relocated):** The README/go.mod/package.json heuristics that today sit unused beside `version check`. They move into the engine so the legacy-split step can use them. They never fail.

- **Settings model (changed, config package):**
  - **Version fields:** gains `schema` and `written_by` on both project and repo settings, and `skills_version` on project settings. It also gains the current-format constants and a package-level writer version.
  - **Loaders:** detect and refuse out-of-date or newer-format files with remediating errors. They also resolve settings-relative store folders into project-rooted paths, remembering each folder's file-form value.
  - **Writers:** stamp the current format and writer version, and write the store folders back in file form.
  - **Defaults:** become `specs`, `plans` and `changelog`.
  - **Boundary:** it stays free of any upgrade logic.

- **Command gate (new, root command):** A pre-run hook on the root command. It asks the engine's Inspect whether the project is behind (format or skills) or ahead, and refuses with `upgrade_required` or `config_newer_format` and a next_action. It exempts `migrate`, `init`, `version check`, `help` and `completion`, and it stays silent when there is no project.

- **`migrate` command (new):** The user-facing upgrade.
  - **Apply:** runs the engine's Apply across the project settings and every registered repo present on disk, then reinstalls skills for the configured agent when stale.
  - **`--dry-run`:** reports the same change list without touching disk.
  - **Output:** a structured JSON report of each change, backup locations, skipped repos and the skills outcome. When nothing is pending, it reports that nothing was needed.

- **`version check` (changed):** Reports skills freshness from `skills_version`, falling back to the legacy file. It gains `upgrade_needed` (replacing `migration_needed`) and `unsupported_format` from the engine's Inspect. Every non-match action names `migrate`, or updating Spektacular for the newer-format case. The existing `match`/`mismatch`/`missing` values keep their meaning.

- **`init` / project setup (changed):** Runs the engine's Apply for the format steps before scaffolding. After a successful skills install it stamps `skills_version` in project settings, and it stops writing the standalone version file.

- **Repo footprint (changed):** Treats a format-version refusal as a hard error instead of a broken file to repair, so an upgrade or newer-format repo.yaml is never overwritten.

- **Installed skill preambles (changed, templates):** The five workflow skills' version-check preamble handles the new statuses and tells the user to run `migrate` (or update Spektacular). The skills still never run it themselves.

- **Configuration reference docs (changed, docs repo):**
  - **Configuration page:** documents the three version fields, the settings-relative folder rule with the new defaults, the `migrate` command and its preview mode, and blocking.
  - **Other pages:** example config blocks on other pages follow the new default values.

## Data Structures & Interfaces

**Settings version fields (config package).** The new top-level YAML keys are:
- **Both files:** `schema` and `written_by`.
- **config.yaml only:** `skills_version`.

In a file, a missing or zero `schema` means format 1. The package exports each kind's current format and the value writers stamp.

```go
const (
    CurrentProjectSchema = 3 // config.yaml (2 until Milestone 3 registers the store-folder step)
    CurrentRepoSchema    = 2 // repo.yaml
)
var WriterVersion = "unknown" // set by cmd from its build version

type Config struct {
    Schema        int    `yaml:"schema"`
    WrittenBy     string `yaml:"written_by,omitempty"`
    SkillsVersion string `yaml:"skills_version,omitempty"`
    // ...existing fields...
}
type RepoConfig struct {
    Schema    int    `yaml:"schema"`
    WrittenBy string `yaml:"written_by,omitempty"`
    // ...existing fields...
}
```

**Store directory contract.** On a loaded `Config`:
- **In memory:** `Spec/Plan/Changelog.Config.Directory` hold project-root-relative paths, for example `.spektacular/specs`, so every consumer is unchanged.
- **On disk:** the file holds settings-relative values, for example `specs`.
- **Round trip:** each directory struct carries an unexported `fileForm` string (the value as read) that `ToYAMLFile` uses to round-trip unchanged values.

**Format refusal errors (config package).** A typed error lets callers tell a format refusal apart from a malformed file. The footprint code relies on this to avoid overwriting.

```go
type FormatError struct {
    Path   string
    Kind   string // "project" | "repo"
    Found  int    // normalised: 0/missing → 1
    Want   int
}
func (e *FormatError) Newer() bool
func IsFormatError(err error) (*FormatError, bool)
```

At the command surface these become `output` errors with codes `config_outdated` (next_action: `<command> migrate`) and `config_newer_format` (next_action: install a newer Spektacular). `Kind` stays a plain string. Adding a named type would be ceremony for two values.

**Upgrade engine (migrate package).**

```go
type Kind string // "project" | "repo"

// A single N→N+1 upgrade. Steps edit the in-memory YAML tree and describe
// side effects; they never touch disk themselves.
type Step struct {
    Kind        Kind
    From        int    // upgrades From → From+1
    Description string
    Run         func(sc *StepContext, doc *yaml.Node) ([]Action, error)
}
type StepContext struct {
    ProjectRoot string // parent of .spektacular
    FileDir     string // folder holding the file being upgraded
}

type Action struct {
    Op     string // "set" | "create" | "remove"
    Path   string // file path (for create/remove) or settings file (for set)
    Key    string `json:",omitempty"` // dotted key for "set"
    From   string `json:",omitempty"`
    To     string `json:",omitempty"`
    Content []byte `json:"-"` // payload for "create"
}

type Installer func(agent string) error

type Options struct {
    ProjectRoot   string
    BinaryVersion string
    DryRun        bool
    Skills        bool      // false for init (it installs itself)
    Install       Installer // required when Skills
}

func Inspect(projectRoot, binaryVersion string) (Report, error)
func Apply(opts Options) (Report, error)
func Registered(kind Kind) []Step // ordered; tests assert contiguity 1..Current-1
```

**Engine report (the `migrate` JSON body and the Inspect result).** `Inspect` and `Apply` return the same shape. That shared shape is what makes preview and apply comparable.

```go
type Report struct {
    Status  string       `json:"status"` // "up_to_date" | "upgrade_needed" | "upgraded" | "unsupported_format"
    DryRun  bool         `json:"dry_run"`
    Files   []FileReport `json:"files"`
    Skipped []SkippedRepo `json:"skipped_repos,omitempty"`
    Skills  SkillsReport `json:"skills"`
}
type FileReport struct {
    Path     string   `json:"path"`
    Kind     Kind     `json:"kind"`
    From     int      `json:"from_schema"`
    To       int      `json:"to_schema"`
    Steps    []string `json:"steps"`   // step descriptions applied/pending
    Actions  []Action `json:"actions"`
    Backup   string   `json:"backup,omitempty"` // set on apply only
}
type SkippedRepo struct { Name, Location, Reason string }
type SkillsReport struct {
    Installed string `json:"installed_version,omitempty"` // from skills_version or legacy file
    Current   string `json:"current_version"`
    Status    string `json:"status"` // "match" | "mismatch" | "missing"
    Agent     string `json:"agent,omitempty"`
    Reinstall bool   `json:"reinstall"` // dry-run: would; apply: did
}
```

On failure, `Apply` returns the partial `Report` together with a `*StepError`. The error carries `{Path, Step, ReachedSchema, Cause}`, which the command surfaces as `migrate_failed` with the reached format in the message.

**`version check` result (changed).** The `VersionCheckResult` shape is unchanged. The status enum becomes `match | mismatch | missing | upgrade_needed | unsupported_format`, and `migration_needed` is removed. `installed_version` is sourced from `skills_version`, falling back to the legacy file.

**No other new interfaces.** `agent.Agent` is reused unchanged through the `Installer` closure.

## Implementation Detail

**New pattern: a versioned-settings upgrade registry.** This is the codebase's first forward-only schema migration mechanism. Each upgrade is a small value with these parts:
- a kind
- a from-version
- a one-line description
- a function

The registry lives next to the steps in one package, and contributors add steps in version order. A registry test asserts two things for each kind: the steps are contiguous from 1 up to the current format, and the last step lands exactly on the config package's current constant. That test is what makes "the next format change ships as a single new step" enforceable. A contributor who bumps the constant without registering a step, or registers a step without bumping, gets a failing test instead of a silently unreachable format.

**Steps work on YAML node trees, not typed structs.** A step receives the parsed document node. It edits keys through a small set of helpers in the engine: get a scalar by dotted path, set a scalar, insert a mapping entry, and delete. Comments, key order and unrelated keys survive. Steps never see a `config.Config`. This keeps the typed model strictly current-format and lets an old file be upgraded even when it would fail current validation. The engine owns all I/O:
- **Reading and detection:** read the file, parse the node tree, read `schema`.
- **Running steps:** run the pending steps in order, collecting actions.
- **Dry-run:** stop at that point.
- **Apply:** back up the file once, then for each step perform its create/remove actions, bump `schema`, write the file atomically (temp file plus rename), and move to the next step.

This follows the atomic-write practice the managed-section installer already uses.

**Engine phases are fixed and ordered.**
1. **Project settings:** upgrade config.yaml through its steps.
2. **Repos:** read the repo registry from the upgraded (in-memory, for dry-run) project document, resolve each entry with the same settings-folder rule the rest of the tool uses, then upgrade each repo.yaml that exists on disk. The colocated repo is covered as an ordinary entry. Missing locations are recorded as skipped and do not fail the run.
3. **Skills:** only when asked and stale.

Inspect is the same code with dry-run forced on and no skills installer. The gate and `version check` therefore report exactly what `migrate` would do.

**Loader behaviour changes, but stays detect-only.** The two typed loaders gain a pre-decode peek at `schema` and refuse any file that is not at the current format. The project loader then does one extra thing after decoding: it resolves the three store folders against the settings folder into project-root-relative paths, and remembers the value it read. The existing legacy rejectors and validators stay as they are, and `Validate` additionally refuses a store folder that escapes the project root. The writers stamp `schema`/`written_by` and write store folders back in file form. That is the only change most readers of the code will notice in the config package: the value in memory and the value on disk for these three keys now differ by design. Doc comments on the fields must say so.

**The cobra tree gains its first pre-run hook.** The gate is a `PersistentPreRunE` on the root command. Exemptions are declared with a cobra annotation on the exempt commands (`migrate`, `init`, `version check`), plus a name check for cobra's generated `help`/`completion`. That keeps the exemption visible at each command's definition rather than in a central list that drifts. No subcommand defines its own persistent pre-run today, so the hook is inherited everywhere. The debug-probe config load in the root runner must stay lenient: an out-of-date file disables the probe instead of erroring.

**Existing patterns followed.**
- **Errors:** they go through the existing error envelope with `next_action`.
- **Tests:** they use the existing `resetRootCmd`/`runRootCmd` harness.
- **RepoConfig construction:** the legacy-split step builds its repo.yaml from `NewDefaultRepoConfig()` plus the scanned metadata, per the knowledge gotcha.
- **Skills install:** it reuses `Agent.Install` unchanged.

The cmd layer builds the installer closure around `Agent.Install`, so the engine stays free of agent and cmd imports.

**Clean-up.** The dead migration helpers beside `version check` are deleted, along with their tests, and the metadata scanner moves into the engine. Two outdated comments are corrected: the store-directory defaults comment and the "project-root relative" comment on the project-root helper.

**Test-fixture impact is a first-class part of the change.** The gate refuses any project whose settings lack a current `schema` or whose skills are not stamped. Every test helper that hand-writes a config.yaml or repo.yaml must therefore write current-format files with `skills_version` matching the test build version, as must the harbor seeded fixtures. Centralising this in the existing config-writing test helper keeps the churn to one place per package, and a grep for raw `config.yaml` writes in tests finds any stragglers.

## Dependencies

- **Uncommitted repo-knowledge resolution fix in project setup (spektacular, on `main`).** The init change that resolves repo.yaml's knowledge location against the settings folder must be committed, or carried into this branch, **before this plan starts**. The new init flow builds on it, and its tests would conflict.
- **`gopkg.in/yaml.v3` (existing external library).** Its `yaml.Node` API is what steps use to edit settings while preserving comments and order. It is already a dependency, so no change is needed.
- **Config package (existing).** It supplies the settings model, the loaders, the writers, `ProjectConfigDir` and `RepoEntry.ResolvedLocation`. It changes to add version fields, format refusal, store-folder resolution and write-back.
- **Agent package (existing).** `agent.Lookup` and `Agent.Install` are reused unchanged for the skills reinstall, wrapped in an installer closure.
- **Repo package (existing).** `EnsureFootprint` changes so it no longer overwrites a repo.yaml it refuses on format grounds. `repo.New`/`Resolve` are not used by the engine. It resolves locations directly, so it works for projects that are not current yet.
- **Output package (existing).** `output.NewError(...).WithNextAction(...)` and the result envelope are reused unchanged for every new refusal and for the `migrate` report.
- **Cobra (existing external library).** It provides `PersistentPreRunE` and command annotations for the gate. No version change is needed.
- **Workflow skill templates (existing).** The five SKILL.md version-check preambles change. The test assertions on the preamble text change with them.
- **Harbor E2E fixtures (existing, not in CI).** The seeded configs in the implement-workflow environment are unversioned and use the old store-folder values. They must stay valid, either through init's automatic upgrade or by being re-seeded at the current format.
- **Prior spec/plan 000045 config-file-migration (historical).** It introduced the legacy split detection and the unused migration helpers that this plan replaces. It is not a blocker.
- **Docs repo, spektacular-website (registered repo `docs`).** Its configuration reference and example pages change. Its own build (`npm run build`, `npx astro check`) must pass. Nothing needs to land there first.

## Testing Approach

Testing follows the project's three-layer architecture. Go unit tests cover the engine and config package. Go command tests drive `rootCmd` through the existing `resetRootCmd`/`runRootCmd` harness and assert on the JSON envelope. Template-contract assertions cover the SKILL.md preambles. Harbor E2E is not extended: its seeded fixtures only need to stay valid, and one harbor run is part of final verification because the fixtures are hand-maintained couplings. The docs repo has no test suite; its verification is its build plus type check.

**Most coverage goes to the upgrade engine and the command gate**, because they decide whether a user's work is found or hidden. The load-bearing assertions are:

- **Preservation:** on an unversioned project containing specs, plans and changelog entries, after `migrate` every entry is still returned by the list commands, and a new spec lands beside the old ones. This is the headline regression guard.
- **Oracles are hand-maintained fixtures:** a representative unversioned (legacy single-file and split) config.yaml/repo.yaml pair with the exact expected post-upgrade YAML is checked in as literal test data. It is never produced by running the engine (per the independent-oracle rule).
- **Preview equals apply:** the dry-run report on a fixture equals the report from the following apply, excluding backup paths. Also, `snapshotDir` of the project before and after dry-run is byte- and path-identical.
- **Idempotence:** a second `migrate` reports `up_to_date` and the snapshot is unchanged.
- **Failure leaves a known state:** making a folder a step must write read-only yields a `migrate_failed` error naming the step and cause, and the file's `schema` is the last reached value.
- **Backups:** the reported backup path exists and is byte-identical to the original.
- **Newer format:** a `schema` above current is refused by every loading command and by `migrate`, the file is unchanged, and the `next_action` says to update Spektacular. The test asserts the `next_action` content, not just that it is non-empty.
- **Registry contiguity:** for each kind, the steps run 1..current-1 with no gaps and end on the config package's constant.
- **Gate:**
  - Out-of-date format or stale/missing skills make a spec command fail with `upgrade_required`, create no files, and give a `next_action` containing `migrate`.
  - `migrate`, `init`, `version check`, `help` and `completion` run while blocked.
  - A differing `written_by` alone never blocks.
- **Skills:**
  - A stale-skills project is reinstalled for the configured agent, and `skills_version` becomes the build version with no other settings changes.
  - With an injected failing installer, `skills_version` is unchanged and `version check` still reports a non-match.
  - A legacy `.spektacular/version` is read by the check, carried into settings by `migrate`, and deleted.
- **Store folders:**
  - After upgrade, setting `spec.config.directory: x` writes a new spec under `.spektacular/x`, and the same holds for plans and changelog.
  - Writers round-trip the file form: `repo add` on a current project leaves `specs` as `specs`, not `.spektacular/specs`.
  - An escaping value is refused with a remediation.
- **Absent repos:** they are skipped and named, and the rest completes.
- **Fresh setup:** both files carry the current `schema`, `written_by` and `skills_version` equal to the build version, specs/plans land in the same on-disk folders as before, and the file holds `specs`/`plans`/`changelog`.

**Fixture migration is part of the work.** Every existing cmd, config, project and repo test fixture that writes settings by hand is brought to the current format. This happens mainly by updating the shared config-writing helper, so the suite stays green under the gate. Existing legacy-rejection and dead-migration tests are removed or rewritten along with the code they covered.

**Deliberate gaps.**
- No property or fuzz testing of YAML node editing; fixture tests over the real legacy shapes are sufficient.
- No concurrency tests. Concurrent `migrate` runs are not a supported scenario.

**Success metrics:**

- *Zero existing specs, plans or changelog entries go missing after upgrading a pre-feature project.* **Behavioural test:** the preservation test above, on a fixture project. The spec also names two real projects, this repository and the documentation-site project, as a release check. That part is **Manual — captured in the implementation test plan**.
- *The next settings-format change ships as a single new upgrade step, with no change to config loading or the check.* **Behavioural test:** the registry-contiguity test pins the step/constant coupling, and a test registers a synthetic extra step in a test-only registry to prove the engine, gate and check pick it up with no other change.
- *Existing projects upgrade by re-running setup alone.* **Behavioural test:** `init claude` on an unversioned fixture project leaves every settings file current and `version check` reporting `match`, with no `migrate` run and no hand edits.
- *Moving to a new release takes one command to bring settings and skills current.* **Behavioural test:** on a fixture that is both format-behind and skills-stale, one `migrate` run makes `version check` report `match`.

## Milestones & Phases

### Milestone 1: Settings carry their version and one command upgrades a project

**What changes**:
- **Version fields:** every settings file Spektacular writes now records its format version and the Spektacular version that wrote it. Project settings also record which Spektacular version installed the agent skills; this replaces the standalone version file.
- **`migrate` command:** users can run it to bring a project's settings, and the settings of every registered repo on disk, up to the current format. It also reinstalls stale agent skills. A `--dry-run` preview lists every change without touching disk.
- **Legacy projects:** older single-file projects upgrade through the same command.
- **Setup and the installation check:** re-running setup applies pending upgrades automatically, and the installation check reports out-of-date projects with an action naming `migrate`.

Nothing is blocked yet, so existing users see new fields appear, a new command, and clearer check results.

**Validation point**: `migrate --dry-run` on an unversioned project changes nothing on disk and lists the same changes that a following `migrate` makes. After `migrate`, `version check` reports `match` and a second `migrate` reports nothing to do.

### Milestone 2: Out-of-date or too-new projects are stopped before they can do damage

**What changes**: until the project is upgraded, commands refuse to run when the project's settings are behind the current format or its installed skills are stale. The refusal message names `migrate`. The exceptions are `migrate`, setup, the installation check and help, so the way out is always reachable. Settings written by a newer Spektacular are refused everywhere, with a message to update Spektacular, and such a file is never overwritten. From this point, a future format change can be shipped safely: nobody can keep working against a file whose meaning has changed.

**Validation point**: on an unversioned project a spec command fails with an error naming `migrate` and creates no files, while `migrate`, `init`, `version check` and `help` still run. A file with a higher format version is refused and left byte-identical. The full test suite passes with every fixture at the current format.

### Milestone 3: Spec, plan and changelog folders follow the settings file

**What changes**:
- **Relative folder settings:** the spec, plan and changelog folder settings are now read relative to the folder holding the settings file, the same rule as repo and knowledge locations.
- **New defaults:** new projects write `specs`, `plans` and `changelog`, which land in the same place on disk as before.
- **Existing projects:** `migrate` (or re-running setup) rewrites their folder settings to the equivalent settings-relative values, so every existing spec, plan and changelog entry stays visible and new ones land beside them.

This is the first real format change delivered through the Milestone 1 upgrade mechanism.

**Validation point**: after upgrading a fixture project that holds specs, plans and changelog entries, every entry is still listed, and a new spec lands beside the existing ones. Setting the spec folder to `x` puts new specs in `.spektacular/x`. Running `repo add` afterwards leaves the file's `specs` value untouched.

### Milestone 4: The documentation site explains versions, the new paths and upgrading

**What changes**: the documentation site's configuration reference covers:
- the format-version, writing-version and installed-skills-version fields
- the settings-relative folder rule and the new default values
- the `migrate` command, including its `--dry-run` preview
- the fact that out-of-date projects are blocked until upgraded

Example configuration elsewhere on the site uses the new default values.

**Validation point**: the site builds and type-checks cleanly. The configuration page shows the three fields, the new defaults and the settings-relative rule, and has a `migrate` section with a `--dry-run` example and the blocking note.

### Milestone 1: Settings carry their version and one command upgrades a project

#### - [x] Phase 1.1: Settings files record their format and writer versions
**Repo:** spektacular

Project and repo settings gain a format version and a "written by" field. Project settings also gain the installed skills version. Every save stamps the current format and the running Spektacular's version automatically, so no existing writer has to change. Loading still accepts older files in this phase. Nothing is blocked yet.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-settings-files-record-their-format-and-writer-versions)

**Acceptance criteria**:
- [x] A freshly written config.yaml and repo.yaml each contain the current format version and the running Spektacular version.
- [x] Rewriting config.yaml for an unrelated reason (for example registering a repo) leaves any recorded installed-skills version unchanged.
- [x] Settings files with no format version still load exactly as before.

#### - [x] Phase 1.2: An upgrade engine with the first registered steps
**Repo:** spektacular

This phase adds the engine that works out what is pending and applies upgrades one format at a time, plus its first three steps:
- the legacy single-file split
- carrying the standalone skills-version file into the settings
- stamping repo files

The engine previews and applies through the same path, backs files up before rewriting them, writes after every step, and reports exactly where it stopped if a step fails. It can also reinstall skills through an installer it is given.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-an-upgrade-engine-with-the-first-registered-steps)

**Acceptance criteria**:
- [x] An unversioned project and its repos are reported as pending with the steps they need, and a current project is reported as having nothing pending.
- [x] A preview changes nothing on disk and reports the same changes a following apply makes.
- [x] An old single-file project gains a repo settings file and a repo registry entry, and its standalone skills-version value moves into the settings.
- [x] Each rewritten file has a byte-identical backup beside it, and its location is reported.
- [x] A step that fails leaves the file at the last format it reached, and the failure names the step and the reason.
- [x] Registered repos that are not on disk are skipped and named, and the rest of the upgrade completes.
- [x] A skills reinstall that fails leaves the recorded installed-skills version untouched.

#### - [x] Phase 1.3: The migrate command, an honest installation check, and self-upgrading setup
**Repo:** spektacular

This phase adds the user-facing `migrate` command, with a `--dry-run` preview, on top of the engine. The installation check starts reporting out-of-date settings and stale skills with an action pointing at `migrate`. Setup applies pending upgrades before it does anything else, and after installing skills it records their version in the settings instead of in the standalone file. The installed skills' version-check instructions are updated to match. The old dead migration helpers are removed.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-the-migrate-command-an-honest-installation-check-and-self-upgrading-setup)

**Acceptance criteria**:
- [x] `migrate` on a project that is both format-behind and skills-stale leaves the installation check reporting a match after one run.
- [x] `migrate --dry-run` reports every settings change, every file created or removed, and whether skills would be reinstalled, and changes nothing.
- [x] Running `migrate` a second time reports that nothing was needed and changes no file.
- [x] Re-running setup on an unversioned project leaves every settings file at the current format without a separate `migrate` run.
- [x] A project that only has the standalone skills-version file is checked against it, and after upgrading the value is in the settings and the file is gone.
- [x] A different "written by" version on its own never makes the check report a problem.
- [x] The installed skills tell the user to run `migrate` on any non-match, and never run it themselves.

### Milestone 2: Out-of-date or too-new projects are stopped before they can do damage

#### - [x] Phase 2.1: Loading refuses settings in the wrong format
**Repo:** spektacular

Loading a project or repo settings file now checks its format version. An older file is refused with an instruction to run `migrate`, and a newer one is refused with an instruction to update Spektacular. Repo scaffolding no longer treats either case as a broken file to overwrite, and the debug logger tolerates an out-of-date file.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-loading-refuses-settings-in-the-wrong-format)

**Acceptance criteria**:
- [x] A settings file with a higher format version than Spektacular supports is refused with a message to update Spektacular, and the file is left byte-identical.
- [x] A settings file behind the current format is refused with a message naming `migrate`.
- [x] Setup and repo registration never overwrite a repo settings file that was refused for its format.

#### - [x] Phase 2.2: Commands are blocked until the project is upgraded
**Repo:** spektacular

A check that runs before every command refuses to proceed when any settings file is behind or ahead of the current format, or when installed skills are stale or unrecorded. The upgrade command, setup, the installation check and help always stay available. Every existing test fixture that writes settings by hand is brought to the current format, so the suite runs under the gate.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-commands-are-blocked-until-the-project-is-upgraded)

**Acceptance criteria**:
- [x] On an unversioned project, a spec command fails with an error naming `migrate` and creates no files, and after `migrate` the same command runs.
- [x] On a project with stale skills, a spec command is refused, and after `migrate` it runs.
- [x] `migrate`, setup, the installation check and help all run on an out-of-date project.
- [x] Commands in a directory with no project still report that no project exists, as before.
- [x] The full test suite passes with the gate active.

### Milestone 3: Spec, plan and changelog folders follow the settings file

#### - [x] Phase 3.1: Store folders resolve from the settings file, with an upgrade step for existing projects
**Repo:** spektacular

The spec, plan and changelog folder settings are now read relative to the folder holding config.yaml. New projects default to `specs`, `plans` and `changelog`. Loading converts these values into the paths the rest of the tool already uses, and saving writes them back in their settings-relative form. A new registered upgrade step rewrites an existing project's folder settings to the equivalent settings-relative values. It is the first format change delivered through the engine.

*Technical detail:* [context.md#phase-31](./context.md#phase-31-store-folders-resolve-from-the-settings-file-with-an-upgrade-step-for-existing-projects)

**Acceptance criteria**:
- [x] After upgrading a project that holds specs, plans and changelog entries, every one of them is still listed and a new spec lands beside the existing ones.
- [x] A settings file two formats behind reaches the current format in one run, with the same folder locations on disk, registered repos and configured agent.
- [x] Setting the spec folder to `x` places new specs in `x` inside the settings folder, and the same holds for plans and changelogs.
- [x] A new project stores specs, plans and changelogs in the same folders on disk as before, and its settings read `specs`, `plans` and `changelog`.
- [x] Registering a repo in an upgraded project leaves the folder settings exactly as written.
- [x] A folder setting that points outside the project is refused with a corrective instruction.

#### - [x] Phase 3.2: Upgrade Spektacular's own projects and update the README
**Repo:** spektacular, docs

The spektacular repository's own project is upgraded with the new command. That upgrade includes the documentation site's registered repo settings, because the docs site has no project settings of its own, only a repo settings file. The phase confirms by hand that nothing disappears from listings, including the docs repo's changelog entries. The README's configuration section is updated to the new fields, defaults and upgrade command.

*Technical detail:* [context.md#phase-32](./context.md#phase-32-upgrade-spektaculars-own-projects-and-update-the-readme)

**Acceptance criteria**:
- [x] After upgrading this repository, the spec, plan and changelog listings contain exactly the same entries as before.
- [x] The documentation site's repo settings are at the current format, and its changelog entries are still reachable through repo-routed listing.
- [x] The README shows the version fields, the settings-relative folder rule with new defaults, and the `migrate` command.

### Milestone 4: The documentation site explains versions, the new paths and upgrading

#### - [x] Phase 4.1: Configuration reference covers versions, folder rule and migrate
**Repo:** docs

This phase extends the configuration reference with:
- the three version fields
- the settings-relative rule for the spec, plan and changelog folders, with their new defaults
- a new section on upgrading a project with `migrate`, covering its `--dry-run` preview and the fact that out-of-date projects are blocked

*Technical detail:* [context.md#phase-41](./context.md#phase-41-configuration-reference-covers-versions-folder-rule-and-migrate)

**Content outline** (configuration.mdx):

1. **Project configuration: config.yaml** (existing Section). The example gains the version lines at the top and shows the new folder values:
   ```yaml
   schema: 3                     # format version; written by Spektacular
   written_by: 0.16.0            # Spektacular version that last wrote this file
   skills_version: 0.16.0        # Spektacular version that installed the agent skills
   name: my-project
   spec:
     provider: file
     config:
       directory: specs          # relative to the folder holding config.yaml
   plan:
     config:
       directory: plans
   changelog:
     config:
       directory: changelog
   ```
2. **Project configuration keys** (existing ConfigurationKeys). The sub text now counts twelve top-level keys. The new ConfigKeys are:
   - `schema` (integer, written by Spektacular): "The settings format version. Never edit it by hand; `migrate` raises it."
   - `written_by` (string): "The Spektacular version that last saved this file. Informational only; it never triggers an upgrade."
   - `skills_version` (string): "The Spektacular version that last installed your agent skills. Updated only by `init` and `migrate`."

   The spec, plan and changelog keys change their default value to `specs`/`plans`/`changelog` and add the sentence "Relative to the folder holding config.yaml, like `repos[].location`."
3. **Repository configuration: repo.yaml** (existing Section). The example gains `schema: 2` and `written_by`.
4. **Repository configuration keys** (existing ConfigurationKeys) gain `schema` and `written_by` entries.
5. **Upgrading a project: migrate** (new Section, `surface` alternated against the section before it):
   - Prose: when Spektacular changes the settings format, or you install a new release, commands stop with an error naming `migrate` until you upgrade. `migrate`, `init`, `version check` and `help` always work.
   - Preview:
     ```bash
     spektacular migrate --dry-run
     ```
   - Apply:
     ```bash
     spektacular migrate
     ```
   - A bullet list of what it does:
     - upgrades config.yaml and every registered repo's repo.yaml present on disk
     - keeps a `.v<N>.old` copy of each rewritten file
     - skips and names repos not on disk
     - reinstalls agent skills when they are stale
   - A note that re-running `spektacular init <agent>` applies the same upgrades, and that a file from a newer Spektacular is refused with a request to update.

**Acceptance criteria**:
- [x] The configuration page shows `schema`, `written_by` and `skills_version`, with what each means.
- [x] The spec, plan and changelog keys show the new defaults and state the settings-relative rule.
- [x] A new upgrading section documents `migrate`, its `--dry-run` preview and that out-of-date projects are blocked until upgraded.
- [x] The site builds and type-checks with no errors or warnings.

#### - [x] Phase 4.2: Example configuration across the site uses the new defaults
**Repo:** docs

Tutorials and explanation pages that show the old `.spektacular/specs`-style folder values in configuration examples are updated to the new values. Prose that names on-disk locations is checked and left alone where the location itself has not moved.

*Technical detail:* [context.md#phase-42](./context.md#phase-42-example-configuration-across-the-site-uses-the-new-defaults)

**Content example** (getting-started.mdx config block):
```yaml
spec:
  provider: file
  id_method: timestamp
  config:
    directory: specs    # relative to .spektacular/, where config.yaml lives
```

**Acceptance criteria**:
- [x] No configuration example on the site shows `directory: .spektacular/...` for spec, plan or changelog.
- [x] On-disk folder descriptions (such as "specs are created in `.spektacular/specs`") remain accurate.
- [x] The site builds and type-checks cleanly.

## Open Questions

- **Does any existing `cmd` or `internal` test depend on the exact byte layout of a written config.yaml/repo.yaml** (for example a golden-file comparison), which the new leading `schema`/`written_by` keys would break? *Depends on:* the full suite running once the Phase 1.1 stamping lands. Research found only substring and parsed-value assertions, but that list was not exhaustive. *Implementer action:* update the oracle by hand to the new literal layout, and never regenerate it from the code under test. If a test turns out to assert that written files are byte-identical to user-authored input, STOP and ask the user whether stamping on every save is acceptable there.
- **Does re-encoding a `yaml.Node` with `yaml.v3` reproduce untouched parts of real user files faithfully enough** (comments, quoting, flow-style lists such as `tags: [docs]`)? *Depends on:* running the engine against this repo's and the harbor fixtures' real settings in Phase 3.2. *Implementer action:* if formatting drift is only cosmetic (indentation, quoting), accept it and note it in the migrate output docs. If comments are lost or values change, STOP and ask the user before upgrading real projects.

## Out of Scope

- **Removing the stray project-root `knowledge/` folder** that older projects, this repository included, carry because of a past setup defect. The defect itself is fixed separately by the uncommitted repo-knowledge resolution change listed under Dependencies (spec Non-Goal).
- **Downgrading settings to an older format.** Only forward upgrades exist, and a newer-format file is refused rather than converted (spec Non-Goal).
- **Automatic rollback of a completed upgrade.** Recovery is by hand, from the `.v<N>.old` copy or version control (spec Non-Goal).
- **Changing any other settings value or on-disk layout** in this feature's format upgrades. This excludes moving the store folders on disk and converting the other legacy shapes that loaders already reject with a manual fix (`address`, `knowledge.scope`, repo-level `knowledge.sources`); those rejectors stay as they are (spec Non-Goal).
- **Store folders outside the project root** (absolute paths elsewhere, or relative values climbing above the project). They are refused with a corrective error, not supported, because the project store never supported them.
- **The existing mismatch between the central changelog listing (namespaced by project name) and `changelog file write` without `--repo` (not namespaced).** It is a pre-existing inconsistency found during research and is not touched here.
- **Concurrent `migrate` runs** against one project. They are not guarded or tested.
- **A new harbor E2E suite for `migrate`.** Existing suites only need to keep passing, since every suite runs init first, which upgrades the seeded fixtures.
- **Upgrading registered repos that are not on disk.** They are skipped and named, and are upgraded the next time they are set up or used with a present checkout (spec requirement).

## Changelog

### 2026-09-19 — Phase 1.1: Settings files record their format and writer versions

**What was done**: `config.yaml` and `repo.yaml` now carry a `schema` format version and a `written_by` Spektacular version, and project settings also carry `skills_version`. Both `ToYAMLFile` writers stamp the current format and `config.WriterVersion`, which `cmd/root.go` sets from the build version at startup; `skills_version` is never touched by the writers. Loading still accepts unversioned files.

**Deviations**: None.

**Files changed**:
- `spektacular: internal/config/schema.go`
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/repo.go`
- `spektacular: cmd/root.go`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/config/repo_test.go`

**Discoveries**: Six existing `repo_test.go` tests asserted the writer's exact output (top-level key counts, round-tripped struct equality, one literal layout) and had to gain `schema`/`written_by` by hand. None compared against user-authored input, so open question 1 did not require a stop. Tests that pin `written_by` use the new `pinWriterVersion(t, v)` helper in `internal/config/repo_test.go`.

### 2026-09-19 — Phase 1.2: An upgrade engine with the first registered steps

**What was done**: Added the `internal/migrate` package. It holds an ordered N→N+1 step registry per settings kind; project 1→2 does the legacy split, seeds the repo registry and carries the standalone skills-version file into `skills_version`, and repo 1→2 only stamps the version. It also holds yaml.v3 node-editing helpers and the `Inspect`/`Apply` engine. The engine previews and applies through the same code path. It writes byte-identical `<file>.v<N>.old` backups, writes each file atomically after every step, reports failures as a `StepError` naming the step and the format reached, skips absent repos, and reinstalls stale skills through an injected `Installer`. `scanProjectMetadata` moved into the package as `scanMetadata`. The dead copy in `cmd/version.go` is deleted in Phase 1.3.

**Deviations**:
- `Inspect` is `Apply` with both `DryRun` and `Skills` set, so a preview also reports whether skills would be reinstalled. A missing agent only fails a real apply, as `ErrNoAgent` inside a `StepError`.
- `SkillsReport.Installed`/`Status` describe the state before any reinstall, so the preview and apply reports compare equal.
- Stamping `skills_version` after a reinstall rewrites config.yaml without a backup, because it changes only that key and `written_by`.
- Added `config.ProjectConfigFileName`, and `config.FormatError`/`IsFormatError` in `internal/config/schema.go`.

**Files changed**:
- `spektacular: internal/migrate/registry.go`
- `spektacular: internal/migrate/node.go`
- `spektacular: internal/migrate/steps_project.go`
- `spektacular: internal/migrate/steps_repo.go`
- `spektacular: internal/migrate/scan.go`
- `spektacular: internal/migrate/errors.go`
- `spektacular: internal/migrate/engine.go`
- `spektacular: internal/migrate/helpers_test.go`
- `spektacular: internal/migrate/node_test.go`
- `spektacular: internal/migrate/registry_test.go`
- `spektacular: internal/migrate/engine_test.go`
- `spektacular: internal/migrate/testdata/**`
- `spektacular: internal/config/schema.go`
- `spektacular: internal/config/config.go`

**Discoveries**:
- A step that creates a file, such as the legacy split's repo.yaml, must be tracked by the engine (`upgrader.created`). Otherwise a dry-run reports the not-yet-created repo.yaml as a skipped repo while the apply does not, and preview and apply diverge.
- Once `schema` is inserted before it, a top-of-file comment stays attached to the following key. The fixtures place comments mid-file, so no golden depends on that placement.

### 2026-09-19 — Phase 1.3: The migrate command, an honest installation check, and self-upgrading setup

**What was done**:
- **`migrate` (new):** it runs `migrate.Apply`, reinstalling skills for the configured agent through an installer closure. It takes `--dry-run` and prints the engine `Report` as JSON.
- **`version check`:** it now sits on `migrate.Inspect` and reports `match|mismatch|missing|upgrade_needed|unsupported_format`. Every non-match action names `<command> migrate`, or updating Spektacular for a newer-format file.
- **`init`:** it applies pending format upgrades before scaffolding. After a successful install it stamps `skills_version` and removes the legacy `.spektacular/version`.
- **Skill preamble:** it moved into a shared partial.
- **Removed:** the dead migration helpers.

**Deviations**:
- The version-check preamble is a shared partial, `templates/partials/version-check.md`, included from all five SKILL.md files. The plan allowed this if a partial mechanism existed, and one did.
- The skills installer passes `io.Discard` as the install output, because the cmd wrapper contract requires stderr to stay empty.
- `noProjectError` was factored out of `loadConfig` so `migrate` can return the same `no_project` error.
- With no config.yaml, `version check` reports `missing` with an action naming `init`, not `migrate`.
- Three wrapper tests in `cmd/root_test.go` now force a `version check` failure with a `config.yaml` directory, not a `version` directory.

**Files changed**:
- `spektacular: cmd/migrate.go`
- `spektacular: cmd/version.go`
- `spektacular: cmd/init.go`
- `spektacular: cmd/root.go`
- `spektacular: internal/migrate/peek.go`
- `spektacular: templates/partials/version-check.md`
- `spektacular: templates/skills/workflows/spek-new/SKILL.md`
- `spektacular: templates/skills/workflows/spek-plan/SKILL.md`
- `spektacular: templates/skills/workflows/spek-implement/SKILL.md`
- `spektacular: templates/skills/workflows/spek-knowledge/SKILL.md`
- `spektacular: templates/skills/workflows/spek-manage-repos/SKILL.md`
- `spektacular: cmd/migrate_test.go`
- `spektacular: cmd/version_test.go`
- `spektacular: cmd/init_test.go`
- `spektacular: cmd/root_test.go`

**Discoveries**: The `gateAnnotation`/`gateExempt` constants are already declared in `cmd/migrate.go` and set on `migrate`, `init` and `version check`, ready for the Phase 2.2 gate. `config.yaml` written by a fresh init now begins with `schema`, `written_by` and `skills_version`, in struct field order.

### 2026-09-19 — Phase 2.1: Loading refuses settings in the wrong format

**What was done**:
- **Loaders:** `config.ParseYAMLFile`/`FromYAMLFile` and `config.RepoConfigFromYAMLFile` refuse any file whose normalised `schema` is not the current one, with a typed `*config.FormatError`.
- **Command-surface errors:** an outdated file maps to `config_outdated`, whose next_action names `<command> migrate` and `--dry-run`. A newer file maps to `config_newer_format`, whose next_action says to install a newer Spektacular. Both go through a shared `formatRefusal` helper.
- **Setup and registration:** `repo.EnsureFootprint` refuses a newer-format repo.yaml and upgrades an older one in place through the engine. It never overwrites either. The debug probe already tolerated load errors, so it needed no change.

**Deviations**:
- **Upgrade on setup:** `EnsureFootprint` upgrades an *outdated* repo.yaml in place through the new `migrate.UpgradeRepoFile`, where the plan said to refuse it. Otherwise a repo that isn't registered yet, which `migrate` can't reach, could never be set up. The spec says such repos upgrade when next set up.
- **Register ordering (bug fix):** `repo.Register` now refuses a newer-format target repo.yaml *before* writing config.yaml. It used to leave a registry entry pointing at an unreadable file, which the gate would turn into a project-wide block.
- **Format errors before footprint errors (bug fix):** three footprint error sites checked `FootprintError` first, so a repo.yaml in the wrong format was reported as a footprint to repair. Those sites are `cmd/knowledge.go`, `cmd/storefile.go` and `cmd/repo.go`, and they now check `formatRefusal` first.
- **Fixture sweep moved earlier:** the sweep planned for 2.2 happened here, because the loader refusal alone broke 217 tests. The shared cmd helpers `writeSpecCommandConfig`, `writeCurrentConfig` and `writeCurrentRepoConfig` now write current-format files, including `skills_version`. Two tests that asserted unversioned files load were deleted.
- **This repo's own workflow:** its config.yaml is still unversioned, so the new build refuses it. Until Phase 3.2 migrates it, the implement workflow is driven with a binary built at the end of Phase 1.3.

**Files changed**:
- `spektacular: internal/config/schema.go`
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/repo.go`
- `spektacular: internal/migrate/engine.go`
- `spektacular: internal/repo/footprint.go`
- `spektacular: internal/repo/register.go`
- `spektacular: cmd/root.go`
- `spektacular: cmd/migrate.go`
- `spektacular: cmd/knowledge.go`
- `spektacular: cmd/storefile.go`
- `spektacular: cmd/repo.go`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/config/repo_test.go`
- `spektacular: internal/config/schema_test.go`
- `spektacular: internal/repo/footprint_test.go`
- `spektacular: internal/repo/register_test.go`
- `spektacular: internal/project/init_test.go`
- `spektacular: cmd/root_test.go`
- `spektacular: cmd/spec_test.go`
- `spektacular: cmd/knowledge_test.go`
- `spektacular: cmd/changelog_file_test.go`
- `spektacular: cmd/format_refusal_test.go`

**Discoveries**:
- **Mapping order at footprint sites:** any cmd site that maps `*repo.FootprintError` to its own `output.ErrorResponse` hides the underlying cause from `toErrorResponse`. Such sites must check `formatRefusal(err)` first, or a repo.yaml in the wrong format gets repair advice.
- **Validate before writing the registry:** `repo.Register` writes config.yaml before it scaffolds the footprint, so a refusal found later leaves a registry entry behind. Validate the target before the write.
- **`project.Init` and an unversioned colocated repo.yaml:** `project.Init` refuses it rather than upgrading it, because it loads the file before `EnsureFootprint`. That is fine, because `cmd init` runs `migrate.Apply` first.

### 2026-09-19 — Phase 2.2: Commands are blocked until the project is upgraded

**What was done**: Added `cmd/gate.go`, a `PersistentPreRunE` on the root command (set in `cmd/root.go` `init()`). It calls `migrate.Inspect` whenever a project config.yaml exists and refuses the command in two cases:
- **`upgrade_required`:** any settings file is behind its format, or the skills are stale or unrecorded. The error lists each file and its format, and the next_action names `<command> migrate` and `--dry-run`.
- **`config_newer_format`:** a settings file is newer than this build supports.

Exempt commands run anyway:
- anything annotated `gate: exempt` (`migrate`, `init`, `version check`)
- `help`, `completion` and its children, and cobra's `__complete`
- the root command itself

With no project, the gate does nothing, so commands still report `no_project`.

**Deviations**:
- **Non-format failures pass through:** when `Inspect` fails for a non-format reason (for example an unparseable config.yaml), the gate lets the command run so its own loader reports the specific error. The loaders still refuse out-of-date files, so nothing runs against one.
- **Engine change:** a registered repo.yaml that can't be read or parsed is now skipped with reason `unreadable settings: …`. It no longer fails the whole upgrade, which would otherwise block every command in a project that has one broken member footprint.
- **Fixture sweep done in 2.1:** most of this phase's planned fixture sweep had already happened, because the loader refusal forced it. Only three tests changed here: they now expect `upgrade_required` where they used to expect `config_outdated`.
- **Harbor run pending:** harbor fixtures are left unversioned as planned. The harbor run is part of final verification.

**Files changed**:
- `spektacular: cmd/gate.go`
- `spektacular: cmd/root.go`
- `spektacular: internal/migrate/engine.go`
- `spektacular: internal/migrate/errors.go`
- `spektacular: cmd/gate_test.go`
- `spektacular: cmd/format_refusal_test.go`
- `spektacular: internal/migrate/engine_test.go`

**Discoveries**:
- **Inspect runs for every command:** any error it returns becomes a project-wide block unless the gate filters it. Only upgrade-relevant outcomes (pending steps, stale skills, newer format) may block; broken files must reach the command's own error.
- **`config_outdated` is unreachable from gated commands:** the gate always intercepts first. The loader refusal is now purely a backstop.
- **Bare `version` is gated:** only `version check` carries the exemption.

### 2026-09-19 — Phase 3.1: Store folders resolve from the settings file, with an upgrade step for existing projects

**What was done**:
- **Folder rule:** `spec/plan/changelog.config.directory` in config.yaml now resolve relative to the folder holding config.yaml.
- **New defaults:** `specs`, `plans` and `changelog`.
- **Loading:** `ParseYAMLFile` resolves each value once into the project-root-relative path every store consumer already uses, and remembers the value as read in an unexported `fileForm`.
- **Saving:** `ToYAMLFile` writes each folder back relative to the file's folder, reusing the value as read while it still resolves to the same place. That prevents double-nesting when a loaded config is re-saved.
- **Validation:** `Config.Validate` refuses a folder outside the project with `config_invalid` and a corrective next_action.
- **Upgrade step:** the new `project2to3` step rewrites existing projects' folder values to settings-relative equivalents, so nothing moves on disk. `config.CurrentProjectSchema` is now 3.

**Deviations**:
- **Where the outside-the-project check runs:** in project-level `Config.Validate`, not in `ChangelogConfig.Validate`. That type is shared with repo.yaml, where a repo's changelog is relative to its own folder and may legitimately point above it.
- **Absent keys:** the `project2to3` step writes them explicitly, e.g. `changelog: {config: {directory: changelog}}`, so a future change of default can never move an existing project's folders.
- **Changelog listing mismatch:** the cmd preservation test puts its changelog entry under `.spektacular/changelog/<project>/`. `artifacts list` only looks there, and `changelog file write` without `--repo` writes flat. That mismatch predates this plan and is out of scope.

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/schema.go`
- `spektacular: internal/migrate/steps_project.go`
- `spektacular: internal/migrate/registry.go`
- `spektacular: internal/project/init.go`
- `spektacular: cmd/root.go`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/config/repo_test.go`
- `spektacular: internal/config/schema_test.go`
- `spektacular: internal/config/storedir_test.go`
- `spektacular: internal/migrate/engine_test.go`
- `spektacular: internal/migrate/steps_project_test.go`
- `spektacular: internal/migrate/testdata/**`
- `spektacular: internal/project/init_test.go`
- `spektacular: cmd/migrate_test.go`
- `spektacular: cmd/version_test.go`
- `spektacular: cmd/gate_test.go`
- `spektacular: cmd/format_refusal_test.go`
- `spektacular: cmd/init_test.go`
- `spektacular: cmd/spec_test.go`
- `spektacular: cmd/plan_file_test.go`
- `spektacular: cmd/changelog_file_test.go`
- `spektacular: cmd/repo_test.go`
- `spektacular: cmd/artifacts_test.go`
- `spektacular: cmd/storefile_list_filter_test.go`
- `spektacular: cmd/storefile_metadata_test.go`
- `spektacular: cmd/file_test.go`
- `spektacular: cmd/root_test.go`

**Discoveries**:
- **Struct equality:** the unexported `fileForm` makes whole-`Config` `require.Equal` comparisons between a loaded and a hand-built Config fail. Compare the fields instead.
- **Quoting:** yaml.v3 quotes YAML-1.1 boolean-looking values such as `y`, `n`, `yes` and `no`. A directory named `y` is written as `directory: "y"`; that is valid and round-trips.
- **Registering the next format change:** adding the store-folder change took one registered step and one constant bump. The engine, gate and `version check` needed no other change, as success metric 2 requires.

### 2026-09-20 — Phase 3.2: Upgrade Spektacular's own projects and update the README

**What was done**: With the user's explicit go-ahead, ran `go run . migrate` on this repository. It took `.spektacular/config.yaml` from format 1 to 3 in one run (carrying `.spektacular/version` into `skills_version` and removing it, and rewriting the three store folders to `specs`, `plans` and `changelog`), and stamped both `.spektacular/repo.yaml` and the docs site's `repo.yaml` at format 2. Skills were already current, so nothing was reinstalled. Every listing (spec, plan, changelog, `changelog --repo docs`, artifacts) is byte-identical to a baseline captured before the gate landed. The README's configuration section gained the three version fields, the settings-relative folder rule with the new defaults, and an "Upgrading (`migrate`)" section. `*.old` was added to the scaffolded `.spektacular/.gitignore`.

**Deviations**:
- **Baseline capture:** the pre-upgrade listings were captured with a binary built at the end of Phase 1.3 (before the gate and the folder change), kept under `.spektacular/tmp/`, and compared after the upgrade. The plan suggested a git worktree of the base commit; a prebuilt binary was equivalent and cheaper.
- **Driving the workflow:** from Phase 2.1 until this upgrade, the implement workflow itself was driven with that same prebuilt binary, because the new build refuses this repo's then-unversioned settings. `go run .` works again from here on.
- **Backups:** the `.v1.old` files were deleted once the listing comparison passed, at the user's choice; git history holds the originals.
- **No settings-only commit:** the plan suggested committing the upgraded settings at this point. They are left uncommitted with the rest of the feature work instead, for one commit at the end.
- **Open question 2 answered:** re-encoding the real files preserved their content. The only formatting change is that `yaml.v3` re-indents to 4 spaces, which these files already used. No comments were lost, since none of the upgraded files carried any.

**Files changed**:
- `spektacular: .spektacular/config.yaml`
- `spektacular: .spektacular/repo.yaml`
- `spektacular: .spektacular/version` (removed)
- `spektacular: README.md`
- `spektacular: templates/.spektacular/.gitignore`
- `docs: .spektacular/repo.yaml`

**Discoveries**:
- **Upgrading a tool with itself:** once the gate and loader refusal land, the repo's own unversioned settings make every non-exempt command refuse, including the ones driving the workflow. A binary built before that point is the way through until the project is migrated.
- **The docs site has no project settings:** it is only a registered repo footprint, so its whole upgrade is the `repo.yaml` stamp, and its changelog stays reachable through repo-routed listing.

### 2026-09-20 — Phase 4.1: Configuration reference covers versions, folder rule and migrate

**What was done**: Extended the docs site's `src/pages/configuration.mdx`. The project example and key reference gained `schema`, `written_by` and `skills_version`, and the sub text now counts twelve top-level keys. The spec, plan and changelog keys show the new `specs`/`plans`/`changelog` defaults and state that each is relative to the folder holding `config.yaml`, with a note that a folder outside the project is refused. The `repo.yaml` example and key reference gained `schema: 2` and `written_by`. A new "Upgrading a project: migrate" section (`surface={false}`, alternating against the `ConfigurationKeys` block before it) covers the `--dry-run` preview, what one run does (per-format upgrades, `.v<N>.old` backups, skipped repos, skills reinstall), that out-of-date projects are blocked until upgraded and which commands still run, that re-running `init` applies the same upgrades, and that a newer-format file is refused rather than converted. Illustrative versions are `0.16.0`, the next release after the Makefile's 0.15.1.

**Deviations**:
- **No in-page anchor:** the `schema` key refers to the upgrading section as prose. This site's section headings render no `id`, so a `#…` link would have dangled.
- **Twelve keys:** the sub text previously said "Nine top-level sections". It now says twelve top-level keys and names them, which the verification counted against both the example and the key list.

**Files changed**:
- `docs: src/pages/configuration.mdx`

**Discoveries**:
- **Section headings carry no id:** `Section.astro` and `SectionHeader.astro` render no anchor target, so in-page `#…` links to a section do not work on this site. Cross-reference sections by name instead.

### 2026-09-20 — Phase 4.2: Example configuration across the site uses the new defaults

**What was done**: The getting-started tutorial's spec example now reads `directory: specs`, with a comment naming the base it resolves from. No configuration example anywhere on the site shows `directory: .spektacular/...` any more. Prose and tree diagrams naming on-disk locations were checked and left alone, because those locations have not moved.

**Deviations**:
- **`projects.mdx` left as is:** the plan allowed adding `schema: 2` to its repo.yaml example. Its examples are partial excerpts illustrating the registry and repo locations, so version lines would be noise there, and the complete examples in the configuration reference carry them.

**Files changed**:
- `docs: src/content/tutorials/getting-started.mdx`

**Discoveries**: None.
