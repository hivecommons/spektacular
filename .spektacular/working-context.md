# Working context — implement 000059_normalise-artifact-addressing

Plan: `000059_normalise-artifact-addressing` (10 tasks, 3 milestones). Repos: spektacular
(`/home/nicj/code/github.com/jumppad-labs/spektacular`, branch `f-normalize`) and docs
(`/home/nicj/code/github.com/jumppad-labs/spektacular-website`, branch `f-normalize-commands`).

## read_plan (2026-09-25)
- Structure valid; all 10 task headings resolve in context.md.
- Drift: none. Plan written at 82ceb1b; no code changes since (only the plan commit).
- Spec coverage: every requirement and acceptance criterion covered; nothing descoped.
- Changelog mode: first-task (no `## Changelog` in plan.md).

## Expected hazard (plan Open Question)
Once the command builder is address-driven, the running workflow's own rendered instructions
may say `plan file read <name>/plan.md` and be refused. Run the refusal's `next_action` (same
command, correctly spelled) and carry on. Do not add temporary old-spelling tolerance.
After task 2 lands: read plan docs with `go run . plan file read 000059_normalise-artifact-addressing plan`.

## Decisions / learnings
- User (2026-09-25): "just do the full implementation" — run all tasks autonomously, no continue/pause prompts.
- Task 1 done: `internal/artifact` (Parse, ParseFeature, StorePath, FeatureDir, NameFromEntry, ErrEmptyName).
  StorePath uses slash-concat to stay byte-identical with old helpers.
- Helper scripts in session scratchpad: tick.py (tick a task), clog.py (append changelog entry).
- Task 2 done: address-driven builder + cmd/storefile_address.go (addressRefusal/addressNotFound/restateCommand).
  Plan docs now read with `plan file read <f> plan`. restateCommand must check f.Changed (pflag Visit leaks across test runs).
- To offer the user at the end (knowledge capture): pflag Visit/Changed gotcha.
