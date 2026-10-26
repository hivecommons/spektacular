---
created_date: "2026-10-21"
---

# Planning summary: 000001_agent-workflow-improvements

## Decisions
- **Bob harbor agent location.** The Makefile's Bob targets expect `tests/harbor/agents/`, but the files sit in `tests/harbor/implement-epic-workflow/agents/`. Plans involved: 000068_task-goal-validation (had planned the move late, in its M3 harbor task) and 000069_compact-agents-md (assumed Bob harbor works and needs a Bob baseline in M1). Outcome (user): 000069_compact-agents-md moves the files in its M1 standing-rules harbor task; 000068_task-goal-validation no longer moves them and relies on that move.
- **Order of the two specs.** Both plans change the per-agent install files in `internal/agent/` and their tests, so they cannot run side by side. Plans involved: 000068_task-goal-validation and 000069_compact-agents-md. Outcome (user): 000069_compact-agents-md goes first, so the Bob harbor agent move lands before 000068's Bob harbor runs; 000068_task-goal-validation depends on 000069_compact-agents-md. An `epic order` run made before the epic was relisted added the opposite dependency, which was removed.

## Order added for shared files
- Added: `000069_compact-agents-md` now depends on `000068_task-goal-validation`. Both change `spektacular:internal/agent/instruction_surface_test.go`, `spektacular:internal/agent/claude.go`, `spektacular:omp.go`, `spektacular:copilot_test.go`, `spektacular:codex_test.go`, `spektacular:omp_test.go`, `spektacular:cmd/omp_test.go`, `spektacular:cmd/migrate_test.go`, `spektacular:tests/harbor/shared/sessionlog.py`, `spektacular:seed/seed.sh`, `spektacular:environment/seed/seed.sh`, `spektacular:instruction.md`, `spektacular:/app/.spektacular`, `spektacular:tests/harbor/repo-workflow/tests/test_verifier_selfcheck.py`, `docs:/home/nicj/code/github.com/hivecommons/spektacular-website`, `docs:src/pages/configuration.mdx`.
- Removed at review: `000069_compact-agents-md` no longer depends on `000068_task-goal-validation`; they may be implemented side by side.

## 000069_compact-agents-md
### Approach
- Each of the eight standing rules becomes a short section in AGENTS.md under its unchanged heading: the obligation that always applies, when to read further, and a "Managed by init; full guidance: [link]" line.
- The full text moves to `.spektacular/guidance/<rule>.md`: the existing template, retitled, nothing removed.
- One table-driven standing-rules installer serves all five agents, replacing five copied lists of eight calls. It writes the guidance files and places each short section through the existing replace-in-place installer.
- `init` and `migrate` already call the agent installer, so conversion and refresh need no new migration step and no settings-format change.
- omp rule files are still cut from the AGENTS.md sections, so they carry the identical short text and omp's deduplication keeps working.
- A draft of all eight short sections is 4,228 bytes, against 21,722 today.
- Rejected: an `@` include; a CLI read command; renamed headings; full text in omp rules; mechanically generated short forms; relying only on existing harbor suites.

### Milestones and tasks
- M1, an end-to-end check proves agents follow the standing rules (lands first for a baseline): add the standing-rules harbor suite, which also moves the Bob harbor agent to `tests/harbor/agents/` so Bob runs of every suite can start (epic decision).
- M2, compact rules with guidance on demand for every agent: share one standing-rule installer across every agent (internal refactor, no output change); install compact sections and guidance files; prove conversion, refresh and the size budget for every agent.
- M3, the docs site explains the layout and the upgrade: document the compact layout on the docs site (docs repo).

### Tasks a person must do
None. Every task is an agent task.

### Out of scope
- Restructuring the skills or step templates.
- Converting this repository's own AGENTS.md or `.spektacular/`.
- Deleting guidance files Spektacular no longer ships; a version-check hook restoring a deleted guidance file.
- Harbor harnesses for Copilot and omp.
- Rewording any rule's full guidance beyond the retitle, the banner and the store-access read exemption.
- A README change; editing the store-files knowledge entry.

