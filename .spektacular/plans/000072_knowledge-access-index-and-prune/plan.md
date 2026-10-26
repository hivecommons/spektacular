---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Plan: 000072_knowledge-access-index-and-prune

<!-- Metadata -->
<!-- Created: 2026-10-24T20:25:36Z -->
<!-- Commit: 6730f8a -->
<!-- Branch: b-bob-epic-fixes -->
<!-- Repository: git@github.com:hivecommons/spektacular.git -->

## Overview

Coding agents will save durable knowledge to the project's knowledge base themselves and tell the user what they wrote, instead of stopping to ask. To keep a self-written knowledge base trustworthy, each knowledge store gains a JSON usage index recording when every entry was created, discovered by search and retrieved by read, safe under parallel workflow runs. A new `spektacular knowledge prune` command removes old entries nobody reads and then has the agent check every remaining entry against the project, correcting or deleting what is wrong, with a dry-run preview and a full report. Users stop being interrupted, and maintainers can see which knowledge earns its place.

## Conventions

- **Error messages must describe the problem and suggest remediation** — every new refusal (index path written or read directly, a verdict for the wrong entry, a verdict whose claim the store contradicts, missing evidence, an invalid `knowledge.prune.max_age_days`, a lock timeout) is built with `output.NewError(...).WithNextAction(...)` naming the exact command to run instead.
- **Spektacular's own files are written through Spektacular, never with file tools** — the usage index is a store file reached only by knowledge commands; the new `knowledge usage` command gives read access, writes to the index path are refused, and prune corrections and deletions go through `knowledge write`/`knowledge delete`.
- **A plan never changes the active skills and configuration** — every instruction change is made in `templates/` and verified with Go tests and throwaway projects; no task re-inits, migrates or regenerates this repo's `.spektacular/`, AGENTS.md or installed skills.
- **Tests must not depend on execution order** — new cmd tests go through `resetRootCmd`/`runKnowledge`; time-dependent prune tests inject a clock rather than relying on the wall clock, and the concurrency test owns its temp store.
- **Passing tests are required before calling work done** — every task finishes with the full Go test suite green; the website task finishes with a clean site build and type check.
- **No em dashes (docs repo)** — the website prose added for this feature uses commas, colons or separate sentences.
- **MDX authoring conventions (docs repo)** — new website content is Section/Prose bands with markdown bodies and fenced code blocks, no layout HTML in page bodies.
- **Alternate section background shading (docs repo)** — any new `Section` on the knowledge-base page sets `surface` opposite to its neighbour.
- **Plans must sketch content structure (docs repo)** — the website task carries a content outline with headings and illustrative examples.
- **File-scoped section headings (docs repo)** — dropped: no new file-scoped reference section is added; the configuration change extends the existing `knowledge` ConfigKey.
- **Site layout conventions (docs repo)** — apply by reusing existing section components only; no new component is needed.

## Architecture & Design Decisions

The work lands in two repos. In **spektacular** it has three parts: a per-store usage index maintained by the knowledge layer, a two-pass `knowledge prune` command, and a rewrite of the generated instructions so agents write knowledge and then report it. In **docs** (the website) it updates the knowledge-base and configuration pages.

**Usage index.** Each knowledge store gets one JSON file at its root, `.usage-index.json`. It holds a record per entry path: `created` (the creation date), `tracked_since` (when the index first saw the entry), and two usage blocks, `discoveries` and `retrievals`. Each block has a total `count`, a `last` time and a capped `recent` list of events; a retrieval event also names the running workflow and spec. The index is changed only inside `internal/knowledge` (new `usage.go`), through a new atomic read-modify-write on the store contract, `store.Writer.Update(path, fn)`. Following knowledge `architecture/working-with-files-from-steps.md` ("a gap in the interface is a reason to widen the interface"), the contract is widened rather than routed around. `FileStore.Update` takes a portable `O_CREATE|O_EXCL` lock file beside the target. The lock comes from a new `internal/filelock` package generalised from `internal/autocommit/lock.go`, which `autocommit` then delegates to. It writes through a temp file and rename, so parallel workflow runs never lose an update. Every index write also reconciles the store: entries with no record get one, with the creation date backfilled from git's first-add date (via an injected dater built on `gitexec.Run`), falling back to the file's modification time; records for files that no longer exist are dropped. This keeps pre-existing entries working with no hand edits, and it starts their tracking clock on first use after the upgrade, not on the first prune. Recording is an explicit call made by the command handlers of `knowledge search` (one discovery per returned hit), and of `knowledge read`, `knowledge always-applied` and `knowledge conventions` (one retrieval per returned entry). `Set.Read`, `Set.Search` and the rest stay pure, so the existence check in `knowledge delete`, `knowledge tags` and prune's own reads never count. `knowledge write` creates a record on first write and leaves an existing one untouched, so the creation date and the counts survive updates. `knowledge delete` drops the record. The index, its lock and its temp files are excluded from list, search, tags and the always-applied reader. `knowledge read`, `write` and `delete` refuse those paths with a remediation naming the new read-only `knowledge usage` command, which is how tools and people read the index through the CLI (convention: store files are reached through Spektacular). A failure to record usage never fails the read or search it rides on: the result carries a `usage_warning` field instead. The running workflow is resolved in `cmd`. `knowledge read` accepts an optional `spec` in `--data`, and `always-applied`/`conventions` accept an optional `--spec` flag. When the named spec has an in-progress workflow (its lane, or the shared `state.json`), that workflow is recorded. The plan discovery step passes its spec name, so parallel epic runs are attributed correctly. Without a spec, the CLI names the workflow only when exactly one is in progress, rather than guess.

**Prune.** `spektacular knowledge prune [--dry-run] [--tier …] [--filter …]` runs the first pass entirely in the command. An entry is removed when it was created more than the configured age ago, has been tracked for at least that long, and has no retrieval within it. A discovery does not keep it. The age is `knowledge.prune.max_age_days` in `config.yaml`, an optional key defaulting to 90 with no settings-format bump, following the `implement.worktrees` precedent. Category descriptions and the index are never candidates. A real run deletes the removed entries and their records in one index update. The command then writes a small run file under `.spektacular/` listing the remaining entries, and returns the first entry's instruction, rendered from a new embedded template with the entry's body inlined. Because the body is inlined, the agent never reads the entry with `knowledge read`, and prune's reads stay untracked. The template carries the maintenance review's verdict rules: only an entry whose subject no longer exists is deleted; an entry that disagrees with the code states the target and is current; unverifiable is current. The agent corrects with `knowledge write`, deletes with `knowledge delete`, then reports through `spektacular knowledge prune verdict --data '{…,"verdict":"current|corrected|deleted","evidence":"…"}'`. That call checks the claim against the store: deleted means the entry is gone, corrected means its checksum changed, current means it is unchanged. It requires evidence for every change, then returns the next instruction, or the final report once the queue is empty. The report lists every entry, with removed entries carrying their age and usage evidence, and the run file is then deleted. In a dry run, nothing in the knowledge base or the index is written: removals are reported as "would remove", and verdicts are recorded as proposals while the entry is required to be unchanged. Running `knowledge prune` again resumes an unfinished run; `--force` discards it. This mirrors how workflows drive agents (one instruction per call), without a competing state machine; `research.md#alternatives-considered-and-rejected` covers the FSM, `--untracked` and frontmatter alternatives.

**Instructions and docs.** Every source template that today tells an agent to offer knowledge and wait now says to write the entry with the `spek-knowledge` skill and then tell the user which entry was written, where (tier, store, path) and why. The sources are `templates/agents/knowledge-trigger.md` and its short form, the plan discovery and walkthrough steps, the implement `update_changelog` and `finished` steps with their partials, `orchestrated-stop`, the two epic orchestrator skills, and the `spek-knowledge` skill's contribute and update intents. Orchestrated children write too, and list what they wrote in their hand-back. The `spek-knowledge` skill's maintenance intent becomes a prune intent that drives the command loop. Its audit (tag review) intent is unchanged, per the spec's Non-Goals. Changes are made only in `templates/`, never in the installed copies (convention: plans never change the active install). README, `docs/knowledge-base.md` and the website's `knowledge-base.mdx` and `configuration.mdx` describe writing without asking, the usage index, prune, its dry-run preview and the age setting.

