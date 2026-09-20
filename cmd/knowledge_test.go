package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/repo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// knowledgeHit mirrors the store.Hit JSON envelope emitted inside a search
// result. Tier and Name are the address of the store the hit came from, and
// together with Path they are everything a read needs.
//
// Tags is emitted for every hit, as an empty list rather than an absent key
// when the entry declares none. Expected literals below therefore state
// Tags: []string{} explicitly: leaving the field out would compare a nil slice
// against the decoded [], which fails on a distinction none of those tests are
// about.
type knowledgeHit struct {
	Tier     string   `json:"tier"`
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Title    string   `json:"title"`
	Excerpts []string `json:"excerpts"`
	Score    float64  `json:"score"`
	Tags     []string `json:"tags"`
}

// knowledgeEntry mirrors the knowledge.Entry JSON envelope emitted by list.
type knowledgeEntry struct {
	Tier string `json:"tier"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// knowledgeSource mirrors the knowledge.SourceInfo JSON envelope emitted by
// sources. Tier and Name together are the store's address: a store declared by
// a registered repo is in the "repo" tier under that repo's registry name, and
// anything the project declares itself is in the "project" tier under its own
// declared name.
type knowledgeSource struct {
	Tier     string `json:"tier"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Location string `json:"location"`
}

// knowledgeAddressResult mirrors the JSON envelope read and write echo back:
// the address the request named plus the entry's path, and — for read only —
// the entry's content.
type knowledgeAddressResult struct {
	Tier    string `json:"tier"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// alwaysAppliedEntry mirrors the knowledge.AlwaysAppliedEntry JSON envelope
// emitted by always-applied.
type alwaysAppliedEntry struct {
	Tier     string `json:"tier"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Category string `json:"category"`
}

// knowledgeCategory mirrors the knowledge.Category JSON envelope emitted by categories.
type knowledgeCategory struct {
	Name       string `json:"name"`
	Purpose    string `json:"purpose"`
	Boundary   string `json:"boundary"`
	Tier       string `json:"tier"`
	EntryShape string `json:"entryShape"`
}

// knowledgeNarrowingCmds are the subcommands that carry the --tier and
// --filter narrowing options. They are listed once here so a command gaining
// or losing the options is a single edit in the tests, and so resetting can
// never miss one.
var knowledgeNarrowingCmds = []*cobra.Command{
	knowledgeSearchCmd,
	knowledgeListCmd,
	knowledgeConventionsCmd,
	knowledgeAlwaysAppliedCmd,
	knowledgeTagsCmd,
}

// twoScopeProject lays out a temp project rooted at a t.TempDir() and chdirs
// into it. Two file-backed knowledge stores are configured. The first comes
// from the colocated repo's config defaults (its repo.yaml declares the
// default source at .spektacular/knowledge), so it is addressed in the repo
// tier under the repo's registry name, "testproj". The second is declared by
// the project itself in .spektacular/config.yaml, so it is addressed in the
// project tier under its declared name, "team". Each store is seeded with a
// top-level file, and the repo-tier store also carries a file nested in a
// subdirectory. The keyword "compass" appears in one file per store. It
// returns the project root plus the two store locations.
func twoScopeProject(t *testing.T) (root, projectLoc, teamLoc string) {
	t.Helper()
	root = t.TempDir()
	t.Chdir(root)

	dataDir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))

	projectLoc = filepath.Join(dataDir, "knowledge")
	teamLoc = filepath.Join(root, "team-knowledge")

	seed := func(loc, name, content string) {
		full := filepath.Join(loc, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	seed(projectLoc, "readme.md", "project readme: the compass points north\n")
	seed(projectLoc, "architecture/initial-idea.md", "an architecture note about widgets\n")
	seed(teamLoc, "guidelines.md", "team guidelines reference the compass too\n")

	cfg := "name: testproj\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: .\n" +
		"knowledge:\n" +
		"  sources:\n" +
		"    - name: team\n" +
		"      provider: file\n" +
		"      config:\n" +
		"        location: " + teamLoc + "\n"
	writeCurrentConfig(t, root, cfg)
	require.NoError(t, config.NewDefaultRepoConfig().ToYAMLFile(filepath.Join(dataDir, config.RepoConfigFileName)))

	return root, projectLoc, teamLoc
}

// runKnowledge invokes the knowledge command tree via runRoot (the same
// wrapper Execute uses) and returns the captured stdout and stderr buffers,
// reusing the setupImplementCmd harness from implement_test.go and the
// t.Chdir working-dir pattern from spec_test.go. On failure, err is the
// *output.ErrorResponse unmarshaled from stdout, matching what a real
// invocation of the CLI returns.
func runKnowledge(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetRootCmd(t)
	out, errBuf := setupImplementCmd(t)
	rootCmd.SetArgs(append([]string{"knowledge"}, args...))
	if code := runRoot(); code != 0 {
		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal(out.Bytes(), &er))
		err = &er
	}
	return out.String(), errBuf.String(), err
}

// Criterion 1 & 2: `knowledge sources` lists every configured store by its
// address, provider, and resolved location in the documented
// {"sources":[...]} envelope.
func TestKnowledgeSources_ListsConfiguredStores(t *testing.T) {
	_, projectLoc, teamLoc := twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)

	var result struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	// The colocated repo's store is addressed in the repo tier under that
	// repo's registry name; the project-declared store is in the project tier
	// under its own name.
	require.Equal(t, []knowledgeSource{
		{Tier: "repo", Name: "testproj", Provider: "file", Location: projectLoc},
		{Tier: "project", Name: "team", Provider: "file", Location: teamLoc},
	}, result.Sources)
}

// Criterion 1 & 2: `knowledge list` enumerates entries across all stores,
// including a file nested in a subdirectory, in the {"entries":[...]} envelope.
// Each entry is tagged with the name of the store it came from.
func TestKnowledgeList_EnumeratesAllStoresIncludingNested(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "list")
	require.NoError(t, err)

	var result struct {
		Entries []knowledgeEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.ElementsMatch(t, []knowledgeEntry{
		{Tier: "repo", Name: "testproj", Path: "readme.md"},
		{Tier: "repo", Name: "testproj", Path: "architecture/initial-idea.md"},
		{Tier: "project", Name: "team", Path: "guidelines.md"},
	}, result.Entries)
}

// Criterion 1 & 2: `knowledge search` returns store-tagged, one-per-document
// hits carrying a locator, title, score, and excerpts in the {"hits":[...]}
// envelope. Each hit reports the tier and name of the store it came from.
// Neither fixture file has an ATX heading, so each title falls back to the
// path; "compass" occurs once per file, so both scores are 1, and the tie is
// broken by configured source order: the repo store before the project one.
func TestKnowledgeSearch_ReturnsStoreTaggedHits(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "search", "compass")
	require.NoError(t, err)

	var result struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, []knowledgeHit{
		{
			Tier:     "repo",
			Name:     "testproj",
			Path:     "readme.md",
			Title:    "readme.md",
			Excerpts: []string{"project readme: the compass points north"},
			Score:    1,
			Tags:     []string{},
		},
		{
			Tier:     "project",
			Name:     "team",
			Path:     "guidelines.md",
			Title:    "guidelines.md",
			Excerpts: []string{"team guidelines reference the compass too"},
			Score:    1,
			Tags:     []string{},
		},
	}, result.Hits)
}

// Phase 2.2 criterion 10 at the command surface: an entry's tags travel all the
// way out to the JSON an agent reads, and an entry with none emits "tags": []
// rather than omitting the key — so a consumer never has to tell "no tags" from
// "this build does not report tags".
//
// The absent-versus-empty distinction is the whole point of the second half, and
// a []string field cannot state it: encoding/json decodes both a missing key and
// a [] into the same empty slice. The raw view below decodes into *[]string
// instead, where a nil pointer means the key was absent and a pointer to an
// empty slice means it was emitted as [].
//
// Both entries mention "sextant" once in their bodies, so both score damp(1) = 1
// and neither can be cut by the relative floor; the tags are the only thing that
// differs between them.
func TestKnowledgeSearch_HitsCarryTagsAndEmitAnEmptyListWhenUntagged(t *testing.T) {
	_, projectLoc, _ := twoScopeProject(t)
	seedKnowledgeFile(t, projectLoc, "learnings/tagged.md",
		"---\n"+
			"tags: [networking, retries]\n"+
			"---\n"+
			"# Sextant Notes\n"+
			"\n"+
			"the sextant is calibrated here\n")
	seedKnowledgeFile(t, projectLoc, "learnings/plain.md",
		"the sextant is mentioned here too\n")

	stdout, _, err := runKnowledge(t, "search", "sextant")
	require.NoError(t, err)

	var typed struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &typed))
	require.Len(t, typed.Hits, 2)

	tags := map[string][]string{}
	for _, h := range typed.Hits {
		tags[h.Path] = h.Tags
	}
	require.Equal(t, []string{"networking", "retries"}, tags["learnings/tagged.md"],
		"a hit must report the tags its entry declares, in declared order")
	require.Empty(t, tags["learnings/plain.md"])

	// The same output decoded so an absent key is distinguishable from [].
	var raw struct {
		Hits []struct {
			Path string    `json:"path"`
			Tags *[]string `json:"tags"`
		} `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw))
	require.Len(t, raw.Hits, 2)
	for _, h := range raw.Hits {
		require.NotNil(t, h.Tags, "hit %q must carry a tags key even when the entry has none", h.Path)
	}
}

// Criterion 3: an empty query and a query matching no document both succeed
// at the command level with an empty (non-nil) hits array, not an error.
func TestKnowledgeSearch_EmptyAndNoMatchQueriesReturnEmptyHits(t *testing.T) {
	twoScopeProject(t)

	for name, query := range map[string]string{
		"empty query": "",
		"no match":    "zzz-no-match-zzz",
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := runKnowledge(t, "search", query)
			require.NoError(t, err)
			require.Empty(t, stderr)
			require.JSONEq(t, `{"error": false, "hits": []}`, stdout)
		})
	}
}

// Criterion 4: `knowledge read` returns the full entry content for an
// addressed store and locator in the {"tier","name","path","content"}
// envelope.
func TestKnowledgeRead_ReturnsFullEntry(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "read", "--data",
		`{"tier":"repo","name":"testproj","path":"architecture/initial-idea.md"}`)
	require.NoError(t, err)

	var result knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, knowledgeAddressResult{
		Tier:    "repo",
		Name:    "testproj",
		Path:    "architecture/initial-idea.md",
		Content: "an architecture note about widgets\n",
	}, result)
}

// Criterion 4: `knowledge write` persists an entry into the addressed store and
// echoes the {"tier","name","path"} envelope; the file lands under that
// store's configured location.
func TestKnowledgeWrite_PersistsEntry(t *testing.T) {
	_, _, teamLoc := twoScopeProject(t)

	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("freshly written knowledge\n"), 0o644))

	stdout, _, err := runKnowledge(t, "write",
		"--data", `{"tier":"project","name":"team","path":"learnings/new.md"}`,
		"--file", contentPath)
	require.NoError(t, err)

	var result knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, knowledgeAddressResult{Tier: "project", Name: "team", Path: "learnings/new.md"}, result)

	persisted := filepath.Join(teamLoc, "learnings", "new.md")
	require.FileExists(t, persisted)
	data, err := os.ReadFile(persisted)
	require.NoError(t, err)
	require.Equal(t, "freshly written knowledge\n", string(data))
}

// Criterion 1 & 2: `knowledge categories` projects the category registry to the
// {"categories":[...]} envelope — every category carries a purpose, boundary,
// tier, and entry shape, and the set of names is the full, ordered model.
func TestKnowledgeCategories_ListsEveryCategoryWithDefinition(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "categories")
	require.NoError(t, err)

	var result struct {
		Categories []knowledgeCategory `json:"categories"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Len(t, result.Categories, 6)

	var names []string
	for _, c := range result.Categories {
		require.NotEmpty(t, c.Purpose)
		require.NotEmpty(t, c.Boundary)
		require.NotEmpty(t, c.Tier)
		require.NotEmpty(t, c.EntryShape)
		names = append(names, c.Name)
	}
	require.ElementsMatch(t,
		[]string{"conventions", "glossary", "architecture", "gotchas", "learnings", "decisions"},
		names)
}

