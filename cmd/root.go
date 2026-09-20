package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/sessionlog"
	"github.com/spf13/cobra"
)

// version and sha are set at build time via -ldflags
// (see dagger/main.go's Build function).
var (
	version = "0.1.0"
	sha     = ""
)

// globalFields holds the raw --fields JSON array string, available to all subcommands.
var globalFields string

var rootCmd = &cobra.Command{
	Use:     "spektacular",
	Short:   "Agent-driven tool for spec-driven development",
	Version: versionString(),
	RunE:    runUnknownSubcommand,
}

// runUnknownSubcommand handles a command group (e.g. spec/plan/implement,
// or the root command itself) invoked with no subcommand or an unrecognized
// one. Cobra's default behavior for a non-Runnable parent command in that
// situation is to print its plain-text help and exit 0 — indistinguishable,
// in a CLI whose entire contract is a structured JSON envelope, from a real
// response; that ambiguity has caused a driving agent to misread the help
// dump as valid output and bypass the CLI entirely. Giving every group
// command this RunE makes it Runnable, so cobra invokes this instead of
// silently falling through to that default: even a mistyped or missing
// subcommand now comes back as the same JSON error shape as everything
// else, naming the valid subcommands to retry with. `--help`/`-h` is
// unaffected — cobra intercepts those before this ever runs.
func runUnknownSubcommand(cmd *cobra.Command, args []string) error {
	var names []string
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() {
			names = append(names, c.Name())
		}
	}
	sort.Strings(names)

	message := fmt.Sprintf("no subcommand given for %q", cmd.CommandPath())
	if len(args) > 0 {
		message = fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath())
	}
	return output.NewError("unknown_subcommand", message).
		WithNextAction(fmt.Sprintf("run one of: %s", strings.Join(names, ", ")))
}

// versionString combines the build-time version and sha into the string
// Cobra prints for --version. sha is omitted when unset (e.g. local go
// build/go run, which never pass -ldflags).
func versionString() string {
	if sha == "" {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, sha)
}

func Execute() {
	os.Exit(runRoot())
}

// runRoot runs the root command and returns the process exit code. It is the
// single place a command's outcome (result or error) is translated into the
// response envelope and written to the output stream — extracted from
// Execute so tests can exercise the full wrapping behavior without an
// os.Exit call.
//
// When the project's debug.enabled config option is on, runRoot also
// records the command issued and the exact response returned to a local
// session log, for reconstructing a session after the fact. This is
// strictly additive: with the option off (the default), runRoot's behavior
// is byte-for-byte identical to what it is without any of this.
func runRoot() int {
	// The debug probe must tolerate running outside a project — the gate
	// belongs to the command handlers, not here.
	cfg, cfgErr := loadConfigLenient()
	debugEnabled := cfgErr == nil && cfg.Debug.Enabled

	var argv []string
	var start time.Time
	var orig io.Writer
	var buf *bytes.Buffer
	var stateBefore *sessionlog.StateSnapshot

	if debugEnabled {
		argv = os.Args[1:]
		start = time.Now()
		stateBefore = readStateSnapshot()
		orig = rootCmd.OutOrStdout()
		buf = &bytes.Buffer{}
		rootCmd.SetOut(io.MultiWriter(orig, buf))
	}

	executedCmd, err := rootCmd.ExecuteC()

	exitCode := 0
	if err != nil {
		output.WriteFailure(rootCmd.OutOrStdout(), toErrorResponse(err), globalFields)
		exitCode = 1
	}

	if debugEnabled {
		stateAfter := readStateSnapshot()
		advanced := stateAdvanced(stateBefore, stateAfter)
		if dir, pathErr := sessionLogDir(); pathErr == nil {
			sessionID := sessionlog.SessionID(stateAfter)
			isStart := isWorkflowStart(executedCmd, exitCode)
			logPath := sessionlog.LogFilePath(dir, cfg.Agent, sessionID, isStart, start)
			sessionlog.Record(logPath, sessionlog.Event{
				Timestamp:   start,
				SessionID:   sessionID,
				Command:     argv,
				DurationMS:  time.Since(start).Milliseconds(),
				ExitCode:    exitCode,
				Response:    buf.String(),
				StateBefore: stateBefore,
				StateAfter:  stateAfter,
				Advanced:    advanced,
			})
		}
		rootCmd.SetOut(orig)
	}

	return exitCode
}

// isWorkflowStart reports whether this invocation is the command that just
// began a brand-new workflow session — a `<kind> new` call (kind ∈
// spec/plan/implement) that actually succeeded in creating fresh state,
// rather than bouncing off an in-progress workflow (a workflow_in_progress
// error, which returns a non-nil error and so exitCode != 0) or running as
// --dry-run (which never touches the real state.json). It deliberately does
// not compare state before/after: a `--force` restart of the same named
// workflow produces a fresh state shaped identically to the state it
// replaced (same kind, name, and first current_step), so that comparison
// can't tell a real restart apart from a true no-op — the command's own
// identity and its --dry-run flag are the only reliable signal. It reads
// the actually-executed leaf command (from ExecuteC, not raw argv) so it
// stays correct regardless of where global flags land in the invocation.
// This is the one moment sessionlog.LogFilePath mints a new, timestamped
// log file instead of continuing to append to the current session's
// existing one.
func isWorkflowStart(executedCmd *cobra.Command, exitCode int) bool {
	if executedCmd == nil || exitCode != 0 {
		return false
	}
	if executedCmd.Name() != "new" || executedCmd.Parent() == nil {
		return false
	}
	switch executedCmd.Parent().Name() {
	case "spec", "plan", "implement":
	default:
		return false
	}
	dryRun, _ := executedCmd.Flags().GetBool("dry-run")
	return !dryRun
}

