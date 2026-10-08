package cmd

import (
	"encoding/json"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/stretchr/testify/require"
)

// This file tests `implement merge` (cmd/implement_merge.go) end to end,
// against the same two-repo fixture as the `epic merge` tests in
// epic_worktree_test.go: the worktrees are made with `epic worktree`, work is
// committed in them, and `implement merge` brings it back.

// implementMergeCLI drives one `implement merge` invocation through the root
// command, asserting only that nothing reached stderr.
func implementMergeCLI(t *testing.T, args ...string) (stdout string, code int) {
	t.Helper()
	resetRootCmd(t)
	out, errOut, code := runRootCmd(t, append([]string{"implement", "merge"}, args...)...)
	require.Empty(t, errOut)
	return out, code
}

// refuseImplementMerge runs an `implement merge` expected to be refused and
// returns the failure envelope, asserting the refusal states its problem and
// gives a next step.
func refuseImplementMerge(t *testing.T, args ...string) output.ErrorResponse {
	t.Helper()
	stdout, code := implementMergeCLI(t, args...)
	require.Equalf(t, 1, code, "expected a refusal, got: %s", stdout)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.NotEmpty(t, er.Message, "a refusal must state the problem")
	require.NotEmpty(t, er.NextAction, "a refusal must give a runnable next step")
	return er
}

// requireWorktreesKept asserts the spec's worktrees, branches and record all
// remain after a refused merge.
func requireWorktreesKept(t *testing.T, f worktreeFixture) {
	t.Helper()
	require.DirExists(t, f.wt("testproj"))
	require.DirExists(t, f.wt("docs"))
	require.Contains(t, gittest.RunGit(t, f.proj, "worktree", "list"), f.wt("testproj"))
	require.Contains(t, gittest.RunGit(t, f.site, "worktree", "list"), f.wt("docs"))
	require.NotEmpty(t, gittest.RunGit(t, f.proj, "branch", "--list", "spek/alpha"))
	require.NotEmpty(t, gittest.RunGit(t, f.site, "branch", "--list", "spek/alpha"))
	_, ok, err := worktree.ReadRecord(f.proj, "alpha")
	require.NoError(t, err)
	require.True(t, ok, "the worktree record was removed by a refused merge")
}

// Criterion 1: a clean two-repo merge brings the work into both main lines and
// removes every worktree, branch and the record.
func TestImplementMerge_CleanTwoRepoMerge(t *testing.T) {
	f := worktreeProject(t, true)
	runEpicWorktreeCmd(t)
	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtCommitAll(t, f.wt("testproj"), "project work")
	wtWriteFile(t, f.wt("docs"), "lib.txt", "lib v2\n")
	wtCommitAll(t, f.wt("docs"), "docs work")

	stdout, code := implementMergeCLI(t, "--data", `{"name":"alpha"}`)
	require.Equalf(t, 0, code, "implement merge failed: %s", stdout)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, map[string]any{"error": false, "spec": "alpha", "merged": true, "removed": true}, got)

	require.Equal(t, "main v2", gittest.RunGit(t, f.proj, "show", "main:main.txt"))
	require.Equal(t, "lib v2", gittest.RunGit(t, f.site, "show", "main:lib.txt"))

	require.NoDirExists(t, f.wt("testproj"))
	require.NoDirExists(t, f.wt("docs"))
	require.NotContains(t, gittest.RunGit(t, f.proj, "worktree", "list"), f.wt("testproj"))
	require.NotContains(t, gittest.RunGit(t, f.site, "worktree", "list"), f.wt("docs"))
	require.Empty(t, gittest.RunGit(t, f.proj, "branch", "--list", "spek/alpha"))
	require.Empty(t, gittest.RunGit(t, f.site, "branch", "--list", "spek/alpha"))
	_, ok, err := worktree.ReadRecord(f.proj, "alpha")
	require.NoError(t, err)
	require.False(t, ok, "the worktree record outlived the merge")
}

