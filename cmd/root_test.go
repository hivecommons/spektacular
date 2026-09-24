package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/sessionlog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

// This file tests cmd/root.go's response-wrapper behavior (runRoot): every
// command's success or failure outcome must be funneled through the same
// envelope and the same output stream, exactly once, with no cobra-generated
// boilerplate. Business logic for each command family is covered elsewhere
// (spec_test.go, plan_test.go, implement_test.go, knowledge_test.go,
// plan_file_test.go, file_test.go, init_test.go, skill.go's own tests); the
// tests here only exercise a representative success and failure per family to
// prove the wrapper itself behaves correctly.

// resetRootCmd returns the package-global rootCmd tree to its pristine state,
// both now and when t ends. Every command is a package global, so a flag
// parsed by one test otherwise stays set for whichever test runs next, which
// makes results depend on test order. It resets every flag on every command,
// its backing variable and its Changed bit together, rather than a
// hand-maintained list, so a newly added flag cannot be missed. Output
// streams are left to the caller (see setupImplementCmd).
func resetRootCmd(t *testing.T) {
	t.Helper()
	reset := func() {
		resetCommandFlags(t, rootCmd)
		rootCmd.SetArgs(nil)
	}
	reset()
	t.Cleanup(reset)
}

// resetCommandFlags resets every local and persistent flag on c and its
// descendants to its default value.
func resetCommandFlags(t *testing.T, c *cobra.Command) {
	t.Helper()
	for _, fs := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
		fs.VisitAll(func(f *pflag.Flag) {
			// A slice flag's Set appends, so its default can only be
			// restored by replacing the whole value.
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				require.NoError(t, sv.Replace(nil), "resetting flag --%s on %q", f.Name, c.CommandPath())
			} else {
				require.NoError(t, f.Value.Set(f.DefValue), "resetting flag --%s on %q", f.Name, c.CommandPath())
			}
			f.Changed = false
		})
	}
	for _, sub := range c.Commands() {
		resetCommandFlags(t, sub)
	}
}

// runRootCmd invokes rootCmd's args through runRoot — the same wrapper
// Execute() uses in production, rather than calling rootCmd.Execute()
// directly — so tests observe the wrapper's formatted output and exit code,
// not just cobra's raw error return.
func runRootCmd(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	out, errBuf := setupImplementCmd(t)
	rootCmd.SetArgs(args)
	code = runRoot()
	return out.String(), errBuf.String(), code
}

// currentConfigHeader is the preamble every hand-written project config.yaml
// fixture carries so the loaders accept it: the current settings format and
// the skills version this (dev) build installs.
func currentConfigHeader() string {
	return fmt.Sprintf("schema: %d\nskills_version: %s\n", config.CurrentProjectSchema, version)
}

// currentRepoHeader is the preamble every hand-written repo.yaml fixture
// carries so the loader accepts it: the current repo settings format.
func currentRepoHeader() string {
	return fmt.Sprintf("schema: %d\n", config.CurrentRepoSchema)
}

// writeCurrentConfig writes body, prefixed with currentConfigHeader, as
// dir/.spektacular/config.yaml, creating the folder as needed.
func writeCurrentConfig(t *testing.T, dir, body string) {
	t.Helper()
	dataDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, config.ProjectConfigFileName), []byte(currentConfigHeader()+body), 0o644))
}

// writeCurrentRepoConfig writes body, prefixed with currentRepoHeader, as
// dir/.spektacular/repo.yaml, creating the folder as needed.
func writeCurrentRepoConfig(t *testing.T, dir, body string) {
	t.Helper()
	dataDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, config.RepoConfigFileName), []byte(currentRepoHeader()+body), 0o644))
}

