## Step {{step}}: {{title}}

Identify the current phase, then research the codebase touchpoints before writing any code.

### Step 1: Pick the current phase

Re-read plan.md through the plan store — `{{config.command}} plan file read {{plan_name}}/plan.md` — and locate the first unchecked `#### - [ ] Phase N.M:` heading under `## Milestones & Phases`. That is **the current phase**. Record its number (e.g. `1.2`), its title, and its `*Technical detail:*` link to a section in the plan's `context.md`.

If every phase is already checked, STOP — this should only happen if the user manually advanced the workflow past `update_changelog` without looping. Report the situation and ask the user what to do.

### Step 2: Read the phase's technical detail

Read the plan's `context.md` through the plan store — `{{config.command}} plan file read {{plan_name}}/context.md` — and find the `### Phase N.M:` heading the `*Technical detail:*` link points to. Read the entire phase section. It should contain file:line references, complexity, token estimate, and an agent strategy.

Always read the plan documents with `{{config.command}} plan file read`, never with the `Read` tool. If the section is missing, unreadable, or empty, STOP and ask the user whether to fix the plan's `context.md` before proceeding. This is a plan/reality mismatch — do not guess.

### Step 3: Delegate codebase research to sub-agents

For non-trivial phases (Medium or High complexity), delegate the codebase research to sub-agents running in parallel. For Low-complexity phases, you can do it yourself in the main context. Use the skill below for orchestration guidance:

```
{{config.command}} skill spawn-implementation-agents
```

The research should cover:

1. Every file:line reference listed in the phase section of the plan's `context.md` — confirm each still exists and the line numbers are approximately correct (drift may have moved them slightly).
2. The integration points where new code will sit — imports, callers, interfaces that need to be satisfied.
3. Existing patterns to follow — similar implementations elsewhere in the codebase that the new code should match in shape.
4. Tests that will need to be updated or added.

Each sub-agent should return a concise summary (not full file dumps) that the main agent can use as a reference when writing code.

### Step 4: STOP-on-mismatch

If any sub-agent reports that a file referenced by the phase no longer exists, or that a named function/type/package has been renamed or removed, STOP immediately. Report the mismatches to the user and ask whether to (a) fix the plan first, (b) proceed with an agreed-upon substitution, or (c) skip this phase.

### Advance

Once analysis is complete and you have a clear picture of the files, patterns, and integration points for the current phase:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}"}'
```
