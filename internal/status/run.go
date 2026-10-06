package status

import (
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/depgraph"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/hivecommons/spektacular/internal/worktree"
)

// The run states of one part — planning or implementing — of a spec.
const (
	RunDone          = "done"
	RunInProgress    = "in_progress"
	RunAwaitingMerge = "awaiting_merge"
	RunReady         = "ready"
	RunBlocked       = "blocked"
)

// The problems that block implementing an epic.
const (
	ProblemUnplanned  = "epic_unplanned"
	ProblemCycle      = "epic_dependency_cycle"
	ProblemOutsideDep = "epic_dependency_outside"
)

// RunSource is what the run view reads beyond the project store and the
// project's lanes. A nil RunSource leaves the run view out of the report, as
// the implement dependency check and the completed-epic guard do, so they
// stay as cheap as before.
type RunSource struct {
	// ProjectRoot is where the project's own workflows run.
	ProjectRoot string
	// Worktrees lists the project's spec worktrees.
	Worktrees func() ([]worktree.SpecWorktrees, error)
	// Touched lists the registered repos a spec's plan touches.
	Touched func(spec string) []string
	// Dirty reports whether any registered repo has uncommitted changes.
	Dirty func() bool
}

// RunPart is one part, planning or implementing, of a spec's run.
type RunPart struct {
	State       string   `json:"state"`
	WaitingOn   []string `json:"waiting_on,omitempty"`
	CurrentStep string   `json:"current_step,omitempty"`
	Root        string   `json:"root,omitempty"`
	Repos       []string `json:"repos,omitempty"`
}

// SpecRun is what a spec still needs, for planning and for implementing.
type SpecRun struct {
	Plan      RunPart `json:"plan"`
	Implement RunPart `json:"implement"`
}

// RunCounts totals one part across an epic.
type RunCounts struct {
	Done          int `json:"done"`
	InProgress    int `json:"in_progress"`
	AwaitingMerge int `json:"awaiting_merge"`
	Ready         int `json:"ready"`
	Blocked       int `json:"blocked"`
	Remaining     int `json:"remaining"`
}

// Problem is something wrong with an epic, with a stable code.
type Problem struct {
	Code    string   `json:"code"`
	Specs   []string `json:"specs"`
	Message string   `json:"message"`
	// Blocks names the parts it stops; planning is never among them.
	Blocks []string `json:"blocks"`
}

// EpicRun is where the epic as a whole stands, for planning and
// implementing.
type EpicRun struct {
	// Order is the dependency order; ties follow the epic's list order.
	Order     []string  `json:"order"`
	Plan      RunCounts `json:"plan"`
	Implement RunCounts `json:"implement"`
	Dirty     bool      `json:"dirty"`
	Problems  []Problem `json:"problems"`
}

// BlocksImplement reports whether any problem stops implementing.
func (r EpicRun) BlocksImplement() bool {
	for _, p := range r.Problems {
		for _, b := range p.Blocks {
			if b == "implement" {
				return true
			}
		}
	}
	return false
}

