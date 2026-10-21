---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Plan: 000069_compact-agents-md

<!-- Metadata -->
<!-- Created: 2026-10-21T09:53:58Z -->
<!-- Commit: b1734e5 -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Spektacular installs eight standing rules into every project's AGENTS.md. At about 22 KB, they are read in full at the start of every agent session, whether or not they matter to the task. This plan shrinks each rule to a short statement of what must always be done and when to look further. The full guidance moves to one file per rule under `.spektacular/guidance/`, which agents open only when the rule applies. `init` installs the result, and `migrate` converts and refreshes it, for Claude, Codex, Copilot, omp and Bob. Teams get lighter, cheaper sessions without the agent losing any rule. A new end-to-end suite proves agents still follow the rules, and the docs site explains the layout.

## Conventions

- **A plan never changes the active skills and configuration** (spektacular) — this feature rewrites the AGENTS.md sections and installs files under `.spektacular/`. No task may re-init or migrate this repository or regenerate its own AGENTS.md. Every check runs through the Go test suite and throwaway temp projects. The change reaches this repo only through a later local install between workflows.
- **Spektacular's own files are written through Spektacular, never with file tools** (spektacular) — the store-access rule is one of the rules being compacted, and its guarded phrases must survive. The new guidance folder is explicitly a read-only install surface, not a store, and the rule's exemption for it must not widen the three write exceptions.
- **Tests must not depend on execution order** (spektacular) — new cmd-level init/migrate tests run through `resetRootCmd`/`runRootCmd` and must pass with shuffled test order.
- **Passing tests are required before calling work done** (spektacular) — every task ends with the full Go test suite passing, the retargeted template-contract tests included.
- **Error messages must describe the problem and suggest remediation** (spektacular) — any new failure path (for example a missing short template or an unwritable guidance folder) surfaces through init/migrate with a concrete next action, never a bare error.
- **No em dashes** (docs) — all new documentation prose in the docs repo uses commas, colons or parentheses instead.
- **Plans must sketch content structure** (docs) — each docs task carries a Content outline or Content example.
- **MDX authoring conventions** (docs) — new docs content uses existing section components, slots and fenced code blocks, with no layout HTML in page bodies, and passes the site build and type check.
- **Label before filename in file-scoped reference headings** (docs) — the new configuration-page section is headed with a plain label before the file names.
- **Alternate section background shading** (docs) — the new configuration-page section sets `surface` opposite to the section before it, and the following section is re-checked.

## Architecture & Design Decisions

The eight standing rules Spektacular installs into AGENTS.md (where the code lives, store access, memory and context, knowledge trigger, spec trigger, design trigger, draft presentation, historical artifacts) each become two installed pieces. The first is a **compact section** in AGENTS.md, under the exact heading it has today. It states the binding obligation, the situation that calls for more detail, and a relative markdown link to the second piece. The second is a **guidance file** at `.spektacular/guidance/<rule>.md` holding the full guidance, written as a plain file with no `@` import, so no agent loads it until it opens the link. Both come from embedded templates in the `spektacular` repo. The existing `templates/agents/<rule>.md` stays the source of the full guidance, which the spec's Technical Approach asks for and which makes "nothing is lost" true by construction. A new `templates/agents/short/<rule>.md` holds each compact statement. All work lands in the `spektacular` repo (`internal/agent/`, `templates/agents/`, `tests/harbor/`, `Makefile`) except the documentation, which lands in the `docs` repo (`src/pages/configuration.mdx`, `src/pages/plugins.mdx`, `src/content/tutorials/getting-started.mdx`).

A single table in `internal/agent/` lists the eight rules in today's install order, each with its slug, heading, placement (repo-sources stays `placeAtTop`) and log noun. One `installStandingRules` function walks it for every agent. For each rule it renders the full template against `{{command}}` and writes it atomically to the guidance file. It then renders the short template, with `{{command}}` and a Go-supplied `{{guidance_path}}`, through the existing `installManagedSectionAt`. That function already replaces a section in place by heading, keeps content Spektacular did not install exactly where it was, and is idempotent (`internal/agent/managed_section.go:41-87`). Because `init` and `migrate` both reach the agent's `Install` (`cmd/init.go:86`, `cmd/migrate.go:76`), installing this way gives "new projects get the compact form", "existing projects convert" and "guidance is kept current" without a new migration step or settings-format bump. An upgrade simply overwrites the full section with the compact one and rewrites every guidance file. The five agents' hand-copied lists of eight installer calls collapse into calls to the shared function. The bespoke duplicate install logic in `historical_artifacts.go` moves onto the generic installer. `ensureClaudeImportsAGENTS` is untouched, so CLAUDE.md keeps importing AGENTS.md exactly as today.