// Criterion 2: a conflict in the sibling repo is refused naming that repo and
// path; neither main line moves and the worktrees, branches and record stay.
func TestImplementMerge_ConflictIsRefusedAndNothingMerges(t *testing.T) {
	f := worktreeProject(t, true)
	runEpicWorktreeCmd(t)
	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtCommitAll(t, f.wt("testproj"), "project work")
	wtWriteFile(t, f.wt("docs"), "lib.txt", "from the spec\n")
	wtCommitAll(t, f.wt("docs"), "docs work")
	wtWriteFile(t, f.site, "lib.txt", "from main\n")
	wtCommitAll(t, f.site, "competing work")
	projHead := gittest.RunGit(t, f.proj, "rev-parse", "HEAD")
	siteHead := gittest.RunGit(t, f.site, "rev-parse", "HEAD")

	er := refuseImplementMerge(t, "--data", `{"name":"alpha"}`)
	require.Equal(t, "epic_merge_conflict", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.Message, "docs: lib.txt")
	require.NotContains(t, er.Message, "testproj")

	require.Equal(t, projHead, gittest.RunGit(t, f.proj, "rev-parse", "HEAD"))
	require.Equal(t, siteHead, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
	require.Empty(t, gittest.RunGit(t, f.proj, "status", "--porcelain"))
	require.Empty(t, gittest.RunGit(t, f.site, "status", "--porcelain"))
	requireWorktreesKept(t, f)
}

// Criterion 3: a branch committing under .spektacular is refused exactly as
// `epic merge` refuses it.
func TestImplementMerge_SpektacularChangeIsRefused(t *testing.T) {
	f := worktreeProject(t, true)
	runEpicWorktreeCmd(t)
	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtCommitAll(t, f.wt("testproj"), "project work")
	wtWriteFile(t, f.wt("docs"), ".spektacular/knowledge/x.md", "a stray entry\n")
	wtCommitAll(t, f.wt("docs"), "docs knowledge")
	projHead := gittest.RunGit(t, f.proj, "rev-parse", "HEAD")
	siteHead := gittest.RunGit(t, f.site, "rev-parse", "HEAD")

	er := refuseImplementMerge(t, "--data", `{"name":"alpha"}`)
	require.Equal(t, "epic_merge_touches_spektacular", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.Message, "docs: .spektacular/knowledge/x.md")
	require.Contains(t, er.NextAction, "spek/alpha")

	require.Equal(t, projHead, gittest.RunGit(t, f.proj, "rev-parse", "HEAD"))
	require.Equal(t, siteHead, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
	requireWorktreesKept(t, f)
}

// Criterion 3: uncommitted work in a spec worktree is refused by the same
// precheck `epic merge` uses.
func TestImplementMerge_DirtyWorktreeIsRefused(t *testing.T) {
	f := worktreeProject(t, true)
	runEpicWorktreeCmd(t)
	wtWriteFile(t, f.wt("docs"), "lib.txt", "uncommitted\n")
	siteHead := gittest.RunGit(t, f.site, "rev-parse", "HEAD")

	er := refuseImplementMerge(t, "--data", `{"name":"alpha"}`)
	require.Equal(t, "worktree_failed", er.Code)
	require.Contains(t, er.Message, "uncommitted work")
	require.Contains(t, er.Message, f.wt("docs"))

	require.Equal(t, siteHead, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
	requireWorktreesKept(t, f)
}

// Criterion 4 and name validation: each refusal carries its code and a next
// action.
func TestImplementMerge_Refusals(t *testing.T) {
	cases := []struct {
		name string
		data string
		code string
	}{
		{"no worktrees", `{"name":"alpha"}`, "worktree_not_found"},
		{"no name", `{}`, "name_required"},
		{"empty name", `{"name":""}`, "name_required"},
		{"invalid name", `{"name":"Not Valid"}`, "name_required"},
		{"name too long", `{"name":"` + "a1234567890123456789012345678901234567890123456789012345678901234" + `"}`, "name_required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worktreeProject(t, true)
			er := refuseImplementMerge(t, "--data", tc.data)
			require.Equal(t, tc.code, er.Code, er.Message)
		})
	}
}

// With no worktrees the refusal names the spec as its resource.
func TestImplementMerge_NoWorktreesNamesTheSpec(t *testing.T) {
	worktreeProject(t, true)
	er := refuseImplementMerge(t, "--data", `{"name":"alpha"}`)
	require.Equal(t, "worktree_not_found", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Equal(t, "alpha has no worktrees to merge", er.Message)
}

func TestImplementMergeSchema_PublishesInputAndOutput(t *testing.T) {
	worktreeProject(t, true)
	stdout, code := implementMergeCLI(t, "--schema")
	require.Equal(t, 0, code, stdout)
	var got struct {
		Input struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"input"`
		Output struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"output"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []string{"name"}, got.Input.Required)
	require.Contains(t, got.Input.Properties, "name")
	keys := make([]string, 0, len(got.Output.Properties))
	for k := range got.Output.Properties {
		keys = append(keys, k)
	}
	require.ElementsMatch(t, []string{"spec", "merged", "removed"}, keys)
}
