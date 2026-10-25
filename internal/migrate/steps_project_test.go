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

// A format-2 project upgrades through the 2→3 step (then on through 3→4),
// and each store
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
		To:     5,
		Backup: cfgPath(root, "config.yaml.v2.old"),
		Steps:  []string{project2to3Desc, project3to4Desc, project4to5Desc},
		Actions: []Action{
			{Op: "set", Path: settings, Key: "spec.config.directory", From: "docs/specs", To: "../docs/specs"},
			{Op: "set", Path: settings, Key: "plan.config.directory", From: ".spektacular/plans", To: "plans"},
			{Op: "set", Path: settings, Key: "changelog.config.directory", To: "changelog"},
			{Op: "set", Path: settings, Key: "epic_split_threshold", To: "moderate"},
			{Op: "set", Path: settings, Key: "epic.provider", To: "file"},
			{Op: "set", Path: settings, Key: "epic.strict_dependencies", To: "false"},
			{Op: "set", Path: settings, Key: "epic.config.directory", To: "epics"},
			{Op: "set", Path: settings, Key: "log.provider", To: "file"},
			{Op: "set", Path: settings, Key: "log.level", To: "standard"},
			{Op: "set", Path: settings, Key: "log.retention", To: "10"},
			{Op: "set", Path: settings, Key: "log.config.directory", To: "logs"},
			{Op: "ignore", Path: cfgPath(root, ".gitignore"), To: "logs/"},
		},
	}}, rep.Files)

	want := "schema: 5\n" +
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
		"        directory: changelog\n" +
		"epic_split_threshold: moderate\n" +
		"epic:\n" +
		"    provider: file\n" +
		"    strict_dependencies: false\n" +
		"    config:\n" +
		"        directory: epics\n" +
		"log:\n" +
		"    provider: file\n" +
		"    level: standard\n" +
		"    retention: 10\n" +
		"    config:\n" +
		"        directory: logs\n"
	require.Equal(t, want, string(readFile(t, settings)))

	// The upgraded file loads with every store where format 2 kept it.
	cfg, err := config.FromYAMLFile(settings)
	require.NoError(t, err)
	require.Equal(t, "docs/specs", cfg.Spec.Config.Directory)
	require.Equal(t, ".spektacular/plans", cfg.Plan.Config.Directory)
	require.Equal(t, ".spektacular/changelog", cfg.Changelog.Config.Directory)
	require.Equal(t, ".spektacular/epics", cfg.Epic.Config.Directory)
}

// An absolute store folder means the same place under either rule, so the
// 2→3 step leaves it exactly as written and reports no change for it; the
// only changes reported are the 3→4 step's epic defaults.
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
	require.Equal(t, []string{project2to3Desc, project3to4Desc, project4to5Desc}, rep.Files[0].Steps)
	require.Equal(t, []Action{
		{Op: "set", Path: settings, Key: "epic_split_threshold", To: "moderate"},
		{Op: "set", Path: settings, Key: "epic.provider", To: "file"},
		{Op: "set", Path: settings, Key: "epic.strict_dependencies", To: "false"},
		{Op: "set", Path: settings, Key: "epic.config.directory", To: "epics"},
		{Op: "set", Path: settings, Key: "log.provider", To: "file"},
		{Op: "set", Path: settings, Key: "log.level", To: "standard"},
		{Op: "set", Path: settings, Key: "log.retention", To: "10"},
		{Op: "set", Path: settings, Key: "log.config.directory", To: "logs"},
		{Op: "ignore", Path: cfgPath(root, ".gitignore"), To: "logs/"},
	}, rep.Files[0].Actions)

	require.Equal(t, "schema: 5\nwritten_by: 0.1.0\n"+body+
		"epic_split_threshold: moderate\n"+
		"epic:\n"+
		"    provider: file\n"+
		"    strict_dependencies: false\n"+
		"    config:\n"+
		"        directory: epics\n"+
		"log:\n"+
		"    provider: file\n"+
		"    level: standard\n"+
		"    retention: 10\n"+
		"    config:\n"+
		"        directory: logs\n", string(readFile(t, settings)))
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
			require.Equal(t, 5, rep.Files[0].To)
			require.Equal(t, []string{project1to2Desc, project2to3Desc, project3to4Desc, project4to5Desc}, rep.Files[0].Steps)

			got, err := config.PeekSchema(settings)
			require.NoError(t, err)
			require.Equal(t, 5, got)

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

