---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Research: 000074_trace-spans-for-agent-work

## Alternatives considered and rejected

- **Derive step spans in `runRoot` from the before/after state snapshots only.** `runRoot` would compare the open step span in the before and after states and write an end and a start. Rejected: one command can pass through several steps (`implement new` returns `read_plan`, `internal/steps/implement/steps.go:83-87`; `Goto` recurses on `pendingGoto`, `internal/workflow/workflow.go:209-223`), so the intermediate step spans would be lost. It also cannot render the new step's span id into the instruction the same command returns.
- **Keep a span stack, or a set of open agent spans, in workflow state.** Rejected by the spec (Technical Approach: "rather than keeping a span stack in workflow state"). It would also make `trace start`/`stop` write state, which the requirement "Trace commands are safe" rules out. And concurrent helpers would race on the read-modify-write of `state.json` (`saveState` is a plain `os.WriteFile`, `internal/workflow/state.go:49-61`).
- **A separate locked registry file of open spans per run.** Rejected: it needs file locking for concurrent helpers (no locking exists anywhere in the codebase) and duplicates what the run's own log already holds. The append-only log is already safe for concurrent writers (000073: one `O_APPEND` write per record).
- **A bare 16-hex span id as the agent-visible identifier.** Rejected: `trace stop` runs in another process and must find the span's run. Orchestrated lanes keep their own state files (`internal/workflow/lane.go:20-22`), a `trace` command names no spec, so `snapshotStatePath` (`cmd/root.go:247-345`) would file it under `state.json`. A standalone span's start and stop would also get two different trace ids under 000073's standalone rule (a fresh trace per command).
- **Writing step and task records from inside `internal/workflow` by importing `tracelog`.** Rejected: 000073 keeps `workflow` independent of `tracelog` (it duplicates `NewRunID` for that reason). The engine instead reports to a narrow sink interface that `cmd` implements.
- **Asking agents to mark steps or tasks.** Rejected by the spec constraint (the user: "it makes sense that goto opens and closes spans, also because it is one less thing for the agent to do").
- **Including the trace wording in each template that asks or launches.** Rejected in favour of one block appended by `stepkit.WriteStepResult` to every step instruction. Questions appear in almost every step template (spec section steps, every repo step, implement STOPs, `partials/proceed-unless-blocked.md` in 11 plan steps), so per-template inclusion would be long and easy to miss. One appended block is pinned by one contract test, like the working-context footer (`internal/stepkit/stepkit.go:191-197`, `cmd/instruction_contract_test.go:239-266`).
- **Detecting helper launches or user waits automatically.** Out of scope (spec Non-Goals). The CLI cannot see agent tool calls.

## Chosen approach — evidence

- The engine already has the transition hooks: per-step `before_<event>` callbacks render the instruction before the transition commits, and `enter_state` persists state on every transition (`internal/workflow/workflow.go:148-181`). `renderStep` re-renders the current step without a transition, which is the resume path (`workflow.go:298-317`). `commitTerminal` marks the last step (`:323-339`).
- Fresh state is built only in `workflow.New` (`workflow.go:102-118`). 000073 adds `RunID` there, and its plan reserves the open step-span id beside it.
- `new --force` and every restart go through `clearState(statePath)` (`cmd/resume.go:216-218`, called from `cmd/spec.go:273,316`, `cmd/plan.go:138`, `cmd/implement.go:197`, `cmd/resume.go:184`). That is one choke point for abandoning the old run's spans.
- The whole-plan current task is computable in the CLI. `goal.Current(plan, outcome, selected)` returns the first open, unskipped task or phase (`internal/goal/current.go:9-27`). `loadGoalView` already resolves it (`internal/steps/implement/goalcheck.go:31-60`). The current-task partial defines the task the same way for the agent (`templates/partials/implement-current-task.md`). A task's work runs analyze → implement → test → verify → (check_goal) → update_plan → update_changelog, then loops back to analyze or leaves to test_plan or finished (`internal/steps/implement/steps.go:36-52`).
- Step instructions are rendered with a shared variable bundle (`internal/stepkit/stepkit.go:139-151`). Blocks are appended after the body (orchestrated hand-back `:157-163`, commit `:166-190`, footer `:191-197`). Partials resolve through `FSPartials` and fail on a missing file (`:237-260`).
- Workflow skills render with `{{command}}` only (`internal/agent/skills.go:53`). So a partial shared between steps and skills uses `{{command}}`, which `stepkit` also supplies (`stepkit.go:146-148`).
- 000073 plan: `runRoot` is the single choke point that writes one `span` record per command through `tracelog.Logger`. The record has `parent_span_id` reserved and a `record` discriminator ready for `span_start`/`span_end`. Run attribution comes from the before/after snapshots of the state file `snapshotStatePath` picks.
- The run's log file is append-only JSON Lines, named from the run (000073 `runFilePath`), so a read of that one file gives every span start and end of the run without a directory scan.

