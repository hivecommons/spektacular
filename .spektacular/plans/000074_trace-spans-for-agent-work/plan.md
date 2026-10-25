---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Plan: 000074_trace-spans-for-agent-work

<!-- Metadata -->
<!-- Created: 2026-10-25T08:20:50Z -->
<!-- Commit: 0416513 -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Spektacular records where a run's time goes beyond its commands. The workflow marks each step and each implement task as a timed span by itself. Agents mark the helper agents they start and the questions they put to the user with two new commands, `spektacular trace start` and `spektacular trace stop`, and every workflow instruction tells them when. Time spent waiting on the user is its own span type, so maintainers can later tell the agent's own working time apart from the time a person took to reply. The run statistics, Harbor and log viewer specs of epic 000002 build on these records.

## Conventions

- **Error messages must describe the problem and suggest remediation**: every `trace start`/`trace stop` refusal must be an `output.NewError` with a `WithNextAction` giving the exact runnable command. This covers a bad type, an empty name, a malformed id, an unknown or closed parent, an unknown or already stopped span, and a step or task span the agent may not stop.
- **A plan never changes the active skills and configuration**: the feature changes step templates, workflow skills and library skills. No task re-installs this repo's skills, regenerates managed sections or migrates `.spektacular/`. The changes are verified by Go template-contract tests and throwaway projects, and the maintainer installs the binary between workflows.
- **Store files are written through the CLI**: the knowledge-entry update (testing architecture) goes through `spektacular knowledge write --from`. The logs directory is not a store, so the logger and its new read side reach it directly.
- **Tests must not depend on order**: new `cmd` tests drive `runRoot` and a package-level span sink. They reset through `resetRootCmd`, use their own temp projects, and assert parents, ids and record kinds rather than timestamps.
- **Passing tests are required before calling work done**: the engine change touches every workflow test, and the appended block touches every instruction-contract test. The full shuffled Go suite and Harbor's shared Python tests must pass.
- **Plans must sketch content structure (docs repo)**: the spans sections added to the logging page carry a content outline with the verified command shapes, types and record fields.
- **No em dashes (docs repo)**: all new docs, README, template and changelog prose avoids em dashes.
- **MDX authoring conventions (docs repo)**: the new sections stay inside the page's `<Prose>` with fenced code blocks and no layout HTML.

## Architecture & Design Decisions

**Shape.** A run's log gains spans that open in one command and close in another, written as separate `span_start` and `span_end` records beside 000073's one-per-command `span` records, in the same run file, through the same logger. There are three sources of these spans:
- **The workflow engine marks steps and tasks itself.** `internal/workflow` mints a span id whenever the workflow enters a step and keeps the open step span in the run's state, beside 000073's `run_id`. For implement it also keeps the open task span. On every transition it reports "this span ended, that one started" to an optional `SpanSink` carried on `workflow.Config`, which is runtime-only and never persisted.
- **`cmd` turns those reports into records.** It implements the sink over `internal/tracelog`, using the logger, level and agent that `runRoot` already resolves.
- **Agents mark helper agents and user waits with two new commands.** `spektacular trace start <name> --type <sub-agent|user-wait|work> [--parent <id>]` returns a span id. `spektacular trace stop <id> [--status ok|error] [--message <text>]` closes it.

`runRoot` sets each command record's `parent_span_id` to the step span the command ran in, so a run reads as steps containing their commands. All of this is in the `spektacular` repo (`internal/workflow`, `internal/tracelog`, `internal/steps/implement`, `internal/stepkit`, `cmd`, `templates`, `tests/harbor`). The `docs` repo gets the documentation (`src/pages/debugging.mdx`).

**Key decisions.**
- **The run's own log is the registry of open spans.** No open-span list is kept in state or in a sidecar file, as the spec requires (no span stack in state). That registry is used to:
  - validate a `trace stop` or a named parent;
  - close every span still open under a step when the step ends, marked abandoned, or under an interrupted step, marked interrupted;
  - find the time of an interrupted step's last command.

  The file logger gains an optional read side for this. Trace commands never write workflow state. Concurrent helpers are safe because each record is a single append.
- **The id an agent holds is `<trace_id>-<span_id>`**, after W3C traceparent. A trace command names no spec, so `trace stop <id>` and `--parent <id>` find their run by the trace id: the in-progress state file, shared or lane, whose `run_id` matches. Otherwise the span is a standalone span in that trace. `runRoot`'s state-file choice uses the same lookup for `trace` commands, so the command record and the span land in the same run file. With no `--parent`, the parent is the current step span of the run `runRoot` would pick (`state.json`). Because orchestrated lanes cannot be found that way, each step instruction names its own step span as the parent.
- **The engine's rules.**
  - Entering a step closes the previous step span (`moved`). Ending a step span also closes any agent span still open under it, marked `abandoned`.
  - Implement steps declare a task role: `analyze` starts a task, and `implement` through `update_changelog` run within one. The task is the plan's current open item, resolved the way `implement goal record` resolves it and handed to the engine by `analyze`. A task span is the parent of its step spans. It opens on entering `analyze` and closes when the workflow leaves the task steps or loops back to `analyze`. Steps outside a task have no parent and belong to the run.
  - Reaching the terminal step records it as an instant span and closes every span still open in the run, marked abandoned.
  - A goto to the current step, which is the resume path, closes the open step span as `interrupted`. Its end time is its last recorded command, leaving out resume probes and trace commands, or its own start if there was none. Spans still open under it close the same way. Then a new span opens for the same step. The task span stays open across a resume, so each task keeps one span.
  - `clearState`, the one place a restart discards a run, first closes that run's open spans, marked abandoned.
  - With logging off, the engine still keeps ids in state but nothing is written, and `trace start`/`stop` succeed, return an id and record nothing.
- **One short block tells agents what to mark.** `stepkit.WriteStepResult` appends one shared partial to every step instruction, straight after the template body. It covers:
  - start a `sub-agent` span before each helper launch and put its id in the helper's brief as the parent for the helper's own spans;
  - start a `user-wait` span before each question to the user;
  - stop each one, chained with the next command the agent runs anyway.

  The partial names the current step span as `--parent`. Orchestrated lanes get the helper rule only, because they never ask the user. The same partial is included in every workflow skill after its version check, without a fixed parent, and its helper rule goes into the two spawn library skills. Keeping it to one block and chaining the calls follows the spec's cost steer: a start for each helper and a stop chained onto the next command, so about one extra tool call per helper.

**Why this beats the alternatives** (see `research.md#alternatives-considered-and-rejected`). If `runRoot` only compared the before and after states, it would miss steps passed through inside one command, such as implement's `new` → `read_plan`. It also could not put the new step's span in the instruction that command returns. Keeping open spans in state would contradict the spec and break the "trace commands never change state" rule, and concurrent helpers would race on `state.json`. A bare span id cannot reach an orchestrated lane's run, or keep a standalone span's start and stop in one trace.

**Conventions that drive specific choices.**
- Every new trace error is an `output.NewError` with a next action naming the exact command to run (error-messages-must-suggest-remediation).
- No task re-installs skills or migrates this repo's `.spektacular/`. Template changes are verified with Go tests and throwaway projects (plans-never-change-the-active-install).
- `cmd` tests reset through `resetRootCmd` and the package-level span sink is cleared with it. Tests compare ids and parents, never wall-clock values (tests-must-not-depend-on-order).

## Component Breakdown

