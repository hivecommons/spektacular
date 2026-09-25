package cmd

import (
	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
)

// A location the CLI reports for a stored document is relative to the folder
// holding the configuration file that declares its store: the settings folder
// holding config.yaml for the central stores, and the repo.yaml folder for a
// repo-routed changelog store. That is the same base every relative location
// in those files resolves against, so a reported location reads the way the
// configured directory is written (`specs/<name>.md`, `plans/<name>/plan.md`)
// and never carries a host path.

// centralLocationBase is the store-relative folder holding config.yaml, the
// base for locations in the central spec, plan and changelog stores, whose
// configured directories are rewritten project-root-relative on load.
const centralLocationBase = config.ProjectConfigDirName

// reportedLocation renders storePath relative to base; see artifact.Location.
func reportedLocation(base, storePath string) string {
	return artifact.Location(base, storePath)
}
