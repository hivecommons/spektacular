---
created_date: "2026-10-01"
document_status: final
closed_date: "2026-10-01"
---

# Plan: 000060_epics-and-seeded-specs

<!-- Metadata -->
<!-- Created: 2026-10-01T12:13:53Z -->
<!-- Commit: 3597a63 -->
<!-- Branch: design/epics-and-seeded-specs -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Builds epics, splitting, dependencies between specs, a single status view and seeding a spec from existing material, to the settled design `epics-and-seeded-specs.md`. Oversized requests stop producing specs with criteria too thin to build from: a complete spec can be split into an epic whose specs each carry their own testable criteria and declared order, checked when implementation starts. Teams that already plan in issues and design documents get specs that ask only about what their material leaves out and that record where their content came from. One `status` command shows where a whole epic stands, replacing four overlapping status and export commands. The work spans the `spektacular` CLI (stores, commands, workflow steps, templates and skills) and the `docs` site.

## Conventions

- **Error messages must describe the problem and suggest remediation** — every new refusal (`epic write` graph errors, membership conflicts, `epic delete`/`spec file delete` guards, `dependencies_unmet`, override refused under strict mode, `status` not-found, `spec new` bad `sources`) is built with `output.NewError(...).WithNextAction(...)`, and tests assert the next_action's content.
- **Store files are written through the CLI** — epics are a new store reached only through `epic` verbs with bodies via `--from`; `epic split` writes the resulting specs itself; templates and skills never touch `epics/` or `specs/` with file tools; `templates/agents/store-access.md` must name the new verbs.
- **Tests must not depend on order** — new `cmd` tests (`status`, `epic`, `implement` dependency check) go through `resetRootCmd`/`runRootCmd`, and tests that allocate several spec IDs during `epic split` pin `spec.id_method: counter`.
- **Passing tests are required before calling work done** — the schema bump and the command removals break many existing tests; `go test ./...` must be green, and the harbor suites touched are run.
- **Glossary: stage, step, task, workflow** — new wording says "implement a spec" (stage vocabulary), the `split` step is a step, not a stage, and "task" keeps its plan meaning (an epic holds specs, not tasks).
- **Docs: MDX authoring rules** — the new epics page and the edited pages keep layout HTML out of MDX bodies, use slots, put blank lines around slot content and use fenced code blocks; verified with the grep guard, `npm run build` and `npx astro check`.
- **Docs: no em dashes** — applies to all new docs prose and the docs CHANGELOG entry.
- **Docs: plans sketch content structure** — each docs task carries a `**Content outline**` or `**Content example**`.
- **Docs: site layout and alternating section background** — the new page is composed from existing section components, with `surface` alternating explicitly, and registered in `Nav.astro`.
- **Docs: label before filename in file-scoped headings** — the `epic` keys are documented within the existing "Project configuration: config.yaml" grouping.

## Architecture & Design Decisions

The work is built on the design `epics-and-seeded-specs.md` (source `design`), which fixes the
epic document, the split triggers and flow, the dependency model and check, the `status` command
and its resolution rules, seeding and provenance. The plan decides how to implement that shape in
the existing code. Everything lands in two repos: `spektacular` (Go CLI, templates and skills)
and `docs` (the Astro documentation site). The implementation splits into five areas, each with
one owner package, so that no rule is written in two places.

**1. Epic store and links (spektacular: `internal/epic`, `cmd/epic.go`, `internal/metadata`,
`internal/config`, `internal/migrate`).** An epic is its own document kind with its own frontmatter
type in a new `internal/epic` package (`created_date`, `document_status`, `closed_date`, `spec`,
`specs[{name, depends_on}]`, `sources[{uri, retrieved_date}]`). It does not reuse the shared
`metadata.Metadata`, because an epic's `specs` clashes with the design back-link list of the same
key and `metadata.Merge` would silently drop the graph (research: `metadata.go:85,215-230`). Specs
gain `epic` and `sources` in the closed `metadata` schema, added in lockstep at every site with the
same tri-state `UpdateOptions` the design references use, so a body-only `spec file write` carries
them forward. The `epic read / write / list / delete / split` verbs are hand-written in
`cmd/epic.go` after the `design` family, taking structured fields through `--data` and bodies
through `--from`, as every other Spektacular write does (convention: store files are written
through the CLI). One function in `internal/epic` validates the graph (missing fields, duplicates,
unknown names, cycles), using a cycle finder lifted out of `plantask` into `internal/depgraph`, so
plan tasks and epic specs share one algorithm. Membership is kept in agreement by one link writer
modelled on `design ref`'s `applyRef`: epic first, then each affected spec's `epic`, restoring
every touched document if a later write fails. It never lets a spec join a second epic or an epic
join an epic. An addition to a completed epic is refused unless the user confirms it. An epic can
be written with no specs yet, for the epic-first route. A new epic is named with the configured
spec ID method, except after a split, where it takes the split spec's name. `epic delete` clears
every member's `epic`, and `spec file delete` refuses while the
spec is still in an epic. The config gains `epic` (provider, `strict_dependencies`,
`config.directory`) and `epic_split_threshold`. Because the spec requires existing projects to
receive them on migration, a `project3to4` step writes both and `CurrentProjectSchema` moves to 4,
rather than relying on load-time defaults.

**2. One status view (spektacular: `internal/status`, `cmd/status.go`).** A pure package resolves a
name (epic, then spec, then plan, then `artifact_not_found`) and builds the single report shape
from the store and the workflow state. It reuses the existing building blocks: document-status
resolution with the strict-plan stale hook, `plantask.Parse`, export tasks and criteria counts,
and repo locations. It owns the one function that classifies a spec (`missing`, `stale`,
`specified`, `planned`, `in_progress`, `implemented`) and computes `ready` / `blocked_by` and the
epic roll-up. The top-level `status [name] [--format pretty|json]` command is a thin renderer over
it. `spec status`, `plan status`, `implement status` and `plan export` are deleted outright, along
with their result types and tests. Every caller (the `spek-implement` skill, the implement task
hints, the README, the harbor plan suite) moves to `status` in the same change, and the old
spellings join the instruction-surface deny-list, following 000059's hard-break pattern.

**3. Dependency check at implement (spektacular: `cmd/implement.go`, implement step templates).**
`implement new` calls the same classifier as `status`, after the stale-plan and task checks and
before the start gate, so a refusal writes no state (precedent: `refuseStalePlan`,
`refuseUnstartableTask`). Unmet direct dependencies produce a `dependencies_unmet` refusal that
names each one and its state ("in progress (2/5 tasks complete)"), in the layer that holds those
facts (gotcha: remediation needs the layer that holds the facts). Under the default its
next_action is the same command with `"override_dependencies": true`, which the agent runs only
after the user says to continue. Under `epic.strict_dependencies: true` the override is itself
refused, and the next_action names the first unmet dependency that is ready. An accepted override
is kept in workflow data and rendered into the changelog steps, so the agent records it. The
implement help text, input description and `spek-implement` skill are reworded to say a spec is
implemented, with no change in behaviour.

**4. Split, the epic-first route and seeding (spektacular: `internal/steps/spec`, `cmd/spec.go`,
`templates/steps/spec`, `templates/partials`, `templates/skills/workflows/spek-new`).** There are
two ways into an epic, and only one of them splits.

- **Split.** It always acts on a complete spec. The spec workflow gains a `split` step on the linear
  path `verification -> split -> finished`. A split the user asks for mid-workflow is noted and acted
  on there, once every section has been gathered, so no back-and-forth FSM edges are needed. The
  agent agrees the specs, their dependencies and where every piece of content goes, then runs one
  fresh-eyes review over all of them. `epic split` then writes every resulting spec complete and
  `final`, rewrites the split spec as its narrowed self, and writes or extends the epic, in one
  operation that is restored in full on failure. No resulting spec needs another workflow run.
- **Epic first.** When a source already has child items, the agent creates the epic with
  `epic write` and no specs. Each child is then specified with the normal workflow, started with
  `spec new` carrying `sources` and `epic`, which joins the spec to the epic as it starts.
- **Chaining.** At `finished`, the agent offers a spec for the next item in the epic's source that
  has none yet. This is agent judgement over the source, so the CLI only passes the epic's name
  and sources to the template.
- **Joining an epic.** In a project that has epics, the agent asks whether a new spec belongs to
  one, and if so reads the epic and its specs before the interview.
- **Shared instructions.** The split check and split flow are written once, as
  `partials/split-check.md` and `partials/split-flow.md`. The `split` step and the `spek-new`
  skill's "split an already-written spec" section both include them, so the two cannot drift.
- **Seeding.** It is instruction-only, split as the spec's technical approach directs. The skill
  recognises, fetches and names the source and checks for child items. The interview step,
  rendered with the recorded sources, seeds the work files and lists the gaps, so a resumed
  session still seeds. Section steps present an existing work file as a draft. Seeding from
  sources is kept separate from PR #65's research seeding.

**5. Documentation (docs: `src/pages/*.mdx`, `src/components/Nav.astro`).** A new `epics.mdx` page
covers epics, splitting, split sensitivity and dependencies. `how-it-works.mdx` gains starting from
existing material and the implement-a-spec wording. A status section replaces the
`plan export` / `plan status` / `implement status` material in `plan-tasks.mdx` and
`documents.mdx`, and `configuration.mdx` documents `epic` and `epic_split_threshold`.

