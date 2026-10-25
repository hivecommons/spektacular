**Input: the log records (read, unchanged).** The report reads the record format that 000073 defines and 000074 extends, and changes none of it: `record` (`span`, `span_start`, `span_end`), `trace_id`, `span_id`, `parent_span_id`, `name`, `start_time`, `end_time`, `status{code,message}`, and these attributes:
- `spektacular.span.type`: `command`, `step`, `task`, `sub-agent`, `user-wait` or `work`;
- `spektacular.span.end_reason`;
- `spektacular.agent`;
- `spektacular.workflow.kind` and `spektacular.workflow.name`;
- `spektacular.step`;
- `spektacular.task.id`.

It reads every `*.jsonl` file in the directory: run files and `standalone_<day>.jsonl` files alike.

**Commands.**

```text
spektacular stats list    [selection] [--dir <path>] [--format pretty|json]
spektacular stats run     [selection] [--gaps 10] [--gap-threshold 5m] [--dir <path>] [--format pretty|json]
spektacular stats compare [selection] [--gap-threshold 5m] [--dir <path>] [--format pretty|json]
spektacular stats summary [selection] [--gap-threshold 5m] [--dir <path>] [--format pretty|json]

selection:  --name <workflow name>   --kind spec|plan|implement|repo   --agent <agent>
            --run <run id or unique prefix of 8+ hex characters>   (repeatable)
```

- `--format` defaults to `pretty`.
- `--dir` defaults to the project's configured log directory.
- `--gap-threshold` takes a Go duration.
- Every subcommand supports `--schema`, and the JSON form honours the global `--fields`.

**Structured output (JSON, field names are a contract for 000076 and 000077).** Every success response carries `"error": false`, `log_dir` (the absolute directory read) and `skipped_lines` (lines that could not be read). The units and conventions are:
- durations are integer milliseconds;
- times are RFC 3339 UTC;
- a figure that does not apply is `null`, never `0`.

*Run summary*: one per run, used by `list`, `run`, `compare` and `summary`.

```json
{"run_id":"4bf92f3577b34da6a3ce929d0e0e4736","kind":"plan","name":"000075_run-stats-command",
 "agent":"claude","log_file":"20261025T082050.123456789Z_claude_plan_000075_run-stats-command.jsonl",
 "started_at":"2026-10-25T08:20:50.123Z","ended_at":"2026-10-25T09:02:11.400Z",
 "wall_ms":2481277,"status":"finished"}
```

`status` is one of `finished`, `interrupted` or `in_progress`.

*`stats list`*: `{"log_dir", "skipped_lines", "runs": [<run summary>, ...]}`, newest first. An empty directory gives `"runs": []`.

*`stats run`*: a report on one run.

```json
{"log_dir":"/abs/.spektacular/logs","skipped_lines":0,"gap_threshold_ms":300000,
 "run":{"...":"run summary"},
 "totals":{"wall_ms":2481277,"working_ms":1803000,"user_wait_ms":678277,"helper_wait_ms":512000,
           "command_ms":4100,"commands":61,"helpers":3,"user_waits":2,
           "gap_ms":1520000,"gaps_over_threshold":3},
 "steps":[{"step":"discovery","entries":2,"duration_ms":812000,"commands":9,"command_ms":610,
           "helper_wait_ms":402000,"user_wait_ms":0,"incomplete":1}],
 "step_entries":[{"step":"discovery","span_id":"a3ce929d0e0e4736","task_id":null,
                  "started_at":"...","ended_at":"...","duration_ms":402000,
                  "end_reason":"interrupted","complete":false}],
 "tasks":[{"task_id":"df1bade8-66a7-4bb8-b261-3dda877ae82c","name":"Trace log package",
           "span_id":"...","started_at":"...","ended_at":"...","duration_ms":300000,
           "helper_wait_ms":120000,"user_wait_ms":0,"end_reason":"moved","complete":true}],
 "helpers":[{"span_id":"...","parent_span_id":"...","name":"research","step":"discovery",
             "task_id":null,"started_at":"...","ended_at":"...","duration_ms":150000,
             "end_reason":"stopped","status":"ok","message":null,"complete":true}],
 "user_waits":[{"...":"same shape as helpers"}],
 "gaps":[{"started_at":"...","ended_at":"...","duration_ms":1200000,"step":"walkthrough",
          "after":{"event":"end","type":"command","name":"plan goto"},
          "before":{"event":"start","type":"command","name":"plan goto"}}]}
```

