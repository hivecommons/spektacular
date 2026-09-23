package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// writeFile creates dir/name (including parents) with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// twoScopeSet stands up two file-backed sources at fresh temp dirs and returns
// the Set plus the two backing directories. Neither source carries a stamped
// tier, so both fall back to the project tier, addressed under the names they
// declare: "project" and "team". Each dir is seeded with a top-level file and a file nested under
// architecture/. Both stores contain a file with the keyword "compass" so
// overlap can be asserted.
func twoScopeSet(t *testing.T) (set *Set, projectDir, teamDir string) {
	t.Helper()
	projectDir = t.TempDir()
	teamDir = t.TempDir()

	writeFile(t, projectDir, "readme.md", "project readme: the compass points north\n")
	writeFile(t, projectDir, "architecture/initial-idea.md", "an architecture note about widgets\n")

	writeFile(t, teamDir, "guidelines.md", "team guidelines reference the compass too\n")
	writeFile(t, teamDir, "architecture/overview.md", "team overview of the system\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{
			Name:     "project",
			Provider: config.ProviderFile,
			Config:   config.FileKnowledgeConfig{Location: projectDir},
		},
		{
			Name:     "team",
			Provider: config.ProviderFile,
			Config:   config.FileKnowledgeConfig{Location: teamDir},
		},
	}

	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)
	return set, projectDir, teamDir
}

// singleSourceSet stands up one file-backed source over an already-seeded dir,
// addressed as the project-tier store "project". It is the minimum Set needed
// to exercise Search end to end, for tests whose subject is ranking rather than
// fan-out across stores.
func singleSourceSet(t *testing.T, dir string) *Set {
	t.Helper()
	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "project", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: dir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)
	return set
}

// Criterion 1: List, Read, and Search fan across every configured scope and
// include entries nested in subdirectories.
func TestSet_FansAcrossScopesIncludingSubdirs(t *testing.T) {
	set, _, _ := twoScopeSet(t)

	entries, err := set.List(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []Entry{
		{Tier: TierProject, Name: "project", Path: "readme.md"},
		{Tier: TierProject, Name: "project", Path: "architecture/initial-idea.md"},
		{Tier: TierProject, Name: "team", Path: "guidelines.md"},
		{Tier: TierProject, Name: "team", Path: "architecture/overview.md"},
	}, entries)

	data, err := set.Read(Address{Tier: TierProject, Name: "team"}, "architecture/overview.md")
	require.NoError(t, err)
	require.Equal(t, []byte("team overview of the system\n"), data)

	hits, err := set.Search("compass", Selector{Tier: TierAll})
	require.NoError(t, err)
	addresses := map[[2]string]bool{}
	for _, h := range hits {
		addresses[[2]string{h.Tier, h.Name}] = true
	}
	require.True(t, addresses[[2]string{"project", "project"}], "search should yield a project hit")
	require.True(t, addresses[[2]string{"project", "team"}], "search should yield a team hit")
}

// Criterion 2: an overlapping topic surfaces from both scopes, each result
// correctly tagged with its scope.
func TestSet_OverlappingEntriesTaggedPerScope(t *testing.T) {
	projectDir := t.TempDir()
	teamDir := t.TempDir()

	// Same Path "notes/topic.md" exists in both scopes, both mentioning "compass".
	writeFile(t, projectDir, "notes/topic.md", "project view: compass discussion\n")
	writeFile(t, teamDir, "notes/topic.md", "team view: compass discussion\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "project", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: projectDir}},
		{Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: teamDir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)

	hits, err := set.Search("compass", Selector{Tier: TierAll})
	require.NoError(t, err)
	hitAddresses := map[[2]string]bool{}
	for _, h := range hits {
		hitAddresses[[2]string{h.Tier, h.Name}] = true
	}
	require.True(t, hitAddresses[[2]string{"project", "project"}], `compass hit should be tagged tier "project", name "project"`)
	require.True(t, hitAddresses[[2]string{"project", "team"}], `compass hit should be tagged tier "project", name "team"`)

	entries, err := set.List(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []Entry{
		{Tier: TierProject, Name: "project", Path: "notes/topic.md"},
		{Tier: TierProject, Name: "team", Path: "notes/topic.md"},
	}, entries)
}

// Phase 2.1 criterion 1: Search ranks hits globally by score, so a strong
// match in the later-configured "team" source outranks a weak match in the
// earlier "project" source. Scores are hand-computed from the fixtures: the
// team file holds three occurrences of "beacon" and the project file one, which
// damp to damp(3) = 2.5849625007211562 and damp(1) = 1 — a single-term query,
// so coverage is 1 for both and the ordering rests on the damped counts alone.
func TestSet_SearchRanksAcrossSourcesByScore(t *testing.T) {
	projectDir := t.TempDir()
	teamDir := t.TempDir()

	writeFile(t, projectDir, "notes.md", "a single beacon here\n")
	writeFile(t, teamDir, "signals.md", "beacon beacon\nthe beacon shines\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "project", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: projectDir}},
		{Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: teamDir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)

	hits, err := set.Search("beacon", Selector{Tier: TierAll})
	require.NoError(t, err)

	order := make([][3]string, len(hits))
	for i, h := range hits {
		order[i] = [3]string{h.Tier, h.Name, h.Path}
	}
	require.Equal(t, [][3]string{
		{"project", "team", "signals.md"},  // damp(3) = 2.5849625 ranks first despite team being configured second
		{"project", "project", "notes.md"}, // damp(1) = 1
	}, order)
}

// Phase 2.1 criterion 2: equal-score hits order by configured source order then
// by path ascending, identically on every call. "lantern" occurs exactly once
// per file, and each scope pairs notes.md with notes/zz.md — names whose
// walk-discovery order (the notes/ dir is descended before the notes.md file)
// differs from path-ascending order ("notes.md" sorts before "notes/zz.md").
//
// This is also Phase 1.3 criterion 4 — results stay in a stable, repeatable
// order across identical searches — and it pins that the relative cutoff does
// not disturb the tie-break. All four hits score exactly 1, so all four equal
// the best score and none falls under the floor of 0.25; the cutoff pass must
// hand back every one of them, still in this order.
func TestSet_SearchTieBreaksBySourceOrderThenPath(t *testing.T) {
	projectDir := t.TempDir()
	teamDir := t.TempDir()

	writeFile(t, projectDir, "notes.md", "project lantern\n")
	writeFile(t, projectDir, "notes/zz.md", "project nested lantern\n")
	writeFile(t, teamDir, "notes.md", "team lantern\n")
	writeFile(t, teamDir, "notes/zz.md", "team nested lantern\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "project", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: projectDir}},
		{Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: teamDir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)

	expected := [][3]string{
		{"project", "project", "notes.md"},
		{"project", "project", "notes/zz.md"},
		{"project", "team", "notes.md"},
		{"project", "team", "notes/zz.md"},
	}

	first, err := set.Search("lantern", Selector{Tier: TierAll})
	require.NoError(t, err)
	second, err := set.Search("lantern", Selector{Tier: TierAll})
	require.NoError(t, err)

	firstOrder := make([][3]string, len(first))
	for i, h := range first {
		firstOrder[i] = [3]string{h.Tier, h.Name, h.Path}
	}
	require.Equal(t, expected, firstOrder)
	require.Equal(t, first, second, "repeated searches must return identical slices")
}

// Phase 2.1 criterion 3: a hit's Path feeds straight into set.Read at the hit's
// store address, which returns the full original fixture content. "overview"
// matches only the team store's nested file, so the round trip crosses into a
// non-first source. A hit carries its store's tier and name, which are exactly
// the two halves of the address it round-trips through.
func TestSet_SearchHitRoundTripsThroughRead(t *testing.T) {
	set, _, _ := twoScopeSet(t)

	hits, err := set.Search("overview", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, "project", hits[0].Tier)
	require.Equal(t, "team", hits[0].Name)

	data, err := set.Read(Address{Tier: Tier(hits[0].Tier), Name: hits[0].Name}, hits[0].Path)
	require.NoError(t, err)
	require.Equal(t, []byte("team overview of the system\n"), data)
}

// Criterion 3: a source pointing at an unreachable location fails NewSet with
// an error that names the offending store by its address.
func TestNewSet_UnreachableSourceFailsNamingStore(t *testing.T) {
	good := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "project", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: good}},
		{Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: missing}},
	}

	set, err := NewSet(cfg, t.TempDir())
	require.Error(t, err)
	require.Nil(t, set)
	require.Contains(t, err.Error(), `knowledge store "team" in the "project" tier is unreachable`)
}

// A relative source location resolves from the folder holding config.yaml, the
// same base a repos entry uses — not from the project root. The two bases are
// one directory apart, so a config written against the wrong one silently
// addresses a sibling of the intended store rather than failing loudly; this
// asserts the base directly rather than trusting a path that happens to exist.
func TestNewSet_RelativeLocationResolvesFromTheConfigDir(t *testing.T) {
	projectRoot := t.TempDir()

	// The store lives beside config.yaml, inside .spektacular/.
	wanted := filepath.Join(projectRoot, ".spektacular", "shared-kb")
	require.NoError(t, os.MkdirAll(filepath.Join(wanted, "architecture"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wanted, "architecture", "note.md"), []byte("from the config dir\n"), 0o644))

	// A decoy at the project root, which is where the pre-1.0 rule would have
	// looked. Resolving against the wrong base finds this one instead.
	decoy := filepath.Join(projectRoot, "shared-kb")
	require.NoError(t, os.MkdirAll(filepath.Join(decoy, "architecture"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(decoy, "architecture", "note.md"), []byte("from the project root\n"), 0o644))

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: "shared-kb"}},
	}

	set, err := NewSet(cfg, projectRoot)
	require.NoError(t, err)

	sources := set.Sources()
	require.Len(t, sources, 1)
	require.Equal(t, wanted, sources[0].Location,
		"a relative knowledge location must resolve from the config.yaml folder, not the project root")

	body, err := set.Read(Address{Tier: TierProject, Name: "team"}, "architecture/note.md")
	require.NoError(t, err)
	require.Equal(t, []byte("from the config dir\n"), body,
		"the store read must come from the config-dir store, not the project-root decoy")
}

// A config written against the pre-1.0 rule points at a directory that exists,
// just one level up from where the location now resolves. The failure must name
// the corrected value outright, so the error migrates the config rather than
// only reporting that something is missing.
func TestNewSet_LegacyRelativeLocationReportsTheCorrectedValue(t *testing.T) {
	projectRoot := t.TempDir()

	// The store sits where the old rule resolved to: <projectRoot>/team-knowledge.
	legacy := filepath.Join(projectRoot, "team-knowledge")
	require.NoError(t, os.MkdirAll(legacy, 0o755))

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: "team-knowledge"}},
	}

	set, err := NewSet(cfg, projectRoot)
	require.Error(t, err)
	require.Nil(t, set)

	require.Contains(t, err.Error(), `knowledge store "team" in the "project" tier is unreachable`)

	// The corrected value belongs in next_action, not the message: the message
	// names the problem, next_action gives the exact runnable fix.
	var resp *output.ErrorResponse
	require.ErrorAs(t, err, &resp)
	require.Contains(t, resp.NextAction, filepath.Join("..", "team-knowledge"),
		"next_action must name the corrected relative location, not merely report the miss")
	require.Contains(t, resp.NextAction, legacy,
		"next_action must name where the store was actually found")
}

// Criterion 4: a write persists into exactly the chosen scope and leaves every
// other scope untouched.
func TestSet_WriteIsolatedToChosenScope(t *testing.T) {
	set, projectDir, teamDir := twoScopeSet(t)

	require.NoError(t, set.Write(Address{Tier: TierProject, Name: "project"}, "note.md", []byte("scoped note")))

	// File exists on disk in the project store dir only.
	require.FileExists(t, filepath.Join(projectDir, "note.md"))
	_, statErr := os.Stat(filepath.Join(teamDir, "note.md"))
	require.True(t, os.IsNotExist(statErr), "note.md must not appear in the team scope dir")

	// Reading back from project returns the content; from team it errors.
	data, err := set.Read(Address{Tier: TierProject, Name: "project"}, "note.md")
	require.NoError(t, err)
	require.Equal(t, []byte("scoped note"), data)

	_, err = set.Read(Address{Tier: TierProject, Name: "team"}, "note.md")
	require.Error(t, err)
}

