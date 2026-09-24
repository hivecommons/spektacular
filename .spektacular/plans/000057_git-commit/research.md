---
created_date: "2026-09-22"
document_status: final
closed_date: "2026-09-22"
---

# Research: 000057_git-commit

## Alternatives considered and rejected

- **Agent runs `git add -A && git commit` itself, told to by step-template prose.** Rejected for four reasons. Spektacular could not enforce "failure stops the workflow": the agent could ignore the failure and `goto` anyway. Enumerating repos, deduplicating shared worktrees and skipping non-git repos would all be left to agent judgement. The Go test suite could not verify any of it. And the constraint that it works the same for Claude, Bob and Codex would rest on each agent's shell tool behaving alike. Evidence: step instructions are the only agent-neutral surface (`internal/stepkit/stepkit.go:89-111`), while shell behaviour is not under Spektacular's control.
- **Commit inside a step callback (for example the `finished` callback).** Rejected. Callbacks run as `before_<event>` (`internal/workflow/workflow.go:115-133`), before `enter_state` saves `state.json` (`workflow.go:147-156`). `.spektacular/state.json` and `.spektacular/working-context.md` are tracked in git (`git ls-files .spektacular`). A commit made inside the callback would therefore always leave `state.json` dirty. The next workflow's start check would then flag uncommitted changes every time, which breaks "clean tree, no question" in practice. A second problem is that callbacks write their instruction JSON straight to stdout (`internal/output/writer.go:23-26`), so a commit failure after that point would print two JSON documents.
- **A standalone `git commit` CLI command the agent calls when told to.** Rejected as the enforcement point. Halting would still depend on the agent choosing not to `goto` after a failure, and commits would not be tied to transitions. Its execution code survives as the internal committer package that the transition hook calls.
- **Widening `repo.GitRunner` with status/add/commit.** Rejected. Its doc comment says it is deliberately narrow and must never grow into a git façade (`internal/repo/git.go:17-21`). The chosen path extracts the exec helper (`git.go:46-71`) into a shared package and adds a separate narrow commit-side interface.
- **Bumping the config schema and adding a migration step for the new key.** Rejected. An optional key with a default needs neither. `ParseYAMLFile` unmarshals over `NewDefault()` (`internal/config/config.go:341-373`), and `spec_trigger_threshold` was added the same way with no step (`internal/migrate/registry.go:59-68`). A bump would force every existing project through `migrate` (`internal/config/schema.go:87-97`).
- **Milestone boundaries detected only by agent judgement in the `update_changelog` template.** Rejected in favour of Go detection from `plan.md`'s checkbox structure. The success metric "every milestone ends with a commit" needs a deterministic guard. The plan scaffold fixes the structure (`templates/scaffold/plan.md:109-131`: `### Milestone N:` then `#### - [ ] Phase N.M`), and the update_changelog template already counts those headings (`templates/steps/implement/07-update_changelog.md:91-111`).
- **The start-of-workflow question as a new FSM step.** Rejected. A step would change every step table and harbor `EXPECTED_STEP_ORDER` oracle. It would also run after `spec new` has already written the scaffold and reset working context (`internal/steps/spec/steps.go:69-100`), so the "before any workflow work" requirement would be violated. The existing resume-report pattern (`cmd/resume.go:67-91,144-163`) already shows how a `new` can refuse with an error-shaped report, change nothing on disk, and tell the agent to ask the user and re-run with a flag.

## Chosen approach — evidence

