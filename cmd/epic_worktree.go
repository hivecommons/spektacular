package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/spf13/cobra"
)

// worktreeGit is the git runner the worktree commands use; a variable so
// tests can swap in a fake, mirroring repoGit.
var worktreeGit worktree.Runner = worktree.NewRunner()

var epicWorktreeInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec": {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64, Description: "the spec whose worktrees to create or merge"},
	},
	Required: []string{"spec"},
}

var epicWorktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Give a spec its own git worktree in every repo its plan touches",
	Long: `Creates a worktree on branch spek/<spec> under .spektacular/worktrees/<spec>/
in the project's repo and in every registered repo the spec's plan names, from
each repo's current HEAD. Inside the project worktree, every touched repo then
resolves to the spec's worktrees. Existing worktrees are returned as they are.`,
	RunE: runEpicWorktree,
}

var epicMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge a finished spec's worktrees back into every repo's main line",
	Long: `Merges branch spek/<spec> into the current branch of every repo the spec
touched, as one unit: a dry run in every repo first, and nothing merged anywhere
if any repo would conflict. On success the spec's worktrees and branches are
removed.`,
	RunE: runEpicMerge,
}

// worktreeSpec reads and validates the {"spec":...} input both commands take.
func worktreeSpec(cmd *cobra.Command, command, verb string) (string, error) {
	dataStr, _ := cmd.Flags().GetString("data")
	var input struct {
		Spec string `json:"spec"`
	}
	if dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
			return "", fmt.Errorf("parsing --data: %w", err)
		}
	}
	if input.Spec == "" || !nameRegexp.MatchString(input.Spec) || len(input.Spec) > 64 {
		return "", output.NewError("spec_required", "a spec name matching ^[a-z0-9_-]+$ is required").
			WithNextAction(fmt.Sprintf(`name the spec: %s epic %s --data '{"spec":"<spec_name>"}'`, command, verb))
	}
	return input.Spec, nil
}

// worktreeManager builds the manager for the project in the current folder.
func worktreeManager() (worktree.Manager, config.Config, store.Store, error) {
	root, err := projectRoot()
	if err != nil {
		return worktree.Manager{}, config.Config{}, nil, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return worktree.Manager{}, config.Config{}, nil, err
	}
	set, err := repo.New(cfg, root, repoGit)
	if err != nil {
		return worktree.Manager{}, config.Config{}, nil, err
	}
	m := worktree.Manager{ProjectRoot: root, Config: cfg, Repos: set, Git: worktreeGit}
	return m, cfg, store.NewSourceStore(root, "project"), nil
}

func runEpicWorktree(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: epicWorktreeInputSchema, Output: &schemaObj{
			Type: "object",
			Properties: map[string]*schemaProp{
				"spec":    {Type: "string"},
				"project": {Type: "string", Description: "the project root inside the spec's project worktree: where its implement run goes"},
				"branch":  {Type: "string"},
				"repos":   {Type: "array", Description: "every touched repo's worktree, the project repo first: {repo, path}"},
				"created": {Type: "boolean", Description: "false when every worktree already existed"},
			},
		}}, "")
	}
	m, cfg, st, err := worktreeManager()
	if err != nil {
		return err
	}
	spec, err := worktreeSpec(cmd, cfg.Command, "worktree")
	if err != nil {
		return err
	}
	if _, err := readSpecFile(st, cfg, spec); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return output.NewError("spec_not_found", fmt.Sprintf("no spec named %q is stored in this project", spec)).
				WithResource(spec).
				WithNextAction(fmt.Sprintf("check the name; run `%s spec file list` to see the stored specs", cfg.Command))
		}
		return err
	}
	touched, err := worktree.TouchedRepos(cfg, st, spec)
	if err != nil {
		return err
	}
	sw, created, err := m.Ensure(spec, touched)
	if err != nil {
		return err
	}
	repos := make([]map[string]string, len(sw.Repos))
	for i, r := range sw.Repos {
		repos[i] = map[string]string{"repo": r.Repo, "path": r.Path}
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(map[string]any{
		"spec":    sw.Spec,
		"project": sw.Project,
		"branch":  worktree.Branch(spec),
		"repos":   repos,
		"created": created,
	})
}

func runEpicMerge(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: epicWorktreeInputSchema, Output: &schemaObj{
			Type: "object",
			Properties: map[string]*schemaProp{
				"spec":    {Type: "string"},
				"merged":  {Type: "boolean"},
				"removed": {Type: "boolean", Description: "every worktree and branch of the spec was removed"},
			},
		}}, "")
	}
	m, cfg, _, err := worktreeManager()
	if err != nil {
		return err
	}
	spec, err := worktreeSpec(cmd, cfg.Command, "merge")
	if err != nil {
		return err
	}
	res, err := m.Merge(spec)
	if err != nil {
		return err
	}
	if len(res.Conflicts) > 0 {
		return mergeConflict(spec, res.Conflicts)
	}
	return output.New(cmd.OutOrStdout(), globalFields).WriteResult(map[string]any{
		"spec":    spec,
		"merged":  res.Merged,
		"removed": res.Removed,
	})
}

// mergeConflict refuses a merge that would conflict, listing the conflicted
// paths per repo. Nothing was merged in any repo.
func mergeConflict(spec string, conflicts map[string][]string) error {
	repos := make([]string, 0, len(conflicts))
	for r := range conflicts {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	parts := make([]string, len(repos))
	for i, r := range repos {
		parts[i] = fmt.Sprintf("%s: %s", r, strings.Join(conflicts[r], ", "))
	}
	return output.NewError("epic_merge_conflict",
		fmt.Sprintf("merging %s would conflict, so nothing was merged in any repo; conflicting paths — %s", spec, strings.Join(parts, "; "))).
		WithResource(spec).
		WithNextAction("show the user the conflicting paths for each repo and stop the run; do not resolve the conflict yourself — the spec's worktrees and branch spek/" + spec + " are left in place for the user")
}

func init() {
	epicWorktreeCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"spec":"000071_example"}')`)
	epicMergeCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"spec":"000071_example"}')`)
	epicCmd.AddCommand(epicWorktreeCmd, epicMergeCmd)
}
