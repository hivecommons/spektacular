# Working context — spec 000063_epic-planning-summary-and-reordering

## Origin
- Came out of the user's first real run of "plan this epic" (from 000062) on their xcl project's 5-spec epic `references-and-secrets` (auto_commit off there).
- The run produced 5 final plans with no mid-run questions, but the end-of-planning review exposed problems:
  - Two independent specs (references-as-written, user-depends-on) both change `configuration-text.mdx` and `encode.go`, and the epic did not order them, so implementing them in parallel would conflict at merge.
  - Plans disagreed on a project-wide rule (CHANGELOG.md hand-edited vs generated). The user is "not sure what is going on with changelog" (that is the xcl repo's own question, not Spektacular's).
  - A plan departed from an interface the user had chosen, and another contradicted a binding knowledge entry. Both surfaced only in the review, not as questions during the run.
  - The review output in chat was dense and garbled (wrapped, truncated lines), with the decisions buried after five summaries.

## Decisions (user's words)
- "we need a summary doc which contains a summary of each plan as a section". The user accepted my proposal: one document per epic in the epic store, read/written via `spektacular epic ...`, one section per spec from the child's DONE: summary, a leading "Decisions to settle" section; the review walks it, and changes go into the plan and the summary.
- "When planning, if dependencies like the documentation one, then we should rebuild the dependencies and re-write it." On asking first: "99% of the time folks are just going to agree so just write". So the orchestrator adds depends_on between overlapping independent specs and rewrites the epic itself, reporting it.
- Proposed (not explicitly confirmed): the earlier-listed spec goes first; no automatic re-plan.
- User: "Yes create the spec, I will action it before re-running".
- Interview answers: scope also includes wider "genuine question" (contradicting recorded user decisions or knowledge entries → QUESTION mid-run) and cross-plan decisions (proposed single answer applied to every affected plan). Added dependency order: epic list order. Docs: update the website Epics page.
- Overview confirmed.
- Requirements confirmed (14).
- Acceptance criteria confirmed (14).
- Constraints confirmed (4). User: epics design may change if beneficial → Technical Approach, not a constraint (design doc must be updated to match).
- Technical approach confirmed.
- Success metrics confirmed.
- Non-goals confirmed.
- Verification: applied reviewer fixes; user chose new contradiction questions apply to ALL planning (single-spec unchanged otherwise).
- Spec committed to store; work dir removed.
- Split offered (3 specs), user declined: keep as one spec.
- User approved workflow auto-commits for the rest of this session.
