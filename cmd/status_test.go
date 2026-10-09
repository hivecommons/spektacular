package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

const idD = "44444444-4444-4444-8444-444444444444"

// writeArtifactStatusFile writes a stored document with the given
// frontmatter and pins its modification time.
func writeArtifactStatusFile(t *testing.T, path, frontmatter string, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(frontmatter+"\n# Artifact\n"), 0o644))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
}

// statusOf runs `status` with args and decodes its JSON result.
func statusOf(t *testing.T, args ...string) map[string]any {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, append([]string{"status"}, args...)...)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// statusSpecs returns the report's specs keyed by name.
func statusSpecs(t *testing.T, got map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, s := range got["specs"].([]any) {
		m := s.(map[string]any)
		out[m["name"].(string)] = m
	}
	return out
}

func statusSpecNames(got map[string]any) []string {
	var names []string
	for _, s := range got["specs"].([]any) {
		names = append(names, s.(map[string]any)["name"].(string))
	}
	return names
}

// stProject is a project with an epic E of three specs: A, implemented; B,
// depending on A, with one of two tasks complete; and C, depending on A,
// with no plan. S is a standalone spec with no plan.
func stProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
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
	plan := func(name, body string) {
		write("plans/"+name+"/plan.md", "---\ncreated_date: 2026-09-29\ndocument_status: final\nspec: "+name+"\n---\n\n"+body)
	}

	ep := epic.Epic{
		CreatedDate:    time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
		Specs: []epic.EpicSpec{
			{Name: "A", DependsOn: []string{}},
			{Name: "B", DependsOn: []string{"A"}},
			{Name: "C", DependsOn: []string{"A"}},
		},
		Body: []byte("## Overview\n\nAn epic.\n"),
	}
	raw, err := ep.Render()
	require.NoError(t, err)
	write("epics/E.md", string(raw))

	for _, n := range []string{"A", "B", "C"} {
		spec(n, "E")
	}
	spec("S", "")
	plan("A", taskPlanDoc(taskBlock("Only task", true, agentFields(idA), "- [x] done")))
	plan("B", taskPlanDoc(
		taskBlock("Add a reader", true, agentFields(idA), "- [x] a", "- [ ] b")+
			taskBlock("Add the writer", false, agentFields(idB, idA+" — Add a reader"), "- [ ] a"),
		taskBlock("Sign the release", false, []string{
			"**Id:** " + idC, "**Repo:** testproj",
			dependsLine(idB + " — Add the writer"),
			"**Execution:** human — needs the production signing key",
		}),
	))
	return dir
}

// Any spec in an epic, or its plan, reports the whole epic with the spec
// asked for named in requested; the epic's own name reports it too.
func TestStatus_SpecInAnEpicReportsTheWholeEpic(t *testing.T) {
	stProject(t)

	for _, name := range []string{"B", "E"} {
		got := statusOf(t, name, "--format", "json")
		require.Equal(t, name, got["requested"])
		require.Equal(t, []string{"A", "B", "C"}, statusSpecNames(got))
		e := got["epic"].(map[string]any)
		require.Equal(t, "E", e["name"])
		require.Equal(t, "draft", e["document_status"])
		require.Equal(t, map[string]any{
			"specs_implemented": float64(1), "specs_total": float64(3),
			"tasks_completed": float64(2), "tasks_total": float64(4),
		}, e["progress"])
		require.Nil(t, got["workflow"])
	}

	specs := statusSpecs(t, statusOf(t, "B", "--format", "json"))
	require.Equal(t, "implemented", specs["A"]["state"])
	require.Equal(t, "in_progress", specs["B"]["state"])
	require.Equal(t, "specified", specs["C"]["state"])
	require.Nil(t, specs["C"]["plan"])
	require.Equal(t, []any{"A"}, specs["B"]["depends_on"])
	require.Equal(t, true, specs["B"]["ready"])
	require.Equal(t, []any{}, specs["B"]["blocked_by"])
	require.Equal(t, "finished", specs["B"]["current_step"])
}

