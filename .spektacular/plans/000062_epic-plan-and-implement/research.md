---
created_date: "2026-10-04"
document_status: final
closed_date: "2026-10-04"
---

# Research: 000062_epic-plan-and-implement

## Alternatives considered and rejected

### How several plan workflows run at once in one working copy

- **One `state.json` holding a map from name to state, plus a lock** (`{"workflows": {"<kind>:<name>": State}}`). Rejected:
  - Every `goto` would have to read, modify and write one shared file under a lock. Today `saveState` is a plain `os.WriteFile` with no lock (`internal/workflow/state.go:48-61`).
  - `gotoWithAutoCommit` snapshots and restores the whole file when a commit fails (`cmd/autocommit.go:152-163`). With a shared file, that restore would roll back the other workflows.
  - It needs a state-format migration (`internal/migrate`).
  - A shared "active" pointer is exactly what parallel runs cannot share.
- **An environment variable or global flag that selects the workflow** (`SPEK_WORKFLOW=<name>`, or a persistent `--workflow` on `rootCmd`, `cmd/root.go:353`). Rejected as the primary mechanism:
  - It is invisible in the step instructions. A subagent that forgets to export it silently drives the shared slot.
  - The instructions are the agent's only source of truth for every `goto` it runs. An explicit name in `--data` shows up in every rendered `goto` line, and tests can check for it (`cmd/instruction_contract_test.go:313-333`).
- **Making every workflow (spec, plan, implement) use a state file per name.** Rejected:
  - The blast radius is too large: resume, the kind guard, `status` with no name, the session log, and the tests that pin single-state behaviour (`cmd/cross_kind_test.go`, `cmd/resume_test.go`, `cmd/startgate_test.go`, `cmd/status_test.go`, `cmd/status_address_test.go`, `templates/skill_resume_test.go`).
  - None of that is needed for a run started on its own. Only epic-run plan children need parallel state.
- **Planning each spec in its own git worktree as well.** Rejected: the spec's constraint says "Parallel planning happens in the project's own working copy."

### Where epic orchestration lives

- **Extending `spek-plan` and `spek-implement` to accept an epic.** Rejected:
  - Their per-spec contract is pinned by tests:
    - mandatory walkthrough wording (`internal/steps/plan/steps_test.go:426-450`);
    - "ask the user" between tasks (`internal/steps/implement/steps_test.go:384-391`);
    - "never choose for the user" on uncommitted changes and dependency overrides (`templates/skills/workflows/spek-implement/SKILL.md:54-71,113-134`).
  - Skills are rendered once at install with only `{{command}}` (`internal/agent/skills.go:51`), so they cannot branch at runtime.
  - The spec's constraint is that children run the standard skills. Folding the orchestrator into those same skills would blur which role an agent is playing.
- **Only a library playbook (`templates/skills/skill_plan-epic.md`, served by `spektacular skill`).** Rejected as the entry point:
  - Library skills are served raw with no mustache rendering (`cmd/skill.go:27-65`), so there is no `{{command}}`.
  - They are not installed or version-checked, and plain wording cannot trigger them.
- **A standalone CLI command that runs the whole epic itself** (`spektacular epic plan <name>` spawning agents). Rejected by the spec's constraint: "driven by an agent skill… not a new standalone CLI command that runs on its own." The CLI never spawns agents.

### Working out what is outstanding

- **Each orchestrating agent works out the plan/implement order itself from today's `status` JSON.** Rejected. Today's report falls short in four ways (the plan instead extends `status` with a `run` view that answers all four):
  - `status` gives `ready` for implementing only (`internal/status/report.go:305-316`).
  - It counts a draft plan from an unfinished plan workflow as `planned` (`internal/status/classify.go:57-77`).
  - It does not re-validate the graph on read: no cycle check (`internal/epic/epic.go:82-122` does not call `Validate`).
  - It reports `implemented` before the implement wrap-up has run (ticking happens at `update_plan`, `internal/steps/implement/steps.go:27-42`).
  - Leaving this to agent judgement goes against the house style that the CLI owns deterministic state.
