package runstats

import (
	"encoding/json"
	"testing"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

// The token fixtures' traces. Run ids are 32 hex digits so a helper's brief
// can name a span as <run id>-<span id>.
const (
	tokSpecTrace = "70c00000000000000000000000000001"
	tokPlanTrace = "70c00000000000000000000000000002"
	tokImplTrace = "70c00000000000000000000000000003"
)

// The shell commands the fixture agents ran.
const (
	specNewCmd       = `spektacular spec new --data '{"name":"000001_tok"}'`
	specGotoApproach = `spektacular spec goto --data '{"step":"approach"}'`
	specGotoFinished = `spektacular spec goto --data '{"step":"finished"}'`
	planNewCmd       = `spektacular plan new --data '{"name":"000002_hlp"}'`
	planGotoArch     = `spektacular plan goto --data '{"step":"architecture"}'`
	planGotoFinished = `spektacular plan goto --data '{"step":"finished"}'`
)

// tokPrices prices two models per million tokens.
var tokPrices = Prices{
	"opus":  {Input: 15, Output: 75, CacheRead: 1.5, CacheWrite: 18.75},
	"haiku": {Input: 1, Output: 5, CacheRead: 0.1, CacheWrite: 1.25},
}

// use is a token count.
func use(in, out, cr, cw int64) agentusage.Usage {
	return agentusage.Usage{Input: in, Output: out, CacheRead: cr, CacheWrite: cw}
}

// req is a request m minutes after t0.
func req(m float64, model string, u agentusage.Usage) agentusage.Request {
	return agentusage.Request{At: at(m), Model: model, Usage: u}
}

// shell is a shell call issued at minute from, its result back at minute to.
func shell(command string, from, to float64) agentusage.ShellCall {
	return agentusage.ShellCall{Command: command, Start: at(from), End: at(to)}
}

// launch is a helper launch at minute m.
func launch(toolUseID, desc, prompt string, m float64) agentusage.Launch {
	return agentusage.Launch{ToolUseID: toolUseID, Description: desc, Prompt: prompt, At: at(m)}
}

// helperT is a helper transcript started by launch launchID (or, with a
// parent, by that helper), running from minute from to minute to.
func helperT(id, launchID, parentID, desc string, from, to float64, rs ...agentusage.Request) agentusage.Transcript {
	return agentusage.Transcript{
		ID: id, File: "/sessions/subagents/" + id + ".jsonl", LaunchID: launchID, ParentID: parentID,
		Description: desc, Start: at(from), End: at(to), Requests: rs,
	}
}

// sums is a figure's four token counts.
func sums(t *testing.T, f *TokenFigure) agentusage.Usage {
	t.Helper()
	require.NotNil(t, f)
	return use(f.Input, f.Output, f.CacheRead, f.CacheWrite)
}

// tokensOf reports the run and builds its token section and row figures.
func tokensOf(run *Run, in *TokenInput) (Report, TokenSection, tokenRows) {
	rep := BuildReport(run, DefaultOptions)
	sec, rows := buildTokens(run, &rep, in)
	return rep, sec, rows
}

// onlyRun assembles records into their one run.
func onlyRun(t *testing.T, rs []tracelog.Record) *Run {
	t.Helper()
	runs := Assemble(rs)
	require.Len(t, runs, 1)
	return runs[0]
}

// tokSpec is spec run 000001_tok by agent, 09:01 to 09:10: overview from
// 09:01.1 to 09:05 and approach to 09:10, each left by a goto, then
// finished. Its working time is 9 minutes.
func tokSpec(t *testing.T, agent string) *Run {
	t.Helper()
	a := fx{trace: tokSpecTrace, kind: "spec", name: "000001_tok", agent: agent}
	return onlyRun(t, recs(
		a.cmd("spec new", "", at(1), at(1.1), argv(0, "spec", "new", "--data", `{"name":"000001_tok"}`)),
		a.span("s1", "overview", tracelog.TypeStep, "", at(1.1), at(5), tracelog.EndMoved),
		a.cmd("spec goto", "s1", at(4.9), at(5), argv(0, "spec", "goto", "--data", `{"step":"approach"}`)),
		a.span("s2", "approach", tracelog.TypeStep, "", at(5), at(10), tracelog.EndMoved),
		a.cmd("spec goto", "s2", at(9.9), at(10), argv(0, "spec", "goto", "--data", `{"step":"finished"}`)),
		a.span("s3", "finished", tracelog.TypeStep, "", at(10), at(10), tracelog.EndFinished),
	))
}

// tokSpecRequests are the spec run's six requests: one before `spec new`,
// three in overview (the last the response that issued the goto) and two
// in approach (the last issuing the goto to finished).
//
// Hand totals: outside 10/1/100/1000; overview 90/9/900/9000; approach
// 110/11/1100/11000; the run 210/21/2100/21000.
func tokSpecRequests() []agentusage.Request {
	return []agentusage.Request{
		req(0.5, "opus", use(10, 1, 100, 1000)),
		req(2, "opus", use(20, 2, 200, 2000)),
		req(3, "opus", use(30, 3, 300, 3000)),
		req(4.85, "opus", use(40, 4, 400, 4000)),
		req(6, "opus", use(50, 5, 500, 5000)),
		req(9.8, "opus", use(60, 6, 600, 6000)),
	}
}

// tokSpecSession is one session whose main transcript drove the spec run,
// with no totals.
func tokSpecSession() *agentusage.Session {
	return &agentusage.Session{ID: "sess-1", Skipped: 2, Transcripts: []agentusage.Transcript{{
		ID: "main", File: "/sessions/sess-1.jsonl", Start: at(0), End: at(11),
		Calls: []agentusage.ShellCall{
			shell(specNewCmd, 1, 1.1),
			shell(specGotoApproach, 4.9, 5),
			shell(specGotoFinished, 9.9, 10),
		},
		Requests: tokSpecRequests(),
	}}}
}

// tokPlan is plan run 000002_hlp, 09:01 to 09:12: discovery from 09:01.1
// to 09:06, architecture to 09:12, then finished, with the extra spans
// given (helper spans).
func tokPlan(t *testing.T, extra ...[]tracelog.Record) *Run {
	t.Helper()
	p := fx{trace: tokPlanTrace, kind: "plan", name: "000002_hlp", agent: "claude"}
	parts := []any{
		p.cmd("plan new", "", at(1), at(1.1), argv(0, "plan", "new", "--data", `{"name":"000002_hlp"}`)),
		p.span("d1", "discovery", tracelog.TypeStep, "", at(1.1), at(6), tracelog.EndMoved),
		p.cmd("plan goto", "d1", at(5.9), at(6), argv(0, "plan", "goto", "--data", `{"step":"architecture"}`)),
		p.span("a1", "architecture", tracelog.TypeStep, "", at(6), at(12), tracelog.EndMoved),
		p.cmd("plan goto", "a1", at(11.9), at(12), argv(0, "plan", "goto", "--data", `{"step":"finished"}`)),
		p.span("f1", "finished", tracelog.TypeStep, "", at(12), at(12), tracelog.EndFinished),
	}
	for _, e := range extra {
		parts = append(parts, e)
	}
	return onlyRun(t, recs(parts...))
}

// helperSpan is a plan run sub-agent span from minute from to minute to.
func helperSpan(id, name string, from, to float64) []tracelog.Record {
	p := fx{trace: tokPlanTrace, kind: "plan", name: "000002_hlp", agent: "claude"}
	return p.span(id, name, tracelog.TypeSubAgent, "", at(from), at(to), tracelog.EndStopped)
}

// planMain is the plan run's main transcript, with one request in
// discovery (1/1/0/0) and one in architecture (2/2/0/0), and the launches
// given.
func planMain(launches ...agentusage.Launch) agentusage.Transcript {
	return agentusage.Transcript{
		ID: "main", File: "/sessions/plan.jsonl", Start: at(0.5), End: at(12.5),
		Calls: []agentusage.ShellCall{
			shell(planNewCmd, 1, 1.1),
			shell(planGotoArch, 5.9, 6),
			shell(planGotoFinished, 11.9, 12),
		},
		Requests: []agentusage.Request{req(3, "opus", use(1, 1, 0, 0)), req(8, "opus", use(2, 2, 0, 0))},
		Launches: launches,
	}
}

// planSession is the plan run's session: its main transcript with the
// launches, then the helper transcripts.
func planSession(launches []agentusage.Launch, helpers ...agentusage.Transcript) *agentusage.Session {
	return &agentusage.Session{ID: "plan-sess", Transcripts: append([]agentusage.Transcript{planMain(launches...)}, helpers...)}
}

// tokImpl is implement run 000003_tsk, 09:01 to 09:20, all in one step,
// tasks: task 1 from 09:02 to 09:08 with a minute's user wait and a
// one-minute helper, then task 2 from 09:09 to 09:15.
func tokImpl(t *testing.T) *Run {
	t.Helper()
	m := fx{trace: tokImplTrace, kind: "implement", name: "000003_tsk", agent: "claude"}
	return onlyRun(t, recs(
		m.cmd("implement new", "", at(1), at(1.1), argv(0, "implement", "new", "--data", `{"name":"000003_tsk"}`)),
		m.span("e1", "tasks", tracelog.TypeStep, "", at(1.1), at(20), tracelog.EndMoved),
		m.span("t1", "task one", tracelog.TypeTask, "e1", at(2), at(8), tracelog.EndStopped, map[string]any{tracelog.AttrTaskID: "1"}),
		m.span("w1", "question", tracelog.TypeUserWait, "t1", at(4), at(5), tracelog.EndStopped),
		m.span("0000000000000a01", "tests", tracelog.TypeSubAgent, "t1", at(5.5), at(6.5), tracelog.EndStopped),
		m.span("t2", "task two", tracelog.TypeTask, "e1", at(9), at(15), tracelog.EndStopped, map[string]any{tracelog.AttrTaskID: "2"}),
		m.cmd("implement goto", "e1", at(19.9), at(20), argv(0, "implement", "goto", "--data", `{"step":"finished"}`)),
		m.span("e2", "finished", tracelog.TypeStep, "", at(20), at(20), tracelog.EndFinished),
	))
}

// tokImplSession drove the implement run: a request in the step before
// task 1 (5/5), two in task 1 (100/10/1000 and 100/20), one in task 2
// (300/40), one in the step after the tasks (7/7), and a helper in task 1
// (50/5).
func tokImplSession() *agentusage.Session {
	return &agentusage.Session{ID: "impl-sess", Transcripts: []agentusage.Transcript{
		{
			ID: "main", File: "/sessions/impl.jsonl", Start: at(0.5), End: at(20.5),
			Calls: []agentusage.ShellCall{
				shell(`spektacular implement new --data '{"name":"000003_tsk"}'`, 1, 1.1),
				shell(`spektacular implement goto --data '{"step":"finished"}'`, 19.9, 20),
			},
			Requests: []agentusage.Request{
				req(1.5, "opus", use(5, 5, 0, 0)),
				req(3, "opus", use(100, 10, 1000, 0)),
				req(7, "opus", use(100, 20, 0, 0)),
				req(10, "opus", use(300, 40, 0, 0)),
				req(16, "opus", use(7, 7, 0, 0)),
			},
			Launches: []agentusage.Launch{launch("toolu_t", "tests", "Write the tests.", 5.505)},
		},
		helperT("agent-t", "toolu_t", "", "tests", 5.51, 6.5, req(6, "opus", use(50, 5, 0, 0))),
	}}
}

func TestTokensUnknown(t *testing.T) {
	cases := []struct {
		name      string
		agent     string
		in        *TokenInput
		code      string
		reasonHas []string
	}{
		{name: "notRead", agent: "claude", in: nil, code: ReasonNotRead,
			reasonHas: []string{"without reading agent sessions"}},
		{name: "bobRun", agent: "bob", in: &TokenInput{Sessions: []*agentusage.Session{tokSpecSession()}},
			code: ReasonAgentUnsupported, reasonHas: []string{"agent is bob"}},
		{name: "readerProblem", agent: "claude",
			in:   &TokenInput{Problems: []agentusage.Problem{{Code: agentusage.ProblemDirMissing, Message: "no sessions directory at /home/u/.claude/projects/p"}}},
			code: agentusage.ProblemDirMissing, reasonHas: []string{"no sessions directory at /home/u/.claude/projects/p"}},
		{name: "noSession", agent: "claude",
			in: &TokenInput{
				Dir: "/home/u/.claude/projects/p",
				// The session ran `spec new` for this name, but twenty
				// minutes after the run's record: not this run's.
				Sessions: []*agentusage.Session{{ID: "other", Transcripts: []agentusage.Transcript{{
					ID: "main", Start: at(20), End: at(31),
					Calls:    []agentusage.ShellCall{shell(specNewCmd, 21, 21.1)},
					Requests: []agentusage.Request{req(22, "opus", use(1, 1, 1, 1))},
				}}}},
				Problems: []agentusage.Problem{
					{Code: agentusage.ProblemDirMissing, Message: "ignored"},
					{Code: agentusage.ProblemUnreadable, Message: "cannot read /home/u/.claude/projects/p/bad.jsonl"},
				},
			},
			code: ReasonSessionNotFound,
			reasonHas: []string{"/home/u/.claude/projects/p ran this run's commands", "(1 sessions searched)",
				"; cannot read /home/u/.claude/projects/p/bad.jsonl"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, sec, rows := tokensOf(tokSpec(t, tc.agent), tc.in)
			require.Equal(t, TokensUnknown, sec.Status)
			require.Equal(t, tc.agent, sec.Agent)
			require.NotNil(t, sec.ReasonCode)
			require.Equal(t, tc.code, *sec.ReasonCode)
			require.NotNil(t, sec.Reason)
			for _, s := range tc.reasonHas {
				require.Contains(t, *sec.Reason, s)
			}
			require.Nil(t, sec.Total)
			require.Nil(t, sec.OutsideSteps)
			require.Nil(t, sec.SessionOverhead)
			require.Empty(t, sec.Sessions)
			require.NotNil(t, sec.Sessions)
			require.Empty(t, sec.Models)
			require.NotNil(t, sec.Models)
			require.Empty(t, sec.UnmatchedHelpers)
			require.NotNil(t, sec.UnmatchedHelpers)
			require.Equal(t, tokenRows{}, rows, "no row has a figure")
		})
	}
}

func TestTokensUnknownJSON(t *testing.T) {
	_, sec, _ := tokensOf(tokSpec(t, "bob"), &TokenInput{})
	b, err := json.Marshal(sec)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, "unknown", got["status"])
	require.Equal(t, "agent_unsupported", got["reason_code"])
	require.Nil(t, got["total"])
	require.Nil(t, got["outside_steps"])
	require.Nil(t, got["session_overhead"])
	require.Equal(t, []any{}, got["models"])
	require.Equal(t, []any{}, got["sessions"])
	require.Equal(t, []any{}, got["unmatched_helpers"])
}