// writeSpecFileFixture lays out a temp project rooted at a t.TempDir() (and
// chdirs into it) with the spec directory configured to docs/specs and one
// existing file seeded at feature.md, for exercising `spec file`.
func writeSpecFileFixture(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../docs/specs\n")
	specPath := filepath.Join(dir, "docs", "specs", "feature.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte("stored body"), 0o644))
	return dir
}

// Criterion 1: every command family reports both success and failure on
// stdout, and stderr stays empty in both cases.
func TestWrapper_SuccessAndFailureBothStreamOnStdoutOnly(t *testing.T) {
	t.Run("spec", func(t *testing.T) {
		// Separate temp dirs per invocation: `spec new`'s resume check runs
		// before its name validation, so a successful call followed by a
		// second call in the same directory would be treated as a resume
		// (success) instead of exercising the validation failure.
		t.Run("success", func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "")
			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
			require.Equal(t, 0, code)
			require.NotEmpty(t, stdout)
			require.Empty(t, stderr)
		})
		t.Run("failure", func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "")
			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":" billing"}`)
			require.Equal(t, 1, code)
			require.NotEmpty(t, stdout)
			require.Empty(t, stderr)
		})
	})

	t.Run("plan", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "")

		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "plan", "new", "--data", `{"name":"myplan"}`)
		require.Equal(t, 0, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)

		resetRootCmd(t)
		stdout, stderr, code = runRootCmd(t, "plan", "new")
		require.Equal(t, 1, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)
	})

	t.Run("implement", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		dataDir := filepath.Join(dir, ".spektacular")
		writeSpecCommandConfig(t, dir, "")
		writeFixturePlan(t, dataDir, "fixture")

		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "implement", "new", "--data", `{"name":"fixture"}`)
		require.Equal(t, 0, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)

		resetRootCmd(t)
		stdout, stderr, code = runRootCmd(t, "implement", "new", "--data", `{"name":"nosuch"}`)
		require.Equal(t, 1, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)
	})

	t.Run("knowledge", func(t *testing.T) {
		twoScopeProject(t)

		stdout, stderr, err := runKnowledge(t, "sources")
		require.NoError(t, err)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)

		stdout, stderr, err = runKnowledge(t, "read")
		require.Error(t, err)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)
	})

	t.Run("file", func(t *testing.T) {
		writeSpecFileFixture(t)

		// file read's success path is a deliberate, documented exception: it
		// writes raw file bytes to stdout, not the JSON envelope. Only its
		// failure path goes through the envelope.
		stdout, stderr, code := runRootCmd(t, "spec", "file", "read", "feature.md")
		require.Equal(t, 0, code)
		require.Equal(t, "stored body", stdout)
		require.Empty(t, stderr)

		stdout, stderr, code = runRootCmd(t, "spec", "file", "read", "missing.md")
		require.Equal(t, 1, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)
	})

	t.Run("skill", func(t *testing.T) {
		stdout, stderr, code := runRootCmd(t, "skill", "determine-feature-slug")
		require.Equal(t, 0, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)

		stdout, stderr, code = runRootCmd(t, "skill", "does-not-exist")
		require.Equal(t, 1, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)
	})

	t.Run("init", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		stdout, stderr, code := runRootCmd(t, "init", "claude")
		require.Equal(t, 0, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)

		stdout, stderr, code = runRootCmd(t, "init", "does-not-exist")
		require.Equal(t, 1, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)
	})

	t.Run("version", func(t *testing.T) {
		// Success representative: a fresh dir with no .spektacular at all —
		// missing state is a successful report, not a failure. Failure
		// representative: config.yaml exists but is a directory, so
		// reading it fails with a genuine (non-IsNotExist) error.
		t.Run("success", func(t *testing.T) {
			t.Chdir(t.TempDir())
			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "version", "check")
			require.Equal(t, 0, code)
			require.NotEmpty(t, stdout)
			require.Empty(t, stderr)
		})
		t.Run("failure", func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".spektacular", "config.yaml"), 0o755))
			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "version", "check")
			require.Equal(t, 1, code)
			require.NotEmpty(t, stdout)
			require.Empty(t, stderr)
		})
	})
}

