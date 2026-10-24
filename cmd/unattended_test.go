package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// unreviewedSpecFixture is a spec an unattended run finished: a draft marked
// unreviewed, carrying the review notes the run wrote.
const unreviewedSpecFixture = "---\ncreated_date: 2026-07-01\ndocument_status: draft\nunreviewed: true\n---\n\n" +
	epicTestSpecBody + "\n## Review Notes\n\n### Assumptions\n\nNo drafting assumptions were recorded.\n"

// spec new records the hand-off in the workflow state, so every later step,
// including one run after a resume, renders the unattended instructions.
func TestSpecNew_UnattendedIsRecordedInTheWorkflow(t *testing.T) {
	root := epicProject(t)

	result, err := runSpecNewForTest(t, "--data", `{"name":"handoff","unattended":true}`)
	require.NoError(t, err)
	require.Contains(t, result.Instruction, "## Running unattended")

	raw, err := os.ReadFile(filepath.Join(root, ".spektacular", "state.json"))
	require.NoError(t, err)
	var state workflow.State
	require.NoError(t, json.Unmarshal(raw, &state))
	require.Equal(t, true, state.Data["unattended"])
}

// Without the flag nothing changes: the run is interactive as before.
func TestSpecNew_AttendedByDefault(t *testing.T) {
	root := epicProject(t)

	result, err := runSpecNewForTest(t, "--data", `{"name":"plain"}`)
	require.NoError(t, err)
	require.NotContains(t, result.Instruction, "unattended")

	raw, err := os.ReadFile(filepath.Join(root, ".spektacular", "state.json"))
	require.NoError(t, err)
	var state workflow.State
	require.NoError(t, json.Unmarshal(raw, &state))
	require.NotContains(t, state.Data, "unattended")
}

func TestSpecNewSchemaDocumentsUnattended(t *testing.T) {
	schema := runSpecNewSchemaForTest(t)
	prop, ok := schema.Input.Properties["unattended"]
	require.True(t, ok, "spec new --schema must document unattended")
	require.Equal(t, "boolean", prop.Type)
	require.NotContains(t, schema.Input.Required, "unattended")
}

// plan new refuses a spec nobody has reviewed, writes nothing, and tells the
// agent how the review and sign-off go.
func TestPlanNew_RefusesAnUnreviewedSpec(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000001_handoff", unreviewedSpecFixture)
	before := snapshotTree(t, root)

	stdout, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"000001_handoff"}`)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.Equal(t, "spec_unreviewed", er.Code)
	require.Equal(t, "000001_handoff", er.Resource)
	require.Contains(t, er.NextAction, "## Review Notes")
	require.Contains(t, er.NextAction, "spec file set-document-status 000001_handoff --document-status final")
	require.Equal(t, before, snapshotTree(t, root), "a refused plan new must write nothing")
}

// Signing the spec off as final clears the mark, and plan new then starts.
func TestPlanNew_StartsOnceTheSpecIsSignedOff(t *testing.T) {
	root := epicProject(t)
	path := writeSpecFixture(t, root, "000001_handoff", unreviewedSpecFixture)

	_, _, code := runRootCmd(t, "spec", "file", "set-document-status", "000001_handoff", "--document-status", "final")
	require.Equal(t, 0, code)
	require.NotContains(t, frontmatterOf(t, path), "unreviewed: true")

	stdout, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"000001_handoff"}`)
	require.Equal(t, 0, code, stdout)
	require.NotContains(t, stdout, "spec_unreviewed")
}

// A rewrite during the review keeps the mark until the spec is signed off.
func TestSpecFileWrite_KeepsTheUnreviewedMark(t *testing.T) {
	root := epicProject(t)
	path := writeSpecFixture(t, root, "000001_handoff", unreviewedSpecFixture)
	staged := filepath.Join(root, "reviewed.md")
	require.NoError(t, os.WriteFile(staged, []byte(epicTestSpecBody), 0o644))

	_, _, code := runRootCmd(t, "spec", "file", "write", "000001_handoff", "--from", staged)
	require.Equal(t, 0, code)
	require.Contains(t, frontmatterOf(t, path), "unreviewed: true")
}

// spec file list flags an unreviewed spec, and only that one.
func TestSpecFileList_ReportsUnreviewedSpecs(t *testing.T) {
	root := epicProject(t)
	writeSpecFixture(t, root, "000001_handoff", unreviewedSpecFixture)
	writeSpecFixture(t, root, "000002_reviewed", epicTestSpecFixed)

	stdout, _, code := runRootCmd(t, "spec", "file", "list")
	require.Equal(t, 0, code)

	var resp struct {
		Files []map[string]any `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &resp))
	byName := map[string]map[string]any{}
	for _, f := range resp.Files {
		byName[f["name"].(string)] = f
	}
	require.Equal(t, true, byName["000001_handoff"]["unreviewed"])
	require.NotContains(t, byName["000002_reviewed"], "unreviewed")
}
