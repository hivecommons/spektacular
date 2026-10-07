// Package status builds the one report `status` gives about a piece of work:
// the epic a spec belongs to (if any), every spec in it, each spec's plan and
// that plan's tasks. A spec's state is derived here and never stored, and the
// implement dependency check and the completed-epic guard classify specs
// through the same function, so the three never disagree.
package status

import (
	"fmt"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/store"
)

// SpecState is a spec's derived state.
type SpecState string

// The spec states, in the order Classify checks them.
const (
	// StateMissing: the spec is named but cannot be read.
	StateMissing SpecState = "missing"
	// StateStale: the plan is stale under plan.strict_spec_changes.
	StateStale SpecState = "stale"
	// StateSpecified: the spec is written and no plan exists.
	StateSpecified SpecState = "specified"
	// StatePlanned: a plan exists with none of its work complete.
	StatePlanned SpecState = "planned"
	// StateInProgress: a plan exists with some but not all work complete.
	StateInProgress SpecState = "in_progress"
	// StateImplemented: a plan exists with every work item complete.
	StateImplemented SpecState = "implemented"
)

// TaskCounts is a plan's progress: work items complete of total. For a
// task-format plan the items are tasks; for an older plan they are phases.
type TaskCounts struct {
	Completed int
	Total     int
}

// PlanFacts is what Classify needs to know about a spec's plan.
type PlanFacts struct {
	// Exists is false when the spec has no plan.
	Exists bool
	// Stale is true when the plan is stale under plan.strict_spec_changes.
	Stale bool
	// Parsed is the plan's parsed work.
	Parsed plantask.Plan
}

// Classify derives a spec's state, checking in the design's order: missing,
// stale, specified, planned, in progress, implemented. specReadable is false
// when the spec cannot be read. A plan with no checkboxes at all is planned:
// with nothing to complete it is never implemented.
func Classify(specReadable bool, plan PlanFacts) (SpecState, TaskCounts) {
	var counts TaskCounts
	if plan.Exists {
		counts.Total = plan.Parsed.Items()
		counts.Completed = counts.Total - plan.Parsed.OpenItems()
	}
	switch {
	case !specReadable:
		return StateMissing, counts
	case !plan.Exists:
		return StateSpecified, counts
	case plan.Stale:
		return StateStale, counts
	case counts.Total == 0 || counts.Completed == 0:
		return StatePlanned, counts
	case counts.Completed < counts.Total:
		return StateInProgress, counts
	default:
		return StateImplemented, counts
	}
}

// Describe words a state for a sentence such as "X depends on Y, which is
// <Describe>": "in progress (2/5 tasks complete)", "planned, not started",
// "unplanned", "stale", "implemented" or "missing".
func Describe(state SpecState, counts TaskCounts) string {
	switch state {
	case StateMissing:
		return "missing"
	case StateStale:
		return "stale"
	case StateSpecified:
		return "unplanned"
	case StatePlanned:
		return "planned, not started"
	case StateInProgress:
		return fmt.Sprintf("in progress (%d/%d tasks complete)", counts.Completed, counts.Total)
	case StateImplemented:
		return "implemented"
	}
	return string(state)
}

// DescribeDependency words a dependency's state as Describe does, except
// that an implemented dependency whose worktrees are not merged yet is "implemented but not yet
// merged".
func DescribeDependency(state SpecState, counts TaskCounts, unmerged bool) string {
	if state == StateImplemented && unmerged {
		return "implemented but not yet merged"
	}
	return Describe(state, counts)
}

// Label is a state as the readable tree shows it: in_progress reads as
// "in progress", every other state as itself.
func Label(state SpecState) string {
	if state == StateInProgress {
		return "in progress"
	}
	return string(state)
}

// DocumentStatus is a plan's reported document status: the stored one, or
// stale when plan.strict_spec_changes is on and its spec changed after it.
// Every view that reports a plan's document status goes through it, so they
// always agree.
func DocumentStatus(cfg config.Config, st store.Reader, planName string, fm *metadata.Metadata) metadata.DocumentStatus {
	if fm == nil {
		return ""
	}
	if cfg.Plan.StrictSpecChanges && PlanIsStale(cfg, st, planName, fm) {
		return metadata.StatusStale
	}
	return fm.DocumentStatus
}

// PlanIsStale reports whether a final plan's spec was modified after the
// plan. The spec is the plan's `spec` frontmatter, or the plan's own name. It
// is the bare comparison: callers apply plan.strict_spec_changes themselves.
func PlanIsStale(cfg config.Config, st store.Reader, planName string, fm *metadata.Metadata) bool {
	if fm == nil || fm.DocumentStatus != metadata.StatusFinal {
		return false
	}
	specName := fm.Spec
	if specName == "" {
		specName = planName
	}
	planInfo, err := st.Stat(planPath(cfg, planName))
	if err != nil {
		return false
	}
	specInfo, err := st.Stat(specPath(cfg, specName))
	if err != nil {
		return false
	}
	return specInfo.ModTime.After(planInfo.ModTime)
}
