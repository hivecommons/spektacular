# Create a Feature Specification using Spektacular

You are testing the `spektacular` CLI tool by creating a complete feature specification.
The binary is already installed at `/usr/local/bin/spektacular`.

## Setup

First initialize the project:

```bash
spektacular init claude
```

## Task

Create a specification for a **user authentication feature using JWT tokens** by
using the `spek-new` skill that was installed during init.

Run the skill:

```
/spek:new user-auth
```

The skill will guide you through the full spec workflow. Follow each instruction
it gives you.

When writing content for each section, use these details about the feature:
- **What**: Stateless user authentication using JWT access and refresh tokens
- **Problem**: The current session-based auth doesn't scale across multiple services
- **Users**: Backend developers consuming the auth API, and end users who log in

Write meaningful, non-placeholder content for every section.

## You are running non-interactively

There is no human at the other end of this session. You are playing both roles:
you drive the workflow **and** you supply the answers it asks for.

So when the workflow interviews you, or asks you to confirm a draft, answer it
yourself from the details above and from the scripted rejection below, and keep
going. Make a reasonable decision on anything they do not cover, record it in
the spec, and move on. **Never end your turn waiting for a reply** — nothing
will answer, and the run will stall at that step. Only stop once the workflow
itself reports it has finished.

## A rejection that reveals a missing requirement

When you reach the **Constraints** step and it presents its draft to you, reject
that draft: tell the agent the draft is missing something, and when it asks a
follow-up question to understand why, explain that **token revocation must be
supported** — a user's refresh token needs to be invalidated immediately if
their account is compromised, and there is currently no requirement covering
this.

This is a **Requirements** gap, not a Constraints one: token revocation is a
behavior the system must deliver, not a boundary the solution must operate
within. Expect the agent to recognize this, add a revocation requirement to
the already-confirmed Requirements section on its own initiative — without
asking you to stop and re-review Requirements in the moment — and then
continue presenting the Constraints draft for you to confirm.

Confirm the Constraints draft once you're satisfied with it, and continue
through the remaining steps as normal.

## After completion

Copy the `.spektacular` directory to `/logs/artifacts/` so results are collected:

```bash
cp -r /app/.spektacular /logs/artifacts/spektacular
```

### Success criteria

- The workflow reaches the `finished` or `done` state
- All steps appear in the completed_steps list
- The created spec file under `.spektacular/specs/` contains content
- Each spec section has meaningful, non-placeholder text
- The spec file is committed through `spektacular spec file write` — never written or edited directly with built-in file tools
