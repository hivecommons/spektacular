---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Test plan: 000074_trace-spans-for-agent-work

The behaviour itself is covered by Go tests (`internal/tracelog`, `internal/workflow`, `internal/steps/implement`, `cmd/log_spans_test.go`, `cmd/trace_test.go`, `cmd/trace_instruction_test.go`, `templates/trace_partial_test.go`) and by Harbor's offline self-checks (`uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests`). What follows needs real agents, real runs or a person, and is done by hand.

Setup for every Harbor procedure: build and install this branch's binary (`make install-local`), then run the suite from the repo root. Harbor needs Docker, the `harbor` CLI and the agent's login, and each run takes 15 to 25 minutes. A run's job folder under `tests/harbor/jobs/<job>/<trial>/` holds the verifier's `junit.xml`, `metrics.json` (under `verifier/`) and the collected `.spektacular/logs/` (under `artifacts/`).

To read a run's spans, use the shared reader from the repo root:

```bash
python3 -c 'import sys; sys.path.insert(0, "tests/harbor/shared"); import spans; [print(s.lane, s.type, s.name, s.start, s.end, s.end_reason, s.parent) for s in spans.read_spans(sys.argv[1])]' <trial>/artifacts/.spektacular/logs
```

## Success metrics

### 1. Step spans cover at least 95% of an implement run

- **What**: in a Harbor implement run, the step spans together cover at least 95% of the time between the run's first and last command.
- **How**: `make harbor-test-implement` (Claude). In the trial's `artifacts/.spektacular/logs/`, take the implement run's file (`*_implement_*.jsonl`). Sum `end_time - start_time` over its `span_end` records whose `spektacular.span.type` is `step`, and divide by (last command record's `end_time` minus first command record's `start_time`). Repeat over three runs.
- **Expected**: the ratio is at least 0.95 in each run. (The `cmd` tests already show step spans are contiguous from the first step to `finished`, so a shortfall means time before the first step or after the last.)
- **Who / when**: the maintainer, before release of the version that ships 000074.

### 2. At least 90% of helper launches are sub-agent spans

- **What**: in Harbor implement, plan-epic and implement-epic runs, at least 90% of helper-agent launches appear as sub-agent spans.
- **How**: run `make harbor-test-implement`, `make harbor-test-epic` and `make harbor-test-implement-epic` (Claude), three runs each. In each trial, `TestChildren::test_every_launch_is_a_sub_agent_span` (epic suites) and `test_reviewer_launch_is_a_sub_agent_span` (goal-check suite, `make harbor-test-goal-check`) list every unmarked launch in `junit.xml`. For the rate, run from the repo root: `python3 -c 'import sys; sys.path.insert(0,"tests/harbor/shared"); import delegation as dl; r=dl.read_run(sys.argv[1], sys.argv[2]); print(dl.marked_rate(r.placed, r.spans))' <trial>/agent/claude-code.txt <trial>/artifacts/.spektacular/logs`.
- **Expected**: the marked rate is at least 0.90 per suite, averaged over its runs.
- **Who / when**: the maintainer, before release.

### 3. At least 90% of questions to the scripted user are user-wait spans

- **What**: in Harbor spec runs, at least 90% of questions put to the scripted user appear as user-wait spans.
- **How**: `make harbor-test-spec` (Claude), five runs. `TestUserWaits::test_constraints_question_is_a_user_wait` checks the scripted Constraints rejection. For the rate, count the questions the agent asked the scripted user in the transcript (`agent/claude-code.txt`: assistant turns that end with a question to the user, in the interview and each section's confirmation) and the `user-wait` spans from the span reader above.
- **Expected**: user-wait spans divided by questions asked is at least 0.90 over the five runs, and the Constraints check passes in each.
- **Who / when**: the maintainer, before release.

### 4. Marking adds no more than two tool calls per plan task

- **What**: marking spans adds on average no more than two agent tool calls per plan task.
- **How**: the Harbor implement suite starts at `test_plan` and runs no plan tasks, so read the average from suites whose children implement real tasks: `make harbor-test-implement-epic` and `make harbor-test-goal-check`. Each trial's `verifier/metrics.json` carries `totals.trace_calls` (every `spektacular trace` call in the session log) and `totals.trace_calls_per_task` (that count divided by the run's task spans). Also note the implement suite's own `totals.trace_calls`.
- **Expected**: `trace_calls_per_task` is at most 2.0 averaged over three runs of each suite.
- **Who / when**: the maintainer, before release.

### 5. A step change stays within the logging time budget

- **What**: the plan's open question: reading the run log on each step change must not cost noticeably more than 10 ms at the standard level.
- **How**: in a throwaway project with logging at `standard`, after a long run (for example the logs of a finished implement-epic trial copied into `.spektacular/logs/`, with a matching in-progress state), time `spektacular spec goto --data '{"step":"<next>"}'` with `time` ten times, with and without the large log present.
- **Expected**: the median difference is at most 10 ms. If it is more, stop and decide whether to add an index file beside the run log before changing anything.
- **Who / when**: the maintainer, once, before release.

## Manual reviews

### 6. The logging page reads right

- **What**: a read-through of the spans sections of the site's "Logging and debugging" page.
- **Where**: in the docs repo, `npm run build && npm run preview`, then open `/debugging/`; the sections are "Spans: where the time went" through "Span records".
- **Look for**: the six span types and which Spektacular records itself match the shipped behaviour; `trace start` and `trace stop` and their options match `spektacular trace start --help` and `spektacular trace stop --help`; the end reasons (`moved`, `finished`, `stopped`, `abandoned`, `interrupted`) and the sample records match a real run's log; no em dashes and no layout HTML.
- **Pass**: nothing on the page contradicts the binary or a real log.

### 7. Sub-agent spans nest in a real epic run

- **What**: a read of one real run's log (plan-epic or implement-epic).
- **Where**: a trial of `make harbor-test-epic` or `make harbor-test-implement-epic`; print its spans with the reader above.
- **Look for**: each child's sub-agent span in the orchestrator's standalone day file (the epic skills mark launches outside any workflow run); inside each child's own lane file, its step spans with the sub-agent spans of its helpers (such as the goal reviewer) nested under the step that launched them (`parent` is that step's span id); and a helper's own spans nesting under the id it was given.
- **Pass**: every sub-agent span inside a lane has a parent that is an open step or agent span of that lane at the span's start, and none is left open once the lane finished (its end reason is `stopped`, or `abandoned` at the latest).
