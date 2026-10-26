package runstats

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/stretchr/testify/require"
)

// goldenSession is the Claude Code session that drove goldenRun: Bash calls
// around the run's `implement new` and `goto` commands; requests before the
// run, in setup, in both analyze entries (one per task), in verify (one by
// a second, unpriced model) and after the run; a helper launched at 3m
// (the research span) with its own transcript; a launch at 22m (the review
// span) whose transcript was never written; and Claude Code's own totals,
// which exceed the transcripts by opus 100/50 and haiku 20/5.
func goldenSession() *agentusage.Session {
	call := func(cmd string, from, to float64) agentusage.ShellCall {
		return agentusage.ShellCall{Command: cmd, Start: at(from).Add(-100 * time.Millisecond), End: at(to).Add(100 * time.Millisecond)}
	}
	req := func(m float64, model string, in, out, cr, cw int64) agentusage.Request {
		return agentusage.Request{At: at(m), Model: model, Usage: agentusage.Usage{Input: in, Output: out, CacheRead: cr, CacheWrite: cw}}
	}
	const opus, haiku = "claude-opus-5-5", "claude-haiku-5-5"
	return &agentusage.Session{
		ID: "bc997d3b-1caa-4579-9318-a6795dd8b757",
		Transcripts: []agentusage.Transcript{
			{
				ID: "main", File: "/home/u/.claude/projects/-home-u-proj/bc997d3b-1caa-4579-9318-a6795dd8b757.jsonl",
				Start: at(-1), End: at(31),
				Calls: []agentusage.ShellCall{
					call(`spektacular implement new --data '{"name":"000075_run-stats-command"}'`, 0, 0.5),
					call(`spektacular implement goto --data '{"step":"analyze"}'`, 1.5, 2),
					call(`spektacular implement goto --data '{"step":"verify"}'`, 7.5, 8),
					call(`spektacular implement goto --data '{"step":"analyze"}'`, 19.5, 20),
				},
				Requests: []agentusage.Request{
					req(-0.5, opus, 10, 100, 1000, 100),
					req(1.2, opus, 5, 200, 2000, 50),
					req(2.5, opus, 5, 1000, 5000, 100),
					req(7.4, opus, 2, 300, 3000, 0),
					req(9, haiku, 1, 50, 0, 0),
					req(15, opus, 3, 400, 4000, 10),
					req(21, opus, 4, 600, 6000, 20),
					req(30.5, opus, 1, 10, 100, 0),
				},
				Launches: []agentusage.Launch{
					{ToolUseID: "toolu_h1", Description: "research", Prompt: "Research the loader.", At: at(3)},
					{ToolUseID: "toolu_h2", Description: "review", Prompt: "Review the report.", At: at(22)},
				},
			},
			{
				ID: "a1", File: "/home/u/.claude/projects/-home-u-proj/bc997d3b-1caa-4579-9318-a6795dd8b757/subagents/agent-a1.jsonl",
				LaunchID: "toolu_h1", Description: "research", Start: at(3), End: at(6),
				Requests: []agentusage.Request{req(4, opus, 2, 500, 1000, 30)},
			},
		},
		Totals: map[string]agentusage.Usage{
			opus:  {Input: 132, Output: 3160, CacheRead: 22100, CacheWrite: 310},
			haiku: {Input: 21, Output: 55},
		},
	}
}

// goldenOptions are the default options with goldenSession as token input,
// opus priced and haiku not.
func goldenOptions() Options {
	o := DefaultOptions
	o.Tokens = &TokenInput{
		Sessions: []*agentusage.Session{goldenSession()},
		Prices:   Prices{"claude-opus-5-5": {Input: 5, Output: 25, CacheRead: 0.5, CacheWrite: 6.25}},
		Dir:      "/home/u/.claude/projects",
	}
	return o
}

// newFigures are the comparison and summary figures token use adds.
var newFigures = map[string]bool{
	"input_tokens": true, "output_tokens": true, "cache_read_tokens": true, "cache_write_tokens": true,
	"cost": true, "working_ms_per_1k_output": true,
}

// stripTokens removes, in place, every key token use added to a stats
// result: the top-level tokens section, tokens on steps, tasks and helpers
// rows, and the figures appended to comparisons and summaries.
func stripTokens(doc map[string]any) {
	delete(doc, "tokens")
	for _, list := range []string{"steps", "tasks", "helpers"} {
		rows, _ := doc[list].([]any)
		for _, r := range rows {
			if m, ok := r.(map[string]any); ok {
				delete(m, "tokens")
				dropNewFigures(m)
			}
		}
	}
	dropNewFigures(doc)
}

// dropNewFigures removes the new figures from m's figures or totals list.
func dropNewFigures(m map[string]any) {
	for _, key := range []string{"figures", "totals"} {
		list, ok := m[key].([]any)
		if !ok {
			continue
		}
		kept := []any{}
		for _, f := range list {
			if fm, ok := f.(map[string]any); ok && newFigures[fm["figure"].(string)] {
				continue
			}
			kept = append(kept, f)
		}
		m[key] = kept
	}
}

