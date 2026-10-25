package tracelog

import (
	"strings"
	"time"
)

// Index answers the questions the span logic asks of one run's records:
// which spans are open, which are closed, what is open beneath a span, and
// when a step's last command ended. It is built from records alone and does
// no I/O.
type Index struct {
	order    []string          // span ids in the order their first record appears
	starts   map[string]Record // span_start records by span id
	ends     map[string]Record // span_end records by span id
	children map[string][]string
	commands []Record // complete span records (commands)
}

// NewIndex indexes recs, which are in file order.
func NewIndex(recs []Record) Index {
	ix := Index{
		starts:   map[string]Record{},
		ends:     map[string]Record{},
		children: map[string][]string{},
	}
	seen := map[string]bool{}
	note := func(r Record) {
		if seen[r.SpanID] {
			return
		}
		seen[r.SpanID] = true
		ix.order = append(ix.order, r.SpanID)
		if r.ParentSpanID != "" {
			ix.children[r.ParentSpanID] = append(ix.children[r.ParentSpanID], r.SpanID)
		}
	}
	for _, r := range recs {
		switch r.Record {
		case RecordSpanStart:
			if _, dup := ix.starts[r.SpanID]; !dup {
				ix.starts[r.SpanID] = r
			}
			note(r)
		case RecordSpanEnd:
			if _, dup := ix.ends[r.SpanID]; !dup {
				ix.ends[r.SpanID] = r
			}
			note(r)
		case RecordSpan:
			ix.commands = append(ix.commands, r)
		}
	}
	return ix
}

// Open reports whether span id was started and has not ended.
func (ix Index) Open(id string) bool {
	_, started := ix.starts[id]
	_, ended := ix.ends[id]
	return started && !ended
}

// Find returns span id's start, whether it is still open, and whether it is
// known at all. A span known only by its end (its start was never logged)
// is found and closed, and its end record stands in for the start.
func (ix Index) Find(id string) (start Record, open, found bool) {
	if s, ok := ix.starts[id]; ok {
		_, ended := ix.ends[id]
		return s, !ended, true
	}
	if e, ok := ix.ends[id]; ok {
		return e, false, true
	}
	return Record{}, false, false
}

// OpenUnder returns the spans still open beneath parentID, at any depth, in
// the order they started. A closed span's open descendants are included.
func (ix Index) OpenUnder(parentID string) []Record {
	under := map[string]bool{}
	queue := []string{parentID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, c := range ix.children[id] {
			if !under[c] && c != parentID {
				under[c] = true
				queue = append(queue, c)
			}
		}
	}
	var out []Record
	for _, id := range ix.order {
		if under[id] && ix.Open(id) {
			out = append(out, ix.starts[id])
		}
	}
	return out
}

// OpenAll returns every open span of the run, in the order they started.
func (ix Index) OpenAll() []Record {
	var out []Record
	for _, id := range ix.order {
		if ix.Open(id) {
			out = append(out, ix.starts[id])
		}
	}
	return out
}

// LastCommandUnder returns the latest end time of a command recorded
// directly under parentID. Workflow-start commands ("spec new", "implement
// new" and the like), which in the middle of a step are only ever resume
// probes, and trace commands are left out, so a resumed step's end is the
// last work done in it.
func (ix Index) LastCommandUnder(parentID string) (time.Time, bool) {
	var last time.Time
	found := false
	for _, c := range ix.commands {
		if c.ParentSpanID != parentID || !countsAsActivity(c.Name) {
			continue
		}
		if !found || c.EndTime.After(last) {
			last = c.EndTime
			found = true
		}
	}
	return last, found
}

// countsAsActivity reports whether a command named name is a step's own
// work, not a resume probe or a trace command.
func countsAsActivity(name string) bool {
	if strings.HasPrefix(name, "trace ") || name == "trace" {
		return false
	}
	words := strings.Fields(name)
	return !(len(words) == 2 && words[1] == "new")
}
