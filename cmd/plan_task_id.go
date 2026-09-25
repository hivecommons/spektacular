package cmd

import (
	"github.com/hivecommons/spektacular/internal/identifier"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/spf13/cobra"
)

// planTaskIDCmd issues one task identifier from the configured provider. Plan
// authors, the plan workflow agent and people alike, ask for an id here for
// every new task instead of inventing one.
var planTaskIDCmd = &cobra.Command{
	Use:   "task-id",
	Short: "Issue a new plan task identifier",
	Args:  cobra.NoArgs,
	RunE:  runPlanTaskID,
}

// taskIDResult is the output of `plan task-id`.
type taskIDResult struct {
	ID string `json:"id"`
}

func runPlanTaskID(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Output: &schemaObj{
				Type:       "object",
				Properties: map[string]*schemaProp{"id": {Type: "string"}},
			},
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	provider, err := identifier.TaskIDProviderFor(cfg.Plan.TaskID.Provider)
	if err != nil {
		return err
	}
	id, err := provider()
	if err != nil {
		return err
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(taskIDResult{ID: id})
}

func init() {
	planCmd.AddCommand(planTaskIDCmd)
}