- A step-callback error vetoes the transition before state is saved (`internal/workflow/workflow.go:115-133`, test `TestFailedStepDoesNotAdvancePersistedState` at `internal/workflow/workflow_test.go:287`). Post-transition work needs its own rollback, because state is saved in `enter_state` (`workflow.go:147-156`) and `commitTerminal` (`workflow.go:287-303`).
- A resume report is an error-shaped response that changes nothing and tells the agent to ask and re-run (`cmd/resume.go:67-91`), a direct model for the uncommitted-changes report.
- The `new` handlers share one shape: `loadConfig`, then `resumeOrClear`, then `workflow.New`, then `wf.Next()` (`cmd/spec.go:156-258`, `cmd/plan.go:68-145`, `cmd/implement.go:73-157`). `resumeOrClear` deletes a finished `state.json` (`cmd/resume.go:151-157`), so the dirty check has to run before that deletion.
- The `goto` handlers copy every non-`step` `--data` key into persisted workflow data (`cmd/plan.go:203-207`). A `commit_message` key must therefore be pulled out before that loop, or it persists into state.
- The registered repos come from `repo.New(cfg, root, git).Entries()` and `LocalSource(name)`, which never clone (`internal/repo/set.go:66,80,133`). `repo list`'s `root` is `ResolvedRepo.Source` (`cmd/repo.go:393`). The project itself is always a registered repo, and there is no empty-registry case (`internal/config/config.go:588-592`, `internal/project/init.go:57-58`).
- Enum settings follow a set pattern: constants, a field, a default in `NewDefault`, and a `switch` in `Validate` that also accepts `""` (`internal/config/config.go:22-26,277,292,553-557`). Tests model on `TestFromYAMLFile_UnknownSpecTriggerThresholdReturnsError` (`internal/config/config_test.go:92-106`).
- `workflow.Config` is built field by field at each construction site (`cmd/spec.go:243,313`, `cmd/plan.go:131,200`, `cmd/implement.go:143,212`, `cmd/repo.go:248,313`). Only `command` reaches templates (`internal/stepkit/stepkit.go:89-97`).
- The footer partial is appended programmatically to every non-terminal step (`internal/stepkit/stepkit.go:51,105-111`, `templates/partials/working-context-footer.md`). This is the model for appending a commit-message instruction partial on the steps that lead into a commit point.
- Real-git test helpers set identity through env and null global/system config (`internal/repo/git_integration_test.go:17-64`), and CI's `golang:latest` image ships git (`dagger/main.go:185-190`).
- Git behaviour relied on: `git status --porcelain` lists modified and untracked files; `git add -A` stages new, modified and deleted files; `git commit -F <file>` runs hooks and uses the user's identity and signing config unless overridden; `git rev-parse --show-toplevel` fails outside a work tree.

## Files examined

- `internal/config/config.go:16-26,262-323,341-373,549-584,588-604,763-778` — enum constants, Config struct, defaults, load-over-default, Validate, repo validation, writer.
- `internal/config/schema.go:11-21,87-97` — schema constants and the strict format check.
- `internal/config/config_test.go:25-34,72-106` — default and enum-validation test style.
- `internal/migrate/registry.go:6-8,31-68`, `internal/migrate/registry_test.go:13` — step registry and contiguity pin (untouched).
- `cmd/migrate_test.go:227-275` — asserts migrate only touches `skills_version`/`written_by` (unaffected).
- `internal/workflow/workflow.go:16-41,73-160,165-303` — Config, callback registration as `before_`, enter_state save, Next/Goto/renderStep/commitTerminal.
- `internal/workflow/state.go:25-61` — InProgress and saveState.
- `internal/stepkit/stepkit.go:51,66-115,143-166` — WriteStepResult vars, footer append, mustache partials.
- `internal/steps/spec/steps.go:25-39,69-100,163-185` — spec step table, `new` writes scaffold and resets working context, `finished` closes the spec.
- `internal/steps/plan/steps.go:31-54,83-87,296-335` — plan step table, `new` auto-advances, `finished` closes the three docs.
- `internal/steps/implement/steps.go:22-37,67-71,115-119,152-181` — implement loop via the multi-source `analyze` step, `finished` closes the changelog and test plan.
- `templates/steps/implement/06-update_plan.md`, `07-update_changelog.md:91-111` — phase tick and loop decision.
- `templates/scaffold/plan.md:109-131` — milestone and phase heading structure.
- `templates/partials/working-context-footer.md` — programmatically appended partial.
- `cmd/spec.go:156-332`, `cmd/plan.go:68-223`, `cmd/implement.go:73-235` — new/goto handlers.
- `cmd/resume.go:67-91,140-192` — resume report, resumeOrClear, guardKind.
- `cmd/root.go:92-140,239-245,263-331,347-362` — runRoot error envelope, debug dir, loadConfig, projectRoot, gate, command registration.
- `cmd/repo.go:18-50,142-182,330-407,438` — command group pattern, `repoGit` swap var, repo list resolution.
- `cmd/repo_test.go:43-117`, `cmd/spec_test.go:26`, `cmd/root_test.go:35-100` — test helpers and fixtures.
- `internal/repo/git.go:17-83` — GitRunner and exec helper.
- `internal/repo/set.go:27-36,66-197` — Set API.
- `internal/repo/git_integration_test.go:17-64` — real-git helpers.
- `internal/output/writer.go:12-85` — Writer writes immediately; ErrorResponse builders.
- `cmd/instruction_contract_test.go:166` and `stepTemplateTable` — template contract tests to keep green.
- `.spektacular/.gitignore` — `*.log` (debug logs), `repos/` ignored; `state.json` and `working-context.md` tracked.
- `docs:src/pages/configuration.mdx:4,28-81,83-278,135-147` — config example, key list sub (the "Thirteen top-level keys" count), `spec_trigger_threshold` ConfigKey pattern.
- `docs:src/components/sections/ConfigKey.astro:2-6` — props (`defaultValue` rendered as HTML).
- `docs:src/pages/how-it-works.mdx:330-341` — the Implement pipeline stage body (candidate for a one-line mention).
- `docs:CHANGELOG.md` — per-spec prose entries, newest first.
- `docs:Makefile` — `make build`, `make check` (astro check).

