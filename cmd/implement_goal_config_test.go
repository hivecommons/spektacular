package cmd

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// Goal-check configuration criterion 4: implement new and implement goto both
// build their workflow config through implementWorkflowConfig, which carries
// implement.goal_check.enabled into the run.
func TestImplementWorkflowConfig_CarriesGoalCheckSwitch(t *testing.T) {
	cfg := config.NewDefault()
	roots := []workflow.CodeRoot{{Repo: "app", Root: "/tmp/app"}}

	on := implementWorkflowConfig(cfg, false, roots)
	require.True(t, on.GoalCheck, "the default configuration runs the check")
	require.Equal(t, "implement", on.Kind)
	require.Equal(t, roots, on.CodeRoots)

	cfg.Implement.GoalCheck.Enabled = false
	off := implementWorkflowConfig(cfg, true, nil)
	require.False(t, off.GoalCheck)
	require.True(t, off.DryRun)
}
