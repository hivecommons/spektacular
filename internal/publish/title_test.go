package publish

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// hostileTitles are titles a third party could put in a spec's H1: a meeting
// or recording title, an email subject, an issue title copied verbatim.
var hostileTitles = map[string]string{
	"slash command":     "/close",
	"slash with args":   "/lgtm cancel and /approve",
	"mention":           "@someone please merge",
	"team mention":      "ask @acme/maintainers to approve",
	"mention in parens": "approved (@reviewer)",
	"newlines":          "Title\n/close\n\n@someone",
	"tabs and CR":       "a\tb\r\nc",
	"ansi":              "\x1b[31mred\x1b[0m alert",
	"osc":               "\x1b]0;pwned\x07title",
	"nul and bell":      "a\x00b\x07c",
	"bidi override":     "safe ‮evil‬ title",
	"script":            "<script>alert(1)</script> title",
	"long":              strings.Repeat("word ", 100),
	"long unbroken":     strings.Repeat("x", 500),
	"long mentions":     strings.Repeat("@a ", 200),
	"markdown link":     "[click](https://example.com) # not a heading",
	"leading heading":   "# Heading",
	"double at":         "@@x",
	"slash mention":     "/@x",
	"backtick":          "a ` b `@c`",
}

// leadingCommand matches a title that starts with a slash command a bot
// would run.
func startsWithSlashCommand(s string) bool { return strings.HasPrefix(s, "/") }

// unquotedMention reports an @handle outside a code span.
func unquotedMention(s string) bool {
	parts := strings.Split(s, "`")
	for i := 0; i < len(parts); i += 2 {
		if mentionRE.MatchString(parts[i]) {
			return true
		}
	}
	return false
}

func requireSafeTitle(t *testing.T, label, got string) {
	t.Helper()
	for _, r := range got {
		require.Falsef(t, unicode.IsControl(r), "%s: title %q carries control character %U", label, got, r)
		require.Falsef(t, isBidiControl(r), "%s: title %q carries directional control %U", label, got, r)
	}
	require.NotContainsf(t, got, "\x1b", "%s: no escape sequence survives", label)
	require.Falsef(t, startsWithSlashCommand(got), "%s: title %q starts with a slash command", label, got)
	require.Falsef(t, unquotedMention(got), "%s: title %q carries a mention outside a code span", label, got)
	require.LessOrEqualf(t, utf8.RuneCountInString(got), MaxTitleRunes, "%s: title is within the cap", label)
	require.Equalf(t, strings.TrimSpace(got), got, "%s: title is trimmed", label)
	require.NotContainsf(t, got, "  ", "%s: whitespace is collapsed", label)
}

func TestSanitizeTitle_HostileTitlesAreNeutralised(t *testing.T) {
	for label, in := range hostileTitles {
		requireSafeTitle(t, label, SanitizeTitle(in, 0))
	}
	require.Equal(t, "`/close`", SanitizeTitle("/close", 0))
	require.Equal(t, "`@someone` please merge", SanitizeTitle("@someone please merge", 0))
	require.Equal(t, "Title /close `@someone`", SanitizeTitle("Title\n/close\n\n@someone", 0),
		"a newline cannot start a second line that a bot reads as a command")
	require.Equal(t, "red alert", SanitizeTitle("\x1b[31mred\x1b[0m alert", 0))
	require.Equal(t, "ask `@acme/maintainers` to approve", SanitizeTitle("ask @acme/maintainers to approve", 0))
	require.Equal(t, "mail ops@example.com", SanitizeTitle("mail ops@example.com", 0), "an email address is not a mention")
}

func TestSanitizeTitle_NormalTitlesAreUnchanged(t *testing.T) {
	for _, in := range []string{
		"RFC: Inbox voice routing",
		"Inbox voice routing",
		"RFC: [DRAFT] Nightly digest email",
		"Support C# and F# projects",
		"Rate limit: 5/min per user",
		"Add `--dry-run` to publish",
		"Ünïcödé títle, with émoji 🛡️ and CJK 标题",
		"v1.2 -> v1.3 migration",
	} {
		require.Equal(t, in, SanitizeTitle(in, 0))
	}
}

