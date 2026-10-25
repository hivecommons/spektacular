---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Context: 000075_run-stats-command

## Current State Analysis

- **There is no report over the logs today.** Timings are pieced together by hand from transcripts and Harbor scripts:
  - `tests/harbor/shared/metrics.py:277-557` gives the step breakdown, child placement and longest-gap idle;
  - `tests/harbor/shared/report.py` gives cross-run medians;
  - `tests/harbor/shared/timeline.py` gives epic lane timelines;
  - `docs/workflow-performance-investigation.md` used one-off scratch scripts.
- **The logs this report reads do not exist yet.**
  - 000073 (planned) replaces the debug session log (`cmd/root.go:110-176`, `internal/sessionlog`) with `internal/tracelog`. It writes one `span` record per command to `.spektacular/logs/<started>_<agent>_<kind>_<name>.jsonl`, or `standalone_<YYYYMMDD>.jsonl`, keyed by the run's trace id (`workflow.State.RunID`). It resolves the directory with `config.LogDir(root)`.
  - 000074 (planned) adds `span_start`/`span_end` records for steps, tasks, `sub-agent`, `user-wait` and `work` spans, with end reasons, parents and task ids.
- **CLI conventions to follow.**
  - Group commands return `unknown_subcommand` when bare (`cmd/root.go:42-68`).
  - People-first reports take `--format pretty|json` with pretty as the default (`cmd/status.go:22-26,173-179`; `cmd/implement_goal.go:393-399`).
  - `--schema` uses `commandSchema` (`cmd/spec.go:34-55`).
  - JSON goes through `output.New(w, globalFields).WriteResult` (`internal/output/writer.go:22-140`), which injects `"error": false` and applies `--fields`.
  - Refusals are `output.NewError(...).WithNextAction(...)`.
- **The gate and logging.** The gate does nothing outside a project (`cmd/gate.go:30-45`) and exempts commands by cobra annotation (`cmd/gate.go:120-135`, `cmd/init.go:21`). `runRoot` logs every command run in a project, and 000073 keeps that, so a `stats` call would append to the day's log unless exempted.
- **Test helpers.** `resetRootCmd` resets slice flags correctly (`cmd/root_test.go:45-62`). `runRootCmd` drives `runRoot` (`cmd/root_test.go:70-76`). `writeCurrentConfig` writes a current-format project (`cmd/root_test.go:93`).
- **Docs.** `docs:src/pages/debugging.mdx` (rewritten by 000073 and extended by 000074) uses `Hero`, `Prose` and `CtaBanner`. `docs:src/components/Nav.astro:16-25` lists the Resources pages.

**Requirement to repo and files**

| Requirement | Repo | Files |
|---|---|---|
| List runs; choose runs | spektacular | `internal/runstats/{load,assemble,select}.go`, `cmd/stats.go` |
| Run totals; steps; tasks; helpers; gaps; unfinished work flagged | spektacular | `internal/runstats/{report,intervals,gaps,render}.go` |
| Compare runs; summarise runs | spektacular | `internal/runstats/{compare,summary,render}.go`, `cmd/stats.go` |
| Structured and table output | spektacular | `cmd/stats.go`, `internal/runstats/render.go` |
| Logs from elsewhere | spektacular | `cmd/stats.go` (`--dir`), `internal/runstats/load.go` |
| Damaged logs do not stop the report | spektacular | `internal/runstats/load.go` |
| The report never changes the logs | spektacular | `internal/runstats/load.go` (read-only), `cmd/root.go` (logging exemption), `cmd/stats.go` (annotation) |
| Documentation describes the report | docs, spektacular | `docs:src/pages/run-stats.mdx`, `docs:src/components/Nav.astro`, `docs:src/pages/debugging.mdx`, `README.md` |

## Per-Task Technical Notes

### Task: Load logs into runs

