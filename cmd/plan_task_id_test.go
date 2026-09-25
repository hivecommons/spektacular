package cmd

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/hivecommons/spektacular/internal/identifier"
	"github.com/stretchr/testify/require"
)

func TestPlanTaskID_DefaultIssuesUUID(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	stdout, _, code := runRootCmd(t, "plan", "task-id")
	require.Equal(t, 0, code, stdout)
	var got struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), got.ID)
}

func TestPlanTaskID_UnknownProviderFailsOnlyTheRequest(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  task_id:\n    provider: nope\n")

	stdout, _, code := runRootCmd(t, "plan", "task-id")
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, identifier.CodeTaskIDProviderUnknown, er.Code)
	require.Contains(t, er.Message, "nope")
	require.Contains(t, er.NextAction, "uuid")

	// The setting does not stop the rest of the CLI from working.
	stdout, code = writePlanDoc(t, taskPlanName, "plan.md", validTaskPlan())
	require.Equal(t, 0, code, stdout)
	stdout, _, code = runRootCmd(t, "plan", "file", "list")
	require.Equal(t, 0, code, stdout)
}
