package tracelog

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"time"
)

// NewTraceID mints a 32-lowercase-hex OpenTelemetry trace id.
func NewTraceID() string { return newID(16) }

// NewSpanID mints a 16-lowercase-hex OpenTelemetry span id.
func NewSpanID() string { return newID(8) }

// fallbackCounter keeps time-derived ids distinct within one process when
// the random source fails.
var fallbackCounter atomic.Uint64

// newID returns n random bytes hex-encoded, never all zeros (an all-zero id
// is invalid in OpenTelemetry). Logging must never fail, so a failure of the
// random source falls back to an id derived from the clock and a counter.
func newID(n int) string {
	b := make([]byte, n)
	for range 3 {
		if _, err := rand.Read(b); err == nil && !allZero(b) {
			return hex.EncodeToString(b)
		}
	}
	clear(b)
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], uint64(time.Now().UnixNano())^fallbackCounter.Add(1)<<48)
	copy(b, t[:])
	if n > 8 {
		binary.BigEndian.PutUint64(b[8:], fallbackCounter.Add(1))
	}
	if allZero(b) {
		b[n-1] = 1
	}
	return hex.EncodeToString(b)
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