// A's worktree record is still there, so its work is not merged yet: B is
// not ready and blocked by A, while A itself is still reported implemented.
// Once the record is gone, B is ready again.
func TestStatus_UnmergedDependencyBlocksItsDependent(t *testing.T) {
	dir := stProject(t)
	recordDir := filepath.Join(dir, ".spektacular", "worktrees", "A")
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	record := `{"spec":"A","repos":{"testproj":"` + filepath.Join(t.TempDir(), "testproj-A") + `"}}`
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "record.json"), []byte(record), 0o644))

	resetRootCmd(t)
	got := statusOf(t, "E", "--format", "json")
	specs := statusSpecs(t, got)
	require.Equal(t, "implemented", specs["A"]["state"])
	require.Equal(t, false, specs["B"]["ready"])
	require.Equal(t, []any{"A"}, specs["B"]["blocked_by"])
	require.Equal(t, float64(1), got["epic"].(map[string]any)["progress"].(map[string]any)["specs_implemented"])

	require.NoError(t, os.RemoveAll(recordDir))
	resetRootCmd(t)
	specs = statusSpecs(t, statusOf(t, "E", "--format", "json"))
	require.Equal(t, "implemented", specs["A"]["state"])
	require.Equal(t, true, specs["B"]["ready"])
	require.Equal(t, []any{}, specs["B"]["blocked_by"])
}

func TestStatus_StandaloneSpecHasANullEpicAndOneSpec(t *testing.T) {
	stProject(t)

	got := statusOf(t, "S", "--format", "json")
	require.Contains(t, got, "epic")
	require.Nil(t, got["epic"])
	require.Equal(t, []string{"S"}, statusSpecNames(got))
	require.Equal(t, "specified", statusSpecs(t, got)["S"]["state"])
}

// A plan reports its tasks with the fields orchestrators read, plus the
// acceptance-criteria counts; completion and criteria are separate facts.
func TestStatus_PlanTasksCarryTheirFields(t *testing.T) {
	stProject(t)

	plan := statusSpecs(t, statusOf(t, "B", "--format", "json"))["B"]["plan"].(map[string]any)
	require.Equal(t, "B", plan["name"])
	require.Equal(t, "final", plan["document_status"])
	require.Equal(t, map[string]any{"tasks_completed": float64(1), "tasks_total": float64(3)}, plan["progress"])
	tasks := plan["tasks"].([]any)
	require.Len(t, tasks, 3)
	require.Equal(t, map[string]any{
		"id":                  idA,
		"title":               "Add a reader",
		"milestone":           float64(1),
		"repo":                map[string]any{"name": "testproj", "location": ""},
		"depends_on":          []any{},
		"execution":           map[string]any{"type": "agent", "reason": ""},
		"completed":           true,
		"acceptance_criteria": map[string]any{"met": float64(1), "total": float64(2)},
	}, tasks[0], "a task declaring none has an empty dependency list, not null")
	require.Equal(t, []any{idA}, tasks[1].(map[string]any)["depends_on"])
	require.Equal(t, map[string]any{
		"id":                  idC,
		"title":               "Sign the release",
		"milestone":           float64(2),
		"repo":                map[string]any{"name": "testproj", "location": ""},
		"depends_on":          []any{idB},
		"execution":           map[string]any{"type": "human", "reason": "needs the production signing key"},
		"completed":           false,
		"acceptance_criteria": map[string]any{"met": float64(0), "total": float64(0)},
	}, tasks[2])
}

// A plan without task structure reports its lifecycle with progress and
// tasks absent.
func TestStatus_LegacyPlanHasNoTaskFields(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	_, code := writePlanDoc(t, taskPlanName, "plan", "# Plan\n\n## Milestones & Phases\n\n### Milestone 1: M\n\n#### - [x] Phase 1.1: Work\n")
	require.Equal(t, 0, code)

	got := statusOf(t, taskPlanName, "--format", "json")
	spec := got["specs"].([]any)[0].(map[string]any)
	plan := spec["plan"].(map[string]any)
	require.Equal(t, taskPlanName, plan["name"])
	require.NotContains(t, plan, "progress")
	require.NotContains(t, plan, "tasks")
}

