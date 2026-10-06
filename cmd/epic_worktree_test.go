package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/stretchr/testify/require"
)

// This file tests `epic worktree` and `epic merge` (cmd/epic_worktree.go)
// end to end against two real git repositories: the project, which registers
// itself as "testproj", and a sibling at <base>/website registered as "docs"
// with a location relative to the project's .spektacular folder — the shape
// of a project with a separate docs site. The sibling's folder name differs
// from its registry name on purpose, so a location that missed the overlay
// would resolve to a folder that does not exist instead of coincidentally to
// the spec's docs worktree.

const worktreeTestSpec = "alpha"

// worktreeTestPlan has one task in each repo.
const worktreeTestPlan = `# Plan: alpha

## Overview

fixture

## Milestones & Tasks

### Milestone 1: Both repos

#### - [ ] Task: Change the project
**Id:** 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95
**Repo:** testproj
**Depends on:** none
**Execution:** agent

#### - [ ] Task: Change the docs
**Id:** 7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34
**Repo:** docs
**Depends on:** none
**Execution:** agent
`

// epicWorktreeResult mirrors the `epic worktree` JSON envelope.
type epicWorktreeResult struct {
	Spec    string `json:"spec"`
	Project string `json:"project"`
	Branch  string `json:"branch"`
	Repos   []struct {
		Repo string `json:"repo"`
		Path string `json:"path"`
	} `json:"repos"`
	Created bool `json:"created"`
}

type worktreeFixture struct {
	proj, site string
}

func (f worktreeFixture) wt(dir string) string {
	return filepath.Join(f.proj, ".spektacular", "worktrees", worktreeTestSpec, dir)
}

func wtWriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func wtCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	gittest.RunGit(t, dir, "add", "-A")
	gittest.RunGit(t, dir, "commit", "-q", "-m", msg)
}

// worktreeProject builds both repos, commits them on main, and chdirs into
// the project. withPlan false leaves the spec unplanned.
func worktreeProject(t *testing.T, withPlan bool) worktreeFixture {
	t.Helper()
	gittest.RequireGit(t)
	pinGitIdentity(t)
	base := tempWorkTree(t)
	f := worktreeFixture{proj: filepath.Join(base, "proj"), site: filepath.Join(base, "website")}

	require.NoError(t, os.MkdirAll(f.site, 0o755))
	gittest.RunGit(t, f.site, "init", "-q", "-b", "main")
	wtWriteFile(t, f.site, "lib.txt", "lib v1\n")
	rc := config.NewDefaultRepoConfig()
	rc.Source = config.DefaultRepoSource
	require.NoError(t, os.MkdirAll(filepath.Join(f.site, ".spektacular"), 0o755))
	require.NoError(t, rc.ToYAMLFile(filepath.Join(f.site, ".spektacular", config.RepoConfigFileName)))
	wtCommitAll(t, f.site, "initial site")

	require.NoError(t, os.MkdirAll(f.proj, 0o755))
	gittest.RunGit(t, f.proj, "init", "-q", "-b", "main")
	writeSpecCommandConfig(t, f.proj, "repos:\n  - name: testproj\n    location: .\n  - name: docs\n    location: ../../website/.spektacular\n")
	wtWriteFile(t, f.proj, "main.txt", "main v1\n")
	wtWriteFile(t, f.proj, ".spektacular/specs/alpha.md", epicTestSpecFixed)
	if withPlan {
		wtWriteFile(t, f.proj, ".spektacular/plans/alpha/plan.md", worktreeTestPlan)
	}
	wtCommitAll(t, f.proj, "initial project")
	t.Chdir(f.proj)
	return f
}

