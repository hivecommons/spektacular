package cmd

import "github.com/hivecommons/spektacular/internal/config"

// The `spec file` subcommand group reads and writes spec files within the
// configured spec directory. See newStoreFileCmd for the shared implementation.
func init() {
	specCmd.AddCommand(newStoreFileCmd(
		"Read and write files in the spec store",
		func(c config.Config) string { return c.Spec.Config.Directory },
		false,
		false,
	))
}
