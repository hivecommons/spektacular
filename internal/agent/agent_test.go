package agent

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// fakeAgent is a minimal Agent implementation used to exercise the registry
// helpers in isolation from any real agent wiring.
type fakeAgent struct{ name string }

func (f fakeAgent) Name() string { return f.name }
func (f fakeAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	return nil
}

// withSourceFS swaps the package-level sourceFS for the duration of the test
// and restores the original via t.Cleanup.
func withSourceFS(t *testing.T, fsys fs.FS) {
	t.Helper()
	orig := sourceFS
	sourceFS = fsys
	t.Cleanup(func() { sourceFS = orig })
}

// registerFake registers a fakeAgent under name and ensures it is removed from
// the registry when the test finishes.
func registerFake(t *testing.T, name string) Agent {
	t.Helper()
	a := fakeAgent{name: name}
	register(a)
	t.Cleanup(func() { delete(registry, name) })
	return a
}

func TestLookup_UnknownAgent(t *testing.T) {
	a, err := Lookup("nope")
	require.Error(t, err)
	require.Nil(t, a)
	require.True(t, errors.Is(err, ErrUnknownAgent), "error should wrap ErrUnknownAgent, got %v", err)

	// Every currently-supported name (if any) should be mentioned in the
	// error message so the user can recover. In isolation the registry may
	// be empty, in which case we've already asserted the sentinel wrap.
	for _, name := range Supported() {
		require.Contains(t, err.Error(), name, "error message should name supported agent %q", name)
	}
}

func TestLookup_ReturnsRegisteredAgent(t *testing.T) {
	want := registerFake(t, "fake")

	got, err := Lookup("fake")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, want, got)
}

func TestSupported_StableOrder(t *testing.T) {
	registerFake(t, "z-fake")
	registerFake(t, "a-fake")

	// Filter the full Supported() list down to just the names we own so the
	// test tolerates any other agents registered by package init().
	all := Supported()
	var got []string
	for _, n := range all {
		if n == "a-fake" || n == "z-fake" {
			got = append(got, n)
		}
	}
	require.Equal(t, []string{"a-fake", "z-fake"}, got)
}

func TestInstallWorkflowSkills_WritesEveryRegisteredSkill(t *testing.T) {
	withSourceFS(t, fstest.MapFS{
		"skills/workflows/spek-new/SKILL.md": &fstest.MapFile{
			Data: []byte("new skill: run {{command}} spec new\n"),
		},
		"skills/workflows/spek-plan/SKILL.md": &fstest.MapFile{
			Data: []byte("plan skill: run {{command}} spec plan\n"),
		},
		"skills/workflows/spek-implement/SKILL.md": &fstest.MapFile{
			Data: []byte("implement skill: run {{command}} spec implement\n"),
		},
		"skills/workflows/spek-knowledge/SKILL.md": &fstest.MapFile{
			Data: []byte("knowledge skill: run {{command}} knowledge search\n"),
		},
		"skills/workflows/spek-manage-repos/SKILL.md": &fstest.MapFile{
			Data: []byte("repos skill: run {{command}} repo list\n"),
		},
		"skills/workflows/spek-design/SKILL.md": &fstest.MapFile{
			Data: []byte("design skill: run {{command}} design sources\n"),
		},
	})

	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	err := installWorkflowSkills(tmp, ".claude/skills", cfg, io.Discard)
	require.NoError(t, err)

	skillsRoot := filepath.Join(tmp, ".claude", "skills")
	for _, name := range []string{"spek-new", "spek-plan", "spek-implement", "spek-knowledge", "spek-manage-repos", "spek-design"} {
		path := filepath.Join(skillsRoot, name, "SKILL.md")
		data, err := os.ReadFile(path)
		require.NoError(t, err, "expected file %s to exist", path)
		content := string(data)
		require.Contains(t, content, "go run .")
		require.NotContains(t, content, "{{command}}")
	}

	// Ensure exactly six SKILL.md files were written under skillsRoot.
	var skillFiles []string
	err = filepath.WalkDir(skillsRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && filepath.Base(p) == "SKILL.md" {
			skillFiles = append(skillFiles, p)
		}
		return nil
	})
	require.NoError(t, err)
	require.Len(t, skillFiles, 6, "expected exactly six SKILL.md files, got %v", skillFiles)
}

// skillFixtureWithImplement returns a source FS holding every workflow skill
// template, with spek-implement's body set to implementBody, plus any extra
// files given.
func skillFixtureWithImplement(implementBody string, extra fstest.MapFS) fstest.MapFS {
	fsys := fstest.MapFS{
		"skills/workflows/spek-new/SKILL.md":          &fstest.MapFile{Data: []byte("new skill\n")},
		"skills/workflows/spek-plan/SKILL.md":         &fstest.MapFile{Data: []byte("plan skill\n")},
		"skills/workflows/spek-implement/SKILL.md":    &fstest.MapFile{Data: []byte(implementBody)},
		"skills/workflows/spek-knowledge/SKILL.md":    &fstest.MapFile{Data: []byte("knowledge skill\n")},
		"skills/workflows/spek-manage-repos/SKILL.md": &fstest.MapFile{Data: []byte("repos skill\n")},
		"skills/workflows/spek-design/SKILL.md":       &fstest.MapFile{Data: []byte("design skill\n")},
	}
	for name, f := range extra {
		fsys[name] = f
	}
	return fsys
}

