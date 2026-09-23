package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotFound is returned when the requested file or directory does not exist.
var ErrNotFound = errors.New("not found")

// DirEntry is a typed direct child of a directory returned by List. IsDir lets
// a caller tell a file from a subdirectory and recurse into the tree.
type DirEntry struct {
	Name    string    // child name, not a full path
	IsDir   bool      // true for a subdirectory — recurse into it via List
	ModTime time.Time // last modification time; zero where the backend cannot report it
}

// FileInfo is what Stat reports about one path. It is deliberately narrow:
// timestamps are the only facts a caller has needed from a backend beyond the
// bytes themselves, and keeping the struct small leaves a non-filesystem
// backend little to fake. A backend that cannot report a field leaves it zero,
// so a consumer can tell "unknown" from a real time and never has to guess.
type FileInfo struct {
	ModTime   time.Time // last modification time
	CreatedAt time.Time // creation time; zero where the backend cannot report it
}

// Hit is a generic search result produced by a store's Search. It describes
// one matching document, carrying a locator and compact excerpts, never the
// full file body.
//
// The fields fall into two groups, and the split is load-bearing rather than
// cosmetic. A store fills in what it observed while reading the document. It
// leaves the second group alone, because every field in it is a statement about
// the whole result set — which addressing scheme the caller uses, and how this
// document ranks against documents from other stores — and a single store can
// see neither.
type Hit struct {
	// Populated by the store.
	Path       string   `json:"path"`     // locator, relative to the store root — pass to Read
	Title      string   `json:"title"`    // the document's first heading, or the locator when it has none
	Excerpts   []string `json:"excerpts"` // compact excerpts, each capped at the excerpt budget
	Checksum   string   `json:"checksum"` // hex SHA-256 over the entry's exact raw bytes; the identity key for byte-identical de-dup
	Tags       []string `json:"tags"`     // the entry's declared tags; always a list, empty rather than absent, so a consumer never has to tell "no tags" from "no tag support"
	BodyCounts []int    `json:"-"`        // occurrences of each query term in the document's text, indexed like the terms; the evidence the knowledge layer ranks on

	// Left zero by the store, stamped by the knowledge layer.
	Tier     string  `json:"tier"`     // addressing tier of the originating store
	Name     string  `json:"name"`     // name of the originating store
	Category string  `json:"category"` // the entry's category, derived from the path
	Score    float64 `json:"score"`    // ranking score, computed once from the evidence above so every store's hits share one scale
}

// SearchOptions carries the narrowing a caller wants applied while a store is
// walked. It exists so Search takes one options parameter rather than growing a
// second method each time a narrowing axis is added; its zero value means "no
// narrowing", so every call site keeps its present meaning.
type SearchOptions struct {
	// Tags restricts results to entries carrying every listed tag, matched
	// exactly against the entry's declared tags. Empty means no restriction.
	//
	// Applying this is an optimisation, not a guarantee: skipping a document
	// during the walk is cheaper than reporting one that will be discarded, but
	// the knowledge layer enforces the filter regardless. A store that ignores
	// this field is slower, never wrong — which is what keeps a provider from
	// having to be trusted on it.
	Tags []string
}

