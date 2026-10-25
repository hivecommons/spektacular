package runstats

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

const (
	listPlanTrace = "bbbbbbbb000000000000000000000002"
	listImplTrace = "cccccccc000000000000000000000003"
	listOrchOrder = "0000000a000000000000000000000010"
	listOrchSpan1 = "0000000b000000000000000000000011"
	listOrchSpan2 = "0000000c000000000000000000000012"
	listOrchMerge = "0000000d000000000000000000000013"
)

// argv is a command's process.command_args and exit code attributes.
func argv(code int, args ...string) map[string]any {
	return map[string]any{tracelog.AttrCommandArgs: args, tracelog.AttrExitCode: code}
}

// listingLogs writes two workflow runs and a day of an orchestrator's
// standalone records into a logs directory:
//
//   - a plan run with a step, a goto moving it, a helper under the step, a
//     user wait and a work span;
//   - an implement run with a failed `implement new` (no exit code
//     attribute) and a helper left open;
//   - outside any run: `epic order`, a helper span per child (the second
//     never stopped) and `epic merge`.
func listingLogs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := fx{trace: listPlanTrace, kind: "plan", name: "000001_alpha", agent: "claude"}
	newPlan := p.cmd("plan new", "", at(2), at(2.1), argv(0, "plan", "new", "--data", `{"name":"000001_alpha"}`))
	newPlan.Attributes[tracelog.AttrStepTo] = "discovery"
	gotoPlan := p.cmd("plan goto", "d1", at(6), at(6.2), argv(0, "plan", "goto", "--data", `{"step":"finished"}`))
	gotoPlan.Attributes[tracelog.AttrStep] = "discovery"
	gotoPlan.Attributes[tracelog.AttrStepTo] = "finished"
	writeLog(t, dir, "20261025T090200.000000000Z_claude_plan_000001_alpha.jsonl", recs(
		newPlan,
		p.span("d1", "discovery", tracelog.TypeStep, "", at(2.1), at(6.2), tracelog.EndMoved),
		p.span("ph", "research", tracelog.TypeSubAgent, "d1", at(3), at(4), tracelog.EndStopped),
		p.span("pu", "question", tracelog.TypeUserWait, "d1", at(4.5), at(5), tracelog.EndStopped),
		p.span("pw", "drafting", tracelog.TypeWork, "d1", at(5), at(5.5), tracelog.EndStopped),
		gotoPlan,
		p.span("pf", "finished", tracelog.TypeStep, "", at(6.2), at(6.2), tracelog.EndFinished),
	))

	m := fx{trace: listImplTrace, kind: "implement", name: "000002_beta", agent: "claude"}
	refused := m.cmd("implement new", "", at(3), at(3.1), map[string]any{
		tracelog.AttrCommandArgs: []string{"implement", "new", "--data", `{"name":"000002_beta"}`},
	})
	refused.Status = tracelog.Status{Code: tracelog.StatusError, Message: "dependencies are not met"}
	newImpl := m.cmd("implement new", "", at(3.5), at(3.6), argv(0, "implement", "new"))
	newImpl.Attributes[tracelog.AttrStepTo] = "read_plan"
	writeLog(t, dir, "20261025T090300.000000000Z_claude_implement_000002_beta.jsonl", recs(
		refused,
		newImpl,
		m.open("i1", "read_plan", tracelog.TypeStep, "", at(3.6)),
		m.open("ih", "reviewer", tracelog.TypeSubAgent, "i1", at(7), map[string]any{tracelog.AttrStep: "read_plan"}),
		m.cmd("plan file read", "i1", at(8), at(8.5), argv(0, "plan", "file", "read", "000002_beta", "plan")),
	))

	o := fx{trace: listOrchOrder, agent: "claude"}
	h1 := fx{trace: listOrchSpan1, agent: "claude"}
	h2 := fx{trace: listOrchSpan2, agent: "claude"}
	g := fx{trace: listOrchMerge, agent: "claude"}
	writeLog(t, dir, "standalone_20261025.jsonl", recs(
		o.cmd("epic order", "", at(0), at(0.5), argv(0, "epic", "order", "--data", `{"name":"e"}`)),
		h1.span("oh1", "plan 000001_alpha", tracelog.TypeSubAgent, "", at(1), at(7), tracelog.EndStopped),
		h2.open("oh2", "implement 000002_beta", tracelog.TypeSubAgent, "", at(1.5)),
		g.cmd("epic merge", "", at(9), at(9.5), argv(0, "epic", "merge", "--data", `{"spec":"000001_alpha"}`)),
	))
	return dir
}

