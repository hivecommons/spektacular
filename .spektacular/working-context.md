# Working context: 000060_epics-and-seeded-specs

## Problem and motivation

- GitHub #55: oversized requests produce specs with acceptance criteria too thin to build from;
  implementing agents fill the gaps with assumptions. Offer to split into an epic.
- GitHub #56: teams already write work up as issues/epics/design docs. Starting a spec from one only
  works with careful phrasing; nothing records where content came from.
- The spec is built to the design document `design/epics-and-seeded-specs.md` (source `design`,
  draft, committed by the user). The spec should reference it, not copy it.

## Decisions from the design conversation (this session)

- **One standalone spec, existing method.** User: "we should follow the existing method until this
  spec has been implemented" — no epic/split used to specify this work itself.
- **Split triggers** (now in the design, `### Split triggers`): a gate (agent must be able to name
  ≥2 specs, each with its own independently verifiable AC), strong signals (any one), weak signals
  (two or more), counter-signals, and "supporting work never counts".
  - User's concern that prompted the supporting-work rule: "if I created an update which would also
    update the docs, this would automatically create an epic?" → No. Code + its docs is one spec.
    Surfaces/repos removed as signals; they matter only via independence.
  - Nothing is ever automatic: always an offer; re-offer after decline only on a new strong signal
    or new independent requirement group.
- **Seeding is a prompt/skill concern, not a CLI feature.** User: "I don't really see why we need
  cli commands here, this is a prompt issue" and "we really need more an instruction in the skill
  on how to progress with this process".
  - Skill: recognise a source however phrased, fetch, check child items (offer split), propose a
    name, `spec new --data '{"name":…,"sources":[{"uri":…}]}'`.
  - Interview step: seed work files from the source, list gaps, ask only about gaps; later steps
    confirm drafts. Lives in the step so it survives resume.
  - CLI's only part: accept `sources` on `spec new`, stamp `retrieved_date`, write to frontmatter.
    `sources` must be added to closed `yamlShape` and `UpdateOptions`.
- **Tool-agnostic fetching.** User: "we should not be specific about what tool or process is used to
  fetch or read the issue, the agent should use its own tools … it could be a linear issue".
  Design names what to collect (title, body, discussion, child items, stable URI), not how.

## Alternatives rejected

- `spec new --from <issue-url>` / `--file <staged source>` + `source` in workflow state: rejected —
  CLI stays network-free and seeding needs no CLI surface beyond `sources`.
- Counting surfaces/repos as split signals: rejected — false positives on code+docs changes.

## Open items carried from the design

- Open decisions 1–7 in the design (closing an epic, naming after split, `status --spec-only`,
  splitting late, chaining beyond specify, detection before a spec exists, detection late).
- Deferred to the spec: detection sensitivity (`epic_split_threshold` favoured), `epic split` input
  format, source text snapshot/hash, posting back to source, implement wording, PR #65.

## Repos

- `spektacular` (tool, Go CLI) and `docs` (spektacular-website, Astro docs site, root
  /home/nicj/code/github.com/hivecommons/spektacular-website).

## User preferences seen this session

- Never commit unless explicitly asked (now in ~/.claude/CLAUDE.md). Project has `auto_commit: full`;
  user committed the design themselves before the workflow started.

## Interview answers (spec 000060)

- Scope: everything in the design, one spec, one release ("I am going to create a release which
  contains all this anyway"). Docs-site updates are in scope.
- Epic done: "once all the related specs milestones are done" → derived from plans.
- "We should be able to split an existing spec."
- Detection only when creating a spec; prompt "once the spec is done", "unless you explicitly
  instructed". AGENTS.md spec-trigger stays the same.
- Child items: present before split → epic + specs set up instead; added after an epic exists →
  suggest separate specs in that epic.
- Design updated to match (## Decisions); design ref recorded on the spec.
- Overview, requirements, ACs, constraints, technical approach all confirmed by the user ("ok") as drafted.
- Tech approach defaults confirmed: no source snapshot, no post-back, PR #65 separate.
- Non-goals to include: AGENTS.md spec-trigger unchanged; plans stay 1:1 with specs.
