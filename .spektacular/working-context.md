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

---

# Plan workflow — 000058_plan-task-graph (started 2026-09-25)

- User chose spec 000058_plan-task-graph; committed the pending `auto_commit: full` config change before starting (commit 93ebe26).
- Discovery done: research in `.spektacular/work/000058_plan-task-graph/research.md`; judgement calls in `assumptions.md` (repo.location git-only; legacy Phase plans bypass write validation; task-id provider resolved on request, no schema bump; v4 UUID via crypto/rand).
- Target repos: spektacular (CLI, templates, skills, harbor oracles, glossary `phase.md`) and docs (how-it-works.mdx, configuration.mdx, new page + Nav.astro).
- Knowledge that binds: harbor oracles must change with step/template/scaffold changes (testing-architecture.md); validation belongs where remediation facts live; no em dashes in docs prose; plan content pages need a **Content outline**.
- Key mechanics: plan.md write choke point `cmd/storefile.go:228-233`; implement FSM has no single-task path; `finished()` hard-fails without feature changelog; milestone parser only knows `Phase`.
- Architecture: Option A chosen (shared plantask reader, existing implement FSM + update_changelog→finished edge, step phases→tasks rename). See assumptions.md.
- Sections drafted through testing_approach; next milestones then phases (this plan itself uses the CURRENT Phase format since the new format does not exist yet).
- Phases drafted: 12 phases (M1 1.1-1.4, M2 2.1-2.2, M3 3.1-3.3, M4 4.1-4.3 incl. docs content outline in plan.md phase 4.3).
- Assembled and staged plan/context/research to .spektacular/tmp/*_template.md
- Verification passed (all sections present; shell commands removed from plan.md working files).
- All three docs committed; now at walkthrough (awaiting user sign-off).
- User signed off the walkthrough (all assumptions accepted, incl. repo.location git-only).

## Implement run (started 2026-09-25)

- Plan `000058_plan-task-graph` validated; first-phase invocation (no `## Changelog` yet). Plan itself is in legacy `Phase N.M` format on purpose.
- Drift: context.md Phase 1.2 names `cmd/spec_file.go`; the spec store-file caller is actually `cmd/file.go`. User chose "proceed, map on the fly".
- Makefile harbor targets: `harbor-test-plan` and `harbor-test-implement` exist.
- Internal packages name commands without the binary prefix in next_action (e.g. 'plan task-id'), like internal/design/errors.go. plantask.Validate(p, repos) therefore takes no command param.
- User chose "run without asking" after Phase 1.1: loop phases autonomously; stop only for real decisions, failures, the glossary knowledge write (propose-then-confirm) and harbor runs.
- Phase 1.1 done: internal/plantask (Parse/Validate/RequireTasks/Task/OpenTasks/OpenItems/CompletedMilestones).
- Phase 3.2 decision: `autocommit.LeadsToCommit` now takes the rendered NextStep; completion points match only when their `to` equals it (milestone candidates still match on source step). An implement completion commit in full mode also computes/records due milestones (a task run can close a milestone at update_changelog→finished).
- Phase 4.2: user approved glossary change — glossary/task.md written, glossary/phase.md deleted (repo tier, spektacular store). Harbor plan/implement suites started in background (user approved); logs in session scratchpad harbor-plan.log / harbor-impl.log.
- Open agent-judgement ACs to settle after harbor/manual: 3.3 AC4 (agent asked for one task), 4.1 AC2-4 (ids/executor authored, mixed-work split, human tasks named at sign-off).
- Implement wrap-up: harbor plan 95/95, implement 14/14. Test plan written (4 post-release metrics + 2 manual agent-behaviour checks). Spec reconciled: 50 checked; left open: "Mixed work is split", "Single-task run uses the plan's context", "Agent can be asked for one task" (manual verification pending).
