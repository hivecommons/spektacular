package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file covers Phase 2.1 (the `version check` command): the pure
// classification helper, and the command's observable behavior through
// runRootCmd — the same wrapper Execute() uses in production.
//
// Every expected version string below is a hand-maintained oracle: the dev
// default the binary compiles with is the literal "0.1.0" (cmd/root.go's
// `version` var), asserted as that literal and never derived from the
// `version` var or versionString() at runtime.

// writeVersionCheckFile seeds dir/.spektacular/version with content.
func writeVersionCheckFile(t *testing.T, dir, content string) string {
	t.Helper()
	dataDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	path := filepath.Join(dataDir, "version")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// runVersionCheckJSON runs `version check` and decodes stdout into a generic
// map so tests can assert both present and absent keys.
func runVersionCheckJSON(t *testing.T) (m map[string]any, code int) {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "version", "check")
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &m))
	return m, code
}

// TestClassifyVersion pins the pure classification decision with hand-written
// expected values for every recorded-content shape the file can hold.
func TestClassifyVersion(t *testing.T) {
	tests := []struct {
		name          string
		recorded      string
		current       string
		wantStatus    string
		wantInstalled string
	}{
		{name: "exact match", recorded: "0.1.0", current: "0.1.0", wantStatus: "match", wantInstalled: "0.1.0"},
		{name: "match with trailing newline", recorded: "0.1.0\n", current: "0.1.0", wantStatus: "match", wantInstalled: "0.1.0"},
		{name: "match with surrounding spaces", recorded: "  0.1.0  ", current: "0.1.0", wantStatus: "match", wantInstalled: "0.1.0"},
		{name: "differing versions", recorded: "9.9.9", current: "0.1.0", wantStatus: "mismatch", wantInstalled: "9.9.9"},
		{name: "empty string", recorded: "", current: "0.1.0", wantStatus: "missing", wantInstalled: ""},
		{name: "whitespace-only string", recorded: " \n\t ", current: "0.1.0", wantStatus: "missing", wantInstalled: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, installed := classifyVersion(tt.recorded, tt.current)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantInstalled, installed)
		})
	}
}

// TestVersionCheck_Match: a version file recording the running binary's own
// version reports "match", both version fields, and — per the spec — no
// action text at all (the key must be absent from the JSON, not just empty).
func TestVersionCheck_Match(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeVersionCheckFile(t, dir, "0.1.0\n")

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, false, m["error"])
	require.Equal(t, "match", m["status"])
	require.Equal(t, "0.1.0", m["installed_version"])
	require.Equal(t, "0.1.0", m["current_version"])
	require.NotContains(t, m, "action", "matching versions must carry no action text")
}

// TestVersionCheck_Mismatch: a recorded version differing from the binary's
// reports "mismatch" with a non-empty re-run instruction, still exit 0 —
// staleness is a report, not a failure. The command must also be strictly
// read-only: the version file's bytes and the .spektacular listing are
// identical before and after the invocation.
func TestVersionCheck_Mismatch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := writeVersionCheckFile(t, dir, "9.9.9\n")

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	entriesBefore, err := os.ReadDir(filepath.Join(dir, ".spektacular"))
	require.NoError(t, err)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, false, m["error"])
	require.Equal(t, "mismatch", m["status"])
	require.Equal(t, "9.9.9", m["installed_version"])
	require.Equal(t, "0.1.0", m["current_version"])
	action, ok := m["action"].(string)
	require.True(t, ok, "mismatch must carry an action string")
	require.NotEmpty(t, action)
	require.Contains(t, action, "re-run")

	// Read-only guarantee.
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "version check must never rewrite the version file")
	entriesAfter, err := os.ReadDir(filepath.Join(dir, ".spektacular"))
	require.NoError(t, err)
	namesOf := func(entries []os.DirEntry) []string {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		return names
	}
	require.Equal(t, namesOf(entriesBefore), namesOf(entriesAfter), "version check must not create or remove files in .spektacular")
}

