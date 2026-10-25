---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
epic: 000002_workflow-tracing-and-run-stats
---

# Feature: 000078_token-use-in-run-stats

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Spektacular's run reports will show how many tokens each run, step, task and helper agent used, and what that cost, beside how long it took. Time alone misleads: a step that took ten minutes because it did a great deal of work is fine, while one that took ten minutes doing very little is the problem worth fixing. Putting tokens next to duration lets maintainers judge each part of a run by both how long it took and how much work it did, and compare runs fairly.


<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [ ] **A run's report gives its token use**
  A report on one run must give the tokens the agent used during it (input, output, cache read and cache write), in total and for each workflow step, each plan task and each helper agent.
- [ ] **Helper agents' tokens are counted**
  Tokens used by helper agents must be counted and attributed to the helper launch that started them and to the step and task it was launched in, even when the helper is still running after that step has ended.
- [ ] **Tokens are attributed by when they were used**
  Each of the main agent's model requests must be attributed to the step and task that were running when the request was made; tokens used outside every step of the run are reported separately.
- [ ] **Time is set against work done**
  For the run and for each step, task and helper, the report must give a figure relating time to tokens: its working time (as the run statistics report defines it, wall time less time waiting on the user; for a helper, its own duration) per thousand output tokens, so a long step that did much work can be told from a long step that did little. Each helper's figure uses only its own time and tokens, even when helpers run at the same time.
- [ ] **The report gives cost**
  The report must give the cost of the run and of each step, task and helper, priced per model from the project's price table. With no price for a model, its cost is reported as unknown, never as zero; a total that includes tokens from an unpriced model gives the cost of the priced part, is marked incomplete and names the unpriced models.
- [ ] **The models used are listed**
  The report must name the models used in the run and the tokens used by each.
- [ ] **Comparisons include tokens**
  Comparing runs must include tokens, cost and the time-per-token figure alongside the timings, with each run's difference from the first.
- [ ] **Summaries include tokens**
  The summary of many runs must give, for tokens, cost and the time-per-token figure, the same median and 90th percentile it gives for timings.
- [ ] **Runs are matched to agent sessions without agent action**
  The system must find which agent session drove each run without the agent doing anything extra during the run; anything Spektacular installs once when it sets up a project does not count as agent action.
- [ ] **Claude Code sessions are supported**
  Token use must be read for runs driven by Claude Code, including its helper agents. For any other agent, token figures are reported as unknown with the reason.
- [ ] **Missing token data stays explicit**
  When a run's agent session cannot be found or read, its token and cost figures must be reported as unknown with the reason, and its timings are unaffected.
- [ ] **Reading token data changes nothing**
  Reading agent session data must not modify the agent's files or the logs.
- [ ] **The documentation describes token figures**
  User documentation must describe the token, cost and time-per-token figures, the price table and which agents are supported.


<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- Token figures must be reported beside timings so time can be judged against work done (user: "timing should be gauged on duration and tokens. That way you are not measuring something very complex which is slow against something simple and fast").
- Token use must be read from the agent's own session data, since Spektacular never sees model requests.
- A missing, unreadable or unrecognised session-data format must yield unknown token figures with a reason, never an error or a failed report.
- Model prices must come only from an optional price table in the project's configuration; no built-in prices are shipped, since prices change.
- The figures must extend the run statistics report defined by 000075_run-stats-command, on which this spec depends.
- Attribution must use the step, task and helper spans defined by 000074_trace-spans-for-agent-work.
- Existing report figures and field names must not change.


<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [ ] **Totals and breakdowns appear**
  For a Claude Code spec run, the report gives input, output, cache-read and cache-write tokens for the run, each step and each helper, and the step figures add up to the run total less the tokens reported outside any step.
- [ ] **Helper tokens land on their launch**
  For a run in which a helper agent was launched in one step and finished after the workflow had moved to the next step, all of that helper's tokens are reported under its launch and counted in the launching step's figures, not the next step's.
- [ ] **Tasks carry tokens**
  For an implement run of a two-task plan, each task shows its tokens and cost.
- [ ] **Time per token is reported**
  For a run, the run total and each step, task and helper show working time per thousand output tokens, and one with no output tokens shows the figure as not applicable rather than infinite or zero.
- [ ] **Cost follows the price table**
  With prices set for the models used, the run's cost equals the tokens of each type multiplied by that model's price for the type, summed. With one of two models unpriced, the run's cost covers the priced model only, is marked incomplete and names the unpriced model.
- [ ] **Models are listed**
  For a run that used two models, the report names both with the tokens each used.
- [ ] **Comparisons show tokens**
  Comparing two runs shows each step's tokens, cost and time per thousand output tokens for both, with the difference.
- [ ] **Summaries show tokens**
  Summarising five runs shows, for tokens, cost and time per thousand output tokens, the median and 90th percentile alongside the timings.
- [ ] **No extra agent step is needed**
  A run driven by Claude Code with no change to how the agent works produces token figures.
- [ ] **Other agents are unknown, not zero**
  For a run driven by another agent, token and cost figures are shown as unknown with a reason.
- [ ] **A missing session is unknown**
  When the agent's session data has been removed, the run's token figures are unknown with a reason and its timings match the report without token data.
- [ ] **Agent files are untouched**
  Every agent session file and log file is byte-identical before and after a report.
- [ ] **The docs describe token figures**
  The documentation site describes the token, cost and time-per-token figures, the price table and supported agents; the site builds without errors.


<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- Read Claude Code's session transcripts (`~/.claude/projects/<project>/<session>.jsonl` and the session's helper transcripts), which record each request's time, model and token usage.
- Prefer matching a run to its transcript by finding the run's own `spektacular` commands in the transcript's tool calls, which needs no hook; a hook that `init` installs to record the session's transcript path into the run log is the fallback.
- Harbor's `tests/harbor/shared/transcripts.py` already reads tokens, models and cost from Claude, Bob and Codex logs and is a useful reference.


<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- For a real implement run in a Claude Code session that drove only that run, the report's run total, including tokens outside any step, matches Claude Code's own usage totals for the session within 1%.
- The maintainer can rank a run's steps by time per thousand output tokens and identify steps that are slow for the work they did.


<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- Showing tokens or cost in the HTML log viewer, or changing Harbor's own token reading.
- Budgets, alerts or limits on token use.

