package runstats

import (
	"fmt"
	"io"
	"math"
	"strconv"
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

// RenderComparison draws a comparison: the runs, each step's duration per
// run with the difference from the first run, and every total the same way.
// A step a run never entered shows as "-".
func RenderComparison(w io.Writer, c Comparison, skipped int) error {
	if len(c.Runs) == 0 {
		_, err := io.WriteString(w, "No runs to compare.\n")
		return err
	}
	b := &strings.Builder{}
	head := "STEP"
	for _, r := range c.Runs {
		head += "\t" + runHeading(r)
	}
	fmt.Fprintf(b, "Comparing %s against %s\n", count(len(c.Runs), "run"), runHeading(c.Runs[0]))
	for _, r := range c.Runs {
		fmt.Fprintf(b, "  %s  %s %s  agent %s  %s  wall %s\n", shortID(r.RunID), r.Kind, r.Name, r.Agent, r.Status, fmtMS(r.WallMS))
	}

	section(b, "Steps (duration)")
	if len(c.Steps) == 0 {
		fmt.Fprintln(b, "none recorded")
	} else {
		tw := table(b)
		fmt.Fprintln(tw, head)
		for _, s := range c.Steps {
			fmt.Fprintf(tw, "%s\t%s\n", s.Step, figureCells(s.Figures[0]))
		}
		tw.Flush()
	}

	section(b, "Totals")
	tw := table(b)
	fmt.Fprintln(tw, "FIGURE"+strings.TrimPrefix(head, "STEP"))
	for _, f := range c.Totals {
		fmt.Fprintf(tw, "%s\t%s\n", f.Figure, figureCells(f))
	}
	tw.Flush()
	skippedLine(b, skipped)
	_, err := io.WriteString(w, b.String())
	return err
}

// RenderSummary draws a summary: per step and in total, the number of runs,
// the median and the 90th percentile.
func RenderSummary(w io.Writer, s Summary, skipped int) error {
	b := &strings.Builder{}
	fmt.Fprintf(b, "Runs: %d\n", len(s.Runs))
	for _, r := range s.Runs {
		fmt.Fprintf(b, "  %s  %s %s  agent %s  %s  wall %s\n", shortID(r.RunID), r.Kind, r.Name, r.Agent, r.Status, fmtMS(r.WallMS))
	}

	section(b, "Steps (duration)")
	if len(s.Steps) == 0 {
		fmt.Fprintln(b, "none recorded")
	} else {
		tw := table(b)
		fmt.Fprintln(tw, "STEP\tRUNS\tMEDIAN\tP90")
		for _, st := range s.Steps {
			d := st.Figures[0]
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", st.Step, d.Count, statValue(d.Figure, d.Median), statValue(d.Figure, d.P90))
		}
		tw.Flush()
	}

	section(b, "Totals")
	tw := table(b)
	fmt.Fprintln(tw, "FIGURE\tRUNS\tMEDIAN\tP90")
	for _, f := range s.Totals {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", f.Figure, f.Count, statValue(f.Figure, f.Median), statValue(f.Figure, f.P90))
	}
	tw.Flush()
	skippedLine(b, skipped)
	_, err := io.WriteString(w, b.String())
	return err
}

// runHeading names a run in a column heading: its short id and start date.
func runHeading(r RunSummary) string {
	return shortID(r.RunID) + " " + r.StartedAt.Format("2006-01-02")
}

// figureCells are a figure's per-run cells: the first run's value, then each
// later run's value with its difference, as "10m01s (-3m31s, -26.0%)".
func figureCells(f Figure) string {
	cells := make([]string, len(f.Values))
	for i, v := range f.Values {
		if v == nil {
			cells[i] = "-"
			continue
		}
		cells[i] = figureValue(f.Figure, *v)
		if i == 0 || f.Delta[i] == nil {
			continue
		}
		diff := figureValue(f.Figure, *f.Delta[i])
		if *f.Delta[i] >= 0 {
			diff = "+" + diff
		}
		if f.DeltaPct[i] != nil {
			diff += fmt.Sprintf(", %+.1f%%", *f.DeltaPct[i])
		}
		cells[i] += " (" + diff + ")"
	}
	return strings.Join(cells, "\t")
}

// isDuration reports whether a figure is in milliseconds.
func isDuration(figure string) bool { return strings.HasSuffix(figure, "_ms") }

func figureValue(figure string, v int64) string {
	if isDuration(figure) {
		return fmtMS(v)
	}
	return fmt.Sprintf("%d", v)
}

// statValue prints a median or percentile, which may be fractional.
func statValue(figure string, v *float64) string {
	if v == nil {
		return "-"
	}
	if isDuration(figure) {
		return fmtMS(int64(math.Round(*v)))
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

// RenderCommands draws the command records as one table, oldest first as
// given. A command outside any run shows "-" for its run.
func RenderCommands(w io.Writer, rows []CommandRow, skipped int) error {
	b := &strings.Builder{}
	if len(rows) == 0 {
		fmt.Fprintln(b, "No recorded commands.")
	} else {
		tw := table(b)
		fmt.Fprintln(tw, "TIME\tRUN\tSTEP\tCOMMAND\tEXIT\tDURATION")
		for _, c := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", c.StartedAt.Format(timeLayout), runCell(c.RunID),
				orDash(c.Step), c.Command, c.ExitCode, fmtMS(c.DurationMS))
		}
		tw.Flush()
	}
	skippedLine(b, skipped)
	_, err := io.WriteString(w, b.String())
	return err
}

// RenderSpans draws the agent-marked spans as one table, oldest first as
// given. An unfinished span's end reason is marked, as in a run report.
func RenderSpans(w io.Writer, rows []SpanRow, skipped int) error {
	b := &strings.Builder{}
	if len(rows) == 0 {
		fmt.Fprintln(b, "No recorded spans.")
	} else {
		unfinished := false
		tw := table(b)
		fmt.Fprintln(tw, "TIME\tRUN\tTYPE\tNAME\tSTEP\tDURATION\tEND")
		for _, s := range rows {
			end := s.EndReason
			if !s.Complete {
				end += unfinishedMark
				unfinished = true
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.StartedAt.Format(timeLayout), runCell(s.RunID),
				s.Type, s.Name, orDash(s.Step), fmtMS(s.DurationMS), end)
		}
		tw.Flush()
		if unfinished {
			fmt.Fprintf(b, "\n%s did not finish: its duration runs to the last event recorded with it.\n", unfinishedMark)
		}
	}
	skippedLine(b, skipped)
	_, err := io.WriteString(w, b.String())
	return err
}

// runCell is a run id cut short, or "-" outside any run.
func runCell(id *string) string {
	if id == nil {
		return "-"
	}
	return shortID(*id)
}
