package runstats

import (
	"sort"
	"strconv"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
)

// Run statuses, derived from the log alone.
const (
	// StatusFinished: the run reached its terminal step.
	StatusFinished = "finished"
	// StatusInterrupted: the run never finished, and was abandoned by a
	// restart or overtaken by a later run of the same kind and name.
	StatusInterrupted = "interrupted"
	// StatusInProgress: any other run.
	StatusInProgress = "in_progress"
)

// EndOpen is the end reason of a span that was never closed.
const EndOpen = "open"

// Event edges on a run's timeline.
const (
	EdgeStart = "start"
	EdgeEnd   = "end"
)

// Run is one workflow run: every record sharing a trace id that carries
// workflow attributes.
type Run struct {
	ID, Kind, Name, Agent, LogFile string
	// Start and End are the run's first and last recorded events; End
	// leaves out the ends of spans closed as abandoned.
	Start, End time.Time
	Status     string // finished | interrupted | in_progress
	// Spans are every non-command span, paired, in start order.
	Spans []Span
	// Commands are the command records, in start order.
	Commands []tracelog.Record
	// Events is the timeline the wall time and gaps are measured on.
	Events []Event
}

// Wall is the run's wall time, from its first recorded event to its last.
func (r *Run) Wall() time.Duration { return r.End.Sub(r.Start) }

// Span is a span_start paired with its span_end. End is the effective end:
// the recorded end, or for a span that is open or abandoned, its end capped
// at the run's last other event.
type Span struct {
	ID, Parent, Name, Type, Step, TaskID string
	Start, End                           time.Time
	EndReason, Status, Message           string
	Complete                             bool
}

// Duration is the span's effective duration.
func (s Span) Duration() time.Duration { return s.End.Sub(s.Start) }

// Event is one point on a run's timeline. Until is after At only for a
// command, which covers its own start-to-end interval; Edge is the edge used
// to label a gap after the event (a command's is its end).
type Event struct {
	At, Until        time.Time
	Edge, Type, Name string
}

// Assemble groups records into runs by trace id, pairs each span_start with
// its span_end, and derives each run's timeline, extent and status. Traces
// that carry no workflow attributes (commands and spans outside any
// workflow) are not runs. The runs are newest first. It is pure.
func Assemble(recs []tracelog.Record) []*Run {
	l := make([]loaded, len(recs))
	for i, r := range recs {
		l[i] = loaded{rec: r}
	}
	return assemble(l)
}

func assemble(recs []loaded) []*Run {
	var order []string
	groups := map[string][]loaded{}
	for _, r := range recs {
		id := r.rec.TraceID
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], r)
	}

	var runs []*Run
	for _, id := range order {
		if r := buildRun(id, groups[id]); r != nil {
			runs = append(runs, r)
		}
	}
	markOvertaken(runs)
	sort.SliceStable(runs, func(i, j int) bool {
		if !runs[i].Start.Equal(runs[j].Start) {
			return runs[i].Start.After(runs[j].Start)
		}
		return runs[i].ID < runs[j].ID
	})
	return runs
}

// buildRun assembles one trace's records, or returns nil when the trace is
// not a workflow run.
func buildRun(id string, recs []loaded) *Run {
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].rec.StartTime.Before(recs[j].rec.StartTime) })

	run := &Run{ID: id}
	for _, r := range recs {
		if run.Kind == "" {
			run.Kind = attr(r.rec, tracelog.AttrWorkflowKind)
		}
		if run.Name == "" {
			run.Name = attr(r.rec, tracelog.AttrWorkflowName)
		}
		if run.Agent == "" {
			run.Agent = attr(r.rec, tracelog.AttrAgent)
		}
	}
	if run.Kind == "" {
		return nil
	}
	if run.Agent == "" {
		run.Agent = "unknown"
	}
	run.LogFile = recs[0].file

	type pair struct{ start, end *tracelog.Record }
	var spanOrder []string
	pairs := map[string]*pair{}
	for i := range recs {
		rec := &recs[i].rec
		if rec.Record == tracelog.RecordSpan && isCommand(*rec) {
			run.Commands = append(run.Commands, *rec)
			continue
		}
		key := rec.SpanID
		if key == "" {
			// Load skips a span with no id; a caller of Assemble may not
			// have. It cannot be paired, so it stands alone.
			key = "\x00" + strconv.Itoa(i)
		}
		p, ok := pairs[key]
		if !ok {
			p = &pair{}
			pairs[key] = p
			spanOrder = append(spanOrder, key)
		}
		switch rec.Record {
		case tracelog.RecordSpanStart:
			if p.start == nil {
				p.start = rec
			}
		case tracelog.RecordSpanEnd, tracelog.RecordSpan:
			// A span closed twice keeps its earliest end.
			if p.end == nil || rec.EndTime.Before(p.end.EndTime) {
				p.end = rec
			}
		}
	}

	for _, key := range spanOrder {
		p := pairs[key]
		run.Spans = append(run.Spans, newSpan(p.start, p.end))
	}
	sort.SliceStable(run.Spans, func(i, j int) bool {
		if !run.Spans[i].Start.Equal(run.Spans[j].Start) {
			return run.Spans[i].Start.Before(run.Spans[j].Start)
		}
		return run.Spans[i].ID < run.Spans[j].ID
	})

	run.Events = timeline(run)
	run.Start, run.End = extent(run.Events)
	capUnfinished(run)
	run.Status = ownStatus(run)
	return run
}

