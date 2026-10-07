package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// These tests drive implement runs the user starts — not an epic
// orchestrator — end to end against the two-repo git fixture of
// epic_worktree_test.go (testproj and docs at <base>/website), every command
// run from the main project root: `implement new` makes the spec's
// worktrees, the agent's code goes only there, the walk to finished commits
// it on spek/<spec>, and `implement merge` brings it back into both main
// lines. Artifacts are written through the CLI only.

// implementFlowBetaPlan is a second spec's plan, touching testproj only.
const implementFlowBetaPlan = `# Plan: beta

## Overview

fixture

## Milestones & Tasks

### Milestone 1: The project

#### - [ ] Task: Change the project for beta
**Id:** 3e8d1a6f-2c4b-4f9e-a17d-6b0c5e2f9a83
**Repo:** testproj
**Depends on:** none
**Execution:** agent
`

// implementFlowProject is the two-repo fixture with alpha planned, a second
// spec beta planned against testproj, spec.id_method external (so the CLI's
// plan and changelog writes accept the unprefixed names), and extraConfig
// ahead of the registry; everything committed as the baseline.
func implementFlowProject(t *testing.T, extraConfig string) worktreeFixture {
	t.Helper()
	f := worktreeProjectWith(t, true, extraConfig+"spec:\n  id_method: external\n")
	wtWriteFile(t, f.proj, ".spektacular/specs/beta.md", epicTestSpecFixed)
	wtWriteFile(t, f.proj, ".spektacular/plans/beta/plan.md", implementFlowBetaPlan)
	wtCommitAll(t, f.proj, "plan beta")
	return f
}

// specWT is spec's worktree of the repo registered as repo.
func specWT(f worktreeFixture, spec, repo string) string {
	return filepath.Join(f.proj, ".spektacular", "worktrees", spec, repo)
}

// implementFlowStart starts spec's run from the main root and returns its
// first instruction.
func implementFlowStart(t *testing.T, spec string) string {
	t.Helper()
	return instructionField(t, epicFlowCLI(t, "implement", "new", "--data", `{"name":"`+spec+`"}`))
}

// implementFlowFinish ticks every task of spec's plan, writes its changelog
// record, and walks its lane to finished naming it on every goto, staging
// the commit message the completion commit needs. It returns the finished
// instruction.
func implementFlowFinish(t *testing.T, proj, spec string) string {
	t.Helper()
	plan := epicFlowRead(t, "plan", "file", "read", spec, "plan")
	require.Contains(t, plan, "#### - [ ] Task:")
	epicFlowWrite(t, proj, strings.ReplaceAll(plan, "#### - [ ] Task:", "#### - [x] Task:"),
		"plan", "file", "write", spec, "plan")
	epicFlowWrite(t, proj, "# "+spec+"\n\nwhat was built\n", "changelog", "file", "write", spec)

	for _, step := range []string{
		"analyze", "implement", "test", "verify", "update_plan", "update_changelog",
		"test_plan", "update_feature_changelog", "reconcile_spec",
	} {
		epicFlowCLI(t, "implement", "goto", "--data", `{"step":"`+step+`","name":"`+spec+`"}`)
	}
	epicFlowStage(t, proj, "git-commit-message.md", "Implement "+spec+"\n\nThe spec's code.\n")
	out := epicFlowCLI(t, "implement", "goto", "--data",
		`{"step":"finished","name":"`+spec+`","commit_message_from":"`+stagedCommitMessagePath+`"}`)
	return instructionField(t, out)
}

// implementFlowMerge runs `implement merge` for spec and requires it to merge
// and remove everything.
func implementFlowMerge(t *testing.T, spec string) {
	t.Helper()
	stdout, code := implementMergeCLI(t, "--data", `{"name":"`+spec+`"}`)
	require.Equalf(t, 0, code, "implement merge failed: %s", stdout)
	require.JSONEq(t, `{"error":false,"spec":"`+spec+`","merged":true,"removed":true}`, stdout)
}

// requireMainCodeClean asserts a main checkout has no modified or untracked
// file outside its .spektacular folder.
func requireMainCodeClean(t *testing.T, dir string) {
	t.Helper()
	require.Empty(t, gittest.RunGit(t, dir, "status", "--porcelain", "--untracked-files=all",
		"--", ".", ":(exclude).spektacular"), dir)
}

