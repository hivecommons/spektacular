// Package design addresses the design documents a project declares.
//
// A design document holds the worked design a feature is built to: a settled
// API shape, a user-facing flow, a data format, or a worked example of one.
// Spektacular owns the reference to such a document, not the document itself.
// It resolves a declared source to a directory, reads and writes bytes there,
// and refuses an address it cannot make sense of. It never adds frontmatter,
// reformats content, or imposes a structure on a design document, which is why
// this package exists instead of the shared artifact-file machinery that
// stamps Spektacular's own lifecycle metadata onto everything it writes.
//
// This package deliberately parallels internal/knowledge rather than sharing
// an abstraction with it. Both follow the same four beats (declare, validate,
// resolve, dispatch on provider) over the same store layer, and that is where
// the similarity correctly stops: knowledge carries tiers, a category
// registry, retrieval tiers, tag vocabularies, ranking and de-duplication,
// none of which design documents have or are permitted to have. A reader who
// has read one package can read the other without relearning anything; a
// reader looking for the shared generic version should not expect to find one.
package design

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/store"
)

// Source is one declared design source, resolved and ready to use.
type Source struct {
	// Name is the name the source is addressed by, unique among the
	// project's design sources.
	Name string `json:"name"`
	// Provider names the backend the documents are held in.
	Provider string `json:"provider"`
	// Location is the absolute directory the documents live in, already
	// resolved from the declaration.
	Location string `json:"location"`
}

// Document addresses one design document: the source it belongs to, and its
// path within that source. Both halves are required — a path alone cannot say
// which declared source was searched, so it could never report a useful
// failure.
type Document struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

// resolvedSource is a declared source with its backing store attached.
//
// The reader is always present. The writer is separate, and may be nil: a
// provider that cannot be written to yields a source that refuses a write by
// name rather than a store that violates the documented contract of
// store.Writer. Every provider shipping today can write, so today the file
// provider sets both; the seam is what a read-only remote provider would slot
// into.
type resolvedSource struct {
	name     string
	provider string
	location string
	reader   store.Reader
	writer   store.Writer
}

// Set is the project's declared design sources, resolved once and addressed by
// name thereafter. It holds one entry per declaration, in declaration order.
//
// A Set has no ranking, no tiers, no categories and no de-duplication. Design
// documents are not indexed and not searched, so there is nothing for those to
// act on.
type Set struct {
	sources []resolvedSource
}

// NewSet resolves every design source the project declares.
//
// It is where a declaration becomes something documents can be read from, and
// where the two construction-time refusals live: a provider this build does
// not implement, and a location that does not resolve to a directory. Both are
// refused eagerly, for the same reason the knowledge set does it — a
// misconfiguration surfaces immediately and identically whichever command was
// run, instead of on first use as an empty source.
//
// A relative location resolves from the folder holding config.yaml, the same
// base every other relative path in that file uses. An absolute location is
// used as written. Neither is required to sit inside the project root: a
// design source points at a folder the team already keeps, wherever it keeps
// it, which is precisely why design locations are not part of
// config.Config.storeDirs().
//
// A project that declares no design sources yields an empty Set and no error.
func NewSet(cfg config.Config, projectRoot string) (*Set, error) {
	set := &Set{}
	for _, src := range cfg.Design.Sources {
		switch src.Provider {
		case config.ProviderFile:
			location := src.Config.Location
			base := config.ProjectConfigDir(projectRoot)
			if !filepath.IsAbs(location) {
				location = filepath.Join(base, location)
			}
			info, err := os.Stat(location)
			if err != nil || !info.IsDir() {
				return nil, unreachableSource(src.Name, src.Config.Location, location, base)
			}
			st := store.NewSourceStore(location, "design:"+src.Name)
			set.sources = append(set.sources, resolvedSource{
				name:     src.Name,
				provider: src.Provider,
				location: location,
				reader:   st,
				writer:   st,
			})
		default:
			return nil, unsupportedProvider(src.Name, src.Provider)
		}
	}
	return set, nil
}

// Sources returns every declared source, in declaration order.
func (s *Set) Sources() []Source {
	out := make([]Source, 0, len(s.sources))
	for _, src := range s.sources {
		out = append(out, Source{Name: src.name, Provider: src.provider, Location: src.location})
	}
	return out
}

