The wire shapes (plan.md task block, `plan export` JSON, `plan status` progress, the `task-id` output and the implement refusal codes) are fixed by `design:plan-task-graph.md`. The contracts below are the internal ones that carry those shapes between components.

**Parsed plan (task reader output).** This is what every consumer receives. `Format` lets callers tell a task plan from a legacy phase plan without looking at the text again.

```go
type Plan struct {
    Format     Format      // FormatTasks | FormatLegacy | FormatNone
    Milestones []Milestone // in plan order
    Tasks      []Task      // in plan order, across milestones
}

type Milestone struct {
    Number int
    Title  string
    Items  int // tasks (or legacy phases) under it
    Open   int // unchecked items
}

type Task struct {
    ID         string
    Title      string
    Milestone  int
    Repo       string   // registry name, exactly one
    DependsOn  []string // ids; empty for "none"
    Execution  Execution
    Completed  bool     // heading checkbox
    Criteria   Criteria
}

type Execution struct{ Type, Reason string } // Type: "agent" | "human"
type Criteria  struct{ Met, Total int }
```

**Reader and validator interface.** `Parse` never fails on structure. It records what it saw, so a legacy plan can still yield milestone counts. `Validate` applies the design's refusal rules to a task-format plan and returns the first structured error, which names the task by title and id. It needs the registered repo names so it can check `**Repo:**`.

```go
func Parse(markdown []byte) Plan
func Validate(p Plan, registeredRepos []string) error // *output.ErrorResponse
func (p Plan) Task(id string) (Task, bool)
func (p Plan) OpenTasks() []Task
func (p Plan) RequireTasks() error // plan_structure_invalid when Format != FormatTasks
```

**Store write validator hook.** The shared store-file command builder takes an optional validator. Only the plan store sets it, and it acts only on paths ending `plan.md`.

```go
type writeValidator func(cfg config.Config, docPath string, body []byte) error
```

**Task id provider.** The provider registry is keyed by config name. The config key is `plan.task_id.provider`, and its default is `uuid`.

```go
type TaskIDProvider interface{ NewID() (string, error) }
func TaskIDProviderFor(name string) (TaskIDProvider, error) // unknown → task_id_provider_unknown
```

```go
type PlanConfig struct {
    // ...existing fields...
    TaskID TaskIDConfig `yaml:"task_id,omitempty"`
}
type TaskIDConfig struct{ Provider string `yaml:"provider"` }
```

**Export document.** This is the Go shape behind the design's JSON. Field names and nesting are exactly the design's.

```go
type Export struct {
    Kind           string       `json:"kind"`            // "plan"
    Name           string       `json:"name"`
    DocumentStatus string       `json:"document_status"`
    Tasks          []ExportTask `json:"tasks"`
}
type ExportTask struct {
    ID        string          `json:"id"`
    Title     string          `json:"title"`
    Milestone int             `json:"milestone"`
    Repo      ExportRepo      `json:"repo"`       // {name, location}
    DependsOn []string        `json:"depends_on"` // never null
    Execution Execution       `json:"execution"`  // {type, reason}
    Completed bool            `json:"completed"`
}
```

**Plan status additions.** These fields are added to the named `plan status` result, `omitempty`, and appear only for task-format plans.

```go
Progress *TaskProgress `json:"progress,omitempty"` // {tasks_completed, tasks_total}
Tasks    []TaskStatus  `json:"tasks,omitempty"`    // {id, title, milestone, completed, acceptance_criteria{met,total}}
```

**Implement input, state and status.**

- The `implement new --data` input gains an optional `task` (string).
- The workflow data gains `task`, which persists in `state.json`.
- `implement status` gains `"task": "<id>"`, set only during a single-task run.
- The step templates receive an `Extra` value `task: {id, title}` when a task is selected. `update_changelog` also receives `last_task: bool`.

**Refusal error codes.** The new codes are:

- `plan_task_invalid`: write validation.
- `plan_structure_invalid`: a legacy or task-less plan used by export or task-implement.
- `export_format_unsupported`.
- `task_id_provider_unknown`.
- `task_not_found`, `task_completed`, `task_dependencies_incomplete` and `task_requires_human`: the implement pre-checks.

They all use the existing `ErrorResponse` envelope with `resource` and `next_action`.