This direction beats the alternatives because each rule lives in exactly one place. The graph
rules are in `internal/epic` plus `internal/depgraph`. Spec classification is in `internal/status`,
shared by `status`, implement and the completed-epic guard. The split guidance is in two partials, shared by the
step and the skill. The CLI stays network-free and non-interactive, and every judgement remains
with the agent and the user. The rejected options (extending `Metadata`, `epic file` through
`newStoreFileCmd`, no migration step, a CLI fetcher, a wildcard FSM source, an interactive CLI
prompt, stub specs written by a split) and their evidence are in
`research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Dependency graph helper (new, spektacular).** Owns cycle detection over a set of named nodes with
  declared dependencies, returning the cycle path. Lifted out of the plan-task validator so plan tasks
  and epic specs share one algorithm. The plan-task validator and the epic validator both call it.
- **Epic document (new, spektacular).** Owns the epic's frontmatter type (lifecycle fields, `spec`,
  `specs[{name, depends_on}]`, `sources`), parsing and rendering it, stamping lifecycle dates on write,
  and validating the specs graph (missing fields, duplicates, unknown names, cycles) through the
  dependency graph helper. It is pure: the epic command and the status builder are its callers.
- **Spec frontmatter (changed, spektacular).** The shared document metadata gains `epic` and
  `sources` (each `{uri, retrieved_date}`) in its closed schema, with tri-state update options so a
  spec rewrite carries both forward and only the epic link writer changes `epic`.
- **Epic link writer (new, spektacular).** Owns membership agreement: given an epic's old and new
  member lists, it writes the epic and then sets or clears `epic` on each affected spec. It refuses a
  spec already in another epic and an epic listed as a member, and restores every touched document if
  a later write fails. Modelled on the design-reference link writer. Used by `epic write`,
  `epic split` and `epic delete`.
- **Epic command family (new, spektacular).** `epic read / write / list / delete / split`. `write`
  takes the body via `--from` and the structured fields (`specs`, `sources`, `spec`,
  `confirm_completed_epic`) via `--data`. It allows an empty specs list, and names a new epic with the
  configured spec ID method when given a bare name. `split` reads one staged JSON description that
  carries every section of every resulting spec. It then:
  - allocates the new specs' IDs one at a time;
  - writes each new spec complete and `final`;
  - rewrites the split spec as its narrowed self;
  - creates or extends the epic and hands the result to the link writer.

  `delete` clears every member's link. All verbs follow the store-access rules and publish `--schema`.
- **Spec store guards (changed, spektacular).** `spec file delete` refuses while the spec belongs to
  an epic, naming the `epic write` that removes it first.
- **Configuration and migration (changed, spektacular).** The config gains the `epic` store section
  (`provider`, `strict_dependencies`, `config.directory`) as a project-relative store directory, and
  `epic_split_threshold` (strict / moderate / lenient), both with defaults and validation. A
  schema-3-to-4 migration step writes both into existing projects when absent. `init` writes them for
  new projects through the defaults. The epics folder is created on the first write by the file store.
- **Artifact addressing (changed, spektacular).** Gains the `epic` kind, so epic names parse and
  resolve like the other single-name kinds.
- **Status builder (new, spektacular).** Owns name resolution (epic, spec, plan), the single report
  shape (`workflow`, `requested`, `epic`, `specs[]` with plans and tasks), effective provenance (a
  spec's own sources followed by its epic's), the epic roll-up and completion, and the one spec
  classifier (`missing`, `stale`, `specified`, `planned`, `in_progress`, `implemented`) with
  `ready` / `blocked_by`. It reuses the document-status resolver with the strict-plan stale hook, the
  plan-task parser, the export task model with acceptance-criteria counts, and repo locations. It
  also renders the pretty tree. Its callers are the `status` command, the implement dependency check
  and the completed-epic guard.
- **Status command (new, spektacular).** Top-level `status [name] [--format pretty|json]`, a thin
  wrapper over the status builder, with no name meaning the workflow in progress.
- **Retired status and export commands (removed, spektacular).** `spec status`, `plan status`,
  `implement status` and `plan export`, their per-kind status result types and their tests. Their
  logic survives inside the status builder. Every caller (implement task hints, the `spek-implement`
  skill, README, harbor suites) moves to `status`, and the old spellings are deny-listed in templates.
- **Implement start checks (changed, spektacular).** `implement new` gains the dependency check,
  using the status builder's classifier. It refuses with `dependencies_unmet` unless overridden, and
  under strict dependencies refuses the override too. It records an accepted override in workflow
  data. Help text and input description say a spec is being implemented.
- **Implement step instructions (changed, spektacular).** The changelog steps render a recorded
  dependency override so the agent writes it into the changelog.
- **Spec workflow (changed, spektacular).** `spec new` accepts `sources` (stamped with the retrieval
  date and held in workflow data) and `epic` (the epic must exist). The `new` step writes the sources
  to the spec and joins it to the epic through the link writer. A new `split` step sits between
  `verification` and `finished`. `finished` passes the spec's epic and that epic's sources to its
  template for the chaining offer. The auto-commit point moves from `verification -> finished` to
  `split -> finished`.
- **Completed-epic guard (new, spektacular).** One check used by `spec new` with `epic`, `epic write`
  and `epic split`: adding a spec to an epic whose specs are all implemented is refused with
  `epic_complete` unless the call confirms it. Completion comes from the status builder.
- **Spec step instructions and partials (changed and new, spektacular).**
  - **New shared partials:** `split-check` holds the gate, signals, supporting-work rule,
    counter-signals and live `epic_split_threshold`. `split-flow` holds agreeing the split,
    redistributing content, the review pass, staging and `epic split`, provenance hand-over, and
    acting on a mid-workflow request at completion.
  - **`split` step:** its template includes both partials.
  - **Interview step:** gains seeding from recorded sources with the gap list, and reading the epic
    and its specs when the spec joins one.
  - **Section steps:** confirm pre-filled drafts.
  - **`finished`:** gains the chaining offer over the epic's source items.
  - **Any step:** a split request made before verification is noted in the working context.
- **Workflow skills and managed agent sections (changed, spektacular).**
  - **`spek-new`:**
    - asks whether a new spec belongs to an existing epic (only when epics exist);
    - recognises and fetches a source, tool-agnostic;
    - handles the child-item route (create the epic, then specify each child);
    - proposes a name and runs `spec new` with `sources` and `epic`;
    - handles a completed-epic warning;
    - splits an already-written spec through the shared partials.
  - **`spek-implement`:** says "implement a spec" and uses `status` for task ids.
  - **Store-access section:** names the `epic` verbs.
  - Generated skill copies are regenerated.
- **Harbor end-to-end suites (changed, spektacular).** Step-order and command oracles, solve scripts
  and the plan-export assertion follow the new `split` step, `spec new` schema and `status`.
- **README (changed, spektacular).** The status section documents `status`.
- **Documentation site (changed, docs).** New epics page (epics, splitting, sensitivity,
  dependencies) registered in the nav. How-it-works covers starting from existing material and
  implementing a spec. The status view replaces the removed commands across plan-tasks and documents.
  The configuration reference documents `epic` and `epic_split_threshold`. A site changelog entry is
  added.

## Data Structures & Interfaces

**Source reference (spec and epic frontmatter).** One shape for provenance, carried by both kinds.

```go
type SourceRef struct {
    URI           string `yaml:"uri" json:"uri"`
    RetrievedDate string `yaml:"retrieved_date" json:"retrieved_date"` // YYYY-MM-DD, stamped by the CLI
}
```

**Spec frontmatter additions.** `Metadata` gains `Epic string` (yaml `epic`) and
`Sources []SourceRef` (yaml `sources`), both omitted when empty. `UpdateOptions` gains
`Epic *string` and `Sources *[]SourceRef` (nil = keep, non-nil = replace, non-nil empty = clear).
Only the epic link writer sets `Epic`; only `spec new` sets `Sources`.

**Epic document.** Its own type, never routed through the shared spec metadata merge.

```go
type EpicSpec struct {
    Name      string   `yaml:"name" json:"name"`
    DependsOn []string `yaml:"depends_on" json:"depends_on"` // always present, [] when none
}
type Epic struct {
    CreatedDate, DocumentStatus, ClosedDate string // shared lifecycle fields
    Spec    string      // the spec whose split produced it; empty when written from a source
    Specs   []EpicSpec  // display order; breaks ties between ready specs
    Sources []SourceRef
    Body    []byte      // "## Overview" and "## Specs"
}
func Parse(raw []byte) (Epic, error)
func (Epic) Render() []byte
func Validate(specs []EpicSpec) error // epic_invalid: missing field, duplicate, unknown dep, cycle
```

**Dependency graph helper.** `FindCycle(order []string, deps map[string][]string) []string`
returns the first cycle as a path with its first node repeated, or nil. It is shared by plan-task
and epic validation.

**`epic` command contracts.**

| Verb | Input | Output |
|---|---|---|
| `epic read <name>` | — | raw document bytes |
| `epic list` | — | `{files:[{name, path, created_date, document_status, closed_date, modified_at}]}` |
| `epic write <name> --from <body> [--data]` | `--data {specs?, sources?, spec?, confirm_completed_epic?}`; omitted fields keep their current value; a bare new name is given an ID | `{name, path, specs, linked:[…], unlinked:[…]}` |
| `epic delete <name>` | — | `{name, deleted, unlinked:[…]}` |
| `epic split --from <staged json>` | split description (below) | `{epic, created:[names], linked:[names], path}` |

Split description, staged under `.spektacular/tmp/`. It carries every section of every resulting
spec, because a split always acts on a complete spec and writes complete specs:

```json
{
  "spec": "000060_epics-and-seeded-specs",    // the spec being split (required)
  "overview": "…epic overview…",              // required when this creates the epic
  "sources": [{"uri": "…"}],                   // provenance moving to the epic
  "confirm_completed_epic": false,             // required true to add to a completed epic
  "specs": [
    {"name": "000060_epics-and-seeded-specs", "depends_on": [],
     "body": {"overview": "…", "requirements": ["…"], "acceptance_criteria": ["…"],
              "constraints": ["…"], "technical_approach": ["…"], "success_metrics": ["…"],
              "non_goals": ["…"]}},                                        // the narrowed spec
    {"title": "epic-split", "depends_on": ["000060_epics-and-seeded-specs"],
     "body": { /* every section, as above */ }, "sources": [{"uri": "…"}]}  // a new spec, by title
  ]
}
```

A new spec's `depends_on` may name another new spec by its `title`. The CLI rewrites every title to
the allocated name before it validates and writes. Each section is a list of items rendered as
that section's bullets or checkboxes, except `overview`, which is prose. `overview` and at least
one acceptance criterion are required for every spec. When the spec being split already belongs to
an epic, the new specs join that epic.

**Status report.** The one shape `status` emits as JSON, which the pretty renderer also draws from.

```go
type Report struct {
    Workflow  *WorkflowInfo `json:"workflow"`  // null when none in progress, or not one of the reported docs
    Requested string        `json:"requested,omitempty"`
    Epic      *EpicStatus   `json:"epic"`      // null for a standalone spec
    Specs     []SpecStatus  `json:"specs"`
}
type WorkflowInfo struct { Kind, Name, CurrentStep string; CompletedSteps []string; UpdatedAt string }
type EpicStatus struct {
    Name, DocumentStatus, CreatedAt string
    Done     bool
    Progress struct{ SpecsImplemented, SpecsTotal, TasksCompleted, TasksTotal int }
    Sources  []SourceRef
}
type SpecStatus struct {
    Name, State, DocumentStatus, CurrentStep string
    DependsOn, BlockedBy []string
    Ready    bool
    Sources  []SourceRef // effective: the spec's own, then its epic's
    Plan     *PlanStatus // null when no plan exists
}
type PlanStatus struct {
    Name, DocumentStatus, CurrentStep string
    Progress *struct{ TasksCompleted, TasksTotal int } // absent for plans without task structure
    Tasks    []TaskStatus
}
type TaskStatus struct { // the former export task fields plus acceptance-criteria counts
    ID, Title string; Milestone int; Repo RepoRef; DependsOn []string
    Execution Execution; Completed bool; AcceptanceCriteria Criteria
}
```

**Spec classifier.** One function, used by `status`, the implement dependency check and the
completed-epic guard.

```go
type SpecState string // missing | stale | specified | planned | in_progress | implemented
func Classify(spec, plan, workflow) (state SpecState, progress TaskCounts)
func Describe(state, progress) string // "in progress (2/5 tasks complete)"
```

**Config additions.**

```yaml
epic_split_threshold: moderate        # strict | moderate | lenient
epic:
  provider: file
  strict_dependencies: false
  config:
    directory: epics