// A format-3 project upgrades through the 3→4 step alone: every absent epic
// setting is written with its default, a value already present is kept and
// not reported, and a second apply changes nothing.
func TestApply_Format3ProjectGainsEpicDefaultsKeepingExistingValues(t *testing.T) {
	body := "written_by: 0.0.9\n" +
		"name: epics\n" +
		"agent: claude\n" +
		"skills_version: 0.1.0\n" +
		"epic_split_threshold: strict\n" +
		"epic:\n" +
		"    config:\n" +
		"        directory: ../docs/epics\n" +
		"repos:\n" +
		"    - name: epics\n" +
		"      location: .\n"
	root := writeProject(t, "schema: 3\n"+body)
	writeCurrentRepo(t, root)
	settings := cfgPath(root, "config.yaml")

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "upgraded", rep.Status)
	require.Equal(t, []FileReport{{
		Path:   settings,
		Kind:   KindProject,
		From:   3,
		To:     5,
		Backup: cfgPath(root, "config.yaml.v3.old"),
		Steps:  []string{project3to4Desc, project4to5Desc},
		Actions: []Action{
			{Op: "set", Path: settings, Key: "epic.provider", To: "file"},
			{Op: "set", Path: settings, Key: "epic.strict_dependencies", To: "false"},
			{Op: "set", Path: settings, Key: "log.provider", To: "file"},
			{Op: "set", Path: settings, Key: "log.level", To: "standard"},
			{Op: "set", Path: settings, Key: "log.retention", To: "10"},
			{Op: "set", Path: settings, Key: "log.config.directory", To: "logs"},
			{Op: "ignore", Path: cfgPath(root, ".gitignore"), To: "logs/"},
		},
	}}, rep.Files)

	want := "schema: 5\n" +
		"written_by: 0.1.0\n" +
		"name: epics\n" +
		"agent: claude\n" +
		"skills_version: 0.1.0\n" +
		"epic_split_threshold: strict\n" +
		"epic:\n" +
		"    config:\n" +
		"        directory: ../docs/epics\n" +
		"    provider: file\n" +
		"    strict_dependencies: false\n" +
		"repos:\n" +
		"    - name: epics\n" +
		"      location: .\n" +
		"log:\n" +
		"    provider: file\n" +
		"    level: standard\n" +
		"    retention: 10\n" +
		"    config:\n" +
		"        directory: logs\n"
	require.Equal(t, want, string(readFile(t, settings)))

	cfg, err := config.FromYAMLFile(settings)
	require.NoError(t, err)
	require.Equal(t, config.EpicSplitThresholdStrict, cfg.EpicSplitThreshold)
	require.Equal(t, config.ProviderFile, cfg.Epic.Provider)
	require.False(t, cfg.Epic.StrictDependencies)
	require.Equal(t, "docs/epics", cfg.Epic.Config.Directory)

	again, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "up_to_date", again.Status)
	require.Equal(t, want, string(readFile(t, settings)))
}

// format4Body is a format-4 project's settings after its schema line, with
// the given debug section (empty for none).
func format4Body(debug string) string {
	return "written_by: 0.1.0\n" +
		"name: logs\n" +
		"agent: claude\n" +
		"skills_version: 0.1.0\n" +
		"auto_commit: off\n" +
		debug +
		"repos:\n" +
		"    - name: logs\n" +
		"      location: .\n" +
		"epic_split_threshold: moderate\n" +
		"epic:\n" +
		"    provider: file\n" +
		"    strict_dependencies: false\n" +
		"    config:\n" +
		"        directory: epics\n"
}

