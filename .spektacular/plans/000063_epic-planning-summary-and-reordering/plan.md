---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Plan: 000063_epic-planning-summary-and-reordering

<!-- Metadata -->
<!-- Created: 2026-10-04T13:47:52Z -->
<!-- Commit: b514a2e -->
<!-- Branch: design/epics-and-seeded-specs -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

When an epic is planned with one request, the user now gets one summary document for the whole epic instead of a dense list in chat. It is kept with the epic, its decisions come first, and it has a section for every plan. Planning orders specs whose plans change the same files on its own. It stops to ask whenever a plan would contradict a decision the user already recorded or a knowledge entry, and it gathers cross-plan disagreements with one proposed answer each. Users with multi-spec epics get plans that can be implemented in parallel without merge conflicts, and a single document they can review and come back to.

## Conventions

- **Error messages must describe the problem and suggest remediation** — `epic order` and `epic summary` add new refusals: `epic_summary_not_found`, `epic_dependency_not_found` and `epic_summary_section_invalid`, plus an unknown or non-member section. Each must be built with `output.NewError(...).WithNextAction(...)` and give a runnable next step.
- **Spektacular's own files are written through Spektacular** — the summary document lives in the epic store and is reached only through `epic summary read/write`. The orchestrator and every child prompt must restate this. `epic delete` removes the summary through the store, never with `rm`.
- **A plan never changes the active skills and configuration** — this plan changes plan step templates, a partial and the `spek-plan-epic` skill. Verify with `go test` and throwaway projects, and never re-init, migrate or run `go run .` against this repo.
- **Tests must not depend on execution order** — new `cmd` tests for `epic order` and `epic summary` go through `resetRootCmd` / `runRootCmd` and each use their own `t.TempDir()` project.
- **Passing tests are required before calling work done** — `go test ./...` must pass in full, including the template-contract tests whose pinned phrases this plan moves into a partial.
- **Docs: MDX authoring, site layout, alternating section background, no em dashes, plans sketch content structure** — the epics page changes in `docs` add subsections inside existing `Section`s, with no layout HTML and no em dashes. They keep the surface alternation intact, and each docs task carries a content outline.

## Architecture & Design Decisions

The work splits into three parts. Two are deterministic CLI parts in the `spektacular` repo. The third is a set of instruction changes, also in `spektacular`, plus documentation in `docs`. All of it builds on 000062's epic planning: the `spek-plan-epic` orchestrator, the `DONE:` / `QUESTION:` / `FAILED:` hand-back, and the orchestrated plan steps.

**1. Ordering overlapping specs: `epic order` (CLI, deterministic).**
- **The new verb.** `epic order <epic>` reads every member spec that has a plan. For each task it collects the files the plan names, as (repo, path) pairs, from the task's section in the plan's `context` document (its "File changes" entries).
  - Paths are the backticked tokens, with any `:line` suffix stripped.
  - A `<repo>:` prefix counts only when it names a registered repo. Otherwise the path belongs to the task's own `**Repo:**` from `plan`, matched by task title.
- **Which pairs are ordered.** It walks every pair of members in the epic's list order. It adds "later depends on earlier" only when all three hold:
  - the two plans share at least one file;
  - neither spec already reaches the other through the dependency graph (a new `depgraph.Reaches`, re-evaluated after each edge it adds, so a chain is never doubled up);
  - the user has not already chosen to let that pair run side by side.
- **How the edges are written.** The edges go through the same body-preserving transaction `joinSpecToEpic` uses (`cmd/epic_link.go:333-356`). Only the epic document changes. No spec is written, so no plan goes stale and nothing is re-planned.
- **Reporting.** The verb appends one line per added dependency, with the shared files, to the summary's "Order added for shared files" section, and returns the same list as JSON.
- **Undoing an edge.** `epic order <epic> --data '{"unorder":{"spec":"B","depends_on":"A"}}'` removes the edge and records the pair on B's epic entry in a new optional `parallel_with` list, so a later `epic order` never re-adds it. It also logs the removal in the summary.
- **Why the CLI and why context.md.** The CLI does this rather than the agent because the spec prefers a tested, deterministic comparison, and agent judgement is what missed the original collision. It reads context.md rather than a new plan.md field because:
  - context.md already names every task's files (`templates/steps/plan/10-tasks.md:52-69`), so the check works on plans that already exist;
  - plan.md stays free of file references.

  The tasks step is tightened so every file change starts with a backticked path, which keeps new plans reliably parseable.
- **Design update.** `parallel_with` changes the epic format, so the epics design (`epics-and-seeded-specs.md`) is updated to match, as the spec's constraint requires.

**2. The epic's planning summary: `epic summary` (CLI).**
- **Where it is kept.** The summary is one Markdown document per epic, kept with the epic in the epic store at `<epics>/<epic>/summary.md`. That folder is invisible to `epic list` and to ID allocation, and `epic delete` removes it in the same transaction. It is never a plan.
- **Reading and writing.** `epic summary read <epic>` returns it. `epic summary write <epic> --data '{"section":"decisions"|"<spec>"}' --from <path>` replaces exactly one section and leaves every other section untouched. This is what keeps earlier specs' sections unchanged when planning is repeated, and what makes a review edit to one plan touch only its own section.
- **Fixed layout.** The CLI renders the sections in a fixed order:
  1. the title;
  2. **Decisions to settle**, always present, and saying "None." when empty;
  3. **Order added for shared files**, written only by `epic order`;
  4. one section per planned spec, in epic list order.

  So decisions always come first whatever order sections were written in.
