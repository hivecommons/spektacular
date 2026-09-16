package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/metadata"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// Kind discriminants emitted per hit in the `artifacts list` envelope. The
// four plan.* variants map to the four sibling doc types inside a plan
// directory; specs and changelog entries are top-level classes with a single
// kind each.
const (
	artifactKindSpec         = "spec"
	artifactKindPlanPlan     = "plan.plan"
	artifactKindPlanContext  = "plan.context"
	artifactKindPlanResearch = "plan.research"
	artifactKindPlanTestPlan = "plan.test-plan"
	artifactKindChangelog    = "changelog"
)

// allArtifactKinds is the enumeration used to validate the `--kind` flag and
// to seed the default (unfiltered) scan set.
var allArtifactKinds = []string{
	artifactKindSpec,
	artifactKindPlanPlan,
	artifactKindPlanContext,
	artifactKindPlanResearch,
	artifactKindPlanTestPlan,
	artifactKindChangelog,
}

// planSiblingKinds maps a plan directory's sibling file names to the kind
// discriminant they should be tagged with in the output envelope.
var planSiblingKinds = map[string]string{
	"plan.md":      artifactKindPlanPlan,
	"context.md":   artifactKindPlanContext,
	"research.md":  artifactKindPlanResearch,
	"test-plan.md": artifactKindPlanTestPlan,
}

var artifactsCmd = &cobra.Command{
	Use:   "artifacts",
	Short: "Query workflow-produced artifacts across every kind",
	RunE:  runUnknownSubcommand,
}

func init() {
	var (
		listKinds          string
		listDocumentStatus string
		listCreatedAfter   string
		listCreatedBefore  string
		listClosedAfter    string
		listClosedBefore   string
	)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts across specs, plans, and changelog entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			kinds, err := parseKindFlag(listKinds)
			if err != nil {
				return err
			}
			filter, err := parseListFilter(listDocumentStatus, listCreatedAfter, listCreatedBefore, listClosedAfter, listClosedBefore)
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			root, err := projectRoot()
			if err != nil {
				return err
			}
			st := store.NewSourceStore(root, "project")

			artifacts := make([]map[string]any, 0)
			artifacts, err = appendSpecArtifacts(artifacts, st, cfg, kinds, filter)
			if err != nil {
				return err
			}
			artifacts, err = appendPlanArtifacts(artifacts, st, cfg, kinds, filter)
			if err != nil {
				return err
			}
			artifacts, err = appendChangelogArtifacts(artifacts, st, cfg, kinds, filter)
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), map[string]any{"artifacts": artifacts}, "")
		},
	}
	listCmd.Flags().StringVar(&listKinds, "kind", "", "Comma-separated list of artifact kinds to include (default: all). One or more of: spec, plan.plan, plan.context, plan.research, plan.test-plan, changelog")
	listCmd.Flags().StringVar(&listDocumentStatus, "document-status", "", "Filter to artifacts whose document status matches; one of "+documentStatusValues())
	listCmd.Flags().StringVar(&listCreatedAfter, "created-after", "", "Filter to artifacts whose created_date is on or after this YYYY-MM-DD date")
	listCmd.Flags().StringVar(&listCreatedBefore, "created-before", "", "Filter to artifacts whose created_date is on or before this YYYY-MM-DD date")
	listCmd.Flags().StringVar(&listClosedAfter, "closed-after", "", "Filter to artifacts whose closed_date is on or after this YYYY-MM-DD date")
	listCmd.Flags().StringVar(&listClosedBefore, "closed-before", "", "Filter to artifacts whose closed_date is on or before this YYYY-MM-DD date")
	artifactsCmd.AddCommand(listCmd)
}

// parseKindFlag validates the comma-separated --kind value against
// allArtifactKinds. An empty raw string means "all kinds"; anything else must
// name only known values.
func parseKindFlag(raw string) (map[string]bool, error) {
	if strings.TrimSpace(raw) == "" {
		set := make(map[string]bool, len(allArtifactKinds))
		for _, k := range allArtifactKinds {
			set[k] = true
		}
		return set, nil
	}
	valid := make(map[string]bool, len(allArtifactKinds))
	for _, k := range allArtifactKinds {
		valid[k] = true
	}
	set := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if !valid[p] {
			return nil, output.NewError("invalid_kind",
				fmt.Sprintf("--kind %q is not one of the recognised artifact classes", p)).
				WithNextAction(fmt.Sprintf("Pass one or more of: %s", strings.Join(allArtifactKinds, ", ")))
		}
		set[p] = true
	}
	if len(set) == 0 {
		return nil, output.NewError("invalid_kind",
			"--kind must name at least one artifact class").
			WithNextAction(fmt.Sprintf("Pass one or more of: %s", strings.Join(allArtifactKinds, ", ")))
	}
	return set, nil
}

