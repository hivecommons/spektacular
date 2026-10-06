---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Context: 000063_epic-planning-summary-and-reordering

## Current State Analysis

- **Epic planning (000062)** is driven by the `spek-plan-epic` skill (`templates/skills/workflows/spek-plan-epic/SKILL.md`).
  - Children run orchestrated `spek-plan` workflows and hand back `DONE:` / `QUESTION:` / `FAILED:`.
  - On `DONE:` the orchestrator only keeps the summary in memory.
  - The end-of-planning review (Step 6) shows the summaries in chat. Nothing is written down, and decisions are not gathered or listed first.
- **Epic dependencies are only written by people.** They go in through `epic write` (whole body plus specs list, `cmd/epic.go:269-377`) or `epic split`. Nothing compares plans, so two independent specs changing the same file stay unordered until their branches conflict at `epic merge` (`internal/worktree/worktree.go:509-592`).
- **Per-task files exist only as prose.** They sit in each plan's `context` document under `### Task:` "File changes" bullets with backticked `path:line` (`templates/steps/plan/10-tasks.md:52-69`). No Go code reads the `context` document.
- **The proceed-or-stop rule is copied by hand.** The plan steps carry an identical "Proceed unless genuinely blocked" paragraph in 11 places.
  - Knowledge is binding ("raise it with the user and ask", `02-discovery.md:35`).
  - Referenced designs are "raise with the user" (`03-architecture.md:21-28`).
  - Nothing names the spec's own sections as recorded decisions.
  - The tasks step's `human` criterion "a stakeholder decision" (`10-tasks.md:37-48`) lets a contradiction be parked as a person task.
- **STOPs become questions under orchestration.** In an orchestrated run, `partials/orchestrated-stop.md` (appended by `internal/stepkit/stepkit.go:133-139`) turns every STOP into a `QUESTION:` hand-back, so a new STOP reaches the user mid-run with no orchestrator change.
- **Interview notes are normally gone.** The spec workflow deletes `.spektacular/work/<spec>/` (including `interview.md`) at verification (`templates/steps/spec/08-verification.md:109-113`). They survive only for unfinished specs.
- **Requirements by repo:**
  - **`spektacular`**: every CLI, model, template and skill change, plus the epics design document in the `design` source.
  - **`docs`**: the Epics page (`src/pages/epics.mdx`) and `CHANGELOG.md`.

## Per-Task Technical Notes

### Task: Make contradicting a recorded decision or knowledge entry a stop in every plan step

- **File changes**:
  - `templates/partials/proceed-unless-blocked.md` (new). The text of the current paragraph (as at `templates/steps/plan/02-discovery.md:90`), word for word, so the existing phrase "proceed without interruption" still renders. Add one sentence: a choice that would contradict a decision the user recorded for this spec (its sections, a design it references, or its interview notes where they still exist), or a knowledge entry, is always a decision only the user can make. STOP and ask, and never record it as a drafting assumption, a `human` task, an open question or a note for the review. The partial must avoid the words "orchestrat", "QUESTION:", "DONE:" and "FAILED:" (`cmd/orchestrated_test.go:94-103`).
  - Replace the paragraph with `{{> partials/proceed-unless-blocked}}` in:
    - `templates/steps/plan/02-discovery.md:90`
    - `templates/steps/plan/03-architecture.md:69`
    - `templates/steps/plan/04-components.md:28`
    - `templates/steps/plan/05-data_structures.md:28`
    - `templates/steps/plan/06-implementation_detail.md:40`
    - `templates/steps/plan/07-dependencies.md:34`
    - `templates/steps/plan/08-testing_approach.md:49`
    - `templates/steps/plan/09-milestones.md:29`
    - `templates/steps/plan/10-tasks.md:91`
    - `templates/steps/plan/11-open_questions.md:38`
    - `templates/steps/plan/12-out_of_scope.md:31`
  - Rendering already resolves partials through `stepkit.FSPartials` (`internal/stepkit/stepkit.go:209-232`). Confirm a partial included from a step template sees `{{plan_name}}` and `{{config.command}}`; they share the render context.
  - Tests:
    - `internal/steps/plan/steps_test.go`: extend `TestGatheringStepsProceedWithoutApprovalGates` (:82-122) to require the new rule's anchor phrase (e.g. "contradict a decision the user recorded") in every gathering step, exactly once.
    - `internal/steps/plan/orchestrated_test.go`: add a case asserting an orchestrated gathering step renders both the new rule and the `QUESTION: <name>` hand-back from `partials/orchestrated-stop.md`.
    - `cmd/orchestrated_test.go:94-103`: must stay green for standalone renders.
    - `cmd/instruction_contract_test.go`: add the partial to any partial inventory if one exists.