## Files examined

- `spektacular:internal/workflow/workflow.go:16-48` — `Config` (runtime-only, not persisted): the place for the span sink and the pre-minted step span.
- `spektacular:internal/workflow/workflow.go:98-185` — `New`: fresh state, `before_` callbacks, `enter_state` persistence.
- `spektacular:internal/workflow/workflow.go:190-223,298-339` — `Next`/`Goto` recursion on `pendingGoto`, `renderStep` (resume), `commitTerminal`.
- `spektacular:internal/workflow/state.go:12-61` — `State` shape and `saveState` (no locking, plain write).
- `spektacular:internal/workflow/lane.go:10-66` — lane state paths `workflows/<kind>-<name>.json`, `LaneNames`.
- `spektacular:cmd/root.go:91-176` — today's recorder in `runRoot`; 000073 rewrites it.
- `spektacular:cmd/root.go:220-398` — `stateSnapshotFile`, `snapshotStatePath` (lane by argument), `readStateSnapshot`.
- `spektacular:cmd/resume.go:179-218` — `resumeOrClear`, `probeResume`, `clearState`.
- `spektacular:cmd/spec.go:273-297,385`, `cmd/plan.go:138-144,224`, `cmd/implement.go:197-210,565`, `cmd/repo.go:253-320`, `cmd/autocommit.go:75-190` — every `workflow.Config` built and every `Goto`.
- `spektacular:cmd/autocommit.go:286-289` — `removeLane` deletes a finished lane's state, after the finishing transition.
- `spektacular:internal/steps/implement/steps.go:36-52,83-120,171-199,293-340` — the implement step table, the `new` → `read_plan` chain, `taskRun`/`taskExtra`/`taskVars`.
- `spektacular:internal/steps/implement/goalcheck.go:31-60` — `loadGoalView` resolves the current item.
- `spektacular:internal/goal/current.go:9-27` — `goal.Current`.
- `spektacular:internal/stepkit/stepkit.go:59-200,237-260` — the bundle, appended blocks, footer gate, partial loader.
- `spektacular:templates/partials/*` — existing partials. `version-check.md` opens all 8 workflow skills.
- `spektacular:templates/steps/implement/02-analyze.md:28-43`, `04-test.md:5`, `05b-check_goal.md:54-72`; `templates/steps/plan/02-discovery.md:47-55`; `templates/steps/spec/08-verification.md:25-41`; `templates/partials/split-flow.md:19` — the steps that launch helpers.
- `spektacular:templates/skills/workflows/spek-plan-epic/SKILL.md:36-62`, `spek-implement-epic/SKILL.md:45-76` — child launches and child prompts.
- `spektacular:templates/skills/skill_spawn-planning-agents.md`, `skill_spawn-implementation-agents.md` — library skills, rendered with an empty data map (`cmd/skill.go:61`).
- `spektacular:cmd/instruction_contract_test.go:43-338` — the step table, `renderAllStepInstructionsWith`, the footer tests, `workflowInstructionCorpus`.
- `spektacular:cmd/orchestrated_test.go:22-92` — the "every orchestrated step carries the block exactly once, before the footer" pattern.
- `spektacular:internal/agent/instruction_surface_test.go:73-128` — every template root walked for forbidden substrings.
- `spektacular:internal/agent/skills.go:26-71` — workflow skill install and render.
- `spektacular:tests/harbor/shared/sessionlog.py:25-29,95-115,212-227,291-317` — the reader breaks on lines without `timestamp`. 000073 adds a `trace_id` branch. Every event counts toward step-window ends.
- `spektacular:tests/harbor/shared/delegation.py:36-56`, `tests/harbor/shared/tests/test_delegation.py:53,796-865` — the delegation oracle and its template tie (the pattern for a new contract tie).
- `spektacular:tests/harbor/shared/transcripts.py:37,85-134,471-580` — Claude and ACP sub-agent launch readers.
- `spektacular:tests/harbor/epic-workflow/tests/test_epic_workflow.py:175-213`, `implement-epic-workflow/tests/test_implement_epic_workflow.py:296-341` — `TestChildren` (the only classes allowed to read the transcript, `test_epic_children.py:393-407`).
- `spektacular:tests/harbor/implement-workflow/environment/{state.json,plan.md}` — the implement suite starts at `test_plan` with every phase ticked, so it runs no plan tasks.
- `spektacular:tests/harbor/spec-workflow/instruction.md:35-64` — the agent plays the scripted user. No question counter exists.
- `spektacular:tests/harbor/shared/metrics.py:397-459,507-523,624-674` — metrics have no tool-call count.
- `spektacular:go.mod:3` — Go 1.25, so the `omitzero` JSON tag is available.
- `docs:src/pages/debugging.mdx` — the logging page 000073 rewrites as "Logging and debugging". This spec adds span sections to it.