**File changes**
- `internal/runstats/doc.go` (new): the package comment. It is the read side of `internal/tracelog`: pure analysis between a read-only loader and a text renderer, and it never writes to the logs directory.
- `internal/runstats/load.go` (new): `Load(dir string) (*Logs, error)`.
  - `os.ReadDir(dir)`. A missing directory gives `ErrNoLogDir` (wrapping `fs.ErrNotExist`); any other read error is returned.
  - It reads every regular `*.jsonl` file, sorted by name, and skips `.gitignore` and directories.
  - Each file is opened with `os.Open` (read-only) and read with `bufio.Reader.ReadBytes('\n')`, so no line length is capped. A final line without `\n` is still parsed.
  - Each line goes through `json.Unmarshal` into `tracelog.Record`. A line is skipped and counted in `Logs.Skipped` when it does not parse, has an empty `TraceID`, has an empty or unknown `Record` (not `span`, `span_start` or `span_end`), has a zero `StartTime`, or is a `span_end` with a zero `EndTime`. Blank lines are skipped without counting.
  - It remembers each record's file base name for `Run.LogFile`, the file of the run's earliest record.
  - It then calls `Assemble`.
- `internal/runstats/assemble.go` (new): `Assemble(recs []tracelog.Record) []*Run` (pure). It also keeps a package-internal variant that carries file names.
  - Group by `TraceID`. A group is a run when any record has a non-empty `spektacular.workflow.kind` attribute. `Kind`, `Name` and `Agent` come from the first record carrying them (`spektacular.agent`, falling back to `unknown`).
  - Commands are `record == "span"` with `spektacular.span.type` `command` or absent. They are sorted by `StartTime`.
  - Pairing: a map from `span_id` to `{start *Record, end *Record}` for `span_start`/`span_end`. A span with only an end uses the end's own `StartTime`. `Span` takes `Type` from `spektacular.span.type`, `Step` from `spektacular.step`, `TaskID` from `spektacular.task.id`, `Parent` from `ParentSpanID`, and `EndReason`, `Status` and `Message` from the end. With no end, `EndReason = "open"`. `Complete` is true for `moved`, `finished` and `stopped`.
  - The timeline (`[]Event`) holds:
    - each command as `{At: start, Until: end, Edge: "end", Type: "command", Name}`; the label used after a gap is its end, and the label used before a gap is its start;
    - each span start as `{At: start, Edge: "start"}`;
    - each span end whose `EndReason != "abandoned"` as `{At: end, Edge: "end"}`.

    It is sorted by `At`, then commands before instants, then by name, for determinism.
  - `Run.Start` is the minimum `At`. `Run.End` is the maximum of `At` and `Until`. Wall time is `End - Start`.
  - Effective ends: for a span that is `open` or `abandoned`, `End = min(recorded end or Run.End, Run.End)`, and never before `Start`. `interrupted` spans keep their recorded end, which 000074 sets to the last command.
  - Status:
    - `finished` if any step span ended with `finished`, or any step span is named `finished`, or any command has `spektacular.step.to == "finished"`;
    - else `interrupted` if any step span ended `abandoned`, or another run with the same `Kind` and `Name` has `Start > this.End`;
    - else `in_progress`.

    The second rule needs every run, so it is a final pass in `Assemble`.
  - Sort runs by `Start` descending, ties by `ID`.
