## Step {{step}}: {{title}}

For each milestone, break the work into **tasks**. A task is a single unit of work, carried out in exactly one registered repo, by one kind of executor: an agent or a person. Each task has **two outputs**, both finalized during the verification step:

- **plan.md entry** — the task's structured lines, a user-scannable summary and outcome-based acceptance criteria
- **The plan's context.md entry** — dense technical notes with file:line detail

### Task content in plan.md

Each task in plan.md is written exactly in this shape:

```markdown
#### - [ ] Task: <short title>
**Id:** <id from {{config.command}} plan task-id>
**Repo:** <one registered repo name>
**Depends on:**
- <id> — <title of the task it depends on>
**Execution:** agent

<summary>

*Technical detail:* <link to this task's section of the plan's context.md>

**Acceptance criteria**:
- [ ] <outcome statement>
```

- **Heading**: `#### - [ ] Task: <short title>` (a markdown checkbox, not `####` alone). The checkbox is the task's completion.
- **Id**: `**Id:** <id>`. Get every id from Spektacular: run `{{config.command}} plan task-id` once per new task and copy the `id` it prints. Never invent an id, never reuse one, and never change the id of a task that already has one, even when you retitle or move it: other tools refer to tasks by id.
- **Repo**: `**Repo:** <name>`, naming **exactly one** registered repo (its registry name, e.g. `spektacular` or `docs`). Always present, even when the project has only the colocated repo. Work that spans two repos is two tasks.
- **Depends on**: required on every task. Either `**Depends on:** none` on the same line, or `**Depends on:**` followed by one `- <id> — <title>` line per task that must be finished before this one can start. The title is there so a reader can tell which task each id is; only the id is read. There is no implied ordering: a task that depends on nothing says `none`, and tasks that can be done in parallel simply do not depend on each other. Every dependency must be a task in this plan, and dependencies must never form a cycle.
- **Execution**: `**Execution:** agent`, or `**Execution:** human — <reason>` with a non-empty reason, decided against the criteria below.
- **Summary**: 2-4 plain-language sentences explaining what the task does and why. No file:line references. No shell commands. A reader should understand the task from this paragraph alone without opening the plan's `context.md`.
- **Technical detail link** into the plan's `context.md`: `*Technical detail:* [context.md#task-<slug>](./context.md#task-<slug>)`, where `<slug>` is the title lower-cased with spaces as hyphens.
- **Acceptance criteria**: A `**Acceptance criteria**:` heading followed by `- [ ]` checkboxes. Each checkbox is an outcome statement in plain language — something a human can read and understand without running a command. "`spec` and `plan` produce the same JSON output as before the refactor" is good; "`go test ./...`" is not.

### Deciding who carries a task out

Mark a task `human` when completing it needs any of:

- secrets or access an agent will not have (production credentials, cloud consoles, signing keys);
- action outside the repo (deploying, releasing, DNS, purchasing or approving something);
- judgement that must be a person's (legal or licensing, design sign-off, a stakeholder decision);
- verification only a person can do (visual or UX review, physical hardware).

Anything else is `agent`. Write the reason after the type, naming which of these applies: `**Execution:** human — needs access to the production signing key`.

**Split mixed work.** A task that would need both an agent and a person is split into two tasks: the agent's part, and the person's part as its own `human` task that depends on the agent's. For example, adding a release workflow that then needs a production signing secret created is an `agent` task (add the workflow) and a `human` task (create the secret) whose `**Depends on:**` lists the agent task.

### Task content in the plan's context.md

Each task in the plan's `context.md` must have:

- **Heading**: `### Task: <title matching plan.md>` so plan.md's `*Technical detail:*` link resolves.
- **File changes**: Specific file:line changes based on research findings; prefix paths in registered repos other than the colocated one with the repo name (`<repo>:path:line`)
- **Complexity**: Low / Medium / High
- **Token estimate**: ~Nk tokens (rough estimate for agent context usage)
- **Agent strategy**:
  - Low: Single agent, sequential execution
  - Medium: 2-3 parallel agents for independent changes
  - High: Parallel analysis, sequential integration

A `human` task's context entry describes what the person must do and how the result is checked, instead of file changes and an agent strategy.

For guidance on agent orchestration: `{{config.command}} skill spawn-implementation-agents`

### Rules

- Every file change must reference a specific file (and line range where applicable) in the plan's `context.md`.
- NO open questions — resolve any uncertainties now.
- Acceptance criteria in plan.md are outcome statements, not shell commands. Verification commands belong in the agent's head, not in plan.md.
- The task summary in plan.md is the primary artifact the user reads — prioritize clarity over completeness.
- `{{config.command}} plan file write` refuses a plan.md whose tasks break any of the rules above — a missing or duplicate id, a missing, second or unregistered repo, a missing dependency declaration, an unknown dependency, a cycle, or an executor other than `agent` or `human` with a reason — and names the task. Getting them right here saves a failed write later.

Before advancing, save this step's work to its **two** working files. Using your own `Write` tool, write:

- the **plan.md** task content (the `#### - [ ] Task:` headings with their `**Id:**`, `**Repo:**`, `**Depends on:**` and `**Execution:**` lines, summaries, technical-detail links, and acceptance criteria described under "Task content in plan.md" above) to `.spektacular/work/{{plan_name}}/tasks_plan.md`, and
- the **context.md** task content (the `### Task:` headings with file:line detail, complexity, token estimate, and agent strategy described under "Task content in context.md" above) to `.spektacular/work/{{plan_name}}/tasks_context.md`.

Both working files are git-tracked and are read back on resume and when the plan documents are assembled, so they must hold the final content. They are **not** plan store documents — write them directly with your file tools and do **not** route them through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted tasks are saved to both working files, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
