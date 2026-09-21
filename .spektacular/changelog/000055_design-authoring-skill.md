---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Design authoring: helping a design get written, not just stored

## What was built

Spektacular could already store a design document and reference it from a spec, but it assumed the
design already existed and only needed writing down. This feature adds the ability to help work one
out, and gives the designs Spektacular writes a life of their own.

**A guided way to author a design.** A new `spek-design` skill covers the four ways a design enters
a project: authoring one that does not exist yet through an interview, bringing in one the user
already has, revising one that exists, and recording a reference to one already stored. The
authoring branch runs the same Flipped Interaction interview the spec workflow uses, with a stated
goal, adaptive questions rather than a script, and an explicit stopping condition: stop once a
further answer would not change the document. It is a static playbook rather than a fifth
state-machine workflow, deliberately, because a workflow could not run inside a spec conversation
and that is exactly when design talk happens.

**An agent that notices.** The standing instruction in the project's agent file was restructured.
It previously conflated noticing with qualifying, so an agent only looked once a detail had already
settled, by which point the conversation had usually moved on. It now opens with an alert that
applies during any conversation working out how something will behave, states plainly that being
alert is not the same as offering, and keeps the existing three-part bar as the gate on offering.
It also covers the three entry cases it used to walk past: a design the user already has, one that
exists and is being changed, and design talk with no spec in sight. The spec workflow's
technical-approach step and the spec skill were brought into line so an agent gets the same answer
wherever it reads.

**Two kinds of design document, kept apart.** A new `design author` command stores a document with
the same lifecycle record every spec and plan carries: when it was captured, where it stands in the
four-value status vocabulary, which spec's conversation produced it, and which specs reference it.
The existing `design write` is unchanged and still stores bytes exactly as supplied, gaining one
guard: it refuses to overwrite a document Spektacular authored, because a verbatim overwrite would
silently strip that record and every back-link with it. The discriminator is the document itself,
so there is no registry to drift out of step.

**Back-links that can be trusted.** Recording a reference is now a change to two documents. The
spec gains the reference and an authored design gains the spec in its own record; removing one
removes both. The two writes cannot be made atomic, so the behaviour when the second fails is
defined rather than left to chance: the operation fails as a whole and the spec is put back exactly
as it was. If that restoration also fails, it is reported as its own distinct outcome naming both
files to repair, because the corrective action is completely different from a retry.

**Documentation that does not contradict the product.** The design documents page stated flatly
that Spektacular never adds frontmatter to a design document. That is still true of documents it
did not author and false of the ones it now does, so the claim was narrowed rather than appended
to, and the page gained sections on the interview and on what an authored design records.

## Why it matters

Design conversation is where a feature's real shape gets settled, and it was the thing most likely
to be lost. A worked design compressed into a one-line steer in a spec is gone; the spec reads
fine and the detail that would have made it buildable is not there. Catching that moment was left
entirely to the user asking.

Equally, a team that already keeps its own design documents had to get nothing new from this, and
that constraint shaped the whole approach. Their files are still stored exactly as supplied, still
gain no frontmatter, and are still untouched by referencing and de-referencing. The two classes of
document are genuinely different and the difference is visible from a listing.

## Deviations from the plan

Four, none of them scope changes.

**The phase 2.2 test seam needed a seam and a root-proof sabotage, not file permissions.** The
plan's first open question asked whether a write failure could be provoked through file
permissions, with a package-level `designSetFactory` as the pre-approved fallback. Neither
answer was right on its own.

A seam is required, but only for the rollback-failure case, and that is a property of the store
rather than of any platform: `FileStore.Write` is `os.MkdirAll` then `os.WriteFile`, and writing
an existing file needs permission on the file rather than its directory, so any state that fails
the rollback fails the identical first spec write too and the run never reaches the compensation.
The spec has to become unwritable between the two writes. The seam added is `writeBackLinkFn`
rather than a design-set factory: same shape and precedent, smaller surface, and the only one of
the two that can do that.

File permissions turned out not to work at all. They work locally and are ignored in CI, which
runs the suite as root inside a container, so a `chmod 0444` sabotage silently does nothing and
the write it was meant to block succeeds. This shipped broken and was caught by CI on the first
run after the merge. The sabotage is now to replace the target file with an empty directory:
`os.ReadFile` and `os.WriteFile` both fail with EISDIR on one, which is a kind-of-file error
rather than a permission check, so no uid is exempt. Verified as both uid 1000 and uid 0.

Five other tests in this repo handle the same problem by skipping when root. That was rejected
here, because it would make CI green by dropping coverage of the one failure mode the spec
raises to a constraint, in the environment where it matters most.

**Two refusal messages were worded more narrowly than the plan specified.** The
`design_frontmatter_not_authored` refusal does not repeat `metadata.Merge`'s underlying cause,
because that cause reads as a complaint about a missing `created_date` and invites an agent to add
one to the team's block, which is the opposite of the intended remediation. And
`design_authored_overwrite`'s next action leaves its `--data` payload unquoted inside the quoted
command, because the obvious phrasing nests single quotes inside single quotes and is not
copy-pasteable.

**Section shading on the documentation page was solved by inserting a pair.** The plan offered
either flipping every subsequent section's `surface` value or placing new sections so the existing
values still alternate. Adding two sections, plain then shaded, after a shaded one preserves the
entire downstream run, so no existing value changed.

**Phase 3.1 was done in one pass rather than two parallel agents**, as the plan's agent strategy
suggested. The template context was already loaded and splitting it would have added an
integration merge for two registry lines.

## Known follow-up

This repository's own installed agent artifacts under `.claude/` and `.bob/` are generated and are
now stale: they do not contain the new `spek-design` skill, and their copies of the spec skill
still carry the pre-change frontmatter sentence. Running `go run . init` brings them into line.
That was deliberately left undone, because installing files is an explicit user-initiated action
rather than something the implement workflow performs. No test depends on those copies; the
regression guard walks the embedded templates and renders through the install path into a
temporary directory.
