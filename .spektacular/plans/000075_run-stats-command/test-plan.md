---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Test plan: 000075_run-stats-command

These are the checks the automated suite cannot make: the spec's three success metrics, each supported but not proven by a test, and the plan's two manual reviews. Each runs against the built binary from the `spektacular` repo, never the installed one, and never against a project's live logs directory directly: copy the logs first, and point `--dir` at the copy.

Setup for every procedure:

```bash
cd <spektacular repo checkout with 000075 merged>
go build -o /tmp/spek-stats .
cp -r <project>/.spektacular/logs /tmp/stats-logs   # a copy; the report must never change the original
```

The logs need runs recorded by a build carrying 000073 and 000074 (command records and step, task, sub-agent and user-wait spans). Logs written earlier hold commands only, which give totals and gaps but no step breakdown.

## 1. Success metric: the investigation's per-step table can be produced from the report alone, apart from model time

- **What to measure**: for one new implement run, every column of the per-step table in `docs/workflow-performance-investigation.md` (Appendix, Implement: step, model, sub-agent wait, calls) except model time, which the logs do not hold.
- **How**:
  1. Run one implement workflow end to end with an agent that marks its helpers (`spektacular trace start ... --type sub-agent`) and waits on the user (`--type user-wait`), as the workflow instructions tell it to.
  2. `/tmp/spek-stats stats run --dir /tmp/stats-logs --kind implement --format json > /tmp/run.json`
  3. From `steps[]`, take per step: `duration_ms` (step wall time), `helper_wait_ms` (the table's "sub-agent wait"), `user_wait_ms` (human time), `commands` (Spektacular calls) and `command_ms` (the stand-in for tool time, covering Spektacular's own commands only).
  4. Rebuild the table from those fields alone and compare it, step by step, with the agent transcript of the same run.
- **Expected result**: every step of the run has a row; each row's helper wait matches the transcript's sub-agent wait for that step within 5 seconds per helper; each row's user wait matches the time the transcript shows the agent waiting on a question; the only column that cannot be filled is model time.
- **Who / when**: a maintainer, once after 000075 merges and again whenever the step, task or agent span records change.

## 2. Success metric: a report over 10 runs returns in under one second

- **What to measure**: wall-clock time of `stats run` (and `stats list`) over a logs directory holding 10 runs, the default retention, at the `verbose` level.
- **How**:
  1. Use a project with `log.level: verbose` and at least 10 recorded workflow runs (`/tmp/spek-stats stats list --dir /tmp/stats-logs` shows 10 or more rows).
  2. Run each three times and take the slowest:

     ```bash
     time /tmp/spek-stats stats list --dir /tmp/stats-logs > /dev/null
     time /tmp/spek-stats stats run --dir /tmp/stats-logs > /dev/null
     time /tmp/spek-stats stats summary --dir /tmp/stats-logs > /dev/null
     ```
  3. Record `du -sh /tmp/stats-logs` beside the times.
- **Expected result**: each `real` time is below 1.0 s on a developer laptop. Supporting evidence: `go test -run xxx -bench BenchmarkLoadAndReport ./internal/runstats` (ten generated runs, 4,400 commands in all, each carrying a 20 KB verbose response, about 90 MB) measured about 0.10 s per load-and-report on the implementing machine. If a real directory exceeds one second, stop and raise it: a pre-decode filter skipping `spektacular.response` inside the package is allowed; an index or cache file beside the logs is not.
- **Who / when**: a maintainer, before the release that ships `stats`.

## 3. Success metric: the structured output holds every figure Harbor derives from transcripts for step, helper and idle timing

- **What to measure**: a field-by-field match between Harbor's figures in `tests/harbor/shared/metrics.py` and the `stats run --format json` output.
- **How**: with `/tmp/run.json` from procedure 1, check each Harbor figure has its counterpart:

  | Harbor (`metrics.py`) | `stats run --format json` |
  |---|---|
  | step entry `step` | `step_entries[].step` |
  | step entry `started_at` | `step_entries[].started_at` |
  | step entry `duration_s` | `step_entries[].duration_ms` / 1000 |
  | child `id`, `parent` | `helpers[].span_id`, `helpers[].parent_span_id` |
  | child `step` | `helpers[].step` |
  | child `spec` (the lane) | the run's `run.name` (one run per spec) |
  | child `launched_at`, `ended_at` | `helpers[].started_at`, `helpers[].ended_at` |
  | child `duration_s` | `helpers[].duration_ms` / 1000 |
  | child `ending` | `helpers[].end_reason` and `helpers[].complete` |
  | `totals.idle_s` (longest gap) | `gaps[0].duration_ms` / 1000 (with `--gaps 1`) |
  | step `children` count | count of `helpers[]` per `step` |

- **Expected result**: every row has a populated counterpart for the run; the longest gap agrees with Harbor's `idle_s` for the same run within 1 second, or differs only because Harbor also counts agent message times the logs do not hold (note which). Character counts (`output`, `prompt_chars`) and models are out of scope: the logs do not hold them.
- **Who / when**: the implementer of spec 000076 (Harbor on built-in logs), before switching Harbor to read this output.

## 4. Manual review: the four reports on real logs are plausible and readable

- **Where**: the built binary against a copy of a logs directory holding at least one spec, one plan and one implement run.
- **What to look at**:

  ```bash
  /tmp/spek-stats stats list --dir /tmp/stats-logs
  /tmp/spek-stats stats run --dir /tmp/stats-logs --kind spec
  /tmp/spek-stats stats run --dir /tmp/stats-logs --kind implement
  /tmp/spek-stats stats compare --dir /tmp/stats-logs --kind plan     # needs two plan runs
  /tmp/spek-stats stats summary --dir /tmp/stats-logs --kind implement
  /tmp/spek-stats stats run --dir /tmp/stats-logs --format json | head -40
  ```
- **What to look for**: the list's statuses match what happened to each run (finished, interrupted by a restart, still in progress); step durations add up to roughly the wall time; helper and user waits are no larger than the wall time; an unfinished helper or task carries `*`; the longest gaps name sensible events either side; columns line up at an 80 to 120 column terminal; a step a compared run never entered shows `-`.
- **Passing**: nothing implausible (a negative or impossibly long figure, a wrong status, a misaligned table), and `diff -r <project>/.spektacular/logs /tmp/stats-logs` still shows only the runs recorded since the copy, never a change made by `stats`.
- **Who / when**: a maintainer, after merge and before release.

## 5. Manual review: the Run statistics documentation page matches the shipped behaviour

- **Where**: the docs repo, `npm ci && npm run build && npm run preview`, then open `/run-stats/`, and the "Run statistics" entry in the Resources menu, and the link at the end of the span records section of `/debugging/`.
- **What to look for**: every command (`stats list`, `stats run`, `stats compare`, `stats summary`), flag (`--dir`, `--format`, `--name`, `--kind`, `--agent`, `--run`, `--gaps`, `--gap-threshold`, `--fields`) and default (latest run; two latest of its kind; every run of its kind; 10 gaps; 5 minutes) on the page behaves as written when run with the built binary; each figure definition matches what the tables and JSON show; the example tables have the real column headings.
- **Passing**: no statement on the page that the binary contradicts, and no broken link.
- **Who / when**: a maintainer or docs reviewer, before the release that ships `stats`.
