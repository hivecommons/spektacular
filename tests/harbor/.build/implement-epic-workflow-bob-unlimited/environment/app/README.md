# todo

A small command-line todo list, run with `python3 -m todo`. Standard library
only; Python 3.12.

The package is a skeleton: `todo/cli.py` parses arguments but has no commands
yet, and nothing is stored.

Tests live in `tests/` and use the standard library's `unittest`:

    python3 -m unittest discover -s tests