## External references

- OpenTelemetry trace data model (spans, parent span id, status Ok/Error): the record format 000073 adopted. Span ids are 8 bytes and trace ids 16 bytes.
- W3C Trace Context `traceparent` (`version-traceid-spanid-flags`): the precedent for carrying trace and span id together in one hyphen-separated, shell-safe token. The agent-visible span id here is `<trace_id>-<span_id>`.

## Prior plans / specs consulted

- `000073_log-mode-and-logger-providers` (plan and context): the record contract, `tracelog` package, `Logger` interface, `Run` and file naming, `run_id` in state, `EnsureRunID`, run attribution in `runRoot`, `SPEKTACULAR_SESSION_LOG` levels, and the Harbor normalising reader. This plan builds on all of them and changes none of their fields.
- Spec `000074_trace-spans-for-agent-work` (this spec). Siblings `000075` (stats), `000076` (Harbor on built-in logs), `000077` (viewer) read what this plan records.
- `docs/workflow-performance-investigation.md` is cited by the spec: long instructions and extra agent turns are the main costs, so the trace wording is one short block and trace calls are chained with commands the agent already runs.

## Open assumptions

- 000073 lands as planned: `tracelog.Record`/`Run`/`Logger`, `workflow.State.RunID`, the run-file naming, and `runRoot`'s run attribution exist with the names in its plan. If any differ, adapt the names. If the record contract itself differs, STOP and ask.
- A `<kind> new` command in the middle of a step is only ever a resume probe. It is left out when finding an interrupted step's last activity.
- Reading the run's log file on each step change and each `trace stop` stays inside the logging time budget at the standard level. Verbose logs are larger, so the reader pre-filters lines before decoding them.
- Agents follow the appended block well enough to reach the 90% success metrics. This is only measurable in Harbor runs (manual).
- The Harbor implement suite runs no plan tasks (it starts at `test_plan`). Task spans and the per-task tool-call metric are therefore measured on the implement-epic and goal-check suites, which do run tasks.

## Drafting assumptions

### Chosen direction: engine-marked steps and tasks, log-backed agent spans (architecture)
- **Decision**: The workflow engine mints step and task span ids, keeps the open ones in state beside `run_id`, and reports transitions to a `SpanSink` on `workflow.Config`. `cmd` writes `span_start`/`span_end` records through the 000073 logger. `trace start`/`trace stop` write agent spans and use the run's log as the registry of open spans. The id an agent holds is `<trace_id>-<span_id>`. One appended instruction block, plus one skill partial, tells agents what to mark.
- **Rationale**: it catches chained transitions and puts the step span in the instruction. It keeps trace commands from writing state, is safe for concurrent helpers, and reaches orchestrated lanes. It is the spec's own Technical Approach made concrete.
- **Rejected**: snapshot diffing in `runRoot`, open spans in state, a locked sidecar registry, bare span ids, per-template wording (see research.md).

### The run's own log is the registry of open spans (discovery)
- **Decision**: `trace stop`, parent validation, closing spans when a step ends and finding an interrupted step's last command all read the run's log file. No open-span list is kept in state or in a separate file.
- **Rationale**: the log is append-only and safe for concurrent helpers. The spec forbids a span stack in state and forbids trace commands changing state. With logging off there is nothing to close.
- **Rejected**: open spans in `state.json` (races, and it changes state); a locked registry file (no locking exists, and it duplicates the log).

### Agent-visible span id is `<trace_id>-<span_id>` (discovery)
- **Decision**: `trace start` returns a 49-character token joining the 32-hex trace id and 16-hex span id with a hyphen. `trace stop` and `--parent` route to the run by its trace id. Records keep the separate `trace_id` and `span_id` fields.
- **Rationale**: a trace command names no spec, so it cannot find an orchestrated lane by argument, and a standalone span needs one trace id across two processes. This follows the W3C traceparent precedent.
- **Rejected**: a bare span id (cannot route lanes or keep standalone spans in one trace).

