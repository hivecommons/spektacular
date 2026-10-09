package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// Report is the one shape `status` emits. Whatever was asked, it carries an
// epic (null for a standalone spec) and a specs list (one entry for a
// standalone spec), so callers never branch on kind.
type Report struct {
	// Workflow is the workflow in progress when it belongs to one of the
	// reported specs or plans; null otherwise.
	Workflow *WorkflowInfo `json:"workflow"`
	// Workflows is every workflow in progress, the shared one first and then
	// each lane, most recently updated first. Only the no-name report sets
	// it; Workflow is its first entry.
	Workflows []WorkflowInfo `json:"workflows,omitempty"`
	Requested string         `json:"requested,omitempty"`
	Epic      *EpicStatus    `json:"epic"`
	Specs     []SpecStatus   `json:"specs"`
}

// MarshalJSON writes a report with no specs, which is the no-name report
// when nothing is in progress, as {"workflow": null} alone.
func (r Report) MarshalJSON() ([]byte, error) {
	if r.Specs == nil {
		return json.Marshal(struct {
			Workflow  *WorkflowInfo  `json:"workflow"`
			Workflows []WorkflowInfo `json:"workflows,omitempty"`
		}{r.Workflow, r.Workflows})
	}
	type plain Report
	return json.Marshal(plain(r))
}

// WorkflowInfo is the workflow in progress, read from the state file.
type WorkflowInfo struct {
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	CurrentStep    string   `json:"current_step"`
	CompletedSteps []string `json:"completed_steps"`
	UpdatedAt      string   `json:"updated_at"`
	// Orchestrated marks a workflow an epic orchestrator started, which runs
	// in its own lane beside any standalone workflow.
	Orchestrated bool `json:"orchestrated,omitempty"`
}

// EpicStatus is an epic's lifecycle and roll-up. The roll-up is derived on
// every call and never written back.
type EpicStatus struct {
	Name           string               `json:"name"`
	DocumentStatus string               `json:"document_status"`
	CreatedAt      string               `json:"created_at"`
	Done           bool                 `json:"done"`
	Progress       EpicProgress         `json:"progress"`
	Sources        []metadata.SourceRef `json:"sources"`
	// Run is where the epic stands for planning and implementing; present
	// when the run view was asked for.
	Run *EpicRun `json:"run,omitempty"`
}

// EpicProgress totals an epic: specs implemented of total, and tasks
// complete of total across every plan with task structure.
type EpicProgress struct {
	SpecsImplemented int `json:"specs_implemented"`
	SpecsTotal       int `json:"specs_total"`
	TasksCompleted   int `json:"tasks_completed"`
	TasksTotal       int `json:"tasks_total"`
}

// SpecStatus is one spec: its lifecycle, derived state, dependencies and
// plan.
type SpecStatus struct {
	Name           string    `json:"name"`
	State          SpecState `json:"state"`
	DocumentStatus string    `json:"document_status"`
	CurrentStep    string    `json:"current_step"`
	DependsOn      []string  `json:"depends_on"`
	Ready          bool      `json:"ready"`
	BlockedBy      []string  `json:"blocked_by"`
	// Sources are effective: the spec's own, then its epic's. The spec's
	// stored record holds only its own.
	Sources []metadata.SourceRef `json:"sources"`
	// Plan is null when the spec has no plan.
	Plan *PlanStatus `json:"plan"`
	// Run is what the spec still needs; present when the run view was asked
	// for.
	Run *SpecRun `json:"run,omitempty"`

	// counts is the plan's progress whatever its format, for the tree.
	counts TaskCounts
	// legacy is set when the plan's work is phases rather than tasks.
	legacy bool
}

// PlanStatus is a spec's plan. Progress and Tasks are absent for a plan
// without task structure.
type PlanStatus struct {
	Name           string        `json:"name"`
	DocumentStatus string        `json:"document_status"`
	CurrentStep    string        `json:"current_step"`
	Progress       *PlanProgress `json:"progress,omitempty"`
	Tasks          []TaskStatus  `json:"tasks,omitempty"`
}