Several design decisions make the compact form work in practice:

- **Headings stay byte-identical.** The installer finds a section by its heading, and a renamed heading would leave the old full text behind on upgrade.
- **The managed banner folds into the link line** ("Managed by `spektacular init`; full guidance: [path](path)"), saving roughly 200 bytes per section while still marking the section as installed. The full banner moves to the top of each guidance file.
- **The store-access rule gains an explicit read exemption** for `.spektacular/guidance/`, in both its compact form and its guidance, per the spec's Constraints. The guidance folder is a read-only install surface, like installed skills, not a store, so the store-files knowledge entry (which governs store directories and three write exceptions) is not contradicted.
- **omp rule files keep being cut from the AGENTS.md section** (`internal/agent/omp.go:44-76`). They therefore carry the identical compact text, stating the same binding rules and preserving omp's deduplication against AGENTS.md. The link's visible text is the project-root path, so it reads correctly from `.omp/rules/` too.
- **Sizes are enforced by a Go test over a fresh install.** The rendered sections must total under 5,120 bytes, each compact body may run to at most 6 lines (15 for repo-sources), prose lines may be at most 100 columns (the closing link line is exempt), and every link must resolve to an existing guidance file. Today's sections total 21,722 bytes, so this exceeds the 75% reduction metric.

This direction beats generating the short form mechanically from the full template. The binding wording and the trigger need deliberate phrasing within a hard line budget, and a mechanical extract would either break that budget or drop the "when to look further" cue. It also beats keeping eight per-rule install functions each with a parallel guidance writer, which would multiply the five copied call lists rather than remove them. Other rejected options: an `@` include, a CLI read verb, full text in omp rules, renamed headings, and relying on existing harbor suites. They are recorded with evidence in `research.md#alternatives-considered-and-rejected`.

Behaviour is proven at three layers, per the testing-architecture knowledge entry:

- **Go tests** cover install, conversion, idempotency, refresh, user-content preservation and the size budget for all five agents, through throwaway projects only. No task re-inits or migrates this repository, per the plans-never-change-the-active-install convention.
- **Template-contract tests** keep every existing phrase needle and retarget each to the compact form (for the binding part) or the guidance file (for the detail), so a needle that is found nowhere fails.
- **A new `tests/harbor/standing-rules` suite** runs `spektacular init <agent>` at image build, so AGENTS.md is loaded at session start, which no existing suite does. It verifies from the CLI session log that Claude, Codex and Bob consult the repo registry, read a spec only through the CLI, and offer rather than start a spec.

## Component Breakdown

- **Standing-rule table (new, spektacular).** The single list of the eight rules Spektacular installs, in install order. Each entry holds the rule's slug, its unchanged AGENTS.md heading, its placement (top for where-the-code-lives, end for the rest) and its log noun. Both the standing-rule installer and the omp rule writer read it, so adding or removing a rule is a one-line change instead of edits to five agent files and the omp list.

- **Standing-rule installer (new, spektacular).** Owns installing every rule for any agent. For each table entry it renders the full guidance template into the rule's guidance file under `.spektacular/guidance/`, then renders the compact template (with the command and the guidance path) into AGENTS.md through the managed-section installer. It runs on every `init` and every `migrate` reinstall, so new projects, converted projects and refreshed guidance all come from the same path. It overwrites guidance files wholesale, which keeps them current and never duplicates them.

- **Managed-section installer (existing, unchanged behaviour).** Continues to own placing one section in AGENTS.md by heading: appending, hoisting to the top, or replacing in place while leaving content Spektacular did not install exactly where it was. The compact sections reuse it as is. The historical-artifacts rule's private copy of this logic is retired in its favour.

- **Rule templates (changed, spektacular).** Each rule now has two embedded templates. The **full guidance template** is the existing text, retitled as a standalone document with the managed banner rewritten for a file rather than a section. The store-access guidance gains the guidance-folder read exemption. The **compact template** is new: the binding obligation, the trigger for reading further, and a single "Managed by init; full guidance: <link>" line, kept within the line and byte budget.

- **Agent installers (changed: claude, codex, bob, copilot, omp).** Each replaces its hand-copied sequence of eight section installers with one call to the standing-rule installer, keeping its own skills, command wrappers and extras. Claude still ensures CLAUDE.md imports AGENTS.md exactly as today.

- **omp rule writer (changed, minimally).** Still cuts each rule's section out of AGENTS.md into an always-applied omp rule file. It now iterates the shared table instead of its own list, so its rule files carry the compact statement, identical to AGENTS.md.