// Criterion 4: the stale-version remediation composes the correct init
// command from the recorded config — the configured command plus the recorded
// agent — after a real init has established both.
func TestVersionCheck_MismatchComposesInitCommandFromConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())
	writeVersionCheckFile(t, dir, "9.9.9\n")

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "mismatch", m["status"])
	action, ok := m["action"].(string)
	require.True(t, ok, "mismatch must carry an action string")
	require.Contains(t, action, "`spektacular init claude`",
		"the remediation must compose the configured command with the recorded agent")
}

// TestVersionCheck_Missing: no .spektacular directory (and hence no version
// file) is the pre-versioning install state — "missing", no
// installed_version key, and a re-run instruction, exit 0.
func TestVersionCheck_Missing(t *testing.T) {
	t.Chdir(t.TempDir())

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, false, m["error"])
	require.Equal(t, "missing", m["status"])
	require.NotContains(t, m, "installed_version", "missing state has nothing installed to report")
	require.Equal(t, "0.1.0", m["current_version"])
	action, ok := m["action"].(string)
	require.True(t, ok, "missing must carry an action string")
	require.NotEmpty(t, action)
	require.Contains(t, action, "re-run")
}

// TestVersionCheck_WhitespaceOnlyFile: a version file that exists but holds
// only whitespace classifies as "missing", not a mismatch, exit 0.
func TestVersionCheck_WhitespaceOnlyFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeVersionCheckFile(t, dir, " \n\t\n")

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, false, m["error"])
	require.Equal(t, "missing", m["status"])
	require.NotContains(t, m, "installed_version")
}

// TestVersionCheck_UnreadableFileIsGenuineFault: a version file that exists
// but cannot be read (here: it is a directory, so os.ReadFile fails with a
// non-IsNotExist error) is a genuine fault — exit 1 with the error envelope
// and the dedicated version_file_unreadable code, never a silent "missing".
func TestVersionCheck_UnreadableFileIsGenuineFault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".spektacular", "version"), 0o755))

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "version", "check")
	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "version_file_unreadable", er.Code)
}

// TestVersionCheck_Schema: --schema short-circuits before any filesystem
// access and prints the output contract, including the status enum.
func TestVersionCheck_Schema(t *testing.T) {
	t.Chdir(t.TempDir())
	resetRootCmd(t)

	stdout, stderr, code := runRootCmd(t, "version", "check", "--schema")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Contains(t, stdout, `"status"`)
	require.Contains(t, stdout, "match")
	require.Contains(t, stdout, "mismatch")
	require.Contains(t, stdout, "missing")
	require.Contains(t, stdout, "migration_needed")
}

