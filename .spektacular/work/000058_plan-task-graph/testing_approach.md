Testing follows the project's three layers: Go unit and command tests, template-contract phrase tests, and harbor E2E suites. All Go tests must pass with test shuffling on, and each new `cmd` test goes through the shared root-command reset helpers.

**Unit tests: task reader and validator (most coverage).** This is the load-bearing component, so it gets table-driven tests with hand-written plan.md fixtures as independent oracles. The tests guarantee the following:

- Every field of a well-formed task is read, and `none` becomes an empty dependency list.
- Dependency titles are ignored for resolution.
- Only the Milestones & Tasks section is read.
- Legacy phase plans parse as `legacy`, with correct milestone counts.
- Acceptance-criteria met and total counts are exact.
- Each refusal rule is rejected with an error naming the task: a missing id, repo, dependency declaration or executor; two repos; an unregistered repo; an unknown dependency id; a two-task cycle and a longer cycle; a duplicate id; an unknown executor; and a human task with no reason.

**Unit tests: id providers.** The default provider returns a valid v4 UUID. Two hundred consecutive ids contain no duplicates. An unknown provider name returns an error naming it. Assertions check format and uniqueness only, never specific values.

**Command tests: `plan file write`, `plan task-id`, `plan export`, `plan status`.** These run in temp projects built with the existing config helpers. They guarantee:

- Each invalid structure is refused, and reading the plan afterwards returns the previously saved bytes unchanged.
- A valid task plan saves, as does a legacy phase plan.
- Ids survive an edit that reorders tasks, adds one and retitles another.
- Export JSON has exactly the design's fields, in plan order, with `depends_on: []` for `none`.
- `pretty` output is identical with and without `--format`. It shows milestones, titles, completion, repo, executor with reason, and dependencies by title.
- An unknown format gives a structured error naming `pretty` and `json`, and prints no tasks.
- `repo.location` is the git source when one is declared, and empty for a file or undeclared source even when the checkout has a git remote.
- Ticking a task makes the next export show it completed.
- Draft and final plans both export, with a document status equal to `plan status`.
- A missing plan gives a structured error.
- A legacy plan gives `plan_structure_invalid`, and the message says what is missing and never mentions age or version.
- For a four-task plan with two complete, one of them at 2/3 criteria, `plan status` reports 2 of 4 and 2/3 criteria for that task, and every pre-existing status field is still present.

**Command and step tests: implement.**

- `implement new` with each refusal case returns the right code, and `task_dependencies_incomplete` lists the dependency ids. No `state.json` is written.
- A valid task starts the run and stores `task`. `implement status` reports it.
- The `update_changelog` callback routes to `finished` when open tasks remain after a task run, and to `test_plan` when none remain or for a whole-plan run.
- `finished` accepts a missing feature changelog only for a non-final task run.
- Template-render tests show the selected task's id and title in the analyze, implement, test, verify and update_plan instructions, and show the whole-plan wording when there is no task.
- The existing FSM walk and step-order tests are extended with the new edge.

**Regression tests: old plans and milestone commits.** The milestone committer's tests run on both a task-format plan and the existing legacy fixtures, so milestone commits still fire for old plans. Whole-plan implement status still counts unchecked work on legacy plans. The harbor implement-workflow fixture stays in legacy format as the end-to-end old-plan check.

**Template-contract tests.**

- Phrase assertions are updated from phase to task wording.
- New assertions cover:
  - the tasks step naming `plan task-id`, the four human-task criteria and the split rule;
  - verification checking `**Id:**`, `**Repo:**`, `**Depends on:**` and `**Execution:**`;
  - the walkthrough naming every human task and its reason;
  - the implement skill documenting `task`.
- The hand-maintained step tables in the instruction-contract, auto-commit and cross-kind tests are updated to the `tasks` step.

**Harbor E2E.**

- The plan-workflow oracles (step order, skills per step, expected sections, solution script) move to the `tasks` step and task format.
- The plan-workflow and implement-workflow suites are each run once before the work is called done, because they do not run in CI.
- The agent-judgement acceptance criteria are checked here or manually. These are: "human tasks named in review", "mixed work is split" on the release-workflow and signing-secret reference scenario, "single-task run uses the plan's context", and "agent can be asked for one task".

**Docs.** The docs site builds and type-checks cleanly, and the layout-HTML guard finds no layout HTML in page bodies.

**Success metrics.**

- *Hive imports plans through `plan export` as its primary path within a month:* Manual — captured in the implementation test plan.
- *Every plan authored in the first month exports without error on its first attempt:* Behavioural test for the mechanism. A plan accepted by `plan file write` always exports successfully: a property test over the valid fixtures exports each one. Observing real adoption is Manual — captured in the implementation test plan.
- *Hive drives implementation through single-task runs, and progress from plan status matches the work done:* Behavioural test for the mechanism. After a single-task run's update_plan tick, `plan status` shows exactly that task completed. Observation in Hive is Manual — captured in the implementation test plan.
- *No orchestrator starts a task before its dependencies, or hands a human task to an agent, because of export output:* Behavioural test. Export always carries `depends_on` and `execution.type`/`reason` as authored, and `implement new` refuses a task with incomplete dependencies or a human executor. Field reports are Manual — captured in the implementation test plan.
