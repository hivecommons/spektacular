package meeting

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden intent streams under testdata/replay")

// A replay fixture (testdata/replay/<name>.jsonl) is a meeting on a fake
// clock. Each line is one of:
//
//	{"options": {"pause_ms": 1500, "heartbeat_s": 15, "max_asks": 3, "offer_ttl_s": 120, "names": "hive,spek"}}
//	{"at": <ms>, "event": {...}}   a stdin line, written as hive would write it
//	{"at": <ms>, "raw": "..."}     a stdin line sent verbatim (malformed input)
//	{"at": <ms>, "eof": true}      stdin closes (default: right after the last line)
//	{"llm": {"kind": "turn", "trigger": "pause", "when": "<substring of the latest line>", "once": true, "response": {...}}}
//
// llm lines script the fake model: the first rule whose kind, trigger and
// when all match a decision answers it (once rules answer one decision
// only); no match is an empty response. The harness steps the clock by
// TickInterval, delivering due lines then ticking, exactly as Run does, and
// takes decisions synchronously. The golden file
// (testdata/replay/<name>.golden.jsonl) records every decision taken and
// every intent emitted, with the fake time it happened at.

type fixtureLine struct {
	Options *struct {
		PauseMs    int    `json:"pause_ms"`
		HeartbeatS int    `json:"heartbeat_s"`
		MaxAsks    int    `json:"max_asks"`
		OfferTTLS  int    `json:"offer_ttl_s"`
		Names      string `json:"names"`
	} `json:"options"`
	At    *int64          `json:"at"`
	Event json.RawMessage `json:"event"`
	Raw   *string         `json:"raw"`
	EOF   bool            `json:"eof"`
	LLM   *llmRule        `json:"llm"`
}

type llmRule struct {
	Kind     string   `json:"kind"`
	Trigger  string   `json:"trigger"`
	When     string   `json:"when"`
	Once     bool     `json:"once"`
	Response Response `json:"response"`
	used     bool
}

type scheduled struct {
	at   int64
	line []byte
}

type replay struct {
	settings Settings
	lines    []scheduled
	eofAt    int64
	rules    []*llmRule
}

func loadReplay(t *testing.T, path string) replay {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	r := replay{settings: DefaultSettings(), eofAt: -1}
	// Fixtures keep heartbeats out of the way unless they set them.
	r.settings.Heartbeat = time.Hour
	for i, raw := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "//") {
			continue
		}
		var fl fixtureLine
		require.NoError(t, json.Unmarshal([]byte(raw), &fl), "%s:%d", path, i+1)
		switch {
		case fl.Options != nil:
			o := fl.Options
			if o.PauseMs > 0 {
				r.settings.Pause = time.Duration(o.PauseMs) * time.Millisecond
			}
			if o.HeartbeatS > 0 {
				r.settings.Heartbeat = time.Duration(o.HeartbeatS) * time.Second
			}
			if o.MaxAsks > 0 {
				r.settings.MaxAsksPer10m = o.MaxAsks
			}
			if o.OfferTTLS > 0 {
				r.settings.OfferTTL = time.Duration(o.OfferTTLS) * time.Second
			}
			if o.Names != "" {
				r.settings.Names = splitNames(o.Names)
			}
		case fl.LLM != nil:
			r.rules = append(r.rules, fl.LLM)
		case fl.At != nil && fl.EOF:
			r.eofAt = *fl.At
		case fl.At != nil && fl.Raw != nil:
			r.lines = append(r.lines, scheduled{at: *fl.At, line: []byte(*fl.Raw)})
		case fl.At != nil && len(fl.Event) > 0:
			r.lines = append(r.lines, scheduled{at: *fl.At, line: []byte(fl.Event)})
		default:
			t.Fatalf("%s:%d: unrecognised fixture line", path, i+1)
		}
	}
	sort.SliceStable(r.lines, func(i, j int) bool { return r.lines[i].at < r.lines[j].at })
	if r.eofAt < 0 && len(r.lines) > 0 {
		r.eofAt = r.lines[len(r.lines)-1].at
	}
	return r
}

