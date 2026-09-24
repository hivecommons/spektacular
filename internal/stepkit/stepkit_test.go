package stepkit

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

type testData struct {
	values map[string]any
}

func (d *testData) Get(key string) (any, bool) {
	v, ok := d.values[key]
	return v, ok
}

func (d *testData) Set(key string, value any) {
	d.values[key] = value
}

type captureWriter struct {
	result any
}

func (c *captureWriter) WriteResult(v any) error {
	c.result = v
	return nil
}

type fakeStrategy struct {
	primary string
	vars    map[string]any
}

func (f fakeStrategy) PathVars(instanceName, storeRoot string) map[string]any {
	out := map[string]any{
		"fake_path":    filepath.Join(storeRoot, "fake", instanceName+".md"),
		"fake_name":    instanceName,
		"fake_store":   storeRoot,
		"fake_primary": filepath.Join(storeRoot, instanceName),
	}
	for k, v := range f.vars {
		out[k] = v
	}
	return out
}

func (f fakeStrategy) PrimaryPathField() string {
	if f.primary != "" {
		return f.primary
	}
	return "fake_primary"
}

type fakeResult struct {
	StepName     string
	InstanceName string
	PrimaryPath  string
	Instruction  string
}

func buildFakeResult(stepName, instanceName, primaryPath, instruction string) any {
	return fakeResult{
		StepName:     stepName,
		InstanceName: instanceName,
		PrimaryPath:  primaryPath,
		Instruction:  instruction,
	}
}

func TestStepTitle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"a", "A"},
		{"overview", "Overview"},
		{"data_structures", "Data Structures"},
		{"multi_word_snake", "Multi Word Snake"},
		{"acceptance_criteria", "Acceptance Criteria"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, StepTitle(tc.in))
	}
}

func TestGetString(t *testing.T) {
	data := &testData{values: map[string]any{
		"present": "hello",
		"typed":   42,
	}}
	require.Equal(t, "hello", GetString(data, "present"))
	require.Equal(t, "", GetString(data, "missing"))
	require.Equal(t, "", GetString(data, "typed"))
}

