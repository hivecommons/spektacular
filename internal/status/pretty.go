package status

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// RequestedMarker flags the requested spec's line in the readable tree.
const RequestedMarker = "← requested"

// barWidth is the number of cells in a progress bar.
const barWidth = 20

// theme is the readable tree's styles. Its renderer decides, from the
// terminal writer, whether colour is used at all: a writer that is not a terminal,
// or NO_COLOR, gets the same text with no escape codes.
type theme struct {
	title, heading, dim, bold, accent, ok, warn, bad, info lipgloss.Style
}

func newTheme(w io.Writer) theme {
	r := lipgloss.NewRenderer(w)
	// The 16 ANSI colours follow the terminal's own theme, light or dark,
	// without querying the terminal for its background.
	return theme{
		title:   r.NewStyle().Bold(true).Foreground(lipgloss.Color("5")),
		heading: r.NewStyle().Bold(true),
		dim:     r.NewStyle().Faint(true),
		bold:    r.NewStyle().Bold(true),
		accent:  r.NewStyle().Foreground(lipgloss.Color("6")),
		ok:      r.NewStyle().Foreground(lipgloss.Color("2")),
		warn:    r.NewStyle().Foreground(lipgloss.Color("3")),
		bad:     r.NewStyle().Foreground(lipgloss.Color("1")),
		info:    r.NewStyle().Foreground(lipgloss.Color("4")),
	}
}

// stateStyle is a spec state's glyph and colour.
func (t theme) stateStyle(state SpecState) (string, lipgloss.Style) {
	switch state {
	case StateImplemented:
		return "✓", t.ok
	case StateInProgress:
		return "◐", t.warn
	case StatePlanned:
		return "○", t.info
	case StateStale:
		return "!", t.bad
	case StateMissing:
		return "✗", t.bad
	default:
		return "·", t.dim
	}
}