func TestTokensSessionsAndSkippedLines(t *testing.T) {
	_, sec, _ := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{tokSpecSession()}})
	require.Equal(t, TokensKnown, sec.Status)
	require.Nil(t, sec.ReasonCode)
	require.Nil(t, sec.Reason)
	require.Equal(t, []SessionRef{{SessionID: "sess-1", Transcript: "main", File: "/sessions/sess-1.jsonl"}}, sec.Sessions)
	require.Equal(t, 2, sec.SkippedLines)
}

func TestTokensUnknownAgentIsRead(t *testing.T) {
	// A run that names no agent is read from Claude Code sessions.
	_, sec, _ := tokensOf(tokSpec(t, ""), &TokenInput{Sessions: []*agentusage.Session{tokSpecSession()}})
	require.Equal(t, "unknown", sec.Agent)
	require.Equal(t, TokensKnown, sec.Status)
	require.Equal(t, use(210, 21, 2100, 21000), sums(t, sec.Total))
}

func TestTokensModelsAndCost(t *testing.T) {
	// The spec run's requests, with the 09:03 one made by haiku.
	s := tokSpecSession()
	s.Transcripts[0].Requests[2].Model = "haiku"
	s.Transcripts[0].Requests = append(s.Transcripts[0].Requests, req(7, "sonnet", use(1000, 0, 0, 0)))

	t.Run("priced", func(t *testing.T) {
		_, sec, rows := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{s}, Prices: tokPrices})
		// haiku 30/3/300/3000 = (30 + 15 + 30 + 3750)/1e6 = 0.003825.
		// opus 180/18/1800/18000 = (2700 + 1350 + 2700 + 337500)/1e6 = 0.34425.
		require.Equal(t, []ModelTokens{
			{Model: "haiku", Input: 30, Output: 3, CacheRead: 300, CacheWrite: 3000, Cost: f64(0.003825)},
			{Model: "opus", Input: 180, Output: 18, CacheRead: 1800, CacheWrite: 18000, Cost: f64(0.34425)},
			{Model: "sonnet", Input: 1000},
		}, sec.Models, "sorted by name; an unpriced model has no cost")
		require.Equal(t, f64(0.348075), sec.Total.Cost)
		require.False(t, sec.Total.CostComplete)
		require.Equal(t, []string{"sonnet"}, sec.Total.UnpricedModels)
		// overview holds opus 60/6/600/6000 and haiku: (900+450+900+112500)/1e6 + 0.003825.
		require.Equal(t, f64(0.118575), rows.steps["overview"].Cost)
		require.True(t, rows.steps["overview"].CostComplete)
		require.Equal(t, []string{}, rows.steps["overview"].UnpricedModels)
	})
}

