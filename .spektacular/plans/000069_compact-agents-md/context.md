---
created_date: "2026-10-21"
document_status: final
closed_date: "2026-10-21"
---

# Context: 000069_compact-agents-md

## Current State Analysis

- Eight standing rules are installed into root `AGENTS.md` as full sections from `templates/agents/*.md` (21,722 bytes of templates). Each is rendered against `{{command}}` and placed by heading through `internal/agent/managed_section.go:41-87`. Repo-sources is placed at the top (`internal/agent/repo_sources.go:23`), and the rest are appended in a fixed order.
- Each of `internal/agent/claude.go:18-49`, `codex.go:13-38`, `bob.go:18-46`, `copilot.go:15-40` and `omp.go:20-38` calls the same eight installers by hand. `historical_artifacts.go:26-111` duplicates the generic placement logic.
- omp additionally writes `.omp/rules/spek-<rule>.md`, each a copy of its AGENTS.md section behind `alwaysApply: true` frontmatter (`omp.go:44-76`), so omp can deduplicate it against AGENTS.md.
- Claude's CLAUDE.md imports AGENTS.md with `@AGENTS.md` (`claude.go:57-97`). This is unchanged by the plan.
- `init` (`cmd/init.go:86`) and `migrate` (`cmd/migrate.go:76`, which runs when `skills_version` differs from the binary, `internal/migrate/engine.go:159-195`) both call `Agent.Install`. A `dev` binary always classifies skills as "match" (`engine.go:403-412`). The cmd tests pin `version = "0.20.0"` (`cmd/main_test.go:13`).
- Per-rule tests assert hand-maintained phrase needles on the rendered AGENTS.md section (for example `internal/agent/store_access_test.go:62-190`). No harbor suite source asserts AGENTS.md text, and every suite except the epic ones has the agent run `init` inside its session, so AGENTS.md is not loaded at session start.
- No docs page describes the standing rules. `configuration.mdx:538-590` documents `migrate`. `plugins.mdx:69-90` and `getting-started.mdx:226-238` describe omp's mirrored rules.

## Per-Task Technical Notes

### Requirement to repo and files

| Spec requirement | Repo | Files |
|---|---|---|
| Each rule stated briefly; binding part inline | spektacular | `templates/agents/short/*.md` (new), `internal/agent/standing_rules.go` (new) |
| Full guidance on demand, not loaded eagerly | spektacular | `templates/agents/*.md` (retitled), `internal/agent/standing_rules.go`, installed to `<project>/.spektacular/guidance/<slug>.md` |
| No rule lost or weakened | spektacular | `internal/agent/*_test.go` phrase needles, `internal/agent/standing_rules_test.go` (new) |
| New projects / existing projects / kept current / other content left alone | spektacular | `internal/agent/managed_section.go` (reused), `cmd/init.go`, `cmd/migrate.go` (unchanged), `cmd/standing_rules_test.go` (new) |
| Works for every agent (incl. omp rules) | spektacular | `internal/agent/{claude,codex,bob,copilot,omp}.go` |
| Guidance files readable directly | spektacular | `templates/agents/store-access.md`, `templates/agents/short/store-access.md` |
| Harbor mirrored / agents still follow rules | spektacular | `tests/harbor/standing-rules/**` (new), `Makefile` |
| Documented | docs | `src/pages/configuration.mdx`, `src/pages/plugins.mdx`, `src/content/tutorials/getting-started.mdx` |

### Task: Add the standing-rules harbor suite