- **Workflow engine span tracking (changed, `spektacular` repo).** The engine owns step and task span identity for a run.
  - It mints a span id on every step entry and keeps the open step span (id, step, start, parent) in the persisted state beside 000073's run id. For implement it keeps the open task span (id, task id, title, start) there too.
  - It decides when spans begin and end: on a transition, on the resume path (a goto to the current step), and on reaching the terminal step.
  - It reports each begin and end to an optional span sink it receives in its runtime configuration.
  - It exposes the current step's span id to the step callback that renders the instruction, so the instruction can name it.
  - Steps can declare a task role (starts a task, or runs within one), and a starting step's callback hands the engine the task it is about to work on.

  The engine stays free of any logging dependency: with no sink it only keeps ids in state, and in a dry run it does nothing.
- **Implement steps (changed, `spektacular` repo).** These declare the task roles: `analyze` starts a task; `implement`, `test`, `verify`, `check_goal`, `update_plan` and `update_changelog` run within one. `analyze` resolves the current task (the single-task run's task, or the plan's first open, unskipped task or phase, the same rule `implement goal record` uses) and hands its id and title to the engine. Nothing the agent sees changes.
- **Trace log package (changed, `spektacular` repo; created by 000073).**
  - Adds the `span_start` and `span_end` record kinds, the span types (`step`, `task`, `sub-agent`, `user-wait`, `work`), the end-reason attribute and the task attributes.
  - Adds the composite span id an agent holds (`<trace_id>-<span_id>`), with parsing and validation.
  - Adds a small span service over a logger for one run: start a span, end a span together with every open span under it, end every open span in a run, find an open span, and find a step's last command.
  - Gives the file logger an optional read side that returns a run's span and command records (or a standalone trace's, from the recent day files), filtering lines before decoding.

  It stays a leaf package with no knowledge of configuration or workflows.
