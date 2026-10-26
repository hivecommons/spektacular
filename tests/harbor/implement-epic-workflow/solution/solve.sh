#!/bin/bash
# Reference solution for the implement-epic-workflow harbor task.
#
# It documents the CLI sequence a correct orchestrated run follows and proves
# the verifier passes on it: each spec gets its worktrees, an orchestrated
# implement lane, its code committed on spek/<spec> by the finished step, and
# a merge before its dependents start. storage and format are interleaved so
# their lanes overlap, standing in for two sub-agents running side by side.
# It writes real implementations, so the behaviour checks pass on their own
# merit. The real test is a harbor run with an agent.
set -euo pipefail

cd /app
spektacular init claude >/dev/null
EPIC=$(spektacular epic list | jq -r '.files[0].name')
# Each task is walked through the implement steps in turn. At check_goal the
# reviewer's reply is recorded: every criterion judged met, with evidence.
TASK_STEPS="implement test verify check_goal"
WRAP_UP="test_plan update_feature_changelog reconcile_spec"

poll() { spektacular status "$EPIC" --format json >/dev/null; }
root() { spektacular status "$EPIC" --format json | jq -r --arg s "$1" '.specs[] | select(.name == $s) | .run.implement.root'; }
goto() { spektacular implement goto --data "{\"step\":\"$2\",\"name\":\"$1\"}" >/dev/null; }
tasks() { spektacular status "$1" --format json | jq -c --arg s "$1" '.specs[] | select(.name == $s) | .plan.tasks[]'; }

start() {
  spektacular epic worktree --data "{\"spec\":\"$1\"}" >/dev/null
  spektacular implement new --data "{\"name\":\"$1\",\"orchestrated\":true}" >/dev/null
}

# Record the goal check of one task: one met verdict per criterion.
record_goal() {
  local spec=$1 id=$2 total=$3 tmp=.spektacular/tmp/$1/goal-check
  mkdir -p "$tmp"
  jq -n --argjson n "$total" '{verdicts: [range(1; $n + 1) | {criterion: ., verdict: "met", evidence: "solution walk: the reference implementation meets it"}]}' > "$tmp/reply.json"
  spektacular implement goal record --data "{\"name\":\"$spec\",\"item\":\"$id\"}" --from "$tmp/reply.json" >/dev/null
  rm -rf "$tmp"
}

# Tick one task's heading and every criterion of it, as update_plan says to
# once all of them are judged met.
tick_task() {
  local spec=$1 id=$2 tmp=.spektacular/tmp
  spektacular plan file read "$spec" plan | python3 -c '
import sys
task_id = sys.argv[1]
lines = sys.stdin.read().split("\n")
heading = None
for i, line in enumerate(lines):
    if line.startswith("#### - [ ] Task:"):
        heading = i
    elif line.strip() == "**Id:** " + task_id and heading is not None:
        lines[heading] = lines[heading].replace("#### - [ ] Task:", "#### - [x] Task:", 1)
        j = i + 1
        while j < len(lines) and not lines[j].startswith("#"):
            if lines[j].startswith("- [ ] "):
                lines[j] = "- [x] " + lines[j][6:]
            j += 1
        break
sys.stdout.write("\n".join(lines))
' "$id" > "$tmp/plan.md"
  spektacular plan file write "$spec" plan --from "$tmp/plan.md" >/dev/null
  rm -f "$tmp/plan.md"
}

# Walk every task through the goal check, write the changelog, walk the
# wrap-up and finish with a commit message, all from the project root.
finish() {
  local spec=$1 tmp=.spektacular/tmp task id total
  local -a all
  mkdir -p "$tmp"
  mapfile -t all < <(tasks "$spec")
  for task in "${all[@]}"; do
    id=$(jq -r .id <<<"$task")
    total=$(jq -r .acceptance_criteria.total <<<"$task")
    goto "$spec" analyze
    for step in $TASK_STEPS; do goto "$spec" "$step"; done
    record_goal "$spec" "$id" "$total"
    goto "$spec" update_plan
    tick_task "$spec" "$id"
    goto "$spec" update_changelog
  done
  printf '# %s\n\nImplemented as planned.\n' "$spec" > "$tmp/changelog.md"
  spektacular changelog file write "$spec" --from "$tmp/changelog.md" >/dev/null
  for step in $WRAP_UP; do goto "$spec" "$step"; done
  printf 'Implement %s\n' "$spec" > "$tmp/git-commit-message.md"
  spektacular implement goto --data "{\"step\":\"finished\",\"name\":\"$spec\",\"commit_message_from\":\"$tmp/git-commit-message.md\"}" >/dev/null
  rm -f "$tmp/changelog.md"
}

merge() { spektacular epic merge --data "{\"spec\":\"$1\"}" >/dev/null; }

poll
start 20260201000001-storage
start 20260201000002-format

cat > "$(root 20260201000001-storage)/todo/storage.py" <<'PY'
"""The todo list, kept in a JSON file."""

import json
import os
import tempfile
from pathlib import Path


