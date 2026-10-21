---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Research: 000068_task-goal-validation

## Alternatives considered and rejected

### Option A: Fold the judgement into the `verify` step's sub-agent

The verify sub-agent already reads the task's criteria and runs commands; it could also return per-criterion verdicts.

**Rejected**: the verify sub-agent needs a shell to run commands (`templates/skills/skill_verify-implementation.md` Step 2), so it cannot be read-only, and it is launched by the implementing agent with the implementer's framing. The spec's Technical Approach rejects this explicitly ("that sub-agent needs a shell and so cannot be read-only").

### Option B: Keep verdicts and the attempt count only in workflow data (state.json / lane file)

A `goal_attempts` key in workflow data, incremented by the check step's callback, with verdicts passed through `goto --data` (`cmd/autocommit.go:86-90` already persists extra `--data` keys).

**Rejected**: lane files are deleted when a run reaches `finished` (`cmd/autocommit.go:106-111`, `287-290`), so nothing could be inspected after the run (spec: "The outcome is recorded ... inspected after the run"). The session log (`internal/sessionlog/record.go:51-56`) snapshots `current_step`/`completed_steps` only, not Data.

### Option C: Store the outcome as a markdown plan document written with `plan file write`

A `goals` document in the plan's folder that the agent writes like `test-plan`.

**Rejected**: the agent would author the verdict record by hand, so nothing would guarantee N verdicts for N criteria, the goal snapshot, or the attempt count; the retry bound would be prose only. A CLI verb that validates and appends gives the guarantees and keeps the store-access rule (convention: store files are written through the CLI).

### Option D: Delegate to a coding agent's native goal feature (Claude Code / Codex `/goal`) or a Stop hook

**Rejected**: spec Constraint forbids it; native goal features are user-invoked or absent in most agents, and `/goal`'s evaluator judges from the transcript, not the files.

### Option E: Refresh the reviewer definition at runtime from the `check_goal` step so a model override needs no reinstall

**Rejected**: steps must write only through the store (`knowledge: architecture/working-with-files-from-steps.md`), and agents load sub-agent definitions at session start, so a definition rewritten mid-session is not reliably picked up. The override is rendered at install time (`init`, and `migrate` on a version change), as every other installed file is (`cmd/init.go:86`, `internal/migrate/engine.go:180-187`).

### Option F: Give the Claude reviewer `Bash` so it can run `git diff` itself

**Rejected**: `Bash` is not read-only. The implementing agent stages the diff under `.spektacular/tmp/` before launching the reviewer, and the reviewer gets `Read`, `Grep`, `Glob` only, so on Claude the read-only rule is enforced by the tool list.

### Option G: Guard merges by refusing `epic merge` whenever the spec's implement lane is in progress

**Rejected in favour of a narrower guard**: lanes are removed at `finished`, so a broad guard would be safe for DONE merges, but it would also block a user deliberately merging a stuck spec. The spec only requires no merge while the goal-check question is open, so `epic merge` refuses only when the lane's current step is `check_goal`.

## Chosen approach — evidence

