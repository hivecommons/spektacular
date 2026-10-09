---
name: spek-implement
description: Implement an approved Spec by executing its Plan.
---

> **Version check first.** Before running any other command, run `spektacular version check`.
> - On `status: "match"`, continue with the skill and produce no version-related output.
> - On `"mismatch"`, `"missing"` or `"upgrade_needed"`, the project's settings or installed Spektacular files are out of date: relay the response's `action` message to the user, ask them to run `spektacular migrate` (they can preview it with `spektacular migrate --dry-run`), and wait for their decision before continuing.
> - On `"unsupported_format"`, relay the `action` message: the project was written by a newer Spektacular, which the user must install before continuing.
> - Never run `migrate` or `init`, and never modify installed files yourself. Upgrading is always an explicit, user-initiated action.

> **STOP. Read this before running any command below.**
> A single successful CLI call — including the very first `implement new` — is **NOT** task completion. It is not a milestone to report back to the user. It is one step out of many in a workflow that you must keep driving, turn after turn, without stopping, until the CLI itself tells you the workflow is *finished*. If you find yourself about to say "successfully completed" or summarize results after calling `implement new` or `implement goto` even once, you are wrong — go back and read the `instruction` field you just received, do what it says, and call `goto` again.

# What this skill does

This skill drives a **multi-step interactive workflow** that implements a spec by executing its approved plan held in the plan store, producing working code, tests, and a changelog. The workflow is owned by the `spektacular` CLI, not by you — the CLI is the state machine and you are the executor, and the CLI (not the filesystem) is how you reach every plan document.

On each turn, the CLI returns JSON containing an `instruction` field. That instruction describes exactly one step (e.g. analyze, implement a task, verify, update changelog, write the test plan, …). You must:

1. Read the `instruction` carefully.
2. Perform the step — this may mean reading the plan, spawning subagents, editing code, running tests, or writing to the changelog.
3. When the step is complete, run the `goto` command named at the bottom of the instruction to advance the state machine.
4. Read the next `instruction` from the new JSON response and repeat.

**This is a loop. Do not stop after the first step.** Keep looping — step → goto → next instruction → step — until a returned instruction tells you the workflow is *finished*. Only then should you report completion to the user.

**Concretely: do not stop after `implement new`.** That command only starts the workflow — it returns the *first* instruction, not a finished implementation. Seeing a clean JSON response with no `error` is not a signal to stop; it is the signal to keep going. Reporting success, summarizing "implementation initialized," or handing control back to the user at this point is the single most common way this skill is executed incorrectly — do not do it.

# Reading and writing plan files

The CLI owns the plan documents — `plan.md`, the plan's `context.md`, and `research.md`. All plan document access goes through `spektacular plan file`:

- `spektacular plan file read <name> <doc>` — read a plan document from the plan store.
- `spektacular plan file write <name> <doc> --from <source-path>` — write a plan document into the plan store from a source file on disk. Stage the body under `.spektacular/tmp/` first, then `rm` the scratch file after a successful write.
- `spektacular plan file list` — list plans in the plan store.

This includes the edits the implement workflow makes to `plan.md` — ticking task checkboxes and appending changelog entries. Read the document with `plan file read`, apply the change, and commit it with `plan file write`. A plan document is addressed by the feature name and the document name as two arguments, never as a path or with a file extension (e.g. `plan file read my-feature plan`).

# How to start

## The plan documents

An implementation works from three documents in the plan store. Together they are the approved plan and the only source of truth for what to build. Read each in full with `spektacular plan file read`, never with the `Read` tool:

- **`plan.md`**: the approved plan. It holds the overview, architecture and design decisions, testing approach, and the `## Milestones & Tasks` checklist of tasks (`## Milestones & Phases` of phases, in a plan written before tasks), whose first unchecked `#### - [ ] Task:` (or `Phase`) heading is the current task. Read it with `spektacular plan file read <plan_name> plan`.
- **The plan's `context.md`**: the per-task technical detail. It holds one `### Task: <title>` section per task (`### Phase N.M:` per phase in an older plan) with the files to change, complexity and agent strategy. Read it with `spektacular plan file read <plan_name> context`.
- **`research.md`**: the decision log. It holds rejected alternatives, supporting evidence, files examined and open assumptions. Read it with `spektacular plan file read <plan_name> research`.

None of these is the working context, `.spektacular/working-context.md`, which holds only a session's notes and never replaces the plan.

> **Cross-repo implementation.** When the plan attributes work to registered member repos, carry each part of the work out in its attributed repo's code (`spektacular repo list` reports where it lives as `root`), and follow the workflow's changelog instructions to write the central record plus one derived entry per affected repo via `spektacular changelog file write ... --repo <name>`.

Ask the user which spec to implement before proceeding. A spec is implemented through its plan, which shares the spec's name. To enumerate the specs that have a plan, run `spektacular plan file list` — the CLI's list is the source of truth for what counts as a plan. You don't need to look for an in-progress workflow yourself — the CLI detects and reports one for you (see below).