// Criterion 7: `knowledge categories` publishes exactly the fields it has
// always published — name, purpose, boundary, tier, entryShape — with the
// retrieval tier still spelled "always-applied" or "looked-up". The addressing
// tier introduced alongside it must not rename, reorder, or re-value anything
// here. Decoded as raw JSON objects so a renamed or dropped key fails.
func TestKnowledgeCategories_PublishesUnchangedFieldsAndTierValues(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "categories")
	require.NoError(t, err)

	var result struct {
		Categories []map[string]any `json:"categories"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.NotEmpty(t, result.Categories)

	for _, c := range result.Categories {
		keys := make([]string, 0, len(c))
		for k := range c {
			keys = append(keys, k)
		}
		require.ElementsMatch(t,
			[]string{"name", "purpose", "boundary", "tier", "entryShape"}, keys,
			"category %v must publish exactly the documented fields", c["name"])
		require.Contains(t, []any{"always-applied", "looked-up"}, c["tier"],
			"category %v must publish a retrieval tier, not an addressing tier", c["name"])
	}
}

// Criterion 2: `categories --schema` declares the output envelope — categories
// is an array property.
func TestKnowledgeCategories_SchemaDeclaresCategoriesArray(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "categories", "--schema")
	require.NoError(t, err)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Output)
	require.Contains(t, schema.Output.Properties, "categories")
	require.Equal(t, "array", schema.Output.Properties["categories"].Type)
}

// Criterion 2: the --schema persistent flag prints the documented input/output
// schema envelope for a subcommand instead of running it.
func TestKnowledgeRead_SchemaDocumentsInputAndOutput(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "read", "--schema")
	require.NoError(t, err)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Input)
	require.Equal(t, []string{"tier", "name", "path"}, schema.Input.Required)
	require.Equal(t, []string{"project", "repo"}, schema.Input.Properties["tier"].Enum)
	require.Equal(t, "string", schema.Input.Properties["name"].Type)
	require.Equal(t, "string", schema.Input.Properties["path"].Type)
	require.NotNil(t, schema.Output)
	for _, field := range []string{"tier", "name", "path", "content"} {
		require.Contains(t, schema.Output.Properties, field)
	}
}

// Criterion 2: `search --schema` declares the per-document hit shape that the
// command emits — hits is an array whose items carry the store address (tier
// and name), path, title, score, excerpts, and the entry's tags, with their
// documented types.
func TestKnowledgeSearch_SchemaDeclaresPerDocumentHitFields(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "search", "--schema", "ignored")
	require.NoError(t, err)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Output)
	require.Contains(t, schema.Output.Properties, "hits")

	hits := schema.Output.Properties["hits"]
	require.Equal(t, "array", hits.Type)
	require.NotNil(t, hits.Items)
	require.Equal(t, "object", hits.Items.Type)
	require.Equal(t, "string", hits.Items.Properties["tier"].Type)
	require.Equal(t, "string", hits.Items.Properties["name"].Type)
	require.NotContains(t, hits.Items.Properties, "scope",
		"a hit no longer carries a packed scope label")
	require.Equal(t, "string", hits.Items.Properties["path"].Type)
	require.Equal(t, "string", hits.Items.Properties["title"].Type)
	require.Equal(t, "number", hits.Items.Properties["score"].Type)
	require.Equal(t, "array", hits.Items.Properties["excerpts"].Type)
	require.NotNil(t, hits.Items.Properties["excerpts"].Items)
	require.Equal(t, "string", hits.Items.Properties["excerpts"].Items.Type)
	require.Equal(t, "string", hits.Items.Properties["category"].Type)
	require.Equal(t, "array", hits.Items.Properties["tags"].Type)
	require.NotNil(t, hits.Items.Properties["tags"].Items)
	require.Equal(t, "string", hits.Items.Properties["tags"].Items.Type)
}

// Criterion 2: a failing subcommand emits the standard ErrorResponse envelope
// on stdout (the same stream success uses) and the command reports failure.
func TestKnowledgeRead_MissingDataEmitsErrorEnvelope(t *testing.T) {
	twoScopeProject(t)

	stdout, stderr, err := runKnowledge(t, "read")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Contains(t, envelope.Message, "--data is required")
}

// Criterion 3: reading a store name that exists in no configured store
// surfaces through the same ErrorResponse envelope, and the refusal names the
// stores that tier does hold — the same names `knowledge sources` reports for
// it. The expected names are hand-written; the cross-check against `sources`
// is what ties the guidance to the configuration a caller can inspect.
func TestKnowledgeRead_UnknownStoreNameNamesTheTiersStores(t *testing.T) {
	twoScopeProject(t)

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)
	var listed struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	var repoNames, projectNames []string
	for _, src := range listed.Sources {
		switch src.Tier {
		case "repo":
			repoNames = append(repoNames, src.Name)
		case "project":
			projectNames = append(projectNames, src.Name)
		}
	}
	require.Equal(t, []string{"testproj"}, repoNames)
	require.Equal(t, []string{"team"}, projectNames)

	stdout, stderr, err := runKnowledge(t, "read", "--data",
		`{"tier":"repo","name":"missing","path":"readme.md"}`)
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "knowledge_store_unknown", envelope.Code)
	require.Contains(t, envelope.Message, "missing")
	require.Contains(t, envelope.NextAction, `stores available in the "repo" tier: testproj`)
	require.NotContains(t, envelope.NextAction, "team", "the project tier's names must not be offered for a repo-tier address")
}

// Criterion 1: a write that omits the tier, the store name, or both is refused
// before any store is touched, records nothing, and leaves the location empty —
// a later read at a fully-stated address finds nothing there.
func TestKnowledgeWrite_WithoutAFullAddressRecordsNothing(t *testing.T) {
	for name, tc := range map[string]struct{ data, code string }{
		"tier omitted": {`{"name":"team","path":"learnings/dropped.md"}`, "knowledge_tier_required"},
		"name omitted": {`{"tier":"project","path":"learnings/dropped.md"}`, "knowledge_name_required"},
		"both omitted": {`{"path":"learnings/dropped.md"}`, "knowledge_tier_required"},
	} {
		t.Run(name, func(t *testing.T) {
			_, projectLoc, teamLoc := twoScopeProject(t)

			contentPath := filepath.Join(t.TempDir(), "payload.md")
			require.NoError(t, os.WriteFile(contentPath, []byte("must not persist\n"), 0o644))

			stdout, _, err := runKnowledge(t, "write", "--data", tc.data, "--file", contentPath)
			require.Error(t, err)

			var envelope output.ErrorResponse
			require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
			require.Equal(t, tc.code, envelope.Code)
			require.NotEmpty(t, envelope.NextAction)

			require.NoFileExists(t, filepath.Join(projectLoc, "learnings", "dropped.md"))
			require.NoFileExists(t, filepath.Join(teamLoc, "learnings", "dropped.md"))

			for _, addr := range []string{
				`{"tier":"repo","name":"testproj","path":"learnings/dropped.md"}`,
				`{"tier":"project","name":"team","path":"learnings/dropped.md"}`,
			} {
				_, _, err := runKnowledge(t, "read", "--data", addr)
				require.Error(t, err, "the refused write must leave nothing to read back at %s", addr)
			}
		})
	}
}

// Criterion 6: "all" names both tiers at once, so it cannot address the single
// store a read or a write acts on, and a tier that is neither of the two
// addressable tiers is rejected outright. Both are refused before any store is
// touched.
func TestKnowledgeReadWrite_RefuseFanOutAndUnknownTiers(t *testing.T) {
	for name, tier := range map[string]string{"fan-out tier": "all", "unknown tier": "team"} {
		t.Run(name, func(t *testing.T) {
			_, projectLoc, _ := twoScopeProject(t)
			data := `{"tier":"` + tier + `","name":"testproj","path":"learnings/fanned.md"}`

			contentPath := filepath.Join(t.TempDir(), "payload.md")
			require.NoError(t, os.WriteFile(contentPath, []byte("no\n"), 0o644))

			for _, args := range [][]string{
				{"read", "--data", data},
				{"write", "--data", data, "--file", contentPath},
			} {
				stdout, _, err := runKnowledge(t, args...)
				require.Error(t, err)
				var envelope output.ErrorResponse
				require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
				require.Equal(t, "knowledge_tier_invalid", envelope.Code)
				require.NotEmpty(t, envelope.NextAction)
			}
			require.NoFileExists(t, filepath.Join(projectLoc, "learnings", "fanned.md"))
		})
	}
}

// alwaysAppliedProject lays out a temp project with a single file-backed
// knowledge store — the colocated repo's default source at
// .spektacular/knowledge, addressed in the repo tier under the repo's registry
// name "testproj" — seeded with one entry under conventions/ and one under
// glossary/, then chdirs into it. It returns the project root. This fixture is
// independent of twoScopeProject so the always-applied tests don't perturb the
// exact-match expectations of the other suites.
func alwaysAppliedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)

	dataDir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))

	loc := filepath.Join(dataDir, "knowledge")
	seed := func(name, content string) {
		full := filepath.Join(loc, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	seed("conventions/style.md", "always use tabs\n")
	seed("glossary/compass.md", "compass: a tool that points north\n")

	cfg := "name: testproj\nrepos:\n  - name: testproj\n    location: .\n"
	writeCurrentConfig(t, root, cfg)
	require.NoError(t, config.NewDefaultRepoConfig().ToYAMLFile(filepath.Join(dataDir, config.RepoConfigFileName)))

	return root
}

// Criterion 2: knowledge write, read, and search behave exactly as before the
// project/repo config split for a solo-repo project. A freshly initialised
// project — whose config.yaml lists no knowledge sources; its only store comes
// entirely from the init-written repo.yaml — round-trips an entry through
// write, read, and search end-to-end. That single store is addressed in the
// repo tier under the registry name init recorded in config.yaml.
func TestKnowledge_RoundTripAfterInit_SoloRepo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	// The colocated repo config exists and is the knowledge authority.
	require.FileExists(t, filepath.Join(dir, ".spektacular", "repo.yaml"))

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	addr := `{"tier":"repo","name":"` + cfg.Name + `","path":"learnings/zanzibar.md"}`

	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("the zanzibar protocol\n"), 0o644))

	// Write into the colocated repo's store.
	stdout, _, err := runKnowledge(t, "write", "--data", addr, "--file", contentPath)
	require.NoError(t, err)
	var wrote knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &wrote))
	require.Equal(t, knowledgeAddressResult{
		Tier: "repo",
		Name: cfg.Name,
		Path: "learnings/zanzibar.md",
	}, wrote)
	require.FileExists(t, filepath.Join(dir, ".spektacular", "knowledge", "learnings", "zanzibar.md"))

	// Read it back at the same address.
	stdout, _, err = runKnowledge(t, "read", "--data", addr)
	require.NoError(t, err)
	var read knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &read))
	require.Equal(t, knowledgeAddressResult{
		Tier:    "repo",
		Name:    cfg.Name,
		Path:    "learnings/zanzibar.md",
		Content: "the zanzibar protocol\n",
	}, read)

	// Search finds it. "zanzibar" appears in no init-seeded README, so the
	// written entry is the only hit.
	stdout, _, err = runKnowledge(t, "search", "zanzibar")
	require.NoError(t, err)
	var searched struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &searched))
	require.Len(t, searched.Hits, 1)
	require.Equal(t, "repo", searched.Hits[0].Tier)
	require.Equal(t, cfg.Name, searched.Hits[0].Name)
	require.Equal(t, "learnings/zanzibar.md", searched.Hits[0].Path)

	// Criterion 3 (Phase 2.4): a solo-repo project's single store is addressed
	// in the repo tier under the colocated repo's registry name.
	stdout, _, err = runKnowledge(t, "sources")
	require.NoError(t, err)
	var listed struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	require.Equal(t, []knowledgeSource{
		{Tier: "repo", Name: cfg.Name, Provider: "file", Location: filepath.Join(dir, ".spektacular", "knowledge")},
	}, listed.Sources)
}

// seedKnowledgeFile writes a knowledge entry at loc/name, creating parents.
func seedKnowledgeFile(t *testing.T, loc, name, content string) {
	t.Helper()
	full := filepath.Join(loc, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// memberRegistryProject lays out a temp project whose config registers the
// colocated repo (named "testproj", location "..") plus one member repo (named
// "member", at a sibling temp dir carrying a default repo.yaml), and declares
// a project-owned "team" source. The two repos' default knowledge stores are
// addressed in the repo tier under the names "testproj" and "member"; the
// project-declared source is addressed in the project tier as "team". Both
// repo stores exist but are empty; the caller seeds entries. It chdirs into
// the project root and returns the three roots.
func memberRegistryProject(t *testing.T) (root, member, teamLoc string) {
	t.Helper()
	root = t.TempDir()
	member = t.TempDir()
	teamLoc = filepath.Join(root, "team-kb")
	t.Chdir(root)

	// Both repos carry a default repo.yaml, as init and `repo add` would
	// have written (writeSpecCommandConfig supplies the colocated one).
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular", "knowledge"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(member, ".spektacular", "knowledge"), 0o755))
	require.NoError(t, os.MkdirAll(teamLoc, 0o755))
	require.NoError(t, config.NewDefaultRepoConfig().ToYAMLFile(
		filepath.Join(member, ".spektacular", config.RepoConfigFileName)))

	writeSpecCommandConfig(t, root,
		"repos:\n"+
			"  - name: testproj\n"+
			"    location: .\n"+
			"  - name: member\n"+
			"    location: "+filepath.Join(member, ".spektacular")+"\n"+
			"knowledge:\n"+
			"  sources:\n"+
			"    - name: team\n"+
			"      provider: file\n"+
			"      config:\n"+
			"        location: "+teamLoc+"\n")

	return root, member, teamLoc
}

// memberWithoutFootprintProject lays out a temp project registering the
// colocated repo plus a member repo that exists on disk but carries no
// .spektacular footprint at all, chdirs into the project root, and returns
// both roots.
func memberWithoutFootprintProject(t *testing.T) (root, member string) {
	t.Helper()
	root = t.TempDir()
	member = t.TempDir()
	t.Chdir(root)

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular", "knowledge"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(member, ".spektacular"), 0o755))
	writeSpecCommandConfig(t, root,
		"repos:\n"+
			"  - name: testproj\n"+
			"    location: .\n"+
			"  - name: member\n"+
			"    location: "+filepath.Join(member, ".spektacular")+"\n")

	return root, member
}

// Criterion 2: `knowledge search` results include entries from every
// registered repo's sources — colocated and member — and from project-owned
// Phase 1.3 criterion 2: `knowledge search` matching entries in more than one
// store returns hits from each — every registered repo's sources, colocated and
// member, and project-owned sources declared in the project config — and every
// hit reports the tier and store name it came from alongside the locator,
// title, score, and excerpts it already reported. Every entry contains
// "beacon" exactly once, so all scores tie and the order is the aggregated
// source order: colocated repo, member repo, then project-owned.
func TestKnowledgeSearch_AggregatesColocatedMemberAndProjectOwnedSources(t *testing.T) {
	root, member, teamLoc := memberRegistryProject(t)
	seedKnowledgeFile(t, filepath.Join(root, ".spektacular", "knowledge"),
		"learnings/colocated-note.md", "the beacon shines in the colocated repo\n")
	seedKnowledgeFile(t, filepath.Join(member, ".spektacular", "knowledge"),
		"learnings/member-note.md", "the beacon shines in the member repo\n")
	seedKnowledgeFile(t, teamLoc, "guidelines.md", "the beacon shines in the team source\n")

	stdout, _, err := runKnowledge(t, "search", "beacon")
	require.NoError(t, err)

	var result struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, []knowledgeHit{
		{
			Tier:     "repo",
			Name:     "testproj",
			Path:     "learnings/colocated-note.md",
			Title:    "learnings/colocated-note.md",
			Excerpts: []string{"the beacon shines in the colocated repo"},
			Score:    1,
			Tags:     []string{},
		},
		{
			Tier:     "repo",
			Name:     "member",
			Path:     "learnings/member-note.md",
			Title:    "learnings/member-note.md",
			Excerpts: []string{"the beacon shines in the member repo"},
			Score:    1,
			Tags:     []string{},
		},
		{
			Tier:     "project",
			Name:     "team",
			Path:     "guidelines.md",
			Title:    "guidelines.md",
			Excerpts: []string{"the beacon shines in the team source"},
			Score:    1,
			Tags:     []string{},
		},
	}, result.Hits)
}

// Criterion 3: `knowledge sources` addresses each store by tier and name — the
// two repo-declared stores sit in the repo tier under their repos' registry
// names, and the project-declared "team" store sits in the project tier under
// its own name. These are the names an address may use and the names a refusal
// offers.
func TestKnowledgeSources_AddressesEachStoreByTierAndName(t *testing.T) {
	root, member, teamLoc := memberRegistryProject(t)

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)

	var result struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, []knowledgeSource{
		{Tier: "repo", Name: "testproj", Provider: "file", Location: filepath.Join(root, ".spektacular", "knowledge")},
		{Tier: "repo", Name: "member", Provider: "file", Location: filepath.Join(member, ".spektacular", "knowledge")},
		{Tier: "project", Name: "team", Provider: "file", Location: teamLoc},
	}, result.Sources)
}

// Criterion 5: this test inverts the pre-addressing behaviour it used to
// assert. Before repo-scoped addressing, a request carrying nothing but a bare
// scope — `{"scope":"project","path":...}` — was silently resolved to the
// first repo in registry order: on this very fixture a read returned the
// colocated repo's copy and a write landed in the colocated repo's store,
// while the member repo's identically-located entry was unreachable and its
// store was never written to. That first-match guess is what addressing exists
// to remove.
//
// The same under-specified request is now refused outright: it names no tier
// and no store, so there is no store to resolve it to, and nothing is read or
// recorded anywhere. The two fully addressed forms of the same request each
// reach their own repo's store instead. The fixture is unchanged, so the
// assertion still stands at the exact point the defect lived.
func TestKnowledgeReadWrite_AmbiguousRequestIsRefusedNotResolvedToFirstRepo(t *testing.T) {
	root, member, _ := memberRegistryProject(t)
	colocatedStore := filepath.Join(root, ".spektacular", "knowledge")
	memberStore := filepath.Join(member, ".spektacular", "knowledge")
	seedKnowledgeFile(t, colocatedStore, "learnings/dup.md", "colocated version\n")
	seedKnowledgeFile(t, memberStore, "learnings/dup.md", "member version\n")

	// The read that used to resolve to the colocated repo's copy is refused,
	// and the refusal says which part of the address is missing and what the
	// caller may name in its place.
	stdout, stderr, err := runKnowledge(t, "read", "--data", `{"scope":"project","path":"learnings/dup.md"}`)
	require.Error(t, err, "an unaddressed read must not resolve to whichever store comes first")
	require.Empty(t, stderr)
	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "knowledge_tier_required", envelope.Code)
	require.Contains(t, envelope.NextAction, `reissue with "tier" set to "project" or "repo"`)
	require.Contains(t, envelope.NextAction, "stores available: testproj, member, team")

	// The write that used to land in the colocated repo's store is refused and
	// records nothing — in either repo, and in the project-owned store.
	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("fresh entry\n"), 0o644))
	stdout, _, err = runKnowledge(t, "write",
		"--data", `{"scope":"project","path":"learnings/fresh.md"}`,
		"--file", contentPath)
	require.Error(t, err, "an unaddressed write must not land in whichever store comes first")
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.Equal(t, "knowledge_tier_required", envelope.Code)
	require.NotEmpty(t, envelope.NextAction)
	require.NoFileExists(t, filepath.Join(colocatedStore, "learnings", "fresh.md"))
	require.NoFileExists(t, filepath.Join(memberStore, "learnings", "fresh.md"))

	// Stated in full, the same request reaches whichever repo it names — the
	// member's copy is reachable now, where first-match resolution could never
	// return it.
	for name, want := range map[string]string{
		"testproj": "colocated version\n",
		"member":   "member version\n",
	} {
		stdout, _, err := runKnowledge(t, "read", "--data",
			`{"tier":"repo","name":"`+name+`","path":"learnings/dup.md"}`)
		require.NoError(t, err)
		var read knowledgeAddressResult
		require.NoError(t, json.Unmarshal([]byte(stdout), &read))
		require.Equal(t, knowledgeAddressResult{
			Tier:    "repo",
			Name:    name,
			Path:    "learnings/dup.md",
			Content: want,
		}, read)
	}
}

// Criterion 5: two registered repos holding an entry at the same location stay
// distinct — each name returns its own repo's content and never the other's,
// and a write to one name lands only in that repo's store.
func TestKnowledgeReadWrite_SameLocationInTwoReposStaysDistinct(t *testing.T) {
	root, member, _ := memberRegistryProject(t)
	seedKnowledgeFile(t, filepath.Join(root, ".spektacular", "knowledge"),
		"learnings/dup.md", "colocated version\n")
	seedKnowledgeFile(t, filepath.Join(member, ".spektacular", "knowledge"),
		"learnings/dup.md", "member version\n")

	for name, want := range map[string]string{
		"testproj": "colocated version\n",
		"member":   "member version\n",
	} {
		stdout, _, err := runKnowledge(t, "read", "--data",
			`{"tier":"repo","name":"`+name+`","path":"learnings/dup.md"}`)
		require.NoError(t, err)
		var read knowledgeAddressResult
		require.NoError(t, json.Unmarshal([]byte(stdout), &read))
		require.Equal(t, knowledgeAddressResult{
			Tier:    "repo",
			Name:    name,
			Path:    "learnings/dup.md",
			Content: want,
		}, read)
	}

	// A write addressed to the member lands only in the member's store.
	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("fresh entry\n"), 0o644))
	_, _, err := runKnowledge(t, "write",
		"--data", `{"tier":"repo","name":"member","path":"learnings/fresh.md"}`,
		"--file", contentPath)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(member, ".spektacular", "knowledge", "learnings", "fresh.md"))
	require.NoFileExists(t, filepath.Join(root, ".spektacular", "knowledge", "learnings", "fresh.md"))
}

// A member repo missing its repo.yaml breaks knowledge aggregation with the
// structured "repo_footprint" error whose next action offers `repo add` as
// the repair.
func TestKnowledgeSources_MemberMissingFootprintErrorsWithRepairOffer(t *testing.T) {
	memberWithoutFootprintProject(t)

	_, stderr, err := runKnowledge(t, "sources")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope *output.ErrorResponse
	require.ErrorAs(t, err, &envelope)
	require.Equal(t, "repo_footprint", envelope.Code)
	require.Contains(t, envelope.Message, "member")
	require.Contains(t, envelope.NextAction, "repo add")
}

// A member repo whose repo.yaml exists but does not parse fails with the same
// "repo_footprint" error shape as a missing one.
func TestKnowledgeSources_MemberInvalidFootprintErrorsWithRepairOffer(t *testing.T) {
	_, member := memberWithoutFootprintProject(t)
	seedKnowledgeFile(t, member, ".spektacular/"+config.RepoConfigFileName, "{{{ not yaml\n")

	_, stderr, err := runKnowledge(t, "sources")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope *output.ErrorResponse
	require.ErrorAs(t, err, &envelope)
	require.Equal(t, "repo_footprint", envelope.Code)
	require.Contains(t, envelope.NextAction, "repo add")
}

// A registry entry whose location is not on disk is a misregistration:
// knowledge commands report it as repo_location_missing, naming the path the
// location resolved to, rather than silently aggregating without it. Git is
// never invoked.
func TestKnowledgeSources_AbsentRepoIsAnError(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	git := &stubGit{}
	swapRepoGit(t, git)

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular", "knowledge"), 0o755))
	writeSpecCommandConfig(t, root,
		"repos:\n"+
			"  - name: testproj\n"+
			"    location: .\n"+
			"  - name: ghost\n"+
			"    location: ../ghost\n")

	_, _, err := runKnowledge(t, "sources")
	require.Error(t, err)

	var envelope *output.ErrorResponse
	require.ErrorAs(t, err, &envelope)
	require.Equal(t, "repo_location_missing", envelope.Code)
	require.Contains(t, envelope.Message, `"ghost"`)
	require.Contains(t, envelope.Message, filepath.Join(root, "ghost"))
	require.Zero(t, git.calls, "knowledge commands must never invoke git")
}

// Phase 2.3: `knowledge always-applied` returns every always-applied entry —
// both conventions and glossary — across all scopes in the {"entries":[...]}
// envelope, each tagged with its scope, path, content, and the category it came
// from so a consumer can tell a convention from a glossary term.
func TestKnowledgeAlwaysApplied_ReturnsConventionsAndGlossaryTagged(t *testing.T) {
	alwaysAppliedProject(t)

	stdout, _, err := runKnowledge(t, "always-applied")
	require.NoError(t, err)

	var result struct {
		Entries []alwaysAppliedEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.ElementsMatch(t, []alwaysAppliedEntry{
		{
			Tier:     "repo",
			Name:     "testproj",
			Path:     "conventions/style.md",
			Content:  "always use tabs\n",
			Category: "conventions",
		},
		{
			Tier:     "repo",
			Name:     "testproj",
			Path:     "glossary/compass.md",
			Content:  "compass: a tool that points north\n",
			Category: "glossary",
		},
	}, result.Entries)
}

// Criterion 4: `knowledge always-applied --tier repo --filter <name>`
// (repeatable) narrows the load to the named store(s) in the repo tier and to
// nothing else — the other registered repo's entries are excluded, and so are
// the project-owned "team" store's, which used to load unconditionally.
// Omitting the options still loads every store, so the narrowed load is the
// smaller of the two.
func TestKnowledgeAlwaysApplied_FilterNarrowsToNamedStores(t *testing.T) {
	root, member, teamLoc := memberRegistryProject(t)
	seedKnowledgeFile(t, filepath.Join(root, ".spektacular", "knowledge"),
		"conventions/colocated.md", "colocated: use tabs\n")
	seedKnowledgeFile(t, filepath.Join(member, ".spektacular", "knowledge"),
		"conventions/member.md", "member: use spaces\n")
	seedKnowledgeFile(t, teamLoc, "conventions/team.md", "team: use semicolons\n")

	load := func(args ...string) []alwaysAppliedEntry {
		t.Helper()
		stdout, _, err := runKnowledge(t, append([]string{"always-applied"}, args...)...)
		require.NoError(t, err)
		var result struct {
			Entries []alwaysAppliedEntry `json:"entries"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &result))
		return result.Entries
	}

	require.ElementsMatch(t, []alwaysAppliedEntry{
		{Tier: "repo", Name: "member", Path: "conventions/member.md", Content: "member: use spaces\n", Category: "conventions"},
	}, load("--tier", "repo", "--filter", "member"),
		"naming a store must exclude every other store, the project-owned team source included")

	require.ElementsMatch(t, []alwaysAppliedEntry{
		{Tier: "repo", Name: "testproj", Path: "conventions/colocated.md", Content: "colocated: use tabs\n", Category: "conventions"},
		{Tier: "repo", Name: "member", Path: "conventions/member.md", Content: "member: use spaces\n", Category: "conventions"},
		{Tier: "project", Name: "team", Path: "conventions/team.md", Content: "team: use semicolons\n", Category: "conventions"},
	}, load(), "with no store named the load still covers every store in both tiers")
}

