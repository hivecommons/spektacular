---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Research: 000063_epic-planning-summary-and-reordering

## Alternatives considered and rejected

- **Overlap input: a new machine-readable `**Files:**` line on every plan.md task.** Rejected:
  - Plans written before this change have no such line. That includes the user's five xcl plans that motivated the spec. Overlap detection would silently find nothing for exactly the case it exists for.
  - It also breaks the standing rule that plan.md carries no file references (`templates/scaffold/plan.md:121-122`, `templates/steps/plan/10-tasks.md:33`).
  - It changes the task format fixed by the plan-task-graph design, which Hive decodes (`internal/plantask/export.go:9-11`).
- **Overlap judged by the orchestrating agent from the children's `DONE:` summaries.** Rejected: the spec's Technical Approach prefers a deterministic CLI comparison covered by tests. Agent judgement is what missed the `configuration-text.mdx` / `encode.go` collision in the first place.
- **Overlap computed at repo level** (`worktree.TouchedRepos`, `internal/worktree/worktree.go:118-150`). Rejected: far too coarse. Almost every spec in this project touches the `spektacular` repo, so every pair would be ordered and all parallelism lost.
- **Overlap computed from real git diffs at merge time** (`Manager.Merge` / `git merge-tree`, `internal/worktree/worktree.go:509-592`). Rejected: that only runs while implementing, after the parallel work is already done. The spec wants the order fixed while planning.
- **Reporting overlaps as an `epic.run.problems` entry in `status`.** Rejected:
  - Every existing problem blocks implement (`internal/status/run.go:96`, `BlocksImplement`). An overlap is resolved automatically, not a blocker.
  - The spec wants the dependency written, not merely reported.
- **Letting the agent add the edges with `epic write`.** Rejected:
  - `epic write` needs the whole body resupplied with `--from` and replaces the whole `specs` list (`cmd/epic.go:269-377`, `:328-330`).
  - That is error-prone, not deterministic, and leaves no record of which edges were automatic.
- **Summary document stored as a plan document** (for example `plan file write <epic> summary`). Rejected by a spec constraint: "The summary document is kept with its epic, not stored as a plan". Plans stay one per spec.
- **Summary as a sidecar file `epics/<name>.summary.md`.** Rejected in favour of the folder `epics/<name>/summary.md`:
  - `<name>.summary` contains a `.` and fails `validSegment`, so the dotted form is merely tolerated rather than designed for.
  - The folder form mirrors how plans group documents (`artifact.Address.StorePath`, `internal/artifact/address.go:156-168`).
  - Both forms are skipped by `epic list` (`NameFromEntry(..., EntryFile)` rejects directories and dotted names, `address.go:183-205`).
- **Summary written whole by the agent every time.** Rejected: repeating the request must keep earlier sections unchanged, and review edits touch one plan at a time. Section-addressed writes, merged by the CLI, make both deterministic and testable.
- **Reading the spec's interview notes as a source of recorded decisions.** Mostly unavailable, so not relied on:
  - `templates/steps/spec/08-verification.md:109-113` removes `.spektacular/work/<spec>/` once the spec is written, so a finished spec's `interview.md` is gone by planning time.
  - It survives only for an unfinished or interrupted spec. Because the plan's work dir shares that path, the planner can check for it cheaply. It is never required.

## Chosen approach — evidence

