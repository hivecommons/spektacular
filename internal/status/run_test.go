package status

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/hivecommons/spektacular/internal/worktree"
)

// newRunEnv is newEnv with a changelog store and the run view switched on:
// no worktrees, and each worktree's store opened as a file store at its root.
func newRunEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.opts.Config.Changelog.Config.Directory = "changelog"
	e.opts.Run = &RunSource{
		ProjectRoot: e.root,
		StoreAt:     func(root string) store.Reader { return store.NewFileStore(root, "project") },
	}
	return e
}

// changelog writes name's changelog record with docStatus.
func (e *env) changelog(name, docStatus string) {
	e.t.Helper()
	e.write("changelog/"+name+".md", "---\ncreated_date: \"2026-09-30\"\ndocument_status: "+docStatus+"\n---\n\n# Changelog\n")
}

// worktreeOf adds a spec worktree for name to the run source and returns an
// env rooted at its project directory, for writing the worktree's own store.
func (e *env) worktreeOf(name string) *env {
	e.t.Helper()
	wt := &env{t: e.t, root: e.t.TempDir()}
	prev := e.opts.Run.Worktrees
	e.opts.Run.Worktrees = func() ([]worktree.SpecWorktrees, error) {
		var all []worktree.SpecWorktrees
		if prev != nil {
			got, err := prev()
			if err != nil {
				return nil, err
			}
			all = got
		}
		return append(all, worktree.SpecWorktrees{Spec: name, Project: wt.root}), nil
	}
	return wt
}

// implementLane writes a live implement lane for name under e's .spektacular.
func (e *env) implementLane(name, step string) {
	e.t.Helper()
	e.write(filepath.Join(".spektacular", workflow.LaneDir, "implement-"+name+".json"),
		`{"kind":"implement","current_step":"`+step+`","completed_steps":["new"],"data":{"name":"`+name+`","orchestrated":true}}`)
}

func members(names ...string) []epic.EpicSpec {
	out := make([]epic.EpicSpec, 0, len(names))
	for _, n := range names {
		out = append(out, epic.EpicSpec{Name: n, DependsOn: []string{}})
	}
	return out
}

func buildRunReport(t *testing.T, e *env, name string) Report {
	t.Helper()
	r, err := Build(e.opts, name)
	require.NoError(t, err)
	return r
}

// For planning, a spec is ready only once every spec it depends on has a
// final plan; a draft plan is not enough, and planned specs are done.
func TestRun_PlanReadinessFollowsFinalPlans(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", []epic.EpicSpec{
		{Name: "A", DependsOn: []string{}},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "C", DependsOn: []string{"B"}},
		{Name: "D", DependsOn: []string{"A", "B"}},
	})
	for _, n := range []string{"A", "B", "C", "D"} {
		e.spec(n, "E")
	}
	e.plan("A", "final", false)
	e.plan("B", "draft", false)

	r := buildRunReport(t, e, "E")
	require.Equal(t, RunPart{State: RunDone}, specByName(t, r, "A").Run.Plan)
	require.Equal(t, RunPart{State: RunReady}, specByName(t, r, "B").Run.Plan)
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"B"}}, specByName(t, r, "C").Run.Plan)
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"B"}}, specByName(t, r, "D").Run.Plan)
	require.Equal(t, RunCounts{Done: 1, Ready: 1, Blocked: 2, Remaining: 3}, r.Epic.Run.Plan)
}

