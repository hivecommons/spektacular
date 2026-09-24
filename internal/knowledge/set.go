// Package knowledge provides a multi-source knowledge layer over the store
// abstraction. A Set is an ordered collection of addressed stores; it fans
// List, Search, and the always-applied reader across its members and tags every
// result with the tier and store name it came from, while Read and Write
// resolve to exactly one store or refuse.
package knowledge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
)

// scopedStore pairs a configured store with the address it answers to and the
// metadata of the source that produced it. The tier and name together are the
// store's identity: they are what a request addresses and what every result
// reports.
type scopedStore struct {
	tier     Tier
	name     string
	provider string
	location string
	store    store.Store
}

// Set is an ordered collection of addressed knowledge stores.
type Set struct {
	sources []scopedStore
}

// Entry is a single knowledge entry, tagged with the store it lives in. The
// triple {tier, name, path} is exactly what a read requires, so any entry can be
// fetched back from what it carries.
type Entry struct {
	Tier Tier   `json:"tier"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Convention is a single always-apply convention, tagged with the store it
// lives in and carrying its full body. Unlike Entry it includes Content,
// because the convention reader returns every convention's text in one call.
type Convention struct {
	Tier    Tier   `json:"tier"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// AlwaysAppliedEntry is a single entry from an always-applied category, tagged
// with the store it lives in, its category, and carrying its full body. The
// Category field lets a consumer tell a convention from a glossary term when
// the always-applied reader returns more than one category at once.
type AlwaysAppliedEntry struct {
	Tier     Tier   `json:"tier"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Category string `json:"category"`
}

// SourceInfo describes one configured knowledge store by the address it
// answers to. The tier and name together are the store's identity, so there is
// no separate repo attribution: a repo-tier store's name is its repo's registry
// name.
type SourceInfo struct {
	Tier     Tier   `json:"tier"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Location string `json:"location"`
}

// NewSet resolves the configured knowledge sources into live, addressed
// stores. A project declaring no shared stores is valid and simply yields an
// empty project tier; nothing is synthesised, since a repo's own store is the
// repo's to declare.
//
// A relative source location is resolved from the folder holding config.yaml
// (see config.ProjectConfigDir) — from the file that declares it — which is
// the same base a repos entry uses, so one rule covers every relative path a
// project config can carry. Repo-tier sources arrive here already absolute,
// resolved against their own repo's root by the caller.
//
// NewSet fails fast: if any source names an unknown provider or points at an
// unreachable location it returns an error naming that store and no Set.
func NewSet(cfg config.Config, projectRoot string) (*Set, error) {
	set := &Set{}
	for _, src := range cfg.Knowledge.Sources {
		tier, name := addressOf(src)
		switch src.Provider {
		case config.ProviderFile:
			location := src.Config.Location
			if !filepath.IsAbs(location) {
				location = filepath.Join(config.ProjectConfigDir(projectRoot), location)
			}
			info, err := os.Stat(location)
			if err != nil || !info.IsDir() {
				return nil, unreachableStore(tier, name, src.Config.Location, location, projectRoot)
			}
			set.sources = append(set.sources, scopedStore{
				tier:     tier,
				name:     name,
				provider: src.Provider,
				location: location,
				store:    store.NewSourceStore(location, string(tier)+":"+name),
			})
		default:
			return nil, fmt.Errorf("knowledge store %q in the %q tier: provider %q is not supported", name, tier, src.Provider)
		}
	}
	return set, nil
}

// addressOf returns the address a source was stamped with during aggregation.
// A source that reaches NewSet unstamped — the knowledge package's own tests
// and any direct caller that builds a config by hand — falls back to the
// project tier, which is the tier a source declared in the project's own config
// belongs to anyway, so a single-tier project needs no aggregator.
func addressOf(src config.SourceConfig) (Tier, string) {
	tier := Tier(src.Tier)
	if tier == "" {
		tier = TierProject
	}
	return tier, src.Name
}

// Search runs the query against every store the selector covers and returns the
// merged hits ranked globally: score descending, with ties broken by configured
// store order and then by path, so the returned slice is the display order and
// is deterministic across runs. A store the selector does not cover is never
// queried at all. If any store errors, Search returns an error naming it and no
// results.
//
// Matching is a ranked OR, not a boolean AND: a document does not have to
// contain every word of the query. It is returned if it carries evidence for
// any of them, ranked on how much of the query it covers and how strongly. A
// caller must therefore not read an empty result as proof that nothing on the
// subject exists, nor a returned hit as proof that every term appeared in it.
//
// Selector.Tags narrows which entries can be returned at all: an entry lacking
// any listed tag is never returned, however well it would otherwise score.
// That is why it is a filter and not another scoring factor.
//
// Weak hits are then dropped relative to the strongest hit in the merged set,
// so a loosely related entry is returned when it is the only thing there and
// disappears once something genuinely relevant is present. The cutoff is
// applied after the merge, deliberately: the strongest hit may live in a
// different store from the weak one being judged.
//
// The query is tokenized here rather than by each store, and each store's hits
// are scored here rather than by the store that produced them. Both belong to
// this layer because both are decisions about the whole result set: the stores
// and this function must index per-term evidence in the same order, and hits
// from different stores are ranked against each other, so they must be scored
// on one scale by one formula. See ranking.go.
func (s *Set) Search(query string, sel Selector) ([]store.Hit, error) {
	if err := s.validateSelector(sel); err != nil {
		return nil, err
	}
	terms := Terms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	type rankedHit struct {
		hit    store.Hit
		source int // index of the originating store, the first tie-break
	}
	var merged []rankedHit
	for i, src := range s.sources {
		if !sel.covers(src) {
			continue
		}
		h, err := src.store.Search(terms, store.SearchOptions{Tags: sel.Tags})
		if err != nil {
			return nil, fmt.Errorf("searching knowledge store %q: %w", src.name, err)
		}
		for _, hit := range h {
			// Stamp the score before the sort, so every hit in the merge is on
			// the one scale regardless of which provider reported it. Only a
			// hit with evidence for no term at all scores zero, and that is not
			// a match under any rule, so it never reaches the ranking. Partial
			// matches do reach it, ranked low.
			// Affinity is computed here, not in the store, so a provider only
			// ever has to report the tags it read and never has to know they
			// affect rank.
			affinity := make([]float64, len(terms))
			for ti, term := range terms {
				affinity[ti] = tagAffinity(term, hit.Tags)
			}
			hit.Score = score(Evidence{BodyCounts: hit.BodyCounts, TagAffinity: affinity})
			if hit.Score == 0 {
				continue
			}
			merged = append(merged, rankedHit{hit: hit, source: i})
		}
	}
	sort.SliceStable(merged, func(a, b int) bool {
		ra, rb := merged[a], merged[b]
		if ra.hit.Score != rb.hit.Score {
			return ra.hit.Score > rb.hit.Score
		}
		if ra.source != rb.source {
			return ra.source < rb.source
		}
		return ra.hit.Path < rb.hit.Path
	})
	alwaysApplied := alwaysAppliedSet()
	var eligible []store.Hit
	for _, r := range merged {
		hit := r.hit
		hit.Tier = string(s.sources[r.source].tier)
		hit.Name = s.sources[r.source].name
		hit.Category = categoryOf(hit.Path)
		if alwaysApplied[hit.Category] {
			// Always-applied categories are loaded in full on every task via
			// AlwaysAppliedEntries; exclude them from search so the same
			// content is never surfaced twice. This is the single registry-driven
			// place the exclusion lives — the store no longer special-cases it.
			continue
		}
		if IsCategoryDescription(hit.Path) {
			// A category's own description documents the category rather than
			// contributing to it, and every one of them shares the same
			// vocabulary, so any query resembling those words returned them
			// ahead of real entries. Excluded here, beside the always-applied
			// rule and driven by the same registry, so the search surface
			// applies both of its exclusions in one place. Deliberately not
			// excluded from List or Tags: the maintenance review has to be
			// able to enumerate descriptions in order to check them.
			continue
		}
		// The tag filter is enforced here, not taken on trust from the store.
		// A store may pre-apply it while walking to avoid reporting documents
		// that would be discarded, but this is the authority: a provider that
		// ignores or mis-implements SearchOptions.Tags cannot leak an untagged
		// entry into the results. Cheap, because the hit already carries its
		// tags.
		if !store.CarriesEveryTag(hit.Tags, sel.Tags) {
			continue
		}
		eligible = append(eligible, hit)
	}
	if len(eligible) == 0 {
		if len(sel.Tags) > 0 {
			if err := s.refuseUnknownTags(sel); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	// The cutoff is measured against the best hit that survives the exclusion
	// above, not against merged[0]. An always-applied entry scoring highest
	// would otherwise set the bar and then be dropped itself, quietly raising
	// the threshold for everything else in the result. eligible is still in the
	// sorted order, so its first element is that best score.
	//
	// This runs after every store has been searched and merged, which is the
	// point: the strongest hit may live in a different store from the weak one
	// being judged, and a per-store cutoff would keep a weak hit alive purely
	// because its own store held nothing better.
	floor := eligible[0].Score * cutoffFraction
	hits := eligible[:0]
	for _, hit := range eligible {
		if hit.Score < floor {
			continue
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

// refuseUnknownTags turns an empty tag-narrowed result into a refusal when the
// reason for it is that a requested tag exists nowhere in scope, rather than
// that the query simply did not match. A caller is then never left reading an
// empty result as "there is no knowledge on this subject" when the truth is
// "you asked for a tag nobody uses".
//
// It runs only when a tag-narrowed search comes back empty. Establishing the
// vocabulary costs a read of every entry in scope, as much again as the search
// itself, and there is no reason to pay that on the path where the search
// worked. A tag that is in the vocabulary but whose entries did not match the
// query still yields an empty result, which is the honest answer.
func (s *Set) refuseUnknownTags(sel Selector) error {
	vocabulary, err := s.Tags(sel)
	if err != nil {
		// The vocabulary is only needed to explain an empty result. Failing to
		// build it must not turn a successful empty search into an error.
		return nil
	}
	inUse := make(map[string]bool, len(vocabulary))
	names := make([]string, 0, len(vocabulary))
	for _, use := range vocabulary {
		inUse[use.Tag] = true
		names = append(names, use.Tag)
	}
	for _, tag := range sel.Tags {
		if inUse[tag] {
			continue
		}
		available := "no entry in scope carries any tag yet"
		if len(names) > 0 {
			available = "tags in use: " + strings.Join(names, ", ")
		}
		return output.NewError(
			ErrCodeTagUnknown,
			fmt.Sprintf("no knowledge entry in scope carries the tag %q", tag),
		).WithResource(tag).WithNextAction(fmt.Sprintf(
			`list the vocabulary with "knowledge tags" and reissue with a tag that is in use (%s)`,
			available,
		))
	}
	return nil
}

// categoryOf returns an entry's category — the first segment of its
// store-relative path. An entry sitting at the scope root, with no path
// separator, has no category and yields the empty string.
func categoryOf(path string) string {
	if category, _, found := strings.Cut(path, "/"); found {
		return category
	}
	return ""
}

// alwaysAppliedSet returns the always-applied category names as a set for
// membership tests, derived from the single registry declaration.
func alwaysAppliedSet() map[string]bool {
	set := make(map[string]bool)
	for _, name := range AlwaysApplied() {
		set[name] = true
	}
	return set
}

// Read returns the full content of a knowledge entry from the one store the
// address names. An address that does not name exactly one store is refused
// rather than resolved to whichever store happens to look closest.
func (s *Set) Read(addr Address, path string) ([]byte, error) {
	src, err := s.resolve(addr)
	if err != nil {
		return nil, err
	}
	content, err := src.store.Read(path)
	if errors.Is(err, store.ErrNotFound) {
		return nil, output.NewError(
			ErrCodeEntryNotFound,
			fmt.Sprintf("no knowledge entry at %q in the %q store", path, addr.Name),
		).WithResource(path).WithNextAction(fmt.Sprintf(
			`list that store's entries with "knowledge list --tier %s --filter %s", or search across stores with "knowledge search <query>"`,
			addr.Tier, addr.Name,
		))
	}
	return content, err
}

// Write persists a knowledge entry into the one store the address names,
// leaving every other store untouched. An address that does not name exactly
// one store is refused and nothing is written anywhere.
func (s *Set) Write(addr Address, path string, content []byte) error {
	src, err := s.resolve(addr)
	if err != nil {
		return err
	}
	return src.store.Write(path, content)
}

// Delete removes a knowledge entry from the one store the address names,
// leaving every other store untouched. An address that does not name exactly
// one store is refused and nothing is removed anywhere.
//
// Removing a path that holds nothing is a success, not a refusal: the storage
// contract states Delete returns nil when the file is absent, so a maintenance
// pass that retries is safe. The caller is the one that reports which of the
// two happened, because the storage layer cannot tell them apart.
func (s *Set) Delete(addr Address, path string) error {
	src, err := s.resolve(addr)
	if err != nil {
		return err
	}
	return src.store.Delete(path)
}

// List recursively enumerates every file entry across every store the selector
// covers, concatenated in configured order. Subdirectories are descended into;
// only file locators are emitted.
func (s *Set) List(sel Selector) ([]Entry, error) {
	if err := s.validateSelector(sel); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, src := range s.sources {
		if !sel.covers(src) {
			continue
		}
		files, err := listFiles(src.store, "")
		if err != nil {
			return nil, fmt.Errorf("listing knowledge store %q: %w", src.name, err)
		}
		for _, f := range files {
			entries = append(entries, Entry{Tier: src.tier, Name: src.name, Path: f})
		}
	}
	return entries, nil
}

// TagUse is one distinct tag and how widely it is already in use.
type TagUse struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"` // entries carrying it, across the covered stores
}

// Tags reports the tag vocabulary already in use across the stores the selector
// covers: every distinct tag, with the number of entries carrying it, most-used
// first and ties broken alphabetically so the order is stable across runs.
//
// It is purely mechanical. It reports what exists and never judges what a new
// entry ought to carry — that judgement belongs to the capture flow, which reads
// this to prefer an established tag over minting a near-duplicate. Putting the
// vocabulary in front of the agent is what makes convergence possible without
// anyone maintaining a taxonomy by hand.
//
// Always-applied categories are excluded, consistent with search: their entries
// are loaded in full on every task and never surfaced as hits, so their tags are
// not part of the vocabulary a new entry should be choosing from.
//
// A store whose entries carry no tags contributes nothing rather than erroring,
// so an untagged knowledge base reports an empty vocabulary.
func (s *Set) Tags(sel Selector) ([]TagUse, error) {
	if err := s.validateSelector(sel); err != nil {
		return nil, err
	}
	alwaysApplied := alwaysAppliedSet()
	counts := make(map[string]int)
	for _, src := range s.sources {
		if !sel.covers(src) {
			continue
		}
		files, err := listFiles(src.store, "")
		if err != nil {
			return nil, fmt.Errorf("listing knowledge store %q: %w", src.name, err)
		}
		for _, path := range files {
			if alwaysApplied[categoryOf(path)] {
				continue
			}
			raw, err := src.store.Read(path)
			if err != nil {
				return nil, fmt.Errorf("reading knowledge entry %q in store %q: %w", path, src.name, err)
			}
			// A malformed block yields no tags rather than failing the listing,
			// exactly as it does during a scan: one bad entry must not make the
			// vocabulary unreadable.
			tags, _, _ := store.ParseEntry(raw)
			for _, tag := range tags {
				counts[tag]++
			}
		}
	}
	uses := make([]TagUse, 0, len(counts))
	for tag, count := range counts {
		uses = append(uses, TagUse{Tag: tag, Count: count})
	}
	sort.Slice(uses, func(a, b int) bool {
		if uses[a].Count != uses[b].Count {
			return uses[a].Count > uses[b].Count
		}
		return uses[a].Tag < uses[b].Tag
	})
	return uses, nil
}

// AlwaysAppliedEntries reads the full body of every entry in every
// always-applied category (conventions, glossary, …) across every store the
// selector covers, concatenated in configured store order then registry
// category order, each tagged with its store, path, and category. These
// categories are loaded in full on every task rather than searched. A store that
// lacks a category's directory contributes nothing rather than erroring, so
// fresh or partially-populated stores still resolve cleanly.
//
// The selector is honoured with no exceptions: a store the selector does not
// name contributes nothing, whichever tier it sits in.
func (s *Set) AlwaysAppliedEntries(sel Selector) ([]AlwaysAppliedEntry, error) {
	return s.readCategories(AlwaysApplied(), sel)
}

// Conventions reads every always-apply convention across every store the
// selector covers, concatenated in configured order, and returns each one's full
// body tagged with its store and path. Conventions live under each store's
// "conventions/" directory. It is the conventions-only view over the generalised
// always-applied reader, so its JSON shape is unchanged. A store that has no such
// directory contributes nothing rather than erroring.
func (s *Set) Conventions(sel Selector) ([]Convention, error) {
	entries, err := s.readCategories([]string{"conventions"}, sel)
	if err != nil {
		return nil, err
	}
	var conventions []Convention
	for _, e := range entries {
		conventions = append(conventions, Convention{Tier: e.Tier, Name: e.Name, Path: e.Path, Content: e.Content})
	}
	return conventions, nil
}

// readCategories reads the full body of every entry in each named category
// across every store the selector covers, in configured store order then the
// given category order, tagging each entry with its category. A store missing a
// category's directory contributes nothing rather than erroring.
//
// Membership is decided by the selector alone, applied to every store without
// special cases. There is deliberately no exemption for a store the reader
// cannot attribute to a repo: a store the selector does not name contributes
// nothing.
func (s *Set) readCategories(categories []string, sel Selector) ([]AlwaysAppliedEntry, error) {
	if err := s.validateSelector(sel); err != nil {
		return nil, err
	}
	var entries []AlwaysAppliedEntry
	for _, src := range s.sources {
		if !sel.covers(src) {
			continue
		}
		for _, category := range categories {
			files, err := listFiles(src.store, category)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				return nil, fmt.Errorf("listing %s in knowledge store %q: %w", category, src.name, err)
			}
			for _, f := range files {
				if IsCategoryDescription(f) {
					// The same rule the search surface applies, at the other
					// place this set consults the registry. A category's own
					// description was being injected into the payload every
					// task receives, spending context on every request to say
					// what the category is for. Skipped before the read, so
					// the payload gets both smaller and cheaper.
					continue
				}
				content, err := src.store.Read(f)
				if err != nil {
					return nil, fmt.Errorf("reading %s entry %q in knowledge store %q: %w", category, f, src.name, err)
				}
				entries = append(entries, AlwaysAppliedEntry{Tier: src.tier, Name: src.name, Path: f, Content: string(content), Category: category})
			}
		}
	}
	return entries, nil
}

// Sources reports the configured stores by tier and name, with their providers
// and resolved locations, in configured order. It is the authoritative list of
// the names a request may address or narrow to.
func (s *Set) Sources() []SourceInfo {
	infos := make([]SourceInfo, len(s.sources))
	for i, src := range s.sources {
		infos[i] = SourceInfo{Tier: src.tier, Name: src.name, Provider: src.provider, Location: src.location}
	}
	return infos
}

// resolve turns an address into exactly one configured store, or into a
// refusal. It is the single place the question "which store?" is answered on
// the read and write path, so no caller can reintroduce a first-match guess.
// Every refusal names the stores available in the tier concerned.
func (s *Set) resolve(addr Address) (scopedStore, error) {
	if err := addr.Validate(); err != nil {
		var names []string
		if validTier(addr.Tier) && addr.Tier != TierAll {
			names = s.namesIn(addr.Tier)
		} else {
			names = append(s.namesIn(TierRepo), s.namesIn(TierProject)...)
		}
		return scopedStore{}, withAvailable(err, addr.Tier, names)
	}
	for _, src := range s.sources {
		if src.tier == addr.Tier && src.name == addr.Name {
			return src, nil
		}
	}
	return scopedStore{}, output.NewError(
		ErrCodeStoreUnknown,
		fmt.Sprintf("no knowledge store named %q in the %q tier", addr.Name, addr.Tier),
	).WithNextAction(availableNames(addr.Tier, s.namesIn(addr.Tier)))
}

// validateSelector checks a selector against the configured stores: the tier
// must be one of the three, and every name in the filter must belong to a store
// the tier reaches. A filter naming a store the tier does not reach is refused
// rather than quietly returning nothing, so a caller is never left reading an
// empty result as "no such knowledge". Validation lives here rather than on the
// value because only the set knows which names exist.
func (s *Set) validateSelector(sel Selector) error {
	if err := sel.Validate(); err != nil {
		return withAvailable(err, sel.Tier, s.namesIn(TierAll))
	}
	if len(sel.Filter) == 0 {
		return nil
	}
	reachable := make(map[string]bool)
	for _, name := range s.namesIn(sel.Tier) {
		reachable[name] = true
	}
	for _, name := range sel.Filter {
		if !reachable[name] {
			return output.NewError(
				ErrCodeStoreUnknown,
				fmt.Sprintf("no knowledge store named %q in the %q tier", name, sel.Tier),
			).WithNextAction(availableNames(sel.Tier, s.namesIn(sel.Tier)))
		}
	}
	return nil
}

// namesIn returns the names of the configured stores in a tier, in configured
// order. TierAll spans both tiers. It is what every refusal's next action is
// built from.
func (s *Set) namesIn(tier Tier) []string {
	var names []string
	for _, src := range s.sources {
		if tier == TierAll || src.tier == tier {
			names = append(names, src.name)
		}
	}
	return names
}

// withAvailable appends the available store names to a validation refusal's
// next action, so a caller is told both how to correct the request and what it
// may name.
func withAvailable(err error, tier Tier, names []string) error {
	var er *output.ErrorResponse
	if !errors.As(err, &er) {
		return err
	}
	listed := tier
	if !validTier(listed) || listed == TierAll {
		listed = TierAll
	}
	return er.WithNextAction(er.NextAction + "; " + availableNames(listed, names))
}

// listFiles recursively walks a store from dir, returning store-relative file
// locators. Directories are descended into via Store.List, which stays one
// level deep — the recursion lives here in the knowledge layer.
func listFiles(st store.Store, dir string) ([]string, error) {
	children, err := st.List(dir)
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
			sub, err := listFiles(st, childPath)
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

// unreachableStore builds the error for a knowledge source whose location does
// not resolve to a directory. When the store is found at the location the
// pre-1.0 rule would have produced — relative to the project root rather than
// to the folder holding config.yaml — the error says so and names the exact
// replacement value, so a config written against the old rule reports its own
// migration rather than only its failure.
func unreachableStore(tier Tier, name, declared, resolved, projectRoot string) error {
	err := output.NewError(
		"knowledge_location_missing",
		fmt.Sprintf("knowledge store %q in the %q tier is unreachable at %s (a relative location is resolved from the folder holding config.yaml, %s)",
			name, tier, resolved, config.ProjectConfigDir(projectRoot)),
	).WithResource(resolved)

	if filepath.IsAbs(declared) {
		return err.WithNextAction(fmt.Sprintf("create %s, or correct the location of the %q store in config.yaml", resolved, name))
	}

	legacy := filepath.Join(projectRoot, declared)
	if info, statErr := os.Stat(legacy); statErr == nil && info.IsDir() {
		corrected, relErr := filepath.Rel(config.ProjectConfigDir(projectRoot), legacy)
		if relErr == nil {
			return err.WithNextAction(fmt.Sprintf(
				"the store is at %s, which is where this location resolved before relative knowledge locations moved to the config.yaml folder; in config.yaml set the %q store's location to %q",
				legacy, name, corrected))
		}
	}

	return err.WithNextAction(fmt.Sprintf("create %s, or correct the location of the %q store in config.yaml", resolved, name))
}
