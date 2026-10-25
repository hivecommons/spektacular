package runstats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fiveImplementRuns: analyze takes 10, 20, 30, 40 and 50 s; verify is
// entered by the first three runs only.
func fiveImplementRuns() []*Run {
	var runs []*Run
	for i := 0; i < 5; i++ {
		steps := []step{{name: "analyze", dur: time.Duration(10*(i+1)) * time.Second}}
		if i < 3 {
			steps = append(steps, step{name: "verify", dur: time.Minute})
		}
		runs = append(runs, seqRun(fmt.Sprintf("%04d0000000000000000000000000000", i+1), "implement", float64(60*i), steps...))
	}
	return runs
}

func statNamed(t *testing.T, ss []Stat, name string) Stat {
	t.Helper()
	for _, s := range ss {
		if s.Figure == name {
			return s
		}
	}
	t.Fatalf("no stat %s", name)
	return Stat{}
}

func stepStats(t *testing.T, s Summary, name string) []Stat {
	t.Helper()
	for _, st := range s.Steps {
		if st.Step == name {
			return st.Figures
		}
	}
	t.Fatalf("no step %s", name)
	return nil
}

func TestSummariseFiveImplementRuns(t *testing.T) {
	s := Summarise(fiveImplementRuns(), DefaultOptions)
	require.Len(t, s.Runs, 5)

	require.Equal(t, Stat{Figure: "duration_ms", Count: 5, Median: f64(30000), P90: f64(50000)},
		statNamed(t, stepStats(t, s, "analyze"), "duration_ms"))
	require.Equal(t, Stat{Figure: "entries", Count: 5, Median: f64(1), P90: f64(1)},
		statNamed(t, stepStats(t, s, "analyze"), "entries"))

	// A step only some runs entered counts only those.
	require.Equal(t, Stat{Figure: "duration_ms", Count: 3, Median: f64(60000), P90: f64(60000)},
		statNamed(t, stepStats(t, s, "verify"), "duration_ms"))

	// Totals: wall is 1 s + analyze (+ 60 s for the first three runs):
	// 71, 81, 91, 41, 51 s.
	require.Equal(t, Stat{Figure: "wall_ms", Count: 5, Median: f64(71000), P90: f64(91000)},
		statNamed(t, s.Totals, "wall_ms"))
	for _, st := range s.Totals[:8] {
		require.Equal(t, 5, st.Count, st.Figure)
	}
	require.Len(t, s.Totals, 14)
	for _, st := range s.Totals[8:] {
		require.Equal(t, Stat{Figure: st.Figure}, st, "no token input: the token figures apply to no run")
	}
}

func TestSummariseEvenCountAndNone(t *testing.T) {
	require.Equal(t, Stat{Figure: "x", Count: 4, Median: f64(25), P90: f64(40)},
		newStat("x", []*float64{f64(40), f64(10), nil, f64(30), f64(20)}))
	require.Equal(t, Stat{Figure: "x", Count: 2, Median: f64(1.5), P90: f64(2)},
		newStat("x", []*float64{f64(2), f64(1)}), "an even count can give a fractional median")
	require.Equal(t, Stat{Figure: "x", Count: 1, Median: f64(7), P90: f64(7)}, newStat("x", []*float64{f64(7)}))
	require.Equal(t, Stat{Figure: "x"}, newStat("x", []*float64{nil, nil}), "no value: null median and p90")
	ten := make([]*float64, 10)
	for i := range ten {
		ten[i] = f64(float64(i + 1))
	}
	require.Equal(t, f64(9), newStat("x", ten).P90, "nearest rank: the 9th of 10")
}

func TestSummaryGoldenJSONAndText(t *testing.T) {
	runs := fiveImplementRuns()
	s := Summarise(runs, withTokens(runs, fiveOutputs()...))
	data, err := json.MarshalIndent(s, "", "  ")
	require.NoError(t, err)
	golden(t, "summary.golden.json", string(data)+"\n")

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	require.Equal(t, sorted("gap_threshold_ms", "runs", "totals", "steps"), keys(t, m))
	require.Equal(t, sorted("figure", "count", "median", "p90"), keys(t, first(t, m["totals"])))
	require.Equal(t, sorted("step", "figures"), keys(t, first(t, m["steps"])))

	var b bytes.Buffer
	require.NoError(t, RenderSummary(&b, s, 0))
	out := b.String()
	golden(t, "summary.golden.txt", out)
	require.Contains(t, out, "Runs: 5\n")
	require.Regexp(t, `(?m)^analyze\s+5\s+30\.0s\s+50\.0s$`, out)
	require.Regexp(t, `(?m)^verify\s+3\s+1m00s\s+1m00s$`, out)
	require.Regexp(t, `(?m)^wall_ms\s+5\s+1m11s\s+1m31s$`, out)
}