// For implementing, a spec is ready only once every dependency has every
// task ticked, a final changelog record and no worktree; its own plan must
// be final too. Implemented specs are done.
func TestRun_ImplementReadinessFollowsFinishedDependencies(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", []epic.EpicSpec{
		{Name: "A", DependsOn: []string{}},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "C", DependsOn: []string{"B"}},
		{Name: "D", DependsOn: []string{}},
		{Name: "F", DependsOn: []string{"D"}},
	})
	for _, n := range []string{"A", "B", "C", "D", "F"} {
		e.spec(n, "E")
	}
	e.plan("A", "final", true, true)
	e.changelog("A", "final")
	e.plan("B", "final", false, false)
	// D has every task ticked but its changelog record is still a draft.
	e.plan("D", "final", true)
	e.changelog("D", "draft")
	e.plan("F", "final", false)

	r := buildRunReport(t, e, "E")
	require.Equal(t, RunPart{State: RunDone}, specByName(t, r, "A").Run.Implement)
	require.Equal(t, RunPart{State: RunReady}, specByName(t, r, "B").Run.Implement)
	// C has no plan of its own, and B is not implemented.
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"C", "B"}}, specByName(t, r, "C").Run.Implement)
	require.Equal(t, StateImplemented, specByName(t, r, "D").State)
	require.NotEqual(t, RunDone, specByName(t, r, "D").Run.Implement.State)
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"D"}}, specByName(t, r, "F").Run.Implement)

	// Once D's changelog is final, F is ready.
	e.changelog("D", "final")
	r = buildRunReport(t, e, "E")
	require.Equal(t, RunPart{State: RunDone}, specByName(t, r, "D").Run.Implement)
	require.Equal(t, RunPart{State: RunReady}, specByName(t, r, "F").Run.Implement)
	require.Equal(t, RunCounts{Done: 2, Ready: 2, Blocked: 1, Remaining: 3}, r.Epic.Run.Implement)
}

// A dependency finished but still in its worktree is not merged, so its
// dependents wait on it.
func TestRun_ImplementWaitsForAMerge(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", []epic.EpicSpec{
		{Name: "A", DependsOn: []string{}},
		{Name: "B", DependsOn: []string{"A"}},
	})
	e.spec("A", "E")
	e.spec("B", "E")
	e.plan("A", "final", true)
	e.changelog("A", "final")
	e.plan("B", "final", false)
	e.worktreeOf("A")

	r := buildRunReport(t, e, "E")
	require.NotEqual(t, RunDone, specByName(t, r, "A").Run.Implement.State)
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"A"}}, specByName(t, r, "B").Run.Implement)
}

// A live lane, in the project or in the spec's worktree, is in progress at
// its current step and root.
func TestRun_LiveLanesAreInProgress(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", members("A", "B", "C", "D"))
	for _, n := range []string{"A", "B", "C", "D"} {
		e.spec(n, "E")
	}
	// A's plan is final, but a plan lane is still running over it.
	e.plan("A", "final", false)
	e.plan("B", "final", false)
	e.plan("C", "final", false)
	e.plan("D", "final", false)
	e.opts.Lane = lanes(map[string]*workflow.State{
		"plan-A":      {Kind: "plan", CurrentStep: "discovery", Data: map[string]any{"name": "A", "orchestrated": true}},
		"implement-B": {Kind: "implement", CurrentStep: "analyze", Data: map[string]any{"name": "B", "orchestrated": true}},
	})
	// D runs in the shared state.json.
	e.opts.State = &workflow.State{Kind: "implement", CurrentStep: "verify", Data: map[string]any{"name": "D"}}
	wt := e.worktreeOf("C")
	wt.implementLane("C", "implement_task")

	r := buildRunReport(t, e, "E")
	require.Equal(t, RunPart{State: RunInProgress, CurrentStep: "discovery", Root: e.root}, specByName(t, r, "A").Run.Plan)
	require.Equal(t, RunPart{State: RunInProgress, CurrentStep: "analyze", Root: e.root}, specByName(t, r, "B").Run.Implement)
	require.Equal(t, RunPart{State: RunInProgress, CurrentStep: "implement_task", Root: wt.root}, specByName(t, r, "C").Run.Implement)
	require.Equal(t, RunPart{State: RunInProgress, CurrentStep: "verify", Root: e.root}, specByName(t, r, "D").Run.Implement)
	require.Equal(t, 3, r.Epic.Run.Implement.InProgress)
	require.Equal(t, 1, r.Epic.Run.Plan.InProgress)
	// A plan lane in progress means the plan is not done, so A is unplanned.
	require.Equal(t, []string{"A"}, problemByCode(t, r.Epic.Run, ProblemUnplanned).Specs)
}

