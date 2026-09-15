---
created_date: "2026-09-15"
status: completed
closed_date: "2026-09-15"
---

# Plan: 000051_working-context-and-plan-reading

<!-- Metadata -->
<!-- Created: 2026-09-15T14:17:10Z -->
<!-- Commit: 0b57a7cbdbd2da3b0326300eb731dd4a99a62bba -->
<!-- Branch: f-knowledge-search -->
<!-- Repository: git@github.com:jumppad-labs/spektacular.git -->

## Overview

When a coding agent resumes an interrupted implementation, it can mistake its own session notes for the implementation plan, because the working context lives in `.spektacular/context.md` and the plan's per-phase detail lives in a file also called `context.md`, and nothing on resume tells the agent to read the plan. This plan renames the working context to `.spektacular/working-context.md` across every workflow, writes the standing keep-context-current footer once and renders it for every continuing step, and defines the plan documents once so that starting and resuming an implementation both tell the agent exactly which documents to read and how. A resumed implementation reads the plan before anything else, finds its phase from the plan, and the steps that write code, write tests and verify a phase fetch that phase's detail themselves, so work continues from the plan the team approved rather than from a partial record of the last session.

## Conventions

- **Passing tests are required before calling work done** (`spektacular:conventions/tests-must-pass-for-done.md`): this change rewrites the expected substrings of many guard tests at once (footer text, working-context path, `context.md` mentions), so the full test suite must pass at the end of every phase, and a guard test whose expectation changes gets updated in the same phase rather than skipped.
- **Plans must sketch content structure, not just summarize it** (`docs:conventions/plan-content-pages.md`): the tutorial edit is a small copy change, so its phase carries a **Content example** showing the exact replacement line.
- **MDX authoring conventions, Rule 4 (fenced code blocks)** (`docs:conventions/mdx-authoring.md`): the quoted footer sits inside a fenced ```markdown block in `unknown-criteria.mdx`, so the edit changes text inside the fence only and adds no JSX or layout markup.
- **No em dashes** (`docs:conventions/no-em-dashes.md`): applies to authored docs prose; the tutorial line is a verbatim quote of the emitted footer, so it keeps the footer's own wording and only the path changes (recorded as an assumption).

## Architecture & Design Decisions

The work lands almost entirely in the `spektacular` repo, with one line in the `docs` repo. It has four strands that share one principle: every piece of agent-facing prose this feature touches is defined in exactly one template and reaches every surface that needs it through the renderer, so the next wording change is a single edit.

**Working-context rename.** The working-context path is defined once in Go (`internal/workingcontext`), and that constant changes from `.spektacular/context.md` to `.spektacular/working-context.md`. The only runtime writer, the `spec new` reset, follows automatically. Nothing reads, moves or reports on an old `.spektacular/context.md`: per the spec's constraint it is simply ignored, and a run interrupted before the upgrade resumes without its old notes. Every template that names the working context (step instructions, the shared resume instruction, the four workflow skills, and the historical-artifacts section installed into `AGENTS.md`) switches to the new name. The tracked installed copies (`.claude/`, `.bob/`, `AGENTS.md`) are regenerated with `init`, never edited by hand. The plan's own `context.md` and its scaffold keep their names, so existing plans stay readable.

**Renderer-owned footer.** The keep-context-current paragraph moves out of the 45 step templates that copy it and into one shared fragment under a new `templates/partials/` directory. The shared step renderer (`stepkit.WriteStepResult`), which all four workflows already go through, appends it after the rendered instruction whenever the step has a next step. Terminal `finished` steps have none, so they never get it. This makes "identical in every continuing step" true by construction rather than by a count in a test. It also removes the special case for `spec new`: its purpose-built capture instruction now ends with the same standing footer as every other step. We picked the renderer over the four per-workflow `writeStep` wrappers because it is the one place all of them share (see `research.md#chosen-approach--evidence`). The same step fixes `spec/00-new.md`'s `{{command}}` placeholder so its next command renders with the prefix.

