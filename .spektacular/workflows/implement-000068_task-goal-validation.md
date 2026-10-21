# Working context: implement 000068_task-goal-validation (orchestrated)

- Orchestrated child of epic 000001_agent-workflow-improvements. Every goto carries "name":"000068_task-goal-validation".
- Code roots (worktrees): spektacular = .spektacular/worktrees/000068_task-goal-validation/spektacular, docs = .spektacular/worktrees/000068_task-goal-validation/docs. Run spektacular CLI only from project root.
- read_plan: structure OK, drift check OK (line numbers shift slightly after 000069; symbols all present). Bob harbor agent already in tests/harbor/agents/ (000069 landed). No ## Changelog yet: first-task mode.
- Tips: docs worktree needs `npm ci` before build; harbor verifier self-checks via `uvx --with pytest pytest -p no:cacheprovider <file>`.
- Design task (plan-task-graph tick rule): child must not write a design; hand back QUESTION with the staged revision.
- Tasks 1-5 done (plan reader, config, goal package, goal commands, reviewer brief). Helpers in scratchpad: tick.py (tick task + criteria), addlog.py (append changelog), g.sh (goto).
- Decisions so far: goal package refuses record while awaiting decision (goal_decision_due); tree fingerprint ignores .spektacular/ paths; skill cmd renders partials.