- **Totals.** `gap_ms` is the total of gaps longer than the threshold, and `gaps_over_threshold` is how many there are. `commands`, `helpers` and `user_waits` count every item, finished or not.
- **Steps.** `steps` has one row per step name, in order of first entry. `incomplete` counts its entries that did not end with `moved` or `finished`.
- **End reasons.** `end_reason` is one of the 000074 reasons (`moved`, `finished`, `stopped`, `abandoned`, `interrupted`), or `open` for a span never closed. `complete` is true for `moved`, `finished` and `stopped`.
- **Unfinished spans.** For a span that is not complete, `ended_at` and `duration_ms` run to where it was cut off. These are the capped times, as Architecture defines them.
- **Gaps.** `gaps` is longest first, at most `--gaps` entries. `after` and `before` name the events either side. `event` is `start` or `end`, `type` is a span type, and `name` is the span or command name.

*`stats compare`*: runs side by side.

```json
{"log_dir":"...","skipped_lines":0,"gap_threshold_ms":300000,
 "runs":[{"...":"run summary, base first"},{"...":"run summary"}],
 "totals":[{"figure":"wall_ms","values":[2481277,1990000],"delta":[0,-491277],"delta_pct":[0,-19.8]}],
 "steps":[{"step":"discovery","figures":[
   {"figure":"duration_ms","values":[812000,null],"delta":[0,null],"delta_pct":[0,null]}]}]}
```

- Arrays line up with `runs`. A step a run never entered is `null` there, with `null` deltas.
- `delta_pct` is `null` when the base value is 0 or `null`. Percentages are rounded to one decimal.
- Total figures, in order: `wall_ms`, `working_ms`, `user_wait_ms`, `helper_wait_ms`, `command_ms`, `commands`, `helpers`, `gap_ms`.
- Step figures, in order: `duration_ms`, `entries`, `commands`, `command_ms`, `helper_wait_ms`, `user_wait_ms`.

*`stats summary`*: many runs at once. It has the same top-level shape as `compare`, except that each figure is `{"figure":"duration_ms","count":5,"median":812000,"p90":1104000}`.
- `count` is the number of runs the figure applies to. For a step, that is the runs that entered it.
- `median` and `p90` are JSON numbers. A median of an even count can be fractional.

**Errors.** Every error is the standard error envelope with a `next_action`:

| Code | When | Next action |
|---|---|---|
| `stats_format_unsupported` | `--format` is not `pretty` or `json` | re-run with `--format pretty` or `--format json` |
| `no_project` | no project and no `--dir` | run from a project or pass `--dir <logs directory>` |
| `stats_log_dir_not_found` | the directory does not exist | check the path; the project's own is the `log.config.directory` setting |
| `stats_bad_selection` | unknown `--kind`, or a malformed `--run` | names the accepted kinds, or the id format |
| `stats_run_not_found` | a `--run` matches no run | run `spektacular stats list` |
| `stats_run_ambiguous` | a `--run` prefix matches several runs | give more characters, listing the matches |
| `stats_no_runs` | no run matches the selection | run `spektacular stats list`, or widen the selection |
| `stats_compare_needs_two` | fewer than two runs to compare | name two runs with `--run`, or widen the selection |
| `stats_bad_option` | `--gaps` below 1, or `--gap-threshold` not positive | gives the accepted range |

**Go interfaces (`internal/runstats`, new).**

```go
type Logs struct{ Dir string; Runs []*Run; Skipped int }
func Load(dir string) (*Logs, error)                 // read-only; error only when the directory cannot be read
func Assemble(recs []tracelog.Record) []*Run          // pure: group, pair, timeline, status

type Run struct {
    ID, Kind, Name, Agent, LogFile string
    Start, End time.Time
    Status   string            // finished | interrupted | in_progress
    Spans    []Span            // every non-command span, paired, in start order
    Commands []tracelog.Record // command records, in start order
    Events   []Event           // the timeline the wall time and gaps are measured on
}
type Span struct {
    ID, Parent, Name, Type, Step, TaskID string
    Start, End time.Time // End is the effective end (capped when not complete)
    EndReason, Status, Message string
    Complete bool
}
type Event struct{ At, Until time.Time; Edge, Type, Name string } // Until > At only for commands

type Filter struct{ Name, Kind, Agent string; Runs []string }
func (l *Logs) Candidates(f Filter) ([]*Run, error) // newest first; *SelectionError for unknown/ambiguous/malformed
func ForReport(c []*Run) (*Run, error)
func ForCompare(c []*Run, f Filter) ([]*Run, error)
func ForSummary(c []*Run, f Filter) ([]*Run, error)

type Options struct{ Gaps int; GapThreshold time.Duration }
func BuildReport(r *Run, o Options) Report
func Compare(runs []*Run, o Options) Comparison
func Summarise(runs []*Run, o Options) Summary
func RenderList(w io.Writer, l *Logs, runs []*Run) error // and RenderReport, RenderComparison, RenderSummary
```

`Report`, `Comparison` and `Summary` are the Go types behind the JSON above, with matching `json` tags. `cmd` adds `log_dir` and `skipped_lines`. No existing type, record field or setting changes. `cmd` gains one cobra annotation key, which marks a command as exempt from logging.
