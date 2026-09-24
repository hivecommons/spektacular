package cmd

import "github.com/hivecommons/spektacular/internal/config"

// The `plan file` subcommand group reads and writes plan documents
// (plan.md, context.md, research.md) within the configured plan directory.
// See newStoreFileCmd for the shared implementation.
func init() {
	planCmd.AddCommand(newStoreFileCmd(
		"Read and write files in the plan store",
		func(c config.Config) string { return c.Plan.Config.Directory },
		true,
		false,
	))
}
