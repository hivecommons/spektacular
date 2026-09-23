package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// `<kind> file list` carries modified_at per entry from the store's own
// listing, so an orchestrator polling N artifacts makes one list call rather
// than N status calls. Files report their own mtime; a plan's directory entry
// reports the directory's, which moves when a document is added or removed,
// not when plan.md is edited — `plan file list <name>` lists the documents
// themselves for that.
func TestStoreFileList_CarriesModifiedAtPerEntry(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	dataDir := filepath.Join(dir, ".spektacular")

	specTime := time.Date(2026, time.January, 4, 5, 6, 7, 0, time.UTC)
	writeArtifactStatusFile(t, filepath.Join(dataDir, "specs", "000001_feature.md"), "---\ncreated_date: 2026-01-02\ndocument_status: final\nclosed_date: 2026-01-03\n---\n", specTime)
	planDocTime := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
	writeArtifactStatusFile(t, filepath.Join(dataDir, "plans", "000001_feature", "plan.md"), "---\ncreated_date: 2026-02-01\ndocument_status: draft\n---\n", planDocTime)
	planDirTime := time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(dataDir, "plans", "000001_feature"), planDirTime, planDirTime))

	specs := runListJSON(t, "spec")
	require.Len(t, specs.Files, 1)
	require.Equal(t, map[string]any{
		"name":            "000001_feature.md",
		"path":            ".spektacular/specs/000001_feature.md",
		"created_date":    "2026-01-02",
		"document_status": "final",
		"closed_date":     "2026-01-03",
		"modified_at":     "2026-01-04T05:06:07Z",
	}, specs.Files[0], "a file entry keeps its existing fields and gains modified_at")

	planDirs := runListJSON(t, "plan")
	require.Len(t, planDirs.Files, 1)
	require.Equal(t, map[string]any{
		"name":        "000001_feature",
		"path":        ".spektacular/plans/000001_feature",
		"modified_at": "2026-02-02T00:00:00Z",
	}, planDirs.Files[0], "a directory entry carries the directory's own mtime and no metadata")

	planDocs := runListJSON(t, "plan", "000001_feature")
	require.Len(t, planDocs.Files, 1)
	require.Equal(t, "2026-02-03T04:05:06Z", planDocs.Files[0]["modified_at"])
	require.Equal(t, "plan.md", planDocs.Files[0]["name"])
}