**File changes**:
- `tests/harbor/standing-rules/task.toml` (new): model on `tests/harbor/epic-workflow/task.toml:1-30`. Set an agent timeout of about 900s (three short ad-hoc requests, no workflow) and a verifier timeout of 120s, and keep the `/app/.spektacular` artifact.
- `tests/harbor/standing-rules/environment/Dockerfile` (new): model on `tests/harbor/epic-workflow/environment/Dockerfile:1-40`. Copy `inventory/` to `/srv/inventory` (outside `/app`) and `git init` it, then run `seed/seed.sh` with the agent name substituted. Set `ENV SPEKTACULAR_SESSION_LOG=on` after seeding so the log holds only the agent's calls.
- `tests/harbor/standing-rules/environment/inventory/` (new): a tiny code repo (for example `inventory/stock.py` with `REORDER_THRESHOLD = 37` and a `needs_reorder()` function). The number must appear nowhere under `/app`.
- `tests/harbor/standing-rules/environment/seed/seed.sh` (new): `cd /app && spektacular init {{agent}}`. This is the key difference from every other suite: init runs at build, so AGENTS.md and CLAUDE.md exist at session start. Then `spektacular repo add --data '{"name":"inventory","location":"/srv/inventory","description":"stock-keeping service","role":"service","tags":["python"]}'` (check the shape with `spektacular repo add --schema`). Then write one final spec through `spec file write <name> --document-status final --from .spektacular/tmp/<name>.md`, as `tests/harbor/epic-workflow/environment/seed/seed.sh:28-33` does, and record its sha256 to `/tmp/spec.sha256` outside `/app`. Use counter or fixed-name IDs so the verifier knows the spec name. Leave the `.spektacular/tmp/` file removed.
- `tests/harbor/standing-rules/environment/seed/spec.md` (new): a short final spec (for example "low-stock alerts") with two requirements.
- `tests/harbor/standing-rules/instruction.md` (new): a non-interactive brief, like `tests/harbor/spec-workflow/instruction.md:25-40` ("no human at the other end"), but without asking the agent to run init. It makes three requests and asks the agent to write its answers to `/app/answers.md` under headings `## Code`, `## Spec`, `## Feature`:
  1. "At what stock level does the inventory service flag an item for reorder?"
  2. "Summarise the requirements of the spec named <name>."
  3. A multi-requirement feature idea (for example email and Slack alerts with per-item thresholds and a daily digest). "Tell me how you would proceed." The instruction must not mention `repo list`, the CLI, or specs-vs-implementation, so the behaviour comes from AGENTS.md alone.
- `tests/harbor/standing-rules/tests/test.sh` (new): run `python3 /tests/timeline.py ... || true`, then `pytest /tests/test_standing_rules.py -v`, then write the reward, as `tests/harbor/epic-workflow/tests/test.sh` does.
- `tests/harbor/standing-rules/tests/test_standing_rules.py` (new): load every event from `/app/.spektacular/debug/*.jsonl` using `tests/harbor/shared/sessionlog.py` (`Event.command`). Write the rules as pure functions over a list of commands plus the answers text, so the self-check can call them. The rules:
  - (a) some event's command starts `repo list`;
  - (b) the `## Code` answer contains `37`;
  - (c) some event's command is `spec file read <name>`, and the spec's sha256 is unchanged (recompute it with `spec file read` output body or from the store file; reading the store file from the verifier is fine);
  - (d) no event's command starts `spec new`;
  - (e) the `## Feature` answer contains an offer phrase (case-insensitive match on "spec" together with one of "capture", "write up", "create a spec", "want me to").
- `tests/harbor/standing-rules/tests/test_verifier_selfcheck.py` (new): model on `tests/harbor/repo-workflow/tests/test_verifier_selfcheck.py:1-30`. For each rule (a) to (e), build one compliant and one violating synthetic command list or answer, and assert both directions. It runs locally only and is never copied into the image.
- `Makefile` (around `:175-207`): add `harbor-test-standing-rules` (defaulting to claude), plus `harbor-test-standing-rules-claude`, `-codex` and `-bob`, and a private `_harbor-test-standing-rules` copied from `_harbor-test-epic`. It copies `tests/harbor/shared/*.py` into tests, builds the linux binary into `environment/`, and seds `{{agent}}` into `instruction.md` and `environment/seed/seed.sh`. Add the new targets to `.PHONY` (`Makefile:26`).
- Bob harbor agent (epic decision, shared with 000068_task-goal-validation): `git mv tests/harbor/implement-epic-workflow/agents/{bob_agent.py,bob.json,bob-artifacts.toml} tests/harbor/agents/`, matching `Makefile:8,18,24` (`HARBOR_AUTH_bob` sets `PYTHONPATH=$(CURDIR)/tests/harbor/agents`; `AGENT_ARTIFACTS` reads `tests/harbor/agents/$(AGENT)-artifacts.toml`). Remove the stale `__pycache__`. Grep the Makefile and suites for any path that pointed at the old location. Today the files sit only in the suite-local folder, so Bob harbor runs cannot import `bob_agent`.

