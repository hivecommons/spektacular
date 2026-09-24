package cmd

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// repoListEntry mirrors the repoInfo JSON envelope emitted by `repo list`.
type repoListEntry struct {
	Name         string   `json:"name"`
	Location     string   `json:"location"`
	Root         string   `json:"root"`
	Provider     string   `json:"provider"`
	Description  string   `json:"description"`
	Role         string   `json:"role"`
	Tags         []string `json:"tags"`
	Dependencies []string `json:"dependencies"`
	Materialized bool     `json:"materialized"`
	StaleNote    string   `json:"stale_note"`
	MetadataNote string   `json:"metadata_note"`
}

// repoAddResult mirrors the `repo add` success envelope.
type repoAddResult struct {
	Registered   bool   `json:"registered"`
	Footprint    string `json:"footprint"`
	MetadataNote string `json:"metadata_note"`
}

// runRepo invokes the repo command tree via runRoot (the same wrapper Execute
// uses) and returns the captured stdout and stderr buffers. On failure, err
// is the *output.ErrorResponse unmarshaled from stdout, matching what a real
// invocation of the CLI returns.
func runRepo(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetRootCmd(t)
	out, errBuf := setupImplementCmd(t)
	rootCmd.SetArgs(append([]string{"repo"}, args...))
	if code := runRoot(); code != 0 {
		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal(out.Bytes(), &er))
		err = &er
	}
	return out.String(), errBuf.String(), err
}

// repoProject lays out a minimal temp project rooted at a t.TempDir() and
// chdirs into it, returning the project root.
func repoProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	return dir
}

// repoAddJSON marshals the given fields into a --data payload, so paths land
// in the JSON correctly escaped.
func repoAddJSON(t *testing.T, fields map[string]any) string {
	t.Helper()
	b, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(b)
}

// swapRepoGit substitutes the package-level GitRunner the repo commands use,
// restoring the original when the test finishes.
func swapRepoGit(t *testing.T, g repo.GitRunner) {
	t.Helper()
	orig := repoGit
	repoGit = g
	t.Cleanup(func() { repoGit = orig })
}

// stubGit is a counting repo.GitRunner for the cmd layer: Clone only creates
// the directory (recording each url→dir pair in clones), and the head
// queries return canned values. calls counts every git invocation, clones
// included, so a zero count proves no git ran at all.
type stubGit struct {
	localHead  string
	remoteHead string
	calls      int
	clones     []stubClone
}

// stubClone is one recorded Clone invocation.
type stubClone struct {
	url string
	dir string
}

func (s *stubGit) Clone(url, dir string) error {
	s.calls++
	s.clones = append(s.clones, stubClone{url: url, dir: dir})
	return os.MkdirAll(dir, 0o755)
}

func (s *stubGit) LocalHead(dir string) (string, error) {
	s.calls++
	return s.localHead, nil
}

func (s *stubGit) RemoteHead(url string) (string, error) {
	s.calls++
	return s.remoteHead, nil
}

// listPaths returns every entry under root — directories and files — as
// root-relative slash paths, directories marked with a trailing slash.
func listPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel += "/"
		}
		paths = append(paths, rel)
		return nil
	}))
	return paths
}

// minimalFootprint is the exact expected content of a freshly footprinted
// repo: the repo config plus the knowledge tree, and nothing else — no agent
// guidance, no version file, no skills. The category list is hand-maintained
// to stay independent of the registry the production code scaffolds from.
var minimalFootprint = []string{
	".spektacular/",
	".spektacular/repo.yaml",
	".spektacular/knowledge/",
	".spektacular/knowledge/conventions/",
	".spektacular/knowledge/conventions/README.md",
	".spektacular/knowledge/glossary/",
	".spektacular/knowledge/glossary/README.md",
	".spektacular/knowledge/architecture/",
	".spektacular/knowledge/architecture/README.md",
	".spektacular/knowledge/gotchas/",
	".spektacular/knowledge/gotchas/README.md",
	".spektacular/knowledge/learnings/",
	".spektacular/knowledge/learnings/README.md",
	".spektacular/knowledge/decisions/",
	".spektacular/knowledge/decisions/README.md",
}