- FSM loop-backs are expressed with multi-source steps: `internal/steps/implement/steps.go:31` (`analyze` from `read_plan`/`update_changelog`) and `:40` (`finished`). `implement` gains `check_goal` as a source; `update_plan` gains `check_goal`, keeping `verify` for the check-off path.
- Every orchestrated step automatically gets the QUESTION hand-back appended (`internal/stepkit/stepkit.go:106-143`; `templates/partials/orchestrated-stop.md`), so the new stop needs no orchestrated-only prose; `cmd/orchestrated_test.go:76,94` pin it for every step in `cmd/instruction_contract_test.go:39-108`.
- Merges happen only on `DONE:` or `awaiting_merge` (`templates/skills/workflows/spek-implement-epic/SKILL.md` Step 5), and status reports `in_progress` while a lane exists (`internal/status/run.go:354-388`), but `epic merge` itself checks no workflow state (`cmd/epic_worktree.go:143-209`, `internal/worktree/worktree.go:670-730`): the explicit guard closes that gap.
- Plans already parse per task; `plantask.Task.Criteria` holds counts only (`internal/plantask/plantask.go:73-77`, `235-243`) and legacy `Phase N.M` headings are counted but not parsed into items (`:103`, `187-201`). Extending the reader with criterion text and legacy phase items keeps one parser for goal building.
- The task's files are already derivable from its context section: `plantask.TaskFiles` (`internal/plantask/files.go:37-64`), used for the goal's scope limits.
- Config sub-keys under `implement` were added before without a settings-format bump because configs parse over `NewDefault` (`internal/config/config.go:155-163`, `396`); `implement.goal_check.enabled: true` follows the same pattern, and old configs load with the check on.
- Agent installs are per-agent `Install()` functions (`internal/agent/{claude,codex,copilot,omp,bob}.go`) refreshed by both `init` and `migrate` (`cmd/init.go:86`, `cmd/migrate.go:66-78`), so writing the reviewer definition there needs no registry.
- A fresh-context reviewer brief pasted verbatim is an established pattern: `templates/steps/spec/08-verification.md:25-40`.
- The session log records every CLI call with its `--data` (`internal/sessionlog/record.go:18-29`), so a `implement goal record` call gives harbor an agent-independent signal of the check, readable through `tests/harbor/shared/sessionlog.py`.
- Anthropic Managed Agents "Outcomes" and Claude Code `/goal` (spec Technical Approach): rubric = criteria, grader in its own context, per-criterion verdict with feedback, bounded retry.

## Files examined