The spec's plan must already exist in the plan store — confirm with `spektacular plan file list`. If it does not, stop and tell the user to run `spektacular plan new` for the spec first.

Start the implement workflow by running:

```
spektacular implement new --data '{"name": "<spec_name>"}'
```

## If the spec depends on specs that are not implemented

A spec in an epic may depend on other specs in it. When one of them is not implemented yet, `implement new` refuses with `code: dependencies_unmet` and starts nothing. Its `message` names each unmet dependency and its state, for example "`<spec>` depends on `<dependency>`, which is in progress (2/5 tasks complete)".

This is a question for the user, not a decision for you. Tell them each unmet dependency and its state, and ask whether to continue anyway:

- **If they choose to continue**, re-run the same command with `"override_dependencies": true` added, exactly as the `next_action` gives it:

  ```
  spektacular implement new --data '{"name": "<spec_name>", "override_dependencies": true}'
  ```

  The workflow starts, and the changelog steps record that implementation started past the unmet dependencies.
- **Otherwise**, offer to implement the first unmet dependency that is ready instead, using the `implement new` command the `next_action` names. When the `next_action` says no dependency is ready yet, tell the user so.

When the project sets `epic.strict_dependencies`, no override exists: `dependencies_unmet` offers no way to continue, and a request that carries `"override_dependencies": true` is refused with `code: dependency_override_refused`. Tell the user each unmet dependency and its state, and offer the ready dependency the `next_action` names. Never add `"override_dependencies": true` without the user's explicit agreement.

Specifying and planning a spec are never held back by its dependencies; only implementation is.

## Implementing one task

A plan written as tasks can be implemented one task at a time. When the user (or an orchestrator) asks for one specific task of a plan, start a single-task run by adding the task's id:

```
spektacular implement new --data '{"name": "<spec_name>", "task": "<task_id>"}'
```

When the user names the task by its title rather than its id, look the id up in the spec's status report first:

```
spektacular status <spec_name> --format json
```

The tasks are under `specs[].plan.tasks`, in the entry whose `name` is the spec; each carries its `id` and `title`. A single-task run still reads the whole plan, its context, research and referenced designs, but implements, tests, verifies and ticks only that task. The feature-level wrap-up (test plan, feature changelog, spec reconciliation) happens only in the run that completes the plan's last open task.

The CLI refuses a task that cannot start, and starts nothing: `task_not_found`, `task_completed`, `task_dependencies_incomplete` (it lists the tasks to implement first) and `task_requires_human` (it gives the reason a person must do it). Relay the refusal and its `next_action` to the user rather than working around it.

**Each spec keeps its own run when worktrees are on.** With `implement.worktrees` on (the default), a run you start keeps its progress and notes in a lane of its own under `.spektacular/workflows/`, keyed by the spec, so runs for different specs can be in progress at once from different terminals. Always pass the spec's name: `implement new --data '{"name":"<spec_name>"}'` resumes that spec's own run when one is in progress, and every `goto` carries the same `name`. A run with no name given only finds a run in the shared slot, and its refusal lists the runs in progress in their own lanes.

**If a workflow was interrupted and is still in progress**, this command does not start a fresh one. Instead it returns a *resume report* — a JSON object with `"resumable": true` plus the in-progress workflow's `kind`, `name`, and `current_step`, and an `instruction` field — and changes nothing on disk. When you get a resume report:

**First check the report's `kind`.** If it is **not** `implement`, a *different* workflow (a spec or plan run) is in progress — you cannot resume it from the implement skill, and the CLI will refuse to. Do **not** run an `implement goto`. Instead follow the report's `instruction`: tell the user a `<kind>` workflow is in progress and let them choose — continue it with that workflow's skill (`spektacular <kind> goto`), or discard it and start the implement run with `spektacular implement new --force`. Only proceed with the steps below when the report's `kind` is `implement`.

