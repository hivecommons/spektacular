package runstats

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

// golden compares got with testdata/name, rewriting it under -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run go test -run %s -update to write it", t.Name())
	require.Equal(t, string(want), got)
}

func TestFmtMS(t *testing.T) {
	cases := map[int64]string{
		0:        "0.0s",
		1234:     "1.2s",
		59_900:   "59.9s",
		60_000:   "1m00s",
		245_000:  "4m05s",
		3723_000: "1h02m03s",
		-211_000: "-3m31s",
		-1_500:   "-1.5s",
	}
	for in, want := range cases {
		require.Equal(t, want, fmtMS(in), "%d", in)
	}
}

func listLogs() *Logs {
	a := fx{trace: "4bf92f3577b34da6a3ce929d0e0e4736", kind: "implement", name: "000075_run-stats-command", agent: "claude"}
	b := fx{trace: "9d0e0e4736a3ce929d0e0e47364bf92f", kind: "plan", name: "000075_run-stats-command", agent: "codex"}
	runs := Assemble(recs(
		b.cmd("plan new", "", at(0), at(1)),
		b.span("p1", "finished", tracelog.TypeStep, "", at(41), at(41), tracelog.EndFinished),
		a.cmd("implement new", "", at(50), at(51)),
		a.cmd("implement goto", "", at(90), at(91.2)),
	))
	return &Logs{Dir: "/logs", Runs: runs, Skipped: 2}
}

func TestRenderListGolden(t *testing.T) {
	logs := listLogs()
	var b bytes.Buffer
	require.NoError(t, RenderList(&b, logs, logs.Runs))
	out := b.String()
	golden(t, "list.golden.txt", out)

	// The table shows the same figures as the JSON summary.
	for _, r := range logs.Runs {
		s := r.Summary()
		line := lineWith(t, out, s.RunID[:8])
		for _, want := range []string{s.Kind, s.Name, s.Agent, s.StartedAt.Format(timeLayout), fmtMS(s.WallMS), s.Status} {
			require.Contains(t, line, want)
		}
	}
	require.Contains(t, out, "Skipped lines: 2")
}

func TestRenderListEmpty(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, RenderList(&b, &Logs{}, nil))
	require.Equal(t, "No recorded runs.\n", b.String())
}

func TestRenderReportGolden(t *testing.T) {
	rep := reportOf(t, DefaultOptions, goldenRun())
	var b bytes.Buffer
	require.NoError(t, RenderReport(&b, rep, 1))
	out := b.String()
	golden(t, "report.golden.txt", out)

	// The table shows the same figures as the structured report.
	require.Contains(t, lineWith(t, out, "Wall time"), fmtMS(rep.Totals.WallMS))
	require.Contains(t, lineWith(t, out, "Working time"), fmtMS(rep.Totals.WorkingMS))
	require.Contains(t, lineWith(t, out, "User wait"), fmtMS(rep.Totals.UserWaitMS))
	require.Contains(t, lineWith(t, out, "Helper wait"), fmtMS(rep.Totals.HelperWaitMS))
	require.Contains(t, lineWith(t, out, "Gaps over"), fmtMS(rep.Totals.GapMS))
	for _, s := range rep.Steps {
		fields := strings.Fields(lineStarting(t, out, s.Step+" "))
		require.Equal(t, s.Step, fields[0])
		require.Equal(t, []string{fmtMS(s.DurationMS), fmtMS(s.CommandMS), fmtMS(s.HelperWaitMS), fmtMS(s.UserWaitMS)},
			[]string{fields[2], fields[4], fields[5], fields[6]}, s.Step)
	}
	require.NotEmpty(t, rep.Tasks)
	require.Contains(t, out, "\nTasks\n")
	require.Contains(t, out, "did not finish")
	require.Contains(t, out, "Skipped lines: 1")
}

func TestRenderReportWithoutTasksOmitsTheTable(t *testing.T) {
	rep := reportOf(t, DefaultOptions, knownWaits())
	var b bytes.Buffer
	require.NoError(t, RenderReport(&b, rep, 0))
	require.NotContains(t, b.String(), "\nTasks\n")
	require.NotContains(t, b.String(), "Skipped lines")
}

// lineStarting is the first line of out starting with s.
func lineStarting(t *testing.T, out, s string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, s) {
			return l
		}
	}
	t.Fatalf("no line starts with %q in:\n%s", s, out)
	return ""
}

// lineWith is the first line of out containing s.
func lineWith(t *testing.T, out, s string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	t.Fatalf("no line contains %q in:\n%s", s, out)
	return ""
}
