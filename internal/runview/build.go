package runview

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hivecommons/spektacular/internal/runstats"
	"github.com/hivecommons/spektacular/internal/tracelog"
)

// Options tune the page data.
type Options struct {
	// GapThreshold is the page's default gap threshold (--gap-threshold).
	GapThreshold time.Duration
	// Skipped is the logs' unreadable line count, shown on the page.
	Skipped int
}

// maxArgsBytes is the most of a command line a row carries.
const maxArgsBytes = 2000

// longestStepCount is the number of step entries the side summary lists.
const longestStepCount = 5

// DefaultFileName is the page's file name when --out names no file:
// spektacular-run-<first 8 characters of the run id>.html.
func DefaultFileName(runID string) string {
	id := runID
	if len(id) > 8 {
		id = id[:8]
	}
	return "spektacular-run-" + id + ".html"
}

// GapFloor is the shortest gap the page carries: one second, or the
// threshold when that is shorter, so the threshold can be lowered on the
// page without the file carrying every sub-second pause.
func GapFloor(threshold time.Duration) time.Duration {
	return min(time.Second, threshold)
}

// node is one bar before it is placed in the tree.
type node struct {
	id, parent, name, typ  string
	start, end             time.Time
	endReason, status, msg string
	step, taskID           string
	args                   string
	exitCode               *int
	command                bool
}

// Build turns r into the page data. It builds the run's report with
// runstats.BuildReport, asking for every gap, so the page's totals and gaps
// are the report's. It is pure: no I/O and no clock.
func Build(r *runstats.Run, o Options) Model {
	rep := runstats.BuildReport(r, runstats.Options{Gaps: 0, GapThreshold: o.GapThreshold})

	m := Model{
		Version: Version,
		Run: RunInfo{
			RunID: r.ID, Kind: r.Kind, Name: r.Name, Agent: r.Agent, Status: r.Status,
			StartedAt: r.Start.UTC(), EndedAt: r.End.UTC(), SkippedLines: o.Skipped,
		},
		Totals: Totals{
			WallMS: rep.Totals.WallMS, WorkingMS: rep.Totals.WorkingMS,
			UserWaitMS: rep.Totals.UserWaitMS, HelperWaitMS: rep.Totals.HelperWaitMS,
		},
		GapThresholdMS: o.GapThreshold.Milliseconds(),
		GapFloorMS:     GapFloor(o.GapThreshold).Milliseconds(),
		Types:          append([]string(nil), Types...),
		Problems:       append([]string(nil), Problems...),
		LongestSteps:   []int{},
	}
	st := &interner{index: map[string]int{}, list: []string{}}

	nodes := collect(r)
	order, parents, depths := tree(nodes)
	m.Rows = newRows(len(order))
	for i, ni := range order {
		n := nodes[ni]
		rw := &m.Rows
		rw.Parent[i] = parents[i]
		rw.Depth[i] = depths[i]
		rw.StartMS[i] = n.start.Sub(r.Start).Milliseconds()
		rw.DurationMS[i] = max(0, n.end.Sub(n.start).Milliseconds())
		rw.Type[i] = typeIndex(n.typ)
		rw.Name[i] = st.add(n.name)
		rw.Problem[i] = problemIndex(n)
		rw.EndReason[i] = st.add(n.endReason)
		rw.Status[i] = statusIndex(n.status)
		rw.Message[i] = st.add(n.msg)
		rw.Step[i] = st.add(n.step)
		rw.TaskID[i] = st.add(n.taskID)
		rw.SpanID[i] = n.id
		rw.Args[i] = st.add(n.args)
		rw.ExitCode[i] = n.exitCode
	}

	m.Gaps = buildGaps(r, rep.Gaps, GapFloor(o.GapThreshold), st)
	m.LongestSteps = longestSteps(m.Rows)
	m.Strings = st.list
	return m
}