func runEpicWorktreeCmd(t *testing.T) epicWorktreeResult {
	t.Helper()
	stdout, code := runEpic(t, "worktree", "--data", `{"spec":"alpha"}`)
	require.Equalf(t, 0, code, "epic worktree failed: %s", stdout)
	var got epicWorktreeResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// Criterion 1, through the command: both repos get a worktree, and asking
// again reports the same worktrees with created false.
func TestEpicWorktree_CreatesThenReturnsTheSameWorktrees(t *testing.T) {
	f := worktreeProject(t, true)

	first := runEpicWorktreeCmd(t)
	require.Equal(t, "alpha", first.Spec)
	require.Equal(t, "spek/alpha", first.Branch)
	require.Equal(t, f.wt("testproj"), first.Project)
	require.True(t, first.Created)
	require.Len(t, first.Repos, 2)
	require.Equal(t, "testproj", first.Repos[0].Repo)
	require.Equal(t, f.wt("testproj"), first.Repos[0].Path)
	require.Equal(t, "docs", first.Repos[1].Repo)
	require.Equal(t, f.wt("docs"), first.Repos[1].Path)
	require.Equal(t, "spek/alpha", gittest.RunGit(t, f.wt("docs"), "rev-parse", "--abbrev-ref", "HEAD"))

	// The main project's record names each repo's code root at the paths
	// the command reported.
	rec, ok, err := worktree.ReadRecord(f.proj, "alpha")
	require.NoError(t, err)
	require.True(t, ok)
	reported := map[string]string{}
	for _, r := range first.Repos {
		reported[r.Repo] = r.Path
	}
	require.Equal(t, reported, rec.Repos)

	second := runEpicWorktreeCmd(t)
	require.False(t, second.Created)
	first.Created = false
	require.Equal(t, first, second)

	// Criterion 3: the main copy sees nothing of the worktrees.
	require.Empty(t, gittest.RunGit(t, f.proj, "status", "--porcelain", "--untracked-files=all"))
}

// Criterion 2: run from inside the spec's project worktree, `repo list`
// reports every touched repo's code in the spec's worktrees, and each repo's
// knowledge store resolves there too.
func TestEpicWorktree_ReposResolveIntoWorktreesFromInside(t *testing.T) {
	f := worktreeProject(t, true)
	res := runEpicWorktreeCmd(t)
	t.Chdir(res.Project)

	resetRootCmd(t)
	out, errOut, code := runRootCmd(t, "repo", "list")
	require.Empty(t, errOut)
	require.Equalf(t, 0, code, "repo list failed: %s", out)
	var listed struct {
		Repos []struct {
			Name string `json:"name"`
			Root string `json:"root"`
		} `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &listed))
	roots := map[string]string{}
	for _, r := range listed.Repos {
		roots[r.Name] = r.Root
	}
	require.Equal(t, map[string]string{"testproj": f.wt("testproj"), "docs": f.wt("docs")}, roots)

	cfg, err := loadConfig()
	require.NoError(t, err)
	sources, err := aggregateKnowledgeSources(cfg, res.Project)
	require.NoError(t, err)
	locations := map[string]string{}
	for _, s := range sources {
		locations[s.Name] = s.Config.Location
	}
	require.Equal(t, filepath.Join(f.wt("docs"), ".spektacular", "knowledge"), locations["docs"])
	require.Equal(t, filepath.Join(f.wt("testproj"), ".spektacular", "knowledge"), locations["testproj"])
}

// Criterion 4, through the command.
func TestEpicMerge_CleanMergeReportsMergedAndRemoved(t *testing.T) {
	f := worktreeProject(t, true)
	runEpicWorktreeCmd(t)
	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtCommitAll(t, f.wt("testproj"), "project work")
	wtWriteFile(t, f.wt("docs"), "lib.txt", "lib v2\n")
	wtCommitAll(t, f.wt("docs"), "docs work")

	stdout, code := runEpic(t, "merge", "--data", `{"spec":"alpha"}`)
	require.Equalf(t, 0, code, "epic merge failed: %s", stdout)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, map[string]any{"error": false, "spec": "alpha", "merged": true, "removed": true}, got)

	main, err := os.ReadFile(filepath.Join(f.proj, "main.txt"))
	require.NoError(t, err)
	require.Equal(t, "main v2\n", string(main))
	lib, err := os.ReadFile(filepath.Join(f.site, "lib.txt"))
	require.NoError(t, err)
	require.Equal(t, "lib v2\n", string(lib))
	require.NoDirExists(t, f.wt("testproj"))
	require.NoDirExists(t, f.wt("docs"))
	require.Empty(t, gittest.RunGit(t, f.site, "branch", "--list", "spek/alpha"))
	_, ok, err := worktree.ReadRecord(f.proj, "alpha")
	require.NoError(t, err)
	require.False(t, ok, "the worktree record outlived the merge")
}

// Criterion 5, through the command: a conflict in the sibling alone is
// refused as epic_merge_conflict naming the path, and neither repo moves.
func TestEpicMerge_ConflictIsRefusedAndNothingMerges(t *testing.T) {
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

	er := refuseEpic(t, "merge", "--data", `{"spec":"alpha"}`)
	require.Equal(t, "epic_merge_conflict", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.Message, "docs: lib.txt")
	require.NotContains(t, er.Message, "testproj")
	require.Contains(t, er.NextAction, "stop")

	require.Equal(t, projHead, gittest.RunGit(t, f.proj, "rev-parse", "HEAD"))
	require.Equal(t, siteHead, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
	require.Empty(t, gittest.RunGit(t, f.proj, "status", "--porcelain"))
	require.Empty(t, gittest.RunGit(t, f.site, "status", "--porcelain"))
	require.DirExists(t, f.wt("docs"))
}

// Through the command: a spec branch that writes under the sibling's
// .spektacular directory is refused as epic_merge_touches_spektacular naming
// the path, and neither repo moves.
func TestEpicMerge_SpektacularChangeIsRefused(t *testing.T) {
	f := worktreeProject(t, true)
	runEpicWorktreeCmd(t)
	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtCommitAll(t, f.wt("testproj"), "project work")
	wtWriteFile(t, f.wt("docs"), ".spektacular/knowledge/x.md", "a stray entry\n")
	wtCommitAll(t, f.wt("docs"), "docs knowledge")
	projHead := gittest.RunGit(t, f.proj, "rev-parse", "HEAD")
	siteHead := gittest.RunGit(t, f.site, "rev-parse", "HEAD")

	er := refuseEpic(t, "merge", "--data", `{"spec":"alpha"}`)
	require.Equal(t, "epic_merge_touches_spektacular", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.Message, "docs: .spektacular/knowledge/x.md")
	require.Contains(t, er.NextAction, "spek/alpha")
	require.Contains(t, er.NextAction, f.wt("docs"))

	require.Equal(t, projHead, gittest.RunGit(t, f.proj, "rev-parse", "HEAD"))
	require.Equal(t, siteHead, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
	require.DirExists(t, f.wt("testproj"))
	require.DirExists(t, f.wt("docs"))
}

func TestEpicWorktree_Refusals(t *testing.T) {
	cases := []struct {
		name     string
		withPlan bool
		args     []string
		code     string
	}{
		{"no spec", true, []string{"worktree", "--data", `{}`}, "spec_required"},
		{"bad spec name", true, []string{"worktree", "--data", `{"spec":"Not Valid"}`}, "spec_required"},
		{"unknown spec", true, []string{"worktree", "--data", `{"spec":"nonesuch"}`}, "spec_not_found"},
		{"unplanned spec", false, []string{"worktree", "--data", `{"spec":"alpha"}`}, "plan_not_found"},
		{"merge without spec", true, []string{"merge", "--data", `{}`}, "spec_required"},
		{"merge without worktrees", true, []string{"merge", "--data", `{"spec":"alpha"}`}, "worktree_not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worktreeProject(t, tc.withPlan)
			er := refuseEpic(t, tc.args...)
			require.Equal(t, tc.code, er.Code, er.Message)
		})
	}
}

// The merge-conflict envelope lists repos in name order with their paths.
func TestMergeConflict_ListsPathsPerRepo(t *testing.T) {
	err := mergeConflict("alpha", map[string][]string{"zeta": {"z.go"}, "docs": {"a.md", "b.md"}})
	er, ok := err.(*output.ErrorResponse)
	require.True(t, ok)
	require.Equal(t, "epic_merge_conflict", er.Code)
	require.Equal(t, "merging alpha would conflict, so nothing was merged in any repo; conflicting paths — docs: a.md, b.md; zeta: z.go", er.Message)
	require.Contains(t, er.NextAction, "spek/alpha")
}

func TestEpicWorktreeSchema_PublishesInputAndOutput(t *testing.T) {
	epicProject(t)
	for verb, outKeys := range map[string][]string{
		"worktree": {"spec", "project", "branch", "repos", "created"},
		"merge":    {"spec", "merged", "removed"},
	} {
		t.Run(verb, func(t *testing.T) {
			stdout, code := runEpic(t, verb, "--schema")
			require.Equal(t, 0, code, stdout)
			var got struct {
				Input struct {
					Required []string `json:"required"`
				} `json:"input"`
				Output struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"output"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &got))
			require.Equal(t, []string{"spec"}, got.Input.Required)
			keys := make([]string, 0, len(got.Output.Properties))
			for k := range got.Output.Properties {
				keys = append(keys, k)
			}
			require.ElementsMatch(t, outKeys, keys)
		})
	}
}
