---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Research: 000072_knowledge-access-index-and-prune

## Alternatives considered and rejected

### Option: Creation date in each entry's frontmatter (`created_date`), stamped by `knowledge write` like `metadata.Merge` does for specs

Creation date in each entry's frontmatter (`created_date`), stamped by `knowledge write` like `metadata.Merge` does for specs.

**Rejected**: every pre-existing entry would have to be rewritten to carry one (a whole-store diff on upgrade), an agent writing a full body with `--file` would clobber it unless write merged frontmatter, and `internal/store/frontmatter.go:10-30` records that knowledge frontmatter is deliberately optional so features ship "with no migration, no backfill". The usage index already holds per-entry records, so the date lives there (spec Non-Goal: "the usage index is the only addition to a store").

### Option: Append-only JSON Lines event log instead of a JSON index

Append-only JSON Lines event log instead of a JSON index.

**Rejected**: the spec's Constraints require "a JSON index"; a JSONL log also grows without bound and needs a reader to fold it into counts.

### Option: Per-process in-memory locking (`sync.Mutex`) or no locking

Per-process in-memory locking (`sync.Mutex`) or no locking.

**Rejected**: each CLI call is its own process and epic runs plan specs in parallel (`internal/workflow/lane.go:10-16`), so read-modify-write without a cross-process lock loses updates.

### Option: `syscall.Flock` advisory lock

`syscall.Flock` advisory lock.

**Rejected**: Spektacular ships for windows too (`dagger/main.go:14`); the portable `O_CREATE|O_EXCL` lock-file pattern already exists in `internal/autocommit/lock.go:41-71`.

### Option: Tracking inside `Set.Read`/`Set.Search` themselves

Tracking inside `Set.Read`/`Set.Search` themselves.

**Rejected**: `knowledge delete` calls `set.Read` to test existence (`cmd/knowledge.go:554`), `Set.Tags` reads every entry, and prune must read every entry untracked. Recording is an explicit step taken by the command handlers for `search`, `read`, `always-applied` and `conventions` only.

### Option: Prune as a full FSM workflow kind (`prune new`/`goto`) with a state.json lane

Prune as a full FSM workflow kind (`prune new`/`goto`) with a state.json lane.

**Rejected**: it would compete with the single shared `state.json` and with resume detection (`internal/workflow/state.go:24-27`), session-log filing (`cmd/root.go:239-295`) and the harbor step oracles, for a loop that has no fixed step order. A small command-owned run file plus a `verdict` subcommand gives the same "one instruction per entry" shape.

### Option: Agent reads each entry with `knowledge read` during the correctness pass, with a `--untracked` flag

Agent reads each entry with `knowledge read` during the correctness pass, with a `--untracked` flag.

**Rejected**: any agent could then hide reads. Prune embeds each entry's body in its instruction, read by the CLI without tracking.

### Option: A `--preview` flag

A `--preview` flag.

**Rejected**: Rejected in favour of the established `--dry-run` name (`cmd/spec.go:411`, `cmd/migrate.go:35`); the docs call it the preview mode.

### Option: Attributing a read to "the" workflow by reading only `.spektacular/state.json`

Attributing a read to "the" workflow by reading only `.spektacular/state.json`.

**Rejected**: an epic's orchestrated children run in lanes beside it (`cmd/root.go:224-295`), so state.json can name an unrelated workflow. Attribution names a workflow only when exactly one is running.

### Option: Putting the prune age in `repo.yaml`

Putting the prune age in `repo.yaml`.

**Rejected**: repo.yaml holds only a repo's own facts shared by every project that registers it (`internal/config/config.go:313-320`); a prune policy is per project.

### Option: Bumping the settings format for the new key

Bumping the settings format for the new key.

**Rejected**: an optional key with a default needs no bump (`implement.worktrees` 48cb1d0, `implement.goal_check` a6b7fa5); a bump makes every project refuse to load until `migrate` (`internal/config/schema.go:88-99`).

## Chosen approach — evidence