// Phase 1.3 criterion 1: an entry removed from the store it was addressed in
// is gone from every retrieval path — it is absent from the listing and it is
// absent from the search results — while every other entry, in that store and
// in the other one, is untouched.
func TestSet_DeleteRemovesTheEntryFromListingAndSearch(t *testing.T) {
	set, _, _ := twoScopeSet(t)

	require.NoError(t, set.Delete(Address{Tier: TierProject, Name: "project"}, "readme.md"))

	entries, err := set.List(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []Entry{
		{Tier: TierProject, Name: "project", Path: "architecture/initial-idea.md"},
		{Tier: TierProject, Name: "team", Path: "guidelines.md"},
		{Tier: TierProject, Name: "team", Path: "architecture/overview.md"},
	}, entries, "the removed entry must be gone and every other entry must remain")

	// Both stores held a file mentioning "compass"; only the team one is left.
	hits, err := set.Search("compass", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []string{"guidelines.md"}, tagPaths(hits),
		"a removed entry must not be returned by a search that previously matched it")
}

// Phase 1.3 criterion 2: removing a path that holds nothing is a success, not a
// refusal, and it changes nothing. Running it twice is safe, so a maintenance
// pass that retries a removal it already made does not fail the second time.
func TestSet_DeleteOfAnAbsentPathSucceedsAndChangesNothing(t *testing.T) {
	set, _, _ := twoScopeSet(t)

	addr := Address{Tier: TierProject, Name: "project"}
	require.NoError(t, set.Delete(addr, "learnings/never-written.md"))
	require.NoError(t, set.Delete(addr, "learnings/never-written.md"),
		"a repeated removal of an absent entry must stay a success")

	// A genuine removal followed by a repeat of itself is the same story: the
	// first call removes, the second finds nothing and still succeeds.
	require.NoError(t, set.Delete(addr, "readme.md"))
	require.NoError(t, set.Delete(addr, "readme.md"))

	entries, err := set.List(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []Entry{
		{Tier: TierProject, Name: "project", Path: "architecture/initial-idea.md"},
		{Tier: TierProject, Name: "team", Path: "guidelines.md"},
		{Tier: TierProject, Name: "team", Path: "architecture/overview.md"},
	}, entries, "removing an absent path must leave the knowledge base exactly as it was")
}

// Sources reports the configured stores by tier and name, with their providers
// and locations, in configured order.
func TestSet_SourcesReportsConfiguredStores(t *testing.T) {
	set, projectDir, teamDir := twoScopeSet(t)

	require.Equal(t, []SourceInfo{
		{Tier: TierProject, Name: "project", Provider: config.ProviderFile, Location: projectDir},
		{Tier: TierProject, Name: "team", Provider: config.ProviderFile, Location: teamDir},
	}, set.Sources())
}

// Phase 2.2 criterion 4: a project that declares no shared stores is valid.
// Nothing is synthesised in its place, so the project tier is simply empty, and
// every fan-out read restricted to that tier returns nothing rather than
// erroring.
func TestNewSet_NoDeclaredSharedStoresYieldsAnEmptyProjectTier(t *testing.T) {
	set, err := NewSet(config.NewDefault(), t.TempDir())
	require.NoError(t, err)
	require.Empty(t, set.Sources(), "a project declaring no shared stores must synthesise none")

	projectOnly := Selector{Tier: TierProject}

	entries, err := set.List(projectOnly)
	require.NoError(t, err)
	require.Empty(t, entries)

	hits, err := set.Search("compass", projectOnly)
	require.NoError(t, err)
	require.Empty(t, hits)

	applied, err := set.AlwaysAppliedEntries(projectOnly)
	require.NoError(t, err)
	require.Empty(t, applied)

	conventions, err := set.Conventions(projectOnly)
	require.NoError(t, err)
	require.Empty(t, conventions)
}

// Conventions returns the full body of every always-apply convention across
// every configured scope, tagged with its scope and path, including those
// nested in subdirectories. The expected set is hand-written and exact, so it
// also proves non-convention files (readme.md, architecture/*) are excluded.
func TestSet_ConventionsFanAcrossScopesIncludingSubdirs(t *testing.T) {
	set, projectDir, teamDir := twoScopeSet(t)

	writeFile(t, projectDir, "conventions/style.md", "project: use tabs not spaces\n")
	writeFile(t, projectDir, "conventions/naming/files.md", "project: kebab-case file names\n")
	writeFile(t, teamDir, "conventions/review.md", "team: two approvals required\n")

	conventions, err := set.Conventions(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []Convention{
		{Tier: TierProject, Name: "project", Path: "conventions/style.md", Content: "project: use tabs not spaces\n"},
		{Tier: TierProject, Name: "project", Path: "conventions/naming/files.md", Content: "project: kebab-case file names\n"},
		{Tier: TierProject, Name: "team", Path: "conventions/review.md", Content: "team: two approvals required\n"},
	}, conventions)
}

// Conventions concatenates per-scope in configured order: every project-scope
// convention precedes every team-scope one. Asserted via the sequence of the
// returned .Name fields, independent of within-store filesystem ordering.
func TestSet_ConventionsConcatenateInConfiguredScopeOrder(t *testing.T) {
	set, projectDir, teamDir := twoScopeSet(t)

	writeFile(t, projectDir, "conventions/a.md", "project a\n")
	writeFile(t, projectDir, "conventions/b.md", "project b\n")
	writeFile(t, teamDir, "conventions/c.md", "team c\n")
	writeFile(t, teamDir, "conventions/d.md", "team d\n")

	conventions, err := set.Conventions(Selector{Tier: TierAll})
	require.NoError(t, err)

	nameSeq := make([]string, len(conventions))
	for i, c := range conventions {
		nameSeq[i] = c.Name
	}
	require.Equal(t, []string{"project", "project", "team", "team"}, nameSeq)
}

// A scope without a conventions/ directory contributes nothing and produces no
// error: only the populated scope's conventions are returned.
func TestSet_ConventionsSkipScopeWithoutConventionsDir(t *testing.T) {
	set, projectDir, _ := twoScopeSet(t)

	// Only the project scope gets a conventions/ dir; team has none.
	writeFile(t, projectDir, "conventions/style.md", "project: lint before commit\n")

	conventions, err := set.Conventions(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []Convention{
		{Tier: TierProject, Name: "project", Path: "conventions/style.md", Content: "project: lint before commit\n"},
	}, conventions)
}

// When no scope has a conventions/ directory, Conventions returns an empty
// result and no error.
func TestSet_ConventionsEmptyWhenNoScopeHasConventions(t *testing.T) {
	set, _, _ := twoScopeSet(t)

	conventions, err := set.Conventions(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Empty(t, conventions)
}

// Phase 2.2 criterion: every search hit is tagged with its category, the first
// segment of its store-relative path. The fixtures live in two looked-up
// categories so the hits survive search; the expected category per path is
// hand-written.
func TestSet_SearchTagsHitsWithCategory(t *testing.T) {
	set, projectDir, _ := twoScopeSet(t)

	writeFile(t, projectDir, "gotchas/x.md", "watch out for the sextant trap\n")
	writeFile(t, projectDir, "architecture/y.md", "the sextant module wiring\n")

	hits, err := set.Search("sextant", Selector{Tier: TierAll})
	require.NoError(t, err)

	byPath := map[string]string{}
	for _, h := range hits {
		byPath[h.Path] = h.Category
	}
	require.Equal(t, "gotchas", byPath["gotchas/x.md"])
	require.Equal(t, "architecture", byPath["architecture/y.md"])
}

// Phase 2.2 criterion: always-applied categories are excluded from search while
// looked-up categories are not. conventions/ and glossary/ entries that match
// the query never appear; the matching gotchas/ entry does.
func TestSet_SearchExcludesAlwaysAppliedCategories(t *testing.T) {
	set, projectDir, _ := twoScopeSet(t)

	writeFile(t, projectDir, "conventions/c.md", "the astrolabe rule\n")
	writeFile(t, projectDir, "glossary/g.md", "astrolabe: a term\n")
	writeFile(t, projectDir, "gotchas/h.md", "astrolabe gotcha\n")

	hits, err := set.Search("astrolabe", Selector{Tier: TierAll})
	require.NoError(t, err)

	categories := map[string]bool{}
	for _, h := range hits {
		categories[h.Category] = true
	}
	require.False(t, categories["conventions"], "conventions must be excluded from search")
	require.False(t, categories["glossary"], "glossary must be excluded from search")
	require.True(t, categories["gotchas"], "looked-up gotchas must appear in search")
}

// hitPaths returns the store-relative path of every hit, in result order.
func hitPaths(hits []store.Hit) []string {
	paths := make([]string, len(hits))
	for i, h := range hits {
		paths[i] = h.Path
	}
	return paths
}

// Phase 2.1 criterion 1: a category's own generated description is never a
// search result, in any category — including the looked-up ones, which the
// always-applied exclusion has no claim over. Every category in the registry
// carries a description here and every one of them matches the query far more
// strongly than the single real entry does, so this also pins that the
// exclusion is applied before the relevance floor is computed: a description
// setting the floor from a score it is then dropped for would take the weak
// real entry down with it.
func TestSet_SearchExcludesEveryCategorysOwnDescription(t *testing.T) {
	set, projectDir, _ := twoScopeSet(t)

	for _, c := range Categories {
		writeFile(t, projectDir, c.Name+"/"+CategoryDescriptionFile,
			"# "+c.Name+"\n\nbinnacle binnacle binnacle binnacle binnacle\n")
	}
	writeFile(t, projectDir, "gotchas/trap.md", "one passing mention of the binnacle\n")

	hits, err := set.Search("binnacle", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []string{"gotchas/trap.md"}, hitPaths(hits),
		"only the real entry may come back; no category description, however strongly it matches")
}

// Phase 2.1 criterion 4: a README a contributor placed deeper inside a category
// is their own file, not the category's generated description, so it is still
// returned by search and still carries that category — the exclusion has no
// claim over content someone wrote themselves.
func TestSet_SearchReturnsAContributorsNestedREADME(t *testing.T) {
	set, projectDir, _ := twoScopeSet(t)

	writeFile(t, projectDir, "gotchas/README.md", "# Gotchas\n\nthe binnacle category\n")
	writeFile(t, projectDir, "gotchas/sub/README.md", "a hand-written binnacle write-up\n")

	hits, err := set.Search("binnacle", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []string{"gotchas/sub/README.md"}, hitPaths(hits))
	require.Equal(t, "gotchas", hits[0].Category,
		"a nested README is an ordinary entry, tagged with the category it sits in")
}

// Phase 2.1 criterion 3: keeping descriptions out of retrieval does not hide
// them. A plain listing still enumerates every one of them, and each is still
// readable at its address — which is what a maintenance review of the
// descriptions depends on.
func TestSet_ListAndReadStillReachEveryCategoryDescription(t *testing.T) {
	set, projectDir, teamDir := twoScopeSet(t)

	writeFile(t, projectDir, "conventions/README.md", "# Conventions\n\nproject conventions\n")
	writeFile(t, projectDir, "gotchas/README.md", "# Gotchas\n\nproject gotchas\n")
	writeFile(t, teamDir, "glossary/README.md", "# Glossary\n\nteam glossary\n")

	entries, err := set.List(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []Entry{
		{Tier: TierProject, Name: "project", Path: "readme.md"},
		{Tier: TierProject, Name: "project", Path: "architecture/initial-idea.md"},
		{Tier: TierProject, Name: "project", Path: "conventions/README.md"},
		{Tier: TierProject, Name: "project", Path: "gotchas/README.md"},
		{Tier: TierProject, Name: "team", Path: "guidelines.md"},
		{Tier: TierProject, Name: "team", Path: "architecture/overview.md"},
		{Tier: TierProject, Name: "team", Path: "glossary/README.md"},
	}, entries)

	content, err := set.Read(Address{Tier: TierProject, Name: "team"}, "glossary/README.md")
	require.NoError(t, err)
	require.Equal(t, "# Glossary\n\nteam glossary\n", string(content))
}

// Phase 2.1 criterion 5: the label vocabulary is untouched by the retrieval
// exclusion. It still excludes the always-applied categories and nothing else,
// so a description in a looked-up category still contributes its labels —
// descriptions are deliberately kept out of search alone, not out of the
// listing paths a maintenance review uses. Real scaffolded descriptions carry
// no frontmatter; these do, because labels are the only thing this path
// reports.
func TestSet_TagsAreUnchangedByTheDescriptionExclusion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "conventions/README.md", "---\ntags: [conventions-doc]\n---\n# Conventions\n")
	writeFile(t, dir, "glossary/README.md", "---\ntags: [glossary-doc]\n---\n# Glossary\n")
	writeFile(t, dir, "gotchas/README.md", "---\ntags: [gotchas-doc]\n---\n# Gotchas\n")
	writeFile(t, dir, "gotchas/trap.md", "---\ntags: [gotchas-doc]\n---\n# Trap\n\nprose\n")

	set := singleSourceSet(t, dir)

	uses, err := set.Tags(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []TagUse{{Tag: "gotchas-doc", Count: 2}}, uses,
		"always-applied descriptions contribute nothing, as before; a looked-up one still counts")
}

