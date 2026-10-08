package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/identifier"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

type specCommandResult struct {
	Step        string `json:"step"`
	SpecPath    string `json:"spec_path"`
	SpecName    string `json:"spec_name"`
	Instruction string `json:"instruction"`
}

func writeSpecCommandConfig(t *testing.T, dir, body string) {
	t.Helper()
	dataDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	// A project config requires a slug-safe `name`; prepend one so fixture
	// bodies stay focused on the section each test exercises. A project must
	// also register at least one repo: unless the body declares its own
	// registry, register the project's own footprint folder (`.` counted
	// from config.yaml, so the .spektacular folder itself), the shape init
	// produces.
	body = "name: testproj\n" + body
	if !strings.Contains(body, "repos:") {
		body += "repos:\n  - name: testproj\n    location: .\n"
	}
	writeCurrentConfig(t, dir, body)
	repoConfigPath := filepath.Join(dataDir, config.RepoConfigFileName)
	if _, err := os.Stat(repoConfigPath); os.IsNotExist(err) {
		rc := config.NewDefaultRepoConfig()
		rc.Source = config.DefaultRepoSource
		require.NoError(t, rc.ToYAMLFile(repoConfigPath))
	}
}

func writeSpecCommandFile(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, ".spektacular", "specs", name+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o644))
}

func setSpecIdentifierNow(t *testing.T, now time.Time) {
	t.Helper()
	original := specIdentifierNow
	specIdentifierNow = func() time.Time { return now }
	t.Cleanup(func() {
		specIdentifierNow = original
	})
}

func setSpecIdentifierRandomID(t *testing.T, id string) {
	t.Helper()
	original := specIdentifierRandomID
	specIdentifierRandomID = func() (string, error) { return id, nil }
	t.Cleanup(func() {
		specIdentifierRandomID = original
	})
}

func runSpecNewForTest(t *testing.T, args ...string) (specCommandResult, error) {
	t.Helper()
	resetRootCmd(t)
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs(append([]string{"spec", "new"}, args...))

	err := rootCmd.Execute()
	if err != nil {
		return specCommandResult{}, err
	}

	var result specCommandResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	return result, nil
}

func runSpecNewSchemaForTest(t *testing.T) commandSchema {
	t.Helper()
	resetRootCmd(t)
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "new", "--schema"})

	require.NoError(t, rootCmd.Execute())

	var schema commandSchema
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &schema))
	return schema
}

func TestSpecNewSchemaDocumentsNameAndOptionalID(t *testing.T) {
	t.Chdir(t.TempDir())
	schema := runSpecNewSchemaForTest(t)

	require.Contains(t, schema.Input.Properties, "name")
	require.Contains(t, schema.Input.Properties, "id")
	require.Equal(t, []string{"name"}, schema.Input.Required)
	require.Equal(t, identifier.MaxPartLength, schema.Input.Properties["name"].MaxLen)
	require.Equal(t, identifier.MaxPartLength, schema.Input.Properties["id"].MaxLen)

	require.Contains(t, schema.Input.Properties, "sources")
	require.Equal(t, "array", schema.Input.Properties["sources"].Type)
	require.Contains(t, schema.Input.Properties["sources"].Items.Properties, "uri")
	require.Contains(t, schema.Input.Properties, "epic")
	require.Equal(t, "string", schema.Input.Properties["epic"].Type)
}

func TestSpecNew_DefaultUsesTimestampPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	result, err := runSpecNewForTest(t, "--data", `{"name":"Billing.Export"}`)
	require.NoError(t, err)

	require.Equal(t, "new", result.Step)
	require.Regexp(t, regexp.MustCompile(`^\d{14}-[0-9a-f]{8}-billing-export$`), result.SpecName)
	require.Equal(t, "specs/"+result.SpecName+".md", result.SpecPath,
		"spec_path is relative to the folder holding config.yaml")
	require.FileExists(t, filepath.Join(dir, ".spektacular", result.SpecPath))
}

func TestSpecNew_TimestampCollisionBumpsSeconds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	setSpecIdentifierNow(t, time.Date(2026, time.May, 9, 1, 2, 3, 0, time.UTC))
	setSpecIdentifierRandomID(t, "a1b2c3d4")
	writeSpecCommandFile(t, dir, "20260509010203-a1b2c3d4-billing-export")

	result, err := runSpecNewForTest(t, "--data", `{"name":"billing-export"}`)
	require.NoError(t, err)

	require.Equal(t, "20260509010204-a1b2c3d4-billing-export", result.SpecName)
	require.Equal(t, "specs/20260509010204-a1b2c3d4-billing-export.md", result.SpecPath)
	require.FileExists(t, filepath.Join(dir, ".spektacular", "specs", "20260509010203-a1b2c3d4-billing-export.md"))
	require.FileExists(t, filepath.Join(dir, ".spektacular", "specs", "20260509010204-a1b2c3d4-billing-export.md"))
}

