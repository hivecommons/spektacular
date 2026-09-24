package spec

import (
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/stepkit"
)

// strategy implements stepkit.PathStrategy for the spec workflow. specDir is
// the configured spec directory the workflow writes into.
type strategy struct {
	specDir string
}

func (strategy) PrimaryPathField() string { return "spec_path" }

func (s strategy) PathVars(instanceName, storeRoot string) map[string]any {
	return map[string]any{
		"spec_path": filepath.Join(storeRoot, SpecFilePath(s.specDir, instanceName)),
		"spec_name": instanceName,
	}
}

// Compile-time interface check.
var _ stepkit.PathStrategy = strategy{}
