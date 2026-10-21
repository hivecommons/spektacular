# Interview: readable commit history

## What is being built

Spektacular's automatic git commits should read like a human wrote them and there should be fewer of them. Today a three-milestone spec implemented in a worktree produces 11 commits, and every subject is an identifier followed by a milestone label before the verb. The user saw this on PR #86: 25 commits, 23 of which came from three epic specs, and subjects like `000066_epic-mid-run-revisions: Milestone 1 — record spec amendments without staling the plan`.

## Who it is for

Anyone reading a Spektacular project's git history: the user reviewing a PR, a teammate reading `git log`, and future archaeology. The traceability (which spec, which milestone) must survive, but it must stop being the first thing in the subject.

## Three parts, all in scope

### 1. Traceability moves into git trailers

- Commit subjects become a short imperative sentence, as any hand-written commit: `Record spec amendments without staling the plan`, `Specify implement runs in worktrees`, `Plan implement runs in worktrees`.
- The spec name and milestone go into trailers at the end of the body. Key for the spec is `Spek:` (user's choice; matches the project's vocabulary and avoids clashing with other tooling). The milestone goes in a `Milestone:` trailer; one trailer per milestone the commit closes, since a commit can close more than one.
- The body still says what was specified, planned or implemented.
- Validation (`ValidateMessage`) checks the trailers rather than searching the subject. The CLI keeps refusing a message that lacks the trailers it needs.
- The instruction the agent receives (`templates/partials/git-commit-message.md`) changes to ask for this shape: it currently says "The subject line must name `<spec>`" and "spelled `Milestone N`".
- Nothing in the Go code reads commit subjects back today (verified by grep), so no in-tree consumer depends on the old shape. External consumers can use `git log --format=%(trailers:key=Spek)` or `git interpret-trailers`.
- `PreWorkflowMessage` (the commit of the user's own pre-existing changes) should carry the same trailer so it is findable too, but keep its plain subject.

### 2. One store-side commit per worktree run, not one per milestone

- In a worktree run the code commits on the `spek/<name>` branch and the project checkout commits the `.spektacular/` progress (plan checkboxes, lane files) with a copy of the same message, once per milestone. The duplicate subjects are the most visible noise in the history.
- Required outcome: the project-side bookkeeping for a worktree run is committed once per run (at finish, or folded into the merge), not once per milestone. Milestone-level code commits on the branch are unchanged.
- The store-side commit's message must say what it is (recording progress for the run) rather than reusing the code commit's subject.

### 3. Opt-in squash on merge

- A configuration switch, off by default, under which the orchestrator merges a spec's worktree branch as a single implement commit per spec. The milestone commits remain on the branch until it is deleted; the squashed commit's body lists the milestones and carries the `Spek:` trailer.
- Default behaviour (switch off) keeps today's merge: milestone commits appear in the merged history.
- Key name and exact placement in config.yaml are for the plan to decide; it belongs alongside `auto_commit`.

## Docs

- The docs site (repo `docs`, spektacular-website, role documentation) describes `auto_commit` on `configuration.mdx` (ConfigKey around line 182) and must describe the new message shape, the once-per-run store commit, and the squash switch. In scope for this spec.
- README's auto_commit paragraph and the `config.yaml` comment in the README likewise.

## Constraints

- Commits still use the user's own git, hooks, identity and signing; never pushed. Unchanged.
- `auto_commit` modes `off | workflow | full` keep their meaning. `workflow` remains the way to get one commit per finished workflow; this spec does not change the default mode. (Noted as a quick win for this project: switch to `workflow`.)
- Existing projects with history in the old shape are not rewritten.
- The validator must still refuse an empty message, a message missing the spec, and a message missing a milestone it closes; only *where* it looks changes.

## Explicitly out of scope

- Changing the default `auto_commit` mode.
- Rewriting existing history or migrating old commit messages.
- Reducing the number of workflows (spec, plan, implement) that each make a completion commit.
- PR-level squash merging on GitHub; that is the user's own choice outside Spektacular.

## Success looks like

- `git log --oneline` on an epic branch reads as a list of sentences; no subject starts with a spec identifier or "Milestone N".
- A three-milestone worktree run adds 1 spec + 1 plan + 3 milestone + 1 finish + 1 merge commit with the switch off, and 1 + 1 + 1 (squashed) with it on, plus one project-side progress commit either way, instead of 11.
- `git log --format='%s %(trailers:key=Spek,valueonly)'` still lists every commit for a spec.
