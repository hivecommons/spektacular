package plantask

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// filesPlan is a task plan whose tasks live in different repos. The expected
// values in the tests below are written out by hand from filesPlan and
// filesContext, never derived from the reader.
const filesPlan = `# Plan: files

## Milestones & Tasks

### Milestone 1: Files are found

#### - [ ] Task: Add the file reader
**Id:** 1a2b3c4d-0000-4000-8000-000000000001
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

**Acceptance criteria**:
- [ ] reads files

#### - [ ] Task: Document the file reader
**Id:** 1a2b3c4d-0000-4000-8000-000000000002
**Repo:** docs
**Depends on:**
- 1a2b3c4d-0000-4000-8000-000000000001 — Add the file reader
**Execution:** agent

**Acceptance criteria**:
- [ ] documents files

#### - [ ] Task: A task without technical notes
**Id:** 1a2b3c4d-0000-4000-8000-000000000003
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

**Acceptance criteria**:
- [ ] nothing
`

const filesContext = "# Context\n" +
	"\n" +
	"## Per-Task Technical Notes\n" +
	"\n" +
	"### Task: Add the file reader\n" +
	"\n" +
	"- `internal/plantask/files.go:12` — add the reader\n" +
	"- `internal/depgraph/depgraph.go:10-20` — add Reaches\n" +
	"- `docs:guide/files.md` — mention it\n" +
	"- `vendor:lib/x.go` — an unregistered prefix\n" +
	"- `README.md` — a bare file name\n" +
	"- `internal/plantask/files.go:40,55` — the same file again\n" +
	"- run `go test` and use `epic.Member`; the `internal/plantask/` package\n" +
	"\n" +
	"```go\n" +
	"// `internal/fenced/example.go`\n" +
	"### Task: A task without technical notes\n" +
	"```\n" +
	"\n" +
	"### Task: Document the file reader\n" +
	"\n" +
	"- `site/files.md:3` — the docs page\n" +
	"- `spektacular:cmd/root.go` — a link from the CLI\n" +
	"\n" +
	"## Testing Approach\n" +
	"\n" +
	"- `internal/plantask/files_test.go` — outside every task section\n"

func TestTaskFiles_ReadsEachTasksFiles(t *testing.T) {
	files := TaskFiles([]byte(filesPlan), []byte(filesContext), []string{"spektacular", "docs"})

	require.Equal(t, map[string][]FileRef{
		"Add the file reader": {
			{Repo: "spektacular", Path: "internal/plantask/files.go"},
			{Repo: "spektacular", Path: "internal/depgraph/depgraph.go"},
			{Repo: "docs", Path: "guide/files.md"},
			{Repo: "spektacular", Path: "vendor:lib/x.go"},
			{Repo: "spektacular", Path: "README.md"},
		},
		"Document the file reader": {
			{Repo: "docs", Path: "site/files.md"},
			{Repo: "spektacular", Path: "cmd/root.go"},
		},
	}, files)
}

func TestTaskFiles_LineSuffixesAreStripped(t *testing.T) {
	context := "### Task: Add the file reader\n" +
		"- `a/one.go:7`\n" +
		"- `a/two.go:10-20`\n" +
		"- `a/three.go:4,9`\n" +
		"- `docs:b/four.md:1-2`\n"
	files := TaskFiles([]byte(filesPlan), []byte(context), []string{"spektacular", "docs"})
	require.Equal(t, []FileRef{
		{Repo: "spektacular", Path: "a/one.go"},
		{Repo: "spektacular", Path: "a/two.go"},
		{Repo: "spektacular", Path: "a/three.go"},
		{Repo: "docs", Path: "b/four.md"},
	}, files["Add the file reader"])
}

func TestTaskFiles_NonPathsAreIgnored(t *testing.T) {
	context := "### Task: Add the file reader\n" +
		"Run `go test`, call `epic.Member`, see the `internal/plantask/` directory\n" +
		"and the `Parse` function.\n"
	files := TaskFiles([]byte(filesPlan), []byte(context), []string{"spektacular", "docs"})
	require.Empty(t, files)
}

func TestTaskFiles_FencedCodeIsIgnored(t *testing.T) {
	context := "### Task: Add the file reader\n" +
		"```\n" +
		"`internal/fenced/example.go`\n" +
		"```\n" +
		"- `internal/real.go`\n"
	files := TaskFiles([]byte(filesPlan), []byte(context), []string{"spektacular"})
	require.Equal(t, map[string][]FileRef{
		"Add the file reader": {{Repo: "spektacular", Path: "internal/real.go"}},
	}, files)
}

func TestTaskFiles_DeduplicatesInFirstSeenOrder(t *testing.T) {
	context := "### Task: Add the file reader\n" +
		"- `b/second.go:1`\n" +
		"- `a/first.go`\n" +
		"- `b/second.go:30-40`\n" +
		"- `spektacular:a/first.go`\n"
	files := TaskFiles([]byte(filesPlan), []byte(context), []string{"spektacular", "docs"})
	require.Equal(t, []FileRef{
		{Repo: "spektacular", Path: "b/second.go"},
		{Repo: "spektacular", Path: "a/first.go"},
	}, files["Add the file reader"])
}

func TestTaskFiles_MissingContextYieldsNothing(t *testing.T) {
	require.Empty(t, TaskFiles([]byte(filesPlan), nil, []string{"spektacular", "docs"}))
	require.Empty(t, TaskFiles([]byte(filesPlan), []byte{}, nil))
	require.Empty(t, TaskFiles(nil, nil, nil))
	require.Empty(t, TaskFiles(nil, []byte(filesContext), []string{"spektacular"}),
		"a context without plan tasks attributes no files")
}

func TestTaskFiles_TaskWithoutSectionIsAbsent(t *testing.T) {
	files := TaskFiles([]byte(filesPlan), []byte(filesContext), []string{"spektacular", "docs"})
	_, ok := files["A task without technical notes"]
	require.False(t, ok, "a heading inside fenced code does not open a section")
}

func TestTaskFiles_SingleRepoFallback(t *testing.T) {
	const plan = `# Plan: no repo

## Milestones & Tasks

### Milestone 1: One

#### - [ ] Task: A task without a repo
**Id:** 2b3c4d5e-0000-4000-8000-000000000001
**Depends on:** none
**Execution:** agent

**Acceptance criteria**:
- [ ] done
`
	context := "### Task: A task without a repo\n" +
		"- `internal/x.go:3`\n"

	require.Equal(t, map[string][]FileRef{
		"A task without a repo": {{Repo: "solo", Path: "internal/x.go"}},
	}, TaskFiles([]byte(plan), []byte(context), []string{"solo"}))

	require.Equal(t, map[string][]FileRef{
		"A task without a repo": {{Repo: "", Path: "internal/x.go"}},
	}, TaskFiles([]byte(plan), []byte(context), []string{"one", "two"}),
		"with several repos and no task repo the file has no repo")
}
