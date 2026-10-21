---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Research: 000069_compact-agents-md

## Alternatives considered and rejected

### Option A: Keep full text in AGENTS.md, add `@` imports for detail

**Rejected**: The spec's Constraints forbid an eager include (Claude Code's `@file` import inlines the file into every session, which is exactly the cost being removed). `internal/agent/claude.go:57` shows `@AGENTS.md` is already how CLAUDE.md pulls AGENTS.md in; a second `@` layer would re-inline the 22 KB.

### Option B: Serve full guidance through a CLI verb (e.g. `spektacular guide <rule>`)

**Rejected**: The spec's Constraints require the guidance to be files under `.spektacular/` that agents read with their own file tools, with an explicit exemption in the store-access rule. A CLI verb would also need gate/error/remediation plumbing (`cmd/migrate.go:17` gate annotations) for no benefit.

### Option C: Rename the managed section headings for the compact form

**Rejected**: `locateManagedSection` (`internal/agent/managed_section.go:96`) finds each section by its exact heading line. A renamed heading would leave the old full section in place on upgrade and append a second one, breaking "Existing projects convert" and "Re-running is idempotent". Headings stay byte-identical.

### Option D: Give omp rule files the full guidance text

**Rejected**: `installOMPRules` (`internal/agent/omp.go:44-76`) copies each AGENTS.md section into `.omp/rules/spek-<name>.md` so omp's content deduplication can suppress the rule when AGENTS.md already carries it (comment at `omp.go:41-44`). A full-text rule would no longer match the AGENTS.md section, defeating dedup and re-loading ~22 KB of always-applied text in every omp session, contradicting the spec's goal.

### Option E: Write omp rules with `../../.spektacular/guidance/...` links

**Rejected**: For the same dedup reason: the rule body must equal the AGENTS.md section. The visible link text is the project-root-relative path, which is what agents act on.

### Option F: One combined guidance file

**Rejected**: The Technical Approach asks for one file per section in a single folder, and a combined file would force reading every rule's detail to reach one.

### Option G: Store guidance next to skills (e.g. `.claude/guidance/`)

**Rejected**: The spec's Constraints put it under `.spektacular/`, and per-agent folders would need five copies.

### Option H: Rely only on the existing harbor suites for the "agents still follow the rules" criterion

**Rejected**: Every existing suite's agent runs `spektacular init` inside the session (`tests/harbor/spec-workflow/instruction.md:9-12`), so AGENTS.md does not exist when the session starts and the standing rules are never loaded as project instructions; and no verifier asserts `repo list` use or offer-not-start (grep of `tests/harbor/*/tests/test_*.py`). A new suite whose image runs `init` at build time is the only way to exercise the compact form end to end.

## Chosen approach — evidence

- `internal/agent/managed_section.go:41-87`: generic `installManagedSectionAt` already renders a template, replaces the section in place by heading, preserves surrounding user content, and is idempotent. Rendering a short template through it converts an existing full section in place, with no new migration step.
- `cmd/migrate.go:66-78` + `internal/migrate/engine.go:159-195`: `migrate` reinstalls via `agent.Install` whenever `skills_version` differs from the binary version, so writing the guidance files inside `Install` makes `init` and `migrate` both install and refresh them.
- `internal/agent/{claude,codex,bob,copilot,omp}.go`: each agent calls the same eight section installers in the same order. One shared table-driven installer removes five copies of that list.
- `internal/agent/historical_artifacts.go:26-111`: a bespoke copy of the generic locate/append/replace logic. It can move onto the generic installer with no behaviour change.
- `internal/agent/omp.go:44-76`: omp rules are built from the AGENTS.md section text, so they automatically carry the compact form once AGENTS.md does.
- `internal/agent/skills.go:41`: `sourceFS` is substitutable in tests (`withSourceFS`), and fixtures (`spec_trigger_test.go:20-26`) must now provide the short and full template paths.
- `internal/agent/store_access_test.go:62-190`, `design_trigger_test.go:161-360`, `repo_sources_test.go:193-194`: hand-maintained phrase needles over the rendered section. Retargeting each needle to the short form or the guidance file is a direct "nothing is lost" proof.
- `internal/agent/instruction_surface_test.go:73`: `instructionTemplateRoots` already walks `agents/` recursively, so the new short templates are covered by the forbidden-substring guards.
- `config.ProjectConfigDirName = ".spektacular"` (`internal/config/config.go:297`) and `ProjectConfigDir` (`:303`) give the guidance folder's base.
- `tests/harbor/epic-workflow/environment/seed/seed.sh` + `tests/harbor/shared/sessionlog.py`: a session-log-based verifier is agent-independent (Claude, Codex, Bob), which is the model for the new standing-rules suite.

