---
created_date: "2026-10-06"
document_status: final
closed_date: "2026-10-06"
---

# Feature: 000064_epic-worktree-store-isolation

## Overview

When an epic is implemented, each spec is built in its own isolated copy of the code. Today, that copy also takes along its own copy of the project's specs, plans and progress records. Updates made while the spec is built land in that copy and reach the project only when the spec's code is merged back. This change keeps every spec, plan, changelog and progress record in the project itself, updated live through Spektacular as each spec is built. The isolated copies hold code and nothing else. Teams running an epic can watch plans progress in the project while the run is going, and the project's records are never handed to version control to reconcile. Each spec in an epic is built by exactly the same implement workflow used for a single spec.

## Requirements

- [x] **Project artifacts are written to the project, never to a spec's isolated copy**
  While an epic is being implemented, every change Spektacular makes to a spec, plan, changelog record, knowledge entry, design document or epic is made in the project's own stores. None of these changes is made in a spec's isolated copy of the code.
- [x] **Isolated copies are used only for code**
  During an epic run, a spec's isolated copies receive code changes, tests and commits only. They receive no changes to Spektacular-managed content.
- [x] **Plan progress is visible in the project while the epic runs**
  As a child agent ticks tasks and records changes in a spec's plan, a user reading that plan from the project sees each update straight away, without waiting for the spec to be merged.
- [x] **Each spec's implement progress is kept in the project**
  A spec's in-progress workflow state and working notes are kept in the project, keyed to that spec. Specs running at the same time keep separate records and never overwrite each other's.
- [x] **The implement workflow tells the agent where to change code**
  When a spec being implemented has isolated copies, the implement workflow tells the agent to make code changes, run tests and verify in those copies, one for each repo the spec touches. When it has none, the workflow points the agent at each repo's registered location, as it does today.
- [x] **Merging refuses a branch that changes the Spektacular directory**
  Merging a finished spec back is refused when the spec's branch changes anything under the Spektacular directory in any repo. The refusal names the offending paths, and nothing is merged in any repo.
- [x] **Status and resume read the project's records**
  Epic status reports each spec's implement state, step and isolated-copy locations from the records held in the project. Repeating "implement this epic" after an interruption resumes each in-progress spec from those records.
- [x] **Documentation describes the new model**
  The public epic documentation says that each spec's isolated copies hold only code, and that specs, plans and progress stay in the project and can be followed there while the run is going.

## Constraints

- Every Spektacular-managed artifact is read and written only through the Spektacular CLI, never with an agent's own file tools. This is a core project rule, set by the user, and it applies to child agents in an epic run as fully as anywhere else.
- The Spektacular directory inside a spec's isolated copy must never be read or written, by agents or by the CLI. (User decision: "in the worktree the .spektacular directory should never be touched.")
- Every Spektacular command in an epic run must be run from the project root, never from inside a spec's isolated copy.
- Epic children must run the standard implement skill unchanged. Any epic-specific behaviour lives in the orchestrator and in the CLI, not in the implement skill. (User decision: "just use the standard implement skill as a sub task in an epic".)
- The child must not learn its code locations from an epic-aware lookup it has to call itself. The implement workflow's own output provides them. (User decision.)
- How isolated copies are created, how code is isolated between specs, and the all-or-nothing merge across repos must stay as they are today. The only exception is the new refusal of Spektacular directory changes at merge.
- Implementing a single spec outside an epic must behave exactly as it does today.
- Store access must not assume a store is a local directory that git can merge, because store providers other than the local directory have to keep working.

## Acceptance Criteria

- [x] **Plan ticks appear in the project mid-run**
  During an epic run, after a child agent finishes a task in a spec's plan, reading that plan through Spektacular from the project shows the task ticked. The spec has not been merged yet.
- [x] **Other artifacts land in the project mid-run**
  A knowledge entry or design document a child records during an epic run can be read through Spektacular from the project before the spec is merged.
- [x] **Changelog records land in the project**
  After a spec finishes in an epic run, its changelog record can be listed and read through Spektacular from the project, before and after the merge, and the record is identical either way.
- [ ] **Isolated copies' Spektacular directory is untouched**
  After an epic run that implements at least two specs, finished or stopped part-way, the Spektacular directory in every spec's isolated copy is byte-for-byte identical to the commit the copy was created from, and has no untracked files.
- [x] **Merged branches carry no Spektacular directory changes**
  After each spec is merged, the merge brings in no changes under the Spektacular directory in any repo.
- [x] **Epic children change code only in their copies**
  When an epic run completes, each spec's code changes appear only on its own branch in its isolated copies, and its artifact changes appear only in the project.
- [x] **Parallel specs keep separate progress**
  With two independent specs implemented at the same time, the epic status shows each one's own current step, and each spec's working notes contain only that spec's work.
- [x] **Implement output names the isolated copies for an epic spec**
  When the implement workflow runs for a spec that has isolated copies, its instructions name those copies' locations as where the code for each touched repo lives, not the registered locations.
- [x] **Implement output is unchanged for a standalone spec**
  When the implement workflow runs for a spec with no isolated copies, its instructions name the registered locations, exactly as before this change.
- [x] **Merge refuses Spektacular directory changes**
  When a spec's branch contains a commit that changes a file under the Spektacular directory, merging that spec is refused. The refusal names the file, and no repo's main line changes.
- [x] **Resume after interruption**
  When an epic run is interrupted while a spec is in progress, repeating "implement this epic" resumes that spec at the step its record in the project shows, in the same isolated copy, without starting it over.
- [x] **Docs updated**
  The public epics page describes isolated copies as holding only code, and says plans and progress can be followed in the project during a run. It no longer says that, inside a spec's isolated copy, repos point to that copy's own checkout.

## Technical Approach

- **Split artifacts from code by working directory.** The child runs `spektacular` from the main project root. Code edits, tests and git commits happen in the spec's worktree paths.
- **The implement workflow supplies the code roots.** When an implement lane runs for a spec that has worktrees, the workflow's step output reports those worktree roots as where each repo's code lives. The main project already knows each spec's worktrees by convention (branch `spek/<spec>` under the project's worktrees directory).
- **Retire the in-worktree repo overlay.** The overlay file `epic worktree` currently writes into the worktree's `.spektacular/` should go. Code-root resolution for a spec comes from the main project instead.
- **Keep lanes, workflow state and working notes in the main project, keyed by spec.** The existing per-spec lane mechanism should already support this. `status` reads them there.
- **Guard at merge.** `epic merge` checks each repo's spec branch for changes under `.spektacular/` and refuses before anything is merged, as part of its existing all-or-nothing dry run.
- **Simplify the orchestrator's child prompt.** It currently tells the child to work from its project worktree and resolve repos there. Instead it should give the project root to run Spektacular from, and nothing else about locations.
- **Area to check:** anything else that resolves paths from the current directory during an implement run, such as the auto-commit targets and the knowledge and changelog stores for each repo. Check each one against the artifact/code split set out in Requirements and Constraints.

## Success Metrics

- No epic run leaves project records needing manual reconciliation after its specs are merged.
- Beyond that, success is measured by the acceptance criteria; no further metrics are defined.

## Non-Goals

- Removing the Spektacular directory from isolated copies, or excluding it from their checkout.
- Changing how the epic planning workflow runs. Planning does not use isolated copies.
- Adding support for new store providers.
- Migrating in-flight epic runs started under the old model. A run in progress when this ships is finished or restarted by hand.