// Criterion 6: `--filter` naming something the repo tier does not hold is
// refused with the structured envelope naming the stores it does hold, rather
// than quietly loading nothing for the caller to misread as "this repo has no
// conventions".
func TestKnowledgeAlwaysApplied_UnknownFilterNamesTheRegisteredStores(t *testing.T) {
	memberRegistryProject(t)

	stdout, stderr, err := runKnowledge(t, "always-applied", "--tier", "repo", "--filter", "ghost")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "knowledge_store_unknown", envelope.Code)
	require.Contains(t, envelope.Message, "ghost")
	require.Contains(t, envelope.NextAction, `stores available in the "repo" tier: testproj, member`)
}

// Phase 2.3: `always-applied --schema` declares the output envelope — entries
// is an array property.
func TestKnowledgeAlwaysApplied_SchemaDeclaresEntriesArray(t *testing.T) {
	alwaysAppliedProject(t)

	stdout, _, err := runKnowledge(t, "always-applied", "--schema")
	require.NoError(t, err)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Output)
	require.Contains(t, schema.Output.Properties, "entries")
	require.Equal(t, "array", schema.Output.Properties["entries"].Type)
}

// sourcedMemberProject lays out a temp project (chdir'd into) registering
// the colocated repo "testproj" at ".." plus a member "api" at ../repos/api
// whose footprint — rendered via the production install path — declares
// `source: file://<code>`, where code is a separate temp dir holding a
// source file and its own (foreign) .spektacular/knowledge entry. It returns
// the project root, the member's location, and the code dir.
func sourcedMemberProject(t *testing.T) (root, location, code string) {
	t.Helper()
	root = t.TempDir()
	code = t.TempDir()
	t.Chdir(root)

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular", "knowledge"), 0o755))
	writeSpecCommandConfig(t, root,
		"repos:\n"+
			"  - name: testproj\n"+
			"    location: .\n"+
			"  - name: api\n"+
			"    location: ../repos/api/.spektacular\n")

	location = filepath.Join(root, "repos", "api")
	_, err := repo.EnsureFootprint(filepath.Join(location, ".spektacular"), config.NewDefaultRepoConfig())
	require.NoError(t, err)
	rcPath := filepath.Join(location, ".spektacular", config.RepoConfigFileName)
	rc, err := config.RepoConfigFromYAMLFile(rcPath)
	require.NoError(t, err)
	rc.Source = config.FileSource(code)
	require.NoError(t, rc.ToYAMLFile(rcPath))

	require.NoError(t, os.WriteFile(filepath.Join(code, "main.go"), []byte("package main\n"), 0o644))
	seedKnowledgeFile(t, filepath.Join(code, ".spektacular", "knowledge"),
		"learnings/foreign.md", "the lighthouse note lives in the code dir\n")

	return root, location, code
}

