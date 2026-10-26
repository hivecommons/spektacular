---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# 000068_task-goal-validation: independent goal check for every implement task

## What was built

Implement runs no longer tick a task's acceptance criteria just because its tests passed. A new step, `check_goal`, runs between `verify` and `update_plan`, and works like this:

1. **The goal.** The CLI turns the current task into a goal: its acceptance criteria in plan order, plus its scope (its repo, the files its context section names, and a fixed rule that nothing belonging to another task may change). A legacy `Phase N.M` item of a plan written before tasks gets a goal the same way.
2. **Staging.** The implementing agent stages each code root's `git diff HEAD` and `git status`.
3. **The reviewer.** The agent launches a separate, read-only reviewer with a fresh context, handing it only the goal, the verification results and pointers to the work, never its own conversation.
4. **The verdicts.** The reviewer reads the files and the diff itself and returns one verdict per criterion (`met`, `not_met` or `impossible`), each with evidence.
5. **The decision.** `spektacular implement goal record` validates the reply and appends it to a per-plan outcome record (`goals.json` in the plan's folder). It also notes whether the work stayed unchanged while the reviewer ran, and returns the decision:
   - **accept**: on to `update_plan`, which then ticks exactly the criteria judged met;
   - **retry**: back to `implement`, which shows the reviewer's reasons. A round allows two retries.
   - **exhausted** or **impossible**: a stop. The user chooses keep fixing, accept the shortfall, skip, or (after fixing the plan) recheck, recorded with `implement goal decide`. A skipped task ticks nothing and is passed over.

`update_plan` refuses until the task's goal check is settled. `implement goal show` prints every goal, verdict, evidence and attempt count after the run.

Other changes:

- **Configuration.** The project setting `implement.goal_check.enabled` is on by default, and older configurations get it on too, with no migration. With it off, every step renders byte for byte as before. `implement.goal_check.models.<agent>` overrides the reviewer's model for one agent.
- **Reviewer helpers.** `spektacular init` (and `migrate`) installs a `spek-goal-reviewer` helper in each agent's own format, embedding one shared brief (also served as `spektacular skill review-task-goal`):
  - Claude: `haiku`, read and search tools only.
  - Codex: `gpt-6-luna`, read-only by instruction only.
  - Copilot: `Claude Haiku 4.5`, read and search tools only.
  - omp: `@smol`, read-only tools.
  - Bob installs no helper. Its reviewer is the built-in `explore` helper, and any agent without a named helper falls back to a fresh-context sub-agent.
- **Epic runs.** `epic merge` refuses a spec whose implement run is stopped at the goal check (`goal_check_open`). The `spek-implement-epic` skill treats both goal-check stops as genuine questions for the user.
- **Plan reader.** It now keeps each criterion's text and parses legacy phases, without changing `status` output.
- **Harbor suites.** The implement and implement-epic suites follow the new step. The implement-epic suite now checks that every task has recorded verdicts. A new goal-check suite seeds a task whose README criterion is missing while its tests pass (and a variant where everything is met), for Claude, Codex and Bob.
- **Design and docs.** The plan-task-graph design's tick rule and the documentation site describe the check, its verdicts, its configuration and each agent's limits.

## Why it matters

Before this change, the agent that wrote the code also decided whether the task's criteria were met, mostly from passing tests. A criterion that only the files could disprove, such as a missing documentation line, was ticked anyway. The independent reviewer judges from the work itself. Teams can now trust a ticked criterion, and work that falls short goes back for another attempt instead of being signed off. Because the CLI, not the agent, owns the goal, the verdicts and the retry bound, the check behaves the same under Claude, Codex, Copilot, omp and Bob.

## Deviations from the plan

**Goal package and commands**

- `goal_decision_due` refuses a new check while a stop awaits the user's decision.
- A reviewer reply may be a bare verdict array or wrapped in one Markdown code fence.
- Several refusals the plan did not list carry runnable fixes: `from_required`, `from_unreadable`, `plan_not_found` and `goal_format_unsupported`.

**Implement step**

- The work fingerprint ignores `.spektacular/` paths on both sides, because the agent and the CLI write there during the check.
- `check_goal` stores the item it rendered as workflow data `goal_item`, cleared at `analyze`. After a skip, the current-item rule has already moved on, and without this `update_plan` would act on the next task.
- `workflow.Config` also carries the registered repo names, so the step and `implement goal record` build the same goal.
- Walk tests use explicit `Goto`, because `Next()` picks a successor at random once `verify` has two. The worktree auto-commit tests now expect `goals.json` in the artifacts commit.

**Reviewer helpers**

- Codex ignores `sandbox_mode` in an agent file, so it was left out, and the Codex reviewer is read-only by instruction only.
- The default models were confirmed against each agent's docs on 2026-10-21.

**Session log and harbor**

- `implement goal record|decide|show` naming a spec with a running implement lane are now filed under that lane in the session log.
- The goal-check suite leaves the seeded work uncommitted so the diff shows it, and runs with `implement.worktrees: false`.

**Design and docs**

- As an orchestrated child, the run handed the design revision back. The orchestrator applied it with `design author` after the user approved it, and recorded it with `spec amend`.
- The homepage's pipeline node had the same wording as the Implement stage, so it was updated too.
