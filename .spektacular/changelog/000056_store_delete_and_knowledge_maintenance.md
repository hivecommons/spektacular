---
created_date: "2026-09-21"
document_status: final
closed_date: "2026-09-21"
---

# Store removal and knowledge maintenance

Knowledge entries and design documents can now be removed through Spektacular's own CLI, and the
knowledge skill can review a knowledge base for whether its entries are still *true* rather than
merely whether they are well labelled.

## Why it matters

Two of Spektacular's five kinds of stored document could not be removed. `spec file delete`, `plan
file delete` and `changelog file delete` all worked; `knowledge` and `design` never gained the verb,
because their command families were hand-written rather than built from the shared factory. Nobody
decided entries should be unremovable — it was an assembly artifact.

The consequence was not cosmetic. An agent running a knowledge-base cleanup and finding an obsolete
entry had no supported way to remove it, so it reached past the tool and deleted the file itself.
That works only while a store happens to be a local directory, and it is exactly the behaviour the
project's standing guidance forbids. The gap also blocked the capability that motivated finding it:
a maintenance review cannot propose removing a stale entry if nothing can remove one.

## What was built

**Removal, on the two families that lacked it.** `knowledge delete` addresses an entry by tier,
store name and path, exactly as read and write do. `design delete` addresses a document by declared
source and path, exactly as its siblings do. Both compose a new `Delete` on their own domain set
over the storage layer's existing `Delete`, so an undeclared store or source is refused with the
names that were available, and removing something already absent reports success and changes
nothing. Both report `deleted` so a caller can tell a real removal from a no-op. Neither was folded
into the shared store-file factory, whose lifecycle-stamping write path a design document must not
acquire.

**Three refusals, each placed in the layer holding the facts its remedy is built from.** A design
one or more specs still reference is not removed: the refusal names every referencing spec with a
runnable `design ref remove` for each, and writes nothing in either direction, so a spec can never
be left pointing at a document that is not there. A category's generated description is not removed,
because it is rendered from the project's definition of that category and nothing would restore it;
the refusal names the command that regenerates it. And an undeclared store or source is refused by
the domain set, which is the only thing that knows the declared names.

**A remedy that actually works.** The repo footprint scaffolder previously wrote a category
description only when it was absent, so a description that had drifted from the registry could be
repaired by no command at all. It now brings a drifted one back into line while leaving a matching
one byte-identical, which is what makes `init` a remedy the refusals above can honestly name.

**Category descriptions out of retrieval.** A category's own `README.md` documents the category
rather than contributing to it, and every one shares the same vocabulary, so any query resembling
those words returned them ahead of real entries. They are now excluded from search results and from
the payload an agent receives on every task, while staying fully visible to a listing and a read.
Measured against this project's own knowledge base: a search that returned eight hits, every one a
category description, now returns three, none of them a description; the always-applied payload
carried two descriptions per store on every request and now carries none.

**Labels that could never be reached are no longer accepted in silence.** Always-applied categories
are deliberately excluded from search and from the label vocabulary, which makes labels on their
entries inert. Writing such an entry still writes it, and now also reports which labels are
unreachable and what to do about it.

**A maintenance review.** The `spek-knowledge` skill gained a fifth intent. It classifies each entry
as current, stale, incorrect or unverifiable, requires every stale or incorrect verdict to name the
specific file, command or behaviour that changed, reports drifted category descriptions without
repairing them, and proposes and confirms one entry at a time. Its load-bearing rule is the one that
decides whether such a review helps or harms: an entry stating a standard the code has not yet met
is **current**, not stale, because an entry states the target and the code is what has yet to meet
it. Only an entry whose subject no longer exists is stale.

**The standing rule now names removal.** The managed agent guidance named read and write and was
silent on removal, which is the silence that made reaching for a raw file operation look
permissible. It names removal explicitly, and a corpus-wide sweep asserts that nothing anywhere in
the agent-facing material instructs removing a managed file with a raw file operation.

**Documentation.** The knowledge base and design documents pages on the site describe removal, and
the design page states plainly that removing a design is not the same as removing a reference to
one — the trap anyone hits today, because the reference command reads like the missing delete and
appears to succeed.

## Deviations from the plan

**One correction to a refusal, caught by smoke-testing rather than by review.** The
category-description refusal first named `spektacular init` as its next action. `init` takes the
agent as a required argument, so that step does not run — the precise failure the project's
error-remediation convention exists to prevent, in the one refusal whose whole job is naming a
working remedy. It now names the agent the project records, and a test holds it.

**One addition beyond the letter of the plan.** `internal/project/init.go` also hard-coded the
description filename; it now uses the registry constant, so the name genuinely has one home, which
was the stated point of naming it there.

**One test-placement improvement.** The `UnreachableTags` unit tests live beside the function in
`category_test.go` rather than in `set_test.go` as the plan suggested, matching the project's
co-location convention.

Everything else was implemented as planned. The three pre-existing delete verbs are unchanged, and
characterisation tests were added first to prove it.

## Worth knowing afterwards

- **`migrate` does not regenerate skills during development.** It reports `up_to_date` when the
  installed and current versions match, so an in-place template edit is not picked up. `init
  <agent>` is what re-renders them.
- **`changelog file delete` has no `--repo` flag at all**, so a repo-routed changelog record cannot
  be removed through the CLI. This predates the change and is now pinned by a test as the recorded
  baseline. Fixing it means adding the flag, not rewiring an existing one.
- **A sweep for raw file removals cannot ban a bare `rm `** — the word `confirm` ends in `rm`, so
  `confirm .spektacular/specs/…` contains `rm .spektacular/specs/`. Word-boundary anchoring, not
  just distinctive path literals, is what makes such a sweep safe.
- This project holds **no design documents at all**, so every design fixture in this work is
  synthetic and the referenced-design refusal has no live instance yet.