// PlanProgress totals a task-format plan's tasks.
type PlanProgress struct {
	TasksCompleted int `json:"tasks_completed"`
	TasksTotal     int `json:"tasks_total"`
}

// TaskStatus is one task: the plan-export task fields plus its
// acceptance-criteria counts. Completion and criteria are separate facts: a
// task can be complete with a criterion unmet.
type TaskStatus struct {
	plantask.ExportTask
	AcceptanceCriteria plantask.Criteria `json:"acceptance_criteria"`
}

// Options is what every report is built from.
type Options struct {
	Config config.Config
	Store  store.Reader
	// State is the workflow state file's contents; nil when there is none.
	State *workflow.State
	// Locate maps a registered repo name to its declared git source; nil
	// reports every location as "".
	Locate func(repo string) string
	// Lane reads the orchestrated workflow of kind for a spec — its lane —
	// or returns nil when it has none. nil reports no lanes.
	Lane func(kind, name string) *workflow.State
	// LaneNames lists the spec names with a lane of kind, so the no-name
	// report can find every lane in progress. nil lists none.
	LaneNames func(kind string) []string
	// Run, when set, adds the run view: what each spec, and the epic, still
	// needs for planning and implementing.
	Run *RunSource
	// Unmerged reports whether a spec's worktrees are still unmerged: its
	// work is done but not yet in the main line. nil reports nothing
	// unmerged.
	Unmerged func(spec string) bool
}

// unmerged reports whether name's worktrees are still unmerged.
func (o Options) unmerged(name string) bool {
	return o.Unmerged != nil && o.Unmerged(name)
}

// metBy reports whether a dependency in state on spec name is met: it is
// implemented and its work has been merged back.
func (o Options) metBy(name string, state SpecState) bool {
	return state == StateImplemented && !o.unmerged(name)
}

// lane is the in-progress orchestrated workflow of kind for name, or nil.
func (o Options) lane(kind, name string) *workflow.State {
	if o.Lane == nil || (kind != "plan" && kind != "implement") {
		return nil
	}
	if s := o.Lane(kind, name); s != nil && s.InProgress() {
		return s
	}
	return nil
}

// Build reports on name, which may be an epic, a spec or a plan (see
// Resolve).
func Build(opts Options, name string) (Report, error) {
	target, err := Resolve(opts.Config, opts.Store, name)
	if err != nil {
		return Report{}, err
	}
	r, err := buildTarget(opts, target)
	if err != nil {
		return Report{}, err
	}
	r.Requested = name
	return r, nil
}

// BuildCurrent reports on the workflows in progress: the same report as Build
// for the first one's artifact, with the workflow block set to it and every
// workflow in progress listed in Workflows. With nothing in progress it
// is a report holding only a null workflow. A workflow whose artifact has not
// been written yet, such as a spec workflow before its spec is saved, is a
// report holding only the workflow block.
func BuildCurrent(opts Options) (Report, error) {
	active := activeWorkflows(opts)
	if len(active) == 0 {
		return Report{}, nil
	}
	primary := active[0]
	r, err := Build(opts, primary.Name)
	var notFound *output.ErrorResponse
	if errors.As(err, &notFound) && notFound.Code == "artifact_not_found" {
		return Report{Workflow: &primary, Workflows: active}, nil
	}
	if err != nil {
		return Report{}, err
	}
	r.Workflow = &primary
	r.Workflows = active
	return r, nil
}

