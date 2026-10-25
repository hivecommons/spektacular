package cmd

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

// The trace commands let an agent mark the spans of its own work that the
// CLI cannot see: the helper agents it starts, the questions it puts to the
// user, and any other stretch it wants timed. Steps, plan tasks and
// commands are recorded by Spektacular itself.
//
// Neither command ever opens a workflow or writes workflow state. The run's
// own log is the registry of open spans: a span's id carries its run's trace
// id, so a stop in another process, or in an orchestrated lane, finds the
// span's run by it. With logging off both succeed and record nothing.
var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Mark spans of agent work (helper agents, waits on the user) in the run's log",
	RunE:  runUnknownSubcommand,
}

var traceStartCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a span and print its id",
	Long: `Start a span named <name> and print its id, "<trace_id>-<span_id>".

The span nests under the current step's span unless --parent names another
open span. Pass a helper agent the id as its parent, so the spans it starts
nest under the span that launched it. --type is sub-agent (a helper agent),
user-wait (waiting for the user to answer) or work (the default).`,
	RunE: runTraceStart,
}

var traceStopCmd = &cobra.Command{
	Use:   "stop <id>",
	Short: "Stop a span started with trace start",
	RunE:  runTraceStop,
}

// maxSpanName bounds a span's name, in characters.
const maxSpanName = 200