func loadListing(t *testing.T) *Logs {
	t.Helper()
	logs, err := Load(listingLogs(t))
	require.NoError(t, err)
	return logs
}


func TestCommandsListsEveryCommandOldestFirst(t *testing.T) {
	logs := loadListing(t)
	rows, err := logs.Commands(Filter{}, false)
	require.NoError(t, err)

	var names []string
	for _, r := range rows {
		names = append(names, r.Command)
	}
	require.Equal(t, []string{"epic order", "plan new", "implement new", "implement new", "plan goto", "plan file read", "epic merge"}, names)

	order := rows[0]
	require.Nil(t, order.RunID, "a standalone command has no run")
	require.Nil(t, order.Kind)
	require.Nil(t, order.Name)
	require.Nil(t, order.Step)
	require.Nil(t, order.StepTo)
	require.Equal(t, str("claude"), order.Agent)
	require.Equal(t, []string{"epic", "order", "--data", `{"name":"e"}`}, order.Args)
	require.Equal(t, 0, order.ExitCode)
	require.Equal(t, int64(30000), order.DurationMS)

	goTo := rows[4]
	require.Equal(t, str(listPlanTrace), goTo.RunID)
	require.Equal(t, str("plan"), goTo.Kind)
	require.Equal(t, str("000001_alpha"), goTo.Name)
	require.Equal(t, str("discovery"), goTo.Step)
	require.Equal(t, str("finished"), goTo.StepTo)
	require.Equal(t, str("d1"), goTo.ParentSpanID)
	require.Equal(t, at(6), goTo.StartedAt)
	require.Equal(t, at(6.2), goTo.EndedAt)
	require.Equal(t, int64(12000), goTo.DurationMS)
	require.Equal(t, "ok", goTo.Status)
	require.Nil(t, goTo.Message)

	refused := rows[2]
	require.Equal(t, str(listImplTrace), refused.RunID)
	require.Equal(t, 1, refused.ExitCode, "with no exit code attribute, an error status is exit 1")
	require.Equal(t, "error", refused.Status)
	require.Equal(t, str("dependencies are not met"), refused.Message)
}

func TestCommandsWithNoArgsAttributeGiveAnEmptyList(t *testing.T) {
	a := fx{trace: listPlanTrace, kind: "plan", name: "x"}
	runs := Assemble(recs(a.cmd("plan new", "", at(0), at(1))))
	logs := &Logs{Runs: runs}
	rows, err := logs.Commands(Filter{}, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, []string{}, rows[0].Args)
	require.Equal(t, 0, rows[0].ExitCode)
}

func TestCommandsSelectionAndOutsideRuns(t *testing.T) {
	logs := loadListing(t)

	rows, err := logs.Commands(Filter{Name: "000002_beta"}, false)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for _, r := range rows {
		require.Equal(t, str(listImplTrace), r.RunID)
	}

	rows, err = logs.Commands(Filter{Runs: []string{listPlanTrace[:8]}}, false)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	rows, err = logs.Commands(Filter{}, true)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "epic order", rows[0].Command)
	require.Equal(t, "epic merge", rows[1].Command)
	for _, r := range rows {
		require.Nil(t, r.RunID)
	}

	_, err = logs.Commands(Filter{Kind: "plan"}, true)
	var se *SelectionError
	require.True(t, errors.As(err, &se))
	require.Equal(t, CodeBadSelection, se.Code)
	require.Contains(t, se.Error(), "--outside-runs")

	_, err = logs.Commands(Filter{Kind: "bogus"}, false)
	require.True(t, errors.As(err, &se))
	require.Equal(t, CodeBadKind, se.Code)
}

