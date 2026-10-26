package runstats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

// step is one step of a fixture run: entered for dur, with one command at
// its end, and a wait on the user of wait from its start when wait > 0.
type step struct {
	name      string
	dur, wait time.Duration
}

// seqRun is a run of kind that enters steps one after another from start
// (minutes after t0), then reaches finished.
func seqRun(trace, kind string, start float64, steps ...step) *Run {
	f := fx{trace: trace, kind: kind, name: "000001_demo", agent: "claude"}
	at := at(start)
	rs := recs(f.cmd(kind+" new", "", at, at.Add(time.Second), argv(0, kind, "new")))
	at = at.Add(time.Second)
	for i, s := range steps {
		id := fmt.Sprintf("%s%02d", trace[:4], i)
		rs = append(rs, f.span(id, s.name, tracelog.TypeStep, "", at, at.Add(s.dur), tracelog.EndMoved)...)
		if s.wait > 0 {
			rs = append(rs, f.span(id+"w", "question", tracelog.TypeUserWait, id, at, at.Add(s.wait), tracelog.EndStopped)...)
		}
		rs = append(rs, f.cmd(kind+" goto", id, at.Add(s.dur-time.Second), at.Add(s.dur), argv(0, kind, "goto")))
		at = at.Add(s.dur)
	}
	rs = append(rs, f.span(trace[:4]+"ff", "finished", tracelog.TypeStep, "", at, at, tracelog.EndFinished)...)
	return Assemble(rs)[0]
}

func twoSpecRuns() []*Run {
	m := time.Minute
	a := seqRun("aaaa0000000000000000000000000001", "spec", 0,
		step{name: "overview", dur: 2 * m}, step{name: "interview", dur: 10 * m}, step{name: "split", dur: 5 * m})
	b := seqRun("bbbb0000000000000000000000000002", "spec", 60,
		step{name: "overview", dur: 2 * m}, step{name: "interview", dur: 12 * m, wait: m})
	return []*Run{a, b}
}

func i64(v int64) *int64     { return &v }
func f64(v float64) *float64 { return &v }

func figureNamed(t *testing.T, fs []Figure, name string) Figure {
	t.Helper()
	for _, f := range fs {
		if f.Figure == name {
			return f
		}
	}
	t.Fatalf("no figure %s", name)
	return Figure{}
}

func stepNamed(t *testing.T, c Comparison, name string) StepFigures {
	t.Helper()
	for _, s := range c.Steps {
		if s.Step == name {
			return s
		}
	}
	t.Fatalf("no step %s", name)
	return StepFigures{}
}