- **A new `epic next` command backed by a new `internal/epicrun` package** (the original draft). Rejected during the walkthrough at the user's suggestion:
  - it would add a second "where does work stand" command;
  - the epics design deliberately folded every status and export command into the one `status` command;
  - the same derivation fits beside the existing classifier in `internal/status`, where the implement check and the completed-epic guard already share it.
- **A separate progress file for each epic run.** Rejected by the spec's Technical Approach, which prefers deriving it from the existing record, because that is what makes a repeated request resume.

### Combining parallel implementation

- **Leaving every worktree branch unmerged until the end.** Rejected:
  - A dependent's `implement new` checks dependencies against its own working copy's stores (`cmd/implement.go:262-317`, `status.DependenciesOf`).
  - So a dependent's worktree must be cut from a main line that already contains its dependencies, and the spec requires merging at every junction.
- **Overriding dependencies in the children (`override_dependencies:true`).** Rejected:
  - It is refused under `epic.strict_dependencies` (`dependency_override_refused`).
  - It would write a false override note into each changelog.

## Chosen approach — evidence

**One state file per workflow is cheap and has a precedent**
- The guided repo add already keeps its own `.spektacular/repo-state.json` "so an add can be started while one of those is in progress" (`cmd/repo.go:184-189`).
- `workflow.New`, `probeResume`, `guardKind` and `gotoWithAutoCommit` all take `statePath` as a parameter (`internal/workflow/workflow.go:80`, `cmd/resume.go:164`, `cmd/resume.go:206`, `cmd/autocommit.go:57`). Only `stateFilePath` (`cmd/spec.go:94-96`) and its callers have to choose the path.
- `goto` copies every `--data` key except `step` into workflow data (`cmd/autocommit.go:84-88`). A `name` routing key has to be removed first, the same way `commit_message_from` is (`cmd/autocommit.go:68-70`).

**Shared scratch paths that would clash under parallel plans**
- `.spektacular/working-context.md`:
  - one per repo: `internal/workingcontext/workingcontext.go:27-32`;
  - the footer partial every step appends: `templates/partials/working-context-footer.md`, `internal/stepkit/stepkit.go:58,141-147`.
- Assembled plan documents use fixed names:
  - `.spektacular/tmp/plan_template.md`, `context_template.md` and `research_template.md`;
  - written by `templates/steps/plan/13-assemble.md:63-67`;
  - read by steps 14-17 and 19.
- The commit message path is fixed:
  - `.spektacular/tmp/git-commit-message.md` (`cmd/autocommit.go:29`, `internal/stepkit/stepkit.go:67`, `templates/partials/git-commit-message.md:24-28`).
- Already per name, so no clash: the `.spektacular/work/<plan_name>/` working files (plan templates 02-17).

**Auto-commit sweeps the whole work tree**
- `git add -A` then `git commit` on every dirty target (`internal/autocommit/git.go:132-142`, `internal/autocommit/commit.go:47-63`).
- Completion points (`internal/autocommit/points.go:29-36`):
  - plan `walkthrough→finished`;
  - implement `reconcile_spec→finished` and `update_changelog→finished`.
- `startGate` refuses `new` on a dirty tree (`cmd/autocommit.go:200-244`).
- This repo runs `auto_commit: full` (`.spektacular/config.yaml`).

**The walkthrough is the only route to `finished`**
- `templates/steps/plan/18-walkthrough.md:3,24`: mandatory, needs an explicit affirmative.
- `internal/steps/plan/steps_test.go:199-207,426-450`.
- `finished` marks the documents final: `templates/steps/plan/19-finished.md:26`.