func TestTokensPace(t *testing.T) {
	t.Run("runStepTaskHelper", func(t *testing.T) {
		_, sec, rows := tokensOf(tokImpl(t), &TokenInput{Sessions: []*agentusage.Session{tokImplSession()}})
		// The run: 19 minutes less a minute's user wait over 87 output.
		require.Equal(t, i64(12413793), sec.Total.WorkingMSPer1kOutput)
		// tasks: 18.9 minutes less the user wait over 87 output.
		require.Equal(t, i64(12344828), rows.steps["tasks"].WorkingMSPer1kOutput)
		// Task 1: 6 minutes less the user wait over 35; task 2: 6 minutes over 40.
		require.Equal(t, i64(8571429), rows.tasks["1"].WorkingMSPer1kOutput)
		require.Equal(t, i64(9000000), rows.tasks["2"].WorkingMSPer1kOutput)
		// The helper: its own minute over its own 5.
		require.Equal(t, i64(12000000), rows.helpers["0000000000000a01"].WorkingMSPer1kOutput)
		// No output: not applicable.
		require.Nil(t, rows.steps["finished"].WorkingMSPer1kOutput)
		require.Nil(t, sec.OutsideSteps.WorkingMSPer1kOutput)
	})

	t.Run("concurrentHelpers", func(t *testing.T) {
		// research runs 09:02 to 09:07 (30 output); review 09:03 to 09:05
		// (40 output), inside it.
		run := tokPlan(t, helperSpan("h1", "research", 2, 7), helperSpan("h2", "review", 3, 5))
		s := planSession(
			[]agentusage.Launch{launch("toolu_1", "research", "Research.", 2.005), launch("toolu_2", "review", "Review.", 3.005)},
			helperT("agent-1", "toolu_1", "", "research", 2.01, 7, req(3, "opus", use(100, 10, 0, 0)), req(6.5, "opus", use(200, 20, 0, 0))),
			helperT("agent-2", "toolu_2", "", "review", 3.01, 5, req(4, "opus", use(10, 40, 0, 0))),
		)
		_, sec, rows := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{s}})
		require.Equal(t, i64(10000000), rows.helpers["h1"].WorkingMSPer1kOutput, "5 minutes over 30, unchanged by review")
		require.Equal(t, i64(3000000), rows.helpers["h2"].WorkingMSPer1kOutput, "2 minutes over 40")
		require.Equal(t, i64(4140845), rows.steps["discovery"].WorkingMSPer1kOutput, "4.9 minutes over 71")
		require.Equal(t, i64(180000000), rows.steps["architecture"].WorkingMSPer1kOutput, "6 minutes over 2")
		require.Equal(t, i64(9041096), sec.Total.WorkingMSPer1kOutput, "11 minutes over 73")
	})
}

