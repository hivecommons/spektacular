package tracelog

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// config.LogProviders is the list log.provider is validated against. The
// config package cannot import this one, so this test pins the two equal.
func TestLogProvidersMatchConfig(t *testing.T) {
	require.Equal(t, Providers(), config.LogProviders)
}
