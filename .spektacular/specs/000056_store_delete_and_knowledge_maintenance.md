---
created_date: "2026-09-21"
document_status: final
closed_date: "2026-09-21"
---

# Feature: 000056_store_delete_and_knowledge_maintenance

## Overview

Knowledge entries and design documents are the only two of Spektacular's five kinds of stored
document that cannot be deleted, so an agent that finds an obsolete entry has to go around the
tool and remove the file itself — a workaround that stops working as soon as documents live
anywhere but a local folder. This feature lets both be removed properly, refusing to remove a
design that specs still reference so the inconsistency becomes an explicit cleanup rather than a
silent break, and builds on it the capability that gap was blocking: maintaining a knowledge base
by judging whether an entry is still true, not merely whether it is well labelled. It also closes
three related defects in how entries are retrieved, labelled and described.

## Requirements

- [x] **A knowledge entry can be deleted**
      An agent can remove a knowledge entry by naming the store it lives in and its path within
      that store.

- [x] **A design document can be deleted**
      An agent can remove a design document by naming its declared source and its path within that
      source.

- [x] **Deleting a design a spec still references is refused**
      The refusal names every spec that references the document and tells the caller how to clear
      those references first, so the reference and the document never disagree.

- [x] **A design nothing references deletes cleanly**
      A document with no recorded references is removed without further conditions, whether or not
      Spektacular authored it.

- [x] **Deleting a document that is not there succeeds and changes nothing**
      When the store or source is one the project declares but holds no document at that path,
      removal reports success rather than an error, in both stores, so a maintenance pass that
      retries is safe.

- [x] **Deleting from a store or source the project does not declare is refused**
      Naming a store or source the project has not declared is refused by name, and the refusal
      lists the names that could have been used. This is the one addressing failure that is an
      error; a path that simply holds nothing is not.

- [x] **Deleting a generated category description is refused**
      A category's own description is not knowledge and nothing restores it once removed, so the
      attempt is refused and the refusal says how to regenerate it instead.

- [x] **An agent can judge whether a knowledge entry is still true**
      An agent can review entries for correctness rather than only for labelling, classifying each
      as current, stale, incorrect or unverifiable.

- [x] **A maintenance review states the evidence for every claim it makes**
      An entry reported as stale or incorrect is reported with the specific thing that changed, so
      the finding can be checked rather than taken on trust.

- [x] **A maintenance review distinguishes an unmet target from an obsolete entry**
      An entry that states a standard the code has not yet met is reported as current rather than
      stale.

- [x] **Agreement to one entry's outcome never applies to another**
      Each entry in a maintenance review is its own decision, so agreeing to one change does not
      carry to the next and declining one does not end the review.

- [x] **Generated category descriptions stay out of retrieval**
      A category's own description is never returned by a search and never included in the
      material an agent receives on every task, so it cannot compete with the entries it
      describes.

- [x] **Labels that cannot be retrieved are not silently accepted**
      Writing an entry whose labels will never be reachable by a search tells the caller so,
      rather than storing them as dead weight.

- [x] **Category descriptions that have drifted are reported**
      A category description that no longer matches the project's current definition of that
      category is surfaced, naming where it is and how to bring it back into line.

- [x] **The documentation site explains how to remove an entry**
      The knowledge and design pages describe removal, including that a referenced design is
      refused, and distinguish removing a design from removing a reference to one.

## Constraints

- Deletion must work through the storage abstraction both subsystems already use, not against the
  local filesystem, so it keeps working for a source backed by something other than a local
  directory.
- Removal must be reachable only through the CLI. No instruction, skill or workflow may tell an
  agent to delete a managed file with its own file tools.
- The two deletion commands must use the addressing their own store already uses, and must not
  introduce a new way of naming a document.
- A design document and the specs referencing it must never be left disagreeing. No outcome of a
  deletion may leave a spec referencing a document that is no longer there.
- Deleting a referenced design must not modify any spec. Clearing references stays a separate,
  explicit act by the caller.
- Every refusal must name what went wrong and give a runnable next step, as every other refusal in
  these command families already does.
- The existing exclusion of always-applied categories from search must not be weakened. Reporting
  its consequence to a caller is in scope; changing it is not.
- Maintenance must not treat an entry as obsolete because the code disagrees with it. An entry
  states the target and the code is what has yet to meet it.
- Maintenance must not write or remove anything without explicit agreement for that specific
  entry.
- Maintenance must add no bulk operation. Every change it makes is to one named entry.
- The documentation site must build and type-check with no errors and no warnings.
- The behaviour of the three stores that can already delete must not change.

## Acceptance Criteria

- [x] **A knowledge entry is gone after deletion**
      Deleting an entry removes it, and a subsequent listing and search of that store no longer
      return it.

- [x] **A design document is gone after deletion**
      Deleting an unreferenced document removes it, and a subsequent listing of that source no
      longer returns it.

- [x] **A referenced design survives the attempt**
      Deleting a design that a spec references is refused, the document is still there afterwards
      unchanged, and no spec's references are altered.

