# Working context — 000068_task-goal-validation

## Problem and motivation

The user wants "a new feature which uses a sub agent to validate that the current task is done, it would check the task against the criteria", investigating "a fast agent like haiku for this". "With bob and other agents we might need to look at how we specify this."

Today in the implement workflow (`templates/steps/implement/`):
- `05-verify.md` delegates to the `verify-implementation` skill in a sub-agent, but that only maps criteria to commands (`make test`, etc.) and reports exit codes. Nothing judges whether each criterion is met.
- `06-update_plan.md` has the main agent (the one that wrote the code) tick each `- [ ]` acceptance criterion "only if that criterion actually passed verification": the author grades itself from test results alone.

The user then said "We should be looking at the concept of goals, I think anthropic has recently posted something here". Research findings:
- Claude Code `/goal` (v2.1.139+, https://code.claude.com/docs/en/goal): session-scoped prompt-based Stop hook; after every turn the small fast model (Haiku by default, `ANTHROPIC_DEFAULT_HAIKU_MODEL`) judges the condition against the transcript only (no tools, no file reads). Verdicts: not yet met (reason fed back as guidance), met (goal clears), impossible (goal clears, recorded failed). User-typed slash command; a skill cannot start it.
- Managed Agents Outcomes (https://platform.claude.com/docs/managed-agents/define-outcomes): markdown rubric of gradeable criteria; separate grader in its own context window; per-criterion feedback; iterate until satisfied or `max_iterations` (default 3, max 20).
- Codex `/goal` behind `features.goals`; persisted, pause/resume; third-party sources say self-judged via a completion audit (unconfirmed officially).
- Bob, Copilot, omp: no native goal feature found.

## Decisions reached

- **Goals are per task** (user: "Per task I think is correct, then we can use the sub agent to independently validate that the work has been done"). Spec-level goals (spec success criteria at reconcile_spec / epic merge gate) were offered as a follow-up, not in scope.
- A task's acceptance criteria (plus constraints such as touching only the task's files) are its goal.
- Validation is by an independent, read-only, fresh-context sub-agent that may read the diff and files (deliberately unlike `/goal`'s transcript-only evaluator), on a cheap/fast model where the agent permits.
- Verdict per criterion: met / not yet met / impossible, with evidence. Not yet met → back to implement with the reasons, bounded retries (Outcomes-style). Met → update_plan may tick it. Impossible → existing STOP-on-mismatch to the user.
- Spektacular owns the goal concept (CLI-held, agent-agnostic), not each agent's native `/goal`. Goals/verdicts through the CLI so harbor verifiers can assert on CLI state.

## Per-agent sub-agent model support (research, sources read 2026-10-21)

- Claude: `.claude/agents/*.md`, `model: haiku`, `tools: Read, Grep, Glob` (read-only). Per-call Agent `model` param overrides frontmatter.
- Codex: `.codex/agents/*.toml` with `model`, `model_reasoning_effort`; required `name`, `description`, `developer_instructions`. No read-only sandbox for sub-agents (inherits parent). Spawns only when explicitly instructed.
- Copilot CLI: `.github/agents/*.agent.md`, `model:`/`models`/`modelPolicy`, `tools:`. Ignores model when session is `Auto`. ALSO loads project `.claude/agents/` → possible duplicate definition.
- omp: `.omp/agents/*.md`, `model: "@smol"` role alias (user-configured cheap model), `tools:`. Ignores `.claude/agents`.
- Bob: no documented per-agent model. Built-in `explore` sub-agent is read-only and "runs on a lighter model" (IDE docs; unconfirmed for Bob Shell). Personas in `.bob/agents/` (unconfirmed). Bob ACP cannot choose a model.
- No agent install in `internal/agent/*.go` writes sub-agent definitions today; step templates never name a model; config has no per-agent model key.

## Open items carried in

- Retry bound; whether a model override lives in `.spektacular/config.yaml` (model names differ per agent); Copilot double-loading `.claude/agents/`; Bob experiment (`explore`); harbor suite seeding a subtly unmet criterion to measure false "met".

## Interview answers (user)

- Placement: new `validate` step after verify (user first asked "What does verify do?"; after explanation chose the separate step).
- Not yet met: retry then ask (bounded, ~2).
- Config: on by default, configurable (disable + per-agent model override).
- Docs repo: yes, update docs.

- Overview and requirements confirmed by the user as drafted.
- At acceptance criteria the user said "just keep going till everything is finished": draft remaining sections from the interview and conversation without stopping for per-section confirmation; present everything in the final summary.

- Referenced design `design:plan-task-graph.md` (task/criteria shape). This spec changes its rule "ticked by implement only when the criterion passed verification" → "ticked only when judged met"; the design document itself was NOT revised — offer that to the user at the end.
- Success-metric numbers (9/10 first-attempt met, <10% spend, <2 min per task) are my proposals, not user-stated; flag for review in the final summary.
- Non-goals drafted without per-item confirmation under the "keep going" instruction; flag for review.
- Fresh-eyes review returned 23 findings; applied under the "keep going" instruction: removed conversation provenance from constraints, deduplicated non-goals/TA, fixed retry limit at two retries, defined "cheaper model tier", recorded goal in outcome, added switched-off compatibility constraint, added legacy-plan AC (phases' criteria judged the same way — my decision, flag), epic spec not merged while question open, Copilot Auto/limitations documented, metric runs ≥10 per agent.
- Spec written to store; split check: not offered (one weak signal, tightly coupled).

## Other

- User chose two separate specs (not an epic). Second spec to follow: compacting AGENTS.md by linking out rather than bundling everything inline.
- Spec name confirmed: task-goal-validation (→ 000068_task-goal-validation).
