package plan

import (
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/stepkit"
)

// strategy implements stepkit.PathStrategy for the plan workflow. planDir and
// specDir are the configured plan and spec directories.
type strategy struct {
	planDir string
	specDir string
}

func (strategy) PrimaryPathField() string { return "plan_path" }

func (s strategy) PathVars(instanceName, storeRoot string) map[string]any {
	planPath := filepath.Join(storeRoot, PlanFilePath(s.planDir, instanceName))
	contextPath := filepath.Join(storeRoot, ContextFilePath(s.planDir, instanceName))
	researchPath := filepath.Join(storeRoot, ResearchFilePath(s.planDir, instanceName))
	specPath := filepath.Join(storeRoot, s.specDir, instanceName+".md")

	return map[string]any{
		"plan_path":     planPath,
		"context_path":  contextPath,
		"research_path": researchPath,
		"plan_dir":      filepath.Dir(planPath),
		"plan_name":     instanceName,
		"spec_path":     specPath,
	}
}

// Compile-time interface check.
var _ stepkit.PathStrategy = strategy{}
