package repo

import (
	"os"
	"path/filepath"
	"testing"

	repodomain "github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

type testData struct {
	values map[string]any
}

func (d *testData) Get(key string) (any, bool) {
	v, ok := d.values[key]
	return v, ok
}

func (d *testData) Set(key string, value any) {
	d.values[key] = value
}

type captureWriter struct {
	result Result
}

func (c *captureWriter) WriteResult(v any) error {
	c.result = v.(Result)
	return nil
}

// renderStep drives one guided-add step callback against the supplied
// workflow data and returns the instruction it rendered.
func renderStep(t *testing.T, cb workflow.StepCallback, data *testData) string {
	t.Helper()
	writer := &captureWriter{}
	st := store.NewFileStore(t.TempDir(), "project")
	_, err := cb(data, writer, st, workflow.Config{Command: "spektacular"})
	require.NoError(t, err)
	return writer.result.Instruction
}

// The terms the seeded fixture repo states about itself. Every one of these
// is hand-written to match seedFixtureRepo's files exactly; none is read back
// out of the examiner at run time.
const (
	fixtureIdentity  = "harbour"
	fixtureManifest  = "package.json"
	fixtureSummary   = "Tide-gate scheduling for dockside cranes"
	fixtureReadme    = "Harbour schedules tide-gate crossings for dockside cranes."
	fixtureLanguage  = "TypeScript"
	nothingToPropose = "so you have nothing to propose"
)

// seedFixtureRepo writes a repo that states something about itself in every
// channel the examination reads: a README with opening prose, a manifest
// declaring both a name and a description, and a source file that fixes the
// language. It returns the folder.
func seedFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "README.md"),
		[]byte("# Harbour\n\nHarbour schedules tide-gate crossings for dockside cranes.\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "package.json"),
		[]byte("{\n  \"name\": \"harbour\",\n  \"description\": \"Tide-gate scheduling for dockside cranes\"\n}\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "index.ts"),
		[]byte("export const berths = 4;\n"),
		0o644,
	))

	return dir
}

// fixtureData returns workflow data pointing the flow at the given folder,
// with no evidence cached yet.
func fixtureData(dir string) *testData {
	return &testData{values: map[string]any{
		"name":     "harbour",
		"location": dir,
	}}
}

// requireStatesFindings asserts an instruction took the readable branch and
// carried every term the fixture repo states about itself.
func requireStatesFindings(t *testing.T, instruction, step string) {
	t.Helper()
	require.Contains(t, instruction, fixtureIdentity, "%s must state the repo's declared identity", step)
	require.Contains(t, instruction, fixtureManifest, "%s must name the manifest the identity came from", step)
	require.Contains(t, instruction, fixtureSummary, "%s must state the repo's declared summary", step)
	require.Contains(t, instruction, fixtureReadme, "%s must state the README's opening prose", step)
	require.Contains(t, instruction, fixtureLanguage, "%s must state the language the repo is written in", step)
	require.NotContains(t, instruction, nothingToPropose, "%s must not take the nothing-to-propose branch", step)
	require.NotContains(t, instruction, "{{", "%s must leave no unrendered mustache", step)
}

// TestGuidedAdd_NameDescriptionRoleAndTagsAllStateTheFindings renders each of
// the four proposing steps against a freshly seeded fixture repo and asserts
// every one of them has the repo's own statements about itself available.
func TestGuidedAdd_NameDescriptionRoleAndTagsAllStateTheFindings(t *testing.T) {
	dir := seedFixtureRepo(t)

	steps := []struct {
		name string
		cb   workflow.StepCallback
	}{
		{"name", name()},
		{"description", description()},
		{"role", role()},
		{"tags", tags()},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			out := renderStep(t, step.cb, fixtureData(dir))
			requireStatesFindings(t, out, step.name)
		})
	}
}

