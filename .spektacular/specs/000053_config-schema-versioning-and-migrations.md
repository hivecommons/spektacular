---
created_date: "2026-09-19"
document_status: final
closed_date: "2026-09-19"
---

# Feature: 000053_config-schema-versioning-and-migrations

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Spektacular's settings files carry no record of which format they are written in, so changing how a setting is read — such as making the spec, plan and changelog folders resolve relative to the settings file like every other path — would silently point existing projects at the wrong folders and hide their existing work. This feature stamps every settings file with its format version and the Spektacular version behind it, and adds a single upgrade command that brings an out-of-date project's settings and installed skills current before anything else runs. Maintainers gain a safe place to ship future format changes, and users with existing projects upgrade in one step without losing or splitting their work.

<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [x] **Settings files record their format version**
  The system must record, inside every project settings file and every repo settings file it writes, a format version: a whole number that increases only when that file's format changes.
- [x] **Settings files record the writing Spektacular version**
  The system must record, inside every settings file it writes, the version of Spektacular that last wrote it, for diagnosis.
- [x] **Project settings record the installed skills version**
  The system must record, in the project settings file, the Spektacular version that last installed the project's agent skills, and must update that value only when skills are installed — never merely because the settings file was rewritten for another reason.
- [x] **The installed skills version replaces the standalone version file**
  Older projects keep their installed skills version in a standalone file outside the settings file. The project settings file must become the single place this value is kept; a project that only has the standalone file must still be recognised, and upgrading it must carry that value into the settings and remove the standalone file.
- [x] **Unversioned files are treated as the oldest format**
  The system must treat a settings file that carries no format version as the oldest known format, so every project created before this feature is recognised as needing upgrade.
- [x] **Out-of-date projects are blocked until upgraded**
  When any settings file is behind the format the running Spektacular expects, or the installed skills version differs from the running Spektacular, every command other than the upgrade command, setup, the installation check and help must refuse to run, with an error naming the upgrade command.
- [x] **The installation check reports out-of-date projects**
  The installation check must report when any settings file is behind the expected format or the installed skills version differs from the running Spektacular, with an action telling the user to run the upgrade command.
- [x] **Users can upgrade a project on demand**
  Users can run a single upgrade command that brings the project's settings, and the settings of every registered repo present on disk, up to the format the running Spektacular expects.
- [x] **Users can preview an upgrade**
  Users can ask the upgrade command to report every settings change, every file move or removal, and whether skills would be reinstalled, without changing anything on disk.
- [x] **Setup upgrades automatically**
  The system must apply any pending upgrades when a user re-runs project setup, before setup does anything else with those settings.
- [x] **Upgrading refreshes stale skills**
  When the installed skills version differs from the running Spektacular, the upgrade command must reinstall the project's agent skills for the agent the project is configured for, after any pending format changes, and record the new installed version.
- [x] **Files any number of formats behind upgrade in one run**
  A settings file any number of formats behind must be brought to the current format by a single upgrade run, with its new format version recorded.
- [x] **Upgrades are safe to re-run**
  Running the upgrade on a project that is already current must change nothing and report that nothing was needed.
- [x] **Originals are kept before a settings file is rewritten**
  The system must keep a recoverable copy of each settings file's previous contents before an upgrade rewrites it, and report where that copy is.
- [x] **A failed upgrade reports where it stopped**
  If an upgrade fails partway, the system must stop, leave the file at the last format it successfully reached, and report what failed and why.
- [x] **Files from a newer Spektacular are refused**
  The system must refuse to read or write a settings file whose format version is newer than the running Spektacular understands, and must tell the user to update Spektacular.
- [x] **Repos not on disk are skipped with a notice**
  The upgrade must skip registered repos that are not present locally and report each one it skipped; each is upgraded when it is next set up or used.
- [x] **Old single-file projects upgrade through the same flow**
  Projects still using the old single settings file (with no separate repo settings) must be upgraded by the same upgrade command and setup flow as any other out-of-date project.
- [x] **Store folders resolve from the settings file**
  The system must resolve the spec, plan and changelog folder settings relative to the folder holding the settings file that declares them, the same rule that already applies to repo and knowledge locations.
- [x] **New projects use settings-relative default folders**
  New projects must default the spec, plan and changelog folders to values written relative to the settings file, so they land in the same places on disk as today.
