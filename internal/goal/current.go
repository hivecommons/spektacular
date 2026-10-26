package goal

import "github.com/hivecommons/spektacular/internal/plantask"

// Current is the item the goal check is for: the run's selected task in a
// single-task run, otherwise the first open item — a task, or a legacy phase
// in a plan written before tasks — that the outcome does not mark skipped. It
// mirrors the current-task rule the implement step templates state.
func Current(p plantask.Plan, outcome Outcome, selected string) (string, bool) {
	if selected != "" {
		return selected, true
	}
	if p.Format == plantask.FormatTasks {
		for _, t := range p.OpenTasks() {
			if outcome.Item(t.ID).State != StateSkipped {
				return t.ID, true
			}
		}
		return "", false
	}
	for _, ph := range p.OpenPhases() {
		if ph.Label != "" && outcome.Item(ph.Label).State != StateSkipped {
			return ph.Label, true
		}
	}
	return "", false
}

// OpenUnskipped counts the open items the outcome does not mark skipped: the
// work a whole-plan run still has to do.
func OpenUnskipped(p plantask.Plan, outcome Outcome) int {
	n := 0
	if p.Format == plantask.FormatTasks {
		for _, t := range p.OpenTasks() {
			if outcome.Item(t.ID).State != StateSkipped {
				n++
			}
		}
		return n
	}
	for _, ph := range p.OpenPhases() {
		if outcome.Item(ph.Label).State != StateSkipped {
			n++
		}
	}
	return n
}