**Implement already has an orchestrator hook**
- The single-task run is `implement new` with `task` (`templates/skills/workflows/spek-implement/SKILL.md:73-87`, worded "the user (or an orchestrator)").
- Autonomous looping is already allowed: "If the user has previously said 'run without asking'… skip the prompt and loop automatically" (`templates/steps/implement/07-update_changelog.md:93-94`).

**Worktrees isolate state for free, but tracked files will conflict on merge**
- The project root is the current directory (`cmd/root.go:266-290,321-340`, "deliberately no parent-directory search"), so a worktree root has its own `.spektacular/`.
- `.spektacular/state.json`, `working-context.md` and `repo-state.json` are tracked in git (`git ls-files .spektacular`).
- Two worktrees that both advance `state.json` will conflict when merged back. The children have to use the state file for their own name so their changed files never overlap.

**The epic graph and classification already exist**
- Graph: `internal/epic/epic.go:24-56` (`EpicSpec{Name, DependsOn}`) and `internal/epic/validate.go:23-54` (`epic_invalid`: unknown dependency, cycle via `depgraph.FindCycle`).
- Classification: `internal/status/classify.go:57-98` (`Classify`, `Describe`), `internal/status/report.go:212-243` (`DependenciesOf`), `report.go:282-331` (`buildTarget`: `ready`, `blocked_by`).
- `internal/depgraph/depgraph.go:11-58` has only `FindCycle`. There is no topological sort or layering.

**Skill installation**
- `internal/agent/skills.go:26-33` (`workflowSkills`) and `internal/agent/commands.go:142-149` (`workflowDescriptions`), which must agree (`internal/agent/agent_test.go:240-253`).
- Six-skill count assertions: `internal/agent/agent_test.go:136,227`, `claude_test.go:25-55`, `bob_test.go:24-52`, `codex_test.go:24-31`.
- Precedent for a description full of trigger phrases: spek-new (`templates/seeding_test.go:85-114`).

**Docs**
- `spektacular-website` repo, `src/pages/epics.mdx`:
  - sections alternate `surface` (`:17,72,123,150,206,249,310`);
  - `:29-31` "An epic is never planned or implemented itself";
  - `:223-226` chaining "never offers to plan or implement the next spek".
- Navigation: `src/components/Nav.astro:12-23`.

## Files examined

**Workflow state, resume and auto-commit**
- `spektacular:cmd/spec.go:94-96` — `stateFilePath` = `<root>/.spektacular/state.json`; `spec new` probe, gate and clear at 184-195, 269, 273.
- `spektacular:cmd/root.go:266-340` — config and project root come from the cwd, with no walk up; `readStateSnapshot` for the session log at 196-237.
- `spektacular:cmd/plan.go:49-194` — `plan new` (probe 84-95, startGate 115, clearState 119) and `plan goto` (schema only `step`, `guardKind` 185, `gotoWithAutoCommit` 192).
- `spektacular:cmd/implement.go:92-239,262-389` — `implement new` and `goto`; `refuseUnmetDependencies`; `refuseUnstartableTask`.
- `spektacular:cmd/resume.go:27-229` — resume report (`workflow_in_progress`, `cross_kind_workflow_in_progress`), `probeResume`, `clearState`, `guardKind`.
- `spektacular:cmd/autocommit.go:29,57-244` — fixed commit-message path, `gotoWithAutoCommit` (data copy 84-88, snapshot/restore 152-180), `startGate`.
- `spektacular:cmd/repo.go:184-250` — `repo-state.json`, the precedent for a separate state file.
- `spektacular:cmd/migrate.go:66-92` — **`installerFor` is declared twice; the `cmd` package does not compile at HEAD** (commit 1188110).
- `spektacular:internal/workflow/state.go:14-61` — `State{Kind, CurrentStep, CompletedSteps, Data}`; `InProgress`; unlocked `saveState`.
- `spektacular:internal/workflow/workflow.go:80-371` — `New(steps, statePath, …)`; save on `enter_state`; `commitTerminal`; next-action rendering (`nextActionForSteps` ~316, `walkthroughRevisionHint` ~303).
- `spektacular:internal/workingcontext/workingcontext.go:27-32` — fixed `.spektacular/working-context.md`; `Reset` is called only by spec `new`.
- `spektacular:internal/stepkit/stepkit.go:58,67,97,141-147` — footer append, commit-message partial, the instance name read from `Data["name"]`.
- `spektacular:internal/autocommit/points.go:20-50` — completion and milestone points.
- `spektacular:internal/autocommit/git.go:132-142`, `commit.go:47-63`, `targets.go:169-203` — `add -A` + commit per work-tree target; targets resolved through `git rev-parse --show-toplevel`.
- `spektacular:internal/sessionlog/record.go:40-84` — session ID `kind:name` taken from the state.

