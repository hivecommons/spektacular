package runstats

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/tracelog"
)

// ErrNoLogDir is returned by Load when the logs directory does not exist. It
// wraps fs.ErrNotExist.
var ErrNoLogDir = fmt.Errorf("log directory not found: %w", fs.ErrNotExist)

// Logs is a logs directory read into runs.
type Logs struct {
	// Dir is the directory read.
	Dir string
	// Runs are the workflow runs found, newest first.
	Runs []*Run
	// Skipped counts the lines that could not be read as records.
	Skipped int
	// Outside are the records of traces with no workflow attributes, in
	// start order: the commands and spans written outside any run, such as
	// an epic orchestrator's. Only the listings read them.
	Outside []tracelog.Record
}

// loaded is one record and the base name of the file it was read from.
type loaded struct {
	rec  tracelog.Record
	file string
}

// Load reads every *.jsonl file in dir into runs. Files are only ever opened
// for reading. A line that is not a valid record is skipped and counted in
// Logs.Skipped; a blank line is skipped without counting. It returns an error
// only when the directory itself cannot be read: ErrNoLogDir when it does not
// exist.
func Load(dir string) (*Logs, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNoLogDir, dir)
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	logs := &Logs{Dir: dir}
	var recs []loaded
	for _, name := range names {
		got, skipped, err := readFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		logs.Skipped += skipped
		for _, r := range got {
			recs = append(recs, loaded{rec: r, file: name})
		}
	}
	logs.Runs, logs.Outside = assemble(recs)
	return logs, nil
}

// readFile reads the records in path, in file order, with no cap on a line's
// length. A last line with no newline is still read.
func readFile(path string) (recs []tracelog.Record, skipped int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	for {
		line, rerr := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if r, ok := parseLine(line); ok {
				recs = append(recs, r)
			} else {
				skipped++
			}
		}
		if rerr == io.EOF {
			return recs, skipped, nil
		}
		if rerr != nil {
			return nil, 0, rerr
		}
	}
}

// parseLine decodes one line into a record, reporting false for a line that
// is not one: unparsable JSON, no trace id or span id, an unknown record
// kind, no start time, or a span_end with no end time.
func parseLine(line []byte) (tracelog.Record, bool) {
	var r tracelog.Record
	if json.Unmarshal(line, &r) != nil {
		return r, false
	}
	if r.TraceID == "" || r.SpanID == "" || r.StartTime.IsZero() {
		return r, false
	}
	switch r.Record {
	case tracelog.RecordSpan, tracelog.RecordSpanStart:
	case tracelog.RecordSpanEnd:
		if r.EndTime.IsZero() {
			return r, false
		}
	default:
		return r, false
	}
	return r, true
}
