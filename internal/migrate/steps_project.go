package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jumppad-labs/spektacular/internal/config"
	"gopkg.in/yaml.v3"
)

// legacyVersionFile is the standalone file older projects used to record the
// Spektacular version that installed their agent skills. Project settings'
// skills_version replaces it.
const legacyVersionFile = "version"

// project1to2 turns an older project into the split config.yaml/repo.yaml
// layout and moves the installed skills version into the settings.
var project1to2 = Step{
	Kind:        KindProject,
	From:        1,
	Description: "split legacy single-file settings and record installed skills version",
	Run:         runProject1to2,
}

func runProject1to2(sc *StepContext, doc *yaml.Node) ([]Action, error) {
	root := docRoot(doc)
	settings := filepath.Join(sc.FileDir, config.ProjectConfigFileName)
	var actions []Action

	// (a) A colocated repo.yaml, from the project's own metadata, when the
	// project predates the split.
	repoPath := filepath.Join(sc.FileDir, config.RepoConfigFileName)
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		repoCfg := scanMetadata(sc.ProjectRoot)
		repoCfg.Source = config.DefaultRepoSource
		repoCfg.Schema = config.CurrentRepoSchema
		repoCfg.WrittenBy = sc.BinaryVersion
		content, err := yaml.Marshal(repoCfg)
		if err != nil {
			return nil, fmt.Errorf("building %s: %w", repoPath, err)
		}
		actions = append(actions, Action{Op: "create", Path: repoPath, Content: content})
	} else if err != nil {
		return nil, fmt.Errorf("checking %s: %w", repoPath, err)
	}

	// (b) A repo registry naming the colocated repo, as init seeds it.
	if repos, _ := lookup(root, "repos"); repos == nil || repos.Kind != yaml.SequenceNode || len(repos.Content) == 0 {
		name, ok := getScalar(root, "name")
		if !ok || strings.TrimSpace(name) == "" {
			name = config.SlugifyName(filepath.Base(sc.ProjectRoot))
			setScalar(root, "name", name)
			actions = append(actions, Action{Op: "set", Path: settings, Key: "name", To: name})
		}
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			scalarKey("name"), {Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			scalarKey("location"), {Kind: yaml.ScalarNode, Tag: "!!str", Value: "."},
		}}
		setNode(root, "repos", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{entry}})
		actions = append(actions, Action{Op: "set", Path: settings, Key: "repos", To: fmt.Sprintf("[{name: %s, location: .}]", name)})
	}

	// (c) The standalone skills-version file, carried into the settings.
	if v, ok := getScalar(root, "skills_version"); !ok || v == "" {
		versionPath := filepath.Join(sc.FileDir, legacyVersionFile)
		data, err := os.ReadFile(versionPath)
		switch {
		case err == nil:
			installed := strings.TrimSpace(string(data))
			if installed != "" {
				setScalar(root, "skills_version", installed)
				actions = append(actions, Action{Op: "set", Path: settings, Key: "skills_version", To: installed})
			}
			actions = append(actions, Action{Op: "remove", Path: versionPath})
		case !os.IsNotExist(err):
			return nil, fmt.Errorf("reading %s: %w", versionPath, err)
		}
	}

	return actions, nil
}

// project2to3 re-expresses the spec, plan and changelog folders relative to
// the folder holding config.yaml, the rule every other relative path in the
// file follows. Format 2 read them from the project root, so each is
// rewritten to the value that names the same folder on disk under the new
// rule, and nothing already stored goes missing.
var project2to3 = Step{
	Kind:        KindProject,
	From:        2,
	Description: "resolve spec, plan and changelog folders from the settings file",
	Run:         runProject2to3,
}

// format2StoreDirs are the store folders format 2 used when a key was absent,
// project-root-relative.
var format2StoreDirs = []struct{ key, fallback string }{
	{"spec.config.directory", ".spektacular/specs"},
	{"plan.config.directory", ".spektacular/plans"},
	{"changelog.config.directory", ".spektacular/changelog"},
}

func runProject2to3(sc *StepContext, doc *yaml.Node) ([]Action, error) {
	root := docRoot(doc)
	settings := filepath.Join(sc.FileDir, config.ProjectConfigFileName)
	var actions []Action
	for _, d := range format2StoreDirs {
		old, present := getScalar(root, d.key)
		if !present || old == "" {
			old = d.fallback
		}
		value := old
		if !filepath.IsAbs(old) {
			rel, err := filepath.Rel(sc.FileDir, filepath.Join(sc.ProjectRoot, old))
			if err != nil {
				return nil, fmt.Errorf("re-expressing %s %q: %w", d.key, old, err)
			}
			value = filepath.ToSlash(rel)
		}
		// An explicit value is always written, so a later change of default
		// can never move an existing project's folders.
		if value != old || !present {
			setScalar(root, d.key, value)
			a := Action{Op: "set", Path: settings, Key: d.key, To: value}
			if present {
				a.From = old
			}
			actions = append(actions, a)
		}
	}
	return actions, nil
}