// A worktree whose own store shows the work finished is awaiting merge; one
// with no live lane and unfinished work is an interrupted run, in progress
// with no step.
func TestRun_FinishedButUnmergedIsAwaitingMerge(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", members("A", "B"))
	e.spec("A", "E")
	e.spec("B", "E")
	e.plan("A", "final", false)
	e.plan("B", "final", false)

	done := e.worktreeOf("A")
	done.spec("A", "E")
	done.plan("A", "final", true, true)
	done.changelog("A", "final")

	half := e.worktreeOf("B")
	half.spec("B", "E")
	half.plan("B", "final", true, false)

	r := buildRunReport(t, e, "E")
	require.Equal(t, RunPart{State: RunAwaitingMerge, Root: done.root}, specByName(t, r, "A").Run.Implement)
	require.Equal(t, RunPart{State: RunInProgress, Root: half.root}, specByName(t, r, "B").Run.Implement)
	require.Equal(t, RunCounts{InProgress: 1, AwaitingMerge: 1, Remaining: 2}, r.Epic.Run.Implement)

	var buf bytes.Buffer
	require.NoError(t, RenderPretty(&buf, r))
	require.Contains(t, buf.String(), "  implementing: 0 done, 1 in progress, 1 awaiting merge, 0 ready, 0 blocked\n")
}

func problemByCode(t *testing.T, run *EpicRun, code string) Problem {
	t.Helper()
	for _, p := range run.Problems {
		if p.Code == code {
			return p
		}
	}
	t.Fatalf("problem %q not reported: %+v", code, run.Problems)
	return Problem{}
}

// problemsEpic is E with A and B depending on each other, C depending on
// Outside (specified, not implemented), Ghost (no spec at all) and Fin
// (outside, implemented), and only C planned.
func (e *env) problemsEpic() {
	e.epic("E", []epic.EpicSpec{
		{Name: "A", DependsOn: []string{"B"}},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "C", DependsOn: []string{"Outside", "Ghost", "Fin"}},
	})
	for _, n := range []string{"A", "B", "C"} {
		e.spec(n, "E")
	}
	e.plan("C", "final", false)
	e.spec("Outside", "")
	e.spec("Fin", "")
	e.plan("Fin", "final", true)
}

// An unplanned spec, a cycle and an unimplemented outside dependency are each
// a problem that blocks implementing and names its specs.
func TestRun_EpicProblemsBlockImplementing(t *testing.T) {
	e := newRunEnv(t)
	e.problemsEpic()

	r := buildRunReport(t, e, "E")
	require.Equal(t, []Problem{
		{
			Code: ProblemCycle, Specs: []string{"A", "B"}, Blocks: []string{"implement"},
			Message: "the dependencies of A, B form a cycle, so they cannot be implemented in any order",
		},
		{
			Code: ProblemOutsideDep, Specs: []string{"Outside", "Ghost"}, Blocks: []string{"implement"},
			Message: "C depends on Outside; C depends on Ghost, which are outside the epic and not implemented",
		},
		{
			Code: ProblemUnplanned, Specs: []string{"A", "B"}, Blocks: []string{"implement"},
			Message: "A, B have no final plan yet; every spec must be planned before the epic is implemented",
		},
	}, r.Epic.Run.Problems)
	require.True(t, r.Epic.Run.BlocksImplement())
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"Outside", "Ghost"}}, specByName(t, r, "C").Run.Implement)
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"A", "B"}}, specByName(t, r, "A").Run.Implement)

	var buf bytes.Buffer
	require.NoError(t, RenderPretty(&buf, r))
	out := buf.String()
	require.Contains(t, out, "  problem (epic_dependency_cycle): the dependencies of A, B form a cycle")
	require.Contains(t, out, "  problem (epic_dependency_outside): C depends on Outside; C depends on Ghost")
	require.Contains(t, out, "  problem (epic_unplanned): A, B have no final plan yet")
}

