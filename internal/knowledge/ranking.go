package knowledge

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/hivecommons/spektacular/internal/store"
)

// Ranking lives here, one layer above the store interface, and deliberately not
// inside any store. A store finds candidate documents and reports what evidence
// each one offers; turning that evidence into a comparable number happens once,
// here, alongside the cross-store merge in Set.Search.
//
// The reason is the merge. Set.Search ranks hits from every covered store
// against each other, so every score it sorts must be on the same scale. A
// formula reproduced inside each provider would have to be reproduced exactly,
// and any drift — a different weight, a rounding difference, a forgotten factor —
// would leave the merged ordering quietly wrong rather than visibly broken.
// The precedent is already in store.Hit: Tier, Name and Category are left empty
// by the store because a store has no notion of its caller's addressing scheme.
// It equally has no notion of the stores it will be ranked against, so Score
// belongs in the same group.
//
// A provider therefore implements "find and describe", never "find, describe and
// rank". The cost of that split is a real constraint on future providers: a
// store must be able to report per-term body occurrence counts to take part in
// this ranking at all.

// The tunable constants of the ranking formula. They are named values in one
// place so tuning is a single-site change rather than an edit scattered across
// the scan.
const (
	// tagWeight is how much a full tag match on one term is worth relative to
	// that term's body occurrences. It is the only signal in this formula that
	// says "this entry is about X" as opposed to "this entry mentions X", so it
	// is set well above what any plausible number of body occurrences can reach:
	// a term would have to appear 128 times to earn 8 from damp alone.
	tagWeight = 8.0

	// minPrefixLen is how many runes the shorter of a term and a tag must reach
	// before a prefix relation counts for anything. Below it, matching is exact
	// only. Without this guard "go" would earn credit from golang, google and
	// gorilla alike, which is the "any short term hits everything" failure that
	// ruled out plain substring matching in the first place.
	minPrefixLen = 4

	// coverageExponent shapes how sharply a document is penalised for
	// carrying evidence for only part of the query. Squaring means a document
	// covering half the query keeps a quarter of its score, which is what puts
	// a partial match well below a full one instead of merely behind it.
	coverageExponent = 2.0

	// cutoffFraction is the share of the best hit's score a hit must reach to
	// be worth showing. It is relative rather than an absolute number on
	// purpose: a loosely related entry should surface when nothing better
	// exists and vanish once something genuinely relevant is present, and a
	// fixed threshold would have to be retuned as a knowledge base grows.
	cutoffFraction = 0.25
)

// Terms splits a free-form query into the ordered, lower-cased terms every
// other part of retrieval indexes evidence by. It lives in the knowledge layer
// rather than in a store because tokenization is a retrieval-policy decision,
// and because evidence is reported as per-term slices: the caller and the store
// must agree on term order by construction rather than by convention.
//
// There is no stemming, no synonym expansion and no plural handling, so
// "router" and "routers" are different terms. A query that is empty or all
// whitespace yields no terms, which callers treat as "search nothing".
func Terms(query string) []string {
	return strings.Fields(strings.ToLower(query))
}

// Evidence is what one scanned document offers for one query. Both slices are
// indexed like the terms slice the query was tokenized into, so element i of
// each describes term i.
type Evidence struct {
	// BodyCounts is how often each term occurs in the document's text.
	BodyCounts []int

	// TagAffinity is how strongly each term matches the document's tags: 0 for
	// no match, 1 for an exact one, and the prefix partial-credit fraction in
	// between. It is []float64 rather than []bool precisely so partial credit is
	// expressible, which makes an exact match the value 1 rather than a separate
	// code path.
	TagAffinity []float64
}

// damp converts a raw occurrence count into what that count is worth. It grows
// with n but ever more slowly, so mentioning a term twenty times is better than
// mentioning it twice and nowhere near ten times better: a long document cannot
// climb the results on volume alone.
//
// damp(1) is exactly 1, and that is deliberate rather than incidental. A single
// occurrence of a single term scores exactly 1.0, which is what the historical
// rule scored it, so every assertion pinning that number stays meaningful.
// Reshaping this curve without preserving damp(1) = 1 breaks them.
func damp(n int) float64 {
	if n <= 0 {
		return 0
	}
	return 1 + math.Log2(float64(n))
}

