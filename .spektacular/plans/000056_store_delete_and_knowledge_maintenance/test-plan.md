---
created_date: "2026-09-21"
document_status: final
closed_date: "2026-09-21"
---

# Test plan: 000056_store_delete_and_knowledge_maintenance

Four of the spec's six success metrics have a manual half. Two are wholly manual, and two were
classified in `plan.md#testing-approach` as part behavioural and part manual — their behavioural
halves are in the suite and are not restated here. Only what a person has to do is below.

Two metrics are absent from this document on purpose, because they are fully covered by automated
tests: *deleting a design never leaves a spec pointing at a document that is not there*
(`TestDesignDelete_RefusedWhileSpecsStillReferenceTheDesign`), and *removal works unchanged when a
store is backed by something other than a local directory*
(`TestSet_DeleteTravelsThroughTheStoreInterface`, `TestSet_DeleteRoutesThroughTheStoreWriter`).

---

## 1. No agent reaches past the CLI to remove a managed file

**Behavioural half, already automated** — `TestEmbeddedTemplatesNeverRemoveManagedFilesDirectly` and
`TestRenderedSkillsNeverRemoveManagedFilesDirectly` sweep every template and rendered skill for an
instruction to remove a managed file with a raw file operation, and
`TestRenderedStoreAccessSectionForbidsRawRemoval` asserts the standing rule names removal as a CLI
verb. Those prove the *instructions* are right. They cannot prove an agent then behaves.

**What to measure**: across real sessions, an agent that decides to remove a knowledge entry or a
design document reaches for `knowledge delete` / `design delete` rather than `rm`, the `Write`
tool, or any other direct file operation.

**How**:

1. Work normally in a Spektacular project for a stretch that includes at least three removals of
   stored knowledge or design documents. The knowledge-base maintenance review in
   `spek-knowledge` is the natural way to generate them.
2. For each removal, check the session transcript for what the agent actually invoked.
3. Also watch for the negative case: an agent that hits a refusal (an undeclared store, a
   referenced design, a category description) and then works around it with a file tool instead of
   acting on the next action.

**Expected result**: every removal goes through `spektacular knowledge delete` or `spektacular
design delete`. Zero raw file removals of anything under a store directory. A refusal is followed by
the step the refusal named, or by the agent asking the user — never by a file tool.

**Who / when**: whoever is dogfooding the tool, over the first few weeks of normal use after this
lands. A single counter-example is worth investigating rather than dismissing: it most likely means
an instruction surface somewhere still reads as permitting it, which is a bug the sweep did not
catch.

---

## 2. The refusal for a referenced design is acted on rather than worked around

**Behavioural half, already automated** — the refusal is asserted to name every referencing spec and
carry a runnable `design ref remove` per spec, and the clear-then-retry sequence is asserted to
succeed with `unresolved: 0` afterwards, so the recovery path provably exists.

**What to measure**: an agent meeting that refusal follows the path it names, rather than deleting
the file directly or abandoning the removal.

**How**:

1. In a project holding at least one authored design referenced by two or more specs, ask an agent
   to remove that design. (Confirm the setup first with `spektacular design list`, whose `specs`
   field lists the referencing specs.)
2. Observe what it does next without prompting it.

**Expected result**: it reads the refusal, runs `design ref remove` once per named spec, retries the
delete, and reports what it did. It does not reach for a file tool, does not silently give up, and
does not clear references the user did not ask it to clear without saying so.

**Who / when**: once, deliberately, shortly after this lands, and then opportunistically whenever a
real referenced design needs removing. Note this project currently holds **no design documents at
all**, so the first run needs a fixture set up by hand.

---

## 3. A knowledge base stays accurate over time rather than only growing

**Wholly manual.** This is a property of repeated use over months, not of any single run, and no
test can stand in for it.

**What to measure**: the knowledge base's entry count and its accuracy both move, rather than the
count only rising.

**How**:

1. Record a baseline now: `spektacular knowledge list --tier all | grep -c '"path"'` for the total,
   and note the count per store.
2. Run a maintenance review (`spek-knowledge`, asking whether the knowledge base is still accurate)
   at a regular interval — monthly is a reasonable starting cadence.
3. After each review, record the new total, how many entries were removed, how many corrected, and
   how many were classified `current` despite the code disagreeing with them.

**Expected result**: over several months, entries are removed and corrected as well as added; the
total does not rise monotonically. The third figure matters most and should be non-zero: entries
classified `current` while the code disagrees are the ones stating a target the code has not met,
and a review that never finds any is probably misclassifying them as stale.

**Who / when**: the project maintainer, at whatever interval the knowledge base actually changes.

---

## 4. A maintenance review's staleness findings hold up when spot-checked

**Partly guarded already** — the classification rule, its worked failure mode, and the
evidence requirement are pinned as phrase assertions on the rendered skill
(`TestRenderedSpekKnowledgeMaintenanceCarriesClassificationRule`,
`TestRenderedSpekKnowledgeMaintenanceRequiresEvidence`). Those guarantee the instruction is present.
Whether a real review's findings survive scrutiny can only be judged by running one.

**What to measure**: findings a maintenance review reports as `stale` or `incorrect` are correct
when checked against the code, and the evidence it cites is real.

**How**:

1. Run a maintenance review over one store, e.g. this repo's own:
   `spek-knowledge`, asking it to review the `spektacular` knowledge base for accuracy.
2. For every entry it reports `stale` or `incorrect`, check the evidence it cited yourself: open the
   file, run the command, or look for the behaviour it says changed.
3. Separately, check the entries it reported `current`. Pick any entry that states a standard the
   code does not currently meet — this repo has several, including the store-access convention — and
   confirm it was **not** proposed for removal or correction on that basis.
4. Check its category-description drift report against reality with
   `diff <(spektacular knowledge read --data '{"tier":"repo","name":"<store>","path":"<category>/README.md"}') <(...)`
   or simply by reading the description and comparing it to `spektacular knowledge categories`.

**Expected result**: every stale or incorrect verdict names a specific file, command or behaviour,
and that evidence checks out. No entry is called stale merely because the code disagrees with it.
No finding rests on "looks old". A reported drifted description genuinely differs from the registry,
and running `spektacular init <agent>` brings it back into line.

**Failure to watch for specifically**: the review proposing removal of an entry that states a target
the code has not met. That is the failure mode the whole classification rule exists to prevent, and
it is the one outcome that would make this feature actively harmful. If it happens, the rule needs
sharpening in `templates/skills/workflows/spek-knowledge/SKILL.md`, not the entry deleting.

**Who / when**: the project maintainer, on the first real maintenance review after this lands, and
again the first time a review is run by a different agent or model, since the behaviour is prose and
its reliability is model-dependent.
