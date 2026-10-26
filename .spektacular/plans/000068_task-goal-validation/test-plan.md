---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Test plan: 000068_task-goal-validation

Manual procedures for the success metrics and manual reviews that the plan's Testing Approach lists as "captured in the implementation test plan". Run every command from the `spektacular` repo root once `spek/000068_task-goal-validation` is merged, with Docker, harbor and agent logins set up as the README's "End-to-end (Harbor)" section describes. Each harbor run writes its results to `tests/harbor/jobs/<timestamp>/<trial>/`. The verifier output is in `verifier/test-stdout.txt`, the agent's logs are in `agent/`, and the CLI's session log is in the `spektacular/debug/` artifact.

Before any run, check that the verifier rules can fail. This takes seconds and needs no agent:

```
uvx --with pytest pytest -p no:cacheprovider tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py
```

The oracle (scripted) runs must also pass before you spend agent runs. First render a build: run `make harbor-test-goal-check-claude SCENARIO=unmet` once, or repeat the `_harbor-test-goal-check` recipe's copy steps by hand. Then point harbor's oracle agent at that build, and repeat with `SCENARIO=met`:

```
harbor run -p tests/harbor/.build/goal-check-workflow-claude-unmet -a oracle -o tests/harbor/jobs
```

Expect 6 passed and 2 transcript checks skipped. The implement-epic oracle runs the same way against `tests/harbor/.build/implement-epic-workflow-claude`, and expects 54 passed.

## Success metrics

### 1. No false "met"

- **What to measure**: in the unmet scenario, the reviewer's first check must judge criterion 3 (the README documents `todo count`) `not_met` in every run.
- **Threshold**: 10 of 10 runs per agent, on each agent's default reviewer model.
- **How**: run each line 10 times. Keep `implement.goal_check.models` unset.

  ```
  make harbor-test-goal-check-claude SCENARIO=unmet
  make harbor-test-goal-check-codex SCENARIO=unmet
  make harbor-test-goal-check-bob SCENARIO=unmet
  ```

  For each run, read `test_scenario_verdicts` in `verifier/test-stdout.txt`, or run this against the `spektacular` artifact:

  ```
  spektacular implement goal show 20260301000000-todo-count --format json | jq '.items[0].checks[0].verdicts[2].verdict'
  ```

- **Expected result**: `"not_met"` in 10 of 10 runs for each of Claude, Codex and Bob. A single `"met"` fails the metric.
- **Who / when**: the implementer, before release, and again whenever the reviewer brief (`templates/partials/goal-reviewer-brief.md`) or a default reviewer model changes.

### 2. Finished work is not sent back

- **What to measure**: in the met scenario, every criterion is judged `met` on the first attempt.
- **Threshold**: at least 9 of 10 runs per agent.
- **How**: run each line 10 times.

  ```
  make harbor-test-goal-check-claude SCENARIO=met
  make harbor-test-goal-check-codex SCENARIO=met
  make harbor-test-goal-check-bob SCENARIO=met
  ```

  Count the runs in which `test_scenario_verdicts` passes. That test requires exactly one check, all `met`.

- **Expected result**: 9 or more passing runs out of 10, for each agent.
- **Who / when**: the implementer, before release, alongside metric 1.

### 3. Reviewer spend under 10% of the implementer's

- **What to measure**: the reviewer's token cost as a share of the implementing agent's, for the same task, on agents where a cheaper model is chosen (Claude, Codex, Copilot, omp).
- **Threshold**: under 10%.
- **How**:
  - **Claude**: in an unmet run, read the `agent/claude-code.txt` transcript. Sum the `usage` of every assistant message whose `parent_tool_use_id` is the `spek-goal-reviewer` Agent call; that is the reviewer. Sum the usage of the messages with no `parent_tool_use_id`; that is the implementer. Price each sum at its model's rate (haiku for the reviewer, the session model for the main agent) and divide.
  - **Codex**: use the per-agent usage in the Codex session log (`~/.codex/sessions/`) for the run.
  - **Copilot and omp**: use the usage in the session's own log, during the manual runs in review 2 below.
- **Expected result**: reviewer cost divided by implementer cost < 0.10 on each agent measured.
- **Who / when**: the implementer, before release, on one unmet run per agent.

### 4. Under 2 minutes added per task

- **What to measure**: the time the goal check adds to a single-task implement run.
- **Threshold**: under 120 seconds per check.
- **How**: in the session log of a met-scenario run (`spektacular/debug/*.jsonl`), take the time from the `implement goto` to `check_goal` to the `implement goal record` that follows it. `python3 tests/harbor/shared/timeline.py <artifact>/spektacular/debug` prints the timeline. Repeat for 5 met runs per agent and take the median.
- **Expected result**: a median under 120 s for each of Claude, Codex and Bob.
- **Who / when**: the implementer, before release, using the runs from metric 2.

