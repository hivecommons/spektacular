# Feature: 20260301000000-todo-count

## Overview

Add a `count` command that prints how many todos the list holds, so a user
can see at a glance how much is left without listing everything.

## Requirements

- [ ] **A count command**
  `python3 -m todo count` must print the number of todos in the list, as a
  bare number on one line.

- [ ] **Documented**
  The README's Usage section must show `todo count` with an example line.

## Constraints

- Standard library only; Python 3.12.
- Only `todo/cli.py`, `README.md` and new test files under `tests/` change.

## Acceptance Criteria

- [ ] **Counts the list**
  With two todos added, `python3 -m todo count` prints `2`; with none it
  prints `0`.

- [ ] **Tested**
  Unit tests cover the count command.

- [ ] **Documented**
  The README's Usage section shows `todo count` with an example line.

## Technical Approach

- A `count` subcommand beside `add` and `list` in `todo/cli.py`.
- Unit tests use `unittest` in `tests/test_count.py`.

## Success Metrics

- `python3 -m unittest discover -s tests` passes.

## Non-Goals

- Counting done and open todos separately.
