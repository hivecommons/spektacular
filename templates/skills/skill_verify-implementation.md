# Verify Implementation

Guide for running a plan phase's verification commands and reporting a concise pass/fail summary. Use this skill when the implement workflow reaches its `verify` step and needs to delegate command execution to a sub-agent.

## Instructions

Run the verification commands for the phase the main agent just implemented and tested, then return a terse pass/fail summary.

### Step 1: Identify the verification commands

Read the current phase's acceptance criteria in `plan.md` and map each criterion to a concrete command. Typical sources:

- **`thoughts/notes/commands.md`** — project-wide command reference documenting `make test`, `make lint`, `go test ./...`, etc.
- **The plan's `## Testing Approach` section** — names the overall test strategy.
- **The phase-specific notes in the plan's `context.md`** — may list phase-specific verification commands.

If no commands are documented, fall back to the standard Go pipeline: `go build ./...`, `go test ./...`, and whatever linter the project uses.

### Step 2: Run each command

Run every command in sequence. For each one:

- Capture the exit code.
- On failure, capture a **short excerpt** (5–10 lines) of the failing output — typically the test that failed, the file:line where it failed, and the assertion message. Do NOT capture the full test output.
- On success, just note "pass".

### Step 3: Return a concise summary

Return a terse report to the main agent, **one line per command**. Example:

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

Do **not** dump the full test output — the main agent has limited context and needs a digest, not a transcript.

### Step 4: Do not try to fix failures

Your job is to report, not to fix. If a command fails, return the failing excerpt and let the main agent decide what to do. The implement workflow's `verify` step will handle the branching logic (re-run the implement step, ask the user, or abandon the phase).
