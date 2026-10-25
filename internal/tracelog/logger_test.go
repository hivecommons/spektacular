package tracelog

import (
	"errors"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// TestNewFileLogger checks the file provider resolves to the file logger.
func TestNewFileLogger(t *testing.T) {
	l, err := New("file", FileOptions{Directory: t.TempDir(), Retention: 10})
	require.NoError(t, err)
	require.IsType(t, &fileLogger{}, l)
}

// TestNewUnknownProviderNamesAcceptedLoggers checks an unknown provider is a
// configuration error whose next action names the accepted loggers.
func TestNewUnknownProviderNamesAcceptedLoggers(t *testing.T) {
	l, err := New("bogus", FileOptions{})
	require.Nil(t, l)
	require.Error(t, err)

	var er *output.ErrorResponse
	require.True(t, errors.As(err, &er))
	require.Equal(t, "config_invalid", er.Code)
	require.Contains(t, er.Message, `"bogus"`)
	require.Contains(t, er.NextAction, "log.provider")
	require.Contains(t, er.NextAction, "file")
}

// TestProviders pins the accepted logger names.
func TestProviders(t *testing.T) {
	require.Equal(t, []string{"file"}, Providers())
}
