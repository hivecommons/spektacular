package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/stretchr/testify/require"
)

// This file tests the dependency check `implement new` runs before a spec's
// implementation starts (refuseUnmetDependencies in cmd/implement.go). A
// standalone spec, or one whose dependencies in its epic are all implemented,
// starts silently. An unmet dependency refuses the run, naming each one and
// its state, until the caller overrides it with "override_dependencies":
// true; the override is then recorded in the workflow data for the changelog
// steps. Under epic.strict_dependencies no override exists. Specifying and
// planning such a spec are never held back.
//
// Commands are driven through resetRootCmd + runRootCmd; fixtures are written
// with os.WriteFile.

// depProject lays out a project with an epic epic-e:
//
//	dep-a   no dependencies; plan with 2 of 5 tasks complete (in progress, ready)
//	dep-b   depends on dep-a; no plan (unplanned, not ready: dep-a is not implemented)
//	dep-i   no dependencies; plan with every task complete (implemented)
//	spec-x  depends on dep-b then dep-a (both unmet; the first ready one is dep-a)
//	spec-y  depends on dep-b (unmet, and not ready)
//	spec-z  depends on dep-i (all implemented)
//
// and a standalone spec solo. spec-x, spec-y, spec-z and solo each have a plan
// with one open task, so implement can start on them. extraConfig is appended
// to the config.
func depProject(t *testing.T, extraConfig string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, implementSharedSlotConfig+extraConfig)
	data := filepath.Join(dir, ".spektacular")

	write := func(rel, content string) {
		p := filepath.Join(data, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	spec := func(name, epicName string) {
		fm := "---\ncreated_date: 2026-09-28\ndocument_status: final\n"
		if epicName != "" {
			fm += "epic: " + epicName + "\n"
		}
		write("specs/"+name+".md", fm+"---\n\n# Spec "+name+"\n")
	}
	plan := func(name string, total, done int) {
		var tasks strings.Builder
		for i := 1; i <= total; i++ {
			id := fmt.Sprintf("%08d-0000-4000-8000-000000000000", i)
			tasks.WriteString(taskBlock(fmt.Sprintf("Task %d", i), i <= done, agentFields(id), "- [ ] works"))
		}
		write("plans/"+name+"/plan.md", "---\ncreated_date: 2026-09-29\ndocument_status: final\nspec: "+name+"\n---\n\n"+taskPlanDoc(tasks.String()))
	}

	ep := epic.Epic{
		CreatedDate:    time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
		Specs: []epic.EpicSpec{
			{Name: "dep-a", DependsOn: []string{}},
			{Name: "dep-b", DependsOn: []string{"dep-a"}},
			{Name: "dep-i", DependsOn: []string{}},
			{Name: "spec-x", DependsOn: []string{"dep-b", "dep-a"}},
			{Name: "spec-y", DependsOn: []string{"dep-b"}},
			{Name: "spec-z", DependsOn: []string{"dep-i"}},
		},
		Body: []byte("## Overview\n\nAn epic.\n"),
	}
	raw, err := ep.Render()
	require.NoError(t, err)
	write("epics/epic-e.md", string(raw))

	for _, n := range []string{"dep-a", "dep-b", "dep-i", "spec-x", "spec-y", "spec-z"} {
		spec(n, "epic-e")
	}
	spec("solo", "")
	plan("dep-a", 5, 2)
	plan("dep-i", 1, 1)
	for _, n := range []string{"spec-x", "spec-y", "spec-z", "solo"} {
		plan(n, 1, 0)
	}
	return dir
}

const strictDependenciesConfig = "epic:\n  strict_dependencies: true\n"

// implementNewData runs `implement new --data <data>`.
func implementNewData(t *testing.T, data string) (string, int) {
	t.Helper()
	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "implement", "new", "--data", data)
	return stdout, code
}

