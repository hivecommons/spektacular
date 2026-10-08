package autocommit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPointFor(t *testing.T) {
	cases := []struct {
		name string
		mode string
		kind string
		from string
		to   string
		want Point
	}{
		{"spec completion in workflow mode", "workflow", "spec", "split", "finished", PointCompletion},
		{"spec completion in full mode", "full", "spec", "split", "finished", PointCompletion},
		{"spec verification moving on to the split step is not a commit point", "full", "spec", "verification", "split", PointNone},
		{"plan completion in workflow mode", "workflow", "plan", "walkthrough", "finished", PointCompletion},
		{"plan completion in full mode", "full", "plan", "walkthrough", "finished", PointCompletion},
		{"implement completion in workflow mode", "workflow", "implement", "reconcile_spec", "finished", PointCompletion},
		{"implement completion in full mode", "full", "implement", "reconcile_spec", "finished", PointCompletion},
		{"a single-task run finishing early commits in workflow mode", "workflow", "implement", "update_changelog", "finished", PointCompletion},
		{"a single-task run finishing early commits in full mode", "full", "implement", "update_changelog", "finished", PointCompletion},
		{"a single-task run finishing early never commits with mode off", "off", "implement", "update_changelog", "finished", PointNone},

		{"a phase wrap-up looping back is a milestone point in full mode", "full", "implement", "update_changelog", "analyze", PointMilestone},
		{"the last phase wrap-up is a milestone point in full mode", "full", "implement", "update_changelog", "test_plan", PointMilestone},
		{"workflow mode never makes a milestone point looping back", "workflow", "implement", "update_changelog", "analyze", PointNone},
		{"workflow mode never makes a milestone point on the last phase", "workflow", "implement", "update_changelog", "test_plan", PointNone},
		{"mode off never makes a milestone point", "off", "implement", "update_changelog", "analyze", PointNone},

		{"mode off never commits", "off", "spec", "split", "finished", PointNone},
		{"absent mode never commits", "", "plan", "walkthrough", "finished", PointNone},

		{"an ordinary transition is not a commit point", "full", "spec", "overview", "requirements", PointNone},
		{"the right steps in the wrong workflow are not a commit point", "full", "plan", "split", "finished", PointNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PointFor(tc.mode, tc.kind, tc.from, tc.to))
		})
	}
}

func TestLeadsToCommit(t *testing.T) {
	cases := []struct {
		name string
		mode string
		kind string
		from string
		next string
		want Point
	}{
		{"spec split leads to a commit in workflow mode", "workflow", "spec", "split", "finished", PointCompletion},
		{"spec split leads to a commit in full mode", "full", "spec", "split", "finished", PointCompletion},
		{"spec verification leads nowhere now the split step follows it", "full", "spec", "verification", "split", PointNone},
		{"plan walkthrough leads to a commit in workflow mode", "workflow", "plan", "walkthrough", "finished", PointCompletion},
		{"plan walkthrough leads to a commit in full mode", "full", "plan", "walkthrough", "finished", PointCompletion},
		{"implement reconcile_spec leads to a commit in workflow mode", "workflow", "implement", "reconcile_spec", "finished", PointCompletion},
		{"implement reconcile_spec leads to a commit in full mode", "full", "implement", "reconcile_spec", "finished", PointCompletion},

		{"implement update_changelog leads to a milestone commit in full mode", "full", "implement", "update_changelog", "test_plan", PointMilestone},
		{"implement update_changelog leads nowhere in workflow mode", "workflow", "implement", "update_changelog", "test_plan", PointNone},
		{"implement update_changelog leads nowhere with mode off", "off", "implement", "update_changelog", "test_plan", PointNone},

		{"mode off never leads to a commit", "off", "implement", "reconcile_spec", "finished", PointNone},
		{"update_changelog finishing a single-task run leads to a completion commit in workflow mode", "workflow", "implement", "update_changelog", "finished", PointCompletion},
		{"update_changelog finishing a single-task run leads to a completion commit in full mode", "full", "implement", "update_changelog", "finished", PointCompletion},
		{"update_changelog looping in a whole-plan run is not a completion commit", "workflow", "implement", "update_changelog", "analyze", PointNone},
		{"an ordinary step leads nowhere", "full", "spec", "overview", "requirements", PointNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, LeadsToCommit(tc.mode, tc.kind, tc.from, tc.next))
		})
	}
}