// Phase 2.2 load-bearing criterion: re-tiering a category in the registry is a
// single self-consistent action. Flipping gotchas to always-applied
// simultaneously makes Search exclude it AND makes AlwaysAppliedEntries load it,
// so the two behaviours can never drift. The mutation is restored in defer so no
// other test is affected. Not parallel: it mutates package state.
func TestRetier_FlipsLoadAndSearchExclusionTogether(t *testing.T) {
	set, projectDir, _ := twoScopeSet(t)

	writeFile(t, projectDir, "gotchas/trap.md", "the binnacle trap\n")

	// Before the flip: gotchas is looked-up — it appears in search and not in
	// the always-applied reader.
	hits, err := set.Search("binnacle", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, "gotchas", hits[0].Category)

	entries, err := set.AlwaysAppliedEntries(Selector{Tier: TierAll})
	require.NoError(t, err)
	for _, e := range entries {
		require.NotEqual(t, "gotchas", e.Category, "gotchas must not be always-applied before the flip")
	}

	// Flip gotchas to always-applied via a single registry edit, restoring the
	// exact original registry afterwards.
	original := Categories
	modified := make([]Category, len(original))
	copy(modified, original)
	for i := range modified {
		if modified[i].Name == "gotchas" {
			modified[i].Tier = CategoryTierAlwaysApplied
		}
	}
	Categories = modified
	defer func() { Categories = original }()

	// After the flip: gotchas is now excluded from search AND now loaded by the
	// always-applied reader, tagged with its category.
	hits, err = set.Search("binnacle", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Empty(t, hits, "gotchas must be excluded from search after the flip")

	entries, err = set.AlwaysAppliedEntries(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Contains(t, entries, AlwaysAppliedEntry{
		Tier:     TierProject,
		Name:     "project",
		Path:     "gotchas/trap.md",
		Content:  "the binnacle trap\n",
		Category: "gotchas",
	})
}

// Phase 2.2 criterion: AlwaysAppliedEntries returns entries from every
// always-applied category — conventions AND glossary — across all scopes, each
// tagged with its category and full content. Expected values are hand-written.
//
// Phase 2.1 criterion 2: each always-applied category here also carries its own
// generated description, exactly as a scaffolded store does. The expected list
// below is unchanged by their presence — the payload every task receives is the
// same entry for entry, and spends no context restating what a category is for.
func TestSet_AlwaysAppliedEntriesReturnsAllAlwaysAppliedCategories(t *testing.T) {
	set, projectDir, teamDir := twoScopeSet(t)

	writeFile(t, projectDir, "conventions/c.md", "project: use tabs\n")
	writeFile(t, projectDir, "glossary/g.md", "compass: a navigation term\n")
	writeFile(t, teamDir, "glossary/term.md", "sextant: another term\n")

	writeFile(t, projectDir, "conventions/README.md", "# Conventions\n\n**Tier:** always-applied\n")
	writeFile(t, projectDir, "glossary/README.md", "# Glossary\n\n**Tier:** always-applied\n")
	writeFile(t, teamDir, "glossary/README.md", "# Glossary\n\n**Tier:** always-applied\n")

	entries, err := set.AlwaysAppliedEntries(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []AlwaysAppliedEntry{
		{Tier: TierProject, Name: "project", Path: "conventions/c.md", Content: "project: use tabs\n", Category: "conventions"},
		{Tier: TierProject, Name: "project", Path: "glossary/g.md", Content: "compass: a navigation term\n", Category: "glossary"},
		{Tier: TierProject, Name: "team", Path: "glossary/term.md", Content: "sextant: another term\n", Category: "glossary"},
	}, entries)
}

// repoScopedSet stands up three file-backed sources: one the project declares
// itself, which carries no stamped tier and so is addressed in the project tier
// under its declared name "project", and two stamped into the repo tier under
// the registry names "repo-a" and "repo-b". Each store gets one convention file
// so filtering can be asserted per store.
func repoScopedSet(t *testing.T) (set *Set, projectDir, repoADir, repoBDir string) {
	t.Helper()
	projectDir = t.TempDir()
	repoADir = t.TempDir()
	repoBDir = t.TempDir()

	writeFile(t, projectDir, "conventions/proj.md", "project: use tabs\n")
	writeFile(t, repoADir, "conventions/a.md", "repo-a: use spaces\n")
	writeFile(t, repoBDir, "conventions/b.md", "repo-b: use semicolons\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{
			Name:     "project",
			Provider: config.ProviderFile,
			Config:   config.FileKnowledgeConfig{Location: projectDir},
		},
		{
			Provider: config.ProviderFile,
			Config:   config.FileKnowledgeConfig{Location: repoADir},
			Tier:     string(TierRepo),
			Name:     "repo-a",
		},
		{
			Provider: config.ProviderFile,
			Config:   config.FileKnowledgeConfig{Location: repoBDir},
			Tier:     string(TierRepo),
			Name:     "repo-b",
		},
	}

	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)
	return set, projectDir, repoADir, repoBDir
}

// narrowingSet stands up four file-backed stores spanning both tiers: two the
// project declares itself, addressed in the project tier as "project" and
// "team", and two contributed by registered repos, addressed in the repo tier
// as "repo-a" and "repo-b". Two stores per tier is what makes a filter naming
// several names meaningful in either tier, which repoScopedSet — one project
// store, two repo stores — cannot express; it is a separate fixture so the
// tests built on repoScopedSet keep their exact expectations.
//
// Each store carries the same three entries under its own name: an
// always-applied convention, an always-applied glossary term, and a looked-up
// gotcha holding the shared search keyword "sextant" so one query reaches every
// store.
func narrowingSet(t *testing.T) *Set {
	t.Helper()

	var sources []config.SourceConfig
	for _, s := range []struct {
		name string
		tier Tier
	}{
		{name: "project", tier: TierProject},
		{name: "team", tier: TierProject},
		{name: "repo-a", tier: TierRepo},
		{name: "repo-b", tier: TierRepo},
	} {
		dir := t.TempDir()
		writeFile(t, dir, "conventions/"+s.name+".md", s.name+" convention\n")
		writeFile(t, dir, "glossary/"+s.name+".md", s.name+" term\n")
		writeFile(t, dir, "gotchas/"+s.name+".md", s.name+" sextant gotcha\n")
		sources = append(sources, config.SourceConfig{
			Provider: config.ProviderFile,
			Config:   config.FileKnowledgeConfig{Location: dir},
			Tier:     string(s.tier),
			Name:     s.name,
		})
	}

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = sources
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)
	return set
}

// distinct returns the unique values in names, in first-seen order.
func distinct(names []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		unique = append(unique, name)
	}
	return unique
}

// Criterion 1: a search restricted to the repo tier returns hits only from the
// repos' own stores, one restricted to the project tier only from the shared
// stores, and one asking for both returns hits from all of them. Every store
// holds the query word exactly once, so the tier is the only thing deciding
// which hits come back.
func TestSet_SearchRestrictedToATierReturnsOnlyThatTiersHits(t *testing.T) {
	set := narrowingSet(t)

	repoHits := [][3]string{
		{"repo", "repo-a", "gotchas/repo-a.md"},
		{"repo", "repo-b", "gotchas/repo-b.md"},
	}
	projectHits := [][3]string{
		{"project", "project", "gotchas/project.md"},
		{"project", "team", "gotchas/team.md"},
	}

	for name, tc := range map[string]struct {
		tier Tier
		want [][3]string
	}{
		"repo tier":    {TierRepo, repoHits},
		"project tier": {TierProject, projectHits},
		"both tiers":   {TierAll, append(append([][3]string{}, repoHits...), projectHits...)},
	} {
		t.Run(name, func(t *testing.T) {
			hits, err := set.Search("sextant", Selector{Tier: tc.tier})
			require.NoError(t, err)

			got := make([][3]string, len(hits))
			for i, h := range hits {
				got[i] = [3]string{h.Tier, h.Name, h.Path}
			}
			require.ElementsMatch(t, tc.want, got)
		})
	}
}

// Criterion 2: the same search narrowed to one store name returns hits only
// from that store, while unnarrowed it reaches every store in its tier.
func TestSet_SearchNarrowedToOneNameReturnsOnlyThatStoresHits(t *testing.T) {
	set := narrowingSet(t)

	narrowed, err := set.Search("sextant", Selector{Tier: TierRepo, Filter: []string{"repo-a"}})
	require.NoError(t, err)
	got := make([][3]string, len(narrowed))
	for i, h := range narrowed {
		got[i] = [3]string{h.Tier, h.Name, h.Path}
	}
	require.Equal(t, [][3]string{{"repo", "repo-a", "gotchas/repo-a.md"}}, got)

	unnarrowed, err := set.Search("sextant", Selector{Tier: TierRepo})
	require.NoError(t, err)
	got = make([][3]string, len(unnarrowed))
	for i, h := range unnarrowed {
		got[i] = [3]string{h.Tier, h.Name, h.Path}
	}
	require.ElementsMatch(t, [][3]string{
		{"repo", "repo-a", "gotchas/repo-a.md"},
		{"repo", "repo-b", "gotchas/repo-b.md"},
	}, got)
}

// Criterion 3, and the "a caller can predict what a request covers" success
// metric: every tier crossed with every shape of filter — none, one name,
// several — covers exactly the stores the selector names, and search, list,
// conventions and the always-applied load all cover the same set. The expected
// coverage of each cell is hand-written, and no cell is a special case.
func TestSet_SelectorCoverageMatrixIsUniformAcrossRetrievalPaths(t *testing.T) {
	set := narrowingSet(t)

	for _, tc := range []struct {
		name string
		sel  Selector
		want []string
	}{
		{"project tier, no narrowing", Selector{Tier: TierProject}, []string{"project", "team"}},
		{"project tier, one name", Selector{Tier: TierProject, Filter: []string{"team"}}, []string{"team"}},
		{"project tier, several names", Selector{Tier: TierProject, Filter: []string{"project", "team"}}, []string{"project", "team"}},
		{"repo tier, no narrowing", Selector{Tier: TierRepo}, []string{"repo-a", "repo-b"}},
		{"repo tier, one name", Selector{Tier: TierRepo, Filter: []string{"repo-a"}}, []string{"repo-a"}},
		{"repo tier, several names", Selector{Tier: TierRepo, Filter: []string{"repo-a", "repo-b"}}, []string{"repo-a", "repo-b"}},
		{"both tiers, no narrowing", Selector{Tier: TierAll}, []string{"project", "team", "repo-a", "repo-b"}},
		{"both tiers, one name", Selector{Tier: TierAll, Filter: []string{"repo-a"}}, []string{"repo-a"}},
		{"both tiers, several names", Selector{Tier: TierAll, Filter: []string{"repo-a", "team"}}, []string{"repo-a", "team"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := set.Search("sextant", tc.sel)
			require.NoError(t, err)
			var searched []string
			for _, h := range hits {
				searched = append(searched, h.Name)
			}
			require.ElementsMatch(t, tc.want, distinct(searched), "search")

			entries, err := set.List(tc.sel)
			require.NoError(t, err)
			var listed []string
			for _, e := range entries {
				listed = append(listed, e.Name)
			}
			require.ElementsMatch(t, tc.want, distinct(listed), "list")

			conventions, err := set.Conventions(tc.sel)
			require.NoError(t, err)
			var convened []string
			for _, c := range conventions {
				convened = append(convened, c.Name)
			}
			require.ElementsMatch(t, tc.want, distinct(convened), "conventions")

			applied, err := set.AlwaysAppliedEntries(tc.sel)
			require.NoError(t, err)
			var loaded []string
			for _, e := range applied {
				loaded = append(loaded, e.Name)
			}
			require.ElementsMatch(t, tc.want, distinct(loaded), "always-applied")
		})
	}
}

