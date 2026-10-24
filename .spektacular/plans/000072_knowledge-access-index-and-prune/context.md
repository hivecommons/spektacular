---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Context: 000072_knowledge-access-index-and-prune

## Current State Analysis

- Knowledge is reached only through `spektacular knowledge` commands (`cmd/knowledge.go`), over a `knowledge.Set` of addressed stores (`internal/knowledge/set.go`). Nothing records usage: `Set.Read`, `Set.Search` and `Set.AlwaysAppliedEntries` are pure reads, and `knowledge delete` reads an entry to test existence (`cmd/knowledge.go:554`).
- Entries carry optional frontmatter with tags only (`internal/store/frontmatter.go:10-60`); there is no creation date, and `FileStore.Stat` reports no birth time (`internal/store/store.go:236-253`).
- The store contract offers Read/List/Exists/Stat/Search and Write/Delete only (`internal/store/store.go:91-131`); there is no atomic update, and the only cross-process lock is the commit lock (`internal/autocommit/lock.go`).
- Orchestrated plan and implement runs keep lanes under `.spektacular/workflows/` (`internal/workflow/lane.go`), so several workflows can be in progress at once; knowledge commands do not name their workflow.
- Every installed instruction that touches capture says "offer, then wait": `templates/agents/knowledge-trigger.md` (+ short), `templates/agents/memory-context.md:11-12`, `templates/steps/plan/02-discovery.md:77-79`, `templates/steps/plan/18-walkthrough.md:25`, `templates/steps/implement/07-update_changelog.md:63-72`, `templates/partials/implement-knowledge-candidates.md`, `templates/partials/orchestrated-stop.md:9`, the epic skills, and `templates/skills/workflows/spek-knowledge/SKILL.md` (contribute/update/maintenance gates). Many phrase tests pin that wording (see Per-Task notes).
- Project config has `knowledge.sources` only (`internal/config/config.go:244-246`); settings format 4 (`internal/config/schema.go:19`).

## Per-Task Technical Notes

### Task: Add a shared file lock and an atomic store update

Requirements covered: "The index must stay correct when several workflow runs read and search knowledge at the same time" (constraint); foundation for "Parallel use is counted exactly".

**File changes**
- `internal/filelock/filelock.go` (new) — `Acquire(path string, timeout, stale time.Duration) (release func(), err error)` and `LockError{Path, Holder}`, moved from `internal/autocommit/lock.go:41-78` (O_CREATE|O_EXCL, "pid N at T" body, exponential backoff from 50ms capped at 1s, stale break, `trimNewline`). Keep a package-level `backoff` var tests can shorten.
- `internal/filelock/filelock_test.go` (new) — port `internal/autocommit/lock_test.go:32-133` cases (serialises 8 holders, timeout, stale break, reacquire) against `Acquire` directly.
- `internal/autocommit/lock.go:41-71` — `AcquireLock(dataDir)` becomes `filelock.Acquire(filepath.Join(dataDir, LockFile), LockTimeout, LockStale)`. Keep `LockFile`, `LockTimeout`, `LockStale` and `LockError` exported (alias or wrap `filelock.LockError` so `errors.As(err, *autocommit.LockError)` callers keep working: grep `LockError` in `cmd/` and `internal/` and keep their behaviour). `lockBackoff` in tests: route to `filelock`'s backoff var.
- `internal/store/store.go:116-123` — add to `Writer`: `Update(path string, fn func(current []byte, exists bool) ([]byte, error)) error` with a doc comment stating the atomicity contract and that a nil result writes nothing.
- `internal/store/store.go` (after `Delete`, ~:198) — `FileStore.Update`: resolve via `f.abs`, `MkdirAll` the parent, take `filelock.Acquire(abs+".lock", 30*time.Second, 10*time.Minute)`, read (ErrNotExist → exists=false), call fn, on non-nil result write to `abs+".tmp-<pid>-<rand>"` then `os.Rename` onto `abs`; release the lock in a defer. Lock/temp suffixes are exported constants (`LockSuffix = ".lock"`, `TempInfix = ".tmp-"`) so the knowledge layer can recognise them.
- `internal/store/ignore.go:85-87` — `func (s *ignoreStore) Update(path string, fn ...) error { return s.inner.Update(path, fn) }`.
- `internal/design/design_test.go:645` (`recordingWriter`) and `internal/knowledge/set_test.go:2329` (`deleteRecordingStore`) — add `Update` (delegate or record) so they still satisfy the interface.
- `internal/store/store_test.go` — tests: missing file passes `exists=false`; fn error leaves bytes unchanged and no temp file remains; nil result is a no-op; concurrency test: 16 goroutines × 25 increments of a JSON counter, each goroutine with its own `NewFileStore(root, …)`, final value 400; lock file removed afterwards.
- `internal/store/ignore_test.go` — `Update` delegates through the wrapper.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential (lock extraction first, then store method, then doubles and tests).

