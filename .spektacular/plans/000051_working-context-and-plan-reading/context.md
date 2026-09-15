---
created_date: "2026-09-15"
status: completed
closed_date: "2026-09-15"
---

# Context: 000051_working-context-and-plan-reading

## Current State Analysis

- **Working context.** `internal/workingcontext/workingcontext.go:24` defines `RelPath = ".spektacular/context.md"`. Its only runtime writer is the `spec new` reset (`internal/steps/spec/steps.go:94`). The name collides with each plan's `context.md` (`internal/steps/implement/strategy.go:19-21`, `internal/steps/plan/steps.go:21`), which is reachable only through `plan file read`.
- **Footer.** 45 step templates under `templates/steps/{spec,plan,implement,repo}/` end with a byte-identical `---` + "**Before you advance:** refresh `.spektacular/context.md` …" paragraph. `templates/steps/spec/00-new.md` is the only continuing step without it, exempted in `templates/context_directive_test.go:39`. Terminal `*-finished.md` steps have none.
- **Renderer.** `internal/stepkit/stepkit.go:55-95` (`WriteStepResult`) is the single render funnel for all four workflows. It renders with plain `mustache.Render` (`:121-127`) and exposes only `config.command`. The skill installer (`internal/agent/skills.go:48`) renders with plain `mustache.Render` and only `command`. `templates/steps/spec/00-new.md:20` uses `{{command}}`, which renders empty at runtime.
- **Resume.** `cmd/resume.go:22` renders the shared `templates/steps/resume.md` for every kind: read per-section work files (spec/plan), read `.spektacular/context.md`, run `repo list`, then `goto <current_step>`. Nothing tells an implement resume to read the plan. `templates/skills/workflows/spek-implement/SKILL.md:58` says the same (read the working context, then goto).
- **Implement steps.** `01-read_plan.md:9-19` reads all three plan documents. `02-analyze.md:7-13` identifies the current phase (first unchecked) and reads its section. `03-implement.md:3` assumes "the analysis summaries from the previous step" are in context. `04-test.md:18` and `05-verify.md:15-16` point at raw `{{plan_path}}`/`{{context_path}}` and never run `plan file read`. `state.json` stores step names, not the phase.
- **Bare `context.md` mentions.** The implement `02-analyze.md:7,13,15,27` and `01-read_plan.md:38`, many plan step templates (`03`, `04`, `05`, `06`, `08`, `10`, `13`, `14`, `16`), the plan/implement skills, and `templates/skills/skill_verify-implementation.md:15` all mention `context.md` without saying whose it is.
- **Tests asserting the old state.** `templates/context_directive_test.go`, `templates/skill_resume_test.go:38`, `templates/guided_add_skill_test.go:139`, `templates/guided_add_conversation_test.go:318-357`, `cmd/resume_test.go:68,124`, `internal/workingcontext/workingcontext_test.go:12`, `internal/steps/spec/steps_test.go:201,248`.
- **Docs.** `docs:src/content/tutorials/unknown-criteria.mdx:54` quotes the footer verbatim.

## Per-Phase Technical Notes

### Phase 1.1: Write the standing footer once and render it for every continuing step

**Requirement → repo/files resolution**: "The keep-context-current instruction is identical in every step" and "Every next command is shown in full" → `spektacular`: `internal/stepkit/stepkit.go`, `templates/partials/`, `templates/steps/**`, guard tests.

**File changes**:

