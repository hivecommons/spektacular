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
		want Point
	}{
		{"spec verification leads to a commit in workflow mode", "workflow", "spec", "verification", PointCompletion},
		{"spec verification leads to a commit in full mode", "full", "spec", "verification", PointCompletion},
		{"plan walkthrough leads to a commit in workflow mode", "workflow", "plan", "walkthrough", PointCompletion},
		{"plan walkthrough leads to a commit in full mode", "full", "plan", "walkthrough", PointCompletion},
		{"implement reconcile_spec leads to a commit in workflow mode", "workflow", "implement", "reconcile_spec", PointCompletion},
		{"implement reconcile_spec leads to a commit in full mode", "full", "implement", "reconcile_spec", PointCompletion},

		{"implement update_changelog leads to a milestone commit in full mode", "full", "implement", "update_changelog", PointMilestone},
		{"implement update_changelog leads nowhere in workflow mode", "workflow", "implement", "update_changelog", PointNone},
		{"implement update_changelog leads nowhere with mode off", "off", "implement", "update_changelog", PointNone},

		{"mode off never leads to a commit", "off", "implement", "reconcile_spec", PointNone},
		{"an ordinary step leads nowhere", "full", "spec", "overview", PointNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, LeadsToCommit(tc.mode, tc.kind, tc.from))
		})
	}
}