// activeWorkflows is every workflow in progress: the shared workflow first,
// then each plan and implement lane, most recently updated first. A lane is
// where a run keeps its state when it is orchestrated or builds in its own
// worktrees, so the shared state alone misses most implement runs.
func activeWorkflows(opts Options) []WorkflowInfo {
	var active []WorkflowInfo
	seen := map[string]bool{}
	if s := opts.State; s != nil && s.InProgress() && stateName(s) != "" {
		info := workflowInfo(s)
		active = append(active, *info)
		seen[info.Kind+"-"+info.Name] = true
	}
	if opts.LaneNames == nil {
		return active
	}
	var lanes []WorkflowInfo
	updated := map[string]time.Time{}
	for _, kind := range []string{"plan", "implement"} {
		for _, name := range opts.LaneNames(kind) {
			lane := opts.lane(kind, name)
			if lane == nil || seen[kind+"-"+name] {
				continue
			}
			seen[kind+"-"+name] = true
			lanes = append(lanes, *laneInfo(kind, name, lane))
			updated[kind+"-"+name] = lane.UpdatedAt
		}
	}
	sort.SliceStable(lanes, func(i, j int) bool {
		return updated[lanes[i].Kind+"-"+lanes[i].Name].After(updated[lanes[j].Kind+"-"+lanes[j].Name])
	})
	return append(active, lanes...)
}

// EpicComplete reports whether the named epic is done: it has specs, and
// every one is implemented.
func EpicComplete(opts Options, epicName string) (bool, error) {
	r, err := buildTarget(opts, Target{Epic: epicName})
	if err != nil {
		return false, err
	}
	return r.Epic.Done, nil
}

// Dependency is one direct dependency of a spec and its state.
type Dependency struct {
	Name     string
	State    SpecState
	Progress TaskCounts
	// Description is Describe(State, Progress).
	Description string
	// Ready is true when every one of the dependency's own dependencies is
	// implemented, so it could itself be implemented now.
	Ready bool
	// Unmerged is true when the dependency is implemented but its worktrees
	// are not merged yet, so its code is not in the main line.
	Unmerged bool
}

// SpecDependencies is what the implement check needs about a spec.
type SpecDependencies struct {
	// Epic is the spec's epic, empty for a standalone spec.
	Epic string
	// Dependencies are the spec's direct dependencies in the epic's order;
	// empty when it has none or is standalone.
	Dependencies []Dependency
}

// Unmet returns the dependencies that are not implemented, or implemented
// but not yet merged, in order.
func (d SpecDependencies) Unmet() []Dependency {
	var unmet []Dependency
	for _, dep := range d.Dependencies {
		if dep.State != StateImplemented || dep.Unmerged {
			unmet = append(unmet, dep)
		}
	}
	return unmet
}

// DependenciesOf classifies each direct dependency of the named spec through
// the same function status uses. A standalone spec, or one its epic lists
// without dependencies, has none.
func DependenciesOf(opts Options, specName string) (SpecDependencies, error) {
	target, ok, err := resolveSpec(opts.Config, opts.Store, specName)
	if err != nil {
		return SpecDependencies{}, err
	}
	if !ok || target.Epic == "" {
		return SpecDependencies{}, nil
	}
	r, err := buildTarget(opts, target)
	if err != nil {
		return SpecDependencies{}, err
	}
	out := SpecDependencies{Epic: target.Epic}
	byName := make(map[string]SpecStatus, len(r.Specs))
	for _, s := range r.Specs {
		byName[s.Name] = s
	}
	me, ok := byName[specName]
	if !ok {
		return out, nil
	}
	for _, d := range me.DependsOn {
		s, ok := byName[d]
		dep := Dependency{Name: d, State: StateMissing}
		if ok {
			dep.State, dep.Progress, dep.Ready = s.State, s.counts, s.Ready
		}
		dep.Unmerged = dep.State == StateImplemented && opts.unmerged(d)
		dep.Description = DescribeDependency(dep.State, dep.Progress, dep.Unmerged)
		out.Dependencies = append(out.Dependencies, dep)
	}
	return out, nil
}

// ReadState reads the workflow state file at path. A missing or unreadable
// file is no state.
func ReadState(path string) *workflow.State {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s workflow.State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	return &s
}

