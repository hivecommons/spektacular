### Milestone 1: Plans carry a checked task structure

**What changes**: A plan can describe its work as tasks, each with an id issued by Spektacular, one repository, an explicit dependency list and an executor. Saving a plan that gets any of this wrong is refused, with an error naming the task and what is wrong, and the previously saved plan is left untouched. Authors ask Spektacular for task ids instead of inventing them, and a project can choose where those ids come from. Plans written before this feature still save, implement and make their milestone commits exactly as before.

**Validation point**: Saving a hand-written task-format plan succeeds. Saving each invalid variant is refused with the task named, and the stored plan is unchanged. `plan task-id` returns fresh UUIDs, and an unknown provider name is reported. Milestone auto-commits still fire for both a legacy plan and a task plan, and the full Go test suite passes.

### Milestone 2: Orchestrators can export a plan's task graph and read per-task progress

**What changes**: Anyone, person or tool, can export a named plan's tasks. The default output is a readable view grouped by milestone. With `--format json` it is the document Hive consumes, carrying each task's id, title, milestone, repository name and declared location, dependencies, executor and completion. `plan status` for a named plan also reports how many tasks are done and, for each task, whether it is complete and how many of its acceptance criteria were met. Plans without task structure are refused with an error that says what is missing.

**Validation point**: Exporting a task plan in both formats matches the design's shapes. Ticking a task shows up in the next export and in plan status. Draft and final plans both export, with the same document status plan status reports. Unsupported formats and legacy plans give structured errors, and the full Go test suite passes.

### Milestone 3: One task of a plan can be implemented on its own

**What changes**: A person or an orchestrator can ask Spektacular, or the agent through the implement skill, to implement one specific task of a plan. The run still reads the whole plan, its context, research and designs, but it builds, tests, verifies and ticks only that task. Tasks that cannot start are refused up front: an unknown task, one already done, one waiting on unfinished dependencies (they are listed), or one that needs a person (the reason is given). Each run records its work in the plan's changelog. The feature-level wrap-up (test plan, feature changelog, spec reconciliation) happens only in the run that completes the plan's last open task. Running implement without choosing a task works exactly as it does today.

**Validation point**: On a two-task fixture plan, the first single-task run ticks only its task and ends without wrap-up. The second run ticks the other task and performs the wrap-up. Each refusal case returns its structured error without starting a workflow. `implement status` shows the task id, a whole-plan run on a legacy plan still completes, and the full Go test suite passes.

### Milestone 4: The plan workflow authors tasks, and the feature is documented

**What changes**: New plans come out of the plan workflow in the task format. The planning agent mints ids, attributes each task to one repository, declares dependencies explicitly and decides each task's executor against stated criteria. It splits work that needs both an agent and a person, and during sign-off it names every task that needs a person along with the reason. Spektacular's vocabulary moves from "phase" to "task" everywhere a user reads it. The public documentation site explains the task format, the human-task criteria, `plan export` and its fields, and how to implement a single task.

**Validation point**: A plan authored through the workflow passes write validation and exports cleanly. The plan-workflow and implement-workflow harbor suites pass with updated oracles. The docs site builds and type-checks with the new page linked from the navigation, and the full Go test suite passes.
