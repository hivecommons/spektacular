---
created_date: "2026-09-15"
status: completed
closed_date: "2026-09-15"
---

# Research: 000051_working-context-and-plan-reading

## Alternatives considered and rejected

- **Keep the footer copied into every step template, only rename the path inside it.** Satisfies the rename but leaves 45 identical copies (`grep -rh "Before you advance" templates/steps | uniq -c` → 45, all byte-identical, all the last paragraph of their file). Rejected: the spec's Technical Approach mandates a renderer-injected footer so a future wording change is one edit, and the user chose it in the interview (working context, "Footer: inject it once from the step renderer").
- **Inject the footer by editing each workflow's `writeStep` wrapper** (`internal/steps/{spec,plan,implement,repo}/steps.go`). Rejected: four copies of the same append; `stepkit.WriteStepResult` (`internal/stepkit/stepkit.go:55`) is the single funnel all four wrappers already call, and it already knows `req.NextStep` (empty only for terminal `finished` steps: `internal/steps/plan/steps.go:333`, `spec/steps.go` finished, `implement/steps.go` finished, `repo/steps.go:185`).
- **Plan documents defined only in the skill, with the CLI resume report pointing at the skill section by name.** Literal reading of Technical Approach bullet 1. Rejected as the sole mechanism: the resume acceptance criteria are phrased per interrupted step ("for an implementation interrupted at each of its steps in turn"), which is the CLI-rendered report (`cmd/resume.go:22 resumeInstruction`, parameterised by `current_step`), and "Start and resume describe the same plan documents ... with the same description of each" cannot hold if the report only points. Also the skill is install-time rendered (`internal/agent/skills.go:48`) and cannot be read by the CLI at runtime.
- **Restate the plan-documents block by hand in the skill, `01-read_plan.md`, and the resume report.** Rejected: three hand copies drift; contradicts "define once".
- **Record the current phase number in workflow state on resume/analyze.** Rejected by the spec (Technical Approach bullet 3) and the user ("check the plan for the next step"); `state.json` only holds step names today.
- **Migrate `.spektacular/context.md` to the new name on first touch, or fail with a fix instruction.** Rejected by the user (spec Constraints bullet 2): ignore the old file.
- **Mechanical check that each `*Technical detail:*` link resolves.** Rejected in the spec discussion and listed as a Non-Goal.
- **Mustache conditional inside the shared `steps/resume.md` for the implement kind** vs **a separate implement resume template**. Both viable; see Chosen approach evidence. A conditional keeps one resume file but mixes implement-only procedure into the shared text that spec/plan/repo must keep unchanged (Non-Goal 1).

## Chosen approach — evidence

- `internal/workingcontext/workingcontext.go:24` — `RelPath = ".spektacular/context.md"` is the only Go definition of the path; `Path` and `Reset` derive from it. Only caller: `internal/steps/spec/steps.go:94` (`spec new` reset). Renaming the constant renames every Go use.
- `internal/stepkit/stepkit.go:55-95` — `WriteStepResult` renders every workflow step (all four `writeStep` wrappers call it; `grep WriteStepResult`). Appending a rendered footer after `RenderTemplate` when `req.NextStep != ""` covers every continuing step in one place.
- `templates/steps/**` — 45 templates end with the identical `---` + **Before you advance** paragraph; removing those lines from each and moving the paragraph to one template (e.g. `templates/steps/context_footer.md`) is mechanical.
- `templates/steps/spec/00-new.md` — only non-terminal step without the footer (exempted in `templates/context_directive_test.go:39`); it also carries the `{{command}}` render bug at line 20 (steps render `{{config.command}}`, `internal/stepkit/stepkit.go:83`). With renderer injection it would receive the footer too; the exemption goes away.
- `github.com/cbroglie/mustache v1.4.0` supports partials: `RenderPartials(data, PartialProvider, ctx)` (`mustache.go:866`), `PartialProvider` interface (`partials.go:12`), `StaticProvider` (`partials.go:74`). A provider backed by `templates.FS` lets one plan-documents partial be included by the implement skill (install render, `internal/agent/skills.go:48`), by `01-read_plan.md` (stepkit render) and by the implement resume instruction (`cmd/resume.go:22`).
- `cmd/resume.go:22-30` — resume template chosen in one function with `kind` in hand; selecting an implement-specific template (or passing an `implement` flag) is a one-line branch.
- `templates/steps/implement/02-analyze.md:7-13` — already defines "current phase = first unchecked `#### - [ ] Phase N.M:`" and reads the phase's section via `plan file read {{plan_name}}/context.md`. `03-implement.md`, `04-test.md`, `05-verify.md` reuse this wording.
- `templates/steps/implement/03-implement.md:3` — "You have the analysis summaries from the previous step" (must go). `04-test.md:17`, `05-verify.md:11-12` point at raw `{{plan_path}}`/`{{context_path}}` instead of `plan file read`.
- `internal/steps/implement/strategy.go:52-66` — `context_path`, `plan_name` already available to implement templates.
- `templates/context_directive_test.go` — existing guard; becomes a rendered-output test (footer present exactly once at the end for continuing steps, absent for finished).

