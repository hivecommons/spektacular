---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Token use in run statistics

## What was built

Spektacular's run reports (`spektacular stats run`, `compare` and `summary`) now give token use and cost beside the timings, for runs driven by Claude Code. Spektacular never sees the agent's model requests, so it reads Claude Code's own session files, read-only, from `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR/projects`, or `--sessions-dir <path>`). The agent does nothing extra and nothing is installed.

- **Finding the session.** A run's driving transcript is the one whose shell calls ran the run's own `<kind> new` and `<kind> goto` commands, matched by time (within a second) and command words, and never by a call whose `--data` names another workflow. A run resumed in a second session is read from both; an orchestrated child is found in its own helper transcript inside the orchestrator's session; a transcript that drove several runs is split between them.
- **Placing tokens.** Each main-agent request counts in the step entry and plan task running when it was made, or outside any step. Each helper transcript, with every helper it started, is matched to its recorded `sub-agent` span by the span id in its brief, then by launch order, and its tokens count on that launch and in the step and task it was launched in, even when it finishes later. A helper with no recorded launch is still counted and listed as unmatched. When the session drove only this run, the tokens Claude Code counted but never wrote into the transcript (its `cost-state` totals less the transcript's requests) are reported as session overhead, so the run total equals Claude Code's own totals. Steps plus the tokens outside any step always equal the run total.
- **Figures.** Input, output, cache-read and cache-write tokens for the run, each step, task and helper; the models used; cost from an optional `stats.prices` table in `config.yaml` (per million tokens, all four prices required, none built in), unknown rather than zero for an unpriced model, and marked incomplete with the unpriced models named; and working time per thousand output tokens (a helper uses its own duration), `null` with no output tokens.
- **Unknown, never an error.** Another agent, a missing sessions folder, no matching session, an unreadable file or an unrecognised format give unknown token figures with a reason code, and every timing stays as it was.
- **Additive output.** The run report's JSON gains a `tokens` object on each step, task and helper row and a top-level `tokens` section; comparisons and summaries append six figures (`input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, `cost`, `working_ms_per_1k_output`) for the totals and each step. A contract test strips the new keys from the regenerated golden files and requires them to equal copies taken before the change. The text tables gain token columns, a Tokens block, a Models table, unmatched helpers and step tables for tokens, cost and pace in comparisons and summaries.
- **Documentation.** The Run statistics page and the configuration reference describe the figures, the session matching, the price table and the unknown reasons; the README notes the feature.

The code is a new read-only leaf package for Claude Code sessions (`internal/agentusage`), pure attribution inside `internal/runstats`, the optional `stats` section in `internal/config`, and the `--sessions-dir` flag in `cmd/stats.go`. `cmd` tests point `CLAUDE_CONFIG_DIR` at an empty directory so no test reads a developer's sessions.

## Why it matters

Time alone misleads: a step that took ten minutes because it did a great deal of work is fine, while one that took ten minutes doing very little is the problem worth fixing. With tokens and pace next to duration, maintainers can judge each part of a run by both how long it took and how much work it did, and compare runs fairly.

## Deviations from the plan

- 000075 had no helper row type: helpers and user waits shared `Wait`. A new `HelperRow{Wait; Tokens}` (embedded, so the JSON stays flat) is the helpers' row type; user waits carry no tokens.
- The token JSON types were written with the attribution task, and `TokenInput` gained `Dir` and an internal memo.
- Matching refuses a shell call whose `--data` names another workflow, so concurrent orchestrated children are not confused; launch-order fallback pairs a response's concurrent launches with the latest spans in start order.
- With no sessions read at all, the reason is the reader's own problem code; `session_not_found` means sessions were read but none drove the run.
- A transcript that drove two runs was briefly counted as owned for overhead; the test author found it and it was fixed before the task was accepted.
- Comparisons and summaries show the step token figures as three extra step tables rather than extra columns.
- The golden run, comparison and summary fixtures now carry command arguments and fixture sessions, so the goldens pin known token figures.
- The incomplete-cost note in the tables also says where prices go; the README's lead-in no longer says reports read no agent transcript.
- Session overhead is unknown (`several_runs`) for orchestrated children, because the orchestrator's session holds other runs; this is by design.