**Complexity**: Medium
**Token estimate**: ~45k tokens
**Agent strategy**: Low, sequential. Write the fixture and seed, then the verifier and self-check, then the Makefile targets. Verify with `python3 -m pytest tests/harbor/standing-rules/tests/test_verifier_selfcheck.py` and a `docker build` of the rendered environment if Docker is available. The harbor runs themselves are manual test-plan items.

### Task: Share one standing-rule installer across every agent

**File changes**:
- `internal/agent/standing_rules.go` (new): `type standingRule struct{ slug, heading string; placement sectionPlacement; noun string }`, plus `var standingRules = []standingRule{...}` in today's order: repo-sources (`placeAtTop`, "Where the Code Lives section"), store-access, memory-context, knowledge-trigger, spec-trigger, design-trigger, draft-presentation, historical-artifacts (each `placeAtEnd`, with nouns taken from the existing wrappers). Add `installStandingRules(projectPath, cfg, out)`, which loops `installManagedSectionAt(projectPath, cfg, out, "agents/"+r.slug+".md", r.heading, r.noun, r.placement)`.
- `internal/agent/{spec_trigger,store_access,memory_context,knowledge_trigger,design_trigger,draft_presentation,repo_sources}.go:1-27`: keep the heading constants, since tests and the omp writer reference them. Turn each `installXSection` into a one-line call to a new `installStandingRule(projectPath, cfg, out, rule)` that looks up the rule by slug, so the existing fixture tests keep compiling. Alternatively, delete the wrappers and update the tests to call `installStandingRule` directly. Either is acceptable, but keep the template path constants the fixture tests use.
- `internal/agent/historical_artifacts.go:26-118`: delete the bespoke `installHistoricalArtifactsSection` body and `locate/append/replaceHistoricalArtifactsSection`, and route through `installStandingRule`. Before deleting, check that `historical_artifacts_test.go:20-159` expectations (append after tessl block, preserve surrounding content) match the generic `appendManagedSection`/`replaceManagedSection`, which are byte-for-byte the same algorithm.
- `internal/agent/claude.go:18-49`: replace the eight calls with `installStandingRules`, then `ensureClaudeImportsAGENTS` unchanged.
- `internal/agent/codex.go:13-38`, `internal/agent/bob.go:18-46`, `internal/agent/copilot.go:15-40`: the same replacement.
- `internal/agent/omp.go:28-37`: replace the inline installer slice with `installStandingRules`. In `omp.go:55-64`, iterate `standingRules` (`"spek-"+r.slug+".md"`, `r.heading`) instead of the local struct list.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential. Before editing, snapshot a fresh install for each agent into temp dirs and assert byte-equality afterwards. The existing `TestInstall*_CrossAgentIdempotency`, `repo_sources_test.go:201` (`TestEveryAgentInstallsRepoSourcesSectionAtTheTop`) and `omp_test.go:36-124` should pass unchanged. Run `go test ./internal/agent/ ./cmd/`.

### Task: Install compact sections and guidance files

