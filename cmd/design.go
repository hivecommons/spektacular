package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hivecommons/spektacular/internal/design"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
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

// designAuthorCmd is the sibling of designWriteCmd for a design Spektacular
// wrote with the user rather than one the team handed over. The two verbs
// exist separately, rather than as one verb with a flag, because whether a
// document is stamped is the whole distinction between the two classes of
// design document, and a flag that silently decides it is exactly the kind of
// thing an agent adds or omits by accident.
var designAuthorCmd = &cobra.Command{
	Use:   "author",
	Short: "Write one design document into a declared source, stamping Spektacular's lifecycle metadata",
	RunE:  runDesignAuthor,
}

// designDeleteCmd removes one design document. It is a hand-written sibling of
// the other design verbs for the same reason they all are (see the comment at
// the top of this file): removal is another verb this family needs, not a
// reason to reconsider the shared store-file factory, whose lifecycle-stamping
// write path a design document must not acquire.
var designDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete one design document from a declared source",
	RunE:  runDesignDelete,
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

// designDocumentItemSchema covers both classes of design document. Every
// document reports its source and path; only one Spektacular authored reports
// the lifecycle keys, and a document the project already had carries none of
// them, which is what makes the two classes distinguishable from a listing
// alone.
var designDocumentItemSchema = map[string]*schemaProp{
	"source":          {Type: "string"},
	"path":            {Type: "string"},
	"created_date":    {Type: "string"},
	"document_status": {Type: "string"},
	"closed_date":     {Type: "string"},
	"spec":            {Type: "string"},
	"specs":           {Type: "array", Items: &schemaProp{Type: "string"}},
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

// designDeleteOutputSchema adds "deleted" to the write result's shape, which
// distinguishes the two successful outcomes: a document was there and was
// removed, or the address was valid and held nothing. Both are successes, so
// a caller tidying up can report what it actually changed without parsing
// prose.
var designDeleteOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"source":   {Type: "string"},
		"path":     {Type: "string"},
		"location": {Type: "string"},
		"deleted":  {Type: "boolean"},
	},
}

// designAuthorOutputSchema adds the two facts a caller most needs to confirm
// after stamping a block: which status was applied, and whether this was a
// first write or an update to a document that already existed, which the
// created date answers. Without them the caller would have to follow every
// author with a read to learn what it just wrote.
var designAuthorOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"source":          {Type: "string"},
		"path":            {Type: "string"},
		"location":        {Type: "string"},
		"document_status": {Type: "string"},
		"created_date":    {Type: "string"},
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