// The readable tree and the JSON document report the same facts: the epic's
// roll-up, every spec's state and counts, and the requested spec's tasks.
func TestStatus_PrettyAndJSONCarryTheSameInformation(t *testing.T) {
	stProject(t)

	def, _, code := runRootCmd(t, "status", "B")
	require.Equal(t, 0, code, def)
	pretty, _, code := runRootCmd(t, "status", "B", "--format", "pretty")
	require.Equal(t, 0, code, pretty)
	require.Equal(t, def, pretty, "pretty is the default")

	got := statusOf(t, "B", "--format", "json")
	e := got["epic"].(map[string]any)
	p := e["progress"].(map[string]any)
	require.Contains(t, pretty, "epic E  (draft)\n")
	require.Contains(t, pretty, "  1/3 specs implemented, 2/4 tasks\n")
	require.Equal(t, float64(1), p["specs_implemented"])
	require.Equal(t, float64(4), p["tasks_total"])

	lines := strings.Split(pretty, "\n")
	lineFor := func(name string) string {
		for _, l := range lines {
			if fields := strings.Fields(l); len(fields) > 1 && fields[1] == name {
				return l
			}
		}
		t.Fatalf("no line for %s in:\n%s", name, pretty)
		return ""
	}
	require.Contains(t, lineFor("A"), "implemented")
	require.Contains(t, lineFor("A"), "1/1 tasks")
	require.Contains(t, lineFor("B"), "in progress")
	require.Contains(t, lineFor("B"), "1/3 tasks")
	require.Contains(t, lineFor("B"), "← requested")
	require.Contains(t, lineFor("C"), "specified")
	require.Contains(t, lineFor("C"), "no plan")
	require.Contains(t, pretty, "depends on: A")

	for _, task := range statusSpecs(t, got)["B"]["plan"].(map[string]any)["tasks"].([]any) {
		require.Contains(t, pretty, task.(map[string]any)["title"])
	}
	require.Contains(t, pretty, "[x] Add a reader")
	require.Contains(t, pretty, "[ ] Add the writer")
	require.Contains(t, pretty, "human: needs the production signing key")
	require.NotContains(t, pretty, "Only task", "siblings show one line each")
}

func TestStatus_UnsupportedFormatIsRefused(t *testing.T) {
	stProject(t)

	stdout, _, code := runRootCmd(t, "status", "B", "--format", "yaml")
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "status_format_unsupported", er.Code)
	require.Contains(t, er.Message, "pretty")
	require.Contains(t, er.Message, "json")
	require.Contains(t, er.NextAction, "--format pretty")
	require.Contains(t, er.NextAction, "--format json")
	require.NotContains(t, stdout, "Add a reader", "nothing is reported")
}

func TestStatus_UnknownNameIsRefusedWithANextStep(t *testing.T) {
	stProject(t)

	for _, format := range []string{"pretty", "json"} {
		stdout, _, code := runRootCmd(t, "status", "nosuch", "--format", format)
		require.Equal(t, 1, code)
		er := decodeError(t, stdout)
		require.Equal(t, "artifact_not_found", er.Code)
		require.Equal(t, "nosuch", er.Resource)
		require.Contains(t, er.NextAction, "spec file list")
		require.Contains(t, er.NextAction, "epic list")
	}
}

func TestStatus_NoNameAndNothingInProgress(t *testing.T) {
	stProject(t)

	stdout, _, code := runRootCmd(t, "status", "--format", "json")
	require.Equal(t, 0, code, stdout)
	require.JSONEq(t, `{"error": false, "workflow": null}`, stdout)

	pretty, _, code := runRootCmd(t, "status")
	require.Equal(t, 0, code, pretty)
	require.Equal(t, "no workflow in progress\n", pretty)
}

// A finished workflow of any kind is not in progress: a plan workflow run to
// completion must not have its step vocabulary reported as current work.
func TestStatus_NoNameAfterAFinishedWorkflowReportsNothingInProgress(t *testing.T) {
	dir := stProject(t)
	writeInProgressState(t, filepath.Join(dir, ".spektacular"), workflow.State{
		Kind:           "plan",
		CurrentStep:    "finished",
		CompletedSteps: []string{"new", "overview", "architecture", "finished"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "B"},
	})

	stdout, _, code := runRootCmd(t, "status", "--format", "json")
	require.Equal(t, 0, code, stdout)
	require.JSONEq(t, `{"error": false, "workflow": null}`, stdout)
	require.NotContains(t, stdout, "architecture")
}

