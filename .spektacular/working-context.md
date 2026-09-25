# Working context — 000058_plan-task-graph (GitHub issue #50)

## Problem and motivation

Issue #50 (clubanderson, Hive): Hive advances Spektacular-backed runs by polling `spektacular plan status <name>`, and once a plan is `final` needs the plan's work as a structured task graph to create implement-stage work. It asked for `spektacular plan export <name> --format json`. There is no machine-readable view of a plan's work today.

What Hive actually does (read from hivecommons/hive `src/docs/spektacular.md`, `src/pkg/spektacular/runner.go`):
- Only the **plan** is exported; the spec is polled for `document_status` only.
- Hive renders exported tasks verbatim into its planner (`[ref] title [repo:x] (depends: …) [execution]`) — "no model is asked to redecompose an already-structured plan". Imports once per run as a DRAFT epic that a human approves before implement work is listed.
- Hive's current decoder: `Plan{kind, name, tasks}`, `PlanTask{id, ref, repo string, title, depends_on []string, execution string}`; uses `ref` then falls back to `id`; `name` must equal the requested bare name; empty `tasks` rejected.
- Until the verb exists Hive falls back to `plan file read <name>/tasks.json` then `<name>/plan.md`, parsing any bullet line as a task — which does not match our `####` phase headings, so it likely mis-imports real plans today.

## Decisions (user-confirmed unless noted)

**Plan format**
- Rename **Phase → Task** in the plan format. Milestones stay as the grouping level ("build on each other in order").
- The plan's task section becomes a **strict, CLI-enforced format**. User: "I am actually fine that there is strict sections to a plan. It gives us many benefits."
- **A task is a single unit of work in a single repo** (user's words). Exactly one `**Repo:**` per task.
- Each task carries: heading checkbox + title, `**Id:**`, `**Repo:**`, `**Depends on:**`, `**Execution:**`, summary, acceptance criteria.
- **`**Depends on:**` is required**: `none` or a list of ids, each entry `<id> — <title>` (title for humans; parser reads the id only). No sequential default — user: "you might not depend on anything everything may be able to be produced in parallel". Explicit `none` separates "parallel" from "forgot".
- **`**Execution:**` is required**: `agent`, or `human — <reason>`. Set in the tasks step using stated criteria: secrets/access the agent lacks, actions outside the repo (deploy, release, DNS, purchases), judgement that must be human (legal, design sign-off, stakeholder decision), human-only verification (visual/UX, physical hardware). Mixed tasks are **split** (one unit of work = one repo = one executor), the human part depending on the agent part. The walkthrough names every human task and its reason.

**Task ids**
- User: "make task a random guid, this could later be used as a global identifier".
- Ids are **opaque strings from a pluggable id provider**, like stores ("then we can make this pluggable like stores"). Default provider `uuid`. Minted by the CLI on request during authoring (e.g. `plan task-id`), written into plan.md; the CLI never rewrites them. Providers with side effects must account for ids minted for tasks later discarded.
- **`depends_on` holds ids, not refs** — user: "it should be the guid not the ref. this enables us to have cross spec dependencies, not needed for now but useful later". For now a dependency must resolve inside the same plan.
- Rejected: plan-local `T1` refs with CLI resolution on write (CLI would rewrite authored content; can't resolve cross-spec ids).

**Export**
- `spektacular plan export <name> --format json` (json the only format).
- **Parsed from plan.md at call time** — no stored plan.json. User: "parsing the markdown feels better". Rejected: static pre-calculated plan.json (two sources of truth that drift — the #39 problem).
- **Validation on `plan file write`** of plan.md rejects: missing Id / Repo / Depends on / Execution, more than one repo, unknown dependency ids, cycles, unregistered repos, invalid execution type, human without reason.
- Shape: `{kind:"plan", name, document_status, tasks:[{id, title, milestone, repo:{name, location}, depends_on:[ids], execution:{type, reason}, completed}]}`. No `ref` (Hive falls back to `id`, so edges match).
- **`repo` is an object `{name, location}`** — user: "repo should be name and location"; "Hive can just parse its plantask.repo from the location inside repo. Name could be useful later on as it links back to spektacular config." `location` = the repo's declared git `source` (repo.yaml); empty when none is declared — never guessed from `git remote` (forks, missing origin). Registry stores no remote for local repos today.
- **`execution` is an object `{type, reason}`** (user's call), `type` is `agent` | `human` (user: "agent / human is best").
- `repo` and `execution` as objects break Hive's current string fields (json.Unmarshal fails) — Hive adapts; note this on #50.

**Progress**
- User: "One thing we should expose in the plan status is has an item been completed … putting this in json would enable a unique dashboard."
- Parsed task model exposes `completed` (task heading checkbox) and `acceptance_criteria {met, total}` (criteria checkboxes ticked by implement's update_plan step). Kept separate — implement may complete a task with an accepted deviation.
- `plan status <name>` gets compact progress (totals + per-task completed/criteria). `plan export` carries graph + `completed`. Both read the same parser.

**Implement task selection**
- User: "make hive use spektacular as the top level for the implementation runner … select a specific task to implement … implement needs things like context and potentially design documents so driving this in the implement command makes sense … let's pre-empt this by adding the feature to the implement skill and command".
- `implement new --data '{"name":…, "task":"<id>"}'`; without `task`, whole plan as today. `implement status` reports the task.
- Pre-checks refuse (with next_action): unknown task, already completed, incomplete dependencies (listed), `human` execution (carries reason).
- Read/analyze read the full plan; implement/test/verify/update_plan scoped to the one task.
- Each run appends to the changelog; test_plan, feature changelog and reconcile_spec run only when the run completes the plan's **last open task** (confirmed).
- **Parallel runs out of scope** — single state.json per project; tracked as issue #62.

**Other**
- One task parser should replace the ad-hoc regexes in `internal/autocommit/milestones.go` and `cmd/implement.go:18` (unchecked-phase count).
- Existing templates to change: `templates/steps/plan/10-phases.md` (and 09/13/18), `templates/steps/implement/06-update_plan.md` and other implement steps, spek-plan / spek-implement skills.
- Docs repo (`docs`, spektacular-website) documents `plan export` and the plan task format.
- A stale abandoned attempt exists at `.spektacular/work/000058_plan-export-json/` and `.spektacular/tmp/spec_template.md` — the user does not remember it; ignore it.

---

# Issue #46

Decisions from the 2026-09-25 discussion are posted on the issue: https://github.com/hivecommons/spektacular/issues/46#issuecomment-5830556246