// Phase 1.4 criterion 4: knowledge aggregation for a member whose repo.yaml
// declares a file source reads the member's knowledge from its LOCATION —
// <root>/repos/api/.spektacular/knowledge — never from anything under the
// code directory the source points at. The code dir carries a decoy entry
// that must not surface, and no git runs.
func TestKnowledge_MemberWithFileSourceAggregatesFromLocationNotSource(t *testing.T) {
	root, location, code := sourcedMemberProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)
	seedKnowledgeFile(t, filepath.Join(location, ".spektacular", "knowledge"),
		"learnings/api-note.md", "the lighthouse note lives at the location\n")
	codeBefore := snapshotDir(t, code)

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)
	var sources struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &sources))
	require.Equal(t, []knowledgeSource{
		{Tier: "repo", Name: "testproj", Provider: "file", Location: filepath.Join(root, ".spektacular", "knowledge")},
		{Tier: "repo", Name: "api", Provider: "file", Location: filepath.Join(root, "repos", "api", ".spektacular", "knowledge")},
	}, sources.Sources, "the member's source is its location's knowledge store, not the code dir's")

	stdout, _, err = runKnowledge(t, "search", "lighthouse")
	require.NoError(t, err)
	var search struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &search))
	require.Equal(t, []knowledgeHit{
		{
			Tier:     "repo",
			Name:     "api",
			Path:     "learnings/api-note.md",
			Title:    "learnings/api-note.md",
			Excerpts: []string{"the lighthouse note lives at the location"},
			Score:    1,
			Tags:     []string{},
		},
	}, search.Hits, "the decoy entry under the code dir must never surface")

	require.Zero(t, git.calls, "knowledge aggregation must never invoke git")
	require.Equal(t, codeBefore, snapshotDir(t, code), "the code dir must be untouched")
}

// Phase 1.4 criterion 5: the colocated project repo (location "..") whose own
// repo.yaml declares `source: file://<elsewhere>` still has its knowledge
// keyed on the project root: aggregation reads the footprint at the location
// (the project root itself), so the source directory — which has no
// .spektacular at all — never triggers the member repo_footprint error, and
// the project's knowledge store is reported as the repo's source.
func TestKnowledgeSources_ColocatedRepoWithFileSourceKeyedOnProjectRoot(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	t.Chdir(root)
	git := &stubGit{}
	swapRepoGit(t, git)

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular", "knowledge"), 0o755))
	writeSpecCommandConfig(t, root,
		"repos:\n"+
			"  - name: testproj\n"+
			"    location: .\n")
	rc := config.NewDefaultRepoConfig()
	rc.Source = config.FileSource(elsewhere)
	require.NoError(t, rc.ToYAMLFile(filepath.Join(root, ".spektacular", config.RepoConfigFileName)))
	elsewhereBefore := snapshotDir(t, elsewhere)

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err, "a source dir without a footprint must not be treated as a broken member")

	var result struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, []knowledgeSource{
		{Tier: "repo", Name: "testproj", Provider: "file", Location: filepath.Join(root, ".spektacular", "knowledge")},
	}, result.Sources)

	require.Zero(t, git.calls)
	require.Equal(t, elsewhereBefore, snapshotDir(t, elsewhere))
	require.NoDirExists(t, filepath.Join(elsewhere, ".spektacular"))
}

