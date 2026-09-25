package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// locationProject lays out a project (chdir'd into) with default store
// directories and one registered member repo named "member", and seeds
// through the CLI a spec, central changelog record and plan documents `plan`
// and `context` for 000001_feat, plus a member-routed changelog record
// 000002_other.
func locationProject(t *testing.T, extraConfig string) {
	t.Helper()
	memberDir := footprintMemberRepo(t)
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeSpecCommandConfig(t, projectDir,
		"spec:\n  id_method: counter\n"+extraConfig+
			"repos:\n  - name: member\n    location: "+memberDir+"\n")
	src := filepath.Join(projectDir, "src.md")
	require.NoError(t, os.WriteFile(src, []byte("body"), 0o644))
	for _, args := range [][]string{
		{"spec", "file", "write", "000001_feat"},
		{"changelog", "file", "write", "000001_feat"},
		{"changelog", "file", "write", "000002_other", "--repo", "member"},
		{"plan", "file", "write", "000001_feat", "plan"},
		{"plan", "file", "write", "000001_feat", "context"},
	} {
		addrOK(t, append(args, "--from", src)...)
	}
}

// listedPaths runs a list command and returns each listed name's path.
func listedPaths(t *testing.T, args ...string) map[string]string {
	t.Helper()
	var res struct {
		Files []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(addrOK(t, args...)), &res))
	paths := map[string]string{}
	for _, f := range res.Files {
		paths[f.Name] = f.Path
	}
	return paths
}

// Every list reports each entry's path relative to the folder holding the
// config file that declares its store: .spektacular/ for the central stores,
// the member's repo.yaml folder for a repo-routed changelog.
func TestStoreFileList_PathIsRelativeToDeclaringConfigFolder(t *testing.T) {
	locationProject(t, "")

	cases := []struct {
		args []string
		want map[string]string
	}{
		{[]string{"spec", "file", "list"}, map[string]string{"000001_feat": "specs/000001_feat.md"}},
		{[]string{"plan", "file", "list"}, map[string]string{"000001_feat": "plans/000001_feat"}},
		{[]string{"plan", "file", "list", "000001_feat"}, map[string]string{
			"plan":    "plans/000001_feat/plan.md",
			"context": "plans/000001_feat/context.md",
		}},
		{[]string{"changelog", "file", "list"}, map[string]string{"000001_feat": "changelog/000001_feat.md"}},
		{[]string{"changelog", "file", "list", "--repo", "member"}, map[string]string{"000002_other": "changelog/testproj/000002_other.md"}},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, listedPaths(t, tc.args...), "%v", tc.args)
	}
}

// A non-default spec directory, resolved relative to .spektacular/, is
// reported the way it is written in config.yaml.
func TestStoreFileList_NonDefaultSpecDirectoryPathReadsAsConfigured(t *testing.T) {
	locationProject(t, "  config:\n    directory: docs/specs\n")

	require.Equal(t, map[string]string{"000001_feat": "docs/specs/000001_feat.md"},
		listedPaths(t, "spec", "file", "list"))
}

// set-document-status reports the updated document's path with the same
// convention as list.
func TestStoreFileSetDocumentStatus_PathIsRelativeToDeclaringConfigFolder(t *testing.T) {
	locationProject(t, "")

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"spec", "file", "set-document-status", "000001_feat"}, "specs/000001_feat.md"},
		{[]string{"plan", "file", "set-document-status", "000001_feat", "plan"}, "plans/000001_feat/plan.md"},
	} {
		var res struct {
			Path string `json:"path"`
		}
		out := addrOK(t, append(tc.args, "--document-status", "final")...)
		require.NoError(t, json.Unmarshal([]byte(out), &res))
		require.Equal(t, tc.want, res.Path, "%v", tc.args)
	}
}