- [x] **Existing projects keep their specs, plans and changelogs**
  Upgrading an existing project must rewrite its spec, plan and changelog folder settings so they point at the same folders on disk as before; no existing spec, plan or changelog entry may disappear from listings after the upgrade.
- [x] **Public docs describe the new behaviour**
  The documentation site must describe the format-version, writing-version and installed-skills-version fields, the settings-relative path rule and new defaults, the upgrade command including its preview mode, and that out-of-date projects are blocked until upgraded.

<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- The version record must live inside the settings files themselves, not in a separate file alongside them — user decision: "put the version of the binary that created the config" in the config.
- Format upgrades must be keyed on a whole-number format version, never on the Spektacular binary version; a binary version may only decide whether skills are reinstalled — user decisions during the interview and when folding the skills refresh into the upgrade.
- Logic that rewrites or upgrades settings and on-disk layout must live in the upgrade tool, not in settings loading; loading may only read the current format and detect that a file is out of date — user decision: "we also need a migrations tool where we can put logic like changing the directories".
- An out-of-date settings format or stale installed skills must block normal commands until the upgrade is run — user decision: "stale skills and config should force an upgrade".
- Must not break the installation check's existing results that installed skills already act on (`match`, `mismatch`, `missing`); new outcomes may be added alongside them, any legacy-layout outcome may be folded into the general upgrade-needed outcome, and the suggested action now points at the upgrade command — existing system: every installed skill runs this check first and branches on those values.
- Agent skills must never apply an upgrade themselves; applying one is always an explicit user action (re-running setup or running the upgrade command) — existing policy stated in every installed skill's version-check preamble.

<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [x] **Fresh setup stamps the format version**
  After setting up a new project, the project settings file and the repo settings file each contain a format version, and the installation check reports a match.
- [x] **Fresh setup stamps the writing and installed skills versions**
  After setting up a new project with a build reporting version V, the settings contain V as both the writing version and the installed skills version.
- [x] **Writing version alone never triggers an upgrade**
  A current-format project whose installed skills version matches the running build, but whose settings name a different writing version, passes the installation check, other commands run normally, and the upgrade command reports nothing to do.
- [x] **Stale skills block commands until upgraded**
  Given a current-format project whose installed skills version differs from the running build, a spec command exits with an error naming the upgrade command; after running the upgrade command, the skills are reinstalled for the configured agent, the installed skills version equals the running build, no other settings value changes, and the spec command runs.
- [x] **Outdated format blocks commands until upgraded**
  Given a project whose settings carry no format version, a spec command exits with an error naming the upgrade command and creates no files; after running the upgrade command, the same spec command runs.
- [x] **Upgrade path stays reachable while blocked**
  On an out-of-date project, the upgrade command, setup, the installation check and help all run without being blocked.
- [x] **A failed skills reinstall does not mark skills as fresh**
  Given a project that is behind on format and has stale skills, if the upgrade rewrites the settings but the skills reinstall then fails, the installed skills version is unchanged and the installation check still reports that an upgrade is needed.
- [x] **Older projects' standalone version file is carried over**
  Given a project with the standalone skills-version file and no installed skills version in its settings, the installation check compares against the standalone file; after the upgrade, the settings hold the installed skills version and the standalone file no longer exists.
- [x] **Unversioned project is detected as outdated**
  Given a project whose settings files carry no format version, the installation check reports that an upgrade is needed and includes an action naming the upgrade command.
- [x] **Upgrade command brings every present settings file current**
  Given an outdated project with two registered repos present on disk, running the upgrade command leaves the project settings file and both repo settings files at the same format version as a freshly set-up project.
- [x] **Preview changes nothing on disk**
  Running the upgrade in preview mode on an outdated project lists each settings change, each file move or removal, and whether skills would be reinstalled, and afterwards every file and folder in the project is byte-for-byte and path-for-path identical to before.
- [x] **Preview and apply agree**
  The set of changes reported by preview on an outdated project matches the changes actually made by the subsequent apply on the same project.
- [x] **One command brings settings and skills current**
  Given a project that is both behind on format and has stale skills, a single run of the upgrade command leaves the installation check reporting a match.
- [x] **Setup upgrades an outdated project**
  Re-running setup on an unversioned project leaves its settings files at the current format version, with no separate upgrade command run.
