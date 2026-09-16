package store

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// This is deliberately not internal/metadata, despite the surface similarity,
// and consolidating the two would break one of them.
//
// internal/metadata owns the *workflow artifact* schema — specs, plans,
// changelogs — where a parseable created_date is mandatory and an artifact
// lacking it genuinely is malformed; its document status is read leniently. Its renderer
// also round-trips through a fixed shape that silently drops unknown keys. Route
// a knowledge entry through it and a tags-only block is rejected as malformed,
// while any write-back deletes the tags.
//
// A knowledge entry has the opposite schema: every field is optional, and an
// entry with no block at all is the normal case rather than an error — which is
// what lets tags ship with no migration, no backfill and no command to run. The
// shared part is about fifteen lines of block detection, copied from
// internal/metadata/frontmatter.go; the divergent part is the schema and its
// validation, which is the part that matters.
//
// A frontmatter block is a document-format concern, not a knowledge-domain one,
// so reading one here does not breach this package's deliberate
// category-agnosticism.

// ParseEntry splits an optional leading YAML frontmatter block off raw,
// returning the tags the entry declares and the remaining body.
//
// An entry with no leading block yields no tags and raw unchanged, which is not
// an error — that is every entry written before tags existed. Nor is an opener
// that is never closed, or a closing marker that is not alone on its line: those
// read as a horizontal rule in prose, so the entry keeps working and simply has
// no tags. Failing a whole search because one entry has a formatting accident
// would be a far worse outcome than missing a tag.
//
// The one case that does return an error is a block that is properly delimited
// but whose YAML will not unmarshal. The tags and body returned alongside it are
// still safe to use — no tags, and the raw bytes as the body — so a caller that
// only wants to keep searching can ignore the error, while a caller auditing
// entries can report it rather than leaving it invisible forever.
//
// tags is normalised: lower-cased, trimmed, emptied entries dropped, duplicates
// removed, first-seen order preserved. Lower-casing on read is what stops "HTTP"
// and "http" ever existing as two separate tags.
func ParseEntry(raw []byte) (tags []string, body []byte, err error) {
	opener := []byte("---\n")
	if !bytes.HasPrefix(raw, opener) {
		return nil, raw, nil
	}
	rest := raw[len(opener):]

	// The closing delimiter is a line containing only `---`.
	closeIdx := bytes.Index(rest, []byte("\n---"))
	if closeIdx < 0 {
		return nil, raw, nil
	}

	block := rest[:closeIdx]
	afterClose := rest[closeIdx+len("\n---"):]

	var remainder []byte
	switch {
	case len(afterClose) == 0:
		remainder = nil
	case afterClose[0] == '\n':
		remainder = afterClose[1:]
	default:
		return nil, raw, nil
	}

	// Drop a single blank line separating the block from the body, if present.
	if bytes.HasPrefix(remainder, []byte("\n")) {
		remainder = remainder[1:]
	}

	// An anonymous struct with the one key this feature reads, so any other key
	// in the block is ignored rather than rejected. That keeps the block usable
	// for a team's own metadata without this parser having to know about it.
	var doc struct {
		Tags []string `yaml:"tags"`
	}
	if err := yaml.Unmarshal(block, &doc); err != nil {
		return nil, raw, fmt.Errorf("malformed knowledge entry frontmatter: %w", err)
	}

	return normaliseTags(doc.Tags), remainder, nil
}

// normaliseTags lower-cases and trims each tag, drops empty ones, and removes
// duplicates while keeping the order they were first written in. It returns nil
// rather than an empty slice when nothing survives, so "declared no tags" and
// "declared an empty list" are the same thing to every caller.
func normaliseTags(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(raw))
	var tags []string
	for _, tag := range raw {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}
