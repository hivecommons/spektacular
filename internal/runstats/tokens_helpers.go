package runstats

import (
	"regexp"
	"sort"
	"time"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/hivecommons/spektacular/internal/tracelog"
)

// spanRef finds composite span ids, <trace>-<span>, in a helper's brief.
var spanRef = regexp.MustCompile(`\b([0-9a-f]{32})-([0-9a-f]{16})\b`)

// ownedLaunch is a helper launch the run owns, with the helper transcripts
// it started (its own and every descendant's).
type ownedLaunch struct {
	launch   agentusage.Launch
	helper   *agentusage.Transcript // the helper it started
	usage    usageByModel
	counted  []*agentusage.Transcript
	sequence int // launch order, to break ties
}

// helperTree is a helper transcript and every helper it started, through
// their parent links, stopping at a helper that drove a run of its own:
// that one counts in its own run, never as a helper.
func helperTree(s *agentusage.Session, root *agentusage.Transcript, in *TokenInput, run *Run) []*agentusage.Transcript {
	tree := []*agentusage.Transcript{root}
	for i := 0; i < len(tree); i++ {
		parent := tree[i]
		for j := range s.Transcripts {
			t := &s.Transcripts[j]
			if t.ID == "main" || t.ParentID != parent.ID || in.isDriver(t, run) {
				continue
			}
			tree = append(tree, t)
		}
	}
	return tree
}

// launches are the helper launches of the driver transcripts that the run
// owns, each with the helper transcript it started, in launch order. A
// launch with no transcript (still starting, or never written) is left
// out, as is a helper transcript that drove a run of its own.
func launches(run *Run, ds []driverRef, in *TokenInput) []*ownedLaunch {
	var out []*ownedLaunch
	for _, d := range ds {
		dt := d.transcript()
		for _, l := range dt.Launches {
			if !in.owns(run, d, l.At) {
				continue
			}
			var helper *agentusage.Transcript
			for j := range d.session.Transcripts {
				t := &d.session.Transcripts[j]
				if t.ID != "main" && t.LaunchID != "" && t.LaunchID == l.ToolUseID {
					helper = t
					break
				}
			}
			if helper == nil || in.isDriver(helper, run) {
				continue
			}
			ol := &ownedLaunch{launch: l, helper: helper, usage: usageByModel{}, sequence: len(out)}
			for _, t := range helperTree(d.session, helper, in, run) {
				ol.counted = append(ol.counted, t)
				for _, r := range t.Requests {
					ol.usage.add(r.Model, r.Usage)
				}
			}
			out = append(out, ol)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].launch.At.Equal(out[j].launch.At) {
			return out[i].launch.At.Before(out[j].launch.At)
		}
		return out[i].sequence < out[j].sequence
	})
	return out
}

// matchHelpers pairs owned launches with the run's sub-agent spans: first
// by the span id the helper's brief names; then, for the launches left, in
// launch order to the latest unmatched spans that started at or before the
// launch (a response's concurrent launches take those spans in their start
// order). It returns the span id each launch matched, "" for none.
func matchHelpers(run *Run, ls []*ownedLaunch) []string {
	var spans []Span
	for _, s := range run.Spans {
		if s.Type == tracelog.TypeSubAgent {
			spans = append(spans, s)
		}
	}
	matched := make([]string, len(ls))
	used := map[string]bool{}
	isSpan := map[string]bool{}
	for _, s := range spans {
		isSpan[s.ID] = true
	}

	for i, l := range ls {
		for _, m := range spanRef.FindAllStringSubmatch(l.launch.Prompt, -1) {
			if m[1] == run.ID && isSpan[m[2]] && !used[m[2]] {
				matched[i] = m[2]
				used[m[2]] = true
				break
			}
		}
	}

	for i := 0; i < len(ls); {
		// A group: the launches made at the same moment (one response).
		j := i
		for j < len(ls) && ls[j].launch.At.Equal(ls[i].launch.At) {
			j++
		}
		var group []int
		for k := i; k < j; k++ {
			if matched[k] == "" {
				group = append(group, k)
			}
		}
		if len(group) > 0 {
			var cands []Span
			limit := ls[i].launch.At.Add(matchTolerance)
			for _, s := range spans {
				if !used[s.ID] && !s.Start.After(limit) {
					cands = append(cands, s)
				}
			}
			if len(cands) > len(group) {
				cands = cands[len(cands)-len(group):]
			}
			// The earliest launches of the group without a span go
			// unmatched when there are fewer spans than launches.
			offset := len(group) - len(cands)
			for c, s := range cands {
				matched[group[offset+c]] = s.ID
				used[s.ID] = true
			}
		}
		i = j
	}
	return matched
}

// placeHelpers puts each owned helper launch's tokens on its span and in
// the step and task of its helper row; a launch that matches no span is
// placed in the step entry and task running when it was made, or outside
// any step, and listed as unmatched. It returns the unmatched helpers, the
// figures still to be built, and every helper transcript counted.
func placeHelpers(run *Run, rep *Report, loc locator, ls []*ownedLaunch, p *placement) (unmatched []unmatchedLaunch) {
	rows := map[string]Wait{}
	for _, h := range rep.Helpers {
		rows[h.SpanID] = h.Wait
	}
	matched := matchHelpers(run, ls)
	for i, l := range ls {
		if id := matched[i]; id != "" {
			row := rows[id]
			var task *string
			if row.TaskID != nil {
				task = row.TaskID
			}
			for m, u := range l.usage {
				p.put(loc.step(row.Step), task, &id, m, u)
			}
			continue
		}
		step, task := loc.step(loc.stepAt(l.launch.At)), loc.taskAt(l.launch.At)
		for m, u := range l.usage {
			p.put(step, task, nil, m, u)
		}
		name := l.helper.Description
		if name == "" {
			name = l.launch.Description
		}
		unmatched = append(unmatched, unmatchedLaunch{
			name: name, step: step, task: task, start: l.helper.Start, end: l.helper.End, usage: l.usage,
		})
	}
	return unmatched
}

// unmatchedLaunch is a helper launch no span matched, before its figure is
// built.
type unmatchedLaunch struct {
	name       string
	step, task *string
	start, end time.Time
	usage      usageByModel
}
