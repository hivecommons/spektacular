# Working context: implement 000072_knowledge-access-index-and-prune (orchestrated)

## Decisions
- read_plan: structure valid (10 tasks, 4 milestones), no drift found against the worktree at a8ba63d, every spec requirement and acceptance criterion covered. No changelog yet: first-task invocation.

## Learnings
- Task 1 (lock + Update) done. `autocommit.LockError` kept as its own type; `filelock.AcquireWithBackoff` added. Tests contending on Update must shorten `filelock.Backoff` (else ~14s).
- `newKnowledgeSet` in cmd/knowledge.go replaces `cfg.Knowledge` with only Sources, so prune must read `cfg.Knowledge.PruneMaxAgeDays()` from its own `loadConfig()`.
- Helpers in the scratchpad: stage.sh (goal-check staging), tick.py <task-id>, changelog.py <entry-file>.
- Commit messages only when a milestone's last task is ticked (subject names spec + "Milestone N").
- Task 2 (usage index) done: reconcile drops records only for files that no longer exist; listFiles filters reserved paths.
- Task 3 done (Milestone 1 complete, committed). knowledgeNow clock var in cmd/knowledge_usage.go; writeTemp/readUsage/fixKnowledgeClock helpers in cmd/knowledge_usage_test.go.
