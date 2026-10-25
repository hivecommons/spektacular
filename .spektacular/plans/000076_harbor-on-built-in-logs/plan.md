---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Plan: 000076_harbor-on-built-in-logs

<!-- Metadata -->
<!-- Created: 2026-10-25T09:00:14Z -->
<!-- Commit: 3913717 -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Harbor, Spektacular's end-to-end test harness, stops measuring runs by parsing each agent's transcript and Spektacular's old debug log. It takes every step timing, helper agent, wait and epic timeline from the logs Spektacular now keeps, read only through `spektacular stats`, at the default log level. `stats` gains two listings for this: commands, and helper and wait spans, including those an epic orchestrator records outside any run.

Transcripts stay the source only for tokens, models and cost. Maintainers get the same measures for Claude, Codex and Bob, a full per-step split of working, helper-wait and user-wait time, and a harness about a third smaller.

## Conventions

- **Error messages must describe the problem and suggest remediation** — the two new `stats` subcommands reuse 000075's refusals and every new error path is an `output.NewError(...).WithNextAction(...)`; Harbor's `runlog` failures also name the fix (install or point `SPEKTACULAR_BIN` at a binary with `stats`, or pass the logs directory).
- **Tests must not depend on order** — the new Go tests run under the shuffled suite through `runRootCmd`/`resetRootCmd` with fixed-timestamp fixture logs; the Python tests use fixed fixtures in their own temp directories and never the wall clock.
- **Passing tests are required before calling work done** — the full shuffled Go suite and `pytest tests/harbor` (shared and per-suite self-checks) must pass, with no failure dismissed as pre-existing.
- **A plan never changes the active skills and configuration** — no task touches a template, skill, settings format or this repo's own `.spektacular/`; the Python integration tests build a throwaway binary from the working tree into a temp directory, never install it, and Harbor suite runs are manual.
- **Store files are written through the CLI** — the testing-architecture and Harbor gotcha knowledge entries are updated with `spektacular knowledge write --from`; the logs directory is not a store, and is read only through `stats`.
- **Plans must sketch content structure (docs repo)** — the Run statistics page additions carry a content example with the verified subcommand names, flags and fields.
- **No em dashes (docs repo)** — applies to the new docs, README and knowledge prose.
- **MDX authoring conventions (docs repo)** — the additions stay inside the page's existing `<Prose>` with fenced code blocks and no layout HTML.

## Architecture & Design Decisions

**Shape.** Harbor stops parsing Spektacular's logs and agent transcripts for timing. One new shared module, `tests/harbor/shared/runlog.py`, is the only place Harbor touches Spektacular's logs, and it reads them only by running `spektacular stats ... --dir <logs> --format json` and parsing the documented JSON. Everything else asks `runlog` (all in the `spektacular` repo, under `tests/harbor/`):
- the metrics recorder (`metrics.py`) for steps, the per-step split, helpers, waits and gaps;
- the epic timeline (`timeline.py`) for lanes, slowest steps, hand-off gaps and resumes;
- the delegation rules (`delegation.py`) for where required helpers were started;
- every suite verifier that today builds a `SessionLog` for argv, step and exit-code rules.

`sessionlog.py` and 000074's `spans.py` are deleted. `transcripts.py` keeps only the token, model, cost and turn readers, the child records the brief and wake-up checks read, and the launch readers the goal-check verifier uses. The throwaway test projects the suites seed drop the verbose log level 000073 sets in their configs, so every run records at the default `standard` level, and the logs reach each trial's artifacts through the existing `/app/.spektacular` artifact (`artifacts/spektacular/logs/`).

**What `stats` has to grow.** 000075 reports workflow runs only. Harbor also needs two things it leaves out: each command's argv, step and exit status, which the verifier rules read, and the records written outside any run, which hold the epic orchestrator's `epic` and `trace` commands and its `sub-agent` spans. Its `status <epic>` polls are not among them: 000073 exempts `status` from logging, so they are never recorded. 000075's plan hands exactly this to this spec. So `internal/runstats` and `cmd/stats.go` gain two read-only flat listings, which take the same selection flags, `--dir` and `--format`:
- `spektacular stats commands` lists every command record, in runs and outside them. Each carries its run, workflow, step, `step_to`, argv, times, exit code and message.
- `spektacular stats spans` lists every agent-marked span (`sub-agent`, `user-wait`, `work`), in runs and outside them.

Neither changes any 000075 output shape, and both inherit the group's logging exemption, so asking never writes to the logs. The analysed figures still come from `stats list` and `stats run`. Harbor never re-derives a wait, a gap or a step duration, so a maintainer running `stats run --dir <trial>/artifacts/spektacular/logs` sees the same numbers as the metrics record.

**Key decisions.**
- **Default level only.** Nothing Harbor reads may need a response or a state snapshot, which only the verbose level records (spec Constraints). Each check that needed one moves to a default-level fact:
  - a resume is a failed `<kind> new` in the lane's own run, and the step it stopped at is that command's `step`;
  - "not already at `update_plan`" is the command's `step`;
  - an implementing child's brief must name its spec's worktree folder `<root>/.spektacular/worktrees/<spec>`, a hand-kept constant, instead of paths parsed from `epic worktree` responses.
