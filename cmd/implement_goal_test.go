package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// goalPlan is a plan of two tasks; task A has three criteria, task B one.
func goalPlan(aDone bool) string {
	return taskPlanDoc(
		taskBlock("Task A", aDone, agentFields(idA), "- [ ] a works", "- [ ] a is tested", "- [ ] the README documents a") +
			taskBlock("Task B", false, agentFields(idB, idA+" — Task A"), "- [ ] b works"),
	)
}

const goalContext = "# Context\n\n### Task: Task A\n\n- `app/a.go` — the code\n- `README.md` — the docs\n"

// legacyGoalPlan is a plan written before tasks.
const legacyGoalPlan = "# Plan: " + taskPlanName + "\n\n## Overview\n\nfixture\n\n## Milestones & Phases\n\n" +
	"### Milestone 1: One\n\n#### - [x] Phase 1.1: Done\n\n#### - [ ] Phase 1.2: Do it\n\n" +
	"**Acceptance criteria**:\n- [ ] it is done\n- [ ] it is tested\n"

// setupGoalProject makes a project in a git repo, writes the plan and its
// context and starts a whole-plan implement run.
func setupGoalProject(t *testing.T, plan string) string {
	t.Helper()
	gittest.RequireGit(t)
	dir := t.TempDir()
	t.Chdir(dir)
	gittest.RunGit(t, dir, "init", "-q")
	gittest.RunGit(t, dir, "config", "user.email", "t@example.com")
	gittest.RunGit(t, dir, "config", "user.name", "t")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# app\n"), 0o644))
	gittest.RunGit(t, dir, "add", "README.md")
	gittest.RunGit(t, dir, "commit", "-q", "-m", "init")

	writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
	stdout, code := writePlanDoc(t, taskPlanName, "plan", plan)
	require.Equal(t, 0, code, stdout)
	stdout, code = writePlanDoc(t, taskPlanName, "context", goalContext)
	require.Equal(t, 0, code, stdout)
	stdout, _, code = runRootCmd(t, "implement", "new", "--data", `{"name":"`+taskPlanName+`"}`)
	require.Equal(t, 0, code, stdout)
	return dir
}

// stageWork writes the work's diff and status where check_goal stages them.
func stageWork(t *testing.T, dir string) {
	t.Helper()
	stage := filepath.Join(dir, ".spektacular", "tmp", taskPlanName, "goal-check")
	require.NoError(t, os.MkdirAll(stage, 0o755))
	diff := gittest.RunGit(t, dir, "diff", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(stage, "testproj.diff"), []byte(diff), 0o644))
	status := gittest.RunGit(t, dir, "status", "--porcelain", "--untracked-files=all")
	require.NoError(t, os.WriteFile(filepath.Join(stage, "testproj.status"), []byte(status), 0o644))
}

