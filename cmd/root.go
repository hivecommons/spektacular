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

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/migrate"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

// version and sha are set at build time via -ldflags
// (see dagger/main.go's Build function). A build without them (go build,
// go run) reports migrate.DevVersion, which the upgrade gate does not hold
// against the project's installed skills version.
var (
	version = migrate.DevVersion
	sha     = ""
)

// globalFields holds the raw --fields JSON array string, available to all subcommands.
var globalFields string

var rootCmd = &cobra.Command{
	Use:     "spektacular",
	Short:   "Agent-driven tool for spec-driven development with speks",
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
// It is also the single place every command is logged. Each command run in
// a project leaves one OpenTelemetry-model span record in the project's log
// (internal/tracelog), through the logger log.provider names, at the level
// log.level sets:
//
//   - standard (the default) records the command, its run, agent, workflow
//     and step, timing, exit status and any error message;
//   - verbose also records the full response and the workflow state before
//     and after the command;
//   - off records nothing.
//
// The level comes from the project's configuration alone. Nothing is
// recorded where no project config exists, so a command run anywhere else,
// such as in a repo's code checkout, never leaves a log behind. A command
// carrying the logging-exempt annotation, on itself or a parent (see
// logExempted), is never logged at all: `status` carries it, so a caller
// that polls status never adds to or trims the log.
//
// Logging never changes a command's output or exit code: at the standard
// level nothing touches the output stream, and the whole record-and-write
// block runs under a recover, with every failure inside the logger dropped.
func runRoot() int {
	// The log probe must tolerate running outside a project — the gate
	// belongs to the command handlers, not here.
	cfg, cfgErr := loadConfigLenient()
	argv := os.Args[1:]
	target, _, findErr := rootCmd.Find(argv)
	level := logLevel(cfg.Log.Level, cfgErr == nil, projectConfigExists(), findErr == nil && logExempted(target))

	var start time.Time
	var snapshotPath string
	var before *stateSnapshot
	var orig io.Writer
	var buf *bytes.Buffer
	if level != config.LogLevelOff {
		start = time.Now()
		snapshotPath = snapshotStatePath(argv)
		// A run already in progress before the upgrade has no run id: give
		// it one now, so this command and every later one share it.
		if snapshotPath != "" {
			_, _ = workflow.EnsureRunID(snapshotPath)
		}
		before = readStateSnapshot(snapshotPath)
		if level == config.LogLevelVerbose {
			orig = rootCmd.OutOrStdout()
			buf = &bytes.Buffer{}
			rootCmd.SetOut(io.MultiWriter(orig, buf))
			terminalOut = orig
		}
	}

	executedCmd, err := rootCmd.ExecuteC()

	exitCode := 0
	if err != nil {
		output.WriteFailure(rootCmd.OutOrStdout(), toErrorResponse(err), globalFields)
		exitCode = 1
	}

	if buf != nil {
		rootCmd.SetOut(orig)
		terminalOut = nil
	}

	if level != config.LogLevelOff && !logExempted(executedCmd) {
		func() {
			defer func() { _ = recover() }()
			var response string
			if buf != nil {
				response = buf.String()
			}
			recordCommand(cfg, level, commandOutcome{
				cmd:      executedCmd,
				argv:     argv,
				start:    start,
				end:      time.Now(),
				exitCode: exitCode,
				err:      err,
				response: response,
				before:   before,
				after:    readStateSnapshot(snapshotPath),
			})
		}()
	}

	return exitCode
}

// Commands that are never logged carry this cobra annotation, on themselves
// or on a command above them: Annotations[logAnnotation] == logExempt. A
// command group marks itself once and every subcommand inherits it through
// logExempted's parent walk. The key and value are the contract other
// command groups use: `status` sets it here, and the run statistics group
// (spec 000075, `stats`) sets the same map.
const (
	logAnnotation = "log"
	logExempt     = "exempt"
)

// logExempted reports whether c, or any command above it, carries the
// logging-exempt annotation. Unlike the upgrade gate's walk it has no
// implicit exemptions: help, completion and the root itself are logged.
func logExempted(c *cobra.Command) bool {
	for ; c != nil; c = c.Parent() {
		if c.Annotations[logAnnotation] == logExempt {
			return true
		}
	}
	return false
}

// forcedLogLevel, when set, replaces the configured log level inside a
// project. Only cmd's TestMain sets it, from the test binary's -log-level
// flag, so the whole suite can run with logging forced off; the shipped
// binary never sets it and no environment variable reaches it.
var forcedLogLevel string

// logLevel is the effective log level of one command: off outside a
// project, when the config cannot be loaded, or for a logging-exempt
// command; otherwise the forced test level when set, else the configured
// one (empty meaning standard).
func logLevel(configured string, configOK, inProject, exempt bool) string {
	if !configOK || !inProject || exempt {
		return config.LogLevelOff
	}
	if forcedLogLevel != "" {
		return forcedLogLevel
	}
	if configured == "" {
		return config.LogLevelStandard
	}
	return configured
}

// commandOutcome is everything a command's record is built from.
type commandOutcome struct {
	cmd           *cobra.Command
	argv          []string
	start, end    time.Time
	exitCode      int
	err           error
	response      string
	before, after *stateSnapshot
}

// recordCommand builds the command's record and hands it to the configured
// logger. A logger that cannot be built (an unknown provider, which config
// validation already refuses) records nothing.
func recordCommand(cfg config.Config, level string, o commandOutcome) {
	root, err := projectRoot()
	if err != nil {
		return
	}
	logger, err := tracelog.New(cfg.Log.Provider, tracelog.FileOptions{
		Directory: cfg.LogDir(root),
		Retention: cfg.Log.Retention,
	})
	if err != nil {
		return
	}
	run, rec := buildRecord(cfg.Agent, level, o)
	logger.Write(run, rec)
}

// buildRecord turns a command's outcome into the run it belongs to and its
// span record.
func buildRecord(agent, level string, o commandOutcome) (tracelog.Run, tracelog.Record) {
	if agent == "" {
		agent = "unknown"
	}
	run, step, stepTo := attributeRun(o.before, o.after, o.start, agent)

	attrs := map[string]any{
		tracelog.AttrSpanType:    tracelog.TypeCommand,
		tracelog.AttrAgent:       agent,
		tracelog.AttrCommandArgs: o.argv,
		tracelog.AttrExitCode:    o.exitCode,
	}
	if !run.Standalone() {
		attrs[tracelog.AttrWorkflowKind] = run.Kind
		attrs[tracelog.AttrWorkflowName] = run.Name
		if step != "" {
			attrs[tracelog.AttrStep] = step
		}
		if stepTo != "" {
			attrs[tracelog.AttrStepTo] = stepTo
		}
	}
	if level == config.LogLevelVerbose {
		attrs[tracelog.AttrResponse] = o.response
		attrs[tracelog.AttrStateBefore] = o.before
		attrs[tracelog.AttrStateAfter] = o.after
	}

	status := tracelog.Status{Code: tracelog.StatusOK}
	if o.err != nil {
		status = tracelog.Status{Code: tracelog.StatusError, Message: toErrorResponse(o.err).Message}
	}

	return run, tracelog.Record{
		Record:     tracelog.RecordSpan,
		TraceID:    run.TraceID,
		SpanID:     tracelog.NewSpanID(),
		Name:       commandName(o.cmd),
		Kind:       tracelog.KindInternal,
		StartTime:  o.start.UTC(),
		EndTime:    o.end.UTC(),
		Status:     status,
		Attributes: attrs,
		Resource:   tracelog.Resource(version),
	}
}

// attributeRun decides which run a command belongs to, from the state
// snapshots taken before and after it: the run in progress afterwards (a
// `new`, a `--force` restart, or any command inside a run); otherwise the
// run in progress before (the command that finished it, or removed its
// lane); otherwise none, and the command is standalone with a trace of its
// own. step is the step the command ran in, when the before-state belongs to
// the same run; stepTo is the step it moved the run to, when that differs.
func attributeRun(before, after *stateSnapshot, start time.Time, agent string) (run tracelog.Run, step, stepTo string) {
	var owner *stateSnapshot
	switch {
	case after.inProgress():
		owner = after
	case before.inProgress():
		owner = before
	default:
		return tracelog.Run{TraceID: tracelog.NewTraceID(), StartedAt: start, Agent: agent}, "", ""
	}

	run = tracelog.Run{TraceID: owner.RunID, Kind: owner.Kind, Name: owner.Name, StartedAt: owner.CreatedAt, Agent: agent}
	if run.TraceID == "" {
		run.TraceID = tracelog.NewTraceID()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = start
	}
	if before.sameRun(owner) {
		step = before.CurrentStep
	}
	if after.sameRun(owner) && after.CurrentStep != step {
		stepTo = after.CurrentStep
	}
	return run, step, stepTo
}

// commandName is the command's path without the binary name, such as
// "spec goto", or "spektacular" for the root command itself.
func commandName(c *cobra.Command) string {
	if c == nil || c == rootCmd {
		return rootCmd.Name()
	}
	return strings.TrimPrefix(c.CommandPath(), rootCmd.Name()+" ")
}

// stateSnapshotFile is the subset of internal/workflow's State shape a
// command's record needs, read directly from the state file the command
// acts on (see snapshotStatePath).
type stateSnapshotFile struct {
	Kind           string         `json:"kind"`
	RunID          string         `json:"run_id"`
	CurrentStep    string         `json:"current_step"`
	CompletedSteps []string       `json:"completed_steps"`
	CreatedAt      time.Time      `json:"created_at"`
	Data           map[string]any `json:"data"`
}

// stateSnapshot is the small view of a workflow's persisted state taken
// immediately before and after a command runs. Its JSON form is what a
// verbose record carries as the state before and after; a nil snapshot means
// no workflow state existed at that point.
type stateSnapshot struct {
	Kind           string    `json:"kind"`
	Name           string    `json:"name"`
	RunID          string    `json:"run_id"`
	CurrentStep    string    `json:"current_step"`
	CompletedSteps []string  `json:"completed_steps"`
	CreatedAt      time.Time `json:"-"`
}

// inProgress reports whether the snapshot is of a run that has started and
// not finished; a nil snapshot is not.
func (s *stateSnapshot) inProgress() bool {
	return s != nil && s.CurrentStep != "" && s.CurrentStep != "finished"
}

// sameRun reports whether s is a snapshot of the same run as other.
func (s *stateSnapshot) sameRun(other *stateSnapshot) bool {
	if s == nil || other == nil {
		return false
	}
	if s.RunID != "" || other.RunID != "" {
		return s.RunID == other.RunID
	}
	return s.Kind == other.Kind && s.Name == other.Name && s.CreatedAt.Equal(other.CreatedAt)
}

// snapshotStatePath is the state file a command's session record follows:
// the lane of an orchestrated plan or implement workflow the command names —
// an orchestrated `new`, a `goto` naming a spec that has a lane, or any other
// command whose arguments name a spec with a running lane, such as a child's
// `plan file write <spec> plan` — and .spektacular/state.json otherwise. It is
// decided once, before the command runs, so both snapshots read the same
// file. "" means none can be found.
//
// Filing by argument keeps a child's work in its own lane, so the shared log
// holds the orchestrator's commands and the few a child runs without naming
// its spec. An orchestrator command naming a spec whose lane is still running
// is filed under that lane too; once the lane finishes and its files are
// removed, commands naming the spec go to the shared log again.
func snapshotStatePath(argv []string) string {
	dir, err := dataDir()
	if err != nil {
		return ""
	}
	shared := stateFilePath(dir)

	var kind, verb, data string
	var args []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--data" || a == "-d":
			if i+1 < len(argv) {
				data = argv[i+1]
				i++
			}
		case strings.HasPrefix(a, "--data="):
			data = strings.TrimPrefix(a, "--data=")
		case a == "--fields" || a == "--stdin" || a == "--file":
			i++ // these take a value, which is not a subcommand
		case strings.HasPrefix(a, "-"):
		case kind == "":
			kind = a
		case verb == "":
			verb = a
		default:
			args = append(args, a)
		}
	}
	if kind != "plan" && kind != "implement" {
		return laneNamedByArgs(dir, kind, append([]string{verb}, args...), shared)
	}
	var input struct {
		Name         string `json:"name"`
		Orchestrated bool   `json:"orchestrated"`
	}
	if json.Unmarshal([]byte(data), &input) != nil || input.Name == "" || !nameRegexp.MatchString(input.Name) {
		return laneNamedByArgs(dir, kind, args, shared)
	}
	lane := workflow.LaneStatePath(dir, kind, input.Name)
	switch verb {
	case "new":
		if input.Orchestrated {
			return lane
		}
		// An implement run started with worktrees on keeps a lane too.
		if kind == "implement" && !dryRunArg(argv) {
			if cfg, err := loadConfig(); err == nil && cfg.Implement.Worktrees {
				return lane
			}
		}
	case "goto":
		if _, err := os.Stat(lane); err == nil {
			return lane
		}
	case "goal":
		// `implement goal record|decide|show` name their spec in --data like
		// a goto, and belong to the same lane, so a verifier finds each
		// recorded goal check in the lane it gates.
		if _, err := os.Stat(lane); err == nil && kind == "implement" {
			return lane
		}
	}
	return laneNamedByArgs(dir, kind, args, shared)
}

