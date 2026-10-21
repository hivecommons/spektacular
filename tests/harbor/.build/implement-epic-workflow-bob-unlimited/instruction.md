# Implement an Epic using Spektacular

You are testing the `spektacular` CLI tool by implementing every spec of an
epic. The binary is already installed at `/usr/local/bin/spektacular`.

## Setup

First initialize the project:

```bash
spektacular init bob
```

## Task

The project in `/app` is a small Python todo app, committed to git. It holds
one epic of four specs, each already planned:

- `20260201000001-storage`: persist todos to a JSON file
- `20260201000002-format`: render the todo list as text lines
- `20260201000003-commands`: `add`, `list` and `done` commands; depends on
  storage and format
- `20260201000004-export`: `todo export` as a Markdown checklist; depends on
  commands

It is the project's only epic; `spektacular epic list` shows its name.

Implement the whole epic by using the `spek-implement-epic` skill that was
installed during init. Run the skill:

```
/spek-implement-epic
```

The skill makes you the orchestrator: it has you give each spec its own git
worktrees and start a separate background sub-agent for it, running the full
implement workflow for that one spec, in the order the specs' dependencies
allow, and merge each finished spec back before anything that depends on it
starts. Specs that do not depend on each other are implemented at the same
time. Follow every instruction it gives you, and the instructions each spec's
implement workflow gives its sub-agent.

## You are running non-interactively

There is no human at the other end of this session. You are playing both
roles: you drive the epic **and** you supply the answers the user would give.

- When the skill tells you about uncommitted work before it starts, go ahead
  without committing it. The uncommitted files are the agent files `init`
  just installed, not code.
- When a sub-agent hands back a question meant for the user, answer it
  yourself with the default it recommends, or from the specs and plans, then
  let that sub-agent carry on.
- If you are offered to save something to the knowledge base, decline.
- Make a reasonable decision on anything else and move on.

**Never end your turn waiting for a reply**: nothing will answer, and the run
will stall at that step. Only stop once `spektacular status <epic> --format
json` reports every spec implemented and the skill has given its final
report.

### Success criteria

- Every spec is implemented and merged into the main line, and its worktrees
  and branch are gone
- `20260201000001-storage` and `20260201000002-format` are implemented at the
  same time, each by its own sub-agent
- `20260201000003-commands` starts only after both are merged, and
  `20260201000004-export` only after commands is merged
- `python3 -m todo` behaves as the specs describe, and the project's tests
  pass
