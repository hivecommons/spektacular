package cmd

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
)

// testLogLevel forces runRoot's log level for the whole suite, so the cmd
// tests can be run with logging forced off (go test ./cmd/ -args
// -log-level=off) as well as with it on by default. Tests that assert
// logging output call useConfiguredLogLevel, which lifts the override.
var testLogLevel = flag.String("log-level", "", "force runRoot's log level: off, standard or verbose (test only)")

// TestMain pins the build version to a release-shaped "0.20.0", so the suite
// exercises the upgrade gate as a released binary sees it. An unstamped build
// reports migrate.DevVersion, which skips the skills-version comparison; the
// tests that cover that set version back to it themselves.
func TestMain(m *testing.M) {
	flag.Parse()
	switch *testLogLevel {
	case "", config.LogLevelOff, config.LogLevelStandard, config.LogLevelVerbose:
	default:
		fmt.Fprintf(os.Stderr, "-log-level must be off, standard or verbose, not %q\n", *testLogLevel)
		os.Exit(2)
	}
	forcedLogLevel = *testLogLevel
	version = "0.20.0"
	os.Exit(m.Run())
}
