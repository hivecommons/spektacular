package migrate

import (
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// TestRegistered_StepsAreContiguousToCurrentSchema pins every kind's steps
// to its config constant: they run From = 1..current-1 with no gaps, so
// bumping a constant without registering its step (or vice versa) fails here.
func TestRegistered_StepsAreContiguousToCurrentSchema(t *testing.T) {
	for kind, want := range map[Kind]int{
		KindProject: config.CurrentProjectSchema,
		KindRepo:    config.CurrentRepoSchema,
	} {
		t.Run(string(kind), func(t *testing.T) {
			steps := Registered(kind)
			require.Len(t, steps, want-1)
			for i, s := range steps {
				require.Equal(t, kind, s.Kind, "step %d", i)
				require.Equal(t, i+1, s.From, "step %d", i)
				require.NotEmpty(t, s.Description, "step %d", i)
				require.NotNil(t, s.Run, "step %d", i)
			}
			require.Equal(t, want, steps[len(steps)-1].From+1)
		})
	}
}

func TestRegistered_ReturnsACopy(t *testing.T) {
	steps := Registered(KindProject)
	steps[0].Description = "changed"
	require.NotEqual(t, "changed", Registered(KindProject)[0].Description)
}

func TestStepsFrom_FailsOnGap(t *testing.T) {
	withSteps(t, KindRepo, []Step{repo1to2}, 3)
	_, err := stepsFrom(KindRepo, 1)
	require.ErrorContains(t, err, "no repo upgrade step registered from format 2")
}
