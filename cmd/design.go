package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jumppad-labs/spektacular/internal/design"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/spf13/cobra"
)

// The design commands deliberately do NOT go through newStoreFileCmd, the
// factory that backs `spec file`, `plan file` and `changelog file`. The verbs
// match and the output conventions match, so a reader will reasonably expect
// design documents to be a fourth registration of it. They are not, for one
// concrete reason: that factory merges Spektacular's own lifecycle frontmatter
// into every document it writes and re-reads that block on every listing
// (cmd/storefile.go), and a design document belongs to the team. Nothing
// Spektacular writes may add, remove or reformat any part of it. These
// commands are therefore hand-written against internal/design, and the
// divergence is signposted here so a maintainer does not try to consolidate
// them.

var designCmd = &cobra.Command{
	Use:   "design",
	Short: "List, read, write and reference the project's design documents",
	RunE:  runUnknownSubcommand,
}

var designSourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List the design sources the project declares, with their resolved locations",
	RunE:  runDesignSources,
}

var designListCmd = &cobra.Command{
	Use:   "list",
	Short: "List design documents across every declared source, or one source with --source",
	RunE:  runDesignList,
}

var designReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read one design document, writing its bytes to stdout unchanged",
	RunE:  runDesignRead,
}

var designWriteCmd = &cobra.Command{
	Use:   "write",
	Short: "Write one design document into a declared source, byte for byte",
	RunE:  runDesignWrite,
}

// designSource is the --source flag shared by the list command.
var designSource string

// designAddressInput is the --data payload for the read and write commands: a
// declared source's name plus the document's path within it.
//
// There is no tier. Design sources are a project-level declaration only, so a
// source's identity is its name alone, and the two-tier addressing the
// knowledge commands need has nothing to act on here.
type designAddressInput struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

// Document returns the document the input addresses.
func (i designAddressInput) Document() design.Document {
	return design.Document{Source: i.Source, Path: i.Path}
}

var designAddressInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"source": {Type: "string"},
		"path":   {Type: "string"},
	},
	Required: []string{"source", "path"},
}

var designSourceItemSchema = map[string]*schemaProp{
	"name":     {Type: "string"},
	"provider": {Type: "string"},
	"location": {Type: "string"},
}

var designSourcesOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"sources": {Type: "array", Items: &schemaProp{Type: "object", Properties: designSourceItemSchema}},
	},
}

var designDocumentItemSchema = map[string]*schemaProp{
	"source": {Type: "string"},
	"path":   {Type: "string"},
}

var designListOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"documents": {Type: "array", Items: &schemaProp{Type: "object", Properties: designDocumentItemSchema}},
	},
}

// designReadOutputSchema describes a string rather than an object, because
// `design read` writes the document's raw bytes to stdout and not a JSON
// envelope. Publishing an object with a "content" field here would advertise a
// shape the command never produces, which is worse than publishing nothing: an
// agent that trusts the schema would try to parse the document as JSON.
var designReadOutputSchema = &schemaObj{Type: "string"}

var designWriteOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"source":   {Type: "string"},
		"path":     {Type: "string"},
		"location": {Type: "string"},
	},
}

// designAddressData parses the --data flag shared by the read and write
// subcommands.
//
// Neither half of the address is checked here beyond being present: only the
// design set knows which sources are declared, so refusing an unknown source
// at this layer would cost the caller the list of names it needs to reissue
// the request. The set refuses before touching a store, so nothing is written
// either way.
func designAddressData(cmd *cobra.Command) (designAddressInput, error) {
	dataStr, _ := cmd.Flags().GetString("data")
	if dataStr == "" {
		return designAddressInput{}, output.NewError(
			"design_data_required",
			"--data is required",
		).WithNextAction(`reissue with the document's address, e.g. --data '{"source":"api","path":"payments/v2.md"}'; run 'design sources' to see the declared source names`)
	}
	var input designAddressInput
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return designAddressInput{}, fmt.Errorf("parsing --data: %w", err)
	}
	return input, nil
}

// newDesignSet builds the design set from the project's settings. Every design
// command starts here, so an unreachable or unsupported source is refused
// identically whichever verb was run.
func newDesignSet() (*design.Set, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	return design.NewSet(cfg, cwd)
}

func runDesignSources(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: designSourcesOutputSchema}, "")
	}
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	sources := set.Sources()
	items := make([]map[string]any, 0, len(sources))
	for _, s := range sources {
		items = append(items, map[string]any{
			"name":     s.Name,
			"provider": s.Provider,
			"location": s.Location,
		})
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"sources": items})
}

func runDesignList(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  nil,
			Output: designListOutputSchema,
			Flags: map[string]*schemaProp{
				"source": {Type: "string"},
			},
		}, "")
	}
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	docs, err := set.List(designSource)
	if err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		items = append(items, map[string]any{"source": d.Source, "path": d.Path})
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"documents": items})
}

// runDesignRead writes the document's raw bytes to stdout rather than a JSON
// envelope, matching `spec file read` and `plan file read`. A design document
// is of arbitrary length and its address was supplied by the caller, so there
// is nothing to return alongside it and escaping a whole document into a JSON
// string would only make it harder to feed back into `design write --from`.
func runDesignRead(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: designAddressInputSchema, Output: designReadOutputSchema}, "")
	}
	input, err := designAddressData(cmd)
	if err != nil {
		return err
	}
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	content, err := set.Read(input.Document())
	if err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write(content)
	return err
}

// runDesignWrite stores the bytes of the file named by --from, unchanged.
func runDesignWrite(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  designAddressInputSchema,
			Output: designWriteOutputSchema,
			Flags: map[string]*schemaProp{
				"from": {Type: "string"},
			},
		}, "")
	}
	input, err := designAddressData(cmd)
	if err != nil {
		return err
	}
	fromPath, _ := cmd.Flags().GetString("from")
	if fromPath == "" {
		return output.NewError(
			"design_from_required",
			"--from is required: a design document's content is read from a file, never from prose on the command line",
		).WithNextAction("stage the document on disk and reissue with --from <path to that file>")
	}
	content, err := os.ReadFile(fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromPath, err)
	}
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	if err := set.Write(input.Document(), content); err != nil {
		return err
	}
	location, err := set.Resolve(input.Document())
	if err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{
		"source":   input.Source,
		"path":     input.Path,
		"location": location,
	})
}

func init() {
	designCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")

	designReadCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"source":"api","path":"payments/v2.md"}')`)
	designWriteCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"source":"api","path":"payments/v2.md"}')`)
	designWriteCmd.Flags().String("from", "", "Read the document's content from the file at <path> (relative to cwd)")
	designListCmd.Flags().StringVar(&designSource, "source", "", "Narrow the listing to one declared source; omit to list every source")

	designCmd.AddCommand(designSourcesCmd, designListCmd, designReadCmd, designWriteCmd)
}
