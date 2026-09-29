package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

type customWorkflowResult struct {
	Error       bool     `json:"error"`
	Workflow    string   `json:"workflow"`
	RunName     string   `json:"run_name"`
	Step        string   `json:"step"`
	NextSteps   []string `json:"next_steps"`
	Instruction string   `json:"instruction"`
	Code        string   `json:"code"`
}

func TestCustomWorkflowRunsBackStepAndRestartsAfterNonFinishedTerminal(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "workflow", "new", "marketing-ideation", "--data", `{"name":"community-launch"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	var result customWorkflowResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.False(t, result.Error)
	require.Equal(t, "marketing-ideation", result.Workflow)
	require.Equal(t, "community-launch", result.RunName)
	require.Equal(t, "brief", result.Step)
	require.Contains(t, result.Instruction, "workflow goto marketing-ideation")

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "marketing-ideation", "--data", `{"step":"audience"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "audience", result.Step)
	require.Contains(t, result.NextSteps, "brief")
	require.Contains(t, result.Instruction, `"step":"brief"`)

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "marketing-ideation", "--data", `{"step":"brief"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "brief", result.Step)

	resetRootCmd(t)
	_, _, code = runRootCmd(t, "workflow", "goto", "marketing-ideation", "--data", `{"step":"audience"}`)
	require.Equal(t, 0, code)
	resetRootCmd(t)
	_, _, code = runRootCmd(t, "workflow", "goto", "marketing-ideation", "--data", `{"step":"ideas"}`)
	require.Equal(t, 0, code)
	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "marketing-ideation", "--data", `{"step":"publish"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "publish", result.Step)
	require.NotContains(t, result.Instruction, `"step":"ideas"`)

	statePath := filepath.Join(dir, ".spektacular", "state.json")
	raw, err := os.ReadFile(statePath)
	require.NoError(t, err)
	var state workflow.State
	require.NoError(t, json.Unmarshal(raw, &state))
	require.Equal(t, "workflow:marketing-ideation", state.Kind)
	require.Equal(t, "publish", state.CurrentStep)
	require.Equal(t, "publish", state.TerminalStep)
	require.False(t, state.InProgress())

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "new", "marketing-ideation", "--data", `{"name":"second-run"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "brief", result.Step)
	require.Equal(t, "second-run", result.RunName)
}

func TestProjectWorkflowDefinedOnlyByYAMLAndMarkdownRunsAndResumes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	writeProjectWorkflowFixture(t, dir)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "workflow", "steps", "release-announcement")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "outline")

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "new", "release-announcement", "--data", `{"name":"v1-launch","channel":"blog"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	var result customWorkflowResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "outline", result.Step)
	require.Contains(t, result.Instruction, "v1-launch")
	require.Contains(t, result.Instruction, "blog")

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "new", "release-announcement", "--data", `{"name":"ignored"}`)
	require.Equal(t, 1, code)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "workflow_in_progress")
	require.Contains(t, stdout, "outline")

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "release-announcement", "--data", `{"step":"draft"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "draft", result.Step)
	require.Contains(t, result.NextSteps, "outline")

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "release-announcement", "--data", `{"step":"outline"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "outline", result.Step)

	resetRootCmd(t)
	_, _, code = runRootCmd(t, "workflow", "goto", "release-announcement", "--data", `{"step":"draft"}`)
	require.Equal(t, 0, code)
	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "release-announcement", "--data", `{"step":"publish"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "publish", result.Step)

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "new", "release-announcement", "--data", `{"name":"v2-launch"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "outline", result.Step)
	require.Equal(t, "v2-launch", result.RunName)
}

func TestProjectWorkflowConditionalTransitionsRejectFalseBranchAndAcceptInjectedData(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	writeConditionalWorkflowFixture(t, dir)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "workflow", "new", "conditional-campaign", "--data", `{"name":"conditional-run"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	var result customWorkflowResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "route", result.Step)
	require.NotContains(t, result.NextSteps, "email")
	require.NotContains(t, result.NextSteps, "blog")

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "conditional-campaign", "--data", `{"step":"blog"}`)
	require.Equal(t, 1, code)
	require.Empty(t, stderr)
	var failure struct {
		Code  string `json:"code"`
		State struct {
			ValidActions []string `json:"valid_actions"`
		} `json:"state"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &failure))
	require.Equal(t, "invalid_transition", failure.Code)
	require.NotContains(t, failure.State.ValidActions, "blog")
	require.NotContains(t, failure.State.ValidActions, "email")

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "conditional-campaign", "--data", `{"step":"email","channel":"email"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "email", result.Step)
	require.Contains(t, result.Instruction, "Email conditional-run")
	state, err := readState(filepath.Join(dir, ".spektacular", "state.json"))
	require.NoError(t, err)
	require.Equal(t, "email", state.CurrentStep)
	require.ElementsMatch(t, []string{"email", "blog"}, state.TerminalSteps)
	require.False(t, state.InProgress())

	resetRootCmd(t)
	stdout, stderr, code = runRootCmd(t, "workflow", "goto", "conditional-campaign", "--data", `{"step":"done"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	state, err = readState(filepath.Join(dir, ".spektacular", "state.json"))
	require.NoError(t, err)
	require.Equal(t, "done", state.CurrentStep)
	require.False(t, state.InProgress())
}

func writeConditionalWorkflowFixture(t *testing.T, dir string) {
	t.Helper()
	root := filepath.Join(dir, ".spektacular", "workflows", "conditional-campaign")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "steps"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "workflow.yaml"), []byte(`name: conditional-campaign
description: Route campaign work by selected channel.
steps:
  - name: route
    prompt: steps/01-route.md
    transitions:
      - to: email
        when:
          key: channel
          equals: email
      - to: blog
        when:
          key: channel
          equals: blog
  - name: email
    prompt: steps/02-email.md
    transitions: []
  - name: blog
    prompt: steps/03-blog.md
    transitions: []
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "steps", "01-route.md"), []byte("Choose a channel.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "steps", "02-email.md"), []byte("Email {{run_name}}.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "steps", "03-blog.md"), []byte("Blog {{run_name}}.\n"), 0o644))
}

func writeProjectWorkflowFixture(t *testing.T, dir string) {
	t.Helper()
	root := filepath.Join(dir, ".spektacular", "workflows", "release-announcement")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "steps"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "workflow.yaml"), []byte(`name: release-announcement
description: Draft a release announcement.
steps:
  - name: outline
    prompt: steps/01-outline.md
  - name: draft
    prompt: steps/02-draft.md
    transitions:
      - to: outline
      - to: publish
  - name: publish
    prompt: steps/03-publish.md
    transitions: []
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "steps", "01-outline.md"), []byte("Outline {{run_name}} for {{channel}}.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "steps", "02-draft.md"), []byte("Draft {{name}}.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "steps", "03-publish.md"), []byte("Publish {{run_name}}.\n"), 0o644))
}