See [research.md#alternatives-considered-and-rejected](./research.md#alternatives-considered-and-rejected) for the rejected options and their evidence.

## Component Breakdown

- **File lock (new, spektacular).** A small, portable cross-process lock taken by creating a lock file exclusively, with backoff, a timeout and a stale-lock break. It is generalised from the commit lock so both the commit lock and the store's atomic update share one implementation. The commit lock keeps its current behaviour by delegating to it.
- **Store contract: atomic update (changed, spektacular).** The store's writing half gains one operation: read the current bytes of a path, let a caller compute the new bytes, and write them back as one indivisible step that no other process can interleave with. The file store implements it with the file lock and a temp-file-and-rename write. The ignore-filtering wrapper delegates it untouched. This is the only way the usage index is ever written.
- **Usage index (new, spektacular knowledge layer).** Owns the index document's format, its reserved file names, and every change to it. It records discoveries and retrievals, creates a record on first write while keeping an existing record's creation date and counts, drops a record on deletion, and computes which entries the age rule removes. On every write it reconciles the index with the store's entries, backfilling a creation date for entries it has not seen before through an injected creation-dater. It runs entirely through the store contract above, so it never touches the filesystem directly.
- **Knowledge set (changed, spektacular).** Keeps its existing read, search, list, tags and always-applied behaviour, which stays untracked. It now hides the index's reserved files from every enumeration and search, refuses reads, writes and deletes addressed at them, and exposes the usage index per addressed store for the command layer to call.
- **Knowledge commands (changed, spektacular).** `search`, `read`, `always-applied` and `conventions` record usage after producing their result, and attach a warning instead of failing when recording cannot complete. `write` ensures a record exists. `delete` forgets one. A new read-only `usage` command reports the index's records for the stores a request covers. The command layer also supplies the two facts the knowledge layer cannot know. One is the running workflow: the spec the caller names through an optional `spec` field or `--spec` flag, matched to its in-progress workflow, or otherwise the single running workflow when there is exactly one. The other is the git-history creation-dater.
- **Prune command (new, spektacular).** `knowledge prune` runs the age-based first pass, applies or (in a dry run) only reports the removals, and keeps a run file of the entries still to check. Each call hands the agent one instruction for the next entry, with the entry's body inlined so the agent never needs a tracked read. `knowledge prune verdict` records the agent's verdict and evidence for that entry, checks the claim against the store, and returns the next instruction or the final report. The run file is removed when the report is delivered. A repeat `knowledge prune` resumes an unfinished run; `--force` restarts it.
- **Prune entry instruction (new template, spektacular).** The prose handed to the agent for each entry. It covers the verdict rules carried over from the maintenance review (an unmet target is current, only a vanished subject is deleted, unverifiable is current, evidence is required for every change), how to correct or delete through the knowledge commands, what changes in a dry run, and the exact verdict command to run next.
- **Project configuration (changed, spektacular).** The knowledge section gains an optional prune age setting in days, with a default of 90 when absent. It is validated as a positive number and needs no settings-format change.
- **Standing rule and agent instructions (changed templates, spektacular).** The knowledge-trigger standing rule (short AGENTS.md form and full guidance), the memory-context guidance, the plan discovery step (which also passes its spec name on knowledge reads) and walkthrough step, the implement update-changelog and finished steps and their knowledge partial, the orchestrated stop partial, and the two epic orchestrator skills. All of them switch from "offer and wait" to "write it with the knowledge skill, then tell the user which entry, where and why". Orchestrated children list the entries they wrote in their hand-back.
- **spek-knowledge skill (changed template, spektacular).** Its contribute and update intents write and then report, with no confirmation gate. Its maintenance intent is replaced by a prune intent that drives the prune command loop and its dry-run preview, and still reports drifted category descriptions without repairing them. The lookup and audit (tag review) intents are unchanged.
- **Template-contract and command tests (changed, spektacular).** The phrase assertions that pin the old offer wording are rewritten to pin the new write-then-report wording, and new tests cover the index, the store update, prune, the setting and the new commands.
- **Harbor suite instructions (changed, spektacular).** Three headless suite instructions that tell the agent to decline knowledge offers are updated, since no offer is made any more.
- **Maintainer documentation (changed, spektacular).** The README's knowledge CLI list and capture paragraph, and the knowledge-base doc's contributing, maintenance and command-reference sections.
- **Website knowledge and configuration pages (changed, docs).** The knowledge-base page describes writing without asking, the usage index and prune with its preview. The configuration page documents the prune age setting under the knowledge key.

## Data Structures & Interfaces

**Store contract: atomic update.** One method is added to the writing half of the store interface. Every store implementation and test double gains it.

```go
// Update reads path (exists=false when absent), passes the bytes to fn, and
// writes fn's result back as one step no other process can interleave with.
// fn returning (nil, nil) leaves the file untouched; an error aborts with no write.
Update(path string, fn func(current []byte, exists bool) ([]byte, error)) error
```

**File lock.** It replaces the commit lock's private implementation. The commit lock keeps its exported behaviour.

```go
func Acquire(path string, timeout, stale time.Duration) (release func(), err error)
type LockError struct{ Path, Holder string }
```

**Usage index document.** One per knowledge store, at the store root as `.usage-index.json`, with its lock and temp files beside it. It is pretty-printed JSON with sorted keys, so diffs stay line-stable.

```json
{
  "version": 1,
  "entries": {
    "gotchas/example.md": {
      "created": "2026-07-01T10:00:00Z",
      "tracked_since": "2026-10-24T09:00:00Z",
      "discoveries": { "count": 3, "last": "2026-10-24T09:12:00Z",
                       "recent": [ { "at": "2026-10-24T09:12:00Z" } ] },
      "retrievals":  { "count": 1, "last": "2026-10-24T09:13:00Z",
                       "recent": [ { "at": "2026-10-24T09:13:00Z",
                                     "workflow": "plan", "spec": "000072_x" } ] }
    }
  }
}
```

Go shapes in the knowledge layer:

```go
type UsageIndex  struct { Version int; Entries map[string]*UsageRecord }
type UsageRecord struct { Created, TrackedSince time.Time; Discoveries, Retrievals UsageLog }
type UsageLog    struct { Count int; Last *time.Time; Recent []UsageEvent } // Recent capped
type UsageEvent  struct { At time.Time; Workflow, Spec string }            // workflow/spec omitted when unknown
type WorkflowRef struct { Kind, Spec string }                              // the one running workflow, if any
type CreatedDater func(storeLocation, entryPath string) (time.Time, bool)  // git first-add date, injected by cmd

func IsReservedPath(path string) bool // index, lock and temp names; excluded and refused everywhere

// Set methods (all through Store.Update; none of them counts as a read)
func (s *Set) RecordDiscoveries(hits []store.Hit, at time.Time) error
func (s *Set) RecordRetrievals(entries []Entry, at time.Time, wf *WorkflowRef) error
func (s *Set) EnsureRecord(addr Address, path string, at time.Time) error
func (s *Set) ForgetRecords(addr Address, paths []string) error
func (s *Set) Usage(sel Selector) ([]EntryUsage, error)           // read-only, no reconcile
func PruneCandidates(usage []EntryUsage, maxAge time.Duration, now time.Time) []PruneRemoval
```

**Command results (JSON, through the existing output writer).**

- `knowledge search`, `read`, `always-applied` and `conventions` keep their shapes. They gain an optional `usage_warning` string, present only when recording failed.
- `knowledge read --data` accepts an optional `"spec"` alongside `tier`/`name`/`path`. `always-applied` and `conventions` accept an optional `--spec <name>`. Both only attribute the retrieval and never change what is returned.
- `knowledge usage` (flags `--tier`, `--filter`) → `{"entries":[{"tier","name","path","created","tracked_since","discoveries":{…},"retrievals":{…}}]}`.
- `knowledge prune` (flags `--dry-run`, `--force`, `--tier`, `--filter`) → `{"dry_run", "max_age_days", "removed":[PruneRemoval], "remaining", "entry":{"tier","name","path"}, "instruction"}`. When no entry remains it returns the final report directly.
- `knowledge prune verdict --data '{"tier","name","path","verdict":"current|corrected|deleted","evidence"}'` → the next `{"entry","instruction","remaining"}`, or the final report.
- The final report is `{"dry_run","max_age_days","entries":[{"tier","name","path","verdict":"removed|corrected|deleted|current","evidence"}],"counts":{…},"complete":true}`. In a dry run the verdicts read `would_remove|would_correct|would_delete|current`.

```go
type PruneRemoval struct { Tier, Name, Path string; Created, TrackedSince time.Time; LastRetrieval *time.Time; Discoveries int }
```

**Prune run file.** It lives at `.spektacular/knowledge-prune.json` and is the command's own state, never a knowledge-store file.

```go
type pruneRun struct {
    StartedAt time.Time; DryRun bool; MaxAgeDays int; Tier string; Filter []string
    Removed  []PruneRemoval
    Queue    []pruneItem      // entries still to check, head = current
    Verdicts []pruneVerdict   // recorded so far
}
type pruneItem    struct { Tier, Name, Path, Checksum string } // checksum at queue time, for verdict checks
type pruneVerdict struct { Tier, Name, Path, Verdict, Evidence string }
```

**Configuration.**

```yaml
knowledge:
  prune:
    max_age_days: 90   # optional; absent or 0 means 90; negative is refused
```

```go
type KnowledgeConfig struct { Sources []SourceConfig; Prune KnowledgePruneConfig `yaml:"prune,omitempty"` }
type KnowledgePruneConfig struct { MaxAgeDays int `yaml:"max_age_days,omitempty"` }
func (k KnowledgeConfig) PruneMaxAgeDays() int // 90 when unset
```

**Prune entry instruction template.** A new embedded mustache template, rendered with `{tier, name, path, body, dry_run, remaining, command}`. The entry body is rendered with triple braces so the agent sees it verbatim.

## Implementation Detail

**New patterns.**

- **Atomic read-modify-write on the store contract.** This is the first store operation that serialises across processes. It follows the existing rule that a missing capability widens the contract (as `Stat` did). Callers get the guarantee from the interface rather than from knowing it is a file. A future non-file backend implements `Update` with its own transaction or compare-and-swap.
- **Usage bookkeeping beside, not inside, the read path.** The knowledge set's existing reads stay pure. Recording is a separate, explicit call that a command handler makes after it has produced the result the agent asked for. A developer reading a handler sees one extra line, "record what this returned", and an internal read can never count by accident. That property is what lets prune and delete read freely.
- **Reconcile-on-write.** Every index update first brings the index in line with the store's actual entries. Unseen entries gain a record with a backfilled creation date and a fresh tracking clock, and records for vanished files are dropped. That one rule covers pre-existing entries, hand-written entries and entries removed outside Spektacular, so no migration step exists. Creation-date lookup is injected (git first-add date from the command layer), keeping the knowledge layer free of git and keeping its tests deterministic.
- **A command-driven agent loop outside the workflow engine.** Prune is a short, resumable loop. One call returns one instruction, and the agent answers through a verdict call that the command checks against the store before advancing. It reuses the workflow pattern (embedded mustache instruction, "run this command next", resume on re-entry) without a state machine, a lane or a step table. The loop's state is a small run file the command owns and deletes on completion. Its first pass is pure computation over the index (`PruneCandidates`), unit-testable with an injected clock.
- **Claim checking.** The verdict call does not trust the agent's word. Deleted means gone, corrected means the content changed, current means untouched, and in a dry run nothing may change at all. The final report can therefore never disagree with the store, which is how "a real prune leaves no entry the report does not account for" is guaranteed rather than hoped for.

**Existing patterns followed.**

- Errors use `output.NewError(...).WithNextAction(...)` with exact runnable remediation. That covers the reserved index path, a verdict for an entry other than the queue head, a contradicted claim, missing evidence, a lock timeout, an invalid age setting, and a prune started while another is unfinished without `--force`.
- New subcommands follow the existing knowledge command shape: a cobra command, a `--schema` output, `--tier`/`--filter` narrowing, `--data` JSON input, and results through the shared output writer. `--dry-run` matches the other commands' preview flag.
- Instructions live as embedded templates rendered through the shared step-template renderer, so template-contract tests can pin their phrasing the same way they pin step templates.
- The optional config key follows the defaulted-setting precedent: an accessor supplies the default, validation refuses only bad explicit values, and there is no settings-format bump.
- The standing-rule heading stays fixed, so AGENTS.md section ordering, the omp rule copies and the legacy-conversion tests are untouched. Only the body text changes, within the compact-section line and byte budgets.

**Code-structure UX.** The knowledge package gains a `usage` file (index format, reconcile, record and forget operations, prune candidate rule) beside `set.go`. `set.go` gains only exclusions and thin per-store accessors. The command package gains a prune file (both subcommands, the run file, report assembly) and a small workflow-resolution helper reused by the read handlers. Instruction text for prune sits with the other templates, and nothing about the index format leaks into `cmd`. The commit lock becomes a two-line wrapper over the shared file lock, so there is one lock implementation to reason about.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Upstream specs and plans: none must land first.** The epic `000002_workflow-tracing-and-run-stats` lists this spec as independent. Sibling `000073_log-mode-and-logger-providers` also edits project configuration (debug becomes logging) and may edit the website configuration page. The two touch different structs and different sections, but whichever merges second rebases its config and docs edits onto the other's.
- **`internal/store` (changed).** Provides the store contract. It gains the atomic `Update` operation, implemented by the file store and delegated by the ignore wrapper; the two test doubles that implement the writer gain it too.
- **`internal/autocommit` lock (changed).** Its lock-file logic moves into a new shared file-lock package that it then delegates to, with behaviour and tests unchanged.
- **`internal/knowledge` (changed).** Gains the usage index and prune-candidate rule, and the set gains exclusions and record accessors. Search ranking is untouched, so the ranking constants and their doc pins do not move.
- **`internal/config` (changed).** `KnowledgeConfig` gains the optional prune age setting and its validation. No settings-format bump and no migrate step.
- **`internal/workflow` (unchanged, used).** Shared state and lane readers are used to resolve the one running workflow for retrieval attribution.
- **`internal/gitexec` (unchanged, used).** Runs `git log` for the creation-date backfill. Git is optional: without it, or outside a work tree, the backfill falls back to the file's modification time.
- **`internal/stepkit` renderer and `templates/` (changed).** A new prune instruction template is added, and the knowledge-capture prose across the standing rules, plan and implement steps, partials and skills is rewritten.
- **`internal/agent` standing rules (unchanged code, changed templates).** It installs the rewritten knowledge-trigger and memory-context text. The compact-section line and byte budgets constrain the short form.
- **External libraries: none added.** Existing `cobra`, `cbroglie/mustache`, `yaml.v3` and `testify` cover everything.
- **Docs website repo `docs` (changed).** The knowledge-base and configuration pages are updated. The site build and type check must pass.

## Testing Approach

Testing follows the project's three layers (knowledge `architecture/testing-architecture.md`): Go unit and command tests for the mechanics, template-contract phrase tests for the rewritten instructions, and Harbor end-to-end runs for agent behaviour, which are manual because Harbor does not run in CI.

**Unit tests (most coverage: the index, the store update and the prune rule).**
- Store atomic update: a missing file passes `exists=false`; an aborting callback writes nothing; a nil result leaves the file untouched; the ignore wrapper delegates. Concurrency: many goroutines, each holding its own store instance (as separate processes would), each increment a counter through `Update`, and the final count equals the number of increments exactly. The lock's timeout and stale-break behaviour carry over from the commit lock's tests, which keep passing against the shared lock.
- Usage index: a discovery and a retrieval each add exactly one to the count, with the given time. A retrieval carries the workflow and spec when one is supplied and omits them otherwise. The recent list stays capped while the count keeps rising. A first write sets `created` and `tracked_since`, and a later write keeps both and keeps the counts. Forgetting removes the record. Reconcile adds records for unseen entries using the injected dater, falls back when the dater has nothing, and drops records for vanished files. Reserved files never appear in list, search, tags or the always-applied reader. The parallel-use guarantee from the spec is asserted here: N concurrent set instances each search for and read the same entry once, and the index shows exactly N more discoveries and N more retrievals.
- Prune rule, with an injected clock: at a 30-day threshold, an entry created 40 days ago and never retrieved is removed. One created 40 days ago and retrieved once within the window is kept. One created 10 days ago is kept. An old entry with discoveries but no retrievals is removed. A pre-existing entry backfilled as 400 days old but tracked for less than the threshold is kept. Category descriptions are never candidates.
- Configuration: an absent key loads as 90, an explicit value round-trips, and a negative value is refused with a `config_invalid` error naming the key and the fix.

**Command tests (throwaway projects through the existing knowledge test helpers).**
- `search` records one discovery per returned hit. `read`, `always-applied` and `conventions` record one retrieval per returned entry. `delete`'s existence check, `tags`, `list` and prune record nothing.
- A read naming a spec whose lane is in progress, with another lane also running, names that spec's workflow. A read without a spec names the single running workflow, and names none when two lanes run or none do.
- `write` to the index path (and to its lock) is refused with remediation naming `knowledge usage`. `read` and `delete` of it are refused too. The index never appears in `list` or `search`.
- An entry written then rewritten keeps its first creation date. An entry seeded on disk before any index exists gets a creation date after the first command, can be searched and read, and is not a prune candidate until tracked for the full threshold.
- A recording failure (index lock held past the timeout) still returns the read's content, with `usage_warning`.
- Prune end to end with a simulated agent (the test performs the writes and deletes an agent would). It covers age-based removal and its report evidence. A corrected entry is accepted only when its checksum changed, and its usage counts are unchanged afterwards. A deleted entry is accepted only when it is gone, and its record is gone. A "current" entry, an unmet-standard entry, must be byte-identical. A verdict for the wrong entry, or missing evidence for a change, is refused. Resume returns the same entry, and `--force` restarts. The final report lists every entry with a verdict, and after a real run the index holds no record for any removed or deleted entry. Dry run: the same age-based removals are reported as "would remove", and every entry file and the index are byte-for-byte unchanged afterwards (hashes compared before and after). Prune's own reads, in a dry run or a real run, change no retrieval count.

**Template-contract tests.** The existing phrase tests that pin "offer, then wait" are rewritten to pin the new wording, and ban the old phrases from every source that installs agent instructions. Those sources are the standing rule's short and full forms, memory-context, the plan discovery and walkthrough steps, the implement update-changelog and finished steps, the orchestrated stop partial, both epic skills and spek-knowledge. New wording is pinned for "write it, then tell the user which entry, where and why". The spek-knowledge contribute and update intents carry no confirmation gate, and the skill advertises the prune intent in place of maintenance while the audit intent's per-entry confirmation stays pinned. The prune instruction template is pinned for the unmet-target rule, the vanished-subject rule, evidence, the dry-run rule and the verdict command. The compact standing-rule budgets keep passing.

**Docs.** The maintainer-doc pin tests keep passing (the ranking constants do not move). The website's guard is its build and type check.

**Success metrics.**
- Zero "should I save this to the knowledge base?" questions in a full run of the end-to-end suites. **Manual — captured in the implementation test plan** (Harbor runs of the plan, implement, epic, implement-epic and goal-check suites after the change, scanned for knowledge-save questions). The template-contract bans guard the source prose automatically.
- 100% of searches and reads reflected in the index, with no lost updates in parallel. **Behavioural test**: the command-level recording tests plus the concurrent-instances test assert exact counts.
- A prune preview on this project's own knowledge base lists every entry with a verdict, and a real prune leaves no entry unaccounted for. The "no entry unaccounted for" half is a **behavioural test** (the final report must cover every entry, and verdict claims are checked against the store). The preview on this project's own knowledge base is **Manual — captured in the implementation test plan**, run with the installed binary between workflows, never during one.
- Every corrected or deleted entry carries evidence a reviewer can check without opening the code. Evidence is required by the verdict call (**behavioural test** for its presence). Whether it is checkable without the code is **Manual — captured in the implementation test plan**.

**Manual reviews.**
- **Manual — captured in the implementation test plan**: an agent session that surfaces a durable fact writes the entry without asking and names it (tier, store, path, reason) in its reply. This is spec acceptance "A learned fact is saved without a question".
- **Manual — captured in the implementation test plan**: in a seeded project with one entry describing a flag wrongly and one describing a removed command, an agent-driven prune corrects the first, deletes the second, and keeps an unmet-standard entry as current, with evidence in the report.
- **Manual — captured in the implementation test plan**: the website knowledge-base and configuration pages read correctly in a local build.

**Deliberate gaps.** No subprocess-level concurrency test: separate store instances in one process exercise the same file-lock path that separate processes use, as the commit lock's tests already do. No new Harbor verifier assertion: the suites are not run in CI, and their instructions are only updated to drop the obsolete "decline the offer" line.

## Milestones & Tasks

### Milestone 1: The knowledge base records how its entries are used

**What changes**: Every knowledge store keeps a usage index. Each search records which entries it surfaced. Each read, and each load of the always-applied rules, records which entries were retrieved, naming the workflow and spec it ran under. Workflow steps say which spec they belong to, so parallel runs are told apart. Every entry has a creation date that survives later updates, and entries written before this change get one automatically. Parallel workflow runs never lose an update. The index cannot be written through the knowledge commands and never shows up as an entry, and a new read-only command lets people and tools see it. Nothing about how agents capture knowledge changes yet.

**Validation point**: The full Go test suite passes, including the exact-count concurrency test. In a throwaway project, a `knowledge search` followed by a `knowledge read` shows one more discovery and one more retrieval in `knowledge usage`, and `knowledge write` to the index path is refused.

#### - [x] Task: Add a shared file lock and an atomic store update
**Id:** 6a171f75-0217-4ee5-864e-228ba48424b0
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Gives the store one operation that reads a file, computes new contents and writes them back without any other process interleaving. Today the store can only overwrite, so two parallel workflow runs updating the same file would lose one update. The commit lock's portable lock-file logic moves into a shared package that both the commit lock and the new store operation use.

*Technical detail:* [context.md#task-add-a-shared-file-lock-and-an-atomic-store-update](./context.md#task-add-a-shared-file-lock-and-an-atomic-store-update)

**Acceptance criteria**:
- [x] Many concurrent updaters, each with its own store instance, incrementing one value end with exactly the expected total.
- [x] An update whose callback fails leaves the file exactly as it was.
- [x] The commit lock behaves as before, with its existing tests unchanged and passing.
- [x] Every store implementation, including the ignore-filtering wrapper and the test doubles, supports the new operation.

#### - [x] Task: Keep a usage index in each knowledge store
**Id:** 8343b890-245d-4391-99eb-fea26576a100
**Repo:** spektacular
**Depends on:**
- 6a171f75-0217-4ee5-864e-228ba48424b0 — Add a shared file lock and an atomic store update
**Execution:** agent

Adds the usage index to the knowledge layer: its JSON format, the operations that record discoveries and retrievals, create a record on first write, forget a record on deletion, and report usage. Every index write also reconciles it with the store, backfilling creation dates for entries it has not seen. The index's files are hidden from every listing and search and are refused as entry addresses.

*Technical detail:* [context.md#task-keep-a-usage-index-in-each-knowledge-store](./context.md#task-keep-a-usage-index-in-each-knowledge-store)

**Acceptance criteria**:
- [x] Recording a discovery or a retrieval adds exactly one to that entry's count, with the time given, and a retrieval keeps the workflow and spec it was given.
- [x] An entry written and then rewritten keeps its first creation date and its counts.
- [x] An entry that predates the index gains a record with a creation date (from version history when available) and a tracking start of now.
- [x] Records for removed entries disappear, and the index never appears as an entry in list, search, tags or the always-applied rules.
- [x] Concurrent searches and reads of one entry from several independent instances are all counted, none lost.

#### - [x] Task: Record usage from the knowledge commands
**Id:** 381790da-cb87-4cbf-b720-35b146519d5a
**Repo:** spektacular
**Depends on:**
- 8343b890-245d-4391-99eb-fea26576a100 — Keep a usage index in each knowledge store
**Execution:** agent

Wires the index into the commands people and agents run. Searches record discoveries. Reads and always-applied loads record retrievals, naming the running workflow and spec: the spec the caller passes, or the only running workflow. Writes ensure a record and deletes forget one. A new read-only `knowledge usage` command shows the index. Writing, reading or deleting the index through the knowledge commands is refused with a pointer to that command, and a failure to record never fails the search or read itself.

*Technical detail:* [context.md#task-record-usage-from-the-knowledge-commands](./context.md#task-record-usage-from-the-knowledge-commands)

**Acceptance criteria**:
- [x] After a search returning an entry, `knowledge usage` shows one more discovery for it at the search's time.
- [x] After an entry is read, or loaded as an always-applied rule, it shows one more retrieval. A read that names its spec is attributed to that spec's running workflow even when other workflows run in parallel, and an unnamed read during a single running workflow names that workflow.
- [x] The delete command's existence check, the tags and list commands, and other internal reads change no count.
- [x] Writing to the index's location through `knowledge write` is refused with a remediation, and the index is not listed or returned as an entry.
- [x] A search or read still returns its result, with a warning, when usage cannot be recorded.
- [x] Existing entries written before the change can still be searched and read, and gain a creation date with no manual edit.

### Milestone 2: A prune command removes unused knowledge and checks the rest

**What changes**: `spektacular knowledge prune` removes entries older than the project's age setting (90 days by default) that have been tracked that long and never retrieved in that time, even if searches surfaced them. It then walks the agent through every remaining entry, one instruction at a time, to correct an entry that describes its subject wrongly, delete one whose subject is gone, and keep one that states a standard the code has not met yet. It finishes with a report of every entry's verdict and evidence. `--dry-run` produces the same report and changes nothing, not even the usage index. Prune's own reads never count as usage.

**Validation point**: The full Go test suite passes, including the age-rule table, the simulated-agent prune run, and the byte-for-byte dry-run check. A dry run in a throwaway project returns an instruction per entry and a complete report, and leaves every file unchanged.

#### - [x] Task: Add the prune age setting
**Id:** 9cba7df7-f7b8-4472-82a1-5647512eb0bd
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

Adds an optional `knowledge.prune.max_age_days` project setting with a default of 90 when it is absent. Existing configurations keep loading unchanged, with no settings-format change and no migration. A negative value is refused with an error that names the key and how to fix it.

*Technical detail:* [context.md#task-add-the-prune-age-setting](./context.md#task-add-the-prune-age-setting)

**Acceptance criteria**:
- [x] A configuration without the key reports an age of 90 days.
- [x] A configured age is read and survives being saved and reloaded.
- [x] A negative age is refused with a message naming the key and the fix.
- [x] Configurations written before the change load without error and without a migration.

#### - [x] Task: Add the knowledge prune command
**Id:** 2e76ead4-71c7-4af7-a8d1-1815e29189ff
**Repo:** spektacular
**Depends on:**
- 381790da-cb87-4cbf-b720-35b146519d5a — Record usage from the knowledge commands
- 9cba7df7-f7b8-4472-82a1-5647512eb0bd — Add the prune age setting
**Execution:** agent

Adds `knowledge prune` and `knowledge prune verdict`. The first pass removes entries that are old, tracked long enough, and never retrieved within the age window. The command then hands the agent one instruction per remaining entry, with the entry's text inlined, to correct it, delete it or keep it. Each verdict is checked against the store before the next instruction. The final report lists every entry with its verdict and evidence. `--dry-run` changes nothing anywhere, and an unfinished run resumes.

*Technical detail:* [context.md#task-add-the-knowledge-prune-command](./context.md#task-add-the-knowledge-prune-command)

**Acceptance criteria**:
- [x] With a 30-day age, an entry created 40 days ago and never retrieved is removed, while one retrieved once in that window and one created 10 days ago are kept.
- [x] An old entry that searches surfaced but nobody read is removed, and an entry tracked for less than the age is never removed for non-use.
- [x] A corrected entry keeps its usage counts, and a removed or deleted entry leaves no record in the index.
- [x] A verdict that the store contradicts, a verdict for the wrong entry, or a change without evidence is refused with a remediation.
- [x] The instruction for each entry tells the agent to keep an entry stating an unmet standard as current, and to delete only an entry whose subject no longer exists.
- [x] A dry run reports the same removals a real run would make and a verdict for every entry, and afterwards every entry and the index are byte-for-byte unchanged.
- [x] Prune's own reads, in a real run or a dry run, change no retrieval count.

### Milestone 3: Agents write knowledge themselves and say what they wrote

**What changes**: The instructions Spektacular installs tell agents to save durable knowledge on their own and then tell the user which entry they wrote, where, and why, instead of offering and waiting. This covers the AGENTS.md standing rule and its guidance, the plan and implement workflow steps, the epic orchestrator skills, and the knowledge skill. The knowledge skill's maintenance review becomes a prune flow that drives the new command, while its tag audit stays as it is. The end-to-end suites stop telling the agent to decline offers that no longer happen.

**Validation point**: The full Go test suite passes with the rewritten template-contract tests, and initialising a throwaway project installs guidance with no "offer and wait" wording.

#### - [x] Task: Rewrite the installed instructions to write knowledge, then report it
**Id:** 3f3644b0-86f5-424d-98e0-285470de7180
**Repo:** spektacular
**Depends on:**
- 381790da-cb87-4cbf-b720-35b146519d5a — Record usage from the knowledge commands
**Execution:** agent

Changes every source template that tells an agent to offer knowledge and wait. These are the AGENTS.md standing rule and its guidance, the memory-context guidance, the plan discovery and walkthrough steps, the implement changelog and finished steps, the orchestrated stop partial, and both epic orchestrator skills. They now say to write the entry through the knowledge skill and then tell the user which entry was written, where and why. Orchestrated children write too and list their entries in the hand-back. The plan discovery step also passes its spec name on its knowledge reads, so parallel runs are attributed. The template-contract tests move to the new wording and ban the old.

*Technical detail:* [context.md#task-rewrite-the-installed-instructions-to-write-knowledge-then-report-it](./context.md#task-rewrite-the-installed-instructions-to-write-knowledge-then-report-it)

**Acceptance criteria**:
- [x] The guidance a fresh `init` installs tells agents to write durable knowledge themselves and report it, and no longer tells them to offer it and wait for a decision.
- [x] No plan, implement or epic instruction asks the user whether to save knowledge, in standalone or orchestrated runs.
- [x] Each instruction tells the agent to name the entry it wrote, its store and path, and why it was worth keeping.
- [x] The AGENTS.md knowledge section keeps its heading and stays within the compact-section limits.
- [x] The plan discovery step's knowledge reads name the plan's spec.

#### - [x] Task: Make the knowledge skill write directly and drive prune
**Id:** 7e7bd29f-237e-44f2-9ba5-b6b97cb6ef43
**Repo:** spektacular
**Depends on:**
- 2e76ead4-71c7-4af7-a8d1-1815e29189ff — Add the knowledge prune command
**Execution:** agent

Updates the `spek-knowledge` skill. Contributing and updating now write the entry and then report it, with no confirmation gate. The maintenance review is replaced by a prune intent that runs the prune command loop, explains its dry-run preview, and still reports drifted category descriptions without repairing them. Lookup and the tag audit are unchanged.

*Technical detail:* [context.md#task-make-the-knowledge-skill-write-directly-and-drive-prune](./context.md#task-make-the-knowledge-skill-write-directly-and-drive-prune)

**Acceptance criteria**:
- [x] The installed skill writes a new or revised entry without asking and then tells the user the tier, store, path, tags and reason.
- [x] Asking whether the knowledge base is still accurate leads the agent to the prune command, with the dry run offered as a preview.
- [x] The tag audit still proposes and confirms one entry at a time, exactly as before.
- [x] Every command the skill tells the agent to run is a real knowledge subcommand.

#### - [x] Task: Drop the obsolete knowledge-offer line from the end-to-end suites
**Id:** f9150144-92e1-4ac8-8908-c38ba4132052
**Repo:** spektacular
**Depends on:**
- 3f3644b0-86f5-424d-98e0-285470de7180 — Rewrite the installed instructions to write knowledge, then report it
**Execution:** agent

Three headless end-to-end suites tell the agent to decline knowledge offers, and after this change no offer is made. The line is removed so the suites describe the current behaviour and do not prompt the agent about a question that no longer exists.

*Technical detail:* [context.md#task-drop-the-obsolete-knowledge-offer-line-from-the-end-to-end-suites](./context.md#task-drop-the-obsolete-knowledge-offer-line-from-the-end-to-end-suites)

**Acceptance criteria**:
- [x] No end-to-end suite instruction mentions declining an offer to save knowledge.
- [x] The suites' instructions still tell the headless agent not to wait for a person.

### Milestone 4: The documentation describes self-written knowledge, the usage index and prune

**What changes**: The README, the maintainer knowledge-base doc and the website's knowledge-base and configuration pages describe the new behaviour. Agents write knowledge without asking and report it. Usage is recorded in an index kept with each store. Prune runs in two passes, has a dry-run preview, and has an age setting.

**Validation point**: The full Go test suite passes (the doc pin tests included), and the website builds and type-checks cleanly.

#### - [x] Task: Update the README and the maintainer knowledge-base doc
**Id:** ccc22bb9-b485-4b1d-be15-06a360ae8440
**Repo:** spektacular
**Depends on:**
- 2e76ead4-71c7-4af7-a8d1-1815e29189ff — Add the knowledge prune command
- 7e7bd29f-237e-44f2-9ba5-b6b97cb6ef43 — Make the knowledge skill write directly and drive prune
**Execution:** agent

Brings the README's knowledge section and the maintainer knowledge-base doc in line. They describe agents writing knowledge without asking and reporting it, the usage index, `knowledge usage`, and `knowledge prune` with its dry run and age setting. The old "proposes and waits" and per-entry maintenance-review passages are replaced.

**Content example** (README "Capturing knowledge", illustrative): "When research surfaces a durable learning, gotcha or convention, the agent writes it to the knowledge base itself through the `spek-knowledge` skill and tells you which entry it wrote, where and why. Usage is recorded in an index inside each store, and `spektacular knowledge prune` (preview with `--dry-run`) removes entries nobody has read within `knowledge.prune.max_age_days` and has the agent correct or delete the rest." New command-reference rows: `spektacular knowledge usage` — "Show each entry's creation date and discovery and retrieval history from the store's usage index"; `spektacular knowledge prune` — "Remove old, unread entries, then check every remaining entry against the project; `--dry-run` previews without changing anything". The maintenance-review subsection becomes "### Pruning the knowledge base", keeping its rule that an entry stating an unmet standard is current.

*Technical detail:* [context.md#task-update-the-readme-and-the-maintainer-knowledge-base-doc](./context.md#task-update-the-readme-and-the-maintainer-knowledge-base-doc)

**Acceptance criteria**:
- [x] Neither document says the agent waits for confirmation before writing knowledge.
- [x] Both list `knowledge usage` and `knowledge prune` with what they do, and the knowledge-base doc explains the two prune passes, the dry run and the age setting.
- [x] The existing documentation pin tests still pass.

#### - [x] Task: Document self-written knowledge, the usage index and prune on the website
**Id:** 7cf807cc-1f23-45ee-9d2d-b400af7cb0df
**Repo:** docs
**Depends on:**
- 2e76ead4-71c7-4af7-a8d1-1815e29189ff — Add the knowledge prune command
**Execution:** agent

Updates the website's knowledge-base page to describe writing without asking, the usage index, and prune with its dry-run preview. Updates the configuration page to document the prune age setting under the knowledge key. The content follows the site's MDX and layout conventions.

**Content outline** (knowledge-base page; wording illustrative, facts fixed):

1. "The lifecycle of an entry", Creating (existing section, rewritten passage): "You rarely run this by hand. When an agent learns something durable while working, it writes the entry itself through the `spek-knowledge` skill and then tells you which entry it wrote, in which store and path, and why. Nothing waits for your approval; your version history is the undo."
2. New Section "How usage is recorded" (placed after "The lifecycle of an entry", `surface` opposite its neighbour): each store keeps `.usage-index.json` at its root; a search that returns an entry records a discovery, a read or an always-applied load records a retrieval (naming the workflow and spec it was made under); entries carry a creation date that survives updates; entries from before the index get one automatically and start being tracked on first use. Example:
   ```bash
   spektacular knowledge usage --tier repo --filter spektacular
   ```
   Plus a sentence that the index is written only by Spektacular and `knowledge write` refuses its path.
3. New Section "Pruning the knowledge base" (alternating `surface`): pass one removes entries older than `knowledge.prune.max_age_days` (default 90) that have been tracked that long and not retrieved in that time, even if searches surfaced them; pass two walks the agent through every remaining entry to correct, delete or keep it (an entry stating a standard the code has not met yet is kept as current; only an entry whose subject is gone is deleted); the report lists every entry as removed, corrected, deleted or current with evidence. Examples:
   ```bash
   spektacular knowledge prune --dry-run
   spektacular knowledge prune
   spektacular knowledge prune verdict --data '{"tier":"repo","name":"spektacular","path":"gotchas/x.md","verdict":"current","evidence":"flag still exists in cmd/knowledge.go"}'
   ```
   And: the dry run (preview) changes no entry and no usage record; prune's own reads never count as usage; recovery from a prune is through version history.
4. "Configuration" section: one paragraph and a YAML block showing `knowledge.prune.max_age_days: 90`.
5. "Tagging an entry": the tag-proposal sentence at the capture passage becomes "chooses tags from the vocabulary"; the tag audit paragraph is unchanged.

**Content example** (configuration page, inside the existing project `knowledge` ConfigKey, `defaultValue="none; <code>prune.max_age_days: 90</code>"`):
- `knowledge.prune.max_age_days`: how many days an entry may go unretrieved before `spektacular knowledge prune` removes it. An entry must also have been tracked for that long. Defaults to 90 when absent; must be positive.

*Technical detail:* [context.md#task-document-self-written-knowledge-the-usage-index-and-prune-on-the-website](./context.md#task-document-self-written-knowledge-the-usage-index-and-prune-on-the-website)

**Acceptance criteria**:
- [x] The knowledge-base page describes writing without asking, the usage index, the prune command, its preview mode and its age setting.
- [x] The configuration page documents `knowledge.prune.max_age_days` with its default of 90.
- [x] The site builds and type-checks without errors, and the new prose contains no em dashes.

## Open Questions

None. Every design choice was settled while planning and recorded as an assumption. One behaviour can only be confirmed once the code runs: whether `git log --follow` reports the first-add date reliably for entries that were renamed inside a store. If it does not, the implementer keeps the modification-time fallback for those entries and notes it in the changelog. This cannot cause a wrong prune, because the tracking clock, not the creation date, protects pre-existing entries.

## Out of Scope

- Ranking search results by usage: the index records usage but does not change how search orders results (spec Non-Goal).
- Undoing a prune or keeping an archive of pruned entries: recovery is through the project's version history (spec Non-Goal).
- Running prune automatically on a schedule: it runs only when invoked (spec Non-Goal).
- Changing the knowledge categories, the tag vocabulary or how stores are laid out: the usage index is the only addition to a store (spec Non-Goal).
- Changing the knowledge skill's tag audit: it keeps its per-entry propose-and-confirm flow (spec Non-Goal).
- Attributing ad hoc reads that do not name their spec while several workflows run in parallel: such a retrieval names no workflow. Reads made by the workflow templates pass their spec and are attributed. Run-level attribution can follow from the epic's trace and run-id work (`000073_log-mode-and-logger-providers`, `000074_trace-spans-for-agent-work`).
- Regenerating this repository's own installed AGENTS.md, guidance and skills: they pick up the new wording the next time the maintainer installs the binary and runs the version check, never inside a workflow.
- New Harbor verifier assertions for knowledge behaviour: the suites only drop the obsolete "decline the offer" line, and the zero-questions metric is checked by a manual Harbor run.

## Changelog


### 2026-10-25 — Task: Add a shared file lock and an atomic store update

**What was done**: The commit lock's lock-file logic moved into a new `internal/filelock` package (`Acquire`, `AcquireWithBackoff`, `LockError`), which `autocommit.AcquireLock` now delegates to. The store's `Writer` gained `Update(path, fn)`, implemented by `FileStore` with a `<path>.lock` file lock and a temp-file-and-rename write, delegated by the ignore wrapper, and added to the three test doubles.

**Deviations**: `autocommit.LockError` stays its own type (the filelock error is mapped onto it) so the commit lock's exact error text and its unchanged tests keep passing; `filelock.AcquireWithBackoff` was added so the commit lock keeps its own `lockBackoff` knob. A third test double, `tagBlindStore` in `internal/knowledge/set_test.go`, also needed `Update`. The temp file is named `<base>.tmp-<random>` via `os.CreateTemp` rather than `<pid>-<rand>`.

**Files changed**:
- `spektacular: internal/filelock/filelock.go`
- `spektacular: internal/filelock/filelock_test.go`
- `spektacular: internal/autocommit/lock.go`
- `spektacular: internal/store/store.go`
- `spektacular: internal/store/ignore.go`
- `spektacular: internal/store/store_test.go`
- `spektacular: internal/store/ignore_test.go`
- `spektacular: internal/design/design_test.go`
- `spektacular: internal/knowledge/set_test.go`

**Discoveries**: The lock's exponential backoff starts at 50ms and caps at 1s, so a contended test (16 writers) takes about 14s unless it shortens `filelock.Backoff`; tests that contend on `Update` should shorten it with a restoring cleanup. `store.LockSuffix` (".lock") and `store.TempInfix` (".tmp-") are exported for the knowledge layer to recognise the index's sidecar files.

### 2026-10-25 — Task: Keep a usage index in each knowledge store

**What was done**: Added `internal/knowledge/usage.go`: the `.usage-index.json` format (version, entries keyed by path, created/tracked_since, discoveries and retrievals with count, last and a recent list capped at 20), `IsReservedPath`, and the Set operations `RecordDiscoveries`, `RecordRetrievals`, `EnsureRecord`, `ForgetRecords`, read-only `Usage` and `SetCreatedDater`. Every index change runs through one `store.Update` that first reconciles the index with the store (backfilling unseen entries from the injected dater, else the file's modification time, and dropping records whose file no longer exists). `set.go` hides the index, its lock and temp files from listings (inside `listFiles`) and search, and refuses them in `Read`, `Write` and `Delete` with `knowledge_index_reserved`.

**Deviations**: Reconcile drops a record only when its file no longer exists (`Exists`), not when the listing omits it, so an entry hidden by `.spektacular_ignore` but read by exact path keeps a stable record. The reserved-path exclusion lives inside `listFiles` itself rather than a separate `entryFiles` helper, since every caller wants it. `EntryUsage` also carries a `tracked` flag in JSON.

**Files changed**:
- `spektacular: internal/knowledge/usage.go`
- `spektacular: internal/knowledge/usage_test.go`
- `spektacular: internal/knowledge/set.go`

**Discoveries**: `ForgetRecords` on a path whose file still exists is undone at the next index write, because reconcile re-adds every listed file; callers must delete the file first (as `knowledge delete` and prune do). Category descriptions (`<category>/README.md`) are never given a record.

### 2026-10-25 — Task: Record usage from the knowledge commands

**What was done**: `knowledge search` records a discovery per returned hit; `knowledge read`, `always-applied` and `conventions` record a retrieval per returned entry, naming the running workflow (an optional `"spec"` in read's `--data`, an optional `--spec` on the two loaders, else the only in-progress workflow across the shared state and the spec/plan/implement lanes). `write` ensures a record, `delete` forgets one, a recording failure becomes a `usage_warning` field, and a new read-only `knowledge usage` command reports the index. `newKnowledgeSet` injects a git first-add dater (`git log --diff-filter=A --follow --format=%aI`).

**Deviations**: The recording-failure test corrupts the index rather than holding its lock past a shortened timeout, so no test hook was added to `internal/store`. Read gets its own input schema (`knowledgeReadInputSchema`) carrying the optional `spec`, leaving write/delete's schema unchanged. Two existing schema tests in `cmd/knowledge_test.go` now list `usage_warning`. A clock var `knowledgeNow` was added for tests. `cmd/root.go`, `cmd/spec.go`, `gittest.go` and `instruction_surface_test.go` needed no change.

**Files changed**:
- `spektacular: cmd/knowledge.go`
- `spektacular: cmd/knowledge_usage.go`
- `spektacular: cmd/knowledge_usage_test.go`
- `spektacular: cmd/knowledge_test.go`

**Discoveries**: As planned, a read naming a spec that has no running workflow falls through to the single-running-workflow rule, so it can be attributed to another spec's workflow when exactly one other runs; the goal reviewer flagged this as untested. Every knowledge search or read now writes `.usage-index.json` into the store root, which in a repo-tier store is a git-tracked file.

### 2026-10-25 — Task: Add the prune age setting

**What was done**: `KnowledgeConfig` gained an optional `prune.max_age_days` (`KnowledgePruneConfig`, omitempty), `DefaultKnowledgePruneMaxAgeDays = 90` and a `PruneMaxAgeDays()` accessor that supplies the default. `KnowledgeConfig.Validate` refuses a negative value with `config_invalid`, naming the key and the fix. No settings-format bump and no migration.

**Deviations**: None.

**Files changed**:
- `spektacular: internal/config/config.go`
- `spektacular: internal/config/config_test.go`

**Discoveries**: None.

### 2026-10-25 — Task: Add the knowledge prune command

**What was done**: Added `knowledge.PruneCandidates` (the age rule: created and tracked for at least the age, no retrieval within it, untracked entries and category descriptions never removed), the embedded instruction `templates/knowledge/prune-entry.md` (entry body inlined with triple braces, the verdict rules, correct/delete commands, dry-run block, exact verdict command), and `cmd/knowledge_prune.go` with `knowledge prune [--dry-run] [--force] [--tier] [--filter]` and `knowledge prune verdict`. The command removes candidates and their records in a real run, queues every remaining entry with its checksum in `.spektacular/knowledge-prune.json`, hands out one instruction per entry, checks each verdict against the store (deleted means gone, corrected means changed, current means unchanged, a dry run must change nothing), and returns a final report of every entry with its verdict and evidence before deleting the run file.

**Deviations**: Added a refusal `knowledge_prune_in_progress` when the requested mode differs from an unfinished run's (a `--dry-run` while a real prune is unfinished, or the reverse), instead of silently resuming the other kind of run; `--force` starts the requested one. The template contract test is package `templates_test`, since stepkit imports `templates`. `cmd/knowledge.go` and the spek-knowledge skill were not touched by this task (the skill is the next task).

**Files changed**:
- `spektacular: internal/knowledge/usage.go`
- `spektacular: internal/knowledge/usage_test.go`
- `spektacular: cmd/knowledge_prune.go`
- `spektacular: cmd/knowledge_prune_test.go`
- `spektacular: templates/knowledge/prune-entry.md`
- `spektacular: templates/knowledge_prune_template_test.go`

**Discoveries**: After `--force`, entries the discarded run already removed are not listed in the new run's report (their removal already happened). `ForgetRecords` stamps its reconcile with the wall clock rather than prune's injected clock, so an entry first tracked during a prune's removal update gets the real time as `tracked_since`.

### 2026-10-25 — Task: Rewrite the installed instructions to write knowledge, then report it

**What was done**: The knowledge-trigger standing rule (full and short forms), memory-context, the plan discovery and walkthrough steps, the implement update_changelog and finished steps, the orchestrated-stop partial and both epic orchestrator skills now tell agents to write durable knowledge through `spek-knowledge` without asking and then tell the user which entry, its tier, store and path, and why. The partial `implement-knowledge-candidates` became `implement-knowledge-written` (a `## Knowledge written` list reported once at the finish). The discovery step passes `--spec {{plan_name}}` to `knowledge always-applied` and `"spec":"{{plan_name}}"` to `knowledge read`. Phrase tests moved to the new wording, and a new `templates/knowledge_capture_wording_test.go` bans the old offer phrases across agents/, steps/plan, steps/implement, partials/ and both epic skills.

**Deviations**: The goal check's first attempt found memory-context did not name the entry, store, path and reason; it now does, and the short rule says "which entry, where (tier, store, path) and why". The plan-epic skill test needed no change (its allowlist did not cover the replaced sentence). The harbor plan suite and the legacy AGENTS.md fixtures were left untouched.

**Files changed**:
- `spektacular: templates/agents/knowledge-trigger.md`
- `spektacular: templates/agents/short/knowledge-trigger.md`
- `spektacular: templates/agents/memory-context.md`
- `spektacular: templates/steps/plan/02-discovery.md`
- `spektacular: templates/steps/plan/18-walkthrough.md`
- `spektacular: templates/steps/plan/19-finished.md`
- `spektacular: templates/steps/implement/07-update_changelog.md`
- `spektacular: templates/steps/implement/12-finished.md`
- `spektacular: templates/partials/implement-knowledge-written.md` (renamed from `implement-knowledge-candidates.md`)
- `spektacular: templates/partials/orchestrated-stop.md`
- `spektacular: templates/skills/workflows/spek-plan-epic/SKILL.md`
- `spektacular: templates/skills/workflows/spek-implement-epic/SKILL.md`
- `spektacular: internal/agent/standing_rules_test.go`
- `spektacular: internal/steps/plan/steps_test.go`
- `spektacular: internal/steps/implement/steps_test.go`
- `spektacular: templates/implement_epic_skill_test.go`
- `spektacular: templates/knowledge_capture_wording_test.go`

**Discoveries**: Until the next task lands, the `spek-knowledge` skill these instructions hand off to still gates contribute/update on confirmation (the reviewer flagged this); the skill task removes that gate. This repository's own installed AGENTS.md and guidance keep the old wording until the maintainer reinstalls between workflows.

### 2026-10-25 — Task: Make the knowledge skill write directly and drive prune

**What was done**: The `spek-knowledge` skill's contribute and update intents now write straight away and then tell the user the tier, store, path, tags and reason (update also summarises the change). The maintenance review became `# Intent: prune`, which runs `knowledge prune` (or `--dry-run` as the preview), follows each returned instruction, answers with `knowledge prune verdict`, presents the final report, and keeps the report-only category-drift check. Decline handling now covers only the audit, the one remaining confirmation gate, whose per-entry flow is unchanged. `instruction_surface_test.go` was rewritten to pin the new wording, the prune intent and its slicing, and checks every `knowledge <verb>` the skill names against the registered subcommands (now including `usage` and `prune`).

**Deviations**: None.

**Files changed**:
- `spektacular: templates/skills/workflows/spek-knowledge/SKILL.md`
- `spektacular: internal/agent/instruction_surface_test.go`

**Discoveries**: `internal/agent/instruction_surface_test.go` keeps a hand-maintained `knowledgeSubcommands` map (cmd imports internal/agent, so it cannot be read from cmd); any new knowledge subcommand must be added there or skill invocations of it fail the test.

### 2026-10-25 — Task: Drop the obsolete knowledge-offer line from the end-to-end suites

**What was done**: Removed "If you are offered to save something to the knowledge base, decline." from the epic, implement-epic and goal-check Harbor suite instructions; their headless "never end your turn waiting for a reply" guidance is unchanged.

**Deviations**: None.

**Files changed**:
- `spektacular: tests/harbor/epic-workflow/instruction.md`
- `spektacular: tests/harbor/implement-epic-workflow/instruction.md`
- `spektacular: tests/harbor/goal-check-workflow/instruction.md`

**Discoveries**: None.

### 2026-10-25 — Task: Update the README and the maintainer knowledge-base doc

**What was done**: The README's knowledge CLI list gained `knowledge usage` and `knowledge prune`, its "Capturing knowledge" paragraph now describes writing without asking and reporting, the usage index and prune, and the project config example shows the optional `knowledge.prune.max_age_days`. `docs/knowledge-base.md` describes write-then-report capture, replaces the maintenance review with "### Pruning the knowledge base" (two passes, the tracking clock, verdicts, the unmet-standard rule, evidence, verdict checks, dry run, resume and `--force`), adds "## The usage index", and extends the command table with `usage`, `prune`, `prune verdict` and the `--spec`/`"spec"` attribution options.

**Deviations**: The goal check's first attempt found the tags passage still said entries wait for confirmation; it now says the skill writes without asking and reports the tags, and the tag-audit passage says the audit is the one flow that still asks (a spec Non-Goal).

**Files changed**:
- `spektacular: README.md`
- `spektacular: docs/knowledge-base.md`

**Discoveries**: None.

### 2026-10-25 — Task: Document self-written knowledge, the usage index and prune on the website

**What was done**: The website's knowledge-base page now says agents write entries themselves and report them, gains "How usage is recorded" (surface) and "Pruning the knowledge base" bands after "The lifecycle of an entry", keeping the surface alternation, documents `knowledge.prune.max_age_days` in its Configuration band, and the tag passage says tags are chosen from the vocabulary and reported. The configuration page adds `prune.max_age_days: 90` to the example and documents the key under the project `knowledge` ConfigKey.

**Deviations**: The guidance file list on the configuration page carries file names only, so it needed no wording change. Ran `npm ci` in the docs worktree to build.

**Files changed**:
- `docs: src/pages/knowledge-base.mdx`
- `docs: src/pages/configuration.mdx`

**Discoveries**: None.