// Criterion 4: the always-applied load narrowed to one repository returns that
// repository's entries and no others — none from the other repository, and none
// from either shared, project-tier store.
func TestSet_AlwaysAppliedNarrowedToOneRepoExcludesSharedStores(t *testing.T) {
	set := narrowingSet(t)

	entries, err := set.AlwaysAppliedEntries(Selector{Tier: TierRepo, Filter: []string{"repo-a"}})
	require.NoError(t, err)
	require.ElementsMatch(t, []AlwaysAppliedEntry{
		{Tier: TierRepo, Name: "repo-a", Path: "conventions/repo-a.md", Content: "repo-a convention\n", Category: "conventions"},
		{Tier: TierRepo, Name: "repo-a", Path: "glossary/repo-a.md", Content: "repo-a term\n", Category: "glossary"},
	}, entries)
}

// Criterion 5: the always-applied load across both tiers, narrowed to one
// repository plus one shared store, returns exactly those two stores' entries.
func TestSet_AlwaysAppliedAcrossTiersNarrowedToARepoAndASharedStore(t *testing.T) {
	set := narrowingSet(t)

	entries, err := set.AlwaysAppliedEntries(Selector{Tier: TierAll, Filter: []string{"repo-b", "team"}})
	require.NoError(t, err)
	require.ElementsMatch(t, []AlwaysAppliedEntry{
		{Tier: TierProject, Name: "team", Path: "conventions/team.md", Content: "team convention\n", Category: "conventions"},
		{Tier: TierProject, Name: "team", Path: "glossary/team.md", Content: "team term\n", Category: "glossary"},
		{Tier: TierRepo, Name: "repo-b", Path: "conventions/repo-b.md", Content: "repo-b convention\n", Category: "conventions"},
		{Tier: TierRepo, Name: "repo-b", Path: "glossary/repo-b.md", Content: "repo-b term\n", Category: "glossary"},
	}, entries)
}

// The "planning loads less irrelevant knowledge" success metric: with four
// stores holding always-applied entries, narrowing the load to one of them
// returns strictly fewer entries than the same load with no narrowing, and
// returns exactly that store's entries.
func TestSet_NarrowedAlwaysAppliedLoadIsStrictlySmaller(t *testing.T) {
	set := narrowingSet(t)

	all, err := set.AlwaysAppliedEntries(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Len(t, all, 8, "four stores each holding one convention and one glossary term")

	narrowed, err := set.AlwaysAppliedEntries(Selector{Tier: TierAll, Filter: []string{"team"}})
	require.NoError(t, err)
	require.Less(t, len(narrowed), len(all), "narrowing must load less, not the same")
	require.ElementsMatch(t, []AlwaysAppliedEntry{
		{Tier: TierProject, Name: "team", Path: "conventions/team.md", Content: "team convention\n", Category: "conventions"},
		{Tier: TierProject, Name: "team", Path: "glossary/team.md", Content: "team term\n", Category: "glossary"},
	}, narrowed)
}

// Criterion 6: narrowing to a name the chosen tier does not contain is refused,
// on every retrieval path, with guidance naming the names that tier does hold —
// rather than quietly returning nothing for a caller to misread as "no such
// knowledge".
func TestSet_SelectorNamingAStoreOutsideTheTierIsRefused(t *testing.T) {
	set := narrowingSet(t)

	// "team" is a real store, but it lives in the project tier, so a repo-tier
	// request cannot reach it.
	outsideTier := Selector{Tier: TierRepo, Filter: []string{"team"}}
	for name, call := range map[string]func(Selector) error{
		"search":         func(sel Selector) error { _, err := set.Search("sextant", sel); return err },
		"list":           func(sel Selector) error { _, err := set.List(sel); return err },
		"conventions":    func(sel Selector) error { _, err := set.Conventions(sel); return err },
		"always-applied": func(sel Selector) error { _, err := set.AlwaysAppliedEntries(sel); return err },
		"tags":           func(sel Selector) error { _, err := set.Tags(sel); return err },
	} {
		t.Run(name, func(t *testing.T) {
			envelope := requireRefusal(t, call(outsideTier), ErrCodeStoreUnknown)
			require.Contains(t, envelope.Message, `"team"`)
			require.Contains(t, envelope.NextAction, `stores available in the "repo" tier: repo-a, repo-b`)

			// Across both tiers every configured store is offered, and no tier
			// is named, since the request narrowed to neither.
			envelope = requireRefusal(t, call(Selector{Tier: TierAll, Filter: []string{"repo-c"}}), ErrCodeStoreUnknown)
			require.Contains(t, envelope.NextAction, "stores available: project, team, repo-a, repo-b")
		})
	}
}

// Criterion 6: every retrieval path validates its selector before it opens a
// store, so a selector stating no tier at all, or an unrecognised one, is
// refused with the code saying which part is wrong.
func TestSet_RetrievalPathsRefuseAnEmptyOrUnknownTier(t *testing.T) {
	set := narrowingSet(t)

	for name, call := range map[string]func(Selector) error{
		"search":         func(sel Selector) error { _, err := set.Search("sextant", sel); return err },
		"list":           func(sel Selector) error { _, err := set.List(sel); return err },
		"conventions":    func(sel Selector) error { _, err := set.Conventions(sel); return err },
		"always-applied": func(sel Selector) error { _, err := set.AlwaysAppliedEntries(sel); return err },
		"tags":           func(sel Selector) error { _, err := set.Tags(sel); return err },
	} {
		t.Run(name, func(t *testing.T) {
			requireRefusal(t, call(Selector{}), ErrCodeTierRequired)
			requireRefusal(t, call(Selector{Tier: "shared"}), ErrCodeTierInvalid)
		})
	}
}

// Criterion 1: a write whose address omits the tier, the store name, or both is
// refused, records nothing in any store, and leaves the location empty — a
// later read of that same location, at a fully-stated address, finds nothing.
func TestSet_WriteWithoutAFullAddressRecordsNothing(t *testing.T) {
	cases := map[string]struct {
		addr Address
		code string
	}{
		"tier omitted": {Address{Name: "project"}, ErrCodeTierRequired},
		"name omitted": {Address{Tier: TierProject}, ErrCodeNameRequired},
		"both omitted": {Address{}, ErrCodeTierRequired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			set, projectDir, teamDir := twoScopeSet(t)

			requireRefusal(t, set.Write(tc.addr, "learnings/dropped.md", []byte("must not persist")), tc.code)

			require.NoFileExists(t, filepath.Join(projectDir, "learnings", "dropped.md"))
			require.NoFileExists(t, filepath.Join(teamDir, "learnings", "dropped.md"))

			for _, store := range []string{"project", "team"} {
				_, err := set.Read(Address{Tier: TierProject, Name: store}, "learnings/dropped.md")
				require.Error(t, err, "the refused write must leave nothing to read back in %q", store)
			}
		})
	}
}

// Criterion 2: the same omission is refused in a project holding exactly one
// store, where there is only one candidate an under-stated address could have
// meant. The single store is never guessed at.
func TestSet_WriteWithoutAFullAddressRefusedWithASingleStore(t *testing.T) {
	onlyDir := t.TempDir()
	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "project", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: onlyDir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)

	requireRefusal(t, set.Write(Address{Name: "project"}, "learnings/x.md", []byte("no")), ErrCodeTierRequired)
	requireRefusal(t, set.Write(Address{Tier: TierProject}, "learnings/x.md", []byte("no")), ErrCodeNameRequired)
	requireRefusal(t, set.Write(Address{}, "learnings/x.md", []byte("no")), ErrCodeTierRequired)

	require.NoFileExists(t, filepath.Join(onlyDir, "learnings", "x.md"))
}

// Phase 1.3 criterion 3: a removal whose address does not name exactly one
// store is refused with the same codes Write gives for the same omission, and
// it removes nothing anywhere — the listing afterwards is the untouched one.
// The refusal names the stores the tier does hold, so a caller can reissue.
func TestSet_DeleteWithoutAFullAddressRemovesNothing(t *testing.T) {
	cases := map[string]struct {
		addr       Address
		code       string
		nextAction string
	}{
		"tier omitted":  {Address{Name: "project"}, ErrCodeTierRequired, "stores available: project, team"},
		"name omitted":  {Address{Tier: TierProject}, ErrCodeNameRequired, `stores available in the "project" tier: project, team`},
		"both omitted":  {Address{}, ErrCodeTierRequired, "stores available: project, team"},
		"unknown store": {Address{Tier: TierProject, Name: "ghost"}, ErrCodeStoreUnknown, `stores available in the "project" tier: project, team`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			set, _, _ := twoScopeSet(t)

			envelope := requireRefusal(t, set.Delete(tc.addr, "readme.md"), tc.code)
			require.Contains(t, envelope.NextAction, tc.nextAction,
				"a refused removal must name the stores the request could have meant")

			entries, err := set.List(Selector{Tier: TierAll})
			require.NoError(t, err)
			require.ElementsMatch(t, []Entry{
				{Tier: TierProject, Name: "project", Path: "readme.md"},
				{Tier: TierProject, Name: "project", Path: "architecture/initial-idea.md"},
				{Tier: TierProject, Name: "team", Path: "guidelines.md"},
				{Tier: TierProject, Name: "team", Path: "architecture/overview.md"},
			}, entries, "an under-stated address must not be resolved to whichever store looks closest")
		})
	}
}

// Criterion 3: a refusal names the stores available in the tier concerned, and
// those names are exactly the ones Sources reports for that tier. The expected
// names are hand-written; the assertion that Sources agrees with them is what
// ties the guidance to the configuration a caller can inspect.
func TestSet_RefusalNamesTheStoresAvailableInTheTier(t *testing.T) {
	set, _, _, _ := repoScopedSet(t)

	var repoNames, projectNames []string
	for _, src := range set.Sources() {
		switch src.Tier {
		case TierRepo:
			repoNames = append(repoNames, src.Name)
		case TierProject:
			projectNames = append(projectNames, src.Name)
		}
	}
	require.Equal(t, []string{"repo-a", "repo-b"}, repoNames)
	require.Equal(t, []string{"project"}, projectNames)

	// An unknown name in the repo tier is told which repo-tier stores exist,
	// and is not offered the project tier's names.
	_, err := set.Read(Address{Tier: TierRepo, Name: "repo-c"}, "conventions/a.md")
	envelope := requireRefusal(t, err, ErrCodeStoreUnknown)
	require.Contains(t, envelope.NextAction, `stores available in the "repo" tier: repo-a, repo-b`)
	require.NotContains(t, envelope.NextAction, "project")

	// An omitted name in the repo tier is told the same list alongside the
	// instruction to restate the address.
	envelope = requireRefusal(t, set.Write(Address{Tier: TierRepo}, "conventions/a.md", []byte("x")), ErrCodeNameRequired)
	require.Contains(t, envelope.NextAction, `stores available in the "repo" tier: repo-a, repo-b`)

	// With no tier to narrow to, every store in either tier is offered.
	envelope = requireRefusal(t, set.Write(Address{Name: "repo-a"}, "conventions/a.md", []byte("x")), ErrCodeTierRequired)
	require.Contains(t, envelope.NextAction, `stores available: repo-a, repo-b, project`)
}

// Criterion 4: a write addressed to a named store is readable back at that same
// address, and the same location under any other name, or in the other tier,
// holds nothing.
func TestSet_WriteIsReadableOnlyAtTheAddressItWasWrittenTo(t *testing.T) {
	set, projectDir, repoADir, repoBDir := repoScopedSet(t)

	target := Address{Tier: TierRepo, Name: "repo-a"}
	require.NoError(t, set.Write(target, "learnings/note.md", []byte("repo-a note")))

	data, err := set.Read(target, "learnings/note.md")
	require.NoError(t, err)
	require.Equal(t, []byte("repo-a note"), data)
	require.FileExists(t, filepath.Join(repoADir, "learnings", "note.md"))

	// Another name in the same tier holds nothing at that location.
	_, err = set.Read(Address{Tier: TierRepo, Name: "repo-b"}, "learnings/note.md")
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(repoBDir, "learnings", "note.md"))

	// The other tier has no store by that name at all.
	_, err = set.Read(Address{Tier: TierProject, Name: "repo-a"}, "learnings/note.md")
	requireRefusal(t, err, ErrCodeStoreUnknown)
	require.NoFileExists(t, filepath.Join(projectDir, "learnings", "note.md"))
}

