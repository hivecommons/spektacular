### Milestone 1: Merging a spec refuses changes to Spektacular's files, and worktrees are recorded in the project

#### - [ ] Task: Record spec worktree code roots in the main project
**Id:** 839eaf80-d6a0-43ef-9249-2b8ca986b3b8
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

When a spec's worktrees are created, write a small record in the main project, beside the spec's worktrees, that maps each touched repo to where its code lives inside them. The record can be read back without running git, and it is deleted when the spec is merged. For now the overlay is still written as well, so epic runs keep working until Milestone 2 switches children over.

*Technical detail:* [context.md#task-record-spec-worktree-code-roots-in-the-main-project](./context.md#task-record-spec-worktree-code-roots-in-the-main-project)

**Acceptance criteria**:
- [ ] After `epic worktree`, the main project holds a record for the spec naming every touched repo's code root inside the spec's worktrees.
- [ ] The record is readable without any git call, and reading it for a spec with no worktrees reports that there is none.
- [ ] After `epic merge`, the record is gone along with the worktrees.
- [ ] The record is never committed to git in the main project.

#### - [ ] Task: Refuse to merge a spec branch that changes the Spektacular directory
**Id:** e3691070-ff05-46e8-b5b0-0869e793c220
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add a check to `epic merge` that runs for every repo before anything is merged. If the spec's branch changes any path under the Spektacular directory, the merge is refused with a new error code. The refusal lists the offending paths per repo, says how to fix it, and nothing is merged anywhere.

*Technical detail:* [context.md#task-refuse-to-merge-a-spec-branch-that-changes-the-spektacular-directory](./context.md#task-refuse-to-merge-a-spec-branch-that-changes-the-spektacular-directory)

**Acceptance criteria**:
- [ ] A spec branch that commits a change under the project's Spektacular directory is refused, with the path named, and no repo's main line moves.
- [ ] A spec branch that commits a change under a sibling repo's Spektacular directory is refused the same way.
- [ ] A branch that changes only code still merges exactly as before.
- [ ] The refusal carries a concrete next action.

### Milestone 2: Epic children build each spec from the project, with the standard implement workflow

#### - [ ] Task: Make the worktree repo view explicit and stop writing into worktrees
**Id:** 8345c4e2-cf84-4f80-b757-54813e6cc116
**Repo:** spektacular
**Depends on:**
- 839eaf80-d6a0-43ef-9249-2b8ca986b3b8 — Record spec worktree code roots in the main project
**Execution:** agent

Building the repo set no longer picks up an overlay from the current directory. A new spec-scoped constructor applies a spec's worktree record when a caller asks for it. Creating worktrees stops writing the overlay into the project worktree, and the overlay's file name, type, reader and git-exclude line are removed. After this, nothing Spektacular does writes inside a worktree's Spektacular directory.

*Technical detail:* [context.md#task-make-the-worktree-repo-view-explicit-and-stop-writing-into-worktrees](./context.md#task-make-the-worktree-repo-view-explicit-and-stop-writing-into-worktrees)

**Acceptance criteria**:
- [ ] After `epic worktree`, the Spektacular directory inside every one of the spec's worktrees is identical to its base commit, with no untracked files.
- [ ] `repo list` run from the main project reports registered locations, even while a spec has worktrees.
- [ ] The spec-scoped repo view relocates only the repos the spec's record maps.

#### - [ ] Task: Tell the implement workflow where spec code lives
**Id:** dd8d60be-3216-4c99-9fdf-9545084bfd6c
**Repo:** spektacular
**Depends on:**
- 839eaf80-d6a0-43ef-9249-2b8ca986b3b8 — Record spec worktree code roots in the main project
**Execution:** agent

`implement new` and `implement goto` read the spec's worktree record from the main project and pass the code roots to the workflow, without git and without saving them in workflow state. The read-plan step's "Where the code lives." block and the feature-changelog step list those roots when they exist. Otherwise they keep today's `repo list` wording word for word, so implementing a spec on its own is unchanged.

*Technical detail:* [context.md#task-tell-the-implement-workflow-where-spec-code-lives](./context.md#task-tell-the-implement-workflow-where-spec-code-lives)

**Acceptance criteria**:
- [ ] For a spec with worktrees, the first implement instruction names each touched repo's worktree code root as where its code lives.
- [ ] For a spec without worktrees, every implement instruction is exactly what it was before this change.
- [ ] `implement new` and `implement goto` still make no git calls and persist no repo roster.

#### - [ ] Task: Split implement auto-commits between worktree code and project artifacts
**Id:** 8616db31-f8c5-4488-b51e-0f07b275c83f
**Repo:** spektacular
**Depends on:**
- 8345c4e2-cf84-4f80-b757-54813e6cc116 — Make the worktree repo view explicit and stop writing into worktrees
**Execution:** agent

When an implement run's spec has worktrees, auto-commit commits code in the spec's worktrees, on its branch. In the main checkouts it commits only that spec's own artifacts: its plan documents, spec, changelog records, scratch and lane files. These are path-scoped commits taken under the existing commit lock, the same way orchestrated plan lanes commit. Implementing a spec without worktrees keeps committing exactly as today.

*Technical detail:* [context.md#task-split-implement-auto-commits-between-worktree-code-and-project-artifacts](./context.md#task-split-implement-auto-commits-between-worktree-code-and-project-artifacts)

**Acceptance criteria**:
- [ ] Code changed during an epic implement run is committed on the spec's branch in its worktree, never on the main checkout's branch.
- [ ] The spec's plan ticks and changelog records are committed in the main checkout, and nothing belonging to another spec or to the user is swept into that commit.
- [ ] Two specs committing at the same time queue on the lock rather than fail.
- [ ] Implementing a spec without worktrees commits exactly as before.

#### - [ ] Task: Read epic implement progress from the main project
**Id:** f2f15f68-42a6-4759-bf09-d6d900160fa1
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

`status` stops reading a spec's implement lane and its finished evidence from inside the spec's worktree, and reads both from the main project. It keeps reporting the spec's worktree as the run's root, and it still reports a finished, unmerged spec as awaiting merge.

*Technical detail:* [context.md#task-read-epic-implement-progress-from-the-main-project](./context.md#task-read-epic-implement-progress-from-the-main-project)

**Acceptance criteria**:
- [ ] A spec with worktrees and an in-progress implement lane in the main project is reported `in_progress`, with its current step and its worktree as root.
- [ ] A spec with worktrees whose plan tasks are all ticked and whose changelog is final in the main project is reported `awaiting_merge`.
- [ ] Nothing under a worktree's Spektacular directory affects what status reports.

#### - [ ] Task: Point epic children at the project root in the skills
**Id:** 432621d7-62ab-4a6c-997f-b2e4d89cfb2b
**Repo:** spektacular
**Depends on:**
- dd8d60be-3216-4c99-9fdf-9545084bfd6c — Tell the implement workflow where spec code lives
**Execution:** agent

Rewrite the epic implement skill's child prompt so the child runs every Spektacular command from the project root and follows `spek-implement`. It no longer works from a worktree or uses `repo list` there, because the workflow's own instructions name the code roots. Update the skill's start, resume and overview wording, the implement skill's orchestrated section, and the `epic worktree` help text to match. Update the template-contract tests that pin these phrases.

*Technical detail:* [context.md#task-point-epic-children-at-the-project-root-in-the-skills](./context.md#task-point-epic-children-at-the-project-root-in-the-skills)

**Acceptance criteria**:
- [ ] The epic implement skill tells each child to run Spektacular from the project root, and no longer tells it to work from, or resolve repos inside, a worktree.
- [ ] The implement skill's orchestrated section no longer mentions worktrees.
- [ ] The `epic worktree` help text no longer says repos resolve inside the project worktree.

#### - [ ] Task: Prove epic spec artifacts stay in the project end to end
**Id:** 6eb73340-282e-43b2-93e7-6b26e9ba24c7
**Repo:** spektacular
**Depends on:**
- e3691070-ff05-46e8-b5b0-0869e793c220 — Refuse to merge a spec branch that changes the Spektacular directory
- 8616db31-f8c5-4488-b51e-0f07b275c83f — Split implement auto-commits between worktree code and project artifacts
- f2f15f68-42a6-4759-bf09-d6d900160fa1 — Read epic implement progress from the main project
- 432621d7-62ab-4a6c-997f-b2e4d89cfb2b — Point epic children at the project root in the skills
**Execution:** agent

Add a cmd-level test that drives one epic spec through worktree creation, an implement run from the main root, and merge, using a real git fixture with a sibling repo. It checks the spec's acceptance criteria together:
- plan ticks and changelog records appear in the main project before the merge;
- the worktrees' Spektacular directories stay untouched;
- the merge brings in code only, and leaves the main project's records unchanged;
- status reports correctly throughout.

*Technical detail:* [context.md#task-prove-epic-spec-artifacts-stay-in-the-project-end-to-end](./context.md#task-prove-epic-spec-artifacts-stay-in-the-project-end-to-end)

**Acceptance criteria**:
- [ ] One test shows a plan tick readable in the main project before merge, and identical after it.
- [ ] The same test shows every worktree's Spektacular directory byte-identical to its base commit at the end of the run.
- [ ] The same test shows the merge changes nothing under any Spektacular directory, and needs no manual reconciliation.

### Milestone 3: The epic documentation describes the new model

#### - [ ] Task: Update the epics page for code-only worktrees
**Id:** b28acf2a-f4ea-4c1a-b1b6-43f98cf81a3c
**Repo:** docs
**Depends on:**
- 432621d7-62ab-4a6c-997f-b2e4d89cfb2b — Point epic children at the project root in the skills
**Execution:** agent

Rewrite the worktree bullet in the "Implement this epic" section of the public epics page. It should say that each spek's worktrees hold only code, and that specs, plans and progress stay in the project, where they can be followed while the run is going. Add a bullet saying that a spek whose branch changes Spektacular's files is not merged.

*Technical detail:* [context.md#task-update-the-epics-page-for-code-only-worktrees](./context.md#task-update-the-epics-page-for-code-only-worktrees)

**Acceptance criteria**:
- [ ] The epics page says worktrees hold only code, and that plans and progress can be followed in the project during a run.
- [ ] It no longer says that, inside a spek's worktrees, repos resolve to the spek's own copy.
- [ ] It says a spek whose branch changes Spektacular's files is refused at merge.