// TestVersionCheck_MigrationNeeded covers Phase 3.2 acceptance criteria:
// migration prompt appears when legacy config detected, prompt explains
// changes, and normal version check proceeds when repo.yaml exists.
func TestVersionCheck_MigrationNeeded(t *testing.T) {
	tests := []struct {
		name           string
		setupFiles     func(t *testing.T, dir string)
		wantStatus     string
		wantAction     bool
		actionContains []string
	}{
		{
			name: "legacy config triggers migration prompt",
			setupFiles: func(t *testing.T, dir string) {
				dataDir := filepath.Join(dir, ".spektacular")
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				configPath := filepath.Join(dataDir, "config.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte("project: test\n"), 0o644))
			},
			wantStatus: "migration_needed",
			wantAction: true,
			actionContains: []string{
				"legacy single-file format",
				"config.yaml.old",
				"repo.yaml",
				"migration",
			},
		},
		{
			name: "already migrated proceeds with normal version check",
			setupFiles: func(t *testing.T, dir string) {
				dataDir := filepath.Join(dir, ".spektacular")
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				configPath := filepath.Join(dataDir, "config.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte("project: test\n"), 0o644))
				repoPath := filepath.Join(dataDir, "repo.yaml")
				require.NoError(t, os.WriteFile(repoPath, []byte("description: test\n"), 0o644))
				versionPath := filepath.Join(dataDir, "version")
				require.NoError(t, os.WriteFile(versionPath, []byte("0.1.0\n"), 0o644))
			},
			wantStatus: "match",
			wantAction: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			tt.setupFiles(t, dir)

			m, code := runVersionCheckJSON(t)
			require.Equal(t, 0, code)
			require.Equal(t, false, m["error"])
			require.Equal(t, tt.wantStatus, m["status"])

			if tt.wantAction {
				action, ok := m["action"].(string)
				require.True(t, ok, "action field must be present and be a string")
				require.NotEmpty(t, action)
				for _, substr := range tt.actionContains {
					require.Contains(t, action, substr)
				}
			} else {
				require.NotContains(t, m, "action", "no action should be present for normal version check")
			}
		})
	}
}

// TestDetectMigrationNeeded covers Phase 1.1 acceptance criteria: detection
// correctly identifies legacy configs, skips when already migrated, and
// handles missing directories gracefully.
func TestDetectMigrationNeeded(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    func(t *testing.T, dir string)
		wantMigration bool
		wantErr       bool
	}{
		{
			name: "legacy config without repo.yaml needs migration",
			setupFiles: func(t *testing.T, dir string) {
				dataDir := filepath.Join(dir, ".spektacular")
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				configPath := filepath.Join(dataDir, "config.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte("project: test\n"), 0o644))
			},
			wantMigration: true,
			wantErr:       false,
		},
		{
			name: "already migrated with both files skips migration",
			setupFiles: func(t *testing.T, dir string) {
				dataDir := filepath.Join(dir, ".spektacular")
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				configPath := filepath.Join(dataDir, "config.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte("project: test\n"), 0o644))
				repoPath := filepath.Join(dataDir, "repo.yaml")
				require.NoError(t, os.WriteFile(repoPath, []byte("description: test\n"), 0o644))
			},
			wantMigration: false,
			wantErr:       false,
		},
		{
			name: "missing .spektacular directory skips migration",
			setupFiles: func(t *testing.T, dir string) {
				// Don't create .spektacular directory
			},
			wantMigration: false,
			wantErr:       false,
		},
		{
			name: "missing config.yaml skips migration",
			setupFiles: func(t *testing.T, dir string) {
				dataDir := filepath.Join(dir, ".spektacular")
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				// Don't create config.yaml
			},
			wantMigration: false,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setupFiles(t, dir)

			dataDir := filepath.Join(dir, ".spektacular")
			gotMigration, err := detectMigrationNeeded(dataDir)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantMigration, gotMigration)
		})
	}
}

// TestExecuteMigration covers Phase 2.1 acceptance criteria: migration
// creates backup, writes repo.yaml, verifies files, and rolls back on error.
func TestExecuteMigration(t *testing.T) {
	tests := []struct {
		name        string
		setupFiles  func(t *testing.T, dir string)
		wantErr     bool
		checkResult func(t *testing.T, dir string)
	}{
		{
			name: "successful migration creates backup and repo.yaml",
			setupFiles: func(t *testing.T, dir string) {
				dataDir := filepath.Join(dir, ".spektacular")
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				configPath := filepath.Join(dataDir, "config.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte("project: test\n"), 0o644))
			},
			wantErr: false,
			checkResult: func(t *testing.T, dir string) {
				dataDir := filepath.Join(dir, ".spektacular")
				// Verify backup exists
				backupPath := filepath.Join(dataDir, "config.yaml.old")
				backupData, err := os.ReadFile(backupPath)
				require.NoError(t, err)
				require.Equal(t, "project: test\n", string(backupData))

				// Verify repo.yaml exists
				repoPath := filepath.Join(dataDir, "repo.yaml")
				_, err = os.Stat(repoPath)
				require.NoError(t, err)

				// Verify original config.yaml still exists
				configPath := filepath.Join(dataDir, "config.yaml")
				_, err = os.Stat(configPath)
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setupFiles(t, dir)

			dataDir := filepath.Join(dir, ".spektacular")
			cfg := config.NewDefaultRepoConfig()
			cfg.Description = "Test project"
			cfg.Role = "application"
			cfg.Tags = []string{"test"}

			err := executeMigration(dataDir, &cfg)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, dir)
			}
		})
	}
}