// With no name, the workflow in progress is reported: the same report as
// `status <name>` for its artifact, with the workflow block set and the live
// step on the artifact it works on.
func TestStatus_NoNameReportsTheWorkflowInProgress(t *testing.T) {
	dir := stProject(t)
	writeInProgressState(t, filepath.Join(dir, ".spektacular"), workflow.State{
		Kind:           "implement",
		CurrentStep:    "analyze",
		CompletedSteps: []string{"new", "read_plan"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      time.Date(2026, time.September, 30, 10, 12, 0, 0, time.UTC),
		Data:           map[string]any{"name": "B", "task": idB},
	})

	got := statusOf(t, "--format", "json")
	require.Equal(t, map[string]any{
		"kind":            "implement",
		"name":            "B",
		"current_step":    "analyze",
		"completed_steps": []any{"new", "read_plan"},
		"updated_at":      "2026-09-30T10:12:00Z",
	}, got["workflow"])
	require.Equal(t, "B", got["requested"])
	require.Equal(t, "E", got["epic"].(map[string]any)["name"])

	// The same workflow block rides on a named report that covers it, and
	// not on one that does not.
	require.Equal(t, got["workflow"], statusOf(t, "C", "--format", "json")["workflow"])
	require.Nil(t, statusOf(t, "S", "--format", "json")["workflow"])

	pretty, _, code := runRootCmd(t, "status")
	require.Equal(t, 0, code, pretty)
	require.Contains(t, pretty, "workflow in progress\n  ● implement  B  at step analyze\n")
	require.Contains(t, pretty, "epic E")
}

// With no name, a run kept in its own lane is reported even when the shared
// state holds nothing in progress, as an implement run with worktrees or an
// epic's orchestrated runs do.
func TestStatus_NoNameReportsARunInItsOwnLane(t *testing.T) {
	dir := stProject(t)
	lane := workflow.State{
		Kind:        "implement",
		CurrentStep: "verify",
		CreatedAt:   fixedResumeTime,
		UpdatedAt:   fixedResumeTime,
		Data:        map[string]any{"name": "B", "orchestrated": true},
	}
	raw, err := json.Marshal(lane)
	require.NoError(t, err)
	path := workflow.LaneStatePath(filepath.Join(dir, ".spektacular"), "implement", "B")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, raw, 0o644))

	got := statusOf(t, "--format", "json")
	w := got["workflow"].(map[string]any)
	require.Equal(t, "implement", w["kind"])
	require.Equal(t, "B", w["name"])
	require.Equal(t, "verify", w["current_step"])
	require.Equal(t, true, w["orchestrated"])
	require.Len(t, got["workflows"], 1)
	require.Equal(t, "E", got["epic"].(map[string]any)["name"])

	pretty, _, code := runRootCmd(t, "status")
	require.Equal(t, 0, code, pretty)
	require.Contains(t, pretty, "● implement  B  at step verify  (orchestrated)")
	require.Contains(t, pretty, "epic E")
}

// A spec workflow whose spec is not written yet is still reported.
func TestStatus_NoNameReportsAWorkflowWhoseArtifactIsNotWrittenYet(t *testing.T) {
	dir := stProject(t)
	writeInProgressState(t, filepath.Join(dir, ".spektacular"), workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new", "interview"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000009_unwritten"},
	})

	got := statusOf(t, "--format", "json")
	w := got["workflow"].(map[string]any)
	require.Equal(t, "spec", w["kind"])
	require.Equal(t, "000009_unwritten", w["name"])
	require.Equal(t, "overview", w["current_step"])
	require.NotContains(t, got, "specs")
}

// A spec whose workflow is live reports its live step; a draft whose
// in-progress workflow belongs to another artifact reports no step and no
// workflow block.
func TestStatus_LiveStepComesOnlyFromItsOwnWorkflow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeArtifactStatusFile(t, filepath.Join(dataDir, "specs", "000002_active.md"), "---\ncreated_date: 2026-01-02\ndocument_status: draft\n---\n", time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC))
	writeArtifactStatusFile(t, filepath.Join(dataDir, "specs", "000003_other.md"), "---\ncreated_date: 2026-01-02\ndocument_status: draft\n---\n", time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC))
	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new", "interview"},
		CreatedAt:      time.Date(2026, time.January, 2, 1, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, time.January, 5, 6, 7, 8, 0, time.UTC),
		Data:           map[string]any{"name": "000002_active"},
	})

	live := statusOf(t, "000002_active", "--format", "json")
	require.Equal(t, "overview", statusSpecs(t, live)["000002_active"]["current_step"])
	require.Equal(t, "draft", statusSpecs(t, live)["000002_active"]["document_status"])
	require.Equal(t, "2026-01-05T06:07:08Z", live["workflow"].(map[string]any)["updated_at"])

	other := statusOf(t, "000003_other", "--format", "json")
	require.Equal(t, "", statusSpecs(t, other)["000003_other"]["current_step"])
	require.Nil(t, other["workflow"], "another artifact's workflow is not this artifact's activity")
}