// fiveOutputs give the five implement runs' analyze steps 10k to 50k
// output tokens.
func fiveOutputs() []map[string]int64 {
	var out []map[string]int64
	for i := 1; i <= 5; i++ {
		out = append(out, map[string]int64{"analyze": int64(10000 * i)})
	}
	return out
}

// tenMinuteRuns are five implement runs whose one step, analyze, takes 10
// minutes.
func tenMinuteRuns() []*Run {
	var runs []*Run
	for i := 0; i < 5; i++ {
		runs = append(runs, seqRun(fmt.Sprintf("%04d0000000000000000000000000000", i+1), "implement", float64(60*i),
			step{name: "analyze", dur: 10 * time.Minute}))
	}
	return runs
}

// Summaries criterion 2: over five runs, the median and 90th percentile of
// tokens, cost and time per thousand output tokens, per step and in total.
func TestSummariseCarriesTokens(t *testing.T) {
	runs := tenMinuteRuns()
	s := Summarise(runs, withTokens(runs, fiveOutputs()...))
	analyze := stepStats(t, s, "analyze")
	require.Equal(t, Stat{Figure: "output_tokens", Count: 5, Median: f64(30000), P90: f64(50000)}, statNamed(t, analyze, "output_tokens"))
	require.Equal(t, Stat{Figure: "cost", Count: 5, Median: f64(0.3), P90: f64(0.5)}, statNamed(t, analyze, "cost"))
	// 10m over 10k..50k: 60, 30, 20, 15 and 12 s per 1k.
	require.Equal(t, Stat{Figure: "working_ms_per_1k_output", Count: 5, Median: f64(20000), P90: f64(60000)},
		statNamed(t, analyze, "working_ms_per_1k_output"))
	require.Equal(t, Stat{Figure: "output_tokens", Count: 5, Median: f64(30000), P90: f64(50000)}, statNamed(t, s.Totals, "output_tokens"))
	require.Equal(t, Stat{Figure: "cost", Count: 5, Median: f64(0.3), P90: f64(0.5)}, statNamed(t, s.Totals, "cost"))
	require.Equal(t, 5, statNamed(t, s.Totals, "working_ms_per_1k_output").Count)
	// The timings are unchanged.
	require.Equal(t, Stat{Figure: "duration_ms", Count: 5, Median: f64(600000), P90: f64(600000)}, statNamed(t, analyze, "duration_ms"))

	var b bytes.Buffer
	require.NoError(t, RenderSummary(&b, s, 0))
	out := b.String()
	require.Contains(t, out, "\nSteps (output tokens)\n")
	require.Regexp(t, `(?m)^analyze\s+5\s+30,000\s+50,000$`, out)
	require.Regexp(t, `(?m)^analyze\s+5\s+0\.30\s+0\.50$`, out)
	require.Regexp(t, `(?m)^analyze\s+5\s+20\.0s\s+1m00s$`, out)
	require.Regexp(t, `(?m)^cost\s+5\s+0\.30\s+0\.50$`, out)
}

// Summaries criterion 3: a run with unknown token use is left out of the
// token figures' count, never counted as zero.
func TestSummariseUnknownTokensAreLeftOut(t *testing.T) {
	runs := tenMinuteRuns()
	outs := fiveOutputs()
	outs[4] = nil
	s := Summarise(runs, withTokens(runs, outs...))
	require.Equal(t, Stat{Figure: "output_tokens", Count: 4, Median: f64(25000), P90: f64(40000)}, statNamed(t, s.Totals, "output_tokens"))
	require.Equal(t, 5, statNamed(t, s.Totals, "wall_ms").Count)
}