// canonical decodes JSON and re-encodes it with sorted keys.
func canonical(t *testing.T, data []byte, strip bool) string {
	t.Helper()
	var doc any
	require.NoError(t, json.Unmarshal(data, &doc))
	if m, ok := doc.(map[string]any); ok && strip {
		stripTokens(m)
	}
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return string(out)
}

// TestTokenFieldsAreAdditive proves token use only adds to the JSON
// contract of plan 000075 (and 000076's listings): with the new keys and
// figures removed, every regenerated golden file equals its copy taken
// before token use was added.
func TestTokenFieldsAreAdditive(t *testing.T) {
	for _, name := range []string{"report.golden.json", "compare.golden.json", "summary.golden.json", "commands.golden.json", "spans.golden.json"} {
		t.Run(name, func(t *testing.T) {
			now, err := os.ReadFile(filepath.Join("testdata", name))
			require.NoError(t, err)
			before, err := os.ReadFile(filepath.Join("testdata", "pre-tokens", name))
			require.NoError(t, err)
			require.Equal(t, canonical(t, before, false), canonical(t, now, true))
		})
	}
}

// TestReportWithoutTokenInput: a report built without session data, as
// 000077's viewer builds one, gives unknown token use with not_read and
// null row figures, and every other figure equals the report built with
// token input.
func TestReportWithoutTokenInput(t *testing.T) {
	without := reportOf(t, Options{Gaps: 10, GapThreshold: 5 * time.Minute}, goldenRun())
	with := reportOf(t, goldenOptions(), goldenRun())

	require.Equal(t, TokensUnknown, without.Tokens.Status)
	require.Equal(t, str(ReasonNotRead), without.Tokens.ReasonCode)
	require.Nil(t, without.Tokens.Total)
	for _, s := range without.Steps {
		require.Nil(t, s.Tokens, s.Step)
	}
	for _, k := range without.Tasks {
		require.Nil(t, k.Tokens, k.Name)
	}
	for _, h := range without.Helpers {
		require.Nil(t, h.Tokens, h.Name)
	}

	require.Equal(t, TokensKnown, with.Tokens.Status)
	require.Equal(t, without.Totals, with.Totals)
	require.Equal(t, without.StepEntries, with.StepEntries)
	require.Equal(t, without.UserWaits, with.UserWaits)
	require.Equal(t, without.Gaps, with.Gaps)

	a, b := canonicalReport(t, without), canonicalReport(t, with)
	require.Equal(t, a, b, "with token use removed, the two reports are identical")
}

func canonicalReport(t *testing.T, r Report) string {
	t.Helper()
	data, err := json.Marshal(r)
	require.NoError(t, err)
	return canonical(t, data, true)
}

// TestReportGivesTokenFigures checks the golden report's token figures
// against hand totals of goldenSession.
func TestReportGivesTokenFigures(t *testing.T) {
	rep := reportOf(t, goldenOptions(), goldenRun())
	tk := rep.Tokens
	require.Equal(t, TokensKnown, tk.Status)
	require.Equal(t, []SessionRef{{
		SessionID: "bc997d3b-1caa-4579-9318-a6795dd8b757", Transcript: "main",
		File: "/home/u/.claude/projects/-home-u-proj/bc997d3b-1caa-4579-9318-a6795dd8b757.jsonl",
	}}, tk.Sessions)

	// The run total is Claude Code's own totals for the session.
	require.Equal(t, int64(153), tk.Total.Input)
	require.Equal(t, int64(3215), tk.Total.Output)
	require.Equal(t, int64(22100), tk.Total.CacheRead)
	require.Equal(t, int64(310), tk.Total.CacheWrite)
	require.False(t, tk.Total.CostComplete)
	require.Equal(t, []string{"claude-haiku-5-5"}, tk.Total.UnpricedModels)
	// opus: 132*5 + 3160*25 + 22100*0.5 + 310*6.25 = 92647.5 per million,
	// rounded to 6 decimals.
	require.Equal(t, f64(0.092648), tk.Total.Cost)

	require.Equal(t, int64(120), tk.SessionOverhead.Input)
	require.Equal(t, int64(55), tk.SessionOverhead.Output)
	require.Equal(t, int64(165), tk.OutsideSteps.Output, "before and after the run, and the overhead")

	out := map[string]int64{}
	var sum int64
	for _, s := range rep.Steps {
		out[s.Step] = s.Tokens.Output
		sum += s.Tokens.Output
	}
	require.Equal(t, map[string]int64{"setup": 200, "analyze": 2400, "verify": 450, "finished": 0}, out)
	require.Equal(t, tk.Total.Output, sum+tk.OutsideSteps.Output, "steps plus outside equal the total")

	// Tasks: Trace log package holds the first analyze entry, its helper
	// and verify; Report builder the second analyze entry.
	require.Equal(t, int64(1000+300+500+50+400), rep.Tasks[0].Tokens.Output)
	require.Equal(t, int64(600), rep.Tasks[1].Tokens.Output)

	// The research helper has its transcript's tokens and its own pace:
	// 3m over 500 output tokens.
	require.Equal(t, "research", rep.Helpers[0].Name)
	require.Equal(t, int64(500), rep.Helpers[0].Tokens.Output)
	require.Equal(t, i64(360000), rep.Helpers[0].Tokens.WorkingMSPer1kOutput)
	require.Nil(t, rep.Helpers[1].Tokens, "the review launch has no transcript")

	require.Equal(t, []ModelTokens{
		{Model: "claude-haiku-5-5", Input: 21, Output: 55},
		{Model: "claude-opus-5-5", Input: 132, Output: 3160, CacheRead: 22100, CacheWrite: 310, Cost: f64(0.092648)},
	}, tk.Models)
}