// Criterion 5: two repos holding an entry at the same location stay distinct —
// each name returns its own repo's content and never the other's, in either
// direction.
func TestSet_TwoRepoStoresAtTheSameLocationStayDistinct(t *testing.T) {
	set, _, repoADir, repoBDir := repoScopedSet(t)

	writeFile(t, repoADir, "learnings/dup.md", "repo-a version\n")
	writeFile(t, repoBDir, "learnings/dup.md", "repo-b version\n")

	fromA, err := set.Read(Address{Tier: TierRepo, Name: "repo-a"}, "learnings/dup.md")
	require.NoError(t, err)
	require.Equal(t, []byte("repo-a version\n"), fromA)

	fromB, err := set.Read(Address{Tier: TierRepo, Name: "repo-b"}, "learnings/dup.md")
	require.NoError(t, err)
	require.Equal(t, []byte("repo-b version\n"), fromB)

	// A write to one name overwrites only that repo's copy.
	require.NoError(t, set.Write(Address{Tier: TierRepo, Name: "repo-b"}, "learnings/dup.md", []byte("repo-b rewritten\n")))

	fromA, err = set.Read(Address{Tier: TierRepo, Name: "repo-a"}, "learnings/dup.md")
	require.NoError(t, err)
	require.Equal(t, []byte("repo-a version\n"), fromA)

	fromB, err = set.Read(Address{Tier: TierRepo, Name: "repo-b"}, "learnings/dup.md")
	require.NoError(t, err)
	require.Equal(t, []byte("repo-b rewritten\n"), fromB)
}

// Criterion 6: Read and Write both address exactly one store, so a tier that is
// neither of the two addressable tiers is rejected, and so is the fan-out tier
// "all" even though it is a declared tier.
func TestSet_ReadAndWriteRefuseFanOutAndUnknownTiers(t *testing.T) {
	set, _, repoADir, _ := repoScopedSet(t)

	for name, tier := range map[string]Tier{
		"fan-out tier": TierAll,
		"unknown tier": Tier("team"),
	} {
		t.Run(name, func(t *testing.T) {
			addr := Address{Tier: tier, Name: "repo-a"}

			_, err := set.Read(addr, "conventions/a.md")
			requireRefusal(t, err, ErrCodeTierInvalid)

			requireRefusal(t, set.Write(addr, "learnings/fanned.md", []byte("no")), ErrCodeTierInvalid)
			require.NoFileExists(t, filepath.Join(repoADir, "learnings", "fanned.md"))
		})
	}
}

// Criterion 4: a read at a correctly addressed store that holds no entry at
// the requested path is refused through the standard error envelope rather
// than escaping as a bare store error with no guidance. The refusal names the
// entry and the store it was looked for in, carries the path as the resource,
// and its next action is the exact narrowed listing that would show what that
// store does hold — the tier and the name the caller already used.
func TestSet_ReadMissingEntryOffersTheListingForThatStore(t *testing.T) {
	set, _, _, _ := repoScopedSet(t)

	_, err := set.Read(Address{Tier: TierRepo, Name: "repo-a"}, "learnings/absent.md")

	envelope := requireRefusal(t, err, ErrCodeEntryNotFound)
	require.Contains(t, envelope.Message, "learnings/absent.md")
	require.Contains(t, envelope.Message, "repo-a")
	require.Equal(t, "learnings/absent.md", envelope.Resource)
	require.Contains(t, envelope.NextAction, "knowledge list --tier repo --filter repo-a",
		"the correction must be runnable as printed, naming the store that was addressed")
	require.Contains(t, envelope.NextAction, "knowledge search")
}

// Phase 1.1 criterion 1/3: scoring is the knowledge layer's job, so the hits
// Set.Search returns carry a non-zero Score even though the store that produced
// them left it at zero. Both expected scores are hand-computed from the fixture
// and damp(n) = 1 + log2(n):
//
//   - notes/tools.md holds "widget" 3 times and "gear" 2 times, so
//     damp(3) + damp(2) = 2.5849625007211562 + 2 = 4.5849625007211562, with
//     coverage 2/2 = 1 leaving it unscaled. Irrational, hence InDelta.
//   - notes/partial.md holds "gear" once and "widget" never, so
//     damp(0) + damp(1) = 1, scaled by coverage (1/2)^2 = 0.25, giving 0.25
//     exactly.
//
// Only notes/tools.md comes back. Phase 1.3's relative cutoff puts the floor at
// 4.5849625007211562 * 0.25 = 1.146240625180289, and notes/partial.md's 0.25
// falls under it. That is a relevance judgement made against the strongest hit
// present, not the missing-term exclusion Phase 1.2 removed: partial.md still
// scores a real, non-zero 0.25, and would still be returned if nothing better
// than it existed (TestSet_SearchReturnsAnEntryMatchingOnlyOneOfThreeTerms
// pins exactly that). What this test asserts remains the stamping itself — the
// store leaves Score at zero and the knowledge layer must fill it in.
func TestSet_SearchStampsScoreOnHits(t *testing.T) {
	projectDir := t.TempDir()

	writeFile(t, projectDir, "notes/tools.md",
		"widget widget on the first line\n"+
			"a gear sits on the second line\n"+
			"another widget beside another gear\n")
	writeFile(t, projectDir, "notes/partial.md", "just a gear in here\n")

	set := singleSourceSet(t, projectDir)

	hits, err := set.Search("widget gear", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1,
		"notes/partial.md scores 0.25, below the 1.146240625180289 cutoff floor set by the strongest hit")

	require.Equal(t, "notes/tools.md", hits[0].Path)
	require.InDelta(t, 4.5849625007211562, hits[0].Score, 1e-9,
		"the knowledge layer must stamp the score the store left at zero")
}

// A hit's Score counts non-overlapping, case-insensitive occurrences of the
// query across the whole document, whether they sit on one line or are spread
// over several, and then damps that count. Uses its own fixture: scoring needs
// files with a known occurrence count, which twoScopeSet does not provide.
//
// Phase 1.2 changed the score from the raw occurrence count to damp(count) =
// 1 + log2(count), so spread.txt's four occurrences are worth damp(4) =
// 1 + log2(4) = 3 rather than 4. The query is a single term, so coverage is
// 1/1 = 1 for every hit and cannot move any of these numbers; each expected
// value is a power of two under the log and so is exact in float64, which is
// why require.Equal is still the right assertion here.
func TestSet_SearchScoreSumsOccurrencesAcrossDocument(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "twice.txt", "a needle next to another needle on one line\n")
	writeFile(t, dir, "once.txt", "just one needle here\n")
	writeFile(t, dir, "upper.txt", "a single NEEDLE in caps\n")
	// Occurrences spread over three lines: 1 + 2 + 1 = 4.
	writeFile(t, dir, "spread.txt",
		"first needle on its own line\n"+
			"a needle beside a NEEDLE midway\n"+
			"closing needle at the end\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("needle", Selector{Tier: TierAll})
	require.NoError(t, err)

	// Independent oracle: hand-maintained expected score per path, each the
	// damped occurrence count — damp(2) = 2, damp(1) = 1, damp(4) = 3. The
	// length check pins exactly one hit per matching document — the map alone
	// would hide a duplicate.
	want := map[string]float64{
		"twice.txt":  2,
		"once.txt":   1,
		"upper.txt":  1,
		"spread.txt": 3,
	}
	require.Len(t, hits, len(want))

	got := make(map[string]float64, len(hits))
	for _, h := range hits {
		got[h.Path] = h.Score
	}
	require.Equal(t, want, got)
}

// Phase 1.2 criteria 1 and 2: matching is a ranked OR, not boolean AND. A
// document missing one of the query's three terms is returned with a low score
// rather than discarded, and the document carrying evidence for all three ranks
// above it.
//
// This replaces the every-term requirement the same fixture used to prove. The
// store's behaviour is unchanged — it returns partial.txt with BodyCounts
// {1, 1, 0}, pinned by TestSearch_ReportsEvidenceWithoutScoring — but score()
// no longer zeroes evidence with a missing term, so Set.Search no longer drops
// it. Both files sit at the store root, so categoryOf returns "" for each and
// the always-applied exclusion plays no part here either.
//
// Hand-computed from damp(n) = 1 + log2(n) and the coverage factor
// (covered/total)^2:
//
//   - scattered.txt has all three terms once each: damp(1) * 3 = 3, coverage
//     3/3 = 1, so 3 exactly.
//   - partial.txt has two of three once each: 1 + 1 + 0 = 2, coverage 2/3, so
//     2 * (2/3)^2 = 8/9 = 0.888..., which is irrational in binary and hence
//     asserted with InDelta.
//
// Phase 1.3 added a relevance cutoff, and this fixture now doubles as the guard
// that it is not set too aggressively: the floor is 3 * cutoffFraction = 0.75,
// partial.txt's 8/9 = 0.889 clears it, and both hits must keep coming back. A
// future tuner raising cutoffFraction above 0.296 would break this test, and
// that is the point — a two-of-three match is a genuinely useful result and the
// cutoff exists to drop noise, not near misses.
func TestSet_SearchRanksPartialMatchesBelowFullOnes(t *testing.T) {
	dir := t.TempDir()
	// All three query words present, each on its own line, in reverse order.
	writeFile(t, dir, "scattered.txt",
		"charlie opens the file\n"+
			"some filler in between\n"+
			"bravo turns up later\n"+
			"and alpha closes it out\n")
	// Contains alpha and bravo but never charlie, so it is a genuine partial
	// match rather than a trivial non-match.
	writeFile(t, dir, "partial.txt",
		"alpha is here\n"+
			"and bravo is here too\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("alpha bravo charlie", Selector{Tier: TierAll})
	require.NoError(t, err)

	var paths []string
	for _, h := range hits {
		paths = append(paths, h.Path)
	}
	require.Equal(t, []string{"scattered.txt", "partial.txt"}, paths,
		"the partial match must be returned, ranked below the full one")

	require.Equal(t, float64(3), hits[0].Score)
	require.InDelta(t, 8.0/9.0, hits[1].Score, 1e-9)
}

// Phase 1.2 criterion 1 in its narrowest form: an entry carrying evidence for
// one query term out of three is returned rather than omitted. The store holds
// exactly one entry and it matches only "charlie", so if the weakest possible
// partial match were still discarded the search would come back empty — which
// is the failure the phase exists to fix.
//
// Hand-computed: damp(1) = 1 for "charlie", 0 for the two missing terms, and
// coverage (1/3)^2 = 1/9, so 1 * 1/9 = 0.111..., asserted with InDelta since
// a ninth is not exact in binary.
//
// This is also Phase 1.3 criterion 1 — an entry matching one of three query
// terms is returned when it is the only match. The relative cutoff cannot touch
// it: a lone hit is its own best score, so the floor is 1/9 * 0.25 = 1/36 and
// the hit sits comfortably above the bar it sets itself.
func TestSet_SearchReturnsAnEntryMatchingOnlyOneOfThreeTerms(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lonely.txt", "charlie stands here on his own\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("alpha bravo charlie", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1, "an entry matching one term of three must still be returned")
	require.Equal(t, "lonely.txt", hits[0].Path)
	require.InDelta(t, 1.0/9.0, hits[0].Score, 1e-9)
}

// Phase 1.3 criterion 2: the same weak entry is kept or dropped depending on
// what else the search found, which is what makes the cutoff relative rather
// than a fixed relevance threshold. Both halves run against the same query and
// the same weak fixture, and the only difference between them is the presence
// of a strong entry — stating the contrast inside one test is the point, since
// either half alone would prove nothing about the other.
//
// Hand-computed from damp(n) = 1 + log2(n) and coverage (covered/total)^2 over
// the three-term query "alpha bravo charlie":
//
//   - lonely.txt matches only "charlie", once: 1 * (1/3)^2 = 1/9 =
//     0.111..., asserted with InDelta since a ninth is not exact in binary.
//   - complete.txt matches all three once each: 3 * (3/3)^2 = 3 exactly.
//
// Alone, lonely.txt is its own best score and survives. Once complete.txt is
// present the floor becomes 3 * 0.25 = 0.75, and 0.111 falls well under it.
func TestSet_SearchDropsWeakMatchOnceAStrongOneIsPresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lonely.txt", "charlie stands here on his own\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("alpha bravo charlie", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1, "with nothing better present the weak match is the best there is")
	require.Equal(t, "lonely.txt", hits[0].Path)
	require.InDelta(t, 1.0/9.0, hits[0].Score, 1e-9)

	// Same store, same query; only a stronger entry is added.
	writeFile(t, dir, "complete.txt",
		"alpha opens the file\n"+
			"bravo turns up next\n"+
			"and charlie closes it out\n")

	hits, err = set.Search("alpha bravo charlie", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1,
		"lonely.txt scores 1/9 = 0.111, below the floor of 3 * 0.25 = 0.75 the strong hit sets")
	require.Equal(t, "complete.txt", hits[0].Path)
	require.Equal(t, float64(3), hits[0].Score)
}

