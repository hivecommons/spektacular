package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const repo1to2Desc = "record format version"
const project1to2Desc = "split legacy single-file settings and record installed skills version"
const project2to3Desc = "resolve spec, plan and changelog folders from the settings file"

func applyOpts(root string) Options {
	return Options{ProjectRoot: root, BinaryVersion: testVersion}
}

// --- Inspect -----------------------------------------------------------------

func TestInspect_UnversionedProjectReportsPendingSteps(t *testing.T) {
	root := newProject(t, "split_unversioned")

	rep, err := Inspect(root, testVersion)
	require.NoError(t, err)

	require.Equal(t, "upgrade_needed", rep.Status)
	require.True(t, rep.DryRun)
	require.True(t, rep.Pending())
	require.Equal(t, []FileReport{
		{Path: cfgPath(root, "config.yaml"), Kind: KindProject, From: 1, To: 3, Steps: []string{project1to2Desc, project2to3Desc}, Actions: []Action{
			{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "spec.config.directory", To: "specs"},
			{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "plan.config.directory", To: "plans"},
			{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "changelog.config.directory", To: "changelog"},
		}},
		{Path: cfgPath(root, "repo.yaml"), Kind: KindRepo, From: 1, To: 2, Steps: []string{repo1to2Desc}, Actions: []Action{}},
	}, rep.Files)
	require.Empty(t, rep.Skipped)
	require.Equal(t, SkillsReport{Installed: "0.1.0", Current: "0.1.0", Status: "match", Agent: "claude"}, rep.Skills)
}

func TestInspect_LegacySingleFileReportsActions(t *testing.T) {
	root := newProject(t, "legacy_single")

	rep, err := Inspect(root, testVersion)
	require.NoError(t, err)

	require.Equal(t, "upgrade_needed", rep.Status)
	require.Len(t, rep.Files, 1, "the created repo.yaml is already current and not reported separately")
	fr := rep.Files[0]
	require.Equal(t, cfgPath(root, "config.yaml"), fr.Path)
	require.Equal(t, 1, fr.From)
	require.Equal(t, 3, fr.To)
	require.Equal(t, []string{project1to2Desc, project2to3Desc}, fr.Steps)
	require.Empty(t, fr.Backup)

	require.Len(t, fr.Actions, 7)
	require.Equal(t, "create", fr.Actions[0].Op)
	require.Equal(t, cfgPath(root, "repo.yaml"), fr.Actions[0].Path)
	require.Equal(t, golden(t, "legacy_single", "repo.yaml"), string(fr.Actions[0].Content))
	require.Equal(t, Action{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "repos", To: "[{name: legacy, location: .}]"}, fr.Actions[1])
	require.Equal(t, Action{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "skills_version", To: "0.0.9"}, fr.Actions[2])
	require.Equal(t, Action{Op: "remove", Path: cfgPath(root, "version")}, fr.Actions[3])
	require.Equal(t, Action{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "spec.config.directory", To: "specs"}, fr.Actions[4])
	require.Equal(t, Action{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "plan.config.directory", To: "plans"}, fr.Actions[5])
	require.Equal(t, Action{Op: "set", Path: cfgPath(root, "config.yaml"), Key: "changelog.config.directory", To: "changelog"}, fr.Actions[6])

	require.Equal(t, SkillsReport{Installed: "0.0.9", Current: "0.1.0", Status: "mismatch", Agent: "claude", Reinstall: true}, rep.Skills)
}

func TestInspect_CurrentProjectHasNothingPending(t *testing.T) {
	root := newProject(t, "current")

	rep, err := Inspect(root, testVersion)
	require.NoError(t, err)

	require.Equal(t, "up_to_date", rep.Status)
	require.Empty(t, rep.Files)
	require.Empty(t, rep.Skipped)
	require.Equal(t, "match", rep.Skills.Status)
	require.False(t, rep.Skills.Reinstall)
	require.False(t, rep.Pending())
}

