package runstats

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

func TestRunKeys(t *testing.T) {
	a := fx{trace: tokSpecTrace, kind: "spec", name: "000001_tok", agent: "claude"}
	run := onlyRun(t, recs(
		a.cmd("spec new", "", at(1), at(1.1), argv(0, "spec", "new")),
		a.cmd("spec status", "", at(2), at(2.1), argv(0, "spec", "status")),
		a.cmd("plan new", "", at(3), at(3.1), argv(0, "plan", "new")),
		a.cmd("spec", "", at(4), at(4.1), argv(0, "spec")),
		a.cmd("spec goto", "", at(5), at(5.1)), // no args attribute
		a.cmd("spec goto", "", at(6), at(6.1), argv(0, "spec", "goto", "--data", `{"step":"x"}`)),
	))
	require.Equal(t, []runKey{{words: "spec new", at: at(1)}, {words: "spec goto", at: at(6)}}, runKeys(run))
}

func TestCallRuns(t *testing.T) {
	run := tokSpec(t, "claude")
	keys := []runKey{{words: "spec new", at: at(1)}}
	sec := time.Second
	cases := []struct {
		name string
		call agentusage.ShellCall
		want bool
	}{
		{"inside", shell(specNewCmd, 0.99, 1.1), true},
		{"record a second before the call", agentusage.ShellCall{Command: specNewCmd, Start: at(1).Add(sec), End: at(1).Add(2 * sec)}, true},
		{"record over a second before the call", agentusage.ShellCall{Command: specNewCmd, Start: at(1).Add(sec + time.Millisecond), End: at(1.5)}, false},
		{"record a second after the result", agentusage.ShellCall{Command: specNewCmd, Start: at(0.5), End: at(1).Add(-sec)}, true},
		{"record over a second after the result", agentusage.ShellCall{Command: specNewCmd, Start: at(0.5), End: at(1).Add(-sec - time.Millisecond)}, false},
		{"end before start", agentusage.ShellCall{Command: specNewCmd, Start: at(1), End: at(0.5)}, true},
		{"whitespace collapsed", shell("spektacular   spec\n  new --data '{\"name\":\"000001_tok\"}'", 1, 1.1), true},
		{"other words", shell("spektacular spec status", 1, 1.1), false},
		{"other kind", shell(planNewCmd, 1, 1.1), false},
		{"no data", shell("spektacular spec new", 1, 1.1), true},
		{"data with no name", shell(`spektacular spec new --data '{"title":"x"}'`, 1, 1.1), true},
		{"another workflow's name", shell(`spektacular spec new --data '{"name":"000009_other"}'`, 1, 1.1), false},
		{"shell-escaped name", shell(`spektacular spec new --data "{\"name\":\"000001_tok\"}"`, 1, 1.1), true},
		{"shell-escaped other name", shell(`spektacular spec new --data "{\"name\": \"000009_other\"}"`, 1, 1.1), false},
		{"one of two names", shell(`spektacular spec new --data '{"name":"000009_other"}' && echo '{"name":"000001_tok"}'`, 1, 1.1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, callRuns(tc.call, run, keys))
		})
	}
}

func TestDrivesNeedsKeys(t *testing.T) {
	// A run with no new or goto record is driven by nothing.
	a := fx{trace: tokSpecTrace, kind: "spec", name: "000001_tok", agent: "claude"}
	run := onlyRun(t, recs(a.span("s1", "overview", tracelog.TypeStep, "", at(1), at(5), tracelog.EndMoved)))
	tr := &tokSpecSession().Transcripts[0]
	require.False(t, drives(tr, run, runKeys(run)))
	require.Empty(t, drivers(run, []*agentusage.Session{tokSpecSession()}))
}

func TestOwnerAt(t *testing.T) {
	r1 := &Run{ID: "r1", Start: at(1), End: at(10)}
	r2 := &Run{ID: "r2", Start: at(5), End: at(20)}
	r3 := &Run{ID: "r3", Start: at(30), End: at(40)}
	cases := []struct {
		name string
		runs []*Run
		at   float64
		want *Run
	}{
		{"before the first", []*Run{r1, r3}, 0, r1},
		{"inside", []*Run{r1, r3}, 5, r1},
		{"at the end", []*Run{r1, r3}, 10, r1},
		{"between, to the next", []*Run{r1, r3}, 15, r3},
		{"after the last, to the last", []*Run{r1, r3}, 45, r3},
		{"overlap, the latest start", []*Run{r1, r2}, 7, r2},
		{"overlap, only the first", []*Run{r1, r2}, 3, r1},
		{"overlap, only the second", []*Run{r1, r2}, 15, r2},
		{"between overlapping and next", []*Run{r1, r2, r3}, 25, r3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Same(t, tc.want, ownerAt(tc.runs, at(tc.at)))
		})
	}
	require.Nil(t, ownerAt(nil, at(1)))
}

