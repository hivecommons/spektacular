# Plan: 20260201000002-format

## Overview

Add `todo/format.py` with `format_todos(todos)`, the one place the todo list's printed layout is defined.

## Architecture & Design Decisions

All work lands in the single registered repo, `todo-app`. A new module, `todo/format.py`, exposes the pure function `format_todos(todos: list[dict]) -> list[str]`. It sorts by `id` and renders each todo as `<id>. [ ] <title>`, or `[x]` when `done` is true. An empty list renders as `["No todos."]`. It does no I/O.

## Component Breakdown

- `todo/format.py` (new): `format_todos`.
- `tests/test_format.py` (new): unit tests for it.

## Data Structures & Interfaces

- `format_todos(todos: list[dict]) -> list[str]`: one line per todo, sorted by `id`; `["No todos."]` for an empty list.

## Implementation Detail

Per task, in [context.md](./context.md).

## Dependencies

Python 3 standard library only. No other spec.

## Testing Approach

`tests/test_format.py` uses `unittest` and checks the layout, the ordering and the empty list.

## Milestones & Tasks

### Milestone 1: Add the list formatter

**What changes**: Add `todo/format.py` with `format_todos(todos)`, the one place the todo list's printed layout is defined.

**Validation point**: Every task's acceptance criteria hold and the test suite passes.

#### - [ ] Task: Add the list formatter
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e02
**Repo:** todo-app
**Depends on:** none
**Execution:** agent

Create `todo/format.py` with `format_todos(todos)` as the architecture describes, and `tests/test_format.py` covering the acceptance criteria. Change no other file.

*Technical detail:* [context.md#task-add-the-list-formatter](./context.md#task-add-the-list-formatter)

**Acceptance criteria**:
- [ ] `format_todos` of todo 2 (done, `b`) and todo 1 (not done, `a`) returns `["1. [ ] a", "2. [x] b"]`.
- [ ] `format_todos([])` returns `["No todos."]`.
- [ ] `python3 -m unittest discover -s tests` passes.

## Open Questions

None.

## Out of Scope

- Colour or terminal width (spec Non-Goal).
- Printing; the commands spec prints these lines.
