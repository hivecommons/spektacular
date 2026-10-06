package implement

import (
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// The changelog steps record an implement run that started past unmet
// dependencies: `implement new` stores them as workflow data
// "dependency_override", and update_changelog and update_feature_changelog
// tell the agent to note them under Deviations, naming each and its state.
// The heading renders once however many dependencies there are, and nothing
// renders when the run overrode nothing.

const overrideHeading = "### Record the dependency override"

// overrideShapes are the two shapes the workflow data holds the list in: as
// the command set it on a fresh run, and after a state.json round trip.
var overrideShapes = map[string]any{
	"fresh": []map[string]any{
		{"name": "dep-b", "state": "unplanned"},
		{"name": "dep-a", "state": "in progress (2/5 tasks complete)"},
	},
	"from state.json": []any{
		map[string]any{"name": "dep-b", "state": "unplanned"},
		map[string]any{"name": "dep-a", "state": "in progress (2/5 tasks complete)"},
	},
}

func changelogCallbacks() map[string]workflow.StepCallback {
	return map[string]workflow.StepCallback{
		"update_changelog":         updateChangelog(),
		"update_feature_changelog": updateFeatureChangelog(),
	}
}

func TestChangelogStepsRecordTheDependencyOverride(t *testing.T) {
	for step, cb := range changelogCallbacks() {
		for shape, override := range overrideShapes {
			t.Run(step+"/"+shape, func(t *testing.T) {
				out := renderStepWithData(t, cb, map[string]any{"name": "test", "dependency_override": override})
				require.Equal(t, 1, strings.Count(out, overrideHeading), "the heading renders once")
				require.Contains(t, out, "**Deviations")
				require.Contains(t, out, "implementation started before these dependencies were implemented")
				require.Contains(t, out, "- `dep-b` — unplanned")
				require.Contains(t, out, "- `dep-a` — in progress (2/5 tasks complete)")
			})
		}
	}
}

func TestChangelogStepsRenderNoOverrideWhenAbsent(t *testing.T) {
	for step, cb := range changelogCallbacks() {
		for name, values := range map[string]map[string]any{
			"absent": {"name": "test"},
			"empty":  {"name": "test", "dependency_override": []any{}},
		} {
			t.Run(step+"/"+name, func(t *testing.T) {
				out := renderStepWithData(t, cb, values)
				require.NotContains(t, out, overrideHeading)
				require.NotContains(t, out, "these dependencies")
			})
		}
	}
}