// One unplanned spec, and one outside dependency, word the message in the
// singular.
func TestRun_SingleProblemsAreSingular(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", []epic.EpicSpec{{Name: "A", DependsOn: []string{"Outside"}}})
	e.spec("A", "E")
	e.spec("Outside", "")

	r := buildRunReport(t, e, "E")
	require.Equal(t, "A depends on Outside, which is outside the epic and not implemented", problemByCode(t, r.Epic.Run, ProblemOutsideDep).Message)
	require.Equal(t, "A has no final plan yet; every spec must be planned before the epic is implemented", problemByCode(t, r.Epic.Run, ProblemUnplanned).Message)
}

// A clean, fully planned epic has no problems, as an empty list.
func TestRun_CleanEpicHasNoProblems(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", members("A"))
	e.spec("A", "E")
	e.plan("A", "final", false)

	r := buildRunReport(t, e, "E")
	require.Equal(t, []Problem{}, r.Epic.Run.Problems)
	require.False(t, r.Epic.Run.BlocksImplement())
}

// Problems never block planning: specs on a cycle, and specs with outside
// dependencies, are still ready to plan.
func TestRun_ProblemsNeverBlockPlanning(t *testing.T) {
	e := newRunEnv(t)
	e.problemsEpic()

	r := buildRunReport(t, e, "E")
	require.Equal(t, RunPart{State: RunReady}, specByName(t, r, "A").Run.Plan)
	require.Equal(t, RunPart{State: RunReady}, specByName(t, r, "B").Run.Plan)
	require.Equal(t, RunPart{State: RunDone}, specByName(t, r, "C").Run.Plan)
	for _, p := range r.Epic.Run.Problems {
		require.NotContains(t, p.Blocks, "plan")
	}
}

// A spec depending on a cycle, without being on it, still waits for the
// cycle's plans.
func TestRun_DependingOnACycleStillOrdersPlanning(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", []epic.EpicSpec{
		{Name: "A", DependsOn: []string{"B"}},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "D", DependsOn: []string{"A"}},
	})
	for _, n := range []string{"A", "B", "D"} {
		e.spec(n, "E")
	}
	r := buildRunReport(t, e, "E")
	require.Equal(t, RunPart{State: RunBlocked, WaitingOn: []string{"A"}}, specByName(t, r, "D").Run.Plan)
	require.Equal(t, []string{"A", "B"}, problemByCode(t, r.Epic.Run, ProblemCycle).Specs)
}

// The run view adds to the report and changes nothing already there; without
// it, no run key appears in the JSON.
func TestRun_LeavesExistingFieldsUnchanged(t *testing.T) {
	for _, name := range []string{"E", "C", "S"} {
		t.Run(name, func(t *testing.T) {
			e := newRunEnv(t)
			e.problemsEpic()
			e.spec("S", "")
			e.opts.Lane = lanes(map[string]*workflow.State{
				"plan-A": {Kind: "plan", CurrentStep: "discovery", Data: map[string]any{"name": "A", "orchestrated": true}},
			})

			with := buildRunReport(t, e, name)
			run := e.opts.Run
			e.opts.Run = nil
			without := buildRunReport(t, e, name)
			e.opts.Run = run

			if with.Epic != nil {
				require.NotNil(t, with.Epic.Run)
				with.Epic.Run = nil
			}
			for i := range with.Specs {
				require.NotNil(t, with.Specs[i].Run)
				with.Specs[i].Run = nil
			}
			require.Equal(t, without, with)

			raw, err := json.Marshal(without)
			require.NoError(t, err)
			var m map[string]any
			require.NoError(t, json.Unmarshal(raw, &m))
			if ep, ok := m["epic"].(map[string]any); ok {
				require.NotContains(t, ep, "run")
			}
			for _, s := range m["specs"].([]any) {
				require.NotContains(t, s.(map[string]any), "run")
			}
		})
	}
}

// The order follows dependencies, with ties in the epic's list order.
func TestRun_OrderFollowsDependenciesThenListOrder(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", []epic.EpicSpec{
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "A", DependsOn: []string{}},
		{Name: "C", DependsOn: []string{}},
	})
	for _, n := range []string{"A", "B", "C"} {
		e.spec(n, "E")
	}
	r := buildRunReport(t, e, "E")
	require.Equal(t, []string{"A", "B", "C"}, r.Epic.Run.Order)
}

