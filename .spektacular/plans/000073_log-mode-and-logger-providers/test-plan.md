---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Test plan: 000073_log-mode-and-logger-providers

The automated suites cover every behavioural guarantee of this spec: the `internal/tracelog`, `internal/workflow`, `internal/config` and `internal/migrate` unit tests, the `cmd` command-log tests (`cmd/log_test.go`, `cmd/log_lane_test.go`, `cmd/log_retention_test.go`), and Harbor's shared Python unit tests. The Go half of output parity runs as part of the suite: `TestLog_OutputParityAcrossLevels`, plus the whole `cmd` suite run a second time with logging forced off (`go test -shuffle=on -count=1 ./cmd/ -args -log-level=off`).

What follows are the success metrics and reviews the plan marks as manual. Each one needs the new binary installed (`make install-local`) or built (`go build -o /tmp/spek .`) from the merged branch.

## Metric 1: step timings can be read from the log alone

- **What to measure**: for one real workflow run, every step's start, end and duration can be read from the log, without the agent's transcript.
- **How**:
  1. In a throwaway project (`mkdir /tmp/lt && cd /tmp/lt && git init -q && spektacular init claude`), have an agent run a real spec workflow to `finished`, for example "create a spec for a todo CLI's `count` command".
  2. Open the run's file: `ls .spektacular/logs/*_claude_spec_*.jsonl`.
  3. Run `jq -r 'select(.attributes["spektacular.step.to"]) | [.start_time, .attributes["spektacular.step"] // "-", .attributes["spektacular.step.to"]] | @tsv' .spektacular/logs/*_claude_spec_*.jsonl`. Each line is the moment the run moved from one step to the next. A step's duration is the gap between the line that moved onto it and the line that moved off it.
- **Expected result**: every step the run passed through appears once as a `step.to` value, in order, ending at `finished`. Every record carries the same `trace_id`, which equals `run_id` in the run's state while it ran. The durations add up to the run's span, give or take the agent's time after the last command.
- **Who / when**: the maintainer, once, before the epic's release.

## Metric 2: the standard level adds under 10 ms to a typical command

- **What to measure**: the difference in mean wall time of `spektacular spec file list` between `log.level: standard` and `log.level: off`, over 100 runs each. Threshold: under 10 ms.
- **How**: in an `init`ed throwaway project, set `log.level` in `.spektacular/config.yaml` to `off`, time 100 runs, then set it to `standard` and time 100 more. For example, run `for l in off standard; do sed -i "s/^    level: .*/    level: $l/" .spektacular/config.yaml; /usr/bin/time -f "$l %e s" sh -c 'for i in $(seq 100); do spektacular spec file list >/dev/null; done'; done`, and divide each total by 100. Repeat three times and compare the means.
- **Expected result**: standard minus off is under 10 ms per command. If it is over 10 ms, stop and raise it before adding any caching (plan Open Questions).
- **Preliminary evidence**: a run on the implementer's machine with a `go build` binary measured a mean of 1.52 to 1.54 ms at `off` and 1.59 to 1.60 ms at `standard`, about 0.07 ms added, over three rounds of 100 runs each.
- **Who / when**: the maintainer, on the installed release binary, before release.

## Metric 3: one 10-task implement run logs under 1 MB at the default level

- **What to measure**: the size of the log file of one complete implement run of a plan with 10 tasks, at `log.level: standard`. Threshold: under 1 MB.
- **How**: run a real `spek-implement` of a 10-task plan to `finished` in a project at the default level (`implement.worktrees` can be on or off). Then run `ls -l .spektacular/logs/*_implement_<spec>.jsonl` and `wc -l` the same file.
- **Expected result**: the file is under 1,048,576 bytes. A standard record is about 400 to 600 bytes, so a few hundred commands come to well under 1 MB.
- **Who / when**: the maintainer, on the first real 10-task implement run after release.

## Metric 4 (Harbor half): output and exit codes do not change with logging

- **What to measure**: the Harbor workflow suites behave the same with the seeded verbose level as before the change: every check passes or fails for the same reasons.
- **How**: run at least `make harbor-test-plan` and `make harbor-test-epic` (Claude) with the new binary. Compare each trial's `verifier/test-stdout.txt` and `verifier/reward.txt` with a run of the same suites on the previous release.
- **Expected result**: the same checks pass. The one exception is the epic suite's removed `test_orchestrator_polled_epic_status`, dropped because `status` is never logged. No failure points at a changed command output or exit code.
- **Who / when**: the maintainer, before release (the suites need Docker and agent logins and do not run in CI).

## Review 1: a Harbor suite still reports sessions from the new logs

- **What the reviewer looks at**: the same `make harbor-test-plan` and `make harbor-test-epic` runs as Metric 4.
- **Where**:
  - Each trial's `artifacts/spektacular/logs/`, which must hold one `<started>_<agent>_<kind>_<name>.jsonl` per run (one per spec lane for the epic suite) and no `.spektacular/debug/`.
  - The epic timeline at the top of the epic suite's results (`python3 tests/harbor/shared/timeline.py <job>/<trial>/artifacts/spektacular/logs`).
  - The `metrics.json` the recorder wrote.
- **What to look for**:
  - The plan suite reports the plan workflow's steps in order.
  - The epic suite reports each spec's lane, its start and finish, its slowest step and its children, and the parallel planning of the two ready specs.
  - `metrics.json` has `steps` filled in, not `unknown`.
  - The only expected difference from a previous-release run is that the timeline's "spawned after" column shows `-`, because the orchestrator's `status` polls are no longer logged.
- **Passing**: the reported workflow steps and parallel runs match a run on the previous release.

## Review 2: the docs pages describe the shipped behaviour

- **What the reviewer looks at**: the built docs site (`npm ci && npm run build && npm run preview` in the docs repo).
- **Where**: `/debugging/` (titled "Logging and debugging" in the navigation) and the `log` key on `/configuration/`.
- **What to look for**: check each statement against the shipped behaviour:
  - The levels: standard keeps no response or state, verbose adds both, off records nothing.
  - The default location `.spektacular/logs/`, and that `log.config.directory` resolves from `.spektacular/`.
  - The run and day file names.
  - Retention: 10 by default, with runs and days counted separately.
  - Switching off with `log.level: off`.
  - That `status` and the `stats` reports are never logged.
  - The sample record's field names.
  - The `migrate` mapping from `debug.enabled`.
  - That no environment variable for logging is described anywhere.
  - That the `stats` sentence is accurate once spec 000075 has shipped.
- **Passing**: every statement matches what the binary does, and no page mentions `debug.enabled`, `.spektacular/debug` (other than the upgrade note) or `SPEKTACULAR_SESSION_LOG`.
