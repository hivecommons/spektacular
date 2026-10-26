package runstats

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/hivecommons/spektacular/internal/tracelog"
)

// matchTolerance is how far outside a shell call's interval a command
// record may start and still be that call's.
const matchTolerance = time.Second

// driverRef is a transcript that drove a run.
type driverRef struct {
	session *agentusage.Session
	index   int // into session.Transcripts
}

func (d driverRef) transcript() *agentusage.Transcript { return &d.session.Transcripts[d.index] }

// runKey is one of a run's own workflow commands: its `<kind> new` or
// `<kind> goto` record.
type runKey struct {
	words string // "<kind> new" or "<kind> goto"
	at    time.Time
}

// runKeys are the run's own `new` and `goto` command records.
func runKeys(run *Run) []runKey {
	var keys []runKey
	for _, c := range run.Commands {
		args := stringsAttr(c, tracelog.AttrCommandArgs)
		if len(args) < 2 || args[0] != run.Kind || (args[1] != "new" && args[1] != "goto") {
			continue
		}
		keys = append(keys, runKey{words: args[0] + " " + args[1], at: c.StartTime})
	}
	return keys
}

// dataName finds the "name" values in a command's --data JSON, quoted or
// shell-escaped.
var dataName = regexp.MustCompile(`\\?"name\\?"\s*:\s*\\?"([^"\\]*)`)

// callRuns reports whether a shell call ran one of a run's keys: the
// record started within the call's interval, give or take matchTolerance,
// and the call's text, whitespace collapsed, holds the command's first two
// words. A call whose --data names a workflow other than the run's is not
// the run's.
func callRuns(call agentusage.ShellCall, run *Run, keys []runKey) bool {
	end := call.End
	if end.Before(call.Start) {
		end = call.Start
	}
	from, to := call.Start.Add(-matchTolerance), end.Add(matchTolerance)
	text := strings.Join(strings.Fields(call.Command), " ")
	for _, k := range keys {
		if k.at.Before(from) || k.at.After(to) || !strings.Contains(text, k.words) {
			continue
		}
		names := dataName.FindAllStringSubmatch(text, -1)
		if len(names) == 0 {
			return true
		}
		for _, n := range names {
			if n[1] == run.Name {
				return true
			}
		}
	}
	return false
}

// drives reports whether a transcript ran any of run's keys.
func drives(t *agentusage.Transcript, run *Run, keys []runKey) bool {
	if len(keys) == 0 {
		return false
	}
	for _, c := range t.Calls {
		if callRuns(c, run, keys) {
			return true
		}
	}
	return false
}

// drivers are the transcripts that drove run: every transcript, main or
// helper, in any session, that ran one of the run's own `new` or `goto`
// commands. A run resumed in a second session has a driver in each.
func drivers(run *Run, sessions []*agentusage.Session) []driverRef {
	keys := runKeys(run)
	var out []driverRef
	for _, s := range sessions {
		for i := range s.Transcripts {
			if drives(&s.Transcripts[i], run, keys) {
				out = append(out, driverRef{session: s, index: i})
			}
		}
	}
	return out
}

// runsDrivenBy are the runs among in.AllRuns (or just run, when none are
// given) that the transcript drove, in start order. It is memoised per
// transcript.
func (in *TokenInput) runsDrivenBy(t *agentusage.Transcript, run *Run) []*Run {
	if in.driven == nil {
		in.driven = map[*agentusage.Transcript][]*Run{}
	}
	if rs, ok := in.driven[t]; ok {
		return rs
	}
	all := in.AllRuns
	if len(all) == 0 {
		all = []*Run{run}
	}
	var rs []*Run
	for _, r := range all {
		if r.End.Before(t.Start.Add(-matchTolerance)) || r.Start.After(t.End.Add(matchTolerance)) {
			continue
		}
		if drives(t, r, runKeys(r)) {
			rs = append(rs, r)
		}
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if !rs[i].Start.Equal(rs[j].Start) {
			return rs[i].Start.Before(rs[j].Start)
		}
		return rs[i].ID < rs[j].ID
	})
	in.driven[t] = rs
	return rs
}

// isDriver reports whether the transcript drove any run in the logs.
func (in *TokenInput) isDriver(t *agentusage.Transcript, run *Run) bool {
	return len(in.runsDrivenBy(t, run)) > 0
}

// ownerAt is which of the runs a transcript drove (in start order) owns a
// moment in it: the run running then (the latest started, if several);
// before the first run, the first; between runs, the next; after the last,
// the last.
func ownerAt(runs []*Run, at time.Time) *Run {
	if len(runs) == 0 {
		return nil
	}
	var owner *Run
	for _, r := range runs {
		if !at.Before(r.Start) && !at.After(r.End) {
			owner = r // later starts win
		}
	}
	if owner != nil {
		return owner
	}
	for _, r := range runs {
		if at.Before(r.Start) {
			return r
		}
	}
	return runs[len(runs)-1]
}