## Manual reviews

### 1. Harbor suites touched by the change

- **What**: every harbor suite this change touches passes with real agents, as the testing-architecture knowledge entry requires.
- **How**:

  ```
  make harbor-test-implement
  make harbor-test-implement-epic-claude
  make harbor-test-implement-epic-codex
  make harbor-test-implement-epic-bob
  make harbor-test-goal-check-claude SCENARIO=unmet
  make harbor-test-goal-check-codex SCENARIO=unmet
  make harbor-test-goal-check-bob SCENARIO=unmet
  ```

- **Look for**: reward 1.0. In implement-epic, `TestGoalCheck` passes for every spec. In goal-check, the two transcript checks pass on Claude, the reviewer-input check passes on Bob, and both are skipped on Codex.
- **Passing**: every run above has reward 1.0.

### 2. Copilot and omp, which harbor cannot drive

- **What**: under Copilot and omp, an implement run records verdicts for every task and launches `spek-goal-reviewer` on the cheaper model.
- **How**:
  1. Make a throwaway project: `git init`, `spektacular init copilot`, then write a one-task plan. The goal-check suite's `environment/seed/` files work, applied as `seed.sh` does.
  2. Run `/spek-implement <plan>` in Copilot CLI. Repeat with `spektacular init omp` and omp.
  3. Afterwards, run `spektacular implement goal show <plan> --format json`.
- **Look for**:
  - every task has at least one check, with one verdict and evidence per criterion;
  - the agent's log shows `spek-goal-reviewer` running on `Claude Haiku 4.5` (Copilot) or on the model omp's `smol` role maps to (omp);
  - omp accepts `model: "@smol"` and `tools: read, grep, glob` in `.omp/agents/spek-goal-reviewer.md`.
- **Passing**: verdicts are recorded for every task, and the helper ran on the cheaper model. If omp rejects `@smol` or the tool names, record it and adjust `internal/agent/subagents.go` and `templates/subagents/omp.md`.

### 3. Codex runs the reviewer on its mini model

- **What**: Codex's transcript carries no sub-agent signal, so a person confirms the reviewer's model.
- **How**: after `make harbor-test-goal-check-codex SCENARIO=unmet`, or a local Codex run in a project initialised with `spektacular init codex`, open the Codex session log for the run.
- **Look for**: a `spek-goal-reviewer` sub-agent session on `gpt-6-luna` that changed no files. Also check `tree_unchanged: true` on every check in `implement goal show`.
- **Passing**: the reviewer ran on `gpt-6-luna` and left the work unchanged.

### 4. Bob uses its explore helper

- **What**: under Bob, the built-in `explore` helper is the reviewer and reads the staged diff.
- **How**: in a `make harbor-test-goal-check-bob SCENARIO=unmet` run, read `agent/trajectory.json` and Bob's own logs (`/root/.bob/logs`, collected per `tests/harbor/agents/bob-artifacts.toml`).
- **Look for**: a `Running subagent` call for `explore` between the `check_goal` goto and the `implement goal record`, whose tool calls read `.spektacular/tmp/20260301000000-todo-count/goal-check/todo-app.diff` and `README.md`.
- **Passing**: explore was the reviewer and read the staged files. If it cannot be named, or cannot read them, the step's generic fresh-context fallback is acceptable. Record it as a discovery, and make the docs say Bob's reviewer runs on Bob's default model.

### 5. An epic stop at the goal check reaches the user and blocks the merge

- **What**: when a child's goal check stops, the orchestrator relays a `QUESTION:` from that spec, and the spec is not merged until it is answered.
- **How**:
  1. In a copy of the implement-epic fixture, change one spec's README criterion so it cannot be met. Alternatively, have the child's reviewer return `impossible`.
  2. Run `/spek-implement-epic` interactively.
  3. While the question is open, run `spektacular epic merge --data '{"spec":"<spec>"}'`.
- **Look for**: the orchestrator relays a `QUESTION: <spec>` naming the not-met or impossible criteria with evidence. The merge is refused with `goal_check_open`, and its `next_action` names the resume command. After you answer, the run goes on and the spec merges.
- **Passing**: you are asked, and the merge is refused until you answer.

### 6. The documentation pages read correctly

- **What**: the docs site's goal-check content reads correctly on the built site.
- **How**: in the docs repo, run `npm ci && npm run build && npm run preview`, then open `/how-it-works/` (the Implement stage and the step tree), `/plan-tasks/` (Acceptance criteria), `/configuration/` (the `implement` key and the YAML example) and `/` (the pipeline's Implement node).
- **Look for**: the three verdicts and what each does, the retry and stop options, the `implement.goal_check` keys, each agent's default model and limits, no em dashes and no broken layout.
- **Passing**: every page renders and reads as described, in light and dark themes.
