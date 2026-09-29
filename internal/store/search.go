package store

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// maxExcerptBytes caps a Hit excerpt so an agent can scan many hits cheaply.
const maxExcerptBytes = 256

// maxExcerptsPerHit caps how many excerpt lines a single Hit carries.
const maxExcerptsPerHit = 3

// scanBufferBytes bounds how long a single scanned line may be; a file
// containing a longer line is scanned up to that line and then skipped.
const scanBufferBytes = 1024 * 1024

// binarySniffBytes is how many leading bytes of a file are checked for a NUL
// byte to classify it as binary — the same convention git uses.
const binarySniffBytes = 8000

// Search returns hits for a pre-tokenized keyword query, scanning only this
// store. It walks the store root in process — no external tool is consulted.
// Terms arrive already lower-cased, in the order the caller indexes evidence
// by; each is matched as a literal, case-insensitive substring, and every
// document offering evidence for at least one of them produces exactly one Hit
// reporting how often each term occurs in it.
//
// The Hit is left unranked as well as unattributed: this store reports what it
// found and never computes a Score. Whether a document is relevant enough to
// show, and how it orders against documents from other stores, are decisions
// about the whole result set, which the knowledge layer makes.
//
// The store is category-agnostic: it scans every directory and never excludes
// one by name — tier-based exclusion of always-applied categories lives in the
// knowledge layer, and .spektacular_ignore exclusion lives in the ignore-aware
// wrapper (NewIgnoreStore), never here. Binary files are skipped, and
// a file containing an over-long line is scanned only up to that line. A
// query with no terms, or one with no matches, returns an empty result, not an
// error.
func (f *FileStore) Search(terms []string, opts SearchOptions) ([]Hit, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	return f.search(terms, opts)
}

