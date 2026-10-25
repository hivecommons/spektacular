// Package tracelog writes one OpenTelemetry-model span record per completed
// CLI command to a log, through a logger chosen by provider name. It knows
// nothing about configuration, workflows or commands: the caller assembles
// a Run and a Record and hands them to a Logger, which turns them into one
// line on disk.
//
// The record format defined here is shared by the whole of epic 000002:
// later specs add records of other kinds (span_start, span_end) and other
// span types without changing a field defined in this file.
package tracelog

import "time"

// Record kinds, span kinds, span types and status codes.
const (
	// RecordSpan is a complete span: it started and ended in one process.
	RecordSpan = "span"
	// KindInternal is the OpenTelemetry span kind every command span has.
	KindInternal = "internal"
	// TypeCommand is the spektacular.span.type of one CLI command.
	TypeCommand = "command"
	// StatusOK and StatusError are the span status codes.
	StatusOK    = "ok"
	StatusError = "error"
)

// Attribute keys a command record carries.
const (
	AttrSpanType     = "spektacular.span.type"
	AttrAgent        = "spektacular.agent"
	AttrWorkflowKind = "spektacular.workflow.kind"
	AttrWorkflowName = "spektacular.workflow.name"
	AttrStep         = "spektacular.step"
	AttrStepTo       = "spektacular.step.to"
	AttrCommandArgs  = "process.command_args"
	AttrExitCode     = "process.exit_code"
	AttrResponse     = "spektacular.response"
	AttrStateBefore  = "spektacular.state.before"
	AttrStateAfter   = "spektacular.state.after"
)

// Resource keys and the service name every record carries.
const (
	ResourceServiceName    = "service.name"
	ResourceServiceVersion = "service.version"
	ServiceName            = "spektacular"
)

// Record is one JSON Lines log record, following the OpenTelemetry trace
// data model. Times are written in UTC.
type Record struct {
	Record       string         `json:"record"`
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      time.Time      `json:"end_time"`
	Status       Status         `json:"status"`
	Attributes   map[string]any `json:"attributes"`
	Resource     map[string]any `json:"resource"`
}

// Status is a span's outcome: code "ok" or "error", with the error's message.
type Status struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// Run says which log a record belongs to. A workflow run has a Kind and
// Name and starts when its state was created; a standalone command has
// neither and starts when the command did. The file logger derives the
// record's file from the Run alone.
type Run struct {
	TraceID   string
	Kind      string
	Name      string
	StartedAt time.Time
	Agent     string
}

// Standalone reports whether the run is a command outside any workflow.
func (r Run) Standalone() bool { return r.Kind == "" }

// Resource returns the resource map every record carries, naming the
// service and the build version.
func Resource(version string) map[string]any {
	return map[string]any{
		ResourceServiceName:    ServiceName,
		ResourceServiceVersion: version,
	}
}
