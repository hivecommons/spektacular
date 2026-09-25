package implement

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStrategyPathVars_NamesOnly asserts the implement strategy provides
// document names only — never a host path — so no instruction can send an
// agent to a file that may not be on disk.
func TestStrategyPathVars_NamesOnly(t *testing.T) {
	s := strategy{planDir: ".spektacular/plans"}

	require.Equal(t, map[string]any{
		"plan_name":              "000039_feature",
		"changelog_section_name": "## Changelog",
	}, s.PathVars("000039_feature", "/store"))
}

// TestStrategyPrimaryLocation_IsConfigRelative pins the plan's reported
// location relative to the folder holding config.yaml.
func TestStrategyPrimaryLocation_IsConfigRelative(t *testing.T) {
	s := strategy{planDir: ".spektacular/plans"}

	require.Equal(t, "plans/000039_feature/plan.md", s.PrimaryLocation("000039_feature"))
}

// TestChangelogFilePath_IsFlat pins the path helper itself: no project name
// segment; the file lives directly under the configured directory.
func TestChangelogFilePath_IsFlat(t *testing.T) {
	require.Equal(t, "changelog/000039_feature.md",
		ChangelogFilePath("changelog", "000039_feature"))
}
