package cmd

import (
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// activeSpans is the span sink of the command being run: set by runRoot
// before the command executes when logging is on, and nil otherwise, so a
// workflow built with logging off reports nothing. Every workflow config a
// command builds takes it through withSpans.
var activeSpans workflow.SpanSink

// withSpans gives c the command's span sink.
func withSpans(c workflow.Config) workflow.Config {
	c.Spans = activeSpans
	return c
}

// newCommandLogger builds the logger log.provider names, writing to the
// project's log directory. ok is false when none can be built (no project
// root, or an unknown provider, which config validation already refuses).
func newCommandLogger(cfg config.Config) (tracelog.Logger, bool) {
	root, err := projectRoot()
	if err != nil {
		return nil, false
	}
	logger, err := tracelog.New(cfg.Log.Provider, tracelog.FileOptions{
		Directory: cfg.LogDir(root),
		Retention: cfg.Log.Retention,
	})
	if err != nil {
		return nil, false
	}
	return logger, true
}

// spanRecorder is the workflow.SpanSink that writes the workflow's step and
// task spans to the run's log through the configured logger, as span_start
// and span_end records beside the run's command records.
type spanRecorder struct {
	logger tracelog.Logger
	agent  string
}

func newSpanRecorder(logger tracelog.Logger, agent string) *spanRecorder {
	if agent == "" {
		agent = "unknown"
	}
	return &spanRecorder{logger: logger, agent: agent}
}

// spans is the span service for run, writing to the same file as the run's
// command records (see attributeRun).
func (r *spanRecorder) spans(run workflow.RunRef) tracelog.Spans {
	return tracelog.Spans{
		Log: r.logger,
		Run: tracelog.Run{TraceID: run.RunID, Kind: run.Kind, Name: run.Name, StartedAt: run.StartedAt, Agent: r.agent},
		Base: map[string]any{
			tracelog.AttrAgent:        r.agent,
			tracelog.AttrWorkflowKind: run.Kind,
			tracelog.AttrWorkflowName: run.Name,
		},
		Resource: tracelog.Resource(version),
	}
}

// spanAttrs are the attributes a workflow span carries beyond the base.
func spanAttrs(s workflow.SpanInfo) map[string]any {
	attrs := map[string]any{}
	if s.Step != "" {
		attrs[tracelog.AttrStep] = s.Step
	}
	if s.TaskID != "" {
		attrs[tracelog.AttrTaskID] = s.TaskID
	}
	return attrs
}

// startRecord rebuilds the start record of a workflow span from what state
// holds, so ending it needs no log read, and works even when its start was
// never logged.
func (r *spanRecorder) startRecord(sp tracelog.Spans, s workflow.SpanInfo) tracelog.Record {
	return sp.StartRecord(s.ID, s.Name, s.Type, s.Parent, s.Start, spanAttrs(s))
}

func (r *spanRecorder) Started(run workflow.RunRef, s workflow.SpanInfo) {
	if run.RunID == "" {
		return
	}
	r.spans(run).StartWithID(s.ID, s.Name, s.Type, s.Parent, s.Start, spanAttrs(s))
}

// Ended closes every span still open under s, marked abandoned, then s.
func (r *spanRecorder) Ended(run workflow.RunRef, s workflow.SpanInfo, at time.Time, reason workflow.EndReason) {
	if run.RunID == "" {
		return
	}
	sp := r.spans(run)
	sp.EndUnder(s.ID, at, tracelog.EndAbandoned)
	sp.End(r.startRecord(sp, s), at, tracelog.ReasonStatus(string(reason)), string(reason))
}

// Interrupted closes s and every span still open under it, marked
// interrupted, at the end of the last command recorded under s (leaving out
// resume probes and trace commands), or at s's start when there was none.
func (r *spanRecorder) Interrupted(run workflow.RunRef, s workflow.SpanInfo) {
	if run.RunID == "" {
		return
	}
	sp := r.spans(run)
	at, ok := sp.LastCommandUnder(s.ID)
	if !ok || at.Before(s.Start) {
		at = s.Start
	}
	sp.EndUnder(s.ID, at, tracelog.EndInterrupted)
	sp.End(r.startRecord(sp, s), at, tracelog.ReasonStatus(tracelog.EndInterrupted), tracelog.EndInterrupted)
}

// RunEnded closes every span still open in the run, marked abandoned,
// whether the run finished or was abandoned.
func (r *spanRecorder) RunEnded(run workflow.RunRef, at time.Time, _ workflow.EndReason) {
	if run.RunID == "" {
		return
	}
	r.spans(run).EndAll(at, tracelog.EndAbandoned)
}
