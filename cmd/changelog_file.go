package cmd

import (
	"github.com/hivecommons/spektacular/internal/config"
)

// The `changelog file` subcommand group reads and writes changelog records.
//
// Without `--repo`, writes land in the project's own configured changelog
// directory directly — no `<project>/` subfolder, because the project owns
// its own store and cannot collide with another project.
//
// With `--repo <name>`, writes route into the named member repo's own
// changelog store, namespaced under a `<project>/` folder so multiple
// projects writing to the same shared repo cannot collide. That routing
// lives in repoRoutedStore, below the CLI surface.
//
// See newStoreFileCmd for the shared implementation.
func init() {
	changelogCmd.AddCommand(newStoreFileCmd(
		"Read and write files in the changelog store",
		func(c config.Config) string { return c.Changelog.Config.Directory },
		true,
		true,
	))
}
