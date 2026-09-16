---
created_date: "2026-09-16"
document_status: final
closed_date: "2026-09-16"
---

# Test Plan: 000052_document-status-vocabulary

Manual procedures for the parts of this change that automated tests can't cover. The other success metric ("no artifact read fails because of its document status") is covered by the metadata and CLI tests.

## 1. Planning agents no longer question a finished spec

**What to measure**: Give a planning agent a spec whose frontmatter says `document_status: final`. The agent must go ahead with planning. It must not ask whether the feature has already been built, and it must not treat the spec's status as evidence that the work is done.

**How**:
1. Build the CLI from this branch in a scratch project: `go build -o /tmp/spek .`, then `/tmp/spek init claude --name status-check`.
2. Write a spec with `/tmp/spek spec file write 000001_demo.md --from demo-spec.md --document-status final`, where `demo-spec.md` is a small, realistic spec, for example "add a `--json` flag to a hello command".
3. In Claude Code, inside the scratch project, run `/spek-plan` and choose `000001_demo`.
4. Read the agent's first few turns.

**Expected result**: Pass if the agent doesn't bring up the spec's status as a reason to doubt the task (for example "the spec is marked final/complete, has this already been implemented?") in 3 out of 3 fresh sessions.

**Who / when**: The maintainer, once, before this change is released.

## 2. Implement-workflow end-to-end suite passes against `document_status`

**What to measure**: The harbor implement-workflow suite, with its oracle updated to `document_status: final` and fixtures seeded with `document_status: draft`, passes end to end. In particular, `test_project_changelog_frontmatter_document_status_final` must pass.

**How**: From the spektacular repo root, with Docker running and Claude credentials available to harbor, run `make harbor-test-implement`. Results are written under `tests/harbor/jobs/`.

**Expected result**: Every test in `tests/harbor/implement-workflow/tests/test_implement_workflow.py` passes (0 failures).

**Who / when**: The maintainer, before merging this branch. The suite doesn't run in CI, and it is the one unchecked acceptance criterion on Phase 2.1.
