# Create an Implementation Plan using Spektacular

You are testing the `spektacular` CLI tool by driving a complete plan workflow
against a pre-existing specification. The binary is already installed at
`/usr/local/bin/spektacular`.

## Setup

First initialize the project:

```bash
spektacular init claude
```

A specification file already exists describing a stateless JWT authentication
feature. Read it through the CLI before you start planning — the plan
workflow's first step needs that context:

```bash
spektacular spec file read 20260101000000-user-auth.md
```

## Task

Drive the full plan workflow against the `20260101000000-user-auth` specification by using
the `/spek:plan` skill that was installed during init:

```
/spek:plan 20260101000000-user-auth
```

The skill will guide you through every plan step from `overview` through
`finished`. Follow each rendered instruction exactly — in particular:

- At the `discovery` step, use your agent-orchestration capability to spawn
  sub-agents in parallel for codebase research, and retrieve the skill the
  step template references (`spawn-planning-agents`). Also load the project's
  always-apply conventions in full with `spektacular knowledge always-applied`,
  as the step instructs.
- At the `tasks` step, retrieve the `spawn-implementation-agents` skill the
  template references.
- At the `verification` step, retrieve the `gather-project-metadata` and
  `determine-feature-slug` skills the template references. Then follow the
  rendered instructions to commit each filled document into the plan store
  with `spektacular plan file write 20260101000000-user-auth/<doc>.md` across the
  `write_plan`, `write_context`, and `write_research` steps. Never write or
  edit the plan documents (`plan.md`, `context.md`, `research.md`) with the
  `Write` or `Edit` tools — `plan file write` is the only supported way to
  write them.
- During the drafting steps (`overview` through `write_research`), draft
  straight through without stopping to ask for section approval or
  confirmation — record the judgement calls you make in the assumption log
  as each step's instruction directs, and interrupt only for a genuinely
  blocking question (this spec contains none).
- At the `walkthrough` step, conduct the guided review the instruction
  describes. You are acting as the reviewing user in this automated run:
  present the walkthrough beats, then give an explicit affirmative sign-off
  yourself and advance to `finished`. Do not skip the walkthrough or jump
  to `finished` before it has rendered.

Write meaningful, non-placeholder content for every section of every artefact.
The plan is a plan for *implementing the JWT authentication feature described
in the spec*, so draft content should talk about JWT, tokens, auth middleware,
and related concepts.

## After completion

Copy the `.spektacular` directory to `/logs/artifacts/` so results are
collected:

```bash
cp -r /app/.spektacular /logs/artifacts/spektacular
```

### Success criteria

- The workflow reaches the `finished` state
- All steps appear in `completed_steps` in canonical order
- The workflow passes through the `walkthrough` review step and reaches
  `finished` only after it — the `finished` instruction is rendered only
  once a `walkthrough` instruction has been rendered
- No user-directed confirmation question is posed during the drafting
  steps — the agent never calls the `AskUserQuestion` tool between
  starting the plan and reaching the `walkthrough` step
- `research.md`'s `## Drafting assumptions` section is populated with the
  judgement calls made while drafting — each entry states the decision,
  the rationale, and the rejected alternatives
- The plan documents stay `status: in-progress` while the walkthrough is
  open and are marked `status: completed` only after the `finished` step
- `plan.md`, `context.md`, `research.md` exist under `.spektacular/plans/20260101000000-user-auth/`
- Each section of each artefact has meaningful, non-placeholder text
- Each plan document is committed through `spektacular plan file write` — never written or edited directly with built-in file tools
- The agent retrieved every template-referenced skill during the step that referenced it
- The agent spawned at least one sub-agent during the `discovery` step