**One definition of the plan documents, rendered where it is needed.** A single mustache partial, `templates/partials/implement-plan-documents.md`, names `plan.md`, the plan's `context.md` and `research.md`, says what each holds, and gives the `plan file read` command for each. It is included in three places: the spek-implement skill's start instructions (rendered at install time), the implement `read_plan` step, and a new implement-specific resume instruction (both rendered at runtime). The skill's own resume path points back to that section instead of repeating it. So start and resume describe the documents identically, and there is only one copy to maintain. To support this, both renderers (the step/resume renderer and the skill installer) resolve partials from the embedded templates filesystem via `mustache.RenderPartials`, and the runtime renderer also exposes the command prefix as `command`, the spelling install-time templates already use, so the partial renders the same on both surfaces. We chose this over having the skill alone hold the definition and the CLI report merely point at it. The resume criteria are written per interrupted step, which only the CLI-rendered report can meet, and a report that only points cannot "describe them the same way". Hand-copying the block into three files was rejected as drift-prone (`research.md#alternatives-considered-and-rejected`).

**Implement resume and phase-scoped steps read the plan.** `resumeInstruction` picks a dedicated `steps/resume_implement.md` when the kind is `implement`. Spec, plan and repo keep the shared `steps/resume.md`, unchanged apart from the rename. The implement resume path holds only the additive items, in this order: a run is in progress; read the plan documents (the partial) before anything else; read the working context; take the current phase to be the first unchecked `#### - [ ] Phase` in `plan.md`; then run the full `implement goto` for the interrupted step. The resume-or-start-new choice and the `--force` alternative still frame the report. The phase is re-derived from the plan, never stored in state. Separately, the `implement`, `test` and `verify` step instructions each work out the current phase the same way and fetch its section with `plan file read <plan_name>/context.md`, so none relies on `analyze` output surviving in context. Throughout the workflows' emitted prose, every mention of the plan's technical-detail document is written as "the plan's `context.md`" or as a path that includes the plan's name. Template-contract tests (hand-written oracles, rendering into test-owned state) enforce the rename, the footer, the qualification, the full command prefix and the resume ordering. This follows the project's rule that prose-driven behaviour is guarded by phrase assertions.

## Component Breakdown

