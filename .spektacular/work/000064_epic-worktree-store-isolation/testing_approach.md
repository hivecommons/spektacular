**Kinds of tests.** Go unit and integration tests carry most of the weight, using real git repositories in temp directories, as the worktree and epic command tests already do. Template-contract tests pin the new two-form step wording and the rewritten skill phrases. No new harbor suite is added. The implement harbor suite runs without worktrees and must keep passing unchanged, which shows that standalone implement is untouched.

**Most coverage, and why.**
- **Worktree package.** The record's lifecycle (written by `Ensure`, read with no git, removed by `Merge`), the absence of any file under a worktree's `.spektacular/` after `Ensure`, and the merge guard: a branch commit under `.spektacular/` in either the project repo or a sibling repo is refused, the refusal names the path, and no repo's main line moves.
- **Implement commands and auto-commit.** End-to-end cmd tests with a real worktree fixture, running `implement new`/`goto` from the main root:
  - the rendered instruction names each worktree root;
  - zero git calls and no persisted roster still hold;
  - a plan tick written through the CLI lands in the main project's plan store, and is readable before any merge;
  - the main project's copy is unchanged in the worktree;
  - auto-commit puts code commits on `spek/<spec>` in the worktree, and artifact commits in the main checkout limited to that spec's paths. Another spec's dirty artifacts in the main checkout stay uncommitted.
- **Status.** An in-progress implement lane in the main project with a worktree reports `in_progress`, its step, and the worktree root. Ticked tasks plus a final changelog in the main stores report `awaiting_merge`. Nothing is read from the worktree's `.spektacular/`.
- **Repo set.** `New` ignores any stray overlay-style file, and the spec-scoped constructor relocates only mapped repos.

**Load-bearing assertions.**
- Artifacts written during an epic implement land in the main project.
- A spec's worktree `.spektacular/` stays byte-identical to its base commit.
- Merge refuses any `.spektacular/` change.
- Standalone implement output is unchanged.

**Conventions followed.** Tests that execute `rootCmd` go through `resetRootCmd`/`runRootCmd`. Real-git fixtures pin identity. Template assertions are hand-copied substring oracles with a message per assertion. Nothing touches this repository's own `.spektacular/`.

**Spec success metrics.**
- *No epic run leaves project records needing manual reconciliation after merges.* **Behavioural test**: the cmd-level epic flow test merges a spec whose artifacts were written in the main project, and asserts that the main project's plan, changelog and spec are unchanged by the merge and the main checkout has no conflict or leftover. **Manual — captured in the implementation test plan**: one real two-spec epic run, to confirm the same in practice.
- *Acceptance criteria as the measure.* Each spec acceptance criterion maps to a behavioural test above. The exceptions are the docs criterion and the live mid-run observation of plan ticks by a person, both **Manual — captured in the implementation test plan**.

**Manual reviews.**
- **Manual — captured in the implementation test plan**: review the rendered epics docs page (site build, type check and visual check of the "Implement this epic" section).
- **Manual — captured in the implementation test plan**: run the harbor implement suite once, because the read-plan and feature-changelog step templates change (testing-architecture knowledge entry).
- **Manual — captured in the implementation test plan**: one real epic run with two independent specs, after installing the built binary locally, between workflows. Watch plan ticks appear in the main project mid-run, confirm each spec worktree's `.spektacular/` is untouched, and confirm both merge cleanly.

**Deliberate gaps.** No harbor epic suite. Orchestrator behaviour is prose, so its regression guard is the skill-phrase contract tests plus the manual epic run.