// collect makes one node per span and per command record of r.
func collect(r *runstats.Run) []node {
	nodes := make([]node, 0, len(r.Spans)+len(r.Commands))
	for _, s := range r.Spans {
		nodes = append(nodes, node{
			id: s.ID, parent: s.Parent, name: s.Name, typ: s.Type,
			start: s.Start, end: s.End, endReason: s.EndReason, status: s.Status, msg: s.Message,
			step: s.Step, taskID: s.TaskID,
		})
	}
	for _, c := range r.Commands {
		end := c.EndTime
		if end.Before(c.StartTime) {
			end = c.StartTime
		}
		code, ok := intAttr(c, tracelog.AttrExitCode)
		if !ok && c.Status.Code != "" && c.Status.Code != tracelog.StatusOK {
			code = 1
		}
		nodes = append(nodes, node{
			id: c.SpanID, parent: c.ParentSpanID, name: c.Name, typ: tracelog.TypeCommand,
			start: c.StartTime, end: end, status: c.Status.Code, msg: c.Status.Message,
			step: attr(c, tracelog.AttrStep), taskID: attr(c, tracelog.AttrTaskID),
			args: commandLine(c), exitCode: &code, command: true,
		})
	}
	return nodes
}

// typeRank orders siblings that start at the same moment: containers first.
var typeRank = map[string]int{
	tracelog.TypeTask: 0, tracelog.TypeStep: 1, tracelog.TypeSubAgent: 2,
	tracelog.TypeUserWait: 3, tracelog.TypeWork: 4, tracelog.TypeCommand: 5,
}

func rank(typ string) int {
	if r, ok := typeRank[typ]; ok {
		return r
	}
	return typeRank[tracelog.TypeWork]
}

// tree orders nodes depth first: roots, then each node's children, siblings
// by start time, type rank and span id. A node is a root when it has no
// parent, or its parent is not in the run. Nodes caught in a parent cycle,
// which no root reaches, are placed at the top level too. It returns the
// node indexes in row order, with each row's parent row and depth.
func tree(nodes []node) (order, parents, depths []int) {
	byID := map[string]int{}
	for i, n := range nodes {
		if n.id == "" {
			continue
		}
		if _, ok := byID[n.id]; !ok {
			byID[n.id] = i
		}
	}
	less := func(a, b int) bool {
		x, y := nodes[a], nodes[b]
		if !x.start.Equal(y.start) {
			return x.start.Before(y.start)
		}
		if rx, ry := rank(x.typ), rank(y.typ); rx != ry {
			return rx < ry
		}
		if x.id != y.id {
			return x.id < y.id
		}
		return a < b
	}

	children := map[int][]int{}
	var roots []int
	for i, n := range nodes {
		p, ok := byID[n.parent]
		if n.parent == "" || !ok || p == i {
			roots = append(roots, i)
			continue
		}
		children[p] = append(children[p], i)
	}
	for _, c := range children {
		sort.Slice(c, func(i, j int) bool { return less(c[i], c[j]) })
	}
	sort.Slice(roots, func(i, j int) bool { return less(roots[i], roots[j]) })

	visited := make([]bool, len(nodes))
	var walk func(n, parentRow, depth int)
	walk = func(n, parentRow, depth int) {
		visited[n] = true
		row := len(order)
		order = append(order, n)
		parents = append(parents, parentRow)
		depths = append(depths, depth)
		for _, c := range children[n] {
			if !visited[c] {
				walk(c, row, depth+1)
			}
		}
	}
	for _, n := range roots {
		walk(n, -1, 0)
	}
	// Anything left is in a parent cycle: place it at the top level.
	rest := []int{}
	for i := range nodes {
		if !visited[i] {
			rest = append(rest, i)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return less(rest[i], rest[j]) })
	for _, n := range rest {
		if !visited[n] {
			walk(n, -1, 0)
		}
	}
	return order, parents, depths
}

// problemIndex is n's problem marker, the first that applies: interrupted,
// abandoned or open from the end reason; failed for an error status, or a
// command with a non-zero exit code; otherwise none.
func problemIndex(n node) int {
	p := ProblemNone
	switch {
	case n.endReason == tracelog.EndInterrupted:
		p = ProblemInterrupted
	case n.endReason == tracelog.EndAbandoned:
		p = ProblemAbandoned
	case n.endReason == runstats.EndOpen:
		p = ProblemOpen
	case n.status == tracelog.StatusError:
		p = ProblemFailed
	case n.command && n.exitCode != nil && *n.exitCode != 0:
		p = ProblemFailed
	}
	for i, v := range Problems {
		if v == p {
			return i
		}
	}
	return 0
}