def _path() -> Path:
    override = os.environ.get("TODO_FILE")
    return Path(override) if override else Path.home() / ".todo.json"


def load() -> list[dict]:
    try:
        with _path().open() as f:
            return json.load(f)
    except FileNotFoundError:
        return []


def save(todos: list[dict]) -> None:
    target = _path()
    fd, tmp = tempfile.mkstemp(dir=target.parent)
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(todos, f)
        os.replace(tmp, target)
    except BaseException:
        Path(tmp).unlink(missing_ok=True)
        raise
PY
cat > "$(root 20260201000001-storage)/tests/test_storage.py" <<'PY'
import os
import tempfile
import unittest
from pathlib import Path

from todo import storage


class StorageTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.file = Path(self.dir.name) / "todos.json"
        os.environ["TODO_FILE"] = str(self.file)

    def tearDown(self):
        del os.environ["TODO_FILE"]
        self.dir.cleanup()

    def test_missing_file_is_empty(self):
        self.assertEqual(storage.load(), [])
        self.assertFalse(self.file.exists())

    def test_round_trip(self):
        todos = [{"id": 1, "title": "a", "done": False}]
        storage.save(todos)
        self.assertEqual(storage.load(), todos)
PY

cat > "$(root 20260201000002-format)/todo/format.py" <<'PY'
"""The printed layout of the todo list."""


def format_todos(todos: list[dict]) -> list[str]:
    if not todos:
        return ["No todos."]
    return [
        f"{t['id']}. [{'x' if t['done'] else ' '}] {t['title']}"
        for t in sorted(todos, key=lambda t: t["id"])
    ]
PY
cat > "$(root 20260201000002-format)/tests/test_format.py" <<'PY'
import unittest

from todo.format import format_todos


class FormatTest(unittest.TestCase):
    def test_layout(self):
        todos = [{"id": 2, "title": "b", "done": True}, {"id": 1, "title": "a", "done": False}]
        self.assertEqual(format_todos(todos), ["1. [ ] a", "2. [x] b"])

    def test_empty(self):
        self.assertEqual(format_todos([]), ["No todos."])
PY

finish 20260201000002-format
finish 20260201000001-storage
poll
merge 20260201000001-storage
merge 20260201000002-format
poll

start 20260201000003-commands
cat > "$(root 20260201000003-commands)/todo/cli.py" <<'PY'
"""Entry point for `python -m todo`."""

import argparse
import sys

from todo import storage
from todo.format import format_todos


def _add(args) -> int:
    todos = storage.load()
    new_id = max((t["id"] for t in todos), default=0) + 1
    todos.append({"id": new_id, "title": args.title, "done": False})
    storage.save(todos)
    print(f"Added {new_id}")
    return 0


def _list(args) -> int:
    for line in format_todos(storage.load()):
        print(line)
    return 0


def _done(args) -> int:
    todos = storage.load()
    for t in todos:
        if t["id"] == args.id:
            t["done"] = True
            storage.save(todos)
            print(f"Done {args.id}")
            return 0
    print(f"No todo {args.id}", file=sys.stderr)
    return 1


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="todo")
    sub = parser.add_subparsers()
    add = sub.add_parser("add", help="add a todo")
    add.add_argument("title")
    add.set_defaults(func=_add)
    sub.add_parser("list", help="list the todos").set_defaults(func=_list)
    done = sub.add_parser("done", help="mark a todo done")
    done.add_argument("id", type=int)
    done.set_defaults(func=_done)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if not hasattr(args, "func"):
        parser.print_usage()
        return 0
    return args.func(args)
PY
cat >> "$(root 20260201000003-commands)/README.md" <<'MD'

## Usage

    python3 -m todo add "buy milk"   # todo add: add a todo
    python3 -m todo list             # todo list: list the todos
    python3 -m todo done 1           # todo done: mark a todo done
MD
finish 20260201000003-commands
poll
merge 20260201000003-commands
poll

start 20260201000004-export
cat > "$(root 20260201000004-export)/todo/export.py" <<'PY'
"""The todo list as a Markdown checklist."""


def to_markdown(todos: list[dict]) -> str:
    return "".join(
        f"- [{'x' if t['done'] else ' '}] {t['title']}\n"
        for t in sorted(todos, key=lambda t: t["id"])
    )
PY
python3 - "$(root 20260201000004-export)/todo/cli.py" <<'PY'
import sys
from pathlib import Path

p = Path(sys.argv[1])
s = p.read_text()
s = s.replace("from todo.format import format_todos\n", "from todo.export import to_markdown\nfrom todo.format import format_todos\n")
s = s.replace(
    "def build_parser()",
    "def _export(args) -> int:\n    sys.stdout.write(to_markdown(storage.load()))\n    return 0\n\n\ndef build_parser()",
)
s = s.replace("    return parser\n", "    sub.add_parser(\"export\", help=\"print the list as a Markdown checklist\").set_defaults(func=_export)\n    return parser\n")
p.write_text(s)
PY
finish 20260201000004-export
poll
merge 20260201000004-export
poll