// Criterion 1: after `repo add`, the project config reflects the entry's
// name, location, and metadata, and the target repo contains exactly its
// config file and knowledge storage — nothing else.
//
// This also stands as Phase 2.1 criterion 4: collapsing repo.yaml's knowledge
// section to a single provider block must leave the scaffolded category
// directories and their READMEs exactly as they were, which is what the
// hand-maintained minimalFootprint listing below pins.
func TestRepoAdd_RegistersEntryAndCreatesMinimalFootprint(t *testing.T) {
	project := repoProject(t)
	target := t.TempDir()

	stdout, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":         "docs",
		"location":     target,
		"description":  "the documentation repo",
		"role":         "documentation",
		"tags":         []string{"docs", "markdown"},
		"dependencies": []string{"api"},
	}))
	require.NoError(t, err)

	var result repoAddResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.True(t, result.Registered)
	require.Equal(t, "created", result.Footprint)

	// The project config carries the entry's membership fields only.
	cfg, err := config.FromYAMLFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 2, "the project entry plus the added repo")
	entry := cfg.Repos[1]
	require.Equal(t, "docs", entry.Name)
	require.Equal(t, filepath.Join(target, ".spektacular"), entry.Location, "add registers the footprint folder it scaffolded")
	require.Equal(t, []string{"api"}, entry.Dependencies)

	// The project config's own YAML carries no descriptive metadata keys at
	// all under the repo entry — RepoEntry is membership-only, so this isn't
	// just "the Go struct has no field", the serialized file itself must not
	// carry these keys either.
	rawCfg, err := os.ReadFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	var generic struct {
		Repos []map[string]any `yaml:"repos"`
	}
	require.NoError(t, yaml.Unmarshal(rawCfg, &generic))
	require.Len(t, generic.Repos, 2)
	for _, field := range []string{"description", "role", "tags"} {
		require.NotContains(t, generic.Repos[1], field, "project config must carry no descriptive metadata fields")
	}

	// The target repo contains exactly the minimal footprint.
	require.ElementsMatch(t, minimalFootprint, listPaths(t, target))

	// The target repo's own config also carries the descriptive metadata: the
	// input is written back into the repo, not only into the project config.
	targetCfg, err := config.RepoConfigFromYAMLFile(filepath.Join(target, ".spektacular", config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, "the documentation repo", targetCfg.Description)
	require.Equal(t, "documentation", targetCfg.Role)
	require.Equal(t, []string{"docs", "markdown"}, targetCfg.Tags)
}

// Phase 2.1 criterion 3: the repo.yaml `repo add` scaffolds is accepted
// as-is — no hand edit — and the store it declares is immediately writable and
// readable through the knowledge commands, addressed by the registry name.
func TestRepoAdd_ScaffoldedKnowledgeStoreIsUsableWithoutEditingItsConfig(t *testing.T) {
	project := repoProject(t)
	target := t.TempDir()

	// The project's own colocated repo joins the store set too, so give it the
	// store its repo.yaml declares; the added repo is what this test is about.
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".spektacular", "knowledge"), 0o755))

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "docs",
		"location": target,
	}))
	require.NoError(t, err)

	// The generated config parses and validates untouched, and declares the
	// single knowledge block at the default location.
	scaffolded, err := config.RepoConfigFromYAMLFile(filepath.Join(target, ".spektacular", config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, config.RepoKnowledgeConfig{
		Provider: "file",
		Config:   config.FileKnowledgeConfig{Location: "knowledge"},
	}, scaffolded.Knowledge)

	contentPath := filepath.Join(t.TempDir(), "payload.md")
	require.NoError(t, os.WriteFile(contentPath, []byte("scaffolded and immediately writable\n"), 0o644))

	_, _, err = runKnowledge(t, "write",
		"--data", `{"tier":"repo","name":"docs","path":"learnings/first.md"}`,
		"--file", contentPath)
	require.NoError(t, err)

	// The entry lands inside the scaffolded store...
	persisted := filepath.Join(target, ".spektacular", "knowledge", "learnings", "first.md")
	require.FileExists(t, persisted)

	// ...and reads back through the same address.
	stdout, _, err := runKnowledge(t, "read", "--data",
		`{"tier":"repo","name":"docs","path":"learnings/first.md"}`)
	require.NoError(t, err)

	var read knowledgeAddressResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &read))
	require.Equal(t, knowledgeAddressResult{
		Tier:    "repo",
		Name:    "docs",
		Path:    "learnings/first.md",
		Content: "scaffolded and immediately writable\n",
	}, read)
}

// Criterion 2: re-running `repo add` for the same repo — from this project or
// another one — changes nothing in the target repo.
func TestRepoAdd_ReAddChangesNothingInTargetRepo(t *testing.T) {
	repoProject(t)
	target := t.TempDir()
	data := repoAddJSON(t, map[string]any{
		"name":        "docs",
		"location":    target,
		"description": "the documentation repo",
		"role":        "documentation",
	})

	_, _, err := runRepo(t, "add", "--data", data)
	require.NoError(t, err)
	before := snapshotDir(t, target)

	// Re-add from the same project: byte-identical target, "unchanged" status.
	stdout, _, err := runRepo(t, "add", "--data", data)
	require.NoError(t, err)
	var result repoAddResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "unchanged", result.Footprint)
	require.Equal(t, before, snapshotDir(t, target))

	// Add the same target from a second project: the second project's config
	// gains the entry, the target repo is still untouched.
	project2 := t.TempDir()
	t.Chdir(project2)
	writeSpecCommandConfig(t, project2, "")

	stdout, _, err = runRepo(t, "add", "--data", data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "unchanged", result.Footprint)
	require.Equal(t, before, snapshotDir(t, target))

	cfg2, err := config.FromYAMLFile(filepath.Join(project2, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg2.Repos, 2)
	require.Equal(t, "docs", cfg2.Repos[1].Name)
	require.Equal(t, filepath.Join(target, ".spektacular"), cfg2.Repos[1].Location)
}

