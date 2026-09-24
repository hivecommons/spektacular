package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

type artifactStatusEnvelope struct {
	Error          bool     `json:"error"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	ArtifactID     string   `json:"artifact_id"`
	DocumentStatus string   `json:"document_status"`
	CurrentStep    string   `json:"current_step"`
	CompletedSteps []string `json:"completed_steps"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	ModifiedAt     string   `json:"modified_at"`
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

// A closed artifact with no live workflow reports the store's mtime as
// modified_at and carries no updated_at at all: absence is the explicit
// signal that nothing is in progress, so a poller cannot mistake a checkout
// or a reformat for activity. The whole response is pinned so a field
// reappearing under either name is a visible diff.
func TestSpecStatusNamedFinalArtifactReportsMetadataStatus(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	modTime := time.Date(2026, time.January, 4, 5, 6, 7, 0, time.UTC)
	writeArtifactStatusFile(t, filepath.Join(dir, ".spektacular", "specs", "000001_feature.md"), "---\ncreated_date: 2026-01-02\ndocument_status: final\nclosed_date: 2026-01-03\nplan: 000001_feature\n---\n", modTime)

	stdout, stderr, code := runRootCmd(t, "spec", "status", "000001_feature")
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	require.JSONEq(t, `{
		"error": false,
		"kind": "spec",
		"name": "000001_feature",
		"artifact_id": "000001_feature",
		"document_status": "final",
		"current_step": "finished",
		"completed_steps": [],
		"created_at": "2026-01-02T00:00:00Z",
		"modified_at": "2026-01-04T05:06:07Z",
		"closed_at": "2026-01-03T00:00:00Z",
		"spec": "",
		"plan": "000001_feature"
	}`, stdout)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw))
	require.NotContains(t, raw, "updated_at", "no live workflow means no updated_at, not a fallback value")
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

	// updated_at is the workflow's own clock and modified_at is the store's;
	// the two are pinned side by side so neither can silently stand in for
	// the other.
	require.JSONEq(t, `{
		"error": false,
		"kind": "spec",
		"name": "000002_active",
		"artifact_id": "000002_active",
		"document_status": "draft",
		"current_step": "overview",
		"completed_steps": ["new", "interview"],
		"created_at": "2026-01-02T00:00:00Z",
		"updated_at": "2026-01-05T06:07:08Z",
		"modified_at": "2026-01-03T00:00:00Z",
		"closed_at": "",
		"spec": "",
		"plan": ""
	}`, stdout)
}

// A draft whose in-progress state belongs to a different artifact is not
// live: it reports modified_at from the store and omits updated_at, and its
// current_step stays empty because nothing closed it either.
func TestPlanStatusNamedDraftArtifactNotInProgressOmitsUpdatedAt(t *testing.T) {
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
	require.Equal(t, "000003_plan", got.ArtifactID)
	require.Equal(t, "draft", got.DocumentStatus)
	require.Equal(t, "", got.CurrentStep)
	require.Empty(t, got.CompletedSteps)
	require.Equal(t, "2026-02-03T04:05:06Z", got.ModifiedAt)
	require.Equal(t, "", got.UpdatedAt)
	require.Equal(t, "000003_plan", got.Spec)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw))
	require.NotContains(t, raw, "updated_at", "another artifact's workflow is not this artifact's activity")
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
	require.Contains(t, schema.Output.Properties, "artifact_id")
	require.Contains(t, schema.Output.Properties, "document_status")
	require.Contains(t, schema.Output.Properties, "closed_at")
	require.Contains(t, schema.Output.Properties, "updated_at")
	require.Contains(t, schema.Output.Properties, "modified_at")
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
