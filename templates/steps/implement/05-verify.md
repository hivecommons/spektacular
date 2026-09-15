## Step {{step}}: {{title}}

Run the verification commands for the current phase and report the result. This step runs in a **sub-agent** so the full command output stays out of the main context.

### Step 1: Load the current phase

Do not rely on an earlier step's output still being in your context. Read the phase from the plan itself:

1. Run `{{config.command}} plan file read {{plan_name}}/plan.md` and take the first unchecked `#### - [ ] Phase N.M:` heading under `## Milestones & Phases` as the current phase.
2. Run `{{config.command}} plan file read {{plan_name}}/context.md` and read that phase's `### Phase N.M:` section of the plan's `context.md` in full.

If the section is missing, unreadable, or empty, STOP and ask the user whether to fix the plan's `context.md` before proceeding. This is a plan/reality mismatch — do not guess.

### Step 2: Delegate to the verify-implementation skill

Launch a sub-agent with the instructions from:

```
{{config.command}} skill verify-implementation
```

The sub-agent should:

1. Read the current phase's acceptance criteria from the current phase in the plan's `plan.md`.
2. Map each criterion to a concrete verification command (typically `make test`, `make lint`, or a phase-specific command listed in the current phase's section of the plan's `context.md` or `thoughts/notes/commands.md`).
3. Run each command from the source of the repo the phase's work landed in, capture exit codes and a short excerpt of any failures.
4. Return a **concise pass/fail summary** — one line per command, no full test output. If everything passes, a single "all green" line is enough.

### STOP-on-mismatch

If any verification command fails, STOP. Report the failures to the user as the sub-agent returned them. Do not advance to the next step — the user must decide whether to:

1. **Fix the code and re-run verification.** Return to the `implement` step with the fixes in mind.
2. **Accept the partial failure and proceed.** Only if the user explicitly chooses this.
3. **Abandon this phase.** Skip to the next unchecked phase.

### Advance

Once verification is green (or the user has explicitly authorized proceeding past a failure):

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