func TestAgentSpansListsHelpersWaitsAndWorkInAndOutsideRuns(t *testing.T) {
	logs := loadListing(t)
	rows, err := logs.AgentSpans(Filter{}, false)
	require.NoError(t, err)

	var ids []string
	for _, r := range rows {
		ids = append(ids, r.SpanID)
	}
	require.Equal(t, []string{"oh1", "oh2", "ph", "pu", "pw", "ih"}, ids)

	byID := map[string]SpanRow{}
	for _, r := range rows {
		byID[r.SpanID] = r
	}

	ph := byID["ph"]
	require.Equal(t, tracelog.TypeSubAgent, ph.Type)
	require.Equal(t, listPlanTrace, ph.TraceID)
	require.Equal(t, str(listPlanTrace), ph.RunID)
	require.Equal(t, str("plan"), ph.Kind)
	require.Equal(t, str("000001_alpha"), ph.Workflow)
	require.Equal(t, "research", ph.Name)
	require.Equal(t, str("discovery"), ph.Step, "the step entry it started in")
	require.Equal(t, str("d1"), ph.ParentSpanID)
	require.Equal(t, tracelog.EndStopped, ph.EndReason)
	require.True(t, ph.Complete)
	require.Equal(t, int64(60000), ph.DurationMS)

	require.Equal(t, tracelog.TypeUserWait, byID["pu"].Type)
	require.Equal(t, tracelog.TypeWork, byID["pw"].Type)

	// The open helper is capped exactly as the run report caps it.
	ih := byID["ih"]
	require.Equal(t, EndOpen, ih.EndReason)
	require.False(t, ih.Complete)
	rep := BuildReport(runByID(t, logs.Runs, listImplTrace), DefaultOptions)
	require.Len(t, rep.Helpers, 1)
	require.Equal(t, rep.Helpers[0].Wait, ih.Wait)
	require.Equal(t, at(8.5), ih.EndedAt)

	oh1 := byID["oh1"]
	require.Nil(t, oh1.RunID)
	require.Nil(t, oh1.Kind)
	require.Nil(t, oh1.Workflow)
	require.Nil(t, oh1.Step)
	require.Equal(t, listOrchSpan1, oh1.TraceID)
	require.True(t, oh1.Complete)
	require.Equal(t, at(7), oh1.EndedAt)

	// A standalone span never stopped holds nothing later in its own trace,
	// so it is capped at its start.
	oh2 := byID["oh2"]
	require.Equal(t, EndOpen, oh2.EndReason)
	require.False(t, oh2.Complete)
	require.Equal(t, at(1.5), oh2.EndedAt)
	require.Zero(t, oh2.DurationMS)
}

func TestAgentSpansSelectionAndOutsideRuns(t *testing.T) {
	logs := loadListing(t)

	rows, err := logs.AgentSpans(Filter{Kind: "plan"}, false)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for _, r := range rows {
		require.Equal(t, str(listPlanTrace), r.RunID)
	}

	rows, err = logs.AgentSpans(Filter{}, true)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "oh1", rows[0].SpanID)
	require.Equal(t, "oh2", rows[1].SpanID)

	_, err = logs.AgentSpans(Filter{Agent: "claude"}, true)
	var se *SelectionError
	require.True(t, errors.As(err, &se))
	require.Equal(t, CodeBadSelection, se.Code)
}

// TestOutsideRecordsLeaveRunsUnchanged: keeping the records outside any run
// changes nothing about the runs, their reports, comparisons or summaries.
func TestOutsideRecordsLeaveRunsUnchanged(t *testing.T) {
	dir := listingLogs(t)
	with, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(dir, "standalone_20261025.jsonl")))
	without, err := Load(dir)
	require.NoError(t, err)

	require.Len(t, with.Outside, 5)
	require.Empty(t, without.Outside)
	require.Len(t, with.Runs, 2)
	for i := range with.Runs {
		w, o := *with.Runs[i], *without.Runs[i]
		w.LogFile, o.LogFile = "", ""
		require.Equal(t, o, w)
		require.Equal(t, BuildReport(without.Runs[i], DefaultOptions), BuildReport(with.Runs[i], DefaultOptions))
	}
	require.Equal(t, Compare(without.Runs, DefaultOptions), Compare(with.Runs, DefaultOptions))
	require.Equal(t, Summarise(without.Runs, DefaultOptions), Summarise(with.Runs, DefaultOptions))
}