```

**Workflow inputs and data.**
- `spec new --data` gains `sources: [{uri}]` (stored in workflow data with `retrieved_date`
  stamped), `epic` (an existing epic to join), and `confirm_completed_epic`.
- `implement new --data` gains `override_dependencies: bool`. An accepted override is stored as
  `dependency_override: [{name, state}]`.

**New error codes.** `epic_complete`, `epic_invalid`, `epic_not_found`, `epic_membership_conflict`,
`epic_nested`, `epic_link_failed`, `epic_link_rollback_failed`, `epic_split_invalid`,
`spec_in_epic_delete`, `sources_invalid`, `dependencies_unmet`, `dependency_override_refused`,
`status_format_unsupported`.

## Implementation Detail

**New module boundaries.** The work adds three small packages, each pure over a store reader:
- a dependency-graph helper;
- the epic document package (type, parse/render, validation);
- the status builder (resolution, classification, report, pretty rendering).

The command layer becomes thin around them. `status`, `epic` and the implement check only load
config, build stores, call the package and write output. A developer looking for "what is a spec's
state?" or "is this epic valid?" finds one function, not three copies in `cmd`.

**Following the design-reference pattern for links.** Membership writes reuse the shape
`design ref add` established:
- write the owning document first, then the back-links;
- keep the original bytes of everything touched;
- restore them in reverse order on failure;
- report `*_link_failed` (retry) apart from `*_link_rollback_failed` (manual reconcile, naming
  every document);
- expose a package-level seam so tests can make the Nth write fail. Failure tests replace a file
  with a directory rather than using chmod, because CI runs as root.

The epic link writer differs from `design ref` in one way. It works on a set: it diffs the old and
new member lists, and validates every spec's current `epic` before it writes anything, so a
membership conflict refuses with no writes at all.

**Closed schema in lockstep.** `epic` and `sources` are added at every closed-schema site of the
shared metadata at once (struct, encode shape, decode shape with lenient list decoding, marshal,
unmarshal, update options, merge on fresh and existing writes). Each gets byte-exact "no key when
empty" guards beside the existing designs guard, so standalone specs written today stay
byte-identical.

**One more step on the linear path.** `split` sits between `verification` and `finished`. Because a
split always acts on a complete spec, a request made earlier is recorded in the working context
and handled there. The FSM gains no back-edges, and progress counts simply include one more step.

**Instructions written once, included twice.** The split check and split flow become partials. The
`split` step template includes both. The `spek-new` skill includes them in a section for splitting
a spec that is already written, with no workflow running. This is the first partial shared between
a step and a skill, and it relies on skills already rendering partials at install time.

**Seeding through template variables, not new steps.** The `new` and `interview` callbacks pass the
recorded sources and the epic being joined to the templates as extra variables. The
interview template then has a seeded branch: seed the work files, list gaps, ask only about gaps.
No step is added. A resumed session re-renders the same branch because the data lives in workflow
state.

**Hard break for the status commands.** The four retired commands are deleted rather than wrapped.
Their logic moves into the status builder, so the behaviour that tests pinned (strict stale
detection, matching live workflow state, acceptance-criteria counts, repo locations,
pretty/json) is re-pinned against `status`. The retired spellings join the instruction-surface
deny-list, so no template or skill can reintroduce them. `--format` validation and the pretty
renderer move with them, and errors stay JSON.

**Implement pre-checks keep their ordered pipeline.** The dependency check slots into the existing
sequence (plan exists, stale plan, task startable, dependencies, start gate, clear state, start).
That keeps the rule that a refusal writes nothing. The warn path is a refusal with an explicit
override flag, so the CLI stays non-interactive.

**Schema bump.** One registered migration step and one constant bump. Shared test helpers write
current-format configs, so most fixtures follow automatically. Hard-coded schema numbers and the
migrate goldens are updated by hand. After the bump this repo's own config needs migrating before
`go run .` works here again. The implementer asks the user before running it.

**Docs.** One new page composed from existing section components, plus edits to existing pages.
No new components are needed.

## Dependencies

- **Design document `epics-and-seeded-specs.md` from the `design` design source.** The settled
  design this plan implements: epic document format, split triggers and flow, dependency model and
  implement check, the `status` command's output and resolution rules, seeding and provenance, and
  the decisions taken. Binding; not changed by this plan.
- **Spec `000060_epics-and-seeded-specs`.** Requirements, acceptance criteria and constraints this
  plan carries out.
- **`internal/metadata` (spektacular).** Shared spec/plan/changelog frontmatter. Changes: gains
  `epic` and `sources`.
- **`internal/plantask` (spektacular).** Plan parsing, task model, export and pretty rendering, graph
  validation. Changes: its cycle finder moves to the new graph helper. Parsing and export are
  reused by the status builder unchanged.
- **`internal/config`, `internal/migrate` (spektacular).** Config model and the per-kind migration
  registry. Changes: the `epic` section, `epic_split_threshold`, and a schema-3-to-4 step.
- **`internal/store`, `internal/artifact`, `internal/identifier` (spektacular).** File store (creates
  folders on write), addressing, and spec ID allocation. Changes: the `epic` artifact kind. The
  others are reused as they are.
- **`internal/workflow`, `internal/stepkit`, `internal/steps/{spec,implement}` (spektacular).** FSM,
  instruction rendering, partials and step callbacks. Changes: the `split` step, new callback extras
  and the implement data.
- **`internal/autocommit` (spektacular).** Commit-point table. Changes: the spec completion point
  moves to `split -> finished`.
- **`internal/agent` (spektacular).** Skill and managed-section installers and template guards.
  Changes: deny-list entries and the store-access command list.
- **Existing `design ref` link writer and `design delete` guard (spektacular).** The pattern the epic
  link writer and the spec-delete guard follow. Not changed.
- **`gopkg.in/yaml.v3`, `github.com/looplab/fsm`, `github.com/spf13/cobra`, `cbroglie/mustache`.**
  Existing libraries for frontmatter, the FSM, commands and templates. No new external dependency.
- **Harbor end-to-end suites (spektacular `tests/harbor`).** Run manually, not in CI. They must be
  updated and run for the spec workflow and plan workflow changes.
- **Docs site (docs repo, Astro 5 + MDX).** Hosts the documentation changes. No new components or
  libraries.
- **Prior plans relied on, all landed:** `000058_plan-task-graph` (task model, export, implement
  pre-check ordering), `000055_design-authoring-skill` (back-link pattern),
  `000056_store_delete_and_knowledge_maintenance` (delete-while-referenced guard),
  `000053_config-schema-versioning-and-migrations` (migration engine), `000057_git-commit` (commit
  points and start gate), `000059_normalise-artifact-addressing` (addressing and the hard-break
  deny-list).
- **Must happen during implementation:** after the schema bump lands, this repository's own
  `.spektacular/config.yaml` must be migrated (with the user's go-ahead) before `go run .` works
  here again. Until then, drive the workflow with a binary built before the bump.
- **Related but not a dependency:** issue #62 (parallel implement runs) builds on the epic
  dependency graph later. PR #65 (research seeding) stays separate.

## Testing Approach

Testing follows the project's three layers:
- Go unit and command tests, run shuffled;
- template-contract phrase tests over the step templates, partials and skills;
- the harbor end-to-end suites, which drive a real agent and are run manually.

The CLI half of the feature is deterministic and is pinned by Go tests. The agent-behaviour half
(split offers, seeding, gap lists, chaining, the epic question) cannot be asserted by Go. It is pinned two ways:
contract tests guarantee the instructions say the right thing, and the harbor runs or the
implementation test plan check that an agent does it.

**Unit tests (most coverage).**
- *Epic package:* parse/render round-trips; `depends_on: []` is always written; graph validation
  refuses each invalid shape (missing name or `depends_on`, duplicate, unknown dependency, self-cycle,
  longer cycle) and names the problem.
- *Dependency-graph helper:* the existing plan-task validation tests stay green after the extraction.
- *Shared metadata:* `epic` and `sources` round-trip, are carried forward by a body-only rewrite,
  are cleared by an empty update, and leave no key when empty. A spec with neither stays
  byte-identical.
- *Status builder:* every spec state in the classifier's order (`missing` does not hide the rest of
  the epic); `ready` / `blocked_by`; the epic roll-up and `done` (true only when every spec has a
  plan with every task complete); resolution of an epic, a member spec, a member's plan, a
  standalone spec and an unknown name; the same top-level shape for each; effective sources; and the
  pretty tree with the requested spec expanded.
- *Config and migration:* defaults and validation for the new keys; schema-3-to-4 adds both blocks
  to an existing config, keeps existing values, and a second apply changes nothing.

**Command tests (integration through the root command).**
- *`epic write` / `split` / `delete`:*
  - after a split, the epic holds only Overview and Specs;
  - an epic can be written with no specs, and a bare new name gets an ID;
  - adding to a completed epic is refused with nothing written, and succeeds with confirmation, after which the epic is no longer done;
  - every listed spec names the epic, and every spec naming it is listed;
  - delete clears every link;
  - joining a second epic, or nesting an epic, is refused with nothing written;
  - an injected mid-write failure restores every document;
  - the epics folder is created on first write;
  - a split with counter IDs allocates distinct names;
  - every resulting spec is `final` with every section given, including a non-empty Overview and at least one acceptance criterion;
  - every requirement and criterion of the description lands in exactly one spec, and shared constraints appear in each spec that lists them;
  - the split spec is rewritten as its narrowed self;
  - a split of a spec already in an epic extends that epic.
- *`spec new`:* `sources` are recorded with today's date; a spec started without sources records
  none; an invalid `sources` entry is refused; a spec started with `epic` is listed by that epic and
  names it; an unknown epic is refused with nothing written; a completed epic needs confirmation.
- *`spec file delete`:* refuses while the spec is in an epic.
- *`status`:*
  - the same epic for any member name or plan name, with `requested` set;
  - a standalone spec has `epic: null` and one spec;
  - with no name, the workflow in progress, or `{"workflow": null}`;
  - json and pretty carry the same information;
  - an unsupported format is refused;
  - the strict-stale and live-step behaviours previously pinned on the retired commands are re-pinned here.
- *`implement new`:*
  - a standalone spec, or one with all dependencies implemented, starts silently;
  - an unmet dependency is refused with each dependency named with its state ("in progress (2/5 tasks complete)");
  - re-running with the override starts and records the override in workflow data;
  - under strict dependencies the override is refused, and the next action names the first ready dependency;
  - a refusal writes no state.
- *Specifying and planning:* a spec whose dependencies are unimplemented runs both workflows with no warning.
- *Retired commands:* `spec status`, `plan status`, `implement status` and `plan export` no longer
  exist, and the subcommand pins are updated.
- *Regression:* the existing spec, plan and implement workflow tests pass unchanged, apart from the
  added `split` step in the spec FSM walk. This covers the "no change without epics" criterion.

**Template-contract tests.**
- The `split` step and the `spek-new` skill both include the split-check and split-flow partials.
- The split check contains the gate, the strong and weak signals, the counter-signals, the
  supporting-work rule and the live `epic_split_threshold` read with its three levels.
- The flow requires explicit agreement and states the re-offer rule.
- The interview template has a seeded branch that lists gaps and asks only about them.
- The section templates present a pre-filled draft to confirm.
- `finished` carries the chaining offer over the epic's source items.
- The `split` step runs one review over every resulting spec, and a split request made mid-workflow
  is recorded and acted on at `split`.
- The interview reads the epic and its specs when the spec joins one, and the skill asks about an
  epic only when the project has epics.
- The changelog steps render a recorded dependency override.
- `spek-new` covers recognising a source (number, link, "spec from …"), fetching it with the
  agent's own tools, child items, an unreachable source, and late child items.
- `spek-implement` and the implement help say "spec".
- No template or skill contains a retired command spelling.
- `store-access` names the `epic` verbs.

**End-to-end (harbor).** The spec-workflow suite gains the `split` step in its oracles and solve
script. The plan-workflow suite's export check moves to `status`. Each touched suite is run once
before the work is called done.

**Success metrics.**
- *A seeded spec asks only about the sections the issue left out.* Contract tests cover the
  instructions (the seeded interview branch, the gap list, confirm-don't-ask in later steps). The
  outcome, that no question re-asks what the issue answered, is **Manual — captured in the
  implementation test plan**.
- *Code plus its docs, tests or config never receives a split offer.* A contract test checks that
  the split check states the supporting-work rule and counter-signals, and that thresholds never
  override them. The agent's actual behaviour across scenarios is **Manual — captured in the
  implementation test plan**.
- *One command shows where an epic stands.* **Behavioural test**: `status` on any member returns
  every spec's state, blockers and task progress in one result, and the retired commands no longer
  exist.

**Deliberate gaps.** There are no Go tests of agent judgement (split detection, seeding quality),
and no browser tests of the docs site. The docs are guarded by the MDX grep rule, `npm run build`
and `astro check`.

## Milestones & Tasks

### Milestone 1: Specs can be grouped into epics

**What changes**: Every project, new or migrated, has an epic store configured, plus a split
sensitivity setting. Epics can be written, read, listed, deleted and created by splitting through
Spektacular's own commands. An epic holds only an overview and its specs with their dependencies,
and the CLI refuses invalid dependency graphs, a spec joining a second epic, and nested epics.
Specs record their epic and their sources, and both survive every later rewrite. An epic and its
specs always agree about membership, including after a delete or a failed write. A project that
never uses epics behaves exactly as before.

**Validation point**: Go tests for the epic package, graph validation, metadata, config and
migration, and the epic commands pass. Migrating an existing project twice adds the `epic` block
once. `go test ./...` is green.

#### - [x] Task: Extract the dependency graph helper
**Id:** bfb8db69-17ea-43f3-9437-d16001d0e03c
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Moves cycle detection out of the plan-task validator into a small shared helper that works on any
named nodes with declared dependencies. Plan validation keeps behaving exactly as it does today, and
epic validation can use the same algorithm later instead of a second copy.

*Technical detail:* [context.md#task-extract-the-dependency-graph-helper](./context.md#task-extract-the-dependency-graph-helper)

**Acceptance criteria**:
- [x] Plans with dependency cycles are still refused with the same error and cycle path as before
- [x] The helper reports a cycle (including a node depending on itself) for any set of named nodes, and reports none for an acyclic set

#### - [x] Task: Record epic and sources on specs
**Id:** e8829ac7-4927-4149-a563-f6130257b7c3
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds two fields to a spec's frontmatter: the epic it belongs to, and the sources that directly
seeded it (each a link and the date it was retrieved). Like design references, both are carried
forward whenever a spec is rewritten, so nothing written later can silently drop them. A spec with
neither field is written exactly as it is today.

*Technical detail:* [context.md#task-record-epic-and-sources-on-specs](./context.md#task-record-epic-and-sources-on-specs)

**Acceptance criteria**:
- [x] A spec's `epic` and `sources` survive a body-only rewrite of the spec
- [x] An update can set, replace or clear each field explicitly
- [x] A spec with no epic and no sources renders byte-identically to today, with neither key present
- [x] Malformed `sources` entries read as absent rather than failing the read

#### - [x] Task: Add the epic store settings
**Id:** b22b59e2-9b53-4c4c-98cf-1733664c2f03
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds the `epic` store section (provider, whether unmet dependencies are refused, and its folder)
and the `epic_split_threshold` sensitivity setting to the project configuration, with defaults and
validation. It also adds `epic` as a document kind for addressing. New projects get both from
`init`. The epics folder is not created up front but on the first write.

*Technical detail:* [context.md#task-add-the-epic-store-settings](./context.md#task-add-the-epic-store-settings)

**Acceptance criteria**:
- [x] A newly initialised project's configuration contains the `epic` section with `provider: file`, `strict_dependencies: false` and directory `epics`, plus `epic_split_threshold: moderate`
- [x] An invalid provider or threshold value is refused with a message saying what is allowed
- [x] `init` does not create an epics folder
- [x] The epic folder setting is resolved relative to the configuration file like other store folders

#### - [x] Task: Add the epic document and its graph validation
**Id:** 23f76514-3bbb-46a7-90c1-30ee64778941
**Repo:** spektacular
**Depends on:**
- bfb8db69-17ea-43f3-9437-d16001d0e03c — Extract the dependency graph helper
- e8829ac7-4927-4149-a563-f6130257b7c3 — Record epic and sources on specs
**Execution:** agent

Introduces the epic as its own document type: lifecycle fields, the originating spec, the list of
specs with their dependencies, and sources. It reads and writes that frontmatter and validates the
dependency graph. It is kept separate from spec frontmatter so an epic's spec list can never be
lost by a spec-style rewrite.

*Technical detail:* [context.md#task-add-the-epic-document-and-its-graph-validation](./context.md#task-add-the-epic-document-and-its-graph-validation)

**Acceptance criteria**:
- [x] An epic with dependencies reads back with exactly the same specs, order and dependencies it was written with
- [x] `depends_on` is always written, as an empty list when a spec has no dependencies
- [x] A graph with a missing name or dependency list, a duplicate spec, an unknown dependency, or a cycle (including self-dependency) is refused with an error naming the problem
- [x] Created and closed dates are stamped and preserved the same way as for other documents

#### - [x] Task: Add the epic commands and membership links
**Id:** 2746958b-631c-409d-8772-c14ca33612d9
**Repo:** spektacular
**Depends on:**
- 23f76514-3bbb-46a7-90c1-30ee64778941 — Add the epic document and its graph validation
- b22b59e2-9b53-4c4c-98cf-1733664c2f03 — Add the epic store settings
**Execution:** agent

Adds `epic read`, `epic write`, `epic list` and `epic delete`. Writing or deleting an epic keeps
each member spec's `epic` field in agreement with the epic's list. If any write fails part-way,
every document touched is restored. A spec cannot join a second epic, an epic cannot be a member of
an epic, and a spec cannot be deleted while it still belongs to an epic. The agent store-access
rules name the new commands.

*Technical detail:* [context.md#task-add-the-epic-commands-and-membership-links](./context.md#task-add-the-epic-commands-and-membership-links)

**Acceptance criteria**:
- [x] After an epic is written, every spec it lists names it, and every spec that names it is listed
- [x] Removing a spec from an epic's list clears that spec's `epic`. Deleting an epic leaves no spec naming it
- [x] Adding a spec that already belongs to another epic, or listing an epic as a member, is refused with an error and nothing is written
- [x] If a write fails part-way, the epic and every spec involved are restored to their previous content
- [x] Writing the first epic in a project with no epics folder creates the folder and succeeds
- [x] Deleting a spec that belongs to an epic is refused, naming the epic and how to remove the spec from it first
- [x] Every refusal states the problem and gives a runnable next step

#### - [x] Task: Split into an epic from the command line
**Id:** 41e74e4b-76c3-4e3e-81f9-d764041509e9
**Repo:** spektacular
**Depends on:**
- 2746958b-631c-409d-8772-c14ca33612d9 — Add the epic commands and membership links
**Execution:** agent

Adds `epic split`, which reads one staged description of a complete spec divided into several, and
does the whole split in a single operation. It allocates IDs for the new specs, writes each of them
complete and final with every section it was given, and rewrites the split spec as its narrowed
self. It then writes or extends the epic and links everything both ways, restoring every document
if anything fails part-way.

*Technical detail:* [context.md#task-split-into-an-epic-from-the-command-line](./context.md#task-split-into-an-epic-from-the-command-line)

**Acceptance criteria**:
- [x] Splitting a standalone spec produces one epic, named after that spec, listing it and the new specs with their dependencies
- [x] The stored epic contains only an overview and its specs list, with no requirements, criteria, constraints, non-goals, technical approach or metrics
- [x] Every resulting spec is complete and final, with every section it was given, a non-empty overview and at least one acceptance criterion
- [x] Every requirement and acceptance criterion in the description appears in exactly one resulting spec, and a constraint shared by several appears in each of them
- [x] Splitting a spec that already belongs to an epic adds the new specs to that same epic
- [x] A description with fewer than two specs, a spec with no acceptance criteria, or an invalid graph is refused and nothing is written
- [x] With counter IDs, the new specs get distinct, sequential names

#### - [x] Task: Migrate existing projects to the epic store
**Id:** 5238912a-7633-4f3a-bf68-92eec733677a
**Repo:** spektacular
**Depends on:**
- b22b59e2-9b53-4c4c-98cf-1733664c2f03 — Add the epic store settings
- 41e74e4b-76c3-4e3e-81f9-d764041509e9 — Split into an epic from the command line
**Execution:** agent

Adds a project configuration migration that writes the `epic` section and `epic_split_threshold`
into existing projects, and raises the configuration schema version so projects are prompted to
migrate. Test fixtures that pin the old schema number are updated. This lands after the rest of
the epic store, because once it does, this repository's own configuration must be migrated before
the CLI runs here again.

*Technical detail:* [context.md#task-migrate-existing-projects-to-the-epic-store](./context.md#task-migrate-existing-projects-to-the-epic-store)

**Acceptance criteria**:
- [x] Migrating an existing project adds the `epic` section and `epic_split_threshold` with their defaults and keeps every existing setting
- [x] Migrating a second time changes nothing
- [x] A project still on the previous schema is told to run `migrate`
- [x] The full test suite passes

#### - [x] Task: Migrate this repository's own configuration
**Id:** 99ad63fd-21f0-42ef-9ec5-6c2e55f9f22c
**Repo:** spektacular
**Depends on:**
- 5238912a-7633-4f3a-bf68-92eec733677a — Migrate existing projects to the epic store
**Execution:** human — needs the user's go-ahead to rewrite this repository's own project configuration

Once the schema bump has landed, the CLI refuses to run in this repository until its own
configuration is migrated. The user reviews a dry run and approves the migration, so the rest of
the implementation can keep driving the workflow with the CLI.

*Technical detail:* [context.md#task-migrate-this-repositorys-own-configuration](./context.md#task-migrate-this-repositorys-own-configuration)

**Acceptance criteria**:
- [x] This repository's configuration is on the new schema and contains the `epic` section
- [x] The CLI runs in this repository again

### Milestone 2: One status command, and implementation checks dependencies

**What changes**: `spektacular status` shows a whole piece of work in one result: the epic, every
spec's state and what blocks it, and every plan's tasks. It is the same shape whether the name is an
epic, a spec or a plan, and with no name it shows the workflow in progress. It is available as
readable text and as JSON. `spec status`, `plan status`, `implement status` and `plan export` are
gone, and everything that called them now uses `status`. Starting implementation of a spec whose
dependencies are not yet implemented names each one and its state, then lets the user continue
(recording that they did) or, if the project requires it, refuses. Implementation is described as
implementing a spec.

**Validation point**: Status, implement and retired-command tests pass. The plan-workflow harbor
suite passes with its export check moved to `status`. `go test ./...` is green.

#### - [x] Task: Build the status report
**Id:** e343da99-f4f8-4da1-adf7-38c4a9984f77
**Repo:** spektacular
**Depends on:**
- 23f76514-3bbb-46a7-90c1-30ee64778941 — Add the epic document and its graph validation
- 99ad63fd-21f0-42ef-9ec5-6c2e55f9f22c — Migrate this repository's own configuration
**Execution:** agent

Builds the single status report in one place. It resolves an epic, spec or plan name to the whole
epic, or to a standalone spec. It classifies every spec's state, works out readiness and blockers,
lists every plan's tasks with their acceptance-criteria counts, rolls up the epic's progress and
completion, and shows each spec's own sources followed by its epic's. It also renders the readable
tree. The status command, the implement dependency check and the completed-epic guard all use it, so
they always agree.

*Technical detail:* [context.md#task-build-the-status-report](./context.md#task-build-the-status-report)

**Acceptance criteria**:
- [x] An epic name, any of its specs' names and any of their plans' names all resolve to the same epic, with the requested name identified
- [x] A standalone spec reports the same top-level shape with no epic and a single spec
- [x] Each spec is classified as missing, stale, specified, planned, in progress or implemented, and a missing spec does not hide the rest of the epic
- [x] An epic is reported done exactly when every spec has a plan whose tasks are all complete
- [x] A spec in an epic shows its own sources followed by its epic's, while its stored record holds only its own
- [x] The readable tree expands the requested spec down to its tasks and shows its siblings on one line each

#### - [x] Task: Replace the status and export commands with status
**Id:** 6960c99f-4530-40a6-b9a1-18f79862ef80
**Repo:** spektacular
**Depends on:**
- e343da99-f4f8-4da1-adf7-38c4a9984f77 — Build the status report
**Execution:** agent

Adds the top-level `status [name] [--format pretty|json]` command and removes `spec status`,
`plan status`, `implement status` and `plan export` outright. Everything that used them moves to
`status` in the same change: the implement skill, the implement task hints, the README and the
plan-workflow harbor suite. Templates and skills are guarded against the old spellings.

*Technical detail:* [context.md#task-replace-the-status-and-export-commands-with-status](./context.md#task-replace-the-status-and-export-commands-with-status)

**Acceptance criteria**:
- [x] `status <name>` returns the report as readable text by default and as structured data with `--format json`, with the same information in both
- [x] `status` with no name reports the workflow in progress, or reports that nothing is in progress
- [x] An unknown name or an unsupported format is refused with a runnable next step
- [x] `spec status`, `plan status`, `implement status` and `plan export` no longer exist, and no template, skill or README presents them
- [x] The implement skill and task-refusal hints point at `status` for task ids

#### - [x] Task: Check spec dependencies when implementation starts
**Id:** 0a3c2970-1993-42c1-bc2a-c298b88e2e94
**Repo:** spektacular
**Depends on:**
- e343da99-f4f8-4da1-adf7-38c4a9984f77 — Build the status report
**Execution:** agent

Before implementation of a spec in an epic starts, checks each spec it depends on. Any that are
not yet implemented are named along with their state. By default the user can choose to continue,
and that override is carried into the changelog. When the project requires strict dependencies,
starting is refused instead, and the user is pointed at the first dependency that is ready to
implement. Implementation is described as implementing a spec throughout, with no change in
behaviour.

*Technical detail:* [context.md#task-check-spec-dependencies-when-implementation-starts](./context.md#task-check-spec-dependencies-when-implementation-starts)

**Acceptance criteria**:
- [x] Implementing a standalone spec, or one whose dependencies are all implemented, starts with no warning
- [x] Starting a spec with an unmet dependency names each one and its state, for example "in progress (2/5 tasks complete)", and starts nothing
- [x] With default settings, choosing to continue starts implementation and the override is recorded in the changelog
- [x] With strict dependencies, starting is refused even when continuing is requested
- [x] Specifying and planning a spec whose dependencies are unimplemented are never warned or blocked
- [x] The implement command's help, input description and skill say a spec is being implemented

### Milestone 3: Specs can be split, grouped from the start, and started from existing material

**What changes**: When a spec is complete, or when the user asks (in which case at completion), the
agent checks whether it is more than one independently useful piece of work. If so, it offers a
concrete split and never acts without agreement. An accepted split turns the complete spec into an
epic of complete specs in one step, with no further interviews. Work that starts as a set of items,
such as a tracker epic with sub-issues, gets an epic first, and each item is then specified as its
own spec in it, with the next item offered when one finishes. A spec can be started from an issue,
tracker epic, design document, file, web page or pasted text, however the request is worded. The
source pre-fills the spec, the interview asks only about the gaps, and the spec records where its
content came from. In a project with epics, a new spec can join one, and the agent reads the epic
and its specs first. Adding to a completed epic needs confirmation.

**Validation point**: Spec workflow, step, epic and template-contract tests pass, including the FSM
walk through `split`. The spec-workflow harbor suite passes with the new step. The manual checks
for the seeding and split-offer metrics are listed in the implementation test plan. `go test ./...`
is green.

#### - [x] Task: Start a spec with sources or in an epic
**Id:** 31c5ced7-fb2d-4af9-982a-f52f33e08318
**Repo:** spektacular
**Depends on:**
- e8829ac7-4927-4149-a563-f6130257b7c3 — Record epic and sources on specs
- 2746958b-631c-409d-8772-c14ca33612d9 — Add the epic commands and membership links
- 99ad63fd-21f0-42ef-9ec5-6c2e55f9f22c — Migrate this repository's own configuration
**Execution:** agent

Lets `spec new` accept the sources a spec is started from, stamping each with today's date and
recording them on the spec. It also accepts an existing epic to join: the spec is linked to that
epic as soon as it is created, so the epic lists it from the start. The sources and the epic are
passed to the interview step so it knows to seed and to read the epic first.

*Technical detail:* [context.md#task-start-a-spec-with-sources-or-in-an-epic](./context.md#task-start-a-spec-with-sources-or-in-an-epic)

**Acceptance criteria**:
- [x] A spec started with sources records each source's link and retrieval date, and a spec started without any records none
- [x] A source entry with no link is refused with the correct shape in its next step
- [x] A spec started in an epic is listed by that epic from the start and names it
- [x] Starting a spec in an epic that does not exist is refused and nothing is written
- [x] Starting a spec without sources or an epic behaves exactly as before

#### - [x] Task: Guard additions to a completed epic
**Id:** 9cdca194-2428-4e62-8c2c-3fb99a6c8efb
**Repo:** spektacular
**Depends on:**
- 31c5ced7-fb2d-4af9-982a-f52f33e08318 — Start a spec with sources or in an epic
- 41e74e4b-76c3-4e3e-81f9-d764041509e9 — Split into an epic from the command line
- e343da99-f4f8-4da1-adf7-38c4a9984f77 — Build the status report
**Execution:** agent

Adding a spec to an epic whose specs are all implemented now needs confirmation, whether the spec
is started in the epic, listed by `epic write`, or created by `epic split`. Without confirmation
the addition is refused with a warning naming the epic as complete. With it, the spec is added and
the epic is reported as in progress again until the new spec is implemented.

*Technical detail:* [context.md#task-guard-additions-to-a-completed-epic](./context.md#task-guard-additions-to-a-completed-epic)

**Acceptance criteria**:
- [x] Adding a spec to a completed epic by any of the three routes is refused with a warning naming the epic, and nothing is written
- [x] Repeating the addition with confirmation adds the spec, and the epic's status is no longer done
- [x] Adding to an epic that is not complete needs no confirmation

#### - [x] Task: Add the split step and chaining to the spec workflow
**Id:** bf23f8d2-1eb3-47af-9472-4485cdc984a1
**Repo:** spektacular
**Depends on:**
- 41e74e4b-76c3-4e3e-81f9-d764041509e9 — Split into an epic from the command line
- 31c5ced7-fb2d-4af9-982a-f52f33e08318 — Start a spec with sources or in an epic
**Execution:** agent

Adds a `split` step between verification and finished, so the split check runs once on every
complete spec. A split asked for earlier in the workflow is noted and handled there. The split
check and split flow are written once, as shared instruction fragments. They cover the gate, the
signals, the supporting-work rule, the counter-signals, the configured sensitivity, agreement,
redistributing the content, one review over every resulting spec, and provenance hand-over. When a
spec in an epic finishes, the agent offers to start a spec for the next item in the epic's source
that has none yet.

*Technical detail:* [context.md#task-add-the-split-step-and-chaining-to-the-spec-workflow](./context.md#task-add-the-split-step-and-chaining-to-the-spec-workflow)

**Acceptance criteria**:
- [x] Every spec workflow passes through the split check before finishing, and a split requested earlier is acted on there
- [x] The split instructions offer a split only when at least two specs, each with independently verifiable criteria, can be named, and say that docs, tests, migrations and config never count towards one
- [x] The instructions never split without the user's agreement, and repeat an offer after a decline only when a new independent requirement group appears
- [x] The split sensitivity is read from `epic_split_threshold`, separately from the spec-trigger setting, and never overrides the supporting-work rule or counter-signals
- [x] An accepted split reviews every resulting spec once and writes them complete, with no further interview
- [x] When a spec in an epic finishes and the epic's source has an item with no spec yet, the agent offers to start one for it
- [x] The completed spec, and any epic and specs a split wrote, are committed together when auto-commit is on

#### - [x] Task: Seed specs from existing material
**Id:** cbdc1ee8-c3fd-4b7b-9ffe-e9697afe9f2a
**Repo:** spektacular
**Depends on:**
- 31c5ced7-fb2d-4af9-982a-f52f33e08318 — Start a spec with sources or in an epic
- bf23f8d2-1eb3-47af-9472-4485cdc984a1 — Add the split step and chaining to the spec workflow
- 9cdca194-2428-4e62-8c2c-3fb99a6c8efb — Guard additions to a completed epic
**Execution:** agent

Teaches the agent to start a spec from existing material, however the request is worded, and to
place it in an epic. The `spek-new` skill:
- asks whether a new spec belongs to an existing epic (only when the project has epics);
- recognises the source and fetches it with the agent's own tools: title, body, discussion, child
  items and a stable link;
- asks for the content if the source cannot be reached;
- for a source with child items, creates the epic and specifies each child in turn;
- proposes a name, and can split a spec that is already written.

The interview step reads the epic and its specs when the spec joins one. It pre-fills the section
drafts from the source, lists the gaps and asks only about those, and every later step confirms its
draft.

*Technical detail:* [context.md#task-seed-specs-from-existing-material](./context.md#task-seed-specs-from-existing-material)

**Acceptance criteria**:
- [x] The skill recognises a source given as a number, a link or "spec from …", and names no specific fetching tool or tracker
- [x] An unreachable source leads the agent to say so and ask for the content, not to start a blank interview
- [x] A source with child items leads to an offer to create an epic and specify each child in it, and a child item added later leads to an offer of a spec for it in that epic
- [x] In a project with epics, starting a spec asks whether it belongs to one, and in a project without epics it does not
- [x] A spec joining an epic has the agent read the epic and its specs before the interview, and offer to record dependencies on them
- [x] A seeded interview lists the uncovered sections and asks only about those, and later steps present their pre-filled drafts to confirm
- [x] Seeding instructions survive a resumed session because they live in the interview step

#### - [x] Task: Update the spec-workflow harbor suite
**Id:** 182ad440-4a84-4f4d-bb32-713d91c08754
**Repo:** spektacular
**Depends on:**
- cbdc1ee8-c3fd-4b7b-9ffe-e9697afe9f2a — Seed specs from existing material
**Execution:** agent

Brings the end-to-end spec workflow suite in line with the new `split` step: the expected step
order, the solve script's step sequence, and an assertion that a single coupled spec reaches
`finished` with no epic written.

*Technical detail:* [context.md#task-update-the-spec-workflow-harbor-suite](./context.md#task-update-the-spec-workflow-harbor-suite)

**Acceptance criteria**:
- [x] The suite expects `split` between verification and finished
- [x] The suite asserts that its single-feature scenario finishes with no epic stored

#### - [ ] Task: Run the harbor suites
**Id:** e4194c32-a882-4edf-bc42-3d12469d82c4
**Repo:** spektacular
**Depends on:**
- 182ad440-4a84-4f4d-bb32-713d91c08754 — Update the spec-workflow harbor suite
- 6960c99f-4530-40a6-b9a1-18f79862ef80 — Replace the status and export commands with status
- 0a3c2970-1993-42c1-bc2a-c298b88e2e94 — Check spec dependencies when implementation starts
**Execution:** human — the harbor suites need agent API credentials and are run outside CI

Runs the spec-workflow, plan-workflow and implement-workflow harbor suites against the finished
CLI, and records the results in the implementation test plan.

*Technical detail:* [context.md#task-run-the-harbor-suites](./context.md#task-run-the-harbor-suites)

**Acceptance criteria**:
- [ ] The spec-workflow, plan-workflow and implement-workflow suites pass

### Milestone 4: The documentation site explains epics, seeding and status

**What changes**: The documentation site has an epics page covering epics, splitting, split
sensitivity and dependencies. It explains starting a spec from existing material and documents the
single status view in place of the removed commands. The configuration reference covers `epic` and
`epic_split_threshold`, and the site describes implementation as implementing a spec. No page still
presents the removed commands as current.

**Validation point**: The site builds, `astro check` reports no errors or warnings, the MDX layout
guard finds nothing, and searching the site for the removed commands finds only historical
references.

#### - [x] Task: Document epics, splitting and dependencies
**Id:** 56ba7ec3-46b1-454a-9089-51b0c24b4d86
**Repo:** docs
**Depends on:**
- bf23f8d2-1eb3-47af-9472-4485cdc984a1 — Add the split step and chaining to the spec workflow
- cbdc1ee8-c3fd-4b7b-9ffe-e9697afe9f2a — Seed specs from existing material
- 0a3c2970-1993-42c1-bc2a-c298b88e2e94 — Check spec dependencies when implementation starts
**Execution:** agent

Adds an epics page to the documentation site, linked from the navigation. It explains what an
epic is, the two ways into one (splitting a complete spec, or creating the epic first), when a split
is offered and how sensitivity changes that, what a split produces, joining and adding to an epic,
chaining,
dependencies between specs and how implementation treats an unmet one, and the `epic` commands.

*Technical detail:* [context.md#task-document-epics-splitting-and-dependencies](./context.md#task-document-epics-splitting-and-dependencies)

**Acceptance criteria**:
- [x] The site has an epics page, reachable from the navigation, covering epics, splitting, the epic-first route, split sensitivity, joining an epic, chaining and dependencies
- [x] The page's examples use the real frontmatter fields, settings and commands
- [x] The page follows the site's layout and authoring rules and contains no em dashes

#### - [x] Task: Document the status view
**Id:** 5034726e-22fc-4895-8897-96849f9d7134
**Repo:** docs
**Depends on:**
- 6960c99f-4530-40a6-b9a1-18f79862ef80 — Replace the status and export commands with status
**Execution:** agent

Replaces the documentation of `plan export`, `plan status` and `implement status` with the single
`status` command: what it reports, its two output formats and its fields. Every other page that
mentions the removed commands is updated, including a migration table from each old command to
`status`.

*Technical detail:* [context.md#task-document-the-status-view](./context.md#task-document-the-status-view)

**Acceptance criteria**:
- [x] The site documents `status` with a readable and a structured example and its output fields
- [x] No page presents `spec status`, `plan status`, `implement status` or `plan export` as current
- [x] A table maps each removed command to its `status` replacement

#### - [x] Task: Document seeding, implementing a spec and the new settings
**Id:** b949fe14-171a-4ab6-a929-a62b750138a1
**Repo:** docs
**Depends on:**
- cbdc1ee8-c3fd-4b7b-9ffe-e9697afe9f2a — Seed specs from existing material
- 0a3c2970-1993-42c1-bc2a-c298b88e2e94 — Check spec dependencies when implementation starts
- 5238912a-7633-4f3a-bf68-92eec733677a — Migrate existing projects to the epic store
**Execution:** agent

Explains starting a spec from an issue, tracker epic, design document, file, web page or pasted
text, and what is recorded as its sources. Rewords the site to say a spec is implemented. The
configuration reference documents the `epic` section and `epic_split_threshold`. Adds a site
changelog entry for the release.

*Technical detail:* [context.md#task-document-seeding-implementing-a-spec-and-the-new-settings](./context.md#task-document-seeding-implementing-a-spec-and-the-new-settings)

**Acceptance criteria**:
- [x] The site explains starting a spec from existing material, the gap-only interview, and the `sources` it records
- [x] The configuration reference documents `epic.provider`, `epic.strict_dependencies`, `epic.config.directory` and `epic_split_threshold`, and its key count is correct
- [x] The site describes implementation as implementing a spec
- [x] The site builds, type-checks with no errors or warnings, and passes the layout guard

## Open Questions

- **Does a shared partial render the CLI prefix the same way in a step and in a skill?** Step
  templates use `{{config.command}}` and skills use `{{command}}`. Which variable the split partials
  can rely on in both contexts only becomes clear when they are first rendered in each. If neither
  variable is available in both, the implementer should pass the missing one in the renderer that
  lacks it, rather than writing the partial twice. If that would change installed skill output for
  other skills, STOP and ask the user.

There are no other open questions. Every other decision is recorded in the assumption log.

## Out of Scope

- Recognising spec-worthy discussion is unchanged. Split offers come only at spec completion or on an explicit request, never from open discussion. The AGENTS.md spec-trigger section is not touched (spec non-goal; design Decision 6).
- Plans stay one-to-one with specs. No plan spans several specs, and an epic is never planned or implemented itself (spec non-goal).
- No syncing with sources. Later changes to an issue are not pulled in, and nothing is written back or posted to the source (spec non-goal; design "Deferred": posting back).
- No storing a snapshot or content hash of the source text. Only the URI and retrieval date are recorded (spec technical approach).
- No fetching or tracker client in the CLI, and no CLI seeding surface beyond `sources` on `spec new` (spec constraints).
- No dependencies on specs outside the same epic or on standalone specs (spec non-goal).
- Chaining stops at specifying. There are no offers to plan or implement the next spec (spec non-goal; design Decision 5).
- A split never renames the epic or its first spec (spec non-goal; design Decision 2).
- No nested epics, and no spec in more than one epic.
- No concurrent workflows for an epic's specs. Only one workflow is active at a time; parallel implement runs belong to issue #62.
- Research seeding from Hive knowledge, ADRs and Context7 (PR #65) is not merged into source seeding.
- No `status --spec-only` flag. Callers pick the requested spec out of `specs` (design Decision 3).
- No compatibility aliases or deprecation period for `spec status`, `plan status`, `implement status` or `plan export` (spec constraint).
- The guided repo-add workflow's separate state is not reported by `status`.
- A glossary entry for "epic" in the knowledge base is not part of the implementation tasks. It is offered to the user separately through the knowledge workflow.

## Changelog


### 2026-10-01 — Task: Extract the dependency graph helper

**What was done**: Moved cycle detection out of the plan-task validator into a new `internal/depgraph` package (`FindCycle(order, deps)`), which works on any named nodes and returns the cycle path with its first node repeated. `plantask.Validate` now builds the id order and dependency map and maps the returned ids back to task titles, so its errors are unchanged.

**Deviations**: None

**Files changed**:
- `spektacular: internal/depgraph/depgraph.go`
- `spektacular: internal/depgraph/depgraph_test.go`
- `spektacular: internal/plantask/validate.go`

**Discoveries**: `FindCycle` ignores dependencies on names outside `order`, so callers must report unknown dependencies before calling it (both plantask and the coming epic validator do).

### 2026-10-01 — Task: Record epic and sources on specs

**What was done**: Added `Epic` and `Sources []SourceRef` (`{uri, retrieved_date}`) to the shared frontmatter metadata at every closed-schema site, with lenient decoding of `sources`. `UpdateOptions` gained tri-state `Epic *string` and `Sources *[]SourceRef`, which `Merge` applies on fresh writes and carries forward on existing ones, so a body-only spec rewrite keeps both.

**Deviations**: None

**Files changed**:
- `spektacular: internal/metadata/metadata.go`
- `spektacular: internal/metadata/merge.go`
- `spektacular: internal/metadata/metadata_test.go`
- `spektacular: internal/metadata/merge_test.go`

**Discoveries**: `epic` decodes as a plain string like `spec` and `plan`, so a hand-edited non-scalar `epic:` value fails the read rather than reading as empty; only `sources` (like `designs`) is lenient.

### 2026-10-01 — Task: Add the epic store settings

**What was done**: Added the `epic` store section (`provider`, `strict_dependencies`, `config.directory`, default `epics`) and `epic_split_threshold` (strict / moderate / lenient, default moderate) to the project configuration, with defaults in `NewDefault`, load-time prefill, validation that names the allowed values, and the epic directory in `storeDirs` so it resolves relative to `config.yaml` and is refused outside the project. Added `artifact.KindEpic`. `init` writes both settings and creates no epics folder.

**Deviations**: None

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/config/storedir_test.go`
- `spektacular: internal/artifact/address.go`
- `spektacular: internal/artifact/address_test.go`
- `spektacular: cmd/init_test.go`