func TestRenderTemplateSuccess(t *testing.T) {
	out, err := RenderTemplate("steps/plan/01-overview.md", map[string]any{
		"step":      "overview",
		"title":     "Overview",
		"next_step": "discovery",
		"config":    map[string]any{"command": "spektacular"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, out)
	require.Contains(t, out, "Overview")
}

func TestRenderTemplateMissing(t *testing.T) {
	_, err := RenderTemplate("steps/plan/does-not-exist.md", map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading template")
}

func TestWriteStepResultStandardAndStrategyVars(t *testing.T) {
	tmp := t.TempDir()
	data := &testData{values: map[string]any{"name": "widget"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	err := WriteStepResult(
		StepRequest{
			StepName:     "overview",
			NextStep:     "discovery",
			TemplatePath: "steps/plan/01-overview.md",
			Strategy:     fakeStrategy{},
		},
		data, writer, st, workflow.Config{Command: "spektacular"},
		buildFakeResult,
	)
	require.NoError(t, err)

	got, ok := writer.result.(fakeResult)
	require.True(t, ok)
	require.Equal(t, "overview", got.StepName)
	require.Equal(t, "widget", got.InstanceName)
	require.Equal(t, filepath.Join(tmp, "widget"), got.PrimaryPath)
	require.NotEmpty(t, got.Instruction)
}

func TestWriteStepResultExtraOverridesStrategy(t *testing.T) {
	tmp := t.TempDir()
	data := &testData{values: map[string]any{"name": "widget"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	// Inject a fake strategy that returns fake_primary = X, then override
	// via Extra to Y. Result should reflect the pre-override strategy value
	// because PrimaryPathField reads from pathVars, not the merged vars —
	// this is intentional so per-callback Extras can override rendered
	// template text without changing the emitted Result's primary path.
	err := WriteStepResult(
		StepRequest{
			StepName:     "overview",
			NextStep:     "discovery",
			TemplatePath: "steps/plan/01-overview.md",
			Strategy:     fakeStrategy{},
			Extra:        map[string]any{"title": "OverriddenTitle"},
		},
		data, writer, st, workflow.Config{Command: "spektacular"},
		buildFakeResult,
	)
	require.NoError(t, err)

	got := writer.result.(fakeResult)
	require.Contains(t, got.Instruction, "OverriddenTitle")
}

func TestWriteStepResultMissingTemplateError(t *testing.T) {
	data := &testData{values: map[string]any{"name": "widget"}}
	writer := &captureWriter{}
	st := store.NewFileStore(t.TempDir(), "project")

	err := WriteStepResult(
		StepRequest{
			StepName:     "overview",
			NextStep:     "discovery",
			TemplatePath: "steps/plan/does-not-exist.md",
			Strategy:     fakeStrategy{},
		},
		data, writer, st, workflow.Config{Command: "spektacular"},
		buildFakeResult,
	)
	require.Error(t, err)
	require.Nil(t, writer.result)
}

func TestWriteStepResultRequiresStrategy(t *testing.T) {
	err := WriteStepResult(
		StepRequest{StepName: "x", TemplatePath: "y"},
		&testData{values: map[string]any{}},
		&captureWriter{},
		store.NewFileStore(t.TempDir(), "project"),
		workflow.Config{},
		buildFakeResult,
	)
	require.Error(t, err)
}

func TestWriteStepResultRequiresBuilder(t *testing.T) {
	err := WriteStepResult(
		StepRequest{StepName: "x", TemplatePath: "y", Strategy: fakeStrategy{}},
		&testData{values: map[string]any{}},
		&captureWriter{},
		store.NewFileStore(t.TempDir(), "project"),
		workflow.Config{},
		nil,
	)
	require.Error(t, err)
}

// workingContextFooter is a hand-maintained copy of the rendered
// templates/partials/working-context-footer.md: the leading mustache comment
// line is dropped by rendering, leaving the paragraph and its newline.
const workingContextFooter = "**Before you advance:** refresh `.spektacular/working-context.md` with your cross-cutting working context only — the key decisions and substitutions made, the answers the user gave to your questions, and learnings worth carrying forward. Keep it to learnings and decisions, not a transcript and not a copy of content already captured elsewhere (such as a section's own working file). Use your own file tools. This file is git-tracked, and a resumed session reads it back to pick up where you left off, so keep it current every time before running the `goto` command above.\n"

func renderStepInstruction(t *testing.T, stepName, nextStep, templatePath string) string {
	t.Helper()
	writer := &captureWriter{}
	err := WriteStepResult(
		StepRequest{
			StepName:     stepName,
			NextStep:     nextStep,
			TemplatePath: templatePath,
			Strategy:     fakeStrategy{},
		},
		&testData{values: map[string]any{"name": "widget"}},
		writer, nil, workflow.Config{Command: "spektacular"},
		buildFakeResult,
	)
	require.NoError(t, err)
	return writer.result.(fakeResult).Instruction
}

func TestWriteStepResultAppendsFooterWhenNextStepSet(t *testing.T) {
	got := renderStepInstruction(t, "overview", "discovery", "steps/plan/01-overview.md")

	require.True(t, strings.HasSuffix(got, "\n\n---\n\n"+workingContextFooter),
		"a continuing step must end with a rule and the working-context footer, got tail:\n%s",
		got[max(0, len(got)-800):])
}

func TestWriteStepResultOmitsFooterOnTerminalStep(t *testing.T) {
	got := renderStepInstruction(t, "finished", "", "steps/plan/19-finished.md")

	require.NotContains(t, got, "**Before you advance:**")
}

func TestFSPartialsResolvesMarkdownFile(t *testing.T) {
	p := FSPartials{FS: fstest.MapFS{
		"partials/x.md": &fstest.MapFile{Data: []byte("shared prose")},
	}}

	got, err := p.Get("partials/x")
	require.NoError(t, err)
	require.Equal(t, "shared prose", got)

	_, err = p.Get("partials/missing")
	require.Error(t, err)
}

func TestMissingPartialFailsRender(t *testing.T) {
	p := FSPartials{FS: fstest.MapFS{
		"partials/x.md": &fstest.MapFile{Data: []byte("shared prose")},
	}}

	_, err := mustache.RenderPartials("before {{> partials/nope}} after", p, nil)
	require.Error(t, err)
}
