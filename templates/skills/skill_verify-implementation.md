# Verify Implementation

Guide for running a plan task's verification commands and reporting a concise pass/fail summary. Use this skill when the implement workflow reaches its `verify` step. The agent driving the workflow runs the commands itself, keeping each command's output short.

## Instructions

Run the verification commands for the task the main agent just implemented and tested, then return a terse pass/fail summary.

### Step 1: Identify the verification commands

Read the current task's acceptance criteria in `plan.md` (only that task's) and map each criterion to a concrete command. Typical sources:

- **`thoughts/notes/commands.md`** — project-wide command reference documenting `make test`, `make lint`, `go test ./...`, etc.
- **The plan's `## Testing Approach` section** — names the overall test strategy.
- **The task's notes in the plan's `context.md`** — may list task-specific verification commands.

If no commands are documented, fall back to the standard Go pipeline: `go build ./...`, `go test ./...`, and whatever linter the project uses.

### Step 2: Run each command

Run every command in sequence, with its output trimmed so a passing run prints almost nothing (pipe it through `tail -n 20`, or use a quiet flag). For each one:

- Capture the exit code.
- On failure, capture a **short excerpt** (5–10 lines) of the failing output — typically the test that failed, the file:line where it failed, and the assertion message. Do NOT capture the full test output.
- On success, just note "pass".

### Step 3: Write a concise summary

Write a terse report, **one line per command**. Example:

```
make test     → pass
make lint     → pass
go vet ./...  → pass
```

If everything passes, a single line is enough:

```
all green
```

If anything fails, include the 5–10 line excerpt under each failing command. Example:

```
make test     → FAIL
    --- FAIL: TestExtraction (0.01s)
        steps_test.go:42: expected "overview", got ""
make lint     → pass
```

Do **not** keep the full test output — the context is limited and needs a digest, not a transcript.

### Step 4: Do not try to fix failures

Verification reports; it does not fix. If a command fails, keep the failing excerpt and do not change the code while verifying. The implement workflow's `verify` step will handle the branching logic (re-run the implement step, ask the user, or abandon the task).
