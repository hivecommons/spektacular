---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Settings format versions, a migrate command, and settings-relative store folders

## What was built

Spektacular's settings files now record the format they were written in, and
there is a single command that brings a project up to date.

**Every settings file carries three versions.** A project's `config.yaml` and
each repo's `repo.yaml` record a `schema` format version and the `written_by`
Spektacular version that last saved them. Project settings also record
`skills_version`, the version that installed the agent skills, which replaces
the standalone `.spektacular/version` file. Only installing skills changes
`skills_version`, so rewriting the file for any other reason leaves it alone.
A file with no `schema` reads as the oldest format.

**A new `migrate` command upgrades a project.** It brings `config.yaml` and
every registered repo's `repo.yaml` present on disk up to the current format,
through as many formats as each file is behind, and reinstalls stale agent
skills for the project's configured agent. `migrate --dry-run` reports exactly
the same changes without touching disk, because preview and apply run the same
code. Each rewritten file gets a byte-identical `<file>.v<N>.old` backup,
files are written after every step, and a failure reports which step failed,
why, and the format the file reached. Repos that are not checked out are
skipped and named. Re-running `init` applies the same upgrades before it does
anything else.

**Out-of-date projects are stopped before they can do damage.** A check runs
before every command: when a settings file is behind the current format, or
the installed skills are stale or unrecorded, the command refuses with an error
naming `migrate`. `migrate`, `init`, `version check` and `help` always run, so
the way out is reachable. Settings written by a newer Spektacular are refused
everywhere with a message to update Spektacular, and such a file is never
rewritten. `version check` gained the matching `upgrade_needed` and
`unsupported_format` results, and the installed agent skills now tell the user
to run `migrate` on any non-match, never running it themselves.

**Upgrades ship as one registered step.** The engine holds an ordered registry
of single-format steps per settings kind, and edits files as YAML node trees so
comments and key order survive. Shipping the next format change means adding
one step and raising one constant, which a test enforces.

**The first real format change rides on that mechanism.** The spec, plan and
changelog folder settings now resolve relative to the folder holding
`config.yaml`, the same rule repo and knowledge locations already followed.
New projects write `specs`, `plans` and `changelog`, which land in the same
places on disk as before. Existing projects have their folder settings
rewritten by the upgrade, so nothing moves and no entry disappears from any
listing. A folder outside the project is refused with the corrected value to
write.

The documentation site's configuration reference now covers the three version
fields, the folder rule with its new defaults, and upgrading with `migrate`.

## Why it matters

Before this, no settings file recorded its format, so the tool could not safely
change how a setting is read. The immediate case was making the store folders
resolve relative to the settings file: without a version to go on, that change
would have silently pointed existing projects at empty folders, hiding every
spec, plan and changelog entry they held.

Maintainers now have a safe place to ship future format changes: one registered
step, with loading and the installation check untouched. Users with existing
projects upgrade in one command, or simply by re-running setup, and can preview
exactly what will change first. Nothing they have already written goes missing,
and a project written by a newer release fails loudly instead of being
misread.

## Deviations from the plan

- **Setup upgrades an outdated repo file instead of refusing it.** The plan had
  repo setup refuse any repo.yaml in the wrong format. That would have stranded
  a repo not yet in the registry, which `migrate` cannot reach, so setup now
  upgrades an older-format repo.yaml in place, with a backup. A newer-format
  file is still refused and never overwritten.
- **Two bugs found and fixed while testing.** Registration wrote the new
  registry entry to `config.yaml` before checking the target repo's settings,
  so a refused registration left an entry pointing at an unreadable file, which
  the new gate would turn into a project-wide block. And three commands
  reported a repo.yaml in the wrong format as a broken footprint, advising
  `repo add` instead of `migrate`.
- **A broken settings file is not treated as an upgrade problem.** A registered
  repo whose repo.yaml cannot be parsed is skipped and named rather than
  failing the whole run, and when the pre-command check fails for any reason
  other than a format mismatch, the command runs so its own error reaches the
  user.
- **The fixture sweep landed a phase early.** Refusing to load out-of-date
  files broke 217 existing tests that wrote unversioned settings, so the sweep
  planned for the blocking phase happened with the refusal instead.
- **Upgrading the tool with itself needed a bridge.** Once the gate landed,
  this repository's own unversioned settings made every non-exempt command
  refuse, including the ones driving the work. A binary built before that point
  drove the workflow until the repository was migrated.
- **The version-check preamble became a shared partial** across the five
  workflow skills, rather than the same text repeated in each.
- **Smaller notes.** The store-folder escape check lives in project-level
  validation, because the changelog settings type is shared with `repo.yaml`,
  where a folder above the settings file is legitimate. The upgrade backups of
  this repository's own settings were deleted after the listings were confirmed
  identical, and `*.old` was added to the scaffolded project ignore file. The
  documentation cross-references its new upgrading section by name, because
  section headings on that site render no anchor target.
