package templates

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// dataFlagOpener is the opening of a shell-ready `--data` example as every
// template writes it: the flag, a space, a single quote, then the JSON object.
// An example that opens this way must close with `}'` on the same line —
// anything else is a command an agent cannot copy and run.
const dataFlagOpener = "--data '{"

// TestDataPayloadExamplesAreWellFormed walks every template and checks each
// single-line `--data '{…}'` example is shell-ready: the JSON object's braces
// balance and the payload is closed by the matching single quote before the
// end of the line.
//
// The failure this catches is silent and easy to introduce by hand — a
// payload opened with `--data '{` and closed with `}"`, which every
// phrase-presence contract test happily accepts and which an agent copying
// the example verbatim would issue as a broken command.
//
// Brace counting (rather than a search for the first `}`) is what makes this
// work against the corpus, because mustache placeholders nest inside the
// payloads: `--data '{"step":"{{next_step}}"}'` opens three braces and closes
// three.
//
// Scope: single-line examples only. Several templates wrap a long invocation
// across two lines, but every one of them breaks the line *after* `--data`,
// so the payload itself is always intact on one line and there is nothing to
// stitch together. A future example that wrapped mid-payload would be
// reported here as unclosed, which is the right prompt to unwrap it.
// Templates that mention `--data` in prose with no payload carry no opener
// and are skipped.
func TestDataPayloadExamplesAreWellFormed(t *testing.T) {
	checked := 0

	err := fs.WalkDir(FS, ".", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || path.Ext(p) != ".md" {
			return nil
		}

		for i, line := range strings.Split(mustReadTemplate(t, p), "\n") {
			lineNo := i + 1
			for _, payload := range dataPayloads(line) {
				checked++
				require.Truef(t, payload.closed,
					"%s:%d: `--data` payload is not closed by `}'` before the end of the line — offending text: %s",
					p, lineNo, payload.text)
			}
		}
		return nil
	})
	require.NoError(t, err, "walking the template tree")

	require.GreaterOrEqual(t, checked, 70,
		"expected the template tree to carry at least 70 `--data` payload examples; a much lower count means the walk stopped finding them")
}

// dataPayload is one `--data '{…` occurrence on a single line, with whether
// its JSON object closed with the matching `}'` on that same line, and the
// text to quote back when it did not.
type dataPayload struct {
	closed bool
	text   string
}

// dataPayloads finds every `--data '{` opener on one line and reports, for
// each, whether the JSON object it opens is closed by `}'` on that line.
func dataPayloads(line string) []dataPayload {
	var found []dataPayload

	for searched := 0; ; {
		rel := strings.Index(line[searched:], dataFlagOpener)
		if rel < 0 {
			return found
		}
		start := searched + rel
		// Position of the `{` that opens the JSON object.
		open := start + len(dataFlagOpener) - 1
		searched = open + 1

		found = append(found, dataPayload{
			closed: closesWithQuote(line[open:]),
			text:   line[start:],
		})
	}
}

// closesWithQuote reports whether the brace-balanced object starting at s[0]
// (which is `{`) is immediately followed by the closing single quote of the
// shell argument.
func closesWithQuote(s string) bool {
	depth := 0
	for i, c := range s {
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i+1 < len(s) && s[i+1] == '\''
			}
		}
	}
	return false
}
