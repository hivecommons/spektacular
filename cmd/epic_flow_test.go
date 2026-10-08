package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// These tests drive one spec of an epic through the whole flow the way an
// orchestrator does — every command run from the main project root — against
// the two-repo fixture of epic_worktree_test.go: spec worktrees, an
// orchestrated implement run committing on completion, status, and the merge.
// The spec's artifacts are only ever written through the CLI.

// epicFlowEpic is the epic the fixture's spec is placed in, so `status` for
// it carries the run view.
const epicFlowEpic = "flow"

// epicFlowFinish advances alpha's orchestrated implement lane to its
// terminal step carrying the staged commit message.
const epicFlowFinish = `{"step":"finished","name":"alpha","commit_message_from":".spektacular/tmp/git-commit-message.md"}`

// epicFlowCLI runs one command from the main root and requires it to succeed.
func epicFlowCLI(t *testing.T, args ...string) string {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equalf(t, 0, code, "%v failed: %s %s", args, stdout, stderr)
	return stdout
}

// epicFlowStage writes body to a scratch file under the project's
// .spektacular/tmp and returns its path, for a CLI write's --from.
func epicFlowStage(t *testing.T, proj, name, body string) string {
	t.Helper()
	path := filepath.Join(proj, ".spektacular", "tmp", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// epicFlowWrite stages body, runs the CLI write args with --from it, and
// removes the staged file once the write succeeds, as the scratch-surface
// convention asks of an agent.
func epicFlowWrite(t *testing.T, proj, body string, args ...string) {
	t.Helper()
	path := epicFlowStage(t, proj, "staged.md", body)
	epicFlowCLI(t, append(args, "--from", path)...)
	require.NoError(t, os.Remove(path))
}

// epicFlowRead returns the document a CLI file read prints.
func epicFlowRead(t *testing.T, args ...string) string {
	t.Helper()
	return epicFlowCLI(t, args...)
}

// epicFlowProject is the worktree fixture in auto_commit workflow mode with
// alpha placed in an epic (spec.id_method external, so the CLI's plan and
// changelog writes accept the fixture's unprefixed spec name), committed as the baseline, and alpha's worktrees
// created. It returns the commit each worktree was created from.
func epicFlowProject(t *testing.T) (worktreeFixture, map[string]string) {
	t.Helper()
	f := worktreeProjectWith(t, true, "auto_commit: workflow\nspec:\n  id_method: external\n")
	epicWrite(t, epicFlowEpic, specsData("alpha"))
	wtCommitAll(t, f.proj, "alpha joins an epic")

	got := runEpicWorktreeCmd(t)
	require.True(t, got.Created)
	bases := map[string]string{}
	for _, dir := range []string{f.wt("testproj"), f.wt("docs")} {
		bases[dir] = gittest.RunGit(t, dir, "rev-parse", "HEAD")
	}
	return f, bases
}

// epicFlowImplement starts alpha's orchestrated implement run from the main
// root and returns the first instruction it hands the agent.
func epicFlowImplement(t *testing.T) string {
	t.Helper()
	return epicFlowCLI(t, "implement", "new", "--data", `{"name":"alpha","orchestrated":true}`)
}

// epicFlowTickPlan ticks every task of alpha's plan through the CLI.
func epicFlowTickPlan(t *testing.T, proj string) {
	t.Helper()
	plan := epicFlowRead(t, "plan", "file", "read", "alpha", "plan")
	require.Contains(t, plan, "#### - [ ] Task: Change the project")
	ticked := strings.ReplaceAll(plan, "#### - [ ] Task:", "#### - [x] Task:")
	epicFlowWrite(t, proj, ticked, "plan", "file", "write", "alpha", "plan")
}

// epicFlowWalkToFinished drives alpha's lane through every step and finishes
// it with a staged commit message, auto-committing.
func epicFlowWalkToFinished(t *testing.T, proj string) {
	t.Helper()
	for _, step := range []string{
		"analyze", "implement", "test", "verify", "update_plan", "update_changelog",
		"test_plan", "update_feature_changelog", "reconcile_spec",
	} {
		epicFlowCLI(t, "implement", "goto", "--data", `{"step":"`+step+`","name":"alpha"}`)
	}
	epicFlowStage(t, proj, "git-commit-message.md", "Implement alpha\n\nBoth repos changed.\n")
	epicFlowCLI(t, "implement", "goto", "--data", epicFlowFinish)
}

// requireSpektacularAtBase: the worktree's .spektacular is exactly its base
// commit's, committed and on disk, ignored files included.
func requireSpektacularAtBase(t *testing.T, wt, base string) {
	t.Helper()
	require.Empty(t, gittest.RunGit(t, wt, "diff", base, "--", ".spektacular"), wt)
	require.Empty(t, gittest.RunGit(t, wt, "diff", base, "HEAD", "--", ".spektacular"), wt)
	require.Empty(t, gittest.RunGit(t, wt, "status", "--porcelain", "--ignored", "--", ".spektacular"), wt)
}

// One spec through the whole epic flow from the main root: its plan tick is
// readable in the main project before the merge and byte-identical after it,
// every worktree's .spektacular stays exactly its base commit's, and the
// merge brings only code — nothing under any .spektacular — with nothing to
// reconcile by hand.
func TestEpicFlow_ArtifactsStayInTheProjectAndMergeBringsOnlyCode(t *testing.T) {
	f, bases := epicFlowProject(t)

	// The run, started from the main root, is told where the code lives.
	instruction := epicFlowImplement(t)
	require.Contains(t, instruction, "This spec is built in its own worktrees")
	require.Contains(t, instruction, "- `testproj`: `"+f.wt("testproj")+"`")
	require.Contains(t, instruction, "- `docs`: `"+f.wt("docs")+"`")

	// The agent's code, in the worktrees only.
	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtWriteFile(t, f.wt("docs"), "lib.txt", "lib v2\n")

	// The agent's artifacts, through the CLI from the main root.
	epicFlowTickPlan(t, f.proj)
	epicFlowWrite(t, f.proj, "# alpha\n\nwhat was built\n", "changelog", "file", "write", "alpha")
	epicFlowWalkToFinished(t, f.proj)

	// Before the merge: the tick is readable in the main project.
	plan := epicFlowRead(t, "plan", "file", "read", "alpha", "plan")
	require.Contains(t, plan, "#### - [x] Task: Change the project")
	require.Contains(t, plan, "#### - [x] Task: Change the docs")
	require.NotContains(t, plan, "- [ ] Task:")

	// status for the epic reports alpha finished and waiting for its merge,
	// its code in the project worktree.
	got := statusOf(t, epicFlowEpic, "--format", "json")
	impl := statusSpecs(t, got)["alpha"]["run"].(map[string]any)["implement"].(map[string]any)
	require.Equal(t, "awaiting_merge", impl["state"])
	require.Equal(t, f.wt("testproj"), impl["root"])
	require.Equal(t, float64(1), got["epic"].(map[string]any)["run"].(map[string]any)["implement"].(map[string]any)["awaiting_merge"])

	// The code was committed on spek/alpha in each worktree, and nothing of
	// Spektacular's reached any worktree.
	for wt, code := range map[string]string{f.wt("testproj"): "main.txt", f.wt("docs"): "lib.txt"} {
		require.Equal(t, "spek/alpha", gittest.RunGit(t, wt, "rev-parse", "--abbrev-ref", "HEAD"), wt)
		require.Equal(t, code, changedIn(t, wt, bases[wt]), wt)
		require.Empty(t, gittest.RunGit(t, wt, "status", "--porcelain", "--untracked-files=all"), wt)
		requireSpektacularAtBase(t, wt, bases[wt])
	}

	// The merge leaves the main project's artifacts byte for byte as they
	// were.
	artifacts := []string{
		filepath.Join(f.proj, ".spektacular", "plans", "alpha", "plan.md"),
		filepath.Join(f.proj, ".spektacular", "changelog", "alpha.md"),
		filepath.Join(f.proj, ".spektacular", "specs", "alpha.md"),
	}
	before := map[string][]byte{}
	for _, p := range artifacts {
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		before[p] = b
	}
	require.Contains(t, string(before[artifacts[1]]), "document_status: final")

	stdout, code := runEpic(t, "merge", "--data", `{"spec":"alpha"}`)
	require.Equalf(t, 0, code, "epic merge failed: %s", stdout)

	for _, p := range artifacts {
		after, err := os.ReadFile(p)
		require.NoError(t, err)
		require.Equal(t, string(before[p]), string(after), p)
	}
	require.Equal(t, plan, epicFlowRead(t, "plan", "file", "read", "alpha", "plan"))

	// The code is on main in both repos; both main checkouts are clean, and
	// the merge commit changed nothing under .spektacular in either.
	for dir, want := range map[string][2]string{
		f.proj: {"main.txt", "main v2\n"},
		f.site: {"lib.txt", "lib v2\n"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, want[0]))
		require.NoError(t, err)
		require.Equal(t, want[1], string(b), dir)
		require.Equal(t, "main", gittest.RunGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"), dir)
		require.Empty(t, gittest.RunGit(t, dir, "status", "--porcelain"), dir)
		require.Equal(t, want[0], gittest.RunGit(t, dir, "diff", "HEAD^1", "HEAD", "--name-only"), dir)
	}
	require.NoDirExists(t, f.wt("testproj"))
	require.NoDirExists(t, f.wt("docs"))
}

// The merge guard, reached through the flow: an implement run from the main
// root, then a stray commit under the sibling worktree's .spektacular on the
// spec branch. The merge is refused as epic_merge_touches_spektacular and
// neither main checkout moves.
func TestEpicFlow_SpektacularChangeOnTheSpecBranchBlocksTheMerge(t *testing.T) {
	f, _ := epicFlowProject(t)
	epicFlowImplement(t)
	wtWriteFile(t, f.wt("testproj"), "main.txt", "main v2\n")
	wtWriteFile(t, f.wt("docs"), "lib.txt", "lib v2\n")
	epicFlowTickPlan(t, f.proj)
	epicFlowWrite(t, f.proj, "# alpha\n\nwhat was built\n", "changelog", "file", "write", "alpha")
	epicFlowWalkToFinished(t, f.proj)

	wtWriteFile(t, f.wt("docs"), ".spektacular/knowledge/stray.md", "a stray entry\n")
	wtCommitAll(t, f.wt("docs"), "stray knowledge")
	projHead := gittest.RunGit(t, f.proj, "rev-parse", "HEAD")
	siteHead := gittest.RunGit(t, f.site, "rev-parse", "HEAD")

	er := refuseEpic(t, "merge", "--data", `{"spec":"alpha"}`)
	require.Equal(t, "epic_merge_touches_spektacular", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.Message, "docs: .spektacular/knowledge/stray.md")
	require.NotContains(t, er.Message, "testproj")

	require.Equal(t, projHead, gittest.RunGit(t, f.proj, "rev-parse", "HEAD"))
	require.Equal(t, siteHead, gittest.RunGit(t, f.site, "rev-parse", "HEAD"))
	for _, dir := range []string{f.proj, f.site} {
		require.Empty(t, gittest.RunGit(t, dir, "status", "--porcelain"), dir)
	}
	main, err := os.ReadFile(filepath.Join(f.proj, "main.txt"))
	require.NoError(t, err)
	require.Equal(t, "main v1\n", string(main))
	require.DirExists(t, f.wt("testproj"))
	require.DirExists(t, f.wt("docs"))
}
