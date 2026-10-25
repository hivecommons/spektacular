package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/runstats"
	"github.com/hivecommons/spektacular/internal/runview"
	"github.com/spf13/cobra"
)

var statsViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Write one run as a single-file HTML waterfall you can open in any browser",
	Long: `Write one recorded run as a self-contained HTML page that draws it as a
waterfall: every step, task, helper agent, wait on the user, other marked
work and command on one timeline, nested under what contains it, with
failures and long gaps marked. The page needs no server and no network.

It selects the run as stats run does: with no selection, the run that
started most recently; otherwise the most recent run the selection matches.
--gap-threshold sets the page's default gap threshold (5m), which can be
changed on the page. The page is written to --out, a file or an existing
directory; by default spektacular-run-<first 8 characters of the run id>.html
in the current directory. It is never written into the logs directory.`,
	Args: cobra.NoArgs,
	RunE: runStatsView,
}

// statsViewResult is the JSON of `stats view`: the source, the page's
// default threshold, the run rendered, and the file written with its bar
// count and size.
type statsViewResult struct {
	statsSource
	GapThresholdMS int64               `json:"gap_threshold_ms"`
	Run            runstats.RunSummary `json:"run"`
	File           string              `json:"file"`
	Rows           int                 `json:"rows"`
	Bytes          int                 `json:"bytes"`
}

func runStatsView(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		flags := statsFigureFlags()
		flags["gap-threshold"] = &schemaProp{Type: "string", Description: "a Go duration; the page's default gap threshold (default 5m)"}
		flags["out"] = &schemaProp{Type: "string", Description: "the file to write, or an existing directory to write the default file name into; default spektacular-run-<id8>.html in the current directory"}
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: statsOutput(map[string]*schemaProp{
				"gap_threshold_ms": {Type: "integer", Description: "the page's default gap threshold"},
				"run":              statsRunSummarySchema,
				"file":             {Type: "string", Description: "the absolute path of the page written"},
				"rows":             {Type: "integer", Description: "the bars on the page, one per span and command"},
				"bytes":            {Type: "integer", Description: "the page's size"},
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
	out, _ := cmd.Flags().GetString("out")
	target, err := viewTarget(out, run.ID, logs.Dir)
	if err != nil {
		return err
	}

	m := runview.Build(run, runview.Options{GapThreshold: opts.GapThreshold, Skipped: logs.Skipped})
	var page bytes.Buffer
	if err := runview.Write(&page, m); err != nil {
		return err
	}
	if err := writeViewFile(target, page.Bytes()); err != nil {
		return err
	}

	rows := len(m.Rows.SpanID)
	if format == statusFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(statsViewResult{
			statsSource: source(logs), GapThresholdMS: opts.GapThreshold.Milliseconds(),
			Run: run.Summary(), File: target, Rows: rows, Bytes: page.Len(),
		})
	}
	w := cmd.OutOrStdout()
	id := run.ID
	if len(id) > 8 {
		id = id[:8]
	}
	fmt.Fprintf(w, "Wrote %s %s (run %s, %d spans) to %s\n", run.Kind, run.Name, id, rows, target)
	if logs.Skipped > 0 {
		fmt.Fprintf(w, "Skipped lines: %d\n", logs.Skipped)
	}
	return nil
}

// viewTarget is the absolute path the page is written to: --out, or the
// default file name in the current directory, or inside --out when it names
// an existing directory. A path inside the logs directory is refused, so
// rendering never changes the logs it reads.
func viewTarget(out, runID, logDir string) (string, error) {
	name := runview.DefaultFileName(runID)
	var target string
	if out == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		target = filepath.Join(cwd, name)
	} else {
		abs, err := filepath.Abs(out)
		if err != nil {
			return "", err
		}
		target = abs
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			target = filepath.Join(abs, name)
		}
	}
	if inside(resolved(logDir), resolved(filepath.Dir(target))) {
		return "", output.NewError("stats_view_out_in_logs",
			fmt.Sprintf("the page would be written to %s, inside the logs directory %s, and reporting never changes the logs", target, logDir)).
			WithResource(target).
			WithNextAction("pass --out with a path outside the logs directory, or omit --out to write to the current directory")
	}
	return target, nil
}

// resolved is path with its symlinks resolved, or path itself when it
// cannot be resolved (it does not exist yet).
func resolved(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return filepath.Clean(path)
}

// inside reports whether path is dir or lies under it.
func inside(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// writeViewFile writes page to target through a temporary file in the same
// directory renamed into place, so an interrupted write never leaves half a
// page.
func writeViewFile(target string, page []byte) error {
	dir := filepath.Dir(target)
	fail := func(err error) error {
		return output.NewError("stats_view_write_failed",
			fmt.Sprintf("the page could not be written to %s: %v", target, err)).
			WithResource(target).
			WithNextAction(fmt.Sprintf("check that %s exists and is writable, or pass --out <writable path>", dir))
	}
	tmp, err := os.CreateTemp(dir, ".spektacular-run-*.html")
	if err != nil {
		return fail(err)
	}
	name := tmp.Name()
	_, werr := tmp.Write(page)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, 0o644)
	}
	if werr == nil {
		werr = os.Rename(name, target)
	}
	if werr != nil {
		_ = os.Remove(name)
		return fail(werr)
	}
	return nil
}

func init() {
	statsViewCmd.Flags().String("out", "", "File to write, or an existing directory for the default file name (default: spektacular-run-<id>.html in the current directory)")
}
