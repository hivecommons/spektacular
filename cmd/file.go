package cmd

import (
	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
)

// The `spec file` subcommand group reads and writes specs, each addressed by
// the feature's bare name. See newStoreFileCmd for the shared implementation.
func init() {
	specCmd.AddCommand(newStoreFileCmd(storeFileKind{
		kind:  artifact.KindSpec,
		short: "Read and write specs in the spec store",
		dir:   func(c config.Config) string { return c.Spec.Config.Directory },
	}))
}
