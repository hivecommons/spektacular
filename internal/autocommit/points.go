package autocommit

import "github.com/hivecommons/spektacular/internal/config"

// Point classifies a workflow transition for committing.
type Point string

const (
	// PointNone means the transition is not a commit point.
	PointNone Point = ""
	// PointCompletion is the commit made when a workflow finishes.
	PointCompletion Point = "completion"
	// PointMilestone is the commit made when an implementation finishes one
	// of its plan's milestones. Only full mode produces it, and the
	// transitions that can are registered in milestonePoints.
	PointMilestone Point = "milestone"
)

// transition identifies one workflow move: a workflow kind and the step
// names it goes from and to.
type transition struct {
	kind string
	from string
	to   string
}

// completionPoints are the transitions that end a workflow. They commit in
// both workflow and full mode.
var completionPoints = map[transition]bool{
	{kind: "spec", from: "verification", to: "finished"}:        true,
	{kind: "plan", from: "walkthrough", to: "finished"}:         true,
	{kind: "implement", from: "reconcile_spec", to: "finished"}: true,
	// A single-task run that leaves tasks open for later runs finishes
	// straight from update_changelog, and its work is committed there.
	{kind: "implement", from: "update_changelog", to: "finished"}: true,
}

// milestonePoints are the transitions that wrap up an implementation phase
// and so can carry a milestone commit in full mode. Both are the moves out of
// update_changelog: back to analyze for another phase, or on to test_plan
// when the last phase is done.
//
// They are only *candidates*. Whether a commit is actually due depends on the
// plan's checkboxes — see CompletedMilestones — because most phase wrap-ups
// land in the middle of a milestone and must ask for nothing.
var milestonePoints = map[transition]bool{
	{kind: "implement", from: "update_changelog", to: "analyze"}:   true,
	{kind: "implement", from: "update_changelog", to: "test_plan"}: true,
}

// ReferencedSteps reports every step name the commit-point tables mention,
// grouped by workflow kind. The tables are keyed on step names, so a step
// renamed in a workflow would silently orphan its commit point; a test pins
// what this returns against the real step lists to stop that.
//
// It is exported for that test alone: the pin has to live outside this
// package, because importing the step packages from here would cycle back
// through the step renderer.
func ReferencedSteps() map[string][]string {
	byKind := map[string][]string{}
	seen := map[string]bool{}
	add := func(kind, step string) {
		if key := kind + "\x00" + step; !seen[key] {
			seen[key] = true
			byKind[kind] = append(byKind[kind], step)
		}
	}
	for _, points := range []map[transition]bool{completionPoints, milestonePoints} {
		for t := range points {
			add(t.kind, t.from)
			add(t.kind, t.to)
		}
	}
	return byKind
}

// PointFor classifies the transition from fromStep to toStep in the named
// workflow kind under the given auto_commit mode. Mode off never produces a
// point, and mode workflow never produces a milestone point.
func PointFor(mode, kind, fromStep, toStep string) Point {
	if mode == "" || mode == config.AutoCommitOff {
		return PointNone
	}
	t := transition{kind: kind, from: fromStep, to: toStep}
	if completionPoints[t] {
		return PointCompletion
	}
	if mode == config.AutoCommitFull && milestonePoints[t] {
		return PointMilestone
	}
	return PointNone
}

// LeadsToCommit classifies the step currently being rendered by where it can
// go next, so the step renderer can tell an agent to prepare a commit
// message before it runs the transition. nextStep is the exit the step's
// instruction names: a completion point counts only when it is that exit,
// because a step such as update_changelog finishes a single-task run but
// loops or continues in a whole-plan run. Milestone points count for any
// exit, since the agent picks between them. Where a step leads to both kinds
// of point, completion wins, since that is the stronger requirement.
func LeadsToCommit(mode, kind, fromStep, nextStep string) Point {
	if mode == "" || mode == config.AutoCommitOff {
		return PointNone
	}
	for t := range completionPoints {
		if t.kind == kind && t.from == fromStep && t.to == nextStep {
			return PointCompletion
		}
	}
	if mode == config.AutoCommitFull {
		for t := range milestonePoints {
			if t.kind == kind && t.from == fromStep {
				return PointMilestone
			}
		}
	}
	return PointNone
}
