---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# Test plan: 000059_normalise-artifact-addressing

The behavioural guarantees behind every success metric are covered by automated tests
(`cmd/storefile_address_test.go`, `cmd/storefile_location_test.go`, `cmd/status_address_test.go`,
`internal/artifact/address_test.go`, and the template and rendered-corpus guards). Two metrics are
about what happens after release and can only be checked by watching for reports.

## 1. No bug reports about a document command refusing a name a list printed

- **What to measure**: issues or support reports where `spec file`, `plan file` or `changelog file`
  refused a name that the matching `list` command printed. Threshold: zero.
- **How**: after the release that ships this change, search the issue trackers
  (hivecommons/spektacular, hivecommons/hive) for `unexpected_extension`, `document_required` and
  `not_found` reports against these commands. For each one, reproduce it on a scratch project:
  `spektacular <kind> file list [<feature>]`, then pass each printed `name` unchanged to
  `spektacular <kind> file read <name>` (`plan file read <feature> <name>` for a plan's documents),
  with and without `--repo <name>` for changelog.
- **Expected result**: no report whose reproduction shows a listed name being refused. A report
  caused by a caller still using an old spelling (`<name>.md`, `<name>/plan.md`) does not count
  against the metric, but should be answered with the migration note in `CHANGELOG.md`.
- **Who / when**: the maintainer, over the first release cycle after shipping and again before
  closing hivecommons/hive#8227.

## 2. No reports of an absolute host path or `internal_error` from the document commands

- **What to measure**: reports where `spec file`, `plan file` or `changelog file` output contains an
  absolute host path, or returns `code: internal_error`, for a correctly or incorrectly spelled
  address. Threshold: zero.
- **How**: search the issue trackers as above for `internal_error` and for paths such as `/home/`,
  `/Users/` or `C:\` in pasted output from these commands, and in `plan status` / `implement status`
  output (`plan_path` must look like `plans/<name>/plan.md`). Reproduce each on a scratch project.
- **Expected result**: none attributable to the addressing change. Known exception, pre-existing and
  out of scope: an unregistered `--repo` name on `changelog file` commands returns `internal_error`;
  track it as its own issue rather than against this metric.
- **Who / when**: the maintainer, over the first release cycle after shipping.
