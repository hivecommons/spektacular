package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/agent"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/migrate"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/spf13/cobra"
)

// gateAnnotation marks a command the upgrade gate lets through on an
// out-of-date project, so the way out (migrate, init, version check) is
// always reachable. The value is gateExempt.
const (
	gateAnnotation = "gate"
	gateExempt     = "exempt"
)

var migrateCmd = &cobra.Command{
	Use:         "migrate",
	Short:       "Upgrade the project's settings and installed skills to this Spektacular version",
	Args:        cobra.NoArgs,
	Annotations: map[string]string{gateAnnotation: gateExempt},
	RunE:        runMigrate,
}

func init() {
	migrateCmd.Flags().Bool("dry-run", false, "report every change without touching disk")
}

func runMigrate(cmd *cobra.Command, _ []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	root, err := projectRoot()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(config.ProjectConfigDir(root), config.ProjectConfigFileName)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return noProjectError(cfgPath)
	}

	rep, err := migrate.Apply(migrate.Options{
		ProjectRoot:   root,
		BinaryVersion: version,
		DryRun:        dryRun,
		Skills:        true,
		Install:       installerFor(root, cfgPath),
	})
	if err != nil {
		return migrateError(err, rep, cfgPath)
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(rep)
}

// installerFor reinstalls the named agent's skills into the project at root,
// from its (by then current-format) settings. The installer's progress lines
// are discarded: migrate's stdout is its JSON report only.
func installerFor(root, cfgPath string) migrate.Installer {
	return func(name string) error {
		a, err := agent.Lookup(name)
		if err != nil {
			return err
		}
		cfg, err := config.FromYAMLFile(cfgPath)
		if err != nil {
			return err
		}
		return a.Install(root, cfg, io.Discard)
	}
}

// migrateError turns an upgrade failure into an error response whose
// next_action says how to recover. rep is the partial report Apply returned
// with the error; cfgPath is the project settings file.
func migrateError(err error, rep migrate.Report, cfgPath string) *output.ErrorResponse {
	command := migrate.PeekCommand(cfgPath)

	if fe, ok := config.IsFormatError(err); ok && fe.Newer() {
		return newerFormatError(fe)
	}

	var se *migrate.StepError
	if errors.As(err, &se) {
		if errors.Is(se, migrate.ErrNoAgent) {
			return output.NewError("migrate_failed", se.Error()).
				WithResource(se.Path).
				WithNextAction(fmt.Sprintf("run `%s init <agent>` with one of: %s, to record the project's agent and install its skills", command, strings.Join(agent.Supported(), ", ")))
		}
		next := fmt.Sprintf("fix the cause above, then re-run `%s migrate`", command)
		for _, f := range rep.Files {
			if f.Path == se.Path && f.Backup != "" {
				next += "; the previous settings are in " + f.Backup
			}
		}
		return output.NewError("migrate_failed", se.Error()).WithResource(se.Path).WithNextAction(next)
	}

	return output.NewError("migrate_failed", err.Error()).
		WithNextAction(fmt.Sprintf("check that the settings files under .spektacular are readable YAML mappings, then re-run `%s migrate`", command))
}

// newerFormatError refuses a settings file written by a newer Spektacular.
func newerFormatError(fe *config.FormatError) *output.ErrorResponse {
	return output.NewError("config_newer_format", fe.Error()).
		WithResource(fe.Path).
		WithNextAction(fmt.Sprintf("install a newer Spektacular release: this file needs settings format %d and this build supports format %d. The file has not been changed", fe.Found, fe.Want))
}

// outdatedFormatError refuses a settings file behind the current format and
// points at migrate.
func outdatedFormatError(fe *config.FormatError) *output.ErrorResponse {
	command := "spektacular"
	if p, err := configFilePath(); err == nil {
		command = migrate.PeekCommand(p)
	}
	return output.NewError("config_outdated", fe.Error()).
		WithResource(fe.Path).
		WithNextAction(fmt.Sprintf("run `%s migrate` to upgrade the project's settings (preview the changes with `%s migrate --dry-run`)", command, command))
}

// formatRefusal returns the format error response when err is, or wraps, a
// settings file in the wrong format, and nil otherwise. Callers that map
// their own errors (a footprint failure, say) check it first, so a repo.yaml
// in the wrong format points at migrate or a newer release rather than at
// repairing a footprint that is not broken.
func formatRefusal(err error) *output.ErrorResponse {
	fe, ok := config.IsFormatError(err)
	if !ok {
		return nil
	}
	if fe.Newer() {
		return newerFormatError(fe)
	}
	return outdatedFormatError(fe)
}
