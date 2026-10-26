package tracelog

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sampleRecord() Record {
	start := time.Date(2026, 10, 24, 9, 3, 34, 911373161, time.UTC)
	return Record{
		Record:    RecordSpan,
		TraceID:   "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:    "00f067aa0ba902b7",
		Name:      "spec goto",
		Kind:      KindInternal,
		StartTime: start,
		EndTime:   start.Add(22628952 * time.Nanosecond),
		Status:    Status{Code: StatusOK},
		Attributes: map[string]any{
			AttrSpanType:     TypeCommand,
			AttrAgent:        "claude",
			AttrWorkflowKind: "spec",
			AttrWorkflowName: "000073_log-mode",
			AttrStep:         "interview",
			AttrStepTo:       "overview",
			AttrCommandArgs:  []string{"spec", "goto"},
			AttrExitCode:     0,
		},
		Resource: Resource("0.21.0"),
	}
}

// TestRecordJSONShape pins the record's field names (the contract later
// specs of the epic read) and the omission of an empty parent span id and
// an empty status message.
func TestRecordJSONShape(t *testing.T) {
	data, err := json.Marshal(sampleRecord())
	require.NoError(t, err)

	want := `{"record":"span","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7",` +
		`"name":"spec goto","kind":"internal","start_time":"2026-10-24T09:03:34.911373161Z",` +
		`"end_time":"2026-10-24T09:03:34.934002113Z","status":{"code":"ok"},` +
		`"attributes":{"process.command_args":["spec","goto"],"process.exit_code":0,"spektacular.agent":"claude",` +
		`"spektacular.span.type":"command","spektacular.step":"interview","spektacular.step.to":"overview",` +
		`"spektacular.workflow.kind":"spec","spektacular.workflow.name":"000073_log-mode"},` +
		`"resource":{"service.name":"spektacular","service.version":"0.21.0"}}`
	require.JSONEq(t, want, string(data))

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	require.NotContains(t, raw, "parent_span_id")
	require.NotContains(t, raw["status"], "message")
}

// TestRecordJSONIncludesParentAndMessageWhenSet checks the optional fields
// appear once they carry a value.
func TestRecordJSONIncludesParentAndMessageWhenSet(t *testing.T) {
	rec := sampleRecord()
	rec.ParentSpanID = "a1b2c3d4e5f60718"
	rec.Status = Status{Code: StatusError, Message: "unknown step"}

	data, err := json.Marshal(rec)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Equal(t, "a1b2c3d4e5f60718", raw["parent_span_id"])
	require.Equal(t, map[string]any{"code": "error", "message": "unknown step"}, raw["status"])
}

// TestSpanStartJSONShape pins a span_start record: no end time and no
// status.
func TestSpanStartJSONShape(t *testing.T) {
	start := time.Date(2026, 10, 24, 9, 3, 34, 930000000, time.UTC)
	rec := Record{
		Record:    RecordSpanStart,
		TraceID:   "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:    "a3ce929d0e0e4736",
		Name:      "interview",
		Kind:      KindInternal,
		StartTime: start,
		Attributes: map[string]any{
			AttrSpanType: TypeStep,
			AttrStep:     "interview",
		},
		Resource: Resource("0.22.0"),
	}
	data, err := json.Marshal(rec)
	require.NoError(t, err)

	want := `{"record":"span_start","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"a3ce929d0e0e4736",` +
		`"name":"interview","kind":"internal","start_time":"2026-10-24T09:03:34.93Z",` +
		`"attributes":{"spektacular.span.type":"step","spektacular.step":"interview"},` +
		`"resource":{"service.name":"spektacular","service.version":"0.22.0"}}`
	require.JSONEq(t, want, string(data))
}

// TestSpanEndRepeatsStart checks a span_end is its start plus the end
// time, status and end reason, and leaves the start's attributes alone.
func TestSpanEndRepeatsStart(t *testing.T) {
	start := time.Date(2026, 10, 24, 9, 3, 34, 930000000, time.UTC)
	sp := Spans{Run: workflowRun(start), Base: map[string]any{AttrAgent: "claude"}, Resource: Resource("0.22.0")}
	st := sp.StartRecord("a3ce929d0e0e4736", "interview", TypeStep, "9d0e0e4736a3ce92", start, map[string]any{AttrStep: "interview"})

	end := endRecord(st, start.Add(time.Minute), Status{Code: StatusOK}, EndMoved)
	data, err := json.Marshal(end)
	require.NoError(t, err)

	want := `{"record":"span_end","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"a3ce929d0e0e4736",` +
		`"parent_span_id":"9d0e0e4736a3ce92","name":"interview","kind":"internal",` +
		`"start_time":"2026-10-24T09:03:34.93Z","end_time":"2026-10-24T09:04:34.93Z","status":{"code":"ok"},` +
		`"attributes":{"spektacular.agent":"claude","spektacular.span.type":"step","spektacular.step":"interview",` +
		`"spektacular.span.end_reason":"moved"},` +
		`"resource":{"service.name":"spektacular","service.version":"0.22.0"}}`
	require.JSONEq(t, want, string(data))
	require.NotContains(t, st.Attributes, AttrEndReason, "ending a span must not change its start")
}

// TestReasonStatus pins the status each end reason gets.
func TestReasonStatus(t *testing.T) {
	require.Equal(t, Status{Code: StatusOK}, ReasonStatus(EndMoved))
	require.Equal(t, Status{Code: StatusOK}, ReasonStatus(EndFinished))
	require.Equal(t, Status{Code: StatusOK}, ReasonStatus(EndStopped))
	require.Equal(t, Status{Code: StatusError, Message: "abandoned"}, ReasonStatus(EndAbandoned))
	require.Equal(t, Status{Code: StatusError, Message: "interrupted"}, ReasonStatus(EndInterrupted))
}
