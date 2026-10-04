package epic

import (
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/depgraph"
	"github.com/hivecommons/spektacular/internal/output"
)

// CodeInvalid refuses an epic whose specs list breaks a graph rule.
const CodeInvalid = "epic_invalid"

// invalidNextAction is where every graph refusal points: the specs list is
// only ever supplied through --data on epic write or in a split description.
const invalidNextAction = "fix the specs list (each entry needs a name and a depends_on list, [] when it has none; parallel_with may name only other specs in the epic) and re-run the epic write or epic split; to let two specs run side by side, use epic order with unorder"

// Validate applies the epic graph rules to specs and returns the first broken
// rule as a structured error naming the offending spec. Rules are checked in
// the order the plan-task validator uses: each entry on its own, then
// duplicates, then unknown dependencies, then cycles, so a cycle is only ever
// reported between specs the epic lists.
func Validate(specs []EpicSpec) error {
	seen := make(map[string]bool, len(specs))
	for i, s := range specs {
		switch {
		case s.Name == "":
			return invalid(fmt.Sprintf("specs[%d]", i), fmt.Sprintf("entry %d of the epic's specs has no name", i+1))
		case s.DependsOn == nil:
			return invalid(s.Name, fmt.Sprintf("spec %q has no depends_on list", s.Name))
		case seen[s.Name]:
			return invalid(s.Name, fmt.Sprintf("spec %q is listed more than once", s.Name))
		}
		seen[s.Name] = true
	}

	for _, s := range specs {
		for _, dep := range s.DependsOn {
			if !seen[dep] {
				return invalid(s.Name, fmt.Sprintf("spec %q depends on %q, which is not a spec in this epic", s.Name, dep))
			}
		}
		for _, other := range s.ParallelWith {
			switch {
			case other == s.Name:
				return invalid(s.Name, fmt.Sprintf("spec %q names itself in parallel_with", s.Name))
			case !seen[other]:
				return invalid(s.Name, fmt.Sprintf("spec %q is parallel_with %q, which is not a spec in this epic", s.Name, other))
			}
		}
	}

	order := make([]string, len(specs))
	deps := make(map[string][]string, len(specs))
	for i, s := range specs {
		order[i] = s.Name
		deps[s.Name] = s.DependsOn
	}
	if cycle := depgraph.FindCycle(order, deps); cycle != nil {
		return invalid(cycle[0], fmt.Sprintf("spec %q is part of a dependency cycle: %s", cycle[0], strings.Join(cycle, " -> ")))
	}
	return nil
}

func invalid(resource, message string) error {
	return output.NewError(CodeInvalid, message).
		WithResource(resource).
		WithNextAction(invalidNextAction)
}
