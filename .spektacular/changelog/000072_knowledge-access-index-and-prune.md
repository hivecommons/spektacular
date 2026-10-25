---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# 000072_knowledge-access-index-and-prune: knowledge usage index and prune

## What was built

Coding agents now save durable knowledge to the project's knowledge base themselves and tell the user what they wrote (the entry, its tier, store and path, and why), instead of offering it and waiting for a decision. To keep a self-written knowledge base trustworthy, every knowledge store now keeps a usage index, and a new prune command clears out unused entries and has the agent check the rest.

- **Usage index (spektacular).** Each store keeps `.usage-index.json` at its root. It records each entry's creation date (set on first write and kept through rewrites), when tracking began, and two histories: discoveries (each `knowledge search` that returned it) and retrievals (each `knowledge read`, `always-applied` or `conventions` load), with retrievals naming the workflow and spec they were made under. Entries that predate the index are reconciled automatically, dated from git history or the file's modification time. All index changes go through a new atomic `store.Writer.Update`, backed by a shared portable file lock (`internal/filelock`, which the commit lock now delegates to), so parallel workflow runs lose no update. The index is hidden from list, search, tags and the always-applied reader, entry commands refuse its path, and a new read-only `knowledge usage` command reports it. A failure to record never fails the read or search; it adds a `usage_warning`.
- **Prune (spektacular).** `spektacular knowledge prune [--dry-run] [--force] [--tier] [--filter]` removes entries created and tracked for at least `knowledge.prune.max_age_days` (a new optional setting, default 90) and not retrieved within it, then hands the agent one instruction per remaining entry with its text inlined. The agent corrects, deletes or keeps each one and answers with `knowledge prune verdict`, which checks the claim against the store (deleted means gone, corrected means changed, current means untouched, and a dry run must change nothing) before handing out the next entry. The final report lists every entry with its verdict and evidence. An entry stating a standard the code has not yet met is kept as current; only an entry whose subject is gone is deleted.
- **Instructions (spektacular).** The installed standing rules, memory-context guidance, plan and implement workflow steps, orchestrated-stop partial, both epic orchestrator skills and the `spek-knowledge` skill now say "write it, then report it". The skill's maintenance review is replaced by a prune intent that drives the command loop; its tag audit is unchanged. The Harbor suites drop their obsolete "decline the offer" line.
- **Documentation (spektacular and docs).** The README, the maintainer knowledge-base doc and the website's knowledge-base and configuration pages describe writing without asking, the usage index, prune with its preview, and the age setting.

## Why it matters

Users are no longer interrupted to approve knowledge capture, and the report after every write and every prune is how they see the knowledge base change. The usage index shows which knowledge earns its place, and prune keeps an agent-written knowledge base from accumulating stale or wrong entries.

## Deviations from the plan

- `autocommit.LockError` stays its own type, mapped from the shared lock's error, so the commit lock's message and tests are unchanged; `filelock.AcquireWithBackoff` keeps the commit lock's own backoff knob.
- Reconcile drops a usage record only when its file no longer exists, so an entry hidden by an ignore rule but read by exact path keeps a stable record.
- The recording-failure test corrupts the index instead of holding its lock past a shortened timeout, so no store test hook was added.
- `knowledge prune` refuses (`knowledge_prune_in_progress`) when the requested mode differs from an unfinished run's, rather than silently resuming a real run when a preview was asked for, or the reverse.
- Goal-check retries tightened two pieces of wording: memory-context now names the entry, tier, store and reason, and the maintainer doc's tag passage no longer describes a confirmation gate (the tag audit, a spec Non-Goal, keeps its own).