// Phase 1.3 criterion 3: the cutoff is applied once, after every store has been
// searched and merged, so the bar a hit is judged against can be set by a hit
// from a different store entirely.
//
// This is the assertion that catches a cutoff computed per store. The weak entry
// is the only thing its own store holds, so a per-store floor would be
// 1/9 * 0.25 = 1/36 and it would survive; only a floor taken from the merged
// results — 3 * 0.25 = 0.75, set by the strong entry over in the second store —
// drops it. That sentence is why this test exists.
//
// Hand-computed over the three-term query "alpha bravo charlie":
//
//   - project/faint.txt matches only "charlie", once: 1 * (1/3)^2 = 1/9 = 0.111.
//   - team/complete.txt matches all three once each: 3 * (3/3)^2 = 3 exactly.
func TestSet_SearchCutoffAppliesAcrossStores(t *testing.T) {
	projectDir := t.TempDir()
	teamDir := t.TempDir()

	writeFile(t, projectDir, "faint.txt", "charlie stands here on his own\n")
	writeFile(t, teamDir, "complete.txt",
		"alpha opens the file\n"+
			"bravo turns up next\n"+
			"and charlie closes it out\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "project", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: projectDir}},
		{Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: teamDir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)

	hits, err := set.Search("alpha bravo charlie", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1,
		"the weak hit in the project store must be cut by the floor the team store's hit sets")
	require.Equal(t, "complete.txt", hits[0].Path)
	require.Equal(t, "team", hits[0].Name)
	require.Equal(t, float64(3), hits[0].Score)
}

// The cutoff bar is taken from the hits that survive the always-applied category
// exclusion, not from the raw merged slice. An always-applied entry is dropped
// from search results whatever it scores — its whole content is loaded on every
// task instead — so letting it set the bar would raise the threshold for
// everything else on behalf of a result nobody ever sees.
//
// "conventions" is an always-applied category per the registry in category.go
// (Categories, CategoryTierAlwaysApplied, projected by AlwaysApplied()), which
// is what Set.Search consults through alwaysAppliedSet().
//
// Hand-computed over the three-term query "alpha bravo charlie", using
// damp(n) = 1 + log2(n) and coverage (covered/total)^2:
//
//   - conventions/style.md matches all three terms once each: 3 * (3/3)^2 = 3.
//     It is the highest-scoring hit the store returns, and it is excluded.
//   - gotchas/trap.md matches only "charlie", once: 1 * (1/3)^2 = 1/9 = 0.111.
//
// The two candidate bars differ by a factor of 27, so the outcome is
// unambiguous. If the conventions entry set the bar the floor would be
// 3 * 0.25 = 0.75, 0.111 would fall under it, and the search would return
// nothing at all. Taking the bar from the survivors leaves gotchas/trap.md as
// the best remaining hit, floor 1/9 * 0.25 = 1/36 = 0.0277..., and it is
// returned.
func TestSet_SearchCutoffIgnoresAlwaysAppliedWhenSettingTheBar(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "conventions/style.md",
		"alpha opens the rule\n"+
			"bravo restates it\n"+
			"and charlie closes it out\n")
	writeFile(t, dir, "gotchas/trap.md", "charlie stands here on his own\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("alpha bravo charlie", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1,
		"the excluded conventions entry must not raise the floor for the hits that remain")
	require.Equal(t, "gotchas/trap.md", hits[0].Path)
	require.InDelta(t, 1.0/9.0, hits[0].Score, 1e-9)
}

// A document with query words on many lines collapses into exactly one hit,
// scored as the hand-computed sum of every term's damped occurrence count
// across the whole document.
//
// Moved up from internal/store in Phase 1.1 and re-scored in Phase 1.2: the
// fixture is unchanged, but each term's count is now damped before summing.
// Hand-computed: widget 3, gear 5, so damp(3) + damp(5) = 2.5849625007211562 +
// 3.321928094887362 = 5.906890595608518; both terms are present, so coverage is
// 2/2 = 1 and leaves the sum alone. That value is irrational, hence InDelta
// rather than an exact comparison. The one-hit-per-document collapsing this
// also covers is still asserted at store level by
// TestSearch_ReportsEvidenceWithoutScoring.
func TestSet_SearchOneHitPerDocument(t *testing.T) {
	dir := t.TempDir()
	// Hand-computed: widget 2+1 = 3, gear 1+1+3 = 5.
	writeFile(t, dir, "machine.txt",
		"widget widget on the first line\n"+
			"a gear sits on the second line\n"+
			"another widget and a gear together\n"+
			"gear gear gear to finish\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("widget gear", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1, "four matching lines must still collapse into one hit")
	require.Equal(t, "machine.txt", hits[0].Path)
	require.InDelta(t, 5.906890595608518, hits[0].Score, 1e-9)
}

// Phase 2.2 criterion 1: an entry is found for what it is about, not only for
// the words it happens to use. channels.md declares the tags "go" and "http"
// and its body contains neither word — deliberately, since a body mention would
// make the hit unremarkable — so the tags are the entire reason it is returned.
//
// Hand-computed from score = sum(tagWeight * affinity + damp(count)) *
// coverage^2 over the three terms of "go http router", with tagWeight = 8:
//
//   - "go" matches a tag exactly: 8 * 1 + damp(0) = 8.
//   - "http" matches a tag exactly: 8 * 1 + damp(0) = 8.
//   - "router" matches nothing at all: 0.
//
// Two of three terms carry evidence, so coverage is (2/3)^2 = 4/9 and the total
// is 16 * 4/9 = 7.111..., irrational in binary and hence asserted with InDelta.
func TestSet_SearchFindsAnEntryByItsTagsAlone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "channels.md",
		"---\n"+
			"tags: [go, http]\n"+
			"---\n"+
			"# Channel Notes\n"+
			"\n"+
			"Messages pass between workers without shared memory.\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("go http router", Selector{Tier: TierAll})
	require.NoError(t, err)

	require.Len(t, hits, 1, "the tagged entry must be found even though its prose matches no term")
	require.Equal(t, "channels.md", hits[0].Path)
	require.InDelta(t, 7.111111111111111, hits[0].Score, 1e-9,
		"two exact tag matches out of three terms: 16 * (2/3)^2")
}

// Phase 2.2 criterion 2: an entry tagged with a term outranks one that merely
// mentions it. Both entries are of comparable length and neither has any other
// advantage, so the tag is the only thing separating them.
//
// Hand-computed with tagWeight = 8 and damp(n) = 1 + log2(n) over the one-term
// query "http", where coverage is 1 for both and cannot move either number:
//
//   - tagged.md matches the tag exactly and never says "http" in its body:
//     8 * 1 + damp(0) = 8 exactly.
//   - prose.md says "http" three times and declares no tags:
//     damp(3) = 2.584962500721156.
//
// prose.md is given three mentions rather than one on purpose. A single mention
// scores damp(1) = 1, which falls under the relative cutoff floor of
// 8 * 0.25 = 2 and is dropped entirely — correct behaviour, but it would leave
// the test asserting a one-element slice and proving the ordering only by
// implication. Three mentions clear the floor (damp(3) = 2.585 > 2), so both
// hits come back and the ordering is stated directly, in the returned slice.
func TestSet_SearchRanksATaggedEntryAboveOneThatOnlyMentionsTheTerm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tagged.md",
		"---\n"+
			"tags: [http]\n"+
			"---\n"+
			"# Timeouts\n"+
			"\n"+
			"Set the request timeout before retrying.\n")
	writeFile(t, dir, "prose.md",
		"an http note in passing\n"+
			"another http line here\n"+
			"a third http line to finish\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("http", Selector{Tier: TierAll})
	require.NoError(t, err)

	paths := make([]string, len(hits))
	for i, h := range hits {
		paths[i] = h.Path
	}
	require.Equal(t, []string{"tagged.md", "prose.md"}, paths,
		"the tagged entry must rank above the one that only mentions the term")
	require.Equal(t, float64(8), hits[0].Score)
	require.InDelta(t, 2.584962500721156, hits[1].Score, 1e-9)
}

// Phase 2.2 criterion 3: volume cannot buy what a tag states. long.md mentions
// "cache" twenty times and declares no tags; tagged.md declares the tag and
// never says the word.
//
// Hand-computed over the one-term query "cache", coverage 1 for both:
//
//   - tagged.md: 8 * 1 + damp(0) = 8 exactly.
//   - long.md: damp(20) = 1 + log2(20) = 5.321928094887363.
//
// Twenty is not a cherry-picked number: no realistic occurrence count reaches
// the tag weight, because damp(n) = 8 needs n = 2^7 = 128 occurrences of one
// term in one entry. The floor here is 8 * 0.25 = 2, which long.md's 5.32
// clears comfortably, so both hits come back and the ordering is asserted
// rather than inferred from an absence.
func TestSet_SearchTagOutranksManyBodyMentions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tagged.md",
		"---\n"+
			"tags: [cache]\n"+
			"---\n"+
			"# Expiry\n"+
			"\n"+
			"Entries expire after an hour.\n")
	// Twenty lines, one occurrence each: twenty occurrences in total.
	writeFile(t, dir, "long.md", strings.Repeat("a cache line\n", 20))

	set := singleSourceSet(t, dir)

	hits, err := set.Search("cache", Selector{Tier: TierAll})
	require.NoError(t, err)

	paths := make([]string, len(hits))
	for i, h := range hits {
		paths[i] = h.Path
	}
	require.Equal(t, []string{"tagged.md", "long.md"}, paths,
		"a short tagged entry must outrank a long one that merely repeats the word")
	require.Equal(t, float64(8), hits[0].Score)
	require.InDelta(t, 5.321928094887363, hits[1].Score, 1e-9)
}

// Phase 2.2 criterion 4: a term that opens a tag earns proportional credit, and
// strictly less than the exact match would. The same fixture is searched twice
// so the two numbers are directly comparable — a second fixture would leave the
// difference resting on the two entries being otherwise identical.
//
// Hand-computed with tagWeight = 8, single-term queries so coverage is 1:
//
//   - "http" against the tag "https": 4 runes over 5 = 0.8, so 8 * 0.8 = 6.4.
//   - "https" against the tag "https": exact, so 8 * 1 = 8 exactly.
func TestSet_SearchTagPrefixScoresBelowTheExactTerm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "transport.md",
		"---\n"+
			"tags: [https]\n"+
			"---\n"+
			"# Transport\n"+
			"\n"+
			"Certificates are rotated by the platform team.\n")

	set := singleSourceSet(t, dir)

	partial, err := set.Search("http", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Len(t, partial, 1, "a prefix of the tag must still find the entry")
	require.Equal(t, "transport.md", partial[0].Path)
	require.InDelta(t, 6.4, partial[0].Score, 1e-9, "8 * (4/5)")

	full, err := set.Search("https", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Len(t, full, 1)
	require.Equal(t, float64(8), full[0].Score, "an exact tag match is the full tag weight")

	require.Less(t, partial[0].Score, full[0].Score,
		"the same entry must score lower for a prefix of its tag than for the tag itself")
}

// Phase 2.2 criterion 5: singular and plural find each other, so neither form
// has to be written down twice on the entry. Each direction gets its own store,
// because putting both tags in one would let a single entry answer both queries
// and prove nothing about either.
//
// Hand-computed: 5 runes over 6 in both directions, so 8 * 5/6 =
// 6.666666666666667 either way — the relation is symmetric by construction, not
// by two separate rules. Coverage is 1 for these single-term queries.
func TestSet_SearchMatchesSingularAndPluralTagsBothWays(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tag   string
		query string
	}{
		{name: "a plural tag is found by the singular term", tag: "apples", query: "apple"},
		{name: "a singular tag is found by the plural term", tag: "apple", query: "apples"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "fruit.md",
				"---\n"+
					"tags: ["+tc.tag+"]\n"+
					"---\n"+
					"# Storage\n"+
					"\n"+
					"Stored in the cellar over winter.\n")

			set := singleSourceSet(t, dir)

			hits, err := set.Search(tc.query, Selector{Tier: TierAll})
			require.NoError(t, err)
			require.Len(t, hits, 1)
			require.Equal(t, "fruit.md", hits[0].Path)
			require.InDelta(t, 6.666666666666667, hits[0].Score, 1e-9, "8 * (5/6)")
		})
	}
}

