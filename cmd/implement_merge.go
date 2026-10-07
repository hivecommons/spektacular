package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/spf13/cobra"
)

var implementMergeInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name": {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64, Description: "the spec whose worktrees to merge back"},
	},
	Required: []string{"name"},
}

var implementMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge a spec's worktrees back into every repo's main line",
	Long: `Merges branch spek/<spec> into the current branch of every repo the spec's
implement run built in its own worktrees, as one unit: a dry run in every repo
first, and nothing merged anywhere if any repo would conflict, or if the branch
changes anything under a .spektacular directory. On success the spec's
worktrees, branches and worktree record are removed. On a conflict they are
kept for the user to resolve.`,
	RunE: runImplementMerge,
}

func runImplementMerge(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: implementMergeInputSchema, Output: worktreeMergeOutputSchema}, "")
	}
	m, cfg, _, err := worktreeManager()
	if err != nil {
		return err
	}
	dataStr, _ := cmd.Flags().GetString("data")
	var input struct {
		Name string `json:"name"`
	}
	if dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
			return fmt.Errorf("parsing --data: %w", err)
		}
	}
	spec := input.Name
	if spec == "" || !nameRegexp.MatchString(spec) || len(spec) > 64 {
		return output.NewError("name_required", "a spec name matching ^[a-z0-9_-]+$ is required").
			WithNextAction(fmt.Sprintf(`name the spec: %s implement merge --data '{"name":"<spec_name>"}'`, cfg.Command))
	}
	// The record is written when the worktrees are made and removed only by
	// a successful merge, so without one there is nothing to merge back.
	if _, ok, err := worktree.ReadRecord(m.ProjectRoot, spec); err != nil {
		return err
	} else if !ok {
		return output.NewError("worktree_not_found", fmt.Sprintf("%s has no worktrees to merge", spec)).
			WithResource(spec).
			WithNextAction(fmt.Sprintf("check the name with `%s spec file list`; a spec implemented with implement.worktrees off, or already merged, has nothing to merge", cfg.Command))
	}
	return writeMergeResult(cmd, m, spec)
}

func init() {
	implementMergeCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"my-feature"}')`)
	implementCmd.AddCommand(implementMergeCmd)
}