### Task: Keep a usage index in each knowledge store

Requirements covered: Searches are recorded, Reads are recorded (layer support), Usage is kept with the knowledge base, Every entry has a creation date, existing entries keep working (constraints), the index never edited or listed as an entry.

**File changes**
- `internal/knowledge/usage.go` (new):
  - `const UsageIndexFile = ".usage-index.json"`, `const usageRecentCap = 20`, `IsReservedPath(path)` true for `UsageIndexFile`, `UsageIndexFile+store.LockSuffix`, and any root-level name starting `UsageIndexFile+store.TempInfix`.
  - Types `UsageIndex{Version int "version"; Entries map[string]*UsageRecord "entries"}`, `UsageRecord{Created, TrackedSince time.Time "created","tracked_since"; Discoveries, Retrievals UsageLog}`, `UsageLog{Count int; Last *time.Time "last,omitempty"; Recent []UsageEvent "recent,omitempty"}`, `UsageEvent{At time.Time "at"; Workflow, Spec string omitempty}`, `WorkflowRef{Kind, Spec string}`, `CreatedDater func(location, path string) (time.Time, bool)`, `EntryUsage{Tier, Name, Path; UsageRecord; Tracked bool}`.
  - Encoding: `json.MarshalIndent(idx, "", "  ")` + trailing newline (map keys sort, so diffs are line-stable). Times in UTC RFC3339.
  - `(s *Set) update(src scopedStore, at time.Time, mutate func(idx *UsageIndex)) error` — single path: `src.store.Update(UsageIndexFile, …)` → unmarshal (empty/absent → `Version:1`), **reconcile** (list entries via `listFiles(src.store, "")`, skip `IsReservedPath` and `IsCategoryDescription`; add missing records with `Created` = dater(src.location, path) → else `src.store.Stat(path).ModTime` → else `at`, `TrackedSince` = `at`; delete records whose path is not listed), then `mutate`, then marshal.
  - `RecordDiscoveries(hits []store.Hit, at)` — group hits by (Tier, Name) → store; increment `Discoveries` for each hit path (append event, trim to cap, set Last).
  - `RecordRetrievals(entries []Entry, at, wf *WorkflowRef)` — same for `Retrievals`, events carry `wf`.
  - `EnsureRecord(addr, path, at)` — record created by reconcile if absent (Created=at for a brand-new file: dater falls back to ModTime ≈ now; set explicitly to `at` when the path had no record before this write), existing untouched.
  - `ForgetRecords(addr, paths)` — reconcile already drops vanished files; also delete explicitly named paths.
  - `Usage(sel Selector) ([]EntryUsage, error)` — read-only: `Read(UsageIndexFile)`, no reconcile, no write; entries listed but unrecorded are returned with `Tracked:false` and zero record (callers treat `TrackedSince` as "now" for the age rule).
  - `(s *Set) SetCreatedDater(d CreatedDater)` — optional injection; nil means no history lookup.
- `internal/knowledge/set.go:163-283` (Search) — skip hits where `IsReservedPath(hit.Path)` next to the always-applied and category-description exclusions; `:393-420` (List), `:435-487` (Tags), `:520-559` (readCategories) — skip `IsReservedPath` paths (simplest: filter inside `listFiles` callers via one helper `entryFiles(st, dir)` that drops reserved paths).
- `internal/knowledge/set.go:345-391` — `Read`, `Write`, `Delete` refuse `IsReservedPath(path)` with `output.NewError("knowledge_index_reserved", "…is the store's usage index, written only by Spektacular")` and next action `run "knowledge usage --tier <tier> --filter <name>" to read it; it cannot be written or deleted directly`.
- `internal/knowledge/set.go:24-31` — `scopedStore` already carries `location`; dater receives it.
- `internal/knowledge/usage_test.go` (new) — counts +1 with given times; workflow/spec kept/omitted; recent cap at 20 while count grows; write-then-rewrite keeps `Created` and counts; reconcile backfills via fake dater, falls back to ModTime when dater returns false; vanished file record dropped; reserved files absent from List/Search/Tags/AlwaysAppliedEntries; refusals on Read/Write/Delete of each reserved name; concurrency: 8 goroutines each with its own `NewSet` over the same temp store, each calls `RecordDiscoveries` and `RecordRetrievals` once for the same entry → exactly +8 each.
- `internal/knowledge/set_test.go` — extend existing List/Search fixtures with an index file present to prove exclusion.

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis, sequential integration: one agent writes `usage.go` + tests while another adds the reserved-path exclusions/refusals in `set.go` + tests; integrate and run `go test ./internal/knowledge/...`.

### Task: Record usage from the knowledge commands

