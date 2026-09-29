package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/research"
	"github.com/spf13/cobra"
)

// knowledgeResearchDefaultLimit bounds each source's findings so a seed stays
// a starting point for the interview rather than a dump.
const knowledgeResearchDefaultLimit = 5

var knowledgeResearchLimit int

var knowledgeResearchCmd = &cobra.Command{
	Use:   "research <query>",
	Short: "Seed spec research from every knowledge source: the knowledge stores, repo ADR folders, the Hive knowledge export and Context7",
	// The query is positional but --schema takes none, so the count is checked
	// in the run function, as search does.
	Args: cobra.MaximumNArgs(1),
	RunE: runKnowledgeResearch,
}

var knowledgeResearchFlags = map[string]*schemaProp{
	"limit": {Type: "integer"},
}

var knowledgeResearchOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"query": {Type: "string"},
		"sources": {
			Type: "array",
			Items: &schemaProp{
				Type: "object",
				Properties: map[string]*schemaProp{
					"source": {Type: "string", Enum: []string{"knowledge", "adr", "hive", "context7"}},
					"status": {Type: "string", Enum: []string{string(research.StatusUsed), string(research.StatusSkipped), string(research.StatusUnavailable)}},
					"reason": {Type: "string"},
					"findings": {
						Type: "array",
						Items: &schemaProp{
							Type: "object",
							Properties: map[string]*schemaProp{
								"title":    {Type: "string"},
								"cite":     {Type: "string"},
								"excerpts": {Type: "array", Items: &schemaProp{Type: "string"}},
								"tags":     {Type: "array", Items: &schemaProp{Type: "string"}},
								"score":    {Type: "number"},
								"read":     {Type: "string"},
							},
						},
					},
				},
			},
		},
	},
}

func runKnowledgeResearch(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeResearchOutputSchema, Flags: knowledgeResearchFlags}, "")
	}
	var terms []string
	if len(args) == 1 {
		terms = knowledge.Terms(args[0])
	}
	if len(terms) == 0 {
		return output.NewError("knowledge_query_required", "research requires a query").
			WithNextAction(`reissue with the feature's key terms as a positional argument, e.g. knowledge research "plan export jira"`)
	}
	if knowledgeResearchLimit < 1 {
		return output.NewError("knowledge_limit_invalid", fmt.Sprintf("--limit must be at least 1, got %d", knowledgeResearchLimit)).
			WithNextAction(fmt.Sprintf("omit --limit to use the default of %d findings per source", knowledgeResearchDefaultLimit))
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	repos, err := repo.New(cfg, cwd, repoGit)
	if err != nil {
		return err
	}
	// ADRs live with a repo's code, so each repo contributes the root its
	// code is on disk at — never its registered footprint location, and
	// never by cloning: a repo whose code is not on disk has none to search.
	var adrRepos []research.ADRRepo
	for _, e := range repos.Entries() {
		if root, ok := repos.LocalSource(e.Name); ok {
			adrRepos = append(adrRepos, research.ADRRepo{Name: e.Name, Root: root})
		}
	}

	sources := []research.Source{
		research.KnowledgeSource{Set: set, Command: cfg.Command},
		research.ADRSource{Repos: adrRepos},
		research.NewHiveSource(),
		research.NewContext7Source(),
	}
	reports := research.Seed(context.Background(), sources, terms, knowledgeResearchLimit)

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"query": args[0], "sources": reports})
}

func init() {
	knowledgeResearchCmd.Flags().IntVar(&knowledgeResearchLimit, "limit", knowledgeResearchDefaultLimit, "Maximum findings reported per source")
	knowledgeCmd.AddCommand(knowledgeResearchCmd)
}