### Engine mints span ids and reports to a sink; `cmd` writes the records (discovery)
- **Decision**: `internal/workflow` mints step and task span ids, keeps the open ones in state, and calls an optional `SpanSink` on `workflow.Config`. `cmd` implements the sink over `tracelog` with the configured logger.
- **Rationale**: this catches chained transitions inside one command, lets the instruction name the new step's span, and keeps `workflow` free of `tracelog`, as 000073 chose.
- **Rejected**: `runRoot`-only snapshot diffing (loses chained steps).

### One appended trace block on every step instruction (discovery)
- **Decision**: `stepkit.WriteStepResult` appends one short partial to every step instruction, placed straight after the template body. The partial names the helper-launch and user-wait rules and the current step's span as `--parent`. Every workflow skill includes the same partial after its version check, and the two spawn library skills include its helper rule.
- **Rationale**: questions occur in nearly every template. One block pinned by one test cannot drift, and it keeps the wording identical everywhere (spec Technical Approach).
- **Rejected**: per-template inclusion (long, easy to miss).

### Abandoned and interrupted spans end with an error status and a reason attribute (architecture)
- **Decision**: a span closed by the workflow ends with `spektacular.span.end_reason` set to `moved`, `finished`, `abandoned` or `interrupted`. An agent's stop sets `stopped`. Abandoned and interrupted spans carry status `{code: "error", message: "abandoned" | "interrupted"}`. Moved and finished step spans are `ok`.
- **Rationale**: 000073 fixes the status codes at `ok|error`. The attribute lets 000075 and 000077 filter without parsing messages, and an error status flags that the span's real end is unknown.
- **Rejected**: new status codes (they would break the 000073 contract); status `ok` with only the attribute (a viewer would show unfinished work as success).

### A `span_end` record repeats the whole span (architecture)
- **Decision**: `span_end` carries every field of its start (name, parent, type, start time, attributes) plus the end time and status. `span_start` omits `end_time` and `status`, through the `omitzero` JSON tag on 000073's `Record` (Go 1.25).
- **Rationale**: readers can treat `span` and `span_end` records as complete spans and a lone `span_start` as still open, so pairing is optional. The writer already has the start, either in state (steps and tasks) or from the log read (agent spans).
- **Rejected**: a minimal end record (every reader would have to pair).

### Step boundary time is the moment of the transition (architecture)
- **Decision**: a step span ends, and the next begins, at the engine's transition time inside the command. The command that moved the workflow is parented to the step it ran in, so it overhangs that step's end by the few milliseconds its remaining work takes.
- **Rationale**: the spec says commands nest under the step they were sent in and that each step ends when the next begins. The engine does not know when the command will end.
- **Rejected**: parenting the moving command to the new step (contradicts "commands sent during a step").

### Task span stays open across a resume (architecture)
- **Decision**: resuming closes only the step span and the spans open under it. The task span continues, so each task keeps exactly one span.
- **Rationale**: the spec names only the step and its children. The acceptance criteria expect one task span per task. 000075 can subtract the gap between an interrupted step's end and the resumed step's start.
- **Rejected**: splitting the task span on resume (two spans per task).

### The terminal step is an instant span (architecture)
- **Decision**: reaching `finished` closes the previous step and every open span in the run, then records `finished` as a step span that starts and ends at once.
- **Rationale**: no later command of the run can close it (after `finished` the state is no longer in progress, and lanes are removed), and the spec says no step span is left open.
- **Rejected**: leaving `finished` out (loses the run's end marker).

### Resume probes and trace commands do not count as a step's last activity (architecture)
- **Decision**: an interrupted step ends at the latest command record parented to it, not counting `<kind> new` commands (resume probes) or `trace` commands.
- **Rationale**: the resume report command and the resuming session's own trace calls are parented to the interrupted step. Counting them would stretch it over the dead time.
- **Rejected**: counting every command (wrong end time).

### `--type` defaults to `work`; agents cannot start step or task spans (architecture)
- **Decision**: `trace start` accepts `sub-agent`, `user-wait` and `work`, defaulting to `work`. `step` and `task` are refused as CLI-owned, and stopping a step or task span is refused.
- **Rationale**: every span still has a type. The engine owns the step and task spans.
- **Rejected**: a required `--type` (one more failure mode, and templates always pass it anyway).

### Spans outside any run are standalone and never closed automatically (architecture)
- **Decision**: a `trace start` with no in-progress run and no parent starts a fresh trace in the day's standalone log, as the epic orchestrators' child launches do. Nothing closes such a span except its `stop`. Links between an orchestrator's span and its child's own run are out of scope.
- **Rationale**: the epic orchestrators run no workflow of their own. 000073 gives each workflow run its own trace.
- **Rejected**: adopting the parent's trace for a child run (contradicts 000073's run id design); an orchestrator run state (a new workflow, out of scope).

