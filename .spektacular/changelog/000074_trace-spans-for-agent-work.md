---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# 000074_trace-spans-for-agent-work: trace spans for agent work

## What was built

A run's log now shows where its time went, not only which commands ran.

- **Steps and tasks are recorded by themselves.** The workflow engine gives every step it enters a span, kept open in the run's state beside the run id, and closes it when the workflow leaves: the next step begins at the same moment, the terminal step is an instant span, a resume after a stopped session closes the stopped step as `interrupted` at its last real command (with every span still open under it) and opens a fresh one, and a `new --force` restart closes the old run's spans as `abandoned`. During implement, `analyze` through `update_changelog` run inside a task span named after the plan's current task, so each task's steps nest under it. Every command record names the step span it ran in as its parent. The spans are written as `span_start` and `span_end` records, beside the one `span` record per command, through the configured logger; a `span_end` repeats its start, so it is a complete span on its own.
- **Agents mark what the CLI cannot see.** `spektacular trace start <name> --type sub-agent|user-wait|work [--parent <id>]` starts a span and prints its id, `<trace_id>-<span_id>`; `spektacular trace stop <id> [--status ok|error] [--message <text>]` ends it. A span nests under the current step unless it names another open span, so helpers running at once each nest under the span that launched them; the id's trace routes it, and the command's own record, to the right run, orchestrated lanes and standalone spans included. A span left open when its parent ends or its run finishes is closed as `abandoned`. Mistakes are refused with a next action and record nothing; the commands never change workflow state, and with logging off they succeed and record nothing.
- **Every workflow tells agents what to mark.** One short shared paragraph is appended to every step instruction, straight after the step's own text, naming the step's span as the parent: mark each helper launch as a `sub-agent` span and give the helper its id, mark each question to the user as a `user-wait` span, and chain each stop with the next command. Orchestrated runs get the helper rule only. Every workflow skill carries the same paragraph, the epic child prompts pass each child its parent span, and the helper-launch library skills carry the helper rule.
- **The Harbor harness reads and checks spans.** Its session-log reader skips the new records, so its windows and timelines are unchanged; a shared span reader feeds new checks that every helper launch in the epic, implement-epic and goal-check suites was marked and that the spec suite's scripted question was a user wait; the metrics record counts trace calls overall and per plan task; and a self-check ties the harness's trace wording to the shared paragraph.
- **Documentation.** The site's logging page explains spans, their types, the trace commands, how spans end and the record format; the README lists the trace commands; the testing-architecture knowledge entry and the goto-to-the-current-step gotcha were updated.

## Why it matters

A workflow run's time goes to steps, plan tasks, helper agents and waits on the user, and none of that was visible in a log of commands. Steps and tasks are now recorded without the agent doing anything, and the agent only marks the helpers it starts and the questions it asks. Waits on the user are their own type, so the run statistics, the Harbor harness and the log viewer specs of this epic can tell the agent's own working time apart from the time a person took to reply.

## Deviations from the plan

- `ParseSpanRef` accepts any `<32 hex>-<16 hex>` id, all-zero parts included, as the acceptance criterion reads.
- The engine's open task span keeps the step that opened it (`step` in state); `RunEnded` gets `finished` or `abandoned` as its reason, and the recorder closes every leftover span as abandoned either way.
- `recordCommand` takes the logger `runRoot` builds, so a command's record and its spans share one logger. Command parents come from the state before the command, so a resuming goto and the resume probe are filed under the interrupted step.
- The trace block's lead-in is "**Mark spans.**", so it reads right where only the helper rule applies.
- Harbor counts trace calls from the session log's `trace` command records rather than transcript Bash calls, both fields sit under `totals`, and `report.py` was left unchanged because its comparison tables are pinned.
- The optional configuration-page note was left out; the logging page covers logging off.
- Two cmd log tests changed for intended behaviour: an old run's file gains an abandoned `span_end` after `--force`, and verbose state snapshots carry `step_span`. Tests comparing instructions across runs normalise the step span id.
