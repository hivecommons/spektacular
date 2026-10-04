# Working context — plan 000063_epic-planning-summary-and-reordering

## Origin (carried from the spec session)
- Came from the user's first real "plan this epic" run on their xcl project's 5-spec epic `references-and-secrets`: two independent specs both changed `configuration-text.mdx` / `encode.go` with no ordering; plans disagreed on CHANGELOG handling; one plan contradicted a user-chosen interface and another a knowledge entry — all only surfaced in a dense end-of-run review.
- User decisions from the spec session: summary doc lives in the epic store, read/written via `spektacular epic ...`; overlap dependencies are written without asking ("99% of the time folks are just going to agree"); earlier-listed spec goes first; no re-plan on added dependency; new contradiction questions apply to all planning (single-spec otherwise unchanged).

## Plan session
- User chose spec 000063 to plan.
- Discovery done. No design refs on the spec; the epics design (`epics-and-seeded-specs.md`) must still be updated (constraint) if the epic format changes.
- Key learnings: per-task files live only in context.md "File changes" (backticked paths, `<repo>:` prefix for other repos); plan.md forbids file refs. Spec work dir (interview.md) is deleted at spec verification, so interview notes are normally unavailable. Any new STOP in a plan step becomes a QUESTION: under orchestration automatically via `partials/orchestrated-stop.md`. Standalone step text must not contain "orchestrat"/"QUESTION:"/"DONE:" (cmd/orchestrated_test.go). depgraph has no reachability helper.
- Architecture chosen: `epic order` (context.md file overlap, depgraph.Reaches, parallel_with on unorder), `epic summary read/write` (section-addressed, epics/<epic>/summary.md, Decisions first), shared proceed-unless-blocked partial with recorded-decision/knowledge STOP, walkthrough summary +project-wide rules, spek-plan-epic writes/walks the summary. Design doc update via `design author`.
- Components + data structures drafted: `parallel_with` on epic spec entries; depgraph.Reaches; plantask TaskFiles(plan, context, repos); `epic order` (+unorder), `epic summary read/write` (sections: decisions, ordering [CLI-only], <spec>); summary rendered title→Decisions→Order added→spec sections.
- Implementation detail drafted (epic hand-written verb pattern, docTxn, summary parse/render round-trip, pure overlap reader, partial for proceed rule).
- Dependencies drafted (000062/000060/000058 landed; no design refs; epics design updated, not built on).
- Testing approach drafted: all 3 success metrics are manual (test plan); epic order/summary command tests are load-bearing.
- Milestones: M1 stop rule in plan steps (all planning); M2 epic summary + epic order CLI + design update; M3 spek-plan-epic summary/review; M4 docs.
- Tasks drafted: 9 tasks (ids allocated), docs task depends on epic order + spek-plan-epic task.
- Open questions: one (old plans' file-change shape vs reader).
- Out of scope drafted.
- Assembled and staged plan/context/research to .spektacular/tmp/<plan>/.
- Verification: removed binary-name commands from plan.md sources; anchors/ids checked.
- plan.md written to store.
- context.md written to store.
- research.md written; work dir removed. Next: walkthrough with user.
- User signed off the walkthrough; advancing to finished.