// requireStarted runs `implement new --data <data>` and asserts it returns the
// first step of a fresh implement run, with no warning about dependencies.
func requireStarted(t *testing.T, data string) map[string]any {
	t.Helper()
	stdout, code := implementNewData(t, data)
	require.Equalf(t, 0, code, "implement new failed: %s", stdout)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "read_plan", result["step"])
	require.NotContains(t, strings.ToLower(stdout), "depends on")
	return result
}

// workflowData reads the persisted workflow data from state.json.
func workflowData(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".spektacular", "state.json"))
	require.NoError(t, err)
	var st struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &st))
	return st.Data
}

func TestImplementDependencies_StandaloneSpecStartsSilently(t *testing.T) {
	dir := depProject(t, strictDependenciesConfig)
	requireStarted(t, `{"name":"solo"}`)
	_, recorded := workflowData(t, dir)["dependency_override"]
	require.False(t, recorded)
}

func TestImplementDependencies_AllDependenciesImplementedStartsSilently(t *testing.T) {
	dir := depProject(t, strictDependenciesConfig)
	requireStarted(t, `{"name":"spec-z"}`)
	_, recorded := workflowData(t, dir)["dependency_override"]
	require.False(t, recorded)
}

func TestImplementDependencies_SpecWithoutDependenciesStartsSilently(t *testing.T) {
	depProject(t, "")
	// dep-a has no dependencies of its own, even though others depend on it.
	requireStarted(t, `{"name":"dep-a"}`)
}

func TestImplementDependencies_UnmetDependencyIsRefusedNamingEachState(t *testing.T) {
	dir := depProject(t, "")

	stdout, code := implementNewData(t, `{"name":"spec-x"}`)
	require.Equal(t, 1, code, stdout)
	er := decodeError(t, stdout)
	require.Equal(t, "dependencies_unmet", er.Code)
	require.Equal(t, "spec-x", er.Resource)
	require.Contains(t, er.Message, "spec-x depends on dep-b, which is unplanned")
	require.Contains(t, er.Message, "spec-x depends on dep-a, which is in progress (2/5 tasks complete)")

	require.Contains(t, er.NextAction, "ask whether to continue")
	require.Contains(t, er.NextAction, `implement new --data '{"name":"spec-x","override_dependencies":true}'`)
	require.Contains(t, er.NextAction, `implement new --data '{"name":"dep-a"}'`,
		"the first unmet dependency that is ready is A, since B waits on A")
	require.NotContains(t, er.NextAction, `{"name":"dep-b"}`)

	require.NoFileExists(t, filepath.Join(dir, ".spektacular", "state.json"), "a refusal starts nothing")
}

func TestImplementDependencies_NoReadyDependencyIsSaid(t *testing.T) {
	depProject(t, "")
	stdout, code := implementNewData(t, `{"name":"spec-y"}`)
	require.Equal(t, 1, code, stdout)
	er := decodeError(t, stdout)
	require.Equal(t, "dependencies_unmet", er.Code)
	require.Contains(t, er.Message, "spec-y depends on dep-b, which is unplanned")
	require.Contains(t, er.NextAction, "no unmet dependency is ready")
	require.NotContains(t, er.NextAction, `implement new --data '{"name":"dep-b"}'`)
}

func TestImplementDependencies_OverrideStartsAndIsRecorded(t *testing.T) {
	dir := depProject(t, "")

	result := requireStarted(t, `{"name":"spec-x","override_dependencies":true}`)
	require.Equal(t, "spec-x", result["plan_name"])

	got := workflowData(t, dir)["dependency_override"]
	require.Equal(t, []any{
		map[string]any{"name": "dep-b", "state": "unplanned"},
		map[string]any{"name": "dep-a", "state": "in progress (2/5 tasks complete)"},
	}, got)
}

func TestImplementDependencies_OverrideKeepsTheTask(t *testing.T) {
	depProject(t, "")
	stdout, code := implementNewData(t, `{"name":"spec-x","task":"00000001-0000-4000-8000-000000000000"}`)
	require.Equal(t, 1, code, stdout)
	er := decodeError(t, stdout)
	require.Contains(t, er.NextAction,
		`'{"name":"spec-x","override_dependencies":true,"task":"00000001-0000-4000-8000-000000000000"}'`,
		"the re-run keeps every field the caller sent")
}

