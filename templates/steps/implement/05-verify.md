## Step {{step}}: {{title}}

Run the verification commands for the current task yourself, in this context, and keep only a short result. Do not launch a sub-agent for them: trimming each command's output keeps the full log out of your context instead.

### Step 1: Load the current task

{{> partials/implement-current-task}}

You need the current task's heading and acceptance criteria from `plan.md`, and its whole section of the plan's `context.md`. When they are already in your context from an earlier step of this task, use them as they are: do not read the documents again. Read them only after a resume or a context compaction, or when the plan has changed since you read it:

1. Run `{{config.command}} plan file read {{plan_name}} plan` and find the current task's heading.
2. Run `{{config.command}} plan file read {{plan_name}} context` and read the current task's section of the plan's `context.md` in full.

If the section is missing, unreadable, or empty, STOP and ask the user whether to fix the plan's `context.md` before proceeding. This is a plan/reality mismatch — do not guess.

{{> partials/implement-code-locations}}

### Step 2: Run the verification commands

Follow the instructions from the skill below, reading it once per session rather than once per task:

```
{{config.command}} skill verify-implementation
```

1. Take the current task's acceptance criteria from the plan's `plan.md`. Verify that task alone.
2. Map each criterion to a concrete verification command (typically `make test`, `make lint`, or a task-specific command listed in the current task's section of the plan's `context.md` or `thoughts/notes/commands.md`).
3. Run each command from the source of the repo the task's work landed in, with its output trimmed so a passing run prints almost nothing (for example by piping it through `tail -n 20`, or a quiet flag). Keep each exit code and a short excerpt of any failure.
4. Write down a **concise pass/fail summary**: one line per command, no full test output. If everything passes, a single "all green" line is enough. The goal check hands this summary to its reviewer.

### STOP-on-mismatch

If any verification command fails, STOP. Report the failures to the user, with their excerpts. Do not advance to the next step — the user must decide whether to:

1. **Fix the code and re-run verification.** Return to the `implement` step with the fixes in mind.
2. **Accept the partial failure and proceed.** Only if the user explicitly chooses this.
3. **Abandon this task.** {{#task}}End this run without ticking it.{{/task}}{{^task}}Skip to the next unchecked task.{{/task}}

{{> partials/implement-spec-conflict}}

### Advance

Once verification is green (or the user has explicitly authorized proceeding past a failure):

```
{{config.command}} implement goto --data '{"step":"{{next_step}}","name":"{{plan_name}}"}'
```