- [x] **The refusal names the specs to clear**
      The refusal for a referenced design names every spec referencing it and gives a runnable step
      to remove those references.

- [x] **Clearing the references makes the delete succeed**
      After the referencing specs drop their references, deleting the same document succeeds.

- [x] **A design the project already had deletes like any other**
      A document carrying no lifecycle record and no references is removed without special
      conditions.

- [x] **Deleting twice is not an error**
      Issuing the same deletion a second time reports success and changes nothing, in both stores.

- [x] **A category description cannot be deleted**
      Attempting to remove a generated category description is refused, the file is still there
      afterwards, and the refusal says how to regenerate it.

- [x] **An undeclared store or source is refused by name**
      Deleting from a store or source the project does not declare is refused, nothing is removed,
      and the refusal lists the names that were available.

- [x] **A maintenance review classifies every entry it reads**
      Running maintenance over a store returns every entry in scope with a verdict of current,
      stale, incorrect or unverifiable.

- [x] **A stale verdict names what changed**
      Every entry reported as stale or incorrect names the specific file, command or behaviour
      that makes it so.

- [x] **An unmet standard is reported as current**
      An entry stating a rule the code does not yet satisfy is classified current, and is neither
      proposed for deletion nor for correction on that basis alone.

- [x] **Maintenance writes nothing without per-entry agreement**
      Declining one entry leaves it untouched and does not stop the review, and agreeing to one
      entry never changes another.

- [x] **A category description never appears in retrieval**
      Searching a store returns no category description, and the material returned to an agent on
      every task contains none.

- [x] **Unreachable labels are reported when set**
      Writing an entry whose labels a search will never reach reports that fact to the caller.

- [x] **Drifted category descriptions are listed with their remedy**
      A category description differing from the project's current definition is reported, naming
      where it is and how to bring it back into line; one matching the definition is not reported.

- [x] **The site documents removal for both stores**
      The knowledge and design documentation pages show how to remove an entry, state that a
      referenced design is refused, and distinguish that from removing a reference.

## Technical Approach

- The storage layer already exposes deletion, and every backend that can write will be able to
  delete, so this is expected to be command-layer work rather than new storage plumbing.
- The three stores that already delete get the verb from shared command machinery. That machinery
  is not the right home for these two, for reasons unrelated to deletion, so expect two commands
  that compose the existing pieces rather than a fourth registration of the shared factory.
- Refusing a referenced design needs the set of specs referencing it. The system already records
  that relationship on the design itself, so the check is expected to be a read rather than a scan
  of the spec store.
- A design the project did not author carries no record of referencing specs, so that check has
  nothing to find for those documents.
- Keeping generated category descriptions out of retrieval looks like one exclusion applied in the
  place that walks a store, covering both the search surface and the always-applied payload,
  rather than two separate fixes.
- Maintenance belongs in the existing knowledge skill as a further intent alongside the ones it
  already has, following the same propose-then-confirm shape rather than inventing a second one.
- Maintenance is prose-driven judgement rather than a command of its own; expect it to compose
  commands that already exist plus the new deletion verb.
- The riskiest part is the classification rule that separates an obsolete entry from one stating an
  unmet target. Getting it backwards deletes the entries most worth keeping, so it deserves the
  sharpest treatment in the plan.

## Success Metrics

- No agent reaches past the CLI to remove a managed file. Any instance of a stale entry being
  deleted with a raw file operation after this ships is a failure of the feature, not of the
  agent.
- A knowledge base stays accurate over time rather than only growing. Entries that have stopped
  being true are found and removed or corrected during maintenance, instead of accumulating until
  someone notices.
- A maintenance review's staleness findings hold up when checked. A reviewer spot-checking the
  evidence for a handful of findings should not find one that misreads an unmet target as an
  obsolete entry.
- Deleting a design never leaves a spec pointing at a document that is not there. Reference
  resolution reports no unresolved references caused by a deletion.
- The refusal for a referenced design is acted on rather than worked around. An agent that hits it
  clears the references and retries, rather than falling back to removing the file by hand.
- Removal works unchanged when a store is backed by something other than a local directory, with
  no part of the feature needing to be revisited for it.

## Non-Goals

- Cascading deletion. An explicit opt-in that clears a design's references for you may be worth
  revisiting later; it is not part of this.
- Bulk, recursive or cross-store operations. Each deletion and each maintenance run names one
  document in one store; removing a whole category, store or source in a single call is out of
  scope.
- Any undo, trash or soft-delete. Deletion is immediate and recovery is whatever version control
  already provides.
- Documenting the removal commands the spec, plan and changelog stores already have. They are
  currently absent from the documentation site, which is a real gap, but closing it is not part of
  this work.
- Automatic repair of a drifted category description. Drift is detected and reported; bringing a
  store back into line stays a separate, deliberate act.
- Maintenance of anything other than knowledge entries. Reviewing specs, plans, changelog records
  or design documents for staleness is out of scope, as is any automated check that a design still
  matches the code.
