# Working context: 000060_epics-and-seeded-specs

## Problem and motivation

- GitHub #55: oversized requests produce specs with acceptance criteria too thin to build from;
  implementing agents fill the gaps with assumptions. Offer to split into an epic.
- GitHub #56: teams already write work up as issues/epics/design docs. Starting a spec from one only
  works with careful phrasing; nothing records where content came from.
- The spec is built to the design document `design/epics-and-seeded-specs.md` (source `design`,
  draft, committed by the user). The spec should reference it, not copy it.

## Decisions from the design conversation (this session)

- **One standalone spec, existing method.** User: "we should follow the existing method until this
  spec has been implemented" — no epic/split used to specify this work itself.
- **Split triggers** (now in the design, `### Split triggers`): a gate (agent must be able to name
  ≥2 specs, each with its own independently verifiable AC), strong signals (any one), weak signals
  (two or more), counter-signals, and "supporting work never counts".
  - User's concern that prompted the supporting-work rule: "if I created an update which would also
    update the docs, this would automatically create an epic?" → No. Code + its docs is one spec.
    Surfaces/repos removed as signals; they matter only via independence.
  - Nothing is ever automatic: always an offer; re-offer after decline only on a new strong signal
    or new independent requirement group.
- **Seeding is a prompt/skill concern, not a CLI feature.** User: "I don't really see why we need
  cli commands here, this is a prompt issue" and "we really need more an instruction in the skill
  on how to progress with this process".
  - Skill: recognise a source however phrased, fetch, check child items (offer split), propose a
    name, `spec new --data '{"name":…,"sources":[{"uri":…}]}'`.
  - Interview step: seed work files from the source, list gaps, ask only about gaps; later steps
    confirm drafts. Lives in the step so it survives resume.
  - CLI's only part: accept `sources` on `spec new`, stamp `retrieved_date`, write to frontmatter.
    `sources` must be added to closed `yamlShape` and `UpdateOptions`.
- **Tool-agnostic fetching.** User: "we should not be specific about what tool or process is used to
  fetch or read the issue, the agent should use its own tools … it could be a linear issue".
  Design names what to collect (title, body, discussion, child items, stable URI), not how.

## Alternatives rejected

- `spec new --from <issue-url>` / `--file <staged source>` + `source` in workflow state: rejected —
  CLI stays network-free and seeding needs no CLI surface beyond `sources`.
- Counting surfaces/repos as split signals: rejected — false positives on code+docs changes.

## Open items carried from the design

- Open decisions 1–7 in the design (closing an epic, naming after split, `status --spec-only`,
  splitting late, chaining beyond specify, detection before a spec exists, detection late).
- Deferred to the spec: detection sensitivity (`epic_split_threshold` favoured), `epic split` input
  format, source text snapshot/hash, posting back to source, implement wording, PR #65.

## Repos

- `spektacular` (tool, Go CLI) and `docs` (spektacular-website, Astro docs site, root
  /home/nicj/code/github.com/hivecommons/spektacular-website).

## User preferences seen this session

- Never commit unless explicitly asked (now in ~/.claude/CLAUDE.md). Project has `auto_commit: full`;
  user committed the design themselves before the workflow started.

## Interview answers (spec 000060)

