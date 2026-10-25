package stepkit

import (
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

type fakeStrategy struct{}

func (fakeStrategy) PathVars(instanceName, _ string) map[string]any {
	return map[string]any{"fake_name": instanceName}
}

func (fakeStrategy) PrimaryLocation(instanceName string) string {
	return "fakes/" + instanceName + "/fake.md"
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
	require.Equal(t, "fakes/widget/fake.md", got.PrimaryPath,
		"the Result's primary path must be the strategy's PrimaryLocation, never a host path")
	require.NotContains(t, got.PrimaryPath, tmp)
	require.NotEmpty(t, got.Instruction)
}

func TestWriteStepResultExtraOverridesStrategy(t *testing.T) {
	tmp := t.TempDir()
	data := &testData{values: map[string]any{"name": "widget"}}
	writer := &captureWriter{}
	st := store.NewFileStore(tmp, "project")

	// Extras override rendered template text; the Result's primary path
	// comes from PrimaryLocation and is unaffected by them.
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
	require.Equal(t, "fakes/widget/fake.md", got.PrimaryPath)
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

// traceBlock is a hand-maintained copy of the rendered
// templates/partials/trace-marking.md for a standalone run whose step span
// is "<run>-<span>" below.
const traceBlock = "**Mark spans.** Before you launch a helper agent, run `spektacular trace start <helper> --type sub-agent --parent 4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7` and put the id it prints in the helper's brief: \"Your parent span is `<id>`: start any span of your own with `--parent <id>`.\" When the helper returns, run `spektacular trace stop <id>` (add `--status error` if it failed), chained with your next command.\n\n" +
	"Before you ask the user a question, run `spektacular trace start <question> --type user-wait --parent 4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7`, and once they answer run `spektacular trace stop <id>`, chained with your next command.\n"

func renderWithSpan(t *testing.T, nextStep, templatePath, stepSpan string) string {
	t.Helper()
	writer := &captureWriter{}
	err := WriteStepResult(
		StepRequest{StepName: "overview", NextStep: nextStep, TemplatePath: templatePath, Strategy: fakeStrategy{}},
		&testData{values: map[string]any{"name": "widget"}},
		writer, nil, workflow.Config{Command: "spektacular", StepSpan: stepSpan},
		buildFakeResult,
	)
	require.NoError(t, err)
	return writer.result.(fakeResult).Instruction
}

func TestWriteStepResultAppendsTheTraceBlockAfterTheStepText(t *testing.T) {
	got := renderWithSpan(t, "discovery", "steps/plan/01-overview.md", "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7")

	require.Equal(t, 1, strings.Count(got, "**Mark spans.**"))
	require.Contains(t, got, "\n\n---\n\n"+traceBlock+"\n---\n\n"+workingContextFooter,
		"the trace block sits between the step text and the footer")
}

func TestWriteStepResultTraceBlockWithoutAStepSpanNamesNoParent(t *testing.T) {
	got := renderWithSpan(t, "", "steps/plan/19-finished.md", "")
	require.Contains(t, got, "**Mark spans.**")
	require.NotContains(t, got, "--parent 4bf9", "a step with no span names none")
	require.True(t, strings.HasSuffix(got, "chained with your next command.\n"), "the terminal step ends with the trace block")
}