func TestInspect_NoProjectIsUpToDate(t *testing.T) {
	rep, err := Inspect(t.TempDir(), testVersion)
	require.NoError(t, err)
	require.Equal(t, "up_to_date", rep.Status)
	require.Equal(t, "missing", rep.Skills.Status)
}

// --- Dry run -------------------------------------------------------------------

func TestApply_DryRunChangesNothing(t *testing.T) {
	for _, fixture := range []string{"legacy_single", "split_unversioned"} {
		t.Run(fixture, func(t *testing.T) {
			root := newProject(t, fixture)
			before := snapshotDir(t, root)

			opts := applyOpts(root)
			opts.DryRun = true
			opts.Skills = true
			_, err := Apply(opts)
			require.NoError(t, err)

			require.Equal(t, before, snapshotDir(t, root))
		})
	}
}

func TestApply_PreviewEqualsApply(t *testing.T) {
	for _, fixture := range []string{"legacy_single", "split_unversioned"} {
		t.Run(fixture, func(t *testing.T) {
			root := newProject(t, fixture)

			preview, err := Inspect(root, testVersion)
			require.NoError(t, err)

			inst := &fakeInstaller{}
			opts := applyOpts(root)
			opts.Skills = true
			opts.Install = inst.install
			applied, err := Apply(opts)
			require.NoError(t, err)

			appliedFiles := append([]FileReport(nil), applied.Files...)
			for i := range appliedFiles {
				require.NotEmpty(t, appliedFiles[i].Backup)
				appliedFiles[i].Backup = ""
			}
			require.Equal(t, preview.Files, appliedFiles)
			require.Equal(t, preview.Skipped, applied.Skipped)
			require.Equal(t, preview.Skills, applied.Skills)
			require.Equal(t, "upgraded", applied.Status)
		})
	}
}

// --- Apply ---------------------------------------------------------------------

func TestApply_LegacySingleFileSplitsAndCarriesSkillsVersion(t *testing.T) {
	root := newProject(t, "legacy_single")

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "upgraded", rep.Status)

	require.Equal(t, golden(t, "legacy_single", "config.yaml"), string(readFile(t, cfgPath(root, "config.yaml"))))
	require.Equal(t, golden(t, "legacy_single", "repo.yaml"), string(readFile(t, cfgPath(root, "repo.yaml"))))

	_, err = os.Stat(cfgPath(root, "version"))
	require.True(t, os.IsNotExist(err), "the legacy version file is removed once carried")
}

func TestApply_UnversionedSplitProjectIsStamped(t *testing.T) {
	root := newProject(t, "split_unversioned")

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "upgraded", rep.Status)
	require.Len(t, rep.Files, 2)

	require.Equal(t, golden(t, "split_unversioned", "config.yaml"), string(readFile(t, cfgPath(root, "config.yaml"))))
	require.Equal(t, golden(t, "split_unversioned", "repo.yaml"), string(readFile(t, cfgPath(root, "repo.yaml"))))
}

func TestApply_UnnamedLegacyProjectTakesNameFromFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "My Project")
	copyFixture(t, "legacy_single", root)
	// Drop the name line from the fixture copy.
	require.NoError(t, os.WriteFile(cfgPath(root, "config.yaml"), []byte("agent: claude\n"), 0644))

	_, err := Apply(applyOpts(root))
	require.NoError(t, err)

	want := "schema: 3\n" +
		"written_by: 0.1.0\n" +
		"agent: claude\n" +
		"name: my-project\n" +
		"repos:\n" +
		"    - name: my-project\n" +
		"      location: .\n" +
		"skills_version: 0.0.9\n" +
		"spec:\n" +
		"    config:\n" +
		"        directory: specs\n" +
		"plan:\n" +
		"    config:\n" +
		"        directory: plans\n" +
		"changelog:\n" +
		"    config:\n" +
		"        directory: changelog\n"
	require.Equal(t, want, string(readFile(t, cfgPath(root, "config.yaml"))))
}

