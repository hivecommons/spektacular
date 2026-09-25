package autocommit

import "github.com/hivecommons/spektacular/internal/plantask"

// CompletedMilestones reports which milestones in a plan have every one of
// their work items ticked, ascending: tasks in a task-format plan, phases in
// a plan written before tasks. A milestone with nothing under it is never
// complete: there is nothing in it to have finished.
//
// The plan is read by the shared task reader, which scans only the
// Milestones & Tasks (or Milestones & Phases) section, so a checkbox
// elsewhere in the plan — an acceptance criterion, a changelog entry — can
// never be mistaken for a work item.
func CompletedMilestones(planMarkdown string) []int {
	return plantask.Parse([]byte(planMarkdown)).CompletedMilestones()
}

// DueMilestones returns the completed milestones that have not been committed
// yet, ascending — the ones a milestone commit at this point must name.
func DueMilestones(completed, committed []int) []int {
	already := make(map[int]bool, len(committed))
	for _, n := range committed {
		already[n] = true
	}
	var due []int
	for _, n := range completed {
		if !already[n] {
			due = append(due, n)
		}
	}
	return due
}
