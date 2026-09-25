#### - [ ] Phase 1.1: Add the plan task reader and validator
**Repo:** spektacular

Add one reusable reader that turns a plan's Milestones & Tasks section into milestones and tasks, recognising legacy phase plans as a separate format. Add the structural rules from the design as a validator that names the offending task. Every later phase reads plans through this reader instead of its own pattern match.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-add-the-plan-task-reader-and-validator)

**Acceptance criteria**:
- [ ] A well-formed task plan is read with every task's id, title, milestone, repo, dependency ids, executor, completion and acceptance-criteria counts; `none` yields no dependencies and dependency titles never affect resolution.
- [ ] A legacy phase plan is recognised as legacy and still reports correct per-milestone completion.
- [ ] Checkbox lines outside the Milestones & Tasks section are never counted as tasks or criteria.
- [ ] Each rule in the design (missing id, repo, dependency declaration or executor; two repos; unregistered repo; unknown dependency; cycle; duplicate id; unknown executor; human without reason) is refused with an error naming the task and the rule.

#### - [ ] Phase 1.2: Refuse invalid task structure when a plan is saved
**Repo:** spektacular

Hook the validator into `plan file write` for plan.md so a task-format plan with any structural error is refused before anything is stored. Legacy phase plans and the plan's other documents save exactly as they do today.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-refuse-invalid-task-structure-when-a-plan-is-saved)

**Acceptance criteria**:
- [ ] Saving a task plan with any invalid structure is refused with a structured error naming the task, and reading the plan afterwards returns the previously saved version unchanged.
- [ ] An unregistered repo refusal lists the registered repo names in its next action.
- [ ] A valid task plan, a legacy phase plan, and context, research and test-plan documents all save successfully.
- [ ] After a task plan is saved, edited (tasks reordered, a task added, titles changed) and saved again, every existing task keeps its identifier.

#### - [ ] Phase 1.3: Issue task identifiers from a pluggable provider
**Repo:** spektacular

Add `plan task-id`, which returns a fresh identifier from the provider named by `plan.task_id.provider`, defaulting to random UUIDs. An unknown provider is reported only when an id is requested, so the rest of the CLI keeps working and existing config files need no migration.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-issue-task-identifiers-from-a-pluggable-provider)

**Acceptance criteria**:
- [ ] Each request returns a new valid random UUID by default, and two hundred consecutive requests contain no duplicates.
- [ ] A config naming a provider that does not exist gets a structured error naming that provider and listing the available ones when an id is requested.
- [ ] Existing config files without the new key load unchanged and use the UUID provider.

#### - [ ] Phase 1.4: Move milestone commits and unchecked-work counts onto the task reader
**Repo:** spektacular

Rebuild milestone completion detection and the unchecked-work count in implement status on the shared reader, so both work for task plans and continue to work for legacy phase plans.

*Technical detail:* [context.md#phase-14](./context.md#phase-14-move-milestone-commits-and-unchecked-work-counts-onto-the-task-reader)

**Acceptance criteria**:
- [ ] A milestone whose tasks are all ticked triggers its milestone commit in a task plan, exactly as a fully ticked milestone of phases does in a legacy plan.
- [ ] Implement status reports the number of unchecked work items for both task and legacy plans, with its existing field name unchanged.
- [ ] All existing milestone-commit behaviour on legacy plans is unchanged.

#### - [ ] Phase 2.1: Add `plan export` with pretty and JSON output
**Repo:** spektacular

Add `plan export <name> [--format pretty|json]`, which parses the plan at call time and prints the design's grouped text view by default or the JSON task graph on request. Repository location comes only from a declared git source, and document status matches what plan status reports.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-add-plan-export-with-pretty-and-json-output)

**Acceptance criteria**:
- [ ] `--format json` prints one document with kind `plan`, the plan's name, its document status, and every task in plan order with exactly the design's fields; a task declaring `none` has an empty dependency list.
- [ ] No format and `--format pretty` print identical text grouped by milestone, showing each task's completion, title, repo, executor (with reason for human tasks), id and dependencies by title.
- [ ] Any other format is refused with a structured error naming `pretty` and `json`, and no tasks are printed.
- [ ] A repo with a declared git source exports that location; a repo with a file or no declared source exports an empty location even when its checkout has a git remote.
- [ ] After a task is ticked, the next export shows it completed; draft and final plans both export with the document status plan status reports.
- [ ] A missing plan, and a plan without task structure, fail with structured errors; the latter says what is missing and never mentions the plan's age or format version.

#### - [ ] Phase 2.2: Report per-task progress in plan status
**Repo:** spektacular

Add task totals and a per-task list (completion plus acceptance criteria met out of total) to `plan status <name>` for task-format plans, alongside every existing field.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-report-per-task-progress-in-plan-status)

