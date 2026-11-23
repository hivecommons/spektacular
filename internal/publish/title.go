package publish

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxTitleRunes caps every title publish files. A spec's H1 can come from a
// third party (a recording's or meeting's title, an email subject), so its
// length is bounded here rather than by the tracker. It leaves room for the
// "RFC: [DRAFT] " prefix under GitHub's 256-character and Jira's 255-character
// summary limits.
const MaxTitleRunes = 200

// requirementTitleRunes is the clip a requirement's title takes from its first
// sentence when it has no bold title.
const requirementTitleRunes = 80

var (
	// ansiRE matches a terminal escape sequence (CSI, OSC or a two-byte
	// escape), so `\x1b[31m` goes whole rather than leaving `[31m` behind.
	ansiRE = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\)?|[@-Z\\\\-_])")
	// mentionRE matches an @handle (a user, or an org/team) that is not part
	// of a word, an email address or an existing code span.
	mentionRE = regexp.MustCompile("(^|[^A-Za-z0-9_`])(@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:/[A-Za-z0-9][A-Za-z0-9_.-]*)?)")
)

// isBidiControl reports the Unicode directional overrides and isolates, which can
// make a title read differently from what it says.
func isBidiControl(r rune) bool {
	return (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069') || r == '\u200E' || r == '\u200F'
}

// SanitizeTitle makes a spec-derived string safe to file as an issue title.
// It strips terminal escapes, control and directional-override characters,
// collapses all whitespace (newlines and tabs included) to single spaces,
// wraps a leading slash command and every @mention outside a code span in a
// code span of its own, so no bot runs the command and no one is notified,
// and caps the length at limit runes (MaxTitleRunes when limit is zero or
// less). An ordinary title is returned unchanged, and
// SanitizeTitle(SanitizeTitle(s)) == SanitizeTitle(s).
//
// Code spans are read the simple way: single backticks, paired in order. A
// title whose backticks do not pair that way (an odd count, or a run of two
// or more) cannot be trusted to hide what it seems to, so its backticks
// become apostrophes first.
func SanitizeTitle(s string, limit int) string {
	if limit <= 0 {
		limit = MaxTitleRunes
	}
	s = ansiRE.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r), isBidiControl(r), r == unicode.ReplacementChar:
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if strings.Count(s, "`")%2 == 1 || strings.Contains(s, "``") {
		s = strings.ReplaceAll(s, "`", "'")
	}

	// Wrapping a mention can expose the text after it, which a word
	// character had shielded ("@a@b"), so repeat until nothing changes. Each
	// pass wraps at least one more @, which bounds the loop.
	out := neutralise(s)
	for out != s {
		s, out = out, neutralise(out)
	}
	if utf8.RuneCountInString(out) <= limit {
		return out
	}
	// The clip goes two short of limit: clip appends its ellipsis after a cut
	// with no space to break at, and a span the cut leaves open is closed.
	out = strings.TrimSuffix(clip(out, limit-2), "…")
	if strings.Count(out, "`")%2 == 1 {
		if strings.HasSuffix(out, "`") {
			out = strings.TrimRight(strings.TrimSuffix(out, "`"), " ")
		} else {
			out += "`"
		}
	}
	return out + "…"
}

// neutralise wraps a leading slash command and each @mention outside a code
// span in a code span of its own.
func neutralise(s string) string {
	// Even segments are outside code spans; odd ones are a span's content.
	segs := strings.Split(s, "`")
	for i := 0; i < len(segs); i += 2 {
		seg := segs[i]
		prefix := ""
		if i == 0 && strings.HasPrefix(seg, "/") {
			cmd, rest, found := strings.Cut(seg, " ")
			prefix = "`" + cmd + "`"
			if found {
				prefix += " "
			}
			seg = rest
		}
		seg = prefix + mentionRE.ReplaceAllString(seg, "$1`$2`")
		// A new span must not touch a neighbouring one, or the two backticks
		// would read as one fence.
		if i > 0 && strings.HasPrefix(seg, "`") {
			seg = " " + seg
		}
		if i < len(segs)-1 && strings.HasSuffix(seg, "`") {
			seg += " "
		}
		segs[i] = seg
	}
	return strings.Join(segs, "`")
}

// markdownInline renders a sanitised title inside a markdown body (the RFC's
// children checklist, a child's dependencies, a task's description) so it
// reads as plain text: outside its code spans, < and > become entities and
// [ ] \ are escaped, so a title can neither inject HTML nor make a link, and
// a leading block marker is escaped so it cannot open a heading, quote or
// list.
func markdownInline(title string) string {
	parts := strings.Split(title, "`")
	for i := 0; i < len(parts); i += 2 {
		// Even parts are outside code spans; an unmatched trailing backtick
		// leaves its tail outside too, which escaping only makes safer.
		parts[i] = inlineEscaper.Replace(parts[i])
	}
	out := strings.Join(parts, "`")
	if out != "" && strings.ContainsRune("#>-+*=|", rune(out[0])) {
		out = `\` + out
	}
	return out
}

var inlineEscaper = strings.NewReplacer(`\`, `\\`, "<", "&lt;", ">", "&gt;", "[", `\[`, "]", `\]`)

// codeSpan wraps s in a code span whose fence is longer than any run of
// backticks inside it, so a title carrying code spans of its own stays
// inside one span.
func codeSpan(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	if longest == 0 {
		return "`" + s + "`"
	}
	fence := strings.Repeat("`", longest+1)
	return fence + " " + s + " " + fence
}
