package runstats

import (
	"sort"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
)

// Options tune a report's gap figures.
type Options struct {
	// Gaps is the most gaps listed; 0 lists every gap.
	Gaps int
	// GapThreshold is the length a gap must exceed to count toward the gap
	// total.
	GapThreshold time.Duration
}

// DefaultOptions list the 10 longest gaps and total those over 5 minutes.
var DefaultOptions = Options{Gaps: 10, GapThreshold: 5 * time.Minute}

// RunSummary is one run's line in a listing, and the head of every report.
type RunSummary struct {
	RunID     string    `json:"run_id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Agent     string    `json:"agent"`
	LogFile   string    `json:"log_file"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	WallMS    int64     `json:"wall_ms"`
	Status    string    `json:"status"`
}

// Summary is the run's summary line.
func (r *Run) Summary() RunSummary {
	return RunSummary{
		RunID: r.ID, Kind: r.Kind, Name: r.Name, Agent: r.Agent, LogFile: r.LogFile,
		StartedAt: r.Start.UTC(), EndedAt: r.End.UTC(), WallMS: ms(r.Wall()), Status: r.Status,
	}
}

// Report is where one run's time went. Every duration is in integer
// milliseconds; a value that does not apply is null.
//
// Its JSON is a contract read by the Harbor harness (spec 000076) and the
// run viewer (spec 000077), fixed by the Data Structures section of plan
// 000075_run-stats-command; the `stats run` command adds log_dir and
// skipped_lines beside these fields. Durations are integer milliseconds
// (*_ms), times are RFC 3339 UTC, and absent values are null, never 0:
//
//	{"gap_threshold_ms":300000,
//	 "run":{"run_id","kind","name","agent","log_file","started_at","ended_at","wall_ms","status"},
//	 "totals":{"wall_ms","working_ms","user_wait_ms","helper_wait_ms",
//	           "command_ms","commands","helpers","user_waits",
//	           "gap_ms","gaps_over_threshold"},
//	 "steps":[{"step","entries","duration_ms","commands","command_ms",
//	           "helper_wait_ms","user_wait_ms","incomplete"}],
//	 "step_entries":[{"step","span_id","task_id","started_at","ended_at","duration_ms",
//	                  "end_reason","complete"}],
//	 "tasks":[{"task_id","name","span_id","started_at","ended_at","duration_ms",
//	           "helper_wait_ms","user_wait_ms","end_reason","complete"}],
//	 "helpers":[{"span_id","parent_span_id","name","step","task_id","started_at","ended_at",
//	             "duration_ms","end_reason","status","message","complete"}],
//	 "user_waits":[same shape as helpers],
//	 "gaps":[{"started_at","ended_at","duration_ms","step",
//	          "after":{"event","type","name"},"before":{"event","type","name"}}]}
//
// status is finished, interrupted or in_progress; end_reason is moved,
// finished, stopped, abandoned, interrupted, or open for a span never
// closed; complete is true for moved, finished and stopped. Renaming a
// field breaks TestReportJSONFieldNamesAndUnits and the golden file
// testdata/report.golden.json before it breaks a consumer.
type Report struct {
	GapThresholdMS int64       `json:"gap_threshold_ms"`
	Run            RunSummary  `json:"run"`
	Totals         Totals      `json:"totals"`
	Steps          []StepRow   `json:"steps"`
	StepEntries    []StepEntry `json:"step_entries"`
	Tasks          []TaskRow   `json:"tasks"`
	Helpers        []Wait      `json:"helpers"`
	UserWaits      []Wait      `json:"user_waits"`
	Gaps           []Gap       `json:"gaps"`
}

// Totals are a run's whole-run figures. Waits are the union of completed
// waits, so overlapping helpers count once; Commands, Helpers and UserWaits
// count every item, finished or not; GapMS totals the gaps longer than the
// threshold, GapsOverThreshold counts them.
type Totals struct {
	WallMS            int64 `json:"wall_ms"`
	WorkingMS         int64 `json:"working_ms"`
	UserWaitMS        int64 `json:"user_wait_ms"`
	HelperWaitMS      int64 `json:"helper_wait_ms"`
	CommandMS         int64 `json:"command_ms"`
	Commands          int   `json:"commands"`
	Helpers           int   `json:"helpers"`
	UserWaits         int   `json:"user_waits"`
	GapMS             int64 `json:"gap_ms"`
	GapsOverThreshold int   `json:"gaps_over_threshold"`
}

// StepRow is one step name's figures over every entry into it. Incomplete
// counts the entries that did not end moved or finished.
type StepRow struct {
	Step         string `json:"step"`
	Entries      int    `json:"entries"`
	DurationMS   int64  `json:"duration_ms"`
	Commands     int    `json:"commands"`
	CommandMS    int64  `json:"command_ms"`
	HelperWaitMS int64  `json:"helper_wait_ms"`
	UserWaitMS   int64  `json:"user_wait_ms"`
	Incomplete   int    `json:"incomplete"`
}

