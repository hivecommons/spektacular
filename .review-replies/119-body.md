Part of #114 — the acceptance criteria that need a test repository with a live hive attached (children reaching `stage/010-actionable`, claim + direct-fix kick, `Fixes #<child>` in the PR, the untrusted-reporter case) cannot be closed from this repo, so this PR does not use `Fixes`.

Stacked on #118 (Linear), which is stacked on #117 (Jira); it needs all three targets.

## What
- `tests/e2e/publish_test.go`: builds the binary (`go build`) and runs `spektacular publish` against fakes of GitHub, Jira Cloud, Jira Data Center and Linear. On every target it asserts the Spek side of `docs/publish-hive-handshake.md`: RFC title starts `RFC:`, RFC marker has no `task=` and ends the body, every child has a marker with `task=` + RFC URL, hangs off the RFC (sub-issue / Epic parent or DC Epic Link / Linear parent), clears the clarity gate (≥ 80 chars **after the marker and digest lines are stripped**, no `<placeholders>`), carries the configured label (`labels: [spek]` on every target) and none of hive's suppress labels and never the parent label, `depends_on` edges are filed as the target's blocked-by relation, the RFC checklist has a line per child, frontmatter records the publication and URLs, git provenance is recorded, a dry run makes zero calls, refusals carry `next_action`.
- Idempotence is asserted on the fake's side too: issue + relation counts are unchanged after a re-run; an edited spec is refused as `publish_conflict` until `--update`, which updates the RFC alone; a closed RFC (GitHub closed / Jira Done / Linear completed **and archived**) is refused. The Jira fake runs with a lagging search index throughout, so the re-run is shown to find its issues via the recorded URLs rather than the search.
- Production: `publish.<target>.labels` / `parent_label` naming a hive suppress label (`hold`, `needs-human`, `needs-decision`, `needs-spec`, `needs-direction`) is refused (`publish_target_unconfigured`) before any network call — `publish.PublishOptions.CheckLabels`, covered by a unit test and an e2e.
- `TestMain` stats every `.go`/`go.mod`/`go.sum` in the module so `go test ./...` cache is invalidated by any cmd change (and the `e2e` make target uses `-count=1`). git stays on PATH via a symlinked tool dir (no `gh`), and the project is a real git repo.
- GitHub fake extracted into `internal/publish/github/githubtest` (now with `Snapshot()`/`Calls()`, `assert` not `require` in handlers); `cmd/publish_test.go` uses it instead of its own fake.
- Handshake doc §6 now states what the fakes prove (that Spek sends the calls the documentation asks for) and what they cannot (a real DC build's Epic Link, Linear's HTML-comment round-trip) — those remain caveats on #117/#118.

## Tests
`make lint && make test` green locally; `make e2e` ~4s.
