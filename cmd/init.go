package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/agent"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/migrate"
	"github.com/hivecommons/spektacular/internal/output"
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

// raiseBobTurnLimitFlag opts in to init changing the user's own Bob
// settings; without it init only warns.
const raiseBobTurnLimitFlag = "raise-bob-turn-limit"

func init() {
	initCmd.Flags().String("name", "", "project name (slug-safe); defaults to the directory name")
	initCmd.Flags().Bool(raiseBobTurnLimitFlag, false, "bob only: set session.maxTurns to 0 (no limit) in your Bob settings, so Bob stops cutting off epic sub-agents")
}

func runInit(cmd *cobra.Command, args []string) error {
	a, err := agent.Lookup(args[0])
	if err != nil {
		return err
	}
	raiseBobTurnLimit, _ := cmd.Flags().GetBool(raiseBobTurnLimitFlag)
	if raiseBobTurnLimit && a.Name() != "bob" {
		return output.NewError("flag_not_applicable",
			fmt.Sprintf("--%s applies only to bob, not %s", raiseBobTurnLimitFlag, a.Name())).
			WithNextAction(fmt.Sprintf("run `spektacular init %s` without --%s", a.Name(), raiseBobTurnLimitFlag))
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

	if a.Name() == "bob" {
		return reportBobTurnLimit(cmd, raiseBobTurnLimit)
	}
	return nil
}

// reportBobTurnLimit deals with Bob Shell's per-task turn limit, which cuts
// off an epic run's sub-agents mid-step: with raise it sets the limit to 0 in
// the user's Bob settings, and otherwise it warns while any limit is in force.
// The limit is a user setting, since Bob reads no per-project session
// settings, so it is changed only when the user asks.
func reportBobTurnLimit(cmd *cobra.Command, raise bool) error {
	out := cmd.OutOrStdout()
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(out, "  Notice:   could not find your home directory to check Bob's turn limit: %v\n", err)
		return nil
	}
	path := agent.BobSettingsPath(home)
	if raise {
		if err := agent.RaiseBobTurnLimit(home); err != nil {
			return fmt.Errorf("raising Bob's turn limit: %w", err)
		}
		fmt.Fprintf(out, "  Bob:      session.maxTurns set to 0 (no limit) in %s\n", path)
		return nil
	}
	limit, err := agent.BobMaxTurns(home)
	if err != nil {
		fmt.Fprintf(out, "  Notice:   could not read Bob's turn limit: %v\n", err)
		return nil
	}
	if limit > 0 {
		fmt.Fprintf(out, "  Notice:   Bob stops every sub-agent after %d turns (session.maxTurns in %s) and hands back whatever it said last, so epic runs stall mid-step and have to be resumed. Rerun with --%s to set it to 0 (no limit), or change it yourself.\n",
			limit, path, raiseBobTurnLimitFlag)
	}
	return nil
}
