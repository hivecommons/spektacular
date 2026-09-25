---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
designs:
    - source: design
      path: plan-task-graph.md
---

# Feature: 000058_plan-task-graph

## Overview

Plans gain a strict, machine-readable task structure — every piece of work has a permanent identifier, one target repository, the work it depends on, and whether an agent or a person must carry it out — and Spektacular can export that task graph, report progress task by task, and implement one chosen task at a time. Today a plan's work can only be read as prose, so orchestrators such as Hive, which turn a finished plan into scheduled implementation work, have to scrape documents and guess at ordering and ownership. With this, an orchestrator can import a plan's work exactly as it was planned, see which pieces are done, and hand each piece back to Spektacular to build with the plan's full context — while plan authors gain an explicit, checked way to say what depends on what and what needs a human.

## Requirements

### Plan tasks

- [ ] **Plans are made of tasks**
  A plan's units of work are called tasks. Tasks are grouped under milestones, and each task is a single unit of work carried out in exactly one registered repository by one kind of executor.
- [ ] **Every task has a permanent identifier**
  Each task carries an identifier that is unique within the plan and opaque to the reader, and that stays the same when tasks are reordered, added, retitled or otherwise edited.
- [ ] **Identifiers are issued by Spektacular**
  Plan authors (agents or humans) obtain task identifiers from Spektacular on request rather than inventing them.
- [ ] **Identifier source is selectable**
  A project can choose where its task identifiers come from without changing the plan format or the export.
- [ ] **Every task states its dependencies**
  Each task declares which other tasks must be finished before it can start, by their identifiers, or explicitly declares that it depends on nothing. A task with no dependency declaration at all is invalid; there is no implied default ordering.
- [ ] **Dependencies are readable in the plan**
  Wherever a plan lists a task's dependencies, a human reader can tell which task each identifier refers to without looking it up.
- [ ] **Every task states its executor**
  Each task declares whether an agent can carry it out or a person must, and a task that needs a person states why.
- [ ] **The planning process decides the executor deliberately**
  When a plan is authored, each task's executor is decided against stated criteria — needing access or secrets an agent does not have, acting outside the repository, requiring human judgement or sign-off, or verification only a person can do. A task that would need both an agent and a person is split into separate tasks, the person's task depending on the agent's.
- [ ] **The plan review surfaces human tasks**
  When a plan is walked through with the user for sign-off, every task that needs a person is named along with its reason.
- [ ] **Invalid task structure is refused when a plan is saved**
  Saving a plan is refused, with an error naming the offending task, when any task is missing its identifier, repository, dependency declaration or executor; names more than one repository or an unregistered one; depends on an identifier not in the plan; takes part in a dependency cycle; declares an unknown executor type; or needs a person without a reason.

### Export

- [ ] **Export a plan's task graph**
  Users and tools can export a named plan, carrying the plan's name and current document status and its tasks in plan order. By default the export is a human-readable view grouped by milestone; on request it is a single machine-readable document that also carries the plan's kind. Requesting an unsupported format is refused with an error naming the supported formats.
- [ ] **Exported tasks carry the full task picture**
  Each exported task carries its identifier, title, milestone, repository (its registered name and its location), dependencies as identifiers, whether an agent or a person carries it out and why, and whether it has been completed.
- [ ] **Repository location is never guessed**
  A task's exported repository location is the canonical source location declared for that repository in the project; when none is declared, the location is empty rather than inferred.
- [ ] **The export always reflects the current plan**
  Exporting a plan reflects its current content, including tasks completed and edits made since it was written.
- [ ] **Export works in any document status**
  A plan can be exported whether it is draft or final; consumers decide readiness from the document status in the output.

### Progress

- [ ] **Plan status reports progress per task**
  Querying a named plan's status reports, for every task, whether it is completed and how many of its acceptance criteria have been met out of how many, together with totals of completed and total tasks.
- [ ] **Completion and criteria are reported separately**
  A task's completion and its acceptance-criteria count are reported as separate facts, so a task marked complete although not all of its acceptance criteria were met is visible as such.

### Implementing a single task

- [ ] **Implement one selected task**
  Users and tools can start an implementation run for one task of a plan, identified by its identifier. Starting a run without selecting a task implements the whole plan as it does today.
- [ ] **Task runs use the whole plan's context**
  A single-task run has the plan's full context, research and referenced design documents available, while the work it performs, tests, verifies and marks complete is limited to the selected task.
- [ ] **Task runs refuse tasks that cannot start**
  Starting a run for a task is refused, with an actionable error, when the task does not exist in the plan, is already completed, has dependencies that are not yet completed (listing them), or needs a person (giving the reason).
- [ ] **Feature-level wrap-up happens once**
  Each single-task run records its work in the feature's changelog. Feature-level wrap-up — the feature test plan, the feature changelog summary and reconciling the spec — happens only in the run that completes the plan's last open task.