func TestImplementDependencies_StrictRefusesTheOverride(t *testing.T) {
	dir := depProject(t, strictDependenciesConfig)

	stdout, code := implementNewData(t, `{"name":"spec-x","override_dependencies":true}`)
	require.Equal(t, 1, code, stdout)
	er := decodeError(t, stdout)
	require.Equal(t, "dependency_override_refused", er.Code)
	require.Contains(t, er.Message, "spec-x depends on dep-a, which is in progress (2/5 tasks complete)")
	require.Contains(t, er.Message, "strict_dependencies")
	require.Contains(t, er.NextAction, `implement new --data '{"name":"dep-a"}'`)
	require.NotContains(t, er.NextAction, "override_dependencies")
	require.NoFileExists(t, filepath.Join(dir, ".spektacular", "state.json"))
}

func TestImplementDependencies_StrictRefusesWithoutOfferingAnOverride(t *testing.T) {
	dir := depProject(t, strictDependenciesConfig)

	stdout, code := implementNewData(t, `{"name":"spec-x"}`)
	require.Equal(t, 1, code, stdout)
	er := decodeError(t, stdout)
	require.Equal(t, "dependencies_unmet", er.Code)
	require.Contains(t, er.Message, "spec-x depends on dep-b, which is unplanned")
	require.Contains(t, er.NextAction, "strict_dependencies")
	require.Contains(t, er.NextAction, `implement new --data '{"name":"dep-a"}'`)
	require.NotContains(t, er.NextAction, "override_dependencies")
	require.NoFileExists(t, filepath.Join(dir, ".spektacular", "state.json"))
}

// writeWorktreeRecord writes spec's worktree record by hand, as the worktree
// manager leaves it until the spec is merged back.
func writeWorktreeRecord(t *testing.T, dir, spec string) string {
	t.Helper()
	recordDir := filepath.Join(dir, ".spektacular", "worktrees", spec)
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	record := `{"spec":"` + spec + `","repos":{"testproj":"` + filepath.Join(t.TempDir(), "testproj-"+spec) + `"}}`
	path := filepath.Join(recordDir, "record.json")
	require.NoError(t, os.WriteFile(path, []byte(record), 0o644))
	return path
}

// dep-i has every task complete, but its worktree record is still there, so
// its work is not merged yet: spec-z's run is refused naming it, offering the
// override and the merge rather than implementing it again.
func TestImplementDependencies_UnmergedDependencyIsUnmet(t *testing.T) {
	dir := depProject(t, "")
	writeWorktreeRecord(t, dir, "dep-i")

	stdout, code := implementNewData(t, `{"name":"spec-z"}`)
	require.Equal(t, 1, code, stdout)
	er := decodeError(t, stdout)
	require.Equal(t, "dependencies_unmet", er.Code)
	require.Equal(t, "spec-z", er.Resource)
	require.Equal(t, "spec-z depends on dep-i, which is implemented but not yet merged", er.Message)
	require.Contains(t, er.NextAction, "ask whether to continue")
	require.Contains(t, er.NextAction, `implement new --data '{"name":"spec-z","override_dependencies":true}'`)
	require.Contains(t, er.NextAction, `implement merge --data '{"name":"dep-i"}'`)
	require.NotContains(t, er.NextAction, `implement new --data '{"name":"dep-i"}'`)
	require.NoFileExists(t, filepath.Join(dir, ".spektacular", "state.json"), "a refusal starts nothing")
}

func TestImplementDependencies_UnmergedDependencyOverrideIsRecorded(t *testing.T) {
	dir := depProject(t, "")
	writeWorktreeRecord(t, dir, "dep-i")

	requireStarted(t, `{"name":"spec-z","override_dependencies":true}`)
	require.Equal(t, []any{
		map[string]any{"name": "dep-i", "state": "implemented but not yet merged"},
	}, workflowData(t, dir)["dependency_override"])
}

