// Package artifact owns how a feature's documents are addressed. A spec and a
// changelog record are addressed by the feature's bare name, exactly as a
// workflow records it; a plan document by that name plus a document name.
// An address never carries a file extension or a path separator, and says
// nothing about how a store persists the document.
//
// This package is also the only place that knows the file provider persists
// each document as a ".md" file: StorePath maps an address onto that layout
// and NameFromEntry maps a listed entry back to its bare name, so the two can
// never disagree.
package artifact

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Kind names the store a document lives in.
type Kind string

const (
	KindSpec      Kind = "spec"
	KindPlan      Kind = "plan"
	KindChangelog Kind = "changelog"
)

// artifact error codes. Each refusal built from one of these carries a next
// action that restates the caller's command correctly spelled, so a caller can
// rerun it without working out the grammar from the message.
const (
	// ErrCodeUnexpectedExtension is returned when an address segment carries a
	// file extension or a path separator, including a plan written as one
	// joined path such as "feature/plan.md".
	ErrCodeUnexpectedExtension = "unexpected_extension"
	// ErrCodeDocumentRequired is returned when a plan document verb is given a
	// feature but no document name.
	ErrCodeDocumentRequired = "document_required"
)

// docExt is how the file provider persists every document. It is spelled out
// nowhere else.
const docExt = ".md"

// Address is what a caller passes to a document verb and what a list prints.
type Address struct {
	Kind    Kind
	Feature string
	// Document names a plan's document ("plan", "context", "research",
	// "test-plan", ...). It is empty for a spec or changelog record.
	Document string
}

// ExtensionError reports an address segment carrying an extension or a path
// separator. Corrected holds the correctly spelled address; for a plan whose
// document could not be recovered from the input, Corrected.Document is empty.
type ExtensionError struct {
	Input     string
	Corrected Address
}

func (e *ExtensionError) Error() string {
	return fmt.Sprintf("%q carries a file extension or path; %s documents are addressed by bare name", e.Input, e.Corrected.Kind)
}

// DocumentRequiredError reports a plan document verb given only a feature.
type DocumentRequiredError struct {
	Feature string
}

func (e *DocumentRequiredError) Error() string {
	return fmt.Sprintf("plan %q needs a document name, such as plan, context or research", e.Feature)
}

// ErrEmptyName is returned for an empty address segment.
var ErrEmptyName = errors.New("an address segment must not be empty")

// Parse validates the positional arguments of a document verb of kind.
// A spec or changelog verb takes exactly [feature]; a plan document verb takes
// [feature, document]. A plan verb given one argument is refused with
// *DocumentRequiredError, or, when that argument is a joined path, with an
// *ExtensionError whose correction splits it into feature and document.
func Parse(kind Kind, args []string) (Address, error) {
	if kind != KindPlan {
		if len(args) != 1 {
			return Address{}, fmt.Errorf("%s documents take exactly one name, got %d arguments", kind, len(args))
		}
		feature, err := ParseFeature(kind, args[0])
		if err != nil {
			return Address{}, err
		}
		return Address{Kind: kind, Feature: feature}, nil
	}

	switch len(args) {
	case 1:
		in := args[0]
		if hasSeparator(in) {
			parts := segments(in)
			corrected := Address{Kind: KindPlan}
			if n := len(parts); n >= 2 {
				corrected.Feature = stripExt(parts[n-2])
				corrected.Document = stripExt(parts[n-1])
			} else if n == 1 {
				corrected.Feature = stripExt(parts[0])
			}
			return Address{}, &ExtensionError{Input: in, Corrected: corrected}
		}
		if strings.Contains(in, ".") {
			return Address{}, &ExtensionError{Input: in, Corrected: Address{Kind: KindPlan, Feature: stripExt(in)}}
		}
		if in == "" {
			return Address{}, ErrEmptyName
		}
		return Address{}, &DocumentRequiredError{Feature: in}
	case 2:
		feature, document := args[0], args[1]
		if !validSegment(feature) || !validSegment(document) {
			if feature == "" || document == "" {
				return Address{}, ErrEmptyName
			}
			bad := feature
			if validSegment(feature) {
				bad = document
			}
			return Address{}, &ExtensionError{Input: bad, Corrected: Address{
				Kind:     KindPlan,
				Feature:  lastStripped(feature),
				Document: lastStripped(document),
			}}
		}
		return Address{Kind: KindPlan, Feature: feature, Document: document}, nil
	default:
		return Address{}, fmt.Errorf("plan documents take a feature and a document name, got %d arguments", len(args))
	}
}

