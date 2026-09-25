package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/identifier"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// storeDirFunc selects the configured, project-root-relative directory that a
// `file` subcommand group operates within.
type storeDirFunc func(config.Config) string

// writeValidator checks a document's body, stripped of front matter, before a
// `file write` stores it. docPath is the store-relative path being written.
type writeValidator func(cfg config.Config, docPath string, body []byte) error

// stripLeadingFrontmatterBlocks removes zero or more leading YAML frontmatter
// blocks from raw. Each `<kind> file write` is idempotent under repeated
// invocation: the caller may pass source content that already carries a
// frontmatter block (e.g. because they cp'd a stored artifact), and the write
// handler must treat that block as prior metadata to be discarded so
// metadata.Merge can prepend a single, freshly-authored block. Malformed or
// absent frontmatter is not an error — Split returns the input unchanged and
// the loop terminates.
func stripLeadingFrontmatterBlocks(raw []byte) []byte {
	body := raw
	for {
		fm, next, err := metadata.Split(body)
		if err != nil || fm == nil {
			return body
		}
		body = next
	}
}

// metadataOptsForDocumentStatus parses the shared `--document-status` flag
// value into an UpdateOptions payload. An empty value means "no document
// status change" (Merge picks the default for a fresh write and preserves an
// existing artifact's status, blank included); any other value must be one of
// the four document statuses, and is rejected with an actionable error
// otherwise.
func metadataOptsForDocumentStatus(raw string) (metadata.UpdateOptions, error) {
	if raw == "" {
		return metadata.UpdateOptions{}, nil
	}
	s, err := parseDocumentStatusFlag(raw)
	if err != nil {
		return metadata.UpdateOptions{}, err
	}
	return metadata.UpdateOptions{DocumentStatus: &s}, nil
}

// validateIDPrefix checks that the leading path segment of a store-relative
// write path carries an ID matching the configured spec.id_method scheme
// (e.g. "000034_feature/plan.md" for counter, "20260709062525-feature.md"
// for timestamp). Plan and changelog entries must reuse the ID minted for
// their originating spec rather than mint their own, so a write whose name
// lacks a matching ID is rejected with guidance to reuse the spec's ID.
func validateIDPrefix(cfg config.Config, writePath string) error {
	name := writePath
	if idx := strings.IndexAny(name, "/\\"); idx >= 0 {
		name = name[:idx]
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))

	if identifier.HasPrefix(cfg.Spec.IDMethod, name) {
		return nil
	}
	return output.NewError("missing_id_prefix",
		fmt.Sprintf("%q has no ID prefix matching the configured spec.id_method (%q) — reuse the ID from the originating spec's filename rather than inventing a new one", writePath, cfg.Spec.IDMethod)).
		WithResource(writePath).
		WithNextAction("Find the originating spec's ID (e.g. via `spec file list`) and prefix this name with it, matching spec.id_method's format.")
}

// storeFileStore builds a store rooted at the project root and returns it
// alongside the configured directory the `file` subcommands resolve paths
// against.
func storeFileStore(dir storeDirFunc) (store.Store, string, error) {
	root, err := projectRoot()
	if err != nil {
		return nil, "", err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", err
	}
	return store.NewSourceStore(root, "project"), dir(cfg), nil
}

// repoRoutedStore builds a store rooted at the named member repo, targeting
// that repo's own configured changelog directory namespaced by the project's
// name — the routing derived per-repo changelog entries write through. A
// missing or broken footprint surfaces as the standard repair offer.
func repoRoutedStore(repoName string) (store.Store, string, error) {
	root, err := projectRoot()
	if err != nil {
		return nil, "", err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", err
	}
	set, err := repo.New(cfg, root, repoGit)
	if err != nil {
		return nil, "", err
	}
	resolved, err := set.Resolve(repoName)
	if err != nil {
		if refusal := formatRefusal(err); refusal != nil {
			return nil, "", refusal
		}
		var fpErr *repo.FootprintError
		if errors.As(err, &fpErr) {
			return nil, "", output.NewError(
				"repo_footprint",
				fpErr.Error(),
			).WithResource(fpErr.Root).
				WithNextAction(fmt.Sprintf("run `%s repo add --data '{\"name\":%q}'` to repair the repo's footprint", cfg.Command, repoName))
		}
		return nil, "", err
	}

	rc, err := config.RepoConfigFromYAMLFile(filepath.Join(resolved.Root, config.RepoConfigFileName))
	if err != nil {
		return nil, "", err
	}
	dir := filepath.Join(rc.Changelog.Config.Directory, cfg.Name)
	return store.NewSourceStore(resolved.Root, "repo:"+repoName), dir, nil
}

