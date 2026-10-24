---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
epic: 000002_workflow-tracing-and-run-stats
---


# Feature: 000072_knowledge-access-index-and-prune

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Coding agents working in a Spektacular project will save what they learn to the project's knowledge base on their own, instead of stopping to ask the user first. To keep a self-written knowledge base trustworthy, Spektacular will record how often each piece of knowledge is found and actually used. A new prune command removes knowledge that is old and never used, then has the agent check every remaining piece against the project and correct or remove what is wrong. Users are no longer interrupted mid-task, and maintainers can see which knowledge earns its place.

<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [ ] **Agents write knowledge without asking**
  When an agent learns something durable while working, it must create or update the knowledge entry itself, without proposing it or asking the user first.
- [ ] **The user sees what was written**
  After an agent writes or updates a knowledge entry on its own, the user must be told which entry was written, where, and why, in the agent's normal reply.
- [ ] **Searches are recorded**
  Each time a knowledge search returns an entry, the knowledge base must record that the entry was discovered, and when.
- [ ] **Reads are recorded**
  Each time an entry is read, including when entries are loaded as always-applied rules or conventions, the knowledge base must record that the entry was retrieved, and when. When a workflow is running, the record must also name that workflow and its spec.
- [ ] **Usage is kept with the knowledge base**
  Every entry's discovery and retrieval history must be kept with the knowledge base itself, readable by tools, so it travels with the project.
- [ ] **Every entry has a creation date**
  Each knowledge entry must have a date it was created, recorded when it is first written and kept unchanged through later updates.
- [ ] **Prune removes old, unused knowledge**
  A prune command must remove every entry that is older than the configured age threshold and has not been retrieved in that time, even if searches have surfaced it.
- [ ] **Prune checks the remaining knowledge for correctness**
  After removing unused entries, the prune command must have the agent check each remaining entry against the project. An entry that describes its subject wrongly is corrected, and an entry whose subject no longer exists is deleted.
- [ ] **Unmet targets are reported as current**
  When the correctness check finds an entry stating a standard the project has not yet met, prune must keep it unchanged and report it as current.
- [ ] **Prune acts and reports**
  Prune must make its removals and corrections without asking, and then report every entry with its verdict — removed, corrected, deleted or current — and the evidence for every change.
- [ ] **Prune can be previewed**
  Prune must offer a preview mode that produces its report without changing any entry or any usage history.
- [ ] **Pruning keeps the usage history in step**
  An entry that prune removes or deletes must also lose its usage history, and a corrected entry must keep its usage history.
- [ ] **Documentation describes the new behaviour**
  The project's website documentation must describe that agents write knowledge without asking, the usage index, and the prune command with its preview mode and settings.

<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- The usage records must be a JSON index stored inside the knowledge base.
- The index must only ever be changed by Spektacular's own knowledge commands; agents and users must never edit it, and every knowledge entry is likewise reached only through those commands.
- The existing binding rule for knowledge stays: an entry that disagrees with the code states the target, and only an entry whose subject no longer exists counts as stale.
- The index must stay correct when several workflow runs read and search knowledge at the same time, as epic runs do in parallel.
- Prune's age threshold must be a project setting with a default, not a fixed value.
- Reads that prune itself makes, in a real run or a preview, must not count as retrievals.
- An entry must not be pruned for non-use until its usage has been tracked for at least the age threshold, so entries that predate the index are not all removed by the first prune.
- Existing knowledge entries written before this change must keep working: the change must not require users to rewrite them by hand.
- Changes to the instructions agents receive must be made in the source Spektacular generates them from, since generated copies are overwritten on the next initialisation.

<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [ ] **A learned fact is saved without a question**
  In a session where the agent discovers a durable fact, a new or updated knowledge entry exists afterwards, the transcript contains no question asking the user whether to save it, and the agent's reply names the entry it wrote.
- [ ] **A search is counted as a discovery**
  After a knowledge search that returns an entry, the index shows one more discovery for that entry, with the search's time.
- [ ] **A read is counted as a retrieval**
  After an entry is read, or loaded as an always-applied rule, the index shows one more retrieval for it with the read's time. A read made while a workflow runs names that workflow and its spec.