Requirements covered: Searches are recorded, Reads are recorded (incl. workflow + spec), Every entry has a creation date (write path), Existing entries carry over, index cannot be hand-edited through the tool, usage readable by tools.

**File changes**
- `cmd/knowledge.go:283-300` (`newKnowledgeSet`) — after `knowledge.NewSet`, call `set.SetCreatedDater(gitCreatedDater)`.
- `cmd/knowledge_usage.go` (new):
  - `gitCreatedDater(location, path string) (time.Time, bool)` — `gitexec.Run(location, nil, "log", "--diff-filter=A", "--follow", "--format=%aI", "--", path)`; take the last non-empty line; parse RFC3339; any error (no git, not a work tree, untracked) → `(zero, false)`.
  - `activeWorkflow(spec string) *knowledge.WorkflowRef` — `dataDir()` (`cmd/root.go:482`); candidates: shared `stateFilePath(dir)` (`cmd/spec.go:94`) if it parses and `InProgress()` (kind + `data.name`); plus every `workflow.LaneNames(dir, k)` for k in plan, implement whose `workflow.ReadLane` is `InProgress()`. Deduplicate by (kind, name). If `spec != ""` and exactly one candidate has that name (prefer a lane over shared state when both match) → return it; if `spec != ""` but no candidate matches, fall through. Exactly one candidate overall → return it; else nil. Never errors.
  - `recordUsage(out map[string]any, fn func() error)` — runs fn, on error sets `out["usage_warning"]` to `"usage not recorded: <err>; the result above is complete, and the next knowledge command retries"`.
  - `knowledgeUsageCmd` (`Use: "usage"`, `--tier`/`--filter`, `--schema`) → `{"entries":[…EntryUsage]}` from `set.Usage(knowledgeSelector())`; output schema var.
- `cmd/knowledge.go:392-413` (`runKnowledgeSearch`) — after hits, `set.RecordDiscoveries(hits, time.Now().UTC())` via `recordUsage`; result map gains optional `usage_warning`.
- `cmd/knowledge.go:646-650` (`knowledgeAddressInput`) — add optional `Spec string \`json:"spec,omitempty"\``; `knowledgeAddressInputSchema` (:270-279) gains an optional `spec` property (not required).
- `cmd/knowledge.go:415-438` (`runKnowledgeRead`) — after successful read, `RecordRetrievals([]Entry{{Tier, Name, Path}}, now, activeWorkflow(input.Spec))`.
- `cmd/knowledge.go:583-600` (`runKnowledgeConventions`) and `:610-631` (`runKnowledgeAlwaysApplied`) — record a retrieval for every returned entry with `activeWorkflow(knowledgeSpec)`; new package var `knowledgeSpec` bound to a `--spec` flag registered on these two commands in `init` (:703-718), added to their schema flags (a copy of `knowledgeNarrowingFlags` plus `spec`, leaving list/tags/usage without it); reset is covered by `resetRootCmd`.
- `cmd/knowledge.go:478-513` (`runKnowledgeWrite`) — before `set.Write`, note whether the path existed (`set.Read` returns not-found); after the write call `set.EnsureRecord(addr, path, now)`; record failure → `usage_warning` (write still succeeds).
- `cmd/knowledge.go:515-569` (`runKnowledgeDelete`) — after delete, `set.ForgetRecords(addr, []string{path})`; failure → `usage_warning`. The existing existence `set.Read` at :554 stays untracked by construction.
- `cmd/knowledge.go:703-718` (`init`) — register `--tier`/`--filter` on `knowledgeUsageCmd`; add it to `knowledgeCmd.AddCommand`.
- Schemas at `cmd/knowledge.go:146-270` — add `usage_warning` to search/read/conventions/always-applied/write/delete output schemas; add `knowledgeUsageOutputSchema`.
- `cmd/knowledge_usage_test.go` (new; uses `twoScopeProject` `cmd/knowledge_test.go:117`, `runKnowledge` :163, `seedKnowledgeFile` :1056, `resetRootCmd`): search → +1 discovery at ~now in `knowledge usage`; read and always-applied → +1 retrieval; with a seeded in-progress `state.json` (kind plan, data.name X) the retrieval names plan/X; with two in-progress lanes (plan-A, plan-B) under `.spektacular/workflows/`, a read with `"spec":"B"` and `always-applied --spec A` name plan/B and plan/A, and a read without a spec names none; delete/tags/list record nothing; `knowledge write --data '{…"path":".usage-index.json"}'` refused with code `knowledge_index_reserved` and next action naming `knowledge usage`; same for `.usage-index.json.lock`, read and delete; `knowledge list` never lists it; write then rewrite keeps `created`; entry seeded before any index gets `created`/`tracked_since` after first command and is searchable/readable; holding the index lock file (create `.usage-index.json.lock` fresh, shorten timeout through a test hook) → read still returns content with `usage_warning`. Git backfill test guarded by `gittest.RequireGit` (`internal/testutil/gittest/gittest.go:18`) committing an entry with `GIT_AUTHOR_DATE` in the past and asserting `created` equals it.
- `internal/agent/instruction_surface_test.go:156-171` — no change needed (verbs list), but any registered-subcommand check picks up `usage` automatically.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential.

