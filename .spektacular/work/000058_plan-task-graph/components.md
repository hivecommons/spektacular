**New components (spektacular)**

- **Plan task reader.** The single reader of a plan's `## Milestones & Tasks` section. It turns plan.md text into an ordered list of milestones and tasks: id, title, milestone number, repo, dependency ids, executor, completion, and acceptance-criteria counts. It reports whether the plan uses the task format or the legacy phase format. It owns structural validation too: missing lines, several or unregistered repos, unknown or duplicate ids, cycles, bad executors, and human tasks with no reason. Its errors name the offending task. Every other component below reads plans through it.
- **Task identifier providers.** A small registry of id providers keyed by name. The default `uuid` provider issues random v4 UUIDs. It lives alongside the existing spec-identifier rules, resolves the configured provider when an id is requested, and refuses an unknown name with an error that lists the available providers.
- **`plan task-id` command.** A thin command that loads the config, asks the provider registry for one id and prints `{ "id": ... }`. Plan authors, both the plan workflow agent and humans, use it while writing tasks.
- **`plan export` command and renderers.** Reads a named plan through the plan store, parses it with the task reader, and resolves each task's repo name to `{name, location}` from the repo registry's declared git source. It takes document status from the same place `plan status` does. It renders either the `pretty` grouped text view or the `json` document. Unsupported formats, missing plans and legacy plans are refused with structured JSON errors.

**Changed components (spektacular)**

- **Store file write handler (`plan file write`).** Gains an optional per-store validator. For plan.md, the validator hands the body and the registered repo names to the task reader and refuses the write before anything is stored. Spec and changelog writes are unaffected.
- **Plan status (named).** The plan-specific extension of the shared artifact status adds `progress` (tasks completed out of total) and a `tasks` list (id, title, milestone, completed, criteria met/total) for task-format plans. Existing fields are unchanged.
- **Project config.** Gains `plan.task_id.provider`, defaulting to `uuid`, with no schema change.
- **Milestone auto-commit parser.** Rebuilt on the task reader so milestone completion is detected for both task-format and legacy phase plans.
- **Implement command.**
  - `implement new` accepts `task` and runs the four task pre-checks before any state is created. It records the selected task in workflow data.
  - `implement status` reports `task`. It also keeps counting unchecked work items in its existing field, which now covers tasks as well as phases.
  - The resume report carries the task.
- **Implement workflow steps.**
  - Callbacks hand the selected task to every step's template.
  - A new decision in `update_changelog` works out whether the run completed the plan's last open task and routes to feature wrap-up or directly to `finished`.
  - `finished` tolerates the absence of a feature changelog for a non-final task run.
  - The auto-commit points gain the new `update_changelog → finished` edge.
- **Implement step templates and implement skill.**
  - Templates speak in tasks, with a legacy-phase fallback so old plans still implement.
  - A task-scoped section limits implement, test, verify and update_plan to the selected task, and read_plan still reads the whole plan, context, research and designs.
  - The `spek-implement` skill documents asking for one task.
  - The shared helper skills (update-changelog, verify-implementation, follow-test-patterns, spawn-implementation-agents) move to task wording.
- **Plan workflow steps, scaffolds and plan skill.**
  - The `phases` step becomes `tasks`. It teaches the task format, when to mint ids, the executor criteria and the rule for splitting mixed work.
  - Assemble and verification check the new required lines.
  - The walkthrough names every human task and its reason.
  - The plan and context scaffolds use `## Milestones & Tasks` and `## Per-Task Technical Notes`.
- **AGENTS.md managed block (store access).** The "ticking a phase checkbox" wording becomes "ticking a task checkbox".
- **Knowledge glossary.** The `phase` term is replaced by a `task` term that defines it as a unit of work under a milestone.
- **Harbor E2E oracles.** The plan-workflow step order, skills-per-step map, expected sections and solution script move to `tasks`. The implement-workflow fixture stays a legacy phase plan, and so doubles as the old-plan regression check.

**Changed components (docs)**

- **Plan task documentation page.** A new page covering the task format, human-task criteria, `plan task-id`, `plan export` (both formats, every field) and single-task implement. It is linked from the site navigation.
- **How it works and Configuration pages.**
  - "phase" becomes "task" in the concepts and pipeline sections.
  - The `plan` config key documents `plan.task_id.provider`.
