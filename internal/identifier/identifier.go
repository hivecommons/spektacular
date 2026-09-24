// Package identifier is the single home for every ID-related rule this
// project uses: normalizing user-supplied name/id input, minting a new
// canonical ID (timestamp, counter, or externally supplied) for a document
// being created, and checking whether a name already carries an ID matching
// a configured scheme. Nothing outside this package should re-implement or
// duplicate any part of these rules — callers that mint IDs (the spec
// workflow) and callers that only need to verify one is present (the
// generic store-file command builder shared by plan/changelog writes) both
// depend on this package instead of on each other.
package identifier

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/store"
)

const (
	MethodTimestamp = config.SpecIDMethodTimestamp
	MethodCounter   = config.SpecIDMethodCounter
	MethodExternal  = config.SpecIDMethodExternal

	MaxPartLength = 64
)

var (
	counterPrefixRE   = regexp.MustCompile(`^(\d{6})_`)
	timestampPrefixRE = regexp.MustCompile(`^(\d{14})-`)
)

// PathFunc maps a configured directory and a resolved document name to the
// store-relative path used for existence checks during ID resolution (e.g.
// spec.SpecFilePath). Resolution never reads or writes document content —
// only Exists — so callers only need to supply the path shape.
type PathFunc func(dir, name string) string

// Request describes the data needed to resolve a canonical document name.
type Request struct {
	Name     string
	ID       string
	Method   string
	Dir      string // configured store directory; defaults to config.DefaultSpecDir when empty
	Store    store.Store
	PathFunc PathFunc // required for generated (non-external) methods; builds the path Store.Exists checks
	Now      func() time.Time
	RandomID func() (string, error)
}

// Result is the canonical, ID-prefixed document name.
type Result struct {
	Name string
}

// Resolve turns a requested name plus optional id into a canonical,
// ID-prefixed name, minting a new ID via the configured method when none is
// supplied explicitly.
func Resolve(req Request) (Result, error) {
	name, err := NormalizePart("name", req.Name)
	if err != nil {
		return Result{}, err
	}

	method := req.Method
	if method == "" {
		method = MethodTimestamp
	}
	if err := validateMethod(method); err != nil {
		return Result{}, err
	}

	dir := req.Dir
	if dir == "" {
		dir = config.DefaultSpecDir
	}

	if req.ID != "" {
		id, err := NormalizePart("id", req.ID)
		if err != nil {
			return Result{}, err
		}
		// An explicit ID only fits the external scheme: under a generated
		// method it would produce a name HasPrefix rejects, so every later
		// ID-prefixed write for the document would fail.
		if method != MethodExternal {
			return Result{}, &IDNotAllowedError{Method: method}
		}
		resolved, err := resolveWithPrefix(req.Store, req.PathFunc, dir, id, name)
		if err != nil {
			return Result{}, err
		}
		return Result{Name: resolved}, nil
	}

	switch method {
	case MethodExternal:
		return Result{}, fmt.Errorf("id is required when id_method is %q", MethodExternal)
	case MethodTimestamp:
		return resolveTimestamp(req, dir, name)
	case MethodCounter:
		return resolveCounter(req, dir, name)
	default:
		return Result{}, &UnsupportedMethodError{Method: method}
	}
}

// NormalizePart normalizes one user-provided name/id component.
func NormalizePart(label, raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if len(raw) > MaxPartLength {
		return "", fmt.Errorf("%s must be at most %d characters", label, MaxPartLength)
	}
	if raw != strings.TrimSpace(raw) {
		return "", fmt.Errorf("%s must not have leading or trailing whitespace", label)
	}

	var b strings.Builder
	lastHyphen := false
	for _, r := range raw {
		switch {
		case r == '/' || r == '\\':
			return "", fmt.Errorf("%s must not contain path separators", label)
		case unicode.IsControl(r):
			return "", fmt.Errorf("%s must not contain control characters", label)
		case isASCIIAlnum(r):
			b.WriteRune(toASCIILower(r))
			lastHyphen = false
		case r == '_':
			b.WriteRune(r)
			lastHyphen = false
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		default:
			return "", fmt.Errorf("%s contains unsupported character %q", label, r)
		}
	}

	out := b.String()
	if out == "" {
		return "", fmt.Errorf("%s normalizes to empty", label)
	}
	return out, nil
}