func TestApply_BackupsAreByteIdenticalAndReported(t *testing.T) {
	root := newProject(t, "split_unversioned")
	origConfig := readFile(t, filepath.Join("testdata", "split_unversioned", "config.yaml"))
	origRepo := readFile(t, filepath.Join("testdata", "split_unversioned", "repo.yaml"))

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Len(t, rep.Files, 2)

	require.Equal(t, cfgPath(root, "config.yaml.v1.old"), rep.Files[0].Backup)
	require.Equal(t, cfgPath(root, "repo.yaml.v1.old"), rep.Files[1].Backup)
	require.Equal(t, origConfig, readFile(t, rep.Files[0].Backup))
	require.Equal(t, origRepo, readFile(t, rep.Files[1].Backup))
}

func TestApply_ExistingDifferentBackupIsNotOverwritten(t *testing.T) {
	root := newProject(t, "split_unversioned")
	stale := []byte("someone else's backup\n")
	require.NoError(t, os.WriteFile(cfgPath(root, "config.yaml.v1.old"), stale, 0644))
	orig := readFile(t, cfgPath(root, "config.yaml"))

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)

	require.Equal(t, cfgPath(root, "config.yaml.v1.old.1"), rep.Files[0].Backup)
	require.Equal(t, orig, readFile(t, rep.Files[0].Backup))
	require.Equal(t, stale, readFile(t, cfgPath(root, "config.yaml.v1.old")))
}

func TestApply_SecondApplyIsUpToDate(t *testing.T) {
	root := newProject(t, "legacy_single")
	inst := &fakeInstaller{}
	opts := applyOpts(root)
	opts.Skills = true
	opts.Install = inst.install

	_, err := Apply(opts)
	require.NoError(t, err)
	require.Equal(t, []string{"claude"}, inst.calls)
	after := snapshotDir(t, root)

	rep, err := Apply(opts)
	require.NoError(t, err)
	require.Equal(t, "up_to_date", rep.Status)
	require.Empty(t, rep.Files)
	require.False(t, rep.Skills.Reinstall)
	require.Equal(t, []string{"claude"}, inst.calls, "no second install")
	require.Equal(t, after, snapshotDir(t, root))
}

// --- Failures ------------------------------------------------------------------

func TestApply_FailingStepLeavesFileAtLastReachedFormat(t *testing.T) {
	root := newProject(t, "split_unversioned")
	cause := errors.New("boom")
	failing := Step{
		Kind:        KindProject,
		From:        2,
		Description: "synthetic failing step",
		Run: func(*StepContext, *yaml.Node) ([]Action, error) {
			return nil, cause
		},
	}
	withSteps(t, KindProject, []Step{project1to2, failing}, 3)

	rep, err := Apply(applyOpts(root))
	require.Error(t, err)

	var se *StepError
	require.True(t, errors.As(err, &se))
	require.Equal(t, cfgPath(root, "config.yaml"), se.Path)
	require.Equal(t, "synthetic failing step", se.Step)
	require.Equal(t, 2, se.ReachedSchema)
	require.ErrorIs(t, err, cause)
	require.Contains(t, err.Error(), "synthetic failing step")
	require.Contains(t, err.Error(), "boom")

	got, err := config.PeekSchema(cfgPath(root, "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, 2, got)

	require.Len(t, rep.Files, 1)
	require.Equal(t, []string{project1to2Desc}, rep.Files[0].Steps)

	// The repo phase never ran.
	repoSchema, err := config.PeekSchema(cfgPath(root, "repo.yaml"))
	require.NoError(t, err)
	require.Equal(t, 1, repoSchema)
}

func TestApply_UnwritableDirectoryFailsAtFirstStep(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := newProject(t, "split_unversioned")
	cfgDir := filepath.Join(root, ".spektacular")
	before := snapshotDir(t, root)
	require.NoError(t, os.Chmod(cfgDir, 0555))
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0755) })

	_, err := Apply(applyOpts(root))
	require.Error(t, err)

	var se *StepError
	require.True(t, errors.As(err, &se))
	require.Equal(t, project1to2Desc, se.Step)
	require.Equal(t, 1, se.ReachedSchema)
	require.Equal(t, before, snapshotDir(t, root))
}

