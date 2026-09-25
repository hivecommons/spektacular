// Package plantask is the single reader of a plan's work. It turns the
// "## Milestones & Tasks" section of a plan.md into milestones and tasks, and
// owns the structural rules a task-format plan must satisfy.
//
// Every piece of code that needs to understand a plan's work — write
// validation, export, status progress, implement and milestone commits — asks
// this package rather than pattern-matching plan.md on its own.
//
// Plans written before the task format describe their work as
// "#### - [ ] Phase N.M:" headings under "## Milestones & Phases". Those parse
// as FormatLegacy: they carry milestone completion but no tasks.
package plantask

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Format says how a plan describes its work.
type Format int

const (
	// FormatNone is a plan with neither task nor phase headings.
	FormatNone Format = iota
	// FormatLegacy is a plan whose work is "Phase N.M" headings only.
	FormatLegacy
	// FormatTasks is a plan with at least one "Task:" heading.
	FormatTasks
)

// String names the format for messages and tests.
func (f Format) String() string {
	switch f {
	case FormatTasks:
		return "tasks"
	case FormatLegacy:
		return "legacy"
	default:
		return "none"
	}
}

// Plan is what the reader saw in a plan's Milestones & Tasks section.
type Plan struct {
	Format     Format
	Milestones []Milestone // in plan order
	Tasks      []Task      // in plan order, across milestones

	// openPhases counts unticked legacy phase headings in the section,
	// whether or not a milestone heading precedes them.
	openPhases int
}

// Milestone is one "### Milestone N:" group and the completion of the work
// items (tasks, or legacy phases) under it.
type Milestone struct {
	Number int
	Title  string
	Items  int // tasks (or legacy phases) under it
	Open   int // unchecked items
}

// Execution is who carries a task out, and why when it is a person.
type Execution struct {
	Type   string `json:"type"`   // "agent" | "human"
	Reason string `json:"reason"` // set for human tasks
}

// Criteria counts a task's acceptance-criteria checkboxes.
type Criteria struct {
	Met   int `json:"met"`
	Total int `json:"total"`
}

// Task is one unit of work read from a "#### - [ ] Task:" block.
type Task struct {
	ID        string
	Title     string
	Milestone int
	Repo      string   // the single registry name; empty when missing or ambiguous
	DependsOn []string // ids; empty for "none"
	Execution Execution
	Completed bool // heading checkbox
	Criteria  Criteria

	// What the reader saw, so Validate can tell "missing" from "empty" and
	// report every repo named on the line.
	hasID       bool
	hasRepo     bool
	repos       []string
	hasDepends  bool
	dependsNone bool
	hasExec     bool
}

var (
	milestoneHeading = regexp.MustCompile(`^###\s+Milestone\s+(\d+)\s*:\s*(.*)$`)
	taskHeading      = regexp.MustCompile(`^####\s+-\s+\[([ xX])\]\s+Task:\s*(.+?)\s*$`)
	legacyHeading    = regexp.MustCompile(`^####\s+-\s+\[([ xX])\]\s+Phase\b`)
	idLine           = regexp.MustCompile(`^\*\*Id:\*\*\s*(.*)$`)
	repoLine         = regexp.MustCompile(`^\*\*Repo:\*\*\s*(.*)$`)
	dependsLine      = regexp.MustCompile(`^\*\*Depends on:\*\*\s*(.*)$`)
	executionLine    = regexp.MustCompile(`^\*\*Execution:\*\*\s*(.*)$`)
	criteriaMarker   = regexp.MustCompile(`^\*\*Acceptance criteria:?\*\*:?`)
	criterionBox     = regexp.MustCompile(`^\s*-\s+\[([ xX])\]`)
	listItem         = regexp.MustCompile(`^\s*-\s+(.*)$`)
	// separator splits "<id> — <title>" and "human — <reason>". The em dash is
	// the format's separator; "-", "--" and ":" are accepted as hand-typed
	// stand-ins.
	separator = regexp.MustCompile(`\s+(?:—|--|-)\s+|\s*:\s+`)
)

// Parse reads plan.md. It never fails: it records what it saw so a legacy or
// partly structured plan still yields milestone counts. Validate applies the
// structural rules.
//
// Only the "## Milestones & Tasks" (or legacy "## Milestones & Phases")
// section is read, so a checkbox anywhere else in the plan — a changelog
// entry, a testing note — never counts as a task or a criterion.
func Parse(markdown []byte) Plan {
	var p Plan
	sawTask, sawLegacy := false, false

	inSection := false
	milestone := -1 // index into p.Milestones, -1 before the first
	task := -1      // index into p.Tasks of the open task block, -1 outside one
	inCriteria := false
	inDepends := false

	closeBlock := func() {
		task = -1
		inCriteria = false
		inDepends = false
	}

	for raw := range strings.SplitSeq(string(markdown), "\n") {
		line := strings.TrimRight(raw, " \t\r")

		if strings.HasPrefix(line, "## ") {
			inSection = strings.HasPrefix(line, "## Milestones & Tasks") ||
				strings.HasPrefix(line, "## Milestones & Phases")
			milestone = -1
			closeBlock()
			continue
		}
		if !inSection {
			continue
		}

		if m := milestoneHeading.FindStringSubmatch(line); m != nil {
			closeBlock()
			n, err := strconv.Atoi(m[1])
			if err != nil {
				milestone = -1
				continue
			}
			milestone = p.milestoneIndex(n, strings.TrimSpace(m[2]))
			continue
		}
		if strings.HasPrefix(line, "### ") {
			// Any other third-level heading ends the task block but not the
			// milestone, as the milestone committer always read it.
			closeBlock()
			continue
		}

		if m := taskHeading.FindStringSubmatch(line); m != nil {
			closeBlock()
			sawTask = true
			t := Task{Title: m[2], Completed: m[1] != " "}
			if milestone >= 0 {
				ms := &p.Milestones[milestone]
				t.Milestone = ms.Number
				ms.Items++
				if !t.Completed {
					ms.Open++
				}
			}
			p.Tasks = append(p.Tasks, t)
			task = len(p.Tasks) - 1
			continue
		}
		if m := legacyHeading.FindStringSubmatch(line); m != nil {
			closeBlock()
			sawLegacy = true
			if m[1] == " " {
				p.openPhases++
			}
			if milestone >= 0 {
				ms := &p.Milestones[milestone]
				ms.Items++
				if m[1] == " " {
					ms.Open++
				}
			}
			continue
		}
		if strings.HasPrefix(line, "#### ") {
			closeBlock()
			continue
		}

		if task < 0 {
			continue
		}
		readTaskLine(&p.Tasks[task], line, &inDepends, &inCriteria)
	}

	switch {
	case sawTask:
		p.Format = FormatTasks
	case sawLegacy:
		p.Format = FormatLegacy
	}
	return p
}

