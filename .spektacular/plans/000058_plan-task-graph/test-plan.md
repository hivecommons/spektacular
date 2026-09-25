---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# Test plan: 000058_plan-task-graph

The mechanisms behind every success metric are covered by automated tests (`cmd/plan_export_test.go`, `cmd/plan_status_progress_test.go`, `cmd/implement_task_test.go`, `cmd/implement_task_run_test.go`). What remains is observing adoption after release, plus two agent-behaviour acceptance criteria no automated suite exercises.

## Success metrics (post-release observation)

### 1. Hive imports plans through `plan export` as its primary path

- **What to measure**: the share of Hive imports of plans written after release that use `spektacular plan export <name> --format json`, rather than the `tasks.json` / `plan.md` fallback. Target: 100% within the first month.
- **How**: in Hive's run logs, count import events per path for plans created after the release date (Hive's `src/pkg/spektacular/runner.go` import path).
- **Expected result**: no fallback-path imports for post-release plans.
- **Who / when**: Hive maintainers, one month after release.

### 2. Every plan authored in the first month exports on its first attempt

- **What to measure**: first-attempt `plan export` failures for plans authored after release. Target: 0.
- **How**: for each plan created after release, run `spektacular plan export <name> --format json` and record the exit code; cross-check Hive's import error logs for `plan_structure_invalid` or `plan_task_invalid`.
- **Expected result**: exit code 0 for every plan; no structural errors in Hive's logs.
- **Who / when**: Spektacular maintainers with Hive maintainers, one month after release.

### 3. Hive drives implementation through single-task runs, and progress matches the work

- **What to measure**: whether Hive starts implementation with `implement new --data '{"name":…,"task":…}'`, and whether `plan status <name>` `progress.tasks_completed` matches the tasks Hive recorded as done, with no manual progress edits.
- **How**: for a sample of imported plans, compare Hive's task completion records with `spektacular plan status <name>` → `progress` and `tasks[].completed`.
- **Expected result**: the counts and the per-task completion match exactly for every sampled plan.
- **Who / when**: Hive maintainers, one month after release.

### 4. No task started before its dependencies, and no human task handed to an agent

- **What to measure**: reported cases of an orchestrator starting a task with incomplete dependencies, or giving a `human` task to an agent, because of export output. Target: 0.
- **How**: review issues on hivecommons/spektacular and Hive for such reports; for any report, check the export's `depends_on` and `execution` for the task against its `plan.md` lines.
- **Expected result**: no reports, or every report traced to a cause other than export output.
- **Who / when**: maintainers, one month after release.

## Agent-behaviour acceptance criteria (pre-release, manual)

### 5. Mixed work is split on the reference scenario (Phase 4.1)

- **How**: in a scratch project with one registered repo, write a spec for "add a release workflow to the repository, then create the production signing secret it needs", run `/spek-plan` on it to completion, then `spektacular plan export <name>`.
- **Expected result**: the plan has an `agent` task that adds the release workflow and a separate task with `**Execution:** human — <reason naming the secret/credential>` whose `**Depends on:**` lists the agent task's id; the walkthrough names the human task and its reason before asking for sign-off.
- **Who / when**: a maintainer, before release.

### 6. An agent can be asked to implement one task (Phase 3.3)

- **How**: in a project with a saved task-format plan of at least two open tasks, ask the agent in plain words to "implement the <task title> task of <plan name>" (via `/spek-implement`).
- **Expected result**: the agent looks up the id (`plan export <name> --format json`) and starts `implement new` with that `task`; `spektacular implement status` shows `"task": "<id>"`; at the end, only that task and its criteria are ticked, and the run finishes without test plan or feature changelog while other tasks remain open.
- **Who / when**: a maintainer, before release.
