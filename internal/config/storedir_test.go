package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// storeDirsFixture is a valid config.yaml body whose spec, plan and
// changelog directories are the given file-form values.
func storeDirsFixture(spec, plan, changelog string) string {
	return withProjectSchema("name: testproj\n" +
		"spec:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    directory: " + spec + "\n" +
		"plan:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    directory: " + plan + "\n" +
		"changelog:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    directory: " + changelog + "\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: ..\n")
}

// Store folders: a relative directory in config.yaml is resolved from the
// folder holding the file, so `x` in <root>/.spektacular/config.yaml puts
// that store in <root>/.spektacular/x.
func TestFromYAMLFile_StoreDirsResolveFromSettingsFolder(t *testing.T) {
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(storeDirsFixture("x", "px", "cx")), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, ".spektacular/x", cfg.Spec.Config.Directory)
	require.Equal(t, ".spektacular/px", cfg.Plan.Config.Directory)
	require.Equal(t, ".spektacular/cx", cfg.Changelog.Config.Directory)
}

// Store folders: an absolute directory inside the project root loads as the
// equivalent project-root-relative path.
func TestFromYAMLFile_AbsoluteStoreDirInsideRootLoadsRelative(t *testing.T) {
	root, path := projectConfigPath(t)
	body := storeDirsFixture(
		filepath.Join(root, "docs", "specs"),
		filepath.Join(root, "docs", "plans"),
		filepath.Join(root, ".spektacular", "changelog"),
	)
	require.NoError(t, os.WriteFile(path, []byte(body), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "docs/specs", cfg.Spec.Config.Directory)
	require.Equal(t, "docs/plans", cfg.Plan.Config.Directory)
	require.Equal(t, ".spektacular/changelog", cfg.Changelog.Config.Directory)
}

// Store folders: a directory that resolves outside the project root is
// refused with config_invalid and a next action naming the offending key and
// how to correct it.
func TestFromYAMLFile_StoreDirOutsideProjectIsRefused(t *testing.T) {
	cases := []struct {
		name           string
		spec, plan, cl string
		wantMessage    string
		wantNextAction string
	}{
		{
			name: "spec", spec: "../../outside", plan: "plans", cl: "changelog",
			wantMessage:    `spec.config.directory "../../outside" is outside the project`,
			wantNextAction: "set `spec.config.directory` to a folder inside the project, relative to the folder holding config.yaml (e.g. `specs`)",
		},
		{
			name: "plan", spec: "specs", plan: "../../outside", cl: "changelog",
			wantMessage:    `plan.config.directory "../../outside" is outside the project`,
			wantNextAction: "set `plan.config.directory` to a folder inside the project, relative to the folder holding config.yaml (e.g. `plans`)",
		},
		{
			name: "changelog", spec: "specs", plan: "plans", cl: "../../outside",
			wantMessage:    `changelog.config.directory "../../outside" is outside the project`,
			wantNextAction: "set `changelog.config.directory` to a folder inside the project, relative to the folder holding config.yaml (e.g. `changelog`)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, path := projectConfigPath(t)
			require.NoError(t, os.WriteFile(path, []byte(storeDirsFixture(tc.spec, tc.plan, tc.cl)), 0644))

			_, err := FromYAMLFile(path)
			require.Error(t, err)
			var er *output.ErrorResponse
			require.ErrorAs(t, err, &er)
			require.Equal(t, "config_invalid", er.Code)
			require.Equal(t, tc.wantMessage, er.Message)
			require.Equal(t, tc.wantNextAction, er.NextAction)
		})
	}
}

// Store folders: loading a file and writing it back to the same path keeps
// the directory values as the author wrote them.
func TestToYAMLFile_StoreDirsRoundTripInFileForm(t *testing.T) {
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(storeDirsFixture("x", "px", "cx")), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "directory: x\n")
	require.Contains(t, string(raw), "directory: px\n")
	require.Contains(t, string(raw), "directory: cx\n")
	require.NotContains(t, string(raw), ".spektacular/", "the in-memory project-rooted form must never be written")

	reloaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, ".spektacular/x", reloaded.Spec.Config.Directory)
	require.Equal(t, ".spektacular/px", reloaded.Plan.Config.Directory)
	require.Equal(t, ".spektacular/cx", reloaded.Changelog.Config.Directory)
}

// Store folders: a new default config writes its store folders relative to
// the settings folder.
func TestToYAMLFile_NewDefaultWritesSettingsRelativeStoreDirs(t *testing.T) {
	_, path := projectConfigPath(t)
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "directory: specs\n")
	require.Contains(t, string(raw), "directory: plans\n")
	require.Contains(t, string(raw), "directory: changelog\n")
	require.NotContains(t, string(raw), ".spektacular/")
}

// Store folders: when a loaded directory is changed in memory, the written
// value is re-derived from the new location rather than the one read.
func TestToYAMLFile_ChangedStoreDirIsRewrittenInFileForm(t *testing.T) {
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(storeDirsFixture("x", "plans", "changelog")), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	// "moved" rather than a one-letter name: yaml quotes a bare `y`, as it
	// is a YAML 1.1 boolean.
	cfg.Spec.Config.Directory = ".spektacular/moved"
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "directory: moved\n")
	require.NotContains(t, string(raw), "directory: x\n")

	reloaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, ".spektacular/moved", reloaded.Spec.Config.Directory)
}

// Store folders: the outside-the-project check binds only config.yaml. A
// repo.yaml changelog directory pointing above the repo's settings folder is
// relative to that folder and is accepted as written.
func TestRepoConfigFromYAMLFile_ChangelogAboveSettingsFolderIsAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), RepoConfigFileName)
	body := "changelog:\n  provider: file\n  config:\n    directory: ../changelog\n"
	require.NoError(t, os.WriteFile(path, []byte(withSchema(body)), 0644))

	cfg, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "../changelog", cfg.Changelog.Config.Directory)
}
