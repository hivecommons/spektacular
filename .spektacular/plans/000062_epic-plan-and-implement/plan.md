---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Plan: 000062_epic-plan-and-implement

<!-- Metadata -->
<!-- Created: 2026-10-04T07:55:30Z -->
<!-- Commit: 2688364 -->
<!-- Branch: design/epics-and-seeded-specs -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Lets a user take a whole epic forward with two requests, "plan this epic" and then "implement this epic", instead of planning and implementing each spec by hand. Two new agent skills drive every outstanding spec through the standard per-spec plan and implement skills, using parallel subagents in dependency order: plans are made in the project's working copy, and implementations each run in their own git worktrees, one per repo they touch, and are merged back at every dependency junction. Small CLI additions make this deterministic and resumable: orchestrated workflows with their own per-spec state, a `run` view in `status` saying what each spec needs next, and worktree and merge helpers. Users with multi-spec epics benefit from far fewer interruptions, and an interrupted run picks up where it stopped when the request is repeated.

## Conventions

- **Error messages must describe the problem and suggest remediation**: `epic worktree`, `epic merge` and the name-routed `goto` add new refusals:
  - `epic_unplanned`
  - `epic_dependency_cycle`
  - `epic_dependency_outside`
  - `epic_merge_conflict`
  - `workflow_not_found`

  Each one must be built with `output.NewError(...).WithNextAction(...)` and give a runnable next step.
- **Spektacular's own files are written through Spektacular**: the epic skills and the child subagents read and write plans only through `plan file`, and every subagent prompt has to restate this rule. The new lane files under `.spektacular/workflows/` are workflow state, not a store, and only the CLI writes them.
- **A plan never changes the active skills and configuration**: this plan changes step templates, skills and workflow state handling. Verify all of it with `go test` and throwaway projects. Never re-init or migrate this repo, and never run `go run .` to drive a workflow.
- **Tests must not depend on execution order**: the new `cmd` tests go through `resetRootCmd` and the shared helpers. Git-backed tests (worktree, merge, scoped commit) each use their own `t.TempDir()` repo.
- **Passing tests are required before calling work done**: `go test ./...` must pass in full. That includes fixing the compile error already present at HEAD (the duplicate `installerFor`).
- **Docs: MDX authoring, site layout, alternating section background, no em dashes, plans must sketch content structure**: the new epics section in `spektacular-website/src/pages/epics.mdx` must follow these rules:
  - compose it from `Section`, with no layout HTML;
  - set `surface` to alternate with its neighbours;
  - write no em dashes;
  - give it a content outline in its task.

## Architecture & Design Decisions

**Options weighed (all within the `epics-and-seeded-specs.md` design).** The design fixes three things: an epic is never planned or implemented itself, plans stay one-to-one with specs, and `status` derives every state. The options below differ only in how much of the orchestration the CLI does deterministically and how much is left to agent prose.

- **A. New epic skills backed by thin, deterministic CLI support. Chosen.**
  - Two new installed skills, `spek-plan-epic` and `spek-implement-epic`, act as orchestrators. Each subagent runs the unchanged `spek-plan` or `spek-implement` skill for one spec.
  - The CLI gains four things:
    - *orchestrated* workflows, each with its own per-name state file, so several can be in progress at once;
    - a `run` view in the existing `status` command, saying for planning and for implementing what is done, running, ready, blocked or broken;
    - two git helpers, `epic worktree` and `epic merge`, that create a spec's worktree and combine it back, aborting on conflict;
    - small template hooks, so an orchestrated plan defers its walkthrough and an orchestrated implement loops without asking between tasks.
  - Pros: order, resume and the refusals are computed by code and covered by `go test`, not re-derived by each agent. Standalone runs keep the shared `state.json` exactly as today. It extends the `repo-state.json` precedent (`cmd/repo.go:184-189`) and the existing "(or an orchestrator)" hook (`spek-implement/SKILL.md:73-87`).
  - Cons: the largest change surface. Every plan and implement `goto` line gains a `name`, and two new skills must be registered.
  - Effort: High.
- **B. The epic skills do everything in prose; the CLI only gains per-name state.**
  - Agents would read `status` JSON, work out the order, and run `git worktree` and `git merge` themselves.
  - Pros: fewer Go changes.
  - Cons:
    - `status` cannot answer the questions this needs: it counts draft plans as `planned` (`internal/status/classify.go:57-77`), reports `implemented` before the wrap-up (`internal/steps/implement/steps.go:27-42`), and never re-checks cycles on read (`internal/epic/epic.go:82-122`).
    - A worktree's in-progress state is invisible from the main copy.
    - Resume would rest on agent reasoning, with no test coverage.
  - Effort: Medium.
- **C. Add an epic mode to `spek-plan` and `spek-implement`.**
  - Rejected. Their stop points are pinned by tests, and skills cannot branch at runtime (`internal/agent/skills.go:51`).
  - Effort: Medium, but brittle.
- Evidence for every option is in `research.md#alternatives-considered-and-rejected`.

**Shape of the solution (spektacular repo).**

*Orchestrated workflows* (`cmd/plan.go`, `cmd/implement.go`, `cmd/resume.go`, `cmd/autocommit.go`, `internal/workflow`, `internal/stepkit`, the plan and implement step templates)
- `plan new` and `implement new` accept `"orchestrated": true`. Such a workflow keeps its state in `.spektacular/workflows/<kind>-<name>.json` and its working notes in `.spektacular/workflows/<kind>-<name>.md`, instead of the shared `state.json` and `working-context.md`.
- It probes only its own file for a resume, so a standalone workflow never blocks it and it never blocks one.
- It skips the start gate. The orchestrator raises uncommitted work once, at the start of the run.
- Every rendered plan and implement `goto` now carries `"name"`:
  - `goto` reads the name, removes it from the data before copying the rest into workflow data, and routes to that name's lane file if one exists;
  - otherwise it uses `state.json` and refuses on a name mismatch (`workflow_not_found`, which names the lanes in progress);
  - a `goto` with no name keeps today's behaviour.
- Scratch paths that would clash under parallel plans become per name: the assembled plan documents move to `.spektacular/tmp/<plan_name>/`, and the staged commit message to `.spektacular/tmp/<name>/git-commit-message.md`.
- An orchestrated plan's walkthrough step:
  - does not ask for sign-off;
  - instead returns a short summary of the plan to the orchestrator and advances, so the plan closes `final`.
- An orchestrated implement loops between tasks without asking. This is the existing "run without asking" path (`templates/steps/implement/07-update_changelog.md:93-94`).
- Genuine STOPs stay STOPs, and the child hands them to its orchestrator rather than to the user. The genuine STOPs are:
  - an unresolved design reference;
  - a decision with no reasonable default;
  - implementation mismatches;
  - failed verification.
- Auto-commit:
  - an orchestrated plan commits only its own paths (its plan store directory, its work directory and its lane files), under a project-level commit lock, so parallel plans in one working copy never sweep each other's files;
  - an orchestrated implement commits normally, because it runs alone in its worktree;
  - a lane's files are removed when it reaches `finished`, before that commit.

*The `run` view in `status`* (`internal/status`, using `internal/epic` and `internal/depgraph`; no new command or package)
- `status <name>` gains a `run` block on each spec, with one part for planning and one for implementing. Each part gives the spec's run state:
  - `done`
  - `in_progress` (live lane, with its `current_step` and where it runs)
  - `awaiting_merge` (implementing only)
  - `ready`
  - `blocked` (with `waiting_on`)
- The implementing part also lists the repos the spec's plan touches.
- What "done" means:
  - for planning, a closed `final` plan with no live plan workflow;
  - for implementing, every task ticked, a final changelog record, no live implement workflow, and no unmerged worktree.