// tagAffinity reports how strongly term matches the best of an entry's tags: 1
// for an exact match, a fraction for a prefix relation, and 0 for everything
// else. Both sides are already lower-cased. Where several tags match, the
// strongest one decides — affinities do not accumulate, or an entry could earn
// several times the tag weight for a single query word.
//
// The relation is bidirectional prefix with proportional partial credit: when
// one string opens the other, the credit is the shorter's length over the
// longer's. So "http" earns four fifths from an "https" tag, and "apple" and
// "apples" find each other at five sixths without either form being written
// down twice.
//
// It is deliberately not a general string-distance measure, and edit distance in
// particular would be actively wrong here. Tags are overwhelmingly short
// technical tokens, which is exactly where edit distance is weakest: it scores
// test/rest and cors/core at 0.75, and tls/tld, dns/dos and api/apt at 0.67 —
// every one of those a pair of unrelated subjects and a plausible tag in this
// codebase. Prefix scores all of them zero while producing identical numbers on
// every pair that should match. Do not "improve" this into a similarity metric.
//
// Two guards keep it honest, and both are load-bearing. The match is anchored at
// the start, so "test" earns nothing from "latest"; and the shorter string must
// reach minPrefixLen, so "go" earns nothing from "golang".
//
// What it does not do is understand words. "route" does not match "routing",
// because "route" is not the opening of "routing" — prefix matching is
// positional, not morphological. Closing that gap needs stemming, which is out
// of scope; the capture flow carries a rule to add a second tag where a form
// differs by more than its ending.
func tagAffinity(term string, tags []string) float64 {
	best := 0.0
	for _, tag := range tags {
		if tag == term {
			return 1
		}
		shorter, longer := term, tag
		if len(longer) < len(shorter) {
			shorter, longer = longer, shorter
		}
		if !strings.HasPrefix(longer, shorter) {
			continue
		}
		shortLen := utf8.RuneCountInString(shorter)
		if shortLen < minPrefixLen {
			continue
		}
		if affinity := float64(shortLen) / float64(utf8.RuneCountInString(longer)); affinity > best {
			best = affinity
		}
	}
	return best
}

// score returns the document's ranking score from the evidence a store
// reported, or 0 when the document carries evidence for no term at all. It is a
// pure function: no filesystem access, no knowledge of stores, cutoffs or
// categories, which is what makes the weights tunable and testable directly.
//
// Matching is a ranked OR. A document missing some of the query's terms is a
// real result with a low score rather than an exclusion — the historical rule
// dropped it outright, which is why a search that should have found the closest
// available entry found nothing instead.
//
// Each term earns the tag weight scaled by how well it matches the entry's tags,
// plus its damped body occurrences. A tag match therefore outweighs any
// realistic number of passing mentions, which is what lets an entry be found for
// what it is about rather than for the words it happens to use. The total is
// then scaled by the share of the query the document has any evidence for at
// all, tag or body, so covering more of what was asked beats covering less.
func score(e Evidence) float64 {
	total := 0.0
	covered := 0
	for i, count := range e.BodyCounts {
		affinity := 0.0
		if i < len(e.TagAffinity) {
			affinity = e.TagAffinity[i]
		}
		termScore := tagWeight*affinity + damp(count)
		if termScore > 0 {
			covered++
		}
		total += termScore
	}
	if covered == 0 {
		return 0
	}
	coverage := float64(covered) / float64(len(e.BodyCounts))
	return total * math.Pow(coverage, coverageExponent)
}

// ScoreHit scores one reported hit against the query terms it was searched
// with: the tag affinity of each term against the hit's declared tags, plus
// the damped body counts the reporter observed. It is the one entry point to
// the formula for anything ranking documents outside a Set — research seeding
// ranks ADR files and Hive knowledge entries with it — so every ranked source
// shares one scale and the constants above stay the single source of truth.
func ScoreHit(terms []string, hit store.Hit) float64 {
	affinity := make([]float64, len(terms))
	for i, term := range terms {
		affinity[i] = tagAffinity(term, hit.Tags)
	}
	return score(Evidence{BodyCounts: hit.BodyCounts, TagAffinity: affinity})
}

// CutoffFloor is the lowest score a hit may carry and still be shown, given
// the best score in the same result set.
func CutoffFloor(best float64) float64 {
	return best * cutoffFraction
}