// Under plan.strict_spec_changes, a plan whose spec changed after it closed
// is reported stale; without it, the plan stays final.
func TestStatus_StrictSpecChangesReportsAStalePlan(t *testing.T) {
	for _, tc := range []struct {
		name, config, docStatus, step, state string
	}{
		{"strict", "plan:\n  strict_spec_changes: true\n", "stale", "stale", "stale"},
		{"not strict", "", "final", "finished", "planned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			dataDir := filepath.Join(dir, ".spektacular")
			writeSpecCommandConfig(t, dir, tc.config)
			writeArtifactStatusFile(t, filepath.Join(dataDir, "specs", "000004_feature.md"), "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n", time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC))
			planPath := filepath.Join(dataDir, "plans", "000004_feature", "plan.md")
			require.NoError(t, os.MkdirAll(filepath.Dir(planPath), 0o755))
			require.NoError(t, os.WriteFile(planPath, []byte("---\ncreated_date: 2026-02-01\ndocument_status: final\nclosed_date: 2026-02-02\nspec: 000004_feature\n---\n\n"+
				taskPlanDoc(taskBlock("Work", false, agentFields(idA)))), 0o644))
			mod := time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC)
			require.NoError(t, os.Chtimes(planPath, mod, mod))

			spec := statusSpecs(t, statusOf(t, "000004_feature", "--format", "json"))["000004_feature"]
			plan := spec["plan"].(map[string]any)
			require.Equal(t, tc.docStatus, plan["document_status"])
			require.Equal(t, tc.step, plan["current_step"])
			require.Equal(t, tc.state, spec["state"])
		})
	}
}

// Under plan.strict_spec_changes, a spec newer than its plan whose body still
// matches its last recorded amendment does not make the plan stale.
func TestStatus_StrictSpecChangesIgnoresRecordedAmendment(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "plan:\n  strict_spec_changes: true\n")

	base := "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n"
	_, body, err := metadata.Split([]byte(base + "\n# Artifact\n"))
	require.NoError(t, err)
	amended := "---\ncreated_date: 2026-02-01\ndocument_status: final\namendments:\n    - at: \"2026-02-03T00:00:00Z\"\n      sections: [Success Metrics]\n      hash: " + metadata.BodyHash(body) + "\n---\n"
	writeArtifactStatusFile(t, filepath.Join(dataDir, "specs", "000004_feature.md"), amended, time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC))

	planPath := filepath.Join(dataDir, "plans", "000004_feature", "plan.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(planPath), 0o755))
	require.NoError(t, os.WriteFile(planPath, []byte("---\ncreated_date: 2026-02-01\ndocument_status: final\nclosed_date: 2026-02-02\nspec: 000004_feature\n---\n\n"+
		taskPlanDoc(taskBlock("Work", false, agentFields(idA)))), 0o644))
	mod := time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(planPath, mod, mod))

	spec := statusSpecs(t, statusOf(t, "000004_feature", "--format", "json"))["000004_feature"]
	plan := spec["plan"].(map[string]any)
	require.Equal(t, "final", plan["document_status"])
	require.Equal(t, "finished", plan["current_step"])
	require.Equal(t, "planned", spec["state"])
}

// Under plan.strict_spec_changes, an amendment recorded through the real
// `spec amend` command keeps the plan final although the spec is newer than
// it, and a further body edit through `spec file write` makes it stale.
func TestStatus_StrictSpecChangesAmendThroughCLI(t *testing.T) {
	const name = "000004_feature"
	_, specPath := strictAmendProject(t, name, "")
	planState := func(t *testing.T) (docStatus, step, state any) {
		t.Helper()
		resetRootCmd(t)
		spec := statusSpecs(t, statusOf(t, name, "--format", "json"))[name]
		plan := spec["plan"].(map[string]any)
		return plan["document_status"], plan["current_step"], spec["state"]
	}
	requireState := func(t *testing.T, wantDoc, wantStep, wantState string) {
		t.Helper()
		docStatus, step, state := planState(t)
		require.Equal(t, wantDoc, docStatus)
		require.Equal(t, wantStep, step)
		require.Equal(t, wantState, state)
	}

	// The mtimes alone make the plan stale before anything is recorded.
	requireState(t, "stale", "stale", "stale")

	// The fixture plan already has one phase ticked, so a plan that is not
	// stale reports its spec in progress.
	amendSuccessMetricViaCLI(t, name, specPath)
	requireState(t, "final", "finished", "in_progress")

	editSpecUnrecordedViaCLI(t, name, specPath)
	requireState(t, "stale", "stale", "stale")
}

