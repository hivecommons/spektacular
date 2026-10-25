package runstats

import (
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
)

// gap is an idle stretch on a run's timeline: no event, and no command
// running, from from to to.
type gap struct {
	from, to      time.Time
	after, before GapEdge
}

// gaps walks the run's timeline and returns every stretch between one event
// (or the end of a command) and the next event, in time order. A command's
// own running time is never a gap, and neither is a stretch under a
// millisecond. Each gap is labelled by the event that
// last extended the covered time before it, and by the event after it.
func gaps(r *Run) []gap {
	var out []gap
	var covered time.Time
	var last GapEdge
	for i, e := range r.Events {
		reach := e.Until
		if e.At.After(reach) {
			reach = e.At
		}
		// A stretch shorter than the millisecond the report counts in is
		// not a gap.
		if i > 0 && e.At.Sub(covered) >= time.Millisecond {
			out = append(out, gap{from: covered, to: e.At, after: last, before: beforeEdge(e)})
		}
		if i == 0 || reach.After(covered) {
			covered = reach
			last = GapEdge{Event: e.Edge, Type: e.Type, Name: e.Name}
		}
	}
	return out
}

// beforeEdge labels the event after a gap: a command by its start.
func beforeEdge(e Event) GapEdge {
	edge := e.Edge
	if e.Type == tracelog.TypeCommand {
		edge = EdgeStart
	}
	return GapEdge{Event: edge, Type: e.Type, Name: e.Name}
}