- **Complexity**: Low
- **Token estimate**: ~15k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Check plans against recorded decisions and knowledge before they are final

- **File changes**:
  - `templates/steps/plan/02-discovery.md`: after the knowledge-outranks-code paragraph (:35), add a **Recorded decisions** paragraph. The user's recorded decisions for this spec are:
    - the spec's Requirements, Constraints, Technical Approach and Non-Goals, and its Acceptance Criteria;
    - every referenced design;
    - `.spektacular/work/{{plan_name}}/interview.md`, read only if it exists. The spec workflow normally removes it once the spec is written, and its absence is normal.

    Change "Ask only questions the code cannot answer" (:72) to also cover the contradiction stop.
  - `templates/steps/plan/03-architecture.md`: in Step 2 (:30-37), before recording the chosen direction, check it against the recorded decisions and the knowledge entries loaded in discovery. Treat any contradiction as a STOP to ask, not a choice to record. Keep the existing "raise with the user" design wording (:21-28) intact; it is pinned by `TestArchitectureStepBuildsOnReferencedDesigns`.
  - `templates/steps/plan/10-tasks.md:37-48`, under "Deciding who carries a task out": add that a contradiction with a recorded decision or knowledge entry is never a `human` task (not even "a stakeholder decision"); stop and ask now. Keep the four criteria verbatim; `TestTasksStepTeachesTheTaskFormat` pins them (`internal/steps/plan/steps_test.go:715-746`).
  - `templates/steps/plan/11-open_questions.md:14-19`: add a "does NOT belong here" example: "This plan departs from the spec's chosen interface" → stop and ask the user now.
  - `templates/steps/plan/14-verification.md`, Step 2 "Quality" (:50-61): add a check that the staged plan contradicts no recorded decision and no knowledge entry. If one does, STOP and ask before Step 3 re-stages.
  - `templates/skills/workflows/spek-plan/SKILL.md`, "Design documents a spec references" (:38-56): add a short paragraph saying planning stops to ask when the plan would contradict a recorded decision or a knowledge entry. Keep it above `# When an orchestrator starts this skill` and free of `QUESTION:` / `DONE:` / `FAILED:` (`templates/orchestrated_skill_section_test.go:81-97`).
  - Tests in `internal/steps/plan/steps_test.go`, one phrase test per step:
    - discovery names `interview.md` and "only if it exists";
    - architecture and verification contain the check;
    - tasks says "never a `human` task";
    - open questions has the new example.
- **Complexity**: Low
- **Token estimate**: ~20k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Find the files each planned task changes

- **File changes**:
  - `internal/depgraph/depgraph.go`: add `Reaches(deps map[string][]string, from, to string) bool`, an iterative DFS with a visited set and no recursion. `from == to` returns false unless a cycle leads back. Names missing from `deps` have no edges. `internal/depgraph/depgraph_test.go`: `TestReaches_*` covering direct, transitive, unrelated, self, unknown, and a cycle that terminates.
  - `internal/plantask/files.go` (new): `type FileRef struct{ Repo, Path string }` and `TaskFiles(plan, context []byte, repos []string) map[string][]FileRef`.
    - Use `Parse(plan)` (`internal/plantask/plantask.go:124`) for task titles and `Task.Repo`.
    - Scan `context` for `### Task: <title>` headings, ending each section at the next `###` or `##`, and collect every backticked token in the section that looks like a path: it contains `/` or a `.ext`, and has no spaces.
    - Strip a trailing `:N`, `:N-M` or `:N,M` suffix.
    - If the token starts `<name>:` and `<name>` is in `repos`, use it as the repo. Otherwise the repo is the task's `Repo`, or the single entry of `repos` when the task has none.
    - Deduplicate, keep the order of first sight, and never error.
    - Tasks whose title has no context section are absent.
  - `internal/plantask/files_test.go` (new): hand-written fixtures following the `taskPlan` const style (`internal/plantask/plantask_test.go:11-70`). Expected values are written by hand. Cover:
    - a prefixed path, an unprefixed path, a `:line` suffix and a range;
    - an unregistered prefix kept as part of the path;
    - a non-path backtick such as `` `go test` ``;
    - a missing context and a missing section.
  - `templates/steps/plan/10-tasks.md:52-56`: the **File changes** bullet requires each entry to begin with the file's path in backticks (`` `path:line` `` or `` `<repo>:path:line` ``), one file per entry.
  - `templates/scaffold/context.md:8-20`: already shaped this way; no change unless the wording conflicts.
  - Add a phrase assertion to `internal/steps/plan/steps_test.go` `TestTasksStepTeachesTheTaskFormat`.
