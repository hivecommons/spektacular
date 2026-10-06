package status

import (
	"fmt"
	"io"
	"strings"

	"github.com/hivecommons/spektacular/internal/plantask"
)

// RequestedMarker flags the requested spec's line in the readable tree.
const RequestedMarker = "← requested"

// RenderPretty writes the readable tree: the workflow in progress when there
// is one, the epic's header with its status and roll-up, then one line per
// spec with its state and task counts. The requested spec is expanded to its
// dependencies, milestones and tasks; its siblings show one line each, plus
// their dependencies. A standalone spec has no epic header.
func RenderPretty(w io.Writer, r Report) error {
	var b strings.Builder
	if r.Workflow != nil {
		fmt.Fprintf(&b, "workflow in progress: %s %s, at step %s\n", r.Workflow.Kind, r.Workflow.Name, r.Workflow.CurrentStep)
		if r.Specs != nil {
			b.WriteString("\n")
		}
	} else if r.Specs == nil {
		b.WriteString("no workflow in progress\n")
	}

	indent := ""
	if r.Epic != nil {
		e := r.Epic
		docStatus := e.DocumentStatus
		if docStatus == "" {
			docStatus = "no status"
		}
		fmt.Fprintf(&b, "epic %s  (%s)  %d/%d specs implemented, %d/%d tasks",
			e.Name, docStatus, e.Progress.SpecsImplemented, e.Progress.SpecsTotal, e.Progress.TasksCompleted, e.Progress.TasksTotal)
		if e.Done {
			b.WriteString(", done")
		}
		b.WriteString("\n")
		if e.Run != nil {
			fmt.Fprintf(&b, "  planning: %s\n", runCountsText(e.Run.Plan))
			fmt.Fprintf(&b, "  implementing: %s\n", runCountsText(e.Run.Implement))
			for _, p := range e.Run.Problems {
				fmt.Fprintf(&b, "  problem (%s): %s\n", p.Code, p.Message)
			}
		}
		b.WriteString("\n")
		indent = "  "
		if len(r.Specs) == 0 {
			b.WriteString(indent + "no specs\n")
		}
	}

	nameW, stateW, countW := 0, 0, 0
	counts := make([]string, len(r.Specs))
	for i, s := range r.Specs {
		counts[i] = countText(s)
		nameW = max(nameW, len(s.Name))
		stateW = max(stateW, len(Label(s.State)))
		countW = max(countW, len(counts[i]))
	}

	requested := requestedIndex(r)
	child := indent + "    "
	for i, s := range r.Specs {
		line := fmt.Sprintf("%s%-*s   %-*s   %-*s", indent, nameW, s.Name, stateW, Label(s.State), countW, counts[i])
		if i == requested {
			line += "   " + RequestedMarker
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
		if len(s.DependsOn) > 0 {
			fmt.Fprintf(&b, "%sdepends on: %s\n", child, strings.Join(s.DependsOn, ", "))
		}
		if i == requested && s.Plan != nil && len(s.Plan.Tasks) > 0 {
			tasks := make([]plantask.ExportTask, len(s.Plan.Tasks))
			for j, t := range s.Plan.Tasks {
				tasks[j] = t.ExportTask
			}
			if err := plantask.WriteTasks(&b, tasks, child, false); err != nil {
				return err
			}
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
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