// A format-4 project's debug setting becomes the log section: debug on is
// the verbose level, debug off or absent the standard level. The debug
// section is gone, the other settings are byte-for-byte unchanged, and the
// upgraded file loads.
func TestApply_Format4DebugBecomesLogLevel(t *testing.T) {
	cases := []struct {
		name      string
		debug     string
		wantLevel string
		removed   Action
	}{
		{name: "debug on", debug: "debug:\n    enabled: true\n", wantLevel: "verbose", removed: Action{Op: "set", Key: "debug.enabled", From: "true"}},
		{name: "debug off", debug: "debug:\n    enabled: false\n", wantLevel: "standard", removed: Action{Op: "set", Key: "debug.enabled", From: "false"}},
		{name: "debug absent", debug: "", wantLevel: "standard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeProject(t, "schema: 4\n"+format4Body(tc.debug))
			writeCurrentRepo(t, root)
			settings := cfgPath(root, "config.yaml")

			rep, err := Apply(applyOpts(root))
			require.NoError(t, err)
			require.Equal(t, "upgraded", rep.Status)
			require.Len(t, rep.Files, 1)
			require.Equal(t, 4, rep.Files[0].From)
			require.Equal(t, 5, rep.Files[0].To)
			require.Equal(t, []string{project4to5Desc}, rep.Files[0].Steps)
			want := []Action{
				{Op: "set", Path: settings, Key: "log.provider", To: "file"},
				{Op: "set", Path: settings, Key: "log.level", To: tc.wantLevel},
				{Op: "set", Path: settings, Key: "log.retention", To: "10"},
				{Op: "set", Path: settings, Key: "log.config.directory", To: "logs"},
			}
			if tc.removed.Op != "" {
				tc.removed.Path = settings
				want = append(want, tc.removed)
			}
			want = append(want, Action{Op: "ignore", Path: cfgPath(root, ".gitignore"), To: "logs/"})
			require.Equal(t, want, rep.Files[0].Actions)

			require.Equal(t, "schema: 5\n"+format4Body("")+
				"log:\n"+
				"    provider: file\n"+
				"    level: "+tc.wantLevel+"\n"+
				"    retention: 10\n"+
				"    config:\n"+
				"        directory: logs\n", string(readFile(t, settings)))

			cfg, err := config.FromYAMLFile(settings)
			require.NoError(t, err)
			require.Equal(t, tc.wantLevel, cfg.Log.Level)
			require.Equal(t, "file", cfg.Log.Provider)
			require.Equal(t, 10, cfg.Log.Retention)
			require.Equal(t, "logs", cfg.Log.Config.Directory)
		})
	}
}

// A log key already present is kept and not reported, even when debug says
// otherwise.
func TestApply_Format4KeepsExistingLogKeys(t *testing.T) {
	root := writeProject(t, "schema: 4\n"+format4Body("debug:\n    enabled: true\n")+
		"log:\n    level: off\n    retention: 3\n")
	writeCurrentRepo(t, root)
	settings := cfgPath(root, "config.yaml")

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, []Action{
		{Op: "set", Path: settings, Key: "log.provider", To: "file"},
		{Op: "set", Path: settings, Key: "log.config.directory", To: "logs"},
		{Op: "set", Path: settings, Key: "debug.enabled", From: "true"},
		{Op: "ignore", Path: cfgPath(root, ".gitignore"), To: "logs/"},
	}, rep.Files[0].Actions)

	cfg, err := config.FromYAMLFile(settings)
	require.NoError(t, err)
	require.Equal(t, "off", cfg.Log.Level)
	require.Equal(t, 3, cfg.Log.Retention)
	require.NotContains(t, string(readFile(t, settings)), "debug")
}

