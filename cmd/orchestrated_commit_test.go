package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// These tests cover an orchestrated plan's completion commit: it carries the
// plan's own files and nothing of another plan running beside it in the same
// working copy, a failure rolls back only that plan's lane, and finished lanes
// are removed. Standalone and implement-lane commits keep committing the
// whole tree. Paths and expected commit contents are hand-written.

// planStepsToWalkthrough are the plan steps a lane walks before its
// completion commit, in order.
var planStepsToWalkthrough = []string{
	"overview", "discovery", "architecture", "components", "data_structures",
	"implementation_detail", "dependencies", "testing_approach", "milestones",
	"tasks", "open_questions", "out_of_scope", "assemble", "verification",
	"write_plan", "write_context", "write_research", "walkthrough",
}

// startPlanLane starts an orchestrated plan lane for name.
func startPlanLane(t *testing.T, name string) {
	t.Helper()
	runOK(t, "plan", "new", "--data", `{"name":"`+name+`","orchestrated":true}`)
}

// walkPlanLane drives the lane for name through steps.
func walkPlanLane(t *testing.T, name string, steps ...string) {
	t.Helper()
	for _, s := range steps {
		runOK(t, "plan", "goto", "--data", `{"step":"`+s+`","name":"`+name+`"}`)
	}
}

// writeLaneWork writes what an agent running the lane for name leaves in the
// tree: its three plan documents (through the store), a per-section working
// file, a scratch file and its lane notes.
func writeLaneWork(t *testing.T, root, name string) {
	t.Helper()
	for _, doc := range []string{"plan", "context", "research"} {
		src := filepath.Join(t.TempDir(), doc+".md")
		require.NoError(t, os.WriteFile(src, []byte("# "+doc+" for "+name+"\n\n## Real content\n"), 0o644))
		runOK(t, "plan", "file", "write", name, doc, "--from", src)
	}
	for rel, body := range map[string]string{
		".spektacular/work/" + name + "/overview.md":  "overview of " + name + "\n",
		".spektacular/tmp/" + name + "/scratch.md":    "scratch for " + name + "\n",
		".spektacular/workflows/plan-" + name + ".md": "notes for " + name + "\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
}

// stageLaneMessage stages body where the lane for name is told to stage its
// commit message.
func stageLaneMessage(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, ".spektacular", "tmp", name, "git-commit-message.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// finishLaneData is the goto --data finishing the plan lane for name with its
// staged message.
func finishLaneData(name string) string {
	return `{"step":"finished","name":"` + name + `","commit_message_from":".spektacular/tmp/` + name + `/git-commit-message.md"}`
}

// headFiles is HEAD's changes as git's name-status lines.
func headFiles(t *testing.T, dir string) string {
	t.Helper()
	return gittest.RunGit(t, dir, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD")
}

// twoPlanLanes builds a git project with auto_commit on and two orchestrated
// plan lanes, alpha and beta, whose lane files are committed in the baseline
// (as an earlier commit would have left them) and which then both reach
// walkthrough with their work written and uncommitted.
func twoPlanLanes(t *testing.T) gitFixture {
	t.Helper()
	fx := gitProject(t, config.AutoCommitWorkflow)
	startPlanLane(t, laneAlpha)
	startPlanLane(t, laneBeta)
	commitFixtures(t, fx)

	for _, name := range []string{laneAlpha, laneBeta} {
		walkPlanLane(t, name, planStepsToWalkthrough...)
		writeLaneWork(t, fx.root, name)
	}
	return fx
}

// The fixture lanes' names carry the counter id prefix the fixture project's
// spec.id_method requires of a plan name.
const (
	laneAlpha = "000001_alpha"
	laneBeta  = "000002_beta"
)

// laneCommitFiles is what name's completion commit holds: its plan
// documents, scratch and working files, and its lane state file's deletion
// (its notes were never committed).
func laneCommitFiles(name string) string {
	return "A\t.spektacular/plans/" + name + "/context.md\n" +
		"A\t.spektacular/plans/" + name + "/plan.md\n" +
		"A\t.spektacular/plans/" + name + "/research.md\n" +
		"A\t.spektacular/tmp/" + name + "/scratch.md\n" +
		"A\t.spektacular/work/" + name + "/overview.md\n" +
		"D\t.spektacular/workflows/plan-" + name + ".json"
}

// Criteria (a) and (b): with two plan lanes in progress, finishing one
// commits only its own files — the other's plan, work and lane files stay
// uncommitted — and finishing the second then commits its own.
func TestOrchestratedPlanCommit_ContainsOnlyItsOwnFiles(t *testing.T) {
	fx := twoPlanLanes(t)
	dataDir := filepath.Join(fx.root, config.ProjectConfigDirName)
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, "unrelated.txt"), []byte("user work\n"), 0o644))

	stageLaneMessage(t, fx.root, laneAlpha, "Plan "+laneAlpha+"\n\nThe alpha plan is complete.\n")
	runOK(t, "plan", "goto", "--data", finishLaneData(laneAlpha))

	require.Equal(t, "2", commitCount(t, fx.root))
	require.Equal(t, "Plan 000001_alpha\n\nThe alpha plan is complete.", gittest.RunGit(t, fx.root, "log", "-1", "--format=%B"))
	require.Equal(t, laneCommitFiles(laneAlpha), headFiles(t, fx.root))
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "plan-000001_alpha.json"))
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "plan-000001_alpha.md"))

	// Beta's work, its lane and the user's file are untouched and unstaged.
	require.Empty(t, gittest.RunGit(t, fx.root, "diff", "--cached", "--name-only"))
	require.Equal(t,
		"M .spektacular/workflows/plan-000002_beta.json\n"+
			"?? .spektacular/plans/000002_beta/\n"+
			"?? .spektacular/tmp/000002_beta/\n"+
			"?? .spektacular/work/000002_beta/\n"+
			"?? .spektacular/workflows/plan-000002_beta.md\n"+
			"?? unrelated.txt",
		gittest.RunGit(t, fx.root, "status", "--porcelain"))
	require.Equal(t, "walkthrough", laneState(t, dataDir, "plan", laneBeta).CurrentStep)

	stageLaneMessage(t, fx.root, laneBeta, "Plan "+laneBeta+"\n\nThe beta plan is complete.\n")
	runOK(t, "plan", "goto", "--data", finishLaneData(laneBeta))

	require.Equal(t, "3", commitCount(t, fx.root))
	require.Equal(t, laneCommitFiles(laneBeta), headFiles(t, fx.root))
	require.Equal(t, "?? unrelated.txt", gittest.RunGit(t, fx.root, "status", "--porcelain"))
}

