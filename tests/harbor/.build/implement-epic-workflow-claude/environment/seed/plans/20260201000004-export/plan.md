# Plan: 20260201000004-export

## Overview

Add `todo/export.py` with `to_markdown(todos)` and a `todo export` command that prints the list as a Markdown checklist.

## Architecture & Design Decisions

All work lands in the single registered repo, `todo-app`. A new module, `todo/export.py`, exposes the pure function `to_markdown(todos: list[dict]) -> str`: todos sorted by `id`, each rendered `- [ ] <title>` or `- [x] <title>` followed by a newline, joined into one string; an empty list gives `""`. `todo/cli.py` gains an `export` subparser whose handler writes `to_markdown(load())` to stdout with `sys.stdout.write`, adding nothing, and returns 0.

## Testing Approach

`tests/test_export.py` uses `unittest`: `to_markdown` directly, and `main(["export"])` in-process with `TODO_FILE` pointed at a temporary file.

## Milestones & Tasks

### Milestone 1: Add the Markdown export

**What changes**: Add `todo/export.py` with `to_markdown(todos)` and a `todo export` command that prints the list as a Markdown checklist.

**Validation point**: The task's acceptance criteria hold and the test suite passes.

#### - [ ] Task: Add the Markdown export
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e04
**Repo:** todo-app
**Depends on:** none
**Execution:** agent

Create `todo/export.py` with `to_markdown(todos)`, add the `export` subcommand to `todo/cli.py`, and add `tests/test_export.py` covering the acceptance criteria. Read todos only through `todo.storage.load()`. Change no other file.

*Technical detail:* [context.md#task-add-the-markdown-export](./context.md#task-add-the-markdown-export)

**Acceptance criteria**:
- [ ] With `buy milk` (done) and `walk dog` (not done), `export` prints `- [x] buy milk` then `- [ ] walk dog`, one per line.
- [ ] `export` on an empty list prints nothing.
- [ ] `python3 -m unittest discover -s tests` passes.

## Open Questions

None.

## Out of Scope

- Formats other than Markdown, or writing to a file (spec Non-Goals).