// TestScanProjectMetadata covers Phase 1.2 acceptance criteria: scanner
// extracts metadata from project files and provides defaults when files
// are missing or unparseable.
func TestScanProjectMetadata(t *testing.T) {
	tests := []struct {
		name            string
		setupFiles      func(t *testing.T, dir string)
		wantDescription string
		wantRole        string
		wantTags        []string
	}{
		{
			name: "extracts description from README H1 title",
			setupFiles: func(t *testing.T, dir string) {
				readme := "# My Awesome Project\n\nSome description here."
				require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644))
			},
			wantDescription: "My Awesome Project",
			wantRole:        "application",
			wantTags:        []string{"general"},
		},
		{
			name: "extracts description from README first paragraph",
			setupFiles: func(t *testing.T, dir string) {
				readme := "This is the first paragraph.\n\nSecond paragraph."
				require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644))
			},
			wantDescription: "This is the first paragraph.",
			wantRole:        "application",
			wantTags:        []string{"general"},
		},
		{
			name: "detects Go project via go.mod",
			setupFiles: func(t *testing.T, dir string) {
				goMod := "module github.com/example/project\n\ngo 1.21\n"
				require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644))
			},
			wantDescription: "A Spektacular project",
			wantRole:        "application",
			wantTags:        []string{"go"},
		},
		{
			name: "detects Node.js project via package.json",
			setupFiles: func(t *testing.T, dir string) {
				packageJSON := `{"name": "my-project", "version": "1.0.0"}`
				require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644))
			},
			wantDescription: "A Spektacular project",
			wantRole:        "application",
			wantTags:        []string{"nodejs"},
		},
		{
			name: "provides defaults when all heuristics fail",
			setupFiles: func(t *testing.T, dir string) {
				// Don't create any project files
			},
			wantDescription: "A Spektacular project",
			wantRole:        "application",
			wantTags:        []string{"general"},
		},
		{
			name: "combines multiple heuristics",
			setupFiles: func(t *testing.T, dir string) {
				readme := "# Spektacular CLI Tool\n\nA command-line tool."
				require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644))
				goMod := "module github.com/example/cli\n\ngo 1.21\n"
				require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644))
				dockerfile := "FROM golang:1.21\n"
				require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644))
			},
			wantDescription: "Spektacular CLI Tool",
			wantRole:        "application",
			wantTags:        []string{"go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setupFiles(t, dir)

			cfg, err := scanProjectMetadata(dir)
			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.Equal(t, tt.wantDescription, cfg.Description)
			require.Equal(t, tt.wantRole, cfg.Role)
			require.Equal(t, tt.wantTags, cfg.Tags)
		})
	}
}

// Phase 2.3 criterion 6: upgrading a project from the older single-file
// configuration must produce a repo.yaml that is accepted without further
// correction. Writing the file is not enough — the migrator seeds it from
// config.NewDefaultRepoConfig(), so this asserts the result actually loads
// back through config.RepoConfigFromYAMLFile, passing the guard that refuses a
// knowledge block in the superseded form, and that the store it declares is
// the repo's own.
func TestExecuteMigration_WritesRepoConfigThatLoadsWithoutCorrection(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "config.yaml"), []byte("project: test\n"), 0o644))

	cfg := config.NewDefaultRepoConfig()
	cfg.Description = "Test project"
	cfg.Role = "application"
	cfg.Tags = []string{"test"}
	require.NoError(t, executeMigration(dataDir, &cfg))

	loaded, err := config.RepoConfigFromYAMLFile(filepath.Join(dataDir, config.RepoConfigFileName))
	require.NoError(t, err, "a migrated repo.yaml must load without further correction")
	require.Equal(t, "file", loaded.Knowledge.Provider)
	require.Equal(t, "knowledge", loaded.Knowledge.Config.Location)
	require.Equal(t, "Test project", loaded.Description)
}
