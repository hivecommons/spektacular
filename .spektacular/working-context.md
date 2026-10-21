# Working context — 000069_compact-agents-md

## Problem and motivation

User, after the 000068 spec: "I would like to look at the agents.md file and see if we can compact this by adding more linked information rather than bundling everything in there." Chose to keep it a separate spec from 000068 (not an epic). Said "just keep going till everything is finished" during 000068.

## Current state observed

- AGENTS.md is ~21.8 KB. Spektacular `init` installs 8 managed sections from `templates/agents/*.md` (repo-sources ~2.1 KB, memory-context ~0.7 KB, spec-trigger ~3.9 KB, historical-artifacts ~3.7 KB, draft-presentation ~1.3 KB, knowledge-trigger ~2.0 KB, store-access ~3.2 KB, design-trigger ~4.9 KB), each with a "Managed by spektacular init" banner. Placement: most appended, repo-sources hoisted to the top.
- AGENTS.md is loaded in full into every session (Claude via `@AGENTS.md` import in CLAUDE.md), so every section costs context in every conversation whether relevant or not.
- omp already gets the sections as separate always-applied rule files under `.omp/rules/spek-*.md` (precedent for splitting into files).

## Decisions (user)
- Goal: less context per session. Inline: short rule + link per section. Location: files under .spektacular/. Rollout: migrate existing projects + update docs site.
- Drafted all sections under the standing "keep going" instruction; fresh-eyes review findings applied (16). My own choices to flag: <5 KB target, ≤6 lines per rule (≤15 for repo-sources), ≥75% token drop metric.
- Split check: not offered (one weak signal, tightly coupled).

# Orchestrator — plan epic 000001_agent-workflow-improvements (2026-10-21)

- Epic created at user's request grouping 000068_task-goal-validation and 000069_compact-agents-md (no dependencies between them).
- Started plan children: 000068_task-goal-validation (start), 000069_compact-agents-md (start).
- 000069_compact-agents-md: DONE (commit caff415). Summary kept in orchestrator context; decides shared standing-rules table in internal/agent and shared installer used by per-agent install funcs (000068 also edits these); .spektacular/guidance/ exempt from store-access rule.
- Questions put to user / answers:
  - auto_commit: full forces a commit at plan `finished`. User chose: allow the auto-commits for this epic's plan lanes (lane files only). Told both children.
- 000068_task-goal-validation: DONE (commit 0e2c6d6).
- Cross-plan decisions (user): Bob harbor agent move lives in 000069 M1 (both plans edited); 000069 goes first, 000068 depends on it (epic relisted; a wrong-way epic-order dependency was added then removed).
- Epic summary written (both spec sections + decisions). Next: end-of-planning review with the user, then final report.
- Knowledge saved: gotchas/harbor-suites-never-load-agents-md-at-session-start.md, gotchas/root-codex-is-an-empty-file.md. Review confirmed. Run complete.