- **Complexity**: Medium
- **Token estimate**: ~20k tokens
- **Agent strategy**: 2 parallel agents (depgraph, and plantask reader plus template), then integrate.

### Task: Keep a planning summary document with each epic

- **File changes**:
  - `internal/epic/summary.go` (new):
    - `type Summary struct{ CreatedDate string; Decisions, Ordering string; Specs []SummarySection }` and `SummarySection{Name, Body string}`.
    - `ParseSummary([]byte) (Summary, error)` splits on `## ` headings: "Decisions to settle" → Decisions, "Order added for shared files" → Ordering, anything else → a spec section.
    - `(Summary) Render(epicName string, order []string) []byte` writes frontmatter `created_date`, `# Planning summary: <epic>`, `## Decisions to settle` ("None." when empty), `## Order added for shared files` ("None added." when empty), then spec sections in `order`, then any others in stored order.
    - `ValidSectionBody(body)` refuses lines starting `# ` or `## `.
  - `internal/epic/summary_test.go`: round-trip, canonical order whatever the write order, an untouched section stays byte-identical, empty defaults.
  - `cmd/epic_summary.go` (new):
    - `epicSummaryCmd` (group, `Use: "summary"`) with `read` and `write`, registered in `cmd/epic.go` `init()` (:483-489). `--schema` follows the epic pattern (:83-140).
    - `summaryPath(cfg, name)` = `<cfg.Epic.Config.Directory>/<name>/summary.md`, built as a store-relative path beside `epicPath` (`cmd/epic_link.go:198-200`).
    - `read`: `epicName`, then `readEpic` (`cmd/epic.go:156-199`) for the existence check. On `store.ErrNotFound` it refuses with `epic_summary_not_found`, next action: "Plan the epic with the spek-plan-epic skill (\"plan this epic\"), which writes its summary, or write a section with `<cmd> epic summary write <epic> --data '{\"section\":\"decisions\"}' --from <path>`". Otherwise it writes the bytes to stdout.
    - `write`: requires `--from` (`epic_from_required`) and `--data` `{section}` (`bad_input`). The section is `decisions` or a member spec (`epic.Member`, `internal/epic/epic.go`). `ordering`, unknown sections, non-members and invalid bodies are refused with `epic_summary_section_invalid`, whose next action lists the valid sections. It parses the existing summary or starts empty, stamps `created_date` once, replaces the section, renders with `e.SpecNames()` and writes through a `docTxn` (`cmd/epic_link.go:32-130`). Output `{epic, section, sections}`.
    - Export an internal `appendOrdering(t *docTxn, cfg, epicName string, order []string, lines []string) error` for `epic order`.
  - `cmd/epic.go` `runEpicDelete` (:420-466): inside the same `docTxn`, `t.delete(summaryPath)` and then the now-empty folder (`FileStore.Delete` removes an empty dir, `internal/store/store.go:191`). Missing summary is a no-op.
  - Tests:
    - `cmd/epic_summary_test.go`, following `cmd/epic_test.go` helpers (`epicProject`, `runEpic`, `refuseEpic`, `snapshotTree`): one test per acceptance criterion, each preceded by `// Criterion:`.
    - Add `summary read` and `summary write` to `TestEpicSchema_EachVerbPublishesInputAndOutput` (`cmd/epic_test.go:690-735`).
    - Add a delete-with-summary test and a rollback test via `failEpicLinkOnCall` (:393).
  - Check that `epic list` (`cmd/epic.go:379-418`) and counter allocation (`internal/identifier/identifier.go:270-296`) ignore the folder, with a test that lists an epic that has a summary.
