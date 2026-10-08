package metadata

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// hashBaseBody is the reference spec body the BodyHash table cases vary: a
// heading, prose, and task lists using every bullet style at two depths, all
// unticked.
const hashBaseBody = "# Spec\n" +
	"\n" +
	"Some prose.\n" +
	"\n" +
	"## Success Metrics\n" +
	"\n" +
	"- [ ] first\n" +
	"  - [ ] nested\n" +
	"* [ ] star bullet\n" +
	"+ [ ] plus bullet\n"

// TestBodyHash_PinnedValue pins BodyHash against digests computed outside Go
// (sha256sum of "- [ ] a\n" and of empty input), and asserts a ticked box
// hashes to the same pinned value as its unticked twin.
func TestBodyHash_PinnedValue(t *testing.T) {
	const unticked = "sha256:e4020b38aef8b24176ab50b1234944d669af801acfeb2e7209dd07c94a917404"
	require.Equal(t, unticked, BodyHash([]byte("- [ ] a\n")))
	require.Equal(t, unticked, BodyHash([]byte("- [x] a\n")))
	require.Equal(t,
		"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		BodyHash(nil))
}

// TestBodyHash_HasSHA256Prefix asserts every hash carries the "sha256:"
// prefix followed by 64 hex characters.
func TestBodyHash_HasSHA256Prefix(t *testing.T) {
	got := BodyHash([]byte(hashBaseBody))
	require.True(t, strings.HasPrefix(got, "sha256:"), "got %q", got)
	require.Len(t, strings.TrimPrefix(got, "sha256:"), 64)
}

// TestBodyHash_CheckboxOnlyDifferencesHashTheSame asserts bodies that differ
// from the base only in which task-list boxes are ticked — lower or upper
// case, nested, or under any of the -, * and + bullets — hash identically, as
// does the same body with CRLF line endings.
func TestBodyHash_CheckboxOnlyDifferencesHashTheSame(t *testing.T) {
	base := BodyHash([]byte(hashBaseBody))

	tests := []struct {
		name string
		body string
	}{
		{name: "lower x on dash bullet", body: strings.Replace(hashBaseBody, "- [ ] first", "- [x] first", 1)},
		{name: "upper X on dash bullet", body: strings.Replace(hashBaseBody, "- [ ] first", "- [X] first", 1)},
		{name: "nested item", body: strings.Replace(hashBaseBody, "  - [ ] nested", "  - [x] nested", 1)},
		{name: "star bullet", body: strings.Replace(hashBaseBody, "* [ ] star", "* [x] star", 1)},
		{name: "plus bullet", body: strings.Replace(hashBaseBody, "+ [ ] plus", "+ [X] plus", 1)},
		{name: "every box ticked", body: strings.ReplaceAll(hashBaseBody, "[ ]", "[x]")},
		{name: "CRLF line endings", body: strings.ReplaceAll(hashBaseBody, "\n", "\r\n")},
		{name: "CRLF and ticked", body: strings.ReplaceAll(strings.ReplaceAll(hashBaseBody, "[ ]", "[X]"), "\n", "\r\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEqual(t, hashBaseBody, tt.body, "the case must actually change the body")
			require.Equal(t, base, BodyHash([]byte(tt.body)))
		})
	}
}

// TestBodyHash_OtherChangesHashDifferently asserts any change to the body
// other than a checkbox mark — prose text, an added line, a changed heading,
// a changed item's text, or a checkbox removed outright — changes the hash.
func TestBodyHash_OtherChangesHashDifferently(t *testing.T) {
	base := BodyHash([]byte(hashBaseBody))

	tests := []struct {
		name string
		body string
	}{
		{name: "prose text changed", body: strings.Replace(hashBaseBody, "Some prose.", "Other prose.", 1)},
		{name: "line added", body: hashBaseBody + "- [ ] another\n"},
		{name: "heading changed", body: strings.Replace(hashBaseBody, "## Success Metrics", "## Constraints", 1)},
		{name: "item text changed", body: strings.Replace(hashBaseBody, "first", "primary", 1)},
		{name: "checkbox removed", body: strings.Replace(hashBaseBody, "- [ ] first", "- first", 1)},
		{name: "trailing newline dropped", body: strings.TrimSuffix(hashBaseBody, "\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEqual(t, base, BodyHash([]byte(tt.body)))
		})
	}
}

// TestNormaliseCheckboxes asserts only ticked marks at the start of a list
// item are reset and CRLF becomes LF; a bracketed x in prose, in the middle
// of an item, or without a bullet is left untouched.
func TestNormaliseCheckboxes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ticked dash item", in: "- [x] a\n", want: "- [ ] a\n"},
		{name: "upper X", in: "- [X] a\n", want: "- [ ] a\n"},
		{name: "indented star item", in: "    * [x] a\n", want: "    * [ ] a\n"},
		{name: "plus item", in: "+ [X] a\n", want: "+ [ ] a\n"},
		{name: "unticked unchanged", in: "- [ ] a\n", want: "- [ ] a\n"},
		{name: "CRLF normalised", in: "- [x] a\r\nb\r\n", want: "- [ ] a\nb\n"},
		{name: "prose mark untouched", in: "Mark it [x] when done.\n", want: "Mark it [x] when done.\n"},
		{name: "mid-item mark untouched", in: "- item [x] here\n", want: "- item [x] here\n"},
		{name: "no bullet untouched", in: "[x] a\n", want: "[x] a\n"},
		{name: "numbered item untouched", in: "1. [x] a\n", want: "1. [x] a\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, string(NormaliseCheckboxes([]byte(tt.in))))
		})
	}
}
