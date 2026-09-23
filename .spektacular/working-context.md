# Working context: plan 000057_git-commit

- Planning against spec 000057_git-commit (user chose it). Spec is final; key points: modes off|workflow|full (default off), commits via git CLI with hooks + identity, agent writes message, pre-workflow dirty-tree question, one commit per registered repo, non-git repos skipped, failure halts, no push, docs config reference entry.
- Target repos: spektacular (code) + docs (spektacular-website, configuration.mdx). No design refs on spec.
- Core design: Go executes git via CLI at commit-point transitions; agent passes `commit_message` in goto --data; commit runs AFTER state save (state.json is git-tracked) with rollback on failure; output buffered until commit succeeds. Start check = `uncommitted_changes` report from `new` (resume-report pattern), agent reruns with `commit_existing` true/false. Milestones detected in Go from plan.md checkboxes (full mode). No schema bump.
- Architecture locked: config key `auto_commit`; new pkgs internal/autocommit + internal/gitexec (extracted from repo/git.go); msg via `commit_message_from` path (validated names spec / Milestone N, deleted before commit); resumeOrClear split into probe+clear; stepkit appends partials/commit-message.md at commit-point-leading steps; skills handle `uncommitted_changes`.
- Phases drafted (M1: 1.1 setting, 1.2 engine+gitexec, 1.3 completion commits; M2: 2.1 start gate, 2.2 skills; M3: 3.1 milestone commits, 3.2 docs). Partial = templates/partials/git-commit-message.md; staged msg = .spektacular/tmp/git-commit-message.md; partial inserted before working-context footer.
- Assembled docs staged in .spektacular/tmp/{plan,context,research}_template.md; slug is the CLI-fixed plan name 000057_git-commit (no slug question asked).
- Verification passed (context section order fixed; shell commands removed from plan.md).
- All three docs written to plan store; work dir removed. Now in walkthrough.
- Walkthrough completed: user approved approach, breakdown, out-of-scope and all 15 drafting assumptions with no changes. Advancing to finished.