- Store contract widening is the sanctioned fix for a missing capability: knowledge `architecture/working-with-files-from-steps.md` ("A gap in the interface is a reason to widen the interface"; `Stat` was added that way). Writer is at `internal/store/store.go:116-123`; the only implementations are `FileStore` (`store.go:182-198`), `ignoreStore` (`internal/store/ignore.go:85-87`) and two test doubles (`internal/design/design_test.go:645`, `internal/knowledge/set_test.go:2329`).
- Lock pattern to generalise: `internal/autocommit/lock.go:41-71` (O_EXCL, backoff, stale break, `LockError`); tests `internal/autocommit/lock_test.go:32-77` (8 goroutines, atomic max-inside counter).
- Atomic write pattern: temp file + rename at `internal/migrate/engine.go:366-380`, `internal/agent/managed_section.go:240`.
- Knowledge layer exclusions live in one place per surface: `Set.Search` excludes always-applied and category descriptions (`internal/knowledge/set.go` Search loop), `IsCategoryDescription` (`internal/knowledge/category.go:125`); the index exclusion is added beside them in Search, List, Tags and readCategories.
- `FileStore.search` walks every file (`internal/store/search.go:61-110`), so the index and lock files would be scanned unless excluded.
- Command handlers that should record: `runKnowledgeSearch`, `runKnowledgeRead`, `runKnowledgeAlwaysApplied`, `runKnowledgeConventions` (`cmd/knowledge.go:396-625`); write/delete hooks at `runKnowledgeWrite`/`runKnowledgeDelete`.
- Running-workflow detection: shared `stateFilePath` (`cmd/spec.go:94`), `State.InProgress` (`internal/workflow/state.go:24-27`), lanes via `workflow.LaneNames`/`ReadLane` (`internal/workflow/lane.go`).
- Instruction rendering outside a workflow: `stepkit.RenderTemplate(path, data)` (`internal/stepkit/stepkit.go:237-243`) renders an embedded mustache template with partials.
- Config precedent for an optional defaulted key: `ImplementConfig` comments `internal/config/config.go:157-181`, validation with `output.NewError("config_invalid", …).WithResource(key).WithNextAction(…)` at `config.go:698-715`; `KnowledgeConfig` at `config.go:244-246`, validated at `config.go:874-898`.
- Git invocation goes only through `gitexec.Run` (`internal/gitexec/gitexec.go:31`); nothing reads git dates today.
- Maintenance verdict rules to reuse for the correctness pass: `templates/skills/workflows/spek-knowledge/SKILL.md` "Intent: maintenance" (current/stale/incorrect/unverifiable, unmet target is current, evidence required).

## Files examined