**Epics, status and dependencies**
- `spektacular:internal/epic/epic.go:24-162` — epic type, `Parse` (no validation), `Render`, `Stamp`.
- `spektacular:internal/epic/validate.go:11-54` — `epic_invalid` rules (empty name, nil `depends_on`, duplicate, unknown dependency, cycle).
- `spektacular:internal/depgraph/depgraph.go:11-58` — `FindCycle` only.
- `spektacular:internal/status/classify.go:21-143` — states, `Classify`, `Describe`, `PlanIsStale`.
- `spektacular:internal/status/report.go:16-439` — report types, `DependenciesOf`, `buildTarget`, `buildSpec`, `buildPlan`, `currentStep`, `matchingWorkflow`, `EpicComplete`, `BuildCurrent`, `ReadState`.
- `spektacular:internal/status/resolve.go:19-89` — name resolution (epic, then spec, then plan); `readEpic`.
- `spektacular:cmd/status.go:19-158` — `status [name] --format`; schema.
- `spektacular:cmd/epic.go:137-466` — `epic read` (raw), `list`, `write` (validates at 340), `delete`.
- `spektacular:cmd/epic_link.go:32-356` — `docTxn`, link checks, `refuseCompletedEpic`, `joinSpecToEpic`.
- `spektacular:cmd/implement_dependencies_test.go:235-253` — planning is never gated by dependencies.

**Plans, implement and changelogs**
- `spektacular:internal/plantask/plantask.go:102-354` — task checkbox parsing; open/total counts.
- `spektacular:internal/steps/plan/steps.go:304-340` — plan.md is first written at `write_plan` (draft) and closed at `finished`.
- `spektacular:internal/steps/plan/strategy.go:19-21` — `PathVars` exposes `plan_name`.
- `spektacular:internal/steps/implement/steps.go:27-249,303-338` — step order; `withDependencyOverride`; `finished` closes the changelog and test plan.
- `spektacular:internal/steps/implement/strategy.go:36-38` — changelog path.
- `spektacular:cmd/plan_task_id.go:24-48`, `internal/identifier/taskid.go:36-58` — random UUID task ids, safe to run concurrently.