// search walks the store root, scans every file once, and emits one Hit per
// file offering evidence for at least one term. Terms must already be
// lower-cased.
func (f *FileStore) search(terms []string, opts SearchOptions) ([]Hit, error) {
	var hits []Hit

	err := filepath.WalkDir(f.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		agg, err := scanFile(path, terms)
		if err != nil {
			return err
		}
		// Fast path only: a document lacking a requested tag is discarded by the
		// knowledge layer anyway, so skipping it here just saves reporting it.
		if !CarriesEveryTag(agg.tags, opts.Tags) {
			return nil
		}
		// The only rejection left here is "this document offers no evidence of
		// any kind", which would otherwise make every file in the store a hit.
		// Deciding how much evidence is enough moved out of this walk and into
		// the knowledge layer, which is the only place hits from every store are
		// visible at once — so this is deliberately not the per-term early
		// return it replaced, and restoring that would reinstate boolean AND.
		//
		// Declared tags count as evidence in their own right, and must, because
		// an entry tagged "go" whose prose never says "go" has to reach the
		// ranking layer to be scored on those tags at all. The store reports it
		// without judging how relevant the tags are: weighing a tag against a
		// query is ranking, and ranking is not this layer's job. An irrelevant
		// tagged entry simply scores zero upstream and is dropped there.
		evidence := len(agg.tags) > 0
		for _, count := range agg.counts {
			if count > 0 {
				evidence = true
				break
			}
		}
		if !evidence {
			return nil
		}
		rel, relErr := filepath.Rel(f.root, path)
		if relErr != nil {
			rel = path
		}
		title := agg.title
		if title == "" {
			title = rel
		}
		excerpts := make([]string, 0, len(agg.best))
		for _, c := range agg.best {
			excerpts = append(excerpts, trimExcerpt(c.text))
		}
		// Always a list, never nil, so the JSON carries "tags": [] for an
		// untagged entry rather than omitting the key.
		tags := agg.tags
		if tags == nil {
			tags = []string{}
		}
		hits = append(hits, Hit{
			Path:       rel,
			Title:      title,
			Excerpts:   excerpts,
			Checksum:   agg.checksum,
			Tags:       tags,
			BodyCounts: agg.counts,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("native search: %w", err)
	}
	return hits, nil
}

// candidateLine is one matching line considered for a Hit's excerpts,
// together with the strength used to rank it: how many distinct terms occur
// on it and the total occurrences of all terms on it.
type candidateLine struct {
	text     string
	distinct int
	total    int
}

// fileAggregate describes one scanned file as a whole: how often each term
// occurred across the file, the text of its first ATX heading, and the
// strongest matching lines, bounded at maxExcerptsPerHit.
type fileAggregate struct {
	counts   []int           // total occurrences per term, indexed like the terms slice
	tags     []string        // the entry's declared tags, nil when it has no frontmatter block
	title    string          // first ATX-heading text, "" when the file has none
	titleSet bool            // whether a heading line was seen, so later ones never win
	best     []candidateLine // strongest lines: distinct terms desc, total desc, file order asc
	checksum string          // hex SHA-256 over the file's exact raw bytes, "" for binary files
}

// addCandidate ranks line into the aggregate's bounded excerpt candidates —
// most distinct terms first, then most total occurrences, then earliest in
// the file. Inserting after equal entries keeps file order for ties, and the
// bound keeps memory line-sized however many lines match.
func (a *fileAggregate) addCandidate(c candidateLine) {
	pos := len(a.best)
	for i, b := range a.best {
		if c.distinct > b.distinct || (c.distinct == b.distinct && c.total > b.total) {
			pos = i
			break
		}
	}
	if pos == maxExcerptsPerHit {
		return
	}
	a.best = append(a.best, candidateLine{})
	copy(a.best[pos+1:], a.best[pos:])
	a.best[pos] = c
	if len(a.best) > maxExcerptsPerHit {
		a.best = a.best[:maxExcerptsPerHit]
	}
}

// scanFile reads the file at path once and aggregates, per term, the number
// of non-overlapping case-insensitive occurrences across the entry's body,
// along with the entry's declared tags, its first ATX-heading text and its
// strongest matching lines. Terms must already be lower-cased. Binary files — a
// NUL byte within the leading binarySniffBytes — quietly yield an empty
// aggregate, and once a line exceeds scanBufferBytes the remainder of the file
// is skipped, keeping the aggregate collected so far.
//
// Everything after the frontmatter block is the body: term counting, title
// detection and excerpt collection all start there, so a tag is never also
// counted as a prose mention of itself. The checksum is the exception and stays
// over the file's exact raw bytes, block included, because it identifies the
// file rather than its prose — two entries differing only in their tags must not
// collapse into one candidate during exact-byte de-duplication.
func scanFile(path string, terms []string) (fileAggregate, error) {
	agg := fileAggregate{counts: make([]int, len(terms))}

	file, err := os.Open(path)
	if err != nil {
		return agg, err
	}
	defer file.Close()

	sniff := make([]byte, binarySniffBytes)
	n, err := io.ReadFull(file, sniff)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return agg, err
	}
	sniff = sniff[:n]
	if bytes.IndexByte(sniff, 0) >= 0 {
		return agg, nil
	}

	// Accumulate a SHA-256 over the file's exact raw bytes during this same
	// read, so the checksum needs no second pass. The sniff bytes were already
	// read off the file, so feed them to the hasher directly; the remainder is
	// teed into the hasher as the scanner consumes it.
	hasher := sha256.New()
	hasher.Write(sniff)
	tee := io.TeeReader(file, hasher)

	// The frontmatter block, if there is one, is by definition at the very start
	// of the file, so it is already sitting in the bytes read for the binary
	// sniff — no second read, and no buffering of scanned lines. The hasher has
	// been fed the whole sniff above, so trimming the block off here changes
	// what is scanned without changing what is checksummed.
	//
	// A malformed block is not allowed to fail the scan: ParseEntry hands back
	// the raw bytes alongside its error, so the entry stays searchable and
	// simply has no tags. A block extending beyond binarySniffBytes reads as no
	// frontmatter for the same reason — degrade, never fail.
	tags, body, ferr := ParseEntry(sniff)
	if ferr != nil {
		tags, body = nil, sniff
	}
	agg.tags = tags

	scanner := bufio.NewScanner(io.MultiReader(bytes.NewReader(body), tee))
	scanner.Buffer(make([]byte, 0, 64*1024), scanBufferBytes)
	for scanner.Scan() {
		agg.scanLine(scanner.Text(), terms)
	}
	if err := scanner.Err(); err != nil {
		// A scanner is unrecoverable after an over-long line, so skip the
		// rest of this file and keep the aggregate collected so far.
		if !errors.Is(err, bufio.ErrTooLong) {
			return agg, err
		}
	}
	// Drain any bytes the scanner did not consume (e.g. after an over-long
	// line) through the hasher so the checksum covers the file's exact raw
	// bytes in full, not just the scanned prefix.
	if _, err := io.Copy(io.Discard, tee); err != nil {
		return agg, err
	}
	agg.checksum = hex.EncodeToString(hasher.Sum(nil))
	return agg, nil
}

// scanLine folds one body line into the aggregate: the first heading becomes
// the title, each term's occurrences are counted, and a matching line is
// ranked as an excerpt candidate. It is the one per-line rule shared by a file
// scan and DescribeBytes, so a document reports the same evidence whether it
// was read off disk or handed over in memory.
func (a *fileAggregate) scanLine(line string, terms []string) {
	if !a.titleSet {
		if text, ok := headingText(line); ok {
			a.title = text
			a.titleSet = true
		}
	}
	lowered := strings.ToLower(line)
	cand := candidateLine{text: line}
	for i, term := range terms {
		if count := strings.Count(lowered, term); count > 0 {
			a.counts[i] += count
			cand.distinct++
			cand.total += count
		}
	}
	if cand.total > 0 {
		a.addCandidate(cand)
	}
}

// DescribeBytes describes an in-memory document exactly as Search describes a
// file: per-term body counts, the first heading as title (falling back to
// locator), the strongest matching lines as excerpts, and a checksum over the
// exact bytes. tags are the document's declared tags, supplied by the caller
// because a document that did not come from a file declares them in its own
// format. ok is false when the document offers no evidence for any term and
// carries no tags, the same rejection Search applies. Terms must already be
// lower-cased.
func DescribeBytes(locator string, content []byte, tags []string, terms []string) (Hit, bool) {
	agg := fileAggregate{counts: make([]int, len(terms)), tags: tags}
	for _, line := range strings.Split(string(content), "\n") {
		agg.scanLine(line, terms)
	}
	evidence := len(tags) > 0
	for _, count := range agg.counts {
		if count > 0 {
			evidence = true
			break
		}
	}
	if !evidence {
		return Hit{}, false
	}
	title := agg.title
	if title == "" {
		title = locator
	}
	excerpts := make([]string, 0, len(agg.best))
	for _, c := range agg.best {
		excerpts = append(excerpts, trimExcerpt(c.text))
	}
	if tags == nil {
		tags = []string{}
	}
	sum := sha256.Sum256(content)
	return Hit{
		Path:       locator,
		Title:      title,
		Excerpts:   excerpts,
		Checksum:   hex.EncodeToString(sum[:]),
		Tags:       tags,
		BodyCounts: agg.counts,
	}, true
}

// headingText reports whether line is an ATX heading — its trimmed form is
// one or more '#'s standing alone or followed by whitespace — and returns
// the heading text with the markers stripped.
func headingText(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	rest := strings.TrimLeft(trimmed, "#")
	if rest == trimmed {
		return "", false
	}
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		// A '#' run flush against text (e.g. "#hashtag") is not a heading.
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// trimExcerpt collapses runs of whitespace in s and caps the result at
// maxExcerptBytes, trimming on a rune boundary so the excerpt stays valid
// UTF-8. It is the single place the excerpt budget is enforced, shared by
// both search paths.
func trimExcerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxExcerptBytes {
		return s
	}
	cut := s[:maxExcerptBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// CarriesEveryTag reports whether an entry's declared tags include every tag in
// want. An empty want matches everything. Matching is exact rather than the
// prefix relation used for ranking: this is a filter, and a caller narrowing to
// "http" is asking for entries about HTTP, not entries about HTTPS as well.
//
// Exported because the knowledge layer enforces the same rule after the merge,
// and the filter must mean exactly one thing on both sides of the interface.
func CarriesEveryTag(have, want []string) bool {
	if len(want) == 0 {
		return true
	}
	if len(have) == 0 {
		return false
	}
	declared := make(map[string]bool, len(have))
	for _, tag := range have {
		declared[tag] = true
	}
	for _, tag := range want {
		if !declared[tag] {
			return false
		}
	}
	return true
}
