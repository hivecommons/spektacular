# Working context — spec 000073_log-mode-and-logger-providers

Epic: 000002_workflow-tracing-and-run-stats (created 2026-10-24, holds 000072_knowledge-access-index-and-prune plus the tracing specs as they are specified).

## Where this came from
Claude session 7749e883 (2026-10-24) investigated why spec/plan/implement are slow and wrote
`docs/workflow-performance-investigation.md`. Its numbers had to be rebuilt from agent transcripts,
Harbor parsers and the debug log. The quick fixes from that report were committed in c44e77c.
The user then asked for the CLI to record its own timings.

## The user's words (verbatim)
1. "I would like the spectacular cli to audit every operation, we are not going to be able to determine things like tool calls but we can track a metric every time that an agent calls spektacular. I generally when it starts a new step. I would also like to add additional metrics so that we add instructions to tell the agent to call spektacular whenever it starts and stops a task, that way we can audit the processes and time duration. This file should be jsonp and should be put into a logs directory. We already have debug mode. I suggest we refactor this debug mode to be a log mode, which has a log directory, a logger like we have plan providers. Log mode is enabled by default only logging time metrics and errors."
2. "Task start stop should be for everything, think like trace logging. In fact, maybe we should just use the otel trace approach and then write a custom output logger. Logs should stay out of git. Logs should have the agent name. We should look at how we can unify the work harbor is doing and replace as much of it as we can with the built in logger. It would be good to have a spektacular command that could show run stats. Eventually we could build a log viewer that renders to html"
3. "I would like to include the knowledge spec in this epic." (000072 is in the epic.)
4. This session: "yes user waits should be their own type so that we can potentially exclude these from timings, I think it makes sense that goto opens and closes spans, also because it is one less thing for the agent to do"

## Epic split (agreed in principle, session 7749e883; each spec still goes through spec new)
1. Log mode and logger providers (this spec, 000073): `debug:` -> `log:` (provider, directory, level); on by default at timing+errors; verbose keeps today's full records; `.spektacular/logs/`, out of git; every CLI call a span with agent name, run ID, step; `migrate` converts old config.
2. Trace spans for agent work: `goto` opens/closes step spans itself (decided); explicit `spektacular trace start/stop` with parent IDs only for work the CLI cannot see (tasks, sub-agent launches: test, verify, goal check, reviewers); user waits are their own span type (decided) so stats can exclude them.
3. `spektacular stats`: per-run/per-step timing, sub-agent wait, longest gaps, run comparisons, JSON.
4. Harbor on the built-in logs: replace `tests/harbor/shared/sessionlog.py`, `timeline.py`, transcript-based step/child/idle parts of `metrics.py`; keep token reading.
5. HTML log viewer (later): waterfall of a run's spans.
Order 1 -> 2 -> 3, then 4 and 5 in parallel. 000072 is independent.

## Facts established (for drafting)
- `internal/sessionlog` writes one JSON Lines record per CLI call when `debug.enabled` is set: command, duration, exit code, full response, workflow state before/after. Files in `.spektacular/debug/`, one per workflow session. `SPEKTACULAR_SESSION_LOG=off` disables it. Full responses make files large.
- `.spektacular/debug/` is already git-ignored; `.spektacular/logs/` needs the same rule.
- Harbor's `tests/harbor/shared/sessionlog.py` (378 lines) reads `.spektacular/debug/` for step windows and parallel runs; most of Harbor's ~3,000 lines of Python parse transcripts. Token counts/model usage cannot come from the CLI.
- "jsonp" read as JSON Lines.
- Providers pattern exists for plan/spec stores; a `log.provider` follows it, file provider first; stdout/OTLP later.

## OTel design (proposed in 7749e883, not contradicted by the user)
- Each `spektacular` call is a short-lived process, so cross-call spans can't be in-memory OTel spans.
- Use OTel's data model (trace ID, span ID, parent span ID, attributes, status); write separate start and end records to JSON Lines and pair them on read.
- `trace start` returns a span ID; `trace stop <id>` closes it; sub-agents get the parent span ID in their brief and pass `--parent` (works for concurrent children, unlike a span stack in workflow state).
- An OTLP exporter can come later as a second logger provider.

## Interview answers (user, 2026-10-24)
- Agent name comes from the project config (`agent`), not detected from the environment.
- "Use the id from the current status.json" -> `.spektacular/state.json` has no id field today (kind, step,
  created_at, data.name). Reading: run/trace ID held in workflow state, minted at `new`, reused on resume.
  Flag at the requirements draft.
- Log retention configured in config, default "last 10" (read as last 10 run log files).
- Old debug files: the user deletes them; "Nobody but me uses it anyway" -> no compatibility burden for
  debug mode or SPEKTACULAR_SESSION_LOG.
- Docs: the Debugging page is updated to show logging (plus configuration reference).
- Spec boundary (log mode + span format + per-call span here; goto step spans, trace verbs, user waits in
  spec 2): proposed, not objected to.
- No dependency on other epic specs: this spec goes first; 000072 is independent.
- Finding: user projects do not git-ignore `.spektacular/debug/` (docs tell users to add it), so init/migrate
  must add `.spektacular/logs/` to the ignore rules.

## Drafting mode (user, 2026-10-24)
- After confirming the overview: "This is fine, just generate it I don't want to answer any more questions."
  Draft every remaining section without per-section confirmation; apply review findings without asking;
  readings (run ID in state, last-10 retention, env override) become drafting assumptions.
