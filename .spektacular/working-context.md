# Working context — 000062_epic-plan-and-implement

## Origin
- User's request (exact phrasing): "We need to create a new spec that enables epics to be planned and implemented as a single command".
- The previous in-progress spec workflow (000061_version-check-upgrades-install, stopped at overview) was discarded with `--force` at the user's choice; its uncommitted draft files remain in the working tree.
- User chose NOT to commit existing uncommitted changes before this workflow started (`commit_existing: false`).
- The project currently has no epics (`spektacular epic list` is empty), so this spec does not belong to an epic.

## Decisions / answers
- Spec name chosen: `epic-plan-and-implement` → minted as `000062_epic-plan-and-implement`.
- Interview answers: delivered as skill(s) driving "plan this epic" then "implement this epic"; fewer interruptions than per-spec interactive runs; only do what's not already done (skip planned/implemented specs); docs repo must be updated.
- Unanswered: extend spek-plan/spek-implement vs new epic skills (left to technical approach). Stop-on-failure is an assumption to confirm.
- Overview confirmed as drafted.
- Requirements confirmed (12 items), incl. stop-on-failure, end-of-planning review summary, and implement refusing/reporting unplanned specs.
- Acceptance criteria confirmed (12, one per requirement).
- Constraints confirmed; design ref added to design:epics-and-seeded-specs.md.
- Technical approach confirmed; no design doc offered (nothing worked/settled enough).
- Success metrics confirmed.
- Non-goals confirmed (5).
- Verification: reviewer found 9 issues; all fixes applied with user's OK (implement refuses up front on unplanned specs; added planning-failure, mid-spec resume, broken-dependency requirements; metrics replaced; 2 non-goals trimmed). Spec written to store.
- Post-verification amendment (user): planning strictly in dependency order (one plan may influence another); parallel subagents using the standard plan/implement skills; planning in parallel in one repo; implementation in git worktrees merged back at every junction; conflicts reported not silently resolved; on failure let running finish, start none; subagent open questions relayed via main agent. Split offer still pending (re-offer after amendment).
- Split offered twice (before and after amendment); user declined: keep as one spec.
