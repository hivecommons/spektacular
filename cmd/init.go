package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/agent"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/migrate"
	"github.com/hivecommons/spektacular/internal/project"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:         "init <agent>",
	Short:       "Initialise a Spektacular project for the specified agent (" + strings.Join(agent.Supported(), ", ") + ")",
	Args:        cobra.ExactArgs(1),
	Annotations: map[string]string{gateAnnotation: gateExempt},
	RunE:        runInit,
}

func init() {
	initCmd.Flags().String("name", "", "project name (slug-safe); defaults to the directory name")
}

func runInit(cmd *cobra.Command, args []string) error {
	a, err := agent.Lookup(args[0])
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Bring an existing project's settings to the current format before
	// setup reads them. Skills are left to the install below.
	cfgPath := filepath.Join(config.ProjectConfigDir(cwd), config.ProjectConfigFileName)
	if _, err := os.Stat(cfgPath); err == nil {
		rep, err := migrate.Apply(migrate.Options{ProjectRoot: cwd, BinaryVersion: version})
		if err != nil {
			return migrateError(err, rep, cfgPath)
		}
	}

	name, _ := cmd.Flags().GetString("name")
	notices, err := project.Init(cwd, name, true)
	if err != nil {
		return fmt.Errorf("initialising project: %w", err)
	}

	// Init is a bootstrap command: it must load config leniently, since it
	// runs precisely where no project exists yet.
	cfg, err := loadConfigLenient()
	if err != nil {
		return err
	}

	cfg.Agent = a.Name()
	if err := cfg.ToYAMLFile(cfgPath); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Spektacular initialised for %s.\n", a.Name())
	fmt.Fprintf(cmd.OutOrStdout(), "  Project:  %s\n", config.ProjectConfigDir(cwd))
	fmt.Fprintf(cmd.OutOrStdout(), "  Skills:   %s\n", version)
	for _, n := range notices {
		fmt.Fprintf(cmd.OutOrStdout(), "  Notice:   %s\n", n)
	}

	if err := a.Install(cwd, cfg, cmd.OutOrStdout()); err != nil {
		return err
	}

	// The installed skills version is recorded only once the install has
	// succeeded; it replaces the standalone version file older projects kept.
	cfg.SkillsVersion = version
	if err := cfg.ToYAMLFile(cfgPath); err != nil {
		return fmt.Errorf("recording installed skills version: %w", err)
	}
	legacy := filepath.Join(config.ProjectConfigDir(cwd), "version")
	if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing legacy version file %s: %w", legacy, err)
	}
	return nil
}