func TestTokensResumed(t *testing.T) {
	// The spec run began in one session and was resumed in a second.
	rs := tokSpecRequests()
	first := &agentusage.Session{ID: "sess-a", Transcripts: []agentusage.Transcript{{
		ID: "main", File: "/sessions/sess-a.jsonl", Start: at(0), End: at(5.2),
		Calls:    []agentusage.ShellCall{shell(specNewCmd, 1, 1.1), shell(specGotoApproach, 4.9, 5)},
		Requests: rs[:4],
	}}}
	second := &agentusage.Session{ID: "sess-b", Transcripts: []agentusage.Transcript{{
		ID: "main", File: "/sessions/sess-b.jsonl", Start: at(5.5), End: at(11),
		Calls:    []agentusage.ShellCall{shell(specGotoFinished, 9.9, 10)},
		Requests: rs[4:],
	}}}
	unrelated := &agentusage.Session{ID: "sess-c", Transcripts: []agentusage.Transcript{{
		ID: "main", Start: at(0), End: at(11),
		Calls:    []agentusage.ShellCall{shell("spektacular spec status", 2, 2.1)},
		Requests: []agentusage.Request{req(2, "opus", use(1000, 1000, 0, 0))},
	}}}

	rep, sec, rows := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{first, unrelated, second}})
	require.Equal(t, []SessionRef{
		{SessionID: "sess-a", Transcript: "main", File: "/sessions/sess-a.jsonl"},
		{SessionID: "sess-b", Transcript: "main", File: "/sessions/sess-b.jsonl"},
	}, sec.Sessions)
	require.Equal(t, use(210, 21, 2100, 21000), sums(t, sec.Total), "both sessions' requests")
	require.Equal(t, use(90, 9, 900, 9000), sums(t, rows.steps["overview"]))
	require.Equal(t, use(110, 11, 1100, 11000), sums(t, rows.steps["approach"]))
	require.Equal(t, sums(t, sec.Total), stepsPlusOutside(t, rep, sec, rows))
}

// twoRuns is one transcript that drove spec run 000001_tok (09:01 to
// 09:10) and then plan run 000005_next (09:15 to 09:25), with a request
// before the spec run (1), in it (2), between the runs (4), in the plan
// run (8) and after it (16), input tokens only.
func twoRuns(t *testing.T) (spec, plan *Run, in *TokenInput) {
	t.Helper()
	spec = tokSpec(t, "claude")
	p := fx{trace: tokPlanTrace, kind: "plan", name: "000005_next", agent: "claude"}
	plan = onlyRun(t, recs(
		p.cmd("plan new", "", at(15), at(15.1), argv(0, "plan", "new", "--data", `{"name":"000005_next"}`)),
		p.span("d1", "discovery", tracelog.TypeStep, "", at(15.1), at(25), tracelog.EndMoved),
		p.cmd("plan goto", "d1", at(24.9), at(25), argv(0, "plan", "goto", "--data", `{"step":"finished"}`)),
		p.span("f1", "finished", tracelog.TypeStep, "", at(25), at(25), tracelog.EndFinished),
	))
	s := &agentusage.Session{ID: "both", Transcripts: []agentusage.Transcript{{
		ID: "main", File: "/sessions/both.jsonl", Start: at(0), End: at(31),
		Calls: []agentusage.ShellCall{
			shell(specNewCmd, 1, 1.1),
			shell(specGotoApproach, 4.9, 5),
			shell(specGotoFinished, 9.9, 10),
			shell(`spektacular plan new --data '{"name":"000005_next"}'`, 15, 15.1),
			shell(planGotoFinished, 24.9, 25),
		},
		Requests: []agentusage.Request{
			req(0.5, "opus", use(1, 0, 0, 0)),
			req(5, "opus", use(2, 0, 0, 0)),
			req(12, "opus", use(4, 0, 0, 0)),
			req(20, "opus", use(8, 0, 0, 0)),
			req(30, "opus", use(16, 0, 0, 0)),
		},
	}}}
	return spec, plan, &TokenInput{Sessions: []*agentusage.Session{s}, AllRuns: []*Run{plan, spec}}
}

func TestTokensTwoRuns(t *testing.T) {
	spec, plan, in := twoRuns(t)

	_, sec, rows := tokensOf(spec, in)
	require.Equal(t, use(3, 0, 0, 0), sums(t, sec.Total), "before and during the spec run")
	require.Equal(t, use(1, 0, 0, 0), sums(t, sec.OutsideSteps))
	require.Equal(t, use(2, 0, 0, 0), sums(t, rows.steps["approach"]))

	_, sec, rows = tokensOf(plan, in)
	require.Equal(t, use(28, 0, 0, 0), sums(t, sec.Total), "between the runs, during the plan run and after it")
	require.Equal(t, use(20, 0, 0, 0), sums(t, sec.OutsideSteps))
	require.Equal(t, use(8, 0, 0, 0), sums(t, rows.steps["discovery"]))
}

func TestRunsDrivenByIsInStartOrder(t *testing.T) {
	spec, plan, in := twoRuns(t)
	tr := &in.Sessions[0].Transcripts[0]
	got := in.runsDrivenBy(tr, plan)
	require.Len(t, got, 2)
	require.Same(t, spec, got[0])
	require.Same(t, plan, got[1])
	require.True(t, in.isDriver(tr, plan))

	// With no runs given, only the run asked about is considered.
	alone := &TokenInput{}
	got = alone.runsDrivenBy(tr, plan)
	require.Len(t, got, 1)
	require.Same(t, plan, got[0])
}