## External references

- git-status porcelain v1 format (`git status --porcelain`) — a stable, machine-readable dirty check that includes untracked files (`??`).
- git-commit(1) — `-F <file>` reads the message from a file; hooks run unless `--no-verify` is given; identity and `commit.gpgsign` come from the user's config.
- git-rev-parse(1) `--show-toplevel` — tells a work tree from a non-git directory, and dedupes registered repos that share one work tree.

## Prior plans / specs consulted

- `000053_config-schema-versioning-and-migrations` (plan) — rule: a schema bump happens only when the file format changes, and every bump ships a migrate step. Confirms an optional defaulted key needs neither. Also sets the docs configuration-page conventions (Label: file headings, ConfigKey slot bodies).
- `000057_git-commit` (spec) — the source of truth for this plan.

## Open assumptions

- The project repo that holds `.spektacular/` is always one of the registered repos (enforced by `validateRepos` and init seeding). If that ever stops holding, `state.json` commits would need the project root added explicitly.
- A registered directory that is not itself a git work tree but sits inside a parent work tree is treated as part of that parent (`rev-parse --show-toplevel` succeeds). Commits then land in the parent repo.
- One agent-written message is applied to every repo committed at a given point, rather than a message per repo.
- Commit-point transitions: spec `verification`→`finished`, plan `walkthrough`→`finished`, implement `reconcile_spec`→`finished`, and (full mode) implement `update_changelog`→`analyze`|`test_plan` when a milestone has newly completed.
- Git's own env handling (no identity override, no `--no-verify`) keeps hooks, identity and signing exactly as a manual commit would. `GIT_TERMINAL_PROMPT=0` only affects credential prompts, and a GPG pinentry that needs a TTY may fail. That failure surfaces as a commit failure, which is the required behaviour.
- The debug session logs under `.spektacular/debug` are `*.log` and git-ignored, so they never dirty the tree.

## Drafting assumptions

### Spektacular executes commits, agent only supplies the message (discovery)
- **Decision**: Go runs git (status/add/commit) at commit-point transitions. The agent writes the message and passes it as `commit_message` in the `goto --data`.
- **Rationale**: only Go can enforce "failure stops the workflow" and behave identically for Claude, Bob and Codex, while the spec's technical approach says the agent writes the message.
- **Rejected**: the agent running git itself (unenforceable, untestable); a Go fixed-template message (contradicts the spec).

### Milestone completion detected by Go from plan.md (discovery)
- **Decision**: in full mode, leaving `update_changelog` checks plan.md for a milestone whose phases are all ticked and that has not been committed yet (tracked in workflow data). When one exists, a commit message is required.
- **Rationale**: this is a deterministic guard for "every milestone ends with a commit", and the plan scaffold fixes the structure.
- **Rejected**: trusting the agent to notice a milestone boundary.

### Start check reuses the resume-report pattern (discovery)
- **Decision**: `new` refuses with an `uncommitted_changes` report and changes nothing on disk. The agent asks the user, then re-runs `new` with `"commit_existing": true|false`.
- **Rationale**: this runs before any workflow writes, needs no new step and no harbor step-order change, and the agent already follows this pattern for resume.
- **Rejected**: a new FSM step, which would run after spec `new` has already written files.

### No schema bump for the new key (discovery)
- **Decision**: add the key with default `off` and no migrate step.
- **Rationale**: it is optional and absent means default, matching the precedent of `spec_trigger_threshold`.
- **Rejected**: a schema bump, which would force every project through migrate.

### One message for every repo at a commit point (discovery)
- **Decision**: a single agent-written message (staged file passed as `commit_message_from`) is used for each repo committed at that point.
- **Rationale**: this is the simplest contract for the agent, and the message describes the work as a whole.
- **Rejected**: a per-repo message map, which is more complex and was not asked for.

