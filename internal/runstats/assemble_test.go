package runstats

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

func TestAssembleRunIdentityAndExtent(t *testing.T) {
	a := fx{trace: traceA, kind: "plan", name: "000002_x", agent: "codex"}
	runs := Assemble(recs(
		a.cmd("plan new", "", at(1), at(1.5)),
		a.span("p1", "discovery", tracelog.TypeStep, "", at(1.5), at(10), tracelog.EndMoved),
		a.cmd("plan goto", "p1", at(9), at(10)),
		a.open("p2", "approach", tracelog.TypeStep, "", at(10)),
		a.cmd("plan goto", "p2", at(12), at(12.5)),
	))
	require.Len(t, runs, 1)
	r := runs[0]
	require.Equal(t, "plan", r.Kind)
	require.Equal(t, "000002_x", r.Name)
	require.Equal(t, "codex", r.Agent)
	require.Equal(t, at(1), r.Start)
	require.Equal(t, at(12.5), r.End, "a run in progress is measured to its latest event")
	require.Equal(t, 11*time.Minute+30*time.Second, r.Wall())
	require.Equal(t, StatusInProgress, r.Status)

	open := r.Spans[1]
	require.Equal(t, "approach", open.Name)
	require.Equal(t, EndOpen, open.EndReason)
	require.False(t, open.Complete)
	require.Equal(t, at(12.5), open.End, "an open span is capped at the run's last event")
}

func TestAssembleAgentFallsBackToUnknown(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n"}
	runs := Assemble(recs(a.cmd("spec new", "", at(0), at(1))))
	require.Equal(t, "unknown", runs[0].Agent)
}

func TestAssemblePairsStartsAndEnds(t *testing.T) {
	a := fx{trace: traceA, kind: "implement", name: "n", agent: "claude"}
	helper := a.open("h1", "research", tracelog.TypeSubAgent, "s1", at(2), map[string]any{tracelog.AttrStep: "analyze"})
	endOnly := closeSpan(a.open("t1", "Task one", tracelog.TypeTask, "", at(1), map[string]any{tracelog.AttrTaskID: "task-1"}), at(8), tracelog.EndMoved)
	runs := Assemble(recs(
		a.cmd("implement new", "", at(0), at(1)),
		endOnly, // its start was never logged
		a.span("s1", "analyze", tracelog.TypeStep, "t1", at(1), at(8), tracelog.EndMoved),
		helper,
		closeSpan(helper, at(4), tracelog.EndStopped),
	))
	r := runs[0]
	require.Len(t, r.Spans, 3)
	byID := map[string]Span{}
	for _, s := range r.Spans {
		byID[s.ID] = s
	}

	task := byID["t1"]
	require.Equal(t, tracelog.TypeTask, task.Type)
	require.Equal(t, "task-1", task.TaskID)
	require.Equal(t, at(1), task.Start, "an end with no start uses its own start time")
	require.Equal(t, at(8), task.End)
	require.True(t, task.Complete)

	h := byID["h1"]
	require.Equal(t, "s1", h.Parent)
	require.Equal(t, "analyze", h.Step)
	require.Equal(t, at(2), h.Start)
	require.Equal(t, at(4), h.End)
	require.Equal(t, tracelog.EndStopped, h.EndReason)
	require.Equal(t, tracelog.StatusOK, h.Status)
	require.True(t, h.Complete)
	require.Equal(t, 2*time.Minute, h.Duration())
}

func TestAssembleAbandonedEndDoesNotStretchTheRun(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	step := a.open("s1", "interview", tracelog.TypeStep, "", at(1))
	helper := a.open("h1", "research", tracelog.TypeSubAgent, "s1", at(2))
	restart := at(60 * 24 * 3) // closed three days later by a --force restart
	runs := Assemble(recs(
		a.cmd("spec new", "", at(0), at(1)),
		step, helper,
		a.cmd("spec goto", "s1", at(5), at(6)),
		closeSpan(helper, restart, tracelog.EndAbandoned),
		closeSpan(step, restart, tracelog.EndAbandoned),
	))
	r := runs[0]
	require.Equal(t, at(6), r.End, "the restart's closing records are left out of the extent")
	require.Equal(t, 6*time.Minute, r.Wall())
	for _, s := range r.Spans {
		require.Equal(t, tracelog.EndAbandoned, s.EndReason)
		require.False(t, s.Complete)
		require.Equal(t, at(6), s.End, "an abandoned span is capped at the run's last other event")
	}
	require.Equal(t, StatusInterrupted, r.Status, "a restart abandoned its step")
	for _, e := range r.Events {
		require.False(t, e.At.After(at(6)))
	}
}

