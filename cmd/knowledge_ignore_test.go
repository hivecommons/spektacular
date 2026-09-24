package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// ignoredSourceProject extends twoScopeProject: the team source gains a
// .spektacular_ignore excluding the noise/ directory (and the ignore file
// itself, so listings stay clean), plus a noise/scratch.md entry that carries
// the same "compass" keyword as the visible fixtures. It returns the team
// source's location.
func ignoredSourceProject(t *testing.T) (teamLoc string) {
	t.Helper()
	_, _, teamLoc = twoScopeProject(t)
	seedKnowledgeFile(t, teamLoc, "noise/scratch.md", "scratch notes: the compass wobbles\n")
	require.NoError(t, os.WriteFile(
		filepath.Join(teamLoc, store.IgnoreFileName),
		[]byte("noise/\n"+store.IgnoreFileName+"\n"), 0o644))
	return teamLoc
}

// Criterion 1: `knowledge list` omits every path matching the source's
// .spektacular_ignore — the noise/ entry and the ignore file itself never
// appear, while all visible entries still do.
func TestKnowledgeList_OmitsIgnoredPaths(t *testing.T) {
	ignoredSourceProject(t)

	stdout, _, err := runKnowledge(t, "list")
	require.NoError(t, err)

	var result struct {
		Entries []knowledgeEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.ElementsMatch(t, []knowledgeEntry{
		{Tier: "repo", Name: "testproj", Path: "readme.md"},
		{Tier: "repo", Name: "testproj", Path: "architecture/initial-idea.md"},
		{Tier: "project", Name: "team", Path: "guidelines.md"},
	}, result.Entries)
}

// Criterion 1: `knowledge search` omits hits inside excluded paths — the
// "compass" occurrence in the team store's noise/scratch.md never surfaces,
// while the visible matches in both stores still do.
func TestKnowledgeSearch_OmitsIgnoredPaths(t *testing.T) {
	ignoredSourceProject(t)

	stdout, _, err := runKnowledge(t, "search", "compass")
	require.NoError(t, err)

	var result struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))

	var addressed []string
	for _, h := range result.Hits {
		addressed = append(addressed, h.Tier+":"+h.Name+"/"+h.Path)
	}
	require.ElementsMatch(t, []string{"repo:testproj/readme.md", "project:team/guidelines.md"}, addressed)
}

// Criterion 2: an excluded path named directly by its exact path is still
// readable — `knowledge read` returns the full content of an entry that
// listing and search exclude.
func TestKnowledgeRead_ExcludedPathStillReadable(t *testing.T) {
	ignoredSourceProject(t)

	stdout, _, err := runKnowledge(t, "read", "--data",
		`{"tier":"project","name":"team","path":"noise/scratch.md"}`)
	require.NoError(t, err)

	var result knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, knowledgeAddressResult{
		Tier:    "project",
		Name:    "team",
		Path:    "noise/scratch.md",
		Content: "scratch notes: the compass wobbles\n",
	}, result)
}

// Criterion 2: an excluded path named directly is still writable —
// `knowledge write` lands the entry under the exclusion, and the file exists
// on disk even though listings never show it.
func TestKnowledgeWrite_ExcludedPathStillWritable(t *testing.T) {
	teamLoc := ignoredSourceProject(t)

	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("hidden but writable\n"), 0o644))

	_, _, err := runKnowledge(t, "write",
		"--data", `{"tier":"project","name":"team","path":"noise/hidden.md"}`,
		"--file", contentPath)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(teamLoc, "noise", "hidden.md"))
	require.NoError(t, err)
	require.Equal(t, "hidden but writable\n", string(data))
}