// Criterion (c): a commit git refuses puts back only the finishing lane — its
// state at walkthrough and its notes restored — while the other lane's files
// stay byte-identical; once the cause is fixed the identical goto succeeds.
func TestOrchestratedPlanCommit_FailureRestoresOnlyItsOwnLane(t *testing.T) {
	fx := twoPlanLanes(t)
	dataDir := filepath.Join(fx.root, config.ProjectConfigDirName)

	hook := filepath.Join(fx.root, ".git", "hooks", "pre-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint-failed-in-hook' >&2\nexit 1\n"), 0o755))

	alphaNotes := filepath.Join(dataDir, "workflows", "plan-000001_alpha.md")
	betaState := filepath.Join(dataDir, "workflows", "plan-000002_beta.json")
	betaNotes := filepath.Join(dataDir, "workflows", "plan-000002_beta.md")
	betaStateBefore := readBytes(t, betaState)
	betaNotesBefore := readBytes(t, betaNotes)

	stageLaneMessage(t, fx.root, laneAlpha, "Plan "+laneAlpha+"\n\nThe alpha plan is complete.\n")
	er := runRefused(t, "plan", "goto", "--data", finishLaneData(laneAlpha))
	require.Equal(t, "auto_commit_failed", er.Code)
	require.Contains(t, er.Message, "lint-failed-in-hook")
	require.Equal(t,
		`fix the cause git reported above (a failing hook, for example), `+
			`re-stage the message at .spektacular/tmp/000001_alpha/git-commit-message.md, `+
			`then re-run: spektacular plan goto --data '{"step":"finished","name":"000001_alpha","commit_message_from":".spektacular/tmp/000001_alpha/git-commit-message.md"}'`,
		er.NextAction)

	require.Equal(t, "1", commitCount(t, fx.root))
	require.Equal(t, "walkthrough", laneState(t, dataDir, "plan", laneAlpha).CurrentStep)
	require.Equal(t, "notes for "+laneAlpha+"\n", string(readBytes(t, alphaNotes)))
	require.Equal(t, betaStateBefore, readBytes(t, betaState))
	require.Equal(t, betaNotesBefore, readBytes(t, betaNotes))
	require.NotContains(t, gittest.RunGit(t, fx.root, "diff", "--cached", "--name-only"), laneBeta,
		"nothing of the other lane may be staged")

	require.NoError(t, os.Remove(hook))
	stageLaneMessage(t, fx.root, laneAlpha, "Plan "+laneAlpha+"\n\nThe alpha plan is complete.\n")
	runOK(t, "plan", "goto", "--data", finishLaneData(laneAlpha))

	require.Equal(t, "2", commitCount(t, fx.root))
	// The failed attempt left alpha's lane deletion staged; the retry's
	// commit must still carry it rather than leave it behind in the index.
	require.Equal(t, laneCommitFiles(laneAlpha), headFiles(t, fx.root))
	require.Empty(t, gittest.RunGit(t, fx.root, "diff", "--cached", "--name-only"))
	require.Equal(t, betaStateBefore, readBytes(t, betaState))
}

