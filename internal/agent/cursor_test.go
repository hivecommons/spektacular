package agent

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCursorAgent_Install(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, cursorAgent{}.Install(tmp, config.NewDefault(), io.Discard))

	// Cursor discovers skills under .cursor/skills and requires each one's
	// frontmatter name to match its folder.
	for _, skill := range workflowSkills {
		skillPath := filepath.Join(tmp, ".cursor", "skills", skill.Name, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		require.NoError(t, err)
		require.Contains(t, string(data), "spektacular version check",
			"every workflow skill must open with the version-check preamble")
		require.NotContains(t, string(data), "{{")
		validateSkillFrontmatter(t, skillPath)
	}

	// Cursor reads root AGENTS.md itself, so every standing rule must be there.
	agents, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	for _, heading := range []string{
		repoSourcesHeading, storeAccessHeading, memoryContextHeading, knowledgeTriggerHeading,
		specTriggerHeading, designTriggerHeading, draftPresentationHeading, historicalArtifactsHeading,
	} {
		require.Contains(t, string(agents), heading)
	}

	// Skills already appear in Cursor's `/` menu, and AGENTS.md is read
	// natively: no wrappers, rule copies, CLAUDE.md or other agents' roots.
	require.NoDirExists(t, filepath.Join(tmp, ".cursor", "commands"))
	require.NoDirExists(t, filepath.Join(tmp, ".cursor", "rules"))
	require.NoFileExists(t, filepath.Join(tmp, "CLAUDE.md"))
	for _, root := range []string{".claude", ".bob", ".agents", ".omp"} {
		require.NoDirExists(t, filepath.Join(tmp, root))
	}
}