- `spektacular:internal/steps/implement/steps.go:27-42` — step table; `verify` → `update_plan`; multi-source `analyze`/`finished`.
- `spektacular:internal/steps/implement/steps.go:119-137` — `verify`/`update_plan` callbacks render with `taskExtra`.
- `spektacular:internal/steps/implement/steps.go:205-264` — `finished` reads `orchestrated`; worktree merge only for standalone runs.
- `spektacular:internal/steps/implement/steps.go:268-301` — `taskRun`/`taskExtra`/`taskVars` read the plan for single-task runs.
- `spektacular:templates/steps/implement/05-verify.md` — verify sub-agent, STOP on failure with fix/accept/abandon.
- `spektacular:templates/steps/implement/06-update_plan.md` — ticks criteria "only if that criterion actually passed verification".
- `spektacular:templates/steps/implement/07-update_changelog.md` — task entry format; whole-plan loop by counting unchecked headings.
- `spektacular:templates/partials/implement-current-task.md` — current-task rule for task and legacy plans.
- `spektacular:templates/partials/orchestrated-stop.md` — appended to every orchestrated step by `stepkit`.
- `spektacular:templates/skills/skill_verify-implementation.md` — verify sub-agent brief; shape for a reviewer skill.
- `spektacular:cmd/skill.go:50` — `spektacular skill <name>` serves `templates/skills/skill_<name>.md`.
- `spektacular:templates/steps/spec/08-verification.md:25-40` — fresh-context reviewer pattern with a verbatim brief.
- `spektacular:internal/stepkit/stepkit.go:61-177` — orchestrated partial, commit partial and footer order.
- `spektacular:internal/workflow/workflow.go:202-240` — Goto validates Src; invalid transitions name valid gotos.
- `spektacular:internal/workflow/lane.go` — lanes `.spektacular/workflows/<kind>-<name>.json`; `ReadLane`.
- `spektacular:cmd/autocommit.go:86-111` — extra goto `--data` keys persisted; lanes removed at `finished`.
- `spektacular:internal/plantask/plantask.go:73-243` — `Criteria{Met,Total}`; criteria parsing; legacy counting only.
- `spektacular:internal/plantask/files.go:37-104` — `TaskFiles` from `### Task:` context sections.
- `spektacular:internal/status/report.go:132-135` — `TaskStatus.AcceptanceCriteria` is the JSON `Criteria`.
- `spektacular:internal/status/run.go:354-409` — `in_progress` while a lane exists; `awaiting_merge` only when finished in project.
- `spektacular:cmd/epic_worktree.go:143-209` — `epic merge` → `writeMergeResult`; no workflow-state check.
- `spektacular:internal/worktree/worktree.go:670-730` — merge refusals (no worktree, dirty, `.spektacular`, conflict).
- `spektacular:internal/config/config.go:155-163,327-411` — `ImplementConfig{Worktrees}`; `Agent` is one string; defaults.
- `spektacular:internal/config/schema.go:19` — `CurrentProjectSchema = 4`; no bump needed for new `implement` sub-keys.
- `spektacular:internal/agent/agent.go:20-48` — `Agent{Name, Install}` registry.
- `spektacular:internal/agent/{claude,codex,copilot,omp,bob}.go` — install targets: `.claude/skills`, `.agents/skills`, `.github/skills`, `.omp/skills`, `.bob/skills`; none installs sub-agents.
- `spektacular:internal/agent/managed_section.go:220` — `writeFileAtomic` helper.
- `spektacular:internal/agent/copilot_test.go:46` — counts the whole tree (9 files): breaks when `.github/agents/` is added.
- `spektacular:internal/agent/omp_test.go:38-93` — snapshot/restore test covers any new `.omp/` file.
- `spektacular:internal/agent/codex_test.go:47-50` — codex must not write `.claude/`.
- `spektacular:internal/agent/instruction_surface_test.go:73` — `instructionTemplateRoots`; a new template folder must be added.
- `spektacular:cmd/init.go:17-103` — `init <agent>`; install then stamp `skills_version`.
- `spektacular:cmd/init_test.go:403-420` — second init must be byte-identical.
- `spektacular:cmd/instruction_contract_test.go:39-108` — hand-maintained step template table.
- `spektacular:internal/steps/implement/steps_test.go:92-208,437` — step order, FSM walks, STOP directive map.
- `spektacular:cmd/{implement_lane,epic_flow,autocommit,implement_task_run,orchestrated_commit,milestones,implement_worktree_flow}_test.go` — hard-coded walks through `verify`,`update_plan`.
- `spektacular:internal/autocommit/points.go:32-48` — commit points don't involve verify/update_plan.
- `spektacular:templates/skills/workflows/spek-implement-epic/SKILL.md:58-105` — QUESTION handling, genuine-question list, merge rules.
- `spektacular:tests/harbor/implement-workflow/` — Claude-only, one fixed scenario starting at `test_plan`, no code; `EXPECTED_STEP_ORDER` in `tests/test_implement_workflow.py:57-70`; `environment/state.json` `completed_steps`.
- `spektacular:tests/harbor/implement-epic-workflow/solution/solve.sh:16` — hard-coded STEPS list.
- `spektacular:tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py:209-281` — agent-independent session-log assertions.
- `spektacular:tests/harbor/implement-epic-workflow/environment/seed/seed.sh:21-60` — fixture written through the CLI with a build-time status guard.
- `spektacular:tests/harbor/shared/sessionlog.py` — `Lane`, `Event.data()`, `goto_step`.
- `spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py:324-363,429-458,551-586` — Claude and Bob (ACP) transcript parsing and step windows.
- `spektacular:Makefile:5-24,168-252` — harbor agent args; implement suite has no render; implement-epic renders per agent with `VARIANT`; Bob expects `tests/harbor/agents/`.
- `spektacular:tests/harbor/implement-epic-workflow/agents/` — Bob harbor agent files live here, not in `tests/harbor/agents/` where the Makefile looks.
- `docs:src/pages/how-it-works.mdx:141,440-487` — implement steps tree; Implement pipeline stage body.
- `docs:src/pages/plan-tasks.mdx:88-93` — "ticked ... only when the criterion passed verification".
- `docs:src/pages/configuration.mdx:72-73,301-313` — `implement` example and `ConfigKey`.
- `docs:src/pages/plugins.mdx:37-106` — per-agent entries; Bob limitation precedent; Copilot absent from the site.

