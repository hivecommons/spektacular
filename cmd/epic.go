package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/identifier"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// The epic commands do NOT go through newStoreFileCmd, the factory behind
// `spec file`, `plan file` and `changelog file`. That factory routes every
// write through metadata.Merge, whose shared frontmatter models a design's
// `specs` as a list of bare names: an epic's `specs` graph of
// {name, depends_on} entries would be silently dropped on the first write.
// Epics have their own frontmatter type (internal/epic) and their own verbs,
// `epic read / write / list / delete / split / summary`, with no `file` level, and every
// write keeps the epic and its specs in agreement (cmd/epic_link.go).

var epicCmd = &cobra.Command{
	Use:   "epic",
	Short: "Read, write, list, delete and split epics: groups of specs with the dependencies between them",
	RunE:  runUnknownSubcommand,
}

var epicReadCmd = &cobra.Command{
	Use:   "read <name>",
	Short: "Read one epic, writing its bytes to stdout unchanged",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEpicRead,
}

var epicWriteCmd = &cobra.Command{
	Use:   "write <name>",
	Short: "Write an epic's body and, through --data, its specs, dependencies and sources, linking each spec to it",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEpicWrite,
}

var epicListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the epics in the epic store",
	Args:  cobra.NoArgs,
	RunE:  runEpicList,
}

var epicDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an epic, clearing the epic field on every spec it lists",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runEpicDelete,
}

// sourceInput is one source as a caller supplies it: a link, and optionally
// the date it was retrieved, which the CLI stamps with today when omitted.
type sourceInput struct {
	URI           string `json:"uri"`
	RetrievedDate string `json:"retrieved_date,omitempty"`
}

// epicWriteInput is the --data payload for epic write. Every field is
// optional, and an omitted field keeps the epic's current value, so a
// body-only rewrite never drops the graph.
type epicWriteInput struct {
	Specs   *[]epic.EpicSpec `json:"specs"`
	Sources *[]sourceInput   `json:"sources"`
	Spec    *string          `json:"spec"`
	// ConfirmCompletedEpic is the user's go-ahead to add a spec to an epic
	// whose specs are all implemented.
	ConfirmCompletedEpic bool `json:"confirm_completed_epic"`
}

var sourceItemSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"uri":            {Type: "string"},
	"retrieved_date": {Type: "string", Description: "YYYY-MM-DD; stamped with today when omitted"},
}}

var epicSpecItemSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"name":          {Type: "string"},
	"depends_on":    {Type: "array", Items: &schemaProp{Type: "string"}, Description: "names of specs in this epic it depends on; [] when none"},
	"parallel_with": {Type: "array", Items: &schemaProp{Type: "string"}, Description: "earlier specs the user allowed to run side by side despite shared files; maintained by epic order, omit to leave unset"},
}}

var epicWriteInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"specs":                  {Type: "array", Items: epicSpecItemSchema, Description: "the epic's specs in display order; omit to keep the current list"},
		"sources":                {Type: "array", Items: sourceItemSchema, Description: "what directly seeded the epic; omit to keep the current list"},
		"spec":                   {Type: "string", Description: "the spec whose split produced the epic; omit to keep"},
		"confirm_completed_epic": {Type: "boolean", Description: "true only after the user agrees to add a spec to an epic whose specs are all implemented"},
	},
}

var epicWriteOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name":     {Type: "string"},
		"path":     {Type: "string"},
		"specs":    {Type: "array", Items: &schemaProp{Type: "string"}},
		"linked":   {Type: "array", Items: &schemaProp{Type: "string"}},
		"unlinked": {Type: "array", Items: &schemaProp{Type: "string"}},
	},
}

var epicListOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"files": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"name":            {Type: "string"},
			"path":            {Type: "string"},
			"modified_at":     {Type: "string"},
			"created_date":    {Type: "string"},
			"document_status": {Type: "string"},
			"closed_date":     {Type: "string"},
		}}},
	},
}

var epicDeleteOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name":     {Type: "string"},
		"deleted":  {Type: "boolean"},
		"unlinked": {Type: "array", Items: &schemaProp{Type: "string"}},
	},
}

