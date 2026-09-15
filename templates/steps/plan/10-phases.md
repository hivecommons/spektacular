## Step {{step}}: {{title}}

For each milestone, define implementation phases. Each phase has **two outputs**, both finalized during the verification step:

- **plan.md entry** — a user-scannable summary and outcome-based acceptance criteria
- **The plan's context.md entry** — dense technical notes with file:line detail

### Phase content in plan.md

Each phase in plan.md must have:

- **Heading**: `#### - [ ] Phase N.M: <short title>` (markdown checkbox, not `####` alone)
- **Repo**: A `**Repo:**` line immediately under the heading, naming the registered repo the phase's work is carried out in (its registry name, e.g. `spektacular` or `docs`). Always present, even when the project has only the colocated repo, or when every phase in the plan targets the same repo — the reader should never have to infer this from prose or compare against other phases. If a phase's work spans more than one repo, list all of them.
- **Summary**: 2-4 plain-language sentences explaining what the phase does and why. No file:line references. No shell commands. A reader should understand the phase from this paragraph alone without opening the plan's `context.md`.
- **Technical detail link** into the plan's `context.md`: `*Technical detail:* [context.md#phase-NM](./context.md#phase-NM-<slug>)`
- **Acceptance criteria**: A `**Acceptance criteria**:` heading followed by `- [ ]` checkboxes. Each checkbox is an outcome statement in plain language — something a human can read and understand without running a command. "`spec` and `plan` produce the same JSON output as before the refactor" is good; "`go test ./...`" is not.

### Phase content in the plan's context.md

Each phase in the plan's `context.md` must have:

- **Heading**: `### Phase N.M: <title matching plan.md>` so plan.md's `*Technical detail:*` link resolves.
- **File changes**: Specific file:line changes based on research findings; prefix paths in registered repos other than the colocated one with the repo name (`<repo>:path:line`)
- **Complexity**: Low / Medium / High
- **Token estimate**: ~Nk tokens (rough estimate for agent context usage)
- **Agent strategy**:
  - Low: Single agent, sequential execution
  - Medium: 2-3 parallel agents for independent changes
  - High: Parallel analysis, sequential integration

For guidance on agent orchestration: `{{config.command}} skill spawn-implementation-agents`

### Rules

- Every file change must reference a specific file (and line range where applicable) in the plan's `context.md`.
- NO open questions — resolve any uncertainties now.
- Acceptance criteria in plan.md are outcome statements, not shell commands. Verification commands belong in the agent's head, not in plan.md.
- The phase summary in plan.md is the primary artifact the user reads — prioritize clarity over completeness.

Before advancing, save this step's work to its **two** working files. Using your own `Write` tool, write:

- the **plan.md** phase content (the `#### - [ ] Phase N.M:` headings, summaries, technical-detail links, and acceptance criteria described under "Phase content in plan.md" above) to `.spektacular/work/{{plan_name}}/phases_plan.md`, and
- the **context.md** phase content (the `### Phase N.M:` headings with file:line detail, complexity, token estimate, and agent strategy described under "Phase content in context.md" above) to `.spektacular/work/{{plan_name}}/phases_context.md`.

Both working files are git-tracked and are read back on resume and when the plan documents are assembled, so they must hold the final content. They are **not** plan store documents — write them directly with your file tools and do **not** route them through `{{config.command}} plan file write` (that command is only for the final plan documents).

**Record your judgement calls.** If drafting this section required a judgement call — a decision made on a reasonable default instead of asking the user — append one entry per call to `.spektacular/work/{{plan_name}}/assumptions.md` using your own `Write` tool (create the file on first use):

```markdown
### <short decision title> (<step name>)
- **Decision**: what was chosen
- **Rationale**: why this was the reasonable default
- **Rejected**: alternatives considered and why not
```

**Proceed unless genuinely blocked.** Do not stop to present this section for review or approval. Only when a decision has no reasonable default — mutually exclusive directions you cannot responsibly choose between, or information only the user holds — STOP and present the options to the user in one block, and do not advance past the point that depends on the answer until they respond. Otherwise proceed without interruption.

Once the drafted phases are saved to both working files, advance:

{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