// A task's location comes only from a declared git source, never from a
// checkout's remotes.
func TestStatus_TaskLocationComesOnlyFromADeclaredGitSource(t *testing.T) {
	locations := func(t *testing.T) []any {
		var out []any
		for _, task := range statusSpecs(t, statusOf(t, "B", "--format", "json"))["B"]["plan"].(map[string]any)["tasks"].([]any) {
			out = append(out, task.(map[string]any)["repo"].(map[string]any)["location"])
		}
		return out
	}

	t.Run("git source", func(t *testing.T) {
		dir := stProject(t)
		rc := config.NewDefaultRepoConfig()
		rc.Source = config.GitSource("https://github.com/example/widgets")
		require.NoError(t, rc.ToYAMLFile(filepath.Join(dir, ".spektacular", config.RepoConfigFileName)))
		for _, l := range locations(t) {
			require.Equal(t, "https://github.com/example/widgets", l)
		}
	})

	t.Run("file source with a git remote", func(t *testing.T) {
		dir := stProject(t)
		gittest.RunGit(t, dir, "init", "-q")
		gittest.RunGit(t, dir, "remote", "add", "origin", "https://github.com/example/should-not-appear")
		for _, l := range locations(t) {
			require.Equal(t, "", l)
		}
	})
}

// Status parses the plan at call time, so a ticked task shows at once.
func TestStatus_ReflectsTheCurrentPlan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	body := validTaskPlan()
	_, code := writePlanDoc(t, taskPlanName, "plan", body)
	require.Equal(t, 0, code)

	completed := func() any {
		got := statusOf(t, taskPlanName, "--format", "json")
		return got["specs"].([]any)[0].(map[string]any)["plan"].(map[string]any)["tasks"].([]any)[1].(map[string]any)["completed"]
	}
	require.Equal(t, false, completed())

	ticked := strings.Replace(body, "#### - [ ] Task: Build the export", "#### - [x] Task: Build the export", 1)
	_, code = writePlanDoc(t, taskPlanName, "plan", ticked)
	require.Equal(t, 0, code)
	require.Equal(t, true, completed())
}

// Every plan `plan file write` accepts, including the plan scaffold's task
// block with its placeholders filled in, reports in both formats.
func TestStatus_EverySavedPlanReports(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	scaffold, err := stepkit.RenderTemplate("scaffold/plan.md", map[string]any{"name": taskPlanName})
	require.NoError(t, err)
	filled := strings.NewReplacer(
		"<id from plan task-id>", idA,
		"<one registered repo name>", "testproj",
	).Replace(scaffold)

	for _, body := range []string{validTaskPlan(), filled} {
		stdout, code := writePlanDoc(t, taskPlanName, "plan", body)
		require.Equal(t, 0, code, stdout)
		for _, format := range []string{"pretty", "json"} {
			stdout, _, code := runRootCmd(t, "status", taskPlanName, "--format", format)
			require.Equal(t, 0, code, stdout)
		}
	}

	got := statusOf(t, taskPlanName, "--format", "json")
	tasks := got["specs"].([]any)[0].(map[string]any)["plan"].(map[string]any)["tasks"].([]any)
	require.Len(t, tasks, 1)
	require.Equal(t, idA, tasks[0].(map[string]any)["id"])
}

func TestStatus_SchemaDescribesTheReport(t *testing.T) {
	t.Chdir(t.TempDir())
	stdout, _, code := runRootCmd(t, "status", "--schema")
	require.Equal(t, 0, code, stdout)
	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	for _, k := range []string{"workflow", "requested", "epic", "specs"} {
		require.Contains(t, schema.Output.Properties, k)
	}
	tasks := schema.Output.Properties["specs"].Items.Properties["plan"].Properties["tasks"].Items.Properties
	require.Contains(t, tasks, "acceptance_criteria")
	require.Contains(t, tasks, "execution")
	require.Equal(t, []string{"pretty", "json"}, schema.Flags["format"].Enum)
}

