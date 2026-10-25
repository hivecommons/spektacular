package tracelog

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	spanIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// TestIDFormatsAndUniqueness mints many ids and checks each is lowercase hex
// of the OpenTelemetry width, never all zeros, and never repeated.
func TestIDFormatsAndUniqueness(t *testing.T) {
	const n = 10000
	traces := make(map[string]struct{}, n)
	spans := make(map[string]struct{}, n)
	for range n {
		tr := NewTraceID()
		require.Regexp(t, traceIDPattern, tr)
		require.NotEqual(t, strings.Repeat("0", 32), tr)
		_, dup := traces[tr]
		require.False(t, dup, "trace id %s repeated", tr)
		traces[tr] = struct{}{}

		sp := NewSpanID()
		require.Regexp(t, spanIDPattern, sp)
		require.NotEqual(t, strings.Repeat("0", 16), sp)
		_, dup = spans[sp]
		require.False(t, dup, "span id %s repeated", sp)
		spans[sp] = struct{}{}
	}
}

// TestAllZero covers the guard that rejects an all-zero id.
func TestAllZero(t *testing.T) {
	require.True(t, allZero(make([]byte, 8)))
	require.False(t, allZero([]byte{0, 0, 1}))
}
