# The Knowledge Base

Knowledge is the accumulated know-how a project draws on when planning — the rules
it commits to, the shared vocabulary it speaks, how the system is built, the traps
to avoid, what past work taught, and why decisions were made. It is a
**planning-time input**: the planning agent reads it while producing a plan, and
the relevant parts are written into the plan itself. The implement workflow then
consumes only the plan documents, so it never re-reads the knowledge base — the
plan is the contract.

This document describes how the knowledge base is organised and how it behaves, so
a maintainer new to the project can understand it without reading the source.

## The category model

Every knowledge entry belongs to a **category**, determined by the first segment of
its path within a knowledge source (an entry at `gotchas/timeouts.md` is a
`gotchas` entry). There are six categories, each with a real definition — a
purpose, a boundary (what looks similar but belongs elsewhere), a retrieval tier,
and an expected entry shape:

| Category | Tier | What it holds |
|----------|------|---------------|
| `conventions` | always-applied | The rules a team always wants honoured — coding standards, naming, formatting, required patterns, house style. A standing rule stated as an instruction to follow. |
| `glossary` | always-applied | The shared vocabulary of the project — the domain and project-specific terms needed to read the rest of the knowledge base and the code. A term and a short gloss. |
| `architecture` | looked-up | How the system is built and how new work must be built into it — components, responsibilities, boundaries, data and control flow. Binding on the work being planned, not a survey of today's code. |
| `gotchas` | looked-up | Sharp edges and non-obvious traps — surprising behaviours and easy mistakes. A warning about a specific trap and how to avoid it. |
| `learnings` | looked-up | Empirical knowledge from doing the work — what was tried, what worked, what didn't. An observation from experience. |
| `decisions` | looked-up | The reasoning behind choices (ADR-style) — options considered, trade-offs weighed, why one path was taken. The why, not the what or how. |

These definitions are the categories' contract with both contributors and the
assistant. The boundaries are what keep the categories distinct: the *reasoning*
behind a rule is a `decision`, not a `convention`; a *defined term* is a
`glossary` entry, not `architecture`; an *empirical finding* is a `learning`, not
a `gotcha`.

### A single source of truth

The category model is declared **once**, in a registry in the code
(`internal/knowledge/category.go`). That one declaration drives everything
downstream:

- **Project initialisation** scaffolds a directory and a self-documenting README
  for each category from the registry, so a freshly initialised project contains
  every category with a real definition rather than a placeholder.
- **Retrieval behaviour** (below) reads each category's tier from the registry.
- **Contribution routing** surfaces the definitions to the assistant via the
  `spektacular knowledge categories` command, so a new entry is filed in the right
  place.

Adding a category, or moving one between tiers, is a change to this registry alone.

## The two retrieval tiers

Categories are split into two tiers by *how they are retrieved*, declared per
category in the registry:

- **Always-applied** (`conventions`, `glossary`). Loaded **in full on every task**
  and deliberately **excluded from search**, so the same content is never surfaced
  twice. These are the rules every plan must reflect and the vocabulary needed to
  read everything else. Keep them compact — their whole content is paid for on
  every task.
- **Looked-up** (`architecture`, `gotchas`, `learnings`, `decisions`). The larger
  reference body, retrieved **only when a query matches**. This is reached through
  keyword search, targeted at the specific surfaces a feature touches.

Because tier membership is read from the single registry declaration, **re-tiering
is one self-consistent action**: flipping a category to always-applied
simultaneously makes the full-load reader include it *and* makes search stop
returning it. The two behaviours cannot drift out of step — there is no second
place to forget to update.

The always-applied set is loaded with `spektacular knowledge always-applied`
(every always-applied entry across all sources, each tagged with its category).
The legacy `spektacular knowledge conventions` command remains for backward
compatibility, returning only the conventions.

## Search behaviour

Search is keyword-based and runs in-process. Results come back ranked — one result
per matching document, strongest match first. Each result carries:

- its **tier** and store **name** (which configured store it came from — together
  with the path, exactly what a read needs to fetch it back),