// Criterion 2: a failing command's response carries "error": true and
// runRoot() returns 1; a succeeding command's response carries "error": false
// and runRoot() returns 0. Checked across every family whose success path
// goes through the JSON envelope; init and file read's success paths are
// deliberate plain-text/raw-bytes exceptions (see TestWrapper_
// SuccessAndFailureBothStreamOnStdoutOnly), so only their failure paths are
// asserted here.
func TestWrapper_ErrorDiscriminantAndExitCode(t *testing.T) {
	assertSuccessEnvelope := func(t *testing.T, stdout string, code int) {
		t.Helper()
		require.Equal(t, 0, code)
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &m))
		require.Equal(t, false, m["error"])
	}
	assertFailureEnvelope := func(t *testing.T, stdout string, code int) {
		t.Helper()
		require.Equal(t, 1, code)
		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.True(t, er.IsError)
	}

	t.Run("spec", func(t *testing.T) {
		// Separate temp dirs per invocation — see the identical note in
		// TestWrapper_SuccessAndFailureBothStreamOnStdoutOnly.
		t.Run("success", func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "")
			resetRootCmd(t)
			stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
			assertSuccessEnvelope(t, stdout, code)
		})
		t.Run("failure", func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "")
			resetRootCmd(t)
			stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":" billing"}`)
			assertFailureEnvelope(t, stdout, code)
		})
	})

	t.Run("plan", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "")

		resetRootCmd(t)
		stdout, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"myplan"}`)
		assertSuccessEnvelope(t, stdout, code)

		resetRootCmd(t)
		stdout, _, code = runRootCmd(t, "plan", "new")
		assertFailureEnvelope(t, stdout, code)
	})

	t.Run("implement", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		dataDir := filepath.Join(dir, ".spektacular")
		writeSpecCommandConfig(t, dir, "")
		writeFixturePlan(t, dataDir, "fixture")

		resetRootCmd(t)
		stdout, _, code := runRootCmd(t, "implement", "new", "--data", `{"name":"fixture"}`)
		assertSuccessEnvelope(t, stdout, code)

		resetRootCmd(t)
		stdout, _, code = runRootCmd(t, "implement", "new", "--data", `{"name":"nosuch"}`)
		assertFailureEnvelope(t, stdout, code)
	})

	t.Run("knowledge", func(t *testing.T) {
		twoScopeProject(t)

		stdout, _, err := runKnowledge(t, "sources")
		require.NoError(t, err)
		assertSuccessEnvelope(t, stdout, 0)

		stdout, _, err = runKnowledge(t, "read")
		require.Error(t, err)
		assertFailureEnvelope(t, stdout, 1)
	})

	t.Run("file", func(t *testing.T) {
		writeSpecFileFixture(t)

		// `list` (unlike `read`) goes through the standard envelope on
		// success, so it stands in as this family's envelope-shaped success
		// representative.
		stdout, _, code := runRootCmd(t, "spec", "file", "list")
		assertSuccessEnvelope(t, stdout, code)

		stdout, _, code = runRootCmd(t, "spec", "file", "read", "missing.md")
		assertFailureEnvelope(t, stdout, code)
	})

	t.Run("skill", func(t *testing.T) {
		stdout, _, code := runRootCmd(t, "skill", "determine-feature-slug")
		assertSuccessEnvelope(t, stdout, code)

		stdout, _, code = runRootCmd(t, "skill", "does-not-exist")
		assertFailureEnvelope(t, stdout, code)
	})

	t.Run("init", func(t *testing.T) {
		// init's success path is a deliberate plain-text exception like file
		// read's (see cmd/init.go's runInit — it fmt.Fprintf's human-readable
		// lines, not the JSON envelope); its exit code is covered by
		// TestWrapper_SuccessAndFailureBothStreamOnStdoutOnly. Only the
		// failure path is envelope-shaped, so only it is asserted here.
		dir := t.TempDir()
		t.Chdir(dir)

		stdout, _, code := runRootCmd(t, "init", "does-not-exist")
		assertFailureEnvelope(t, stdout, code)
	})

	t.Run("version", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			t.Chdir(t.TempDir())
			resetRootCmd(t)
			stdout, _, code := runRootCmd(t, "version", "check")
			assertSuccessEnvelope(t, stdout, code)
		})
		t.Run("failure", func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".spektacular", "config.yaml"), 0o755))
			resetRootCmd(t)
			stdout, _, code := runRootCmd(t, "version", "check")
			assertFailureEnvelope(t, stdout, code)
		})
	})
}

// Criterion 3: a failure is printed exactly once on stdout, with no
// additional cobra-generated usage/help text anywhere in stdout or stderr —
// guards against the old double-print bug where cobra's own default error
// printer ran before the tool's own fallback.
func TestWrapper_FailureIsPrintedExactlyOnceWithNoCobraBoilerplate(t *testing.T) {
	assertNoCobraBoilerplate := func(t *testing.T, stdout, stderr string, code int) {
		t.Helper()
		require.Equal(t, 1, code)
		require.Empty(t, stderr, "cobra's default error/usage printer must be silenced")
		require.Equal(t, 1, strings.Count(stdout, `"error": true`), "the failure envelope must appear exactly once")
		require.NotContains(t, stdout, "Usage:")
		require.NotContains(t, stdout, "Error:")
	}

	t.Run("spec", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "")
		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":" billing"}`)
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("plan", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "")
		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "plan", "new")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("implement", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "")
		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "implement", "new", "--data", `{"name":"nosuch"}`)
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("knowledge", func(t *testing.T) {
		twoScopeProject(t)
		stdout, stderr, err := runKnowledge(t, "read")
		require.Error(t, err)
		assertNoCobraBoilerplate(t, stdout, stderr, 1)
	})

	t.Run("file", func(t *testing.T) {
		writeSpecFileFixture(t)
		stdout, stderr, code := runRootCmd(t, "spec", "file", "read", "missing.md")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("skill", func(t *testing.T) {
		stdout, stderr, code := runRootCmd(t, "skill", "does-not-exist")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("init", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		stdout, stderr, code := runRootCmd(t, "init", "does-not-exist")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("version", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".spektacular", "config.yaml"), 0o755))
		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "version", "check")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	// An unknown top-level command/flag never reaches any RunE — cobra itself
	// rejects it — so this is the one case where runRoot's fallback
	// (toErrorResponse's internal_error branch) is what stops cobra's default
	// printer from producing a second, differently-shaped failure message.
	t.Run("unknown top-level command", func(t *testing.T) {
		stdout, stderr, code := runRootCmd(t, "no-such-command")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	// Unlike an unknown top-level command, an unrecognized (or missing)
	// subcommand under a recognized group (e.g. `spec list` or bare `spec`)
	// used to never reach any RunE either — but for a different reason:
	// cobra's Find() resolves as far as "spec" and, since the group itself
	// had no Run/RunE, silently printed its plain-text help and returned nil
	// (exit 0), not an error. runUnknownSubcommand closes that gap.
	t.Run("unrecognized subcommand under a group", func(t *testing.T) {
		stdout, stderr, code := runRootCmd(t, "spec", "list")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("group invoked with no subcommand", func(t *testing.T) {
		stdout, stderr, code := runRootCmd(t, "spec")
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})

	t.Run("root invoked with no arguments", func(t *testing.T) {
		stdout, stderr, code := runRootCmd(t)
		assertNoCobraBoilerplate(t, stdout, stderr, code)
	})
}

// TestUnknownSubcommand_ReturnsStructuredErrorNamingValidSubcommands covers
// the actual incident this fix addresses: an agent invents a plausible but
// nonexistent subcommand (e.g. `plan resume <name>`, mirroring a real
// mis-invocation seen in practice) or omits one entirely, and must get a
// structured, retryable error rather than cobra's silent, exit-0 help dump.
func TestUnknownSubcommand_ReturnsStructuredErrorNamingValidSubcommands(t *testing.T) {
	t.Run("unrecognized subcommand names the offending word and the group", func(t *testing.T) {
		stdout, _, code := runRootCmd(t, "plan", "resume", "somename")
		require.Equal(t, 1, code)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.True(t, er.IsError)
		require.Equal(t, "unknown_subcommand", er.Code)
		require.Equal(t, `unknown subcommand "resume" for "spektacular plan"`, er.Message)
		require.Equal(t, "run one of: file, goto, new, status, steps", er.NextAction)
	})

	t.Run("missing subcommand reports no subcommand given", func(t *testing.T) {
		stdout, _, code := runRootCmd(t, "spec")
		require.Equal(t, 1, code)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.Equal(t, "unknown_subcommand", er.Code)
		require.Equal(t, `no subcommand given for "spektacular spec"`, er.Message)
		require.Equal(t, "run one of: file, goto, new, status, steps", er.NextAction)
	})

	t.Run("--help still works and is unaffected", func(t *testing.T) {
		stdout, _, code := runRootCmd(t, "spec", "--help")
		require.Equal(t, 0, code)
		require.Contains(t, stdout, "Manage spek workflow")
		require.NotContains(t, stdout, `"error"`)
	})
}

// TestGotoStepRequired_ReturnsStructuredErrorAcrossAllKinds covers the
// `goto` counterpart to the `new` commands' name_required fix: a missing or
// empty step must return a structured error with a concrete next_action
// (the correct --data shape, plus which `steps` subcommand discovers valid
// step names), not a bare, unremediated string. This is the exact error an
// agent hit mid-session trying `plan goto <name> <step>` (positional-style,
// no --data at all) before finding the correct flag shape.
func TestGotoStepRequired_ReturnsStructuredErrorAcrossAllKinds(t *testing.T) {
	kinds := []struct {
		kind      string
		stepsHint string
	}{
		{"spec", "spec steps"},
		{"plan", "plan steps"},
		{"implement", "implement steps"},
	}

	for _, k := range kinds {
		t.Run(k.kind, func(t *testing.T) {
			t.Run("no --data at all", func(t *testing.T) {
				stdout, _, code := runRootCmd(t, k.kind, "goto")
				require.Equal(t, 1, code)

				var er output.ErrorResponse
				require.NoError(t, json.Unmarshal([]byte(stdout), &er))
				require.Equal(t, "step_required", er.Code)
				require.Equal(t, "no step was provided", er.Message)
				require.Contains(t, er.NextAction, `--data '{"step":"<step_name>"}'`)
				require.Contains(t, er.NextAction, k.stepsHint)
			})

			t.Run("--data with no step key", func(t *testing.T) {
				stdout, _, code := runRootCmd(t, k.kind, "goto", "--data", `{}`)
				require.Equal(t, 1, code)

				var er output.ErrorResponse
				require.NoError(t, json.Unmarshal([]byte(stdout), &er))
				require.Equal(t, "step_required", er.Code)
				require.Equal(t, `"step" is missing or empty in --data`, er.Message)
				require.Contains(t, er.NextAction, `--data '{"step":"<step_name>"}'`)
				require.Contains(t, er.NextAction, k.stepsHint)
			})
		})
	}
}

// sessionLogDebugDir is the project-relative directory runRoot's
// sessionLogDir() helper resolves to, mirrored here so tests can list and
// read session log files back independently of root.go.
func sessionLogDebugDir(dir string) string {
	return filepath.Join(dir, ".spektacular", "debug")
}

// sessionLogFiles returns every session log file present in dir's debug
// directory, in no particular order.
func sessionLogFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(sessionLogDebugDir(dir), "*.jsonl"))
	require.NoError(t, err)
	return matches
}

// sessionLogFilePath returns the path to the sole session log file in dir's
// debug directory. Every test in this file drives at most one workflow
// session per temp dir, so exactly one match is expected — finding zero or
// more than one is a test-authoring error, not a case to handle gracefully.
func sessionLogFilePath(t *testing.T, dir string) string {
	t.Helper()
	matches := sessionLogFiles(t, dir)
	require.Len(t, matches, 1, "expected exactly one session log file in %s", sessionLogDebugDir(dir))
	return matches[0]
}

// Phase 1.2, criterion 1: with debug off — either because the project config
// has no debug section at all (the default), or because it explicitly sets
// debug.enabled: false — running a command must produce no session log file,
// and the command's own observable behavior (stdout, exit code) must be
// exactly what it was before this phase.
func TestSessionLog_DisabledProducesNoRecordFile(t *testing.T) {
	t.Run("no debug section in config", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "")
		resetRootCmd(t)

		stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)

		require.Empty(t, sessionLogFiles(t, dir), "session log must not be created when debug is off")
	})

	t.Run("explicit debug.enabled false", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: false\n")
		resetRootCmd(t)

		stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code)
		require.NotEmpty(t, stdout)
		require.Empty(t, stderr)

		require.Empty(t, sessionLogFiles(t, dir), "session log must not be created when debug is explicitly off")
	})
}

