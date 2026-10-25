package runstats

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// The text renderers draw the same figures as the JSON, as plain tables for
// people: no colour, times in UTC, run ids cut to their first 8 characters,
// and a value that does not apply printed as "-".

// timeLayout is how the tables print a moment, always in UTC.
const timeLayout = "2006-01-02 15:04:05"

// unfinishedMark follows the end reason of an item that did not finish.
const unfinishedMark = "*"

// RenderList draws the runs as one table, newest first as given.
func RenderList(w io.Writer, l *Logs, runs []*Run) error {
	b := &strings.Builder{}
	if len(runs) == 0 {
		fmt.Fprintln(b, "No recorded runs.")
	} else {
		tw := table(b)
		fmt.Fprintln(tw, "RUN\tKIND\tNAME\tAGENT\tSTARTED\tWALL\tSTATUS")
		for _, r := range runs {
			s := r.Summary()
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				shortID(s.RunID), s.Kind, s.Name, s.Agent, s.StartedAt.Format(timeLayout), fmtMS(s.WallMS), s.Status)
		}
		tw.Flush()
	}
	skippedLine(b, l.Skipped)
	_, err := io.WriteString(w, b.String())
	return err
}

// RenderReport draws one run's report: a header, the totals, then the
// steps, tasks (when there are any), helper launches, waits on the user and
// the longest gaps.
func RenderReport(w io.Writer, rep Report, skipped int) error {
	b := &strings.Builder{}
	r := rep.Run
	fmt.Fprintf(b, "%s %s  agent %s  run %s  %s  started %s\n",
		r.Kind, r.Name, r.Agent, shortID(r.RunID), r.Status, r.StartedAt.Format(timeLayout))

	t := rep.Totals
	fmt.Fprintln(b)
	tw := table(b)
	fmt.Fprintf(tw, "Wall time\t%s\n", fmtMS(t.WallMS))
	fmt.Fprintf(tw, "Working time\t%s\n", fmtMS(t.WorkingMS))
	fmt.Fprintf(tw, "User wait\t%s\t(%s)\n", fmtMS(t.UserWaitMS), count(t.UserWaits, "wait"))
	fmt.Fprintf(tw, "Helper wait\t%s\t(%s)\n", fmtMS(t.HelperWaitMS), count(t.Helpers, "helper"))
	fmt.Fprintf(tw, "Command time\t%s\t(%s)\n", fmtMS(t.CommandMS), count(t.Commands, "command"))
	fmt.Fprintf(tw, "Gaps over %s\t%s\t(%s)\n", fmtMS(rep.GapThresholdMS), fmtMS(t.GapMS), count(t.GapsOverThreshold, "gap"))
	tw.Flush()

	unfinished := false
	mark := func(reason string, complete bool) string {
		if complete {
			return reason
		}
		unfinished = true
		return reason + unfinishedMark
	}

	section(b, "Steps")
	if len(rep.Steps) == 0 {
		fmt.Fprintln(b, "none recorded")
	} else {
		tw = table(b)
		fmt.Fprintln(tw, "STEP\tENTRIES\tDURATION\tCOMMANDS\tCMD TIME\tHELPER WAIT\tUSER WAIT\tINCOMPLETE")
		for _, s := range rep.Steps {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%s\t%s\t%s\t%d\n", s.Step, s.Entries, fmtMS(s.DurationMS),
				s.Commands, fmtMS(s.CommandMS), fmtMS(s.HelperWaitMS), fmtMS(s.UserWaitMS), s.Incomplete)
		}
		tw.Flush()
	}

	if len(rep.Tasks) > 0 {
		section(b, "Tasks")
		tw = table(b)
		fmt.Fprintln(tw, "TASK\tSTARTED\tDURATION\tHELPER WAIT\tUSER WAIT\tEND")
		for _, k := range rep.Tasks {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", k.Name, k.StartedAt.Format(timeLayout), fmtMS(k.DurationMS),
				fmtMS(k.HelperWaitMS), fmtMS(k.UserWaitMS), mark(k.EndReason, k.Complete))
		}
		tw.Flush()
	}

	waits := func(title string, ws []Wait) {
		section(b, title)
		if len(ws) == 0 {
			fmt.Fprintln(b, "none")
			return
		}
		tw := table(b)
		fmt.Fprintln(tw, "NAME\tSTEP\tSTARTED\tDURATION\tEND")
		for _, x := range ws {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", x.Name, orDash(x.Step), x.StartedAt.Format(timeLayout),
				fmtMS(x.DurationMS), mark(x.EndReason, x.Complete))
		}
		tw.Flush()
	}
	waits("Helpers", rep.Helpers)
	waits("User waits", rep.UserWaits)

	section(b, "Longest gaps")
	if len(rep.Gaps) == 0 {
		fmt.Fprintln(b, "none")
	} else {
		tw = table(b)
		fmt.Fprintln(tw, "DURATION\tSTEP\tSTARTED\tAFTER\tBEFORE")
		for _, g := range rep.Gaps {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", fmtMS(g.DurationMS), orDash(g.Step), g.StartedAt.Format(timeLayout),
				edge(g.After), edge(g.Before))
		}
		tw.Flush()
	}

	if unfinished {
		fmt.Fprintf(b, "\n%s did not finish: counted up to where it was cut off, and left out of every wait total.\n", unfinishedMark)
	}
	skippedLine(b, skipped)
	_, err := io.WriteString(w, b.String())
	return err
}

func table(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0) }

func section(b *strings.Builder, title string) { fmt.Fprintf(b, "\n%s\n", title) }

func skippedLine(b *strings.Builder, n int) {
	if n > 0 {
		fmt.Fprintf(b, "\nSkipped lines: %d\n", n)
	}
}

// edge names an event beside a gap, as "end command plan goto".
func edge(e GapEdge) string { return e.Event + " " + e.Type + " " + e.Name }

// count is n and noun, plural unless n is 1.
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func orDash(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

// fmtMS prints a duration in milliseconds: under a minute as 12.3s,
// otherwise as 4m05s or 1h02m03s.
func fmtMS(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	d := time.Duration(v) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%s%.1fs", sign, d.Seconds())
	}
	h := int64(d / time.Hour)
	m := int64(d % time.Hour / time.Minute)
	s := int64(d % time.Minute / time.Second)
	if h > 0 {
		return fmt.Sprintf("%s%dh%02dm%02ds", sign, h, m, s)
	}
	return fmt.Sprintf("%s%dm%02ds", sign, m, s)
}
