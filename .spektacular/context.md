# Working context — 000051_working-context-and-plan-reading

## The problem

An implement workflow was resumed in the user's demo repo and failed at Phase 2.1.
The agent reported that the plan's `context.md` "has no Phase 2.1 section — only
Phases 1.1–1.5 were detailed when the plan was written", called it a plan/reality
mismatch, and proposed deriving the missing detail from the codebase.

**The plan was not incomplete.** The user confirmed the plan's `context.md` does
contain Phase 2.1. The agent never read the plan on resume. It read
`.spektacular/context.md` — the working-context file the previous session had
filled with per-phase notes for the phases it had reached (1.1–1.5) — and took
that for the plan's technical detail.

Two causes combined:

1. **Resume never tells the agent to read the plan.** The implement skill's resume
   path (`templates/skills/workflows/spek-implement/SKILL.md:58`) says read
   `.spektacular/context.md`, then `goto` the current step. The shared resume
   template (`templates/steps/resume.md`) says the same plus `repo list`. Neither
   mentions `plan.md`, the plan's `context.md`, or `research.md`. Whether the plan
   gets read depends on which step the resume lands on: `read_plan` and `analyze`
   read it; `03-implement.md` reads nothing and assumes "you have the analysis
   summaries from the previous step"; `04-test.md` and `05-verify.md` point at a
   raw `{{plan_path}}` rather than `plan file read`. `state.json` records step
   names, not the phase number, so the current phase is also lost.
2. **Two different files are both called `context.md`.**
   `.spektacular/context.md` (working context, `internal/workingcontext/workingcontext.go:24`,
   `RelPath`) and `<plan>/context.md` (the plan's per-phase technical detail,
   reachable only through `plan file read`). Every implement step template from
   `read_plan` to `reconcile_spec` mentions both. `02-analyze.md` gives the explicit
   `plan file read <plan>/context.md` command but also says bare "context.md" at
   lines 7, 15 and 27, and its footer says refresh `.spektacular/context.md`.

## What the user asked for (their words, preserve these)

- "I think having working context is good, this should be used for all phases,
  this can be confusing with context in the plan directory." → keep the concept,
  rename the file so it cannot be confused with the plan's `context.md`; name
  agreed as `working-context`.
- "We also need to make it very clear that in implement resume it says read the
  f-ing plan" → resume must instruct reading the plan, unmissably.
- "in fact the standard start implement should have instructions on what files to
  read" → the normal implement start defines which plan documents to read.
- "Resume should only add the implementation is in process, read the
  working-context and check the plan for the next step" → resume is minimal and
  additive on top of the start instructions, not a separate parallel definition.

## Scope as agreed

1. Rename `.spektacular/context.md` → `.spektacular/working-context.md`, used by
   every workflow (spec, plan, implement, repo add). Reach:
   `workingcontext.RelPath`, 45 step templates carrying an identical "Before you
   advance" footer, four workflow skills, `templates/steps/resume.md`, guard tests
   (`templates/context_directive_test.go`, `templates/skill_resume_test.go`,
   `templates/guided_add_skill_test.go`, `cmd/resume_test.go`,
   `internal/steps/spec/steps_test.go`), `internal/steps/spec/steps.go:94`
   (`workingcontext.Reset`), and the regenerated `.claude/` and `.bob/` copies.
2. Implement start names the plan documents explicitly — `plan.md`, the plan's
   `context.md`, `research.md` — each via `plan file read`, in one block that both
   start and resume point at.
3. Implement resume only adds: an implement run is in progress; read the plan
   documents (that same block); read `working-context.md`; find the next unchecked
   phase in `plan.md`; then `goto` the current step.
4. Templates always say "the plan's `context.md`", never bare "context.md". The
   mid-phase steps (`03-implement`, `04-test`, `05-verify`) fetch the current
   phase's detail themselves via `plan file read`.

## Interview answers (user decisions, 2026-09-15)

- **Existing `.spektacular/context.md`: ignore it.** Not migrated, not refused. The
  user chose this over "move on first touch" (recommended) and "fail with a fix
  instruction". Accepted consequence: a run interrupted before upgrading resumes
  without its working context.
- **Footer: inject it once from the step renderer.** In scope. Replaces 45 copies.
- **Docs repo: update `src/content/tutorials/unknown-criteria.mdx:54`**, which
  quotes the footer verbatim. The site's other `context.md` mentions are about the
  plan's file and stay.
