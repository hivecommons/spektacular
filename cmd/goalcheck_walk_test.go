package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/goal"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/steps/implement"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// passGoalCheck records a goal check judging every criterion of the plan's
// current item met, as the reviewer of a finished task would, so a walk
// through check_goal can go on to update_plan. It works in the project the
// test runs in.
func passGoalCheck(t *testing.T, plan string) {
	t.Helper()
	cfg, err := loadConfig()
	require.NoError(t, err)
	root, err := projectRoot()
	require.NoError(t, err)
	st := store.NewSourceStore(root, "project")
	planBody, err := st.Read(implement.PlanFilePath(cfg.Plan.Config.Directory, plan))
	require.NoError(t, err)
	contextBody, _ := st.Read(implement.ContextFilePath(cfg.Plan.Config.Directory, plan))
	outcome, err := goal.Load(st, cfg.Plan.Config.Directory, plan)
	require.NoError(t, err)
	item, ok := goal.Current(plantask.Parse(planBody), outcome, runSelectedTask(root, plan))
	require.True(t, ok, "plan %s has no current item to check", plan)
	g, err := goal.Build(planBody, contextBody, item, repoNames(cfg))
	require.NoError(t, err)

	verdicts := make([]goal.Verdict, len(g.Criteria))
	for i := range g.Criteria {
		verdicts[i] = goal.Verdict{Criterion: i + 1, Verdict: goal.VerdictMet, Evidence: "test walk"}
	}
	raw, err := json.Marshal(map[string]any{"verdicts": verdicts})
	require.NoError(t, err)
	reply := filepath.Join(t.TempDir(), "reply.json")
	require.NoError(t, os.WriteFile(reply, raw, 0o644))

	data, err := json.Marshal(map[string]string{"name": plan, "item": item})
	require.NoError(t, err)
	stdout, _, code := runRootCmd(t, "implement", "goal", "record", "--data", string(data), "--from", reply)
	require.Equalf(t, 0, code, "recording the goal check failed: %s", stdout)
}

// sharedImplementPlan is the plan the shared-slot implement run is for.
func sharedImplementPlan(t *testing.T) string {
	t.Helper()
	dir, err := dataDir()
	require.NoError(t, err)
	st, err := readState(stateFilePath(dir))
	require.NoError(t, err)
	require.NotNil(t, st, "no implement run in the shared slot")
	name, _ := st.Data["name"].(string)
	return name
}
