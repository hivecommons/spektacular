---
created_date: "2026-09-15"
status: completed
closed_date: "2026-09-15"
---

# Test Plan: 000051_working-context-and-plan-reading

The contract tests in `cmd/instruction_contract_test.go`, `cmd/resume_test.go` and `internal/steps/implement/steps_test.go` cover the instruction-level half of the first success metric. Both procedures below cover what only real agent runs can show.

## Metric 1: A resumed implementation continues from the plan

**What to measure**: An implementation interrupted after finishing some phases and resumed in a fresh session carries on into the next unfinished phase, using that phase's detail from the plan. It must not report the plan as missing detail it actually contains.

**How**:
1. In a Spektacular project (the demo repo that showed the original failure is ideal), install the new build (`spektacular init claude`) and pick an approved plan with at least three phases whose `context.md` has a `### Phase N.M:` section for each.
2. Start `/spek-implement` and let it complete at least one phase (through `update_changelog`), then let it get partway into the next phase (for example into `implement` or `test`).
3. End the session. Seed `.spektacular/working-context.md` with notes that cover **only** the phases already done, which reproduces the original trap.
4. Open a **fresh** agent session and run `/spek-implement` for the same plan.
5. Check the resume report the CLI returns (`spektacular implement new --data '{"name":"<plan>"}'`). It must come from `resume_implement.md`, with four items in this order: read the plan documents, read the working context, find the first unchecked phase, run `implement goto`.
6. Choose to resume and watch what the agent does next.

**Expected result** (pass only if all hold):
- Before any `implement goto`, the agent runs `spektacular plan file read <plan>/plan.md`, `.../context.md` and `.../research.md`.
- It names the current phase as the first unchecked `#### - [ ] Phase` in `plan.md`, not the phase the working-context notes describe.
- On landing at `implement`, `test` or `verify`, it runs `plan file read <plan>/context.md` and works from that phase's section.
- Zero reports that the plan's `context.md` "has no section" for a phase that has one, and zero offers to derive missing detail from the codebase.

**Who / when**: The feature owner, once before this change is released, repeated for resume points at `analyze`, `implement` and `verify`.

## Metric 2: No plan/reality mismatches caused by reading the working context

**What to measure**: Across the first five implementation runs resumed in a fresh session after release, zero report a plan/reality mismatch that traces back to the agent reading `.spektacular/working-context.md` in place of the plan.

**How**:
1. For each of the first five real resumed implement runs after release, keep the agent transcript. Harbor job transcripts under `tests/harbor/jobs/*/agent/` or the agent's session log both work.
2. Search each transcript for mismatch reports: `grep -niE "plan/reality mismatch|no Phase [0-9.]+ section|missing .*context.md" <transcript>`.
3. For each hit, check whether the agent read `.spektacular/working-context.md` before (or instead of) `plan file read <plan>/context.md` for the phase it complained about.

**Expected result**: 0 of 5 runs has a mismatch report traced to the working context. Any count above 0 fails.

**Who / when**: The maintainers, tracked from release until five resumed runs have been collected.

## Outstanding verification (not a success metric)

The harbor suites were not run during implementation because the Docker daemon was unavailable. Before merging, run them from the spektacular repo root with Docker running and agent credentials set:

```
make harbor-test-plan
make harbor-test-spec
```

**Expected result**: Both print passing verifier results under `=== Test Results ===`. If an oracle encodes the old `.spektacular/context.md` path or the old footer placement, update that oracle in the same change. Once both pass, tick the last acceptance criterion of Phase 2.3.