// scanArtifact reads a single artifact through the store, splits its
// frontmatter, and returns the entry map to append (or nil to skip). The
// filter is applied here — bare (no-frontmatter) artifacts drop out of any
// active filter but appear in unfiltered scans.
func scanArtifact(st store.Store, kind, storePath, name string, filter artifactFilter) (map[string]any, error) {
	raw, err := st.Read(storePath)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	entry := map[string]any{
		"kind": kind,
		"name": name,
		"path": filepath.ToSlash(strings.TrimPrefix(storePath, st.Root()+string(filepath.Separator))),
	}
	fm, _, splitErr := metadata.Split(raw)
	if splitErr == nil && fm != nil {
		entry["created_date"] = fm.CreatedDate.Format("2006-01-02")
		entry["document_status"] = string(fm.DocumentStatus)
		if !fm.ClosedDate.IsZero() {
			entry["closed_date"] = fm.ClosedDate.Format("2006-01-02")
		}
	}
	if filter.active() {
		if fm == nil {
			return nil, nil
		}
		if !filter.matches(*fm) {
			return nil, nil
		}
	}
	return entry, nil
}

func appendSpecArtifacts(out []map[string]any, st store.Store, cfg config.Config, kinds map[string]bool, filter artifactFilter) ([]map[string]any, error) {
	if !kinds[artifactKindSpec] {
		return out, nil
	}
	specDir := cfg.Spec.Config.Directory
	entries, err := st.List(specDir)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".md") {
			continue
		}
		entry, err := scanArtifact(st, artifactKindSpec, filepath.Join(specDir, e.Name), e.Name, filter)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			out = append(out, entry)
		}
	}
	return out, nil
}

func appendPlanArtifacts(out []map[string]any, st store.Store, cfg config.Config, kinds map[string]bool, filter artifactFilter) ([]map[string]any, error) {
	// Skip the whole plans scan if none of the plan.* kinds are requested.
	anyPlanKind := kinds[artifactKindPlanPlan] || kinds[artifactKindPlanContext] || kinds[artifactKindPlanResearch] || kinds[artifactKindPlanTestPlan]
	if !anyPlanKind {
		return out, nil
	}
	plansDir := cfg.Plan.Config.Directory
	dirs, err := st.List(plansDir)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return out, nil
		}
		return nil, err
	}
	for _, planDirEntry := range dirs {
		if !planDirEntry.IsDir {
			continue
		}
		planPath := filepath.Join(plansDir, planDirEntry.Name)
		siblings, err := st.List(planPath)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, err
		}
		for _, s := range siblings {
			if s.IsDir {
				continue
			}
			kind, ok := planSiblingKinds[s.Name]
			if !ok {
				continue
			}
			if !kinds[kind] {
				continue
			}
			entry, err := scanArtifact(st, kind, filepath.Join(planPath, s.Name), s.Name, filter)
			if err != nil {
				return nil, err
			}
			if entry != nil {
				out = append(out, entry)
			}
		}
	}
	return out, nil
}

func appendChangelogArtifacts(out []map[string]any, st store.Store, cfg config.Config, kinds map[string]bool, filter artifactFilter) ([]map[string]any, error) {
	if !kinds[artifactKindChangelog] {
		return out, nil
	}
	// Changelog entries live one folder level down, under the project-named
	// namespace folder; a store predating any entry simply has no folder yet.
	changelogDir := filepath.Join(cfg.Changelog.Config.Directory, cfg.Name)
	entries, err := st.List(changelogDir)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".md") {
			continue
		}
		entry, err := scanArtifact(st, artifactKindChangelog, filepath.Join(changelogDir, e.Name), e.Name, filter)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			out = append(out, entry)
		}
	}
	return out, nil
}
