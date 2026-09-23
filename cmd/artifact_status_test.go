package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

type artifactStatusEnvelope struct {
	Error          bool     `json:"error"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	DocumentStatus string   `json:"document_status"`
	CurrentStep    string   `json:"current_step"`
	CompletedSteps []string `json:"completed_steps"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	ClosedAt       string   `json:"closed_at"`
	Spec           string   `json:"spec"`
	Plan           string   `json:"plan"`
}

func writeArtifactStatusFile(t *testing.T, path, frontmatter string, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(frontmatter+"\n# Artifact\n"), 0o644))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
}

func TestSpecStatusNamedFinalArtifactReportsMetadataStatus(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	modTime := time.Date(2026, time.January, 4, 5, 6, 7, 0, time.UTC)
	writeArtifactStatusFile(t, filepath.Join(dir, ".spektacular", "specs", "000001_feature.md"), "---\ncreated_date: 2026-01-02\ndocument_status: final\nclosed_date: 2026-01-03\nplan: 000001_feature\n---\n", modTime)

	stdout, stderr, code := runRootCmd(t, "spec", "status", "000001_feature")
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	var got artifactStatusEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.False(t, got.Error)
	require.Equal(t, "spec", got.Kind)
	require.Equal(t, "000001_feature", got.Name)
	require.Equal(t, "final", got.DocumentStatus)
	require.Equal(t, "finished", got.CurrentStep)
	require.Empty(t, got.CompletedSteps)
	require.Equal(t, "2026-01-02T00:00:00Z", got.CreatedAt)
	require.Equal(t, "2026-01-03T00:00:00Z", got.ClosedAt)
	require.Equal(t, "2026-01-04T05:06:07Z", got.UpdatedAt)
	require.Equal(t, "000001_feature", got.Plan)
}

func TestSpecStatusNamedInProgressArtifactUsesMatchingState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeArtifactStatusFile(t, filepath.Join(dataDir, "specs", "000002_active.md"), "---\ncreated_date: 2026-01-02\ndocument_status: draft\n---\n", time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC))
	stateUpdated := time.Date(2026, time.January, 5, 6, 7, 8, 0, time.UTC)
	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new", "interview"},
		CreatedAt:      time.Date(2026, time.January, 2, 1, 0, 0, 0, time.UTC),
		UpdatedAt:      stateUpdated,
		Data:           map[string]any{"name": "000002_active"},
	})

	stdout, stderr, code := runRootCmd(t, "spec", "status", "000002_active")
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	var got artifactStatusEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, "overview", got.CurrentStep)
	require.Equal(t, []string{"new", "interview"}, got.CompletedSteps)
	require.Equal(t, "2026-01-05T06:07:08Z", got.UpdatedAt)
}

func TestPlanStatusNamedDraftArtifactNotInProgressUsesFileTime(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	modTime := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	writeArtifactStatusFile(t, filepath.Join(dataDir, "plans", "000003_plan", "plan.md"), "---\ncreated_date: 2026-02-01\ndocument_status: draft\nspec: 000003_plan\n---\n", modTime)
	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "plan",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new"},
		CreatedAt:      time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC),
		Data:           map[string]any{"name": "some-other-plan"},
	})

	stdout, stderr, code := runRootCmd(t, "plan", "status", "000003_plan")
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	var got artifactStatusEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, "plan", got.Kind)
	require.Equal(t, "000003_plan", got.Name)
	require.Equal(t, "draft", got.DocumentStatus)
	require.Equal(t, "", got.CurrentStep)
	require.Empty(t, got.CompletedSteps)
	require.Equal(t, "2026-02-03T04:05:06Z", got.UpdatedAt)
	require.Equal(t, "000003_plan", got.Spec)
}

func TestSpecStatusNamedMissingArtifactReturnsJSONError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	stdout, stderr, code := runRootCmd(t, "spec", "status", "missing")
	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "artifact_not_found", er.Code)
	require.Contains(t, er.Message, "spec")
	require.Contains(t, er.Message, "missing")
	require.Contains(t, er.NextAction, "spec file list")
}

func TestStatusNamedSchemaReportsArtifactShape(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	stdout, stderr, code := runRootCmd(t, "plan", "status", "000003_plan", "--schema")
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.Contains(t, schema.Output.Properties, "kind")
	require.Contains(t, schema.Output.Properties, "document_status")
	require.Contains(t, schema.Output.Properties, "closed_at")
	require.NotContains(t, schema.Output.Properties, "plan_path")
}

func TestSpecStatusWithoutNameKeepsWorkflowStatusShape(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new", "interview"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000004_resume"},
	})

	stdout, stderr, code := runRootCmd(t, "spec", "status")
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)
	require.JSONEq(t, `{
		"error": false,
		"spec_name": "000004_resume",
		"spec_path": "`+filepath.Join(dir, ".spektacular", "specs", "000004_resume.md")+`",
		"current_step": "overview",
		"completed_steps": ["new", "interview"],
		"total_steps": 11,
		"progress": "2/11",
		"steps": [
			{"name":"new","status":"completed"},
			{"name":"interview","status":"completed"},
			{"name":"overview","status":"current"},
			{"name":"requirements","status":"pending"},
			{"name":"acceptance_criteria","status":"pending"},
			{"name":"constraints","status":"pending"},
			{"name":"technical_approach","status":"pending"},
			{"name":"success_metrics","status":"pending"},
			{"name":"non_goals","status":"pending"},
			{"name":"verification","status":"pending"},
			{"name":"finished","status":"pending"}
		]
	}`, stdout)
}
