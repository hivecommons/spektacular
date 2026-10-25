package tracelog

import (
	"maps"
	"slices"
	"time"
)

// Spans writes and queries the spans of one run through a logger. Base holds
// the attributes added to every record it writes (the agent and the
// workflow), and Resource the resource map. The queries need a logger with
// a read side (Reader); without one they find nothing and the sweeps that
// close open spans do nothing.
//
// Like Logger.Write, no method returns an error or panics outward: span
// logging can never change a command.
type Spans struct {
	Log      Logger
	Run      Run
	Base     map[string]any
	Resource map[string]any
}

// Start mints a span id and writes the span_start record of a span named
// name, of type typ, under parent ("" for none), starting at at. attrs are
// added to the record's attributes. It returns the record written.
func (s Spans) Start(name, typ, parent string, at time.Time, attrs map[string]any) Record {
	return s.StartWithID(NewSpanID(), name, typ, parent, at, attrs)
}

// StartWithID is Start with a span id chosen by the caller, as the workflow
// chooses the ids of its step and task spans.
func (s Spans) StartWithID(id, name, typ, parent string, at time.Time, attrs map[string]any) (rec Record) {
	defer func() { _ = recover() }()
	rec = s.StartRecord(id, name, typ, parent, at, attrs)
	s.write(rec)
	return rec
}

// StartRecord builds, without writing, the span_start record StartWithID
// would write. A workflow span whose start was never logged is ended from
// this record.
func (s Spans) StartRecord(id, name, typ, parent string, at time.Time, attrs map[string]any) Record {
	a := make(map[string]any, len(s.Base)+len(attrs)+1)
	maps.Copy(a, s.Base)
	maps.Copy(a, attrs)
	a[AttrSpanType] = typ
	return Record{
		Record:       RecordSpanStart,
		TraceID:      s.Run.TraceID,
		SpanID:       id,
		ParentSpanID: parent,
		Name:         name,
		Kind:         KindInternal,
		StartTime:    at.UTC(),
		Attributes:   a,
		Resource:     s.Resource,
	}
}

// End writes the span_end record of the span start opened: a copy of the
// start with the end time, the status and the end reason added.
func (s Spans) End(start Record, at time.Time, st Status, reason string) {
	defer func() { _ = recover() }()
	s.write(endRecord(start, at, st, reason))
}

func endRecord(start Record, at time.Time, st Status, reason string) Record {
	end := start
	end.Record = RecordSpanEnd
	end.EndTime = at.UTC()
	if end.EndTime.Before(end.StartTime) {
		end.EndTime = end.StartTime
	}
	end.Status = st
	end.Attributes = maps.Clone(start.Attributes)
	if end.Attributes == nil {
		end.Attributes = map[string]any{}
	}
	end.Attributes[AttrEndReason] = reason
	return end
}

// EndUnder ends every span still open beneath parentID, at any depth, at at
// and for reason, deepest first. Spans elsewhere in the run are left alone.
func (s Spans) EndUnder(parentID string, at time.Time, reason string) {
	defer func() { _ = recover() }()
	ix, ok := s.Index()
	if !ok {
		return
	}
	s.endEach(ix.OpenUnder(parentID), at, reason)
}

// EndAll ends every span still open in the run, at at and for reason,
// deepest first.
func (s Spans) EndAll(at time.Time, reason string) {
	defer func() { _ = recover() }()
	ix, ok := s.Index()
	if !ok {
		return
	}
	s.endEach(ix.OpenAll(), at, reason)
}

// endEach ends open, which is in start order, latest started first, so a
// span's children end before it does.
func (s Spans) endEach(open []Record, at time.Time, reason string) {
	st := ReasonStatus(reason)
	for _, rec := range slices.Backward(open) {
		s.write(endRecord(rec, at, st, reason))
	}
}

// Find returns span id's start record, whether it is still open, and
// whether the run's log knows it at all.
func (s Spans) Find(id string) (start Record, open, found bool) {
	defer func() { _ = recover() }()
	ix, ok := s.Index()
	if !ok {
		return Record{}, false, false
	}
	return ix.Find(id)
}

// LastCommandUnder returns the end time of the last command recorded under
// the span parentID, leaving out workflow-start and trace commands.
func (s Spans) LastCommandUnder(parentID string) (time.Time, bool) {
	defer func() { _ = recover() }()
	ix, ok := s.Index()
	if !ok {
		return time.Time{}, false
	}
	return ix.LastCommandUnder(parentID)
}

// Index reads the run's log and indexes it. It reports false when the
// logger has no read side or the log cannot be read.
func (s Spans) Index() (Index, bool) {
	r, ok := s.Log.(Reader)
	if !ok {
		return Index{}, false
	}
	recs, err := r.Records(s.Run)
	if err != nil {
		return Index{}, false
	}
	return NewIndex(recs), true
}

func (s Spans) write(rec Record) {
	if s.Log != nil {
		s.Log.Write(s.Run, rec)
	}
}
