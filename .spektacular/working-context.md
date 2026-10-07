# Working context — plan 000065_implement-in-worktrees

Plan workflow started 2026-10-07 for spec 000065_implement-in-worktrees (worktrees for every implement run, merge-back, repo worktree setup command, sub-agent code-location guidance, narrowed epic dirty check).

## Spec-time decisions carried forward
- Worktrees default ON for every implement run; config opt-out; epic runs always use worktrees.
- Standalone run merges back all-or-nothing at end (reuse epic merge); on conflict refuse naming paths; agent never resolves.
- Spec branch always gets commits regardless of auto_commit.
- Unmerged-but-implemented dependency = unmet; warn/refuse per design epics-and-seeded-specs.md (epic.strict_dependencies).
- Reuse PR #79 dirty_repos approach (RunSource.Dirty(repos) []string; epic.run.dirty_repos alongside dirty bool). Don't reuse its wording.
- Docs repo in scope.

## Plan-time decisions / answers
(none yet)
- (discovery) Non-git / no-commit touched repo on a standalone worktree run → REFUSE with remediation (commit it, or set opt-out). User chose.
- (discovery) Merge trigger: finished step instructs agent to run new `implement merge` command (shares worktree.Manager.Merge with epic merge). User chose.
- (discovery) Single-task runs merge only when the plan is complete; partial task runs keep/reuse worktrees. User chose.

- (architecture) Direction recorded in assumptions.md; new cmd `implement merge`; config key `implement.worktrees`; repo.yaml `worktree_setup`; partial `implement-code-locations.md` for steps 02-05.

- (milestones) M1 epic fixes (setup, sub-agent partial, dirty_repos); M2 forced commits + implement merge + unmerged deps; M3 standalone worktrees default on + opt-out + finished merge + skill + harbor; M4 docs.

- (tasks) User chose: worktree-on standalone runs use per-spec lanes (data lane:true, separate from orchestrated) so two terminals work. New task e74f4569.

- (write) plan, context, research committed to store; working dir removed.
- (walkthrough) 2026-10-07: user reviewed approach and milestones/tasks, then signed off ("ok perfect just commit"), skipping the out-of-scope and assumptions beats. No changes requested. Next: implement 000065.

## Learnings
- Repos: spektacular (root = this dir), docs (root /home/nicj/code/github.com/hivecommons/spektacular-website).
- Design ref design:epics-and-seeded-specs.md resolved; requires implement check and status to share one dependency classifier → put "unmerged" in status.DependenciesOf (record exists ⇒ unmerged).
- Config parses over NewDefault(), so default-true bool needs no schema bump. Chose `implement.worktrees`.
- worktree.Ensure/ensureOne(made) is the hook for repo.yaml `worktree_setup`.
- With auto_commit off PointFor=PointNone → need forced code-only commits at completion for worktree runs (stepkit LeadsToCommit too).
- Dependency check today always refuses with override offer (no plain warn).
- Harbor implement suite needs git fixture or opt-out once default on.

## Implement run (started 2026-10-07)
- Plan approved & committed (0adbaf0). Implement workflow started, non-orchestrated, auto_commit on.
- Drift: cmd/workflow_slot_test.go and internal/status/report_test.go don't exist. User chose "proceed and adapt": status unmerged tests go in internal/status/status_test.go; ignore the slot-test note.
- Spec coverage: complete. Changelog mode: first task.
- Task 98c6a685 (worktree_setup field): guided add (internal/steps/repo/registration.go) deliberately not extended; only `repo add --data` sets it (plan scope).
- Task 98c6a685 tests written (config/repo/cmd/templates guided_add_skill_test).
- Task 98c6a685 verified: build/vet/gofmt/full go test/make lint green.
- User chose: run all remaining tasks without asking (stop only for failures/real questions).
- Task acc28f89: setup runs in Manager.setUp after ensureOne made=true, only for touched repos; failure -> discard worktree (+branch if created) and worktree_setup_failed; record not written. cmd var worktreeSetup wired into worktreeManager().
- Task 182b302a: PointForRun/LeadsToCommitForRun/CodeOnly in autocommit; worktree flag = len(wfCfg.CodeRoots)>0; git-commit partial has commit.code_only wording; commitImplementLane(codeOnly) skips main-checkout artifact commit.
- Task 7462c057: Options.Unmerged + metBy in buildTarget BlockedBy; DescribeDependency; unmergedFn(root) in cmd/implement.go wired into refuseUnmetDependencies and status. Unmerged dep next_action offers implement merge. Status has no dependency descriptions (only ready/blocked_by) — deviation noted.