// Criterion 3: `repo list` reports every registered repo with descriptive
// metadata sourced from the repo's OWN repo.yaml (not from any project-config
// entry — RepoEntry no longer carries descriptive fields at all); a repo
// whose registered location is not on disk reports metadata absent, an
// empty root, and materialized false, and listing never invokes git or
// clones it.
func TestRepoList_ReportsRegisteredReposWithMetadataAndRoots(t *testing.T) {
	_ = repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)

	// A materialized local repo carrying its own descriptive metadata in its
	// own repo.yaml.
	target := t.TempDir()
	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "docs",
		"location":    target,
		"description": "the documentation repo",
		"role":        "documentation",
	}))
	require.NoError(t, err)

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)

	var result struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Len(t, result.Repos, 2)

	docs := result.Repos[1]
	require.Equal(t, "docs", docs.Name)
	require.Equal(t, "the documentation repo", docs.Description)
	require.Equal(t, "documentation", docs.Role)
	require.Equal(t, target, docs.Root, "an on-disk local repo resolves to its absolute root")
	require.True(t, filepath.IsAbs(docs.Root))
	require.False(t, docs.Materialized)

	require.Zero(t, git.calls, "listing must never invoke git")
}

// A registered repo whose location is not on disk is a misregistration, and
// `repo list` says so instead of listing it with an empty root: the error is
// coded repo_location_missing, names the registered value and the absolute
// path it resolved to (so a relative location written from the project root
// instead of from config.yaml's folder is visible at a glance), and git is
// never invoked.
func TestRepoList_MissingLocationIsAnError(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)

	writeSpecCommandConfig(t, project,
		"repos:\n"+
			"  - name: absent\n"+
			"    location: ./repos/absent\n")

	stdout, stderr, err := runRepo(t, "list")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "repo_location_missing", envelope.Code)
	require.Contains(t, envelope.Message, `"absent"`, "the error must name the repo")
	require.Contains(t, envelope.Message, `"./repos/absent"`, "the error must echo the registered location")
	require.Contains(t, envelope.Message, filepath.Join(project, ".spektacular", "repos", "absent"), "the error must name the absolute path the location resolved to")
	require.Contains(t, envelope.Message, "config.yaml", "the error must say what a relative location is resolved from")
	require.Contains(t, envelope.NextAction, "repos[].location")
	require.Zero(t, git.calls, "listing must never invoke git")
}

// A registered location that exists but holds no .spektacular/repo.yaml is
// reported as repo_footprint_missing, naming the file that was expected, so
// a repo.yaml placed one level too high is not silently ignored.
func TestRepoList_MissingFootprintIsAnError(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)

	writeSpecCommandConfig(t, project,
		"repos:\n"+
			"  - name: api\n"+
			"    location: ../repos/api/.spektacular\n")
	location := filepath.Join(project, "repos", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(location, ".spektacular"), 0o755))
	// The footprint file sits one level above the registered folder, so it
	// is not where the CLI looks.
	require.NoError(t, config.NewDefaultRepoConfig().ToYAMLFile(filepath.Join(location, config.RepoConfigFileName)))

	stdout, stderr, err := runRepo(t, "list")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "repo_footprint_missing", envelope.Code)
	require.Contains(t, envelope.Message, `"api"`)
	require.Contains(t, envelope.NextAction, filepath.Join(location, ".spektacular", config.RepoConfigFileName), "the next action must name the file that was expected")
	require.Zero(t, git.calls, "listing must never invoke git")
}

