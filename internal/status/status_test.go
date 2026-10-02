package status

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
)

type env struct {
	t    *testing.T
	root string
	opts Options
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{Command: "spektacular"}
	cfg.Spec.Config.Directory = "specs"
	cfg.Plan.Config.Directory = "plans"
	cfg.Epic.Config.Directory = "epics"
	return &env{t: t, root: root, opts: Options{Config: cfg, Store: store.NewFileStore(root, "project")}}
}

func (e *env) write(rel, content string) {
	e.t.Helper()
	p := filepath.Join(e.root, rel)
	require.NoError(e.t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(e.t, os.WriteFile(p, []byte(content), 0o644))
}

func (e *env) spec(name, epicName string, sources ...string) {
	e.t.Helper()
	var b strings.Builder
	b.WriteString("---\ncreated_date: \"2026-09-28\"\ndocument_status: final\n")
	if epicName != "" {
		fmt.Fprintf(&b, "epic: %s\n", epicName)
	}
	if len(sources) > 0 {
		b.WriteString("sources:\n")
		for _, s := range sources {
			fmt.Fprintf(&b, "  - uri: %s\n    retrieved_date: \"2026-09-28\"\n", s)
		}
	}
	b.WriteString("---\n\n# Spec\n")
	e.write("specs/"+name+".md", b.String())
}

func (e *env) epic(name string, specs []epic.EpicSpec, sources ...string) {
	e.t.Helper()
	ep := epic.Epic{
		CreatedDate:    time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
		Specs:          specs,
		Body:           []byte("## Overview\n\nAn epic.\n"),
	}
	for _, s := range sources {
		ep.Sources = append(ep.Sources, metadata.SourceRef{URI: s, RetrievedDate: "2026-09-28"})
	}
	raw, err := ep.Render()
	require.NoError(e.t, err)
	e.write("epics/"+name+".md", string(raw))
}

// plan writes a task-format plan for name; done says which tasks are ticked.
// Each task has two criteria, the first met.
func (e *env) plan(name, docStatus string, done ...bool) {
	e.t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "---\ncreated_date: \"2026-09-29\"\ndocument_status: %s\nspec: %s\n---\n\n# Plan\n\n## Milestones & Tasks\n\n### Milestone 1: Work\n\n", docStatus, name)
	prev := ""
	for i, d := range done {
		box := " "
		if d {
			box = "x"
		}
		id := fmt.Sprintf("%s-task-%d", name, i+1)
		deps := "none"
		if prev != "" {
			deps = "\n- " + prev + " — Task " + fmt.Sprint(i)
		}
		fmt.Fprintf(&b, "#### - [%s] Task: Task %d\n**Id:** %s\n**Repo:** spektacular\n**Depends on:** %s\n**Execution:** agent\n\n**Acceptance criteria**:\n- [x] first\n- [ ] second\n\n", box, i+1, id, deps)
		prev = id
	}
	e.write("plans/"+name+"/plan.md", b.String())
}

func (e *env) legacyPlan(name string, done ...bool) {
	e.t.Helper()
	var b strings.Builder
	b.WriteString("---\ncreated_date: \"2026-09-29\"\ndocument_status: final\n---\n\n## Milestones & Phases\n\n### Milestone 1: Old\n\n")
	for i, d := range done {
		box := " "
		if d {
			box = "x"
		}
		fmt.Fprintf(&b, "#### - [%s] Phase 1.%d: Step\n\n", box, i+1)
	}
	e.write("plans/"+name+"/plan.md", b.String())
}

func (e *env) touch(rel string, at time.Time) {
	e.t.Helper()
	require.NoError(e.t, os.Chtimes(filepath.Join(e.root, rel), at, at))
}

// standardEpic is E with A implemented, B in progress depending on A, C
// specified depending on B, and D named but missing.
func (e *env) standardEpic() {
	e.epic("E", []epic.EpicSpec{
		{Name: "A", DependsOn: []string{}},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "C", DependsOn: []string{"B"}},
		{Name: "D", DependsOn: []string{}},
	}, "https://example.com/epic-source")
	e.spec("A", "E")
	e.spec("B", "E", "https://example.com/b-source")
	e.spec("C", "E")
	e.plan("A", "final", true, true)
	e.plan("B", "final", true, false, false)
}