**Discoveries**: Without a schema bump, an existing schema-3 project already loads the epic defaults (the loader prefills them), so the CLI keeps working here until the migration task bumps the schema.

### 2026-10-01 — Task: Add the epic document and its graph validation

**What was done**: Added the `internal/epic` package: the epic's own frontmatter type (`created_date`, `document_status`, `closed_date`, `spec`, `specs[{name, depends_on}]`, `sources`), `Parse`, `Render` (always writing `depends_on`, as `[]` when empty, and `specs`), `Stamp` for lifecycle dates with the same rules as `metadata.Merge`, and `Validate`, which refuses a missing name or `depends_on`, a duplicate, an unknown dependency or a cycle with `epic_invalid` naming the spec. Cycle detection uses `depgraph.FindCycle`.

**Deviations**: To avoid duplicating fence handling and lifecycle rules, `internal/metadata` now exports `SplitRaw` (which `Split` delegates to), `IsClosed`, `ValidateDocumentStatus` and `DateFormat`.

**Files changed**:
- `spektacular: internal/epic/epic.go`
- `spektacular: internal/epic/validate.go`
- `spektacular: internal/epic/epic_test.go`
- `spektacular: internal/epic/validate_test.go`
- `spektacular: internal/metadata/frontmatter.go`
- `spektacular: internal/metadata/metadata.go`
- `spektacular: internal/metadata/metadata_test.go`