- spektacular:internal/knowledge/set.go — Set fan-out; Read/Write/Delete resolve one store; Search exclusions and cutoff; listFiles recursion; no tracking.
- spektacular:internal/knowledge/category.go:105-190 — `CategoryDescriptionFile`, `IsCategoryDescription`, `UnreachableTags` (write-result report pattern).
- spektacular:cmd/knowledge.go — every knowledge subcommand, schemas, `newKnowledgeSet`, `aggregateKnowledgeSources`, write via `--file`, delete's existence read at :554.
- spektacular:internal/store/store.go — Reader/Writer/Store interfaces, FileStore, `Stat.CreatedAt` always zero.
- spektacular:internal/store/ignore.go — ignoreStore delegates writes; filters List/Search.
- spektacular:internal/store/search.go:61-110 — whole-tree walk.
- spektacular:internal/store/frontmatter.go:10-60 — knowledge frontmatter is optional, tags only, deliberately not internal/metadata.
- spektacular:internal/autocommit/lock.go — portable lock file implementation to generalise.
- spektacular:internal/workflow/lane.go, state.go — lanes per orchestrated run; InProgress.
- spektacular:cmd/root.go:110-420 — session log filing by lane; commands not naming a spec go to shared state.
- spektacular:internal/sessionlog/record.go — session id from (kind,name).
- spektacular:internal/config/config.go:236-470, 595-640 — Config struct, NewDefault, KnowledgeConfig, legacy-scope guard.
- spektacular:internal/config/schema.go:15-99 — format versions; checkSchema refuses mismatch.
- spektacular:internal/gitexec/gitexec.go — Run/RunCode only.
- spektacular:internal/stepkit/stepkit.go:237-250 — RenderTemplate, FSPartials.
- spektacular:templates/agents/knowledge-trigger.md, templates/agents/short/knowledge-trigger.md — offer-then-wait rule installed into AGENTS.md and guidance.
- spektacular:templates/skills/workflows/spek-knowledge/SKILL.md — five intents, propose-then-confirm everywhere, maintenance review.
- spektacular:templates/partials/implement-knowledge-candidates.md, templates/partials/orchestrated-stop.md:9, templates/steps/implement/07-update_changelog.md:63-72, templates/steps/implement/12-finished.md:18,24,60,66, templates/steps/plan/02-discovery.md:77-79, templates/steps/plan/18-walkthrough.md:25, templates/steps/plan/19-finished.md:30, templates/skills/workflows/spek-plan-epic/SKILL.md:68,108, templates/skills/workflows/spek-implement-epic/SKILL.md:82,127 — every instruction that offers knowledge and waits.
- spektacular:internal/steps/implement/steps_test.go:438-471 — pins the Knowledge-candidates collection (bans "knowledge write" in update_changelog) and the finished step's offer partial.
- spektacular:internal/steps/plan/steps_test.go:184-194, 549-574 — discovery must say "confirm" before a knowledge write; walkthrough offer phrases ("produce no offer", "explicit acceptance", "not offered again").
- spektacular:internal/agent/instruction_surface_test.go:156-171, 307-394, 541-809, 957-1016 — spek-knowledge contribute/update propose-then-confirm phrases, "five branches (lookup / contribute / update / audit / maintenance)", maintenance per-entry confirm; audit phrases must stay.
- spektacular:internal/agent/standing_rules_test.go:601-617, 818-829, 888, 919 — knowledge-trigger compact must contain "offer"/"never write" and trigger "Before offering, read the full guidance."; full-only "This is the recognition trigger only"; memory-context full-only "The skill handles tier and store selection"; compact sections ≤6 lines, ≤100 runes per line, all sections ≤5120 bytes. Heading "## Knowledge-Worthy Discovery Recognition" pinned across cmd/standing_rules_test.go and internal/agent tests.
- spektacular:internal/agent/standing_rules.go:58-110 — standing-rule table; short form to AGENTS.md, full form to .spektacular/guidance/<slug>.md.
- spektacular:templates/agents/memory-context.md:10-12 — also says the skill does "propose-then-confirm".
- spektacular:templates/implement_epic_skill_test.go:211-242, templates/plan_epic_skill_test.go:135-192 — pin "Offer to save any durable discovery…" (implement-epic) and an allowlist for every "ask*" word in both epic skills.
- spektacular:tests/harbor/{epic-workflow,implement-epic-workflow,goal-check-workflow}/instruction.md (lines 55/56/41) — tell the headless agent to decline knowledge offers; obsolete after this change. No harbor verifier checks knowledge offers; plan suite bans AskUserQuestion in drafting windows (test_plan_workflow.py:944-965).
- spektacular:README.md:251-268 — CLI list and "Capturing knowledge" (says the agent never persists unprompted).
- spektacular:docs/knowledge-base.md:333-425 — Contributing, tag audit, maintenance review, command reference table.
- spektacular:cmd/docs_test.go:153-329 — README/docs phrase pins (none pin the offer wording, the maintenance review or the command table).
- spektacular:internal/knowledge/ranking_docs_test.go — pins ranking constants in docs/knowledge-base.md and a knowledge entry.
- spektacular:tests/harbor/plan-workflow/tests/test_plan_workflow.py:114-121 — `CONVENTIONS_READ_COMMAND = "knowledge always-applied"` oracle (unchanged by this plan).
- docs:src/pages/knowledge-base.mdx — Hero, Section+Prose bands; capture passage 116-118, tag confirm 339-341, tag audit 343-367, Configuration 373-483; no prune/maintenance/usage content.
- docs:src/pages/configuration.mdx:342-356 — project `knowledge` ConfigKey to extend; implement ConfigKey 305-340 shape for defaulted subkeys; guidance file list 616-651.
- docs:src/pages/epics.mdx:499-566 — "from the command line" Section/Prose/bash pattern.
- docs:package.json, Makefile — `npm run build`, `npx astro check` are the only guards.

## External references

- None needed; OpenTelemetry and other epic topics belong to sibling specs.

## Prior plans / specs consulted