func (r *replay) decide(rec *[]string, clock *int64) Decider {
	return DeciderFunc(func(_ context.Context, req Request) (Response, error) {
		last := ""
		if n := len(req.Window); n > 0 {
			last = req.Window[n-1].Text
		}
		call, _ := json.Marshal(map[string]any{
			"kind": req.Kind, "trigger": req.Trigger, "ask_allowed": req.AskAllowed, "say_allowed": req.SayAllowed, "last": last,
		})
		*rec = append(*rec, fmt.Sprintf(`{"at":%d,"decide":%s}`, *clock, call))
		for _, rule := range r.rules {
			if rule.used || (rule.Kind != "" && rule.Kind != req.Kind) || (rule.Trigger != "" && rule.Trigger != req.Trigger) {
				continue
			}
			if rule.When != "" && !strings.Contains(strings.ToLower(last), strings.ToLower(rule.When)) {
				continue
			}
			if rule.Once {
				rule.used = true
			}
			return rule.Response, nil
		}
		return Response{}, nil
	})
}

// runReplay replays a fixture and returns the recorded stream, one JSON
// object per line.
func runReplay(t *testing.T, r replay, stderr *bytes.Buffer) []string {
	t.Helper()
	var out bytes.Buffer
	var rec []string
	var clock int64
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	at := func(ms int64) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }
	s := &Session{
		Engine:   NewEngine(Options{Settings: r.settings, Title: "Replay", Log: stderr}),
		Encoder:  NewEncoder(&out, stderr),
		Decider:  r.decide(&rec, &clock),
		Settings: r.settings,
	}
	collect := func() {
		for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
			if l != "" {
				rec = append(rec, fmt.Sprintf(`{"at":%d,"intent":%s}`, clock, l))
			}
		}
		out.Reset()
	}
	require.NoError(t, s.Start(at(0)))
	collect()
	step := TickInterval.Milliseconds()
	next := 0
	for clock = 0; ; clock += step {
		for next < len(r.lines) && r.lines[next].at <= clock {
			require.NoError(t, s.Line(r.lines[next].line, at(clock)))
			next++
			collect()
			if s.Engine.Ended() {
				break
			}
		}
		if s.Engine.Ended() || (next == len(r.lines) && clock >= r.eofAt) {
			break
		}
		require.NoError(t, s.Tick(at(clock)))
		collect()
	}
	require.NoError(t, s.Close(at(clock)))
	collect()
	return rec
}

func TestReplayGolden(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "replay", "*.jsonl"))
	require.NoError(t, err)
	var inputs []string
	for _, f := range fixtures {
		if !strings.HasSuffix(f, ".golden.jsonl") {
			inputs = append(inputs, f)
		}
	}
	require.NotEmpty(t, inputs)
	for _, f := range inputs {
		name := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			got := runReplay(t, loadReplay(t, f), &stderr)
			golden := strings.TrimSuffix(f, ".jsonl") + ".golden.jsonl"
			body := strings.Join(got, "\n") + "\n"
			if *update {
				require.NoError(t, os.WriteFile(golden, []byte(body), 0o644))
			}
			want, err := os.ReadFile(golden)
			require.NoError(t, err, "missing golden; run go test ./internal/meeting -run TestReplayGolden -update")
			require.Equal(t, string(want), body, "stderr:\n%s", stderr.String())
			for _, l := range got {
				assertValidRecord(t, l)
			}
		})
	}
}

// assertValidRecord checks every recorded intent is a valid contract line.
func assertValidRecord(t *testing.T, rec string) {
	t.Helper()
	var r struct {
		Intent json.RawMessage `json:"intent"`
	}
	require.NoError(t, json.Unmarshal([]byte(rec), &r))
	if len(r.Intent) == 0 {
		return
	}
	var in struct {
		Type, Text, QID, RID, Query, Reason, Section string
	}
	require.NoError(t, json.Unmarshal(r.Intent, &in))
	require.NoError(t, Intent{Type: in.Type, Text: in.Text, QID: in.QID, RID: in.RID, Query: in.Query, Reason: in.Reason, Section: in.Section}.Validate(), rec)
}

// intentsOf returns the intents in a recorded stream, with their times.
func intentsOf(t *testing.T, rec []string) []struct {
	At     int64
	Intent map[string]string
} {
	t.Helper()
	var out []struct {
		At     int64
		Intent map[string]string
	}
	for _, l := range rec {
		var r struct {
			At     int64             `json:"at"`
			Intent map[string]string `json:"intent"`
		}
		require.NoError(t, json.Unmarshal([]byte(l), &r))
		if r.Intent != nil {
			out = append(out, struct {
				At     int64
				Intent map[string]string
			}{r.At, r.Intent})
		}
	}
	return out
}

