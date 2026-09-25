An implementation works from three documents in the plan store. Together they are the approved plan and the only source of truth for what to build. Read each in full with `{{command}} plan file read`, never with the `Read` tool:

- **`plan.md`**: the approved plan. It holds the overview, architecture and design decisions, testing approach, and the `## Milestones & Tasks` checklist of tasks (`## Milestones & Phases` of phases, in a plan written before tasks), whose first unchecked `#### - [ ] Task:` (or `Phase`) heading is the current task. Read it with `{{command}} plan file read <plan_name> plan`.
- **The plan's `context.md`**: the per-task technical detail. It holds one `### Task: <title>` section per task (`### Phase N.M:` per phase in an older plan) with the files to change, complexity and agent strategy. Read it with `{{command}} plan file read <plan_name> context`.
- **`research.md`**: the decision log. It holds rejected alternatives, supporting evidence, files examined and open assumptions. Read it with `{{command}} plan file read <plan_name> research`.

None of these is the working context, `.spektacular/working-context.md`, which holds only a session's notes and never replaces the plan.