- [x] **Multi-format upgrade preserves effective settings**
  Given a settings file two or more formats behind, one upgrade run leaves it at the current format version, and its effective settings (folder locations on disk, registered repos, configured agent) are the same as before the upgrade.
- [x] **Re-running on a current project is a no-op**
  Running the upgrade command on a project already at the current format with current skills reports that nothing was needed and leaves every file unchanged.
- [x] **Previous settings are recoverable**
  After an upgrade rewrites a settings file, the upgrade output names the location of a copy of that file's pre-upgrade contents, and that copy is byte-for-byte identical to the original.
- [x] **A failing upgrade stops and reports**
  When an upgrade fails partway (for example, a folder it must write cannot be written), the command exits with an error naming what failed and why, and the file's recorded format version is the last one successfully reached.
- [x] **Newer-format files are refused**
  Given a settings file whose format version is higher than the running Spektacular supports, every command that loads it exits with an error telling the user to update Spektacular, and the file is unchanged.
- [x] **Absent repos are skipped and named**
  Given a registered repo whose location is not on disk, the upgrade completes for everything else and its output names that repo as skipped.
- [x] **Old single-file projects upgrade through the same command**
  Given a project with only an old single settings file and no repo settings file, the installation check reports the same upgrade-needed result as any other outdated project, and the upgrade command produces both project and repo settings files at the current format.
- [x] **Store folders resolve from the settings file**
  In an upgraded project, setting the spec folder to a relative value `x` causes a newly created spec to be written under `x` inside the folder that holds the project settings file, not under `x` at the project root; the same holds for plans and changelogs.
- [x] **New projects keep today's on-disk layout**
  A newly set-up project stores specs, plans and changelogs in the same on-disk folders as a project set up before this feature, and its settings express those folders without repeating the settings folder's own name.
- [x] **Existing specs, plans and changelogs survive the upgrade**
  Given an existing project containing specs, plans and changelog entries, after the upgrade every one of them is still returned by the corresponding list commands, and a newly created spec is written to the same folder as the existing ones.
- [x] **Docs cover the new behaviour**
  The documentation site's configuration reference shows the format-version, writing-version and installed-skills-version fields, states the settings-relative path rule with the new default folder values, documents the upgrade command and its preview mode, and states that out-of-date projects are blocked until upgraded.

<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- Record the three versions as top-level fields (working names `schema` and `written_by` in both settings files, `skills_version` in the project settings only); the planner may choose final names. `skills_version` is kept distinct from `written_by` because other commands rewrite the settings without reinstalling skills.
- Model upgrades as an ordered registry of single-format steps (N → N+1), each owning its own detection and rewrite/move logic, so a future format change is added by registering one more step.
- Bump the format version only for changes that make older files read wrongly; backward-compatible additions (an optional field with a sensible default) should not bump it.
- Have the upgrade command, setup, the installation check and the command gate share one engine: the check and gate ask it what is pending, the command and setup ask it to apply.
- Treat the existing legacy single-file → project/repo split as the first registered step, replacing the separate detection path the installation check uses today.
- Resolve the spec, plan and changelog folder settings once, when settings are loaded, into the project-rooted paths the rest of the tool already uses, so existing call sites need not change.
- Reuse the existing setup install path for the skills reinstall, for the agent recorded in the settings.
- Keep the previous contents of a rewritten settings file as a sibling backup (as the existing legacy migration does with a `.old` copy).

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Zero existing specs, plans or changelog entries go missing from listings after upgrading a project created before this feature (checked against this project and the documentation-site project before release).
- The next settings-format change after this one ships as a single new upgrade step, with no change to config loading or to the installation check.
- Existing projects upgrade by re-running setup alone, without users hand-editing settings files.
- Moving to a new Spektacular release takes one command to bring both settings and installed skills current.

<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- Removing the knowledge folder that some older projects have at the project root because of a past setup defect is out of scope; the defect itself is fixed separately.
- Downgrading settings to an older format is out of scope; only forward upgrades are supported.
- Rolling back a completed upgrade automatically is out of scope; recovery is from the kept copy of the previous settings (or version control) by hand.
- Changing any other settings value or on-disk layout beyond the spec, plan and changelog folder paths is out of scope for this feature's first format upgrade.
