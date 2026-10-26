---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Child agents in harbor runs

## What was built

Spektacular's harbor end-to-end runs now record, judge and report the child agents a coding agent starts.

**Every run records its children.**
- The run metrics record (`metrics.json`, now schema version 2) lists each child agent the run started, nested children included.
- Each entry shows:
  - which agent started it;
  - its kind and description;
  - the full context it was given, and that context's size;
  - whether it really ran in the background;
  - its start and end, and its own reported run time;
  - its models and output;
  - how it ended, and the first line of what it handed back;
  - the spec and workflow step it was started in.
- Every step entry counts the children it started, so delegations a step only suggests, such as plan discovery, show whether they happened.
- The run records its idle time: the longest stretch with no agent, child or Spektacular activity.
- Codex and Bob records mark what their logs cannot show as unknown, never zero.

**The local report shows them.**
- `make harbor-report ARGS='--run <trial>'` adds an idle line and a table with one row per child.
- `--prompts` prints every child's full context.
- Runs idle for more than a quarter of their wall time are flagged `◀ idle`.

**Claude runs fail when delegation is skipped.** A new shared module holds a hand-kept oracle of the steps whose instructions require a child agent, and rules over the placed children:
- the spec workflow's independent review;
- the implement workflow's test, verify and goal-check steps;
- one child per spec in the epic workflows.

The spec, goal-check, epic and implement-epic suites now fail a Claude run when:
- a required step completed without a child;
- a required child was not given what its instructions say to pass, such as its spec, the project root, that its run is orchestrated, its worktree paths, the instruction to resume, or the staged spec to review only as written;
- an epic orchestrator set a timed wake-up, timer or sleep while children ran.

A self-check ties the oracle to the step templates' wording. Codex and Bob runs are recorded but never failed by these rules.

**The instructions say it plainly.**
- The spec review and the implement test and verify steps now state that an agent able to start sub-agents must delegate, and that doing the step itself "does not complete this step".
- The epic skills forbid timed wake-ups while children run.
- The implement-epic skill names each spec's worktree paths in its child's prompt.

**The headless background-child limit is settled by a recorded run.**
- A new probe task (`make harbor-test-background-child`) ran one long background child under headless Claude Code 2.1.310. The child was stopped about ten minutes after the main agent ended its turn, and no completion notice woke the session.
- The epic skills keep their foreground workaround, now grounded in that recorded outcome rather than an unverified claim. They no longer present being told "no one answers" as the reason by itself.
- The epic suites check that workaround: per-spec children run in the foreground, and no orchestrator turn ends while one runs.

**Two checks now judge what ran.** These read Spektacular's own session log rather than matching the text of the agent's commands, so piped, chained, looped or variable-built commands no longer fail correct runs:
- the plan suite's walkthrough-before-finished check;
- the implement suite's project and per-repo changelog checks.

The plan suite no longer fails a run whose discovery used no child. The plan and implement suites now tell the agent, like every other suite, that the session is headless and no one answers between turns.

**Reference runs are checked in.** The 2026-10-23 reference runs (Sonnet 4.6 and 5.5 epic planning, Sonnet 5.5 plan and implement) and the probe run are checked in under `tests/harbor/shared/tests/data/`. A trimming script blanks every message to the same length and refuses to write a copy whose computed values differ, so the acceptance criteria are proven offline.

## Why it matters

Before this, the tests made the main agent hold its children inside one turn, which real sessions do not do. They never checked that a required independent review actually happened, so silent regressions passed unnoticed. On 2026-10-23:
- Sonnet 5.5 skipped the spec review child;
- both 4.6 and 5.5 skipped every implement test and verify child in the epic runs;
- a 1200-second wake-up left an epic-planning session idle for 16 of its 21 minutes.

Maintainers can now see how delegation changes between models and releases, and such regressions fail the run.

## Deviations from the plan

- **Probe outcome.** The probe found background children stopped, so the plan's "stopped" branch was taken. The epic suites check the foreground workaround instead of background children with notice-driven turns.
- **"Same message" is not checked.** "Children launched from the same assistant message" cannot be told from the log, so the foreground rule checks foreground children and that no turn ended while one ran.
- **Timers include any sleep.** User decision: any orchestrator shell `sleep` counts as a timer, foreground or background. The reference run's polling sleep ran in the foreground.
- **Close phrasings count.** User decision: the spec-review brief passes on "only what is written" or "exactly as written". "Orchestrated" passes on `"orchestrated":true` or the skill's own words "run is orchestrated".
- **Context rules split.** They are `epic_context_gaps` (epic runs only) and `review_context_gaps`, plus `missing_spec_children`.
- **Readers corrected while building.**
  - Children started by children stream no messages, so their models and output are unknown.
  - A turn ends at the main agent's own last message, with turns delimited by `system/init` because results can be written late.
  - A stopped child has no hand-back.
- **Two probe runs.** The first probe run was invalid: Claude Code refuses a standalone `sleep 300`. The probe was fixed and run again.
- **Work in parallel.** The probe task was built in parallel with Milestone 3, and its files landed in that milestone's commit.
- **Corrected durations.** The plan's s55 child durations were wrong. The notifications report 109.605/111.142/145.246 s.

## Notes for maintainers

- A harbor run leaves root-owned `agent/sessions` directories under the gitignored `tests/harbor/jobs/`. While they are there, `go test ./...` cannot walk the tree, so move or remove old jobs first.
- Re-run `make harbor-test-background-child` after a Claude Code upgrade. If background children start to survive, change the epic skills, `delegation.PROBE_OUTCOME` and the epic suites together.