func TestSpecNew_ExplicitIDUnderCounterModeRejectedWithoutSideEffects(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")

	_, err := runSpecNewForTest(t, "--data", `{"name":"Billing Export","id":"36"}`)

	var cliErr *output.ErrorResponse
	require.ErrorAs(t, err, &cliErr)
	require.Equal(t, "id_not_allowed", cliErr.Code)
	require.Contains(t, cliErr.NextAction, `without "id"`)
	require.NoDirExists(t, filepath.Join(dataDir, "specs"))
	require.NoFileExists(t, filepath.Join(dataDir, "state.json"))
}

func TestSpecNew_ExternalModeWithIDCreatesSpec(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: external\n")

	result, err := runSpecNewForTest(t, "--data", `{"name":"Billing Export","id":"EXT.User@123"}`)
	require.NoError(t, err)

	require.Equal(t, "ext-user-123-billing-export", result.SpecName)
	require.Equal(t, "specs/ext-user-123-billing-export.md", result.SpecPath)
	require.FileExists(t, filepath.Join(dir, ".spektacular", "specs", "ext-user-123-billing-export.md"))
}

func TestSpecNew_ExternalModeRequiresIDWithoutSideEffects(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: external\n")

	_, err := runSpecNewForTest(t, "--data", `{"name":"Billing Export"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "id is required")
	require.NoDirExists(t, filepath.Join(dataDir, "specs"))
	require.NoFileExists(t, filepath.Join(dataDir, "state.json"))
}

func TestSpecNew_CounterModeUsesNextValueFromStore(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")
	writeSpecCommandFile(t, dir, "000007_old-feature")

	result, err := runSpecNewForTest(t, "--data", `{"name":"billing-export"}`)
	require.NoError(t, err)

	require.Equal(t, "000008_billing-export", result.SpecName)
	require.Equal(t, "specs/000008_billing-export.md", result.SpecPath)
	require.FileExists(t, filepath.Join(dir, ".spektacular", "specs", "000008_billing-export.md"))
}

func TestSpecNew_CounterModeCollisionBumpsValue(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")
	writeSpecCommandFile(t, dir, "000007_old-feature")
	writeSpecCommandFile(t, dir, "000008_billing-export")

	result, err := runSpecNewForTest(t, "--data", `{"name":"billing-export"}`)
	require.NoError(t, err)

	require.Equal(t, "000009_billing-export", result.SpecName)
	require.Equal(t, "specs/000009_billing-export.md", result.SpecPath)
	require.FileExists(t, filepath.Join(dir, ".spektacular", "specs", "000009_billing-export.md"))
}

func TestSpecNew_DryRunReportsCanonicalNameWithoutWrites(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")
	writeSpecCommandFile(t, dir, "000007_old-feature")

	result, err := runSpecNewForTest(t, "--dry-run", "--data", `{"name":"billing-export"}`)
	require.NoError(t, err)

	require.Equal(t, "000008_billing-export", result.SpecName)
	require.Equal(t, "specs/000008_billing-export.md", result.SpecPath)
	require.NoFileExists(t, filepath.Join(dataDir, "specs", "000008_billing-export.md"))
	require.NoFileExists(t, filepath.Join(dataDir, "state.json"))
}

func TestSpecNew_ValidationFailuresLeaveNoSpecOrState(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "untrimmed name",
			data: `{"name":" billing"}`,
			want: "leading or trailing whitespace",
		},
		{
			name: "path separator id",
			data: `{"name":"billing","id":"bad/id"}`,
			want: "path separators",
		},
		{
			name: "untrimmed id",
			data: `{"name":"billing","id":"ext "}`,
			want: "leading or trailing whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			dataDir := filepath.Join(dir, ".spektacular")
			writeSpecCommandConfig(t, dir, "")

			_, err := runSpecNewForTest(t, "--data", tt.data)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
			require.NoDirExists(t, filepath.Join(dataDir, "specs"))
			require.NoFileExists(t, filepath.Join(dataDir, "state.json"))
		})
	}
}

func TestSpecNew_RejectsUnknownConfiguredIDMethod(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeCurrentConfig(t, dir, "name: testproj\nspec:\n  id_method: unsupported\n")

	resetRootCmd(t)
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "new", "--data", `{"name":"fixture"}`})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec.id_method")
	require.NoFileExists(t, filepath.Join(dataDir, "specs", "fixture.md"))
	require.NoFileExists(t, filepath.Join(dataDir, "state.json"))
}

// fixedResumeTime is a deterministic timestamp for seeded in-progress state so
// byte-for-byte oracles never depend on time.Now().
var fixedResumeTime = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

func TestSpecNew_InProgressReturnsWorkflowInProgressErrorAndPreservesState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})

	before, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"whatever"}`)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "000024_resume", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "overview", er.State.Current)
	require.NotEmpty(t, er.NextAction)

	after, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)

	require.NoDirExists(t, filepath.Join(dataDir, "specs"))
}