- `internal/runstats/select.go` (new):
  - `Filter{Name, Kind, Agent string; Runs []string}`.
  - `Kinds = []string{"spec","plan","implement","repo"}`. An unknown kind gives `*SelectionError{Code:"bad_kind"}`.
  - `(*Logs).Candidates(f)`:
    - validates each `--run` against `^[0-9a-f]{8,32}$` (`bad_run_id`) and resolves it by prefix over all runs: 0 matches gives `run_not_found`, more than 1 gives `run_ambiguous` with the matching ids;
    - when runs are named, the candidates are those runs in the given order, filtered by the other fields;
    - otherwise the candidates are every run matching `Name`, `Kind` and `Agent` (exact matches), newest first.
  - `ForReport(c)`: `c[0]`, or `ErrNoRuns`.
  - `ForCompare(c, f)`:
    - with two or more named runs, returns `c`, which is `ErrTooFew` if filtering left fewer than 2;
    - with exactly one named run, returns `ErrTooFew`;
    - otherwise it keeps the candidates with `Kind == c[0].Kind`, takes the first two, reverses them (oldest first), and returns `ErrTooFew` if fewer than 2.
  - `ForSummary(c, f)`: named runs as given; otherwise every candidate of `c[0].Kind`, oldest first. `ErrNoRuns` when there are none.
- `internal/runstats/load_test.go`, `assemble_test.go`, `select_test.go` (new): table tests over records built by a test helper (`rec(kind, traceID, spanID, name, start, end, attrs)` with fixed `time.Date` values), plus temp-dir files for the loader:
  - a malformed line, a missing trace, an unknown record kind, a truncated last line, and a 2 MB line;
  - grouping across a run file and a day file;
  - a standalone trace excluded;
  - an end with no start;
  - an abandoned restart end that does not stretch the wall time;
  - each status rule;
  - each selection and error;
  - the default per mode;
  - a loader test that hashes every file before and after `Load` and finds them equal.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) loader and assembly with their tests; (b) selection with its tests, against a stub `[]*Run`. Integrate sequentially.

### Task: Report on one run

