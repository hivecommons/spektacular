# Plan: 20260301000000-todo-count

## Overview

Add a `count` command to the todo CLI that prints how many todos the list holds, with tests and a Usage line in the README.

## Architecture & Design Decisions

All work lands in the single registered repo, `todo-app`. `todo/cli.py` gains a `count` subcommand whose handler prints `len(load())`. `tests/test_count.py` covers it, and the README's Usage section gains a `todo count` line.

## Component Breakdown

- `todo/cli.py`: the `count` subcommand.
- `tests/test_count.py` (new): tests for it.
- `README.md`: a `todo count` line in the Usage section.

## Data Structures & Interfaces

- `todo count` prints `len(load())` followed by a newline.

## Implementation Detail

Per task, in [context.md](./context.md).

## Dependencies

The existing `todo.storage.load()`. Python 3 standard library otherwise.

## Testing Approach

`tests/test_count.py` uses `unittest` and checks an empty list and a list of two.

## Milestones & Tasks

### Milestone 1: Count the todos

**What changes**: `python3 -m todo count` prints how many todos the list holds, and the README documents it.

**Validation point**: Every task's acceptance criteria hold and the test suite passes.

#### - [ ] Task: Add the count command
**Id:** 5d1e7a3c-2b4f-4c6d-8e9a-0b1c2d3e4f50
**Repo:** todo-app
**Depends on:** none
**Execution:** agent

Add the `count` subcommand to `todo/cli.py`, cover it in `tests/test_count.py`, and document it in the README's Usage section. Change no other file.

*Technical detail:* [context.md#task-add-the-count-command](./context.md#task-add-the-count-command)

**Acceptance criteria**:
- [ ] `python3 -m todo count` prints the number of todos in the list: `2` with two todos added, `0` with none.
- [ ] Unit tests in `tests/test_count.py` cover the count command.
- [ ] The README's Usage section documents `todo count` with an example line.

## Open Questions

None.

## Out of Scope

- Counting done and open todos separately (spec Non-Goal).
