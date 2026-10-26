## Step {{step}}: {{title}}

Write tests for the code you just implemented. Where they are written depends on the task's complexity, recorded in its section of the plan's `context.md`:

- **High**: the tests are written by a **sub-agent**, so the test-authoring context doesn't pollute the main implementation context. If your agent can start sub-agents, launch one before any test is written: a High-complexity task's tests are never written in your own context.
- **Low or Medium**, or no complexity recorded: write the tests yourself, in this context. Do not launch a sub-agent for them.

### Step 1: Load the current task

{{> partials/implement-current-task}}

You need the current task's heading and acceptance criteria from `plan.md`, and its whole section of the plan's `context.md`. When they are already in your context from an earlier step of this task, use them as they are: do not read the documents again. Read them only after a resume or a context compaction, or when the plan has changed since you read it:

1. Run `{{config.command}} plan file read {{plan_name}} plan` and find the current task's heading.
2. Run `{{config.command}} plan file read {{plan_name}} context` and read the current task's section of the plan's `context.md` in full.

If the section is missing, unreadable, or empty, STOP and ask the user whether to fix the plan's `context.md` before proceeding. This is a plan/reality mismatch — do not guess.

For a High-complexity task, pass the current task's acceptance criteria and its section of the plan's `context.md` to the sub-agent.

{{> partials/implement-code-locations}}

### Step 2: Write the tests with the follow-test-patterns skill

The tests follow the instructions from the skill below. For a High-complexity task, launch the sub-agent with them. Otherwise follow them yourself, reading the skill once per session rather than once per task:

```
{{config.command}} skill follow-test-patterns
```

Whoever writes the tests should:

1. Read the project's test conventions (typically documented in `thoughts/notes/testing.md` or discoverable via the existing plan/spec step tests).
2. Identify the package or packages the new code lives in.
3. Write `*_test.go` files that match the conventions — `stretchr/testify/require` assertions, `t.TempDir()` for fixtures, co-located with the package under test, inside the source of the repo the task attributes the work to.
4. Cover the current task's acceptance criteria from the plan's `plan.md`, and no other task's — each criterion should have a corresponding passing test assertion.
5. Cover any **success metric** that the plan's `## Testing Approach` flagged as a behavioural test and that this task delivers — write the actual test (e.g. a latency or throughput assertion). If a metric the plan expected to be automatable proves otherwise against the real code, do not force it: note it so the later `test_plan` step captures it as a manual procedure instead.
6. Return a concise summary: which files were written/modified and what each test asserts.

While a sub-agent runs, wait for it to return. Do not sleep, poll its output or schedule a wake-up: your agent tells you when it finishes.

### STOP-on-mismatch

If the test conventions it discovered don't match what the plan assumes, or that required test infrastructure (fixtures, helpers) is missing, STOP. Report to the user and ask for guidance before continuing.

{{> partials/implement-spec-conflict}}

### Advance

Once the tests are written (and, for a High-complexity task, the sub-agent has returned its summary):

```
{{config.command}} implement goto --data '{"step":"{{next_step}}","name":"{{plan_name}}"}'
```