- **Per-task files already exist, in context.md.** `templates/steps/plan/10-tasks.md:52-69` requires a `### Task: <title>` section with "**File changes**" that reference specific files. Paths are backticked `path:line`, and paths in other registered repos are prefixed `<repo>:path:line`. The scaffold `templates/scaffold/context.md:8-20` uses bullets shaped `` - `<file:line>` — <description> ``. Parsing backticked path tokens per `### Task:` section therefore works on existing plans, and tightening the template makes new plans reliable.
- **Task-to-repo attribution.** `internal/plantask/plantask.go:80-98` `Task.Repo` comes from plan.md. Context sections match plan tasks by title (plan.md's link `context.md#task-<slug>`, `10-tasks.md:34`). An unprefixed path belongs to the task's `Repo`.
- **The graph tools exist, but there is no reachability check.** `internal/depgraph/depgraph.go`: `FindCycle` :11, `CycleMembers` :64, `TopoOrder` :137, and a direct-edge-only `dependsOn` :186. "No ordering, direct or transitive" needs a new `Reaches(deps, from, to)`.
- **A body-preserving epic graph mutation already exists.** `joinSpecToEpic` (`cmd/epic_link.go:333-356`) is the model. It reads the epic, clones `Specs`, calls `epic.Validate`, `epic.Stamp`, `Render` and `applyEpicLinks` inside a `docTxn`, and leaves `current.Body` untouched. Adding edges does not change membership, so the links diff is empty and only the epic is written.
- **Writing the epic never touches a spec**, so plans cannot go stale. Stale detection is the spec changing after its plan (`plan.strict_spec_changes`, `internal/status/report.go` `Classify`). `applyEpicLinks` writes spec back-links only for added or removed members (`cmd/epic_link.go:253-288`).
- **Epic frontmatter types.** `internal/epic/epic.go`: `EpicSpec{Name, DependsOn}`, with `yamlIn` / `yamlOut` and strict decoding. `internal/epic/validate.go`: `Validate` with code `epic_invalid`. A new optional field must be added to both YAML shapes and validated.
- **Epic verb conventions.** `cmd/epic.go`:
  - `epicStore` :142, `epicName` :156, `epicNotFound` :177, `readEpic` :184.
  - Errors are built as `output.NewError(code,msg).WithResource().WithNextAction()`.
  - Verbs are registered in `init()` :483-489, with a `--schema` persistent flag.
  - `TestEpicSchema_EachVerbPublishesInputAndOutput` (`cmd/epic_test.go:690-735`) must list any new verb.
- **Store layout.** `FileStore.Write` creates parent dirs (`internal/store/store.go:180`). `Delete` removes one file, or one empty dir, and is idempotent (:191). `runEpicDelete` (`cmd/epic.go:420-466`) deletes only `<name>.md` in a `docTxn`, so it must also remove the summary and its folder.
- **Folder versus counter allocation.** `nextCounterFromStore` (`internal/identifier/identifier.go:270-296`) scans entry names for a `<digits>_` prefix. A summary folder named after its epic carries the same counter, so allocation is unaffected.
- **Orchestrator and child contract live in templates.**
  - `templates/skills/workflows/spek-plan-epic/SKILL.md`: the hand-back contract, "What counts as a genuine open question", Step 4 handling and the Step 6 review.
  - `templates/steps/plan/18-walkthrough.md:33-56`: the orchestrated summary has four points.
  - `templates/steps/plan/19-finished.md:29-31`: the `DONE:` line.
  - `templates/partials/orchestrated-stop.md`: turns every STOP into `QUESTION:`. Any new STOP in a plan step automatically becomes a mid-run question under orchestration, while the other children carry on.
- **Where the plan steps decide things today.**
  - "Proceed unless genuinely blocked" is copied into 11 steps (02:90 … 12:31).
  - Knowledge outranks code, with "raise it with the user and ask" (02-discovery.md:35).
  - Designs are "raise with the user" (03-architecture.md:21-28).
  - The tasks step's `human` criterion includes "a stakeholder decision" (10-tasks.md:37-48), which is how a contradiction can currently be parked as a person task.
  - Verification (14-verification.md) is the last check before the plan is written, the natural home for "check the finished plan against recorded decisions and knowledge".
- **Template-contract test seams.**
  - `internal/steps/plan/steps_test.go`: `TestGatheringStepsProceedWithoutApprovalGates` :82-122, `TestTasksStepTeachesTheTaskFormat` :715-746, `TestArchitectureStepBuildsOnReferencedDesigns` :796-807.
  - `internal/steps/plan/orchestrated_test.go`: `TestOrchestratedWalkthroughHasNoSignOff` :28-46 and `TestOrchestratedFinishedClosesDocsAndHandsBackDone` :61-84.
  - `templates/plan_epic_skill_test.go`: the question definition :153-157 and :200-207, the review :144-151, and the ask-word allowlist :159-184.
  - `cmd/orchestrated_test.go:94-103`: standalone steps must not contain "orchestrat", "QUESTION:", "DONE:" or "FAILED:".
- **Docs site.** `spektacular-website/src/pages/epics.mdx`:
  - "Planning and implementing an epic" runs :312-422 (`surface={false}`), with subsections "Plan this epic" :329, "What still stops for you" :354 and "Picking up where you left off" :368.
  - "Working with epics from the command line" runs :424-482 (`surface`) and lists `list`, `read`, `write`, `split` and `delete` only.
  - Site changelog: `spektacular-website/CHANGELOG.md`, newest first, `## <spec>` followed by prose.

## Files examined

- `spektacular:templates/skills/workflows/spek-plan-epic/SKILL.md` — the orchestrator loop, hand-back contract, genuine-question definition and end-of-planning review (chat only, no document).
- `spektacular:templates/skills/workflows/spek-plan/SKILL.md:119-126` — the "When an orchestrator starts this skill" section, which must stay last.
- `spektacular:templates/steps/plan/02-discovery.md:16-35,72,81-90` — design resolution, knowledge-outranks-code, the clarify step, judgement-call logging and proceed-unless-blocked.
- `spektacular:templates/steps/plan/03-architecture.md:21-32,56,69` — designs are built on, and a disagreement is raised with the user.
- `spektacular:templates/steps/plan/10-tasks.md:3,12-69,91` — the task block, the human criteria and the context.md per-task "File changes" format.
- `spektacular:templates/steps/plan/14-verification.md:56` — the verification checklist, the place to add a recorded-decisions check.
- `spektacular:templates/steps/plan/18-walkthrough.md:3-56` — the standalone sign-off and the orchestrated four-point summary.
- `spektacular:templates/steps/plan/19-finished.md:26-31` — the orchestrated `DONE:` line.
- `spektacular:templates/partials/orchestrated-stop.md:1-9` — STOP becomes `QUESTION:` under orchestration.
- `spektacular:templates/scaffold/context.md:8-20` — the per-task technical notes shape.
- `spektacular:templates/scaffold/plan.md:121-141` — no file refs in plan.md, and the task block.
- `spektacular:templates/steps/spec/08-verification.md:109-113` — the spec work dir, including interview.md, is removed once the spec is written.
- `spektacular:internal/stepkit/stepkit.go:83-172,209-232` — rendering, the `orchestrated` var and partial appending.
- `spektacular:internal/plantask/plantask.go:80-115,124-152,224-276` — the task parser. It has no files field.
- `spektacular:internal/depgraph/depgraph.go:11,64,137,186` — graph helpers. There is no reachability helper.
- `spektacular:internal/epic/epic.go` / `validate.go` — the epic frontmatter type, Parse/Render/Stamp and Validate (`epic_invalid`).
- `spektacular:cmd/epic.go:21-28,66-140,142-247,269-489` — the epic verbs, `--data` types, schemas, write, list and delete.
- `spektacular:cmd/epic_link.go:32-166,198-200,253-288,333-356` — the `docTxn`, `epicPath`, `applyEpicLinks` and the `joinSpecToEpic` model.
- `spektacular:cmd/epic_split.go:357-400,566-616` — split graph extension and body rendering.
- `spektacular:cmd/storefile.go:196+`, `cmd/plan_file.go:12-30` — the generic store-file verbs, using `metadata.Merge` to stamp created_date.
- `spektacular:internal/artifact/address.go:28,49-55,156-205` — `KindEpic`, `StorePath` and `NameFromEntry`.
- `spektacular:internal/store/store.go:84-203` — the Store interface and FileStore Write, Delete and List semantics.
- `spektacular:internal/identifier/identifier.go:249-296` — counter scanning, unaffected by a same-named folder.
- `spektacular:internal/status/run.go:17-226`, `report.go:23-439`, `pretty.go:19-82`, `cmd/status.go:48-248` — the run view. Plans are parsed in `buildPlan`, and context.md is never read.
- `spektacular:internal/worktree/worktree.go:118-150,509-628` — repo-level touch detection and merge-time conflict detection.
- `spektacular:cmd/epic_test.go:17-30,55-205,393,690-735` — epic test helpers (`epicProject`, `runEpic`, `refuseEpic`, `snapshotTree`) and the schema test.
- `spektacular:internal/status/status_test.go:26-151`, `run_test.go:20-458` — `newEnv`, `e.plan` and `e.epic` fixtures.
- `spektacular:internal/steps/plan/steps_test.go`, `orchestrated_test.go`, `cmd/orchestrated_test.go`, `cmd/instruction_contract_test.go`, `templates/plan_epic_skill_test.go`, `templates/orchestrated_skill_section_test.go`, `templates/seeding_test.go:43-75` — the template-contract tests and their phrase helpers.
- `spektacular:tests/harbor/plan-workflow/` — step order and command oracles. The step order does not change here.
- `docs:src/pages/epics.mdx:1-495` — the page structure, surface alternation and the sections to extend.
- `docs:CHANGELOG.md:1-3` — the entry format.
- `docs:src/pages/how-it-works.mdx:399-426,478-481` — the plan stage and walkthrough text, and knowledge being binding.

## External references

- None. All behaviour is defined in this repo's templates, Go code and the epics design.

## Prior plans / specs consulted

- `000062_epic-plan-and-implement` (plan and context, historical). It introduced orchestrated lanes, the `status` run view, `spek-plan-epic`, the hand-back contract and the chat-only end-of-planning review. Its context.md shows the per-task "File changes" bullets this plan's overlap parser reads.
- Design `epics-and-seeded-specs.md` (source `design`, draft, not referenced by this spec). It describes the epic format (`specs: [{name, depends_on}]`) and its validation. Its "Planning. Unchanged." line and its frontmatter field table must be updated for the summary document and the new optional field.
- Spec `000063_epic-planning-summary-and-reordering` working-context origin notes. These come from the user's xcl `references-and-secrets` epic: two independent specs both changed `configuration-text.mdx` / `encode.go`; changelog handling disagreed; a user-chosen interface and a knowledge entry were contradicted; the chat review was garbled.

## Open assumptions

- Context.md per-task sections in existing plans name their files as backticked paths, so the parser finds most shared files in old plans. If a real epic's context.md uses another shape, the parser misses those files and the template change only helps new plans.
- Two specs "change the same file" when the same (repo, path) appears in both plans' context.md task sections. Line numbers are ignored, and so is whether a path is read or written.
- The orchestrator runs ordering once all children have handed back (including in stopping mode), not after each `DONE:`. Edges only matter for implementation, which has not started.
- When a project registers one repo, unprefixed paths belong to that repo. A `<name>:` prefix counts as a repo only when `<name>` is a registered repo.

## Drafting assumptions

### Overlap is read from context.md's per-task file changes (discovery)
- **Decision**: The CLI finds shared files by parsing the backticked paths in each plan's context.md `### Task:` sections, keyed by (repo, path). The tasks step template is tightened so every file change starts with a backticked path.
- **Rationale**: It works on plans that already exist, including the motivating xcl plans. It keeps plan.md free of file references, and it is deterministic and testable.
- **Rejected**: A new `**Files:**` line on plan.md tasks, because old plans would find nothing and it would break the plan.md and task-graph format. Agent judgement, because it is not deterministic and is what missed the original collision. Repo-level overlap, because it is too coarse.

### Interview notes are optional input, the spec is the record (discovery)
- **Decision**: Planners check the spec's sections, referenced designs and knowledge entries. They read `.spektacular/work/<spec>/interview.md` only if it still exists.
- **Rationale**: The spec workflow deletes its work dir once the spec is written, so the notes are normally gone, and every decision in them was confirmed into the spec's sections.
- **Rejected**: Changing the spec workflow to keep the notes, which is out of scope and would grow the spec's footprint. Ignoring them entirely, which would waste the notes when they do exist.

### Summary document lives at epics/<name>/summary.md (discovery)
- **Decision**: The summary is stored in a folder named after the epic, inside the epic store.
- **Rationale**: It mirrors how plans group documents. `epic list` and counter allocation both ignore it. Deleting the epic removes it in the same transaction.
- **Rejected**: A dotted sidecar `<name>.summary.md`, which is tolerated only by accident of name validation. Storing it as a plan document, which a spec constraint forbids.

### Chosen direction: CLI ordering + section-addressed summary + shared STOP rule (architecture)
- **Decision**: (1) a new `epic order` verb deterministically adds later-depends-on-earlier edges for planned specs whose context.md task files overlap and that have no ordering. It records each addition in the summary, and `--data unorder` removes an edge and records the pair in a new optional `parallel_with` on the epic entry. (2) A new `epic summary read/write` keeps `<epics>/<epic>/summary.md`, written one section at a time and rendered in a fixed order with Decisions first. (3) A shared "proceed unless genuinely blocked" partial makes contradicting a recorded decision or a knowledge entry a STOP in every plan step, which orchestration already turns into a QUESTION. The walkthrough's orchestrated summary gains a "project-wide rules" point, and spek-plan-epic writes and walks the summary.
- **Rationale**: Deterministic and testable where the spec prefers it. It works on existing plans and is idempotent across repeated runs. It reuses 000062's hand-back and STOP-to-QUESTION mechanism, so single-spec planning gains only the new questions.
- **Rejected**: The agent rewriting the epic with `epic write` (whole-body rewrite, no record of automatic edges). The agent writing the whole summary each time (earlier sections could drift on repeat). Overlap judged by the agent (non-deterministic).

### Undoing an added dependency is remembered in the epic (architecture)
- **Decision**: Removing an edge at review records the pair on the later spec's entry as `parallel_with: [<earlier>]`, so `epic order` never re-adds it.
- **Rationale**: Otherwise a repeated "plan this epic" would silently re-add a dependency the user removed. The record belongs with the graph it qualifies.
- **Rejected**: Remembering it in the summary document (the CLI would have to parse prose back). Not remembering it (the undo would not stick).

### epic order runs once at the end of the loop (architecture)
- **Decision**: The orchestrator runs `epic order` after the loop ends (finished or stopped), before the review, and not after each `DONE:`.
- **Rationale**: Dependencies only constrain implementation, which has not started. Both plans of a pair must exist to compare them. Running once is simpler and the verb is idempotent anyway.
- **Rejected**: Running after every `DONE:`, which gives the same final result with more noise.

### "Proceed unless genuinely blocked" becomes a partial (architecture)
- **Decision**: Replace the eleven hand-copied paragraphs with `{{> partials/proceed-unless-blocked}}`, which carries the recorded-decision and knowledge rule.
- **Rationale**: One place to state the widened rule. Existing tests already assert the rendered phrase in every gathering step, so they keep guarding it.
- **Rejected**: Editing eleven copies, which risks drift.

### Conventions selected (architecture)
- **Decision**: Keep error-remediation, store-through-CLI, never-change-active-install, test-order, tests-must-pass, and the five docs conventions. The glossary is used for wording only.
- **Rationale**: Each bears on a new verb, template change or docs change in this plan.
- **Rejected**: None dropped; every loaded convention applies.

### Nine tasks across four milestones; docs waits for the skill (tasks)
- **Decision**: M1 has two template tasks: a shared stop partial, then step-specific checks. M2 has the overlap reader plus reachability, `epic summary`, `epic order` (with `parallel_with`) and the design update. M3 has the walkthrough's project-wide-rules point and the `spek-plan-epic` changes. M4 is one docs task that depends on `epic order` and the skill, so the documented names are final. `epic summary` depends on nothing and can start at once.
- **Rationale**: Tasks that edit the same template file are serialised: 10-tasks.md is touched by both M1 tasks and the reader task, and spek-plan SKILL.md by the check and walkthrough tasks. That avoids conflicting edits, and the remaining tasks can run in parallel.
- **Rejected**: One large instructions task, which would be hard to review. Docs depending on nothing, which risks documenting names that later change.

### The ordering section is CLI-only and append-only (tasks)
- **Decision**: Only `epic order` writes "Order added for shared files", as an append-only log of "Added" and "Removed at review" lines. `epic summary write` refuses that section.
- **Rationale**: The log is the deterministic record the acceptance criteria check ("names the shared file", "shows it was removed"). Agent rewrites could lose it.
- **Rejected**: Letting the orchestrator write it from `epic order`'s JSON, which is non-deterministic and duplicates the source of truth.

## Rehydration cues

- `spektacular spec file read 000063_epic-planning-summary-and-reordering`
- `spektacular design read --data '{"source":"design","path":"epics-and-seeded-specs.md"}'`
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'` (template-contract layer, harbor oracles)
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/working-with-files-from-steps.md"}'` (store access from cmd)
- Re-read `templates/skills/workflows/spek-plan-epic/SKILL.md`, `templates/steps/plan/{10-tasks,14-verification,18-walkthrough,19-finished}.md`, `templates/partials/orchestrated-stop.md`, `cmd/epic.go`, `cmd/epic_link.go`, `internal/epic/*.go` and `internal/depgraph/depgraph.go`.
- `spektacular plan file read 000062_epic-plan-and-implement context` for the per-task "File changes" shape.