// Reader provides read-only access to a project's data directory. It is the
// half of Store a caller needs to read, list, test and search, named
// separately so a source can hold a reader unconditionally and a writer only
// when its provider can supply one. A backend that cannot be written to is
// then a source that refuses writes by name, rather than a Store that
// violates the documented contract of Write and Delete.
//
// All paths are relative to the store root.
type Reader interface {
	// Read returns the contents of the file at path.
	Read(path string) ([]byte, error)
	// List returns the direct children of the directory at path. Each entry
	// reports whether it is a directory, so a caller can recurse the tree.
	List(path string) ([]DirEntry, error)
	// Exists reports whether a file or directory exists at path.
	Exists(path string) bool
	// Stat reports the timestamps of the file or directory at path. Returns
	// ErrNotFound if missing. Fields a backend cannot report are left zero.
	Stat(path string) (FileInfo, error)
	// Search returns hits for a pre-tokenized keyword query, scanning only
	// this store. Terms arrive already lower-cased and in the order the
	// caller indexes evidence by, so an implementation must report per-term
	// results in that same order.
	//
	// An implementation finds candidate documents and describes them; it does
	// not rank them. Hits are left unattributed and unscored: a store has no
	// notion of its caller's addressing scheme, and no notion of the other
	// stores its results will be merged and ranked against, so filling in
	// Tier, Name, Category and Score is the knowledge layer's job.
	//
	// The contract that follows from this: to take part in ranking at all, a
	// store must be able to report BodyCounts — how often each term occurs in
	// a document's text. A backend that can only return its own opaque
	// relevance score cannot rank coherently alongside the others.
	Search(terms []string, opts SearchOptions) ([]Hit, error)
}

// Writer provides the mutating half of Store. All paths are relative to the
// store root.
type Writer interface {
	// Write creates or overwrites the file at path with content.
	// Parent directories are created automatically.
	Write(path string, content []byte) error
	// Delete removes the file at path. Returns nil if the file does not exist.
	Delete(path string) error
}

// Store provides read/write access to a project's data directory.
// All paths are relative to the store root.
type Store interface {
	Reader
	Writer
	// Root returns the absolute path to the store root directory.
	Root() string
}

// FileStore implements Store over the local filesystem.
// All paths are resolved relative to root and must not escape it.
type FileStore struct {
	root  string
	label string
}

// NewFileStore creates a FileStore rooted at root, carrying label. The label is
// an opaque diagnostic tag: the store never interprets it and never puts it on a
// hit. Attributing a hit to where it came from is the caller's job, since the
// generic store layer has no notion of what a knowledge tier or a changelog repo
// is.
func NewFileStore(root, label string) *FileStore {
	return &FileStore{root: filepath.Clean(root), label: label}
}

// Root returns the absolute path to the store root directory.
func (f *FileStore) Root() string {
	return f.root
}

// Label returns the diagnostic label the store was constructed with.
func (f *FileStore) Label() string {
	return f.label
}

// abs resolves a relative path against the root, rejecting path traversal.
func (f *FileStore) abs(path string) (string, error) {
	joined := filepath.Join(f.root, path)
	rel, err := filepath.Rel(f.root, joined)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q escapes store root", path)
	}
	return joined, nil
}

func (f *FileStore) Read(path string) ([]byte, error) {
	abs, err := f.abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return data, err
}

func (f *FileStore) Write(path string, content []byte) error {
	abs, err := f.abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, content, 0644)
}

func (f *FileStore) Delete(path string) error {
	abs, err := f.abs(path)
	if err != nil {
		return err
	}
	err = os.Remove(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (f *FileStore) List(path string) ([]DirEntry, error) {
	abs, err := f.abs(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	result := make([]DirEntry, len(entries))
	for i, e := range entries {
		result[i] = DirEntry{Name: e.Name(), IsDir: e.IsDir()}
		// An entry that vanishes between ReadDir and Info is still listed;
		// it simply carries no timestamp, which is the documented "unknown".
		if info, infoErr := e.Info(); infoErr == nil {
			result[i].ModTime = info.ModTime()
		}
	}
	return result, nil
}

func (f *FileStore) Exists(path string) bool {
	abs, err := f.abs(path)
	if err != nil {
		return false
	}
	_, err = os.Stat(abs)
	return err == nil
}

// Stat reports the filesystem timestamps for path. CreatedAt is left zero:
// birth time is not portable across the platforms Go's os package supports,
// and reporting it on some hosts but not others would make the field a
// platform quirk rather than a contract.
func (f *FileStore) Stat(path string) (FileInfo, error) {
	abs, err := f.abs(path)
	if err != nil {
		return FileInfo{}, err
	}
	info, err := os.Stat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return FileInfo{}, ErrNotFound
	}
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{ModTime: info.ModTime()}, nil
}