// RepoLocations maps a registered repo name to its declared git source: the
// location in its repo.yaml when that source's provider is git, and "" for a
// file source, no source, or a repo that is not on disk. It never clones and
// never asks git, so a checkout's remotes are never mistaken for a declared
// location.
func RepoLocations(cfg config.Config, root string, git repo.GitRunner) func(string) string {
	set, err := repo.New(cfg, root, git)
	if err != nil {
		return func(string) string { return "" }
	}
	return func(name string) string {
		meta, ok := set.DescriptiveMetadata(name)
		if !ok {
			return ""
		}
		kind, location, err := meta.ParseSource("")
		if err != nil || kind != config.SourceGit {
			return ""
		}
		return location
	}
}

// buildTarget assembles the report for a resolved target, without Requested.
func buildTarget(opts Options, target Target) (Report, error) {
	var r Report
	if target.Epic == "" {
		r.Specs = []SpecStatus{buildSpec(opts, target.Spec, []string{}, nil)}
	} else {
		e, err := readEpic(opts.Config, opts.Store, target.Epic)
		if err != nil {
			return Report{}, err
		}
		r.Epic = epicStatus(target.Epic, e)
		r.Specs = make([]SpecStatus, 0, len(e.Specs))
		for _, member := range e.Specs {
			deps := append([]string{}, member.DependsOn...)
			r.Specs = append(r.Specs, buildSpec(opts, member.Name, deps, e.Sources))
		}
	}

	states := make(map[string]SpecState, len(r.Specs))
	for _, s := range r.Specs {
		states[s.Name] = s.State
	}
	for i := range r.Specs {
		s := &r.Specs[i]
		s.BlockedBy = []string{}
		for _, d := range s.DependsOn {
			if !opts.metBy(d, states[d]) {
				s.BlockedBy = append(s.BlockedBy, d)
			}
		}
		s.Ready = len(s.BlockedBy) == 0
	}

	if r.Epic != nil {
		p := &r.Epic.Progress
		p.SpecsTotal = len(r.Specs)
		for _, s := range r.Specs {
			if s.State == StateImplemented {
				p.SpecsImplemented++
			}
			if s.Plan != nil && s.Plan.Progress != nil {
				p.TasksCompleted += s.Plan.Progress.TasksCompleted
				p.TasksTotal += s.Plan.Progress.TasksTotal
			}
		}
		r.Epic.Done = p.SpecsTotal > 0 && p.SpecsImplemented == p.SpecsTotal
	}

	r.Workflow = matchingWorkflow(opts, r.Specs)
	if opts.Run != nil {
		if err := buildRun(opts, &r); err != nil {
			return Report{}, err
		}
	}
	return r, nil
}

func epicStatus(name string, e epic.Epic) *EpicStatus {
	sources := append([]metadata.SourceRef{}, e.Sources...)
	return &EpicStatus{
		Name:           name,
		DocumentStatus: string(e.DocumentStatus),
		CreatedAt:      dateAsRFC3339(e.CreatedDate),
		Sources:        sources,
	}
}

// buildSpec reports one spec. A spec that cannot be read is reported as
// missing rather than failing, so one broken link never hides the rest of an
// epic.
func buildSpec(opts Options, name string, dependsOn []string, epicSources []metadata.SourceRef) SpecStatus {
	s := SpecStatus{Name: name, DependsOn: dependsOn, Sources: []metadata.SourceRef{}}

	readable := false
	if raw, err := opts.Store.Read(specPath(opts.Config, name)); err == nil {
		if fm, _, err := metadata.Split(raw); err == nil {
			readable = true
			if fm != nil {
				s.DocumentStatus = string(fm.DocumentStatus)
				s.Sources = append(s.Sources, fm.Sources...)
			}
			s.CurrentStep = currentStep(opts, "spec", name, metadata.DocumentStatus(s.DocumentStatus), fm != nil)
		}
	}
	s.Sources = append(s.Sources, epicSources...)

	facts := PlanFacts{}
	s.Plan, facts = buildPlan(opts, name)
	s.State, s.counts = Classify(readable, facts)
	s.legacy = facts.Exists && facts.Parsed.Format == plantask.FormatLegacy
	return s
}

