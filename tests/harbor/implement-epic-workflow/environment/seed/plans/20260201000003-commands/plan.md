# Plan: 20260201000003-commands

## Overview

Add the `add`, `list` and `done` commands to `todo/cli.py`, reading and writing through `todo.storage` and printing through `todo.format`, with help for each command and a usage section in the README.

## Architecture & Design Decisions

All work lands in the single registered repo, `todo-app`. `todo/cli.py` gains `argparse` subparsers `add <title>`, `list` and `done <id>`, each with a one-line `help` string and dispatched to a small handler through `set_defaults(func=...)`. `add` appends `{"id": max_id + 1, "title": title, "done": False}` (id 1 for an empty list), saves, and prints `Added <id>`. `list` prints each line of `format_todos(load())`. `done` marks the todo done, saves, and prints `Done <id>`; an unknown id prints `No todo <id>` to stderr and returns 1. With no subcommand, `main` prints the usage and returns 0. The README gains a `## Usage` section.

## Component Breakdown

- `todo/cli.py`: the `add`, `list` and `done` subcommands and their handlers.
- `README.md`: a `## Usage` section.
- `tests/test_commands.py` (new): tests for each command, the help text and the README.

## Data Structures & Interfaces

- `main(argv: list[str] | None = None) -> int`: parses `argv` and dispatches to the subcommand's handler.
- Output: `Added <id>`, the lines of `format_todos(load())`, `Done <id>`, or `No todo <id>` on stderr.

## Implementation Detail

Per task, in [context.md](./context.md).

## Dependencies

`todo.storage` (spec `20260201000001-storage`) and `todo.format` (spec `20260201000002-format`), both merged before this one starts. Python 3 standard library otherwise.

## Testing Approach

`tests/test_commands.py` uses `unittest`, points `TODO_FILE` at a temporary file, and calls `main([...])` in-process, capturing stdout and stderr. Each task adds the tests for its own behaviour and runs the whole suite.

## Milestones & Tasks

### Milestone 1: The todo commands, with help and usage

**What changes**: Add the `add`, `list` and `done` commands to `todo/cli.py`, reading and writing through `todo.storage` and printing through `todo.format`, with help for each command and a usage section in the README.

**Validation point**: Every task's acceptance criteria hold and the test suite passes.

#### - [ ] Task: Add the add command
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e31
**Repo:** todo-app
**Depends on:** none
**Execution:** agent

Give `todo/cli.py` its subparsers and the `add` subcommand as the architecture describes, and start `tests/test_commands.py` with tests for it.

*Technical detail:* [context.md#task-add-the-add-command](./context.md#task-add-the-add-command)

**Acceptance criteria**:
- [ ] On an empty list, `add "buy milk"` prints `Added 1` and stores a not-done todo with id 1.
- [ ] A second `add` gets id 2.
- [ ] `python3 -m unittest discover -s tests` passes.

#### - [ ] Task: Add the list command
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e32
**Repo:** todo-app
**Depends on:**
- 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e31 — Add the add command
**Execution:** agent

Add the `list` subcommand, printing each line of `todo.format.format_todos` for the stored todos, with tests.

*Technical detail:* [context.md#task-add-the-list-command](./context.md#task-add-the-list-command)

**Acceptance criteria**:
- [ ] `list` on an empty list prints `No todos.`.
- [ ] After `add "buy milk"`, `list` prints `1. [ ] buy milk`.
- [ ] `python3 -m unittest discover -s tests` passes.

#### - [ ] Task: Add the done command
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e33
**Repo:** todo-app
**Depends on:**
- 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e32 — Add the list command
**Execution:** agent

Add the `done <id>` subcommand, marking the todo done and printing `Done <id>`, with tests.

*Technical detail:* [context.md#task-add-the-done-command](./context.md#task-add-the-done-command)

**Acceptance criteria**:
- [ ] `done 1` prints `Done 1` and `list` then prints `1. [x] buy milk`.
- [ ] `python3 -m unittest discover -s tests` passes.

#### - [ ] Task: Reject an unknown id
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e34
**Repo:** todo-app
**Depends on:**
- 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e33 — Add the done command
**Execution:** agent

Make `done` with an id that does not exist print `No todo <id>` to stderr and return 1, leaving the list unchanged, with tests.

*Technical detail:* [context.md#task-reject-an-unknown-id](./context.md#task-reject-an-unknown-id)

**Acceptance criteria**:
- [ ] `done 99` on a list without id 99 returns 1 and writes `No todo 99` to stderr.
- [ ] The stored list is unchanged.
- [ ] `python3 -m unittest discover -s tests` passes.

#### - [ ] Task: Describe each command in help
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e35
**Repo:** todo-app
**Depends on:**
- 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e34 — Reject an unknown id
**Execution:** agent

Give each subparser a one-line `help` string so `todo --help` lists `add`, `list` and `done` with a description, with a test.

*Technical detail:* [context.md#task-describe-each-command-in-help](./context.md#task-describe-each-command-in-help)

**Acceptance criteria**:
- [ ] `todo --help` exits 0 and names `add`, `list` and `done`, each with a description.
- [ ] `python3 -m unittest discover -s tests` passes.

#### - [ ] Task: Document usage in the README
**Id:** 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e36
**Repo:** todo-app
**Depends on:**
- 0c4f1d8e-6a2b-4c3d-9e5f-1a2b3c4d5e35 — Describe each command in help
**Execution:** agent

Add a `## Usage` section to `README.md` showing `todo add`, `todo list` and `todo done`, each with an example.

*Technical detail:* [context.md#task-document-usage-in-the-readme](./context.md#task-document-usage-in-the-readme)

**Acceptance criteria**:
- [ ] `README.md` has a `## Usage` section naming `todo add`, `todo list` and `todo done`.

## Open Questions

None.

## Out of Scope

- Deleting or editing todos (spec Non-Goal).
- Export, which the export spec adds.