// Phase 1.3 criterion 1: `knowledge list` in a project with two registered
// repositories and one project-owned shared store enumerates entries from all
// three, and every entry reports the tier and the store name it came from. The
// expected set is hand-written and exact, so an entry arriving with a blank or
// wrong address fails.
func TestKnowledgeList_ReportsTierAndNameAcrossBothReposAndTheSharedStore(t *testing.T) {
	root, member, teamLoc := memberRegistryProject(t)
	seedKnowledgeFile(t, filepath.Join(root, ".spektacular", "knowledge"),
		"learnings/colocated-note.md", "a note from the colocated repo\n")
	seedKnowledgeFile(t, filepath.Join(member, ".spektacular", "knowledge"),
		"learnings/member-note.md", "a note from the member repo\n")
	seedKnowledgeFile(t, teamLoc, "guidelines.md", "a note from the team store\n")

	stdout, _, err := runKnowledge(t, "list")
	require.NoError(t, err)

	var result struct {
		Entries []knowledgeEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.ElementsMatch(t, []knowledgeEntry{
		{Tier: "repo", Name: "testproj", Path: "learnings/colocated-note.md"},
		{Tier: "repo", Name: "member", Path: "learnings/member-note.md"},
		{Tier: "project", Name: "team", Path: "guidelines.md"},
	}, result.Entries)
}

// Phase 1.3 criterion 3: taking any single search hit and issuing a read using
// only the identifying information that hit carries — its tier, its store name,
// and its path — returns the entry that hit excerpted, including when an entry
// of the same name lives in every other store. All three stores hold
// learnings/dup.md with different bodies and every copy matches the query, so
// the path alone cannot pick one out; only the address the hit reports can. The
// excerpt and content expected of each store are hand-written from the
// fixtures, so a hit stamped with another store's address is caught by the
// excerpt before the read is even issued.
func TestKnowledgeSearch_HitAddressAloneReadsBackThatHitsEntry(t *testing.T) {
	root, member, teamLoc := memberRegistryProject(t)
	seedKnowledgeFile(t, filepath.Join(root, ".spektacular", "knowledge"),
		"learnings/dup.md", "the harbour note as the colocated repo tells it\n")
	seedKnowledgeFile(t, filepath.Join(member, ".spektacular", "knowledge"),
		"learnings/dup.md", "the harbour note as the member repo tells it\n")
	seedKnowledgeFile(t, teamLoc,
		"learnings/dup.md", "the harbour note as the team store tells it\n")

	want := map[[2]string]struct{ excerpt, content string }{
		{"repo", "testproj"}: {
			excerpt: "the harbour note as the colocated repo tells it",
			content: "the harbour note as the colocated repo tells it\n",
		},
		{"repo", "member"}: {
			excerpt: "the harbour note as the member repo tells it",
			content: "the harbour note as the member repo tells it\n",
		},
		{"project", "team"}: {
			excerpt: "the harbour note as the team store tells it",
			content: "the harbour note as the team store tells it\n",
		},
	}

	stdout, _, err := runKnowledge(t, "search", "harbour")
	require.NoError(t, err)

	var searched struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &searched))
	require.Len(t, searched.Hits, 3, "every store holding the entry must produce its own hit")

	seen := map[[2]string]bool{}
	for _, hit := range searched.Hits {
		address := [2]string{hit.Tier, hit.Name}
		expected, configured := want[address]
		require.True(t, configured, "a hit reported the address %v, which no configured store answers to", address)
		require.False(t, seen[address], "two hits reported the same address %v", address)
		seen[address] = true
		require.Equal(t, "learnings/dup.md", hit.Path)
		require.Equal(t, []string{expected.excerpt}, hit.Excerpts)

		// The read is issued from the hit alone: nothing but its tier, name and
		// path is carried across.
		data, err := json.Marshal(map[string]string{"tier": hit.Tier, "name": hit.Name, "path": hit.Path})
		require.NoError(t, err)

		stdout, _, err := runKnowledge(t, "read", "--data", string(data))
		require.NoError(t, err)
		var read knowledgeAddressResult
		require.NoError(t, json.Unmarshal([]byte(stdout), &read))
		require.Equal(t, knowledgeAddressResult{
			Tier:    hit.Tier,
			Name:    hit.Name,
			Path:    hit.Path,
			Content: expected.content,
		}, read)
	}
	require.Len(t, seen, len(want), "each configured store must be reachable from its own hit")
}

// Phase 1.3 criterion 4: `knowledge sources` reports a tier and a name for
// every configured store, and nothing else beyond the provider and location it
// has always reported — in particular no separate repository attribution, since
// a repo-tier store's name *is* its repo. Decoded as raw JSON objects so a
// lingering "repo" key, or any other stray field, fails.
func TestKnowledgeSources_PublishExactlyTheAddressProviderAndLocation(t *testing.T) {
	memberRegistryProject(t)

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)

	var result struct {
		Sources []map[string]any `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Len(t, result.Sources, 3)

	for _, src := range result.Sources {
		keys := make([]string, 0, len(src))
		for k := range src {
			keys = append(keys, k)
		}
		require.ElementsMatch(t,
			[]string{"tier", "name", "provider", "location"}, keys,
			"store %v must publish exactly its address, provider and location", src["name"])
	}
}

// Phase 1.3 criterion 5: two stores declared under the same name within one
// tier are refused when the store list is built, while the same name used once
// in each tier is accepted — a tier plus a name is the identity, not the name
// alone.
func TestRequireUniqueStoreNames_RefusesDuplicatesWithinATierOnly(t *testing.T) {
	source := func(tier, name string) config.SourceConfig {
		return config.SourceConfig{Provider: config.ProviderFile, Tier: tier, Name: name}
	}

	for name, tc := range map[string]struct {
		sources  []config.SourceConfig
		accepted bool
	}{
		"two shared stores under one name": {
			sources: []config.SourceConfig{source("project", "team"), source("project", "team")},
		},
		"two repo stores under one name": {
			sources: []config.SourceConfig{source("repo", "docs"), source("repo", "docs")},
		},
		"the same name once in each tier": {
			sources:  []config.SourceConfig{source("repo", "team"), source("project", "team")},
			accepted: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := requireUniqueStoreNames(tc.sources)
			if tc.accepted {
				require.NoError(t, err)
				return
			}
			var envelope *output.ErrorResponse
			require.ErrorAs(t, err, &envelope)
			require.Equal(t, "knowledge_store_name_duplicate", envelope.Code)
			require.NotEmpty(t, envelope.NextAction)
		})
	}
}

// Phase 1.3 criterion 5, through the command: a project declaring a shared
// store named after a registered repo is accepted, because the two sit in
// different tiers and are therefore two distinct addresses.
//
// Phase 2.1 removed the companion case that used to live here — a repo.yaml
// declaring two knowledge sources under one registry name — because a repo now
// declares exactly one store and can no longer produce that collision. The
// refusal itself is still covered directly against requireUniqueStoreNames.
func TestKnowledgeSources_DuplicateNameWithinATierIsRefusedAcrossTiersIsAccepted(t *testing.T) {
	t.Run("a shared store named after a registered repo", func(t *testing.T) {
		root := t.TempDir()
		teamLoc := filepath.Join(root, "team-kb")
		t.Chdir(root)

		require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular", "knowledge"), 0o755))
		require.NoError(t, os.MkdirAll(teamLoc, 0o755))
		writeSpecCommandConfig(t, root,
			"repos:\n"+
				"  - name: team\n"+
				"    location: .\n"+
				"knowledge:\n"+
				"  sources:\n"+
				"    - name: team\n"+
				"      provider: file\n"+
				"      config:\n"+
				"        location: "+teamLoc+"\n")

		stdout, _, err := runKnowledge(t, "sources")
		require.NoError(t, err, "the same name in each tier is two addresses, not a duplicate")

		var result struct {
			Sources []knowledgeSource `json:"sources"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &result))
		require.Equal(t, []knowledgeSource{
			{Tier: "repo", Name: "team", Provider: "file", Location: filepath.Join(root, ".spektacular", "knowledge")},
			{Tier: "project", Name: "team", Provider: "file", Location: teamLoc},
		}, result.Sources)
	})
}

// severalSharedStoresProject lays out a temp project declaring three shared
// knowledge stores under the distinct names "team", "global" and "legal",
// alongside the colocated repo's own store. It chdirs into the project root and
// returns the root plus the three shared stores' locations in declared order.
func severalSharedStoresProject(t *testing.T) (root string, teamLoc, globalLoc, legalLoc string) {
	t.Helper()
	root = t.TempDir()
	t.Chdir(root)

	teamLoc = filepath.Join(root, "team-kb")
	globalLoc = filepath.Join(root, "global-kb")
	legalLoc = filepath.Join(root, "legal-kb")

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular", "knowledge"), 0o755))
	for _, loc := range []string{teamLoc, globalLoc, legalLoc} {
		require.NoError(t, os.MkdirAll(loc, 0o755))
	}

	writeSpecCommandConfig(t, root,
		"repos:\n"+
			"  - name: testproj\n"+
			"    location: .\n"+
			"knowledge:\n"+
			"  sources:\n"+
			"    - name: team\n"+
			"      provider: file\n"+
			"      config:\n"+
			"        location: "+teamLoc+"\n"+
			"    - name: global\n"+
			"      provider: file\n"+
			"      config:\n"+
			"        location: "+globalLoc+"\n"+
			"    - name: legal\n"+
			"      provider: file\n"+
			"      config:\n"+
			"        location: "+legalLoc+"\n")

	return root, teamLoc, globalLoc, legalLoc
}

// Phase 2.2 criterion 1: a project may declare any number of shared knowledge
// stores, each identified by its own name. A config declaring three loads and
// validates, and all three are addressed in the project tier under exactly the
// names the file wrote, in declared order.
func TestKnowledgeSources_ListsEverySharedStoreUnderItsDeclaredName(t *testing.T) {
	root, teamLoc, globalLoc, legalLoc := severalSharedStoresProject(t)

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)

	var result struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, []knowledgeSource{
		{Tier: "repo", Name: "testproj", Provider: "file", Location: filepath.Join(root, ".spektacular", "knowledge")},
		{Tier: "project", Name: "team", Provider: "file", Location: teamLoc},
		{Tier: "project", Name: "global", Provider: "file", Location: globalLoc},
		{Tier: "project", Name: "legal", Provider: "file", Location: legalLoc},
	}, result.Sources)
}

// Phase 2.2 criterion 2: a write naming the project tier and a declared shared
// store's name lands in that store, and a read at the same tier and name
// returns the content supplied. "global" is the second of the three declared
// stores, so a write resolved by declaration order rather than by name would
// land in the wrong directory.
func TestKnowledge_WriteAndReadRoundTripThroughADeclaredSharedStore(t *testing.T) {
	_, teamLoc, globalLoc, legalLoc := severalSharedStoresProject(t)

	addr := `{"tier":"project","name":"global","path":"learnings/astrolabe.md"}`
	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("the astrolabe rule\n"), 0o644))

	stdout, _, err := runKnowledge(t, "write", "--data", addr, "--file", contentPath)
	require.NoError(t, err)
	var wrote knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &wrote))
	require.Equal(t, knowledgeAddressResult{
		Tier: "project",
		Name: "global",
		Path: "learnings/astrolabe.md",
	}, wrote)

	require.FileExists(t, filepath.Join(globalLoc, "learnings", "astrolabe.md"))
	require.NoFileExists(t, filepath.Join(teamLoc, "learnings", "astrolabe.md"))
	require.NoFileExists(t, filepath.Join(legalLoc, "learnings", "astrolabe.md"))

	stdout, _, err = runKnowledge(t, "read", "--data", addr)
	require.NoError(t, err)
	var read knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &read))
	require.Equal(t, knowledgeAddressResult{
		Tier:    "project",
		Name:    "global",
		Path:    "learnings/astrolabe.md",
		Content: "the astrolabe rule\n",
	}, read)
}

// Phase 2.2 criterion 4: a project declaring no shared stores is valid, and a
// request restricted to the project tier returns nothing rather than failing.
// alwaysAppliedProject's config.yaml carries no knowledge section at all, so
// its only store is the colocated repo's, in the repo tier.
func TestKnowledgeList_ProjectTierWithNoSharedStoresReturnsNoEntries(t *testing.T) {
	alwaysAppliedProject(t)

	stdout, _, err := runKnowledge(t, "list", "--tier", "project")
	require.NoError(t, err)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.JSONEq(t, "false", string(envelope["error"]))
	require.JSONEq(t, "[]", string(envelope["entries"]))
}

