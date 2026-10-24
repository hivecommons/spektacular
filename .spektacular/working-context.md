# Working context — spec 000074_trace-spans-for-agent-work

Epic: 000002_workflow-tracing-and-run-stats. Spec 2 of the tracing split. Builds on
000073_log-mode-and-logger-providers (log mode, OTel-model records, run/trace ID from workflow state,
per-command spans). 000072 (knowledge) is independent.

## The user's words (verbatim)
- "I would also like to add additional metrics so that we add instructions to tell the agent to call spektacular whenever it starts and stops a task, that way we can audit the processes and time duration."
- "Task start stop should be for everything, think like trace logging. In fact, maybe we should just use the otel trace approach and then write a custom output logger."
- "yes user waits should be their own type so that we can potentially exclude these from timings, I think it makes sense that goto opens and closes spans, also because it is one less thing for the agent to do"
- On 000073: "This is fine, just generate it I don't want to answer any more questions." Applied to the
  whole epic: specs are drafted without per-section confirmation and review findings applied without asking.

## Decisions
- `goto` opens and closes step spans itself (user).
- User waits are their own span type, so stats can exclude them (user).
- Explicit trace start/stop only for work the CLI cannot see: sub-agent launches (test, verify, goal check,
  reviewers, epic children) and waits on the user (session 7749e883 proposal + user's "one less thing").
- Where the workflow already knows a unit's boundary (implement's current task), the CLI records it too
  (extends the user's reasoning; drafting assumption).
- Cross-process spans: separate start and end records paired on read; sub-agents get the parent span ID
  in their brief and pass it, which works for concurrent children (7749e883 proposal).

## Dependencies
- 000074 depends on 000073 (record format, run ID). Recorded in the epic at the interview step (order
  1 -> 2 -> 3 agreed in 7749e883).
