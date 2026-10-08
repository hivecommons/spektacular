package metadata

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// checkboxMark matches a ticked task-list mark at the start of a list item,
// keeping the indent and bullet so only the mark itself is rewritten.
var checkboxMark = regexp.MustCompile(`(?m)^(\s*[-*+] )\[[xX]\]`)

// NormaliseCheckboxes returns body with line endings normalised to LF and
// every ticked task-list mark (`[x]` or `[X]`) reset to `[ ]`, so two bodies
// that differ only in which boxes are ticked compare equal.
func NormaliseCheckboxes(body []byte) []byte {
	s := strings.ReplaceAll(string(body), "\r\n", "\n")
	return []byte(checkboxMark.ReplaceAllString(s, "${1}[ ]"))
}

// BodyHash returns "sha256:<hex>" of body after NormaliseCheckboxes. It is the
// one definition spec amend records and plan staleness compares, so ticking a
// spec's checkboxes never undoes a recorded amendment.
func BodyHash(body []byte) string {
	sum := sha256.Sum256(NormaliseCheckboxes(body))
	return "sha256:" + hex.EncodeToString(sum[:])
}
