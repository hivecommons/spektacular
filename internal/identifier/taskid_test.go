package identifier

import (
	"errors"
	"regexp"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestTaskIDProviderFor_DefaultIssuesUniqueV4UUIDs(t *testing.T) {
	for _, name := range []string{"", "uuid"} {
		p, err := TaskIDProviderFor(name)
		require.NoError(t, err)

		seen := map[string]bool{}
		for range 200 {
			id, err := p()
			require.NoError(t, err)
			require.Regexp(t, uuidV4, id)
			require.False(t, seen[id], "duplicate id %s", id)
			seen[id] = true
		}
	}
}

func TestTaskIDProviderFor_UnknownProviderIsNamed(t *testing.T) {
	_, err := TaskIDProviderFor("nope")
	var er *output.ErrorResponse
	require.True(t, errors.As(err, &er))
	require.Equal(t, CodeTaskIDProviderUnknown, er.Code)
	require.Contains(t, er.Message, `"nope"`)
	require.Contains(t, er.NextAction, "uuid")
}