// provenanceOpts returns the merge options a repo-routed changelog write is
// stamped with: the project's name and source, plus the spec and plan
// identifiers derived from the written filename (the plan-slug-equals-
// spec-slug convention). Stamping is mechanical and CLI-owned so derived
// entries always carry reliable provenance regardless of what the staged
// body contains.
func provenanceOpts(cfg config.Config, writePath string) metadata.UpdateOptions {
	slug := strings.TrimSuffix(filepath.Base(writePath), filepath.Ext(writePath))
	return metadata.UpdateOptions{
		Project:       cfg.Name,
		ProjectSource: cfg.Source,
		Spec:          slug,
		Plan:          slug,
	}
}

// newStoreFileCmd builds a `file` subcommand group (write/read/delete/list)
// that reads and writes files within a configured store directory. Path
// arguments are resolved relative to that directory, so callers pass a file
// name rather than a full project path. It backs `spec file`, `plan file`,
// and `changelog file`, which differ only in which configured directory they
// target and whether writes require an ID prefix.
//
// requireID gates ID-prefix validation on write. Spec is where an ID is
// born (resolved by ResolveIdentifier before spec new ever calls this write
// path), so `spec file` passes false. Plan and changelog names must reuse
// that same spec ID rather than mint their own, so `plan file` and
// `changelog file` pass true: a write whose leading path segment lacks an ID
// matching the configured spec.id_method scheme is rejected.
//
// repoRouted adds an optional `--repo <name>` flag to write, read, and list:
// when set, the command operates on the named member repo's own changelog
// store (rooted at the resolved repo, namespaced by the project name) instead
// of the central one, and writes are auto-stamped with provenance front
// matter. Only the changelog group opts in.
//
// validate, when non-nil, checks a document's body before it is stored. A
// refusal returns before anything is written, so the stored document is left
// exactly as it was. Only the plan group sets one.
func newStoreFileCmd(short string, dir storeDirFunc, requireID, repoRouted bool, validate writeValidator) *cobra.Command {
	fileCmd := &cobra.Command{Use: "file", Short: short, RunE: runUnknownSubcommand}

	// resolveStore picks the central store or, when repoRouted and the
	// command's --repo flag is set, the named member repo's store.
	resolveStore := func(repoName string) (store.Store, string, error) {
		if repoName == "" {
			return storeFileStore(dir)
		}
		return repoRoutedStore(repoName)
	}

	var (
		fromPath           string
		documentStatusFlag string
		writeRepoName      string
	)
	write := &cobra.Command{
		Use:   "write <path>",
		Short: "Write the contents of a source file into the store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if requireID {
				if err := validateIDPrefix(cfg, args[0]); err != nil {
					return err
				}
			}
			st, storeDir, err := resolveStore(writeRepoName)
			if err != nil {
				return err
			}
			content, err := os.ReadFile(fromPath)
			if err != nil {
				return fmt.Errorf("reading source file %q: %w", fromPath, err)
			}
			storePath := filepath.Join(storeDir, args[0])
			existing, err := st.Read(storePath)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			opts, err := metadataOptsForDocumentStatus(documentStatusFlag)
			if err != nil {
				return err
			}
			if writeRepoName != "" {
				prov := provenanceOpts(cfg, args[0])
				opts.Project = prov.Project
				opts.ProjectSource = prov.ProjectSource
				opts.Spec = prov.Spec
				opts.Plan = prov.Plan
			}
			body := stripLeadingFrontmatterBlocks(content)
			if validate != nil {
				if err := validate(cfg, args[0], body); err != nil {
					return err
				}
			}
			merged, err := metadata.Merge(existing, body, opts)
			if err != nil {
				return output.NewError("metadata_merge_failed", err.Error()).WithResource(args[0])
			}
			return st.Write(storePath, merged)
		},
	}
	write.Flags().StringVar(&fromPath, "from", "", "Path to the source file whose contents will be written into the store")
	_ = write.MarkFlagRequired("from")
	write.Flags().StringVar(&documentStatusFlag, "document-status", "", "Optional document status to apply: one of "+documentStatusValues())

	var readRepoName string
	read := &cobra.Command{
		Use:   "read <path>",
		Short: "Read a file from the store and write it to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, storeDir, err := resolveStore(readRepoName)
			if err != nil {
				return err
			}
			content, err := st.Read(filepath.Join(storeDir, args[0]))
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return output.NewError("not_found", fmt.Sprintf("file %q not found", args[0])).
						WithResource(args[0])
				}
				return err
			}
			_, err = cmd.OutOrStdout().Write(content)
			return err
		},
	}

	del := &cobra.Command{
		Use:   "delete <path>",
		Short: "Delete a file from the store",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			st, storeDir, err := storeFileStore(dir)
			if err != nil {
				return err
			}
			return st.Delete(filepath.Join(storeDir, args[0]))
		},
	}

	var (
		listDocumentStatus string
		listCreatedAfter   string
		listCreatedBefore  string
		listClosedAfter    string
		listClosedBefore   string
		listRepoName       string
	)
	list := &cobra.Command{
		Use:   "list [path]",
		Short: "List files in the store, optionally filtered by metadata",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter, err := parseListFilter(listDocumentStatus, listCreatedAfter, listCreatedBefore, listClosedAfter, listClosedBefore)
			if err != nil {
				return err
			}
			st, storeDir, err := resolveStore(listRepoName)
			if err != nil {
				return err
			}
			path := storeDir
			if len(args) > 0 {
				path = filepath.Join(storeDir, args[0])
			}
			entries, err := st.List(path)
			if err != nil {
				return err
			}
			files := make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				entryPath := filepath.Join(path, e.Name)
				storeRel := filepath.ToSlash(strings.TrimPrefix(entryPath, st.Root()+string(filepath.Separator)))
				item := map[string]any{
					"name": e.Name,
					"path": storeRel,
				}
				// modified_at is the store's modification time for the entry
				// itself: a file's content change, or for a directory the
				// last child added or removed. It is omitted when the backend
				// cannot report one, and it is never workflow activity.
				if !e.ModTime.IsZero() {
					item["modified_at"] = e.ModTime.UTC().Format(time.RFC3339)
				}
				var fm *metadata.Metadata
				if !e.IsDir {
					raw, readErr := st.Read(entryPath)
					if readErr == nil {
						if parsed, _, splitErr := metadata.Split(raw); splitErr == nil && parsed != nil {
							fm = parsed
							item["created_date"] = parsed.CreatedDate.Format("2006-01-02")
							item["document_status"] = string(parsed.DocumentStatus)
							if !parsed.ClosedDate.IsZero() {
								item["closed_date"] = parsed.ClosedDate.Format("2006-01-02")
							}
						}
					}
				}
				if filter.active() {
					if e.IsDir || fm == nil {
						continue
					}
					if !filter.matches(*fm) {
						continue
					}
				}
				files = append(files, item)
			}
			return output.Write(cmd.OutOrStdout(), map[string]any{"files": files}, "")
		},
	}
	list.Flags().StringVar(&listDocumentStatus, "document-status", "", "Filter to artifacts whose document status matches; one of "+documentStatusValues())
	list.Flags().StringVar(&listCreatedAfter, "created-after", "", "Filter to artifacts whose created_date is on or after this YYYY-MM-DD date")
	list.Flags().StringVar(&listCreatedBefore, "created-before", "", "Filter to artifacts whose created_date is on or before this YYYY-MM-DD date")
	list.Flags().StringVar(&listClosedAfter, "closed-after", "", "Filter to artifacts whose closed_date is on or after this YYYY-MM-DD date")
	list.Flags().StringVar(&listClosedBefore, "closed-before", "", "Filter to artifacts whose closed_date is on or before this YYYY-MM-DD date")

	if repoRouted {
		const repoFlagHelp = "Route through the named registered repo's own changelog store instead of the central one"
		write.Flags().StringVar(&writeRepoName, "repo", "", repoFlagHelp)
		read.Flags().StringVar(&readRepoName, "repo", "", repoFlagHelp)
		list.Flags().StringVar(&listRepoName, "repo", "", repoFlagHelp)
	}

	var setDocumentStatusFlag string
	setDocumentStatus := &cobra.Command{
		Use:   "set-document-status <path>",
		Short: "Update the document status of a stored artifact without rewriting its body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if requireID {
				if err := validateIDPrefix(cfg, args[0]); err != nil {
					return err
				}
			}
			opts, err := metadataOptsForDocumentStatus(setDocumentStatusFlag)
			if err != nil {
				return err
			}
			if opts.DocumentStatus == nil {
				return output.NewError("missing_document_status",
					"--document-status is required for set-document-status").
					WithNextAction(fmt.Sprintf("Pass --document-status with one of %s.", documentStatusValues()))
			}
			st, storeDir, err := storeFileStore(dir)
			if err != nil {
				return err
			}
			storePath := filepath.Join(storeDir, args[0])
			existing, err := st.Read(storePath)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return output.NewError("not_found", fmt.Sprintf("file %q not found", args[0])).
						WithResource(args[0])
				}
				return err
			}
			body := existing
			if _, split, splitErr := metadata.Split(existing); splitErr == nil {
				body = split
			}
			merged, err := metadata.Merge(existing, body, opts)
			if err != nil {
				return output.NewError("metadata_merge_failed", err.Error()).WithResource(args[0])
			}
			if err := st.Write(storePath, merged); err != nil {
				return err
			}
			fm, _, splitErr := metadata.Split(merged)
			payload := map[string]any{
				"path":            args[0],
				"document_status": string(*opts.DocumentStatus),
			}
			if splitErr == nil && fm != nil && !fm.ClosedDate.IsZero() {
				payload["closed_date"] = fm.ClosedDate.Format("2006-01-02")
			}
			return output.Write(cmd.OutOrStdout(), payload, "")
		},
	}
	setDocumentStatus.Flags().StringVar(&setDocumentStatusFlag, "document-status", "", "Document status to apply: one of "+documentStatusValues())
	_ = setDocumentStatus.MarkFlagRequired("document-status")

	fileCmd.AddCommand(write, read, del, list, setDocumentStatus)
	return fileCmd
}
