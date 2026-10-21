# Plan: 20260201000003-commands

## Overview

Add the `add`, `list` and `done` commands to `todo/cli.py`, reading and writing through `todo.storage` and printing through `todo.format`.

## Architecture & Design Decisions

All work lands in the single registered repo, `todo-app`. `todo/cli.py` gains `argparse` subparsers `add <title>`, `list` and `done <id>`, each dispatched to a small handler through `set_defaults(func=...)`. `add` appends `{"id": max_id + 1, "title": title, "done": False}` (id 1 for an empty list), saves, and prints `Added <id>`. `list` prints each line of `format_todos(load())`. `done` marks the todo done, saves, and prints `Done <id>`; an unknown id prints `No todo <id>` to stderr and returns 1. With no subcommand, `main` prints the usage and returns 0.

## Testing Approach

`tests/test_commands.py` uses `unittest`, points `TODO_FILE` at a temporary file, and calls `main([...])` in-process, capturing stdout and stderr.

## Milestones & Tasks

### Milestone 1: Add the add, list and done commands

**What changes**: Add the `add`, `list` and `done` commands to `todo/cli.py`, reading and writing through `todo.storage` and printing through `todo.format`.

**Validation point**: The task's acceptance criteria hold and the test suite passes.

#### - [ ] Task: Add the add, list and done commands
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e03
**Repo:** todo-app
**Depends on:** none
**Execution:** agent

Extend `todo/cli.py` with the three subcommands as the architecture describes, and add `tests/test_commands.py` covering the acceptance criteria. Use only `todo.storage.load()`/`save()` and `todo.format.format_todos`. Change no other file.

*Technical detail:* [context.md#task-add-the-add-list-and-done-commands](./context.md#task-add-the-add-list-and-done-commands)

**Acceptance criteria**:
- [ ] On an empty list, `add "buy milk"` prints `Added 1` and `list` then prints `1. [ ] buy milk`.
- [ ] `done 1` prints `Done 1` and `list` then prints `1. [x] buy milk`.
- [ ] `done 99` on a list without id 99 returns 1 and writes `No todo 99` to stderr.
- [ ] `python3 -m unittest discover -s tests` passes.

## Open Questions

None.

## Out of Scope

- Deleting or editing todos (spec Non-Goal).
- Export, which the export spec adds.
