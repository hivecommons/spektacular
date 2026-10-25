package runstats

import (
	"time"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/hivecommons/spektacular/internal/tracelog"
)

// placement is where a run's tokens went: per step name, per task key, per
// helper span id, and outside any step. Every placed token is in exactly
// one of steps and outside, and also in all, so the steps plus outside
// equal all by construction. Tasks and helpers are views over the same
// tokens, not further buckets.
type placement struct {
	steps   map[string]usageByModel
	tasks   map[string]usageByModel
	helpers map[string]usageByModel
	outside usageByModel
	all     usageByModel
}

func newPlacement() *placement {
	return &placement{
		steps:   map[string]usageByModel{},
		tasks:   map[string]usageByModel{},
		helpers: map[string]usageByModel{},
		outside: usageByModel{},
		all:     usageByModel{},
	}
}

// put places u of model in step (outside any step when nil), in task when
// it has one, and on a helper span when it has one.
func (p *placement) put(step, task, helper *string, model string, u agentusage.Usage) {
	p.all.add(model, u)
	if step == nil {
		p.outside.add(model, u)
	} else {
		bucket(p.steps, *step).add(model, u)
	}
	if task != nil {
		bucket(p.tasks, *task).add(model, u)
	}
	if helper != nil {
		bucket(p.helpers, *helper).add(model, u)
	}
}

func bucket(m map[string]usageByModel, key string) usageByModel {
	b, ok := m[key]
	if !ok {
		b = usageByModel{}
		m[key] = b
	}
	return b
}

// locator finds the step entry and task span running at a moment of a run.
type locator struct {
	entries []StepEntry
	steps   map[string]bool // step names with a report row
	tasks   []Span
}

func newLocator(run *Run, rep *Report) locator {
	l := locator{entries: rep.StepEntries, steps: map[string]bool{}}
	for _, s := range rep.Steps {
		l.steps[s.Step] = true
	}
	for _, s := range run.Spans {
		if s.Type == tracelog.TypeTask {
			l.tasks = append(l.tasks, s)
		}
	}
	return l
}

// stepAt is the name of the step entry whose [start, end) holds at.
func (l locator) stepAt(at time.Time) *string {
	for _, e := range l.entries {
		if !at.Before(e.StartedAt) && at.Before(e.EndedAt) {
			return optional(e.Step)
		}
	}
	return nil
}

// taskAt is the key of the task span whose [start, end) holds at.
func (l locator) taskAt(at time.Time) *string {
	for _, s := range l.tasks {
		if !at.Before(s.Start) && at.Before(s.End) {
			k := taskKey(s.TaskID, s.ID)
			return &k
		}
	}
	return nil
}

// step is name when the report has a row for it, otherwise nil (outside
// any step), so every placed token lands in a reported step or outside.
func (l locator) step(name *string) *string {
	if name == nil || !l.steps[*name] {
		return nil
	}
	return name
}

// taskKey is how a task row is keyed: its task id, or its span when it has
// none.
func taskKey(taskID, spanID string) string {
	if taskID != "" {
		return taskID
	}
	return "span:" + spanID
}

// owns reports whether run owns a moment of a driver transcript, among the
// runs that transcript drove.
func (in *TokenInput) owns(run *Run, d driverRef, at time.Time) bool {
	o := ownerAt(in.runsDrivenBy(d.transcript(), run), at)
	return o != nil && o.ID == run.ID
}

// placeRequests puts each request of the driver transcripts that this run
// owns in the step entry and task running when it was made, or outside any
// step.
func placeRequests(run *Run, loc locator, ds []driverRef, in *TokenInput, p *placement) {
	for _, d := range ds {
		for _, r := range d.transcript().Requests {
			if !in.owns(run, d, r.At) {
				continue
			}
			p.put(loc.step(loc.stepAt(r.At)), loc.taskAt(r.At), nil, r.Model, r.Usage)
		}
	}
}