## Files examined

- `internal/workingcontext/workingcontext.go:24` — `RelPath` constant; package doc names the old path.
- `internal/workingcontext/workingcontext_test.go:12,39` — asserts `Path` joins `.spektacular/context.md`.
- `internal/steps/spec/steps.go:67,90-95` — `new` resets working context; comments/error text say `context.md`.
- `internal/steps/spec/steps_test.go:140,172,201,225,248,285-287` — fixtures and assertions on `.spektacular/context.md`.
- `internal/stepkit/stepkit.go:55-127` — single render funnel; `RenderTemplate` uses plain `mustache.Render`.
- `internal/stepkit/stepkit_test.go` — render tests using `steps/plan/01-overview.md`.
- `internal/steps/{plan,implement,repo,spec}/steps.go` — `writeStep` wrappers; `NextStep` is `""` only for `finished`.
- `internal/steps/implement/strategy.go:17-66` — path helpers and template vars.
- `internal/steps/implement/steps_test.go:238-247` — read_plan asserts `plan file read`, `context.md`, `research.md`.
- `internal/steps/plan/steps_test.go:145,151,655` — assert bare `context.md` in plan step output (will need qualified wording).
- `cmd/resume.go:13-45` — `resumeInstruction`, `mismatchInstruction`.
- `cmd/resume_test.go:68,124` — assert `.spektacular/context.md` in resume output for every kind.
- `templates/steps/resume.md:12` — shared resume: work files, working context, `repo list`, goto.
- `templates/steps/resume_mismatch.md` — cross-kind report; no working-context mention.
- `templates/steps/implement/01-read_plan.md:9-19,38-40` — lists the three docs with commands; bare `context.md` in link-format line.
- `templates/steps/implement/02-analyze.md:7,13,15,27` — bare "context.md" mentions.
- `templates/steps/implement/03-implement.md`, `04-test.md`, `05-verify.md` — no phase-detail fetch.
- `templates/steps/plan/{03,04,05,06,08,10,13,14,16,19}-*.md` — bare `context.md` mentions meaning the plan's document (need "the plan's `context.md`").
- `templates/steps/plan/13-assemble.md:30,35` — references `.spektacular/context.md` notes (rename).
- `templates/steps/spec/00-new.md:8,20`, `00b-interview.md:7`, `08-verification.md:34` — working-context mentions; `{{command}}` bug.
- `templates/skills/workflows/spek-implement/SKILL.md:31,58` — plan documents list (no descriptions); resume reads only working context.
- `templates/skills/workflows/{spek-new,spek-plan,spek-manage-repos}/SKILL.md` — working-context mentions at 47/89, 47/66, 74.
- `templates/skills/skill_verify-implementation.md:15` — bare "`context.md`" meaning the plan's.
- `templates/agents/historical-artifacts.md:58,62` — "`context.md`" meaning the working-context file (rename); installed into AGENTS.md.
- `templates/context_directive_test.go` — footer marker guard on template files; floor 30.
- `templates/skill_resume_test.go:38` — all four skills must contain `.spektacular/context.md`.
- `templates/guided_add_skill_test.go:139` — manage-repos skill must contain `.spektacular/context.md`.
- `templates/guided_add_conversation_test.go:318-357` — em-dash check on repo step templates excludes the footer line and expects 8 footers in the files.
- `internal/agent/instruction_surface_test.go` — pattern for walking `templates.FS` and rendered skills into `t.TempDir()`.
- `internal/agent/skills.go:30-60` — skills rendered with `mustache.Render` and only `command`.
- `.claude/skills/*`, `.bob/skills/*`, `AGENTS.md` — tracked installed copies that contain the old name; regenerate with `go run . init claude` / `init bob`.
- `docs:src/content/tutorials/unknown-criteria.mdx:54` — quotes the footer verbatim with the old path.
- `docs:src/pages/index.mdx:38`, `docs:src/pages/how-it-works.mdx:298`, `docs:src/content/tutorials/getting-started.mdx:832` — refer to the plan's `context.md`; unchanged.
- `README.md:23,84` — plan's `context.md`; unchanged.
- `tests/harbor/**` — no reference to `.spektacular/context.md` or the footer; the plan suite's `context.md` references are the plan document.

