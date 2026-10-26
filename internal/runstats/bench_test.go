package runstats

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/tracelog"
)

// BenchmarkLoadAndReport loads a generated logs directory of ten runs (the
// default retention), each of about 600 records whose commands carry a 20 KB
// verbose response, and reports on every run. It supports the success
// metric of a report over ten runs in under one second; the measurement
// that counts is the manual one with the built binary.
func BenchmarkLoadAndReport(b *testing.B) {
	dir := b.TempDir()
	response := strings.Repeat("x", 20<<10)
	for r := 0; r < 10; r++ {
		f := fx{trace: fmt.Sprintf("%08x000000000000000000000000", r+1), kind: "implement", name: "000075_run-stats-command", agent: "claude"}
		start := float64(r * 600)
		var rs []tracelog.Record
		for s := 0; s < 40; s++ {
			id := fmt.Sprintf("%04x%012x", r, s)
			from := start + float64(s*10)
			rs = append(rs, f.span(id, fmt.Sprintf("step%d", s%8), tracelog.TypeStep, "", at(from), at(from+10), tracelog.EndMoved)...)
			rs = append(rs, f.span(id+"h", "helper", tracelog.TypeSubAgent, id, at(from+1), at(from+4), tracelog.EndStopped)...)
			for c := 0; c < 11; c++ {
				cmd := f.cmd("implement goto", id, at(from+float64(c)*0.8), at(from+float64(c)*0.8+0.01))
				cmd.SpanID = fmt.Sprintf("%s%02d", id[:14], c)
				cmd.Attributes[tracelog.AttrResponse] = response
				rs = append(rs, cmd)
			}
		}
		writeLogTB(b, dir, fmt.Sprintf("run%02d.jsonl", r), rs)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logs, err := Load(dir)
		if err != nil {
			b.Fatal(err)
		}
		if len(logs.Runs) != 10 {
			b.Fatalf("got %d runs", len(logs.Runs))
		}
		for _, r := range logs.Runs {
			_ = BuildReport(r, DefaultOptions)
		}
	}
}
