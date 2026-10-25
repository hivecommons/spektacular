package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/runstats"
	"github.com/spf13/cobra"
)

// statsCmd groups the run statistics reports (internal/runstats), which read
// a logs directory and report where each workflow run's time went. The
// group carries the logging-exempt annotation, and every subcommand inherits
// it through logExempted's parent walk, so a `stats` command is never
// logged: inside a project, reporting can never append to, or trigger the
// retention sweep of, the logs it reads.
var statsCmd = &cobra.Command{
	Annotations: map[string]string{logAnnotation: logExempt},
	Use:         "stats",
	Short:       "Report where workflow runs' time went, from the project's logs",
	Long: `Report where workflow runs' time went, from the logs Spektacular keeps.
The reports read the project's logs directory, or any logs directory given
with --dir, and never change a log: stats commands are never logged.

Every subcommand takes the same selection, combined with AND:
  --name <workflow name>   --kind spec|plan|implement|repo   --agent <agent>
  --run <run id or a unique prefix of 8 or more hex characters> (repeatable)`,
	RunE: runUnknownSubcommand,
}

var statsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the recorded workflow runs, newest first",
	Args:  cobra.NoArgs,
	RunE:  runStatsList,
}

var statsRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Report where one run's time went: totals, steps, tasks, helpers, waits and gaps",
	Long: `Report where one run's time went: its totals, each step and plan task,
every helper launch and wait on the user, and the longest gaps between
recorded events. With no selection it reports the run that started most
recently; otherwise the most recent run the selection matches.`,
	Args: cobra.NoArgs,
	RunE: runStatsRun,
}

var statsCompareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare runs side by side, step by step and in total, against the first",
	Long: `Compare two or more runs side by side, step by step and in total, with each
run's difference from the first in time and percentage. A step a run never
entered is absent (null), not zero. With no selection it compares the two
most recent runs of the latest run's kind, oldest first; with two or more
--run values, exactly those runs, in that order.`,
	Args: cobra.NoArgs,
	RunE: runStatsCompare,
}

var statsSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Summarise many runs: per step and in total, the count, median and 90th percentile",
	Long: `Summarise a set of runs: for each step and for the totals, the number of runs
it applies to, the median and the 90th percentile (nearest rank). With no
selection it summarises every run of the latest run's kind; with --run
values, exactly those runs.`,
	Args: cobra.NoArgs,
	RunE: runStatsSummary,
}

// statsSource is what every stats result carries beside its figures: the
// logs directory read and the lines in it that could not be read.
type statsSource struct {
	LogDir       string `json:"log_dir"`
	SkippedLines int    `json:"skipped_lines"`
}

type statsListResult struct {
	statsSource
	Runs []runstats.RunSummary `json:"runs"`
}

type statsRunResult struct {
	statsSource
	runstats.Report
}

// statsCompareResult and statsSummaryResult are the JSON of `stats compare`
// and `stats summary`, a contract read by the Harbor harness (spec 000076)
// and fixed by the Data Structures section of plan 000075_run-stats-command:
//
//	compare: {"log_dir","skipped_lines","gap_threshold_ms",
//	          "runs":[<run summary>, base first],
//	          "totals":[{"figure":"wall_ms","values":[..],"delta":[..],"delta_pct":[..]}],
//	          "steps":[{"step":"discovery","figures":[{"figure":"duration_ms","values":[..],"delta":[..],"delta_pct":[..]}]}]}
//	summary: the same top-level shape, each figure {"figure","count","median","p90"}
//
// Arrays line up with runs; a step a run never entered is null there, with
// null deltas; delta_pct is null when the base is 0 or null, rounded to one
// decimal. Total figures, in order: wall_ms, working_ms, user_wait_ms,
// helper_wait_ms, command_ms, commands, helpers, gap_ms. Step figures, in
// order: duration_ms, entries, commands, command_ms, helper_wait_ms,
// user_wait_ms. count is the runs a figure applies to (for a step, the runs
// that entered it); median and p90 are numbers, null when count is 0.
type statsCompareResult struct {
	statsSource
	runstats.Comparison
}

type statsSummaryResult struct {
	statsSource
	runstats.Summary
}

// statsSelectionFlags are the flags every stats subcommand's schema lists.
func statsSelectionFlags() map[string]*schemaProp {
	return map[string]*schemaProp{
		"dir":    {Type: "string", Description: "logs directory to read; defaults to the project's log.config.directory"},
		"format": {Type: "string", Enum: []string{statusFormatPretty, statusFormatJSON}},
		"name":   {Type: "string", Description: "only runs of this workflow name"},
		"kind":   {Type: "string", Enum: runstats.Kinds},
		"agent":  {Type: "string", Description: "only runs by this agent"},
		"run":    {Type: "array", Items: &schemaProp{Type: "string", Pattern: "^[0-9a-f]{8,32}$"}, Description: "a run id or unique prefix; repeatable"},
	}
}