- **Complexity**: Medium
- **Token estimate**: ~35k tokens
- **Agent strategy**: 2 parallel agents (internal/epic summary model, and cmd verbs plus delete), then integrate.

### Task: Order overlapping specs automatically

- **File changes**:
  - `internal/epic/epic.go`: add `ParallelWith []string` to `EpicSpec` with `yaml:"parallel_with,omitempty" json:"parallel_with,omitempty"`, in `yamlIn` / `yamlOut`, rendered only when non-empty. `internal/epic/validate.go`: refuse a `parallel_with` naming a non-member or the spec itself (`epic_invalid`), and update `invalidNextAction` to mention `epic order`. Tests in `internal/epic/*_test.go`.
  - `cmd/epic_order.go` (new), with `epicOrderCmd` registered in `cmd/epic.go` `init()` and `--data` input `{unorder?: {spec, depends_on}}` plus a schema.
    - **Ordering run:**
      1. `readEpic`, then load each member's `plan` and `context` documents through the store using the plan dir and `artifact.Address{Kind: KindPlan, Feature, Document}` (as `cmd/plan_file.go` does). A member without `plan` is reported as `unplanned`.
      2. Build the file sets with `plantask.TaskFiles`, using registered repo names from the repo registry, as `cmd/status.go:215-248` gets them.
      3. Build `deps` from `Specs`. For i<j in list order, if `shared(i,j)` is non-empty and `!Reaches(deps, j, i) && !Reaches(deps, i, j)` and i is not in `Specs[j].ParallelWith`, append i to `Specs[j].DependsOn` and update `deps`.
      4. If anything was added: `epic.Validate`, `epic.Stamp(&current, next, nil, now)`, `Render` with the body untouched, and in one `docTxn` write the epic (the `joinSpecToEpic` shape, `cmd/epic_link.go:333-356`, with no link changes) and `appendOrdering` with one line per edge: "- Added: `<j>` now depends on `<i>`. Both change `repo:path`, …".
      5. Output `{epic, added:[{spec, depends_on, files:[{repo,path}]}], unplanned}`.
    - **Unorder run:** require the edge to exist in `Specs[spec].DependsOn`, else refuse with `epic_dependency_not_found` (next action: `<cmd> status <epic> --format json` to see the current dependencies). Remove it, add `depends_on` to `ParallelWith`, validate and write, and log "- Removed at review: `<spec>` no longer depends on `<dep>`; they may be implemented side by side." Output `{epic, removed}`.
  - `cmd/epic_order_test.go` (new):
    - fixtures written with `os.WriteFile`: an epic of A, B and C, and plans with hand-written plan.md and context.md;
    - A and B share `` `pkg/x.go` ``, and C shares with A but already depends on B;
    - assert B gains [A] and C gains nothing;
    - plan and spec files are byte-identical before and after (read the bytes and compare, including the `created_date` frontmatter);
    - a second run adds nothing;
    - the summary contains the line naming the file;
    - unorder removes the edge, writes `parallel_with`, a rerun does not re-add, and the summary has the removal line;
    - unorder of a missing edge is refused and `snapshotTree` is unchanged;
    - an unplanned member is reported.
  - Add `order` to `TestEpicSchema_EachVerbPublishesInputAndOutput`.
  - Confirm `internal/status` (run view, `Classify`) and `cmd/epic_split.go` `splitGraph` (:357-400) carry `ParallelWith` through untouched. A split must not drop it, so add a test.
- **Complexity**: High
- **Token estimate**: ~45k tokens
- **Agent strategy**: Parallel analysis (epic model plus validation, and the command), then sequential integration and tests.

### Task: Update the epics design for the summary and automatic ordering