- **Upgrade path (`migrate`) and `init` (existing, unchanged).** Both already invoke the agent installer, which is how existing projects convert and guidance is refreshed. Neither gains a step or a settings-format change.

- **Template-contract and install tests (changed and new, spektacular).** The per-rule phrase tests now render both surfaces, and each needle is asserted on the surface that must carry it (binding parts on the compact form, detail on the guidance file). New tests cover the size budget, link resolution, the read exemption, conversion of a pre-change AGENTS.md, idempotency, refresh of outdated guidance, preservation of user content, all five agents, and the CLAUDE.md import.

- **Standing-rules harbor suite (new, spektacular).** An end-to-end suite whose image initialises the project for the agent under test before the session starts. The registered code then lives outside the starting directory, and a final spec is seeded. The agent is asked an ad-hoc code question, asked about the spec, and led through a multi-requirement feature discussion. The verifier reads the CLI session log and the agent's answer file: it checks that the registry was consulted, the spec was read through the CLI and never edited, and no spec workflow was started. It runs for Claude, Codex and Bob, alongside the existing suites, whose sources need no oracle change. The same task moves the Bob harbor agent to `tests/harbor/agents/`, where the Makefile already looks, so Bob runs of every suite can start (an epic decision shared with 000068_task-goal-validation, which relies on the move).

- **Documentation (changed, docs repo).** The configuration reference gains a section describing the compact AGENTS.md layout and the guidance folder, and its migrate section says how an upgrade converts a project. The omp plugin and getting-started text say the omp rules mirror the compact sections.

## Data Structures & Interfaces

No public interfaces, CLI commands, JSON payloads or settings keys change. The `Agent` interface (`Name`, `Install`) and the `migrate.Installer` contract stay as they are, and there is no settings-format bump. The new contracts are internal to the agent package plus two on-disk layouts.

**`standingRule` and the `standingRules` table.** This is the shared description of one installed rule, read by the installer and by the omp rule writer.

```go
type standingRule struct {
    slug      string           // "repo-sources", "store-access", ... (file and template name)
    heading   string           // exact AGENTS.md heading, unchanged from today
    placement sectionPlacement // placeAtTop for repo-sources, placeAtEnd for the rest
    noun      string           // log phrase, e.g. "Where the Code Lives section"
}

var standingRules = []standingRule{ /* eight entries, today's install order */ }

func installStandingRules(projectPath string, cfg config.Config, out io.Writer) error
```

**Template contract.** Two embedded templates per rule, each rendered with mustache:

| Template | Variables | Rendered to |
|---|---|---|
| `agents/<slug>.md` (full guidance) | `command` | `.spektacular/guidance/<slug>.md` |
| `agents/short/<slug>.md` (compact) | `command`, `guidance_path` | the rule's section in `AGENTS.md` |

`guidance_path` is the project-root-relative path `.spektacular/guidance/<slug>.md`, built in Go from the config folder name. The compact template must use it both as the link text and as the link target.

**Compact section shape** (the on-disk contract the size test and omp rely on):

```markdown
## <heading, unchanged>

<binding obligation and trigger, at most 6 lines (15 for repo-sources), prose lines 100 columns or fewer>
Managed by `<command> init`; full guidance: [<guidance_path>](<guidance_path>).   (counts as a body line; exempt from the column limit)
```

The section ends where the next `# ` or `## ` heading begins, so a compact body may not contain one.

**Guidance file shape.** A standalone markdown document: a `#` title matching the rule, a one-paragraph managed banner (written by `init`, refreshed by `migrate`, edit the Spektacular source and not the file), then the full guidance text. Spektacular overwrites it on every install, and agents only read it.

**omp rule file** (shape unchanged): `---\nalwaysApply: true\n---\n\n` followed by the AGENTS.md section text, trimmed. It now carries the compact text.

## Implementation Detail

**One table, one installer.** Today, adding a standing rule means a new wrapper file, a heading constant, an extra line in each of five agent installers, and an entry in omp's own list. After this change, a rule is one entry in the standing-rule table plus its two templates. A developer reading an agent installer sees "install skills, install wrappers, install standing rules, agent-specific extras" and nothing more. The thin per-rule wrapper files reduce to their heading constants, or fold into the table outright. The existing per-rule tests still reference the headings, so each constant is kept wherever a test or the omp writer names it. The bespoke historical-artifacts installer is deleted in favour of the generic managed-section path that every other rule already uses.