// stageReply writes a reviewer reply and returns its path.
func stageReply(t *testing.T, dir, reply string) string {
	t.Helper()
	path := filepath.Join(dir, ".spektacular", "tmp", taskPlanName, "goal-check", "reply.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(reply), 0o644))
	return path
}

func reply(verdicts ...string) string {
	type v struct {
		Criterion int    `json:"criterion"`
		Verdict   string `json:"verdict"`
		Evidence  string `json:"evidence"`
	}
	out := struct {
		Verdicts []v `json:"verdicts"`
	}{}
	for i, verdict := range verdicts {
		out.Verdicts = append(out.Verdicts, v{i + 1, verdict, "looked at criterion " + string(rune('1'+i))})
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

type goalRecordResult struct {
	Plan          string `json:"plan"`
	Item          string `json:"item"`
	Round         int    `json:"round"`
	Attempt       int    `json:"attempt"`
	Decision      string `json:"decision"`
	State         string `json:"state"`
	RetriesLeft   int    `json:"retries_left"`
	Met           []int  `json:"met"`
	NotMet        []int  `json:"not_met"`
	Impossible    []int  `json:"impossible"`
	TreeUnchanged *bool  `json:"tree_unchanged"`
	Warning       string `json:"warning"`
	NextStep      string `json:"next_step"`
	NextAction    string `json:"next_action"`
}

func recordGoal(t *testing.T, dir, item, replyBody string) (goalRecordResult, string, int) {
	t.Helper()
	path := stageReply(t, dir, replyBody)
	stdout, _, code := runRootCmd(t, "implement", "goal", "record",
		"--data", `{"name":"`+taskPlanName+`","item":"`+item+`"}`, "--from", path)
	var res goalRecordResult
	if code == 0 {
		require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	}
	return res, stdout, code
}

func decideGoal(t *testing.T, item, decision string) (goalRecordResult, string, int) {
	t.Helper()
	stdout, _, code := runRootCmd(t, "implement", "goal", "decide",
		"--data", `{"name":"`+taskPlanName+`","item":"`+item+`","decision":"`+decision+`","reason":"because"}`)
	var res goalRecordResult
	if code == 0 {
		require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	}
	return res, stdout, code
}

func TestImplementGoalRecord_ValidReplyRecordsOneVerdictPerCriterionAndNamesTheNextStep(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))
	goalNow = func() time.Time { return time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { goalNow = time.Now })

	stageWork(t, dir)
	res, stdout, code := recordGoal(t, dir, idA, reply("met", "met", "not_met"))
	require.Equal(t, 0, code, stdout)
	require.Equal(t, "retry", res.Decision)
	require.Equal(t, "retrying", res.State)
	require.Equal(t, 1, res.Round)
	require.Equal(t, 1, res.Attempt)
	require.Equal(t, 2, res.RetriesLeft)
	require.Equal(t, []int{1, 2}, res.Met)
	require.Equal(t, []int{3}, res.NotMet)
	require.Equal(t, []int{}, res.Impossible)
	require.Equal(t, "implement", res.NextStep)
	require.Equal(t, `spektacular implement goto --data '{"step":"implement","name":"`+taskPlanName+`"}'`, res.NextAction)
	require.NotNil(t, res.TreeUnchanged)
	require.True(t, *res.TreeUnchanged, res.Warning)

	stageWork(t, dir)
	res, stdout, code = recordGoal(t, dir, idA, reply("met", "met", "met"))
	require.Equal(t, 0, code, stdout)
	require.Equal(t, "accept", res.Decision)
	require.Equal(t, 2, res.Attempt, "the recorded attempt count rises")
	require.Equal(t, "update_plan", res.NextStep)
	require.Contains(t, res.NextAction, `"step":"update_plan"`)

	stdout, _, code = runRootCmd(t, "implement", "goal", "show", taskPlanName, "--format", "json")
	require.Equal(t, 0, code, stdout)
	var shown struct {
		Plan  string `json:"plan"`
		Items []struct {
			Item     string `json:"item"`
			Attempts int    `json:"attempts"`
			State    string `json:"state"`
			Checks   []struct {
				At       string                      `json:"at"`
				Goal     struct{ Criteria []string } `json:"goal"`
				Verdicts []struct {
					Criterion int    `json:"criterion"`
					Verdict   string `json:"verdict"`
					Evidence  string `json:"evidence"`
				} `json:"verdicts"`
				TreeUnchanged bool `json:"tree_unchanged"`
			} `json:"checks"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &shown))
	require.Len(t, shown.Items, 1)
	item := shown.Items[0]
	require.Equal(t, idA, item.Item)
	require.Equal(t, 2, item.Attempts)
	require.Equal(t, "accepted", item.State)
	require.Equal(t, "2026-10-21T10:00:00Z", item.Checks[0].At)
	require.Equal(t, []string{"a works", "a is tested", "the README documents a"}, item.Checks[0].Goal.Criteria)
	for _, c := range item.Checks {
		require.Len(t, c.Verdicts, 3, "N verdicts for N criteria")
		for _, v := range c.Verdicts {
			require.NotEmpty(t, v.Evidence)
		}
		require.True(t, c.TreeUnchanged)
	}
}

func TestImplementGoalRecord_StopsHandBackTheDecideCommand(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))

	res, stdout, code := recordGoal(t, dir, idA, reply("met", "impossible", "not_met"))
	require.Equal(t, 0, code, stdout)
	require.Equal(t, "impossible", res.Decision)
	require.Equal(t, 1, res.Attempt, "no retry is counted")
	require.Equal(t, "awaiting_decision", res.State)
	require.Equal(t, "check_goal", res.NextStep)
	require.Contains(t, res.NextAction, `implement goal decide --data '{"name":"`+taskPlanName+`","item":"`+idA+`","decision":"keep_fixing|accept|skip|recheck"}'`)

	// A further check is refused until the user's answer is recorded.
	_, stdout, code = recordGoal(t, dir, idA, reply("met", "met", "met"))
	require.Equal(t, 1, code)
	require.Equal(t, "goal_decision_due", decodeError(t, stdout).Code)

	res, stdout, code = decideGoal(t, idA, "accept")
	require.Equal(t, 0, code, stdout)
	require.Equal(t, "accepted_shortfall", res.State)
	require.Equal(t, "update_plan", res.NextStep)
	require.Equal(t, []int{1}, res.Met)
	require.Contains(t, res.NextAction, `"step":"update_plan"`)
}

func TestImplementGoalRecord_ExhaustedAfterTwoRetriesThenKeepFixingOpensARound(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))
	for attempt, want := range []string{"retry", "retry", "exhausted"} {
		res, stdout, code := recordGoal(t, dir, idA, reply("met", "met", "not_met"))
		require.Equal(t, 0, code, stdout)
		require.Equal(t, want, res.Decision)
		require.Equal(t, attempt+1, res.Attempt)
	}
	res, stdout, code := decideGoal(t, idA, "keep_fixing")
	require.Equal(t, 0, code, stdout)
	require.Equal(t, "implement", res.NextStep)
	require.Equal(t, "retrying", res.State)

	res, stdout, code = recordGoal(t, dir, idA, reply("met", "met", "not_met"))
	require.Equal(t, 0, code, stdout)
	require.Equal(t, 2, res.Round)
	require.Equal(t, 1, res.Attempt)
	require.Equal(t, "retry", res.Decision)
}

func TestImplementGoalRecord_RefusesMalformedRepliesWithAFix(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))
	cases := map[string]struct {
		reply string
		want  string
	}{
		"missing criterion": {reply("met", "met"), "criterion 3 has no verdict"},
		"extra criterion":   {reply("met", "met", "met", "met"), "names criterion 4"},
		"unknown verdict":   {reply("met", "partly", "met"), `unknown verdict "partly"`},
		"empty evidence":    {`{"verdicts":[{"criterion":1,"verdict":"met","evidence":""},{"criterion":2,"verdict":"met","evidence":"x"},{"criterion":3,"verdict":"met","evidence":"x"}]}`, "criterion 1 has no evidence"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, stdout, code := recordGoal(t, dir, idA, tc.reply)
			require.Equal(t, 1, code)
			er := decodeError(t, stdout)
			require.Equal(t, "goal_reply_invalid", er.Code)
			require.Contains(t, er.Message, tc.want)
			require.Contains(t, er.NextAction, `{"verdicts":[`)
		})
	}

	stdout, _, code := runRootCmd(t, "implement", "goal", "show", taskPlanName, "--format", "json")
	require.Equal(t, 0, code)
	require.Contains(t, stdout, `"items": []`, "a refused reply records nothing")
}

func TestImplementGoalRecord_RefusesWithoutFromOrForAnotherItem(t *testing.T) {
	setupGoalProject(t, goalPlan(false))

	stdout, _, code := runRootCmd(t, "implement", "goal", "record", "--data", `{"name":"`+taskPlanName+`","item":"`+idA+`"}`)
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "from_required", er.Code)
	require.Contains(t, er.NextAction, ".spektacular/tmp/"+taskPlanName+"/goal-check/reply.json")

	dir, _ := os.Getwd()
	_, stdout, code = recordGoal(t, dir, idB, reply("met"))
	require.Equal(t, 1, code)
	er = decodeError(t, stdout)
	require.Equal(t, "goal_item_not_current", er.Code)
	require.Contains(t, er.Message, "the current item is "+idA)
	require.Contains(t, er.NextAction, `"step":"check_goal"`)

	_, stdout, code = recordGoal(t, dir, "nope", reply("met"))
	require.Equal(t, 1, code)
	er = decodeError(t, stdout)
	require.Equal(t, "goal_item_not_found", er.Code)
	require.Contains(t, er.NextAction, "plan file read "+taskPlanName+" plan")
}

func TestImplementGoalDecide_RefusedWhenNotDueOrUnknown(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))

	_, stdout, code := decideGoal(t, idA, "accept")
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "goal_decision_not_due", er.Code)
	require.Contains(t, er.NextAction, "implement goal show "+taskPlanName)

	_, _, code = recordGoal(t, dir, idA, reply("impossible", "met", "met"))
	require.Equal(t, 0, code)
	_, stdout, code = decideGoal(t, idA, "give_up")
	require.Equal(t, 1, code)
	er = decodeError(t, stdout)
	require.Equal(t, "goal_decision_invalid", er.Code)
	require.Contains(t, er.NextAction, "keep_fixing, accept, skip, recheck")
	require.Contains(t, er.NextAction, `"name":"`+taskPlanName+`"`)
}

func TestImplementGoal_SwitchedOffRefusesAndSaysHowToSwitchOn(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))
	writeSpecCommandConfig(t, dir, "implement:\n  worktrees: false\n  goal_check:\n    enabled: false\n")

	_, stdout, code := recordGoal(t, dir, idA, reply("met", "met", "met"))
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "goal_check_disabled", er.Code)
	require.Contains(t, er.NextAction, "implement.goal_check.enabled: true")

	_, stdout, code = decideGoal(t, idA, "accept")
	require.Equal(t, 1, code)
	require.Equal(t, "goal_check_disabled", decodeError(t, stdout).Code)

	_, _, code = runRootCmd(t, "implement", "goal", "show", taskPlanName)
	require.Equal(t, 0, code, "recorded outcomes stay readable whatever the switch says")
}

func TestImplementGoalShow_PrettyListsGoalVerdictsEvidenceAndAttempts(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))
	_, _, code := recordGoal(t, dir, idA, reply("met", "met", "not_met"))
	require.Equal(t, 0, code)
	_, _, code = recordGoal(t, dir, idA, reply("met", "met", "met"))
	require.Equal(t, 0, code)

	stdout, _, code := runRootCmd(t, "implement", "goal", "show", taskPlanName)
	require.Equal(t, 0, code, stdout)
	for _, want := range []string{
		idA + "  Task A",
		"state: accepted, 2 attempt(s)",
		"1. a works",
		"3. the README documents a",
		"scope: repo testproj; Change only the files that belong to this item",
		"- testproj: app/a.go",
		"round 1, attempt 1",
		"3. not_met: looked at criterion 3",
		"round 1, attempt 2",
		"3. met: looked at criterion 3",
	} {
		require.Contains(t, stdout, want)
	}

	stdout, _, code = runRootCmd(t, "implement", "goal", "show", taskPlanName, "--item", idB, "--format", "json")
	require.Equal(t, 0, code)
	require.Contains(t, stdout, `"state": "pending"`)
	require.NotContains(t, stdout, idA)
}

func TestImplementGoalRecord_LegacyPhaseIsRecordedByItsLabel(t *testing.T) {
	dir := setupGoalProject(t, legacyGoalPlan)
	res, stdout, code := recordGoal(t, dir, "Phase 1.2", reply("met", "met"))
	require.Equal(t, 0, code, stdout)
	require.Equal(t, "Phase 1.2", res.Item)
	require.Equal(t, "accept", res.Decision)

	_, stdout, code = recordGoal(t, dir, "Phase 1.1", reply("met"))
	require.Equal(t, 1, code, "a ticked phase is not the current item")
	require.Equal(t, "goal_item_not_current", decodeError(t, stdout).Code)
}

func TestImplementGoalRecord_TreeUnchangedFalseWhenTheWorkChangedAfterStaging(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# app\n\nwork in progress\n"), 0o644))
	stageWork(t, dir)

	// The reviewer edits a file between staging and recording.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# app\n\nchanged by the reviewer\n"), 0o644))
	res, stdout, code := recordGoal(t, dir, idA, reply("met", "met", "not_met"))
	require.Equal(t, 0, code, "a changed tree is recorded, never refused")
	require.NotNil(t, res.TreeUnchanged)
	require.False(t, *res.TreeUnchanged)
	require.Contains(t, res.Warning, "testproj's diff changed between staging and recording")

	// A new untracked file is caught too, while Spektacular's own files are not.
	stageWork(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644))
	res, _, code = recordGoal(t, dir, idA, reply("met", "met", "not_met"))
	require.Equal(t, 0, code)
	require.False(t, *res.TreeUnchanged)
	require.Contains(t, res.Warning, "testproj's status changed")

	stageWork(t, dir)
	res, _, code = recordGoal(t, dir, idA, reply("met", "met", "met"))
	require.Equal(t, 0, code)
	require.True(t, *res.TreeUnchanged, "the reply and the record itself, under .spektacular, do not count: %s", res.Warning)

	stdout, _, code = runRootCmd(t, "implement", "goal", "show", taskPlanName, "--format", "json")
	require.Equal(t, 0, code)
	require.Equal(t, 2, strings.Count(stdout, `"tree_unchanged": false`))
}

func TestImplementGoalRecord_MissingStagedWorkRecordsTreeUnknown(t *testing.T) {
	dir := setupGoalProject(t, goalPlan(false))
	res, stdout, code := recordGoal(t, dir, idA, reply("met", "met", "met"))
	require.Equal(t, 0, code, stdout)
	require.False(t, *res.TreeUnchanged)
	require.Contains(t, res.Warning, "no staged diff for testproj")
}