**Discoveries**: yaml.v3 decodes an absent `depends_on` to a nil slice and `depends_on: []` to a non-nil empty one, which is how Validate tells "missing" from "none". Callers building an `EpicSpec` by hand must pass `[]string{}`, not nil.

### 2026-10-01 — Task: Add the epic commands and membership links

**What was done**: Added the hand-written `epic read / write / list / delete` command family (`cmd/epic.go`) and the membership link writer (`cmd/epic_link.go`). `epic write` takes the body via `--from` and `specs` / `sources` / `spec` via `--data` (omitted fields keep their value), names a bare new epic with the configured ID method run against the epic directory, validates the graph, refuses an unknown spec, a spec in another epic or a nested epic before writing anything, then writes the epic and sets or clears each spec's `epic`. Every write goes through a small `docTxn` that restores all touched documents on failure (`epic_link_failed` / `epic_link_rollback_failed`). `epic delete` unlinks every member, and `spec file delete` refuses while the spec is in an epic (`spec_in_epic_delete`) through a new `preDelete` hook on the store-file factory. The store-access agent section names `epic` and `epic delete`.

**Deviations**: The link writer's rollback is a reusable `docTxn` rather than inline originals, so `epic split` can put its new and narrowed specs in the same transaction. `epic write` also accepts `--document-status`, like the other writes.