**Two renders per rule, from one source of truth.** The full guidance stays in the existing template file, so the text agents follow today is exactly what lands in the guidance file (plus a retitle, a file-oriented banner and the store-access exemption). The new compact template is short, hand-written prose. Each compact form follows one pattern: the binding obligation first ("always / never ..."), then the trigger for reading further ("before you X, read the full guidance"), then the shared "Managed by init; full guidance: <link>" line. Repo-sources keeps its "STOP" lead and the four non-negotiables inline, because it must be acted on before anything else is read.

**Install order and atomicity follow existing patterns.** Guidance files are written with the same temp-file-and-rename helper AGENTS.md uses, before the compact section that links to them, so an interrupted install never leaves a link pointing at a missing file. The guidance folder is created on demand. Writes are unconditional overwrites, matching how skills are refreshed, so re-running is byte-identical and a newer Spektacular simply replaces outdated guidance. A guidance file Spektacular no longer ships is not deleted. No rule is being removed, so this is out of scope.

**Conversion is not a special case.** A pre-change project's AGENTS.md holds full sections under the same headings. The in-place replace swaps each one for its compact form and leaves every byte of user content around it untouched. That is the same code path as a routine refresh, so there is no migration code to maintain and no separate "convert" branch to test beyond proving the path works on a pre-change fixture.

**Tests move from "the section says X" to "the right surface says X".** The per-rule phrase tests gain two render helpers, one for the compact section and one for the guidance file. Each existing needle is assigned to one surface: the binding obligation must be in the compact form, and detail may live in the guidance. New cross-cutting tests run each real agent's install into a temp project and check the budget, the links, the exemption, idempotency, conversion, refresh and user-content preservation. Fixture-based tests that substitute the template filesystem now supply the short template and guidance template for the rule under test.

**Harbor follows the epic suite's agent-independent pattern.** The new suite seeds its fixture through the CLI at image build and, unlike every existing suite, runs the agent's `init` there too, so the project instructions are in place when the session begins. Its verifier reads the CLI session log rather than a transcript, so one verifier serves Claude, Codex and Bob. The Makefile gets one target per agent, mirroring the epic suite's targets.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references (`design ref list` reports zero, none unresolved).
- **`internal/agent` managed-section installer** (spektacular): provides by-heading append, hoist and in-place replacement that keeps user content. It is reused unchanged and is the conversion mechanism.
- **`internal/agent` agent installers and the omp rule writer** (spektacular): changed to call the shared standing-rule installer and table.
- **`templates` embedded filesystem** (spektacular): it already embeds every file under `templates/` (`all:*`), so the new `agents/short/` templates need no embed change.
- **`github.com/cbroglie/mustache`** (external, already a dependency): renders both templates. No version change.
- **`internal/config`** (spektacular): provides the `.spektacular` folder name used to build the guidance path. Unchanged.
- **`cmd` init and migrate, `internal/migrate` engine** (spektacular): both already call the agent installer on install or skills-version mismatch. Unchanged, and relied on for conversion and refresh.
- **Harbor tooling** (`harbor` CLI, Docker, agent credentials) and `tests/harbor/shared/sessionlog.py` (spektacular): the new standing-rules suite is built on them. Existing suite oracles need no change, because no suite source asserts AGENTS.md text.
- **Docs site toolchain** (docs repo: Astro 5, `@astrojs/mdx`, existing section components): it hosts the documentation updates. No new components are expected.
- **Sibling spec 000068_task-goal-validation** (same epic): it also edits the per-agent installers to add a reviewer helper. Neither spec must land first, but whichever lands second rebases onto the other's agent-installer changes. If 000068 adds a standing rule, that rule must be added as a table entry with short and full templates.
- **Knowledge entries relied on:** `architecture/testing-architecture.md` (harbor coupling and runs), `conventions/plans-never-change-the-active-install.md` (no re-init of this repo), and `conventions/store-files-must-be-written-through-the-cli.md` (the store rule being compacted). No change to them is required. An optional clarifying note about the guidance folder is listed as a discovery for the user.

## Testing Approach

Testing follows the project's three layers (testing-architecture knowledge entry). Most coverage sits in the Go layers, because install, conversion and size are deterministic. The harbor layer carries the one thing Go cannot prove: that agents still behave under the compact text.

**Go unit and integration tests (agent package and cmd).** These are the bulk of the coverage, on the standing-rule installer and the agent installers.

