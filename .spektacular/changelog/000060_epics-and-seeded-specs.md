---
created_date: "2026-10-02"
document_status: final
closed_date: "2026-10-02"
---

# 000060_epics-and-seeded-specs: epics, splitting, seeded specs and one status view

## What was built

**Epics.** A new epic store (`epic` in `config.yaml`: `provider`, `strict_dependencies`,
`config.directory`, default `epics`) and the commands `epic read / write / list / delete / split`.
An epic holds an overview and its specs, each with its `depends_on` list. The CLI refuses invalid
graphs (missing fields, duplicates, unknown names, cycles), a spec in two epics and nested epics.
Specs carry `epic` and `sources` in their frontmatter, and both survive every rewrite. Membership is
kept in agreement both ways by one link writer; every multi-document write runs in a small
transaction that restores every touched document on failure. `spec file delete` refuses while a
spec is in an epic. Settings move to format 4, and the upgrade writes the epic block and
`epic_split_threshold` into existing projects.

**Splitting.** `epic split` turns one complete spec into an epic of complete, `final` specs in a
single operation: it allocates IDs, renders every section, narrows the split spec, writes or extends
the epic and links everything. The spec workflow gained a `split` step between verification and
finished, driven by two shared instruction fragments (the split check, with its gate, signals,
supporting-work rule, counter-signals and live `epic_split_threshold`; and the split flow, with
agreement, redistribution, one review over every resulting spec and provenance), which the
`spek-new` skill also uses to split an already-written spec. A finished spec in an epic offers a
spec for the next source item that has none.

**Starting from existing material.** `spec new` accepts `sources` (stamped with today's date) and an
`epic` to join from the start. The interview seeds the section drafts from the material, lists the
gaps and asks only about those, and every section step confirms a pre-filled draft. The `spek-new`
skill recognises a source however it is phrased, fetches it with the agent's own tools, handles
unreachable sources and child items (epic first, then one spec per child), and asks about joining
an epic only when the project has one. Adding to a completed epic needs explicit confirmation.

**One status view.** `spektacular status [name] [--format pretty|json]` reports a whole epic from
any epic, spec or plan name: each spec's state (`missing`, `stale`, `specified`, `planned`,
`in_progress`, `implemented`), readiness, blockers, effective sources and plan tasks with
acceptance-criteria counts, plus the epic's roll-up and `done`. With no name it reports the
workflow in progress. `spec status`, `plan status`, `implement status` and `plan export` are
removed, and every caller moved to `status`.

**Implementing a spec.** `implement new` checks the spec's direct dependencies in its epic before
it starts: unmet ones are named with their state ("in progress (2/5 tasks complete)") and the user
may continue with `override_dependencies`, which is recorded in the changelog, or, under
`epic.strict_dependencies`, is refused. Help, input and skill now say a spec is being implemented.

**Docs site.** A new epics page; the status view replaces the export and status material with a
table mapping the removed commands; starting from existing material; "implement a spec" wording;
the epic settings in the configuration reference.

## Why it matters

Large requests used to become one spec whose acceptance criteria were too thin to build from, and
work already written up in a tracker had to be re-explained with no record of its origin. Specs can
now be started from that material and asked only about its gaps, an oversized spec can be split
into an epic of specs that each carry their own testable criteria and declared order, and one
command shows how far the whole epic has progressed.

## Deviations from the plan

- `metadata` exports `SplitRaw`, `IsClosed`, `ValidateDocumentStatus` and `DateFormat` so the epic
  type reuses its fence handling and lifecycle rules; `plantask` gained `Plan.Items()` and an
  exported `WriteTasks`.
- The link writer's rollback became a reusable `docTxn`, so `epic split` and `spec new` share one
  transaction mechanism with `epic write`.
- `spec new` joins the epic from `cmd` after the `new` step runs, rather than inside the step
  callback, to keep a single link-writing implementation.
- `workflow.Config` gained `EpicDir`. The open question about the command variable in shared
  partials resolved itself: steps and skills both provide `{{command}}`.
- An epic with no specs is never complete; legacy (phase) plans count towards a spec's state but
  stay out of the epic's task roll-up.
- `status` with no name reports a workflow-only result when the workflow's artifact is not written
  yet, and reports nothing for a finished workflow. Its `workflow` block carries no task id.
- Under strict dependencies with no override requested the refusal is `dependencies_unmet`;
  `dependency_override_refused` is used only when an override was asked for.
- Source links render unescaped (`{{{.}}}`) in templates, after a test found `&` arriving as
  `&amp;`.
- This repository's own configuration was migrated mid-implementation (a human task), and its
  `command` now points at the installed binary. That task should never have existed; it is
  recorded as the convention that a plan never changes the active install.
- Only the spec-workflow harbor suite was run (passed); the plan-workflow and implement-workflow
  suites are listed in the test plan.