// buildRun adds the run view to a report: a run block on each spec and, for
// an epic, on the epic. Everything is read from what is on disk — the stores,
// the lanes and the worktrees — so asking again after an interruption
// resumes exactly where the work stands.
func buildRun(opts Options, r *Report) error {
	src := opts.Run
	worktrees := map[string]worktree.SpecWorktrees{}
	if src.Worktrees != nil {
		all, err := src.Worktrees()
		if err != nil {
			return err
		}
		for _, sw := range all {
			worktrees[sw.Spec] = sw
		}
	}

	members := make([]string, len(r.Specs))
	deps := make(map[string][]string, len(r.Specs))
	isMember := make(map[string]bool, len(r.Specs))
	for i, s := range r.Specs {
		members[i] = s.Name
		deps[s.Name] = s.DependsOn
		isMember[s.Name] = true
	}
	onCycle := map[string]bool{}
	cycle := depgraph.CycleMembers(members, deps)
	for _, n := range cycle {
		onCycle[n] = true
	}

	planDone := make(map[string]bool, len(r.Specs))
	implDone := make(map[string]bool, len(r.Specs))
	for _, s := range r.Specs {
		planDone[s.Name] = isPlanDone(opts, s)
		implDone[s.Name] = isImplDone(opts, s, worktrees)
	}

	// Outside dependencies: names an epic member depends on that the epic
	// does not list. One that is implemented is fine.
	outsideImplemented := map[string]bool{}
	var outside []string
	for _, s := range r.Specs {
		for _, d := range s.DependsOn {
			if isMember[d] {
				continue
			}
			if _, seen := outsideImplemented[d]; seen {
				continue
			}
			ext := buildSpec(withoutRun(opts), d, nil, nil)
			outsideImplemented[d] = ext.State == StateImplemented
			if ext.State != StateImplemented {
				outside = append(outside, d)
			}
		}
	}

	for i := range r.Specs {
		s := &r.Specs[i]
		run := &SpecRun{}
		run.Plan = planPart(opts, *s, planDone, isMember, onCycle)
		run.Implement = implementPart(opts, *s, worktrees, planDone, implDone, isMember, outsideImplemented)
		if src.Touched != nil {
			run.Implement.Repos = src.Touched(s.Name)
		}
		s.Run = run
	}

	if r.Epic == nil {
		return nil
	}
	er := &EpicRun{Order: depgraph.TopoOrder(members, deps), Problems: []Problem{}}
	if er.Order == nil {
		er.Order = []string{}
	}
	for _, s := range r.Specs {
		count(&er.Plan, s.Run.Plan.State)
		count(&er.Implement, s.Run.Implement.State)
	}
	if src.Dirty != nil {
		er.Dirty = src.Dirty()
	}

	implement := []string{"implement"}
	if len(cycle) > 0 {
		er.Problems = append(er.Problems, Problem{
			Code: ProblemCycle, Specs: cycle, Blocks: implement,
			Message: fmt.Sprintf("the dependencies of %s form a cycle, so they cannot be implemented in any order", strings.Join(cycle, ", ")),
		})
	}
	if len(outside) > 0 {
		var who []string
		for _, s := range r.Specs {
			for _, d := range s.DependsOn {
				if !isMember[d] && !outsideImplemented[d] {
					who = append(who, fmt.Sprintf("%s depends on %s", s.Name, d))
				}
			}
		}
		er.Problems = append(er.Problems, Problem{
			Code: ProblemOutsideDep, Specs: outside, Blocks: implement,
			Message: fmt.Sprintf("%s, which %s outside the epic and not implemented", strings.Join(who, "; "), isOrAre(len(outside))),
		})
	}
	var unplanned []string
	for _, s := range r.Specs {
		if !planDone[s.Name] {
			unplanned = append(unplanned, s.Name)
		}
	}
	if len(unplanned) > 0 {
		er.Problems = append(er.Problems, Problem{
			Code: ProblemUnplanned, Specs: unplanned, Blocks: implement,
			Message: fmt.Sprintf("%s %s no final plan yet; every spec must be planned before the epic is implemented", strings.Join(unplanned, ", "), hasOrHave(len(unplanned))),
		})
	}
	r.Epic.Run = er
	return nil
}

func withoutRun(opts Options) Options {
	opts.Run = nil
	return opts
}

func count(c *RunCounts, state string) {
	switch state {
	case RunDone:
		c.Done++
		return
	case RunInProgress:
		c.InProgress++
	case RunAwaitingMerge:
		c.AwaitingMerge++
	case RunReady:
		c.Ready++
	case RunBlocked:
		c.Blocked++
	}
	c.Remaining++
}

// isPlanDone: the spec has a plan that is final, and no plan workflow is in
// progress for it. A draft plan from an unfinished plan workflow is not
// planned.
func isPlanDone(opts Options, s SpecStatus) bool {
	if s.Plan == nil || s.Plan.DocumentStatus != string(metadata.StatusFinal) {
		return false
	}
	return planWorkflow(opts, s.Name) == nil
}

// planWorkflow is the plan workflow in progress for name: its lane, or the
// shared workflow when that is planning it.
func planWorkflow(opts Options, name string) *workflow.State {
	if lane := opts.lane("plan", name); lane != nil {
		return lane
	}
	if st := opts.State; st != nil && st.InProgress() && st.Kind == "plan" && stateName(st) == name {
		return st
	}
	return nil
}