- **File changes**:
  - Read it with `spektacular design read --data '{"source":"design","path":"epics-and-seeded-specs.md"}'` and stage the edited body under `.spektacular/tmp/`. Rewrite with `spektacular design author --data '{"source":"design","path":"epics-and-seeded-specs.md"}' --from <staged>`, which preserves the capture date and back-links. Then remove the scratch file. Never use `design write` (refused for an authored document) and never edit the file directly.
  - Content edits:
    - "Epics → Planning" ("Unchanged."): replace with a short description. Planning an epic with one request keeps a summary document with the epic (decisions first, ordering log, one section per plan). When planning ends, overlapping planned specs are ordered later-after-earlier, and the user can undo an addition at the review.
    - The field table (the `specs` / `epic` / `sources` rows): add a `parallel_with` row on the epic, optional, listing earlier specs the user allowed to run side by side despite shared files, and maintained only by `epic order`.
    - The `epic` verb list (`epic read / write / list / delete / split`): add `summary read/write` and `order`.
    - "Dependencies between specs": add one paragraph saying `epic order` may add an edge for overlapping files, and that this is the only automatic writer of the graph.
- **Complexity**: Low
- **Token estimate**: ~12k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Hand back the project-wide rules each plan follows

- **File changes**:
  - `templates/steps/plan/18-walkthrough.md:33-56`: in the orchestrated branch, add point 5, "The project-wide rules this plan relies on or decides: any convention governing files or practices that other specs may also touch (for example how the changelog is kept, or a shared interface's shape), one line each with what this plan does, or say there are none." The standalone branch (:3-32) is unchanged.
  - `templates/steps/plan/19-finished.md:29-31`: update the `DONE:` contents list to name the project-wide rules.
  - `templates/skills/workflows/spek-plan/SKILL.md:119-126`: add the extra summary point to "There is no sign-off walkthrough: the walkthrough step has you prepare a summary instead." Keep the existing pinned phrases in `templates/orchestrated_skill_section_test.go:24-52` intact, and add a phrase for the new point.
  - Tests:
    - `internal/steps/plan/orchestrated_test.go` `TestOrchestratedWalkthroughHasNoSignOff` (:28-46) and `TestOrchestratedFinishedClosesDocsAndHandsBackDone` (:61-84): require the project-wide rules wording.
    - `TestStandaloneWalkthroughKeepsSignOff` (:50-57): stays green.
- **Complexity**: Low
- **Token estimate**: ~10k tokens
- **Agent strategy**: Single agent, sequential.

### Task: Build and review the epic's planning summary in "plan this epic"

- **File changes**: all in `templates/skills/workflows/spek-plan-epic/SKILL.md`.
  - **"# Spektacular's files are reached through Spektacular" (:21-23):** add `{{command}} epic summary` for the summary document.
  - **"## The hand-back contract" (:59-65):** the `DONE:` contents gain "the project-wide rules it relies on or decides".
  - **"## What counts as a genuine open question" (:67-69):** add "a plan choice that would contradict a decision the user recorded for that spec (in the spec, a design it references, or its interview notes where they still exist), or a knowledge entry". Keep every phrase pinned at `templates/plan_epic_skill_test.go:153-157,200-207`.
  - **"# Step 4: Handling a hand-back", `DONE:` (:73):** also "write its section of the epic's summary: stage the summary under `.spektacular/tmp/` and run `{{command}} epic summary write <epic> --data '{\"section\":\"<spec>\"}' --from <path>`, then remove the scratch file". The section covers the approach, milestones and tasks, tasks for a person (or none), out of scope, drafting assumptions and project-wide rules, using `###` sub-headings only.
  - **New "# Step 6: Order and gather decisions"**, after the loop ends, finished or stopped, and only if a plan was produced in this run (renumber the later steps):
    1. Run `{{command}} epic order <epic>`. Mention each added dependency in one line. They are written without asking.
    2. Compare the project-wide rules in this run's `DONE:` summaries, and in the existing summary sections (`{{command}} epic summary read <epic>`). For each rule on which plans disagree, write an entry under Decisions to settle with `epic summary write --data '{"section":"decisions"}'`. Each entry names the rule, every plan involved and what each does, one proposed answer with a one-line reason, and the status "Open". An empty list keeps "None.".
  - **"# Step 6: The end-of-planning review" (:84-88), becoming Step 7:**
    - Read the summary with `{{command}} epic summary read <epic>` and present it as plain text, section by section and decisions first: Decisions to settle, then Order added for shared files, then each plan.
    - For each open decision, ask the user to accept the proposed answer or give another. Apply the answer to every plan involved with `plan file read/write` through `.spektacular/tmp/`, rewrite each involved spec's section, and rewrite the decisions entry as "Settled: <outcome>".
    - Any other change is applied to the plan document and to that spec's section.
    - Removing an added dependency uses `{{command}} epic order <epic> --data '{"unorder":{"spec":"<later>","depends_on":"<earlier>"}}'`.
    - Close on the existing direct confirmation question.
  - **Child prompt (:48-57):** the store rule names `epic summary`, and children never write the summary.
  - **Final report:** add "Dependencies added for shared files" with the count.
  - **`templates/plan_epic_skill_test.go`:**
    - add phrase pins for each new behaviour above, using the `section` / `flat` / `requirePhrases` helpers (`templates/seeding_test.go:43-75`);
    - update the review pins (:144-151) if wording moves;
    - extend the ask-word allowlist (:164-171) for new sentences containing "ask";
    - keep `templates/data_payload_wellformed_test.go` green for the new `--data` examples.