**File changes**:
- `templates/agents/short/<slug>.md` (new, eight files): `## <heading>`, a blank line, the body, then the link line `Managed by `{{command}} init`; full guidance: [{{guidance_path}}]({{guidance_path}}).`. **Content example** (wording illustrative; rendered with `command=spektacular`; the draft measured 4,228 bytes in total, repo-sources 12 body lines and the rest at most 6):

  ```markdown
  ## Where the Code Lives — Read This First

  **STOP. The directory you are running in is not necessarily the code.** A project holding only
  `.spektacular/`, `knowledge/` and `repos/` is normal; its code lives wherever the registry says.

  - **Before any code-touching work** (searching, reading, editing, running tests, verifying, or
    answering how something works), run `spektacular repo list` and work in each repo's `root`.
  - **Never assume the starting directory holds a repo's code, and never guess a path.** The
    registry is the only authority on where code lives.
  - **Pass the relevant repo's `root` explicitly to every sub-agent you launch.**
  - **If a repo's `root` is not on disk, stop and tell the user** the path the registry gave.

  This binds everywhere: every workflow, ad-hoc question, skill and sub-agent.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/repo-sources.md](.spektacular/guidance/repo-sources.md).

  ## Spektacular's Files Are Reached Through Spektacular

  Never read, list, write, edit or delete a store file (specs, plans, context, research, test plans,
  changelog, knowledge, designs, epics) with your own tools; use the `spektacular` CLI and `--from
  <path>`. Yours to write: `.spektacular/tmp/`, `.spektacular/work/<name>/`,
  `.spektacular/working-context.md`; yours to read: `.spektacular/guidance/`. Binds every sub-agent:
  say so when delegating. Before touching anything under `.spektacular/`, read the full guidance.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/store-access.md](.spektacular/guidance/store-access.md).

  ## Memory & Context

  In this repository, never write to your per-user memory store; route anything worth remembering
  through the `spek-knowledge` skill instead. Outside this repository, use your memory as normal.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/memory-context.md](.spektacular/guidance/memory-context.md).

  ## Knowledge-Worthy Discovery Recognition

  When you surface something durable and non-obvious (an unwritten convention, a gotcha, the reason
  behind a choice), offer to save it via `spek-knowledge`, naming the tags; never write unprompted.
  A decline is final for that discovery. Before offering, read the full guidance.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/knowledge-trigger.md](.spektacular/guidance/knowledge-trigger.md).

  ## Spec-Worthy Discussion Recognition

  When a discussion or a request (an issue, a ticket, a pasted doc) reaches spec-worthy detail,
  offer to capture it as a spec before implementing; never start the spec workflow unilaterally.
  Calibrate with `spec_trigger_threshold` in `.spektacular/config.yaml`, read at decision time.
  Before offering, read the full guidance.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/spec-trigger.md](.spektacular/guidance/spec-trigger.md).

  ## Design-Worthy Detail Recognition

  When a discussion settles a worked design (an API shape, a flow, a data format) too large to
  write into a spec, or the user brings or changes a design document, offer to capture it with the
  `spek-design` skill; never write a design document unprompted. Before offering, read the full
  guidance.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/design-trigger.md](.spektacular/guidance/design-trigger.md).

  ## Presenting Drafts and Confirmations

  Always show a draft in full as plain chat text first; never put the draft itself inside a yes/no
  or multiple-choice dialog. A short confirmation question may follow it.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/draft-presentation.md](.spektacular/guidance/draft-presentation.md).

  ## Historical Artifacts: Specs and Plans as Archaeology

  Specs and plans under `.spektacular/specs/` and `.spektacular/plans/` record past intent, not
  current behaviour: never read them to explain how the code works today, only for "why was X
  built this way?" questions or within the workflow that owns them. Knowledge entries are binding.
  Before reading a spec or plan, read the full guidance.
  Managed by `spektacular init`; full guidance: [.spektacular/guidance/historical-artifacts.md](.spektacular/guidance/historical-artifacts.md).
  ```

  Hard-coded `spektacular`/`.spektacular/config.yaml` in the example become `{{command}}`/literal paths in the templates. Keep `{{command}}` wherever a command is named, so `TestOMPInstall_PreservesProjectFilesAndRefreshesOwnedFiles` (`omp_test.go:81-89`, which asserts rules contain the custom command) still holds.
- `templates/agents/<slug>.md` (eight files): change the first line from `## <heading>` to `# <heading>`. Rewrite the banner (`:3-5` in each) to say: managed by `{{command}} init` and refreshed by `{{command}} migrate`; edit `templates/agents/<slug>.md` in the Spektacular source, not this file; agents read this file and never edit it. Leave all other text unchanged, except in `store-access.md`. After the three-exceptions list (`:44-50`), add a paragraph: the guidance files under `.spektacular/guidance/` are not a store; read them with your own file tools when a rule points you there, and never edit, move or delete them, because `init` and `migrate` regenerate them.
- `internal/agent/standing_rules.go`: add `guidanceDir = filepath.Join(config.ProjectConfigDirName, "guidance")` and `guidancePath(slug) = filepath.ToSlash(filepath.Join(guidanceDir, slug+".md"))`. In `installStandingRule`, first render `agents/<slug>.md` with `{"command": cfg.Command}`, then `os.MkdirAll(<project>/.spektacular/guidance, 0755)` and `writeFileAtomic`, logging `  Guidance: <path>`. After that, call `installManagedSectionAt` with the short template.
- `internal/agent/managed_section.go:41-50`: extend the render data to accept extra variables (for example an `installManagedSectionWith(..., data map[string]string)` used by the standing-rule installer, with `installManagedSectionAt` delegating with `{"command": ...}`), so `guidance_path` reaches the short template. Placement and replacement logic is unchanged.
- `internal/agent/store_access_test.go:62-190`: add `renderStoreAccessGuidance` (reads `<tmp>/.spektacular/guidance/store-access.md`) beside `renderStoreAccessSection` (AGENTS.md). Assign needles as follows:
  - **compact:** "never build a store path by hand" or the equivalent compact wording, the `.spektacular/tmp/`, `work/` and `working-context.md` exceptions, "sub-agent", and `.spektacular/guidance/`;
  - **guidance:** every existing literal (`:79-85`, `:97-110`, `:126-151`, `:163-180`), unchanged.

  Add a needle that both surfaces mention `.spektacular/guidance/`.
