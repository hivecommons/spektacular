### Chosen direction: shared task reader + single implement FSM with Go-side last-task routing (architecture)
- **Decision**: Option A. New `internal/plantask` reader (tasks + legacy phases) used by write validation, export, status, implement and milestone commits; validation in the shared store-file write handler; task-id providers in `internal/identifier`; single-task implement runs inside the existing implement FSM with a `task` data key, `{{#task}}` template sections, an `update_changelog` callback deciding last-open-task, and a new `update_changelog → finished` edge.
- **Rationale**: one parser satisfies the spec's technical approach; reusing the FSM keeps whole-plan runs unchanged and avoids duplicating 12 templates; Go-side routing makes the wrap-up rule testable.
- **Rejected**: Option B, a separate task-scoped implement FSM (duplicated templates, tests, harbor oracles); Option C, prose-only routing (untestable, the exact failure the spec's "wrap-up once" requirement guards against).

### Rename plan step `phases` → `tasks` (architecture)
- **Decision**: rename the FSM step, its template (`10-tasks.md`) and working files (`tasks_plan.md`, `tasks_context.md`); plan section `## Milestones & Tasks`, context section `## Per-Task Technical Notes`.
- **Rationale**: spec says plan units of work are tasks; leaving "phases" in the step name would keep two vocabularies.
- **Rejected**: keeping the `phases` step name (vocabulary drift). Cost accepted: a plan workflow paused exactly at `phases` cannot resume across the upgrade.

### document_status in export matches plan status (architecture)
- **Decision**: export reuses the `plan status <name>` document-status computation including `strictPlanStatusHook`.
- **Rationale**: AC "each output's document status matches what plan status reports".
- **Rejected**: raw frontmatter only (would disagree when strict staleness applies).

### Conventions selected (architecture)
- **Decision**: all four spektacular conventions plus glossary; docs conventions for the docs task only.
- **Rationale**: see conventions.md; em-dash rule does not apply to the plan.md data separator.
- **Rejected**: none dropped outright.

### repo.location only from a git source (discovery)
- **Decision**: `repo.location` is the declared source location only when `source.provider` is `git`; otherwise `""`.
- **Rationale**: design says "the repo's declared git source"; spec forbids inference; a `file` source is a local path, not a canonical location.
- **Rejected**: exporting the resolved file path (leaks local paths, not a canonical location); falling back to project `source` or `git remote` (inference).

### Legacy plans bypass write validation (discovery)
- **Decision**: task validation on `plan file write` applies only when plan.md uses the task format (any `Task:` heading or a `## Milestones & Tasks` section). Phase-only plans write unvalidated.
- **Rationale**: implement's `update_plan` rewrites plan.md through `plan file write`; refusing legacy plans would break whole-plan implement of old plans, which the spec forbids.
- **Rejected**: refusing any plan without tasks (breaks old plans).

### Task-id provider resolved at request time (discovery)
- **Decision**: unknown `plan.task_id.provider` errors from `plan task-id`, not from config load; no schema bump.
- **Rationale**: load-time validation would block every command through the gate; AC phrases the error as occurring on request; the key is additive with a default.
- **Rejected**: `PlanConfig.Validate` check; schema 3→4 migration.

### UUID without new dependency (discovery)
- **Decision**: v4 UUID from `crypto/rand` in `internal/identifier`.
- **Rationale**: 10 lines of code; package already uses crypto/rand; avoids a go.mod addition.
- **Rejected**: `github.com/google/uuid`.

### Write-validation error code name (data_structures)
- **Decision**: structural write refusals use a single code `plan_task_invalid` whose message names the task and the rule broken.
- **Rationale**: design names refusal conditions but not a code; one code with a specific message keeps callers simple.
- **Rejected**: one code per rule (8 codes for one failure class).

### Export format error code (data_structures)
- **Decision**: `export_format_unsupported`, message names `pretty` and `json`.
- **Rationale**: design is silent on the code; mirrors existing `*_unsupported`/`unknown_*` naming.
- **Rejected**: reusing `invalid_input`.

### This plan uses the current Phase format (phases)
- **Decision**: this plan is written in today's `Phase N.M` format with `**Repo:**` lines.
- **Rationale**: the task format, its validator and the task-aware implement workflow do not exist until this plan is implemented; the current implement workflow parses phases.
- **Rejected**: writing the plan in the new task format (current implement would not find its work items).

### Task-run commit edge classification left to implementer with a stated rule (phases)
- **Decision**: `update_changelog → finished` is a completion commit point that must still record due milestones; exact wiring decided while reading `cmd/autocommit.go:79-125`.
- **Rationale**: both commit kinds apply at that edge; the design does not cover auto-commit.
- **Rejected**: leaving task-run work uncommitted until a later run.

### Keep `unchecked_phases` field name (phases)
- **Decision**: `implement status` keeps `unchecked_phases` and counts tasks for task plans.
- **Rationale**: spec constraint that implement progress reporting keeps working; renaming breaks consumers.
- **Rejected**: renaming the field to `unchecked_tasks`.

### Glossary change goes through spek-knowledge confirmation (phases)
- **Decision**: replacing `glossary/phase.md` with `glossary/task.md` is planned but confirmed with the user during implementation.
- **Rationale**: knowledge writes are propose-then-confirm by project rule.
- **Rejected**: writing the entry unprompted.

### Docs page is a new standalone page (phases)
- **Decision**: new `plan-tasks.mdx` page under the Resources nav, plus wording updates to how-it-works and configuration.
- **Rationale**: no existing CLI reference page; design-documents.mdx is the precedent for a feature page.
- **Rejected**: folding everything into how-it-works.mdx (already 480 lines).
