## Step {{step}}: {{title}}

Assemble the three plan documents from the per-section working files you wrote during the earlier steps and stage them to scratch. This step **only assembles and stages** — it writes nothing to the plan store. The next step verifies what you stage here; the write steps after that commit it.

### Step 1: Gather Metadata

Use the `gather-project-metadata` skill to collect: ISO timestamp, git commit, branch, and repository info.
For skill details: `{{config.command}} skill gather-project-metadata`

### Step 2: Determine Feature Slug

Use the `determine-feature-slug` skill to determine the plan file namespace and number.
For skill details: `{{config.command}} skill determine-feature-slug`

### Step 3: Fill in the Three Scaffolds

Assemble all three scaffolds from the per-section working files — no placeholders, no open questions. The working files hold body content only; each scaffold below owns its headings and their order. Drop each working file's content under the matching heading.

**plan.md** ← files under `.spektacular/work/{{plan_name}}/`:
- `conventions.md` → `## Conventions`
- `architecture.md` → `## Architecture & Design Decisions`
- `components.md` → `## Component Breakdown`
- `data_structures.md` → `## Data Structures & Interfaces`
- `implementation_detail.md` → `## Implementation Detail`
- `dependencies.md` → `## Dependencies`
- `testing_approach.md` → `## Testing Approach`
- `milestones.md` + `tasks_plan.md` → `## Milestones & Tasks`
- `open_questions.md` → `## Open Questions`
- `out_of_scope.md` → `## Out of Scope`
- `## Overview` ← derive from the spec you read in step 01 and your `.spektacular/working-context.md` notes.

**The plan's context.md** ← files under `.spektacular/work/{{plan_name}}/`:
- `tasks_context.md` → `## Per-Task Technical Notes`
- `testing_approach.md` → `## Testing Strategy` (recast at per-task granularity)
- `## Current State Analysis`, `## Project References`, `## Token Management Strategy`, `## Migration Notes`, `## Performance Considerations` ← your research findings in `research.md` and `.spektacular/working-context.md`.

**research.md** ← files under `.spektacular/work/{{plan_name}}/`:
- `research.md` → the research sections it maps directly onto (alternatives through rehydration cues)
- `assumptions.md` → `## Drafting assumptions`

If a required working file is missing, the matching gathering step was not completed — STOP and complete it before assembling. In particular, confirm both `tasks_plan.md` and `tasks_context.md` exist, since the tasks step writes two files that feed plan.md and the plan's `context.md` respectively. The one exception is `assumptions.md`: a plan can legitimately record zero judgement calls, so if it is missing do **not** STOP — write an explicit "No drafting assumptions were recorded." line under `## Drafting assumptions` instead.

#### plan.md scaffold

```markdown
{{plan_template}}
```

#### The plan's context.md scaffold

```markdown
{{context_template}}
```

#### research.md scaffold

```markdown
{{research_template}}
```

### Step 4: Stage each assembled document to scratch

Using your own `Write` tool, write each assembled document to its scratch path under `.spektacular/tmp/`. These staged files are what the verification step reads and what the write steps commit — **nothing is written to the plan store in this step**:

- plan.md → `.spektacular/tmp/plan_template.md`
- the plan's `context.md` → `.spektacular/tmp/context_template.md`
- research.md → `.spektacular/tmp/research_template.md`

Staging to `.spektacular/tmp/` with your own `Write` tool is correct here. Do **not** run `{{config.command}} plan file write` yet — committing to the plan store happens in the write steps.

Then advance:

```
{{config.command}} plan goto --data '{"step":"{{next_step}}"}'
```