var statsSourceProps = map[string]*schemaProp{
	"log_dir":       {Type: "string", Description: "the absolute logs directory read"},
	"skipped_lines": {Type: "integer", Description: "lines that could not be read as records"},
}

var statsRunSummarySchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"run_id":     {Type: "string"},
	"kind":       {Type: "string"},
	"name":       {Type: "string"},
	"agent":      {Type: "string"},
	"log_file":   {Type: "string"},
	"started_at": {Type: "string", Description: "RFC 3339 UTC"},
	"ended_at":   {Type: "string", Description: "RFC 3339 UTC"},
	"wall_ms":    {Type: "integer"},
	"status":     {Type: "string", Enum: []string{runstats.StatusFinished, runstats.StatusInterrupted, runstats.StatusInProgress}},
}}

// statsOutput is a stats result's schema: the source fields plus props.
func statsOutput(props map[string]*schemaProp) *schemaObj {
	all := map[string]*schemaProp{"error": {Type: "boolean"}}
	for k, v := range statsSourceProps {
		all[k] = v
	}
	for k, v := range props {
		all[k] = v
	}
	return &schemaObj{Type: "object", Properties: all}
}

func runStatsList(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: statsOutput(map[string]*schemaProp{
				"runs": {Type: "array", Items: statsRunSummarySchema, Description: "newest first"},
			}),
			Flags: statsSelectionFlags(),
		}, "")
	}
	format, err := statsFormat(cmd)
	if err != nil {
		return err
	}
	logs, err := statsLogs(cmd)
	if err != nil {
		return err
	}
	runs, err := logs.Candidates(statsFilter(cmd))
	if err != nil {
		return statsError(err)
	}
	if format == statusFormatJSON {
		res := statsListResult{statsSource: source(logs), Runs: []runstats.RunSummary{}}
		for _, r := range runs {
			res.Runs = append(res.Runs, r.Summary())
		}
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(res)
	}
	return runstats.RenderList(cmd.OutOrStdout(), logs, runs)
}

func runStatsRun(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		flags := statsSelectionFlags()
		flags["gaps"] = &schemaProp{Type: "integer", Description: "the most gaps listed, longest first; at least 1 (default 10)"}
		flags["gap-threshold"] = &schemaProp{Type: "string", Description: "a Go duration; gaps longer than it count toward gap_ms (default 5m)"}
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: statsOutput(map[string]*schemaProp{
				"gap_threshold_ms": {Type: "integer"},
				"run":              statsRunSummarySchema,
				"totals":           {Type: "object", Description: "wall_ms, working_ms, user_wait_ms, helper_wait_ms, command_ms, commands, helpers, user_waits, gap_ms, gaps_over_threshold"},
				"steps":            {Type: "array", Description: "one row per step name, in order of first entry"},
				"step_entries":     {Type: "array", Description: "every entry into a step"},
				"tasks":            {Type: "array", Description: "one row per plan task"},
				"helpers":          {Type: "array", Description: "every helper-agent launch"},
				"user_waits":       {Type: "array", Description: "every wait on the user"},
				"gaps":             {Type: "array", Description: "the longest gaps, longest first"},
			}),
			Flags: flags,
		}, "")
	}
	format, opts, logs, c, _, err := statsSelect(cmd)
	if err != nil {
		return err
	}
	run, err := runstats.ForReport(c)
	if err != nil {
		return statsError(err)
	}
	rep := runstats.BuildReport(run, opts)
	if format == statusFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(statsRunResult{statsSource: source(logs), Report: rep})
	}
	return runstats.RenderReport(cmd.OutOrStdout(), rep, logs.Skipped)
}

// statsFigureSchema is the schema of a comparison's or a summary's output.
func statsFigureSchema(figure *schemaProp) *schemaObj {
	return statsOutput(map[string]*schemaProp{
		"gap_threshold_ms": {Type: "integer"},
		"runs":             {Type: "array", Items: statsRunSummarySchema, Description: "the runs, in the order compared (the first is the base)"},
		"totals":           {Type: "array", Items: figure, Description: "wall_ms, working_ms, user_wait_ms, helper_wait_ms, command_ms, commands, helpers, gap_ms"},
		"steps": {Type: "array", Description: "one entry per step name", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"step":    {Type: "string"},
			"figures": {Type: "array", Items: figure, Description: "duration_ms, entries, commands, command_ms, helper_wait_ms, user_wait_ms"},
		}}},
	})
}

