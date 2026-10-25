package tracelog

import (
	"fmt"
	"strings"
)

// SpanRef is the span identifier an agent holds: the run's trace id and the
// span's own id, written "<trace_id>-<span_id>" after the W3C traceparent
// precedent. Carrying the trace lets a command in another process, which
// names no spec, find the run the span belongs to.
type SpanRef struct {
	TraceID string
	SpanID  string
}

// String renders the reference as "<trace_id>-<span_id>".
func (r SpanRef) String() string { return r.TraceID + "-" + r.SpanID }

// ParseSpanRef parses "<32 lowercase hex>-<16 lowercase hex>". Anything else
// is rejected with the reason.
func ParseSpanRef(s string) (SpanRef, error) {
	trace, span, ok := strings.Cut(s, "-")
	if !ok {
		return SpanRef{}, fmt.Errorf("span id %q is not <trace_id>-<span_id>: it has no hyphen", s)
	}
	if err := checkHexID("trace id", trace, 32); err != nil {
		return SpanRef{}, fmt.Errorf("span id %q: %w", s, err)
	}
	if err := checkHexID("span id", span, 16); err != nil {
		return SpanRef{}, fmt.Errorf("span id %q: %w", s, err)
	}
	return SpanRef{TraceID: trace, SpanID: span}, nil
}

// checkHexID reports whether id is exactly n lowercase hex characters.
func checkHexID(what, id string, n int) error {
	if len(id) != n {
		return fmt.Errorf("its %s %q must be %d lowercase hex characters, not %d", what, id, n, len(id))
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return fmt.Errorf("its %s %q must be lowercase hex (0-9, a-f)", what, id)
		}
	}
	return nil
}
