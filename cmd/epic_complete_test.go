package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file tests the guard on adding a spec to a completed epic
// (refuseCompletedEpic in cmd/epic_link.go), through each of the three routes
// that add one: `epic write` gaining a spec, `epic split` extending an
// existing epic, and `spec new` with an epic. An epic is complete when it has
// at least one spec and every spec has a plan whose tasks are all ticked.
// Adding to it is refused with nothing written until the caller confirms with
// "confirm_completed_epic": true; once confirmed, the epic reads as not done
// again. An epic that is not complete needs no confirmation.
//
// Commands are driven through resetRootCmd + runRootCmd, fixtures are written
// with os.WriteFile, and a refusal is shown to write nothing by a byte
// snapshot of the whole project tree.

// The completed-epic fixtures: the member spec, implemented, and a standalone
// spec the routes try to add.
const (
	doneMember = "000010_a"
	newcomer   = "000011_b"
)

// writeMemberPlan stores a plan for spec name with one task, ticked or not.
func writeMemberPlan(t *testing.T, root, name string, ticked bool) {
	t.Helper()
	path := filepath.Join(root, ".spektacular", "plans", name, "plan.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	body := "---\ncreated_date: 2026-09-29\ndocument_status: final\nspec: " + name + "\n---\n\n" +
		taskPlanDoc(taskBlock("Only task", ticked, agentFields(idA), "- [x] done"))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// completeEpicProject is a project whose epic testEpic holds one spec with a
// plan whose every task is ticked, plus a standalone spec. With complete
// false the member's task is left unticked, so the epic is not complete.
func completeEpicProject(t *testing.T, complete bool) string {
	t.Helper()
	root := epicProject(t)
	writeSpecFixture(t, root, doneMember, epicTestSpecFixed)
	writeSpecFixture(t, root, newcomer, epicTestSpecFixed)
	epicWrite(t, testEpic, specsData(doneMember))
	writeMemberPlan(t, root, doneMember, complete)
	require.Equal(t, complete, epicDone(t, testEpic), "fixture: the epic's completion")
	return root
}

// epicDone reads `status <epic> --format json` and returns epic.done.
func epicDone(t *testing.T, name string) bool {
	t.Helper()
	resetRootCmd(t)
	got := statusOf(t, name, "--format", "json")
	e, ok := got["epic"].(map[string]any)
	require.Truef(t, ok, "status %s reports no epic: %v", name, got)
	done, ok := e["done"].(bool)
	require.True(t, ok, "epic.done must be a boolean")
	return done
}

// requireCompletedEpicRefusal asserts er is the completed-epic warning: it
// names the epic and tells the agent to ask the user before retrying with
// the confirmation.
func requireCompletedEpicRefusal(t *testing.T, er output.ErrorResponse) {
	t.Helper()
	require.Equal(t, "epic_complete", er.Code)
	require.Equal(t, testEpic, er.Resource)
	require.Contains(t, er.Message, "epic "+testEpic+" is complete")
	require.Contains(t, er.Message, "every spec is implemented")
	require.Contains(t, er.NextAction, "ask the user")
	require.Contains(t, er.NextAction, "only if they agree")
	require.Contains(t, er.NextAction, `"confirm_completed_epic": true`)
}

// splitAddingC is an `epic split` description that narrows doneMember and
// splits off a new spec "c" in the epic doneMember belongs to.
func splitAddingC(confirm bool) map[string]any {
	desc := map[string]any{
		"spec": doneMember,
		"specs": []map[string]any{
			{"name": doneMember, "depends_on": []string{}, "body": sb{"overview": "Narrowed a.", "acceptance_criteria": []string{"a works"}}},
			{"title": "c", "depends_on": []string{doneMember}, "body": sb{"overview": "Split-off c.", "acceptance_criteria": []string{"c works"}}},
		},
	}
	if confirm {
		desc["confirm_completed_epic"] = true
	}
	return desc
}

// Criterion: adding a spec to a completed epic by `epic write` is refused with
// a warning naming the epic and nothing is written; with confirmation the
// spec is added and the epic is no longer done.
func TestEpicComplete_EpicWriteNeedsConfirmation(t *testing.T) {
	root := completeEpicProject(t, true)
	body := epicBodyFile(t)
	before := snapshotTree(t, root)

	er := refuseEpic(t, "write", testEpic, "--from", body, "--data", specsData(doneMember, newcomer))
	requireCompletedEpicRefusal(t, er)
	require.Contains(t, er.NextAction, "epic write "+testEpic)
	require.Contains(t, er.NextAction, "--data")
	require.Equal(t, before, snapshotTree(t, root), "a refused addition writes nothing")

	got := epicWrite(t, testEpic, `{"specs":[{"name":"`+doneMember+`","depends_on":[]},{"name":"`+newcomer+`","depends_on":[]}],"confirm_completed_epic":true}`)
	require.Equal(t, []string{newcomer}, got.Linked)
	require.Equal(t, []string{doneMember, newcomer}, epicSpecsOf(t, epicFilePath(root, testEpic)))
	requireAgreement(t, root, testEpic, doneMember, newcomer)
	require.False(t, epicDone(t, testEpic), "a confirmed addition reopens the epic")
}

// An `epic write` to a completed epic that adds no spec (rewriting the body,
// or dropping a spec) is not an addition and needs no confirmation.
func TestEpicComplete_EpicWriteWithoutAdditionIsNotGuarded(t *testing.T) {
	completeEpicProject(t, true)
	got := epicWrite(t, testEpic, specsData(doneMember))
	require.Empty(t, got.Linked)
}

// Criterion: extending a completed epic by `epic split` is refused with
// nothing written; with confirmation in the staged description the split
// lands and the epic is no longer done.
func TestEpicComplete_EpicSplitNeedsConfirmation(t *testing.T) {
	root := completeEpicProject(t, true)
	staged := stageSplit(t, splitAddingC(false))
	before := snapshotTree(t, root)

	er := refuseEpic(t, "split", "--from", staged)
	requireCompletedEpicRefusal(t, er)
	require.Contains(t, er.NextAction, "epic split")
	require.Contains(t, er.NextAction, "staged description")
	require.Equal(t, before, snapshotTree(t, root), "a refused split writes nothing")

	got := epicSplit(t, splitAddingC(true))
	require.Equal(t, testEpic, got.Epic)
	require.Equal(t, []string{"000012_c"}, got.Created)
	require.Equal(t, []string{doneMember, "000012_c"}, epicSpecsOf(t, epicFilePath(root, testEpic)))
	require.False(t, epicDone(t, testEpic), "a confirmed split reopens the epic")
}

// Criterion: starting a spec in a completed epic with `spec new` is refused
// with nothing written (no spec, no workflow state); with confirmation the
// spec joins and the epic is no longer done.
func TestEpicComplete_SpecNewNeedsConfirmation(t *testing.T) {
	root := completeEpicProject(t, true)
	before := snapshotTree(t, root)

	_, err := runSpecNewForTest(t, "--data", `{"name":"joiner","epic":"`+testEpic+`"}`)
	var cliErr *output.ErrorResponse
	require.ErrorAs(t, err, &cliErr)
	requireCompletedEpicRefusal(t, *cliErr)
	require.Contains(t, cliErr.NextAction, "spec new")
	require.Contains(t, cliErr.NextAction, "--data")
	require.Equal(t, before, snapshotTree(t, root), "a refused spec new writes nothing")
	require.NoFileExists(t, filepath.Join(root, ".spektacular", "state.json"))

	result, err := runSpecNewForTest(t, "--data", `{"name":"joiner","epic":"`+testEpic+`","confirm_completed_epic":true}`)
	require.NoError(t, err)
	require.Equal(t, []string{doneMember, result.SpecName}, epicSpecsOf(t, epicFilePath(root, testEpic)))
	requireAgreement(t, root, testEpic, doneMember, result.SpecName)
	require.False(t, epicDone(t, testEpic), "a confirmed spec new reopens the epic")
}

// Criterion: adding to an epic that is not complete needs no confirmation, by
// any route. "Not complete" covers a member with an unticked task and an epic
// with no specs at all.
func TestEpicComplete_IncompleteEpicNeedsNoConfirmation(t *testing.T) {
	t.Run("epic write", func(t *testing.T) {
		root := completeEpicProject(t, false)
		got := epicWrite(t, testEpic, specsData(doneMember, newcomer))
		require.Equal(t, []string{newcomer}, got.Linked)
		requireAgreement(t, root, testEpic, doneMember, newcomer)
	})

	t.Run("epic split", func(t *testing.T) {
		root := completeEpicProject(t, false)
		got := epicSplit(t, splitAddingC(false))
		require.Equal(t, []string{"000012_c"}, got.Created)
		require.Equal(t, []string{doneMember, "000012_c"}, epicSpecsOf(t, epicFilePath(root, testEpic)))
	})

	t.Run("spec new", func(t *testing.T) {
		root := completeEpicProject(t, false)
		result, err := runSpecNewForTest(t, "--data", `{"name":"joiner","epic":"`+testEpic+`"}`)
		require.NoError(t, err)
		require.Equal(t, []string{doneMember, result.SpecName}, epicSpecsOf(t, epicFilePath(root, testEpic)))
	})

	t.Run("an epic with no specs is not complete", func(t *testing.T) {
		root := epicProject(t)
		epicWrite(t, testEpic, "")
		require.False(t, epicDone(t, testEpic))
		result, err := runSpecNewForTest(t, "--data", `{"name":"joiner","epic":"`+testEpic+`"}`)
		require.NoError(t, err)
		require.Equal(t, []string{result.SpecName}, epicSpecsOf(t, epicFilePath(root, testEpic)))
	})
}

// `spec new --schema` publishes the confirmation key as a boolean.
func TestEpicComplete_SpecNewSchemaPublishesConfirmation(t *testing.T) {
	schema := runSpecNewSchemaForTest(t)
	require.Contains(t, schema.Input.Properties, "confirm_completed_epic")
	require.Equal(t, "boolean", schema.Input.Properties["confirm_completed_epic"].Type)
}