- its **category** label (so a consumer can tell a `gotchas` warning from an
  `architecture` fact from a `learnings` finding),
- a **checksum** (a content hash, used for de-duplication — see below),
- its **tags** (what the entry declares itself to be about; an empty list when it
  declares nothing),
- a **title**, a **score**, and up to three **excerpts** of the strongest matching
  lines.

**A document does not have to contain every word of the query.** It is returned if
it carries evidence for any of them, and ranked on how good a match it is. Two
consequences follow, and both matter to anything consuming search: a returned hit
is not proof that every query word appeared in it, and an empty result is not
proof that nothing on the subject exists.

Always-applied categories never appear in search results — they are already loaded
in full — so search surfaces only the looked-up reference knowledge.

### How a result is ranked

The query is lower-cased and split on whitespace into terms. There is no stemming,
no synonyms and no plural handling, so `router` and `routers` are different terms.

For each term, an entry offers at most two kinds of evidence:

- a **tag match**, full strength when the term equals one of the entry's tags and
  reduced when the two are prefix-related (see below);
- **body occurrences**, the number of case-insensitive substring matches in the
  entry's text. The frontmatter block is not part of the text, so a tag is never
  also counted as a mention of itself.

```text
damp(n)        = 0                         if n = 0
               = 1 + log2(n)               otherwise

tagAffinity(t) = 1                         if t equals one of the tags
               = len(shorter)/len(longer)  if t and a tag are prefix-related
                                           and the shorter is >= minPrefixLen
               = 0                         otherwise

termScore(t)   = tagWeight * tagAffinity(t) + damp(bodyCount(t))

coverage       = (terms with any evidence) / (total terms)

score          = sum(termScore(t) for t in terms) * coverage ^ coverageExponent
```

with `tagWeight = 8`, `coverageExponent = 2` and `minPrefixLen = 4`. Where a term
is prefix-related to more than one tag, the strongest match wins; affinities do
not add up.

**The named constants in `internal/knowledge/ranking.go` are the single source of
truth for these values.** They are quoted here, on the published knowledge base
page, and in the `architecture/knowledge-search-ranking.md` entry, so changing one
is a four-file change: the code and all three documents, or they disagree.

A worked example, searching `http routing` against two entries:

| | Entry A, tagged `http, routing` | Entry B, untagged, mentions "http" 8 times |
|---|---|---|
| `http` | tag match, 0 in body: 8 + 0 = 8 | no tag, 8 in body: 0 + (1 + log2 8) = 4 |
| `routing` | tag match, 2 in body: 8 + 2 = 10 | no evidence: 0 |
| coverage | 2 of 2 = 1.0 | 1 of 2 = 0.5 |
| **score** | (8 + 10) * 1.0² = **18** | 4 * 0.5² = **1** |

Entry B mentions the word far more often and still ranks well below Entry A,
because Entry A is *about* both terms and Entry B is about neither.

**Prefix partial credit.** A tag does not have to be typed exactly. When a search
term and a tag are prefix-related, the tag still counts, at the shorter string's
length over the longer's: `http` finds a `https` tag at 80%, `apple` and `apples`
find each other at 83%, `https` finds `https-security` at 36%. Two guards are
deliberate. The match must be anchored at the start, so `test` does not find
`latest`; and the shorter of the two must be at least `minPrefixLen` characters,
so `go` matches only exactly rather than reaching `golang`, `google` and
`gorilla`.

This is **positional, not morphological** — it is not stemming. `route` does not
find `routing`, because `route` is not the opening of `routing`. Where a form
differs by more than its ending and both are likely search words, carry both tags.
Edit distance was considered and rejected for exactly this surface: on short
technical tokens it scores `test`/`rest` and `cors`/`core` at 0.75 and
`tls`/`tld`, `dns`/`dos` and `api`/`apt` at 0.67, all unrelated subjects and all
plausible tags here, while producing scores identical to prefix matching on every
pair that should match.

