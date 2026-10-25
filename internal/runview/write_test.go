package runview

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/runstats"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

var sampleDir = flag.String("sample-dir", "", "write the sample pages for the manual browser checks into this directory")

// page writes m and returns the document.
func page(t testing.TB, m Model) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, Write(&b, m))
	return b.Bytes()
}

func TestWriteRoundTrip(t *testing.T) {
	m, _ := build(t, append(treeRecords(), pausedGapRecord()...), 5*time.Minute)
	got, err := Decode(page(t, m))
	require.NoError(t, err)
	require.Equal(t, m, got, "the run data in the page reads back exactly as built")
}

func TestWriteRoundTripGeneratedRun(t *testing.T) {
	m, _ := build(t, genRun(500, 7), 5*time.Minute)
	got, err := Decode(page(t, m))
	require.NoError(t, err)
	require.Equal(t, m, got)
}

func TestWritePageIsSelfContained(t *testing.T) {
	m, _ := build(t, append(treeRecords(), pausedGapRecord()...), 5*time.Minute)
	p := strings.ToLower(string(page(t, m)))
	for _, s := range []string{"src=", "href=", "url(", "@import", "http://", "https://", "<link", "<iframe"} {
		require.NotContains(t, p, s, "the page references nothing outside itself")
	}
	require.Equal(t, 1, strings.Count(p, "<style>"), "the stylesheet is inline")
	require.Equal(t, 2, strings.Count(p, "<script"), "only the data element and the inline script")
}

func TestWriteHostileNamesStayText(t *testing.T) {
	const closeScript = `</script><script>alert(1)</script>`
	const img = `<img src=x onerror=alert(1)>`
	f := run1
	hostile := func(a, b string) []tracelog.Record {
		return recs(
			f.span("s1", a, tracelog.TypeStep, "", at(0), at(10), tracelog.EndMoved, step(b)),
			f.span("a1", b, tracelog.TypeSubAgent, "s1", at(1), at(4), tracelog.EndStopped, step(a)),
			f.cmd("c1", "implement goto", "s1", a, at(5), at(5).Add(time.Second), 0,
				map[string]any{tracelog.AttrCommandArgs: []string{"implement", "goto", "--data", a, b}}),
		)
	}
	clean, _ := build(t, hostile("analyze", "research"), 5*time.Minute)
	m, _ := build(t, hostile(closeScript, img), 5*time.Minute)
	cleanPage, p := string(page(t, clean)), string(page(t, m))

	for _, s := range []string{closeScript, img, "<img", "alert(1)</script>", "<script>alert"} {
		require.NotContains(t, p, s, "a hostile name never appears raw")
	}
	require.Equal(t, strings.Count(cleanPage, "<script"), strings.Count(p, "<script"), "no script element is opened")
	require.Equal(t, strings.Count(cleanPage, "</script"), strings.Count(p, "</script"), "no script element is closed early")
	require.Equal(t, strings.Count(cleanPage, "<"), strings.Count(p, "<"), "no tag of any kind is added")

	got, err := Decode([]byte(p))
	require.NoError(t, err)
	require.Equal(t, m, got)
	require.Equal(t, closeScript, str(got, got.Rows.Name[rowOf(t, got, "s1")]))
	require.Equal(t, img, str(got, got.Rows.Name[rowOf(t, got, "a1")]))
	require.Equal(t, closeScript, str(got, got.Rows.Step[rowOf(t, got, "c1")]))
	require.Equal(t, "implement goto --data "+closeScript+" "+img, str(got, got.Rows.Args[rowOf(t, got, "c1")]))
}

func TestWriteIsDeterministic(t *testing.T) {
	m, _ := build(t, append(treeRecords(), pausedGapRecord()...), 5*time.Minute)
	dir := t.TempDir()
	var files [][]byte
	for i := range 2 {
		path := filepath.Join(dir, fmt.Sprintf("page-%d.html", i))
		fh, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, Write(fh, m))
		require.NoError(t, fh.Close())
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		files = append(files, b)
	}
	require.Equal(t, files[0], files[1], "writing the same run twice gives identical files")
	again, _ := build(t, append(treeRecords(), pausedGapRecord()...), 5*time.Minute)
	require.Equal(t, files[0], page(t, again), "a fresh build of the same records writes the same page")

	p := string(files[0])
	wd, err := os.Getwd()
	require.NoError(t, err)
	for _, path := range []string{dir, filepath.Dir(dir), wd} {
		require.False(t, strings.Contains(p, path), "the page carries no file path, found %q", path)
	}

	// The only moments in the page are the run's own start and end.
	own := map[string]bool{
		m.Run.StartedAt.Format(time.RFC3339): true,
		m.Run.EndedAt.Format(time.RFC3339):   true,
	}
	stamps := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[^"]*`).FindAllString(p, -1)
	require.NotEmpty(t, stamps)
	for _, s := range stamps {
		require.True(t, own[s], "timestamp %q is not the run's own", s)
	}
	for _, d := range regexp.MustCompile(`\d{4}-\d{2}-\d{2}`).FindAllString(p, -1) {
		require.Equal(t, t0.Format("2006-01-02"), d, "the only date is the run's")
	}
	if today := time.Now().Format("2006-01-02"); today != t0.Format("2006-01-02") {
		require.NotContains(t, p, today, "no generation date")
	}
}