// Phase 2.2 criterion 6: a compound tag is a real but weak match for the term it
// opens with, and ranks below an entry tagged with that term exactly.
//
// Hand-computed over the one-term query "https", coverage 1 for both:
//
//   - exact.md, tagged "https": 8 * 1 = 8 exactly.
//   - compound.md, tagged "https-security": 5 runes over 14 =
//     0.35714285714285715, so 8 * that = 2.857142857142857.
//
// Both are returned. The floor is 8 * 0.25 = 2 and the compound entry's 2.857
// clears it, which is the intended outcome rather than a coincidence: a
// narrower entry on the same subject is a useful result, just not the first
// one.
func TestSet_SearchCompoundTagRanksBelowTheExactTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exact.md",
		"---\n"+
			"tags: [https]\n"+
			"---\n"+
			"# Transport\n"+
			"\n"+
			"Certificates are rotated quarterly.\n")
	writeFile(t, dir, "compound.md",
		"---\n"+
			"tags: [https-security]\n"+
			"---\n"+
			"# Ciphers\n"+
			"\n"+
			"Ciphers are pinned by the platform team.\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("https", Selector{Tier: TierAll})
	require.NoError(t, err)

	paths := make([]string, len(hits))
	for i, h := range hits {
		paths[i] = h.Path
	}
	require.Equal(t, []string{"exact.md", "compound.md"}, paths,
		"the compound-tagged entry must be returned, ranked below the exactly tagged one")
	require.Equal(t, float64(8), hits[0].Score)
	require.InDelta(t, 2.857142857142857, hits[1].Score, 1e-9, "8 * (5/14)")
}

// Phase 2.2 criterion 9: where a term is prefix-related to more than one of an
// entry's tags, the strongest match decides and the matches do not add. This
// entry declares both "https" and "https-security", so an accumulating rule
// would score 8 * 1 + 8 * 5/14 = 10.857142857142858 and let an entry buy rank by
// declaring several spellings of one subject. The contract is 8 exactly — the
// same score TestSet_SearchCompoundTagRanksBelowTheExactTag pins for the
// exactly tagged entry that carries only one of these two tags.
func TestSet_SearchStrongestTagMatchWinsRatherThanAccumulating(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "transport.md",
		"---\n"+
			"tags: [https, https-security]\n"+
			"---\n"+
			"# Transport\n"+
			"\n"+
			"Certificates are rotated quarterly.\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("https", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, float64(8), hits[0].Score,
		"two matching tags must not earn more than the strongest one of them")
}

// Phase 2.2 criteria 7 and 8 at the search level, where the consequence of the
// two guards is visible: an entry whose tags do not genuinely match earns
// nothing from them and is not returned at all.
//
// This also pins the whole path the Phase 2.2 candidacy rule opened. Both
// entries declare tags, so the store reports both as candidates on their tags
// alone — it does not judge relevance. The ranking layer scores them at 0 and
// Set.Search drops them, which is where the judgement belongs. A search that
// returned either file would mean the store's looser candidacy had leaked out
// as a result.
//
//   - "go" earns nothing from the tag "golang": the shorter side is 2 runes,
//     below minPrefixLen = 4, so short terms match exactly or not at all.
//   - "test" earns nothing from the tag "rest": the strings share no opening,
//     and a similarity measure that scored them alike would return a document
//     about REST for a query about testing.
func TestSet_SearchTagCreditNeedsAnOpeningAndEnoughLength(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "golang.md",
		"---\n"+
			"tags: [golang]\n"+
			"---\n"+
			"# Runtime\n"+
			"\n"+
			"The scheduler multiplexes work onto threads.\n")
	writeFile(t, dir, "rest.md",
		"---\n"+
			"tags: [rest]\n"+
			"---\n"+
			"# Resources\n"+
			"\n"+
			"Resources are addressed by URL.\n")

	set := singleSourceSet(t, dir)

	for _, tc := range []struct {
		name  string
		query string
	}{
		{name: "a two-letter term earns nothing from a longer tag", query: "go"},
		{name: "a term sharing no opening with a tag earns nothing", query: "test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := set.Search(tc.query, Selector{Tier: TierAll})
			require.NoError(t, err)
			require.Empty(t, hits,
				"a tag that does not genuinely match must earn no credit, so the entry is not a hit")
		})
	}
}

// Phase 2.2 criterion 10: every hit reports the tags of the entry it came from,
// and an untagged entry reports an empty list rather than nothing at all — so a
// consumer never has to tell "this entry has no tags" from "this provider does
// not do tags". Both fixtures mention the query word once in their bodies, so
// both score damp(1) = 1 and neither can be cut, and the tags are the only
// thing that differs between them.
//
// require.Empty alone would pass on a nil slice, which is exactly the value
// this asserts against, so the untagged entry is checked with require.NotNil
// first.
func TestSet_SearchHitsCarryTagsAndAnEmptyListWhenUntagged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tagged.md",
		"---\n"+
			"tags: [navigation, safety]\n"+
			"---\n"+
			"# Beacons\n"+
			"\n"+
			"the lighthouse stands on the point\n")
	writeFile(t, dir, "plain.md", "the lighthouse is visible from here\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("lighthouse", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Len(t, hits, 2)

	tags := map[string][]string{}
	for _, h := range hits {
		tags[h.Path] = h.Tags
	}
	require.Equal(t, []string{"navigation", "safety"}, tags["tagged.md"],
		"a hit must report its entry's declared tags, in the order they were declared")
	require.NotNil(t, tags["plain.md"],
		"an untagged entry's hit must carry an allocated empty slice, not nil")
	require.Empty(t, tags["plain.md"])
}

// tagPaths returns the paths of hits in the order Search returned them, so a
// tag-narrowing test states its expectation as a hand-written list.
func tagPaths(hits []store.Hit) []string {
	paths := make([]string, 0, len(hits))
	for _, h := range hits {
		paths = append(paths, h.Path)
	}
	return paths
}

// Phase 2.3 criteria 1 and 2 together: narrowing to a tag returns only entries
// carrying it, and an entry lacking it is never returned however well it scores.
//
// The excluded entry is deliberately the higher-scoring one. loud.md says
// "http" 200 times, which is what it takes to beat a tag: damp(200) =
// 1 + log2(200) = 8.644, against tagged.md's exact tag match of
// tagWeight * 1 + damp(0) = 8. The unnarrowed search asserts that ordering
// directly, so the narrowed search dropping loud.md can only be the filter
// acting — a ranking tweak that merely demoted untagged entries would still
// have returned it, and would still pass a test where the tagged entry happened
// to score higher anyway.
func TestSet_SearchTagFilterExcludesAHigherScoringUntaggedEntry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tagged.md",
		"---\n"+
			"tags: [http]\n"+
			"---\n"+
			"# Timeouts\n"+
			"\n"+
			"Set the request timeout before retrying.\n")
	writeFile(t, dir, "loud.md",
		"# Protocol Notes\n"+
			"\n"+
			strings.Repeat("http ", 200)+"\n")

	set := singleSourceSet(t, dir)

	all, err := set.Search("http", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []string{"loud.md", "tagged.md"}, tagPaths(all),
		"unnarrowed, the untagged entry outranks the tagged one")

	narrowed, err := set.Search("http", Selector{Tier: TierAll, Tags: []string{"http"}})
	require.NoError(t, err)
	require.Equal(t, []string{"tagged.md"}, tagPaths(narrowed),
		"the filter is absolute: the higher-scoring untagged entry is excluded outright")
	require.Equal(t, 8.0, narrowed[0].Score,
		"one exact tag match on a one-term query: tagWeight * 1 + damp(0)")
}

// Phase 2.3 criterion 3: repeated tags are AND, not OR — an entry must carry
// every tag listed. Only the entry declaring both tags survives, and the
// unnarrowed search shows all three would otherwise be returned.
func TestSet_SearchWithTwoTagsReturnsOnlyEntriesCarryingBoth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "http-only.md",
		"---\n"+
			"tags: [http]\n"+
			"---\n"+
			"# Inbound\n"+
			"\n"+
			"the gateway accepts inbound calls\n")
	writeFile(t, dir, "routing-only.md",
		"---\n"+
			"tags: [routing]\n"+
			"---\n"+
			"# Paths\n"+
			"\n"+
			"the gateway resolves paths to handlers\n")
	writeFile(t, dir, "both.md",
		"---\n"+
			"tags: [http, routing]\n"+
			"---\n"+
			"# Edge\n"+
			"\n"+
			"the gateway sits at the edge\n")

	set := singleSourceSet(t, dir)

	all, err := set.Search("gateway", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"http-only.md", "routing-only.md", "both.md"}, tagPaths(all),
		"all three entries mention the query term once and score identically")

	narrowed, err := set.Search("gateway", Selector{Tier: TierAll, Tags: []string{"http", "routing"}})
	require.NoError(t, err)
	require.Equal(t, []string{"both.md"}, tagPaths(narrowed),
		"an entry carrying only one of the two listed tags must not be returned")
}

// tagBlindStore is a store that reports a fixed set of hits whatever options it
// is handed, and records the options it saw. It stands in for a provider that
// ignores SearchOptions.Tags — through negligence, a backend that cannot express
// the filter, or a bug — which is the case the knowledge layer's own enforcement
// exists to cover.
type tagBlindStore struct {
	hits    []store.Hit
	sawTags []string
}

func (s *tagBlindStore) Root() string                          { return "" }
func (s *tagBlindStore) Read(string) ([]byte, error)           { return nil, store.ErrNotFound }
func (s *tagBlindStore) Write(string, []byte) error            { return nil }
func (s *tagBlindStore) Delete(string) error                   { return nil }
func (s *tagBlindStore) List(string) ([]store.DirEntry, error) { return nil, nil }
func (s *tagBlindStore) Exists(string) bool                    { return false }
func (s *tagBlindStore) Stat(string) (store.FileInfo, error) {
	return store.FileInfo{}, store.ErrNotFound
}

func (s *tagBlindStore) Search(_ []string, opts store.SearchOptions) ([]store.Hit, error) {
	s.sawTags = opts.Tags
	return s.hits, nil
}

// Phase 2.3, the authority test: the knowledge layer is what guarantees the tag
// filter, and the store's copy of it is only an optimisation. A store that
// ignores SearchOptions.Tags entirely must not be able to leak an entry lacking
// the requested tag into the results.
//
// The Set is built in-package rather than through NewSet because NewSet resolves
// a config into file-backed stores and has no injection point for a provider —
// that is by design, since providers are declared in configuration. Building the
// scopedStore directly exercises the real Set.Search over the real source list,
// with only the provider swapped, which is the seam the guarantee actually sits
// on.
//
// The hand-computed scores over the one-term query "http" show neither hit is
// dropped by ranking: tagged scores tagWeight * 1 + damp(2) = 8 + 2 = 10, and
// untagged scores damp(5) = 1 + log2(5) = 3.322, comfortably above the relative
// cutoff floor of 10 * 0.25 = 2.5. The unnarrowed search asserts both come back,
// so the narrowed one returning a single hit is the post-merge filter and
// nothing else.
func TestSet_SearchEnforcesTagsEvenWhenTheStoreIgnoresThem(t *testing.T) {
	fake := &tagBlindStore{hits: []store.Hit{
		{Path: "learnings/tagged.md", Title: "Tagged", Tags: []string{"http"}, BodyCounts: []int{2}},
		{Path: "learnings/untagged.md", Title: "Untagged", Tags: []string{"database"}, BodyCounts: []int{5}},
	}}
	set := &Set{sources: []scopedStore{{
		tier:     TierProject,
		name:     "blind",
		provider: "test",
		location: "in-memory",
		store:    fake,
	}}}

	all, err := set.Search("http", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []string{"learnings/tagged.md", "learnings/untagged.md"}, tagPaths(all),
		"the fake reports both hits and both clear the cutoff")

	narrowed, err := set.Search("http", Selector{Tier: TierAll, Tags: []string{"http"}})
	require.NoError(t, err)
	require.Equal(t, []string{"learnings/tagged.md"}, tagPaths(narrowed),
		"the knowledge layer must exclude the untagged hit the store handed back regardless")
	require.Equal(t, []string{"http"}, fake.sawTags,
		"the store is still offered the filter as a fast path, it is simply not trusted on it")
}

