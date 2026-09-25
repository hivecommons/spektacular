---
tags: [workflow, fsm, step, template, addressing]
---

# Workflow Step Architecture

## Overview

A workflow is a linear FSM (finite state machine) driven by `looplab/fsm`. Each step is a
`workflow.StepConfig` that wires together an FSM event name, source/destination states, and an
optional callback. Steps are defined in domain packages (e.g. `internal/steps/spec`) and passed into
`workflow.New()`.

## Packages

| Package | Responsibility |
|---|---|
| `internal/workflow` | FSM, state persistence, `Data` interface, `Config` |
| `internal/steps/spec` | Spec-specific steps, templates, result types (likewise `internal/steps/plan`, `internal/steps/implement`, `internal/steps/repo`) |
| `internal/stepkit` | Shared step plumbing — template rendering, path strategies, result assembly |
| `cmd` | Thin command handlers — parse input, build the store, create workflow, call `Next`/`Goto`, write output |

## Adding a Step

### 1. Add the step to `spec.Steps()`

```go
// internal/steps/spec/steps.go
func Steps() []workflow.StepConfig {
    return []workflow.StepConfig{
        // ...existing steps...
        {Name: "my_step", Src: []string{"previous_step"}, Dst: "my_step", Callback: myStep()},
    }
}
```

The FSM uses `Src` to guard transitions — calling `goto my_step` from any state not in `Src`
returns an error. `Dst` is the state the FSM moves into after the step fires.

### 2. Write the callback

```go
func myStep() workflow.StepCallback {
    return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
        return "", writeStep("my_step", "next_step", "steps/spec/NN-my_step.md", data, out, st, cfg, nil)
    }
}
```

The callback receives:
- `data workflow.Data` — read/write key-value store persisted in state. Read spec context here
  (`name`, etc.). Write anything the step needs to record for later steps.
- `out workflow.ResultWriter` — write the result JSON the agent receives.
- `st store.Store` — the project store, rooted at the project root. This is how a step reads and
  writes files; see `architecture/working-with-files-from-steps.md`. It is `nil` for workflows
  constructed without one, so guard before using it.
- `cfg workflow.Config` — runtime config (`Command`, `Kind`, `DryRun`, `SpecDir`, `PlanDir`,
  `ChangelogDir`). Not persisted.

It returns `(nextStep, error)`. A non-empty `nextStep` is a **deferred transition**: the engine
records it and, once the current FSM event has finished, `Next`/`Goto` advances to that step
(recursing if that step names one in turn). Return `""` to stay where the transition table put you —
which is what most steps do, because the ordinary forward move is already encoded in `Src`/`Dst`.

**Rules:**
- The callback must not access workflow internals (current step, completed steps, totals).
- File writes belong in the callback, done through `st` — never through `os` directly, and never by
  stuffing file content into the `Data` store, which is for small values the next step needs.
- If `cfg.DryRun` is true, skip all side effects.

### 3. Add the template

Create `templates/steps/spec/NN-my_step.md`, where `NN` is the step's position in the workflow
order (e.g. `01-overview.md`). The template receives the bundle constructed in
`stepkit.WriteStepResult` plus `config`:

```
{{step}}          — step name (snake_case)
{{title}}         — step name formatted as title case
{{next_step}}     — the step this one advances to
{{spec_name}}     — the spec's name (its address; plan and implement steps get {{plan_name}})
{{config.command}} — the CLI binary name (e.g. "spektacular")
```

Templates should instruct the agent what to do, where to read/write, and what to call next. A template reaches a store document only through a CLI read by address (`{{config.command}} spec file read {{spec_name}}`, `{{config.command}} plan file read {{plan_name}} plan`), never through a file path. The store may not be on disk. See `architecture/working-with-files-from-steps.md`.

### 4. Add a template for the spec scaffold if needed

The spec scaffold (`templates/scaffold/spec.md`) defines the initial Markdown structure of a new
spec file. Add a new section heading if the step writes to a new section.

## Data Store

State is split into two concerns:

**Core** (managed by workflow, not accessible to callbacks):
- `current_step`, `completed_steps`, timestamps

**Data** (`map[string]any`, persisted alongside core, accessible to callbacks):
- Callbacks read and write arbitrary keys here.
- Seeded by the command handler before the workflow starts (e.g. `name`).
- Mutations from callbacks are persisted on the next state transition.

## Callback Signature

```go
type StepCallback func(data Data, out ResultWriter, st store.Store, cfg Config) (string, error)
```

This is the only interface between the workflow engine and domain logic. Keep it narrow.

## Config

`workflow.Config` carries runtime-only values that are not persisted:

```go
type Config struct {
    Command      string // CLI binary name, injected into templates as {{config.command}}
    Kind         string // "spec"/"plan"/"implement" — stamped onto freshly-created State by New()
    DryRun       bool   // skip all side effects when true
    SpecDir      string // store-relative directories the spec, plan and changelog
    PlanDir      string // workflows write into, sourced from the project
    ChangelogDir string // configuration
}
```

Set at workflow construction time by the command handler. Never stored in state.

## Flow: `spec new`

```
runSpecNew
  → store.NewSourceStore(root, "project")
  → workflow.New(spec.Steps(), statePath, wfCfg, st, out)
  → wf.SetData("name", ...)
  → wf.Next()        // fires "new" step: writes the spec scaffold file through the store,
                     //   renders its template and writes Result to the agent
```

## Flow: `spec goto`

```
runSpecGoto
  → workflow.New(spec.Steps(), statePath, wfCfg, st, out)  // loads persisted state
  → wf.Goto(stepName)                                      // FSM errors if transition invalid
```

`New` and `Goto` both take the output writer from construction — `out` is handed to `New`, not to
each call. The FSM guards against calling a step from the wrong state. The agent must always call
steps in order unless the workflow explicitly allows branching via multiple `Src` entries.