// laneNamedByArgs is the state file of a running lane whose spec one of args
// names, preferring the lane of the command's own kind, or fallback when none
// does. An argument that is not a spec name never matches, since only a
// running orchestrated workflow has a lane file.
func laneNamedByArgs(dir, kind string, args []string, fallback string) string {
	kinds := []string{"plan", "implement"}
	if kind == "implement" {
		kinds = []string{"implement", "plan"}
	}
	for _, k := range kinds {
		for _, a := range args {
			if !nameRegexp.MatchString(a) {
				continue
			}
			lane := workflow.LaneStatePath(dir, k, a)
			if _, err := os.Stat(lane); err == nil {
				return lane
			}
		}
	}
	return fallback
}

// dryRunArg reports whether argv asks for a dry run.
func dryRunArg(argv []string) bool {
	for _, a := range argv {
		if a == "--dry-run" || a == "-n" || a == "--dry-run=true" {
			return true
		}
	}
	return false
}

// readStateSnapshot reads the workflow state file at path and returns the
// small view of it a command's record needs, or nil if no workflow has run
// yet (no state file exists) or it can't be read/parsed — never an error,
// since a snapshot read can never be allowed to affect the command's own
// outcome.
func readStateSnapshot(path string) *stateSnapshot {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var sf stateSnapshotFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil
	}

	name, _ := sf.Data["name"].(string)
	return &stateSnapshot{
		Kind:           sf.Kind,
		Name:           name,
		RunID:          sf.RunID,
		CurrentStep:    sf.CurrentStep,
		CompletedSteps: sf.CompletedSteps,
		CreatedAt:      sf.CreatedAt,
	}
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

// projectConfigExists reports whether the current working directory holds
// a project config.
func projectConfigExists() bool {
	cfgPath, err := configFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(cfgPath)
	return err == nil
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
		fmt.Sprintf("no Spektacular (Spek) project is configured in %s (missing .spektacular/config.yaml)", cwd),
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
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(knowledgeCmd)
	rootCmd.AddCommand(designCmd)
	rootCmd.AddCommand(epicCmd)
	rootCmd.AddCommand(repoCmd)
	rootCmd.AddCommand(skillCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(artifactsCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)
}