func TestInstallWorkflowSkills_RendersPartialWithCommand(t *testing.T) {
	withSourceFS(t, skillFixtureWithImplement(
		"# Implement\n\n{{> partials/p}}\nafter\n",
		fstest.MapFS{
			"partials/p.md": &fstest.MapFile{Data: []byte("read with `{{command}} plan file read`\n")},
		},
	))

	tmp := t.TempDir()
	err := installWorkflowSkills(tmp, ".claude/skills", config.Config{Command: "spekx"}, io.Discard)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmp, ".claude", "skills", "spek-implement", "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "# Implement\n\nread with `spekx plan file read`\nafter\n", string(data))
}

func TestInstallWorkflowSkills_MissingPartialFails(t *testing.T) {
	withSourceFS(t, skillFixtureWithImplement("# Implement\n\n{{> partials/p}}\n", nil))

	err := installWorkflowSkills(t.TempDir(), ".claude/skills", config.Config{Command: "spekx"}, io.Discard)
	require.ErrorContains(t, err, "partials/p", "a missing partial must fail the install, naming the partial")
}

func TestInstallCommandWrappers_UsesFilenameFunc(t *testing.T) {
	withSourceFS(t, fstest.MapFS{
		"commands/wrapper.md": &fstest.MapFile{
			Data: []byte("cmd: {{command}} / skill: {{skill}} / desc: {{description}}\n"),
		},
	})

	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	filename := func(s string) string {
		return strings.TrimPrefix(s, "spek-") + ".md"
	}

	err := installCommandWrappers(tmp, ".claude/commands/spek", filename, cfg, io.Discard)
	require.NoError(t, err)

	cmdRoot := filepath.Join(tmp, ".claude", "commands", "spek")
	expected := map[string]string{
		"new.md":          "spek-new",
		"plan.md":         "spek-plan",
		"implement.md":    "spek-implement",
		"knowledge.md":    "spek-knowledge",
		"manage-repos.md": "spek-manage-repos",
		"design.md":       "spek-design",
	}
	for base, skillName := range expected {
		path := filepath.Join(cmdRoot, base)
		data, err := os.ReadFile(path)
		require.NoError(t, err, "expected file %s to exist", path)
		content := string(data)
		require.Contains(t, content, skillName)
		require.Contains(t, content, "go run .")
		require.NotContains(t, content, "{{command}}")
		require.NotContains(t, content, "{{skill}}")
	}

	// Ensure exactly six files were written under cmdRoot.
	entries, err := os.ReadDir(cmdRoot)
	require.NoError(t, err)
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	require.Len(t, files, 6, "expected exactly six wrapper files, got %v", files)
}

// TestWorkflowSkillsAndDescriptionsAgree asserts the two install registries
// stay in step: every skill in workflowSkills has a non-empty description in
// workflowDescriptions, and workflowDescriptions carries no entry for a skill
// that is not installed.
//
// This is asserted over the whole table rather than one row because the
// failure it guards is silent: installCommandWrappers renders
// workflowDescriptions[s.Name] straight into the wrapper, so a skill added to
// one table and not the other ships an empty description into every
// non-Claude agent's slash-command menu, with no error anywhere.
func TestWorkflowSkillsAndDescriptionsAgree(t *testing.T) {
	for _, s := range workflowSkills {
		desc, ok := workflowDescriptions[s.Name]
		require.Truef(t, ok, "workflowSkills lists %q but workflowDescriptions has no entry for it, so its command wrapper would render an empty description", s.Name)
		require.NotEmptyf(t, desc, "workflowDescriptions entry for %q is empty, so its command wrapper would render an empty description", s.Name)
	}

	installed := make(map[string]bool, len(workflowSkills))
	for _, s := range workflowSkills {
		installed[s.Name] = true
	}
	for name := range workflowDescriptions {
		require.Truef(t, installed[name], "workflowDescriptions describes %q, which workflowSkills does not install", name)
	}
}

// validateSkillFrontmatter reads the SKILL.md at path, parses its YAML
// frontmatter, and asserts the name/description fields satisfy the
// agentskills.io naming rules and match the parent directory basename.
func validateSkillFrontmatter(t *testing.T, path string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "reading skill file %s", path)

	parts := strings.SplitN(string(raw), "---", 3)
	require.GreaterOrEqual(t, len(parts), 3, "skill file %s should contain a YAML frontmatter block delimited by ---", path)

	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &fm), "parsing frontmatter in %s", path)

	expectedName := filepath.Base(filepath.Dir(path))
	require.Equal(t, expectedName, fm.Name, "skill name in %s should match parent directory basename", path)

	nameRegex := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	require.Regexp(t, nameRegex, fm.Name, "skill name %q in %s must match agentskills.io naming rules", fm.Name, path)
	require.GreaterOrEqual(t, len(fm.Name), 1, "skill name in %s must be at least 1 character", path)
	require.LessOrEqual(t, len(fm.Name), 64, "skill name in %s must be at most 64 characters", path)

	require.NotEmpty(t, fm.Description, "skill description in %s must be non-empty", path)
}
