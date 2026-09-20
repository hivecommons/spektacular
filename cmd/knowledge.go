package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/knowledge"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/repo"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/spf13/cobra"
)

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Search, read, list, and write across the configured knowledge stores",
	RunE:  runUnknownSubcommand,
}

var knowledgeSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the knowledge stores a request covers, returning ranked, one-per-document results",
	// The query is positional but --schema takes none, so the count is checked
	// in the run function rather than here: a caller must be able to discover
	// this command's interface without already knowing how to invoke it.
	Args: cobra.MaximumNArgs(1),
	RunE: runKnowledgeSearch,
}

var knowledgeReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read a knowledge entry from one addressed store",
	RunE:  runKnowledgeRead,
}

var knowledgeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every knowledge entry across the stores a request covers",
	RunE:  runKnowledgeList,
}

var knowledgeWriteCmd = &cobra.Command{
	Use:   "write",
	Short: "Write a knowledge entry into one addressed store",
	RunE:  runKnowledgeWrite,
}

var knowledgeSourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List the configured knowledge stores by tier and name, with their locations",
	RunE:  runKnowledgeSources,
}

var knowledgeConventionsCmd = &cobra.Command{
	Use:   "conventions",
	Short: "Read every always-apply convention across the stores a request covers",
	RunE:  runKnowledgeConventions,
}

var knowledgeCategoriesCmd = &cobra.Command{
	Use:   "categories",
	Short: "List the knowledge categories and their definitions, tiers, and entry shapes",
	RunE:  runKnowledgeCategories,
}

var knowledgeTagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List the tag vocabulary already in use, most-used first, across the stores a request covers",
	RunE:  runKnowledgeTags,
}

// knowledgeTier and knowledgeFilter back the --tier and --filter options
// shared by search, list, conventions and always-applied. They replace the
// narrower --repo option, which could only name repo-tier stores and silently
// exempted every other store from the narrowing it expressed.
var (
	knowledgeTier   string
	knowledgeFilter []string
)

// knowledgeTags backs the --tag option, which narrows a search to entries
// carrying every tag listed. It is registered on search alone: the other
// fan-out commands enumerate stores rather than entries, so an entry-level
// filter has nothing to bind to there.
var knowledgeTags []string

var knowledgeAlwaysAppliedCmd = &cobra.Command{
	Use:   "always-applied",
	Short: "Read every always-applied entry (conventions and glossary) across the stores a request covers",
	RunE:  runKnowledgeAlwaysApplied,
}

// knowledgeAddressProps are the two fields every knowledge result carries to
// say where it came from. Declared once so no schema can describe the address
// differently from its neighbours.
var knowledgeAddressProps = map[string]*schemaProp{
	"tier": {Type: "string", Enum: []string{string(knowledge.TierProject), string(knowledge.TierRepo)}},
	"name": {Type: "string"},
}

// knowledgeItemSchema builds an array-of-objects property whose items carry the
// address plus the given extra fields.
func knowledgeItemSchema(extra map[string]*schemaProp) *schemaProp {
	props := make(map[string]*schemaProp, len(knowledgeAddressProps)+len(extra))
	for k, v := range knowledgeAddressProps {
		props[k] = v
	}
	for k, v := range extra {
		props[k] = v
	}
	return &schemaProp{Type: "array", Items: &schemaProp{Type: "object", Properties: props}}
}

// knowledgeNarrowingFlags describes the options the fan-out commands take on
// the command line rather than in --data.
var knowledgeNarrowingFlags = map[string]*schemaProp{
	"tier":   {Type: "string", Enum: []string{string(knowledge.TierProject), string(knowledge.TierRepo), string(knowledge.TierAll)}},
	"filter": {Type: "array", Items: &schemaProp{Type: "string"}},
}

// knowledgeSearchFlags is the schema search advertises. Search takes one option
// the other fan-out commands do not, so it cannot share knowledgeNarrowingFlags:
// adding --tag there would advertise a flag the other three silently ignore.
var knowledgeSearchFlags = func() map[string]*schemaProp {
	flags := make(map[string]*schemaProp, len(knowledgeNarrowingFlags)+1)
	for name, prop := range knowledgeNarrowingFlags {
		flags[name] = prop
	}
	flags["tag"] = &schemaProp{Type: "array", Items: &schemaProp{Type: "string"}}
	return flags
}()

var knowledgeSearchOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"hits": {
			Type: "array",
			Items: &schemaProp{
				Type: "object",
				Properties: map[string]*schemaProp{
					"tier":     {Type: "string"},
					"name":     {Type: "string"},
					"path":     {Type: "string"},
					"title":    {Type: "string"},
					"score":    {Type: "number"},
					"category": {Type: "string"},
					"checksum": {Type: "string"},
					"excerpts": {Type: "array", Items: &schemaProp{Type: "string"}},
					"tags":     {Type: "array", Items: &schemaProp{Type: "string"}},
				},
			},
		},
	},
}

var knowledgeTagsOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"tags": {
			Type: "array",
			Items: &schemaProp{
				Type: "object",
				Properties: map[string]*schemaProp{
					"tag":   {Type: "string"},
					"count": {Type: "number"},
				},
			},
		},
	},
}

var knowledgeReadOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"tier":    {Type: "string"},
		"name":    {Type: "string"},
		"path":    {Type: "string"},
		"content": {Type: "string"},
	},
}

var knowledgeListOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"entries": knowledgeItemSchema(map[string]*schemaProp{"path": {Type: "string"}}),
	},
}

var knowledgeWriteOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"tier": {Type: "string"},
		"name": {Type: "string"},
		"path": {Type: "string"},
	},
}

var knowledgeSourcesOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"sources": knowledgeItemSchema(map[string]*schemaProp{
			"provider": {Type: "string"},
			"location": {Type: "string"},
		}),
	},
}

var knowledgeConventionsOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"conventions": knowledgeItemSchema(map[string]*schemaProp{
			"path":    {Type: "string"},
			"content": {Type: "string"},
		}),
	},
}

var knowledgeCategoriesOutputSchema = &schemaObj{
	Type:       "object",
	Properties: map[string]*schemaProp{"categories": {Type: "array"}},
}

var knowledgeAlwaysAppliedOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"entries": knowledgeItemSchema(map[string]*schemaProp{
			"path":     {Type: "string"},
			"content":  {Type: "string"},
			"category": {Type: "string"},
		}),
	},
}

var knowledgeAddressInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"tier": {Type: "string", Enum: []string{string(knowledge.TierProject), string(knowledge.TierRepo)}},
		"name": {Type: "string"},
		"path": {Type: "string"},
	},
	Required: []string{"tier", "name", "path"},
}

// newKnowledgeSet builds a knowledge.Set from the aggregated knowledge
// configuration: every registered repo's declared sources in registry order
// (the colocated repo first by construction), followed by any project-owned
// sources from the project config. A repo's knowledge travels with the repo
// into every project that registers it.
func newKnowledgeSet() (*knowledge.Set, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	sources, err := aggregateKnowledgeSources(cfg, cwd)
	if err != nil {
		return nil, err
	}
	cfg.Knowledge = config.KnowledgeConfig{Sources: sources}

	return knowledge.NewSet(cfg, cwd)
}