// Criterion: the same physical repo registered into two separate projects
// reports the identical, current description under both projects once it is
// edited a single time in the repo's own repo.yaml — descriptive metadata is
// never duplicated per-project.
func TestRepoList_SameRepoAcrossTwoProjectsSeesOneEditedDescription(t *testing.T) {
	target := t.TempDir()

	project1 := repoProject(t)
	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "shared",
		"location":    target,
		"description": "original description",
	}))
	require.NoError(t, err)

	project2 := t.TempDir()
	t.Chdir(project2)
	writeSpecCommandConfig(t, project2, "")
	_, _, err = runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "shared",
		"location":    target,
		"description": "original description",
	}))
	require.NoError(t, err)

	// Edit the repo's own repo.yaml once, directly — not through either
	// project.
	repoCfgPath := filepath.Join(target, ".spektacular", config.RepoConfigFileName)
	repoCfg, err := config.RepoConfigFromYAMLFile(repoCfgPath)
	require.NoError(t, err)
	repoCfg.Description = "updated description"
	require.NoError(t, repoCfg.ToYAMLFile(repoCfgPath))

	listDescription := func(projectDir string) string {
		t.Chdir(projectDir)
		stdout, _, err := runRepo(t, "list")
		require.NoError(t, err)
		var result struct {
			Repos []repoListEntry `json:"repos"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &result))
		require.Len(t, result.Repos, 2)
		return result.Repos[1].Description
	}

	require.Equal(t, "updated description", listDescription(project1))
	require.Equal(t, "updated description", listDescription(project2))
}

// stale_note projection: a repo whose repo.yaml declares a git source, and
// whose materialized clone's local head differs from the remote's, carries a
// non-empty stale note in the listing, and reports the clone — the resolved
// source — as its root while echoing the registered location. The clone is
// pre-created so listing never clones.
func TestRepoList_StaleNoteFromDifferingHeads(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{localHead: "aaa111", remoteHead: "bbb222"}
	swapRepoGit(t, git)

	writeSpecCommandConfig(t, project,
		"repos:\n"+
			"  - name: member\n"+
			"    location: ../repos/member/.spektacular\n")

	// The repo's own footprint at its registered location declares the git
	// source.
	location := filepath.Join(project, "repos", "member")
	require.NoError(t, os.MkdirAll(filepath.Join(location, ".spektacular"), 0o755))
	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Source = config.GitSource("https://example.com/member.git")
	require.NoError(t, repoCfg.ToYAMLFile(
		filepath.Join(location, ".spektacular", config.RepoConfigFileName)))

	// Materialize the clone by hand so listing resolves it without cloning.
	clone := filepath.Join(project, ".spektacular", repo.MaterializeDirName, "member")
	require.NoError(t, os.MkdirAll(clone, 0o755))

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)

	var result struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Len(t, result.Repos, 1)

	member := result.Repos[0]
	require.Equal(t, "member", member.Name)
	require.Equal(t, filepath.Join(project, "repos", "member", ".spektacular"), member.Location, "the listed location is absolute")
	require.Equal(t, filepath.Join(project, ".spektacular", "repos", "member"), member.Root,
		"root is the clone of the git source, not the registered location")
	require.True(t, member.Materialized)
	require.Contains(t, member.StaleNote, "aaa111")
	require.Contains(t, member.StaleNote, "bbb222")
	require.Empty(t, git.clones, "listing must never clone")
	require.NoDirExists(t, filepath.Join(clone, ".spektacular"), "listing must not footprint the clone")
}

// upsert: re-adding a registered name with different metadata updates the
// config entry in place, leaves the target repo's footprint shape (its set of
// files) unchanged, but writes the new descriptive metadata into the target
// repo's own repo.yaml.
func TestRepoAdd_UpsertUpdatesMetadataAndRepoConfigWithoutChangingFootprintShape(t *testing.T) {
	project := repoProject(t)
	target := t.TempDir()

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "docs",
		"location":    target,
		"description": "first description",
		"role":        "documentation",
		"tags":        []string{"docs"},
	}))
	require.NoError(t, err)
	beforePaths := listPaths(t, target)

	stdout, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "docs",
		"location":    target,
		"description": "second description",
		"role":        "reference",
		"tags":        []string{"docs", "api"},
	}))
	require.NoError(t, err)

	var result repoAddResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.True(t, result.Registered)
	require.Equal(t, "unchanged", result.Footprint)

	cfg, err := config.FromYAMLFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 2, "an upsert must update in place, not append a duplicate")
	require.Equal(t, "docs", cfg.Repos[1].Name)

	// The footprint's file set is unchanged — same paths present, still the
	// minimal footprint shape.
	require.ElementsMatch(t, beforePaths, listPaths(t, target))

	// But repo.yaml's content legitimately changed: the target repo's own
	// config now carries the new descriptive metadata.
	targetCfg, err := config.RepoConfigFromYAMLFile(filepath.Join(target, ".spektacular", config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, "second description", targetCfg.Description)
	require.Equal(t, "reference", targetCfg.Role)
	require.Equal(t, []string{"docs", "api"}, targetCfg.Tags)
}

// Criterion 3: registering a repository with no descriptive metadata
// supplied leaves an already-described repository's own configuration
// unchanged — a re-add that omits description/role/tags must not
// blank out metadata written by an earlier, fuller registration.
func TestRepoAdd_ReAddWithNoMetadataLeavesRepoConfigUnchanged(t *testing.T) {
	repoProject(t)
	target := t.TempDir()

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "docs",
		"location":    target,
		"description": "the documentation repo",
		"role":        "documentation",
		"tags":        []string{"docs", "markdown"},
	}))
	require.NoError(t, err)

	// Re-register the same repo supplying only name + location — no
	// descriptive fields at all.
	_, _, err = runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "docs",
		"location": target,
	}))
	require.NoError(t, err)

	targetCfg, err := config.RepoConfigFromYAMLFile(filepath.Join(target, ".spektacular", config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, "the documentation repo", targetCfg.Description, "an omitted description must not blank out an existing one")
	require.Equal(t, "documentation", targetCfg.Role)
	require.Equal(t, []string{"docs", "markdown"}, targetCfg.Tags)
}

// Phase 1.4 criterion 2: `repo add` with no location is rejected at input
// validation as an invalid_data envelope that names the repo and the missing
// key, and whose next action carries a payload example including
// "location" — before anything is written or registered.
func TestRepoAdd_MissingLocationErrors(t *testing.T) {
	project := repoProject(t)

	stdout, stderr, err := runRepo(t, "add", "--data", `{"name":"ghost"}`)
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "invalid_data", envelope.Code)
	require.Contains(t, envelope.Message, `"location"`)
	require.Contains(t, envelope.Message, "ghost", "the error must name the repo")
	require.Contains(t, envelope.NextAction, `"location"`, "the next action must carry a payload example with location")

	cfg, err := config.FromYAMLFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 1, "a rejected add must not register anything beyond the project itself")
}

// Phase 1.4 criterion 2: the removed registry key "address" is rejected as
// invalid_data rather than silently ignored; the message points at "source"
// and the next action carries a corrected payload with the given address as
// its source alongside a location.
func TestRepoAdd_AddressKeyRejectedWithSourceHint(t *testing.T) {
	project := repoProject(t)

	stdout, stderr, err := runRepo(t, "add", "--data", `{"name":"x","location":"./x","address":"git@h:o/x.git"}`)
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Equal(t, "invalid_data", envelope.Code)
	require.Contains(t, envelope.Message, `"address"`)
	require.Contains(t, envelope.Message, `"source"`)
	require.Contains(t, envelope.NextAction, `"source":"git@h:o/x.git"`)
	require.Contains(t, envelope.NextAction, `"location"`)

	cfg, err := config.FromYAMLFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 1, "a rejected add must not register anything beyond the project itself")
	require.NoDirExists(t, filepath.Join(project, "x"), "a rejected add must not create the location")
}

// `repo add` with no --data fails with a message carrying a --data example.
func TestRepoAdd_MissingDataErrors(t *testing.T) {
	repoProject(t)

	stdout, stderr, err := runRepo(t, "add")
	require.Error(t, err)
	require.Empty(t, stderr)

	var envelope output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.True(t, envelope.IsError)
	require.Contains(t, envelope.Message, "--data is required")
	require.Contains(t, envelope.Message, `"name"`, "the error must carry a usable --data example")
}

// `repo add --schema` prints the documented input/output schema instead of
// running the command.
func TestRepoAdd_SchemaDocumentsInputAndOutput(t *testing.T) {
	repoProject(t)

	stdout, _, err := runRepo(t, "add", "--schema")
	require.NoError(t, err)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Input)
	for _, field := range []string{"name", "location", "source", "description", "role", "tags", "dependencies"} {
		require.Contains(t, schema.Input.Properties, field)
	}
	for _, field := range []string{"address", "local"} {
		require.NotContains(t, schema.Input.Properties, field, "the removed registry keys must not be advertised as input")
	}
	require.Equal(t, []string{"name", "location"}, schema.Input.Required)
	require.NotNil(t, schema.Output)
	require.Contains(t, schema.Output.Properties, "registered")
	require.Contains(t, schema.Output.Properties, "footprint")
	require.ElementsMatch(t, []string{"created", "repaired", "unchanged"}, schema.Output.Properties["footprint"].Enum)
}

// `repo list --schema` declares the per-repo projection the listing emits.
func TestRepoList_SchemaDeclaresReposArray(t *testing.T) {
	repoProject(t)

	stdout, _, err := runRepo(t, "list", "--schema")
	require.NoError(t, err)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Output)
	require.Contains(t, schema.Output.Properties, "repos")

	repos := schema.Output.Properties["repos"]
	require.Equal(t, "array", repos.Type)
	require.NotNil(t, repos.Items)
	for _, field := range []string{"name", "location", "root", "materialized", "stale_note"} {
		require.Contains(t, repos.Items.Properties, field)
	}
	for _, field := range []string{"address", "local"} {
		require.NotContains(t, repos.Items.Properties, field, "the removed registry keys must not be advertised in the listing")
	}
}

// `repo add` with no descriptive metadata supplied returns a metadata_note
// naming the repo, nudging the user toward describing it.
func TestRepoAdd_NoMetadataReturnsMetadataNote(t *testing.T) {
	repoProject(t)
	target := t.TempDir()

	stdout, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "docs",
		"location": target,
	}))
	require.NoError(t, err)

	var result repoAddResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.NotEmpty(t, result.MetadataNote, "registering with no descriptive metadata must produce a metadata_note")
	require.Contains(t, result.MetadataNote, "docs", "the note must name the repo")
}

// `repo add` with descriptive metadata supplied returns no metadata_note.
func TestRepoAdd_WithMetadataReturnsNoMetadataNote(t *testing.T) {
	repoProject(t)
	target := t.TempDir()

	stdout, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "docs",
		"location":    target,
		"description": "the documentation repo",
		"role":        "documentation",
	}))
	require.NoError(t, err)

	var result repoAddResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Empty(t, result.MetadataNote, "registering with descriptive metadata must not produce a metadata_note")
}

// `repo list` reports a metadata_note for a materialized repo whose own
// repo.yaml carries no descriptive metadata.
func TestRepoList_MaterializedNoMetadataReturnsMetadataNote(t *testing.T) {
	repoProject(t)
	target := t.TempDir()

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "docs",
		"location": target,
	}))
	require.NoError(t, err)

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)

	var result struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Len(t, result.Repos, 2)
	require.NotEmpty(t, result.Repos[1].MetadataNote, "a materialized repo with no descriptive metadata must carry a metadata_note")
	require.Contains(t, result.Repos[1].MetadataNote, "docs")
}

// `repo list` reports no metadata_note for a materialized repo whose own
// repo.yaml carries descriptive metadata.
func TestRepoList_MaterializedWithMetadataReturnsNoMetadataNote(t *testing.T) {
	repoProject(t)
	target := t.TempDir()

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "docs",
		"location":    target,
		"description": "the documentation repo",
		"role":        "documentation",
	}))
	require.NoError(t, err)

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)

	var result struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Len(t, result.Repos, 2)
	require.Empty(t, result.Repos[1].MetadataNote, "a materialized repo with descriptive metadata must carry no metadata_note")
}

// `repo list` reports no metadata_note for a repo whose registered location
// is not on disk: the note only makes sense once a repo can actually be
// inspected, and an absent entry already reports no metadata for an
// unrelated reason (there is nothing to read), so nagging about it here
// would be premature and misleading.
func TestRepoList_UnmaterializedReturnsNoMetadataNote(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)

	writeSpecCommandConfig(t, project,
		"repos:\n"+
			"  - name: api\n"+
			"    location: ../repos/api/.spektacular\n")
	location := filepath.Join(project, "repos", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(location, ".spektacular"), 0o755))
	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Source = config.GitSource("https://example.com/api.git")
	repoCfg.Description = "the API service"
	require.NoError(t, repoCfg.ToYAMLFile(
		filepath.Join(location, ".spektacular", config.RepoConfigFileName)))

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)

	var result struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Len(t, result.Repos, 1)
	require.False(t, result.Repos[0].Materialized)
	require.Empty(t, result.Repos[0].MetadataNote, "a described repo whose git source is not cloned yet must carry no metadata_note")
	require.Zero(t, git.calls, "listing must never invoke git or clone the uncloned repo")
}

// Phase 1.4 criterion 1: `repo add` with a relative location that does not
// exist yet and a file:// source creates the footprint at the location —
// not at the source — and records the source in the repo's own repo.yaml.
// A later add for the same repo that omits the source (supplying only
// name, location, and a new description) keeps the stored source line, and
// the code directory never gains a .spektacular/ of its own.
func TestRepoAdd_FileSourceWrittenToRepoYAMLAndKeptOnReAdd(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)
	code := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(code, "main.go"), []byte("package main\n"), 0o644))
	codeBefore := snapshotDir(t, code)

	location := filepath.Join(project, "repos", "api")
	require.NoDirExists(t, location)

	stdout, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "api",
		"location": "../repos/api",
		"source":   "file://" + code,
	}))
	require.NoError(t, err)
	var result repoAddResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.True(t, result.Registered)
	require.Equal(t, "created", result.Footprint)

	// The footprint lives at the location, which add created.
	require.ElementsMatch(t, minimalFootprint, listPaths(t, location))
	repoYAML := filepath.Join(location, ".spektacular", "repo.yaml")
	raw, err := os.ReadFile(repoYAML)
	require.NoError(t, err)
	require.Contains(t, string(raw), "source:\n    provider: file\n    config:\n        location: "+code+"\n")

	// The registry carries the relative location as written.
	cfg, err := config.FromYAMLFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 2)
	require.Equal(t, filepath.Join("../repos/api", ".spektacular"), cfg.Repos[1].Location)

	// A re-add without a source, carrying only a new description, keeps the
	// stored source and records the description.
	_, _, err = runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":        "api",
		"location":    "../repos/api",
		"description": "the API service",
	}))
	require.NoError(t, err)
	raw, err = os.ReadFile(repoYAML)
	require.NoError(t, err)
	require.Contains(t, string(raw), "source:\n    provider: file\n    config:\n        location: "+code+"\n", "an add without source must leave the stored source alone")
	require.Contains(t, string(raw), "description: the API service\n")

	// The code directory is untouched throughout and gained no footprint.
	require.Equal(t, codeBefore, snapshotDir(t, code))
	require.NoDirExists(t, filepath.Join(code, ".spektacular"))
	require.Zero(t, git.calls, "a file source never invokes git")
}

// Phase 1.4 criterion 1: `repo add` with a git source clones it exactly once
// into the project's working folder <root>/.spektacular/repos/<name>, and a
// following `repo list` reports that clone as the repo's root, materialized,
// without cloning again.
func TestRepoAdd_GitSourceClonesIntoProjectReposFolderOnce(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{localHead: "abc", remoteHead: "abc"}
	swapRepoGit(t, git)

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "api",
		"location": "../repos/api",
		"source":   "https://example.com/api.git",
	}))
	require.NoError(t, err)

	clone := filepath.Join(project, ".spektacular", "repos", "api")
	require.Equal(t, []stubClone{{url: "https://example.com/api.git", dir: clone}}, git.clones,
		"add must clone the git source exactly once, into the project's repos folder")
	require.DirExists(t, clone)

	// The footprint sits at the location, and the clone is never footprinted.
	require.ElementsMatch(t, minimalFootprint, listPaths(t, filepath.Join(project, "repos", "api")))
	require.NoDirExists(t, filepath.Join(clone, ".spektacular"))

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)
	var listed struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	require.Len(t, listed.Repos, 2)
	require.Equal(t, "api", listed.Repos[1].Name)
	require.Equal(t, filepath.Join(project, "repos", "api", ".spektacular"), listed.Repos[1].Location)
	require.Equal(t, clone, listed.Repos[1].Root, "the listed root is the clone of the git source")
	require.True(t, listed.Repos[1].Materialized)
	require.Empty(t, listed.Repos[0].StaleNote, "matching heads carry no stale note")
	require.Len(t, git.clones, 1, "listing must not clone again")
}

// Phase 1.4 criterion 3: `repo list` reports the repo's resolved source as
// its root, for every file-source spelling — an absolute path, a path
// relative to the directory holding repo.yaml, an environment-variable
// reference, and a file:// URL — without ever invoking git. The repo.yaml
// files are written by hand and the entries registered directly in the
// project config so nothing but listing runs.
func TestRepoList_RootIsResolvedFileSource(t *testing.T) {
	cases := []struct {
		name   string
		source func(project, code string) string
	}{
		{name: "absolute", source: func(_, code string) string { return code }},
		// repo.yaml sits at <project>/repos/api/.spektacular/repo.yaml, so
		// three levels up is the project root.
		{name: "relative", source: func(_, _ string) string { return "../../../code" }},
		{name: "env var", source: func(_, _ string) string { return "${SPEK_TEST_CODE_DIR}" }},
		{name: "file url", source: func(_, code string) string { return "file://" + code }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := repoProject(t)
			git := &stubGit{}
			swapRepoGit(t, git)
			code := filepath.Join(project, "code")
			require.NoError(t, os.MkdirAll(code, 0o755))
			t.Setenv("SPEK_TEST_CODE_DIR", code)

			writeSpecCommandConfig(t, project,
				"repos:\n"+
					"  - name: api\n"+
					"    location: ../repos/api/.spektacular\n")

			location := filepath.Join(project, "repos", "api")
			require.NoError(t, os.MkdirAll(filepath.Join(location, ".spektacular"), 0o755))
			repoCfg := config.NewDefaultRepoConfig()
			src, err := config.SourceFromInput(tc.source(project, code))
			require.NoError(t, err)
			repoCfg.Source = src
			repoCfg.Description = "the API service"
			require.NoError(t, repoCfg.ToYAMLFile(
				filepath.Join(location, ".spektacular", config.RepoConfigFileName)))

			stdout, _, err := runRepo(t, "list")
			require.NoError(t, err)
			var listed struct {
				Repos []repoListEntry `json:"repos"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
			require.Len(t, listed.Repos, 1)

			api := listed.Repos[0]
			require.Equal(t, "api", api.Name)
			require.Equal(t, filepath.Join(project, "repos", "api", ".spektacular"), api.Location)
			require.Equal(t, code, api.Root, "root must be the resolved source directory")
			require.True(t, filepath.IsAbs(api.Root))
			require.False(t, api.Materialized, "a file source is not a project-managed clone")
			require.Equal(t, "the API service", api.Description, "metadata still comes from repo.yaml at the location")
			require.Zero(t, git.calls, "a file source never invokes git")
		})
	}
}

