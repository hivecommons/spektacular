---
created_date: "2026-10-08"
document_status: final
closed_date: "2026-10-08"
---

# Test Plan: 000066_epic-mid-run-revisions

The automated suite covers the mechanics:
- `spec amend` writes the new text, an `## Amendments` entry and a metadata record, and writes only the project's spec store;
- a recorded amendment keeps a strict-mode plan current for `status`, `implement new`/`goto` and an orchestrated lane;
- an unrecorded edit still makes the plan stale;
- the step and skill templates carry the stop, apply and re-verify wording.

The procedures below cover the success metrics and reviews the plan marks as manual. They need a release-shaped install of this branch's `spektacular`, with the regenerated skills: run `make install-local` from the merged checkout, then `spektacular migrate` in the throwaway project used.

## Setting up the seeded conflict (shared by M1 to M3)

1. In a throwaway git project with a second registered repo, run `spektacular init claude` and set `plan.strict_spec_changes: true` in `.spektacular/config.yaml`.
2. Author a design, `design/rounding.md`, in a declared design source with `spektacular design author`. Its rule: "amounts round half-even".
3. Create an epic with two speks, `000002_billing` and `000003_invoices`, where `000003_invoices` depends on `000002_billing`. Reference the design from `000002_billing` with `spektacular design ref add`. Give `000002_billing` the success metric "Amounts round half-up", which contradicts the design.
4. Plan the epic ("plan this epic"), approve the plans, then commit everything.

## M1. An epic run with a spec/design conflict completes without hand edits

- **What to measure:** the epic run finishes with both speks implemented and merged. Nobody edits a file by hand, no `override_dependencies` is used, and nothing is written under any `.spektacular/worktrees/*/*/.spektacular/`.
- **How:**
  1. Ask the agent to "implement this epic".
  2. When `000002_billing`'s child hands back `QUESTION: 000002_billing`, check that the question names the spek, the Success Metrics section, the conflict with `design/rounding.md` and a proposed amendment.
  3. Before you answer, check that `spektacular spec file read 000002_billing` and the design are unchanged.
  4. Approve "amend the spek to half-even".
  5. Let the run finish.
  6. Throughout, run `git -C .spektacular/worktrees/000002_billing/<repo> status --short -- .spektacular` for each repo (and the same for `000003_invoices`). It must stay empty.
- **Expected result:**
  - `spektacular status <epic> --format json` reports both speks `done`, and no spek is ever reported `stale`.
  - The orchestrator applied the amendment itself, from the project root.
  - The child's next turn re-read the spek and re-ran verification before advancing.
  - No `override_dependencies` appears in either plan's changelog.
- **Who / when:** the release owner, before releasing this branch, on a local machine with Claude Code.

## M2. The spek explains its own history

- **What to measure:** after the M1 run, the spek's text matches what was built, and its `## Amendments` section explains every mid-run change without reading the plan changelog.
- **How:** run `spektacular spec file read 000002_billing`.
- **Expected result:**
  - The Success Metrics section says "half-even".
  - `## Amendments` holds one entry of the form `- **<date>: Success Metrics** (epic <epic> implement run, task <task>)`, with the reason on the next line.
  - The frontmatter `amendments` list has one record whose `hash` matches.
  - The plan's changelog entry for that task carries an `**Amendments**:` line naming the spek, Success Metrics and the reason.
  - The built code rounds half-even.
- **Who / when:** the release owner, immediately after M1.

## M3. No epic merge is refused because of the amendment

- **What to measure:** no `epic merge` in the M1 run is refused with `epic_merge_touches_spektacular`.
- **How:** review the orchestrator's progress lines and final report from M1, and run `git log --oneline` in each repo's main checkout.
- **Expected result:**
  - Both speks' merge commits are present.
  - The final report lists no spek stopped by `epic_merge_touches_spektacular`.
  - No worktrees are left behind.
- **Who / when:** the release owner, immediately after M1.

## R1. Harbor implement-workflow run

- **What to review:** the implement workflow still drives end to end with the new spec-conflict section in the analyze, implement, test and verify steps.
- **How:** from the spektacular checkout, run `make harbor-install` once, then `make harbor-test-implement`.
- **Passing:** the suite passes, including its `EXPECTED_STEP_ORDER` oracle (unchanged by this spec), and the agent transcript shows no stop raised for a spec conflict, since the fixture has none.
- **Who / when:** the release owner, before releasing this branch.

## R2. Read-through of the updated docs pages

- **What to review:** the new content reads correctly and fits the site.
- **How:** in the spektacular-website checkout, run `npm run build && npm run preview`. Open:
  - `/how-it-works/`: the "Implement the Spek" stage, its last paragraph;
  - `/epics/`: "What still stops for you", then the new "When the spek or a design is wrong" subsection;
  - `/documents/`: "Amending a spek during implementation";
  - `/design-documents/`: "A design can be corrected while it is being built";
  - `/configuration/`: the `plan` key and its example;
  - `/plan-tasks/`: the `stale` state.
- **Passing:**
  - Each page names the stop, your approval, `spec amend` and the `## Amendments` record where relevant.
  - The code blocks render.
  - The how-it-works paragraph's link reaches the Epics page.
  - There are no em dashes in the new text.
  - Section background shading still alternates, since no new top-level section was added.
- **Who / when:** the docs owner, before the site is deployed.
