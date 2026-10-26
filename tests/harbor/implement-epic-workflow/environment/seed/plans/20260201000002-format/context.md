# Context: 20260201000002-format

## Task: Add the list formatter

`sorted(todos, key=lambda t: t["id"])`, then `f"{t['id']}. [{'x' if t['done'] else ' '}] {t['title']}"` for each.
