# Working context — implement 000059_normalise-artifact-addressing

Plan: `000059_normalise-artifact-addressing`. Repos: spektacular (branch `f-normalize`) and docs
(`../spektacular-website`, branch `f-normalize-commands`).

## State (2026-09-25)
- Every agent task is done, ticked and logged in the plan's `## Changelog`. Milestones 2 and 3 are
  committed (auto_commit) in both repos.
- ONLY OPEN TASK: "Manually test a full run with the new addressing" (human). The workflow is
  parked at `analyze`. After the person runs it and it passes: tick that task in plan.md (via
  `plan file read/write <f> plan`), go through update_changelog, then `goto test_plan` (a Milestone 1
  commit message will be required), then update_feature_changelog and reconcile_spec.
- CHANGELOG.md already has the 000059 entry with the breaking-change paragraph; the
  update_feature_changelog step should verify it, not add a second one.

## Decisions / learnings
- User (2026-09-25): "just do the full implementation" — run autonomously, no continue prompts.
- Plan docs are read with `go run . plan file read <f> plan|context|research` now.
- restateCommand must check pflag `f.Changed` (Visit leaks flags across test invocations).
- `go run . init <agent>` also rewrites config.yaml (agent, written_by, skills_version):
  restore config.yaml after regenerating skill copies.
- Pending knowledge-capture offers for the user: the pflag Visit/Changed gotcha; init rewriting config.yaml.
- Pre-existing, out of scope: unregistered `--repo` on changelog file commands returns internal_error.
