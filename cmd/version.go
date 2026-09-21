package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/migrate"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/spf13/cobra"
)

// VersionCheckResult is returned by the version check command. The
// "error": false discriminant is injected by the output writer — never
// declared on the struct.
type VersionCheckResult struct {
	Status           string `json:"status"`                      // "match" | "mismatch" | "missing" | "upgrade_needed" | "unsupported_format"
	InstalledVersion string `json:"installed_version,omitempty"` // the installed skills version from project settings (or the legacy version file); empty when missing
	CurrentVersion   string `json:"current_version"`             // the running binary's version
	Action           string `json:"action,omitempty"`            // set on any non-match: instruction to relay to the user
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Report and check the installed Spektacular version",
	RunE:  runUnknownSubcommand,
}

var versionCheckCmd = &cobra.Command{
	Use:         "check",
	Short:       "Check whether the project's settings and installed files match the current binary version",
	Annotations: map[string]string{gateAnnotation: gateExempt},
	RunE:        runVersionCheck,
}

func runVersionCheck(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type:       "object",
				Properties: map[string]*schemaProp{},
			},
			Output: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"status":            {Type: "string", Enum: []string{"match", "mismatch", "missing", "upgrade_needed", "unsupported_format"}},
					"installed_version": {Type: "string"},
					"current_version":   {Type: "string"},
					"action":            {Type: "string"},
				},
			},
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	root, err := projectRoot()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(config.ProjectConfigDir(root), config.ProjectConfigFileName)
	command := migrate.PeekCommand(cfgPath)
	result := VersionCheckResult{CurrentVersion: version}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		result.Status = "missing"
		result.Action = fmt.Sprintf("no Spektacular project is set up here — ask the user to run `%s init <agent>` to set one up", command)
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(result)
	}

	rep, err := migrate.Inspect(root, version)
	if fe, ok := config.IsFormatError(err); ok && fe.Newer() {
		result.Status = "unsupported_format"
		result.Action = fmt.Sprintf("the project's settings were written by a newer Spektacular (%s needs settings format %d; this build supports format %d) — ask the user to update Spektacular to a newer release before continuing", fe.Path, fe.Found, fe.Want)
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(result)
	}
	if err != nil {
		return output.NewError("migration_check_failed", fmt.Sprintf("checking whether the project needs an upgrade: %v", err)).
			WithResource(cfgPath).
			WithNextAction("Verify the .spektacular directory structure and permissions, and that its settings files are readable YAML.")
	}

	result.InstalledVersion = rep.Skills.Installed
	result.Status = rep.Skills.Status
	if len(rep.Files) > 0 {
		result.Status = "upgrade_needed"
	}
	if result.Status != "match" {
		result.Action = upgradeAction(result.Status, command)
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(result)
}

// upgradeAction composes the instruction an agent relays to the user when
// the project is out of date. Every out-of-date status is fixed by migrate.
func upgradeAction(status, command string) string {
	reason := "the installed Spektacular skills are out of date for this Spektacular"
	switch status {
	case "missing":
		reason = "the installed Spektacular skills have no recorded version"
	case "upgrade_needed":
		reason = "the project's settings are in an older format than this Spektacular uses"
	}
	return fmt.Sprintf("%s — ask the user to run `%s migrate` to upgrade (they can preview the changes with `%s migrate --dry-run`)", reason, command, command)
}

func init() {
	versionCheckCmd.Flags().Bool("schema", false, "Print the input/output schema and exit")
	versionCmd.AddCommand(versionCheckCmd)
}