// knowledgeSchema runs `knowledge <sub> --schema` and returns the decoded
// commandSchema alongside the raw top-level JSON object, so a test can assert
// both what the schema declares and which keys it publishes at all.
func knowledgeSchema(t *testing.T, sub string) (commandSchema, map[string]json.RawMessage) {
	t.Helper()
	stdout, _, err := runKnowledge(t, sub, "--schema")
	require.NoError(t, err)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw))
	return schema, raw
}

// Criterion 1: the fan-out commands publish the narrowing options they
// accept on the command line, so a caller reading only the published interface
// learns that it may name a tier — and exactly which three values that tier
// takes — and may repeat a store name to narrow within it. The expected enum
// is hand-written and exact: a fourth value appearing, or "all" going missing,
// fails here.
func TestKnowledgeSchemas_FanOutCommandsPublishNarrowingFlags(t *testing.T) {
	twoScopeProject(t)

	for _, sub := range []string{"search", "list", "conventions", "always-applied", "tags"} {
		t.Run(sub, func(t *testing.T) {
			schema, _ := knowledgeSchema(t, sub)
			require.NotNil(t, schema.Flags, "%s must publish the options it narrows on", sub)

			tier := schema.Flags["tier"]
			require.NotNil(t, tier, "%s must publish its --tier option", sub)
			require.Equal(t, "string", tier.Type)
			require.Equal(t, []string{"project", "repo", "all"}, tier.Enum)

			filter := schema.Flags["filter"]
			require.NotNil(t, filter, "%s must publish its --filter option", sub)
			require.Equal(t, "array", filter.Type)
			require.NotNil(t, filter.Items)
			require.Equal(t, "string", filter.Items.Type)
		})
	}
}

// Criterion 1: the commands that take no command-line narrowing publish no
// flags block at all, so a caller cannot mistake `knowledge read` or
// `knowledge sources` for something it can narrow. Asserted on the raw JSON so
// an empty-but-present "flags" key fails.
func TestKnowledgeSchemas_AddressedAndUnnarrowedCommandsPublishNoFlags(t *testing.T) {
	twoScopeProject(t)

	for _, sub := range []string{"read", "write", "sources", "categories"} {
		t.Run(sub, func(t *testing.T) {
			_, raw := knowledgeSchema(t, sub)
			require.NotContains(t, raw, "flags",
				"%s takes no command-line narrowing and must not advertise any", sub)
		})
	}
}

// Criterion 1: `write --schema` declares the address a write travels on — the
// tier, the store name, and the entry path, all three required — with the tier
// restricted to the two addressable tiers. "all" names both tiers at once and
// so is not offered here, unlike on the fan-out commands.
func TestKnowledgeWrite_SchemaDocumentsTheAddressItRequires(t *testing.T) {
	twoScopeProject(t)

	schema, _ := knowledgeSchema(t, "write")
	require.NotNil(t, schema.Input)
	require.Equal(t, []string{"tier", "name", "path"}, schema.Input.Required)
	require.Equal(t, []string{"project", "repo"}, schema.Input.Properties["tier"].Enum)
	require.Equal(t, "string", schema.Input.Properties["name"].Type)
	require.Equal(t, "string", schema.Input.Properties["path"].Type)
	require.NotNil(t, schema.Output)
	require.ElementsMatch(t, []string{"tier", "name", "path"}, mapKeys(schema.Output.Properties))
}

// Criterion 1: every output envelope that returns a list of knowledge items
// declares the address each item carries — its tier and its store name — plus
// that command's own per-item fields, and never the packed "scope" label the
// address replaced. These four schemas were previously declared as bare
// arrays with no item shape at all, so a caller could not tell from the
// published interface that a listed entry says which store it came from.
func TestKnowledgeSchemas_ItemOutputsDeclareTheAddressAndNoScope(t *testing.T) {
	twoScopeProject(t)

	for _, tc := range []struct {
		sub, property string
		extra         map[string]string // extra item field -> declared type
	}{
		{"list", "entries", map[string]string{"path": "string"}},
		{"sources", "sources", map[string]string{"provider": "string", "location": "string"}},
		{"conventions", "conventions", map[string]string{"path": "string", "content": "string"}},
		{"always-applied", "entries", map[string]string{"path": "string", "content": "string", "category": "string"}},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			schema, _ := knowledgeSchema(t, tc.sub)
			require.NotNil(t, schema.Output)
			require.Contains(t, schema.Output.Properties, tc.property)

			items := schema.Output.Properties[tc.property]
			require.Equal(t, "array", items.Type)
			require.NotNil(t, items.Items)
			require.Equal(t, "object", items.Items.Type)

			require.Equal(t, "string", items.Items.Properties["tier"].Type)
			require.Equal(t, []string{"project", "repo"}, items.Items.Properties["tier"].Enum)
			require.Equal(t, "string", items.Items.Properties["name"].Type)
			require.NotContains(t, items.Items.Properties, "scope",
				"an item carries an address, not a packed scope label")

			want := []string{"tier", "name"}
			for field, declared := range tc.extra {
				require.Equal(t, declared, items.Items.Properties[field].Type)
				want = append(want, field)
			}
			require.ElementsMatch(t, want, mapKeys(items.Items.Properties),
				"%s items must declare exactly the fields they carry", tc.sub)
		})
	}
}

// mapKeys returns the keys of a schema property map, for comparison against a
// hand-written expected set.
func mapKeys(props map[string]*schemaProp) []string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	return keys
}

// Criterion 2: a caller holding nothing but the published interface can issue
// a fully addressed write that succeeds. The --data payload is built from the
// field names `write --schema` reports as required and the tier values it
// offers in their enum — nothing here is copied from the production input
// struct — and the store name is taken from `knowledge sources`, the listing
// the interface points a caller at. The entry is then read back at that same
// address to prove the write landed where it said it did.
func TestKnowledgeWrite_SchemaAdvertisedFieldsAloneIssueASuccessfulWrite(t *testing.T) {
	twoScopeProject(t)

	schema, _ := knowledgeSchema(t, "write")
	require.NotNil(t, schema.Input)
	require.Len(t, schema.Input.Required, 3, "the interface must state the whole address")

	// Discover the configured stores the way a caller would.
	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)
	var listed struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))

	// Pick a tier the schema offers that some configured store answers to, and
	// a store name within it.
	var tier, name string
	for _, offered := range schema.Input.Properties["tier"].Enum {
		for _, src := range listed.Sources {
			if src.Tier == offered {
				tier, name = offered, src.Name
				break
			}
		}
		if tier != "" {
			break
		}
	}
	require.NotEmpty(t, tier, "no configured store sits in any tier the interface offers")

	const entryPath = "learnings/from-the-schema.md"
	data := map[string]string{}
	for _, field := range schema.Input.Required {
		switch field {
		case "tier":
			data[field] = tier
		case "name":
			data[field] = name
		case "path":
			data[field] = entryPath
		default:
			t.Fatalf("the interface requires %q, which a caller cannot supply from what it publishes", field)
		}
	}
	payload, err := json.Marshal(data)
	require.NoError(t, err)

	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("written from the published interface\n"), 0o644))

	stdout, _, err = runKnowledge(t, "write", "--data", string(payload), "--file", contentPath)
	require.NoError(t, err, "a write built only from the published interface must succeed")
	var wrote knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &wrote))
	require.Equal(t, knowledgeAddressResult{Tier: tier, Name: name, Path: entryPath}, wrote)

	stdout, _, err = runKnowledge(t, "read", "--data", string(payload))
	require.NoError(t, err)
	var read knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &read))
	require.Equal(t, knowledgeAddressResult{
		Tier:    tier,
		Name:    name,
		Path:    entryPath,
		Content: "written from the published interface\n",
	}, read)
}

// Criterion 4: a correctly addressed read of an entry that does not exist
// reaches the caller as a structured refusal naming the missing entry and its
// store, carrying the path as the resource, and offering the exact listing
// command that would show what the store does hold. Before this it surfaced as
// a bare internal_error with an empty next action.
func TestKnowledgeRead_MissingEntryOffersTheListingForThatStore(t *testing.T) {
	twoScopeProject(t)

	stdout, stderr, err := runKnowledge(t, "read", "--data",
		`{"tier":"project","name":"team","path":"learnings/absent.md"}`)
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "knowledge_entry_not_found", envelope.Code)
	require.Contains(t, envelope.Message, "learnings/absent.md")
	require.Contains(t, envelope.Message, "team")
	require.Equal(t, "learnings/absent.md", envelope.Resource)
	require.Contains(t, envelope.NextAction, "knowledge list --tier project --filter team",
		"the correction must be the runnable listing for the store that was addressed")
}

// Criterion 4: `knowledge search` with no query is refused in the standard
// envelope, and the correction is a runnable search — the query in its
// positional slot — rather than a restatement of the problem.
func TestKnowledgeSearch_MissingQueryOffersARunnableSearch(t *testing.T) {
	twoScopeProject(t)

	stdout, stderr, err := runKnowledge(t, "search")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "knowledge_query_required", envelope.Code)
	require.Contains(t, envelope.Message, "query")
	require.Contains(t, envelope.NextAction, `knowledge search "database timeout"`,
		"the correction must be a search a caller can run as printed")
}

// narrowingProject seeds every store in memberRegistryProject — the two repos'
// stores and the project-owned "team" store — with one always-applied
// convention and one looked-up learning carrying the shared keyword "sextant",
// so a single query reaches every store and each retrieval path has something
// to return from all three. It returns the store names in configured order.
func narrowingProject(t *testing.T) []string {
	t.Helper()
	root, member, teamLoc := memberRegistryProject(t)

	for name, loc := range map[string]string{
		"testproj": filepath.Join(root, ".spektacular", "knowledge"),
		"member":   filepath.Join(member, ".spektacular", "knowledge"),
		"team":     teamLoc,
	} {
		seedKnowledgeFile(t, loc, "conventions/"+name+".md", name+" convention\n")
		seedKnowledgeFile(t, loc, "learnings/"+name+".md", name+" sextant learning\n")
	}
	return []string{"testproj", "member", "team"}
}