- **Budget and links.** A fresh install for every one of the five agents yields compact sections totalling under 5,120 bytes. Each compact body is at most 6 lines (15 for where-the-code-lives), every prose line is 100 columns or fewer (the closing link line is exempt), and every section links to a guidance file that exists, with a path under `.spektacular/guidance/`.
- **Every agent.** Claude, Codex, Copilot, omp and Bob each produce the compact AGENTS.md and all eight guidance files. omp's rule files equal the AGENTS.md sections, so they state the same binding rules and carry no full text.
- **CLAUDE.md unchanged.** It still imports AGENTS.md with the same line.
- **Conversion.** A fixture AGENTS.md holding today's full sections, with user content before, between and after them, converts to the compact form through both `init` and a versioned `migrate` reinstall. Every user byte stays unchanged and in place, and there is exactly one section per heading.
- **Idempotency.** Running `init` twice, or `migrate` after `init`, leaves AGENTS.md and every guidance file byte-identical to a single run.
- **Refresh.** Guidance files deliberately made stale (or deleted) are restored to the shipped content on the next reinstall.
- **Not eager.** No installed instruction file (AGENTS.md, CLAUDE.md, omp rules) contains an `@` import of a guidance file or any guidance file's body text.

**Template-contract tests.** These turn "nothing is lost" and "binding part is inline" into assertions.

- **Needles keep a home.** Every existing hand-maintained phrase needle for the eight rules is kept and assigned to a surface. Needles naming the binding obligation (never write store files with file tools, run `repo list` before code work, offer and never start a spec, never write knowledge unprompted, and so on) are asserted on the compact section. Detail needles are asserted on the guidance file.
- **Every requirement still found.** A new test asserts that each guidance file contains the full text of its rule (the rendered full template). A further assertion requires each compact section to name its trigger, using a hand-maintained per-rule trigger phrase list.
- **Exemption present.** The store-access rule's guidance-folder read exemption is asserted on both surfaces. The existing forbidden-substring guards automatically cover the new short templates.

**End-to-end (harbor).** A new standing-rules suite runs for Claude, Codex and Bob. Its image initialises the project before the session, so the compact AGENTS.md is the agent's real project instructions. The verifier asserts from the CLI session log and the agent's answer file that the agent:

- ran `repo list` and answered a code question with a fact only present in the registered root
- read the seeded spec through `spec file read` and left it unchanged
- never ran `spec new` during the feature discussion, while its answer offers to capture a spec

Existing suites need no oracle changes. They are re-run to show pass rates do not drop.

**Deliberate gaps.**

- Copilot and omp have no harbor harness, so their behaviour under the compact text is a manual check.
- The not-eager criterion is proven structurally (no `@` import, no inlined text) rather than by inspecting a live agent's context. That inspection is a manual check for Claude.
- Docs pages are checked by the docs repo's build guards rather than by tests.

**Success metrics.**

- *Tokens spent on standing instructions fall by at least 75%.* **Behavioural test**: the byte-budget test pins the rendered sections under 5,120 bytes against a 21,722-byte baseline (above 76%). **Manual — captured in the implementation test plan**: confirm the reduction in a live Claude session's context accounting (for example `/context` before and after) for one initialised project.
- *No end-to-end suite's pass rate drops for any agent.* **Manual — captured in the implementation test plan**: run every harbor suite (spec, plan, implement, repo, epic, implement-epic, and the new standing-rules suite) for each harbor-supported agent, before and after the change, and compare pass rates.

**Manual reviews and checks.** Each of these is **Manual — captured in the implementation test plan**:

- A harbor baseline-and-after run of all suites for Claude, Codex and Bob (the pass-rate metric above).
- A Copilot CLI session and an omp session in a freshly initialised throwaway project. Each must consult `repo list` before code work, read a spec only through the CLI, and offer rather than start a spec.
- A rule-by-rule human read of each compact section beside its guidance file. It confirms the binding obligation is inline, the trigger is clear, and no instruction from today's full text is missing.
- A live check that a new Claude session in an initialised project does not show guidance file content until the agent opens one.
- A docs review: the rendered configuration and plugins pages describe the compact layout, the guidance folder and how migrate converts a project, and the docs build guards pass.

## Milestones & Tasks

### Milestone 1: An end-to-end check proves agents follow the standing rules from their project instructions

**What changes**: Spektacular gains an end-to-end suite in which the agent starts its session in a project that is already initialised, so the standing rules in AGENTS.md are its real project instructions. That is not true of any suite today. The suite checks that Claude, Codex and Bob find the code through the repo registry, read a spec only through the CLI, and offer rather than start a spec during a feature discussion. Landing it before the rules change gives a baseline pass rate for the full-text rules, so the compact form can be compared against it. Users see no change yet.

**Validation point**: The suite builds and runs for each of Claude, Codex and Bob against the current full-text rules, and its verifier's self-check tests pass. The baseline results are recorded for comparison in the implementation test plan. The full Go test suite passes.

