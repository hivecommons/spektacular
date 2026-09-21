---
created_date: "2026-09-20"
document_status: final
closed_date: "2026-09-20"
---

# Design documents as a project-level artifact

Spektacular gained design documents as a fourth artifact class alongside specs, plans and
changelog records. A project declares one or more named design sources pointing at wherever it
already keeps its worked designs, reaches the documents there through the CLI the same way it
reaches every other artifact, and records references to them on the specs they bind. A spec can
now point at a settled API shape or user-facing flow instead of absorbing it.

## What was built

**Declaring and reaching design documents.** A new optional `design.sources` key in
`config.yaml` declares any number of named sources, each with a provider and a location,
reusing the same source type that already backs the project's shared knowledge stores. A
relative location resolves from the folder holding `config.yaml`; an absolute one is used as
written. Neither has to sit inside the project, which is the whole point: a source points at a
folder the team already keeps. A new `internal/design` package resolves each declaration to a
directory and refuses fast when one is not there, naming the source, the path it resolved to and
the base it resolved from. A `spektacular design` command family exposes it: `sources`, `list`
(across every source at once, or narrowed with `--source`), `read` (raw bytes to stdout) and
`write --from` (bytes stored unchanged). Nothing Spektacular writes adds frontmatter, reformats
content, or imposes a structure on a design document.

**References that survive.** A spec records its references in its own frontmatter, as a
`designs` list of source-and-path pairs, modelled in the artifact metadata schema and preserved
by its merge step. That preservation is what makes a reference durable: the spec workflow
commits by writing a freshly assembled body over the stored file, and an unmodelled key would be
dropped on that write. `design ref add`, `remove` and `list` manage them. Recording validates
the source against the project's declarations before touching the spec, so a reference naming an
undeclared source is refused with the declared names listed and the spec is left untouched. Any
number of specs may reference the same design, and the design holds no back-links, so it
outlives the feature that introduced it.

**Capture and consumption, as workflow behaviour.** A new managed `AGENTS.md` section gives
every agent the standing rule for recognising a settled API shape, user-facing flow, data format
or worked example, with a deliberately high three-part bar and the established accept, defer and
decline outcomes. The spec workflow's technical-approach step makes the offer at exactly the
point it otherwise tells the agent to compress a worked design into a one-line steer. On the
consuming side, the plan workflow must resolve and read every referenced design before it
designs anything, build on it rather than re-derive it, name each document and its source in the
finished plan's dependencies, and stop and report rather than plan around a reference it cannot
find. Nothing is ever written without explicit agreement.

**Room for a read-only provider, without building one.** The storage interface's read half was
named separately, so a design source holds a reader unconditionally and a writer only when its
provider can supply one. A provider that cannot write is therefore a named refusal rather than a
violated contract. No remote provider, provider registry or capability negotiation was built.

**Documentation.** The documentation site gained a concept page explaining what a design
document is, when to reach for one instead of a spec section, how designs relate to specs and
plans, and the full command surface with worked examples, reachable from the site navigation.
The configuration reference documents the new settings key in the same shape as the existing
shared-store key and links out to the concept page. The repository README gained the matching
key so the two descriptions of the same file do not drift.

## Why it matters

A spec that absorbs a worked design becomes unreadable, and the detail that made it precise is
exactly what the compression step throws away. Before this, a settled API shape had two
possible homes: inlined into the spec, where it swamped the requirements, or compressed into a
one-line steer the plan workflow was explicitly free to discard. Design detail raised in
conversation was routinely lost.

Three things follow from the shape chosen here. Specs stay readable, because the design is
referenced rather than copied. Implementation stays bound to the design that was agreed, because
the plan workflow is obliged to read every reference and build on it. And teams adopt the feature
without moving a file, because storage is declared rather than imposed: you name the places your
designs already live.

## Deviations from the plan

Five, all recorded per phase with their reasoning.

- **Every configuration refusal carries a next action**, including the unsupported-provider and
  empty-location cases where the equivalent knowledge validator returns a bare error. The
  phase's own criterion required a correction for all four cases, and the repo convention
  applies to every error path. The knowledge validator was left alone as out of scope, so the
  two diverge deliberately and the code says why.
- **`Set.Exists` was added** beyond the plan's five-method projection, because the reference
  resolution report needs a per-reference presence check and answering it through a read would
  pull a whole file off disk to produce a boolean.
- **A template-wide guard on `--data` examples** was added after I wrote three malformed
  payloads (opened with `'{`, closed with `}"`) that every existing contract test passed,
  because they assert phrases are present and never that a quoted command is well formed. An
  agent copying one would have issued a broken command.
- **The when-to-use rule of thumb is bullets, not the table the content outline specified**,
  because the site's body stylesheet carries no table styling and adding it would be a site-wide
  change outside this feature.
- **Two pre-existing drift repairs in the end-to-end suites**, made because this work's own
  criterion requires the suites to pass rather than record a failure as pre-existing: a
  status-lifecycle oracle still asserting a vocabulary replaced several features ago, and the
  spec suite's agent timeout, raised from 900 to 1200 seconds with the user's explicit agreement
  after a run missed by 0.4 percent.

## Verification

The full Go suite passes under `-shuffle=on -count=2`. The plan-workflow end-to-end suite passes
92 of 92, including a new oracle proving a real agent runs `design ref list` during discovery
unprompted. The spec-workflow suite passes 45 of 45. The documentation site builds and
type-checks with no errors and no warnings.

Four of the spec's five success metrics mix a testable mechanism with a claim about how people
behave over time; the mechanism half of each is automated and the remainder is written up as
manual procedures in the plan's test plan. That includes the one deliberate coverage decision:
the end-to-end spec suite does not assert the accept path of the capture offer, because doing so
would require scripting the user side of a conversation whose content determines whether the
offer fires, making the assertion a test of the script rather than of the behaviour.