// aggregateKnowledgeSources builds the effective knowledge store list for the
// project: each registered repo's declared sources (its repo.yaml, read from the
// repo's on-disk root; registry order, so the colocated repo comes first), then
// the project's own shared sources declared in the project config. Repos not
// materialized locally are skipped — their knowledge joins the set once the repo
// is on disk. Relative source locations in a repo's config resolve against that
// repo's root.
//
// This is where a store's identity is established: a repo's contribution is the
// repo tier under that repo's registry name, and the project's contributions are
// the project tier under their own declared names. Names are checked unique
// within a tier here, which is the one place both tiers are visible at once.
func aggregateKnowledgeSources(cfg config.Config, projectRoot string) ([]config.SourceConfig, error) {
	set, err := repo.New(cfg, projectRoot, repoGit)
	if err != nil {
		return nil, err
	}

	var sources []config.SourceConfig
	for _, e := range set.Entries() {
		root, _ := set.LocalRoot(e.Name)
		rc, err := set.Footprint(e.Name)
		if err != nil {
			// A footprint that is unreadable or absent is repairable by
			// re-adding the repo, but a footprint the loader deliberately
			// refused already carries its own remediation naming the file and
			// the block required. Offering "repair the footprint" over the top
			// of that would send the caller somewhere that cannot fix it.
			var refusal *output.ErrorResponse
			if errors.As(err, &refusal) {
				return nil, refusal
			}
			if refusal := formatRefusal(err); refusal != nil {
				return nil, refusal
			}
			var fpErr *repo.FootprintError
			if errors.As(err, &fpErr) {
				return nil, output.NewError(
					"repo_footprint",
					fmt.Sprintf("repo %q at %s has a missing or invalid footprint: %v", e.Name, fpErr.Root, fpErr.Err),
				).WithResource(filepath.Join(fpErr.Root, config.RepoConfigFileName)).
					WithNextAction(fmt.Sprintf("run `%s repo add --data '{\"name\":%q,\"location\":%q}'` to repair the repo's footprint", cfg.Command, e.Name, e.Location))
			}
			return nil, err
		}

		kc := rc.WithDefaults(root).Knowledge
		src := config.SourceConfig{
			Tier:     string(knowledge.TierRepo),
			Name:     e.Name,
			Provider: kc.Provider,
			Config:   kc.Config,
		}
		if src.Provider == config.ProviderFile && !filepath.IsAbs(src.Config.Location) {
			src.Config.Location = filepath.Join(root, src.Config.Location)
		}
		sources = append(sources, src)
	}

	for _, src := range cfg.Knowledge.Sources {
		src.Tier = string(knowledge.TierProject)
		sources = append(sources, src)
	}

	if err := requireUniqueStoreNames(sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// requireUniqueStoreNames rejects two stores sharing a name within one tier,
// which would make an address ambiguous. The same name in each tier is fine:
// a tier plus a name is the identity, not the name alone. Repo names are
// already unique through the registry, so in practice this catches duplicate
// shared-store names and stands as a second line of defence for the rest.
func requireUniqueStoreNames(sources []config.SourceConfig) error {
	seen := make(map[string]bool, len(sources))
	for _, src := range sources {
		key := src.Tier + ":" + src.Name
		if seen[key] {
			return output.NewError(
				"knowledge_store_name_duplicate",
				fmt.Sprintf("two knowledge stores are named %q in the %q tier", src.Name, src.Tier),
			).WithNextAction(fmt.Sprintf("give each store in the %q tier a unique name, then re-run", src.Tier))
		}
		seen[key] = true
	}
	return nil
}

func runKnowledgeSearch(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeSearchOutputSchema, Flags: knowledgeSearchFlags}, "")
	}
	if len(args) == 0 {
		return output.NewError("knowledge_query_required", "search requires a query").
			WithNextAction(`reissue with the query as a positional argument, e.g. knowledge search "database timeout" --tier repo`)
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	hits, err := set.Search(args[0], knowledgeSelector())
	if err != nil {
		return err
	}
	if hits == nil {
		hits = []store.Hit{}
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"hits": hits})
}

func runKnowledgeRead(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: knowledgeAddressInputSchema, Output: knowledgeReadOutputSchema}, "")
	}
	input, err := knowledgeAddressData(cmd)
	if err != nil {
		return err
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	content, err := set.Read(input.Address(), input.Path)
	if err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{
		"tier":    input.Tier,
		"name":    input.Name,
		"path":    input.Path,
		"content": string(content),
	})
}

func runKnowledgeList(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeListOutputSchema, Flags: knowledgeNarrowingFlags}, "")
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	entries, err := set.List(knowledgeSelector())
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []knowledge.Entry{}
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"entries": entries})
}

func runKnowledgeTags(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeTagsOutputSchema, Flags: knowledgeNarrowingFlags}, "")
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	tags, err := set.Tags(knowledgeSelector())
	if err != nil {
		return err
	}
	if tags == nil {
		tags = []knowledge.TagUse{}
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"tags": tags})
}

func runKnowledgeWrite(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: knowledgeAddressInputSchema, Output: knowledgeWriteOutputSchema}, "")
	}
	input, err := knowledgeAddressData(cmd)
	if err != nil {
		return err
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	content, err := readKnowledgeContent(cmd)
	if err != nil {
		return err
	}
	if err := set.Write(input.Address(), input.Path, content); err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"tier": input.Tier, "name": input.Name, "path": input.Path})
}

func runKnowledgeSources(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeSourcesOutputSchema}, "")
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"sources": set.Sources()})
}

func runKnowledgeConventions(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeConventionsOutputSchema, Flags: knowledgeNarrowingFlags}, "")
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	conventions, err := set.Conventions(knowledgeSelector())
	if err != nil {
		return err
	}
	if conventions == nil {
		conventions = []knowledge.Convention{}
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"conventions": conventions})
}

func runKnowledgeCategories(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeCategoriesOutputSchema}, "")
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"categories": knowledge.Categories})
}

