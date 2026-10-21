# Implement 000069_compact-agents-md — working context

- Orchestrated run (epic 000001_agent-workflow-improvements). Code lives in worktrees:
  - spektacular: .spektacular/worktrees/000069_compact-agents-md/spektacular
  - docs: .spektacular/worktrees/000069_compact-agents-md/docs
- read_plan: structure OK, drift only minor line offsets (omp.go installOMPRules now at :48). Spec coverage complete. No changelog yet (first task).
- Task 1 (harbor suite) analysis: `repo add` takes `location` = repo root; `spec file read` prints raw markdown; session-log `command` excludes the binary name (`["repo","list"]`). Bob agent finds bob.json via `Path(__file__).with_name`, so moving all three files together is enough. README lists harbor make targets (lines ~406-420): add the new ones there.
- Task 1 implement: verifier rules are pure functions returning a violation message or None; offer phrases broadened slightly beyond the plan's four (added "write it up", "would you like", "shall i"). Seed leak check greps for REORDER_THRESHOLD/needs_reorder rather than bare 37 (timestamps). Makefile copies only test.sh + test_standing_rules.py (+ shared/*.py), so the self-check stays out of the image.
- Task 1 test: self-check at tests/harbor/standing-rules/tests/test_verifier_selfcheck.py (38 tests). Local python3 lacks pytest: run with `uvx --with pytest pytest -p no:cacheprovider ...`. Feature-offer rule now matches "spec" as a whole word. Docker builds for claude/codex/bob images verified; in-container verifier dry run: compliant -> reward 1, violating -> 6 rules fail.
- Task 1 verify: all green (docker render/build, self-check 38 pass, make -n targets, Bob agent paths, go build/test/vet).
- Task 1 done (Milestone 1 complete, commit made on advance). Helpers: scratchpad tick.py / addlog.py for plan edits.