func TestCompareTwoSpecRuns(t *testing.T) {
	c := Compare(twoSpecRuns(), DefaultOptions)
	require.Len(t, c.Runs, 2)
	require.Equal(t, "aaaa0000000000000000000000000001", c.Runs[0].RunID, "the first run is the base")
	require.Equal(t, int64(300000), c.GapThresholdMS)

	require.Equal(t, []string{"overview", "interview", "split", "finished"}, func() []string {
		var out []string
		for _, s := range c.Steps {
			out = append(out, s.Step)
		}
		return out
	}())

	interview := figureNamed(t, stepNamed(t, c, "interview").Figures, "duration_ms")
	require.Equal(t, Figure{
		Figure:   "duration_ms",
		Values:   []*float64{f64(600000), f64(720000)},
		Delta:    []*float64{f64(0), f64(120000)},
		DeltaPct: []*float64{f64(0), f64(20)},
	}, interview)

	// A step only the first run entered is absent, never zero, for the other.
	split := figureNamed(t, stepNamed(t, c, "split").Figures, "duration_ms")
	require.Equal(t, []*float64{f64(300000), nil}, split.Values)
	require.Equal(t, []*float64{f64(0), nil}, split.Delta)
	require.Equal(t, []*float64{f64(0), nil}, split.DeltaPct)
	entries := figureNamed(t, stepNamed(t, c, "split").Figures, "entries")
	require.Equal(t, []*float64{f64(1), nil}, entries.Values)

	// A zero base has a difference but no percentage.
	wait := figureNamed(t, stepNamed(t, c, "interview").Figures, "user_wait_ms")
	require.Equal(t, []*float64{f64(0), f64(60000)}, wait.Values)
	require.Equal(t, []*float64{f64(0), f64(60000)}, wait.Delta)
	require.Equal(t, []*float64{nil, nil}, wait.DeltaPct)

	// Totals compare the same way: wall 17m01s against 14m01s.
	wall := figureNamed(t, c.Totals, "wall_ms")
	require.Equal(t, []*float64{f64(1021000), f64(841000)}, wall.Values)
	require.Equal(t, []*float64{f64(0), f64(-180000)}, wall.Delta)
	require.Equal(t, []*float64{f64(0), f64(-17.6)}, wall.DeltaPct)

	var names []string
	for _, f := range c.Totals {
		names = append(names, f.Figure)
	}
	require.Equal(t, []string{"wall_ms", "working_ms", "user_wait_ms", "helper_wait_ms", "command_ms", "commands", "helpers", "gap_ms",
		"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "cost", "working_ms_per_1k_output"}, names, "the token figures come after 000075's")
	names = nil
	for _, f := range c.Steps[0].Figures {
		names = append(names, f.Figure)
	}
	require.Equal(t, []string{"duration_ms", "entries", "commands", "command_ms", "helper_wait_ms", "user_wait_ms",
		"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "cost", "working_ms_per_1k_output"}, names)
}

func TestCompareStepOrderAddsLaterRunsSteps(t *testing.T) {
	m := time.Minute
	a := seqRun("aaaa0000000000000000000000000001", "plan", 0, step{name: "discovery", dur: m})
	b := seqRun("bbbb0000000000000000000000000002", "plan", 10, step{name: "approach", dur: m}, step{name: "discovery", dur: m})
	c := Compare([]*Run{a, b}, DefaultOptions)
	var order []string
	for _, s := range c.Steps {
		order = append(order, s.Step)
	}
	require.Equal(t, []string{"discovery", "finished", "approach"}, order)
	approach := figureNamed(t, stepNamed(t, c, "approach").Figures, "duration_ms")
	require.Equal(t, []*float64{nil, f64(60000)}, approach.Values)
	require.Equal(t, []*float64{nil, nil}, approach.Delta, "no base, no difference")
	require.Equal(t, []*float64{nil, nil}, approach.DeltaPct)
}

func TestCompareGoldenJSONAndText(t *testing.T) {
	runs := twoSpecRuns()
	c := Compare(runs, withTokens(runs, map[string]int64{"overview": 1000, "interview": 5000}, map[string]int64{"overview": 2000, "interview": 4000}))
	data, err := json.MarshalIndent(c, "", "  ")
	require.NoError(t, err)
	golden(t, "compare.golden.json", string(data)+"\n")

	var b bytes.Buffer
	require.NoError(t, RenderComparison(&b, c, 0))
	out := b.String()
	golden(t, "compare.golden.txt", out)
	require.Regexp(t, `(?m)^interview\s+10m00s\s+12m00s \(\+2m00s, \+20\.0%\)$`, out)
	require.Regexp(t, `(?m)^split\s+5m00s\s+-$`, out)
	require.Regexp(t, `(?m)^wall_ms\s+17m01s\s+14m01s \(-3m00s, -17\.6%\)$`, out)
	require.Regexp(t, `(?m)^user_wait_ms\s+0\.0s\s+1m00s \(\+1m00s\)$`, out)
	require.Regexp(t, `(?m)^commands\s+4\s+3 \(-1, -25\.0%\)$`, out)
}

// sessionFor is a Claude Code session that drove run: a Bash call around
// each of its commands, and one opus request a second into each step entry
// with output tokens out[step] (none for a step not named).
func sessionFor(run *Run, out map[string]int64) *agentusage.Session {
	tr := agentusage.Transcript{ID: "main", File: run.ID + ".jsonl", Start: run.Start, End: run.End}
	for _, c := range run.Commands {
		tr.Calls = append(tr.Calls, agentusage.ShellCall{Command: "spektacular " + c.Name,
			Start: c.StartTime.Add(-100 * time.Millisecond), End: c.EndTime.Add(100 * time.Millisecond)})
	}
	for _, s := range run.Spans {
		if n, ok := out[s.Name]; ok && s.Type == tracelog.TypeStep {
			tr.Requests = append(tr.Requests, agentusage.Request{At: s.Start.Add(time.Second), Model: "claude-opus-5-5",
				Usage: agentusage.Usage{Output: n}})
		}
	}
	return &agentusage.Session{ID: "s-" + run.ID[:4], Transcripts: []agentusage.Transcript{tr}}
}

// withTokens are the default options with token input: for each run, a
// session giving its steps out[i]'s output tokens (no session for a nil
// map), and opus priced at 10 per million output tokens.
func withTokens(runs []*Run, out ...map[string]int64) Options {
	o := DefaultOptions
	in := &TokenInput{Prices: Prices{"claude-opus-5-5": {Output: 10}}, AllRuns: runs, Dir: "/sessions"}
	for i, r := range runs {
		if i < len(out) && out[i] != nil {
			in.Sessions = append(in.Sessions, sessionFor(r, out[i]))
		}
	}
	o.Tokens = in
	return o
}

// Comparisons criterion 1: two runs' step and total tokens, cost and time
// per thousand output tokens, with the second's difference in amount and
// percentage.
func TestCompareCarriesTokens(t *testing.T) {
	runs := twoSpecRuns()
	c := Compare(runs, withTokens(runs, map[string]int64{"overview": 1000, "interview": 5000}, map[string]int64{"overview": 2000, "interview": 4000}))

	overview := stepNamed(t, c, "overview").Figures
	require.Equal(t, Figure{Figure: "output_tokens", Values: []*float64{f64(1000), f64(2000)},
		Delta: []*float64{f64(0), f64(1000)}, DeltaPct: []*float64{f64(0), f64(100)}}, figureNamed(t, overview, "output_tokens"))
	require.Equal(t, Figure{Figure: "cost", Values: []*float64{f64(0.01), f64(0.02)},
		Delta: []*float64{f64(0), f64(0.01)}, DeltaPct: []*float64{f64(0), f64(100)}}, figureNamed(t, overview, "cost"))
	// 2m over 1k and over 2k output tokens.
	require.Equal(t, Figure{Figure: "working_ms_per_1k_output", Values: []*float64{f64(120000), f64(60000)},
		Delta: []*float64{f64(0), f64(-60000)}, DeltaPct: []*float64{f64(0), f64(-50)}}, figureNamed(t, overview, "working_ms_per_1k_output"))
	require.Equal(t, []*float64{f64(0), f64(0)}, figureNamed(t, overview, "input_tokens").Values)

	// interview: 10m over 5k; 12m less a 1m wait over 4k.
	pace := figureNamed(t, stepNamed(t, c, "interview").Figures, "working_ms_per_1k_output")
	require.Equal(t, []*float64{f64(120000), f64(165000)}, pace.Values)
	require.Equal(t, []*float64{f64(0), f64(37.5)}, pace.DeltaPct)

	// Totals: 6k output tokens each; the run's working time per 1k.
	require.Equal(t, []*float64{f64(6000), f64(6000)}, figureNamed(t, c.Totals, "output_tokens").Values)
	require.Equal(t, []*float64{f64(0.06), f64(0.06)}, figureNamed(t, c.Totals, "cost").Values)
	total := figureNamed(t, c.Totals, "working_ms_per_1k_output")
	require.Equal(t, []*float64{f64(170167), f64(130167)}, total.Values, "17m01s and 13m01s over 6k output tokens")
	require.Equal(t, []*float64{f64(0), f64(-40000)}, total.Delta)
	require.Equal(t, []*float64{f64(0), f64(-23.5)}, total.DeltaPct)

	// A step only the first run entered: no tokens there (cost 0, pace not
	// applicable), absent for the other run.
	split := stepNamed(t, c, "split").Figures
	require.Equal(t, []*float64{f64(0), nil}, figureNamed(t, split, "output_tokens").Values)
	require.Equal(t, []*float64{f64(0), nil}, figureNamed(t, split, "cost").Values)
	require.Equal(t, []*float64{nil, nil}, figureNamed(t, split, "working_ms_per_1k_output").Values)
}

// Comparisons criterion 3: a run whose token use is unknown is absent for
// the token figures, never zero, and its timings are those without tokens.
func TestCompareUnknownTokensAreAbsent(t *testing.T) {
	runs := twoSpecRuns()
	c := Compare(runs, withTokens(runs, map[string]int64{"overview": 1000}, nil))
	for _, name := range tokenFigures {
		f := figureNamed(t, c.Totals, name)
		require.Nil(t, f.Values[1], name)
		require.Nil(t, f.Delta[1], name)
		require.Nil(t, f.DeltaPct[1], name)
		require.Nil(t, figureNamed(t, stepNamed(t, c, "overview").Figures, name).Values[1], name)
	}
	require.Equal(t, f64(1000), figureNamed(t, c.Totals, "output_tokens").Values[0])

	// No priced model: a null cost, while the tokens are known.
	o := withTokens(runs, map[string]int64{"overview": 1000}, map[string]int64{"overview": 1000})
	o.Tokens.Prices = nil
	cost := figureNamed(t, Compare(runs, o).Totals, "cost")
	require.Equal(t, []*float64{nil, nil}, cost.Values)

	plain := Compare(twoSpecRuns(), DefaultOptions)
	for i, f := range plain.Totals[:8] {
		require.Equal(t, f, c.Totals[i], f.Figure)
	}
	for i, s := range plain.Steps {
		require.Equal(t, s.Figures[:6], c.Steps[i].Figures[:6], s.Step)
	}
}

func TestRenderComparisonShowsTokens(t *testing.T) {
	runs := twoSpecRuns()
	c := Compare(runs, withTokens(runs, map[string]int64{"overview": 1000, "interview": 5000}, map[string]int64{"overview": 2000, "interview": 4000}))
	var b bytes.Buffer
	require.NoError(t, RenderComparison(&b, c, 0))
	out := b.String()
	require.Contains(t, out, "\nSteps (output tokens)\n")
	require.Regexp(t, `(?m)^overview\s+1,000\s+2,000 \(\+1,000, \+100\.0%\)$`, out)
	require.Contains(t, out, "\nSteps (cost)\n")
	require.Regexp(t, `(?m)^overview\s+0\.01\s+0\.02 \(\+0\.01, \+100\.0%\)$`, out)
	require.Contains(t, out, "\nSteps (time per 1k output tokens)\n")
	require.Regexp(t, `(?m)^overview\s+2m00s\s+1m00s \(-1m00s, -50\.0%\)$`, out)
	require.Regexp(t, `(?m)^output_tokens\s+6,000\s+6,000 \(\+0, \+0\.0%\)$`, out)
	require.Regexp(t, `(?m)^working_ms_per_1k_output\s+2m50s\s+2m10s \(-40\.0s, -23\.5%\)$`, out)

	// Without token use the step token tables are left out.
	b.Reset()
	require.NoError(t, RenderComparison(&b, Compare(runs, DefaultOptions), 0))
	require.NotContains(t, b.String(), "Steps (output tokens)")
	require.Regexp(t, `(?m)^output_tokens\s+-\s+-$`, b.String())
}
