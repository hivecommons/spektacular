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
		fmt.Fprintln(tw, "STEP\tENTRIES\tDURATION\tCOMMANDS\tCMD TIME\tHELPER WAIT\tUSER WAIT\tINCOMPLETE\t"+tokenHeads)
		for _, s := range rep.Steps {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%s\t%s\t%s\t%d\t%s\n", s.Step, s.Entries, fmtMS(s.DurationMS),
				s.Commands, fmtMS(s.CommandMS), fmtMS(s.HelperWaitMS), fmtMS(s.UserWaitMS), s.Incomplete, tokenCells(s.Tokens))
		}
		tw.Flush()
	}

	renderTokens(b, rep)

	if len(rep.Tasks) > 0 {
		section(b, "Tasks")
		tw = table(b)
		fmt.Fprintln(tw, "TASK\tSTARTED\tDURATION\tHELPER WAIT\tUSER WAIT\tEND\t"+tokenHeads)
		for _, k := range rep.Tasks {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", k.Name, k.StartedAt.Format(timeLayout), fmtMS(k.DurationMS),
				fmtMS(k.HelperWaitMS), fmtMS(k.UserWaitMS), mark(k.EndReason, k.Complete), tokenCells(k.Tokens))
		}
		tw.Flush()
	}

	section(b, "Helpers")
	if len(rep.Helpers) == 0 {
		fmt.Fprintln(b, "none")
	} else {
		tw := table(b)
		fmt.Fprintln(tw, "NAME\tSTEP\tSTARTED\tDURATION\tEND\t"+tokenHeads)
		for _, x := range rep.Helpers {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", x.Name, orDash(x.Step), x.StartedAt.Format(timeLayout),
				fmtMS(x.DurationMS), mark(x.EndReason, x.Complete), tokenCells(x.Tokens))
		}
		tw.Flush()
	}

	section(b, "User waits")
	if len(rep.UserWaits) == 0 {
		fmt.Fprintln(b, "none")
	} else {
		tw := table(b)
		fmt.Fprintln(tw, "NAME\tSTEP\tSTARTED\tDURATION\tEND")
		for _, x := range rep.UserWaits {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", x.Name, orDash(x.Step), x.StartedAt.Format(timeLayout),
				fmtMS(x.DurationMS), mark(x.EndReason, x.Complete))
		}
		tw.Flush()
	}

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
	if t := rep.Tokens.Total; t != nil && !t.CostComplete {
		fmt.Fprintf(b, "\n%s after a cost: incomplete, no price for %s (prices per million tokens go under stats.prices in .spektacular/config.yaml).\n",
			incompleteMark, strings.Join(t.UnpricedModels, ", "))
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

	// The steps' token figures, when any run's token use is known.
	if comparedTokens(c) {
		for _, tf := range stepTokenTables {
			section(b, "Steps ("+tf.title+")")
			tw := table(b)
			fmt.Fprintln(tw, head)
			for _, s := range c.Steps {
				fmt.Fprintf(tw, "%s\t%s\n", s.Step, figureCells(namedFigure(s.Figures, tf.figure)))
			}
			tw.Flush()
		}
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

	// The steps' token figures, when any run's token use is known.
	if summarisedTokens(s) {
		for _, tf := range stepTokenTables {
			section(b, "Steps ("+tf.title+")")
			tw := table(b)
			fmt.Fprintln(tw, "STEP\tRUNS\tMEDIAN\tP90")
			for _, st := range s.Steps {
				d := namedStat(st.Figures, tf.figure)
				fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", st.Step, d.Count, statValue(d.Figure, d.Median), statValue(d.Figure, d.P90))
			}
			tw.Flush()
		}
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
func isDuration(figure string) bool {
	return strings.HasSuffix(figure, "_ms") || figure == "working_ms_per_1k_output"
}

// figureValue prints one value of a figure: a duration as a time, a token
// count with thousands separators, a cost with 2 decimals, and any other
// count as a number.
func figureValue(figure string, v float64) string {
	switch {
	case isDuration(figure):
		return fmtMS(int64(math.Round(v)))
	case strings.HasSuffix(figure, "_tokens"):
		return fmtTokens(int64(math.Round(v)))
	case figure == "cost":
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// statValue prints a median or percentile, which may be fractional.
func statValue(figure string, v *float64) string {
	if v == nil {
		return "-"
	}
	return figureValue(figure, *v)
}

// stepTokenTables are the step token figures a comparison or summary shows
// beside the steps' durations.
var stepTokenTables = []struct{ figure, title string }{
	{"output_tokens", "output tokens"},
	{"cost", "cost"},
	{"working_ms_per_1k_output", "time per 1k output tokens"},
}

func namedFigure(fs []Figure, name string) Figure {
	for _, f := range fs {
		if f.Figure == name {
			return f
		}
	}
	return Figure{Figure: name}
}

func namedStat(fs []Stat, name string) Stat {
	for _, f := range fs {
		if f.Figure == name {
			return f
		}
	}
	return Stat{Figure: name}
}

// comparedTokens reports whether any compared run's token use is known.
func comparedTokens(c Comparison) bool {
	for _, v := range namedFigure(c.Totals, "output_tokens").Values {
		if v != nil {
			return true
		}
	}
	return false
}

// summarisedTokens reports whether any summarised run's token use is known.
func summarisedTokens(s Summary) bool {
	return namedStat(s.Totals, "output_tokens").Count > 0
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

// incompleteMark follows a cost that leaves out unpriced models.
const incompleteMark = "*"

// tokenHeads are the token columns beside a step's, task's or helper's
// timings.
const tokenHeads = "OUT TOKENS\tCOST\tTIME/1K OUT"

// tokenCells are a token figure's columns: output tokens, cost and time per
// thousand output tokens, each "?" when token use is unknown.
func tokenCells(f *TokenFigure) string {
	if f == nil {
		return "?\t?\t?"
	}
	return fmtTokens(f.Output) + "\t" + fmtCost(f) + "\t" + fmtPace(f.WorkingMSPer1kOutput)
}

// fmtTokens prints a token count with thousands separators.
func fmtTokens(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// fmtCost prints a figure's cost with 2 decimals: "?" when unknown, marked
// when it leaves out unpriced models.
func fmtCost(f *TokenFigure) string {
	if f == nil || f.Cost == nil {
		return "?"
	}
	c := strconv.FormatFloat(*f.Cost, 'f', 2, 64)
	if !f.CostComplete {
		c += incompleteMark
	}
	return c
}

// fmtPace prints working time per thousand output tokens, "n/a" when it
// does not apply.
func fmtPace(v *int64) string {
	if v == nil {
		return "n/a"
	}
	return fmtMS(*v)
}

// tokenRow is a figure's four token types and cost, as table cells.
func tokenRow(f *TokenFigure) string {
	if f == nil {
		return "?\t?\t?\t?\t?"
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s", fmtTokens(f.Input), fmtTokens(f.Output),
		fmtTokens(f.CacheRead), fmtTokens(f.CacheWrite), fmtCost(f))
}

// renderTokens draws the Tokens block: the sessions read, each step's four
// token types and cost, the tokens outside any step (with the session
// overhead in them), the total, the models used and any helper no launch
// matched. For unknown token use it prints only the reason.
func renderTokens(b *strings.Builder, rep Report) {
	tk := rep.Tokens
	section(b, "Tokens")
	if tk.Status != TokensKnown {
		code, reason := "", ""
		if tk.ReasonCode != nil {
			code = *tk.ReasonCode
		}
		if tk.Reason != nil {
			reason = *tk.Reason
		}
		fmt.Fprintf(b, "unknown (%s): %s\n", code, reason)
		return
	}
	var from []string
	for _, s := range tk.Sessions {
		from = append(from, "session "+shortID(s.SessionID)+" ("+s.Transcript+")")
	}
	fmt.Fprintf(b, "Read from %s\n", strings.Join(from, ", "))

	tw := table(b)
	fmt.Fprintln(tw, "STEP\tINPUT\tOUTPUT\tCACHE READ\tCACHE WRITE\tCOST")
	for _, s := range rep.Steps {
		fmt.Fprintf(tw, "%s\t%s\n", s.Step, tokenRow(s.Tokens))
	}
	fmt.Fprintf(tw, "(outside any step)\t%s\n", tokenRow(tk.OutsideSteps))
	if tk.SessionOverhead != nil {
		fmt.Fprintf(tw, "  of which session overhead\t%s\n", tokenRow(tk.SessionOverhead))
	} else if tk.SessionOverheadReason != nil {
		fmt.Fprintf(tw, "  session overhead unknown (%s)\t\t\t\t\t\n", *tk.SessionOverheadReason)
	}
	fmt.Fprintf(tw, "TOTAL\t%s\n", tokenRow(tk.Total))
	tw.Flush()

	section(b, "Models")
	tw = table(b)
	fmt.Fprintln(tw, "MODEL\tINPUT\tOUTPUT\tCACHE READ\tCACHE WRITE\tCOST")
	for _, m := range tk.Models {
		cost := "?"
		if m.Cost != nil {
			cost = strconv.FormatFloat(*m.Cost, 'f', 2, 64)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", m.Model, fmtTokens(m.Input), fmtTokens(m.Output),
			fmtTokens(m.CacheRead), fmtTokens(m.CacheWrite), cost)
	}
	tw.Flush()

	if len(tk.UnmatchedHelpers) > 0 {
		section(b, "Unmatched helpers")
		tw = table(b)
		fmt.Fprintln(tw, "NAME\tSTEP\tSTARTED\tDURATION\t"+tokenHeads)
		for _, h := range tk.UnmatchedHelpers {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", h.Name, orDash(h.Step), h.StartedAt.Format(timeLayout),
				fmtMS(h.DurationMS), tokenCells(h.Tokens))
		}
		tw.Flush()
	}
}
