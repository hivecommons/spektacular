package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

const (
	storeAccessTemplatePath = "agents/store-access.md"
	storeAccessHeading      = "## Spektacular's Files Are Reached Through Spektacular"
)

// installStoreAccessSection writes (or updates in place) the managed
// "Spektacular's Files Are Reached Through Spektacular" section in
// <projectPath>/AGENTS.md, rendering the embedded template against
// cfg.Command. Idempotent: re-running for the same projectPath leaves a
// single section and does not duplicate.
//
// This section is the single copy of the rule. The workflow skills used to
// each restate it, which meant an agent was only told while one of them was
// loaded — never in an ad-hoc session, and never in a sub-agent, which
// inherits AGENTS.md but not the skill that spawned it. The skills now name
// only the commands their own store needs.
func installStoreAccessSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSection(projectPath, cfg, out, storeAccessTemplatePath, storeAccessHeading, "Spektacular file access section")
}
