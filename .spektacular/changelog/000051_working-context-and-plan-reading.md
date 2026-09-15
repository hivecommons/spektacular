---
created_date: "2026-09-15"
status: completed
closed_date: "2026-09-15"
---

# Working context gets its own name, and resumed implementations read the plan first

## What was built

**A distinct name for session notes.** The notes an agent keeps as it works now live in `.spektacular/working-context.md`, not `.spektacular/context.md`. No plan document shares that name any more. Every workflow step, the resume instructions, the four workflow skills, the historical-artifacts section installed into `AGENTS.md`, and the website tutorial that quotes the instruction all use the new name. An old `.spektacular/context.md` from an earlier version is ignored: never read, moved or reported. The plan's own `context.md` keeps its name, so existing plans still read unchanged.

**One keep-context-current footer.** The "Before you advance: refresh your working context" paragraph was copied into 45 step templates. It now lives in one fragment, and the shared step renderer appends it to every step that continues to another step. Finished steps get none. The first spec step, which used to lack the footer and showed its next command without the command prefix, now matches every other step.

**Shared template fragments.** Both the runtime step renderer and the skill installer now resolve mustache partial includes from the embedded templates. A missing fragment fails the render instead of rendering empty.

**Implementations read the plan at start and on resume.** One fragment describes the three plan documents: `plan.md` (the approved plan and phase checklist), the plan's `context.md` (per-phase technical detail) and `research.md` (the decision log), with the command to read each. The implement skill's start instructions, the `read_plan` step and a new implement-only resume instruction all show it. When an interrupted implementation is found, the resume report now tells the agent, in order: read the plan documents, read the working context, find the current phase as the plan's first unchecked phase, then continue the interrupted step. The skill's resume path says the same. Spec, plan and repo-add resumes are unchanged apart from the rename.

**Phase steps fetch their own detail.** The steps that write code, write tests and verify a phase each read the current phase and its technical detail from the plan themselves. They no longer assume an earlier step's analysis is still in context. Every agent-facing mention of `context.md` now makes clear it is the plan's.

**Guards.** New template-contract tests render every step of every workflow, both resume instructions, the installed skills, the helper skills and the managed `AGENTS.md` sections. They check that:
- every continuing step ends with the identical footer
- every `goto` command carries the prefix
- the old path appears nowhere
- the implement resume reads the plan before continuing, at every step
- start and resume describe the plan documents identically
- no `context.md` mention is unqualified

## Why it matters

An agent resuming an interrupted implementation could mistake its own session notes for the plan's technical detail, because both lived in files named `context.md` and nothing on resume told it to read the plan. It then reported that phases the plan clearly described were never planned, and offered to invent the missing detail. With distinct names, an explicit read-the-plan-first resume, and phase steps that fetch their own detail, resumed work continues from the plan the team approved.

## Deviations from the plan

- The harbor plan and spec E2E suites were not run during implementation, because the Docker daemon was unavailable. They are recorded as outstanding verification in the implementation test plan, and that half of Phase 2.3's last acceptance criterion stays unchecked.
- No separate unit test was added for the runtime `{{command}}` variable in Phase 1.1. It is covered from Phase 2.1 onward by the `read_plan` step test, once a fragment uses it.
- The planned extension of the start-instructions test to the resume instruction was dropped as redundant. The per-step resume test already compares the resume against the same rendered fragment.
- Helper skills served by `skill <name>` are returned raw rather than rendered, so the contract tests fetch them through the real command path.
