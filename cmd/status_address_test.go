package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/stretchr/testify/require"
)

const addressPlanBody = "# Plan\n\n## Milestones & Phases\n\n#### - [ ] Phase 1.1: Work\n"

// runStatusIn runs `<kind> status` in the current project and decodes it.
func runStatusIn(t *testing.T, kind string) (string, map[string]any) {
	t.Helper()
	stdout, _, code := runRootCmd(t, kind, "status")
	require.Equal(t, 0, code, stdout)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return stdout, got
}

// Both workflow-status forms address the plan by name and document, and
// report its location relative to the folder holding config.yaml rather than
// as a host path.
func TestWorkflowStatus_AddressesPlanByNameAndRelativePath(t *testing.T) {
	for _, kind := range []string{"plan", "implement"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "")
			if kind == "implement" {
				_, code := writePlanDoc(t, taskPlanName, "plan", addressPlanBody)
				require.Equal(t, 0, code)
			}
			_, _, code := runRootCmd(t, kind, "new", "--data", `{"name":"`+taskPlanName+`"}`)
			require.Equal(t, 0, code)

			stdout, got := runStatusIn(t, kind)
			require.Equal(t, "20260709000000-feature", got["plan_name"])
			require.Equal(t, "plan", got["plan_document"])
			require.Equal(t, "plans/20260709000000-feature/plan.md", got["plan_path"])
			require.NotContains(t, stdout, dir)
		})
	}
}

func TestWorkflowStatus_SchemaDescribesPlanAddress(t *testing.T) {
	for _, kind := range []string{"plan", "implement"} {
		t.Run(kind, func(t *testing.T) {
			stdout, _, code := runRootCmd(t, kind, "status", "--schema")
			require.Equal(t, 0, code, stdout)
			var schema commandSchema
			require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
			require.Contains(t, schema.Output.Properties, "plan_document")
			require.Contains(t, schema.Output.Properties["plan_path"].Description, "relative")
		})
	}
}

// The name a workflow records in its state is enough, on its own, to read
// every document the feature owns.
func TestWorkflowStateName_AddressesEveryArtifact(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	src := func(body string) string {
		p := filepath.Join(t.TempDir(), "source.md")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		return p
	}
	for _, args := range [][]string{
		{"spec", "file", "write", taskPlanName, "--from", src("spec body")},
		{"plan", "file", "write", taskPlanName, "plan", "--from", src(addressPlanBody)},
		{"changelog", "file", "write", taskPlanName, "--from", src("changelog body")},
		{"implement", "new", "--data", `{"name":"` + taskPlanName + `"}`},
	} {
		stdout, _, code := runRootCmd(t, args...)
		require.Equal(t, 0, code, stdout)
	}

	raw, err := os.ReadFile(stateFilePath(filepath.Join(dir, ".spektacular")))
	require.NoError(t, err)
	var state struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &state))
	name := state.Data.Name

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"spec", "file", "read", name}, "spec body"},
		{[]string{"plan", "file", "read", name, "plan"}, addressPlanBody},
		{[]string{"changelog", "file", "read", name}, "changelog body"},
	} {
		stdout, _, code := runRootCmd(t, tc.args...)
		require.Equal(t, 0, code, stdout)
		_, body, err := metadata.Split([]byte(stdout))
		require.NoError(t, err)
		require.Equal(t, tc.want, string(body), tc.args)
	}
}

// runWorkflowStep runs one workflow command and decodes its result.
func runWorkflowStep(t *testing.T, args ...string) (string, map[string]any) {
	t.Helper()
	stdout, _, code := runRootCmd(t, args...)
	require.Equal(t, 0, code, stdout)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return stdout, got
}

// The new and goto results of every document workflow report the primary
// document's location relative to the folder holding config.yaml, never a
// host path; plan and implement also name the plan document by address.
func TestWorkflowResults_ReportConfigRelativeLocation(t *testing.T) {
	t.Run("spec", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")

		for _, args := range [][]string{
			{"spec", "new", "--data", `{"name":"billing"}`},
			{"spec", "goto", "--data", `{"step":"interview"}`},
			{"spec", "status"},
		} {
			stdout, got := runWorkflowStep(t, args...)
			require.Equal(t, "000001_billing", got["spec_name"], args)
			require.Equal(t, "specs/000001_billing.md", got["spec_path"], args)
			require.NotContains(t, stdout, dir, args)
		}
	})

	for _, tc := range []struct {
		kind     string
		nextStep string
	}{
		{"plan", "overview"},
		{"implement", "analyze"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "")
			if tc.kind == "implement" {
				_, code := writePlanDoc(t, taskPlanName, "plan", addressPlanBody)
				require.Equal(t, 0, code)
			}

			for _, args := range [][]string{
				{tc.kind, "new", "--data", `{"name":"` + taskPlanName + `"}`},
				{tc.kind, "goto", "--data", `{"step":"` + tc.nextStep + `"}`},
			} {
				stdout, got := runWorkflowStep(t, args...)
				require.Equal(t, "20260709000000-feature", got["plan_name"], args)
				require.Equal(t, "plan", got["plan_document"], args)
				require.Equal(t, "plans/20260709000000-feature/plan.md", got["plan_path"], args)
				require.NotContains(t, stdout, dir, args)
			}
		})
	}
}

// The new-result schemas describe the reported location as relative, and
// plan and implement document the plan_document address field.
func TestWorkflowNew_SchemaDescribesRelativeLocation(t *testing.T) {
	for _, tc := range []struct {
		kind, pathField string
		hasDocument     bool
	}{
		{"spec", "spec_path", false},
		{"plan", "plan_path", true},
		{"implement", "plan_path", true},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			stdout, _, code := runRootCmd(t, tc.kind, "new", "--schema")
			require.Equal(t, 0, code, stdout)
			var schema commandSchema
			require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
			require.Contains(t, schema.Output.Properties[tc.pathField].Description, "relative")
			if tc.hasDocument {
				require.Contains(t, schema.Output.Properties, "plan_document")
			}
		})
	}
}