**Files changed**:
- `spektacular: cmd/epic.go`
- `spektacular: cmd/epic_link.go`
- `spektacular: cmd/epic_test.go`
- `spektacular: cmd/storefile.go`
- `spektacular: cmd/file.go`
- `spektacular: cmd/file_test.go`
- `spektacular: cmd/root.go`
- `spektacular: cmd/root_test.go`
- `spektacular: templates/agents/store-access.md`
- `spektacular: internal/agent/store_access_test.go`

**Discoveries**: After a split the epic and its first spec share a name, so "is this member an epic?" cannot be decided by the name existing in the epic store. A member is treated as a nested epic only when no spec of that name exists. This repo's own AGENTS.md store-access section is regenerated only by `init`, which is deferred to the skills regeneration in the status-command task.

### 2026-10-01 — Task: Split into an epic from the command line

**What was done**: Added `epic split --from <staged json>` (`cmd/epic_split.go`). It validates the description (at least two specs, the split spec named exactly once, every new spec titled, a non-empty overview and at least one acceptance criterion each, an overview when creating the epic) and the graph before writing. It allocates new spec IDs one at a time and writes each spec from the spec scaffold, complete and `final`, then rewrites the split spec as its narrowed self (keeping dates, designs and status). It creates an epic named after the split spec, or extends the one it already belongs to, with an Overview and a Specs table, moves the description's sources to the epic, and links everything both ways inside one transaction that restores all documents on failure.