func TestPageShellHasEachMarkerOnce(t *testing.T) {
	shell, err := asset("page.html")
	require.NoError(t, err)
	for _, mk := range []string{markerCSS, markerData, markerJS} {
		require.Equal(t, 1, strings.Count(shell, mk), "marker %q", mk)
	}
	require.Contains(t, shell, dataOpen+markerData+"</script>", "the data marker is the data element's only content")

	css, err := asset("viewer.css")
	require.NoError(t, err)
	js, err := asset("viewer.js")
	require.NoError(t, err)
	for _, mk := range []string{markerCSS, markerData, markerJS} {
		require.NotContains(t, css, mk)
		require.NotContains(t, js, mk)
	}
}

func TestWritePageOf2000SpansIsUnder1MB(t *testing.T) {
	m, _ := build(t, genRun(2000, 1), 5*time.Minute)
	require.Len(t, m.Rows.SpanID, 2000, "one row per generated span and command")
	p := page(t, m)
	t.Logf("page of %d rows: %d bytes", len(m.Rows.SpanID), len(p))
	require.Less(t, len(p), 1_000_000)
}

func TestGenRunIsRealistic(t *testing.T) {
	a, b := genRun(2000, 1), genRun(2000, 1)
	require.Equal(t, a, b, "the generator is deterministic")

	m, r := build(t, a, 5*time.Minute)
	count := map[string]int{}
	problems := 0
	for i := range m.Rows.SpanID {
		count[m.Types[m.Rows.Type[i]]]++
		if m.Rows.Problem[i] != 0 {
			problems++
		}
	}
	for _, typ := range Types {
		require.Positive(t, count[typ], "the run has %s rows", typ)
	}
	share := float64(count[tracelog.TypeCommand]) / float64(len(m.Rows.SpanID))
	require.InDelta(t, 0.7, share, 0.1, "about 70%% of rows are commands")
	require.Positive(t, problems, "the run has problem rows")
	require.Positive(t, over(m, 5*time.Minute), "the run has gaps over the default threshold")
	require.Equal(t, r.Spans[0].Start, t0, "times are fixed, not the wall clock")
}