func replayFixture(t *testing.T, name string) []string {
	t.Helper()
	var stderr bytes.Buffer
	return runReplay(t, loadReplay(t, filepath.Join("testdata", "replay", name+".jsonl")), &stderr)
}

func countType(t *testing.T, rec []string, typ string) int {
	n := 0
	for _, in := range intentsOf(t, rec) {
		if in.Intent["type"] == typ {
			n++
		}
	}
	return n
}

// The properties below restate what the goldens show, so a regenerated
// golden that breaks a rule still fails.

func TestReplay_QuestionsOnlyAtPauses(t *testing.T) {
	rec := replayFixture(t, "pause_questions")
	for _, l := range rec {
		var r struct {
			Decide *struct {
				Trigger    string `json:"trigger"`
				AskAllowed bool   `json:"ask_allowed"`
			} `json:"decide"`
		}
		require.NoError(t, json.Unmarshal([]byte(l), &r))
		if r.Decide != nil && r.Decide.Trigger != TriggerPause {
			require.False(t, r.Decide.AskAllowed, "ask allowed outside a pause: %s", l)
		}
	}
	asks := 0
	for _, in := range intentsOf(t, rec) {
		if in.Intent["type"] == IntentAsk {
			asks++
			// The fixture's speech stops at 1000 ms; the pause is 1500 ms.
			require.GreaterOrEqual(t, in.At, int64(2500))
		}
	}
	require.Equal(t, 1, asks)
}

func TestReplay_RateLimit(t *testing.T) {
	rec := replayFixture(t, "rate_limit")
	var times []int64
	for _, in := range intentsOf(t, rec) {
		if in.Intent["type"] == IntentAsk {
			times = append(times, in.At)
		}
	}
	for i := range times {
		n := 0
		for j := range times {
			if times[j] <= times[i] && times[i]-times[j] < AskWindow.Milliseconds() {
				n++
			}
		}
		require.LessOrEqual(t, n, DefaultMaxAsksPer10m, "asks at %v", times)
	}
	require.Len(t, times, 4, "three asks, then one more once the first leaves the window")
}

func TestReplay_Dedupe(t *testing.T) {
	require.Equal(t, 2, countType(t, replayFixture(t, "dedupe"), IntentAsk))
}

func TestReplay_ExplicitVersusImplicitResearch(t *testing.T) {
	rec := replayFixture(t, "research")
	var queries []string
	for _, in := range intentsOf(t, rec) {
		if in.Intent["type"] == IntentResearch {
			queries = append(queries, in.Intent["query"])
		}
	}
	require.Equal(t, []string{
		"Postgres logical replication slot limits",
		"the pricing of the large runners",
		"whether the vendor supports SAML",
		"the flaky arm64 builds",
	}, queries)
}

func TestReplay_SayOnlyWhenAddressed(t *testing.T) {
	rec := replayFixture(t, "say_addressed")
	require.Equal(t, 1, countType(t, rec, IntentSay))
}

func TestReplay_SummaryOnEnd(t *testing.T) {
	rec := replayFixture(t, "summary_end")
	ins := intentsOf(t, rec)
	last := ins[len(ins)-1].Intent
	require.Equal(t, IntentNotesAppend, last["type"])
	require.Equal(t, SectionSummary, last["section"])
	require.Contains(t, last["text"], "Decisions:\n- Ship the meeting mode behind a flag")
	require.Contains(t, last["text"], "Actions:\n- Ana: write the contract tests")
	require.Contains(t, last["text"], "Open questions:\n- Who owns the hive side rollout?")
	require.Contains(t, last["text"], "Answered:\n- Which hive version ships it first? v5.3 (Bo)")
}

func TestReplay_MalformedInput(t *testing.T) {
	var stderr bytes.Buffer
	rec := runReplay(t, loadReplay(t, filepath.Join("testdata", "replay", "malformed.jsonl")), &stderr)
	require.Equal(t, 1, countType(t, rec, IntentNotesAppend))
	require.Contains(t, stderr.String(), "skipping malformed input line")
	require.Contains(t, stderr.String(), "skipping unknown event type")
	require.Contains(t, stderr.String(), "skipping non-final segment")
}

func TestReplay_HeartbeatTiming(t *testing.T) {
	rec := replayFixture(t, "heartbeat")
	var beats []int64
	for _, in := range intentsOf(t, rec) {
		if in.Intent["type"] == IntentHeartbeat {
			beats = append(beats, in.At)
		}
	}
	require.Equal(t, []int64{0, 15000, 30000, 45000, 60000}, beats)
}
