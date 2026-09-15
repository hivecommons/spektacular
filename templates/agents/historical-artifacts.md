## Historical Artifacts: Specs and Plans as Archaeology

> Managed by `{{command}} init` — edit `templates/agents/historical-artifacts.md`
> in the Spektacular source, not this section in place. Hand edits will not
> survive the next init.

In this repository, treat every file under `.spektacular/specs/` and
`.spektacular/plans/` as a historical, archaeological record. Each one
describes the intent behind a past change — *why* something was proposed,
what was in scope at that moment, and how the author framed the problem.
None of them are a description of what the codebase does today. Intent
recorded in a spec or plan may have been reshaped during implementation,
descoped, or abandoned entirely; only the shipped code, its tests, and its
configuration authoritatively describe current behavior.

Because of that, when you are exploring the codebase, summarizing a
feature, tracing how something works, or answering any question about
current-state behavior, do not read files under `.spektacular/specs/` or
`.spektacular/plans/` — through the `Read` tool, through `{{command}} spec
file read`, through `{{command}} plan file read`, or through any other
channel. Ground your answer in source files, tests, and configuration
instead, and cite paths under those directories rather than under the
spec or plan stores.

You may read a historical spec or plan only when the user is genuinely
investigating past intent — questions like "why was X built this way?",
"what was the original plan for Y?", or "which spec introduced Z?". In
that case, read the relevant document, and cite it explicitly as
historical context for a past decision rather than as a description of
current behavior. Archaeology is the only allowed reason to open these
files outside an active workflow.

There is one further exception: while a spec, plan, or implement
workflow is actively running, the workflow that owns its artifact may
read and update that artifact freely. That is what the workflow is for,
and it uses the dedicated CLI (`{{command}} spec file read/write`,
`{{command}} plan file read/write`) to do so. Once the workflow closes —
or for any agent that is not the workflow currently driving the
artifact — the artifact is historical again and subject to the same
rules as every other spec or plan on disk.

This rule applies everywhere you operate in the repository, not only
inside spec, plan, or implement workflow steps. It binds ad-hoc
questions, unrelated skills, and general exploration alike. Users
should not have to restate it in each session.

The knowledge base is the exception to all of the above, and the
distinction matters. Entries under a repo's or the project's knowledge
sources are **not** a record of past intent. They are current, curated
statements of how this project works and how work in it must be done,
and they are **binding**. Where a knowledge entry and the current code
disagree, the entry states the target and the code is what has yet to
meet it. Never conclude that an entry is stale, obsolete, or superseded
merely because the code does something else — that difference is work to
do, not evidence against the entry. If you genuinely believe an entry is
wrong, say so to the user and ask; do not silently overrule it.

More broadly, the remainder of `.spektacular/` — `working-context.md` and
changelog records — is generated output *about* the codebase, not the
codebase itself. A broad grep or file scan run to understand
current-state behavior should treat `.spektacular/specs/`,
`.spektacular/plans/`, `working-context.md` and the changelog as out of scope,
unless the task explicitly concerns them.