func TestImplementDependencies_StrictRefusesAnUnmergedDependency(t *testing.T) {
	t.Run("without override", func(t *testing.T) {
		dir := depProject(t, strictDependenciesConfig)
		writeWorktreeRecord(t, dir, "dep-i")

		stdout, code := implementNewData(t, `{"name":"spec-z"}`)
		require.Equal(t, 1, code, stdout)
		er := decodeError(t, stdout)
		require.Equal(t, "dependencies_unmet", er.Code)
		require.Equal(t, "spec-z depends on dep-i, which is implemented but not yet merged", er.Message)
		require.Contains(t, er.NextAction, "strict_dependencies")
		require.Contains(t, er.NextAction, `implement merge --data '{"name":"dep-i"}'`)
		require.NotContains(t, er.NextAction, "override_dependencies")
		require.NoFileExists(t, filepath.Join(dir, ".spektacular", "state.json"))
	})

	t.Run("with override", func(t *testing.T) {
		dir := depProject(t, strictDependenciesConfig)
		writeWorktreeRecord(t, dir, "dep-i")

		stdout, code := implementNewData(t, `{"name":"spec-z","override_dependencies":true}`)
		require.Equal(t, 1, code, stdout)
		er := decodeError(t, stdout)
		require.Equal(t, "dependency_override_refused", er.Code)
		require.Contains(t, er.Message, "spec-z depends on dep-i, which is implemented but not yet merged")
		require.Contains(t, er.Message, "strict_dependencies")
		require.Contains(t, er.NextAction, `implement merge --data '{"name":"dep-i"}'`)
		require.NotContains(t, er.NextAction, "override_dependencies")
		require.NoFileExists(t, filepath.Join(dir, ".spektacular", "state.json"))
	})
}

// Once the dependency is merged back its record is gone, and the dependent
// starts without a warning, even under strict dependencies.
func TestImplementDependencies_MergedDependencyStartsSilently(t *testing.T) {
	dir := depProject(t, strictDependenciesConfig)
	record := writeWorktreeRecord(t, dir, "dep-i")

	_, code := implementNewData(t, `{"name":"spec-z"}`)
	require.Equal(t, 1, code, "refused while dep-i is unmerged")

	require.NoError(t, os.RemoveAll(filepath.Dir(record)))
	requireStarted(t, `{"name":"spec-z"}`)
	_, recorded := workflowData(t, dir)["dependency_override"]
	require.False(t, recorded)
}

func TestImplementDependencies_SpecifyingAndPlanningAreNeverHeldBack(t *testing.T) {
	t.Run("spec new joining an epic with unimplemented specs", func(t *testing.T) {
		depProject(t, strictDependenciesConfig)
		resetRootCmd(t)
		stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"joiner","epic":"epic-e"}`)
		require.Equalf(t, 0, code, "spec new failed: %s", stdout)
		require.NotContains(t, strings.ToLower(stdout), "depends on")
		require.NotContains(t, stdout, "dependencies_unmet")
	})

	t.Run("plan new for a spec whose dependency is unimplemented", func(t *testing.T) {
		depProject(t, strictDependenciesConfig)
		resetRootCmd(t)
		stdout, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"dep-b"}`)
		require.Equalf(t, 0, code, "plan new failed: %s", stdout)
		require.NotContains(t, strings.ToLower(stdout), "depends on")
		require.NotContains(t, stdout, "dependencies_unmet")
	})
}

func TestImplementDependencies_SchemaAndHelpNameTheSpec(t *testing.T) {
	require.Contains(t, implementNewCmd.Short, "spec")

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "implement", "new", "--schema")
	require.Equal(t, 0, code, stdout)
	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.Contains(t, schema.Input.Properties["name"].Description, "spec to implement")
	require.Equal(t, "boolean", schema.Input.Properties["override_dependencies"].Type)
}
