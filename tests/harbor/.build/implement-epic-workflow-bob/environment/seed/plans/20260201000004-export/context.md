# Context: 20260201000004-export

## Task: Add the Markdown export

`"".join(f"- [{'x' if t['done'] else ' '}] {t['title']}\n" for t in sorted(todos, key=lambda t: t['id']))`.
