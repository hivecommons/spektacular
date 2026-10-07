package cmd

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// The run view's dirty probe looks only at the repos it is handed, reports
// them by the touched name, and never changes the user's checkouts.
func TestStatusRunSource_DirtyOnlyChecksTouchedRepos(t *testing.T) {
	f := worktreeProject(t, true)
	cfg, err := loadConfig()
	require.NoError(t, err)
	src := statusRunSource(cfg, f.proj, store.NewSourceStore(f.proj, "project"))
	headProj := gittest.RunGit(t, f.proj, "rev-parse", "HEAD")
	headSite := gittest.RunGit(t, f.site, "rev-parse", "HEAD")

	require.Empty(t, src.Dirty([]string{"testproj", "docs"}), "clean checkouts report nothing")

	// The registered docs repo has untracked work, but no task touches it.
	wtWriteFile(t, f.site, "untracked.txt", "user work\n")
	require.Empty(t, src.Dirty([]string{"testproj"}))
	require.Equal(t, []string{"docs"}, src.Dirty([]string{"testproj", "docs"}))
	require.Empty(t, src.Dirty(nil), "no plans means no touched repos")
	require.Empty(t, src.Dirty([]string{}))

	// Tracked modifications are included as well as untracked files.
	wtWriteFile(t, f.proj, "main.txt", "user change\n")
	require.Equal(t, []string{"testproj", "docs"}, src.Dirty([]string{"testproj", "docs"}))
	require.Equal(t, []string{"testproj"}, src.Dirty([]string{"testproj", "testproj"}), "duplicates are reported once")

	// Repos sharing a checkout still report only the touched names.
	cfg.Repos = append(cfg.Repos, config.RepoEntry{Name: "alias", Location: "."})
	src = statusRunSource(cfg, f.proj, store.NewSourceStore(f.proj, "project"))
	require.Equal(t, []string{"alias"}, src.Dirty([]string{"alias"}))
	require.Equal(t, []string{"testproj", "alias"}, src.Dirty([]string{"alias", "testproj"}))

	// Reporting never cleans or commits the user's changes.
	require.Contains(t, gittest.RunGit(t, f.proj, "status", "--porcelain"), "main.txt")
	require.Contains(t, gittest.RunGit(t, f.site, "status", "--porcelain"), "untracked.txt")
	require.Equal(t, headProj, gittest.RunGit(t, f.proj, "rev-parse", "HEAD"))
	require.Equal(t, headSite, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
}

// worktreeTestPlanProjectOnly has one task, in the project repo alone, so the
// registered docs repo is never touched.
const worktreeTestPlanProjectOnly = `# Plan: alpha

## Overview

fixture

## Milestones & Tasks

### Milestone 1: Project only

#### - [ ] Task: Change the project
**Id:** 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95
**Repo:** testproj
**Depends on:** none
**Execution:** agent
`

// Through the command: dirt in a registered repo the epic's plans never touch
// leaves the epic clean, dirt in a touched repo raises the flag and names it.
func TestStatus_EpicDirtyNamesOnlyTouchedRepos(t *testing.T) {
	f := worktreeProject(t, false)
	ep := epic.Epic{
		CreatedDate:    time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
		Specs:          []epic.EpicSpec{{Name: "alpha", DependsOn: []string{}}},
		Body:           []byte("## Overview\n\nAn epic.\n"),
	}
	raw, err := ep.Render()
	require.NoError(t, err)
	wtWriteFile(t, f.proj, ".spektacular/epics/E.md", string(raw))
	wtWriteFile(t, f.proj, ".spektacular/specs/alpha.md", specInEpicFixture("E"))
	wtWriteFile(t, f.proj, ".spektacular/plans/alpha/plan.md", worktreeTestPlanProjectOnly)
	wtCommitAll(t, f.proj, "epic E")

	epicRun := func() map[string]any {
		t.Helper()
		got := statusOf(t, "E", "--format", "json")
		return got["epic"].(map[string]any)["run"].(map[string]any)
	}

	run := epicRun()
	require.Equal(t, false, run["dirty"])
	require.Equal(t, []any{}, run["dirty_repos"])

	// The docs repo is registered but untouched: its dirt is not reported.
	wtWriteFile(t, f.site, "untracked.txt", "user work\n")
	run = epicRun()
	require.Equal(t, false, run["dirty"])
	require.Equal(t, []any{}, run["dirty_repos"])

	// The touched project repo gets a tracked change: flagged and named.
	wtWriteFile(t, f.proj, "main.txt", "user change\n")
	run = epicRun()
	require.Equal(t, true, run["dirty"])
	require.Equal(t, []any{"testproj"}, run["dirty_repos"])
}
