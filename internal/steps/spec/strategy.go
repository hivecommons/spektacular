package spec

import (
	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/stepkit"
)

// strategy implements stepkit.PathStrategy for the spec workflow. specDir is
// the configured spec directory the workflow writes into.
type strategy struct {
	specDir string
}

func (s strategy) PrimaryLocation(instanceName string) string {
	return artifact.Location(config.ProjectConfigDirName, SpecFilePath(s.specDir, instanceName))
}

func (strategy) PathVars(instanceName, _ string) map[string]any {
	return map[string]any{
		"spec_name": instanceName,
	}
}

// Compile-time interface check.
var _ stepkit.PathStrategy = strategy{}
