package meeting

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults for every tunable. Each has an environment variable (see
// FromEnv); the contract's own limits keep the names and defaults it gives.
const (
	DefaultPause           = 1500 * time.Millisecond
	DefaultMaxAsksPer10m   = 3
	DefaultHeartbeat       = 15 * time.Second
	DefaultDecisionTimeout = 2500 * time.Millisecond
	DefaultSummaryTimeout  = 10 * time.Second
	DefaultWindowSegments  = 40
	DefaultOfferTTL        = 2 * time.Minute
	DefaultContextMaxBytes = 64 * 1024
	DefaultNames           = "hive,spek,spektacular"

	// AskWindow is the sliding window the ask limit counts over.
	AskWindow = 10 * time.Minute
)

// Environment variable names.
const (
	EnvPause           = "SPEK_MEETING_PAUSE_MS"
	EnvMaxAsks         = "SPEK_MEETING_MAX_ASKS_PER_10M"
	EnvHeartbeat       = "SPEK_MEETING_HEARTBEAT_S"
	EnvDecisionTimeout = "SPEK_MEETING_LLM_TIMEOUT_MS"
	EnvSummaryTimeout  = "SPEK_MEETING_SUMMARY_TIMEOUT_MS"
	EnvWindowSegments  = "SPEK_MEETING_WINDOW_SEGMENTS"
	EnvOfferTTL        = "SPEK_MEETING_OFFER_TTL_S"
	EnvContextMaxBytes = "SPEK_MEETING_CONTEXT_MAX_BYTES"
	EnvNames           = "SPEK_MEETING_NAMES"
)

// Settings are the engine's tunables.
type Settings struct {
	// Pause is the silence after the last final segment that counts as a
	// natural pause, at which Spek may ask a question.
	Pause time.Duration
	// MaxAsksPer10m bounds asks over any ten-minute window.
	MaxAsksPer10m int
	// Heartbeat is the interval between heartbeat intents.
	Heartbeat time.Duration
	// DecisionTimeout bounds one decision call to the model.
	DecisionTimeout time.Duration
	// SummaryTimeout bounds the closing summary call.
	SummaryTimeout time.Duration
	// WindowSegments is how many recent transcript lines the model sees.
	WindowSegments int
	// OfferTTL is how long an offer to research stays open for a "yes".
	OfferTTL time.Duration
	// ContextMaxBytes bounds the --context files sent to the model.
	ContextMaxBytes int
	// Names are what the meeting calls Spek ("hive", "spek"), lower case.
	Names []string
}

// DefaultSettings returns the defaults.
func DefaultSettings() Settings {
	return Settings{
		Pause:           DefaultPause,
		MaxAsksPer10m:   DefaultMaxAsksPer10m,
		Heartbeat:       DefaultHeartbeat,
		DecisionTimeout: DefaultDecisionTimeout,
		SummaryTimeout:  DefaultSummaryTimeout,
		WindowSegments:  DefaultWindowSegments,
		OfferTTL:        DefaultOfferTTL,
		ContextMaxBytes: DefaultContextMaxBytes,
		Names:           splitNames(DefaultNames),
	}
}

// FromEnv returns the defaults overridden by any SPEK_MEETING_* variable
// that is set. A value that does not parse, or is not positive, is reported
// to log and the default kept.
func FromEnv(getenv func(string) string, log io.Writer) Settings {
	if getenv == nil {
		getenv = os.Getenv
	}
	s := DefaultSettings()
	num := func(name string, def int) int {
		v := strings.TrimSpace(getenv(name))
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			fmt.Fprintf(log, "spektacular meeting: ignoring %s=%q (want a positive integer); using %d\n", name, v, def)
			return def
		}
		return n
	}
	s.Pause = time.Duration(num(EnvPause, int(DefaultPause/time.Millisecond))) * time.Millisecond
	s.MaxAsksPer10m = num(EnvMaxAsks, DefaultMaxAsksPer10m)
	s.Heartbeat = time.Duration(num(EnvHeartbeat, int(DefaultHeartbeat/time.Second))) * time.Second
	s.DecisionTimeout = time.Duration(num(EnvDecisionTimeout, int(DefaultDecisionTimeout/time.Millisecond))) * time.Millisecond
	s.SummaryTimeout = time.Duration(num(EnvSummaryTimeout, int(DefaultSummaryTimeout/time.Millisecond))) * time.Millisecond
	s.WindowSegments = num(EnvWindowSegments, DefaultWindowSegments)
	s.OfferTTL = time.Duration(num(EnvOfferTTL, int(DefaultOfferTTL/time.Second))) * time.Second
	s.ContextMaxBytes = num(EnvContextMaxBytes, DefaultContextMaxBytes)
	if v := strings.TrimSpace(getenv(EnvNames)); v != "" {
		if names := splitNames(v); len(names) > 0 {
			s.Names = names
		}
	}
	return s
}

func splitNames(v string) []string {
	var out []string
	for _, n := range strings.Split(v, ",") {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			out = append(out, n)
		}
	}
	return out
}