// TestGuidedAdd_ExaminesTheRepoOnceAndReusesTheResult renders the name step
// against the fixture, then deletes the README and the manifest from disk
// before rendering description, role and tags from the same workflow data.
// The later steps must still state what those deleted files said: if the flow
// re-examined the repo they would no longer have those findings to state.
func TestGuidedAdd_ExaminesTheRepoOnceAndReusesTheResult(t *testing.T) {
	dir := seedFixtureRepo(t)
	data := fixtureData(dir)

	first := renderStep(t, name(), data)
	requireStatesFindings(t, first, "name")

	// Take away everything the README and manifest contributed. A second
	// examination could no longer report an identity, a summary or README
	// prose from this folder.
	require.NoError(t, os.Remove(filepath.Join(dir, "README.md")))
	require.NoError(t, os.Remove(filepath.Join(dir, "package.json")))

	later := []struct {
		name string
		cb   workflow.StepCallback
	}{
		{"description", description()},
		{"role", role()},
		{"tags", tags()},
	}

	for _, step := range later {
		t.Run(step.name, func(t *testing.T) {
			out := renderStep(t, step.cb, data)
			require.Contains(t, out, fixtureIdentity, "%s must reuse the identity gathered before the files were deleted", step.name)
			require.Contains(t, out, fixtureManifest, "%s must reuse the manifest gathered before the files were deleted", step.name)
			require.Contains(t, out, fixtureSummary, "%s must reuse the summary gathered before the files were deleted", step.name)
			require.Contains(t, out, fixtureReadme, "%s must reuse the README prose gathered before the files were deleted", step.name)
		})
	}
}

// TestGuidedAdd_RepoStatingNothingRendersTheNothingToProposeBranch points the
// flow at a bare empty folder. Such a repo is an ordinary case rather than a
// failure: every proposing step must render and take the branch that asks
// rather than proposes.
func TestGuidedAdd_RepoStatingNothingRendersTheNothingToProposeBranch(t *testing.T) {
	dir := t.TempDir()

	steps := []struct {
		name string
		cb   workflow.StepCallback
	}{
		{"name", name()},
		{"description", description()},
		{"role", role()},
		{"tags", tags()},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			out := renderStep(t, step.cb, fixtureData(dir))
			require.Contains(t, out, nothingToPropose, "%s must take the nothing-to-propose branch", step.name)
			require.NotContains(t, out, "{{", "%s must leave no unrendered mustache", step.name)
		})
	}
}

// TestPlacementStep_WritableTargetRendersTheDefaultHoldsBranch asserts that
// when the project's files could be created inside the target repo, placement
// tells the agent to record the default and advance without asking.
func TestPlacementStep_WritableTargetRendersTheDefaultHoldsBranch(t *testing.T) {
	out := renderStep(t, placement(), fixtureData(t.TempDir()))

	require.Contains(t, out, "The default holds: those files go inside the repo being added.",
		"a writable target must render the default-holds branch")
	require.NotContains(t, out, "the default is not available and you do have to ask",
		"a writable target must not render the ask branch")
	require.NotContains(t, out, "{{", "placement must leave no unrendered mustache")
}

// TestPlacementStep_ReadOnlyTargetRendersTheAskBranch asserts that when the
// project's files could not be created inside the target repo, placement asks
// instead of assuming the default.
func TestPlacementStep_ReadOnlyTargetRendersTheAskBranch(t *testing.T) {
	dir := readOnlyDir(t)

	out := renderStep(t, placement(), fixtureData(dir))

	require.Contains(t, out, "the default is not available and you do have to ask",
		"a read-only target must render the ask branch")
	require.NotContains(t, out, "The default holds: those files go inside the repo being added.",
		"a read-only target must not render the default-holds branch")
	require.NotContains(t, out, "{{", "placement must leave no unrendered mustache")
}

// readOnlyDir returns a directory nothing can be created inside. The test is
// skipped for root, which bypasses the permission bits and would make the
// probe succeed regardless of the mode. The mode is restored on cleanup so
// t.TempDir's own removal can still descend into it.
func readOnlyDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission bits, so a read-only directory cannot be simulated")
	}

	dir := filepath.Join(t.TempDir(), "locked")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	return dir
}

// TestWritable_ReportsWhetherAnEntryCouldBeCreated covers the probe directly
// across the cases the placement step depends on it separating.
func TestWritable_ReportsWhetherAnEntryCouldBeCreated(t *testing.T) {
	t.Run("fresh temp dir is writable", func(t *testing.T) {
		require.True(t, writable(t.TempDir()))
	})

	t.Run("empty path is not writable", func(t *testing.T) {
		require.False(t, writable(""))
	})

	t.Run("nonexistent path is not writable", func(t *testing.T) {
		require.False(t, writable(filepath.Join(t.TempDir(), "absent")))
	})

	t.Run("a file is not writable", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "notadir.txt")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
		require.False(t, writable(file), "a regular file is not somewhere a folder can be created")
	})

	t.Run("read-only dir is not writable", func(t *testing.T) {
		require.False(t, writable(readOnlyDir(t)))
	})
}

