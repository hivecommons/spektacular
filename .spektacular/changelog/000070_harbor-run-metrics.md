---
created_date: "2026-10-23"
document_status: final
closed_date: "2026-10-23"
---

# Harbor run metrics: a standard record for every run, and a report to read them

## What was built

Every automated harbor run of a coding agent through Spektacular now leaves a metrics record, `verifier/metrics.json`, beside its pass-or-fail reward. A local report reads those records across collected jobs.

**The record** has the same shape for Claude, Codex, Bob and oracle runs, and for every suite:
- **Identity:** suite, scenario, agent, model, Spektacular version, commit and dirty state, taken from a `build.json` each make target writes.
- **Result:** the reward as written, and the check score overall and per test class (or per module for function-style suites), from pytest's JUnit XML.
- **Totals:** wall time, model time, turns, cost, and input, output, cache-read and cache-write tokens. Claude's come from the `result` summaries at the end of its transcript. When background sub-agents split a run into several summaries, turns are summed, the cumulative figures come from the last summary, and wall time is the transcript's first-to-last span. Bob and Codex totals come from the ATIF trajectory harbor writes, with Codex's own rollout as a fallback.
- **Steps:** each workflow the run drove (spec, plan or implement), with every step it entered, in order, its duration and the characters the agent wrote there. Durations come from Spektacular's own session log, so they mean the same for every agent. Characters are attributed in one of three ways: by time; by the agent's own `goto` calls for Bob, whose log has no times; or, in orchestrated Claude runs, by which workflow a sub-agent's launch prompt names.
- **Sub-agents:** started, completed, failed and stopped, and each sub-agent's duration.
- **Unknowns:** anything a log does not supply is `null` and listed in `unknown` with a reason, never written as zero.

The recorder runs in each suite's verifier after the reward is written, and its failure is ignored. Recording metrics therefore never changes a run's result. All eight suites are wired the same way, and the implement suite now renders into `.build/` like the rest.

**The report** (`make harbor-report`, `tests/harbor/shared/report.py`) reads only each trial's record and `result.json`, so it needs no `sudo` for the root-owned files runs leave behind. It has three views:
- a table of every recorded run, grouped by suite and scenario;
- `--run <trial>`, one run's step breakdown, flagging any step over a third of the run's wall time;
- `--baseline … --candidate …`, the per-suite median of time, turns, tokens, cost and check score before and after a change, flagging any measure that at least doubled and any check-score drop of 0.1 or more.

The README's harbor section documents the record's fields, which fields each agent leaves unknown, and the report's commands. Self-checks under `tests/harbor/shared/tests/` cover the readers, the step windows, the recorder's failure paths, the report, the suite wiring and the README. That is 228 checks, run locally without a container.

On the reference Claude plan run, the record and the report reproduce the earlier hand analysis without any script: the `assemble` step took 368.8 s of a 948.4 s run (39%) and wrote 77,921 characters, and it is the only step flagged.

## Why it matters

Before this, a run reported only pass or fail, and harbor's cost and token fields were empty. The people improving Spektacular could not tell which workflow was slow, why, or whether a change made it faster, cheaper or better. Now one report command finds where a workflow spends its time, such as the planning step that retypes text it has already written. A before-and-after comparison judges whether a fix helped, in place of an impression.

## Deviations from the plan

- **Spec lanes.** The session-log reader gained an optional `kinds` parameter. The spec suite logs to `spec_<name>` lane files, which the existing reader never treated as lanes. The recorder passes spec, plan and implement, while the default (and so the epic timeline and verifiers) is unchanged.
- **Runs with no workflow.** A run that drove no workflow (the repo suite) records `workflows: []` with nothing unknown. Only a missing or empty session log makes the steps unknown.
- **Resumed lanes.** A lane resumed by a second child within one log file starts a fresh step window at the resume, so the hand-off gap is never counted as time in a step.
- **Uncached input.** ATIF prompt tokens and Codex's rollout input tokens include cached tokens. The record subtracts them, so `input_tokens` means uncached input for every agent.
- **Codex turns.** Only Codex `token_count` events carrying usage count as turns. The others are rate-limit updates.
- **Wall-time fallback.** It applies to any run whose agent log leaves wall time unknown, oracle runs included, not only Bob. It uses Bob's shell logs first, then the session-log span.
- **Orchestrator output.** In orchestrated Claude runs, characters from sub-agents that name no workflow, and characters written outside every step window, count toward `orchestrator_output` along with the main agent's.
- **Extra self-checks.** Two self-checks the plan did not ask for were added: a static check of the suite wiring, and a check that keeps the README in step with the record.
- **Fixes from testing and review.** The test and review steps found and fixed these before acceptance:
  - `run.agent` listed as unknown when the log had supplied it;
  - a non-object record skipped silently;
  - `--run <trial dir>` failing outside the default jobs directory;
  - a floating-point miss at the exact 0.1 score-drop threshold;
  - several README statements that did not match the code.
- **Accidental trial.** During verification, one real spec-workflow Claude trial was started by mistake, by executing the output of `make -n harbor-test-*`. It was stopped during container setup and fully cleaned up. The gotcha is recorded in the plan changelog.
