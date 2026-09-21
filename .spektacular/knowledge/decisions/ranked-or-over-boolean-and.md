---
tags: [search, ranking, retrieval, knowledge]
---

# Knowledge search ranks rather than excludes

**Decision:** `knowledge search` matches a document that carries evidence for
*any* query term, ranks the results, and drops weak matches by a cutoff relative
to the best hit. It does not require every query term to be present.

## The choice this replaced

Search required *every* term to occur in a document, as a literal
case-insensitive substring. A document missing one term was not ranked low, it
was excluded. Adding a term to a query could therefore only ever remove results,
never reorder them.

## Why it changed

The query is chosen by the coding agent at discovery time, and it is not stable.
Across three plan runs against one unchanged spec, the agent searched
`http endpoints go`, then `http handler`, then
`http endpoints handler routing`. One on-topic entry was retrieved by all three
only because it happened to contain every one of those words. A fourth plausible
phrasing for the same subject, `api`, returned nothing at all — the entry is
about HTTP API routing but never uses the word "API".

Under AND matching, an author must therefore anticipate the exact vocabulary a
model will later invent, and the only available tactic is to pack an entry with
synonyms. That degrades the entry for human readers and still cannot be
exhaustive.

## Rationale for the specific shape

**Coverage scales the score.** The fraction of query terms with any evidence
multiplies the result, so an entry matching one term of three ranks far below one
matching all three. Without this, ranked OR would flood results with documents
that share a single common word.

**The cutoff is relative to the best hit, not absolute.** A weak match surfaces
when nothing better exists and disappears when something does. An absolute
threshold would need tuning per corpus and would silently change meaning as a
knowledge base grew.

## Alternatives considered

- **Keep AND, and rely on authors writing richer entries.** Rejected: it makes
  retrieval depend on guessing a model's vocabulary, and it rewards
  keyword-stuffing over clear writing.
- **Keep AND, with an OR fallback when a query returns nothing.** Rejected: two
  ranking regimes, and the fallback fires exactly when the agent has already
  concluded there is no relevant knowledge.
- **Query expansion — synonyms or stemming before matching.** Rejected as the
  primary fix: it guesses at vocabulary rather than removing the dependence on
  it, and it makes results harder to explain. Not ruled out as a later addition.
- **An absolute score threshold.** Rejected in favour of the relative cutoff, for
  the tuning and corpus-growth reasons above.

## Consequences

- A query can return a weakly related entry where it previously returned nothing.
  That is intended: near-zero scores are the mechanism, exclusion is not.
- Ranking quality now carries the weight that boolean matching used to. Scoring
  signals — tag matches, and diminishing returns on repeated body occurrences —
  become load-bearing rather than cosmetic.
- Callers that treated "no results" as proof of absence are relying on a
  guarantee search no longer makes.