// typeIndex is typ's index in Types; a type the page does not know is drawn
// as other marked work.
func typeIndex(typ string) int {
	for i, t := range Types {
		if t == typ {
			return i
		}
	}
	for i, t := range Types {
		if t == tracelog.TypeWork {
			return i
		}
	}
	return 0
}

func statusIndex(code string) int {
	switch code {
	case tracelog.StatusOK:
		return StatusOK
	case tracelog.StatusError:
		return StatusError
	}
	return StatusNone
}

// buildGaps keeps the report's gaps of at least floor, in time order.
func buildGaps(r *runstats.Run, all []runstats.Gap, floor time.Duration, st *interner) Gaps {
	kept := []runstats.Gap{}
	for _, g := range all {
		if g.DurationMS >= floor.Milliseconds() {
			kept = append(kept, g)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].StartedAt.Before(kept[j].StartedAt) })
	out := Gaps{
		StartMS: make([]int64, len(kept)), DurationMS: make([]int64, len(kept)),
		Step: make([]int, len(kept)), After: make([]int, len(kept)), Before: make([]int, len(kept)),
	}
	for i, g := range kept {
		out.StartMS[i] = g.StartedAt.Sub(r.Start).Milliseconds()
		out.DurationMS[i] = g.DurationMS
		out.Step[i] = -1
		if g.Step != nil {
			out.Step[i] = st.add(*g.Step)
		}
		out.After[i] = st.add(edgeLabel(g.After))
		out.Before[i] = st.add(edgeLabel(g.Before))
	}
	return out
}

// edgeLabel names an event beside a gap, such as "end of command plan goto".
func edgeLabel(e runstats.GapEdge) string {
	return strings.TrimSpace(fmt.Sprintf("%s of %s %s", e.Event, e.Type, e.Name))
}

// longestSteps is the row indexes of the longest step entries, longest
// first, at most longestStepCount.
func longestSteps(rw Rows) []int {
	stepType := typeIndex(tracelog.TypeStep)
	var steps []int
	for i, t := range rw.Type {
		if t == stepType {
			steps = append(steps, i)
		}
	}
	sort.SliceStable(steps, func(a, b int) bool {
		x, y := steps[a], steps[b]
		if rw.DurationMS[x] != rw.DurationMS[y] {
			return rw.DurationMS[x] > rw.DurationMS[y]
		}
		return rw.StartMS[x] < rw.StartMS[y]
	})
	if len(steps) > longestStepCount {
		steps = steps[:longestStepCount]
	}
	return append([]int{}, steps...)
}

func newRows(n int) Rows {
	return Rows{
		Parent: make([]int, n), Depth: make([]int, n), StartMS: make([]int64, n), DurationMS: make([]int64, n),
		Type: make([]int, n), Name: make([]int, n), Problem: make([]int, n), EndReason: make([]int, n),
		Status: make([]int, n), Message: make([]int, n), Step: make([]int, n), TaskID: make([]int, n),
		SpanID: make([]string, n), Args: make([]int, n), ExitCode: make([]*int, n),
	}
}

// interner is the string table: each distinct string once, in first-use
// order, so the output is deterministic. The empty string is -1.
type interner struct {
	index map[string]int
	list  []string
}

func (s *interner) add(v string) int {
	if v == "" {
		return -1
	}
	if i, ok := s.index[v]; ok {
		return i
	}
	s.index[v] = len(s.list)
	s.list = append(s.list, v)
	return len(s.list) - 1
}

// commandLine is a command's arguments joined by spaces, cut to
// maxArgsBytes with a trailing ellipsis.
func commandLine(r tracelog.Record) string {
	var args []string
	switch v := r.Attributes[tracelog.AttrCommandArgs].(type) {
	case []string:
		args = v
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				args = append(args, s)
			}
		}
	}
	line := strings.Join(args, " ")
	if len(line) <= maxArgsBytes {
		return line
	}
	cut := maxArgsBytes
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + "…"
}

// attr is a record's string attribute, or "".
func attr(r tracelog.Record, key string) string {
	s, _ := r.Attributes[key].(string)
	return s
}

// intAttr is a record's integer attribute, read from a decoded log line
// (float64) or built in memory.
func intAttr(r tracelog.Record, key string) (int, bool) {
	switch v := r.Attributes[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	}
	return 0, false
}