// buildPlan reports the spec's plan, which shares its name; nil when there is
// none.
func buildPlan(opts Options, name string) (*PlanStatus, PlanFacts) {
	raw, err := opts.Store.Read(planPath(opts.Config, name))
	if err != nil {
		return nil, PlanFacts{}
	}
	fm, body, err := metadata.Split(raw)
	if err != nil {
		fm, body = nil, raw
	}
	parsed := plantask.Parse(body)
	docStatus := DocumentStatus(opts.Config, opts.Store, name, fm)
	p := &PlanStatus{
		Name:           name,
		DocumentStatus: string(docStatus),
		CurrentStep:    currentStep(opts, "plan", name, docStatus, fm != nil),
	}
	if parsed.Format == plantask.FormatTasks {
		locate := opts.Locate
		if locate == nil {
			locate = func(string) string { return "" }
		}
		export := plantask.NewExport(name, p.DocumentStatus, parsed, locate)
		p.Progress = &PlanProgress{TasksTotal: len(export.Tasks)}
		p.Tasks = make([]TaskStatus, len(export.Tasks))
		for i, t := range export.Tasks {
			if t.Completed {
				p.Progress.TasksCompleted++
			}
			p.Tasks[i] = TaskStatus{ExportTask: t, AcceptanceCriteria: parsed.Tasks[i].Criteria}
		}
	}
	return p, PlanFacts{Exists: true, Stale: docStatus == metadata.StatusStale, Parsed: parsed}
}

// currentStep is the live step when a workflow in progress is this kind
// working on this name — the shared one, or the name's own lane; otherwise
// "stale" for a stale document, "finished" for a closed one, and "" for one
// still open with no live workflow.
func currentStep(opts Options, kind, name string, docStatus metadata.DocumentStatus, hasFrontmatter bool) string {
	if s := opts.State; s != nil && s.InProgress() && s.Kind == kind && stateName(s) == name {
		return s.CurrentStep
	}
	if lane := opts.lane(kind, name); lane != nil {
		return lane.CurrentStep
	}
	if docStatus == metadata.StatusStale {
		return "stale"
	}
	if hasFrontmatter && isClosed(docStatus) {
		return "finished"
	}
	return ""
}

// matchingWorkflow is the workflow block when the shared workflow in
// progress works on one of the reported specs or plans, or else when one of
// them has a plan or implement lane in progress (the first, in
// report order).
func matchingWorkflow(opts Options, specs []SpecStatus) *WorkflowInfo {
	if s := opts.State; s != nil && s.InProgress() {
		name := stateName(s)
		for _, spec := range specs {
			if spec.Name == name || (spec.Plan != nil && spec.Plan.Name == name) {
				return workflowInfo(s)
			}
		}
	}
	for _, spec := range specs {
		for _, kind := range []string{"plan", "implement"} {
			if lane := opts.lane(kind, spec.Name); lane != nil {
				return laneInfo(kind, spec.Name, lane)
			}
		}
	}
	return nil
}

// laneInfo is the workflow block for the kind lane of name. It is marked
// orchestrated only when an epic orchestrator started it; an interactive run
// that builds in its own worktrees keeps a lane too.
func laneInfo(kind, name string, lane *workflow.State) *WorkflowInfo {
	info := workflowInfo(lane)
	info.Kind = kind
	info.Name = name
	info.Orchestrated, _ = lane.Data["orchestrated"].(bool)
	return info
}

func workflowInfo(s *workflow.State) *WorkflowInfo {
	return &WorkflowInfo{
		Kind:           s.Kind,
		Name:           stateName(s),
		CurrentStep:    s.CurrentStep,
		CompletedSteps: append([]string{}, s.CompletedSteps...),
		UpdatedAt:      timestampAsRFC3339(s.UpdatedAt),
	}
}

func stateName(s *workflow.State) string {
	v, ok := s.Data["name"]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func isClosed(s metadata.DocumentStatus) bool {
	return s == metadata.StatusFinal || s == metadata.StatusSuperseded || s == metadata.StatusArchived
}

func dateAsRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
}

func timestampAsRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
