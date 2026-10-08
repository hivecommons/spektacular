# Working context: implement 000066_epic-mid-run-revisions

## Decisions
- User chose "run without asking": loop through tasks without pausing.
- User chose to implement 000066_epic-mid-run-revisions (interactive run, full plan).
- Code lives in worktrees, not the plan's listed roots:
  - spektacular: .spektacular/worktrees/000066_epic-mid-run-revisions/spektacular
  - docs: .spektacular/worktrees/000066_epic-mid-run-revisions/docs
- read_plan: structure valid, no drift (only line numbers shifted), full spec coverage, first-task changelog mode.

## Learnings
- `laneProject` is defined in cmd/workflow_lane_test.go; cmd/implement_lane_test.go wraps it as `implementLaneProject`.
- The stored spec for 000066 itself has a stray second frontmatter-like block ("--" line) at the top of its body; the section splitter must treat pre-`## ` preamble as a non-amendable section.
- Use `grep --include='*.go'` (quoted) in zsh.
- Fixed (user asked for a fix, not a knowledge entry): 13 cmd tests now `t.Chdir(t.TempDir())`; `go test -shuffle=on ./...` runs green directly in the worktree. New cmd tests must chdir to a temp dir.
- Task 1 (Record amendments in spec metadata) verified green.
- Amendment frontmatter uses yaml flow style for `sections` and `design`, matching the plan's on-disk example.
- Task 2 (staleness exemption) verified green.
- spec amend: `cmd/spec_amend.go` + pure helpers in `cmd/spec_amend_section.go`. Section compare trims surrounding whitespace and normalises checkboxes; the preamble is heading "". A staged body missing the existing `## Amendments` section is refused as rewriting history, so agents must stage from `spec file read` output. Design "unresolved" uses `design.Set.Exists` (Resolve does not check existence).
- Throwaway-project smoke test: build `spektacular` into the scratchpad and `init claude` in a scratch git repo.
