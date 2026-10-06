{{#task}}
This run implements **only task `{{task.title}}`** (id `{{task.id}}`). That task is **the current task**: the `#### - [ ] Task: {{task.title}}` heading in `plan.md` whose `**Id:**` line is `{{task.id}}`, and the `### Task: {{task.title}}` section of the plan's `context.md` that its `*Technical detail:*` link points to. Work only on the current task: do not start, test, verify or tick any other task, even when other tasks are open.
{{/task}}
{{^task}}
**The current task** is the first unchecked work item in the plan's milestones section:

- In a plan whose work is written as tasks (a `## Milestones & Tasks` section), it is the first unchecked `#### - [ ] Task: <title>` heading. Its technical detail is the `### Task: <title>` section of the plan's `context.md` that its `*Technical detail:*` link points to.
- In a plan written before tasks (a `## Milestones & Phases` section), it is the first unchecked `#### - [ ] Phase N.M:` heading. Its technical detail is the matching `### Phase N.M:` section of the plan's `context.md`.
{{/task}}

**Code locations and delegation:** Use the repo `root` paths resolved by `read_plan` from this spec's workflow project root, not your current directory. Pass the workflow project root and each relevant code root explicitly to **every sub-agent** you launch (including task implementers, test authors and verifiers). Require them to edit and run checks from those code roots; the workflow project root is for CLI/store commands, not necessarily code. Never substitute a main checkout for a spec's worktree.

**Worktree dependencies:** `git worktree add` checks out tracked files only: ignored dependencies and build state such as `node_modules`, virtual environments and generated assets are absent. Follow each repo's documented dependency setup in its worktree (for example, `npm ci` when it uses a package-lock), then build and test there. Do not install into the main checkout or share mutable dependencies by symlinking them from it.
