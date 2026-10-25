package tracelog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Reader is the optional read side of a Logger. A logger that implements it
// lets the span logic find open spans in a run's log: close the spans still
// open under one that ends, validate a stop or a parent, and find a step's
// last command. A logger without it still has its spans written, but
// unclosed agent spans are not swept and stops are not validated.
type Reader interface {
	// Records returns run's span, span_start and span_end records in file
	// order. A missing log is no records, not an error.
	Records(run Run) ([]Record, error)
}

// maxRecordLine bounds one line the reader decodes. A verbose record carries
// a whole response, so lines can be large.
const maxRecordLine = 16 << 20

// Records reads run's records from its file. A standalone run's records are
// read from the day files of its start and the day before, keeping only
// those of its trace. Lines that are not span records, cannot be parsed, or
// were left partly written are skipped.
func (l *fileLogger) Records(run Run) ([]Record, error) {
	if !run.Standalone() {
		return readRecordFile(runFilePath(l.dir, run), "")
	}
	day := run.StartedAt.UTC()
	var recs []Record
	for _, d := range []time.Time{day.AddDate(0, 0, -1), day} {
		path := filepath.Join(l.dir, runFileName(Run{StartedAt: d}))
		got, err := readRecordFile(path, run.TraceID)
		if err != nil {
			return nil, err
		}
		recs = append(recs, got...)
	}
	return recs, nil
}

// recordMarker is the substring every span record's line holds. Checking for
// it before decoding skips other lines cheaply. It also matches the
// two longer kinds, which the decode tells apart.
var recordMarker = []byte(`"record":"span`)

// readRecordFile returns the span records in path, in file order, keeping
// only trace's when trace is not empty.
func readRecordFile(path, trace string) ([]Record, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var traceMarker []byte
	if trace != "" {
		traceMarker = []byte(`"trace_id":"` + trace + `"`)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxRecordLine)
	var recs []Record
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, recordMarker) {
			continue
		}
		if traceMarker != nil && !bytes.Contains(line, traceMarker) {
			continue
		}
		var r Record
		if json.Unmarshal(line, &r) != nil {
			continue
		}
		switch r.Record {
		case RecordSpan, RecordSpanStart, RecordSpanEnd:
		default:
			continue
		}
		if trace != "" && r.TraceID != trace {
			continue
		}
		recs = append(recs, r)
	}
	// A line longer than maxRecordLine stops the scan: the records read so
	// far are still returned, since reading a log must never fail a command.
	return recs, nil
}