// stateSnapshotFile is the subset of internal/workflow's State shape needed
// to build a sessionlog.StateSnapshot, read directly rather than importing
// internal/workflow — cmd already owns the path-construction logic for
// state.json and this keeps sessionlog a pure leaf package.
type stateSnapshotFile struct {
	Kind           string         `json:"kind"`
	CurrentStep    string         `json:"current_step"`
	CompletedSteps []string       `json:"completed_steps"`
	Data           map[string]any `json:"data"`
}

// readStateSnapshot reads .spektacular/state.json and returns the small
// view of it a session record needs, or nil if no workflow has run yet (no
// state file exists) or it can't be read/parsed — never an error, since a
// snapshot read can never be allowed to affect the command's own outcome.
func readStateSnapshot() *sessionlog.StateSnapshot {
	dir, err := dataDir()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(stateFilePath(dir))
	if err != nil {
		return nil
	}

	var sf stateSnapshotFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil
	}

	name, _ := sf.Data["name"].(string)
	return &sessionlog.StateSnapshot{
		Kind:           sf.Kind,
		Name:           name,
		CurrentStep:    sf.CurrentStep,
		CompletedSteps: sf.CompletedSteps,
	}
}

// stateAdvanced reports whether after meaningfully differs from before —
// the signal that a command actually moved a piece of work forward, as
// opposed to a rejected or silently no-op command (e.g. Goto to the
// already-current step) that returns no error but changes nothing. This is
// deliberately independent of whatever error rootCmd.Execute() returned.
func stateAdvanced(before, after *sessionlog.StateSnapshot) bool {
	if (before == nil) != (after == nil) {
		return true
	}
	if before == nil {
		return false
	}
	return before.Kind != after.Kind ||
		before.Name != after.Name ||
		before.CurrentStep != after.CurrentStep ||
		len(before.CompletedSteps) != len(after.CompletedSteps)
}

// sessionLogDir returns the project's already-gitignored debug directory,
// which holds one session log file per workflow session (see
// sessionlog.LogFilePath) plus any ad hoc commands run outside one.
func sessionLogDir() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "debug"), nil
}

// toErrorResponse converts any error returned by a command into the shared
// ErrorResponse shape: an already-built *output.ErrorResponse passes through
// unchanged (even when wrapped, so a remediation built deep in a loader
// survives the wrapping its callers add), anything else falls back to a
// generic internal_error.
func toErrorResponse(err error) *output.ErrorResponse {
	var er *output.ErrorResponse
	if errors.As(err, &er) {
		return er
	}
	if refusal := formatRefusal(err); refusal != nil {
		return refusal
	}
	return output.NewError("internal_error", err.Error())
}

func configFilePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	return filepath.Join(cwd, ".spektacular", "config.yaml"), nil
}

// loadConfig loads the project config from the current working directory.
// It is the project gate every project-operating command goes through: when
// no config file exists here, it returns an explicit "no project" error
// pointing at init rather than falling back to defaults. There is
// deliberately no parent-directory search — Spektacular runs against the
// project directory itself. Returns an error if the config file exists but
// is invalid.
func loadConfig() (config.Config, error) {
	cfgPath, err := configFilePath()
	if err != nil {
		return config.Config{}, err
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return config.Config{}, noProjectError(cfgPath)
	}
	return config.FromYAMLFile(cfgPath)
}

// noProjectError reports that no project is configured where cfgPath (the
// expected .spektacular/config.yaml) would be.
func noProjectError(cfgPath string) *output.ErrorResponse {
	cwd := filepath.Dir(filepath.Dir(cfgPath))
	return output.NewError(
		"no_project",
		fmt.Sprintf("no Spektacular project is configured in %s (missing .spektacular/config.yaml)", cwd),
	).WithResource(cfgPath).
		WithNextAction("run `spektacular init <agent>` in the project directory to initialise a project, or change to a directory that contains one")
}

// loadConfigLenient loads the project config like loadConfig but preserves
// the pre-gate fall-back: a missing config file yields the defaults instead
// of an error. Only bootstrap paths that must run before a project exists
// (init, version's stale-advice composition, and the root debug probe) may
// use it.
func loadConfigLenient() (config.Config, error) {
	cfgPath, err := configFilePath()
	if err != nil {
		return config.Config{}, err
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return config.NewDefault(), nil
	}
	return config.FromYAMLFile(cfgPath)
}

// dataDir returns the .spektacular directory for the current working directory.
// Both spec and plan workflows share this directory (and a single state.json).
func dataDir() (string, error) {
	root, err := projectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".spektacular"), nil
}

// projectRoot returns the project root — the current working directory.
// Every relative path in config.yaml is written relative to the folder
// holding it (config.ProjectConfigDir); the spec, plan and changelog store
// directories are resolved to project-root-relative paths at load (e.g.
// ".spektacular/specs"), so store consumers join them onto this root.
func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	return cwd, nil
}

func init() {
	// Every settings file this build writes records the version that wrote it.
	config.WriterVersion = version
	// The response envelope wrapper in runRoot is the only place a command's
	// outcome is ever printed, so the CLI framework's own default error/usage
	// printing is turned off to avoid printing a failure a second time.
	// Every command except the upgrade path is refused on an out-of-date
	// project (see gate).
	rootCmd.PersistentPreRunE = gate
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	rootCmd.PersistentFlags().StringVar(&globalFields, "fields", "", `JSON array of output fields to include (e.g. '["step","instruction"]')`)
	rootCmd.AddCommand(specCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(changelogCmd)
	rootCmd.AddCommand(implementCmd)
	rootCmd.AddCommand(knowledgeCmd)
	rootCmd.AddCommand(repoCmd)
	rootCmd.AddCommand(skillCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(artifactsCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)
}
