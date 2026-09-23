package store

import (
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// IgnoreFileName is the per-source exclusion file, rooted at the source it
// governs. It uses gitignore pattern syntax and binds only Spektacular's own
// listing and search — never direct access to an explicitly named path, and
// never agents' native file tools.
const IgnoreFileName = ".spektacular_ignore"

// IgnoreMatcher reports whether a store-relative path is excluded from
// listing and search results.
type IgnoreMatcher interface {
	Ignored(relPath string, isDir bool) bool
}

// passMatcher ignores nothing — the matcher for a source with no ignore file.
type passMatcher struct{}

func (passMatcher) Ignored(string, bool) bool { return false }

// gitignoreMatcher adapts the compiled gitignore patterns to IgnoreMatcher.
type gitignoreMatcher struct {
	compiled *gitignore.GitIgnore
}

func (m gitignoreMatcher) Ignored(relPath string, isDir bool) bool {
	p := filepath.ToSlash(relPath)
	if m.compiled.MatchesPath(p) {
		return true
	}
	// Directory patterns like `build/` match the directory itself only with
	// the trailing slash present, so directories are checked both ways.
	if isDir && m.compiled.MatchesPath(p+"/") {
		return true
	}
	return false
}

// LoadIgnore reads root's .spektacular_ignore and compiles it into a
// matcher. A missing (or unreadable) ignore file yields a matcher that
// passes everything, so sources without exclusions behave exactly as before.
func LoadIgnore(root string) IgnoreMatcher {
	raw, err := os.ReadFile(filepath.Join(root, IgnoreFileName))
	if err != nil {
		return passMatcher{}
	}
	compiled := gitignore.CompileIgnoreLines(strings.Split(string(raw), "\n")...)
	return gitignoreMatcher{compiled: compiled}
}

// ignoreStore is a decorator over any Store that filters listing and search
// through an IgnoreMatcher. Read, Write, Exists, Stat, Delete, and Root delegate
// untouched: a directly named path is never blocked by an exclusion rule.
type ignoreStore struct {
	inner   Store
	matcher IgnoreMatcher
}

// NewIgnoreStore wraps inner so List and Search exclude paths the matcher
// ignores. The underlying store stays domain-agnostic — exclusion policy
// lives entirely in this wrapper, and construction sites opt in.
func NewIgnoreStore(inner Store, m IgnoreMatcher) Store {
	return &ignoreStore{inner: inner, matcher: m}
}

// NewSourceStore builds the standard store for a source directory: a
// FileStore rooted at root, wrapped with the exclusions declared by the
// root's own .spektacular_ignore (a pass-through when none exists). The label
// is an opaque diagnostic tag; the store never interprets it.
func NewSourceStore(root, label string) Store {
	return NewIgnoreStore(NewFileStore(root, label), LoadIgnore(root))
}

func (s *ignoreStore) Root() string { return s.inner.Root() }

func (s *ignoreStore) Read(path string) ([]byte, error) { return s.inner.Read(path) }

func (s *ignoreStore) Write(path string, content []byte) error { return s.inner.Write(path, content) }

func (s *ignoreStore) Delete(path string) error { return s.inner.Delete(path) }

func (s *ignoreStore) Exists(path string) bool { return s.inner.Exists(path) }

func (s *ignoreStore) Stat(path string) (FileInfo, error) { return s.inner.Stat(path) }

func (s *ignoreStore) List(path string) ([]DirEntry, error) {
	entries, err := s.inner.List(path)
	if err != nil {
		return nil, err
	}
	filtered := entries[:0]
	for _, e := range entries {
		rel := filepath.ToSlash(filepath.Join(path, e.Name))
		if s.matcher.Ignored(rel, e.IsDir) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered, nil
}

// Search forwards the query and its options to the wrapped store unchanged —
// dropping an option here would make it appear to work everywhere except the
// stores that actually carry an ignore file — and then removes hits on ignored
// paths.
func (s *ignoreStore) Search(terms []string, opts SearchOptions) ([]Hit, error) {
	hits, err := s.inner.Search(terms, opts)
	if err != nil {
		return nil, err
	}
	filtered := hits[:0]
	for _, h := range hits {
		if s.matcher.Ignored(h.Path, false) {
			continue
		}
		filtered = append(filtered, h)
	}
	return filtered, nil
}
