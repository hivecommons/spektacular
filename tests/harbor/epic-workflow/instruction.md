# Plan an Epic using Spektacular

You are testing the `spektacular` CLI tool by planning every spec of an epic.
The binary is already installed at `/usr/local/bin/spektacular`.

## Setup

First initialize the project:

```bash
spektacular init {{agent}}
```

## Task

The project in `/app` is a small Python todo app. It already holds one epic
of three complete specs:

- `20260101000001-storage`: persist todos to a JSON file
- `20260101000002-commands`: `add`, `list` and `done` commands; depends on
  storage
- `20260101000003-help`: usage text and a `--version` flag

It is the project's only epic; `spektacular epic list` shows its name.

Plan the whole epic by using the `spek-plan-epic` skill that was installed
during init. Run the skill:

```
{{spek_plan_epic}}
```

The skill makes you the orchestrator: it has you start a separate
sub-agent for each spec, each running the full plan workflow for that one
spec, in the order the specs' dependencies allow. Follow every instruction it
gives you, and the instructions each spec's plan workflow gives its
sub-agent.

This task is planning only. Do not implement anything and do not change the
code in `/app`.

## You are running non-interactively

There is no human at the other end of this session. You are playing both
roles: you drive the epic **and** you supply the answers the user would give.

- When a sub-agent hands back a question meant for the user, answer it
  yourself from the specs above, then let that spec carry on, as the point
  below says.
- When the skill puts a disagreement between plans to you with a proposed
  answer, accept the proposal.
- At the end-of-planning review, confirm the summary without changes.
- This session is headless: no one replies between your turns. The answers
  above are all you will get.
- Make a reasonable decision on anything else, record it where the workflow
  asks, and move on.

**Never end your turn waiting for a reply**: nothing will answer, and the run
will stall at that step. Only stop once `spektacular status <epic> --format
json` reports every spec planned and the skill has given its final report.

### Success criteria

- Every spec of the epic has a final plan
- `20260101000002-commands` is planned only after `20260101000001-storage`
- Each spec is planned by its own sub-agent, in its own orchestrated lane
- The epic's planning summary has a section for every spec and a decisions
  section, and `spektacular epic order` has been run
