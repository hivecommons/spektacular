package runstats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
	rs := recs(f.cmd(kind+" new", "", at, at.Add(time.Second)))
	at = at.Add(time.Second)
	for i, s := range steps {
		id := fmt.Sprintf("%s%02d", trace[:4], i)
		rs = append(rs, f.span(id, s.name, tracelog.TypeStep, "", at, at.Add(s.dur), tracelog.EndMoved)...)
		if s.wait > 0 {
			rs = append(rs, f.span(id+"w", "question", tracelog.TypeUserWait, id, at, at.Add(s.wait), tracelog.EndStopped)...)
		}
		rs = append(rs, f.cmd(kind+" goto", id, at.Add(s.dur-time.Second), at.Add(s.dur)))
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
		Values:   []*int64{i64(600000), i64(720000)},
		Delta:    []*int64{i64(0), i64(120000)},
		DeltaPct: []*float64{f64(0), f64(20)},
	}, interview)

	// A step only the first run entered is absent, never zero, for the other.
	split := figureNamed(t, stepNamed(t, c, "split").Figures, "duration_ms")
	require.Equal(t, []*int64{i64(300000), nil}, split.Values)
	require.Equal(t, []*int64{i64(0), nil}, split.Delta)
	require.Equal(t, []*float64{f64(0), nil}, split.DeltaPct)
	entries := figureNamed(t, stepNamed(t, c, "split").Figures, "entries")
	require.Equal(t, []*int64{i64(1), nil}, entries.Values)

	// A zero base has a difference but no percentage.
	wait := figureNamed(t, stepNamed(t, c, "interview").Figures, "user_wait_ms")
	require.Equal(t, []*int64{i64(0), i64(60000)}, wait.Values)
	require.Equal(t, []*int64{i64(0), i64(60000)}, wait.Delta)
	require.Equal(t, []*float64{nil, nil}, wait.DeltaPct)

	// Totals compare the same way: wall 17m01s against 14m01s.
	wall := figureNamed(t, c.Totals, "wall_ms")
	require.Equal(t, []*int64{i64(1021000), i64(841000)}, wall.Values)
	require.Equal(t, []*int64{i64(0), i64(-180000)}, wall.Delta)
	require.Equal(t, []*float64{f64(0), f64(-17.6)}, wall.DeltaPct)

	var names []string
	for _, f := range c.Totals {
		names = append(names, f.Figure)
	}
	require.Equal(t, []string{"wall_ms", "working_ms", "user_wait_ms", "helper_wait_ms", "command_ms", "commands", "helpers", "gap_ms"}, names)
	names = nil
	for _, f := range c.Steps[0].Figures {
		names = append(names, f.Figure)
	}
	require.Equal(t, []string{"duration_ms", "entries", "commands", "command_ms", "helper_wait_ms", "user_wait_ms"}, names)
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
	require.Equal(t, []*int64{nil, i64(60000)}, approach.Values)
	require.Equal(t, []*int64{nil, nil}, approach.Delta, "no base, no difference")
	require.Equal(t, []*float64{nil, nil}, approach.DeltaPct)
}

func TestCompareGoldenJSONAndText(t *testing.T) {
	c := Compare(twoSpecRuns(), DefaultOptions)
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