### Task: Add the prune age setting

Requirements covered: "Prune's age threshold must be a project setting with a default, not a fixed value."

**File changes**
- `internal/config/config.go:244-246` — `KnowledgeConfig` gains `Prune KnowledgePruneConfig \`yaml:"prune,omitempty"\``; new `KnowledgePruneConfig{MaxAgeDays int \`yaml:"max_age_days,omitempty"\`}`; `const DefaultKnowledgePruneMaxAgeDays = 90`; `func (k KnowledgeConfig) PruneMaxAgeDays() int` returns the default when `MaxAgeDays == 0`. Doc comment cites the `implement.worktrees` precedent (no settings-format bump; a config written before the key parses over NewDefault).
- `internal/config/config.go:874-898` (`KnowledgeConfig.Validate`) — refuse `MaxAgeDays < 0` with `output.NewError("config_invalid", "knowledge.prune.max_age_days must be a positive number of days").WithResource("knowledge.prune.max_age_days").WithNextAction("set knowledge.prune.max_age_days in .spektacular/config.yaml to a positive number of days (or remove the key to use 90)")`.
- `internal/config/config_test.go` (beside `TestFromYAMLFile_AbsentImplementWorktreesLoadsAsOn` :214 and `TestToYAMLFile_ImplementWorktreesFalseRoundTrips` :231) — absent → 90; `max_age_days: 30` loads and round-trips through `ToYAMLFile`; `-1` refused with `config_invalid` and the resource key; `TestNewDefault_HasExpectedDefaults` (:25) unchanged (zero value).
- No change to `internal/config/schema.go` or `internal/migrate`.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential.

### Task: Add the knowledge prune command

Requirements covered: Prune removes old, unused knowledge; Prune checks the remaining knowledge; Unmet targets reported as current; Prune acts and reports; Prune can be previewed; Pruning keeps the usage history in step; prune's own reads do not count; pre-index entries protected for the full threshold.

