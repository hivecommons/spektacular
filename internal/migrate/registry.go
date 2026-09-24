// Package migrate upgrades Spektacular settings files between format
// versions. It is the single authority on whether a project is out of date
// and how to bring it current: `migrate`, `init`, `version check` and the
// command gate all go through Inspect or Apply.
//
// Each format change ships as one registered Step that moves a settings kind
// from format N to N+1, together with a bump of the matching constant in the
// config package. A registry test pins the two together.
package migrate

import (
	"fmt"

	"github.com/hivecommons/spektacular/internal/config"
	"gopkg.in/yaml.v3"
)

// Kind names a settings file kind.
type Kind string

const (
	// KindProject is a project's config.yaml.
	KindProject Kind = "project"
	// KindRepo is a repo's repo.yaml.
	KindRepo Kind = "repo"
)

// Step is a single From → From+1 upgrade. A step edits the in-memory YAML
// tree and describes any side effects as Actions; it never touches disk
// itself, which is what lets a preview and an apply share one code path.
type Step struct {
	Kind        Kind
	From        int
	Description string
	Run         func(sc *StepContext, doc *yaml.Node) ([]Action, error)
}

// StepContext tells a step where the file it is upgrading lives.
type StepContext struct {
	ProjectRoot   string // parent of .spektacular
	FileDir       string // folder holding the file being upgraded
	BinaryVersion string // the running Spektacular version
}

// Action is a side effect of a step, reported in previews and performed by
// Apply.
type Action struct {
	Op      string `json:"op"`            // "set" | "create" | "remove"
	Path    string `json:"path"`          // the file created/removed, or the settings file for "set"
	Key     string `json:"key,omitempty"` // dotted key for "set"
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Content []byte `json:"-"` // payload for "create"
}

// registry holds the ordered steps for each kind, and current the format
// each kind is upgraded to. Both are variables only so tests can register a
// synthetic step.
var (
	registry = map[Kind][]Step{
		KindProject: {project1to2, project2to3},
		KindRepo:    {repo1to2},
	}
	current = map[Kind]int{
		KindProject: config.CurrentProjectSchema,
		KindRepo:    config.CurrentRepoSchema,
	}
)

// Registered returns the steps registered for kind, in order.
func Registered(kind Kind) []Step {
	return append([]Step(nil), registry[kind]...)
}

// currentSchema returns the format this build upgrades kind to.
func currentSchema(kind Kind) int {
	return current[kind]
}

// stepsFrom returns the steps that take a kind from format `from` to
// current, failing when a format in between has no registered step.
func stepsFrom(kind Kind, from int) ([]Step, error) {
	var steps []Step
	for n := from; n < currentSchema(kind); n++ {
		found := false
		for _, s := range registry[kind] {
			if s.From == n {
				steps = append(steps, s)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no %s upgrade step registered from format %d", kind, n)
		}
	}
	return steps, nil
}