// Issue #80: a plan lane's completion commit still succeeds when its scratch
// folder holds nothing but the staged commit message — so it is empty once
// the message is removed — and when the scratch folder is git-ignored. The
// lane's own plan, work and lane files are committed either way.
func TestOrchestratedPlanCommit_ScratchFolderEmptyOrIgnored(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T, root string){
		"holds only the message": func(t *testing.T, root string) {
			require.NoError(t, os.RemoveAll(filepath.Join(root, ".spektacular", "tmp", laneAlpha)))
		},
		"is git-ignored": func(t *testing.T, root string) {
			require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".spektacular/tmp/\n"), 0o644))
			gittest.RunGit(t, root, "add", ".gitignore")
			gittest.RunGit(t, root, "commit", "-q", "-m", "ignore scratch")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fx := gitProject(t, config.AutoCommitWorkflow)
			startPlanLane(t, laneAlpha)
			commitFixtures(t, fx)
			walkPlanLane(t, laneAlpha, planStepsToWalkthrough...)
			writeLaneWork(t, fx.root, laneAlpha)
			setup(t, fx.root)
			before := gittest.RunGit(t, fx.root, "rev-parse", "HEAD")

			stageLaneMessage(t, fx.root, laneAlpha, "Plan "+laneAlpha+"\n\nThe alpha plan is complete.\n")
			runOK(t, "plan", "goto", "--data", finishLaneData(laneAlpha))

			require.Equal(t, before, gittest.RunGit(t, fx.root, "rev-parse", "HEAD~1"))
			require.Equal(t,
				"A\t.spektacular/plans/"+laneAlpha+"/context.md\n"+
					"A\t.spektacular/plans/"+laneAlpha+"/plan.md\n"+
					"A\t.spektacular/plans/"+laneAlpha+"/research.md\n"+
					"A\t.spektacular/work/"+laneAlpha+"/overview.md\n"+
					"D\t.spektacular/workflows/plan-"+laneAlpha+".json",
				headFiles(t, fx.root))
			require.Empty(t, gittest.RunGit(t, fx.root, "status", "--porcelain"))
		})
	}
}

// Criterion (d): a standalone plan's completion commit still takes the whole
// tree, a file unrelated to the plan included.
func TestStandalonePlanCommit_StillCommitsTheWholeTree(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	runOK(t, "plan", "new", "--data", `{"name":"billing"}`)
	walkSteps(t, "plan", planStepsToWalkthrough[1:]...)
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, "unrelated.txt"), []byte("user work\n"), 0o644))

	stageCommitMessage(t, fx, "Plan billing\n\nThe implementation plan for billing is complete.\n")
	runOK(t, "plan", "goto", "--data", finishWithMessage)

	require.Equal(t, "2", commitCount(t, fx.root))
	require.Contains(t, strings.Split(headFiles(t, fx.root), "\n"), "A\tunrelated.txt")
	require.Empty(t, gittest.RunGit(t, fx.root, "status", "--porcelain"))
	require.Equal(t, "finished", currentStep(t, fx))
}

// Criterion (d): an orchestrated implement's completion commit still takes
// the whole tree, and its finished lane's files are gone from the tree.
func TestOrchestratedImplementCommit_StillCommitsTheWholeTree(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	dataDir := filepath.Join(fx.root, config.ProjectConfigDirName)
	writeFixturePlan(t, dataDir, "billing")
	changelog := filepath.Join(dataDir, config.DefaultChangelogDir, "billing.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(changelog), 0o755))
	require.NoError(t, os.WriteFile(changelog, []byte("# billing\n\nwhat was built\n"), 0o644))
	commitFixtures(t, fx)

	runOK(t, "implement", "new", "--data", `{"name":"billing","orchestrated":true}`)
	for _, s := range []string{
		"analyze", "implement", "test", "verify", "update_plan", "update_changelog",
		"test_plan", "update_feature_changelog", "reconcile_spec",
	} {
		runOK(t, "implement", "goto", "--data", `{"step":"`+s+`","name":"billing"}`)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "workflows", "implement-billing.md"), []byte("notes\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, "unrelated.txt"), []byte("user work\n"), 0o644))

	stageLaneMessage(t, fx.root, "billing", "Implement billing\n\nThe billing plan is built.\n")
	runOK(t, "implement", "goto", "--data",
		`{"step":"finished","name":"billing","commit_message_from":".spektacular/tmp/billing/git-commit-message.md"}`)

	require.Equal(t, "2", commitCount(t, fx.root))
	require.Contains(t, strings.Split(headFiles(t, fx.root), "\n"), "A\tunrelated.txt")
	require.Empty(t, gittest.RunGit(t, fx.root, "status", "--porcelain"))
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "implement-billing.json"))
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "implement-billing.md"))
}

// With auto_commit off a finished plan lane's files are still removed, and
// no commit is made.
func TestOrchestratedPlanFinish_RemovesLaneWithAutoCommitOff(t *testing.T) {
	fx := gitProject(t, config.AutoCommitOff)
	dataDir := filepath.Join(fx.root, config.ProjectConfigDirName)
	startPlanLane(t, laneAlpha)
	walkPlanLane(t, laneAlpha, planStepsToWalkthrough...)
	writeLaneWork(t, fx.root, laneAlpha)

	runOK(t, "plan", "goto", "--data", `{"step":"finished","name":"`+laneAlpha+`"}`)

	require.Equal(t, "1", commitCount(t, fx.root))
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "plan-000001_alpha.json"))
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "plan-000001_alpha.md"))
	require.FileExists(t, filepath.Join(dataDir, "plans", laneAlpha, "plan.md"))
}
