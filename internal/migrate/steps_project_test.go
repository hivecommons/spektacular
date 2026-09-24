package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// writeCurrentRepo gives the project's colocated repo a current repo.yaml,
// so a project-only upgrade reports nothing else.
func writeCurrentRepo(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.WriteFile(cfgPath(root, "repo.yaml"), []byte("schema: 2\nwritten_by: 0.1.0\n"), 0644))
}

// A format-2 project upgrades through the 2→3 step alone, and each store
// folder is re-expressed relative to the settings folder so it still names
// the same place on disk: a custom `docs/specs` becomes `../docs/specs`, the
// old default `.spektacular/plans` becomes `plans`, and an absent changelog
// key is written out explicitly as `changelog`.
func TestApply_Format2ProjectReexpressesStoreFolders(t *testing.T) {
	root := writeProject(t, "schema: 2\n"+
		"written_by: 0.0.9\n"+
		"name: custom\n"+
		"agent: claude\n"+
		"skills_version: 0.1.0\n"+
		"spec:\n"+
		"    provider: file\n"+
		"    config:\n"+
		"        directory: docs/specs\n"+
		"plan:\n"+
		"    provider: file\n"+
		"    config:\n"+
		"        directory: .spektacular/plans\n"+
		"repos:\n"+
		"    - name: custom\n"+
		"      location: .\n")
	writeCurrentRepo(t, root)
	settings := cfgPath(root, "config.yaml")

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "upgraded", rep.Status)
	require.Equal(t, []FileReport{{
		Path:   settings,
		Kind:   KindProject,
		From:   2,
		To:     3,
		Backup: cfgPath(root, "config.yaml.v2.old"),
		Steps:  []string{project2to3Desc},
		Actions: []Action{
			{Op: "set", Path: settings, Key: "spec.config.directory", From: "docs/specs", To: "../docs/specs"},
			{Op: "set", Path: settings, Key: "plan.config.directory", From: ".spektacular/plans", To: "plans"},
			{Op: "set", Path: settings, Key: "changelog.config.directory", To: "changelog"},
		},
	}}, rep.Files)

	want := "schema: 3\n" +
		"written_by: 0.1.0\n" +
		"name: custom\n" +
		"agent: claude\n" +
		"skills_version: 0.1.0\n" +
		"spec:\n" +
		"    provider: file\n" +
		"    config:\n" +
		"        directory: ../docs/specs\n" +
		"plan:\n" +
		"    provider: file\n" +
		"    config:\n" +
		"        directory: plans\n" +
		"repos:\n" +
		"    - name: custom\n" +
		"      location: .\n" +
		"changelog:\n" +
		"    config:\n" +
		"        directory: changelog\n"
	require.Equal(t, want, string(readFile(t, settings)))

	// The upgraded file loads with every store where format 2 kept it.
	cfg, err := config.FromYAMLFile(settings)
	require.NoError(t, err)
	require.Equal(t, "docs/specs", cfg.Spec.Config.Directory)
	require.Equal(t, ".spektacular/plans", cfg.Plan.Config.Directory)
	require.Equal(t, ".spektacular/changelog", cfg.Changelog.Config.Directory)
}

// An absolute store folder means the same place under either rule, so the
// 2→3 step leaves it exactly as written and reports no change for it.
func TestApply_Format2AbsoluteStoreFoldersAreUnchanged(t *testing.T) {
	root := t.TempDir()
	specs := filepath.Join(root, "docs", "specs")
	plans := filepath.Join(root, "docs", "plans")
	changelog := filepath.Join(root, "docs", "changelog")
	body := "name: absolute\n" +
		"spec:\n" +
		"    config:\n" +
		"        directory: " + specs + "\n" +
		"plan:\n" +
		"    config:\n" +
		"        directory: " + plans + "\n" +
		"changelog:\n" +
		"    config:\n" +
		"        directory: " + changelog + "\n" +
		"repos:\n" +
		"    - name: absolute\n" +
		"      location: .\n"
	cfgDir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	settings := cfgPath(root, "config.yaml")
	require.NoError(t, os.WriteFile(settings, []byte("schema: 2\nwritten_by: 0.0.9\n"+body), 0644))
	writeCurrentRepo(t, root)

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Len(t, rep.Files, 1)
	require.Equal(t, []string{project2to3Desc}, rep.Files[0].Steps)
	require.Equal(t, []Action{}, rep.Files[0].Actions)

	require.Equal(t, "schema: 3\nwritten_by: 0.1.0\n"+body, string(readFile(t, settings)))
}

// A project two formats behind reaches the current format in one run, and
// the typed config loaded afterwards has the same store folders, repos and
// agent the project had before the upgrade. The expected values are written
// out by hand from what format 1 meant: store folders relative to the
// project root, defaulting to .spektacular/<store>.
func TestApply_TwoFormatsBehindReachesCurrentWithSameSettings(t *testing.T) {
	cases := []struct {
		name          string
		setup         func(t *testing.T) string
		wantSpec      string
		wantPlan      string
		wantChangelog string
		wantRepos     []config.RepoEntry
		wantAgent     string
	}{
		{
			name:          "default folders",
			setup:         func(t *testing.T) string { return newProject(t, "legacy_single") },
			wantSpec:      ".spektacular/specs",
			wantPlan:      ".spektacular/plans",
			wantChangelog: ".spektacular/changelog",
			wantRepos:     []config.RepoEntry{{Name: "legacy", Location: "."}},
			wantAgent:     "claude",
		},
		{
			name: "custom spec folder",
			setup: func(t *testing.T) string {
				return writeProject(t, "name: custom\n"+
					"agent: codex\n"+
					"spec:\n"+
					"    config:\n"+
					"        directory: docs/specs\n"+
					"repos:\n"+
					"    - name: custom\n"+
					"      location: .\n"+
					"    - name: api\n"+
					"      location: ../api\n")
			},
			wantSpec:      "docs/specs",
			wantPlan:      ".spektacular/plans",
			wantChangelog: ".spektacular/changelog",
			wantRepos: []config.RepoEntry{
				{Name: "custom", Location: "."},
				{Name: "api", Location: "../api"},
			},
			wantAgent: "codex",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.setup(t)
			settings := cfgPath(root, "config.yaml")

			rep, err := Apply(applyOpts(root))
			require.NoError(t, err)
			require.Equal(t, "upgraded", rep.Status)
			require.Equal(t, 1, rep.Files[0].From)
			require.Equal(t, 3, rep.Files[0].To)
			require.Equal(t, []string{project1to2Desc, project2to3Desc}, rep.Files[0].Steps)

			got, err := config.PeekSchema(settings)
			require.NoError(t, err)
			require.Equal(t, 3, got)

			cfg, err := config.FromYAMLFile(settings)
			require.NoError(t, err)
			require.Equal(t, tc.wantSpec, cfg.Spec.Config.Directory)
			require.Equal(t, tc.wantPlan, cfg.Plan.Config.Directory)
			require.Equal(t, tc.wantChangelog, cfg.Changelog.Config.Directory)
			require.Equal(t, tc.wantRepos, cfg.Repos)
			require.Equal(t, tc.wantAgent, cfg.Agent)
		})
	}
}