// Phase 2.3 criterion 1, across stores: Selector.covers stays tag-blind, so a
// store whose entries mostly lack the tag is still searched. The tag here exists
// in the second store only; if covers consulted Tags, that store would be
// skipped and the search would find nothing at all.
func TestSet_SearchTagPresentInOneStoreStillReachesEveryStore(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	writeFile(t, firstDir, "notes.md", "the compass points north\n")
	writeFile(t, secondDir, "bearings.md",
		"---\n"+
			"tags: [navigation]\n"+
			"---\n"+
			"# Bearings\n"+
			"\n"+
			"the compass is read against the chart\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Name: "first", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: firstDir}},
		{Name: "second", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: secondDir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)

	all, err := set.Search("compass", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"notes.md", "bearings.md"}, tagPaths(all),
		"both stores hold an entry mentioning the query term")

	narrowed, err := set.Search("compass", Selector{Tier: TierAll, Tags: []string{"navigation"}})
	require.NoError(t, err)
	require.Equal(t, []string{"bearings.md"}, tagPaths(narrowed),
		"the second store must still be searched; tags narrow entries, never whole stores")
}

// Phase 3.1 criteria 1 and 2: the vocabulary reports every distinct tag with the
// number of entries carrying it, most-used first, and equally-used tags in
// alphabetical order so the listing is stable across runs.
//
// The fixture is built so both halves of the ordering rule are load-bearing:
// "http" is carried by three entries, and "cache" and "routing" are carried by
// two each — a tie that can only be resolved by the alphabetical rule. Counting
// by hand: http appears in a.md, b.md and c.md; routing in a.md and d.md; cache
// in b.md and e.md.
func TestSet_TagsReportsEachTagWithItsEntryCountMostUsedFirst(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/a.md", "---\ntags: [http, routing]\n---\n# A\n\nfirst note\n")
	writeFile(t, dir, "learnings/b.md", "---\ntags: [http, cache]\n---\n# B\n\nsecond note\n")
	writeFile(t, dir, "learnings/c.md", "---\ntags: [http]\n---\n# C\n\nthird note\n")
	writeFile(t, dir, "gotchas/d.md", "---\ntags: [routing]\n---\n# D\n\nfourth note\n")
	writeFile(t, dir, "decisions/e.md", "---\ntags: [cache]\n---\n# E\n\nfifth note\n")

	set := singleSourceSet(t, dir)

	uses, err := set.Tags(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []TagUse{
		{Tag: "http", Count: 3},
		{Tag: "cache", Count: 2},
		{Tag: "routing", Count: 2},
	}, uses, "the whole slice is compared, so the order is part of the assertion")
}

// Phase 3.1 criterion 4: a knowledge base whose entries carry no frontmatter at
// all — which is every knowledge base that existed before tags — reports an
// empty vocabulary rather than failing.
func TestSet_TagsOnAnUntaggedKnowledgeBaseIsEmptyNotAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/plain.md", "# Plain\n\nno frontmatter block at all\n")
	writeFile(t, dir, "gotchas/also-plain.md", "# Also Plain\n\nstill no block\n")

	set := singleSourceSet(t, dir)

	uses, err := set.Tags(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Empty(t, uses)
}

// Phase 3.1: the always-applied categories are excluded from the vocabulary,
// consistent with search — their entries are loaded in full on every task and
// never surfaced as hits, so their tags are not part of the vocabulary a new
// entry chooses from. Both categories in the registry's always-applied tier
// carry a tag here, and neither may appear; the looked-up sibling does.
func TestSet_TagsExcludesAlwaysAppliedCategories(t *testing.T) {
	require.Equal(t, []string{"conventions", "glossary"}, AlwaysApplied(),
		"the fixture below covers whichever categories the registry declares always-applied")

	dir := t.TempDir()
	writeFile(t, dir, "conventions/naming.md", "---\ntags: [naming]\n---\n# Naming\n\nname things well\n")
	writeFile(t, dir, "glossary/beacon.md", "---\ntags: [vocabulary]\n---\n# Beacon\n\na beacon is a marker\n")
	writeFile(t, dir, "learnings/timeouts.md", "---\ntags: [http]\n---\n# Timeouts\n\nset the timeout\n")

	set := singleSourceSet(t, dir)

	uses, err := set.Tags(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []TagUse{{Tag: "http", Count: 1}}, uses,
		"an always-applied entry's tags contribute nothing, however they are written")
}

// Phase 3.1 criterion 3: the vocabulary honours narrowing exactly as the other
// listing paths do. Counts are summed across the stores in scope: "http" is
// carried by one entry in each store, so the unnarrowed listing must report it
// twice — a per-store count would report 1 here and pass every other assertion.
func TestSet_TagsCountsAcrossStoresAndHonoursNarrowing(t *testing.T) {
	repoDir := t.TempDir()
	teamDir := t.TempDir()
	writeFile(t, repoDir, "learnings/one.md", "---\ntags: [http, deploy]\n---\n# One\n\nrepo note\n")
	writeFile(t, teamDir, "learnings/two.md", "---\ntags: [http]\n---\n# Two\n\nteam note\n")

	cfg := config.NewDefault()
	cfg.Knowledge.Sources = []config.SourceConfig{
		{Tier: string(TierRepo), Name: "repo-a", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: repoDir}},
		{Tier: string(TierProject), Name: "team", Provider: config.ProviderFile, Config: config.FileKnowledgeConfig{Location: teamDir}},
	}
	set, err := NewSet(cfg, t.TempDir())
	require.NoError(t, err)

	all, err := set.Tags(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []TagUse{
		{Tag: "http", Count: 2}, // one entry in each store
		{Tag: "deploy", Count: 1},
	}, all)

	repoTier, err := set.Tags(Selector{Tier: TierRepo})
	require.NoError(t, err)
	require.Equal(t, []TagUse{
		{Tag: "deploy", Count: 1}, // tied at one entry each, so alphabetical
		{Tag: "http", Count: 1},
	}, repoTier)

	narrowed, err := set.Tags(Selector{Tier: TierProject, Filter: []string{"team"}})
	require.NoError(t, err)
	require.Equal(t, []TagUse{{Tag: "http", Count: 1}}, narrowed,
		"only the narrowed store's tags are reported")
}

// Phase 3.1: one entry with a properly delimited but unparseable YAML block must
// not make the vocabulary unreadable. It contributes no tags — the same outcome
// a scan gives it — while its well-formed neighbour is still counted.
func TestSet_TagsSurvivesAMalformedFrontmatterBlock(t *testing.T) {
	dir := t.TempDir()
	// The flow sequence is never closed, which is a YAML scanner error.
	writeFile(t, dir, "learnings/broken.md", "---\ntags: [http\n---\nprose here\n")
	writeFile(t, dir, "learnings/good.md", "---\ntags: [routing]\n---\n# Good\n\nprose here\n")

	set := singleSourceSet(t, dir)

	uses, err := set.Tags(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []TagUse{{Tag: "routing", Count: 1}}, uses,
		"the malformed entry yields no tags, and the listing still reports the good one")
}

// Phase 3.1 criterion 1 of the refusal: a search narrowed to a tag no entry in
// scope carries is refused rather than returning an empty result, and the
// refusal names both the offending tag and the vocabulary that is in use, so the
// caller can reissue without going and reading every entry.
func TestSet_SearchUnknownTagIsRefusedNamingTheVocabulary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/tagged.md", "---\ntags: [http]\n---\n# Beacon\n\nthe beacon signal is strong\n")

	set := singleSourceSet(t, dir)

	_, err := set.Search("beacon", Selector{Tier: TierAll, Tags: []string{"no-such-tag"}})
	envelope := requireRefusal(t, err, ErrCodeTagUnknown)
	require.Contains(t, envelope.Message, "no-such-tag")
	require.Contains(t, envelope.NextAction, "knowledge tags")
	require.Contains(t, envelope.NextAction, "http", "the refusal names the tags that are in use")
}

// Phase 3.1, the distinction the refusal turns on: a tag that IS in the
// vocabulary, whose entries simply did not match the query, returns an empty
// result and no error. That is the honest answer, and without this test the
// refusal could over-fire and turn every fruitless tag-narrowed search into an
// error. The entry carries "http" and the query terms appear nowhere in it.
func TestSet_SearchKnownTagWithNoMatchingEntryReturnsEmptyNotARefusal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/tagged.md", "---\ntags: [http]\n---\n# Gateway\n\nthe gateway accepts inbound calls\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("sextant", Selector{Tier: TierAll, Tags: []string{"http"}})
	require.NoError(t, err)
	require.Empty(t, hits)
}

// Phase 3.1: the refusal path must not fire when no tag was requested at all. A
// search that simply matches nothing still returns an empty result and no error.
func TestSet_SearchWithNoTagNarrowingStillReturnsEmptyOnNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/tagged.md", "---\ntags: [http]\n---\n# Gateway\n\nthe gateway accepts inbound calls\n")

	set := singleSourceSet(t, dir)

	hits, err := set.Search("sextant", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Empty(t, hits)
}

// deleteRecordingStore is a store that holds no files at all and simply records
// every path it was asked to remove. It stands in for a provider that is not a
// directory on disk — a remote or database-backed store — which is the case the
// knowledge layer's routing through store.Store exists to cover.
type deleteRecordingStore struct {
	deleted []string
}

func (s *deleteRecordingStore) Root() string                          { return "" }
func (s *deleteRecordingStore) Read(string) ([]byte, error)           { return nil, store.ErrNotFound }
func (s *deleteRecordingStore) Write(string, []byte) error            { return nil }
func (s *deleteRecordingStore) List(string) ([]store.DirEntry, error) { return nil, nil }
func (s *deleteRecordingStore) Exists(string) bool                    { return false }
func (s *deleteRecordingStore) Stat(string) (store.FileInfo, error) {
	return store.FileInfo{}, store.ErrNotFound
}

func (s *deleteRecordingStore) Search([]string, store.SearchOptions) ([]store.Hit, error) {
	return nil, nil
}

func (s *deleteRecordingStore) Delete(path string) error {
	s.deleted = append(s.deleted, path)
	return nil
}

// Phase 1.3 criterion 7: a removal reaches the store through the same storage
// abstraction reading and writing use, so it works against any provider and not
// only a directory of files. The subject is the routing, so the stores here are
// deliberately not FileStores: nothing on disk is touched, and the proof is that
// the addressed provider — and only the addressed provider — saw the call.
//
// The Set is built in-package rather than through NewSet for the same reason
// TestSet_SearchEnforcesTagsEvenWhenTheStoreIgnoresThem does: NewSet resolves a
// config into file-backed stores and has no injection point for a provider, by
// design. Building the scopedStore directly exercises the real Set.Delete over
// the real source list, with only the provider swapped.
func TestSet_DeleteTravelsThroughTheStoreInterface(t *testing.T) {
	addressed := &deleteRecordingStore{}
	other := &deleteRecordingStore{}
	set := &Set{sources: []scopedStore{
		{tier: TierProject, name: "remote", provider: "test", location: "in-memory", store: addressed},
		{tier: TierRepo, name: "elsewhere", provider: "test", location: "in-memory", store: other},
	}}

	require.NoError(t, set.Delete(Address{Tier: TierProject, Name: "remote"}, "learnings/x.md"))

	require.Equal(t, []string{"learnings/x.md"}, addressed.deleted,
		"the removal must be handed to the addressed store through the Store interface")
	require.Empty(t, other.deleted, "no other store may be asked to remove anything")

	// A refused address never reaches a provider at all.
	requireRefusal(t, set.Delete(Address{Tier: TierProject, Name: "ghost"}, "learnings/x.md"), ErrCodeStoreUnknown)
	require.Equal(t, []string{"learnings/x.md"}, addressed.deleted,
		"a refused address must not reach any provider")
	require.Empty(t, other.deleted)
}