// names returns the declared source names, for an error that has to tell a
// caller what it could have asked for instead.
func (s *Set) names() []string {
	out := make([]string, 0, len(s.sources))
	for _, src := range s.sources {
		out = append(out, src.name)
	}
	return out
}

// lookup finds a declared source by name.
//
// Every addressed operation goes through here before it touches a store, so an
// unknown source name and a document that is missing from a source that does
// exist are never confused for one another. A name is never resolved on the
// caller's behalf, not even when only one source is declared: predictability
// matters more than convenience to an agent that has to trust the answer.
func (s *Set) lookup(name string) (*resolvedSource, error) {
	if name == "" {
		return nil, incompleteAddress("source", s.names())
	}
	for i := range s.sources {
		if s.sources[i].name == name {
			return &s.sources[i], nil
		}
	}
	return nil, unknownSource(name, s.names())
}

// List returns the documents in one source, or in every source when
// sourceName is empty. Each document carries the source it came from, so a
// fanned-out listing is unambiguous.
//
// Sources are walked in declaration order and paths within a source are
// sorted, so the same project always lists in the same order. A source's
// .spektacular_ignore, if it has one, filters the listing: that comes free
// with the standard store constructor and is the escape hatch when a design
// folder holds material that should not be listed.
func (s *Set) List(sourceName string) ([]Document, error) {
	if sourceName != "" {
		src, err := s.lookup(sourceName)
		if err != nil {
			return nil, err
		}
		return listSource(src)
	}
	var out []Document
	for i := range s.sources {
		docs, err := listSource(&s.sources[i])
		if err != nil {
			return nil, err
		}
		out = append(out, docs...)
	}
	return out, nil
}

// listSource collects one source's documents.
func listSource(src *resolvedSource) ([]Document, error) {
	paths, err := listFiles(src.reader, "")
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make([]Document, 0, len(paths))
	for _, p := range paths {
		out = append(out, Document{Source: src.name, Path: p})
	}
	return out, nil
}

// listFiles walks a reader from dir, returning store-relative file paths.
// store.Reader.List stays one level deep by contract, so the recursion lives
// here, exactly as it does in the knowledge set.
func listFiles(r store.Reader, dir string) ([]string, error) {
	children, err := r.List(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, child := range children {
		childPath := child.Name
		if dir != "" {
			childPath = dir + "/" + child.Name
		}
		if child.IsDir {
			sub, err := listFiles(r, childPath)
			if err != nil {
				return nil, err
			}
			files = append(files, sub...)
			continue
		}
		files = append(files, childPath)
	}
	return files, nil
}

// Read returns one design document's bytes, exactly as they are on disk.
func (s *Set) Read(d Document) ([]byte, error) {
	src, err := s.lookup(d.Source)
	if err != nil {
		return nil, err
	}
	if d.Path == "" {
		return nil, incompleteAddress("path", s.names())
	}
	content, err := src.reader.Read(d.Path)
	if err != nil {
		return nil, notFound(src, d.Path, err)
	}
	return content, nil
}

// Write stores one design document's bytes unchanged.
//
// Nothing is added, removed or reformatted: no frontmatter is stamped and no
// structure is imposed, because the document belongs to the team and not to
// Spektacular. A source whose provider cannot write refuses by name.
func (s *Set) Write(d Document, content []byte) error {
	src, err := s.lookup(d.Source)
	if err != nil {
		return err
	}
	if d.Path == "" {
		return incompleteAddress("path", s.names())
	}
	if src.writer == nil {
		return readOnlySource(src, d.Path)
	}
	return src.writer.Write(d.Path, content)
}

// Resolve returns the absolute path a document addresses, whether or not
// anything is there. It exists so a caller can report the exact location it
// searched without reading the file, which is what makes an unresolved
// reference report useful rather than merely negative.
func (s *Set) Resolve(d Document) (string, error) {
	src, err := s.lookup(d.Source)
	if err != nil {
		return "", err
	}
	if d.Path == "" {
		return "", incompleteAddress("path", s.names())
	}
	return filepath.Join(src.location, filepath.FromSlash(d.Path)), nil
}

// Exists reports whether an addressed document is present in its source.
func (s *Set) Exists(d Document) (bool, error) {
	src, err := s.lookup(d.Source)
	if err != nil {
		return false, err
	}
	if d.Path == "" {
		return false, incompleteAddress("path", s.names())
	}
	return src.reader.Exists(d.Path), nil
}
