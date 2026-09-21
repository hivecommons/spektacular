---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Test Plan: 000055_design-authoring-skill

Two of the spec's six success metrics are fully covered by automated behavioural tests and do not
appear here:

- **Bringing in an existing design costs no rework** is guaranteed by
  `TestDesignWrite_ReplacesDocumentsSpektacularDidNotAuthor` (byte-exact round trip through the
  verbatim path, including a document carrying the team's own frontmatter) together with
  `TestDesignRef_ADesignWithNoLifecycleRecordIsLeftUntouched` (the file is byte-identical after an
  add and a remove).
- **Back-links can be trusted** is guaranteed by
  `TestDesignRef_BackLinksAgreeWithTheSpecsThatReferenceThem` (bidirectional consistency across a
  sequence of adds and removes over two specs and two designs) plus both failure paths in
  `TestDesignRef_BackLinkFailureLeavesNoDisagreementBehind`.

The four below are statements about real use, output quality, or user perception. No assertion can
express them, so each is paired here with a procedure and with the testable proxy that does exist.

## Setup, required before any procedure below

These procedures exercise the agent-facing surfaces, which are **generated artifacts**. This
repository's own installed copies under `.claude/` and `.bob/` are stale: they do not contain the
new `spek-design` skill, and `.claude/skills/spek-new/SKILL.md` and `.bob/skills/spek-new/SKILL.md`
still carry the pre-change sentence "with no frontmatter added and nothing reformatted" at line 58.

Before running anything here, reinstall them and restart the agent session so it picks up the new
instruction file and skills:

```bash
go run . init
git diff --stat .claude/ .bob/ AGENTS.md
```

Expected: `AGENTS.md`'s Design-Worthy Detail Recognition section is rewritten, a
`spek-design/SKILL.md` appears under each agent's skills directory, and `spek-new/SKILL.md` no
longer contains the old sentence. Confirm with:

```bash
grep -rn "with no frontmatter added and nothing reformatted" .claude/ .bob/
```

Expected: no matches.

**Who / when**: whoever is verifying this feature, once, before the procedures below. `init` is a
user-initiated action and was deliberately not run by the implement workflow.

## Metric 1: Design conversation produces a design rather than being lost

**What to measure.** Whether an agent, during an ordinary conversation, notices design-worthy
detail and offers to capture it without being asked. The failure this feature exists to fix is the
detail being compressed to a one-line steer and lost.

**How.** In a fresh agent session in a Spektacular project, hold a conversation that works out a
worked design without ever using the words "design document". Describe an API's shape or a data
format in enough detail that it is settled and worked, for example a retry policy with its
triggers, its attempt limit and its backoff. Do not prompt the agent to capture anything.

Run this three times, varying the entry case:

1. The design emerges in conversation with a spec workflow already running.
2. The design emerges with **no spec in existence**.
3. State that you **already have** the design written in a file, and say nothing further.

**Expected result.** In all three the agent offers, unprompted, before the conversation moves on,
and says what it would capture and which declared source it would write it to. In case 2 it does
not ask which spec to attach it to. In case 3 it offers to store the existing file rather than
proposing to interview you about it. Declining in any case leaves no file behind: confirm with
`go run . design list`.

**Proxy already covered.** That the instruction surface actually tells the agent to watch and
offer, and covers all three entry cases, is asserted by
`TestRenderedDesignTriggerSectionSeparatesAlertnessFromOffering` and
`TestRenderedDesignTriggerSectionNamesThreeEntryCases`.

**Who / when.** A maintainer, over the first weeks of real use. A single session is weak evidence;
the metric is about the rate at which these moments get caught.

## Metric 2: Users are helped to write designs they would not have written unaided

**What to measure.** Whether the guided interview produces a usable design document in a case where
the user would otherwise not have written one.

**How.** Accept the offer from Metric 1 case 1 or 2, or invoke the skill directly, and let the
interview run to its own stopping condition without steering it. Then read the result:

```bash
go run . design list
go run . design read --data '{"source":"<name>","path":"<path>"}'
```

**Expected result.** The interview asks adaptive questions that follow from your answers rather
than working through a fixed list, states when it has enough, and stops on its own. The document
that results is one you could hand to someone else to build from, and it carries a lifecycle block
with `created_date` and `document_status: draft`, plus `spec:` when the conversation belonged to a
spec.

**Proxy already covered.** That the skill states a goal and an explicit stopping condition is
asserted by `TestSpekDesignSkillCarriesItsContract`.

**Who / when.** Observed over real use. Whether a design was produced by interview or handed over
complete is not recorded anywhere and deliberately is not worth recording.

## Metric 3: The two classes of design document do not confuse users

**What to measure.** Whether a reader can tell, without reading the code, which documents
Spektacular changes and which it does not.

**How.** Two parts.

Documentation, run once after any change to the pages:

```bash
cd /home/nicj/code/github.com/jumppad-labs/spektacular-website
npm run build
npx astro check
grep -rn "never adds frontmatter\|with no frontmatter added" src/pages/
```

Expected: build succeeds; `astro check` reports **0 errors and 0 warnings** (one pre-existing hint
about `document.execCommand` in an unrelated component is not a warning and is not in scope); the
grep returns nothing. Then read the "What a design document is" and "What an authored design
records" sections and confirm they state the distinction plainly and do not contradict each other.

Behaviour, to confirm the docs describe what actually happens:

```bash
go run . design list
```

Expected: an authored design reports `created_date`, `document_status`, and `spec`/`specs` where
they apply; a design the project already had reports nothing but `source` and `path`.

**Expected result.** A reader who has only the site can predict which of their own files will be
touched. Ask someone who did not implement this to read the page and say what happens to a design
they wrote themselves; they should answer "nothing is added to it" without hedging.

**Who / when.** A maintainer at pre-release, plus one reader who was not involved in the work.

## Metric 4: Authored designs stay proportionate

**What to measure.** Whether an authored design is the size of the design rather than the size of
the conversation that produced it. The failure mode is a transcript.

**How.** After each authored design produced under Metric 2, compare the document against the
conversation:

```bash
go run . design read --data '{"source":"<name>","path":"<path>"}' | wc -l
```

**Expected result.** The document records decisions and not the exchange that produced them: no
"you said", no question-and-answer structure, no restating of options that were considered and
dropped unless the rejection is itself part of the design. A design settled in four or five
exchanges should not produce pages. There is no line-count threshold, deliberately, because the
right length is a property of the design; the pass condition is that a reader cannot reconstruct
the interview from the document.

**Proxy already covered.** That the skill states the stopping condition as a rule, and states that
the document records the decisions rather than the conversation, is asserted by
`TestSpekDesignSkillCarriesItsContract`.

**Who / when.** Whoever runs Metric 2, on the same documents, immediately afterwards.
