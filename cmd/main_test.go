package cmd

import (
	"os"
	"testing"
)

// TestMain pins the build version to a release-shaped "0.20.0", so the suite
// exercises the upgrade gate as a released binary sees it. An unstamped build
// reports migrate.DevVersion, which skips the skills-version comparison; the
// tests that cover that set version back to it themselves.
func TestMain(m *testing.M) {
	version = "0.20.0"
	os.Exit(m.Run())
}
