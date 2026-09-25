---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# 000058_plan-task-graph: plan task graph, export and single-task implement

## What was built

Plans now describe their work as **tasks**. Each task sits under a milestone and carries four structured lines: an `**Id:**` issued by Spektacular, exactly one registered `**Repo:**`, an explicit `**Depends on:**` (`none` or `- <id> — <title>` lines), and an `**Execution:**` of `agent` or `human — <reason>`. A single reader (`internal/plantask`) parses a plan's `## Milestones & Tasks` section, and plans written before tasks (`Phase N.M` headings) still parse as their own format.

- **Checked on save.** `plan file write` refuses a task-format plan.md that breaks a structural rule: a missing line, a second or unregistered repo, an unknown or cyclic dependency, a duplicate id, a bad executor, or a human task without a reason. The error (`plan_task_invalid`) names the task, and the stored plan is left untouched. Plans without tasks save as before.
- **Task ids.** `plan task-id` issues ids from a pluggable provider (`plan.task_id.provider`, default `uuid`, random v4 UUIDs). An unknown provider fails only the id request.
- **Export.** `plan export <name> [--format pretty|json]` parses the plan at call time. The default is a readable view grouped by milestone; `json` is the task graph (`kind`, `name`, `document_status`, `tasks[]` with `id`, `title`, `milestone`, `repo {name, location}`, `depends_on`, `execution {type, reason}`, `completed`). `repo.location` comes only from a repo's declared git source, and `document_status` is computed exactly as `plan status` computes it.
- **Progress.** `plan status <name>` adds `progress {tasks_completed, tasks_total}` and per-task completion with acceptance criteria met/total, keeping every existing field.
- **Single-task implement.** `implement new --data '{"name":…,"task":"<id>"}'` refuses tasks that cannot start (`task_not_found`, `task_completed`, `task_dependencies_incomplete`, `task_requires_human`, or `plan_structure_invalid` for a plan without tasks) before any state is written. A task run reads the whole plan, context, research and designs, but implements, tests, verifies and ticks only its task. It finishes straight after its changelog entry while other tasks remain open, and the run that completes the last open task does the test plan, feature changelog and spec reconciliation. `implement status` and the resume report name the task. Whole-plan runs behave as before, and milestone commits work for both plan formats.
- **Authoring.** The plan workflow's `phases` step is now `tasks`. It mints ids with `plan task-id`, decides each executor against four criteria (secrets or access, action outside the repo, human judgement, human-only verification), and splits mixed work. The walkthrough names every human task before sign-off. Templates, skills, scaffolds, the glossary and the harbor oracles all use task vocabulary.
- **Docs.** A new "Plan tasks" page on the documentation site, with How it works and Configuration updated.

## Why it matters

Orchestrators such as Hive (issue #50) previously had to scrape plan prose and guess at ordering and ownership. They can now import a plan's work exactly as planned, see what is done, and hand each task back to Spektacular to build with the plan's full context. Plan authors get a checked way to say what depends on what and what needs a person.

## Deviations from the plan

- `autocommit.LeadsToCommit` now takes the step's rendered next step, so the new `update_changelog → finished` completion point applies only to single-task runs that finish early. In full mode an implement completion commit also records any milestone the task closed.
- `plan status`'s `--schema` uses a plan-specific schema, leaving spec status's schema unchanged.
- Two agent-behaviour acceptance criteria were not exercised by automated suites and are captured as manual procedures in the test plan: splitting mixed work on the release-workflow/signing-secret scenario, and asking an agent in plain words for one task.
- Harbor: plan-workflow passed 95/95 and implement-workflow 14/14 (legacy plan) after the Docker daemon was started for the rerun.