// ParseFeature validates a single feature segment for kind, as taken by a
// spec or changelog verb and by `plan file list <feature>`.
func ParseFeature(kind Kind, in string) (string, error) {
	if in == "" {
		return "", ErrEmptyName
	}
	if !validSegment(in) {
		return "", &ExtensionError{Input: in, Corrected: Address{Kind: kind, Feature: lastStripped(in)}}
	}
	return in, nil
}

// StorePath is the file provider's location of the addressed document under a
// store directory: <dir>/<feature>.md, or <dir>/<feature>/<document>.md for a
// plan document.
func (a Address) StorePath(dir string) string {
	if a.Kind == KindPlan {
		return FeatureDir(dir, a.Feature) + "/" + a.Document + docExt
	}
	return dir + "/" + a.Feature + docExt
}

// FeatureDir is the folder holding a plan's documents: <dir>/<feature>.
func FeatureDir(dir, feature string) string {
	return dir + "/" + feature
}

// EntryKind says what kind of listed entry is addressable.
type EntryKind int

const (
	// EntryFile is a document file: a spec, a changelog record, or one of a
	// plan's documents.
	EntryFile EntryKind = iota
	// EntryFeatureDir is a plan's feature folder.
	EntryFeatureDir
)

// NameFromEntry turns a listed store entry back into the bare address segment
// that parses to it. ok is false for an entry that is not an addressable
// document of the wanted kind: a folder when files are wanted, a file that is
// not a document, or a name that would not parse.
func NameFromEntry(entryName string, isDir bool, want EntryKind) (name string, ok bool) {
	switch want {
	case EntryFile:
		if isDir || !strings.HasSuffix(entryName, docExt) {
			return "", false
		}
		name = strings.TrimSuffix(entryName, docExt)
	case EntryFeatureDir:
		if !isDir {
			return "", false
		}
		name = entryName
	default:
		return "", false
	}
	if name == "" || !validSegment(name) {
		return "", false
	}
	return name, true
}

// validSegment reports whether s is usable as one address segment: no
// extension dot and no path separator.
func validSegment(s string) bool {
	return s != "" && !strings.Contains(s, ".") && !hasSeparator(s)
}

func hasSeparator(s string) bool {
	return strings.ContainsAny(s, `/\`)
}

// segments splits s on path separators, dropping empty elements.
func segments(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '\\' })
}

// stripExt removes everything from the first "." in s.
func stripExt(s string) string {
	before, _, _ := strings.Cut(s, ".")
	return before
}

// lastStripped is the last path element of s with its extension removed:
// the bare segment a caller most plausibly meant.
func lastStripped(s string) string {
	parts := segments(s)
	if len(parts) == 0 {
		return stripExt(s)
	}
	return stripExt(parts[len(parts)-1])
}

// Location renders storePath, a path relative to its store's root, relative
// to base, the store-relative folder holding the config file that declares
// the store. It is how every document location is reported: in the terms the
// configured directory is written in, never as a host path. An empty base
// means the store is rooted at that folder already.
func Location(base, storePath string) string {
	if base == "" {
		return filepath.ToSlash(filepath.Clean(storePath))
	}
	rel, err := filepath.Rel(base, storePath)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(storePath))
	}
	return filepath.ToSlash(rel)
}