- [ ] **Implementation status names the task**
  While a single-task run is in progress, implementation status reports which task is being implemented.
- [ ] **Agents can be asked to implement one task**
  Users and orchestrators working through an agent can ask for one specific task of a plan to be implemented.

### Existing plans

- [ ] **Structural errors explain what is missing**
  Exporting, or running a single task from, a plan whose tasks lack the required structure fails with an error that describes what is missing (for example, that the plan contains no task identifiers) rather than an error about the plan's age or format version.

### Documentation

- [ ] **Public documentation covers the feature**
  The public documentation site describes the export command and its output, the plan task format (identifiers, dependencies, executor and the criteria for human tasks), and how to implement a single task.

## Constraints

- The export must be invocable as `spektacular plan export <name> --format json` — the invocation Hive (issue #50) builds against — which must produce the JSON document.
- When `--format` is absent the export must default to `pretty`, a human-readable view; `pretty` and `json` are the supported formats.
- Export errors must keep the structured JSON error shape whatever format was requested.
- The export must keep Hive's field names for the fields it already reads: top-level `kind`, `name` and `tasks`, and per-task `id`, `title` and `depends_on`.
- Each exported task's repository must be a `repo` object with `name` and `location` keys, and its executor an `execution` object with `type` and `reason` keys, where `type` is `agent` or `human`.
- Dependencies must reference task identifiers, not plan-local labels, so that dependencies across specs remain possible later.
- Dependencies must resolve to tasks within the same plan.
- Task identifiers must be issued by Spektacular through a pluggable provider, selectable like storage backends, defaulting to random UUIDs; once written into a plan they must never be rewritten by Spektacular.
- The export must be derived from the plan document at the time it is requested. No separately stored export of the task graph may exist alongside the plan.
- Plans written before this feature must not be rewritten or migrated, and whole-plan implementation and milestone commits must keep working on them.
- Existing `plan status` output fields must remain unchanged; per-task progress is added alongside them. Milestone commits and implement progress reporting must keep working for plans in the task format.
- Plans must be read and written through the plan store, so the feature works for any configured store backend.
- Errors must use the CLI's existing structured JSON error shape.
- The plan task format, the `plan export` and `plan status` JSON shapes, identifier issuing, and single-task implement refusals must follow the design document `design:plan-task-graph.md`, which settles them.

## Acceptance Criteria

### Plan tasks

- [ ] **Authored plans are made of tasks**
  A plan authored through the plan workflow presents its work as tasks grouped under milestones, and each task names exactly one registered repository.
- [ ] **Identifiers survive edits**
  After a plan is saved, edited (tasks reordered, a task added, titles changed) and saved again, every task that existed before keeps exactly the identifier it had.
- [ ] **Identifiers come from Spektacular**
  Requesting a task identifier from Spektacular returns a new value each time; with default settings each value is a valid random UUID, and two hundred consecutive requests return no duplicates.
- [ ] **Identifier source is configurable**
  A project whose configuration names an identifier provider that does not exist gets an error naming that provider when it requests an identifier, showing the setting is honoured; with the setting absent, identifiers are random UUIDs.
- [ ] **Dependencies are explicit**
  Saving a plan in which one task has no dependency declaration is refused; saving the same plan with that task declaring "none" succeeds, and its export shows an empty dependency list for it.
- [ ] **Dependencies are human-readable**
  In a saved plan, each dependency entry shows the title of the task it refers to alongside its identifier.
- [ ] **Executor is explicit**
  Saving a plan with a task that has no executor declaration, an executor other than agent or human, or a human executor with no reason is refused; a plan whose tasks declare `agent` or `human` with a reason saves successfully.
- [ ] **Human tasks are named in review**
  During the plan sign-off walkthrough of a plan containing a human task, the agent names that task and its reason before asking for sign-off.
- [ ] **Mixed work is split**
  Judged on a reference scenario — a feature that adds a release workflow to a repository and then needs a production signing secret created — the authored plan contains the credential step as a separate human task that depends on the agent task.
- [ ] **Invalid structure is refused on save**
  Saving a plan is refused, with an error naming the task concerned, for each of: a task with no identifier, a task with no repository, a task naming two repositories, a task naming an unregistered repository, a dependency on an identifier not in the plan, and two tasks that depend on each other. After each refusal, reading the plan returns the previously saved version unchanged.

### Export

- [ ] **Export produces the task graph**
  Exporting a saved plan by name exits successfully and prints one JSON document containing kind `plan`, the plan's name, its document status, and one task per plan task, in plan order.
- [ ] **Format option**
  Exporting with no format option and with `pretty` produce identical human-readable output showing, grouped by milestone, each task's title, completion, repository, executor (with the reason for human tasks) and dependencies by title; `--format json` produces the JSON document; any other value exits with a structured JSON error naming both supported formats and prints no tasks.
- [ ] **Task fields are complete**
  Each exported task has an identifier, title, milestone, repository name and location, dependency identifiers, executor type and reason, and a completed flag, all matching what the plan states.
- [ ] **Location only when declared**
  A task targeting a repository with a declared source location exports that location; a task targeting a repository with no declared source exports an empty location, even when the repository's checkout has a git remote configured.
- [ ] **Export is live**
  After a task is marked complete in a plan, the next export shows that task as completed without any other command being run.
- [ ] **Draft and final both export**
  Exporting a draft plan and a final plan both succeed, and each output's document status matches what plan status reports for it.
- [ ] **Errors are structured**
  Exporting a plan name that does not exist exits with a structured JSON error in the same shape as other CLI errors.

### Progress

- [ ] **Per-task progress in plan status**
  For a plan with four tasks of which two are complete, and one complete task has two of three acceptance criteria met, plan status for that plan reports two of four tasks completed, and reports that task as completed with two of three criteria met.

### Implementing a single task

- [ ] **Single-task run is scoped**
  Starting an implementation run for one task of a multi-task plan results, at the end of the run, in only that task and its acceptance criteria being marked complete in the plan; all other tasks are unchanged.
- [ ] **Whole-plan runs unchanged**
  Starting an implementation run without selecting a task behaves as before this feature, working through the whole plan.
- [ ] **Single-task run uses the plan's context**
  A single-task run for a task whose correct implementation depends on a fact stated only in the plan's research document or a referenced design document produces an implementation that follows that fact.
- [ ] **Unstartable tasks refused**
  Starting a single-task run is refused with a structured, actionable error for: an identifier not in the plan; a task already completed; a task with an incomplete dependency (the error lists that dependency); and a human task (the error includes its reason). No workflow is started in any of these cases.
- [ ] **Wrap-up only on the last task**
  In a two-task plan, the run that completes the first task adds an entry to the changelog but does not produce the feature test plan, the feature changelog summary or the spec reconciliation; the run that completes the second task produces the feature test plan, the feature changelog summary and the spec reconciliation.
- [ ] **Status names the task**
  While a single-task run is in progress, implementation status output includes the selected task's identifier.
- [ ] **Agent can be asked for one task**
  Asking the agent to implement a specific task of a plan starts a single-task run for that task.

### Existing plans

- [ ] **Old plans still implement**
  A plan written before this feature can still be implemented as a whole, and its milestone commits are still made, without editing the plan.
- [ ] **Old plans fail clearly for task features**
  Exporting, or starting a single-task run against, a plan written before this feature fails with a structured error stating what the plan is missing (such as task identifiers), and the error does not refer to the plan's age or format version.

### Documentation

- [ ] **Documentation published**
  The public documentation site has pages describing the export command and every output field, the plan task format including the criteria for human tasks, and how to implement a single task.

## Technical Approach

- Parse the plan document with one reusable reader of milestones and tasks, shared by the export, plan status progress, single-task implementation and the existing milestone-commit and implement-progress features, rather than adding another ad-hoc pattern match alongside the ones that exist today.
- Because plans are validated when they are written, the export can assume a saved plan's task structure is valid and focus on reporting it.
- Hold the task identifier in the plan document next to the task's other structured lines, and show beside each dependency the title of the task it depends on; the title is for readers and is not part of the reference.
- Consider how identifier providers that create something external when issuing an identifier handle identifiers issued for tasks that are later discarded.
- The work is carried by the plan and implement workflow guidance and the implement skill as much as by the CLI; expect those to change alongside the format.

## Success Metrics

- Within the first month after release, Hive imports plans through `plan export` as its primary path and no longer relies on its `tasks.json` / `plan.md` fallback for plans written after release.
- Across plans authored in the first month after release, every plan exports without error on its first attempt, because structural problems are caught when the plan is saved.
- Within the first month after release, Hive drives implementation of imported plans through single-task `spektacular implement` runs, and the task progress it reads from plan status matches the work actually done, with no manual progress updates.
- Across plans authored in the first month after release, no reported cases of an orchestrator starting a task before its dependencies were complete, or handing a human task to an agent, because of export output.

## Non-Goals

- Running independent tasks concurrently — implementation runs stay one at a time per project (tracked in #62).
- Resolving dependencies on tasks in other specs' plans.
- A way for external runners to mark tasks complete outside Spektacular's implement workflow; orchestrators implement through Spektacular instead.
- Changes to Hive itself.
- The artifact addressing and status overhaul in #46, beyond the per-task progress this feature adds to plan status.
- Export formats beyond `pretty` and `json` (such as YAML); these may be added later.
- Exporting specs or implementation runs, and any work breakdown finer than a task.
