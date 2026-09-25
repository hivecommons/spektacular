package cmd

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// TestEmitResumeReport_JSONCarriesWorkflowIdentityAndInstruction asserts that
// a same-kind in-progress workflow produces the shared ErrorResponse shape
// (error=true, workflow_in_progress code, the workflow's name as Resource,
// its current step as State.Current, and the rendered resume instruction as
// NextAction) and that this shape round-trips through JSON, since it flows
// all the way to stdout via that encoding in production.
func TestEmitResumeReport_JSONCarriesWorkflowIdentityAndInstruction(t *testing.T) {
	instruction, err := resumeInstruction("spektacular", "spec", "000024_resume", "overview", "")
	require.NoError(t, err)
	require.NotEmpty(t, instruction)

	state := &workflow.State{
		Kind:        "spec",
		CurrentStep: "overview",
		Data:        map[string]any{"name": "000024_resume"},
	}

	reportErr := emitResumeReport("spektacular", "spec", state)
	require.Error(t, reportErr)
	er, ok := reportErr.(*output.ErrorResponse)
	require.True(t, ok, "emitResumeReport must return an *output.ErrorResponse")

	require.True(t, er.IsError)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "000024_resume", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "overview", er.State.Current)
	require.Equal(t, instruction, er.NextAction)

	encoded, err := json.Marshal(er)
	require.NoError(t, err)
	out := string(encoded)

	require.Contains(t, out, `"error":true`)
	require.Contains(t, out, `"code":"workflow_in_progress"`)
	require.Contains(t, out, `"resource":"000024_resume"`)
	require.Contains(t, out, `"current":"overview"`)
	require.Contains(t, out, `"next_action":`)

	var roundTrip output.ErrorResponse
	require.NoError(t, json.Unmarshal(encoded, &roundTrip))
	require.NotEmpty(t, roundTrip.NextAction)
}

func TestResumeInstruction_AsksResumeVsNewWithBothCommands(t *testing.T) {
	out, err := resumeInstruction("spektacular", "spec", "000024_resume", "overview", "")
	require.NoError(t, err)

	require.NotContains(t, out, "{{")

	require.Contains(t, out, "spec")
	require.Contains(t, out, "000024_resume")
	require.Contains(t, out, "overview")

	require.Contains(t, out, `spektacular spec goto --data '{"step":"overview"}'`)
	require.Contains(t, out, "spektacular spec new --force")
	require.Contains(t, out, ".spektacular/working-context.md")

	require.Contains(t, out, "resume")
	require.Contains(t, out, "new")
}

func TestResumeInstruction_InterpolatesAcrossKinds(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		kind        string
		instance    string
		currentStep string
		wantGoto    string
		wantNew     string
	}{
		{
			name:        "spec",
			command:     "spektacular",
			kind:        "spec",
			instance:    "000024_resume",
			currentStep: "overview",
			wantGoto:    `spektacular spec goto --data '{"step":"overview"}'`,
			wantNew:     "spektacular spec new --force",
		},
		{
			name:        "plan",
			command:     "spek",
			kind:        "plan",
			instance:    "000024_resume",
			currentStep: "tasks",
			wantGoto:    `spek plan goto --data '{"step":"tasks"}'`,
			wantNew:     "spek plan new --force",
		},
		{
			name:        "implement",
			command:     "spektacular",
			kind:        "implement",
			instance:    "000024_resume",
			currentStep: "execute",
			wantGoto:    `spektacular implement goto --data '{"step":"execute"}'`,
			wantNew:     "spektacular implement new --force",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := resumeInstruction(tt.command, tt.kind, tt.instance, tt.currentStep, "")
			require.NoError(t, err)

			require.NotContains(t, out, "{{")
			require.Contains(t, out, tt.kind)
			require.Contains(t, out, tt.instance)
			require.Contains(t, out, tt.currentStep)
			require.Contains(t, out, tt.wantGoto)
			require.Contains(t, out, tt.wantNew)
			require.Contains(t, out, ".spektacular/working-context.md")
		})
	}
}

// implementResumeSteps is a hand-maintained list of every implement workflow
// step a run can be interrupted at. It is deliberately not derived from the
// workflow definition: the resume prompt must read the plan first whichever
// of these steps the run stopped at.
var implementResumeSteps = []string{
	"read_plan",
	"analyze",
	"implement",
	"test",
	"verify",
	"update_plan",
	"update_changelog",
	"test_plan",
	"update_feature_changelog",
	"reconcile_spec",
}

var numberedItemLine = regexp.MustCompile(`^\d+\. `)

func TestResumeImplement_ReadsPlanFirstAtEveryStep(t *testing.T) {
	const command = "spekx"

	block, err := stepkit.RenderTemplate("partials/implement-plan-documents.md", map[string]any{"command": command})
	require.NoError(t, err)
	block = normalizeIndent(strings.TrimSpace(block))
	require.NotEmpty(t, block)

	for _, step := range implementResumeSteps {
		t.Run(step, func(t *testing.T) {
			out, err := resumeInstruction(command, "implement", "demo-feature", step, "")
			require.NoError(t, err)

			require.Contains(t, normalizeIndent(out), block,
				"the implement resume must include the shared plan-documents block")
			require.Contains(t, out, "first unchecked")

			planRead := command + " plan file read <plan_name>/plan.md"
			// The partial also names the working-context path, so anchor on
			// the numbered item that tells the agent to read it.
			workingContext := "\n2. Read `.spektacular/working-context.md`"
			gotoCmd := command + ` implement goto --data '{"step":"` + step + `"}'`

			planIdx := strings.Index(out, planRead)
			wcIdx := strings.Index(out, workingContext)
			gotoIdx := strings.Index(out, gotoCmd)
			require.NotEqual(t, -1, planIdx, "must read plan.md")
			require.NotEqual(t, -1, wcIdx, "must read the working context as item 2")
			require.NotEqual(t, -1, gotoIdx, "must give the goto command for the stopped step")
			require.Less(t, planIdx, wcIdx, "the plan must be read before the working context")
			require.Less(t, wcIdx, gotoIdx, "the working context must be read before resuming")

			require.NotContains(t, out, "repo list")
			require.NotContains(t, out, ".spektacular/work/")

			start := strings.Index(out, "### To resume")
			end := strings.Index(out, "### To discard")
			require.NotEqual(t, -1, start)
			require.Greater(t, end, start)
			items := 0
			for _, line := range strings.Split(out[start:end], "\n") {
				if numberedItemLine.MatchString(line) {
					items++
				}
			}
			require.Equal(t, 4, items, "the resume section must hold exactly four numbered items")
		})
	}
}

func TestResumeNonImplementUsesSharedTemplate(t *testing.T) {
	for _, kind := range []string{"spec", "plan", "repo"} {
		t.Run(kind, func(t *testing.T) {
			out, err := resumeInstruction("spekx", kind, "demo-feature", "overview", "")
			require.NoError(t, err)

			require.Contains(t, out, "spekx repo list")
			require.Contains(t, out, ".spektacular/work/demo-feature/")
			require.NotContains(t, out, "Read the plan before anything else")
		})
	}
}
