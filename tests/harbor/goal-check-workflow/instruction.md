# Finish implementing a planned task using Spektacular

You are testing the `spektacular` CLI tool by finishing an implement run. The
binary is already installed at `/usr/local/bin/spektacular`.

## Setup

First initialize the project:

```bash
spektacular init {{agent}}
```

## Task

The project in `/app` is a small Python todo app, committed to git. Its plan
`20260301000000-todo-count` has one task, "Add the count command", and an
implement run for it is already in progress: it stopped at the `verify` step.
The task's work is in the working tree, uncommitted.

{{scenario}}

Resume the implement run with the `spek-implement` skill that was installed
during init:

```
{{spek_implement}}
```

Resume the run that is in progress rather than starting a new one, and follow
every instruction each step gives you until the workflow reaches its finished
step.

## You are running non-interactively

There is no human at the other end of this session. You supply the answers
the user would give.

- When asked whether to resume the run in progress or start a new one,
  resume it.
- When a step asks the user something, answer with the default it
  recommends, or from the spec and plan, and carry on.
- If you are offered to save something to the knowledge base, decline.
- Make a reasonable decision on anything else and move on.

**Never end your turn waiting for a reply**: nothing will answer. Only stop
once the implement workflow has reached its finished step.
