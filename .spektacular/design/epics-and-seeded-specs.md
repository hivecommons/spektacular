---
created_date: "2026-09-28"
document_status: draft
---

# Epics and seeded specs

High-level design for GitHub issues #55 (detect oversized specs and offer to split them into an
epic) and #56 (start a spec from existing material). Draft: the decisions still open are listed at
the end.

## Problem

- **Oversized requests (#55).** When a request is too big for one spec, the interview ends with
  acceptance criteria too thin to build from, and the implementing agent fills the gaps with its
  own assumptions.
- **Existing write-ups (#56).** Teams already write work up as GitHub issues, epics and design
  docs. Starting a spec from one depends on how carefully the request is worded, and nothing
  records where the content came from.

## Terminology

| Term | Meaning |
|---|---|
| **Spec** | An ordinary Spektacular specification: the same sections, workflow and lifecycle as today. Everything below adds to specs without changing them. |
| **Epic** | A parent document grouping several specs that together deliver one oversized request. It holds only the overall overview, the list of its specs with their dependencies. Everything else, including constraints, non-goals and every acceptance criterion, lives in its specs. An epic is never planned or implemented itself; its specs are. It lives in the optional epics store. |
| **Epic's specs** | The specs belonging to an epic. Each is a normal spec with one extra frontmatter field, `epic: <name>`, and is planned and implemented on its own. A spec belongs to at most one epic. |
| **Standalone spec** | A spec with no epic, which is every spec today. |
| **Split** | Turning a request that is too big for one spec into an epic plus its specs. It is always offered to the user, never done automatically. The spec in progress becomes the epic's first spec. |
| **Stub** | A draft spec written by a split. Its Overview and Acceptance Criteria are filled in, plus any constraints or non-goals carried over from the split, and it has not been through its own workflow run yet. Starting `spec new` on a stub's name continues it from what is already there. A stub is a draft, but not every draft is a stub. |
| **Dependency** | Spec B depends on spec A, both in the same epic, when B cannot be implemented until A has been. It does not constrain writing or planning B. Recorded in the epic. |
| **Implemented** | A spec whose plan exists and has every task complete. A spec that is `final` is only written; a spec with a plan is only planned. Neither counts as implemented. |
| **Ready** | A spec in an epic whose dependencies are all implemented. Derived on read, never stored. |
| **Chaining** | When one of an epic's specs finishes, offering to start the next ready spec that has not yet been through its own workflow run. |
| **Source** | Existing material a spec or epic is started from: a GitHub issue or epic (with its sub-issues and task lists), a design document, or a markdown file. The agent fetches it; the CLI never does. |
| **Seeding** | Pre-filling a spec's work files from a source (or from a stub) before its interview, so each step confirms a draft instead of asking from scratch. |
| **Gap** | A spec section the source doesn't cover, or covers too thinly to build from. After seeding, the interview asks only about gaps. |
| **Provenance** | The record of where a document's content came from: the `sources` frontmatter field, a list of `{uri, retrieved_date}`, on the document the source directly seeded. Never inherited or replicated: a spec in an epic reaches its epic's sources through `epic`. |

## Approach

Split (#55) and seeding (#56) share one mechanism: **pre-filled work files**. A spec step that
finds its work file under `.spektacular/work/<name>/` already filled presents it as a draft to
confirm rather than asking cold. Continuing a stub is seeding from that stub. This makes the
existing spec-trigger carry-forward behaviour systematic instead of conversational.

## Process

This is how a piece of work moves from a request to shipped code once epics and seeding exist. The
sections after this one detail each part.

```
                 ┌───────────────── 1. START ─────────────────┐
                 │ conversation     issue / design doc / file │
                 │      │           agent fetches + stages    │
                 │      └──────────────┬──────────────────────┘
                 ▼                     ▼
            spec new ──────────▶ seed work files, list gaps
                                       │
                 ┌───────────────── 2. SPECIFY ───────────────┐
                 │ interview (gaps only) → sections → verify  │
                 │         │ standing check in every step     │
                 │         ▼ too big? offer split             │
                 │   split: agree specs, deps, overview + AC  │
                 │   epic split → epic + stubs                │
                 │   narrow current spec, carry on            │
                 │         │                                  │
                 │   finished → chain to next stub ──┐        │
                 │         ▲                         │        │
                 │         └──── spec new <stub> ◀───┘        │
                 └────────────────────┬───────────────────────┘
                                      ▼
                 ┌───────────────── 3. PLAN ──────────────────┐
                 │ plan new <spec> for each spec, any order   │
                 └────────────────────┬───────────────────────┘
                                      ▼
                 ┌───────────────── 4. IMPLEMENT ─────────────┐
                 │ implement new <plan>                       │
                 │   dependency check (warn / refuse)         │
                 │   tasks → complete                         │
                 └────────────────────┬───────────────────────┘
                                      ▼
                     epic done when every spec is implemented

          5. STATUS at any point: spektacular status <epic | spec | plan>
```

**1. Start.** Work starts from a conversation, as today, or from a source. With a source, the agent
fetches it (for example `gh issue view`), stages it, and runs `spec new` with the source URI. The
CLI never fetches. Seeding pre-fills the spec's work files and lists the gaps.

**2. Specify.** The spec workflow runs as today, but the interview asks only about gaps, and every
section step confirms a pre-filled draft where one exists. The standing check runs in every step.
If the work is too big, the agent offers a split; the user decides.

- On a split, the user and agent agree the specs, their dependencies, and each one's overview and
  acceptance criteria. `epic split` writes the epic and a stub per new spec. The spec in progress
  is narrowed to the first spec and carries on from where it was.
- When a spec finishes, chaining offers the next stub. Accepting starts `spec new` on it, which
  seeds from the stub, so its interview again asks only about gaps. The user can stop at any point
  and pick up any stub later.
- A source that already lists child items goes straight to the split, with one spec per child.
- Without a split, the result is one standalone spec, exactly as today.

**3. Plan.** Unchanged. Each spec gets its own plan with `plan new <spec>`, in any order. Planning a
spec whose dependencies are not implemented yet is normal and never checked.

**4. Implement.** `implement new <plan>` runs the dependency check before the first task: every
spec the plan's spec depends on must be implemented, meaning its plan has every task complete.
Unmet dependencies warn and let the user continue, or refuse under `epic.strict_dependencies`.
Implementation then runs as today.

**5. Status.** At any point, `spektacular status <name>` with an epic, spec or plan name shows the
whole epic: each spec's state, what blocks it, and every plan's tasks. With no name, it shows the
workflow in progress.

**Who decides what.** The user decides every fork: whether to split, the list of specs and their
dependencies, each drafted section, whether to chain to the next stub, and whether to continue past
an unmet dependency. The agent proposes; the CLI allocates IDs, writes the epic and stubs, keeps
the links both ways, validates the graph, and derives status.

**Order of work.** The process does not force an order. A user can specify every spec in the epic
before planning any (breadth first), or take one spec from spec through implement before starting
the next (depth first). Dependencies only constrain implementation.

## Epics

**Store.** An optional `epic` location in `config.yaml`, file provider only:

```yaml
epic:
  provider: file
  strict_dependencies: false
  config:
    directory: epics
```

`strict_dependencies` sets how implement treats an unmet dependency (see *Dependencies between
specs*): `false`, the default, warns and lets the user continue; `true` refuses. It follows the
precedent of `plan.strict_spec_changes`.

CLI verbs `epic read / write / list / delete`, under the same store-access rules as specs and plans.
Nothing changes in a project until its first split.

**Links.** An epic lists its specs in `specs`, each entry carrying its dependencies. Each of those specs names its epic in
`epic`. Both sides are always written together, following the design-ref pattern
(`cmd/design_ref.go`, `writeBackLink`). Epics don't nest.

**Planning.** Unchanged. Plans stay 1:1 with specs, so each of an epic's specs is planned on its
own. Everything a plan needs is in its spec.

**Dependencies.** The epic holds the dependency graph between its specs, the same way a plan holds
the graph between its tasks (`plan-task-graph.md`). One level up the same shape repeats:

```
epic ──contains──▶ specs ──depends_on──▶ specs
plan ──contains──▶ tasks ──depends_on──▶ tasks
```

See *Dependencies between specs* below.

### Epic document

The body has two sections: Overview and Specs. Everything else lives in the specs. An acceptance
criterion that can only be verified once several specs are in place belongs to the spec that
completes it: the one that depends on the others. A constraint or non-goal that applies to several specs is written into each of
them: a spec stays complete on its own, and its plan never has to read the epic. An epic never has
Requirements, Acceptance Criteria, Constraints, Non-Goals, Technical Approach or Success Metrics sections; wanting one is
a sign the work is really a single spec, or that the content belongs in the specs.

Example: this work itself. A spec was started from issues #55 and #56 together (a spec seeded from
two sources), and during its requirements step the standing check found three independent pieces of
work. The user accepted the split, producing `epics/000060_epics-and-seeded-specs.md`:

```markdown
---
created_date: "2026-09-28"
document_status: draft
spec: 000060_epics-and-seeded-specs
specs:
    - name: 000060_epics-and-seeded-specs
      depends_on: []
    - name: 000061_epic-split
      depends_on: [000060_epics-and-seeded-specs]
    - name: 000062_spec-seeding
      depends_on: [000060_epics-and-seeded-specs]
sources:
    - uri: https://github.com/hivecommons/spektacular/issues/55
      retrieved_date: "2026-09-28"
    - uri: https://github.com/hivecommons/spektacular/issues/56
      retrieved_date: "2026-09-28"
---

# Epics and seeded specs

## Overview
Give the spec workflow two ways in besides a blank interview. A request too big
for one spec is split into an epic whose specs each carry their own testable
acceptance criteria, so implementing agents stop filling gaps with assumptions.
A spec can be started from material that already exists, such as a GitHub issue,
so the interview asks only about what the material leaves out, and the spec
records where its content came from.

## Specs
| # | Spec | Scope |
|---|---|---|
| 1 | 000060_epics-and-seeded-specs | Epic store and links: `epic` config, `epic` CLI verbs, `epic` / `specs` / `sources` frontmatter |
| 2 | 000061_epic-split | Detection in every spec step, the `split` step, `epic split`, chaining |
| 3 | 000062_spec-seeding | `spec new` with a source, seeding at interview, gap list, effective provenance on read |
```

The spec in progress kept its name and became the epic's first spec, narrowed to the epic store and
links. Its two sources moved to the epic, because they described the whole request. Frontmatter of
the epic's second spec, a stub written by the split:

```yaml
---
created_date: "2026-09-28"
document_status: draft
epic: 000060_epics-and-seeded-specs
---
```

It has no `sources`: it was seeded from its epic, and reaches #55 and #56 through `epic`. Had the
request been a GitHub epic with sub-issues instead, each stub would carry its own sub-issue's URI.
Its body is the normal spec scaffold with Overview and Acceptance Criteria filled in, plus any Constraints and Non-Goals the split copied into it.

| Field | On | Rule |
|---|---|---|
| `spec` | epic | Provenance: the spec whose interview produced the split, which is always the epic's first spec. Absent when the epic was written straight from a source. |
| `specs` | epic | The epic's specs, each an entry `{name, depends_on}`. `name` is the spec. `depends_on` lists the names of the specs in this epic it depends on, and is always present: `[]` when there are none. List order is display order, and breaks ties between specs that are ready at the same time. Maintained only by the CLI and validated on every write. |
| `epic` | spec | The spec's epic. The reverse of `specs`, always written with it. |
| `sources` | both | Provenance: only what directly seeded this document, never copied from the epic (see Provenance). Optional. |

`specs` already exists in the shared `Metadata` as a list of names: the specs that reference a
design document. An epic's `specs` has a different shape, a list of `{name, depends_on}` entries,
so the epic does not reuse that field. Epics are a new store with their own CLI verbs, and they get
their own frontmatter type: the shared lifecycle fields (`created_date`, `document_status`,
`closed_date`) plus `spec`, `specs` and `sources`. A design's `specs` is left exactly as it is.
`epic` and `sources` are new on specs and must be added to the closed `yamlShape` and to
`UpdateOptions`, or they are silently dropped on the next write.

## Split (#55)

**Detection.** A standing check in every spec step, interview through verification, written once
as a shared partial in the step templates. The agent watches for:

- requirements that don't depend on each other
- more than one user-facing surface
- acceptance criteria that can't all be verified by one change
- a source that lists child items

**Offer, never act.** "This sounds like more than one spec. Split it into an epic with these N
specs?", naming them. On decline, today's behaviour continues unchanged, and the offer is not
repeated unless the scope visibly grows. How readily the check fires is configurable (see open
decisions).

**Flow.** A new `split` step that every spec step can jump to.

1. **Breadth-first pass.** With the user, agree the list of specs and the dependencies between
   them, and give each a short overview and real, testable acceptance criteria. This is what fixes the thin-criteria problem: no spec
   leaves the split without its own criteria.
2. **`epic split` (CLI).** From one staged description, in a single operation: allocate the new
   specs' IDs with the configured ID method, write the epic with its dependency graph, write one
   stub per remaining spec, and link the epic and its specs both ways.
3. **Narrow the spec in progress** to the epic's first spec. Work already done that belongs to it
   stays; the overall overview moves to the epic; what belongs to other specs moves to their stubs,
   and a constraint or non-goal that applies to several specs is copied into each. The workflow continues from the
   step it was at.
4. **Chaining.** When the spec reaches `finished`, it offers the next spec in the epic that is
   still a stub, preferring one that is ready.
   Accepting starts `spec new` on that stub, which seeds from it. Only one workflow is ever active,
   which fits the single `.spektacular/state.json`. The user can also stop and continue any stub
   later.

## Dependencies between specs

Dependencies are recorded on each entry of the epic's `specs`, as that entry's `depends_on`. They
replace any prose ordering: the graph is the order.

**Validation on write.** `epic write` and `epic split` refuse an epic whose `specs`:

- has an entry without `name` or without `depends_on`;
- names the same spec twice;
- has a `depends_on` naming a spec that is not an entry in the same `specs`;
- contains a cycle, including a spec depending on itself.

A dependency on a spec outside the epic, or on a standalone spec, is not expressible yet, just as a
plan task cannot yet depend on a task in another plan.

**What a dependency means.** B depends on A when B cannot be implemented until A has been. Writing
and planning B are never held back: specifying and planning ahead is normal. The only point the
graph is checked is when implementation of B starts.

**Implemented is read from the plan.** A spec counts as implemented only when its plan exists and
every task in it is complete, read from the plan's tasks as `status` reports them. A `final`
spec is only written. A spec with a plan is only planned, even a `final` plan with no tasks done.

**The check at implement.** When implement starts on a spec, before its first task, it walks this
decision tree:

```
Is the spec in an epic?
├── no  → proceed (standalone specs have no dependencies)
└── yes → does the epic list dependencies for this spec?
          ├── no  → proceed
          └── yes → for each direct dependency D, classify it:
                    ├── D has no plan                          → unplanned
                    ├── D's plan is stale (its spec changed)   → stale
                    ├── D's plan has 0 of N tasks complete     → planned, not started
                    ├── D's plan has k of N tasks complete     → in progress (k/N)
                    └── D's plan has N of N tasks complete     → implemented
                    │
                    Are all dependencies implemented?
                    ├── yes → proceed silently
                    └── no  → name each unmet dependency and its state, e.g.
                              "000061_epic-split depends on 000060_epics-and-seeded-specs,
                               which is in progress (2/5 tasks complete)."
                              │
                              Is epic.strict_dependencies true?
                              ├── no (default) → warn: "Continue anyway?"
                              │                  ├── continue → proceed; the override is noted
                              │                  │              in the changelog
                              │                  └── stop     → offer to implement the first unmet
                              │                                 dependency that is itself ready
                              └── yes          → refuse; offer to implement the first unmet
                                                 dependency that is itself ready
```

Only direct dependencies are checked. A dependency counts as implemented only when its own plan is
complete, so a missing transitive dependency shows up through it; after an earlier override, the
warning names the direct dependency, and its own check names the rest. Under
`strict_dependencies: true` no override exists, so the graph is always implemented in order.

**Reported by `status`.** See *Status* below. The implement check and `status` classify a
dependency through the same function, so they always agree.

**Beyond the epic.** The graph gives parallel implement runs (#62) the specs that can safely run at
once: those with no dependency path between them. `status` shows the spec graph above
each plan's task graph.

## Status

### Today

Status is spread over five commands that overlap:

| Command | Reports |
|---|---|
| `spec status` | The spec workflow in progress, if any. |
| `spec status <name>` | One spec's lifecycle, and its current step if its workflow is live. |
| `plan status` | The plan workflow in progress, if any. |
| `plan status <name>` | One plan's lifecycle plus task progress and acceptance criteria counts. |
| `plan export <name>` | One plan's task graph: repo, dependencies, executor, completion. Pretty or JSON. |

To see where a piece of work stands, a caller runs three commands and joins the results by name.
With epics it would be one more command and one more level to join. All five are replaced by one.

### One command: `spektacular status <name>`

A single top-level command reports everything about a piece of work, from the epic down to each
task:

```
epic ──specs[].name──▶ spec ──(plan of the same name)──▶ plan ──tasks──▶ task
```

**Resolving the name.** `<name>` may be an epic, a spec, or a plan.

1. An epic name → report that epic.
2. A spec name → if the spec has an `epic`, report that epic; otherwise report the spec on its own.
3. A plan name → resolve to its spec (the plan's `spec` frontmatter, or the same name), then as 2.
4. Nothing matches → `artifact_not_found`, naming the stores searched.

Giving any spec in an epic returns the whole epic: that is the view needed to see whether it is
blocked, and by what. The spec asked for is named in `requested`, so a caller that only wants that
spec picks it out of `specs`. The epic and its first spec share a name after a split, and both
resolve to the epic, so the collision is harmless here.

**One shape, whatever was asked.** The result always has an `epic` (null for a standalone spec) and
a `specs` list (one entry for a standalone spec). Callers never branch on kind.

```json
{
  "requested": "000061_epic-split",
  "epic": {
    "name": "000060_epics-and-seeded-specs",
    "document_status": "draft",
    "created_at": "2026-09-28T00:00:00Z",
    "progress": { "specs_implemented": 1, "specs_total": 3, "tasks_completed": 7, "tasks_total": 11 }
  },
  "specs": [
    {
      "name": "000060_epics-and-seeded-specs",
      "state": "implemented",
      "document_status": "final",
      "current_step": "finished",
      "depends_on": [],
      "ready": true,
      "blocked_by": [],
      "plan": {
        "name": "000060_epics-and-seeded-specs",
        "document_status": "final",
        "current_step": "finished",
        "progress": { "tasks_completed": 5, "tasks_total": 5 },
        "tasks": [
          {
            "id": "7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34",
            "title": "Add the epic store",
            "milestone": 1,
            "repo": { "name": "spektacular", "location": "" },
            "depends_on": [],
            "execution": { "type": "agent", "reason": "" },
            "completed": true,
            "acceptance_criteria": { "met": 3, "total": 3 }
          }
        ]
      }
    },
    {
      "name": "000061_epic-split",
      "state": "in_progress",
      "document_status": "final",
      "current_step": "finished",
      "depends_on": ["000060_epics-and-seeded-specs"],
      "ready": true,
      "blocked_by": [],
      "plan": { 
        "name": "000061_epic-split", 
        "document_status": "final", 
        "current_step": "", 
        "progress": { 
          "tasks_completed": 2, 
          "tasks_total": 6
        }, 
        "tasks": [] 
      }
    },
    {
      "name": "000062_spec-seeding",
      "state": "stub",
      "document_status": "draft",
      "current_step": "",
      "depends_on": ["000060_epics-and-seeded-specs"],
      "ready": true,
      "blocked_by": [],
      "plan": null
    }
  ]
}
```

(Task lists after the first are elided here; the real output carries every task.)

- **Per spec:** lifecycle and live workflow step, as `spec status <name>` reports today; the
  derived state; dependencies, readiness and what blocks it.
- **Per plan:** lifecycle and live workflow step, task progress, and every task with the fields
  `plan export` reports today plus the acceptance-criteria counts `plan status <name>` reports.
  A plan without task structure reports its lifecycle with `progress` and `tasks` absent, as
  today.
- **Per epic:** its lifecycle and a roll-up, never written back: specs implemented of total, tasks
  complete of total across every plan.

`--format pretty` (the default, as for `plan export`) prints the same tree for people:

```
$ spektacular status 000061_epic-split
epic 000060_epics-and-seeded-specs  (draft)  1/3 specs implemented, 7/11 tasks

  000060_epics-and-seeded-specs   implemented   5/5 tasks
  000061_epic-split               in progress   2/6 tasks   ← requested
      depends on: 000060_epics-and-seeded-specs
      Milestone 1
        [x] Add detection partial              spektacular   agent
        [ ] Add the split step                 spektacular   agent
            depends on: Add detection partial
        ...
  000062_spec-seeding             stub          no plan
      depends on: 000060_epics-and-seeded-specs
```

The requested spec is expanded down to its tasks; its siblings show one line each. `--format json`
always carries everything.

**A spec's state** is derived, never stored, checked in this order:

| State | When |
|---|---|
| `missing` | The spec is named in the epic but cannot be read. Reported, not fatal, so one broken link does not hide the rest of the epic. |
| `stale` | The plan is stale under `plan.strict_spec_changes`: the spec changed after it. |
| `stub` | The spec is a draft that has not been through its own workflow run. |
| `specified` | The spec has been written, and no plan exists. |
| `planned` | A plan exists with 0 of N tasks complete. |
| `in_progress` | A plan exists with k of N tasks complete, 0 < k < N. |
| `implemented` | A plan exists with N of N tasks complete. |

The implement dependency check classifies a spec through the same function, so `status` and
implement never disagree. It is built on what `cmd/artifact_status.go` and `cmd/plan_export.go`
already do: `resolveDocumentStatus` with `strictPlanStatusHook`, `planTaskProgress`, and
`plantask.NewExport`.

### No name: the workflow in progress

`spektacular status` with no name reports the workflow in progress, whichever kind it is. The
single `.spektacular/state.json` already records its kind and the artifact it is working on, so no
per-kind command is needed. The result is the same shape as `status <name>` for that artifact,
with a `workflow` block added:

```json
{
  "workflow": {
    "kind": "implement",
    "name": "000061_epic-split",
    "current_step": "analyze",
    "completed_steps": ["new", "read_plan"],
    "updated_at": "2026-09-30T10:12:00Z"
  },
  "requested": "000061_epic-split",
  "epic": { "...": "as above" },
  "specs": [ "...as above" ]
}
```

`status <name>` carries the same `workflow` block when the workflow in progress belongs to one of
the reported specs or plans, and `null` otherwise. With no name and no workflow in progress, the
result is `{"workflow": null}`.

### The existing commands are removed

`status` replaces every status and export command outright. There are no aliases and no
deprecation period:

| Removed | Replaced by |
|---|---|
| `spec status`, `plan status`, `implement status` | `status` |
| `spec status <name>`, `plan status <name>` | `status <name>` |
| `plan export <name> [--format]` | `status <name> [--format]` |

Everything that names the removed commands moves to `status` in the same change:

- the `spek-implement` skill, which reads task ids with `plan export <plan> --format json`;
- `cmd/implement.go`, whose error hint points at `plan export`;
- the README's workflow-progress section;
- the `plantask` export renderer and `cmd/artifact_status.go`, which become the building blocks of
  `status` rather than commands of their own.

External orchestrators that call the per-artifact verbs or `plan export` must move to `status`.
The release notes say so, and name the field that replaces each old one.

## Seeding (#56)

- **The agent fetches, the CLI stays network-free.** The `spek-new` skill documents the path:
  fetch the source (`gh issue view` including sub-issues and task lists, WebFetch, `design read`,
  or a local file), stage it under `.spektacular/tmp/`, then:
  ```
  spec new --data '{"name":"…","source":"<uri>"}' --file <staged file>
  ```
  `source` is kept in workflow state and written to `sources` when the spec is written.
- **The interview step seeds first.** It maps the source onto the spec sections, writes the
  pre-filled work files, and lists the gaps. The interview asks only about gaps; every later step
  confirms its pre-filled draft.
- **A source with child items** trips detection straight away. The breadth-first pass then works
  from the children: one spec per child item, each seeded from that child.

### Walk-through: a small issue

A small issue produces one standalone spec, and from the outside the only difference from today is
its `sources`. For a hypothetical issue #70, "Add a `--json` flag to `spec list`":

1. The agent fetches the issue, including its comments, since decisions are often made there, and
   stages it:
   `gh issue view 70 --json title,body,comments,labels,url` → `.spektacular/tmp/issue-70.md`.
2. It proposes a name from the title, the user confirms, and it starts the spec:
   `spec new --data '{"name":"spec-list-json","source":"https://github.com/hivecommons/spektacular/issues/70"}' --file .spektacular/tmp/issue-70.md`.
   The CLI keeps the URI in workflow state and records nothing else yet.
3. The interview step seeds the work files: the title and body map to the overview and
   requirements, checklist items to acceptance criteria, "must / must not" remarks to constraints.
   It then lists the gaps, for example "no success metrics; the issue doesn't say what the output
   looks like when there are no specs", and asks only about those. A thin issue simply leaves more
   gaps, and the result is close to a normal interview that starts from a drafted overview.
4. Each section step confirms its pre-filled draft. Detection runs as usual and does not fire.
5. At verification the spec is written with
   `sources: [{uri: https://github.com/hivecommons/spektacular/issues/70, retrieved_date: "2026-09-28"}]`.

## Provenance

`sources` is one field with one shape, and both epics and specs can carry it. Each document records
only what directly seeded its content. Nothing is inherited or replicated: provenance is a chain,
and each link is stored once.

```
issue  →  epic  →  spec
```

After a split, a spec is seeded from its epic, not from the epic's source. It already records that
through its `epic` field, so it carries no copy of the epic's `sources`.

| Case | Epic `sources` | Spec `sources` |
|---|---|---|
| Small issue → one standalone spec | none (no epic) | the issue |
| Oversized single issue → split | the issue | none; provenance runs through `epic` |
| GitHub epic with sub-issues | the parent issue | its own sub-issue |
| Split mid-interview, nothing fetched | none | none |
| Spec seeded from an issue plus a design doc | — | both URIs |

- **On a split**, the spec in progress hands its source over to the epic: it was seeded from the
  whole request, and after the split the whole request is the epic's scope. A source that was only
  about the narrowed first spec stays on it.
- **A spec in an epic** records a source only when something seeded it directly, beyond what the
  epic accounts for: its own sub-issue, or material brought in for that spec alone.
- **Effective provenance** is resolved on read. `status` reports a spec's own
  `sources` followed by its epic's, so the full trail back to the issue is visible from the spec
  without being stored on it.

A seed design doc that the spec must be built to also becomes a normal design ref. `sources`
records only where content came from.

## Non-goals

- No fetching or GitHub client in the CLI.
- No sync back to, or from, the source.
- No concurrent workflows for an epic's specs (that belongs with #62).
- No nested epics.
- No plans that span several specs.

## Open decisions

Questions for review, grouped by the three areas this design most needs feedback on.

**Epic document format**

1. **Closing an epic.** Derive the epic's `document_status` from its specs (done once every spec is
   implemented), or leave it for the user to set.
2. **Naming after a split.** The spec in progress keeps its name and becomes the epic's first
   spec, and the epic takes the same name. Should the split rename either to fit its narrower or
   wider scope?

**The `status` command**

3. **Asking for one spec only.** As drafted, a spec in an epic always returns the whole epic. A flag
   such as `--spec-only` could return just that spec's entry, for callers polling one spec.

**The process**

4. **Splitting late.** Allow a split from any step and redistribute the work done so far, or only
   up to the end of requirements, after which the scope problem is noted rather than split.
5. **Chaining beyond specify.** As drafted, chaining only moves between stubs during specify. It
   could also offer planning once every spec in the epic is written, and offer the next ready
   spec's implementation when one finishes.

### Deferred to the spec

Detail that does not change the format, the command or the process, left for the specs that
implement this design:

- **Detection sensitivity:** a new `epic_split_threshold`, or reuse `spec_trigger_threshold`.
- **`epic split` input:** JSON through `--data`, or a staged markdown epic draft the CLI parses.
- **Source text:** keep only URI and date, add a content hash to detect changes, or store a
  snapshot of the source with the spec.
- **Posting back to the source:** whether the agent, with the user's permission, comments on the
  issue naming the spec created from it.
- **PR #65:** whether research seeding from Hive knowledge, ADRs and Context7 shares the seeding
  stage or stays separate.