// knowledgeStoresReached runs one fan-out subcommand with the given narrowing
// options and returns the distinct store names its results came from, which is
// what narrowing decides. Search is given the keyword every store carries.
func knowledgeStoresReached(t *testing.T, sub string, args ...string) []string {
	t.Helper()
	invocation := []string{sub}
	if sub == "search" {
		invocation = append(invocation, "sextant")
	}
	stdout, _, err := runKnowledge(t, append(invocation, args...)...)
	require.NoError(t, err)

	var result struct {
		Hits        []knowledgeHit       `json:"hits"`
		Entries     []alwaysAppliedEntry `json:"entries"`
		Conventions []alwaysAppliedEntry `json:"conventions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))

	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, h := range result.Hits {
		add(h.Name)
	}
	for _, e := range result.Entries {
		add(e.Name)
	}
	for _, c := range result.Conventions {
		add(c.Name)
	}
	return names
}

// Criterion 1 and the narrowing behaviour behind it: --tier and --filter reach
// the same set of stores on all four fan-out commands. Each expected set is
// hand-written: the two repo-tier stores are "testproj" and "member", the one
// project-tier store is "team", and "all" is the union.
func TestKnowledgeNarrowing_TierAndFilterSelectTheSameStoresOnEveryFanOutCommand(t *testing.T) {
	for _, sub := range []string{"search", "list", "conventions", "always-applied"} {
		t.Run(sub, func(t *testing.T) {
			for name, tc := range map[string]struct {
				args []string
				want []string
			}{
				"no narrowing":             {nil, []string{"testproj", "member", "team"}},
				"tier all":                 {[]string{"--tier", "all"}, []string{"testproj", "member", "team"}},
				"tier repo":                {[]string{"--tier", "repo"}, []string{"testproj", "member"}},
				"tier project":             {[]string{"--tier", "project"}, []string{"team"}},
				"one filter":               {[]string{"--filter", "member"}, []string{"member"}},
				"repeated filter":          {[]string{"--filter", "member", "--filter", "team"}, []string{"member", "team"}},
				"tier and filter together": {[]string{"--tier", "repo", "--filter", "testproj"}, []string{"testproj"}},
			} {
				t.Run(name, func(t *testing.T) {
					narrowingProject(t)
					require.ElementsMatch(t, tc.want, knowledgeStoresReached(t, sub, tc.args...))
				})
			}
		})
	}
}

// Criterion 4: a --filter naming a store the chosen tier does not reach is
// refused on every fan-out command, in the standard envelope, and the
// correction names the stores that tier does hold rather than leaving the
// caller to read an empty result as "there is no such knowledge".
func TestKnowledgeNarrowing_UnknownFilterIsRefusedNamingTheStoresTheTierHolds(t *testing.T) {
	// always-applied is covered on its own fixture by
	// TestKnowledgeAlwaysApplied_UnknownFilterNamesTheRegisteredStores.
	for _, sub := range []string{"search", "list", "conventions"} {
		t.Run(sub, func(t *testing.T) {
			narrowingProject(t)

			args := []string{sub}
			if sub == "search" {
				args = append(args, "sextant")
			}
			stdout, stderr, err := runKnowledge(t, append(args, "--tier", "repo", "--filter", "ghost")...)
			require.Error(t, err)
			require.Empty(t, stderr)

			var envelope output.ErrorResponse
			require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
			require.True(t, envelope.IsError)
			require.Equal(t, "knowledge_store_unknown", envelope.Code)
			require.Contains(t, envelope.Message, "ghost")
			require.Contains(t, envelope.NextAction, `stores available in the "repo" tier: testproj, member`)
		})
	}
}

// The --tier and --filter options are registered on four commands but backed
// by two shared package-level variables, so narrowing set by one invocation
// would carry into the next unless it is cleared between runs. A narrowed call
// followed by an unnarrowed one on a different command must leave the second
// call covering every store: if resetting misses a command, this fails.
func TestKnowledgeNarrowing_DoesNotLeakBetweenInvocations(t *testing.T) {
	all := narrowingProject(t)

	require.ElementsMatch(t, []string{"member"},
		knowledgeStoresReached(t, "always-applied", "--tier", "repo", "--filter", "member"))
	require.ElementsMatch(t, all, knowledgeStoresReached(t, "list"),
		"an unnarrowed list must cover every store after a narrowed always-applied")

	require.ElementsMatch(t, []string{"team"},
		knowledgeStoresReached(t, "list", "--tier", "project"))
	require.ElementsMatch(t, all, knowledgeStoresReached(t, "search"),
		"an unnarrowed search must cover every store after a narrowed list")

	require.ElementsMatch(t, []string{"testproj"},
		knowledgeStoresReached(t, "search", "--filter", "testproj"))
	require.ElementsMatch(t, all, knowledgeStoresReached(t, "conventions"),
		"unnarrowed conventions must cover every store after a narrowed search")

	require.ElementsMatch(t, []string{"testproj", "member"},
		knowledgeStoresReached(t, "conventions", "--tier", "repo"))
	require.ElementsMatch(t, all, knowledgeStoresReached(t, "always-applied"),
		"an unnarrowed always-applied must cover every store after narrowed conventions")
}

// knowledgeConfigLoadingCmds are every knowledge subcommand that loads the
// project's configuration before doing its work, each with the minimum
// arguments needed to get past its own input validation and reach the load.
// `read` and `write` both parse their address before loading, so they are given
// a well-formed one; `write` never reaches --file, which is read only after the
// store set is built. `categories` is deliberately absent: it answers from the
// built-in category definitions and never loads a config, so no configuration
// can make it fail.
var knowledgeConfigLoadingCmds = map[string][]string{
	"search":         {"search", "compass"},
	"read":           {"read", "--data", `{"tier":"repo","name":"testproj","path":"readme.md"}`},
	"list":           {"list"},
	"write":          {"write", "--data", `{"tier":"repo","name":"testproj","path":"learnings/note.md"}`},
	"sources":        {"sources"},
	"conventions":    {"conventions"},
	"always-applied": {"always-applied"},
	"tags":           {"tags"},
}

// knowledgeProjectWithConfigs lays out a temp project rooted at a t.TempDir()
// and chdirs into it, writing the two configuration files verbatim so a test
// can put either of them into the superseded form by hand. It returns the
// project root.
func knowledgeProjectWithConfigs(t *testing.T, projectCfg, repoCfg string) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)

	dataDir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	writeCurrentConfig(t, root, projectCfg)
	writeCurrentRepoConfig(t, root, repoCfg)

	return root
}

// Phase 2.3 criteria 1 & 2: when a registered repo's repo.yaml still declares
// knowledge in the superseded form, every knowledge operation that loads
// configuration fails, and it fails with the loader's own config_invalid
// refusal naming that file.
//
// The code assertion is the point of the test. Aggregation used to fold any
// footprint failure into a repo_footprint error whose next action was "run
// `repo add` to repair the repo's footprint" — advice that cannot fix a stale
// knowledge block and hides the remediation the loader already wrote.
func TestKnowledge_SupersededRepoKnowledgeBlockFailsEveryOperation(t *testing.T) {
	const projectCfg = "name: testproj\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: .\n"
	const legacyRepoCfg = "knowledge:\n" +
		"  sources:\n" +
		"    - name: testproj\n" +
		"      provider: file\n" +
		"      config:\n" +
		"        location: knowledge\n"

	for name, args := range knowledgeConfigLoadingCmds {
		t.Run(name, func(t *testing.T) {
			root := knowledgeProjectWithConfigs(t, projectCfg, legacyRepoCfg)
			repoPath := filepath.Join(root, ".spektacular", config.RepoConfigFileName)

			stdout, _, err := runKnowledge(t, args...)
			require.Error(t, err)

			var envelope output.ErrorResponse
			require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
			// config_invalid, not repo_footprint: the refusal reaches the
			// caller intact instead of being replaced by "re-add the repo".
			require.Equal(t, "config_invalid", envelope.Code)
			require.Contains(t, envelope.Message, repoPath)
			require.Contains(t, envelope.Message, "'sources'")
			require.Equal(t, repoPath, envelope.Resource)
			require.Contains(t, envelope.NextAction, "provider: file")
			require.NotContains(t, envelope.NextAction, "repo add")
		})
	}
}

// Phase 2.3 criteria 1 & 2: when the project's own config.yaml still
// identifies its shared stores by the removed 'scope' key, every knowledge
// operation that loads configuration fails with a config_invalid error naming
// config.yaml and the key.
func TestKnowledge_SupersededProjectKnowledgeScopeFailsEveryOperation(t *testing.T) {
	const legacyProjectCfg = "name: testproj\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: .\n" +
		"knowledge:\n" +
		"  sources:\n" +
		"    - scope: team\n" +
		"      provider: file\n" +
		"      config:\n" +
		"        location: ./team-knowledge\n"
	const repoCfg = "knowledge:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    location: knowledge\n"

	for name, args := range knowledgeConfigLoadingCmds {
		t.Run(name, func(t *testing.T) {
			root := knowledgeProjectWithConfigs(t, legacyProjectCfg, repoCfg)
			cfgPath := filepath.Join(root, ".spektacular", "config.yaml")

			stdout, _, err := runKnowledge(t, args...)
			require.Error(t, err)

			var envelope output.ErrorResponse
			require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
			require.Equal(t, "config_invalid", envelope.Code)
			require.Contains(t, envelope.Message, cfgPath)
			require.Contains(t, envelope.Message, "knowledge.sources[0]")
			require.Contains(t, envelope.Message, "'scope'")
			require.Equal(t, cfgPath, envelope.Resource)
			require.Contains(t, envelope.NextAction, "- name: team")
		})
	}
}

// Phase 2.3 criterion 5: with the same project corrected — the repo declaring
// its single knowledge store as one provider block — the knowledge operation
// that was refused above succeeds against that store.
func TestKnowledge_CorrectedRepoKnowledgeBlockSucceeds(t *testing.T) {
	const projectCfg = "name: testproj\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: .\n"
	const correctedRepoCfg = "knowledge:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    location: knowledge\n"

	root := knowledgeProjectWithConfigs(t, projectCfg, correctedRepoCfg)
	loc := filepath.Join(root, ".spektacular", "knowledge")
	require.NoError(t, os.MkdirAll(loc, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(loc, "readme.md"), []byte("the compass points north\n"), 0o644))

	stdout, _, err := runKnowledge(t, "sources")
	require.NoError(t, err)

	var result struct {
		Sources []knowledgeSource `json:"sources"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, []knowledgeSource{
		{Tier: "repo", Name: "testproj", Provider: "file", Location: loc},
	}, result.Sources)
}

// Phase 2.1 criterion 3: tags ship with no migration, so every knowledge
// subcommand must still run unchanged against a knowledge base whose entries
// carry no frontmatter block at all — which is every entry that exists today.
// twoScopeProject seeds exactly such a base, and the arguments come from
// knowledgeConfigLoadingCmds, the same map the config-failure tests drive, so
// the coverage here stays in step with the command surface. `categories` is
// added back because it answers from the built-in registry and so is absent
// there, and `write` is given the --file its own success path needs.
//
// The subcommand assertions alone would pass against a store that quietly
// dropped every entry, so the list and search assertions below carry the
// "no entry is rejected as invalid" half of the criterion: all three seeded
// entries are still enumerated, and the ones mentioning the keyword are still
// found with their bodies intact.
func TestKnowledge_EveryCommandRunsAgainstAnUntaggedKnowledgeBase(t *testing.T) {
	subcommands := map[string][]string{"categories": {"categories"}}
	for name, args := range knowledgeConfigLoadingCmds {
		subcommands[name] = args
	}

	for name, args := range subcommands {
		t.Run(name, func(t *testing.T) {
			twoScopeProject(t)

			if name == "write" {
				contentPath := filepath.Join(t.TempDir(), "payload.md")
				require.NoError(t, os.WriteFile(contentPath, []byte("untagged content\n"), 0o644))
				args = append(append([]string{}, args...), "--file", contentPath)
			}

			stdout, stderr, err := runKnowledge(t, args...)
			require.NoError(t, err,
				"%s must succeed against a knowledge base whose entries carry no tags", name)
			require.Empty(t, stderr)

			var envelope map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
			require.JSONEq(t, "false", string(envelope["error"]),
				"%s must not report an error envelope for untagged entries", name)
		})
	}

	t.Run("no entry is rejected", func(t *testing.T) {
		twoScopeProject(t)

		stdout, _, err := runKnowledge(t, "list")
		require.NoError(t, err)
		var listed struct {
			Entries []knowledgeEntry `json:"entries"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
		require.ElementsMatch(t, []knowledgeEntry{
			{Tier: "repo", Name: "testproj", Path: "readme.md"},
			{Tier: "repo", Name: "testproj", Path: "architecture/initial-idea.md"},
			{Tier: "project", Name: "team", Path: "guidelines.md"},
		}, listed.Entries, "an untagged entry must never be dropped from the listing")

		stdout, _, err = runKnowledge(t, "search", "compass")
		require.NoError(t, err)
		var found struct {
			Hits []knowledgeHit `json:"hits"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &found))
		require.Equal(t, []knowledgeHit{
			{
				Tier:     "repo",
				Name:     "testproj",
				Path:     "readme.md",
				Title:    "readme.md",
				Excerpts: []string{"project readme: the compass points north"},
				Score:    1,
				Tags:     []string{},
			},
			{
				Tier:     "project",
				Name:     "team",
				Path:     "guidelines.md",
				Title:    "guidelines.md",
				Excerpts: []string{"team guidelines reference the compass too"},
				Score:    1,
				Tags:     []string{},
			},
		}, found.Hits, "untagged entries must still match, title and excerpt unchanged")
	})
}

