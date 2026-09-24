package repo

import (
	"fmt"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	repodomain "github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// Placement values. They name where the project keeps its own files for a
// repo, and nothing else: the vocabulary a user is shown never includes them.
const (
	// placementInside puts the project's files for a repo inside that repo,
	// which is the default whenever the repo can take them.
	placementInside = "inside"
	// placementProject puts them in a folder under the project instead,
	// leaving the repo with nothing but its own code.
	placementProject = "project"
)

// doRegister performs the registration the conversation gathered.
//
// It runs as the register step's callback, which the engine invokes before
// the transition it belongs to is committed. That ordering is what makes
// "nothing is written until the user says so" a property of the machine
// rather than of the agent doing as it was told: an error here vetoes the
// transition, so a declined or failed confirmation leaves the workflow
// standing on confirm with the target repo and the project registry both
// untouched.
func doRegister(data workflow.Data, st store.Store) (repodomain.RegistrationResult, error) {
	var zero repodomain.RegistrationResult

	if st == nil {
		return zero, fmt.Errorf("store required for register step")
	}
	projectRoot := st.Root()

	name := stepkit.GetString(data, "name")
	if name == "" {
		return zero, output.NewError("name_required", "the repo has no name to register under").
			WithNextAction(`go back to the name step and record a name before registering`)
	}

	target := targetDir(data)
	if target == "" {
		return zero, output.NewError("location_required", "the repo being added has no known location").
			WithNextAction(`go back to the locate step and record the folder the repo's code lives in`)
	}

	cfg, err := config.FromYAMLFile(filepath.Join(config.ProjectConfigDir(projectRoot), "config.yaml"))
	if err != nil {
		return zero, fmt.Errorf("reading project config: %w", err)
	}

	reg := repodomain.Registration{
		Name:        name,
		Description: stepkit.GetString(data, "description"),
		Role:        stepkit.GetString(data, "role"),
		Tags:        stringSlice(data, "tags"),
	}

	// The placement decision becomes a location and, when the repo is to be
	// left holding nothing but code, a source naming where that code lives.
	// Locations are written relative to the folder holding config.yaml,
	// because that is what they are resolved against when read back.
	switch stepkit.GetString(data, "placement") {
	case placementProject:
		reg.Location = relativeToConfig(projectRoot, filepath.Join(projectRoot, "repos", name))
		reg.Source = target
	default:
		reg.Location = relativeToConfig(projectRoot, target)
	}

	res, err := repodomain.Register(&cfg, projectRoot, repodomain.NewGitRunner(), reg)
	if err != nil {
		return zero, err
	}
	return res, nil
}

// relativeToConfig expresses an absolute path the way a registry entry must
// be written: relative to the folder holding config.yaml, which is what
// RepoEntry.ResolvedLocation resolves against. An absolute path is kept when
// no relative form exists, for instance across drives.
func relativeToConfig(projectRoot, abs string) string {
	rel, err := filepath.Rel(config.ProjectConfigDir(projectRoot), abs)
	if err != nil {
		return abs
	}
	return rel
}

// stringSlice reads a list of strings out of gathered answers. The values
// arrive as JSON, so they may be either a typed slice within the session that
// gathered them or a slice of any after a resume.
func stringSlice(data workflow.Data, key string) []string {
	raw, ok := data.Get(key)
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