## External references

- cbroglie/mustache v1.4.0 source (`~/go/pkg/mod/github.com/cbroglie/mustache@v1.4.0/partials.go`, `mustache.go:866`) — confirms partial support and the provider interface, needed for a single plan-documents definition rendered at both install time and runtime.
- Local experiment (planning, scratchpad) with cbroglie/mustache v1.4.0: a three-space-indented standalone `{{> partial}}` indents every non-blank line of the partial by three spaces. This is why cross-surface partial comparisons normalise indentation.

## Prior plans / specs consulted

- `000024_resume` plan (archaeology) — introduced `kind` in state, the shared `steps/resume.md`, re-render on `goto <current_step>`, and the agent-owned `.spektacular/context.md` refreshed by a uniform per-step directive. Explains why the footer was copied into each template and why resume re-emits the interrupted step.
- `000051_working-context-and-plan-reading` spec — source of truth for scope.
- The spec workflow's working context for 000051 — root cause of the demo-repo failure (agent read the working context, took it for the plan's `context.md`).

## Open assumptions

- The "resume instructions for an implementation" in the acceptance criteria are both the CLI-rendered resume report for kind `implement` and the spek-implement skill's "To resume" path; both must carry the same additive items.
- "No other procedural steps" applies to the resume path itself. The surrounding report framing (ask the user resume vs start new; the `--force` discard alternative) stays, as it is the choice presented before resuming, not a resume step. The `repo list` step is dropped from the implement resume path: the installed AGENTS.md "Where the Code Lives" section already requires it every session, and `read_plan` carries it for fresh starts.
- A `context.md` occurrence inside a longer file name (`phases_context.md`, `context_template.md`) is not an occurrence of the plan's `context.md` for the qualification rule; the guard test matches `context.md` only when not preceded by a word character or `_`.
- `scaffold/context.md` (the plan scaffold) keeps its name and content (Constraint 3).
- The docs repo's no-em-dash convention does not apply to the verbatim-quoted footer in `unknown-criteria.mdx`; only the path changes there.
- The docs repo currently has an unrelated uncommitted change in `src/pages/knowledge-base.mdx`; the docs edit must not touch or commit it.
- Dogfooding hazard: this repo runs the CLI with `go run .`, so once the rename phase lands mid-implementation, later steps of the same implement run will emit `.spektacular/working-context.md`. The implementing agent must carry the current `.spektacular/context.md` content across by hand for that run (the product deliberately ignores the old file).

## Drafting assumptions

### Scope of "resume instructions" (discovery)
- **Decision**: Treat both the CLI resume report for kind `implement` and the spek-implement skill's "To resume" path as resume instructions that must carry the additive items.
- **Rationale**: The acceptance criterion "interrupted at each of its steps in turn" only makes sense for the CLI report; the failure happened while the agent followed the skill.
- **Rejected**: Skill only (misses the per-step report); report only (skill would still say read working context then goto).

### Resume-vs-new framing and repo list (discovery)
- **Decision**: Keep the ask-resume-or-new framing and `--force` alternative; drop `repo list` from the implement resume path.
- **Rationale**: "No other procedural steps" targets the resume path; the choice framing precedes it. AGENTS.md already mandates `repo list` every session.
- **Rejected**: Removing the choice framing (would let an agent resume without asking the user).

### Qualification rule ignores compound file names (discovery)
- **Decision**: `phases_context.md` and `context_template.md` are not bare `context.md` occurrences.
- **Rationale**: They are distinct working/scratch file names that cannot be confused with the plan document.
- **Rejected**: Renaming those files (out of scope, no ambiguity).