func TestApply_AbsentRepoIsSkippedAndOthersUpgraded(t *testing.T) {
	root := newProject(t, "split_unversioned")
	cfg := "name: split\n" +
		"agent: claude\n" +
		"skills_version: 0.1.0\n" +
		"repos:\n" +
		"    - name: ghost\n" +
		"      location: ../ghost/.spektacular\n" +
		"    - name: split\n" +
		"      location: .\n"
	require.NoError(t, os.WriteFile(cfgPath(root, "config.yaml"), []byte(cfg), 0644))

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "upgraded", rep.Status)

	require.Equal(t, []SkippedRepo{
		{Name: "ghost", Location: filepath.Join(root, "ghost", ".spektacular"), Reason: "not on disk"},
	}, rep.Skipped)
	require.Len(t, rep.Files, 2)
	require.Equal(t, cfgPath(root, "repo.yaml"), rep.Files[1].Path)
	require.Equal(t, golden(t, "split_unversioned", "repo.yaml"), string(readFile(t, cfgPath(root, "repo.yaml"))))
}

// A registered repo whose repo.yaml cannot be parsed is broken, not out of
// date: it is skipped with its reason, left untouched, and the rest of the
// project still upgrades.
func TestApply_UnreadableRepoIsSkippedAndOthersUpgraded(t *testing.T) {
	root := newProject(t, "split_unversioned")
	brokenDir := filepath.Join(root, "broken", ".spektacular")
	require.NoError(t, os.MkdirAll(brokenDir, 0755))
	brokenPath := filepath.Join(brokenDir, "repo.yaml")
	require.NoError(t, os.WriteFile(brokenPath, []byte("{{{\n"), 0644))
	cfg := "name: split\n" +
		"agent: claude\n" +
		"skills_version: 0.1.0\n" +
		"repos:\n" +
		"    - name: broken\n" +
		"      location: ../broken/.spektacular\n" +
		"    - name: split\n" +
		"      location: .\n"
	require.NoError(t, os.WriteFile(cfgPath(root, "config.yaml"), []byte(cfg), 0644))

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "upgraded", rep.Status)

	require.Len(t, rep.Skipped, 1)
	require.Equal(t, "broken", rep.Skipped[0].Name)
	require.Equal(t, brokenDir, rep.Skipped[0].Location)
	require.True(t, strings.HasPrefix(rep.Skipped[0].Reason, "unreadable settings"), rep.Skipped[0].Reason)
	require.Equal(t, "{{{\n", string(readFile(t, brokenPath)))

	require.Len(t, rep.Files, 2)
	require.Equal(t, cfgPath(root, "config.yaml"), rep.Files[0].Path)
	require.Equal(t, cfgPath(root, "repo.yaml"), rep.Files[1].Path)
	require.Equal(t, golden(t, "split_unversioned", "repo.yaml"), string(readFile(t, cfgPath(root, "repo.yaml"))))
}

func TestApply_FailingInstallerLeavesSkillsVersionUntouched(t *testing.T) {
	root := newProject(t, "legacy_single")
	cause := errors.New("install failed")
	inst := &fakeInstaller{err: cause}
	opts := applyOpts(root)
	opts.Skills = true
	opts.Install = inst.install

	_, err := Apply(opts)
	require.Error(t, err)
	var se *StepError
	require.True(t, errors.As(err, &se))
	require.Equal(t, "reinstall agent skills", se.Step)
	require.ErrorIs(t, err, cause)
	require.Equal(t, []string{"claude"}, inst.calls)

	// The format upgrade landed, but skills_version still records the old
	// install: the file is exactly the no-skills upgrade.
	require.Equal(t, golden(t, "legacy_single", "config.yaml"), string(readFile(t, cfgPath(root, "config.yaml"))))

	rep, err := Inspect(root, testVersion)
	require.NoError(t, err)
	require.Equal(t, "mismatch", rep.Skills.Status)
	require.Equal(t, "0.0.9", rep.Skills.Installed)
}