// TraceStartResult is the output of `trace start`.
type TraceStartResult struct {
	SpanID   string `json:"span_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Parent   string `json:"parent,omitempty"`
	Recorded bool   `json:"recorded"`
}

// TraceStopResult is the output of `trace stop`.
type TraceStopResult struct {
	SpanID   string `json:"span_id"`
	Status   string `json:"status"`
	Recorded bool   `json:"recorded"`
}

// traceRun is the run a trace command acts on: an in-progress workflow's
// state, or none for a standalone span.
type traceRun struct {
	state *workflow.State
}

// resolveTraceRun finds the in-progress workflow run a span reference
// belongs to, by its trace id: the shared state, any plan or implement lane,
// or the repo workflow's state. With no reference it is the run in the
// shared state, when one is in progress. statePath is "" and state nil when
// there is none: the span is standalone.
func resolveTraceRun(dir string, ref *tracelog.SpanRef) (statePath string, state *workflow.State) {
	shared := stateFilePath(dir)
	if ref == nil {
		if s, _ := readState(shared); s != nil && s.InProgress() && s.RunID != "" {
			return shared, s
		}
		return "", nil
	}
	for _, p := range traceStateCandidates(dir) {
		if s, _ := readState(p); s != nil && s.InProgress() && s.RunID == ref.TraceID {
			return p, s
		}
	}
	return "", nil
}

// traceStateCandidates are every state file a run can live in.
func traceStateCandidates(dir string) []string {
	paths := []string{stateFilePath(dir)}
	for _, kind := range []string{"plan", "implement"} {
		for _, name := range workflow.LaneNames(dir, kind) {
			paths = append(paths, workflow.LaneStatePath(dir, kind, name))
		}
	}
	return append(paths, repoStateFilePath(dir))
}

// traceSpans is the span service a trace command writes through, for the
// run in state, or for a standalone trace.
func traceSpans(r *spanRecorder, state *workflow.State, traceID string, now time.Time) tracelog.Spans {
	if state != nil {
		return r.spans(state.RunRef())
	}
	return tracelog.Spans{
		Log:      r.logger,
		Run:      tracelog.Run{TraceID: traceID, StartedAt: now, Agent: r.agent},
		Base:     map[string]any{tracelog.AttrAgent: r.agent},
		Resource: tracelog.Resource(version),
	}
}

// traceCommand is the CLI invocation prefix next actions name.
func traceCommand() string {
	if cfg, err := loadConfigLenient(); err == nil && cfg.Command != "" {
		return cfg.Command
	}
	return "spektacular"
}

func runTraceStart(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input: &schemaObj{Type: "object", Properties: map[string]*schemaProp{
				"name": {Type: "string", Description: "the span's name, given as the positional argument", MaxLen: maxSpanName},
			}, Required: []string{"name"}},
			Output: &schemaObj{Type: "object", Properties: map[string]*schemaProp{
				"span_id":  {Type: "string", Description: "the span's id, <trace_id>-<span_id>", Pattern: spanRefPattern},
				"name":     {Type: "string"},
				"type":     {Type: "string", Enum: tracelog.AgentTypes},
				"parent":   {Type: "string", Pattern: spanRefPattern},
				"recorded": {Type: "boolean", Description: "false when logging is off: nothing was written"},
			}},
			Flags: map[string]*schemaProp{
				"type":   {Type: "string", Enum: tracelog.AgentTypes, Description: "the span's type (default work)"},
				"parent": {Type: "string", Pattern: spanRefPattern, Description: "the open span to nest under (default: the current step's span)"},
			},
		}, "")
	}
	command := traceCommand()
	typ, _ := cmd.Flags().GetString("type")
	parentArg, _ := cmd.Flags().GetString("parent")

	name := ""
	if len(args) > 0 {
		name = strings.TrimSpace(strings.Join(args, " "))
	}
	if name == "" || strings.ContainsAny(name, "\r\n") || utf8.RuneCountInString(name) > maxSpanName {
		return output.NewError("invalid_span_name",
			fmt.Sprintf("a span needs a name of one line and at most %d characters, not %q", maxSpanName, name)).
			WithNextAction(fmt.Sprintf("name what the span times, for example: %s trace start test-writer --type %s", command, typ))
	}
	if !isAgentType(typ) {
		msg := fmt.Sprintf("span type %q is not one an agent can start", typ)
		if typ == tracelog.TypeStep || typ == tracelog.TypeTask {
			msg = fmt.Sprintf("span type %q is recorded by the workflow itself, as it moves", typ)
		}
		return output.NewError("invalid_span_type", msg).
			WithResource(typ).
			WithNextAction(fmt.Sprintf("pass --type as one of %s: %s trace start %s --type sub-agent", strings.Join(tracelog.AgentTypes, ", "), command, quoteArg(name)))
	}
	var parentRef *tracelog.SpanRef
	if parentArg != "" {
		ref, err := tracelog.ParseSpanRef(parentArg)
		if err != nil {
			return output.NewError("invalid_span_id", "--parent "+err.Error()).
				WithResource(parentArg).
				WithNextAction(fmt.Sprintf("pass --parent the id a trace start printed, or a step span id from the step's instruction, or leave it out to nest under the current step: %s trace start %s --type %s", command, quoteArg(name), typ))
		}
		parentRef = &ref
	}

	dir, err := dataDir()
	if err != nil {
		return err
	}
	_, state := resolveTraceRun(dir, parentRef)
	traceID := ""
	switch {
	case parentRef != nil:
		traceID = parentRef.TraceID
	case state != nil:
		traceID = state.RunID
	default:
		traceID = tracelog.NewTraceID()
	}

	rec, on := activeSpans.(*spanRecorder)
	if !on || rec == nil {
		res := TraceStartResult{SpanID: tracelog.SpanRef{TraceID: traceID, SpanID: tracelog.NewSpanID()}.String(), Name: name, Type: typ, Recorded: false}
		if parentRef != nil {
			res.Parent = parentRef.String()
		}
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(res)
	}

	now := time.Now().UTC()
	sp := traceSpans(rec, state, traceID, now)
	parent := ""
	switch {
	case parentRef != nil:
		if !traceParentOpen(sp, state, parentRef.SpanID) {
			return output.NewError("parent_not_open",
				fmt.Sprintf("span %s is not open: it was never started, has been stopped, or its step has ended", parentRef)).
				WithResource(parentRef.String()).
				WithNextAction(fmt.Sprintf("start the span without --parent to nest it under the current step (%s trace start %s --type %s), or pass --parent an open span's id", command, quoteArg(name), typ))
		}
		parent = parentRef.SpanID
	case state != nil && state.StepSpan != nil:
		parent = state.StepSpan.ID
	}

	attrs := map[string]any{}
	if state != nil {
		attrs[tracelog.AttrStep] = state.CurrentStep
	}
	start := sp.Start(name, typ, parent, now, attrs)
	res := TraceStartResult{
		SpanID:   tracelog.SpanRef{TraceID: traceID, SpanID: start.SpanID}.String(),
		Name:     name,
		Type:     typ,
		Recorded: true,
	}
	if parent != "" {
		res.Parent = tracelog.SpanRef{TraceID: traceID, SpanID: parent}.String()
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(res)
}

// traceParentOpen reports whether span id is open in the run: the
// workflow's own open step or task span, or an agent span started and not
// stopped.
func traceParentOpen(sp tracelog.Spans, state *workflow.State, id string) bool {
	if state != nil {
		if (state.StepSpan != nil && state.StepSpan.ID == id) || (state.TaskSpan != nil && state.TaskSpan.ID == id) {
			return true
		}
	}
	start, open, found := sp.Find(id)
	if !found || !open {
		return false
	}
	// A step or task span the workflow has left behind is closed even when
	// its end was never logged.
	typ, _ := start.Attributes[tracelog.AttrSpanType].(string)
	return isAgentType(typ)
}

func runTraceStop(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input: &schemaObj{Type: "object", Properties: map[string]*schemaProp{
				"id": {Type: "string", Description: "the id trace start printed, given as the positional argument", Pattern: spanRefPattern},
			}, Required: []string{"id"}},
			Output: &schemaObj{Type: "object", Properties: map[string]*schemaProp{
				"span_id":  {Type: "string", Pattern: spanRefPattern},
				"status":   {Type: "string", Enum: []string{tracelog.StatusOK, tracelog.StatusError}},
				"recorded": {Type: "boolean", Description: "false when logging is off: nothing was written"},
			}},
			Flags: map[string]*schemaProp{
				"status":  {Type: "string", Enum: []string{tracelog.StatusOK, tracelog.StatusError}, Description: "how the span ended (default ok)"},
				"message": {Type: "string", Description: "what went wrong, with --status error"},
			},
		}, "")
	}
	command := traceCommand()
	status, _ := cmd.Flags().GetString("status")
	message, _ := cmd.Flags().GetString("message")

	idArg := ""
	if len(args) > 0 {
		idArg = args[0]
	}
	ref, err := tracelog.ParseSpanRef(idArg)
	if err != nil {
		return output.NewError("invalid_span_id", err.Error()).
			WithResource(idArg).
			WithNextAction(fmt.Sprintf("pass the id the trace start printed: %s trace stop <trace_id>-<span_id>", command))
	}
	if status != tracelog.StatusOK && status != tracelog.StatusError {
		return output.NewError("invalid_span_status",
			fmt.Sprintf("span status %q is not %q or %q", status, tracelog.StatusOK, tracelog.StatusError)).
			WithResource(status).
			WithNextAction(fmt.Sprintf("pass --status ok, or --status error with a --message: %s trace stop %s --status error --message \"<what went wrong>\"", command, ref))
	}

	rec, on := activeSpans.(*spanRecorder)
	if !on || rec == nil {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(TraceStopResult{SpanID: ref.String(), Status: status, Recorded: false})
	}

	dir, err := dataDir()
	if err != nil {
		return err
	}
	_, state := resolveTraceRun(dir, &ref)
	if state != nil && ((state.StepSpan != nil && state.StepSpan.ID == ref.SpanID) || (state.TaskSpan != nil && state.TaskSpan.ID == ref.SpanID)) {
		return workflowOwnedSpanError(command, ref)
	}
	now := time.Now().UTC()
	sp := traceSpans(rec, state, ref.TraceID, now)
	start, open, found := sp.Find(ref.SpanID)
	if !found {
		return output.NewError("span_not_open",
			fmt.Sprintf("span %s is not open: it was never started here, or its run has finished", ref)).
			WithResource(ref.String()).
			WithNextAction(fmt.Sprintf("check the id against the one trace start printed; a span whose run has finished needs no stop. Start a new span with: %s trace start <name> --type sub-agent", command))
	}
	if typ, _ := start.Attributes[tracelog.AttrSpanType].(string); !isAgentType(typ) {
		return workflowOwnedSpanError(command, ref)
	}
	if !open {
		return output.NewError("span_already_stopped",
			fmt.Sprintf("span %s has already ended", ref)).
			WithResource(ref.String()).
			WithNextAction("nothing to do: the span is recorded. Carry on with your next command")
	}
	st := tracelog.Status{Code: status}
	if status == tracelog.StatusError {
		st.Message = message
	}
	sp.End(start, now, st, tracelog.EndStopped)
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(TraceStopResult{SpanID: ref.String(), Status: status, Recorded: true})
}

// workflowOwnedSpanError refuses to stop a step or task span.
func workflowOwnedSpanError(command string, ref tracelog.SpanRef) error {
	return output.NewError("span_owned_by_workflow",
		fmt.Sprintf("span %s is a step or task span: Spektacular closes step and task spans when the workflow moves", ref)).
		WithResource(ref.String()).
		WithNextAction(fmt.Sprintf("stop only the spans you started with %s trace start; move the workflow with its goto to end the step", command))
}

// isAgentType reports whether typ is a span type an agent may start.
func isAgentType(typ string) bool {
	for _, t := range tracelog.AgentTypes {
		if t == typ {
			return true
		}
	}
	return false
}

// spanRefPattern is the shape of a span id an agent holds.
const spanRefPattern = `^[0-9a-f]{32}-[0-9a-f]{16}$`

// quoteArg quotes a span name for a shell when it needs it.
func quoteArg(s string) string {
	if s == "" {
		return "<name>"
	}
	if strings.ContainsAny(s, " \t'\"$`\\") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

func init() {
	traceStartCmd.Flags().String("type", tracelog.TypeWork, "Span type: sub-agent, user-wait or work")
	traceStartCmd.Flags().String("parent", "", "Id of the open span to nest under (default: the current step's span)")
	traceStartCmd.Flags().Bool("schema", false, "Print the input/output schema and exit")
	traceStopCmd.Flags().String("status", tracelog.StatusOK, "How the span ended: ok or error")
	traceStopCmd.Flags().String("message", "", "What went wrong, with --status error")
	traceStopCmd.Flags().Bool("schema", false, "Print the input/output schema and exit")
	traceCmd.AddCommand(traceStartCmd, traceStopCmd)
	rootCmd.AddCommand(traceCmd)
}