// readTaskLine folds one line inside a task block into t.
func readTaskLine(t *Task, line string, inDepends, inCriteria *bool) {
	if *inDepends {
		if m := listItem.FindStringSubmatch(line); m != nil && !criterionBox.MatchString(line) {
			if id := firstToken(m[1]); id != "" {
				t.DependsOn = append(t.DependsOn, id)
			}
			return
		}
		*inDepends = false
	}

	if *inCriteria {
		if m := criterionBox.FindStringSubmatch(line); m != nil {
			t.Criteria.Total++
			if m[1] != " " {
				t.Criteria.Met++
			}
		}
		return
	}

	switch {
	case idLine.MatchString(line):
		t.hasID = true
		t.ID = firstToken(idLine.FindStringSubmatch(line)[1])
	case repoLine.MatchString(line):
		t.hasRepo = true
		t.repos = splitRepos(repoLine.FindStringSubmatch(line)[1])
		if len(t.repos) == 1 {
			t.Repo = t.repos[0]
		}
	case dependsLine.MatchString(line):
		t.hasDepends = true
		rest := strings.TrimSpace(dependsLine.FindStringSubmatch(line)[1])
		switch {
		case strings.EqualFold(rest, "none"):
			t.dependsNone = true
		case rest != "":
			for _, f := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' }) {
				if id := firstToken(f); id != "" {
					t.DependsOn = append(t.DependsOn, id)
				}
			}
		default:
			*inDepends = true
		}
	case executionLine.MatchString(line):
		t.hasExec = true
		t.Execution = parseExecution(executionLine.FindStringSubmatch(line)[1])
	case criteriaMarker.MatchString(line):
		*inCriteria = true
	}
}

// parseExecution splits "human — <reason>" into its type and reason. The type
// is kept as written (lower-cased) so an unknown one can be reported.
func parseExecution(s string) Execution {
	s = strings.TrimSpace(s)
	parts := separator.Split(s, 2)
	e := Execution{Type: strings.ToLower(strings.Trim(strings.TrimSpace(parts[0]), "`"))}
	if len(parts) == 2 {
		e.Reason = strings.TrimSpace(parts[1])
	}
	return e
}

// splitRepos returns every repo name on a "**Repo:**" line, so a line naming
// two can be refused rather than silently read as one.
func splitRepos(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		f = strings.Trim(f, "`*.;")
		if f == "" || strings.EqualFold(f, "and") || f == "&" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// firstToken returns the first whitespace-separated token of s with any
// surrounding backticks removed: the id in "<id> — <title>".
func firstToken(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], "`")
}

// Task returns the task with the given id.
func (p Plan) Task(id string) (Task, bool) {
	for _, t := range p.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}

// OpenTasks returns the tasks whose heading checkbox is not ticked, in plan
// order.
func (p Plan) OpenTasks() []Task {
	var open []Task
	for _, t := range p.Tasks {
		if !t.Completed {
			open = append(open, t)
		}
	}
	return open
}

// OpenItems counts the unchecked work items in the section: tasks in a
// task-format plan, or phase headings in an older plan, whether or not they
// sit under a milestone heading.
func (p Plan) OpenItems() int {
	if p.Format == FormatTasks {
		return len(p.OpenTasks())
	}
	return p.openPhases
}

// CompletedMilestones returns the numbers of milestones that have work items
// and all of them ticked, ascending. A milestone with nothing under it is
// never complete.
func (p Plan) CompletedMilestones() []int {
	var done []int
	for _, m := range p.Milestones {
		if m.Items > 0 && m.Open == 0 {
			done = append(done, m.Number)
		}
	}
	sort.Ints(done)
	return done
}

// milestoneIndex returns the index of milestone n, adding it on first sight.
// A number repeated further down the section continues the same milestone.
func (p *Plan) milestoneIndex(n int, title string) int {
	for i, m := range p.Milestones {
		if m.Number == n {
			return i
		}
	}
	p.Milestones = append(p.Milestones, Milestone{Number: n, Title: title})
	return len(p.Milestones) - 1
}
