package plantask

import (
	"fmt"
	"io"
	"strings"
)

// Export is a plan's task graph as `plan export` reports it. Field names and
// nesting are fixed by the plan-task-graph design; consumers such as Hive
// decode them directly.
type Export struct {
	Kind           string       `json:"kind"` // always "plan"
	Name           string       `json:"name"`
	DocumentStatus string       `json:"document_status"`
	Tasks          []ExportTask `json:"tasks"`
}

// ExportTask is one task of an Export.
type ExportTask struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Milestone int        `json:"milestone"`
	Repo      ExportRepo `json:"repo"`
	DependsOn []string   `json:"depends_on"` // never null: [] for "none"
	Execution Execution  `json:"execution"`
	Completed bool       `json:"completed"`
}

// ExportRepo names a task's repo and where its code lives. Location is the
// repo's declared git source, and empty when none is declared.
type ExportRepo struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

// NewExport builds the export of a parsed task-format plan. location maps a
// registered repo name to its declared git source ("" when none).
func NewExport(name, documentStatus string, p Plan, location func(repo string) string) Export {
	e := Export{Kind: "plan", Name: name, DocumentStatus: documentStatus, Tasks: []ExportTask{}}
	cache := map[string]string{}
	for _, t := range p.Tasks {
		loc, ok := cache[t.Repo]
		if !ok {
			loc = location(t.Repo)
			cache[t.Repo] = loc
		}
		deps := append([]string{}, t.DependsOn...)
		e.Tasks = append(e.Tasks, ExportTask{
			ID:        t.ID,
			Title:     t.Title,
			Milestone: t.Milestone,
			Repo:      ExportRepo{Name: t.Repo, Location: loc},
			DependsOn: deps,
			Execution: t.Execution,
			Completed: t.Completed,
		})
	}
	return e
}

// RenderPretty writes the human-readable export: a header with the plan's
// status and progress, then tasks grouped by milestone in plan order, each
// with its completion, title, repo and executor, its id beneath, and its
// dependencies by title.
func RenderPretty(w io.Writer, e Export) error {
	done := 0
	titleW, repoW := 0, 0
	titles := make(map[string]string, len(e.Tasks))
	for _, t := range e.Tasks {
		if t.Completed {
			done++
		}
		titleW = max(titleW, len(t.Title))
		repoW = max(repoW, len(t.Repo.Name))
		titles[t.ID] = t.Title
	}

	status := e.DocumentStatus
	if status == "" {
		status = "no status"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  (%s)  %d/%d tasks complete\n", e.Name, status, done, len(e.Tasks))

	milestone := -1
	for _, t := range e.Tasks {
		if t.Milestone != milestone {
			milestone = t.Milestone
			fmt.Fprintf(&b, "\nMilestone %d\n", milestone)
		}
		box := " "
		if t.Completed {
			box = "x"
		}
		exec := t.Execution.Type
		if t.Execution.Reason != "" {
			exec += ": " + t.Execution.Reason
		}
		fmt.Fprintf(&b, "  [%s] %-*s   %-*s   %s\n", box, titleW, t.Title, repoW, t.Repo.Name, exec)
		fmt.Fprintf(&b, "      %s\n", t.ID)
		if len(t.DependsOn) > 0 {
			names := make([]string, len(t.DependsOn))
			for i, id := range t.DependsOn {
				names[i] = titles[id]
				if names[i] == "" {
					names[i] = id
				}
			}
			fmt.Fprintf(&b, "      depends on: %s\n", strings.Join(names, ", "))
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}
