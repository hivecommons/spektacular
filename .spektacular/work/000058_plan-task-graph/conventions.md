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