// pad renders text in style and pads it, outside the style, to width cells.
func pad(style lipgloss.Style, text string, width int) string {
	return style.Render(text) + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

// bar is a progress bar of done out of total, filled in style.
func (t theme) bar(done, total int, style lipgloss.Style) string {
	filled := 0
	if total > 0 {
		filled = done * barWidth / total
	}
	return style.Render(strings.Repeat("━", filled)) + t.dim.Render(strings.Repeat("─", barWidth-filled))
}

// RenderPretty writes the readable tree: every workflow in progress when
// there is one, the epic's header with its status, progress bar and
// roll-up, then one line per spec with its state and task counts. The
// requested spec is expanded to its dependencies, milestones and tasks; its
// siblings show one line each, plus their dependencies. A standalone spec has
// no epic header.
func RenderPretty(w io.Writer, r Report) error {
	return RenderPrettyStyled(w, r, w)
}

// RenderPrettyStyled is RenderPretty, with colour decided by whether term,
// rather than w, is a terminal: for a w that wraps the terminal, such as a
// writer that also copies the output into a log.
func RenderPrettyStyled(w io.Writer, r Report, term io.Writer) error {
	t := newTheme(term)
	var b strings.Builder

	workflows := r.Workflows
	if len(workflows) == 0 && r.Workflow != nil {
		workflows = []WorkflowInfo{*r.Workflow}
	}
	if len(workflows) > 0 {
		writeWorkflows(&b, t, workflows)
		if r.Specs != nil {
			b.WriteString("\n")
		}
	} else if r.Specs == nil {
		b.WriteString(t.dim.Render("no workflow in progress") + "\n")
	}

	indent := ""
	if r.Epic != nil {
		writeEpic(&b, t, r.Epic)
		b.WriteString("\n")
		indent = "  "
		if len(r.Specs) == 0 {
			b.WriteString(indent + t.dim.Render("no specs") + "\n")
		}
	}

	nameW, stateW, countW := 0, 0, 0
	counts := make([]string, len(r.Specs))
	for i, s := range r.Specs {
		counts[i] = countText(s)
		nameW = max(nameW, lipgloss.Width(s.Name))
		stateW = max(stateW, lipgloss.Width(Label(s.State)))
		countW = max(countW, lipgloss.Width(counts[i]))
	}

	requested := requestedIndex(r)
	child := indent + "    "
	for i, s := range r.Specs {
		glyph, style := t.stateStyle(s.State)
		name := pad(t.bold, s.Name, nameW)
		if i == requested {
			name = pad(t.accent.Bold(true), s.Name, nameW)
		}
		line := fmt.Sprintf("%s%s %s   %s   %s", indent, style.Render(glyph), name,
			pad(style, Label(s.State), stateW), pad(lipgloss.NewStyle(), counts[i], countW))
		if i == requested {
			line += "   " + t.accent.Render(RequestedMarker)
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
		if len(s.DependsOn) > 0 {
			b.WriteString(child + t.dim.Render("depends on: "+strings.Join(s.DependsOn, ", ")) + "\n")
		}
		if i == requested && s.Plan != nil && len(s.Plan.Tasks) > 0 {
			writeTasks(&b, t, s.Plan.Tasks, child)
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// writeWorkflows lists every workflow in progress, one aligned line each.
func writeWorkflows(b *strings.Builder, t theme, workflows []WorkflowInfo) {
	title := "workflow in progress"
	if len(workflows) > 1 {
		title = fmt.Sprintf("%d workflows in progress", len(workflows))
	}
	b.WriteString(t.title.Render(title) + "\n")
	kindW, nameW := 0, 0
	for _, wf := range workflows {
		kindW = max(kindW, lipgloss.Width(wf.Kind))
		nameW = max(nameW, lipgloss.Width(wf.Name))
	}
	for _, wf := range workflows {
		line := fmt.Sprintf("  %s %s  %s  %s %s", t.warn.Render("●"), pad(t.info, wf.Kind, kindW),
			pad(t.bold, wf.Name, nameW), t.dim.Render("at step"), wf.CurrentStep)
		if wf.Orchestrated {
			line += "  " + t.dim.Render("(orchestrated)")
		}
		b.WriteString(line + "\n")
	}
}

// writeEpic writes the epic's header: its name and status, a progress bar
// over its tasks, and the planning and implementing roll-up when present.
func writeEpic(b *strings.Builder, t theme, e *EpicStatus) {
	docStatus := e.DocumentStatus
	if docStatus == "" {
		docStatus = "no status"
	}
	header := t.title.Render("epic "+e.Name) + "  " + t.dim.Render("("+docStatus+")")
	if e.Done {
		header += "  " + t.ok.Render("✓ done")
	}
	b.WriteString(header + "\n")

	barStyle := t.warn
	if e.Done {
		barStyle = t.ok
	}
	fmt.Fprintf(b, "  %s  %d/%d specs implemented, %d/%d tasks\n",
		t.bar(e.Progress.TasksCompleted, e.Progress.TasksTotal, barStyle),
		e.Progress.SpecsImplemented, e.Progress.SpecsTotal, e.Progress.TasksCompleted, e.Progress.TasksTotal)
	if e.Run != nil {
		fmt.Fprintf(b, "  %s %s\n", t.heading.Render("planning:"), runCountsText(e.Run.Plan))
		fmt.Fprintf(b, "  %s %s\n", t.heading.Render("implementing:"), runCountsText(e.Run.Implement))
		for _, p := range e.Run.Problems {
			fmt.Fprintf(b, "  %s %s\n", t.bad.Render("problem ("+p.Code+"):"), p.Message)
		}
	}
}

// writeTasks writes the requested spec's tasks under their milestones, each
// with its checkbox, repo, execution and the titles of the tasks it depends
// on.
func writeTasks(b *strings.Builder, t theme, tasks []TaskStatus, indent string) {
	titleW, repoW := 0, 0
	titles := make(map[string]string, len(tasks))
	for _, task := range tasks {
		titleW = max(titleW, lipgloss.Width(task.Title))
		repoW = max(repoW, lipgloss.Width(task.Repo.Name))
		titles[task.ID] = task.Title
	}

	milestone := -1
	for _, task := range tasks {
		if task.Milestone != milestone {
			milestone = task.Milestone
			fmt.Fprintf(b, "%s%s\n", indent, t.heading.Render(fmt.Sprintf("Milestone %d", milestone)))
		}
		box, boxStyle, titleStyle := "[ ]", t.dim, lipgloss.NewStyle()
		if task.Completed {
			box, boxStyle, titleStyle = "[x]", t.ok, t.dim
		}
		exec := task.Execution.Type
		execStyle := t.dim
		if task.Execution.Reason != "" {
			exec += ": " + task.Execution.Reason
		}
		if task.Execution.Type == "human" {
			execStyle = t.warn
		}
		line := fmt.Sprintf("%s  %s %s   %s   %s", indent, boxStyle.Render(box), pad(titleStyle, task.Title, titleW),
			pad(t.info, task.Repo.Name, repoW), execStyle.Render(exec))
		b.WriteString(strings.TrimRight(line, " ") + "\n")
		if len(task.DependsOn) > 0 {
			names := make([]string, len(task.DependsOn))
			for i, id := range task.DependsOn {
				names[i] = titles[id]
				if names[i] == "" {
					names[i] = id
				}
			}
			b.WriteString(indent + "      " + t.dim.Render("depends on: "+strings.Join(names, ", ")) + "\n")
		}
	}
}

// countText is a spec's progress column: its plan's task counts, phase
// counts for an older plan, or why there are none.
func countText(s SpecStatus) string {
	switch {
	case s.Plan == nil:
		return "no plan"
	case s.Plan.Progress != nil:
		return fmt.Sprintf("%d/%d tasks", s.Plan.Progress.TasksCompleted, s.Plan.Progress.TasksTotal)
	case s.legacy:
		return fmt.Sprintf("%d/%d phases", s.counts.Completed, s.counts.Total)
	default:
		return "no tasks"
	}
}

// requestedIndex is the spec the report was asked about: the one whose name,
// or whose plan's name, is the requested name. -1 when the request named the
// epic itself and no spec shares its name, or a standalone report has no
// requested name.
func requestedIndex(r Report) int {
	if r.Requested == "" {
		return -1
	}
	for i, s := range r.Specs {
		if s.Name == r.Requested || (s.Plan != nil && s.Plan.Name == r.Requested) {
			return i
		}
	}
	return -1
}

// runCountsText words one part's counts for the epic header, for example
// "1 done, 1 in progress, 2 ready, 0 blocked".
func runCountsText(c RunCounts) string {
	text := fmt.Sprintf("%d done, %d in progress", c.Done, c.InProgress)
	if c.AwaitingMerge > 0 {
		text += fmt.Sprintf(", %d awaiting merge", c.AwaitingMerge)
	}
	return text + fmt.Sprintf(", %d ready, %d blocked", c.Ready, c.Blocked)
}

// WatchHeader is the line above each frame of a watch: what is watched, how
// often it refreshes, when this frame was drawn, and how to stop.
func WatchHeader(term io.Writer, title string, interval time.Duration, at time.Time) string {
	t := newTheme(term)
	return t.title.Render(title) + "  " + t.dim.Render(fmt.Sprintf("every %s · %s · ctrl+c to quit", interval, at.Format("15:04:05")))
}

// WatchError is a refusal shown in a watch frame instead of the tree, so a
// watch survives a document that is briefly missing or mid-write.
func WatchError(term io.Writer, message string) string {
	return newTheme(term).bad.Render("✗ " + message)
}
