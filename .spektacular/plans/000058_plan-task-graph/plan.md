---
created_date: "2026-09-25"
document_status: final
closed_date: "2026-09-25"
---

# Plan: 000058_plan-task-graph

<!-- Metadata -->
<!-- Created: 2026-09-25T10:20:01Z -->
<!-- Commit: 93ebe26 -->
<!-- Branch: f-plan-export -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Plans gain a strict, machine-readable task structure. Every unit of work gets a permanent Spektacular-issued id, one repository, explicit dependencies and an agent-or-human executor. Spektacular can then export that task graph (`plan export`), report per-task progress in `plan status`, and implement one chosen task at a time. Orchestrators such as Hive (issue #50) currently scrape prose and guess ordering and ownership. This lets them import a plan's work exactly as planned and hand each task back to Spektacular to build with the plan's full context, while plan authors gain a checked way to say what depends on what and what needs a person.

## Conventions

- **Error messages must describe the problem and suggest remediation** — every new refusal (plan write validation, `plan_structure_invalid`, `task_*` implement refusals, unknown export format, unknown task-id provider) is an `output.NewError(...).WithNextAction(...)` with a runnable next step.
- **Spektacular's own files are written through Spektacular** — export, status and implement pre-checks read plan.md through the plan store; templates keep using `plan file read/write --from`; no template may introduce stdin/heredoc writes (`instruction_surface_test.go`).
- **Tests must not depend on execution order** — new `cmd` tests for `plan export`, `plan task-id`, and `implement new --task` go through `runRootCmd`/`resetRootCmd`; uuid tests assert format/uniqueness, never specific values.
- **Passing tests are required before calling work done** — the phase→task rename breaks many existing phrase and fixture tests; each phase finishes with the full Go test suite green.
- **Phase / Stage / Step / Workflow glossary** — "phase" is renamed "task" in the plan format; the `phase.md` glossary entry is replaced with a `task.md` entry through `knowledge` so the vocabulary stays binding and consistent (a task sits below a milestone, never a synonym for a step).
- **No em dashes (docs)** — all new docs prose avoids `—`; note the plan.md format itself uses `—` as a separator in `**Depends on:**` and `**Execution:**` lines, which is a data format quoted in code blocks, not prose.
- **MDX authoring conventions (docs)** — new/changed pages use named-slot components, fenced code blocks, no layout HTML in page bodies.
- **Site layout / alternate section background / label before filename (docs)** — the new page is composed from existing section components with alternating `surface`, and is registered in `Nav.astro`.
- **Plans must sketch content structure (docs)** — the documentation task carries a **Content outline** with headings and illustrative examples using the verified field names.

Dropped: none of the loaded conventions were judged irrelevant except that docs layout conventions apply only to the docs task.

## Architecture & Design Decisions

The feature is built on the settled design `design:plan-task-graph.md`: the task format in `## Milestones & Tasks`, the `plan task-id` command with a pluggable provider, `plan export` in `pretty` and `json` form, per-task progress in `plan status <name>`, and single-task `implement new` with the four refusal codes. The architecture decides how to carry that shape through the code. It has four parts:

- **One task reader (spektacular).** A new package, `internal/plantask`, parses plan.md into milestones and tasks. It reads each task's heading checkbox, title, id, repo, dependencies, executor and acceptance-criteria checkboxes. It also recognises the legacy `Phase N.M` headings, reporting them as a `legacy` format that carries only milestone and completion data.
- **Consumers of the reader.** Every consumer goes through this one reader instead of its own regex: write validation, `plan export`, `plan status` progress, the implement pre-checks and last-task decision, `implement status`, and milestone auto-commits (`internal/autocommit/milestones.go`).
- **Validation on write.** It runs inside the shared store-file `write` handler, after the body is stripped and before the store write (`cmd/storefile.go:228-233`). A refusal therefore leaves the previous plan.md byte-identical. The validator is given the registered repo names so its errors can name the offending task and list the valid repos. That follows the rule that validation lives beside the facts its remediation needs (`gotchas/remediation-needs-the-layer-that-holds-the-facts.md`).
- **Legacy plans pass validation untouched.** A plan with no task headings is treated as legacy and written unvalidated, so implement's `update_plan` keeps working on old plans. Export and single-task implement refuse such a plan with `plan_structure_invalid`, and the message says what is missing.

Task ids are minted in `internal/identifier`, the package that already owns every id rule, through a small provider registry keyed by `plan.task_id.provider`. The default is `uuid`: a v4 UUID built from `crypto/rand`, so no new dependency is needed. The provider is resolved when `plan task-id` is called, not when the config loads. An unknown name fails only that command, with an error naming the provider and listing the available ones. The rest of the CLI keeps working, and the optional key needs no schema bump or migration (`internal/config/schema.go:15-21`).

`plan export` reads the plan through the plan store and parses it. It then resolves each task's `repo.location` from the repo's declared source: the location when the provider is `git`, and empty otherwise. It never uses `git remote`. The export takes `document_status` from the same computation `plan status <name>` uses, including the strict-staleness hook, so the two always agree. `json` is written through the existing output writer. `pretty` is the CLI's first text renderer, and errors keep the JSON envelope whatever format was requested. `plan status <name>` gains `progress` and `tasks` through a plan-only extension of `runArtifactStatus`, leaving its existing fields unchanged.

Single-task implement works within the existing implement FSM rather than adding a second step list:

- **Starting a run.** `implement new` accepts `task`. The pre-checks (`task_not_found`, `task_completed`, `task_dependencies_incomplete`, `task_requires_human`) run after the plan-exists and stale-plan checks and before the start gate and state reset, so a refusal starts nothing. The id is stored in workflow data as `task`.
- **Scoping each step.** Callbacks pass the selected task (id and title) to templates through `Extra`, and mustache `{{#task}}` sections limit analyze, implement, test, verify and update_plan to that one task. read_plan still reads the whole plan, its context, its research and any referenced designs.
- **Deciding the wrap-up.** A Go callback on `update_changelog` reads plan.md and decides whether the run completed the plan's last open task. A run that did goes to `test_plan → update_feature_changelog → reconcile_spec → finished`. A run that did not goes straight to `finished` through a new `update_changelog → finished` edge, with its own auto-commit point. `finished()` skips the feature-changelog requirement for such a run.

The plan workflow's `phases` step becomes a `tasks` step. That step decides each task's executor against the design's human-task criteria, splits mixed work, and mints ids with `plan task-id`. The walkthrough names every human task and its reason. The scaffolds, verification, skills, glossary and harbor oracles all move in the same change (`architecture/testing-architecture.md`). The docs site gains a page on the plan task format, export and single-task implement. See [research.md#alternatives-considered-and-rejected](./research.md#alternatives-considered-and-rejected) for the rejected options: a stored `plan.json`, CLI-rewritten local refs, load-time provider validation, a separate single-task FSM, and routing on template prose alone.

## Component Breakdown

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

## Data Structures & Interfaces

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

## Implementation Detail

**A plan-reading module boundary (new pattern).** Code that needs to understand a plan's work stops pattern-matching plan.md on its own. It asks the task reader instead. The reader works line by line with anchored heading patterns, the same style as the current milestone scanner. The project has no markdown library and this feature does not add one. Parsing is confined to the `## Milestones & Tasks` section, with the legacy `## Milestones & Phases` section recognised as well. Anything outside that section, including `- [ ]` lines in the Changelog or Testing Approach, never counts as a task. `Parse` is tolerant and records what it saw, while `Validate` is strict. That split lets the milestone committer and whole-plan implement keep working on old or partly structured plans, while writes of task-format plans are held to the full rule set.

**Validation at the store boundary.** The shared store-file command builder gains an optional validator. This is the first time that builder enforces content rules rather than just metadata. The plan store is the only one that opts in, and it validates only `plan.md`, so `context.md`, `research.md` and `test-plan.md` writes are unchanged. Because validation runs before the store write, a refused write has no side effects, and callers see a normal structured error.

**Pluggable id providers (following the store pattern).** Task ids follow the same "provider name in config → implementation" idea as stores. They are held in a small in-package registry so another provider can be added later without touching the plan format or the export. Following the existing identifier package, providers are plain functions behind a name, not a plugin system.

**First non-JSON output mode.** `plan export --format pretty` is the first command that prints human text on success. It is written as a pure renderer from the export document to text, so the JSON and pretty paths share every lookup and differ only in the final write. Errors never switch format: they always go through the existing JSON failure path.

**Single-task implement as data, not a new workflow.** The implement FSM gains one edge (`update_changelog → finished`) and one piece of persisted data (`task`). The scoping lives in two places:

- Step callbacks pass the selected task to templates.
- Templates use conditional mustache sections to swap "the first unchecked task" for "the selected task".

The last-task decision moves from template prose into Go, so it is unit-testable. The template still renders both exits, but it tells the agent which one to take. A developer reading the implement steps will see one workflow with one optional input, not two parallel step lists.

**Legacy compatibility is explicit.** Wherever templates or Go code name the work unit, they name "task" first and fall back to the legacy "Phase N.M" form only where an old plan must still run: implement templates, the milestone committer and the unchecked-work count in `implement status`. The fallback is described in one shared partial rather than repeated in every template.

**Vocabulary change across the prose surface.** Renaming phase → task touches many templates, skills, the plan step name, scaffolds, the glossary and the harbor oracles. It is done as a mechanical rename followed by the semantic additions (ids, dependencies, executor), so the rename's diff stays reviewable and phrase-assertion tests fail in one place at a time.

## Dependencies

- **Design documents this plan was built on**: `plan-task-graph.md` from the `design` design source. This is the settled task format, `plan task-id` output and provider config, `plan export` pretty and JSON shapes, `plan status` progress shape, and the single-task implement refusal codes that this plan implements.
- **Plan store and store-file command builder** (internal): the only path for reading and writing plan.md. It needs a change: an optional per-store content validator on `write`.
- **Artifact metadata** (internal): the source of a plan's `document_status`. Used as-is.
- **Artifact status and the strict-staleness hook** (internal): the named `plan status` output. It needs a change: a plan-only extension for per-task progress. The export reuses its document-status computation.
- **Repo registry and repo config source** (internal): the registered repo names, used for write validation, and each repo's declared source, used for export `repo.location`. Used as-is.
- **Identifier package** (internal): the home of id rules. It needs a change: it gains the task id provider registry.
- **Project config** (internal): it needs a change: `plan.task_id.provider` with a `uuid` default. No schema bump or migration.
- **Workflow engine and step kit** (internal): the FSM, deferred goto and mustache `Extra` values. Used as-is. The implement step table gains one edge.
- **Auto-commit milestone detection and commit points** (internal): it needs a change: it is rebuilt on the task reader, and gains a commit point for `update_changelog → finished`.
- **Output envelope** (internal): structured JSON results and errors. Used as-is, plus a plain text writer for `--format pretty`.
- **`crypto/rand`** (Go standard library): the randomness for v4 UUIDs. No new third-party module.
- **Harbor E2E suites** (test infrastructure): the plan-workflow oracles must change in the same change. Verification needs the `harbor` CLI, Docker and Claude credentials, and these runs do not happen in CI.
- **docs repo (spektacular-website)**: it hosts the public documentation for this feature. It must build and type-check cleanly.
- **Upstream specs and plans**: none must land first. Spec `000058_plan-task-graph` (GitHub issue #50) is the source. Parallel implement runs are deferred to issue #62. Hive's adoption of the `repo` and `execution` object shapes is a consumer change outside this plan, noted on #50.

## Testing Approach

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

## Milestones & Phases

### Milestone 1: Plans carry a checked task structure

**What changes**: A plan can describe its work as tasks, each with an id issued by Spektacular, one repository, an explicit dependency list and an executor. Saving a plan that gets any of this wrong is refused, with an error naming the task and what is wrong, and the previously saved plan is left untouched. Authors ask Spektacular for task ids instead of inventing them, and a project can choose where those ids come from. Plans written before this feature still save, implement and make their milestone commits exactly as before.

**Validation point**: Saving a hand-written task-format plan succeeds. Saving each invalid variant is refused with the task named, and the stored plan is unchanged. `plan task-id` returns fresh UUIDs, and an unknown provider name is reported. Milestone auto-commits still fire for both a legacy plan and a task plan, and the full Go test suite passes.

#### - [x] Phase 1.1: Add the plan task reader and validator
**Repo:** spektacular

Add one reusable reader that turns a plan's Milestones & Tasks section into milestones and tasks, recognising legacy phase plans as a separate format. Add the structural rules from the design as a validator that names the offending task. Every later phase reads plans through this reader instead of its own pattern match.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-add-the-plan-task-reader-and-validator)

**Acceptance criteria**:
- [x] A well-formed task plan is read with every task's id, title, milestone, repo, dependency ids, executor, completion and acceptance-criteria counts; `none` yields no dependencies and dependency titles never affect resolution.
- [x] A legacy phase plan is recognised as legacy and still reports correct per-milestone completion.
- [x] Checkbox lines outside the Milestones & Tasks section are never counted as tasks or criteria.
- [x] Each rule in the design (missing id, repo, dependency declaration or executor; two repos; unregistered repo; unknown dependency; cycle; duplicate id; unknown executor; human without reason) is refused with an error naming the task and the rule.

#### - [x] Phase 1.2: Refuse invalid task structure when a plan is saved
**Repo:** spektacular

Hook the validator into `plan file write` for plan.md so a task-format plan with any structural error is refused before anything is stored. Legacy phase plans and the plan's other documents save exactly as they do today.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-refuse-invalid-task-structure-when-a-plan-is-saved)

**Acceptance criteria**:
- [x] Saving a task plan with any invalid structure is refused with a structured error naming the task, and reading the plan afterwards returns the previously saved version unchanged.
- [x] An unregistered repo refusal lists the registered repo names in its next action.
- [x] A valid task plan, a legacy phase plan, and context, research and test-plan documents all save successfully.
- [x] After a task plan is saved, edited (tasks reordered, a task added, titles changed) and saved again, every existing task keeps its identifier.

#### - [x] Phase 1.3: Issue task identifiers from a pluggable provider
**Repo:** spektacular

Add `plan task-id`, which returns a fresh identifier from the provider named by `plan.task_id.provider`, defaulting to random UUIDs. An unknown provider is reported only when an id is requested, so the rest of the CLI keeps working and existing config files need no migration.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-issue-task-identifiers-from-a-pluggable-provider)

**Acceptance criteria**:
- [x] Each request returns a new valid random UUID by default, and two hundred consecutive requests contain no duplicates.
- [x] A config naming a provider that does not exist gets a structured error naming that provider and listing the available ones when an id is requested.
- [x] Existing config files without the new key load unchanged and use the UUID provider.

#### - [x] Phase 1.4: Move milestone commits and unchecked-work counts onto the task reader
**Repo:** spektacular

Rebuild milestone completion detection and the unchecked-work count in implement status on the shared reader, so both work for task plans and continue to work for legacy phase plans.

*Technical detail:* [context.md#phase-14](./context.md#phase-14-move-milestone-commits-and-unchecked-work-counts-onto-the-task-reader)

**Acceptance criteria**:
- [x] A milestone whose tasks are all ticked triggers its milestone commit in a task plan, exactly as a fully ticked milestone of phases does in a legacy plan.
- [x] Implement status reports the number of unchecked work items for both task and legacy plans, with its existing field name unchanged.
- [x] All existing milestone-commit behaviour on legacy plans is unchanged.

### Milestone 2: Orchestrators can export a plan's task graph and read per-task progress

**What changes**: Anyone, person or tool, can export a named plan's tasks. The default output is a readable view grouped by milestone. With `--format json` it is the document Hive consumes, carrying each task's id, title, milestone, repository name and declared location, dependencies, executor and completion. `plan status` for a named plan also reports how many tasks are done and, for each task, whether it is complete and how many of its acceptance criteria were met. Plans without task structure are refused with an error that says what is missing.

**Validation point**: Exporting a task plan in both formats matches the design's shapes. Ticking a task shows up in the next export and in plan status. Draft and final plans both export, with the same document status plan status reports. Unsupported formats and legacy plans give structured errors, and the full Go test suite passes.

#### - [x] Phase 2.1: Add `plan export` with pretty and JSON output
**Repo:** spektacular

Add `plan export <name> [--format pretty|json]`, which parses the plan at call time and prints the design's grouped text view by default or the JSON task graph on request. Repository location comes only from a declared git source, and document status matches what plan status reports.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-add-plan-export-with-pretty-and-json-output)

**Acceptance criteria**:
- [x] `--format json` prints one document with kind `plan`, the plan's name, its document status, and every task in plan order with exactly the design's fields; a task declaring `none` has an empty dependency list.
- [x] No format and `--format pretty` print identical text grouped by milestone, showing each task's completion, title, repo, executor (with reason for human tasks), id and dependencies by title.
- [x] Any other format is refused with a structured error naming `pretty` and `json`, and no tasks are printed.
- [x] A repo with a declared git source exports that location; a repo with a file or no declared source exports an empty location even when its checkout has a git remote.
- [x] After a task is ticked, the next export shows it completed; draft and final plans both export with the document status plan status reports.
- [x] A missing plan, and a plan without task structure, fail with structured errors; the latter says what is missing and never mentions the plan's age or format version.

#### - [x] Phase 2.2: Report per-task progress in plan status
**Repo:** spektacular

Add task totals and a per-task list (completion plus acceptance criteria met out of total) to `plan status <name>` for task-format plans, alongside every existing field.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-report-per-task-progress-in-plan-status)

**Acceptance criteria**:
- [x] For a four-task plan with two complete, one of which has two of three criteria met, plan status reports two of four tasks completed and that task as completed with two of three criteria met.
- [x] A task marked complete with unmet criteria is visibly reported as such.
- [x] Every field plan status reported before this change is still present with the same meaning, for task and legacy plans alike.

### Milestone 3: One task of a plan can be implemented on its own

**What changes**: A person or an orchestrator can ask Spektacular, or the agent through the implement skill, to implement one specific task of a plan. The run still reads the whole plan, its context, research and designs, but it builds, tests, verifies and ticks only that task. Tasks that cannot start are refused up front: an unknown task, one already done, one waiting on unfinished dependencies (they are listed), or one that needs a person (the reason is given). Each run records its work in the plan's changelog. The feature-level wrap-up (test plan, feature changelog, spec reconciliation) happens only in the run that completes the plan's last open task. Running implement without choosing a task works exactly as it does today.

**Validation point**: On a two-task fixture plan, the first single-task run ticks only its task and ends without wrap-up. The second run ticks the other task and performs the wrap-up. Each refusal case returns its structured error without starting a workflow. `implement status` shows the task id, a whole-plan run on a legacy plan still completes, and the full Go test suite passes.

#### - [x] Phase 3.1: Select a task when starting implement
**Repo:** spektacular

Let `implement new` take a task id, refuse tasks that cannot start before any workflow state is created, and record the chosen task so status and resume report it. Without a task, implement starts exactly as today.

*Technical detail:* [context.md#phase-31](./context.md#phase-31-select-a-task-when-starting-implement)

**Acceptance criteria**:
- [x] Starting a run for an unknown task, a completed task, a task with an incomplete dependency (the error lists it), or a human task (the error gives its reason) is refused with a structured, actionable error and no workflow is started.
- [x] Starting a run for a legacy plan with a task id fails with an error saying the plan has no task ids.
- [x] A valid task starts a run; implement status and the resume report include the selected task's id.
- [x] Starting a run without a task behaves as before this feature.

#### - [x] Phase 3.2: Route feature wrap-up to the run that completes the last open task
**Repo:** spektacular

After a single-task run records its changelog entry, decide in code whether any tasks remain open: if so the run finishes directly, otherwise it continues into the test plan, feature changelog and spec reconciliation. Auto-commits and the finishing checks follow the new route.

*Technical detail:* [context.md#phase-32](./context.md#phase-32-route-feature-wrap-up-to-the-run-that-completes-the-last-open-task)

**Acceptance criteria**:
- [x] In a two-task plan, the run completing the first task ends without producing a test plan, feature changelog or spec reconciliation.
- [x] The run completing the second task produces the test plan, the feature changelog and the spec reconciliation.
- [x] A whole-plan run follows exactly the route it does today.
- [x] The work of a run that finishes early is committed under the configured auto-commit mode.

#### - [x] Phase 3.3: Scope the implement instructions and skill to the selected task
**Repo:** spektacular

Rewrite the implement step instructions, shared partials and helper skills in task vocabulary, with a fallback for legacy phase plans, and make analyze, implement, test, verify and update-plan work only on the selected task during a single-task run. Document in the implement skill how to ask for one task.

*Technical detail:* [context.md#phase-33](./context.md#phase-33-scope-the-implement-instructions-and-skill-to-the-selected-task)

**Acceptance criteria**:
- [x] During a single-task run, every implement instruction names the selected task and limits work, tests, verification and ticking to it, while read-plan still reads the full plan, context, research and referenced designs.
- [x] At the end of a single-task run on a multi-task plan, only that task and its criteria are ticked.
- [x] A whole-plan run on a legacy phase plan still completes with milestone commits made, without editing the plan.
- [ ] Asking the agent to implement a specific task of a plan starts a single-task run for it.

### Milestone 4: The plan workflow authors tasks, and the feature is documented

**What changes**: New plans come out of the plan workflow in the task format. The planning agent mints ids, attributes each task to one repository, declares dependencies explicitly and decides each task's executor against stated criteria. It splits work that needs both an agent and a person, and during sign-off it names every task that needs a person along with the reason. Spektacular's vocabulary moves from "phase" to "task" everywhere a user reads it. The public documentation site explains the task format, the human-task criteria, `plan export` and its fields, and how to implement a single task.

**Validation point**: A plan authored through the workflow passes write validation and exports cleanly. The plan-workflow and implement-workflow harbor suites pass with updated oracles. The docs site builds and type-checks with the new page linked from the navigation, and the full Go test suite passes.

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

## Open Questions

- **Does `ToYAMLFile` now emit `plan.task_id` into rewritten config files and migration goldens?** Depends on how the default-seeded `PlanConfig` marshals with `omitempty` on a struct field, which surfaces only when the config and migrate test suites run. If goldens change, update them only when the change is the new key alone; if anything else shifts, STOP and ask the user.
- **Does an in-flight plan workflow paused at the `phases` step exist in any user project at release?** Only observable at upgrade time. If `plan goto` reports an `invalid_transition` for `phases` during implementation testing, STOP and ask the user whether to add a resume alias from `phases` to `tasks`.

No other implementation-time uncertainties remain; every other decision is recorded in the assumption log.

## Out of Scope

- **Running independent tasks concurrently.** Implement runs stay one at a time per project, because there is a single workflow state file. This is tracked in issue #62.
- **Dependencies on tasks in other specs' plans.** Ids make these expressible, but every dependency must resolve inside the same plan for now.
- **Marking tasks complete from outside Spektacular.** Orchestrators implement through `implement new` with a `task` instead.
- **Changes to Hive.** Hive adopts the `repo` and `execution` object shapes itself, and the note is on issue #50.
- **The artifact addressing and status overhaul in #46**, beyond the per-task progress added to `plan status <name>`.
- **Export formats beyond `pretty` and `json`**, such as YAML.
- **Exporting specs or implementation runs, and any work breakdown finer than a task.**
- **Migrating or rewriting plans written before this feature.** They keep whole-plan implement and milestone commits, and are refused only by export and single-task implement.
- **Inferring a repo's location from git remotes or from file-provider sources.** `repo.location` is filled only from a declared git source.
- **Other task id providers.** Only `uuid` ships. The registry is the extension point, and providers with external side effects are a future concern.
- **Adding per-task progress to `plan status` without a name** (the in-progress workflow view). Only the named artifact status gains it.

## Changelog

### 2026-09-25 — Phase 1.1: Add the plan task reader and validator

**What was done**: Added `internal/plantask`, the single reader of a plan's `## Milestones & Tasks` (or legacy `## Milestones & Phases`) section. `Parse` returns milestones and tasks (id, title, milestone, repo, dependency ids, execution, completion, criteria met/total) and reports the plan's format; `Validate` applies the design's structural rules and returns a `plan_task_invalid` error naming the task; `RequireTasks` returns `plan_structure_invalid` ("the plan contains no task ids") for legacy or task-less plans.

**Deviations**: Tests were written alongside the code in the implement step rather than by a separate test sub-agent. Added `Plan.OpenItems()` and `Plan.CompletedMilestones()` beyond the planned interface, ready for Phase 1.4. `Validate` takes no command-name parameter: next_action names commands without the binary prefix (`'plan task-id'`), matching `internal/design/errors.go`.

**Files changed**:
- `spektacular: internal/plantask/plantask.go`
- `spektacular: internal/plantask/validate.go`
- `spektacular: internal/plantask/plantask_test.go`
- `spektacular: internal/plantask/validate_test.go`

**Discoveries**: A repeated `### Milestone N:` heading continues the same milestone (the old autocommit scanner merged by number), so `Parse` merges too. The field separator accepts `—`, `-`, `--` and `:` so hand-typed plans still parse.

### 2026-09-25 — Phase 1.2: Refuse invalid task structure when a plan is saved

**What was done**: `newStoreFileCmd` gained an optional `writeValidator`, called after front matter is stripped and before the metadata merge and store write, so a refusal leaves the stored file untouched. The plan group passes `validatePlanDocument`, which checks only `plan.md`, only when it is in the task format, against the registered repo names from `cfg.Repos`.

**Deviations**: The spec store-file caller lives in `cmd/file.go`, not `cmd/spec_file.go` as the plan said (drift noted at read_plan). Added a shared cmd test fixture file (`cmd/plantask_fixture_test.go`) for task-format plans, for reuse by the export, status and implement tests.

**Files changed**:
- `spektacular: cmd/storefile.go`
- `spektacular: cmd/plan_file.go`
- `spektacular: cmd/file.go`
- `spektacular: cmd/changelog_file.go`
- `spektacular: cmd/plantask_fixture_test.go`
- `spektacular: cmd/plan_file_validate_test.go`

**Discoveries**: Every test project registers at least one repo (`writeSpecCommandConfig` adds `testproj`), so the unregistered-repo rule always has a list to offer.

### 2026-09-25 — Phase 1.3: Issue task identifiers from a pluggable provider

**What was done**: Added `plan task-id`, which prints `{ "id": ... }` from the provider named by `plan.task_id.provider`. The provider registry lives in `internal/identifier/taskid.go` with one entry, `uuid`, a v4 UUID from `crypto/rand`. An unknown name is refused only when an id is requested, with `task_id_provider_unknown` naming it and listing the available providers. `PlanConfig` gained `TaskID` (default `uuid`, `config.DefaultTaskIDProvider`) with no schema bump.

**Deviations**: None.

**Files changed**:
- `spektacular: internal/identifier/taskid.go`
- `spektacular: internal/identifier/taskid_test.go`
- `spektacular: internal/config/config.go`
- `spektacular: cmd/plan_task_id.go`
- `spektacular: cmd/plan_task_id_test.go`
- `spektacular: cmd/root_test.go`

**Discoveries**: Open question resolved: `ToYAMLFile` now writes `plan.task_id.provider: uuid` into newly written configs, and no migration golden changed. `cmd/root_test.go` pins the `plan` subcommand list in its unknown-subcommand test, so each new `plan` subcommand updates it. `plan file list` on an empty plan store fails with `not found` (existing behaviour).

### 2026-09-25 — Phase 1.4: Move milestone commits and unchecked-work counts onto the task reader

**What was done**: `autocommit.CompletedMilestones` now delegates to `plantask.Parse(...).CompletedMilestones()`, so milestone commits fire for task plans as well as phase plans. `implement status` computes `unchecked_phases` with `plantask.Parse(...).OpenItems()` (open tasks, or open phase headings in older plans); the field name is unchanged. The commit-message refusal now says "whose tasks are now all complete".

**Deviations**: None. To keep legacy behaviour identical the reader was adjusted: a non-milestone `###` heading no longer ends the current milestone, and open phase headings are counted even when no milestone heading precedes them (the old `unchecked_phases` regex did).

**Files changed**:
- `spektacular: internal/autocommit/milestones.go`
- `spektacular: internal/autocommit/milestones_test.go`
- `spektacular: internal/autocommit/message.go`
- `spektacular: internal/plantask/plantask.go`
- `spektacular: internal/plantask/plantask_test.go`
- `spektacular: cmd/implement.go`
- `spektacular: cmd/implement_test.go`
- `spektacular: cmd/autocommit.go`
- `spektacular: cmd/milestones_test.go`

**Discoveries**: The old `unchecked_phases` regex matched open phase headings anywhere in plan.md; the reader counts them only inside the Milestones section, a slight tightening that no test or fixture depends on.

### 2026-09-25 — Phase 2.1: Add `plan export` with pretty and JSON output

**What was done**: Added `plan export <name> [--format pretty|json]`. It reads plan.md through the plan store at call time, parses it with the task reader, refuses plans without task structure (`plan_structure_invalid`), and builds `plantask.Export` in the design's shape. `repo.location` comes from the repo's repo.yaml only when its source provider is git (never from git remotes, never cloning). JSON goes through the output writer; `pretty` is a pure renderer (`plantask.RenderPretty`). Unsupported formats give `export_format_unsupported`; a missing plan gives `artifact_not_found`. Document status comes from a new shared `resolveDocumentStatus(fm, strictPlanStatusHook(...))`, which `runArtifactStatus` now uses too.

**Deviations**: The format is checked before config or plan are read, so a bad format never prints tasks. The export model and renderer live in `internal/plantask/export.go`.

**Files changed**:
- `spektacular: cmd/plan_export.go`
- `spektacular: cmd/plan_export_test.go`
- `spektacular: cmd/artifact_status.go`
- `spektacular: cmd/root_test.go`
- `spektacular: internal/plantask/export.go`
- `spektacular: internal/plantask/plantask_test.go`

**Discoveries**: `Set.DescriptiveMetadata` reads repo.yaml only for repos on disk, so a registry entry whose location is not checked out exports an empty location. The JSON output carries the usual `"error": false` discriminant from `output.Write`; unknown fields are ignored by Go decoders such as Hive's.

### 2026-09-25 — Phase 2.2: Report per-task progress in plan status

**What was done**: `plan status <name>` now adds `progress {tasks_completed, tasks_total}` and `tasks[] {id, title, milestone, completed, acceptance_criteria {met, total}}` for task-format plans, through a new plan-only `artifactBodyHook` passed to `runArtifactStatus` (spec status passes nil). Every existing field is unchanged, and plans without tasks report exactly the previous key set.

**Deviations**: Rather than extending the shared `artifactStatusOutputSchema` (which spec status also publishes), plan status's `--schema` uses a new `planArtifactStatusOutputSchema` that adds the two optional fields, so spec status's schema is unchanged.

**Files changed**:
- `spektacular: cmd/artifact_status.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/plan_status_progress_test.go`

**Discoveries**: None.

### 2026-09-25 — Phase 3.1: Select a task when starting implement

**What was done**: `implement new --data '{"name":…,"task":"<id>"}'` now runs `refuseUnstartableTask` after the stale-plan check and before the start gate and state reset. It refuses with `plan_structure_invalid`, `task_not_found`, `task_completed`, `task_dependencies_incomplete` (listing the open dependencies and naming the run to start first) or `task_requires_human` (with the reason), so no state is written. A valid task is stored in workflow data as `task`. `implement status` reports it as `task` (omitted for whole-plan runs), and the resume report's message and instruction carry it.

**Deviations**: None.

**Files changed**:
- `spektacular: cmd/implement.go`
- `spektacular: cmd/resume.go`
- `spektacular: cmd/resume_test.go`
- `spektacular: cmd/instruction_contract_test.go`
- `spektacular: cmd/implement_task_test.go`
- `spektacular: internal/steps/implement/result.go`
- `spektacular: templates/steps/resume_implement.md`

**Discoveries**: None.

### 2026-09-25 — Phase 3.2: Route feature wrap-up to the run that completes the last open task

**What was done**: The implement FSM gained a `update_changelog → finished` edge. In a single-task run the `update_changelog` callback reads the plan and passes `task` and `last_task` to the template, which names one exit: `test_plan` when no open task remains, `finished` otherwise. `finished()` skips the feature-changelog requirement and test-plan close only for a task run that leaves tasks open; a whole-plan run, or the task run that completes the last task, still requires the feature changelog. `update_changelog → finished` is a new completion commit point.

**Deviations**: Wiring decision (left to the implementer by the plan): `autocommit.LeadsToCommit` now takes the step's rendered next step, and a completion point counts only when it is that exit. Without this, adding the new completion point would have told every whole-plan run to make a completion commit at `update_changelog`. In full mode an implement completion commit also computes the due milestones, requires the message to name them and records them in `committed_milestones`, and the commit partial says so (`commit.may_close_milestone`).

**Files changed**:
- `spektacular: internal/steps/implement/steps.go`
- `spektacular: internal/steps/implement/steps_test.go`
- `spektacular: internal/autocommit/points.go`
- `spektacular: internal/autocommit/points_test.go`
- `spektacular: internal/stepkit/stepkit.go`
- `spektacular: cmd/autocommit.go`
- `spektacular: templates/partials/git-commit-message.md`
- `spektacular: templates/steps/implement/07-update_changelog.md`
- `spektacular: templates/steps/implement/12-finished.md`
- `spektacular: cmd/implement_task_run_test.go`

**Discoveries**: The FSM allows `update_changelog → finished` for every run, but `finished()` still refuses with `changelog_missing` unless the run is a task run with tasks left open, so neither a whole-plan run nor the last task's run can skip the wrap-up.

### 2026-09-25 — Phase 3.3: Scope the implement instructions and skill to the selected task

**What was done**: Added `partials/implement-current-task.md`, which defines "the current task": the selected task (title and id) in a single-task run, or otherwise the first unchecked `Task:` heading, falling back to the first unchecked `Phase N.M:` heading in a plan written before tasks. read_plan, analyze, implement, test, verify and update_plan now get `task {id,title}` from their callbacks (`taskExtra`), and their templates use the partial and limit the work, tests, verification and ticking to that task. read_plan accepts `## Milestones & Tasks` or `## Milestones & Phases`, and in a task run says the whole plan, context, research and referenced designs are still read. The remaining implement templates, the plan-documents partial, the commit partial, the resume template, the `spek-implement` skill (new "Implementing one task" section with the `task` input, id lookup via `plan export --format json` and the four refusals) and the four helper skills now use task wording.

**Deviations**: The criterion "asking the agent to implement a specific task starts a single-task run" is agent behaviour. It is left unticked until it is checked manually; the skill text is covered by `TestImplementSkillDocumentsSingleTaskRuns`. This repo's own installed skills (`.claude/skills/…`) are not re-rendered: installed files are refreshed only by the user's `init`/`migrate`.

**Files changed**:
- `spektacular: templates/partials/implement-current-task.md`
- `spektacular: templates/partials/implement-plan-documents.md`
- `spektacular: templates/partials/git-commit-message.md`
- `spektacular: templates/steps/implement/01-read_plan.md`
- `spektacular: templates/steps/implement/02-analyze.md`
- `spektacular: templates/steps/implement/03-implement.md`
- `spektacular: templates/steps/implement/04-test.md`
- `spektacular: templates/steps/implement/05-verify.md`
- `spektacular: templates/steps/implement/06-update_plan.md`
- `spektacular: templates/steps/implement/07-update_changelog.md`
- `spektacular: templates/steps/implement/09-test_plan.md`
- `spektacular: templates/steps/implement/10-update_feature_changelog.md`
- `spektacular: templates/steps/implement/11-reconcile_spec.md`
- `spektacular: templates/steps/implement/12-finished.md`
- `spektacular: templates/steps/resume_implement.md`
- `spektacular: templates/skills/workflows/spek-implement/SKILL.md`
- `spektacular: templates/skills/skill_update-changelog.md`
- `spektacular: templates/skills/skill_verify-implementation.md`
- `spektacular: templates/skills/skill_follow-test-patterns.md`
- `spektacular: templates/skills/skill_spawn-implementation-agents.md`
- `spektacular: templates/skill_resume_test.go`
- `spektacular: internal/steps/implement/steps.go`
- `spektacular: internal/steps/implement/steps_test.go`
- `spektacular: cmd/instruction_contract_test.go`

**Discoveries**: This implement run is itself a whole-plan run of a legacy phase plan on the updated binary, and the updated templates drove it correctly from Phase 3.3 onwards.
