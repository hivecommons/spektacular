package plan

import (
	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/stepkit"
)

// strategy implements stepkit.PathStrategy for the plan workflow. planDir is
// the configured plan directory.
type strategy struct {
	planDir string
}

func (s strategy) PrimaryLocation(instanceName string) string {
	return artifact.Location(config.ProjectConfigDirName, PlanFilePath(s.planDir, instanceName))
}

func (strategy) PathVars(instanceName, _ string) map[string]any {
	return map[string]any{
		"plan_name": instanceName,
	}
}

// Compile-time interface check.
var _ stepkit.PathStrategy = strategy{}
