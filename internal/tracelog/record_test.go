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
