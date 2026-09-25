package plantask

import (
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
)

// Error codes the reader returns.
const (
	// CodeTaskInvalid refuses a task-format plan that breaks a structural rule.
	CodeTaskInvalid = "plan_task_invalid"
	// CodeStructureInvalid refuses a plan with no task structure where one is
	// needed (export, single-task implement).
	CodeStructureInvalid = "plan_structure_invalid"
)

// Validate applies the task format's structural rules to a task-format plan
// and returns the first broken rule as a structured error naming the task.
// registeredRepos are the repo names a task's "**Repo:**" line may use.
//
// Rules are checked task by task in plan order, then across tasks: unknown
// dependencies before cycles, so a cycle is only ever reported between tasks
// that exist.
func Validate(p Plan, registeredRepos []string) error {
	registered := make(map[string]bool, len(registeredRepos))
	for _, r := range registeredRepos {
		registered[r] = true
	}

	byID := map[string]Task{}
	for _, t := range p.Tasks {
		if err := validateTask(t, registered, registeredRepos); err != nil {
			return err
		}
		if _, dup := byID[t.ID]; dup {
			return invalid(t, fmt.Sprintf("id %s is already used by task %q", t.ID, byID[t.ID].Title),
				"run 'plan task-id' for a fresh id and set it as this task's **Id:**; never change the id of the task that had it first")
		}
		byID[t.ID] = t
	}

	for _, t := range p.Tasks {
		for _, dep := range t.DependsOn {
			if _, ok := byID[dep]; !ok {
				return invalid(t, fmt.Sprintf("depends on %s, which is not a task in this plan", dep),
					"list only ids of tasks in this plan under **Depends on:**, or declare **Depends on:** none")
			}
		}
	}

	if cycle := findCycle(p.Tasks, byID); cycle != nil {
		titles := make([]string, len(cycle))
		for i, t := range cycle {
			titles[i] = fmt.Sprintf("%q", t.Title)
		}
		return invalid(cycle[0], "is part of a dependency cycle: "+strings.Join(titles, " -> "),
			"remove one of the **Depends on:** entries in the cycle so the tasks can be ordered")
	}
	return nil
}

// validateTask checks the rules that concern one task on its own.
func validateTask(t Task, registered map[string]bool, registeredRepos []string) error {
	switch {
	case !t.hasID || t.ID == "":
		return invalid(t, "has no **Id:** line",
			"run 'plan task-id' and add the id as **Id:** <id> under the task heading")
	case !t.hasRepo || len(t.repos) == 0:
		return invalid(t, "has no **Repo:** line", repoAction(registeredRepos))
	case !t.hasDepends || (!t.dependsNone && len(t.DependsOn) == 0):
		return invalid(t, "has no dependency declaration",
			"declare **Depends on:** none, or list one '- <id> — <title>' line per task it depends on")
	case !t.hasExec || t.Execution.Type == "":
		return invalid(t, "has no **Execution:** line",
			"set **Execution:** agent, or **Execution:** human — <reason> when a person must carry it out")
	case len(t.repos) > 1:
		return invalid(t, fmt.Sprintf("names more than one repo (%s); a task is carried out in exactly one", strings.Join(t.repos, ", ")),
			"split the task into one task per repo, or "+repoAction(registeredRepos))
	case !registered[t.Repo]:
		return invalid(t, fmt.Sprintf("names repo %q, which is not registered", t.Repo), repoAction(registeredRepos))
	case t.Execution.Type != "agent" && t.Execution.Type != "human":
		return invalid(t, fmt.Sprintf("declares execution %q; it must be agent or human", t.Execution.Type),
			"set **Execution:** agent, or **Execution:** human — <reason>")
	case t.Execution.Type == "human" && t.Execution.Reason == "":
		return invalid(t, "needs a person but gives no reason",
			"write the reason after the type: **Execution:** human — <why a person must do it>")
	}
	return nil
}

// repoAction tells the author which repo names are valid.
func repoAction(registeredRepos []string) string {
	if len(registeredRepos) == 0 {
		return "register the repo with 'repo add', then set **Repo:** to its name"
	}
	return "set **Repo:** to exactly one of: " + strings.Join(registeredRepos, ", ")
}

// invalid builds the refusal for task t. The task is named by title and id so
// the author can find it; the resource is the id, or the title when the id is
// the thing missing.
func invalid(t Task, rule, next string) error {
	name := fmt.Sprintf("task %q", t.Title)
	resource := t.Title
	if t.ID != "" {
		name = fmt.Sprintf("task %q (%s)", t.Title, t.ID)
		resource = t.ID
	}
	return output.NewError(CodeTaskInvalid, name+" "+rule).
		WithResource(resource).
		WithNextAction(next)
}

// findCycle returns the tasks of the first dependency cycle found, in
// dependency order, with the first task repeated at the end; nil when the
// graph is acyclic. The search visits tasks in plan order, so the report is
// stable for a given plan.
func findCycle(tasks []Task, byID map[string]Task) []Task {
	const (
		white = iota
		grey
		black
	)
	colour := map[string]int{}
	var stack []Task
	var found []Task

	var visit func(t Task) bool
	visit = func(t Task) bool {
		colour[t.ID] = grey
		stack = append(stack, t)
		for _, dep := range t.DependsOn {
			next, ok := byID[dep]
			if !ok {
				continue
			}
			switch colour[dep] {
			case grey:
				for i, s := range stack {
					if s.ID == dep {
						found = append(append([]Task{}, stack[i:]...), next)
						return true
					}
				}
			case white:
				if visit(next) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		colour[t.ID] = black
		return false
	}

	for _, t := range tasks {
		if colour[t.ID] == white && visit(t) {
			return found
		}
	}
	return nil
}

// RequireTasks refuses a plan that does not describe its work as tasks. The
// message says what is missing, never how old the plan is.
func (p Plan) RequireTasks() error {
	if p.Format == FormatTasks {
		return nil
	}
	return output.NewError(CodeStructureInvalid,
		"the plan contains no task ids: its work is not written as '#### - [ ] Task:' headings with **Id:**, **Repo:**, **Depends on:** and **Execution:** lines").
		WithNextAction("rewrite the plan's work as tasks, each with an id from 'plan task-id', or implement the whole plan without selecting a task")
}
