package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// planVisualsInclude is how a plan step pulls in the rule for drawing a
// mock-up or a flow chart instead of describing it in prose.
const planVisualsInclude = "{{> partials/plan-visuals}}"

// TestPlanDesignStepsCarryTheVisualsRule verifies that every plan step which
// drafts a design section carries the drawing rule exactly once, so the rule
// is in front of the agent wherever a visual change or a complex flow is
// written down.
func TestPlanDesignStepsCarryTheVisualsRule(t *testing.T) {
	for _, p := range []string{
		"steps/plan/03-architecture.md",
		"steps/plan/04-components.md",
		"steps/plan/06-implementation_detail.md",
	} {
		require.Equalf(t, 1, strings.Count(mustReadTemplate(t, p), planVisualsInclude),
			"%s must include the plan-visuals partial exactly once", p)
	}
}

// TestPlanVisualsRuleNamesBothCasesAndTheStore verifies the rule covers the
// two cases a plan draws, fixes SVG as the format, and routes the image
// through the plan store rather than the agent's own file tools.
func TestPlanVisualsRuleNamesBothCasesAndTheStore(t *testing.T) {
	body := mustReadTemplate(t, "partials/plan-visuals.md")
	for _, needle := range []string{
		"design mock-up",
		"flow chart",
		"SVG",
		"{{config.command}} plan image write {{plan_name}}",
		"{{config.command}} plan image list {{plan_name}}",
		"`link`",
	} {
		require.Containsf(t, body, needle, "the plan-visuals partial must contain %q", needle)
	}
}

// TestPlanVerificationChecksImages verifies the verification step checks
// that linked images exist and stored images are linked.
func TestPlanVerificationChecksImages(t *testing.T) {
	body := mustReadTemplate(t, "steps/plan/14-verification.md")
	require.Contains(t, body, "{{config.command}} plan image list {{plan_name}}")
	require.Contains(t, body, "missing image")
}