- **Trace commands (new, `spektacular` repo).** A `trace` command group with `start` and `stop`.
  - `start` takes a name, a type and an optional parent, resolves the run (by the parent's trace, otherwise the run in the shared state), validates the parent, writes a `span_start` and returns the composite id.
  - `stop` takes an id and an optional status and message, finds the span's run by its trace, refuses an unknown, already stopped, step or task span, and writes a complete `span_end`.

  Neither ever writes workflow state. With logging off both succeed, return an id and write nothing. Every refusal names the problem and the next command.
- **Command recorder and span sink in `cmd` (changed, `spektacular` repo).**
  - `runRoot` (from 000073) builds a span sink for the command when logging is on and gives it to every workflow the command constructs.
  - It sets each command record's parent to the step span the command ran in.
  - It routes `trace` commands' records to the run their id names.

  The restart path that clears an old state first closes that run's open spans as abandoned. The sink adapts the engine's begin and end reports into trace log records for the run.
- **Step instruction renderer (changed, `spektacular` repo).** Appends the shared trace block to every step instruction, straight after the template body and before the orchestrator hand-back, commit and working-context blocks. It renders the block with the current step's span id as the parent. Orchestrated lanes get the helper-launch rule only.
- **Shared trace partial and skills (new partial, changed skills, `spektacular` repo).** One partial holds the wording: mark each helper launch as a `sub-agent` span and pass its id to the helper as the parent for the helper's own spans; mark each question to the user as a `user-wait` span; stop each one chained with the next command. Every workflow skill (spec, plan, implement, plan-epic, implement-epic, design, knowledge, manage-repos) includes it after its version check, without a fixed parent. The two spawn library skills include its helper rule, which needs the command name passed to library-skill rendering.
- **Harbor harness (changed, `spektacular` repo).**
  - The shared session-log reader treats only `span` records as command events and skips `span_start` and `span_end`, so step windows and timelines do not move.
  - A new shared span reader gives the verifiers sub-agent and user-wait spans per run.
  - The plan-epic, implement-epic and goal-check verifiers check, in their transcript-reading `TestChildren` classes, that each helper launch has a sub-agent span. The spec verifier checks that its scripted question was marked as a user-wait span.
  - The metrics record counts trace calls, overall and per plan task.
  - A local self-check ties the oracle's trace command wording to the shared partial, the way the delegation oracle is tied to its templates.
- **Documentation (changed, `docs` and `spektacular` repos).** The site's logging page (rewritten by 000073) gains sections on spans: the types, which spans Spektacular records itself, the start and stop commands, and the record kinds. The README lists the `trace` commands. The testing-architecture knowledge entry names the new trace oracle coupling.

## Data Structures & Interfaces

**Span records (log line format v1, extended).** This plan adds two record kinds next to 000073's `span` record. It changes no existing field. Both new kinds go in the run's file, or the day's standalone file, and use the same `trace_id` and `resource`. Records are shown on several lines here; each is one JSON line in the log.

A step span opening (`span_start` omits `end_time` and `status`):

```json
{"record":"span_start",
 "trace_id":"4bf92f3577b34da6a3ce929d0e0e4736",
 "span_id":"a3ce929d0e0e4736",
 "name":"interview",
 "kind":"internal",
 "start_time":"2026-10-24T09:03:34.930000000Z",
 "attributes":{
   "spektacular.span.type":"step",
   "spektacular.agent":"claude",
   "spektacular.workflow.kind":"spec",
   "spektacular.workflow.name":"000074_trace-spans",
   "spektacular.step":"interview"},
 "resource":{"service.name":"spektacular","service.version":"0.22.0"}}
```

The same span closing. A `span_end` repeats every field of its start and adds the end time, status and end reason, so it is a complete span on its own:

```json
{"record":"span_end",
 "trace_id":"4bf92f3577b34da6a3ce929d0e0e4736",
 "span_id":"a3ce929d0e0e4736",
 "name":"interview",
 "kind":"internal",
 "start_time":"2026-10-24T09:03:34.930000000Z",
 "end_time":"2026-10-24T09:11:02.417000000Z",
 "status":{"code":"ok"},
 "attributes":{
   "spektacular.span.type":"step",
   "spektacular.span.end_reason":"moved",
   "...":"as in the start"},
 "resource":{"...":"as in the start"}}
```

- **Span types** (`spektacular.span.type`):
  - `command`: 000073, one `span` record per command;
  - `step`: written by the workflow;
  - `task`: written by the workflow, implement only;
  - `sub-agent`, `user-wait` and `work`: started by agents.
- **End reasons** (`spektacular.span.end_reason`, on `span_end` only) and the status each gets:

  | Reason | When | Status |
  |---|---|---|
  | `moved` | a step or task left normally | `ok` |
  | `finished` | the terminal step's instant span, and the step or task closed on reaching it | `ok` |
  | `stopped` | an agent's `trace stop` | `ok`, or `error` with the agent's message |
  | `abandoned` | a span still open when its parent ended, its run finished, or its run was restarted | `{"code":"error","message":"abandoned"}` |
  | `interrupted` | a step left open by a stopped session, closed on resume, and the spans open under it | `{"code":"error","message":"interrupted"}` |

- **Parents.**
  - A step span's `parent_span_id` is the open task span, or absent: the step belongs to the run.
  - A task span has no parent.
  - An agent span's parent is the span it names, or the current step span by default.
  - Every 000073 `command` record gains `parent_span_id`: the step span it ran in.
- **Task spans.** The span is named by the task's title and carries `spektacular.task.id`, the task id or, in an older plan, the phase label. It also carries `spektacular.step` of the step that opened it (`analyze`).
- **Agent spans.** They carry the workflow attributes of their run, when they have one, and `spektacular.step` of the step that was current when they started.

**Span id held by agents.** This is `<trace_id>-<span_id>`: 32 and 16 lowercase hex characters joined by a hyphen, 49 characters in all, for example `4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7`. It is returned by `trace start` and accepted by `trace stop` and `--parent`. Step instructions name the current step span in this form.

**Trace commands.**

```text
spektacular trace start <name> [--type sub-agent|user-wait|work] [--parent <id>]
  -> {"span_id":"<trace>-<span>","name":"test-writer","type":"sub-agent",
      "parent":"<trace>-<parent span>","recorded":true}

spektacular trace stop <id> [--status ok|error] [--message <text>]
  -> {"span_id":"<trace>-<span>","status":"ok","recorded":true}
```

- `--type` defaults to `work`.
- `recorded` is `false` when logging is off. The command still succeeds and still returns an id.
- Refusals are `output` errors, each with a next action:
  - `invalid_span_name`: an empty name or more than one line;
  - `invalid_span_type`, including `step` and `task`;
  - `invalid_span_id`: a malformed id or parent;
  - `invalid_span_status`;
  - `parent_not_open`: unknown, or already closed;
  - `span_not_open`: never started, or its run has finished;
  - `span_already_stopped`;
  - `span_owned_by_workflow`: a step or task span.

  A refused command writes no span record.

**Workflow state (`state.json` and lane files).** Two optional top-level fields go beside 000073's `run_id`:

```json
"step_span": {"id":"a3ce929d0e0e4736","name":"interview","parent":"","start":"2026-10-24T09:03:34.93Z"},
"task_span": {"id":"9d0e0e4736a3ce92","name":"Trace log package","task":"df1bade8-...","start":"..."}
```

```go
// internal/workflow
type OpenSpan struct {
    ID     string    `json:"id"`               // 16 hex
    Name   string    `json:"name"`             // step name, or task title
    Task   string    `json:"task,omitempty"`   // task id (task spans)
    Parent string    `json:"parent,omitempty"` // task span id (step spans inside a task)
    Start  time.Time `json:"start"`
}
// State gains:
//   StepSpan *OpenSpan `json:"step_span,omitempty"`
//   TaskSpan *OpenSpan `json:"task_span,omitempty"`

type TaskRole int // TaskNone (default) | TaskStart | TaskWithin
// StepConfig gains:  Task TaskRole
const TaskDataKey = "trace_task" // {"id","title"}: set by a TaskStart step's callback

type RunRef struct { RunID, Kind, Name string; StartedAt time.Time }
type SpanInfo struct {
    ID, Parent, Name, Type, Step, TaskID string // Type "step" | "task"
    Start time.Time
}
type EndReason string // "moved" | "finished" | "interrupted" | "abandoned"

// SpanSink receives the engine's span events. Ending a span also ends every
// agent span still open under it, with the reason abandoned (or interrupted).
type SpanSink interface {
    Started(run RunRef, s SpanInfo)
    Ended(run RunRef, s SpanInfo, at time.Time, reason EndReason)
    Interrupted(run RunRef, s SpanInfo) // ends at the last activity under s, or s.Start
    RunEnded(run RunRef, at time.Time, reason EndReason) // ends every open span
}
// Config gains (runtime only, never persisted):
//   Spans    SpanSink // nil: ids are kept in state, nothing is reported
//   StepSpan string   // composite id of the step span being rendered; "" on the terminal step
func NewSpanID() string                              // 16 hex, crypto/rand
func AbandonRun(statePath string, sink SpanSink)     // used by restart before the state is cleared
```

```go
// internal/tracelog (additions)
const RecordSpanStart, RecordSpanEnd = "span_start", "span_end"
const TypeStep, TypeTask, TypeSubAgent, TypeUserWait, TypeWork = "step", "task", "sub-agent", "user-wait", "work"
const AttrEndReason, AttrTaskID = "spektacular.span.end_reason", "spektacular.task.id"
// Record: EndTime and Status gain `omitzero` (unchanged output for "span" records).

type SpanRef struct{ TraceID, SpanID string }
func ParseSpanRef(s string) (SpanRef, error)
func (r SpanRef) String() string // "<trace>-<span>"

// Reader is the optional read side of a Logger; the file logger implements it.
type Reader interface { Records(run Run) ([]Record, error) } // span* records only, in file order

// Spans writes and queries the spans of one run through a logger.
type Spans struct { Log Logger; Run Run; Base map[string]any; Resource map[string]any }
func (s Spans) Start(name, typ, parent string, at time.Time, attrs map[string]any) Record
func (s Spans) End(start Record, at time.Time, status Status, reason string)
func (s Spans) EndUnder(parentID string, at time.Time, reason string) // open descendants
func (s Spans) EndAll(at time.Time, reason string)
func (s Spans) Find(spanID string) (start Record, open bool, found bool)
func (s Spans) LastCommandUnder(parentID string) (time.Time, bool) // skips "<kind> new" and "trace" commands
```

**Shared trace partial (instruction text).** This is one partial, `partials/trace-marking.md`, built from `partials/trace-helpers.md` and a user-wait line. It renders with `command`, an optional `trace_parent` and `orchestrated`. Illustrative rendering for a non-orchestrated step:

```text
**Mark helpers and questions.** Before you launch a helper agent, run
`spektacular trace start <helper> --type sub-agent --parent <step span id>` and put the id it
returns in the helper's brief: "Your parent span is <id>: start any span of your own with
`--parent <id>`." Before you ask the user a question, run
`spektacular trace start <question> --type user-wait --parent <step span id>`. When the helper
returns or the user answers, run `spektacular trace stop <id>` (add `--status error` if the
helper failed), chained with your next command.
```

## Implementation Detail

**The engine gains a span lifecycle beside its step lifecycle.** A reader of the workflow engine sees one new concern next to persistence: every transition now also ends one step span and starts the next. The existing hooks carry it, so there is no new control flow.
- Before a step's event fires, the engine mints the coming step's span id and puts it on the runtime config, so the `before_` callback that renders the instruction can name it.
- `enter_state`, which already marks the source step complete and saves state, now also:
  - ends the open step span (and the task span, when the task scope ends or restarts);
  - opens the task span when a task starts;
  - opens the new step span;
  - stores both in state;
  - reports each change to the sink.
- The resume path (`renderStep`) gains the interrupted close and a fresh step span, and saves state. It still leaves `current_step` and `completed_steps` untouched, so the "goto to the current step is a no-op" contract holds for the workflow's progress.
- The terminal-step bookkeeping (`commitTerminal`) records the instant terminal span and ends the run's open spans.

Callbacks keep the narrow `StepCallback` signature. The only domain coupling is the declarative task role on `StepConfig` and one well-known data key, which the task-starting step sets. That follows the existing pattern where callbacks hand values to the engine through `Data`.

**A sink interface at the edge, mirroring 000073's logger boundary.** `internal/workflow` defines what it reports (`SpanSink`). `cmd` decides how it is written, and `internal/tracelog` decides the record shape. That is the same layering 000073 gives `runRoot` and the `Logger`: the leaf packages know nothing of each other.
- `cmd` builds one sink per command when logging is on, as part of the `runRoot` pipeline 000073 introduces.
- It hands that sink to every `workflow.Config` it constructs through one small helper, so no construction site carries logging logic.
- The package-level sink is reset with the command tree, so tests stay order-independent.

**The log becomes readable by its writer.** Until now the file logger only appended. It gains a read side behind an optional interface. Only the span logic uses it, and a future logger that cannot read back still works: its spans are written, but unclosed agent spans are not swept and trace stops are not validated. The read is one file per run (or the recent day files for a standalone trace). Lines are pre-filtered by substring before JSON decoding, so a verbose log's large responses are skipped cheaply. From those records, a small pure index (open spans, closed spans, parent links, last command per parent) answers every question the span logic asks. That index is unit-tested on its own, separately from the files.

**Agent commands follow the CLI's existing shape.** `trace` is a new cobra group with the conventions every other command group already follows:
- positional name or id, and flags for options;
- a JSON result through the shared output writer;
- `output.NewError` refusals with next actions;
- a `--schema` description.

Its run resolution is shared with `runRoot`'s state-file choice: one helper maps a span id's trace to the matching in-progress state file. So a trace command's own record and the span it writes always land in the same run file.

**Instructions get a fourth appended block.** The step renderer already appends the orchestrator hand-back, the commit block and the working-context footer. The trace block comes first, straight after the template body, because it is part of what the step asks the agent to do, not how the step ends. It is a partial, so the identical wording reaches step instructions and installed skills, and it is pinned the same way the footer is: one contract test over every rendered step, plus a raw-template test that no template copies it. The library-skill renderer starts passing the command name, so shared partials render the same everywhere.

**Harbor follows the existing oracle pattern.**
- The span check is a hand-kept oracle in the shared Python helpers, tied to the partial's wording by a local self-check, as the delegation oracle is tied to its templates.
- The session-log reader stays a normalising reader: it keeps treating only command records as events, so every existing window, timeline and pinned reference value is unchanged.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports none).
- **Upstream spec `000073_log-mode-and-logger-providers` must land first.** It is planned but not implemented. This plan builds on its `internal/tracelog` package (`Record`, `Status`, `Run`, `Logger`, the provider registry, the file logger and its run-file naming), on `workflow.State.RunID` with `NewRunID`/`EnsureRunID`, on its rewritten `runRoot` (level resolution, before and after snapshots, run attribution, one `span` record per command), and on `SPEKTACULAR_SESSION_LOG`'s `off|standard|verbose`. It also relies on 000073's Harbor normalising reader and its `.spektacular/logs` paths. No task here starts until 000073's Milestone 1 is merged, and the Harbor task needs its Milestone 3. If 000073 lands with different names, adapt to them. If its record fields differ, STOP and ask.
- **Downstream specs that read what this plan records:** `000075_run-stats-command` (step, task, sub-agent and user-wait spans, end reasons, and user-wait time kept apart from working time), `000076_harbor-on-built-in-logs` (replaces this plan's Harbor span reader and checks), and `000077_html-log-viewer` (draws the spans). The record kinds, span types, end reasons and the agent-visible id in Data Structures are their contract and must not drift during implementation.
- **`internal/workflow` (changed).** Span tracking in the engine, `OpenSpan`, `TaskRole`, `SpanSink`, `NewSpanID`, `AbandonRun`, and the `Config.Spans`/`Config.StepSpan` runtime fields.
- **`internal/tracelog` (changed, created by 000073).** New record kinds, types and attributes, `SpanRef`, the `Reader` side of the file logger, and the `Spans` service. The `omitzero` tags go on `EndTime`/`Status`, which leaves `span` records unchanged.
- **`internal/steps/implement` (changed).** The task roles on its step table, and `analyze` handing the current task to the engine. It uses `internal/goal`'s current-item rule unchanged.
- **`internal/stepkit` (changed).** The appended trace block and its `trace_parent` variable.
- **`cmd` (changed).** The new `trace` command group, the sink and its wiring into every `workflow.Config`, `parent_span_id` on command records, trace-id routing in the state-file choice, abandoning on `clearState`, and library skills rendered with the command name.
- **`templates` (changed).** New partials `trace-marking.md` and `trace-helpers.md`, included by every workflow skill and the two spawn library skills.
- **`internal/output` (used, unchanged).** `NewError(...).WithNextAction` for every trace refusal, and the shared result writer.
- **Go standard library only** (`crypto/rand`, `encoding/hex`, `encoding/json`, `bufio`, `os`), plus the existing `cobra` and `cbroglie/mustache`. No new module.
- **Harbor test harness (changed, Python).** `tests/harbor/shared/sessionlog.py`, a new shared span reader, `delegation.py`'s oracle and its self-check, `metrics.py`, and the spec, plan-epic, implement-epic and goal-check verifiers. Its unit tests run under pytest. Its suites need Docker, the Harbor CLI and agent logins, and are not in CI.
- **Documentation site, `docs` repo (changed).** The logging page `src/pages/debugging.mdx` as 000073 rewrites it. It is checked by the site build and `astro check`.
- **Knowledge entries (changed through the CLI):** `architecture/testing-architecture.md` in the `spektacular` repo store gains the trace oracle coupling, and `gotchas/goto-to-current-step-is-a-silent-noop.md` notes the fresh step span a resume goto opens.

## Testing Approach

**Test types and where they slot in.** All Go tests run under the project's shuffled, order-independent run. `cmd` tests go through `resetRootCmd`, which also clears the package-level span sink, and use their own temp projects. Assertions compare record kinds, span ids, parents, types, names and end reasons, never wall-clock values. The only time assertions are orderings (an end not before its start; one step's end equal to the next step's start).

- **Unit tests in `internal/workflow`**, the heart of the feature. A recording fake sink drives:
  - every transition: one end and one start, in order, with the new step's id stored in state and available to the rendering callback;
  - chained transitions in one command (implement `new` → `read_plan`) report each intermediate step;
  - task roles: entering `analyze` opens a task span named from the handed-over task; within-task steps are parented to it; the loop back to `analyze` ends one task and starts the next; leaving to `test_plan` or `finished` ends it;
  - a goto to the current step reports an interrupted end and a fresh start, keeps `current_step`/`completed_steps`, and keeps the task span;
  - reaching the terminal step reports an instant terminal span and a run end;
  - no sink still keeps ids in state, and a dry run reports and saves nothing;
  - `AbandonRun` reports a run end for an in-progress state and nothing for a finished or missing one.
- **Unit tests in `internal/tracelog`**:
  - `span_start`/`span_end` JSON shapes, with `end_time` and `status` omitted on starts and the existing `span` record output byte-identical;
  - `SpanRef` parsing (valid, wrong lengths, uppercase, missing hyphen);
  - the read side (a run file; recent day files for standalone traces; a partially written last line and non-span lines are skipped);
  - the span index (open, closed, descendants, last command under a parent skipping `new` and `trace` commands);
  - `Spans` operations (an end repeats the start; `EndUnder` closes descendants transitively and nothing else; `EndAll`).
- **Command-level integration tests in `cmd`**, which carry most acceptance criteria end to end through `runRoot`:
  - a spec run through `new`, `interview` and `overview` gives closed spans for `new` and `interview`, each ending when the next began, and an open `overview`;
  - `finished` leaves no open step span;
  - `spec new --force` mid-step abandons the old run's step span;
  - every command record carries its step span as parent;
  - an interrupted session with an open helper span is closed on resume as described;
  - a two-task plan's implement run gives two task spans, each named and parenting its steps, with no trace command run;
  - `trace start`/`stop` happy path, error status, default parent, explicit parents for two concurrent helpers and their own children;
  - user-wait typing;
  - spans abandoned on step change and on finish;
  - every refusal (bad parent, unknown or double stop, step span stop, bad type, bad id) returns its error code with a next action and writes no span record;
  - trace commands leave `current_step` and `completed_steps` byte-identical;
  - logging off: both commands succeed and nothing is written;
  - an orchestrated lane's spans and trace commands land in the lane's run file through the composite id.
- **Template-contract tests** (Go):
  - every rendered step instruction carries the trace block exactly once, before the orchestrator, commit and footer blocks, naming the current step span as parent (and none on the terminal step);
  - orchestrated instructions carry the helper rule but not the user-wait rule;
  - no raw template copies the block;
  - every workflow skill includes the trace partial after its version check;
  - the two spawn library skills include the helper rule and render the command name;
  - the instruction-surface walk covers the new partials.
- **Harbor Python unit tests**:
  - the session-log reader ignores `span_start`/`span_end` lines, with existing windows, timelines and pinned reference values unchanged;
  - the span reader pairs starts and ends across lane and standalone files;
  - the new verifier helpers match launches to sub-agent spans, against fixtures;
  - the metrics record counts trace calls per task;
  - the oracle self-check ties the trace wording to the partial.
- **Docs repo.** The site build and `astro check` pass, and the MDX guard returns nothing.

**Load-bearing guarantees.**
- Steps and tasks are recorded with no agent action.
- Every command and agent span nests under the right parent.
- No span of a run is left open once the run finishes or restarts.
- Trace commands never change workflow state and never write a record when they refuse.
- Logging off records nothing and fails nothing.
- Every workflow instruction and skill tells the agent what to mark, in one identical wording.

**Deliberate gaps.**
- No automated Harbor suite run: the suites need Docker and paid agent logins and are not in CI.
- No automated check that real agents follow the instructions. That is measured by the success metrics below.
- The resume test simulates a stopped session by running commands in sequence with a deliberately unstopped span, since a real session kill is not reproducible in Go.

**Success metrics.**
- *In a Harbor implement run, the step spans together cover at least 95% of the time between the run's first and last command.* **Manual — captured in the implementation test plan.** Supporting behavioural test: in `cmd`, the step spans of a whole spec run and a whole implement run are contiguous (each ends exactly when the next starts) from the first command to `finished`, so the only uncovered time is before the first step.
- *In Harbor implement, plan-epic and implement-epic runs, at least 90% of helper-agent launches appear as sub-agent spans.* **Manual — captured in the implementation test plan.** Supporting checks: the plan-epic, implement-epic and goal-check verifiers compare Claude launches with sub-agent spans per run and report the rate.
- *In Harbor spec runs, at least 90% of questions put to the scripted user appear as user-wait spans.* **Manual — captured in the implementation test plan.** Supporting check: the spec verifier requires a user-wait span around the scripted rejection follow-up.
- *Marking spans adds on average no more than two agent tool calls per plan task in the Harbor implement suite.* **Manual — captured in the implementation test plan.** The metrics record gains a trace-call count overall and per plan task. Because the implement suite starts at `test_plan` and runs no plan tasks, the per-task average is read from the implement-epic and goal-check suites, whose children implement real tasks, alongside the implement suite's own total.

**Manual reviews.**
- **Manual — captured in the implementation test plan:** a read-through of the spans sections of the logging page on the built site, checking the types, the automatically recorded spans and both commands against the shipped behaviour.
- **Manual — captured in the implementation test plan:** a read of one real run's log (plan-epic or implement-epic), checking that the sub-agent spans nest under the step that launched them and that a child's own spans nest under the id it was given.

## Milestones & Tasks

### Milestone 1: Every run's log shows its steps and tasks

**What changes**: A run's log now shows where the time went between commands, with nothing new asked of the agent.
- Each workflow step appears as a timed span that starts when the workflow enters the step and ends when it leaves.
- Every command sent during a step is nested under it.
- When implementing a plan, each task appears as its own span, with the steps of that task's work inside it.
- A finished run leaves no step open.
- Restarting a run with `--force` closes the old run's open step, marked abandoned.
- Resuming a run after its session stopped closes the interrupted step at its last command, marked interrupted, and starts the step afresh.

**Validation point**: Integration tests show the step spans of a spec run through `new`, `interview` and `overview`, with commands parented to their steps. They also show two task spans parenting their steps in a two-task implement run, an abandoned step after `--force`, an interrupted step after a resume, and no open span after `finished`. The full Go suite passes.

#### - [x] Task: Span records and run log reading
**Id:** b436d238-2219-447a-a109-8ee0d495e300
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Extends the trace log package from spec 000073 with spans that open in one command and close in another. It adds the `span_start` and `span_end` record kinds, the span types and end reasons, and the span id agents hold (`<trace_id>-<span_id>`). The file logger gains a read side that returns a run's span and command records. A small span service on top of it starts and ends spans, closes every open span under a parent or in a whole run, and finds a step's last command. Nothing calls it yet. This task starts only once 000073's trace log package is merged.

*Technical detail:* [context.md#task-span-records-and-run-log-reading](./context.md#task-span-records-and-run-log-reading)

**Acceptance criteria**:
- [x] A span start serialises without an end time or status, a span end repeats every field of its start and adds the end time, status and end reason, and a command record serialises exactly as before.
- [x] A span id in the form `<32 hex>-<16 hex>` is accepted and split into its trace and span ids. Anything else is rejected with a reason.
- [x] Reading a run's log returns its span and command records in file order, and skips a partly written last line and malformed lines without failing.
- [x] Ending a span also closes every span still open beneath it, at any depth, and leaves spans elsewhere in the run alone. Ending a run closes every open span in it.
- [x] A step's last command is found among the commands nested under it, ignoring workflow-start commands and trace commands.

#### - [x] Task: Workflow engine marks every step as a span
**Id:** 70be742b-f019-422a-aea6-ece15b1520ba
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Teaches the workflow engine to give every step it enters a span id and to keep the open step span in the run's state, beside 000073's run identifier. Each time the workflow moves, finishes or is resumed, it reports which span ended and which began to an optional receiver, so a later task can write them to the log. The new step's id is also available to the step that renders the instruction. A resume (a goto to the current step) closes the old step span as interrupted and opens a new one, without changing the workflow's progress. A restart reports the old run's spans as abandoned. This task starts only once 000073's run identifier is merged.

*Technical detail:* [context.md#task-workflow-engine-marks-every-step-as-a-span](./context.md#task-workflow-engine-marks-every-step-as-a-span)

**Acceptance criteria**:
- [x] Every move from one step to the next ends the old step span and starts a new one, in that order, and the run's state names the new open span.
- [x] A command that passes through several steps at once reports a span for each step it passed through.
- [x] Going to the current step reports the open step span as interrupted and starts a fresh one for the same step, while the current step and completed steps stay exactly as they were.
- [x] Reaching the last step records it as an instant span and reports the end of the run, leaving no step span open.
- [x] With no receiver, span ids are still kept in state and nothing else changes. A dry run reports nothing and saves nothing.
- [x] Abandoning an in-progress run reports its open spans as abandoned. Abandoning a finished or missing run reports nothing.

#### - [x] Task: Implement tasks become spans
**Id:** c6327f6b-a3b6-45f8-b5ec-b9ff5a81ac77
**Repo:** spektacular
**Depends on:**
- 70be742b-f019-422a-aea6-ece15b1520ba — Workflow engine marks every step as a span
**Execution:** agent

Lets a workflow declare which steps make up the work on one task. For implement, a task starts at `analyze` and runs through `update_changelog`. The engine then keeps a task span open over those steps and nests their step spans under it. `analyze` names the task: the single-task run's task, or the plan's first open task that the goal check has not skipped, the same rule the goal check uses. Looping back for the next task ends one task span and starts the next. The agent does nothing new.

*Technical detail:* [context.md#task-implement-tasks-become-spans](./context.md#task-implement-tasks-become-spans)

**Acceptance criteria**:
- [x] Entering `analyze` opens a task span named after the current task and carrying its id, and every step span through `update_changelog` is nested under it.
- [x] Looping back to `analyze` ends the task span and opens one for the next task. Moving on to the test plan or finishing ends it.
- [x] A goal-check retry that returns to `implement` stays inside the same task span.
- [x] Steps outside a task, such as `read_plan` and `test_plan`, have no parent span.
- [x] Resuming a step inside a task keeps the same task span open.

#### - [x] Task: Record step and task spans in every run's log
**Id:** 0f6d5548-9fc2-4ec1-9c7a-3d2ffdd18e82
**Repo:** spektacular
**Depends on:**
- b436d238-2219-447a-a109-8ee0d495e300 — Span records and run log reading
- 70be742b-f019-422a-aea6-ece15b1520ba — Workflow engine marks every step as a span
- c6327f6b-a3b6-45f8-b5ec-b9ff5a81ac77 — Implement tasks become spans
**Execution:** agent

Connects the engine's span reports to the log, so every real run records its steps and tasks. When logging is on, each command builds a receiver that writes the reported spans through the configured logger to the run's log, and hands it to every workflow the command runs. Each command record names the step span it ran in as its parent. Restarting a run with `--force` first closes that run's open spans as abandoned. Logging off writes nothing, as before.

*Technical detail:* [context.md#task-record-step-and-task-spans-in-every-runs-log](./context.md#task-record-step-and-task-spans-in-every-runs-log)

**Acceptance criteria**:
- [x] After a spec workflow moves through `new`, `interview` and `overview`, its log holds closed step spans for `new` and `interview`, each ending when the next began, and an open span for `overview`.
- [x] Every command recorded while a run is on a step names that step's span as its parent.
- [x] After a workflow reaches `finished`, no step span of that run is left open.
- [x] Running `spec new --force` while a run is mid-step closes that run's open step span, marked abandoned.
- [x] Resuming a run whose session stopped mid-step, with a helper span still open, closes the interrupted step at its last recorded command, marked interrupted, closes the helper span the same way, and records a new span for the resumed step.
- [x] Implementing a plan with two tasks leaves one task span per task, each named after its task and holding the step spans of its work, with no trace command run.
- [x] An orchestrated lane's step spans land in the lane's own run log.
- [x] With logging off, workflows behave exactly as before and nothing is written.

### Milestone 2: Agents can mark helper agents and waits on the user

**What changes**: Agents get two commands. `spektacular trace start <name> --type sub-agent|user-wait|work` begins a span and returns its id. `spektacular trace stop <id>` ends it, optionally with an error status. A span nests under the current step unless the agent names another open span as its parent, so helpers running at the same time each nest under the span that launched them. Time spent waiting on the user is its own type, so it can later be counted apart from working time. A span left open when its parent ends, or when the run finishes, is closed and marked abandoned. Mistakes are refused with a message saying what to do: an unknown or closed parent, an unknown or already stopped span, or an attempt to stop a step. The trace commands never move the workflow. With logging off they succeed and record nothing.

**Validation point**: Integration tests cover a sub-agent span recorded with its name, times and status, an error stop, the default parent, two concurrent helpers each with their own parent and children, a user-wait span, abandoned spans on step change and on finish, every refusal, an untouched workflow state, and logging off. They also show a lane's spans landing in the lane's own run log.

#### - [ ] Task: Trace start and stop commands
**Id:** b78010a5-aa8f-4faa-ae73-0b3d93fdf88b
**Repo:** spektacular
**Depends on:**
- 0f6d5548-9fc2-4ec1-9c7a-3d2ffdd18e82 — Record step and task spans in every run's log
**Execution:** agent

Adds `spektacular trace start <name> --type sub-agent|user-wait|work [--parent <id>]`, which records the start of a span and returns its id, and `spektacular trace stop <id> [--status ok|error] [--message <text>]`, which records its end. A span nests under the current step unless the agent names another open span. Its id carries the run's trace, so a helper in another process, or in an orchestrated lane, lands in the right run. Every mistake is refused with a message saying what to do, and a refusal records nothing. The commands never touch the workflow's state, and with logging off they succeed and record nothing.

*Technical detail:* [context.md#task-trace-start-and-stop-commands](./context.md#task-trace-start-and-stop-commands)

**Acceptance criteria**:
- [ ] Starting a sub-agent span named `test-writer` returns an id, and stopping it records a sub-agent span named `test-writer` with a start, an end and a success status.
- [ ] Stopping a span with an error status records it with an error status and the given message.
- [ ] A span started without a parent is nested under the current step span.
- [ ] Two sub-agent spans started at the same time with different parents keep their own parents, and spans each helper starts with its parent's id nest under the right one.
- [ ] A user-wait span is recorded as user-wait, and no other kind of span ever is.
- [ ] A span left open is closed and marked abandoned when the workflow moves to the next step, or when it reaches `finished`.
- [ ] Naming an unknown or closed parent, stopping an id never started or already stopped, stopping a step or task span, or giving a bad type, name or id each returns an error that names the problem and the next step, and records no span.
- [ ] Starting and stopping spans leaves the workflow's current step and completed steps unchanged.
- [ ] With logging off, both commands succeed and nothing is written.
- [ ] A trace command run for an orchestrated lane's span is recorded in that lane's run log, along with the span.

### Milestone 3: Every workflow tells agents what to mark

**What changes**: Every step instruction carries one short, identical paragraph. It tells the agent to:
- mark each helper agent it launches as a sub-agent span and give the helper the span's id as its parent;
- mark each question it asks the user as a user-wait span;
- chain the stop with the command it runs next, so marking adds as few turns as possible.

The paragraph names the current step's span as the parent. The spec, plan, implement, epic-planning and epic-implementation skills (and the other workflow skills) carry the same wording. Orchestrated runs, which never ask the user, get only the helper part. The Harbor harness learns to read the new span records without changing its existing reports. It checks that helper launches and the spec suite's scripted question were marked, and counts the trace calls per plan task.

**Validation point**: Template-contract tests show the paragraph exactly once in every rendered step instruction and in every workflow skill, and never copied into a raw template. Harbor's shared unit tests pass, with unchanged pinned reference values and new span-reader and oracle self-check cases. A real Harbor run is checked by hand (see Testing Approach).

#### - [ ] Task: Trace instructions in every step and skill
**Id:** ff62713e-af21-4cd3-8675-63444035eece
**Repo:** spektacular
**Depends on:**
- 70be742b-f019-422a-aea6-ece15b1520ba — Workflow engine marks every step as a span
- b78010a5-aa8f-4faa-ae73-0b3d93fdf88b — Trace start and stop commands
**Execution:** agent

Adds one short shared paragraph telling the agent to:
- mark each helper agent it launches as a sub-agent span and put the span's id in the helper's brief as its parent;
- mark each question to the user as a user-wait span;
- chain each stop with its next command.

The step renderer appends it to every step instruction, naming the current step's span as the parent. Orchestrated runs get only the helper part. Every workflow skill carries the same paragraph after its version check, and the two library skills that guide launching helpers carry the helper part. This repository's installed skills are not regenerated.

*Technical detail:* [context.md#task-trace-instructions-in-every-step-and-skill](./context.md#task-trace-instructions-in-every-step-and-skill)

**Acceptance criteria**:
- [ ] Every step instruction of the spec, plan, implement and repo workflows carries the trace paragraph exactly once, right after the step's own text, naming the current step's span as the parent. The last step names none.
- [ ] Orchestrated plan and implement instructions carry the helper part and not the user-wait part.
- [ ] Every workflow skill, including epic planning and epic implementation, carries the same paragraph, and the helper-launch library skills carry the helper part with the command name filled in.
- [ ] No step template file contains a copy of the paragraph. It comes only from the shared partial.

#### - [ ] Task: Harbor reads and checks span records
**Id:** 3923cbea-e0d0-4215-8a86-1517ef3c0c2e
**Repo:** spektacular
**Depends on:**
- b78010a5-aa8f-4faa-ae73-0b3d93fdf88b — Trace start and stop commands
- ff62713e-af21-4cd3-8675-63444035eece — Trace instructions in every step and skill
**Execution:** agent

Keeps the Harbor harness working now that run logs hold span records, and uses them. The shared session-log reader ignores span starts and ends, so step windows, timelines and pinned reference values stay as they are. A new shared reader collects a run's sub-agent and user-wait spans. The epic-planning, epic-implementation and goal-check verifiers check that each helper launch was marked. The spec verifier checks that its scripted question was marked as a user wait. The metrics record counts trace calls overall and per plan task. A self-check ties the harness's trace wording to the shared paragraph. This task needs 000073's Harbor changes to be merged.

*Technical detail:* [context.md#task-harbor-reads-and-checks-span-records](./context.md#task-harbor-reads-and-checks-span-records)

**Acceptance criteria**:
- [ ] A run log containing span starts and ends gives the same steps, windows and timelines as one without them, and every existing pinned reference value is unchanged.
- [ ] The span reader returns each run's sub-agent and user-wait spans, pairing starts and ends across lane and day logs.
- [ ] The epic-planning, epic-implementation and goal-check verifiers report a helper launch with no matching sub-agent span as a failure, and the spec verifier fails when its scripted question has no user-wait span.
- [ ] The metrics record shows the number of trace calls in the run and the average per plan task.
- [ ] Changing the trace paragraph's command wording without updating the harness makes a Harbor self-check fail.

### Milestone 4: Spans are documented

**What changes**: The documentation site's logging page explains spans:
- the five span types;
- which spans Spektacular records by itself (steps, tasks, commands);
- how agents start and stop the others;
- how parents work;
- what an abandoned or interrupted span means;
- the two new record kinds.

The README lists the `trace` commands. The project's testing-architecture knowledge entry names the new Harbor trace oracle that must change with the trace wording.

**Validation point**: The site builds and `astro check` passes, with no layout HTML in page bodies and no em dashes in the new text. The knowledge entry reads back through the CLI with the new coupling.

#### - [ ] Task: Documentation site describes spans
**Id:** 2cccbac8-a943-4e23-993d-d29054f2a037
**Repo:** docs
**Depends on:**
- b78010a5-aa8f-4faa-ae73-0b3d93fdf88b — Trace start and stop commands
- ff62713e-af21-4cd3-8675-63444035eece — Trace instructions in every step and skill
**Execution:** agent

Adds sections on spans to the logging page that spec 000073 rewrites as "Logging and debugging". They explain the span types, the spans Spektacular records by itself, the `trace start` and `trace stop` commands and how parents work, and what abandoned and interrupted mean. They also show the two new record kinds.

**Content outline** (logging page, new sections inserted after "Reading a record" and before "Upgrading from debug mode"; the wording is illustrative, the commands, types, fields and values are fixed):
1. `## Spans: where the time went`: a run's log shows more than commands. Each workflow step and each plan task is a span, and agents mark the helper agents they start and the questions they ask. Example: "a plan run reads as `discovery` (12 min) containing its commands and two `sub-agent` spans, then `architecture` ...".
2. `## Span types`: a list.
   - `command`: one per command, recorded automatically.
   - `step`: each workflow step, from entering to leaving it, recorded automatically.
   - `task`: each plan task during implement, from `analyze` to `update_changelog`, recorded automatically.
   - `sub-agent`: a helper agent, marked by the agent.
   - `user-wait`: time waiting for the user to answer, marked by the agent and kept apart so it can be left out of working time.
   - `work`: any other stretch the agent wants to time.
3. `## Marking spans`: the commands in a fenced block:
   ```bash
   spektacular trace start test-writer --type sub-agent
   # {"span_id":"4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7", ...}
   spektacular trace stop 4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7
   spektacular trace stop <id> --status error --message "tests did not compile"
   ```
   A span nests under the current step unless `--parent <id>` names another open span. Pass a helper its span id so its own spans nest under it. Workflow instructions already tell agents when to do this.
4. `## How spans end`: a step ends when the workflow moves on. A span still open when its parent ends or the run finishes is closed as `abandoned`. A step left open by a stopped session is closed as `interrupted` when the run resumes, at its last command. `spec new --force` abandons the old run's spans. With `log.level: off` the commands succeed and record nothing.
5. `## Span records`: a sample `span_start` and its `span_end` line (from the plan's Data Structures), and field notes: `record` (`span`, `span_start`, `span_end`), `parent_span_id`, `spektacular.span.type`, `spektacular.span.end_reason` (`moved`, `finished`, `stopped`, `abandoned`, `interrupted`), `spektacular.task.id`.

*Technical detail:* [context.md#task-documentation-site-describes-spans](./context.md#task-documentation-site-describes-spans)

**Acceptance criteria**:
- [ ] The logging page describes the six span types, says which spans Spektacular records by itself, and shows `trace start` and `trace stop` with their options.
- [ ] The page explains parents, abandoned and interrupted spans, and logging off, and shows a span start and end record.
- [ ] The site builds and type-checks without errors, and the new text has no layout HTML and no em dashes.

#### - [ ] Task: README and knowledge describe spans
**Id:** f1a446d5-c4a3-4daf-b490-86e56d138aaf
**Repo:** spektacular
**Depends on:**
- b78010a5-aa8f-4faa-ae73-0b3d93fdf88b — Trace start and stop commands
- 3923cbea-e0d0-4215-8a86-1517ef3c0c2e — Harbor reads and checks span records
**Execution:** agent

Brings the project's own descriptions up to date. The README's command overview gains the two `trace` commands with a line on what Spektacular records by itself and where to read more. The testing-architecture knowledge entry adds the new Harbor trace oracle to its list of couplings that must change with the templates. The goto-to-the-current-step gotcha notes that such a goto still leaves the workflow's progress alone but now opens a fresh step span. Historical plans and changelog entries are left as they are.

*Technical detail:* [context.md#task-readme-and-knowledge-describe-spans](./context.md#task-readme-and-knowledge-describe-spans)

**Acceptance criteria**:
- [ ] The README shows `trace start` and `trace stop` with their options and says that steps and tasks are recorded automatically.
- [ ] The testing-architecture knowledge entry names the trace oracle and the shared paragraph it mirrors, and reads back through the knowledge CLI.
- [ ] The goto-to-the-current-step gotcha says the progress is still unchanged and that the step's span is closed as interrupted and reopened.

## Open Questions

- **Whether 000073 lands with the names and record fields this plan builds on.** This depends on 000073's implementation (`tracelog.Record`, `Run`, `Logger`, the run-file naming, `State.RunID`, and `runRoot`'s snapshot and attribution helpers). If only names differ, adapt to them. If a record field or the run attribution rule differs from its plan, STOP and ask the user, because 000075 to 000077 read the same contract.
- **Whether reading the run log on each step change and each `trace stop` stays within the logging time budget on a verbose log.** This depends on real log sizes from a long run. If the manual timing check in the test plan shows a step change costing noticeably more than 10 ms at the standard level, STOP and ask before adding a cache, for example an index file beside the run log, because that would add a second file the logger keeps.
- **Whether any existing test asserts the exact text or suffix of a rendered step instruction or skill in a way the new trace block breaks beyond simple placement.** This only surfaces once the block is appended everywhere. Update placement and count assertions. If a test pins behaviour the spec now changes in another way, STOP and ask.

No other implementation-time uncertainties remain. The design choices are settled in Architecture & Design Decisions and the assumption log.

## Out of Scope

- **Summarising, comparing or excluding user waits from timings.** That is `000075_run-stats-command`. This plan only records the spans and keeps user waits in their own type (spec Non-Goals).
- **Replacing Harbor's transcript-based timing.** That is `000076_harbor-on-built-in-logs`. This plan only keeps Harbor's reader working with the new records and adds span checks.
- **Drawing spans as a waterfall.** That is `000077_html-log-viewer`.
- **Detecting helper launches or waits on the user without the agent marking them**, and **recording the agent's own tool or model calls as spans**. The CLI cannot see them (spec Non-Goals).
- **Changing how or when helper agents are launched.** The instructions only add marking around launches that already happen (spec Non-Goals).
- **Linking an epic orchestrator's sub-agent span to the child's own workflow run.** Each run keeps its own trace (000073). The orchestrator's spans live in a standalone trace, and the child's run in its own trace.
- **Closing standalone spans automatically.** A span started outside any workflow run is closed only by its `stop`.
- **Trace wording in the AGENTS.md standing rules and managed sections.** They are not workflow instructions, and their renderer has no partial support.
- **Reworking the Harbor implement suite so it runs plan tasks.** It starts at `test_plan`. The per-task cost metric is read from suites that run tasks.
- **Re-installing this repository's own skills or migrating its `.spektacular/`** (plans-never-change-the-active-install). The maintainer installs the new binary between workflows.
- **Editing historical plans, test plans and changelog entries.**

## Changelog


### 2026-10-25 — Task: Span records and run log reading

**What was done**: The trace log package gained the `span_start` and `span_end` record kinds, the step, task, sub-agent, user-wait and work span types, the end reasons with the status each gets, and the agent-held span id `<trace_id>-<span_id>` with its parser. The file logger gained a read side that returns a run's span records in file order, and a pure index plus a `Spans` service start and end spans, close every open span under a parent or in a whole run, and find a step's last command.

**Deviations**: `ParseSpanRef` accepts any id of the right form, including all-zero parts; the context's "reject all-zero parts" note was dropped because the acceptance criterion accepts every `<32 hex>-<16 hex>` id (goal-check retry). `Spans.End` returns nothing; `StartRecord` (build without writing) and `Index()` were added so the workflow sink can end spans whose start was never logged and query the index once.

**Files changed**:
- `spektacular: internal/tracelog/record.go`
- `spektacular: internal/tracelog/record_test.go`
- `spektacular: internal/tracelog/spanref.go`
- `spektacular: internal/tracelog/spanref_test.go`
- `spektacular: internal/tracelog/reader.go`
- `spektacular: internal/tracelog/reader_test.go`
- `spektacular: internal/tracelog/index.go`
- `spektacular: internal/tracelog/index_test.go`
- `spektacular: internal/tracelog/spans.go`
- `spektacular: internal/tracelog/spans_test.go`

**Discoveries**: The reader returns every `span` (command) record, not only those with a parent, so the "command records in file order" criterion holds; the pre-filter is the `"record":"span` substring. `LastCommandUnder` looks at direct children only. A standalone run's records are read from its start day's file and the day before.

### 2026-10-25 — Task: Workflow engine marks every step as a span

**What was done**: The workflow engine now mints a span id for every step it enters, keeps the open step span in state (`step_span`, beside `run_id`), and reports each end and start to an optional `SpanSink` on `workflow.Config`. Before each step callback it puts the step span's composite id (`<run id>-<span id>`) on `Config.StepSpan`, empty on the terminal step. A goto to the current step reports the open span interrupted and opens a fresh one, saving state but leaving `current_step` and `completed_steps` alone; reaching the terminal step records an instant span and ends the run. `AbandonRun` reports an in-progress run's open spans as abandoned.

**Deviations**: `OpenSpan` gained a `step` field (the step that opened a task span), so a task span's `spektacular.step` survives in state. The engine passes `abandoned` as the `RunEnded` reason; the sink closes every leftover span as abandoned in any case. The span lifecycle lives in new `moveSpans`/`resumeSpans`/`prepareSpan` helpers rather than a `taskTransition` hook; the task handling is added there by the next task. Tests are in a new `internal/workflow/spans_test.go` rather than `workflow_test.go`.

**Files changed**:
- `spektacular: internal/workflow/state.go`
- `spektacular: internal/workflow/spans.go`
- `spektacular: internal/workflow/workflow.go`
- `spektacular: internal/workflow/spans_test.go`

**Discoveries**: looplab/fsm reads `w.cfg` at callback time, so setting `w.cfg.StepSpan` before `FSM.Event` reaches the `before_` callback that renders the instruction. The implicit `done` event enters no step and is ignored by the span logic. Nothing calls `AbandonRun` yet; the restart wiring is the "Record step and task spans" task.

### 2026-10-25 — Task: Implement tasks become spans

**What was done**: Steps can now declare a task role (`TaskStart`, `TaskWithin`, or the default `TaskNone`). For implement, `analyze` starts a task and `implement` through `update_changelog` run within it, so the engine keeps a task span open over them and nests their step spans under it; looping back to `analyze` ends one task span and starts the next, and leaving to `test_plan` or `finished` ends it. `analyze` hands the engine the current task (the single-task run's task, or the goal check's current item, with a phase titled by its title or label) under `workflow.TaskDataKey`.

**Deviations**: The engine also passes `finished` (not `abandoned`) as the `RunEnded` reason on reaching the terminal step, as the plan's context describes; the sink closes leftover spans as abandoned. Tests are in a new `internal/steps/implement/task_spans_test.go` and in `internal/workflow/spans_test.go`, reusing the goal-check fixtures, rather than in `steps_test.go`.

**Files changed**:
- `spektacular: internal/workflow/workflow.go`
- `spektacular: internal/workflow/spans.go`
- `spektacular: internal/workflow/spans_test.go`
- `spektacular: internal/steps/implement/steps.go`
- `spektacular: internal/steps/implement/task_spans_test.go`

**Discoveries**: A run from before spans existed, resumed mid-task, has no task span; the first within-task step opens one from the data `analyze` left. Finding the task never fails `analyze`: an unreadable plan just names the span "task".

### 2026-10-25 — Task: Record step and task spans in every run's log

**What was done**: `cmd` now records the workflow's step and task spans in the run's log. When logging is on, `runRoot` builds the configured logger once, sets a package-level `spanRecorder` (a `workflow.SpanSink` over `tracelog.Spans`) before the command runs and clears it after; every spec, plan, implement and repo workflow config takes it. Each command record names the step span it ran in as `parent_span_id`, `clearState` abandons a restarted run's open spans, and the recorder closes spans open under an ending span as abandoned, ends an interrupted step at its last real command, and closes leftovers as abandoned when the run ends.

**Deviations**: `recordCommand` now takes the logger `runRoot` built instead of building its own, so the command record and the spans share one logger. Command parents come from the state before the command when it belongs to the run, so a resuming `goto <current step>` and the resume probe are filed under the interrupted step span. Two existing log tests changed for the intended behaviour: `TestLog_SpecRunSharesOneTraceID` now accepts the old run's file gaining an abandoned `span_end` after `--force`, and `TestLog_ResponseAndStateOnlyAtVerbose` expects the new `step_span` key in the verbose state snapshots. `readRecords` in the cmd log tests now returns command records only; `readSpanLog` returns every record.

**Files changed**:
- `spektacular: cmd/spans.go`
- `spektacular: cmd/root.go`
- `spektacular: cmd/resume.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/repo.go`
- `spektacular: cmd/implement.go`
- `spektacular: cmd/root_test.go`
- `spektacular: cmd/log_test.go`
- `spektacular: cmd/log_spans_test.go`

**Discoveries**: Most cmd tests log at the standard level by default, so every workflow test now also writes span records; helpers that assumed one record kind had to filter. State gains `step_span`/`task_span` even with logging off (ids are kept, nothing is written). Repo workflow commands are snapshotted from `state.json`, not `repo-state.json` (000073 behaviour), so a repo run's step spans land in its own run file but its command records are not parented to them.
