# Plan: 20260201000001-storage

## Overview

Add `todo/storage.py`, which keeps the todo list in a JSON file: `~/.todo.json`, or the file named by `TODO_FILE`. Saves are atomic. The commands spec builds on its `load()` and `save()`.

## Architecture & Design Decisions

All work lands in the single registered repo, `todo-app`. A new module, `todo/storage.py`, exposes `load() -> list[dict]` and `save(todos: list[dict]) -> None`. A private `_path()` resolves the file on every call: `TODO_FILE` when set and non-empty, otherwise `Path.home() / ".todo.json"`. `load()` returns `[]` for a missing file. `save()` writes JSON to a `tempfile.mkstemp` file in the target's directory and `os.replace`s it over the target, removing the temporary file if anything fails first.

## Component Breakdown

- `todo/storage.py` (new): `load`, `save` and the private `_path`.
- `tests/test_storage.py` (new): unit tests for the module.

## Data Structures & Interfaces

- `load() -> list[dict]`: the stored todos, `[]` when the file is missing.
- `save(todos: list[dict]) -> None`: writes the list as JSON, atomically.
- A todo is `{"id": int, "title": str, "done": bool}`.

## Implementation Detail

Per task, in [context.md](./context.md).

## Dependencies

Python 3 standard library only (`json`, `os`, `pathlib`, `tempfile`). No other spec.

## Testing Approach

`tests/test_storage.py` uses `unittest`. Every test points `TODO_FILE` and `HOME` at a temporary directory, so the real home directory is never touched.

## Milestones & Tasks

### Milestone 1: Add the storage module with atomic save

**What changes**: Add `todo/storage.py`, which keeps the todo list in a JSON file: `~/.todo.json`, or the file named by `TODO_FILE`. Saves are atomic. The commands spec builds on its `load()` and `save()`.

**Validation point**: Every task's acceptance criteria hold and the test suite passes.

#### - [ ] Task: Add the storage module with atomic save
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e01
**Repo:** todo-app
**Depends on:** none
**Execution:** agent

Create `todo/storage.py` with `load()` and `save(todos)` as the architecture describes, and `tests/test_storage.py` covering the round trip, the missing file and the `TODO_FILE` override. Change no other file.

*Technical detail:* [context.md#task-add-the-storage-module-with-atomic-save](./context.md#task-add-the-storage-module-with-atomic-save)

**Acceptance criteria**:
- [ ] Saving a list and loading it again returns an equal list.
- [ ] Loading a path that does not exist returns `[]` and creates no file.
- [ ] With `TODO_FILE` set, load and save use that path only.
- [ ] `python3 -m unittest discover -s tests` passes.

## Open Questions

None.

## Out of Scope

- Concurrent writers (spec Non-Goal).
- Any format other than JSON (spec Non-Goal).
- CLI commands, which the commands spec adds.
