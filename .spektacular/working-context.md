# Working context: implement 000064_epic-worktree-store-isolation

- Implementation complete (all 9 tasks, 3 milestone commits + completion commit,
  local only, never pushed). User was away ("run until done"); auto_commit: full
  commits were allowed to proceed despite the global "no commits unless asked"
  rule; flagged in the final report.
- Key deviation: worktree record maps repo -> code root (not .spektacular
  location) so nothing reads a worktree's .spektacular; constructors are
  repo.NewWithCodeRoots / autocommit.TargetsWithCodeRoots.
- Spec left one criterion unchecked ("Isolated copies' Spektacular directory is
  untouched" for a two-spec run): covered by the manual epic run in the test plan.
- `go test ./...` fails on a root-owned dir under tests/harbor/jobs; use
  `go test . ./cmd/... ./internal/... ./templates/...`.