func specByName(t *testing.T, r Report, name string) SpecStatus {
	t.Helper()
	for _, s := range r.Specs {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("spec %q not in report", name)
	return SpecStatus{}
}

func topLevelKeys(t *testing.T, r Report) []string {
	t.Helper()
	raw, err := json.Marshal(r)
	require.NoError(t, err)
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestClassify(t *testing.T) {
	tasks := func(done ...bool) PlanFacts {
		var b strings.Builder
		b.WriteString("## Milestones & Tasks\n\n")
		for i, d := range done {
			box := " "
			if d {
				box = "x"
			}
			fmt.Fprintf(&b, "#### - [%s] Task: T%d\n", box, i)
		}
		return PlanFacts{Exists: true, Parsed: plantask.Parse([]byte(b.String()))}
	}
	legacy := PlanFacts{Exists: true, Parsed: plantask.Parse([]byte("## Milestones & Phases\n\n#### - [x] Phase 1.1: a\n#### - [ ] Phase 1.2: b\n"))}
	empty := PlanFacts{Exists: true, Parsed: plantask.Parse([]byte("## Milestones & Tasks\n\nnothing yet\n"))}
	stale := tasks(true, true)
	stale.Stale = true

	cases := []struct {
		name     string
		readable bool
		plan     PlanFacts
		want     SpecState
		counts   TaskCounts
	}{
		{"missing spec", false, tasks(true), StateMissing, TaskCounts{1, 1}},
		{"no plan", true, PlanFacts{}, StateSpecified, TaskCounts{}},
		{"stale plan", true, stale, StateStale, TaskCounts{2, 2}},
		{"none complete", true, tasks(false, false), StatePlanned, TaskCounts{0, 2}},
		{"some complete", true, tasks(true, false, false), StateInProgress, TaskCounts{1, 3}},
		{"all complete", true, tasks(true, true), StateImplemented, TaskCounts{2, 2}},
		{"no checkboxes", true, empty, StatePlanned, TaskCounts{0, 0}},
		{"legacy phases", true, legacy, StateInProgress, TaskCounts{1, 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, counts := Classify(c.readable, c.plan)
			require.Equal(t, c.want, got)
			require.Equal(t, c.counts, counts)
		})
	}
}

func TestDescribe(t *testing.T) {
	require.Equal(t, "in progress (2/5 tasks complete)", Describe(StateInProgress, TaskCounts{2, 5}))
	require.Equal(t, "planned, not started", Describe(StatePlanned, TaskCounts{0, 5}))
	require.Equal(t, "unplanned", Describe(StateSpecified, TaskCounts{}))
	require.Equal(t, "stale", Describe(StateStale, TaskCounts{}))
	require.Equal(t, "implemented", Describe(StateImplemented, TaskCounts{5, 5}))
	require.Equal(t, "missing", Describe(StateMissing, TaskCounts{}))
}

func TestBuild_EveryNameResolvesToTheEpic(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()
	// A plan whose name differs from its spec resolves through its spec field.
	e.write("plans/P/plan.md", "---\ncreated_date: \"2026-09-29\"\ndocument_status: draft\nspec: C\n---\n\n# Plan\n")

	for _, name := range []string{"E", "A", "B", "C", "P"} {
		t.Run(name, func(t *testing.T) {
			r, err := Build(e.opts, name)
			require.NoError(t, err)
			require.NotNil(t, r.Epic)
			require.Equal(t, "E", r.Epic.Name)
			require.Equal(t, name, r.Requested)
			require.Len(t, r.Specs, 4)
		})
	}
}

func TestBuild_EpicReport(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()

	r, err := Build(e.opts, "B")
	require.NoError(t, err)

	require.Equal(t, "draft", r.Epic.DocumentStatus)
	require.Equal(t, "2026-09-28T00:00:00Z", r.Epic.CreatedAt)
	require.False(t, r.Epic.Done)
	require.Equal(t, EpicProgress{SpecsImplemented: 1, SpecsTotal: 4, TasksCompleted: 3, TasksTotal: 5}, r.Epic.Progress)

	names := []string{}
	for _, s := range r.Specs {
		names = append(names, s.Name)
	}
	require.Equal(t, []string{"A", "B", "C", "D"}, names, "specs follow the epic's order")

	a := specByName(t, r, "A")
	require.Equal(t, StateImplemented, a.State)
	require.Equal(t, "final", a.DocumentStatus)
	require.Equal(t, "finished", a.CurrentStep)
	require.True(t, a.Ready)
	require.Equal(t, []string{}, a.BlockedBy)
	require.Equal(t, &PlanProgress{TasksCompleted: 2, TasksTotal: 2}, a.Plan.Progress)

	b := specByName(t, r, "B")
	require.Equal(t, StateInProgress, b.State)
	require.Equal(t, []string{"A"}, b.DependsOn)
	require.True(t, b.Ready)
	require.Len(t, b.Plan.Tasks, 3)
	task := b.Plan.Tasks[1]
	require.Equal(t, "B-task-2", task.ID)
	require.Equal(t, "Task 2", task.Title)
	require.Equal(t, 1, task.Milestone)
	require.Equal(t, "spektacular", task.Repo.Name)
	require.Equal(t, []string{"B-task-1"}, task.DependsOn)
	require.Equal(t, "agent", task.Execution.Type)
	require.False(t, task.Completed)
	require.Equal(t, plantask.Criteria{Met: 1, Total: 2}, task.AcceptanceCriteria)

	c := specByName(t, r, "C")
	require.Equal(t, StateSpecified, c.State)
	require.Nil(t, c.Plan)
	require.False(t, c.Ready)
	require.Equal(t, []string{"B"}, c.BlockedBy)

	d := specByName(t, r, "D")
	require.Equal(t, StateMissing, d.State, "a missing spec is reported, not fatal")
	require.Nil(t, d.Plan)
}

func TestBuild_TaskJSONCarriesExportFieldsAndCriteria(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()
	r, err := Build(e.opts, "E")
	require.NoError(t, err)

	raw, err := json.Marshal(r)
	require.NoError(t, err)
	var doc struct {
		Epic  map[string]json.RawMessage `json:"epic"`
		Specs []struct {
			Plan *struct {
				Progress map[string]int               `json:"progress"`
				Tasks    []map[string]json.RawMessage `json:"tasks"`
			} `json:"plan"`
		} `json:"specs"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	for _, k := range []string{"name", "document_status", "created_at", "done", "progress", "sources"} {
		require.Contains(t, doc.Epic, k)
	}
	require.Equal(t, map[string]int{"tasks_completed": 2, "tasks_total": 2}, doc.Specs[0].Plan.Progress)
	keys := []string{}
	for k := range doc.Specs[0].Plan.Tasks[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"acceptance_criteria", "completed", "depends_on", "execution", "id", "milestone", "repo", "title"}, keys)
	require.Nil(t, doc.Specs[2].Plan)
}

func TestBuild_LegacyPlanHasNoProgressOrTasks(t *testing.T) {
	e := newEnv(t)
	e.spec("L", "")
	e.legacyPlan("L", true, true)

	r, err := Build(e.opts, "L")
	require.NoError(t, err)
	s := r.Specs[0]
	require.Equal(t, StateImplemented, s.State)
	require.NotNil(t, s.Plan)
	require.Nil(t, s.Plan.Progress)
	require.Nil(t, s.Plan.Tasks)

	raw, err := json.Marshal(s.Plan)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "progress")
	require.NotContains(t, string(raw), "tasks")
}

func TestBuild_EpicDone(t *testing.T) {
	t.Run("every spec implemented", func(t *testing.T) {
		e := newEnv(t)
		e.epic("E", []epic.EpicSpec{{Name: "A", DependsOn: []string{}}, {Name: "B", DependsOn: []string{"A"}}})
		e.spec("A", "E")
		e.spec("B", "E")
		e.plan("A", "final", true)
		e.plan("B", "final", true, true)
		r, err := Build(e.opts, "E")
		require.NoError(t, err)
		require.True(t, r.Epic.Done)
		done, err := EpicComplete(e.opts, "E")
		require.NoError(t, err)
		require.True(t, done)
	})
	t.Run("a spec with no plan", func(t *testing.T) {
		e := newEnv(t)
		e.epic("E", []epic.EpicSpec{{Name: "A", DependsOn: []string{}}, {Name: "B", DependsOn: []string{}}})
		e.spec("A", "E")
		e.spec("B", "E")
		e.plan("A", "final", true)
		done, err := EpicComplete(e.opts, "E")
		require.NoError(t, err)
		require.False(t, done)
	})
	t.Run("a plan with open tasks", func(t *testing.T) {
		e := newEnv(t)
		e.epic("E", []epic.EpicSpec{{Name: "A", DependsOn: []string{}}})
		e.spec("A", "E")
		e.plan("A", "final", true, false)
		done, err := EpicComplete(e.opts, "E")
		require.NoError(t, err)
		require.False(t, done)
	})
	t.Run("no specs", func(t *testing.T) {
		e := newEnv(t)
		e.epic("E", []epic.EpicSpec{})
		done, err := EpicComplete(e.opts, "E")
		require.NoError(t, err)
		require.False(t, done)
	})
	t.Run("unknown epic", func(t *testing.T) {
		e := newEnv(t)
		_, err := EpicComplete(e.opts, "nope")
		var er *output.ErrorResponse
		require.True(t, errors.As(err, &er))
		require.Equal(t, "artifact_not_found", er.Code)
	})
}

func TestBuild_StandaloneSpec(t *testing.T) {
	e := newEnv(t)
	e.spec("S", "", "https://example.com/s")
	e.plan("S", "final", true, false)

	for _, name := range []string{"S"} {
		r, err := Build(e.opts, name)
		require.NoError(t, err)
		require.Nil(t, r.Epic)
		require.Len(t, r.Specs, 1)
		s := r.Specs[0]
		require.Equal(t, "S", s.Name)
		require.Equal(t, StateInProgress, s.State)
		require.Equal(t, []string{}, s.DependsOn)
		require.True(t, s.Ready)
		require.Equal(t, []metadata.SourceRef{{URI: "https://example.com/s", RetrievedDate: "2026-09-28"}}, s.Sources)
	}

	// The same top-level keys as an epic report.
	e.standardEpic()
	standalone, err := Build(e.opts, "S")
	require.NoError(t, err)
	inEpic, err := Build(e.opts, "B")
	require.NoError(t, err)
	want := []string{"epic", "requested", "specs", "workflow"}
	require.Equal(t, want, topLevelKeys(t, standalone))
	require.Equal(t, want, topLevelKeys(t, inEpic))
}

func TestBuild_PlanOfMissingSpecReportsItMissing(t *testing.T) {
	e := newEnv(t)
	e.plan("orphan", "draft", false)
	r, err := Build(e.opts, "orphan")
	require.NoError(t, err)
	require.Nil(t, r.Epic)
	require.Len(t, r.Specs, 1)
	require.Equal(t, StateMissing, r.Specs[0].State)
	require.NotNil(t, r.Specs[0].Plan)
}

func TestBuild_UnknownName(t *testing.T) {
	e := newEnv(t)
	_, err := Build(e.opts, "nothing")
	var er *output.ErrorResponse
	require.True(t, errors.As(err, &er))
	require.Equal(t, "artifact_not_found", er.Code)
	require.Equal(t, "nothing", er.Resource)
	for _, s := range []string{"epics", "specs", "plans"} {
		require.Contains(t, er.Message, s)
	}
	for _, s := range []string{"spektacular epic list", "spektacular spec file list", "spektacular plan file list"} {
		require.Contains(t, er.NextAction, s)
	}
}

func TestBuild_EffectiveSources(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()
	r, err := Build(e.opts, "B")
	require.NoError(t, err)

	require.Equal(t, []metadata.SourceRef{
		{URI: "https://example.com/b-source", RetrievedDate: "2026-09-28"},
		{URI: "https://example.com/epic-source", RetrievedDate: "2026-09-28"},
	}, specByName(t, r, "B").Sources, "own sources first, then the epic's")
	require.Equal(t, []metadata.SourceRef{
		{URI: "https://example.com/epic-source", RetrievedDate: "2026-09-28"},
	}, specByName(t, r, "A").Sources)
	require.Equal(t, []metadata.SourceRef{
		{URI: "https://example.com/epic-source", RetrievedDate: "2026-09-28"},
	}, specByName(t, r, "D").Sources, "a missing spec still carries its epic's sources")

	raw, err := os.ReadFile(filepath.Join(e.root, "specs/B.md"))
	require.NoError(t, err)
	fm, _, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Equal(t, []metadata.SourceRef{{URI: "https://example.com/b-source", RetrievedDate: "2026-09-28"}}, fm.Sources,
		"the stored record holds only the spec's own sources")
}

func TestBuild_StalePlan(t *testing.T) {
	e := newEnv(t)
	e.spec("S", "")
	e.plan("S", "final", true)
	old := time.Now().Add(-time.Hour)
	e.touch("plans/S/plan.md", old)
	e.touch("specs/S.md", old.Add(time.Minute))

	r, err := Build(e.opts, "S")
	require.NoError(t, err)
	require.Equal(t, StateImplemented, r.Specs[0].State, "staleness only counts under strict_spec_changes")

	e.opts.Config.Plan.StrictSpecChanges = true
	r, err = Build(e.opts, "S")
	require.NoError(t, err)
	s := r.Specs[0]
	require.Equal(t, StateStale, s.State)
	require.Equal(t, "stale", s.Plan.DocumentStatus)
	require.Equal(t, "stale", s.Plan.CurrentStep)
}

func TestBuild_WorkflowBlock(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()
	updated := time.Date(2026, 9, 30, 10, 12, 0, 0, time.UTC)

	t.Run("implement on a reported plan", func(t *testing.T) {
		e.opts.State = &workflow.State{Kind: "implement", CurrentStep: "analyze", CompletedSteps: []string{"new", "read_plan"}, UpdatedAt: updated, Data: map[string]any{"name": "B"}}
		r, err := Build(e.opts, "A")
		require.NoError(t, err)
		require.Equal(t, &WorkflowInfo{Kind: "implement", Name: "B", CurrentStep: "analyze", CompletedSteps: []string{"new", "read_plan"}, UpdatedAt: "2026-09-30T10:12:00Z"}, r.Workflow)
		require.Equal(t, "finished", specByName(t, r, "B").Plan.CurrentStep, "an implement workflow is not the plan's own step")
	})
	t.Run("spec workflow on a reported spec shows its live step", func(t *testing.T) {
		e.opts.State = &workflow.State{Kind: "spec", CurrentStep: "requirements", Data: map[string]any{"name": "C"}}
		r, err := Build(e.opts, "E")
		require.NoError(t, err)
		require.NotNil(t, r.Workflow)
		require.Equal(t, "requirements", specByName(t, r, "C").CurrentStep)
	})
	t.Run("workflow on something else", func(t *testing.T) {
		e.opts.State = &workflow.State{Kind: "spec", CurrentStep: "overview", Data: map[string]any{"name": "other"}}
		r, err := Build(e.opts, "E")
		require.NoError(t, err)
		require.Nil(t, r.Workflow)
		raw, err := json.Marshal(r)
		require.NoError(t, err)
		require.Contains(t, string(raw), `"workflow":null`)
	})
	t.Run("finished workflow", func(t *testing.T) {
		e.opts.State = &workflow.State{Kind: "plan", CurrentStep: "finished", Data: map[string]any{"name": "B"}}
		r, err := Build(e.opts, "E")
		require.NoError(t, err)
		require.Nil(t, r.Workflow)
	})
}

func TestBuildCurrent(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()

	t.Run("nothing in progress", func(t *testing.T) {
		e.opts.State = nil
		r, err := BuildCurrent(e.opts)
		require.NoError(t, err)
		raw, err := json.Marshal(r)
		require.NoError(t, err)
		require.JSONEq(t, `{"workflow": null}`, string(raw))
	})
	t.Run("a workflow in progress", func(t *testing.T) {
		e.opts.State = &workflow.State{Kind: "plan", CurrentStep: "milestones", Data: map[string]any{"name": "B"}}
		r, err := BuildCurrent(e.opts)
		require.NoError(t, err)
		require.Equal(t, "B", r.Requested)
		require.Equal(t, "plan", r.Workflow.Kind)
		require.Equal(t, "E", r.Epic.Name)
		require.Equal(t, "milestones", specByName(t, r, "B").Plan.CurrentStep)
	})
	t.Run("a workflow whose artifact is not written yet", func(t *testing.T) {
		e.opts.State = &workflow.State{Kind: "spec", CurrentStep: "overview", Data: map[string]any{"name": "unwritten"}}
		r, err := BuildCurrent(e.opts)
		require.NoError(t, err)
		raw, err := json.Marshal(r)
		require.NoError(t, err)
		require.JSONEq(t, `{"workflow": {"kind": "spec", "name": "unwritten", "current_step": "overview", "completed_steps": [], "updated_at": ""}}`, string(raw))
	})
}

func TestReadState(t *testing.T) {
	dir := t.TempDir()
	require.Nil(t, ReadState(filepath.Join(dir, "state.json")))
	p := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"kind":"spec","current_step":"overview","data":{"name":"X"}}`), 0o644))
	s := ReadState(p)
	require.NotNil(t, s)
	require.Equal(t, "spec", s.Kind)
	require.Equal(t, "X", stateName(s))
}