## External references

- Anthropic Managed Agents "Outcomes" — rubric plus independent grader with per-criterion feedback; the model for verdict + evidence + bounded retry.
- Claude Code `/goal` — evaluator in its own context; judges from transcript only (why the reviewer reads files instead).
- Claude Code sub-agents (`.claude/agents/*.md`, frontmatter `name`, `description`, `tools`, `model`) — reviewer definition format.
- Codex custom agents (`.codex/agents/*.toml`) — reviewer definition format; model field.
- GitHub Copilot custom agents (`.github/agents/*.agent.md`, `tools`, `model`) — reviewer definition; `model` ignored when the session model is `Auto`.
- oh-my-pi agents (`.omp/agents/`, `@smol` role alias) — reviewer definition.
- Bob Shell built-in `explore` sub-agent — read-only, lighter model.

## Prior plans / specs consulted

- `000058_plan-task-graph` (design `design:plan-task-graph.md`) — task format; criteria are `- [ ]`/`- [x]` boxes; legacy plans keep working; `plan status` reports `acceptance_criteria` separately from completion.
- `000066_epic-mid-run-revisions` (plan) — precedent for adding a stop to implement steps that the orchestrated partial converts into a `QUESTION:`, and for updating `spek-implement-epic`'s genuine-question list; no FSM step was added there.
- `000065_implement-in-worktrees` / `000062_epic-plan-and-implement` — worktree code roots passed to sub-agents (`partials/implement-code-locations.md`); orchestrator merges on DONE.
- Epic `000001_agent-workflow-improvements` — sibling spec `000069_compact-agents-md` rewrites the AGENTS.md managed sections in `internal/agent`; this plan adds no managed section, to avoid overlapping it.

## Open assumptions

- Codex custom-agent files are TOML under `.codex/agents/` with `name`, `description`, `developer_instructions` and `model`; the exact fast default model name (a "mini" tier of the default Codex model) must be confirmed against current Codex docs at implementation. If Codex offers `sandbox_mode = "read-only"` for custom agents, set it as well as the instruction.
- Copilot custom agents are `.github/agents/<name>.agent.md` with `tools` and `model`; the exact display name of the cheap default model must be confirmed. Copilot is assumed to prefer `.github/agents/` over `.claude/agents/` for a same-named agent, as it does for skills (`README.md:130-135`).
- omp agents are markdown with frontmatter under `.omp/agents/` and accept `model: "@smol"`; confirm field names against omp docs.
- Claude Code sub-agent definitions honour `model: haiku` and a user's `CLAUDE_CODE_SUBAGENT_MODEL` overrides it (spec: user settings win).
- Bob's built-in `explore` sub-agent can be named from a prompt and can read the staged diff and files; if it cannot, the generic fresh-context fallback applies under Bob.
- `plan file list <feature>` ignores non-`.md` files (`internal/artifact/address.go:183-189`), so `goals.json` in a plan's folder does not appear as a document.

## Drafting assumptions

### Outcome stored as `goals.json` in the plan's folder, written only by a CLI verb (discovery)
- **Decision**: record each check in a JSON file beside the plan documents, appended only by `spektacular implement goal record`/`decide` and read back by `implement goal show`.
- **Rationale**: lane data is deleted at `finished`, and an agent-authored markdown document could not guarantee N verdicts or the attempt count; the session log also captures every record call for harbor.
- **Rejected**: workflow data only (lost after the run); a hand-written `goals.md` plan document (no validation).

### No settings-format bump for `implement.goal_check` (discovery)
- **Decision**: add `implement.goal_check.enabled` (default true) and `implement.goal_check.models.<agent>` without raising `CurrentProjectSchema`.
- **Rationale**: configs parse over `NewDefault`, and `implement.worktrees` and `plan.strict_spec_changes` were added the same way; old configs get the check on, as the spec requires.
- **Rejected**: a `project4to5` migration step that writes the section (adds churn with no behavioural gain).

