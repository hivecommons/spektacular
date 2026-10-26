---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Context: 000074_trace-spans-for-agent-work

## Current State Analysis

- No spans beyond commands exist yet. 000073 (planned, not implemented) makes `runRoot` write one `span` record per command to `.spektacular/logs/<started>_<agent>_<kind>_<name>.jsonl` (or `standalone_<day>.jsonl`). It also adds `run_id` to workflow state and reserves `parent_span_id`, the `record` discriminator and `spektacular.span.type` for this spec.
- The workflow engine (`internal/workflow/workflow.go:98-339`) persists state in `enter_state` on every transition. It renders instructions in `before_<event>` callbacks, chains steps through `pendingGoto`, and re-renders the current step through `renderStep` (the resume path) without touching progress. Its runtime `Config` (`:16-48`) is never persisted.
- Restarts go through `clearState` (`cmd/resume.go:216-218`). Orchestrated runs keep their own state file under `.spektacular/workflows/<kind>-<name>.json` (`internal/workflow/lane.go:20-22`), which `runRoot` selects by argument (`cmd/root.go:247-345`). A finished lane's files are removed after the finishing transition (`cmd/autocommit.go:286-289`).
- In implement, the CLI knows the task only for single-task runs (`data["task"]`, `internal/steps/implement/steps.go:293-317`). For a whole-plan run, the agent picks the first open task, the same rule `goal.Current` applies (`internal/goal/current.go:9-27`).
- Step instructions are rendered by `stepkit.WriteStepResult` (`internal/stepkit/stepkit.go:103-200`), which appends the orchestrator, commit and footer blocks. Workflow skills render with `{{command}}` only (`internal/agent/skills.go:53`). Library skills render with no data (`cmd/skill.go:61`).
- Harbor's session-log reader (`tests/harbor/shared/sessionlog.py:95-115`) builds an event from every line. Its verifiers detect helper launches from transcripts (`tests/harbor/shared/transcripts.py`). The implement suite runs no plan tasks, and there is no question counter or tool-call metric.

## Per-Task Technical Notes

### Task: Span records and run log reading

**File changes**
- `internal/tracelog/record.go` (created by 000073). Add:
  - constants `RecordSpanStart = "span_start"`, `RecordSpanEnd = "span_end"`;
  - types `TypeStep = "step"`, `TypeTask = "task"`, `TypeSubAgent = "sub-agent"`, `TypeUserWait = "user-wait"`, `TypeWork = "work"`, and `AgentTypes = []string{TypeSubAgent, TypeUserWait, TypeWork}`;
  - end reasons `EndMoved = "moved"`, `EndFinished = "finished"`, `EndStopped = "stopped"`, `EndAbandoned = "abandoned"`, `EndInterrupted = "interrupted"`;
  - attribute keys `AttrEndReason = "spektacular.span.end_reason"`, `AttrTaskID = "spektacular.task.id"`.

  Change the `EndTime` tag to `json:"end_time,omitzero"` and `Status` to `json:"status,omitzero"` (Go 1.25, `go.mod:3`). `span` records always set both, so their output is unchanged. Pin that with a golden.
- `internal/tracelog/spanref.go` (new):
  - `type SpanRef struct{ TraceID, SpanID string }`;
  - `ParseSpanRef(s string) (SpanRef, error)`, which accepts exactly `^[0-9a-f]{32}-[0-9a-f]{16}$` and rejects all-zero parts;
  - `String()`.

  The returned error is plain (`fmt.Errorf`); `cmd` wraps it into an `output` error with a next action.
- `internal/tracelog/reader.go` (new):
  - `type Reader interface{ Records(run Run) ([]Record, error) }`.
  - `fileLogger.Records`:
    - opens `runFilePath(dir, run)` (000073);
    - for a standalone run (`run.Kind == ""`), reads today's and yesterday's `standalone_<day>.jsonl` (UTC) and keeps only records whose `TraceID == run.TraceID`;
    - scans with `bufio.Scanner` and a 16 MB max token size (verbose lines can be large);
    - pre-filters each line by substring (`"record":"span_start"`, `"record":"span_end"`, or `"record":"span"` with a `"parent_span_id"` key) before `json.Unmarshal`;
    - skips unparsable lines;
    - treats a missing file as no records, not an error.
- `internal/tracelog/index.go` (new). `type Index` is built by `NewIndex(recs []Record)`:
  - `starts map[spanID]Record`, `ends map[spanID]Record`, `children map[parentID][]spanID`;
  - `Open(id) bool`, `Find(id) (start Record, open, found bool)`;
  - `OpenUnder(parentID) []Record`: transitive, open only, in start order;
  - `OpenAll() []Record`;
  - `LastCommandUnder(parentID) (time.Time, bool)`: the max `EndTime` of `record == "span"` records whose `ParentSpanID == parentID`, skipping names `"<kind> new"` (`spec new`, `plan new`, `implement new`, `repo new`) and names starting `"trace "`.

  A pure function with no I/O, so it gets the most unit tests.
