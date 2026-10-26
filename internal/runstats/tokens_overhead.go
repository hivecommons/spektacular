package runstats

import "github.com/hivecommons/spektacular/internal/agentusage"

// overhead is the tokens Claude Code counted for the driver sessions that
// their transcripts never record (permission checks, titles and other side
// requests): each session's own totals less every request in its
// transcripts, by model. It is known only when every transcript of every
// driver session belongs to this run (counted holds those placed here) and
// each session has its totals; otherwise it is nil with the reason.
func overhead(run *Run, ds []driverRef, counted map[*agentusage.Transcript]bool, in *TokenInput) (usageByModel, string) {
	sessions := []*agentusage.Session{}
	seen := map[*agentusage.Session]bool{}
	for _, d := range ds {
		if !seen[d.session] {
			seen[d.session] = true
			sessions = append(sessions, d.session)
		}
	}

	out := usageByModel{}
	for _, s := range sessions {
		for i := range s.Transcripts {
			t := &s.Transcripts[i]
			// A transcript that drove runs must have driven only this one;
			// any other must be a helper counted here.
			if rs := in.runsDrivenBy(t, run); len(rs) > 0 {
				if len(rs) == 1 && rs[0].ID == run.ID {
					continue
				}
				return nil, OverheadSeveralRuns
			}
			if !counted[t] {
				return nil, OverheadSeveralRuns
			}
		}
		if s.Totals == nil {
			return nil, OverheadNoTotals
		}
		transcripts := usageByModel{}
		for _, t := range s.Transcripts {
			for _, r := range t.Requests {
				transcripts.add(r.Model, r.Usage)
			}
		}
		for m := range transcripts {
			if _, ok := s.Totals[m]; !ok {
				return nil, OverheadBelowTranscript
			}
		}
		for m, total := range s.Totals {
			d := total.Sub(transcripts[m])
			if d.Input < 0 || d.Output < 0 || d.CacheRead < 0 || d.CacheWrite < 0 {
				return nil, OverheadBelowTranscript
			}
			if !d.IsZero() {
				out.add(m, d)
			}
		}
	}
	return out, ""
}