var epicReadOutputSchema = &schemaObj{
	Type:       "object",
	Properties: map[string]*schemaProp{"content": {Type: "string", Description: "the epic's raw bytes, written to stdout unchanged"}},
}

// epicStore loads the config and builds the project store.
func epicStore() (config.Config, store.Store, error) {
	root, err := projectRoot()
	if err != nil {
		return config.Config{}, nil, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, store.NewSourceStore(root, "project"), nil
}

// epicName parses the single <name> argument.
func epicName(command string, args []string) (string, error) {
	if len(args) == 0 {
		return "", output.NewError("epic_name_required", "an epic name is required").
			WithNextAction(fmt.Sprintf("reissue naming the epic; run `%s epic list` to see the stored epics", command))
	}
	addr, err := artifact.Parse(artifact.KindEpic, args)
	if err != nil {
		var extErr *artifact.ExtensionError
		if errors.As(err, &extErr) {
			return "", output.NewError(artifact.ErrCodeUnexpectedExtension,
				fmt.Sprintf("%q carries a file extension or path; epics are addressed by bare name", args[0])).
				WithResource(args[0]).
				WithNextAction(fmt.Sprintf("reissue with the bare name %q", extErr.Corrected.Feature))
		}
		return "", output.NewError("bad_input", fmt.Sprintf("invalid epic name %q: %v", args[0], err)).
			WithResource(args[0]).
			WithNextAction(fmt.Sprintf("run `%s epic list` to see the names it accepts", command))
	}
	return addr.Feature, nil
}

func epicNotFound(cfg config.Config, name string) error {
	return output.NewError("epic_not_found", fmt.Sprintf("no epic named %q is stored in this project", name)).
		WithResource(name).
		WithNextAction(fmt.Sprintf("run `%s epic list` to see the stored epics", cfg.Command))
}

// readEpic reads and parses the named epic, returning its raw bytes too.
func readEpic(cfg config.Config, st store.Store, name string) (epic.Epic, []byte, error) {
	raw, err := st.Read(epicPath(cfg, name))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return epic.Epic{}, nil, epicNotFound(cfg, name)
		}
		return epic.Epic{}, nil, err
	}
	e, err := epic.Parse(raw)
	if err != nil {
		return epic.Epic{}, nil, output.NewError("epic_malformed", fmt.Sprintf("epic %q cannot be read: %v", name, err)).
			WithResource(name).
			WithNextAction(fmt.Sprintf("repair the epic's frontmatter, or rewrite it with `%s epic write %s --from <body> --data '{\"specs\":[…]}'`", cfg.Command, name))
	}
	return e, raw, nil
}

// stampSources validates caller-supplied sources and stamps each missing
// retrieval date with today.
func stampSources(cfg config.Config, in []sourceInput, today time.Time) ([]metadata.SourceRef, error) {
	out := make([]metadata.SourceRef, 0, len(in))
	for i, s := range in {
		if s.URI == "" {
			return nil, output.NewError("sources_invalid", fmt.Sprintf("sources[%d] has no uri", i)).
				WithResource(fmt.Sprintf("sources[%d]", i)).
				WithNextAction(`give every source a link, e.g. "sources":[{"uri":"https://github.com/org/repo/issues/45"}]; the CLI stamps retrieved_date itself`)
		}
		date := s.RetrievedDate
		if date == "" {
			date = today.Format(metadata.DateFormat)
		} else if _, err := time.Parse(metadata.DateFormat, date); err != nil {
			return nil, output.NewError("sources_invalid", fmt.Sprintf("sources[%d] has retrieved_date %q, which is not YYYY-MM-DD", i, date)).
				WithResource(fmt.Sprintf("sources[%d]", i)).
				WithNextAction("omit retrieved_date to have the CLI stamp today's date, or give it as YYYY-MM-DD")
		}
		out = append(out, metadata.SourceRef{URI: s.URI, RetrievedDate: date})
	}
	return out, nil
}