**The cutoff.** Once every store has been searched and the results merged, any hit
scoring below `cutoffFraction` of the best hit is dropped, with
`cutoffFraction = 0.25`. The threshold is relative rather than fixed, so a loosely
related entry surfaces when nothing better exists and falls away once something
genuinely relevant is present, and it keeps its meaning as a knowledge base grows.
In the example above, Entry B is dropped: 1 is below 0.25 × 18. Ties break by
configured store order and then by path, so repeated searches return the same
order.

### Where each part is computed

A store finds candidate documents and describes them: per-term body occurrence
counts, the entry's tags, the locator, title, excerpts and checksum. It does
**not** compute a score. Tokenizing the query, scoring every reported hit,
merging, sorting and applying the cutoff all happen once in the knowledge layer,
above the store interface.

The reason is the merge. Hits from every covered store are ranked against each
other, so every score must be on the same scale; a formula reproduced inside each
provider would have to be reproduced exactly, and any drift would leave the merged
ordering quietly wrong rather than visibly broken. The precedent is already in the
result type — `tier`, `name` and `category` are left empty by the store because it
has no notion of its caller's addressing scheme, and `score` joins them because it
has no notion of the stores it will be ranked against. One consequence is worth
stating for anyone writing a backend: a store must be able to report per-term
occurrence counts to take part in this ranking at all. The public reference for
writing a storage backend covers the contract in full.

### Tags on an entry

An entry may open with a YAML frontmatter block declaring what it is about,
independently of the words its prose happens to use:

```markdown
---
tags: [go, http, routing]
---

# HTTP routing standard

All Go services route HTTP endpoints through chi.
```

The block list form works too:

```markdown
---
tags:
  - go
  - http
  - routing
---
```

The rules, each of which is a real behaviour rather than a convention:

- **Tags are optional.** An entry without a block is perfectly valid and fully
  searchable. There is nothing to migrate, no backfill, and no command to run.
- **The block must lead the file**, opened and closed by a line containing only
  `---`.
- **`tags` is the only key read.** Any other key is ignored, so the block is safe
  to use for other metadata.
- **Tags are lower-cased, trimmed and de-duplicated** when read, so `HTTP` and
  `http` can never both exist as separate tags.
- **A malformed or unclosed block is treated as no frontmatter**, not as an error.
  An entry that happens to open with a horizontal rule keeps working, and one bad
  entry can never fail a search across a whole store.
- **The block is not part of the entry's prose**, so a tag is not also counted as
  a body mention of itself. The checksum is the deliberate exception: it covers the
  file's exact raw bytes including the block, because it identifies the file rather
  than its prose, so two entries differing only in their tags stay distinct during
  de-duplication.

You rarely write this block by hand. The `spek-knowledge` skill proposes tags when
it captures an entry, drawn from the vocabulary already in use, and shows them
alongside the destination for confirmation before anything is written.

## De-duplication and consolidation

The same knowledge often lives in more than one place — copied across stores, or
worded differently in two entries. A lookup does not return that pile of
overlapping hits. Instead it returns a single consolidated, source-cited answer,
produced in two strictly separated stages:

1. **Exact de-duplication (deterministic).** Search results are grouped by their
   `checksum`. Results sharing a checksum are **byte-identical** copies of the same
   entry; each group collapses to one candidate, unioning the source citations of
   every copy. This is pure equality — no thresholds, no judgement.
2. **Consolidation (judgement).** The surviving unique candidates are handed to a
   consolidation **sub-agent**, which reads their full bodies and classifies the
   *relationship* between them:
   - **equivalent** (same point, different words) → merged into one cited point;
   - **refinement** (one is a more specific case) → kept alongside what it
     refines and noted, never hidden (there is no precedence between stores —
     see below);
   - **genuine contradiction** (sources actually disagree) → **surfaced** as a
     conflict naming both sources, never silently dropped;
   - **distinct** → both kept.

   The sub-agent returns one consolidated answer, each point citing the source(s)
   it was drawn from. The raw per-source hit list is never the output. (Where an
   executing agent cannot spawn a sub-agent, the identical consolidation runs
   inline; only the context isolation differs.)

### Why the mechanical layer is exact, not fuzzy