// authoredMetadata reports whether a design document is one Spektacular
// authored, returning its lifecycle block if so and nil if not.
//
// A design source holds two kinds of document and the document itself is the
// discriminator: one carrying a block Spektacular wrote is ours, one without
// is the team's. That keeps the fact in the only place that cannot drift from
// the document it describes, which is why there is no registry or naming rule.
//
// The error case is deliberately swallowed rather than propagated, and that is
// the whole reason this helper exists instead of three inline calls to Split.
// Probed against the real parser: a Spektacular block yields (non-nil, nil), a
// document with no leading --- yields (nil, nil), and a document carrying the
// team's own frontmatter such as title:/author: yields (nil, error), because
// UnmarshalYAML cannot parse the created_date that is not there. An
// unterminated block does the same. Propagating that error would make `design
// list` and `design ref add` fail outright on a perfectly ordinary design file
// that happens to carry a YAML header of the team's own, which is precisely the
// file this feature promises not to disturb. So an error means not authored.
func authoredMetadata(raw []byte) *metadata.Metadata {
	fm, _, err := metadata.Split(raw)
	if err != nil {
		return nil
	}
	return fm
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
		item := map[string]any{"source": d.Source, "path": d.Path}
		// A document that cannot be read is still listed, as its address is
		// what the listing is for; it simply reports no lifecycle fields.
		if raw, readErr := set.Read(d); readErr == nil {
			if fm := authoredMetadata(raw); fm != nil {
				item["created_date"] = fm.CreatedDate.Format("2006-01-02")
				item["document_status"] = string(fm.DocumentStatus)
				if !fm.ClosedDate.IsZero() {
					item["closed_date"] = fm.ClosedDate.Format("2006-01-02")
				}
				// Provenance and back-links are omitted when absent rather
				// than reported empty, so a listing never invites a caller to
				// distinguish "no originating spec" from "the empty string".
				if fm.Spec != "" {
					item["spec"] = fm.Spec
				}
				if len(fm.Specs) > 0 {
					item["specs"] = fm.Specs
				}
			}
		}
		items = append(items, item)
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
	// Refuse to clobber a document Spektacular authored. A verbatim overwrite
	// would strip its lifecycle block and with it every back-link, leaving
	// specs referencing a design that no longer lists them, and it would do so
	// without raising anything. The check is here rather than in
	// internal/design because that package has no notion of metadata and must
	// not gain one, and because this is the layer holding both the resolved
	// path and the name of the sibling command to point at.
	if exists, existsErr := set.Exists(input.Document()); existsErr != nil {
		return existsErr
	} else if exists {
		raw, readErr := set.Read(input.Document())
		if readErr != nil {
			return readErr
		}
		if authoredMetadata(raw) != nil {
			location, resolveErr := set.Resolve(input.Document())
			if resolveErr != nil {
				location = input.Path
			}
			return output.NewError(
				"design_authored_overwrite",
				fmt.Sprintf("%s carries a lifecycle record Spektacular wrote, and a verbatim write would strip it along with every spec referencing this design", location),
			).WithResource(location).WithNextAction(
				// Quoted so the next action is copy-pasteable as-is; nesting
				// single quotes inside single quotes would not be.
				fmt.Sprintf(`rewrite it with 'design author --data {"source":%q,"path":%q} --from <path>', which preserves its capture date and the specs referencing it`, input.Source, input.Path))
		}
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

// refuseIfReferenced refuses to remove a design that one or more specs still
// reference, and returns nil when nothing references it.
//
// The set of referencing specs is read from the document's own lifecycle
// record rather than found by scanning the spec store. That list is maintained
// transactionally with the spec side by writeBackLink (cmd/design_ref.go), so
// it is the same record `design list` and `design ref list` already report
// from, and reading it costs one read rather than a walk.
//
// authoredMetadata is used rather than metadata.Split precisely because it
// swallows a parse error: a design carrying the team's own YAML header must
// read as unauthored rather than blowing up. A document with no lifecycle
// block, or one whose specs list is empty, therefore falls straight through to
// the removal — which is the correct answer, not an oversight, because a
// design the project did not author carries no record of referencing specs.
//
// Nothing is written on this path in either direction. The document is left
// byte for byte as it was, and no spec is touched: clearing a reference stays
// an explicit, separate act by the caller, which is also why there is no
// compensating-rollback problem here.
func refuseIfReferenced(set *design.Set, input designAddressInput) error {
	raw, err := set.Read(input.Document())
	if err != nil {
		return err
	}
	fm := authoredMetadata(raw)
	if fm == nil || len(fm.Specs) == 0 {
		return nil
	}
	location, resolveErr := set.Resolve(input.Document())
	if resolveErr != nil {
		location = input.Path
	}
	steps := make([]string, 0, len(fm.Specs))
	for _, spec := range fm.Specs {
		// Quoted so each step is copy-pasteable as-is; nesting single quotes
		// inside single quotes would not be.
		steps = append(steps, fmt.Sprintf(`design ref remove --data '{"spec":%q,"source":%q,"path":%q}'`,
			spec, input.Source, input.Path))
	}
	return output.NewError(
		"design_referenced_delete",
		fmt.Sprintf("%s is still referenced by %s, and removing it would leave %s pointing at a document that is not there; nothing has been changed",
			location, specList(fm.Specs), pluralSpecSubject(len(fm.Specs))),
	).WithResource(location).WithNextAction(fmt.Sprintf(
		"clear each reference first, then retry the delete: %s, then design delete --data '{\"source\":%q,\"path\":%q}'",
		strings.Join(steps, "; "), input.Source, input.Path))
}

// specList renders spec names for a refusal message, naming every one of them
// rather than the first and a count: a caller has to clear each reference
// individually, so each name is a step it needs.
func specList(specs []string) string {
	quoted := make([]string, len(specs))
	for i, s := range specs {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	switch len(quoted) {
	case 1:
		return "the spec " + quoted[0]
	case 2:
		return "the specs " + quoted[0] + " and " + quoted[1]
	default:
		return "the specs " + strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}

// pluralSpecSubject keeps the refusal's second clause grammatical whether one
// spec references the document or several.
func pluralSpecSubject(n int) string {
	if n == 1 {
		return "that spec"
	}
	return "those specs"
}

// runDesignDelete removes one addressed design document.
//
// Whether anything was there is settled before the removal with Exists,
// because the storage contract's Delete returns nil either way and cannot
// report it. The write path already calls Exists for the same reason, so this
// is the family's existing shape rather than a new one.
func runDesignDelete(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: designAddressInputSchema, Output: designDeleteOutputSchema}, "")
	}
	input, err := designAddressData(cmd)
	if err != nil {
		return err
	}
	set, err := newDesignSet()
	if err != nil {
		return err
	}
	deleted, err := set.Exists(input.Document())
	if err != nil {
		return err
	}
	if deleted {
		if err := refuseIfReferenced(set, input); err != nil {
			return err
		}
	}
	if err := set.Delete(input.Document()); err != nil {
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
		"deleted":  deleted,
	})
}

