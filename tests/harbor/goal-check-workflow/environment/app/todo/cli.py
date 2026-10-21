"""Entry point for `python -m todo`: a todo list kept in a JSON file."""

import argparse
import json
import os
from pathlib import Path


def _path() -> Path:
    override = os.environ.get("TODO_FILE")
    return Path(override) if override else Path.home() / ".todo.json"


def load() -> list[str]:
    try:
        return json.loads(_path().read_text())
    except FileNotFoundError:
        return []


def save(todos: list[str]) -> None:
    _path().write_text(json.dumps(todos))


def _add(args) -> int:
    todos = load()
    todos.append(args.title)
    save(todos)
    print(f"Added {len(todos)}")
    return 0


def _list(args) -> int:
    for i, title in enumerate(load(), start=1):
        print(f"{i}. {title}")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="todo")
    sub = parser.add_subparsers()
    add = sub.add_parser("add", help="add a todo")
    add.add_argument("title")
    add.set_defaults(func=_add)
    sub.add_parser("list", help="list the todos").set_defaults(func=_list)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if not hasattr(args, "func"):
        parser.print_usage()
        return 0
    return args.func(args)