- [ ] **Parallel use is counted exactly**
  After several processes running at the same time each search for and read the same entry once, the index shows exactly that many additional discoveries and retrievals for it.
- [ ] **The index cannot be hand-edited through the tool**
  Writing to the index's location through the knowledge write command is refused, and the index is not listed or returned as a knowledge entry.
- [ ] **Creation dates survive updates**
  An entry written and then updated keeps the creation date of its first write.
- [ ] **Existing entries carry over**
  An entry written before this change has a creation date afterwards with no manual edit, can still be searched and read, and is not pruned for non-use until it has been tracked for the full age threshold.
- [ ] **Old, unused entries are pruned**
  With the age threshold set to 30 days, an entry created 40 days ago and never retrieved is removed by prune. An entry created 40 days ago and retrieved once is kept. An entry created 10 days ago and never retrieved is kept.
- [ ] **Surfaced but unread entries are still pruned**
  An old entry that searches discovered but that was never retrieved is removed by prune.
- [ ] **Prune's own reads are not usage**
  After prune, or a prune preview, reads every entry, no entry's retrieval count has changed because of it.
- [ ] **Wrong entries are corrected and gone subjects are deleted**
  In a project where one entry describes a command's flag wrongly and another describes a command that no longer exists, prune corrects the first and deletes the second, and its report gives the evidence for each.
- [ ] **An unmet standard survives prune**
  An entry stating a rule the code does not yet follow is still present, unchanged, after prune, and the report lists it as current.
- [ ] **Preview changes nothing**
  A prune preview lists the same age-based removals a real run would make and a verdict for every remaining entry, and afterwards every entry and the index are byte-for-byte unchanged.
- [ ] **The index stays consistent after prune**
  After prune, the index holds no record for any removed or deleted entry, and a corrected entry's discovery and retrieval counts are unchanged.
- [ ] **Agents are no longer told to ask**
  The instructions installed into a project tell agents to write durable knowledge themselves and report it, and no longer tell them to offer it and wait for the user's decision.
- [ ] **The website documents it**
  The website's knowledge documentation describes writing without asking, the usage index, the prune command, its preview mode and its age setting.

<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- Keep the usage index as one JSON file inside the knowledge base, updated by the knowledge commands themselves.
- Run prune's first pass entirely in the command, comparing each entry's creation date and tracked retrieval history against the configured age (default around 90 days).
- Drive prune's second pass the way the workflows already drive agents: the command hands the agent one instruction per remaining entry to check against the project, and the agent corrects or deletes through the existing knowledge commands. A knowledge skill wraps the whole flow.
- Replace the current "offer, then wait for the user's decision" knowledge-capture behaviour in the generated agent instructions and the knowledge skill with "write it, then tell the user". The knowledge skill's existing maintenance review, which classifies entries as current, stale, incorrect or unverifiable, is the starting point for the correctness pass.
- Entries written before this change have no recorded creation date; backfilling one from the store's version history is one option, left to the plan.
- Known risk: automatic writes and deletions remove the user's chance to catch a bad entry before it lands; the report after each write and each prune is the mitigation.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Zero "should I save this to the knowledge base?" questions to the user in a full run of Spektacular's end-to-end test suites after the change.
- 100% of knowledge searches and reads in a test run are reflected in the index, with no lost updates when workflow runs run in parallel.
- A prune preview on this project's own knowledge base completes and lists every entry with a verdict, and a real prune leaves no entry the report does not account for.
- Every entry that prune corrects or deletes in a test project carries evidence in the report that a reviewer can check without opening the code.

<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- Ranking search results by usage is out of scope; the index records usage but does not change how search orders results.
- Undoing a prune, or keeping an archive of pruned entries, is out of scope; recovery is through the project's version history.
- Running prune automatically on a schedule is out of scope; it runs when invoked.
- Changing the knowledge categories, the tag vocabulary or how stores are laid out is out of scope; the usage index is the only addition to a store.
- The knowledge skill's review of whether entries are tagged well is unchanged.