- `internal/stepkit/stepkit.go:121-127` (`RenderTemplate`): switch `mustache.Render` to `mustache.RenderPartials(string(tmplBytes), FSPartials{FS: templates.FS}, data)`. Add the exported type `FSPartials struct{ FS fs.FS }` with `Get(name string) (string, error)`. It reads `name + ".md"` from `FS` and returns a wrapped error (`fmt.Errorf("loading partial %s: %w", name, err)`) when the file is missing. Add a compile-time check `var _ mustache.PartialProvider = FSPartials{}`.
- `internal/stepkit/stepkit.go:78-83` (standard vars in `WriteStepResult`): add `"command": cfg.Command` next to `"config"`.
- `internal/stepkit/stepkit.go:87-90`: after `RenderTemplate`, when `req.NextStep != ""`, render `partials/working-context-footer.md` with the same `vars` and set `instruction = strings.TrimRight(instruction, "\n") + "\n\n---\n\n" + footer`. Update the `WriteStepResult` doc comment to say continuing steps get the footer appended.
- New `templates/partials/working-context-footer.md`: the current footer paragraph verbatim, with no leading `---` (the renderer adds the rule). The path stays `.spektacular/context.md` in this phase and is renamed in Phase 1.2.
- At the top of the footer fragment, add a mustache comment `{{! ... }}` saying `stepkit.WriteStepResult` appends it to every step with a next step. The comment renders to nothing. Add no README to `templates/partials/`.
- `templates/steps/{spec,plan,implement,repo}/*.md` (45 files; list with `grep -rl "Before you advance" templates/steps`): delete the trailing block (blank line, `---`, blank line, footer paragraph, trailing newline). Each file then ends with its own last line (normally the goto code fence) plus one newline. Every copy is byte-identical and is the file's last paragraph (checked in discovery), so a scripted removal is safe. Re-check afterwards that `grep -rc "Before you advance" templates/steps` shows zero everywhere.
- `templates/steps/spec/00-new.md:20`: `{{command}} spec goto` → `{{config.command}} spec goto`.
- `templates/context_directive_test.go`: delete it. Its template-file walk and `exemptFromContextDirective` no longer describe the contract. Its replacement is the rendered-output test below.
- `templates/guided_add_conversation_test.go:318-357` (`TestGuidedAddInstructionsUseNoEmDashes`): remove `contextDirectiveFooterPrefix`, the footer-skipping branch, `expectedFooters` and its assertion. The repo templates no longer contain the footer, so the whole file is authored prose. Keep the `checked` count.
- `internal/stepkit/stepkit_test.go`: add unit tests.
  - A step with `NextStep` set ends with `"\n\n---\n\n" + <hand-written footer const>`.
  - A step with `NextStep == ""` (render `steps/plan/19-finished.md`) does not contain the footer marker.
  - `FSPartials` over an `fstest.MapFS` resolves `partials/x` to `partials/x.md`, and a missing name returns an error.
  - `RenderTemplate` of a template that includes a missing partial returns an error.
  - `{{command}}` renders in a step template.
- New `cmd/instruction_contract_test.go` (package `cmd`, which can reach stepkit, all step packages, the resume renderers and the agent installer): add the shared harness `renderAllStepInstructions(t, command string) []renderedInstruction`.
  - It renders every step template through `stepkit.WriteStepResult` using a test `PathStrategy` that supplies realistic vars: `name`, `plan_name`, `spec_name` = `"demo-feature"`, and `plan_path`/`context_path`/`research_path`/`spec_path`/`changelog_path` containing `demo-feature`.
  - The next step for each template comes from a **hand-written table** per workflow (`{workflow, templatePath, nextStep}`), mirroring `internal/steps/*/steps.go`, with `""` for each `*-finished.md`.
  - Its completeness check walks `templates.FS` under `steps/<wf>/` and requires every `.md` to appear in the table, so a new step cannot slip past.
- `cmd/instruction_contract_test.go`: add `TestContinuingStepsEndWithIdenticalFooter` (every row with a next step ends with exactly one copy of the hand-written footer const, and every terminal row contains none).
- `cmd/instruction_contract_test.go`: add `TestNoTemplateFileCarriesFooter` (walk `steps/`, no file contains `**Before you advance:**`).
- `cmd/instruction_contract_test.go`: add `TestNextCommandsCarryPrefix`.
  - Use the non-default command `"spekx"`.
  - The corpus is all rendered step instructions, plus `resumeInstruction` and `mismatchInstruction` for each kind, plus every workflow skill installed via `agent.Get("claude").Install` into `t.TempDir()` with `cfg.Command = "spekx"`.
  - For every match of `\b(spec|plan|implement|repo) goto\b` inside a code span or fenced block, the preceding text on the same line must end with `spekx `.
  - Scope the check to code: the skills' prose mentions (e.g. "after calling `plan new` or `plan goto`", SKILL.md:12) name a subcommand rather than showing a command to run. Treat a match as a next command only when followed by ` --data`.
  - Also assert that no rendered output contains `{{`.

