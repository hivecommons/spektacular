---
created_date: "2026-09-25"
document_status: draft
spec: 000058_plan-task-graph
specs:
    - 000058_plan-task-graph
---

# Plan task graph

The worked design for spec `000058_plan-task-graph` (GitHub issue #50): the task format inside a
plan, the JSON the CLI emits for it, and how implement selects a single task.

## Tasks in plan.md

A plan's `## Milestones & Tasks` section groups tasks under milestones. Milestones keep their
current shape (title, "What changes", validation point) and are numbered. A task is a single unit
of work, in a single repo, for a single kind of executor.

```markdown
### Milestone 1: Plans can be exported

#### - [ ] Task: Add the `plan export` command
**Id:** 7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34
**Repo:** spektacular
**Depends on:**
- 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95 — Add a plan task reader
**Execution:** agent

Summary: two to four plain-language sentences.

*Technical detail:* [context.md#…](./context.md#…)

**Acceptance criteria**:
- [ ] Outcome statement
- [ ] Outcome statement
```

| Line | Rule |
|---|---|
| Heading | `#### - [ ] Task: <title>`. The checkbox is the task's completion; `[x]` means completed. |
| `**Id:**` | Required. Opaque string issued by the CLI (see Identifiers). Never rewritten once written. |
| `**Repo:**` | Required. Exactly one registered repo name. |
| `**Depends on:**` | Required. Either `none` on the same line, or a list with one `- <id> — <title>` entry per dependency. Only the id is parsed; the title is for readers and may drift without breaking anything. |
| `**Execution:**` | Required. `agent`, or `human — <reason>` with a non-empty reason. |
| Acceptance criteria | `- [ ]` / `- [x]` checkboxes, ticked by implement only when the criterion passed verification. |

A task with no dependencies is written:

```markdown
**Depends on:** none
```

### When a task is `human`

The planner marks a task `human` when completing it needs any of:

- secrets or access an agent will not have (production credentials, cloud consoles, signing keys);
- action outside the repo (deploying, releasing, DNS, purchasing or approving something);
- judgement that must be a person's (legal or licensing, design sign-off, a stakeholder decision);
- verification only a person can do (visual or UX review, physical hardware).

Anything else is `agent`. A task that would need both is split: the human part becomes its own
task depending on the agent task. The plan walkthrough names every human task and its reason.

### Validation on write

Writing a plan's `plan.md` is refused, naming the task, when any task:

- lacks `**Id:**`, `**Repo:**`, `**Depends on:**` or `**Execution:**`;
- names more than one repo, or a repo not in the registry;
- depends on an id that is not a task in the same plan;
- is part of a dependency cycle;
- has an execution type other than `agent` or `human`, or `human` with no reason;
- shares its id with another task.

## Identifiers

Task ids come from the CLI, never invented by the author:

```console
$ spektacular plan task-id
{ "id": "7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34" }
```

The issuing provider is pluggable, configured like a store:

```yaml
# .spektacular/config.yaml
plan:
  task_id:
    provider: uuid   # default
```

Consumers treat ids as opaque strings. `uuid` returns a random UUID. A provider that creates
something external when issuing an id must tolerate ids issued for tasks later discarded.

`depends_on` references ids, not local labels, so a dependency on a task in another spec's plan
is expressible later. For now every dependency must resolve inside the same plan.

## plan export

```console
$ spektacular plan export <name> [--format pretty|json]
```

`--format` defaults to `pretty`; `json` is the machine form Hive requests. Other formats (YAML,
…) may be added later. Errors are the structured JSON error envelope whatever format was
requested. The export parses `plan.md` at call time; nothing is stored alongside the plan. It
succeeds in any document status.

### pretty (default)

```console
$ spektacular plan export 000058_plan-task-graph
000058_plan-task-graph  (final)  1/3 tasks complete

Milestone 1
  [x] Add a plan task reader                      spektacular   agent
      0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95
  [ ] Add the `plan export` command               spektacular   agent
      7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34
      depends on: Add a plan task reader

Milestone 2
  [ ] Publish the release signing key             spektacular   human: needs access to the production key vault
      e4a8c3f1-2b6d-4f90-a7c5-91d0b3e6f428
      depends on: Add the `plan export` command
```

Tasks are grouped by milestone in plan order, each showing completion, title, repo name,
executor (with the reason for human tasks), id, and dependencies by title.

### json

```console
$ spektacular plan export 000058_plan-task-graph --format json
```

```json
{
  "kind": "plan",
  "name": "000058_plan-task-graph",
  "document_status": "final",
  "tasks": [
    {
      "id": "0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95",
      "title": "Add a plan task reader",
      "milestone": 1,
      "repo": { "name": "spektacular", "location": "https://github.com/hivecommons/spektacular" },
      "depends_on": [],
      "execution": { "type": "agent", "reason": "" },
      "completed": true
    },
    {
      "id": "7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34",
      "title": "Add the `plan export` command",
      "milestone": 1,
      "repo": { "name": "spektacular", "location": "https://github.com/hivecommons/spektacular" },
      "depends_on": ["0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95"],
      "execution": { "type": "agent", "reason": "" },
      "completed": false
    },
    {
      "id": "e4a8c3f1-2b6d-4f90-a7c5-91d0b3e6f428",
      "title": "Publish the release signing key",
      "milestone": 2,
      "repo": { "name": "spektacular", "location": "https://github.com/hivecommons/spektacular" },
      "depends_on": ["7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34"],
      "execution": { "type": "human", "reason": "needs access to the production key vault" },
      "completed": false
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `kind`, `name` | Always `plan` and the bare plan name requested. |
| `document_status` | The plan's lifecycle status; the consumer decides readiness. |
| `tasks` | Every task, in plan order. |
| `id` | The task's id. There is no `ref`; consumers that fall back from `ref` to `id` (Hive) get matching edges. |
| `milestone` | The number of the milestone the task sits under. |
| `repo.name` | The registry name, linking back to Spektacular's config. |
| `repo.location` | The repo's declared git source. Empty when none is declared — never read from `git remote`. |
| `depends_on` | Ids of the tasks this one depends on; `[]` for `none`. |
| `execution` | `type` is `agent` or `human`; `reason` is set for `human`. |
| `completed` | The task heading's checkbox. |

`repo` and `execution` are objects rather than the strings Hive's decoder currently expects;
Hive adapts to this shape (for `repo`, deriving `owner/repo` from `location`).

## plan status progress

`plan status <name>` gains per-task progress from the same parser:

```json
{
  "progress": { "tasks_completed": 1, "tasks_total": 3 },
  "tasks": [
    {
      "id": "0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95",
      "title": "Add a plan task reader",
      "milestone": 1,
      "completed": true,
      "acceptance_criteria": { "met": 2, "total": 3 }
    }
  ]
}
```

`completed` and `acceptance_criteria` are separate: implement can complete a task with an accepted
deviation, and that must stay visible. `plan status` answers "how far along"; `plan export`
answers "what is the work".

## Implementing one task

```console
$ spektacular implement new --data '{"name":"000058_plan-task-graph","task":"7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34"}'
```

Without `task`, implement runs the whole plan as before. With it:

- The run reads the whole plan, its context, research and referenced designs, but implements,
  tests, verifies and ticks only the selected task.
- Each run records its work in the changelog. Feature-level wrap-up — the test plan, the feature
  changelog and spec reconciliation — runs only in the run that completes the plan's last open
  task.
- `implement status` includes `"task": "<id>"`.

Refusals, all structured errors with a `next_action`; no workflow is started:

| Code | When | Carries |
|---|---|---|
| `task_not_found` | No task with that id in the plan | the id |
| `task_completed` | The task's checkbox is already ticked | the id |
| `task_dependencies_incomplete` | A dependency is not completed | the incomplete dependency ids |
| `task_requires_human` | Execution is `human` | the reason |

## Plans without task structure

Plans written before this format are not migrated. Whole-plan implement and milestone commits keep
working on them. `plan export` and task-scoped implement refuse them with a structural error that
says what is missing — for example `plan_structure_invalid`: "the plan contains no task ids" — not
an error about the plan's age or format version.

## Not covered

Parallel implement runs (issue #62), cross-spec dependency resolution, formats other than `pretty` and `json`,
and inferring repo location from git remotes.