### Drafting assumptions
- Short templates live under `templates/agents/short/`; conversion happens through the normal reinstall.
- The guidance folder is `.spektacular/guidance/`, committed with the project.
- Section headings stay byte-identical, including the em dash in the repo-sources heading, so upgrades replace rather than duplicate sections.
- omp rules carry the compact text, identical to AGENTS.md.
- Size limits are in bytes: under 5,120 in total; at most 6 lines per rule (15 for repo-sources) counting the link line; prose lines at most 100 columns, link line exempt.
- The new harbor suite runs `init` at image build, so AGENTS.md is loaded at session start.
- The guidance-file read exemption does not contradict the store-files knowledge entry, since the guidance folder is not a store.
- The byte budget stands in for the token metric, confirmed once by hand.
- Copilot and omp are verified by hand; only Claude, Codex and Bob run in harbor.
- The installer refactor is folded into M2; the installer change and phrase-test retargeting are one task; the compact-install task depends on the harbor suite.
- The Bob harbor agent move lives in M1 of this plan (epic decision; 000068_task-goal-validation relies on it).
- The new docs section goes after the migrate section with a shaded background; the docs content outline sits in the plan task.

### Project-wide rules
- Decides: the standing rules are one table in `internal/agent`; any new rule is a table entry plus a short and a full template.
- Decides: the per-agent install functions call one shared standing-rules installer. 000068_task-goal-validation also edits these functions, so whichever lands second rebases.
- Decides: `.spektacular/guidance/` is a read-only folder of installed files agents read directly; the store-access rule exempts it.
- Decides (epic): this plan moves the Bob harbor agent to `tests/harbor/agents/`.
- Relies on: no plan task re-inits or migrates this repository.
- Relies on: harbor oracles change with the surfaces they mirror, and harbor runs are manual test-plan items.
- Relies on: the changelog is kept through the implement workflow's changelog store; the root CHANGELOG.md is not hand-edited.
- Relies on: docs prose uses no em dashes, content skeletons sit in plan.md, and the MDX and shading conventions apply.

### Manual checks
- Token reduction of at least 75%, confirmed with Claude's context accounting in one initialised project before and after.
- No harbor suite's pass rate drops: run every suite (spec, plan, implement, repo, epic, implement-epic, standing-rules) for Claude, Codex and Bob, before and after.
- In a fresh throwaway project, Copilot and omp each run `repo list` before code work, read a spec only through the CLI, and offer rather than start a spec.
- A person reads each short section beside its guidance file and confirms the obligation is inline, the trigger is clear and nothing was lost.
- A new Claude session does not contain guidance-file content until the agent opens a file.
- A docs review of the configuration and plugins pages, with the docs build checks passing.

## 000068_task-goal-validation
### Approach
- A new implement step, `check_goal`, runs between `verify` and `update_plan`; the CLI owns the step and the retry rule, agents only judge.
- The CLI builds each task's goal (criteria, repo, files from `context.md`; legacy plans by `Phase N.M`). The implementer stages the diff and status, then launches a fresh read-only reviewer with only the goal, the verify summary and pointers to the work.
- `spektacular implement goal record` validates one verdict with evidence per criterion, appends to the plan's `goals.json`, and returns the next `goto`. All met: `update_plan`. Any impossible: stop and ask. Not met: back to `implement` with reasons, at most 2 retries per round, then stop and ask (`keep_fixing`, `accept`, `skip`, `recheck`, recorded with `implement goal decide`).
- `update_plan` refuses until the task is settled and ticks only criteria judged met.
- `init` installs a `spek-goal-reviewer` helper for Claude (`haiku`; Read/Grep/Glob), Codex (mini model, read-only by instruction), Copilot (cheap model, read/search tools) and omp (`@smol`). Bob uses its built-in `explore` helper; any other agent falls back to a generic sub-agent with `spektacular skill review-task-goal`.
- Config: `implement.goal_check.enabled` (on by default, no settings-format bump) and `implement.goal_check.models.<agent>`.
- `epic merge` refuses with `goal_check_open` while a spec's implement run is stopped at `check_goal`.
- Rejected: folding the check into `verify`; keeping results only in run state; agent-written verdict documents; rewriting the helper's model mid-session.