// The per-kind status commands and plan export are gone, with no aliases.
func TestStatus_RetiredCommandsAreUnknownSubcommands(t *testing.T) {
	stProject(t)

	for _, args := range [][]string{
		{"spec", "status"},
		{"spec", "status", "B"},
		{"plan", "status"},
		{"plan", "status", "B"},
		{"implement", "status"},
		{"plan", "export", "B"},
	} {
		resetRootCmd(t)
		stdout, _, code := runRootCmd(t, args...)
		require.Equal(t, 1, code, "%v: %s", args, stdout)
		er := decodeError(t, stdout)
		require.Equal(t, "unknown_subcommand", er.Code, "%v", args)
		require.NotContains(t, stdout, "Add a reader", "%v", args)
	}
}

// A task run that cannot start points at status for the spec's task ids.
func TestStatus_TaskRefusalsPointAtStatus(t *testing.T) {
	taskRunProject(t, taskRunPlan())

	for _, task := range []string{"99999999-9999-4999-8999-999999999999", idA, idD} {
		stdout, code := implementNewTask(t, task)
		require.Equal(t, 1, code)
		er := decodeError(t, stdout)
		require.Contains(t, er.NextAction, "status "+taskPlanName+" --format json", task)
		require.NotContains(t, er.NextAction, "plan export")
	}
}

// An orchestrated plan in progress runs in its own lane, not state.json:
// status for its spec shows the lane's live step on the plan, and the
// workflow block marked orchestrated, while a standalone workflow in
// state.json keeps its block free of the orchestrated key.
func TestStatus_ReportsAnOrchestratedPlanLane(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	data := filepath.Join(dir, ".spektacular")
	write := func(rel, content string) {
		p := filepath.Join(data, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	ep := epic.Epic{
		CreatedDate:    time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
		Specs: []epic.EpicSpec{
			{Name: "alpha", DependsOn: []string{}},
			{Name: "beta", DependsOn: []string{}},
		},
		Body: []byte("## Overview\n\nAn epic.\n"),
	}
	raw, err := ep.Render()
	require.NoError(t, err)
	write("epics/lanes.md", string(raw))
	for _, n := range []string{"alpha", "beta"} {
		write("specs/"+n+".md", "---\ncreated_date: 2026-09-28\ndocument_status: final\nepic: lanes\n---\n\n# Spec "+n+"\n")
	}
	write("specs/solo.md", "---\ncreated_date: 2026-09-28\ndocument_status: final\n---\n\n# Spec solo\n")
	// alpha's plan is already written as a draft; beta's is not written yet.
	write("plans/alpha/plan.md", "---\ncreated_date: 2026-09-29\ndocument_status: draft\nspec: alpha\n---\n\n# Plan\n")

	startPlanLane(t, "alpha")
	walkPlanLane(t, "alpha", "discovery", "architecture")
	startPlanLane(t, "beta")
	walkPlanLane(t, "beta", "discovery")
	require.NoFileExists(t, stateFilePath(data), "orchestrated starts never write state.json")

	got := statusOf(t, "alpha", "--format", "json")
	specs := statusSpecs(t, got)
	require.Equal(t, "architecture", specs["alpha"]["plan"].(map[string]any)["current_step"])
	require.Nil(t, specs["beta"]["plan"])
	w := got["workflow"].(map[string]any)
	require.Equal(t, "plan", w["kind"])
	require.Equal(t, "alpha", w["name"], "the first in-progress lane in report order")
	require.Equal(t, "architecture", w["current_step"])
	require.Equal(t, true, w["orchestrated"])

	// A standalone plan for another spec lives in state.json; the lanes'
	// steps still come through, and its own block has no orchestrated key.
	runOK(t, "plan", "new", "--data", `{"name":"solo"}`)
	solo := statusOf(t, "solo", "--format", "json")
	sw := solo["workflow"].(map[string]any)
	require.Equal(t, "solo", sw["name"])
	require.Equal(t, "overview", sw["current_step"])
	require.NotContains(t, sw, "orchestrated")

	again := statusOf(t, "lanes", "--format", "json")
	require.Equal(t, "architecture", statusSpecs(t, again)["alpha"]["plan"].(map[string]any)["current_step"])
	require.Equal(t, true, again["workflow"].(map[string]any)["orchestrated"])
}

// Naming an epic adds the run view: the epic's run block and a run block on
// each spec, in a project that is not a git repo and has no worktrees.
func TestStatus_EpicReportCarriesTheRunView(t *testing.T) {
	dir := stProject(t)
	data := filepath.Join(dir, ".spektacular")
	// A's changelog record is final, so A is fully implemented.
	writeArtifactStatusFile(t, filepath.Join(data, "changelog", "A.md"),
		"---\ncreated_date: 2026-09-30\ndocument_status: final\n---", time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))

	got := statusOf(t, "E", "--format", "json")
	run := got["epic"].(map[string]any)["run"].(map[string]any)
	require.Equal(t, []any{"A", "B", "C"}, run["order"])
	require.Equal(t, map[string]any{"done": float64(2), "in_progress": float64(0), "ready": float64(1), "blocked": float64(0), "awaiting_merge": float64(0), "remaining": float64(1)}, run["plan"])
	require.Equal(t, map[string]any{"done": float64(1), "in_progress": float64(0), "ready": float64(1), "blocked": float64(1), "awaiting_merge": float64(0), "remaining": float64(2)}, run["implement"])
	require.Equal(t, false, run["dirty"])
	require.Equal(t, []any{}, run["dirty_repos"], "clean runs expose an empty repo list, not null")
	problems := run["problems"].([]any)
	require.Len(t, problems, 1)
	require.Equal(t, "epic_unplanned", problems[0].(map[string]any)["code"])
	require.Equal(t, []any{"C"}, problems[0].(map[string]any)["specs"])

	specs := statusSpecs(t, got)
	require.Equal(t, map[string]any{
		"plan":      map[string]any{"state": "done"},
		"implement": map[string]any{"state": "done", "repos": []any{"testproj"}},
	}, specs["A"]["run"], "repos are the registered repos the plan touches")
	require.Equal(t, map[string]any{
		"plan":      map[string]any{"state": "done"},
		"implement": map[string]any{"state": "ready", "repos": []any{"testproj"}},
	}, specs["B"]["run"])
	require.Equal(t, map[string]any{
		"plan":      map[string]any{"state": "ready"},
		"implement": map[string]any{"state": "blocked", "waiting_on": []any{"C"}},
	}, specs["C"]["run"])

	// The pretty report's epic header carries the same totals.
	stdout, _, code := runRootCmd(t, "status", "E")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "planning: 2 done, 0 in progress, 1 ready, 0 blocked")
	require.Contains(t, stdout, "implementing: 1 done, 0 in progress, 1 ready, 1 blocked")
	require.Contains(t, stdout, "problem (epic_unplanned): C has no final plan yet")
}