1. Ask the user whether to **resume** the in-progress implement run or **start a new one**. (The report's `instruction` field restates both options.)
2. **To resume**, work through these in order:
   1. Read the plan documents listed under **The plan documents** above, in full, before anything else, whichever step the run stopped at.
   2. Read `.spektacular/working-context.md`, the git-tracked working-context file the previous session left behind, for its learnings and the answers the user gave to your questions. It is a session log, not the plan.
   3. Find the current task: the task the report names, for a single-task run, or otherwise the first unchecked `#### - [ ] Task:` heading in `plan.md` (the first unchecked `#### - [ ] Phase` heading in a plan written before tasks).
   4. Run the resume command using the report's `current_step`:

      ```
      spektacular implement goto --data '{"step":"<current_step>"}'
      ```
3. **To start fresh** (discarding the in-progress workflow — it remains recoverable via git), re-run with `--force`:

   ```
   spektacular implement new --force --data '{"name": "<spec_name>"}'
   ```

Otherwise the command returns the first `instruction` and a fresh workflow has started. From that point on, follow the loop above: do what the instruction says, then call `spektacular implement goto --data '{"step":"<next_step>"}'` to get the next one. Do not invent step names — every instruction tells you the exact `goto` command to run next.

## If the project has uncommitted changes

When the project sets `auto_commit` to `workflow` or `full`, `implement new` may instead return an **uncommitted-changes report** (`code: uncommitted_changes`) and change nothing on disk. Its `message` names every registered repository holding uncommitted work, and `resource` lists their names.

This is a question for the user, not a decision for you. Tell them which repositories have uncommitted changes and ask whether to git commit that work **before** the implement workflow starts. Then re-run the same command with their answer:

To commit the existing changes first:

```
spektacular implement new --data '{"name": "<spec_name>", "commit_existing": true}'
```

To start without committing them:

```
spektacular implement new --data '{"name": "<spec_name>", "commit_existing": false}'
```

- `true` commits the existing changes first, in their own commit whose message says they are the user's work from before the workflow. The workflow then starts on a clean tree.
- `false` starts the workflow without committing, so the workflow's own automatic commits will include that work alongside the agent's.

Never choose for the user, and never guess from context which they would want — the whole point of the report is that their uncommitted work is about to be swept into a commit they did not make. If the commit fails (`code: auto_commit_failed`), tell them which repository failed and the reason git gave; the workflow has not started.

# Worktrees

Unless the project sets `implement.worktrees: false`, a run you start builds the spec in its own git worktrees, one per repo its plan touches, on branch `spek/<spec_name>`. Each step says where each repo's code lives: work only there, give every sub-agent those exact locations, and run `spektacular` itself from the project root. Your changes stay out of the main checkouts until the spec is merged back.

- When the plan is complete, the finished step tells you to run `spektacular implement merge --data '{"name":"<spec_name>"}'`. It merges every touched repo or none.
- If the merge is refused, report the repos and conflicting paths to the user and stop. Never resolve a conflict yourself, and never merge, rebase or switch branches on your own initiative: conflicts are the user's to resolve, and the worktrees are kept for them.
- A `worktree_unavailable` or `worktree_setup_failed` refusal from `implement new` means the worktrees could not be made. Report it to the user with its `next_action`.

# When the spec or a design is wrong

Implementing, testing or verifying can show that the spec itself, or a design it references, is wrong: a requirement, acceptance criterion, constraint or success metric is wrong or contradicts another, or contradicts a rule in a referenced design. The implement steps treat this as a stop, and the path out of it is always the same:

1. **Raise it.** Stop, and tell the user the document, the section, the conflict and the amendment you propose. Do not work around it, and do not build to whichever text you prefer.
2. **The user approves.** Apply nothing until the user explicitly approves the amendment. If they decline, carry on as they decide. An amendment that would invalidate the plan's tasks is not this path: the user re-plans.
3. **Apply and record it, in the project.** For the spec, read it with `spektacular spec file read <spec_name>`, change only the Requirements, Acceptance Criteria, Constraints or Success Metrics the user approved, stage the full result under `.spektacular/tmp/<spec_name>/`, and run `spektacular spec amend --data '{"name":"<spec_name>","reason":"<why>","run":"interactive implement run, task <task>"}' --from <staged file>`. For a design, revise it with `spektacular design author`, or store the new version the user supplies with `spektacular design write` for a design they wrote, then record it with `spektacular spec amend` and a `"design":{"source":"<name>","path":"<path>"}` field. `spec amend` appends a dated entry to the spec's `## Amendments` section and records it in the spec's metadata; a recorded amendment does not make the plan stale, so the run carries on.
4. **Pick it up.** Re-read the amended spec or design, re-run the current step's check of the task against it, and name the amendment under **Amendments** in the task's plan changelog entry.

# When an orchestrator starts this skill

The `spek-implement-epic` skill implements a whole epic by starting one agent per spec, each running this skill for one spec. If you were started that way, your prompt says so, and five things change:

- Start with `spektacular implement new --data '{"name":"<spec_name>","orchestrated":true}'`, from the project root you were given. The run keeps its own progress record and notes in a lane and skips the uncommitted-changes question. Running the same command again resumes the lane.
- Every `goto` carries `"name":"<spec_name>"`, exactly as the instructions print it.
- Never ask the user anything yourself, and never ask whether to continue between tasks: tasks run one after another. Hand each genuine question back to your orchestrator as a final message whose first line is `QUESTION: <spec_name>`, and wait for its answer.
- You never write the spec's text or a design document, and never run `spektacular spec amend`. Propose any amendment in your `QUESTION:` hand-back, naming the document, section, conflict and proposed change, and continue once your orchestrator says it is applied.
- End the run with `DONE: <spec_name>` and the completion summary, or with `FAILED: <spec_name>` and the reason.