### Chosen direction: Go-side commits at transitions with post-save commit and state rollback (architecture)
- **Decision**: the goto handler buffers output, snapshots state.json, transitions, then commits each dirty target; on failure it restores the snapshot and returns `auto_commit_failed`.
- **Rationale**: state.json and working-context.md are git-tracked, so a commit inside a before_ callback would always leave the tree dirty; post-save plus rollback keeps the tree clean and still halts on failure.
- **Rejected**: committing in the step callback; an agent-run git; a standalone commit command.

### Config key name `auto_commit`, top-level scalar (architecture)
- **Decision**: `auto_commit: off|workflow|full`, placed next to `spec_trigger_threshold`.
- **Rationale**: it matches the existing top-level behaviour switches and the spec's "automatic-commit mode" wording. yaml.v3 decodes the plain `off` into a string field as "off" and quotes it on marshal; a round-trip test pins this.
- **Rejected**: a nested `git:` section, not warranted for one key.

### Commit message passed by path (`commit_message_from`) (architecture)
- **Decision**: the agent stages the message under `.spektacular/tmp/` and passes its path. Go validates it, deletes it, and never persists it into workflow data.
- **Rationale**: this follows the "bodies by --from path" knowledge rule and avoids shell-quoting problems with multi-line JSON.
- **Rejected**: an inline `commit_message` string (quoting fragility); `--file`/`--stdin` (they persist into workflow data and stdin is banned).

### Go validates the message names the spec (and Milestone N) (architecture)
- **Decision**: refuse with `commit_message_invalid` when the message omits the spec name, or `Milestone N` at a milestone point.
- **Rationale**: this guards the success metric "every message traceable to spec and milestone" deterministically.
- **Rejected**: trusting prose instructions alone.

### Conventions selection (architecture)
- **Decision**: keep error-remediation, store-through-CLI, test-order, tests-green, docs content-outline, MDX, label-before-filename, and no-em-dashes. Drop alternate-section-background and site-layout.
- **Rationale**: the docs change is one ConfigKey entry inside an existing block, with no new band.
- **Rejected**: n/a.

### Several milestones completing at once must all be named (implementation_detail)
- **Decision**: when more than one uncommitted milestone is complete at a milestone point, the message must name each one, and all of them are recorded as committed together.
- **Rationale**: this is rare (it happens on resume); one commit keeps the history honest without inventing a split commit.
- **Rejected**: one commit per milestone at the same point, because the changes cannot be split by milestone after the fact.

### Start gate runs after spec name resolution (implementation_detail)
- **Decision**: the gate runs in `spec new` after the identifier is resolved and before the scaffold is written.
- **Rationale**: the pre-workflow commit message must name the spec, and ID resolution only reads.
- **Rejected**: running it before resolution, which would leave the message without the final spec name.

### No new harbor scenario; real-agent flow is a manual test-plan item (testing_approach)
- **Decision**: harbor oracles are unchanged; agent-driven commit conversations go into the implementation test plan.
- **Rationale**: the default mode is byte-identical and the step order is unchanged; the Go end-to-end tests cover the mechanics deterministically.
- **Rejected**: a new harbor scenario (~25-minute runs, not in CI) for behaviour already pinned in Go.

### Three milestones; docs folded into Milestone 3 (milestones)
- **Decision**: M1 completion commits (plus setting and engine), M2 start-of-workflow question, M3 milestone commits plus docs. `full` is accepted from M1 and behaves as `workflow` until M3.
- **Rationale**: each milestone is independently shippable and user-visible; docs describe all three values, so they land once `full` is real.
- **Rejected**: a separate docs-only milestone (too small to stand alone).

### Signing-pinentry behaviour left as the only open question (open_questions)
- **Decision**: treat an interactive-signing failure as an ordinary commit failure; any wording tweak is decided at implementation.
- **Rationale**: it can only be observed with a real signing setup; the spec's failure semantics already cover it.
- **Rejected**: pre-emptively disabling signing (violates a constraint).

## Rehydration cues

- `go run . spec file read 000057_git-commit.md` — the spec.
- `go run . repo list` — repo roots (spektacular, docs).
- `go run . knowledge always-applied --tier repo --filter spektacular --filter docs` — conventions.
- `go run . knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/workflow-steps.md"}'` and `gotchas/fsm-cancel-only-works-before-transition-commits.md`, `architecture/testing-architecture.md`.
- Re-read `internal/workflow/workflow.go` (Next/Goto/enter_state), `cmd/resume.go`, `cmd/plan.go` runPlanGoto, `internal/repo/git.go`, `internal/stepkit/stepkit.go:66-115`.
- Docs: `/home/nicj/code/github.com/jumppad-labs/spektacular-website/src/pages/configuration.mdx`.