func TestDependenciesOf(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()
	e.spec("S", "")

	deps, err := DependenciesOf(e.opts, "C")
	require.NoError(t, err)
	require.Equal(t, "E", deps.Epic)
	require.Equal(t, []Dependency{{Name: "B", State: StateInProgress, Progress: TaskCounts{1, 3}, Description: "in progress (1/3 tasks complete)", Ready: true}}, deps.Dependencies)
	require.Len(t, deps.Unmet(), 1)

	deps, err = DependenciesOf(e.opts, "B")
	require.NoError(t, err)
	require.Empty(t, deps.Unmet())

	deps, err = DependenciesOf(e.opts, "A")
	require.NoError(t, err)
	require.Equal(t, "E", deps.Epic)
	require.Empty(t, deps.Dependencies)

	deps, err = DependenciesOf(e.opts, "S")
	require.NoError(t, err)
	require.Equal(t, SpecDependencies{}, deps, "a standalone spec has no dependencies")
}

func TestRenderPretty_Epic(t *testing.T) {
	e := newEnv(t)
	e.standardEpic()
	r, err := Build(e.opts, "B")
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, RenderPretty(&buf, r))
	want := strings.Join([]string{
		"epic E  (draft)  1/4 specs implemented, 3/5 tasks",
		"",
		"  A   implemented   2/2 tasks",
		"  B   in progress   1/3 tasks   ← requested",
		"      depends on: A",
		"      Milestone 1",
		"        [x] Task 1   spektacular   agent",
		"        [ ] Task 2   spektacular   agent",
		"            depends on: Task 1",
		"        [ ] Task 3   spektacular   agent",
		"            depends on: Task 2",
		"  C   specified     no plan",
		"      depends on: B",
		"  D   missing       no plan",
		"",
	}, "\n")
	require.Equal(t, want, buf.String())
}

func TestRenderPretty_StandaloneAndWorkflow(t *testing.T) {
	e := newEnv(t)
	e.spec("S", "")
	e.plan("S", "final", true)
	e.opts.State = &workflow.State{Kind: "implement", CurrentStep: "verify", Data: map[string]any{"name": "S"}}
	r, err := Build(e.opts, "S")
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, RenderPretty(&buf, r))
	require.Equal(t, strings.Join([]string{
		"workflow in progress: implement S, at step verify",
		"",
		"S   implemented   1/1 tasks   ← requested",
		"    Milestone 1",
		"      [x] Task 1   spektacular   agent",
		"",
	}, "\n"), buf.String())

	buf.Reset()
	require.NoError(t, RenderPretty(&buf, Report{}))
	require.Equal(t, "no workflow in progress\n", buf.String())
}