func TestSanitizeTitle_IsIdempotent(t *testing.T) {
	inputs := []string{"RFC: Inbox voice routing", ""}
	for _, in := range hostileTitles {
		inputs = append(inputs, in, "RFC: "+in)
	}
	for _, in := range inputs {
		once := SanitizeTitle(in, 0)
		require.Equalf(t, once, SanitizeTitle(once, 0), "sanitising %q twice must equal sanitising it once", in)
		short := SanitizeTitle(in, requirementTitleRunes)
		require.LessOrEqual(t, utf8.RuneCountInString(short), requirementTitleRunes)
		require.Equalf(t, short, SanitizeTitle(short, requirementTitleRunes), "idempotent at a smaller cap too: %q", in)
	}
}

func TestMarkdownInline_TitleCannotInjectMarkupIntoABody(t *testing.T) {
	require.Equal(t, "RFC: Inbox voice routing", markdownInline("RFC: Inbox voice routing"))
	require.Equal(t, "&lt;script&gt;alert(1)&lt;/script&gt;", markdownInline("<script>alert(1)</script>"))
	require.Equal(t, `\[click\](https://example.com)`, markdownInline("[click](https://example.com)"))
	require.Equal(t, `\# Heading`, markdownInline("# Heading"))
	require.Equal(t, "ping `@a<b>`", markdownInline("ping `@a<b>`"), "a code span is left as it is")
}

func TestCodeSpan_HoldsATitleWithBackticks(t *testing.T) {
	require.Equal(t, "`RFC: Inbox voice routing`", codeSpan("RFC: Inbox voice routing"))
	require.Equal(t, "`` RFC: ping `@bob` ``", codeSpan("RFC: ping `@bob`"))
}

// The invariant end to end: a spec whose H1 and requirements are hostile
// publishes titles and bodies with no live command, mention or markup.
func TestBuild_HostileSpecTitlesAreSanitised(t *testing.T) {
	src := specSource(t)
	body := string(src.Body)
	h1 := titleRE.FindString(body)
	require.NotEmpty(t, h1)
	hostile := "# Feature: /close @someone please merge <script>x</script>\x1b[31m " + strings.Repeat("y", 500)
	body = strings.Replace(body, h1, hostile, 1)
	body = strings.Replace(body, "## Requirements\n\n", "## Requirements\n\n- [ ] @someone /approve this <b>now</b>. More detail follows here.\n", 1)
	src.Body = []byte(body)

	p, err := Build(src)
	require.NoError(t, err)

	requireSafeTitle(t, "RFC", p.RFC.Title)
	require.Equal(t, "`@someone` /approve this <b>now</b>", p.Children[0].Title, "a requirement title is sanitised too")
	require.True(t, strings.HasPrefix(p.RFC.Title, "RFC: `/close` `@someone` please merge"), p.RFC.Title)
	for _, c := range p.Children {
		requireSafeTitle(t, "child "+c.ID, c.Title)
		for _, line := range strings.Split(c.Body, "\n") {
			require.Falsef(t, startsWithSlashCommand(strings.TrimSpace(line)), "child %s body line %q starts with a slash command", c.ID, line)
		}
		require.Contains(t, c.Body, "\n- Part of RFC: `` RFC: `/close` `@someone` please merge <script>",
			"the RFC title is quoted whole in one code span in the child body")
	}
	_, children, found := strings.Cut(p.RFC.Body, "\n## Children\n")
	require.True(t, found)
	for _, line := range strings.Split(children, "\n") {
		if strings.HasPrefix(line, "- [ ] ") {
			require.Falsef(t, unquotedMention(line), "checklist line %q carries a live mention", line)
			require.NotContains(t, line, "<b>")
		}
	}
}

// FuzzSanitizeTitle holds the invariant for any input: the result is safe and
// a second pass changes nothing. `go test` runs the seeds; -fuzz explores.
func FuzzSanitizeTitle(f *testing.F) {
	for _, in := range hostileTitles {
		f.Add(in)
	}
	f.Add("RFC: Inbox voice routing")
	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			t.Skip()
		}
		once := SanitizeTitle(in, 0)
		requireSafeTitle(t, "fuzz", once)
		require.Equal(t, once, SanitizeTitle(once, 0))
	})
}
