package implement

import (
	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/stepkit"
)

// PlanFilePath returns the store-relative path for a plan's plan.md file under
// the configured plan directory.
// Kept as a copy of internal/steps/plan.PlanFilePath to avoid a cross-package
// dependency for a 10-line constant function.
func PlanFilePath(dir, name string) string {
	return PlanDocumentPath(dir, name, "plan")
}

// ContextFilePath returns the store-relative path for a plan's context.md file
// under the configured plan directory.
func ContextFilePath(dir, name string) string {
	return PlanDocumentPath(dir, name, "context")
}

// ResearchFilePath returns the store-relative path for a plan's research.md file
// under the configured plan directory.
func ResearchFilePath(dir, name string) string {
	return PlanDocumentPath(dir, name, "research")
}

// ChangelogFilePath returns the store-relative path for a feature's
// project-level changelog record under the configured changelog directory.
// The project-level record is a flat file per feature at the root of the
// project's changelog directory — no `<project>/` subfolder, because the
// project owns its own store. Per-repo derived entries live under member
// repos' own changelog stores and are routed by the `--repo` flag of
// `changelog file write`, not by this helper.
func ChangelogFilePath(dir, name string) string {
	return artifact.Address{Kind: artifact.KindChangelog, Feature: name}.StorePath(dir)
}

// PlanDocumentPath returns the store-relative path of the plan document
// addressed by feature name and document under the configured plan directory.
func PlanDocumentPath(dir, name, document string) string {
	return artifact.Address{Kind: artifact.KindPlan, Feature: name, Document: document}.StorePath(dir)
}

// strategy implements stepkit.PathStrategy for the implement workflow. planDir
// is the configured plan directory.
type strategy struct {
	planDir string
}

func (s strategy) PrimaryLocation(instanceName string) string {
	return artifact.Location(config.ProjectConfigDirName, PlanFilePath(s.planDir, instanceName))
}

func (strategy) PathVars(instanceName, _ string) map[string]any {
	return map[string]any{
		"plan_name":              instanceName,
		"changelog_section_name": "## Changelog",
	}
}

// Compile-time interface check.
var _ stepkit.PathStrategy = strategy{}
