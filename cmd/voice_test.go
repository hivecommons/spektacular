package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/publish/github/githubtest"
)

// voiceSummary is what a recorder's "spec-shaped" summary template yields:
// headings that match the scaffold, bullets that become checklist items.
const voiceSummary = `# Cluster drift alerts

## Summary
Page the on-call when a cluster drifts from its desired state, so drift is fixed before users notice it.

## Requirements
- Detect drift: compare each cluster's live state with its desired state every minute
- Page on-call: route a drift alert through the existing alert channel once per cluster

## Acceptance criteria
- Detect drift: a cluster changed by hand shows as drifted in the console within 60 seconds
- Page on-call: exactly one alert fires per drifted cluster until it is reconciled

## Out of scope
- auto-remediation
`

func voiceProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	writeSpecCommandConfig(t, root, "")
	t.Setenv("GITHUB_TOKEN", githubtest.Token)
	t.Setenv("GH_TOKEN", "")
	return root
}

func writeVoiceNote(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestVoicePublishesASpecShapedNoteAsAnRFCWithChildren(t *testing.T) {
	root := voiceProject(t)
	gh := githubtest.New(t)
	t.Setenv("SPEKTACULAR_GITHUB_API", gh.URL)
	note := writeVoiceNote(t, root, "memo.md", voiceSummary)

	stdout, stderr, code := runRootCmd(t, "voice", "--from", "file", "--path", note, "--publish", "--repo", "acme/demo", "--format", "json")
	require.Empty(t, stderr)
	require.Equal(t, 0, code, stdout)
	var res struct {
		RFCURL  string   `json:"rfc_url"`
		Created []string `json:"created"`
		Voice   struct {
			SpecName string   `json:"spec_name"`
			SpecPath string   `json:"spec_path"`
			URI      string   `json:"uri"`
			RFCURL   string   `json:"rfc_url"`
			Children []string `json:"children"`
		} `json:"voice"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	require.Equal(t, "https://github.com/acme/demo/issues/1", res.RFCURL)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created, "one child per requirement")
	require.True(t, strings.HasSuffix(res.Voice.SpecName, "-cluster-drift-alerts"), res.Voice.SpecName)
	require.Equal(t, res.RFCURL, res.Voice.RFCURL)
	require.Equal(t, []string{"https://github.com/acme/demo/issues/2", "https://github.com/acme/demo/issues/3"}, res.Voice.Children)
	require.True(t, strings.HasPrefix(res.Voice.URI, "file://"), res.Voice.URI)
	require.Contains(t, gh.Issues[2].Title, "Detect drift")
	require.Contains(t, gh.Issues[2].Body, "within 60 seconds", "the traced acceptance criterion travels with the child")

	raw, err := os.ReadFile(filepath.Join(root, res.Voice.SpecPath))
	require.NoError(t, err)
	fm, body, err := metadata.Split(raw)
	require.NoError(t, err)
	require.Equal(t, metadata.StatusFinal, fm.DocumentStatus)
	require.Len(t, fm.Sources, 1)
	require.Equal(t, res.Voice.URI, fm.Sources[0].URI)
	require.Len(t, fm.Published, 1)
	require.Contains(t, string(body), "# Feature: Cluster drift alerts")
	require.Contains(t, string(body), "- [ ] **Detect drift**: compare each cluster's live state")
	require.Contains(t, string(body), "## Non-Goals\n\n- auto-remediation")

	staged := filepath.Join(root, ".spektacular", "work", res.Voice.SpecName, "voice")
	for _, f := range []string{"note.md", "draft.md"} {
		_, err := os.Stat(filepath.Join(staged, f))
		require.NoError(t, err, f)
	}
}

func TestVoiceRefusesToPublishANoteWithoutRequirements(t *testing.T) {
	root := voiceProject(t)
	note := writeVoiceNote(t, root, "memo.txt", "We should alert on cluster drift and page whoever is on call.")

	stdout, _, code := runRootCmd(t, "voice", "--from", "file", "--path", note, "--publish", "--repo", "acme/demo")
	resp := requirePublishRefusal(t, stdout, code, "voice_not_spec_shaped")
	require.Contains(t, resp.Message, "Requirements or Acceptance Criteria")
	require.Contains(t, resp.NextAction, "re-run without --publish")
	_, err := os.Stat(filepath.Join(root, ".spektacular", "specs"))
	require.True(t, os.IsNotExist(err), "a refusal writes no spec")
}

func TestVoiceWithoutPublishStagesTheNoteAndStartsTheSpecWorkflow(t *testing.T) {
	root := voiceProject(t)
	note := writeVoiceNote(t, root, "memo.txt", "We should alert on cluster drift and page whoever is on call.")

	stdout, stderr, code := runRootCmd(t, "voice", "--from", "file", "--path", note, "--name", "drift")
	require.Empty(t, stderr)
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, `"step": "new"`, "the spec workflow has started")

	staged := filepath.Join(root, ".spektacular", "tmp", "voice", "drift")
	transcript, err := os.ReadFile(filepath.Join(staged, "transcript.md"))
	require.NoError(t, err)
	require.Contains(t, string(transcript), "alert on cluster drift")
	_, err = os.Stat(filepath.Join(staged, "draft.md"))
	require.NoError(t, err)

	state, err := os.ReadFile(filepath.Join(root, ".spektacular", "state.json"))
	require.NoError(t, err)
	require.Contains(t, string(state), `"material": ".spektacular/tmp/voice/drift"`)
	require.Contains(t, string(state), `"uri": "file://`)

	// The interview that follows tells the agent to read the staged folder.
	stdout, _, code = runRootCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "Read the material already fetched to `.spektacular/tmp/voice/drift`")
	require.NotContains(t, stdout, "Fetch each source again with your own tools")
}

func TestVoiceFromStdinDryRunPreviewsTheChildren(t *testing.T) {
	voiceProject(t)
	resetRootCmd(t)
	rootCmd.SetIn(strings.NewReader(voiceSummary))
	t.Cleanup(func() { rootCmd.SetIn(nil) })
	stdout, _, code := runRootCmd(t, "voice", "--from", "stdin", "--publish", "--dry-run", "--format", "json")
	require.Equal(t, 0, code, stdout)
	var res struct {
		DryRun   bool     `json:"dry_run"`
		Children []string `json:"children"`
		Voice    struct {
			URI      string `json:"uri"`
			SpecPath string `json:"spec_path"`
		} `json:"voice"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	require.True(t, res.DryRun)
	require.Equal(t, []string{"Detect drift", "Page on-call"}, res.Children)
	require.Equal(t, "stdin://", res.Voice.URI)
	_, err := os.Stat(res.Voice.SpecPath)
	require.True(t, os.IsNotExist(err), "dry run writes nothing")
}
