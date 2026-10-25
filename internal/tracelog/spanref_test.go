package tracelog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseSpanRef covers the accepted form and each way an id is refused.
func TestParseSpanRef(t *testing.T) {
	ok, err := ParseSpanRef("4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7")
	require.NoError(t, err)
	require.Equal(t, SpanRef{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7"}, ok)
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7", ok.String())

	zeros, err := ParseSpanRef("00000000000000000000000000000000-0000000000000000")
	require.NoError(t, err, "any id of the right form is accepted")
	require.Equal(t, SpanRef{TraceID: "00000000000000000000000000000000", SpanID: "0000000000000000"}, zeros)

	bad := []struct{ name, in, reason string }{
		{"empty", "", "no hyphen"},
		{"bare span id", "00f067aa0ba902b7", "no hyphen"},
		{"short trace", "4bf92f3577b34da6-00f067aa0ba902b7", "32 lowercase hex"},
		{"short span", "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa", "16 lowercase hex"},
		{"long span", "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7ff", "16 lowercase hex"},
		{"uppercase", "4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7", "lowercase hex"},
		{"not hex", "4bf92f3577b34da6a3ce929d0e0e473z-00f067aa0ba902b7", "lowercase hex"},
		{"extra hyphen", "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa-0ba902b7", "16 lowercase hex"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSpanRef(tc.in)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.reason)
		})
	}
}