- **Complexity**: Medium
- **Token estimate**: ~30k tokens
- **Agent strategy**: Single agent, sequential (one file plus its test).

### Task: Document the epic planning summary, automatic ordering and planning questions

- **File changes**:
  - `docs:src/pages/epics.mdx`:
    - frontmatter description (lines 1-5): mention the planning summary;
    - "Plan this epic" (:329-337): replace the bullet at :335-337 with the summary and ordering text below;
    - "What still stops for you" (:354-360): extend;
    - "Dependencies between specs" (:251-310): add a one-line cross-reference;
    - "Working with epics from the command line" (:424-482): add both verbs between the delete paragraph (:464-469) and the status paragraph (:471).

    No new top-level `Section`, so the surface alternation (`surface={false}` at :312, `surface` at :424) is unchanged. No `<div>`, `<section>` or `class=`, and no em dashes.
  - `docs:CHANGELOG.md`: a new top entry `## 000063_epic-planning-summary-and-reordering`, as one or two prose paragraphs naming the sections changed.
  - **Content outline**:

    *Plan this epic* (replacing the last bullet):

    ```mdx
    - Planning keeps a summary document with the epic: the decisions you need to
      make come first, then any order Spektacular added, then one section for
      each plan. Making the same request again adds sections for newly planned
      speks and keeps the others.
    - Speks whose plans change the same files, and that nothing orders yet, are
      ordered for you: the spek listed later waits for the one listed earlier.
      Nothing is re-planned, and the summary names the shared files.
    - The review walks the summary. Changes you ask for are made to the plan and
      to its section together, and you can remove an added order there.
    ```

    *What still stops for you* (extended):

    ```mdx
    Planning also stops to ask when a plan would contradict something you
    already decided: a choice in the spek, in a design it references, or an
    entry in the knowledge base. It never leaves that as a task for a person or
    a note for the review. When plans disagree with each other on a rule for the
    whole project, such as how the changelog is kept, the summary lists it under
    the decisions to settle with one proposed answer, and settling it updates
    every plan involved.
    ```

    *Dependencies between specs* (one line):

    ```mdx
    Planning an epic can add a dependency itself, when two speks change the same
    files and nothing orders them. See [Plan this epic](#planning-and-implementing-an-epic).
    ```

    *Working with epics from the command line* (new paragraphs):

    ````mdx
    Read the summary that planning an epic keeps, or update one section of it:

    ```bash
    spektacular epic summary read 000070_example
    spektacular epic summary write 000070_example --data '{"section":"decisions"}' --from ./decisions.md
    ```

    Order speks whose plans change the same files, or undo one added order:

    ```bash
    spektacular epic order 000070_example
    spektacular epic order 000070_example --data '{"unorder":{"spec":"000072_b","depends_on":"000071_a"}}'
    ```
    ````
  - The site's prose says "spek" for a spec (as at :331-342). Follow it.
- **Complexity**: Low
- **Token estimate**: ~15k tokens
- **Agent strategy**: Single agent, sequential. Verify with `make check` and `make build` in the docs repo, plus the Rule 1 grep from the MDX convention.

## Testing Strategy