- `internal/agent/design_trigger_test.go:161-360`, `spec_trigger_test.go`, `knowledge_trigger_test.go`, `memory_context_test.go`, `draft_presentation_test.go`, `historical_artifacts_test.go`, `repo_sources_test.go:185-200`: the same split. Existing literal needles move to a guidance render, unchanged. Add hand-maintained compact needles per rule:
  - repo-sources: "repo list", "root", "sub-agent", "STOP";
  - spec-trigger: "offer", "never start";
  - design-trigger: "offer", "spek-design", "never write";
  - knowledge-trigger: "offer", "never write";
  - memory-context: "spek-knowledge";
  - draft-presentation: "plain chat text";
  - historical-artifacts: "past intent", "Knowledge entries are binding".

  Fixture FS helpers (`spec_trigger_test.go:20-26` and the matching helpers in each per-rule test) must now include both `agents/short/<slug>.md` and `agents/<slug>.md`. The exact-output tests compare AGENTS.md against the rendered short fixture, plus a link line rendered with `guidance_path`.
- `internal/agent/repo_sources_test.go:193`: the assertion `"> Managed by `spektacular init`"` now targets the guidance file. AGENTS.md instead asserts "Managed by `spektacular init`; full guidance:".
- `internal/agent/managed_section_test.go:28-140`: unchanged, apart from compiling against any signature change.
- `internal/agent/claude_test.go:18-62`, `codex_test.go`, `bob_test.go`, `copilot_test.go`: wherever they assert AGENTS.md body phrases from the full text, move the assertion to the guidance file. Add `require.FileExists` for all eight guidance files.
- `internal/agent/instruction_surface_test.go:73`: no change needed (`agents` is walked recursively), but confirm the new `short/` templates pass `TestEmbeddedTemplatesAvoidStdinInstructionSurface`.

**Complexity**: High
**Token estimate**: ~70k tokens
**Agent strategy**: Parallel analysis, sequential integration. One sub-agent drafts the eight short templates against the line and byte budget, while another inventories every needle in the per-rule tests and assigns each to a surface. One agent then integrates the installer change and the test retargeting and runs `go test ./internal/agent/ ./cmd/ ./templates/`. No task step may run `spektacular init` or `migrate` in this repository.

### Task: Prove conversion, refresh and the size budget for every agent

**File changes**:
- `internal/agent/standing_rules_test.go` (new):
  - `TestStandingRules_BudgetForEveryAgent`: for each of `claude`, `codex`, `bob`, `copilot` and `omp`, `Lookup(name).Install(tmp, config.NewDefault(), io.Discard)` into a fresh temp dir. Then `locateManagedSection` each `standingRules` heading. Assert:
    - the sum of section byte lengths is under 5,120;
    - the non-blank body lines are at most 6 (15 for repo-sources);
    - every non-link line is at most 100 runes;
    - the body contains exactly one markdown link whose target equals `guidancePath(slug)`, and that file exists;
    - the body contains no `@` immediately followed by a guidance path;
    - the guidance file's body (after its banner) does not appear in AGENTS.md, CLAUDE.md or `.omp/rules/*`.
  - `TestStandingRules_GuidanceHoldsFullTemplate`: each guidance file equals the mustache render of `agents/<slug>.md`. This is the "nothing is lost" anchor, together with the retargeted needles.
  - `TestStandingRules_ConvertsLegacyFullSections`: write a fixture AGENTS.md built from the pre-change full sections (a `testdata/agents-md-legacy.md` captured from `git show HEAD~:templates/agents/*.md`, rendered with `spektacular`, with user blocks `# Team rules\n\nKeep A.\n` before, `## Our section\n\nKeep B.\n` between store-access and memory-context, and `## Trailing\n\nKeep C.\n` at the end). Run `Install` for each agent and assert:
    - each user block is present byte-for-byte, in the same relative order;
    - each heading occurs once;
    - every section is compact.
  - `TestStandingRules_Idempotent`: install twice per agent and snapshot the whole tree (reuse `ompSnapshot` from `omp_test.go:15-34`); the snapshots are equal.
  - `TestStandingRules_RefreshesStaleGuidance`: overwrite two guidance files with `stale` and delete a third, reinstall, and assert all three match the shipped render.