- 000073_log-mode-and-logger-providers (sibling in the epic, independent): replaces debug with log mode and changes config; run identifiers come from workflow state. No shared files with this plan except `internal/config/config.go` (different struct) and possibly the website configuration page (different section).
- Epic 000002_workflow-tracing-and-run-stats: lists 000072 as independent of the other specs.

## Open assumptions

- `O_EXCL` lock files behave atomically on every filesystem a knowledge store sits on (local disks; not verified on network filesystems).
- `git log --diff-filter=A --follow --format=%aI -- <file>` gives the first-add date for an entry when its store is inside a git work tree; outside one, the store's modification time stands in.
- The epic orchestrator skills can carry "children write knowledge and report it" without changing the hand-back contract's first-line keywords.
- Harbor verifiers contain no assertion that requires a knowledge offer (confirmed by search); only three suite instructions mention declining offers.

## Drafting assumptions

### Creation date lives in the usage index (discovery)
- **Decision**: each entry's creation date is held in its store's usage index record, not in the entry's frontmatter.
- **Rationale**: existing entries need no rewrite (spec constraint), an agent writing a whole body cannot clobber it, and knowledge frontmatter is deliberately optional and tags-only (`internal/store/frontmatter.go:10-30`). The spec's Non-Goals name the index as the only addition to a store.
- **Rejected**: a `created_date` frontmatter field stamped on write (rewrites every existing entry, needs a merge on every write).

### Backfill from git history, then modification time (discovery)
- **Decision**: an entry with no index record gets one the first time its store's index is written; its creation date is the date git first added the file when the store is in a git work tree, else the store's modification time for it, else now. Its tracking clock (`tracked_since`) starts at that moment.
- **Rationale**: the spec names version history as an option; the tracking clock, not the creation date, protects pre-existing entries from the first prune, so an imprecise fallback cannot cause a wrong removal.
- **Rejected**: a `migrate` step (repo-tier stores live outside the project and a plan never migrates the active install); stamping "now" as the creation date (a false date for old entries).

