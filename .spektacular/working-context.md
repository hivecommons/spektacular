# Working context — spec 000072_knowledge-access-index-and-prune

## Where this came from
Raised by the user on 2026-10-24, mid-way through implementing 000071_harbor-child-agents. The trigger: the agent kept stopping to ask "want me to save this to the knowledge base?" (the Knowledge-Worthy Discovery Recognition rule in AGENTS.md, `templates/agents/knowledge-trigger.md`, plus the implement workflow's update_changelog Step 2b offer). The user deferred the spec until that implement run finished (it has; merged 3e6e62e).

## The user's words (verbatim)
"We need to make a change, you ask me if you should update your knowledge base, we need to write a spec where you just write your knowledge base, but we are going to keep metrics on when these are used. Both discovered in search and actually retrieved to include in a plan. This should be a json based index that is located in the knowledge base. The index should only ever be update by the spektacular knowledge tool. Then all knowledge base entries are updated automatically. We also introduce a knowledge prune command that uses the access index the date knowledge was created to determine what knowledge is still valid. Invalid entries will be pruned. The second pass of prune is to check the correctness of knowledge, each existing entry is read and checked for correctness. If invalid it is corrected or deleted."

## Requirements as understood so far (to confirm in the interview)
- Agents write knowledge entries without asking the user first (replaces the propose-then-confirm offer).
- A JSON access index, stored inside the knowledge base, records usage metrics per entry:
  - when an entry is discovered (returned by `knowledge search`);
  - when it is actually retrieved for inclusion in a plan (read and used).
- Only the `spektacular knowledge` CLI ever writes the index (agents never edit it).
- "Then all knowledge base entries are updated automatically" — meaning to clarify (likely: writes happen automatically / the index is maintained automatically on every CLI access).
- New `spektacular knowledge prune` command, two passes:
  1. Validity by usage and age: uses the access index and each entry's creation date to decide which entries are still valid; invalid ones are pruned.
  2. Correctness: each remaining entry is read and checked for correctness; an incorrect one is corrected or deleted.

## Relevant current state (facts, for drafting)
- CLI: `spektacular knowledge` has search, read, list, write, delete, tags, categories, sources, always-applied, conventions. No index or prune today.
- The `spek-knowledge` skill (lookup / contribute / update / audit / maintenance) enforces propose-then-confirm for every write and delete; its maintenance intent already classifies entries current / stale / incorrect / unverifiable, with per-entry confirmation.
- Binding knowledge rule (AGENTS.md "Historical Artifacts"): a knowledge entry that disagrees with the code states the target; the code is what is wrong. An entry is stale only if its subject no longer exists. Prune's correctness pass must respect this or the spec must consciously change it.
- Stores: project tier and repo tiers (e.g. repo `spektacular`), multiple stores; entries are Markdown with YAML frontmatter (tags).

## Open questions to raise in the interview
- Where exactly the index lives with multiple stores (one per store vs one per project) and how it is kept out of search results / git noise.
- What counts as "retrieved to include in a plan" (a `knowledge read`? a read during a plan workflow step?).
- Prune pass 1 thresholds: never discovered/retrieved within N days of creation? configurable?
- Prune pass 2: run by the CLI alone or by an agent skill (correctness needs judgement); dry run / report before deleting?
- Whether auto-write keeps any user visibility (e.g. a note of what was written).

## Interview answers (user, 2026-10-24)
- Retrieved = any `knowledge read` (+ always-applied/conventions loads), noting active workflow/spec.
- Prune pass 1: older than configurable age (default ~90 d) since creation AND never retrieved.
- Pass 2 keeps the binding knowledge rule (correct if wrong; delete if subject gone; report, never delete, an unmet target).
- Pass 2 runner: CLI drives, agent judges (step-like instructions), skill wraps.
- Prune is automatic with a report; `--dry-run` reports only.
- "Updated automatically" = agent writes need no approval; user sees a note.
- Docs website repo docs are in scope.

## Drafting (user said "just write the whole spec and commit it, I will check it later")
- All sections drafted without per-section confirmation, per the user's instruction.
- Fresh-eyes review findings applied without confirmation (same instruction): JSON index inside the knowledge base moved to Constraints (user-mandated); added constraints that prune's own reads don't count as retrievals and that entries aren't pruned for non-use until tracked for the full threshold (else the first prune would delete every pre-existing entry); report covers every entry with a verdict and evidence; preview AC made checkable; added ACs for parallel counting, pre-existing entries, prune's reads.