- `internal/tracelog/spans.go` (new). `type Spans struct{ Log Logger; Run Run; Base map[string]any; Resource map[string]any }`. Base holds the agent and workflow attributes the caller adds to every record.
  - `Start(name, typ, parent string, at time.Time, attrs map[string]any) Record`: mints `NewSpanID()` unless `attrs` carries a preset id (steps and tasks pass the engine's id through a `SpanID` field on an options struct; prefer `StartWithID(id, ...)` for clarity), builds `record: span_start`, `kind: internal`, merges Base and attrs, sets `spektacular.span.type`, writes it, and returns it.
  - `End(start Record, at time.Time, st Status, reason string)`: copies start, sets `Record = span_end`, `EndTime`, `Status`, `attributes[AttrEndReason]`, and writes it.
  - `EndUnder(parentID, at, reason)`: if `Log` implements `Reader`, builds an `Index` and calls `End` on each `OpenUnder(parentID)`, deepest first, with status `{error, reason}` for abandoned or interrupted.
  - `EndAll(at, reason)`: the same over `OpenAll()`.
  - `Find`, `LastCommandUnder`: delegate to the index. Both return not-found when `Log` has no `Reader`.

  Every method contains panics (`defer recover`), like 000073's `Write`.
- `internal/tracelog/*_test.go` (new or extended):
  - golden JSON of a `span_start` (no `end_time`/`status`) and a `span_end` (a full copy plus the end reason), and the existing `span` golden unchanged;
  - `ParseSpanRef` table;
  - `Records` over a temp run file with mixed command, span and garbage lines and a truncated last line;
  - standalone `Records` across two day files filtered by trace;
  - `Index` table: nested open spans, closed parents, `LastCommandUnder` skipping `spec new` and `trace stop`;
  - `Spans.EndUnder` closes a grandchild and a child but not a sibling under another parent;
  - `EndAll`;
  - a logger without `Reader` (a test fake) makes `EndUnder` a no-op.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: 2 parallel agents: (a) record constants, `SpanRef` and `Index`, with tests; (b) the file `Reader` and the `Spans` service, with tests. Integrate sequentially.

### Task: Workflow engine marks every step as a span

**File changes**
- `internal/workflow/state.go:14-21`: add `StepSpan *OpenSpan \`json:"step_span,omitempty"\`` and `TaskSpan *OpenSpan \`json:"task_span,omitempty"\`` to `State`. Add the `OpenSpan` type and `NewSpanID()` (8 random bytes, hex, retried if all zero, with a time-derived fallback, the same as 000073's `NewRunID` style; it stays independent of `tracelog`). Add `func (s *State) RunRef() RunRef` (`RunID`, `Kind`, `Data["name"]`, `CreatedAt`).
- `internal/workflow/spans.go` (new):
  - `RunRef`, `SpanInfo`, `EndReason` constants, and the `SpanSink` interface (Data Structures);
  - `func AbandonRun(statePath string, sink SpanSink)`: `loadState`, then return if not `InProgress()` or `RunID == ""`, otherwise call `sink.RunEnded(ref, now, "abandoned")` (the sink closes the step, task and agent spans);
  - helpers `stepInfo(s *OpenSpan)` and `taskInfo(...)`.
- `internal/workflow/workflow.go:16-48` (`Config`): add `Spans SpanSink` and `StepSpan string`, both documented as runtime only.
- `internal/workflow/workflow.go:98-185` (`New`): keep `w.nextSpan string`.
  - In `Goto` (`:209-223`) and `Next` (`:190-204`), before `w.FSM.Event`, set `w.nextSpan = NewSpanID()`. Set `w.cfg.StepSpan = w.state.RunID + "-" + w.nextSpan` when `RunID != ""` and the event's destination is not the terminal step (`w.steps[len-1].Dst`), otherwise `""`. The `before_` callback (`:148-157`) passes `w.cfg`, so the rendered instruction sees it.
  - In `enter_state` (`:172-181`), before `saveState`, call `w.moveSpans(e.Src, e.Dst, now)`:
    - if `state.StepSpan != nil`: `sink.Ended(ref, stepInfo, now, "moved")`;
    - set `state.StepSpan = &OpenSpan{ID: w.nextSpan, Name: e.Dst, Parent: <task span id when in task>, Start: now}` and call `sink.Started`. The task handling is added by the next task; leave a `w.taskTransition(src, dst, now)` hook that is a no-op here;
    - if `e.Dst` is the terminal step: `sink.Ended(ref, newStep, now, "finished")` and `sink.RunEnded(ref, now, "finished")`, which closes any task and agent spans left open (abandoned, see the sink), then set `state.StepSpan = nil`.
  - Skip every sink call when `cfg.Spans == nil` or `cfg.DryRun`. Ids are still kept in state when there is no sink (dry runs never save, `:178`).
- `internal/workflow/workflow.go:298-317` (`renderStep`): before calling the callback, mint `w.nextSpan` and set `w.cfg.StepSpan` as above. After the callback succeeds:
  - if `state.StepSpan != nil`, call `sink.Interrupted(ref, stepInfo)`;
  - set a fresh `state.StepSpan` (same name, same parent, `Start: now`) and call `sink.Started`;
  - `saveState` unless `DryRun`;
  - leave `CurrentStep`/`CompletedSteps` untouched.

  Update the doc comment and the gotcha note in the comment. The knowledge entry `gotchas/goto-to-current-step-is-a-silent-noop.md` still holds for progress; no change is needed.
- `internal/workflow/workflow.go:323-339` (`commitTerminal`): no change. Terminal span handling lives in `enter_state`.
- `internal/workflow/workflow_test.go` (extend) with `type fakeSink struct{ events []string }`:
  - spec-like 3-step workflow: `New` + `Next` + `Goto(b)` + `Goto(c)` gives `start a`, `end a moved`, `start b`, `end b moved`, `start c`, and the state's `step_span` is `c`'s id;
  - a step whose callback returns the next step reports both;
  - `Goto(current)` gives `interrupted` then `start` with the same name, and `CurrentStep`/`CompletedSteps` are deep-equal before and after;
  - reaching the terminal step gives `start`/`end finished` for it and `RunEnded`, and `step_span` is absent;
  - the callback observes `cfg.StepSpan == runID+"-"+<stored id>`, and `""` on the terminal step;
  - a nil sink still stores ids; `DryRun` reports nothing;
  - `AbandonRun` on in-progress, finished and missing states.

  Use the fresh state's `RunID` from 000073 (set in `New`).

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential: state and types, then the engine hooks, then tests.

### Task: Implement tasks become spans

**File changes**
- `internal/workflow/workflow.go:68-76` (`StepConfig`): add `Task TaskRole`. Add the `TaskRole` constants and `const TaskDataKey = "trace_task"` in `internal/workflow/spans.go`.
- `internal/workflow/workflow.go` (`taskTransition` hook from the previous task). With `role(dst)`:
  - `TaskStart`: if `state.TaskSpan != nil`, `sink.Ended(ref, taskInfo, now, "moved")`. Read `data[TaskDataKey]` (a `map[string]any` with `id` and `title`; fall back to name `"task"`), set `state.TaskSpan = &OpenSpan{ID: NewSpanID(), Name: title, Task: id, Start: now}`, and call `sink.Started` with `Type: "task"`.
  - `TaskWithin`: keep the task span. If none is open (a legacy run resumed mid-task) and the data key is set, open one as above.
  - `TaskNone`: if a task span is open, `sink.Ended(..., "moved")` (or `"finished"` when `dst` is terminal) and set it to nil.

  The new step span's `Parent` is `state.TaskSpan.ID` when `role(dst) != TaskNone` and a task span is open. Order of events: end step, end or start task, start step.
- `internal/workflow/workflow.go:298-317` (`renderStep`): the task span is untouched.
- `internal/steps/implement/steps.go:36-52`: set `Task: workflow.TaskStart` on `analyze`, and `Task: workflow.TaskWithin` on `implement`, `test`, `verify`, `check_goal`, `update_plan` and `update_changelog`.
- `internal/steps/implement/steps.go:99-108` (`analyze`): after `taskExtra`, call a new `currentTask(data, st, cfg) (id, title string, ok bool)` and `data.Set(workflow.TaskDataKey, map[string]any{"id": id, "title": title})` when ok.
  - `currentTask`: a single-task run uses `taskRun` (`:293-317`). Otherwise read the plan and the outcome as `loadGoalView` does (`goalcheck.go:31-60`) and use `goal.Current(p, outcome, "")`.
  - The title is `p.Task(id).Title` for tasks, and for phases the phase's title, or the label when it has none (`plantask.Plan.Phase`, `internal/plantask/plantask.go:410`).
  - A missing plan (dry-run preview) sets nothing.
- `internal/steps/implement/steps_test.go` (extend):
  - a two-task plan fixture (reuse `plantask_fixture_test.go` helpers in `cmd`, or the package's own fixture builders) with a fake sink: entering `analyze` opens a task span named after task 1 with its id, and `implement`..`update_changelog` step spans are parented to it;
  - after ticking task 1 and `update_changelog` → `analyze`, task 1's span ends and task 2's opens;
  - `update_changelog` → `test_plan` ends it, and `test_plan` has no parent;
  - `check_goal` → `implement` keeps the same task span (goal check on);
  - resume at `test` keeps the task span id;
  - `workflow_test.go` covers the generic roles on a synthetic step table.

**Complexity**: Medium
**Token estimate**: ~18k tokens
**Agent strategy**: Single agent, sequential.

### Task: Record step and task spans in every run's log

**File changes**
- `cmd/spans.go` (new):
  - `type spanRecorder struct{ logger tracelog.Logger; agent, version string; level string }` implements `workflow.SpanSink`.
  - `spansFor(run workflow.RunRef) tracelog.Spans` builds a `tracelog.Run{TraceID: run.RunID, Kind, Name, StartedAt: run.StartedAt, Agent}` and a `Base` of `spektacular.agent`, `spektacular.workflow.kind` and `spektacular.workflow.name`.
  - `Started`: `StartWithID(s.ID, s.Name, s.Type, s.Parent, s.Start, {spektacular.step: s.Step, spektacular.task.id: s.TaskID when set})`.
  - `Ended`: first `EndUnder(s.ID, at, abandoned)`, then `End(startRecordFrom(s), at, ok, reason)`. The start record is rebuilt from `SpanInfo`, so it needs no log read for steps and tasks.
  - `Interrupted`: `at := LastCommandUnder(s.ID)` falling back to `s.Start`, then `EndUnder(s.ID, at, interrupted)` and `End(..., {error, "interrupted"}, interrupted)`.
  - `RunEnded`: `EndAll(at, abandoned)` for agent spans, plus the open task and step spans that state names (they may have no start in the log if logging was off at their start; write the end anyway, built from state).

  The `AbandonRun` path passes the state's step and task spans through `RunEnded`'s `SpanInfo` list. Adjust the interface to `RunEnded(run, open []SpanInfo, at, reason)` if that is cleaner, and keep Data Structures in sync.
  - A package var `activeSpans workflow.SpanSink` (nil when logging is off) and `func withSpans(c workflow.Config) workflow.Config { c.Spans = activeSpans; return c }`.
- `cmd/root.go` (`runRoot`, as rewritten by 000073):
  - when the level is not off, build the logger once and set `activeSpans = &spanRecorder{...}` before `rootCmd.ExecuteC()`, and reset it to nil after;
  - when building the command record, set `ParentSpanID`: the before-snapshot's `step_span.id` when the before-state is in progress and belongs to the record's run (same `run_id`), otherwise the after-snapshot's when it belongs to the run, otherwise none;
  - extend 000073's `stateSnapshotFile`/`stateSnapshot` with `StepSpan *struct{ ID string \`json:"id"\` } \`json:"step_span"\``. At verbose, the state attributes may include `step_span` too.
- `cmd/root_test.go:35-43` (`resetRootCmd`): also set `activeSpans = nil` in `reset`.
- Wire `withSpans` into every config: `cmd/spec.go:276`, `cmd/spec.go:385`, `cmd/plan.go:141`, `cmd/plan.go:224`, `cmd/repo.go:253`, `cmd/repo.go:318`, `cmd/implement.go:565` (the shared builder). Also check `cmd/autocommit.go:75` and any `workflow.New(... wfCfg ...)` built from these. The schema-only `workflow.New(..., workflow.Config{}, nil, nil)` calls (`cmd/spec.go:338,404`, `cmd/plan.go:166,243`, `cmd/repo.go:273`, `cmd/implement.go:242,299,540`) stay without spans.
- `cmd/resume.go:216-218` (`clearState`): before `os.Remove`, `if activeSpans != nil { workflow.AbandonRun(statePath, activeSpans) }`. Every restart passes through here (`cmd/spec.go:273,316`, `cmd/plan.go:138`, `cmd/implement.go:197`, `cmd/resume.go:184`). A finished state reports nothing.
- `cmd/log_spans_test.go` (new). Integration tests through `runRoot` in temp projects (helpers from 000073's rewritten `cmd/root_test.go`: `logFiles`, `readRecords`):
  - spec `new`, `goto interview`, `goto overview`: span_start/span_end for `new` and `interview` with `end == next start`, and an open `overview` (a start without an end);
  - each command record's `parent_span_id` equals its step's span id;
  - drive a spec to `finished` (follow `cmd/spec_test.go`'s full-run helper): no step span is open, and `finished` has an instant span;
  - `spec new --force` mid-step gives the old run's step `span_end` with `end_reason: abandoned` and status error;
  - resume: start a run, `goto` a step, append a `span_start` for a helper through `trace start` (or, before the trace task exists, write a span_start record directly with `tracelog`), run a `plan file read`-like command, then the resume `new` (report) and `goto <current>`. The interrupted step ends at that command's end time, not the resume probe's, and the helper ends `interrupted`, followed by a new step span;
  - a two-task implement run (reuse `cmd/plantask_fixture_test.go` and the implement run helpers in `cmd/implement_task_run_test.go`, ticking each task through `plan file write`): two task spans named after the tasks, each parenting `analyze`..`update_changelog`;
  - an orchestrated plan lane: the step spans are in the lane's run file;
  - `log.level: off` writes no files, and the workflow output is byte-identical to standard.

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis (one agent maps every `workflow.Config` construction and `clearState` caller, and the test helpers 000073 left in `cmd`), then sequential integration of the sink, `runRoot` and the tests by one agent.

### Task: Trace start and stop commands

**File changes**
- `cmd/trace.go` (new), a cobra group `trace` registered in `init()` like `cmd/status.go:281`.
  - `trace start <name>` with flags `--type` (default `work`), `--parent`, and `--schema` (the `commandSchema` pattern, `cmd/repo.go:198-210`).
  - `trace stop <id>` with flags `--status` (default `ok`), `--message`, and `--schema`.
- Shared resolution, `resolveTraceRun(ref *tracelog.SpanRef) (statePath string, state *workflow.State)`:
  - with a ref, scan `stateFilePath(dir)`, `workflow.LaneNames(dir, "plan"|"implement")` lane files and `repoStateFilePath(dir)` for an in-progress state whose `RunID == ref.TraceID`;
  - with no ref, `stateFilePath(dir)` when in progress;
  - otherwise none (standalone).

  Build `tracelog.Run` from the state (as `cmd/spans.go`), or `Run{TraceID: ref.TraceID or NewTraceID(), StartedAt: now}` for standalone.
- `start`:
  1. Validate the name (non-empty after trim, no newline, at most 200 runes) → `invalid_span_name`.
  2. Validate the type against `tracelog.AgentTypes` → `invalid_span_type`, with a next action listing the three.
  3. Parse `--parent` → `invalid_span_id`.
  4. If logging is off (the level resolved as in `runRoot`; read `activeSpans == nil` or a shared `logLevel()` helper), return `{span_id: <trace>-<new span>, recorded: false}`.
  5. Resolve the parent. An explicit parent must be the state's open `step_span` or `task_span` id, or an open agent span per `Spans.Find`; otherwise `parent_not_open` ("span <id> is not open: it was never started, has been stopped, or its step has ended"), with a next action to start without `--parent` or with the current step's id from `status`. The default is the state's `step_span.id`, or none when there is no run.
  6. Attributes: `spektacular.step` = the state's current step.
  7. `Spans.Start` and return `{span_id, name, type, parent, recorded: true}`.
- `stop`:
  1. Parse the id → `invalid_span_id`.
  2. Validate the status (`ok|error`) → `invalid_span_status`.
  3. If logging is off → `{recorded: false}`.
  4. Resolve the run by the ref. If the id equals the state's `step_span`/`task_span` id → `span_owned_by_workflow` ("Spektacular closes step and task spans when the workflow moves").
  5. `Spans.Find`: not found → `span_not_open`; found and closed → `span_already_stopped`.
  6. `End(start, now, {code, message}, "stopped")`.

  Every refusal writes nothing and carries `WithNextAction`. Neither command opens a `workflow.Workflow` or writes state.
- `cmd/root.go` (`snapshotStatePath`, `:247-345`): for `kind == "trace"`, parse the id positional (`stop`) or the `--parent` value (`start`, `--parent` and `--parent=`). If it parses and `resolveTraceRun` finds a state, return that path, so the command record lands in the same run file as the span. Otherwise keep the shared fallback. Add `--parent`, `--type`, `--status` and `--message` to the value-taking flags list (`:265-266`), so their values are not read as positionals.
- `cmd/trace_test.go` (new), through `runRoot`:
  - start and stop of `test-writer` sub-agent: a `span_start` with the parent equal to the step span and a `span_end` with `stopped`/`ok`, sharing the id;
  - `--status error --message x` → error status and message;
  - two starts with different explicit parents (two agent spans), and children started with each id: parents are correct;
  - user-wait typing, and no other span typed user-wait in a full run;
  - a span started and never stopped, then `goto` next → `abandoned`; a span in the last step, then `goto finished` → `abandoned`;
  - each refusal code: unknown parent, closed parent (stopped earlier), never-started id, double stop, step span stop, bad type `step`, empty name, malformed id. Each has a non-empty `next_action` and no new span record (count lines before and after);
  - `state.json` bytes are identical before and after start and stop;
  - `log.level: off`: `recorded:false`, exit 0, no log files;
  - an orchestrated plan lane: start with `--parent <lane step span>`, stop it; the span and both command records are in the lane's run file;
  - standalone (no workflow): start and stop share a trace in the day file.
- `cmd/instruction_contract_test.go` (if it pins the command list or help output): add `trace`.

**Complexity**: High
**Token estimate**: ~40k tokens
**Agent strategy**: Parallel analysis (one agent studies `snapshotStatePath` and lane resolution, one the cobra and schema patterns in `cmd/repo.go`/`cmd/status.go`), sequential integration by one agent.

### Task: Trace instructions in every step and skill

**File changes**
- `templates/partials/trace-helpers.md` (new): the helper sentence(s) from the plan's Data Structures. Use `{{command}}` and `{{#trace_parent}} --parent {{trace_parent}}{{/trace_parent}}`, and keep it to about 50 words. Ids are hex, so double-brace escaping is harmless (see `gotchas/mustache-html-escaping.md`).
- `templates/partials/trace-marking.md` (new): a bold lead-in "**Mark helpers and questions.**", then `{{> partials/trace-helpers}}`, then `{{^orchestrated}}` the user-wait sentence `{{/orchestrated}}`, then the stop sentence ("chained with your next command"). Keep the whole rendered block under about 110 words (spec cost steer).
- `internal/stepkit/stepkit.go:139-151`: add `"trace_parent": cfg.StepSpan` to `vars`.
- `internal/stepkit/stepkit.go:153-157`: right after rendering the template body, render `partials/trace-marking.md` with `vars` and append it as `"\n\n---\n\n" + block`, before the orchestrated block (`:157-163`), the commit block (`:166-190`) and the footer (`:191-197`). Add a `traceMarkingPartialPath` constant beside `:59,83,87`. It applies to every step, terminal ones included (they render with no parent, because the engine sets `StepSpan` to `""` there).
- `templates/skills/workflows/{spek-new,spek-plan,spek-implement,spek-plan-epic,spek-implement-epic,spek-design,spek-knowledge,spek-manage-repos}/SKILL.md:6`: add `{{> partials/trace-marking}}` on a line after `{{> partials/version-check}}`, with a blank line between. These render with `{command}` only (`internal/agent/skills.go:53`), so there is no parent and the user-wait rule is kept.
- `templates/skills/workflows/spek-plan-epic/SKILL.md:52-62` and `spek-implement-epic/SKILL.md:64-76` (child prompt lists): add one bullet: "its parent span: the id `trace start` returned for this child".
- `templates/skills/skill_spawn-planning-agents.md` and `templates/skills/skill_spawn-implementation-agents.md`: add `{{> partials/trace-helpers}}` near the top (after the opening paragraph).
- `cmd/skill.go:61`: pass `map[string]any{"command": <configured command, defaulting to "spektacular">}` to `RenderTemplate`. Load the config leniently, as `runRoot` does. Check that `templates/goal_reviewer_brief_test.go:45` still passes (that brief has no `{{`).
- Tests:
  - `cmd/instruction_contract_test.go` (new tests beside `:239-266`):
    - `TestEveryStepCarriesTraceBlockOnce`: over `renderAllStepInstructionsWith(...)` for both `orchestrated` values and every auto-commit mode, `strings.Count(instr, "**Mark helpers and questions.**") == 1`, placed before the `## Running under an orchestrator` heading, the commit block and the footer.
    - With a `cfg.StepSpan` set in the render helper (extend `:177-216` to pass one), the instruction contains `--parent <that id>`.
    - Orchestrated instructions contain `--type sub-agent` and not `--type user-wait`; standalone contain both.
    - `TestNoTemplateFileCarriesTraceBlock`: walk raw `steps/` for the lead-in.
  - `internal/stepkit/stepkit_test.go` (near `:226`): appends the block; an empty `StepSpan` renders no `--parent`.
  - `templates/trace_partial_test.go` (new):
    - every `skills/workflows/*/SKILL.md` contains `{{> partials/trace-marking}}` after `{{> partials/version-check}}`;
    - the two spawn skills include `trace-helpers`;
    - the rendered skill (via `mustache.RenderPartials` with `{command: "spektacular"}`) contains `spektacular trace start` and no `--parent`.
  - `cmd/skill_test.go:59-101`: `skill spawn-planning-agents` output contains `spektacular trace start`.
  - `internal/agent/instruction_surface_test.go:73`: the partials root is already walked; confirm the new partials pass the forbidden-substring checks.
  - Update any existing exact-suffix or exact-content assertions broken by the extra block: `cmd/orchestrated_test.go:76-92` (ordering), `internal/steps/*/orchestrated_test.go`, and `templates/*_test.go` that compare whole rendered skills (`plan_epic_skill_test.go:64`, `implement_epic_skill_test.go:53` require the body to start with version-check, which still holds).
- No task re-installs `.claude/skills` or other installed copies in this repo (plans-never-change-the-active-install).

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) the partials, stepkit and the step contract tests; (b) the skills, library-skill rendering and skill tests. Integrate, then run the full Go suite.

### Task: Harbor reads and checks span records

**File changes**
- `tests/harbor/shared/sessionlog.py:95-115` (`read_events`, with 000073's new-format branch): skip lines whose `record` is `span_start` or `span_end`, before any field access. Only `record == "span"` (or old-format lines) become `Event`s. Add a test in `tests/harbor/shared/tests/test_sessionlog_windows.py`: a lane file with span starts and ends gives identical lanes, windows and `max(e.end)` to the same file without them. `tests/harbor/shared/tests/test_reference_runs.py` pinned values are unchanged.
- `tests/harbor/shared/spans.py` (new):
  - `read_spans(log_dir) -> list[Span]` reads every `*.jsonl` (lanes and `standalone_*`) and pairs `span_start`/`span_end` by `span_id` (a `span_end` alone is a complete span). `Span` has `trace_id, span_id, parent, name, type, start, end, status, end_reason, lane`.
  - Helpers `of_type(spans, t)` and `user_waits(spans)`.
- `tests/harbor/shared/delegation.py:36-56`: add the oracle constants `TRACE_START = "trace start"`, `SUB_AGENT_TYPE = "--type sub-agent"`, `USER_WAIT_TYPE = "--type user-wait"`, and a helper `unmarked_launches(children, spans) -> list` that matches each Claude launch (`transcripts.claude_children`) to a sub-agent span whose start lies between the launch's tool_use time minus 120 s and its start. A rate helper `marked_rate(...)` feeds metrics.
- `tests/harbor/shared/tests/test_delegation.py:796-865`: add `test_trace_oracle_matches_partial`. It reads `templates/partials/trace-helpers.md` and `trace-marking.md` and asserts each oracle constant appears there, the same template tie as `oracle_template_mismatches`.
- Verifiers, inside the transcript-reading `TestChildren` classes only (AST guard `tests/harbor/shared/tests/test_epic_children.py:393-407`):
  - `tests/harbor/epic-workflow/tests/test_epic_workflow.py:188-213` and `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py:307-341`: `test_every_launch_is_a_sub_agent_span`, which reads spans from the collected `.spektacular/logs` (`DEBUG_DIR` after 000073);
  - `tests/harbor/goal-check-workflow/tests/test_goal_check.py:240-310`: the reviewer launch has a sub-agent span.
- `tests/harbor/spec-workflow/tests/test_spec_workflow.py:449-488` (near `TestCrossSectionAmendment`): a new class `TestUserWaits` that requires at least one `user-wait` span whose start falls inside the constraints step window (the scripted rejection follow-up, `tests/harbor/spec-workflow/instruction.md:47-64`). It reads only the session log.
- `tests/harbor/shared/metrics.py:624-674` (`build_record`): add `totals.trace_calls` (Bash tool calls whose command contains `spektacular trace`, from the transcript) and `tasks.trace_calls_per_task` (trace calls divided by the run's task spans, when any). Update `tests/harbor/shared/report.py:190,212,460` to show `trace_calls`. Add tests in `tests/harbor/shared/tests/test_metrics.py`.
- `tests/harbor/shared/tests/test_suite_wiring.py`: if it lists the shared files each suite copies, add `spans.py` (the Makefile copies `tests/harbor/shared/*.py` non-recursively, so no Makefile change is needed).
- Fixtures: `tests/harbor/shared/tests/data/spans/` (new), a small lane file and day file with nested spans for the reader and the verifier helpers.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) the reader skip, `spans.py` and metrics, with unit tests; (b) the oracle, its self-check and the verifier checks. Finish with `uvx --with pytest pytest --import-mode=importlib tests/harbor/shared/tests` and the per-suite self-checks.

### Task: Documentation site describes spans

**File changes**
- `docs:src/pages/debugging.mdx` (as rewritten by 000073): insert the five `##` sections from the plan's content outline inside the existing `<Prose>`, after `## Reading a record` and before `## Upgrading from debug mode`. Use fenced `bash` and `json` blocks only (MDX Rule 1 and Rule 4). Take the sample records from the plan's Data Structures section. Update the frontmatter description to mention spans ("...see what each level records, where logs go, and how spans time a run.").
- `docs:src/pages/configuration.mdx`: in the `log` ConfigKey (added by 000073), one line: "With logging off, `trace start` and `trace stop` succeed and record nothing." This is optional if it reads as clutter; the debugging page is the requirement.
- Do not edit `docs:CHANGELOG.md` history.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential. Verify with `npm run build`, `npx astro check`, `grep -nE "<div|<section|class=" src/pages/*.mdx` (expect none), and an em-dash grep of the changed files.

### Task: README and knowledge describe spans

**File changes**
- `README.md:30-36` ("How It Works", after the `status` block): a short paragraph and a code block:
  ```bash
  spektacular trace start <name> --type sub-agent|user-wait|work [--parent <id>]   # returns a span id
  spektacular trace stop <id> [--status ok|error] [--message <text>]
  ```
  Say that steps, plan tasks and commands are recorded in the run's log automatically, that agents mark helper agents and waits on the user, and link to the logging page (`https://spektacular.dev/debugging/`). No em dashes.
- Knowledge `architecture/testing-architecture.md` (repo store `spektacular`): read with `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`. In the oracle list, add a bullet: "The trace oracle (`TRACE_START`, `SUB_AGENT_TYPE`, `USER_WAIT_TYPE` in `tests/harbor/shared/delegation.py`) mirrors the shared trace partials `templates/partials/trace-helpers.md` and `trace-marking.md`; a self-check ties them, and the epic, implement-epic, goal-check and spec verifiers read span records from the session log." Stage it under `.spektacular/tmp/`, write it with `spektacular knowledge write ... --from <path>`, and remove the scratch file.
- Knowledge `gotchas/goto-to-current-step-is-a-silent-noop.md`: add one paragraph saying that since 000074 such a goto still changes no `current_step` or `completed_steps`, but closes the open step span as interrupted and opens a fresh one (the resume path), so `state.json`'s `step_span` changes. Write it the same way.
- Leave `CHANGELOG.md` alone (the implement workflow's changelog step adds this feature's entry), and leave prior plans alone.

**Complexity**: Low
**Token estimate**: ~6k tokens
**Agent strategy**: Single agent, sequential.

### Requirement to repo and files

| Requirement (spec) | Repo | Files |
|---|---|---|
| Workflow steps recorded without agent action; interrupted steps closed on resume; unclosed spans closed with their parent | spektacular | `internal/workflow/{workflow,state,spans}.go`, `cmd/spans.go`, `cmd/resume.go`, `internal/tracelog/{spans,index,reader}.go` |
| Commands nest under their step | spektacular | `cmd/root.go` |
| Plan tasks recorded without agent action | spektacular | `internal/workflow/workflow.go`, `internal/steps/implement/steps.go` |
| Agents start and stop spans; nest under a chosen parent; span types; user waits typed; trace commands safe; respect log mode | spektacular | `cmd/trace.go`, `cmd/root.go`, `internal/tracelog/{record,spanref,spans}.go` |
| Instructions tell agents what to mark | spektacular | `templates/partials/trace-{marking,helpers}.md`, `internal/stepkit/stepkit.go`, `templates/skills/workflows/*/SKILL.md`, `templates/skills/skill_spawn-*.md`, `cmd/skill.go`, `tests/harbor/shared/*` |
| Documentation describes spans | docs, spektacular | `docs:src/pages/debugging.mdx`, `README.md`, knowledge `architecture/testing-architecture.md` |

## Testing Strategy

Per task:
- **Span records and run log reading**: `tracelog` unit tests cover the start and end record goldens (with the command golden unchanged), `SpanRef` parsing, the file reader (mixed and truncated lines, standalone day files), the pure `Index` (open, closed, descendants, last command skipping `new` and `trace`), and the `Spans` operations, including a logger without a read side.
- **Workflow engine marks every step as a span**: `workflow` unit tests with a recording fake sink cover transitions, chained steps, resume (interrupted plus a fresh span, progress unchanged), the terminal instant span and run end, `cfg.StepSpan` as seen by the callback, no sink, dry run, and `AbandonRun`.
- **Implement tasks become spans**: engine tests for task roles on a synthetic table, and implement step tests on a two-task plan: task span naming, parenting, the loop to the next task, leaving to `test_plan`, a goal-check retry staying inside the task, and resume keeping the task span.
- **Record step and task spans in every run's log**: `cmd` integration tests through `runRoot` cover the spec `new`/`interview`/`overview` spans, command parents, no open span after `finished`, `--force` abandoning, resume interrupting at the last command, a two-task implement run, an orchestrated lane's file, and logging off parity.
- **Trace start and stop commands**: `cmd` integration tests cover the happy path, error status, default and explicit parents, concurrent helpers and their children, user-wait typing, abandonment on step change and on finish, every refusal code with a next action and no record, byte-identical state, logging off, the lane routing of both the span and the command record, and standalone traces.
- **Trace instructions in every step and skill**: contract tests check that the block appears once on every rendered step, its placement, the `--parent` id, the orchestrated variant, that no raw template copies it, that every workflow skill includes the partial, and that the spawn skills render the helper rule with the command name. The instruction-surface walk is also run.
- **Harbor reads and checks span records**: Python unit tests cover the reader skip with pinned reference values unchanged, span pairing, launch-to-span matching, metrics trace counts, and the oracle-to-partial self-check.
- **Documentation site describes spans**: `npm run build`, `npx astro check`, the MDX layout guard and an em-dash grep.
- **README and knowledge describe spans**: a read-back of the knowledge entry through the CLI.

Success metrics and manual reviews (Manual — captured in the implementation test plan):
- step-span coverage of at least 95% in a Harbor implement run;
- at least 90% of helper launches marked in implement, plan-epic and implement-epic runs;
- at least 90% of scripted questions marked in spec runs;
- at most two extra tool calls per plan task, read from the implement-epic and goal-check suites because the implement suite runs no tasks;
- a read-through of the logging page;
- a read of one real epic run's log for parent nesting.

## Project References

- Spec: `000074_trace-spans-for-agent-work` (epic `000002_workflow-tracing-and-run-stats`). Upstream: `000073_log-mode-and-logger-providers` (its plan and context define the record contract, `tracelog`, `run_id` and `runRoot`). Downstream readers: `000075_run-stats-command`, `000076_harbor-on-built-in-logs`, `000077_html-log-viewer`.
- Design documents: none (the spec references none).
- Knowledge entries:
  - conventions `error-messages-must-suggest-remediation`, `plans-never-change-the-active-install`, `store-files-must-be-written-through-the-cli`, `tests-must-not-depend-on-order`, `tests-must-pass-for-done` (repo `spektacular`);
  - `plan-content-pages`, `no-em-dashes`, `mdx-authoring` (repo `docs`);
  - architecture `testing-architecture.md` (updated by this plan) and `workflow-steps.md`;
  - gotchas `goto-to-current-step-is-a-silent-noop.md`, `fsm-cancel-only-works-before-transition-commits.md`, `mustache-html-escaping.md`, `harbor-agent-log-fields.md`.
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.
- Spec source: `docs/workflow-performance-investigation.md` (instruction length and agent turns are the main workflow costs).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Task tiers:
- Medium: span records and reading, the engine step spans, implement task spans, the trace instructions, Harbor.
- High: recording through `cmd`, the trace commands.
- Low: the docs site, README and knowledge.

## Migration Notes

- No settings format change. Workflow state gains two optional fields, `step_span` and `task_span`, written by the engine. An in-progress state from before the upgrade has neither: its next transition simply starts a step span, and a resume finds no step span to interrupt. No `migrate` step is needed, and older binaries ignore the fields.
- The log gains `span_start` and `span_end` records. Readers that look only at `span` records (000073's Harbor reader, after the skip added here) are unaffected.
- This repository's installed skills and `.spektacular/` are not regenerated by any task. Install the new binary with `make install-local` between workflows.

## Performance Considerations

- A step change now adds:
  - one or two extra `O_APPEND` writes (a step end and a start);
  - a read of the run's log file to close open agent spans under the ending step. At the standard level that file is a few hundred short lines. At verbose, lines are pre-filtered by substring before decoding.
- `trace start` with the default parent needs no log read. `trace start` with an agent-span parent and `trace stop` each read the run file once. Neither writes state.
- Instruction length grows by one block of about 110 words per step. That is a deliberate, measured cost (spec Technical Approach), kept to one paragraph. Agent turns grow by about one tool call per helper, since stops are chained with the next command. Both are checked by the manual metrics.
- If the manual timing check shows a step change costing noticeably more than 10 ms at the standard level, STOP and ask before adding an index file (see Open Questions).