### Workflow attribution: caller-named spec, else the single running workflow (discovery, revised at verification)
- **Decision**: `knowledge read --data` takes an optional `"spec"` and `always-applied`/`conventions` take an optional `--spec`. When that spec has an in-progress workflow (lane or shared state.json), the retrieval names it. The plan discovery template (the only workflow template that tells agents to read knowledge) passes `{{plan_name}}`. Without a spec, a retrieval names a workflow only when exactly one is in progress, and otherwise none.
- **Rationale**: the requirement says a read during a running workflow names that workflow and its spec, and epic runs plan specs in parallel, so the CLI cannot tell lanes apart without the caller saying which. Only one template needs the extra argument. The harbor oracle (`"knowledge always-applied"` substring) is unaffected. A wrong attribution is worse than a missing one (knowledge `architecture/working-with-files-from-steps.md`), so ad hoc reads still never guess.
- **Rejected**: exactly-one rule alone (misses every read in a parallel epic run, which contradicts the requirement); always attributing to state.json (names an unrelated workflow during lanes); an environment variable (an orchestrator cannot set it for a child's shell).

### Chosen direction: per-store JSON usage index + command-driven prune loop (architecture)
- **Decision**: a `.usage-index.json` per knowledge store maintained by `internal/knowledge` through a new atomic `store.Writer.Update` (O_EXCL lock file + temp/rename); recording called explicitly by the search/read/always-applied/conventions handlers; `knowledge prune [--dry-run]` first pass in the command, then one inlined-entry instruction per remaining entry, answered through `knowledge prune verdict`, which checks each claim against the store; instructions rewritten to "write, then report".
- **Rationale**: satisfies the JSON-index and concurrency constraints without a new state machine; keeps prune's own reads untracked by construction; widening the store contract is the sanctioned route (knowledge architecture/working-with-files-from-steps.md).
- **Rejected**: FSM prune workflow, tracking inside Set.Read, frontmatter creation dates, flock, JSONL log (see research.md alternatives).

### Recording failure does not fail the read (architecture)
- **Decision**: if the index cannot be updated (lock timeout, write error), `search`/`read`/`always-applied`/`conventions` still return their result, with a `usage_warning` field naming the problem and remedy.
- **Rationale**: usage bookkeeping must not block agents from knowledge they need; with a 30s lock wait, lost updates under normal parallel load do not occur.
- **Rejected**: failing the command (blocks the workflow over bookkeeping); silently ignoring (hides lost updates).

### Prune age setting is optional with a code default (architecture)
- **Decision**: `knowledge.prune.max_age_days` (int, omitempty); absent or 0 means 90 through an accessor; negative refused as `config_invalid`. No settings-format bump; `init` does not write the key.
- **Rationale**: follows `implement.worktrees`/`implement.goal_check` precedent; keeps generated configs and the sibling 000073 config change from colliding.
- **Rejected**: writing a non-zero default into NewDefault (every saved config gains a knowledge block, churns config tests); repo.yaml (repo facts only).

### Dry-run flag name (architecture)
- **Decision**: the preview mode is `knowledge prune --dry-run`, documented as the preview.
- **Rationale**: `--dry-run` is the established preview flag on spec/plan/implement/repo/migrate.
- **Rejected**: a new `--preview` flag name.

### Prune run file location (architecture)
- **Decision**: `.spektacular/knowledge-prune.json`, written by the command, removed when the final report is returned.
- **Rationale**: outside every knowledge store (so a dry run leaves the knowledge base byte-identical) and outside the lane folder that `LaneNames` globs.
- **Rejected**: inside the knowledge store; inside `.spektacular/workflows/`.

### Orchestrated children write knowledge too (architecture)
- **Decision**: in orchestrated runs children write durable knowledge themselves and list the entries written in their hand-back; the epic skills report those entries instead of offering them.
- **Rationale**: the spec requires agents to write without asking, and orchestration does not exempt them; index updates are lock-safe in parallel.
- **Rejected**: keeping children on "list discoveries for the orchestrator to offer" (still a question to the user).

### Conventions selected (architecture)
- **Decision**: applied error-remediation, store-files-through-CLI, plans-never-change-active-install, tests-order-independent, tests-must-pass, and the docs repo's no-em-dashes, MDX authoring, alternate shading, content-outline and site-layout conventions; dropped file-scoped headings (no new file-scoped section).
- **Rationale**: each bears on a surface this plan changes.
- **Rejected**: listing the glossary entries (vocabulary, not rules).

### Partial renamed to report written knowledge (tasks)
- **Decision**: `implement-knowledge-candidates` becomes `implement-knowledge-written`; update_changelog writes durable discoveries immediately and lists them under `## Knowledge written` for the finished step to report.
- **Rationale**: the spec removes the offer, but the user must still be told what was written; reporting once at the end keeps the run's flow.
- **Rejected**: reporting each write mid-task only (lost in a long run).

### Ten tasks across four milestones (tasks)
- **Decision**: three index tasks, two prune tasks, three instruction tasks (standing rules and steps; skill; harbor instructions) and two doc tasks (maintainer docs; website in the docs repo). The skill task depends on prune because it names the new subcommands, which a contract test checks.
- **Rationale**: each task is one repo and independently testable; the template tasks touch no Go code paths of the index.
- **Rejected**: one large template task (too many pinned test files for one agent context).

### Website section placement (tasks)
- **Decision**: two new bands on the knowledge-base page, "How usage is recorded" and "Pruning the knowledge base", after "The lifecycle of an entry", re-alternating `surface` below them.
- **Rationale**: follows the lifecycle (create, search, prune) and the alternation convention.
- **Rejected**: a separate prune page (the knowledge page is the single home for the knowledge base).

## Rehydration cues

- `spektacular spec file read 000072_knowledge-access-index-and-prune`
- Read `internal/knowledge/set.go`, `cmd/knowledge.go`, `internal/store/store.go`, `internal/autocommit/lock.go`.
- Read `templates/skills/workflows/spek-knowledge/SKILL.md` and `templates/agents/knowledge-trigger.md`.
- `spektacular knowledge read --data '{"tier":"repo","name":"spektacular","path":"architecture/working-with-files-from-steps.md"}'` and `architecture/testing-architecture.md`.
- Website: `/home/nicj/code/github.com/hivecommons/spektacular-website/src/pages/knowledge-base.mdx` and `configuration.mdx`.
