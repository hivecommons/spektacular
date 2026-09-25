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
		{"spec completion in workflow mode", "workflow", "spec", "verification", "finished", PointCompletion},
		{"spec completion in full mode", "full", "spec", "verification", "finished", PointCompletion},
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

		{"mode off never commits", "off", "spec", "verification", "finished", PointNone},
		{"absent mode never commits", "", "plan", "walkthrough", "finished", PointNone},

		{"an ordinary transition is not a commit point", "full", "spec", "overview", "requirements", PointNone},
		{"the right steps in the wrong workflow are not a commit point", "full", "plan", "verification", "finished", PointNone},
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
		{"spec verification leads to a commit in workflow mode", "workflow", "spec", "verification", "finished", PointCompletion},
		{"spec verification leads to a commit in full mode", "full", "spec", "verification", "finished", PointCompletion},
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