De-duplication and ranking answer different questions, and the tools differ
accordingly. De-duplication asks whether two entries are *the same*, which is a
yes-or-no fact and is therefore decided on exact bytes. Ranking asks which entry
is a better *answer* to a query, which is a matter of degree and is therefore
scored, with prefix partial credit and damping. The argument below is about the
first question only; it is not an argument against inexact ranking.


The deterministic de-dup step matches on **exact byte-identity** rather than a
normalized or similarity-based fingerprint. This is deliberate, because **lexical
distance does not track semantic distance**:

- "Always retry" and "never retry" are ~95% lexically identical yet a total
  contradiction.
- "Add jitter to retries" and "randomize backoff intervals" share almost no words
  yet state the same knowledge.

A graded similarity score (edit distance, SimHash/MinHash) would mis-rank exactly
these cases and reduces to a brittle magic threshold — and a mechanical similarity
index for equivalence is precisely the kind of embedding/semantic index the
project does not build. The real need is not a difference *scalar* but a
relationship *classification* (a 70%-similar pair could be a refinement or a
contradiction), which only an LLM does reliably.

So exact-equivalence is the most effective *mechanical* primitive precisely because
it is narrow: it claims the one provable point — zero difference — for free, and
routes everything semantic to the judgement layer, fed the inputs that make that
judgement good (full bodies, the store each came from, and the category label).

## Two tiers, and how a store is addressed

Knowledge lives in one of two tiers. Every registered repo contributes exactly one
store, holding what is true of that repo's own code, declared as a single provider
block in its `repo.yaml` and addressed by the name the project registered the repo
under. The project declares any number of shared stores under `knowledge.sources`
in `.spektacular/config.yaml`, each with its own `name`, for knowledge that belongs
to no single repo. Names must be unique within a tier; the same name may appear
once in each, because a store is identified by its tier and name together.

Reading and writing address exactly one store, so they take a `tier` and a `name`
alongside the path. A request that leaves either out is refused rather than
resolved to whichever store looks closest, and the refusal lists the names
available in that tier so it can be reissued immediately.

Searching, listing, conventions, and the always-applied load take `--tier
<project|repo|all>` and a repeatable `--filter <name>`. Omitting the narrowing
covers every store the tier reaches; naming stores covers exactly those. No store
is ever included or excluded implicitly, which is what lets planning work on one
repo skip every other repo's standing rules.

**There is no precedence between stores.** A repo's own store answers what is true
of that repo's code, and the project's shared stores answer what spans repos; they
answer different questions rather than overriding one another. During consolidation
a refinement is kept alongside what it refines and noted rather than hidden, and a
*genuine* disagreement is surfaced as a conflict naming both stores, not silently
resolved.

## Contributing knowledge

Contributions are routed to the right category at the moment they are filed. The
`spek-knowledge` skill loads the category definitions
(`spektacular knowledge categories`), picks the category whose purpose matches the
entry and whose boundary does not exclude it, and steers over-long or
multi-paragraph content out of the always-applied `glossary` toward a more fitting
category — keeping the always-applied retrieval tier compact. The entry is filed at
`<category>/<slug>.md`.

The skill also loads the tag vocabulary already in use
(`spektacular knowledge tags`) and proposes tags for the entry, preferring an
established tag over a near-duplicate so the vocabulary converges instead of
fragmenting as the knowledge base grows. As always, the assistant proposes the
destination — the tier, the store name, and the path — along with the tags and the
body, and waits for explicit confirmation before writing. That is one gate, not
several: tags are confirmed with everything else.

### Auditing the tags on existing entries

Entries written before tags existed, or tagged carelessly, can be reviewed the
same way. There is no separate command: invoke the `spek-knowledge` skill and ask
it to audit the knowledge base tags. The skill discriminates the audit intent from
the wording of the request, the same way it tells a lookup from a contribution.

The flow is read-only until you approve something. It enumerates the entries in
scope with `knowledge list`, loads the vocabulary in use with `knowledge tags`,
reads each entry, and for each one reports two things:

- **Unsupported tags** — a tag the entry's content does not bear out, reported for
  removal.