**Deviations**: Added `docTxn.abort` so a refusal raised mid-transaction (before the link writer) still reports a failed restore as `epic_link_rollback_failed`. A new spec may carry an `id` (for `spec.id_method: external`) and a `scope` for the epic's table; the epic body starts with a `# Epic: <name>` title.

**Files changed**:
- `spektacular: cmd/epic_split.go`
- `spektacular: cmd/epic_split_test.go`
- `spektacular: cmd/epic_link.go`
- `spektacular: cmd/epic_test.go`
- `spektacular: cmd/root_test.go`

**Discoveries**: When an epic is extended, the scopes of existing members are read back from the epic body's Specs table, so a hand-edited table keeps its wording across later splits.

### 2026-10-01 — Task: Migrate existing projects to the epic store

**What was done**: Raised `CurrentProjectSchema` to 4 and registered a `project3to4` migration step that writes `epic_split_threshold: moderate` and the `epic` block (`provider: file`, `strict_dependencies: false`, `config.directory: epics`) into an existing project when each key is absent, keeping existing values; a second migrate changes nothing. A project still on format 3 is refused with `upgrade_required` naming `migrate`. Swept every test fixture that pinned format 3 as current, updated expectations for older-format fixtures that now migrate through to 4, and regenerated the migrate goldens.

**Deviations**: None. The fixture sweep was wider than context.md listed, as found in read_plan (version, config, repo, design, project init and engine tests).

**Files changed**:
- `spektacular: internal/config/schema.go`
- `spektacular: internal/migrate/steps_project.go`
- `spektacular: internal/migrate/registry.go`
- `spektacular: internal/migrate/steps_project_test.go`
- `spektacular: internal/migrate/engine_test.go`
- `spektacular: internal/migrate/testdata/current/config.yaml`
- `spektacular: internal/migrate/testdata/golden/legacy_single/config.yaml.golden`
- `spektacular: internal/migrate/testdata/golden/split_unversioned/config.yaml.golden`
- `spektacular: internal/config/config_test.go`
- `spektacular: internal/config/repo_test.go`
- `spektacular: internal/config/schema_test.go`
- `spektacular: internal/design/design_test.go`
- `spektacular: internal/project/init_test.go`
- `spektacular: cmd/gate_test.go`
- `spektacular: cmd/format_refusal_test.go`
- `spektacular: cmd/version_test.go`
- `spektacular: cmd/migrate_test.go`
- `spektacular: cmd/init_test.go`

**Discoveries**: Current-format fixtures are hand-maintained literals by convention (not derived from `CurrentProjectSchema`), so every schema bump needs the same sweep. Apply always runs to the current format, so the older step tests (2->3) also see the 3->4 actions.

### 2026-10-01 — Task: Start a spec with sources or in an epic