func statsFigureFlags() map[string]*schemaProp {
	flags := statsSelectionFlags()
	flags["gap-threshold"] = &schemaProp{Type: "string", Description: "a Go duration; gaps longer than it count toward gap_ms (default 5m)"}
	return flags
}

func runStatsCompare(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: statsFigureSchema(&schemaProp{Type: "object", Properties: map[string]*schemaProp{
				"figure":    {Type: "string"},
				"values":    {Type: "array", Items: &schemaProp{Type: "integer"}, Description: "one per run; null where it does not apply"},
				"delta":     {Type: "array", Items: &schemaProp{Type: "integer"}, Description: "each value less the first run's; null where either is null"},
				"delta_pct": {Type: "array", Items: &schemaProp{Type: "number"}, Description: "delta as a percentage of the first run's value, one decimal; null when that is null or 0"},
			}}),
			Flags: statsFigureFlags(),
		}, "")
	}
	format, opts, logs, c, f, err := statsSelect(cmd)
	if err != nil {
		return err
	}
	runs, err := runstats.ForCompare(c, f)
	if err != nil {
		return statsError(err)
	}
	cmp := runstats.Compare(runs, opts)
	if format == statusFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(statsCompareResult{statsSource: source(logs), Comparison: cmp})
	}
	return runstats.RenderComparison(cmd.OutOrStdout(), cmp, logs.Skipped)
}

func runStatsSummary(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: statsFigureSchema(&schemaProp{Type: "object", Properties: map[string]*schemaProp{
				"figure": {Type: "string"},
				"count":  {Type: "integer", Description: "the runs the figure applies to; for a step, the runs that entered it"},
				"median": {Type: "number", Description: "null when count is 0"},
				"p90":    {Type: "number", Description: "nearest rank; null when count is 0"},
			}}),
			Flags: statsFigureFlags(),
		}, "")
	}
	format, opts, logs, c, f, err := statsSelect(cmd)
	if err != nil {
		return err
	}
	runs, err := runstats.ForSummary(c, f)
	if err != nil {
		return statsError(err)
	}
	sum := runstats.Summarise(runs, opts)
	if format == statusFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(statsSummaryResult{statsSource: source(logs), Summary: sum})
	}
	return runstats.RenderSummary(cmd.OutOrStdout(), sum, logs.Skipped)
}

// statsSelect does what every report shares: it validates the format and
// options, reads the logs, and resolves the selection's candidates.
func statsSelect(cmd *cobra.Command) (format string, opts runstats.Options, logs *runstats.Logs, c []*runstats.Run, f runstats.Filter, err error) {
	if format, err = statsFormat(cmd); err != nil {
		return
	}
	if opts, err = statsOptions(cmd); err != nil {
		return
	}
	if logs, err = statsLogs(cmd); err != nil {
		return
	}
	f = statsFilter(cmd)
	if c, err = logs.Candidates(f); err != nil {
		err = statsError(err)
	}
	return
}

func source(l *runstats.Logs) statsSource {
	return statsSource{LogDir: l.Dir, SkippedLines: l.Skipped}
}

// statsFormat validates --format, as status does.
func statsFormat(cmd *cobra.Command) (string, error) {
	format, _ := cmd.Flags().GetString("format")
	if format != statusFormatPretty && format != statusFormatJSON {
		return "", output.NewError("stats_format_unsupported",
			fmt.Sprintf("stats format %q is not supported; supported formats: %s, %s", format, statusFormatPretty, statusFormatJSON)).
			WithResource(format).
			WithNextAction(fmt.Sprintf("re-run with --format %s or --format %s", statusFormatPretty, statusFormatJSON))
	}
	return format, nil
}

// statsOptions reads the gap options. --gaps exists only on the commands
// that list gaps.
func statsOptions(cmd *cobra.Command) (runstats.Options, error) {
	o := runstats.DefaultOptions
	if f := cmd.Flags().Lookup("gaps"); f != nil {
		n, _ := cmd.Flags().GetInt("gaps")
		if n < 1 {
			return o, output.NewError("stats_bad_option",
				fmt.Sprintf("--gaps %d is not allowed: at least one gap is listed", n)).
				WithResource("--gaps").
				WithNextAction("re-run with --gaps 1 or more (the default is 10)")
		}
		o.Gaps = n
	}
	th, _ := cmd.Flags().GetDuration("gap-threshold")
	if th <= 0 {
		return o, output.NewError("stats_bad_option",
			fmt.Sprintf("--gap-threshold %s is not allowed: the threshold must be longer than zero", th)).
			WithResource("--gap-threshold").
			WithNextAction("re-run with a positive Go duration such as --gap-threshold 5m (the default)")
	}
	o.GapThreshold = th
	return o, nil
}

