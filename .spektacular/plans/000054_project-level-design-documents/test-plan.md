---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Test Plan: 000054_project-level-design-documents

Four of the spec's five success metrics mix a testable mechanism with a claim about how
people behave over time. The mechanism half of each is covered by automated tests and is
**not** repeated here. What follows is only the half no test can assert: a field observation
about specs, conversations, or a project's history. One metric, "teams adopt design documents
without relocating anything", is fully behavioural and needs no manual procedure at all.

Each procedure below is grounded in the shipped commands. None requires reading the code.

## 1. Specs reference a settled design instead of restating it

**Metric**: reviewing specs written after delivery, none restates a design its reference already
points at.

**Automated already**: that a spec can record a reference, and that recording one puts none of
the design's content into the spec body
(`cmd/design_ref_test.go`, `TestDesignRefAdd_LeavesTheSpecBodyUntouched`).

**What to measure by hand**: whether authors actually use it that way.

**How**: for each spec written after this feature shipped, in whichever repos are registered:

```bash
go run . spec file list
go run . design ref list --data '{"spec":"<spec name>"}'
```

For every spec whose `refs` array is non-empty, read the spec and each referenced design:

```bash
go run . spec file read <spec name>.md
go run . design read --data '{"source":"<source>","path":"<path>"}'
```

**Expected result**: no spec's body restates content the referenced design already carries. A
one-line pointer or summary is correct and expected; a paragraph reproducing the design's
shape, field list, or worked example is the failure this metric is watching for.

**Who and when**: whoever reviews specs, at spec review. Worth a deliberate sweep once about
ten specs have been written against declared design sources, since the metric is about a
pattern rather than any single spec.

## 2. An accepted capture offer leaves both a document and a reference

**Metric**: for every capture offer the user accepts, a design document exists in a declared
source and the spec references it.

**Automated already**: that the spec workflow's technical-approach step makes the offer, states
the accept, defer and decline outcomes, and directs both the write and the reference recording
(`internal/steps/spec/steps_test.go`). The end-to-end suites additionally prove a real agent
reaches the design commands and uses them without probing loops.

**What to measure by hand**: that the *accept* path actually produces both halves in a real
conversation. This is the deliberate decision recorded against the plan's second open question:
the end-to-end spec suite does **not** assert this. Doing so would mean seeding a design source
into that environment and scripting the user side of the conversation to accept an offer that
depends on the interview's content, which makes the assertion a test of the script rather than
of the behaviour, on a suite whose runtime already spans 446 to 903 seconds. It is verified by
hand instead, which this procedure is.

**How**: run a spec workflow in a project that declares at least one design source, and steer
the conversation until a settled API shape, user-facing flow, or data format is agreed. When the
agent offers to capture it, accept. Then:

```bash
go run . design list --source <source>
go run . design ref list --data '{"spec":"<spec name>"}'
go run . spec file read <spec name>.md
```

**Expected result**: all three hold. The design document appears in the source's listing; the
spec's `refs` array contains it with `"resolved": true`; and the spec body carries a pointer to
it rather than its content. Repeat once with a **decline**: nothing is written to any design
source, and the detail does not appear in the spec body either.

**Who and when**: whoever first uses the spec workflow on a feature with a settled design,
once. A second run covering the decline path is worth the few extra minutes, since declining
silently writing something is the more damaging failure.

## 3. No broken reference is first discovered during implementation

**Metric**: unresolved design references are reported while planning, and none is first
discovered during implementation.

**Automated already**: the reporting half in full. `design ref list` reports per-reference
resolution and the absolute location searched with an unresolved count
(`cmd/design_ref_test.go`), `design read` fails with `design_not_found` naming what it searched,
and the plan workflow's discovery step is asserted to carry both the resolve-and-read obligation
and the stop-on-unresolved rule (`internal/steps/plan/steps_test.go`). The plan-workflow harbor
suite confirms a real agent runs `design ref list` during discovery.

**What to measure by hand**: the word *first*. A test cannot observe that no broken reference
ever slipped past planning into implementation.

**How**: keep a note whenever a design reference is found broken, recording which workflow
surfaced it. The planning path reports it like this:

```bash
go run . design ref list --data '{"spec":"<spec name>"}'
```

and a non-zero `unresolved` count carries a `next_action` naming the corrective commands.

**Expected result**: every broken reference that occurs is surfaced by the plan workflow's
discovery step. A count of zero discovered-during-implementation is the pass condition; one or
more means either the discovery obligation is not firing or an agent is planning around it, and
the transcript should be checked for the `design ref list` call before blaming the prose.

**Who and when**: whoever runs the implement workflow, continuously, reviewed per release.

## 4. Designs accumulate references from more than one spec

**Metric**: at least some designs accumulate references from more than one spec.

**Automated already**: the capability. Two specs can reference the same design, each shows its
reference independently, and the design document is unchanged by either
(`cmd/design_ref_test.go`, `TestDesignRef_TwoSpecsShareOneDesignIndependently`).

**What to measure by hand**: whether it happens in a real project, which is a question about
months of history rather than about the code.

**How**: for each declared source, list its documents and count how many specs reference each:

```bash
go run . design list --source <source>
go run . spec file list
```

then, for each spec, `go run . design ref list --data '{"spec":"<spec name>"}'` and tally the
`{source, path}` pairs across all specs.

**Expected result**: at least one design document is referenced by two or more specs. Note that
failing this is not necessarily a defect in the feature: it may equally mean the project's
designs are genuinely feature-specific. Read it as a signal about whether design documents are
being written at a reusable altitude, not as a bug.

**Who and when**: whoever reviews the project's practices, once a body of specs exists. Not
meaningful before roughly a dozen specs have been written against declared design sources.