- `cmd/standing_rules_test.go` (new), following `cmd/omp_test.go:11-46` and `cmd/migrate_test.go:232-286`:
  - `TestInit_InstallsCompactRulesForEveryAgent`: `runRootCmd(t, "init", agent)` per agent in a fresh `t.TempDir()`, with `resetRootCmd`. Assert the eight `.spektacular/guidance/*.md` files exist, and CLAUDE.md contains `@AGENTS.md` for claude.
  - `TestMigrate_ConvertsPreChangeProject`: write `config.yaml` with `skills_version: 0.0.1` and `agent: claude`, a legacy AGENTS.md (the same testdata fixture) and no guidance folder. Run `runMigrateReport`. `cmd/main_test.go:13` pins the binary version to `"0.20.0"`, so an older `skills_version` triggers the reinstall. Assert `Skills.Reinstall`, the compact sections, preserved user blocks, and all guidance files present. Run migrate again after resetting `skills_version` and assert byte-identical results.
  - `TestMigrate_RefreshesOutdatedGuidance`: an initialised project with mutated guidance and an older `skills_version`; after migrate, every guidance file matches the shipped render.
- `internal/agent/testdata/agents-md-legacy.md` (new): the captured pre-change AGENTS.md fixture described above, frozen as a file so the test does not depend on git history.

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Medium. One agent writes the agent-package tests and the legacy fixture, another the cmd-level init and migrate tests. Then run `go test -shuffle=on ./...` in full (the tests-must-not-depend-on-order and tests-must-pass conventions).

### Task: Document the compact layout on the docs site

**File changes**:
- `docs:src/pages/configuration.mdx:538-590`: after the `Upgrading a project: migrate` section (`surface={false}`), add a new `<Section heading="Standing rules: AGENTS.md and .spektacular/guidance/" surface={true}>`. Check that `CtaBanner` at `:592` still reads correctly after a shaded band. Append one bullet to the migrate section's "One run:" list (`:563-576`).

  **Content outline** for the new section:
  - *Sub (slot)*: "Spektacular installs eight standing rules for your agent. Each sits in your root `AGENTS.md` as a short statement, and its full guidance lives in its own file that the agent opens only when the rule applies."
  - *What a short statement holds*: "Each rule keeps its own heading in `AGENTS.md`. Below it is the obligation the agent must always honour, the situation that calls for more detail, and a link such as `[.spektacular/guidance/spec-trigger.md](.spektacular/guidance/spec-trigger.md)`. Spektacular's part of `AGENTS.md` stays under 5 KB, so every session starts lighter."
  - *Where the guidance lives*: a fenced tree:

    ```text
    .spektacular/guidance/
      repo-sources.md
      store-access.md
      memory-context.md
      knowledge-trigger.md
      spec-trigger.md
      design-trigger.md
      draft-presentation.md
      historical-artifacts.md
    ```

    Then: "Commit the folder with the project so the links resolve for everyone. Agents read these files with their own tools when a rule sends them there. They are never imported into a session up front."
  - *Do not edit them*: "`init` writes the guidance files and `migrate` rewrites them for each new Spektacular version, so local edits are lost. Put your own rules in your own sections of `AGENTS.md`, which Spektacular never changes."
  - *Agent notes*: "Claude reads `AGENTS.md` through the `@AGENTS.md` import in `CLAUDE.md`. Codex, Copilot and Bob read `AGENTS.md` directly. omp's always-applied `.omp/rules/spek-*.md` files carry the same short statements."