- **Render bug in `templates/steps/spec/00-new.md:20`: include.** It uses
  `{{command}}`; step templates are rendered with `{{config.command}}`. Confirmed
  by reading the template.
- The current phase is recovered from the plan's first unchecked phase, following
  the user's "check the plan for the next step"; recording it in workflow state
  is not part of this.

## Rejected during the discussion

- A mechanical check that every `*Technical detail:*` link resolves to a heading,
  run at plan finish and `implement new`. Proposed on a misreading of the
  screenshot (assumed the plan was incomplete). It does not address this failure
  and was dropped.

## Noticed while starting this spec

- Starting this spec cleared `.spektacular/context.md`, which had held the 000050
  implement run's working context (already committed in `0b57a7c`). That is
  `workingcontext.Reset` doing its job.

## Spec committed (2026-09-15)

Fresh-eyes review returned 19 findings; 18 applied, 1 partly set aside (the
reviewer suggested moving the command-prefix bug to its own spec; the user had
chosen to include it, so only its wording fix was kept). User approved the
revised spec as shown. The "first five resumed runs" success-metric count was
drafted by the assistant and accepted by the user without change.

Stored spec: 12 requirements, 12 acceptance criteria, 3 constraints, 5 technical
approach bullets, 2 success metrics, 2 non-goals. Mechanism (single definition,
renderer-injected footer, first-unchecked-phase rule) lives only in Technical
Approach. Next step for this work is `spek-plan` against
`000051_working-context-and-plan-reading`.

## Plan workflow started (2026-09-15)

User picked spec `000051_working-context-and-plan-reading` for `spek-plan`.
Plan name matches the spec name. Spec read in the overview step.

## Discovery learnings (plan workflow)

- Target repos: `spektacular` (all code/templates/tests) and `docs` (one line in `src/content/tutorials/unknown-criteria.mdx:54`). Docs repo has an unrelated uncommitted change in `src/pages/knowledge-base.mdx`: leave it alone.
- `stepkit.WriteStepResult` is the single render funnel; `NextStep == ""` only for `finished` steps, so it is the natural footer-injection point.
- All 45 footer copies are byte-identical and the last paragraph of their file.
- cbroglie/mustache v1.4.0 supports partials (`RenderPartials` + `PartialProvider`), which enables one plan-documents block shared by the skill (install time), `read_plan`, and the implement resume report (runtime).
- Dogfooding hazard: implementing the rename with `go run .` changes the emitted working-context path mid-run; carry context across by hand.
- Judgement calls logged in `.spektacular/work/000051_working-context-and-plan-reading/assumptions.md`.

## Architecture decisions (plan workflow)

- Chosen: renderer-owned single sources. Footer fragment + mustache partial for plan documents under new `templates/partials/`; `stepkit.WriteStepResult` appends footer when `NextStep != ""`; `RenderPartials` with FS provider in stepkit and skill installer; stepkit exposes `command` alias; new `steps/resume_implement.md` selected by `resumeInstruction` for kind implement.
- Resume path order: in progress → read plan documents → read working context → first unchecked phase → goto. Resume-vs-new choice and `--force` stay as framing.

## Phases drafted (plan workflow)

- M1: 1.1 footer via renderer + partial support + prefix guard (spektacular); 1.2 rename + regenerate installed copies (spektacular); 1.3 tutorial line (docs).
- M2: 2.1 plan-documents partial in skill + read_plan (installer gains RenderPartials); 2.2 `resume_implement.md` + skill resume path; 2.3 phase steps fetch detail + qualify every `context.md` + guard.
- Cross-surface guards live in `cmd/instruction_contract_test.go` (hand-written step table). Qualification regex must exclude `working-context.md` (preceded by `-`).

## Assembled (plan workflow)

- Staged `.spektacular/tmp/{plan,context,research}_template.md` from the working files; metadata commit 0b57a7c on `f-knowledge-search`.
- Verification passed: all sections filled, 6/6 phases have Repo lines, technical-detail anchors resolve, removed the one shell command from plan.md Conventions.
- plan.md committed to store.
- context.md committed to store.
- research.md committed; working dir removed. Next: walkthrough with user.
- Walkthrough done; user signed off explicitly ("yeah it looks fine, commit it") with no changes. User asked to commit.