**Acceptance criteria**:
- [ ] For a four-task plan with two complete, one of which has two of three criteria met, plan status reports two of four tasks completed and that task as completed with two of three criteria met.
- [ ] A task marked complete with unmet criteria is visibly reported as such.
- [ ] Every field plan status reported before this change is still present with the same meaning, for task and legacy plans alike.

#### - [ ] Phase 3.1: Select a task when starting implement
**Repo:** spektacular

Let `implement new` take a task id, refuse tasks that cannot start before any workflow state is created, and record the chosen task so status and resume report it. Without a task, implement starts exactly as today.

*Technical detail:* [context.md#phase-31](./context.md#phase-31-select-a-task-when-starting-implement)

**Acceptance criteria**:
- [ ] Starting a run for an unknown task, a completed task, a task with an incomplete dependency (the error lists it), or a human task (the error gives its reason) is refused with a structured, actionable error and no workflow is started.
- [ ] Starting a run for a legacy plan with a task id fails with an error saying the plan has no task ids.
- [ ] A valid task starts a run; implement status and the resume report include the selected task's id.
- [ ] Starting a run without a task behaves as before this feature.

#### - [ ] Phase 3.2: Route feature wrap-up to the run that completes the last open task
**Repo:** spektacular

After a single-task run records its changelog entry, decide in code whether any tasks remain open: if so the run finishes directly, otherwise it continues into the test plan, feature changelog and spec reconciliation. Auto-commits and the finishing checks follow the new route.

*Technical detail:* [context.md#phase-32](./context.md#phase-32-route-feature-wrap-up-to-the-run-that-completes-the-last-open-task)

**Acceptance criteria**:
- [ ] In a two-task plan, the run completing the first task ends without producing a test plan, feature changelog or spec reconciliation.
- [ ] The run completing the second task produces the test plan, the feature changelog and the spec reconciliation.
- [ ] A whole-plan run follows exactly the route it does today.
- [ ] The work of a run that finishes early is committed under the configured auto-commit mode.

#### - [ ] Phase 3.3: Scope the implement instructions and skill to the selected task
**Repo:** spektacular

Rewrite the implement step instructions, shared partials and helper skills in task vocabulary, with a fallback for legacy phase plans, and make analyze, implement, test, verify and update-plan work only on the selected task during a single-task run. Document in the implement skill how to ask for one task.

*Technical detail:* [context.md#phase-33](./context.md#phase-33-scope-the-implement-instructions-and-skill-to-the-selected-task)

**Acceptance criteria**:
- [ ] During a single-task run, every implement instruction names the selected task and limits work, tests, verification and ticking to it, while read-plan still reads the full plan, context, research and referenced designs.
- [ ] At the end of a single-task run on a multi-task plan, only that task and its criteria are ticked.
- [ ] A whole-plan run on a legacy phase plan still completes with milestone commits made, without editing the plan.
- [ ] Asking the agent to implement a specific task of a plan starts a single-task run for it.

#### - [ ] Phase 4.1: Author plans as tasks in the plan workflow
**Repo:** spektacular

Turn the plan workflow's phases step into a tasks step that mints ids with `plan task-id`, attributes one repo per task, requires explicit dependencies, and decides the executor against the design's criteria, splitting mixed work. Update scaffolds, assembly, verification, the plan skill and the managed AGENTS.md wording, and make the walkthrough name every human task and its reason.

*Technical detail:* [context.md#phase-41](./context.md#phase-41-author-plans-as-tasks-in-the-plan-workflow)

**Acceptance criteria**:
- [ ] A plan authored through the workflow presents its work as tasks under milestones, each naming exactly one registered repo, and it saves and exports without error.
- [ ] Each task in an authored plan carries an id obtained from Spektacular, a dependency declaration showing titles beside ids, and an executor decided against the stated criteria.
- [ ] On the reference scenario (add a release workflow, then create a production signing secret), the authored plan has the credential step as a separate human task depending on the agent task.
- [ ] During sign-off of a plan containing a human task, the agent names that task and its reason before asking for sign-off.

#### - [ ] Phase 4.2: Update the glossary and end-to-end suites to tasks
**Repo:** spektacular

Replace the glossary's phase term with a task term and move the plan-workflow harbor oracles and solution to the tasks step and format, keeping the implement-workflow fixture as a legacy plan so it doubles as the old-plan regression check. Run both harbor suites.

*Technical detail:* [context.md#phase-42](./context.md#phase-42-update-the-glossary-and-end-to-end-suites-to-tasks)

**Acceptance criteria**:
- [ ] The knowledge glossary defines a task (a unit of work under a milestone, in one repo, for one executor) and no longer defines phase as the plan's unit of work.
- [ ] The plan-workflow harbor suite passes against the tasks step and task format.
- [ ] The implement-workflow harbor suite passes against its legacy phase plan.

#### - [ ] Phase 4.3: Document the plan task format, export and single-task implement
**Repo:** docs

Add a documentation page covering the task format, the human-task criteria, `plan task-id`, `plan export` and every output field, per-task progress in plan status, and implementing a single task, and link it from the navigation. Move "phase" wording to "task" on the existing pages and document the new config key.

*Technical detail:* [context.md#phase-43](./context.md#phase-43-document-the-plan-task-format-export-and-single-task-implement)

**Content outline** (new page "Plan tasks", wording illustrative; field names and commands fixed):

1. *Hero / intro*: "A plan's work is a set of tasks. Each task is one unit of work, in one repository, for one kind of executor, and Spektacular can export them, report progress on them and implement them one at a time."
2. *The task format*: a fenced markdown example exactly as in the design:
   ```markdown
   #### - [ ] Task: Add the `plan export` command
   **Id:** 7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34
   **Repo:** spektacular
   **Depends on:**
   - 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95 — Add a plan task reader
   **Execution:** agent
   ```
   followed by one `ConfigKey`-style entry per line (`Id`, `Repo`, `Depends on`, `Execution`, acceptance criteria) stating the rule, and "Saving a plan that breaks a rule is refused, naming the task."
3. *When a task needs a person*: the four criteria as a list (secrets or access, action outside the repo, human judgement, human-only verification), plus "a task needing both is split, and the person's task depends on the agent's".
4. *Task identifiers*: `spektacular plan task-id` returning `{ "id": "..." }`, and the `plan.task_id.provider` setting (default `uuid`).
5. *Exporting a plan*: `spektacular plan export <name>` pretty sample, then `--format json` sample, then a field table: `kind`, `name`, `document_status`, `tasks`, `id`, `title`, `milestone`, `repo.name`, `repo.location` ("the repo's declared git source; empty when none is declared"), `depends_on`, `execution.type`, `execution.reason`, `completed`.
6. *Tracking progress*: `spektacular plan status <name>` sample showing `progress.tasks_completed`, `progress.tasks_total`, and a `tasks[]` entry with `acceptance_criteria.met/total`, with the note that completion and criteria are separate facts.
7. *Implementing one task*: `spektacular implement new --data '{"name":"<plan>","task":"<id>"}'`, what the run reads and what it limits itself to, the wrap-up-on-last-task rule, and a table of refusals (`task_not_found`, `task_completed`, `task_dependencies_incomplete`, `task_requires_human`).
8. *Plans written before tasks*: "Older plans still implement as a whole; export and single-task runs explain what the plan is missing."

**Content example** (How it works, concepts): "A **task** is one unit of work in a plan: it sits under a milestone, targets one repository, and is carried out by an agent or by a person." It replaces the current phase definition, and the stage tree reads `... → milestones → tasks → ...`.

**Acceptance criteria**:
- [ ] The documentation site has a page describing the export command and every output field, the plan task format including the criteria for human tasks, and how to implement a single task, reachable from the navigation.
- [ ] The How it works page describes plans in terms of tasks rather than phases.
- [ ] The Configuration page documents `plan.task_id.provider` and its default.
- [ ] The site builds and type-checks cleanly, with no layout HTML in page bodies and no em dashes in new prose.