// TestTokensPaceSeparatesBusyAndIdleSteps supports the success metric: of
// two ten-minute steps, the one that did much work has a low figure and the
// one that did little a clearly higher one, as the JSON gives them.
func TestTokensPaceSeparatesBusyAndIdleSteps(t *testing.T) {
	a := fx{trace: tokSpecTrace, kind: "spec", name: "000004_pace", agent: "claude"}
	run := onlyRun(t, recs(
		a.cmd("spec new", "", at(0), at(0.1), argv(0, "spec", "new", "--data", `{"name":"000004_pace"}`)),
		a.span("s1", "drafting", tracelog.TypeStep, "", at(0.1), at(10.1), tracelog.EndMoved),
		a.span("s2", "waiting", tracelog.TypeStep, "", at(10.1), at(20.1), tracelog.EndMoved),
	))
	s := &agentusage.Session{ID: "pace", Transcripts: []agentusage.Transcript{{
		ID: "main", Start: at(0), End: at(20.1),
		Calls:    []agentusage.ShellCall{shell(`spektacular spec new --data '{"name":"000004_pace"}'`, 0, 0.1)},
		Requests: []agentusage.Request{req(5, "opus", use(0, 1000, 0, 0)), req(15, "opus", use(0, 20, 0, 0))},
	}}}
	_, _, rows := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{s}})

	pace := func(step string) float64 {
		b, err := json.Marshal(rows.steps[step])
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(b, &got))
		return got["working_ms_per_1k_output"].(float64)
	}
	require.Equal(t, 600000.0, pace("drafting"), "10 minutes over 1000 output")
	require.Equal(t, 30000000.0, pace("waiting"), "10 minutes over 20 output")
	require.Greater(t, pace("waiting"), 10*pace("drafting"))
}
