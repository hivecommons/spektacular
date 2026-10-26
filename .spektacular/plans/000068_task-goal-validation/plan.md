---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Plan: 000068_task-goal-validation

<!-- Metadata -->
<!-- Created: 2026-10-21T09:57:39Z -->
<!-- Commit: b1734e5 -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

This plan turns each plan task's acceptance criteria and scope into a goal. A separate, inexpensive, read-only reviewer agent checks that goal with a fresh context after verification and before anything is ticked. The reviewer judges every criterion as met, not yet met or impossible, with evidence. Only met criteria are ticked, and shortfalls get up to two retries before the run stops to ask the user. Every outcome is recorded so it can be inspected afterwards.

It closes the gap where the agent that wrote the code also certified it from passing tests. It works the same under Claude, Codex, Copilot, omp and Bob. Teams can trust a ticked criterion, and can switch the check off or choose the reviewer's model per agent.

## Conventions

- **Error messages must describe the problem and suggest remediation** — `implement goal record`/`decide`/`show`, the `update_plan` guard and the `epic merge` `goal_check_open` refusal all return `output.NewError(...).WithNextAction(...)` naming the exact command to run.
- **Store files are written through the CLI, never with file tools** — the outcome record is written only by `implement goal record`/`decide`; verdict replies are supplied with `--from` a file staged under `.spektacular/tmp/`; the design update goes through `design author`; no template uses stdin or heredoc bodies (pinned by `internal/agent/instruction_surface_test.go`).
- **A plan never changes the active install** — step, skill and sub-agent templates change only under `templates/`; this repo's `.claude/`, `.spektacular/config.yaml` and installed skills are never regenerated; verification uses the Go test suite and throwaway projects.
- **Tests must not depend on order** — new `cmd` tests for the `implement goal` verbs and the merge guard run through `runRootCmd`/`resetRootCmd`; outcome tests avoid wall-clock comparisons.
- **Passing tests are required before calling work done** — every task in the `spektacular` repo ends with the full Go test suite green, including the hard-coded step walks this plan updates.
- **Docs: MDX authoring conventions** — new docs content goes in existing `ConfigKey`/`Prose`/`PipelineStage` slots with fenced code blocks; no layout HTML in page bodies.
- **Docs: no em dashes** — all new docs prose and the docs `CHANGELOG.md` entry avoid em dashes.
- **Docs: plans must sketch content structure** — the docs task carries a content example for each page change.
- **Docs: site layout conventions** — compose from existing section components; no new widths or heading sizes.
- **Docs: alternate section background shading** — considered: new content goes inside existing sections, so no band is added and shading is unchanged.
- **Docs: label before filename in file-scoped headings** — considered: no new file-scoped section heading is added, so it does not bite.

## Architecture & Design Decisions

The check is a new implement step, `check_goal`, between `verify` and `update_plan`, built on the task shape settled in the referenced design `design:plan-task-graph.md`: a task's goal is its `**Acceptance criteria**:` checkboxes plus its scope (its one `**Repo:**` and the files its `context.md` section names). The CLI owns the goal, the verdicts and the retry bound; the agents only do the judging. Concretely:

