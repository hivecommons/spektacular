package runstats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

// t0 is the fixed moment every fixture is built around. No test reads the
// wall clock.
var t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

// at is t0 plus m minutes.
func at(m float64) time.Time { return t0.Add(time.Duration(m * float64(time.Minute))) }

// fx builds the records of one trace. A fixture with an empty kind builds a
// standalone trace.
type fx struct{ trace, kind, name, agent string }

func (f fx) base(typ string, extra map[string]any) map[string]any {
	a := map[string]any{tracelog.AttrSpanType: typ}
	if f.agent != "" {
		a[tracelog.AttrAgent] = f.agent
	}
	if f.kind != "" {
		a[tracelog.AttrWorkflowKind] = f.kind
		a[tracelog.AttrWorkflowName] = f.name
	}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

// cmd is a command record under parent (a step span id, or "").
func (f fx) cmd(name, parent string, start, end time.Time, extra ...map[string]any) tracelog.Record {
	var x map[string]any
	if len(extra) > 0 {
		x = extra[0]
	}
	return tracelog.Record{
		Record: tracelog.RecordSpan, TraceID: f.trace, SpanID: "c" + name + start.Format("150405.000"),
		ParentSpanID: parent, Name: name, Kind: tracelog.KindInternal,
		StartTime: start, EndTime: end, Status: tracelog.Status{Code: tracelog.StatusOK},
		Attributes: f.base(tracelog.TypeCommand, x),
	}
}

// open is a span_start record.
func (f fx) open(id, name, typ, parent string, start time.Time, extra ...map[string]any) tracelog.Record {
	var x map[string]any
	if len(extra) > 0 {
		x = extra[0]
	}
	return tracelog.Record{
		Record: tracelog.RecordSpanStart, TraceID: f.trace, SpanID: id, ParentSpanID: parent,
		Name: name, Kind: tracelog.KindInternal, StartTime: start, Attributes: f.base(typ, x),
	}
}

// close is the span_end record closing start at end for reason.
func closeSpan(start tracelog.Record, end time.Time, reason string) tracelog.Record {
	r := start
	r.Record = tracelog.RecordSpanEnd
	r.EndTime = end
	r.Status = tracelog.ReasonStatus(reason)
	r.Attributes = map[string]any{}
	for k, v := range start.Attributes {
		r.Attributes[k] = v
	}
	r.Attributes[tracelog.AttrEndReason] = reason
	return r
}

// span is a span's start and end records.
func (f fx) span(id, name, typ, parent string, start, end time.Time, reason string, extra ...map[string]any) []tracelog.Record {
	s := f.open(id, name, typ, parent, start, extra...)
	return []tracelog.Record{s, closeSpan(s, end, reason)}
}

// recs flattens records and record lists.
func recs(parts ...any) []tracelog.Record {
	var out []tracelog.Record
	for _, p := range parts {
		switch v := p.(type) {
		case tracelog.Record:
			out = append(out, v)
		case []tracelog.Record:
			out = append(out, v...)
		default:
			panic("recs: unexpected part")
		}
	}
	return out
}

// writeLog writes records as JSON Lines to dir/name, followed by any raw
// lines given.
func writeLog(t *testing.T, dir, name string, rs []tracelog.Record, raw ...string) {
	t.Helper()
	var data []byte
	for _, r := range rs {
		line, err := json.Marshal(r)
		require.NoError(t, err)
		data = append(data, line...)
		data = append(data, '\n')
	}
	for _, l := range raw {
		data = append(data, l...)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))
}

// snapshot maps every entry in dir to its bytes.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		out[e.Name()] = string(b)
	}
	return out
}

// runByID finds a run.
func runByID(t *testing.T, runs []*Run, id string) *Run {
	t.Helper()
	for _, r := range runs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no run %s", id)
	return nil
}