func TestWriteReturnsTheWritersError(t *testing.T) {
	m, _ := build(t, treeRecords(), 5*time.Minute)
	boom := errors.New("disk full")
	require.ErrorIs(t, Write(failingWriter{boom}, m), boom)
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestDecodeRejectsAPageWithoutData(t *testing.T) {
	_, err := Decode([]byte("<html><body>nothing</body></html>"))
	require.Error(t, err)
	_, err = Decode([]byte(dataOpen + `{"version":1}`))
	require.Error(t, err, "an unclosed data element")
	_, err = Decode([]byte(dataOpen + `{not json</script>`))
	require.Error(t, err)
}

// everyTypeRecords is a run with every span type, and each problem: a failed
// helper, an interrupted step, an abandoned span, a never-closed helper and a
// command that exits 2.
func everyTypeRecords() []tracelog.Record {
	f := run1
	failed := f.open("failedHelper", "broken helper", tracelog.TypeSubAgent, "step2", at(12), step("implement"))
	failedEnd := closeSpan(failed, at(14), tracelog.EndStopped)
	failedEnd.Status = tracelog.Status{Code: tracelog.StatusError, Message: "helper crashed"}
	interrupted := f.open("step3", "verify", tracelog.TypeStep, "", at(30), step("verify"))
	return recs(
		treeRecords()[:14], // the tree without its terminal step3 "finished"
		failed, failedEnd,
		f.span("ab1", "abandoned work", tracelog.TypeWork, "step2", at(16), at(18), tracelog.EndAbandoned, step("implement")),
		f.cmd("bad", "implement goto", "step2", "implement", at(20), at(20).Add(2*time.Second), 2,
			map[string]any{tracelog.AttrCommandArgs: []string{"implement", "goto", "--data", `{"step":"bogus"}`}}),
		interrupted, closeSpan(interrupted, at(34), tracelog.EndInterrupted),
		f.cmd("cmd4", "implement goto", "step3", "verify", at(31), at(31).Add(300*time.Millisecond), 0),
		f.open("open1", "helper still running", tracelog.TypeSubAgent, "step3", at(32), step("verify")),
		f.span("work2", "read logs", tracelog.TypeWork, "open1", at(32.5), at(33), tracelog.EndStopped, step("verify")),
	)
}

func TestEveryTypeSampleHasEachTypeAndProblem(t *testing.T) {
	m, _ := build(t, everyTypeRecords(), 5*time.Minute)
	types, problems := map[string]bool{}, map[string]bool{}
	for i := range m.Rows.SpanID {
		types[m.Types[m.Rows.Type[i]]] = true
		problems[m.Problems[m.Rows.Problem[i]]] = true
	}
	for _, typ := range Types {
		require.True(t, types[typ], "type %s", typ)
	}
	for _, p := range Problems {
		require.True(t, problems[p], "problem %q", p)
	}
	problem := func(id string) string { return m.Problems[m.Rows.Problem[rowOf(t, m, id)]] }
	require.Equal(t, ProblemFailed, problem("failedHelper"))
	require.Equal(t, ProblemFailed, problem("bad"))
	require.Equal(t, ProblemInterrupted, problem("step3"))
	require.Equal(t, ProblemAbandoned, problem("ab1"))
	require.Equal(t, ProblemOpen, problem("open1"))
}

// TestSamplePages writes the pages for the manual browser checks:
//
//	go test ./internal/runview -run TestSamplePages -sample-dir DIR
func TestSamplePages(t *testing.T) {
	if *sampleDir == "" {
		t.Skip("set -sample-dir to write the sample pages")
	}
	require.NoError(t, os.MkdirAll(*sampleDir, 0o755))
	samples := []struct {
		file string
		recs []tracelog.Record
	}{
		{"every-type.html", everyTypeRecords()},
		{"gaps.html", pausedRecords()},
		{"spans-2000.html", genRun(2000, 1)},
		{"spans-5000.html", genRun(5000, 2)},
	}
	for _, s := range samples {
		m, _ := build(t, s.recs, 5*time.Minute)
		p := page(t, m)
		path := filepath.Join(*sampleDir, s.file)
		require.NoError(t, os.WriteFile(path, p, 0o644))
		t.Logf("%s: %d rows, %d bytes", path, len(m.Rows.SpanID), len(p))
	}
}

func BenchmarkBuildAndWrite2000(b *testing.B) { benchBuildAndWrite(b, 2000) }

// BenchmarkBuildAndWrite5000 tracks the Go side of the large-run target: a
// 5,000-span run built and written as a page.
func BenchmarkBuildAndWrite5000(b *testing.B) { benchBuildAndWrite(b, 5000) }

func benchBuildAndWrite(b *testing.B, n int) {
	runs := runstats.Assemble(genRun(n, 1))
	if len(runs) != 1 {
		b.Fatalf("got %d runs, want 1", len(runs))
	}
	var size int
	for b.Loop() {
		m := Build(runs[0], Options{GapThreshold: 5 * time.Minute})
		var buf bytes.Buffer
		if err := Write(&buf, m); err != nil {
			b.Fatal(err)
		}
		size = buf.Len()
	}
	b.ReportMetric(float64(size), "page-bytes")
}

// TestPageCarriesItsControls pins the page's interactive controls, which
// the script looks up by id: zoom reset, expand and collapse, the gap
// threshold, and the side summary.
func TestPageCarriesItsControls(t *testing.T) {
	m, _ := build(t, treeRecords(), 5*time.Minute)
	out := string(page(t, m))
	for _, id := range []string{"reset-zoom", "expand-all", "collapse-steps", "gap-threshold", "gap-summary",
		"longest-steps", "longest-gaps", "details-list", "chart", "waterfall", "legend", "totals"} {
		require.Contains(t, out, `id="`+id+`"`)
		require.Contains(t, out, `getElementById("`+id+`")`, "the script reads #%s", id)
	}
	for _, label := range []string{"Reset zoom", "Expand all", "Collapse to steps", "Gap threshold", "Longest steps", "Longest gaps"} {
		require.Contains(t, out, label)
	}
	require.NotContains(t, out, "\u2014", "no em dash in the page's own text")
	require.NotContains(t, out, "—")
}

// genSteps are the implement workflow's steps, in order.
var genSteps = []string{"analyze", "implement", "test", "verify", "check_goal", "update_changelog"}

var genFiles = []string{
	"internal/runview/build.go", "internal/runview/write.go", "internal/runview/model.go",
	"internal/runstats/assemble.go", "internal/tracelog/record.go", "cmd/run_view.go",
}

var genWords = []string{"gap", "threshold", "waterfall", "helper", "span", "trace", "render", "canvas", "report", "timeline"}

// genRun is a deterministic implement run of exactly n spans and commands:
// tasks holding the workflow's steps, each step holding commands (about
// 70% of all rows), helpers with nested work, waits on the user and other
// work, with a few pauses of minutes and some problem spans. Times are
// fixed offsets from t0; seed picks the shape.
func genRun(n int, seed int64) []tracelog.Record {
	rng := rand.New(rand.NewSource(seed))
	f := run1
	var out []tracelog.Record
	count := 0
	now := t0
	newID := func(prefix string) string {
		count++
		return fmt.Sprintf("%s%05d", prefix, count)
	}
	secs := func(lo, hi int) time.Duration {
		return time.Duration(lo*1000+rng.Intn((hi-lo)*1000+1)) * time.Millisecond
	}
	commandLine := func(step string) (string, []string) {
		switch rng.Intn(4) {
		case 0, 1:
			return "implement goto", []string{"implement", "goto", "--data",
				fmt.Sprintf(`{"step":%q,"name":%q}`, step, f.name)}
		case 2:
			return "implement file read", []string{"implement", "file", "read", "--name", f.name,
				"--file", genFiles[rng.Intn(len(genFiles))]}
		default:
			q := genWords[rng.Intn(len(genWords))] + " " + genWords[rng.Intn(len(genWords))]
			return "knowledge search", []string{"knowledge", "search", "--query", q}
		}
	}

	for task := 0; count < n; task++ {
		taskID := fmt.Sprintf("%08x", rng.Uint32())
		tAttrs := map[string]any{tracelog.AttrTaskID: taskID}
		taskOpen := f.open(newID("task"), fmt.Sprintf("Task %d: build part %d", task+1, task+1), tracelog.TypeTask, "", now, tAttrs)
		out = append(out, taskOpen)
		for _, stepName := range genSteps {
			if count >= n {
				break
			}
			sAttrs := map[string]any{tracelog.AttrStep: stepName, tracelog.AttrTaskID: taskID}
			stepOpen := f.open(newID("step"), stepName, tracelog.TypeStep, taskOpen.SpanID, now, sAttrs)
			out = append(out, stepOpen)
			parent := stepOpen.SpanID
			children := 3 + rng.Intn(25)
			for c := 0; c < children && count < n; c++ {
				now = now.Add(secs(1, 20))
				if rng.Intn(60) == 0 {
					now = now.Add(time.Duration(6+rng.Intn(15)) * time.Minute) // a long pause
				}
				r := rng.Float64()
				switch {
				case r < 0.84:
					name, args := commandLine(stepName)
					code := 0
					if rng.Intn(50) == 0 {
						code = 2
					}
					end := now.Add(secs(0, 2))
					out = append(out, f.cmd(newID("cmd"), name, parent, stepName, now, end, code,
						map[string]any{tracelog.AttrCommandArgs: args, tracelog.AttrTaskID: taskID}))
					now = end
				case r < 0.91:
					h := f.open(newID("agent"), "helper: "+genWords[rng.Intn(len(genWords))], tracelog.TypeSubAgent, parent, now, sAttrs)
					out = append(out, h)
					works := min(1+rng.Intn(3), n-count)
					for range works {
						now = now.Add(secs(1, 10))
						w := f.open(newID("work"), "work: "+genWords[rng.Intn(len(genWords))], tracelog.TypeWork, h.SpanID, now, sAttrs)
						now = now.Add(secs(5, 90))
						out = append(out, w, closeSpan(w, now, tracelog.EndStopped))
					}
					now = now.Add(secs(1, 10))
					end := closeSpan(h, now, tracelog.EndStopped)
					if rng.Intn(20) == 0 {
						end.Status = tracelog.Status{Code: tracelog.StatusError, Message: "helper failed"}
					}
					out = append(out, end)
				case r < 0.96:
					w := f.open(newID("wait"), "question for the user", tracelog.TypeUserWait, parent, now, sAttrs)
					now = now.Add(secs(10, 180))
					out = append(out, w, closeSpan(w, now, tracelog.EndStopped))
				default:
					w := f.open(newID("work"), "work: "+genWords[rng.Intn(len(genWords))], tracelog.TypeWork, parent, now, sAttrs)
					now = now.Add(secs(2, 60))
					reason := tracelog.EndStopped
					if rng.Intn(15) == 0 {
						reason = tracelog.EndAbandoned
					}
					out = append(out, w, closeSpan(w, now, reason))
				}
			}
			now = now.Add(secs(0, 2))
			reason := tracelog.EndMoved
			if rng.Intn(40) == 0 {
				reason = tracelog.EndInterrupted
			}
			out = append(out, closeSpan(stepOpen, now, reason))
		}
		out = append(out, closeSpan(taskOpen, now, tracelog.EndMoved))
	}
	return out
}