- The epic gains a `run` block with:
  - the dependency order (ties follow the epic's list order);
  - a summary count for each part;
  - `dirty`, saying whether any registered repo has uncommitted changes;
  - `problems`, each with a stable code, the specs involved, a message and whether it blocks implementing:
    - `epic_unplanned`, naming every spec without a final plan;
    - `epic_dependency_cycle`;
    - `epic_dependency_outside`, for a dependency outside the epic that is not implemented.
- **`status` still never refuses; it reports.** The implement-epic skill refuses up front, implementing nothing, whenever `problems` holds anything that blocks implementing, and relays its message.
- **Planning is never blocked by a problem**, because the design says the tool never refuses to plan over dependencies. A cycle or an outside name only drops the ordering between the specs involved.
- The existing `ready` and `blocked_by` fields stay exactly as they are, so current callers and the implement dependency check are unaffected.
- This keeps the design's "one command for status": everything a caller needs about an epic, including what to do next, comes from `status`. It also reuses the classifier that the implement check and the completed-epic guard already share.
- `depgraph` gains the cycle-member detection this needs, next to `FindCycle`.
- Because `status` reads only the stores, lane files and worktrees, repeating a request resumes exactly where the epic stands. There is no epic-run progress file.

*`epic worktree` and `epic merge`* (`cmd/epic_worktree.go`, new `internal/worktree` package over `internal/gitexec`)
- **Every repo a spec touches gets its own worktree.** The repos come from the spec's plan: the project's own repo, plus every registered repo named by the plan's tasks. Repos that share one git work tree get a single worktree.
- `epic worktree`, given a spec name:
  - creates `.spektacular/worktrees/<spec>/<repo>` for each touched repo, on branch `spek/<spec>` from that repo's current HEAD;
  - writes a *repo overlay* into the project worktree's `.spektacular/`. It is kept out of git and maps each repo name to its worktree;
  - returns the existing worktrees if they are already there, so a resume finds them.
- **The overlay redirects every repo inside the worktree.** It is applied where the registered repos are resolved (`repo.New`, `internal/repo/set.go:66`). So from inside the worktree, every repo resolves to that spec's worktrees and never to the shared checkouts. That covers `repo list`, the repo roots the implementing agent is told to work in, each repo's knowledge and changelog stores, and auto-commit targets. Without it, a relative location such as `docs`'s would resolve from the wrong place (`internal/repo/set.go:93-124`).
- `epic merge`, given a spec name, is all-or-nothing across repos:
  - first runs a dry-run merge (`git merge-tree --write-tree`) of `spek/<spec>` into every touched repo's current branch;
  - if any repo would conflict, merges nothing and returns `epic_merge_conflict` listing the conflicted paths per repo;
  - otherwise merges each repo with `--no-ff`, then removes every worktree and branch for the spec.
- No spec has to run on its own: specs touching other repos run in parallel like any other.
- `status` reads lane files when it reports a plan or spec `current_step` (`internal/status/report.go` `currentStep`), so a live orchestrated plan shows its step.

**Skills** (`templates/skills/workflows/spek-plan-epic/SKILL.md`, `spek-implement-epic/SKILL.md`, registered in `internal/agent/skills.go` and `commands.go`)
- The descriptions quote "plan this epic" and "implement this epic" so plain wording triggers them (precedent: spek-new).
- Each skill runs a loop:
  1. Read `status <epic> --format json`.
  2. Start one subagent per ready spec, giving it the spec name, the root to run in, the orchestrated `new` (or the `goto` that resumes it) and the hand-back rules.
  3. Report progress.
  4. Relay any question a child raises to the user and send the answer back, while the other children carry on.
  5. When a child finishes, merge it (implement), then read `status` again.
- On a failure, or on a question the user declines to answer now, the skill starts nothing new, lets running children finish, and reports what stopped the run and why.
- Planning ends with one summary per plan produced in the run, for the user to review. Changes the user asks for are applied through `plan file write`.
- Every run ends by listing what was completed, skipped as already done, and still outstanding.

**Documentation (docs repo).**
- `src/pages/epics.mdx` gains a "Planning and implementing an epic" section.
- The sentence "an epic is never planned or implemented itself" (`:29-31`) stays true, because each spec still gets an ordinary plan. It gains a link to the new section. The chaining sentence (`:223-226`) also stays true: chaining is part of specifying, and specifying is unchanged.
- The README's How It Works section gets a short note.

**Why A wins.** Every rule the spec makes testable is computed by code that `go test` covers, rather than restated in two skills' prose:
- the order of work;
- what is outstanding;
- resuming part-way through;
- refusing broken or unplanned epics;
- conflict detection.

This follows the project's model: the CLI is the state machine and the agent is the executor. Standalone `spek-plan` and `spek-implement` behave exactly as before.

**Conventions.**
- Every new refusal carries a runnable `next_action` (*error messages must suggest remediation*).
- Store files are reached only through the CLI (*store files must be written through the CLI*).
- Template and state changes are verified with `go test` and throwaway projects, never on this repo's install (*plans never change the active install*).
- The full suite stays order-independent and green (*tests must not depend on order*, *tests must pass for done*).

## Component Breakdown

**Changed components (spektacular repo)**

- **Workflow state routing** (changed). Owns where a workflow's state and working notes are kept and how a command finds them.
  - Standalone workflows keep the shared `state.json` and `working-context.md`.
  - An orchestrated plan or implement workflow is a *lane*: its own state file and notes file, keyed by kind and spec name.
  - `new` decides the slot; `goto`, `status` and the session log resolve it from the name.
  - Owns the name-mismatch and lane-not-found refusals.
  - Reuses the existing `workflow.New` engine unchanged. Only the path it is handed differs, following the separate `repo-state.json` already used by the guided repo add.
- **Plan and implement `new` handlers** (changed).
  - Accept `orchestrated`.
  - For a lane: probe only that lane for a resume, skip the start gate, and record `orchestrated` in workflow data so steps can render for it.
  - Refuse a standalone `new` for a spec whose lane is in progress.
- **Plan and implement `goto` handlers** (changed).
  - Accept `name`.
  - Strip it from the data before the remaining keys are copied into workflow data.
  - Route to the lane or the shared slot through workflow state routing.
- **Auto-commit** (changed). Gains a path-scoped commit for plan lanes, run under a project-level commit lock:
  - only the lane's plan store directory, its work directory and its lane files are staged and committed;
  - standalone and implement-lane commits keep today's whole-tree behaviour.
  - The staged commit-message location becomes per name.
- **Plan step templates** (changed).
  - Every `goto` line carries the name.
  - The assembled documents are staged per name.
  - The working-context footer names the lane's notes file when orchestrated.
  - The walkthrough step has an orchestrated branch: return a plan summary to the orchestrator and advance to `finished` with no sign-off.
- **Implement step templates** (changed).
  - Every `goto` line carries the name.
  - The working-context footer is lane-aware.
  - When orchestrated, the between-task question is skipped and the run loops on its own.
  - Every STOP is handed back to the orchestrator rather than asked of a user directly.
- **Resume templates** (changed). The resume `goto` carries the name, and the report names the lane when one is in progress.
- **Status report** (changed). A plan's or spec's `current_step` also reads that spec's lane, so a live orchestrated plan shows its step.
- **Dependency graph helpers** (changed). Beside `FindCycle`, add:
  - a function that returns every spec on a cycle;
  - a stable topological ordering that breaks ties by the epic's list order (the order the design says breaks ties between ready specs).

**New components (spektacular repo)**

- **Status report** (changed, further). Gains the `run` view on each spec and on the epic:
  - planning and implementing run states;
  - the repos each spec's plan touches;
  - the dependency order and summary counts;
  - uncommitted changes;
  - problems that block implementing.

  Built on the existing classifier, the epic graph and the dependency helpers. It reads only the stores, the lanes and the worktrees, so the same request always resumes from where the epic stands. No new command or package: the design already made `status` the one place a caller learns where work stands.
- **Worktree manager** (new, `internal/worktree`, over the existing git runner).
  - Creates a worktree and branch for a spec in every repo the spec's plan touches, from each repo's current HEAD, or returns the existing ones.
  - Writes the repo overlay into the project worktree.
  - Lists a spec's worktrees for the planner.
  - Merges a spec across all its repos as one unit: a dry run in every repo first, and nothing merged if any repo would conflict. On success it removes every worktree and branch.
  - New because nothing in the codebase manages worktrees today.
- **Repo overlay** (changed: repo resolution). When a project worktree carries a repo overlay, resolving the registered repos maps each named repo to its worktree location. Everything built on repo resolution then follows: repo roots, per-repo stores and auto-commit targets. A project without an overlay is unaffected.
- **`epic worktree` and `epic merge` commands** (new). Thin CLI verbs over the worktree manager, so the orchestrating skill never hand-rolls git commands and every outcome is a JSON result or a remediable refusal.
- **`spek-plan-epic` skill** (new). The planning orchestrator.
  - Resolves the epic from a name or plain wording.
  - Loops on the `run` view of `status` and starts one subagent per ready spec, each running the standard `spek-plan` as an orchestrated lane in the project's working copy.
  - Relays genuine questions to the user and sends the answers back, while the other children keep going.
  - Stops starting new specs on a failure or a declined question.
  - Reports progress after each change.
  - Ends with one summary per plan produced, applying any review changes through `plan file write`, and a completed / skipped / outstanding report.
- **`spek-implement-epic` skill** (new). The implementation orchestrator.
  - Refuses up front when `status` reports a problem that blocks implementing.
  - For each ready spec, creates its worktrees (one per touched repo) and starts a subagent running the standard `spek-implement` as an orchestrated lane in the spec's project worktree.
  - Merges each finished spec before its dependents start, and reports conflicts.
  - Applies the same relay, stop-on-failure, progress and final-report rules as `spek-plan-epic`.
- **Skill registration** (changed). The installer's skill list and the Bob command descriptions gain the two epic skills, so `init` and `migrate` install them for every agent.

**Documentation (docs repo)**

- **Epics page** (changed).
  - A new section covers planning and implementing an epic, including resume and skip behaviour.
  - The existing statement that an epic is never planned or implemented itself stays true, because each spec still gets its own ordinary plan and implementation. It gains a pointer to the new section rather than a correction.
- **Project README** (changed, spektacular repo). A short pointer in How It Works to the two epic requests.

## Data Structures & Interfaces

**Workflow slot.** The location a workflow's state and working notes live in. Workflow state routing resolves it, and the `new`, `goto`, `status` and session-log paths consume it.

```go
type Slot struct {
    Kind         string // "plan" | "implement" (lanes); any kind for the shared slot
    Name         string // spec name; "" when unknown (a goto with no name)
    StatePath    string // .spektacular/state.json  |  .spektacular/workflows/<kind>-<name>.json
    NotesPath    string // .spektacular/working-context.md  |  .spektacular/workflows/<kind>-<name>.md
    Orchestrated bool   // true for a lane
}

// ResolveSlot finds the slot for a goto: the lane if one exists for kind+name,
// else the shared slot, refusing when the shared slot holds a different name.
func ResolveSlot(dataDir, kind, name string) (Slot, error)
```

The `workflow.State` JSON shape is unchanged. A lane's file has exactly the same format as `state.json`. Its `data` also carries `"orchestrated": true`, which step templates render against.

**Orchestration inputs on existing commands** (additions to `--data`):

```jsonc
// plan new / implement new
{ "name": "<spec>", "orchestrated": true }        // orchestrated: optional, default false
// plan goto / implement goto
{ "step": "<step>", "name": "<spec>" }             // name: optional; routing key, never stored in data
```

**`run` view in `status`.** Added to the existing `status <name> --format json` report and read by both epic skills. Existing fields are unchanged and elided here.

```jsonc
{
  "requested": "000070_example",
  "epic": {
    "name": "000070_example",
    "...": "existing fields",
    "run": {
      "order": ["000071_a", "000073_c", "000072_b"],          // dependency order, ties in list order
      "plan":      { "done": 1, "in_progress": 1, "ready": 1, "blocked": 0, "remaining": 2 },
      "implement": { "done": 0, "in_progress": 0, "awaiting_merge": 0, "ready": 0, "blocked": 3, "remaining": 3 },
      "dirty": false,                                         // any registered repo has uncommitted changes
      "problems": [
        { "code": "epic_unplanned" | "epic_dependency_cycle" | "epic_dependency_outside",
          "specs": ["..."], "message": "...", "blocks": ["implement"] }
      ]
    }
  },
  "specs": [
    {
      "name": "000071_a",
      "...": "existing fields (state, ready, blocked_by, plan, ...)",
      "run": {
        "plan": {
          "state": "done" | "in_progress" | "ready" | "blocked",
          "waiting_on": ["..."],        // blocked only: dependencies without a final plan
          "current_step": "tasks",      // in_progress only
          "root": "/abs/path"           // in_progress only: where the lane runs
        },
        "implement": {
          "state": "done" | "in_progress" | "awaiting_merge" | "ready" | "blocked",
          "waiting_on": ["..."],        // blocked only: dependencies not implemented and merged
          "current_step": "analyze",    // in_progress only
          "root": "/abs/path",          // in_progress / awaiting_merge: the spec's project worktree
          "repos": ["spektacular", "docs"]  // repos the spec's plan touches; each gets a worktree
        }
      }
    }
  ]
}
```

A standalone spec gets the same per-spec `run` block, with `epic` null as today.

**Worktree manager interface.**

```go
type RepoWorktree struct {
    Repo   string // registered repo name
    Path   string // <project>/.spektacular/worktrees/<spec>/<repo>
    Branch string // spek/<spec>
}

type SpecWorktrees struct {
    Spec    string
    Project string         // path of the project repo's worktree: where the lane runs
    Repos   []RepoWorktree // every touched repo, project repo first
}

type Manager interface {
    Ensure(spec string, repos []string) (SpecWorktrees, bool /*created*/, error) // idempotent; writes the overlay
    List() ([]SpecWorktrees, error)                                              // spek/* worktrees only
    Merge(spec string) (MergeResult, error)                                      // all repos or none
}

type MergeResult struct {
    Spec      string
    Merged    bool
    Conflicts map[string][]string // repo name -> conflicted paths; set only when Merged is false (nothing merged)
    Removed   bool                // every worktree and branch removed after a clean merge
}

// Repo overlay, written into <project worktree>/.spektacular/ and kept out of git.
type RepoOverlay struct {
    Spec  string            `json:"spec"`
    Repos map[string]string `json:"repos"` // repo name -> absolute location inside its worktree
}
```

**`epic worktree` and `epic merge` CLI results.**

```jsonc
// epic worktree --data '{"spec":"<name>"}'
{ "spec": "...", "project": "/abs/.spektacular/worktrees/<spec>/spektacular", "branch": "spek/<spec>",
  "repos": [ { "repo": "spektacular", "path": "..." }, { "repo": "docs", "path": "..." } ], "created": true, "error": false }
// epic merge --data '{"spec":"<name>"}'
{ "spec": "...", "merged": true, "removed": true, "error": false }
```

A conflict returns the `epic_merge_conflict` envelope. Its `message` lists the conflicted paths per repo, nothing is merged in any repo, and `next_action` tells the agent to report the conflict to the user and stop.

**Dependency helpers** (additions beside `FindCycle`):

```go
func CycleMembers(order []string, deps map[string][]string) []string // every node on any cycle, in order
func TopoOrder(order []string, deps map[string][]string) []string     // stable; ties broken by order; cycle members appended in order
```

**Path-scoped commit.**

```go
// CommitPaths stages and commits only paths (additions, changes and deletions), under the project commit lock.
func (g Git) CommitPaths(dir string, paths []string, message string) error
```

**Hand-back contract between a child subagent and its orchestrator.** This is a skill-level convention, not a Go type. A child's final message begins with exactly one of these lines:
- `DONE: <spec>`, followed by the plan summary (planning) or the completion summary (implementing).
- `QUESTION: <spec>`, followed by the question, the options and any recommended default.
- `FAILED: <spec>`, followed by the step reached and the reason.

The orchestrator resumes a child that asked a question by sending it the user's answer.

## Implementation Detail

**New pattern: workflow lanes.**
- Today a command's workflow state is one fixed file chosen by the project root. This plan adds a second axis: the *slot*.
- The workflow engine stays exactly as it is, still handed a state path. Every handler that builds a state path goes through one slot resolver instead of calling the fixed-path helper directly.
- A developer reading a handler sees the resolver choose the slot first: the lane for an orchestrated `new`, or name-based routing for a `goto`. Everything after that is today's code.
- The resolver is the only place that knows the two layouts. Each refusal reads as one decision there:
  - a lane in progress blocks a standalone `new` for the same spec;
  - a name mismatch against the shared slot;
  - no workflow for that name.
- This follows the guided repo add's separate state file, generalised from one extra fixed file to one file per orchestrated spec.

**Name on every plan and implement `goto`.**
- The step templates render the spec name into every `goto` payload. So do the resume templates and the CLI's own next-action hints.
- The `goto` handlers strip `name` before copying data, the same way `commit_message_from` is stripped today, so the name never pollutes persisted workflow data.
- A `goto` without a name still works against the shared slot, so old instructions and hand-typed commands keep working.
- The instruction-contract tests gain a check that every rendered plan and implement `goto` carries the name.

**Orchestrated rendering, driven by workflow data, not skill prose.**
- `orchestrated` lives in the lane's workflow data, so the step templates branch on it with ordinary mustache sections:
  - the walkthrough's sign-off becomes a hand-back summary;
  - the between-task question disappears;
  - the working-context footer names the lane's notes file.
- This puts the behaviour change where the tests already pin plan and implement behaviour. The standalone wording stays byte-for-byte the same, and the existing pins continue to hold. New pins cover the orchestrated branches.

**Per-name scratch.**
- Fixed scratch names that two parallel plans would overwrite gain the spec name as a subdirectory: the assembled plan documents and the staged commit message.
- This holds whether or not the run is orchestrated, so there is one shape to read and test.

**Scoped auto-commit.**
- The auto-commit path learns one new case. When the workflow is a plan lane, it stages and commits only that lane's own paths, under a project-level lock that serialises commits across parallel lanes.
- Every other case (standalone, implement lanes in their worktrees) keeps today's whole-tree commit.
- The commit message rules (it must name the spec) are unchanged.
- On a failed commit, the snapshot and restore act on the lane's own state file, so they can no longer roll back another workflow.

**Extending `status` with a run view.**
- A pure function of what is on disk:
  - the epic's graph;
  - each spec's classification from the existing status classifier;
  - the lanes in the project and in each spec's worktree;
  - the worktrees themselves;
  - the repos each plan's tasks touch.
- It sits inside the status package beside the classifier, built when the report is built. So it is unit-testable with the same fixture projects the status tests already use, and repeating a request is naturally a resume.
- Its rules live as small, named predicates ("planned for planning", "implemented for implementing", "touched repos"), not inline in the report builder.
- Problems are decided here with stable codes, so the skill only checks a list and never infers them. Planning is never blocked, which keeps the design's rule that planning is never held back by dependencies.

**New module: the worktree manager.**
- A small wrapper over the existing git runner, with a fake for tests, mirroring how auto-commit swaps its git implementation in tests.
- Integration tests use real temporary repositories.
- Worktree location and branch naming are fixed conventions, so `status` can rediscover them without any stored record.

**New skills as orchestrators.**
- The two epic skills are a new orchestration shape for this project. They are agent-level loops over the `run` view of `status`, and their child subagents each drive an ordinary workflow.
- They follow the existing skill conventions:
  - the version-check partial first;
  - `{{command}}` rendering;
  - store access only through the CLI;
  - a description rich in trigger phrases, as spek-new has.
- They state the hand-back contract and the "genuine open question" definition once. Every child prompt repeats the store-access rule and the repo root to run in, because a subagent inherits neither.
- The existing `spek-plan` and `spek-implement` skills gain only a short paragraph. It says that when started by an orchestrator, they pass `orchestrated` and hand questions back instead of asking.

**Followed vs new.**
- Followed:
  - error envelopes with `next_action`;
  - CLI-owned state;
  - store access through the CLI;
  - mustache step templates branched on workflow data;
  - registered workflow skills;
  - fake git in unit tests.
- New:
  - lanes;
  - name-routed `goto`;
  - path-scoped commits under a lock;
  - git worktree management;
  - an epic-level read model;
  - orchestrator skills.

## Dependencies

**Design documents this plan was built on**
- `epics-and-seeded-specs.md` from the `design` design source. It is the settled shape this plan builds on:
  - what an epic is, with `specs[{name, depends_on}]` and list order breaking ties;
  - that an epic is never planned or implemented itself, and plans stay one-to-one with specs;
  - that dependencies constrain only implementation, and the tool never refuses to plan because of them;
  - that "implemented" means every task in the plan is complete;
  - that `status` derives every state.

  This plan adds no fields to the epic and changes none of these rules.

**Upstream work that must already be in place (all landed)**
- `000060_epics-and-seeded-specs`: the epic store, epic validation, the `status` command and its shared classifier, and the implement-time dependency check. The new `run` view is built on these; the existing fields, the implement check and the classifier stay unchanged.
- `000057_git-commit`: the auto-commit modes, commit points, commit-message validation and start gate. This plan extends it with a path-scoped commit for plan lanes.
- `000058_plan-task-graph`: task headings with a repo and dependencies. The planner and the worktree manager read each plan's task repos to decide which repos get a worktree.

**Must be fixed before any other task**
- The `cmd` package does not compile at HEAD because `installerFor` is declared twice. Removing the duplicate is the first task, since every later task verifies with `go test`.

**Internal packages**
- `internal/workflow`: unchanged engine; still handed a state path.
- `internal/status`: the classifier is reused unchanged. `current_step` resolution changes to read lanes.
- `internal/epic`: reused (`Parse`, `SpecNames`, `Validate` rules); no changes.
- `internal/depgraph`: gains cycle-member detection and a stable topological order.
- `internal/autocommit`: gains a path-scoped commit and a commit lock.
- `internal/gitexec`: reused as the git runner for the new worktree manager; no changes.
- `internal/plantask`: reused to read each plan's task repos and completion; no changes.
- `internal/agent`: skill registration gains the two epic skills.
- `internal/workingcontext`: gains lane-aware note paths.
- `internal/stepkit`: the commit-message and working-context footer rendering become per name and lane-aware.
- `internal/sessionlog`: reads the lane's state when a command names one.
- New package: `internal/worktree` (worktree manager). The run view lives in the existing `internal/status`.

**External**
- `git` with `worktree` support (git 2.5 or later, already implied by the project's minimum git use). No new Go modules.
- An agent with subagent orchestration that can continue a stopped subagent: Claude Code background agents with SendMessage, and the equivalents in Bob and Codex. Without it, the relay falls back to restarting a child on the same spec, which resumes its lane.

**Docs repo**
- `spektacular-website`, the documentation site: the epics page gains a section. It depends on the site's existing `Section` component and conventions; no site changes beyond content.

## Testing Approach

**Strategy.** All deterministic behaviour is pinned with `go test`. That covers ordering, outstanding work, resume, refusals, lanes, scoped commits, worktrees and merge conflicts. Only orchestration that depends on a live agent is left to a manual end-to-end run: overlapping subagents, the question relay and the review conversation. The tests follow the existing conventions:
- `cmd` tests go through `resetRootCmd` and the shared helpers;
- git-backed tests each build their own temporary repository;
- template tests render the real templates through the real installer into a temporary directory;
- the whole suite passes under `-shuffle=on`.

**Unit tests (most coverage).**
- **The `run` view in `status`.** This carries the spec's rules, so it gets the most coverage. Table tests over fixture projects check that:
  - planning order follows dependencies, and a dependency must have a *final* plan, not a draft;
  - specs whose dependencies are planned are `ready` together, and specs already planned are `done`;
  - a live lane gives `in_progress` with its step;
  - for implementing:
    - `implemented` needs every task ticked, a final changelog and no live lane;
    - an unmerged finished worktree is `awaiting_merge`;
    - each spec reports the repos its plan touches;
  - `problems` reports `epic_unplanned`, `epic_dependency_cycle` and `epic_dependency_outside` with the names in the message, each marked as blocking implementing;
  - planning is never blocked by a problem;
  - the existing `ready` and `blocked_by` fields are unchanged;
  - epic list order breaks ties.
- **Dependency helpers.** Cycle members and a stable topological order, including self-loops, disjoint graphs and names outside the order.
- **Slot resolution.**
  - Routing: a lane wins when present, the shared slot is used otherwise, and a name mismatch is refused.
  - Each refusal carries a `next_action`.
- **Path-scoped commit.**
  - Only the given paths are committed, deletions included.
  - Another lane's dirty files are left untouched.
  - The lock serialises concurrent callers.
- **Worktree manager** (fake git for argument shapes; real temporary repos for behaviour).
  - Creation is idempotent, and makes one worktree per touched repo (one for repos that share a work tree).
  - Listing shows only `spek/*`.
  - A clean merge lands in every touched repo and removes every worktree and branch.
  - If a conflict exists in any repo, nothing is merged in any repo, and the conflicted paths are reported per repo.
- **Repo overlay.** With an overlay present, repo resolution maps each repo (including a relatively located one such as `docs`) to its worktree, and so do the per-repo stores and the auto-commit targets. With no overlay, resolution is unchanged.

**Command (integration) tests.**
- `plan new` / `implement new` with `orchestrated`:
  - writes the lane file, not `state.json`;
  - runs alongside a standalone workflow and a second lane;
  - skips the start gate;
  - a repeated orchestrated `new` returns a resume report naming the lane's step.
- `goto` with `name` advances the right lane and never stores `name` in data.
- `goto` with no name behaves exactly as before.
- Two plan lanes can be driven step by step, interleaved, in one project and both reach `finished`. Their path-scoped completion commits contain only their own files.
- `status <spec>` shows a live lane's `current_step`.
- `status <epic> --format json` carries the `run` view. `epic worktree` and `epic merge` return the JSON shapes in the plan and the error envelopes on refusal.
- A spec touching two repos gets two worktrees. An implement lane run inside it reports both repo roots inside the worktrees. A conflict in the second repo returns `epic_merge_conflict` and leaves both repos' main lines clean.

**Contract tests (templates and skills).**
- Every rendered plan and implement `goto` carries the workflow name.
- Every scratch path is per name.
- The orchestrated walkthrough hands back a summary and advances with no sign-off question.
- The orchestrated implement loop has no between-task question.
- The lane footer names the lane's notes file.
- The standalone renderings are unchanged: the existing pins on the mandatory walkthrough and on "ask the user" between tasks still pass.
- Both epic skills are installed for every agent, start with the version check, render `{{command}}`, and use only CLI store access. Their descriptions contain "plan this epic" and "implement this epic".
- Both skills state:
  - the hand-back contract;
  - the definition of a genuine open question;
  - stop-on-failure ("start nothing new, let running work finish");
  - the end-of-planning review;
  - the final completed / skipped / outstanding report.

**Regression.** The full existing suite, which pins the single-workflow behaviour, resume, cross-kind refusals and status shapes, must stay green. Standalone runs are meant to be byte-for-byte unchanged apart from the added `name` in `goto` lines.

**Acceptance criteria that need a live agent.** Each item is **Manual — captured in the implementation test plan**.
- Plan or implement an epic on request, end to end: one request and no per-spec starts. A harbor-style run against a throwaway project.
- Independent specs run in overlapping time, for both planning and implementing.
- A child's open question reaches the user, the answer lands in that plan, and an independent child keeps going.
- Stop on failure and stop on a declined question: running work finishes, nothing new starts, and the final message is correct.
- Resuming part-way: interrupt the run and repeat the request. The unit and command tests cover the CLI side; the agent side is checked here.
- Progress messages during a run, and the review shown at the end of planning.

**Success metrics.**
- *"Epics are taken from written to implemented without the user starting any individual spec's plan or implementation."* **Manual — captured in the implementation test plan.** It needs a live agent driving both epic skills over a throwaway epic. The CLI preconditions (orchestrated lanes, the `run` view's ordering) are covered by behavioural tests above.
- *"Across real epic planning runs, the number of user prompts per run is at most one plus the number of genuine open questions raised."* **Manual — captured in the implementation test plan.** It is observed over real runs. The contract tests guarantee the skill and templates contain no other stop point: no per-section confirmation, no orchestrated walkthrough sign-off, and no per-spec resume or uncommitted-changes questions.

**Deliberate gaps.**
- No automated test spawns real subagents. The orchestration loop is agent behaviour, checked manually as above.
- The docs page is verified by the site's build and type check plus the MDX guard, not by Go tests.

## Milestones & Tasks

### Milestone 1: Several plan and implement workflows can be in progress at once

**What changes**: A plan or implement workflow can now be started as an *orchestrated* run that keeps its own progress record and notes.
- Several orchestrated runs, and an ordinary run, can be in progress in the same project without blocking or overwriting each other.
- Each plan's completion commit holds only that plan's own files.
- Every plan and implement instruction now names the spec it belongs to, so an agent can never move the wrong run forward.
- An orchestrated plan returns a summary instead of asking for sign-off.
- An orchestrated implementation carries on from task to task without asking.
- Ordinary single-spec runs look and behave as before.
- The milestone starts by removing a stray duplicate declaration that currently stops the code from compiling, which every later check depends on.

**Validation point**: Two orchestrated plan runs for different specs can be driven step by step, interleaved, in one project alongside an ordinary run, and both finish with correct, separate commits. The full test suite passes, including every existing check on ordinary runs.

#### - [x] Task: Restore a compiling build
**Id:** a78b1879-184c-4bbf-b728-34effbc9d780
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

The last commit left a second, identical copy of the migrate installer helper, so the command package no longer compiles and no test can run. Remove the duplicate so the suite runs again before any other change lands.

*Technical detail:* [context.md#task-restore-a-compiling-build](./context.md#task-restore-a-compiling-build)

**Acceptance criteria**:
- [x] The project builds and the full test suite runs and passes with no change in behaviour.

#### - [x] Task: Run orchestrated plan and implement workflows in their own lanes
**Id:** ec3dddb2-84b5-4e2c-8870-d839962ed9c3
**Repo:** spektacular
**Depends on:**
- a78b1879-184c-4bbf-b728-34effbc9d780 — Restore a compiling build
**Execution:** agent

Introduce workflow slots. Starting a plan or implement workflow with `orchestrated` keeps its progress and notes in a lane file of its own rather than the shared record, skips the uncommitted-changes gate, and records that it is orchestrated. `goto` accepts the spec name and routes to that name's lane, or to the shared record when the name matches. Name mismatches and unknown names are refused with a runnable next step. Ordinary runs keep the shared record exactly as today.

*Technical detail:* [context.md#task-run-orchestrated-plan-and-implement-workflows-in-their-own-lanes](./context.md#task-run-orchestrated-plan-and-implement-workflows-in-their-own-lanes)

**Acceptance criteria**:
- [x] Two orchestrated plan workflows for different specs and one ordinary workflow can all be in progress in one project, each advancing independently.
- [x] Repeating an orchestrated start for a spec whose lane is in progress returns a resume report for that lane instead of starting over.
- [x] An ordinary start for a spec whose lane is in progress is refused, naming the lane.
- [x] A `goto` naming a spec reaches that spec's workflow, and the name is never stored in the workflow's data.
- [x] A `goto` naming a spec that has no workflow, or that does not match the shared record, is refused with a next step.
- [x] A `goto` with no name behaves exactly as before.

#### - [x] Task: Name the spec in every plan and implement instruction and keep scratch files per spec
**Id:** 180b71a6-7359-48c3-9d60-9e674a36f94f
**Repo:** spektacular
**Depends on:**
- ec3dddb2-84b5-4e2c-8870-d839962ed9c3 — Run orchestrated plan and implement workflows in their own lanes
**Execution:** agent

Every `goto` that the plan and implement steps, the resume reports and the CLI's own hints print now carries the spec name, so an agent always routes to the right workflow. The scratch files plan steps stage (the assembled documents and the commit message) move to a per-spec folder, so two plans running at once never overwrite each other's staged files.

*Technical detail:* [context.md#task-name-the-spec-in-every-plan-and-implement-instruction-and-keep-scratch-files-per-spec](./context.md#task-name-the-spec-in-every-plan-and-implement-instruction-and-keep-scratch-files-per-spec)

**Acceptance criteria**:
- [x] Every rendered plan and implement instruction that advances the workflow names the spec it belongs to, and a contract test enforces this.
- [x] Resume reports for plan and implement tell the agent to resume with the spec named.
- [x] Staged plan documents and staged commit messages live in a folder for their own spec.
- [x] Ordinary plan and implement runs still pass every existing instruction check.

#### - [x] Task: Let an orchestrated run hand back instead of asking
**Id:** 0737c1b4-8835-4246-a780-e5e6911b9cbe
**Repo:** spektacular
**Depends on:**
- 180b71a6-7359-48c3-9d60-9e674a36f94f — Name the spec in every plan and implement instruction and keep scratch files per spec
**Execution:** agent

Teach the steps what to do when the run is orchestrated:
- The walkthrough returns a short plan summary to the orchestrator and finishes the plan without a sign-off question.
- An implementation loops from task to task without asking.
- Every genuine stop is phrased as a question handed back to the orchestrator.
- The working-notes footer points at the lane's own notes file.

Ordinary runs keep their wording unchanged.

*Technical detail:* [context.md#task-let-an-orchestrated-run-hand-back-instead-of-asking](./context.md#task-let-an-orchestrated-run-hand-back-instead-of-asking)

**Acceptance criteria**:
- [x] An orchestrated plan finishes, closing its documents final, without asking the user for sign-off, and hands back a summary of the plan.
- [x] An orchestrated implementation moves on to its next task without asking.
- [x] Orchestrated steps name the lane's own notes file, not the shared one.
- [x] The ordinary walkthrough and between-task question are unchanged, and their existing checks still pass.

#### - [x] Task: Scope orchestrated plan commits to their own files
**Id:** c42805f7-06dc-4373-bd55-6e71404802d5
**Repo:** spektacular
**Depends on:**
- ec3dddb2-84b5-4e2c-8870-d839962ed9c3 — Run orchestrated plan and implement workflows in their own lanes
**Execution:** agent

When auto-commit is on and an orchestrated plan finishes, the commit stages only that plan's store documents, its working files and its lane files. Commits are serialised through a project-level lock, so parallel plans in one working copy never sweep up each other's work or collide on git's index. A lane's files are removed when it finishes, and a failed commit restores only that lane's own record.

*Technical detail:* [context.md#task-scope-orchestrated-plan-commits-to-their-own-files](./context.md#task-scope-orchestrated-plan-commits-to-their-own-files)

**Acceptance criteria**:
- [x] A finishing orchestrated plan's commit contains its own plan, working files and lane files, and nothing belonging to another plan in progress.
- [x] Two plans finishing at the same moment both commit successfully, one after the other.
- [x] A failed commit leaves every other workflow's progress untouched.
- [x] Ordinary and implement-lane commits behave exactly as before.

#### - [x] Task: Show orchestrated runs in status and the session log
**Id:** f5868aaa-8731-4333-9cdb-037dcffb4de9
**Repo:** spektacular
**Depends on:**
- ec3dddb2-84b5-4e2c-8870-d839962ed9c3 — Run orchestrated plan and implement workflows in their own lanes
**Execution:** agent

`status` for a spec or epic reports the live step of any orchestrated plan for that spec, read from its lane, just as it does for the shared workflow today. The debug session log records commands against the lane they drove rather than whichever workflow holds the shared record.

*Technical detail:* [context.md#task-show-orchestrated-runs-in-status-and-the-session-log](./context.md#task-show-orchestrated-runs-in-status-and-the-session-log)

**Acceptance criteria**:
- [x] While an orchestrated plan is in progress, `status` for its spec shows the plan's current step.
- [x] Session-log entries for a lane's commands are filed under that lane.
- [x] Existing status output for ordinary runs is unchanged.

### Milestone 2: Spektacular can say what an epic still needs, and isolate parallel implementation

**What changes**: One read-only request reports, for planning or for implementing, where every spec in an epic stands:
- done, in progress (and at which step), awaiting merge, ready to start, or blocked (and on what);
- how many specs remain.

When asked about implementing, it refuses up front, naming the problem, if any spec is unplanned, if the dependencies form a cycle, or if a spec depends on an unimplemented spec outside the epic. Two companion requests isolate parallel implementation:
- one gives a spec its own git worktree in every repo it touches, with every repo resolving to those worktrees from inside;
- the other merges a finished spec back into every repo's main line together, or merges nothing and reports the conflicting files.

Because all of this is read from what is on disk, asking again after an interruption picks up exactly where the epic stands.

**Validation point**: Against fixture epics:
- the report orders work by dependency, skips finished specs, and shows in-progress runs;
- implementing refuses unplanned, cyclic and outside-dependency epics;
- a worktree is created, merged and removed;
- a spec touching two repos gets a worktree in each, and a conflict in either is reported with neither repo merged.

#### - [x] Task: Order epic specs and find cycles
**Id:** 58dd162c-be4d-4444-bb7c-82b33f1cb3ff
**Repo:** spektacular
**Depends on:**
- a78b1879-184c-4bbf-b728-34effbc9d780 — Restore a compiling build
**Execution:** agent

Add two graph helpers beside the existing cycle finder:
- one returns every spec that sits on a cycle;
- one returns a stable dependency order whose ties follow the epic's own list order.

The `run` view in `status` builds on both.

*Technical detail:* [context.md#task-order-epic-specs-and-find-cycles](./context.md#task-order-epic-specs-and-find-cycles)

**Acceptance criteria**:
- [x] Every spec on any cycle, including one depending on itself, is reported.
- [x] Specs are ordered so that every dependency comes first, with ties kept in list order.
- [x] Names outside the graph never break the ordering.

#### - [x] Task: Give each spec its own worktree and merge it back
**Id:** febac6f5-23c3-4b75-b38d-020596c30cd9
**Repo:** spektacular
**Depends on:**
- a78b1879-184c-4bbf-b728-34effbc9d780 — Restore a compiling build
**Execution:** agent

Add a worktree manager, a repo overlay and two epic commands:
- `epic worktree` creates a worktree and branch for the spec in every repo its plan touches, inside the project and kept out of git. It writes an overlay so that, inside the spec's project worktree, every registered repo resolves to that spec's worktrees. If they already exist, it returns them.
- `epic merge` merges a finished spec into every touched repo's main line as one unit. It first checks every repo for conflicts; if any would conflict, it merges nothing and reports the conflicting files per repo. On success it removes all the spec's worktrees and branches.

*Technical detail:* [context.md#task-give-each-spec-its-own-worktree-and-merge-it-back](./context.md#task-give-each-spec-its-own-worktree-and-merge-it-back)

**Acceptance criteria**:
- [x] A spec whose plan touches two repos gets a worktree in each, and asking again returns the same worktrees.
- [x] Inside the spec's project worktree, every touched repo (its code, knowledge and changelog) resolves to the spec's worktree, never to the shared checkout.
- [x] The worktree directories are never picked up by commits in the main working copy.
- [x] A clean merge brings the spec's changes into every touched repo's main line and removes its worktrees and branches.
- [x] If any repo would conflict, nothing is merged in any repo and the conflicting files are reported per repo.

#### - [x] Task: Report what an epic still needs in status
**Id:** d8ca7865-f5e6-4c4c-a657-b200501cdaca
**Repo:** spektacular
**Depends on:**
- 58dd162c-be4d-4444-bb7c-82b33f1cb3ff — Order epic specs and find cycles
- febac6f5-23c3-4b75-b38d-020596c30cd9 — Give each spec its own worktree and merge it back
- f5868aaa-8731-4333-9cdb-037dcffb4de9 — Show orchestrated runs in status and the session log
**Execution:** agent

Extend `status` with a run view, rather than adding a new command. For both planning and implementing, each spec is classified as done, in progress (with its step and where it runs), awaiting merge (implementing only), ready, or blocked (with what it waits on). The epic gets:
- the dependency order and counts;
- the repos each spec touches;
- whether any repo has uncommitted changes;
- a list of problems that block implementing: unplanned specs, cycles, and unimplemented outside dependencies.

Everything comes from what is on disk, so the same request resumes where the epic stands. The existing status fields are unchanged.

*Technical detail:* [context.md#task-report-what-an-epic-still-needs-in-status](./context.md#task-report-what-an-epic-still-needs-in-status)

**Acceptance criteria**:
- [x] For planning, a spec is ready only once every spec it depends on has a final plan, and specs already planned are reported done.
- [x] For implementing, a spec is ready only once every dependency is fully implemented and merged, and specs already implemented are reported done.
- [x] A spec with a live lane, in the project or in its worktree, is reported in progress at its current step.
- [x] A finished but unmerged spec is reported awaiting merge.
- [x] An epic with an unplanned spec, a cycle, or an unimplemented outside dependency reports each as a problem that blocks implementing, naming the specs.
- [x] Problems never block planning, and the existing status fields are unchanged.

### Milestone 3: "Plan this epic" and "implement this epic"

**What changes**: Users can ask the agent to plan an epic, or implement one, in plain words. The agent drives every outstanding spec through the standard per-spec skills, using separate agents that run side by side where dependencies allow:
- plans are made in dependency order in the project's working copy;
- implementations each run in their own worktrees, one per repo they touch, and are merged back before anything that depends on them starts.

A genuine open question from one spec is put to the user and the answer is sent back, while the other specs keep going. A failure, or a question the user chooses not to answer now, stops new work from starting and lets running work finish. The user sees progress as the run goes, a summary of every plan produced at the end of planning, and a final account of what was completed, skipped as already done, and still outstanding.

**Validation point**:
- Both skills are installed for every supported agent and pass the instruction contract checks.
- A manual end-to-end run on a throwaway three-spec epic plans and then implements it from two requests, with no per-spec starts.

#### - [x] Task: Write the plan-this-epic skill
**Id:** 55a5dad7-fd89-428a-b86c-1c640865c6d1
**Repo:** spektacular
**Depends on:**
- 0737c1b4-8835-4246-a780-e5e6911b9cbe — Let an orchestrated run hand back instead of asking
- c42805f7-06dc-4373-bd55-6e71404802d5 — Scope orchestrated plan commits to their own files
- d8ca7865-f5e6-4c4c-a657-b200501cdaca — Report what an epic still needs in status
**Execution:** agent

Write the `spek-plan-epic` skill. It works out which epic is meant, then loops on the `run` view of `status`, starting one subagent per ready spec. Each subagent runs the standard plan skill as an orchestrated run in the project's working copy, or resumes one already in progress. The skill:
- relays genuine questions to the user and sends the answers back while the other specs continue;
- stops starting new specs on a failure or a declined question;
- reports progress as it goes;
- ends with a summary of every plan it produced for the user to review, and an account of what was completed, skipped and still outstanding.

*Technical detail:* [context.md#task-write-the-plan-this-epic-skill](./context.md#task-write-the-plan-this-epic-skill)

**Acceptance criteria**:
- [x] The skill is triggered by plain wording such as "plan this epic" as well as by the epic's name.
- [x] It plans only specs without a plan, in dependency order, with independent specs started together.
- [x] Its only user stops are genuine open questions from a spec and the end-of-planning review.
- [x] It defines a genuine open question, the hand-back contract with its subagents, and stop-on-failure behaviour.
- [x] Repeating the request after an interruption resumes in-progress plans and skips finished ones.

#### - [x] Task: Write the implement-this-epic skill
**Id:** bc7be7b2-0add-4574-b7ed-8e46acf16832
**Repo:** spektacular
**Depends on:**
- 0737c1b4-8835-4246-a780-e5e6911b9cbe — Let an orchestrated run hand back instead of asking
- d8ca7865-f5e6-4c4c-a657-b200501cdaca — Report what an epic still needs in status
**Execution:** agent

Write the `spek-implement-epic` skill. It works out which epic is meant and refuses up front when `status` reports a problem that blocks implementing. Otherwise it loops: for each ready spec, it creates the spec's worktrees, one per repo its plan touches. A subagent then runs the standard implement skill as an orchestrated run in the spec's project worktree, where every repo resolves to that spec's worktrees. When a spec finishes, the skill merges it back before anything that depends on it starts, and stops to show the user any conflict. Question relay, stop-on-failure, progress and the final account follow the planning skill.

*Technical detail:* [context.md#task-write-the-implement-this-epic-skill](./context.md#task-write-the-implement-this-epic-skill)

**Acceptance criteria**:
- [x] The skill is triggered by plain wording such as "implement this epic" as well as by the epic's name.
- [x] It implements nothing when any spec is unplanned or the dependencies are broken, and names the problem.
- [x] Each independent spec is implemented in its own worktree at the same time as the others, and is merged back before its dependents start.
- [x] A merge conflict stops the run and is shown to the user, not resolved silently.
- [x] It asks for no input between specs unless a genuine question or failure arises.
- [x] Repeating the request resumes in-progress specs in their worktrees and skips implemented ones.

#### - [x] Task: Install the epic skills and teach the per-spec skills about orchestration
**Id:** 153eff91-88c7-43b3-8ecb-23cb093be6a0
**Repo:** spektacular
**Depends on:**
- 55a5dad7-fd89-428a-b86c-1c640865c6d1 — Write the plan-this-epic skill
- bc7be7b2-0add-4574-b7ed-8e46acf16832 — Write the implement-this-epic skill
**Execution:** agent

Register both epic skills, so that `init` and `migrate` install them for Claude, Bob and Codex, with Bob command wrappers. Add a short paragraph to the plan and implement skills explaining how they behave when an orchestrator starts them: pass `orchestrated`, carry the spec name, and hand questions back rather than asking. Update the tests that pin the installed skill list.

*Technical detail:* [context.md#task-install-the-epic-skills-and-teach-the-per-spec-skills-about-orchestration](./context.md#task-install-the-epic-skills-and-teach-the-per-spec-skills-about-orchestration)

**Acceptance criteria**:
- [x] A freshly initialised project has both epic skills installed for every supported agent.
- [x] The plan and implement skills explain what changes when an orchestrator starts them, and are otherwise unchanged.
- [x] Every installed skill still passes the instruction contract checks.

### Milestone 4: Planning and implementing an epic is documented

**What changes**: The public documentation's epics page explains how to plan and implement a whole epic:
- the two requests;
- the dependency order and parallel work;
- what interrupts the user;
- worktrees and merging;
- what happens on failure;
- that repeating a request resumes and skips completed work.

The project README points to it.

**Validation point**: The documentation site builds and type-checks cleanly with the new section in place, and the section covers resume and skip behaviour.

#### - [x] Task: Document planning and implementing an epic on the website
**Id:** 6d0a6408-f711-455f-aa8c-b941aa1a70c2
**Repo:** docs
**Depends on:**
- 153eff91-88c7-43b3-8ecb-23cb093be6a0 — Install the epic skills and teach the per-spec skills about orchestration
**Execution:** agent

Add a "Planning and implementing an epic" section to the epics page. It covers:
- the two requests;
- dependency order and parallel agents;
- what still stops for the user;
- worktrees and merging;
- what happens on a failure;
- that repeating a request resumes and skips completed work.

Link to it from the page's opening explanation, and add a site changelog entry.

*Technical detail:* [context.md#task-document-planning-and-implementing-an-epic-on-the-website](./context.md#task-document-planning-and-implementing-an-epic-on-the-website)

**Content outline**:

1. **Planning and implementing an epic** (section heading). Lead paragraph: "Once an epic's specs are written, two requests take it the rest of the way. Ask your agent to *plan this epic*, review the plans, then ask it to *implement this epic*."
2. **Plan this epic.**
   - Ready specs are planned side by side by separate agents.
   - A spec waits for the specs it depends on to be planned first, so its plan can build on theirs.
   - Specs that already have a plan are skipped.
   - Ends with a summary of every plan produced, for review.
3. **Implement this epic.**
   - Refuses up front if any spec has no plan, naming it.
   - Each ready spec is built in its own git worktrees, one for each repo it changes, on a branch named `spek/<spec>` in each.
   - A spec is merged back before anything that depends on it starts. A conflict stops the run and is shown to you.
   - A spec's changes are merged into every repo together, or not at all if any repo conflicts.
4. **What still stops for you.** Genuine open questions only (example: "planning `000071_a` needs to know whether the export keeps the old field"). Other specs keep going while you answer. The end-of-planning review.
5. **When something fails.** Nothing new starts, running work finishes, and you are told which spec failed, why, and what completed.
6. **Picking up where you left off.** Make the same request again. Finished specs are skipped and an interrupted spec resumes at the step it reached. Include an example progress line: "Planning 2 of 5 specs: 000071_a (tasks), 000073_c (discovery). 2 remaining."
7. **Seeing where an epic stands.** A fenced example of `status` for an example epic, with a trimmed JSON result showing `run` values.

**Acceptance criteria**:
- [x] The epics page has a section describing planning and implementing an epic, including that a repeated request resumes and that completed work is skipped.
- [x] The section follows the site conventions (no layout markup in the page body, alternating section shading, no em dashes), and the site builds and type-checks cleanly.

#### - [x] Task: Point the README at epic planning and implementation
**Id:** 8b83f742-73d2-4f6b-acfd-7e8ace473672
**Repo:** spektacular
**Depends on:**
- 153eff91-88c7-43b3-8ecb-23cb093be6a0 — Install the epic skills and teach the per-spec skills about orchestration
**Execution:** agent

Add a short paragraph to the README's How It Works section. It introduces "plan this epic" and "implement this epic" and says a repeated request resumes. It links to the website's epics documentation.

*Technical detail:* [context.md#task-point-the-readme-at-epic-planning-and-implementation](./context.md#task-point-the-readme-at-epic-planning-and-implementation)

**Content example**:

> **Epics.** When a piece of work is split into an epic, ask your agent to *plan this epic*, review the plans, then *implement this epic*. Independent specs are worked on in parallel, each spec is built in its own worktree and merged before the specs that depend on it, and repeating either request picks up where it stopped. See [Epics](https://spektacular.dev/epics/).

**Acceptance criteria**:
- [x] The README tells a reader that an epic can be planned and implemented with one request each, and that repeating a request resumes.

## Open Questions

- **Does every supported agent (Claude Code, Bob, Codex) let an orchestrator continue a subagent that stopped with a question?**
  - **Depends on:** each agent's subagent tooling at the time of implementation.
  - **What to do:** confirm it while writing the epic skills.
    - Where an agent cannot continue a stopped child, write the skill's fallback: start a fresh child on the same spec with the user's answer included. The child resumes its lane through the name-routed `goto`.
    - If an agent has no subagent capability at all, STOP and ask the user whether that agent should run the epic's specs one at a time instead.
- **Does `git commit --only -- <paths>` behave correctly when one of the paths is a directory deleted in the same commit, such as the lane files removed at `finished`, on the git versions in use?**
  - **Depends on:** git's pathspec handling for removed paths.
  - **What to do:** the scoped-commit tests exercise this case. If it fails, stage the deletions with `git rm --cached` before committing. Do not fall back to `git add -A`.

## Out of Scope

- **Specifying an epic in one request ("spec this epic").** This feature starts once the epic's specs exist. (Spec non-goal.)
- **One request that plans and then implements an epic.** Planning and implementing stay as two requests, with a review in between. (Spec non-goal.)
- **Planning or implementing a chosen subset of an epic's specs.** A request always covers every outstanding spec. (Spec non-goal.)
- **Detecting or refreshing stale plans after a spec changes.** An existing plan counts as planned, as it does today. (Spec non-goal.)
- **Parallel spec workflows, and parallel standalone runs.** Only plan and implement workflows started by an epic orchestrator get their own lanes. Ordinary runs keep the single shared workflow record.
- **Splitting other-repo work (such as docs) into its own spec during an epic split.** The epics design keeps code and its docs in one spec. With a worktree per touched repo, that does not limit parallel implementation, so the split rule is unchanged.
- **Resolving merge conflicts automatically.** A conflict stops the run and is shown to the user. Spektacular never resolves one.
- **A limit on how many specs run at once.** Every ready spec is started. A concurrency limit setting can follow if real runs need one.
- **Changing what an epic is, or the dependency check run when a spec is implemented on its own.** The epic format, its validation and the single-spec dependency check are unchanged, as the `epics-and-seeded-specs.md` design settles.
- **Fixing `skill spawn-planning-agents`, which tells agents to read the plan and spec directories directly.** This breaks the store-access rule but predates this feature and is unrelated to it. Tracked in [#70](https://github.com/hivecommons/spektacular/issues/70).
- **Updating this repository's own installed skills and configuration.** Per the "plans never change the active install" convention, the new skills reach this repo only through a normal install and migrate between workflows.

## Changelog

### 2026-10-04 — Task: Restore a compiling build

**What was done**: Removed the second, unindented copy of `installerFor` from `cmd/migrate.go` (introduced by commit 1188110), so the `cmd` package compiles again. The full suite passes under `-shuffle=on`.

**Deviations**: None.

**Files changed**:
- `spektacular: cmd/migrate.go`

**Discoveries**: `go list ./...` (and so `go test ./...`) fails on a permission-denied `tests/harbor/jobs/...` directory. Run the suite as `go test -shuffle=on . ./cmd/... ./internal/... ./templates/...`; `dagger/` is a separate module.

### 2026-10-04 — Task: Run orchestrated plan and implement workflows in their own lanes

**What was done**: `plan new` and `implement new` accept `"orchestrated": true`. Such a run keeps its state in `.spektacular/workflows/<kind>-<name>.json`, probes only that lane for a resume, skips the uncommitted-changes gate and records `orchestrated` in workflow data. Both `goto` commands accept `name`, strip it before data is copied, and route to the lane, or to `state.json` when it holds that name. Unknown or mismatched names are refused with `workflow_not_found`. A standalone `new` for a spec whose lane is in progress is refused with `workflow_in_progress`, naming the lane.

**Deviations**: The lane path helpers went straight into `internal/workflow/lane.go` (`LaneStatePath`, `LaneNotesPath`, `LaneStateRel`, `LaneNotesRel`, `ReadLane`, `LaneNames`) instead of starting in `cmd` and moving there in a later task. A named goto against a shared record of another kind falls through to the existing cross-kind report rather than `workflow_not_found`. A standalone `new` is refused while a lane is in progress even with `--force`; the next action offers `new --force` with `orchestrated` to discard the lane.

**Files changed**:
- `spektacular: internal/workflow/lane.go`
- `spektacular: internal/workflow/lane_test.go`
- `spektacular: cmd/workflow_slot.go`
- `spektacular: cmd/workflow_lane_test.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/implement.go`

**Discoveries**: Until the next two tasks land, instructions rendered inside a lane (step `goto` lines, the resume report and the working-context footer) still name the shared slot. An agent following them would drive `state.json`, so lanes are not usable end to end before then.

### 2026-10-04 — Task: Name the spec in every plan and implement instruction and keep scratch files per spec

**What was done**: Every `goto` in the plan and implement step templates now carries `"name":"{{plan_name}}"`. So do the resume and cross-kind resume reports for plan and implement, the commit-message partial, the engine's `invalid_transition` hints and the implement `changelog_missing` hint. Plan and implement scratch files moved under `.spektacular/tmp/<plan_name>/`, and the staged commit message moved to `.spektacular/tmp/<name>/git-commit-message.md` through a new shared `autocommit.MessageTmpPath`. Contract tests require the name in every rendered plan and implement goto, and per-spec scratch paths.

**Deviations**: The per-name commit-message path applies to every kind, spec included, for one shape; only plan and implement gotos carry the name. The resume templates also became lane-aware here: an orchestrated report names the lane's notes file and offers `new --force` with `orchestrated`. `resumeInstruction` gained an `orchestrated` parameter.

**Files changed**:
- `spektacular: templates/steps/plan/*.md (01-19)`
- `spektacular: templates/steps/implement/*.md (01-11)`
- `spektacular: templates/steps/resume.md`
- `spektacular: templates/steps/resume_implement.md`
- `spektacular: templates/steps/resume_mismatch.md`
- `spektacular: templates/partials/git-commit-message.md`
- `spektacular: internal/autocommit/message.go`
- `spektacular: internal/stepkit/stepkit.go`
- `spektacular: internal/workflow/workflow.go`
- `spektacular: internal/steps/implement/steps.go`
- `spektacular: cmd/autocommit.go`
- `spektacular: cmd/resume.go`
- `spektacular: cmd/instruction_contract_test.go`, `cmd/autocommit_test.go`, `cmd/milestones_test.go`, `cmd/cross_kind_test.go`, `cmd/resume_test.go`, `cmd/implement_task_run_test.go`, `cmd/implement_test.go`
- `spektacular: internal/autocommit/message_test.go`, `internal/workflow/workflow_test.go`, `internal/steps/plan/steps_test.go`, `internal/steps/implement/steps_test.go`

**Discoveries**: The contract harness renders the write steps (15-17, 19) through `WriteStepResult` with no store, so it only ever sees their "already committed" branch. Per-spec staging for those steps is pinned by unit tests in `internal/steps/plan` instead.

### 2026-10-04 — Task: Let an orchestrated run hand back instead of asking

**What was done**: `stepkit` now exposes `orchestrated` and `working_context_path` to every step template, and the footer names the lane's own notes file for an orchestrated run. Every step of an orchestrated workflow gets a "Running under an orchestrator" section. It says to hand each STOP back as `QUESTION: <spec>`, report an unrecoverable failure as `FAILED: <spec>`, and never ask between tasks or offer knowledge capture. The orchestrated walkthrough reads the plan back, prepares a summary and advances to `finished` with no sign-off. The plan's finished step then hands back `DONE: <spec>` with that summary. An orchestrated implement loops between tasks without asking, lists discoveries for the orchestrator instead of offering them, and ends with `DONE: <spec>`. An orchestrated resume report resumes without asking. Standalone renders are byte-identical to before.

**Deviations**: The plan had a partial included at each STOP sentence. Instead, one orchestrated section is appended by `stepkit` to every orchestrated step, so no STOP can be missed. The `implement-plan-documents` partial falls back to `.spektacular/working-context.md` when no `working_context_path` is set, because it is also rendered into the installed `spek-implement` skill.

**Files changed**:
- `spektacular: internal/stepkit/stepkit.go`
- `spektacular: cmd/resume.go`
- `spektacular: templates/partials/orchestrated-stop.md`
- `spektacular: templates/partials/working-context-footer.md`
- `spektacular: templates/partials/implement-plan-documents.md`
- `spektacular: templates/steps/plan/13-assemble.md`
- `spektacular: templates/steps/plan/18-walkthrough.md`
- `spektacular: templates/steps/plan/19-finished.md`
- `spektacular: templates/steps/implement/07-update_changelog.md`
- `spektacular: templates/steps/implement/12-finished.md`
- `spektacular: templates/steps/resume.md`
- `spektacular: templates/steps/resume_implement.md`
- `spektacular: cmd/orchestrated_test.go`, `cmd/instruction_contract_test.go`
- `spektacular: internal/steps/plan/orchestrated_test.go`, `internal/steps/implement/orchestrated_test.go`

**Discoveries**: Mustache standalone section tags (`{{^x}}` on a line of their own) are stripped together with their newline. Wrapping existing prose can drop a blank line, so check standalone renders for byte identity after any such edit. Partials shared between step templates and install-time skills only get `{{command}}` at install time, so any new variable they use needs a fallback.

### 2026-10-04 — Task: Scope orchestrated plan commits to their own files

**What was done**:
- `autocommit.Git` gained `CommitPaths`. It stages only the given paths (additions, changes and deletions) and commits them with `--only`, leaving every other change in the tree as it was.
- A new project-level commit lock at `.spektacular/workflows/.commit.lock` serialises these commits. It waits up to 30s and breaks a lock older than 10 minutes.
- When an orchestrated plan finishes with auto-commit on, it commits only its own files: its plan store directory, `.spektacular/work/<name>/`, `.spektacular/tmp/<name>/` and its lane files.
- Every lane's files are removed when it reaches `finished` (before the commit, so a plan lane's deletion is part of it), and also when auto-commit is off.
- A failed commit restores only that lane's state and notes.
- Standalone and implement-lane commits are unchanged.

**Deviations**:
- `CommitPaths` keeps a path that is on disk, in the index, or in HEAD.
  - The HEAD case covers a retry after a rejected commit, whose lane deletion is already staged.
  - Paths known only to HEAD go to the commit but not to `git add`, which refuses them.
- The plan's open question is answered: `git commit --only` with a fully deleted tracked directory works on the installed git.
- Known minor issue: `AcquireLock` can race when two waiters both find the same stale lock. One may remove the other's fresh lock. This needs a lock already abandoned for 10 minutes and is not handled.

**Files changed**:
- `spektacular: internal/autocommit/git.go`
- `spektacular: internal/autocommit/lock.go`
- `spektacular: internal/autocommit/lock_test.go`
- `spektacular: internal/autocommit/git_integration_test.go`
- `spektacular: internal/autocommit/targets_test.go`
- `spektacular: cmd/autocommit.go`
- `spektacular: cmd/orchestrated_commit_test.go`
- `spektacular: cmd/orchestrated_test.go`

**Discoveries**:
- `git ls-files` reports the index, not HEAD, and `git add -A -- <path>` fails for a path that is in neither the work tree nor the index. So a path-scoped commit that retries after a hook rejection must look at HEAD to find deletions it already staged.

### 2026-10-04 — Task: Show orchestrated runs in status and the session log

**What was done**: `status.Options` gained a `Lane` reader, which `cmd/status.go` wires to `workflow.ReadLane`. A plan's current step now comes from the shared workflow when that matches, and otherwise from the spec's in-progress lane. When the shared workflow matches none of the reported specs, the report's `workflow` block falls back to the first in-progress plan or implement lane, marked `orchestrated: true`. The debug session log now snapshots the lane for an orchestrated `new`, or for a `goto` naming a spec that has a lane, so lane commands are filed under `<kind>:<name>`. A lane's finishing command, whose files are then removed, is filed under the lane too.

**Deviations**: The dependency-check call sites (`cmd/implement.go`, `cmd/epic_link.go`) leave `Lane` nil, since they never report a current step. When a spec has both a plan lane and an implement lane in progress, the `workflow` block shows the plan lane.

**Files changed**:
- `spektacular: internal/status/report.go`
- `spektacular: cmd/status.go`
- `spektacular: cmd/root.go`
- `spektacular: internal/status/status_test.go`
- `spektacular: cmd/status_test.go`
- `spektacular: cmd/sessionlog_lane_test.go`

**Discoveries**: The session log reads `os.Args`, not cobra's args, so routing it by `--data` needs its own argv scan that skips the values of value-taking flags (`--data`, `-d`, `--fields`, `--stdin`, `--file`).

### 2026-10-04 — Task: Order epic specs and find cycles

**What was done**: Added `depgraph.CycleMembers`, which uses Tarjan's strongly connected components to return every node on any cycle, self-loops included, in declared order. Added `depgraph.TopoOrder`, a stable Kahn ordering that always takes the earliest-declared ready node. Nodes that can never become ready, on a cycle or behind one, are appended in declared order. Dependencies on names outside the graph are ignored by both.

**Deviations**: None.

**Files changed**:
- `spektacular: internal/depgraph/depgraph.go`
- `spektacular: internal/depgraph/depgraph_test.go`

**Discoveries**: None.

### 2026-10-04 — Task: Give each spec its own worktree and merge it back

**What was done**: Added the `internal/worktree` manager and the `epic worktree` / `epic merge` commands.
- **Creating worktrees.** `epic worktree` gives a spec a worktree on `spek/<spec>` under `.spektacular/worktrees/<spec>/<repo>`, from HEAD:
  - one in the project's repo;
  - one in every registered repo its plan's tasks name, with repos sharing a checkout sharing one worktree.
- **Repeat runs.** Existing worktrees and branches are reused.
- **Keeping them out of git.** It adds the worktree folder and the overlay to the project repo's `info/exclude`.
- **Repo overlay.** It writes a repo overlay (`.spektacular/worktree-repos.json`) into the project worktree. `repo.New` applies the overlay, so from inside the worktree every repo, with its stores and auto-commit targets, resolves into the spec's worktrees.
- **Merging.** `epic merge` is all or nothing. It first refuses:
  - a merge already in progress;
  - a dirty spec worktree;
  - main-copy changes the merge would overwrite.

  It then dry-runs `git merge-tree` in every repo. On any conflict it merges nothing and returns `epic_merge_conflict` with paths per repo. Otherwise it merges each repo `--no-ff` and removes the worktrees and branches.
- New project templates ignore `worktrees/`, `worktree-repos.json` and the commit lock.

**Deviations**:
- Added `gitexec.RunCode` (stdout plus exit code), because `merge-tree` reports conflicts through exit code 1 and stdout, and `Run` drops stdout on failure. The plan said `gitexec` would not change.
- Every registered repo in the project's own work tree is mapped into the project worktree, touched or not.
- Not covered by tests:
  - a project nested below its repo's top level;
  - the `MERGE_HEAD` refusal;
  - rollback when a real merge fails after a clean dry run in a later repo. Earlier repos stay merged and the error names them.

**Files changed**:
- `spektacular: internal/worktree/worktree.go`
- `spektacular: internal/worktree/worktree_test.go`
- `spektacular: internal/gitexec/gitexec.go`
- `spektacular: internal/gitexec/gitexec_test.go`
- `spektacular: internal/repo/set.go`
- `spektacular: internal/repo/set_test.go`
- `spektacular: cmd/epic_worktree.go`
- `spektacular: cmd/epic_worktree_test.go`
- `spektacular: cmd/root_test.go`
- `spektacular: cmd/epic_test.go`
- `spektacular: templates/.spektacular/.gitignore`

**Discoveries**:
- `gitexec` trims all whitespace from output. That strips the leading space of the first `git status --porcelain` line, so column-based porcelain parsing silently misreads the first path. Use `--name-only` queries instead.

### 2026-10-04 — Task: Report what an epic still needs in status

**What was done**: `status <name>` now adds a `run` view, built in `internal/status/run.go` from an injectable `RunSource` (worktrees, worktree stores, touched repos, dirty check) that only `cmd/status.go` sets.

Each spec gets `run.plan` and `run.implement`:
- **States:** `done`, `in_progress` (with `current_step` and `root`), `awaiting_merge` (implementing only), `ready`, or `blocked` (with `waiting_on`). `run.implement` also lists the repos its plan touches.
- **Planned** means a final plan with no plan workflow live.
- **Implemented** means every task ticked, a final changelog, no implement workflow live and no worktree left.
- **Worktree specs:** a spec with a worktree reads its lane and finish state from the worktree's own store.

The epic gets `run`:
- **`order`:** the dependency order, with ties following the epic's list order.
- **Counts:** per part.
- **`dirty`:** whether any registered repo has uncommitted changes.
- **`problems`:** `epic_dependency_cycle`, `epic_dependency_outside` and `epic_unplanned`, each naming its specs and blocking only implementing.

Planning ignores dependencies outside the epic and dependencies between specs on a cycle. The readable output adds planning and implementing lines and any problems under the epic header. The schema is updated. Existing fields are unchanged, and the dependency check and completed-epic guard build without the run view.

**Deviations**:
- Outside dependencies count as met when their spec is implemented, the same rule the implement-time dependency check uses. Unlike members, they need no final changelog or merge.
- A spec whose tasks are all ticked but whose changelog is still a draft, with no live workflow or worktree, reports `ready`, not `done`.

**Files changed**:
- `spektacular: internal/status/run.go`
- `spektacular: internal/status/run_test.go`
- `spektacular: internal/status/report.go`
- `spektacular: internal/status/pretty.go`
- `spektacular: cmd/status.go`
- `spektacular: cmd/status_test.go`

**Discoveries**: None.

### 2026-10-04 — Task: Write the plan-this-epic skill

**What was done**: Added `templates/skills/workflows/spek-plan-epic/SKILL.md`. Its description triggers on "plan this epic", "plan the epic", "plan <epic name>" and "plan all the specs in this epic". The skill:
- finds the epic, then loops on `status <epic> --format json` (`run.plan`);
- starts a child running the standard `spek-plan` skill as an orchestrated run for every ready spec, resumes in-progress ones, skips done ones and lets blocked ones wait;
- defines the child prompt (spec, project root, the orchestrated `plan new`, the store-access rule), the `DONE:`/`QUESTION:`/`FAILED:` hand-back contract and what a genuine open question is;
- relays questions while other children keep going, and enters stopping mode on a failure or a declined question;
- reports progress after each change, and ends with an end-of-planning review applied through `plan file write` and a completed/skipped/outstanding report.

**Deviations**: A child resumes through the same orchestrated `plan new`, whose resume report names the lane's notes and the `goto` to run. Building the lane path by hand would sidestep the CLI. A child waiting on a question counts as running, so a spec never gets two children. The open question about subagent continuation is handled with the planned fallback: restart a child on the same spec with the answer, which resumes its lane.

**Files changed**:
- `spektacular: templates/skills/workflows/spek-plan-epic/SKILL.md`
- `spektacular: templates/plan_epic_skill_test.go`

**Discoveries**: None.

### 2026-10-04 — Task: Write the implement-this-epic skill

**What was done**: Added `templates/skills/workflows/spek-implement-epic/SKILL.md`. Its description triggers on "implement this epic", "build the epic" and "implement <epic name>". The skill:
- refuses up front, implementing nothing, when `status` reports a problem that blocks implementing, and relays each problem;
- asks once about uncommitted work when `dirty`;
- loops on `run.implement`. For each ready spec it creates the worktrees with `epic worktree` and starts a child running the standard `spek-implement` skill as an orchestrated run in the returned project worktree. It resumes in-progress specs in their worktree and merges `awaiting_merge` and finished specs with `epic merge` before their dependents start;
- on `epic_merge_conflict`, or any other worktree or merge refusal, shows it to the user and stops;
- shares the planning skill's hand-back contract, question relay, stopping mode and progress line;
- ends with a completed/skipped/outstanding report that also lists any worktrees left behind.

**Deviations**: Also says what happens when the user declines to commit uncommitted work: the run goes ahead, and that work is not in the worktrees.

**Files changed**:
- `spektacular: templates/skills/workflows/spek-implement-epic/SKILL.md`
- `spektacular: templates/implement_epic_skill_test.go`

**Discoveries**: None.

### 2026-10-04 — Task: Install the epic skills and teach the per-spec skills about orchestration

**What was done**: Registered `spek-plan-epic` and `spek-implement-epic` in `workflowSkills` and `workflowDescriptions`, so `init` and `migrate` install them for Claude, Bob (with command wrappers) and Codex. Appended a "When an orchestrator starts this skill" section to the `spek-plan` and `spek-implement` skills. It covers starting with `"orchestrated":true`, the `name` on every goto, handing questions back as `QUESTION:`, and ending with `DONE:` or `FAILED:`. For the plan skill it adds that the walkthrough prepares a summary instead of asking for sign-off; for the implement skill, that tasks run without asking between them. Tests that pin the skill list now expect eight skills, and new tests check installation for every agent, the new sections, and the epic skills' place in the contract corpus.

**Deviations**: The orchestrator sections go at the end of each skill, so the existing pinned text is untouched.

**Files changed**:
- `spektacular: internal/agent/skills.go`
- `spektacular: internal/agent/commands.go`
- `spektacular: templates/skills/workflows/spek-plan/SKILL.md`
- `spektacular: templates/skills/workflows/spek-implement/SKILL.md`
- `spektacular: internal/agent/agent_test.go`, `internal/agent/claude_test.go`, `internal/agent/bob_test.go`, `internal/agent/codex_test.go`
- `spektacular: cmd/init_test.go`, `cmd/migrate_test.go`, `cmd/instruction_contract_test.go`
- `spektacular: templates/orchestrated_skill_section_test.go`

**Discoveries**: None.

### 2026-10-04 — Task: Document planning and implementing an epic on the website

**What was done**: The website's epics page gains a "Planning and implementing an epic" section, placed between "Dependencies between specs" and "Working with epics from the command line". It follows the plan's content outline:
- the two requests;
- planning order, parallel agents and skipping;
- implementing in per-repo worktrees, with all-or-nothing merging before dependents start and refusal of unplanned or broken epics;
- what still stops for the user;
- what happens on a failure;
- resuming by repeating the request, with an example progress line;
- a trimmed `status --format json` example of the `run` view.

The command-line section flips to `surface`, so the bands still alternate. The page's opening explanation points readers to the new section, and its description mentions it. The site changelog has an entry for 000062.

**Deviations**: The opening explanation names the section in plain text instead of linking to an anchor, because `Section` renders no heading ids and adding them would change the site beyond content. The section uses "spek", following the page's own wording. The optional cross-link from `how-it-works.mdx` was not added.

**Files changed**:
- `docs: src/pages/epics.mdx`
- `docs: CHANGELOG.md`

**Discoveries**: `Section.astro` gives section headings no `id`, so in-page anchor links to a section do not resolve.

### 2026-10-04 — Task: Point the README at epic planning and implementation

**What was done**: Added an "Epics" paragraph to the README's How It Works section, after the `status` reference. It introduces "plan this epic" and "implement this epic", says independent specs run in parallel in their own worktrees and merge before their dependents, says a repeated request resumes, mentions the `run` view `status` adds for an epic, and links to the website's Epics page.

**Deviations**: Added one sentence on the `status` run view, because this README section documents the `status` report.

**Files changed**:
- `spektacular: README.md`

**Discoveries**: None.
