package artifact

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAcceptsBareAddresses(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		args []string
		want Address
	}{
		{"spec", KindSpec, []string{"f"}, Address{Kind: KindSpec, Feature: "f"}},
		{"changelog", KindChangelog, []string{"f"}, Address{Kind: KindChangelog, Feature: "f"}},
		{"plan document", KindPlan, []string{"f", "plan"}, Address{Kind: KindPlan, Feature: "f", Document: "plan"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.kind, tc.args)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestStorePathMatchesWorkflowLayout(t *testing.T) {
	cases := []struct {
		name string
		addr Address
		dir  string
		want string
	}{
		{"spec", Address{KindSpec, "f", ""}, ".spektacular/specs", ".spektacular/specs/f.md"},
		{"changelog", Address{KindChangelog, "f", ""}, ".spektacular/changelog", ".spektacular/changelog/f.md"},
		{"plan", Address{KindPlan, "f", "plan"}, ".spektacular/plans", ".spektacular/plans/f/plan.md"},
		{"test plan", Address{KindPlan, "f", "test-plan"}, ".spektacular/plans", ".spektacular/plans/f/test-plan.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.addr.StorePath(tc.dir))
		})
	}
}

func TestFeatureDir(t *testing.T) {
	require.Equal(t, ".spektacular/plans/f", FeatureDir(".spektacular/plans", "f"))
}

func TestParseRefusesExtensionsWithCorrection(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		args []string
		want Address
	}{
		{"spec md", KindSpec, []string{"x.md"}, Address{Kind: KindSpec, Feature: "x"}},
		{"spec markdown", KindSpec, []string{"x.markdown"}, Address{Kind: KindSpec, Feature: "x"}},
		{"spec path", KindSpec, []string{"specs/x.md"}, Address{Kind: KindSpec, Feature: "x"}},
		{"changelog md", KindChangelog, []string{"x.md"}, Address{Kind: KindChangelog, Feature: "x"}},
		{"plan joined path with ext", KindPlan, []string{"x/plan.md"}, Address{Kind: KindPlan, Feature: "x", Document: "plan"}},
		{"plan joined path", KindPlan, []string{"x/plan"}, Address{Kind: KindPlan, Feature: "x", Document: "plan"}},
		{"plan single ext", KindPlan, []string{"x.md"}, Address{Kind: KindPlan, Feature: "x"}},
		{"plan feature ext", KindPlan, []string{"x.md", "plan"}, Address{Kind: KindPlan, Feature: "x", Document: "plan"}},
		{"plan document ext", KindPlan, []string{"x", "plan.md"}, Address{Kind: KindPlan, Feature: "x", Document: "plan"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.kind, tc.args)
			var extErr *ExtensionError
			require.True(t, errors.As(err, &extErr), "want *ExtensionError, got %v", err)
			require.Equal(t, tc.want, extErr.Corrected)
		})
	}
}

func TestParseFeatureRefusesExtension(t *testing.T) {
	_, err := ParseFeature(KindPlan, "x.md")
	var extErr *ExtensionError
	require.True(t, errors.As(err, &extErr), "want *ExtensionError, got %v", err)
	require.Equal(t, Address{Kind: KindPlan, Feature: "x"}, extErr.Corrected)
}

func TestParsePlanRequiresDocument(t *testing.T) {
	_, err := Parse(KindPlan, []string{"x"})
	var docErr *DocumentRequiredError
	require.True(t, errors.As(err, &docErr), "want *DocumentRequiredError, got %v", err)
	require.Equal(t, DocumentRequiredError{Feature: "x"}, *docErr)
}

func TestParseRefusesEmptySegment(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		args []string
	}{
		{"spec", KindSpec, []string{""}},
		{"changelog", KindChangelog, []string{""}},
		{"plan single", KindPlan, []string{""}},
		{"plan feature", KindPlan, []string{"", "plan"}},
		{"plan document", KindPlan, []string{"x", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.kind, tc.args)
			require.ErrorIs(t, err, ErrEmptyName)
		})
	}
	_, err := ParseFeature(KindSpec, "")
	require.ErrorIs(t, err, ErrEmptyName)
}

func TestParseRefusesWrongArity(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		args []string
	}{
		{"spec none", KindSpec, nil},
		{"spec two", KindSpec, []string{"a", "b"}},
		{"changelog two", KindChangelog, []string{"a", "b"}},
		{"plan none", KindPlan, nil},
		{"plan three", KindPlan, []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.kind, tc.args)
			require.Error(t, err)
		})
	}
}

func TestNameFromEntry(t *testing.T) {
	cases := []struct {
		name     string
		entry    string
		isDir    bool
		want     EntryKind
		wantName string
		wantOK   bool
	}{
		{"document file", "f.md", false, EntryFile, "f", true},
		{"dir when files wanted", "f.md", true, EntryFile, "", false},
		{"non-document file", "notes.txt", false, EntryFile, "", false},
		{"dotted document name", "a.b.md", false, EntryFile, "", false},
		{"feature dir", "f", true, EntryFeatureDir, "f", true},
		{"file when dirs wanted", "f", false, EntryFeatureDir, "", false},
		{"dotted dir", ".git", true, EntryFeatureDir, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NameFromEntry(tc.entry, tc.isDir, tc.want)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantName, got)
		})
	}
}

func TestNameFromEntryRoundTripsThroughParse(t *testing.T) {
	name, ok := NameFromEntry("f.md", false, EntryFile)
	require.True(t, ok)
	addr, err := Parse(KindSpec, []string{name})
	require.NoError(t, err)
	require.Equal(t, "f", addr.Feature)
}

func TestErrorCodes(t *testing.T) {
	require.Equal(t, "unexpected_extension", ErrCodeUnexpectedExtension)
	require.Equal(t, "document_required", ErrCodeDocumentRequired)
}
