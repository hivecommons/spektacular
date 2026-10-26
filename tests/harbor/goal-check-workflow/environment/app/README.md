# todo

A small command-line todo list, run with `python3 -m todo`. Standard library
only; Python 3.12. The list is kept in `~/.todo.json`, or in the file
`TODO_FILE` names.

## Usage

    python3 -m todo add "buy milk"   # todo add: add a todo
    python3 -m todo list             # todo list: list the todos

## Tests

Tests live in `tests/` and use the standard library's `unittest`:

    python3 -m unittest discover -s tests
