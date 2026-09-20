package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Settings format versions. Each settings kind carries its own integer
// format version in a top-level `schema` key; it rises by one only when that
// file's format changes, and every rise ships as a registered upgrade step in
// the migrate package. A file with no `schema` key is format 1.
const (
	// CurrentProjectSchema is the config.yaml format this build reads and
	// writes. Format 3 resolves the store folders from the settings file.
	CurrentProjectSchema = 3
	// CurrentRepoSchema is the repo.yaml format this build reads and writes.
	CurrentRepoSchema = 2
)

// WriterVersion is the Spektacular version stamped into every settings file
// as `written_by`. cmd sets it from its build version at start-up; it is
// informational only and never drives an upgrade.
var WriterVersion = "unknown"

// NormaliseSchema maps a settings file's raw `schema` value to its format
// version: a missing or zero value is the oldest format, 1.
func NormaliseSchema(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

// PeekSchema reads only the `schema` key of the settings file at path and
// returns its normalised format version, without decoding or validating the
// rest of the file.
func PeekSchema(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("reading settings file %s: %w", path, err)
	}
	var head struct {
		Schema int `yaml:"schema"`
	}
	if err := yaml.Unmarshal(raw, &head); err != nil {
		return 0, fmt.Errorf("parsing settings file %s: %w", path, err)
	}
	return NormaliseSchema(head.Schema), nil
}

// FormatError reports a settings file whose format version is not the one
// this build reads. It lets callers tell a format refusal apart from a
// malformed file, so a file written by a newer Spektacular is never treated
// as broken and overwritten.
type FormatError struct {
	Path  string
	Kind  string // "project" | "repo"
	Found int    // normalised: a missing schema is 1
	Want  int
}

func (e *FormatError) Error() string {
	if e.Newer() {
		return fmt.Sprintf("%s settings file %s is format %d, newer than this Spektacular supports (format %d)", e.Kind, e.Path, e.Found, e.Want)
	}
	return fmt.Sprintf("%s settings file %s is format %d; this Spektacular needs format %d", e.Kind, e.Path, e.Found, e.Want)
}

// Newer reports whether the file was written in a format newer than this
// build supports.
func (e *FormatError) Newer() bool { return e.Found > e.Want }

// IsFormatError reports whether err is, or wraps, a *FormatError.
func IsFormatError(err error) (*FormatError, bool) {
	var fe *FormatError
	if errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}

// checkSchema refuses settings content that is not at format want. A
// content that does not parse is left for the full decode to report.
func checkSchema(content, path, kind string, want int) error {
	var head struct {
		Schema int `yaml:"schema"`
	}
	if err := yaml.Unmarshal([]byte(content), &head); err != nil {
		return nil
	}
	if n := NormaliseSchema(head.Schema); n != want {
		return &FormatError{Path: path, Kind: kind, Found: n, Want: want}
	}
	return nil
}