**Complexity**: Medium
**Token estimate**: ~35k
**Agent strategy**: Two parallel agents: (a) the stepkit renderer changes plus the unit tests; (b) the scripted footer removal across 45 templates plus the `00-new.md` fix. Then one agent integrates the `cmd` contract harness and tests, and deletes and updates the old guards. Run `go test ./...` at the end.

### Phase 1.2: Rename the working context to working-context everywhere

**Requirement → repo/files resolution**: "Session notes have a name of their own", "Every workflow keeps using the working context" → `spektacular`: `internal/workingcontext`, `internal/steps/spec`, `templates/partials`, `templates/steps`, `templates/skills/workflows/*`, `templates/agents/historical-artifacts.md`, installed copies.

**File changes**:

- `internal/workingcontext/workingcontext.go:1-2,24`: set `RelPath = ".spektacular/working-context.md"`, and update the package doc to name the new path. Add one sentence saying a file under the previous name `.spektacular/context.md` is deliberately ignored (not migrated, not an error).
- `internal/workingcontext/workingcontext_test.go:12`: expect `filepath.Join("/proj", ".spektacular", "working-context.md")`. Update the line-39 comment.
- `internal/steps/spec/steps.go:67,90-95`: change the comments and the error text `resetting context.md` to `resetting working context`. No logic change.
- `internal/steps/spec/steps_test.go:140,172,285-287`: update the comments. At `:201` (`TestNewStep_ResetsContextMd`, rename it to `TestNewStep_ResetsWorkingContext`), seed `.spektacular/working-context.md` and assert it is emptied. Also seed a `.spektacular/context.md` with `"legacy"` and assert it is **unchanged** afterwards (ignored, not migrated or deleted). Also assert no `.spektacular/context.md` is created when none existed. At `:248`, assert the instruction contains `.spektacular/working-context.md`.
- `templates/partials/working-context-footer.md`: change the path to `.spektacular/working-context.md`. Leave the rest of the wording as it is.
- Step template body mentions (the footers are already gone after Phase 1.1): `templates/steps/spec/00-new.md:8`, `templates/steps/spec/00b-interview.md:7`, `templates/steps/spec/08-verification.md:34`, `templates/steps/plan/13-assemble.md:30,35`. Replace `.spektacular/context.md` with `.spektacular/working-context.md`. Re-grep `grep -rn "spektacular/context\.md" templates` for any stragglers.
- `templates/steps/resume.md:12`: change to `.spektacular/working-context.md`.
- `templates/skills/workflows/spek-new/SKILL.md:31,47,89`, `templates/skills/workflows/spek-plan/SKILL.md:47,66`, `templates/skills/workflows/spek-manage-repos/SKILL.md:74`, `templates/skills/workflows/spek-implement/SKILL.md:58`: replace `.spektacular/context.md` with `.spektacular/working-context.md`. In `spek-plan/SKILL.md:47`, keep the clause distinguishing it from "the plan's own `context.md` document".
- `templates/agents/historical-artifacts.md:58,62`: replace "`context.md`" (meaning the sidecar) with "`working-context.md`" on both lines.
- `cmd/resume_test.go:68,124`: expect `.spektacular/working-context.md`.
- `templates/skill_resume_test.go:38` and `templates/guided_add_skill_test.go:139`: expect `.spektacular/working-context.md`.
- `internal/agent/historical_artifacts_test.go`: checked during planning, it has no assertion on the `context.md` line, so no change is needed.
- `cmd/instruction_contract_test.go`: add `TestNoEmittedInstructionNamesOldWorkingContext`. Over the Phase 1.1 corpus, plus the managed AGENTS.md sections rendered via the production install into `t.TempDir()`, plus `templates/skills/skill_*.md` rendered via the `skill` command path, assert no output contains `.spektacular/context.md`.
- `cmd/instruction_contract_test.go`: add `TestEachWorkflowNamesWorkingContext`. For each of spec, plan, implement and repo, at least one rendered step instruction and the installed skill contain `.spektacular/working-context.md`.
- Regenerate the tracked installed copies: `go run . init claude` and `go run . init bob` from the repo root. Review `git diff .claude .bob AGENTS.md` and confirm the only changes are the rename, plus any Phase 1.1 prose that reached skills. Do not hand-edit them.
- **Dogfooding hazard**: this repo runs the CLI with `go run .`, so from this phase on, the running implement workflow emits `.spektacular/working-context.md`. Before advancing past this phase, copy the current `.spektacular/context.md` content into `.spektacular/working-context.md` by hand for this run. Leave the old file for the user to delete; the product deliberately does not migrate it.