**What was done**: `spec new` accepts `sources` (a list of `{uri}`, each stamped with today's date; a missing uri is refused with `sources_invalid` showing the shape) and `epic` (which must exist, else `epic_not_found`), both checked before the start gate so a refusal writes nothing. The `new` step writes the sources to the spec's frontmatter. When an epic is named, `runSpecNew` joins the spec to it right after the `new` step through the existing link writer (`joinSpecToEpic` in `cmd/epic_link.go`), so the epic lists it with `depends_on: []` from the start. A failed join rolls back the spec and the epic and clears the workflow state. The `interview` step passes `sources` (URIs) and `epic` to its template.

**Deviations**: The join runs in `cmd` after the `new` step rather than inside the step callback, because the step package cannot import the link writer and the plan forbids a second copy. The `new` step's output is buffered until the join succeeds. A failed join does not undo the `new` step's reset of the working context.

**Files changed**:
- `spektacular: cmd/spec.go`
- `spektacular: cmd/epic_link.go`
- `spektacular: cmd/spec_test.go`
- `spektacular: internal/steps/spec/steps.go`
- `spektacular: internal/steps/spec/steps_test.go`

**Discoveries**: Workflow data survives a round trip through `state.json` as `[]any` of maps, so a typed value set in `spec new` (the stamped sources) must be decoded leniently by the step that reads it on a later turn (`sourcesFrom`).

### 2026-10-01 — Task: Build the status report

**What was done**: Added the `internal/status` package. It resolves a name (epic, then spec, then plan, else `artifact_not_found`) and builds the single report shape: the workflow block, `requested`, the epic with its roll-up, `done` and sources, and every spec with its state, readiness, blockers, effective sources (own, then the epic's) and plan with tasks and acceptance-criteria counts. It owns the one classifier (`missing`, `stale`, `specified`, `planned`, `in_progress`, `implemented`) with `Describe` wording, plus `DependenciesOf` for the implement check, `EpicComplete` for the completed-epic guard, and the pretty tree renderer.

**Deviations**: `plantask` gained `Plan.Items()` (total work items, so a legacy plan's phase count is available) and exported `WriteTasks` (the task-list half of `RenderPretty`, whose output is unchanged). `cmd/plan.go`'s stale check and `cmd/plan_export.go`'s repo locations now delegate to `internal/status`. An epic with no specs is never done, so the completed-epic guard does not block its first spec. Legacy (phase) plans count towards a spec's state but stay out of the epic's task roll-up, and the JSON omits their `progress` and `tasks`.

**Files changed**:
- `spektacular: internal/status/classify.go`
- `spektacular: internal/status/resolve.go`
- `spektacular: internal/status/report.go`
- `spektacular: internal/status/pretty.go`
- `spektacular: internal/status/status_test.go`
- `spektacular: internal/plantask/plantask.go`
- `spektacular: internal/plantask/export.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/plan_export.go`

**Discoveries**: A plan's live `current_step` is taken from the workflow state only when the workflow's kind matches (spec or plan). An implement run appears only in the report's `workflow` block, and the plan's step reads "finished".

### 2026-10-01 — Task: Add the split step and chaining to the spec workflow

**What was done**: The spec workflow gained a `split` step on the linear path `verification -> split -> finished`, rendered from `08b-split.md`. That template includes two new shared partials, `split-check` (the gate, strong and weak signals, the supporting-work rule, counter-signals, and a live `epic_split_threshold` read with what strict, moderate and lenient change) and `split-flow` (agreement, redistributing content, one fresh-eyes review over every resulting spec, provenance, staging and `epic split --from`). It honours a split request recorded earlier in the working context. The interview and section steps say to record a mid-workflow split request and continue. `finished` offers a spec for the next source item with no spec yet when the spec is in an epic. The spec completion commit point moved to `split -> finished`, so a split's epic and specs are committed with the spec.

**Deviations**: `workflow.Config` gained `EpicDir` so the spec steps can read the spec's epic and its sources. The plan's open question is resolved: both step and skill rendering provide `{{command}}`, so the partials use it and no renderer changed. `08-verification.md` needed no edit because its goto uses `{{next_step}}`.

**Files changed**:
- `spektacular: internal/steps/spec/steps.go`
- `spektacular: internal/steps/spec/steps_test.go`
- `spektacular: internal/steps/spec/split_test.go`
- `spektacular: internal/workflow/workflow.go`
- `spektacular: internal/autocommit/points.go`
- `spektacular: internal/autocommit/points_test.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/autocommit_test.go`
- `spektacular: cmd/startgate_test.go`
- `spektacular: cmd/artifact_status_test.go`
- `spektacular: cmd/instruction_contract_test.go`
- `spektacular: templates/partials/split-check.md`
- `spektacular: templates/partials/split-flow.md`
- `spektacular: templates/steps/spec/08b-split.md`
- `spektacular: templates/steps/spec/09-finished.md`
- `spektacular: templates/steps/spec/00b-interview.md`
- `spektacular: templates/steps/spec/01-overview.md`
- `spektacular: templates/steps/spec/02-requirements.md`
- `spektacular: templates/steps/spec/03-acceptance_criteria.md`
- `spektacular: templates/steps/spec/04-constraints.md`
- `spektacular: templates/steps/spec/05-technical_approach.md`
- `spektacular: templates/steps/spec/06-success_metrics.md`
- `spektacular: templates/steps/spec/07-non_goals.md`
- `spektacular: templates/split_test.go`

**Discoveries**: cbroglie/mustache has no index access (`list.0`), so a template that needs "is this list non-empty" gets an explicit boolean from the callback (`epic_has_sources`).

### 2026-10-01 — Task: Replace the status and export commands with status

**What was done**: Added the top-level `status [name] [--format pretty|json]` command (`cmd/status.go`), a thin renderer over `internal/status`: pretty by default, JSON with `--format json`, `{"workflow": null}` with no name and nothing in progress, `status_format_unsupported` and `artifact_not_found` refusals with runnable next actions. Removed `spec status`, `plan status`, `implement status` and `plan export` outright, along with `cmd/artifact_status.go`, `cmd/plan_export.go` and the per-kind `StatusResult` types. Moved every caller to `status`: the implement task hint, the `spek-implement` skill (tasks under `specs[].plan.tasks`), the README and the plan-workflow harbor check. Added the retired spellings to the instruction-surface deny-list, and re-pinned the old commands' tested behaviour in `cmd/status_test.go`.

**Deviations**: `status.BuildCurrent` now returns a workflow-only report when the workflow in progress has not written its artifact yet, rather than refusing. A plan without task structure reports its lifecycle (no `progress` or `tasks`) instead of being refused as `plan export` did. With no name, a finished workflow reports `{"workflow": null}`.

**Files changed**:
- `spektacular: cmd/status.go`
- `spektacular: cmd/status_test.go`
- `spektacular: cmd/root.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/plan.go`
- `spektacular: cmd/implement.go`
- `spektacular: cmd/artifact_status.go` (removed)
- `spektacular: cmd/plan_export.go` (removed)
- `spektacular: cmd/artifact_status_test.go` (removed)
- `spektacular: cmd/plan_status_progress_test.go` (removed)
- `spektacular: cmd/plan_export_test.go` (removed)
- `spektacular: cmd/cross_kind_test.go`
- `spektacular: cmd/implement_test.go`
- `spektacular: cmd/implement_task_test.go`
- `spektacular: cmd/status_address_test.go`
- `spektacular: cmd/root_test.go`
- `spektacular: internal/status/report.go`
- `spektacular: internal/status/status_test.go`
- `spektacular: internal/steps/spec/result.go`
- `spektacular: internal/steps/plan/result.go`
- `spektacular: internal/steps/implement/result.go`
- `spektacular: internal/plantask/export.go`
- `spektacular: internal/agent/instruction_surface_test.go`
- `spektacular: internal/output/writer_test.go`
- `spektacular: templates/skills/workflows/spek-implement/SKILL.md`
- `spektacular: templates/skill_resume_test.go`
- `spektacular: README.md`
- `spektacular: tests/harbor/plan-workflow/tests/test_plan_workflow.py`

**Discoveries**: `plan export X --format json` now fails with `internal_error` rather than `unknown_subcommand`, because cobra rejects the unknown flag before the subcommand lookup (existing behaviour for unknown flags). The generated `.claude/skills` / `.bob/skills` copies and this repo's knowledge entry `architecture/working-with-files-from-steps.md` still mention the retired commands until `init` and a knowledge update.

### 2026-10-01 — Task: Document the status view

**What was done**: `plan-tasks.mdx` replaces "Exporting a plan", "Export fields" and "Tracking progress" with one "Seeing where work stands: status" section: a readable example matching the real pretty renderer, a `--format json` example with the real field names, the no-name and nothing-in-progress cases, and a "Status fields" reference covering all six spec states. `documents.mdx` points at `spektacular status` and gains a table mapping each removed command to its replacement. `how-it-works.mdx` says `spektacular status`. The site changelog entry notes the replacement.

**Deviations**: The removed-command table is separate from the existing "Upgrading from earlier spellings" table, because that table's intro describes spellings refused with `unexpected_extension`, which the removed commands are not. The `workflow` block example shows no `task` field, because the design's (and the code's) workflow block has none.

**Files changed**:
- `docs: src/pages/plan-tasks.mdx`
- `docs: src/pages/documents.mdx`
- `docs: src/pages/how-it-works.mdx`
- `docs: CHANGELOG.md`

**Discoveries**: The retired `implement status` reported the selected task id from workflow data. `status` does not surface it, so a single-task run's task is visible only in `state.json` (`data.task`).

### 2026-10-01 — Task: Check spec dependencies when implementation starts

**What was done**: `implement new` checks a spec's direct dependencies in its epic after the stale-plan and task checks and before the start gate, using `status.DependenciesOf`, so a refusal writes no state. Unmet dependencies are refused with `dependencies_unmet`, naming each one and its state (for example "in progress (2/5 tasks complete)"). The next action offers the same command with `"override_dependencies": true` after the user agrees, or the first ready dependency. Under `epic.strict_dependencies` the override is refused with `dependency_override_refused`. An accepted override is stored as `dependency_override` in workflow data, and both changelog steps render it so the agent records it under Deviations. The implement help, input description and `spek-implement` skill now say a spec is implemented, and the skill handles both refusals.

**Deviations**: Under strict dependencies with no override requested, the refusal is `dependencies_unmet` without the override option, and `dependency_override_refused` is used only when an override was asked for. The `spek-implement` description in `internal/agent/commands.go` and the README quick start were also reworded to "implement a spec".

**Files changed**:
- `spektacular: cmd/implement.go`
- `spektacular: cmd/implement_dependencies_test.go`
- `spektacular: cmd/implement_test.go`
- `spektacular: internal/steps/implement/steps.go`
- `spektacular: internal/steps/implement/dependency_override_test.go`
- `spektacular: templates/steps/implement/07-update_changelog.md`
- `spektacular: templates/steps/implement/10-update_feature_changelog.md`
- `spektacular: templates/skills/workflows/spek-implement/SKILL.md`
- `spektacular: templates/skill_resume_test.go`
- `spektacular: internal/agent/commands.go`
- `spektacular: internal/agent/uncommitted_changes_test.go`
- `spektacular: README.md`

**Discoveries**: The re-run payload in the next action is rebuilt from the caller's `--data` with `override_dependencies` added, so a single-task run keeps its `task` field when it is re-run.

### 2026-10-01 — Task: Guard additions to a completed epic

**What was done**: Added `refuseCompletedEpic` (`cmd/epic_link.go`), built on `status.EpicComplete`. Adding a spec to an epic whose specs are all implemented, through `spec new` with `epic`, through `epic write` when its list gains a spec, or through `epic split` when it extends an epic, is refused with `epic_complete` naming the epic, and nothing is written. Re-running with `"confirm_completed_epic": true` (in `--data`, or in the staged split description) adds the spec, and the epic then reads as not done. Each route publishes the new field in its `--schema`. The split flow and `spek-new` skill tell the agent to ask the user before confirming.

**Deviations**: An epic with no specs is never complete, so its first spec needs no confirmation.

**Files changed**:
- `spektacular: cmd/epic_link.go`
- `spektacular: cmd/epic.go`
- `spektacular: cmd/epic_split.go`
- `spektacular: cmd/spec.go`
- `spektacular: cmd/epic_complete_test.go`
- `spektacular: cmd/epic_test.go`
- `spektacular: cmd/epic_split_test.go`
- `spektacular: templates/partials/split-flow.md`

**Discoveries**: None

### 2026-10-01 — Task: Seed specs from existing material

**What was done**: The interview step gained an epic branch (read the epic and each member spec first, do not re-ask what they cover, offer to record dependencies) and a seeded branch, rendered from the recorded sources so it survives a resume. The seeded branch has the agent fetch with its own tools, ask for the content if a source is unreachable, seed the section work files (title and body to overview and requirements, checklists to acceptance criteria, must/must-not to constraints, out of scope to non-goals), list the gaps, ask only about them, and record provenance in `interview.md`. Each section step presents a pre-filled work file as a draft to confirm. The `spek-new` skill gained trigger wording for sources and asks about joining an epic only when `epic list` shows one. It also gained the "Starting from existing material" section (recognise any phrasing, tool-agnostic fetching, unreachable sources, child items, naming, `spec new` with `sources`), the epic-first route for sources with child items (including late child items), `epic_complete` handling, and "Splitting a spec that is already written" through the shared split partials.

**Deviations**: The interview step is given a `seeded` boolean alongside the source list, because a mustache section over the list would repeat the branch once per source. Source links render unescaped (`{{{.}}}`) in the interview and the finished step's chaining offer, so a link containing `&` reaches the agent intact.

**Files changed**:
- `spektacular: templates/steps/spec/00b-interview.md`
- `spektacular: templates/steps/spec/01-overview.md`
- `spektacular: templates/steps/spec/02-requirements.md`
- `spektacular: templates/steps/spec/03-acceptance_criteria.md`
- `spektacular: templates/steps/spec/04-constraints.md`
- `spektacular: templates/steps/spec/05-technical_approach.md`
- `spektacular: templates/steps/spec/06-success_metrics.md`
- `spektacular: templates/steps/spec/07-non_goals.md`
- `spektacular: templates/steps/spec/09-finished.md`
- `spektacular: templates/skills/workflows/spek-new/SKILL.md`
- `spektacular: internal/steps/spec/steps.go`
- `spektacular: internal/steps/spec/seeding_test.go`
- `spektacular: templates/seeding_test.go`

**Discoveries**: Mustache `{{.}}` HTML-escapes, so any URL rendered into an instruction must use the triple-brace form or the agent receives `&amp;` in query strings.

### 2026-10-01 — Task: Update the spec-workflow harbor suite

**What was done**: The spec-workflow suite's `EXPECTED_STEP_ORDER` now has `split` between `verification` and `finished`. A new `TestSplitStep` asserts the step was completed, that the agent called `spec goto split`, and that the single coupled feature finishes with no epic stored. The solve script runs `spec goto split` before `finished`. The plan-workflow and implement-workflow oracles have no spec step lists. Their only retired-command use (the plan-workflow export check) moved to `status` in the status-command task.

**Deviations**: None

**Files changed**:
- `spektacular: tests/harbor/spec-workflow/tests/test_spec_workflow.py`
- `spektacular: tests/harbor/spec-workflow/solution/solve.sh`

**Discoveries**: None

### 2026-10-01 — Task: Document epics, splitting and dependencies

**What was done**: Added `src/pages/epics.mdx`, linked from the Resources menu after Design Documents. It covers what an epic is (real frontmatter), when a split is offered (gate, signals, supporting-work rule, counter-signals), split sensitivity (`epic_split_threshold`), what a split produces, starting with an epic (epic-first, chaining, joining, completed-epic confirmation), dependencies between specs and how implement treats an unmet one (with the real `dependencies_unmet` envelope and the strict-mode behaviour), and the `epic` commands, linking to the status docs.

**Deviations**: The `dependencies_unmet` sample and the strict-mode wording were reconciled with the CLI's actual refusal text after the dependency check landed.

**Files changed**:
- `docs: src/pages/epics.mdx`
- `docs: src/components/Nav.astro`
- `docs: CHANGELOG.md`

**Discoveries**: None

### 2026-10-01 — Task: Document seeding, implementing a spec and the new settings

**What was done**: `how-it-works.mdx` explains starting a spek from existing material (fetching with the agent's own tools, drafting what it can, asking only about gaps, the recorded `sources` example), says the spek is implemented, and notes `sources`/`epic` in the spek format. `index.mdx` and `getting-started.mdx` say a spek is implemented. `configuration.mdx` shows `epic_split_threshold` and the `epic` block in its example, counts sixteen top-level keys, adds `epic_split_threshold` and `epic` (`provider`, `strict_dependencies`, `config.directory`) references, and notes that migrate adds them. A `000060_epics-and-seeded-specs` site changelog entry covers the release.

**Deviations**: The configuration example's `schema:` was raised to 4 to match the new settings format.

**Files changed**:
- `docs: src/pages/how-it-works.mdx`
- `docs: src/pages/index.mdx`
- `docs: src/content/tutorials/getting-started.mdx`
- `docs: src/pages/configuration.mdx`
- `docs: CHANGELOG.md`

**Discoveries**: The configuration page said "Fourteen top-level keys" before this change; it now lists sixteen.

### 2026-10-02 — Task: Migrate this repository's own configuration

**What was done**: The user migrated this repository's `.spektacular/config.yaml` to settings format 4 (it now carries `epic_split_threshold: moderate` and the `epic` block), ran `init` to regenerate the installed skills and managed AGENTS.md sections, and switched `command` from `go run .` to the installed `spektacular` binary (`make install-local`). The CLI runs here again and `version check` reports `match`.

**Deviations**: Beyond the migration, `command` now points at the installed binary rather than `go run .`, so workflows are no longer driven by the working tree being changed.

**Files changed**:
- `spektacular: .spektacular/config.yaml`
- `spektacular: AGENTS.md`
- `spektacular: .claude/skills/*/SKILL.md`
- `spektacular: Makefile`

**Discoveries**: This task should never have existed: a plan must not change the install driving its own workflows. Recorded as the convention `conventions/plans-never-change-the-active-install.md` in the spektacular knowledge store.
