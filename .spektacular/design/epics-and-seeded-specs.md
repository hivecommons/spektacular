---
created_date: "2026-09-28"
document_status: draft
specs:
    - 000060_epics-and-seeded-specs
    - 000062_epic-plan-and-implement
---

# Epics and seeded specs

High-level design for GitHub issues #55 (detect oversized specs and offer to split them into an
epic) and #56 (start a spec from existing material). The decisions taken are listed at the end.

## Problem

- **Oversized requests (#55).** When a request is too big for one spec, the interview ends with
  acceptance criteria too thin to build from, and the implementing agent fills the gaps with its
  own assumptions.
- **Existing write-ups (#56).** Teams already write work up as GitHub issues, epics and desikn
  docs. Starting a spec from one depends on how carefully the request is worded, and nothing
  records where the content came from.

## Terminology

| Term | Meaning |
|---|---|
| **Spec** | An ordinary Spektacular specification: the same sections, workflow and lifecycle as today. Everything below adds to specs without changing them. |
| **Epic** | A parent document grouping several specs that together deliver one oversized request. It holds only the overall overview, the list of its specs with their dependencies. Everything else, including constraints, non-goals and every acceptance criterion, lives in its specs. An epic is never planned or implemented itself; its specs are. It lives in the optional epics store. |
| **Epic's specs** | The specs belonging to an epic. Each is a normal spec with one extra frontmatter field, `epic: <name>`, and is planned and implemented on its own. A spec belongs to at most one epic. |
| **Standalone spec** | A spec with no epic, which is every spec today. |
| **Split** | Turning a complete spec that is too big into an epic plus several complete specs. It is always offered to the user, never done automatically, and always acts on a complete spec: a split asked for mid-workflow is acted on once the spec is complete. The spec being split becomes the epic's first spec. |
| **Dependency** | Spec B depends on spec A, both in the same epic, when B cannot be implemented until A has been. It does not constrain writing or planning B. Recorded in the epic. |
| **Implemented** | A spec whose plan exists and has every task complete. A spec that is `final` is only written; a spec with a plan is only planned. Neither counts as implemented. |
| **Ready** | A spec in an epic whose dependencies are all implemented. Derived on read, never stored. |
| **Chaining** | When one of an epic's specs finishes, offering to start a spec for the next item in the epic's source that has no spec yet. |
| **Source** | Existing material a spec or epic is started from: an issue or epic from any tracker (with its child items and task lists), a design document, a file, a web page, or pasted text. The agent fetches it with its own tools; the CLI never does. |
| **Seeding** | Pre-filling a spec's work files from a source before its interview, so each step confirms a draft instead of asking from scratch. Done by the agent, following the `spek-new` skill and the interview step. The CLI only records provenance. |
| **Gap** | A spec section the source doesn't cover, or covers too thinly to build from. After seeding, the interview asks only about gaps. |
| **Provenance** | The record of where a document's content came from: the `sources` frontmatter field, a list of `{uri, retrieved_date}`, on the document the source directly seeded. Never inherited or replicated: a spec in an epic reaches its epic's sources through `epic`. |

## Approach

There are two ways into an epic, and only one of them splits:

- **Split (#55).** A spec is complete and turns out to be more than one piece of work. Its content
  is already written and agreed, so the split redistributes it into several complete specs in one
  operation (`epic split`). Nothing needs a second interview.
- **Epic first (#55, #56).** The work starts as a set of items, such as a tracker epic with
  sub-issues. An epic is created first, and each spec is then built from new with the normal spec
  workflow, seeded from its own item, joining the epic as it starts.

Seeding (#56) uses **pre-filled work files**: a spec step that finds its work file under
`.spektacular/work/<name>/` already filled presents it as a draft to confirm rather than asking
cold. This makes the existing spec-trigger carry-forward behaviour systematic instead of
conversational.

## Process

This is how a piece of work moves from a request to shipped code once epics and seeding exist. The
sections after this one detail each part.

```
                 ┌───────────────── 1. START ─────────────────┐
                 │ conversation     issue / design doc / file │
                 │      │           agent fetches             │
                 │      └──────────────┬──────────────────────┘
                 ▼                     ▼
            spec new ──────────▶ seed work files, list gaps
                                       │
                 ┌───────────────── 2. SPECIFY ───────────────┐
                 │ in an epic? read the epic and its specs    │
                 │ interview (gaps only) → sections → verify  │
                 │         │ check when spec complete         │
                 │         ▼ too big? offer split             │
                 │   split: agree specs, deps, redistribute   │
                 │   review all, epic split → epic + specs    │
                 │         │                                  │
                 │   finished → offer next source item ──┐    │
                 │         ▲                             │    │
                 │         └── spec new <item> in epic ◀─┘    │
                 └────────────────────┬───────────────────────┘
                                      ▼
                 ┌───────────────── 3. PLAN ──────────────────┐
                 │ plan new <spec> for each spec, any order   │
                 └────────────────────┬───────────────────────┘
                                      ▼
                 ┌───────────────── 4. IMPLEMENT ─────────────┐
                 │ implement new <spec>                       │
                 │   dependency check (warn / refuse)         │
                 │   tasks → complete                         │
                 └────────────────────┬───────────────────────┘
                                      ▼
                     epic done when every spec is implemented

          5. STATUS at any point: spektacular status <epic | spec | plan>
```

**1. Start.** Work starts from a conversation, as today, or from a source. With a source, the
`spek-new` skill has the agent fetch it with its own tools and run `spec new` with the source in
`sources`. The CLI never fetches. The interview step seeds the spec's work files and lists the
gaps.

**2. Specify.** The spec workflow runs as today, but the interview asks only about gaps, and every
section step confirms a pre-filled draft where one exists. In a project that has epics, the agent
asks whether a new spec belongs to one; if so, it reads the epic and its specs before the
interview. When the spec is complete, the split check runs once; if the work is too big, the agent
offers a split and the user decides. The user can also ask for a split at any point, on any spec;
a request made mid-workflow is acted on once the spec is complete.

- On a split, the user and agent agree the specs and their dependencies, and redistribute the
  complete spec's content between them. One review covers every resulting spec, then `epic split`
  writes the epic and every new spec, complete and `final`. The spec being split is narrowed to
  its own part in the same operation.
- A source that already lists child items leads to an offer to create an epic first. Each child is
  then specified as its own spec in that epic, seeded from the child. When one finishes, chaining
  offers the next child that has no spec yet. The user can stop at any point and continue later.
- Without a split or an epic, the result is one standalone spec, exactly as today.

**3. Plan.** Each spec gets its own plan with `plan new <spec>`, in any order. Planning a
spec whose dependencies are not implemented yet is normal and never checked. An epic can also be
planned with one request, which keeps a planning summary with the epic and orders specs whose
plans change the same files (see *Epics → Planning*).

**4. Implement.** What gets implemented is a spec, not a plan: the user implements a spec, and the
plan is how the implement workflow carries it out. `implement new <spec>` finds the spec's plan (it
shares the spec's name) and runs the dependency check before the first task: every spec it depends
on must be implemented, meaning that spec's plan has every task complete.
Unmet dependencies warn and let the user continue, or refuse under `epic.strict_dependencies`.
Implementation then runs as today.

**5. Status.** At any point, `spektacular status <name>` with an epic, spec or plan name shows the
whole epic: each spec's state, what blocks it, and every plan's tasks. With no name, it shows the
workflow in progress.

**Who decides what.** The user decides every fork: whether to split, the list of specs and their
dependencies, each drafted section, whether a new spec joins an epic, whether to add to a completed
epic, whether to start the next item, and whether to continue past an unmet dependency. The agent
proposes; the CLI allocates IDs, writes the epic and its specs, keeps the links both ways,
validates the graph, and derives status.

**Order of work.** The process does not force an order. A user can specify every spec in the epic
before planning any (breadth first), or take one spec from spec through implement before starting
the next (depth first). Dependencies only constrain implementation.

## Epics

**Store.** An `epic` location in `config.yaml`, file provider only. `init` writes it for new
projects and `migrate` adds it, with the defaults below, to existing ones, so every project has an
epic store configured before its first split. The `epics` directory is created the first time an
epic is written. Using epics stays optional: a project that never splits sees no change in
behaviour.

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

CLI verbs `epic read / write / list / delete / split / order` and `epic summary read / write`, under the same store-access rules as specs
and plans. An epic can be written with no specs yet, for the epic-first route.

**Joining an epic.** A spec joins an epic when it is started in one
(`spec new --data '{"name":"…","epic":"<epic>"}'`), when `epic write` lists it, or through
`epic split`. In a project that has epics, starting a new spec has the agent ask whether it belongs
to one, unless the request already says. A spec joining an epic has the agent read the epic and
every spec in it first, so the interview builds on what they cover and offers to record
dependencies on them.

**Adding to a completed epic.** An epic whose specs are all implemented is complete. Any addition
to it (`spec new` into it, `epic write` or `epic split`) is refused with `epic_complete`, naming the
epic, and nothing is written. Repeating it with explicit confirmation adds the spec, and the epic
is reported as in progress again until the new spec is implemented. Completion is derived, so no
status has to be reset.

**Links.** An epic lists its specs in `specs`, each entry carrying its dependencies. Each of those specs names its epic in
`epic`. Both sides are always written together, following the design-ref pattern
(`cmd/design_ref.go`, `writeBackLink`). Epics don't nest.

**Planning.** Plans stay 1:1 with specs, so each of an epic's specs is planned on its own, and
everything a plan needs is in its spec. Planning an epic with one request ("plan this epic") adds
two things around those plans:

- **A planning summary kept with the epic**, at `<epics>/<epic>/summary.md` in the epic store. It is
  never a plan, `epic list` and ID allocation ignore its folder, and `epic delete` removes it with
  the epic. The CLI renders it in a fixed order: the decisions settled while planning first
  ("None." when there are none), then the order added for shared files, then one section per
  planned spec in epic list order, each listing the manual checks its test plan will carry.
  `epic summary write` replaces one section at a time (`decisions` or a member spec), so repeated
  planning and review edits leave the other sections untouched; only `epic order` writes the
  ordering section, as an append-only log.
- **Decisions are settled before the summary exists.** When the plans disagree on a project-wide
  rule, such as how the changelog is kept, the orchestrator puts each disagreement to the user
  with one proposed answer and applies the outcome to every plan involved. Only then does it
  write the summary, so the summary records settled decisions and never an open question.
- **Automatic ordering of overlapping specs.** When planning ends, `epic order` compares the files
  each planned spec's tasks name in the plan's context document. Two specs that share a file and
  have no ordering between them, directly or through other specs, are ordered without asking: the
  spec listed later depends on the one listed earlier. Only the epic is written, so nothing is
  re-planned. The review walks the summary, and the user can undo an added dependency there with
  `epic order --data '{"unorder":…}'`, which records the pair in `parallel_with` so it is never
  added again.

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
two sources), and when the spec was complete the split check found three independent pieces of
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
| 2 | 000061_epic-split | Detection when a spec is complete, the `split` step, `epic split`, chaining |
| 3 | 000062_spec-seeding | `spec new` with a source, seeding at interview, gap list, effective provenance on read |
```

The spec being split kept its name and became the epic's first spec, narrowed to the epic store and
links. Its two sources moved to the epic, because they described the whole request. Frontmatter of
the epic's second spec, written complete by the split:

```yaml
---
created_date: "2026-09-28"
document_status: final
closed_date: "2026-09-28"
epic: 000060_epics-and-seeded-specs
---
```

It has no `sources`: its content came from the spec that was split, and it reaches #55 and #56
through `epic`. Had the work started from a GitHub epic with sub-issues instead, each spec would
carry its own sub-issue's URI. Its body is a complete spec: every section filled with the content
the split moved to it, plus a copy of each constraint and non-goal shared with the others.

| Field | On | Rule |
|---|---|---|
| `spec` | epic | Provenance: the spec whose interview produced the split, which is always the epic's first spec. Absent when the epic was written straight from a source. |
| `specs` | epic | The epic's specs, each an entry `{name, depends_on}`, with an optional `parallel_with`. `name` is the spec. `depends_on` lists the names of the specs in this epic it depends on, and is always present: `[]` when there are none. List order is display order, and breaks ties between specs that are ready at the same time. Maintained only by the CLI and validated on every write. |
| `parallel_with` | epic, on a `specs` entry | Optional, omitted when empty. Earlier specs in the same epic that the user allowed to be implemented side by side with this one even though their plans share files. Maintained only by `epic order`, and read only by it, so a dependency the user removed is never re-added. |
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

### Split triggers

**Detection.** The check runs once, when a spec is complete, at the end of its workflow, not in
every step. It also runs whenever the user explicitly asks for a split, at any point and on any
spec, including one already written. The AGENTS.md section that recognises spec-worthy discussion
is unchanged and never offers an epic: detection belongs to creating a spec. The check asks one
question: *is this more than one independently useful piece of work?* Size alone is not the test.

**The gate.** The agent offers a split only if it can name at least two specs, each with its own
overview and at least one acceptance criterion verifiable without the others. If it cannot name
them, it does not offer, however many signals have fired. This keeps every offer concrete.

**Strong signals.** Any one is enough, provided the gate passes:

- the source lists child items (sub-issues, a task list);
- the requirements fall into groups that could each ship and be useful alone;
- the acceptance criteria cannot all be verified by one change;
- the user's own wording phases the work ("phase 1", "first … then later", "v1 is just …").

**Weak signals.** At least two together are needed:

- more than about seven requirements;
- more than one design document needed;
- an interview that is not converging, where each answer opens new areas;
- a section draft that keeps growing content belonging to a different concern.

**Supporting work never counts.** Docs, tests, migrations, config, changelog entries, and skill or
template updates that describe or support the same change belong in the same spec. Touching
several repos or several surfaces is not a signal in itself; it matters only when a part would be
useful on its own, such as a new tutorial unrelated to the code change. Code plus its docs is one
spec.

**Counter-signals.** These suppress the offer even when signals have fired:

- the requirements are tightly coupled, so neither part is useful or testable alone;
- several surfaces or repos serve one capability, or one atomic change.

**Offer, never act.** "This sounds like more than one spec. Split it into an epic with these N
specs?", naming each with its one-line scope. On decline, today's behaviour continues unchanged.
The offer is repeated only if the scope visibly grows: a new strong signal, or a new independent
requirement group, appears after the decline. Re-wording what was already there does not count.
How readily the check fires is configurable (see *Deferred to the spec*).

### Split flow

**Flow.** A new `split` step between `verification` and `finished`. It always acts on a complete
spec. A split the user asks for mid-workflow is noted, the workflow carries on gathering every
section, and the request is acted on here. A spec already written, with no workflow running, is
split through the same instructions from the `spek-new` skill.

1. **Agree the split.** With the user, agree the list of specs and the dependencies between them,
   and which content goes where. Every requirement and acceptance criterion goes to exactly one
   spec; a constraint or non-goal that applies to several specs is copied into each; the overall
   overview moves to the epic. Each spec must leave with its own testable acceptance criteria;
   where one is thin, fill it in with the user now. This is what fixes the thin-criteria problem.
2. **Review.** One fresh-eyes review covers every resulting spec, as verification does for a single
   spec.
3. **`epic split` (CLI).** From one staged description carrying every section of every spec, in a
   single operation: allocate the new specs' IDs with the configured ID method, write each new spec
   complete and `final`, rewrite the spec being split as its narrowed self, write or extend the
   epic with its dependency graph, and link the epic and its specs both ways. A failure part-way
   restores everything. If the split spec has a plan, that plan goes stale like it would after any
   other spec change.

No resulting spec needs a further workflow run. Only one workflow is ever active, which fits the
single `.spektacular/state.json`.

**Splitting a spec already in an epic.** The new specs join the same epic, since epics do not
nest. The epic's dependency graph is extended to include them.

**Sources with child items.** A source that already has child items when work starts, such as a
tracker epic with sub-issues, does not produce a single spec, and does not use `epic split`. The
agent offers to create an epic first (`epic write`, with the parent as the epic's source and no
specs yet), then starts the first child's spec in it with `spec new`, seeded from the child. When a
spec in the epic finishes, chaining offers the next child with no spec yet. A child item that
appears after the epic exists, because the user brings it up or the agent sees it when revisiting
the source, leads the agent to offer a new spec for it in that epic.

## Dependencies between specs

Dependencies are recorded on each entry of the epic's `specs`, as that entry's `depends_on`. They
replace any prose ordering: the graph is the order.

**Validation on write.** `epic write` and `epic split` refuse an epic whose `specs`:

- has an entry without `name` or without `depends_on`;
- names the same spec twice;
- has a `depends_on` naming a spec that is not an entry in the same `specs`;
- contains a cycle, including a spec depending on itself;
- has a `parallel_with` naming a spec that is not an entry in the same `specs`, or the entry itself.

A dependency on a spec outside the epic, or on a standalone spec, is not expressible yet, just as a
plan task cannot yet depend on a task in another plan.

**The one automatic writer.** People write the graph through `epic write` and `epic split`.
`epic order` is the only command that adds a dependency itself: when two planned specs name the
same file in their plans and nothing orders them, it makes the later-listed one depend on the
earlier one, and records the addition in the epic's planning summary.

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
      "state": "specified",
      "document_status": "final",
      "current_step": "finished",
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
  000062_spec-seeding             specified     no plan
      depends on: 000060_epics-and-seeded-specs
```

The requested spec is expanded down to its tasks; its siblings show one line each. `--format json`
always carries everything.

**A spec's state** is derived, never stored, checked in this order:

| State | When |
|---|---|
| `missing` | The spec is named in the epic but cannot be read. Reported, not fatal, so one broken link does not hide the rest of the epic. |
| `stale` | The plan is stale under `plan.strict_spec_changes`: the spec changed after it. |
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

Seeding is agent behaviour, not a CLI feature. Instructions carry it in two places: the `spek-new`
skill (recognising a source, fetching it, starting the workflow) and the interview step (mapping
the source onto the spec and finding the gaps). The CLI's only part is recording provenance.

**Recognising a source.** The skill's description and trigger wording pick up a source however the
request is phrased: "spec from issue 45", "#45", "start from LIN-123", a pasted link to any
tracker, a design document's name, a file path, or pasted text the user says to start from. Being
understood should not depend on careful wording.

**What the skill tells the agent to do:**

1. **Identify the source.** An issue or epic from any tracker (GitHub, Linear, Jira, …), a design
   document, a file, a web page, or pasted text. If it is ambiguous, such as a bare issue number
   with no tracker or repo, ask the user rather than guess.
2. **Fetch it with the agent's own tools.** Spektacular prescribes no tool: a CLI, an MCP server,
   a web fetch or a file read are all fine. Whatever the tool, collect:
   - the title and body;
   - the discussion (comments, since decisions are often made there);
   - child items: sub-issues, linked children, task-list entries;
   - a stable URI for the source, to record as provenance.

   If the agent has no tool that can reach the source, or fetching fails, it says so and asks the
   user to paste the content. It never falls back to a cold interview without saying so.
3. **Check for child items.** If the source already has child items, offer to set up an epic with
   one spec per child instead of a single spec (see *Sources with child items*).
4. **Propose a name** from the source's title, and let the user confirm it.
5. **Start the workflow with provenance:**
   `spec new --data '{"name":"…","sources":[{"uri":"…"}]}'`.

**What the interview step tells the agent to do.** This lives in the step, not the skill, so it
also applies on resume:

1. **Seed first.** Map the source onto the spec sections and write the pre-filled work files under
   `.spektacular/work/<name>/`:
   - title and body → overview and requirements;
   - checklist or task-list items → acceptance criteria;
   - "must / must not" remarks → constraints;
   - "out of scope" remarks → non-goals.

   `interview.md` records what came from the source.
2. **List the gaps** to the user: sections the source does not cover, or covers too thinly to
   build from. The interview asks only about those. A thin source simply leaves more gaps.
3. **Every later section step** presents its pre-filled work file as a draft to confirm or refine,
   never asking from scratch.

**The CLI records provenance and nothing else.** `spec new` accepts `sources` in `--data` as a list
of `{uri}`. The CLI stamps each entry's `retrieved_date`, holds the list in workflow state, and
writes it to the spec's frontmatter when the spec is written. `sources` is added to the closed
`yamlShape` and to `UpdateOptions`. There is no fetching, no staged source file and no new flag.

### Walk-through: a small issue

A small issue produces one standalone spec, and from the outside the only difference from today is
its `sources`. For a hypothetical issue #70, "Add a `--json` flag to `spec list`":

1. The user asks: "use the spek-new skill to start a new specification from GitHub issue 70", or
   just "spec from #70".
2. The agent fetches the issue and its comments with whatever tool it has; in this project that
   happens to be `gh`. Nothing is staged, because the agent already holds the content.
3. It proposes a name from the title, the user confirms, and it starts the spec:
   `spec new --data '{"name":"spec-list-json","sources":[{"uri":"https://github.com/hivecommons/spektacular/issues/70"}]}'`.
4. The interview step seeds the work files: the title and body map to the overview and
   requirements, checklist items to acceptance criteria, "must / must not" remarks to constraints.
   It then lists the gaps, for example "no success metrics; the issue doesn't say what the output
   looks like when there are no specs", and asks only about those. A thin issue simply leaves more
   gaps, and the result is close to a normal interview that starts from a drafted overview.
5. Each section step confirms its pre-filled draft. Detection runs as usual and does not fire.
6. At verification the spec is written with
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
| Tracker epic with child items | the parent issue | its own child item |
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

- No fetching or tracker client in the CLI.
- No CLI surface for seeding beyond accepting `sources` on `spec new`.
- No sync back to, or from, the source.
- No concurrent workflows for an epic's specs (that belongs with #62).
- No nested epics.
- No plans that span several specs.

## Decisions

1. **Closing an epic.** An epic is done when every one of its specs is implemented, meaning every
   task in every plan is complete. Derived on read and reported by `status`, never stored.
2. **Naming after a split.** No renaming. The epic and its first spec share the spec's name.
3. **`status` for one spec.** No `--spec-only` flag; callers pick the spec out of `specs` using
   `requested`.
4. **Splitting late.** Allowed at any point, including on a spec already written. A split always
   acts on a complete spec: a request made mid-workflow is acted on once the spec is complete.
5. **Chaining.** Only while specifying, and only to items of the epic's source that have no spec
   yet. A split produces complete specs, so there is nothing to chain to after one.
6. **Detection before a spec exists.** None. The spec-trigger behaviour is unchanged.
7. **Detection timing.** Once when a spec is complete, plus on explicit request.

### Deferred to the spec

Detail that does not change the format, the command or the process, left for the specs that
implement this design:

- **Detection sensitivity:** a new `epic_split_threshold` (strict / moderate / lenient, default
  `moderate`), separate from `spec_trigger_threshold`. It moves the gate and signal counts, never
  the supporting-work rule or the counter-signals.
- **Source text:** keep only URI and date, add a content hash to detect changes, or store a
  snapshot of the source with the spec.
- **Posting back to the source:** whether the agent, with the user's permission, comments on the
  issue naming the spec created from it.
- **Implement wording:** `implement new` takes the spec's name. The plan shares it, so behaviour
  does not change, but the help text ("against an existing plan"), its input field and the
  `spek-implement` skill are reworded to say a spec is being implemented.
- **PR #65:** whether research seeding from Hive knowledge, ADRs and Context7 shares the seeding
  stage or stays separate.