func TestApply_MissingAgentReturnsErrNoAgent(t *testing.T) {
	root := writeProject(t, "schema: 3\nwritten_by: 0.1.0\nname: noagent\nskills_version: 0.0.9\n")
	before := snapshotDir(t, root)
	inst := &fakeInstaller{}
	opts := applyOpts(root)
	opts.Skills = true
	opts.Install = inst.install

	_, err := Apply(opts)
	require.ErrorIs(t, err, ErrNoAgent)
	require.Empty(t, inst.calls)
	require.Equal(t, before, snapshotDir(t, root))

	// A preview does not fail: it only reports the reinstall.
	rep, err := Inspect(root, testVersion)
	require.NoError(t, err)
	require.True(t, rep.Skills.Reinstall)
}

func TestApply_NewerSchemaIsRefusedUnchanged(t *testing.T) {
	raw := "schema: 99\nname: future\n"
	root := writeProject(t, raw)

	for name, run := range map[string]func() (Report, error){
		"inspect": func() (Report, error) { return Inspect(root, testVersion) },
		"apply":   func() (Report, error) { return Apply(applyOpts(root)) },
	} {
		t.Run(name, func(t *testing.T) {
			rep, err := run()
			fe, ok := config.IsFormatError(err)
			require.True(t, ok, "want *config.FormatError, got %v", err)
			require.True(t, fe.Newer())
			require.Equal(t, 99, fe.Found)
			require.Equal(t, config.CurrentProjectSchema, fe.Want)
			require.Equal(t, "unsupported_format", rep.Status)
			require.Equal(t, raw, string(readFile(t, cfgPath(root, "config.yaml"))))
		})
	}
}

// --- Extensibility ---------------------------------------------------------------

// TestApply_SyntheticStepIsPickedUp shows a new format change needs only a
// registered step and a bumped current format: Inspect reports it and Apply
// runs it with no other code change.
func TestApply_SyntheticStepIsPickedUp(t *testing.T) {
	root := newProject(t, "current")
	synthetic := Step{
		Kind:        KindProject,
		From:        3,
		Description: "synthetic 3 to 4",
		Run: func(sc *StepContext, doc *yaml.Node) ([]Action, error) {
			setScalar(docRoot(doc), "synthetic", "added")
			return []Action{{Op: "set", Path: filepath.Join(sc.FileDir, "config.yaml"), Key: "synthetic", To: "added"}}, nil
		},
	}
	withSteps(t, KindProject, []Step{project1to2, project2to3, synthetic}, 4)

	preview, err := Inspect(root, testVersion)
	require.NoError(t, err)
	require.Equal(t, "upgrade_needed", preview.Status)
	require.Len(t, preview.Files, 1)
	require.Equal(t, 3, preview.Files[0].From)
	require.Equal(t, 4, preview.Files[0].To)
	require.Equal(t, []string{"synthetic 3 to 4"}, preview.Files[0].Steps)

	rep, err := Apply(applyOpts(root))
	require.NoError(t, err)
	require.Equal(t, "upgraded", rep.Status)
	require.Equal(t, cfgPath(root, "config.yaml.v3.old"), rep.Files[0].Backup)

	want := "schema: 4\n" +
		"written_by: 0.1.0\n" +
		"name: current\n" +
		"agent: claude\n" +
		"skills_version: 0.1.0\n" +
		"spec:\n" +
		"    config:\n" +
		"        directory: specs\n" +
		"plan:\n" +
		"    config:\n" +
		"        directory: plans\n" +
		"changelog:\n" +
		"    config:\n" +
		"        directory: changelog\n" +
		"repos:\n" +
		"    - name: current\n" +
		"      location: .\n" +
		"synthetic: added\n"
	require.Equal(t, want, string(readFile(t, cfgPath(root, "config.yaml"))))
}