// TestSpecNew_InProgressNoDataReturnsWorkflowInProgressError asserts that the
// in-progress check runs before the name is required: `spec new` with no
// --data still fails with the shared workflow_in_progress error (rather than
// erroring on the missing name), so the driving agent can offer resume
// without first prompting for a spec name.
func TestSpecNew_InProgressNoDataReturnsWorkflowInProgressError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})

	before, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "spec", "new")
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "000024_resume", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "overview", er.State.Current)

	after, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestSpecNew_ForceStartsFreshOverInProgress(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})

	resetRootCmd(t)
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "new", "--force", "--data", `{"name":"billing"}`})
	require.NoError(t, rootCmd.Execute())

	var result specCommandResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Equal(t, "new", result.Step)
	require.Equal(t, "specs/"+result.SpecName+".md", result.SpecPath)
	require.FileExists(t, filepath.Join(dataDir, "specs", result.SpecName+".md"))
}

// TestSpecNew_CleanDirSucceedsWithoutError asserts that `spec new` in a clean
// directory (no in-progress state) succeeds normally through the standard
// success envelope, rather than being reported as an in-progress workflow.
func TestSpecNew_CleanDirSucceedsWithoutError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &m))
	require.Equal(t, false, m["error"])
}

func TestSpecNew_KindlessInProgressStateErrorsWithoutClobber(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")

	writeInProgressState(t, dataDir, workflow.State{
		CurrentStep:    "overview",
		CompletedSteps: []string{"new"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "legacy"},
	})

	before, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)

	resetRootCmd(t)
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "new", "--data", `{"name":"whatever"}`})
	err = rootCmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "kind marker")

	after, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// Criterion 3 (spec): spec.config.directory is relative to the folder
// holding config.yaml, so `directory: x` stores new specs in .spektacular/x.
func TestSpecNew_CustomDirectoryResolvesFromSettingsFolder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: x\n")

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"n"}`)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	var result specCommandResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "x/"+result.SpecName+".md", result.SpecPath)
	require.FileExists(t, filepath.Join(dir, ".spektacular", "x", result.SpecName+".md"))
	require.NoDirExists(t, filepath.Join(dir, "x"))
}

// A store folder that resolves outside the project is refused with
// config_invalid, and the next action names the key and a valid example.
func TestSpecNew_DirectoryOutsideProjectIsRefused(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../../elsewhere\n")

	er := runRootError(t, "spec", "new", "--data", `{"name":"n"}`)
	require.Equal(t, "config_invalid", er.Code)
	require.Contains(t, er.Message, "spec.config.directory")
	require.Contains(t, er.NextAction, "`spec.config.directory`")
	require.Contains(t, er.NextAction, "`specs`")
	require.NoDirExists(t, filepath.Join(filepath.Dir(dir), "elsewhere"))
}

// specFrontmatter returns the stored spec's frontmatter as one string.
func specFrontmatter(t *testing.T, root, name string) string {
	t.Helper()
	return strings.Join(frontmatterOf(t, filepath.Join(root, ".spektacular", "specs", name+".md")), "\n")
}

// A spec started with sources records each link with today's retrieval date;
// one started without records none.
func TestSpecNew_Sources(t *testing.T) {
	t.Run("each source is recorded with today's date", func(t *testing.T) {
		root := epicProject(t)
		result, err := runSpecNewForTest(t, "--data", `{"name":"seeded","sources":[{"uri":"https://example.com/issues/45"},{"uri":"https://example.com/doc"}]}`)
		require.NoError(t, err)
		fm := specFrontmatter(t, root, result.SpecName)
		today := time.Now().UTC().Format("2006-01-02")
		require.Contains(t, fm, "uri: https://example.com/issues/45\n      retrieved_date: \""+today+"\"")
		require.Contains(t, fm, "uri: https://example.com/doc\n      retrieved_date: \""+today+"\"")
	})

	t.Run("a spec started without sources records none", func(t *testing.T) {
		root := epicProject(t)
		result, err := runSpecNewForTest(t, "--data", `{"name":"plain","sources":[]}`)
		require.NoError(t, err)
		require.NotContains(t, specFrontmatter(t, root, result.SpecName), "sources:")
	})

	t.Run("a source with no uri is refused and nothing is written", func(t *testing.T) {
		root := epicProject(t)
		before := snapshotTree(t, root)
		_, err := runSpecNewForTest(t, "--data", `{"name":"seeded","sources":[{"uri":"https://example.com/a"},{}]}`)
		var cliErr *output.ErrorResponse
		require.ErrorAs(t, err, &cliErr)
		require.Equal(t, "sources_invalid", cliErr.Code)
		require.Equal(t, "sources[1]", cliErr.Resource)
		require.Contains(t, cliErr.NextAction, `"sources":[{"uri":"https://`)
		require.Equal(t, before, snapshotTree(t, root))
	})
}

