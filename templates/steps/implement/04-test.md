## Step {{step}}: {{title}}

Write tests for the code you just implemented. This step runs in a **sub-agent** so the test-authoring context doesn't pollute the main implementation context. Do not write tests in the main context.

### Step 1: Load the current phase

Do not rely on an earlier step's output still being in your context. Read the phase from the plan itself:

1. Run `{{config.command}} plan file read {{plan_name}}/plan.md` and take the first unchecked `#### - [ ] Phase N.M:` heading under `## Milestones & Phases` as the current phase.
2. Run `{{config.command}} plan file read {{plan_name}}/context.md` and read that phase's `### Phase N.M:` section of the plan's `context.md` in full.

If the section is missing, unreadable, or empty, STOP and ask the user whether to fix the plan's `context.md` before proceeding. This is a plan/reality mismatch — do not guess.

Pass the current phase's acceptance criteria and its section of the plan's `context.md` to the sub-agent.

### Step 2: Delegate to the follow-test-patterns skill

Launch a sub-agent with the instructions from:

```
{{config.command}} skill follow-test-patterns
```

The sub-agent should:

1. Read the project's test conventions (typically documented in `thoughts/notes/testing.md` or discoverable via the existing plan/spec step tests).
2. Identify the package or packages the new code lives in.
3. Write `*_test.go` files that match the conventions — `stretchr/testify/require` assertions, `t.TempDir()` for fixtures, co-located with the package under test, inside the source of the repo the phase attributes the work to.
4. Cover the phase's acceptance criteria from the current phase in the plan's `plan.md` — each criterion should have a corresponding passing test assertion.
5. Cover any **success metric** that the plan's `## Testing Approach` flagged as a behavioural test and that this phase delivers — write the actual test (e.g. a latency or throughput assertion). If a metric the plan expected to be automatable proves otherwise against the real code, do not force it: note it so the later `test_plan` step captures it as a manual procedure instead.
6. Return a concise summary: which files were written/modified and what each test asserts.

### STOP-on-mismatch

If the sub-agent reports that the test conventions it discovered don't match what the plan assumes, or that required test infrastructure (fixtures, helpers) is missing, STOP. Report to the user and ask for guidance before continuing.

### Advance

Once tests are written and the sub-agent has returned its summary:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