- **The record is agent-independent.** Schema 3 of `metrics.json` takes every step, helper, wait and gap figure from the logs, with no per-agent branch. Per-step output characters and transcript sub-agent counts are dropped rather than kept for some agents. A step's split is `working = duration - helper wait - user wait`, floored at 0, and the three always add up to its duration. The totals report user wait, helper wait and gap time (gaps over `stats`' default 5-minute threshold) in place of the single longest gap (`idle_s`). Tokens, models and cost come from the same transcript readers as today, unchanged, and a characterisation test pinned before the change proves it.
- **Unknown, never zero; never fail.** `runlog` turns a missing binary, a missing logs directory, a `stats` error or unparsable output into one reason. The recorder lists every log-sourced field under `unknown` with that reason, and nothing falls back to transcripts. The existing per-section `gather` keeps a broken section from costing the record or the reward.
- **Delegation follows spans, briefs follow transcripts.**
  - A required step (the hand-kept `REQUIRED` oracle, unchanged) passes when a completed attempt of it holds a `sub-agent` span carrying that step.
  - A per-spec lane passes when an orchestrator `sub-agent` span was open when the lane's first run started.
  - The brief checks still read prompts from the transcript. A review child is tied to its span by launch time (within 120 s, as 000074's marking check does), and an epic child to its lane by the spec its prompt names.
  - All of these stay inside today's Claude-gated `TestChildren` classes, so no suite's pass or fail rules change beyond where they read their data (spec Non-Goals).
- **Hand-off gaps come from the orchestrator's `sub-agent` spans.** The orchestrator starts a `sub-agent` span outside any run when it launches a child, and stops it when it handles the child's hand-back (000074). A child's span is the orchestrator `sub-agent` span that was open when the child's first command ran; a span never stopped counts as open from its start on.
  - *Spawned after* runs from the start of the child's span to the child's first command.
  - *Picked up after* runs from the child's last command to the end of the child's span. If the span was never stopped, it runs to the orchestrator's next recorded command or span instead.
  - *Merged after* is unchanged: from the child's last command to the orchestrator's `epic merge` of its spec.

  A child with no orchestrator span shows no spawned-after or picked-up-after gap. The epic timeline keeps its layout, but these two gaps are not comparable with runs measured from status polls.

**Why this direction.** It satisfies the constraint that Harbor reads the logs through the run statistics report: the report is extended rather than bypassed, as 000075 anticipated. It also keeps one definition of every figure in Go, which is tested with golden files. A Python log reader would duplicate the span pairing and the interval rules, and would contradict the spec. Keeping verbose logging would contradict the default-level constraint. Changing `stats run` or `stats list` would break shapes 000075 pins and 000077 reuses. See `research.md#alternatives-considered-and-rejected`. The project's conventions shape the work:
- every new `stats` refusal names its fix (error-messages-must-suggest-remediation);
- the Python tests use fixed fixtures and their own temp directories (tests-must-not-depend-on-order);
- the plan touches no skill, template or settings, and installs nothing (plans-never-change-the-active-install);
- knowledge entries are updated through the CLI (store-files-must-be-written-through-the-cli).

**Review decision (2026-10-25, epic end-of-planning review).** The user decided that `spektacular status` is never logged (000073 marks it logging-exempt, as `stats` is) and that `SPEKTACULAR_SESSION_LOG` is removed entirely, with Harbor's verbose level set in the seeded test projects' configs instead. This plan therefore:
- sets those seeded configs back to the default level, rather than removing an image variable;
- measures the epic hand-off gaps from the orchestrator's `sub-agent` spans, since the `status <epic>` polls they were measured from are no longer recorded;
- retires the epic suite's check that the orchestrator polled `status <epic>`, the one verifier assertion the logs can no longer support.

## Component Breakdown

- **Run statistics package (changed, `spektacular` repo; created by 000075).** It gains two pure listings over the loaded logs. Assembly now keeps the records of traces outside any workflow run as well, instead of dropping them; nothing else about runs, reports, comparisons or summaries changes.
  - A **command listing** gives every command record, in time order, with the run and workflow it belongs to (or none), its step and the step it moved to, its argv, times, exit code and status message.
  - A **span listing** gives every agent-marked span (helper, user wait, work), paired and capped by the same rules as the run report, with its run (or none), step, task and parent.

  Both honour the existing run selection, and both have text renderers.
- **Stats command group (changed, `spektacular` repo; created by 000075).** It gains `stats commands` and `stats spans`, with the shared `--dir`, `--format`, selection flags, `--schema` and the JSON envelope, plus an `--outside-runs` flag that keeps only records written outside any run. They inherit the group's logging exemption and reuse its refusals.
- **Run log reader (new, Harbor shared helper).** It is the harness's only route to Spektacular's logs and replaces both the debug-log reader and 000074's span reader.
  - It runs the `stats` subcommands against a logs directory and parses their JSON into small typed records: runs, run reports, commands and spans.
  - It turns any failure into one reason, for the caller to report as unknown: no binary, no directory, a `stats` refusal, a timeout, or unparsable output.
  - It offers the views the harness needs: the runs of a workflow (a lane), its start, finish, resumes and children, the steps it entered, the orchestrator's commands and `sub-agent` spans outside any run, each child's orchestrator span, the orchestrator's next command or span after a child, merges, and changelog writes per repo.

  It caches each answer for the life of one reader, so a verifier asks `stats` once per question.
- **Metrics recorder (changed, Harbor).** It writes record schema 3:
  - run identity, check score and reward, unchanged;
  - the transcript totals (tokens, models, cost, turns, model time), unchanged;
  - from the run log reader: per workflow run, each step's duration, entries, working, helper-wait and user-wait time and helper count; the run's own waits and gaps; trial totals for user wait, helper wait and gaps; every helper with its spec, step, task, times and end. Wall time falls back to the logs only when the transcript has none;
  - child briefs (prompt, kind, description, models) from the transcript.

  A missing or unreadable log makes every log-sourced field unknown with the reader's reason, and no section can fail the record or touch the reward. Its transcript-based step windows, child placement, per-step output and idle time are removed.
- **Transcript readers (changed, Harbor).** They keep three groups of readers:
  - the token, model, cost and turn readers for Claude, ATIF and Codex, unchanged;
  - the child records, turns and wake-ups that the brief, wake-up and background checks read, and the background-child probe;
  - the launch and main-model readers the goal-check verifier uses.

  The message and character readers, the goto-from-transcript parser, the launch-root map and the transcript sub-agent counts are removed.
- **Delegation rules (changed, Harbor).** The hand-kept oracle of required, suggested and per-spec children stays unchanged.
  - The required-child rule judges each completed attempt of a required step from the helper spans that carry that step.
  - The per-spec-child rule judges each lane from the orchestrator's helper spans that were open when the lane started.
  - The brief rules still read prompts from the transcript. A review child is joined to its span by launch time; an epic child to its lane by the spec its prompt names; an implementing child's brief must name its worktree folder.
  - The wake-up and foreground rules keep reading the transcript's turns and children.
  - 000074's launch-to-span marking check moves onto the run log reader.
- **Epic timeline (changed, Harbor).** It prints the same report from the run log reader:
  - each lane's child start and finish and its slowest step (the step with the longest duration);
  - spawned-after and picked-up-after gaps from the orchestrator's `sub-agent` spans, and merged-after gaps from its `epic merge` commands, all outside any run;
  - resumed children and where each stopped;
  - Bob's turn-limit count from Bob's own logs.

  It works on a trial with no debug directory and says plainly when the logs are missing.
- **Cross-run report (changed, Harbor).** It reads schema-3 records:
  - The run table swaps the idle column for user-wait and gap columns. A run whose gaps exceed a quarter of its wall time is marked.
  - The one-run breakdown shows each step's split, its helpers, and the run's totals.
  - A new two-trial comparison shows each trial's per-step split and totals side by side, with the difference of each figure.
  - The suite medians gain user wait, helper wait and gap time.

  Older records still load and show the new figures as unknown.
- **Suite verifiers (changed, Harbor).** Every rule that built a session log asks the run log reader instead. These are the epic and implement-epic orchestration checks, the goal-check event rules, the plan steps-entered rule, the implement changelog-writes rule and the standing-rules command list. Their assertions are unchanged, except that the epic suite's check that the orchestrator polled `status <epic>` is removed: `status` is never logged (review decision, 2026-10-25). The spec suite's user-wait check (from 000074) reads the reader's spans.
- **Suite images and test scripts (changed, Harbor).** The configs of the test projects the suites seed no longer set a log level, so runs record at the default level. The timeline calls read `.spektacular/logs`. Every suite with a binary keeps collecting `.spektacular`, which carries the logs.
- **Harbor self-tests and fixtures (changed).**
  - The debug-log tests are replaced by run-log-reader tests over canned `stats` JSON (000075's golden files where they exist) and by one integration module that builds a throwaway binary and runs it over fixture logs.
  - Reference runs keep their transcripts, lose their old debug logs, and pin today's token, model and cost figures.
  - The reference trimmer copies new-format logs.
  - The wiring and README checks follow the new schema and the default-level seeded configs.
- **Documentation (changed).**
  - The README's Harbor sections describe the logs, schema 3, the split, the gaps and the two-trial comparison.
  - The docs site's Run statistics page gains the two listings.
  - The testing-architecture and Harbor-artifacts knowledge entries describe Harbor on `stats` at the default level, and the agent-log-fields gotcha names where the remaining readers live.

## Data Structures & Interfaces

**Input contract (read, unchanged).** Harbor reads 000075's `stats list` and `stats run` JSON exactly as 000075's plan fixes it, and adds nothing to them:
- durations are integer `*_ms`;
- times are RFC 3339 UTC;
- every response carries `log_dir` and `skipped_lines`;
- unfinished items carry `end_reason` and `complete`;
- a figure that does not apply is `null`.

**New `stats` subcommands (`spektacular` repo; the field names are a contract for Harbor).**

```text
spektacular stats commands [selection] [--outside-runs] [--dir <path>] [--format pretty|json]
spektacular stats spans    [selection] [--outside-runs] [--dir <path>] [--format pretty|json]
```

- The selection is 000075's: `--name`, `--kind`, `--agent`, and a repeatable `--run`.
- With no selection, a listing gives every record in the directory.
- With a selection, it gives only the records of the matching runs.
- `--outside-runs` gives only the records of traces with no workflow, and cannot be combined with a selection.
- Records come oldest first.
- Both subcommands support `--schema`, honour `--fields`, and refuse with 000075's codes. A selection combined with `--outside-runs` is `stats_bad_selection`.

*`stats commands`*:

```json
{"log_dir":"/abs/.spektacular/logs","skipped_lines":0,
 "commands":[{"run_id":"4bf9...","kind":"implement","name":"000075_run-stats-command","agent":"claude",
   "span_id":"00f067aa0ba902b7","parent_span_id":"a3ce929d0e0e4736",
   "command":"implement goto","args":["implement","goto","--data","{\"step\":\"update_plan\"}"],
   "step":"check_goal","step_to":"update_plan",
   "started_at":"...","ended_at":"...","duration_ms":23,"exit_code":0,"status":"ok","message":null}]}
```

- Outside a run, `run_id`, `kind` and `name` are `null`, and `step` and `step_to` are `null` when the record lacks them.
- `args` is `process.command_args`.
- `exit_code` is `process.exit_code`, or 0 or 1 from the status when that attribute is absent.
- `message` is the error message of a failed command, or `null`.

*`stats spans`*: the shape of 000075's `helpers` entries, with five more fields: `type` (`sub-agent`, `user-wait` or `work`), `trace_id`, `run_id`, `kind` and `workflow` (the workflow's name; `name` stays the span's name). Outside a run, the last three are `null`. Unfinished spans are capped, with `end_reason` and `complete`, exactly as in `stats run`.

```go
// internal/runstats (additions)
// Logs gains: Outside []tracelog.Record // records of traces with no workflow attributes
type CommandRow struct{ /* json tags as above */ }
type SpanRow struct{ /* 000075 helper fields + Type, TraceID, RunID, Kind, Name */ }
func (l *Logs) Commands(f Filter, outsideRuns bool) ([]CommandRow, error)
func (l *Logs) AgentSpans(f Filter, outsideRuns bool) ([]SpanRow, error)
func RenderCommands(w io.Writer, rows []CommandRow) error
func RenderSpans(w io.Writer, rows []SpanRow) error
```

**Run log reader (`tests/harbor/shared/runlog.py`, new).** This is the Python contract every Harbor consumer uses:

```python
class Unavailable(Exception): reason: str      # no binary / no dir / stats error / timeout / bad JSON

@dataclass
class Command:   # one `stats commands` row
    run_id: str | None; kind: str | None; name: str | None
    command: str; args: list[str]; step: str | None; step_to: str | None
    at: datetime; end: datetime; exit_code: int; message: str | None
    ok: bool                        # exit_code == 0
    def data(self) -> dict: ...     # the --data JSON, or {}
    def is_verb(self, *verb: str) -> bool: ...
    goto_step: str | None           # the --data step of a goto

@dataclass
class Span:      # one `stats spans` row (or a `stats run` helper)
    span_id: str; parent: str | None; type: str; name: str
    run_id: str | None; kind: str | None; spec: str | None   # spec = workflow name
    step: str | None; task_id: str | None
    start: datetime; end: datetime; end_reason: str; complete: bool; status: str | None

@dataclass
class Lane:      # every run of one workflow (kind, name), oldest first
    kind: str; name: str; runs: list[dict]          # `stats list` run summaries
    commands: list[Command]
    start: Command | None    # first successful `<kind> new`
    finish: Command | None   # first successful goto to `finished`
    last: Command | None     # the lane's last recorded command (the last child's last command)
    resumes: list[Command]   # failed `<kind> new` after the start
    children: int            # starts + resumes
    child_starts: list[Command]  # each child's first command: the start, then each resume or restart
    stops: list[str]         # each resume's `step`

class RunLog:
    def __init__(self, log_dir: Path, binary: str | None = None, runner=subprocess.run): ...
    def runs(self) -> list[dict]                     # stats list
    def report(self, run_id: str) -> dict             # stats run --run <id>, cached
    def commands(self) -> list[Command]               # stats commands, cached
    def spans(self) -> list[Span]                     # stats spans, cached
    def lanes(self, kinds=("plan", "implement")) -> dict[tuple[str, str], Lane]
    def outside(self) -> list[Command]                # commands with run_id None
    def orchestrator_spans(self) -> list[Span]        # `sub-agent` spans with run_id None
    def child_span(self, first: Command) -> Span | None   # orchestrator span open at a child's first command
    def orchestrator_reaction(self, lane: Lane) -> Command | Span | None  # next outside command or span after lane.last
    def merge(self, spec: str) -> Command | None
    def steps_entered(self, kind: str) -> list[str]   # step_to of each successful new/goto
    def changelog_writes_by_repo(self, name: str | None = None) -> dict[str | None, int]
```

- The binary is `$SPEKTACULAR_BIN`, or `spektacular` on `PATH`.
- Every call that cannot answer raises `Unavailable` with a reason that names the fix.
- `child_span(first)` is the earliest orchestrator span, not already matched to another child, that was open at `first.at`. Children are matched in the order of their first commands, across every lane, so children launched together take their spans in launch order. A span that was never stopped (`complete` false) counts as open from its start on, whatever its capped end.
- `orchestrator_reaction(lane)` is the first outside-run command, or orchestrator span start or stop, after `lane.last`. Commands naming only the lane's own spec are skipped, as today.

**Metrics record, schema 3 (`verifier/metrics.json`).** Only the changed sections are shown. `run` and `result` are unchanged, and the transcript totals are unchanged.

```json
{"schema_version":3,
 "totals":{"wall_time_s":512.3,"model_time_s":301.0,"turns":40,"cost_usd":0.91,
           "input_tokens":1200,"output_tokens":34000,"cache_read_tokens":900000,"cache_write_tokens":51000,
           "trace_calls":6,
           "user_wait_s":0.0,"helper_wait_s":140.2,"gap_s":0.0,"gaps":0,"gap_threshold_s":300},
 "steps":{"workflows":[{"run_id":"4bf9...","kind":"spec","name":"user-auth","status":"finished",
   "wall_s":498.1,"user_wait_s":0.0,"helper_wait_s":140.2,"gap_s":0.0,"gaps":0,
   "steps":[{"step":"verification","started_at":"...","entries":1,"duration_s":180.4,
             "working_s":40.2,"helper_wait_s":140.2,"user_wait_s":0.0,
             "commands":4,"helpers":1,"incomplete":0}]}]},
 "subagents":{"started":1,"completed":1,"failed":0,"unfinished":0,"durations_s":[140.2],
   "helpers":[{"span_id":"...","parent":"...","name":"spec-review","spec":"user-auth","kind":"spec",
               "step":"verification","task_id":null,"outside_run":false,
               "started_at":"...","ended_at":"...","duration_s":140.2,
               "end_reason":"stopped","status":"ok","complete":true}],
   "briefs":[{"id":"toolu_...","parent":null,"kind":"general-purpose","description":"...",
              "models":["claude-sonnet-5-5"],"prompt":"...","prompt_chars":1723}]},
 "unknown":[{"field":"steps","reason":"no Spektacular logs at /app/.spektacular/logs"}]}
```

- **Removed:** `totals.idle_s`, `steps.unit`, `steps.orchestrator_output`, `steps[].output`, `steps[].children` (renamed `helpers`), `subagents.stopped` and `subagents.children`. `subagents.unfinished` counts helpers that are not complete.
- **Steps.** One `workflows` entry per run, in start order, carrying 000075's per-step rows converted to seconds. `working_s` is `max(0, duration - helper wait - user wait)`. `helpers` counts the run's `sub-agent` spans whose `step` is that step.
- **Helpers.** An orchestrator helper written outside any run has `outside_run: true` and `step: null`. Its `spec` is the lane whose first run started while it was open, or `null`.
- **Briefs** come from the transcript: Claude in full, Bob with kind, description and prompt only, and `null` for Codex with an unknown reason.
- **Unknown fields.** A log-sourced field that cannot be read is `null`. It is listed in `unknown` with the run log reader's reason and is never filled from a transcript.

**Delegation oracle additions (`tests/harbor/shared/delegation.py`).** `REQUIRED`, `SUGGESTED` and `PER_SPEC_CHILD` are unchanged. Two hand-kept constants are added:
- `WORKTREE_DIR = ".spektacular/worktrees"`, tied by a self-check to `internal/worktree`'s `Dir`;
- `LAUNCH_SPAN_WINDOW_S = 120`, the furthest a launch may follow its `sub-agent` span's start.

`read_run(transcript, log_dir)` returns these fields:
- `children` and `turns`, `wakeups` and `end`, from the transcript;
- `log` (a `RunLog`), `spans` and `lanes`, from the run log reader.

**Cross-run report (`report.py`).** One new form:

```text
python3 report.py --compare <trial A> <trial B>
```

It prints each workflow's per-step table (duration, working, helper wait, user wait, helpers) for both trials, with B's difference from A. Then it prints the totals (wall, user wait, helper wait, gaps, cost, tokens) the same way. A step only one trial entered shows `-`, never 0. `--baseline/--candidate` medians gain the measures `user wait m`, `helper wait m` and `gap m`.

## Implementation Detail

**A narrow seam between Harbor and Spektacular.** Today, five Harbor modules and six verifiers each know the debug log's file naming, line format and state snapshots. After this change, one Python module knows how to call `stats` and what its JSON means, and everyone else gets typed records from it.
- The seam is a subprocess call with an injectable runner. Unit tests replace the runner with canned responses, so no test needs Docker or an installed binary.
- The canned responses are 000075's own golden JSON files where they exist. If Go renames a field, the golden file changes and the Python tests break with it, so the two sides cannot drift apart silently.
- The same idea covers the two new listings, whose golden files this plan adds.
- An integration module builds a throwaway binary from the working tree and runs the reader against fixture logs. That is the only place the two halves are run together.

**Domain rules move onto the reader, not into each verifier.** The epic rules today live on `SessionLog`:
- what counts as a lane, a start, a finish and a resume;
- what counts as an epic status poll or an orchestrator reaction.

They stay together on the run log reader, rewritten over `stats` commands, so the verifiers keep calling `lane.start`, `lane.finish`, `lane.children` and `log.merge(spec)` with the same meanings. The status poll is the exception: `status` is never logged, so `epic_polls()` goes, and the hand-off gaps that started or ended at a poll move onto the orchestrator's spans, found through `child_span` (review decision, 2026-10-25). A verifier diff is then mostly mechanical: build a `RunLog` from the logs directory instead of a `SessionLog`, and swap `state_before` for `step`. That keeps the change reviewable, and makes it plain that no other assertion changed (spec Non-Goals).

**The metrics recorder becomes an assembler.** Today `metrics.py` computes steps, placement and idle time itself, and branches per agent: it joins messages by timestamp for Claude, by goto order for Bob, and gives up for Codex. Afterwards, every log-sourced section is a conversion of `stats` output from milliseconds to seconds, keyed per run, with one per-step derivation (`working`), and has no agent branch at all. The transcript side shrinks to the totals readers and the brief records. The existing `gather` pattern, where each section is read on its own and its failure costs only its fields, is kept. It now has two kinds of source: transcript sections, and log sections that share one reader and one failure reason.

**Spans replace time windows for placement.** Placement used to be inferred by testing whether a launch time fell inside a step window built from state snapshots. Now it is recorded: a helper span carries its step and its parent. The required-child rule groups a lane's step entries into attempts, as runs of consecutive entries of the same step, so a resumed entry is not a new attempt. It then asks whether a complete attempt holds a span of that step. Only the brief checks still need to join the transcript to the logs, and they do it with one documented rule: a transcript launch matches the latest unused `sub-agent` span of the same run that started at most 120 s before it.

**Go follows 000075's layering.** The two listings are pure functions over `Logs`, beside the run report, with text renderers beside the others. The command layer adds two subcommands with the group's shared flags and error mapping. Assembly gains one bucket, `Outside`, for records it used to drop, and `Assemble`'s output for runs stays byte-for-byte the same, as the existing golden files prove.

**What a developer sees afterwards.** The six shared modules lose about a third of their lines, from about 3,000 (the spec's success metric):
- `sessionlog.py` (378 lines) and `spans.py` are gone;
- `runlog.py` takes their place at a fraction of the size;
- `metrics.py` loses placement, windows and idle time;
- `transcripts.py` loses the message and sub-agent-count readers;
- `timeline.py` becomes a printer over reader views.

The self-tests are organised the same way: reader tests over canned JSON, recorder tests over a fake reader, rule tests over hand-built spans and commands, and one integration module.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports none).
- **Upstream spec `000075_run-stats-command` must land first.** It is planned but not implemented. This plan builds on:
  - the `stats list` and `stats run` JSON contract and the golden files that pin it;
  - `internal/runstats` (`Load`, `Assemble`, `Run.Commands`, the span pairing and capping rules, `Filter`, the renderers);
  - `cmd/stats.go` (shared flags, error mapping, the logging-exempt group).

  It extends the package and the command group with two listings. No task starts until 000075's Milestones 1 and 2 are merged. If only names differ, adapt. If a field's meaning or a figure's definition differs, STOP and ask.
- **Upstream spec `000074_trace-spans-for-agent-work` must land first** (through 000075). This plan relies on:
  - its span records: step, task, `sub-agent` and `user-wait` spans, with parents and end reasons, and agent spans carrying `spektacular.step`;
  - orchestrator spans being written outside any run: the epic orchestrator starts a `sub-agent` span when it launches a child and stops it when it handles the hand-back, which is what the hand-off gaps are measured from.

  It replaces 000074's Harbor additions: `shared/spans.py`, the span checks in the verifiers, and the metrics trace-call count. It moves them onto the run log reader. The trace oracle constants and their self-check stay.
- **Upstream spec `000073_log-mode-and-logger-providers` must land first** (through 000074). This plan relies on:
  - its default-level record attributes (`process.command_args`, `process.exit_code`, `spektacular.step`, `spektacular.step.to`, status message);
  - run files and `standalone_<day>.jsonl` under `.spektacular/logs/`;
  - default retention of 10 runs;
  - the logging exemption it gives `status` (and `stats`), so an orchestrator's `status <epic>` polls never reach the logs.

  It removes 000073's Harbor normalising reader and the log-level line 000073 adds to the configs of the test projects the suites seed (000073 removes `SPEKTACULAR_SESSION_LOG` entirely).
- **Concurrent spec `000077_html-log-viewer`** (planned at the same time, not a dependency). It also extends `internal/runstats` and `cmd/stats.go`. The two plans add separate files and subcommands. Whichever lands second rebases onto the other's additions to the package and the command registration, and keeps both sets of golden files.
- **`internal/output`, `internal/tracelog` and `cobra` (used, unchanged).** They provide the JSON envelope and errors, the record type and constants, and the subcommands.
- **Harbor test harness (changed, Python 3, standard library only).** It covers `tests/harbor/shared/*`, every suite's verifier, `test.sh` and Dockerfile, and the shared self-tests under pytest. Its suites need Docker, the Harbor CLI and agent logins, and are not in CI.
- **Go toolchain (test-time only, optional).** The Python integration module builds a throwaway `spektacular` binary with `go build`, and skips when Go is unavailable.
- **The `spektacular` binary in each suite image (unchanged mechanism).** It is built from the working tree by the Makefile's render targets, so the verifier, which runs in the agent's container, has the new `stats` subcommands. The background-child image has no binary, and its record reports log measures as unknown.
- **Documentation site, `docs` repo (changed).** The Run statistics page `src/pages/run-stats.mdx` added by 000075 gains the two listings. It is checked by the site build and `astro check`.
- **Knowledge entries (changed through the CLI):** `architecture/testing-architecture.md`, `gotchas/harbor-agent-logs-and-artifacts.md` and `gotchas/harbor-agent-log-fields.md` in the `spektacular` repo store.
- **Investigation report `docs/workflow-performance-investigation.md` (read, unchanged).** It is the source of the per-step split and total-idle recommendations.

## Testing Approach

**Test types and where they slot in.** The Go side runs in the project's shuffled `go test`. The Harbor side runs under `pytest tests/harbor`: the shared self-tests plus each suite's own self-checks, run by hand as today, and not in CI. No test needs Docker, an agent login or an installed binary, and no test reads the wall clock.

- **Go unit and contract tests (`internal/runstats`).**
  - The command listing returns every command record across run files and day files, oldest first. Each record carries its run, workflow, step, `step_to`, argv, exit code and message. Records outside any run have null run fields.
  - The span listing returns helper, user-wait and work spans in runs and outside them, paired and capped exactly as the run report caps them.
  - The selection narrows both listings, and `--outside-runs` keeps only records with no workflow.
  - Assembly's run output is unchanged: 000075's golden files still match.
  - New golden JSON for each listing pins the field names Harbor reads.
- **Go command tests (`cmd`)**, through `runRootCmd`/`resetRootCmd` over a seeded fixture project:
  - both subcommands in JSON and pretty form, with `--fields`;
  - `--dir` on a copy outside a project;
  - a selection combined with `--outside-runs` refused with a next action;
  - every log file byte-identical before and after.
- **Python unit tests for the run log reader**, fed through its injectable runner with canned `stats` responses. 000075's golden files are used where they exist, so a Go rename breaks these tests too. They cover:
  - typed parsing of runs, reports, commands and spans;
  - every failure (no binary, no directory, a refusal, a timeout, bad JSON) as `Unavailable` with a reason;
  - lanes: start, finish, resumes as failed `<kind> new`, children, and stops from `step`;
  - each child's orchestrator span (including a resumed lane's later child, children launched together, and a span never stopped);
  - the orchestrator's next command or span after a child, merges, steps entered, and changelog writes per repo, each re-expressed from today's session-log rule tests.
- **Python recorder tests (metrics)**, over a fake reader and hand-built transcripts:
  - schema 3 keys;
  - per-step split arithmetic, with working never negative;
  - helpers with spec and step, including an orchestrator helper outside any run;
  - trial totals summed over runs;
  - a missing or broken log making every log-sourced field unknown with the reason, and nothing filled in from transcripts;
  - a failing section costing only its fields;
  - the reward never written.
- **Python rule tests (delegation and verifiers)**, over hand-built spans and commands:
  - a required step with and without a helper span, across attempts and a resumed entry;
  - a per-spec lane with and without an open orchestrator span;
  - brief checks joined by launch time, and the worktree-folder brief;
  - the oracle and worktree-constant self-checks against the templates and `internal/worktree`;
  - each suite verifier's log-reading rules re-run against fake readers, with their assertions unchanged apart from the retired status-poll check.
- **Python characterisation tests.** Before any transcript reader changes, the reference runs' token, model and cost figures are pinned as the current code computes them. After the change, the recorder over the same collected artifacts gives identical figures.
- **Python integration test.** It builds a throwaway binary from the working tree into a temp directory and runs the reader, the recorder and the timeline over fixture logs. Those cover a spec run with a helper and a user wait, and an epic trial with two lanes, an orchestrator's commands and `sub-agent` spans outside any run (one span never stopped) and a resumed lane. It checks that:
  - the record's steps equal `stats run` for the same directory;
  - the epic timeline lists both lanes with their gaps, measured from the orchestrator's spans;
  - a copied directory gives the same answers.

  It skips when Go is unavailable.
- **Report tests**:
  - the schema-3 table and breakdown;
  - the gap mark;
  - the two-trial comparison with per-step split, totals, differences, and a step only one trial entered shown as `-`;
  - medians of the new measures;
  - schema-2 records still loading.
- **Wiring tests**:
  - no suite's seeded test-project config sets a log level other than the default;
  - every timeline call reads `.spektacular/logs`;
  - every suite with a binary collects `/app/.spektacular`;
  - `metrics.py` stays the last, error-ignoring step after the reward;
  - the README metrics table matches schema 3.

**Load-bearing guarantees.**
- Every step, helper, wait, gap and timeline figure comes from `stats` over the logs alone, and equals what `stats` reports on the same directory.
- When the logs are missing, those figures are unknown with a reason, never zero, and never filled in from transcripts.
- Tokens, models and cost are unchanged.
- Gathering metrics never fails a run or changes its reward.
- No verifier assertion changes, only where it reads its data, except the epic suite's status-poll check, retired because `status` is never logged.

**Deliberate gaps.**
- No automated Harbor suite run: the suites need Docker and paid agent logins. Their acceptance criteria are manual checks.
- The reference runs keep their transcripts but lose their old-format debug logs. New-format reference logs need real runs, so the log side is covered by synthetic fixtures and the integration test.

**Success metrics.**
- *The harness's shared measurement scripts shrink by at least a third from about 3,000 lines.* **Behavioural test:** a self-test counts the lines of the shared Python modules (not their tests) and requires 2,000 or fewer, so the reduction is enforced and cannot quietly regrow.
- *For the same scenario, Claude, Codex and Bob records report the same set of steps and a helper count for every helper-starting step, with no agent-specific fields.* **Manual — captured in the implementation test plan.** Supporting behavioural test: the recorder over the same fixture logs, paired with a Claude transcript, an ATIF trajectory and a Codex rollout in turn, gives identical `steps` and `subagents.helpers` sections and the same key set, differing only in transcript totals and briefs.
- *No Harbor suite's pass rate drops, comparing 5 runs per suite for Claude on the harness before and after the change.* **Manual — captured in the implementation test plan.** The before runs must be collected on the harness as 000073 to 000075 leave it, before this plan's Harbor changes merge. A person-run task in Milestone 1 does that.

**Manual reviews.**
- **Manual — captured in the implementation test plan:** a Harbor implement run under Claude, Codex and Bob each yields a metrics record whose steps and step timings match `spektacular stats run` on the same trial's collected logs.
- **Manual — captured in the implementation test plan:** a Harbor implement run under Codex that starts helpers records a non-zero helper count, with each helper placed in its spec and step.
- **Manual — captured in the implementation test plan:** a Harbor spec run's record reports total user wait, total gap time and each step's working, helper-wait and user-wait time.
- **Manual — captured in the implementation test plan:** the epic timeline for a Harbor implement-epic trial lists each spec's child start and finish, its slowest step and both hand-off gaps, measured from the orchestrator's `sub-agent` spans, from a trial with no debug directory.
- **Manual — captured in the implementation test plan:** a collected trial holds `artifacts/spektacular/logs/`, and `spektacular stats run --dir` on that copy gives the trial's step timings.
- **Manual — captured in the implementation test plan:** removing a collected trial's logs and re-running `metrics.py` on it gives step, helper and wait measures unknown with a reason, an exit status of 0, and the same reward.
- **Manual — captured in the implementation test plan:** `report.py --compare` over two real trials shows each trial's per-step split and totals and their differences, and reads clearly.

## Milestones & Tasks

### Milestone 1: Spektacular's run report lists every command and helper, including the orchestrator's

**What changes**: `spektacular stats` gains two listings that give the raw facts behind the run report:
- `stats commands` lists every recorded command, with the run and step it ran in, the step it moved to, its arguments, timing, exit code and any error message;
- `stats spans` lists every helper agent, wait on the user and other marked span, with its run, step, task and outcome.

Both include what was recorded outside any workflow run, such as an epic orchestrator's `epic` commands and the helpers it launched. 000075 left those out, and Harbor needs them for epic timelines and per-spec checks. Both take the same run selection, `--dir` and `--format json` as the other `stats` commands, and neither writes to the logs. The documentation site describes them. Before anything in Harbor changes, a maintainer records the baseline Harbor pass rates, and the reference runs' token, model and cost figures are pinned as today's code computes them.

**Validation point**: Go unit, golden and command tests for both listings pass in the full shuffled suite, and 000075's golden files are unchanged. The site builds. The baseline jobs exist for every suite. The characterisation test passes on the current harness.

#### - [ ] Task: List commands and spans in the run statistics package
**Id:** 49fb2534-0645-4447-a50e-92865db63a87
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Teaches the run statistics package from 000075 to list two kinds of record:
- every recorded command, with the run, workflow and step it belongs to, the step it moved to, its arguments, timing, exit code and error message;
- every helper agent, wait on the user and other marked span, paired and capped by the same rules the run report uses.

Records written outside any workflow run, which 000075 drops, are now kept for these listings, so an epic orchestrator's commands and helpers are visible. The run report, comparisons and summaries are unchanged. This task starts only once 000075's package is merged.

*Technical detail:* [context.md#task-list-commands-and-spans-in-the-run-statistics-package](./context.md#task-list-commands-and-spans-in-the-run-statistics-package)

**Acceptance criteria**:
- [ ] Listing commands over a logs directory with two runs and a day of standalone commands gives every command, oldest first. Each carries its run, workflow, step, step moved to, arguments, timing, exit code and any error message, and a standalone command has no run.
- [ ] Listing spans gives every helper, user-wait and work span in runs and outside them, each with its run, step, task, parent and outcome. An unfinished span is capped and marked exactly as the run report marks it.
- [ ] A run selection narrows both listings to that run's records, and asking only for records outside runs gives just the orchestrator's.
- [ ] The run report, comparison and summary output is unchanged for the same logs.
- [ ] The structured form of both listings uses exactly the field names in the plan's Data Structures section.

#### - [ ] Task: List commands and spans from the command line
**Id:** 1510a2c7-a35f-49f9-9fd4-ae8854c980dc
**Repo:** spektacular
**Depends on:**
- 49fb2534-0645-4447-a50e-92865db63a87 — List commands and spans in the run statistics package
**Execution:** agent

Adds `spektacular stats commands` and `spektacular stats spans`. They take the same logs directory, run selection and output formats as the other `stats` commands, plus a switch for records outside any run. Both print a readable table or the standard JSON envelope. Like the rest of the group, they are never logged themselves, and every mistake is refused with a message saying what to do.

*Technical detail:* [context.md#task-list-commands-and-spans-from-the-command-line](./context.md#task-list-commands-and-spans-from-the-command-line)

**Acceptance criteria**:
- [ ] `stats commands` and `stats spans` show the fixture project's records in table and JSON form, with the same figures in both, and the JSON honours `--fields`.
- [ ] Pointing `--dir` at a copy of the logs from outside any project gives the same output as in the project.
- [ ] Combining a run selection with the outside-runs switch is refused with a message naming the fix, and the other refusals match the rest of the group.
- [ ] Every log file is byte-identical before and after both commands, run inside a project with logging on.

#### - [ ] Task: Documentation site describes the two listings
**Id:** 11b365fd-7097-426c-b440-623d7c2fd973
**Repo:** docs
**Depends on:**
- 1510a2c7-a35f-49f9-9fd4-ae8854c980dc — List commands and spans from the command line
**Execution:** agent

Adds a section to the documentation site's Run statistics page that explains `stats commands` and `stats spans`: what each lists, how to narrow them, how to see what an epic orchestrator did outside any run, and the fields their structured output carries.

**Content example** (new section on the Run statistics page, placed after "Summarising runs" and before "What each figure means"; the wording is illustrative, the commands, flags and field names are fixed):

1. Heading `## Listing commands and spans`.
2. Opening prose: "`spektacular stats commands` lists every command Spektacular recorded, oldest first: the run and step it ran in, the step it moved to, its arguments, how long it took and its exit code. `spektacular stats spans` lists every helper agent, wait on you and other marked span, with its step and how it ended."
3. A fenced example block:
   ```bash
   spektacular stats commands --name 000075_run-stats-command
   spektacular stats spans --kind implement --format json
   spektacular stats commands --outside-runs   # what an epic orchestrator ran
   ```
4. Closing prose: both take the same `--name`, `--kind`, `--agent`, `--run`, `--dir` and `--format` as the other commands; `--outside-runs` shows only what was recorded outside any workflow run and cannot be combined with a run selection; in JSON, commands carry `run_id`, `step`, `step_to`, `args`, `exit_code` and `message`, and spans carry `type`, `run_id`, `workflow`, `step`, `task_id`, `end_reason` and `complete`.

*Technical detail:* [context.md#task-documentation-site-describes-the-two-listings](./context.md#task-documentation-site-describes-the-two-listings)

**Acceptance criteria**:
- [ ] The Run statistics page explains both listings, how to narrow them, the outside-runs switch and their structured fields.
- [ ] The site builds and type-checks without errors, and the new text has no layout HTML and no em dashes.

#### - [ ] Task: Record baseline Harbor pass rates
**Id:** 9ce541c6-1a0c-4bfb-8ba7-b3b42e231ceb
**Repo:** spektacular
**Depends on:** none
**Execution:** human — needs Docker, the Harbor CLI and paid Claude agent logins that an agent does not have, and must run before this plan's Harbor changes merge

Runs each Harbor suite 5 times under Claude on the harness exactly as 000073 to 000075 leave it, before any Harbor change from this plan is merged. Keeps the collected jobs under a recognisable job-name prefix, so the success metric "no suite's pass rate drops" can later be compared before and after.

*Technical detail:* [context.md#task-record-baseline-harbor-pass-rates](./context.md#task-record-baseline-harbor-pass-rates)

**Acceptance criteria**:
- [ ] Five collected Claude runs exist for every Harbor suite, from a build that includes 000073 to 000075 and none of this plan's Harbor changes.
- [ ] Each suite's pass count out of 5 is noted where the later comparison will find it.

#### - [ ] Task: Pin today's token, model and cost figures
**Id:** 70054b85-d967-47e6-ac0d-e5c7a1741288
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds a characterisation self-test that records the token usage, models and cost the current harness computes from each checked-in reference run's transcript. The figures are pinned before any transcript reader changes, so later tasks can prove they leave them exactly as they were.

*Technical detail:* [context.md#task-pin-todays-token-model-and-cost-figures](./context.md#task-pin-todays-token-model-and-cost-figures)

**Acceptance criteria**:
- [ ] For every reference run, the self-test pins the token counts, model, models used and cost as today's code computes them, and passes on the unchanged harness.
- [ ] The pinned values come from the transcripts alone and do not depend on Spektacular's logs.

### Milestone 2: Harbor measures and judges every run from Spektacular's own logs

**What changes**: Every Harbor run's metrics record takes its workflow steps and their timings from Spektacular's own logs, for Claude, Codex and Bob alike. So do the helper agents started (with the spec and step each was started in), the total time waiting on the user, the total gap time, and each step's split into working, helper-wait and user-wait time. These replace the single longest gap. Tokens, models and cost still come from the agent's transcript, unchanged. When the logs are missing, these measures say unknown and why. Gathering metrics still never fails a run or changes its reward. The delegation checks judge whether a required helper was started, and in which spec and step, from helper spans. Every suite verifier that read the old session log reads the logs through `stats`, with its pass and fail rules unchanged, apart from the epic suite's status-poll check, retired because `status` is never logged. The test projects the suites seed record at Spektacular's default log level with no special setting, and each trial's artifacts carry the logs.

**Validation point**: The run-log reader, recorder, rule and wiring self-tests pass under `pytest tests/harbor`, together with the integration test against a throwaway binary. That test shows the record's steps equal to `stats run` on the same logs, and a missing log giving unknown measures. The characterisation test still passes with identical token figures.

#### - [ ] Task: Harbor run log reader
**Id:** db80ecd4-547e-4e8b-8845-dea9901a726a
**Repo:** spektacular
**Depends on:**
- 1510a2c7-a35f-49f9-9fd4-ae8854c980dc — List commands and spans from the command line
**Execution:** agent

Adds the one shared helper through which Harbor reads Spektacular's logs. It runs the `stats` commands against a logs directory and turns their JSON into typed runs, reports, commands and spans. On top of those it offers the views the harness needs: each workflow's lane with its start, finish, resumes and children; the steps entered; the orchestrator's commands and helper spans outside any run; the orchestrator span each child was launched under, the orchestrator's reactions and merges; and changelog writes per repo. Any failure comes back as one reason the caller can report, never as an empty answer.

*Technical detail:* [context.md#task-harbor-run-log-reader](./context.md#task-harbor-run-log-reader)

**Acceptance criteria**:
- [ ] Given `stats` output for an epic trial, the reader shows each lane's start, finish, children and the step each earlier child stopped at, with a resume counted from a failed workflow start in that lane.
- [ ] Each child is matched to the orchestrator helper span open when it began, including a resumed lane's later child, children launched together and a span never stopped, and the orchestrator's next command or span after a finished lane is found.
- [ ] Merges, the steps a plan entered and changelog writes per repo match what the old session-log rules gave for the same run.
- [ ] A missing binary, a missing logs directory, a `stats` refusal, a timeout or unreadable output each produce a stated reason rather than an empty result.
- [ ] The reader reads the logs only through `stats`, and an integration test with a freshly built binary over fixture logs gives the same answers as the canned responses.

#### - [ ] Task: Metrics record from Spektacular's logs
**Id:** 706125e3-2526-433c-8cd3-4ee71d792436
**Repo:** spektacular
**Depends on:**
- db80ecd4-547e-4e8b-8845-dea9901a726a — Harbor run log reader
- 70054b85-d967-47e6-ac0d-e5c7a1741288 — Pin today's token, model and cost figures
**Execution:** agent

Moves every run's metrics record to schema 3. It now takes the following from Spektacular's logs, the same way for every agent:
- the steps and their timings;
- each step's working, helper-wait and user-wait time and its helper count;
- every helper with the spec and step it was started in;
- the total user-wait, helper-wait and gap time.

It keeps the transcript's tokens, models, cost and child briefs. The transcript-based step windows, child placement, per-step output and longest-gap idle time are removed. A missing or unreadable log leaves those measures unknown with the reason, and the record is still written without touching the reward. The README's run metrics table follows the new schema.

*Technical detail:* [context.md#task-metrics-record-from-spektaculars-logs](./context.md#task-metrics-record-from-spektaculars-logs)

**Acceptance criteria**:
- [ ] For a trial whose logs hold a spec run with a helper and a user wait, the record's steps and step timings equal the run statistics report for those logs, and each step shows working, helper-wait and user-wait time adding up to its duration.
- [ ] Every helper is listed with its spec and step, including one an epic orchestrator started outside any run, and each step shows how many helpers it started.
- [ ] The record reports total user-wait, helper-wait and gap time in place of the single longest gap.
- [ ] With the logs removed, every step, helper and wait measure is unknown with a reason, none is filled in from the transcript, the recorder exits successfully, and the reward is untouched.
- [ ] Token usage, models and cost equal the pinned figures for every reference run.
- [ ] The same logs paired with a Claude, a Bob and a Codex transcript give identical steps and helper sections.

#### - [ ] Task: Delegation checks judge from spans
**Id:** 86f58e38-51a5-47ef-bf2b-a9927f501170
**Repo:** spektacular
**Depends on:**
- db80ecd4-547e-4e8b-8845-dea9901a726a — Harbor run log reader
**Execution:** agent

Points the delegation rules at helper spans:
- A required step passes when a completed attempt of it started a helper, as recorded in Spektacular's logs.
- An epic lane passes when the orchestrator had a helper running as the lane started.

The hand-kept list of which steps require a helper is unchanged. The checks on what a helper's brief contained still read the transcript, joined to the logs by launch time. An implementing child's brief must name its spec's worktree folder. The wake-up and foreground rules are unchanged.

*Technical detail:* [context.md#task-delegation-checks-judge-from-spans](./context.md#task-delegation-checks-judge-from-spans)

**Acceptance criteria**:
- [ ] A run whose required step started a helper passes, and one whose required step completed without a helper span fails, naming the workflow, spec, step and attempt.
- [ ] A resumed step is still judged as one attempt, and the step a lane ended on is not judged.
- [ ] An epic lane with no orchestrator helper running when it started fails, naming the spec.
- [ ] A spec review child whose brief lacks the staged spec, and an implementing child whose brief lacks its worktree folder, are still reported.
- [ ] Self-checks fail if the worktree-folder constant or the required-step list drifts from the code and templates they mirror.

#### - [ ] Task: Suite verifiers read the logs through stats
**Id:** 251726e2-dd95-44de-ac4f-9855da375968
**Repo:** spektacular
**Depends on:**
- db80ecd4-547e-4e8b-8845-dea9901a726a — Harbor run log reader
- 86f58e38-51a5-47ef-bf2b-a9927f501170 — Delegation checks judge from spans
**Execution:** agent

Moves every suite verifier that built the old session log onto the run log reader, keeping each assertion as it is. That covers:
- the epic and implement-epic orchestration checks (lanes, one child per spec, ordering, parallelism, merges, worktrees, the goal record before each plan update);
- the goal-check event rules;
- the plan suite's walkthrough check;
- the implement suite's changelog writes;
- the standing-rules command list.

The span checks 000074 added (each launch marked, the scripted question marked as a user wait) move onto the reader too. The epic suite's check that the orchestrator polled `status <epic>` is removed, because `status` is never logged and the logs cannot show it (review decision, 2026-10-25).

*Technical detail:* [context.md#task-suite-verifiers-read-the-logs-through-stats](./context.md#task-suite-verifiers-read-the-logs-through-stats)

**Acceptance criteria**:
- [ ] No suite verifier or verifier self-check imports the old session-log reader or reads `.spektacular/debug`.
- [ ] Every check that read a response or a state snapshot now reads a fact recorded at the default log level, and its pass and fail outcomes are unchanged on the self-check fixtures.
- [ ] The epic suite no longer checks for `status <epic>` polls, and no verifier reads a `status` command from the logs.
- [ ] Each suite's self-check passes against fake readers built from canned `stats` output.

#### - [ ] Task: Suite images record at the default level
**Id:** 581898a1-0209-4fd4-8202-e45182d2c69a
**Repo:** spektacular
**Depends on:**
- 706125e3-2526-433c-8cd3-4ee71d792436 — Metrics record from Spektacular's logs
- 251726e2-dd95-44de-ac4f-9855da375968 — Suite verifiers read the logs through stats
**Execution:** agent

Removes the verbose log level that 000073 sets in the config of every test project the suites seed, so each run records at Spektacular's default level with no special setting. Each trial's artifacts carry the logs through the existing collection of the project's `.spektacular`. The wiring self-test pins all three: no seeded config sets a log level other than the default, every suite that runs Spektacular collects its logs, and metrics stays the last, failure-ignoring step after the reward.

*Technical detail:* [context.md#task-suite-images-record-at-the-default-level](./context.md#task-suite-images-record-at-the-default-level)

**Acceptance criteria**:
- [ ] No suite's seeded test-project config sets a log level other than the default, and the wiring self-test fails if one does.
- [ ] Every suite that runs Spektacular collects the project's `.spektacular`, so its logs come back in the trial's artifacts.
- [ ] Metrics gathering is still the last step after the reward, and its failure is ignored.

### Milestone 3: Harbor's timeline and cross-run reports use the new measures, and the old parsers are gone

**What changes**:
- The epic timeline reports from Spektacular's logs: each spec's child start and finish, its slowest step, the hand-off gaps, and resumed children. It works for a trial with no old debug directory.
- The cross-run report shows each run's per-step split and totals. A new two-trial comparison shows the difference in every per-step figure and total, and the suite medians include user wait, helper wait and gap time.
- The old debug-log reader, the transcript-based step, helper-placement and idle code, and the old-format reference logs are removed, leaving one source for each measure.
- The shared measurement scripts end at two-thirds of their size or less.
- The README's Harbor sections and the project's knowledge entries describe the new arrangement.

**Validation point**: The timeline and report self-tests, the integration test and the line-count self-test pass. No shared module or verifier imports the old reader. The full `pytest tests/harbor` and Go suites pass. The manual Harbor checks in the test plan are then run.

#### - [ ] Task: Epic timeline from Spektacular's logs
**Id:** fc67375e-111c-4c2d-9b03-0bba26bde5b0
**Repo:** spektacular
**Depends on:**
- db80ecd4-547e-4e8b-8845-dea9901a726a — Harbor run log reader
**Execution:** agent

Rebuilds the epic timeline on the run log reader. It prints:
- each spec's child start and finish, step count, children and slowest step;
- the spawned-after and picked-up-after gaps, from the helper span the orchestrator started for each child and stopped when it handled the hand-back;
- the merged-after gap, from the orchestrator's `epic merge`;
- the resumed children and where they stopped;
- Bob's turn-limit count.

Spawned after runs from the start of the child's helper span to the child's first command. Picked up after runs from the child's last command to the end of that span, or to the orchestrator's next recorded command or span when the span was never stopped. The suites' test scripts point it at `.spektacular/logs`. It reports plainly when the logs are missing, and never fails.

*Technical detail:* [context.md#task-epic-timeline-from-spektaculars-logs](./context.md#task-epic-timeline-from-spektaculars-logs)

**Acceptance criteria**:
- [ ] For a fixture implement-epic trial with no debug directory, the timeline lists each spec's child start and finish, its slowest step and both hand-off gaps, measured from the orchestrator's helper spans, and the merge gap.
- [ ] A child whose orchestrator helper span was never stopped takes its picked-up gap from the orchestrator's next recorded command or span, and a child with no orchestrator span shows no spawned-after or picked-up-after gap.
- [ ] A resumed lane shows its extra child and the step the earlier one stopped at.
- [ ] With the logs missing, it prints why and exits successfully.

#### - [ ] Task: Cross-run report shows the per-step split
**Id:** b3638e0c-26e0-4c89-bc06-f086d7ca64e8
**Repo:** spektacular
**Depends on:**
- 706125e3-2526-433c-8cd3-4ee71d792436 — Metrics record from Spektacular's logs
**Execution:** agent

Teaches the cross-run report to read schema 3:
- The run table shows user-wait and gap time and marks runs dominated by gaps.
- One run's breakdown shows each step's split, its helpers and the run's totals.
- A new two-trial comparison shows both trials' per-step split and totals side by side, with the difference of every figure.
- The suite medians gain user wait, helper wait and gap time.

Older records still load and show the new figures as unknown.

*Technical detail:* [context.md#task-cross-run-report-shows-the-per-step-split](./context.md#task-cross-run-report-shows-the-per-step-split)

**Acceptance criteria**:
- [ ] Comparing two trials shows each trial's per-step working, helper-wait and user-wait time and totals, with the second trial's difference from the first. A step only one trial entered shows as absent, not zero.
- [ ] One run's breakdown shows its per-step split and helpers, and the run table shows user-wait and gap time.
- [ ] Before-and-after medians include user wait, helper wait and gap time.
- [ ] A schema-2 record still loads, with the new figures shown as unknown.

#### - [ ] Task: Remove the old parsers and debug fixtures
**Id:** a49f5e7b-8b38-4224-a665-b462ecfc2c64
**Repo:** spektacular
**Depends on:**
- 706125e3-2526-433c-8cd3-4ee71d792436 — Metrics record from Spektacular's logs
- 86f58e38-51a5-47ef-bf2b-a9927f501170 — Delegation checks judge from spans
- 251726e2-dd95-44de-ac4f-9855da375968 — Suite verifiers read the logs through stats
- fc67375e-111c-4c2d-9b03-0bba26bde5b0 — Epic timeline from Spektacular's logs
**Execution:** agent

Deletes what the new reader replaced:
- the old debug-log reader and 000074's span reader, with their tests;
- the transcript readers nothing uses any more;
- the old-format debug logs in the reference runs.

The reference trimmer learns to copy new-format logs from a collected trial. A self-test then holds the shared measurement scripts to two-thirds of their former size, so each measure has one source and the harness stays small.

*Technical detail:* [context.md#task-remove-the-old-parsers-and-debug-fixtures](./context.md#task-remove-the-old-parsers-and-debug-fixtures)

**Acceptance criteria**:
- [ ] The old session-log reader and span reader no longer exist, and nothing in the harness refers to them or to `.spektacular/debug`.
- [ ] The reference runs keep their transcripts and pinned token, child and turn figures, and no longer carry old debug logs.
- [ ] Trimming a collected trial keeps its Spektacular logs and leaves every log-sourced measure unchanged.
- [ ] The shared measurement scripts total 2,000 lines or fewer, and a self-test fails if they grow past that.
- [ ] The full Harbor self-test suite and the Go test suite pass.

#### - [ ] Task: README and knowledge describe Harbor on built-in logs
**Id:** c855d276-ed7e-46bc-9418-3e426dac25e6
**Repo:** spektacular
**Depends on:**
- 581898a1-0209-4fd4-8202-e45182d2c69a — Suite images record at the default level
- b3638e0c-26e0-4c89-bc06-f086d7ca64e8 — Cross-run report shows the per-step split
- a49f5e7b-8b38-4224-a665-b462ecfc2c64 — Remove the old parsers and debug fixtures
**Execution:** agent

Brings the project's own descriptions up to date. The README's Harbor sections describe:
- the collected logs and the default level;
- reading them with `stats`;
- the epic timeline, with hand-off gaps from the orchestrator's helper spans;
- the two-trial comparison;
- the self-tests, including the integration test's need for Go.

The testing-architecture knowledge entry says Harbor reads Spektacular's logs only through `stats` at the default level, and names the new hand-kept constants. The Harbor artifacts gotcha drops the session-log override and says the seeded test projects record at the default level. The agent-log-fields gotcha says which readers remain.

*Technical detail:* [context.md#task-readme-and-knowledge-describe-harbor-on-built-in-logs](./context.md#task-readme-and-knowledge-describe-harbor-on-built-in-logs)

**Acceptance criteria**:
- [ ] The README's Harbor sections name `.spektacular/logs`, the default level, `stats` as the source, the timeline command, the two-trial comparison and how to run the self-tests, and no longer describe the debug log or `idle_s`.
- [ ] The three knowledge entries read back through the knowledge CLI with the new arrangement and no current-tense mention of the debug session log or the override.

## Open Questions

- **Whether 000073 to 000075 land with the record attributes and `stats` output this plan reads.** This depends on their implementations. The plan reads:
  - at the default level: `process.command_args`, `process.exit_code`, `spektacular.step`, `spektacular.step.to` and the status message;
  - `stats list` and `stats run` field names and units;
  - `internal/runstats`' `Logs`, `Run.Commands`, span pairing and `Filter`.

  If only names differ, adapt to them. If a field's meaning or a figure's definition differs, or the default level drops an attribute listed here, STOP and ask the user. The record's measures and the verifier rules rest on them.
- **Whether a lane resume shows in the lane's run as a failed `<kind> new` carrying its step.** This only shows once 000073's run attribution is running against a real orchestrated resume. If the refusal is filed elsewhere, or carries no step, count resumes from the lane's step entries that ended `interrupted`, and take the stop from that entry's step. If neither records a resume, STOP and ask the user before changing the one-child-per-spec checks.
- **Whether real agents mark enough helper launches as spans for the span-judged checks to keep their pass rates.** This depends on 000074's instructions working in practice. It surfaces in the manual Harbor runs and the before-and-after pass-rate comparison. If a suite's pass rate drops because spans are missing rather than helpers, STOP and ask the user: the fix lies in 000074's instructions or in the checks' data source, and either changes a recorded decision. The epic hand-off gaps depend on the same marking: if real epic trials show children with no orchestrator span, or spans the orchestrator never stops, report it rather than adding another source for the gaps.
- **Whether the verifiers' own CLI calls reach the logs once `SPEKTACULAR_SESSION_LOG` is gone.** The verifiers' `cli()` helpers set it to `off` today to keep their calls out of the agent's log, and 000073 removes the variable. Their `status` calls are exempt, but their other calls (such as `epic list`) would become outside-run records after the agent's last command. Follow whatever 000073's revised plan does for these calls. If they do reach the logs and change a verifier's or the timeline's answer (for example, becoming the orchestrator's next command after a child), STOP and ask the user.
- **Whether any suite starts more than 10 workflow runs in one trial**, which would let default retention prune a run before the verifier reads it. This only shows in real runs. If a collected trial is missing a run it started, STOP and ask the user. Raising retention would be a special setting, which the spec rules out.
- **Whether the shared scripts reach the 2,000-line ceiling.** This only becomes measurable once the rewrites exist. If the line-count self-test fails after every planned removal, STOP and ask the user before loosening the ceiling or dropping a report feature.

No other implementation-time uncertainties remain. Every design choice is settled in Architecture & Design Decisions, Data Structures and the assumption log.

## Out of Scope

- **New Harbor suites**, such as a single-spec implement suite with real tasks (spec Non-Goals; recommended in `docs/workflow-performance-investigation.md`).
- **Changing any suite's scenarios or its pass and fail checks** beyond where they read their data (spec Non-Goals). Span-judged checks stay in the Claude-gated classes. Running them for Codex and Bob is a later decision.
- **A visual view of runs.** That is spec `000077_html-log-viewer`.
- **Tokens, models and cost from Spektacular's logs.** The logs cannot see them, so they stay with the transcripts (spec Constraints). Model time per step is not added either.
- **Per-step output characters and transcript sub-agent counts in the metrics record.** They are dropped, not replaced, because they are transcript-based and agent-specific.
- **Linking an orchestrator's helper span to the child's own run.** Orchestrator helpers get a spec, and each child its hand-off span, by time only (000074 Out of Scope).
- **New-format reference runs.** Recording real trials to replace the old-format debug logs in the checked-in reference runs needs paid agent runs. The trimmer is ready for them, but none is recorded here.
- **Regrading collected trials in a separate verifier container** (`harbor trial regrade`). The verifier still runs in the agent's container, which has the binary.
- **Changing the run report's figures, its 5-minute gap threshold, retention or the log levels.** Those belong to 000073 to 000075. Harbor uses their defaults.
- **Installing the new binary or touching this repository's own `.spektacular/`** (plans-never-change-the-active-install). Harbor suite runs are manual.