// isImplDone: every task is ticked, the spec's final changelog record is
// written, no implement workflow is in progress for it anywhere, and no
// worktree of it is left unmerged.
func isImplDone(opts Options, s SpecStatus, worktrees map[string]worktree.SpecWorktrees) bool {
	if s.State != StateImplemented || !changelogFinal(opts, opts.Store, s.Name) {
		return false
	}
	if _, ok := worktrees[s.Name]; ok {
		return false
	}
	return implementWorkflow(opts, s.Name) == nil
}

// implementWorkflow is the implement workflow in progress for name in the
// project itself: its lane, or the shared workflow.
func implementWorkflow(opts Options, name string) *workflow.State {
	if lane := opts.lane("implement", name); lane != nil {
		return lane
	}
	if st := opts.State; st != nil && st.InProgress() && st.Kind == "implement" && stateName(st) == name {
		return st
	}
	return nil
}

// changelogFinal reports whether the spec's project changelog record in st is
// final: the implement workflow's wrap-up closes it last.
func changelogFinal(opts Options, st store.Reader, name string) bool {
	if st == nil {
		return false
	}
	raw, err := st.Read(artifact.Address{Kind: artifact.KindChangelog, Feature: name}.StorePath(opts.Config.Changelog.Config.Directory))
	if err != nil {
		return false
	}
	fm, _, err := metadata.Split(raw)
	return err == nil && fm != nil && fm.DocumentStatus == metadata.StatusFinal
}

// planPart decides the planning state. Dependencies on specs outside the
// epic, and between specs on a cycle, do not order planning: the tool never
// refuses to plan over dependencies.
func planPart(opts Options, s SpecStatus, planDone, isMember, onCycle map[string]bool) RunPart {
	if wf := planWorkflow(opts, s.Name); wf != nil {
		return RunPart{State: RunInProgress, CurrentStep: wf.CurrentStep, Root: opts.Run.ProjectRoot}
	}
	if planDone[s.Name] {
		return RunPart{State: RunDone}
	}
	var waiting []string
	for _, d := range s.DependsOn {
		if !isMember[d] || (onCycle[d] && onCycle[s.Name]) {
			continue
		}
		if !planDone[d] {
			waiting = append(waiting, d)
		}
	}
	if len(waiting) > 0 {
		return RunPart{State: RunBlocked, WaitingOn: waiting}
	}
	return RunPart{State: RunReady}
}

// implementPart decides the implementing state.
func implementPart(opts Options, s SpecStatus, worktrees map[string]worktree.SpecWorktrees, planDone, implDone, isMember, outsideImplemented map[string]bool) RunPart {
	if implDone[s.Name] {
		return RunPart{State: RunDone}
	}
	sw, hasWorktrees := worktrees[s.Name]
	if wf := implementWorkflow(opts, s.Name); wf != nil {
		// The run is always recorded in the project; a spec built in its
		// own worktrees reports them as where its code is.
		root := opts.Run.ProjectRoot
		if hasWorktrees {
			root = sw.Project
		}
		return RunPart{State: RunInProgress, CurrentStep: wf.CurrentStep, Root: root}
	}
	if hasWorktrees {
		if finishedInProject(opts, s.Name) {
			return RunPart{State: RunAwaitingMerge, Root: sw.Project}
		}
		// A worktree with no live run and unfinished work: the run was
		// interrupted, and resumes from its record in the project.
		return RunPart{State: RunInProgress, Root: sw.Project}
	}

	var waiting []string
	if !planDone[s.Name] {
		waiting = append(waiting, s.Name)
	}
	for _, d := range s.DependsOn {
		if isMember[d] {
			if !implDone[d] {
				waiting = append(waiting, d)
			}
		} else if !outsideImplemented[d] {
			waiting = append(waiting, d)
		}
	}
	if len(waiting) > 0 {
		return RunPart{State: RunBlocked, WaitingOn: waiting}
	}
	return RunPart{State: RunReady}
}

// finishedInProject reports whether the spec's implementation is complete,
// read from the project's own stores, as every implement run records it:
// every task ticked and its changelog record final. Nothing inside a spec's
// worktrees is read.
func finishedInProject(opts Options, name string) bool {
	if opts.Store == nil {
		return false
	}
	inner := withoutRun(opts)
	inner.State = nil
	inner.Lane = nil
	return buildSpec(inner, name, nil, nil).State == StateImplemented && changelogFinal(opts, opts.Store, name)
}

func isOrAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func hasOrHave(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}
