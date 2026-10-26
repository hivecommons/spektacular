package runview

import (
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
)

// Version is the page data contract's version. A change to a field the page
// script reads bumps it.
const Version = 1

// Problem markers, the values of Problems. A row's Problem indexes them.
const (
	ProblemNone        = ""
	ProblemFailed      = "failed"
	ProblemInterrupted = "interrupted"
	ProblemAbandoned   = "abandoned"
	ProblemOpen        = "open"
)

// Types are the span types a row's Type indexes, in a fixed order.
var Types = []string{
	tracelog.TypeCommand, tracelog.TypeStep, tracelog.TypeTask,
	tracelog.TypeSubAgent, tracelog.TypeUserWait, tracelog.TypeWork,
}

// Problems are the problem markers a row's Problem indexes, in a fixed
// order; 0 is none.
var Problems = []string{ProblemNone, ProblemFailed, ProblemInterrupted, ProblemAbandoned, ProblemOpen}

// Row statuses, the values of Rows.Status.
const (
	StatusNone  = -1 // an open span, which has no status yet
	StatusOK    = 0
	StatusError = 1
)

// Model is the page data: the contract between the Go builder and the page
// script, embedded in the page as one JSON object. Rows and gaps are
// columnar: index i across their arrays is item i. Strings that repeat are
// indexes into Strings, and -1 means none.
//
//	{"version":1,
//	 "run":{"run_id","kind","name","agent","status","started_at","ended_at","skipped_lines"},
//	 "totals":{"wall_ms","working_ms","user_wait_ms","helper_wait_ms"},
//	 "gap_threshold_ms","gap_floor_ms",
//	 "types":[..],"problems":[..],"strings":[..],
//	 "rows":{"parent","depth","start_ms","duration_ms","type","name","problem","end_reason",
//	         "status","message","step","task_id","span_id","args","exit_code"},
//	 "gaps":{"start_ms","duration_ms","step","after","before"},
//	 "longest_steps":[row index, ..]}
//
// Rows are in tree order: each parent before its children, siblings by
// start time, so a row's descendants are the rows after it with a greater
// depth. Times are milliseconds from the run's start; a duration runs to the
// span's effective end. Renaming a field breaks the golden file
// testdata/model.golden.json before it breaks the page.
type Model struct {
	Version        int      `json:"version"`
	Run            RunInfo  `json:"run"`
	Totals         Totals   `json:"totals"`
	GapThresholdMS int64    `json:"gap_threshold_ms"`
	GapFloorMS     int64    `json:"gap_floor_ms"`
	Types          []string `json:"types"`
	Problems       []string `json:"problems"`
	Strings        []string `json:"strings"`
	Rows           Rows     `json:"rows"`
	Gaps           Gaps     `json:"gaps"`
	LongestSteps   []int    `json:"longest_steps"`
}

// RunInfo is the run's identity. It deliberately carries no log file or
// other path, so the page does not depend on where the logs were read.
type RunInfo struct {
	RunID        string    `json:"run_id"`
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	Agent        string    `json:"agent"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	SkippedLines int       `json:"skipped_lines"`
}

// Totals are the run statistics report's whole-run times, copied from it.
type Totals struct {
	WallMS       int64 `json:"wall_ms"`
	WorkingMS    int64 `json:"working_ms"`
	UserWaitMS   int64 `json:"user_wait_ms"`
	HelperWaitMS int64 `json:"helper_wait_ms"`
}

// Rows are one bar per span and command record, columnar.
type Rows struct {
	Parent     []int    `json:"parent"`      // row index, or -1 at the top level
	Depth      []int    `json:"depth"`       // nesting level, 0 at the top
	StartMS    []int64  `json:"start_ms"`    // offset from the run's start
	DurationMS []int64  `json:"duration_ms"` // to the effective end
	Type       []int    `json:"type"`        // index into Types
	Name       []int    `json:"name"`        // string index
	Problem    []int    `json:"problem"`     // index into Problems
	EndReason  []int    `json:"end_reason"`  // string index, or -1
	Status     []int    `json:"status"`      // StatusOK, StatusError or StatusNone
	Message    []int    `json:"message"`     // string index, or -1
	Step       []int    `json:"step"`        // string index, or -1
	TaskID     []int    `json:"task_id"`     // string index, or -1
	SpanID     []string `json:"span_id"`
	Args       []int    `json:"args"`      // a command's command line, string index, or -1
	ExitCode   []*int   `json:"exit_code"` // a command's exit code, null for a span
}

// Gaps are the idle stretches of the run of at least the gap floor, in time
// order, columnar.
type Gaps struct {
	StartMS    []int64 `json:"start_ms"`
	DurationMS []int64 `json:"duration_ms"`
	Step       []int   `json:"step"`   // string index, or -1
	After      []int   `json:"after"`  // label string index, such as "end of command plan goto"
	Before     []int   `json:"before"` // label string index
}