func TestRenderReportShowsTokenFigures(t *testing.T) {
	rep := reportOf(t, goldenOptions(), goldenRun())
	var b bytes.Buffer
	require.NoError(t, RenderReport(&b, rep, 0))
	out := b.String()

	head := lineStarting(t, out, "STEP      ENTRIES")
	for _, col := range []string{"OUT TOKENS", "COST", "TIME/1K OUT"} {
		require.Contains(t, head, col)
	}
	// analyze: 2,400 output tokens, opus only; its pace is 16m (no user
	// wait) per 2.4k output tokens.
	analyze := lineStarting(t, out, "analyze  ")
	require.Contains(t, analyze, "2,400")
	require.Contains(t, analyze, fmtMS(16*60*1000*1000/2400))
	// verify used the unpriced haiku: its cost is marked incomplete.
	require.Contains(t, lineStarting(t, out, "verify  "), "*")
	require.Contains(t, lineStarting(t, out, "finished  "), "n/a")
	require.Contains(t, lineWith(t, out, "research  analyze"), "500")

	require.Contains(t, out, "\nTokens\nRead from session bc997d3b (main)\n")
	require.Contains(t, lineStarting(t, out, "(outside any step)"), "165")
	require.Contains(t, lineWith(t, out, "of which session overhead"), "120")
	total := lineStarting(t, out, "TOTAL")
	require.Contains(t, total, "3,215")
	require.Contains(t, total, "22,100")
	require.Contains(t, total, "0.09*")
	require.Contains(t, out, "\nModels\n")
	require.Contains(t, lineStarting(t, out, "claude-haiku-5-5"), "?")
	require.Contains(t, out, "* after a cost: incomplete, no price for claude-haiku-5-5 (prices per million tokens go under stats.prices in .spektacular/config.yaml).")
}

func TestRenderReportUnknownTokens(t *testing.T) {
	rep := reportOf(t, DefaultOptions, goldenRun())
	var b bytes.Buffer
	require.NoError(t, RenderReport(&b, rep, 0))
	out := b.String()
	require.Contains(t, out, "\nTokens\nunknown (not_read): the report was built without reading agent sessions\n")
	require.NotContains(t, out, "\nModels\n")
	fields := strings.Fields(lineStarting(t, out, "analyze  "))
	require.Equal(t, []string{"?", "?", "?"}, fields[len(fields)-3:])
	// Every timing is the same as with token use known.
	require.Contains(t, lineStarting(t, out, "analyze  "), fmtMS(16*60*1000))
}

func TestRenderReportListsUnmatchedHelpers(t *testing.T) {
	rep := reportOf(t, goldenOptions(), goldenRun())
	rep.Tokens.UnmatchedHelpers = []UnmatchedHelper{{
		Name: "stray look", Step: str("verify"), StartedAt: at(12), EndedAt: at(13), DurationMS: 60000,
		Tokens: &TokenFigure{Output: 1200, Cost: f64(0.03), CostComplete: true, UnpricedModels: []string{}, WorkingMSPer1kOutput: i64(50000)},
	}}
	var b bytes.Buffer
	require.NoError(t, RenderReport(&b, rep, 0))
	out := b.String()
	require.Contains(t, out, "\nUnmatched helpers\n")
	line := lineStarting(t, out, "stray look")
	for _, want := range []string{"verify", "1m00s", "1,200", "0.03", "50.0s"} {
		require.Contains(t, line, want)
	}
}

func TestFmtTokens(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 641466: "641,466", 158466888: "158,466,888", -1200: "-1,200"} {
		require.Equal(t, want, fmtTokens(in), "%d", in)
	}
}

func TestFmtCostAndPace(t *testing.T) {
	require.Equal(t, "?", fmtCost(nil))
	require.Equal(t, "?", fmtCost(&TokenFigure{}))
	require.Equal(t, "43.18", fmtCost(&TokenFigure{Cost: f64(43.1799), CostComplete: true}))
	require.Equal(t, "43.18*", fmtCost(&TokenFigure{Cost: f64(43.18)}))
	require.Equal(t, "n/a", fmtPace(nil))
	require.Equal(t, "2.8s", fmtPace(i64(2811)))
}
