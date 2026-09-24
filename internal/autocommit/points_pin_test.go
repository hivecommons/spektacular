// This file is an external test package on purpose. The pin below needs the
// real step lists, and importing the step packages from inside autocommit
// would cycle: steps -> stepkit -> autocommit. An external test package can
// import both sides without closing that loop.
package autocommit_test

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/autocommit"
	"github.com/hivecommons/spektacular/internal/steps/implement"
	"github.com/hivecommons/spektacular/internal/steps/plan"
	"github.com/hivecommons/spektacular/internal/steps/spec"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// Every step name a commit point references must exist in the workflow it
// names, so renaming a step cannot silently orphan a commit point and leave
// the workflow finishing without a commit.
func TestCommitPointsReferenceRealStepNames(t *testing.T) {
	stepNames := map[string][]string{
		"spec":      workflow.New(spec.Steps(), "", workflow.Config{}, nil, nil).StepNames(),
		"plan":      workflow.New(plan.Steps(), "", workflow.Config{}, nil, nil).StepNames(),
		"implement": workflow.New(implement.Steps(), "", workflow.Config{}, nil, nil).StepNames(),
	}

	referenced := autocommit.ReferencedSteps()
	require.NotEmpty(t, referenced, "the commit-point tables reference no steps at all")

	for kind, steps := range referenced {
		names, ok := stepNames[kind]
		require.Truef(t, ok, "commit point names unknown workflow kind %q", kind)
		for _, step := range steps {
			require.Containsf(t, names, step,
				"commit point in %q names step %q, which no longer exists", kind, step)
		}
	}
}