// resolveNewEpicName names a new epic. A name that already carries an ID
// matching spec.id_method is used as given (a split's epic takes its spec's
// name); a bare name is given an ID with the configured method, run against
// the epic directory, exactly as spec new names a spec.
func resolveNewEpicName(cfg config.Config, st store.Store, name string) (string, error) {
	if identifier.HasPrefix(cfg.Spec.IDMethod, name) {
		return name, nil
	}
	res, err := identifier.Resolve(identifier.Request{
		Name:   name,
		Method: cfg.Spec.IDMethod,
		Dir:    cfg.Epic.Config.Directory,
		Store:  st,
		PathFunc: func(dir, n string) string {
			return artifact.Address{Kind: artifact.KindEpic, Feature: n}.StorePath(dir)
		},
	})
	if err != nil {
		return "", output.NewError("epic_name_invalid", fmt.Sprintf("cannot name epic %q: %v", name, err)).
			WithResource(name).
			WithNextAction("reissue with a short lowercase name made of letters, digits and hyphens")
	}
	return res.Name, nil
}

func runEpicRead(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: epicReadOutputSchema}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	name, err := epicName(cfg.Command, args)
	if err != nil {
		return err
	}
	_, raw, err := readEpic(cfg, st, name)
	if err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write(raw)
	return err
}

func runEpicWrite(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Input:  epicWriteInputSchema,
			Output: epicWriteOutputSchema,
			Flags:  map[string]*schemaProp{"from": {Type: "string"}, "document-status": {Type: "string"}},
		}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	name, err := epicName(cfg.Command, args)
	if err != nil {
		return err
	}
	fromPath, _ := cmd.Flags().GetString("from")
	if fromPath == "" {
		return output.NewError("epic_from_required",
			"--from is required: an epic's body is read from a file, never from prose on the command line").
			WithNextAction(fmt.Sprintf("stage the body (## Overview and ## Specs) under .spektacular/tmp/ and reissue `%s epic write %s --from <that file>`", cfg.Command, name))
	}
	content, err := os.ReadFile(fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromPath, err)
	}
	var input epicWriteInput
	if dataStr, _ := cmd.Flags().GetString("data"); dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
			return output.NewError("bad_input", fmt.Sprintf("parsing --data: %v", err)).
				WithNextAction(`reissue with --data as a JSON object, e.g. '{"specs":[{"name":"000061_example","depends_on":[]}]}'`)
		}
	}
	var status *metadata.DocumentStatus
	if raw, _ := cmd.Flags().GetString("document-status"); raw != "" {
		s, err := parseDocumentStatusFlag(raw)
		if err != nil {
			return err
		}
		status = &s
	}

	var existing *epic.Epic
	current, _, err := readEpic(cfg, st, name)
	switch {
	case err == nil:
		existing = &current
	case isCode(err, "epic_not_found"):
		if name, err = resolveNewEpicName(cfg, st, name); err != nil {
			return err
		}
	default:
		return err
	}

	next := epic.Epic{Specs: []epic.EpicSpec{}, Body: stripLeadingFrontmatterBlocks(content)}
	if existing != nil {
		next.Spec, next.Specs, next.Sources = existing.Spec, existing.Specs, existing.Sources
	}
	if input.Specs != nil {
		next.Specs = *input.Specs
	}
	if input.Spec != nil {
		next.Spec = *input.Spec
	}
	today := time.Now().UTC()
	if input.Sources != nil {
		if next.Sources, err = stampSources(cfg, *input.Sources, today); err != nil {
			return err
		}
	}
	if err := epic.Validate(next.Specs); err != nil {
		return err
	}
	if next, err = epic.Stamp(existing, next, status, today); err != nil {
		return output.NewError("invalid_document_status", err.Error()).
			WithNextAction(fmt.Sprintf("pass --document-status with one of %s", documentStatusValues()))
	}

	links := epicLinks{epic: name, newList: next.SpecNames()}
	if existing != nil {
		links.oldList = existing.SpecNames()
	}
	if added, _ := links.diff(); existing != nil && len(added) > 0 {
		retry := fmt.Sprintf("the same `%s epic write %s` with \"confirm_completed_epic\": true added to --data", cfg.Command, name)
		if err := refuseCompletedEpic(cfg, st, name, input.ConfirmCompletedEpic, retry); err != nil {
			return err
		}
	}
	if err := checkEpicLinks(st, cfg, links); err != nil {
		return err
	}
	rendered, err := next.Render()
	if err != nil {
		return err
	}
	linked, unlinked, err := applyEpicLinks(newDocTxn(st), cfg, links, rendered)
	if err != nil {
		return err
	}
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{
		"name":     name,
		"path":     reportedLocation(centralLocationBase, epicPath(cfg, name)),
		"specs":    nonNil(next.SpecNames()),
		"linked":   nonNil(linked),
		"unlinked": nonNil(unlinked),
	})
}