- **Refusals.** Reading an epic with no summary is refused with `epic_summary_not_found`, whose next step says how to get one: plan the epic with "plan this epic". A section body that contains its own level-1 or level-2 heading is refused, so the section structure cannot be corrupted.

**3. Instructions: questions while planning, and the summary-driven review.**
- **Recorded decisions and knowledge become a STOP.** The plan steps gain one shared rule: a plan choice that contradicts a decision the user recorded for this spec, or a knowledge entry, is always a genuine stop for the user. Recorded decisions are those in the spec's sections, in a design it references, or in `interview.md` where the spec's notes still exist. Such a contradiction is never a `human` task, an open question or a note left for the review.
  - The rule lives in a partial, which replaces the eleven hand-copied "Proceed unless genuinely blocked" paragraphs.
  - Architecture and verification each carry an explicit check against recorded decisions and knowledge.
  - The tasks and open-questions steps say a contradiction may not be parked there.
- **Same STOP for one spec and for an epic.** Because `partials/orchestrated-stop.md` already turns every STOP into a `QUESTION:` hand-back under orchestration, single-spec planning gains exactly these questions and nothing else. During epic planning the other children carry on meanwhile, unchanged from 000062.
- **The children's summaries.** The orchestrated walkthrough summary gains a fifth point: the project-wide rules the plan relies on or decides (such as how the changelog is kept). The orchestrator needs these to spot cross-plan disagreements.
- **What `spek-plan-epic` changes.**
  - Its "genuine open question" definition widens to match the new rule.
  - On each `DONE:` it writes that spec's section with `epic summary write`.
  - When the loop ends it runs `epic order`.
  - It compares the children's project-wide rules and writes each disagreement into Decisions to settle, naming the rule, the plans involved and one proposed answer.
  - The end-of-planning review then walks the summary document, decisions first. Each change is applied to the plan document and to that spec's section. A settled disagreement updates every plan involved and is marked settled with its outcome. Removing an added dependency uses `epic order --data unorder`.
- **Why a document rather than chat.** A summary document beats the chat-only review the user found dense and garbled: it can be re-read, it survives repeated planning, and the CLI enforces its decisions-first shape.

**Conventions that drive specific choices.**
- The new refusals (`epic_summary_not_found`, `epic_dependency_not_found`, invalid section) carry a runnable next step (*error messages must suggest remediation*).
- The summary is read and written only through `epic summary`, and child prompts restate that (*store files are written through the CLI*).
- Template and skill changes are verified with `go test` only, never by re-initialising this repo (*a plan never changes the active install*).

