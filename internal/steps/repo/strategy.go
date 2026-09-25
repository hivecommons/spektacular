package repo

import (
	"github.com/hivecommons/spektacular/internal/stepkit"
)

// strategy implements stepkit.PathStrategy for the guided add workflow.
// repoPath is the absolute folder the repo's code lives at, resolved from the
// location gathered during the flow. Unlike the spec and plan workflows,
// whose primary path is a document this workflow writes, the guided add's
// primary path is a folder it reads and, at the very end, registers.
type strategy struct {
	repoPath string
}

// PrimaryLocation is the repo's code folder. It is not a stored document, so
// the rule that documents are reported by config-relative location does not
// apply to it.
func (s strategy) PrimaryLocation(string) string { return s.repoPath }

func (s strategy) PathVars(instanceName, _ string) map[string]any {
	return map[string]any{
		"repo_path": s.repoPath,
		"repo_name": instanceName,
	}
}

// Compile-time interface check.
var _ stepkit.PathStrategy = strategy{}
