---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Test plan: 000053_config-schema-versioning-and-migrations

Three of this feature's four success metrics are fully covered by automated
behavioural tests and are not repeated here. They are, with their covering
tests:

- *The next settings-format change ships as a single new upgrade step*:
  `TestRegistered_StepsAreContiguousToCurrentSchema` and
  `TestApply_SyntheticStepIsPickedUp` (`internal/migrate`).
- *Existing projects upgrade by re-running setup alone*:
  `TestInit_UpgradesUnversionedProject` (`cmd`).
- *Moving to a new release takes one command*:
  `TestMigrate_FormatBehindAndSkillsStaleReachesMatch` (`cmd`).

The automated half of the first metric (no spec, plan or changelog entry goes
missing) is covered by `TestMigrate_LegacyStoreFoldersKeepEveryArtifact` and
`TestMigrate_LegacyCustomSpecFolderMovesNothing` (`cmd`). Its manual half, the
release check against two real projects, is procedure 1 below. Procedure 2 is
the end-to-end harness run the plan's testing strategy asks for at final
verification.

## 1. No artifact goes missing when a real project is upgraded

**What to measure**: after upgrading a real, pre-feature project, every spec,
plan and changelog entry it held is still returned by the list commands, and a
newly created spec lands beside the existing ones. The threshold is exact
equality of the listings, before and after.

**Who / when**: the maintainer, once per release that raises a settings format,
before tagging. Run it against this repository and the documentation-site
project.

**Setup**: a checkout of each project at the release commit, and a build of the
*previous* release (the one the project's settings were written by), because
the new build refuses to run against settings it considers out of date:

```bash
git worktree add /tmp/spek-prev <previous-release-tag>
go build -o /tmp/spek-prev-bin /tmp/spek-prev
```

**How**:

1. From the project root, capture the listings with the previous build:

   ```bash
   /tmp/spek-prev-bin spec file list       > /tmp/before-spec.json
   /tmp/spek-prev-bin plan file list       > /tmp/before-plan.json
   /tmp/spek-prev-bin changelog file list  > /tmp/before-changelog.json
   /tmp/spek-prev-bin artifacts list       > /tmp/before-artifacts.json
   ```

   For each registered repo that routes its own changelog, also capture
   `changelog file list --repo <name>`; for this repository that is `docs`.
2. Preview the upgrade with the new build, and read the change list:

   ```bash
   go run . migrate --dry-run
   ```
3. Confirm the project is unchanged by the preview (`git status` reports no
   modification under `.spektacular/`), then apply it:

   ```bash
   go run . migrate
   ```
4. Re-run each listing from step 1 with the new build, into `/tmp/after-*.json`,
   and diff each pair.
5. Create a spec and confirm where it lands:

   ```bash
   go run . spec new --data '{"name":"upgrade-check"}'
   ```

**Expected result**:

- Every `diff /tmp/before-*.json /tmp/after-*.json` is empty, including each
  repo-routed changelog listing.
- `go run . version check` reports `status: "match"`.
- The new spec is written into the same folder as the existing ones, and
  appears in `spec file list`. Delete it afterwards.
- Each rewritten settings file has a byte-identical `.v<N>.old` copy beside it.
  Remove them once the diffs pass.

**Recorded outcome (2026-09-20, pre-release check)**: run against this
repository and, through its `docs` repo entry, the documentation site.
`config.yaml` went from format 1 to 3 and both `repo.yaml` files to format 2.
All five listings (54 specs, 52 plans, 21 changelog entries, 9 docs-repo
changelog entries, 244 artifacts) were byte-identical before and after, and
`version check` reported `match`. The documentation site has no project
settings of its own, so its check reduces to its `repo.yaml` and its
repo-routed changelog entries.

## 2. Seeded end-to-end fixtures still upgrade through setup

**What to measure**: the Harbor end-to-end suites, whose seeded project
fixtures are deliberately left at the oldest format, still run. Every suite
runs `spektacular init <agent>` first, which must upgrade the seeded settings
before anything else happens.

**Who / when**: the maintainer, at final verification before a release. It is
not part of CI, needs Docker and real agent credentials, and takes roughly 25
minutes per suite.

**How**: from the repository root,

```bash
make harbor-test-implement
```

**Expected result**: the suite passes. In particular the run does not fail with
`upgrade_required`, `config_outdated` or `config_newer_format`, which would mean
setup did not upgrade the seeded fixture before the workflow commands ran.

**Status**: not run in this implementation session. It needs Docker and live
agent credentials, so it is left for the maintainer at release time.