### Chosen direction: single-source prose via renderer (architecture)
- **Decision**: (1) rename `workingcontext.RelPath` to `.spektacular/working-context.md`; (2) footer moved to `templates/partials/working-context-footer.md`, appended by `stepkit.WriteStepResult` when `NextStep != ""`; (3) plan documents defined once in mustache partial `templates/partials/implement-plan-documents.md`, included by the spek-implement skill, `01-read_plan.md`, and a new `steps/resume_implement.md`; both renderers use `mustache.RenderPartials` with an FS-backed provider; (4) `resumeInstruction` selects `resume_implement.md` for kind `implement`; `03-implement`, `04-test`, `05-verify` fetch the phase section themselves.
- **Key design decisions**: the partial uses the `{{command}}` spelling and `<plan_name>` placeholder so it renders identically at install time and runtime; stepkit exposes `command` alongside `config.command` to support it. `spec new` loses its footer exemption. The skill's resume path refers to the skill's plan-documents section rather than including it a second time.
- **Rationale**: One edit point per piece of prose; start/resume wording identical by construction; matches Technical Approach bullets 1-2 while satisfying the per-step resume criteria.
- **Rejected**: Option B, skill-only definition with CLI report pointing (fails "same description", CLI cannot read the skill); Option C, plan-documents text as a Go string var injected into templates (moves prose into Go, against the template-contract testing architecture); per-workflow footer append in each `writeStep` (four copies); mustache conditional in shared `resume.md` (mixes implement-only procedure into the shared text that Non-Goal 1 keeps unchanged).