**File changes**
- `internal/knowledge/usage.go` — `PruneRemoval{Tier, Name, Path; Created, TrackedSince time.Time; LastRetrieval *time.Time; Discoveries int}` and `PruneCandidates(usage []EntryUsage, maxAge time.Duration, now time.Time) []PruneRemoval`: candidate iff `now-Created > maxAge` AND `now-TrackedSince >= maxAge` (untracked → TrackedSince=now → never) AND (`Retrievals.Last == nil` OR `now-*Last > maxAge`). Skips `IsCategoryDescription`.
- `internal/knowledge/usage_test.go` — table at maxAge 30d: created 40d/never retrieved → removed; 40d/retrieved once 5d ago → kept; 10d → kept; 40d with discoveries only → removed; backfilled created 400d but tracked 10d → kept; category README → never.
- `cmd/knowledge_prune.go` (new):
  - `knowledgePruneCmd` (`Use: "prune"`, flags `--dry-run`, `--force`, `--tier`, `--filter`, `--schema`) and `knowledgePruneVerdictCmd` (`Use: "verdict"`, `--data`, `--schema`), added under `knowledgePruneCmd`; register `knowledgePruneCmd` in `cmd/knowledge.go:717`.
  - Run file `filepath.Join(dataDir(), "knowledge-prune.json")` holding `pruneRun{StartedAt, DryRun, MaxAgeDays, Tier, Filter, Removed, Queue []pruneItem{Tier,Name,Path,Checksum}, Verdicts []pruneVerdict{Tier,Name,Path,Verdict,Evidence}}`, written with temp+rename.
  - `prune` with an existing run file and no `--force` → re-emit the queue head's instruction (resume), noting `"resumed": true`; with `--force` → discard and start over.
  - Start: `cfg.Knowledge.PruneMaxAgeDays()`; `set.Usage(sel)`; `PruneCandidates`. Real run: `set.Delete` each candidate then `set.ForgetRecords` per store (one index update per store). Dry run: no writes to any store. Queue = every remaining entry from `set.List(sel)` minus candidates, reserved paths and category descriptions, with `sha256` of `set.Read` bytes (untracked read).
  - Instruction: `stepkit.RenderTemplate("knowledge/prune-entry.md", map[string]any{"command": cfg.Command, "tier", "name", "path", "body": string(content), "dry_run": run.DryRun, "remaining": len(queue)})`.
  - `verdict`: parse `{tier,name,path,verdict,evidence}`; refuse when no run file (`knowledge_prune_not_running`, next action `run "<cmd> knowledge prune"`), when address ≠ queue head (`knowledge_prune_wrong_entry`, next action names the head's exact verdict command), unknown verdict, empty evidence for `corrected`/`deleted`. Claim checks via untracked `set.Read`: dry run → entry checksum must equal queued (`knowledge_prune_dry_run_changed`); real `deleted` → entry must be absent; `corrected` → present with different checksum; `current` → identical checksum (`knowledge_prune_claim_mismatch`, next action says what the store shows and how to fix). On accept: append verdict, pop head; next instruction or final report.
  - Final report `{"dry_run","max_age_days","entries":[…removed as "removed"/"would_remove" with evidence "created <date>, tracked since <date>, last retrieved <date|never>, <n> discoveries"; then verdicts mapped to corrected|deleted|current or would_correct|would_delete|current],"counts","complete":true}`; delete the run file. When the queue is empty at start, return the report immediately.
  - Errors through `output.NewError(...).WithNextAction(...)`; output schemas for both subcommands.
- `templates/knowledge/prune-entry.md` (new) — sections: the entry (tier/name/path, `{{{body}}}` verbatim in a fenced block); "Do not run `knowledge read` or `knowledge search` for this entry: its text is above, and prune's reads must not count as usage"; check it against the project's code; verdict rules (copy from `templates/skills/workflows/spek-knowledge/SKILL.md:124-157`: current incl. unmet standard and unverifiable, stale ⇒ deleted only when the subject no longer exists, incorrect ⇒ corrected, never from age or tone, evidence names the file/command/behaviour); how to correct (stage under `.spektacular/tmp/`, `{{command}} knowledge write --data … --file …`, carry frontmatter) or delete (`{{command}} knowledge delete --data …`); `{{#dry_run}}` block: change nothing, record the verdict you would give; the exact `{{command}} knowledge prune verdict --data '{"tier":"{{tier}}","name":"{{name}}","path":"{{path}}","verdict":"<current|corrected|deleted>","evidence":"<…>"}'` command; `{{remaining}}` entries left.
- `templates/knowledge_prune_template_test.go` (new, package `templates`) — pins: "states a standard the code has not yet met", "is **current**", "only an entry whose subject no longer exists", "evidence", "Do not run", "knowledge prune verdict", dry-run "change nothing".
- `cmd/knowledge_prune_test.go` (new; `twoScopeProject`, `runKnowledge`, inject the clock through a package var `pruneNow = time.Now`) — seed index records with chosen `created`/`tracked_since`/retrievals; age-rule cases at `max_age_days: 30`; old discovered-but-unread removed; real run: removed files gone and records gone; simulated agent corrects entry A via `knowledge write` then verdict corrected → accepted, A's counts unchanged; deletes entry B via `knowledge delete` then verdict deleted → accepted, B's record gone; entry C (unmet standard) verdict current → accepted, bytes unchanged; contradicted claims refused; wrong-entry verdict refused; missing evidence refused; resume returns the same head; `--force` restarts; report lists every entry; afterwards index has no record for removed/deleted. Dry run: hash every file in the store (via `os.ReadFile` in the test only) before and after, identical; report lists "would_remove" for the same candidates the real run removes; retrieval counts unchanged after prune and after a dry run.

**Complexity**: High
**Token estimate**: ~55k tokens
**Agent strategy**: Parallel analysis, sequential integration: one agent writes `PruneCandidates` + tests and the template + its contract test; a second writes `cmd/knowledge_prune.go` + command tests against them; integrate last.

### Task: Rewrite the installed instructions to write knowledge, then report it

Requirements covered: Agents write knowledge without asking; The user sees what was written; "Agents are no longer told to ask"; constraint "changes … made in the source Spektacular generates them from".

**File changes** (templates only; never `.spektacular/guidance/`, `AGENTS.md` or `.claude/skills/` in this repo)
- `templates/agents/knowledge-trigger.md:13-38` — replace the offer/accept/defer/decline flow with: recognise the moment (keep :8-12), write the entry yourself with the `spek-knowledge` skill (choosing tier, store, category and tags as it directs), then in your normal reply tell the user which entry you wrote, its tier, store and path, and why it is worth keeping; update an existing entry instead of duplicating; keep the closing "recognition trigger only" sentence (pinned by `internal/agent/standing_rules_test.go:610-617` fullOnly) reworded as needed.
- `templates/agents/short/knowledge-trigger.md:3-5` — e.g. "When you surface something durable and non-obvious (an unwritten convention, a gotcha, the reason behind a choice), write it via `spek-knowledge` yourself, then tell the user which entry, where and why. Before writing, read the full guidance." Keep heading; ≤6 lines, ≤100 runes per line, total budget 5120 bytes (`standing_rules_test.go:818-829,888,919`).
- `templates/agents/memory-context.md:11-12` — "The skill handles tier and store selection, search-before-write, and reporting what it wrote." (keeps the pinned fullOnly prefix "The skill handles tier and store selection").
- `templates/steps/plan/02-discovery.md:14` and `:33` — add `--spec {{plan_name}}` to the `knowledge always-applied` command and `"spec":"{{plan_name}}"` to the `knowledge read --data` example, so parallel lanes attribute their reads; keep the `knowledge always-applied` substring intact (harbor `CONVENTIONS_READ_COMMAND`, `tests/harbor/plan-workflow/tests/test_plan_workflow.py:121`); extend `internal/steps/plan/steps_test.go:184-194` to pin both.
- `templates/steps/plan/02-discovery.md:77-79` (Step 6) — write the learning with `knowledge write` after `knowledge sources`/`categories`/`tags` (or via the `spek-knowledge` skill), then mention the entry (tier, store, path, why) in the step's summary to the user; no confirmation.
- `templates/steps/plan/18-walkthrough.md:25` — a correction that reveals a durable lesson: write it through `spek-knowledge` and tell the user in the same review message which entry was written, where and why; drop accept/decline/defer.
- `templates/steps/implement/07-update_changelog.md:63-72` (Step 2b) — standalone and orchestrated: for each durable discovery, write it through `spek-knowledge` now (phrased as the general lesson) and add a line to a `## Knowledge written` list in `{{working_context_path}}` (standalone) or to the closing summary (orchestrated), naming tier, store, path and why.
- `templates/partials/implement-knowledge-candidates.md` — rename to `templates/partials/implement-knowledge-written.md`: "### Report the knowledge written" — if the run collected a `## Knowledge written` list, tell the user each entry (tier, store, path, why) in one message; otherwise say nothing. Update both includes in `templates/steps/implement/12-finished.md:18,60`; `:24,66` and `templates/steps/plan/19-finished.md:30` change "any durable discovery worth saving to the knowledge base" to "and every knowledge entry you wrote (tier, store, path, why)".
- `templates/partials/orchestrated-stop.md:9` — "Never ask whether to continue between tasks. Write durable knowledge yourself and list each entry you wrote (tier, store, path, why) in your closing summary."
- `templates/skills/workflows/spek-plan-epic/SKILL.md:68,108` and `templates/skills/workflows/spek-implement-epic/SKILL.md:82,127` — hand-back lists "every knowledge entry it wrote"; final review/report: "Report every knowledge entry the children wrote (tier, store, path, why)"; avoid new `ask*` words (allowlists `templates/plan_epic_skill_test.go:168-192`, `templates/implement_epic_skill_test.go:211-231`).
- Tests to rewrite to the new wording and add bans for "offer", "explicit acceptance", "not offered again", "waits for", "Knowledge candidates":
  - `internal/steps/implement/steps_test.go:438-471`
  - `internal/steps/plan/steps_test.go:184-194` (replace the "confirm" requirement with the write-then-report phrase) and `:549-574`
  - `internal/agent/standing_rules_test.go:601-617` (knowledge-trigger compact must contain "write" and "tell the user", trigger "read the full guidance", fullOnly kept)
  - `templates/implement_epic_skill_test.go:237-242`
  - add a `templates/` test that walks `agents/`, `steps/`, `partials/` and `skills/workflows/spek-{plan,implement}-epic` for the banned offer phrases.
- Leave `internal/agent/testdata/agents-md-legacy.md` and `cmd/testdata/agents-md-legacy.md` untouched (legacy-conversion inputs).

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: 2-3 parallel agents: (a) standing rules + memory-context + their tests, (b) plan/implement step templates, partials + their tests, (c) epic skills + their tests; then one `go test ./...`.

### Task: Make the knowledge skill write directly and drive prune

Requirements covered: Agents write knowledge without asking (skill), The user sees what was written, prune wrapped by a knowledge skill; Non-Goal: audit unchanged.

**File changes**
- `templates/skills/workflows/spek-knowledge/SKILL.md:10` — "five branches (lookup / contribute / update / audit / prune)"; "read, contribute, update, audit, and prune operations".
- `:12-24` (When to invoke) — keep "Is the knowledge base still accurate?" and "Review the knowledge base for anything out of date." as prune triggers; add "Prune the knowledge base."
- `:49-84` (contribute) — steps 1-5 unchanged; replace steps 6-7 with: write with `knowledge write --data … --file …`, then tell the user tier, store, path, tags and why (no confirmation); step 8 unchanged.
- `:85-99` (update) — same change: write, then report the change (a short diff summary).
- `:100-123` (audit) — unchanged, including per-entry confirmation (pinned by `internal/agent/instruction_surface_test.go:663-715`).
- `:124-157` (maintenance) — replace with "# Intent: prune": run `{{command}} knowledge prune --dry-run` when the user asks to preview, otherwise `{{command}} knowledge prune`; follow each returned instruction and answer with `{{command}} knowledge prune verdict …` until the final report; present the report (every entry with verdict and evidence); then the category-description drift check from old step 6 (report only, `{{command}} init <agent>` as the remedy). State that prune acts without asking and that recovery is through version history.
- `:159-167` (Decline handling) — narrow to the audit intent (the only remaining confirmation gate); remove the maintenance paragraph and the "write without explicit user approval is a bug" sentence for contribute/update.
- `internal/agent/instruction_surface_test.go` — rewrite `:307-394` (contribute/update now pin "write" + "tell the user the tier, store name, path, tags" and ban "Only after explicit confirmation" in those intents), `:541-560` and `:788-809` (five branches incl. prune), `:682-687` (decline wording now audit-only), `:750-779` slices (prune slice instead of maintenance), `:957-1016` (replace with prune intent pins: "knowledge prune", "knowledge prune verdict", "--dry-run", drift report); keep `:663-715` audit pins.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential.

### Task: Drop the obsolete knowledge-offer line from the end-to-end suites

Requirements covered: Success metric "zero should-I-save questions" (suite instructions no longer presuppose an offer); harbor coupling per knowledge `architecture/testing-architecture.md`.

**File changes**
- `tests/harbor/epic-workflow/instruction.md:55` — remove "- If you are offered to save something to the knowledge base, decline."
- `tests/harbor/implement-epic-workflow/instruction.md:56` — same.
- `tests/harbor/goal-check-workflow/instruction.md:41` — same.
- Do not touch `tests/harbor/.build/` or `tests/harbor/jobs/` (generated, gitignored).

**Complexity**: Low
**Token estimate**: ~3k tokens
**Agent strategy**: Single agent, sequential.

### Task: Update the README and the maintainer knowledge-base doc

Requirements covered: Documentation describes the new behaviour (maintainer docs; the website is the next task).

**File changes**
- `README.md:255-264` (CLI list) — add bullets for `knowledge usage` and `knowledge prune` (with `--dry-run` and `knowledge prune verdict`).
- `README.md:266-268` ("Capturing knowledge") — replace "proposes … waits for your explicit confirmation … never persists … unprompted" with write-then-report wording and a sentence on the usage index and prune.
- `README.md:279-330` (project configuration block) — add a commented `knowledge.prune.max_age_days` line only if it does not disturb the pinned fenced-block checks in `cmd/docs_test.go:198-244` (block must keep `sources:` and `- name: team`); otherwise describe it in prose.
- `docs/knowledge-base.md:333-349` (Contributing knowledge) — write-then-report; `:381-406` ("Reviewing whether entries are still true") → "### Pruning the knowledge base" (two passes, age rule incl. tracking clock, verdicts, unmet-standard rule, evidence, dry run, prune's reads not counted, recovery via version history); new "## The usage index" section (file at store root, discoveries vs retrievals, workflow attribution only when one workflow runs, creation dates and backfill, never edited by hand, `knowledge usage`); `:408-425` command table — rows for `knowledge usage` and `knowledge prune`; update the delete row's "maintenance pass" wording to "prune".
- Keep pinned phrases (`cmd/docs_test.go:153-329`, `internal/knowledge/ranking_docs_test.go`): "There is no precedence between stores", "## Two tiers, and how a store is addressed", ranking constants unchanged.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential.

### Task: Document self-written knowledge, the usage index and prune on the website

Requirements covered: Documentation describes the new behaviour; acceptance "The website documents it".

**File changes**
- `docs:src/pages/knowledge-base.mdx:4` — description adds "and pruned".
- `docs:src/pages/knowledge-base.mdx:116-118` — capture passage rewritten (write then report).
- `docs:src/pages/knowledge-base.mdx:147-152` ("Keeping it up to date") — point to the new prune section.
- `docs:src/pages/knowledge-base.mdx:164-165` — "a maintenance pass that retries is safe" → "a prune that retries is safe".
- `docs:src/pages/knowledge-base.mdx:177` (after "The lifecycle of an entry" `</Section>`) — new `<Section heading="How usage is recorded">` and `<Section heading="Pruning the knowledge base">` bands, each with `<Prose nested>` markdown and fenced bash blocks per the plan's content outline; set `surface` so bands alternate (lifecycle is plain at :94, ranking is `surface` at :179: insert usage as `surface`, prune plain, and flip `surface` on the following sections as needed so alternation holds down the page).
- `docs:src/pages/knowledge-base.mdx:339-341` — tag-proposal sentence: tags chosen from the vocabulary, written without a confirmation step; `:343-367` tag audit unchanged.
- `docs:src/pages/knowledge-base.mdx:373-483` (Configuration) — paragraph + YAML block for `knowledge.prune.max_age_days`.
- `docs:src/pages/configuration.mdx:79-85` — add `prune:` / `max_age_days: 90` to the example `knowledge:` block; `:342-356` project `knowledge` ConfigKey — add the `knowledge.prune.max_age_days` bullet and `defaultValue` per the content example (HTML `<code>` in `defaultValue`). The "Seventeen top-level keys" count at :105-109 is unchanged (subkey).
- `docs:src/pages/configuration.mdx:640` — guidance list entry for `knowledge-trigger.md`: describe it as "write durable knowledge, then report it".
- Rules: no `<div>`, `<section>`, `class=` in page bodies; blank lines inside slots; no em dashes in new prose; run `npm run build` and `npx astro check`.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential.

## Testing Strategy

Per task (detail in each task's notes above):

- **Add a shared file lock and an atomic store update**: unit tests for `filelock` (ported commit-lock cases) and `FileStore.Update` (absent file, aborting callback, nil result, 16x25 concurrent increments with separate store instances), ignore-wrapper delegation.
- **Keep a usage index in each knowledge store**: unit tests for record/ensure/forget, recent cap, reconcile with fake dater and ModTime fallback, reserved-path exclusion and refusal, exact counts under 8 concurrent set instances.
- **Record usage from the knowledge commands**: cmd tests in throwaway projects: discovery/retrieval counts via `knowledge usage`, workflow attribution with one vs two running workflows, untracked internal reads, index-path refusals, creation-date survival and backfill (git-guarded), `usage_warning` when the lock is held.
- **Add the prune age setting**: config load/round-trip/refusal tests beside the `implement.worktrees` ones.
- **Add the knowledge prune command**: `PruneCandidates` table with an injected clock; template contract test; cmd tests with a simulated agent for removal, corrected/deleted/current claim checks, refusals, resume/force, report completeness, index consistency, byte-identical dry run, untracked prune reads.
- **Rewrite the installed instructions to write knowledge, then report it**: rewritten phrase tests plus a banned-phrase walk over agent templates, steps, partials and epic skills; compact standing-rule budgets.
- **Make the knowledge skill write directly and drive prune**: rewritten instruction-surface tests (contribute/update without gate, prune intent, audit unchanged, registered subcommands).
- **Drop the obsolete knowledge-offer line from the end-to-end suites**: no Go test; covered by the manual Harbor runs.
- **Update the README and the maintainer knowledge-base doc**: existing doc pin tests keep passing.
- **Document self-written knowledge, the usage index and prune on the website**: `npm run build` and `npx astro check` in the docs repo.

Manual items for the implementation test plan: Harbor runs of plan, implement, epic, implement-epic and goal-check suites showing zero knowledge-save questions; a prune dry run on this project's own knowledge base with the installed binary (between workflows); an agent session writing an entry without asking and naming it; an agent-driven prune in a seeded project (wrong flag corrected, removed command deleted, unmet standard kept) with checkable evidence; reading the website pages in a local build.

## Project References

- Spec: `spektacular spec file read 000072_knowledge-access-index-and-prune` (epic `000002_workflow-tracing-and-run-stats`, independent of its siblings).
- Design documents: none referenced by the spec.
- Knowledge entries: `architecture/working-with-files-from-steps.md` (widen the store contract), `architecture/testing-architecture.md` (three layers, harbor coupling), `architecture/knowledge-search-ranking.md` (ranking untouched), `gotchas/storedirs-rewrites-paths-and-forbids-outside-root.md` (no new store dir in config), `gotchas/mustache-html-escaping.md` (render the entry body with triple braces), and the conventions listed in plan.md.
- Repo roots: `spektacular` at `/home/nicj/code/github.com/hivecommons/spektacular`; `docs` at `/home/nicj/code/github.com/hivecommons/spektacular-website`.
- Requirement → repo/files: index, recording, creation dates, usage command, prune, setting, instructions, harbor instructions, README and maintainer doc → `spektacular` (files per task above); website documentation → `docs` (`src/pages/knowledge-base.mdx`, `src/pages/configuration.mdx`).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Largest tasks: the usage index (~45k) and the prune command (~55k), each split across two agents with sequential integration.

## Migration Notes

No settings-format bump and no `migrate` step. Existing entries gain index records lazily on the first knowledge command that writes the index; their tracking clock starts then, so the first prune cannot remove them for non-use until a full age window has passed. The index file appears in each knowledge store (and in git) after first use. Never regenerate or migrate this repository's own `.spektacular/` during a workflow; install the binary between workflows.

## Performance Considerations

Every search and read now does one locked read-modify-write of a small JSON file per store touched, plus a store listing for reconcile; the backfill's `git log` runs once per entry ever (only for entries without a record). The recent-event lists are capped at 20 per entry so the index stays small. Lock waits are bounded at 30s, and a recording failure never fails the command.