// taggedSearchProject lays the standard two-store project out and seeds the
// repo-tier store with three entries under learnings/ that all mention the
// marker term "beacon": one carrying both tags, one carrying only "http", and
// one carrying none. The marker is chosen so none of twoScopeProject's own
// fixture files can appear in these results.
//
// Every entry mentions "beacon" twice or once, so all three clear the relative
// cutoff and an empty narrowed result can only come from the tag filter.
//
// It returns the two store locations so a test needing more than the tagged
// search entries can seed either store without restating the layout.
func taggedSearchProject(t *testing.T) (projectLoc, teamLoc string) {
	t.Helper()
	_, projectLoc, teamLoc = twoScopeProject(t)
	seedKnowledgeFile(t, projectLoc, "learnings/both.md",
		"---\n"+
			"tags: [http, routing]\n"+
			"---\n"+
			"# Beacon\n"+
			"\n"+
			"the beacon signal is strong\n")
	seedKnowledgeFile(t, projectLoc, "learnings/http-only.md",
		"---\n"+
			"tags: [http]\n"+
			"---\n"+
			"# Beacon Two\n"+
			"\n"+
			"another beacon note\n")
	seedKnowledgeFile(t, projectLoc, "learnings/untagged.md",
		"the beacon signal is here as well\n")
	return projectLoc, teamLoc
}

// searchPaths runs `knowledge search` with the given arguments and returns the
// paths of the hits it reported, so a tag test states its expectation as a
// hand-written list.
func searchPaths(t *testing.T, args ...string) []string {
	t.Helper()
	stdout, _, err := runKnowledge(t, append([]string{"search"}, args...)...)
	require.NoError(t, err)

	var result struct {
		Hits []knowledgeHit `json:"hits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	paths := make([]string, 0, len(result.Hits))
	for _, h := range result.Hits {
		paths = append(paths, h.Path)
	}
	return paths
}

// Phase 2.3 criterion 4: the narrowing option is discoverable from the
// command's own published description of itself, alongside the two narrowing
// options search already advertised. A caller reading only the schema must
// learn that --tag exists and that it is repeatable.
func TestKnowledgeSearchSchema_PublishesTheTagNarrowingOption(t *testing.T) {
	twoScopeProject(t)

	schema, _ := knowledgeSchema(t, "search")
	require.NotNil(t, schema.Flags)
	require.Contains(t, schema.Flags, "tier", "the existing narrowing options must survive alongside --tag")
	require.Contains(t, schema.Flags, "filter")

	tag := schema.Flags["tag"]
	require.NotNil(t, tag, "search must publish its --tag option")
	require.Equal(t, "array", tag.Type, "--tag is repeatable, so it publishes as an array")
	require.NotNil(t, tag.Items)
	require.Equal(t, "string", tag.Items.Type)
}

// Phase 2.3 criterion 4, the other half: --tag is registered on search alone,
// so the other three fan-out commands must not advertise it. This is the reason
// knowledgeSearchFlags exists separately from knowledgeNarrowingFlags —
// folding --tag into the shared map would have the three commands publish an
// option they silently ignore.
func TestKnowledgeSchemas_OnlySearchPublishesTheTagOption(t *testing.T) {
	twoScopeProject(t)

	for _, sub := range []string{"list", "conventions", "always-applied", "tags"} {
		t.Run(sub, func(t *testing.T) {
			schema, _ := knowledgeSchema(t, sub)
			require.NotNil(t, schema.Flags, "%s still publishes its own narrowing options", sub)
			require.Contains(t, schema.Flags, "tier")
			require.NotContains(t, schema.Flags, "tag",
				"%s does not accept --tag and must not advertise it", sub)
		})
	}
}

// Phase 2.3 criteria 1 and 2 at the command line: --tag reaches the knowledge
// layer and narrows the result to entries carrying the tag. The unnarrowed run
// asserts what the fixture would otherwise return, so the narrowed one is a
// statement about the flag rather than about the fixture.
func TestKnowledgeSearch_TagNarrowsToEntriesCarryingIt(t *testing.T) {
	taggedSearchProject(t)

	require.ElementsMatch(t,
		[]string{"learnings/both.md", "learnings/http-only.md", "learnings/untagged.md"},
		searchPaths(t, "beacon"),
		"unnarrowed, every seeded entry is returned")

	require.Equal(t,
		[]string{"learnings/both.md"},
		searchPaths(t, "beacon", "--tag", "routing"),
		"only the entry carrying the tag may be returned")
}

// Phase 2.3 criterion 3 at the command line: repeating --tag is AND, not OR.
// The http-only entry carries one of the two tags and must not be returned,
// which is exactly what an OR reading would get wrong.
func TestKnowledgeSearch_RepeatedTagIsAnd(t *testing.T) {
	taggedSearchProject(t)

	require.Equal(t,
		[]string{"learnings/both.md"},
		searchPaths(t, "beacon", "--tag", "http", "--tag", "routing"))
}

// Phase 2.3: --tag values are lower-cased before they become a selector, so a
// tag typed in any case finds an entry whose declared tags were lower-cased on
// read. Without that normalisation "HTTP" would match nothing at all.
func TestKnowledgeSearch_TagValueIsLowerCased(t *testing.T) {
	taggedSearchProject(t)

	require.ElementsMatch(t,
		[]string{"learnings/both.md", "learnings/http-only.md"},
		searchPaths(t, "beacon", "--tag", "HTTP"))
}

// Phase 3.1: a --tag naming a tag no entry carries is refused rather than
// answered with an empty result a caller would read as "there is no knowledge on
// this subject". This is the inversion of the Phase 2.3 test that pinned the
// interim behaviour: refusing needed a way to say which tags do exist, which is
// exactly what `knowledge tags` now delivers. The next action names that runnable
// command, per the repo's convention that every refusal carries its remediation.
func TestKnowledgeSearch_UnknownTagIsRefusedWithTheVocabulary(t *testing.T) {
	taggedSearchProject(t)

	stdout, stderr, err := runKnowledge(t, "search", "beacon", "--tag", "no-such-tag")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "knowledge_tag_unknown", envelope.Code)
	require.Contains(t, envelope.Message, "no-such-tag")
	require.Contains(t, envelope.NextAction, "knowledge tags",
		"the correction names the runnable command that lists the vocabulary")
	require.Contains(t, envelope.NextAction, "http",
		"and the tags that are in use, so the caller can reissue without reading every entry")
}

// tagVocabularyProject lays the tagged search fixture out and adds one entry to
// the project-tier "team" store, so the vocabulary spans both stores and a tag
// used in each has to be summed. Counting by hand across the two stores:
// "http" is carried by learnings/both.md and learnings/http-only.md in the
// repo-tier store, "routing" by learnings/both.md there and learnings/edge.md in
// the team store, and "cache" by learnings/edge.md alone — so http and routing
// tie at two and cache trails at one.
func tagVocabularyProject(t *testing.T) {
	t.Helper()
	_, teamLoc := taggedSearchProject(t)
	seedKnowledgeFile(t, teamLoc, "learnings/edge.md",
		"---\n"+
			"tags: [routing, cache]\n"+
			"---\n"+
			"# Edge\n"+
			"\n"+
			"the team's note on edge routing\n")
}

// knowledgeTagUse mirrors the knowledge.TagUse JSON envelope emitted by tags:
// one distinct tag and the number of entries carrying it.
type knowledgeTagUse struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// knowledgeTagVocabulary runs `knowledge tags` with the given narrowing options
// and returns the vocabulary it reported, in the order it reported it.
func knowledgeTagVocabulary(t *testing.T, args ...string) []knowledgeTagUse {
	t.Helper()
	stdout, _, err := runKnowledge(t, append([]string{"tags"}, args...)...)
	require.NoError(t, err)

	var result struct {
		Tags []knowledgeTagUse `json:"tags"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	return result.Tags
}

// Phase 3.1 criteria 1 and 2 at the command line: `knowledge tags` emits each
// distinct tag with its entry count, most-used first and ties broken
// alphabetically. The expected slice is hand-counted from the fixture and
// compared whole, so the order is part of the assertion.
func TestKnowledgeTags_ReportsTheVocabularyMostUsedFirst(t *testing.T) {
	tagVocabularyProject(t)

	require.Equal(t, []knowledgeTagUse{
		{Tag: "http", Count: 2},    // both.md and http-only.md
		{Tag: "routing", Count: 2}, // both.md and the team store's edge.md — tied, so alphabetical
		{Tag: "cache", Count: 1},   // edge.md alone
	}, knowledgeTagVocabulary(t))
}

// Phase 3.1 criterion 3 at the command line: the listing honours --tier and
// --filter exactly as the other fan-out commands do. Each expected vocabulary is
// hand-counted for the stores that narrowing leaves in scope.
func TestKnowledgeTags_HonoursTierAndFilter(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want []knowledgeTagUse
	}{
		"tier repo": {
			[]string{"--tier", "repo"},
			[]knowledgeTagUse{{Tag: "http", Count: 2}, {Tag: "routing", Count: 1}},
		},
		"tier project": {
			[]string{"--tier", "project"},
			[]knowledgeTagUse{{Tag: "cache", Count: 1}, {Tag: "routing", Count: 1}},
		},
		"filter one store": {
			[]string{"--filter", "team"},
			[]knowledgeTagUse{{Tag: "cache", Count: 1}, {Tag: "routing", Count: 1}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			tagVocabularyProject(t)
			require.Equal(t, tc.want, knowledgeTagVocabulary(t, tc.args...))
		})
	}
}

// Phase 3.1 criterion 4 at the command line: a knowledge base whose entries
// carry no tags reports an empty vocabulary. The assertion is on the raw JSON so
// the distinction that matters is stated: "tags" must be present and an empty
// array, not null and not an omitted key, since a caller decoding it should get
// a list it can iterate over rather than a missing value to guard against.
func TestKnowledgeTags_UntaggedKnowledgeBaseEmitsAnEmptyArray(t *testing.T) {
	twoScopeProject(t)

	stdout, stderr, err := runKnowledge(t, "tags")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.Contains(t, envelope, "tags")
	require.JSONEq(t, "[]", string(envelope["tags"]))
}

// Phase 3.1 criterion 1, via the published interface: `tags --schema` declares
// the shape of what it emits — an array of {tag, count} objects — alongside the
// narrowing options it accepts, so a caller can consume the vocabulary without
// running the command first. The enum and item types of --tier and --filter are
// asserted for every fan-out command in
// TestKnowledgeSchemas_FanOutCommandsPublishNarrowingFlags.
func TestKnowledgeTags_SchemaDeclaresTheVocabularyShape(t *testing.T) {
	twoScopeProject(t)

	schema, _ := knowledgeSchema(t, "tags")
	require.NotNil(t, schema.Output)
	require.Equal(t, []string{"tags"}, mapKeys(schema.Output.Properties))

	tags := schema.Output.Properties["tags"]
	require.Equal(t, "array", tags.Type)
	require.NotNil(t, tags.Items)
	require.Equal(t, "object", tags.Items.Type)
	require.Equal(t, "string", tags.Items.Properties["tag"].Type)
	require.Equal(t, "number", tags.Items.Properties["count"].Type)
	require.ElementsMatch(t, []string{"tag", "count"}, mapKeys(tags.Items.Properties),
		"a vocabulary item carries the tag and its count, and no store address")

	require.NotNil(t, schema.Flags)
	require.Contains(t, schema.Flags, "tier")
	require.Contains(t, schema.Flags, "filter")
}