### `command` alias in runtime step vars (architecture)
- **Decision**: Add `command` (= `cfg.Command`) to stepkit's standard vars and to the resume render context, in addition to `config.command`.
- **Rationale**: Lets one partial serve install-time skills (`{{command}}`) and runtime templates without a second copy. Step templates keep writing `{{config.command}}`; `00-new.md` is still corrected to that spelling for consistency.
- **Rejected**: Partial in `{{config.command}}` spelling with the installer passing `config.command` (puts two spellings into rendered skills' source family and breaks the skill-file placeholder convention); partial without a command prefix (weaker instruction, and the AC asks for the command to read each document).

### Convention selection (architecture)
- **Decision**: Apply `tests-must-pass-for-done`, docs `plan-content-pages`, docs `mdx-authoring` Rule 4, docs `no-em-dashes` (authored prose only). Drop `error-messages-must-suggest-remediation` (no new user-facing CLI error paths), `alternate-section-background`, `file-scoped-section-headings`, `site-layout` (no page or section structure changes).
- **Rationale**: Only these bear on the surfaces touched.
- **Rejected**: Listing all conventions (noise).

### Em dashes in the quoted tutorial footer (architecture)
- **Decision**: Keep the footer's existing wording (with its em dashes) in the new partial and in the verbatim tutorial quote; only the path changes.
- **Rationale**: The spec asks for the new name, not a rewording; the tutorial quotes emitted output verbatim.
- **Rejected**: Rewriting the footer without em dashes (unrequested wording change to every instruction).

### Fragments directory rather than per-surface copies (components)
- **Decision**: Introduce one new component, shared template fragments, rather than reusing an existing template location.
- **Rationale**: No existing directory holds cross-surface prose; `templates/steps/` fragments would be walked by step-template guard tests as if they were steps, and `templates/agents/` is for managed AGENTS.md sections.
- **Rejected**: Putting the footer at `templates/steps/context_footer.md` (collides with the step-directory walks and the resume templates that live there).

### Missing partial is an error (data_structures)
- **Decision**: `FSPartials.Get` returns an error when `<name>.md` is absent, unlike mustache's providers, which render empty.
- **Rationale**: A typo in an include would otherwise silently drop the plan-documents block from the skill or resume instruction, which is the exact failure this feature fixes.
- **Rejected**: `mustache.StaticProvider` or `FileProvider` (silent empty on missing; FileProvider reads the OS filesystem, not the embedded FS).

### `command` also added to the mismatch renderer (data_structures)
- **Decision**: Pass `command` in `mismatchInstruction` context as well.
- **Rationale**: Keeps both resume renderers' contexts identical, so a future fragment include in either works without surprise. Costs one map entry.
- **Rejected**: Only the implement resume path (asymmetric contexts invite a render bug later).

### Success metric 1 split into automated + manual (testing_approach)
- **Decision**: Cover the instruction guarantees with contract tests, and classify real-agent resume behaviour as manual.
- **Rationale**: Only a model run proves an agent follows the prose. The harbor suites do not cover an interrupted implement run and do not run in CI.
- **Rejected**: A new harbor implement-resume suite (substantial new E2E infrastructure beyond the spec's scope).

### Qualification guard is line-based (testing_approach)
- **Decision**: A `context.md` mention counts as qualified when its line contains "plan's" or a plan-name path (`{{plan_name}}/`, `<plan_name>/`, `<name>/`, or the concrete plan name).
- **Rationale**: Gives a deterministic, hand-maintainable oracle for "qualified in words or by path"; the templates are line-oriented prose.
- **Rejected**: Natural-language parsing, or allow-listing each occurrence (brittle and tautological).

### Phase ordering: footer before rename (phases)
- **Decision**: Move the footer into the renderer (1.1) before renaming the working context (1.2).
- **Rationale**: After 1.1 the rename touches one footer file instead of 45 copies, and the rendered-output harness built in 1.1 is reused by every later guard.
- **Rejected**: Rename first (45 extra edits that 1.1 then deletes).

### Contract harness lives in `cmd` with a hand-written step table (phases)
- **Decision**: Cross-surface guards go in `cmd/instruction_contract_test.go`, rendering step templates through `stepkit.WriteStepResult` from a hand-written `{template, nextStep}` table, with a completeness check against the template directory.
- **Rationale**: `cmd` can reach stepkit, the resume renderers and the agent installer without import cycles (a `package templates` test importing stepkit would cycle). Driving real callbacks would trip store-validation side effects in plan write steps. The hand-written table follows the independent-oracle rule.
- **Rejected**: Deriving next steps from `Steps()` at runtime (tautological); running full workflows through `rootCmd` (slow, store side effects).

### Next-command guard matches only `goto ... --data` (phases)
- **Decision**: The prefix guard treats `<kind> goto` followed by ` --data` as a next command.
- **Rationale**: The skills legitimately name subcommands in prose ("after calling `plan goto`"); only runnable commands must carry the prefix.
- **Rejected**: Flagging every `plan goto` substring (false positives in the skills' STOP banners).

### Delete `templates/context_directive_test.go` (phases)
- **Decision**: Replace it with the rendered-output footer test rather than keep both.
- **Rationale**: Its template-file walk would now assert the opposite of the contract. Keeping a second footer guard would be a redundant assertion.
- **Rejected**: Inverting it in place (the rendered test already covers "no template carries the footer").

### Partial comparisons normalise indentation (open_questions)
- **Decision**: Tests compare the plan-documents block across surfaces after trimming leading whitespace per line.
- **Rationale**: Experiment showed an indented include indents the partial's lines, so the resume instruction's copy differs from the skill's only in indentation.
- **Rejected**: Forcing the include to column 0 in the resume list (breaks the numbered-list structure).

### Resume-report JSON description mismatch left alone (out_of_scope)
- **Decision**: Do not change the skills' `"resumable": true` wording even though `cmd/resume.go` emits a `workflow_in_progress` error envelope.
- **Rationale**: Unrelated to the spec's failure mode and not requested. Changing it would widen scope, and the existing `templates/skill_resume_test.go` asserts the phrase.
- **Rejected**: Folding the fix into Phase 2.2.

## Rehydration cues

- `go run . repo list` — roots for `spektacular` and `docs`.
- `go run . spec file read 000051_working-context-and-plan-reading.md`
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'` — template-contract tests and harbor couplings.
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/workflow-steps.md"}'`
- Re-read: `internal/stepkit/stepkit.go`, `internal/workingcontext/workingcontext.go`, `cmd/resume.go`, `templates/steps/resume.md`, `templates/steps/implement/0[1-5]-*.md`, `templates/skills/workflows/spek-implement/SKILL.md`, `templates/context_directive_test.go`.
- `grep -rn "context\.md" templates --include='*.md'` — full list of occurrences to rename or qualify.
- `grep -rh "Before you advance" templates/steps | sort | uniq -c` — footer copies.