// A spec started in an epic is listed by it from the start, with no
// dependencies, and names it; an unknown epic is refused with nothing written.
func TestSpecNew_InEpic(t *testing.T) {
	t.Run("the epic lists the spec and the spec names the epic", func(t *testing.T) {
		root := epicProject(t)
		writeSpecFixture(t, root, "000001_existing", epicTestSpecFixed)
		epicWrite(t, testEpic, specsData("000001_existing"))

		result, err := runSpecNewForTest(t, "--data", `{"name":"joiner","epic":"`+testEpic+`"}`)
		require.NoError(t, err)
		require.Equal(t, "new", result.Step)

		require.Equal(t, []string{"000001_existing", result.SpecName}, epicSpecsOf(t, epicFilePath(root, testEpic)))
		require.Contains(t, strings.Join(frontmatterOf(t, epicFilePath(root, testEpic)), "\n"),
			"- name: "+result.SpecName+"\n      depends_on: []")
		require.Equal(t, testEpic, specEpicOf(t, filepath.Join(root, ".spektacular", "specs", result.SpecName+".md")))
		requireAgreement(t, root, testEpic, "000001_existing", result.SpecName)
		require.FileExists(t, filepath.Join(root, ".spektacular", "state.json"))
	})

	t.Run("an unknown epic is refused and nothing is written", func(t *testing.T) {
		root := epicProject(t)
		before := snapshotTree(t, root)
		_, err := runSpecNewForTest(t, "--data", `{"name":"joiner","epic":"000099_missing"}`)
		var cliErr *output.ErrorResponse
		require.ErrorAs(t, err, &cliErr)
		require.Equal(t, "epic_not_found", cliErr.Code)
		require.Contains(t, cliErr.NextAction, "epic list")
		require.Equal(t, before, snapshotTree(t, root))
		require.NoDirExists(t, filepath.Join(root, ".spektacular", "specs"))
		require.NoFileExists(t, filepath.Join(root, ".spektacular", "state.json"))
	})

	t.Run("a failed join removes the new spec and restores the epic", func(t *testing.T) {
		root := epicProject(t)
		epicWrite(t, testEpic, "")
		epicBefore, err := os.ReadFile(epicFilePath(root, testEpic))
		require.NoError(t, err)
		failEpicLinkOnCall(t, 1, nil)

		resetRootCmd(t)
		stdout, _ := setupImplementCmd(t)
		rootCmd.SetArgs([]string{"spec", "new", "--data", `{"name":"joiner","epic":"` + testEpic + `"}`})
		err = rootCmd.Execute()
		var cliErr *output.ErrorResponse
		require.ErrorAs(t, err, &cliErr)
		require.Equal(t, "epic_link_failed", cliErr.Code)
		require.Empty(t, stdout.String(), "the new step's result is not reported for a spec that was removed")

		epicAfter, err := os.ReadFile(epicFilePath(root, testEpic))
		require.NoError(t, err)
		require.Equal(t, string(epicBefore), string(epicAfter))
		entries, _ := os.ReadDir(filepath.Join(root, ".spektacular", "specs"))
		require.Empty(t, entries)
		require.NoFileExists(t, filepath.Join(root, ".spektacular", "state.json"))
	})
}

// Without sources or an epic, spec new writes a spec carrying neither key.
func TestSpecNew_WithoutSourcesOrEpicIsUnchanged(t *testing.T) {
	root := epicProject(t)
	result, err := runSpecNewForTest(t, "--data", `{"name":"plain"}`)
	require.NoError(t, err)
	require.Equal(t, "new", result.Step)
	fm := specFrontmatter(t, root, result.SpecName)
	require.NotContains(t, fm, "sources:")
	require.NotContains(t, fm, "epic:")
	require.NoDirExists(t, filepath.Join(root, ".spektacular", "epics"))
}