// statsFilter builds the run selection from the flags.
func statsFilter(cmd *cobra.Command) runstats.Filter {
	name, _ := cmd.Flags().GetString("name")
	kind, _ := cmd.Flags().GetString("kind")
	agent, _ := cmd.Flags().GetString("agent")
	runs, _ := cmd.Flags().GetStringSlice("run")
	return runstats.Filter{Name: name, Kind: kind, Agent: agent, Runs: runs}
}

// statsLogs reads the logs directory: --dir, made absolute, or else the
// project's configured log directory. Only reading the project's own logs
// needs a project.
func statsLogs(cmd *cobra.Command) (*runstats.Logs, error) {
	dir, _ := cmd.Flags().GetString("dir")
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		dir = abs
	} else {
		cfgPath, err := configFilePath()
		if err != nil {
			return nil, err
		}
		if !projectConfigExists() {
			return nil, noProjectError(cfgPath).
				WithNextAction("run from a Spektacular project, or pass --dir <logs directory>")
		}
		cfg, err := loadConfig()
		if err != nil {
			return nil, err
		}
		root, err := projectRoot()
		if err != nil {
			return nil, err
		}
		dir = cfg.LogDir(root)
	}
	logs, err := runstats.Load(dir)
	if errors.Is(err, runstats.ErrNoLogDir) {
		return nil, output.NewError("stats_log_dir_not_found",
			fmt.Sprintf("the logs directory %s does not exist", dir)).
			WithResource(dir).
			WithNextAction("check the path given with --dir; a project's own logs are in its log.config.directory setting (.spektacular/logs by default), created when its first command is logged")
	}
	if err != nil {
		return nil, err
	}
	return logs, nil
}

// statsError turns the run statistics package's refusals into the error
// envelope, each with the fix.
func statsError(err error) error {
	var se *runstats.SelectionError
	switch {
	case errors.As(err, &se):
		switch se.Code {
		case runstats.CodeBadKind:
			return output.NewError("stats_bad_selection", se.Error()).
				WithResource(se.Value).
				WithNextAction(fmt.Sprintf("re-run with --kind set to one of: %s", strings.Join(runstats.Kinds, ", ")))
		case runstats.CodeBadRunID:
			return output.NewError("stats_bad_selection", se.Error()).
				WithResource(se.Value).
				WithNextAction("give --run a run id, or its first 8 or more hex characters, as `spektacular stats list` shows")
		case runstats.CodeRunNotFound:
			return output.NewError("stats_run_not_found", se.Error()).
				WithResource(se.Value).
				WithNextAction("run `spektacular stats list` to see recorded runs and their ids")
		case runstats.CodeRunAmbiguous:
			return output.NewError("stats_run_ambiguous", se.Error()).
				WithResource(se.Value).
				WithNextAction(fmt.Sprintf("give more characters of the run id; it matches %s", strings.Join(se.Matches, ", ")))
		}
	case errors.Is(err, runstats.ErrNoRuns):
		return output.NewError("stats_no_runs", "no recorded run matches the selection").
			WithNextAction("run `spektacular stats list` to see recorded runs, or widen the selection")
	case errors.Is(err, runstats.ErrTooFew):
		return output.NewError("stats_compare_needs_two", "a comparison needs at least two runs, and the selection gives fewer").
			WithNextAction("name two runs with --run <id> --run <id> (see `spektacular stats list`), or widen the selection")
	}
	return err
}

func init() {
	statsCmd.PersistentFlags().String("dir", "", "Logs directory to read (default: the project's log directory)")
	statsCmd.PersistentFlags().String("format", statusFormatPretty, "Output format: pretty or json")
	statsCmd.PersistentFlags().String("name", "", "Only runs of this workflow name")
	statsCmd.PersistentFlags().String("kind", "", "Only runs of this kind: spec, plan, implement or repo")
	statsCmd.PersistentFlags().String("agent", "", "Only runs by this agent")
	statsCmd.PersistentFlags().StringSlice("run", nil, "A run id, or a unique prefix of 8 or more hex characters (repeatable)")
	statsCmd.PersistentFlags().Duration("gap-threshold", 5*time.Minute, "Gaps longer than this count toward the gap total")

	statsRunCmd.Flags().Int("gaps", runstats.DefaultOptions.Gaps, "The most gaps to list, longest first")
	for _, c := range []*cobra.Command{statsListCmd, statsRunCmd, statsCompareCmd, statsSummaryCmd} {
		c.Flags().Bool("schema", false, "Print the input/output schema and exit")
		statsCmd.AddCommand(c)
	}
}