Per task:
- **Make contradicting a recorded decision or knowledge entry a stop in every plan step.** Phrase tests in `internal/steps/plan/steps_test.go` (every gathering step renders the rule once) and `internal/steps/plan/orchestrated_test.go` (the orchestrated render carries the rule and the `QUESTION:` hand-back). `cmd/orchestrated_test.go` keeps standalone renders free of orchestration words.
- **Check plans against recorded decisions and knowledge before they are final.** One phrase test per changed step: discovery, architecture, tasks, open questions and verification. The `spek-plan` skill phrase goes in `templates/orchestrated_skill_section_test.go`'s neighbour checks, and `TestOrchestratedSectionIsSelfContainedAndLast` stays green.
- **Find the files each planned task changes.** `internal/depgraph/depgraph_test.go` `TestReaches_*`. `internal/plantask/files_test.go` uses hand-written fixtures. A tasks-step phrase test covers the backticked-path rule.
- **Keep a planning summary document with each epic.** `internal/epic/summary_test.go` covers the model round-trip and canonical order. `cmd/epic_summary_test.go` has one `// Criterion:` test per acceptance criterion, plus refusals (with `snapshotTree` unchanged), delete and rollback. The schema test is updated.
- **Order overlapping specs automatically.** `internal/epic` tests cover `parallel_with` parse, render and validation. `cmd/epic_order_test.go` covers:
  - the added edge;
  - transitive skip;
  - idempotence;
  - byte-identical plans and specs;
  - summary log lines;
  - unorder plus no re-add;
  - refusal;
  - unplanned members;
  - split preserving `parallel_with`.
- **Update the epics design for the summary and automatic ordering.** Checked by reading it back with `design read`: the capture date is unchanged, `specs` back-links are unchanged, and the new content is present. There is no Go test.
- **Hand back the project-wide rules each plan follows.** The orchestrated walkthrough and finished phrase tests. The standalone walkthrough test stays green.
- **Build and review the epic's planning summary in "plan this epic".** Phrase pins in `templates/plan_epic_skill_test.go`, the ask-word allowlist, and `templates/data_payload_wellformed_test.go` for the new `--data` examples.
- **Document the epic planning summary, automatic ordering and planning questions.** In the docs repo: `make check`, `make build`, and `grep -nE "<div|<section|class=" src/pages/*.mdx` returning nothing.

Across the plan:
- `go test ./...` must pass in full.
- The plan-workflow harbor oracles (`tests/harbor/plan-workflow/`) are reviewed for drift against the edited steps.
- The live-agent acceptance criteria and all three success metrics go into the implementation test plan.

## Project References

- **Spec:** `000063_epic-planning-summary-and-reordering` (`spektacular spec file read 000063_epic-planning-summary-and-reordering`).
- **Prior plan:** `000062_epic-plan-and-implement` (orchestration, hand-back and run view).
- **Design kept in step:** `epics-and-seeded-specs.md` from the `design` source.
- **Knowledge, `spektacular` store:**
  - conventions: error remediation, store files written through the CLI, plans never changing the active install, test order independence, tests must pass;
  - architecture: `architecture/working-with-files-from-steps.md` (store access from `cmd`) and `architecture/testing-architecture.md` (template-contract layer, harbor oracles);
  - gotchas: `gotchas/mustache-html-escaping.md` (render any `--data` example in skills with care).
- **Knowledge, `docs` store:** conventions on MDX authoring, site layout, alternating section background, no em dashes and plan content pages.
- **Repo roots:**
  - `spektacular`: `/home/nicj/code/github.com/hivecommons/spektacular`
  - `docs`: `/home/nicj/code/github.com/hivecommons/spektacular-website`

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

- **No settings or schema migration.** The epic format gains only an optional `parallel_with` field, which older epics simply lack. No `config.yaml` change, and no change to `init` or `migrate`.
- **Updated templates and skills reach a project through the usual version check** after the new binary is installed between workflows (`make install-local`). Never install it while a workflow is in progress, and never re-init or migrate this repo from a task.

## Performance Considerations

- **`epic order` cost.** It reads two documents per planned member and compares every pair, which is O(n²) in specs with small sets. Epics hold a handful of specs, so this is negligible.
- **`Reaches` cost.** It is a DFS over the epic graph per candidate pair, also negligible at epic sizes.