- **The CLI builds the goal.** The plan reader learns each criterion's text and, for plans written before tasks, each `Phase N.M` block's criteria, so the same goal is built for a task or a legacy phase. The `check_goal` step renders the current item's goal (numbered criteria and scope limits, nothing from any other task) into its instruction. The goal concept never touches a coding agent's native goal feature or a hook (spec constraint).
- **The reviewer is a named, read-only helper.** `spektacular init` installs `spek-goal-reviewer` in each agent's native sub-agent format, with a cheaper default model and read-only tools where the agent allows it: Claude `.claude/agents/` (`haiku`; `Read`, `Grep`, `Glob`), Codex `.codex/agents/` (a mini model; an explicit do-not-edit instruction), Copilot `.github/agents/` (a cheap `model:`, read and search tools), omp `.omp/agents/` (`@smol`). Bob has no per-helper model setting, so the step tells Bob to use its built-in read-only `explore` helper. Any agent without a named helper falls back to a generic fresh-context sub-agent given the same brief, which `spektacular skill review-task-goal` serves. Every definition embeds one shared brief, so all agents judge to the same rules.
- **Fresh context, reads the work itself.** The implementing agent stages each touched repo's diff and its untracked-file list under `.spektacular/tmp/<plan>/goal-check/`, then hands the reviewer only three things: the goal block exactly as rendered, the verify step's pass/fail summary, and pointers (code roots, staged diff paths, the task's files). Nothing from its own conversation goes in. The reviewer opens the files itself and returns one verdict per criterion (`met`, `not_met`, `impossible`) with non-empty evidence.
- **One CLI verb records and decides.** The agent writes the reviewer's JSON reply unchanged to a staged file and passes it to `implement goal record` together with the plan and item. The CLI:
  - refuses a reply that does not hold exactly one valid verdict with evidence for every criterion;
  - appends the attempt, with a snapshot of the goal, to the plan's outcome record;
  - returns the decision and the exact `goto` to run next.

**The decision rule lives in Go, not prose:**

- All `met`: accept and go to `update_plan`.
- Any `impossible`: stop at once and ask how to proceed.
- Any `not_met` with retries left (at most two per round): return to `implement`, whose instruction then carries the reviewer's reasons.
- Otherwise: stop and ask to keep fixing, accept the shortfall, or skip.

The user's answer is recorded with `spektacular implement goal decide`, which returns the next `goto`. `keep_fixing` starts a new round of retries; `recheck` re-runs the check after the plan's criterion text has been corrected. The FSM gains `check_goal` (from `verify`) and two loop-back sources: `implement` from `check_goal`, and `update_plan` from `check_goal` alongside `verify`. `verify` names `check_goal` as its next step only when `implement.goal_check.enabled` is on (default on). With it off, `verify` goes straight to `update_plan` and every template renders exactly as today, ticking from verification (spec constraint). With it on, `update_plan` refuses, with a runnable `next_action`, until the current item's latest round is settled (accepted, accepted shortfall, or skipped). It then renders the exact criteria judged met as the only boxes to tick. Nothing is ticked before the check has run, and only `met` criteria are ever ticked. A skipped item ticks nothing, and in a whole-plan run the current-item rule and the remaining-task count pass over items the record marks skipped.

**Where the outcome lives.** The outcome record is a JSON file in the plan's own folder in the plan store, `goals.json`, holding every check of every task: goal, verdicts, attempt and round, and decisions. It is written only by `implement goal record`/`decide` and read back with `implement goal show`, readable or as JSON (convention: store files are written through the CLI). Lane data was rejected because lanes are deleted at `finished`. Every record and decide call also lands in the session log with its `--data`, which gives harbor an agent-independent trace.

**Stops in orchestrated runs.** An orchestrated child's stops at `check_goal` become `QUESTION:` hand-backs through the existing orchestrated partial, with no new prose. The `spek-implement-epic` skill adds both stops to its list of genuine questions. `epic merge` gains a hard refusal (`goal_check_open`) while the spec's implement lane stands at `check_goal`, so a spec with an open goal-check question can never be merged.

**Configuration.** It is `implement.goal_check.enabled` and `implement.goal_check.models.<agent>`. These are new sub-keys parsed over the defaults, so no settings-format bump and no migration step are needed, and older configurations get the check switched on. A model override is rendered into that agent's installed reviewer by `init`, and by `migrate` on a version change; other agents keep their defaults. User-level agent settings that pin a helper's model still win, and Copilot's `Auto` session model ignores a helper's model; the docs state both rather than working around them.

**Repos.**

- `spektacular` (`/home/nicj/code/github.com/hivecommons/spektacular`) gets:
  - the plan reader, goal package, config, CLI verbs, FSM step, templates, skills, agent installs and `epic merge` guard;
  - the harbor `implement-workflow` and `implement-epic-workflow` surfaces, plus a new goal-check harbor suite (spec constraint; knowledge: testing architecture);
  - the update to the referenced design's tick rule, which becomes "ticked only when judged met", done through `spektacular design author` as the spec requires.
- `docs` (`/home/nicj/code/github.com/hivecommons/spektacular-website`) gets the workflow and configuration reference pages.

**Self-hosting and conventions.** Templates change only under `templates/`. This repo's installed skills and `.spektacular/config.yaml` are never regenerated, and everything is verified with the Go test suite and throwaway projects (convention: a plan never changes the active install). Every new refusal carries a runnable `next_action` (convention: error messages must suggest remediation).

**Why this direction.** Folding the judgement into `verify` was rejected because that sub-agent needs a shell. Agent-written verdict documents would leave the N-verdicts rule and the retry bound unenforced. Runtime rewriting of helper definitions conflicts with how agents load helpers. See `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Plan reader** (changed, `spektacular`). It already parses tasks, their criteria counts and legacy phase headings. Now it also keeps each criterion's text, in order, and parses legacy `Phase N.M` blocks into items with their own criteria. It answers "which item is the goal for" without changing the JSON that `status` and `plan export` produce. The goal package is its only new consumer; the task-file reader it already offers supplies each item's files.
- **Goal package** (new, `spektacular`). This is the single owner of the goal concept:
  - It builds an item's goal (key, title, numbered criteria, repo and files) from the plan and `context.md`.
  - It resolves the current item: the run's selected task, or else the first open item not marked skipped.
  - It validates a reviewer's reply (exactly one verdict with evidence per criterion).
  - It applies the decision rule (accept, retry, stop for exhausted, stop for impossible) and the round and attempt bookkeeping.
  - It reads and appends the outcome record through the store.

  The CLI verbs and the implement step callbacks both call it, so the two can never disagree.
- **`implement goal` commands** (new, `spektacular`). These are three verbs beside `implement new/goto/steps`:
  - `record` stores one check from a staged reply and answers with the decision and next `goto`.
  - `decide` stores the user's answer after a stop and answers with the next `goto`.
  - `show` reads the outcome back, pretty or JSON.

  All three are thin wrappers over the goal package, using the shared error and next-action plumbing.
- **Implement workflow steps** (changed, `spektacular`).
  - The FSM gains `check_goal` and the loop-backs `check_goal → implement` and `check_goal → update_plan`.
  - The `verify` callback names `check_goal` as its next step only when the check is enabled.
  - `check_goal` renders the current item's goal, the reviewer launch rules and the record command.
  - `implement` renders the reviewer's reasons during a retry.
  - `update_plan` refuses until the item's round is settled, then renders exactly which criteria to tick, or nothing for a skip.

  It reads the switch from a new runtime flag on the workflow config, which the command layer sets from project configuration.
- **Implement step templates and partials** (changed, `spektacular`).
  - A new `check_goal` template covers staging the diff, the reviewer input contract, launching the named helper (Bob's `explore`, or a generic fresh-context fallback), recording, and the two stops with their options.
  - `implement` gains a retry block; `update_plan` gains a goal-check branch.
  - The current-item partial and the plan-changelog loop pass over skipped items, and the plan-changelog entry records a skip or an accepted shortfall as a deviation.

  With the check off, every template renders as today.
- **Reviewer brief and skill** (new, `spektacular`). This is one shared brief stating:
  - the read-only rule;
  - the input it receives;
  - how to judge each criterion from the files, never from the implementer's claims;
  - the three verdicts, the evidence rule, and the JSON reply shape.

  It is served by `spektacular skill review-task-goal` for the generic fallback and embedded in every installed reviewer definition.
- **Agent installers** (changed, `spektacular`). Each agent's install writes a `spek-goal-reviewer` definition in its native format, rendered from the shared brief with that agent's default cheap model or the project's override, and read-only tools where supported:
  - Claude, Codex, Copilot and omp get a definition.
  - Bob installs none, and its step text points at its built-in `explore` helper.
  - `init` and `migrate` already call each install, so both refresh the definition.
- **Project configuration** (changed, `spektacular`). The `implement` section gains a `goal_check` subsection with an on/off switch, default on, and a per-agent model map, default empty. It is validated like the other keys, with a runnable fix in every error. Its consumers are the agent installers (model) and the implement command layer (switch).
- **`epic merge`** (changed, `spektacular`). It refuses while the spec's implement lane stands at `check_goal`, naming the open question as the reason and the resume path as the fix.
- **`spek-implement-epic` and `spek-implement` skills** (changed, `spektacular`).
  - The epic skill's genuine-question list and child prompt name the goal-check stops: attempts exhausted, or a criterion judged impossible.
  - The implement skill's step overview mentions the check.
- **Referenced design `design:plan-task-graph.md`** (changed, through `spektacular design author`). The acceptance-criteria rule becomes "ticked by implement only when the goal check judged the criterion met (from verification when the check is switched off)", as the spec's constraint requires.
- **Harbor suites** (changed and new, `spektacular`).
  - The `implement-workflow` and `implement-epic-workflow` surfaces gain the new step. In implement-epic this covers the solution walk and a verdict assertion for every task.
  - A new goal-check suite seeds a task whose criterion is subtly unmet while its tests pass, and one whose criteria are all met. It is rendered for Claude, Codex and Bob, and its verifier reads `implement goal show` and the session log. The reviewer's launch and model are checked where a transcript can show them.
  - Bob runs rely on the Bob harbor agent being in the shared location the Makefile expects. Sibling spec 000069_compact-agents-md moves it in its first milestone (epic decision); this plan does not move it.
- **Documentation site** (changed, `docs`).
  - The how-it-works implement step list and the Implement stage describe the check and its three verdicts.
  - The plan-tasks acceptance-criteria rule changes to "ticked only when judged met".
  - The configuration reference documents `implement.goal_check` and each agent's reviewer model and limitations.
  - The docs `CHANGELOG.md` gains an entry.

## Data Structures & Interfaces

**Plan reader additions (`plantask`).** Criterion text and legacy phase items join the parsed plan. Neither is serialised into `status` or `plan export` output, which therefore stay byte-identical.

```go
type Criterion struct {
    Text string // the line after "- [ ] " / "- [x] ", trimmed
    Met  bool   // its checkbox
}

// Criteria keeps its JSON shape {met,total}; Items is new and not serialised.
type Criteria struct {
    Met   int         `json:"met"`
    Total int         `json:"total"`
    Items []Criterion `json:"-"`
}

// Phase is one "#### - [ ] Phase N.M: <title>" block of a plan written before tasks.
type Phase struct {
    Label     string   // "Phase N.M"
    Title     string
    Completed bool
    Criteria  Criteria
}

// on Plan
Phases []Phase // in plan order; empty for task-format plans
```

**Goal and outcome (`internal/goal`, new).** An item is a task, keyed by its `**Id:**`, or a legacy phase, keyed by its `Phase N.M` label.

```go
type Goal struct {
    Item     string      `json:"item"`     // task id, or "Phase N.M"
    Title    string      `json:"title"`
    Criteria []string    `json:"criteria"` // in plan order; verdict i judges Criteria[i]
    Scope    Scope       `json:"scope"`
}
type Scope struct {
    Repo  string             `json:"repo"`            // the task's **Repo:**, or the only registered repo
    Files []plantask.FileRef `json:"files"`           // from the item's context.md section; may be empty
    Rule  string             `json:"rule"`            // "change only this item's files; nothing belonging to another task"
}

type Verdict struct {
    Criterion int    `json:"criterion"` // 1-based index into Goal.Criteria
    Verdict   string `json:"verdict"`   // "met" | "not_met" | "impossible"
    Evidence  string `json:"evidence"`  // non-empty: where it looked, or why
}

type Check struct {
    At       string    `json:"at"`       // RFC3339 UTC, stamped by the CLI
    Round    int       `json:"round"`    // 1-based; a keep_fixing or recheck decision opens a new round
    Attempt  int       `json:"attempt"`  // 1-based within the round; at most 1 + MaxRetries
    Goal     Goal      `json:"goal"`     // snapshot at check time
    Verdicts []Verdict `json:"verdicts"` // exactly len(Goal.Criteria)
    Decision string    `json:"decision"` // "accept" | "retry" | "exhausted" | "impossible"
    TreeUnchanged bool `json:"tree_unchanged"` // the code roots matched the diff/status staged just before the reviewer ran
}

type Decision struct {
    At       string `json:"at"`
    Round    int    `json:"round"`
    Decision string `json:"decision"` // "keep_fixing" | "accept" | "skip" | "recheck"
    Reason   string `json:"reason,omitempty"`
}

type ItemOutcome struct {
    Item      string     `json:"item"`
    Title     string     `json:"title"`
    Checks    []Check    `json:"checks"`
    Decisions []Decision `json:"decisions"`
    Attempts  int        `json:"attempts"` // total checks across rounds (derived on read)
    State     string     `json:"state"`    // "pending" | "retrying" | "awaiting_decision" | "accepted" | "accepted_shortfall" | "skipped"
}

type Outcome struct {
    Plan  string        `json:"plan"`
    Items []ItemOutcome `json:"items"` // in first-checked order
}

const MaxRetries = 2
```

The outcome is persisted as `goals.json` in the plan's folder of the plan store and written only through `internal/goal`. Each item's `State` is derived from its latest check and decision, which makes settlement a pure function: `accepted`, `accepted_shortfall` and `skipped` settle an item; anything else blocks `update_plan`.

**CLI contracts (new).**

```console
spektacular implement goal record --data '{"name":"<plan>","item":"<task id | Phase N.M>"}' --from <staged reply.json>
spektacular implement goal decide --data '{"name":"<plan>","item":"<…>","decision":"keep_fixing|accept|skip|recheck","reason":"<optional>"}'
spektacular implement goal show <plan> [--item <…>] [--format pretty|json]
```

The staged reply is the reviewer's verdict list exactly as returned: `{"verdicts":[{"criterion":1,"verdict":"met","evidence":"…"}, …]}`.

`record` result:

```json
{
  "plan": "<plan>",
  "item": "<…>",
  "round": 1,
  "attempt": 2,
  "decision": "retry",
  "retries_left": 0,
  "met": [1, 3],
  "not_met": [2],
  "impossible": [],
  "next_step": "implement",
  "next_action": "spektacular implement goto --data '{\"step\":\"implement\",\"name\":\"<plan>\"}'"
}
```

`next_step` is `update_plan` for `accept`, `implement` for `retry`, and `check_goal` for `exhausted`/`impossible`, where the agent stays to ask the user. `decide` returns the same shape with `next_step` of `implement` (keep_fixing), `update_plan` (accept, skip) or `check_goal` (recheck).

Refusals, all `output.NewError(...).WithNextAction(...)`:

- `goal_check_disabled`: the check is switched off.
- `goal_item_not_found`: no such task or phase in the plan.
- `goal_item_not_current`: not the run's current item.
- `goal_reply_invalid`: the staged reply is not valid JSON, has a missing or duplicate criterion, an unknown verdict, or empty evidence; the message names which.
- `goal_decision_not_due`: a `decide` when the item is not awaiting a decision.
- `goal_decision_invalid`: an unknown decision value.

`show` returns the `Outcome` JSON, or a pretty per-item listing of the goal, each check's verdicts with evidence, the attempt count and the decisions.

**Workflow config (changed).** `workflow.Config` gains `GoalCheck bool`, a runtime-only value set from `implement.goal_check.enabled` by the implement command layer, the same way `CodeRoots` is.

**Project configuration (changed).**

```yaml
implement:
    worktrees: true
    goal_check:
        enabled: true          # default; absent in older configs means on
        models:                # optional, per agent; absent agents keep their default
            claude: sonnet
            codex: gpt-5.4-mini  # example value only
```

```go
type GoalCheckConfig struct {
    Enabled bool              `yaml:"enabled"`
    Models  map[string]string `yaml:"models,omitempty"`
}
// ImplementConfig.GoalCheck GoalCheckConfig `yaml:"goal_check"`
```

**Installed reviewer definition (new file per agent).** The definition is named `spek-goal-reviewer`, and its body is the shared brief:

| Agent | Path | Model default | Tools |
|---|---|---|---|
| claude | `.claude/agents/spek-goal-reviewer.md` | `haiku` | `Read, Grep, Glob` |
| codex | `.codex/agents/spek-goal-reviewer.toml` | the Codex mini model | instruction-only read-only (plus `sandbox_mode = "read-only"` if supported) |
| copilot | `.github/agents/spek-goal-reviewer.agent.md` | a cheap Copilot model | `read`, `search` |
| omp | `.omp/agents/spek-goal-reviewer.md` | `@smol` | read-only tool set |
| bob | none; uses the built-in `explore` helper | Bob's lighter model | read-only by design |

**Reviewer input and reply (contract between the implementing agent and the reviewer).** The input is:

- the goal block, verbatim from the `check_goal` instruction;
- the verify step's pass/fail summary;
- the code roots, staged diff paths and the item's files.

No conversation is included. The reply is the JSON verdict list above and nothing else.

**`epic merge` (changed).** It adds the refusal `goal_check_open` (resource: the spec), returned when the spec's implement lane's `current_step` is `check_goal`.

## Implementation Detail

**A CLI-owned judgement loop inside a linear FSM.** The implement FSM stays linear, with named loop-backs, as it already does for `analyze`. The new pattern is that the loop's bound and branch are decided by a CLI verb (`implement goal record`/`decide`) rather than by template prose. The step template tells the agent to run the verb and then the `goto` it returns. The FSM still guards which transitions are legal (`check_goal → implement | update_plan`), and the `update_plan` callback refuses to render until the goal package says the item is settled. A developer reading the code finds the rule in one place: the goal package's decision function. That function is a pure function of the recorded checks and decisions, so it is unit-tested without a workflow.

**One goal builder, two callers.** The goal is built from the parsed plan and `context.md` by the goal package. The `check_goal` callback renders it into the instruction, and `record` rebuilds it to validate the reply and snapshot it. Because both use the same builder, the recorded goal always matches what the reviewer was shown, as long as the plan did not change in between. If it did, the criteria count differs and `record` refuses with a `recheck` hint. The current-item rule (the selected task in a single-task run, otherwise the first open item not skipped in the outcome) lives beside the builder. The `check_goal`, `implement` (retry block) and `update_plan` callbacks all call it, so the agent's prose rule in the current-task partial is mirrored by code rather than re-derived.

**The plan reader grows, its outputs do not.** Criterion text and legacy phase items are added as fields that are excluded from JSON. Validation, `plan export` and `status` keep their output exactly, so `plan status` progress, export JSON and every existing test fixture stay valid. Legacy plans gain a parsed `Phases` list that only the goal package reads.

**Install-time rendering of per-agent helpers.** A new embedded template folder, `templates/subagents/`, sits apart from `templates/agents/`, which already means AGENTS.md section fragments. It holds one shared brief partial and one wrapper per agent format (Markdown frontmatter for Claude, Copilot and omp; TOML for Codex). Each agent's `Install` renders its wrapper with the effective model (the override from config, else that agent's default) and writes it atomically next to its skills, following the existing skills-install shape. The brief is also served as a skill (`spektacular skill review-task-goal`), so the generic fallback and every named helper read identical rules. The instruction-surface guard's template roots and the data-payload test walk the new folder.

**Templates branch on two flags, both off by default in rendering terms.** `check_goal` is a new template. `implement` renders a retry block only when the goal package reports the item is mid-round with a `retry` decision. `update_plan` renders a goal-check branch, listing exactly the criteria to tick, only when the check is enabled; the switched-off branch is today's text byte for byte. The current-item partial and the plan-changelog loop gain one skipped-item clause each, also under the enabled flag. Standalone renders keep the words "orchestrator", "QUESTION:" and their kin out, as `cmd/orchestrated_test.go` requires, because the stops rely on the appended orchestrated partial for hand-backs.

**Followed patterns.**

- Error envelopes with next actions.
- `--from` staged bodies.
- Commands registered beside `implement merge`.
- The multi-source FSM steps.
- The spec workflow's verbatim-brief fresh-context reviewer.
- Harbor rendering per agent (from implement-epic) and per scenario (from repo-workflow).
- A build-time fixture guard (from implement-epic's `seed.sh`).
- Agent-independent verification from CLI state and the session log (from the epic suites).

**New patterns.**

- A workflow step whose next transition is computed by a separate CLI verb.
- A per-plan JSON outcome record in the plan store that only one package writes.
- Installed sub-agent definitions per coding agent.

## Dependencies

**Design documents this plan was built on:**

- `plan-task-graph.md` from the `design` design source. It is the settled task shape the goal is built from: `**Id:**`, `**Repo:**`, the acceptance-criteria checkboxes and legacy-plan compatibility. This plan changes one rule in it, the criteria tick rule, through `design author`, as the spec requires.

**Internal packages (`spektacular` repo):**

- `internal/plantask`: the plan reader and task-file reader. It is extended with criterion text and legacy phase items, with no change to serialised output.
- `internal/steps/implement` and `internal/workflow`: the step table, callbacks and FSM. They gain the `check_goal` step, loop-backs and a runtime `GoalCheck` flag on `workflow.Config`.
- `internal/stepkit`: template rendering and the appended orchestrated hand-back partial. It is used unchanged.
- `internal/store` and `internal/metadata`: the store the outcome record is written through. They are used unchanged.
- `internal/config`: the project configuration. It gains `implement.goal_check` with no settings-format bump.
- `internal/agent`: the per-agent installers. Each installs the reviewer definition.
- `internal/output`: error envelopes with `next_action`. It is used unchanged.
- `cmd` (`implement`, `epic merge`, `skill`): the new `implement goal` verbs and the merge guard, and the reviewer skill is served by the existing `skill` command.
- `internal/sessionlog` and `tests/harbor/shared/sessionlog.py`: these record and read the `implement goal` calls. They are used unchanged.

**External tools (no new Go libraries):**

- Coding agents' native sub-agent formats: Claude Code `.claude/agents`, Codex `.codex/agents`, GitHub Copilot `.github/agents`, oh-my-pi `.omp/agents`, and Bob's built-in `explore` helper. Field names and the cheap default model names must be confirmed against each agent's current documentation when the installer task starts.
- Harbor CLI and Docker, for the harbor suites. They are not run in CI.
- The `docs` repo's Astro site build and type check.

**Upstream specs and plans:**

- `000058_plan-task-graph` (implemented): the task format, the plan reader and task-scoped implement runs this plan builds on.
- `000065_implement-in-worktrees` and `000062_epic-plan-and-implement` (implemented): the worktree code roots handed to sub-agents, and the orchestrator merge rules the guard extends.
- `000066_epic-mid-run-revisions` (implemented): the orchestrated stop and hand-back pattern for implement steps, reused as is.
- Sibling `000069_compact-agents-md` (same epic, no dependency): it rewrites AGENTS.md managed sections in `internal/agent`. This plan adds no managed section, but both plans touch agent install files and their tests, so `spektacular epic order` may order them.

Nothing must land before this plan starts.

## Testing Approach

The plan uses all three of the project's test layers (knowledge: testing architecture). Go tests carry the deterministic guarantees, template-contract tests pin the prose, and harbor suites prove the behaviour with real agents. Harbor runs are manual because the suites do not run in CI.

**Go unit and command tests (the bulk of the coverage).**

- **Goal package.** This is the load-bearing component, tested as pure functions:
  - Building a goal from a task-format plan and from a legacy phase plan gives every criterion of that item, in order, plus its repo and files, and nothing from any other item.
  - Reply validation accepts exactly one verdict per criterion with non-empty evidence. It rejects missing, duplicate and out-of-range criteria, unknown verdicts and empty evidence.
  - The decision rule:
    - all met → accept;
    - any impossible → stop on the first check, with no retry counted;
    - not met → retry, twice, then stop as exhausted;
    - `keep_fixing` opens a new round;
    - `recheck` re-runs the check;
    - `accept` and `skip` settle the item.
  - The outcome record round-trips through the store and derives the attempt count and state.
- **Plan reader.** Criterion text and legacy phases are parsed. `status` and `plan export` output for existing fixtures is byte-identical.
- **`implement goal` commands.** Driven through the root command with the shared reset helper (convention: tests must not depend on order), they check:
  - `record` stores N verdicts for N criteria and returns the right `next_step`, and its refusals carry a runnable `next_action`;
  - `decide` is refused when no decision is due;
  - `show` reads back the goal, every verdict with evidence, and the attempt count in both formats;
  - the verbs refuse when the check is switched off.
- **Implement FSM and callbacks.** These checks cover:
  - the exact step order including `check_goal`, and the multi-source wiring;
  - a full walk with the check on (verify → check_goal → update_plan) and with it off (verify → update_plan, with templates identical to today's);
  - the retry loop back to `implement`, whose instruction carries the reviewer's reasons;
  - `update_plan` refusing before the item is settled, then listing exactly the criteria judged met, ticking only those when the user accepts a mix and ticking nothing for a skip;
  - single-task and whole-plan runs, and a legacy-phase plan;
  - the hard-coded step walks in the command tests, updated to pass through `check_goal`.
- **Config.** `implement.goal_check` defaults on, including for a config written before the key. `enabled: false` and a `models` map round-trip, and an unknown agent name in `models` is refused with a fix.
- **Agent installers.** Every supported agent's install writes its reviewer definition, except Bob, with the default cheap model, or the override for that agent only. Read-only tools are declared where the format allows. A second install is byte-identical, and Codex still writes nothing under `.claude/`. The existing whole-tree counts are updated.
- **`epic merge` guard.** A spec whose implement lane stands at `check_goal` is refused with `goal_check_open`; at any other step the existing behaviour is unchanged.

**Template-contract tests.** These pin that:

- the `check_goal` instruction names the reviewer helper, Bob's `explore`, the generic fresh-context fallback with `spektacular skill review-task-goal`, the input contract (goal, verification results, pointers, and no conversation), the staged-reply `--from` record command, and both stops with their options;
- its STOP directive is present (the existing every-non-terminal-step check), and orchestrated renders carry the hand-back while standalone renders carry no orchestrator words;
- the reviewer brief forbids edits, requires one verdict with evidence per criterion, and tells the reviewer to judge from the files, not the implementer's claims;
- the `spek-implement-epic` skill lists the goal-check stops as genuine questions;
- no template uses stdin or heredoc bodies.

**Harbor suites (behavioural proof, run by hand).** The changes cover the `implement-workflow` and `implement-epic-workflow` oracles and solution walk, plus a new goal-check suite with two scenarios, rendered for Claude, Codex and Bob:

- **Unmet scenario.** The task's documentation-line criterion is missing from the files while its tests pass and the run is told the task is complete. The verifier asserts:
  - the first recorded check judges that criterion `not_met`, with evidence;
  - a retry follows, and the recorded attempt count rises;
  - the working tree is unchanged across the reviewer's run, and the reviewer's input holds no conversation;
  - nothing is ticked before a verdict exists for every criterion;
  - only `met` criteria are ticked.
- **Met scenario.** The verifier asserts that every criterion is judged `met` on the first attempt.

On Claude the verifier also asserts, from the transcript, that a `spek-goal-reviewer` helper was launched and returned on the cheaper model. Bob shows an ACP sub-agent launch, and Codex has no sub-agent signal in its transcript. The implement-epic verifier asserts recorded verdicts for every task of every spec.

**Success metrics.**

- No false "met": the unmet scenario returns `not_met` for the seeded criterion in all of at least 10 runs per agent, on each agent's default reviewer model. **Manual — captured in the implementation test plan.**
- Finished work is not sent back: the met scenario returns `met` for every criterion on the first attempt in at least 9 of 10 runs. **Manual — captured in the implementation test plan.**
- Reviewer spend under 10% of the implementing agent's spend for the same task, on agents where a fast model can be chosen. **Manual — captured in the implementation test plan.**
- The check adds under 2 minutes per task in the harbor implement suite. **Manual — captured in the implementation test plan.**

**Manual reviews and checks.**

- Harbor runs of every suite this plan touches (`implement-workflow`, `implement-epic-workflow` under Claude, Codex and Bob, and the new goal-check suite under Claude, Codex and Bob) pass, as the testing-architecture entry requires of any change to workflow steps. **Manual — captured in the implementation test plan.**
- Under Copilot and omp, which harbor cannot drive, an implement run on a throwaway project records verdicts for every task, and the agent log shows `spek-goal-reviewer` on the cheaper model. **Manual — captured in the implementation test plan.**
- Under Codex, the reviewer runs on the configured mini model, which a person confirms in the Codex session log since the transcript carries no sub-agent signal. **Manual — captured in the implementation test plan.**
- Under Bob, the `explore` helper is used as the reviewer and reads the staged diff; if it cannot, the generic fallback is used and the docs say so. **Manual — captured in the implementation test plan.**
- In an epic run whose child stops at the goal check, the orchestrator relays a `QUESTION:` from that spec and the spec is not merged until it is answered. **Manual — captured in the implementation test plan.**
- The documentation pages read correctly on the built site. **Manual — captured in the implementation test plan.**

**Deliberate gaps.** No Go test launches a real sub-agent; the reviewer's judgement is proved only by harbor. Copilot and omp have no harbor driver, so their coverage is the installer tests plus the manual check above.

## Milestones & Tasks

### Milestone 1: Implement runs check every task's goal before ticking it

**What changes**: With this milestone, an implement run no longer ticks a task's acceptance criteria straight from passing tests.

- After verification, a separate fresh-context reviewer, started with a shared brief, reads the work and judges each criterion as met, not yet met or impossible, with evidence.
- Only criteria judged met are ticked. Work that falls short goes back for up to two more attempts with the reviewer's reasons, and then the run stops to ask whether to keep fixing, accept the shortfall or skip the task. An impossible criterion stops the run at once.
- Every task's goal, verdicts and attempt count are recorded and can be read back after the run.
- Plans written before tasks are judged phase by phase. Projects can switch the check off and get today's behaviour exactly.

**Validation point**: The full Go test suite passes, covering the goal rules, the new commands, the step flow with the check on and off, and legacy plans. A throwaway project's implement run records a verdict for every criterion of every task and ticks only the met ones.

#### - [x] Task: Teach the plan reader criterion text and legacy phases
**Id:** 1e1a7dab-7778-471a-b361-0ee7e83d2dc2
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

The plan reader only counts acceptance criteria today, and it does not read the work items of plans written before tasks. This task makes it keep each criterion's text in order, and parse every legacy `Phase N.M` block with its own criteria and the files its context section names. That gives the goal check the same material for a phase as for a task. Nothing the reader already reports changes.

*Technical detail:* [context.md#task-teach-the-plan-reader-criterion-text-and-legacy-phases](./context.md#task-teach-the-plan-reader-criterion-text-and-legacy-phases)

**Acceptance criteria**:
- [x] Reading a task-format plan gives every task's acceptance criteria as text, in plan order, with each one's ticked state.
- [x] Reading a plan written before tasks gives each phase's label, title, completion and acceptance criteria.
- [x] The files a legacy phase's context section names are reported for that phase, as they already are for a task.
- [x] `plan status` and `plan export` produce exactly the same output as before for existing plans.

#### - [x] Task: Add the goal-check configuration
**Id:** 7f68231b-d84a-4042-bea0-c2754a2e69e8
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Projects get an `implement.goal_check` setting with an on/off switch, on by default, and an optional reviewer model for each coding agent. Older configurations load with the check on. The implement commands pass the switch to the workflow, so steps can tell whether the check runs.

*Technical detail:* [context.md#task-add-the-goal-check-configuration](./context.md#task-add-the-goal-check-configuration)

**Acceptance criteria**:
- [x] A new project's configuration has the goal check switched on, and a configuration written before the setting existed also loads with it on.
- [x] A project can switch the check off, and the setting survives being saved and loaded again.
- [x] A project can set a reviewer model for one agent without affecting the others, and naming an unknown agent is refused with a message saying which agents are valid.
- [x] Implement runs started and resumed through the CLI see whether the check is switched on.

#### - [x] Task: Add the goal package
**Id:** 3a9d0384-743f-4fd4-b1c8-14622f54f571
**Repo:** spektacular
**Depends on:**
- 1e1a7dab-7778-471a-b361-0ee7e83d2dc2 — Teach the plan reader criterion text and legacy phases
**Execution:** agent

A new package owns the goal concept. It builds an item's goal from the plan: the criteria plus the scope limits of the repo and the item's files. It works out which item is current, and validates a reviewer's verdicts. It decides what happens next: accept, retry, or stop because attempts ran out or a criterion is impossible. It reads and appends the per-plan outcome record through the store, so the outcome can be inspected after the run.

*Technical detail:* [context.md#task-add-the-goal-package](./context.md#task-add-the-goal-package)

**Acceptance criteria**:
- [x] A task's goal lists every acceptance criterion of that task, in order, with its repo and files, and nothing from any other task. A legacy phase's goal is built the same way.
- [x] A reply is accepted only when it holds exactly one verdict (met, not yet met or impossible) with non-empty evidence for every criterion.
- [x] All criteria met is accepted. Any impossible criterion stops at once with no retry. Not yet met is retried at most twice, then stops to ask.
- [x] After a stop, keep fixing allows two more retries, while accept and skip settle the task, and a recheck runs the check again.
- [x] Every check and decision is kept with its goal, verdicts, evidence and attempt number, and can be read back after the run.
- [x] The current item is the run's selected task, or else the first open task or phase that has not been skipped.

#### - [x] Task: Add the implement goal commands
**Id:** 923b7ace-176f-4b8f-b518-60b2c28887d2
**Repo:** spektacular
**Depends on:**
- 3a9d0384-743f-4fd4-b1c8-14622f54f571 — Add the goal package
- 7f68231b-d84a-4042-bea0-c2754a2e69e8 — Add the goal-check configuration
**Execution:** agent

This task adds `spektacular implement goal record`, `decide` and `show`.

- `record` stores one check from the reviewer's staged reply and says which step comes next.
- `decide` stores the user's answer after a stop and says which step comes next.
- `show` prints a plan's recorded goals, verdicts, evidence and attempt counts, readable or as JSON.

Every refusal says what was wrong and the exact command to fix it.

*Technical detail:* [context.md#task-add-the-implement-goal-commands](./context.md#task-add-the-implement-goal-commands)

**Acceptance criteria**:
- [x] Recording a valid reply stores one verdict per criterion and answers with the decision and the exact next command.
- [x] A reply with a missing or extra criterion, an unknown verdict or empty evidence is refused, naming the problem and how to fix it.
- [x] Recording a decision when none is due, or naming an unknown decision, is refused with a runnable fix.
- [x] After a run, `show` lists each task's goal, every criterion's verdict and evidence, and the attempt count, in both readable and JSON form.
- [x] With the check switched off, the commands refuse and say how to switch it on.
- [x] Each recorded check notes whether the work was the same when the verdicts were recorded as when it was staged for the reviewer.

#### - [x] Task: Write the reviewer brief and skill
**Id:** 89519cf7-7f43-43aa-b374-ee88bae92d5f
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

One shared brief tells a reviewer how to judge a goal:

- It works read-only.
- It gets only the goal, the verification results and pointers to the work.
- It opens the changed files itself instead of trusting the implementer.
- It gives one verdict with evidence per criterion and returns a fixed JSON reply.

The brief is served as `spektacular skill review-task-goal` for agents with no named reviewer, and is written so the installed reviewers can embed it unchanged.

*Technical detail:* [context.md#task-write-the-reviewer-brief-and-skill](./context.md#task-write-the-reviewer-brief-and-skill)

**Acceptance criteria**:
- [x] `spektacular skill review-task-goal` prints the reviewer brief.
- [x] The brief forbids changing any file and says to judge each criterion from the files and the diff, never from the implementer's description.
- [x] The brief defines the three verdicts, requires evidence for every one, and fixes the JSON reply shape.
- [x] The brief is a single shared text that the installed reviewers can include unchanged.

#### - [x] Task: Add the check_goal step to the implement workflow
**Id:** 2804db4c-2664-4c84-a0ac-a0821b77ed21
**Repo:** spektacular
**Depends on:**
- 3a9d0384-743f-4fd4-b1c8-14622f54f571 — Add the goal package
- 923b7ace-176f-4b8f-b518-60b2c28887d2 — Add the implement goal commands
- 89519cf7-7f43-43aa-b374-ee88bae92d5f — Write the reviewer brief and skill
- 7f68231b-d84a-4042-bea0-c2754a2e69e8 — Add the goal-check configuration
**Execution:** agent

The implement workflow gains a `check_goal` step between verification and the plan update.

- **The step.** It shows the current task's goal and has the agent stage the work's diff. It launches the reviewer with only the goal, the verification results and pointers, then records the verdicts.
- **What follows.** The CLI's decision sends the run on to the plan update, back to implementing with the reviewer's reasons, or to a stop that asks the user.
- **The plan update.** It refuses until the task is settled, then ticks only the criteria judged met.
- **Whole-plan runs.** Skipped tasks are passed over.
- **Check switched off.** Every step behaves exactly as today.

*Technical detail:* [context.md#task-add-the-check_goal-step-to-the-implement-workflow](./context.md#task-add-the-check_goal-step-to-the-implement-workflow)

**Acceptance criteria**:
- [x] With the check on, every task goes from verification to the goal check, and no criterion is ticked until a verdict is recorded for every one of its criteria.
- [x] The reviewer is told to start fresh and is handed only the goal, the verification results and pointers to the work, never the implementing agent's conversation.
- [x] A not-yet-met verdict sends the run back to implementing with the reviewer's reasons, and the next check follows.
- [x] When attempts run out, the run stops and asks to keep fixing, accept or skip. When a criterion is impossible, it stops right after that check and asks how to proceed.
- [x] After the user accepts a mix of verdicts, exactly the criteria judged met are ticked. A skipped task ticks nothing and is passed over in a whole-plan run.
- [x] With the check switched off, no verdicts are recorded and criteria are ticked from verification as before.
- [x] A plan written before tasks runs to completion with each phase judged the same way.

### Milestone 2: Every coding agent gets a cheap, read-only reviewer, and epic runs stop safely

**What changes**: Initialising a project for Claude, Codex, Copilot or omp now installs a named reviewer helper in that agent's own format. It runs on a cheaper model than the agent's default where the agent allows it, and with read-only tools where the agent supports them. Bob uses its built-in read-only explore helper. A project can pick the reviewer's model separately for each agent. In an epic, a spec whose run stops at the goal check reaches the user as a question from that spec, and its work cannot be merged while the question is open.

**Validation point**: The full Go test suite passes, covering every agent's install with default and overridden models, the epic merge refusal and the epic skill wording. Initialising throwaway projects shows the reviewer definition for each agent.

#### - [x] Task: Install a reviewer helper for every agent
**Id:** 1383ae8c-d97a-44f0-bcda-af2bdb6a697d
**Repo:** spektacular
**Depends on:**
- 89519cf7-7f43-43aa-b374-ee88bae92d5f — Write the reviewer brief and skill
- 7f68231b-d84a-4042-bea0-c2754a2e69e8 — Add the goal-check configuration
**Execution:** agent

Installing Spektacular for Claude, Codex, Copilot or omp now writes a `spek-goal-reviewer` helper in that agent's native format. Each one embeds the shared brief, runs on a cheaper model than the agent's default, and has read-only tools where the format allows. A project's per-agent model override replaces the default for that agent only. Bob installs no helper and uses its built-in read-only explore helper. The CLI README describes the helper and its models.

*Technical detail:* [context.md#task-install-a-reviewer-helper-for-every-agent](./context.md#task-install-a-reviewer-helper-for-every-agent)

**Acceptance criteria**:
- [x] Initialising a project for Claude, Codex, Copilot or omp installs a reviewer helper that embeds the shared brief.
- [x] Each installed helper names a cheaper model than that agent's default working model, with no project configuration needed.
- [x] The Claude and Copilot helpers can only read and search, and the Codex helper is told never to change files.
- [x] Setting a reviewer model for one agent changes that agent's helper only. Other agents keep their defaults.
- [x] Re-running init, or upgrading with migrate, refreshes the helper and leaves a second run byte-identical.
- [x] Initialising for Bob installs no helper, and the README explains that Bob's reviewer is its built-in explore helper.

#### - [x] Task: Keep epic merges off an open goal-check question
**Id:** 06981fa2-6f64-4817-9517-f2be6022faad
**Repo:** spektacular
**Depends on:**
- 2804db4c-2664-4c84-a0ac-a0821b77ed21 — Add the check_goal step to the implement workflow
**Execution:** agent

In an epic, a spec whose run stops at the goal check hands a question back to the orchestrator, and the spec must not be merged while it is open. `epic merge` refuses while the spec's run stands at the goal check, saying the question must be answered first. The epic implement skill counts both goal-check stops as genuine questions to put to the user.

*Technical detail:* [context.md#task-keep-epic-merges-off-an-open-goal-check-question](./context.md#task-keep-epic-merges-off-an-open-goal-check-question)

**Acceptance criteria**:
- [x] Merging a spec whose implement run is stopped at the goal check is refused, with a message naming the open question and how to resume the run.
- [x] Merging a spec at any other point behaves as before.
- [x] The epic implement skill tells the orchestrator that attempts running out and an impossible criterion are genuine questions to put to the user.
- [x] An orchestrated run's goal-check stop is handed back as a question from that spec's run.

### Milestone 3: The check is proved end to end with real agents

**What changes**: This milestone is internal test infrastructure with no user-visible behaviour of its own. It is worth its own milestone because the testing-architecture rule requires every harbor suite a workflow-step change touches to move in the same change, and because the spec's success metrics are measured here. The implement and implement-epic suites follow the new step. A new goal-check suite seeds one task whose criterion is subtly unmet while its tests pass, and one whose criteria are all met, under Claude, Codex and Bob.

**Validation point**: The harbor suites build and their verifiers pass locally against recorded runs. The repeated-run success metrics are captured in the implementation test plan.

#### - [x] Task: Update the implement and implement-epic harbor suites
**Id:** 2b37b105-9a88-410e-a62e-d53724bcdbd3
**Repo:** spektacular
**Depends on:**
- 2804db4c-2664-4c84-a0ac-a0821b77ed21 — Add the check_goal step to the implement workflow
- 923b7ace-176f-4b8f-b518-60b2c28887d2 — Add the implement goal commands
**Execution:** agent

This task brings the existing harbor suites in line with the new step.

- **Implement suite.** Its expected step order and seeded run state now include the goal check.
- **Implement-epic suite.** Its solution walk records verdicts at the goal check, and its verifier asserts that every task of every spec has recorded verdicts, whichever agent ran.
- **Bob.** Bob runs of these suites rely on the Bob harbor agent in the shared location the Makefile expects, which sibling spec 000069_compact-agents-md moves there. This task does not move it.

*Technical detail:* [context.md#task-update-the-implement-and-implement-epic-harbor-suites](./context.md#task-update-the-implement-and-implement-epic-harbor-suites)

**Acceptance criteria**:
- [x] The implement suite expects the goal check between verification and the plan update.
- [x] The implement-epic solution walks through the goal check and records verdicts for every task.
- [x] The implement-epic verifier fails a run in which any task of any spec has no recorded verdicts.

#### - [x] Task: Add the goal-check harbor suite
**Id:** 6cad4738-30a2-40f0-bd5e-de2248854c0c
**Repo:** spektacular
**Depends on:**
- 2804db4c-2664-4c84-a0ac-a0821b77ed21 — Add the check_goal step to the implement workflow
- 1383ae8c-d97a-44f0-bcda-af2bdb6a697d — Install a reviewer helper for every agent
- 2b37b105-9a88-410e-a62e-d53724bcdbd3 — Update the implement and implement-epic harbor suites
**Execution:** agent

A new harbor suite has two scenarios, rendered for Claude, Codex and Bob:

- **Unmet.** A small app's task has a criterion missing from the files (a required documentation line), while its tests pass and the run is told the work is done.
- **Met.** Every criterion is met.

The verifier works from the CLI's recorded outcome and session log, so one verifier serves every agent. It checks:

- the verdicts;
- that nothing is ticked before the check;
- the retry and attempt count;
- that the reviewer leaves the files untouched and its input carries no conversation;
- where a transcript shows it, that the reviewer was a separate helper on a cheaper model.

*Technical detail:* [context.md#task-add-the-goal-check-harbor-suite](./context.md#task-add-the-goal-check-harbor-suite)

**Acceptance criteria**:
- [x] The unmet scenario's verifier passes only when the seeded criterion is first judged not yet met, a retry follows and the recorded attempt count rises.
- [x] The met scenario's verifier passes only when every criterion is judged met on the first attempt.
- [x] Both scenarios fail a run in which any criterion is ticked before a verdict exists for all of them, or a criterion not judged met is ticked.
- [x] Both scenarios fail a run in which the reviewer changed any file, or its recorded input holds anything beyond the goal, the verification results and pointers.
- [x] On Claude, the verifier confirms that the verdicts came from a separate reviewer helper running on a cheaper model than the main agent.
- [x] The suite builds and runs for Claude, Codex and Bob from make targets, with the scenario chosen per run.

### Milestone 4: The check is documented and the task design matches it

**What changes**: The documentation site's workflow and configuration reference describe:

- the goal check and its three verdicts;
- the on/off setting and the per-agent model override;
- each agent's model limitations: Bob has no per-helper model, Copilot's `Auto` session model ignores the helper's model, and Codex's reviewer is read-only by instruction only.

The plan-task design now says criteria are ticked only when judged met.

**Validation point**: The docs site builds and type-checks cleanly. The design document reads back with the new tick rule.

#### - [x] Task: Update the tick rule in the plan-task-graph design
**Id:** ecbf3c65-b5a3-40ba-abbe-0852b7d0ebe5
**Repo:** spektacular
**Depends on:**
- 2804db4c-2664-4c84-a0ac-a0821b77ed21 — Add the check_goal step to the implement workflow
**Execution:** agent

The referenced task design still says criteria are ticked when they passed verification. This task revises that rule to say they are ticked only when the goal check judged them met, or from verification when the check is switched off. The revision goes through `spektacular design author`, so the document keeps its capture date and its links to the specs that reference it. In an orchestrated run the child never writes a design itself: it hands the staged revision back for the orchestrator to apply.

*Technical detail:* [context.md#task-update-the-tick-rule-in-the-plan-task-graph-design](./context.md#task-update-the-tick-rule-in-the-plan-task-graph-design)

**Acceptance criteria**:
- [x] The plan-task-graph design says acceptance criteria are ticked only when judged met by the goal check, or from verification when the check is switched off.
- [x] The design keeps its capture date and lists both specs that reference it.
- [x] Nothing else in the design changes.

#### - [x] Task: Document the goal check on the docs site
**Id:** c41f3c86-6417-4e0d-bb9d-b9612c617241
**Repo:** docs
**Depends on:**
- 2804db4c-2664-4c84-a0ac-a0821b77ed21 — Add the check_goal step to the implement workflow
- 1383ae8c-d97a-44f0-bcda-af2bdb6a697d — Install a reviewer helper for every agent
**Execution:** agent

The documentation site describes the goal check:

- **Workflow reference.** The implement step list and the Implement stage cover the check, its three verdicts and the retry rule.
- **Plan tasks page.** It says criteria are ticked only when judged met.
- **Configuration reference.** It documents the on/off setting, the per-agent model override and each agent's reviewer model and limitations.
- **Changelog.** The docs changelog gains an entry.

*Technical detail:* [context.md#task-document-the-goal-check-on-the-docs-site](./context.md#task-document-the-goal-check-on-the-docs-site)

**Acceptance criteria**:
- [x] The workflow reference describes the goal check, where it runs, and its three verdicts (met, not yet met, impossible) with what each one does.
- [x] The plan tasks page says criteria are ticked only when the goal check judged them met.
- [x] The configuration reference lists the on/off setting, the per-agent model override and each agent's default reviewer model.
- [x] The configuration reference states each agent's model limitations: Bob has no per-helper model, Copilot ignores the helper's model under `Auto`, and Codex's reviewer is read-only by instruction only.
- [x] The site builds and type-checks cleanly.

## Open Questions

These items can only be settled by exercising the agents themselves at implementation time. Everything else is decided in this plan.

- **Can Bob's built-in `explore` helper be named from a prompt, and can it read the staged diff and the task's files?** This depends on Bob Shell's helper behaviour, which only a Bob run shows. The `check_goal` template already falls back to a generic fresh-context sub-agent when `explore` is unavailable. If `explore` cannot be used, keep the fallback wording, record it in the plan changelog's Discoveries, and make the docs say Bob's reviewer runs on Bob's default model. STOP and ask the user only if neither path can return the verdict reply.
- **Do the agents' current helper formats match the fields this plan assumes?** The fields are Codex `.codex/agents/*.toml` (`model`, `developer_instructions`, possibly `sandbox_mode`), Copilot `.github/agents/*.agent.md` (`tools`, `model`), and omp `.omp/agents/*.md` (`model: "@smol"`, its read-only tool names). This depends on each agent's documentation at implementation time. If a format differs only in field names, adapt the template and record the deviation. If an agent has no project-level helper format at all, STOP and ask the user, because the spec expects a named reviewer with a cheaper model for Claude, Codex, Copilot and omp.
- **Does Copilot pick `.github/agents/spek-goal-reviewer` over a same-named `.claude/agents/` definition in a project initialised for both?** This depends on Copilot's discovery order. If Copilot shows both, or picks Claude's, record it as a discovery and say in the docs to remove `.claude/agents/spek-goal-reviewer.md` in Copilot-only use. No STOP is needed, since both carry the same brief.

## Out of Scope

- **Spec-level goals.** Checking a spec's own success criteria at reconciliation, or gating an epic's merge on them, is not part of this plan (spec Non-Goal). It is a possible follow-up spec.
- **Changing how verification commands run.** The `verify` step and its sub-agent are unchanged. The check only adds a judgement after them (spec Non-Goal).
- **A verdict for scope limits.** The task's scope (its repo, its files and the "nothing from another task" rule) is part of the goal the reviewer reads, but only acceptance criteria get verdicts, so a task with N criteria records exactly N verdicts.
- **Changing the model a helper uses at runtime.** A per-agent model override takes effect when the reviewer definition is installed (`init`, or `migrate` on upgrade). Nothing rewrites it mid-session.
- **Working around agent limits.** Copilot's `Auto` session model ignoring a helper's model, Codex's lack of a read-only helper mode, and Bob's lack of a per-helper model are documented, not worked around (spec Technical Approach).
- **Adding Copilot or omp to harbor.** Harbor drives only Claude, Codex and Bob today. Copilot and omp are covered by installer tests and a manual check.
- **Documenting Copilot as a shipping agent on the docs site.** The site does not list Copilot today. This plan mentions it only in the reviewer's per-agent limits on the configuration page.
- **Removing reviewer files an older release installed.** No installer removes stale files today, and this plan adds no clean-up.
- **The AGENTS.md managed sections.** No managed section is added or changed; sibling spec `000069_compact-agents-md` owns that surface.
- **Refusing every merge of an in-progress spec.** `epic merge` refuses only at the goal check; other in-progress merges behave as before.

## Changelog

### 2026-10-21 — Task: Teach the plan reader criterion text and legacy phases

**What was done**: The plan reader now keeps each acceptance criterion's text and ticked state in order (`Criteria.Items`, never serialised), joining an indented spec-style detail line onto its criterion. It parses every legacy `#### - [ ] Phase N.M:` block into a `Phase` (label, title, milestone, repo, completion, criteria) with `Plan.Phase` and `Plan.OpenPhases` lookups, and `PhaseFiles` reads the files a `### Phase N.M:` context section names. `status` copies only the criteria counts, so its JSON and pretty output stay byte-identical.

**Deviations**: `Phase` also carries `Milestone` and `Repo` (from a phase's `**Repo:**` line), which the plan's struct did not list; `PhaseFiles` uses that repo as the attribution fallback, like a task's. There is no `plan export` command in the CLI (the export is served through `status`), so the byte-identity check compared `status --format json` and pretty output for three plans with binaries built before and after the change. The tests were written alongside the code in the main context rather than by a separate test sub-agent.

**Files changed**:
- `spektacular: internal/plantask/plantask.go`
- `spektacular: internal/plantask/files.go`
- `spektacular: internal/plantask/plantask_test.go`
- `spektacular: internal/plantask/files_test.go`
- `spektacular: internal/status/report.go`

**Discoveries**: `status` builds `TaskStatus.AcceptanceCriteria` straight from the parsed `Criteria`, so any field added to `plantask.Criteria` reaches status's Go types even when it is excluded from JSON; status now copies only `Met`/`Total`.

### 2026-10-21 — Task: Add the goal-check configuration

**What was done**: The `implement` config section gains `goal_check` (`enabled`, default on, and an optional per-agent `models` map), parsed over the defaults so older configs load with the check on and no settings-format bump is needed. An unknown agent name or an empty model in `models` is refused with `config_invalid`, naming the valid agents and the fix. `workflow.Config` gains a runtime-only `GoalCheck` flag, which `implement new` and `implement goto` now both set through one helper, `implementWorkflowConfig`.

**Deviations**: The two duplicated `workflow.Config` literals in `cmd/implement.go` were folded into `implementWorkflowConfig`, so the flag is set in exactly one place and is unit-tested there; the end-to-end proof that a run's steps see the flag comes with the `check_goal` step's walk tests.

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/workflow/workflow.go`
- `spektacular: cmd/implement.go`
- `spektacular: cmd/implement_goal_config_test.go`
- `spektacular: internal/agent/agent_test.go`

**Discoveries**: `config.SupportedAgentNames` duplicates `agent.Supported()` because `internal/config` cannot import `internal/agent`; `TestSupportedAgentNamesMatchConfig` pins the two together.

### 2026-10-21 — Task: Add the goal package

**What was done**: New package `internal/goal` owns the goal concept. `Build` turns a task (by id) or a legacy phase (by `Phase N.M` label) into a goal of its numbered criteria plus its scope (repo, the files its context section names, the fixed scope rule). `Current` picks the run's selected task or else the first open, not-skipped item. `ValidateReply` accepts only exactly one verdict with evidence per criterion, naming every problem otherwise. `Decide`/`NewCheck`/`NextAttempt` apply the rule (impossible stops at once, all met accepts, not met retries twice within a round, then exhausted), and `ApplyDecision` handles keep_fixing (new round), recheck (new round at check_goal), accept and skip. `Load`/`Save`/`AppendCheck`/`AppendDecision` keep the record as `goals.json` in the plan's folder of the plan store, deriving each item's attempt count and state on load.

**Deviations**: `ValidateReply` also reads a bare verdict array and a reply wrapped in one Markdown code fence, since reviewers often fence JSON; the record still stores only the validated verdicts. `NextAttempt` refuses a new check while an item awaits the user's decision (`goal_decision_due`, a refusal code the plan did not list), and a check of an already-settled item opens a new round. Helpers `NextStep`, `RetriesLeft`, `Shortfall`, `CriteriaWith` and `OpenUnskipped` were added for the CLI verbs and step callbacks. Tests were written in the main context.

**Files changed**:
- `spektacular: internal/goal/goal.go`
- `spektacular: internal/goal/decide.go`
- `spektacular: internal/goal/current.go`
- `spektacular: internal/goal/store.go`
- `spektacular: internal/goal/goal_test.go`
- `spektacular: internal/goal/decide_test.go`

**Discoveries**: A `Decision` carries the round whose stop it answers, so an item's state is a pure function of its latest check and any decision on that round; no stored state field is trusted.

### 2026-10-21 — Task: Add the implement goal commands

**What was done**: New `spektacular implement goal record|decide|show` (`cmd/implement_goal.go`). `record` reads the reviewer's reply from `--from`, rebuilds the item's goal from the plan and context, refuses an item that is not the run's current one, validates the reply, appends the check and answers with the decision, round, attempt, retries left, the met/not-met/impossible criteria, `next_step` and the exact next command (the `decide` command shape for a stop). Before appending it compares each code root's live `git diff HEAD` and `git status --porcelain --untracked-files=all` with the `<repo>.diff`/`<repo>.status` staged under `.spektacular/tmp/<plan>/goal-check/`, storing `tree_unchanged` and a warning on a mismatch or missing file. `decide` records the user's answer and returns the next goto. `show` prints the outcome readably or as JSON, whatever the switch says. Every refusal carries a runnable `next_action`.

**Deviations**: The tree comparison leaves `.spektacular/` paths out on both sides, since the agent stages its files and the CLI writes `goals.json` there during the check. Code roots are the spec's worktree roots, else every registered repo whose code is on disk (`repo.Set.LocalSource`). Extra refusals beyond the plan's list: `from_required`, `from_unreadable`, `plan_not_found`, `goal_decision_due` (a record while a stop awaits the user) and `goal_format_unsupported`. `cmd/instruction_contract_test.go` has no command-name oracle for implement subcommands, so nothing was added there. Tests were written in the main context.

**Files changed**:
- `spektacular: cmd/implement_goal.go`
- `spektacular: cmd/implement_goal_test.go`

**Discoveries**: `gitexec.Run` trims its output, so the staged diff and status are compared after trimming both sides; a reviewer's edit to the content of an already-untracked file is not visible to `git status` and so is not caught by the fingerprint.

### 2026-10-21 — Task: Write the reviewer brief and skill

**What was done**: The single reviewer brief lives in `templates/partials/goal-reviewer-brief.md`: the independent-reviewer stance, the read-only rule, the inputs, how to judge from the files and the diff rather than the implementer's claims, the three verdicts, the evidence rule and the fixed JSON reply shape. `templates/skills/skill_review-task-goal.md` wraps it with a short preamble, and `spektacular skill review-task-goal` serves it; the skill command now renders every library skill through the embedded-template renderer so a skill can include a shared partial.

**Deviations**: None.

**Files changed**:
- `spektacular: templates/partials/goal-reviewer-brief.md`
- `spektacular: templates/skills/skill_review-task-goal.md`
- `spektacular: cmd/skill.go`
- `spektacular: cmd/skill_test.go`
- `spektacular: templates/goal_reviewer_brief_test.go`

**Discoveries**: `spektacular skill <name>` used to serve the file raw; no existing library skill contains a mustache tag, so rendering all of them is byte-identical for the existing skills. The brief must stay free of `{{`, `'''` and `"""` so it embeds unchanged in mustache-rendered and TOML helper definitions; a test pins this.

### 2026-10-21 — Task: Add the check_goal step to the implement workflow

**What was done**: The implement FSM gains `check_goal` (from `verify`) with loop-backs `check_goal → implement` and `check_goal → update_plan`; `verify` names `check_goal` as its next step only when `workflow.Config.GoalCheck` is on. The new `steps/implement/05b-check_goal.md` renders the current item's goal block verbatim, the staging of each code root's diff and status, the reviewer launch (the `spek-goal-reviewer` helper, Bob's `explore`, or a fresh-context sub-agent given `skill review-task-goal`) with the goal/verification/pointers-only input rule, the `implement goal record --from` command, and the exhausted and impossible stops with their options and the `decide` command. `implement` renders the reviewer's reasons on a retry; `update_plan` refuses (`goal_check_not_settled`, with a runnable next action, including the switch-off escape for a run upgraded mid-task) until the item is settled, then lists exactly the criteria judged met, or nothing for a skip. The current-task partial and `update_changelog` pass over skipped items, and a single-task run's last-task count and `finished` exclude skipped tasks. With the check off every instruction renders byte-identically to before.

**Deviations**: `workflow.Config` also gains `Repos` (the registered repo names) so the step builds the same goal as `implement goal record`. The step stores the item it rendered as workflow data `goal_item` (cleared at `analyze`), because after a skip the current-item rule already passes over the skipped task and `update_plan` would otherwise act on the next one. A `goal_check` template flag is added to every implement step's variables when the check is on, so the shared current-task partial can carry the skipped-item clause. Tests that walk the FSM now use explicit `Goto`, since `verify` has two successors and `Next()` picks one at random; cmd walks pass through `check_goal` with a `passGoalCheck` helper that records an all-met reply, except walks over plans whose tasks are ticked up front, which go `verify → update_plan` as the FSM allows. The worktree auto-commit tests now expect `goals.json` in the project's artifacts commit.

**Files changed**:
- `spektacular: internal/steps/implement/steps.go`
- `spektacular: internal/steps/implement/goalcheck.go`
- `spektacular: internal/steps/implement/goalcheck_test.go`
- `spektacular: internal/steps/implement/steps_test.go`
- `spektacular: internal/workflow/workflow.go`
- `spektacular: cmd/implement.go`
- `spektacular: cmd/implement_goal.go`
- `spektacular: templates/steps/implement/05b-check_goal.md`
- `spektacular: templates/steps/implement/03-implement.md`
- `spektacular: templates/steps/implement/06-update_plan.md`
- `spektacular: templates/steps/implement/07-update_changelog.md`
- `spektacular: templates/partials/implement-current-task.md`
- `spektacular: templates/skills/workflows/spek-implement/SKILL.md`
- `spektacular: cmd/instruction_contract_test.go`
- `spektacular: cmd/goalcheck_walk_test.go`
- `spektacular: cmd/autocommit_test.go`
- `spektacular: cmd/implement_test.go`
- `spektacular: cmd/implement_task_run_test.go`
- `spektacular: cmd/implement_lane_test.go`
- `spektacular: cmd/milestones_test.go`
- `spektacular: cmd/orchestrated_commit_test.go`

**Discoveries**: looplab/fsm's `AvailableTransitions` iterates a map, so `Workflow.Next()` is nondeterministic from any step with two successors; walks must use `Goto`. Mustache `{{x}}` HTML-escapes apostrophes and quotes, so criterion text and evidence are rendered with `{{{x}}}`. `goals.json` sits in the plan's folder, so auto-commit sweeps it into the artifacts commit with the plan.

### 2026-10-21 — Task: Install a reviewer helper for every agent

**What was done**: `init`/`migrate` now install a `spek-goal-reviewer` helper for Claude (`.claude/agents/`, `haiku`, `Read, Grep, Glob`), Codex (`.codex/agents/*.toml`, `gpt-6-luna`, told never to change files), Copilot (`.github/agents/*.agent.md`, `Claude Haiku 4.5`, `read` and `search`) and omp (`.omp/agents/`, `@smol`, `read, grep, glob`), rendered from `templates/subagents/` with the shared reviewer brief embedded unchanged and the project's `implement.goal_check.models` override for that agent only. Bob installs none. The README gains a "Goal reviewer" section with the paths, default models, the override and each agent's limits.

**Deviations**: Model names and formats were confirmed against the agents' docs on 2026-10-21 (sources in `internal/agent/subagents.go`). Codex's docs say `sandbox_mode` in an agent file does not override the parent session, so the plan's optional `sandbox_mode = "read-only"` was left out and the Codex reviewer is read-only by instruction only. The Codex default is `gpt-6-luna`, which its docs name the faster, lower-cost model for lighter subagent work, beside the default `gpt-6.1-sol`. omp's read-only tools follow its plan-mode list (`read`, `grep`, `glob`); the `@smol` alias in an agent's `model` field is documented only as a role and should be confirmed in the manual omp run. `cmd/migrate_test.go` checks that migrate's reinstall writes the helper, and `cmd/init_test.go`'s existing whole-tree second-run check covers byte identity.

**Files changed**:
- `spektacular: internal/agent/subagents.go`
- `spektacular: internal/agent/subagents_test.go`
- `spektacular: internal/agent/claude.go`
- `spektacular: internal/agent/codex.go`
- `spektacular: internal/agent/copilot.go`
- `spektacular: internal/agent/omp.go`
- `spektacular: internal/agent/copilot_test.go`
- `spektacular: internal/agent/instruction_surface_test.go`
- `spektacular: templates/subagents/claude.md`
- `spektacular: templates/subagents/codex.toml`
- `spektacular: templates/subagents/copilot.agent.md`
- `spektacular: templates/subagents/omp.md`
- `spektacular: cmd/migrate_test.go`
- `spektacular: README.md`

**Discoveries**: Copilot in VS Code also loads `.claude/agents/`, so a project initialised for both Claude and Copilot may show two `spek-goal-reviewer` definitions; both carry the same brief, and the README says so. Copilot honours a custom agent's `model` only in the IDEs, not in the cloud agent, and its docs do not say what happens under `Auto`.

### 2026-10-21 — Task: Keep epic merges off an open goal-check question

**What was done**: `epic merge` refuses with `goal_check_open` (resource: the spec) while the spec's implement lane is in progress at `check_goal`, naming the open question and the command that resumes the run; any other step, or no lane, merges as before, and `implement merge` is unchanged. The `spek-implement-epic` skill's genuine-question list and child prompt now name the goal check running out of attempts or judging a criterion impossible, and its merge step says a spec with an open goal-check question is never merged. The orchestrated `check_goal` render carries the `QUESTION:` hand-back through the shared orchestrated partial; the standalone render carries none.

**Deviations**: None.

**Files changed**:
- `spektacular: cmd/epic_worktree.go`
- `spektacular: cmd/epic_worktree_test.go`
- `spektacular: templates/skills/workflows/spek-implement-epic/SKILL.md`
- `spektacular: templates/implement_epic_skill_test.go`
- `spektacular: internal/steps/implement/orchestrated_test.go`

**Discoveries**: None.

### 2026-10-21 — Task: Update the implement and implement-epic harbor suites

**What was done**: The implement suite's `EXPECTED_STEP_ORDER` and seeded `state.json` include `check_goal` between `verify` and `update_plan` (its run starts at `test_plan`, so nothing in it reaches `update_plan`). The implement-epic solution walk now takes every task of every spec through `analyze → implement → test → verify → check_goal`, records an all-met reply with `implement goal record`, then `update_plan`, ticks that task and runs `update_changelog`, before the wrap-up. Its verifier gains `TestGoalCheck`: every task of every spec (from `status`) must have recorded checks with one verdict with evidence per criterion, and in each spec's lane every move into `update_plan` must follow a recorded `goal record` or `decide`. The agent timeout rises from 5400s to 7000s. An oracle harbor run of the rendered suite passed all 54 verifier tests, the 8 new ones included.

**Deviations**: `implement goal record|decide|show` naming a spec with a running implement lane are now filed under that lane in the session log (`cmd/root.go` `snapshotStatePath`), so the verifier finds each record in the lane it gates; before this they went to the shared log. The Bob harbor agent was already in `tests/harbor/agents/` (sibling 000069 landed first), so no note was needed.

**Files changed**:
- `spektacular: tests/harbor/implement-workflow/tests/test_implement_workflow.py`
- `spektacular: tests/harbor/implement-workflow/environment/state.json`
- `spektacular: tests/harbor/implement-epic-workflow/solution/solve.sh`
- `spektacular: tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py`
- `spektacular: tests/harbor/implement-epic-workflow/task.toml`
- `spektacular: cmd/root.go`
- `spektacular: cmd/sessionlog_lane_test.go`

**Discoveries**: The session log files a command under a lane only for `new`, `goto` and commands whose positional arguments name the spec; a verb that names its spec only in `--data` needs its own case in `snapshotStatePath`.

### 2026-10-21 — Task: Add the goal-check harbor suite

**What was done**: New suite `tests/harbor/goal-check-workflow/`. Its image commits a small todo app, then `seed/seed.sh` writes a final spec and a one-task plan (criteria: `todo count` prints the count, unit tests cover it, the README documents it) through the CLI, puts the task's work in the working tree (README line only in the `met` scenario), walks the implement run to `verify` and fails the build unless the tests pass and the run stands at `verify`. `instruction.md` resumes the run with `spek-implement`; the `unmet` scenario adds the false claim that the work is complete. The agent-independent verifier `tests/test_goal_check.py` checks, from `implement goal show`, the plan and the session log: N verdicts with evidence per check, the recorded goal and scope, the scenario's verdicts (unmet: criterion 3 `not_met` first, a `goto implement`, a second check with a higher attempt; met: one all-met check), nothing ticked or `update_plan` before the first record, only met criteria ticked, and `tree_unchanged` on every check. From Claude's transcript (and Bob's ACP trajectory) it checks the reviewer's input carries the goal and staged paths but not the claim, and on Claude that a `spek-goal-reviewer` helper returned on a haiku model while the main agent did not run on haiku. `tests/harbor/shared/transcripts.py` holds the transcript readers, and `test_verifier_selfcheck.py` proves every rule fails a run that breaks it (10 passing). `make harbor-test-goal-check[-claude|-codex|-bob]` with `SCENARIO=unmet|met` renders and runs it. Oracle harbor runs of both scenarios pass (6 passed, the 2 transcript checks skipped).

**Deviations**: The seeded work is left uncommitted rather than committed, so the staged `git diff HEAD` the reviewer reads shows the task's work. The seed sets `implement.worktrees: false`, so the run works in `/app` itself. The scenario is baked into the build (`environment/seed/scenario` and `tests/scenario`) as well as exported as `SPEK_SCENARIO`, since the seed needs it at image build time. The no-tick-before-check rule reads the session log's `plan file write` and `goto update_plan` events, as their content is not logged. The transcript readers were written fresh for this suite in `tests/harbor/shared/transcripts.py` rather than copied from the plan suite, which keeps its own.

**Files changed**:
- `spektacular: tests/harbor/goal-check-workflow/instruction.md`
- `spektacular: tests/harbor/goal-check-workflow/scenario-unmet.md`
- `spektacular: tests/harbor/goal-check-workflow/scenario-met.md`
- `spektacular: tests/harbor/goal-check-workflow/task.toml`
- `spektacular: tests/harbor/goal-check-workflow/environment/Dockerfile`
- `spektacular: tests/harbor/goal-check-workflow/environment/app/`
- `spektacular: tests/harbor/goal-check-workflow/environment/seed/`
- `spektacular: tests/harbor/goal-check-workflow/solution/solve.sh`
- `spektacular: tests/harbor/goal-check-workflow/tests/test.sh`
- `spektacular: tests/harbor/goal-check-workflow/tests/test_goal_check.py`
- `spektacular: tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py`
- `spektacular: tests/harbor/shared/transcripts.py`
- `spektacular: Makefile`

**Discoveries**: In a harbor image whose seed copies only `config.yaml`, the registered repo's `repo.yaml` appears once the agent's own `init` runs, so the code root the goal check fingerprints resolves to `/app` from then on.

### 2026-10-21 — Task: Update the tick rule in the plan-task-graph design

**What was done**: The `Acceptance criteria` row of the "Tasks in plan.md" table in design `design:plan-task-graph.md` now says criteria are ticked by implement only when the goal check judged the criterion met, or, with `implement.goal_check.enabled: false`, only when it passed verification. `design read` shows the new row, `created_date: "2026-09-25"` and `specs` listing `000058_plan-task-graph` and `000068_task-goal-validation`; nothing else in the document changed, and `design ref list` for this spec still resolves.

**Deviations**: As an orchestrated child, this run staged the revision and handed it back as a `QUESTION:`; the orchestrator applied it with `design author` after the user approved it.

**Amendments**: Design `design:plan-task-graph.md`, the acceptance-criteria tick rule, revised as spec 000068's constraint requires; recorded by the orchestrator with `spektacular spec amend` carrying the design field.

**Files changed**:
- `spektacular: design:plan-task-graph.md` (through `spektacular design author`)

**Discoveries**: None.

### 2026-10-21 — Task: Document the goal check on the docs site

**What was done**: How it works: the Implement stage's step tree names `check_goal`, the Implement pipeline node reads "independent reviewer checks each criterion", and a new paragraph describes the fresh-context, read-only reviewer, the three verdicts and what each does, the two-retry rule and the keep fixing, accept or skip stop, `implement goal show`, the epic stop and the off switch. Plan tasks: criteria are ticked only when the goal check judged them met, or from verification with the check off. Configuration: the example and the `implement` key document `implement.goal_check.enabled` and `implement.goal_check.models.<agent>`, each agent's default reviewer model and each agent's limits. The docs `CHANGELOG.md` gains an entry. `make build` and `make check` are clean (0 errors, 0 warnings; the 2 hints are the existing JSON-LD script ones), and there is no layout HTML in page bodies and no em dash in the new text.

**Deviations**: The homepage's Implement pipeline node (`src/pages/index.mdx`) repeated the same "validates against acceptance criteria" wording, so it was updated too, as the plan allowed. The configuration page's frontmatter description lists no `implement` sub-keys and was left unchanged. The reviewer models are those the installer task confirmed: `haiku`, `gpt-6-luna`, `Claude Haiku 4.5` and `@smol`.

**Files changed**:
- `docs: src/pages/how-it-works.mdx`
- `docs: src/pages/index.mdx`
- `docs: src/pages/plan-tasks.mdx`
- `docs: src/pages/configuration.mdx`
- `docs: CHANGELOG.md`

**Discoveries**: None.