// Phase 1.4 criterion 3: a repo whose repo.yaml declares a git source that
// has not been cloned yet is listed with an empty root and materialized
// false — listing never clones — while its descriptive metadata is still
// read from repo.yaml at its location.
func TestRepoList_UnclonedGitSourceReportsNoRootWithoutCloning(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)

	writeSpecCommandConfig(t, project,
		"repos:\n"+
			"  - name: api\n"+
			"    location: ../repos/api/.spektacular\n")

	location := filepath.Join(project, "repos", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(location, ".spektacular"), 0o755))
	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Source = config.GitSource("https://example.com/api.git")
	repoCfg.Description = "the API service"
	require.NoError(t, repoCfg.ToYAMLFile(
		filepath.Join(location, ".spektacular", config.RepoConfigFileName)))

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)
	var listed struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	require.Len(t, listed.Repos, 1)

	api := listed.Repos[0]
	require.Equal(t, "api", api.Name)
	require.Equal(t, filepath.Join(project, "repos", "api", ".spektacular"), api.Location)
	require.Equal(t, "", api.Root, "an uncloned git source has no root yet")
	require.False(t, api.Materialized)
	require.Equal(t, "the API service", api.Description)
	require.Zero(t, git.calls, "listing must never invoke git or clone")
	require.NoDirExists(t, filepath.Join(project, ".spektacular", "repos", "api"))
}

