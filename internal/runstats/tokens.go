package runstats

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/agentusage"
)

// Token use is attributed after the timing report is built, from the
// report's step entries, task spans and helper rows and the parsed agent
// sessions, and never changes a timing figure. The stages, one file each:
//
//   - match (tokens_match.go): the transcripts that drove the run, found
//     by the run's own `new` and `goto` commands in their shell calls;
//   - place (tokens_place.go): each main-agent request in the step entry
//     and task running when it was made, or outside any step;
//   - helpers (tokens_helpers.go): each helper transcript on its launch's
//     span, step and task, matched by the span id in its brief, then by
//     launch order, else listed as unmatched;
//   - overhead (tokens_overhead.go): Claude Code's own totals less the
//     transcripts' requests, when the session drove only this run;
//   - figures (here): sums by row, cost and time per output token.

// claudeAgents are the run agents whose token use is read from Claude Code
// sessions: Claude Code itself, and a run that names no agent.
var claudeAgents = map[string]bool{"claude": true, "unknown": true}

// buildTokens is the run's token section and its rows' token figures. With
// no input, an unsupported agent, or no session that drove the run, the
// section is unknown with the reason and there are no row figures.
func buildTokens(run *Run, rep *Report, in *TokenInput) (TokenSection, tokenRows) {
	sec := TokenSection{Agent: run.Agent, Sessions: []SessionRef{}, Models: []ModelTokens{}, UnmatchedHelpers: []UnmatchedHelper{}}
	unknown := func(code, reason string) (TokenSection, tokenRows) {
		sec.Status = TokensUnknown
		sec.ReasonCode, sec.Reason = &code, &reason
		return sec, tokenRows{}
	}
	if in == nil {
		return unknown(ReasonNotRead, "the report was built without reading agent sessions")
	}
	if !claudeAgents[run.Agent] {
		return unknown(ReasonAgentUnsupported, fmt.Sprintf("token use is read only for Claude Code sessions; this run's agent is %s", run.Agent))
	}
	if len(in.Sessions) == 0 && len(in.Problems) > 0 {
		return unknown(in.Problems[0].Code, in.Problems[0].Message)
	}
	ds := drivers(run, in.Sessions)
	if len(ds) == 0 {
		msg := fmt.Sprintf("no Claude Code session under %s ran this run's commands (%d sessions searched)", in.Dir, len(in.Sessions))
		for _, p := range in.Problems {
			if p.Code == agentusage.ProblemUnreadable || p.Code == agentusage.ProblemUnrecognised {
				msg += "; " + p.Message
				break
			}
		}
		return unknown(ReasonSessionNotFound, msg)
	}

	sec.Status = TokensKnown
	seenSession := map[*agentusage.Session]bool{}
	for _, d := range ds {
		t := d.transcript()
		sec.Sessions = append(sec.Sessions, SessionRef{SessionID: d.session.ID, Transcript: t.ID, File: t.File})
		if !seenSession[d.session] {
			seenSession[d.session] = true
			sec.SkippedLines += d.session.Skipped
		}
	}

	loc := newLocator(run, rep)
	p := newPlacement()
	placeRequests(run, loc, ds, in, p)
	ls := launches(run, ds, in)
	unmatched := placeHelpers(run, rep, loc, ls, p)

	counted := map[*agentusage.Transcript]bool{}
	for _, d := range ds {
		counted[d.transcript()] = true
	}
	for _, l := range ls {
		for _, t := range l.counted {
			counted[t] = true
		}
	}
	over, reason := overhead(run, ds, counted, in)
	if over != nil {
		p.outside.addAll(over)
		p.all.addAll(over)
		f := figure(over, in.Prices, 0, false)
		sec.SessionOverhead = &f
	} else {
		sec.SessionOverheadReason = &reason
	}

	rows := tokenRows{steps: map[string]*TokenFigure{}, tasks: map[string]*TokenFigure{}, helpers: map[string]*TokenFigure{}}
	for _, s := range rep.Steps {
		f := figure(p.steps[s.Step], in.Prices, max(s.DurationMS-s.UserWaitMS, 0), true)
		rows.steps[s.Step] = &f
	}
	for _, k := range rep.Tasks {
		key := taskKey(k.TaskID, k.SpanID)
		f := figure(p.tasks[key], in.Prices, max(k.DurationMS-k.UserWaitMS, 0), true)
		rows.tasks[key] = &f
	}
	matchedSpans := map[string]bool{}
	for id := range p.helpers {
		matchedSpans[id] = true
	}
	for _, h := range rep.Helpers {
		if !matchedSpans[h.SpanID] {
			continue // a launch with no transcript: unknown
		}
		f := figure(p.helpers[h.SpanID], in.Prices, h.DurationMS, true)
		rows.helpers[h.SpanID] = &f
	}

	total := figure(p.all, in.Prices, rep.Totals.WorkingMS, true)
	outside := figure(p.outside, in.Prices, 0, false)
	sec.Total, sec.OutsideSteps = &total, &outside

	models := make([]string, 0, len(p.all))
	for m := range p.all {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		u := p.all[m]
		mt := ModelTokens{Model: m, Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
		if c, ok := price(in.Prices, m, u); ok {
			c = round6(c)
			mt.Cost = &c
		}
		sec.Models = append(sec.Models, mt)
	}

	for _, u := range unmatched {
		dur := ms(u.end.Sub(u.start))
		f := figure(u.usage, in.Prices, dur, true)
		var task *string
		if u.task != nil && !strings.HasPrefix(*u.task, "span:") {
			task = u.task
		}
		sec.UnmatchedHelpers = append(sec.UnmatchedHelpers, UnmatchedHelper{
			Name: u.name, Step: u.step, TaskID: task,
			StartedAt: u.start.UTC(), EndedAt: u.end.UTC(), DurationMS: dur, Tokens: &f,
		})
	}
	return sec, rows
}

// figure is the token figure of u: its four sums; the cost of its priced
// models (null when it holds tokens and none are priced, 0 when it holds
// none), whether every model was priced and which were not; and, when it
// has a time of its own (timed), workingMS per thousand output tokens.
func figure(u usageByModel, prices Prices, workingMS int64, timed bool) TokenFigure {
	sum := u.sum()
	f := TokenFigure{Input: sum.Input, Output: sum.Output, CacheRead: sum.CacheRead, CacheWrite: sum.CacheWrite, UnpricedModels: []string{}}

	var cost float64
	priced := 0
	for m, x := range u {
		if x.IsZero() {
			continue
		}
		if c, ok := price(prices, m, x); ok {
			cost += c
			priced++
		} else {
			f.UnpricedModels = append(f.UnpricedModels, m)
		}
	}
	sort.Strings(f.UnpricedModels)
	f.CostComplete = len(f.UnpricedModels) == 0
	if priced > 0 || sum.IsZero() {
		c := round6(cost)
		f.Cost = &c
	}

	if timed && sum.Output > 0 {
		v := int64(math.Round(float64(workingMS) * 1000 / float64(sum.Output)))
		f.WorkingMSPer1kOutput = &v
	}
	return f
}