// TestListingJSONFieldNames pins the listings' JSON to the contract in the
// Data Structures section of plan 000076_harbor-on-built-in-logs: the key
// sets below are written by hand from it.
func TestListingJSONFieldNames(t *testing.T) {
	logs := loadListing(t)
	cmds, err := logs.Commands(Filter{}, false)
	require.NoError(t, err)
	spans, err := logs.AgentSpans(Filter{}, false)
	require.NoError(t, err)

	var doc struct {
		Commands []any `json:"commands"`
		Spans    []any `json:"spans"`
	}
	data, err := json.Marshal(map[string]any{"commands": cmds, "spans": spans})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &doc))

	require.Equal(t, sorted("run_id", "kind", "name", "agent", "span_id", "parent_span_id", "command", "args",
		"step", "step_to", "started_at", "ended_at", "duration_ms", "exit_code", "status", "message"), keys(t, doc.Commands[0]))
	require.Equal(t, sorted("span_id", "parent_span_id", "name", "step", "task_id", "started_at", "ended_at",
		"duration_ms", "end_reason", "status", "message", "complete",
		"type", "trace_id", "run_id", "kind", "workflow"), keys(t, doc.Spans[0]))
}

func TestListingGoldenJSON(t *testing.T) {
	logs := loadListing(t)
	cmds, err := logs.Commands(Filter{}, false)
	require.NoError(t, err)
	spans, err := logs.AgentSpans(Filter{}, false)
	require.NoError(t, err)

	for name, v := range map[string]any{
		"commands.golden.json": map[string]any{"commands": cmds},
		"spans.golden.json":    map[string]any{"spans": spans},
	} {
		got, err := json.MarshalIndent(v, "", "  ")
		require.NoError(t, err)
		got = append(got, '\n')
		path := filepath.Join("testdata", name)
		if *update {
			require.NoError(t, os.WriteFile(path, got, 0o644))
		}
		want, err := os.ReadFile(path)
		require.NoError(t, err, "run go test -run TestListingGoldenJSON -update to write it")
		require.Equal(t, string(want), string(got), name)
	}
}

func TestRenderCommandsAndSpans(t *testing.T) {
	logs := loadListing(t)
	cmds, err := logs.Commands(Filter{}, false)
	require.NoError(t, err)
	spans, err := logs.AgentSpans(Filter{}, false)
	require.NoError(t, err)

	var b bytes.Buffer
	require.NoError(t, RenderCommands(&b, cmds, 2))
	out := b.String()
	require.Contains(t, out, "TIME                 RUN       STEP       COMMAND         EXIT  DURATION")
	require.Contains(t, out, "2026-10-25 09:00:00  -         -          epic order      0     30.0s")
	require.Contains(t, out, "2026-10-25 09:06:00  bbbbbbbb  discovery  plan goto       0     12.0s")
	require.Contains(t, out, "2026-10-25 09:03:00  cccccccc  -          implement new   1     6.0s")
	require.Contains(t, out, "Skipped lines: 2")

	b.Reset()
	require.NoError(t, RenderSpans(&b, spans, 0))
	out = b.String()
	require.Contains(t, out, "TIME                 RUN       TYPE       NAME                   STEP       DURATION  END")
	require.Contains(t, out, "2026-10-25 09:03:00  bbbbbbbb  sub-agent  research               discovery  1m00s     stopped")
	require.Contains(t, out, "2026-10-25 09:01:30  -         sub-agent  implement 000002_beta  -          0.0s      open*")
	require.Contains(t, out, "did not finish")

	b.Reset()
	require.NoError(t, RenderCommands(&b, nil, 0))
	require.Equal(t, "No recorded commands.\n", b.String())
	b.Reset()
	require.NoError(t, RenderSpans(&b, nil, 0))
	require.Equal(t, "No recorded spans.\n", b.String())
}