Rejected alternatives and the evidence behind them are in [research.md](./research.md#alternatives-considered-and-rejected).

## Component Breakdown

- **Dependency graph helpers (changed).** Gains a reachability query: does spec X reach spec Y through `depends_on`, directly or through other specs? The ordering pass asks it before every edge it might add, and asks again after each edge it adds. The existing cycle and topological-order helpers are unchanged.

- **Epic document model (changed).** Each entry in an epic's `specs` gains an optional `parallel_with` list. It names earlier specs the user chose to let run side by side despite shared files.
  - The model parses it, renders it only when non-empty, and validates it: members only, no self-reference.
  - Everything else about the epic format is unchanged.
  - `status`, implementing, splitting and joining all ignore the field.

- **Plan file overlap reader (new).** Given one plan's `plan` and `context` documents and the registered repo names, it returns, for each task, the set of (repo, path) pairs the task says it changes.
  - Files come from the backticked paths in the task's `### Task:` section of `context`, with line suffixes stripped.
  - A file is attributed to the repo its prefix names, or else to the task's own repo from `plan`.
  - It never fails. A plan without a context section simply contributes nothing.
  - It lives beside the existing plan task parser, which it reuses for task titles and repos.

- **Epic ordering command, `epic order` (new).** It owns ordering an epic's planned specs by overlap, and undoing that ordering.
  - It reads the epic and each member's plan through the store, uses the overlap reader and graph helpers to decide edges in list order, and writes the epic through the existing body-preserving epic transaction.
  - It reports added edges with their shared files.
  - It appends each event to the summary's ordering section through the summary component.
  - With `unorder`, it removes one edge and records `parallel_with`.

- **Epic summary command and document, `epic summary read/write` (new).** It owns the per-epic planning summary kept in the epic store.
  - It parses and renders the document as a fixed sequence of sections: title, Decisions to settle, Order added for shared files, then one section per planned spec in epic list order.
  - It replaces one section per write and refuses malformed or unknown sections.
  - It exposes an internal append hook for the ordering section, which `epic order` uses.

- **Epic delete (changed).** Also removes the epic's summary document and its folder, inside the same transaction as the epic, so a rollback restores both.

- **Plan step instructions (changed).**
  - A new shared partial states when to proceed and when to stop, and adds the recorded-decision and knowledge-entry rule. It replaces the eleven hand-copied paragraphs.
  - Discovery names where recorded decisions live.
  - Architecture and verification each check the plan against them.
  - Tasks and open questions forbid parking a contradiction as a person task or open question.
  - The orchestrated walkthrough summary gains a "project-wide rules" point, and the tasks step asks for backticked paths in every file change.
  - Ordinary runs keep their wording apart from the new rule. Under orchestration, the existing stop partial turns the new STOP into a `QUESTION:` hand-back.

- **`spek-plan-epic` skill (changed).** The orchestrator now does the following:
  - writes each child's section into the summary as it hands back;
  - runs `epic order` once the loop ends;
  - compares the children's project-wide rules and records disagreements with one proposed answer under Decisions to settle;
  - runs the end-of-planning review by walking the summary, applying every change to both the plan and the summary;
  - widens its definition of a genuine question, and its child prompt restates the summary's store rule.

- **`spek-plan` skill (changed, lightly).** Its orchestrated section mentions the extra summary point. Its standalone text mentions that contradicting a recorded decision or a knowledge entry stops for the user.

- **Epics design document (changed).** Records the `parallel_with` field, the summary document and the `epic order` / `epic summary` verbs, and replaces "Planning. Unchanged." with how planning an epic now ends. It is rewritten through `design author`.

- **Epics documentation page and site changelog (changed, `docs` repo).** The page describes:
  - the summary document;
  - the automatic ordering of overlapping specs, and undoing it;
  - the questions raised while planning;
  - the two new command-line verbs.

  The site changelog gains an entry.

## Data Structures & Interfaces

**Epic entry: `parallel_with` (epic frontmatter, changed).** It is optional and omitted when empty. It names earlier specs in the same epic that the user chose to let be implemented side by side even though their plans share files. Only the ordering pass reads it.

```yaml
specs:
    - name: 000071_references-as-written
      depends_on: []
    - name: 000072_user-depends-on
      depends_on: []
      parallel_with: [000071_references-as-written]   # user removed the added order at review
```

**Graph reachability (Go, new).** It answers whether `from` reaches `to` through `deps`, directly or transitively. Names absent from `deps` have no edges.

```go
func Reaches(deps map[string][]string, from, to string) bool
```

**Plan file overlap (Go, new).** For one plan, it gives the files each task says it changes.

```go
type FileRef struct { Repo, Path string }          // Path is repo-relative, no ":line"
func TaskFiles(plan, context []byte, repos []string) map[string][]FileRef  // task title -> files
```

**`epic order` (CLI, new).**
- **Input:** `epic order <epic> [--data '{"unorder":{"spec":"<later>","depends_on":"<earlier>"}}']`. It takes no `--from`.
- **Output when ordering:**

```json
{
  "epic": "000070_references-and-secrets",
  "added": [
    { "spec": "000072_user-depends-on", "depends_on": "000071_references-as-written",
      "files": [ { "repo": "docs", "path": "src/content/configuration-text.mdx" },
                 { "repo": "xcl",  "path": "encode.go" } ] }
  ],
  "unplanned": ["000074_secrets-store"]
}
```

`unplanned` lists members with no plan, which were left out of the comparison.

- **Output with `unorder`:** `{"epic": "...", "removed": {"spec": "...", "depends_on": "..."}}`.
- **Refusals:** `epic_name_required`, `epic_not_found` and `epic_invalid` (reused); `epic_dependency_not_found` (the edge to remove is not in the epic); `bad_input`.
- **Schema:** `--schema` publishes all of the above.

**`epic summary` (CLI, new).**
- **Read:** `epic summary read <epic>` writes the document to stdout unchanged, like `epic read`.
- **Write:** `epic summary write <epic> --data '{"section":"decisions"|"<member spec>"}' --from <path>`. It returns `{"epic": "...", "section": "...", "sections": ["decisions", "ordering", "<spec>", ...]}`.
- **Refusals:**
  - `epic_summary_not_found`: read with no summary. The next step says to plan the epic with "plan this epic", which writes it.
  - `epic_summary_section_invalid`: an unknown section, `ordering` (written only by `epic order`), a spec not in the epic, or a body containing a `#` or `##` heading.
  - `epic_from_required`, `epic_not_found` and `bad_input` (reused).

**Summary document (store document, new).** It lives in the epic store, in the epic's own folder. The CLI always renders it in this order:

```markdown
---
created_date: "2026-10-05"
---

# Planning summary: 000070_references-and-secrets

## Decisions to settle
None.

## Order added for shared files
- Added: `000072_user-depends-on` now depends on `000071_references-as-written`. Both change `docs:src/content/configuration-text.mdx`, `xcl:encode.go`.

## 000071_references-as-written
<the section body the orchestrator wrote>

## 000072_user-depends-on
<…>
```

- When a section is missing, Decisions renders "None." and the ordering section renders "None added."
- Spec sections appear in epic list order. A section for a spec no longer in the epic is kept, at the end.
- `created_date` is stamped once and preserved after that.

**Internal hook (Go, new).** `epic order` uses it to append ordering events without going through the CLI write path.

```go
func appendOrdering(t *docTxn, cfg config.Config, epic string, lines []string) error
```

## Implementation Detail

**Epic verbs follow the existing hand-written epic command pattern, not the generic store-file factory.**
- **Name and existence checks.** `epic order` and `epic summary` resolve and validate the epic name exactly as `epic read` and `epic write` do. A missing epic is refused with the same next step.
- **Transactions.** Every change goes through the cross-document transaction already used for linking specs. That includes the epic graph, the summary and the delete. So a failure part-way restores every document the command touched.
- **Ordering, as one transaction.** `epic order` follows the "body-preserving graph mutation" shape that joining a spec to an epic introduced: read the epic, clone its entries, change only `depends_on` / `parallel_with`, validate, stamp, render with the body untouched, and write. In the same transaction it appends to the summary's ordering section. The summary is created if it does not exist, and nothing is written when no edge is added.
- **`summary` is a nested command group** under `epic`, mirroring how `plan file` groups its document verbs. Its `--schema` output is published the same way as the other epic verbs.

**The summary is a small section-structured document with one parser and one renderer.**
- **Parsing.** The CLI splits the body on level-2 headings into named sections:
  - `Decisions to settle` → `decisions`
  - `Order added for shared files` → `ordering`
  - any other heading → a spec name
- **Rendering.** It always renders them in the canonical order, whatever the order of the writes. This one round-trip is the whole contract. A write replaces one section's body and re-renders, which is what keeps every other section byte-for-byte unchanged across repeated planning and review edits.
- **Content stays the agent's.** The CLI owns only the skeleton and the ordering log. The content of every other section is the orchestrator's, written from a child's `DONE:` summary or the review.

**Overlap detection is a pure function over two documents, kept separate from the command.**
- **The reader sits beside the plan task parser.** It reuses the parser for task titles and repos. It adds a tolerant scan of `context`'s `### Task:` sections, never failing, which mirrors the parser's own "never fails" contract.
- **The command is the only part with I/O.** It loads documents through the store and feeds the reader and the graph helper.
- **Tests stay deterministic.** Most of the logic is tested with in-memory fixtures; only a thin layer is tested through the CLI.
- **Edge decisions are made in list order.** The graph is consulted afresh before each candidate pair, so the result is deterministic and independent of map iteration.

**Instruction changes follow the established template-contract pattern.**
- **A partial replaces the copies.** The proceed-or-stop rule moves into a partial, included where the eleven copies were. This is the first plan-step partial included in template source rather than appended in code, and it uses the same partial resolver as the skills.
- **Pinned phrases.** New phrases are pinned by the existing phrase-assertion tests: the plan step tests, the orchestrated step tests and the epic skill tests.
- **Standalone text stays clean.** Standalone wording contains none of the orchestration-only words, so the guard that keeps orchestration out of ordinary runs still holds.
- **The skills.** `spek-plan-epic` keeps its existing structure (find, loop, child prompt, hand-back, progress, review, report). It gains summary writes in hand-back handling, an ordering-and-decisions pass between the loop and the review, and a review that walks the document.

**Design and docs.**
- **Epics design.** The design is updated through the design CLI, preserving its capture date and back-links.
- **Epics page.** The page gains subsections inside its existing "Planning and implementing an epic" and command-line sections. No new top-level band is added, so the surface alternation is untouched.

## Dependencies

- **000062_epic-plan-and-implement (prior plan, landed).** Provides the `spek-plan-epic` orchestrator, orchestrated plan lanes, the `DONE:` / `QUESTION:` / `FAILED:` hand-back, the stop partial that turns a STOP into `QUESTION:`, and the `status` run view. Everything here builds on it. It is already merged on this branch, so nothing must land first.
- **000060_epics-and-seeded-specs (prior plan, landed).** Provides the epic store, the `epic` verbs, the epic document model and its validation, and the cross-document transaction used for linking. This plan extends the model with `parallel_with` and adds two verbs.
- **000058_plan-task-graph (prior plan, landed).** Provides the plan task parser. The overlap reader reuses it for task titles and repos. The task format itself is not changed.
- **Internal packages (changed).**
  - The dependency graph helpers gain reachability.
  - The plan task package gains the file-overlap reader.
  - The epic model gains `parallel_with`.
  - The `epic` command group gains `order` and `summary`, and its delete removes the summary.
- **Internal packages (used unchanged).** The store, metadata stamping, the artifact addressing, the output error builder, the step-rendering kit and its partial resolver.
- **Plan step templates, partials and skills (changed).** The plan steps, a new proceed-or-stop partial, `spek-plan-epic` and `spek-plan`. Installed into projects by `init` / `migrate`, which need no change.
- **External libraries.** None new. The existing YAML library and mustache renderer are used as they are.
- **`docs` repo (spektacular-website).** The Epics page and the site changelog change. It depends on the CLI verbs' final names and output, so its milestone comes last.
- **Design documents this plan was built on: none.** The spec carries no design references. This plan also **updates** `epics-and-seeded-specs.md` from the `design` source, as the spec's constraint requires once the epic format changes. That design is a document to keep in step, not one this plan was built to.

## Testing Approach

Testing follows the project's three layers:
- Go unit and command tests for the deterministic CLI parts;
- template-contract phrase tests for the instruction changes;
- manual end-to-end checks for live-agent behaviour, captured in the implementation test plan.

**Unit tests (most coverage: the deterministic core).**
- **Reachability.** Covers direct, transitive, none, self and unknown names.
- **The overlap reader.** Uses hand-written plan and context fixtures, with expected values written by hand rather than derived from the parser, following the plan-task tests. It guarantees:
  - backticked paths are collected per task, and line suffixes are stripped;
  - a registered-repo prefix wins and an unregistered prefix is kept as part of the path;
  - unprefixed paths take the task's repo;
  - a plan without a context section contributes nothing and never errors.
- **The epic model.** `parallel_with` round-trips, is omitted when empty, and is refused when it names a non-member or the spec itself.
- **The summary parser and renderer.** Writing sections in any order always renders title, Decisions, Order added, then specs in epic list order. Replacing one section leaves every other section byte-for-byte unchanged. Empty Decisions renders "None.".

**Command tests (through the root command with a fresh temp project each, per the test-order convention).**
- **`epic order`.** It is load-bearing for the ordering criteria. Given independent A and B, listed first and second, whose plans share a file:
  - B gains `depends_on: [A]` without any prompt;
  - the output and the summary name the shared file;
  - A's and B's plan documents, and both specs, are byte-identical before and after, with created dates unchanged;
  - pairs already ordered directly or through a third spec gain nothing;
  - a second run adds nothing;
  - unplanned members are listed and skipped.
- **`epic order` with `unorder`.** It removes the edge and records `parallel_with`, a later `epic order` does not re-add it, and the summary logs the removal. Removing a non-existent edge is refused with a next step.
- **`epic summary`.** Read and write round-trip. Read without a summary is refused with `epic_summary_not_found`, whose next step names "plan this epic". Writing `ordering`, a non-member spec, an unknown section or a body containing a top-level heading is refused, and a refused write changes nothing on disk.
- **`epic delete`.** Removes the summary with the epic, and a forced mid-transaction failure restores both.
- **Schema test.** The epic schema test lists both new verbs.

**Template-contract tests (instruction behaviour).**
- **Plan steps.** Every gathering step still renders "proceed without interruption" through the new partial, and also renders the rule that contradicting a recorded decision or a knowledge entry is a stop for the user. Discovery names the spec's sections, referenced designs and, where it exists, `interview.md`. Architecture and verification carry the explicit check. Tasks and open questions forbid parking a contradiction as a `human` task or open question.
- **Orchestrated rendering.** The same STOP still carries the `QUESTION:` hand-back. The orchestrated walkthrough and `DONE:` summary list the fifth point, project-wide rules.
- **Standalone rendering.** It still contains no orchestration words, and its step list and sign-off are unchanged, which is the "single-spec planning is otherwise unchanged" guarantee.
- **The `spek-plan-epic` skill.** Pinned phrases cover:
  - writing each child's section with `epic summary write` on `DONE:`;
  - running `epic order` when the loop ends;
  - recording cross-plan disagreements under Decisions to settle with the rule, the plans and one proposed answer;
  - the review walking the summary decisions first;
  - every change applied to both the plan and the summary;
  - settling a disagreement updating every plan involved and marking it settled;
  - undoing an added dependency with `unorder`;
  - the widened genuine-question definition.

  The ask-word allowlist is extended for any new sentence containing "ask".
- **Harbor.** The plan-workflow harbor oracles are checked for drift in the same change. The step order is unchanged, but any command-substring oracle the edited steps name must still hold.

**Spec success metrics.**
- **No merge conflict from two specs that planning left unordered while both changed the same file** — Manual — captured in the implementation test plan. It needs real epic implementation runs. The `epic order` command tests guarantee the mechanism: any shared file between unordered planned specs yields an edge.
- **No contradiction of a recorded decision or knowledge entry first appears at the review; each is raised while planning runs** — Manual — captured in the implementation test plan. It needs live agents judging contradictions. The template-contract tests guarantee every plan step carries the stop rule, and that orchestration turns it into a mid-run question.
- **Users settle every listed decision without opening an individual plan** — Manual — captured in the implementation test plan. It is observed in real epic reviews. The skill tests guarantee each decision names the rule, the plans and a proposed answer, and that settling updates every plan.

**Also manual (live-agent acceptance criteria).** The following are flagged for the implementation test plan, because they need real agents driving an epic:
- the three-spec end-to-end summary;
- repeated planning keeping the first section unchanged;
- a review change landing in both plan and summary;
- the design-contradiction question while an independent spec keeps planning;
- the knowledge-contradiction question;
- the changelog disagreement with its proposed answer and settlement;
- the published docs page.

**Deliberate gaps.** No new harbor suite is added for epic planning. 000062 left epic orchestration to manual test-plan procedures, and this plan follows that, because a multi-agent epic run does not fit the single-agent harbor harness.

## Milestones & Tasks

### Milestone 1: Planning asks before contradicting your decisions or the knowledge base

**What changes**: Whenever a plan is drafted, on its own or as part of an epic, the planner now checks its choices against what the user already decided and against the knowledge base, before the plan is final.
- **What counts as a recorded decision:** the spec's own sections, any design the spec references, and the spec's interview notes where those still exist.
- **When a choice contradicts one:** planning stops and asks the user. It never leaves the contradiction as a task for a person, an open question, or a note for the review.
- **During epic planning:** the stop reaches the user as a question while the other specs keep planning.
- **Everything else is unchanged:** planning one spec keeps its steps, its other questions and its sign-off. The instruction text that says when to proceed and when to stop now lives in one shared place instead of eleven copies, so the rule cannot drift between steps.

**Validation point**: The rendered plan steps carry the new stop rule in every drafting step. Architecture and verification carry the explicit check, and tasks and open questions forbid parking a contradiction. Orchestrated rendering turns the stop into a question hand-back. Ordinary single-spec rendering is otherwise unchanged. The full test suite passes.

#### - [ ] Task: Make contradicting a recorded decision or knowledge entry a stop in every plan step
**Id:** 73cd67dd-ae53-4fcd-8aa6-836626ebbd4a
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

The eleven drafting steps each carry their own copy of the paragraph that says when to carry on and when to stop for the user. Move it into one shared piece of instruction text that every one of them includes. Add the new rule there: a plan choice that would contradict a decision the user recorded for the spec, or a knowledge entry, always stops for the user. Under epic planning, the existing hand-back turns that stop into a question automatically.

*Technical detail:* [context.md#task-make-contradicting-a-recorded-decision-or-knowledge-entry-a-stop-in-every-plan-step](./context.md#task-make-contradicting-a-recorded-decision-or-knowledge-entry-a-stop-in-every-plan-step)

**Acceptance criteria**:
- [ ] Every drafting step of the plan workflow tells the agent to carry on without interruption, except to stop and ask when a choice would contradict a recorded decision or a knowledge entry.
- [ ] The wording lives in one place, and every drafting step shows the same text.
- [ ] When the plan is run for an epic, the same stop is handed back as a question to the orchestrator.
- [ ] Planning one spec on its own shows no orchestration wording, and its steps are otherwise unchanged.

#### - [ ] Task: Check plans against recorded decisions and knowledge before they are final
**Id:** f4575a0e-c560-44fa-aadb-e4e61e4a2f95
**Repo:** spektacular
**Depends on:**
- 73cd67dd-ae53-4fcd-8aa6-836626ebbd4a — Make contradicting a recorded decision or knowledge entry a stop in every plan step
**Execution:** agent

Give the planner explicit places to apply the new rule:
- Discovery names where the user's recorded decisions live: the spec's sections, any referenced design, and the spec's interview notes when they still exist.
- Architecture checks the chosen direction against them.
- Verification checks the finished plan against them and against the knowledge base before anything is written.
- The tasks and open-questions steps say a contradiction may never be parked as a task for a person or an open question.

The plan skill's own description says the same.

*Technical detail:* [context.md#task-check-plans-against-recorded-decisions-and-knowledge-before-they-are-final](./context.md#task-check-plans-against-recorded-decisions-and-knowledge-before-they-are-final)

**Acceptance criteria**:
- [ ] Discovery tells the planner which recorded decisions to check, and says interview notes are read only when they still exist.
- [ ] Architecture and verification each tell the planner to compare the plan with the spec's recorded decisions, referenced designs and knowledge entries, and to stop and ask on any contradiction.
- [ ] The tasks step says a contradiction is never a task for a person, and the open-questions step says it is never parked as an open question.
- [ ] A plan for one spec on its own still has the same steps and sign-off as before.

### Milestone 2: An epic keeps a planning summary, and overlapping specs are ordered automatically

**What changes**: Two new commands appear, and the epic format gains one optional field.
- **The summary document.** An epic can now hold one summary document kept with the epic, read and updated with the new `epic summary` command. Its decisions always come first, followed by any order added for shared files and then one section per planned spec. Updating one section never disturbs the others. Asking for a summary that does not exist says how to get one. Deleting an epic deletes its summary too.
- **Ordering overlapping specs.** A new `epic order` command compares the files each planned spec's tasks change. When two specs share a file and nothing orders them, directly or through other specs, the later-listed spec is made to depend on the earlier one, without asking. Each addition is reported with the shared files and recorded in the summary.
- **Nothing is re-planned.** Adding a dependency changes only the order the specs are implemented in. No spec or plan changes.
- **Undoing an addition.** An added dependency can be undone, and the undo is remembered so it is never re-added.
- **The epics design** is updated to describe the new field, the summary and the commands.

**Validation point**: In a throwaway project with an epic of three planned specs, two of which share a file, `epic order` adds exactly one dependency, reports the shared file and leaves every plan byte-identical. A second run adds nothing. Undoing it removes the dependency and a later run does not re-add it. `epic summary` reads and writes sections in their fixed order. The full test suite passes.

#### - [ ] Task: Find the files each planned task changes
**Id:** 77b6c91f-d4e2-455a-985f-54b78e2b93e8
**Repo:** spektacular
**Depends on:**
- f4575a0e-c560-44fa-aadb-e4e61e4a2f95 — Check plans against recorded decisions and knowledge before they are final
**Execution:** agent

Add two small building blocks for ordering:
- A reader that lists, for each task in a plan, the files its technical notes say it changes, attributed to the right repo.
- A graph query that says whether one spec already comes after another, directly or through other specs.

The tasks step is tightened so every file change in the technical notes starts with the file's path in backticks, so new plans are always readable this way.

*Technical detail:* [context.md#task-find-the-files-each-planned-task-changes](./context.md#task-find-the-files-each-planned-task-changes)

**Acceptance criteria**:
- [ ] For a plan with technical notes, the reader returns each task's files with the right repo, ignoring line numbers.
- [ ] A plan without technical notes yields no files and never fails.
- [ ] The graph query is true for a direct or indirect dependency and false otherwise.
- [ ] The tasks step asks for every file change to start with its path in backticks.

#### - [ ] Task: Keep a planning summary document with each epic
**Id:** eb25fb94-3707-4956-8e35-8867fb6e69d0
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add `epic summary read` and `epic summary write`. They keep one summary document per epic in the epic store, written one section at a time. The CLI always lays it out in the same order: the decisions to settle, then any order added for shared files, then one section per planned spec in epic order. Updating one section never changes another. Reading a summary that does not exist says how to get one, and deleting an epic deletes its summary too.

*Technical detail:* [context.md#task-keep-a-planning-summary-document-with-each-epic](./context.md#task-keep-a-planning-summary-document-with-each-epic)

**Acceptance criteria**:
- [ ] An epic's summary can be written section by section and read back, with decisions first whatever order the sections were written in.
- [ ] The decisions section is always present and says there are none when it is empty.
- [ ] Rewriting one spec's section leaves every other section exactly as it was.
- [ ] Reading the summary of an epic that has none is refused with a message saying how to get one.
- [ ] Writing an unknown section, a spec outside the epic, or content that would break the layout is refused and changes nothing.
- [ ] Deleting an epic also deletes its summary.

#### - [ ] Task: Order overlapping specs automatically
**Id:** 519d3710-8a35-465d-aaaa-69e6a11a9f36
**Repo:** spektacular
**Depends on:**
- 77b6c91f-d4e2-455a-985f-54b78e2b93e8 — Find the files each planned task changes
- eb25fb94-3707-4956-8e35-8867fb6e69d0 — Keep a planning summary document with each epic
**Execution:** agent

Add `epic order`. For every pair of planned specs in an epic that change the same file and have no ordering between them, directly or through other specs, it makes the later-listed spec depend on the earlier one, without asking. Each addition is returned and logged in the summary with the shared files. Only the epic changes, so no spec or plan is touched or re-planned. The same command can undo one added dependency. It remembers the pair as allowed to run side by side, so it is never added again.

*Technical detail:* [context.md#task-order-overlapping-specs-automatically](./context.md#task-order-overlapping-specs-automatically)

**Acceptance criteria**:
- [ ] Two independent planned specs that change the same file end up with the later-listed one depending on the earlier one, and the user is not asked.
- [ ] Specs already ordered directly or through another spec gain no new dependency, and running the command again adds nothing.
- [ ] The added dependency is listed in the summary and in the command's result, naming the shared files.
- [ ] Both specs' plans keep the same content and created date before and after.
- [ ] Undoing an added dependency removes it from the epic, the summary shows it was removed, and a later run does not add it back.
- [ ] Specs without a plan are left out and reported as unplanned.

#### - [ ] Task: Update the epics design for the summary and automatic ordering
**Id:** 0633acc8-d444-41ef-94ef-2a24eb4b900a
**Repo:** spektacular
**Depends on:**
- 519d3710-8a35-465d-aaaa-69e6a11a9f36 — Order overlapping specs automatically
**Execution:** agent

The epic format gains an optional field, so the epics design document must say so. Update it to describe:
- the optional field;
- the summary document kept with an epic;
- the two new commands;
- how planning an epic now ends.

This replaces its line that planning is unchanged. The document keeps its capture date and its links to the specs that reference it.

*Technical detail:* [context.md#task-update-the-epics-design-for-the-summary-and-automatic-ordering](./context.md#task-update-the-epics-design-for-the-summary-and-automatic-ordering)

**Acceptance criteria**:
- [ ] The epics design describes the new optional field, the summary document, both commands, and automatic ordering at the end of epic planning.
- [ ] The design keeps its original capture date and its links to the specs that reference it.

### Milestone 3: "Plan this epic" ends with one summary document to review

**What changes**: Planning an epic with one request now builds the epic's summary document as each spec's plan finishes.
- **Ordering.** When planning ends, the orchestrator orders overlapping specs automatically.
- **Cross-plan disagreements.** It compares the rules each plan follows across the project, such as how the changelog is kept. Every disagreement goes under the decisions to settle, naming the plans involved and one proposed answer.
- **The review.** The end-of-planning review walks that document instead of a dense list in chat. Decisions come first.
- **Applying changes.** Any change the user asks for is made to the affected plan and its summary section together. Settling a disagreement updates every plan involved and marks it settled. An added dependency can be removed there and then.
- **Repeating the request.** Repeating "plan this epic" keeps the sections for specs planned before and adds sections for newly planned ones.

**Validation point**: The rendered `spek-plan-epic` skill and orchestrated plan steps carry the summary writes, the ordering pass, the decisions-with-a-proposed-answer rule, the summary-driven review and the widened question definition, each pinned by tests. The full test suite passes. The live end-to-end behaviour is listed in the implementation test plan.

#### - [ ] Task: Hand back the project-wide rules each plan follows
**Id:** 09097321-9971-4411-b1b1-ff1fbbd5f5ce
**Repo:** spektacular
**Depends on:**
- f4575a0e-c560-44fa-aadb-e4e61e4a2f95 — Check plans against recorded decisions and knowledge before they are final
**Execution:** agent

When a plan is made for an epic, the summary it hands back gains one more point: the rules it follows that govern files or practices beyond its own spec, such as how the changelog is kept. Without it, the orchestrator cannot see that two plans disagree. Planning one spec on its own is unaffected.

*Technical detail:* [context.md#task-hand-back-the-project-wide-rules-each-plan-follows](./context.md#task-hand-back-the-project-wide-rules-each-plan-follows)

**Acceptance criteria**:
- [ ] A plan made for an epic hands back the project-wide rules it relies on or decides, one per line, alongside its approach, tasks, scope and assumptions.
- [ ] The plan skill's section on running under an orchestrator mentions the extra point.
- [ ] Planning one spec on its own still ends with the same sign-off walkthrough.

#### - [ ] Task: Build and review the epic's planning summary in "plan this epic"
**Id:** 91495004-f25f-4fd6-8ab9-cf1376893901
**Repo:** spektacular
**Depends on:**
- 73cd67dd-ae53-4fcd-8aa6-836626ebbd4a — Make contradicting a recorded decision or knowledge entry a stop in every plan step
- 519d3710-8a35-465d-aaaa-69e6a11a9f36 — Order overlapping specs automatically
- 09097321-9971-4411-b1b1-ff1fbbd5f5ce — Hand back the project-wide rules each plan follows
**Execution:** agent

Teach the "plan this epic" skill to build the summary as it goes:
- **On each finish.** Write each spec's section when its plan finishes.
- **When the loop ends.** Order overlapping specs. Compare the project-wide rules the plans handed back, and list every disagreement under the decisions to settle, naming the plans and one proposed answer.
- **The review.** Walk the summary document with the user, decisions first. Each change goes to both the plan and its section. Settling a disagreement updates every plan involved and marks it settled. An added dependency can be removed there.
- **Questions.** Its definition of a question worth stopping for now includes contradicting the user's recorded decisions or a knowledge entry.

*Technical detail:* [context.md#task-build-and-review-the-epics-planning-summary-in-plan-this-epic](./context.md#task-build-and-review-the-epics-planning-summary-in-plan-this-epic)

**Acceptance criteria**:
- [ ] Each spec planned in a run gets its own section in the epic's summary, and sections for specs planned before are kept.
- [ ] When planning ends, overlapping specs are ordered and every dependency added is visible in the summary before the review.
- [ ] Each disagreement between plans on a project-wide rule is listed under the decisions to settle, with the plans involved and one proposed answer.
- [ ] The review presents the summary, decisions first. A change the user asks for is made to the plan and to its section in the summary.
- [ ] Settling a disagreement, by accepting the proposal or choosing another answer, updates every plan involved and marks the decision settled.
- [ ] The skill counts contradicting a recorded decision or a knowledge entry as a genuine question, and every other spec keeps planning while it waits.

### Milestone 4: The epics documentation describes the summary, the ordering and the questions

**What changes**: The public Epics page explains:
- the summary document an epic gets when it is planned with one request, and the command to read it;
- how overlapping specs are ordered automatically, and how to undo an added dependency at the review;
- which questions planning now stops to ask, and that the other specs keep planning meanwhile.

The site changelog records the change.

**Validation point**: The site builds and type-checks cleanly, page bodies contain no layout markup and no em dashes, and the Epics page's section shading still alternates.

#### - [ ] Task: Document the epic planning summary, automatic ordering and planning questions
**Id:** d989203f-7328-47c8-a96a-d30530f137b5
**Repo:** docs
**Depends on:**
- 519d3710-8a35-465d-aaaa-69e6a11a9f36 — Order overlapping specs automatically
- 91495004-f25f-4fd6-8ab9-cf1376893901 — Build and review the epic's planning summary in "plan this epic"
**Execution:** agent

Extend the Epics page:
- **"Plan this epic"** describes the summary document and the automatic ordering of overlapping specs, including undoing an added dependency at the review.
- **"What still stops for you"** describes the new questions about recorded decisions and the knowledge base.
- **The command-line section** gains `epic summary` and `epic order`.

The site changelog records the change.

*Technical detail:* [context.md#task-document-the-epic-planning-summary-automatic-ordering-and-planning-questions](./context.md#task-document-the-epic-planning-summary-automatic-ordering-and-planning-questions)

**Acceptance criteria**:
- [ ] The Epics page describes the summary document, the automatic ordering of overlapping specs and the questions raised while planning.
- [ ] The command-line section shows how to read the summary and how to order or unorder specs.
- [ ] The site changelog has an entry for this change.
- [ ] The site builds and checks cleanly, and the page's section shading still alternates.

## Open Questions

- **Do plans written before this change name their files in a shape the overlap reader picks up?**
  - **Why it matters:** the reader collects backticked paths from each task's technical notes. That is the format the tasks step has always prescribed, but older plans may have phrased some file changes in prose without backticks.
  - **Depends on:** running `epic order` against a real epic whose plans predate this change, such as the user's own epic that prompted this spec.
  - **What the implementer should do:** run it against such an epic, or against this repo's own planned specs, and compare the files it reports with the plans by eye. If a material number of file changes are missed, STOP and ask the user whether to widen the reader (for example, to unbackticked paths at the start of a bullet) or to accept that older plans are only partly covered.

## Out of Scope

**From the spec's non-goals:**
- A summary document for an epic that was not planned with one request. `epic summary` exists for any epic, but only "plan this epic" fills it.
- Any change to how implementing an epic works, apart from it following the dependencies `epic order` adds.
- Refusing tasks for a person that only ask for a review. That is a separate follow-up.
- Settling what any project's own rules are, such as how a project keeps its changelog. Planning surfaces the disagreement and proposes an answer, and the user decides.

**Left out by this plan's design:**
- Keeping the spec workflow's interview notes after a spec is written. Planners read them only when they still exist, and the spec's sections remain the record of the user's decisions.
- Showing overlaps or `parallel_with` in `status`. The run view and its schema are unchanged, and `epic order`'s own result and the summary report the ordering.
- Comparing files across specs from real code changes. Overlap is read from what the plans say they change. Merge-time conflict detection when implementing is unchanged.
- A new harbor end-to-end suite for epic planning. Live epic behaviour is covered by the implementation test plan, as in 000062.
- Changing the plan task format in `plan`, including any per-task file list there. Files stay in the plan's technical notes.