// runDesignAuthor stores the bytes of the file named by --from with
// Spektacular's lifecycle block merged in. It is runDesignWrite with a merge
// step in front of it, which is exactly the shape the feature intends: the
// block is produced by the same code that produces a spec's or a plan's, and
// the bytes land through the same byte-level store path as any other design
// write.
func runDesignAuthor(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  designAddressInputSchema,
			Output: designAuthorOutputSchema,
			Flags: map[string]*schemaProp{
				"from":            {Type: "string"},
				"document-status": {Type: "string"},
				"spec":            {Type: "string"},
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

	documentStatus, _ := cmd.Flags().GetString("document-status")
	opts, err := metadataOptsForDocumentStatus(documentStatus)
	if err != nil {
		return err
	}
	opts.Spec, _ = cmd.Flags().GetString("spec")
	// opts.Specs is deliberately left nil. Back-links are owned by the
	// reference verbs, and nil is what preserves them across a revision.

	set, err := newDesignSet()
	if err != nil {
		return err
	}
	doc := input.Document()

	// A document that is not there yet is the ordinary first-write case, not
	// a failure, so Exists is asked rather than letting Read's refusal decide
	// it: distinguishing them by matching on an error string would couple this
	// to the wording of a refusal.
	var existing []byte
	exists, err := set.Exists(doc)
	if err != nil {
		return err
	}
	if exists {
		existing, err = set.Read(doc)
		if err != nil {
			return err
		}
	}

	body := stripLeadingFrontmatterBlocks(content)
	merged, err := metadata.Merge(existing, body, opts)
	if err != nil {
		// Merge refuses frontmatter it cannot parse rather than replacing it,
		// which is right: a block the team wrote is theirs. The raw error does
		// not say that, so it is reworded into something the caller can act on.
		location, resolveErr := set.Resolve(doc)
		if resolveErr != nil {
			location = input.Path
		}
		// The underlying cause is deliberately not repeated. It reads as a
		// complaint about a missing created_date, which invites the caller to
		// add one to the team's block rather than leave the block alone.
		return output.NewError(
			"design_frontmatter_not_authored",
			fmt.Sprintf("%s already carries a frontmatter block Spektacular did not write, so a lifecycle record cannot be merged into it", location),
		).WithResource(location).WithNextAction(
			"remove that frontmatter block from the document if it is no longer wanted, or author to a different path and keep the original as one of your own")
	}

	if err := set.Write(doc, merged); err != nil {
		return err
	}
	location, err := set.Resolve(doc)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"source":   input.Source,
		"path":     input.Path,
		"location": location,
	}
	if fm, _, splitErr := metadata.Split(merged); splitErr == nil && fm != nil {
		payload["document_status"] = string(fm.DocumentStatus)
		payload["created_date"] = fm.CreatedDate.Format("2006-01-02")
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(payload)
}

func init() {
	designCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")

	designReadCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"source":"api","path":"payments/v2.md"}')`)
	designWriteCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"source":"api","path":"payments/v2.md"}')`)
	designWriteCmd.Flags().String("from", "", "Read the document's content from the file at <path> (relative to cwd)")
	designAuthorCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"source":"api","path":"payments/v2.md"}')`)
	designAuthorCmd.Flags().String("from", "", "Read the document's content from the file at <path> (relative to cwd)")
	designAuthorCmd.Flags().String("document-status", "", "Optional document status to apply: one of "+documentStatusValues())
	designAuthorCmd.Flags().String("spec", "", "Optional name of the spec whose conversation produced this design")
	designDeleteCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"source":"api","path":"payments/v2.md"}')`)
	designListCmd.Flags().StringVar(&designSource, "source", "", "Narrow the listing to one declared source; omit to list every source")

	designCmd.AddCommand(designSourcesCmd, designListCmd, designReadCmd, designWriteCmd, designAuthorCmd, designDeleteCmd)
}
