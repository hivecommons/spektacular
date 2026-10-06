---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Test plan: 000063_epic-planning-summary-and-reordering

The deterministic parts are covered by `go test`:
- `epic order`, including unorder, transitive skip, idempotence, byte-identical plans and the split carry-over (`cmd/epic_order_test.go`, `cmd/epic_split_test.go`);
- `epic summary` and delete (`cmd/epic_summary_test.go`, `internal/epic/summary_test.go`);
- the file reader and reachability (`internal/plantask/files_test.go`, `internal/depgraph/depgraph_test.go`);
- every instruction change, as template-contract tests (`internal/steps/plan/*_test.go`, `templates/plan_epic_skill_test.go`, `templates/orchestrated_skill_section_test.go`).

What follows needs live agents and real epic runs.

**Setup for every procedure.**
- Install the new binary between workflows with `make install-local`, never while a workflow is in progress.
- Create a throwaway project with `spektacular init` in an empty git repo, and register a second repo with the `spek-manage-repos` skill when a procedure needs two.
- Write the specs with the `spek-new` skill and group them with `spektacular epic write <epic> --from <body> --data '{"specs":[...]}'`.
- Plan with "plan this epic" (the `spek-plan-epic` skill), and read results with `spektacular epic summary read <epic>`, `spektacular epic read <epic>` and `spektacular status <epic> --format json`.

## Success metrics

### 1. No merge conflict from two specs that planning left unordered while both changed the same file
- **What to measure:** across real epic implementation runs, the number of `epic merge` conflicts between two specs that had no dependency path between them after planning while both plans named the same file. Threshold: **0**.
- **How:**
  1. For each real epic planned with "plan this epic", record the `added` list from the end-of-planning `spektacular epic order <epic>` (it is also logged under "Order added for shared files" in `epic summary read`).
  2. Implement the epic with "implement this epic".
  3. For every merge conflict the run reports, check with `spektacular epic read <epic>` whether the two specs were ordered. Check whether both plans' context documents name the conflicting file in a `### Task:` section (`spektacular plan file read <spec> context`).
- **Expected result:** every conflicting pair was either already ordered, or does not name the conflicting file in both plans' technical notes. In the second case it is a gap in the plans, not in `epic order`, and should be logged as such. Count of unordered conflicts where both plans named the file: 0.
- **Who / when:** the maintainer, over the next few real epics (starting with the xcl `references-and-secrets` epic), before calling the feature proven.

### 2. No contradiction of a recorded decision or knowledge entry first appears at the end-of-planning review
- **What to measure:** in real epic planning runs, the number of contradictions of a recorded user decision (spec sections, referenced design, interview notes) or a knowledge entry that surface for the first time in the Step 7 review rather than as a `QUESTION:` while planning. Threshold: **0**.
- **How:** during each "plan this epic" run, note every question relayed to you. At the review, read each plan's drafting assumptions and the summary's sections, and look for any that contradict the spec or a knowledge entry (`spektacular knowledge search <topic>`).
- **Expected result:** every contradiction found was already raised as a question during the run. None appears first in the summary or as a `human` task.
- **Who / when:** the maintainer, on each real epic planning run.

### 3. Users settle every listed decision without opening an individual plan
- **What to measure:** in real epic reviews, the number of listed decisions under "Decisions to settle" for which the user had to open a plan (`plan file read`) to decide. Threshold: **0**.
- **How:** at each review, settle every decision from the summary text alone. Note any decision whose entry did not name the rule, every plan involved with what each does, and one proposed answer with its reason.
- **Expected result:** every decision is settled from the summary alone, and its entry reads "Settled: <outcome>" afterwards.
- **Who / when:** the maintainer, on each real epic review.

## Live-agent acceptance criteria

Run each of these once against the new binary, in a throwaway project.

1. **A three-spec end-to-end summary.**
   - Epic of three unplanned, independent specs; run "plan this epic".
   - Pass: `epic summary read` shows "Decisions to settle" first, "Order added for shared files", then exactly three spec sections in epic order. Each section covers approach, milestones and tasks, tasks needing a person (or "none"), out of scope and drafting assumptions.
2. **Repeated planning keeps the first section.**
   - Start "plan this epic" and decline to answer the first question (stopping mode) once one spec is `DONE:`. Save `epic summary read` output, repeat the request to completion, and compare.
   - Pass: the first spec's section is byte-identical, and the other sections were added.
3. **A review change lands in both plan and summary.**
   - At the review, ask for a change to one plan, then read that plan (`plan file read <spec> plan`) and the summary.
   - Pass: both show the change.
4. **Overlapping specs are ordered and the order can be undone.**
   - Two independent specs A then B whose plans both name the same file in their context documents.
   - Pass: after planning, `epic read` shows B `depends_on: [A]` with no question asked, and the summary names the shared file. At the review, ask to remove it. Then `epic read` shows B `depends_on: []` and `parallel_with: [A]`, the summary has the "Removed at review" line, and a further `spektacular epic order <epic>` returns `"added": []`.
5. **Design contradiction is a question while another spec keeps planning.**
   - Spec X references a design recording choice P, and one requirement pushes toward not-P. Spec Y is independent.
   - Pass: a `QUESTION:` about the contradiction is relayed before X's plan is final, and Y's plan progresses meanwhile (`status` shows Y's step advancing).
6. **Knowledge contradiction is a question.**
   - A spec whose requirement contradicts a knowledge entry.
   - Pass: planning asks before the plan is final, and no task in the plan assigns updating the entry to a person.
7. **Changelog disagreement.**
   - Two specs whose plans keep `CHANGELOG.md` differently (one hand-edited, one generated).
   - Pass: "Decisions to settle" names the rule, both plans and one proposed answer with status "Open". Accept it: both plans follow it and the entry reads "Settled: …". Repeat with a different answer of your own: both plans follow that.
8. **Single-spec planning is otherwise unchanged.**
   - Plan one standalone spec with `spek-plan`.
   - Pass: no summary is written (`epic summary read` has no epic to read), no epic changes, and the walkthrough and sign-off are as before. The only new stops are contradiction questions.
9. **Published docs.**
   - After the site deploys, open spektacular.dev/epics/.
   - Pass: "Plan this epic", "What still stops for you" and the command-line section describe the summary, the automatic ordering and the questions, and the "Plan this epic" link in "Dependencies between specs" jumps to the right heading.
