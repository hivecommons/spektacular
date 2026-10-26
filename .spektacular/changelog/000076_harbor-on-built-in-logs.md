---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# 000076_harbor-on-built-in-logs: Harbor measures runs from Spektacular's own logs

## What was built

Harbor, Spektacular's end-to-end test harness, no longer measures runs by parsing each agent's transcript and Spektacular's old debug output. Every step timing, helper agent, wait on the user, gap and epic timeline now comes from the logs Spektacular keeps under `.spektacular/logs/`, recorded at the default log level, and Harbor reads them only through `spektacular stats`. Transcripts stay the source only for tokens, models, cost, turns and what each helper was told.

- **Two new `stats` listings.** `spektacular stats commands` lists every recorded command (its run, workflow, step, the step it moved to, arguments, timing, exit code and error message), and `spektacular stats spans` lists every helper agent, wait on the user and other marked span. Both include what was recorded outside any workflow run, such as an epic orchestrator's commands and helper spans. Both take the usual selection, `--dir` and `--format json`, plus `--outside-runs`, and are never logged. Golden JSON files pin their field names. The documentation site's Run statistics page describes them.
- **A run log reader for Harbor** (`tests/harbor/shared/runlog.py`). It runs the `stats` subcommands, parses their JSON into commands, spans and lanes, and gives the views the harness needs: each lane's start, finish, resumes and children; the orchestrator span each child was launched under; the orchestrator's reaction, merges, steps entered and changelog writes. Any failure is a stated reason, never an empty answer.
- **Metrics record schema 3.** For every agent alike, it records each workflow run's steps with their working, helper-wait and user-wait split and helper counts, every helper with its spec and step, and user-wait, helper-wait and gap totals, in place of the single longest gap. Missing logs make those fields unknown with the reason, never filled in from a transcript, and gathering metrics still never fails a run or changes its reward.
- **Delegation checks judge from spans.** Required helpers come from `sub-agent` spans per step attempt, and per-spec children from the orchestrator's spans. Brief checks still read prompts, joined to spans by launch time. An implementing child's brief must name its worktree folder, tied to `internal/worktree`'s `Dir`.
- **Every suite verifier reads the logs through the reader**, with its pass and fail rules unchanged. The test projects the suites seed record at the default level.
- **The epic timeline** reports from the logs. Its hand-off gaps are measured from the orchestrator's `sub-agent` spans, since `status` is never logged.
- **The cross-run report** shows the per-step split, a helper table, a new two-trial `--compare`, and medians of user wait, helper wait and gap time. Older records still load.
- **Removed:** the old debug-log and span readers, the transcript readers nothing used any more, and the reference runs' old-format logs. The reference trimmer copies new-format logs. A self-test holds the shared scripts to 3,000 lines. The README and three knowledge entries describe the new arrangement.

## Why it matters

Maintainers get the same measures for Claude, Codex and Bob, including helper counts for agents whose transcripts show no helpers. They get a full per-step split of working, helper-wait and user-wait time, and an epic timeline that works on any trial. Every figure has one definition, in Go, and `spektacular stats run --dir <trial>/artifacts/spektacular/logs` on a collected trial shows the same numbers as its metrics record.

## Deviations from the plan

- **The line ceiling.** The plan's 2,000-line ceiling could not be met. After every planned removal, the shared scripts total 2,990 lines (3,299 before this plan's Harbor changes), because the run log reader and the two-trial comparison were added and the remaining transcript readers serve kept checks. The user chose to accept the size reached. The spec's success metric now reads "The harness's old parsers are removed and its shared measurement scripts do not grow, with no measure parsed in two places", and the self-test ceiling is 3,000.
- **The baseline task** ("Record baseline Harbor pass rates") was run by the epic orchestrator: 50 jobs `base76-*` from commit bfd5dc1. Their pass counts are in the plan's changelog and the test plan.
- **Per-spec child matching.** The per-spec child rule and the orchestrator helpers' specs match each lane to its own orchestrator span (`RunLog.child_span`), not "any span open at the lane's start", so two children launched together each need a span.
- **Reordered work.**
  - The verifiers' delegation call sites moved in the delegation task.
  - `test_sessionlog_rules.py` was deleted in the verifier task.
  - Transcript placement helpers stayed in `metrics.py` for one task, until delegation moved onto spans.
- **Images.** The spec, plan and repo images' config lines were removed rather than set to `standard`. The seed scripts switch logging back to `standard`.
- **`--compare` pairing.** It pairs workflows by kind and name, else in order within a kind, because generated spec names differ per trial.
- **Not changed:** the epic suite's `status` poll check had already been retired by 000073, and the suites' `test.sh` timeline calls already read `.spektacular/logs`.