- Scope: everything in the design, one spec, one release ("I am going to create a release which
  contains all this anyway"). Docs-site updates are in scope.
- Epic done: "once all the related specs milestones are done" → derived from plans.
- "We should be able to split an existing spec."
- Detection only when creating a spec; prompt "once the spec is done", "unless you explicitly
  instructed". AGENTS.md spec-trigger stays the same.
- Child items: present before split → epic + specs set up instead; added after an epic exists →
  suggest separate specs in that epic.
- Design updated to match (## Decisions); design ref recorded on the spec.
- Overview, requirements, ACs, constraints, technical approach all confirmed by the user ("ok") as drafted.
- Tech approach defaults confirmed: no source snapshot, no post-back, PR #65 separate.
- Non-goals to include: AGENTS.md spec-trigger unchanged; plans stay 1:1 with specs.

---

# Plan workflow: 000060_epics-and-seeded-specs (started 2026-10-01)

- Spec chosen by the user: 000060_epics-and-seeded-specs. User committed pending changes themselves
  before `plan new` (no commit_existing flag needed).
- Spec references design `design` / `epics-and-seeded-specs.md` — binding; architecture builds on it.
- Success metrics to carry into Testing Approach: (1) seeded spec asks only gap questions,
  (2) code+docs/tests/config never gets a split offer, (3) one status command shows whole epic.

## Plan discovery learnings (2026-10-01)

- Repos: spektacular (Go CLI) and docs (website). Research saved to .spektacular/work/000060_epics-and-seeded-specs/research.md.
- Key calls: separate `internal/epic` frontmatter type (shared Metadata.Specs clashes); hand-written
  `epic` verbs like `cmd/design.go`; schema bump 3->4 with `project3to4` (spec AC demands migration);
  `split` step on the linear path verification -> split -> finished, also reachable from section
  steps; split text in partials shared with spek-new skill; implement dependency check refuses with
  `dependencies_unmet`, override by re-run with `override_dependencies: true`; stub = unclosed
  draft not in active workflow; epic named after its first spec.
- Self-hosting trap: after schema bump, this repo needs `go run . migrate` (ask user first).
- Drafted sections saved in work dir: research, architecture, conventions, components, data_structures,
  implementation_detail, dependencies, testing_approach, milestones, tasks_plan, tasks_context, assumptions.
- 19 tasks / 4 milestones; ids from `plan task-id` are in tasks_plan.md (do not regenerate).
- 2026-10-01: plan.md, context.md, research.md committed to the plan store; work dir removed. Now in walkthrough.
- Walkthrough (2026-10-01), user decisions applied to spec, design and plan:
  - splits always act on a complete spec (a mid-workflow request waits for completion) and write complete `final` specs via `epic split` (all sections in the JSON); no stubs, no continuing stubs;
  - work that starts as items goes epic-first: `epic write` with no specs, then `spec new` with `epic` + `sources` per child; chaining = offer the next source item with no spec;
  - when starting a spec and epics exist, ask about joining one, then read the epic and its specs;
  - adding to a completed epic needs confirmation (`epic_complete`, `confirm_completed_epic`); completion stays derived;
  - the user wants `epic split` kept as one command (reliability over agent-composed steps).
- New task 9cdca194 "Guard additions to a completed epic"; task 31c5ced7 retitled "Start a spec with sources or in an epic".

---

# Implement workflow: 000060_epics-and-seeded-specs (started 2026-10-01)

- read_plan: structure valid, spec fully covered, first-task run (no `## Changelog` yet).
- Drift found; user chose "proceed, adapt during implementation":
  - Extra hard-coded `schema: 3` fixtures to sweep in the migrate task: cmd/version_test.go:198,214,246;
    internal/config/config_test.go:812,1081,1096,1142; internal/config/repo_test.go:35;
    internal/design/design_test.go:247; internal/project/init_test.go:309,409;
    internal/migrate/engine_test.go:190,273,406 (2->3 step tests may stay).
  - docs configuration.mdx currently says "Fourteen top-level keys" -> becomes Sixteen.
  - 08-verification.md uses `{{next_step}}`; only Go step order changes.
  - Knowledge entry architecture/working-with-files-from-steps.md:183 mentions `spec status` — update via spek-knowledge (offer to user) in the status-command task.
- auto_commit: full in this project; the CLI commits at workflow points. Never commit by hand.
- User chose "run without asking": loop tasks automatically; still stop on failures, drift, human tasks, decisions.
- Helper scripts (session scratch, recreate if lost): tick.py ticks a task + its ACs; addlog.py appends a changelog entry.
- Task 1 (depgraph) done 2026-10-01.
- Tasks 2-6 done 2026-10-01 (metadata epic/sources, epic config, internal/epic, epic commands + docTxn link writer, epic split).
- Task 7 (schema bump to 4 + project3to4) in progress. Workflow is driven with a pre-bump binary:
  /tmp/claude-1000/-home-nicj-code-github-com-hivecommons-spektacular/511e649f-2bdc-4441-8428-ca3119c7190d/scratchpad/spek-prebump
  (session scratch; if lost, `git stash` is NOT acceptable — instead ask the user to run the human migrate task).
  Next task 99ad63fd is human: user reviews `go run . migrate --dry-run` and approves `go run . migrate`.
- 2026-10-01 user: "I will run everything myself, but please go ahead and complete the code."
  -> Do NOT run `migrate`, `init`, harbor, or advance the workflow FSM past the human task (advancing triggers auto_commit).
  -> Implement + test + verify each remaining agent task; tick it and append its changelog entry via
     `plan file write` with the pre-bump binary (no FSM transition). Human tasks stay unticked for the user.
  -> Workflow state is parked at update_changelog/analyze before task 99ad63fd (human migrate).
  -> Skill copies (.claude/skills, .bob/skills) and this repo's AGENTS.md managed sections need `init` — user runs it.
- Recorded (ticked + changelog): 31c5 spec new sources/epic, e343 status report, bf23 split step.
- Docs tasks 56ba (epics page) and b949 (seeding/settings) written in docs repo (uncommitted, on main), build+astro check green;
  NOT yet recorded: reconcile the `dependencies_unmet` JSON sample in docs epics.mdx with the real CLI message after 0a3c lands.
- Guard 9cdc code written (refuseCompletedEpic in cmd/epic_link.go; wired into epic write / epic split / spec new); tests pending.
- 2026-10-01: ALL agent tasks implemented, tested, verified (go test ./... green x2; docs build + astro check green) and recorded
  (ticked + changelog entries) via plan file write. Remaining unchecked: 99ad63fd (human: migrate this repo's config) and
  e4194c32 (human: run harbor suites). No git commits made; docs repo changes are uncommitted on its main branch.
- To resume once the user has run `go run . migrate`: `go run . implement goto --data '{"step":"analyze"}'` picks up the
  human migrate task (tick it), then harbor (tick after the user runs it), then test_plan, update_feature_changelog,
  reconcile_spec, finished. Auto-commit fires at milestone boundaries when the FSM advances.
- Still for the user: `go run . init` to regenerate .claude/.bob skill copies and AGENTS.md managed sections (restore
  agent/written_by/skills_version in config.yaml afterwards); knowledge entry architecture/working-with-files-from-steps.md:183
  mentions retired `spec status` (offered via spek-knowledge).