// The ignore action creates a missing .gitignore with the logs line, and
// appends it once to an existing one, keeping its content; a second upgrade
// of an already-current project adds nothing.
func TestApply_Format4IgnoresLogsFolderOnce(t *testing.T) {
	t.Run("missing .gitignore is created", func(t *testing.T) {
		root := writeProject(t, "schema: 4\n"+format4Body(""))
		writeCurrentRepo(t, root)

		_, err := Apply(applyOpts(root))
		require.NoError(t, err)
		require.Equal(t, "logs/\n", string(readFile(t, cfgPath(root, ".gitignore"))))
	})

	t.Run("existing .gitignore gains the line once", func(t *testing.T) {
		root := writeProject(t, "schema: 4\n"+format4Body(""))
		writeCurrentRepo(t, root)
		gi := cfgPath(root, ".gitignore")
		require.NoError(t, os.WriteFile(gi, []byte("*.tmp\nrepos/"), 0644))

		_, err := Apply(applyOpts(root))
		require.NoError(t, err)
		require.Equal(t, "*.tmp\nrepos/\nlogs/\n", string(readFile(t, gi)))

		again, err := Apply(applyOpts(root))
		require.NoError(t, err)
		require.Equal(t, "up_to_date", again.Status)
		require.Equal(t, "*.tmp\nrepos/\nlogs/\n", string(readFile(t, gi)))

		// Performing the action again directly is a no-op too.
		require.NoError(t, perform([]Action{{Op: "ignore", Path: gi, To: "logs/"}}))
		require.Equal(t, "*.tmp\nrepos/\nlogs/\n", string(readFile(t, gi)))
	})

	t.Run("a line already present is not duplicated", func(t *testing.T) {
		root := writeProject(t, "schema: 4\n"+format4Body(""))
		writeCurrentRepo(t, root)
		gi := cfgPath(root, ".gitignore")
		require.NoError(t, os.WriteFile(gi, []byte("logs/\n*.tmp\n"), 0644))

		_, err := Apply(applyOpts(root))
		require.NoError(t, err)
		require.Equal(t, "logs/\n*.tmp\n", string(readFile(t, gi)))
	})
}

// A preview lists the ignore action and changes nothing on disk.
func TestInspect_Format4PreviewListsIgnoreAndChangesNothing(t *testing.T) {
	root := writeProject(t, "schema: 4\n"+format4Body("debug:\n    enabled: true\n"))
	writeCurrentRepo(t, root)
	before := snapshotDir(t, root)

	rep, err := Inspect(root, testVersion)
	require.NoError(t, err)
	require.True(t, rep.DryRun)
	require.Contains(t, rep.Files[0].Actions, Action{Op: "ignore", Path: cfgPath(root, ".gitignore"), To: "logs/"})
	require.Equal(t, before, snapshotDir(t, root))
}

// Migration never reads, moves or deletes the old debug folder.
func TestApply_Format4LeavesDebugFolderUntouched(t *testing.T) {
	root := writeProject(t, "schema: 4\n"+format4Body("debug:\n    enabled: true\n"))
	writeCurrentRepo(t, root)
	debugDir := cfgPath(root, "debug")
	require.NoError(t, os.MkdirAll(debugDir, 0755))
	files := map[string]string{
		"20261001T000000.000000000Z_claude_spec_x.jsonl": "{\"seq\":0}\n",
		"session-log.jsonl": "old\n",
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(debugDir, name), []byte(body), 0644))
	}

	_, err := Apply(applyOpts(root))
	require.NoError(t, err)

	entries, err := os.ReadDir(debugDir)
	require.NoError(t, err)
	require.Len(t, entries, len(files))
	for name, body := range files {
		require.Equal(t, body, string(readFile(t, filepath.Join(debugDir, name))))
	}
}