- **Missing tags** — a subject the entry is clearly about but carries no tag for,
  proposed from the existing vocabulary where something fits and as a new tag only
  where nothing does.

Changes are proposed and confirmed **one entry at a time**. Accepting one entry's
changes never applies another's, and declining one does not carry to the next, so
a suggestion you disagree with costs a "no" rather than a cleanup. Narrow the
scope with `--tier` and `--filter` when you only want part of the knowledge base
reviewed.

Two judgements the audit is deliberately careful about, because they pull in
opposite directions. It will not tell an entry tagged `https` to use `http`
instead: those are different subjects, and prefix matching already relates them at
reduced strength. But it will report `apples` sitting beside `apple` as removable,
because prefix matching already reaches it and a redundant tag is how a vocabulary
silently doubles.

### Reviewing whether entries are still true

The audit above asks whether an entry is well *labelled*. Its sibling, the
maintenance review, asks the question that matters more over time: whether the
entry is still *true*. It is the same skill and the same shape — invoke
`spek-knowledge` and ask whether the knowledge base is still accurate — and it is
read-only until you approve something, one entry at a time.

Each entry in scope is classified as **current**, **stale**, **incorrect** or
**unverifiable**, and every stale or incorrect verdict has to name the specific
file, command or behaviour that changed, so you can check a finding rather than
take it on trust. "Looks old" is not a finding.

One rule decides whether such a review helps or harms, and it is worth stating
plainly: an entry describing a standard the code has not met yet is **current**,
not stale. In this project an entry states the target and the code is what has yet
to meet it, so a difference between the two is work to do, never evidence against
the entry. Only an entry whose subject no longer exists is stale. Without that
rule a review would propose deleting exactly the entries doing the most work.

The review also reports any category description that no longer matches the
project's current definition of that category, naming where it is and pointing at
`spektacular init <agent>` as the remedy. It reports drift and never repairs it —
bringing a store back into line stays something you run deliberately. An entry you
agree to remove is removed with `spektacular knowledge delete`, and only after you
agree to that entry specifically.

## Command reference

Agents (and you) reach knowledge through the `spektacular knowledge` commands
rather than reading the files directly, so access stays consistent across stores.
Each command has a `--schema` mode that prints its input/output schema.

| Command | Purpose |
|---------|---------|
| `spektacular knowledge search <query> [--tier T] [--filter N] [--tag G]` | Keyword-search the stores the request covers (always-applied categories excluded); ranked, one tier-, store- and category-tagged result per matching document, each with title, score, excerpts, tags, and a content checksum. A document need not contain every query word. Repeatable `--tag` restricts results to entries carrying every tag listed |
| `spektacular knowledge read --data '{"tier":"repo","name":"docs","path":"architecture/x.md"}'` | Read one entry's full body from one addressed store |
| `spektacular knowledge list [--tier T] [--filter N]` | List every entry across the stores the request covers |
| `spektacular knowledge write --data '{"tier":"repo","name":"docs","path":"gotchas/x.md"}' --file <path>` | Write an entry into one addressed store (content from `--file`, or stdin) |
| `spektacular knowledge delete --data '{"tier":"repo","name":"docs","path":"gotchas/x.md"}'` | Remove one entry from one addressed store. An address naming an undeclared store is refused with the names available in that tier; an address holding nothing reports success and changes nothing, so a maintenance pass that retries is safe. A category's own generated `README.md` is refused, since it is rendered from the project's definition of that category rather than written by anyone |
| `spektacular knowledge sources` | List the configured stores by tier and name, with their locations |
| `spektacular knowledge categories` | List the category definitions — purpose, boundary, tier, and entry shape |
| `spektacular knowledge tags [--tier T] [--filter N]` | List the tag vocabulary already in use across the stores the request covers, each with the number of entries carrying it, most-used first |
| `spektacular knowledge always-applied [--tier T] [--filter N]` | Read every always-applied entry (conventions and glossary) across the stores the request covers, each tagged with its category |
| `spektacular knowledge conventions [--tier T] [--filter N]` | Read every convention across the stores the request covers (the conventions-only view) |