func runKnowledgeAlwaysApplied(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: knowledgeAlwaysAppliedOutputSchema, Flags: knowledgeNarrowingFlags}, "")
	}
	set, err := newKnowledgeSet()
	if err != nil {
		return err
	}
	entries, err := set.AlwaysAppliedEntries(knowledgeSelector())
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []knowledge.AlwaysAppliedEntry{}
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"entries": entries})
}

// knowledgeSelector builds the selector the fan-out commands narrow on from
// the --tier and --filter options. The set validates it: an unrecognised tier
// or a filter naming a store the tier does not reach is refused there, with the
// names available, which is information only the set holds.
func knowledgeSelector() knowledge.Selector {
	// Tags are lower-cased to match how they are normalised when read off an
	// entry, so --tag HTTP finds an entry tagged http.
	var tags []string
	for _, tag := range knowledgeTags {
		if tag = strings.ToLower(strings.TrimSpace(tag)); tag != "" {
			tags = append(tags, tag)
		}
	}
	return knowledge.Selector{Tier: knowledge.Tier(knowledgeTier), Filter: knowledgeFilter, Tags: tags}
}

// knowledgeAddressInput is the --data payload for the read and write commands:
// the store's address (tier and name) plus the entry's path within it.
type knowledgeAddressInput struct {
	Tier knowledge.Tier `json:"tier"`
	Name string         `json:"name"`
	Path string         `json:"path"`
}

// Address returns the store address the input names.
func (i knowledgeAddressInput) Address() knowledge.Address {
	return knowledge.Address{Tier: i.Tier, Name: i.Name}
}

// knowledgeAddressData parses the --data flag shared by the read and write
// subcommands and checks the entry path. The address itself is deliberately
// left for the knowledge set to validate: only the set knows which stores are
// configured, so refusing here would cost the caller the list of names it needs
// to reissue the request. Read and Write both refuse before touching a store,
// so nothing is recorded either way.
func knowledgeAddressData(cmd *cobra.Command) (knowledgeAddressInput, error) {
	dataStr, _ := cmd.Flags().GetString("data")
	if dataStr == "" {
		return knowledgeAddressInput{}, fmt.Errorf(`--data is required (e.g. --data '{"tier":"repo","name":"docs","path":"learnings/x.md"}')`)
	}
	var input knowledgeAddressInput
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return knowledgeAddressInput{}, fmt.Errorf("parsing --data: %w", err)
	}
	if input.Path == "" {
		return knowledgeAddressInput{}, output.NewError(
			"knowledge_path_required",
			`--data must include a non-empty "path"`,
		).WithNextAction(`reissue with "path" set to the entry's location within the store, e.g. "conventions/naming.md"`)
	}
	return input, nil
}

// readKnowledgeContent reads the entry body for a write: from the --file path
// when set, otherwise from standard input.
func readKnowledgeContent(cmd *cobra.Command) ([]byte, error) {
	filePath, _ := cmd.Flags().GetString("file")
	if filePath != "" {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", filePath, err)
		}
		return content, nil
	}
	content, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	return content, nil
}

func init() {
	knowledgeCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")

	knowledgeReadCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"tier":"repo","name":"docs","path":"learnings/x.md"}')`)
	knowledgeWriteCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"tier":"repo","name":"docs","path":"learnings/x.md"}')`)
	knowledgeWriteCmd.Flags().String("file", "", "Read entry content from the file at <path> (relative to cwd); stdin is used when omitted")
	for _, c := range []*cobra.Command{knowledgeSearchCmd, knowledgeListCmd, knowledgeConventionsCmd, knowledgeAlwaysAppliedCmd, knowledgeTagsCmd} {
		c.Flags().StringVar(&knowledgeTier, "tier", string(knowledge.TierAll), `Which knowledge to cover: "project", "repo", or "all"`)
		c.Flags().StringArrayVar(&knowledgeFilter, "filter", nil, "Narrow to the named store(s) within the tier (repeatable); omit to cover every store the tier reaches")
	}

	knowledgeSearchCmd.Flags().StringArrayVar(&knowledgeTags, "tag", nil, "Narrow to entries carrying the tag (repeatable); an entry must carry every tag listed, and one lacking any of them is never returned")

	knowledgeCmd.AddCommand(knowledgeSearchCmd, knowledgeReadCmd, knowledgeListCmd, knowledgeWriteCmd, knowledgeSourcesCmd, knowledgeConventionsCmd, knowledgeCategoriesCmd, knowledgeAlwaysAppliedCmd, knowledgeTagsCmd)
}