// newSpan builds a span from its start and end records, either of which may
// be missing (not both). With no start, the end's own start time is used;
// with no end, the span is open and ends, for now, at its start.
func newSpan(start, end *tracelog.Record) Span {
	src := start
	if src == nil {
		src = end
	}
	s := Span{
		ID:     src.SpanID,
		Parent: src.ParentSpanID,
		Name:   src.Name,
		Type:   attr(*src, tracelog.AttrSpanType),
		Step:   attr(*src, tracelog.AttrStep),
		TaskID: attr(*src, tracelog.AttrTaskID),
		Start:  src.StartTime,
	}
	if end == nil {
		s.EndReason = EndOpen
		s.End = s.Start
		return s
	}
	s.End = end.EndTime
	if s.End.Before(s.Start) {
		s.End = s.Start
	}
	s.EndReason = attr(*end, tracelog.AttrEndReason)
	s.Status = end.Status.Code
	s.Message = end.Status.Message
	switch s.EndReason {
	case tracelog.EndMoved, tracelog.EndFinished, tracelog.EndStopped:
		s.Complete = true
	case "":
		// A complete span record with no end reason ended normally.
		s.Complete = true
	}
	return s
}

// unfinished reports whether s's recorded end does not mark a real end: it
// was never closed, or was closed by a sweep at a later moment.
func (s Span) unfinished() bool {
	return s.EndReason == EndOpen || s.EndReason == tracelog.EndAbandoned
}

// timeline is the run's sorted event list: every command, every span start,
// and every span end except an abandoned one, whose time is the moment a
// later sweep closed it.
func timeline(run *Run) []Event {
	var ev []Event
	for _, c := range run.Commands {
		until := c.EndTime
		if until.Before(c.StartTime) {
			until = c.StartTime
		}
		ev = append(ev, Event{At: c.StartTime, Until: until, Edge: EdgeEnd, Type: tracelog.TypeCommand, Name: c.Name})
	}
	for _, s := range run.Spans {
		ev = append(ev, Event{At: s.Start, Until: s.Start, Edge: EdgeStart, Type: s.Type, Name: s.Name})
		if !s.unfinished() {
			ev = append(ev, Event{At: s.End, Until: s.End, Edge: EdgeEnd, Type: s.Type, Name: s.Name})
		}
	}
	sort.SliceStable(ev, func(i, j int) bool {
		a, b := ev[i], ev[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		ac, bc := a.Type == tracelog.TypeCommand, b.Type == tracelog.TypeCommand
		if ac != bc {
			return ac
		}
		if a.Edge != b.Edge {
			return a.Edge == EdgeStart
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Name < b.Name
	})
	return ev
}

// extent is the first and last moment on a timeline.
func extent(ev []Event) (start, end time.Time) {
	for i, e := range ev {
		if i == 0 || e.At.Before(start) {
			start = e.At
		}
		last := e.Until
		if e.At.After(last) {
			last = e.At
		}
		if i == 0 || last.After(end) {
			end = last
		}
	}
	return start, end
}

// capUnfinished gives every open or abandoned span its effective end: its
// recorded end (or, open, the run's end) capped at the run's last other
// event, and never before its own start. An interrupted span keeps its
// recorded end, which is already its last command.
func capUnfinished(run *Run) {
	for i := range run.Spans {
		s := &run.Spans[i]
		if !s.unfinished() {
			continue
		}
		end := run.End
		if s.EndReason == tracelog.EndAbandoned && s.End.Before(end) {
			end = s.End
		}
		if end.Before(s.Start) {
			end = s.Start
		}
		s.End = end
	}
}

// ownStatus is the status the run's own records give: finished when it
// reached its terminal step, interrupted when a restart abandoned a step,
// and otherwise in progress. A later run of the same workflow can still
// turn an in-progress run into an interrupted one (markOvertaken).
func ownStatus(run *Run) string {
	for _, s := range run.Spans {
		if s.Type == tracelog.TypeStep && (s.EndReason == tracelog.EndFinished || s.Name == StatusFinished) {
			return StatusFinished
		}
	}
	for _, c := range run.Commands {
		if attr(c, tracelog.AttrStepTo) == StatusFinished {
			return StatusFinished
		}
	}
	for _, s := range run.Spans {
		if s.Type == tracelog.TypeStep && s.EndReason == tracelog.EndAbandoned {
			return StatusInterrupted
		}
	}
	return StatusInProgress
}

// markOvertaken marks interrupted every in-progress run that a later run of
// the same kind and name started after.
func markOvertaken(runs []*Run) {
	for _, r := range runs {
		if r.Status != StatusInProgress {
			continue
		}
		for _, o := range runs {
			if o != r && o.Kind == r.Kind && o.Name == r.Name && o.Start.After(r.End) {
				r.Status = StatusInterrupted
				break
			}
		}
	}
}

// isCommand reports whether a complete span record is a command: its span
// type is command, or it has none.
func isCommand(r tracelog.Record) bool {
	t := attr(r, tracelog.AttrSpanType)
	return t == "" || t == tracelog.TypeCommand
}

// attr is a record's string attribute, or "" when absent or not a string.
func attr(r tracelog.Record, key string) string {
	s, _ := r.Attributes[key].(string)
	return s
}
