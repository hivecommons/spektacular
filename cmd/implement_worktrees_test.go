package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/stretchr/testify/require"
)

// These tests cover the worktrees `implement new` makes for a run the user
// starts: with implement.worktrees on, the spec gets a worktree on spek/<spec>
// in every repo its plan touches before the first instruction, which names
// those worktrees as where the code lives. They run against the real
// two-repo git fixture from epic_worktree_test.go (testproj and docs), with a
// third registered repo, "extra", at <base>/extra.

// extraRepoTask is a plan task in the third repo, appended to
// worktreeTestPlan when the plan touches it.
const extraRepoTask = `
#### - [ ] Task: Change the extra repo
**Id:** 5f3a9c1e-7b2d-4e8f-a604-2c9d1b7e3f50
**Repo:** extra
**Depends on:** none
**Execution:** agent
`

// extraRepoState is how the third repo is laid out on disk.
type extraRepoState int

const (
	extraCommitted extraRepoState = iota // a git repo with a commit
	extraNoGit                           // a plain folder, not in git
	extraNoCommits                       // git init-ed, nothing committed
)

// implementWorktreeFixture is the two-repo git fixture plus the third repo.
type implementWorktreeFixture struct {
	worktreeFixture
	extra string
}

// implementWorktreeProject builds the git fixture with extraConfig ahead of
// the registry, registers a third repo "extra" laid out as state, plans the
// spec alpha (touching extra only when touchExtra), commits the project and
// chdirs into it.
func implementWorktreeProject(t *testing.T, extraConfig string, state extraRepoState, touchExtra bool) implementWorktreeFixture {
	t.Helper()
	f := implementWorktreeFixture{worktreeFixture: worktreeProjectWith(t, false, extraConfig)}
	f.extra = filepath.Join(filepath.Dir(f.proj), "extra")

	rc := config.NewDefaultRepoConfig()
	rc.Source = config.DefaultRepoSource
	require.NoError(t, os.MkdirAll(filepath.Join(f.extra, ".spektacular"), 0o755))
	require.NoError(t, rc.ToYAMLFile(filepath.Join(f.extra, ".spektacular", config.RepoConfigFileName)))
	wtWriteFile(t, f.extra, "extra.txt", "extra v1\n")
	switch state {
	case extraCommitted:
		gittest.RunGit(t, f.extra, "init", "-q", "-b", "main")
		wtCommitAll(t, f.extra, "initial extra")
	case extraNoCommits:
		gittest.RunGit(t, f.extra, "init", "-q", "-b", "main")
	}

	cfgPath := filepath.Join(f.proj, ".spektacular", "config.yaml")
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath,
		append(raw, []byte("  - name: extra\n    location: ../../extra/.spektacular\n")...), 0o644))
	plan := worktreeTestPlan
	if touchExtra {
		plan += extraRepoTask
	}
	wtWriteFile(t, f.proj, ".spektacular/plans/alpha/plan.md", plan)
	wtCommitAll(t, f.proj, "register extra and plan alpha")
	return f
}

// requireSpecBranch asserts whether repo dir has the spec's branch.
func requireSpecBranch(t *testing.T, dir string, want bool) {
	t.Helper()
	got := gittest.RunGit(t, dir, "branch", "--list", "spek/alpha")
	if want {
		require.NotEmptyf(t, got, "%s must have branch spek/alpha", dir)
	} else {
		require.Emptyf(t, got, "%s must have no spek/alpha branch", dir)
	}
}