**File changes**
- `internal/runstats/intervals.go` (new): `type interval struct{ from, to time.Time }`, `union([]interval) []interval` (sort and merge overlapping and touching intervals), `clip([]interval, interval) []interval`, `total([]interval) time.Duration`. These are pure.
- `internal/runstats/report.go` (new): `Options{Gaps int; GapThreshold time.Duration}` and `DefaultOptions = {10, 5*time.Minute}`. `BuildReport(r *Run, o Options) Report`. Its parts:
  - **Helper and user-wait unions.** `helpers` is `union` of the intervals of complete spans with `Type == "sub-agent"`; `waits` is the same for `user-wait`.
  - **`Totals`:**
    - `WallMS` is `Run.End - Run.Start`;
    - `UserWaitMS` is `total(waits)` and `HelperWaitMS` is `total(helpers)`;
    - `WorkingMS` is `WallMS - UserWaitMS`;
    - `CommandMS` is the sum of command durations;
    - `Commands` is the number of command records;
    - `Helpers` and `UserWaits` count every span of those types;
    - `GapMS` and `GapsOverThreshold` come from the gap pass.
  - **Step entries.** Every span with `Type == "step"`, in start order. Each entry carries:
    - `step` (span name), `span_id`, and `task_id` (the parent task span's `TaskID`, or null);
    - `started_at`, `ended_at` and `duration_ms`, from the effective end;
    - `end_reason` and `complete`.
  - **Steps.** These group the entries by name, in order of first entry:
    - `entries`, `duration_ms` (sum) and `incomplete` (entries not ending in `moved`/`finished`);
    - `commands` and `command_ms`, from command records whose `ParentSpanID` is one of the entries' span ids;
    - `helper_wait_ms` and `user_wait_ms`, as the sum over entries of `total(clip(union, entry))`.
  - **Tasks.** Spans with `Type == "task"`, keyed by `TaskID`; a task with several spans sums them, taking the first start and the last end. They carry `task_id`, `name`, `span_id` (the first), the times, `duration_ms`, the clipped waits, `end_reason` (the last span's) and `complete`.
  - **Helpers and user waits.** Each span of the type, in start order. They carry `span_id`, `parent_span_id`, `name`, `step` (the attribute, or the name of the step entry containing its start), `task_id` (from the containing task span), the times, `duration_ms`, `end_reason`, `status` (null when open), `message` (null when empty) and `complete`.
  - JSON tags are exactly as in plan.md's Data Structures. `*string` or `omitempty` is not used: absent values are explicit `null` via pointer fields.
- `internal/runstats/gaps.go` (new): `gaps(r *Run) []Gap`.
  - It walks `r.Events`, tracking `covered`, the latest `max(At, Until)` so far, and `lastLabel`, the event that set it.
  - For each next event with `At > covered`, it emits `Gap{From: covered, To: At, After: lastLabel, Before: next}`. Label edges: a command's `After` edge is `end`, and its `Before` edge is `start`.
  - `Step` is the name of the step entry whose `[Start, End)` contains `From`, or null.
  - The report sorts gaps by duration descending (ties by time), keeps `o.Gaps`, and totals those with duration `> o.GapThreshold` into `GapMS` and `GapsOverThreshold`.
- `internal/runstats/report_test.go`, `gaps_test.go`, `intervals_test.go` (new): fixtures named for the spec's acceptance criteria.
  - `knownWaits`: two user waits of 3 m and 2 m and one helper of 4 m, with no overlap. Working equals wall less 5 m, and helper wait is 4 m. A variant has two overlapping helpers (10:00–10:04 and 10:02–10:06), giving 6 m, not 8 m.
  - `stepTwice`: `interview` entered twice, with 3 commands in each.
  - `twoTasks`: an implement run with two task spans and step spans parented to them, and a helper inside task 2.
  - `threeHelpers`: three helper launches.
  - `twentyMinutePause`, run with `--gaps 3`, and with thresholds of 5 m and 30 m.
  - `interrupted`: a step closed `interrupted` at its last command, plus an open helper and an abandoned user wait. Both are listed with `complete:false`, and are absent from the totals and from the step's waits.
  - `investigationRow`: one step with commands, a helper and a user wait, checking `duration_ms`, `command_ms`, `helper_wait_ms` and `user_wait_ms` (the success-metric support).
- `internal/runstats/testdata/report.golden.json` (new): the golden JSON of a fixture run's report, pinning every field name and unit (contract for 000076 and 000077). It is regenerated only with an `-update` flag.

**Complexity**: High
**Token estimate**: ~40k tokens
**Agent strategy**: Parallel analysis: (a) intervals and gaps with tests; (b) totals, steps, tasks, helpers and waits with tests. Sequential integration into `BuildReport` and the golden file by one agent.

### Task: Keep the stats commands out of the logs

**File changes**
- `cmd/root.go` (`runRoot` as 000073 rewrites it, today `:110-176`; the record write is step 5 of 000073's pipeline, `recordCommand`). Add:
  - `const logAnnotation = "spektacular.log"` and `const logExempt = "exempt"`, beside `sessionLogEnv`;
  - `func logExempted(c *cobra.Command) bool`, which walks `c` and its parents and returns true when any has `Annotations[logAnnotation] == logExempt`, modelled on `gateExempted` (`cmd/gate.go:120-135`).

  In `runRoot`, after `executedCmd, err := rootCmd.ExecuteC()`, skip `recordCommand` when `logExempted(executedCmd)`. The verbose tee is still restored. The pre-run steps are unchanged; for an exempt command they only read state, apart from 000073's one-time idempotent legacy run-id backfill, which never touches the logs. Document the rule in the `runRoot` comment: read-only log commands are never logged.
- `cmd/root_test.go` (extend the log tests 000073 rewrites):
  - `TestLogExemptedWalksParents`: a throwaway cobra tree with an annotated group, where a child is exempt and a sibling outside the group is not;
  - `TestExemptCommandWritesNoRecord`: register an annotated hidden test command on `rootCmd` for the test and remove it on cleanup, or use `stats list` once it exists. With `log.level: verbose`, running it leaves the logs directory as it was, and a following `status` still writes one record.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: List runs and report on one from the command line

**File changes**
- `cmd/stats.go` (new):
  - `statsCmd` has `Use: "stats"`, `RunE: runUnknownSubcommand` (`cmd/root.go:54`) and `Annotations: {logAnnotation: logExempt}`.
  - Subcommands `statsListCmd` and `statsRunCmd`. Persistent flags on `statsCmd`:
    - `--dir`, `--format` (default `pretty`; values `pretty`/`json` as `statusFormatPretty`/`statusFormatJSON`, `cmd/status.go:22-26`), `--name`, `--kind`, `--agent`;
    - `--run`, a `StringSlice`;
    - `--gap-threshold`, a `Duration` defaulting to `5m`.

    `--gaps` (`Int`, default 10) is on `statsRunCmd` only. `--schema` is on each subcommand.
  - `statsLogs(cmd)` resolves the directory:
    - `--dir` is made absolute with `filepath.Abs`;
    - otherwise it requires a project (`projectConfigExists`, `cmd/root.go:426`), giving `noProjectError` with the next action changed to "run from a Spektacular project, or pass --dir <logs directory>". It then uses `loadConfig()` and `cfg.LogDir(root)` (000073).

    It then calls `runstats.Load`. `ErrNoLogDir` maps to `stats_log_dir_not_found`, with a next action naming `--dir` and `log.config.directory`.
  - `statsFilter(cmd)` builds a `runstats.Filter`. `statsError(err)` maps `*SelectionError` and the sentinels to the codes in plan.md's error table, each with `WithResource` and `WithNextAction`. For example, `stats_run_not_found` gives "run `spektacular stats list` to see recorded runs and their ids". `stats_run_ambiguous` lists the matching ids and says "give more characters of the run id".
  - The format check copies `cmd/status.go:173-179`, with code `stats_format_unsupported`. Option checks: `--gaps < 1` or `--gap-threshold <= 0` give `stats_bad_option`.
  - The JSON response structs embed `Source{LogDir string \`json:"log_dir"\`; SkippedLines int \`json:"skipped_lines"\`}`: `listResult{Source; Runs []runstats.RunSummary}` and `runResult{Source; GapThresholdMS int64; runstats.Report}`. They are written via `output.New(cmd.OutOrStdout(), globalFields).WriteResult(v)` (`internal/output/writer.go:22`). Pretty output goes through `runstats.RenderList`/`RenderReport` to `cmd.OutOrStdout()`.
  - `--schema` uses `commandSchema` (`cmd/spec.go:50`), with an output schema listing the top-level fields and a flags map.
- `cmd/root.go:516-528`: `rootCmd.AddCommand(statsCmd)`.
- `internal/runstats/summary_types.go` (new) or `report.go`: `RunSummary` (the JSON shape in plan.md) and `(*Run).Summary()`.
- `internal/runstats/render.go` (new): `RenderList(w, runs)` and `RenderReport(w, rep, skipped)`, plain text through `text/tabwriter`, with no colour.
  - The run id is shown as its first 8 characters.
  - Durations are formatted by `fmtMS`: under 60 s as `12.3s`, otherwise `4m05s` or `1h02m03s`. `null` prints as `-`.
  - The report prints a header line (kind, name, agent, run id, status, start), a totals block, then the Steps table (STEP, ENTRIES, DURATION, COMMANDS, CMD TIME, HELPER WAIT, USER WAIT, INCOMPLETE). It follows with the Tasks table (only when there are tasks), Helpers (NAME, STEP, STARTED, DURATION, END), User waits, and Gaps (DURATION, STEP, AFTER, BEFORE).
  - The last line is `Skipped lines: N` when N > 0.
  - Golden text tests: `internal/runstats/testdata/list.golden.txt` and `report.golden.txt`.
- `cmd/stats_test.go` (new):
  - The helper `seedLogs(t, dir)` writes fixture JSON Lines with fixed timestamps. It produces three finished runs (spec, plan, implement, two agents) and one in-progress plan run, in files named as 000073 names them, plus a `standalone_20261025.jsonl`.
  - A project comes from `writeCurrentConfig` (`cmd/root_test.go:93`) with the default `log` section. Every test uses `runRootCmd` (`cmd/root_test.go:70`).
  - Tests:
    - list order and columns (JSON and pretty);
    - the default report is the latest;
    - `--name`, `--kind`, `--agent`, `--kind` with `--agent`, `--run` full and prefix;
    - `--fields '["run"]'`;
    - JSON parses with `"error": false`;
    - pretty contains the same numbers as the JSON (wall time and per-step durations);
    - `--dir` on a `cp -r` copy outside the project, from a non-project working directory, gives identical JSON apart from `log_dir`;
    - an appended malformed line gives `skipped_lines: 1` with the other figures equal;
    - every refusal code has a non-empty `next_action`;
    - a bare `stats` gives `unknown_subcommand`;
    - byte-identical logs: hash every file in the logs directory, run `stats list` and `stats run` in the project, hash again, and compare (no new files, no removed files, same bytes).

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (a) `render.go` with golden tests; (b) `cmd/stats.go` with the `cmd` tests. Integrate sequentially.

### Task: Compare and summarise runs

**File changes**
- `internal/runstats/compare.go` (new): `Compare(runs []*Run, o Options) Comparison`.
  - It builds a `BuildReport` for each run.
  - Step order is the first run's step order, then the steps of later runs not yet seen, in their order.
  - The total figures, in order, are `wall_ms`, `working_ms`, `user_wait_ms`, `helper_wait_ms`, `command_ms`, `commands`, `helpers` and `gap_ms`. The step figures, in order, are `duration_ms`, `entries`, `commands`, `command_ms`, `helper_wait_ms` and `user_wait_ms`.
  - Each figure is `Figure{Figure string; Values []*int64; Delta []*int64; DeltaPct []*float64}`:
    - `Values[i]` is null when run i never entered the step;
    - `Delta[i]` is `Values[i] - Values[0]` (0 for i == 0), null if either is null;
    - `DeltaPct[i]` is `Delta[i] / Values[0] * 100`, rounded to 0.1, null if `Values[0]` is null or 0.
  - `Comparison{Runs []RunSummary; Totals []Figure; Steps []StepFigures}`, with JSON tags as in plan.md.
- `internal/runstats/summary.go` (new): `Summarise(runs []*Run, o Options) Summary`.
  - It uses the same figure lists, with `Stat{Figure string; Count int; Median *float64; P90 *float64}` computed over the non-null values.
  - Median: sort; the middle value for an odd count, the mean of the two middle values for an even count.
  - P90: nearest-rank, `sorted[ceil(0.9*n)-1]`.
  - Count 0 gives null median and p90.
- `internal/runstats/render.go` (extend): `RenderComparison` and `RenderSummary`.
  - Comparison Steps table: STEP, then one DURATION column per run headed by the run's 8-character id and start date. Later runs show `10m01s (-3m31s, -26.0%)`, and an absent step shows `-`.
  - The comparison then prints a Totals table with the same layout for every total figure. Counts print as integers.
  - Summary prints `RUNS: n` and a Steps table (STEP, RUNS, MEDIAN, P90 for duration), then a Totals table (FIGURE, RUNS, MEDIAN, P90).
  - Golden text tests.
- `internal/runstats/compare_test.go`, `summary_test.go` (new):
  - two spec runs, where the second lacks `split` and has a longer `interview`: values, deltas, percentages, and null for the absent step;
  - a zero base gives a null percentage;
  - five implement runs with known durations (for example 10, 20, 30, 40, 50 s) give count 5, median 30 s and p90 50 s;
  - an even count gives a fractional median;
  - a step in 3 of 5 runs gives count 3.
  - `testdata/compare.golden.json` and `summary.golden.json` pin the shapes.

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: 2 parallel agents: (a) compare with its tests; (b) summary with its tests. Rendering is added sequentially after both.

### Task: Compare and summarise from the command line

**File changes**
- `cmd/stats.go` (extend): `statsCompareCmd` and `statsSummaryCmd`, sharing the persistent flags.
  - Compare: `Candidates`, then `ForCompare`, then `Compare`; `ErrTooFew` maps to `stats_compare_needs_two` with the next action "name two runs with --run <id> --run <id> (see `spektacular stats list`), or widen the selection".
  - Summary: `ForSummary`, then `Summarise`.
  - JSON: `compareResult{Source; GapThresholdMS; runstats.Comparison}` and `summaryResult{Source; GapThresholdMS; runstats.Summary}`. Pretty uses `RenderComparison`/`RenderSummary`. Each has `--schema`.
- `cmd/stats_test.go` (extend):
  - add a second finished spec run and five implement runs to the fixture, or a separate fixture builder;
  - the default compare takes the two latest of the latest kind, oldest first;
  - `--run a --run b` keeps that order;
  - one `--run` gives `stats_compare_needs_two`;
  - the default summary covers every run of the latest kind;
  - `--kind implement` gives count 5;
  - JSON and pretty agree;
  - byte-identical logs for `stats compare` and `stats summary`.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Documentation site describes run statistics

**File changes**
- `docs:src/pages/run-stats.mdx` (new):
  - frontmatter `layout: ../layouts/Shell.astro`, `title: "Run statistics - Spektacular"`, `description: "Report where a workflow run's time went, compare runs and summarise many, from Spektacular's own logs."`;
  - imports `Hero`, `Prose`, `CtaBanner` and `Button`, as `docs:src/pages/debugging.mdx:6-9` does;
  - body: `<Hero>`, one `<Prose>` holding sections 2 to 10 of the plan's content outline as markdown with fenced code blocks, then `<CtaBanner>` with a `<Button href="/debugging/">`. The structure mirrors `debugging.mdx`.
  - It follows the MDX conventions: a blank line inside slot bodies, no `<div>`, `<section>` or `class=`, and no em dashes.
- `docs:src/components/Nav.astro:16-25`: add `{ label: "Run statistics", href: "/run-stats/" }` to the Resources children, straight after the logging page's entry (labelled "Logging and debugging" by 000073).
- `docs:src/pages/debugging.mdx` (as 000073 and 000074 leave it): one sentence at the end of the "Reading a record" section, or of the spans sections if present: "To turn these records into timings per step, task and helper, see [Run statistics](/run-stats/)."
- Verify with `npm run build`, `npx astro check`, and `grep -nE "<div|<section|class=" src/pages/*.mdx`, which must return nothing.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: README lists the stats commands

**File changes**
- `README.md:29-40` ("How It Works", after the `status` code block and its paragraphs, before "Addressing specs, plans and changelog records"): add a short paragraph and a code block with the four `stats` forms (the plan's content example). Add one line saying `--format json` gives the structured report, `--dir <path>` reads a copied logs directory, and the figures are defined on the documentation site's Run statistics page. No em dashes.

**Complexity**: Low
**Token estimate**: ~3k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Every Go test runs under the shuffled `go test` (`make test`). Fixtures use fixed `time.Date` timestamps and their own temp directories, and nothing reads the wall clock.

- **Load logs into runs**: `internal/runstats` table tests.
  - The loader: skipped-line kinds, a long line, read-only hashing.
  - Assembly: grouping across files, standalone exclusion, pairing, an end without a start, the abandoned end left out of the wall time, all three statuses.
  - Selection: each filter, full id and prefix, ambiguous, unknown and malformed ids, the defaults per mode.
- **Report on one run**: interval helper tests; gap tests (20-minute pause, `--gaps 3`, 5 m and 30 m thresholds, a command never being a gap); report tests (known waits, overlapping helpers, a step entered twice, two tasks, three helpers, interrupted and unfinished work, the investigation row); the `report.golden.json` contract.
- **Keep the stats commands out of the logs**: `cmd` tests that the annotation walk exempts a child of an annotated group, that an exempt command writes nothing at the verbose level, and that a normal command still writes one record.
- **List runs and report on one from the command line**: `cmd` integration tests over a seeded fixture project.
  - List order and columns; the default report; every selection; `--fields`.
  - JSON envelope parsing, and pretty-JSON agreement.
  - A copied `--dir` from outside a project.
  - The skipped line; every refusal with a next action; a bare `stats`.
  - Byte-identical logs.
  - Golden text for the list and report renderers.
- **Compare and summarise runs**: comparison values, deltas, percentages, absent steps and a zero base; summary count, median (odd and even) and nearest-rank p90 over five runs; the golden JSON and text.
- **Compare and summarise from the command line**: `cmd` tests for the defaults, `--run` order, the too-few refusal, the summary by kind, format agreement and byte-identical logs.
- **Documentation site describes run statistics**: `npm run build`, `npx astro check`, and the MDX layout-HTML grep.
- **README lists the stats commands**: reviewed in the diff. No test asserts README prose.

Success metrics and manual reviews are classified in plan.md's Testing Approach. The supporting benchmark `BenchmarkLoadAndReport` in `internal/runstats` generates a ten-run, verbose-sized directory under `b.TempDir()`.

## Project References

- Spec `000075_run-stats-command`, epic `000002_workflow-tracing-and-run-stats`.
- Upstream plans:
  - `000073_log-mode-and-logger-providers`: record format, file naming, `config.LogDir`, `runRoot` pipeline;
  - `000074_trace-spans-for-agent-work`: span kinds, types, end reasons, parents, task ids.
- Downstream specs: `000076_harbor-on-built-in-logs` (reads the JSON), `000077_html-log-viewer` (reuses `internal/runstats`).
- Design documents: none.
- Knowledge entries applied:
  - `conventions/error-messages-must-suggest-remediation.md`;
  - `conventions/tests-must-not-depend-on-order.md`;
  - `conventions/tests-must-pass-for-done.md`;
  - `conventions/plans-never-change-the-active-install.md`;
  - `conventions/store-files-must-be-written-through-the-cli.md`;
  - the docs repo's `conventions/mdx-authoring.md`, `conventions/plan-content-pages.md`, `conventions/no-em-dashes.md` and `conventions/site-layout.md`.
- Reference material: `docs/workflow-performance-investigation.md` (method and per-step table); `tests/harbor/shared/metrics.py`, `report.py` and `timeline.py` (Harbor's figures).
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

This plan: Load logs into runs (Medium, ~30k), Report on one run (High, ~40k), Keep the stats commands out of the logs (Low, ~8k), List runs and report on one from the command line (Medium, ~30k), Compare and summarise runs (Medium, ~20k), Compare and summarise from the command line (Low, ~12k), Documentation site describes run statistics (Low, ~12k), README lists the stats commands (Low, ~3k).

## Migration Notes

None. No settings format, state field or record field changes, and nothing is migrated. The logging exemption only stops `stats` commands from being recorded. Installing the new binary is left to the maintainer between workflows (plans-never-change-the-active-install).

## Performance Considerations

- Success metric: a report over 10 runs in under one second on a laptop.
- The loader streams each file line by line with no length cap and decodes into `tracelog.Record`. Verbose lines (full responses) dominate the cost.
- Everything after loading is linear or `n log n` in the number of records, through sorting and interval merges.
- `BenchmarkLoadAndReport` (ten generated runs, about 600 records each with 20 KB verbose responses) is the supporting evidence; the manual measurement uses the installed binary.
- If it is slow, an in-package pre-decode filter that skips `spektacular.response` is allowed. An index or cache file is not (see Open Questions).