// HasPrefix reports whether name begins with an ID matching the given
// id_method scheme. The timestamp method accepts both timestamp-generated
// names and legacy counter-generated names so projects can migrate away from
// branch-local counters without renaming historical artifacts.
// MethodExternal has no fixed shape to check mechanically, so it always
// reports true — external IDs are the caller's responsibility. An empty
// method defaults to MethodTimestamp, matching Resolve's own default.
func HasPrefix(method, name string) bool {
	if method == "" {
		method = MethodTimestamp
	}
	switch method {
	case MethodCounter:
		return counterPrefixRE.MatchString(name)
	case MethodTimestamp:
		return timestampPrefixRE.MatchString(name) || counterPrefixRE.MatchString(name)
	case MethodExternal:
		return true
	default:
		return false
	}
}

// UnsupportedMethodError reports an id_method value outside the three
// supported schemes. It carries the offending value so a caller embedding
// this package's rules behind its own config field (e.g. spec.id_method)
// can re-render the message with that field's name.
type UnsupportedMethodError struct {
	Method string
}

func (e *UnsupportedMethodError) Error() string {
	return fmt.Sprintf("unsupported id_method %q", e.Method)
}

// IDNotAllowedError reports an explicit ID supplied while a generated
// id_method (timestamp or counter) is configured. It carries the method so a
// caller can render the message against its own config field.
type IDNotAllowedError struct {
	Method string
}

func (e *IDNotAllowedError) Error() string {
	return fmt.Sprintf("an explicit id is only accepted when id_method is %q, not %q", MethodExternal, e.Method)
}

func validateMethod(method string) error {
	switch method {
	case MethodTimestamp, MethodCounter, MethodExternal:
		return nil
	default:
		return &UnsupportedMethodError{Method: method}
	}
}

func resolveTimestamp(req Request, dir, name string) (Result, error) {
	now := req.Now
	if now == nil {
		now = time.Now
	}
	randomID := req.RandomID
	if randomID == nil {
		randomID = randomHexID
	}

	timestamp := now().UTC()
	for {
		suffix, err := randomID()
		if err != nil {
			return Result{}, err
		}
		resolved := fmt.Sprintf("%s-%s-%s", timestamp.Format("20060102150405"), suffix, name)
		exists, err := documentExists(req.Store, req.PathFunc, dir, resolved)
		if err != nil {
			return Result{}, err
		}
		if !exists {
			return Result{Name: resolved}, nil
		}
		timestamp = timestamp.Add(time.Second)
	}
}

func randomHexID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating timestamp id suffix: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func resolveCounter(req Request, dir, name string) (Result, error) {
	next, err := nextCounterFromStore(req.Store, dir)
	if err != nil {
		return Result{}, err
	}
	for {
		resolved := fmt.Sprintf("%06d_%s", next, name)
		exists, err := documentExists(req.Store, req.PathFunc, dir, resolved)
		if err != nil {
			return Result{}, err
		}
		if !exists {
			return Result{Name: resolved}, nil
		}
		next++
	}
}

// nextCounterFromStore scans dir for filenames whose names start with
// `<digits>_` and returns max+1. Returns 1 when no such files exist (or the
// directory does not yet exist).
func nextCounterFromStore(st store.Store, dir string) (int, error) {
	if st == nil {
		return 0, fmt.Errorf("store required for identifier resolution")
	}
	entries, err := st.List(dir)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return 1, nil
		}
		return 0, err
	}
	max := 0
	for _, e := range entries {
		base := strings.TrimSuffix(e.Name, ".md")
		m := counterPrefixRE.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil
}

func resolveWithPrefix(st store.Store, pathFn PathFunc, dir, prefix, name string) (string, error) {
	resolved := fmt.Sprintf("%s-%s", prefix, name)
	exists, err := documentExists(st, pathFn, dir, resolved)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("document %q already exists", resolved)
	}
	return resolved, nil
}

func documentExists(st store.Store, pathFn PathFunc, dir, name string) (bool, error) {
	if st == nil {
		return false, fmt.Errorf("store required for identifier resolution")
	}
	if pathFn == nil {
		return false, fmt.Errorf("path function required for identifier resolution")
	}
	return st.Exists(pathFn(dir, name)), nil
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func toASCIILower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