func runEpicList(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: epicListOutputSchema}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	dir := cfg.Epic.Config.Directory
	files := make([]map[string]any, 0)
	entries, err := st.List(dir)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	for _, e := range entries {
		name, ok := artifact.NameFromEntry(e.Name, e.IsDir, artifact.EntryFile)
		if !ok {
			continue
		}
		path := filepath.Join(dir, e.Name)
		item := map[string]any{
			"name": name,
			"path": reportedLocation(centralLocationBase, path),
		}
		if !e.ModTime.IsZero() {
			item["modified_at"] = e.ModTime.UTC().Format(time.RFC3339)
		}
		if raw, readErr := st.Read(path); readErr == nil {
			if parsed, parseErr := epic.Parse(raw); parseErr == nil && !parsed.CreatedDate.IsZero() {
				item["created_date"] = parsed.CreatedDate.Format(metadata.DateFormat)
				item["document_status"] = string(parsed.DocumentStatus)
				if !parsed.ClosedDate.IsZero() {
					item["closed_date"] = parsed.ClosedDate.Format(metadata.DateFormat)
				}
			}
		}
		files = append(files, item)
	}
	return output.Write(cmd.OutOrStdout(), map[string]any{"files": files}, "")
}

func runEpicDelete(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: epicDeleteOutputSchema}, "")
	}
	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	name, err := epicName(cfg.Command, args)
	if err != nil {
		return err
	}
	current, _, err := readEpic(cfg, st, name)
	if err != nil {
		return err
	}
	// Remove the planning summary, unlink every member, then remove the
	// epic, all inside one transaction: a failure part-way leaves the epic,
	// its summary and every spec as they were.
	t := newDocTxn(st)
	if err := t.delete(summaryPath(cfg, name)); err != nil {
		return t.fail(err)
	}
	var unlinked []string
	for _, member := range current.SpecNames() {
		spec, readErr := readSpecFile(st, cfg, member)
		if errors.Is(readErr, store.ErrNotFound) {
			continue
		}
		if readErr != nil {
			return t.fail(readErr)
		}
		if spec.epic() != name {
			continue
		}
		if err := writeEpicLinkFn(t, spec, ""); err != nil {
			return t.fail(err)
		}
		unlinked = append(unlinked, member)
	}
	if err := t.delete(epicPath(cfg, name)); err != nil {
		return t.fail(err)
	}
	// The summary's folder goes last, once nothing can roll back into it. A
	// rollback rewrites the summary, which recreates the folder, so the folder
	// itself is not part of the transaction. A folder that still holds
	// anything else is left in place, so its removal error is ignored.
	_ = st.Delete(summaryDir(cfg, name))
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{
		"name":     name,
		"deleted":  true,
		"unlinked": nonNil(unlinked),
	})
}

// nonNil returns s, or an empty slice when s is nil, so a JSON list is never
// written as null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// isCode reports whether err is a structured refusal with the given code.
func isCode(err error, code string) bool {
	var resp *output.ErrorResponse
	return errors.As(err, &resp) && resp.Code == code
}

func init() {
	epicCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")
	epicWriteCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"specs":[{"name":"000061_example","depends_on":[]}],"sources":[{"uri":"https://…"}]}')`)
	epicWriteCmd.Flags().String("from", "", "Read the epic's body from the file at <path> (relative to cwd)")
	epicWriteCmd.Flags().String("document-status", "", "Optional document status to apply: one of "+documentStatusValues())
	epicCmd.AddCommand(epicReadCmd, epicWriteCmd, epicListCmd, epicDeleteCmd)
}