// StepEntry is one entry into a step.
type StepEntry struct {
	Step       string    `json:"step"`
	SpanID     string    `json:"span_id"`
	TaskID     *string   `json:"task_id"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMS int64     `json:"duration_ms"`
	EndReason  string    `json:"end_reason"`
	Complete   bool      `json:"complete"`
}

// TaskRow is one plan task of an implement run, over every span it had.
type TaskRow struct {
	TaskID       string    `json:"task_id"`
	Name         string    `json:"name"`
	SpanID       string    `json:"span_id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	DurationMS   int64     `json:"duration_ms"`
	HelperWaitMS int64     `json:"helper_wait_ms"`
	UserWaitMS   int64     `json:"user_wait_ms"`
	EndReason    string    `json:"end_reason"`
	Complete     bool      `json:"complete"`
}

// Wait is one helper-agent launch or one wait on the user.
type Wait struct {
	SpanID       string    `json:"span_id"`
	ParentSpanID *string   `json:"parent_span_id"`
	Name         string    `json:"name"`
	Step         *string   `json:"step"`
	TaskID       *string   `json:"task_id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	DurationMS   int64     `json:"duration_ms"`
	EndReason    string    `json:"end_reason"`
	Status       *string   `json:"status"`
	Message      *string   `json:"message"`
	Complete     bool      `json:"complete"`
}

// Gap is an idle stretch on the run's timeline, with the step it fell in
// and the events either side.
type Gap struct {
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMS int64     `json:"duration_ms"`
	Step       *string   `json:"step"`
	After      GapEdge   `json:"after"`
	Before     GapEdge   `json:"before"`
}

// GapEdge names an event beside a gap: its edge (start or end), its span
// type, and its span or command name.
type GapEdge struct {
	Event string `json:"event"`
	Type  string `json:"type"`
	Name  string `json:"name"`
}

// BuildReport reports where r's time went. It is pure.
func BuildReport(r *Run, o Options) Report {
	rep := Report{
		GapThresholdMS: ms(o.GapThreshold),
		Run:            r.Summary(),
		Steps:          []StepRow{},
		StepEntries:    []StepEntry{},
		Tasks:          []TaskRow{},
		Helpers:        []Wait{},
		UserWaits:      []Wait{},
		Gaps:           []Gap{},
	}

	var helperIvs, waitIvs []interval
	var steps, tasks []Span
	byID := map[string]Span{}
	for _, s := range r.Spans {
		byID[s.ID] = s
		switch s.Type {
		case tracelog.TypeSubAgent:
			rep.Totals.Helpers++
			if s.Complete {
				helperIvs = append(helperIvs, interval{s.Start, s.End})
			}
		case tracelog.TypeUserWait:
			rep.Totals.UserWaits++
			if s.Complete {
				waitIvs = append(waitIvs, interval{s.Start, s.End})
			}
		case tracelog.TypeStep:
			steps = append(steps, s)
		case tracelog.TypeTask:
			tasks = append(tasks, s)
		}
	}
	helpers, waits := union(helperIvs), union(waitIvs)

	t := &rep.Totals
	t.WallMS = ms(r.Wall())
	t.UserWaitMS = ms(total(waits))
	t.HelperWaitMS = ms(total(helpers))
	t.WorkingMS = t.WallMS - t.UserWaitMS
	t.Commands = len(r.Commands)
	cmdsBySpan := map[string][]time.Duration{}
	var cmdTotal time.Duration
	for _, c := range r.Commands {
		d := c.EndTime.Sub(c.StartTime)
		if d < 0 {
			d = 0
		}
		cmdTotal += d
		cmdsBySpan[c.ParentSpanID] = append(cmdsBySpan[c.ParentSpanID], d)
	}
	t.CommandMS = ms(cmdTotal)

	waitsIn := func(start, end time.Time) (helper, user time.Duration) {
		w := interval{start, end}
		return total(clip(helpers, w)), total(clip(waits, w))
	}

	// Step entries and the per-step rows they roll up into.
	rowIndex := map[string]int{}
	for _, s := range steps {
		entry := StepEntry{
			Step: s.Name, SpanID: s.ID, TaskID: taskOf(s, byID),
			StartedAt: s.Start.UTC(), EndedAt: s.End.UTC(), DurationMS: ms(s.Duration()),
			EndReason: s.EndReason, Complete: s.Complete,
		}
		rep.StepEntries = append(rep.StepEntries, entry)

		i, ok := rowIndex[s.Name]
		if !ok {
			i = len(rep.Steps)
			rowIndex[s.Name] = i
			rep.Steps = append(rep.Steps, StepRow{Step: s.Name})
		}
		row := &rep.Steps[i]
		row.Entries++
		row.DurationMS += entry.DurationMS
		if s.EndReason != tracelog.EndMoved && s.EndReason != tracelog.EndFinished {
			row.Incomplete++
		}
		var cmdTime time.Duration
		for _, d := range cmdsBySpan[s.ID] {
			cmdTime += d
		}
		row.Commands += len(cmdsBySpan[s.ID])
		row.CommandMS += ms(cmdTime)
		h, u := waitsIn(s.Start, s.End)
		row.HelperWaitMS += ms(h)
		row.UserWaitMS += ms(u)
	}

	// Tasks, one row per task id over every span it had.
	taskIndex := map[string]int{}
	for _, s := range tasks {
		key := s.TaskID
		if key == "" {
			key = "span:" + s.ID
		}
		h, u := waitsIn(s.Start, s.End)
		i, ok := taskIndex[key]
		if !ok {
			taskIndex[key] = len(rep.Tasks)
			rep.Tasks = append(rep.Tasks, TaskRow{
				TaskID: s.TaskID, Name: s.Name, SpanID: s.ID,
				StartedAt: s.Start.UTC(), EndedAt: s.End.UTC(), DurationMS: ms(s.Duration()),
				HelperWaitMS: ms(h), UserWaitMS: ms(u), EndReason: s.EndReason, Complete: s.Complete,
			})
			continue
		}
		row := &rep.Tasks[i]
		row.DurationMS += ms(s.Duration())
		row.HelperWaitMS += ms(h)
		row.UserWaitMS += ms(u)
		if s.End.After(row.EndedAt) {
			row.EndedAt = s.End.UTC()
		}
		row.EndReason, row.Complete = s.EndReason, s.Complete
	}

	// Helper launches and waits on the user.
	for _, s := range r.Spans {
		if s.Type != tracelog.TypeSubAgent && s.Type != tracelog.TypeUserWait {
			continue
		}
		w := Wait{
			SpanID: s.ID, ParentSpanID: optional(s.Parent), Name: s.Name,
			Step: optional(s.Step), TaskID: taskOf(s, byID),
			StartedAt: s.Start.UTC(), EndedAt: s.End.UTC(), DurationMS: ms(s.Duration()),
			EndReason: s.EndReason, Status: optional(s.Status), Message: optional(s.Message), Complete: s.Complete,
		}
		if w.Step == nil {
			w.Step = stepAt(steps, s.Start)
		}
		if w.TaskID == nil {
			w.TaskID = taskAt(tasks, s.Start)
		}
		if s.Type == tracelog.TypeSubAgent {
			rep.Helpers = append(rep.Helpers, w)
		} else {
			rep.UserWaits = append(rep.UserWaits, w)
		}
	}

	// Gaps: every one counts toward the threshold total; the longest are
	// listed.
	all := gaps(r)
	for _, g := range all {
		if d := g.to.Sub(g.from); d > o.GapThreshold {
			t.GapMS += ms(d)
			t.GapsOverThreshold++
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		di, dj := all[i].to.Sub(all[i].from), all[j].to.Sub(all[j].from)
		if di != dj {
			return di > dj
		}
		return all[i].from.Before(all[j].from)
	})
	if o.Gaps > 0 && len(all) > o.Gaps {
		all = all[:o.Gaps]
	}
	for _, g := range all {
		rep.Gaps = append(rep.Gaps, Gap{
			StartedAt: g.from.UTC(), EndedAt: g.to.UTC(), DurationMS: ms(g.to.Sub(g.from)),
			Step: stepAt(steps, g.from), After: g.after, Before: g.before,
		})
	}
	return rep
}

// taskOf is the task id of the task span above s, through its parents.
func taskOf(s Span, byID map[string]Span) *string {
	seen := map[string]bool{}
	for p := s.Parent; p != "" && !seen[p]; {
		seen[p] = true
		parent, ok := byID[p]
		if !ok {
			return nil
		}
		if parent.Type == tracelog.TypeTask {
			return optional(parent.TaskID)
		}
		p = parent.Parent
	}
	return nil
}

// stepAt is the name of the step entry whose [start, end) holds at.
func stepAt(steps []Span, at time.Time) *string {
	for _, s := range steps {
		if !at.Before(s.Start) && at.Before(s.End) {
			return optional(s.Name)
		}
	}
	return nil
}

// taskAt is the task id of the task span whose [start, end) holds at.
func taskAt(tasks []Span, at time.Time) *string {
	for _, s := range tasks {
		if !at.Before(s.Start) && at.Before(s.End) {
			return optional(s.TaskID)
		}
	}
	return nil
}

// optional is s, or nil when s is empty.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ms is d in whole milliseconds.
func ms(d time.Duration) int64 { return d.Milliseconds() }