// requireNoWorktrees asserts the run made no worktree, branch or record.
func requireNoWorktrees(t *testing.T, f implementWorktreeFixture) {
	t.Helper()
	require.NoDirExists(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha"))
	_, ok, err := worktree.ReadRecord(f.proj, "alpha")
	require.NoError(t, err)
	require.False(t, ok, "no worktree record may be written")
	for _, dir := range []string{f.proj, f.site, f.extra} {
		requireSpecBranch(t, dir, false)
	}
}

// codeLocationSection returns read_plan's "Where the code lives" block of an
// instruction, up to the next heading.
func codeLocationSection(t *testing.T, instruction string) string {
	t.Helper()
	const heading = "**Where the code lives.**"
	i := strings.Index(instruction, heading)
	require.GreaterOrEqualf(t, i, 0, "the instruction must have a %q section", heading)
	rest := instruction[i+len(heading):]
	if j := strings.Index(rest, "\n### "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// countingGit is a worktree.Runner that only counts its calls.
type countingGit struct{ calls int }

func (g *countingGit) Run(string, ...string) (string, int, error) {
	g.calls++
	return "", 1, nil
}

// Criteria 1 and 2: a run started on its own whose plan touches testproj and
// docs gets a worktree on spek/alpha in each, recorded, and none in the
// untouched extra repo; its first instruction names the worktrees, never the
// main checkouts.
func TestImplementWorktrees_NewMakesWorktreesInTouchedReposOnly(t *testing.T) {
	f := implementWorktreeProject(t, "", extraCommitted, false)

	out := runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)

	for _, dir := range []string{f.wt("testproj"), f.wt("docs")} {
		require.Equal(t, "spek/alpha", gittest.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	}
	require.FileExists(t, filepath.Join(f.wt("testproj"), "main.txt"))
	require.FileExists(t, filepath.Join(f.wt("docs"), "lib.txt"))
	requireSpecBranch(t, f.proj, true)
	requireSpecBranch(t, f.site, true)
	requireSpecBranch(t, f.extra, false)
	require.NoDirExists(t, f.wt("extra"))

	rec, ok, err := worktree.ReadRecord(f.proj, "alpha")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{"testproj": f.wt("testproj"), "docs": f.wt("docs")}, rec.Repos)

	section := codeLocationSection(t, instructionField(t, out))
	require.Contains(t, section, "This spec is built in its own worktrees")
	require.Contains(t, section, "- `testproj`: `"+f.wt("testproj")+"`")
	require.Contains(t, section, "- `docs`: `"+f.wt("docs")+"`")
	for _, main := range []string{f.proj, f.site, f.extra} {
		require.NotContains(t, section, "`"+main+"`", "the section must not name a main checkout")
	}
	require.NotContains(t, section, "`extra`")
}

// Criterion 3: with implement.worktrees off, a run makes no worktree and no
// branch, and its instruction sends the agent to repo list.
func TestImplementWorktrees_OffMakesNone(t *testing.T) {
	f := implementWorktreeProject(t, implementSharedSlotConfig, extraCommitted, false)

	out := runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)

	requireNoWorktrees(t, f)
	require.Contains(t, codeLocationSection(t, instructionField(t, out)), "Run `spektacular repo list` now if you have not already")
}

// An orchestrated child's worktrees are its orchestrator's to make, so its
// start makes none even when the spec has no record.
func TestImplementWorktrees_OrchestratedMakesNone(t *testing.T) {
	f := implementWorktreeProject(t, "", extraCommitted, false)

	runOK(t, "implement", "new", "--data", `{"name":"alpha","orchestrated":true}`)

	requireNoWorktrees(t, f)
}

// A dry run makes no worktree.
func TestImplementWorktrees_DryRunMakesNone(t *testing.T) {
	f := implementWorktreeProject(t, "", extraCommitted, false)

	runOK(t, "implement", "new", "--dry-run", "--data", `{"name":"alpha"}`)

	requireNoWorktrees(t, f)
}

// Criterion 4: a spec whose worktrees already exist (here from `epic
// worktree`) starts without running git, and the run uses those worktrees.
func TestImplementWorktrees_ExistingRecordRunsNoGit(t *testing.T) {
	f := implementWorktreeProject(t, "", extraCommitted, false)
	runEpicWorktreeCmd(t)
	before := readBytes(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "record.json"))

	git := &countingGit{}
	prev := worktreeGit
	worktreeGit = git
	t.Cleanup(func() { worktreeGit = prev })

	out := runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)

	require.Zero(t, git.calls, "an existing record must not run git")
	require.Equal(t, before, readBytes(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "record.json")))
	section := codeLocationSection(t, instructionField(t, out))
	require.Contains(t, section, "- `testproj`: `"+f.wt("testproj")+"`")
	require.Contains(t, section, "- `docs`: `"+f.wt("docs")+"`")
}

// Criterion 4: a later run for the same spec reuses the worktrees the first
// made: same paths, same commits, no new worktree.
func TestImplementWorktrees_LaterRunReusesWorktrees(t *testing.T) {
	f := implementWorktreeProject(t, "", extraCommitted, false)
	runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)
	recordPath := filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "record.json")
	before := readBytes(t, recordPath)
	// Work committed in a worktree survives, so it was not recreated.
	wtWriteFile(t, f.wt("docs"), "lib.txt", "lib v2\n")
	wtCommitAll(t, f.wt("docs"), "docs work")
	docsHead := gittest.RunGit(t, f.wt("docs"), "rev-parse", "HEAD")
	worktreesBefore := gittest.RunGit(t, f.site, "worktree", "list", "--porcelain")

	out := runOK(t, "implement", "new", "--force", "--data", `{"name":"alpha"}`)

	require.Equal(t, before, readBytes(t, recordPath))
	require.Equal(t, docsHead, gittest.RunGit(t, f.wt("docs"), "rev-parse", "HEAD"))
	require.Equal(t, worktreesBefore, gittest.RunGit(t, f.site, "worktree", "list", "--porcelain"))
	section := codeLocationSection(t, instructionField(t, out))
	require.Contains(t, section, "- `testproj`: `"+f.wt("testproj")+"`")
	require.Contains(t, section, "- `docs`: `"+f.wt("docs")+"`")
}

// Criterion 5: a touched repo that is not in git refuses the run with
// worktree_unavailable, naming the repo and offering the opt-out, and
// starts nothing.
func TestImplementWorktrees_TouchedRepoNotInGitIsRefused(t *testing.T) {
	f := implementWorktreeProject(t, "", extraNoGit, true)

	er := runRefused(t, "implement", "new", "--data", `{"name":"alpha"}`)

	require.Equal(t, "worktree_unavailable", er.Code)
	require.Equal(t, "extra", er.Resource)
	require.Contains(t, er.Message, f.extra+" is not in a git repository")
	require.Contains(t, er.NextAction, "implement.worktrees: false")
	require.NoDirExists(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha"))
	requireSpecBranch(t, f.proj, false)
	requireSpecBranch(t, f.site, false)
}

// Criterion 5: a touched repo with no commits refuses the run the same way,
// before any worktree is made.
func TestImplementWorktrees_TouchedRepoWithNoCommitsIsRefused(t *testing.T) {
	f := implementWorktreeProject(t, "", extraNoCommits, true)

	er := runRefused(t, "implement", "new", "--data", `{"name":"alpha"}`)

	require.Equal(t, "worktree_unavailable", er.Code)
	require.Equal(t, "extra", er.Resource)
	require.Contains(t, er.Message, f.extra+" has no commits yet")
	require.Contains(t, er.NextAction, "git init")
	require.Contains(t, er.NextAction, "implement.worktrees: false")
	requireNoWorktrees(t, f)
}