#### - [ ] Task: Add the standing-rules harbor suite
**Id:** 60cf1809-b0f5-4cd1-bbe5-de5327c59509
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Add a new end-to-end harbor suite in which the project is initialised for the agent under test while the image is built, so AGENTS.md (and CLAUDE.md for Claude) are the agent's real project instructions when the session starts. The project registers a code repository that lives outside the starting directory and holds a final spec. The agent is given three plain requests: answer a question about the code, summarise the spec, and discuss a multi-requirement feature idea. The verifier checks, from the CLI's own session log and the agent's answer file, that the agent used the repo registry, read the spec only through the CLI, and offered a spec rather than starting one. One Makefile target per agent (Claude, Codex, Bob) runs it, mirroring the epic suite. The task also moves the Bob harbor agent to the shared location the Makefile already expects, so Bob runs of this suite and every existing suite can start; sibling spec 000068_task-goal-validation relies on this move.

*Technical detail:* [context.md#task-add-the-standing-rules-harbor-suite](./context.md#task-add-the-standing-rules-harbor-suite)

**Acceptance criteria**:
- [ ] The suite's image starts the agent in a project already initialised for that agent, with a registered repository outside the starting directory and a seeded final spec.
- [ ] The verifier passes only when the agent consulted the repo registry and answered the code question with a fact that exists only in the registered repository.
- [ ] The verifier passes only when the spec was read through the CLI and is unchanged at the end of the run.
- [ ] The verifier passes only when no spec workflow was started and the agent's answer offers to capture the feature as a spec.
- [ ] Self-check tests show each verifier rule fails on a deliberately non-compliant session log and passes on a compliant one, without running an agent.
- [ ] Claude, Codex and Bob each have a make target for the suite, and existing suites' sources are unchanged.
- [ ] The Bob harbor agent lives in the shared location the Makefile expects, and Bob runs of the new suite and the existing suites find it.

### Milestone 2: Every initialised or upgraded project gets compact standing rules with guidance on demand

**What changes**: Running `spektacular init` for any of Claude, Codex, Copilot, omp or Bob now writes each standing rule into AGENTS.md as a short statement. Each statement gives the obligation that always applies and when to read further, with a link to the full guidance in `.spektacular/guidance/`. Spektacular's part of AGENTS.md drops from about 22 KB to under 5 KB. Running `spektacular migrate` after upgrading converts an existing project's full sections in place and installs or refreshes the guidance files. Content the team wrote in AGENTS.md stays exactly where it was. omp's always-applied rule files carry the same short statements. CLAUDE.md keeps importing AGENTS.md as before, and the store-access rule now says the guidance files may be read directly. Internally, the five agents share one table-driven installer instead of five copied lists. That cleanup is folded into this milestone because it is what makes the change land once for all agents.

**Validation point**: In throwaway projects, `init` for each agent produces compact sections under 5,120 bytes with every link resolving to a guidance file. A project carrying today's full sections and its own content converts through `migrate` with the user content untouched. Re-running is byte-identical, stale guidance is refreshed, and every pre-existing phrase needle is found on the compact or guidance surface. The full Go test suite passes. No step touches this repository's own AGENTS.md or `.spektacular/`.

#### - [ ] Task: Share one standing-rule installer across every agent
**Id:** b8dee74c-23b3-4a0a-bc94-db43b09ff41f
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Replace the five hand-copied lists of eight section installers (Claude, Codex, Bob, Copilot, omp) with one table of the standing rules and one installer that walks it. omp's rule writer reads the same table instead of its own list. The historical-artifacts rule moves off its private copy of the section-placement logic onto the shared path. This is an internal cleanup with no change to any installed file. It makes the compact change land once for all agents and turns any future rule into a single table entry.

*Technical detail:* [context.md#task-share-one-standing-rule-installer-across-every-agent](./context.md#task-share-one-standing-rule-installer-across-every-agent)

**Acceptance criteria**:
- [ ] For every agent, an install into an empty project and into a project with existing content produces byte-identical files to the install before the change.
- [ ] Every agent installs the eight rules in the same order as before, with where-the-code-lives still placed at the top.
- [ ] omp still writes eight always-applied rule files, each equal to its AGENTS.md section.
- [ ] Adding a rule requires only a new table entry and its template.

#### - [ ] Task: Install compact sections and guidance files
**Id:** e72523c4-3135-4984-af96-5164147515d4
**Repo:** spektacular
**Depends on:**
- b8dee74c-23b3-4a0a-bc94-db43b09ff41f — Share one standing-rule installer across every agent
- 60cf1809-b0f5-4cd1-bbe5-de5327c59509 — Add the standing-rules harbor suite
**Execution:** agent

Give each of the eight rules a short template stating its binding obligation, the situation in which to read further, and a link to its guidance file. The existing full template becomes a standalone guidance document. The shared installer now writes every guidance file under `.spektacular/guidance/` and puts the short form into AGENTS.md under the unchanged heading. The store-access rule, in both forms, says the guidance files may be read with the agent's own file tools. Every existing phrase test is kept and pointed at the surface that must carry its phrase, so no instruction can silently disappear.

*Technical detail:* [context.md#task-install-compact-sections-and-guidance-files](./context.md#task-install-compact-sections-and-guidance-files)

**Acceptance criteria**:
- [ ] After installing, each rule appears in AGENTS.md as a short statement under its existing heading, naming when to read further and linking to its guidance file.
- [ ] Each rule's guidance file exists under `.spektacular/guidance/` and holds the full text of today's rule, with no instruction removed.
- [ ] The obligation an agent must always honour is stated in the short statement for every rule.
- [ ] The store-access rule tells agents, in both forms, that guidance files are read directly and never edited.
- [ ] No installed instruction file imports a guidance file or contains its body.
- [ ] Every phrase previously asserted on a rule is still asserted, on the short statement or on the guidance file.
- [ ] CLAUDE.md still imports AGENTS.md exactly as before.

#### - [ ] Task: Prove conversion, refresh and the size budget for every agent
**Id:** a1f2f163-775d-496b-8096-62db7e3ce6ee
**Repo:** spektacular
**Depends on:**
- e72523c4-3135-4984-af96-5164147515d4 — Install compact sections and guidance files
**Execution:** agent

Add tests over real `init` and `migrate` runs in throwaway projects for Claude, Codex, Copilot, omp and Bob. They show that Spektacular's part of AGENTS.md is under 5 KB and within the per-rule line limits, that every link resolves, and that a project carrying today's full sections converts in place. They also show that user content around the sections survives unchanged, that running twice changes nothing, and that stale or missing guidance is restored. These tests pin the spec's acceptance criteria so a later edit cannot quietly break them. The tests use temporary projects only and never touch this repository's own setup.

*Technical detail:* [context.md#task-prove-conversion-refresh-and-the-size-budget-for-every-agent](./context.md#task-prove-conversion-refresh-and-the-size-budget-for-every-agent)

**Acceptance criteria**:
- [ ] For each of the five agents, a fresh initialisation leaves Spektacular's AGENTS.md sections under 5,120 bytes. Each short statement is within 6 lines (15 for where-the-code-lives) and links to a guidance file that exists.
- [ ] A project initialised before this change, with its own content before, between and after Spektacular's sections, upgrades to the compact form with that content unchanged and in place.
- [ ] Running initialisation or upgrade twice leaves AGENTS.md and every guidance file identical to running it once.
- [ ] After an upgrade from an older Spektacular version, every outdated or missing guidance file matches the shipped content.
- [ ] omp's rule files hold the same short statements as AGENTS.md.

### Milestone 3: The documentation site explains the compact layout and how upgrading converts a project

**What changes**: The documentation website's configuration reference describes the compact AGENTS.md layout: what each short statement holds, where the guidance files live, that agents read them on demand, and that they are regenerated rather than edited. Its upgrade section says `migrate` converts an existing project's full sections and refreshes the guidance. The omp integration and getting-started pages say the omp rules mirror the compact sections.

**Validation point**: The docs site builds and type-checks cleanly, the page-body layout guard finds no layout HTML, no em dashes are introduced, and the rendered pages describe the layout, the guidance folder and the upgrade conversion.

#### - [ ] Task: Document the compact layout on the docs site
**Id:** 3adb6bae-2971-4422-9424-c224f3e527af
**Repo:** docs
**Depends on:**
- e72523c4-3135-4984-af96-5164147515d4 — Install compact sections and guidance files
**Execution:** agent

Add a section to the configuration reference that explains the compact AGENTS.md layout. It covers what each short statement holds, that the full guidance lives in `.spektacular/guidance/`, one file per rule, that agents open it only when a rule's situation arises, and that the files are regenerated rather than edited. Extend the page's upgrade section to say `migrate` converts an existing project's full sections and refreshes the guidance while leaving the team's own AGENTS.md content alone. Update the omp integration and getting-started wording to say the omp rules mirror the compact sections.

**Content outline** (new configuration-page section "Standing rules: AGENTS.md and .spektacular/guidance/", placed after the migrate section; wording illustrative, facts fixed):
- *Sub*: "Spektacular installs eight standing rules for your agent. Each sits in your root `AGENTS.md` as a short statement, and its full guidance lives in its own file that the agent opens only when the rule applies."
- *What a short statement holds*: "Each rule keeps its heading in `AGENTS.md`. Below it is the obligation the agent must always honour, when to read further, and a link such as `[.spektacular/guidance/spec-trigger.md](.spektacular/guidance/spec-trigger.md)`. Spektacular's part of `AGENTS.md` stays under 5 KB."
- *Where the guidance lives*: a fenced tree of `.spektacular/guidance/` listing `repo-sources.md`, `store-access.md`, `memory-context.md`, `knowledge-trigger.md`, `spec-trigger.md`, `design-trigger.md`, `draft-presentation.md` and `historical-artifacts.md`, then: "Commit the folder so the links resolve for everyone. Agents read these files with their own tools when a rule sends them there; they are never imported into a session up front."
- *Do not edit them*: "`init` writes the guidance files and `migrate` rewrites them for each new Spektacular version, so local edits are lost. Put your own rules in your own sections of `AGENTS.md`, which Spektacular never changes."
- *Agent notes*: "Claude reads `AGENTS.md` through the `@AGENTS.md` import in `CLAUDE.md`. Codex, Copilot and Bob read it directly. omp's always-applied `.omp/rules/spek-*.md` files carry the same short statements."

**Content example** (migrate section, new bullet in "One run:"): "converts the standing rules in `AGENTS.md` to their short form and installs or refreshes the guidance files in `.spektacular/guidance/`, leaving any content you added to `AGENTS.md` where it was"

**Content example** (omp plugin and getting-started text): "The rules mirror the short standing-rule sections of root `AGENTS.md`, each linking to its full guidance in `.spektacular/guidance/`."

*Technical detail:* [context.md#task-document-the-compact-layout-on-the-docs-site](./context.md#task-document-the-compact-layout-on-the-docs-site)

**Acceptance criteria**:
- [ ] The configuration page has a section describing the compact layout, the guidance folder and that guidance is read on demand.
- [ ] The upgrade section says how `migrate` converts an existing project and refreshes guidance without touching the team's own content.
- [ ] The omp integration and getting-started text describe the omp rules as mirroring the compact sections.
- [ ] The site builds and type-checks cleanly, the page bodies contain no layout HTML, and the new prose uses no em dashes.

## Open Questions

- **Does a live agent follow a compact rule's trigger and open the guidance file when the situation arises?** This depends on each agent's real behaviour under the compact text, which only shows up in the harbor and manual runs. If an agent in the standing-rules suite or a manual Copilot or omp session breaks a rule that it honoured in the baseline run, STOP and ask the user before rewording. The fix is to strengthen the compact statement's binding wording (within the budget), never to move binding text back into the guidance file alone or to raise the size budget unasked.
- **Does omp still deduplicate the compact rule files against AGENTS.md?** The existing code assumes exact-text deduplication, which can only be observed in a live omp session. If omp shows each compact rule twice, record it in the test plan result and continue: the rules are short, and identical text is still the spec's "same binding rules". Only STOP and ask if omp loads guidance file content eagerly.

No other questions remain open. Everything else was resolved in planning and recorded in the assumption log.

## Out of Scope

- **Restructuring the workflow skills or step templates.** They are already loaded on demand (spec Non-Goals).
- **Converting this repository's own AGENTS.md and `.spektacular/`.** Per the plans-never-change-the-active-install convention, this repo picks up the compact form only when a person installs the new binary locally between workflows and the next version check migrates it.
- **Deleting guidance files Spektacular no longer ships.** No rule is being removed, so stale-file cleanup in `.spektacular/guidance/` is left until one is.
- **Restoring a deleted guidance file between releases.** `migrate` reinstalls only when the installed skills version differs from the binary, and `init` always reinstalls. A user who deletes a guidance file and stays on the same version restores it by re-running `init`. A version-check hook for missing guidance is not added.
- **Harbor harnesses for Copilot and omp.** Neither agent has harbor arguments today, so their behaviour under the compact rules is a manual check. Building adapters is separate work.
- **Changing the wording or substance of any rule's full guidance**, beyond retitling it as a standalone file, rewriting its managed banner, and adding the store-access read exemption.
- **A spektacular README change.** Its agent descriptions stay accurate, and the website carries the new documentation.
- **Updating the store-files knowledge entry.** The guidance folder is not a store, so the entry stays correct. An optional clarifying note is offered to the user as a discovery, not planned as a task.