// An implement run built in its own worktrees commits its code at completion
// even with automatic commits off; its milestones stay mode-gated, and every
// other run behaves exactly as PointFor says.
func TestPointForRun(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		kind      string
		from      string
		to        string
		worktrees bool
		want      Point
	}{
		{"off with worktrees commits at reconcile_spec completion", "off", "implement", "reconcile_spec", "finished", true, PointCompletion},
		{"off with worktrees commits when a single-task run finishes", "off", "implement", "update_changelog", "finished", true, PointCompletion},
		{"absent mode with worktrees commits at completion", "", "implement", "reconcile_spec", "finished", true, PointCompletion},
		{"off with worktrees makes no milestone point looping back", "off", "implement", "update_changelog", "analyze", true, PointNone},
		{"off with worktrees makes no milestone point on the last phase", "off", "implement", "update_changelog", "test_plan", true, PointNone},
		{"off with worktrees leaves an ordinary transition alone", "off", "implement", "analyze", "implement", true, PointNone},

		{"off without worktrees never commits at completion", "off", "implement", "reconcile_spec", "finished", false, PointNone},
		{"off without worktrees never commits a single-task finish", "off", "implement", "update_changelog", "finished", false, PointNone},

		{"workflow mode with worktrees commits at completion", "workflow", "implement", "reconcile_spec", "finished", true, PointCompletion},
		{"workflow mode with worktrees makes no milestone point", "workflow", "implement", "update_changelog", "test_plan", true, PointNone},
		{"full mode with worktrees commits at completion", "full", "implement", "reconcile_spec", "finished", true, PointCompletion},
		{"full mode with worktrees keeps its milestone point", "full", "implement", "update_changelog", "analyze", true, PointMilestone},

		{"a plan run with worktrees in off mode never commits", "off", "plan", "walkthrough", "finished", true, PointNone},
		{"a spec run with worktrees in off mode never commits", "off", "spec", "split", "finished", true, PointNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PointForRun(tc.mode, tc.kind, tc.from, tc.to, tc.worktrees))
		})
	}
}

func TestLeadsToCommitForRun(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		kind      string
		from      string
		next      string
		worktrees bool
		want      Point
	}{
		{"off with worktrees: reconcile_spec leads to a completion commit", "off", "implement", "reconcile_spec", "finished", true, PointCompletion},
		{"off with worktrees: a single-task update_changelog leads to a completion commit", "off", "implement", "update_changelog", "finished", true, PointCompletion},
		{"absent mode with worktrees: reconcile_spec leads to a completion commit", "", "implement", "reconcile_spec", "finished", true, PointCompletion},
		{"off with worktrees: update_changelog toward test_plan leads nowhere", "off", "implement", "update_changelog", "test_plan", true, PointNone},
		{"off with worktrees: update_changelog looping back leads nowhere", "off", "implement", "update_changelog", "analyze", true, PointNone},

		{"off without worktrees: reconcile_spec leads nowhere", "off", "implement", "reconcile_spec", "finished", false, PointNone},
		{"off without worktrees: a single-task finish leads nowhere", "off", "implement", "update_changelog", "finished", false, PointNone},

		{"workflow mode with worktrees: reconcile_spec leads to a completion commit", "workflow", "implement", "reconcile_spec", "finished", true, PointCompletion},
		{"workflow mode with worktrees: update_changelog toward test_plan leads nowhere", "workflow", "implement", "update_changelog", "test_plan", true, PointNone},
		{"full mode with worktrees: update_changelog toward test_plan leads to a milestone commit", "full", "implement", "update_changelog", "test_plan", true, PointMilestone},
		{"full mode with worktrees: reconcile_spec leads to a completion commit", "full", "implement", "reconcile_spec", "finished", true, PointCompletion},

		{"a plan run with worktrees in off mode leads nowhere", "off", "plan", "walkthrough", "finished", true, PointNone},
		{"a spec run with worktrees in off mode leads nowhere", "off", "spec", "split", "finished", true, PointNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, LeadsToCommitForRun(tc.mode, tc.kind, tc.from, tc.next, tc.worktrees))
		})
	}
}

// Only an implement run with worktrees under auto_commit off (or absent)
// commits code alone.
func TestCodeOnly(t *testing.T) {
	cases := []struct {
		mode      string
		kind      string
		worktrees bool
		want      bool
	}{
		{"off", "implement", true, true},
		{"", "implement", true, true},
		{"off", "implement", false, false},
		{"", "implement", false, false},
		{"workflow", "implement", true, false},
		{"full", "implement", true, false},
		{"off", "plan", true, false},
		{"off", "spec", true, false},
	}

	for _, tc := range cases {
		require.Equalf(t, tc.want, CodeOnly(tc.mode, tc.kind, tc.worktrees),
			"CodeOnly(%q, %q, %v)", tc.mode, tc.kind, tc.worktrees)
	}
}