// Dirty and the touched repos come straight from the run source.
func TestRun_DirtyAndReposPassThrough(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", members("A", "B"))
	e.spec("A", "E")
	e.spec("B", "E")
	touched := map[string][]string{"A": {"api", "web"}, "B": {"api"}}
	e.opts.Run.Touched = func(spec string) []string { return touched[spec] }

	r := buildRunReport(t, e, "E")
	require.False(t, r.Epic.Run.Dirty, "no Dirty func is clean")
	require.Equal(t, []string{"api", "web"}, specByName(t, r, "A").Run.Implement.Repos)
	require.Equal(t, []string{"api"}, specByName(t, r, "B").Run.Implement.Repos)
	require.Empty(t, r.Epic.Run.DirtyRepos)
	require.Nil(t, specByName(t, r, "A").Run.Plan.Repos, "repos are for implementing only")

	e.opts.Run.Dirty = func(repos []string) []string {
		require.Equal(t, []string{"api", "web"}, repos, "only the union of touched repos is checked")
		return []string{"web"}
	}
	r = buildRunReport(t, e, "E")
	require.True(t, r.Epic.Run.Dirty)
	require.Equal(t, []string{"web"}, r.Epic.Run.DirtyRepos)

	e.opts.Run.Dirty = func(repos []string) []string { return nil }
	r = buildRunReport(t, e, "E")
	require.False(t, r.Epic.Run.Dirty)
	require.Equal(t, []string{}, r.Epic.Run.DirtyRepos)
}

// A standalone spec gets a run block; its epic stays null.
func TestRun_StandaloneSpec(t *testing.T) {
	e := newRunEnv(t)
	e.spec("S", "")

	r := buildRunReport(t, e, "S")
	require.Nil(t, r.Epic)
	require.Len(t, r.Specs, 1)
	require.Equal(t, &SpecRun{
		Plan:      RunPart{State: RunReady},
		Implement: RunPart{State: RunBlocked, WaitingOn: []string{"S"}},
	}, r.Specs[0].Run)

	raw, err := json.Marshal(r)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Nil(t, m["epic"])
	require.Equal(t, map[string]any{
		"plan":      map[string]any{"state": "ready"},
		"implement": map[string]any{"state": "blocked", "waiting_on": []any{"S"}},
	}, m["specs"].([]any)[0].(map[string]any)["run"])
}

// A worktree listing that fails fails the report.
func TestRun_WorktreeListingErrorIsReturned(t *testing.T) {
	e := newRunEnv(t)
	e.spec("S", "")
	boom := errors.New("boom")
	e.opts.Run.Worktrees = func() ([]worktree.SpecWorktrees, error) { return nil, boom }
	_, err := Build(e.opts, "S")
	require.ErrorIs(t, err, boom)
}

// The epic header in pretty output carries the planning and implementing
// totals.
func TestRun_PrettyShowsPlanningAndImplementing(t *testing.T) {
	e := newRunEnv(t)
	e.epic("E", []epic.EpicSpec{
		{Name: "A", DependsOn: []string{}},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "C", DependsOn: []string{}},
	})
	for _, n := range []string{"A", "B", "C"} {
		e.spec(n, "E")
	}
	e.plan("A", "final", true)
	e.changelog("A", "final")
	e.plan("B", "final", false)
	e.plan("C", "final", false)

	r := buildRunReport(t, e, "E")
	var buf bytes.Buffer
	require.NoError(t, RenderPretty(&buf, r))
	out := buf.String()
	require.Contains(t, out, "\n  planning: 3 done, 0 in progress, 0 ready, 0 blocked\n")
	require.Contains(t, out, "\n  implementing: 1 done, 0 in progress, 2 ready, 0 blocked\n")
	require.NotContains(t, out, "problem (")

	// Without the run view, neither line appears.
	e.opts.Run = nil
	buf.Reset()
	require.NoError(t, RenderPretty(&buf, buildRunReport(t, e, "E")))
	require.NotContains(t, buf.String(), "planning:")
	require.NotContains(t, buf.String(), "implementing:")
}