- **Working-context location (changed, `spektacular`)** — Owns the single definition of where the agent's working-context file lives, and the reset that clears it when a fresh spec workflow starts. Its name changes to `working-context`; nothing else about its behaviour changes. It never looks for, migrates or rejects a file under the old name. The spec workflow's `new` step is its only runtime caller.
- **Shared template fragments (new, `spektacular`)** — A small set of prose fragments defined once and rendered into several agent-facing surfaces. There are two: the standing keep-context-current footer, and the implement plan-documents block (each plan document's name, what it holds, and the command to read it). They have no behaviour of their own. They exist so the prose that must stay identical across surfaces has exactly one copy.
- **Step renderer (changed, `spektacular`)** — The shared pipeline every workflow uses to turn a step into an instruction. It gains three duties: resolving fragment includes from the embedded templates, exposing the command prefix under the spelling the fragments use, and appending the footer fragment to every step that has a next step. It stays unaware of which workflow it is rendering and of what the fragments say.
- **Skill installer (changed, `spektacular`)** — Renders the workflow skills into each agent's install directory. It gains fragment-include resolution, so the implement skill's start instructions carry the plan-documents block from the same source the runtime surfaces use.
- **Resume instruction selection (changed, `spektacular`)** — Builds the in-progress report's instruction. For an implement workflow it now renders an implement-specific resume instruction. Spec, plan and repo workflows keep the shared one. The cross-kind mismatch report is unchanged.
- **Implement resume instruction (new, `spektacular`)** — The implement-only resume prompt. It keeps the resume-or-start-new framing and the discard alternative, and its resume path holds only the additive items: a run is in progress, read the plan documents (via the shared fragment), read the working context, find the current phase as the plan's first unchecked phase, then continue the interrupted step with the full command.
- **Shared resume instruction (changed, `spektacular`)** — Still used by spec, plan and repo workflows, and changed only by the working-context rename.
- **Implement step instructions (changed, `spektacular`)** — `read_plan` includes the plan-documents fragment in place of its own list. `implement`, `test` and `verify` each identify the current phase from the plan and fetch that phase's technical detail themselves, no longer assuming earlier summaries are still in context. Every mention of the plan's technical-detail document is qualified as the plan's.
- **Spec, plan and repo step instructions (changed, `spektacular`)** — Lose their copied footer (the renderer now supplies it), switch to the new working-context name where they name it, and qualify every mention of the plan's `context.md`. `spec new` also gets the full command prefix on its next command.
- **Workflow skills (changed, `spektacular`)** — All four skills read and refresh the working context under its new name. The implement skill gains a plan-documents section in its start instructions (the fragment), and its resume path points to that section and lists the same additive items as the CLI report. The plan skill's description of the sidecar keeps distinguishing it from the plan's `context.md`.
- **Runtime helper skills and managed agent sections (changed, `spektacular`)** — The verify-implementation helper skill qualifies its `context.md` mention as the plan's. The historical-artifacts section installed into `AGENTS.md` names the working-context file by its new name.
- **Template-contract guard tests (changed and new, `spektacular`)** — Enforce the feature against rendered output using hand-written oracles: the old working-context path appears nowhere; every continuing step ends with the exact footer and terminal steps do not; every `context.md` mention is qualified; every next command carries the prefix; the implement resume instruction, rendered for each implement step, orders read-the-plan before the goto and carries only the additive items; start and resume render identical plan-documents text; `implement`, `test` and `verify` carry the phase-detail read command. Existing guards that assert the old path or count footers inside template files are updated in the same change.
- **Installed agent copies (regenerated, `spektacular`)** — The tracked `.claude/` and `.bob/` skill copies and the managed `AGENTS.md` sections are regenerated with `init` so this repo dogfoods the new instructions.
- **Website tutorial (changed, `docs`)** — The tutorial that quotes the footer verbatim shows the new working-context name. Other website mentions of `context.md` refer to the plan document and stay as they are.

## Data Structures & Interfaces

This feature adds no persisted data, no state fields, no JSON output fields and no CLI flags. The workflow state file, the resume report's JSON envelope and the plan documents keep their current shapes. The contracts that do change are between the renderer, its templates, and the prose surfaces they produce.

**Working-context path constant (changed)**

```go
// internal/workingcontext
const RelPath = ".spektacular/working-context.md" // was ".spektacular/context.md"
func Path(root string) string   // unchanged signature
func Reset(path string) error   // unchanged signature
```

The one Go definition of the working-context file's location. Every Go caller already derives from it.

**Template fragment provider (new)**

```go
// internal/stepkit
// FSPartials resolves mustache partial includes ({{> partials/<name>}})
// from an fs.FS, reading "<name>.md". A missing partial is an error,
// never a silent empty render.
type FSPartials struct{ FS fs.FS }
func (p FSPartials) Get(name string) (string, error) // satisfies mustache.PartialProvider
```

Shared by the runtime step renderer (over the embedded templates) and the skill installer (over its `sourceFS`, so test fixtures keep working). It is the only way a fragment reaches a surface.

**Template include contract (new)**

- Fragments live under `templates/partials/` and are included by path: `{{> partials/implement-plan-documents}}`.
- A fragment may use only variables every including surface provides: `{{command}}` (the CLI prefix). It uses the literal `<plan_name>` for the plan's name, because install-time skills do not know it.
- `templates/partials/working-context-footer.md` is not included by any template. The renderer appends it.

**Step render variables (changed)**

```go
// stepkit.WriteStepResult standard vars
vars := map[string]any{
    "step": ..., "title": ..., "next_step": ...,
    "config":  map[string]any{"command": cfg.Command}, // existing spelling used by step templates
    "command": cfg.Command,                            // new: spelling used by shared fragments
}
```

The resume renderers (`resumeInstruction`, and `mismatchInstruction` for symmetry) add the same `command` key to their context.

**Step instruction output contract (changed)**

```
instruction = render(template, vars)
if NextStep != "":
    instruction = trimTrailingNewlines(instruction) + "\n\n---\n\n" + render("partials/working-context-footer.md", vars)
```

Every continuing step's `instruction` string ends with exactly one copy of the footer. Terminal steps (`NextStep == ""`) end with their template's own text. The `Result` structs each workflow emits keep their fields.

**Resume instruction selection (changed)**

```go
// cmd/resume.go
func resumeInstruction(command, kind, name, currentStep string) (string, error)
// kind == "implement" → "steps/resume_implement.md"
// otherwise           → "steps/resume.md"
```

The signature and the `workflow_in_progress` error envelope (`code`, `resource`, `state.current`, `next_action`) are unchanged. Only the template chosen for `next_action` depends on the kind.

## Implementation Detail

**New pattern: shared template fragments.** Until now each template has been a self-contained file, and prose that needed to appear in several places was copied. This plan adds a fragments area of the templates tree and mustache partial includes to both render paths (runtime step and resume rendering, and install-time skill rendering). A developer who wants the same paragraph on more than one surface writes it once as a fragment and includes it by name. A fragment is written against the smallest variable set every includer provides: the command prefix, plus placeholder text for anything only some surfaces know, such as the plan's name. Includes resolve from the same filesystem the including template came from, so tests that substitute a fixture filesystem for the skill installer keep working, and a missing fragment fails the render loudly.

**New pattern: renderer-appended standing text.** The renderer now adds prose to a step's instruction on its own. The rule is structural: a step with a next step gets the footer, and a terminal step does not. So step template authors no longer end every file with the same paragraph, and a template that still carries a copy would show the footer twice (the guard tests catch that). Someone reading a step template will no longer see the footer in it. The renderer's doc comment and the fragment's own header say where it comes from.

**Existing patterns followed.**
- Workflow-kind-specific prose lives in its own template file, not in conditionals inside a shared one. The implement resume instruction is a sibling of the shared resume instruction and the cross-kind mismatch instruction, and the resume renderer picks between them by kind, the same way it already picks between same-kind and cross-kind.
- "Current phase = first unchecked phase heading in `plan.md`" is the rule the `analyze` step already uses. The resume instruction and the `implement`, `test` and `verify` steps restate that rule in the same words rather than inventing a new one, and nothing stores the phase.
- Plan documents are only ever reached through `plan file read`. The mid-phase steps drop their raw path references in favour of the command.
- Behaviour expressed as prose is guarded by template-contract tests with hand-written oracles. The new guards render through the production renderers (step renderer, resume renderer, skill installer into a test-owned temp directory) rather than scanning raw template files, because the footer and fragments exist only in rendered output.
- Installed agent copies are regenerated by `init`, never edited by hand.

**Code-shape changes.**
- The step renderer's template loader switches from a plain render to a partial-aware render. Every existing caller (step scaffolds, resume instructions, step instructions) goes through it unchanged, and templates without includes render byte-identically.
- Four workflows' step templates lose their trailing footer block. This is a large but purely subtractive diff, done in the same change as the renderer append so no build ever emits zero or two footers.
- Existing template-contract tests that assert the old path, count footers in template files, or exempt `spec new` from the footer are rewritten to assert against rendered output. Tests that assert bare `context.md` in plan and implement step output switch to the qualified wording.

**What is not changing shape.** The workflow engine, the state file, the result JSON, the plan store and its documents, and the per-workflow step lists are untouched. No new command or flag is added.

## Dependencies

- **`github.com/cbroglie/mustache` v1.4.0 (external, already a dependency)** — Provides `RenderPartials` and the `PartialProvider` interface that fragment includes rely on. No version change is needed.
- **`internal/stepkit` (internal)** — The shared step renderer. It changes to resolve partials, expose the `command` variable and append the footer. Every workflow's step output depends on it, so it lands together with the template footer removal.
- **`internal/workingcontext` (internal)** — Holds the working-context path constant. It changes, and the `spec new` step picks the change up with no code change of its own.
- **`internal/agent` skill installer (internal)** — Renders workflow skills at `init`. It changes to resolve partials, which must be in place before the implement skill can include the plan-documents fragment.
- **`cmd` resume renderer (internal)** — Selects the resume template. It changes to pick the implement-specific template and to pass `command`.
- **`templates` embedded FS (internal)** — Gains the `partials/` directory and `steps/resume_implement.md`. The `all:*` embed directive already picks up new directories, so nothing changes there.
- **Harbor E2E suites, `tests/harbor/plan-workflow` and `tests/harbor/spec-workflow` (internal, not in CI)** — Their oracles were checked in discovery and reference neither the working-context path nor the footer text, so no oracle change is expected. Per the testing-architecture knowledge entry, a step-template change still includes a harbor run in verification, to catch drift the oracles did not anticipate.
- **`docs` repo (`spektacular-website`)** — One tutorial line changes. It is independent of the Go change and can land in any order, but the website shows the new name only once both have shipped. The repo currently has an unrelated uncommitted change that this work must leave alone.
- **Prior plan `000024_resume` (planning, landed)** — Introduced the `kind` marker, re-render on `goto <current_step>`, the shared resume template and the per-step footer that this plan restructures. Nothing needs to land first.
- **Spec `000051_working-context-and-plan-reading` (planning)** — Source of scope and acceptance criteria. Nothing else must land before this plan starts.

## Testing Approach

Almost all of this feature is agent-facing prose, so the load-bearing tests are **template-contract tests**: they render instructions through the production renderers and assert anchor phrases, the project's established layer for prose-driven behaviour. They are backed by a few **Go unit tests** for the mechanics (the path constant, the partial provider, footer append, resume template selection), and one **harbor E2E run** as the non-CI behavioural check.

**Where tests slot in.** Unit tests sit next to the packages they cover (working context, step renderer, resume renderer, skill installer). Contract tests join the existing `templates` package guards and the per-workflow step tests. All use `testify/require`. Expected strings (the footer text, the new path, the plan-documents wording anchors, the list of implement steps) are **hand-written oracles** in the tests, never read back from the templates under test. Anything that needs the filesystem, such as the `spec new` reset or rendered skills, works inside a test-owned temp directory through the production install or render path.

**What the tests guarantee.**
- **Rename.** The path constant resolves to the new name. `spec new` writes the working context under the new name and creates no `.spektacular/context.md`. No emitted instruction (every step of every workflow, both resume instructions, the mismatch instruction, every installed workflow skill, the runtime helper skills and the managed agent sections) contains the old working-context path. Each of the spec, plan, implement and repo workflows emits the new path in at least one step instruction and in its skill.
- **Footer.** Rendering every step of every workflow shows each continuing step ending in exactly one copy of the hand-written footer, byte-identical across steps, and each terminal step carrying none. No step template file still contains the footer text, so it can never be emitted twice. `spec new` is no longer exempt.
- **Full command prefix.** No rendered step, resume or skill instruction contains a bare `spec goto`, `plan goto`, `implement goto` or `repo goto` that lacks the configured prefix directly before it. The test uses a non-default prefix so a missing or unrendered placeholder is visible.
- **Plan documents at start.** The installed implement skill and the rendered `read_plan` step both name `plan.md`, the plan's `context.md` and `research.md`, say what each holds, and give the `plan file read` command for each. The plan-documents text is byte-identical in the installed skill, the `read_plan` step and the implement resume instruction.
- **Implement resume.** For every implement step name used as the interrupted step, the rendered resume instruction contains the in-progress statement, the plan-documents block, the working-context read, the "first unchecked phase in `plan.md`" rule and the full `implement goto` for that step, with the read-the-plan instruction appearing before the goto. Its resume path has exactly those ordered items and nothing else (no `repo list`, no `.spektacular/work/`). Spec, plan and repo resume instructions still render the shared template.
- **Skill resume path.** The implement skill's resume path points to its plan-documents section, reads the working context under the new name, and names the first-unchecked-phase rule.
- **Phase-scoped steps.** The rendered `implement`, `test` and `verify` instructions each contain `plan file read <plan>/context.md` with the concrete plan name, and none says an earlier step's summaries or analysis are already available.
- **Qualified `context.md`.** In every emitted instruction, each mention of `context.md` as a standalone file name (compound names such as `phases_context.md` excluded) sits on a line that qualifies it as the plan's, either in words ("the plan's") or as a path or command containing the plan's name or its placeholder.
- **Partial provider.** A missing fragment fails the render with an error. Templates without includes render exactly as before.

**Existing tests updated, not added alongside.** Guards that assert the old path, count footers in template files, exempt `spec new`, or expect bare `context.md` are rewritten in the same phase as the change they guard. We do not add a second assertion for a bug class an updated guard already catches.

**Website.** The docs change is a one-line copy edit inside a fenced block. It is checked by the docs repo's own guards (build, type check, the no-layout-HTML grep). No new automated test is added.

**Deliberate gaps.** No test drives a real agent through an interrupted-then-resumed implement run in CI. That behaviour is prose followed by a model and only the harbor layer can exercise it. The harbor plan and spec suites are run once as part of final verification, since this change touches every step template.

**Success metrics.**
- *"An implementation interrupted after completing some phases and resumed in a fresh session continues into the next unfinished phase using that phase's detail from the plan, and does not report the plan as missing detail it actually contains."* The instruction-level part is a **behavioural test**: the implement-resume contract test above guarantees that, at every step, a resumed agent is told to read the plan documents first, find the first unchecked phase and continue, and the phase-scoped step tests guarantee `implement`, `test` and `verify` fetch that phase's detail from the plan. Whether a real agent then actually continues correctly in a fresh session is **Manual — captured in the implementation test plan**.
- *"Over the first five implementation runs resumed in a fresh session after this ships, none reports a plan/reality mismatch traced to the agent reading the working context in place of the plan."* **Manual — captured in the implementation test plan** (field observation across real runs after release).

## Milestones & Phases

### Milestone 1: Session notes have a name of their own

**What changes**: The notes an agent keeps as it works (the working context) move to `.spektacular/working-context.md`, so no plan document shares their name any more. Every spec, plan, implement and repo-add instruction, every workflow skill, and the website tutorial that quotes the instructions use the new name. The standing "refresh your working context before advancing" paragraph now reads the same at the end of every continuing step, because it is written once instead of copied into each step. Every next command an agent is told to run now shows the full command prefix, including the one at the start of a new spec that used to render without it. An old `.spektacular/context.md` left by an earlier version is ignored.

**Validation point**: Running any step of any workflow emits the new name and never the old one. Every continuing step ends in the identical footer and terminal steps have none. No emitted next command lacks its prefix. The installed skills and `AGENTS.md` in this repo are regenerated with the new name, the website tutorial shows it, and the full test suite passes.

#### - [ ] Phase 1.1: Write the standing footer once and render it for every continuing step

**Repo:** `spektacular`

The keep-context-current paragraph is currently copied into 45 step instructions. This phase moves it into one shared fragment and has the step renderer append it to every step that continues to another step, and gives the renderer the ability to include shared fragments, which Milestone 2 relies on. It also fixes the new-spec instruction whose next command rendered without its command prefix, and adds guards that every continuing step ends with the identical footer and that no next command is ever shown without its prefix.

*Technical detail:* [context.md#phase-11](./context.md#phase-11-write-the-standing-footer-once-and-render-it-for-every-continuing-step)

**Acceptance criteria**:

- [ ] Every continuing step in the spec, plan, implement and repo workflows ends with exactly one copy of the same keep-context-current paragraph, and no finished step carries it.
- [ ] Changing the footer's wording means editing one file, and no step instruction file still contains its own copy.
- [ ] Starting a new spec shows the next command with the configured command prefix, and it now ends with the standing footer like every other continuing step.
- [ ] No emitted step, resume or skill instruction shows a `spec goto`, `plan goto`, `implement goto` or `repo goto` without the command prefix in front of it.
- [ ] Instructions that include no shared fragment render exactly as they did before, apart from the footer now being supplied by the renderer.
- [ ] The full test suite passes.

#### - [ ] Phase 1.2: Rename the working context to working-context everywhere

**Repo:** `spektacular`

This phase moves the agent's working context from `.spektacular/context.md` to `.spektacular/working-context.md`. The change runs through the one path definition, the reset at the start of a spec, the footer, every step and resume instruction and skill that names it, and the section installed into `AGENTS.md`. An old file under the previous name is left alone and never read. The tracked installed skill copies and `AGENTS.md` in this repo are regenerated so the project dogfoods the new name.

*Technical detail:* [context.md#phase-12](./context.md#phase-12-rename-the-working-context-to-working-context-everywhere)

**Acceptance criteria**:

- [ ] Starting a spec clears `.spektacular/working-context.md` and never creates or writes a `.spektacular/context.md`.
- [ ] The spec, plan, implement and repo-add workflows each emit instructions that read or refresh the working context under its new name, and their skills say the same.
- [ ] No emitted instruction, installed skill or managed `AGENTS.md` section mentions `.spektacular/context.md`.
- [ ] An existing `.spektacular/context.md` from an earlier version is neither moved, read, nor reported as an error.
- [ ] Existing plans, including their own `context.md`, still read and write without change.
- [ ] The installed `.claude/` and `.bob/` skills and `AGENTS.md` in this repo show the new name.
- [ ] The full test suite passes.

#### - [ ] Phase 1.3: Show the new name in the website tutorial

**Repo:** `docs`

The website's tutorial on unknown acceptance criteria quotes the keep-context-current paragraph word for word. This phase updates that quote to the new working-context name so the published docs match what agents now receive. The site's other `context.md` mentions describe the plan document and stay as they are.

*Technical detail:* [context.md#phase-13](./context.md#phase-13-show-the-new-name-in-the-website-tutorial)

**Content example**:

The quoted footer inside the tutorial's fenced `markdown` block changes only in its path:

```markdown
**Before you advance:** refresh `.spektacular/working-context.md` with your cross-cutting working context only — the key decisions and substitutions made, the answers the user gave to your questions, and learnings worth carrying forward. Keep it to learnings and decisions, not a transcript and not a copy of content already captured elsewhere (such as a section's own working file). Use your own file tools. This file is git-tracked, and a resumed session reads it back to pick up where you left off, so keep it current every time before running the `goto` command above.
```

**Acceptance criteria**:

- [ ] The tutorial's quoted instruction shows `.spektacular/working-context.md` and matches the footer agents now receive.
- [ ] The website's other mentions of the plan's `context.md` are unchanged.
- [ ] The site builds and type-checks cleanly, and no layout markup has been added to page bodies.
- [ ] The unrelated uncommitted change already in the docs repo is left untouched.

### Milestone 2: Implementations read the plan when they start and when they resume

**What changes**: Starting an implementation now tells the agent exactly which plan documents to read (`plan.md`, the plan's `context.md` and `research.md`), what each one holds and how to read it. Resuming an interrupted implementation tells the agent, before anything else, to read those same documents (described in the same words), then its working context, then to find the current phase as the plan's first unchecked phase, and only then to continue the interrupted step. The steps that write code, write tests and verify a phase each fetch that phase's technical detail from the plan themselves, so a resume landing on any of them works from the approved plan rather than from whatever the last session left in its notes. Every instruction that mentions the plan's `context.md` now says it belongs to the plan.

**Validation point**: The implement skill's start instructions and the `read_plan` step list the three plan documents with descriptions and read commands, identical to the resume instruction's list. For an implementation interrupted at each of its steps, the resume instruction orders read-the-plan before the continue command and carries only the additive items. The `implement`, `test` and `verify` steps each include the phase-detail read command. No emitted instruction mentions `context.md` without qualifying it as the plan's. The full test suite and a harbor run pass.

#### - [ ] Phase 2.1: Define the plan documents once and show them when an implementation starts

**Repo:** `spektacular`

This phase writes a single shared description of the three plan documents: `plan.md`, the plan's `context.md` and `research.md`, what each holds, and the command to read each. The implement skill's start instructions and the implement workflow's first step both include it. To make that work, the skill installer learns to include shared fragments the same way the step renderer already does, so both surfaces render the same words from one source.

*Technical detail:* [context.md#phase-21](./context.md#phase-21-define-the-plan-documents-once-and-show-them-when-an-implementation-starts)

**Acceptance criteria**:

- [ ] The installed implement skill's start instructions name `plan.md`, the plan's `context.md` and `research.md`, state what each contains, and give the command to read each.
- [ ] The implement workflow's first step shows the same plan-documents description, word for word, and still directs a full read of all three before validation.
- [ ] The plan-documents description distinguishes the plan's `context.md` from the working context.
- [ ] A skill or step that includes a shared fragment that does not exist fails to render instead of silently omitting it.
- [ ] Skills that include no fragment install exactly as before.
- [ ] The full test suite passes.

#### - [ ] Phase 2.2: Make a resumed implementation read the plan first

**Repo:** `spektacular`

When an interrupted implementation is found, the resume instruction now comes from an implement-specific template. It adds only what differs from starting: a run is in progress; read the plan documents first (the same shared description); read the working context; find the current phase as the plan's first unchecked phase; then continue the interrupted step with the full command. The implement skill's resume path says the same, pointing back to its plan-documents section. Spec, plan and repo-add resumes are unchanged apart from the rename.

*Technical detail:* [context.md#phase-22](./context.md#phase-22-make-a-resumed-implementation-read-the-plan-first)

**Acceptance criteria**:

- [ ] For an implementation interrupted at each of its steps, the resume instruction tells the agent to read the plan documents, and that instruction comes before the command to continue the interrupted step.
- [ ] The resume instruction describes the plan documents in exactly the words the start instructions use.
- [ ] The resume instruction tells the agent to determine the current phase from the plan as its first unchecked phase.
- [ ] The resume path carries only the in-progress statement, read the plan, read the working context, find the next phase and the continue command, and no other procedural steps.
- [ ] The implement skill's resume path lists the same items and refers to its plan-documents section rather than restating it.
- [ ] Resuming a spec, plan or repo-add workflow produces the same instruction as before apart from the working-context name.
- [ ] The full test suite passes.

#### - [ ] Phase 2.3: Have phase steps fetch their own detail and name the plan's context.md unambiguously

**Repo:** `spektacular`

The steps that write code, write tests and verify a phase stop assuming earlier analysis is still in the agent's context. Each works out the current phase from the plan and reads that phase's technical detail with the plan-read command. Every mention of `context.md` across the workflows' instructions and skills is also qualified as the plan's, either in words or through a path that includes the plan's name. A guard enforces this for all emitted instructions.

*Technical detail:* [context.md#phase-23](./context.md#phase-23-have-phase-steps-fetch-their-own-detail-and-name-the-plans-contextmd-unambiguously)

**Acceptance criteria**:

- [ ] The instructions for writing code, writing tests and verifying a phase each include the command to read the current phase's technical detail from the plan.
- [ ] None of those instructions says an earlier step's summaries or analysis are already available.
- [ ] Every mention of `context.md` in every emitted step instruction, resume instruction and skill is qualified as belonging to the plan, in words or by a path or command containing the plan's name.
- [ ] A new, unqualified mention of `context.md` added to any instruction fails the test suite.
- [ ] The full test suite passes, and the harbor plan and spec suites pass on a verification run.

## Open Questions

None. Every uncertainty found during planning was settled before this point.

- Mustache partial support and indentation behaviour were checked by experiment against the pinned library version.
- The test files touched by the rename were located and read.
- The harbor oracles were checked for the old path and the footer and do not reference them.
- The docs repo's affected line was pinned.

Any drift the harbor verification run turns up is ordinary verification work, not an open design question. If it does surface an oracle that encodes the old wording, update that oracle in the same change.

## Out of Scope

- **Resume behaviour of the spec, plan and repo-add workflows.** Apart from the working-context rename, these workflows resume exactly as they do today. Naming the documents to read at start, and reading them again on resume, applies only to the implement workflow (spec Non-Goals).
- **Checking that every phase's technical-detail link resolves to a matching section.** A mechanical link check at plan finish or `implement new` was considered and dropped because it does not address the resume failure (spec Non-Goals). The existing prose check in the implement `read_plan` step is unchanged.
- **Migrating, deleting or warning about an old `.spektacular/context.md`.** An old file is ignored. A run interrupted before upgrading resumes without its previous working context (spec Constraints; user decision).
- **Renaming any plan document.** The plan's `context.md`, its scaffold and existing plans keep their names and content (spec Constraints).
- **Recording the current phase in workflow state.** The phase is always re-derived from the plan's first unchecked phase. No state or JSON shape changes.
- **Rewording the footer beyond the rename.** The keep-context-current paragraph keeps its existing wording, em dashes included. Only the path changes and it moves into one place.
- **Other website pages that mention `context.md`.** Those refer to the plan document and are correct as they stand.
- **A new harbor E2E suite for an interrupted-then-resumed implement run.** Real-agent resume behaviour is covered by the manual test plan the implement workflow produces. The existing harbor plan and spec suites are only re-run for drift.
- **Fixing the skills' description of the resume report as `"resumable": true`.** The CLI actually returns a `workflow_in_progress` error envelope. This mismatch predates the spec and is not part of it. Worth a separate issue.