func TestAssembleInterruptedStepKeepsItsCutOff(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	runs := Assemble(recs(
		a.cmd("spec new", "", at(0), at(1)),
		a.span("s1", "interview", tracelog.TypeStep, "", at(1), at(4), tracelog.EndInterrupted),
		a.cmd("spec goto", "s1", at(3), at(4)),
		a.span("s1b", "interview", tracelog.TypeStep, "", at(30), at(31), tracelog.EndMoved),
	))
	s := runs[0].Spans[0]
	require.Equal(t, tracelog.EndInterrupted, s.EndReason)
	require.False(t, s.Complete)
	require.Equal(t, at(4), s.End)
}

func TestAssembleStatus(t *testing.T) {
	spec := func(trace string, start float64, extra ...tracelog.Record) []tracelog.Record {
		a := fx{trace: trace, kind: "spec", name: "same", agent: "claude"}
		return append(recs(
			a.cmd("spec new", "", at(start), at(start+1)),
			a.open(trace[:4]+"s", "interview", tracelog.TypeStep, "", at(start+1)),
		), extra...)
	}
	finishedBySpan := fx{trace: "f1000000000000000000000000000001", kind: "plan", name: "p", agent: "claude"}
	finishedByCommand := fx{trace: "f2000000000000000000000000000002", kind: "plan", name: "q", agent: "claude"}

	runs := Assemble(recs(
		// finished: the terminal step's instant span closed finished.
		finishedBySpan.cmd("plan new", "", at(0), at(1)),
		finishedBySpan.span("fs", "finished", tracelog.TypeStep, "", at(1), at(1), tracelog.EndFinished),
		// finished: a command moved it to the finished step (pre-span logs).
		finishedByCommand.cmd("plan goto", "", at(0), at(1), map[string]any{tracelog.AttrStepTo: "finished"}),
		// overtaken: a later run of the same kind and name started after it.
		spec("11110000000000000000000000000001", 0),
		spec("22220000000000000000000000000002", 10),
		// a different name never overtakes.
		fx{trace: "33330000000000000000000000000003", kind: "spec", name: "other", agent: "claude"}.cmd("spec new", "", at(100), at(101)),
	))
	status := map[string]string{}
	for _, r := range runs {
		status[r.ID[:4]] = r.Status
	}
	require.Equal(t, map[string]string{
		"f100": StatusFinished,
		"f200": StatusFinished,
		"1111": StatusInterrupted,
		"2222": StatusInProgress,
		"3333": StatusInProgress,
	}, status)
}

func TestAssembleNewestFirstAndStandaloneExcluded(t *testing.T) {
	older := fx{trace: "0100000000000000000000000000000a", kind: "spec", name: "a", agent: "claude"}
	newer := fx{trace: "0200000000000000000000000000000b", kind: "plan", name: "a", agent: "claude"}
	alone := fx{trace: traceS, agent: "claude"}
	runs := Assemble(recs(
		older.cmd("spec new", "", at(0), at(1)),
		alone.cmd("status", "", at(3), at(4)),
		alone.open("w", "orchestrator lane", tracelog.TypeSubAgent, "", at(3)),
		newer.cmd("plan new", "", at(5), at(6)),
	))
	require.Len(t, runs, 2)
	require.Equal(t, newer.trace, runs[0].ID)
	require.Equal(t, older.trace, runs[1].ID)
}

func TestAssembleTimelineLeavesOutAbandonedEnds(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	w := a.open("w1", "notes", tracelog.TypeWork, "", at(2))
	runs := Assemble(recs(
		a.cmd("spec new", "", at(0), at(1)),
		w,
		a.cmd("spec goto", "", at(3), at(4)),
		closeSpan(w, at(4), tracelog.EndAbandoned),
	))
	require.Equal(t, []Event{
		{At: at(0), Until: at(1), Edge: EdgeEnd, Type: tracelog.TypeCommand, Name: "spec new"},
		{At: at(2), Until: at(2), Edge: EdgeStart, Type: tracelog.TypeWork, Name: "notes"},
		{At: at(3), Until: at(4), Edge: EdgeEnd, Type: tracelog.TypeCommand, Name: "spec goto"},
	}, runs[0].Events)
}