// Phase 1.4 criterion 3: a repo whose repo.yaml declares no source at all
// still reports its location — as an absolute path — as its root, exactly as
// before sources existed.
func TestRepoList_NoSourceRootIsLocation(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)

	writeSpecCommandConfig(t, project,
		"repos:\n"+
			"  - name: api\n"+
			"    location: ../repos/api/.spektacular\n")

	location := filepath.Join(project, "repos", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(location, ".spektacular"), 0o755))
	require.NoError(t, config.NewDefaultRepoConfig().ToYAMLFile(
		filepath.Join(location, ".spektacular", config.RepoConfigFileName)))

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)
	var listed struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	require.Len(t, listed.Repos, 1)
	footprint := filepath.Join(location, ".spektacular")
	require.Equal(t, footprint, listed.Repos[0].Location)
	require.Equal(t, footprint, listed.Repos[0].Root, "with no source the root is the location itself")
	require.False(t, listed.Repos[0].Materialized)
	require.Zero(t, git.calls)
}

// `repo list` reports the provider each repo declares for its own source, so
// the field says how the code at `root` was reached: `file` for a directory,
// `git` for a clone, and nothing at all for a repo that declares no source
// and is therefore its own footprint folder.
func TestRepoList_ProviderIsTheRepoSourceProvider(t *testing.T) {
	project := repoProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)

	writeSpecCommandConfig(t, project,
		"repos:\n"+
			"  - name: filed\n"+
			"    location: ../repos/filed/.spektacular\n"+
			"  - name: cloned\n"+
			"    location: ../repos/cloned/.spektacular\n"+
			"  - name: bare\n"+
			"    location: ../repos/bare/.spektacular\n")

	write := func(name string, source config.RepoSourceConfig) {
		t.Helper()
		dir := filepath.Join(project, "repos", name, ".spektacular")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		cfg := config.NewDefaultRepoConfig()
		cfg.Source = source
		require.NoError(t, cfg.ToYAMLFile(filepath.Join(dir, config.RepoConfigFileName)))
	}
	write("filed", config.FileSource(".."))
	write("cloned", config.GitSource("https://example.com/cloned.git"))
	write("bare", config.RepoSourceConfig{})

	stdout, _, err := runRepo(t, "list")
	require.NoError(t, err)

	var listed struct {
		Repos []repoListEntry `json:"repos"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	require.Len(t, listed.Repos, 3)

	require.Equal(t, "filed", listed.Repos[0].Name)
	require.Equal(t, "file", listed.Repos[0].Provider, "a file source reports the file provider")

	require.Equal(t, "cloned", listed.Repos[1].Name)
	require.Equal(t, "git", listed.Repos[1].Provider, "a git source reports the git provider")

	require.Equal(t, "bare", listed.Repos[2].Name)
	require.Empty(t, listed.Repos[2].Provider, "a repo declaring no source reports no provider")

	require.Zero(t, git.calls, "listing must never invoke git")
}

// `repo add` rewrites config.yaml; the store folders it carries stay in their
// settings-relative form and are never expanded to project-root paths.
func TestRepoAdd_KeepsSettingsRelativeStoreDirectories(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	writeSpecCommandConfig(t, project, "spec:\n  config:\n    directory: specs\n")
	target := t.TempDir()

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "docs",
		"location": target,
	}))
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "directory: specs\n")
	require.NotContains(t, string(raw), ".spektacular/specs")
	require.Contains(t, string(raw), "name: docs\n", "the added repo must be registered")
}