- `docs:src/pages/configuration.mdx:563-576` **Content example** (new bullet): "- converts the standing rules in `AGENTS.md` to their short form and installs or refreshes the guidance files in `.spektacular/guidance/`, leaving any content you added to `AGENTS.md` where it was".
- `docs:src/pages/plugins.mdx:69-76` **Content example**: "...and eight always-applied `spek-*.md` rules in `.omp/rules/`. The rules mirror the short standing-rule sections of root `AGENTS.md`, each linking to its full guidance in `.spektacular/guidance/`, so they remain visible beside another tool's instruction file or the project's own `.omp/AGENTS.md`."
- `docs:src/content/tutorials/getting-started.mdx:226-229` **Content example**: "...and eight always-applied standing-rule files in `.omp/rules/`, which carry the same short rules as root `AGENTS.md` and link to the full guidance in `.spektacular/guidance/`."

Verify against the docs conventions: `grep -nE "<div|<section|class=" src/pages/*.mdx` returns nothing, `npm run build` and `npx astro check` pass, and the added text contains no em dash.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential, working in `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Testing Strategy

- **Add the standing-rules harbor suite:** Python self-check tests for every verifier rule, which run locally with no container. A Docker build of the rendered environment checks the seed. The harbor runs for Claude, Codex and Bob, before and after the compact change, are manual test-plan items.
- **Share one standing-rule installer across every agent:** a byte-equality comparison of each agent's install before and after the refactor. The existing per-rule, cross-agent, repo-sources-at-top and omp tests must pass unchanged.
- **Install compact sections and guidance files:**
  - Per-rule tests render both surfaces. Every existing needle is kept, now on the guidance file, and new hand-maintained compact needles assert the binding obligation inline.
  - The store-access exemption is asserted on both surfaces.
  - The embedded-template guards cover the new `short/` templates.
- **Prove conversion, refresh and the size budget for every agent:** agent-package and cmd-level tests over real installs in temp projects, covering:
  - the byte budget, line limits and link resolution;
  - no eager include;
  - guidance equal to the full render;
  - legacy conversion with user content preserved;
  - idempotency;
  - stale guidance refreshed;
  - `init` for all five agents and `migrate` from an older skills version.

  Run the full suite with `go test -shuffle=on ./...`.
- **Document the compact layout on the docs site:** the docs repo guards (`grep -nE "<div|<section|class=" src/pages/*.mdx` empty, `npm run build`, `npx astro check`) plus an em-dash check of the added prose.
- **Manual checks** (in the implementation test plan):
  - harbor pass rates before and after, for all suites and agents;
  - Copilot and omp sessions in a throwaway project;
  - a Claude `/context` reading for the token reduction;
  - a live check that guidance is not loaded eagerly;
  - a rule-by-rule human read of compact against guidance;
  - a docs review.

## Project References

- Spec: `000069_compact-agents-md` (epic `000001_agent-workflow-improvements`). Sibling spec `000068_task-goal-validation` also edits the agent installers.
- Design documents: none referenced.
- Knowledge (repo `spektacular`):
  - `architecture/testing-architecture.md`
  - `conventions/plans-never-change-the-active-install.md`
  - `conventions/store-files-must-be-written-through-the-cli.md`
  - `conventions/tests-must-not-depend-on-order.md`
  - `conventions/tests-must-pass-for-done.md`
  - `conventions/error-messages-must-suggest-remediation.md`
- Knowledge (repo `docs`): `conventions/no-em-dashes.md`, `conventions/plan-content-pages.md`, `conventions/mdx-authoring.md`, `conventions/file-scoped-section-headings.md`, `conventions/alternate-section-background.md`.
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Only "Install compact sections and guidance files" is High. Split the short-template drafting from the needle inventory, then integrate in one agent.

## Migration Notes

- There is no settings-format bump and no new migration step. Existing projects convert when `migrate` reinstalls the agent after a version upgrade, or on any re-run of `init`, because the installer replaces each full section in place by its unchanged heading and writes `.spektacular/guidance/`.
- The guidance folder should be committed. Teams on shared multi-agent projects should re-run `init` for each agent, as today, to refresh every agent's files.
- This repository's own AGENTS.md is not converted by the plan. It picks up the change only after a person runs `make install-local` between workflows (plans-never-change-the-active-install).

## Performance Considerations

- The session-start instruction payload drops from about 22 KB to under 5 KB (a reduction of more than 76%). Install writes eight extra small files per run. This is negligible beside the skills install.
