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
// the skills version this build installs.
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
	t.Chdir(t.TempDir())
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
		writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
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
		stdout, stderr, code := runRootCmd(t, "spec", "file", "read", "feature")
		require.Equal(t, 0, code)
		require.Equal(t, "stored body", stdout)
		require.Empty(t, stderr)

		stdout, stderr, code = runRootCmd(t, "spec", "file", "read", "missing")
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
	t.Chdir(t.TempDir())
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
		writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
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

		stdout, _, code = runRootCmd(t, "spec", "file", "read", "missing")
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
		stdout, stderr, code := runRootCmd(t, "spec", "file", "read", "missing")
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
	t.Chdir(t.TempDir())
	t.Run("unrecognized subcommand names the offending word and the group", func(t *testing.T) {
		stdout, _, code := runRootCmd(t, "plan", "resume", "somename")
		require.Equal(t, 1, code)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.True(t, er.IsError)
		require.Equal(t, "unknown_subcommand", er.Code)
		require.Equal(t, `unknown subcommand "resume" for "spektacular plan"`, er.Message)
		require.Equal(t, "run one of: file, goto, new, steps, task-id", er.NextAction)
	})

	t.Run("missing subcommand reports no subcommand given", func(t *testing.T) {
		stdout, _, code := runRootCmd(t, "spec")
		require.Equal(t, 1, code)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.Equal(t, "unknown_subcommand", er.Code)
		require.Equal(t, `no subcommand given for "spektacular spec"`, er.Message)
		require.Equal(t, "run one of: amend, file, goto, new, steps", er.NextAction)
	})

	t.Run("missing epic subcommand names the epic verbs", func(t *testing.T) {
		stdout, _, code := runRootCmd(t, "epic")
		require.Equal(t, 1, code)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.Equal(t, "unknown_subcommand", er.Code)
		require.Equal(t, `no subcommand given for "spektacular epic"`, er.Message)
		require.Equal(t, "run one of: delete, list, merge, order, read, split, summary, worktree, write", er.NextAction)
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
	t.Chdir(t.TempDir())
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
		"spec":      {"new", "goto", "steps"},
		"plan":      {"new", "goto", "steps"},
		"implement": {"new", "goto", "steps"},
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