**Templates and skills**
- `spektacular:templates/steps/plan/02-17,18-walkthrough.md,19-finished.md` — "proceed unless genuinely blocked" in every drafting step; mandatory walkthrough; fixed tmp names in 13-17 and 19.
- `spektacular:templates/steps/implement/01-12` — STOP points; the between-task question at 07:93-94; the single-task path at 07:65-83.
- `spektacular:templates/steps/resume.md`, `resume_implement.md`, `resume_mismatch.md` — the resume `goto` carries no name.
- `spektacular:templates/partials/working-context-footer.md`, `git-commit-message.md:24-28`, `version-check.md:1-5`.
- `spektacular:templates/skills/workflows/spek-plan/SKILL.md`, `spek-implement/SKILL.md`, `spek-new/SKILL.md` — per-spec stop points; orchestrator hook (implement :73-87); trigger-phrase description precedent.
- `spektacular:templates/skills/skill_spawn-planning-agents.md:23-24` — tells agents to check `.spektacular/plans/` and `.spektacular/specs/` directly, against the store-access rule (left as is; not this spec's scope).
- `spektacular:internal/agent/skills.go:23-69`, `commands.go:142-185`, `claude.go`, `bob.go`, `codex.go` — the install pipeline.

**Tests that pin behaviour**
- `spektacular:internal/agent/agent_test.go:86-283`, `claude_test.go:25-55`, `bob_test.go:24-52`, `codex_test.go:24-31` — the skill list and counts.
- `spektacular:internal/agent/instruction_surface_test.go:17-250,1137-1221` — forbidden substrings, addressing, cross-repo wording, no direct removal.
- `spektacular:cmd/instruction_contract_test.go:30-697` — step template table, footer, `goto` prefix regex (spec|plan|implement|repo only), working-context naming per workflow, auto-commit rendering.
- `spektacular:templates/skill_resume_test.go:13-94`, `skill_list_command_test.go`, `data_payload_wellformed_test.go:40`, `split_test.go:161` (spec chaining must not offer to plan or implement).
- `spektacular:internal/steps/plan/steps_test.go:82-127,199-207,426-522,703` and `internal/steps/implement/steps_test.go:384-449`.

**Docs**
- `docs:src/pages/epics.mdx:4,12-15,17-381` — the existing epics page; the sentences that this feature makes untrue.
- `docs:src/pages/plan-tasks.mdx:166,345,516` — status and the single-task implement docs.
- `docs:src/pages/how-it-works.mdx:79-110,399-458` — plan and implement steps.
- `docs:src/components/Nav.astro:6-26` — the hard-coded navigation.
- `docs:CHANGELOG.md` — one entry per spec.
- `spektacular:README.md:21-41,75-91` — How It Works / status, and the quick start.

## External references

- `git worktree` (git-scm.com/docs/git-worktree): `add -b <branch> <path> <commit-ish>`, `remove`, `list --porcelain`.
  - Why it matters: each implement child gets its own working copy and `.spektacular/`.
  - `list --porcelain` lets a repeated request find a worktree left from an interrupted run.
- `git merge --no-ff` / `git merge --abort`: combine a child branch into the main line; on a conflict, abort and report it rather than resolve it (spec requirement).

## Prior plans / specs consulted

- `000060_epics-and-seeded-specs` (plan, final). Establishes:
  - the epic store, `specs[{name, depends_on}]` and validation;
  - `status <name>` and the one shared classifier;
  - the implement dependency check (`dependencies_unmet`, `override_dependencies`, `strict_dependencies`).
  - Its out-of-scope note, "No concurrent workflows for an epic's specs… belongs to issue #62", is exactly what this plan now takes on.
- `000057_git-commit` (plan, final). Establishes:
  - auto-commit modes and points;
  - the commit message staged under `.spektacular/tmp/` and validated to name the spec;
  - the start gate;
  - that `state.json` and `working-context.md` are tracked.
  - Out of scope there: "Running workflows in separate git worktrees. Deferred to a future spec."
- The `epics-and-seeded-specs.md` design (source `design`, binding):
  - an epic is never planned or implemented itself; each spec is;
  - plans stay one-to-one with specs;
  - dependencies constrain only implementation;
  - `status` derives every state;
  - its non-goal "No concurrent workflows for an epic's specs (that belongs with #62)" is the work this spec picks up.
- `000048_interruptible-workflow` does not exist as a plan. Resume semantics were read from the code instead.

## Open assumptions

- Claude Code background subagents can stop with a question in their final report and be continued with the answer (SendMessage). The orchestrating skill is written in agent-neutral terms ("your agent orchestration capability"), as `skill_spawn-planning-agents.md` is. If an agent cannot continue a stopped subagent, the relay degrades: the subagent is restarted on the same spec and resumes its in-progress plan from its own state file.
- Every registered repo a spec touches can be given a git worktree. That holds for a repo outside the project's own git tree too: `docs` is located at `../../spektacular-website/.spektacular` and has its own `.git`. It also holds for a materialized git-source clone under `.spektacular/repos/`, which is its own repository. A repo whose source is not on disk cannot get one, and `epic worktree` refuses with a `repo add` next step.
- Every reader of repo locations goes through `repo.New`, so applying the overlay there redirects them all. This covers `repo list`, `LocalRoot` and `LocalSource`, the per-repo knowledge and changelog stores, and `autocommit.Targets`. If implementation finds a reader that resolves locations another way, STOP and ask before working around it.
- `.spektacular/state.json` and `working-context.md` stay tracked in git (000057). The plan routes children to per-name files rather than untracking these.
- `go test -shuffle=on ./...` passes at HEAD once the duplicate `installerFor` is removed. Not verified beyond the compile error.

## Drafting assumptions

### Children use one state file per workflow; standalone runs keep state.json (discovery)
- **Decision**: Concurrency is opt-in for plan and implement workflows started by an epic orchestrator. They keep their state in a file for their own name, and every `goto` carries `name`. A run started on its own keeps using the shared `.spektacular/state.json` exactly as today.
- **Rationale**: `repo-state.json` is already a separate state file (`cmd/repo.go:184-189`), and every state function takes a `statePath`. This limits the blast radius and avoids a state-format migration. Per-name files also keep worktree branches from conflicting on tracked state when they are merged.
- **Rejected**: Moving every workflow to per-name state, which touches resume, kind guards and many pinned tests for no user benefit. A shared map plus a lock, which loses updates and cross-rolls-back on commit failure. An environment-variable selector, which is invisible in instructions.

### The pre-existing compile error is fixed as part of this plan (discovery)
- **Decision**: The first task removes the duplicate `installerFor` in `cmd/migrate.go`.
- **Rationale**: The project convention says tests must pass before work is done, and nothing compiles until this is fixed. It is a one-line, obviously unintended duplicate.
- **Rejected**: Leaving it for a separate fix, which would block every verification step in this plan.

### Chosen direction: two new epic skills backed by deterministic CLI support (architecture)
- **Decision**: Add `spek-plan-epic` and `spek-implement-epic` as orchestrator skills. Children run the unchanged `spek-plan` and `spek-implement` skills. The CLI gains:
  - orchestrated workflows, with lane state per name;
  - a `run` view in `status`, which reports what is done, in progress, ready, blocked or broken, for planning and for implementing;
  - `epic worktree` and `epic merge`;
  - template hooks for a deferred walkthrough and an implement run that does not stop between tasks.
- **Rationale**:
  - The spec's testable rules (order, outstanding work, resume, refusals, conflicts) become code covered by `go test` rather than prose in two skills.
  - Standalone runs stay exactly as today.
  - The spec's Technical Approach asked the plan to choose between extending the existing skills and adding new ones.
- **Rejected**:
  - Agent-only orchestration over `status`: `status` cannot tell a draft plan from a final one, or wrap-up from implemented, and cannot see state inside a worktree.
  - An epic mode in the existing skills: their stop points are test-pinned, and skills cannot branch at runtime.

### Every plan and implement goto carries the workflow name (architecture)
- **Decision**: Plan and implement step templates render `"name"` in every `goto --data`. `goto` routes to that name's lane file if one exists. Otherwise it uses `state.json` and checks that the name matches.
- **Rationale**:
  - One uniform rendering avoids conditional mustache inside JSON.
  - An agent always holds the routing key, and tests can check for it.
  - A name mismatch against `state.json` becomes a caught error instead of silently driving the wrong workflow.
- **Rejected**:
  - Rendering the name only in orchestrated mode: this needs fragile triple-mustache inside JSON braces.
  - An environment-variable selector: it never appears in the instructions.

### An orchestrated plan defers its walkthrough to the epic review (architecture)
- **Decision**: In an orchestrated plan, the walkthrough step returns a short summary to the orchestrator and advances to `finished` without asking for sign-off. The orchestrator shows every summary to the user at the end of planning and applies requested changes through `plan file write`.
- **Rationale**:
  - The spec requires that the only stop is the end-of-planning review, plus genuine questions.
  - A plan must reach `final` so that dependents can be planned after it, and so that implementing can recognise it as planned.
- **Rejected**:
  - Parking plans at the walkthrough step: dependents would never see a final plan, and a lane would stay open until the end.
  - Keeping per-spec walkthroughs: this contradicts "fewer interruptions".

### "Genuine open question" means the STOPs the step templates already define (architecture)
- **Decision**: A genuine open question is one of the existing STOPs:
  - an unresolved design reference;
  - a drafting decision with no reasonable default;
  - a disagreement with a referenced design;
  - an implementation mismatch;
  - a failed verification;
  - a task that has grown beyond its scope.

  Everything else is decided, recorded in assumptions.md, and surfaced in the end-of-planning review.
- **Rationale**: The drafting steps already say "proceed unless genuinely blocked", and the spec's Technical Approach asked the plan to define the term.
- **Rejected**: A new, separate threshold setting. It would add configuration that no requirement asks for.

### The run view lives in `status`, not a new `epic next` command (walkthrough, user direction)
- **Decision**: `status <name>` gains a `run` block on each spec (planning and implementing states, `waiting_on`, step, root, touched repos) and on the epic (order, counts, `dirty`, `problems`). `status` reports and never refuses. The implement-epic skill refuses when a problem blocks implementing.
- **Rationale**:
  - The user asked whether the information could be added to `status` to avoid a new component.
  - The epics design made `status` the single place a caller learns where work stands.
  - The logic belongs beside the classifier the implement check already shares.
- **Rejected**:
  - A separate `epic next` command plus an `internal/epicrun` package: a second status-like surface, and a new package.
  - Refusing inside `status`: it would break the design's "status reports, never refuses" shape, and every existing caller.

### Every repo a spec touches gets its own worktree, with a repo overlay (walkthrough, user direction)
- **Decision**:
  - `epic worktree` creates a `spek/<spec>` worktree in every repo the spec's plan touches.
  - It writes a repo overlay into the project worktree, kept out of git, so every repo resolves to that spec's worktrees inside it. `repo.New` applies the overlay.
  - `epic merge` dry-runs a merge in every repo with `git merge-tree`, and merges all of them or none.
- **Rationale**:
  - During the walkthrough the user asked for all repos to get their own worktree. Specs often touch several repos, such as an API and a command, or code and docs.
  - Running specs that touch another repo one at a time would have taken away most of the parallelism.
  - Applying the overlay at `repo.New`, the single place repo locations are resolved, redirects every reader at once.
  - Merging all repos or none means a conflict never leaves a spec half-merged.
- **Rejected**:
  - Running such specs alone in the main working copy (the original draft): it serialises any spec that touches docs or another repo.
  - Mirroring the directory layout so relative locations happen to resolve: this breaks for absolute locations and git-source clones.
  - Splitting other-repo work into its own spec during an epic split: this contradicts the epics design ("code plus its docs is one spec"), and it is not needed once each repo has a worktree.

### Worktrees live under .spektacular/worktrees/<spec>/<repo> on branch spek/<spec> (architecture)
- **Decision**: Every worktree is placed under the project's own `.spektacular/` directory, kept out of git, one directory per touched repo. Each is created from that repo's current HEAD.
- **Rationale**:
  - `status` can find it deterministically on resume.
  - Keeping it inside the project avoids writing outside it.
- **Rejected**: A sibling directory next to the project, which would scatter state outside the project.

### Plan lanes commit only their own paths, under a lock (architecture)
- **Decision**: When an orchestrated plan completes, it commits only:
  - its plan store directory;
  - `.spektacular/work/<name>/`;
  - its lane files.

  The commit runs under a project-level lock file.
- **Rationale**: Under `git add -A`, parallel plans in one working copy would sweep each other's in-flight files, and would contend for `index.lock`.
- **Rejected**: Disabling auto-commit for lanes and committing once at the end. That loses the per-plan commit point that resume and history rely on.

### Children hand back with DONE / QUESTION / FAILED lines (data_structures)
- **Decision**: A child subagent ends with a final message whose first line is `DONE:`, `QUESTION:` or `FAILED:` followed by the spec name. The orchestrator continues a child that asked a question by sending it the user's answer.
- **Rationale**: A fixed, parseable hand-back keeps the question relay and stop-on-failure rules unambiguous across agents (Claude, Bob, Codex). It also needs no CLI channel.
- **Rejected**: A CLI-mediated question queue (a file or command). It adds state the spec does not ask for, and subagent tools already support stop-and-resume.

### Lane files use the same JSON format as state.json (data_structures)
- **Decision**: A lane's state file is a plain `workflow.State`. It is marked as a lane by `"orchestrated": true` in its data and by where it lives.
- **Rationale**: The engine (`workflow.New`), the status reader and resume can reuse it with no format change and no migration.
- **Rejected**: A new lane envelope type. It would need parallel load and save code.

### Worktrees are kept out of git through info/exclude as well as the template .gitignore (tasks)
- **Decision**: `epic worktree` makes sure `.spektacular/worktrees/` is listed in the repository's `info/exclude`. It is also added to the embedded `.spektacular/.gitignore` for new projects.
- **Rationale**: A worktree nested inside the main working copy and left unignored would be swept into commits by `git add -A` as an embedded repository. `info/exclude` covers existing projects without rewriting tracked files, which also respects "plans never change the active install".
- **Rejected**: Asking users to edit `.gitignore` by hand. Placing worktrees outside the project.

### A lane's files are deleted when it finishes (tasks)
- **Decision**: When a lane reaches `finished`, its `.json` and `.md` files are removed. For plan lanes, the removal is included in that lane's path-scoped commit.
- **Rationale**: Finished lanes would otherwise pile up in the tracked tree. Without the lane file, `status` falls back to the document state, which says the same thing.
- **Rejected**: Keeping finished lanes. This clutters history and makes a finished lane look the same as one waiting to be resumed.

### No separate human task for the end-to-end check (tasks)
- **Decision**: The end-to-end check with a live agent is left to the implement workflow's test plan, which the testing approach already marks "Manual — captured in the implementation test plan". It is not a plan task.
- **Rationale**: The implement workflow produces the test-plan artifact for manual checks. A duplicate human task would block completion of the plan for no benefit.
- **Rejected**: A `human` task "run an epic end to end".

## Rehydration cues

- `spektacular spec file read 000062_epic-plan-and-implement`
- `spektacular design read --data '{"source":"design","path":"epics-and-seeded-specs.md"}'`
- `spektacular knowledge always-applied --tier repo --filter spektacular --filter docs`
- `spektacular plan file read 000060_epics-and-seeded-specs plan`; `spektacular plan file read 000057_git-commit plan`
- Re-read: `cmd/autocommit.go`, `cmd/resume.go`, `cmd/plan.go`, `cmd/repo.go:184-250`, `internal/status/report.go`, `internal/epic/validate.go`, `internal/agent/skills.go`, `templates/steps/plan/18-walkthrough.md`, `templates/steps/implement/07-update_changelog.md`, `templates/skills/workflows/spek-plan/SKILL.md`, `spek-implement/SKILL.md`.
- `go build ./cmd/...` shows the duplicate `installerFor` until it is fixed.
- `spektacular skill spawn-planning-agents`, `spektacular skill spawn-implementation-agents`.
