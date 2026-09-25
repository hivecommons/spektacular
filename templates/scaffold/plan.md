# Plan: {{name}}

<!-- Metadata -->
<!-- Created: -->
<!-- Commit: -->
<!-- Branch: -->
<!-- Repository: -->

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the plan. Answer:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits?
  No file paths, no commands, no implementation detail. A reviewer should be
  able to decide whether the plan is worth reading in full from this section
  alone.
-->
## Overview



<!--
  CONVENTIONS
  The project conventions (always-apply standards) that bear on this work,
  each with a one-line reason it applies — and only the relevant ones, not
  the whole knowledge base. Conventions are loaded in full during discovery
  and the relevant subset is chosen as the design is locked, then confirmed
  by the user. Cite a convention inline in the sections it drives. If no
  conventions are relevant (or the project has none), state that plainly,
  e.g. "No project conventions apply to this feature." An empty or generic
  list is a visible signal the knowledge base was not consulted.
-->
## Conventions



<!--
  ARCHITECTURE & DESIGN DECISIONS
  The chosen design direction in 2-4 short paragraphs. Explain the shape of
  the solution, the key decisions and their trade-offs, and why the chosen
  direction beats the alternatives. Cross-reference
  research.md#alternatives-considered-and-rejected so readers can drill into
  the evidence for rejected options. This is plan.md's load-bearing section —
  a reviewer should be able to spot missing architectural patterns or design
  gaps from this section without needing to read context.md.
-->
## Architecture & Design Decisions



<!--
  COMPONENT BREAKDOWN
  The components (new or changed) that make up the solution, with their
  responsibilities and how they interact. One bullet or short paragraph per
  component. Name the component, state what it owns, and describe its
  relationship to the other components. Do not list file paths or line
  numbers here — component responsibilities, not implementation sites.
-->
## Component Breakdown



<!--
  DATA STRUCTURES & INTERFACES
  The types, interface signatures, and serialization boundaries introduced or
  changed by the plan. Show type shapes in pseudocode or a short code block
  where useful. Focus on the contract between components, not internal
  representation detail.
-->
## Data Structures & Interfaces



<!--
  IMPLEMENTATION DETAIL
  High-level only. Sketch new patterns being introduced, major code-shape
  changes, and code-structure UX — enough for a reviewer to spot missing
  patterns or design gaps. This is NOT per-task file:line work — that
  belongs in context.md. If you find yourself writing "in file X at line Y",
  stop and move it to context.md.
-->
## Implementation Detail



<!--
  DEPENDENCIES
  The internal packages, external libraries, upstream specs, or prior plans
  this work depends on. One bullet per dependency with a one-line note on
  what it provides and whether it needs any changes.
-->
## Dependencies



<!--
  TESTING APPROACH
  High-level overview of the testing strategy: what kinds of tests
  (unit, integration, contract, regression), which components get the most
  coverage, and what the load-bearing assertions are. Per-task testing
  detail — which specific tests live in which specific files — stays in
  context.md.
-->
## Testing Approach



<!--
  MILESTONES & TASKS
  2-4 milestones. Each milestone leads with a "What changes" summary
  paragraph describing the user-visible difference when the milestone lands.
  Each task is one unit of work in exactly one registered repo, for one
  executor, and carries **Id:** (from `plan task-id`), **Repo:**,
  **Depends on:** (`none` or `- <id> — <title>` lines) and **Execution:**
  (`agent`, or `human — <reason>`) lines. Each task has a 2-4 sentence
  plain-language summary, a *Technical detail:*
  link to context.md, and an **Acceptance criteria**: checkbox list with
  outcome statements (not shell commands). No file:line references in
  plan.md task content — those live in context.md.
-->
## Milestones & Tasks

### Milestone 1: <user-facing title>

**What changes**: <one-paragraph description of the user-visible difference when this milestone lands. Written in plain language, no file paths, no commands.>

#### - [ ] Task: <short title>
**Id:** <id from plan task-id>
**Repo:** <one registered repo name>
**Depends on:** none
**Execution:** agent

<2-4 sentence summary of what this task does and why. Plain language. No file:line references. No shell commands.>

*Technical detail:* [context.md#task-<slug>](./context.md#task-<slug>)

**Acceptance criteria**:

- [ ] <outcome statement in plain language>
- [ ] <outcome statement in plain language>


<!--
  OPEN QUESTIONS
  Strictly for questions that genuinely cannot be resolved until
  implementation begins. Anything resolvable by asking the user, reading the
  code, or running a quick experiment must be resolved now — not parked
  here. If this section is empty, that is the expected outcome of a healthy
  planning pass.
-->
## Open Questions



<!--
  OUT OF SCOPE
  Explicit exclusions agreed during planning. Each bullet states what is NOT
  being done and, where useful, where it is tracked instead. This is as
  important as the requirements — it prevents scope creep and sets clear
  expectations for reviewers.
-->
## Out of Scope