### Conventions selected (architecture)
- **Decision**: apply error-messages-must-suggest-remediation, plans-never-change-the-active-install, store-files-must-be-written-through-the-cli, tests-must-not-depend-on-order, tests-must-pass-for-done, and the docs plan-content-pages, no-em-dashes and mdx-authoring conventions. Drop the docs site-layout, alternate-section-background and file-scoped-section-headings conventions.
- **Rationale**: the dropped conventions govern new bands, sections and reference headings, and this work only adds prose sections inside an existing `<Prose>`.
- **Rejected**: listing every convention.

### Harbor gains span checks inside its transcript-reading classes (components)
- **Decision**: the sub-agent span checks go into the plan-epic, implement-epic and goal-check verifiers' `TestChildren` classes. The spec suite checks for a user-wait span around its scripted rejection question. The implement suite gets no task checks, because it runs no plan tasks.
- **Rationale**: only `TestChildren` may read the transcript (AST guard, `test_epic_children.py:393-407`). The spec suite has no question counter, but its one scripted follow-up question is a fixed point to check.
- **Rejected**: changing the implement suite's fixture so it runs tasks (a large Harbor rework, outside this spec); counting every question with a prose heuristic (unreliable).

### Per-task tool-call metric read from suites that run tasks (testing_approach)
- **Decision**: the "no more than two extra tool calls per plan task in the Harbor implement suite" metric is computed from the implement-epic and goal-check suites' task runs, with the implement suite's own total reported beside it.
- **Rationale**: the implement suite starts at `test_plan` with every phase ticked (`tests/harbor/implement-workflow/environment/state.json`), so it runs no plan tasks and has no per-task average. The metric's intent, the cost per plan task, is measurable only where tasks run. Flagged for the review.
- **Rejected**: reworking the implement suite's fixture to run tasks (changes what that suite tests, and is outside this spec).

### Task split and ordering (tasks)
- **Decision**: there are nine tasks. Each milestone-1 piece is its own task (records and reading, engine steps, implement tasks, recording through `cmd`). Every task depends on 000073 landing, which is stated in each summary and not modelled as an in-plan dependency. The Harbor and docs tasks come after the commands and the instruction block.
- **Rationale**: the engine and tracelog pieces are independent and parallel. The wiring task is the integration point. The task-id rules allow only in-plan dependencies.
- **Rejected**: one large "spans" task (too big to verify); splitting the instruction block per workflow (one partial, one test).

### Epic child prompts carry the child's parent span (tasks)
- **Decision**: the plan-epic and implement-epic child prompt lists gain one bullet passing the child the id `trace start` returned for it.
- **Rationale**: the spec requires that helpers are given their parent span. The child prompt is that brief.
- **Rejected**: relying only on the generic partial's wording (the orchestrator assembles the brief from these lists).

### Resume goto refreshes `step_span` in state (verification)
- **Decision**: a goto to the current step now writes `step_span` to state, while `current_step` and `completed_steps` stay unchanged. The gotcha entry describing that goto as changing no state is updated to say so.
- **Rationale**: the spec requires the resumed step to be recorded as a new span, with its id kept in state. The gotcha's substance (no progress, no signal that nothing advanced) still holds.
- **Rejected**: keeping the old span id across a resume (contradicts the spec's interrupted-and-new-span requirement).

## Rehydration cues

- `spektacular spec file read 000074_trace-spans-for-agent-work`
- `spektacular plan file read 000073_log-mode-and-logger-providers plan` and `... context`
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`, and `architecture/workflow-steps.md`, `gotchas/goto-to-current-step-is-a-silent-noop.md`, `gotchas/fsm-cancel-only-works-before-transition-commits.md`
- Re-read `internal/workflow/workflow.go`, `internal/stepkit/stepkit.go:103-200`, `cmd/root.go:247-345`, `cmd/resume.go:179-218`, `internal/steps/implement/steps.go:36-52`