## Files examined

- spektacular:templates/agents/*.md: eight section templates, 21,722 bytes in total (design-trigger 4,905; spec-trigger 3,889; historical-artifacts 3,654; store-access 3,156; repo-sources 2,083; knowledge-trigger 2,046; draft-presentation 1,311; memory-context 678).
- spektacular:internal/agent/managed_section.go:1-260: placement (end, top), locate by heading up to the next `# `/`## `, atomic write.
- spektacular:internal/agent/repo_sources.go:23: repo-sources uses `placeAtTop`.
- spektacular:internal/agent/historical_artifacts.go:26-111: duplicate install logic.
- spektacular:internal/agent/{spec_trigger,store_access,memory_context,knowledge_trigger,design_trigger,draft_presentation}.go: thin wrappers, each with a template path and heading constant.
- spektacular:internal/agent/{claude,codex,bob,copilot,omp}.go: per-agent install order; claude also runs `ensureClaudeImportsAGENTS` (`claude.go:61-97`), which must not change.
- spektacular:internal/agent/omp.go:44-76: omp rules copy AGENTS.md sections behind an `alwaysApply` frontmatter.
- spektacular:internal/agent/skills.go:41-71: `sourceFS`, skill install pattern.
- spektacular:internal/agent/*_test.go: per-section create/append/idempotent/preserve/template-change/cross-agent tests, and phrase needles.
- spektacular:internal/agent/omp_test.go:36-90: omp rule = AGENTS.md section, refreshed on reinstall, 8 rules.
- spektacular:cmd/init.go:55-108: init runs migrate (settings only), then `a.Install`, then stamps `skills_version`.
- spektacular:cmd/migrate.go:66-78, internal/migrate/engine.go:159-195,403-412: reinstall on version mismatch; a `dev` binary always reads as "match".
- spektacular:cmd/omp_test.go:11-46, cmd/migrate_test.go:232-286, cmd/init_test.go:374-421: end-to-end init/migrate tests to extend.
- spektacular:tests/harbor/*: no suite source asserts AGENTS.md text; the epic suites run `init claude` at image build only to borrow the config; Makefile:13-15,191-207 shows the agent args and build pattern.
- spektacular:CHANGELOG.md: last root entry is 000059; later features were recorded only through the changelog store.
- docs:src/pages/configuration.mdx:538-590: "Upgrading a project: migrate" section, bullet list of what one run does.
- docs:src/pages/plugins.mdx:69-90: the omp plugin text says the rules mirror Spektacular's AGENTS.md sections.
- docs:src/content/tutorials/getting-started.mdx:226-238: omp install description.
- docs: no page currently describes the standing rules or AGENTS.md sections.

## External references

- Claude Code memory docs (`@path` imports in CLAUDE.md are inlined at session start): this is why the guidance must use plain markdown links, not `@`.
- omp rule files (`alwaysApply: true` frontmatter, content deduplication against AGENTS.md), as documented in the comment at `internal/agent/omp.go:41-44`: this is why omp rules must stay byte-identical to the AGENTS.md section.

## Prior plans / specs consulted

- 000066_epic-mid-run-revisions (plan): harbor runs are recorded as manual checks in the Testing Approach rather than as tasks. Docs tasks carry content examples. This plan follows the same pattern.
- Knowledge `architecture/testing-architecture.md`: harbor oracles must be updated in the same change as the surface they mirror, and verification includes a harbor run of each touched suite.
- Knowledge `conventions/plans-never-change-the-active-install.md`: no task may re-init or migrate this repo or regenerate its AGENTS.md. Verify with `go test` and throwaway projects only.
- Knowledge `conventions/store-files-must-be-written-through-the-cli.md`: covers store directories and the three write exceptions. The guidance folder is a read-only install surface like installed skills, not a store.

## Open assumptions

- The "under 5 KB" figure is measured in bytes of the rendered Spektacular sections in AGENTS.md (headings included), and bytes stand in for tokens in the ≥75% success metric. If tokens must be measured with a tokenizer, STOP and ask.
- The "at most 6 lines" limit counts the non-blank lines below the `##` heading as rendered, the link line included. Prose lines are hard-wrapped at 100 columns or fewer, and the link line is exempt.
- omp deduplicates rule content against AGENTS.md by exact text (per the existing code comment). If omp's dedup turns out to work otherwise, the rule-file decision still holds (same binding text) but the dedup benefit should be re-checked.
- Claude, Codex, Copilot, Bob and omp do not auto-load arbitrary `.md` files under `.spektacular/guidance/`. Each reads only its own instruction-file names.
- The harbor session log records every CLI invocation, including `repo list` and `spec file read`.

## Drafting assumptions

### Chosen direction: table-driven compact sections plus guidance files (architecture)
- **Decision**: add a short template per rule (`templates/agents/short/<rule>.md`) and keep the existing full template as the guidance source. One table-driven `installStandingRules`, shared by all five agents, writes `.spektacular/guidance/<rule>.md` and installs each compact section through the existing in-place managed-section installer. Conversion and refresh ride on `Install`, which both `init` and `migrate` already call. There is no new migration step and no settings-format bump.
- **Key design decisions**: headings unchanged (so sections are replaced in place); banner folded into the link line; a `{{guidance_path}}` template variable supplied by Go; omp rules stay cut from AGENTS.md (compact and identical); a Go size-budget test (<5,120 bytes, at most 6 or 15 body lines including the link line, prose lines at most 100 columns, links resolve); every existing phrase needle retargeted to the short or guidance surface; and a new session-log-verified harbor standing-rules suite.
- **Rejected**: generating short forms mechanically from the full template (can't hold binding wording within the line budget); a parallel per-rule guidance writer in each of eight files (multiplies duplication); plus the alternatives listed in research.md.

### Conventions selected (architecture)
- **Decision**: apply five spektacular conventions (active-install, store-files, test order, tests-pass, error remediation) and five docs conventions (no em dashes, content skeletons, MDX authoring, label-before-filename headings, alternate shading). The glossary is used for vocabulary only.
- **Rationale**: each bears directly on a surface this plan touches (agent templates and install, cmd tests, docs pages).
- **Rejected**: none deliberately dropped. Every loaded convention touches this work.

### Guidance folder is `.spektacular/guidance/` (discovery)
- **Decision**: install one guidance file per rule at `.spektacular/guidance/<rule>.md`, committed with the project (not gitignored).
- **Rationale**: the spec requires a single folder under `.spektacular/`. "guidance" is the spec's own word and clashes with no store or config name. It must be committed so every teammate's AGENTS.md links resolve.
- **Rejected**: `.spektacular/rules/` (easily confused with `.omp/rules/`); a gitignored folder (links would break for teammates).

### Section headings stay byte-identical (discovery)
- **Decision**: keep every managed `##` heading exactly as it is today, the em dash in "Where the Code Lives — Read This First" included.
- **Rationale**: the installer finds a section by its heading. Changing a heading would duplicate sections on upgrade rather than convert them.
- **Rejected**: shorter headings to save bytes.

### omp rules carry the compact text, identical to AGENTS.md (discovery)
- **Decision**: `.omp/rules/spek-<rule>.md` keeps being built from the AGENTS.md section, so it now carries the compact statement and the guidance link (link text is the project-root path).
- **Rationale**: this preserves omp's deduplication against AGENTS.md, keeps always-applied context small, and states the same binding rules (spec constraint).
- **Rejected**: full text in omp rules (defeats the size goal and dedup); rule-relative `../../` links (breaks dedup).

### Size and line limits are measured in bytes and 100-column lines (discovery)
- **Decision**: "under 5 KB" means fewer than 5,120 bytes across the rendered Spektacular sections. "At most 6 (15) lines" counts every non-blank line below the heading, the closing link line included. Prose lines are hard-wrapped at 100 columns or fewer, and the link line is exempt from the column limit because the guidance path alone runs past it. A draft of all eight sections measures 4,228 bytes, with 12 body lines for repo-sources and at most 6 for the rest. Bytes stand in for tokens in the 75% metric.
- **Rationale**: deterministic and checkable in a Go test. Byte and token ratios track closely for English markdown.
- **Rejected**: tokenizer-based measurement (adds a dependency, and the result varies by agent).

### New harbor standing-rules suite (discovery)
- **Decision**: add a small `tests/harbor/standing-rules` suite whose image runs `spektacular init <agent>` at build time, so AGENTS.md is loaded at session start. It is verified from the session log (and the answer file), for Claude, Codex and Bob.
- **Rationale**: no existing suite loads AGENTS.md at session start or checks repo-registry use or offer-not-start. Without a new suite the "agents still follow the rules" criterion is unverifiable.
- **Rejected**: relying on existing suites alone; a manual-only check.

### Guidance file read exemption is not a store-rule contradiction (discovery)
- **Decision**: the guidance folder is treated as a read-only install surface (like installed skills), not a store. The store-access rule gains an explicit read exemption for it, as the spec's Constraints require. The knowledge entry on store files is not contradicted, since it governs store directories and its three exceptions are write surfaces.
- **Rationale**: the entry enumerates stores (specs, plans, knowledge, designs and so on). Guidance files are none of these.
- **Rejected**: adding a CLI read verb (the spec rules it out).

### Byte budget stands in for the token metric (testing_approach)
- **Decision**: the 75% token metric is asserted automatically as a byte budget (under 5,120 bytes against 21,722) and confirmed manually once with Claude's context accounting.
- **Rationale**: a deterministic byte check runs in CI. Token and byte ratios track closely for English markdown.
- **Rejected**: embedding a tokenizer in tests.

### Copilot and omp verified manually (testing_approach)
- **Decision**: Copilot and omp behaviour under the compact text is a manual check, and the harbor standing-rules suite covers Claude, Codex and Bob.
- **Rationale**: the Makefile has harbor agent arguments only for claude, codex and bob.
- **Rejected**: building harbor adapters for Copilot and omp (out of scope).

### Harbor suite lands first as a baseline milestone (milestones)
- **Decision**: build the standing-rules harbor suite in Milestone 1, before the rules change, so its pass rate can be measured against the full-text rules.
- **Rationale**: the spec's success metric compares pass rates "before the change". The new suite can only have a "before" if it exists first.
- **Rejected**: building the suite alongside the compact change (no baseline).

### Installer refactor folded into Milestone 2 (milestones)
- **Decision**: the table-driven installer refactor is a task inside Milestone 2, not its own milestone.
- **Rationale**: it has no user-visible effect on its own and exists to make the compact change land once for all agents.
- **Rejected**: a separate internal-cleanup milestone.

### Task split for the compact change (tasks)
- **Decision**: three spektacular tasks in Milestone 2: the installer refactor; the templates, installer and phrase-test retarget in one task (tests break as soon as the installer changes, and each task must end green); and the cross-agent conversion, budget and refresh tests.
- **Rationale**: every task leaves `go test ./...` passing, per the tests-must-pass convention.
- **Rejected**: separate template and installer tasks (the intermediate state fails tests).

### Short-form install depends on the harbor suite (tasks)
- **Decision**: the "Install compact sections and guidance files" task depends on the harbor-suite task.
- **Rationale**: the suite must exist at a commit before the compact change, so the baseline run can be made against the full-text rules.
- **Rejected**: no dependency (the baseline could be lost).

### Repo README not updated (tasks)
- **Decision**: no change to the spektacular README. Its agent descriptions ("standing rules in root AGENTS.md", "rules mirror Spektacular's sections") stay accurate.
- **Rationale**: the spec's Documented requirement names the website.
- **Rejected**: a README paragraph duplicating the docs page.

### Docs section placed after the migrate section (tasks)
- **Decision**: the new configuration-page section goes after "Upgrading a project: migrate", with `surface={true}`, so no existing section's shading changes.
- **Rationale**: the section is not a settings file, and placing it last keeps the alternate-shading convention with one edit.
- **Rejected**: a new docs page (heavier than needed); placement before migrate (would flip migrate's shading).

### Docs content skeleton lives in plan.md (verification)
- **Decision**: the docs task's Content outline and Content examples sit inside its plan.md task entry, with the fuller version repeated in context.md.
- **Rationale**: the docs repo's plan-content-pages convention requires the skeleton inside the relevant task in plan.md.
- **Rejected**: keeping the skeleton only in context.md (contradicts the convention).

## Rehydration cues

- `spektacular spec file read 000069_compact-agents-md`
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'`
- Read `internal/agent/managed_section.go`, `internal/agent/omp.go`, `internal/agent/claude.go`, `internal/agent/store_access_test.go`.
- `wc -c templates/agents/*.md` (baseline 21,722 bytes).
- Read `tests/harbor/epic-workflow/environment/seed/seed.sh`, `tests/harbor/shared/sessionlog.py`, `Makefile` lines 1-30 and 180-210.
- Docs repo: `/home/nicj/code/github.com/hivecommons/spektacular-website/src/pages/configuration.mdx` lines 538-590, `src/pages/plugins.mdx` lines 69-90.
