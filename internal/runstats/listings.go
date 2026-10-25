package runstats

import (
	"encoding/json"
	"slices"
	"sort"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
)

// CodeBadSelection is the selection error of a run selection combined with
// a request for only the records outside any run.
const CodeBadSelection = "bad_selection"

// The listings give the raw facts behind the run report: every command
// record and every agent-marked span, in runs and outside them. Their JSON
// is a contract read by the Harbor harness (spec 000076), fixed by the Data
// Structures section of plan 000076_harbor-on-built-in-logs. Times are
// RFC 3339 UTC, durations integer milliseconds, and an absent value is null:
//
//	commands: [{"run_id","kind","name","agent","span_id","parent_span_id",
//	            "command","args","step","step_to","started_at","ended_at",
//	            "duration_ms","exit_code","status","message"}]
//	spans:    [{<a run report's helpers entry>,"type","trace_id","run_id","kind","workflow"}]
//
// Outside any run, run_id, kind and name (workflow for a span) are null.
// Renaming a field breaks the golden files testdata/commands.golden.json and
// testdata/spans.golden.json before it breaks a consumer.

// CommandRow is one command record.
type CommandRow struct {
	RunID        *string   `json:"run_id"`
	Kind         *string   `json:"kind"`
	Name         *string   `json:"name"`
	Agent        *string   `json:"agent"`
	SpanID       string    `json:"span_id"`
	ParentSpanID *string   `json:"parent_span_id"`
	Command      string    `json:"command"`
	Args         []string  `json:"args"`
	Step         *string   `json:"step"`
	StepTo       *string   `json:"step_to"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	DurationMS   int64     `json:"duration_ms"`
	ExitCode     int       `json:"exit_code"`
	Status       string    `json:"status"`
	Message      *string   `json:"message"`
}

// SpanRow is one agent-marked span: a helper agent, a wait on the user or
// other work. It carries a run report's helper fields, its span type, and
// the run it belongs to; Workflow is the run's workflow name, since Name is
// the span's.
type SpanRow struct {
	Wait
	Type     string  `json:"type"`
	TraceID  string  `json:"trace_id"`
	RunID    *string `json:"run_id"`
	Kind     *string `json:"kind"`
	Workflow *string `json:"workflow"`
}

// selected reports whether f narrows the runs at all.
func (f Filter) selected() bool {
	return f.Name != "" || f.Kind != "" || f.Agent != "" || len(f.Runs) > 0
}

// listed is the runs a listing covers, and whether it covers the records
// outside any run too: with outsideRuns only those (refused beside a
// selection); with a selection only the selected runs; otherwise every run
// and every record outside them.
func (l *Logs) listed(f Filter, outsideRuns bool) (runs []*Run, outside bool, err error) {
	switch {
	case outsideRuns && f.selected():
		return nil, false, &SelectionError{Code: CodeBadSelection, Value: "--outside-runs"}
	case outsideRuns:
		return nil, true, nil
	case f.selected():
		runs, err = l.Candidates(f)
		return runs, false, err
	}
	return l.Runs, true, nil
}

// Commands lists the command records the selection covers, oldest first.
func (l *Logs) Commands(f Filter, outsideRuns bool) ([]CommandRow, error) {
	runs, outside, err := l.listed(f, outsideRuns)
	if err != nil {
		return nil, err
	}
	rows := []CommandRow{}
	for _, r := range runs {
		for _, c := range r.Commands {
			rows = append(rows, commandRow(c, r))
		}
	}
	if outside {
		for _, c := range l.Outside {
			if c.Record == tracelog.RecordSpan && isCommand(c) {
				rows = append(rows, commandRow(c, nil))
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].StartedAt.Equal(rows[j].StartedAt) {
			return rows[i].StartedAt.Before(rows[j].StartedAt)
		}
		return rows[i].SpanID < rows[j].SpanID
	})
	return rows, nil
}

// commandRow is c's row; run is nil outside any run.
func commandRow(c tracelog.Record, run *Run) CommandRow {
	end := c.EndTime
	if end.Before(c.StartTime) {
		end = c.StartTime
	}
	row := CommandRow{
		Agent: optional(attr(c, tracelog.AttrAgent)), SpanID: c.SpanID, ParentSpanID: optional(c.ParentSpanID),
		Command: c.Name, Args: stringsAttr(c, tracelog.AttrCommandArgs),
		Step: optional(attr(c, tracelog.AttrStep)), StepTo: optional(attr(c, tracelog.AttrStepTo)),
		StartedAt: c.StartTime.UTC(), EndedAt: end.UTC(), DurationMS: ms(end.Sub(c.StartTime)),
		Status: c.Status.Code, Message: optional(c.Status.Message),
	}
	code, ok := intAttr(c, tracelog.AttrExitCode)
	if !ok && c.Status.Code != tracelog.StatusOK {
		code = 1
	}
	row.ExitCode = code
	if c.Status.Code != tracelog.StatusError {
		row.Message = nil
	}
	if run != nil {
		row.RunID, row.Kind, row.Name = optional(run.ID), optional(run.Kind), optional(run.Name)
		if row.Agent == nil {
			row.Agent = optional(run.Agent)
		}
	}
	return row
}

// AgentSpans lists the agent-marked spans (sub-agent, user-wait and work)
// the selection covers, oldest first, paired and capped as the run report
// caps them.
func (l *Logs) AgentSpans(f Filter, outsideRuns bool) ([]SpanRow, error) {
	runs, outside, err := l.listed(f, outsideRuns)
	if err != nil {
		return nil, err
	}
	rows := []SpanRow{}
	for _, r := range runs {
		byID, steps, tasks := spanIndex(r.Spans)
		for _, s := range r.Spans {
			if !slices.Contains(tracelog.AgentTypes, s.Type) {
				continue
			}
			rows = append(rows, SpanRow{
				Wait: waitRow(s, byID, steps, tasks), Type: s.Type, TraceID: r.ID,
				RunID: optional(r.ID), Kind: optional(r.Kind), Workflow: optional(r.Name),
			})
		}
	}
	if outside {
		rows = append(rows, outsideSpans(l.Outside)...)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].StartedAt.Equal(rows[j].StartedAt) {
			return rows[i].StartedAt.Before(rows[j].StartedAt)
		}
		return rows[i].SpanID < rows[j].SpanID
	})
	return rows, nil
}

// outsideSpans pairs the agent-marked spans among records outside any run,
// trace by trace. A span left open or abandoned ends, at most, at the last
// event of its own trace, and never before its start.
func outsideSpans(recs []tracelog.Record) []SpanRow {
	var order []string
	byTrace := map[string][]tracelog.Record{}
	for _, r := range recs {
		if _, ok := byTrace[r.TraceID]; !ok {
			order = append(order, r.TraceID)
		}
		byTrace[r.TraceID] = append(byTrace[r.TraceID], r)
	}
	var rows []SpanRow
	for _, trace := range order {
		commands, spans := splitTrace(byTrace[trace])
		var last time.Time
		for _, c := range commands {
			last = later(last, later(c.StartTime, c.EndTime))
		}
		for _, s := range spans {
			last = later(last, s.Start)
			if !s.unfinished() {
				last = later(last, s.End)
			}
		}
		byID, steps, tasks := spanIndex(spans)
		for _, s := range spans {
			if !slices.Contains(tracelog.AgentTypes, s.Type) {
				continue
			}
			if s.unfinished() {
				end := last
				if s.EndReason == tracelog.EndAbandoned && s.End.Before(end) {
					end = s.End
				}
				if end.Before(s.Start) {
					end = s.Start
				}
				s.End = end
			}
			rows = append(rows, SpanRow{Wait: waitRow(s, byID, steps, tasks), Type: s.Type, TraceID: trace})
		}
	}
	return rows
}

// spanIndex is spans by id, with the step and task spans among them.
func spanIndex(spans []Span) (byID map[string]Span, steps, tasks []Span) {
	byID = map[string]Span{}
	for _, s := range spans {
		byID[s.ID] = s
		switch s.Type {
		case tracelog.TypeStep:
			steps = append(steps, s)
		case tracelog.TypeTask:
			tasks = append(tasks, s)
		}
	}
	return byID, steps, tasks
}

// later is the later of a and b.
func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// stringsAttr is a record's string-list attribute, read from a decoded log
// line ([]any) or built in memory ([]string); an empty list when absent.
func stringsAttr(r tracelog.Record, key string) []string {
	out := []string{}
	switch v := r.Attributes[key].(type) {
	case []string:
		out = append(out, v...)
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// intAttr is a record's integer attribute, read from a decoded log line
// (float64) or built in memory.
func intAttr(r tracelog.Record, key string) (int, bool) {
	switch v := r.Attributes[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	}
	return 0, false
}