**Complexity**: Medium
**Token estimate**: ~25k
**Agent strategy**: Single agent, sequential. The change is mostly mechanical search-and-replace, guided by `grep -rn "spektacular/context\.md"`, then test updates, then regenerating the installed copies.

### Phase 1.3: Show the new name in the website tutorial

**Requirement → repo/files resolution**: "Published documentation shows the current name" → `docs`: `src/content/tutorials/unknown-criteria.mdx`.

**File changes**:

- `docs:src/content/tutorials/unknown-criteria.mdx:54`: inside the existing fenced ```markdown block, change `.spektacular/context.md` to `.spektacular/working-context.md`. Leave the rest of the quoted paragraph byte-for-byte unchanged, so it still matches the rendered footer. The docs no-em-dash convention covers authored prose, and this is a verbatim quote.
- Do not change `docs:src/pages/index.mdx:38`, `docs:src/pages/how-it-works.mdx:298` or `docs:src/content/tutorials/getting-started.mdx:832`. They describe the plan's `context.md`.
- Do not stage or modify `docs:src/pages/knowledge-base.mdx`, which already has an unrelated uncommitted change.
- Verify from the docs root: `npm run build`, `npx astro check`, and the Rule 1 grep `grep -nE "<div|<section|class=" src/pages/*.mdx` (expect zero matches, as before).

**Complexity**: Low
**Token estimate**: ~5k
**Agent strategy**: Single agent, sequential.

### Phase 2.1: Define the plan documents once and show them when an implementation starts

**Requirement → repo/files resolution**: "Starting an implementation states which plan documents to read" and "Starting and resuming describe the plan documents consistently" (start half) → `spektacular`: `templates/partials/implement-plan-documents.md`, `internal/agent/skills.go`, `templates/skills/workflows/spek-implement/SKILL.md`, `templates/steps/implement/01-read_plan.md`.

**File changes**:

- New `templates/partials/implement-plan-documents.md`. It may use only `{{command}}` and literal `<plan_name>`. Content shape (the wording is illustrative, but the three documents, their descriptions and the commands are fixed):

  ```markdown
  An implementation works from three documents in the plan store. They are the approved plan and the only source of truth for what to build. Read each in full with `plan file read`, never with the `Read` tool:

  - **`plan.md`**: the approved plan. Overview, architecture and design decisions, testing approach, and the `## Milestones & Phases` checklist, whose first unchecked `#### - [ ] Phase` is the current phase. Read it with `{{command}} plan file read <plan_name>/plan.md`.
  - **The plan's `context.md`**: the per-phase technical detail. One `### Phase N.M:` section per phase with the files to change, complexity and agent strategy. Read it with `{{command}} plan file read <plan_name>/context.md`.
  - **`research.md`**: the decision log. Rejected alternatives, supporting evidence, files examined and open assumptions. Read it with `{{command}} plan file read <plan_name>/research.md`.

  None of these is the working context, `.spektacular/working-context.md`, which holds only a session's notes and never replaces the plan.
  ```

- `internal/agent/skills.go:48`: change `mustache.Render(...)` to `mustache.RenderPartials(string(tmplBytes), stepkit.FSPartials{FS: sourceFS}, map[string]string{"command": cfg.Command})`. Update the doc comment at `:39` to mention fragment includes. `internal/agent` already imports `stepkit` in tests; check that the non-test import adds no cycle (stepkit imports `templates`, `store` and `workflow`, not `agent`).
- `templates/skills/workflows/spek-implement/SKILL.md:39` (`# How to start`): before "Ask the user which plan to implement", add a `## The plan documents` subsection containing `{{> partials/implement-plan-documents}}` on its own line.
- `templates/skills/workflows/spek-implement/SKILL.md:29-37` (`# Reading and writing plan files`): leave the read/write mechanics. Change "`plan.md`, `context.md`, and `research.md`" at `:31` to "`plan.md`, the plan's `context.md`, and `research.md`".
- `templates/steps/implement/01-read_plan.md:9-19` (Step 1): keep the heading and the "owned by the workflow while running" sentence. Replace the three-command block with `{{> partials/implement-plan-documents}}` followed by "Here `<plan_name>` is `{{plan_name}}`. Read all three now, before any check below." Keep "These are the source of truth for every downstream step."
- `internal/steps/implement/steps_test.go:238-247` (`TestReadPlanStepContainsFullReadDirective`): keep the `in full` and `plan file read` assertions, and assert `plan file read <plan_name>/context.md` and `research.md`.
- `internal/agent/agent_test.go` (it covers `installWorkflowSkills` with a fixture `sourceFS`): add a case where the fixture skill includes `{{> partials/p}}` and the fixture FS has `partials/p.md` containing `{{command}}`; assert the rendered skill contains the rendered partial. Add a case where the partial is missing and assert install returns an error.
- `cmd/instruction_contract_test.go`: add `TestImplementStartListsPlanDocuments`.
  - Install the claude skills into `t.TempDir()` and read `spek-implement/SKILL.md`. Render `read_plan` via the harness.
  - Assert each contains the hand-written anchors `plan file read <plan_name>/plan.md`, `plan file read <plan_name>/context.md`, `plan file read <plan_name>/research.md`, `the per-phase technical detail` and `the decision log`.
  - Assert the rendered partial block (render `partials/implement-plan-documents.md` via `stepkit.RenderTemplate` with `command`) appears in both after trimming leading whitespace from every line of the haystack and the needle (the helper `normalizeIndent`), since an included partial inherits the include tag's indentation.

**Complexity**: Medium
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential: partial file, installer change with tests, skill and read_plan includes, then contract test.

### Phase 2.2: Make a resumed implementation read the plan first

**Requirement → repo/files resolution**: "A resumed implementation reads the plan before continuing", "A resumed implementation finds its place from the plan", "Resuming adds only what differs from starting", "Starting and resuming describe the plan documents consistently" (resume half) → `spektacular`: `cmd/resume.go`, `templates/steps/resume_implement.md`, `templates/skills/workflows/spek-implement/SKILL.md`.

**File changes**:

- `cmd/resume.go:22-30` (`resumeInstruction`): choose `"steps/resume_implement.md"` when `kind == "implement"`, else `"steps/resume.md"`. Add `"command": command` to the context map. Update the doc comment.
- `cmd/resume.go:38-45` (`mismatchInstruction`): add `"command": command` to the context map, for symmetry.
- New `templates/steps/resume_implement.md`. Its structure (the wording of each item is illustrative; the order and the item set are fixed):

  ```markdown
  ## Resume: an in-progress implement workflow was found

  An unfinished **implement** workflow (`{{name}}`) is already in progress. It stopped at step **`{{current_step}}`**. Nothing has been changed on disk.

  Ask the user whether to **resume** the existing workflow or **start a new one**, then follow the matching path below.

  ### To resume from where it stopped

  1. **Read the plan before anything else.** Do this first, whichever step the run stopped at. Here `<plan_name>` is `{{name}}`.

     {{> partials/implement-plan-documents}}

  2. Read `.spektacular/working-context.md` for the previous session's learnings and the user's answers. It is a session log, not the plan.
  3. Find the current phase from the plan: the first unchecked `#### - [ ] Phase N.M:` heading under `## Milestones & Phases` in `plan.md`. Its technical detail is the matching `### Phase N.M:` section of the plan's `context.md`.
  4. Continue the interrupted step:

     ```
     {{config.command}} implement goto --data '{"step":"{{current_step}}"}'
     ```

  ### To discard it and start fresh

  This overwrites the in-progress workflow's state (recoverable via git if needed):

  ```
  {{config.command}} implement new --force
  ```
  ```

  The partial is indented under a list item. Checked during planning against cbroglie/mustache v1.4.0: a standalone `{{> partial}}` tag indented by three spaces indents every non-blank line of the partial by the same three spaces, so the block renders as a nested part of item 1. Because of that indentation, cross-surface comparisons of the partial must normalise leading whitespace per line (see the tests below).
- `templates/skills/workflows/spek-implement/SKILL.md:57-63` (resume step 2): replace it with an ordered sub-list:
  1. Read the plan documents listed under **The plan documents** above, in full, before anything else.
  2. Read `.spektacular/working-context.md`.
  3. Find the current phase as the first unchecked `#### - [ ] Phase` in `plan.md`.
  4. Run `{{command}} implement goto --data '{"step":"<current_step>"}'`.

  Do not include the partial a second time.
- `cmd/resume_test.go:56-70,80-127`: the existing kind table keeps working. The implement row's `currentStep: "execute"` still renders. Keep the `wantGoto`/`wantNew` and working-context assertions.
- `cmd/resume_test.go`: add `TestResumeImplement_ReadsPlanFirstAtEveryStep`. Use a hand-written list of implement steps: `read_plan`, `analyze`, `implement`, `test`, `verify`, `update_plan`, `update_changelog`, `test_plan`, `update_feature_changelog`, `reconcile_spec`. For each step:
  - Render `resumeInstruction("spekx", "implement", "demo-feature", step)`.
  - Assert it contains the rendered plan-documents partial (after `normalizeIndent`), `.spektacular/working-context.md`, `first unchecked` and `spekx implement goto --data '{"step":"<step>"}'`.
  - Assert the index of `plan file read <plan_name>/plan.md` is less than the index of `.spektacular/working-context.md`, which is less than the index of the goto command.
  - Assert it does **not** contain `repo list` or `.spektacular/work/`.
  - Assert the `### To resume` section holds exactly four numbered items (count the lines matching `^\d+\. ` between `### To resume` and `### To discard`).
- `cmd/resume_test.go`: add `TestResumeNonImplementUsesSharedTemplate`. For spec, plan and repo, the output contains `.spektacular/work/{{name}}`-style wording, or the repo exemption, and `repo list`, confirming the shared template is still used.
- `templates/skill_resume_test.go`: add an implement-only assertion. The skill body contains `The plan documents`, `first unchecked` and `{{> partials/implement-plan-documents}}` exactly once (the resume path refers to the section rather than including the partial again).
- `cmd/instruction_contract_test.go` `TestImplementStartListsPlanDocuments` (from 2.1): extend it to assert that the resume instruction contains the same partial text (after `normalizeIndent`) as the installed skill and `read_plan`.

**Complexity**: Medium
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential: template and selector, then skill, then tests.

### Phase 2.3: Have phase steps fetch their own detail and name the plan's context.md unambiguously

**Requirement → repo/files resolution**: "Steps that act on a phase fetch its detail themselves", "The plan's technical detail is always named unambiguously" → `spektacular`: `templates/steps/implement/0[1-5]-*.md`, `templates/steps/plan/*.md`, `templates/skills/workflows/spek-{implement,plan}/SKILL.md`, `templates/skills/skill_verify-implementation.md`.

**File changes**:

- `templates/steps/implement/03-implement.md:3`: replace "You have the analysis summaries from the previous step — use them as your map…" with a `### Step 1: Load the current phase` block:
  - Read `{{config.command}} plan file read {{plan_name}}/plan.md`, and take the first unchecked `#### - [ ] Phase N.M:` as the current phase.
  - Read `{{config.command}} plan file read {{plan_name}}/context.md`, and read its `### Phase N.M:` section in full.
  - Then write the code. If the section is missing, STOP (same wording as `02-analyze.md:15`).
- `templates/steps/implement/03-implement.md:10,16`: change "described in `{{context_path}}`" to "described in the plan's `context.md` phase section", and "Update `{{plan_path}}` and/or `{{context_path}}`" to "Update the plan's `plan.md` and/or `context.md` through `{{config.command}} plan file write`".
- `templates/steps/implement/04-test.md:13-19`: before the sub-agent list, add the same Load-the-current-phase block (both `plan file read` commands with `{{plan_name}}`). Tell the main agent to pass the phase's acceptance criteria and its section of the plan's `context.md` to the sub-agent. At `:18`, change "from `{{plan_path}}`" to "from the current phase in the plan's `plan.md`".
- `templates/steps/implement/05-verify.md:13-16`: add the same block. At `:15`, change "from `{{plan_path}}`" to "from the current phase in the plan's `plan.md`". At `:16`, change "listed in `{{context_path}}`" to "listed in the current phase's section of the plan's `context.md`".
- `templates/steps/implement/02-analyze.md:7,13,15,27`: qualify each bare mention. Line 7: "to a section in the plan's `context.md`". Line 13: "Read the plan's `context.md` through the plan store". Line 15: "fix the plan's `context.md`". Line 27: "listed in the phase section of the plan's `context.md`".
- `templates/steps/implement/01-read_plan.md:38`: prefix the line with "In the plan's `plan.md`, every phase has a `*Technical detail:*` link into the plan's `context.md`, in the form `[context.md#phase-NM](./context.md#...)`." `:39,45,58` use `{{context_path}}` (an absolute path containing the plan name), which already qualifies.
- `templates/steps/plan/03-architecture.md:39`, `04-components.md:9`, `05-data_structures.md:11`, `06-implementation_detail.md:7,9`, `08-testing_approach.md:7,9`, `10-phases.md:6,14,15,18,20,35,43`, `13-assemble.md:32,41,49,66`, `14-verification.md:29,53`, `16-write_context.md:3,6,16`: rewrite each bare `context.md` as "the plan's `context.md`", keeping the sentence otherwise intact. Headings such as `10-phases.md:18` become `### Phase content in the plan's context.md`. Lines already carrying `{{plan_name}}/context.md` (`16-write_context.md:9`, `18-walkthrough.md:9`, `19-finished.md:8`) need nothing. Lines naming `phases_context.md` or `context_template.md` alone are not mentions.
- `templates/skills/workflows/spek-plan/SKILL.md:16,33,45`: change "`plan.md`, `context.md`, and `research.md`" to "`plan.md`, the plan's `context.md`, and `research.md`". Line 47 already says "the plan's own `context.md`".
- `templates/skills/skill_verify-implementation.md:15`: change "in `context.md`" to "in the plan's `context.md`".
- `internal/steps/plan/steps_test.go:145,151,655`: these still pass with the qualified wording, since `Contains "context.md"` remains true. Leave them unless their messages now mislead.
- `internal/steps/implement/steps_test.go`: add `TestPhaseStepsReadPhaseDetail`. For `implementStep()`, `testStep()` and `verify()` rendered with `name: "test"`, assert the output contains `spektacular plan file read test/context.md` and `spektacular plan file read test/plan.md`. Assert it does not contain `analysis summaries`, `from the previous step` or `already available`.
- `cmd/instruction_contract_test.go`: add `TestContextMdAlwaysQualified`.
  - Corpus: the rendered step instructions, both resume instructions for every kind, the installed workflow skills (with `<name>`/`<plan_name>` placeholders), the rendered `skill_*.md` helper skills, and the rendered managed AGENTS.md sections.
  - Find each occurrence of `context.md` not preceded by `[A-Za-z0-9_-]` (this excludes `phases_context.md`, `context_template.md` and `working-context.md`).
  - Each such occurrence's line must contain `plan's`, or `demo-feature/context.md`, `<plan_name>/context.md`, `<name>/context.md`, or `/demo-feature/` (the absolute `context_path`).
  - On failure, report the file and line.
- Final verification for the milestone: `go test ./...`; `make harbor-test-plan` and `make harbor-test-spec` (Docker + credentials; ~25 min each), recording any oracle drift they surface.

**Complexity**: Medium
**Token estimate**: ~30k
**Agent strategy**: Two parallel agents: (a) implement templates 01-05 plus the implement step test; (b) plan templates, skills and the helper skill qualification. Then one agent adds the contract test, iterates on any flagged line, and runs the full suite and harbor.

## Testing Strategy

- **Phase 1.1.**
  - stepkit unit tests: footer appended when a next step exists and absent otherwise; `FSPartials` resolves files and errors when one is missing; `{{command}}` renders.
  - New `cmd/instruction_contract_test.go` harness (hand-written `{template, nextStep}` table plus a completeness walk) with `TestContinuingStepsEndWithIdenticalFooter`, `TestNoTemplateFileCarriesFooter` and `TestNextCommandsCarryPrefix` (command `spekx`; corpus is steps, resume, mismatch and installed skills).
  - `templates/context_directive_test.go` is deleted; the guided-add em-dash test drops its footer exclusion.
- **Phase 1.2.**
  - `workingcontext_test` expects the new path.
  - Spec `new` tests seed both files and assert the new one is cleared, the legacy file is untouched, and no `.spektacular/context.md` is created.
  - `TestNoEmittedInstructionNamesOldWorkingContext` and `TestEachWorkflowNamesWorkingContext` in the contract file.
  - Resume and skill guards expect the new path.
- **Phase 1.3.** Docs build, `astro check` and the Rule 1 grep. No new automated test.
- **Phase 2.1.**
  - `agent_test.go` fixture-FS partial include, plus the missing-partial error.
  - `TestImplementStartListsPlanDocuments` (installed skill and `read_plan` both contain the anchors and the normalised partial).
  - The `read_plan` step test asserts the `<plan_name>` commands.
- **Phase 2.2.**
  - `TestResumeImplement_ReadsPlanFirstAtEveryStep` (per implement step: anchors, ordering plan < working context < goto, exactly four resume items, no `repo list` or `.spektacular/work/`).
  - `TestResumeNonImplementUsesSharedTemplate`.
  - An implement skill assertion (a plan-documents section, the partial included exactly once, the first-unchecked rule).
  - The contract test is extended to check the resume partial text.
- **Phase 2.3.**
  - `TestPhaseStepsReadPhaseDetail` in the implement step tests.
  - `TestContextMdAlwaysQualified` across every emitted surface (line-based; compound names excluded by a `[A-Za-z0-9_-]` look-behind).
  - Full suite, then harbor plan and spec runs.
- **Success metrics.**
  - Metric 1 is covered at the instruction level by the Phase 2.2 and 2.3 contract tests. Real-agent resume behaviour is manual, captured in the implementation test plan.
  - Metric 2 (five real resumed runs after release) is manual, captured in the implementation test plan.

## Project References

- `spektacular:architecture/testing-architecture.md`: three test layers; step-template changes include a harbor run in verification.
- `spektacular:architecture/workflow-steps.md`: step/callback/template wiring.
- `spektacular:conventions/tests-must-pass-for-done.md`
- `docs:conventions/plan-content-pages.md`, `docs:conventions/mdx-authoring.md`, `docs:conventions/no-em-dashes.md`

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Phase 1.3 is Low. Phases 1.1, 1.2, 2.1, 2.2 and 2.3 are Medium. Phase 1.1's footer removal is a scripted mechanical edit, so keep the 45-file diff out of the main context and check it with `grep -rc "Before you advance" templates/steps`.

## Migration Notes

- No data migration. An existing `.spektacular/context.md` is deliberately ignored (spec constraint). A run interrupted before upgrading resumes without its previous working context.
- **Dogfooding in this repo:** the implement run for this plan uses `go run .`, so once Phase 1.2 lands the run's later steps emit `.spektacular/working-context.md`. Before leaving Phase 1.2, copy the current `.spektacular/context.md` content into `.spektacular/working-context.md` by hand, and leave the old file for the user to remove.
- Regenerate `.claude/`, `.bob/` and `AGENTS.md` with `go run . init claude` and `go run . init bob` after Phases 1.1, 1.2 and 2.1-2.3 touch skills or managed sections (at minimum once at the end of 1.2 and once at the end of 2.3).

## Performance Considerations

None of significance. Rendering the footer adds one extra template render per step instruction, and partial resolution reads small embedded files.