### Model override takes effect at install time (discovery)
- **Decision**: the per-agent model is rendered into the installed reviewer definition by `init`, and by `migrate` on a version change; documentation says to re-run `spektacular init <agent>` after changing the override.
- **Rationale**: agents load sub-agent definitions at session start, and steps must not write outside the store.
- **Rejected**: rewriting the definition from the step at runtime.

### Narrow merge guard (discovery)
- **Decision**: `epic merge` refuses while the spec's implement lane is at `check_goal`.
- **Rationale**: closes the one gap where an orchestrator could merge while a goal-check question is open, without blocking a user deliberately merging a stuck spec.
- **Rejected**: refusing any merge while the lane is in progress.

### Same reviewer name for every agent (discovery)
- **Decision**: every agent's definition is named `spek-goal-reviewer`; Copilot gets its own `.github/agents/` copy and is assumed to prefer it over `.claude/agents/`, as it does for skills.
- **Rationale**: one name in the step instruction; a project initialised for Claude and Copilot then holds two same-named definitions instead of two different reviewers.
- **Rejected**: Copilot reusing `.claude/agents/` (Claude's `haiku` alias is not a Copilot model name); skipping `.claude/agents` when Copilot is present (init installs one agent at a time and cannot know).

### Chosen direction: a CLI-owned `check_goal` step with a named read-only reviewer (architecture)
- **Decision**: new FSM step `check_goal` between `verify` and `update_plan`; the CLI builds the goal, validates and records verdicts (`implement goal record`/`decide`/`show`), decides accept / retry / stop, and gates `update_plan`; each agent gets an installed `spek-goal-reviewer` (Bob uses `explore`; any agent without one uses a generic fresh-context sub-agent with the same brief).
- **Rationale**: matches the spec's Technical Approach; puts the N-verdicts rule, the two-retry bound and the "tick only met" rule in Go where they are testable, leaving only judging to agents.
- **Rejected**: folding into `verify` (needs a shell); prose-only retry counting and agent-written verdict documents (unenforceable); lane-only storage (deleted at finish).

### Retry rounds and user decisions (architecture)
- **Decision**: a round allows one check plus two retries; when it is exhausted, or on any `impossible`, the user picks `keep_fixing` (new round, back to `implement`), `accept` (tick only met), `skip` (tick nothing) or, after correcting a criterion's text in the plan, `recheck` (re-run the check). The decision is recorded with `implement goal decide`.
- **Rationale**: the spec names keep fixing / accept / skip; "keep fixing" must actually allow further attempts, and correcting the plan is how today's plan-versus-reality stop is resolved.
- **Rejected**: a single lifetime budget per task (keep fixing would be exhausted immediately).

### Retry re-runs the whole implement → test → verify chain (architecture)
- **Decision**: a not-yet-met retry goes back to `implement`, then `test`, `verify` and `check_goal` again.
- **Rationale**: keeps the FSM linear, and fixes get tests and fresh verification evidence for the next check.
- **Rejected**: jumping straight from `implement` to `check_goal` (no verification evidence for the new code).

### Template file name `05b-check_goal.md` (architecture)
- **Decision**: name the new template `steps/implement/05b-check_goal.md` rather than renumbering `06`–`12`.
- **Rationale**: the numbering already has a gap (no `08`), and renaming would churn every test and table that names template paths.
- **Rejected**: renumbering the later templates.

### Conventions selected (architecture)
- **Decision**: apply the five spektacular conventions and the docs MDX, no-em-dash, content-structure and layout conventions; shading and file-scoped headings are noted as considered but not biting.
- **Rationale**: the plan touches CLI errors, store writes, templates, tests and docs pages in existing sections.
- **Rejected**: none dropped silently.

### Items are keyed by task id or legacy phase label (data_structures)
- **Decision**: the goal, the outcome record and the CLI address an "item": a task by its `**Id:**`, or a legacy phase by its `Phase N.M` label.
- **Rationale**: plans written before tasks have no ids, and the spec requires their phases to be judged the same way.
- **Rejected**: requiring ids (would break legacy plans); keying by title (titles may drift).

### The CLI reports the next step, and stops stay on `check_goal` (data_structures)
- **Decision**: `record`/`decide` return `next_step` and a runnable `next_action`; for the exhausted and impossible stops the run stays at `check_goal` until `decide` is recorded.
- **Rationale**: the retry bound is enforced in Go and the agent never computes it; staying on `check_goal` is what the `epic merge` guard keys on.
- **Rejected**: a separate stop step in the FSM (adds a step with no work of its own).

### The CLI fingerprints the work to prove the reviewer changed nothing (tasks)
- **Decision**: the agent stages each code root's diff and status before launching the reviewer, and `implement goal record` compares them with the live tree, storing `tree_unchanged` on the check.
- **Rationale**: it gives the "leaves the work unchanged" criterion an agent-independent check, from CLI state, that harbor can assert for every agent, including those whose read-only rule is instruction-only (Codex).
- **Rejected**: transcript-only checks (no signal for Codex); refusing the record on a mismatch (would lose the evidence).

### Harbor runs and per-agent confirmation are manual checks, not tasks (tasks)
- **Decision**: harbor suites, Copilot/omp runs, Codex model confirmation, Bob `explore` confirmation and the epic-question check are manual items in the Testing Approach; no `human` tasks.
- **Rationale**: they are checks a person makes, not work that makes the change exist (design: a review is never a task); the testing-architecture entry requires the runs, and the test plan carries them.
- **Rejected**: `human` tasks for harbor runs (would never be ticked).

### The design update is an agent task with an orchestrated hand-back (tasks)
- **Decision**: the plan-task-graph tick-rule revision is an `agent` task done through `design author`; in an orchestrated run the child hands the staged revision back as a `QUESTION:` for the orchestrator to apply.
- **Rationale**: the spec requires the design update, and the epic child prompt forbids children writing design documents, so the existing amendment-style hand-back is the path.
- **Rejected**: doing it during planning (outside the plan workflow's remit); a `human` task (no human-only reason applies).

### Bob harbor agent files move to `tests/harbor/agents/` in sibling spec 000069 (epic decision)
- **Decision**: this plan does not move the Bob harbor agent. Sibling spec 000069_compact-agents-md moves the tracked files from `tests/harbor/implement-epic-workflow/agents/` to `tests/harbor/agents/`, where `Makefile:8,18,24` already looks, in its first milestone.
- **Rationale**: settled by the user while planning epic 000001_agent-workflow-improvements. 000069's first milestone needs a Bob baseline before anything else lands, so the move belongs there; Bob runs of this plan's suites rely on it.
- **Rejected**: moving it in this plan's harbor milestone (originally drafted; too late for 000069's baseline); pointing the Makefile at the suite-local folder (every suite needs the agent).

### Default reviewer models for Codex and Copilot are confirmed at implementation (tasks)
- **Decision**: Claude `haiku` and omp `@smol` are fixed; the Codex mini model and the cheap Copilot model names are confirmed against current agent docs when the installer task starts, and recorded in a code comment and the docs.
- **Rationale**: model catalogues change faster than the plan; the spec requires only "a smaller, cheaper tier than the agent's default".
- **Rejected**: hard-coding a name now that may not exist by implementation time.

## Rehydration cues

- `spektacular spec file read 000068_task-goal-validation`; `spektacular design read --data '{"source":"design","path":"plan-task-graph.md"}'`.
- `spektacular plan file read 000068_task-goal-validation plan|context|research`.
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/testing-architecture.md"}'` and `gotchas/harbor-agent-logs-and-artifacts.md`.
- Re-read `internal/steps/implement/steps.go`, `templates/steps/implement/05-verify.md`, `06-update_plan.md`, `templates/partials/implement-current-task.md`, `internal/plantask/plantask.go`, `internal/agent/claude.go`, `cmd/epic_worktree.go`.
- `spektacular implement steps` lists the step table after the change.
