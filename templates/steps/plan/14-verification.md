## Step {{step}}: {{title}}

The three plan documents were assembled and staged in the previous step at:

- `.spektacular/tmp/plan_template.md`
- `.spektacular/tmp/context_template.md`
- `.spektacular/tmp/research_template.md`

Verify them here. This step **only checks correctness** — it writes nothing to the plan store (the write steps that follow do that). If a scratch file is missing (the `.spektacular/tmp/` path is git-ignored and does not survive a crash), re-assemble it from the per-section working files under `.spektacular/work/{{plan_name}}/` before verifying.

### Step 1: Every required section is present and filled

A common failure mode is silently dropping a section when assembling. Check each staged document against the section list below and confirm the heading is present AND filled with real content (not empty, not a placeholder).

**plan.md — required `##` sections** (in order):

1. `## Overview`
2. `## Conventions`
3. `## Architecture & Design Decisions`
4. `## Component Breakdown`
5. `## Data Structures & Interfaces`
6. `## Implementation Detail`
7. `## Dependencies`
8. `## Testing Approach`
9. `## Milestones & Tasks`
10. `## Open Questions`
11. `## Out of Scope`

**The plan's context.md — required `##` sections** (in order):

1. `## Current State Analysis`
2. `## Per-Task Technical Notes`
3. `## Testing Strategy`
4. `## Project References`
5. `## Token Management Strategy`
6. `## Migration Notes`
7. `## Performance Considerations`

**research.md — required `##` sections** (in order):

1. `## Alternatives considered and rejected`
2. `## Chosen approach — evidence`
3. `## Files examined`
4. `## External references`
5. `## Prior plans / specs consulted`
6. `## Open assumptions`
7. `## Drafting assumptions` (filled with the recorded judgement calls, or an explicit "No drafting assumptions were recorded." line)
8. `## Rehydration cues`

### Step 2: Quality

- **plan.md** — readable in under a minute; no shell commands anywhere. Every task has:
  - an `**Id:**` line holding an id issued by `{{config.command}} plan task-id`, unique in the plan;
  - a `**Repo:**` line naming exactly one registered repo. In a project with more than one registered repo, confirm the line is actually present on every single task, not just the tasks that read as obviously cross-repo — this is the check most likely to be skipped;
  - a `**Depends on:**` line, either `none` or one `- <id> — <title>` entry per dependency, each id belonging to a task in this plan, with no dependency cycle;
  - an `**Execution:**` line, `agent` or `human — <reason>`, decided against the criteria in the tasks step, with any work that needs both an agent and a person split into two tasks;
  - a summary paragraph, a `*Technical detail:*` link, and outcome-based acceptance criteria.

  `{{config.command}} plan file write` enforces the structural rules above and refuses a plan.md that breaks them, naming the task. If a write step is refused, fix the named task in `tasks_plan.md`, re-assemble and retry.
- **The plan's context.md** — per-task technical notes under headings matching plan.md's `*Technical detail:*` anchors.
- **research.md** — alternatives considered and rejected with citations. Dense enough to rehydrate a cold session.

### Step 3: Fix and re-stage

If any section is missing, empty, or fails a quality check, fix it in the **owning working file** under `.spektacular/work/{{plan_name}}/` (the working files are the durable source), then re-assemble the affected scratch file from the working files so the staged document reflects the change. Re-run this verification until every section in every list above is present and filled. Do **not** advance until the staged documents are correct.

This step does not touch the plan store. Never write or edit the plan documents with the `Write` or `Edit` tools — the write steps that follow commit them through `{{config.command}} plan file write`, which is the only supported way.

Then advance:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
```