// requireSpecGone asserts spec has no worktree folder, no spek/ branch in
// either repo, and no record.
func requireSpecGone(t *testing.T, f worktreeFixture, spec string) {
	t.Helper()
	require.NoDirExists(t, filepath.Join(f.proj, ".spektacular", "worktrees", spec))
	for _, dir := range []string{f.proj, f.site} {
		require.Empty(t, gittest.RunGit(t, dir, "branch", "--list", "spek/"+spec), dir)
		require.NotContains(t, gittest.RunGit(t, dir, "worktree", "list"), filepath.Join("worktrees", spec), dir)
	}
	_, ok, err := worktree.ReadRecord(f.proj, spec)
	require.NoError(t, err)
	require.False(t, ok, "the worktree record outlived the merge")
}

// requireOnMain asserts main in dir holds file with content, committed and on
// disk.
func requireOnMain(t *testing.T, dir, file, content string) {
	t.Helper()
	require.Equal(t, "main", gittest.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"), dir)
	require.Equal(t, strings.TrimSuffix(content, "\n"), gittest.RunGit(t, dir, "show", "main:"+file), dir)
	require.Equal(t, content, string(readBytes(t, filepath.Join(dir, file))), dir)
}

// cleanTwoRepoRun drives alpha from `implement new` to a merged finish under
// config extraConfig and returns the fixture with the commits each main line
// started from.
func cleanTwoRepoRun(t *testing.T, extraConfig string, beforeMerge func(f worktreeFixture, bases map[string]string)) worktreeFixture {
	t.Helper()
	f := implementFlowProject(t, extraConfig)
	bases := map[string]string{
		f.proj: gittest.RunGit(t, f.proj, "rev-parse", "HEAD"),
		f.site: gittest.RunGit(t, f.site, "rev-parse", "HEAD"),
	}

	section := codeLocationSection(t, implementFlowStart(t, "alpha"))
	require.Contains(t, section, "- `testproj`: `"+f.wt("testproj")+"`")
	require.Contains(t, section, "- `docs`: `"+f.wt("docs")+"`")
	requireUserLane(t, filepath.Join(f.proj, ".spektacular"), "alpha", "read_plan")

	wtWriteFile(t, f.wt("testproj"), "feature.txt", "alpha feature\n")
	wtWriteFile(t, f.wt("docs"), "guide.txt", "alpha guide\n")

	finished := implementFlowFinish(t, f.proj, "alpha")
	require.Contains(t, finished, `implement merge --data '{"name":"alpha"}'`)

	// Before the merge: the code is committed on spek/alpha in each worktree
	// and nowhere in either main checkout, outside .spektacular.
	for wt, code := range map[string]string{f.wt("testproj"): "feature.txt", f.wt("docs"): "guide.txt"} {
		require.Equal(t, "spek/alpha", gittest.RunGit(t, wt, "rev-parse", "--abbrev-ref", "HEAD"), wt)
		require.Empty(t, gittest.RunGit(t, wt, "status", "--porcelain", "--untracked-files=all"), wt)
		require.Contains(t, gittest.RunGit(t, wt, "log", "-1", "--format=%B"), "Implement alpha", wt)
		require.Contains(t, gittest.RunGit(t, wt, "show", "--name-only", "--format=", "HEAD"), code, wt)
	}
	for _, dir := range []string{f.proj, f.site} {
		requireMainCodeClean(t, dir)
		require.NoFileExists(t, filepath.Join(dir, "feature.txt"))
		require.NoFileExists(t, filepath.Join(dir, "guide.txt"))
	}
	if beforeMerge != nil {
		beforeMerge(f, bases)
	}

	implementFlowMerge(t, "alpha")

	requireOnMain(t, f.proj, "feature.txt", "alpha feature\n")
	requireOnMain(t, f.site, "guide.txt", "alpha guide\n")
	requireSpecGone(t, f, "alpha")
	return f
}

// Criteria 3 and 2, with automatic commits off and no implement key at all
// (so worktrees are on by default): the run builds in worktrees, nothing
// reaches either main checkout before the merge, and the merge leaves both
// main lines holding the code with no worktree, branch or record left.
func TestImplementWorktreeFlow_CleanTwoRepoRunAutoCommitOff(t *testing.T) {
	f := cleanTwoRepoRun(t, "auto_commit: off\n", func(f worktreeFixture, bases map[string]string) {
		// Off means nothing was committed in either main line.
		for dir, base := range bases {
			require.Equal(t, base, gittest.RunGit(t, dir, "rev-parse", "HEAD"), dir)
		}
	})
	cfg := string(readBytes(t, filepath.Join(f.proj, ".spektacular", "config.yaml")))
	require.NotContains(t, cfg, "implement:", "this run must rely on the default, not an implement key")
}

// The same run with auto_commit workflow: the spec's artifacts are committed
// in the project's main line, its code only on the spec branch until the
// merge.
func TestImplementWorktreeFlow_CleanTwoRepoRunAutoCommitWorkflow(t *testing.T) {
	cleanTwoRepoRun(t, "auto_commit: workflow\n", func(f worktreeFixture, bases map[string]string) {
		// Only the spec's artifacts — no code, and no lane state, which a
		// finished lane removes.
		require.Equal(t,
			".spektacular/changelog/alpha.md\n"+
				".spektacular/plans/alpha/plan.md",
			changedIn(t, f.proj, bases[f.proj]))
	})
}

// Criterion 1: two specs against the same repo are in progress at once, each
// in its own worktree and lane; one's code is in neither the other's
// worktree nor the main checkout until it merges; then both finish and merge
// cleanly in turn.
func TestImplementWorktreeFlow_TwoSpecsAreIsolatedUntilMerged(t *testing.T) {
	f := implementFlowProject(t, "auto_commit: off\n")
	dataDir := filepath.Join(f.proj, ".spektacular")
	alphaWT, betaWT := specWT(f, "alpha", "testproj"), specWT(f, "beta", "testproj")

	implementFlowStart(t, "alpha")
	section := codeLocationSection(t, implementFlowStart(t, "beta"))
	require.Contains(t, section, "- `testproj`: `"+betaWT+"`")
	require.NotContains(t, section, alphaWT)
	require.Equal(t, "spek/beta", gittest.RunGit(t, betaWT, "rev-parse", "--abbrev-ref", "HEAD"))
	require.NoDirExists(t, specWT(f, "beta", "docs"), "beta's plan does not touch docs")

	epicFlowCLI(t, "implement", "goto", "--data", `{"step":"analyze","name":"alpha"}`)
	requireUserLane(t, dataDir, "alpha", "analyze")
	requireUserLane(t, dataDir, "beta", "read_plan")

	wtWriteFile(t, alphaWT, "alpha.txt", "alpha work\n")
	wtWriteFile(t, betaWT, "beta.txt", "beta work\n")
	require.NoFileExists(t, filepath.Join(betaWT, "alpha.txt"))
	require.NoFileExists(t, filepath.Join(alphaWT, "beta.txt"))
	for _, dir := range []string{f.proj, f.site} {
		requireMainCodeClean(t, dir)
	}

	implementFlowFinish(t, f.proj, "alpha")
	requireUserLane(t, dataDir, "beta", "read_plan")
	require.NoFileExists(t, filepath.Join(betaWT, "alpha.txt"))
	require.NoFileExists(t, filepath.Join(f.proj, "alpha.txt"))
	requireMainCodeClean(t, f.proj)

	implementFlowMerge(t, "alpha")
	requireOnMain(t, f.proj, "alpha.txt", "alpha work\n")
	requireSpecGone(t, f, "alpha")
	// Beta's worktree still sees only its own work.
	require.NoFileExists(t, filepath.Join(betaWT, "alpha.txt"))
	require.FileExists(t, filepath.Join(betaWT, "beta.txt"))
	require.NoFileExists(t, filepath.Join(f.proj, "beta.txt"))

	implementFlowFinish(t, f.proj, "beta")
	implementFlowMerge(t, "beta")
	requireOnMain(t, f.proj, "alpha.txt", "alpha work\n")
	requireOnMain(t, f.proj, "beta.txt", "beta work\n")
	requireSpecGone(t, f, "beta")
	requireMainCodeClean(t, f.proj)
}

// Criterion 4: a conflict in docs moves neither main line and keeps the
// worktrees and branches.
func TestImplementWorktreeFlow_ConflictMergesNothing(t *testing.T) {
	f := implementFlowProject(t, "auto_commit: off\n")
	implementFlowStart(t, "alpha")

	// Competing work lands on docs' main line after the run started.
	wtWriteFile(t, f.site, "lib.txt", "from main\n")
	wtCommitAll(t, f.site, "competing work")

	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtWriteFile(t, f.wt("docs"), "lib.txt", "from the spec\n")
	implementFlowFinish(t, f.proj, "alpha")
	projHead := gittest.RunGit(t, f.proj, "rev-parse", "HEAD")
	siteHead := gittest.RunGit(t, f.site, "rev-parse", "HEAD")

	er := refuseImplementMerge(t, "--data", `{"name":"alpha"}`)
	require.Equal(t, "epic_merge_conflict", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.Message, "docs: lib.txt")
	require.NotContains(t, er.Message, "testproj")

	require.Equal(t, projHead, gittest.RunGit(t, f.proj, "rev-parse", "HEAD"))
	require.Equal(t, siteHead, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
	require.Equal(t, "main v1\n", string(readBytes(t, filepath.Join(f.proj, "main.txt"))))
	require.Equal(t, "from main\n", string(readBytes(t, filepath.Join(f.site, "lib.txt"))))
	for _, dir := range []string{f.proj, f.site} {
		requireMainCodeClean(t, dir)
	}
	requireWorktreesKept(t, f)
}

// Criterion 5: with worktrees off the run uses the shared slot and its code
// lands in the main checkouts, while `epic worktree` for another spec in the
// same project still makes worktrees.
func TestImplementWorktreeFlow_OptOutBuildsInMainAndEpicStillUsesWorktrees(t *testing.T) {
	f := implementFlowProject(t, implementSharedSlotConfig)
	dataDir := filepath.Join(f.proj, ".spektacular")

	section := codeLocationSection(t, implementFlowStart(t, "alpha"))
	require.Contains(t, section, "Run `spektacular repo list` now if you have not already")
	require.Equal(t, "alpha", sharedState(t, dataDir).Data["name"])
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "implement-alpha.json"))
	require.NoDirExists(t, filepath.Join(dataDir, "worktrees", "alpha"))
	for _, dir := range []string{f.proj, f.site} {
		require.Empty(t, gittest.RunGit(t, dir, "branch", "--list", "spek/*"), dir)
	}

	// The registered roots are the main checkouts: the code shows there.
	wtWriteFile(t, f.proj, "main.txt", "main v2\n")
	wtWriteFile(t, f.site, "lib.txt", "lib v2\n")
	require.Equal(t, "M main.txt", gittest.RunGit(t, f.proj, "status", "--porcelain", "--", ".", ":(exclude).spektacular"))
	require.Equal(t, "M lib.txt", gittest.RunGit(t, f.site, "status", "--porcelain", "--", ".", ":(exclude).spektacular"))

	stdout, code := runEpic(t, "worktree", "--data", `{"spec":"beta"}`)
	require.Equalf(t, 0, code, "epic worktree failed: %s", stdout)
	betaWT := specWT(f, "beta", "testproj")
	require.Equal(t, "spek/beta", gittest.RunGit(t, betaWT, "rev-parse", "--abbrev-ref", "HEAD"))
	require.Equal(t, "main v1\n", string(readBytes(t, filepath.Join(betaWT, "main.txt"))),
		"the epic's worktree starts from the committed line, not alpha's edits")
	_, ok, err := worktree.ReadRecord(f.proj, "beta")
	require.NoError(t, err)
	require.True(t, ok)
}

// Criterion 6: the harbor implement suite's project opts out of worktrees,
// so its runs keep the shared state.json its oracle reads.
func TestImplementWorktreeFlow_HarborSuiteOptsOut(t *testing.T) {
	// Read the key itself: the suite's config is an older settings format
	// the environment brings up to date, so it is not loaded as a Config.
	var cfg struct {
		Implement struct {
			Worktrees *bool `yaml:"worktrees"`
		} `yaml:"implement"`
	}
	raw := readBytes(t, filepath.Join("..", "tests", "harbor", "implement-workflow", "environment", "config.yaml"))
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	require.NotNil(t, cfg.Implement.Worktrees, "the suite must set implement.worktrees explicitly")
	require.False(t, *cfg.Implement.Worktrees)

	oracle := string(readBytes(t, filepath.Join("..", "tests", "harbor", "implement-workflow", "tests", "test_implement_workflow.py")))
	require.Contains(t, oracle, `PROJECT_SPEK_DIR / "state.json"`)
}