// Phase 1.2, criterion 2: with debug on, a command that succeeds and a
// command that is rejected (fails validation, returning an
// *output.ErrorResponse via the standard envelope machinery) each produce a
// corresponding entry in the session log — neither is silently dropped — and
// that entry's exit code and response body match exactly what the caller
// itself received.
func TestSessionLog_EnabledRecordsSuccessAndRejection(t *testing.T) {
	// runRoot records os.Args[1:] (the real process argv), not the args
	// handed to rootCmd.SetArgs by runRootCmd — that override only affects
	// cobra's own parsing. Inside this test binary os.Args is constant
	// regardless of which subcommand runRootCmd drives, so every entry
	// recorded during this test carries the same Command value: the test
	// binary's own invocation, captured here for comparison.
	wantArgv := append([]string(nil), os.Args[1:]...)

	readSoleEvent := func(t *testing.T, dir string) sessionlog.Event {
		t.Helper()
		data, err := os.ReadFile(sessionLogFilePath(t, dir))
		require.NoError(t, err)
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		require.Len(t, lines, 1, "exactly one recorded entry expected")
		var ev sessionlog.Event
		require.NoError(t, json.Unmarshal([]byte(lines[0]), &ev))
		return ev
	}

	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
		resetRootCmd(t)

		stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code)

		ev := readSoleEvent(t, dir)
		require.Equal(t, wantArgv, ev.Command)
		require.Equal(t, 0, ev.ExitCode)
		require.Equal(t, stdout, ev.Response)
		require.GreaterOrEqual(t, ev.DurationMS, int64(0))
	})

	t.Run("rejected", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
		resetRootCmd(t)

		stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":" billing"}`)
		require.Equal(t, 1, code)

		ev := readSoleEvent(t, dir)
		require.Equal(t, wantArgv, ev.Command)
		require.Equal(t, 1, ev.ExitCode)
		require.Equal(t, stdout, ev.Response)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(ev.Response), &er), "the recorded response must be the exact envelope body")
		require.True(t, er.IsError)
	})

	t.Run("errors", func(t *testing.T) {
		// Distinct from "rejected" above: malformed --data JSON never reaches
		// the *output.ErrorResponse builder machinery at all — it fails in
		// cmd/spec.go's own json.Unmarshal and returns a plain error, which
		// toErrorResponse falls back to wrapping as a generic internal_error.
		// This exercises that fallback path, not the validation-rejection one.
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
		resetRootCmd(t)

		stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{not-json`)
		require.Equal(t, 1, code)

		ev := readSoleEvent(t, dir)
		require.Equal(t, wantArgv, ev.Command)
		require.Equal(t, 1, ev.ExitCode)
		require.Equal(t, stdout, ev.Response)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(ev.Response), &er), "the recorded response must be the exact envelope body")
		require.True(t, er.IsError)
		require.Equal(t, "internal_error", er.Code)
	})
}

// Phase 1.2, criterion 3: turning debug on must not change what the caller
// receives, for a representative command from every command family — not
// just spec. This is the load-bearing "non-invasive" guarantee, checked as
// an explicit byte-identical comparison for each of the same command
// families TestWrapper_SuccessAndFailureBothStreamOnStdoutOnly already
// exercises (spec, plan, implement, knowledge, skill). "init" is
// deliberately excluded here: its behavior legitimately branches on whether
// a `.spektacular/config.yaml` already exists (it only writes one if none is
// present), so pre-seeding one with a debug-on config to flip the toggle
// would make the two runs asymmetric for a reason unrelated to this
// feature — not a fair byte-identical comparison.
func TestSessionLog_EnabledDoesNotChangeCallerVisibleOutput(t *testing.T) {
	type family struct {
		name string
		// run seeds a fresh temp dir (chdir'd) with the family's fixtures,
		// enables debug when debugOn is true, and returns the dir plus the
		// args to invoke.
		run func(t *testing.T, debugOn bool) (dir string, args []string)
	}

	// Both runs get a project config — the project gate makes one mandatory
	// for project-operating commands — so the debug toggle is the only
	// difference between the compared invocations. Spec IDs use the counter
	// method: the default timestamp method would name the two runs' specs
	// differently whenever they straddle a second boundary.
	withDebug := func(t *testing.T, dir string, debugOn bool) {
		const base = "spec:\n  id_method: counter\n"
		if !debugOn {
			writeSpecCommandConfig(t, dir, base)
			return
		}
		writeSpecCommandConfig(t, dir, base+"debug:\n  enabled: true\n")
	}

	families := []family{
		{
			name: "spec",
			run: func(t *testing.T, debugOn bool) (string, []string) {
				dir := t.TempDir()
				t.Chdir(dir)
				withDebug(t, dir, debugOn)
				resetRootCmd(t)
				return dir, []string{"spec", "new", "--data", `{"name":"billing"}`}
			},
		},
		{
			name: "plan",
			run: func(t *testing.T, debugOn bool) (string, []string) {
				dir := t.TempDir()
				t.Chdir(dir)
				withDebug(t, dir, debugOn)
				resetRootCmd(t)
				return dir, []string{"plan", "new", "--data", `{"name":"myplan"}`}
			},
		},
		{
			name: "implement",
			run: func(t *testing.T, debugOn bool) (string, []string) {
				dir := t.TempDir()
				t.Chdir(dir)
				dataDir := filepath.Join(dir, ".spektacular")
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				writeFixturePlan(t, dataDir, "fixture")
				withDebug(t, dir, debugOn)
				resetRootCmd(t)
				return dir, []string{"implement", "new", "--data", `{"name":"fixture"}`}
			},
		},
		{
			name: "knowledge",
			run: func(t *testing.T, debugOn bool) (string, []string) {
				root, _, _ := twoScopeProject(t)
				if debugOn {
					f, err := os.OpenFile(filepath.Join(root, ".spektacular", "config.yaml"), os.O_APPEND|os.O_WRONLY, 0o644)
					require.NoError(t, err)
					_, err = f.WriteString("debug:\n  enabled: true\n")
					require.NoError(t, err)
					require.NoError(t, f.Close())
				}
				resetRootCmd(t)
				return root, []string{"knowledge", "sources"}
			},
		},
		{
			name: "skill",
			run: func(t *testing.T, debugOn bool) (string, []string) {
				dir := t.TempDir()
				t.Chdir(dir)
				withDebug(t, dir, debugOn)
				return dir, []string{"skill", "determine-feature-slug"}
			},
		},
	}

	for _, f := range families {
		t.Run(f.name, func(t *testing.T) {
			offDir, offArgs := f.run(t, false)
			offOut, _, offCode := runRootCmd(t, offArgs...)

			onDir, onArgs := f.run(t, true)
			onOut, _, onCode := runRootCmd(t, onArgs...)

			require.Equal(t, offCode, onCode)

			// Responses can embed each run's own absolute temp-dir path
			// (e.g. spec_path), so the two necessarily-distinct temp dirs
			// are normalized to a common placeholder before the
			// byte-identical comparison; everything else must match exactly.
			normalize := func(s, dir string) string { return strings.ReplaceAll(s, dir, "TESTDIR") }
			require.Equal(t, normalize(offOut, offDir), normalize(onOut, onDir))
		})
	}
}

// readSessionLogEvents reads back every recorded entry in dir's session log,
// in the order they were appended, for tests that drive more than one
// command through runRootCmd and need to inspect each recorded
// sessionlog.Event in turn. Phase 1.2's TestSessionLog_EnabledRecordsSuccess
// AndRejection has its own single-event equivalent inlined as a local
// closure (readSoleEvent) — this is the multi-event generalization needed by
// Phase 2.1's state-tracking tests below.
func readSessionLogEvents(t *testing.T, dir string) []sessionlog.Event {
	t.Helper()
	data, err := os.ReadFile(sessionLogFilePath(t, dir))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	events := make([]sessionlog.Event, len(lines))
	for i, line := range lines {
		require.NoError(t, json.Unmarshal([]byte(line), &events[i]))
	}
	return events
}

// Phase 2.1, criterion 1: a command that genuinely advances a piece of work
// in progress is recorded as having advanced it — both the founding call of
// a workflow (StateBefore nil, since no state.json exists yet) and a
// subsequent real step advance (StateBefore and StateAfter both non-nil, but
// meaningfully different).
func TestSessionLog_AdvancedTrueWhenStateChanges(t *testing.T) {
	t.Run("founding call: spec new in a fresh project", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
		resetRootCmd(t)

		_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code)

		events := readSessionLogEvents(t, dir)
		require.Len(t, events, 1)
		ev := events[0]
		require.Nil(t, ev.StateBefore, "no state.json exists before the founding call")
		require.NotNil(t, ev.StateAfter)
		require.Equal(t, "spec", ev.StateAfter.Kind)
		require.Equal(t, "new", ev.StateAfter.CurrentStep)
		require.True(t, ev.Advanced)
	})

	t.Run("subsequent real step advance: goto to a different, reachable step", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
		resetRootCmd(t)

		_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code)

		_, _, code = runRootCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
		require.Equal(t, 0, code)

		_, _, code = runRootCmd(t, "spec", "goto", "--data", `{"step":"overview"}`)
		require.Equal(t, 0, code)

		_, _, code = runRootCmd(t, "spec", "goto", "--data", `{"step":"requirements"}`)
		require.Equal(t, 0, code)

		events := readSessionLogEvents(t, dir)
		require.Len(t, events, 4)
		ev := events[3]
		require.NotNil(t, ev.StateBefore)
		require.Equal(t, "overview", ev.StateBefore.CurrentStep)
		require.NotNil(t, ev.StateAfter)
		require.Equal(t, "requirements", ev.StateAfter.CurrentStep)
		require.True(t, ev.Advanced)
	})
}

// Phase 2.1, criterion 2: the goto-to-current-step scenario from the
// goto-to-current-step-is-a-silent-noop gotcha — repeating the step the
// workflow is already on returns exit code 0 and no error, and looks like a
// normal, successful step response, but must be recorded as Advanced: false
// with StateBefore and StateAfter identical.
func TestSessionLog_AdvancedFalseWhenGotoRepeatsCurrentStep(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
	resetRootCmd(t)

	_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)

	stdout, _, code := runRootCmd(t, "spec", "goto", "--data", `{"step":"new"}`)
	require.Equal(t, 0, code, "goto to the already-current step returns success, not an error")

	events := readSessionLogEvents(t, dir)
	require.Len(t, events, 2)
	ev := events[1]
	require.NotNil(t, ev.StateBefore)
	require.NotNil(t, ev.StateAfter)
	require.Equal(t, *ev.StateBefore, *ev.StateAfter, "goto to the current step is a silent no-op: state must be unchanged")
	require.False(t, ev.Advanced, "must not be reported as advanced despite exit code 0 and no error")
	require.Equal(t, 0, ev.ExitCode)
	require.Equal(t, stdout, ev.Response)
}

// Phase 2.1, criterion 3: a command unrelated to any piece of work in
// progress — run before any workflow has ever run in the project, so
// state.json does not exist — is recorded without error, with both
// StateBefore and StateAfter nil and Advanced false, and the command itself
// still succeeds normally.
func TestSessionLog_UnrelatedCommandRecordsNothingToAdvance(t *testing.T) {
	root, _, _ := twoScopeProject(t)
	f, err := os.OpenFile(filepath.Join(root, ".spektacular", "config.yaml"), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("debug:\n  enabled: true\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	resetRootCmd(t)

	require.NoFileExists(t, filepath.Join(root, ".spektacular", "state.json"), "no workflow has ever run in this project")

	stdout, _, code := runRootCmd(t, "knowledge", "sources")
	require.Equal(t, 0, code)

	events := readSessionLogEvents(t, root)
	require.Len(t, events, 1)
	ev := events[0]
	require.Nil(t, ev.StateBefore)
	require.Nil(t, ev.StateAfter)
	require.False(t, ev.Advanced)
	require.Equal(t, 0, ev.ExitCode)
	require.Equal(t, stdout, ev.Response)
}

// Phase 2.2, criterion 1: every entry recorded while working on the same
// named piece of work carries the same session id, including the founding
// call (whose StateBefore is nil, since state.json doesn't exist yet) and a
// later call that resumes the same instance.
func TestSessionLog_SameSessionIDAcrossFoundingAndResumedCalls(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// id_method: counter makes the resolved spec name deterministic
	// ("000001_billing") rather than timestamp-based, so the session id can
	// be asserted as an exact string.
	writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\nspec:\n  id_method: counter\n")
	resetRootCmd(t)

	_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)

	_, _, code = runRootCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
	require.Equal(t, 0, code)

	events := readSessionLogEvents(t, dir)
	require.Len(t, events, 2)

	const wantSessionID = "spec:000001_billing"
	require.Equal(t, wantSessionID, events[0].SessionID, "founding call must carry the session id, not the no-active-workflow sentinel")
	require.Equal(t, wantSessionID, events[1].SessionID, "resuming call on the same instance must carry the same session id")
}

// Phase 2.2, criterion 2: entries recorded while working on two differently
// named (or differently kinded) pieces of work never carry the same session
// id. Each piece of work gets its own fresh project, since a second `... new`
// call in a directory that already has state is a resume rather than a
// distinct new instance (see the note on TestWrapper_
// SuccessAndFailureBothStreamOnStdoutOnly's spec subtest).
func TestSessionLog_DistinctSessionIDsForDifferentNamedWork(t *testing.T) {
	newSpec := func(t *testing.T, name string) string {
		t.Helper()
		dir := t.TempDir()
		t.Chdir(dir)
		// id_method: counter makes the resolved spec name deterministic
		// ("000001_<name>") rather than timestamp-based.
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\nspec:\n  id_method: counter\n")
		resetRootCmd(t)

		_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"`+name+`"}`)
		require.Equal(t, 0, code)

		events := readSessionLogEvents(t, dir)
		require.Len(t, events, 1)
		return events[0].SessionID
	}

	newPlan := func(t *testing.T, name string) string {
		t.Helper()
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
		resetRootCmd(t)

		_, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"`+name+`"}`)
		require.Equal(t, 0, code)

		events := readSessionLogEvents(t, dir)
		require.Len(t, events, 1)
		return events[0].SessionID
	}

	t.Run("same kind, different names", func(t *testing.T) {
		billingID := newSpec(t, "billing")
		widgetID := newSpec(t, "widget")

		require.Equal(t, "spec:000001_billing", billingID)
		require.Equal(t, "spec:000001_widget", widgetID)
		require.NotEqual(t, billingID, widgetID)
	})

	t.Run("different kinds", func(t *testing.T) {
		specID := newSpec(t, "billing")
		planID := newPlan(t, "myplan")

		require.Equal(t, "spec:000001_billing", specID)
		require.Equal(t, "plan:myplan", planID)
		require.NotEqual(t, specID, planID)
	})
}

// Phase 2.2, criterion 3: a command issued before any piece of work has
// started (no state.json on disk yet) is still recorded, under the
// consistent "no-active-workflow" placeholder rather than being dropped or
// erroring.
func TestSessionLog_NoActiveWorkflowSessionIDBeforeAnyWorkflow(t *testing.T) {
	root, _, _ := twoScopeProject(t)
	f, err := os.OpenFile(filepath.Join(root, ".spektacular", "config.yaml"), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("debug:\n  enabled: true\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	resetRootCmd(t)

	require.NoFileExists(t, filepath.Join(root, ".spektacular", "state.json"), "no workflow has ever run in this project")

	stdout, _, code := runRootCmd(t, "knowledge", "sources")
	require.Equal(t, 0, code)
	require.NotEmpty(t, stdout)

	events := readSessionLogEvents(t, root)
	require.Len(t, events, 1)
	require.Equal(t, "no-active-workflow", events[0].SessionID)
}

// TestSessionLog_FilenameEncodesAgentAndSessionID asserts that the file a
// workflow's founding `new` call is recorded into is named after the
// project's configured agent and the session id (workflow kind + name), so
// a plain directory listing identifies every session without opening a
// file.
func TestSessionLog_FilenameEncodesAgentAndSessionID(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\nagent: testbot\n")
	resetRootCmd(t)

	_, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)

	path := sessionLogFilePath(t, dir)
	name := filepath.Base(path)
	require.Contains(t, name, "testbot")
	require.Contains(t, name, "plan_billing")
	require.Regexp(t, `^\d{8}T\d{6}\.\d{9}Z_`, name, "filename must start with a sortable timestamp")
}

// TestSessionLog_ForceRestartStartsANewFileNotAppendingToPrevious asserts
// that `--force` restarting a workflow — a genuinely new session under the
// same name, per resumeOrClear — mints a second, distinct log file rather
// than appending onto the abandoned session's log, and later commands
// resume into that newest file.
func TestSessionLog_ForceRestartStartsANewFileNotAppendingToPrevious(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
	resetRootCmd(t)

	_, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)
	first := sessionLogFilePath(t, dir)

	_, _, code = runRootCmd(t, "plan", "new", "--force", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)

	files := sessionLogFiles(t, dir)
	require.Len(t, files, 2, "the restart must mint a second file alongside the first, not overwrite or append to it")
	require.Contains(t, files, first, "the first session's file must be untouched, not renamed away")

	firstEvents, err := os.ReadFile(first)
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimRight(string(firstEvents), "\n"), "\n"), 1, "the abandoned session's file must still hold only its own event")

	_, _, code = runRootCmd(t, "plan", "goto", "--data", `{"step":"discovery"}`)
	require.Equal(t, 0, code)

	newest := ""
	for _, f := range files {
		if f != first {
			newest = f
		}
	}
	newestEvents, err := os.ReadFile(newest)
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimRight(string(newestEvents), "\n"), "\n"), 2, "the goto must resume into the restarted session's file, not the abandoned one")
}

// Criterion 3: the published interface of every command family other than
// knowledge is exactly what it was before the knowledge commands gained a
// flags block — an input schema and an output schema, and the error
// discriminant the writer injects into every response, and nothing else. The
// expected key set is hand-written and exhaustive, so a family picking up a
// "flags" key, or any other new top-level key, fails here rather than
// silently changing an interface its callers depend on.
func TestSchema_NonKnowledgeFamiliesPublishExactlyInputAndOutput(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	for family, subcommands := range map[string][]string{
		"spec":      {"new", "goto", "status", "steps"},
		"plan":      {"new", "goto", "status", "steps"},
		"implement": {"new", "goto", "status", "steps"},
		"repo":      {"add", "list"},
	} {
		t.Run(family, func(t *testing.T) {
			for _, sub := range subcommands {
				t.Run(sub, func(t *testing.T) {
					switch family {
					case "spec":
						resetRootCmd(t)
					case "plan":
						resetRootCmd(t)
					case "implement":
						resetRootCmd(t)
					case "repo":
						resetRootCmd(t)
					}

					stdout, stderr, code := runRootCmd(t, family, sub, "--schema")
					require.Equal(t, 0, code)
					require.Empty(t, stderr)

					var raw map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(stdout), &raw))

					keys := make([]string, 0, len(raw))
					for k := range raw {
						keys = append(keys, k)
					}
					require.ElementsMatch(t, []string{"input", "output", "error"}, keys,
						"`%s %s --schema` must publish exactly the interface it published before", family, sub)
				})
			}
		})
	}
}
