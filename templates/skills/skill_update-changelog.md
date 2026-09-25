# Update Changelog

Guide for writing task entries into a plan's inline `## Changelog` section and for finalizing the section with a summary at the end of the implement workflow.

## Instructions

The implement workflow's `update_changelog` step calls this skill once per task. The first call for a given plan creates the `## Changelog` section; subsequent calls append new entries under it. At the very end, before the workflow transitions to `test_plan`, the main agent should also prepend a `### FINAL SUMMARY` block above all the per-task entries.

### Lifecycle

**First invocation (section does not yet exist in `plan.md`):**

1. Append a new `## Changelog\n\n` heading **after** the existing `## Out of Scope` section in `plan.md`. If `## Out of Scope` is missing, append to the very end of the file.
2. Below the `## Changelog` heading, write the first task entry using the per-entry format below.

**Subsequent invocations (section exists):**

1. Locate the `## Changelog` section in `plan.md`.
2. Append a new task entry **below any existing entries** under the section.

**Final invocation (no more unchecked tasks remain):**

1. After appending the last per-task entry, prepend a `### FINAL SUMMARY` block **immediately below** the `## Changelog` heading, above all per-task entries.
2. The `update_feature_changelog` step then writes the project changelog record and one derived record per affected repo, each at the location its configuration declares.

### Per-entry format

Each per-task entry has this shape (in a plan written before tasks, the heading names the phase: `### <YYYY-MM-DD> — Phase N.M: <phase title>`):

```
### <YYYY-MM-DD> — Task: <task title>

**What was done**: <1-3 sentences summarizing the code change in plain language. Focus on behavior, not mechanics.>

**Deviations**: <Anything that didn't match the plan — renamed files, different structure, scope adjustments, etc. Write "None" explicitly if there were none.>

**Files changed**:
- `path/to/file.go`
- `path/to/another/file.go`

**Discoveries**: <Anything the next task or a future maintainer should know — a tricky API, a hidden constraint, a renamed symbol, a missed edge case, a workaround. Write "None" if there was nothing notable.>
```

### FINAL SUMMARY format

Prepend this block immediately below `## Changelog` on the last invocation:

```
### FINAL SUMMARY

<2-4 sentence overall summary of what this plan delivered. Written for a reader who has read plan.md but wants a post-implementation recap of what actually shipped versus what was originally planned.>

**Total tasks**: N/M completed

**Notable deviations from the plan**: <Any deviations worth calling out at the summary level. If none, "None".>
```

### Rules

- **Date stamps are literal.** Use the actual date the task was completed (ISO format `YYYY-MM-DD`), not a placeholder.
- **Task titles must match `plan.md`.** Copy the task's heading text verbatim so the changelog entry is easy to cross-reference.
- **`Files changed` is a flat list of paths.** No line numbers, no diffs, no descriptions per file.
- **`Discoveries` is where drift notes go.** If the implement workflow's `read_plan` gate found drift and the user chose to "proceed with mismatches in memory", record the mismatches here so they're visible in the plan history.
