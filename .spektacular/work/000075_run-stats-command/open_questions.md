- **Whether 000073 and 000074 land with the record fields, constants and meanings this plan reads.** This depends on their implementations. The plan reads:
  - the `tracelog.Record` fields and `config.LogDir`;
  - the span types and end reasons;
  - an interrupted step ending at its last command;
  - abandoned ends written at the restart time;
  - command records parented to their step span;
  - `spektacular.task.id` on task spans and `spektacular.step` on agent spans.

  If only names differ, adapt to them. If a field's meaning, an end reason's timing or the parenting rule differs from their plans, STOP and ask the user, because the figures' definitions and the output contract for 000076 and 000077 rest on them.
- **Whether loading a real verbose-level logs directory of 10 runs stays under one second.** This depends on real log sizes, which only exist once 000073 and 000074 are running. If the manual measurement in the test plan exceeds one second, STOP and ask the user before adding anything that changes the design. For example, skipping verbose attributes with a pre-decode filter is an internal change and can be made; an index or cache file beside the logs cannot, because it would change the logs directory.

No other implementation-time uncertainties remain. Every design choice is settled in Architecture & Design Decisions, Data Structures and the assumption log.