func TestStatus_SchemaDescribesTheRunView(t *testing.T) {
	t.Chdir(t.TempDir())
	stdout, _, code := runRootCmd(t, "status", "--schema")
	require.Equal(t, 0, code, stdout)
	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	epicRun := schema.Output.Properties["epic"].Properties["run"]
	require.NotNil(t, epicRun)
	for _, k := range []string{"order", "plan", "implement", "dirty", "dirty_repos", "problems"} {
		require.Contains(t, epicRun.Properties, k)
	}
	specRun := schema.Output.Properties["specs"].Items.Properties["run"]
	require.NotNil(t, specRun)
	require.Contains(t, specRun.Properties, "plan")
	require.Contains(t, specRun.Properties["implement"].Properties, "waiting_on")
	require.Contains(t, stdout, "awaiting_merge")
}

// Without a name, status reports the workflow in progress and leaves the run
// view out; naming the epic shows the shared workflow's spec in progress.
func TestStatus_RunViewOnlyWhenANameIsGiven(t *testing.T) {
	dir := stProject(t)
	writeInProgressState(t, filepath.Join(dir, ".spektacular"), workflow.State{
		Kind:        "implement",
		CurrentStep: "analyze",
		CreatedAt:   fixedResumeTime,
		UpdatedAt:   time.Date(2026, time.September, 30, 10, 12, 0, 0, time.UTC),
		Data:        map[string]any{"name": "B", "task": idB},
	})

	bare := statusOf(t, "--format", "json")
	require.NotContains(t, bare["epic"].(map[string]any), "run")
	for _, s := range bare["specs"].([]any) {
		require.NotContains(t, s.(map[string]any), "run")
	}

	named := statusOf(t, "E", "--format", "json")
	impl := statusSpecs(t, named)["B"]["run"].(map[string]any)["implement"].(map[string]any)
	require.Equal(t, "in_progress", impl["state"])
	require.Equal(t, "analyze", impl["current_step"])
	require.Equal(t, filepath.Base(dir), filepath.Base(impl["root"].(string)))
}
