### Milestone 1: Merging a spec refuses changes to Spektacular's files, and worktrees are recorded in the project

**What changes**: `epic merge` refuses a spec whose branch changes anything under the Spektacular directory in any repo. It names the offending paths, gives a way out, and merges nothing. Creating a spec's worktrees also records, in the main project, where each touched repo's code lives inside them. Epic runs otherwise behave as today. This milestone adds the guard and the record that Milestone 2 relies on, and nothing more.

**Validation point**: Worktree and epic command tests show the refusal for a project-repo and a sibling-repo `.spektacular/` change, with nothing merged. They also show the record written on `epic worktree`, readable without git, and removed on merge. the full test suite passes.

### Milestone 2: Epic children build each spec from the project, with the standard implement workflow

**What changes**: A child agent in an epic runs every Spektacular command from the project root. The implement workflow's own instructions tell it where each repo's code lives in the spec's worktrees. Plan ticks, changelog records and progress notes land in the project as they happen. Code is committed on the spec's branch, and only that spec's artifacts are committed in the project. Epic status reads progress from the project, and still names each spec's worktree. Nothing in an epic run touches a worktree's Spektacular directory any longer. The orchestrator's child prompt and the implement skill drop their worktree instructions. Implementing a spec on its own is unchanged.

**Validation point**: cmd-level tests with a real worktree fixture show four things. The instruction names the worktree roots. A plan tick written from the main root is readable in the project before merge. The worktree's `.spektacular/` is byte-identical to its base. Commits split between the worktree (code) and the main checkout (that spec's artifacts only). Status tests show `in_progress` and `awaiting_merge` from main-project records. Template-contract tests pass with the new wording, standalone implement renders exactly as before, and the full test suite passes.

### Milestone 3: The epic documentation describes the new model

**What changes**: The public epics page says that each spec's worktrees hold only code, and that specs, plans and progress stay in the project, where they can be followed during a run. The CLI's own help text for `epic worktree` says the same.

**Validation point**: The docs site builds and type-checks cleanly, and the "Implement this epic" section reads correctly. This is checked by hand and captured in the implementation test plan.