### Milestones and tasks
- M1, implement runs check every task's goal before ticking it: teach the plan reader criterion text and legacy phases; add the goal-check configuration; add the goal package; add the implement goal commands; write the reviewer brief and skill; add the `check_goal` step to the implement workflow.
- M2, every agent gets a cheap read-only reviewer and epic runs stop safely: install a reviewer helper for every agent (with a README paragraph); keep epic merges off an open goal-check question.
- M3, proved end to end: update the implement and implement-epic harbor suites; add the goal-check harbor suite (unmet and met scenarios for Claude, Codex and Bob). Bob runs rely on 000069_compact-agents-md having moved the Bob harbor agent.
- M4, documented: update the tick rule in the plan-task-graph design (through `design author`; in an orchestrated run the child hands the staged revision back as a question); document the goal check on the docs site (docs repo).

### Tasks a person must do
None. All 12 tasks are agent tasks.

### Out of scope
- Spec-level goals; changing how verification commands run; verdicts for scope limits.
- Changing a helper's model mid-session; working around Copilot `Auto`, Codex having no read-only mode, or Bob having no per-helper model (documented instead).
- Copilot and omp in harbor; listing Copilot as a shipping agent on the docs site; removing stale reviewer files from older installs.
- AGENTS.md managed sections (owned by 000069_compact-agents-md).
- Refusing merges of in-progress specs at any step other than `check_goal`.

### Drafting assumptions
- Results live in `goals.json` in the plan's folder of the plan store, written only by `implement goal record`/`decide`.
- No settings-format bump: `goal_check` loads over the defaults, so older configs get the check on.
- A model override takes effect when the helper is installed (`init`, or `migrate` on upgrade).
- Every agent's helper is named `spek-goal-reviewer`; Copilot is assumed to prefer `.github/agents/` over `.claude/agents/`.
- Retries come in rounds of one check plus 2 retries; a retry re-runs implement, test and verify.
- The new template is `05b-check_goal.md` rather than renumbering later ones.
- Items are keyed by task id, or by `Phase N.M` in older plans; the run stays on `check_goal` during a stop.
- `record` fingerprints the code (live diff and status against the staged files) and stores `tree_unchanged` to prove the reviewer changed nothing.
- Harbor runs and per-agent confirmations are manual checks, not tasks.
- The Bob harbor agent is moved by 000069_compact-agents-md, not by this plan (epic decision).
- Default Codex and Copilot model names are confirmed against current docs when implementation starts.

### Project-wide rules
- Changes to implement workflow steps update every harbor suite they touch (implement, implement-epic), plus the new goal-check suite.
- Plan store files are written only through the CLI; `goals.json` only through `implement goal` verbs, with replies passed `--from` a file under `.spektacular/tmp/`.
- Sub-agent templates live in a new `templates/subagents/` folder, separate from `templates/agents/` (AGENTS.md section fragments).
- Edits the per-agent `Install()` files (`internal/agent/{claude,codex,copilot,omp}.go`) and their tests, which 000069_compact-agents-md also changes; adds no managed AGENTS.md section.
- `implement` config sub-keys are added without a settings-format bump, following `implement.worktrees`.
- `epic merge` gains a refusal tied to the run's current step (`goal_check_open`).
- No plan task re-inits or migrates this repository.
- Docs conventions: no em dashes, content examples in docs tasks, no layout HTML.

### Manual checks
- Unmet harbor scenario: the seeded criterion is judged not met in all of at least 10 runs per agent.
- Met scenario: every criterion judged met on the first attempt in at least 9 of 10 runs.
- The reviewer costs under 10% of the implementer's spend where a fast model can be chosen.
- The check adds under 2 minutes per task in the harbor implement suite.
- Harbor runs pass for implement-workflow, implement-epic-workflow and goal-check under Claude, Codex and Bob.
- Under Copilot and omp, an implement run on a throwaway project records verdicts for every task, and the log shows `spek-goal-reviewer` on the cheaper model.
- Under Codex, the reviewer runs on the configured mini model (from the session log).
- Under Bob, `explore` is used as the reviewer and reads the staged diff, or the fallback is used and documented.
- In an epic run, a goal-check stop reaches the user as a question from that spec, and the spec is not merged until it is answered.
- The docs pages read correctly on the built site.
