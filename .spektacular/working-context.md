# Working context — implement 000059_normalise-artifact-addressing

Implementation complete (2026-09-25). All 10 tasks ticked; the user confirmed the manual
end-to-end check. Test plan, project and per-repo (spektacular, docs) changelog records written;
spec reconciled (all requirements and acceptance criteria satisfied).

## Learnings worth carrying forward
- Plan docs are read with `go run . plan file read <f> plan|context|research`.
- restateCommand must check pflag `f.Changed` (Visit leaks flags across test invocations).
- `go run . init <agent>` also rewrites config.yaml (agent, written_by, skills_version).
- Pre-existing, out of scope: unregistered `--repo` on changelog file commands returns internal_error.