// TestWritable_LeavesNothingBehind asserts the probe is transient: a
// successful probe must leave the target folder exactly as it found it.
func TestWritable_LeavesNothingBehind(t *testing.T) {
	dir := seedFixtureRepo(t)

	before, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, before, 3, "the fixture seeds a README, a manifest and one source file")

	require.True(t, writable(dir))

	after, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, after, 3, "the probe must remove whatever it created")
}

// TestEvidenceVars_RendersTheLowercaseKeySet asserts the rendered map carries
// exactly the seven lowercase keys the templates branch on, so an instruction
// renders identically whether the map was just built or read back out of the
// JSON-persisted workflow data.
func TestEvidenceVars_RendersTheLowercaseKeySet(t *testing.T) {
	vars := evidenceVars(repodomain.Evidence{
		Readable:  true,
		Readme:    "opening prose",
		Manifest:  "package.json",
		Identity:  "harbour",
		Summary:   "a summary",
		TopLevel:  []string{"src/"},
		Languages: []string{"TypeScript"},
	})

	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	require.ElementsMatch(t, []string{
		"readable",
		"readme",
		"manifest",
		"identity",
		"summary",
		"top_level",
		"languages",
		"top_level_line",
		"languages_line",
	}, keys)

	require.Equal(t, true, vars["readable"])
	require.Equal(t, "opening prose", vars["readme"])
	require.Equal(t, "package.json", vars["manifest"])
	require.Equal(t, "harbour", vars["identity"])
	require.Equal(t, "a summary", vars["summary"])
	require.Equal(t, []string{"src/"}, vars["top_level"])
	require.Equal(t, []string{"TypeScript"}, vars["languages"])
	require.Equal(t, "src/", vars["top_level_line"])
	require.Equal(t, "TypeScript", vars["languages_line"])
}

// TestEvidenceVars_ZeroEvidenceIsNotReadable asserts a repo that stated
// nothing renders as not readable, which is what sends every proposing
// template down its ask branch.
func TestEvidenceVars_ZeroEvidenceIsNotReadable(t *testing.T) {
	vars := evidenceVars(repodomain.Evidence{})
	require.Equal(t, false, vars["readable"])
}

// TestEvidenceExtra_CachesTheRenderedMapNotTheStruct asserts the value left in
// workflow data is the already-rendered map. Workflow data round-trips through
// JSON, so caching the Evidence struct would make these instructions render
// one way in the session that gathered the evidence and another after a
// resume.
func TestEvidenceExtra_CachesTheRenderedMapNotTheStruct(t *testing.T) {
	data := fixtureData(seedFixtureRepo(t))

	extra := evidenceExtra(data)
	require.Contains(t, extra, "evidence")

	cached, ok := data.Get("evidence")
	require.True(t, ok, "evidenceExtra must cache its examination in workflow data")
	require.IsType(t, map[string]any{}, cached,
		"the cache must hold the rendered map, not a value that JSON round-trips into a different shape")
	_, isStruct := cached.(repodomain.Evidence)
	require.False(t, isStruct,
		"caching the Evidence struct would render differently before and after a resume")

	vars := cached.(map[string]any)
	require.Equal(t, true, vars["readable"])
	require.Equal(t, fixtureIdentity, vars["identity"])
	require.Equal(t, fixtureManifest, vars["manifest"])
}

// TestEvidenceExtra_ReturnsAPreSeededCacheWithoutExamining seeds workflow data
// with a cache whose identity could not have come from the folder on disk, and
// asserts that identity is what reaches the rendered instruction. A resumed
// add reads its cache back this way, and must not re-read the repo.
func TestEvidenceExtra_ReturnsAPreSeededCacheWithoutExamining(t *testing.T) {
	const sentinel = "resumed-from-state-not-from-disk"

	data := fixtureData(seedFixtureRepo(t))
	seeded := map[string]any{
		"readable":  true,
		"readme":    "",
		"manifest":  "",
		"identity":  sentinel,
		"summary":   "",
		"top_level": []string{},
		"languages": []string{},
	}
	data.Set("evidence", seeded)

	extra := evidenceExtra(data)
	require.Equal(t, map[string]any{"evidence": seeded}, extra,
		"a cached map must be handed back unchanged")

	cached, ok := data.Get("evidence")
	require.True(t, ok)
	require.Equal(t, seeded, cached, "the examiner must not overwrite a cache that is already present")

	out := renderStep(t, name(), data)
	require.Contains(t, out, sentinel, "the name step must state the cached identity")
	require.NotContains(t, out, fixtureSummary, "the repo on disk must not have been examined")
	require.NotContains(t, out, fixtureReadme, "the repo on disk must not have been examined")
}
